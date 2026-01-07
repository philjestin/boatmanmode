// Package claude provides a wrapper around the Claude CLI.
// Supports both direct exec and tmux-based execution for large prompts.
package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/handshake/boatmanmode/internal/tmux"
)

// Client wraps the Claude CLI.
type Client struct {
	// Command is the claude command to use (default: "claude")
	Command string

	// WorkDir is the working directory for claude commands
	WorkDir string

	// Env is additional environment variables to set
	Env map[string]string

	// UseTmux enables tmux-based execution (better for large prompts)
	UseTmux bool

	// TmuxManager manages tmux sessions
	TmuxManager *tmux.Manager

	// SessionName is the name for tmux sessions
	SessionName string

	// Stream enables streaming output
	Stream bool

	// Debug enables debug output
	Debug bool
}

// StreamChunk represents a chunk from Claude's stream-json output.
type StreamChunk struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Message struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// New creates a new Claude CLI client.
func New() *Client {
	return &Client{
		Command: "claude",
		Env:     make(map[string]string),
		Stream:  true,
		UseTmux: false,
		Debug:   os.Getenv("BOATMAN_DEBUG") == "1",
	}
}

// NewWithWorkDir creates a client that runs in a specific directory.
func NewWithWorkDir(workDir string) *Client {
	return &Client{
		Command: "claude",
		WorkDir: workDir,
		Env:     make(map[string]string),
		Stream:  true,
		UseTmux: false,
		Debug:   os.Getenv("BOATMAN_DEBUG") == "1",
	}
}

// NewWithTmux creates a client that uses tmux for execution.
func NewWithTmux(workDir, sessionName string) *Client {
	return &Client{
		Command:     "claude",
		WorkDir:     workDir,
		Env:         make(map[string]string),
		UseTmux:     true,
		TmuxManager: tmux.NewManager("boatman"),
		SessionName: sessionName,
		Stream:      true,
		Debug:       os.Getenv("BOATMAN_DEBUG") == "1",
	}
}

// Message sends a message to Claude and returns the response.
func (c *Client) Message(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Use tmux for large prompts or when explicitly enabled
	if c.UseTmux || len(userPrompt) > 100000 || len(systemPrompt) > 50000 {
		return c.messageTmux(ctx, systemPrompt, userPrompt)
	}

	if c.Stream {
		return c.messageStreaming(ctx, systemPrompt, userPrompt)
	}
	return c.messageNonStreaming(ctx, systemPrompt, userPrompt)
}

// messageTmux sends a message using tmux session.
func (c *Client) messageTmux(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.TmuxManager == nil {
		c.TmuxManager = tmux.NewManager("boatman")
	}

	sessionName := c.SessionName
	if sessionName == "" {
		sessionName = "claude"
	}

	sess, err := c.TmuxManager.CreateSession(sessionName, c.WorkDir)
	if err != nil {
		return "", fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Don't kill session on completion - let user inspect if needed
	// defer c.TmuxManager.KillSession(sess)

	return c.TmuxManager.RunClaudeStreaming(ctx, sess, systemPrompt, userPrompt)
}

// messageStreaming sends a message and streams the response.
func (c *Client) messageStreaming(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--tools", "", // Disable tools for clean text output
	}

	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}

	args = append(args, userPrompt)

	cmd := exec.CommandContext(ctx, c.Command, args...)

	if c.WorkDir != "" {
		cmd.Dir = c.WorkDir
	}

	cmd.Env = os.Environ()
	for k, v := range c.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start claude: %w", err)
	}

	// Stream and collect the response
	var fullResponse strings.Builder

	fmt.Println("   ┌─────────────────────────────────────────────────────────────")

	lineBuffer := ""
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("error reading stream: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			if c.Debug {
				fmt.Printf("\n[DEBUG] Failed to parse chunk: %s\n", line)
			}
			continue
		}

		// Handle different chunk types
		var text string
		switch chunk.Type {
		case "content_block_delta":
			text = chunk.Content
		case "message_stop":
			continue
		case "result":
			for _, content := range chunk.Message.Content {
				if content.Type == "text" {
					text = content.Text
				}
			}
		}

		if text != "" {
			fullResponse.WriteString(text)

			// Stream to terminal with formatting
			lineBuffer += text
			for {
				idx := strings.Index(lineBuffer, "\n")
				if idx == -1 {
					break
				}
				fmt.Printf("   │ %s\n", lineBuffer[:idx])
				lineBuffer = lineBuffer[idx+1:]
			}
		}
	}

	// Print any remaining content
	if lineBuffer != "" {
		fmt.Printf("   │ %s\n", lineBuffer)
	}

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("claude command failed: %w\nstderr: %s", err, stderr.String())
	}

	fmt.Println("   └─────────────────────────────────────────────────────────────")
	fmt.Printf("   📄 Total: %d chars\n", fullResponse.Len())

	return fullResponse.String(), nil
}

// messageNonStreaming sends a message without streaming.
func (c *Client) messageNonStreaming(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	args := []string{
		"-p",
		"--output-format", "text",
	}

	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}

	args = append(args, userPrompt)

	cmd := exec.CommandContext(ctx, c.Command, args...)

	if c.WorkDir != "" {
		cmd.Dir = c.WorkDir
	}

	cmd.Env = os.Environ()
	for k, v := range c.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	if c.Debug {
		fmt.Printf("[DEBUG] Running: %s %v\n", c.Command, args[:min(3, len(args))])
		fmt.Printf("[DEBUG] WorkDir: %s\n", c.WorkDir)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude command failed: %w\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String()[:min(500, stdout.Len())])
	}

	return strings.TrimSpace(stdout.String()), nil
}

// MessageWithFiles sends a message with file context to Claude.
func (c *Client) MessageWithFiles(ctx context.Context, systemPrompt, userPrompt string, files []string) (string, error) {
	args := []string{
		"-p",
		"--output-format", "text",
	}

	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}

	for _, f := range files {
		args = append(args, "--add-dir", f)
	}

	args = append(args, userPrompt)

	cmd := exec.CommandContext(ctx, c.Command, args...)

	if c.WorkDir != "" {
		cmd.Dir = c.WorkDir
	}

	cmd.Env = os.Environ()
	for k, v := range c.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude command failed: %w\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String()[:min(500, stdout.Len())])
	}

	return strings.TrimSpace(stdout.String()), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
