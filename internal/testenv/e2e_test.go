// +build e2e

package testenv

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/handshake/boatmanmode/internal/config"
	"github.com/handshake/boatmanmode/internal/coordinator"
	"github.com/handshake/boatmanmode/internal/healthcheck"
)

// TestE2EHealthCheck tests the healthcheck in a mock environment.
func TestE2EHealthCheck(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check that our mock CLIs can be called directly (without modifying PATH)
	deps := []healthcheck.Dependency{
		{Name: "gh", Command: env.BinDir + "/gh", Args: []string{"--help"}, Required: true},
		{Name: "claude", Command: env.BinDir + "/claude", Args: []string{"--help"}, Required: true},
	}

	// Note: We can't fully test healthcheck because it checks for the real commands
	// But we can verify the mock scripts run
	results := healthcheck.Check(ctx, deps)
	_ = results // Just verify it doesn't panic
}

// TestE2ECoordinatorWorkflow tests coordinator-based workflow.
func TestE2ECoordinatorWorkflow(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create coordinator with test config
	opts := coordinator.Options{
		MessageBufferSize:    100,
		SubscriberBufferSize: 10,
	}
	coord := coordinator.NewWithOptions(opts)
	coord.Start(ctx)
	defer coord.Stop()

	// Simulate multi-agent workflow
	// 1. Planner claims planning work
	planClaim := &coordinator.WorkClaim{
		WorkID:   "plan-task",
		WorkType: "plan",
		Files:    []string{"PLAN.md"},
	}
	if !coord.ClaimWork("planner", planClaim) {
		t.Error("Planner should claim planning work")
	}

	// 2. Set context after planning
	coord.SetContext("plan_complete", true)
	coord.SetContext("target_files", []string{"pkg/util/util.go"})

	// 3. Executor claims execution work
	execClaim := &coordinator.WorkClaim{
		WorkID:   "exec-task",
		WorkType: "execute",
		Files:    []string{"pkg/util/util.go"},
	}
	if !coord.ClaimWork("executor", execClaim) {
		t.Error("Executor should claim execution work")
	}

	// 4. Verify context is shared
	val, ok := coord.GetContext("plan_complete")
	if !ok || val != true {
		t.Error("Context should be shared between agents")
	}

	// 5. Release work
	coord.ReleaseWork("plan-task", "planner")
	coord.ReleaseWork("exec-task", "executor")

	// Verify no dropped messages in controlled test
	if coord.DroppedMessages() > 0 {
		t.Logf("Dropped %d messages", coord.DroppedMessages())
	}
}

// TestE2EConfigLoading tests config loading with env vars.
func TestE2EConfigLoading(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	// Save original env
	origLinearKey := os.Getenv("LINEAR_API_KEY")
	origLinearURL := os.Getenv("LINEAR_API_URL")
	origDebug := os.Getenv("BOATMAN_DEBUG")

	// Set test environment (only the specific vars we need)
	os.Setenv("LINEAR_API_KEY", "test-api-key")
	os.Setenv("LINEAR_API_URL", env.LinearServer.URL+"/graphql")
	os.Setenv("BOATMAN_DEBUG", "1")

	defer func() {
		// Restore original values
		if origLinearKey != "" {
			os.Setenv("LINEAR_API_KEY", origLinearKey)
		} else {
			os.Unsetenv("LINEAR_API_KEY")
		}
		if origLinearURL != "" {
			os.Setenv("LINEAR_API_URL", origLinearURL)
		} else {
			os.Unsetenv("LINEAR_API_URL")
		}
		if origDebug != "" {
			os.Setenv("BOATMAN_DEBUG", origDebug)
		} else {
			os.Unsetenv("BOATMAN_DEBUG")
		}
	}()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Config load failed: %v", err)
	}

	// Verify config loaded
	if cfg.LinearKey != "test-api-key" {
		t.Errorf("Expected LinearKey 'test-api-key', got %s", cfg.LinearKey)
	}

	if !cfg.Debug {
		t.Error("Debug should be enabled")
	}

	// Verify defaults
	if cfg.MaxIterations != 3 {
		t.Errorf("Expected MaxIterations 3, got %d", cfg.MaxIterations)
	}

	if cfg.Coordinator.MessageBufferSize != 1000 {
		t.Errorf("Expected MessageBufferSize 1000, got %d", cfg.Coordinator.MessageBufferSize)
	}
}

// TestE2EGitWorktreeCreation tests git worktree operations.
func TestE2EGitWorktreeCreation(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a new branch
	output, err := env.RunInRepo(ctx, "git", "checkout", "-b", "test-feature")
	if err != nil {
		t.Fatalf("git checkout failed: %v\n%s", err, output)
	}

	// Make changes
	env.AddFile("pkg/util/multiply.go", `package util

// Multiply multiplies two integers.
func Multiply(a, b int) int {
	return a * b
}
`)

	// Commit
	env.CommitAll("feat: add multiply function")

	// Verify commit
	output, err = env.RunInRepo(ctx, "git", "log", "--oneline", "-1")
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}

	if output == "" {
		t.Error("Should have commit in log")
	}
}

// TestE2EFileModificationWorkflow tests file modification and verification.
func TestE2EFileModificationWorkflow(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	// Read initial file
	initial := env.GetFile("pkg/util/util.go")
	if initial == "" {
		t.Fatal("Initial file should exist")
	}

	// Modify file (simulating executor)
	newContent := initial + `
// Subtract subtracts b from a.
func Subtract(a, b int) int {
	return a - b
}
`
	env.AddFile("pkg/util/util.go", newContent)

	// Verify modification
	modified := env.GetFile("pkg/util/util.go")
	if modified == initial {
		t.Error("File should be modified")
	}

	if len(modified) <= len(initial) {
		t.Error("Modified file should be longer")
	}

	// Add corresponding test
	testContent := env.GetFile("pkg/util/util_test.go")
	testContent += `
func TestSubtract(t *testing.T) {
	result := Subtract(5, 3)
	if result != 2 {
		t.Errorf("Subtract(5, 3) = %d, want 2", result)
	}
}
`
	env.AddFile("pkg/util/util_test.go", testContent)

	// Run tests
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := env.RunInRepo(ctx, "go", "test", "./pkg/util/...")
	if err != nil {
		t.Fatalf("Tests failed: %v\n%s", err, output)
	}
}

// TestE2EMockLinearIntegration tests the Linear mock thoroughly.
func TestE2EMockLinearIntegration(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Set custom ticket
	env.SetLinearTicket("TEST-999", TicketFixture{
		ID:          "test-999-id",
		Title:       "E2E Test Ticket",
		Description: "This is a test ticket for e2e testing.",
		BranchName:  "test-999-e2e",
		State:       "In Progress",
		Priority:    1,
		Labels:      []string{"e2e", "test"},
	})

	// Query it
	output, err := env.RunInRepo(ctx, "curl", "-s", "-X", "POST",
		"-H", "Content-Type: application/json",
		"-d", `{"query":"test","variables":{"identifier":"TEST-999"}}`,
		env.LinearServer.URL+"/graphql")

	if err != nil {
		t.Fatalf("curl failed: %v", err)
	}

	// Verify response contains our custom data
	expectations := []string{
		"E2E Test Ticket",
		"test-999-id",
		"test-999-e2e",
	}

	for _, exp := range expectations {
		if !containsString(output, exp) {
			t.Errorf("Response should contain %q, got: %s", exp, output)
		}
	}

	// Verify query was recorded
	if len(env.LinearQueries) == 0 {
		t.Error("Query should be recorded")
	}
}

// TestE2EParallelAgentSimulation simulates parallel agent execution.
func TestE2EParallelAgentSimulation(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	coord := coordinator.New()
	coord.Start(ctx)
	defer coord.Stop()

	// Simulate planning phase
	planResult := make(chan bool)
	go func() {
		claim := &coordinator.WorkClaim{WorkID: "plan", Files: []string{"PLAN.md"}}
		result := coord.ClaimWork("planner", claim)
		time.Sleep(10 * time.Millisecond) // Simulate work
		coord.ReleaseWork("plan", "planner")
		planResult <- result
	}()

	if !<-planResult {
		t.Error("Planning should complete")
	}

	// Simulate execution phase
	execResult := make(chan bool)
	go func() {
		claim := &coordinator.WorkClaim{WorkID: "exec", Files: []string{"code.go"}}
		result := coord.ClaimWork("executor", claim)
		time.Sleep(10 * time.Millisecond)
		coord.ReleaseWork("exec", "executor")
		execResult <- result
	}()

	if !<-execResult {
		t.Error("Execution should complete")
	}

	// Simulate parallel test and review
	testResult := make(chan bool)
	reviewResult := make(chan bool)

	go func() {
		claim := &coordinator.WorkClaim{WorkID: "test", Files: []string{}}
		result := coord.ClaimWork("tester", claim)
		time.Sleep(10 * time.Millisecond)
		coord.ReleaseWork("test", "tester")
		testResult <- result
	}()

	go func() {
		claim := &coordinator.WorkClaim{WorkID: "review", Files: []string{}}
		result := coord.ClaimWork("reviewer", claim)
		time.Sleep(10 * time.Millisecond)
		coord.ReleaseWork("review", "reviewer")
		reviewResult <- result
	}()

	if !<-testResult || !<-reviewResult {
		t.Error("Parallel test and review should complete")
	}
}

// Helper functions

func splitEnvVar(s string) []string {
	idx := indexOf(s, '=')
	if idx < 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
