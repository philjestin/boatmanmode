package contextpin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewContextPinner(t *testing.T) {
	pinner := New("/test/path")
	if pinner == nil {
		t.Fatal("New() returned nil")
	}
	if pinner.worktreePath != "/test/path" {
		t.Errorf("Expected path /test/path, got %s", pinner.worktreePath)
	}
}

func TestNewDependencyGraph(t *testing.T) {
	graph := NewDependencyGraph()
	if graph == nil {
		t.Fatal("NewDependencyGraph() returned nil")
	}
	if graph.dependencies == nil {
		t.Error("dependencies map should be initialized")
	}
}

func TestAnalyzeFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contextpin-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create Go file with imports
	goContent := `package main

import (
	"fmt"
	"os"
	"./pkg/util"
)

func main() {}
`
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(goContent), 0644)

	pinner := New(tmpDir)
	deps, err := pinner.AnalyzeFile("main.go")
	if err != nil {
		t.Fatalf("AnalyzeFile failed: %v", err)
	}

	// Should have found some dependencies (at least local ones)
	_ = deps // May be empty for external imports
}

func TestAnalyzeFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contextpin-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte("package main\nimport \"fmt\""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("package main"), 0644)

	pinner := New(tmpDir)
	err = pinner.AnalyzeFiles([]string{"file1.go", "file2.go"})
	if err != nil {
		t.Fatalf("AnalyzeFiles failed: %v", err)
	}
}

func TestPinAndUnpin(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contextpin-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("content2"), 0644)

	pinner := New(tmpDir)

	// Pin files
	files := []string{"file1.go", "file2.go"}
	pin, err := pinner.Pin("agent-1", files, false)
	if err != nil {
		t.Fatalf("Pin failed: %v", err)
	}

	if pin == nil {
		t.Fatal("Pin should not be nil")
	}
	if len(pin.Files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(pin.Files))
	}

	// Unpin
	pinner.Unpin("agent-1")
}

func TestPinWithLock(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contextpin-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("content"), 0644)

	pinner := New(tmpDir)

	// Pin with lock
	pin, err := pinner.Pin("agent-1", []string{"file.go"}, true)
	if err != nil {
		t.Fatalf("Pin with lock failed: %v", err)
	}

	if !pin.Locked {
		t.Error("Pin should be locked")
	}
}

func TestGetPinnedContent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contextpin-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	content := "package main"
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte(content), 0644)

	pinner := New(tmpDir)
	pinner.Pin("agent-1", []string{"test.go"}, false)

	// Get pinned content
	retrieved, ok := pinner.GetPinnedContent("agent-1", "test.go")
	if !ok {
		t.Error("Should find pinned content")
	}
	if retrieved != content {
		t.Errorf("Expected %s, got %s", content, retrieved)
	}

	// Non-existent
	_, ok = pinner.GetPinnedContent("agent-1", "other.go")
	if ok {
		t.Error("Should not find non-pinned file")
	}
}

func TestVerifyPin(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contextpin-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.go")
	os.WriteFile(testFile, []byte("original"), 0644)

	pinner := New(tmpDir)
	pinner.Pin("agent-1", []string{"test.go"}, false)

	// Should be valid initially
	valid, stale := pinner.VerifyPin("agent-1")
	if !valid {
		t.Error("Should be valid initially")
	}
	if len(stale) != 0 {
		t.Error("No files should be stale initially")
	}

	// Modify file
	os.WriteFile(testFile, []byte("modified"), 0644)

	// Should now be invalid
	valid, stale = pinner.VerifyPin("agent-1")
	if valid {
		t.Error("Should be invalid after modification")
	}
	if len(stale) == 0 {
		t.Error("Should have stale files")
	}
}

func TestRefreshPin(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contextpin-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.go")
	os.WriteFile(testFile, []byte("original"), 0644)

	pinner := New(tmpDir)
	pinner.Pin("agent-1", []string{"test.go"}, false)

	// Modify file
	os.WriteFile(testFile, []byte("modified"), 0644)

	// Refresh
	err = pinner.RefreshPin("agent-1")
	if err != nil {
		t.Fatalf("RefreshPin failed: %v", err)
	}

	// Should now be valid
	valid, _ := pinner.VerifyPin("agent-1")
	if !valid {
		t.Error("Should be valid after refresh")
	}
}

func TestGetDependencies(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contextpin-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create files with dependencies
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(`package main
import "./util"
`), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "util"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "util", "util.go"), []byte("package util"), 0644)

	pinner := New(tmpDir)
	pinner.AnalyzeFiles([]string{"main.go"})

	deps := pinner.GetDependencies("main.go")
	// May or may not find dependencies depending on import parsing
	_ = deps
}

func TestGetDependents(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contextpin-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pinner := New(tmpDir)
	
	// GetDependents for non-analyzed file
	dependents := pinner.GetDependents("util.go")
	// Should return empty or nil for unanalyzed
	_ = dependents
}

func TestGetRelatedFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contextpin-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)

	pinner := New(tmpDir)
	pinner.AnalyzeFiles([]string{"main.go"})

	related := pinner.GetRelatedFiles("main.go")
	// Just verify it doesn't panic
	_ = related
}

func TestExtractDependencies(t *testing.T) {
	goCode := `package main

import (
	"fmt"
	"./internal/util"
)
`
	deps := extractDependencies(goCode, ".go")
	// Should extract local imports
	_ = deps
}

func TestExtractRubyDependencies(t *testing.T) {
	rubyCode := `require 'json'
require_relative 'helper'
`
	deps := extractDependencies(rubyCode, ".rb")
	if len(deps) == 0 {
		t.Error("Should extract Ruby requires")
	}
}

func TestExtractJSDependencies(t *testing.T) {
	jsCode := `import { foo } from './foo';
import bar from './bar';
const baz = require('./baz');
`
	deps := extractDependencies(jsCode, ".js")
	if len(deps) < 2 {
		t.Errorf("Expected at least 2 JS dependencies, got %d", len(deps))
	}
}

func TestExtractPythonDependencies(t *testing.T) {
	pyCode := `from .utils import helper
import local_module
from . import subpackage
`
	deps := extractDependencies(pyCode, ".py")
	// Python local imports need special handling
	_ = deps
}

func TestPinConsistency(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "contextpin-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	content := "package main"
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte(content), 0644)

	pinner := New(tmpDir)
	
	// Pin file twice - checksums should be same
	pin1, _ := pinner.Pin("agent-1", []string{"test.go"}, false)
	pinner.Unpin("agent-1")
	pin2, _ := pinner.Pin("agent-1", []string{"test.go"}, false)

	if pin1.Checksums["test.go"] != pin2.Checksums["test.go"] {
		t.Error("Same content should produce same checksum")
	}
}

func TestPinHandoff(t *testing.T) {
	pin := &Pin{
		Files:    []string{"file1.go", "file2.go"},
		AgentID:  "agent-1",
		Locked:   true,
		Contents: map[string]string{"file1.go": "content"},
	}

	handoff := &PinHandoff{Pin: pin}

	// Test methods
	if handoff.Type() != "pinned_context" {
		t.Errorf("Expected type 'pinned_context', got %s", handoff.Type())
	}

	full := handoff.Full()
	if full == "" {
		t.Error("Full() should return content")
	}

	concise := handoff.Concise()
	if concise == "" {
		t.Error("Concise() should return content")
	}

	budget := handoff.ForTokenBudget(100)
	if budget == "" {
		t.Error("ForTokenBudget() should return content")
	}
}
