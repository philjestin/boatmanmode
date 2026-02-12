package claude

import (
	"testing"
)

func TestNewWithTools(t *testing.T) {
	tests := []struct {
		name          string
		tools         []string
		expectEnabled bool
	}{
		{
			name:          "All tools allowed (nil)",
			tools:         nil,
			expectEnabled: true,
		},
		{
			name:          "Specific tools allowed",
			tools:         []string{"Read", "Grep", "Glob"},
			expectEnabled: true,
		},
		{
			name:          "All tools disabled (empty slice)",
			tools:         []string{},
			expectEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewWithTools("/tmp", "test", tt.tools)

			if client.EnableTools != tt.expectEnabled {
				t.Errorf("EnableTools = %v, want %v", client.EnableTools, tt.expectEnabled)
			}

			if len(tt.tools) > 0 {
				if len(client.AllowedTools) != len(tt.tools) {
					t.Errorf("AllowedTools length = %d, want %d", len(client.AllowedTools), len(tt.tools))
				}
			}
		})
	}
}

func TestNewWithTmux_BackwardCompat(t *testing.T) {
	client := NewWithTmux("/tmp", "test")

	if client.EnableTools {
		t.Error("NewWithTmux should disable tools for backward compatibility")
	}

	if client.AllowedTools != nil {
		t.Error("NewWithTmux should have nil AllowedTools")
	}
}
