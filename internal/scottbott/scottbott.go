// Package scottbott provides the ScottBott peer-review integration.
// It invokes the existing peer-review Claude skill in the target repository.
package scottbott

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ReviewResult represents the outcome of a code review.
type ReviewResult struct {
	Passed   bool     `json:"passed"`
	Score    int      `json:"score"`
	Summary  string   `json:"summary"`
	Issues   []Issue  `json:"issues"`
	Praise   []string `json:"praise"`
	Guidance string   `json:"guidance"`
}

// Issue represents a specific problem found during review.
type Issue struct {
	Severity    string `json:"severity"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
}

// ScottBott invokes the peer-review skill.
type ScottBott struct {
	workDir     string
	sessionName string
	outputDir   string
}

// New creates a new ScottBott instance.
func New() *ScottBott {
	return &ScottBott{
		sessionName: "reviewer",
		outputDir:   filepath.Join(os.TempDir(), "boatman-sessions"),
	}
}

// NewForIteration creates a ScottBott for a specific review iteration.
func NewForIteration(iteration int) *ScottBott {
	return &ScottBott{
		sessionName: fmt.Sprintf("reviewer-%d", iteration),
		outputDir:   filepath.Join(os.TempDir(), "boatman-sessions"),
	}
}

// NewWithWorkDir creates a ScottBott that runs in a specific directory.
func NewWithWorkDir(workDir string, iteration int) *ScottBott {
	return &ScottBott{
		workDir:     workDir,
		sessionName: fmt.Sprintf("reviewer-%d", iteration),
		outputDir:   filepath.Join(os.TempDir(), "boatman-sessions"),
	}
}

// Review performs a code review using the peer-review Claude skill.
func (s *ScottBott) Review(ctx context.Context, ticketContext, diff string) (*ReviewResult, error) {
	os.MkdirAll(s.outputDir, 0755)

	// Write the review prompt to a file
	promptFile := filepath.Join(s.outputDir, fmt.Sprintf("%s-prompt.txt", s.sessionName))
	prompt := formatReviewPrompt(ticketContext, diff)
	if err := os.WriteFile(promptFile, []byte(prompt), 0644); err != nil {
		return nil, fmt.Errorf("failed to write prompt: %w", err)
	}
	defer os.Remove(promptFile)

	// Output file for capturing response
	outputFile := filepath.Join(s.outputDir, fmt.Sprintf("%s.out", s.sessionName))

	fmt.Printf("   📏 Review: %d chars context, %d chars diff\n", len(ticketContext), len(diff))
	fmt.Println("   🔍 Invoking peer-review skill...")

	start := time.Now()

	// Invoke Claude with the peer-review agent/skill
	// The peer-review skill should exist in the repo's .claude/ directory
	cmd := exec.CommandContext(ctx, "claude", 
		"-p",
		"--agent", "peer-review",  // Use the peer-review skill
		"--output-format", "text",
	)
	
	// Pipe the prompt via stdin
	promptContent, _ := os.ReadFile(promptFile)
	cmd.Stdin = strings.NewReader(string(promptContent))

	if s.workDir != "" {
		cmd.Dir = s.workDir
	}

	output, err := cmd.Output()
	elapsed := time.Since(start)

	if err != nil {
		// If peer-review skill doesn't exist, fall back to system prompt
		fmt.Println("   ⚠️  peer-review skill not found, using fallback...")
		return s.reviewWithFallback(ctx, ticketContext, diff)
	}

	fmt.Printf("   ⏱️  Review completed in %s\n", elapsed.Round(time.Second))

	// Save output for debugging
	os.WriteFile(outputFile, output, 0644)

	// Parse the response
	response := strings.TrimSpace(string(output))
	return parseReviewResponse(response)
}

// reviewWithFallback uses a system prompt if peer-review skill isn't available.
func (s *ScottBott) reviewWithFallback(ctx context.Context, ticketContext, diff string) (*ReviewResult, error) {
	systemPrompt := `You are a senior staff engineer conducting a peer code review.
Be thorough, constructive, and focused on correctness, security, and maintainability.

Respond with ONLY a JSON object:
{
  "passed": boolean,
  "score": number (0-100),
  "summary": "2-3 sentence summary",
  "issues": [{"severity": "critical|major|minor", "file": "path", "line": 0, "description": "what's wrong", "suggestion": "how to fix"}],
  "praise": ["good things"],
  "guidance": "guidance for fixing if failed"
}

Pass if: no critical issues, ≤2 major issues, code meets requirements.`

	prompt := formatReviewPrompt(ticketContext, diff)

	promptFile := filepath.Join(s.outputDir, fmt.Sprintf("%s-fallback-prompt.txt", s.sessionName))
	sysFile := filepath.Join(s.outputDir, fmt.Sprintf("%s-fallback-system.txt", s.sessionName))
	
	os.WriteFile(promptFile, []byte(prompt), 0644)
	os.WriteFile(sysFile, []byte(systemPrompt), 0644)
	defer os.Remove(promptFile)
	defer os.Remove(sysFile)

	start := time.Now()

	cmd := exec.CommandContext(ctx, "claude",
		"-p",
		"--output-format", "text",
		"--system-prompt", systemPrompt,
	)
	cmd.Stdin = strings.NewReader(prompt)

	if s.workDir != "" {
		cmd.Dir = s.workDir
	}

	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("review failed: %w\nOutput: %s", err, string(output))
	}

	fmt.Printf("   ⏱️  Review completed in %s\n", elapsed.Round(time.Second))

	return parseReviewResponse(strings.TrimSpace(string(output)))
}

// formatReviewPrompt creates the prompt for code review.
func formatReviewPrompt(ticketContext, diff string) string {
	return fmt.Sprintf(`## Ticket Context
%s

## Code Changes
%s

Review these changes against the requirements. Provide your assessment.`, ticketContext, diff)
}

// parseReviewResponse extracts ReviewResult from Claude's response.
func parseReviewResponse(response string) (*ReviewResult, error) {
	// Extract JSON from response (handle markdown code blocks)
	jsonStr := extractJSON(response)

	var result ReviewResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		// If JSON parsing fails, try to interpret as a pass/fail
		lower := strings.ToLower(response)
		if strings.Contains(lower, "lgtm") || strings.Contains(lower, "approved") {
			return &ReviewResult{
				Passed:  true,
				Score:   80,
				Summary: response[:min(200, len(response))],
			}, nil
		}
		return nil, fmt.Errorf("failed to parse review: %w\nResponse: %s", err, response[:min(500, len(response))])
	}

	return &result, nil
}

// extractJSON extracts JSON from a response that might be wrapped in markdown.
func extractJSON(text string) string {
	text = strings.TrimSpace(text)

	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		if idx := strings.LastIndex(text, "```"); idx != -1 {
			text = text[:idx]
		}
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		if idx := strings.LastIndex(text, "```"); idx != -1 {
			text = text[:idx]
		}
	}

	return strings.TrimSpace(text)
}

// FormatReview returns a human-readable format of the review.
func (r *ReviewResult) FormatReview() string {
	var sb strings.Builder

	sb.WriteString("   ┌─────────────────────────────────────────┐\n")
	if r.Passed {
		sb.WriteString("   │  ✅ REVIEW PASSED                       │\n")
	} else {
		sb.WriteString("   │  ❌ REVIEW FAILED                       │\n")
	}
	sb.WriteString("   └─────────────────────────────────────────┘\n")

	sb.WriteString(fmt.Sprintf("   📊 Score: %d/100\n\n", r.Score))
	sb.WriteString(fmt.Sprintf("   📝 Summary:\n      %s\n\n", r.Summary))

	if len(r.Praise) > 0 {
		sb.WriteString("   👍 What's good:\n")
		for _, p := range r.Praise {
			sb.WriteString(fmt.Sprintf("      • %s\n", p))
		}
		sb.WriteString("\n")
	}

	if len(r.Issues) > 0 {
		sb.WriteString("   🔍 Issues found:\n")
		for i, issue := range r.Issues {
			icon := "💡"
			switch issue.Severity {
			case "critical":
				icon = "🚨"
			case "major":
				icon = "⚠️"
			case "minor":
				icon = "📝"
			}

			sb.WriteString(fmt.Sprintf("      %d. %s [%s] %s\n", i+1, icon, strings.ToUpper(issue.Severity), issue.Description))
			if issue.File != "" {
				location := issue.File
				if issue.Line > 0 {
					location = fmt.Sprintf("%s:%d", issue.File, issue.Line)
				}
				sb.WriteString(fmt.Sprintf("         📍 %s\n", location))
			}
			if issue.Suggestion != "" {
				sb.WriteString(fmt.Sprintf("         💡 %s\n", issue.Suggestion))
			}
			sb.WriteString("\n")
		}
	}

	if r.Guidance != "" && !r.Passed {
		sb.WriteString("   📋 Guidance:\n")
		for _, line := range strings.Split(r.Guidance, "\n") {
			sb.WriteString(fmt.Sprintf("      %s\n", line))
		}
	}

	return sb.String()
}

// GetIssueDescriptions returns a list of issue descriptions for handoff.
func (r *ReviewResult) GetIssueDescriptions() []string {
	issues := make([]string, len(r.Issues))
	for i, issue := range r.Issues {
		issues[i] = fmt.Sprintf("[%s] %s", issue.Severity, issue.Description)
		if issue.Suggestion != "" {
			issues[i] += " → " + issue.Suggestion
		}
	}
	return issues
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
