// Package executor handles AI-powered task execution using Claude CLI.
// It takes a ticket and executes the development work described.
package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/handshake/boatmanmode/internal/claude"
	"github.com/handshake/boatmanmode/internal/handoff"
	"github.com/handshake/boatmanmode/internal/linear"
)

// Executor performs AI-powered development tasks.
type Executor struct {
	client       *claude.Client
	worktreePath string
}

// ExecutionResult represents the outcome of task execution.
type ExecutionResult struct {
	Success      bool
	FilesChanged []string
	Summary      string
	Error        error
}

// New creates a new Executor.
func New(worktreePath string) *Executor {
	return &Executor{
		client:       claude.NewWithTmux(worktreePath, "executor"),
		worktreePath: worktreePath,
	}
}

// NewRefactorExecutor creates an executor for a refactor iteration.
func NewRefactorExecutor(worktreePath string, iteration int) *Executor {
	sessionName := fmt.Sprintf("refactor-%d", iteration)
	return &Executor{
		client:       claude.NewWithTmux(worktreePath, sessionName),
		worktreePath: worktreePath,
	}
}

// Execute performs the development task described in the ticket.
func (e *Executor) Execute(ctx context.Context, ticket *linear.Ticket) (*ExecutionResult, error) {
	fmt.Println("   📖 Building execution prompt from ticket...")
	prompt := e.buildPrompt(ticket)

	systemPrompt := `You are an expert software developer. Execute the given task by generating code.

CRITICAL RULES:
1. You MUST output actual file contents - never just describe what files to create
2. You MUST use the exact format below for EVERY file
3. Do NOT ask for permissions - just output the code
4. Do NOT say "I'll provide files" - actually provide them
5. If implementation exists, add/modify tests or make improvements as needed

OUTPUT FORMAT (follow exactly):

## Analysis
One paragraph explaining your approach.

## Changes

### FILE: path/to/file.rb
` + "```ruby" + `
# Complete file contents here - NOT a description, actual code
class MyClass
  def my_method
    # implementation
  end
end
` + "```" + `

### FILE: path/to/another_file.rb
` + "```ruby" + `
# Another complete file
` + "```" + `

REMEMBER: Every ### FILE: block MUST contain actual code inside the code fence, not a description of what code to write.`

	fmt.Println("   🤖 Sending task to Claude (streaming)...")
	fmt.Printf("   📝 Prompt size: %d chars\n", len(prompt))
	
	start := time.Now()
	response, err := e.client.Message(ctx, systemPrompt, prompt)
	elapsed := time.Since(start)
	
	if err != nil {
		return nil, fmt.Errorf("failed to call Claude: %w", err)
	}

	fmt.Printf("   ⏱️  Claude responded in %s\n", elapsed.Round(time.Second))
	fmt.Printf("   📄 Response size: %d chars\n", len(response))

	// Parse and apply file changes
	fmt.Println("   📦 Parsing response and extracting file changes...")
	filesChanged, err := e.parseAndApplyChanges(response)
	if err != nil {
		return &ExecutionResult{
			Success: false,
			Error:   err,
		}, nil
	}

	if len(filesChanged) == 0 {
		// No files found - show what Claude returned for debugging
		fmt.Println("   ⚠️  No files were extracted from response!")
		fmt.Println("   📋 Response preview:")
		preview := response
		if len(preview) > 1500 {
			preview = preview[:1500] + "\n... (truncated)"
		}
		for _, line := range strings.Split(preview, "\n") {
			fmt.Printf("      │ %s\n", line)
		}
		return &ExecutionResult{
			Success: false,
			Error:   fmt.Errorf("claude did not produce any file changes - check response format"),
		}, nil
	}

	fmt.Printf("   ✏️  Writing %d files to worktree\n", len(filesChanged))
	for _, f := range filesChanged {
		fmt.Printf("      • %s\n", f)
	}

	return &ExecutionResult{
		Success:      true,
		FilesChanged: filesChanged,
		Summary:      extractSummary(response),
	}, nil
}

// Refactor applies feedback from ScottBott to improve the code.
func (e *Executor) Refactor(ctx context.Context, ticket *linear.Ticket, reviewFeedback string, changedFiles []string) (*ExecutionResult, error) {
	fmt.Println("   📖 Reading changed files...")
	currentFiles, err := e.GetSpecificFiles(changedFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to read current files: %w", err)
	}
	fmt.Printf("   📁 Loaded %d changed files\n", len(changedFiles))

	prompt := fmt.Sprintf(`## Original Ticket
%s

## Current Implementation
%s

## Review Feedback (MUST ADDRESS)
%s

Please refactor the code to address all the feedback. Provide complete updated files.`, 
		e.buildPrompt(ticket), 
		currentFiles,
		reviewFeedback)

	systemPrompt := `You are refactoring code based on peer review feedback.
Address ALL issues raised in the review.
Maintain the original functionality while improving code quality.

Format your response with complete file contents:

### FILE: path/to/file.go
` + "```go" + `
// Full updated file contents
` + "```"

	fmt.Println("   🤖 Sending refactor request to Claude...")
	fmt.Printf("   📝 Prompt size: %d chars\n", len(prompt))

	start := time.Now()
	response, err := e.client.Message(ctx, systemPrompt, prompt)
	elapsed := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("failed to call Claude: %w", err)
	}

	fmt.Printf("   ⏱️  Claude responded in %s\n", elapsed.Round(time.Second))

	fmt.Println("   📦 Applying refactored changes...")
	filesChanged, err := e.parseAndApplyChanges(response)
	if err != nil {
		return &ExecutionResult{
			Success: false,
			Error:   err,
		}, nil
	}

	fmt.Printf("   ✏️  Updated %d files\n", len(filesChanged))
	for _, f := range filesChanged {
		fmt.Printf("      • %s\n", f)
	}

	return &ExecutionResult{
		Success:      true,
		FilesChanged: filesChanged,
		Summary:      "Refactored based on review feedback",
	}, nil
}

// RefactorWithHandoff uses a structured handoff for refactoring.
func (e *Executor) RefactorWithHandoff(ctx context.Context, h *handoff.RefactorHandoff) (*ExecutionResult, error) {
	prompt := h.ToPrompt()
	
	systemPrompt := `You are refactoring code based on peer review feedback.
Address ALL listed issues. Maintain functionality while improving quality.
Output complete updated files using the specified format.`

	fmt.Printf("   📝 Handoff: %d issues, %d files\n", len(h.Issues), len(h.FilesToUpdate))
	fmt.Println("   🤖 Sending refactor request...")

	start := time.Now()
	response, err := e.client.Message(ctx, systemPrompt, prompt)
	elapsed := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("failed to call Claude: %w", err)
	}

	fmt.Printf("   ⏱️  Completed in %s\n", elapsed.Round(time.Second))

	filesChanged, err := e.parseAndApplyChanges(response)
	if err != nil {
		return &ExecutionResult{Success: false, Error: err}, nil
	}

	fmt.Printf("   ✏️  Updated %d files\n", len(filesChanged))
	for _, f := range filesChanged {
		fmt.Printf("      • %s\n", f)
	}

	return &ExecutionResult{
		Success:      true,
		FilesChanged: filesChanged,
		Summary:      "Refactored based on review feedback",
	}, nil
}

// GetSpecificFiles reads specific files from the worktree (exported for handoff).
func (e *Executor) GetSpecificFiles(files []string) (string, error) {
	return e.getSpecificFiles(files)
}

// buildPrompt creates the execution prompt from a ticket.
func (e *Executor) buildPrompt(ticket *linear.Ticket) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", ticket.Title))
	sb.WriteString(fmt.Sprintf("**Ticket ID:** %s\n", ticket.Identifier))

	if len(ticket.Labels) > 0 {
		sb.WriteString(fmt.Sprintf("**Labels:** %s\n", strings.Join(ticket.Labels, ", ")))
	}

	sb.WriteString("\n## Description\n")
	sb.WriteString(ticket.Description)

	return sb.String()
}

// parseAndApplyChanges extracts file changes from response and writes them.
func (e *Executor) parseAndApplyChanges(response string) ([]string, error) {
	var filesChanged []string

	lines := strings.Split(response, "\n")
	var currentFile string
	var inCodeBlock bool
	var codeContent strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "### FILE:") {
			// Save previous file if any
			if currentFile != "" && codeContent.Len() > 0 {
				if err := e.writeFile(currentFile, codeContent.String()); err != nil {
					return filesChanged, err
				}
				filesChanged = append(filesChanged, currentFile)
			}

			currentFile = strings.TrimSpace(strings.TrimPrefix(line, "### FILE:"))
			codeContent.Reset()
			inCodeBlock = false
		} else if strings.HasPrefix(line, "```") && currentFile != "" {
			if inCodeBlock {
				// End of code block
				inCodeBlock = false
			} else {
				// Start of code block
				inCodeBlock = true
			}
		} else if inCodeBlock {
			codeContent.WriteString(line)
			codeContent.WriteString("\n")
		}
	}

	// Don't forget the last file
	if currentFile != "" && codeContent.Len() > 0 {
		if err := e.writeFile(currentFile, codeContent.String()); err != nil {
			return filesChanged, err
		}
		filesChanged = append(filesChanged, currentFile)
	}

	return filesChanged, nil
}

// writeFile writes content to a file in the worktree.
func (e *Executor) writeFile(relativePath, content string) error {
	fullPath := filepath.Join(e.worktreePath, relativePath)

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return os.WriteFile(fullPath, []byte(content), 0644)
}

// getSpecificFiles reads specific files from the worktree.
func (e *Executor) getSpecificFiles(files []string) (string, error) {
	var sb strings.Builder

	for _, relPath := range files {
		fullPath := filepath.Join(e.worktreePath, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			fmt.Printf("   ⚠️  Could not read %s: %v\n", relPath, err)
			continue
		}

		sb.WriteString(fmt.Sprintf("### FILE: %s\n```\n%s\n```\n\n", relPath, string(content)))
	}

	return sb.String(), nil
}

// GetDiff returns the git diff for the worktree.
func (e *Executor) GetDiff() (string, error) {
	// First try diff against HEAD
	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = e.worktreePath
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		return string(output), nil
	}

	// Try diff of staged changes
	cmd = exec.Command("git", "diff", "--cached")
	cmd.Dir = e.worktreePath
	output, err = cmd.Output()
	if err == nil && len(output) > 0 {
		return string(output), nil
	}

	// Try diff of unstaged changes
	cmd = exec.Command("git", "diff")
	cmd.Dir = e.worktreePath
	output, err = cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get diff: %w", err)
	}

	return string(output), nil
}

// StageChanges stages all changes in the worktree.
func (e *Executor) StageChanges() error {
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = e.worktreePath
	return cmd.Run()
}

// Commit creates a commit with the given message.
func (e *Executor) Commit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = e.worktreePath
	return cmd.Run()
}

// Push pushes the branch to origin.
func (e *Executor) Push(branchName string) error {
	cmd := exec.Command("git", "push", "-u", "origin", branchName)
	cmd.Dir = e.worktreePath
	return cmd.Run()
}

// isSourceFile checks if the file extension indicates source code.
func isSourceFile(ext string) bool {
	sourceExts := map[string]bool{
		".go":   true,
		".js":   true,
		".ts":   true,
		".tsx":  true,
		".jsx":  true,
		".py":   true,
		".rb":   true,
		".rs":   true,
		".java": true,
		".c":    true,
		".cpp":  true,
		".h":    true,
		".hpp":  true,
		".md":   true,
		".yaml": true,
		".yml":  true,
		".json": true,
		".toml": true,
	}
	return sourceExts[ext]
}

// extractSummary extracts the analysis section from the response.
func extractSummary(response string) string {
	lines := strings.Split(response, "\n")
	var inAnalysis bool
	var summary strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "## Analysis") {
			inAnalysis = true
			continue
		}
		if strings.HasPrefix(line, "## ") && inAnalysis {
			break
		}
		if inAnalysis {
			summary.WriteString(line)
			summary.WriteString("\n")
		}
	}

	return strings.TrimSpace(summary.String())
}
