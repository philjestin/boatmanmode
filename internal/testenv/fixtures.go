package testenv

// Fixtures contains pre-defined test data for e2e tests.

// DefaultTicket returns a standard test ticket.
func DefaultTicket() TicketFixture {
	return TicketFixture{
		ID:          "issue-123",
		Title:       "Add multiply function to util package",
		Description: "We need a Multiply function in the util package that multiplies two integers and returns the result.",
		BranchName:  "eng-123-add-multiply",
		State:       "In Progress",
		Priority:    1,
		Labels:      []string{"enhancement", "backend"},
	}
}

// BugFixTicket returns a bug fix ticket.
func BugFixTicket() TicketFixture {
	return TicketFixture{
		ID:          "issue-456",
		Title:       "Fix divide by zero in calculator",
		Description: "The Divide function panics when dividing by zero. Add proper error handling.",
		BranchName:  "eng-456-fix-divide-zero",
		State:       "In Progress",
		Priority:    0, // Urgent
		Labels:      []string{"bug", "critical"},
	}
}

// RefactorTicket returns a refactoring ticket.
func RefactorTicket() TicketFixture {
	return TicketFixture{
		ID:          "issue-789",
		Title:       "Refactor util package for better error handling",
		Description: "Update all functions in the util package to return errors instead of panicking.",
		BranchName:  "eng-789-refactor-errors",
		State:       "Ready",
		Priority:    2,
		Labels:      []string{"refactor", "tech-debt"},
	}
}

// ClaudeResponses contains canned Claude responses for testing.
var ClaudeResponses = struct {
	// Planning phase responses
	PlanSimpleFeature string
	PlanBugFix        string

	// Execution phase responses
	ExecuteAddFunction string
	ExecuteFixBug      string

	// Review phase responses
	ReviewPass         string
	ReviewNeedsChanges string
	ReviewFail         string

	// Refactor phase responses
	RefactorMinor string
}{
	PlanSimpleFeature: `Based on the ticket, I'll implement a Multiply function.

## Plan
1. Add Multiply function to pkg/util/util.go
2. Add unit tests in pkg/util/util_test.go
3. Ensure all tests pass

## Files to modify
- pkg/util/util.go
- pkg/util/util_test.go

## Approach
- Follow existing code style
- Add comprehensive tests
`,

	PlanBugFix: `This is a bug fix for divide by zero handling.

## Plan
1. Add error return to Divide function
2. Check for zero divisor
3. Update tests

## Files to modify
- pkg/util/util.go
- pkg/util/util_test.go
`,

	ExecuteAddFunction: `I'll add the Multiply function to the util package.

<file path="pkg/util/util.go">
package util

// Add adds two numbers.
func Add(a, b int) int {
	return a + b
}

// Multiply multiplies two numbers.
func Multiply(a, b int) int {
	return a * b
}
</file>

<file path="pkg/util/util_test.go">
package util

import "testing"

func TestAdd(t *testing.T) {
	result := Add(2, 3)
	if result != 5 {
		t.Errorf("Add(2, 3) = %d, want 5", result)
	}
}

func TestMultiply(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{2, 3, 6},
		{0, 5, 0},
		{-2, 3, -6},
	}
	for _, tt := range tests {
		result := Multiply(tt.a, tt.b)
		if result != tt.want {
			t.Errorf("Multiply(%d, %d) = %d, want %d", tt.a, tt.b, result, tt.want)
		}
	}
}
</file>
`,

	ExecuteFixBug: `I'll fix the divide by zero bug.

<file path="pkg/util/util.go">
package util

import "errors"

// ErrDivideByZero is returned when attempting to divide by zero.
var ErrDivideByZero = errors.New("division by zero")

// Add adds two numbers.
func Add(a, b int) int {
	return a + b
}

// Divide divides a by b, returning an error if b is zero.
func Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, ErrDivideByZero
	}
	return a / b, nil
}
</file>
`,

	ReviewPass: `## Code Review

**Overall Assessment: PASS ✅**

The implementation looks good:
- Function is correctly implemented
- Tests cover edge cases
- Code follows project conventions

No issues found.
`,

	ReviewNeedsChanges: `## Code Review

**Overall Assessment: NEEDS CHANGES ⚠️**

### Issues Found

1. **Missing error handling** (major)
   - The function should validate inputs

2. **Incomplete test coverage** (minor)
   - Add test for negative numbers

### Recommendations
- Add input validation
- Expand test coverage
`,

	ReviewFail: `## Code Review

**Overall Assessment: FAIL ❌**

### Critical Issues

1. **Function does not compile**
   - Syntax error on line 15

2. **Tests are broken**
   - TestMultiply references undefined function
`,

	RefactorMinor: `I'll address the review feedback.

<file path="pkg/util/util.go">
package util

// Add adds two numbers.
func Add(a, b int) int {
	return a + b
}

// Multiply multiplies two numbers.
// Returns 0 if either input is 0 for efficiency.
func Multiply(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	return a * b
}
</file>
`,
}

// ScenarioHappyPath returns responses for a successful workflow.
func ScenarioHappyPath() []string {
	return []string{
		ClaudeResponses.PlanSimpleFeature,
		ClaudeResponses.ExecuteAddFunction,
		ClaudeResponses.ReviewPass,
	}
}

// ScenarioNeedsRefactor returns responses where refactoring is needed.
func ScenarioNeedsRefactor() []string {
	return []string{
		ClaudeResponses.PlanSimpleFeature,
		ClaudeResponses.ExecuteAddFunction,
		ClaudeResponses.ReviewNeedsChanges,
		ClaudeResponses.RefactorMinor,
		ClaudeResponses.ReviewPass,
	}
}

// ScenarioFailsReview returns responses where review never passes.
func ScenarioFailsReview() []string {
	return []string{
		ClaudeResponses.PlanSimpleFeature,
		ClaudeResponses.ExecuteAddFunction,
		ClaudeResponses.ReviewFail,
		ClaudeResponses.RefactorMinor,
		ClaudeResponses.ReviewFail,
		ClaudeResponses.RefactorMinor,
		ClaudeResponses.ReviewFail,
	}
}
