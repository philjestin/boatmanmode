// Package main is the entry point for the boatman CLI.
// It orchestrates AI-powered development workflows: fetch tickets from Linear,
// execute tasks, review with ScottBott, and create PRs.
package main

import (
	"fmt"
	"os"

	"github.com/philjestin/boatmanmode/internal/cli"
)

// Build information. Populated at build time by GoReleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

func main() {
	// Set version info for CLI
	cli.SetVersionInfo(version, commit, date, builtBy)

	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}


