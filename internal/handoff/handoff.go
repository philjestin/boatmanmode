// Package handoff provides structured context passing between agents.
// Each handoff contains only the essential information needed for the next agent.
package handoff

import (
	"fmt"
	"strings"

	"github.com/handshake/boatmanmode/internal/linear"
)

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

// ToPrompt formats the handoff as a prompt for the executor.
func (h *ExecutionHandoff) ToPrompt() string {
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

// ToPrompt formats the handoff for the reviewer.
func (h *ReviewHandoff) ToPrompt() string {
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

// ToPrompt formats the handoff for the refactor agent.
func (h *RefactorHandoff) ToPrompt() string {
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


