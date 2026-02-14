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

	// Review pass criteria
	Review ReviewConfig

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

// ReviewConfig holds review pass criteria settings.
type ReviewConfig struct {
	// MaxCriticalIssues is the maximum number of critical issues allowed to pass.
	MaxCriticalIssues int

	// MaxMajorIssues is the maximum number of major issues allowed to pass.
	MaxMajorIssues int

	// MinVerificationConfidence is the minimum confidence percentage (0-100) for diff verification.
	MinVerificationConfidence int

	// StrictParsing enables strict keyword matching in natural language review parsing.
	StrictParsing bool
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

	// Model configuration per agent type
	Models ModelConfig

	// EnablePromptCaching enables prompt caching for cost reduction.
	// Note: Requires Claude CLI version that supports --cache-system-prompt flag.
	// Set to true only if your CLI version supports it.
	EnablePromptCaching bool
}

// ModelConfig holds model selection per agent type.
// Leave empty to use the Claude CLI's default model.
// Model names vary by provider (Anthropic API vs Vertex AI vs AWS Bedrock).
type ModelConfig struct {
	// Planner model for planning phase (empty = CLI default)
	Planner string

	// Executor model for code generation (empty = CLI default)
	Executor string

	// Reviewer model for code review (empty = CLI default)
	Reviewer string

	// Refactor model for fixing issues (empty = CLI default)
	Refactor string

	// Preflight model for validation (empty = CLI default)
	Preflight string

	// TestRunner model for test output parsing (empty = CLI default)
	TestRunner string
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
		MaxIterations: getIntOrDefault("max_iterations", 5), // Increased from 3 to 5
		BaseBranch:    getStringOrDefault("base_branch", "main"),
		AutoPR:        viper.GetBool("auto_pr"),
		ReviewSkill:   getStringOrDefault("review_skill", "peer-review"),
		Debug:         os.Getenv("BOATMAN_DEBUG") == "1",
		EnableTools:   viper.GetBool("enable_tools"),

		Review: ReviewConfig{
			MaxCriticalIssues:         getIntOrDefault("review.max_critical_issues", 1),    // Allow 1 critical (was 0)
			MaxMajorIssues:            getIntOrDefault("review.max_major_issues", 3),       // Allow 3 major (was 2)
			MinVerificationConfidence: getIntOrDefault("review.min_verification_confidence", 50), // 50% confidence threshold
			StrictParsing:             getBoolOrDefault("review.strict_parsing", false),    // Relaxed by default
		},

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
			EnablePromptCaching:  getBoolOrDefault("claude.enable_prompt_caching", false),
			Models: ModelConfig{
				Planner:    getStringOrDefault("claude.models.planner", ""),    // Empty = use CLI default
				Executor:   getStringOrDefault("claude.models.executor", ""),   // Empty = use CLI default
				Reviewer:   getStringOrDefault("claude.models.reviewer", ""),   // Empty = use CLI default
				Refactor:   getStringOrDefault("claude.models.refactor", ""),   // Empty = use CLI default
				Preflight:  getStringOrDefault("claude.models.preflight", ""),  // Empty = use CLI default
				TestRunner: getStringOrDefault("claude.models.test_runner", ""), // Empty = use CLI default
			},
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

// getBoolOrDefault returns viper bool value or default if not set.
func getBoolOrDefault(key string, defaultVal bool) bool {
	if viper.IsSet(key) {
		return viper.GetBool(key)
	}
	return defaultVal
}
