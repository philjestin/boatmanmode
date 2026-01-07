// Package tmux provides tmux session management for running Claude agents.
// Each agent runs in its own tmux session, allowing real-time monitoring.
package tmux

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Session represents a tmux session running a Claude agent.
type Session struct {
	Name       string
	WorkDir    string
	OutputFile string
	DoneFile   string
}

// Manager handles tmux session lifecycle.
type Manager struct {
	sessionPrefix string
	outputDir     string
}

// NewManager creates a new tmux session manager.
func NewManager(prefix string) *Manager {
	outputDir := filepath.Join(os.TempDir(), "boatman-sessions")
	os.MkdirAll(outputDir, 0755)

	return &Manager{
		sessionPrefix: prefix,
		outputDir:     outputDir,
	}
}

// CreateSession creates a new tmux session for a Claude agent.
func (m *Manager) CreateSession(name, workDir string) (*Session, error) {
	sessionName := fmt.Sprintf("%s-%s", m.sessionPrefix, name)
	outputFile := filepath.Join(m.outputDir, fmt.Sprintf("%s.out", sessionName))
	doneFile := filepath.Join(m.outputDir, fmt.Sprintf("%s.done", sessionName))

	// Kill existing session if any
	exec.Command("tmux", "kill-session", "-t", sessionName).Run()

	// Remove old files
	os.Remove(outputFile)
	os.Remove(doneFile)

	// Create new detached session
	var cmd *exec.Cmd
	if workDir != "" {
		cmd = exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", workDir)
	} else {
		cmd = exec.Command("tmux", "new-session", "-d", "-s", sessionName)
	}
	
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Set up the session with a clean display
	time.Sleep(100 * time.Millisecond)
	m.sendKeys(sessionName, "clear")
	time.Sleep(50 * time.Millisecond)
	m.sendKeys(sessionName, fmt.Sprintf("echo '🚣 Boatman Agent: %s'", name))
	time.Sleep(50 * time.Millisecond)
	if workDir != "" {
		m.sendKeys(sessionName, fmt.Sprintf("echo '📁 %s'", workDir))
		time.Sleep(50 * time.Millisecond)
	}
	m.sendKeys(sessionName, "echo '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━'")
	time.Sleep(50 * time.Millisecond)

	return &Session{
		Name:       sessionName,
		WorkDir:    workDir,
		OutputFile: outputFile,
		DoneFile:   doneFile,
	}, nil
}

// RunClaudeStreaming runs Claude with live output in the tmux session.
// The output streams directly to the terminal for live viewing.
// When complete, the output is captured via tmux capture-pane.
func (m *Manager) RunClaudeStreaming(ctx context.Context, sess *Session, systemPrompt, userPrompt string) (string, error) {
	// Write prompt to file (avoids command line length limits)
	promptFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-prompt.txt", sess.Name))
	if err := os.WriteFile(promptFile, []byte(userPrompt), 0644); err != nil {
		return "", fmt.Errorf("failed to write prompt file: %w", err)
	}
	defer os.Remove(promptFile)

	// Create runner script
	scriptFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-run.sh", sess.Name))
	
	var script string
	if systemPrompt != "" {
		sysFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-system.txt", sess.Name))
		if err := os.WriteFile(sysFile, []byte(systemPrompt), 0644); err != nil {
			return "", fmt.Errorf("failed to write system prompt file: %w", err)
		}
		defer os.Remove(sysFile)
		
		// Run claude with unbuffered output using script or stdbuf
		// The prompt is passed as a file argument to avoid stdin buffering issues
		script = fmt.Sprintf(`#!/bin/bash
echo ''
echo '🤖 Claude is working...'
echo ''

# Try to use unbuffer for real-time output, fall back to stdbuf, then direct
if command -v unbuffer &> /dev/null; then
    unbuffer claude -p --output-format text --system-prompt "$(cat '%s')" "$(cat '%s')"
elif command -v stdbuf &> /dev/null; then
    stdbuf -oL claude -p --output-format text --system-prompt "$(cat '%s')" "$(cat '%s')"
else
    claude -p --output-format text --system-prompt "$(cat '%s')" "$(cat '%s')"
fi

EXIT_CODE=$?
echo ''
echo '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━'
if [ $EXIT_CODE -eq 0 ]; then
    echo '✅ Claude completed successfully'
else
    echo '❌ Claude exited with code: '$EXIT_CODE
fi
touch '%s'
`, sysFile, promptFile, sysFile, promptFile, sysFile, promptFile, sess.DoneFile)
	} else {
		script = fmt.Sprintf(`#!/bin/bash
echo ''
echo '🤖 Claude is working...'
echo ''

# Try to use unbuffer for real-time output, fall back to stdbuf, then direct
if command -v unbuffer &> /dev/null; then
    unbuffer claude -p --output-format text "$(cat '%s')"
elif command -v stdbuf &> /dev/null; then
    stdbuf -oL claude -p --output-format text "$(cat '%s')"
else
    claude -p --output-format text "$(cat '%s')"
fi

EXIT_CODE=$?
echo ''
echo '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━'
if [ $EXIT_CODE -eq 0 ]; then
    echo '✅ Claude completed successfully'
else
    echo '❌ Claude exited with code: '$EXIT_CODE
fi
touch '%s'
`, promptFile, promptFile, promptFile, sess.DoneFile)
	}

	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		return "", fmt.Errorf("failed to write script: %w", err)
	}
	defer os.Remove(scriptFile)

	// Clear done file
	os.Remove(sess.DoneFile)

	// Run the script in tmux
	m.sendKeys(sess.Name, scriptFile)

	// Wait for completion
	return m.waitAndCapture(ctx, sess)
}

// waitAndCapture waits for Claude to finish and captures the output.
func (m *Manager) waitAndCapture(ctx context.Context, sess *Session) (string, error) {
	fmt.Println("   ┌─────────────────────────────────────────────────────────────")
	fmt.Printf("   │ 📺 Session: %s\n", sess.Name)
	fmt.Println("   │ 💡 Watch live: boatman watch")
	fmt.Println("   │ 💡 Or: tmux attach -t " + sess.Name)
	fmt.Println("   └─────────────────────────────────────────────────────────────")
	fmt.Println("   ⏳ Waiting for Claude (watch live with 'boatman watch')...")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(15 * time.Minute)
	startTime := time.Now()
	lastDot := time.Now()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for Claude response")
		case <-ticker.C:
			// Check if done
			if _, err := os.Stat(sess.DoneFile); err == nil {
				elapsed := time.Since(startTime)
				fmt.Printf("\n   ⏱️  Completed in %s\n", elapsed.Round(time.Second))
				
				// Capture the pane content
				output, err := m.capturePane(sess, 5000) // Capture last 5000 lines
				if err != nil {
					return "", fmt.Errorf("failed to capture output: %w", err)
				}
				
				// Save for debugging
				os.WriteFile(sess.OutputFile, []byte(output), 0644)
				
				return extractClaudeOutput(output), nil
			}
			
			// Print progress dots
			if time.Since(lastDot) >= 5*time.Second {
				fmt.Print(".")
				lastDot = time.Now()
			}
		}
	}
}

// capturePane captures the tmux pane content.
func (m *Manager) capturePane(sess *Session, lines int) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", sess.Name, "-p", "-S", fmt.Sprintf("-%d", lines))
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// extractClaudeOutput extracts just Claude's response from the captured pane.
func extractClaudeOutput(paneContent string) string {
	lines := strings.Split(paneContent, "\n")
	
	var output strings.Builder
	inOutput := false
	
	for _, line := range lines {
		// Start capturing after "Starting Claude..."
		if strings.Contains(line, "Starting Claude") {
			inOutput = true
			continue
		}
		
		// Stop at the completion marker
		if strings.Contains(line, "━━━━━━━━━━") && inOutput {
			break
		}
		
		if inOutput {
			output.WriteString(line)
			output.WriteString("\n")
		}
	}
	
	return strings.TrimSpace(output.String())
}

// RunClaude runs Claude in the session with the given prompt via stdin.
func (m *Manager) RunClaude(ctx context.Context, sess *Session, systemPrompt, userPrompt string) (string, error) {
	return m.RunClaudeStreaming(ctx, sess, systemPrompt, userPrompt)
}

// KillSession terminates a tmux session.
func (m *Manager) KillSession(sess *Session) error {
	cmd := exec.Command("tmux", "kill-session", "-t", sess.Name)
	return cmd.Run()
}

// AttachSession attaches to a tmux session (for debugging).
func (m *Manager) AttachSession(sess *Session) error {
	cmd := exec.Command("tmux", "attach", "-t", sess.Name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ListSessions lists all boatman tmux sessions.
func (m *Manager) ListSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		return nil, nil // No sessions
	}

	var sessions []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		name := scanner.Text()
		if strings.HasPrefix(name, m.sessionPrefix) {
			sessions = append(sessions, name)
		}
	}

	return sessions, nil
}

// sendKeys sends keys to a tmux session.
func (m *Manager) sendKeys(sessionName, keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, keys, "Enter")
	return cmd.Run()
}

// CapturePane captures the current pane content (exported).
func (m *Manager) CapturePane(sess *Session) (string, error) {
	return m.capturePane(sess, 1000)
}
