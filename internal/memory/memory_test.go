package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "memory-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	if store.baseDir != tmpDir {
		t.Errorf("Expected baseDir %s, got %s", tmpDir, store.baseDir)
	}
}

func TestGetMemory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "memory-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := NewStore(tmpDir)

	mem, err := store.Get("/path/to/project")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if mem == nil {
		t.Fatal("Memory should not be nil")
	}

	// Same path should return cached memory
	mem2, _ := store.Get("/path/to/project")
	if mem.ProjectID != mem2.ProjectID {
		t.Error("Same path should return same memory")
	}

	// Different path should return different memory
	mem3, _ := store.Get("/different/project")
	if mem.ProjectID == mem3.ProjectID {
		t.Error("Different path should return different memory")
	}
}

func TestLearnPattern(t *testing.T) {
	mem := &Memory{
		Patterns: []Pattern{},
	}

	pattern := Pattern{
		ID:          "test-pattern",
		Type:        "naming",
		Description: "Use snake_case for variables",
		Weight:      0.8,
	}

	mem.LearnPattern(pattern)

	if len(mem.Patterns) != 1 {
		t.Errorf("Expected 1 pattern, got %d", len(mem.Patterns))
	}

	// Learn same pattern again - should update
	pattern.SuccessRate = 0.9
	mem.LearnPattern(pattern)

	if len(mem.Patterns) != 1 {
		t.Error("Duplicate pattern should update, not add")
	}
	if mem.Patterns[0].UsageCount != 2 {
		t.Errorf("Expected usage count 2, got %d", mem.Patterns[0].UsageCount)
	}
}

func TestLearnIssue(t *testing.T) {
	mem := &Memory{
		CommonIssues: []CommonIssue{},
	}

	issue := CommonIssue{
		Type:        "style",
		Description: "Missing error handling",
		Solution:    "Add error check",
	}

	mem.LearnIssue(issue)

	if len(mem.CommonIssues) != 1 {
		t.Errorf("Expected 1 issue, got %d", len(mem.CommonIssues))
	}

	// Learn similar issue - should update frequency
	issue2 := CommonIssue{
		Type:        "style",
		Description: "Missing error handling in function",
	}
	mem.LearnIssue(issue2)

	if len(mem.CommonIssues) != 1 {
		t.Error("Similar issue should update, not add")
	}
	if mem.CommonIssues[0].Frequency != 2 {
		t.Errorf("Expected frequency 2, got %d", mem.CommonIssues[0].Frequency)
	}
}

func TestLearnPrompt(t *testing.T) {
	mem := &Memory{
		SuccessfulPrompts: []PromptRecord{},
	}

	mem.LearnPrompt("feature", "Add the feature", "Success", 85)

	if len(mem.SuccessfulPrompts) != 1 {
		t.Errorf("Expected 1 prompt, got %d", len(mem.SuccessfulPrompts))
	}

	if mem.SuccessfulPrompts[0].SuccessScore != 85 {
		t.Errorf("Expected score 85, got %d", mem.SuccessfulPrompts[0].SuccessScore)
	}
}

func TestUpdateStats(t *testing.T) {
	mem := &Memory{}

	mem.UpdateStats(true, 2, 5*time.Minute)

	if mem.Stats.TotalSessions != 1 {
		t.Errorf("Expected 1 session, got %d", mem.Stats.TotalSessions)
	}
	if mem.Stats.SuccessfulSessions != 1 {
		t.Errorf("Expected 1 successful, got %d", mem.Stats.SuccessfulSessions)
	}
	if mem.Stats.TotalIterations != 2 {
		t.Errorf("Expected 2 iterations, got %d", mem.Stats.TotalIterations)
	}

	// Add failed session
	mem.UpdateStats(false, 3, 10*time.Minute)

	if mem.Stats.TotalSessions != 2 {
		t.Errorf("Expected 2 sessions, got %d", mem.Stats.TotalSessions)
	}
	if mem.Stats.SuccessfulSessions != 1 {
		t.Error("Successful sessions should still be 1")
	}
}

func TestGetPatternsForFile(t *testing.T) {
	mem := &Memory{
		Patterns: []Pattern{
			{ID: "go-1", Type: "naming", Description: "Go pattern", FileMatcher: "*.go", Weight: 0.8},
			{ID: "rb-1", Type: "naming", Description: "Ruby pattern", FileMatcher: "*.rb", Weight: 0.9},
			{ID: "all-1", Type: "general", Description: "General pattern", Weight: 0.7},
		},
	}

	// Go file - should get at least the Go pattern
	patterns := mem.GetPatternsForFile("pkg/util.go")
	if len(patterns) < 1 {
		t.Errorf("Expected at least 1 pattern for .go file, got %d", len(patterns))
	}

	// Ruby file - should get at least the Ruby pattern
	patterns = mem.GetPatternsForFile("app/models/user.rb")
	if len(patterns) < 1 {
		t.Errorf("Expected at least 1 pattern for .rb file, got %d", len(patterns))
	}
}

func TestGetBestPromptForType(t *testing.T) {
	mem := &Memory{
		SuccessfulPrompts: []PromptRecord{
			{TicketType: "feature", Prompt: "Low score", SuccessScore: 60},
			{TicketType: "feature", Prompt: "High score", SuccessScore: 90},
			{TicketType: "bugfix", Prompt: "Other type", SuccessScore: 95},
		},
	}

	best := mem.GetBestPromptForType("feature")
	if best == nil {
		t.Fatal("Should find best prompt")
	}
	if best.SuccessScore != 90 {
		t.Errorf("Expected score 90, got %d", best.SuccessScore)
	}

	// Non-existent type
	none := mem.GetBestPromptForType("nonexistent")
	if none != nil {
		t.Error("Should return nil for non-existent type")
	}
}

func TestToContext(t *testing.T) {
	mem := &Memory{
		Patterns: []Pattern{
			{Type: "naming", Description: "Use camelCase", Weight: 0.9},
			{Type: "testing", Description: "Add unit tests", Weight: 0.8},
		},
		CommonIssues: []CommonIssue{
			{Description: "Missing docs", Solution: "Add docstring", Frequency: 3},
		},
		Preferences: Preferences{
			PreferredTestFramework: "go test",
			NamingConventions:      map[string]string{"functions": "camelCase"},
		},
	}

	context := mem.ToContext(10000)

	if context == "" {
		t.Error("ToContext should return content")
	}

	// Test truncation
	shortContext := mem.ToContext(50)
	if len(shortContext) > 250 { // 50 tokens * ~5 chars
		t.Error("Should truncate to token budget")
	}
}

func TestSaveAndReload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "memory-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := NewStore(tmpDir)
	mem, _ := store.Get("/test/project")

	// Add data
	mem.LearnPattern(Pattern{
		ID:          "test",
		Type:        "naming",
		Description: "Test pattern",
		Weight:      0.8,
	})
	mem.LearnIssue(CommonIssue{
		Type:        "style",
		Description: "Test issue",
	})

	// Save
	err = store.Save(mem)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Clear cache and reload
	store.cache = make(map[string]*Memory)
	mem2, _ := store.Get("/test/project")

	if len(mem2.Patterns) != 1 {
		t.Errorf("Expected 1 pattern after reload, got %d", len(mem2.Patterns))
	}
	if len(mem2.CommonIssues) != 1 {
		t.Errorf("Expected 1 issue after reload, got %d", len(mem2.CommonIssues))
	}
}

func TestFormatStats(t *testing.T) {
	mem := &Memory{
		Stats: SessionStats{
			TotalSessions:      10,
			SuccessfulSessions: 8,
			AvgIterationsPerPR: 2.5,
			AvgDuration:        15 * time.Minute,
		},
		Patterns:     make([]Pattern, 5),
		CommonIssues: make([]CommonIssue, 3),
	}

	formatted := mem.FormatStats()
	if formatted == "" {
		t.Error("FormatStats should return content")
	}
}

func TestHashPath(t *testing.T) {
	hash1 := hashPath("/path/to/project")
	hash2 := hashPath("/path/to/project")
	hash3 := hashPath("/different/project")

	if hash1 != hash2 {
		t.Error("Same path should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("Different paths should produce different hashes")
	}
}

func TestSimilar(t *testing.T) {
	// Identical
	if !similar("hello world", "hello world") {
		t.Error("Identical strings should be similar")
	}

	// Contained
	if !similar("hello world test", "hello world") {
		t.Error("Containing strings should be similar")
	}

	// Word overlap
	if !similar("error handling missing", "missing error handling code") {
		t.Error("High word overlap should be similar")
	}

	// No overlap
	if similar("hello world", "foo bar") {
		t.Error("No overlap should not be similar")
	}
}

func TestAnalyzer(t *testing.T) {
	mem := &Memory{
		Patterns:     []Pattern{},
		CommonIssues: []CommonIssue{},
		Preferences: Preferences{
			FileOrganization: make(map[string]string),
		},
	}

	analyzer := NewAnalyzer(mem)

	// Analyze successful completion
	files := []string{"pkg/util_test.go", "pkg/util.go"}
	analyzer.AnalyzeSuccess(files, 85)

	// Should have learned patterns
	if len(mem.Patterns) == 0 {
		t.Error("Should learn patterns from success")
	}

	// Analyze issue
	analyzer.AnalyzeIssue("major", "Missing security check", "Add validation", "auth.go")

	if len(mem.CommonIssues) != 1 {
		t.Errorf("Expected 1 issue, got %d", len(mem.CommonIssues))
	}
	if mem.CommonIssues[0].Type != "security" {
		t.Errorf("Expected security type, got %s", mem.CommonIssues[0].Type)
	}
}

func TestPatternLimit(t *testing.T) {
	mem := &Memory{
		Patterns: []Pattern{},
	}

	// Add many patterns
	for i := 0; i < 150; i++ {
		mem.LearnPattern(Pattern{
			ID:          string(rune('a' + i%26)) + string(rune('0'+i/26)),
			Type:        "test",
			Description: "Pattern",
			Weight:      float64(i) / 150.0,
		})
	}

	// Should be limited to 100
	if len(mem.Patterns) > 100 {
		t.Errorf("Patterns should be limited to 100, got %d", len(mem.Patterns))
	}
}

func TestCommonIssueLimit(t *testing.T) {
	mem := &Memory{
		CommonIssues: []CommonIssue{},
	}

	// Add many issues
	for i := 0; i < 60; i++ {
		mem.LearnIssue(CommonIssue{
			ID:          string(rune('a' + i%26)) + string(rune('0'+i/26)),
			Type:        "test",
			Description: "Issue " + string(rune('0'+i)),
		})
	}

	// Should be limited to 50
	if len(mem.CommonIssues) > 50 {
		t.Errorf("Issues should be limited to 50, got %d", len(mem.CommonIssues))
	}
}

func TestPromptLimit(t *testing.T) {
	mem := &Memory{
		SuccessfulPrompts: []PromptRecord{},
	}

	// Add many prompts
	for i := 0; i < 30; i++ {
		mem.LearnPrompt("feature", "prompt", "result", i*3)
	}

	// Should be limited to 20
	if len(mem.SuccessfulPrompts) > 20 {
		t.Errorf("Prompts should be limited to 20, got %d", len(mem.SuccessfulPrompts))
	}

	// Should keep highest scoring
	for _, p := range mem.SuccessfulPrompts {
		if p.SuccessScore < 30 {
			t.Error("Should keep high scoring prompts")
		}
	}
}

func TestDefaultStore(t *testing.T) {
	// Test with empty baseDir (should use default)
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore with default path failed: %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expectedDir := filepath.Join(homeDir, ".boatman", "memory")

	if store.baseDir != expectedDir {
		t.Errorf("Expected default dir %s, got %s", expectedDir, store.baseDir)
	}
}
