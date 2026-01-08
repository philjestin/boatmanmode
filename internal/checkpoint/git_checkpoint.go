// Package checkpoint provides git-integrated checkpoint capabilities.
// Checkpoints are persisted as git commits for durability and rollback.
package checkpoint

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GitCheckpointManager extends the standard manager with git integration.
// Each checkpoint creates a git commit, enabling git-based rollback and history.
type GitCheckpointManager struct {
	*Manager
	worktreePath string
	useGit       bool
	commitPrefix string // Prefix for checkpoint commit messages
}

// GitCheckpointOptions configures git checkpoint behavior.
type GitCheckpointOptions struct {
	// WorktreePath is the git worktree to commit to
	WorktreePath string
	// UseGit enables git commit integration
	UseGit bool
	// CommitPrefix is prepended to checkpoint commit messages (default: "[checkpoint]")
	CommitPrefix string
	// BaseDir for JSON checkpoint storage (fallback)
	BaseDir string
}

// NewGitCheckpointManager creates a git-integrated checkpoint manager.
func NewGitCheckpointManager(opts GitCheckpointOptions) (*GitCheckpointManager, error) {
	baseMgr, err := NewManager(opts.BaseDir)
	if err != nil {
		return nil, err
	}

	prefix := opts.CommitPrefix
	if prefix == "" {
		prefix = "[checkpoint]"
	}

	mgr := &GitCheckpointManager{
		Manager:      baseMgr,
		worktreePath: opts.WorktreePath,
		useGit:       opts.UseGit,
		commitPrefix: prefix,
	}

	return mgr, nil
}

// SetWorktreePath updates the worktree path for git operations.
func (g *GitCheckpointManager) SetWorktreePath(path string) {
	g.worktreePath = path
	if g.Current != nil {
		g.Current.WorktreePath = path
	}
}

// CommitCheckpoint creates a git commit with the current checkpoint state.
func (g *GitCheckpointManager) CommitCheckpoint(message string) error {
	if !g.useGit || g.worktreePath == "" {
		return nil // Git not enabled or no worktree
	}

	if g.Current == nil {
		return fmt.Errorf("no active checkpoint")
	}

	// Write checkpoint state to a file in the worktree
	stateFile := filepath.Join(g.worktreePath, ".boatman-state.json")
	data, err := json.MarshalIndent(g.Current, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	// Stage and commit
	commitMsg := fmt.Sprintf("%s %s: %s (step: %s, iter: %d)",
		g.commitPrefix,
		g.Current.TicketID,
		message,
		g.Current.CurrentStep,
		g.Current.Iteration,
	)

	if err := g.gitExec("add", "-A"); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Check if there are changes to commit
	if !g.hasChanges() {
		return nil // Nothing to commit
	}

	if err := g.gitExec("commit", "-m", commitMsg, "--allow-empty"); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	return nil
}

// BeginStepWithCommit marks step start and creates a git commit.
func (g *GitCheckpointManager) BeginStepWithCommit(step Step) error {
	g.BeginStep(step)
	return g.CommitCheckpoint(fmt.Sprintf("begin %s", step))
}

// CompleteStepWithCommit marks step complete and creates a git commit.
func (g *GitCheckpointManager) CompleteStepWithCommit(step Step, output interface{}) error {
	g.CompleteStep(step, output)
	return g.CommitCheckpoint(fmt.Sprintf("complete %s", step))
}

// FailStepWithCommit marks step failed and creates a git commit.
func (g *GitCheckpointManager) FailStepWithCommit(step Step, err error) error {
	g.FailStep(step, err)
	return g.CommitCheckpoint(fmt.Sprintf("failed %s: %s", step, err.Error()))
}

// Rollback reverts to a previous checkpoint using git reset.
func (g *GitCheckpointManager) Rollback(steps int) error {
	if !g.useGit || g.worktreePath == "" {
		return fmt.Errorf("git integration not enabled")
	}

	// Find the target commit
	target := fmt.Sprintf("HEAD~%d", steps)

	// Hard reset to that commit
	if err := g.gitExec("reset", "--hard", target); err != nil {
		return fmt.Errorf("git reset failed: %w", err)
	}

	// Reload checkpoint state from the state file
	return g.reloadStateFromGit()
}

// RollbackToStep reverts to the last commit for a specific step.
func (g *GitCheckpointManager) RollbackToStep(step Step) error {
	if !g.useGit || g.worktreePath == "" {
		return fmt.Errorf("git integration not enabled")
	}

	// Find the commit for this step
	commitHash, err := g.findCommitForStep(step)
	if err != nil {
		return err
	}

	// Reset to that commit
	if err := g.gitExec("reset", "--hard", commitHash); err != nil {
		return fmt.Errorf("git reset failed: %w", err)
	}

	return g.reloadStateFromGit()
}

// RollbackToIteration reverts to a specific iteration.
func (g *GitCheckpointManager) RollbackToIteration(iteration int) error {
	if !g.useGit || g.worktreePath == "" {
		return fmt.Errorf("git integration not enabled")
	}

	// Find the commit for this iteration
	commitHash, err := g.findCommitForIteration(iteration)
	if err != nil {
		return err
	}

	// Reset to that commit
	if err := g.gitExec("reset", "--hard", commitHash); err != nil {
		return fmt.Errorf("git reset failed: %w", err)
	}

	return g.reloadStateFromGit()
}

// GitHistory represents a checkpoint from git history.
type GitHistory struct {
	CommitHash string    `json:"commit_hash"`
	Message    string    `json:"message"`
	Step       Step      `json:"step"`
	Iteration  int       `json:"iteration"`
	Timestamp  time.Time `json:"timestamp"`
}

// GetGitHistory returns the git commit history for checkpoints.
func (g *GitCheckpointManager) GetGitHistory() ([]GitHistory, error) {
	if !g.useGit || g.worktreePath == "" {
		return nil, fmt.Errorf("git integration not enabled")
	}

	// Get log with checkpoint commits only
	output, err := g.gitOutput("log", "--oneline", "--grep", g.commitPrefix, "--format=%H|%s|%ai")
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	var history []GitHistory
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}

		entry := GitHistory{
			CommitHash: parts[0],
			Message:    parts[1],
		}

		// Parse timestamp
		if ts, err := time.Parse("2006-01-02 15:04:05 -0700", parts[2]); err == nil {
			entry.Timestamp = ts
		}

		// Extract step and iteration from message
		entry.Step, entry.Iteration = g.parseCommitMessage(parts[1])

		history = append(history, entry)
	}

	return history, nil
}

// GetCheckpointAtCommit retrieves checkpoint state at a specific commit.
func (g *GitCheckpointManager) GetCheckpointAtCommit(commitHash string) (*Checkpoint, error) {
	if !g.useGit || g.worktreePath == "" {
		return nil, fmt.Errorf("git integration not enabled")
	}

	// Read the state file at that commit
	output, err := g.gitOutput("show", fmt.Sprintf("%s:.boatman-state.json", commitHash))
	if err != nil {
		return nil, fmt.Errorf("failed to read state at commit: %w", err)
	}

	var cp Checkpoint
	if err := json.Unmarshal([]byte(output), &cp); err != nil {
		return nil, fmt.Errorf("failed to parse checkpoint: %w", err)
	}

	return &cp, nil
}

// CompareCheckpoints shows the diff between two checkpoint commits.
func (g *GitCheckpointManager) CompareCheckpoints(commit1, commit2 string) (string, error) {
	if !g.useGit || g.worktreePath == "" {
		return "", fmt.Errorf("git integration not enabled")
	}

	output, err := g.gitOutput("diff", commit1, commit2, "--stat")
	if err != nil {
		return "", fmt.Errorf("git diff failed: %w", err)
	}

	return output, nil
}

// CreateSnapshotBranch creates a branch at the current checkpoint.
func (g *GitCheckpointManager) CreateSnapshotBranch(name string) error {
	if !g.useGit || g.worktreePath == "" {
		return fmt.Errorf("git integration not enabled")
	}

	branchName := fmt.Sprintf("checkpoint/%s/%s", g.Current.TicketID, name)
	return g.gitExec("branch", branchName)
}

// ListSnapshotBranches lists all checkpoint branches.
func (g *GitCheckpointManager) ListSnapshotBranches() ([]string, error) {
	if !g.useGit || g.worktreePath == "" {
		return nil, fmt.Errorf("git integration not enabled")
	}

	output, err := g.gitOutput("branch", "--list", "checkpoint/*")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var branches []string
	for _, line := range lines {
		branch := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if branch != "" {
			branches = append(branches, branch)
		}
	}

	return branches, nil
}

// RestoreFromBranch restores state from a snapshot branch.
func (g *GitCheckpointManager) RestoreFromBranch(branchName string) error {
	if !g.useGit || g.worktreePath == "" {
		return fmt.Errorf("git integration not enabled")
	}

	// Cherry-pick or reset to the branch
	if err := g.gitExec("checkout", branchName, "--", "."); err != nil {
		return fmt.Errorf("failed to restore from branch: %w", err)
	}

	return g.reloadStateFromGit()
}

// CleanupOldCheckpoints removes checkpoint commits older than the given duration.
func (g *GitCheckpointManager) CleanupOldCheckpoints(age time.Duration) error {
	// This is informational - we don't actually remove git history
	// but we can squash old checkpoint commits if needed

	history, err := g.GetGitHistory()
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-age)
	var toCleanup []GitHistory
	for _, h := range history {
		if h.Timestamp.Before(cutoff) {
			toCleanup = append(toCleanup, h)
		}
	}

	if len(toCleanup) > 0 {
		fmt.Printf("Found %d old checkpoint commits that could be squashed\n", len(toCleanup))
	}

	return nil
}

// FormatGitHistory returns a formatted string of git checkpoint history.
func (g *GitCheckpointManager) FormatGitHistory() (string, error) {
	history, err := g.GetGitHistory()
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("# Checkpoint History (Git)\n\n")

	for i, h := range history {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, h.CommitHash[:8], h.Message))
		sb.WriteString(fmt.Sprintf("   Step: %s, Iteration: %d\n", h.Step, h.Iteration))
		sb.WriteString(fmt.Sprintf("   Time: %s\n\n", h.Timestamp.Format(time.RFC3339)))
	}

	return sb.String(), nil
}

// --- Internal helpers ---

func (g *GitCheckpointManager) gitExec(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.worktreePath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (g *GitCheckpointManager) gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.worktreePath
	output, err := cmd.Output()
	return string(output), err
}

func (g *GitCheckpointManager) hasChanges() bool {
	output, err := g.gitOutput("status", "--porcelain")
	if err != nil {
		return false
	}
	return strings.TrimSpace(output) != ""
}

func (g *GitCheckpointManager) reloadStateFromGit() error {
	stateFile := filepath.Join(g.worktreePath, ".boatman-state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return fmt.Errorf("failed to read state file: %w", err)
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return fmt.Errorf("failed to parse checkpoint: %w", err)
	}

	g.Current = &cp
	return nil
}

func (g *GitCheckpointManager) findCommitForStep(step Step) (string, error) {
	output, err := g.gitOutput("log", "--oneline", "--grep", fmt.Sprintf("%s.*%s", g.commitPrefix, step), "-1", "--format=%H")
	if err != nil {
		return "", err
	}

	hash := strings.TrimSpace(output)
	if hash == "" {
		return "", fmt.Errorf("no commit found for step %s", step)
	}

	return hash, nil
}

func (g *GitCheckpointManager) findCommitForIteration(iteration int) (string, error) {
	output, err := g.gitOutput("log", "--oneline", "--grep", fmt.Sprintf("%s.*iter: %d", g.commitPrefix, iteration), "-1", "--format=%H")
	if err != nil {
		return "", err
	}

	hash := strings.TrimSpace(output)
	if hash == "" {
		return "", fmt.Errorf("no commit found for iteration %d", iteration)
	}

	return hash, nil
}

func (g *GitCheckpointManager) parseCommitMessage(msg string) (Step, int) {
	// Parse step from message like "[checkpoint] ENG-123: complete execution (step: execution, iter: 1)"
	stepRe := regexp.MustCompile(`step: (\w+)`)
	iterRe := regexp.MustCompile(`iter: (\d+)`)

	var step Step
	var iteration int

	if matches := stepRe.FindStringSubmatch(msg); len(matches) > 1 {
		step = Step(matches[1])
	}

	if matches := iterRe.FindStringSubmatch(msg); len(matches) > 1 {
		if n, err := strconv.Atoi(matches[1]); err == nil {
			iteration = n
		}
	}

	return step, iteration
}

// --- Utility functions ---

// SquashCheckpoints squashes multiple checkpoint commits into one.
// This is useful for cleaning up history before creating a PR.
func (g *GitCheckpointManager) SquashCheckpoints(message string) error {
	if !g.useGit || g.worktreePath == "" {
		return fmt.Errorf("git integration not enabled")
	}

	// Find the first checkpoint commit
	output, err := g.gitOutput("log", "--oneline", "--grep", g.commitPrefix, "--reverse", "-1", "--format=%H")
	if err != nil {
		return err
	}

	firstCommit := strings.TrimSpace(output)
	if firstCommit == "" {
		return fmt.Errorf("no checkpoint commits found")
	}

	// Get the parent of the first checkpoint
	parentOutput, err := g.gitOutput("rev-parse", firstCommit+"^")
	if err != nil {
		// No parent means it's the root commit
		return fmt.Errorf("cannot squash: first checkpoint is at or near root")
	}

	parent := strings.TrimSpace(parentOutput)

	// Soft reset to parent
	if err := g.gitExec("reset", "--soft", parent); err != nil {
		return fmt.Errorf("soft reset failed: %w", err)
	}

	// Commit everything as one
	if err := g.gitExec("commit", "-m", message); err != nil {
		return fmt.Errorf("squash commit failed: %w", err)
	}

	return nil
}

// ExportHistory exports checkpoint history to a file.
func (g *GitCheckpointManager) ExportHistory(outputPath string) error {
	history, err := g.GetGitHistory()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, data, 0644)
}
