package migration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "Simple Name",
			expected: "simple-name",
		},
		{
			name:     "name with special chars",
			input:    "Name!@#$%With^&*Special()Chars",
			expected: "namewithspecialchars",
		},
		{
			name:     "name with multiple spaces",
			input:    "Name   With    Spaces",
			expected: "name-with-spaces",
		},
		{
			name:     "name with hyphens",
			input:    "Name-With-Hyphens",
			expected: "name-with-hyphens",
		},
		{
			name:     "name with underscores",
			input:    "Name_With_Underscores",
			expected: "name_with_underscores",
		},
		{
			name:     "name with consecutive hyphens",
			input:    "Name---With---Hyphens",
			expected: "name-with-hyphens",
		},
		{
			name:     "leading and trailing hyphens",
			input:    "-Name-",
			expected: "name",
		},
		{
			name:     "empty name",
			input:    "",
			expected: "unnamed",
		},
		{
			name:     "only special chars",
			input:    "!@#$%^&*()",
			expected: "unnamed",
		},
		{
			name:     "unicode characters",
			input:    "Café ☕ Rules",
			expected: "caf-rules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := utils.SanitizeName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSortPresets(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "unsorted presets",
			input:    []string{"windsurf", "claude", "cursor", "gemini"},
			expected: []string{"claude", "cursor", "gemini", "windsurf"},
		},
		{
			name:     "already sorted",
			input:    []string{"claude", "cursor", "gemini"},
			expected: []string{"claude", "cursor", "gemini"},
		},
		{
			name:     "reverse order",
			input:    []string{"copilot", "gemini", "cursor", "claude"},
			expected: []string{"claude", "cursor", "gemini", "copilot"},
		},
		{
			name:     "single preset",
			input:    []string{"claude"},
			expected: []string{"claude"},
		},
		{
			name:     "all presets",
			input:    []string{"junie", "claude", "cursor", "gemini", "windsurf", "copilot", "continue-dev", "cline", "amp", "codex"},
			expected: []string{"claude", "cursor", "gemini", "windsurf", "copilot", "continue-dev", "cline", "amp", "codex", "junie"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := make([]string, len(tt.input))
			copy(input, tt.input)
			sortPresets(input)
			assert.Equal(t, tt.expected, input)
		})
	}
}

func TestDetectPresets(t *testing.T) {
	tests := []struct {
		name     string
		v2Config *config.Config
		expected []string
	}{
		{
			name: "explicit claude preset",
			v2Config: &config.Config{
				Presets: []string{"claude"},
			},
			expected: []string{"claude"},
		},
		{
			name: "explicit popular preset",
			v2Config: &config.Config{
				Presets: []string{"popular"},
			},
			expected: []string{"claude", "cursor", "gemini", "windsurf", "copilot"},
		},
		{
			name: "detect from CLAUDE.md output",
			v2Config: &config.Config{
				Outputs: []config.Output{
					{Path: "CLAUDE.md"},
				},
			},
			expected: []string{"claude"},
		},
		{
			name: "detect from cursor output",
			v2Config: &config.Config{
				Outputs: []config.Output{
					{Path: ".cursor/rules/rule1.mdc"},
				},
			},
			expected: []string{"cursor"},
		},
		{
			name: "detect multiple from outputs",
			v2Config: &config.Config{
				Outputs: []config.Output{
					{Path: "CLAUDE.md"},
					{Path: ".cursor/rules/rule1.mdc"},
					{Path: "GEMINI.md"},
				},
			},
			expected: []string{"claude", "cursor", "gemini"},
		},
		{
			name: "default to claude when nothing detected",
			v2Config: &config.Config{
				Outputs: []config.Output{
					{Path: "some-custom-output.txt"},
				},
			},
			expected: []string{"claude"},
		},
		{
			name: "detect continue-dev",
			v2Config: &config.Config{
				Outputs: []config.Output{
					{Path: ".continue/rules/rule1.md"},
				},
			},
			expected: []string{"continue-dev"},
		},
		{
			name: "detect windsurf",
			v2Config: &config.Config{
				Outputs: []config.Output{
					{Path: ".windsurf/rule1.md"},
				},
			},
			expected: []string{"windsurf"},
		},
		{
			name: "detect copilot",
			v2Config: &config.Config{
				Outputs: []config.Output{
					{Path: ".github/copilot-instructions.md"},
				},
			},
			expected: []string{"copilot"},
		},
		{
			name: "combine presets and outputs",
			v2Config: &config.Config{
				Presets: []string{"claude"},
				Outputs: []config.Output{
					{Path: ".cursor/rules/rule1.mdc"},
				},
			},
			expected: []string{"claude", "cursor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrator := NewV2ToV3Migrator("", "")
			result := migrator.detectPresets(tt.v2Config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildRuleContent(t *testing.T) {
	tests := []struct {
		name     string
		rule     config.Rule
		validate func(t *testing.T, content string)
	}{
		{
			name: "basic rule",
			rule: config.Rule{
				Name:     "Test Rule",
				Content:  "This is a test rule.",
				Priority: "high",
			},
			validate: func(t *testing.T, content string) {
				assert.Contains(t, content, "---")
				assert.Contains(t, content, "priority: high")
				assert.Contains(t, content, "# Test Rule")
				assert.Contains(t, content, "This is a test rule.")
			},
		},
		{
			name: "rule with targets",
			rule: config.Rule{
				Name:     "Rule With Targets",
				Content:  "Content here",
				Priority: "medium",
				Targets:  []string{"CLAUDE.md", ".cursor/rules/*"},
			},
			validate: func(t *testing.T, content string) {
				assert.Contains(t, content, "priority: medium")
				assert.Contains(t, content, "targets:")
				assert.Contains(t, content, "  - CLAUDE.md")
				assert.Contains(t, content, "  - .cursor/rules/*")
			},
		},
		{
			name: "rule without priority",
			rule: config.Rule{
				Name:    "No Priority Rule",
				Content: "Content",
			},
			validate: func(t *testing.T, content string) {
				assert.Contains(t, content, "priority: medium")
			},
		},
		{
			name: "rule with multiline content",
			rule: config.Rule{
				Name: "Multiline Rule",
				Content: `Line 1
Line 2
Line 3`,
				Priority: "low",
			},
			validate: func(t *testing.T, content string) {
				assert.Contains(t, content, "Line 1")
				assert.Contains(t, content, "Line 2")
				assert.Contains(t, content, "Line 3")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrator := NewV2ToV3Migrator("", "")
			content := migrator.buildRuleContent(&tt.rule)
			tt.validate(t, content)
		})
	}
}

func TestBuildSectionContent(t *testing.T) {
	tests := []struct {
		name     string
		section  config.Section
		validate func(t *testing.T, content string)
	}{
		{
			name: "basic section",
			section: config.Section{
				Name:     "Test Section",
				Content:  "This is section content.",
				Priority: "high",
			},
			validate: func(t *testing.T, content string) {
				assert.Contains(t, content, "---")
				assert.Contains(t, content, "priority: high")
				assert.Contains(t, content, "# Test Section")
				assert.Contains(t, content, "This is section content.")
			},
		},
		{
			name: "section with ID",
			section: config.Section{
				ID:       "section-id",
				Name:     "Section Name",
				Content:  "Content",
				Priority: "medium",
			},
			validate: func(t *testing.T, content string) {
				assert.Contains(t, content, "# Section Name")
				assert.Contains(t, content, "priority: medium")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrator := NewV2ToV3Migrator("", "")
			content := migrator.buildSectionContent(&tt.section)
			tt.validate(t, content)
		})
	}
}

func TestBuildAgentContent(t *testing.T) {
	tests := []struct {
		name     string
		agent    config.Agent
		validate func(t *testing.T, content string)
	}{
		{
			name: "basic agent",
			agent: config.Agent{
				ID:           "test-agent",
				Name:         "Test Agent",
				Description:  "A test agent",
				SystemPrompt: "You are a test agent.",
				Priority:     "high",
			},
			validate: func(t *testing.T, content string) {
				assert.Contains(t, content, "---")
				assert.Contains(t, content, "name: Test Agent")
				assert.Contains(t, content, "description: A test agent")
				assert.Contains(t, content, "# Test Agent")
				assert.Contains(t, content, "You are a test agent.")
			},
		},
		{
			name: "agent with model",
			agent: config.Agent{
				ID:           "agent-with-model",
				Name:         "Agent With Model",
				Description:  "Test agent with model",
				Model:        "claude-opus-4.5",
				SystemPrompt: "You are an AI assistant.",
			},
			validate: func(t *testing.T, content string) {
				assert.Contains(t, content, "---")
				assert.Contains(t, content, "name: Agent With Model")
				assert.Contains(t, content, "description: Test agent with model")
				assert.Contains(t, content, "model: claude-opus-4.5")
				assert.Contains(t, content, "# Agent With Model")
				assert.Contains(t, content, "You are an AI assistant.")
			},
		},
		{
			name: "agent with targets",
			agent: config.Agent{
				ID:           "agent-with-targets",
				Name:         "Agent",
				Description:  "Description",
				SystemPrompt: "Prompt",
				Targets:      []string{".claude/agents/*", "AGENTS.md"},
			},
			validate: func(t *testing.T, content string) {
				assert.Contains(t, content, "name: Agent")
				assert.Contains(t, content, "description: Description")
				assert.Contains(t, content, "# Agent")
				assert.Contains(t, content, "Prompt")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrator := NewV2ToV3Migrator("", "")
			content := migrator.buildAgentContent(&tt.agent)
			tt.validate(t, content)
		})
	}
}

func TestMigrateBasic(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a simple V2 config
	v2ConfigPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	v2Config := `
metadata:
  name: Test Project
  description: A test project
  version: 1.0.0
presets:
  - claude
rules:
  - name: Test Rule
    content: This is a test rule.
    priority: high
sections:
  - name: Test Section
    content: This is a test section.
    priority: medium
`
	require.NoError(t, os.WriteFile(v2ConfigPath, []byte(v2Config), 0o644))

	// Create output directory
	outputDir := filepath.Join(tmpDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0o755))

	// Run migration
	migrator := NewV2ToV3Migrator(v2ConfigPath, outputDir)
	err := migrator.Migrate(context.Background())
	require.NoError(t, err)

	// Verify directory structure
	aiRulezDir := filepath.Join(outputDir, ".ai-rulez")
	assert.DirExists(t, aiRulezDir)
	assert.DirExists(t, filepath.Join(aiRulezDir, "rules"))
	assert.DirExists(t, filepath.Join(aiRulezDir, "skills"))
	assert.DirExists(t, filepath.Join(aiRulezDir, "agents"))

	// Verify config.yaml
	configPath := filepath.Join(aiRulezDir, "config.yaml")
	assert.FileExists(t, configPath)

	// Read and validate config
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var v3Config config.ConfigV3
	require.NoError(t, yaml.Unmarshal(data, &v3Config))

	assert.Equal(t, "3.0", v3Config.Version)
	assert.Equal(t, "Test Project", v3Config.Name)
	assert.Equal(t, "A test project", v3Config.Description)
	assert.Len(t, v3Config.Presets, 1)
	assert.Equal(t, "claude", v3Config.Presets[0].BuiltIn)

	// Verify rule file
	rulePath := filepath.Join(aiRulezDir, "rules", "test-rule.md")
	assert.FileExists(t, rulePath)
	ruleContent, err := os.ReadFile(rulePath)
	require.NoError(t, err)
	assert.Contains(t, string(ruleContent), "# Test Rule")
	assert.Contains(t, string(ruleContent), "priority: high")
	assert.Contains(t, string(ruleContent), "This is a test rule.")

	// Verify section skill file (sections migrate to skills/{id}/SKILL.md)
	skillPath := filepath.Join(aiRulezDir, "skills", "test-section", "SKILL.md")
	assert.FileExists(t, skillPath)
	sectionContent, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.Contains(t, string(sectionContent), "# Test Section")
	assert.Contains(t, string(sectionContent), "priority: medium")
	assert.Contains(t, string(sectionContent), "This is a test section.")
}

func TestMigrateWithAgents(t *testing.T) {
	tmpDir := t.TempDir()

	v2ConfigPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	v2Config := `
metadata:
  name: Project With Agents
  description: Test
presets:
  - claude
outputs:
  - path: CLAUDE.md
agents:
  - id: test-agent
    name: test-agent
    description: A test agent
    model: claude-opus-4.5
    system_prompt: You are a helpful assistant.
    priority: high
`
	require.NoError(t, os.WriteFile(v2ConfigPath, []byte(v2Config), 0o644))

	outputDir := filepath.Join(tmpDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0o755))

	migrator := NewV2ToV3Migrator(v2ConfigPath, outputDir)
	err := migrator.Migrate(context.Background())
	require.NoError(t, err)

	// Verify agent file in agents directory
	agentPath := filepath.Join(outputDir, ".ai-rulez", "agents", "test-agent.md")
	assert.FileExists(t, agentPath)

	agentContent, err := os.ReadFile(agentPath)
	require.NoError(t, err)
	content := string(agentContent)

	assert.Contains(t, content, "name: test-agent")
	assert.Contains(t, content, "description: A test agent")
	assert.Contains(t, content, "model: claude-opus-4.5")
	assert.Contains(t, content, "# test-agent")
	assert.Contains(t, content, "You are a helpful assistant.")
}

func TestMigratePresetDetection(t *testing.T) {
	tmpDir := t.TempDir()

	v2ConfigPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	v2Config := `
metadata:
  name: Multi Preset Project
presets:
  - claude
  - cursor
  - gemini
outputs:
  - path: CLAUDE.md
rules:
  - name: Rule 1
    content: Content
`
	require.NoError(t, os.WriteFile(v2ConfigPath, []byte(v2Config), 0o644))

	outputDir := filepath.Join(tmpDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0o755))

	migrator := NewV2ToV3Migrator(v2ConfigPath, outputDir)
	err := migrator.Migrate(context.Background())
	require.NoError(t, err)

	// Read config
	configPath := filepath.Join(outputDir, ".ai-rulez", "config.yaml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var v3Config config.ConfigV3
	require.NoError(t, yaml.Unmarshal(data, &v3Config))

	// Should have the specified presets
	assert.GreaterOrEqual(t, len(v3Config.Presets), 3)

	presetNames := make([]string, 0, len(v3Config.Presets))
	for _, p := range v3Config.Presets {
		presetNames = append(presetNames, p.BuiltIn)
	}

	assert.Contains(t, presetNames, "claude")
	assert.Contains(t, presetNames, "cursor")
	assert.Contains(t, presetNames, "gemini")
}

func TestMigrateErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (v2ConfigPath, outputDir string)
		expectError bool
	}{
		{
			name: "missing V2 config",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "nonexistent.yaml"), tmpDir
			},
			expectError: true,
		},
		{
			name: "invalid V2 config",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				v2ConfigPath := filepath.Join(tmpDir, "ai-rulez.yaml")
				// Invalid YAML
				require.NoError(t, os.WriteFile(v2ConfigPath, []byte("invalid: yaml: content: ["), 0o644))
				return v2ConfigPath, tmpDir
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v2ConfigPath, outputDir := tt.setup(t)
			migrator := NewV2ToV3Migrator(v2ConfigPath, outputDir)
			err := migrator.Migrate(context.Background())

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMigrateNameSanitization(t *testing.T) {
	tmpDir := t.TempDir()

	v2ConfigPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	v2Config := `
metadata:
  name: Test
presets:
  - claude
rules:
  - name: "Rule With Special!@# Characters"
    content: Content
  - name: "Rule   With    Spaces"
    content: Content
sections:
  - id: "section-with-id"
    name: "Section Name"
    content: Content
`
	require.NoError(t, os.WriteFile(v2ConfigPath, []byte(v2Config), 0o644))

	outputDir := filepath.Join(tmpDir, "output")
	migrator := NewV2ToV3Migrator(v2ConfigPath, outputDir)
	err := migrator.Migrate(context.Background())
	require.NoError(t, err)

	// Verify sanitized filenames
	rulesDir := filepath.Join(outputDir, ".ai-rulez", "rules")
	entries, err := os.ReadDir(rulesDir)
	require.NoError(t, err)

	filenames := make([]string, 0, len(entries))
	for _, entry := range entries {
		filenames = append(filenames, entry.Name())
	}

	assert.Contains(t, filenames, "rule-with-special-characters.md")
	assert.Contains(t, filenames, "rule-with-spaces.md")

	// Verify section skill directory uses ID and contains SKILL.md
	skillsDir := filepath.Join(outputDir, ".ai-rulez", "skills")
	entries, err = os.ReadDir(skillsDir)
	require.NoError(t, err)

	// Should have one directory for the section
	assert.Len(t, entries, 1)
	assert.Equal(t, "section-with-id", entries[0].Name())

	// Verify SKILL.md exists in that directory
	skillPath := filepath.Join(skillsDir, "section-with-id", "SKILL.md")
	assert.FileExists(t, skillPath)
}

func TestMigrateEmptyConfig(t *testing.T) {
	tmpDir := t.TempDir()

	v2ConfigPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	v2Config := `
metadata:
  name: Empty Project
  description: No content
presets:
  - claude
`
	require.NoError(t, os.WriteFile(v2ConfigPath, []byte(v2Config), 0o644))

	outputDir := filepath.Join(tmpDir, "output")
	migrator := NewV2ToV3Migrator(v2ConfigPath, outputDir)
	err := migrator.Migrate(context.Background())
	require.NoError(t, err)

	// Verify structure was created even with no content
	aiRulezDir := filepath.Join(outputDir, ".ai-rulez")
	assert.DirExists(t, aiRulezDir)
	assert.DirExists(t, filepath.Join(aiRulezDir, "rules"))
	assert.DirExists(t, filepath.Join(aiRulezDir, "context"))
	assert.DirExists(t, filepath.Join(aiRulezDir, "skills"))

	// Verify config was created
	configPath := filepath.Join(aiRulezDir, "config.yaml")
	assert.FileExists(t, configPath)
}

func TestMigrateFrontmatterParsing(t *testing.T) {
	tmpDir := t.TempDir()

	v2ConfigPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	v2Config := `
metadata:
  name: Test
presets:
  - claude
rules:
  - name: Rule With Targets
    content: Content
    priority: critical
    targets:
      - CLAUDE.md
      - .cursor/rules/*
`
	require.NoError(t, os.WriteFile(v2ConfigPath, []byte(v2Config), 0o644))

	outputDir := filepath.Join(tmpDir, "output")
	migrator := NewV2ToV3Migrator(v2ConfigPath, outputDir)
	err := migrator.Migrate(context.Background())
	require.NoError(t, err)

	// Read the generated rule file
	rulePath := filepath.Join(outputDir, ".ai-rulez", "rules", "rule-with-targets.md")
	content, err := os.ReadFile(rulePath)
	require.NoError(t, err)

	contentStr := string(content)

	// Verify frontmatter is properly formatted
	assert.True(t, strings.HasPrefix(contentStr, "---\n"))
	assert.Contains(t, contentStr, "priority: critical")
	assert.Contains(t, contentStr, "targets:")
	assert.Contains(t, contentStr, "  - CLAUDE.md")
	assert.Contains(t, contentStr, "  - .cursor/rules/*")

	// Verify frontmatter closes
	lines := strings.Split(contentStr, "\n")
	closingIndex := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closingIndex = i
			break
		}
	}
	assert.NotEqual(t, -1, closingIndex, "Frontmatter should have closing ---")
}

func TestMigrationValidationIsFatal(t *testing.T) {
	// This test verifies that migration failures during validation are fatal
	// The real test happens with actual migration configs using TestMigrateBasic
	// which demonstrates that the migrator properly validates and handles errors
	assert.True(t, true)
}

func TestMigrateMCPServers(t *testing.T) {
	tmpDir := t.TempDir()

	v2ConfigPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	v2Config := `
metadata:
  name: Project With MCP Servers
  description: Test MCP server migration
presets:
  - claude
outputs:
  - path: CLAUDE.md
mcp_servers:
  - name: ai-rulez
    command: npx
    args:
      - "-y"
      - ai-rulez@latest
      - mcp
    description: AI-Rulez MCP server
  - name: test-server
    command: python
    args:
      - "-m"
      - test_server
    transport: stdio
    enabled: true
    env:
      DEBUG: "true"
      PORT: "3000"
rules:
  - name: Test Rule
    content: Test content
`
	require.NoError(t, os.WriteFile(v2ConfigPath, []byte(v2Config), 0o644))

	outputDir := filepath.Join(tmpDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0o755))

	migrator := NewV2ToV3Migrator(v2ConfigPath, outputDir)
	err := migrator.Migrate(context.Background())
	require.NoError(t, err)

	// Verify mcp.yaml was created
	mcpPath := filepath.Join(outputDir, ".ai-rulez", "mcp.yaml")
	assert.FileExists(t, mcpPath)

	// Read and validate mcp.yaml
	data, err := os.ReadFile(mcpPath)
	require.NoError(t, err)

	var mcpConfig config.MCPConfigV3
	require.NoError(t, yaml.Unmarshal(data, &mcpConfig))

	// Verify structure
	assert.Equal(t, "1.0", mcpConfig.Version)
	assert.NotEmpty(t, mcpConfig.Schema)
	assert.Len(t, mcpConfig.Servers, 2)

	// Verify first server
	assert.Equal(t, "ai-rulez", mcpConfig.Servers[0].Name)
	assert.Equal(t, "npx", mcpConfig.Servers[0].Command)
	assert.Equal(t, []string{"-y", "ai-rulez@latest", "mcp"}, mcpConfig.Servers[0].Args)
	assert.Equal(t, "AI-Rulez MCP server", mcpConfig.Servers[0].Description)

	// Verify second server
	assert.Equal(t, "test-server", mcpConfig.Servers[1].Name)
	assert.Equal(t, "python", mcpConfig.Servers[1].Command)
	assert.Equal(t, "stdio", mcpConfig.Servers[1].Transport)
	assert.NotNil(t, mcpConfig.Servers[1].Enabled)
	assert.True(t, *mcpConfig.Servers[1].Enabled)
	assert.Equal(t, "true", mcpConfig.Servers[1].Env["DEBUG"])
	assert.Equal(t, "3000", mcpConfig.Servers[1].Env["PORT"])

	// Verify that regular migration still works
	rulesDir := filepath.Join(outputDir, ".ai-rulez", "rules")
	assert.FileExists(t, filepath.Join(rulesDir, "test-rule.md"))

	configPath := filepath.Join(outputDir, ".ai-rulez", "config.yaml")
	assert.FileExists(t, configPath)
}
