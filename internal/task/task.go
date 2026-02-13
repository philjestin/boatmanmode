// Package task provides an abstraction over different work input sources.
// This allows boatmanmode to work with Linear tickets, inline prompts, or file-based prompts.
package task

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/philjestin/boatmanmode/internal/linear"
)

// TaskSource represents the origin of a task.
type TaskSource string

const (
	SourceLinear TaskSource = "linear"
	SourcePrompt TaskSource = "prompt"
	SourceFile   TaskSource = "file"
)

// TaskMetadata holds additional task information.
type TaskMetadata struct {
	Source    TaskSource
	CreatedAt time.Time
	FilePath  string // For file-based tasks
}

// Task represents work to be done, regardless of source.
type Task interface {
	GetID() string
	GetTitle() string
	GetDescription() string
	GetBranchName() string
	GetLabels() []string
	GetMetadata() TaskMetadata
}

// LinearTask wraps a Linear ticket to implement the Task interface.
type LinearTask struct {
	ticket *linear.Ticket
}

// NewLinearTask creates a Task from a Linear ticket.
func NewLinearTask(ticket *linear.Ticket) Task {
	return &LinearTask{ticket: ticket}
}

// GetID returns the Linear ticket identifier (e.g., "ENG-123").
func (t *LinearTask) GetID() string {
	return t.ticket.Identifier
}

// GetTitle returns the ticket title.
func (t *LinearTask) GetTitle() string {
	return t.ticket.Title
}

// GetDescription returns the ticket description.
func (t *LinearTask) GetDescription() string {
	return t.ticket.Description
}

// GetBranchName returns the branch name from Linear or generates one.
func (t *LinearTask) GetBranchName() string {
	if t.ticket.BranchName != "" {
		return t.ticket.BranchName
	}
	return fmt.Sprintf("%s-%s", t.ticket.Identifier, sanitizeBranchName(t.ticket.Title))
}

// GetLabels returns the ticket labels.
func (t *LinearTask) GetLabels() []string {
	return t.ticket.Labels
}

// GetMetadata returns task metadata.
func (t *LinearTask) GetMetadata() TaskMetadata {
	return TaskMetadata{
		Source:    SourceLinear,
		CreatedAt: time.Now(), // Linear tickets don't expose creation time in our client
	}
}

// GetTicket returns the underlying Linear ticket for backward compatibility.
// This allows existing code to access ticket-specific fields if needed.
func (t *LinearTask) GetTicket() *linear.Ticket {
	return t.ticket
}

// generateTaskID creates a unique task ID for non-Linear tasks.
func generateTaskID() string {
	timestamp := time.Now().Format("20060102-150405")
	hash := randomString(6)
	return fmt.Sprintf("prompt-%s-%s", timestamp, hash)
}

// randomString generates a random alphanumeric string of given length.
func randomString(n int) string {
	b := make([]byte, n/2+1)
	rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

// extractTitle attempts to extract a meaningful title from prompt text.
func extractTitle(prompt string) string {
	lines := strings.Split(strings.TrimSpace(prompt), "\n")
	if len(lines) == 0 {
		return "Untitled task"
	}

	firstLine := strings.TrimSpace(lines[0])

	// Check for markdown header: # Title
	if strings.HasPrefix(firstLine, "#") {
		title := strings.TrimSpace(strings.TrimPrefix(firstLine, "#"))
		title = strings.TrimSpace(strings.TrimPrefix(title, "#")) // Handle ## as well
		if len(title) > 0 {
			return truncateTitle(title)
		}
	}

	// Use first line (up to 50 chars)
	if len(firstLine) > 0 {
		return truncateTitle(firstLine)
	}

	return "Untitled task"
}

// truncateTitle limits title length.
func truncateTitle(title string) string {
	const maxLen = 50
	if len(title) <= maxLen {
		return title
	}
	return title[:maxLen]
}

// sanitizeBranchName makes a string safe for use in git branch names.
func sanitizeBranchName(s string) string {
	s = strings.ToLower(s)

	// Replace spaces and slashes with hyphens
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ":", "")

	// Remove characters that are invalid in branch names
	reg := regexp.MustCompile(`[^a-z0-9\-_]`)
	s = reg.ReplaceAllString(s, "")

	// Remove consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	s = reg.ReplaceAllString(s, "-")

	// Trim hyphens from start/end
	s = strings.Trim(s, "-")

	// Limit length
	if len(s) > 30 {
		s = s[:30]
	}

	if s == "" {
		return "untitled"
	}

	return s
}
