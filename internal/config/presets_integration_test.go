package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPresetsIntegrationWithLoadConfig(t *testing.T) {
	tests := []struct {
		name            string
		configYAML      string
		expectedOutputs int
		expectedPaths   []string
		expectError     bool
	}{
		{
			name: "config with only presets",
			configYAML: `
metadata:
  name: "TestProject"
presets:
  - "claude"
`,
			expectedOutputs: 3,
			expectedPaths:   []string{"CLAUDE.md", ".claude/agents/", ".mcp.json"},
			expectError:     false,
		},
		{
			name: "config with popular preset",
			configYAML: `
metadata:
  name: "TestProject"
presets:
  - "popular"
`,
			expectedOutputs: 9,
			expectedPaths:   []string{".cursor/rules/", ".github/copilot-instructions.md", ".windsurf/", "CLAUDE.md", ".mcp.json", ".gemini/settings.json", "GEMINI.md", "AGENTS.md", ".claude/agents/"},
			expectError:     false,
		},
		{
			name: "config with presets and outputs merged",
			configYAML: `
metadata:
  name: "TestProject"
presets:
  - "claude"
outputs:
  - path: "GEMINI.md"
`,
			expectedOutputs: 4,
			expectedPaths:   []string{"CLAUDE.md", ".claude/agents/", ".mcp.json", "GEMINI.md"},
			expectError:     false,
		},
		{
			name: "config with presets and outputs override",
			configYAML: `
metadata:
  name: "TestProject"
presets:
  - "claude"
outputs:
  - path: "CLAUDE.md"
    naming_scheme: "custom.txt"
`,
			expectedOutputs: 3,
			expectedPaths:   []string{"CLAUDE.md", ".claude/agents/", ".mcp.json"},
			expectError:     false,
		},
		{
			name: "config with multiple presets",
			configYAML: `
metadata:
  name: "TestProject"
presets:
  - "claude"
  - "cursor"
  - "gemini"
`,
			expectedOutputs: 6,
			expectedPaths:   []string{".cursor/rules/", "CLAUDE.md", ".claude/agents/", ".mcp.json", "GEMINI.md", ".gemini/settings.json"},
			expectError:     false,
		},
		{
			name: "config with continue preset (multiple outputs)",
			configYAML: `
metadata:
  name: "TestProject"
presets:
  - "continue-dev"
`,
			expectedOutputs: 2,
			expectedPaths:   []string{".continue/prompts/ai_rulez_prompts.yaml", ".continue/rules/"},
			expectError:     false,
		},
		{
			name: "config with invalid preset",
			configYAML: `
metadata:
  name: "TestProject"
presets:
  - "invalid-preset"
`,
			expectedOutputs: 0,
			expectError:     true,
		},
		{
			name: "config with amp and codex presets",
			configYAML: `
metadata:
  name: "TestProject"
presets:
  - "amp"
  - "codex"
`,
			expectedOutputs: 1,
			expectedPaths:   []string{"AGENTS.md"},
			expectError:     false,
		},
		{
			name: "config with junie preset",
			configYAML: `
metadata:
  name: "TestProject"
presets:
  - "junie"
`,
			expectedOutputs: 1,
			expectedPaths:   []string{".junie/guidelines.md"},
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "ai_rulez.yaml")
			err := os.WriteFile(configPath, []byte(tt.configYAML), 0o644)
			require.NoError(t, err)

			config, err := LoadConfigWithIncludes(context.Background(), configPath)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, config)

			assert.Len(t, config.Outputs, tt.expectedOutputs, "unexpected number of outputs")

			for _, expectedPath := range tt.expectedPaths {
				found := false
				for _, output := range config.Outputs {
					if output.Path == expectedPath {
						found = true
						break
					}
				}
				assert.True(t, found, "expected path %s not found in outputs", expectedPath)
			}

			if len(config.Outputs) > 1 {
				for i := 1; i < len(config.Outputs); i++ {
					assert.LessOrEqual(t, config.Outputs[i-1].Path, config.Outputs[i].Path,
						"outputs should be sorted by path")
				}
			}
		})
	}
}

func TestPresetsValidation(t *testing.T) {
	tests := []struct {
		name        string
		configYAML  string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config with presets only",
			configYAML: `
metadata:
  name: "TestProject"
presets:
  - "claude"
`,
			expectError: false,
		},
		{
			name: "valid config with outputs only",
			configYAML: `
metadata:
  name: "TestProject"
outputs:
  - path: "CLAUDE.md"
`,
			expectError: false,
		},
		{
			name: "invalid config with neither presets nor outputs",
			configYAML: `
metadata:
  name: "TestProject"
`,
			expectError: true,
			errorMsg:    "either 'outputs' or 'presets' is required",
		},
		{
			name: "valid config with both presets and outputs",
			configYAML: `
metadata:
  name: "TestProject"
presets:
  - "claude"
outputs:
  - path: "GEMINI.md"
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config Config
			err := yaml.Unmarshal([]byte(tt.configYAML), &config)
			require.NoError(t, err)

			err = config.Validate()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPresetsWithExtendsAndIncludes(t *testing.T) {
	tmpDir := t.TempDir()

	baseConfig := `
metadata:
  name: "BaseProject"
presets:
  - "claude"
`
	baseConfigPath := filepath.Join(tmpDir, "base.yaml")
	err := os.WriteFile(baseConfigPath, []byte(baseConfig), 0o644)
	require.NoError(t, err)

	includeConfig := `
outputs:
  - path: "CUSTOM.md"
rules:
  - name: "Test Rule"
    content: "Test content"
`
	includeConfigPath := filepath.Join(tmpDir, "include.yaml")
	err = os.WriteFile(includeConfigPath, []byte(includeConfig), 0o644)
	require.NoError(t, err)

	mainConfig := `
metadata:
  name: "MainProject"
extends: "./base.yaml"
includes:
  - "./include.yaml"
presets:
  - "cursor"
`
	mainConfigPath := filepath.Join(tmpDir, "main.yaml")
	err = os.WriteFile(mainConfigPath, []byte(mainConfig), 0o644)
	require.NoError(t, err)

	config, err := LoadConfigWithIncludes(context.Background(), mainConfigPath)
	require.NoError(t, err)
	assert.NotNil(t, config)

	assert.Len(t, config.Outputs, 4)

	paths := make(map[string]bool)
	for _, output := range config.Outputs {
		paths[output.Path] = true
	}
	assert.True(t, paths["CLAUDE.md"], "should have CLAUDE.md from base preset")
	assert.True(t, paths[".claude/agents/"], "should have .claude/agents/ from base preset")
	assert.True(t, paths[".mcp.json"], "should have .mcp.json from base preset")
	assert.True(t, paths[".cursor/rules/"], "should have .cursor/rules/ from main preset")

	assert.Len(t, config.Rules, 1)
	assert.Equal(t, "Test Rule", config.Rules[0].Name)
}

func TestPresetsSortedOutput(t *testing.T) {
	config := &Config{
		Presets: []string{"popular"},
	}

	for i := 0; i < 10; i++ {
		config.Outputs = []Output{}

		err := expandConfigPresets(config)
		require.NoError(t, err)

		var paths []string
		for _, output := range config.Outputs {
			paths = append(paths, output.Path)
		}

		for j := 1; j < len(paths); j++ {
			assert.LessOrEqual(t, paths[j-1], paths[j],
				"outputs should be consistently sorted on iteration %d", i)
		}

		if i == 0 {
			expectedPaths := paths
			for j := 1; j < 10; j++ {
				config.Outputs = []Output{}
				err := expandConfigPresets(config)
				require.NoError(t, err)

				var newPaths []string
				for _, output := range config.Outputs {
					newPaths = append(newPaths, output.Path)
				}

				assert.Equal(t, expectedPaths, newPaths,
					"output order should be consistent across runs")
			}
			break
		}
	}
}

func TestPresetsErrorMessages(t *testing.T) {
	config := &Config{
		Presets: []string{"invalid-preset"},
	}

	err := expandConfigPresets(config)
	assert.Error(t, err)

	errorMsg := err.Error()
	assert.Contains(t, errorMsg, "invalid-preset")
}
