// Package cli provides the command-line interface for boatman.
package cli

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "boatman",
	Short: "AI-powered development agent",
	Long: `BoatmanMode is an AI agent that automates development workflows:

  1. Fetch tickets from Linear
  2. Create isolated git worktrees
  3. Execute development tasks using Claude
  4. Review changes with ScottBott (peer-review skill)
  5. Iterate until review passes
  6. Create pull requests automatically`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.boatman.yaml)")

	// API Keys
	rootCmd.PersistentFlags().String("linear-key", "", "Linear API key")

	// Bind flags to viper
	viper.BindPFlag("linear_key", rootCmd.PersistentFlags().Lookup("linear-key"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".boatman")
	}

	viper.SetEnvPrefix("BOATMAN")
	viper.AutomaticEnv()

	viper.ReadInConfig()
}

