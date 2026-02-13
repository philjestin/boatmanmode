package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/philjestin/boatmanmode"
)

func main() {
	// Get Linear API key from environment
	linearKey := os.Getenv("LINEAR_KEY")
	if linearKey == "" {
		log.Fatal("LINEAR_KEY environment variable is required")
	}

	// Create configuration
	cfg := &boatmanmode.Config{
		LinearKey:     linearKey,
		BaseBranch:    "main",
		MaxIterations: 3,
		ReviewSkill:   "peer-review",
		EnableTools:   true,
		Claude: boatmanmode.ClaudeConfig{
			EnablePromptCaching: true,
		},
	}

	// Create agent
	a, err := boatmanmode.NewAgent(cfg)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Create a task from a prompt
	t, err := boatmanmode.NewPromptTask(
		`# Add health check endpoint

Add a simple HTTP health check endpoint at /health that returns:
- status: "healthy"
- timestamp: current time in ISO 8601 format
- version: application version from config

Follow existing API patterns in the codebase.`,
		"", // auto-generate title
		"", // auto-generate branch name
	)
	if err != nil {
		log.Fatalf("Failed to create task: %v", err)
	}

	fmt.Printf("Created task: %s\n", t.GetID())
	fmt.Printf("Title: %s\n", t.GetTitle())
	fmt.Printf("Branch: %s\n", t.GetBranchName())
	fmt.Println()

	// Execute the workflow
	ctx := context.Background()
	result, err := a.Work(ctx, t)
	if err != nil {
		log.Fatalf("Work failed: %v", err)
	}

	// Display results
	fmt.Println()
	fmt.Println("=== Results ===")
	if result.PRCreated {
		fmt.Printf("✅ PR created: %s\n", result.PRURL)
		fmt.Printf("Iterations: %d\n", result.Iterations)
		fmt.Printf("Tests passed: %v\n", result.TestsPassed)
		if result.TestCoverage > 0 {
			fmt.Printf("Test coverage: %.1f%%\n", result.TestCoverage)
		}
	} else {
		fmt.Printf("⚠️  Work completed but PR not created: %s\n", result.Message)
	}
}
