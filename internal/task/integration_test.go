package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/philjestin/boatmanmode/internal/linear"
)

// TestTaskInterfaceCompatibility ensures all task types work with the same interface.
func TestTaskInterfaceCompatibility(t *testing.T) {
	// Create different task types
	linearTicket := &linear.Ticket{
		Identifier:  "ENG-999",
		Title:       "Test Linear Task",
		Description: "This is a test ticket from Linear",
		Labels:      []string{"test", "integration"},
	}

	promptText := "# Test Prompt Task\n\nImplement a new feature for testing"

	// Create tasks
	tasks := []Task{
		NewLinearTask(linearTicket),
		NewPromptTask(promptText, "", ""),
		NewPromptTask(promptText, "Custom Title", "custom-branch"),
	}

	// Verify all tasks implement the interface correctly
	for i, task := range tasks {
		if task.GetID() == "" {
			t.Errorf("Task %d: GetID() returned empty string", i)
		}

		if task.GetTitle() == "" {
			t.Errorf("Task %d: GetTitle() returned empty string", i)
		}

		if task.GetDescription() == "" {
			t.Errorf("Task %d: GetDescription() returned empty string", i)
		}

		if task.GetBranchName() == "" {
			t.Errorf("Task %d: GetBranchName() returned empty string", i)
		}

		// GetLabels can be empty, just verify it returns non-nil
		if task.GetLabels() == nil {
			t.Errorf("Task %d: GetLabels() returned nil", i)
		}

		metadata := task.GetMetadata()
		if metadata.Source == "" {
			t.Errorf("Task %d: GetMetadata().Source is empty", i)
		}

		if metadata.CreatedAt.IsZero() {
			t.Errorf("Task %d: GetMetadata().CreatedAt is zero", i)
		}
	}
}

// TestLinearTaskBackwardCompatibility verifies LinearTask can be used in place of linear.Ticket.
func TestLinearTaskBackwardCompatibility(t *testing.T) {
	ticket := &linear.Ticket{
		ID:          "linear-id-123",
		Identifier:  "ENG-456",
		Title:       "Backward Compatibility Test",
		Description: "Testing backward compatibility",
		Labels:      []string{"compat"},
		BranchName:  "custom-branch",
		State:       "In Progress",
		Priority:    1,
	}

	linearTask := NewLinearTask(ticket).(*LinearTask)

	// Verify we can still access the original ticket
	originalTicket := linearTask.GetTicket()
	if originalTicket.ID != ticket.ID {
		t.Errorf("Expected ticket ID %s, got %s", ticket.ID, originalTicket.ID)
	}

	// Verify all fields match
	if linearTask.GetID() != ticket.Identifier {
		t.Error("ID mismatch")
	}
	if linearTask.GetTitle() != ticket.Title {
		t.Error("Title mismatch")
	}
	if linearTask.GetDescription() != ticket.Description {
		t.Error("Description mismatch")
	}
}

// TestPromptTaskUniqueness verifies that multiple prompt tasks get unique IDs.
func TestPromptTaskUniqueness(t *testing.T) {
	prompt := "Same prompt text"

	task1 := NewPromptTask(prompt, "", "")
	task2 := NewPromptTask(prompt, "", "")

	if task1.GetID() == task2.GetID() {
		t.Error("Two prompt tasks with same content should have different IDs")
	}

	// But they should have the same title (extracted from same prompt)
	if task1.GetTitle() != task2.GetTitle() {
		t.Error("Tasks with same prompt should have same extracted title")
	}
}

// TestFileTaskWithRealFile verifies file tasks work with actual files.
func TestFileTaskWithRealFile(t *testing.T) {
	tmpDir := t.TempDir()
	taskFile := filepath.Join(tmpDir, "my-task.md")

	content := `# Implement user authentication

Add JWT-based authentication to the API with the following features:
- Login endpoint
- Token validation middleware
- Refresh token support`

	if err := os.WriteFile(taskFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	task, err := NewFileTask(taskFile, "", "")
	if err != nil {
		t.Fatalf("Failed to create file task: %v", err)
	}

	// Verify title extraction
	if task.GetTitle() != "Implement user authentication" {
		t.Errorf("Expected title 'Implement user authentication', got %s", task.GetTitle())
	}

	// Verify full content is in description
	if task.GetDescription() != content {
		t.Error("Description should match file content")
	}

	// Verify metadata
	metadata := task.GetMetadata()
	if metadata.Source != SourceFile {
		t.Errorf("Expected source %s, got %s", SourceFile, metadata.Source)
	}

	if metadata.FilePath != taskFile {
		t.Errorf("Expected file path %s, got %s", taskFile, metadata.FilePath)
	}
}

// TestBranchNameSafety verifies all generated branch names are git-safe.
func TestBranchNameSafety(t *testing.T) {
	unsafeTitles := []string{
		"Fix: Bug in /api/v1/users endpoint",
		"Add support for C++",
		"Feature: User @mentions",
		"Update README.md with instructions",
		"Refactor (Phase 1)",
	}

	for _, title := range unsafeTitles {
		prompt := "# " + title + "\n\nDetails..."
		task := NewPromptTask(prompt, "", "")

		branchName := task.GetBranchName()

		// Verify no forbidden characters
		if containsAny(branchName, []string{" ", ":", "@", "~", "^", "?", "*", "[", "\\", ".."}) {
			t.Errorf("Branch name contains unsafe characters: %s (from title: %s)", branchName, title)
		}

		// Verify it's not empty
		if branchName == "" {
			t.Error("Branch name should not be empty")
		}
	}
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
