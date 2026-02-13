// Package boatmanmode provides a public API for using BoatmanMode as a library.
// This is a wrapper around the internal packages to provide a stable public API.
package boatmanmode

import (
	"context"

	"github.com/philjestin/boatmanmode/internal/agent"
	"github.com/philjestin/boatmanmode/internal/config"
	"github.com/philjestin/boatmanmode/internal/linear"
	"github.com/philjestin/boatmanmode/internal/task"
)

// Agent orchestrates the complete development workflow.
type Agent struct {
	internal *agent.Agent
}

// Config contains the configuration for an Agent.
type Config struct {
	// Linear API key
	LinearKey string

	// Git configuration
	BaseBranch string // Base branch for worktrees (default: "main")

	// Workflow configuration
	MaxIterations int    // Maximum review/refactor iterations (default: 3)
	ReviewSkill   string // Claude skill for code review (default: "peer-review")
	EnableTools   bool   // Enable Claude tools (default: true)

	// Claude configuration
	Claude ClaudeConfig
}

// ClaudeConfig contains Claude-specific configuration.
type ClaudeConfig struct {
	// Model selection per agent type
	Models struct {
		Planner  string // Model for planning
		Executor string // Model for execution
		Refactor string // Model for refactoring
	}
	EnablePromptCaching bool // Enable prompt caching (default: true)
}

// WorkResult represents the outcome of a work execution.
type WorkResult struct {
	PRCreated    bool    // Whether a PR was created
	PRURL        string  // URL of the created PR
	Message      string  // Status message
	Iterations   int     // Number of review/refactor iterations
	TestsPassed  bool    // Whether tests passed
	TestCoverage float64 // Test coverage percentage
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

// TaskMetadata holds additional task information.
type TaskMetadata struct {
	Source    TaskSource
	CreatedAt string
	FilePath  string // For file-based tasks
}

// TaskSource represents the origin of a task.
type TaskSource string

const (
	// SourceLinear indicates the task came from a Linear ticket
	SourceLinear TaskSource = "linear"
	// SourcePrompt indicates the task came from an inline prompt
	SourcePrompt TaskSource = "prompt"
	// SourceFile indicates the task came from a file
	SourceFile TaskSource = "file"
)

// NewAgent creates a new Agent with the given configuration.
func NewAgent(cfg *Config) (*Agent, error) {
	// Convert public config to internal config
	internalCfg := &config.Config{
		LinearKey:     cfg.LinearKey,
		BaseBranch:    cfg.BaseBranch,
		MaxIterations: cfg.MaxIterations,
		ReviewSkill:   cfg.ReviewSkill,
		EnableTools:   cfg.EnableTools,
		Claude: config.ClaudeConfig{
			EnablePromptCaching: cfg.Claude.EnablePromptCaching,
		},
	}

	// Set defaults
	if internalCfg.BaseBranch == "" {
		internalCfg.BaseBranch = "main"
	}
	if internalCfg.MaxIterations == 0 {
		internalCfg.MaxIterations = 3
	}
	if internalCfg.ReviewSkill == "" {
		internalCfg.ReviewSkill = "peer-review"
	}

	// Copy model configuration
	internalCfg.Claude.Models.Planner = cfg.Claude.Models.Planner
	internalCfg.Claude.Models.Executor = cfg.Claude.Models.Executor
	internalCfg.Claude.Models.Refactor = cfg.Claude.Models.Refactor

	a, err := agent.New(internalCfg)
	if err != nil {
		return nil, err
	}

	return &Agent{internal: a}, nil
}

// Work executes the complete development workflow for a task.
func (a *Agent) Work(ctx context.Context, t Task) (*WorkResult, error) {
	// Convert public task to internal task
	var internalTask task.Task
	switch v := t.(type) {
	case *PromptTask:
		internalTask = v.internal
	case *FileTask:
		internalTask = v.internal
	case *LinearTask:
		internalTask = v.internal
	}

	result, err := a.internal.Work(ctx, internalTask)
	if err != nil {
		return nil, err
	}

	return &WorkResult{
		PRCreated:    result.PRCreated,
		PRURL:        result.PRURL,
		Message:      result.Message,
		Iterations:   result.Iterations,
		TestsPassed:  result.TestsPassed,
		TestCoverage: result.TestCoverage,
	}, nil
}

// PromptTask represents a task created from an inline text prompt.
type PromptTask struct {
	internal task.Task
}

// NewPromptTask creates a task from an inline prompt.
func NewPromptTask(prompt string, title string, branchName string) (*PromptTask, error) {
	t, err := task.CreateFromPrompt(prompt, title, branchName)
	if err != nil {
		return nil, err
	}
	return &PromptTask{internal: t}, nil
}

func (t *PromptTask) GetID() string              { return t.internal.GetID() }
func (t *PromptTask) GetTitle() string           { return t.internal.GetTitle() }
func (t *PromptTask) GetDescription() string     { return t.internal.GetDescription() }
func (t *PromptTask) GetBranchName() string      { return t.internal.GetBranchName() }
func (t *PromptTask) GetLabels() []string        { return t.internal.GetLabels() }
func (t *PromptTask) GetMetadata() TaskMetadata {
	m := t.internal.GetMetadata()
	return TaskMetadata{
		Source:    TaskSource(m.Source),
		CreatedAt: m.CreatedAt.String(),
		FilePath:  m.FilePath,
	}
}

// FileTask represents a task read from a file.
type FileTask struct {
	internal task.Task
}

// NewFileTask creates a task from a file.
func NewFileTask(filePath string, title string, branchName string) (*FileTask, error) {
	t, err := task.CreateFromFile(filePath, title, branchName)
	if err != nil {
		return nil, err
	}
	return &FileTask{internal: t}, nil
}

func (t *FileTask) GetID() string              { return t.internal.GetID() }
func (t *FileTask) GetTitle() string           { return t.internal.GetTitle() }
func (t *FileTask) GetDescription() string     { return t.internal.GetDescription() }
func (t *FileTask) GetBranchName() string      { return t.internal.GetBranchName() }
func (t *FileTask) GetLabels() []string        { return t.internal.GetLabels() }
func (t *FileTask) GetMetadata() TaskMetadata {
	m := t.internal.GetMetadata()
	return TaskMetadata{
		Source:    TaskSource(m.Source),
		CreatedAt: m.CreatedAt.String(),
		FilePath:  m.FilePath,
	}
}

// LinearTask represents a task from a Linear ticket.
type LinearTask struct {
	internal task.Task
}

// NewLinearTask creates a task from a Linear ticket ID.
func NewLinearTask(ctx context.Context, linearAPIKey string, ticketID string) (*LinearTask, error) {
	linearClient := linear.New(linearAPIKey)
	t, err := task.CreateFromLinear(ctx, linearClient, ticketID)
	if err != nil {
		return nil, err
	}
	return &LinearTask{internal: t}, nil
}

func (t *LinearTask) GetID() string              { return t.internal.GetID() }
func (t *LinearTask) GetTitle() string           { return t.internal.GetTitle() }
func (t *LinearTask) GetDescription() string     { return t.internal.GetDescription() }
func (t *LinearTask) GetBranchName() string      { return t.internal.GetBranchName() }
func (t *LinearTask) GetLabels() []string        { return t.internal.GetLabels() }
func (t *LinearTask) GetMetadata() TaskMetadata {
	m := t.internal.GetMetadata()
	return TaskMetadata{
		Source:    TaskSource(m.Source),
		CreatedAt: m.CreatedAt.String(),
		FilePath:  m.FilePath,
	}
}
