package cost

import (
	"math"
	"strings"
	"testing"
)

// floatEquals compares two floats with a tolerance for floating point errors.
func floatEquals(a, b float64) bool {
	return math.Abs(a-b) < 0.0001
}

func TestUsage_Add(t *testing.T) {
	u1 := Usage{
		InputTokens:      1000,
		OutputTokens:     500,
		CacheReadTokens:  200,
		CacheWriteTokens: 100,
		TotalCostUSD:     0.05,
	}

	u2 := Usage{
		InputTokens:      2000,
		OutputTokens:     1000,
		CacheReadTokens:  300,
		CacheWriteTokens: 150,
		TotalCostUSD:     0.10,
	}

	result := u1.Add(u2)

	if result.InputTokens != 3000 {
		t.Errorf("InputTokens = %d, want 3000", result.InputTokens)
	}
	if result.OutputTokens != 1500 {
		t.Errorf("OutputTokens = %d, want 1500", result.OutputTokens)
	}
	if result.CacheReadTokens != 500 {
		t.Errorf("CacheReadTokens = %d, want 500", result.CacheReadTokens)
	}
	if result.CacheWriteTokens != 250 {
		t.Errorf("CacheWriteTokens = %d, want 250", result.CacheWriteTokens)
	}
	if !floatEquals(result.TotalCostUSD, 0.15) {
		t.Errorf("TotalCostUSD = %f, want 0.15", result.TotalCostUSD)
	}
}

func TestUsage_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		usage    Usage
		expected bool
	}{
		{
			name:     "empty usage",
			usage:    Usage{},
			expected: true,
		},
		{
			name: "has input tokens",
			usage: Usage{
				InputTokens: 100,
			},
			expected: false,
		},
		{
			name: "has output tokens",
			usage: Usage{
				OutputTokens: 100,
			},
			expected: false,
		},
		{
			name: "has cost only",
			usage: Usage{
				TotalCostUSD: 0.01,
			},
			expected: false,
		},
		{
			name: "has only cache tokens",
			usage: Usage{
				CacheReadTokens: 100,
			},
			expected: true, // Cache tokens alone don't make it non-empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.usage.IsEmpty(); got != tt.expected {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTracker_Add(t *testing.T) {
	tracker := NewTracker()

	tracker.Add("Step 1", Usage{InputTokens: 100, OutputTokens: 50, TotalCostUSD: 0.01})
	tracker.Add("Step 2", Usage{InputTokens: 200, OutputTokens: 100, TotalCostUSD: 0.02})

	steps := tracker.Steps()
	if len(steps) != 2 {
		t.Errorf("Steps() len = %d, want 2", len(steps))
	}

	if steps[0].Step != "Step 1" {
		t.Errorf("steps[0].Step = %s, want Step 1", steps[0].Step)
	}
	if steps[1].Step != "Step 2" {
		t.Errorf("steps[1].Step = %s, want Step 2", steps[1].Step)
	}
}

func TestTracker_Total(t *testing.T) {
	tracker := NewTracker()

	tracker.Add("Planning", Usage{
		InputTokens:     1000,
		OutputTokens:    500,
		CacheReadTokens: 200,
		TotalCostUSD:    0.05,
	})
	tracker.Add("Execution", Usage{
		InputTokens:     2000,
		OutputTokens:    1000,
		CacheReadTokens: 400,
		TotalCostUSD:    0.10,
	})
	tracker.Add("Review", Usage{
		InputTokens:     500,
		OutputTokens:    200,
		CacheReadTokens: 100,
		TotalCostUSD:    0.02,
	})

	total := tracker.Total()

	if total.InputTokens != 3500 {
		t.Errorf("Total InputTokens = %d, want 3500", total.InputTokens)
	}
	if total.OutputTokens != 1700 {
		t.Errorf("Total OutputTokens = %d, want 1700", total.OutputTokens)
	}
	if total.CacheReadTokens != 700 {
		t.Errorf("Total CacheReadTokens = %d, want 700", total.CacheReadTokens)
	}
	if !floatEquals(total.TotalCostUSD, 0.17) {
		t.Errorf("Total TotalCostUSD = %f, want 0.17", total.TotalCostUSD)
	}
}

func TestTracker_HasUsage(t *testing.T) {
	tracker := NewTracker()

	if tracker.HasUsage() {
		t.Error("empty tracker should not have usage")
	}

	tracker.Add("Step", Usage{InputTokens: 100})

	if !tracker.HasUsage() {
		t.Error("tracker with steps should have usage")
	}
}

func TestTracker_Summary(t *testing.T) {
	tracker := NewTracker()

	// Empty tracker should return empty string
	if tracker.Summary() != "" {
		t.Error("empty tracker summary should be empty string")
	}

	// Add some steps
	tracker.Add("Planning", Usage{
		InputTokens:     12450,
		OutputTokens:    3200,
		CacheReadTokens: 8000,
		TotalCostUSD:    0.0234,
	})
	tracker.Add("Execution", Usage{
		InputTokens:     45000,
		OutputTokens:    12000,
		CacheReadTokens: 32000,
		TotalCostUSD:    0.089,
	})

	summary := tracker.Summary()

	// Check that summary contains expected content
	if !strings.Contains(summary, "COST SUMMARY") {
		t.Error("summary should contain 'COST SUMMARY'")
	}
	if !strings.Contains(summary, "Planning") {
		t.Error("summary should contain 'Planning'")
	}
	if !strings.Contains(summary, "Execution") {
		t.Error("summary should contain 'Execution'")
	}
	if !strings.Contains(summary, "TOTAL") {
		t.Error("summary should contain 'TOTAL'")
	}
	if !strings.Contains(summary, "$") {
		t.Error("summary should contain cost with '$'")
	}
}

func TestFormatWithCommas(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{12, "12"},
		{123, "123"},
		{1234, "1,234"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{12345678, "12,345,678"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := formatWithCommas(tt.input); got != tt.expected {
				t.Errorf("formatWithCommas(%d) = %s, want %s", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "-"},
		{100, "100"},
		{1000, "1,000"},
		{100000, "100,000"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := formatTokens(tt.input); got != tt.expected {
				t.Errorf("formatTokens(%d) = %s, want %s", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{0, "-"},
		{0.01, "$0.0100"},
		{0.1234, "$0.1234"},
		{1.5, "$1.5000"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := formatCost(tt.input); got != tt.expected {
				t.Errorf("formatCost(%f) = %s, want %s", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTruncateStep(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"Short", 10, "Short"},
		{"Exactly10!", 10, "Exactly10!"},
		{"This is a long step name", 10, "This is..."},
		{"Planning", 20, "Planning"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := truncateStep(tt.input, tt.maxLen); got != tt.expected {
				t.Errorf("truncateStep(%s, %d) = %s, want %s", tt.input, tt.maxLen, got, tt.expected)
			}
		})
	}
}

func TestTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewTracker()
	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(n int) {
			tracker.Add("Step", Usage{InputTokens: n * 100})
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	steps := tracker.Steps()
	if len(steps) != 10 {
		t.Errorf("expected 10 steps, got %d", len(steps))
	}
}
