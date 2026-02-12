// Package config handles configuration management for boatman.
package config

import (
	"errors"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the boatman agent.
type Config struct {
	// Linear API
	LinearKey string

	// Workflow settings
	MaxIterations int
	BaseBranch    string
	AutoPR        bool
	ReviewSkill   string

	// Coordinator settings
	Coordinator CoordinatorConfig

	// Retry settings
	Retry RetryConfig

	// Claude settings
	Claude ClaudeConfig

	// Token budgets
	TokenBudget TokenBudgetConfig

	// Debug enables verbose logging
	Debug bool

	// EnableTools enables Claude CLI tool capabilities for agents
	EnableTools bool
}

// CoordinatorConfig holds coordinator-specific settings.
type CoordinatorConfig struct {
	// MessageBufferSize is the size of the main message channel buffer.
	MessageBufferSize int

	// SubscriberBufferSize is the size of per-subscriber channel buffers.
	SubscriberBufferSize int
}

// RetryConfig holds retry behavior settings.
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts.
	MaxAttempts int

	// InitialDelay is the initial delay before first retry.
	InitialDelay time.Duration

	// MaxDelay caps the maximum delay between retries.
	MaxDelay time.Duration
}

// ClaudeConfig holds Claude CLI settings.
type ClaudeConfig struct {
	// Command is the claude command to use.
	Command string

	// UseTmux enables tmux for large prompts.
	UseTmux bool

	// LargePromptThreshold is the character count above which to use tmux.
	LargePromptThreshold int

	// Timeout for Claude operations (0 = no timeout).
	Timeout time.Duration
}

// TokenBudgetConfig holds context token budget settings.
type TokenBudgetConfig struct {
	// Context is the token budget for context in prompts.
	Context int

	// Plan is the token budget for planning information.
	Plan int

	// Review is the token budget for review feedback.
	Review int
}

// Load reads configuration from viper and environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		LinearKey:     getEnvOrViper("LINEAR_API_KEY", "linear_key"),
		MaxIterations: getIntOrDefault("max_iterations", 3),
		BaseBranch:    getStringOrDefault("base_branch", "main"),
		AutoPR:        viper.GetBool("auto_pr"),
		ReviewSkill:   getStringOrDefault("review_skill", "peer-review"),
		Debug:         os.Getenv("BOATMAN_DEBUG") == "1",
		EnableTools:   viper.GetBool("enable_tools"),

		Coordinator: CoordinatorConfig{
			MessageBufferSize:    getIntOrDefault("coordinator.message_buffer_size", 1000),
			SubscriberBufferSize: getIntOrDefault("coordinator.subscriber_buffer_size", 100),
		},

		Retry: RetryConfig{
			MaxAttempts:  getIntOrDefault("retry.max_attempts", 3),
			InitialDelay: getDurationOrDefault("retry.initial_delay", 500*time.Millisecond),
			MaxDelay:     getDurationOrDefault("retry.max_delay", 30*time.Second),
		},

		Claude: ClaudeConfig{
			Command:              getStringOrDefault("claude.command", "claude"),
			UseTmux:              viper.GetBool("claude.use_tmux"),
			LargePromptThreshold: getIntOrDefault("claude.large_prompt_threshold", 100000),
			Timeout:              getDurationOrDefault("claude.timeout", 0),
		},

		TokenBudget: TokenBudgetConfig{
			Context: getIntOrDefault("token_budget.context", 8000),
			Plan:    getIntOrDefault("token_budget.plan", 2000),
			Review:  getIntOrDefault("token_budget.review", 4000),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that required configuration is present.
func (c *Config) Validate() error {
	if c.LinearKey == "" {
		return errors.New("linear API key is required (set LINEAR_API_KEY or --linear-key)")
	}
	return nil
}

// getEnvOrViper returns the value from environment variable or viper config.
func getEnvOrViper(envKey, viperKey string) string {
	if val := os.Getenv(envKey); val != "" {
		return val
	}
	return viper.GetString(viperKey)
}

// getIntOrDefault returns viper int value or default if not set.
func getIntOrDefault(key string, defaultVal int) int {
	if viper.IsSet(key) {
		return viper.GetInt(key)
	}
	return defaultVal
}

// getStringOrDefault returns viper string value or default if not set.
func getStringOrDefault(key string, defaultVal string) string {
	if viper.IsSet(key) {
		return viper.GetString(key)
	}
	return defaultVal
}

// getDurationOrDefault returns viper duration value or default if not set.
func getDurationOrDefault(key string, defaultVal time.Duration) time.Duration {
	if viper.IsSet(key) {
		return viper.GetDuration(key)
	}
	return defaultVal
}
