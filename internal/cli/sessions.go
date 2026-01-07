package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/handshake/boatmanmode/internal/tmux"
	"github.com/spf13/cobra"
)

// sessionsCmd manages tmux sessions.
var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Manage agent tmux sessions",
	Long:  `List, attach to, or kill tmux sessions used by boatman agents.`,
}

var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active agent sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := tmux.NewManager("boatman")
		sessions, err := mgr.ListSessions()
		if err != nil {
			return err
		}

		if len(sessions) == 0 {
			fmt.Println("No active boatman sessions")
			return nil
		}

		fmt.Println("Active boatman sessions:")
		for _, s := range sessions {
			fmt.Printf("  • %s\n", s)
		}
		fmt.Println()
		fmt.Println("Watch with: boatman watch")
		fmt.Println("Attach with: tmux attach -t <session-name>")

		return nil
	},
}

var sessionsAttachCmd = &cobra.Command{
	Use:   "attach [session-name]",
	Short: "Attach to an agent session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := tmux.NewManager("boatman")
		sess := &tmux.Session{Name: args[0]}
		return mgr.AttachSession(sess)
	},
}

var sessionsKillCmd = &cobra.Command{
	Use:   "kill [session-name]",
	Short: "Kill an agent session",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := tmux.NewManager("boatman")

		if len(args) == 0 {
			// Kill all boatman sessions
			sessions, err := mgr.ListSessions()
			if err != nil {
				return err
			}
			for _, name := range sessions {
				sess := &tmux.Session{Name: name}
				mgr.KillSession(sess)
				fmt.Printf("Killed session: %s\n", name)
			}
			return nil
		}

		sess := &tmux.Session{Name: args[0]}
		if err := mgr.KillSession(sess); err != nil {
			return err
		}
		fmt.Printf("Killed session: %s\n", args[0])
		return nil
	},
}

// watchCmd watches active agent sessions in a split view.
var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch active agent sessions",
	Long: `Opens a tmux view showing all active boatman agent sessions.
	
Use Ctrl+B then arrow keys to switch between panes.
Use Ctrl+B then D to detach.`,
	RunE: runWatch,
}

func runWatch(cmd *cobra.Command, args []string) error {
	mgr := tmux.NewManager("boatman")
	sessions, err := mgr.ListSessions()
	if err != nil || len(sessions) == 0 {
		fmt.Println("No active boatman sessions to watch.")
		fmt.Println("Start a job first: boatman work <ticket-id>")
		return nil
	}

	// If only one session, just attach to it
	if len(sessions) == 1 {
		fmt.Printf("Attaching to %s...\n", sessions[0])
		return attachToSession(sessions[0])
	}

	// Multiple sessions - create a watch session with splits
	watchSession := "boatman-watch"
	
	// Kill existing watch session
	exec.Command("tmux", "kill-session", "-t", watchSession).Run()
	
	// Create new watch session attached to first agent session
	createCmd := exec.Command("tmux", "new-session", "-d", "-s", watchSession, "-t", sessions[0])
	if err := createCmd.Run(); err != nil {
		// Fallback: just attach to first session
		return attachToSession(sessions[0])
	}

	// Link additional sessions as splits
	for i := 1; i < len(sessions) && i < 4; i++ { // Max 4 panes
		splitCmd := exec.Command("tmux", "split-window", "-t", watchSession, "-h")
		splitCmd.Run()
		time.Sleep(100 * time.Millisecond)
		
		// Switch the new pane to the other session
		sendCmd := exec.Command("tmux", "send-keys", "-t", watchSession, 
			fmt.Sprintf("tmux switch-client -t %s", sessions[i]), "Enter")
		sendCmd.Run()
	}

	// Balance the panes
	exec.Command("tmux", "select-layout", "-t", watchSession, "tiled").Run()

	// Attach to the watch session
	return attachToSession(watchSession)
}

// attachToSession attaches to a tmux session.
func attachToSession(name string) error {
	// Check if we're already in tmux
	if os.Getenv("TMUX") != "" {
		// Switch client instead of attach
		cmd := exec.Command("tmux", "switch-client", "-t", name)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	cmd := exec.Command("tmux", "attach", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// cleanupCmd cleans up finished sessions.
var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up finished agent sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := tmux.NewManager("boatman")
		sessions, _ := mgr.ListSessions()
		
		cleaned := 0
		for _, name := range sessions {
			// Check if session is idle (no running command)
			checkCmd := exec.Command("tmux", "list-panes", "-t", name, "-F", "#{pane_current_command}")
			output, err := checkCmd.Output()
			if err != nil {
				continue
			}
			
			// If just showing bash/zsh, it's idle
			command := strings.TrimSpace(string(output))
			if command == "bash" || command == "zsh" || command == "sh" {
				sess := &tmux.Session{Name: name}
				mgr.KillSession(sess)
				fmt.Printf("Cleaned up idle session: %s\n", name)
				cleaned++
			}
		}
		
		if cleaned == 0 {
			fmt.Println("No idle sessions to clean up")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(watchCmd)
	
	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsCmd.AddCommand(sessionsAttachCmd)
	sessionsCmd.AddCommand(sessionsKillCmd)
	sessionsCmd.AddCommand(cleanupCmd)
}
