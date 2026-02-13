package task

import (
	"context"
	"fmt"

	"github.com/philjestin/boatmanmode/internal/linear"
)

// InputMode represents how the task was specified.
type InputMode string

const (
	ModeLinear InputMode = "linear"
	ModePrompt InputMode = "prompt"
	ModeFile   InputMode = "file"
)

// CreateFromLinear creates a Task from a Linear ticket.
func CreateFromLinear(ctx context.Context, linearClient *linear.Client, ticketID string) (Task, error) {
	ticket, err := linearClient.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Linear ticket: %w", err)
	}
	return NewLinearTask(ticket), nil
}

// CreateFromPrompt creates a Task from an inline prompt.
func CreateFromPrompt(prompt string, overrideTitle, overrideBranch string) (Task, error) {
	if prompt == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}
	return NewPromptTask(prompt, overrideTitle, overrideBranch), nil
}

// CreateFromFile creates a Task from a file.
func CreateFromFile(filePath string, overrideTitle, overrideBranch string) (Task, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}
	return NewFileTask(filePath, overrideTitle, overrideBranch)
}
