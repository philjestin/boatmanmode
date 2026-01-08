package testrunner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFrameworkGo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testrunner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create go.mod
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)

	agent := New(tmpDir)
	framework, err := agent.DetectFramework()
	if err != nil {
		t.Fatalf("DetectFramework failed: %v", err)
	}

	if framework.Name != "go" {
		t.Errorf("Expected 'go', got %s", framework.Name)
	}
	if framework.Command != "go" {
		t.Errorf("Expected command 'go', got %s", framework.Command)
	}
}

func TestDetectFrameworkNode(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testrunner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create package.json with jest
	os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{
		"name": "test",
		"devDependencies": {
			"jest": "^29.0.0"
		}
	}`), 0644)

	agent := New(tmpDir)
	framework, err := agent.DetectFramework()
	if err != nil {
		t.Fatalf("DetectFramework failed: %v", err)
	}

	if framework.Name != "jest" {
		t.Errorf("Expected 'jest', got %s", framework.Name)
	}
}

func TestDetectFrameworkRSpec(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testrunner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create Gemfile with rspec
	os.WriteFile(filepath.Join(tmpDir, "Gemfile"), []byte(`
		source 'https://rubygems.org'
		gem 'rspec'
	`), 0644)

	agent := New(tmpDir)
	framework, err := agent.DetectFramework()
	if err != nil {
		t.Fatalf("DetectFramework failed: %v", err)
	}

	if framework.Name != "rspec" {
		t.Errorf("Expected 'rspec', got %s", framework.Name)
	}
}

func TestDetectFrameworkPytest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testrunner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create pytest.ini
	os.WriteFile(filepath.Join(tmpDir, "pytest.ini"), []byte("[pytest]"), 0644)

	agent := New(tmpDir)
	framework, err := agent.DetectFramework()
	if err != nil {
		t.Fatalf("DetectFramework failed: %v", err)
	}

	if framework.Name != "pytest" {
		t.Errorf("Expected 'pytest', got %s", framework.Name)
	}
}

func TestDetectFrameworkNone(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testrunner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	agent := New(tmpDir)
	_, err = agent.DetectFramework()
	if err == nil {
		t.Error("Expected error when no framework detected")
	}
}

func TestIsTestFile(t *testing.T) {
	agent := &Agent{}

	goFramework := &Framework{Name: "go"}
	if !agent.isTestFile("pkg/util_test.go", goFramework) {
		t.Error("Should detect Go test file")
	}
	if agent.isTestFile("pkg/util.go", goFramework) {
		t.Error("Should not detect non-test Go file")
	}

	rspecFramework := &Framework{Name: "rspec"}
	if !agent.isTestFile("spec/models/user_spec.rb", rspecFramework) {
		t.Error("Should detect RSpec test file")
	}

	jestFramework := &Framework{Name: "jest"}
	if !agent.isTestFile("src/utils/helper.test.ts", jestFramework) {
		t.Error("Should detect Jest test file")
	}
	if !agent.isTestFile("src/utils/helper.spec.js", jestFramework) {
		t.Error("Should detect Jest spec file")
	}

	pytestFramework := &Framework{Name: "pytest"}
	if !agent.isTestFile("tests/test_user.py", pytestFramework) {
		t.Error("Should detect pytest test file")
	}
}

func TestFindTestFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testrunner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create source and test files
	os.MkdirAll(filepath.Join(tmpDir, "pkg"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "pkg", "util.go"), []byte("package pkg"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "pkg", "util_test.go"), []byte("package pkg"), 0644)

	agent := New(tmpDir)
	goFramework := &Framework{Name: "go"}

	testFile := agent.findTestFile("pkg/util.go", goFramework)
	if testFile != "pkg/util_test.go" {
		t.Errorf("Expected 'pkg/util_test.go', got %s", testFile)
	}
}

func TestParseGoOutput(t *testing.T) {
	agent := &Agent{}
	result := &TestResult{}

	output := `=== RUN   TestExample
--- PASS: TestExample (0.00s)
=== RUN   TestOther
--- FAIL: TestOther (0.01s)
=== RUN   TestSkipped
--- SKIP: TestSkipped (0.00s)
FAIL
coverage: 75.5% of statements
`

	agent.parseGoOutput(result, output)

	if result.PassedTests != 1 {
		t.Errorf("Expected 1 passed, got %d", result.PassedTests)
	}
	if result.FailedTests != 1 {
		t.Errorf("Expected 1 failed, got %d", result.FailedTests)
	}
	if result.SkippedTests != 1 {
		t.Errorf("Expected 1 skipped, got %d", result.SkippedTests)
	}
	if result.Passed {
		t.Error("Result should be failed")
	}
	if result.Coverage != 75.5 {
		t.Errorf("Expected coverage 75.5, got %f", result.Coverage)
	}
	if len(result.FailedNames) != 1 || result.FailedNames[0] != "TestOther" {
		t.Errorf("Expected failed test name 'TestOther', got %v", result.FailedNames)
	}
}

func TestParseRspecOutput(t *testing.T) {
	agent := &Agent{}
	result := &TestResult{}

	output := `
Finished in 0.5 seconds
15 examples, 2 failures, 3 pending
`

	agent.parseRspecOutput(result, output)

	if result.TotalTests != 15 {
		t.Errorf("Expected 15 total, got %d", result.TotalTests)
	}
	if result.FailedTests != 2 {
		t.Errorf("Expected 2 failed, got %d", result.FailedTests)
	}
	if result.SkippedTests != 3 {
		t.Errorf("Expected 3 pending, got %d", result.SkippedTests)
	}
	if result.PassedTests != 10 {
		t.Errorf("Expected 10 passed, got %d", result.PassedTests)
	}
}

func TestParseJestOutput(t *testing.T) {
	agent := &Agent{}
	result := &TestResult{}

	output := `
Test Suites: 2 passed, 2 total
Tests:       1 failed, 2 skipped, 5 passed, 8 total
Snapshots:   0 total
Time:        2.5 s

All files |   85.71 |    75 |     100 |   85.71 |
`

	agent.parseJestOutput(result, output)

	if result.TotalTests != 8 {
		t.Errorf("Expected 8 total, got %d", result.TotalTests)
	}
	if result.FailedTests != 1 {
		t.Errorf("Expected 1 failed, got %d", result.FailedTests)
	}
	if result.PassedTests != 5 {
		t.Errorf("Expected 5 passed, got %d", result.PassedTests)
	}
	if result.SkippedTests != 2 {
		t.Errorf("Expected 2 skipped, got %d", result.SkippedTests)
	}
	if result.Coverage != 85.71 {
		t.Errorf("Expected coverage 85.71, got %f", result.Coverage)
	}
}

func TestParsePytestOutput(t *testing.T) {
	agent := &Agent{}
	result := &TestResult{}

	output := `
========================= test session starts ==========================
collected 10 items
5 passed, 2 failed, 3 skipped in 1.23s
`

	agent.parsePytestOutput(result, output)

	if result.PassedTests != 5 {
		t.Errorf("Expected 5 passed, got %d", result.PassedTests)
	}
	if result.FailedTests != 2 {
		t.Errorf("Expected 2 failed, got %d", result.FailedTests)
	}
	if result.SkippedTests != 3 {
		t.Errorf("Expected 3 skipped, got %d", result.SkippedTests)
	}
}

func TestTestResultHandoff(t *testing.T) {
	result := &TestResult{
		Passed:       true,
		Framework:    "go",
		TotalTests:   10,
		PassedTests:  8,
		FailedTests:  0,
		SkippedTests: 2,
		Coverage:     85.5,
	}

	handoff := &TestResultHandoff{Result: result}

	// Test Full
	full := handoff.Full()
	if full == "" {
		t.Error("Full() should return content")
	}

	// Test Concise
	concise := handoff.Concise()
	if concise == "" {
		t.Error("Concise() should return content")
	}

	// Test Type
	if handoff.Type() != "test_result" {
		t.Errorf("Expected type 'test_result', got %s", handoff.Type())
	}
}

func TestFilesHandoff(t *testing.T) {
	handoff := &FilesHandoff{Files: []string{"file1.go", "file2.go"}}

	if handoff.Type() != "files" {
		t.Errorf("Expected type 'files', got %s", handoff.Type())
	}

	concise := handoff.Concise()
	if concise != "2 files to test" {
		t.Errorf("Expected '2 files to test', got %s", concise)
	}
}

func TestRunAllNoFramework(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testrunner-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	agent := New(tmpDir)
	result, err := agent.RunAll(context.Background())
	if err != nil {
		t.Fatalf("RunAll failed: %v", err)
	}

	// No framework = pass
	if !result.Passed {
		t.Error("Should pass when no framework detected")
	}
}
