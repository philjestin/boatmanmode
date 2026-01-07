// Package worktree manages git worktrees for isolated development.
package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Manager handles git worktree operations.
type Manager struct {
	repoPath     string
	worktreeBase string
}

// Worktree represents an active git worktree.
type Worktree struct {
	Path       string
	BranchName string
	BaseBranch string
}

// New creates a new worktree manager.
func New(repoPath string) (*Manager, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Verify it's a git repo
	if _, err := os.Stat(filepath.Join(absPath, ".git")); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", absPath)
	}

	worktreeBase := filepath.Join(absPath, ".worktrees")

	return &Manager{
		repoPath:     absPath,
		worktreeBase: worktreeBase,
	}, nil
}

// Create creates a new worktree for the given branch name.
// If the worktree/branch already exists, it reuses it.
func (m *Manager) Create(branchName, baseBranch string) (*Worktree, error) {
	// Sanitize branch name for filesystem
	safeBranchName := sanitizeBranchName(branchName)
	worktreePath := filepath.Join(m.worktreeBase, safeBranchName)

	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); err == nil {
		fmt.Printf("♻️  Reusing existing worktree: %s\n", worktreePath)
		return &Worktree{
			Path:       worktreePath,
			BranchName: branchName,
			BaseBranch: baseBranch,
		}, nil
	}

	// Ensure worktree base directory exists
	if err := os.MkdirAll(m.worktreeBase, 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree base: %w", err)
	}

	// Fetch latest from remote
	if err := m.runGit("fetch", "origin", baseBranch); err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}

	// Check if branch already exists
	branchExists := m.runGit("show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", branchName)) == nil

	if branchExists {
		// Branch exists but worktree doesn't - create worktree for existing branch
		if err := m.runGit("worktree", "add", worktreePath, branchName); err != nil {
			return nil, fmt.Errorf("failed to create worktree for existing branch: %w", err)
		}
	} else {
		// Create the worktree with a new branch
		if err := m.runGit("worktree", "add", "-b", branchName, worktreePath, fmt.Sprintf("origin/%s", baseBranch)); err != nil {
			return nil, fmt.Errorf("failed to create worktree: %w", err)
		}
	}

	return &Worktree{
		Path:       worktreePath,
		BranchName: branchName,
		BaseBranch: baseBranch,
	}, nil
}

// Remove removes a worktree and its branch.
func (m *Manager) Remove(wt *Worktree) error {
	if err := m.runGit("worktree", "remove", wt.Path, "--force"); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	// Optionally delete the branch
	if err := m.runGit("branch", "-D", wt.BranchName); err != nil {
		// Branch deletion failure is not critical
		fmt.Printf("Warning: failed to delete branch %s: %v\n", wt.BranchName, err)
	}

	return nil
}

// List returns all active worktrees.
func (m *Manager) List() ([]*Worktree, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = m.repoPath

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var worktrees []*Worktree
	lines := strings.Split(string(output), "\n")

	var currentPath, currentBranch string
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			currentBranch = strings.TrimPrefix(line, "branch refs/heads/")
		} else if line == "" && currentPath != "" {
			if strings.HasPrefix(currentPath, m.worktreeBase) {
				worktrees = append(worktrees, &Worktree{
					Path:       currentPath,
					BranchName: currentBranch,
				})
			}
			currentPath = ""
			currentBranch = ""
		}
	}

	return worktrees, nil
}

// runGit executes a git command in the repo directory.
func (m *Manager) runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.repoPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// sanitizeBranchName makes a branch name safe for filesystem use.
func sanitizeBranchName(name string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		" ", "-",
		":", "-",
	)
	return replacer.Replace(name)
}

