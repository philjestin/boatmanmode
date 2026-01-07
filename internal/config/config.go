// Package config handles configuration management for boatman.
package config

import (
	"errors"
	"os"

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
}

// Load reads configuration from viper and environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		LinearKey:     getEnvOrViper("LINEAR_API_KEY", "linear_key"),
		MaxIterations: viper.GetInt("max_iterations"),
		BaseBranch:    viper.GetString("base_branch"),
		AutoPR:        viper.GetBool("auto_pr"),
	}

	// Set defaults
	if cfg.MaxIterations == 0 {
		cfg.MaxIterations = 3
	}
	if cfg.BaseBranch == "" {
		cfg.BaseBranch = "main"
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
