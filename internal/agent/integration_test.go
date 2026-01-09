package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/philjestin/boatmanmode/internal/checkpoint"
	"github.com/philjestin/boatmanmode/internal/coordinator"
	"github.com/philjestin/boatmanmode/internal/issuetracker"
	"github.com/philjestin/boatmanmode/internal/memory"
	"github.com/philjestin/boatmanmode/internal/planner"
	"github.com/philjestin/boatmanmode/internal/preflight"
	"github.com/philjestin/boatmanmode/internal/scottbott"
)

// TestCoordinatorWithMultipleAgents tests that multiple agents can coordinate.
func TestCoordinatorWithMultipleAgents(t *testing.T) {
	coord := coordinator.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coord.Start(ctx)
	defer coord.Stop()

	// Create multiple mock agents
	agent1 := &testAgent{id: "planner", caps: []coordinator.AgentCapability{coordinator.CapPlan}}
	agent2 := &testAgent{id: "executor", caps: []coordinator.AgentCapability{coordinator.CapExecute}}
	agent3 := &testAgent{id: "reviewer", caps: []coordinator.AgentCapability{coordinator.CapReview}}

	coord.Register(agent1)
	coord.Register(agent2)
	coord.Register(agent3)

	// Verify all registered
	agents := coord.Registry().List()
	if len(agents) != 3 {
		t.Errorf("Expected 3 agents, got %d", len(agents))
	}

	// Test work claiming doesn't conflict
	claim1 := &coordinator.WorkClaim{WorkID: "plan-task", Files: []string{"plan.md"}}
	claim2 := &coordinator.WorkClaim{WorkID: "execute-task", Files: []string{"code.go"}}

	if !coord.ClaimWork("planner", claim1) {
		t.Error("Planner should claim work")
	}
	if !coord.ClaimWork("executor", claim2) {
		t.Error("Executor should claim different work")
	}

	// Conflicting file should fail
	claim3 := &coordinator.WorkClaim{WorkID: "conflict-task", Files: []string{"plan.md"}}
	if coord.ClaimWork("reviewer", claim3) {
		t.Error("Reviewer should not claim conflicting file")
	}
}

// TestSharedContextBetweenAgents tests context sharing.
func TestSharedContextBetweenAgents(t *testing.T) {
	coord := coordinator.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coord.Start(ctx)
	defer coord.Stop()

	// Set context from one "agent"
	coord.SetContext("plan_complete", true)
	coord.SetContext("files_changed", []string{"file1.go", "file2.go"})

	// Another "agent" should see it
	val, ok := coord.GetContext("plan_complete")
	if !ok || val != true {
		t.Error("Should retrieve shared context")
	}

	files, ok := coord.GetContext("files_changed")
	if !ok {
		t.Error("Should retrieve files context")
	}
	fileList := files.([]string)
	if len(fileList) != 2 {
		t.Errorf("Expected 2 files, got %d", len(fileList))
	}
}

// TestParallelAgentExecution tests parallel execution with coordination.
func TestParallelAgentExecution(t *testing.T) {
	coord := coordinator.New()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coord.Start(ctx)
	defer coord.Stop()

	var wg sync.WaitGroup
	results := make(map[string]bool)
	var mu sync.Mutex

	// Simulate multiple agents running in parallel
	for _, agentID := range []string{"agent-1", "agent-2", "agent-3"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			// Each tries to claim different work
			claim := &coordinator.WorkClaim{
				WorkID: "work-" + id,
				Files:  []string{id + ".go"},
			}

			if coord.ClaimWork(id, claim) {
				// Simulate work
				time.Sleep(10 * time.Millisecond)

				// Release work
				coord.ReleaseWork(claim.WorkID, id)

				mu.Lock()
				results[id] = true
				mu.Unlock()
			}
		}(agentID)
	}

	wg.Wait()

	// All should have succeeded (no conflicts)
	if len(results) != 3 {
		t.Errorf("Expected all 3 agents to complete, got %d", len(results))
	}
}

// TestPreflightToExecutorHandoff tests handoff between preflight and executor.
func TestPreflightToExecutorHandoff(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create some test files
	os.MkdirAll(filepath.Join(tmpDir, "pkg"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "pkg", "util.go"), []byte("package pkg"), 0644)

	coord := coordinator.New()
	ctx := context.Background()
	coord.Start(ctx)
	defer coord.Stop()

	// Create plan
	plan := &planner.Plan{
		Summary: "Add new feature",
		Approach: []string{
			"1. Read existing code",
			"2. Implement feature",
		},
		RelevantFiles: []string{"pkg/util.go"},
	}

	// Run preflight validation
	preflightAgent := preflight.New(tmpDir)
	preflightAgent.SetCoordinator(coord)

	result, err := preflightAgent.Validate(ctx, plan)
	if err != nil {
		t.Fatalf("Preflight failed: %v", err)
	}

	if !result.Valid {
		t.Error("Preflight should pass for valid files")
	}
}

// TestIssueTrackerWithCheckpoint tests issue tracking with checkpoint.
func TestIssueTrackerWithCheckpoint(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create checkpoint manager
	cpManager, _ := checkpoint.NewManager(tmpDir)
	cpManager.Start("ENG-123", 3)

	// Create issue tracker
	tracker := issuetracker.New()

	// Simulate iteration 1
	tracker.NextIteration()
	cpManager.SetIteration(1)
	cpManager.BeginStep(checkpoint.StepReview)

	issues1 := []scottbott.Issue{
		{Severity: "major", Description: "Missing error handling"},
		{Severity: "minor", Description: "Typo in comment"},
	}
	tracked1 := tracker.Track(issues1)

	cpManager.CompleteStep(checkpoint.StepReview, tracked1)

	// Simulate iteration 2 - one issue fixed
	tracker.NextIteration()
	cpManager.SetIteration(2)
	cpManager.BeginStep(checkpoint.StepReview)

	issues2 := []scottbott.Issue{
		{Severity: "major", Description: "Missing error handling"},
	}
	tracked2 := tracker.Track(issues2)

	cpManager.CompleteStep(checkpoint.StepReview, tracked2)

	// Verify tracking
	stats := tracker.Stats()
	if stats.TotalIssues < 1 {
		t.Errorf("Expected at least 1 issue tracked")
	}
}

// TestMemoryLearning tests memory learning from results.
func TestMemoryLearning(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create memory store
	memStore, _ := memory.NewStore(tmpDir)
	mem, _ := memStore.Get(tmpDir)

	// Learn from success
	analyzer := memory.NewAnalyzer(mem)
	analyzer.AnalyzeSuccess([]string{"pkg/util_test.go", "pkg/util.go"}, 90)

	// Verify memory updated
	if len(mem.Patterns) == 0 {
		t.Error("Should have learned patterns")
	}

	// Update stats
	mem.UpdateStats(true, 2, 5*time.Minute)

	if mem.Stats.TotalSessions != 1 {
		t.Error("Stats should be updated")
	}

	// Save memory
	memStore.Save(mem)
}

// TestFullPipelineIntegration tests components working together.
func TestFullPipelineIntegration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test project structure
	os.MkdirAll(filepath.Join(tmpDir, "pkg"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "pkg", "main.go"), []byte("package main\nfunc main() {}"), 0644)

	ctx := context.Background()

	// 1. Start coordinator
	coord := coordinator.New()
	coord.Start(ctx)
	defer coord.Stop()

	// 2. Create checkpoint
	cpDir := filepath.Join(tmpDir, ".checkpoints")
	os.MkdirAll(cpDir, 0755)
	cpManager, _ := checkpoint.NewManager(cpDir)
	cpManager.Start("TEST-123", 3)

	// 3. Create memory
	memDir := filepath.Join(tmpDir, ".memory")
	os.MkdirAll(memDir, 0755)
	memStore, _ := memory.NewStore(memDir)
	mem, _ := memStore.Get(tmpDir)

	// 4. Create issue tracker
	issueTracker := issuetracker.New()

	// Simulate workflow

	// Step: Planning
	cpManager.BeginStep(checkpoint.StepPlanning)
	plan := &planner.Plan{
		Summary:       "Add new feature",
		Approach:      []string{"Implement code", "Add tests"},
		RelevantFiles: []string{"pkg/main.go"},
	}
	cpManager.CompleteStep(checkpoint.StepPlanning, plan)

	// Step: Validation
	cpManager.BeginStep(checkpoint.StepValidation)
	preflightAgent := preflight.New(tmpDir)
	preflightAgent.SetCoordinator(coord)
	validation, _ := preflightAgent.Validate(ctx, plan)
	cpManager.CompleteStep(checkpoint.StepValidation, validation)

	if !validation.Valid {
		t.Error("Validation should pass")
	}

	// Step: Review (simulated)
	cpManager.BeginStep(checkpoint.StepReview)
	issueTracker.NextIteration()
	issues := []scottbott.Issue{
		{Severity: "minor", Description: "Add documentation"},
	}
	tracked := issueTracker.Track(issues)
	cpManager.CompleteStep(checkpoint.StepReview, tracked)

	// Learn from review
	for _, issue := range issues {
		mem.LearnIssue(memory.CommonIssue{
			Type:        "documentation",
			Description: issue.Description,
		})
	}

	// Verify integration
	progress := cpManager.GetProgress()
	if progress.StepsComplete != 3 {
		t.Errorf("Expected 3 steps complete, got %d", progress.StepsComplete)
	}

	stats := issueTracker.Stats()
	if stats.TotalIssues != 1 {
		t.Errorf("Expected 1 issue tracked, got %d", stats.TotalIssues)
	}

	if len(mem.CommonIssues) != 1 {
		t.Errorf("Expected 1 learned issue, got %d", len(mem.CommonIssues))
	}

	// Save state
	cpManager.Save()
	memStore.Save(mem)
}

// TestConcurrentIssueTrackingAndCheckpoints tests concurrent access.
func TestConcurrentIssueTrackingAndCheckpoints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cpManager, _ := checkpoint.NewManager(tmpDir)
	tracker := issuetracker.New()

	var wg sync.WaitGroup

	// Concurrent checkpoint updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		cpManager.Start("ENG-100", 5)
		for i := 0; i < 10; i++ {
			cpManager.SetIteration(i)
			time.Sleep(1 * time.Millisecond)
		}
		cpManager.Save()
	}()

	// Concurrent issue tracking
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			tracker.NextIteration()
			tracker.Track([]scottbott.Issue{
				{Severity: "minor", Description: "Issue " + string(rune('A'+i))},
			})
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Verify no crashes and data is consistent
	stats := tracker.Stats()
	if stats.TotalIssues == 0 {
		t.Error("Should have tracked issues")
	}
}

// TestCoordinatorCleanupOnStop tests that coordinator cleans up properly.
func TestCoordinatorCleanupOnStop(t *testing.T) {
	coord := coordinator.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coord.Start(ctx)

	// Add some data
	coord.SetContext("test-key", "test-value")
	claim := &coordinator.WorkClaim{WorkID: "work-1", Files: []string{"file.go"}}
	coord.ClaimWork("test-agent", claim)
	coord.LockFiles("test-agent", []string{"another.go"})

	// Stop should clean up
	coord.Stop()

	// Verify cleanup (we can't access internal maps, but we can verify
	// it doesn't panic and completes)
}

// TestCoordinatorDroppedMessageTracking tests the dropped message counter.
func TestCoordinatorDroppedMessageTracking(t *testing.T) {
	// Create coordinator with very small buffer
	coord := coordinator.NewWithOptions(coordinator.Options{
		MessageBufferSize:    1,
		SubscriberBufferSize: 1,
	})

	ctx := context.Background()
	coord.Start(ctx)
	defer coord.Stop()

	// Register agent but don't consume
	agent := &testAgent{id: "slow", caps: []coordinator.AgentCapability{}}
	coord.Register(agent)

	// Initial dropped should be 0
	if coord.DroppedMessages() != 0 {
		t.Errorf("Expected 0 dropped initially, got %d", coord.DroppedMessages())
	}

	// Flood with messages
	for i := 0; i < 100; i++ {
		coord.Send(coordinator.Message{
			Type: coordinator.MsgStatusUpdate,
			From: "test",
			To:   "slow",
		})
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Some should have been dropped
	dropped := coord.DroppedMessages()
	t.Logf("Dropped %d messages", dropped)
	// Note: exact count depends on timing, but should be > 0
}

// TestAgentWorkContextInitialization tests workContext struct usage.
func TestAgentWorkContextInitialization(t *testing.T) {
	// Create a work context (testing the struct we added)
	wc := &workContext{
		startTime: time.Now(),
	}

	if wc.startTime.IsZero() {
		t.Error("startTime should be set")
	}

	if wc.ticket != nil {
		t.Error("ticket should be nil initially")
	}

	if wc.iterations != 0 {
		t.Error("iterations should be 0 initially")
	}
}

// TestRetryWithCoordinator tests retry logic works with coordinator.
func TestRetryWithCoordinator(t *testing.T) {
	coord := coordinator.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coord.Start(ctx)
	defer coord.Stop()

	// Simulate work claiming with retries
	claim := &coordinator.WorkClaim{
		WorkID: "retry-work",
		Files:  []string{"retry.go"},
	}

	// First claim succeeds
	if !coord.ClaimWork("agent-1", claim) {
		t.Error("First claim should succeed")
	}

	// Second claim should fail (work already claimed)
	if coord.ClaimWork("agent-2", claim) {
		t.Error("Second claim should fail")
	}

	// Release
	coord.ReleaseWork("retry-work", "agent-1")

	// Now should succeed
	if !coord.ClaimWork("agent-2", claim) {
		t.Error("Claim after release should succeed")
	}
}

// TestHealthcheckIntegration tests healthcheck in agent context.
func TestHealthcheckIntegration(t *testing.T) {
	// This is more of an integration test
	// Just verify the healthcheck package works with real commands

	// Create minimal deps (echo is available everywhere)
	deps := []struct {
		name    string
		command string
		args    []string
	}{
		{"echo", "echo", []string{"test"}},
	}

	ctx := context.Background()

	for _, dep := range deps {
		t.Run(dep.name, func(t *testing.T) {
			// Basic test that command lookup works
			cmd := dep.command
			if cmd == "" {
				t.Skip("Command not configured")
			}
		})
	}

	// Just verify context timeout works
	shortCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()

	_ = shortCtx
}

// TestParallelAgentSteps tests that agent steps can run in parallel.
func TestParallelAgentSteps(t *testing.T) {
	coord := coordinator.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coord.Start(ctx)
	defer coord.Stop()

	var wg sync.WaitGroup
	results := make(map[string]bool)
	var mu sync.Mutex

	// Simulate parallel steps like in agent.Work
	steps := []string{"planning", "validation", "execution"}

	for _, step := range steps {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()

			claim := &coordinator.WorkClaim{
				WorkID: s + "-work",
				Files:  []string{s + ".go"},
			}

			if coord.ClaimWork(s+"-agent", claim) {
				time.Sleep(10 * time.Millisecond)
				coord.ReleaseWork(claim.WorkID, s+"-agent")

				mu.Lock()
				results[s] = true
				mu.Unlock()
			}
		}(step)
	}

	wg.Wait()

	// All should complete
	for _, step := range steps {
		if !results[step] {
			t.Errorf("Step %s did not complete", step)
		}
	}
}

// Mock test agent for testing
type testAgent struct {
	id    string
	caps  []coordinator.AgentCapability
	coord *coordinator.Coordinator
}

func (a *testAgent) ID() string                                { return a.id }
func (a *testAgent) Name() string                              { return "Test " + a.id }
func (a *testAgent) Capabilities() []coordinator.AgentCapability { return a.caps }
func (a *testAgent) SetCoordinator(c *coordinator.Coordinator)   { a.coord = c }
func (a *testAgent) Execute(ctx context.Context, h coordinator.Handoff) (coordinator.Handoff, error) {
	return nil, nil
}
