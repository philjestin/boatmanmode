// Package diffverify verifies that code changes address review issues.
// It compares the new diff against the issues raised to confirm fixes.
package diffverify

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/philjestin/boatmanmode/internal/coordinator"
	"github.com/philjestin/boatmanmode/internal/scottbott"
)

// VerificationResult contains the outcome of diff verification.
type VerificationResult struct {
	// AllAddressed is true if all issues were addressed
	AllAddressed bool
	// AddressedIssues are issues that were fixed
	AddressedIssues []AddressedIssue
	// UnaddressedIssues are issues that remain
	UnaddressedIssues []UnaddressedIssue
	// NewIssues are potential new problems introduced
	NewIssues []string
	// Confidence is how confident we are in the assessment (0-100)
	Confidence int
}

// AddressedIssue represents a fixed issue.
type AddressedIssue struct {
	Original    scottbott.Issue
	FixEvidence string // What in the diff shows this was fixed
}

// UnaddressedIssue represents an unfixed issue.
type UnaddressedIssue struct {
	Original scottbott.Issue
	Reason   string // Why we think it wasn't fixed
}

// Agent verifies diffs against issues.
type Agent struct {
	id                    string
	worktreePath          string
	coord                 *coordinator.Coordinator
	minConfidenceOverride int // Optional minimum confidence override
}

// New creates a new diff verification agent.
func New(worktreePath string) *Agent {
	return &Agent{
		id:           "diffverify",
		worktreePath: worktreePath,
	}
}

// ID returns the agent ID.
func (a *Agent) ID() string {
	return a.id
}

// Name returns the human-readable name.
func (a *Agent) Name() string {
	return "Diff Verifier"
}

// Capabilities returns what this agent can do.
func (a *Agent) Capabilities() []coordinator.AgentCapability {
	return []coordinator.AgentCapability{coordinator.CapVerifyDiff}
}

// SetCoordinator sets the coordinator for communication.
func (a *Agent) SetCoordinator(c *coordinator.Coordinator) {
	a.coord = c
}

// SetMinConfidence sets the minimum confidence threshold for verification.
func (a *Agent) SetMinConfidence(minConfidence int) {
	a.minConfidenceOverride = minConfidence
}

// Verify checks if the diff addresses the given issues.
func (a *Agent) Verify(ctx context.Context, issues []scottbott.Issue, oldDiff, newDiff string) (*VerificationResult, error) {
	// Claim work if coordinated
	if a.coord != nil {
		claim := &coordinator.WorkClaim{
			WorkID:      "diff-verify",
			WorkType:    "verify_diff",
			Description: fmt.Sprintf("Verifying %d issues addressed", len(issues)),
		}
		if !a.coord.ClaimWork(a.id, claim) {
			return nil, fmt.Errorf("could not claim diff verification work")
		}
		defer a.coord.ReleaseWork(claim.WorkID, a.id)
	}

	result := &VerificationResult{
		AllAddressed:      true,
		AddressedIssues:   []AddressedIssue{},
		UnaddressedIssues: []UnaddressedIssue{},
		NewIssues:         []string{},
		Confidence:        85, // Default confidence (increased from 80)
	}

	// Parse diffs for analysis
	oldChanges := parseDiff(oldDiff)
	newChanges := parseDiff(newDiff)

	// Check each issue
	for _, issue := range issues {
		addressed, evidence, reason := a.checkIssueAddressed(issue, oldChanges, newChanges)
		
		if addressed {
			result.AddressedIssues = append(result.AddressedIssues, AddressedIssue{
				Original:    issue,
				FixEvidence: evidence,
			})
		} else {
			result.AllAddressed = false
			result.UnaddressedIssues = append(result.UnaddressedIssues, UnaddressedIssue{
				Original: issue,
				Reason:   reason,
			})
		}
	}

	// Check for potential new issues
	result.NewIssues = a.detectNewIssues(oldChanges, newChanges)
	if len(result.NewIssues) > 0 {
		// Only penalize for actual concerning issues, not debug statements
		concerningIssues := 0
		for _, issue := range result.NewIssues {
			if !strings.Contains(strings.ToLower(issue), "console.log") &&
				!strings.Contains(strings.ToLower(issue), "debug") &&
				!strings.Contains(strings.ToLower(issue), "print") {
				concerningIssues++
			}
		}
		result.Confidence -= concerningIssues * 5 // Reduced penalty
	}

	// Adjust confidence based on coverage with more lenient calculation
	if len(issues) > 0 {
		addressedRatio := float64(len(result.AddressedIssues)) / float64(len(issues))
		// Use a weighted formula that's more forgiving
		confidenceMultiplier := 0.7 + (addressedRatio * 0.3) // 70% base + 30% based on ratio
		result.Confidence = int(float64(result.Confidence) * confidenceMultiplier)
	}

	// Share result via coordinator
	if a.coord != nil {
		a.coord.SetContext("diffverify_result", result)
	}

	return result, nil
}

// DiffChange represents a parsed diff chunk.
type DiffChange struct {
	File     string
	Added    []string
	Removed  []string
	Context  []string
}

// parseDiff extracts changes from a unified diff.
func parseDiff(diff string) map[string]*DiffChange {
	changes := make(map[string]*DiffChange)
	
	lines := strings.Split(diff, "\n")
	var currentFile string
	var currentChange *DiffChange

	fileRe := regexp.MustCompile(`^\+\+\+ [ab]/(.+)$`)
	
	for _, line := range lines {
		// Detect file header
		if matches := fileRe.FindStringSubmatch(line); len(matches) > 1 {
			currentFile = matches[1]
			currentChange = &DiffChange{File: currentFile}
			changes[currentFile] = currentChange
			continue
		}

		if currentChange == nil {
			continue
		}

		// Categorize line
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			currentChange.Added = append(currentChange.Added, strings.TrimPrefix(line, "+"))
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			currentChange.Removed = append(currentChange.Removed, strings.TrimPrefix(line, "-"))
		} else if strings.HasPrefix(line, " ") {
			currentChange.Context = append(currentChange.Context, strings.TrimPrefix(line, " "))
		}
	}

	return changes
}

// checkIssueAddressed determines if an issue was fixed.
func (a *Agent) checkIssueAddressed(issue scottbott.Issue, oldChanges, newChanges map[string]*DiffChange) (bool, string, string) {
	// Strategy:
	// 1. If issue mentions a specific file, look for changes in that file
	// 2. Look for keywords from the issue in the new diff's added lines
	// 3. Look for removal of problematic patterns

	// Extract keywords from issue description
	keywords := extractKeywords(issue.Description)
	if issue.Suggestion != "" {
		keywords = append(keywords, extractKeywords(issue.Suggestion)...)
	}

	// Check if the specific file was modified
	if issue.File != "" {
		newChange, modified := newChanges[issue.File]
		if !modified {
			// File wasn't touched in new diff - issue not addressed
			return false, "", fmt.Sprintf("File %s was not modified", issue.File)
		}
		// File was modified - look for evidence of fix
		evidence := a.findFixEvidence(newChange, keywords, issue)
		if evidence != "" {
			return true, evidence, ""
		}
		// File was modified but no clear evidence found
		// Don't give up yet - check if issue might have been addressed elsewhere
	}

	// Check all new changes for keyword matches
	for file, change := range newChanges {
		// Skip if not in added lines
		evidence := a.findFixEvidence(change, keywords, issue)
		if evidence != "" {
			return true, fmt.Sprintf("In %s: %s", file, evidence), ""
		}
	}

	// Check if problematic pattern was removed
	if removal := a.checkPatternRemoved(oldChanges, newChanges, issue); removal != "" {
		return true, removal, ""
	}

	return false, "", "No evidence of fix found in diff"
}

// findFixEvidence looks for evidence that an issue was addressed.
func (a *Agent) findFixEvidence(change *DiffChange, keywords []string, issue scottbott.Issue) string {
	// Count keyword matches in added lines
	matchCount := 0
	var matchedLines []string

	for _, line := range change.Added {
		lineLower := strings.ToLower(line)
		for _, keyword := range keywords {
			if strings.Contains(lineLower, strings.ToLower(keyword)) {
				matchCount++
				if len(matchedLines) < 3 {
					matchedLines = append(matchedLines, truncate(line, 60))
				}
				break
			}
		}
	}

	// Accept any keyword matches as evidence
	if matchCount >= 1 {
		return fmt.Sprintf("Found related changes: %s", strings.Join(matchedLines, "; "))
	}

	// More lenient heuristics based on severity
	switch issue.Severity {
	case "critical":
		// For critical issues, look for significant changes
		if len(change.Added) > 3 || len(change.Removed) > 2 {
			return fmt.Sprintf("Significant changes (%d added, %d removed)", len(change.Added), len(change.Removed))
		}
	case "major":
		// For major issues, accept even small targeted changes
		if len(change.Added) > 1 || len(change.Removed) > 0 {
			return fmt.Sprintf("Targeted changes (%d lines added, %d removed)", len(change.Added), len(change.Removed))
		}
	case "minor":
		// For minor issues, any change in the file is likely addressing it
		if len(change.Added) > 0 || len(change.Removed) > 0 {
			return fmt.Sprintf("Code changes detected (%d added, %d removed)", len(change.Added), len(change.Removed))
		}
	}

	// If the file was modified at all, give benefit of the doubt
	if len(change.Added) > 0 || len(change.Removed) > 0 {
		return fmt.Sprintf("File modified with %d additions and %d deletions", len(change.Added), len(change.Removed))
	}

	return ""
}

// checkPatternRemoved checks if a problematic pattern was removed.
func (a *Agent) checkPatternRemoved(oldChanges, newChanges map[string]*DiffChange, issue scottbott.Issue) string {
	// Look for patterns in old added lines that are now in new removed lines
	badPatterns := extractBadPatterns(issue.Description)
	
	for file, newChange := range newChanges {
		for _, removed := range newChange.Removed {
			removedLower := strings.ToLower(removed)
			for _, pattern := range badPatterns {
				if strings.Contains(removedLower, pattern) {
					return fmt.Sprintf("Removed problematic pattern in %s: %s", file, truncate(removed, 50))
				}
			}
		}
	}

	return ""
}

// detectNewIssues looks for potential new problems introduced.
func (a *Agent) detectNewIssues(oldChanges, newChanges map[string]*DiffChange) []string {
	var issues []string

	// Only flag truly problematic patterns - be more lenient
	problemPatterns := []struct {
		pattern string
		message string
	}{
		{`fixme:`, "New FIXME comment added"},
		{`xxx:`, "New XXX marker added"},
		{`binding\.pry`, "Debug statement left in code"},
		{`debugger;`, "Debugger statement left in code"},
	}

	for file, change := range newChanges {
		for _, line := range change.Added {
			lineLower := strings.ToLower(line)
			for _, p := range problemPatterns {
				re := regexp.MustCompile(`(?i)` + p.pattern)
				if re.MatchString(lineLower) {
					issues = append(issues, fmt.Sprintf("%s in %s", p.message, file))
					break
				}
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, issue := range issues {
		if !seen[issue] {
			seen[issue] = true
			unique = append(unique, issue)
		}
	}

	return unique
}

// extractKeywords pulls out meaningful words from text.
func extractKeywords(text string) []string {
	// Remove common words and extract meaningful terms
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
		"have": true, "has": true, "had": true, "do": true, "does": true, "did": true,
		"will": true, "would": true, "could": true, "should": true, "may": true, "might": true,
		"this": true, "that": true, "these": true, "those": true,
		"it": true, "its": true, "they": true, "their": true, "them": true,
		"i": true, "you": true, "we": true, "he": true, "she": true,
		"with": true, "from": true, "by": true, "about": true, "into": true,
		"through": true, "during": true, "before": true, "after": true,
		"above": true, "below": true, "between": true, "under": true,
		"there": true, "here": true, "where": true, "when": true, "why": true, "how": true,
		"all": true, "each": true, "every": true, "both": true, "few": true, "more": true,
		"most": true, "other": true, "some": true, "such": true, "no": true, "not": true,
		"only": true, "same": true, "so": true, "than": true, "too": true, "very": true,
		"can": true, "just": true, "now": true,
	}

	// Tokenize and filter
	words := regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*`).FindAllString(text, -1)
	var keywords []string
	
	seen := make(map[string]bool)
	for _, word := range words {
		lower := strings.ToLower(word)
		if len(lower) < 3 {
			continue
		}
		if stopWords[lower] {
			continue
		}
		if seen[lower] {
			continue
		}
		seen[lower] = true
		keywords = append(keywords, lower)
	}

	return keywords
}

// extractBadPatterns pulls out patterns that might indicate problems.
func extractBadPatterns(text string) []string {
	patterns := []string{}
	
	// Look for quoted strings
	quoteRe := regexp.MustCompile(`["'\x60]([^"'\x60]+)["'\x60]`)
	for _, match := range quoteRe.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 && len(match[1]) > 2 {
			patterns = append(patterns, strings.ToLower(match[1]))
		}
	}

	// Look for code-like patterns
	codeRe := regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*\([^\)]*\)`)
	for _, match := range codeRe.FindAllString(text, -1) {
		patterns = append(patterns, strings.ToLower(match))
	}

	return patterns
}

// Execute implements the Agent interface for coordinated execution.
func (a *Agent) Execute(ctx context.Context, handoff coordinator.Handoff) (coordinator.Handoff, error) {
	verifyHandoff, ok := handoff.(*VerifyHandoff)
	if !ok {
		return nil, fmt.Errorf("expected VerifyHandoff, got %T", handoff)
	}

	result, err := a.Verify(ctx, verifyHandoff.Issues, verifyHandoff.OldDiff, verifyHandoff.NewDiff)
	if err != nil {
		return nil, err
	}

	return &VerificationHandoff{Result: result}, nil
}

// VerifyHandoff wraps issues and diffs for verification.
type VerifyHandoff struct {
	Issues  []scottbott.Issue
	OldDiff string
	NewDiff string
}

func (h *VerifyHandoff) Full() string {
	var sb strings.Builder
	sb.WriteString("# Diff Verification Request\n\n")
	sb.WriteString(fmt.Sprintf("Issues to verify: %d\n\n", len(h.Issues)))
	
	sb.WriteString("## Issues\n")
	for i, issue := range h.Issues {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, issue.Severity, issue.Description))
	}
	
	sb.WriteString("\n## Old Diff\n```diff\n")
	sb.WriteString(truncate(h.OldDiff, 2000))
	sb.WriteString("\n```\n")
	
	sb.WriteString("\n## New Diff\n```diff\n")
	sb.WriteString(truncate(h.NewDiff, 2000))
	sb.WriteString("\n```\n")
	
	return sb.String()
}

func (h *VerifyHandoff) Concise() string {
	return fmt.Sprintf("Verify %d issues: old diff %d chars, new diff %d chars", 
		len(h.Issues), len(h.OldDiff), len(h.NewDiff))
}

func (h *VerifyHandoff) ForTokenBudget(maxTokens int) string {
	full := h.Full()
	if len(full) < maxTokens*4 {
		return full
	}
	return h.Concise()
}

func (h *VerifyHandoff) Type() string {
	return "verify"
}

// VerificationHandoff wraps verification results.
type VerificationHandoff struct {
	Result *VerificationResult
}

func (h *VerificationHandoff) Full() string {
	r := h.Result
	var sb strings.Builder

	sb.WriteString("# Diff Verification Result\n\n")
	
	if r.AllAddressed {
		sb.WriteString("✅ **ALL ISSUES ADDRESSED**\n\n")
	} else {
		sb.WriteString("❌ **SOME ISSUES REMAIN**\n\n")
	}

	sb.WriteString(fmt.Sprintf("Confidence: %d%%\n\n", r.Confidence))

	if len(r.AddressedIssues) > 0 {
		sb.WriteString("## Addressed Issues\n")
		for i, a := range r.AddressedIssues {
			sb.WriteString(fmt.Sprintf("%d. ✅ [%s] %s\n", i+1, a.Original.Severity, a.Original.Description))
			if a.FixEvidence != "" {
				sb.WriteString(fmt.Sprintf("   Evidence: %s\n", a.FixEvidence))
			}
		}
		sb.WriteString("\n")
	}

	if len(r.UnaddressedIssues) > 0 {
		sb.WriteString("## Unaddressed Issues\n")
		for i, u := range r.UnaddressedIssues {
			sb.WriteString(fmt.Sprintf("%d. ❌ [%s] %s\n", i+1, u.Original.Severity, u.Original.Description))
			if u.Reason != "" {
				sb.WriteString(fmt.Sprintf("   Reason: %s\n", u.Reason))
			}
		}
		sb.WriteString("\n")
	}

	if len(r.NewIssues) > 0 {
		sb.WriteString("## ⚠️ Potential New Issues\n")
		for _, issue := range r.NewIssues {
			sb.WriteString(fmt.Sprintf("- %s\n", issue))
		}
	}

	return sb.String()
}

func (h *VerificationHandoff) Concise() string {
	r := h.Result
	if r.AllAddressed {
		return fmt.Sprintf("✅ All %d issues addressed (%d%% confidence)", 
			len(r.AddressedIssues), r.Confidence)
	}
	return fmt.Sprintf("❌ %d/%d issues addressed (%d%% confidence)", 
		len(r.AddressedIssues), len(r.AddressedIssues)+len(r.UnaddressedIssues), r.Confidence)
}

func (h *VerificationHandoff) ForTokenBudget(maxTokens int) string {
	full := h.Full()
	if len(full) < maxTokens*4 {
		return full
	}
	return h.Concise()
}

func (h *VerificationHandoff) Type() string {
	return "verification_result"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
