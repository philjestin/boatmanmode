// Package cost provides token usage and cost tracking for Claude API calls.
package cost

import (
	"fmt"
	"strings"
	"sync"
)

// Usage represents token usage and cost from a single Claude API call.
type Usage struct {
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_input_tokens"`
	CacheWriteTokens int     `json:"cache_creation_input_tokens"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
}

// Add combines two Usage records.
func (u Usage) Add(other Usage) Usage {
	return Usage{
		InputTokens:      u.InputTokens + other.InputTokens,
		OutputTokens:     u.OutputTokens + other.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens + other.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens + other.CacheWriteTokens,
		TotalCostUSD:     u.TotalCostUSD + other.TotalCostUSD,
	}
}

// IsEmpty returns true if no tokens were used.
func (u Usage) IsEmpty() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.TotalCostUSD == 0
}

// StepUsage pairs a step name with its usage.
type StepUsage struct {
	Step  string
	Usage Usage
}

// Tracker aggregates usage across multiple steps.
type Tracker struct {
	steps []StepUsage
	mu    sync.Mutex
}

// NewTracker creates a new cost tracker.
func NewTracker() *Tracker {
	return &Tracker{
		steps: make([]StepUsage, 0),
	}
}

// Add records usage for a named step.
func (t *Tracker) Add(step string, usage Usage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.steps = append(t.steps, StepUsage{Step: step, Usage: usage})
}

// Steps returns all recorded step usages.
func (t *Tracker) Steps() []StepUsage {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]StepUsage, len(t.steps))
	copy(result, t.steps)
	return result
}

// Total returns the aggregated usage across all steps.
func (t *Tracker) Total() Usage {
	t.mu.Lock()
	defer t.mu.Unlock()

	var total Usage
	for _, s := range t.steps {
		total = total.Add(s.Usage)
	}
	return total
}

// HasUsage returns true if any usage has been recorded.
func (t *Tracker) HasUsage() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.steps) > 0
}

// Summary returns a formatted summary table of all usage.
func (t *Tracker) Summary() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.steps) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("\n   💰 COST SUMMARY\n")
	sb.WriteString("   ─────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("   %-20s %10s %10s %10s %10s\n", "Step", "Input", "Output", "Cache", "Cost"))
	sb.WriteString("   ─────────────────────────────────────────────────────────────────\n")

	var total Usage
	for _, s := range t.steps {
		sb.WriteString(fmt.Sprintf("   %-20s %10s %10s %10s %10s\n",
			truncateStep(s.Step, 20),
			formatTokens(s.Usage.InputTokens),
			formatTokens(s.Usage.OutputTokens),
			formatTokens(s.Usage.CacheReadTokens),
			formatCost(s.Usage.TotalCostUSD),
		))
		total = total.Add(s.Usage)
	}

	sb.WriteString("   ─────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("   %-20s %10s %10s %10s %10s\n",
		"TOTAL",
		formatTokens(total.InputTokens),
		formatTokens(total.OutputTokens),
		formatTokens(total.CacheReadTokens),
		formatCost(total.TotalCostUSD),
	))

	return sb.String()
}

// formatTokens formats a token count with comma separators.
func formatTokens(n int) string {
	if n == 0 {
		return "-"
	}
	return formatWithCommas(n)
}

// formatWithCommas adds comma separators to a number.
func formatWithCommas(n int) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	var result strings.Builder
	remainder := len(str) % 3
	if remainder > 0 {
		result.WriteString(str[:remainder])
		if len(str) > remainder {
			result.WriteString(",")
		}
	}

	for i := remainder; i < len(str); i += 3 {
		result.WriteString(str[i : i+3])
		if i+3 < len(str) {
			result.WriteString(",")
		}
	}

	return result.String()
}

// formatCost formats a USD cost.
func formatCost(cost float64) string {
	if cost == 0 {
		return "-"
	}
	return fmt.Sprintf("$%.4f", cost)
}

// truncateStep truncates a step name to fit the column width.
func truncateStep(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
