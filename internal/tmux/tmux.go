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

// parseScript is the shell code that parses Claude's stream-json output.
// It's shared between the with-system-prompt and without-system-prompt versions.
// RESULT_FILE env var should be set before calling to save the result content.
const parseScript = `
# Parse JSON to show activity and token usage
parse_claude_output() {
    while IFS= read -r line; do
        if echo "$line" | grep -q '"type":"assistant"'; then
            if echo "$line" | grep -q '"name":"Bash"'; then
                cmd=$(echo "$line" | sed -n 's/.*"command":"\([^"]*\)".*/\1/p' | head -1)
                if [ -n "$cmd" ]; then
                    echo "🔧 Running: $cmd"
                fi
            elif echo "$line" | grep -q '"name":"Edit"'; then
                file=$(echo "$line" | sed -n 's/.*"file_path":"\([^"]*\)".*/\1/p' | head -1)
                if [ -n "$file" ]; then
                    echo "✏️  Editing: $file"
                fi
            elif echo "$line" | grep -q '"name":"Write"'; then
                file=$(echo "$line" | sed -n 's/.*"file_path":"\([^"]*\)".*/\1/p' | head -1)
                if [ -n "$file" ]; then
                    echo "📝 Writing: $file"
                fi
            elif echo "$line" | grep -q '"name":"Read"'; then
                file=$(echo "$line" | sed -n 's/.*"file_path":"\([^"]*\)".*/\1/p' | head -1)
                if [ -n "$file" ]; then
                    echo "📖 Reading: $file"
                fi
            elif echo "$line" | grep -q '"name":"Glob"'; then
                echo "🔍 Searching files..."
            elif echo "$line" | grep -q '"name":"Grep"'; then
                echo "🔍 Searching content..."
            elif echo "$line" | grep -q '"type":"text"' && ! echo "$line" | grep -q '"tool_use"'; then
                text=$(echo "$line" | sed -n 's/.*"text":"\([^"]*\)".*/\1/p' | head -1 | head -c 200)
                if [ -n "$text" ]; then
                    echo "💭 $text"
                fi
            fi
        elif echo "$line" | grep -q '"type":"result"'; then
            echo ""
            echo "📊 Task completed!"
            # Save the result line for later parsing (contains full response)
            if [ -n "$RESULT_FILE" ]; then
                echo "$line" > "$RESULT_FILE"
            fi
            # Extract and display token usage
            cost=$(echo "$line" | sed -n 's/.*"total_cost_usd":\([0-9.]*\).*/\1/p')
            input=$(echo "$line" | grep -o '"input_tokens":[0-9]*' | head -1 | sed 's/"input_tokens"://')
            output=$(echo "$line" | grep -o '"output_tokens":[0-9]*' | head -1 | sed 's/"output_tokens"://')
            cache=$(echo "$line" | grep -o '"cache_read_input_tokens":[0-9]*' | head -1 | sed 's/"cache_read_input_tokens"://')
            if [ -n "$cost" ]; then
                printf "💰 Cost: \$%.4f\n" "$cost"
            fi
            if [ -n "$input" ] || [ -n "$output" ]; then
                echo "📈 Tokens: ${input:-0} in / ${output:-0} out / ${cache:-0} cached"
            fi
        fi
    done
}
`

// RunClaudeStreaming runs Claude with live output in the tmux session.
// The output streams directly to the terminal for live viewing.
// When complete, the output is captured via tmux capture-pane.
// ClaudeOptions holds options for Claude CLI invocation.
type ClaudeOptions struct {
	Model               string
	EnablePromptCaching bool
}

func (m *Manager) RunClaudeStreaming(ctx context.Context, sess *Session, systemPrompt, userPrompt string) (string, error) {
	return m.RunClaudeStreamingWithOptions(ctx, sess, systemPrompt, userPrompt, ClaudeOptions{})
}

func (m *Manager) RunClaudeStreamingWithOptions(ctx context.Context, sess *Session, systemPrompt, userPrompt string, opts ClaudeOptions) (string, error) {
	// Write prompt to file (avoids command line length limits)
	promptFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-prompt.txt", sess.Name))
	if err := os.WriteFile(promptFile, []byte(userPrompt), 0644); err != nil {
		return "", fmt.Errorf("failed to write prompt file: %w", err)
	}
	// Don't delete prompt file until after completion - tmux needs it
	
	// Result file stores Claude's JSON result for later parsing
	resultFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-result.json", sess.Name))
	os.Remove(resultFile) // Clear any old result

	// Create runner script
	scriptFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-run.sh", sess.Name))

	// Build Claude CLI flags
	claudeFlags := "-p --dangerously-skip-permissions --verbose --output-format stream-json"
	if opts.Model != "" {
		claudeFlags += fmt.Sprintf(" --model %s", opts.Model)
	}
	if opts.EnablePromptCaching {
		claudeFlags += " --cache-system-prompt"
	}

	var script string
	if systemPrompt != "" {
		sysFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-system.txt", sess.Name))
		if err := os.WriteFile(sysFile, []byte(systemPrompt), 0644); err != nil {
			return "", fmt.Errorf("failed to write system prompt file: %w", err)
		}

		script = fmt.Sprintf(`#!/bin/bash
echo ''
echo '🤖 Claude is working (with file write permissions)...'
echo '📝 Activity will stream below:'
echo ''

# Set result file path for parse_claude_output to save result
export RESULT_FILE='%s'

%s

# Read into variables
SYSTEM_PROMPT="$(cat '%s')"
USER_PROMPT="$(cat '%s')"

# Run Claude with stream-json and parse output
claude %s --system-prompt "$SYSTEM_PROMPT" "$USER_PROMPT" 2>&1 | parse_claude_output

EXIT_CODE=$?
echo ''
echo '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━'
if [ $EXIT_CODE -eq 0 ]; then
    echo '✅ Claude completed successfully'
else
    echo '❌ Claude exited with code: '$EXIT_CODE
fi
touch '%s'

# Cleanup (leave result file for parsing)
rm -f '%s' '%s' '%s'
`, resultFile, parseScript, sysFile, promptFile, claudeFlags, sess.DoneFile, promptFile, sysFile, scriptFile)
	} else {
		script = fmt.Sprintf(`#!/bin/bash
echo ''
echo '🤖 Claude is working (with file write permissions)...'
echo '📝 Activity will stream below:'
echo ''

# Set result file path for parse_claude_output to save result
export RESULT_FILE='%s'

%s

# Read into variable
USER_PROMPT="$(cat '%s')"

# Run Claude with stream-json and parse output
claude %s "$USER_PROMPT" 2>&1 | parse_claude_output

EXIT_CODE=$?
echo ''
echo '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━'
if [ $EXIT_CODE -eq 0 ]; then
    echo '✅ Claude completed successfully'
else
    echo '❌ Claude exited with code: '$EXIT_CODE
fi
touch '%s'

# Cleanup (leave result file for parsing)
rm -f '%s' '%s'
`, resultFile, parseScript, promptFile, claudeFlags, sess.DoneFile, promptFile, scriptFile)
	}

	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		return "", fmt.Errorf("failed to write script: %w", err)
	}
	// Script cleans itself up after running

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

	timeout := time.After(60 * time.Minute) // 60 min timeout for complex tasks
	startTime := time.Now()
	lastDot := time.Now()
	
	// Result file where the stream-json result is saved
	resultFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-result.json", sess.Name))

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
				
				// Try to extract actual result from the result file (has full response)
				if resultContent, err := os.ReadFile(resultFile); err == nil && len(resultContent) > 0 {
					defer os.Remove(resultFile) // Clean up after reading
					return extractResultFromJSON(string(resultContent)), nil
				}
				
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

// extractResultFromJSON extracts Claude's text result from the stream-json result line.
func extractResultFromJSON(jsonLine string) string {
	// The result line contains: {"type":"result","result":"actual content here",...}
	// Extract the "result" field
	
	// Find "result":" and extract the value
	resultKey := `"result":"`
	startIdx := strings.Index(jsonLine, resultKey)
	if startIdx == -1 {
		return jsonLine // Return as-is if no result field found
	}
	
	startIdx += len(resultKey)
	
	// Find the end of the string value (handle escaped quotes)
	var sb strings.Builder
	escaped := false
	for i := startIdx; i < len(jsonLine); i++ {
		c := jsonLine[i]
		if escaped {
			// Handle common escape sequences
			switch c {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			default:
				sb.WriteByte(c)
			}
			escaped = false
		} else if c == '\\' {
			escaped = true
		} else if c == '"' {
			// End of string
			break
		} else {
			sb.WriteByte(c)
		}
	}
	
	return sb.String()
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
// Now handles the parsed stream-json activity output.
func extractClaudeOutput(paneContent string) string {
	lines := strings.Split(paneContent, "\n")
	
	var output strings.Builder
	inOutput := false
	
	for _, line := range lines {
		// Start capturing after "Activity will stream" or "Claude is working"
		if strings.Contains(line, "Activity will stream") || strings.Contains(line, "Claude is working") {
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
