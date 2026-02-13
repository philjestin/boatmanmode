# Using BoatmanMode as a Go Library

BoatmanMode can be used as a Go module/library in your own applications, allowing you to programmatically execute development workflows.

## Installation

Add BoatmanMode to your project:

```bash
go get github.com/philjestin/boatmanmode@latest
```

Or install a specific version:

```bash
go get github.com/philjestin/boatmanmode@v1.0.0
```

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/philjestin/boatmanmode"
)

func main() {
    // Load configuration
    cfg := &boatmanmode.Config{
        LinearKey:      "your-linear-api-key",
        BaseBranch:     "main",
        MaxIterations:  3,
        ReviewSkill:    "peer-review",
        EnableTools:    true,
    }

    // Create agent
    a, err := boatmanmode.NewAgent(cfg)
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Create a task from a prompt
    t, err := boatmanmode.NewPromptTask(
        "Add a health check endpoint at /health",
        "", // auto-generate title
        "", // auto-generate branch name
    )
    if err != nil {
        log.Fatalf("Failed to create task: %v", err)
    }

    // Execute the workflow
    ctx := context.Background()
    result, err := a.Work(ctx, t)
    if err != nil {
        log.Fatalf("Work failed: %v", err)
    }

    // Check results
    if result.PRCreated {
        fmt.Printf("✅ PR created: %s\n", result.PRURL)
    } else {
        fmt.Printf("⚠️  %s\n", result.Message)
    }
}
```

## Task Types

### 1. Linear Tickets

```go
import "github.com/philjestin/boatmanmode"

// Fetch from Linear
t, err := boatmanmode.NewLinearTask(ctx, "your-api-key", "ENG-123")
```

### 2. Inline Prompts

```go
t, err := boatmanmode.NewPromptTask(
    "Implement user authentication with JWT tokens",
    "Authentication Feature",  // custom title
    "feature/auth",           // custom branch
)
```

### 3. File-based Prompts

```go
t, err := boatmanmode.NewFileTask(
    "./tasks/implement-caching.md",
    "",  // auto-generate title from file
    "",  // auto-generate branch
)
```

## Working with Task Interface

All task types implement the `Task` interface:

```go
type Task interface {
    GetID() string
    GetTitle() string
    GetDescription() string
    GetBranchName() string
    GetLabels() []string
    GetMetadata() TaskMetadata
}
```

This allows you to write code that works with any task source:

```go
import "github.com/philjestin/boatmanmode"

func processTask(t boatmanmode.Task) {
    fmt.Printf("Task ID: %s\n", t.GetID())
    fmt.Printf("Title: %s\n", t.GetTitle())
    fmt.Printf("Branch: %s\n", t.GetBranchName())

    metadata := t.GetMetadata()
    fmt.Printf("Source: %s\n", metadata.Source)
}
```

## Configuration Options

```go
type Config struct {
    // Linear API configuration
    LinearKey string

    // Git configuration
    BaseBranch string  // Base branch for worktrees (default: "main")

    // Workflow configuration
    MaxIterations int   // Max review/refactor iterations (default: 3)
    ReviewSkill   string // Claude skill for review (default: "peer-review")
    EnableTools   bool   // Enable Claude tools (default: true)

    // Claude configuration
    Claude ClaudeConfig
}

type ClaudeConfig struct {
    Models struct {
        Planner  string // Model for planning (default: uses Claude default)
        Executor string // Model for execution (default: uses Claude default)
        Refactor string // Model for refactoring (default: uses Claude default)
    }
    EnablePromptCaching bool // Enable prompt caching (default: true)
}
```

## Examples

### Example 1: Batch Processing Tasks

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/philjestin/boatmanmode/internal/agent"
    "github.com/philjestin/boatmanmode/internal/config"
    "github.com/philjestin/boatmanmode/internal/task"
)

func main() {
    cfg := &config.Config{
        LinearKey:     "your-api-key",
        BaseBranch:    "main",
        MaxIterations: 3,
        EnableTools:   true,
    }

    a, err := agent.New(cfg)
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // List of tasks to process
    prompts := []string{
        "Add health check endpoint",
        "Implement rate limiting",
        "Add request logging middleware",
    }

    ctx := context.Background()

    for i, prompt := range prompts {
        fmt.Printf("\n=== Processing task %d/%d ===\n", i+1, len(prompts))

        t, err := task.CreateFromPrompt(prompt, "", "")
        if err != nil {
            log.Printf("Failed to create task: %v", err)
            continue
        }

        result, err := a.Work(ctx, t)
        if err != nil {
            log.Printf("Task failed: %v", err)
            continue
        }

        if result.PRCreated {
            fmt.Printf("✅ PR created: %s\n", result.PRURL)
        }
    }
}
```

### Example 2: Custom Task Source

Implement your own task source by implementing the `Task` interface:

```go
package main

import (
    "time"

    "github.com/philjestin/boatmanmode/internal/task"
)

// JiraTask implements task.Task for Jira issues
type JiraTask struct {
    issueKey    string
    summary     string
    description string
    labels      []string
}

func NewJiraTask(issueKey, summary, description string, labels []string) task.Task {
    return &JiraTask{
        issueKey:    issueKey,
        summary:     summary,
        description: description,
        labels:      labels,
    }
}

func (t *JiraTask) GetID() string {
    return t.issueKey
}

func (t *JiraTask) GetTitle() string {
    return t.summary
}

func (t *JiraTask) GetDescription() string {
    return t.description
}

func (t *JiraTask) GetBranchName() string {
    // Sanitize for git branch name
    return fmt.Sprintf("%s-%s",
        strings.ToLower(t.issueKey),
        sanitizeBranchName(t.summary))
}

func (t *JiraTask) GetLabels() []string {
    return t.labels
}

func (t *JiraTask) GetMetadata() task.TaskMetadata {
    return task.TaskMetadata{
        Source:    task.TaskSource("jira"),
        CreatedAt: time.Now(),
    }
}

// Now use it with the agent
func main() {
    jiraTask := NewJiraTask(
        "PROJ-123",
        "Add authentication",
        "Implement JWT-based authentication for the API",
        []string{"feature", "security"},
    )

    // ... create agent and execute
    result, err := agent.Work(ctx, jiraTask)
}
```

### Example 3: Building a Web Service

```go
package main

import (
    "encoding/json"
    "net/http"

    "github.com/philjestin/boatmanmode/internal/agent"
    "github.com/philjestin/boatmanmode/internal/config"
    "github.com/philjestin/boatmanmode/internal/task"
)

type TaskRequest struct {
    Prompt string `json:"prompt"`
    Title  string `json:"title"`
    Branch string `json:"branch"`
}

type TaskResponse struct {
    TaskID    string `json:"task_id"`
    PRCreated bool   `json:"pr_created"`
    PRURL     string `json:"pr_url,omitempty"`
    Message   string `json:"message,omitempty"`
}

func main() {
    cfg := &config.Config{
        LinearKey:     "your-api-key",
        BaseBranch:    "main",
        MaxIterations: 3,
        EnableTools:   true,
    }

    a, err := agent.New(cfg)
    if err != nil {
        panic(err)
    }

    http.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
            return
        }

        var req TaskRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }

        // Create task from request
        t, err := task.CreateFromPrompt(req.Prompt, req.Title, req.Branch)
        if err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }

        // Execute async (you'd probably want to use a queue)
        go func() {
            result, err := a.Work(r.Context(), t)
            if err != nil {
                // Handle error (log, notify, etc.)
                return
            }
            // Store result, notify user, etc.
        }()

        // Return task ID immediately
        resp := TaskResponse{
            TaskID:  t.GetID(),
            Message: "Task queued for processing",
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(resp)
    })

    http.ListenAndServe(":8080", nil)
}
```

## Exported Packages

### Core Packages

- `github.com/philjestin/boatmanmode/internal/agent` - Main workflow orchestration
- `github.com/philjestin/boatmanmode/internal/task` - Task abstraction and implementations
- `github.com/philjestin/boatmanmode/internal/config` - Configuration management

### Specialized Packages

- `github.com/philjestin/boatmanmode/internal/linear` - Linear API client
- `github.com/philjestin/boatmanmode/internal/executor` - Claude-powered code execution
- `github.com/philjestin/boatmanmode/internal/planner` - Task planning and analysis
- `github.com/philjestin/boatmanmode/internal/scottbott` - Code review agent
- `github.com/philjestin/boatmanmode/internal/worktree` - Git worktree management

### Utility Packages

- `github.com/philjestin/boatmanmode/internal/cost` - Cost tracking for Claude API
- `github.com/philjestin/boatmanmode/internal/handoff` - Context passing between agents
- `github.com/philjestin/boatmanmode/internal/retry` - Retry logic with backoff

## API Stability

BoatmanMode follows [Semantic Versioning](https://semver.org/):

- **Major version (v2.x.x)**: Breaking API changes
- **Minor version (v1.x.x)**: New features, backward compatible
- **Patch version (v1.0.x)**: Bug fixes, backward compatible

### Current Status

The library is currently in **active development**. While we maintain backward compatibility within major versions, APIs may evolve as we gather feedback.

### Import Path

Always use the versioned import path:

```go
import "github.com/philjestin/boatmanmode"
```

For specific versions:

```bash
go get github.com/philjestin/boatmanmode@v1.0.0
```

## Best Practices

1. **Error Handling**: Always check errors returned by the agent
2. **Context**: Use context with timeouts for long-running operations
3. **Configuration**: Load configuration from environment or config files
4. **Concurrency**: The agent is not thread-safe; use one agent per goroutine
5. **Resource Cleanup**: Worktrees are automatically cleaned up on completion

## Integration Examples

### GitHub Actions

Use boatmanmode in GitHub Actions workflows:

```yaml
name: Auto-implement feature
on:
  issues:
    types: [labeled]

jobs:
  implement:
    if: contains(github.event.issue.labels.*.name, 'auto-implement')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Install boatman
        run: |
          go install github.com/philjestin/boatmanmode/cmd/boatman@latest

      - name: Execute task
        env:
          LINEAR_KEY: ${{ secrets.LINEAR_KEY }}
        run: |
          boatman work --prompt "${{ github.event.issue.body }}"
```

### CI/CD Pipelines

Integrate into your deployment pipeline to auto-fix issues:

```go
// In your CI/CD tool
if testsFailed {
    prompt := fmt.Sprintf("Fix failing test: %s\nError: %s",
        failedTest.Name,
        failedTest.Error)

    task, _ := task.CreateFromPrompt(prompt, "", "")
    result, _ := agent.Work(ctx, task)

    if result.PRCreated {
        notifyTeam(result.PRURL)
    }
}
```

## Support

- **Documentation**: [GitHub Wiki](https://github.com/philjestin/boatmanmode/wiki)
- **Issues**: [GitHub Issues](https://github.com/philjestin/boatmanmode/issues)
- **Examples**: See the `examples/` directory in the repository

## License

BoatmanMode is released under the MIT License. See LICENSE file for details.
