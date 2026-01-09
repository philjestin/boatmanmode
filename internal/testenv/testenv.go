// Package testenv provides an end-to-end test environment for boatman.
// It sets up mock servers and fixtures to enable full workflow testing
// without hitting real APIs.
package testenv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Environment represents a complete test environment for e2e testing.
type Environment struct {
	t *testing.T

	// Paths
	RootDir     string // Temporary root directory
	RepoDir     string // Git repository directory
	WorktreeDir string // Worktree directory
	BinDir      string // Mock binaries directory

	// Mock servers
	LinearServer *httptest.Server
	linearMux    *http.ServeMux

	// Mock responses
	linearResponses map[string]interface{}
	claudeResponses []string
	claudeIndex     int
	mu              sync.Mutex

	// Recorded interactions
	ClaudePrompts []string
	LinearQueries []string

	// Cleanup functions
	cleanup []func()
}

// New creates a new test environment.
func New(t *testing.T) *Environment {
	t.Helper()

	rootDir, err := os.MkdirTemp("", "boatman-e2e-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	env := &Environment{
		t:               t,
		RootDir:         rootDir,
		RepoDir:         filepath.Join(rootDir, "repo"),
		WorktreeDir:     filepath.Join(rootDir, "worktrees"),
		BinDir:          filepath.Join(rootDir, "bin"),
		linearResponses: make(map[string]interface{}),
		claudeResponses: []string{},
		ClaudePrompts:   []string{},
		LinearQueries:   []string{},
	}

	env.cleanup = append(env.cleanup, func() {
		os.RemoveAll(rootDir)
	})

	// Create directories
	for _, dir := range []string{env.RepoDir, env.WorktreeDir, env.BinDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	return env
}

// Setup initializes all components of the test environment.
func (e *Environment) Setup() *Environment {
	e.setupGitRepo()
	e.setupLinearMock()
	e.setupClaudeMock()
	e.setupGitHubMock()
	return e
}

// Cleanup tears down the test environment.
func (e *Environment) Cleanup() {
	for i := len(e.cleanup) - 1; i >= 0; i-- {
		e.cleanup[i]()
	}
}

// setupGitRepo creates a git repository with test files.
func (e *Environment) setupGitRepo() {
	e.t.Helper()

	// Initialize git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = e.RepoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			e.t.Fatalf("Failed to run %v: %v\n%s", args, err, out)
		}
	}

	// Create initial project structure
	files := map[string]string{
		"go.mod": `module example.com/testproject

go 1.21
`,
		"main.go": `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`,
		"pkg/util/util.go": `package util

// Add adds two numbers.
func Add(a, b int) int {
	return a + b
}
`,
		"pkg/util/util_test.go": `package util

import "testing"

func TestAdd(t *testing.T) {
	result := Add(2, 3)
	if result != 5 {
		t.Errorf("Add(2, 3) = %d, want 5", result)
	}
}
`,
		".cursor/rules/project.mdc": `# Project Rules
- Use conventional commits
- All functions must have tests
- Error handling is required
`,
	}

	for path, content := range files {
		fullPath := filepath.Join(e.RepoDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			e.t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			e.t.Fatalf("Failed to write %s: %v", path, err)
		}
	}

	// Initial commit
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = e.RepoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git add failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = e.RepoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

// setupLinearMock creates a mock Linear GraphQL server.
func (e *Environment) setupLinearMock() {
	e.t.Helper()

	e.linearMux = http.NewServeMux()
	e.linearMux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		e.mu.Lock()
		e.LinearQueries = append(e.LinearQueries, req.Query)
		e.mu.Unlock()

		// Default ticket response
		response := map[string]interface{}{
			"data": map[string]interface{}{
				"issue": map[string]interface{}{
					"id":          "issue-123",
					"identifier":  "ENG-123",
					"title":       "Add multiply function to util package",
					"description": "We need a Multiply function in the util package that multiplies two integers.",
					"branchName":  "eng-123-add-multiply",
					"priority":    1,
					"state": map[string]interface{}{
						"name": "In Progress",
					},
					"labels": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{"name": "enhancement"},
							{"name": "backend"},
						},
					},
				},
			},
		}

		// Check for custom response
		if identifier, ok := req.Variables["identifier"].(string); ok {
			if custom, exists := e.linearResponses[identifier]; exists {
				response = custom.(map[string]interface{})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	e.LinearServer = httptest.NewServer(e.linearMux)
	e.cleanup = append(e.cleanup, func() {
		e.LinearServer.Close()
	})
}

// setupClaudeMock creates a mock claude CLI.
func (e *Environment) setupClaudeMock() {
	e.t.Helper()

	// Create mock claude script
	mockScript := fmt.Sprintf(`#!/bin/bash
# Mock Claude CLI for testing

# Record the prompt
PROMPT_FILE="%s/claude_prompts.log"
echo "---PROMPT---" >> "$PROMPT_FILE"
echo "$@" >> "$PROMPT_FILE"

# Read response from response file
RESPONSE_FILE="%s/claude_response.txt"
if [ -f "$RESPONSE_FILE" ]; then
    cat "$RESPONSE_FILE"
else
    echo '{"type":"result","message":{"content":[{"type":"text","text":"Mock response: I will implement the requested changes."}]}}'
fi
`, e.RootDir, e.RootDir)

	mockPath := filepath.Join(e.BinDir, "claude")
	if err := os.WriteFile(mockPath, []byte(mockScript), 0755); err != nil {
		e.t.Fatalf("Failed to write mock claude: %v", err)
	}
}

// setupGitHubMock creates a mock gh CLI.
func (e *Environment) setupGitHubMock() {
	e.t.Helper()

	mockScript := `#!/bin/bash
# Mock GitHub CLI for testing

if [[ "$1" == "pr" && "$2" == "create" ]]; then
    echo "https://github.com/example/repo/pull/42"
    exit 0
fi

if [[ "$1" == "auth" && "$2" == "status" ]]; then
    echo "Logged in to github.com"
    exit 0
fi

echo "Mock gh: $@"
`

	mockPath := filepath.Join(e.BinDir, "gh")
	if err := os.WriteFile(mockPath, []byte(mockScript), 0755); err != nil {
		e.t.Fatalf("Failed to write mock gh: %v", err)
	}
}

// SetLinearTicket sets a custom Linear ticket response.
func (e *Environment) SetLinearTicket(identifier string, ticket TicketFixture) {
	e.mu.Lock()
	defer e.mu.Unlock()

	labels := make([]map[string]interface{}, len(ticket.Labels))
	for i, l := range ticket.Labels {
		labels[i] = map[string]interface{}{"name": l}
	}

	e.linearResponses[identifier] = map[string]interface{}{
		"data": map[string]interface{}{
			"issue": map[string]interface{}{
				"id":          ticket.ID,
				"identifier":  identifier,
				"title":       ticket.Title,
				"description": ticket.Description,
				"branchName":  ticket.BranchName,
				"priority":    ticket.Priority,
				"state":       map[string]interface{}{"name": ticket.State},
				"labels":      map[string]interface{}{"nodes": labels},
			},
		},
	}
}

// SetClaudeResponse sets the next Claude response.
func (e *Environment) SetClaudeResponse(response string) {
	responsePath := filepath.Join(e.RootDir, "claude_response.txt")
	
	// Format as stream-json
	jsonResponse := fmt.Sprintf(`{"type":"result","message":{"content":[{"type":"text","text":%s}]}}`,
		jsonEscape(response))
	
	if err := os.WriteFile(responsePath, []byte(jsonResponse), 0644); err != nil {
		e.t.Fatalf("Failed to write claude response: %v", err)
	}
}

// SetClaudeResponseSequence sets a sequence of Claude responses.
func (e *Environment) SetClaudeResponseSequence(responses []string) {
	e.mu.Lock()
	e.claudeResponses = responses
	e.claudeIndex = 0
	e.mu.Unlock()
}

// GetEnv returns environment variables for running commands with mocks.
func (e *Environment) GetEnv() []string {
	env := os.Environ()
	
	// Prepend mock bin dir to PATH
	for i, v := range env {
		if strings.HasPrefix(v, "PATH=") {
			env[i] = fmt.Sprintf("PATH=%s:%s", e.BinDir, strings.TrimPrefix(v, "PATH="))
			break
		}
	}

	// Set Linear API URL to our mock
	env = append(env, fmt.Sprintf("LINEAR_API_URL=%s/graphql", e.LinearServer.URL))
	env = append(env, "LINEAR_API_KEY=test-api-key")
	env = append(env, "BOATMAN_DEBUG=1")

	return env
}

// RunInRepo runs a command in the repo directory with mock environment.
func (e *Environment) RunInRepo(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = e.RepoDir
	cmd.Env = e.GetEnv()

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// AddFile adds a file to the test repository.
func (e *Environment) AddFile(path, content string) {
	fullPath := filepath.Join(e.RepoDir, path)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		e.t.Fatalf("Failed to create dir: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		e.t.Fatalf("Failed to write file: %v", err)
	}
}

// CommitAll commits all changes in the repo.
func (e *Environment) CommitAll(message string) {
	cmds := [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", message},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = e.RepoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			e.t.Fatalf("git command failed: %v\n%s", err, out)
		}
	}
}

// GetFile reads a file from the repo.
func (e *Environment) GetFile(path string) string {
	content, err := os.ReadFile(filepath.Join(e.RepoDir, path))
	if err != nil {
		e.t.Fatalf("Failed to read file: %v", err)
	}
	return string(content)
}

// FileExists checks if a file exists in the repo.
func (e *Environment) FileExists(path string) bool {
	_, err := os.Stat(filepath.Join(e.RepoDir, path))
	return err == nil
}

// TicketFixture represents a test ticket.
type TicketFixture struct {
	ID          string
	Title       string
	Description string
	BranchName  string
	State       string
	Priority    int
	Labels      []string
}

// jsonEscape escapes a string for JSON.
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
