// Package testrunner provides test execution capabilities.
// It detects the test framework and runs relevant tests.
package testrunner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/handshake/boatmanmode/internal/coordinator"
)

// TestResult contains the outcome of test execution.
type TestResult struct {
	Passed      bool
	Framework   string
	TotalTests  int
	PassedTests int
	FailedTests int
	SkippedTests int
	Coverage    float64
	Output      string
	Duration    time.Duration
	FailedNames []string
}

// Agent runs tests for the project.
type Agent struct {
	id           string
	worktreePath string
	coord        *coordinator.Coordinator
}

// New creates a new test runner agent.
func New(worktreePath string) *Agent {
	return &Agent{
		id:           "testrunner",
		worktreePath: worktreePath,
	}
}

// ID returns the agent ID.
func (a *Agent) ID() string {
	return a.id
}

// Name returns the human-readable name.
func (a *Agent) Name() string {
	return "Test Runner"
}

// Capabilities returns what this agent can do.
func (a *Agent) Capabilities() []coordinator.AgentCapability {
	return []coordinator.AgentCapability{coordinator.CapTest}
}

// SetCoordinator sets the coordinator for communication.
func (a *Agent) SetCoordinator(c *coordinator.Coordinator) {
	a.coord = c
}

// Framework represents a detected test framework.
type Framework struct {
	Name    string
	Command string
	Args    []string
	// Pattern to match test files for targeted runs
	FilePattern string
}

// DetectFramework figures out what test framework the project uses.
func (a *Agent) DetectFramework() (*Framework, error) {
	// Check for Go
	if _, err := os.Stat(filepath.Join(a.worktreePath, "go.mod")); err == nil {
		return &Framework{
			Name:        "go",
			Command:     "go",
			Args:        []string{"test", "-v", "-cover", "./..."},
			FilePattern: "*_test.go",
		}, nil
	}

	// Check for Ruby/Rails (RSpec)
	if _, err := os.Stat(filepath.Join(a.worktreePath, "Gemfile")); err == nil {
		// Check for rspec
		gemfile, _ := os.ReadFile(filepath.Join(a.worktreePath, "Gemfile"))
		if strings.Contains(string(gemfile), "rspec") {
			return &Framework{
				Name:        "rspec",
				Command:     "bundle",
				Args:        []string{"exec", "rspec", "--format", "progress"},
				FilePattern: "*_spec.rb",
			}, nil
		}
		// Check for minitest
		return &Framework{
			Name:        "minitest",
			Command:     "bundle",
			Args:        []string{"exec", "rake", "test"},
			FilePattern: "*_test.rb",
		}, nil
	}

	// Check for Node.js
	if _, err := os.Stat(filepath.Join(a.worktreePath, "package.json")); err == nil {
		pkgJSON, _ := os.ReadFile(filepath.Join(a.worktreePath, "package.json"))
		content := string(pkgJSON)

		// Check for jest
		if strings.Contains(content, "jest") {
			return &Framework{
				Name:        "jest",
				Command:     "npx",
				Args:        []string{"jest", "--coverage", "--passWithNoTests"},
				FilePattern: "*.test.{js,ts,jsx,tsx}",
			}, nil
		}

		// Check for vitest
		if strings.Contains(content, "vitest") {
			return &Framework{
				Name:        "vitest",
				Command:     "npx",
				Args:        []string{"vitest", "run", "--coverage"},
				FilePattern: "*.test.{js,ts,jsx,tsx}",
			}, nil
		}

		// Check for mocha
		if strings.Contains(content, "mocha") {
			return &Framework{
				Name:        "mocha",
				Command:     "npx",
				Args:        []string{"mocha"},
				FilePattern: "*.test.js",
			}, nil
		}

		// Default to npm test
		return &Framework{
			Name:    "npm",
			Command: "npm",
			Args:    []string{"test", "--", "--passWithNoTests"},
		}, nil
	}

	// Check for Python
	if _, err := os.Stat(filepath.Join(a.worktreePath, "pytest.ini")); err == nil {
		return &Framework{
			Name:        "pytest",
			Command:     "pytest",
			Args:        []string{"-v", "--cov"},
			FilePattern: "test_*.py",
		}, nil
	}
	if _, err := os.Stat(filepath.Join(a.worktreePath, "setup.py")); err == nil {
		return &Framework{
			Name:        "pytest",
			Command:     "pytest",
			Args:        []string{"-v"},
			FilePattern: "test_*.py",
		}, nil
	}
	if _, err := os.Stat(filepath.Join(a.worktreePath, "pyproject.toml")); err == nil {
		return &Framework{
			Name:        "pytest",
			Command:     "pytest",
			Args:        []string{"-v"},
			FilePattern: "test_*.py",
		}, nil
	}

	return nil, fmt.Errorf("no test framework detected")
}

// RunAll runs all tests in the project.
func (a *Agent) RunAll(ctx context.Context) (*TestResult, error) {
	framework, err := a.DetectFramework()
	if err != nil {
		return &TestResult{
			Passed: true, // No tests = pass
			Output: "No test framework detected",
		}, nil
	}

	return a.runTests(ctx, framework, framework.Args)
}

// RunForFiles runs tests relevant to specific changed files.
func (a *Agent) RunForFiles(ctx context.Context, changedFiles []string) (*TestResult, error) {
	framework, err := a.DetectFramework()
	if err != nil {
		return &TestResult{
			Passed: true,
			Output: "No test framework detected",
		}, nil
	}

	// Find related test files
	testFiles := a.findRelatedTests(changedFiles, framework)
	if len(testFiles) == 0 {
		// No specific tests found, run all
		return a.RunAll(ctx)
	}

	// Build targeted test command
	args := a.buildTargetedArgs(framework, testFiles)
	return a.runTests(ctx, framework, args)
}

// findRelatedTests finds test files related to changed files.
func (a *Agent) findRelatedTests(changedFiles []string, framework *Framework) []string {
	var testFiles []string
	seen := make(map[string]bool)

	for _, file := range changedFiles {
		// If it's already a test file, include it
		if a.isTestFile(file, framework) {
			if !seen[file] {
				testFiles = append(testFiles, file)
				seen[file] = true
			}
			continue
		}

		// Find corresponding test file
		testFile := a.findTestFile(file, framework)
		if testFile != "" && !seen[testFile] {
			testFiles = append(testFiles, testFile)
			seen[testFile] = true
		}
	}

	return testFiles
}

// isTestFile checks if a file is a test file.
func (a *Agent) isTestFile(file string, framework *Framework) bool {
	switch framework.Name {
	case "go":
		return strings.HasSuffix(file, "_test.go")
	case "rspec":
		return strings.HasSuffix(file, "_spec.rb")
	case "minitest":
		return strings.HasSuffix(file, "_test.rb")
	case "jest", "vitest", "mocha":
		return strings.Contains(file, ".test.") || strings.Contains(file, ".spec.")
	case "pytest":
		base := filepath.Base(file)
		return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")
	}
	return false
}

// findTestFile finds the test file for a source file.
func (a *Agent) findTestFile(file string, framework *Framework) string {
	dir := filepath.Dir(file)
	base := filepath.Base(file)
	ext := filepath.Ext(file)
	name := strings.TrimSuffix(base, ext)

	var candidates []string

	switch framework.Name {
	case "go":
		candidates = []string{
			filepath.Join(dir, name+"_test.go"),
		}
	case "rspec":
		// Ruby: app/models/user.rb -> spec/models/user_spec.rb
		specDir := strings.Replace(dir, "app/", "spec/", 1)
		candidates = []string{
			filepath.Join(specDir, name+"_spec.rb"),
			filepath.Join("spec", dir, name+"_spec.rb"),
		}
	case "jest", "vitest":
		// JavaScript: src/utils/helper.ts -> src/utils/helper.test.ts
		candidates = []string{
			filepath.Join(dir, name+".test"+ext),
			filepath.Join(dir, name+".spec"+ext),
			filepath.Join(dir, "__tests__", base),
		}
	case "pytest":
		candidates = []string{
			filepath.Join(dir, "test_"+base),
			filepath.Join("tests", "test_"+base),
		}
	}

	for _, candidate := range candidates {
		fullPath := filepath.Join(a.worktreePath, candidate)
		if _, err := os.Stat(fullPath); err == nil {
			return candidate
		}
	}

	return ""
}

// buildTargetedArgs builds command args for specific test files.
func (a *Agent) buildTargetedArgs(framework *Framework, testFiles []string) []string {
	switch framework.Name {
	case "go":
		// go test -v ./path/to/package...
		packages := make(map[string]bool)
		for _, file := range testFiles {
			dir := filepath.Dir(file)
			packages["./"+ dir] = true
		}
		args := []string{"test", "-v", "-cover"}
		for pkg := range packages {
			args = append(args, pkg)
		}
		return args

	case "rspec":
		args := []string{"exec", "rspec", "--format", "progress"}
		args = append(args, testFiles...)
		return args

	case "jest", "vitest":
		args := []string{framework.Name}
		args = append(args, testFiles...)
		return args

	case "pytest":
		args := []string{"-v"}
		args = append(args, testFiles...)
		return args
	}

	return framework.Args
}

// runTests executes tests and parses results.
func (a *Agent) runTests(ctx context.Context, framework *Framework, args []string) (*TestResult, error) {
	// Claim work if coordinated
	if a.coord != nil {
		claim := &coordinator.WorkClaim{
			WorkID:      "test-run",
			WorkType:    "test",
			Description: fmt.Sprintf("Running %s tests", framework.Name),
		}
		if !a.coord.ClaimWork(a.id, claim) {
			return nil, fmt.Errorf("could not claim test work")
		}
		defer a.coord.ReleaseWork(claim.WorkID, a.id)
	}

	start := time.Now()

	cmd := exec.CommandContext(ctx, framework.Command, args...)
	cmd.Dir = a.worktreePath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	output := stdout.String() + "\n" + stderr.String()

	result := &TestResult{
		Framework: framework.Name,
		Output:    output,
		Duration:  duration,
	}

	// Parse output based on framework
	a.parseOutput(result, output, framework)

	// If command failed but we couldn't parse failures, check exit code
	if err != nil && result.FailedTests == 0 {
		result.Passed = false
		result.FailedTests = 1
	}

	// Share result via coordinator
	if a.coord != nil {
		a.coord.SetContext("test_result", result)
	}

	return result, nil
}

// parseOutput extracts test statistics from output.
func (a *Agent) parseOutput(result *TestResult, output string, framework *Framework) {
	switch framework.Name {
	case "go":
		a.parseGoOutput(result, output)
	case "rspec":
		a.parseRspecOutput(result, output)
	case "jest", "vitest":
		a.parseJestOutput(result, output)
	case "pytest":
		a.parsePytestOutput(result, output)
	default:
		// Generic pass/fail detection
		result.Passed = !strings.Contains(strings.ToLower(output), "fail")
	}
}

// parseGoOutput parses go test output.
func (a *Agent) parseGoOutput(result *TestResult, output string) {
	result.Passed = true

	// Count passes and failures
	passRe := regexp.MustCompile(`--- PASS:`)
	failRe := regexp.MustCompile(`--- FAIL:`)
	skipRe := regexp.MustCompile(`--- SKIP:`)
	coverRe := regexp.MustCompile(`coverage: (\d+\.\d+)%`)

	result.PassedTests = len(passRe.FindAllString(output, -1))
	result.FailedTests = len(failRe.FindAllString(output, -1))
	result.SkippedTests = len(skipRe.FindAllString(output, -1))
	result.TotalTests = result.PassedTests + result.FailedTests + result.SkippedTests

	if matches := coverRe.FindStringSubmatch(output); len(matches) > 1 {
		fmt.Sscanf(matches[1], "%f", &result.Coverage)
	}

	if result.FailedTests > 0 || strings.Contains(output, "FAIL") {
		result.Passed = false
		
		// Extract failed test names
		failNameRe := regexp.MustCompile(`--- FAIL: (\S+)`)
		for _, match := range failNameRe.FindAllStringSubmatch(output, -1) {
			if len(match) > 1 {
				result.FailedNames = append(result.FailedNames, match[1])
			}
		}
	}
}

// parseRspecOutput parses rspec output.
func (a *Agent) parseRspecOutput(result *TestResult, output string) {
	// "15 examples, 0 failures, 2 pending"
	re := regexp.MustCompile(`(\d+) examples?, (\d+) failures?(?:, (\d+) pending)?`)
	if matches := re.FindStringSubmatch(output); len(matches) > 2 {
		fmt.Sscanf(matches[1], "%d", &result.TotalTests)
		fmt.Sscanf(matches[2], "%d", &result.FailedTests)
		if len(matches) > 3 && matches[3] != "" {
			fmt.Sscanf(matches[3], "%d", &result.SkippedTests)
		}
		result.PassedTests = result.TotalTests - result.FailedTests - result.SkippedTests
		result.Passed = result.FailedTests == 0
	}
}

// parseJestOutput parses jest output.
func (a *Agent) parseJestOutput(result *TestResult, output string) {
	// "Tests:       1 failed, 5 passed, 6 total"
	re := regexp.MustCompile(`Tests:\s+(?:(\d+) failed, )?(?:(\d+) skipped, )?(\d+) passed, (\d+) total`)
	if matches := re.FindStringSubmatch(output); len(matches) > 3 {
		if matches[1] != "" {
			fmt.Sscanf(matches[1], "%d", &result.FailedTests)
		}
		if matches[2] != "" {
			fmt.Sscanf(matches[2], "%d", &result.SkippedTests)
		}
		fmt.Sscanf(matches[3], "%d", &result.PassedTests)
		fmt.Sscanf(matches[4], "%d", &result.TotalTests)
		result.Passed = result.FailedTests == 0
	}

	// Coverage: "All files |   85.71 |    75 |     100 |   85.71 |"
	coverRe := regexp.MustCompile(`All files\s+\|\s+(\d+(?:\.\d+)?)\s+\|`)
	if matches := coverRe.FindStringSubmatch(output); len(matches) > 1 {
		fmt.Sscanf(matches[1], "%f", &result.Coverage)
	}
}

// parsePytestOutput parses pytest output.
func (a *Agent) parsePytestOutput(result *TestResult, output string) {
	// "5 passed, 1 failed, 2 skipped"
	passedRe := regexp.MustCompile(`(\d+) passed`)
	failedRe := regexp.MustCompile(`(\d+) failed`)
	skippedRe := regexp.MustCompile(`(\d+) skipped`)

	if matches := passedRe.FindStringSubmatch(output); len(matches) > 1 {
		fmt.Sscanf(matches[1], "%d", &result.PassedTests)
	}
	if matches := failedRe.FindStringSubmatch(output); len(matches) > 1 {
		fmt.Sscanf(matches[1], "%d", &result.FailedTests)
	}
	if matches := skippedRe.FindStringSubmatch(output); len(matches) > 1 {
		fmt.Sscanf(matches[1], "%d", &result.SkippedTests)
	}

	result.TotalTests = result.PassedTests + result.FailedTests + result.SkippedTests
	result.Passed = result.FailedTests == 0
}

// Execute implements the Agent interface for coordinated execution.
func (a *Agent) Execute(ctx context.Context, handoff coordinator.Handoff) (coordinator.Handoff, error) {
	filesHandoff, ok := handoff.(*FilesHandoff)
	if ok && len(filesHandoff.Files) > 0 {
		result, err := a.RunForFiles(ctx, filesHandoff.Files)
		if err != nil {
			return nil, err
		}
		return &TestResultHandoff{Result: result}, nil
	}

	result, err := a.RunAll(ctx)
	if err != nil {
		return nil, err
	}
	return &TestResultHandoff{Result: result}, nil
}

// FilesHandoff wraps file list for targeted testing.
type FilesHandoff struct {
	Files []string
}

func (h *FilesHandoff) Full() string {
	return fmt.Sprintf("Files to test:\n%s", strings.Join(h.Files, "\n"))
}

func (h *FilesHandoff) Concise() string {
	return fmt.Sprintf("%d files to test", len(h.Files))
}

func (h *FilesHandoff) ForTokenBudget(maxTokens int) string {
	return h.Full()
}

func (h *FilesHandoff) Type() string {
	return "files"
}

// TestResultHandoff wraps test results for the next agent.
type TestResultHandoff struct {
	Result *TestResult
}

func (h *TestResultHandoff) Full() string {
	r := h.Result
	var sb strings.Builder

	sb.WriteString("# Test Results\n\n")
	
	if r.Passed {
		sb.WriteString("✅ **ALL TESTS PASSED**\n\n")
	} else {
		sb.WriteString("❌ **TESTS FAILED**\n\n")
	}

	sb.WriteString(fmt.Sprintf("- Framework: %s\n", r.Framework))
	sb.WriteString(fmt.Sprintf("- Total: %d tests\n", r.TotalTests))
	sb.WriteString(fmt.Sprintf("- Passed: %d\n", r.PassedTests))
	sb.WriteString(fmt.Sprintf("- Failed: %d\n", r.FailedTests))
	sb.WriteString(fmt.Sprintf("- Skipped: %d\n", r.SkippedTests))
	if r.Coverage > 0 {
		sb.WriteString(fmt.Sprintf("- Coverage: %.1f%%\n", r.Coverage))
	}
	sb.WriteString(fmt.Sprintf("- Duration: %s\n", r.Duration.Round(time.Millisecond)))

	if len(r.FailedNames) > 0 {
		sb.WriteString("\n## Failed Tests\n")
		for _, name := range r.FailedNames {
			sb.WriteString(fmt.Sprintf("- %s\n", name))
		}
	}

	if !r.Passed && r.Output != "" {
		sb.WriteString("\n## Output (truncated)\n```\n")
		output := r.Output
		if len(output) > 2000 {
			output = output[len(output)-2000:]
		}
		sb.WriteString(output)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}

func (h *TestResultHandoff) Concise() string {
	r := h.Result
	if r.Passed {
		return fmt.Sprintf("✅ Tests passed (%d/%d, %.1f%% coverage)", 
			r.PassedTests, r.TotalTests, r.Coverage)
	}
	return fmt.Sprintf("❌ Tests failed (%d failed, %d passed)", 
		r.FailedTests, r.PassedTests)
}

func (h *TestResultHandoff) ForTokenBudget(maxTokens int) string {
	full := h.Full()
	if len(full) < maxTokens*4 {
		return full
	}
	return h.Concise()
}

func (h *TestResultHandoff) Type() string {
	return "test_result"
}
