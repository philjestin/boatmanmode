package config

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestGetIntOrDefault(t *testing.T) {
	// Reset viper for clean test
	viper.Reset()

	// Test default value when key not set
	result := getIntOrDefault("test.key", 42)
	if result != 42 {
		t.Errorf("Expected default 42, got %d", result)
	}

	// Test with key set
	viper.Set("test.key", 100)
	result = getIntOrDefault("test.key", 42)
	if result != 100 {
		t.Errorf("Expected 100, got %d", result)
	}
}

func TestGetStringOrDefault(t *testing.T) {
	viper.Reset()

	// Test default value
	result := getStringOrDefault("test.str", "default")
	if result != "default" {
		t.Errorf("Expected 'default', got %s", result)
	}

	// Test with key set
	viper.Set("test.str", "custom")
	result = getStringOrDefault("test.str", "default")
	if result != "custom" {
		t.Errorf("Expected 'custom', got %s", result)
	}
}

func TestGetDurationOrDefault(t *testing.T) {
	viper.Reset()

	// Test default value
	result := getDurationOrDefault("test.duration", 5*time.Second)
	if result != 5*time.Second {
		t.Errorf("Expected 5s, got %v", result)
	}

	// Test with key set
	viper.Set("test.duration", 10*time.Second)
	result = getDurationOrDefault("test.duration", 5*time.Second)
	if result != 10*time.Second {
		t.Errorf("Expected 10s, got %v", result)
	}
}

func TestGetEnvOrViper(t *testing.T) {
	viper.Reset()

	// Test env var takes precedence
	os.Setenv("TEST_ENV_KEY", "from-env")
	defer os.Unsetenv("TEST_ENV_KEY")

	viper.Set("test_viper_key", "from-viper")

	result := getEnvOrViper("TEST_ENV_KEY", "test_viper_key")
	if result != "from-env" {
		t.Errorf("Expected 'from-env', got %s", result)
	}

	// Test fallback to viper when env not set
	os.Unsetenv("TEST_ENV_KEY")
	result = getEnvOrViper("TEST_ENV_KEY", "test_viper_key")
	if result != "from-viper" {
		t.Errorf("Expected 'from-viper', got %s", result)
	}
}

func TestConfigValidate(t *testing.T) {
	// Valid config
	cfg := &Config{LinearKey: "test-key"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Valid config should not error: %v", err)
	}

	// Invalid config - missing Linear key
	cfg = &Config{LinearKey: ""}
	if err := cfg.Validate(); err == nil {
		t.Error("Should error when Linear key is missing")
	}
}

func TestConfigDefaultValues(t *testing.T) {
	viper.Reset()
	os.Setenv("LINEAR_API_KEY", "test-api-key")
	defer os.Unsetenv("LINEAR_API_KEY")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Check defaults
	if cfg.MaxIterations != 3 {
		t.Errorf("Expected MaxIterations 3, got %d", cfg.MaxIterations)
	}
	if cfg.BaseBranch != "main" {
		t.Errorf("Expected BaseBranch 'main', got %s", cfg.BaseBranch)
	}

	// Coordinator defaults
	if cfg.Coordinator.MessageBufferSize != 1000 {
		t.Errorf("Expected MessageBufferSize 1000, got %d", cfg.Coordinator.MessageBufferSize)
	}
	if cfg.Coordinator.SubscriberBufferSize != 100 {
		t.Errorf("Expected SubscriberBufferSize 100, got %d", cfg.Coordinator.SubscriberBufferSize)
	}

	// Retry defaults
	if cfg.Retry.MaxAttempts != 3 {
		t.Errorf("Expected Retry.MaxAttempts 3, got %d", cfg.Retry.MaxAttempts)
	}
	if cfg.Retry.InitialDelay != 500*time.Millisecond {
		t.Errorf("Expected Retry.InitialDelay 500ms, got %v", cfg.Retry.InitialDelay)
	}
	if cfg.Retry.MaxDelay != 30*time.Second {
		t.Errorf("Expected Retry.MaxDelay 30s, got %v", cfg.Retry.MaxDelay)
	}

	// Claude defaults
	if cfg.Claude.Command != "claude" {
		t.Errorf("Expected Claude.Command 'claude', got %s", cfg.Claude.Command)
	}
	if cfg.Claude.LargePromptThreshold != 100000 {
		t.Errorf("Expected Claude.LargePromptThreshold 100000, got %d", cfg.Claude.LargePromptThreshold)
	}

	// Token budget defaults
	if cfg.TokenBudget.Context != 8000 {
		t.Errorf("Expected TokenBudget.Context 8000, got %d", cfg.TokenBudget.Context)
	}
}

func TestConfigCustomValues(t *testing.T) {
	viper.Reset()
	os.Setenv("LINEAR_API_KEY", "test-api-key")
	defer os.Unsetenv("LINEAR_API_KEY")

	// Set custom values
	viper.Set("max_iterations", 5)
	viper.Set("base_branch", "develop")
	viper.Set("coordinator.message_buffer_size", 2000)
	viper.Set("retry.max_attempts", 5)
	viper.Set("claude.command", "custom-claude")
	viper.Set("token_budget.context", 16000)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.MaxIterations != 5 {
		t.Errorf("Expected MaxIterations 5, got %d", cfg.MaxIterations)
	}
	if cfg.BaseBranch != "develop" {
		t.Errorf("Expected BaseBranch 'develop', got %s", cfg.BaseBranch)
	}
	if cfg.Coordinator.MessageBufferSize != 2000 {
		t.Errorf("Expected MessageBufferSize 2000, got %d", cfg.Coordinator.MessageBufferSize)
	}
	if cfg.Retry.MaxAttempts != 5 {
		t.Errorf("Expected Retry.MaxAttempts 5, got %d", cfg.Retry.MaxAttempts)
	}
	if cfg.Claude.Command != "custom-claude" {
		t.Errorf("Expected Claude.Command 'custom-claude', got %s", cfg.Claude.Command)
	}
	if cfg.TokenBudget.Context != 16000 {
		t.Errorf("Expected TokenBudget.Context 16000, got %d", cfg.TokenBudget.Context)
	}
}

func TestConfigDebugFromEnv(t *testing.T) {
	viper.Reset()
	os.Setenv("LINEAR_API_KEY", "test-api-key")
	defer os.Unsetenv("LINEAR_API_KEY")

	// Without debug
	cfg, _ := Load()
	if cfg.Debug {
		t.Error("Debug should be false by default")
	}

	// With debug
	os.Setenv("BOATMAN_DEBUG", "1")
	defer os.Unsetenv("BOATMAN_DEBUG")

	cfg, _ = Load()
	if !cfg.Debug {
		t.Error("Debug should be true when BOATMAN_DEBUG=1")
	}
}

func TestLoadWithoutLinearKey(t *testing.T) {
	viper.Reset()
	os.Unsetenv("LINEAR_API_KEY")

	_, err := Load()
	if err == nil {
		t.Error("Should error when LINEAR_API_KEY is not set")
	}
}

func TestCoordinatorConfig(t *testing.T) {
	cfg := CoordinatorConfig{
		MessageBufferSize:    500,
		SubscriberBufferSize: 50,
	}

	if cfg.MessageBufferSize != 500 {
		t.Errorf("Expected 500, got %d", cfg.MessageBufferSize)
	}
	if cfg.SubscriberBufferSize != 50 {
		t.Errorf("Expected 50, got %d", cfg.SubscriberBufferSize)
	}
}

func TestRetryConfig(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     1 * time.Minute,
	}

	if cfg.MaxAttempts != 5 {
		t.Errorf("Expected 5, got %d", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 1*time.Second {
		t.Errorf("Expected 1s, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 1*time.Minute {
		t.Errorf("Expected 1m, got %v", cfg.MaxDelay)
	}
}

func TestClaudeConfig(t *testing.T) {
	cfg := ClaudeConfig{
		Command:              "claude",
		UseTmux:              true,
		LargePromptThreshold: 50000,
		Timeout:              5 * time.Minute,
	}

	if cfg.Command != "claude" {
		t.Errorf("Expected 'claude', got %s", cfg.Command)
	}
	if !cfg.UseTmux {
		t.Error("UseTmux should be true")
	}
	if cfg.LargePromptThreshold != 50000 {
		t.Errorf("Expected 50000, got %d", cfg.LargePromptThreshold)
	}
	if cfg.Timeout != 5*time.Minute {
		t.Errorf("Expected 5m, got %v", cfg.Timeout)
	}
}

func TestTokenBudgetConfig(t *testing.T) {
	cfg := TokenBudgetConfig{
		Context: 8000,
		Plan:    2000,
		Review:  4000,
	}

	if cfg.Context != 8000 {
		t.Errorf("Expected 8000, got %d", cfg.Context)
	}
	if cfg.Plan != 2000 {
		t.Errorf("Expected 2000, got %d", cfg.Plan)
	}
	if cfg.Review != 4000 {
		t.Errorf("Expected 4000, got %d", cfg.Review)
	}
}
