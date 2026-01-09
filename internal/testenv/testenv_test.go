package testenv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnvironmentSetup(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	// Verify directories exist
	for _, dir := range []string{env.RootDir, env.RepoDir, env.WorktreeDir, env.BinDir} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory should exist: %s", dir)
		}
	}

	// Verify git repo is initialized
	gitDir := filepath.Join(env.RepoDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error(".git directory should exist")
	}

	// Verify initial files exist
	files := []string{"go.mod", "main.go", "pkg/util/util.go", "pkg/util/util_test.go"}
	for _, f := range files {
		if !env.FileExists(f) {
			t.Errorf("File should exist: %s", f)
		}
	}
}

func TestLinearMockServer(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	// Verify Linear server is running
	if env.LinearServer == nil {
		t.Fatal("Linear server should be running")
	}

	// Test that we can make requests
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use curl to test (simpler than importing http client here)
	output, err := env.RunInRepo(ctx, "curl", "-s", "-X", "POST",
		"-H", "Content-Type: application/json",
		"-d", `{"query":"test","variables":{"identifier":"ENG-123"}}`,
		env.LinearServer.URL+"/graphql")

	if err != nil {
		t.Fatalf("Failed to query Linear mock: %v", err)
	}

	if !strings.Contains(output, "ENG-123") {
		t.Errorf("Response should contain ticket identifier, got: %s", output)
	}
}

func TestCustomLinearTicket(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	// Set custom ticket
	env.SetLinearTicket("CUSTOM-1", TicketFixture{
		ID:          "custom-id",
		Title:       "Custom ticket title",
		Description: "Custom description",
		BranchName:  "custom-branch",
		State:       "Todo",
		Priority:    2,
		Labels:      []string{"custom"},
	})

	// Query for custom ticket
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := env.RunInRepo(ctx, "curl", "-s", "-X", "POST",
		"-H", "Content-Type: application/json",
		"-d", `{"query":"test","variables":{"identifier":"CUSTOM-1"}}`,
		env.LinearServer.URL+"/graphql")

	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}

	if !strings.Contains(output, "Custom ticket title") {
		t.Errorf("Response should contain custom title, got: %s", output)
	}
}

func TestMockClaudeCLI(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	// Set a response
	env.SetClaudeResponse("Test response from mock Claude")

	// Verify mock script exists
	mockPath := filepath.Join(env.BinDir, "claude")
	if _, err := os.Stat(mockPath); os.IsNotExist(err) {
		t.Fatal("Mock claude should exist")
	}

	// Verify it's executable
	info, _ := os.Stat(mockPath)
	if info.Mode()&0111 == 0 {
		t.Error("Mock claude should be executable")
	}
}

func TestMockGitHubCLI(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	// Verify mock script exists
	mockPath := filepath.Join(env.BinDir, "gh")
	if _, err := os.Stat(mockPath); os.IsNotExist(err) {
		t.Fatal("Mock gh should exist")
	}

	// Test PR creation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := env.RunInRepo(ctx, filepath.Join(env.BinDir, "gh"),
		"pr", "create", "--title", "Test", "--body", "Test body")

	if err != nil {
		t.Fatalf("gh pr create failed: %v", err)
	}

	if !strings.Contains(output, "github.com") {
		t.Errorf("Should return PR URL, got: %s", output)
	}
}

func TestAddAndReadFile(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	// Add a new file
	env.AddFile("new/path/file.go", "package new\n\nfunc New() {}\n")

	// Verify it exists
	if !env.FileExists("new/path/file.go") {
		t.Error("New file should exist")
	}

	// Read it back
	content := env.GetFile("new/path/file.go")
	if !strings.Contains(content, "func New()") {
		t.Errorf("Content should contain function, got: %s", content)
	}
}

func TestCommitAll(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	// Add a file and commit
	env.AddFile("test.txt", "test content")
	env.CommitAll("Add test file")

	// Verify commit was made
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := env.RunInRepo(ctx, "git", "log", "--oneline", "-1")
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}

	if !strings.Contains(output, "Add test file") {
		t.Errorf("Commit message should be in log, got: %s", output)
	}
}

func TestGetEnv(t *testing.T) {
	env := New(t).Setup()
	defer env.Cleanup()

	envVars := env.GetEnv()

	// Check for required env vars
	hasLinearURL := false
	hasLinearKey := false
	hasDebug := false
	hasPath := false

	for _, v := range envVars {
		if strings.HasPrefix(v, "LINEAR_API_URL=") {
			hasLinearURL = true
			if !strings.Contains(v, env.LinearServer.URL) {
				t.Errorf("LINEAR_API_URL should point to mock server")
			}
		}
		if strings.HasPrefix(v, "LINEAR_API_KEY=") {
			hasLinearKey = true
		}
		if v == "BOATMAN_DEBUG=1" {
			hasDebug = true
		}
		if strings.HasPrefix(v, "PATH=") && strings.Contains(v, env.BinDir) {
			hasPath = true
		}
	}

	if !hasLinearURL {
		t.Error("Should have LINEAR_API_URL")
	}
	if !hasLinearKey {
		t.Error("Should have LINEAR_API_KEY")
	}
	if !hasDebug {
		t.Error("Should have BOATMAN_DEBUG")
	}
	if !hasPath {
		t.Error("PATH should include mock bin dir")
	}
}

func TestDefaultFixtures(t *testing.T) {
	ticket := DefaultTicket()

	if ticket.ID == "" {
		t.Error("Default ticket should have ID")
	}
	if ticket.Title == "" {
		t.Error("Default ticket should have title")
	}
	if len(ticket.Labels) == 0 {
		t.Error("Default ticket should have labels")
	}
}

func TestScenarioFixtures(t *testing.T) {
	happyPath := ScenarioHappyPath()
	if len(happyPath) < 3 {
		t.Error("Happy path should have at least 3 responses")
	}

	needsRefactor := ScenarioNeedsRefactor()
	if len(needsRefactor) < 4 {
		t.Error("Needs refactor should have at least 4 responses")
	}

	failsReview := ScenarioFailsReview()
	if len(failsReview) < 5 {
		t.Error("Fails review should have multiple retry responses")
	}
}

func TestCleanup(t *testing.T) {
	env := New(t).Setup()
	rootDir := env.RootDir

	// Verify directory exists before cleanup
	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		t.Fatal("Root dir should exist before cleanup")
	}

	env.Cleanup()

	// Verify directory is removed after cleanup
	if _, err := os.Stat(rootDir); !os.IsNotExist(err) {
		t.Error("Root dir should be removed after cleanup")
	}
}
