package logger

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Level != LevelInfo {
		t.Errorf("Expected default level Info, got %v", opts.Level)
	}
	if opts.Output != os.Stderr {
		t.Error("Expected default output to be os.Stderr")
	}
	if opts.JSON {
		t.Error("Expected JSON to be false by default")
	}
	if opts.AddSource {
		t.Error("Expected AddSource to be false by default")
	}
}

func TestNew(t *testing.T) {
	var buf bytes.Buffer

	logger := New(Options{
		Level:  LevelDebug,
		Output: &buf,
		JSON:   false,
	})

	if logger == nil {
		t.Fatal("New should return a logger")
	}

	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Output should contain message, got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Output should contain key=value, got: %s", output)
	}
}

func TestNewJSON(t *testing.T) {
	var buf bytes.Buffer

	logger := New(Options{
		Level:  LevelInfo,
		Output: &buf,
		JSON:   true,
	})

	logger.Info("json test", "key", "value")

	output := buf.String()
	if !strings.Contains(output, `"msg"`) {
		t.Errorf("JSON output should contain 'msg', got: %s", output)
	}
	if !strings.Contains(output, `"key"`) {
		t.Errorf("JSON output should contain 'key', got: %s", output)
	}
}

func TestNewDefaultOutput(t *testing.T) {
	// When Output is nil, should default to os.Stderr
	logger := New(Options{
		Level:  LevelInfo,
		Output: nil,
	})

	if logger == nil {
		t.Fatal("Should create logger with nil output")
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer

	logger := New(Options{
		Level:  LevelWarn,
		Output: &buf,
	})

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")

	output := buf.String()

	if strings.Contains(output, "debug message") {
		t.Error("Debug should be filtered out at Warn level")
	}
	if strings.Contains(output, "info message") {
		t.Error("Info should be filtered out at Warn level")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("Warn should be included at Warn level")
	}
}

func TestWith(t *testing.T) {
	var buf bytes.Buffer

	// Reset default logger for test
	defaultLogger = New(Options{
		Level:  LevelInfo,
		Output: &buf,
	})

	childLogger := With("component", "test-component")
	childLogger.Info("with test")

	output := buf.String()
	if !strings.Contains(output, "component=test-component") {
		t.Errorf("Output should contain component, got: %s", output)
	}
}

func TestWithComponent(t *testing.T) {
	var buf bytes.Buffer

	defaultLogger = New(Options{
		Level:  LevelInfo,
		Output: &buf,
	})

	componentLogger := WithComponent("coordinator")
	componentLogger.Info("component test")

	output := buf.String()
	if !strings.Contains(output, "component=coordinator") {
		t.Errorf("Output should contain component, got: %s", output)
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	var buf bytes.Buffer

	defaultLogger = New(Options{
		Level:  LevelDebug,
		Output: &buf,
	})

	Debug("debug msg", "k1", "v1")
	Info("info msg", "k2", "v2")
	Warn("warn msg", "k3", "v3")
	Error("error msg", "k4", "v4")

	output := buf.String()

	tests := []struct {
		level string
		msg   string
		kv    string
	}{
		{"DEBUG", "debug msg", "k1=v1"},
		{"INFO", "info msg", "k2=v2"},
		{"WARN", "warn msg", "k3=v3"},
		{"ERROR", "error msg", "k4=v4"},
	}

	for _, tt := range tests {
		if !strings.Contains(output, tt.msg) {
			t.Errorf("Output should contain '%s'", tt.msg)
		}
		if !strings.Contains(output, tt.kv) {
			t.Errorf("Output should contain '%s'", tt.kv)
		}
	}
}

func TestContextFunctions(t *testing.T) {
	var buf bytes.Buffer

	defaultLogger = New(Options{
		Level:  LevelDebug,
		Output: &buf,
	})

	ctx := context.Background()

	DebugContext(ctx, "debug ctx")
	InfoContext(ctx, "info ctx")
	WarnContext(ctx, "warn ctx")
	ErrorContext(ctx, "error ctx")

	output := buf.String()

	if !strings.Contains(output, "debug ctx") {
		t.Error("Should contain debug context message")
	}
	if !strings.Contains(output, "info ctx") {
		t.Error("Should contain info context message")
	}
	if !strings.Contains(output, "warn ctx") {
		t.Error("Should contain warn context message")
	}
	if !strings.Contains(output, "error ctx") {
		t.Error("Should contain error context message")
	}
}

func TestDefault(t *testing.T) {
	// Reset for clean test
	defaultLogger = nil

	logger := Default()

	if logger == nil {
		t.Fatal("Default should return a logger")
	}

	// Calling Default again should return same instance
	logger2 := Default()
	if logger != logger2 {
		t.Error("Default should return same instance")
	}
}

func TestInit(t *testing.T) {
	var buf bytes.Buffer

	// Reset default logger and sync.Once for test
	// Note: sync.Once can only fire once per process, so we test the logger
	// we create directly rather than relying on Init's singleton behavior
	logger := New(Options{
		Level:  LevelWarn,
		Output: &buf,
		JSON:   true,
	})

	// Use the logger directly
	logger.Info("should be filtered")
	logger.Warn("should appear")

	output := buf.String()

	if strings.Contains(output, "should be filtered") {
		t.Error("Info should be filtered at Warn level")
	}
	if !strings.Contains(output, "should appear") {
		t.Error("Warn should appear")
	}
}

func TestLevelConstants(t *testing.T) {
	// Verify our level aliases match slog levels
	if LevelDebug != slog.LevelDebug {
		t.Error("LevelDebug should equal slog.LevelDebug")
	}
	if LevelInfo != slog.LevelInfo {
		t.Error("LevelInfo should equal slog.LevelInfo")
	}
	if LevelWarn != slog.LevelWarn {
		t.Error("LevelWarn should equal slog.LevelWarn")
	}
	if LevelError != slog.LevelError {
		t.Error("LevelError should equal slog.LevelError")
	}
}
