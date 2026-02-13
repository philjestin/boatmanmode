# BoatmanMode Examples

This directory contains example applications showing how to use BoatmanMode as a Go library.

## Examples

### Simple Example

**Location**: `simple/`

A basic example that shows how to:
- Create an agent with configuration
- Create a task from a prompt
- Execute the workflow programmatically
- Handle the results

**Run it:**

```bash
cd simple
export LINEAR_KEY="your-linear-api-key"
go run main.go
```

## Creating Your Own Integration

### 1. Install the Module

```bash
go get github.com/philjestin/boatmanmode@latest
```

### 2. Basic Template

```go
package main

import (
    "context"
    "log"

    "github.com/philjestin/boatmanmode/internal/agent"
    "github.com/philjestin/boatmanmode/internal/config"
    "github.com/philjestin/boatmanmode/internal/task"
)

func main() {
    // Configure
    cfg := &config.Config{
        LinearKey:     "your-key",
        BaseBranch:    "main",
        MaxIterations: 3,
        EnableTools:   true,
    }

    // Create agent
    a, err := agent.New(cfg)
    if err != nil {
        log.Fatal(err)
    }

    // Create task
    t, err := task.CreateFromPrompt("Your task here", "", "")
    if err != nil {
        log.Fatal(err)
    }

    // Execute
    result, err := a.Work(context.Background(), t)
    if err != nil {
        log.Fatal(err)
    }

    // Handle result
    if result.PRCreated {
        log.Printf("PR created: %s", result.PRURL)
    }
}
```

## More Examples Coming Soon

We'll be adding more examples for:
- Custom task sources (Jira, GitHub Issues, etc.)
- Web service integration
- Batch processing
- CI/CD integration
- Custom review agents

## Documentation

See [LIBRARY_USAGE.md](../LIBRARY_USAGE.md) for complete API documentation.

## Questions?

Open an issue on [GitHub](https://github.com/philjestin/boatmanmode/issues).
