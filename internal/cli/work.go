package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/philjestin/boatmanmode/internal/agent"
	"github.com/philjestin/boatmanmode/internal/config"
	"github.com/philjestin/boatmanmode/internal/linear"
	"github.com/philjestin/boatmanmode/internal/task"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// workCmd represents the work command - the main workflow executor.
var workCmd = &cobra.Command{
	Use:   "work [ticket-id-or-prompt]",
	Short: "Execute a task from Linear, prompt, or file",
	Long: `Execute a development task from multiple input sources.

Input modes:
  1. Linear ticket (default):    boatman work ENG-123
  2. Inline prompt:              boatman work --prompt "Add authentication"
  3. File-based prompt:          boatman work --file ./task.txt

The agent will:
  1. Prepare the task
  2. Create a git worktree for isolated development
  3. Execute the task using Claude
  4. Review with ScottBott
  5. Refactor if needed until review passes
  6. Create a pull request

Flags like --title and --branch-name can override auto-generated values for prompt/file mode.`,
	Args: cobra.ExactArgs(1),
	RunE: runWork,
}

func init() {
	rootCmd.AddCommand(workCmd)

	// Existing flags
	workCmd.Flags().Int("max-iterations", 3, "Maximum review/refactor iterations")
	workCmd.Flags().String("base-branch", "main", "Base branch for worktree")
	workCmd.Flags().Bool("auto-pr", true, "Automatically create PR on success")
	workCmd.Flags().Bool("dry-run", false, "Run without making changes")
	workCmd.Flags().Int("timeout", 60, "Timeout in minutes for each Claude agent")
	workCmd.Flags().String("review-skill", "peer-review", "Claude skill/agent to use for code review")

	// New input mode flags
	workCmd.Flags().Bool("prompt", false, "Treat argument as inline prompt text")
	workCmd.Flags().Bool("file", false, "Read prompt from file")
	workCmd.Flags().String("title", "", "Override auto-generated task title (prompt/file mode only)")
	workCmd.Flags().String("branch-name", "", "Override auto-generated branch name (prompt/file mode only)")

	viper.BindPFlag("max_iterations", workCmd.Flags().Lookup("max-iterations"))
	viper.BindPFlag("base_branch", workCmd.Flags().Lookup("base-branch"))
	viper.BindPFlag("auto_pr", workCmd.Flags().Lookup("auto-pr"))
	viper.BindPFlag("timeout", workCmd.Flags().Lookup("timeout"))
	viper.BindPFlag("review_skill", workCmd.Flags().Lookup("review-skill"))
}

// runWork executes the main workflow for a given task.
func runWork(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate and parse input mode
	t, err := parseTaskInput(cmd, args, cfg)
	if err != nil {
		return err
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun {
		fmt.Println("🏃 Dry run mode - no changes will be made")
	}

	a, err := agent.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	result, err := a.Work(ctx, t)
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

// parseTaskInput determines the input mode and creates the appropriate Task.
func parseTaskInput(cmd *cobra.Command, args []string, cfg *config.Config) (task.Task, error) {
	input := args[0]

	// Get mode flags
	isPrompt, _ := cmd.Flags().GetBool("prompt")
	isFile, _ := cmd.Flags().GetBool("file")
	overrideTitle, _ := cmd.Flags().GetString("title")
	overrideBranch, _ := cmd.Flags().GetString("branch-name")

	// Validate: only one mode can be set
	modesSet := 0
	if isPrompt {
		modesSet++
	}
	if isFile {
		modesSet++
	}
	if modesSet > 1 {
		return nil, fmt.Errorf("only one of --prompt or --file can be specified")
	}

	// Validate: title/branch overrides only work with prompt/file mode
	if !isPrompt && !isFile {
		if overrideTitle != "" {
			return nil, fmt.Errorf("--title can only be used with --prompt or --file")
		}
		if overrideBranch != "" {
			return nil, fmt.Errorf("--branch-name can only be used with --prompt or --file")
		}
	}

	// Create task based on mode
	if isPrompt {
		fmt.Println("📝 Prompt mode")
		return task.CreateFromPrompt(input, overrideTitle, overrideBranch)
	}

	if isFile {
		fmt.Println("📄 File mode")
		// Validate file exists
		if _, err := os.Stat(input); err != nil {
			return nil, fmt.Errorf("task file does not exist: %s", input)
		}
		return task.CreateFromFile(input, overrideTitle, overrideBranch)
	}

	// Default: Linear mode
	fmt.Println("🎫 Linear mode")
	linearClient := linear.New(cfg.LinearKey)
	return task.CreateFromLinear(ctx, linearClient, input)
}

// ctx is needed for CreateFromLinear
var ctx = context.Background()
