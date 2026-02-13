// Package tmux provides tmux session management for running Claude agents.
// Each agent runs in its own tmux session, allowing real-time monitoring.
package tmux

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/philjestin/boatmanmode/internal/cost"
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
	_ = os.MkdirAll(outputDir, 0755) // Best effort, failure handled later when writing files

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

	// Kill existing session if any (ignore error if session doesn't exist)
	_ = exec.Command("tmux", "kill-session", "-t", sessionName).Run()

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
	_ = m.sendKeys(sessionName, "clear")
	time.Sleep(50 * time.Millisecond)
	_ = m.sendKeys(sessionName, fmt.Sprintf("echo '🚣 Boatman Agent: %s'", name))
	time.Sleep(50 * time.Millisecond)
	if workDir != "" {
		_ = m.sendKeys(sessionName, fmt.Sprintf("echo '📁 %s'", workDir))
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
// RAW_OUTPUT_FILE env var should be set to save raw output for debugging.
const parseScript = `
# Parse JSON to show activity and token usage
parse_claude_output() {
    while IFS= read -r line; do
        # Save all lines to raw output file for debugging
        if [ -n "$RAW_OUTPUT_FILE" ]; then
            echo "$line" >> "$RAW_OUTPUT_FILE"
        fi

        # Check for result type (handles "type":"result" or "type": "result")
        if echo "$line" | grep -qE '"type"\s*:\s*"result"'; then
            echo ""
            echo "📊 Task completed!"
            # Save the result line for later parsing (contains full response)
            if [ -n "$RESULT_FILE" ]; then
                echo "$line" > "$RESULT_FILE"
            fi
            # Extract and display token usage
            cost=$(echo "$line" | sed -n 's/.*"total_cost_usd":\([0-9.]*\).*/\1/p')
            input=$(echo "$line" | grep -oE '"input_tokens"\s*:\s*[0-9]+' | head -1 | grep -oE '[0-9]+')
            output=$(echo "$line" | grep -oE '"output_tokens"\s*:\s*[0-9]+' | head -1 | grep -oE '[0-9]+')
            cache=$(echo "$line" | grep -oE '"cache_read_input_tokens"\s*:\s*[0-9]+' | head -1 | grep -oE '[0-9]+')
            if [ -n "$cost" ]; then
                printf "💰 Cost: \$%.4f\n" "$cost"
            fi
            if [ -n "$input" ] || [ -n "$output" ]; then
                echo "📈 Tokens: ${input:-0} in / ${output:-0} out / ${cache:-0} cached"
            fi
        # Check for assistant messages with tool use
        elif echo "$line" | grep -qE '"type"\s*:\s*"assistant"'; then
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
            elif echo "$line" | grep -qE '"type"\s*:\s*"text"' && ! echo "$line" | grep -q '"tool_use"'; then
                text=$(echo "$line" | sed -n 's/.*"text":"\([^"]*\)".*/\1/p' | head -1 | head -c 200)
                if [ -n "$text" ]; then
                    echo "💭 $text"
                fi
            fi
        # Capture error messages
        elif echo "$line" | grep -qiE '(error|failed|exception|invalid)'; then
            echo "⚠️  $line"
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

func (m *Manager) RunClaudeStreaming(ctx context.Context, sess *Session, systemPrompt, userPrompt string) (string, *cost.Usage, error) {
	return m.RunClaudeStreamingWithOptions(ctx, sess, systemPrompt, userPrompt, ClaudeOptions{})
}

func (m *Manager) RunClaudeStreamingWithOptions(ctx context.Context, sess *Session, systemPrompt, userPrompt string, opts ClaudeOptions) (string, *cost.Usage, error) {
	// Write prompt to file (avoids command line length limits)
	promptFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-prompt.txt", sess.Name))
	if err := os.WriteFile(promptFile, []byte(userPrompt), 0644); err != nil {
		return "", nil, fmt.Errorf("failed to write prompt file: %w", err)
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
	// Note: Prompt caching happens automatically at the API level, no flag needed

	// Raw output file for debugging when result parsing fails
	rawOutputFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-raw.txt", sess.Name))
	os.Remove(rawOutputFile) // Clear any old output

	var script string
	if systemPrompt != "" {
		sysFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-system.txt", sess.Name))
		if err := os.WriteFile(sysFile, []byte(systemPrompt), 0644); err != nil {
			return "", nil, fmt.Errorf("failed to write system prompt file: %w", err)
		}

		script = fmt.Sprintf(`#!/bin/bash
echo ''
echo '🤖 Claude is working (with file write permissions)...'
echo '📝 Activity will stream below:'
echo ''

# Set file paths for parse_claude_output
export RESULT_FILE='%s'
export RAW_OUTPUT_FILE='%s'

# Clear raw output file
> "$RAW_OUTPUT_FILE"

%s

# Read into variables
SYSTEM_PROMPT="$(cat '%s')"
USER_PROMPT="$(cat '%s')"

# Check if claude CLI exists
if ! command -v claude &> /dev/null; then
    echo '❌ Error: claude CLI not found in PATH'
    touch '%s'
    exit 1
fi

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

# Cleanup (leave result and raw files for parsing)
rm -f '%s' '%s' '%s'
`, resultFile, rawOutputFile, parseScript, sysFile, promptFile, sess.DoneFile, claudeFlags, sess.DoneFile, promptFile, sysFile, scriptFile)
	} else {
		script = fmt.Sprintf(`#!/bin/bash
echo ''
echo '🤖 Claude is working (with file write permissions)...'
echo '📝 Activity will stream below:'
echo ''

# Set file paths for parse_claude_output
export RESULT_FILE='%s'
export RAW_OUTPUT_FILE='%s'

# Clear raw output file
> "$RAW_OUTPUT_FILE"

%s

# Read into variable
USER_PROMPT="$(cat '%s')"

# Check if claude CLI exists
if ! command -v claude &> /dev/null; then
    echo '❌ Error: claude CLI not found in PATH'
    touch '%s'
    exit 1
fi

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

# Cleanup (leave result and raw files for parsing)
rm -f '%s' '%s'
`, resultFile, rawOutputFile, parseScript, promptFile, sess.DoneFile, claudeFlags, sess.DoneFile, promptFile, scriptFile)
	}

	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		return "", nil, fmt.Errorf("failed to write script: %w", err)
	}
	// Script cleans itself up after running

	// Clear done file
	os.Remove(sess.DoneFile)

	// Run the script in tmux
	m.sendKeys(sess.Name, scriptFile)

	// Wait for completion and get usage
	return m.waitAndCapture(ctx, sess)
}

// waitAndCapture waits for Claude to finish and captures the output.
func (m *Manager) waitAndCapture(ctx context.Context, sess *Session) (string, *cost.Usage, error) {
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
	rawOutputFile := filepath.Join(m.outputDir, fmt.Sprintf("%s-raw.txt", sess.Name))

	for {
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-timeout:
			return "", nil, fmt.Errorf("timeout waiting for Claude response")
		case <-ticker.C:
			// Check if done
			if _, err := os.Stat(sess.DoneFile); err == nil {
				elapsed := time.Since(startTime)
				fmt.Printf("\n   ⏱️  Completed in %s\n", elapsed.Round(time.Second))

				// Capture the pane content
				output, err := m.capturePane(sess, 5000) // Capture last 5000 lines
				if err != nil {
					return "", nil, fmt.Errorf("failed to capture output: %w", err)
				}

				// Save for debugging (best effort, ignore errors)
				_ = os.WriteFile(sess.OutputFile, []byte(output), 0644)

				// Try to extract actual result and usage from the result file
				if resultContent, err := os.ReadFile(resultFile); err == nil && len(resultContent) > 0 {
					defer os.Remove(resultFile) // Clean up after reading
					resultText := extractResultFromJSON(string(resultContent))
					usage := parseResultUsage(string(resultContent))

					// Display usage if available
					if usage != nil && !usage.IsEmpty() {
						fmt.Printf("   💰 Cost: $%.4f (in: %d, out: %d, cache: %d)\n",
							usage.TotalCostUSD, usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens)
					}

					// Clean up raw file on success
					os.Remove(rawOutputFile)
					return resultText, usage, nil
				}

				// Result file missing or empty - try raw output file for debugging
				if rawContent, err := os.ReadFile(rawOutputFile); err == nil && len(rawContent) > 0 {
					// Try to extract result from raw stream-json lines
					resultText := extractResultFromRawOutput(string(rawContent))
					if resultText != "" {
						os.Remove(rawOutputFile)
						return resultText, nil, nil
					}
					// Log that we have raw output but couldn't parse it
					if os.Getenv("BOATMAN_DEBUG") == "1" {
						fmt.Printf("   ⚠️  Raw output available (%d bytes) but no result found\n", len(rawContent))
						fmt.Printf("   📁 Debug file: %s\n", rawOutputFile)
					}
				}

				// Final fallback: extract from pane content
				fallbackResult := extractClaudeOutput(output)
				if fallbackResult == "" && os.Getenv("BOATMAN_DEBUG") == "1" {
					fmt.Println("   ⚠️  No result could be extracted from Claude's output")
					fmt.Printf("   📁 Check pane output: %s\n", sess.OutputFile)
				}
				return fallbackResult, nil, nil
			}

			// Print progress dots
			if time.Since(lastDot) >= 5*time.Second {
				fmt.Print(".")
				lastDot = time.Now()
			}
		}
	}
}

// extractResultFromRawOutput tries to find a result from raw stream-json lines.
func extractResultFromRawOutput(rawContent string) string {
	lines := strings.Split(rawContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Look for result type lines
		if strings.Contains(line, `"type"`) && strings.Contains(line, `"result"`) {
			result := extractResultFromJSON(line)
			if result != "" {
				return result
			}
		}
	}
	return ""
}

// parseResultUsage extracts usage data from the result JSON line.
func parseResultUsage(jsonLine string) *cost.Usage {
	// Try to parse as JSON first
	var result struct {
		Usage struct {
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
			CacheReadTokens  int `json:"cache_read_input_tokens"`
			CacheWriteTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
		TotalCostUSD float64 `json:"total_cost_usd"`
	}

	if err := json.Unmarshal([]byte(jsonLine), &result); err == nil {
		return &cost.Usage{
			InputTokens:      result.Usage.InputTokens,
			OutputTokens:     result.Usage.OutputTokens,
			CacheReadTokens:  result.Usage.CacheReadTokens,
			CacheWriteTokens: result.Usage.CacheWriteTokens,
			TotalCostUSD:     result.TotalCostUSD,
		}
	}

	// Fallback: extract using string parsing (for malformed JSON)
	usage := &cost.Usage{}

	// Extract total_cost_usd
	if idx := strings.Index(jsonLine, `"total_cost_usd":`); idx != -1 {
		start := idx + len(`"total_cost_usd":`)
		end := start
		for end < len(jsonLine) && (jsonLine[end] == '.' || (jsonLine[end] >= '0' && jsonLine[end] <= '9')) {
			end++
		}
		if val, err := strconv.ParseFloat(jsonLine[start:end], 64); err == nil {
			usage.TotalCostUSD = val
		}
	}

	// Extract input_tokens
	if idx := strings.Index(jsonLine, `"input_tokens":`); idx != -1 {
		start := idx + len(`"input_tokens":`)
		end := start
		for end < len(jsonLine) && jsonLine[end] >= '0' && jsonLine[end] <= '9' {
			end++
		}
		if val, err := strconv.Atoi(jsonLine[start:end]); err == nil {
			usage.InputTokens = val
		}
	}

	// Extract output_tokens
	if idx := strings.Index(jsonLine, `"output_tokens":`); idx != -1 {
		start := idx + len(`"output_tokens":`)
		end := start
		for end < len(jsonLine) && jsonLine[end] >= '0' && jsonLine[end] <= '9' {
			end++
		}
		if val, err := strconv.Atoi(jsonLine[start:end]); err == nil {
			usage.OutputTokens = val
		}
	}

	// Extract cache_read_input_tokens
	if idx := strings.Index(jsonLine, `"cache_read_input_tokens":`); idx != -1 {
		start := idx + len(`"cache_read_input_tokens":`)
		end := start
		for end < len(jsonLine) && jsonLine[end] >= '0' && jsonLine[end] <= '9' {
			end++
		}
		if val, err := strconv.Atoi(jsonLine[start:end]); err == nil {
			usage.CacheReadTokens = val
		}
	}

	// Extract cache_creation_input_tokens
	if idx := strings.Index(jsonLine, `"cache_creation_input_tokens":`); idx != -1 {
		start := idx + len(`"cache_creation_input_tokens":`)
		end := start
		for end < len(jsonLine) && jsonLine[end] >= '0' && jsonLine[end] <= '9' {
			end++
		}
		if val, err := strconv.Atoi(jsonLine[start:end]); err == nil {
			usage.CacheWriteTokens = val
		}
	}

	if usage.IsEmpty() {
		return nil
	}
	return usage
}

// extractResultFromJSON extracts Claude's text result from the stream-json result line.
func extractResultFromJSON(jsonLine string) string {
	// The result line contains: {"type":"result","message":{"content":[{"type":"text","text":"..."}]},...}
	// We need to extract the text from message.content[].text

	// Try parsing as JSON first (most reliable)
	var result struct {
		Type    string `json:"type"`
		Result  string `json:"result"` // Some versions use this simple format
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}

	if err := json.Unmarshal([]byte(jsonLine), &result); err == nil {
		// Check for simple "result" field first (some CLI versions)
		if result.Result != "" {
			return result.Result
		}

		// Extract from message.content array
		var sb strings.Builder
		for _, content := range result.Message.Content {
			if content.Type == "text" && content.Text != "" {
				sb.WriteString(content.Text)
			}
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}

	// Fallback: try simple string extraction for "result":"..." format
	resultKey := `"result":"`
	startIdx := strings.Index(jsonLine, resultKey)
	if startIdx != -1 {
		startIdx += len(resultKey)
		return extractJSONString(jsonLine, startIdx)
	}

	// Fallback: try to extract from "text":"..." patterns in content array
	// Look for text fields within the content array
	textKey := `"text":"`
	var allText strings.Builder
	searchPos := 0
	for {
		idx := strings.Index(jsonLine[searchPos:], textKey)
		if idx == -1 {
			break
		}
		startIdx := searchPos + idx + len(textKey)
		text := extractJSONString(jsonLine, startIdx)
		if text != "" {
			allText.WriteString(text)
		}
		searchPos = startIdx + len(text) + 1
	}

	if allText.Len() > 0 {
		return allText.String()
	}

	return "" // Return empty if nothing found
}

// extractJSONString extracts a JSON string value starting at the given position.
// Handles escape sequences properly.
func extractJSONString(jsonLine string, startIdx int) string {
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
func (m *Manager) RunClaude(ctx context.Context, sess *Session, systemPrompt, userPrompt string) (string, *cost.Usage, error) {
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
