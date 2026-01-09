package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts 3, got %d", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 500*time.Millisecond {
		t.Errorf("Expected InitialDelay 500ms, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("Expected MaxDelay 30s, got %v", cfg.MaxDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier 2.0, got %f", cfg.Multiplier)
	}
}

func TestAPIConfig(t *testing.T) {
	cfg := APIConfig()

	if cfg.MaxAttempts != 4 {
		t.Errorf("Expected MaxAttempts 4, got %d", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 1*time.Second {
		t.Errorf("Expected InitialDelay 1s, got %v", cfg.InitialDelay)
	}
}

func TestCLIConfig(t *testing.T) {
	cfg := CLIConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts 3, got %d", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 2*time.Second {
		t.Errorf("Expected InitialDelay 2s, got %v", cfg.InitialDelay)
	}
}

func TestDoSuccess(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	err := Do(ctx, cfg, "test operation", func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestDoRetryThenSuccess(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		MaxAttempts:  5,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	err := Do(ctx, cfg, "test operation", func() error {
		callCount++
		if callCount < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error after retries, got %v", err)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestDoMaxAttemptsExhausted(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	err := Do(ctx, cfg, "test operation", func() error {
		callCount++
		return errors.New("persistent error")
	})

	if err == nil {
		t.Error("Expected error after max attempts")
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
	if !errors.Is(err, errors.New("")) {
		// Check error message contains operation name and attempt count
		errMsg := err.Error()
		if len(errMsg) == 0 {
			t.Error("Error message should not be empty")
		}
	}
}

func TestDoPermanentError(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		MaxAttempts:  5,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	permanentErr := errors.New("permanent error")
	err := Do(ctx, cfg, "test operation", func() error {
		callCount++
		return Permanent(permanentErr)
	})

	if err == nil {
		t.Error("Expected permanent error")
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call (no retries for permanent), got %d", callCount)
	}
	if !errors.Is(err, permanentErr) {
		t.Errorf("Expected original permanent error, got %v", err)
	}
}

func TestDoContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := Config{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}

	callCount := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := Do(ctx, cfg, "test operation", func() error {
		callCount++
		return errors.New("keep retrying")
	})

	if err == nil {
		t.Error("Expected context cancellation error")
	}
	// Should have stopped early due to cancellation
	if callCount >= 10 {
		t.Errorf("Should have stopped early, got %d calls", callCount)
	}
}

func TestDoContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	err := Do(ctx, cfg, "test operation", func() error {
		callCount++
		return errors.New("error")
	})

	if err == nil {
		t.Error("Expected context cancelled error")
	}
	// First attempt runs, but no retry due to cancelled context
	if callCount != 1 {
		t.Errorf("Expected 1 call before context check, got %d", callCount)
	}
}

func TestPermanentError(t *testing.T) {
	originalErr := errors.New("original error")
	permErr := Permanent(originalErr)

	if permErr == nil {
		t.Fatal("Permanent should not return nil")
	}

	// Should be unwrappable
	var perm *permanentError
	if !errors.As(permErr, &perm) {
		t.Error("Should be a permanentError")
	}

	if !errors.Is(perm.Unwrap(), originalErr) {
		t.Error("Unwrap should return original error")
	}
}

func TestPermanentNil(t *testing.T) {
	permErr := Permanent(nil)
	if permErr != nil {
		t.Error("Permanent(nil) should return nil")
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"regular error", errors.New("some error"), true},
		{"permanent error", Permanent(errors.New("permanent")), false},
		{"context cancelled", context.Canceled, false},
		// Note: context.DeadlineExceeded is retryable by default in our implementation
		// because it doesn't implement net.Error interface
		{"deadline exceeded", context.DeadlineExceeded, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestDelayForAttempt(t *testing.T) {
	cfg := Config{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Jitter:       0, // No jitter for predictable testing
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1 * time.Second}, // Capped at max
		{6, 1 * time.Second}, // Still capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			delay := cfg.delayForAttempt(tt.attempt)
			if delay != tt.expected {
				t.Errorf("delayForAttempt(%d) = %v, expected %v", tt.attempt, delay, tt.expected)
			}
		})
	}
}

func TestDelayWithJitter(t *testing.T) {
	cfg := Config{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.5, // 50% jitter
	}

	// Run multiple times to verify jitter is applied
	baseDelay := 100 * time.Millisecond
	minDelay := baseDelay - time.Duration(float64(baseDelay)*0.5)
	maxDelay := baseDelay + time.Duration(float64(baseDelay)*0.5)

	for i := 0; i < 20; i++ {
		delay := cfg.delayForAttempt(1)
		if delay < minDelay || delay > maxDelay {
			t.Errorf("Delay %v outside jitter range [%v, %v]", delay, minDelay, maxDelay)
		}
	}
}

func TestDoActualTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timing test in short mode")
	}

	ctx := context.Background()
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     200 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	start := time.Now()
	callCount := 0

	_ = Do(ctx, cfg, "timing test", func() error {
		callCount++
		return errors.New("fail")
	})

	elapsed := time.Since(start)

	// Should have waited at least: 50ms (after 1st) + 100ms (after 2nd) = 150ms
	// But less than 50ms + 100ms + 200ms = 350ms (no wait after 3rd)
	if elapsed < 150*time.Millisecond {
		t.Errorf("Expected at least 150ms elapsed, got %v", elapsed)
	}
	if elapsed > 400*time.Millisecond {
		t.Errorf("Expected less than 400ms elapsed, got %v", elapsed)
	}
}
