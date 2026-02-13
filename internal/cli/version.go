package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

// SetVersionInfo sets the version information for the CLI.
// Called from main.go with values injected by GoReleaser.
func SetVersionInfo(v, c, d, b string) {
	version = v
	commit = c
	date = d
	builtBy = b
}

// versionCmd represents the version command.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Display the version, commit hash, build date, and other build information.`,
	Run: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")

		if verbose {
			fmt.Printf("BoatmanMode %s\n", version)
			fmt.Printf("  Commit:     %s\n", commit)
			fmt.Printf("  Built:      %s\n", date)
			fmt.Printf("  Built by:   %s\n", builtBy)
			fmt.Printf("  Go version: %s\n", runtime.Version())
			fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		} else {
			fmt.Printf("boatman version %s\n", version)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().BoolP("verbose", "v", false, "Show verbose version information")
}
