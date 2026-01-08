// Package agent orchestrates the complete workflow:
// fetch ticket → create worktree → execute → review → refactor → PR
package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/handshake/boatmanmode/internal/config"
	"github.com/handshake/boatmanmode/internal/executor"
	"github.com/handshake/boatmanmode/internal/github"
	"github.com/handshake/boatmanmode/internal/handoff"
	"github.com/handshake/boatmanmode/internal/linear"
	"github.com/handshake/boatmanmode/internal/planner"
	"github.com/handshake/boatmanmode/internal/scottbott"
	"github.com/handshake/boatmanmode/internal/worktree"
)

// Agent orchestrates the development workflow.
type Agent struct {
	config       *config.Config
	linearClient *linear.Client
}

// WorkResult represents the outcome of the work command.
type WorkResult struct {
	PRCreated  bool
	PRURL      string
	Message    string
	Iterations int
}

// New creates a new Agent.
func New(cfg *config.Config) (*Agent, error) {
	return &Agent{
		config:       cfg,
		linearClient: linear.New(cfg.LinearKey),
	}, nil
}

// Work executes the complete workflow for a ticket.
func (a *Agent) Work(ctx context.Context, ticketID string) (*WorkResult, error) {
	totalStart := time.Now()
	
	printStep(1, 8, "Fetching ticket from Linear")
	fmt.Printf("   🎫 Ticket ID: %s\n", ticketID)

	ticket, err := a.linearClient.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ticket: %w", err)
	}

	fmt.Printf("   📋 Title: %s\n", ticket.Title)
	fmt.Printf("   🏷️  Labels: %s\n", strings.Join(ticket.Labels, ", "))
	fmt.Println()
	fmt.Println("   📝 Description:")
	printIndented(truncate(ticket.Description, 800), "      ")
	fmt.Println()

	// Step 2: Create worktree
	printStep(2, 8, "Setting up git worktree")
	
	repoPath, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	fmt.Printf("   📂 Repo: %s\n", repoPath)

	wtManager, err := worktree.New(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree manager: %w", err)
	}

	branchName := ticket.BranchName
	if branchName == "" {
		branchName = fmt.Sprintf("%s-%s", ticket.Identifier, sanitize(ticket.Title))
	}
	fmt.Printf("   🌿 Branch: %s\n", branchName)

	wt, err := wtManager.Create(branchName, a.config.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}
	fmt.Printf("   📁 Worktree: %s\n", wt.Path)
	fmt.Println()

	// Step 3: Planning - Claude agent analyzes ticket and codebase
	printStep(3, 8, "Planning (analyzing ticket & codebase)")
	
	planAgent := planner.New(wt.Path)
	plan, err := planAgent.Analyze(ctx, ticket)
	if err != nil {
		fmt.Printf("   ⚠️  Planning failed: %v (continuing without plan)\n", err)
		plan = nil
	}
	fmt.Println()

	// Step 4: Execute the task with plan handoff
	printStep(4, 8, "Executing development task")
	
	exec := executor.New(wt.Path)
	result, err := exec.ExecuteWithPlan(ctx, ticket, plan)
	if err != nil {
		return nil, fmt.Errorf("execution failed: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("execution failed: %v", result.Error)
	}
	fmt.Println()

	// Stage changes
	fmt.Println("   📥 Staging changes...")
	if err := exec.StageChanges(); err != nil {
		return nil, fmt.Errorf("failed to stage changes: %w", err)
	}

	// Step 5-6: Review loop
	printStep(5, 8, "Code review with ScottBott")
	
	var iterations int
	var reviewResult *scottbott.ReviewResult

	for iterations < a.config.MaxIterations {
		iterations++
		fmt.Printf("\n   🔄 Review iteration %d of %d\n", iterations, a.config.MaxIterations)
		fmt.Println("   ─────────────────────────────")

		// Get diff for review
		fmt.Println("   📊 Generating diff for review...")
		diff, err := exec.GetDiff()
		if err != nil {
			return nil, fmt.Errorf("failed to get diff: %w", err)
		}
		fmt.Printf("   📏 Diff size: %d lines\n", strings.Count(diff, "\n"))

		// Create review handoff with concise context
		reviewHandoff := handoff.NewReviewHandoff(ticket, diff, result.FilesChanged)

		// Review with ScottBott - fresh agent for each review, runs in worktree
		fmt.Println("   🤖 Invoking peer-review skill...")
		reviewer := scottbott.NewWithWorkDir(wt.Path, iterations)
		reviewResult, err = reviewer.Review(ctx, reviewHandoff.ToPrompt(), diff)
		if err != nil {
			return nil, fmt.Errorf("review failed: %w", err)
		}
		fmt.Println()

		fmt.Println(reviewResult.FormatReview())

		if reviewResult.Passed {
			fmt.Println("   ✅ Review passed! Proceeding to PR...")
			break
		}

		if iterations >= a.config.MaxIterations {
			fmt.Println("   ⚠️  Maximum iterations reached without passing review")
			break
		}

		// Refactor based on feedback - use a fresh agent for each iteration
		printStep(6, 8, fmt.Sprintf("Refactoring (attempt %d)", iterations))
		
		// Create concise refactor handoff
		refactorExec := executor.NewRefactorExecutor(wt.Path, iterations)
		currentCode, _ := refactorExec.GetSpecificFiles(result.FilesChanged)
		
		refactorHandoff := handoff.NewRefactorHandoff(
			ticket,
			reviewResult.GetIssueDescriptions(),
			reviewResult.Guidance,
			result.FilesChanged,
			currentCode,
		)
		
		refactorResult, err := refactorExec.RefactorWithHandoff(ctx, refactorHandoff)
		if err != nil {
			return nil, fmt.Errorf("refactor failed: %w", err)
		}
		
		// Update the result with new files for next iteration
		result.FilesChanged = refactorResult.FilesChanged

		if !refactorResult.Success {
			return nil, fmt.Errorf("refactor failed: %v", refactorResult.Error)
		}

		// Stage new changes
		fmt.Println("   📥 Staging refactored changes...")
		if err := exec.StageChanges(); err != nil {
			return nil, fmt.Errorf("failed to stage changes: %w", err)
		}
	}

	// Only proceed if review passed
	if !reviewResult.Passed {
		return &WorkResult{
			PRCreated:  false,
			Message:    "Review did not pass after max iterations",
			Iterations: iterations,
		}, nil
	}

	// Step 6: Commit
	printStep(7, 8, "Committing and pushing")
	
	commitMsg := fmt.Sprintf("feat(%s): %s\n\n%s",
		ticket.Identifier,
		ticket.Title,
		reviewResult.Summary,
	)
	fmt.Println("   💾 Creating commit...")
	fmt.Printf("   📝 Message: %s\n", strings.Split(commitMsg, "\n")[0])

	if err := exec.Commit(commitMsg); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Println("   📤 Pushing to origin...")
	if err := exec.Push(branchName); err != nil {
		return nil, fmt.Errorf("failed to push: %w", err)
	}
	fmt.Println()

	// Step 7: Create PR
	printStep(8, 8, "Creating pull request")

	prBody := fmt.Sprintf(`## %s

### Ticket
[%s](https://linear.app/issue/%s)

### Description
%s

### Changes
%s

---
*Automated by BoatmanMode 🚣*
`,
		ticket.Title,
		ticket.Identifier,
		ticket.Identifier,
		ticket.Description,
		reviewResult.Summary,
	)

	// Change to worktree directory for gh CLI
	origDir, _ := os.Getwd()
	os.Chdir(wt.Path)
	defer os.Chdir(origDir)

	fmt.Println("   🔗 Running: gh pr create")
	prResult, err := github.CreatePR(ctx, ticket.Title, prBody, a.config.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	totalElapsed := time.Since(totalStart)
	
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Println("✅ WORKFLOW COMPLETE")
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Printf("   🎫 Ticket:     %s\n", ticket.Identifier)
	fmt.Printf("   🌿 Branch:     %s\n", branchName)
	fmt.Printf("   🔄 Iterations: %d\n", iterations)
	fmt.Printf("   ⏱️  Total time: %s\n", totalElapsed.Round(time.Second))
	fmt.Printf("   🔗 PR:         %s\n", prResult.URL)
	fmt.Println("═══════════════════════════════════════════════════════")

	return &WorkResult{
		PRCreated:  true,
		PRURL:      prResult.URL,
		Message:    "Successfully created PR",
		Iterations: iterations,
	}, nil
}

// printStep prints a formatted step header.
func printStep(current, total int, description string) {
	fmt.Println()
	fmt.Printf("━━━ Step %d/%d: %s ━━━\n", current, total, description)
}

// printIndented prints text with indentation.
func printIndented(text, indent string) {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		fmt.Printf("%s%s\n", indent, line)
	}
}

// getRepoURL gets the remote URL for the repository.
func getRepoURL(repoPath string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// truncate shortens a string to the given length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// sanitize makes a string safe for use in branch names.
func sanitize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ":", "")
	if len(s) > 30 {
		s = s[:30]
	}
	return s
}
