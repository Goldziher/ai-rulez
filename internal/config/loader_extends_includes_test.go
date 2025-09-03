package config

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== Extends + Includes Combined Tests ==========

func TestLoadConfigWithExtendsAndIncludes(t *testing.T) {
	t.Run("local extends with local includes", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create base config
		baseConfigFile := filepath.Join(tempDir, "base.yaml")
		baseContent := `
metadata:
  name: Base Project
  description: Base description
outputs:
  - path: base-output.md
rules:
  - name: Base Rule
    content: Base rule content
    priority: medium
agents:
  - name: base-agent
    description: Base agent
    system_prompt: Base prompt
    priority: low
`
		require.NoError(t, os.WriteFile(baseConfigFile, []byte(baseContent), 0o644))

		// Create include file
		includeFile := filepath.Join(tempDir, "include.yaml")
		includeContent := `
rules:
  - name: Include Rule
    content: Include rule content
    priority: high
agents:
  - name: include-agent
    description: Include agent
    system_prompt: Include prompt
    priority: medium
`
		require.NoError(t, os.WriteFile(includeFile, []byte(includeContent), 0o644))

		// Create child config with both extends and includes
		childConfigFile := filepath.Join(tempDir, "child.yaml")
		childContent := `
extends: "./base.yaml"
includes:
  - "./include.yaml"
metadata:
  name: Child Project
  version: 2.0.0
outputs:
  - path: child-output.md
rules:
  - name: Child Rule
    content: Child rule content
    priority: critical
  - name: Base Rule  # Override base rule
    content: Overridden base rule
    priority: critical
`
		require.NoError(t, os.WriteFile(childConfigFile, []byte(childContent), 0o644))

		// Load configuration
		cfg, err := LoadConfigWithIncludes(context.Background(), childConfigFile)
		require.NoError(t, err)
		assert.NotNil(t, cfg)

		// Check metadata inheritance and override
		assert.Equal(t, "Child Project", cfg.Metadata.Name)
		assert.Equal(t, "2.0.0", cfg.Metadata.Version)
		assert.Equal(t, "Base description", cfg.Metadata.Description) // Inherited from base

		// Check outputs are combined (base + child)
		assert.Len(t, cfg.Outputs, 2)
		outputPaths := []string{cfg.Outputs[0].Path, cfg.Outputs[1].Path}
		assert.Contains(t, outputPaths, "base-output.md")
		assert.Contains(t, outputPaths, "child-output.md")

		// Check rules are combined from all sources (base + include + child)
		assert.Len(t, cfg.Rules, 3)
		ruleNames := make(map[string]string)
		for _, rule := range cfg.Rules {
			ruleNames[rule.Name] = rule.Content
		}
		assert.Equal(t, "Overridden base rule", ruleNames["Base Rule"])    // Child overrides base
		assert.Equal(t, "Include rule content", ruleNames["Include Rule"]) // From include
		assert.Equal(t, "Child rule content", ruleNames["Child Rule"])     // From child

		// Check agents are combined from all sources (base + include)
		assert.Len(t, cfg.Agents, 2)
		agentNames := make([]string, len(cfg.Agents))
		for i, agent := range cfg.Agents {
			agentNames[i] = agent.Name
		}
		assert.Contains(t, agentNames, "base-agent")
		assert.Contains(t, agentNames, "include-agent")
	})

	t.Run("remote extends with local includes", func(t *testing.T) {
		// Create remote base config
		baseConfigContent := `
metadata:
  name: Remote Base
  description: Remote base description
outputs:
  - path: remote-output.md
rules:
  - name: Remote Rule
    content: Remote rule content
    priority: medium
agents:
  - name: remote-agent
    description: Remote agent
    system_prompt: Remote prompt
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/yaml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(baseConfigContent))
		}))
		defer server.Close()

		tempDir := t.TempDir()

		// Create local include file
		includeFile := filepath.Join(tempDir, "local-include.yaml")
		includeContent := `
rules:
  - name: Local Include Rule
    content: Local include rule content
    priority: high
sections:
  - name: Local Section
    content: Local section content
    priority: medium
`
		require.NoError(t, os.WriteFile(includeFile, []byte(includeContent), 0o644))

		// Create child config with remote extends and local includes
		childConfigFile := filepath.Join(tempDir, "child.yaml")
		childContent := fmt.Sprintf(`
extends: %s
includes:
  - "./local-include.yaml"
metadata:
  name: Mixed Sources Project
  version: 3.0.0
rules:
  - name: Child Rule
    content: Child rule content
    priority: critical
`, server.URL)
		require.NoError(t, os.WriteFile(childConfigFile, []byte(childContent), 0o644))

		// Create test loader
		loader := &configLoader{
			visited:      make(map[string]bool),
			baseDir:      tempDir,
			remoteClient: createTestClient(),
		}

		cfg, err := loader.loadConfig(context.Background(), childConfigFile)
		require.NoError(t, err)

		// Should have rules from remote base + local include + child
		assert.Len(t, cfg.Rules, 3)
		ruleNames := make([]string, len(cfg.Rules))
		for i, rule := range cfg.Rules {
			ruleNames[i] = rule.Name
		}
		assert.Contains(t, ruleNames, "Remote Rule")
		assert.Contains(t, ruleNames, "Local Include Rule")
		assert.Contains(t, ruleNames, "Child Rule")

		// Should have sections from local include
		assert.Len(t, cfg.Sections, 1)
		assert.Equal(t, "Local Section", cfg.Sections[0].Name)

		// Should have agents from remote base
		assert.Len(t, cfg.Agents, 1)
		assert.Equal(t, "remote-agent", cfg.Agents[0].Name)
	})

	t.Run("extends with includes that have extends (nested)", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create grandparent config
		grandparentFile := filepath.Join(tempDir, "grandparent.yaml")
		grandparentContent := `
metadata:
  name: Grandparent
  description: Grandparent description
rules:
  - name: Grandparent Rule
    content: Grandparent rule content
    priority: low
`
		require.NoError(t, os.WriteFile(grandparentFile, []byte(grandparentContent), 0o644))

		// Create parent config that extends grandparent
		parentFile := filepath.Join(tempDir, "parent.yaml")
		parentContent := `
extends: "./grandparent.yaml"
metadata:
  name: Parent
outputs:
  - path: parent-output.md
rules:
  - name: Parent Rule
    content: Parent rule content
    priority: medium
`
		require.NoError(t, os.WriteFile(parentFile, []byte(parentContent), 0o644))

		// Create include file that also extends
		includeFile := filepath.Join(tempDir, "include.yaml")
		includeContent := `
extends: "./grandparent.yaml"
rules:
  - name: Include Rule
    content: Include rule content
    priority: high
  - name: Grandparent Rule  # Override grandparent rule in include
    content: Include overrides grandparent
    priority: high
`
		require.NoError(t, os.WriteFile(includeFile, []byte(includeContent), 0o644))

		// Create child config that extends parent and includes the include
		childFile := filepath.Join(tempDir, "child.yaml")
		childContent := `
extends: "./parent.yaml"
includes:
  - "./include.yaml"
metadata:
  name: Child
  version: 1.0.0
rules:
  - name: Child Rule
    content: Child rule content
    priority: critical
`
		require.NoError(t, os.WriteFile(childFile, []byte(childContent), 0o644))

		cfg, err := LoadConfigWithIncludes(context.Background(), childFile)
		require.NoError(t, err)

		// Check complex inheritance and merging
		assert.Equal(t, "Child", cfg.Metadata.Name)
		assert.Equal(t, "1.0.0", cfg.Metadata.Version)
		assert.Equal(t, "Grandparent description", cfg.Metadata.Description) // From grandparent via parent

		// Should have rules from all levels
		assert.Len(t, cfg.Rules, 4)
		ruleNames := make(map[string]string)
		for _, rule := range cfg.Rules {
			ruleNames[rule.Name] = rule.Content
		}
		assert.Contains(t, ruleNames, "Grandparent Rule")
		assert.Contains(t, ruleNames, "Parent Rule")
		assert.Contains(t, ruleNames, "Include Rule")
		assert.Contains(t, ruleNames, "Child Rule")

		// The include should override grandparent rule (include processing happens after extends)
		assert.Equal(t, "Include overrides grandparent", ruleNames["Grandparent Rule"])
	})
}
