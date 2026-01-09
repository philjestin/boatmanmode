package diffverify

import (
	"context"
	"testing"

	"github.com/philjestin/boatmanmode/internal/scottbott"
)

func TestParseDiff(t *testing.T) {
	diff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,5 +1,6 @@
 package main
 
+import "fmt"
+
 func main() {
-    println("hello")
+    fmt.Println("hello")
 }
`

	changes := parseDiff(diff)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(changes))
	}

	change, ok := changes["file.go"]
	if !ok {
		t.Fatal("Expected file.go in changes")
	}

	if len(change.Added) != 3 {
		t.Errorf("Expected 3 added lines, got %d", len(change.Added))
	}
	if len(change.Removed) != 1 {
		t.Errorf("Expected 1 removed line, got %d", len(change.Removed))
	}
}

func TestVerifyIssueAddressed(t *testing.T) {
	agent := New("/tmp")

	issues := []scottbott.Issue{
		{
			Severity:    "major",
			Description: "Use fmt.Println instead of println",
			File:        "file.go",
		},
	}

	oldDiff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,3 @@
 func main() {
-    // empty
+    println("hello")
 }
`

	newDiff := `--- a/file.go
+++ b/file.go
@@ -1,4 +1,5 @@
+import "fmt"
 func main() {
-    println("hello")
+    fmt.Println("hello")
 }
`

	result, err := agent.Verify(context.Background(), issues, oldDiff, newDiff)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if len(result.AddressedIssues) != 1 {
		t.Errorf("Expected 1 addressed issue, got %d", len(result.AddressedIssues))
	}
	if len(result.UnaddressedIssues) != 0 {
		t.Errorf("Expected 0 unaddressed issues, got %d", len(result.UnaddressedIssues))
	}
}

func TestVerifyIssueNotAddressed(t *testing.T) {
	agent := New("/tmp")

	issues := []scottbott.Issue{
		{
			Severity:    "major",
			Description: "Add error handling",
			File:        "other.go",
		},
	}

	oldDiff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,3 @@
+// some change
`

	newDiff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,3 @@
+// different change
`

	result, err := agent.Verify(context.Background(), issues, oldDiff, newDiff)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if result.AllAddressed {
		t.Error("Should not be all addressed")
	}
	if len(result.UnaddressedIssues) != 1 {
		t.Errorf("Expected 1 unaddressed issue, got %d", len(result.UnaddressedIssues))
	}
}

func TestDetectNewIssues(t *testing.T) {
	agent := New("/tmp")

	oldChanges := make(map[string]*DiffChange)
	newChanges := map[string]*DiffChange{
		"file.go": {
			File: "file.go",
			Added: []string{
				"// TODO: fix this later",
				"debugger",
				"console.log('test')",
			},
		},
	}

	issues := agent.detectNewIssues(oldChanges, newChanges)

	if len(issues) < 2 {
		t.Errorf("Expected at least 2 new issues detected, got %d", len(issues))
	}
}

func TestExtractKeywords(t *testing.T) {
	text := "Use fmt.Println instead of the println function"
	keywords := extractKeywords(text)

	if len(keywords) == 0 {
		t.Error("Should extract keywords")
	}

	// Should have meaningful words
	found := make(map[string]bool)
	for _, kw := range keywords {
		found[kw] = true
	}

	if !found["println"] {
		t.Error("Should include 'println'")
	}
	if !found["function"] {
		t.Error("Should include 'function'")
	}

	// Should not have stop words
	if found["the"] {
		t.Error("Should not include stop word 'the'")
	}
	if found["of"] {
		t.Error("Should not include stop word 'of'")
	}
}

func TestExtractBadPatterns(t *testing.T) {
	text := "Remove the 'println' call and use \"fmt.Println\" instead"
	patterns := extractBadPatterns(text)

	if len(patterns) == 0 {
		t.Error("Should extract quoted patterns")
	}

	found := false
	for _, p := range patterns {
		if p == "println" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Should include 'println' from quoted string")
	}
}

func TestVerificationHandoff(t *testing.T) {
	result := &VerificationResult{
		AllAddressed: true,
		AddressedIssues: []AddressedIssue{
			{
				Original:    scottbott.Issue{Severity: "major", Description: "Fix error"},
				FixEvidence: "Added error handling",
			},
		},
		Confidence: 85,
	}

	handoff := &VerificationHandoff{Result: result}

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
	if handoff.Type() != "verification_result" {
		t.Errorf("Expected type 'verification_result', got %s", handoff.Type())
	}
}

func TestVerifyHandoff(t *testing.T) {
	handoff := &VerifyHandoff{
		Issues: []scottbott.Issue{
			{Severity: "major", Description: "Test issue"},
		},
		OldDiff: "old diff content",
		NewDiff: "new diff content",
	}

	if handoff.Type() != "verify" {
		t.Errorf("Expected type 'verify', got %s", handoff.Type())
	}

	concise := handoff.Concise()
	if concise == "" {
		t.Error("Concise() should return content")
	}
}

func TestConfidenceCalculation(t *testing.T) {
	agent := New("/tmp")

	issues := []scottbott.Issue{
		{Severity: "major", Description: "Issue 1"},
		{Severity: "major", Description: "Issue 2"},
	}

	// Both addressed
	oldDiff := `+++ b/file.go
+// Issue 1 fix
+// Issue 2 fix
`
	newDiff := oldDiff

	result, _ := agent.Verify(context.Background(), issues, oldDiff, newDiff)

	// Confidence should be reasonable (not 0)
	if result.Confidence < 50 {
		t.Errorf("Confidence should be at least 50, got %d", result.Confidence)
	}
}
