// Package preflight provides pre-execution validation.
// It validates that the planner's output is feasible before
// spending resources on execution.
package preflight

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/handshake/boatmanmode/internal/coordinator"
	"github.com/handshake/boatmanmode/internal/planner"
)

// ValidationResult contains the outcome of pre-flight checks.
type ValidationResult struct {
	Valid    bool
	Warnings []Warning
	Errors   []ValidationError
	// Suggestions for improving the plan
	Suggestions []string
	// ValidatedFiles are files confirmed to exist
	ValidatedFiles []string
	// MissingFiles are files referenced but not found
	MissingFiles []string
}

// Warning is a non-blocking issue.
type Warning struct {
	Code    string
	Message string
	File    string
}

// ValidationError is a blocking issue.
type ValidationError struct {
	Code    string
	Message string
	File    string
}

// Agent performs pre-flight validation.
type Agent struct {
	id          string
	worktreePath string
	coord       *coordinator.Coordinator
}

// New creates a new pre-flight validation agent.
func New(worktreePath string) *Agent {
	return &Agent{
		id:          "preflight",
		worktreePath: worktreePath,
	}
}

// ID returns the agent ID.
func (a *Agent) ID() string {
	return a.id
}

// Name returns the human-readable name.
func (a *Agent) Name() string {
	return "Pre-flight Validator"
}

// Capabilities returns what this agent can do.
func (a *Agent) Capabilities() []coordinator.AgentCapability {
	return []coordinator.AgentCapability{coordinator.CapValidate}
}

// SetCoordinator sets the coordinator for communication.
func (a *Agent) SetCoordinator(c *coordinator.Coordinator) {
	a.coord = c
}

// Validate checks if a plan is feasible.
func (a *Agent) Validate(ctx context.Context, plan *planner.Plan) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:          true,
		Warnings:       []Warning{},
		Errors:         []ValidationError{},
		Suggestions:    []string{},
		ValidatedFiles: []string{},
		MissingFiles:   []string{},
	}

	// Claim work if coordinated
	if a.coord != nil {
		claim := &coordinator.WorkClaim{
			WorkID:      "preflight-validation",
			WorkType:    "validate",
			Description: "Pre-flight validation of execution plan",
		}
		if !a.coord.ClaimWork(a.id, claim) {
			return nil, fmt.Errorf("could not claim preflight work")
		}
		defer a.coord.ReleaseWork(claim.WorkID, a.id)
	}

	// 1. Validate referenced files exist
	a.validateFiles(plan, result)

	// 2. Validate referenced directories exist
	a.validateDirectories(plan, result)

	// 3. Validate patterns are still applicable
	a.validatePatterns(plan, result)

	// 4. Check for potential conflicts
	a.checkConflicts(plan, result)

	// 5. Validate approach is coherent
	a.validateApproach(plan, result)

	// Set overall validity
	result.Valid = len(result.Errors) == 0

	// Share result via coordinator
	if a.coord != nil {
		a.coord.SetContext("preflight_result", result)
	}

	return result, nil
}

// validateFiles checks that referenced files exist.
func (a *Agent) validateFiles(plan *planner.Plan, result *ValidationResult) {
	for _, file := range plan.RelevantFiles {
		fullPath := filepath.Join(a.worktreePath, file)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			result.MissingFiles = append(result.MissingFiles, file)
			result.Warnings = append(result.Warnings, Warning{
				Code:    "FILE_NOT_FOUND",
				Message: fmt.Sprintf("Referenced file does not exist: %s", file),
				File:    file,
			})
		} else if err == nil {
			result.ValidatedFiles = append(result.ValidatedFiles, file)
		}
	}

	// Too many missing files is an error
	if len(result.MissingFiles) > len(plan.RelevantFiles)/2 && len(plan.RelevantFiles) > 2 {
		result.Errors = append(result.Errors, ValidationError{
			Code:    "TOO_MANY_MISSING",
			Message: fmt.Sprintf("%d of %d referenced files are missing", len(result.MissingFiles), len(plan.RelevantFiles)),
		})
	}
}

// validateDirectories checks that referenced directories exist.
func (a *Agent) validateDirectories(plan *planner.Plan, result *ValidationResult) {
	for _, dir := range plan.RelevantDirs {
		fullPath := filepath.Join(a.worktreePath, dir)
		info, err := os.Stat(fullPath)
		if os.IsNotExist(err) {
			result.Warnings = append(result.Warnings, Warning{
				Code:    "DIR_NOT_FOUND",
				Message: fmt.Sprintf("Referenced directory does not exist: %s", dir),
				File:    dir,
			})
		} else if err == nil && !info.IsDir() {
			result.Warnings = append(result.Warnings, Warning{
				Code:    "NOT_A_DIR",
				Message: fmt.Sprintf("Referenced path is not a directory: %s", dir),
				File:    dir,
			})
		}
	}
}

// validatePatterns checks that referenced patterns are still valid.
func (a *Agent) validatePatterns(plan *planner.Plan, result *ValidationResult) {
	for _, pattern := range plan.ExistingPatterns {
		// Look for file references in patterns
		// Pattern format: "Pattern: Use XyzResolver for queries in path/to/file.rb"
		lower := strings.ToLower(pattern)
		
		// Check for common pattern indicators
		if strings.Contains(lower, "deprecated") {
			result.Warnings = append(result.Warnings, Warning{
				Code:    "DEPRECATED_PATTERN",
				Message: fmt.Sprintf("Pattern may reference deprecated code: %s", truncate(pattern, 100)),
			})
		}

		// Check if pattern references a specific file that doesn't exist
		words := strings.Fields(pattern)
		for _, word := range words {
			if strings.Contains(word, "/") && (strings.HasSuffix(word, ".rb") || 
				strings.HasSuffix(word, ".go") || 
				strings.HasSuffix(word, ".ts") ||
				strings.HasSuffix(word, ".py")) {
				
				cleanPath := strings.Trim(word, "`,\"'()")
				fullPath := filepath.Join(a.worktreePath, cleanPath)
				if _, err := os.Stat(fullPath); os.IsNotExist(err) {
					result.Warnings = append(result.Warnings, Warning{
						Code:    "PATTERN_FILE_MISSING",
						Message: fmt.Sprintf("Pattern references missing file: %s", cleanPath),
						File:    cleanPath,
					})
				}
			}
		}
	}
}

// checkConflicts looks for potential issues.
func (a *Agent) checkConflicts(plan *planner.Plan, result *ValidationResult) {
	// Check if any files are locked by other agents
	if a.coord != nil {
		for _, file := range plan.RelevantFiles {
			if locked, holder := a.coord.IsFileLocked(file); locked && holder != a.id {
				result.Errors = append(result.Errors, ValidationError{
					Code:    "FILE_LOCKED",
					Message: fmt.Sprintf("File %s is locked by agent %s", file, holder),
					File:    file,
				})
			}
		}
	}

	// Check for conflicting approach steps
	approachLower := make([]string, len(plan.Approach))
	for i, step := range plan.Approach {
		approachLower[i] = strings.ToLower(step)
	}

	// Look for contradictory steps
	for i, step := range approachLower {
		for j := i + 1; j < len(approachLower); j++ {
			other := approachLower[j]
			// Simple contradiction detection
			if strings.Contains(step, "create") && strings.Contains(other, "delete") {
				if extractTarget(step) == extractTarget(other) {
					result.Warnings = append(result.Warnings, Warning{
						Code:    "CONTRADICTORY_STEPS",
						Message: fmt.Sprintf("Steps %d and %d may be contradictory", i+1, j+1),
					})
				}
			}
		}
	}
}

// validateApproach checks the approach for coherence.
func (a *Agent) validateApproach(plan *planner.Plan, result *ValidationResult) {
	if len(plan.Approach) == 0 {
		result.Warnings = append(result.Warnings, Warning{
			Code:    "NO_APPROACH",
			Message: "Plan has no approach steps defined",
		})
		return
	}

	// Check for vague steps
	vagueWords := []string{"maybe", "might", "possibly", "could", "perhaps", "somehow"}
	for i, step := range plan.Approach {
		lower := strings.ToLower(step)
		for _, vague := range vagueWords {
			if strings.Contains(lower, vague) {
				result.Warnings = append(result.Warnings, Warning{
					Code:    "VAGUE_STEP",
					Message: fmt.Sprintf("Step %d contains vague language: %s", i+1, truncate(step, 80)),
				})
				break
			}
		}
	}

	// Check for missing test strategy
	if plan.TestStrategy == "" {
		result.Suggestions = append(result.Suggestions, "Consider adding a test strategy to the plan")
	}
}

// Execute implements the Agent interface for coordinated execution.
func (a *Agent) Execute(ctx context.Context, handoff coordinator.Handoff) (coordinator.Handoff, error) {
	// Extract plan from handoff
	planHandoff, ok := handoff.(*PlanHandoff)
	if !ok {
		return nil, fmt.Errorf("expected PlanHandoff, got %T", handoff)
	}

	result, err := a.Validate(ctx, planHandoff.Plan)
	if err != nil {
		return nil, err
	}

	return &ValidationHandoff{Result: result}, nil
}

// PlanHandoff wraps a plan for the preflight agent.
type PlanHandoff struct {
	Plan *planner.Plan
}

func (h *PlanHandoff) Full() string {
	return h.Plan.ToHandoff()
}

func (h *PlanHandoff) Concise() string {
	return fmt.Sprintf("Plan: %s (%d steps, %d files)", h.Plan.Summary, len(h.Plan.Approach), len(h.Plan.RelevantFiles))
}

func (h *PlanHandoff) ForTokenBudget(maxTokens int) string {
	full := h.Full()
	if len(full) < maxTokens*4 { // rough char-to-token ratio
		return full
	}
	return h.Concise()
}

func (h *PlanHandoff) Type() string {
	return "plan"
}

// ValidationHandoff wraps validation results for the next agent.
type ValidationHandoff struct {
	Result *ValidationResult
}

func (h *ValidationHandoff) Full() string {
	var sb strings.Builder
	sb.WriteString("# Pre-flight Validation Result\n\n")
	
	if h.Result.Valid {
		sb.WriteString("✅ **VALID** - Ready for execution\n\n")
	} else {
		sb.WriteString("❌ **INVALID** - Cannot proceed\n\n")
	}

	if len(h.Result.Errors) > 0 {
		sb.WriteString("## Errors\n")
		for _, e := range h.Result.Errors {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", e.Code, e.Message))
		}
		sb.WriteString("\n")
	}

	if len(h.Result.Warnings) > 0 {
		sb.WriteString("## Warnings\n")
		for _, w := range h.Result.Warnings {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", w.Code, w.Message))
		}
		sb.WriteString("\n")
	}

	if len(h.Result.ValidatedFiles) > 0 {
		sb.WriteString("## Validated Files\n")
		for _, f := range h.Result.ValidatedFiles {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	if len(h.Result.Suggestions) > 0 {
		sb.WriteString("## Suggestions\n")
		for _, s := range h.Result.Suggestions {
			sb.WriteString(fmt.Sprintf("- %s\n", s))
		}
	}

	return sb.String()
}

func (h *ValidationHandoff) Concise() string {
	if h.Result.Valid {
		return fmt.Sprintf("✅ Valid (%d files verified, %d warnings)", 
			len(h.Result.ValidatedFiles), len(h.Result.Warnings))
	}
	return fmt.Sprintf("❌ Invalid (%d errors, %d warnings)", 
		len(h.Result.Errors), len(h.Result.Warnings))
}

func (h *ValidationHandoff) ForTokenBudget(maxTokens int) string {
	full := h.Full()
	if len(full) < maxTokens*4 {
		return full
	}
	return h.Concise()
}

func (h *ValidationHandoff) Type() string {
	return "validation"
}

// Helper functions

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func extractTarget(step string) string {
	// Very simple target extraction
	words := strings.Fields(step)
	for i, w := range words {
		if strings.Contains(w, "/") || strings.Contains(w, ".") {
			return w
		}
		if i > 0 && (words[i-1] == "create" || words[i-1] == "delete" || words[i-1] == "modify") {
			return w
		}
	}
	return ""
}
