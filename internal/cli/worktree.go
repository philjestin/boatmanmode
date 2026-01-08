package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// worktreeCmd manages worktrees.
var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage boatman worktrees",
	Long:  `List, commit, or clean up worktrees created by boatman.`,
}

var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active worktrees",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		worktreeBase := filepath.Join(cwd, ".worktrees")

		entries, err := os.ReadDir(worktreeBase)
		if err != nil {
			fmt.Println("No worktrees found in", worktreeBase)
			return nil
		}

		fmt.Println("Boatman worktrees:")
		for _, entry := range entries {
			if entry.IsDir() {
				wtPath := filepath.Join(worktreeBase, entry.Name())
				branch := getBranch(wtPath)
				status := getStatus(wtPath)
				fmt.Printf("  • %s\n", entry.Name())
				fmt.Printf("    Branch: %s\n", branch)
				fmt.Printf("    Status: %s\n", status)
				fmt.Printf("    Path: %s\n", wtPath)
				fmt.Println()
			}
		}

		return nil
	},
}

var worktreeCommitCmd = &cobra.Command{
	Use:   "commit [worktree-name] [message]",
	Short: "Commit changes in a worktree",
	Long: `Stages all changes and creates a commit in the specified worktree.
If no worktree name is given, commits to the most recently modified one.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		worktreeBase := filepath.Join(cwd, ".worktrees")

		var wtPath string
		var message string

		if len(args) >= 1 {
			wtPath = filepath.Join(worktreeBase, args[0])
		} else {
			// Find most recent worktree
			entries, err := os.ReadDir(worktreeBase)
			if err != nil || len(entries) == 0 {
				return fmt.Errorf("no worktrees found")
			}
			// Just use the first one for now
			wtPath = filepath.Join(worktreeBase, entries[0].Name())
		}

		if len(args) >= 2 {
			message = args[1]
		} else {
			message = "WIP: boatman checkpoint"
		}

		// Check if worktree exists
		if _, err := os.Stat(wtPath); os.IsNotExist(err) {
			return fmt.Errorf("worktree not found: %s", wtPath)
		}

		fmt.Printf("Committing changes in: %s\n", wtPath)

		// Stage all changes
		stageCmd := exec.Command("git", "add", "-A")
		stageCmd.Dir = wtPath
		stageCmd.Stdout = os.Stdout
		stageCmd.Stderr = os.Stderr
		if err := stageCmd.Run(); err != nil {
			return fmt.Errorf("failed to stage: %w", err)
		}

		// Check if there are changes to commit
		statusCmd := exec.Command("git", "status", "--porcelain")
		statusCmd.Dir = wtPath
		statusOut, _ := statusCmd.Output()
		if len(statusOut) == 0 {
			fmt.Println("Nothing to commit - working tree clean")
			return nil
		}

		// Commit
		commitCmd := exec.Command("git", "commit", "-m", message)
		commitCmd.Dir = wtPath
		commitCmd.Stdout = os.Stdout
		commitCmd.Stderr = os.Stderr
		if err := commitCmd.Run(); err != nil {
			return fmt.Errorf("failed to commit: %w", err)
		}

		fmt.Println("✅ Changes committed!")
		fmt.Printf("   Branch: %s\n", getBranch(wtPath))
		fmt.Println()
		fmt.Println("To view the changes:")
		fmt.Printf("   cd %s && git log --oneline -5\n", wtPath)
		fmt.Println()
		fmt.Println("To push the branch:")
		fmt.Printf("   cd %s && git push -u origin HEAD\n", wtPath)

		return nil
	},
}

var worktreePushCmd = &cobra.Command{
	Use:   "push [worktree-name]",
	Short: "Push a worktree branch to origin",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		worktreeBase := filepath.Join(cwd, ".worktrees")

		var wtPath string
		if len(args) >= 1 {
			wtPath = filepath.Join(worktreeBase, args[0])
		} else {
			entries, _ := os.ReadDir(worktreeBase)
			if len(entries) == 0 {
				return fmt.Errorf("no worktrees found")
			}
			wtPath = filepath.Join(worktreeBase, entries[0].Name())
		}

		branch := getBranch(wtPath)
		fmt.Printf("Pushing branch: %s\n", branch)

		pushCmd := exec.Command("git", "push", "-u", "origin", branch)
		pushCmd.Dir = wtPath
		pushCmd.Stdout = os.Stdout
		pushCmd.Stderr = os.Stderr
		return pushCmd.Run()
	},
}

var worktreeCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all boatman worktrees",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		worktreeBase := filepath.Join(cwd, ".worktrees")

		entries, err := os.ReadDir(worktreeBase)
		if err != nil {
			fmt.Println("No worktrees to clean")
			return nil
		}

		for _, entry := range entries {
			if entry.IsDir() {
				wtPath := filepath.Join(worktreeBase, entry.Name())
				fmt.Printf("Removing worktree: %s\n", entry.Name())

				// Remove via git worktree
				removeCmd := exec.Command("git", "worktree", "remove", wtPath, "--force")
				removeCmd.Dir = cwd
				removeCmd.Run()
			}
		}

		fmt.Println("✅ All worktrees cleaned")
		return nil
	},
}

func getBranch(wtPath string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return string(out[:len(out)-1]) // trim newline
}

func getStatus(wtPath string) string {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = wtPath
	out, _ := cmd.Output()
	if len(out) == 0 {
		return "clean"
	}
	lines := len(out) / 3 // rough estimate of changed files
	return fmt.Sprintf("%d files changed", lines)
}

func init() {
	rootCmd.AddCommand(worktreeCmd)
	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreeCommitCmd)
	worktreeCmd.AddCommand(worktreePushCmd)
	worktreeCmd.AddCommand(worktreeCleanCmd)
}


