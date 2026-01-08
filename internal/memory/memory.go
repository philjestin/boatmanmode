// Package memory provides cross-session learning capabilities.
// It persists patterns, preferences, and lessons learned across sessions.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Memory stores learned patterns and preferences.
type Memory struct {
	mu sync.RWMutex

	// ProjectID identifies the project (usually repo path hash)
	ProjectID string `json:"project_id"`

	// Patterns stores code patterns learned from successful PRs
	Patterns []Pattern `json:"patterns"`

	// CommonIssues stores frequently encountered issues
	CommonIssues []CommonIssue `json:"common_issues"`

	// SuccessfulPrompts stores prompts that led to passing reviews
	SuccessfulPrompts []PromptRecord `json:"successful_prompts"`

	// FilePatterns stores patterns for specific file types/paths
	FilePatterns map[string][]string `json:"file_patterns"`

	// Preferences stores learned preferences
	Preferences Preferences `json:"preferences"`

	// Stats tracks success rates and timing
	Stats SessionStats `json:"stats"`

	// LastUpdated is when memory was last modified
	LastUpdated time.Time `json:"last_updated"`

	// path is where this memory is stored
	path string
}

// Pattern represents a learned code pattern.
type Pattern struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // "naming", "structure", "testing", "api", etc.
	Description string    `json:"description"`
	Example     string    `json:"example,omitempty"`
	FileMatcher string    `json:"file_matcher,omitempty"` // Glob pattern for applicable files
	Weight      float64   `json:"weight"`                  // How strongly to apply (0-1)
	UsageCount  int       `json:"usage_count"`
	SuccessRate float64   `json:"success_rate"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CommonIssue represents a frequently encountered issue.
type CommonIssue struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // "style", "logic", "security", "performance", etc.
	Description string    `json:"description"`
	Solution    string    `json:"solution"`
	Frequency   int       `json:"frequency"` // Times encountered
	AutoFix     bool      `json:"auto_fix"`  // Can be auto-fixed
	FileMatcher string    `json:"file_matcher,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// PromptRecord stores a successful prompt.
type PromptRecord struct {
	ID           string    `json:"id"`
	TicketType   string    `json:"ticket_type"`   // Feature type/category
	Prompt       string    `json:"prompt"`        // The prompt that worked
	Result       string    `json:"result"`        // Brief description of result
	SuccessScore int       `json:"success_score"` // 0-100
	CreatedAt    time.Time `json:"created_at"`
}

// Preferences stores learned preferences.
type Preferences struct {
	// PreferredTestFramework is the detected/preferred test framework
	PreferredTestFramework string `json:"preferred_test_framework"`

	// NamingConventions stores naming preferences
	NamingConventions map[string]string `json:"naming_conventions"`

	// FileOrganization stores preferred file organization
	FileOrganization map[string]string `json:"file_organization"`

	// CodeStyle stores code style preferences
	CodeStyle map[string]string `json:"code_style"`

	// CommitMessageFormat stores preferred commit format
	CommitMessageFormat string `json:"commit_message_format"`

	// ReviewerThresholds stores review pass thresholds
	ReviewerThresholds map[string]int `json:"reviewer_thresholds"`
}

// SessionStats tracks historical statistics.
type SessionStats struct {
	TotalSessions       int           `json:"total_sessions"`
	SuccessfulSessions  int           `json:"successful_sessions"`
	TotalIterations     int           `json:"total_iterations"`
	AvgIterationsPerPR  float64       `json:"avg_iterations_per_pr"`
	AvgDuration         time.Duration `json:"avg_duration"`
	CommonFailurePoints []string      `json:"common_failure_points"`
}

// Store manages memory persistence.
type Store struct {
	baseDir string
	cache   map[string]*Memory
	mu      sync.RWMutex
}

// NewStore creates a new memory store.
func NewStore(baseDir string) (*Store, error) {
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		baseDir = filepath.Join(homeDir, ".boatman", "memory")
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create memory directory: %w", err)
	}

	return &Store{
		baseDir: baseDir,
		cache:   make(map[string]*Memory),
	}, nil
}

// Get retrieves or creates memory for a project.
func (s *Store) Get(projectPath string) (*Memory, error) {
	projectID := hashPath(projectPath)

	s.mu.RLock()
	if mem, ok := s.cache[projectID]; ok {
		s.mu.RUnlock()
		return mem, nil
	}
	s.mu.RUnlock()

	// Load from disk
	memPath := filepath.Join(s.baseDir, projectID+".json")
	mem := &Memory{
		ProjectID:    projectID,
		path:         memPath,
		FilePatterns: make(map[string][]string),
		Preferences: Preferences{
			NamingConventions:  make(map[string]string),
			FileOrganization:   make(map[string]string),
			CodeStyle:          make(map[string]string),
			ReviewerThresholds: make(map[string]int),
		},
	}

	if data, err := os.ReadFile(memPath); err == nil {
		if err := json.Unmarshal(data, mem); err != nil {
			// Corrupted file, start fresh
			mem = &Memory{
				ProjectID:    projectID,
				path:         memPath,
				FilePatterns: make(map[string][]string),
				Preferences: Preferences{
					NamingConventions:  make(map[string]string),
					FileOrganization:   make(map[string]string),
					CodeStyle:          make(map[string]string),
					ReviewerThresholds: make(map[string]int),
				},
			}
		}
	}

	s.mu.Lock()
	s.cache[projectID] = mem
	s.mu.Unlock()

	return mem, nil
}

// Save persists memory to disk.
func (s *Store) Save(mem *Memory) error {
	mem.mu.Lock()
	mem.LastUpdated = time.Now()
	data, err := json.MarshalIndent(mem, "", "  ")
	mem.mu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to marshal memory: %w", err)
	}

	return os.WriteFile(mem.path, data, 0644)
}

// LearnPattern adds or updates a pattern.
func (mem *Memory) LearnPattern(p Pattern) {
	mem.mu.Lock()
	defer mem.mu.Unlock()

	// Check for existing pattern
	for i, existing := range mem.Patterns {
		if existing.ID == p.ID {
			// Update existing
			mem.Patterns[i].UsageCount++
			mem.Patterns[i].UpdatedAt = time.Now()
			if p.SuccessRate > 0 {
				// Weighted average
				count := float64(mem.Patterns[i].UsageCount)
				mem.Patterns[i].SuccessRate = (mem.Patterns[i].SuccessRate*(count-1) + p.SuccessRate) / count
			}
			return
		}
	}

	// Add new pattern
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	p.UsageCount = 1
	mem.Patterns = append(mem.Patterns, p)

	// Limit patterns
	if len(mem.Patterns) > 100 {
		// Remove lowest weight patterns
		sort.Slice(mem.Patterns, func(i, j int) bool {
			return mem.Patterns[i].Weight > mem.Patterns[j].Weight
		})
		mem.Patterns = mem.Patterns[:100]
	}
}

// LearnIssue records a common issue.
func (mem *Memory) LearnIssue(issue CommonIssue) {
	mem.mu.Lock()
	defer mem.mu.Unlock()

	// Check for existing
	for i, existing := range mem.CommonIssues {
		if similar(existing.Description, issue.Description) {
			mem.CommonIssues[i].Frequency++
			if issue.Solution != "" && existing.Solution == "" {
				mem.CommonIssues[i].Solution = issue.Solution
			}
			return
		}
	}

	// Add new
	issue.CreatedAt = time.Now()
	issue.Frequency = 1
	mem.CommonIssues = append(mem.CommonIssues, issue)

	// Limit
	if len(mem.CommonIssues) > 50 {
		sort.Slice(mem.CommonIssues, func(i, j int) bool {
			return mem.CommonIssues[i].Frequency > mem.CommonIssues[j].Frequency
		})
		mem.CommonIssues = mem.CommonIssues[:50]
	}
}

// LearnPrompt records a successful prompt.
func (mem *Memory) LearnPrompt(ticketType, prompt, result string, score int) {
	mem.mu.Lock()
	defer mem.mu.Unlock()

	record := PromptRecord{
		ID:           fmt.Sprintf("%s-%d", ticketType, time.Now().Unix()),
		TicketType:   ticketType,
		Prompt:       prompt,
		Result:       result,
		SuccessScore: score,
		CreatedAt:    time.Now(),
	}

	mem.SuccessfulPrompts = append(mem.SuccessfulPrompts, record)

	// Keep only high-scoring prompts
	if len(mem.SuccessfulPrompts) > 20 {
		sort.Slice(mem.SuccessfulPrompts, func(i, j int) bool {
			return mem.SuccessfulPrompts[i].SuccessScore > mem.SuccessfulPrompts[j].SuccessScore
		})
		mem.SuccessfulPrompts = mem.SuccessfulPrompts[:20]
	}
}

// UpdateStats updates session statistics.
func (mem *Memory) UpdateStats(successful bool, iterations int, duration time.Duration) {
	mem.mu.Lock()
	defer mem.mu.Unlock()

	mem.Stats.TotalSessions++
	if successful {
		mem.Stats.SuccessfulSessions++
	}
	mem.Stats.TotalIterations += iterations

	// Update averages
	if mem.Stats.SuccessfulSessions > 0 {
		mem.Stats.AvgIterationsPerPR = float64(mem.Stats.TotalIterations) / float64(mem.Stats.SuccessfulSessions)
	}

	// Rolling average for duration
	if mem.Stats.AvgDuration == 0 {
		mem.Stats.AvgDuration = duration
	} else {
		mem.Stats.AvgDuration = (mem.Stats.AvgDuration + duration) / 2
	}
}

// GetPatternsForFile returns patterns applicable to a file path.
func (mem *Memory) GetPatternsForFile(filePath string) []Pattern {
	mem.mu.RLock()
	defer mem.mu.RUnlock()

	var applicable []Pattern

	for _, p := range mem.Patterns {
		if p.FileMatcher == "" {
			applicable = append(applicable, p)
			continue
		}

		matched, _ := filepath.Match(p.FileMatcher, filePath)
		if matched {
			applicable = append(applicable, p)
		}
	}

	// Sort by weight
	sort.Slice(applicable, func(i, j int) bool {
		return applicable[i].Weight > applicable[j].Weight
	})

	return applicable
}

// GetCommonIssuesForFile returns common issues for a file path.
func (mem *Memory) GetCommonIssuesForFile(filePath string) []CommonIssue {
	mem.mu.RLock()
	defer mem.mu.RUnlock()

	var applicable []CommonIssue

	for _, issue := range mem.CommonIssues {
		if issue.FileMatcher == "" {
			applicable = append(applicable, issue)
			continue
		}

		matched, _ := filepath.Match(issue.FileMatcher, filePath)
		if matched {
			applicable = append(applicable, issue)
		}
	}

	// Sort by frequency
	sort.Slice(applicable, func(i, j int) bool {
		return applicable[i].Frequency > applicable[j].Frequency
	})

	return applicable
}

// GetBestPromptForType returns the best prompt for a ticket type.
func (mem *Memory) GetBestPromptForType(ticketType string) *PromptRecord {
	mem.mu.RLock()
	defer mem.mu.RUnlock()

	var best *PromptRecord
	bestScore := 0

	for i, p := range mem.SuccessfulPrompts {
		if p.TicketType == ticketType && p.SuccessScore > bestScore {
			best = &mem.SuccessfulPrompts[i]
			bestScore = p.SuccessScore
		}
	}

	return best
}

// ToContext formats memory as context for agents.
func (mem *Memory) ToContext(maxTokens int) string {
	mem.mu.RLock()
	defer mem.mu.RUnlock()

	var sb strings.Builder

	sb.WriteString("# Project Memory\n\n")

	// Add key patterns
	if len(mem.Patterns) > 0 {
		sb.WriteString("## Key Patterns\n")
		count := 0
		for _, p := range mem.Patterns {
			if p.Weight >= 0.7 && count < 10 {
				sb.WriteString(fmt.Sprintf("- [%s] %s", p.Type, p.Description))
				if p.Example != "" {
					sb.WriteString(fmt.Sprintf(" (e.g., %s)", truncate(p.Example, 50)))
				}
				sb.WriteString("\n")
				count++
			}
		}
		sb.WriteString("\n")
	}

	// Add common issues
	if len(mem.CommonIssues) > 0 {
		sb.WriteString("## Common Issues to Avoid\n")
		count := 0
		for _, issue := range mem.CommonIssues {
			if issue.Frequency >= 2 && count < 5 {
				sb.WriteString(fmt.Sprintf("- %s", issue.Description))
				if issue.Solution != "" {
					sb.WriteString(fmt.Sprintf(" → %s", issue.Solution))
				}
				sb.WriteString("\n")
				count++
			}
		}
		sb.WriteString("\n")
	}

	// Add preferences
	if mem.Preferences.PreferredTestFramework != "" {
		sb.WriteString("## Preferences\n")
		sb.WriteString(fmt.Sprintf("- Test framework: %s\n", mem.Preferences.PreferredTestFramework))
		for k, v := range mem.Preferences.NamingConventions {
			sb.WriteString(fmt.Sprintf("- Naming (%s): %s\n", k, v))
		}
		for k, v := range mem.Preferences.CodeStyle {
			sb.WriteString(fmt.Sprintf("- Style (%s): %s\n", k, v))
		}
		sb.WriteString("\n")
	}

	result := sb.String()

	// Truncate if needed
	if len(result)/4 > maxTokens {
		return result[:maxTokens*4] + "\n... (memory truncated)"
	}

	return result
}

// FormatStats returns formatted statistics.
func (mem *Memory) FormatStats() string {
	mem.mu.RLock()
	defer mem.mu.RUnlock()

	s := mem.Stats
	successRate := 0.0
	if s.TotalSessions > 0 {
		successRate = float64(s.SuccessfulSessions) / float64(s.TotalSessions) * 100
	}

	return fmt.Sprintf(
		"Sessions: %d (%d successful, %.1f%% rate)\n"+
			"Avg iterations per PR: %.1f\n"+
			"Avg duration: %s\n"+
			"Patterns learned: %d\n"+
			"Common issues tracked: %d",
		s.TotalSessions, s.SuccessfulSessions, successRate,
		s.AvgIterationsPerPR,
		s.AvgDuration.Round(time.Second),
		len(mem.Patterns),
		len(mem.CommonIssues),
	)
}

// Helper functions

func hashPath(path string) string {
	// Simple hash for project ID
	h := uint32(2166136261) // FNV offset
	for _, c := range path {
		h ^= uint32(c)
		h *= 16777619 // FNV prime
	}
	return fmt.Sprintf("p%08x", h)
}

func similar(a, b string) bool {
	// Simple similarity check
	a = strings.ToLower(a)
	b = strings.ToLower(b)

	if a == b {
		return true
	}

	// Check if one contains the other
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return true
	}

	// Word overlap
	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)

	overlap := 0
	for _, wa := range wordsA {
		for _, wb := range wordsB {
			if wa == wb {
				overlap++
			}
		}
	}

	minLen := len(wordsA)
	if len(wordsB) < minLen {
		minLen = len(wordsB)
	}

	if minLen == 0 {
		return false
	}

	return float64(overlap)/float64(minLen) > 0.5
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// Analyzer extracts patterns from successful completions.
type Analyzer struct {
	mem *Memory
}

// NewAnalyzer creates a pattern analyzer.
func NewAnalyzer(mem *Memory) *Analyzer {
	return &Analyzer{mem: mem}
}

// AnalyzeSuccess extracts patterns from a successful completion.
func (a *Analyzer) AnalyzeSuccess(filesChanged []string, reviewScore int) {
	// Analyze file paths for organization patterns
	for _, file := range filesChanged {
		dir := filepath.Dir(file)
		ext := filepath.Ext(file)
		base := filepath.Base(file)

		// Learn file organization patterns
		if dir != "." {
			key := ext + "_location"
			a.mem.mu.Lock()
			if a.mem.Preferences.FileOrganization == nil {
				a.mem.Preferences.FileOrganization = make(map[string]string)
			}
			a.mem.Preferences.FileOrganization[key] = dir
			a.mem.mu.Unlock()
		}

		// Learn naming patterns
		if strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, "_spec.rb") {
			a.mem.LearnPattern(Pattern{
				ID:          "test_naming_" + ext,
				Type:        "naming",
				Description: fmt.Sprintf("Test files for %s use suffix pattern", ext),
				FileMatcher: "*" + ext,
				Weight:      0.8,
			})
		}
	}

	// If high score, record patterns
	if reviewScore >= 80 {
		a.mem.LearnPattern(Pattern{
			ID:          fmt.Sprintf("success_%d", time.Now().Unix()),
			Type:        "success",
			Description: fmt.Sprintf("Pattern from %d-file change scored %d", len(filesChanged), reviewScore),
			Weight:      float64(reviewScore) / 100.0,
		})
	}
}

// AnalyzeIssue learns from encountered issues.
func (a *Analyzer) AnalyzeIssue(severity, description, suggestion, file string) {
	issueType := "general"
	if strings.Contains(strings.ToLower(description), "security") {
		issueType = "security"
	} else if strings.Contains(strings.ToLower(description), "performance") {
		issueType = "performance"
	} else if strings.Contains(strings.ToLower(description), "style") {
		issueType = "style"
	}

	a.mem.LearnIssue(CommonIssue{
		ID:          fmt.Sprintf("issue_%d", time.Now().Unix()),
		Type:        issueType,
		Description: description,
		Solution:    suggestion,
		FileMatcher: "*" + filepath.Ext(file),
	})
}
