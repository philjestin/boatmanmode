package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/philjestin/boatmanmode/internal/linear"
)

func TestLinearTask(t *testing.T) {
	ticket := &linear.Ticket{
		ID:          "ticket-123",
		Identifier:  "ENG-456",
		Title:       "Add authentication",
		Description: "Implement JWT-based authentication",
		Labels:      []string{"feature", "security"},
		BranchName:  "custom-branch-name",
	}

	task := NewLinearTask(ticket)

	if task.GetID() != "ENG-456" {
		t.Errorf("expected ID %s, got %s", "ENG-456", task.GetID())
	}

	if task.GetTitle() != "Add authentication" {
		t.Errorf("expected title %s, got %s", "Add authentication", task.GetTitle())
	}

	if task.GetBranchName() != "custom-branch-name" {
		t.Errorf("expected branch %s, got %s", "custom-branch-name", task.GetBranchName())
	}

	metadata := task.GetMetadata()
	if metadata.Source != SourceLinear {
		t.Errorf("expected source %s, got %s", SourceLinear, metadata.Source)
	}

	// Test with GetTicket for backward compatibility
	lt, ok := task.(*LinearTask)
	if !ok {
		t.Error("expected LinearTask type")
	}
	if lt.GetTicket().Identifier != "ENG-456" {
		t.Error("GetTicket() should return original ticket")
	}
}

func TestLinearTask_GeneratedBranchName(t *testing.T) {
	ticket := &linear.Ticket{
		Identifier:  "ENG-789",
		Title:       "Fix bug in auth flow",
		Description: "Details...",
	}

	task := NewLinearTask(ticket)

	branchName := task.GetBranchName()
	if !strings.HasPrefix(branchName, "ENG-789-") {
		t.Errorf("expected branch to start with ENG-789-, got %s", branchName)
	}

	// Should be sanitized
	if strings.Contains(branchName, " ") {
		t.Error("branch name should not contain spaces")
	}
}

func TestPromptTask(t *testing.T) {
	prompt := "# Add user registration\n\nImplement user registration with email validation"

	task := NewPromptTask(prompt, "", "")

	// ID should be auto-generated
	id := task.GetID()
	if !strings.HasPrefix(id, "prompt-") {
		t.Errorf("expected ID to start with prompt-, got %s", id)
	}

	// Title should be extracted from markdown header
	title := task.GetTitle()
	if title != "Add user registration" {
		t.Errorf("expected title %s, got %s", "Add user registration", title)
	}

	// Description should be full prompt
	if task.GetDescription() != prompt {
		t.Error("description should match original prompt")
	}

	// Branch name should be generated
	branchName := task.GetBranchName()
	if !strings.Contains(branchName, "add-user-registration") {
		t.Errorf("expected branch to contain sanitized title, got %s", branchName)
	}

	metadata := task.GetMetadata()
	if metadata.Source != SourcePrompt {
		t.Errorf("expected source %s, got %s", SourcePrompt, metadata.Source)
	}
}

func TestPromptTask_WithOverrides(t *testing.T) {
	prompt := "Do some work"

	task := NewPromptTask(prompt, "Custom Title", "custom-branch")

	if task.GetTitle() != "Custom Title" {
		t.Errorf("expected overridden title, got %s", task.GetTitle())
	}

	if task.GetBranchName() != "custom-branch" {
		t.Errorf("expected overridden branch, got %s", task.GetBranchName())
	}
}

func TestFileTask(t *testing.T) {
	// Create temporary file
	tmpDir := t.TempDir()
	taskFile := filepath.Join(tmpDir, "task.txt")

	content := "# Refactor error handling\n\nUpdate error handling to use custom error types"
	if err := os.WriteFile(taskFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	task, err := NewFileTask(taskFile, "", "")
	if err != nil {
		t.Fatalf("failed to create file task: %v", err)
	}

	// Title should be extracted from file content
	if task.GetTitle() != "Refactor error handling" {
		t.Errorf("expected extracted title, got %s", task.GetTitle())
	}

	// Description should be full file content
	if task.GetDescription() != content {
		t.Error("description should match file content")
	}

	metadata := task.GetMetadata()
	if metadata.Source != SourceFile {
		t.Errorf("expected source %s, got %s", SourceFile, metadata.Source)
	}

	if metadata.FilePath != taskFile {
		t.Errorf("expected file path %s, got %s", taskFile, metadata.FilePath)
	}
}

func TestFileTask_NonexistentFile(t *testing.T) {
	_, err := NewFileTask("/nonexistent/file.txt", "", "")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		expected string
	}{
		{
			name:     "markdown header",
			prompt:   "# Add authentication\n\nDetails...",
			expected: "Add authentication",
		},
		{
			name:     "double hash",
			prompt:   "## Fix bug\n\nMore details",
			expected: "Fix bug",
		},
		{
			name:     "first line no header",
			prompt:   "Implement user login\n\nWith JWT tokens",
			expected: "Implement user login",
		},
		{
			name:     "empty lines",
			prompt:   "\n\nActual task\n\nDetails",
			expected: "Actual task",
		},
		{
			name:     "very long first line",
			prompt:   "This is a very long task description that exceeds fifty characters and should be truncated",
			expected: "This is a very long task description that exceeds ", // 50 chars exactly
		},
		{
			name:     "empty prompt",
			prompt:   "",
			expected: "Untitled task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTitle(tt.prompt)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "spaces to hyphens",
			input:    "Add User Auth",
			expected: "add-user-auth",
		},
		{
			name:     "remove special chars",
			input:    "Fix: Bug#123",
			expected: "fix-bug123",
		},
		{
			name:     "slashes to hyphens",
			input:    "feature/user-login",
			expected: "feature-user-login",
		},
		{
			name:     "long name truncated",
			input:    "This is a very long branch name that should be truncated",
			expected: "this-is-a-very-long-branch-nam", // 30 chars exactly
		},
		{
			name:     "consecutive hyphens",
			input:    "Add   Multiple   Spaces",
			expected: "add-multiple-spaces",
		},
		{
			name:     "only special chars",
			input:    "!!!",
			expected: "untitled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeBranchName(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGenerateTaskID(t *testing.T) {
	id1 := generateTaskID()
	id2 := generateTaskID()

	// IDs should be unique
	if id1 == id2 {
		t.Error("generated IDs should be unique")
	}

	// Should have expected format
	if !strings.HasPrefix(id1, "prompt-") {
		t.Errorf("ID should start with prompt-, got %s", id1)
	}

	// Should contain timestamp and hash
	parts := strings.Split(id1, "-")
	if len(parts) != 4 { // prompt-YYYYMMDD-HHMMSS-hash
		t.Errorf("expected 4 parts in ID, got %d: %s", len(parts), id1)
	}
}
