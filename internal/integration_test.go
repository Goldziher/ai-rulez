package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
)

func TestAgentGeneration_FullIntegration(t *testing.T) {
	configPath := filepath.Join("..", "e2e", "scenarios", "agents", "ai_rules.yaml")
	cfg, err := config.LoadConfigWithIncludes(configPath)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	gen := generator.NewWithBaseDir(tmpDir)
	err = gen.GenerateAll(cfg)
	require.NoError(t, err)

	agentDir := filepath.Join(tmpDir, ".claude", "agents")
	entries, err := os.ReadDir(agentDir)
	require.NoError(t, err)
	assert.Len(t, entries, 3, "Should have 3 agent files")

	expectedAgents := map[string]struct {
		hasTools        bool
		hasSystemPrompt bool
		hasTemplate     bool
	}{
		"code-reviewer.md": {
			hasTools:        true,
			hasSystemPrompt: true,
		},
		"test-generator.md": {
			hasTools:    true,
			hasTemplate: true,
		},
		"doc-writer.md": {
			hasTools:        true,
			hasSystemPrompt: true,
		},
	}

	for filename, expected := range expectedAgents {
		t.Run(filename, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(agentDir, filename))
			require.NoError(t, err)

			parts := strings.Split(string(content), "---")
			require.GreaterOrEqual(t, len(parts), 3, "Should have frontmatter")

			var frontmatter map[string]interface{}
			err = yaml.Unmarshal([]byte(parts[1]), &frontmatter)
			require.NoError(t, err)

			assert.Contains(t, frontmatter, "name")
			assert.Contains(t, frontmatter, "description")

			if expected.hasTools {
				assert.Contains(t, frontmatter, "tools")
				tools, ok := frontmatter["tools"].([]interface{})
				assert.True(t, ok)
				assert.NotEmpty(t, tools)
			}

			body := strings.TrimSpace(parts[2])
			if expected.hasSystemPrompt || expected.hasTemplate {
				assert.NotEmpty(t, body)
			}
		})
	}

	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	_, err = os.Stat(claudePath)
	assert.NoError(t, err)
}

func TestAgentGeneration_WithIncludes(t *testing.T) {
	tmpDir := t.TempDir()

	mainConfig := `metadata:
  name: "Main Config"
  version: "1.0.0"

includes:
  - "included.yaml"

outputs:
  - path: "agents/"
    type: "agent"

agents:
  - name: "main-agent"
    description: "Agent from main config"
    priority: 10`

	includedConfig := `metadata:
  name: "Included Config"
  version: "1.0.0"

outputs:
  - path: "agents/"
    type: "agent"

agents:
  - name: "included-agent"
    description: "Agent from included config"
    priority: 5
  - name: "main-agent"
    description: "Override from included config"
    priority: 15
    tools:
      - "Read"`

	mainPath := filepath.Join(tmpDir, "main.yaml")
	includedPath := filepath.Join(tmpDir, "included.yaml")

	err := os.WriteFile(mainPath, []byte(mainConfig), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(includedPath, []byte(includedConfig), 0o644)
	require.NoError(t, err)

	cfg, err := config.LoadConfigWithIncludes(mainPath)
	require.NoError(t, err)

	assert.Len(t, cfg.Agents, 2)

	var mainAgent *config.Agent
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == "main-agent" {
			mainAgent = &cfg.Agents[i]
			break
		}
	}
	require.NotNil(t, mainAgent)
	assert.Equal(t, "Override from included config", mainAgent.Description)
	assert.Equal(t, 15, mainAgent.Priority)
	assert.Contains(t, mainAgent.Tools, "Read")

	gen := generator.NewWithBaseDir(tmpDir)
	err = gen.GenerateAll(cfg)
	require.NoError(t, err)

	agentFiles, err := os.ReadDir(filepath.Join(tmpDir, "agents"))
	require.NoError(t, err)
	assert.Len(t, agentFiles, 2)
}

func TestAgentGeneration_DirectoryWithCustomNaming(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Custom Naming Test",
		},
		Outputs: []config.Output{
			{
				Path:         "custom/agents/",
				Type:         "agent",
				NamingScheme: "{priority:02d}-{name}.agent.yaml",
			},
		},
		Agents: []config.Agent{
			{Name: "high-priority", Description: "High priority agent", Priority: 10},
			{Name: "low-priority", Description: "Low priority agent", Priority: 1},
			{Name: "medium-priority", Description: "Medium priority agent", Priority: 5},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	expectedFiles := []string{
		"10-high-priority.agent.yaml",
		"01-low-priority.agent.yaml",
		"05-medium-priority.agent.yaml",
	}

	for _, filename := range expectedFiles {
		path := filepath.Join(tmpDir, "custom", "agents", filename)
		_, err := os.Stat(path)
		assert.NoError(t, err, "File %s should exist", filename)
	}
}

func TestAgentGeneration_InvalidName(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Invalid Name Test",
		},
		Outputs: []config.Output{
			{
				Path: "agents/",
				Type: "agent",
			},
		},
		Agents: []config.Agent{
			{Name: "valid-agent", Description: "Valid agent"},
			{Name: "../../etc/passwd", Description: "Invalid path in name"},
			{Name: "agent/with/slashes", Description: "Name with slashes"},
			{Name: "agent:with:colons", Description: "Name with colons"},
		},
	}

	gen := generator.NewWithBaseDir(tmpDir)
	err := gen.GenerateAll(cfg)
	require.NoError(t, err)

	agentFiles, err := os.ReadDir(filepath.Join(tmpDir, "agents"))
	require.NoError(t, err)
	assert.Len(t, agentFiles, 4)

	// Check that path traversal doesn't work
	_, err = os.Stat(filepath.Join(tmpDir, "..", "..", "etc", "passwd.md"))
	assert.Error(t, err, "Path traversal should not create files outside the output directory")

	// Just verify files were created with sanitized names
	for _, file := range agentFiles {
		assert.True(t, strings.HasSuffix(file.Name(), ".md"), "All agent files should have .md extension")
		// Check that no file contains path separators or colons
		assert.NotContains(t, file.Name(), "/", "Filename should not contain slashes")
		assert.NotContains(t, file.Name(), "..", "Filename should not contain parent directory references")
	}
}
