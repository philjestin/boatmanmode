// Package healthcheck verifies external dependencies are available.
// Run these checks at startup to fail fast with helpful error messages.
package healthcheck

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Dependency represents an external dependency to check.
type Dependency struct {
	Name        string
	Command     string
	Args        []string
	Required    bool
	Description string
}

// Result represents the health check result for a dependency.
type Result struct {
	Name      string
	Available bool
	Version   string
	Error     error
}

// Results holds all health check results.
type Results struct {
	All     []Result
	Passed  bool
	Missing []string
}

// DefaultDependencies returns the standard dependencies for boatman.
func DefaultDependencies() []Dependency {
	return []Dependency{
		{
			Name:        "git",
			Command:     "git",
			Args:        []string{"--version"},
			Required:    true,
			Description: "Git version control",
		},
		{
			Name:        "gh",
			Command:     "gh",
			Args:        []string{"--version"},
			Required:    true,
			Description: "GitHub CLI for PR creation",
		},
		{
			Name:        "claude",
			Command:     "claude",
			Args:        []string{"--version"},
			Required:    true,
			Description: "Anthropic Claude CLI for AI assistance",
		},
		{
			Name:        "tmux",
			Command:     "tmux",
			Args:        []string{"-V"},
			Required:    false, // Optional, only needed for large prompts
			Description: "Terminal multiplexer for large prompts",
		},
	}
}

// Check verifies all dependencies are available.
func Check(ctx context.Context, deps []Dependency) *Results {
	results := &Results{
		All:    make([]Result, 0, len(deps)),
		Passed: true,
	}

	for _, dep := range deps {
		result := checkDependency(ctx, dep)
		results.All = append(results.All, result)

		if !result.Available && dep.Required {
			results.Passed = false
			results.Missing = append(results.Missing, dep.Name)
		}
	}

	return results
}

// CheckDefault checks the default dependencies.
func CheckDefault(ctx context.Context) *Results {
	return Check(ctx, DefaultDependencies())
}

// checkDependency checks if a single dependency is available.
func checkDependency(ctx context.Context, dep Dependency) Result {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, dep.Command, dep.Args...)
	output, err := cmd.Output()

	if err != nil {
		return Result{
			Name:      dep.Name,
			Available: false,
			Error:     fmt.Errorf("%s not found: %w", dep.Name, err),
		}
	}

	// Extract version from output (first line usually)
	version := strings.TrimSpace(string(output))
	if idx := strings.Index(version, "\n"); idx > 0 {
		version = version[:idx]
	}

	return Result{
		Name:      dep.Name,
		Available: true,
		Version:   version,
	}
}

// Format returns a human-readable summary of health check results.
func (r *Results) Format() string {
	var sb strings.Builder

	sb.WriteString("Dependency Health Check\n")
	sb.WriteString("═══════════════════════════════════════\n")

	for _, result := range r.All {
		if result.Available {
			sb.WriteString(fmt.Sprintf("  ✅ %s: %s\n", result.Name, result.Version))
		} else {
			sb.WriteString(fmt.Sprintf("  ❌ %s: not found\n", result.Name))
		}
	}

	sb.WriteString("═══════════════════════════════════════\n")

	if r.Passed {
		sb.WriteString("All required dependencies available.\n")
	} else {
		sb.WriteString(fmt.Sprintf("Missing required: %s\n", strings.Join(r.Missing, ", ")))
	}

	return sb.String()
}

// Error returns an error if any required dependencies are missing.
func (r *Results) Error() error {
	if r.Passed {
		return nil
	}
	return fmt.Errorf("missing required dependencies: %s", strings.Join(r.Missing, ", "))
}
