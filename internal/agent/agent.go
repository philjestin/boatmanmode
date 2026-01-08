// Package agent orchestrates the complete workflow:
// fetch ticket → create worktree → validate → execute → test → review → verify → refactor → PR
package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/handshake/boatmanmode/internal/config"
	"github.com/handshake/boatmanmode/internal/contextpin"
	"github.com/handshake/boatmanmode/internal/coordinator"
	"github.com/handshake/boatmanmode/internal/diffverify"
	"github.com/handshake/boatmanmode/internal/executor"
	"github.com/handshake/boatmanmode/internal/github"
	"github.com/handshake/boatmanmode/internal/handoff"
	"github.com/handshake/boatmanmode/internal/linear"
	"github.com/handshake/boatmanmode/internal/planner"
	"github.com/handshake/boatmanmode/internal/preflight"
	"github.com/handshake/boatmanmode/internal/scottbott"
	"github.com/handshake/boatmanmode/internal/testrunner"
	"github.com/handshake/boatmanmode/internal/worktree"
)

// Agent orchestrates the development workflow.
type Agent struct {
	config       *config.Config
	linearClient *linear.Client
	coordinator  *coordinator.Coordinator
}

// WorkResult represents the outcome of the work command.
type WorkResult struct {
	PRCreated    bool
	PRURL        string
	Message      string
	Iterations   int
	TestsPassed  bool
	TestCoverage float64
}

// New creates a new Agent.
func New(cfg *config.Config) (*Agent, error) {
	return &Agent{
		config:       cfg,
		linearClient: linear.New(cfg.LinearKey),
		coordinator:  coordinator.New(),
	}, nil
}

// Work executes the complete workflow for a ticket.
func (a *Agent) Work(ctx context.Context, ticketID string) (*WorkResult, error) {
	totalStart := time.Now()

	// Start the coordinator
	a.coordinator.Start(ctx)
	defer a.coordinator.Stop()

	printStep(1, 9, "Fetching ticket from Linear")
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
	printStep(2, 9, "Setting up git worktree")

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

	// Initialize context pinner for multi-file coordination
	pinner := contextpin.New(wt.Path)
	pinner.SetCoordinator(a.coordinator)

	// Step 3: Planning - Run in parallel with initial file analysis
	printStep(3, 9, "Planning & analysis (parallel)")

	var plan *planner.Plan
	var planErr error
	var wg sync.WaitGroup

	// Start planner
	wg.Add(1)
	go func() {
		defer wg.Done()
		planAgent := planner.New(wt.Path)
		plan, planErr = planAgent.Analyze(ctx, ticket)
		if planErr != nil {
			fmt.Printf("   ⚠️  Planning failed: %v (continuing without plan)\n", planErr)
			plan = nil
		}
	}()

	wg.Wait()
	fmt.Println()

	// Step 4: Pre-flight validation
	printStep(4, 9, "Pre-flight validation")

	if plan != nil {
		preflightAgent := preflight.New(wt.Path)
		preflightAgent.SetCoordinator(a.coordinator)
		validation, err := preflightAgent.Validate(ctx, plan)
		if err != nil {
			fmt.Printf("   ⚠️  Validation error: %v\n", err)
		} else {
			fmt.Printf("   %s\n", (&preflight.ValidationHandoff{Result: validation}).Concise())
			if !validation.Valid {
				fmt.Println("   ⚠️  Validation failed but continuing...")
				for _, e := range validation.Errors {
					fmt.Printf("      ❌ %s\n", e.Message)
				}
			}
			for _, w := range validation.Warnings {
				fmt.Printf("      ⚠️  %s\n", w.Message)
			}
		}

		// Pin files from the plan for context consistency
		if len(plan.RelevantFiles) > 0 {
			fmt.Println("   📌 Pinning context for relevant files...")
			pinner.AnalyzeFiles(plan.RelevantFiles)
			if _, err := pinner.Pin("executor", plan.RelevantFiles, false); err != nil {
				fmt.Printf("   ⚠️  Could not pin files: %v\n", err)
			}
		}
	}
	fmt.Println()

	// Step 5: Execute the task with plan handoff
	printStep(5, 9, "Executing development task")

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

	// Step 6: Run tests (parallel with initial review)
	printStep(6, 9, "Running tests & initial review (parallel)")

	var testResult *testrunner.TestResult
	var reviewResult *scottbott.ReviewResult
	var initialDiff string

	// Get diff for review
	initialDiff, err = exec.GetDiff()
	if err != nil {
		return nil, fmt.Errorf("failed to get diff: %w", err)
	}

	wg.Add(2)

	// Run tests in parallel
	go func() {
		defer wg.Done()
		testAgent := testrunner.New(wt.Path)
		testAgent.SetCoordinator(a.coordinator)
		testResult, _ = testAgent.RunForFiles(ctx, result.FilesChanged)
	}()

	// Run initial review in parallel
	go func() {
		defer wg.Done()
		reviewHandoff := handoff.NewReviewHandoff(ticket, initialDiff, result.FilesChanged)
		reviewer := scottbott.NewWithWorkDir(wt.Path, 1)
		reviewResult, _ = reviewer.Review(ctx, reviewHandoff.Concise(), initialDiff)
	}()

	wg.Wait()

	// Display test results
	if testResult != nil {
		fmt.Printf("   🧪 Tests: %s\n", (&testrunner.TestResultHandoff{Result: testResult}).Concise())
	}

	// Display review results
	if reviewResult != nil {
		fmt.Println(reviewResult.FormatReview())
	}
	fmt.Println()

	// Step 7: Review & refactor loop with diff verification
	printStep(7, 9, "Review & refactor loop")

	var iterations int
	var previousDiff = initialDiff

	for iterations < a.config.MaxIterations {
		iterations++
		fmt.Printf("\n   🔄 Iteration %d of %d\n", iterations, a.config.MaxIterations)
		fmt.Println("   ─────────────────────────────")

		// If we already have a review from iteration 1, use it
		if iterations == 1 && reviewResult != nil {
			// Already have review from parallel execution
		} else {
			// Get fresh diff and review
			diff, err := exec.GetDiff()
			if err != nil {
				return nil, fmt.Errorf("failed to get diff: %w", err)
			}
			fmt.Printf("   📏 Diff size: %d lines\n", strings.Count(diff, "\n"))

			reviewHandoff := handoff.NewReviewHandoff(ticket, diff, result.FilesChanged)
			reviewer := scottbott.NewWithWorkDir(wt.Path, iterations)
			reviewResult, err = reviewer.Review(ctx, reviewHandoff.ForTokenBudget(handoff.DefaultBudget.Context), diff)
			if err != nil {
				return nil, fmt.Errorf("review failed: %w", err)
			}
			fmt.Println(reviewResult.FormatReview())
			previousDiff = diff
		}

		if reviewResult.Passed {
			fmt.Println("   ✅ Review passed!")

			// Run final tests to confirm
			if testResult == nil || !testResult.Passed {
				testAgent := testrunner.New(wt.Path)
				testResult, _ = testAgent.RunForFiles(ctx, result.FilesChanged)
				if testResult != nil && !testResult.Passed {
					fmt.Printf("   ⚠️  Tests failed: %s\n", (&testrunner.TestResultHandoff{Result: testResult}).Concise())
					// Continue to refactor to fix tests
					reviewResult.Passed = false
					reviewResult.Issues = append(reviewResult.Issues, scottbott.Issue{
						Severity:    "major",
						Description: fmt.Sprintf("Tests failed: %d failures", testResult.FailedTests),
					})
				}
			}

			if reviewResult.Passed {
				break
			}
		}

		if iterations >= a.config.MaxIterations {
			fmt.Println("   ⚠️  Maximum iterations reached without passing review")
			break
		}

		// Refactor based on feedback
		fmt.Printf("   🔧 Refactoring (attempt %d)...\n", iterations)

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

		// Verify the diff addresses the issues
		if len(reviewResult.Issues) > 0 {
			newDiff, _ := exec.GetDiff()
			verifier := diffverify.New(wt.Path)
			verifier.SetCoordinator(a.coordinator)
			verification, _ := verifier.Verify(ctx, reviewResult.Issues, previousDiff, newDiff)
			if verification != nil {
				fmt.Printf("   🔍 Verification: %s\n", (&diffverify.VerificationHandoff{Result: verification}).Concise())
				if len(verification.UnaddressedIssues) > 0 {
					fmt.Printf("   ⚠️  %d issues may not be addressed\n", len(verification.UnaddressedIssues))
				}
			}
		}

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

	// Release context pins
	pinner.Unpin("executor")

	// Only proceed if review passed
	if !reviewResult.Passed {
		return &WorkResult{
			PRCreated:  false,
			Message:    "Review did not pass after max iterations",
			Iterations: iterations,
		}, nil
	}

	// Step 8: Commit
	printStep(8, 9, "Committing and pushing")

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

	// Step 9: Create PR
	printStep(9, 9, "Creating pull request")

	prBody := fmt.Sprintf(`## %s

### Ticket
[%s](https://linear.app/issue/%s)

### Description
%s

### Changes
%s

### Quality
- Review iterations: %d
- Tests: %s
- Coverage: %.1f%%

---
*Automated by BoatmanMode 🚣*
`,
		ticket.Title,
		ticket.Identifier,
		ticket.Identifier,
		ticket.Description,
		reviewResult.Summary,
		iterations,
		formatTestStatus(testResult),
		getTestCoverage(testResult),
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
	fmt.Printf("   🧪 Tests:      %s\n", formatTestStatus(testResult))
	fmt.Printf("   ⏱️  Total time: %s\n", totalElapsed.Round(time.Second))
	fmt.Printf("   🔗 PR:         %s\n", prResult.URL)
	fmt.Println("═══════════════════════════════════════════════════════")

	return &WorkResult{
		PRCreated:    true,
		PRURL:        prResult.URL,
		Message:      "Successfully created PR",
		Iterations:   iterations,
		TestsPassed:  testResult == nil || testResult.Passed,
		TestCoverage: getTestCoverage(testResult),
	}, nil
}

// formatTestStatus formats test result for display.
func formatTestStatus(result *testrunner.TestResult) string {
	if result == nil {
		return "N/A"
	}
	if result.Passed {
		return fmt.Sprintf("✅ %d passed", result.PassedTests)
	}
	return fmt.Sprintf("❌ %d failed, %d passed", result.FailedTests, result.PassedTests)
}

// getTestCoverage extracts coverage from test result.
func getTestCoverage(result *testrunner.TestResult) float64 {
	if result == nil {
		return 0
	}
	return result.Coverage
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
