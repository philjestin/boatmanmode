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

	"github.com/philjestin/boatmanmode/internal/claude"
	"github.com/philjestin/boatmanmode/internal/handoff"
	"github.com/philjestin/boatmanmode/internal/linear"
	"github.com/philjestin/boatmanmode/internal/planner"
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
func New(worktreePath string, enableTools bool) *Executor {
	var client *claude.Client
	if enableTools {
		// Full toolset for development: Read, Edit, Bash, Grep, Glob
		client = claude.NewWithTools(worktreePath, "executor", nil) // nil = allow all tools
	} else {
		// Backward compatibility - no tools
		client = claude.NewWithTmux(worktreePath, "executor")
	}
	return &Executor{
		client:       client,
		worktreePath: worktreePath,
	}
}

// NewRefactorExecutor creates an executor for a refactor iteration.
func NewRefactorExecutor(worktreePath string, iteration int, enableTools bool) *Executor {
	sessionName := fmt.Sprintf("refactor-%d", iteration)
	var client *claude.Client
	if enableTools {
		// Full toolset for refactoring: Read, Edit, Bash, Grep, Glob
		client = claude.NewWithTools(worktreePath, sessionName, nil) // nil = allow all tools
	} else {
		// Backward compatibility - no tools
		client = claude.NewWithTmux(worktreePath, sessionName)
	}
	return &Executor{
		client:       client,
		worktreePath: worktreePath,
	}
}

// Execute performs the development task described in the ticket.
func (e *Executor) Execute(ctx context.Context, ticket *linear.Ticket) (*ExecutionResult, error) {
	// Use planning agent to analyze the ticket first
	return e.ExecuteWithPlan(ctx, ticket, nil)
}

// ExecuteWithPlan performs execution with an optional pre-computed plan.
func (e *Executor) ExecuteWithPlan(ctx context.Context, ticket *linear.Ticket, plan *planner.Plan) (*ExecutionResult, error) {
	// Build prompt with ticket
	fmt.Println("   📖 Building execution prompt...")
	prompt := e.buildPrompt(ticket)

	// Add planning handoff if available
	if plan != nil {
		prompt += "\n\n---\n\n" + plan.ToHandoff()
		fmt.Printf("   📋 Added plan handoff (%d files, %d steps)\n", len(plan.RelevantFiles), len(plan.Approach))
	}

	// Load project rules (like Cursor does)
	projectRules := e.LoadProjectRules()

	// Build system prompt with project rules
	systemPrompt := `You are an expert software developer with access to code exploration and editing tools.

Your development workflow:
1. Use Grep/Glob to find relevant files
2. Use Read to understand existing code
3. Use Edit to make changes
4. Use Bash to run tests and verify your changes
5. Iterate until tests pass

Execute the development task described. Do not ask for permission - just implement and verify the solution.

You have been given a plan from a planning agent. Follow the approach and read the key files first.
If implementation already exists, add tests or make improvements as needed.`

	if projectRules != "" {
		systemPrompt = projectRules + "\n\n---\n\n" + systemPrompt
	}

	// Phase 3: Execute with Claude
	fmt.Println("   🤖 Phase 3: Executing with Claude...")
	fmt.Printf("   📝 Prompt size: %d chars\n", len(prompt))
	
	start := time.Now()
	response, err := e.client.Message(ctx, systemPrompt, prompt)
	elapsed := time.Since(start)
	
	if err != nil {
		return nil, fmt.Errorf("failed to call Claude: %w", err)
	}

	fmt.Printf("   ⏱️  Claude responded in %s\n", elapsed.Round(time.Second))
	fmt.Printf("   📄 Response size: %d chars\n", len(response))

	// Claude in agentic mode writes files directly - detect what changed via git
	fmt.Println("   📦 Detecting file changes in worktree...")
	filesChanged, err := e.detectChangedFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to detect changes: %w", err)
	}

	if len(filesChanged) == 0 {
		// No files changed - show Claude's response for debugging
		fmt.Println("   ⚠️  No files were changed in the worktree!")
		fmt.Println("   📋 Claude's response:")
		preview := response
		if len(preview) > 2000 {
			preview = preview[:2000] + "\n... (truncated)"
		}
		for _, line := range strings.Split(preview, "\n") {
			fmt.Printf("      │ %s\n", line)
		}
		return &ExecutionResult{
			Success: false,
			Error:   fmt.Errorf("claude did not produce any file changes - check response above"),
		}, nil
	}

	fmt.Printf("   ✏️  Claude modified %d files:\n", len(filesChanged))
	for _, f := range filesChanged {
		fmt.Printf("      • %s\n", f)
	}

	return &ExecutionResult{
		Success:      true,
		FilesChanged: filesChanged,
		Summary:      extractSummary(response),
	}, nil
}

// detectChangedFiles uses git status to find what files Claude modified.
func (e *Executor) detectChangedFiles() ([]string, error) {
	// Get list of changed files (staged, unstaged, and untracked)
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = e.worktreePath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	var files []string
	for _, line := range strings.Split(string(output), "\n") {
		// Don't TrimSpace - it removes the leading space from status like " M file"
		if len(line) < 4 {
			continue
		}
		
		// Format is "XY filename" where XY is 2 status chars + 1 space
		// Examples: " M packs/file.rb", "A  packs/file.rb", "?? packs/file.rb"
		file := line[3:]
		
		// Handle renamed files: "R  old -> new"
		if idx := strings.Index(file, " -> "); idx != -1 {
			file = file[idx+4:]
		}
		
		// Skip directories (end with /)
		if strings.HasSuffix(file, "/") {
			continue
		}
		
		// Verify it's a file, not a directory
		fullPath := filepath.Join(e.worktreePath, file)
		if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
			continue
		}
		
		files = append(files, file)
	}

	return files, nil
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

	// Build system prompt - emphasize following project rules
	systemPrompt := `You are refactoring code based on peer review feedback.

CRITICAL INSTRUCTIONS:
1. The handoff contains "Project Rules & Standards" - you MUST follow these exactly
2. The handoff contains "Original Requirements" from the ticket - ensure your fixes align with these
3. Address ALL listed issues while following the project rules
4. Maintain functionality while improving quality
5. Output complete updated files using the specified format

Common mistakes to avoid:
- Ignoring project-specific patterns (e.g., authorization error handling)
- Using errors-as-data when the rules say to raise exceptions (or vice versa)
- Missing required fields from the schema
- Not following the project's code organization conventions`

	fmt.Printf("   📝 Handoff: %d issues, %d files\n", len(h.Issues), len(h.FilesToUpdate))
	if h.ProjectRules != "" {
		fmt.Println("   📋 Project rules included in handoff")
	}
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

// LoadProjectRules loads project rules from various sources (like Cursor does).
// NOTE: Claude CLI automatically reads CLAUDE.md, so we skip that.
// We limit total size to avoid overwhelming the context.
// Exported so agent.go can use it for refactor handoffs.
func (e *Executor) LoadProjectRules() string {
	var rules strings.Builder
	rulesCount := 0
	maxSize := 50000 // 50KB max to avoid context bloat

	// 1. Check for .cursorrules (single file - highest priority)
	cursorrules := filepath.Join(e.worktreePath, ".cursorrules")
	if content, err := os.ReadFile(cursorrules); err == nil && len(content) < maxSize {
		rules.WriteString("# Cursor Rules (from .cursorrules)\n\n")
		rules.WriteString(string(content))
		rules.WriteString("\n\n")
		rulesCount++
	}

	// 2. Check for pack-specific CLAUDE.md (more focused than root)
	// Look for packs/*/CLAUDE.md that might be relevant
	packsDir := filepath.Join(e.worktreePath, "packs")
	if entries, err := os.ReadDir(packsDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			packClaude := filepath.Join(packsDir, entry.Name(), "CLAUDE.md")
			if content, err := os.ReadFile(packClaude); err == nil {
				if rules.Len()+len(content) < maxSize {
					rules.WriteString(fmt.Sprintf("# Pack Rules: %s/CLAUDE.md\n\n", entry.Name()))
					rules.WriteString(string(content))
					rules.WriteString("\n\n")
					rulesCount++
				}
				break // Only include first pack CLAUDE.md to limit size
			}
		}
	}

	if rulesCount > 0 {
		fmt.Printf("   📋 Loaded %d project rule file(s) (%d KB)\n", rulesCount, rules.Len()/1024)
	}

	return strings.TrimSpace(rules.String())
}

