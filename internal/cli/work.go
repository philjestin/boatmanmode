package cli

import (
	"context"
	"fmt"

	"github.com/handshake/boatmanmode/internal/agent"
	"github.com/handshake/boatmanmode/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// workCmd represents the work command - the main workflow executor.
var workCmd = &cobra.Command{
	Use:   "work [ticket-id]",
	Short: "Execute a ticket from Linear",
	Long: `Fetch a ticket from Linear and execute the development task.

The agent will:
  1. Fetch the ticket details from Linear
  2. Create a git worktree for isolated development
  3. Execute the task using Claude
  4. Review with ScottBott
  5. Refactor if needed until review passes
  6. Create a pull request`,
	Args: cobra.ExactArgs(1),
	RunE: runWork,
}

func init() {
	rootCmd.AddCommand(workCmd)

	workCmd.Flags().Int("max-iterations", 3, "Maximum review/refactor iterations")
	workCmd.Flags().String("base-branch", "main", "Base branch for worktree")
	workCmd.Flags().Bool("auto-pr", true, "Automatically create PR on success")
	workCmd.Flags().Bool("dry-run", false, "Run without making changes")

	viper.BindPFlag("max_iterations", workCmd.Flags().Lookup("max-iterations"))
	viper.BindPFlag("base_branch", workCmd.Flags().Lookup("base-branch"))
	viper.BindPFlag("auto_pr", workCmd.Flags().Lookup("auto-pr"))
}

// runWork executes the main workflow for a given ticket.
func runWork(cmd *cobra.Command, args []string) error {
	ticketID := args[0]
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun {
		fmt.Println("🏃 Dry run mode - no changes will be made")
	}

	a, err := agent.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	result, err := a.Work(ctx, ticketID)
	if err != nil {
		return fmt.Errorf("work failed: %w", err)
	}

	if result.PRCreated {
		fmt.Printf("✅ PR created: %s\n", result.PRURL)
	} else {
		fmt.Printf("⚠️  Work completed but PR not created: %s\n", result.Message)
	}

	return nil
}

