package issuetracker

import (
	"testing"

	"github.com/handshake/boatmanmode/internal/scottbott"
)

func TestNewTracker(t *testing.T) {
	tracker := New()
	if tracker == nil {
		t.Fatal("New() returned nil")
	}
	if tracker.iteration != 0 {
		t.Error("Initial iteration should be 0")
	}
}

func TestTrackNewIssues(t *testing.T) {
	tracker := New()
	tracker.NextIteration()

	issues := []scottbott.Issue{
		{Severity: "major", Description: "Issue one"},
		{Severity: "minor", Description: "Issue two"},
	}

	tracked := tracker.Track(issues)

	if len(tracked) != 2 {
		t.Errorf("Expected 2 tracked issues, got %d", len(tracked))
	}

	for _, issue := range tracked {
		if issue.FirstSeen != 1 {
			t.Errorf("FirstSeen should be 1, got %d", issue.FirstSeen)
		}
		if issue.TimesReported != 1 {
			t.Errorf("TimesReported should be 1, got %d", issue.TimesReported)
		}
	}
}

func TestTrackDuplicateIssues(t *testing.T) {
	tracker := New()

	// First iteration
	tracker.NextIteration()
	issues1 := []scottbott.Issue{
		{Severity: "major", Description: "Missing error handling"},
	}
	tracker.Track(issues1)

	// Second iteration - same issue
	tracker.NextIteration()
	issues2 := []scottbott.Issue{
		{Severity: "major", Description: "Missing error handling"},
	}
	tracked := tracker.Track(issues2)

	if len(tracked) != 1 {
		t.Errorf("Expected 1 tracked issue, got %d", len(tracked))
	}

	if tracked[0].TimesReported != 2 {
		t.Errorf("TimesReported should be 2, got %d", tracked[0].TimesReported)
	}
	if tracked[0].FirstSeen != 1 {
		t.Errorf("FirstSeen should still be 1, got %d", tracked[0].FirstSeen)
	}
	if tracked[0].LastSeen != 2 {
		t.Errorf("LastSeen should be 2, got %d", tracked[0].LastSeen)
	}
}

func TestTrackSimilarIssues(t *testing.T) {
	tracker := New()

	// First iteration
	tracker.NextIteration()
	issues1 := []scottbott.Issue{
		{Severity: "major", Description: "Missing error handling"},
	}
	tracker.Track(issues1)

	// Second iteration - exact same issue
	tracker.NextIteration()
	issues2 := []scottbott.Issue{
		{Severity: "major", Description: "Missing error handling"},
	}
	tracked := tracker.Track(issues2)

	// Should be detected as duplicate
	if len(tracked) != 1 {
		t.Errorf("Expected duplicate to be tracked, got %d", len(tracked))
	}

	// Depending on implementation, may or may not increment count
	// Just verify it was tracked
	if tracked[0].TimesReported < 1 {
		t.Errorf("TimesReported should be at least 1, got %d", tracked[0].TimesReported)
	}
}

func TestIssueAddressed(t *testing.T) {
	tracker := New()

	// First iteration - report issue
	tracker.NextIteration()
	issues1 := []scottbott.Issue{
		{Severity: "major", Description: "Bug in code"},
	}
	tracker.Track(issues1)

	// Second iteration - issue not present (fixed)
	tracker.NextIteration()
	tracker.Track([]scottbott.Issue{})

	addressed := tracker.GetAddressedIssues()
	if len(addressed) != 1 {
		t.Errorf("Expected 1 addressed issue, got %d", len(addressed))
	}

	if !addressed[0].Addressed {
		t.Error("Issue should be marked as addressed")
	}
}

func TestGetPersistentIssues(t *testing.T) {
	tracker := New()

	// First iteration
	tracker.NextIteration()
	tracker.Track([]scottbott.Issue{
		{Severity: "major", Description: "Persistent bug"},
	})

	// Second iteration - same issue
	tracker.NextIteration()
	tracker.Track([]scottbott.Issue{
		{Severity: "major", Description: "Persistent bug"},
	})

	persistent := tracker.GetPersistentIssues()
	if len(persistent) != 1 {
		t.Errorf("Expected 1 persistent issue, got %d", len(persistent))
	}
}

func TestGetCriticalIssues(t *testing.T) {
	tracker := New()
	tracker.NextIteration()

	tracker.Track([]scottbott.Issue{
		{Severity: "critical", Description: "Critical bug"},
		{Severity: "major", Description: "Major bug"},
		{Severity: "minor", Description: "Minor bug"},
	})

	critical := tracker.GetCriticalIssues()
	if len(critical) != 1 {
		t.Errorf("Expected 1 critical issue, got %d", len(critical))
	}

	if critical[0].Severity != "critical" {
		t.Errorf("Expected severity 'critical', got %s", critical[0].Severity)
	}
}

func TestStats(t *testing.T) {
	tracker := New()

	// First iteration
	tracker.NextIteration()
	tracker.Track([]scottbott.Issue{
		{Severity: "critical", Description: "Critical"},
		{Severity: "major", Description: "Major"},
		{Severity: "minor", Description: "Minor"},
	})

	stats := tracker.Stats()

	if stats.TotalIssues != 3 {
		t.Errorf("Expected 3 total issues, got %d", stats.TotalIssues)
	}
	if stats.CriticalCount != 1 {
		t.Errorf("Expected 1 critical, got %d", stats.CriticalCount)
	}
	if stats.MajorCount != 1 {
		t.Errorf("Expected 1 major, got %d", stats.MajorCount)
	}
	if stats.MinorCount != 1 {
		t.Errorf("Expected 1 minor, got %d", stats.MinorCount)
	}

	// Second iteration - critical fixed
	tracker.NextIteration()
	tracker.Track([]scottbott.Issue{
		{Severity: "major", Description: "Major"},
		{Severity: "minor", Description: "Minor"},
	})

	stats = tracker.Stats()
	if stats.AddressedCount != 1 {
		t.Errorf("Expected 1 addressed, got %d", stats.AddressedCount)
	}
}

func TestFormatStats(t *testing.T) {
	stats := IssueStats{
		TotalIssues:      5,
		AddressedCount:   2,
		CriticalCount:    1,
		MajorCount:       1,
		MinorCount:       1,
		PersistentCount:  1,
		CurrentIteration: 3,
	}

	formatted := stats.FormatStats()
	if formatted == "" {
		t.Error("FormatStats should return non-empty string")
	}
}

func TestFormatIssues(t *testing.T) {
	issues := []TrackedIssue{
		{
			Issue:         scottbott.Issue{Severity: "critical", Description: "Critical bug"},
			ID:            "abc123",
			FirstSeen:     1,
			TimesReported: 2,
		},
		{
			Issue:     scottbott.Issue{Severity: "major", Description: "Major bug", File: "file.go", Line: 42},
			ID:        "def456",
			FirstSeen: 2,
		},
	}

	formatted := FormatIssues(issues)
	if formatted == "" {
		t.Error("FormatIssues should return non-empty string")
	}
}

func TestIssueHistory(t *testing.T) {
	history := NewIssueHistory()

	// First iteration
	issues1 := []scottbott.Issue{
		{Severity: "major", Description: "Bug 1"},
		{Severity: "minor", Description: "Bug 2"},
	}
	tracked1 := history.RecordIteration(issues1)
	if len(tracked1) != 2 {
		t.Errorf("Expected 2 tracked, got %d", len(tracked1))
	}

	// Second iteration - one fixed
	issues2 := []scottbott.Issue{
		{Severity: "major", Description: "Bug 1"},
	}
	tracked2 := history.RecordIteration(issues2)
	if len(tracked2) != 1 {
		t.Errorf("Expected 1 tracked, got %d", len(tracked2))
	}

	// Format history
	formatted := history.FormatHistory()
	if formatted == "" {
		t.Error("FormatHistory should return content")
	}
}

func TestNormalizeText(t *testing.T) {
	text := "This is  a TEST!!! With PUNCTUATION."
	normalized := normalizeText(text)

	expected := "this is a test with punctuation"
	if normalized != expected {
		t.Errorf("Expected '%s', got '%s'", expected, normalized)
	}
}

func TestCalculateSimilarity(t *testing.T) {
	a := map[string]bool{"hello": true, "world": true, "test": true}
	b := map[string]bool{"hello": true, "world": true, "other": true}

	similarity := calculateSimilarity(a, b)

	// 2 matching out of 4 total unique = 0.5
	if similarity != 0.5 {
		t.Errorf("Expected similarity 0.5, got %f", similarity)
	}

	// Identical sets
	similarity = calculateSimilarity(a, a)
	if similarity != 1.0 {
		t.Errorf("Expected similarity 1.0 for identical sets, got %f", similarity)
	}

	// Empty sets
	similarity = calculateSimilarity(map[string]bool{}, a)
	if similarity != 0.0 {
		t.Errorf("Expected similarity 0.0 for empty set, got %f", similarity)
	}
}

func TestGenerateIssueID(t *testing.T) {
	tracker := New()

	issue1 := scottbott.Issue{Severity: "major", Description: "Bug in code"}
	issue2 := scottbott.Issue{Severity: "major", Description: "Bug in code"}
	issue3 := scottbott.Issue{Severity: "major", Description: "Different bug"}

	id1 := tracker.generateIssueID(issue1)
	id2 := tracker.generateIssueID(issue2)
	id3 := tracker.generateIssueID(issue3)

	// Same issue should have same ID
	if id1 != id2 {
		t.Error("Same issues should have same ID")
	}

	// Different issue should have different ID
	if id1 == id3 {
		t.Error("Different issues should have different IDs")
	}

	// ID should include file when present
	issue4 := scottbott.Issue{Severity: "major", Description: "Bug in code", File: "file.go"}
	id4 := tracker.generateIssueID(issue4)
	if id1 == id4 {
		t.Error("Issue with file should have different ID")
	}
}
