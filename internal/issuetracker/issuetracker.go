// Package issuetracker provides issue deduplication across review iterations.
// It tracks which issues have been reported, addressed, or persist across iterations.
package issuetracker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/philjestin/boatmanmode/internal/scottbott"
)

// TrackedIssue extends an issue with tracking metadata.
type TrackedIssue struct {
	scottbott.Issue
	ID              string    // Unique hash-based ID
	FirstSeen       int       // Iteration when first seen
	LastSeen        int       // Most recent iteration
	TimesReported   int       // How many times reported
	Addressed       bool      // Was it addressed?
	AddressedAt     int       // Iteration when addressed
	SimilarIssues   []string  // IDs of similar issues
}

// IssueTracker tracks issues across iterations.
type IssueTracker struct {
	issues        map[string]*TrackedIssue
	iteration     int
	// Similarity threshold (0-1, higher = stricter matching)
	similarityThreshold float64
}

// New creates a new IssueTracker.
func New() *IssueTracker {
	return &IssueTracker{
		issues:              make(map[string]*TrackedIssue),
		iteration:           0,
		similarityThreshold: 0.7,
	}
}

// NextIteration advances to the next iteration.
func (t *IssueTracker) NextIteration() {
	t.iteration++
}

// CurrentIteration returns the current iteration number.
func (t *IssueTracker) CurrentIteration() int {
	return t.iteration
}

// Track adds issues from a review, deduplicating as needed.
// Returns the deduplicated list of new/persistent issues.
func (t *IssueTracker) Track(issues []scottbott.Issue) []TrackedIssue {
	var result []TrackedIssue
	seenThisIteration := make(map[string]bool)

	for _, issue := range issues {
		// Generate ID for this issue
		id := t.generateIssueID(issue)

		// Check for exact match
		if existing, ok := t.issues[id]; ok {
			existing.LastSeen = t.iteration
			existing.TimesReported++
			existing.Addressed = false // Re-appeared, so not addressed
			seenThisIteration[id] = true
			result = append(result, *existing)
			continue
		}

		// Check for similar issues
		similarID := t.findSimilarIssue(issue)
		if similarID != "" {
			existing := t.issues[similarID]
			existing.LastSeen = t.iteration
			existing.TimesReported++
			existing.Addressed = false
			existing.SimilarIssues = appendUnique(existing.SimilarIssues, id)
			seenThisIteration[similarID] = true
			result = append(result, *existing)
			continue
		}

		// New issue
		tracked := &TrackedIssue{
			Issue:         issue,
			ID:            id,
			FirstSeen:     t.iteration,
			LastSeen:      t.iteration,
			TimesReported: 1,
			Addressed:     false,
		}
		t.issues[id] = tracked
		seenThisIteration[id] = true
		result = append(result, *tracked)
	}

	// Mark issues not seen this iteration as potentially addressed
	for id, issue := range t.issues {
		if !seenThisIteration[id] && issue.LastSeen == t.iteration-1 {
			issue.Addressed = true
			issue.AddressedAt = t.iteration
		}
	}

	return result
}

// GetNewIssues returns only issues that are new this iteration.
func (t *IssueTracker) GetNewIssues() []TrackedIssue {
	var result []TrackedIssue
	for _, issue := range t.issues {
		if issue.FirstSeen == t.iteration {
			result = append(result, *issue)
		}
	}
	return result
}

// GetPersistentIssues returns issues that have appeared in multiple iterations.
func (t *IssueTracker) GetPersistentIssues() []TrackedIssue {
	var result []TrackedIssue
	for _, issue := range t.issues {
		if issue.TimesReported > 1 && !issue.Addressed {
			result = append(result, *issue)
		}
	}
	return result
}

// GetAddressedIssues returns issues that were addressed.
func (t *IssueTracker) GetAddressedIssues() []TrackedIssue {
	var result []TrackedIssue
	for _, issue := range t.issues {
		if issue.Addressed {
			result = append(result, *issue)
		}
	}
	return result
}

// GetUnaddressedIssues returns issues that haven't been addressed.
func (t *IssueTracker) GetUnaddressedIssues() []TrackedIssue {
	var result []TrackedIssue
	for _, issue := range t.issues {
		if !issue.Addressed {
			result = append(result, *issue)
		}
	}
	return result
}

// GetCriticalIssues returns unaddressed critical issues.
func (t *IssueTracker) GetCriticalIssues() []TrackedIssue {
	var result []TrackedIssue
	for _, issue := range t.issues {
		if !issue.Addressed && issue.Severity == "critical" {
			result = append(result, *issue)
		}
	}
	return result
}

// Stats returns statistics about tracked issues.
func (t *IssueTracker) Stats() IssueStats {
	stats := IssueStats{
		TotalIssues:      len(t.issues),
		CurrentIteration: t.iteration,
	}

	for _, issue := range t.issues {
		if issue.Addressed {
			stats.AddressedCount++
		} else {
			switch issue.Severity {
			case "critical":
				stats.CriticalCount++
			case "major":
				stats.MajorCount++
			case "minor":
				stats.MinorCount++
			}
		}

		if issue.TimesReported > 1 {
			stats.PersistentCount++
		}
	}

	return stats
}

// IssueStats contains summary statistics.
type IssueStats struct {
	TotalIssues      int
	AddressedCount   int
	CriticalCount    int
	MajorCount       int
	MinorCount       int
	PersistentCount  int
	CurrentIteration int
}

// FormatStats returns a formatted string of statistics.
func (s IssueStats) FormatStats() string {
	return fmt.Sprintf(
		"Iteration %d: %d total issues (%d addressed, %d critical, %d major, %d minor, %d persistent)",
		s.CurrentIteration, s.TotalIssues, s.AddressedCount, s.CriticalCount, s.MajorCount, s.MinorCount, s.PersistentCount,
	)
}

// generateIssueID creates a unique ID for an issue.
func (t *IssueTracker) generateIssueID(issue scottbott.Issue) string {
	// Normalize the description for consistent hashing
	normalized := normalizeText(issue.Description)

	// Include file and line if present
	content := normalized
	if issue.File != "" {
		content += "|" + issue.File
	}
	if issue.Line > 0 {
		content += fmt.Sprintf("|%d", issue.Line)
	}

	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:8]) // First 8 bytes = 16 hex chars
}

// findSimilarIssue finds an existing issue that is similar.
func (t *IssueTracker) findSimilarIssue(issue scottbott.Issue) string {
	normalizedNew := normalizeText(issue.Description)
	newWords := extractWords(normalizedNew)

	for id, existing := range t.issues {
		if existing.Addressed {
			continue // Skip addressed issues
		}

		// Same severity is a good start
		if existing.Severity != issue.Severity {
			continue
		}

		// Same file is a strong signal
		if issue.File != "" && existing.File == issue.File {
			// Check text similarity
			existingWords := extractWords(normalizeText(existing.Description))
			similarity := calculateSimilarity(newWords, existingWords)
			if similarity >= t.similarityThreshold*0.8 { // Lower threshold for same file
				return id
			}
		}

		// Check text similarity
		existingWords := extractWords(normalizeText(existing.Description))
		similarity := calculateSimilarity(newWords, existingWords)
		if similarity >= t.similarityThreshold {
			return id
		}
	}

	return ""
}

// normalizeText normalizes text for comparison.
func normalizeText(s string) string {
	s = strings.ToLower(s)
	// Remove punctuation
	s = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(s, " ")
	// Normalize whitespace
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// extractWords extracts unique words from text.
func extractWords(s string) map[string]bool {
	words := make(map[string]bool)
	for _, word := range strings.Fields(s) {
		if len(word) > 2 { // Skip very short words
			words[word] = true
		}
	}
	return words
}

// calculateSimilarity calculates Jaccard similarity between word sets.
func calculateSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	intersection := 0
	for word := range a {
		if b[word] {
			intersection++
		}
	}

	union := len(a) + len(b) - intersection
	return float64(intersection) / float64(union)
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// FormatIssues formats tracked issues for display.
func FormatIssues(issues []TrackedIssue) string {
	var sb strings.Builder

	for i, issue := range issues {
		icon := "📝"
		switch issue.Severity {
		case "critical":
			icon = "🚨"
		case "major":
			icon = "⚠️"
		}

		sb.WriteString(fmt.Sprintf("%d. %s [%s] %s\n", i+1, icon, strings.ToUpper(issue.Severity), issue.Description))

		if issue.File != "" {
			sb.WriteString(fmt.Sprintf("   📍 %s", issue.File))
			if issue.Line > 0 {
				sb.WriteString(fmt.Sprintf(":%d", issue.Line))
			}
			sb.WriteString("\n")
		}

		if issue.TimesReported > 1 {
			sb.WriteString(fmt.Sprintf("   🔄 Reported %d times (first: iteration %d)\n", issue.TimesReported, issue.FirstSeen))
		}

		if issue.Suggestion != "" {
			sb.WriteString(fmt.Sprintf("   💡 %s\n", issue.Suggestion))
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// IssueHistory tracks the full history for reporting.
type IssueHistory struct {
	tracker    *IssueTracker
	iterations []IterationRecord
}

// IterationRecord records issues per iteration.
type IterationRecord struct {
	Iteration   int
	Timestamp   time.Time
	Issues      []TrackedIssue
	NewCount    int
	ResolvedIDs []string
}

// NewIssueHistory creates a new history tracker.
func NewIssueHistory() *IssueHistory {
	return &IssueHistory{
		tracker:    New(),
		iterations: []IterationRecord{},
	}
}

// RecordIteration records issues for an iteration.
func (h *IssueHistory) RecordIteration(issues []scottbott.Issue) []TrackedIssue {
	h.tracker.NextIteration()

	// Find what was resolved this iteration
	var resolvedIDs []string
	for id, issue := range h.tracker.issues {
		if !issue.Addressed {
			// Mark for potential resolution
			resolvedIDs = append(resolvedIDs, id)
		}
	}

	// Track new issues
	tracked := h.tracker.Track(issues)

	// Find actually resolved (not in tracked)
	trackedIDs := make(map[string]bool)
	for _, t := range tracked {
		trackedIDs[t.ID] = true
	}

	var actuallyResolved []string
	for _, id := range resolvedIDs {
		if !trackedIDs[id] {
			actuallyResolved = append(actuallyResolved, id)
		}
	}

	// Count new
	newCount := 0
	for _, t := range tracked {
		if t.FirstSeen == h.tracker.iteration {
			newCount++
		}
	}

	// Record
	h.iterations = append(h.iterations, IterationRecord{
		Iteration:   h.tracker.iteration,
		Timestamp:   time.Now(),
		Issues:      tracked,
		NewCount:    newCount,
		ResolvedIDs: actuallyResolved,
	})

	return tracked
}

// GetTracker returns the underlying tracker.
func (h *IssueHistory) GetTracker() *IssueTracker {
	return h.tracker
}

// FormatHistory formats the full history.
func (h *IssueHistory) FormatHistory() string {
	var sb strings.Builder

	sb.WriteString("# Issue History\n\n")

	for _, record := range h.iterations {
		sb.WriteString(fmt.Sprintf("## Iteration %d (%s)\n", record.Iteration, record.Timestamp.Format("15:04:05")))
		sb.WriteString(fmt.Sprintf("- New issues: %d\n", record.NewCount))
		sb.WriteString(fmt.Sprintf("- Resolved: %d\n", len(record.ResolvedIDs)))
		sb.WriteString(fmt.Sprintf("- Total active: %d\n\n", len(record.Issues)-len(record.ResolvedIDs)))
	}

	sb.WriteString("## Current Status\n")
	sb.WriteString(h.tracker.Stats().FormatStats())
	sb.WriteString("\n")

	return sb.String()
}
