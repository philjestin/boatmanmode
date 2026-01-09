// Package retry provides exponential backoff retry logic for transient failures.
package retry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"time"
)

// Config defines retry behavior.
type Config struct {
	// MaxAttempts is the maximum number of attempts (including the first).
	MaxAttempts int

	// InitialDelay is the delay before the first retry.
	InitialDelay time.Duration

	// MaxDelay caps the delay between retries.
	MaxDelay time.Duration

	// Multiplier is the factor by which delay increases each retry.
	Multiplier float64

	// Jitter adds randomness to delays (0-1, where 0.1 = 10% jitter).
	Jitter float64
}

// DefaultConfig returns sensible retry defaults.
func DefaultConfig() Config {
	return Config{
		MaxAttempts:  3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
	}
}

// APIConfig returns retry config tuned for API calls.
func APIConfig() Config {
	return Config{
		MaxAttempts:  4,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.2,
	}
}

// CLIConfig returns retry config tuned for CLI tool invocations.
func CLIConfig() Config {
	return Config{
		MaxAttempts:  3,
		InitialDelay: 2 * time.Second,
		MaxDelay:     15 * time.Second,
		Multiplier:   1.5,
		Jitter:       0.1,
	}
}

// Do executes the function with retry logic.
// The function should return an error that is retryable (network errors, 5xx, etc.)
// or a permanent error wrapped with Permanent().
func Do(ctx context.Context, cfg Config, operation string, fn func() error) error {
	var lastErr error

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is permanent (should not retry)
		var permErr *permanentError
		if errors.As(err, &permErr) {
			return permErr.Unwrap()
		}

		// Check context cancellation
		if ctx.Err() != nil {
			return fmt.Errorf("%s failed: %w (context cancelled)", operation, lastErr)
		}

		// Don't sleep after last attempt
		if attempt >= cfg.MaxAttempts {
			break
		}

		// Calculate delay with exponential backoff and jitter
		delay := cfg.delayForAttempt(attempt)

		slog.Debug("retrying operation",
			"operation", operation,
			"attempt", attempt,
			"max_attempts", cfg.MaxAttempts,
			"delay", delay,
			"error", err.Error())

		select {
		case <-ctx.Done():
			return fmt.Errorf("%s failed: %w (context cancelled during backoff)", operation, lastErr)
		case <-time.After(delay):
		}
	}

	return fmt.Errorf("%s failed after %d attempts: %w", operation, cfg.MaxAttempts, lastErr)
}

// delayForAttempt calculates the delay for a given attempt number.
func (c Config) delayForAttempt(attempt int) time.Duration {
	delay := float64(c.InitialDelay) * math.Pow(c.Multiplier, float64(attempt-1))

	// Add jitter
	if c.Jitter > 0 {
		jitterRange := delay * c.Jitter
		delay += (rand.Float64()*2 - 1) * jitterRange
	}

	// Cap at max delay
	if delay > float64(c.MaxDelay) {
		delay = float64(c.MaxDelay)
	}

	return time.Duration(delay)
}

// permanentError wraps an error that should not be retried.
type permanentError struct {
	err error
}

func (e *permanentError) Error() string {
	return e.err.Error()
}

func (e *permanentError) Unwrap() error {
	return e.err
}

// Permanent wraps an error to indicate it should not be retried.
// Use this for validation errors, 4xx responses, etc.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// IsRetryable returns true if the error is likely transient.
// This checks for network errors and common transient conditions.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for permanent error wrapper
	var permErr *permanentError
	if errors.As(err, &permErr) {
		return false
	}

	// Network errors are retryable
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	// Context cancellation is not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Default to retryable for unknown errors
	return true
}
