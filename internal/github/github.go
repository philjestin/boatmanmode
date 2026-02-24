// Package github provides GitHub integration via the gh CLI.
package github

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// PRResult represents the result of PR creation.
type PRResult struct {
	URL string
}

// CreatePR creates a pull request using the gh CLI.
// Deprecated: Use CreatePRInDir instead for explicit working directory.
func CreatePR(ctx context.Context, title, body, baseBranch string) (*PRResult, error) {
	return CreatePRInDir(ctx, "", title, body, baseBranch)
}

// CreatePRInDir creates a pull request using the gh CLI in the specified directory.
// Any extra flags are appended directly to the `gh pr create` command.
func CreatePRInDir(ctx context.Context, workDir, title, body, baseBranch string, extraFlags ...string) (*PRResult, error) {
	args := []string{"pr", "create",
		"--title", title,
		"--body", body,
		"--base", baseBranch,
	}

	args = append(args, extraFlags...)

	cmd := exec.CommandContext(ctx, "gh", args...)

	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh pr create failed: %w\nstderr: %s", err, stderr.String())
	}

	prURL := strings.TrimSpace(stdout.String())

	return &PRResult{
		URL: prURL,
	}, nil
}
