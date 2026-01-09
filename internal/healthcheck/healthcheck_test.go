package healthcheck

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDefaultDependencies(t *testing.T) {
	deps := DefaultDependencies()

	if len(deps) == 0 {
		t.Fatal("Should have default dependencies")
	}

	// Check expected dependencies exist
	expectedNames := map[string]bool{
		"git":   false,
		"gh":    false,
		"claude": false,
		"tmux":  false,
	}

	for _, dep := range deps {
		if _, ok := expectedNames[dep.Name]; ok {
			expectedNames[dep.Name] = true
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("Expected dependency %s not found", name)
		}
	}
}

func TestCheckDependency_Git(t *testing.T) {
	// Git should be available on any dev machine
	ctx := context.Background()

	dep := Dependency{
		Name:     "git",
		Command:  "git",
		Args:     []string{"--version"},
		Required: true,
	}

	result := checkDependency(ctx, dep)

	if !result.Available {
		t.Skip("git not available, skipping test")
	}

	if result.Version == "" {
		t.Error("Should have version string")
	}

	if !strings.Contains(result.Version, "git version") {
		t.Errorf("Expected 'git version' in output, got: %s", result.Version)
	}
}

func TestCheckDependency_NotFound(t *testing.T) {
	ctx := context.Background()

	dep := Dependency{
		Name:     "nonexistent-command-xyz",
		Command:  "nonexistent-command-xyz",
		Args:     []string{"--version"},
		Required: true,
	}

	result := checkDependency(ctx, dep)

	if result.Available {
		t.Error("Non-existent command should not be available")
	}

	if result.Error == nil {
		t.Error("Should have error for missing command")
	}
}

func TestCheckDependency_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	// Create a context that's already timed out
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // Ensure timeout

	dep := Dependency{
		Name:    "git",
		Command: "git",
		Args:    []string{"--version"},
	}

	result := checkDependency(ctx, dep)

	// Result depends on race condition, but shouldn't panic
	_ = result
}

func TestCheck_AllAvailable(t *testing.T) {
	ctx := context.Background()

	// Test with commands that should be available
	deps := []Dependency{
		{Name: "echo", Command: "echo", Args: []string{"hello"}, Required: true},
	}

	results := Check(ctx, deps)

	if !results.Passed {
		t.Error("Should pass when all required deps available")
	}

	if len(results.Missing) != 0 {
		t.Errorf("Should have no missing deps, got: %v", results.Missing)
	}

	if len(results.All) != 1 {
		t.Errorf("Should have 1 result, got %d", len(results.All))
	}
}

func TestCheck_MissingRequired(t *testing.T) {
	ctx := context.Background()

	deps := []Dependency{
		{Name: "echo", Command: "echo", Args: []string{"hello"}, Required: true},
		{Name: "missing", Command: "nonexistent-xyz", Args: []string{}, Required: true},
	}

	results := Check(ctx, deps)

	if results.Passed {
		t.Error("Should fail when required dep missing")
	}

	if len(results.Missing) != 1 {
		t.Errorf("Should have 1 missing, got: %v", results.Missing)
	}

	if results.Missing[0] != "missing" {
		t.Errorf("Expected 'missing' in missing list, got: %v", results.Missing)
	}
}

func TestCheck_MissingOptional(t *testing.T) {
	ctx := context.Background()

	deps := []Dependency{
		{Name: "echo", Command: "echo", Args: []string{"hello"}, Required: true},
		{Name: "optional-missing", Command: "nonexistent-xyz", Args: []string{}, Required: false},
	}

	results := Check(ctx, deps)

	if !results.Passed {
		t.Error("Should pass when only optional dep missing")
	}

	if len(results.Missing) != 0 {
		t.Errorf("Missing list should only contain required deps, got: %v", results.Missing)
	}

	// But the result should still show it's not available
	found := false
	for _, r := range results.All {
		if r.Name == "optional-missing" && !r.Available {
			found = true
		}
	}
	if !found {
		t.Error("Optional missing dep should be in results as unavailable")
	}
}

func TestResults_Format(t *testing.T) {
	results := &Results{
		All: []Result{
			{Name: "git", Available: true, Version: "git version 2.40.0"},
			{Name: "missing", Available: false},
		},
		Passed:  false,
		Missing: []string{"missing"},
	}

	output := results.Format()

	if !strings.Contains(output, "git") {
		t.Error("Format should contain 'git'")
	}
	if !strings.Contains(output, "✅") {
		t.Error("Format should contain checkmark for available")
	}
	if !strings.Contains(output, "❌") {
		t.Error("Format should contain X for unavailable")
	}
	if !strings.Contains(output, "missing") {
		t.Error("Format should mention missing deps")
	}
}

func TestResults_FormatAllPassing(t *testing.T) {
	results := &Results{
		All: []Result{
			{Name: "git", Available: true, Version: "git version 2.40.0"},
		},
		Passed:  true,
		Missing: []string{},
	}

	output := results.Format()

	if !strings.Contains(output, "All required") {
		t.Error("Format should indicate all passed")
	}
}

func TestResults_Error(t *testing.T) {
	// Passing results
	passing := &Results{Passed: true}
	if passing.Error() != nil {
		t.Error("Passing results should have nil error")
	}

	// Failing results
	failing := &Results{
		Passed:  false,
		Missing: []string{"dep1", "dep2"},
	}
	err := failing.Error()
	if err == nil {
		t.Error("Failing results should have error")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "dep1") || !strings.Contains(errMsg, "dep2") {
		t.Errorf("Error should list missing deps, got: %s", errMsg)
	}
}

func TestCheckDefault(t *testing.T) {
	ctx := context.Background()

	// Just verify it doesn't panic
	results := CheckDefault(ctx)

	if results == nil {
		t.Fatal("CheckDefault should return results")
	}

	if len(results.All) == 0 {
		t.Error("Should have checked some dependencies")
	}
}

func TestResult_String(t *testing.T) {
	available := Result{Name: "git", Available: true, Version: "2.40.0"}
	unavailable := Result{Name: "missing", Available: false}

	// Verify fields are accessible
	if available.Name != "git" {
		t.Error("Name should be git")
	}
	if !available.Available {
		t.Error("Should be available")
	}
	if unavailable.Available {
		t.Error("Should not be available")
	}
}
