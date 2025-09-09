package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDetectV1Schema(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]interface{}
		expected bool
	}{
		{
			name: "v1_string_template_in_output",
			config: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"path":     ".cursorrules",
						"template": "default",
					},
				},
			},
			expected: true,
		},
		{
			name: "v1_file_reference_template",
			config: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"path":     ".cursorrules",
						"template": "@templates/custom.tmpl",
					},
				},
			},
			expected: true,
		},
		{
			name: "v1_inline_template",
			config: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"path":     ".cursorrules",
						"template": "{{.Rules}}",
					},
				},
			},
			expected: true,
		},
		{
			name: "v2_object_template",
			config: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"path": ".cursorrules",
						"template": map[string]interface{}{
							"type":  "builtin",
							"value": "default",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "v1_template_in_agents",
			config: map[string]interface{}{
				"agents": []interface{}{
					map[string]interface{}{
						"name":     "test-agent",
						"template": "agent",
					},
				},
			},
			expected: true,
		},
		{
			name: "v2_template_in_agents",
			config: map[string]interface{}{
				"agents": []interface{}{
					map[string]interface{}{
						"name": "test-agent",
						"template": map[string]interface{}{
							"type":  "builtin",
							"value": "agent",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "no_template",
			config: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"path": ".cursorrules",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := detectV1Schema(tt.config)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCheckUnsupportedFeatures(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]interface{}
		expectErr bool
		errMsg    string
	}{
		{
			name: "config_with_includes",
			config: map[string]interface{}{
				"includes": []interface{}{
					"other-config.yaml",
				},
			},
			expectErr: true,
			errMsg:    "cannot automatically migrate configuration with 'includes'",
		},
		{
			name: "config_without_includes",
			config: map[string]interface{}{
				"outputs": []interface{}{},
			},
			expectErr: false,
		},
		{
			name: "config_with_empty_includes",
			config: map[string]interface{}{
				"includes": []interface{}{},
			},
			expectErr: false,
		},
		{
			name: "config_with_extends",
			config: map[string]interface{}{
				"extends": "base-config.yaml",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkUnsupportedFeatures(tt.config)
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMigrateTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "string_template_to_object",
			input: map[string]interface{}{
				"path":     ".cursorrules",
				"template": "default",
			},
			expected: map[string]interface{}{
				"path": ".cursorrules",
				"template": map[string]interface{}{
					"type":  "builtin",
					"value": "default",
				},
			},
		},
		{
			name: "file_reference_to_object",
			input: map[string]interface{}{
				"path":     ".cursorrules",
				"template": "@templates/custom.tmpl",
			},
			expected: map[string]interface{}{
				"path": ".cursorrules",
				"template": map[string]interface{}{
					"type":  "file",
					"value": "templates/custom.tmpl",
				},
			},
		},
		{
			name: "already_object_unchanged",
			input: map[string]interface{}{
				"path": ".cursorrules",
				"template": map[string]interface{}{
					"type":  "builtin",
					"value": "default",
				},
			},
			expected: map[string]interface{}{
				"path": ".cursorrules",
				"template": map[string]interface{}{
					"type":  "builtin",
					"value": "default",
				},
			},
		},
		{
			name: "no_template_unchanged",
			input: map[string]interface{}{
				"path": ".cursorrules",
			},
			expected: map[string]interface{}{
				"path": ".cursorrules",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := migrateTemplate(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, tt.input)
		})
	}
}

func TestMigrateConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "migrate_outputs_and_agents",
			input: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"path":     ".cursorrules",
						"template": "default",
					},
					map[string]interface{}{
						"path":     "docs.md",
						"template": "@templates/doc.tmpl",
					},
				},
				"agents": []interface{}{
					map[string]interface{}{
						"name":     "test-agent",
						"template": "agent",
					},
				},
			},
			expected: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"path": ".cursorrules",
						"template": map[string]interface{}{
							"type":  "builtin",
							"value": "default",
						},
					},
					map[string]interface{}{
						"path": "docs.md",
						"template": map[string]interface{}{
							"type":  "file",
							"value": "templates/doc.tmpl",
						},
					},
				},
				"agents": []interface{}{
					map[string]interface{}{
						"id":          "test-agent",
						"name":        "test-agent",
						"description": "Agent: test-agent",
						"template": map[string]interface{}{
							"type":  "builtin",
							"value": "agent",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := migrateConfig(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, tt.input)
		})
	}
}

func TestValidateV2Schema(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]interface{}
		expectErr bool
	}{
		{
			name: "valid_v2_config",
			config: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"path": ".cursorrules",
						"template": map[string]interface{}{
							"type":  "builtin",
							"value": "default",
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_string_template",
			config: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"path":     ".cursorrules",
						"template": "default",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "invalid_template_missing_type",
			config: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"path": ".cursorrules",
						"template": map[string]interface{}{
							"value": "default",
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "invalid_template_type",
			config: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"path": ".cursorrules",
						"template": map[string]interface{}{
							"type":  "unknown",
							"value": "default",
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateV2Schema(tt.config)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIsBuiltinTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"builtin_default", "default", true},
		{"builtin_documentation", "documentation", true},
		{"builtin_cursor", "cursor", true},
		{"short_unknown", "mycustom", true},
		{"inline_with_spaces", "This is inline", false},
		{"inline_with_braces", "{{.Rules}}", false},
		{"inline_multiline", "Line1\nLine2", false},
		{"very_long_name", strings.Repeat("a", 50), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBuiltinTemplate(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMigrateV1ToV2_Integration(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("successful_migration", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "test-v1.yaml")
		v1Config := `metadata:
  name: TestProject
outputs:
  - path: .cursorrules
    template: default
  - path: docs.md
    template: "@templates/doc.tmpl"
agents:
  - name: test-agent
    template: agent
    system_prompt: "You are a test agent"
`
		err := os.WriteFile(configPath, []byte(v1Config), 0o644)
		require.NoError(t, err)

		err = MigrateV1ToV2(configPath)
		require.NoError(t, err)

		backupPath := filepath.Join(tmpDir, "test-v1.v1-backup.yaml")
		assert.NoFileExists(t, backupPath, "Backup should be cleaned up after successful migration")

		data, err := os.ReadFile(configPath)
		require.NoError(t, err)

		var config map[string]interface{}
		err = yaml.Unmarshal(data, &config)
		require.NoError(t, err)

		outputs := config["outputs"].([]interface{})
		output1 := outputs[0].(map[string]interface{})
		template1 := output1["template"].(map[string]interface{})
		assert.Equal(t, "builtin", template1["type"])
		assert.Equal(t, "default", template1["value"])

		output2 := outputs[1].(map[string]interface{})
		template2 := output2["template"].(map[string]interface{})
		assert.Equal(t, "file", template2["type"])
		assert.Equal(t, "templates/doc.tmpl", template2["value"])

		agents := config["agents"].([]interface{})
		agent := agents[0].(map[string]interface{})
		agentTemplate := agent["template"].(map[string]interface{})
		assert.Equal(t, "builtin", agentTemplate["type"])
		assert.Equal(t, "agent", agentTemplate["value"])
	})

	t.Run("migration_with_includes_fails", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "test-v1-includes.yaml")
		v1ConfigWithIncludes := `metadata:
  name: TestProject
includes:
  - other-config.yaml
outputs:
  - path: .cursorrules
    template: default
`
		err := os.WriteFile(configPath, []byte(v1ConfigWithIncludes), 0o644)
		require.NoError(t, err)

		err = MigrateV1ToV2(configPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot automatically migrate configuration with 'includes'")

		backupPath := filepath.Join(tmpDir, "test-v1-includes.v1-backup.yaml")
		assert.NoFileExists(t, backupPath)
	})

	t.Run("already_v2_no_migration", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "test-v2.yaml")
		v2Config := `metadata:
  name: TestProject
outputs:
  - path: .cursorrules
    template:
      type: builtin
      value: default
`
		err := os.WriteFile(configPath, []byte(v2Config), 0o644)
		require.NoError(t, err)

		originalData, _ := os.ReadFile(configPath)

		err = MigrateV1ToV2(configPath)
		require.NoError(t, err)

		newData, _ := os.ReadFile(configPath)
		assert.Equal(t, originalData, newData)

		backupPath := filepath.Join(tmpDir, "test-v2.v1-backup.yaml")
		assert.NoFileExists(t, backupPath)
	})
}

func TestDetectAndMigrate(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "v1_schema_migrates",
			content: `$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json
metadata:
  name: TestProject
outputs:
  - path: .cursorrules
    template: default`,
			expected: "v1",
		},
		{
			name: "v2_schema_unchanged",
			content: `metadata:
  name: TestProject
outputs:
  - path: .cursorrules
    template:
      type: builtin
      value: default`,
			expected: "v2",
		},
		{
			name: "no_templates_v2",
			content: `metadata:
  name: TestProject
outputs:
  - path: .cursorrules`,
			expected: "v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(tmpDir, tt.name+".yaml")
			err := os.WriteFile(configPath, []byte(tt.content), 0o644)
			require.NoError(t, err)

			version, err := DetectAndMigrate(configPath)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, version)

			if tt.expected == "v1" {
				data, err := os.ReadFile(configPath)
				require.NoError(t, err)
				assert.Contains(t, string(data), "ai-rules-v2.schema.json")
			}
		})
	}
}

func TestBackupConfig(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `metadata:
  name: TestProject`

	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err)

	backupPath, err := BackupConfig(configPath)
	require.NoError(t, err)

	assert.Contains(t, backupPath, "test-config.v1-backup.yaml")

	backupData, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, configContent, string(backupData))
}

func TestMigrateIfNeeded(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("migrates_v1_config", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "v1-config.yaml")
		v1Config := `metadata:
  name: TestProject
outputs:
  - path: .cursorrules
    template: default`

		err := os.WriteFile(configPath, []byte(v1Config), 0o644)
		require.NoError(t, err)

		err = MigrateIfNeeded(configPath)
		require.NoError(t, err)

		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Contains(t, string(data), "type: builtin")
		assert.Contains(t, string(data), "value: default")

		backupPath := filepath.Join(tmpDir, "v1-config.v1-backup.yaml")
		assert.NoFileExists(t, backupPath)
	})

	t.Run("no_migration_for_v2_config", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "v2-config.yaml")
		v2Config := `metadata:
  name: TestProject
outputs:
  - path: .cursorrules
    template:
      type: builtin
      value: default`

		err := os.WriteFile(configPath, []byte(v2Config), 0o644)
		require.NoError(t, err)

		originalData, _ := os.ReadFile(configPath)

		err = MigrateIfNeeded(configPath)
		require.NoError(t, err)

		newData, _ := os.ReadFile(configPath)
		assert.Equal(t, originalData, newData)

		backupPath := filepath.Join(tmpDir, "v2-config.v1-backup.yaml")
		assert.NoFileExists(t, backupPath)
	})
}

func TestExtractNamedTargets(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]interface{}
		expected map[string][]string
	}{
		{
			name: "basic_named_targets",
			config: map[string]interface{}{
				"targets": map[string]interface{}{
					"docs": []interface{}{"*.md", "docs/**/*.md"},
					"code": []interface{}{"*.go", "*.py"},
				},
			},
			expected: map[string][]string{
				"docs": {"*.md", "docs/**/*.md"},
				"code": {"*.go", "*.py"},
			},
		},
		{
			name:     "no_targets_section",
			config:   map[string]interface{}{},
			expected: map[string][]string{},
		},
		{
			name: "empty_targets",
			config: map[string]interface{}{
				"targets": map[string]interface{}{},
			},
			expected: map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractNamedTargets(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveTargetsInObject(t *testing.T) {
	namedTargets := map[string][]string{
		"docs":   {"*.md", "docs/**/*.md"},
		"code":   {"*.go", "*.py"},
		"claude": {"CLAUDE.md", ".claude/**/*.md"},
	}

	tests := []struct {
		name     string
		obj      map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "resolve_single_named_target",
			obj: map[string]interface{}{
				"targets": []interface{}{"@docs"},
			},
			expected: map[string]interface{}{
				"targets": []interface{}{"*.md", "docs/**/*.md"},
			},
		},
		{
			name: "resolve_multiple_named_targets",
			obj: map[string]interface{}{
				"targets": []interface{}{"@docs", "@code"},
			},
			expected: map[string]interface{}{
				"targets": []interface{}{"*.md", "docs/**/*.md", "*.go", "*.py"},
			},
		},
		{
			name: "mixed_named_and_direct_targets",
			obj: map[string]interface{}{
				"targets": []interface{}{"@docs", "*.js", "@code"},
			},
			expected: map[string]interface{}{
				"targets": []interface{}{"*.md", "docs/**/*.md", "*.js", "*.go", "*.py"},
			},
		},
		{
			name:     "no_targets_field",
			obj:      map[string]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "unresolved_named_target",
			obj: map[string]interface{}{
				"targets": []interface{}{"@nonexistent"},
			},
			expected: map[string]interface{}{
				"targets": []interface{}{"@nonexistent"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolveTargetsInObject(tt.obj, namedTargets)
			assert.Equal(t, tt.expected, tt.obj)
		})
	}
}

func TestMigrateConfigWithRootTargets(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	v1Config := `$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json

targets:
  claude-only:
    - "CLAUDE.md"
    - ".claude/**/*.md"
  claude-agents:
    - ".claude/agents/*.md"

outputs:
  - file: "CLAUDE.md"
  - file: "GEMINI.md"

agents:
  - name: "test-agent"
    targets: ["@claude-agents"]
    description: "Test agent"

rules:
  - name: "Test Rule"
    targets: ["@claude-only"]
    priority: 100
    content: "Test content"
`

	err := os.WriteFile(configPath, []byte(v1Config), 0o644)
	require.NoError(t, err)

	err = MigrateV1ToV2(configPath)
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var migratedConfig map[string]interface{}
	err = yaml.Unmarshal(data, &migratedConfig)
	require.NoError(t, err)

	assert.Equal(t, "https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json", migratedConfig["$schema"])

	assert.NotContains(t, migratedConfig, "targets")

	outputs := migratedConfig["outputs"].([]interface{})
	output1 := outputs[0].(map[string]interface{})
	assert.Equal(t, "CLAUDE.md", output1["path"])
	assert.NotContains(t, output1, "file")

	agents := migratedConfig["agents"].([]interface{})
	agent1 := agents[0].(map[string]interface{})
	agentTargets := agent1["targets"].([]interface{})
	assert.Equal(t, []interface{}{".claude/agents/*.md"}, agentTargets)

	rules := migratedConfig["rules"].([]interface{})
	rule1 := rules[0].(map[string]interface{})
	ruleTargets := rule1["targets"].([]interface{})
	assert.Equal(t, []interface{}{"CLAUDE.md", ".claude/**/*.md"}, ruleTargets)

	backupPath := getBackupPath(configPath)
	assert.NoFileExists(t, backupPath)
}

func TestBackupCleanupOnSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	v1Config := `$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json

outputs:
  - file: "test.md"
`

	err := os.WriteFile(configPath, []byte(v1Config), 0o644)
	require.NoError(t, err)

	err = MigrateV1ToV2(configPath)
	require.NoError(t, err)

	backupPath := getBackupPath(configPath)
	assert.NoFileExists(t, backupPath, "Backup file should be cleaned up after successful migration")
}

func TestBackupCleanupOnValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	v1Config := `$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json

agents:
  - name: "test-agent"
    description: "Test agent"
    template: 123  # This will cause validation failure as numbers can't be migrated
`

	err := os.WriteFile(configPath, []byte(v1Config), 0o644)
	require.NoError(t, err)

	err = MigrateV1ToV2(configPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "migration validation failed, restored from backup")

	backupPath := getBackupPath(configPath)
	assert.NoFileExists(t, backupPath, "Backup file should be cleaned up after restore")

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "ai-rules-v1.schema.json")
}

func TestPriorityMigration(t *testing.T) {
	t.Run("relative_priority_bucketing", func(t *testing.T) {
		tests := []struct {
			name     string
			input    []int
			expected map[int]string
		}{
			{
				name:     "single_priority_defaults_to_medium",
				input:    []int{50},
				expected: map[int]string{50: "medium"},
			},
			{
				name:     "two_priorities_minimal_medium",
				input:    []int{1, 100},
				expected: map[int]string{1: "minimal", 100: "medium"},
			},
			{
				name:  "five_priorities_all_buckets",
				input: []int{1, 25, 50, 75, 100},
				expected: map[int]string{
					1:   "minimal",
					25:  "low",
					50:  "medium",
					75:  "high",
					100: "critical",
				},
			},
			{
				name:  "duplicate_priorities_handled",
				input: []int{5, 5, 10, 10, 20},
				expected: map[int]string{
					5:  "minimal",
					10: "low",
					20: "high",
				},
			},
			{
				name:  "large_distribution",
				input: []int{1, 2, 3, 4, 5, 10, 15, 20, 25, 30},
				expected: map[int]string{
					1:  "minimal",
					2:  "minimal",
					3:  "low",
					4:  "low",
					5:  "medium",
					10: "medium",
					15: "high",
					20: "high",
					25: "critical",
					30: "critical",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := createRelativePriorityMapping(tt.input)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("collect_all_priorities", func(t *testing.T) {
		config := map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{"priority": 100},
				map[string]interface{}{"priority": 50},
				map[string]interface{}{},
			},
			"agents": []interface{}{
				map[string]interface{}{"priority": 80},
				map[string]interface{}{"priority": float64(30)},
			},
			"sections": []interface{}{
				map[string]interface{}{"priority": "10"},
				map[string]interface{}{"priority": 5},
			},
		}

		priorities := collectAllPriorities(config)
		expected := []int{100, 50, 80, 30, 10, 5}
		assert.ElementsMatch(t, expected, priorities)
	})

	t.Run("end_to_end_priority_migration", func(t *testing.T) {
		config := map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"name":     "High Priority Rule",
					"priority": 100,
					"content":  "High priority content",
				},
				map[string]interface{}{
					"name":     "Low Priority Rule",
					"priority": 5,
					"content":  "Low priority content",
				},
			},
			"agents": []interface{}{
				map[string]interface{}{
					"name":        "medium-agent",
					"description": "Medium agent",
					"priority":    50,
				},
			},
		}

		migratePrioritiesRelative(config)

		rules := config["rules"].([]interface{})
		rule1 := rules[0].(map[string]interface{})
		rule2 := rules[1].(map[string]interface{})

		assert.Equal(t, "high", rule1["priority"])
		assert.Equal(t, "minimal", rule2["priority"])

		agents := config["agents"].([]interface{})
		agent1 := agents[0].(map[string]interface{})
		assert.Equal(t, "low", agent1["priority"])
	})
}

func TestGenerateHumanReadableID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple_lowercase",
			input:    "test-agent",
			expected: "test-agent",
		},
		{
			name:     "mixed_case_conversion",
			input:    "TestAgent",
			expected: "testagent",
		},
		{
			name:     "spaces_to_hyphens",
			input:    "Go Code Agent",
			expected: "go-code-agent",
		},
		{
			name:     "special_characters",
			input:    "Test & Debug Agent!",
			expected: "test-debug-agent",
		},
		{
			name:     "multiple_spaces_consecutive_special",
			input:    "Test   !!!   Agent   $$$",
			expected: "test-agent",
		},
		{
			name:     "leading_trailing_special",
			input:    "   !!!Test Agent!!!   ",
			expected: "test-agent",
		},
		{
			name:     "unicode_characters",
			input:    "Tëst Ägënt 🤖",
			expected: "t-st-g-nt",
		},
		{
			name:     "numbers_preserved",
			input:    "Agent V2.1",
			expected: "agent-v2-1",
		},
		{
			name:     "very_long_name_truncated",
			input:    "This is a very long agent name that exceeds the maximum length limit and should be truncated appropriately",
			expected: "this-is-a-very-long-agent-name-that-exceeds-the-ma",
		},
		{
			name:     "empty_string",
			input:    "",
			expected: "",
		},
		{
			name:     "only_special_characters",
			input:    "!@#$%^&*()",
			expected: "",
		},
		{
			name:     "hyphen_preservation",
			input:    "pre-existing-hyphens",
			expected: "pre-existing-hyphens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateHumanReadableID(tt.input)
			assert.Equal(t, tt.expected, result)

			assert.NotContains(t, result, "--")
			if result != "" {
				assert.NotEqual(t, "-", string(result[0]))
				assert.NotEqual(t, "-", string(result[len(result)-1]))
			}
			assert.LessOrEqual(t, len(result), 50)
		})
	}
}

func TestSectionMigration(t *testing.T) {
	t.Run("title_to_name_conversion", func(t *testing.T) {
		section := map[string]interface{}{
			"title":   "Project Overview",
			"content": "Overview content",
		}

		err := migrateSection(section)
		require.NoError(t, err)

		assert.Equal(t, "Project Overview", section["name"])
		assert.NotContains(t, section, "title")
	})

	t.Run("id_generation_from_name", func(t *testing.T) {
		section := map[string]interface{}{
			"title":   "Complex Section Name!",
			"content": "Section content",
		}

		err := migrateSection(section)
		require.NoError(t, err)

		assert.Equal(t, "Complex Section Name!", section["name"])
		assert.Equal(t, "complex-section-name", section["id"])
	})

	t.Run("existing_id_preserved", func(t *testing.T) {
		section := map[string]interface{}{
			"id":      "custom-section-id",
			"title":   "Section Title",
			"content": "Section content",
		}

		err := migrateSection(section)
		require.NoError(t, err)

		assert.Equal(t, "custom-section-id", section["id"])
		assert.Equal(t, "Section Title", section["name"])
	})

	t.Run("no_title_no_id_generation", func(t *testing.T) {
		section := map[string]interface{}{
			"content": "Section content",
		}

		err := migrateSection(section)
		require.NoError(t, err)

		assert.NotContains(t, section, "id")
		assert.NotContains(t, section, "name")
	})
}

func TestUserRulezErrorHandling(t *testing.T) {
	t.Run("user_rulez_causes_error", func(t *testing.T) {
		config := map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "Test Project",
			},
			"outputs": []interface{}{
				map[string]interface{}{"path": "test.md"},
			},
			"user_rulez": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"name":    "User Rule",
						"content": "User specific rule",
					},
				},
			},
		}

		err := migrateConfig(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user_rulez is no longer supported in v2")
		assert.Contains(t, err.Error(), ".local configuration files")
	})

	t.Run("empty_user_rulez_causes_error", func(t *testing.T) {
		config := map[string]interface{}{
			"user_rulez": map[string]interface{}{},
		}

		err := migrateConfig(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user_rulez is no longer supported")
	})

	t.Run("no_user_rulez_no_error", func(t *testing.T) {
		config := map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "Test Project",
			},
			"outputs": []interface{}{
				map[string]interface{}{"path": "test.md"},
			},
		}

		err := migrateConfig(config)
		assert.NoError(t, err)
	})
}

func TestMigrateSectionTitleReferences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple title reference",
			input:    "{{.Title}}",
			expected: "{{.Name}}",
		},
		{
			name:     "title with spaces",
			input:    "{{ .Title }}",
			expected: "{{.Name}}",
		},
		{
			name:     "title in range context",
			input:    "{{range .Sections}}\n## {{.Title}}\n{{.Content}}\n{{end}}",
			expected: "{{range .Sections}}\n## {{.Name}}\n{{.Content}}\n{{end}}",
		},
		{
			name:     "multiple title references",
			input:    "{{.Title}} - {{.Title}}",
			expected: "{{.Name}} - {{.Name}}",
		},
		{
			name:     "no title references",
			input:    "{{.Name}} {{.Content}}",
			expected: "{{.Name}} {{.Content}}",
		},
		{
			name:     "mixed content",
			input:    "# Document\n{{range .Sections}}\n## {{.Title}}\n{{.Content}}\n{{end}}",
			expected: "# Document\n{{range .Sections}}\n## {{.Name}}\n{{.Content}}\n{{end}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := migrateSectionTitleReferences(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMigrateTemplateWithTitleReferences(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "string template with title reference",
			input: map[string]interface{}{
				"template": "{{range .Sections}}{{.Title}}{{end}}",
			},
			expected: map[string]interface{}{
				"template": map[string]interface{}{
					"type":  "inline",
					"value": "{{range .Sections}}{{.Name}}{{end}}",
				},
			},
		},
		{
			name: "already migrated template with title",
			input: map[string]interface{}{
				"template": map[string]interface{}{
					"type":  "inline",
					"value": "{{.Title}}",
				},
			},
			expected: map[string]interface{}{
				"template": map[string]interface{}{
					"type":  "inline",
					"value": "{{.Name}}",
				},
			},
		},
		{
			name: "builtin template unchanged",
			input: map[string]interface{}{
				"template": "default",
			},
			expected: map[string]interface{}{
				"template": map[string]interface{}{
					"type":  "builtin",
					"value": "default",
				},
			},
		},
		{
			name: "file template unchanged",
			input: map[string]interface{}{
				"template": "@templates/custom.tmpl",
			},
			expected: map[string]interface{}{
				"template": map[string]interface{}{
					"type":  "file",
					"value": "templates/custom.tmpl",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := migrateTemplate(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, tt.input)
		})
	}
}

func TestFullMigrationWithTitleReferences(t *testing.T) {
	// Create a temporary v1 config with .Title references
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "ai-rulez.yaml")
	
	v1Config := `$schema: "https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json"
metadata:
  name: TestProject
  version: 1.0.0

sections:
  - title: "Section One"
    content: "Content one"
    priority: 10
  - title: "Section Two"
    content: "Content two"
    priority: 5

outputs:
  - file: "output.md"
    template: |
      # Document
      {{range .Sections}}
      ## {{.Title}}
      {{.Content}}
      {{end}}`

	err := os.WriteFile(configPath, []byte(v1Config), 0644)
	require.NoError(t, err)

	// Perform migration
	version, err := DetectAndMigrate(configPath)
	require.NoError(t, err)
	assert.Equal(t, "v1", version)

	// Read migrated content
	migratedContent, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Check that migration was successful
	migratedStr := string(migratedContent)
	
	// Check schema was updated
	assert.Contains(t, migratedStr, "ai-rules-v2.schema.json")
	
	// Check sections have name instead of title
	assert.Contains(t, migratedStr, "name: Section One")
	assert.Contains(t, migratedStr, "name: Section Two")
	assert.NotContains(t, migratedStr, "title:")
	
	// Check outputs have path instead of file
	assert.Contains(t, migratedStr, "path: output.md")
	assert.NotContains(t, migratedStr, "file:")
	
	// Check template was converted to object format
	assert.Contains(t, migratedStr, "type: inline")
	
	// Check .Title was replaced with .Name in template
	assert.Contains(t, migratedStr, "{{.Name}}")
	assert.NotContains(t, migratedStr, "{{.Title}}")
	
	// Check priorities were converted
	assert.Contains(t, migratedStr, "priority: medium")
	assert.Contains(t, migratedStr, "priority: minimal")
}

func TestComprehensiveMigration(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "comprehensive-v1.yaml")

	v1Config := `$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json
metadata:
  name: Comprehensive Migration Test
  version: 2.1.0
  description: Testing all v1 to v2 migration features
outputs:
  - file: CLAUDE.md
    template: default
  - path: agents/
    type: agent
    template: "@templates/agent.tmpl"
  - path: .cursorrules
    template: "{{.Rules}}"
rules:
  - name: Most Important Rule
    priority: 100
    content: This rule has the highest priority
  - name: Standard Rule
    priority: 50
    content: This is a standard priority rule
  - name: Basic Rule
    priority: 1
    content: This is the lowest priority rule
agents:
  - name: code-reviewer
    description: Expert code reviewer
    priority: 80
    tools: ["Read", "Grep", "Glob"]
    template: "specialized"
    system_prompt: You are an expert code reviewer
  - name: helper-agent
    description: General purpose helper
    priority: 20
    system_prompt: You help with general tasks
sections:
  - title: Project Architecture
    priority: 90
    content: |
      # Architecture Overview
      This section describes the project architecture.
  - title: Development Guidelines  
    priority: 30
    content: Guidelines for development
  - title: Testing Strategy
    priority: 60
    content: How we approach testing
`

	err := os.WriteFile(configPath, []byte(v1Config), 0o644)
	require.NoError(t, err)

	err = MigrateV1ToV2(configPath)
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var config map[string]interface{}
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err)

	assert.Equal(t, "https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json", config["$schema"])

	metadata := config["metadata"].(map[string]interface{})
	assert.Equal(t, "Comprehensive Migration Test", metadata["name"])
	assert.Equal(t, "2.1.0", metadata["version"])
	assert.Equal(t, "Testing all v1 to v2 migration features", metadata["description"])

	outputs := config["outputs"].([]interface{})
	assert.Len(t, outputs, 3)

	output1 := outputs[0].(map[string]interface{})
	assert.Equal(t, "CLAUDE.md", output1["path"])
	assert.NotContains(t, output1, "file")
	template1 := output1["template"].(map[string]interface{})
	assert.Equal(t, "builtin", template1["type"])
	assert.Equal(t, "default", template1["value"])

	output2 := outputs[1].(map[string]interface{})
	template2 := output2["template"].(map[string]interface{})
	assert.Equal(t, "file", template2["type"])
	assert.Equal(t, "templates/agent.tmpl", template2["value"])

	output3 := outputs[2].(map[string]interface{})
	template3 := output3["template"].(map[string]interface{})
	assert.Equal(t, "inline", template3["type"])
	assert.Equal(t, "{{.Rules}}", template3["value"])

	rules := config["rules"].([]interface{})
	assert.Len(t, rules, 3)

	rule1 := rules[0].(map[string]interface{})
	assert.Equal(t, "Most Important Rule", rule1["name"])
	assert.Equal(t, "critical", rule1["priority"])

	rule2 := rules[1].(map[string]interface{})
	assert.Equal(t, "Standard Rule", rule2["name"])
	assert.Equal(t, "low", rule2["priority"])

	rule3 := rules[2].(map[string]interface{})
	assert.Equal(t, "Basic Rule", rule3["name"])
	assert.Equal(t, "minimal", rule3["priority"])

	agents := config["agents"].([]interface{})
	assert.Len(t, agents, 2)

	agent1 := agents[0].(map[string]interface{})
	assert.Equal(t, "code-reviewer", agent1["name"])
	assert.Equal(t, "code-reviewer", agent1["id"])
	assert.Equal(t, "high", agent1["priority"])
	agentTemplate1 := agent1["template"].(map[string]interface{})
	assert.Equal(t, "builtin", agentTemplate1["type"])
	assert.Equal(t, "specialized", agentTemplate1["value"])
	assert.Equal(t, []interface{}{"Read", "Grep", "Glob"}, agent1["tools"])

	agent2 := agents[1].(map[string]interface{})
	assert.Equal(t, "helper-agent", agent2["name"])
	assert.Equal(t, "helper-agent", agent2["id"])
	assert.Equal(t, "minimal", agent2["priority"])
	assert.NotContains(t, agent2, "template")

	sections := config["sections"].([]interface{})
	assert.Len(t, sections, 3)

	section1 := sections[0].(map[string]interface{})
	assert.Equal(t, "Project Architecture", section1["name"])
	assert.Equal(t, "project-architecture", section1["id"])
	assert.Equal(t, "high", section1["priority"])
	assert.NotContains(t, section1, "title")
	assert.Contains(t, section1["content"], "# Architecture Overview")

	section2 := sections[1].(map[string]interface{})
	assert.Equal(t, "Development Guidelines", section2["name"])
	assert.Equal(t, "development-guidelines", section2["id"])
	assert.Equal(t, "low", section2["priority"])

	section3 := sections[2].(map[string]interface{})
	assert.Equal(t, "Testing Strategy", section3["name"])
	assert.Equal(t, "testing-strategy", section3["id"])
	assert.Equal(t, "medium", section3["priority"])

	assert.NotContains(t, config, "targets")

	backupPath := filepath.Join(tmpDir, "comprehensive-v1.v1-backup.yaml")
	assert.NoFileExists(t, backupPath)
}
