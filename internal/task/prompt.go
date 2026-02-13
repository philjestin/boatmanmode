package task

import (
	"fmt"
	"os"
	"time"
)

// PromptTask represents a task created from an inline text prompt.
type PromptTask struct {
	id          string
	title       string
	description string
	branchName  string
	labels      []string
	createdAt   time.Time
}

// NewPromptTask creates a Task from an inline prompt.
func NewPromptTask(prompt string, overrideTitle, overrideBranch string) Task {
	id := generateTaskID()
	title := overrideTitle
	if title == "" {
		title = extractTitle(prompt)
	}

	branchName := overrideBranch
	if branchName == "" {
		// Generate branch name: prompt-timestamp-hash-title
		branchName = fmt.Sprintf("%s-%s", id, sanitizeBranchName(title))
	}

	return &PromptTask{
		id:          id,
		title:       title,
		description: prompt,
		branchName:  branchName,
		labels:      []string{},
		createdAt:   time.Now(),
	}
}

// GetID returns the auto-generated task ID.
func (t *PromptTask) GetID() string {
	return t.id
}

// GetTitle returns the extracted or overridden title.
func (t *PromptTask) GetTitle() string {
	return t.title
}

// GetDescription returns the full prompt text.
func (t *PromptTask) GetDescription() string {
	return t.description
}

// GetBranchName returns the generated or overridden branch name.
func (t *PromptTask) GetBranchName() string {
	return t.branchName
}

// GetLabels returns an empty list (prompts don't have labels).
func (t *PromptTask) GetLabels() []string {
	return t.labels
}

// GetMetadata returns task metadata.
func (t *PromptTask) GetMetadata() TaskMetadata {
	return TaskMetadata{
		Source:    SourcePrompt,
		CreatedAt: t.createdAt,
	}
}

// FileTask represents a task read from a file.
type FileTask struct {
	*PromptTask
	filePath string
}

// NewFileTask creates a Task from a file.
func NewFileTask(filePath string, overrideTitle, overrideBranch string) (Task, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read task file: %w", err)
	}

	promptTask := NewPromptTask(string(content), overrideTitle, overrideBranch).(*PromptTask)

	return &FileTask{
		PromptTask: promptTask,
		filePath:   filePath,
	}, nil
}

// GetMetadata returns task metadata with file path.
func (t *FileTask) GetMetadata() TaskMetadata {
	metadata := t.PromptTask.GetMetadata()
	metadata.Source = SourceFile
	metadata.FilePath = t.filePath
	return metadata
}
