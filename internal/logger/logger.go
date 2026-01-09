// Package logger provides structured logging for boatman.
// Uses log/slog for structured, leveled logging with consistent formatting.
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

var (
	defaultLogger *slog.Logger
	once          sync.Once
)

// Level represents logging verbosity.
type Level = slog.Level

const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// Init initializes the default logger with the given options.
func Init(opts Options) {
	once.Do(func() {
		defaultLogger = New(opts)
		slog.SetDefault(defaultLogger)
	})
}

// Options configures the logger.
type Options struct {
	// Level is the minimum log level to output.
	Level Level

	// Output is where logs are written. Defaults to os.Stderr.
	Output io.Writer

	// JSON enables JSON output format instead of text.
	JSON bool

	// AddSource includes file:line in log output.
	AddSource bool
}

// DefaultOptions returns sensible defaults for CLI usage.
func DefaultOptions() Options {
	level := LevelInfo
	if os.Getenv("BOATMAN_DEBUG") == "1" {
		level = LevelDebug
	}

	return Options{
		Level:     level,
		Output:    os.Stderr,
		JSON:      false,
		AddSource: false,
	}
}

// New creates a new logger with the given options.
func New(opts Options) *slog.Logger {
	if opts.Output == nil {
		opts.Output = os.Stderr
	}

	handlerOpts := &slog.HandlerOptions{
		Level:     opts.Level,
		AddSource: opts.AddSource,
	}

	var handler slog.Handler
	if opts.JSON {
		handler = slog.NewJSONHandler(opts.Output, handlerOpts)
	} else {
		handler = slog.NewTextHandler(opts.Output, handlerOpts)
	}

	return slog.New(handler)
}

// Default returns the default logger, initializing it if necessary.
func Default() *slog.Logger {
	if defaultLogger == nil {
		Init(DefaultOptions())
	}
	return defaultLogger
}

// With returns a logger with the given attributes.
func With(args ...any) *slog.Logger {
	return Default().With(args...)
}

// WithComponent returns a logger tagged with a component name.
func WithComponent(name string) *slog.Logger {
	return Default().With("component", name)
}

// Debug logs at debug level.
func Debug(msg string, args ...any) {
	Default().Debug(msg, args...)
}

// Info logs at info level.
func Info(msg string, args ...any) {
	Default().Info(msg, args...)
}

// Warn logs at warn level.
func Warn(msg string, args ...any) {
	Default().Warn(msg, args...)
}

// Error logs at error level.
func Error(msg string, args ...any) {
	Default().Error(msg, args...)
}

// DebugContext logs at debug level with context.
func DebugContext(ctx context.Context, msg string, args ...any) {
	Default().DebugContext(ctx, msg, args...)
}

// InfoContext logs at info level with context.
func InfoContext(ctx context.Context, msg string, args ...any) {
	Default().InfoContext(ctx, msg, args...)
}

// WarnContext logs at warn level with context.
func WarnContext(ctx context.Context, msg string, args ...any) {
	Default().WarnContext(ctx, msg, args...)
}

// ErrorContext logs at error level with context.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	Default().ErrorContext(ctx, msg, args...)
}
