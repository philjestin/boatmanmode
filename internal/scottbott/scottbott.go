// Package scottbott provides the ScottBott peer-review integration.
// It invokes the existing peer-review Claude skill in the target repository.
package scottbott

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/philjestin/boatmanmode/internal/config"
	"github.com/philjestin/boatmanmode/internal/cost"
)

// ReviewResult represents the outcome of a code review.
type ReviewResult struct {
	Passed   bool     `json:"passed"`
	Score    int      `json:"score"`
	Summary  string   `json:"summary"`
	Issues   []Issue  `json:"issues"`
	Praise   []string `json:"praise"`
	Guidance string   `json:"guidance"`
}

// Issue represents a specific problem found during review.
type Issue struct {
	Severity    string `json:"severity"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
}

// ScottBott invokes the review skill.
type ScottBott struct {
	workDir             string
	sessionName         string
	outputDir           string
	skill               string
	model               string
	enablePromptCaching bool
	cfg                 *config.Config
}

// New creates a new ScottBott instance.
func New(cfg *config.Config) *ScottBott {
	return &ScottBott{
		sessionName:         "reviewer",
		outputDir:           filepath.Join(os.TempDir(), "boatman-sessions"),
		skill:               cfg.ReviewSkill,
		model:               cfg.Claude.Models.Reviewer,
		enablePromptCaching: cfg.Claude.EnablePromptCaching,
		cfg:                 cfg,
	}
}

// NewForIteration creates a ScottBott for a specific review iteration.
func NewForIteration(iteration int, cfg *config.Config) *ScottBott {
	return &ScottBott{
		sessionName:         fmt.Sprintf("reviewer-%d", iteration),
		outputDir:           filepath.Join(os.TempDir(), "boatman-sessions"),
		skill:               cfg.ReviewSkill,
		model:               cfg.Claude.Models.Reviewer,
		enablePromptCaching: cfg.Claude.EnablePromptCaching,
		cfg:                 cfg,
	}
}

// NewWithWorkDir creates a ScottBott that runs in a specific directory.
func NewWithWorkDir(workDir string, iteration int, cfg *config.Config) *ScottBott {
	return &ScottBott{
		workDir:             workDir,
		sessionName:         fmt.Sprintf("reviewer-%d", iteration),
		outputDir:           filepath.Join(os.TempDir(), "boatman-sessions"),
		skill:               cfg.ReviewSkill,
		model:               cfg.Claude.Models.Reviewer,
		enablePromptCaching: cfg.Claude.EnablePromptCaching,
		cfg:                 cfg,
	}
}

// NewWithSkill creates a ScottBott with a specific skill/agent for review.
func NewWithSkill(workDir string, iteration int, skill string, cfg *config.Config) *ScottBott {
	if skill == "" {
		skill = "peer-review"
	}
	return &ScottBott{
		workDir:             workDir,
		sessionName:         fmt.Sprintf("reviewer-%d", iteration),
		outputDir:           filepath.Join(os.TempDir(), "boatman-sessions"),
		skill:               skill,
		model:               cfg.Claude.Models.Reviewer,
		enablePromptCaching: cfg.Claude.EnablePromptCaching,
		cfg:                 cfg,
	}
}

// Review performs a code review using the peer-review Claude skill.
// Note: Usage data is not available when using the skill/agent mode as it uses text output.
func (s *ScottBott) Review(ctx context.Context, ticketContext, diff string) (*ReviewResult, *cost.Usage, error) {
	os.MkdirAll(s.outputDir, 0755)

	// Write the review prompt to a file
	promptFile := filepath.Join(s.outputDir, fmt.Sprintf("%s-prompt.txt", s.sessionName))
	prompt := formatReviewPrompt(ticketContext, diff)
	if err := os.WriteFile(promptFile, []byte(prompt), 0644); err != nil {
		return nil, nil, fmt.Errorf("failed to write prompt: %w", err)
	}
	defer os.Remove(promptFile)

	// Output file for capturing response
	outputFile := filepath.Join(s.outputDir, fmt.Sprintf("%s.out", s.sessionName))

	fmt.Printf("   📏 Review: %d chars context, %d chars diff\n", len(ticketContext), len(diff))
	fmt.Printf("   🔍 Invoking %s skill...\n", s.skill)

	start := time.Now()

	// Invoke Claude with the configured review agent/skill
	// The skill should exist in the repo's .claude/ directory
	args := []string{
		"-p",
		"--agent", s.skill,
		"--output-format", "text",
	}

	// Add model if specified
	if s.model != "" {
		args = append(args, "--model", s.model)
	}

	// Note: Prompt caching is automatically handled by Claude CLI when using system prompts
	// No explicit flag needed in current version (2.1.39+)

	cmd := exec.CommandContext(ctx, "claude", args...)

	// Pipe the prompt via stdin
	promptContent, _ := os.ReadFile(promptFile)
	cmd.Stdin = strings.NewReader(string(promptContent))

	if s.workDir != "" {
		cmd.Dir = s.workDir
	}

	output, err := cmd.Output()
	elapsed := time.Since(start)

	if err != nil {
		// If skill doesn't exist, fall back to system prompt
		fmt.Printf("   ⚠️  %s skill not found, using fallback...\n", s.skill)
		return s.reviewWithFallback(ctx, ticketContext, diff)
	}

	fmt.Printf("   ⏱️  Review completed in %s\n", elapsed.Round(time.Second))

	// Save output for debugging
	os.WriteFile(outputFile, output, 0644)

	// Parse the response
	response := strings.TrimSpace(string(output))
	result, err := s.parseReviewResponse(response)
	// Text output format doesn't include usage data
	return result, nil, err
}

// reviewWithFallback uses a system prompt if peer-review skill isn't available.
func (s *ScottBott) reviewWithFallback(ctx context.Context, ticketContext, diff string) (*ReviewResult, *cost.Usage, error) {
	systemPrompt := `You are a senior staff engineer conducting a peer code review.
Be thorough, constructive, and focused on correctness, security, and maintainability.

Respond with ONLY a JSON object:
{
  "passed": boolean,
  "score": number (0-100),
  "summary": "2-3 sentence summary",
  "issues": [{"severity": "critical|major|minor", "file": "path", "line": 0, "description": "what's wrong", "suggestion": "how to fix"}],
  "praise": ["good things"],
  "guidance": "guidance for fixing if failed"
}

Pass if: no critical issues, ≤2 major issues, code meets requirements.`

	prompt := formatReviewPrompt(ticketContext, diff)

	promptFile := filepath.Join(s.outputDir, fmt.Sprintf("%s-fallback-prompt.txt", s.sessionName))
	sysFile := filepath.Join(s.outputDir, fmt.Sprintf("%s-fallback-system.txt", s.sessionName))

	os.WriteFile(promptFile, []byte(prompt), 0644)
	os.WriteFile(sysFile, []byte(systemPrompt), 0644)
	defer os.Remove(promptFile)
	defer os.Remove(sysFile)

	start := time.Now()

	cmd := exec.CommandContext(ctx, "claude",
		"-p",
		"--output-format", "text",
		"--system-prompt", systemPrompt,
	)
	cmd.Stdin = strings.NewReader(prompt)

	if s.workDir != "" {
		cmd.Dir = s.workDir
	}

	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		return nil, nil, fmt.Errorf("review failed: %w\nOutput: %s", err, string(output))
	}

	fmt.Printf("   ⏱️  Review completed in %s\n", elapsed.Round(time.Second))

	result, err := s.parseReviewResponse(strings.TrimSpace(string(output)))
	// Text output format doesn't include usage data
	return result, nil, err
}

// formatReviewPrompt creates the prompt for code review.
func formatReviewPrompt(ticketContext, diff string) string {
	return fmt.Sprintf(`## Ticket Context
%s

## Code Changes
%s

Review these changes against the requirements. Provide your assessment.`, ticketContext, diff)
}

// parseReviewResponse extracts ReviewResult from Claude's response.
func (s *ScottBott) parseReviewResponse(response string) (*ReviewResult, error) {
	// First try JSON extraction
	jsonStr := extractJSON(response)

	var result ReviewResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
		return &result, nil
	}

	// If JSON parsing fails, parse natural language response
	return s.parseNaturalLanguageReview(response)
}

// parseNaturalLanguageReview extracts review info from a natural language response.
func (s *ScottBott) parseNaturalLanguageReview(response string) (*ReviewResult, error) {
	lower := strings.ToLower(response)

	result := &ReviewResult{
		Score:  70, // Default score
		Issues: []Issue{},
		Praise: []string{},
	}

	// Determine pass/fail
	// Look for explicit pass indicators
	if strings.Contains(lower, "lgtm") ||
		strings.Contains(lower, "looks good") ||
		strings.Contains(lower, "approved") ||
		strings.Contains(lower, "ready to merge") ||
		strings.Contains(lower, "no critical issues") && !strings.Contains(lower, "major issues") {
		result.Passed = true
		result.Score = 85
	}

	// Look for explicit fail indicators - only if strict parsing is enabled
	if s.cfg != nil && s.cfg.Review.StrictParsing {
		if strings.Contains(lower, "must be addressed") ||
			strings.Contains(lower, "blocking") ||
			strings.Contains(lower, "critical issue") ||
			strings.Contains(lower, "cannot be merged") ||
			strings.Contains(lower, "needs work") ||
			strings.Contains(lower, "issues that need to be addressed") {
			result.Passed = false
			result.Score = 50
		}
	} else {
		// In relaxed mode, only fail on very explicit blocking language
		if strings.Contains(lower, "cannot be merged") ||
			strings.Contains(lower, "blocking issue") {
			result.Passed = false
			result.Score = 50
		}
	}

	// Extract summary - first paragraph or first 300 chars
	lines := strings.Split(response, "\n")
	var summaryBuilder strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			if summaryBuilder.Len() > 0 {
				break
			}
			continue
		}
		summaryBuilder.WriteString(line)
		summaryBuilder.WriteString(" ")
		if summaryBuilder.Len() > 200 {
			break
		}
	}
	result.Summary = strings.TrimSpace(summaryBuilder.String())
	if len(result.Summary) > 300 {
		result.Summary = result.Summary[:300] + "..."
	}

	// Extract issues by looking for common patterns
	issuePatterns := []string{
		"issue", "problem", "bug", "error", "fix", "should", "must", "need to",
		"incorrect", "missing", "wrong", "critical", "major", "minor",
	}

	for _, line := range lines {
		lineLower := strings.ToLower(line)
		for _, pattern := range issuePatterns {
			if strings.Contains(lineLower, pattern) {
				// This line might describe an issue
				line = strings.TrimSpace(line)
				line = strings.TrimPrefix(line, "- ")
				line = strings.TrimPrefix(line, "* ")
				line = strings.TrimPrefix(line, "• ")

				if len(line) > 20 && len(line) < 500 && !strings.HasPrefix(line, "#") {
					severity := "minor"
					if strings.Contains(lineLower, "critical") {
						severity = "critical"
					} else if strings.Contains(lineLower, "major") || strings.Contains(lineLower, "must") {
						severity = "major"
					}

					result.Issues = append(result.Issues, Issue{
						Severity:    severity,
						Description: line,
					})
				}
				break
			}
		}
	}

	// Deduplicate and limit issues
	seen := make(map[string]bool)
	var uniqueIssues []Issue
	for _, issue := range result.Issues {
		key := strings.ToLower(issue.Description[:min(50, len(issue.Description))])
		if !seen[key] && len(uniqueIssues) < 10 {
			seen[key] = true
			uniqueIssues = append(uniqueIssues, issue)
		}
	}
	result.Issues = uniqueIssues

	// Count critical/major issues to determine pass/fail
	criticalCount := 0
	majorCount := 0
	for _, issue := range result.Issues {
		if issue.Severity == "critical" {
			criticalCount++
		} else if issue.Severity == "major" {
			majorCount++
		}
	}

	// Use configurable thresholds or defaults
	maxCritical := 1
	maxMajor := 3
	if s.cfg != nil {
		maxCritical = s.cfg.Review.MaxCriticalIssues
		maxMajor = s.cfg.Review.MaxMajorIssues
	}

	if criticalCount > maxCritical || majorCount > maxMajor {
		result.Passed = false
		result.Score = 40 + (10 - criticalCount*10 - majorCount*5)
		if result.Score < 20 {
			result.Score = 20
		}
	}

	// Extract guidance - look for "fix" or "recommendation" sections
	var guidanceBuilder strings.Builder
	inGuidance := false
	for _, line := range lines {
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, "recommend") ||
			strings.Contains(lineLower, "suggestion") ||
			strings.Contains(lineLower, "to fix") ||
			strings.Contains(lineLower, "next step") {
			inGuidance = true
		}
		if inGuidance {
			guidanceBuilder.WriteString(line)
			guidanceBuilder.WriteString("\n")
			if guidanceBuilder.Len() > 500 {
				break
			}
		}
	}
	result.Guidance = strings.TrimSpace(guidanceBuilder.String())

	// If no guidance extracted, use the issues as guidance
	if result.Guidance == "" && len(result.Issues) > 0 {
		var issueGuidance strings.Builder
		issueGuidance.WriteString("Please address the following issues:\n")
		for i, issue := range result.Issues {
			if i >= 5 {
				break
			}
			issueGuidance.WriteString(fmt.Sprintf("%d. %s\n", i+1, issue.Description))
		}
		result.Guidance = issueGuidance.String()
	}

	return result, nil
}

// extractJSON extracts JSON from a response that might be wrapped in markdown.
func extractJSON(text string) string {
	text = strings.TrimSpace(text)

	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		if idx := strings.LastIndex(text, "```"); idx != -1 {
			text = text[:idx]
		}
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		if idx := strings.LastIndex(text, "```"); idx != -1 {
			text = text[:idx]
		}
	}

	return strings.TrimSpace(text)
}

// FormatReview returns a human-readable format of the review.
func (r *ReviewResult) FormatReview() string {
	var sb strings.Builder

	sb.WriteString("   ┌─────────────────────────────────────────┐\n")
	if r.Passed {
		sb.WriteString("   │  ✅ REVIEW PASSED                       │\n")
	} else {
		sb.WriteString("   │  ❌ REVIEW FAILED                       │\n")
	}
	sb.WriteString("   └─────────────────────────────────────────┘\n")

	sb.WriteString(fmt.Sprintf("   📊 Score: %d/100\n\n", r.Score))
	sb.WriteString(fmt.Sprintf("   📝 Summary:\n      %s\n\n", r.Summary))

	if len(r.Praise) > 0 {
		sb.WriteString("   👍 What's good:\n")
		for _, p := range r.Praise {
			sb.WriteString(fmt.Sprintf("      • %s\n", p))
		}
		sb.WriteString("\n")
	}

	if len(r.Issues) > 0 {
		sb.WriteString("   🔍 Issues found:\n")
		for i, issue := range r.Issues {
			icon := "💡"
			switch issue.Severity {
			case "critical":
				icon = "🚨"
			case "major":
				icon = "⚠️"
			case "minor":
				icon = "📝"
			}

			sb.WriteString(fmt.Sprintf("      %d. %s [%s] %s\n", i+1, icon, strings.ToUpper(issue.Severity), issue.Description))
			if issue.File != "" {
				location := issue.File
				if issue.Line > 0 {
					location = fmt.Sprintf("%s:%d", issue.File, issue.Line)
				}
				sb.WriteString(fmt.Sprintf("         📍 %s\n", location))
			}
			if issue.Suggestion != "" {
				sb.WriteString(fmt.Sprintf("         💡 %s\n", issue.Suggestion))
			}
			sb.WriteString("\n")
		}
	}

	if r.Guidance != "" && !r.Passed {
		sb.WriteString("   📋 Guidance:\n")
		for _, line := range strings.Split(r.Guidance, "\n") {
			sb.WriteString(fmt.Sprintf("      %s\n", line))
		}
	}

	return sb.String()
}

// GetIssueDescriptions returns a list of issue descriptions for handoff.
func (r *ReviewResult) GetIssueDescriptions() []string {
	issues := make([]string, len(r.Issues))
	for i, issue := range r.Issues {
		issues[i] = fmt.Sprintf("[%s] %s", issue.Severity, issue.Description)
		if issue.Suggestion != "" {
			issues[i] += " → " + issue.Suggestion
		}
	}
	return issues
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
