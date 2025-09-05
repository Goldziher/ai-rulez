package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandPresets(t *testing.T) {
	tests := []struct {
		name        string
		presets     []string
		expected    int
		expectError bool
	}{
		{
			name:        "valid single preset",
			presets:     []string{"claude"},
			expected:    2,
			expectError: false,
		},
		{
			name:        "valid popular preset",
			presets:     []string{"popular"},
			expected:    4,
			expectError: false,
		},
		{
			name:        "multiple valid presets",
			presets:     []string{"claude", "cursor"},
			expected:    3,
			expectError: false,
		},
		{
			name:        "invalid preset",
			presets:     []string{"invalid-preset"},
			expected:    0,
			expectError: true,
		},
		{
			name:        "mixed valid and invalid presets",
			presets:     []string{"claude", "invalid-preset"},
			expected:    0,
			expectError: true,
		},
		{
			name:        "empty presets",
			presets:     []string{},
			expected:    0,
			expectError: false,
		},
		{
			name:        "duplicate presets",
			presets:     []string{"claude", "claude"},
			expected:    4,
			expectError: false,
		},
		{
			name:        "amp and codex presets (same output)",
			presets:     []string{"amp", "codex"},
			expected:    2,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputs, err := ExpandPresets(tt.presets)

			if tt.expectError {
				assert.Error(t, err)
				assert.IsType(t, ErrInvalidPreset{}, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, outputs, tt.expected)
			}
		})
	}
}

func TestPresetRegistry(t *testing.T) {
	tests := []struct {
		name          string
		preset        string
		expectedPaths []string
		shouldExist   bool
	}{
		{
			name:          "claude preset",
			preset:        "claude",
			expectedPaths: []string{"CLAUDE.md", ".claude/agents/"},
			shouldExist:   true,
		},
		{
			name:          "cursor preset",
			preset:        "cursor",
			expectedPaths: []string{".cursor/rules/"},
			shouldExist:   true,
		},
		{
			name:          "popular preset",
			preset:        "popular",
			expectedPaths: []string{"CLAUDE.md", ".cursor/rules/", ".windsurf/", ".github/copilot-instructions.md"},
			shouldExist:   true,
		},
		{
			name:        "nonexistent preset",
			preset:      "nonexistent",
			shouldExist: false,
		},
		{
			name:          "amp preset",
			preset:        "amp",
			expectedPaths: []string{"AGENTS.md"},
			shouldExist:   true,
		},
		{
			name:          "codex preset",
			preset:        "codex",
			expectedPaths: []string{"AGENTS.md"},
			shouldExist:   true,
		},
		{
			name:          "continue preset with multiple outputs",
			preset:        "continue-dev",
			expectedPaths: []string{".continue/rules/", ".continue/prompts/ai_rulez_prompts.yaml"},
			shouldExist:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputs, exists := PresetRegistry[tt.preset]

			if tt.shouldExist {
				assert.True(t, exists, "preset should exist in registry")
				assert.Len(t, outputs, len(tt.expectedPaths))

				for _, expectedPath := range tt.expectedPaths {
					found := false
					for _, output := range outputs {
						if output.Path == expectedPath {
							found = true
							break
						}
					}
					assert.True(t, found, "expected path %s not found", expectedPath)
				}
			} else {
				assert.False(t, exists, "preset should not exist in registry")
			}
		})
	}
}

func TestMergeOutputs(t *testing.T) {
	tests := []struct {
		name     string
		base     []Output
		override []Output
		expected []Output
	}{
		{
			name: "no conflicts",
			base: []Output{
				{Path: "CLAUDE.md"},
				{Path: ".cursor/rules/", Type: "rule"},
			},
			override: []Output{
				{Path: "GEMINI.md"},
			},
			expected: []Output{
				{Path: "CLAUDE.md"},
				{Path: ".cursor/rules/", Type: "rule"},
				{Path: "GEMINI.md"},
			},
		},
		{
			name: "override conflict",
			base: []Output{
				{Path: "CLAUDE.md"},
				{Path: ".cursor/rules/", Type: "rule"},
			},
			override: []Output{
				{Path: "CLAUDE.md", NamingScheme: "{name}.txt"},
			},
			expected: []Output{
				{Path: "CLAUDE.md", NamingScheme: "{name}.txt"},
				{Path: ".cursor/rules/", Type: "rule"},
			},
		},
		{
			name:     "empty base",
			base:     []Output{},
			override: []Output{{Path: "CLAUDE.md"}},
			expected: []Output{{Path: "CLAUDE.md"}},
		},
		{
			name:     "empty override",
			base:     []Output{{Path: "CLAUDE.md"}},
			override: []Output{},
			expected: []Output{{Path: "CLAUDE.md"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeOutputs(tt.base, tt.override)

			assert.Len(t, result, len(tt.expected))

			for _, expected := range tt.expected {
				found := false
				for _, actual := range result {
					if actual.Path == expected.Path &&
						actual.Type == expected.Type &&
						actual.NamingScheme == expected.NamingScheme {
						found = true
						break
					}
				}
				assert.True(t, found, "expected output %+v not found", expected)
			}
		})
	}
}

func TestExpandConfigPresets(t *testing.T) {
	tests := []struct {
		name            string
		config          *Config
		expectError     bool
		expectedOutputs int
	}{
		{
			name: "config with presets only",
			config: &Config{
				Presets: []string{"claude", "cursor"},
				Outputs: []Output{},
			},
			expectError:     false,
			expectedOutputs: 3,
		},
		{
			name: "config with presets and outputs - merge",
			config: &Config{
				Presets: []string{"claude"},
				Outputs: []Output{
					{Path: "GEMINI.md"},
				},
			},
			expectError:     false,
			expectedOutputs: 3,
		},
		{
			name: "config with presets and outputs - override",
			config: &Config{
				Presets: []string{"claude"},
				Outputs: []Output{
					{Path: "CLAUDE.md", NamingScheme: "custom"},
				},
			},
			expectError:     false,
			expectedOutputs: 2,
		},
		{
			name: "config with no presets",
			config: &Config{
				Presets: []string{},
				Outputs: []Output{
					{Path: "GEMINI.md"},
				},
			},
			expectError:     false,
			expectedOutputs: 1,
		},
		{
			name: "config with invalid preset",
			config: &Config{
				Presets: []string{"invalid"},
				Outputs: []Output{},
			},
			expectError:     true,
			expectedOutputs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalOutputsLen := len(tt.config.Outputs)

			err := expandConfigPresets(tt.config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, tt.config.Outputs, tt.expectedOutputs)

				if len(tt.config.Presets) > 0 {
					assert.GreaterOrEqual(t, len(tt.config.Outputs), originalOutputsLen)
				}
			}
		})
	}
}
