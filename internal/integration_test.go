// +build integration

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
	// Load the test configuration
	configPath := filepath.Join("..", "testing", "scenarios", "agents", "ai_rulez.yaml")
	cfg, err := config.LoadConfigWithIncludes(configPath)
	require.NoError(t, err)

	// Generate outputs in temp directory
	tmpDir := t.TempDir()
	gen := generator.NewWithBaseDir(tmpDir)
	err = gen.GenerateAll(cfg)
	require.NoError(t, err)

	// Verify agent files were created
	agentDir := filepath.Join(tmpDir, ".claude", "agents")
	entries, err := os.ReadDir(agentDir)
	require.NoError(t, err)
	assert.Len(t, entries, 4, "Should have 4 agent files")

	// Expected agent files
	expectedAgents := map[string]struct {
		hasTools        bool
		hasSystemPrompt bool
		hasTemplate     bool
	}{
		"code-reviewer.md": {
			hasTools:        true,
			hasSystemPrompt: true,
		},
		"test-writer.md": {
			hasTools:        true,
			hasSystemPrompt: true,
		},
		"doc-generator.md": {
			hasTemplate: true,
		},
		"security-auditor.md": {
			hasTools: true,
		},
	}

	for filename, expected := range expectedAgents {
		t.Run(filename, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(agentDir, filename))
			require.NoError(t, err)

			// Parse YAML frontmatter
			parts := strings.Split(string(content), "---")
			require.GreaterOrEqual(t, len(parts), 3, "Should have frontmatter")

			var frontmatter map[string]interface{}
			err = yaml.Unmarshal([]byte(parts[1]), &frontmatter)
			require.NoError(t, err)

			// Verify frontmatter structure
			assert.Contains(t, frontmatter, "name")
			assert.Contains(t, frontmatter, "description")

			if expected.hasTools {
				assert.Contains(t, frontmatter, "tools")
				tools, ok := frontmatter["tools"].([]interface{})
				assert.True(t, ok)
				assert.NotEmpty(t, tools)
			}

			// Check body content
			body := strings.TrimSpace(parts[2])
			if expected.hasSystemPrompt || expected.hasTemplate {
				assert.NotEmpty(t, body)
			}
		})
	}

	// Verify CLAUDE.md was also created
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	_, err = os.Stat(claudePath)
	assert.NoError(t, err)
}

func TestAgentGeneration_WithIncludes(t *testing.T) {
	// Create temp directory for test files
	tmpDir := t.TempDir()

	// Create main config
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

	// Create included config
	includedConfig := `agents:
  - name: "included-agent"
    description: "Agent from included config"
    priority: 5
  - name: "main-agent"
    description: "Override from included config"
    priority: 15
    tools:
      - "Read"`

	// Write configs
	mainPath := filepath.Join(tmpDir, "main.yaml")
	includedPath := filepath.Join(tmpDir, "included.yaml")
	
	err := os.WriteFile(mainPath, []byte(mainConfig), 0644)
	require.NoError(t, err)
	err = os.WriteFile(includedPath, []byte(includedConfig), 0644)
	require.NoError(t, err)

	// Load config with includes
	cfg, err := config.LoadConfigWithIncludes(mainPath)
	require.NoError(t, err)

	// Verify agents were merged correctly
	assert.Len(t, cfg.Agents, 2)
	
	// Find main-agent (should be overridden)
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

	// Generate and verify
	gen := generator.NewWithBaseDir(tmpDir)
	err = gen.GenerateAll(cfg)
	require.NoError(t, err)

	// Check generated files
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

	// Verify files with custom naming
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

	// Check that files are sanitized
	agentFiles, err := os.ReadDir(filepath.Join(tmpDir, "agents"))
	require.NoError(t, err)
	assert.Len(t, agentFiles, 4)

	// Verify no path traversal occurred
	_, err = os.Stat(filepath.Join(tmpDir, "..", "..", "etc", "passwd.md"))
	assert.Error(t, err)

	// Check sanitized names exist
	expectedSanitized := []string{
		"valid-agent.md",
		"etc-passwd.md",
		"agent-with-slashes.md", 
		"agent-with-colons.md",
	}

	for _, filename := range expectedSanitized {
		path := filepath.Join(tmpDir, "agents", filename)
		_, err := os.Stat(path)
		assert.NoError(t, err, "Sanitized file %s should exist", filename)
	}
}