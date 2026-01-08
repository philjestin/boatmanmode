package preflight

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/handshake/boatmanmode/internal/planner"
)

func TestValidateExistingFiles(t *testing.T) {
	// Create temp directory with some files
	tmpDir, err := os.MkdirTemp("", "preflight-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	os.WriteFile(filepath.Join(tmpDir, "existing.go"), []byte("package main"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "subdir", "file.go"), []byte("package subdir"), 0644)

	agent := New(tmpDir)

	plan := &planner.Plan{
		Summary: "Test plan",
		RelevantFiles: []string{
			"existing.go",
			"subdir/file.go",
			"missing.go", // This doesn't exist
		},
		RelevantDirs: []string{
			"subdir",
			"nonexistent", // This doesn't exist
		},
	}

	result, err := agent.Validate(context.Background(), plan)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Should have validated files
	if len(result.ValidatedFiles) != 2 {
		t.Errorf("Expected 2 validated files, got %d", len(result.ValidatedFiles))
	}

	// Should have missing files
	if len(result.MissingFiles) != 1 {
		t.Errorf("Expected 1 missing file, got %d", len(result.MissingFiles))
	}

	// Should have warnings for missing items
	if len(result.Warnings) < 2 {
		t.Errorf("Expected at least 2 warnings, got %d", len(result.Warnings))
	}
}

func TestValidateWithMostFilesMissing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preflight-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create only one file
	os.WriteFile(filepath.Join(tmpDir, "only.go"), []byte("package main"), 0644)

	agent := New(tmpDir)

	plan := &planner.Plan{
		Summary: "Test plan with mostly missing files",
		RelevantFiles: []string{
			"only.go",
			"missing1.go",
			"missing2.go",
			"missing3.go",
			"missing4.go",
		},
	}

	result, err := agent.Validate(context.Background(), plan)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Should be invalid due to too many missing files
	if result.Valid {
		t.Error("Should be invalid when most files are missing")
	}

	// Should have error
	hasError := false
	for _, e := range result.Errors {
		if e.Code == "TOO_MANY_MISSING" {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Error("Should have TOO_MANY_MISSING error")
	}
}

func TestValidatePatterns(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preflight-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	agent := New(tmpDir)

	plan := &planner.Plan{
		Summary: "Test patterns",
		ExistingPatterns: []string{
			"Use deprecated API in path/to/file.go",
			"Standard pattern with no issues",
		},
	}

	result, err := agent.Validate(context.Background(), plan)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Should have warning for deprecated pattern
	hasDeprecatedWarning := false
	for _, w := range result.Warnings {
		if w.Code == "DEPRECATED_PATTERN" {
			hasDeprecatedWarning = true
			break
		}
	}
	if !hasDeprecatedWarning {
		t.Error("Should have warning for deprecated pattern")
	}
}

func TestValidateApproach(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "preflight-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	agent := New(tmpDir)

	// Test vague approach
	plan := &planner.Plan{
		Summary: "Vague plan",
		Approach: []string{
			"Maybe implement the feature",
			"Possibly add tests",
			"Perhaps update docs",
		},
	}

	result, err := agent.Validate(context.Background(), plan)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Should have warnings for vague steps
	vagueCount := 0
	for _, w := range result.Warnings {
		if w.Code == "VAGUE_STEP" {
			vagueCount++
		}
	}
	if vagueCount != 3 {
		t.Errorf("Expected 3 VAGUE_STEP warnings, got %d", vagueCount)
	}

	// Test missing test strategy suggestion
	hasSuggestion := false
	for _, s := range result.Suggestions {
		if s == "Consider adding a test strategy to the plan" {
			hasSuggestion = true
			break
		}
	}
	if !hasSuggestion {
		t.Error("Should suggest adding test strategy")
	}
}

func TestValidationHandoff(t *testing.T) {
	result := &ValidationResult{
		Valid:          true,
		ValidatedFiles: []string{"file1.go", "file2.go"},
		Warnings:       []Warning{{Code: "WARN", Message: "Test warning"}},
	}

	handoff := &ValidationHandoff{Result: result}

	// Test Full
	full := handoff.Full()
	if full == "" {
		t.Error("Full() should return content")
	}
	if !contains(full, "VALID") {
		t.Error("Full() should contain VALID")
	}

	// Test Concise
	concise := handoff.Concise()
	if !contains(concise, "2 files verified") {
		t.Error("Concise() should mention validated files")
	}

	// Test Type
	if handoff.Type() != "validation" {
		t.Errorf("Expected type 'validation', got %s", handoff.Type())
	}
}

func TestInvalidResult(t *testing.T) {
	result := &ValidationResult{
		Valid: false,
		Errors: []ValidationError{
			{Code: "ERROR", Message: "Test error"},
		},
	}

	handoff := &ValidationHandoff{Result: result}

	concise := handoff.Concise()
	if !contains(concise, "Invalid") {
		t.Error("Concise() should indicate invalid")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && 
		(s == substr || len(s) > len(substr) && 
			(s[:len(substr)] == substr || contains(s[1:], substr)))
}
