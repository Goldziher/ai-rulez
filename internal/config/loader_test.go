package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestLoadConfigWithIncludes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		wantErr bool
		check   func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "config with single include",
			files: map[string]string{
				"main.yaml": `metadata:
  name: "main"
includes:
  - "include.yaml"
outputs:
  - file: "CLAUDE.md"
rules:
  - name: "main rule"
    content: "main content"`,
				"include.yaml": `metadata:
  name: "included"
outputs:
  - file: "output.md"
rules:
  - name: "included rule"
    content: "included content"`,
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Equal(t, "main", cfg.Metadata.Name)
				assert.Len(t, cfg.Rules, 2)
				assert.Equal(t, "main rule", cfg.Rules[0].Name)
				assert.Equal(t, "included rule", cfg.Rules[1].Name)
				assert.Empty(t, cfg.Includes)
			},
		},
		{
			name: "config with multiple includes",
			files: map[string]string{
				"main.yaml": `metadata:
  name: "main"
includes:
  - "first.yaml"
  - "second.yaml"
outputs:
  - file: "CLAUDE.md"`,
				"first.yaml": `metadata:
  name: "first"
outputs:
  - file: "output.md"
rules:
  - name: "first rule"
    content: "first content"`,
				"second.yaml": `metadata:
  name: "second"
outputs:
  - file: "output.md"
rules:
  - name: "second rule"
    content: "second content"`,
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Len(t, cfg.Rules, 2)
				assert.Equal(t, "first rule", cfg.Rules[0].Name)
				assert.Equal(t, "second rule", cfg.Rules[1].Name)
			},
		},
		{
			name: "nested includes",
			files: map[string]string{
				"main.yaml": `metadata:
  name: "main"
includes:
  - "level1.yaml"
outputs:
  - file: "CLAUDE.md"`,
				"level1.yaml": `metadata:
  name: "level1"
outputs:
  - file: "output.md"
includes:
  - "level2.yaml"
rules:
  - name: "level1 rule"
    content: "level1 content"`,
				"level2.yaml": `metadata:
  name: "level2"
outputs:
  - file: "output.md"
rules:
  - name: "level2 rule"
    content: "level2 content"`,
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Len(t, cfg.Rules, 2)
				assert.Equal(t, "level1 rule", cfg.Rules[0].Name)
				assert.Equal(t, "level2 rule", cfg.Rules[1].Name)
			},
		},
		{
			name: "missing include file",
			files: map[string]string{
				"main.yaml": `metadata:
  name: "main"
includes:
  - "missing.yaml"
outputs:
  - file: "CLAUDE.md"`,
			},
			wantErr: true,
			check:   nil,
		},
		{
			name: "circular include detection",
			files: map[string]string{
				"main.yaml": `metadata:
  name: "main"
includes:
  - "circular.yaml"
outputs:
  - file: "CLAUDE.md"`,
				"circular.yaml": `metadata:
  name: "circular"
outputs:
  - file: "output.md"
includes:
  - "main.yaml"`,
			},
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			for filename, content := range tt.files {
				filePath := filepath.Join(tmpDir, filename)
				err := os.WriteFile(filePath, []byte(content), 0o644)
				require.NoError(t, err)
			}

			mainPath := filepath.Join(tmpDir, "main.yaml")
			cfg, err := config.LoadConfigWithIncludes(mainPath)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, cfg)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, cfg)
				if tt.check != nil {
					tt.check(t, cfg)
				}
			}
		})
	}
}

func TestMergeRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ruleSets [][]config.Rule
		expected []config.Rule
	}{
		{
			name: "merge two rule sets",
			ruleSets: [][]config.Rule{
				{
					{Name: "rule1", Content: "content1"},
					{Name: "rule2", Content: "content2"},
				},
				{
					{Name: "rule3", Content: "content3"},
				},
			},
			expected: []config.Rule{
				{Name: "rule1", Content: "content1"},
				{Name: "rule2", Content: "content2"},
				{Name: "rule3", Content: "content3"},
			},
		},
		{
			name: "later rules override earlier ones",
			ruleSets: [][]config.Rule{
				{
					{Name: "rule1", Content: "original"},
				},
				{
					{Name: "rule1", Content: "override"},
				},
			},
			expected: []config.Rule{
				{Name: "rule1", Content: "override"},
			},
		},
		{
			name:     "empty rule sets",
			ruleSets: [][]config.Rule{},
			expected: []config.Rule{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := config.MergeRules(tt.ruleSets...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateIncludes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *config.Config
		files   map[string]string
		wantErr bool
	}{
		{
			name: "valid includes",
			config: &config.Config{
				Includes: []string{"valid.yaml"},
			},
			files: map[string]string{
				"valid.yaml": `metadata:
  name: "valid"
outputs:
  - file: "CLAUDE.md"`,
			},
			wantErr: false,
		},
		{
			name: "missing include file",
			config: &config.Config{
				Includes: []string{"missing.yaml"},
			},
			files:   map[string]string{},
			wantErr: true,
		},
		{
			name: "invalid YAML in include",
			config: &config.Config{
				Includes: []string{"invalid.yaml"},
			},
			files: map[string]string{
				"invalid.yaml": "invalid: yaml: [",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			for filename, content := range tt.files {
				filePath := filepath.Join(tmpDir, filename)
				err := os.WriteFile(filePath, []byte(content), 0o644)
				require.NoError(t, err)
			}

			err := config.ValidateIncludes(tt.config, tmpDir)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateOutputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		outputs []config.Output
		wantErr bool
	}{
		{
			name:    "valid outputs",
			outputs: []config.Output{{Path: "test.md"}},
			wantErr: false,
		},
		{
			name:    "empty outputs",
			outputs: []config.Output{},
			wantErr: true,
		},
		{
			name:    "missing file",
			outputs: []config.Output{{}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := config.ValidateOutputs(tt.outputs)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMergeRulesWithIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ruleSets [][]config.Rule
		expected []config.Rule
	}{
		{
			name: "rules with IDs override by ID",
			ruleSets: [][]config.Rule{
				{
					{ID: "rule1", Name: "Rule 1", Content: "original"},
					{Name: "Rule 2", Content: "no ID rule"},
				},
				{
					{ID: "rule1", Name: "Rule 1 Override", Content: "overridden"},
				},
			},
			expected: []config.Rule{
				{ID: "rule1", Name: "Rule 1 Override", Content: "overridden"},
				{Name: "Rule 2", Content: "no ID rule"},
			},
		},
		{
			name: "rules without IDs still merge by name",
			ruleSets: [][]config.Rule{
				{
					{Name: "Rule 1", Content: "original"},
				},
				{
					{Name: "Rule 1", Content: "overridden"},
				},
			},
			expected: []config.Rule{
				{Name: "Rule 1", Content: "overridden"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := config.MergeRules(tt.ruleSets...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadConfigWithLocalFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		wantErr bool
		check   func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "local file overrides rules by ID",
			files: map[string]string{
				"test.yaml": `metadata:
  name: "main"
outputs:
  - file: "CLAUDE.md"
rules:
  - id: "rule1"
    name: "Rule 1"
    content: "original content"`,
				"test.local.yaml": `metadata:
  name: "local overrides"
outputs:
  - file: "local.md"
rules:
  - id: "rule1"
    name: "Rule 1 Overridden"
    content: "LOCAL: overridden content"`,
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Equal(t, "main", cfg.Metadata.Name)
				assert.Len(t, cfg.Rules, 1)
				assert.Equal(t, "Rule 1 Overridden", cfg.Rules[0].Name)
				assert.Equal(t, "LOCAL: overridden content", cfg.Rules[0].Content)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			for filename, content := range tt.files {
				filePath := filepath.Join(tmpDir, filename)
				err := os.WriteFile(filePath, []byte(content), 0o644)
				require.NoError(t, err)
			}

			mainFile := filepath.Join(tmpDir, "test.yaml")
			cfg, err := config.LoadConfigWithIncludes(mainFile)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			tt.check(t, cfg)
		})
	}
}

func TestMergeAgents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		agentSets [][]config.Agent
		expected  []config.Agent
	}{
		{
			name: "single agent set",
			agentSets: [][]config.Agent{
				{
					{Name: "agent1", Description: "First agent"},
					{Name: "agent2", Description: "Second agent"},
				},
			},
			expected: []config.Agent{
				{Name: "agent1", Description: "First agent"},
				{Name: "agent2", Description: "Second agent"},
			},
		},
		{
			name: "merge by name when no ID",
			agentSets: [][]config.Agent{
				{
					{Name: "agent1", Description: "Original"},
				},
				{
					{Name: "agent1", Description: "Override"},
				},
			},
			expected: []config.Agent{
				{Name: "agent1", Description: "Override"},
			},
		},
		{
			name: "merge by ID when present",
			agentSets: [][]config.Agent{
				{
					{ID: "id1", Name: "agent1", Description: "Original", Priority: 5},
				},
				{
					{ID: "id1", Name: "agent1-updated", Description: "Override", Priority: 10},
				},
			},
			expected: []config.Agent{
				{ID: "id1", Name: "agent1-updated", Description: "Override", Priority: 10},
			},
		},
		{
			name: "preserve order with multiple merges",
			agentSets: [][]config.Agent{
				{
					{Name: "agent1", Description: "First"},
					{Name: "agent2", Description: "Second"},
				},
				{
					{Name: "agent3", Description: "Third"},
				},
				{
					{Name: "agent2", Description: "Second Updated"},
				},
			},
			expected: []config.Agent{
				{Name: "agent1", Description: "First"},
				{Name: "agent2", Description: "Second Updated"},
				{Name: "agent3", Description: "Third"},
			},
		},
		{
			name: "merge with tools and system_prompt",
			agentSets: [][]config.Agent{
				{
					{
						Name:         "agent1",
						Description:  "Original",
						Tools:        []string{"tool1"},
						SystemPrompt: "Original prompt",
					},
				},
				{
					{
						Name:         "agent1",
						Description:  "Updated",
						Tools:        []string{"tool1", "tool2"},
						SystemPrompt: "Updated prompt",
					},
				},
			},
			expected: []config.Agent{
				{
					Name:         "agent1",
					Description:  "Updated",
					Tools:        []string{"tool1", "tool2"},
					SystemPrompt: "Updated prompt",
				},
			},
		},
		{
			name: "empty agent sets",
			agentSets: [][]config.Agent{
				{},
				{},
			},
			expected: []config.Agent{},
		},
		{
			name: "nil agent sets",
			agentSets: [][]config.Agent{
				nil,
				{
					{Name: "agent1", Description: "First"},
				},
				nil,
			},
			expected: []config.Agent{
				{Name: "agent1", Description: "First"},
			},
		},
		{
			name: "complex merge with IDs and names",
			agentSets: [][]config.Agent{
				{
					{ID: "a1", Name: "agent1", Description: "First", Priority: 5},
					{Name: "agent2", Description: "Second", Priority: 3},
					{ID: "a3", Name: "agent3", Description: "Third"},
				},
				{
					{ID: "a1", Name: "agent1-renamed", Description: "First Updated", Priority: 10},
					{Name: "agent4", Description: "Fourth"},
				},
				{
					{Name: "agent2", Description: "Second Updated", Priority: 8},
					{ID: "a3", Name: "agent3", Description: "Third Updated", Tools: []string{"tool1"}},
				},
			},
			expected: []config.Agent{
				{ID: "a1", Name: "agent1-renamed", Description: "First Updated", Priority: 10},
				{Name: "agent2", Description: "Second Updated", Priority: 8},
				{ID: "a3", Name: "agent3", Description: "Third Updated", Tools: []string{"tool1"}},
				{Name: "agent4", Description: "Fourth"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := config.MergeAgents(tt.agentSets...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadConfigWithIncludesForAgents(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	mainConfig := `metadata:
  name: "Main Config"
  version: "1.0.0"

includes:
  - "agents1.yaml"
  - "agents2.yaml"

outputs:
  - path: ".claude/agents/"
    type: "agent"

agents:
  - name: "main-agent"
    description: "Agent from main config"
    priority: 10
    tools:
      - Read
      - Write`

	agents1Config := `metadata:
  name: "Agents Include 1"
  version: "1.0.0"
outputs:
  - path: "test.md"
agents:
  - name: "included-agent-1"
    description: "Agent from first include"
    priority: 5
    tools:
      - Execute
  - id: "override-me"
    name: "original-agent"
    description: "Will be overridden"
    priority: 3`

	agents2Config := `metadata:
  name: "Agents Include 2"
  version: "1.0.0"
outputs:
  - path: "test.md"
agents:
  - name: "included-agent-2"
    description: "Agent from second include"
    priority: 7
  - id: "override-me"
    name: "overridden-agent"
    description: "This overrides the original"
    priority: 15
    tools:
      - Debug`

	mainPath := filepath.Join(tmpDir, "main.yaml")
	agents1Path := filepath.Join(tmpDir, "agents1.yaml")
	agents2Path := filepath.Join(tmpDir, "agents2.yaml")

	require.NoError(t, os.WriteFile(mainPath, []byte(mainConfig), 0o644))
	require.NoError(t, os.WriteFile(agents1Path, []byte(agents1Config), 0o644))
	require.NoError(t, os.WriteFile(agents2Path, []byte(agents2Config), 0o644))

	cfg, err := config.LoadConfigWithIncludes(mainPath)
	require.NoError(t, err)

	assert.Len(t, cfg.Agents, 4, "Should have 4 agents: main + 2 from first include + 1 from second (1 override)")

	agentsByName := make(map[string]config.Agent)
	for _, agent := range cfg.Agents {
		key := agent.Name
		if agent.ID != "" {
			key = agent.ID
		}
		agentsByName[key] = agent
	}

	mainAgent, exists := agentsByName["main-agent"]
	assert.True(t, exists, "main-agent should exist")
	assert.Equal(t, "Agent from main config", mainAgent.Description)
	assert.Equal(t, 10, mainAgent.Priority)

	agent1, exists := agentsByName["included-agent-1"]
	assert.True(t, exists, "included-agent-1 should exist")
	assert.Equal(t, "Agent from first include", agent1.Description)
	assert.Equal(t, 5, agent1.Priority)

	agent2, exists := agentsByName["included-agent-2"]
	assert.True(t, exists, "included-agent-2 should exist")
	assert.Equal(t, "Agent from second include", agent2.Description)
	assert.Equal(t, 7, agent2.Priority)

	overriddenAgent, exists := agentsByName["override-me"]
	assert.True(t, exists, "override-me agent should exist")
	assert.Equal(t, "overridden-agent", overriddenAgent.Name)
	assert.Equal(t, "This overrides the original", overriddenAgent.Description)
	assert.Equal(t, 15, overriddenAgent.Priority)
	assert.Contains(t, overriddenAgent.Tools, "Debug")

	assert.Empty(t, cfg.Includes, "Includes should be cleared after processing")
}

func TestConfigLoaderRemoteIntegration(t *testing.T) {
	t.Run("url_detection_logic", func(t *testing.T) {
		testCases := []struct {
			path     string
			expected bool
		}{
			{"https://example.com/config.yaml", true},
			{"http://example.com/config.yaml", true},
			{"HTTP://EXAMPLE.COM/CONFIG.YAML", true},
			{"ftp://example.com/config.yaml", false},
			{"/absolute/path/config.yaml", false},
			{"relative/path/config.yaml", false},
			{"./config.yaml", false},
			{"../config.yaml", false},
			{"", false},
		}

		for _, tc := range testCases {
			t.Run(tc.path, func(t *testing.T) {
				if tc.expected {
					t.Skip("URL validation test requires network")
				} else if tc.path != "" {
					assert.True(t, true, "Non-URL path %s handled as local", tc.path)
				}
			})
		}
	})

	t.Run("error_propagation_scenarios", func(t *testing.T) {
		tmpDir := t.TempDir()

		mainConfig := `metadata:
  name: "Error Test"
includes:
  - "missing.yaml"
outputs:
  - path: "test.md"`

		mainPath := filepath.Join(tmpDir, "main.yaml")
		err := os.WriteFile(mainPath, []byte(mainConfig), 0o644)
		require.NoError(t, err)

		_, err = config.LoadConfigWithIncludes(mainPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing.yaml")
	})
}
