// Package handoff provides structured context passing between agents.
// Each handoff contains only the essential information needed for the next agent.
// All handoffs implement the Handoff interface for adaptive sizing.
package handoff

import (
	"fmt"
	"strings"

	"github.com/handshake/boatmanmode/internal/linear"
)

// Handoff is the interface for passing context between agents.
// It supports multiple sizing strategies for token efficiency.
type Handoff interface {
	// Full returns the complete context
	Full() string
	// Concise returns a summary suitable for quick handoffs
	Concise() string
	// ForTokenBudget returns context sized to fit within token budget
	ForTokenBudget(maxTokens int) string
	// Type returns the handoff type for routing
	Type() string
}

// TokenBudget represents token limits for different contexts.
type TokenBudget struct {
	System  int // Max tokens for system prompt
	User    int // Max tokens for user prompt
	Context int // Max tokens for additional context
	Total   int // Total token budget
}

// DefaultBudget provides reasonable defaults for most models.
var DefaultBudget = TokenBudget{
	System:  8000,
	User:    50000,
	Context: 30000,
	Total:   100000,
}

// EstimateTokens provides a rough token count for a string.
// Assumes ~4 chars per token for English text.
func EstimateTokens(s string) int {
	return len(s) / 4
}

// TruncateToTokens truncates a string to fit within a token budget.
func TruncateToTokens(s string, maxTokens int) string {
	maxChars := maxTokens * 4
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "\n... (truncated)"
}

// ExecutionHandoff contains context for the initial code execution.
type ExecutionHandoff struct {
	TicketID    string
	Title       string
	Description string
	Labels      []string
	BranchName  string
}

// NewExecutionHandoff creates a handoff from a Linear ticket.
func NewExecutionHandoff(ticket *linear.Ticket) *ExecutionHandoff {
	return &ExecutionHandoff{
		TicketID:    ticket.Identifier,
		Title:       ticket.Title,
		Description: ticket.Description,
		Labels:      ticket.Labels,
		BranchName:  ticket.BranchName,
	}
}

// Full returns the complete execution context.
func (h *ExecutionHandoff) Full() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", h.Title))
	sb.WriteString(fmt.Sprintf("**Ticket:** %s\n", h.TicketID))
	if len(h.Labels) > 0 {
		sb.WriteString(fmt.Sprintf("**Labels:** %s\n", strings.Join(h.Labels, ", ")))
	}
	sb.WriteString("\n## Requirements\n\n")
	sb.WriteString(h.Description)
	return sb.String()
}

// Concise returns a summary of the execution context.
func (h *ExecutionHandoff) Concise() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s (%s)\n\n", h.Title, h.TicketID))
	sb.WriteString(extractRequirements(h.Description))
	return sb.String()
}

// ForTokenBudget returns context sized to fit within token budget.
func (h *ExecutionHandoff) ForTokenBudget(maxTokens int) string {
	full := h.Full()
	if EstimateTokens(full) <= maxTokens {
		return full
	}
	
	concise := h.Concise()
	if EstimateTokens(concise) <= maxTokens {
		return concise
	}
	
	return TruncateToTokens(concise, maxTokens)
}

// Type returns the handoff type.
func (h *ExecutionHandoff) Type() string {
	return "execution"
}

// ToPrompt formats the handoff as a prompt for the executor.
// Kept for backward compatibility.
func (h *ExecutionHandoff) ToPrompt() string {
	return h.Full()
}

// ReviewHandoff contains context for ScottBott peer review.
type ReviewHandoff struct {
	TicketID     string
	Title        string
	Requirements string // Concise summary of what was requested
	Diff         string // The actual code changes
	FilesChanged []string
}

// NewReviewHandoff creates a handoff for code review.
func NewReviewHandoff(ticket *linear.Ticket, diff string, filesChanged []string) *ReviewHandoff {
	return &ReviewHandoff{
		TicketID:     ticket.Identifier,
		Title:        ticket.Title,
		Requirements: extractRequirements(ticket.Description),
		Diff:         diff,
		FilesChanged: filesChanged,
	}
}

// Full returns the complete review context.
func (h *ReviewHandoff) Full() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Review: %s (%s)\n\n", h.Title, h.TicketID))
	sb.WriteString("## Requirements Summary\n\n")
	sb.WriteString(h.Requirements)
	sb.WriteString("\n\n## Files Changed\n\n")
	for _, f := range h.FilesChanged {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	sb.WriteString("\n## Diff\n\n```diff\n")
	sb.WriteString(h.Diff)
	sb.WriteString("\n```\n")
	return sb.String()
}

// Concise returns a summary of the review context.
func (h *ReviewHandoff) Concise() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Review: %s (%s)\n\n", h.Title, h.TicketID))
	sb.WriteString("## Requirements\n\n")
	sb.WriteString(h.Requirements)
	sb.WriteString(fmt.Sprintf("\n\n## Changes: %d files, %d lines\n", 
		len(h.FilesChanged), strings.Count(h.Diff, "\n")))
	for _, f := range h.FilesChanged {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	return sb.String()
}

// ForTokenBudget returns context sized to fit within token budget.
func (h *ReviewHandoff) ForTokenBudget(maxTokens int) string {
	full := h.Full()
	if EstimateTokens(full) <= maxTokens {
		return full
	}
	
	// Try with truncated diff
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Review: %s (%s)\n\n", h.Title, h.TicketID))
	sb.WriteString("## Requirements Summary\n\n")
	sb.WriteString(h.Requirements)
	sb.WriteString("\n\n## Files Changed\n\n")
	for _, f := range h.FilesChanged {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	
	// Calculate remaining budget for diff
	headerTokens := EstimateTokens(sb.String())
	diffBudget := maxTokens - headerTokens - 100 // Reserve 100 for formatting
	
	sb.WriteString("\n## Diff (truncated)\n\n```diff\n")
	sb.WriteString(TruncateToTokens(h.Diff, diffBudget))
	sb.WriteString("\n```\n")
	
	return sb.String()
}

// Type returns the handoff type.
func (h *ReviewHandoff) Type() string {
	return "review"
}

// ToPrompt formats the handoff for the reviewer.
// Kept for backward compatibility.
func (h *ReviewHandoff) ToPrompt() string {
	return h.Full()
}

// RefactorHandoff contains context for a refactor iteration.
type RefactorHandoff struct {
	TicketID      string
	Title         string
	Requirements  string   // Original requirements
	Issues        []string // Specific issues to fix
	Guidance      string   // Review guidance
	FilesToUpdate []string // Files that need changes
	CurrentCode   string   // Current implementation
}

// NewRefactorHandoff creates a handoff for refactoring.
func NewRefactorHandoff(ticket *linear.Ticket, issues []string, guidance string, filesToUpdate []string, currentCode string) *RefactorHandoff {
	return &RefactorHandoff{
		TicketID:      ticket.Identifier,
		Title:         ticket.Title,
		Requirements:  extractRequirements(ticket.Description),
		Issues:        issues,
		Guidance:      guidance,
		FilesToUpdate: filesToUpdate,
		CurrentCode:   currentCode,
	}
}

// Full returns the complete refactor context.
func (h *RefactorHandoff) Full() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Refactor: %s (%s)\n\n", h.Title, h.TicketID))
	
	sb.WriteString("## Original Requirements\n\n")
	sb.WriteString(h.Requirements)
	
	sb.WriteString("\n\n## Issues to Fix\n\n")
	for i, issue := range h.Issues {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, issue))
	}
	
	sb.WriteString("\n## Guidance\n\n")
	sb.WriteString(h.Guidance)
	
	sb.WriteString("\n\n## Files to Update\n\n")
	for _, f := range h.FilesToUpdate {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	
	sb.WriteString("\n## Current Implementation\n\n")
	sb.WriteString(h.CurrentCode)
	
	sb.WriteString("\n\n## Instructions\n\n")
	sb.WriteString("Fix ALL listed issues. Output complete updated files using this format:\n\n")
	sb.WriteString("### FILE: path/to/file.ext\n```\n// complete file contents\n```\n")
	
	return sb.String()
}

// Concise returns a summary of the refactor context.
func (h *RefactorHandoff) Concise() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Refactor: %s (%s)\n\n", h.Title, h.TicketID))
	
	sb.WriteString("## Issues to Fix\n\n")
	for i, issue := range h.Issues {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, issue))
	}
	
	sb.WriteString(fmt.Sprintf("\n## Files: %d\n", len(h.FilesToUpdate)))
	for _, f := range h.FilesToUpdate {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	
	return sb.String()
}

// ForTokenBudget returns context sized to fit within token budget.
func (h *RefactorHandoff) ForTokenBudget(maxTokens int) string {
	full := h.Full()
	if EstimateTokens(full) <= maxTokens {
		return full
	}
	
	// Build incrementally
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Refactor: %s (%s)\n\n", h.Title, h.TicketID))
	
	// Issues are most important
	sb.WriteString("## Issues to Fix (MUST ADDRESS ALL)\n\n")
	for i, issue := range h.Issues {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, issue))
	}
	
	// Guidance is helpful
	if h.Guidance != "" {
		sb.WriteString("\n## Guidance\n\n")
		sb.WriteString(TruncateToTokens(h.Guidance, 500))
	}
	
	sb.WriteString("\n\n## Files to Update\n\n")
	for _, f := range h.FilesToUpdate {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	
	// Calculate remaining budget for code
	headerTokens := EstimateTokens(sb.String())
	codeBudget := maxTokens - headerTokens - 200
	
	if codeBudget > 500 {
		sb.WriteString("\n## Current Implementation (truncated)\n\n")
		sb.WriteString(TruncateToTokens(h.CurrentCode, codeBudget))
	}
	
	sb.WriteString("\n\n## Instructions\n\n")
	sb.WriteString("Fix ALL listed issues. Output complete updated files using format:\n")
	sb.WriteString("### FILE: path/to/file.ext\n```\n// contents\n```\n")
	
	return sb.String()
}

// Type returns the handoff type.
func (h *RefactorHandoff) Type() string {
	return "refactor"
}

// ToPrompt formats the handoff for the refactor agent.
// Kept for backward compatibility.
func (h *RefactorHandoff) ToPrompt() string {
	return h.Full()
}

// CompoundHandoff combines multiple handoffs into one.
type CompoundHandoff struct {
	Handoffs []Handoff
}

// NewCompoundHandoff creates a compound handoff from multiple sources.
func NewCompoundHandoff(handoffs ...Handoff) *CompoundHandoff {
	return &CompoundHandoff{Handoffs: handoffs}
}

// Full returns all handoffs combined.
func (h *CompoundHandoff) Full() string {
	var parts []string
	for _, ho := range h.Handoffs {
		parts = append(parts, ho.Full())
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// Concise returns concise versions of all handoffs.
func (h *CompoundHandoff) Concise() string {
	var parts []string
	for _, ho := range h.Handoffs {
		parts = append(parts, ho.Concise())
	}
	return strings.Join(parts, "\n\n")
}

// ForTokenBudget distributes budget across handoffs.
func (h *CompoundHandoff) ForTokenBudget(maxTokens int) string {
	if len(h.Handoffs) == 0 {
		return ""
	}
	
	// First pass: try concise for all
	conciseTotal := EstimateTokens(h.Concise())
	if conciseTotal <= maxTokens {
		// We have room for more detail
		budgetPerHandoff := maxTokens / len(h.Handoffs)
		var parts []string
		for _, ho := range h.Handoffs {
			parts = append(parts, ho.ForTokenBudget(budgetPerHandoff))
		}
		return strings.Join(parts, "\n\n---\n\n")
	}
	
	// Tight on tokens, use pure concise
	return h.Concise()
}

// Type returns the handoff type.
func (h *CompoundHandoff) Type() string {
	return "compound"
}

// PipelineHandoff tracks context through a pipeline of agents.
type PipelineHandoff struct {
	// Original is the original handoff that started the pipeline
	Original Handoff
	// History is the sequence of handoffs from each agent
	History []Handoff
	// Current is the current handoff
	Current Handoff
}

// NewPipelineHandoff creates a new pipeline handoff.
func NewPipelineHandoff(original Handoff) *PipelineHandoff {
	return &PipelineHandoff{
		Original: original,
		History:  []Handoff{},
		Current:  original,
	}
}

// Advance moves to the next stage with a new handoff.
func (h *PipelineHandoff) Advance(next Handoff) {
	h.History = append(h.History, h.Current)
	h.Current = next
}

// Full returns the current handoff's full content.
func (h *PipelineHandoff) Full() string {
	return h.Current.Full()
}

// Concise returns the current handoff's concise content.
func (h *PipelineHandoff) Concise() string {
	return h.Current.Concise()
}

// ForTokenBudget returns current handoff sized for budget.
func (h *PipelineHandoff) ForTokenBudget(maxTokens int) string {
	return h.Current.ForTokenBudget(maxTokens)
}

// Type returns the current handoff type.
func (h *PipelineHandoff) Type() string {
	return h.Current.Type()
}

// WithHistory returns context with history for debugging.
func (h *PipelineHandoff) WithHistory(maxHistoryItems int) string {
	var sb strings.Builder
	
	sb.WriteString("# Pipeline Context\n\n")
	sb.WriteString("## Original\n")
	sb.WriteString(h.Original.Concise())
	sb.WriteString("\n\n")
	
	// Include recent history
	start := 0
	if len(h.History) > maxHistoryItems {
		start = len(h.History) - maxHistoryItems
	}
	
	if len(h.History) > 0 {
		sb.WriteString("## History (recent)\n")
		for i := start; i < len(h.History); i++ {
			sb.WriteString(fmt.Sprintf("\n### Step %d: %s\n", i+1, h.History[i].Type()))
			sb.WriteString(h.History[i].Concise())
			sb.WriteString("\n")
		}
	}
	
	sb.WriteString("\n## Current\n")
	sb.WriteString(h.Current.Full())
	
	return sb.String()
}

// extractRequirements pulls out the key requirements from a description.
func extractRequirements(description string) string {
	// If description is short, use it as-is
	if len(description) < 500 {
		return description
	}
	
	// Try to extract just the goal/requirements sections
	lines := strings.Split(description, "\n")
	var result strings.Builder
	inRelevantSection := false
	
	for _, line := range lines {
		lower := strings.ToLower(line)
		
		// Start capturing at these sections
		if strings.Contains(lower, "goal") || 
		   strings.Contains(lower, "requirement") ||
		   strings.Contains(lower, "must") ||
		   strings.Contains(lower, "should") {
			inRelevantSection = true
		}
		
		// Stop at implementation details
		if strings.Contains(lower, "implementation approach") ||
		   strings.Contains(lower, "technical context") ||
		   strings.Contains(lower, "constraints") {
			inRelevantSection = false
		}
		
		if inRelevantSection || result.Len() < 300 {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}
	
	summary := strings.TrimSpace(result.String())
	if len(summary) < 100 {
		// Fallback: just truncate
		if len(description) > 800 {
			return description[:800] + "..."
		}
		return description
	}
	
	return summary
}
