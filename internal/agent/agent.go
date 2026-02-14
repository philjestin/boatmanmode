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

	"github.com/philjestin/boatmanmode/internal/config"
	"github.com/philjestin/boatmanmode/internal/contextpin"
	"github.com/philjestin/boatmanmode/internal/coordinator"
	"github.com/philjestin/boatmanmode/internal/cost"
	"github.com/philjestin/boatmanmode/internal/diffverify"
	"github.com/philjestin/boatmanmode/internal/events"
	"github.com/philjestin/boatmanmode/internal/executor"
	"github.com/philjestin/boatmanmode/internal/github"
	"github.com/philjestin/boatmanmode/internal/handoff"
	"github.com/philjestin/boatmanmode/internal/linear"
	"github.com/philjestin/boatmanmode/internal/planner"
	"github.com/philjestin/boatmanmode/internal/preflight"
	"github.com/philjestin/boatmanmode/internal/scottbott"
	"github.com/philjestin/boatmanmode/internal/task"
	"github.com/philjestin/boatmanmode/internal/testrunner"
	"github.com/philjestin/boatmanmode/internal/worktree"
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

// workContext holds state shared between workflow steps.
type workContext struct {
	task         task.Task
	worktree     *worktree.Worktree
	branchName   string
	pinner       *contextpin.ContextPinner
	plan         *planner.Plan
	exec         *executor.Executor
	execResult   *executor.ExecutionResult
	testResult   *testrunner.TestResult
	reviewResult *scottbott.ReviewResult
	iterations   int
	startTime    time.Time
	costTracker  *cost.Tracker
}

// New creates a new Agent.
func New(cfg *config.Config) (*Agent, error) {
	return &Agent{
		config:       cfg,
		linearClient: linear.New(cfg.LinearKey),
		coordinator:  coordinator.New(),
	}, nil
}

// Work executes the complete workflow for a task.
// Orchestrates 9 steps: prepare → worktree → plan → validate → execute → test → review → commit → PR
func (a *Agent) Work(ctx context.Context, t task.Task) (*WorkResult, error) {
	wc := &workContext{
		task:        t,
		startTime:   time.Now(),
		costTracker: cost.NewTracker(),
	}

	// Start the coordinator
	a.coordinator.Start(ctx)
	defer a.coordinator.Stop()

	// Step 1: Prepare task (already received as parameter)
	if err := a.stepPrepareTask(ctx, wc); err != nil {
		return nil, err
	}

	// Step 2: Setup worktree
	if err := a.stepSetupWorktree(ctx, wc); err != nil {
		return nil, err
	}

	// Step 3: Planning
	if err := a.stepPlanning(ctx, wc); err != nil {
		return nil, err
	}

	// Step 4: Pre-flight validation
	if err := a.stepPreflightValidation(ctx, wc); err != nil {
		return nil, err
	}

	// Step 5: Execute development task
	if err := a.stepExecute(ctx, wc); err != nil {
		return nil, err
	}

	// Step 6: Run tests and initial review (parallel)
	if err := a.stepTestAndReview(ctx, wc); err != nil {
		return nil, err
	}

	// Step 7: Review & refactor loop
	if err := a.stepRefactorLoop(ctx, wc); err != nil {
		return nil, err
	}

	// Release context pins
	wc.pinner.Unpin("executor")

	// Check if review passed
	if !wc.reviewResult.Passed {
		return &WorkResult{
			PRCreated:  false,
			Message:    "Review did not pass after max iterations",
			Iterations: wc.iterations,
		}, nil
	}

	// Step 8: Commit and push
	if err := a.stepCommitAndPush(ctx, wc); err != nil {
		return nil, err
	}

	// Step 9: Create PR
	return a.stepCreatePR(ctx, wc)
}

// stepPrepareTask displays task information (Step 1).
func (a *Agent) stepPrepareTask(ctx context.Context, wc *workContext) error {
	agentID := fmt.Sprintf("prepare-%s", wc.task.GetID())
	events.AgentStarted(agentID, "Preparing Task", fmt.Sprintf("Preparing task %s", wc.task.GetID()))

	printStep(1, 9, "Preparing task")
	fmt.Printf("   🎫 Task ID: %s\n", wc.task.GetID())

	metadata := wc.task.GetMetadata()
	fmt.Printf("   📋 Source: %s\n", metadata.Source)
	fmt.Printf("   📝 Title: %s\n", wc.task.GetTitle())

	labels := wc.task.GetLabels()
	if len(labels) > 0 {
		fmt.Printf("   🏷️  Labels: %s\n", strings.Join(labels, ", "))
	}

	fmt.Println()
	fmt.Println("   📝 Description:")
	printIndented(truncate(wc.task.GetDescription(), 800), "      ")
	fmt.Println()

	events.AgentCompleted(agentID, "Preparing Task", "success")
	return nil
}

// stepSetupWorktree creates a git worktree for the task (Step 2).
func (a *Agent) stepSetupWorktree(ctx context.Context, wc *workContext) error {
	agentID := fmt.Sprintf("worktree-%s", wc.task.GetID())
	events.AgentStarted(agentID, "Setup Worktree", "Creating isolated git worktree")

	printStep(2, 9, "Setting up git worktree")

	repoPath, err := os.Getwd()
	if err != nil {
		events.AgentCompleted(agentID, "Setup Worktree", "failed")
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	fmt.Printf("   📂 Repo: %s\n", repoPath)

	wtManager, err := worktree.New(repoPath)
	if err != nil {
		events.AgentCompleted(agentID, "Setup Worktree", "failed")
		return fmt.Errorf("failed to create worktree manager: %w", err)
	}

	branchName := wc.task.GetBranchName()
	fmt.Printf("   🌿 Branch: %s\n", branchName)

	wt, err := wtManager.Create(branchName, a.config.BaseBranch)
	if err != nil {
		events.AgentCompleted(agentID, "Setup Worktree", "failed")
		return fmt.Errorf("failed to create worktree: %w", err)
	}
	fmt.Printf("   📁 Worktree: %s\n", wt.Path)
	fmt.Println()

	wc.worktree = wt
	wc.branchName = branchName

	// Initialize context pinner for multi-file coordination
	wc.pinner = contextpin.New(wt.Path)
	wc.pinner.SetCoordinator(a.coordinator)

	events.AgentCompleted(agentID, "Setup Worktree", "success")
	return nil
}

// stepPlanning runs the planning agent to analyze the task (Step 3).
func (a *Agent) stepPlanning(ctx context.Context, wc *workContext) error {
	agentID := fmt.Sprintf("planning-%s", wc.task.GetID())
	events.AgentStarted(agentID, "Planning & Analysis", "Analyzing codebase and creating implementation plan")

	printStep(3, 9, "Planning & analysis (parallel)")

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		planAgent := planner.New(wc.worktree.Path, a.config)
		plan, usage, err := planAgent.Analyze(ctx, wc.task)
		if err != nil {
			fmt.Printf("   ⚠️  Planning failed: %v (continuing without plan)\n", err)
			events.AgentCompleted(agentID, "Planning & Analysis", "failed")
			return
		}
		wc.plan = plan
		if usage != nil {
			wc.costTracker.Add("Planning", *usage)
		}
		// Include plan summary in metadata
		planText := ""
		if plan != nil {
			planText = plan.Summary
		}
		events.AgentCompletedWithData(agentID, "Planning & Analysis", "success", map[string]any{
			"plan": planText,
		})
	}()

	wg.Wait()
	fmt.Println()

	return nil
}

// stepPreflightValidation validates the plan before execution (Step 4).
func (a *Agent) stepPreflightValidation(ctx context.Context, wc *workContext) error {
	agentID := fmt.Sprintf("preflight-%s", wc.task.GetID())
	events.AgentStarted(agentID, "Pre-flight Validation", "Validating implementation plan")

	printStep(4, 9, "Pre-flight validation")

	if wc.plan == nil {
		fmt.Println("   ⏭️  Skipping (no plan)")
		fmt.Println()
		events.AgentCompleted(agentID, "Pre-flight Validation", "success")
		return nil
	}

	preflightAgent := preflight.New(wc.worktree.Path)
	preflightAgent.SetCoordinator(a.coordinator)
	validation, err := preflightAgent.Validate(ctx, wc.plan)
	if err != nil {
		fmt.Printf("   ⚠️  Validation error: %v\n", err)
		events.AgentCompleted(agentID, "Pre-flight Validation", "failed")
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
		events.AgentCompleted(agentID, "Pre-flight Validation", "success")
	}

	// Pin files from the plan for context consistency
	if len(wc.plan.RelevantFiles) > 0 {
		fmt.Println("   📌 Pinning context for relevant files...")
		wc.pinner.AnalyzeFiles(wc.plan.RelevantFiles)
		if _, err := wc.pinner.Pin("executor", wc.plan.RelevantFiles, false); err != nil {
			fmt.Printf("   ⚠️  Could not pin files: %v\n", err)
		}
	}

	fmt.Println()
	return nil
}

// stepExecute runs the executor to implement the task (Step 5).
func (a *Agent) stepExecute(ctx context.Context, wc *workContext) error {
	agentID := fmt.Sprintf("execute-%s", wc.task.GetID())
	events.AgentStarted(agentID, "Execution", "Implementing code changes")

	printStep(5, 9, "Executing development task")

	wc.exec = executor.New(wc.worktree.Path, a.config)
	result, usage, err := wc.exec.ExecuteWithPlan(ctx, wc.task, wc.plan)
	if err != nil {
		events.AgentCompleted(agentID, "Execution", "failed")
		return fmt.Errorf("execution failed: %w", err)
	}

	if usage != nil {
		wc.costTracker.Add("Execution", *usage)
	}

	if !result.Success {
		events.AgentCompleted(agentID, "Execution", "failed")
		return fmt.Errorf("execution failed: %v", result.Error)
	}

	wc.execResult = result
	fmt.Println()

	// Stage changes
	fmt.Println("   📥 Staging changes...")
	if err := wc.exec.StageChanges(); err != nil {
		events.AgentCompleted(agentID, "Execution", "failed")
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	// Get diff for metadata
	diff, _ := wc.exec.GetDiff()
	events.AgentCompletedWithData(agentID, "Execution", "success", map[string]any{
		"diff": diff,
	})
	return nil
}

// stepTestAndReview runs tests and initial review in parallel (Step 6).
func (a *Agent) stepTestAndReview(ctx context.Context, wc *workContext) error {
	testAgentID := fmt.Sprintf("test-%s", wc.task.GetID())
	reviewAgentID := fmt.Sprintf("review-1-%s", wc.task.GetID())

	printStep(6, 9, "Running tests & initial review (parallel)")

	// Get diff for review
	initialDiff, err := wc.exec.GetDiff()
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Run tests in parallel
	go func() {
		defer wg.Done()
		events.AgentStarted(testAgentID, "Running Tests", "Running unit tests for changed files")
		testAgent := testrunner.New(wc.worktree.Path)
		testAgent.SetCoordinator(a.coordinator)
		wc.testResult, _ = testAgent.RunForFiles(ctx, wc.execResult.FilesChanged)
		if wc.testResult != nil && wc.testResult.Passed {
			events.AgentCompleted(testAgentID, "Running Tests", "success")
		} else {
			events.AgentCompleted(testAgentID, "Running Tests", "failed")
		}
	}()

	// Run initial review in parallel
	go func() {
		defer wg.Done()
		events.AgentStarted(reviewAgentID, "Code Review #1", "Reviewing code quality and best practices")
		reviewHandoff := handoff.NewReviewHandoff(wc.task, initialDiff, wc.execResult.FilesChanged)
		reviewer := scottbott.NewWithSkill(wc.worktree.Path, 1, a.config.ReviewSkill, a.config)
		reviewResult, usage, _ := reviewer.Review(ctx, reviewHandoff.Concise(), initialDiff)
		wc.reviewResult = reviewResult
		if usage != nil {
			wc.costTracker.Add("Review #1", *usage)
		}
		if reviewResult != nil && reviewResult.Passed {
			feedback := reviewResult.Summary
			if feedback == "" && len(reviewResult.Issues) > 0 {
				feedback = fmt.Sprintf("Found %d issues", len(reviewResult.Issues))
			}
			events.AgentCompletedWithData(reviewAgentID, "Code Review #1", "success", map[string]any{
				"feedback": feedback,
				"issues":   reviewResult.Issues,
			})
		} else {
			feedback := ""
			if reviewResult != nil {
				feedback = reviewResult.Summary
			}
			events.AgentCompletedWithData(reviewAgentID, "Code Review #1", "failed", map[string]any{
				"feedback": feedback,
			})
		}
	}()

	wg.Wait()

	// Display test results
	if wc.testResult != nil {
		fmt.Printf("   🧪 Tests: %s\n", (&testrunner.TestResultHandoff{Result: wc.testResult}).Concise())
	}

	// Display review results
	if wc.reviewResult != nil {
		fmt.Println(wc.reviewResult.FormatReview())
	}
	fmt.Println()

	return nil
}

// stepRefactorLoop runs the review/refactor loop until passing or max iterations (Step 7).
func (a *Agent) stepRefactorLoop(ctx context.Context, wc *workContext) error {
	printStep(7, 9, "Review & refactor loop")

	previousDiff, _ := wc.exec.GetDiff()

	for wc.iterations < a.config.MaxIterations {
		wc.iterations++
		fmt.Printf("\n   🔄 Iteration %d of %d\n", wc.iterations, a.config.MaxIterations)
		fmt.Println("   ─────────────────────────────")

		events.Progress(fmt.Sprintf("Review & refactor iteration %d of %d", wc.iterations, a.config.MaxIterations))

		// Use existing review for first iteration, get fresh review for subsequent
		if wc.iterations > 1 || wc.reviewResult == nil {
			if err := a.doReview(ctx, wc, &previousDiff); err != nil {
				return err
			}
		}

		if wc.reviewResult.Passed {
			fmt.Println("   ✅ Review passed!")

			// Run final tests to confirm
			if wc.testResult == nil || !wc.testResult.Passed {
				testAgent := testrunner.New(wc.worktree.Path)
				wc.testResult, _ = testAgent.RunForFiles(ctx, wc.execResult.FilesChanged)
				if wc.testResult != nil && !wc.testResult.Passed {
					fmt.Printf("   ⚠️  Tests failed: %s\n", (&testrunner.TestResultHandoff{Result: wc.testResult}).Concise())
					wc.reviewResult.Passed = false
					wc.reviewResult.Issues = append(wc.reviewResult.Issues, scottbott.Issue{
						Severity:    "major",
						Description: fmt.Sprintf("Tests failed: %d failures", wc.testResult.FailedTests),
					})
				}
			}

			if wc.reviewResult.Passed {
				break
			}
		}

		if wc.iterations >= a.config.MaxIterations {
			fmt.Println("   ⚠️  Maximum iterations reached without passing review")
			break
		}

		// Refactor based on feedback
		if err := a.doRefactor(ctx, wc, previousDiff); err != nil {
			return err
		}
	}

	return nil
}

// doReview gets a fresh diff and runs the review.
func (a *Agent) doReview(ctx context.Context, wc *workContext, previousDiff *string) error {
	diff, err := wc.exec.GetDiff()
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}
	fmt.Printf("   📏 Diff size: %d lines\n", strings.Count(diff, "\n"))

	reviewHandoff := handoff.NewReviewHandoff(wc.task, diff, wc.execResult.FilesChanged)
	reviewer := scottbott.NewWithSkill(wc.worktree.Path, wc.iterations, a.config.ReviewSkill, a.config)
	reviewResult, usage, err := reviewer.Review(ctx, reviewHandoff.ForTokenBudget(handoff.DefaultBudget.Context), diff)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	if usage != nil {
		wc.costTracker.Add(fmt.Sprintf("Review #%d", wc.iterations), *usage)
	}

	fmt.Println(reviewResult.FormatReview())
	wc.reviewResult = reviewResult
	*previousDiff = diff

	return nil
}

// doRefactor performs refactoring based on review feedback.
func (a *Agent) doRefactor(ctx context.Context, wc *workContext, previousDiff string) error {
	refactorAgentID := fmt.Sprintf("refactor-%d-%s", wc.iterations, wc.task.GetID())
	events.AgentStarted(refactorAgentID, fmt.Sprintf("Refactoring #%d", wc.iterations), "Applying code review feedback")

	fmt.Printf("   🔧 Refactoring (attempt %d)...\n", wc.iterations)

	refactorExec := executor.NewRefactorExecutor(wc.worktree.Path, wc.iterations, a.config)
	currentCode, _ := refactorExec.GetSpecificFiles(wc.execResult.FilesChanged)

	// Load project rules for proper refactoring
	projectRules := refactorExec.LoadProjectRules()

	refactorHandoff := handoff.NewRefactorHandoff(
		wc.task,
		wc.reviewResult.GetIssueDescriptions(),
		wc.reviewResult.Guidance,
		wc.execResult.FilesChanged,
		currentCode,
		projectRules,
	)

	refactorResult, usage, err := refactorExec.RefactorWithHandoff(ctx, refactorHandoff)
	if err != nil {
		events.AgentCompleted(refactorAgentID, fmt.Sprintf("Refactoring #%d", wc.iterations), "failed")
		return fmt.Errorf("refactor failed: %w", err)
	}

	if usage != nil {
		wc.costTracker.Add(fmt.Sprintf("Refactor #%d", wc.iterations), *usage)
	}

	// Verify the diff addresses the issues
	if len(wc.reviewResult.Issues) > 0 {
		newDiff, _ := wc.exec.GetDiff()
		verifier := diffverify.New(wc.worktree.Path)
		verifier.SetCoordinator(a.coordinator)
		verification, _ := verifier.Verify(ctx, wc.reviewResult.Issues, previousDiff, newDiff)
		if verification != nil {
			fmt.Printf("   🔍 Verification: %s\n", (&diffverify.VerificationHandoff{Result: verification}).Concise())
			if len(verification.UnaddressedIssues) > 0 {
				fmt.Printf("   ⚠️  %d issues may not be addressed\n", len(verification.UnaddressedIssues))
			}
		}
	}

	wc.execResult.FilesChanged = refactorResult.FilesChanged

	if !refactorResult.Success {
		events.AgentCompleted(refactorAgentID, fmt.Sprintf("Refactoring #%d", wc.iterations), "failed")
		return fmt.Errorf("refactor failed: %v", refactorResult.Error)
	}

	// Stage new changes
	fmt.Println("   📥 Staging refactored changes...")
	if err := wc.exec.StageChanges(); err != nil {
		events.AgentCompleted(refactorAgentID, fmt.Sprintf("Refactoring #%d", wc.iterations), "failed")
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	// Get refactored diff for metadata
	refactorDiff, _ := wc.exec.GetDiff()
	events.AgentCompletedWithData(refactorAgentID, fmt.Sprintf("Refactoring #%d", wc.iterations), "success", map[string]any{
		"refactor_diff": refactorDiff,
	})
	return nil
}

// stepCommitAndPush commits and pushes changes (Step 8).
func (a *Agent) stepCommitAndPush(ctx context.Context, wc *workContext) error {
	agentID := fmt.Sprintf("commit-%s", wc.task.GetID())
	events.AgentStarted(agentID, "Commit & Push", "Committing and pushing changes to remote")

	printStep(8, 9, "Committing and pushing")

	commitMsg := fmt.Sprintf("feat(%s): %s\n\n%s",
		wc.task.GetID(),
		wc.task.GetTitle(),
		wc.reviewResult.Summary,
	)
	fmt.Println("   💾 Creating commit...")
	fmt.Printf("   📝 Message: %s\n", strings.Split(commitMsg, "\n")[0])

	if err := wc.exec.Commit(commitMsg); err != nil {
		events.AgentCompleted(agentID, "Commit & Push", "failed")
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Println("   📤 Pushing to origin...")
	if err := wc.exec.Push(wc.branchName); err != nil {
		events.AgentCompleted(agentID, "Commit & Push", "failed")
		return fmt.Errorf("failed to push: %w", err)
	}
	fmt.Println()

	events.AgentCompleted(agentID, "Commit & Push", "success")
	return nil
}

// stepCreatePR creates a pull request (Step 9).
func (a *Agent) stepCreatePR(ctx context.Context, wc *workContext) (*WorkResult, error) {
	agentID := fmt.Sprintf("pr-%s", wc.task.GetID())
	events.AgentStarted(agentID, "Create PR", "Creating pull request")

	printStep(9, 9, "Creating pull request")

	// Format PR body based on task source
	metadata := wc.task.GetMetadata()
	var prBody string

	if metadata.Source == task.SourceLinear {
		// Linear mode - include ticket link
		prBody = fmt.Sprintf(`## %s

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
			wc.task.GetTitle(),
			wc.task.GetID(),
			wc.task.GetID(),
			wc.task.GetDescription(),
			wc.reviewResult.Summary,
			wc.iterations,
			formatTestStatus(wc.testResult),
			getTestCoverage(wc.testResult),
		)
	} else {
		// Prompt/File mode - no ticket link
		taskType := "Prompt-based"
		if metadata.Source == task.SourceFile {
			taskType = "File-based"
		}

		prBody = fmt.Sprintf(`## %s

### Task
%s task (%s)

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
			wc.task.GetTitle(),
			taskType,
			wc.task.GetID(),
			truncate(wc.task.GetDescription(), 500),
			wc.reviewResult.Summary,
			wc.iterations,
			formatTestStatus(wc.testResult),
			getTestCoverage(wc.testResult),
		)
	}

	fmt.Println("   🔗 Running: gh pr create")
	prResult, err := github.CreatePRInDir(ctx, wc.worktree.Path, wc.task.GetTitle(), prBody, a.config.BaseBranch)
	if err != nil {
		events.AgentCompleted(agentID, "Create PR", "failed")
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	events.AgentCompleted(agentID, "Create PR", "success")
	a.printWorkflowSummary(wc, prResult.URL)

	return &WorkResult{
		PRCreated:    true,
		PRURL:        prResult.URL,
		Message:      "Successfully created PR",
		Iterations:   wc.iterations,
		TestsPassed:  wc.testResult == nil || wc.testResult.Passed,
		TestCoverage: getTestCoverage(wc.testResult),
	}, nil
}

// printWorkflowSummary prints the final workflow completion summary.
func (a *Agent) printWorkflowSummary(wc *workContext, prURL string) {
	totalElapsed := time.Since(wc.startTime)

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════")
	fmt.Println("✅ WORKFLOW COMPLETE")
	fmt.Println("═══════════════════════════════════════════════════════════════════════")
	fmt.Printf("   🎫 Task:       %s\n", wc.task.GetID())
	fmt.Printf("   🌿 Branch:     %s\n", wc.branchName)
	fmt.Printf("   🔄 Iterations: %d\n", wc.iterations)
	fmt.Printf("   🧪 Tests:      %s\n", formatTestStatus(wc.testResult))
	fmt.Printf("   ⏱️  Total time: %s\n", totalElapsed.Round(time.Second))
	fmt.Printf("   🔗 PR:         %s\n", prURL)

	// Display cost summary if any usage was tracked
	if wc.costTracker.HasUsage() {
		fmt.Print(wc.costTracker.Summary())
	}

	fmt.Println("═══════════════════════════════════════════════════════════════════════")
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
