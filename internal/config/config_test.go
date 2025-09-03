package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// ========== Core Config Loading Tests ==========

func TestLoadConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "test.yaml")

		configContent := `
metadata:
  name: Test Project
  version: 1.0.0
outputs:
  - path: output.md
rules:
  - name: Test Rule
    content: Test content
    priority: medium
`
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "Test Project", cfg.Metadata.Name)
		assert.Len(t, cfg.Rules, 1)
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := config.LoadConfig("/nonexistent/file.yaml")
		assert.Error(t, err)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "invalid.yaml")

		require.NoError(t, os.WriteFile(configFile, []byte("invalid: yaml: content:"), 0o644))

		_, err := config.LoadConfig(configFile)
		assert.Error(t, err)
	})
}

func TestSaveConfig(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "save_test.yaml")

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:    "Save Test",
			Version: "1.0.0",
		},
		Outputs: []config.Output{
			{Path: "test.md"},
		},
	}

	err := config.SaveConfig(cfg, configFile)
	require.NoError(t, err)

	loaded, err := config.LoadConfig(configFile)
	require.NoError(t, err)
	assert.Equal(t, cfg.Metadata.Name, loaded.Metadata.Name)
}

// ========== Config with Includes Tests ==========

func TestLoadConfigWithIncludes(t *testing.T) {
	t.Run("simple include", func(t *testing.T) {
		tempDir := t.TempDir()

		includeFile := filepath.Join(tempDir, "include.yaml")
		includeContent := `
rules:
  - name: Included Rule
    content: From include
    priority: low
`
		require.NoError(t, os.WriteFile(includeFile, []byte(includeContent), 0o644))

		mainFile := filepath.Join(tempDir, "main.yaml")
		mainContent := `
metadata:
  name: Main Config
outputs:
  - path: output.md
includes:
  - include.yaml
rules:
  - name: Main Rule
    content: From main
    priority: medium
`
		require.NoError(t, os.WriteFile(mainFile, []byte(mainContent), 0o644))

		cfg, err := config.LoadConfigWithIncludes(context.Background(), mainFile)
		require.NoError(t, err)
		assert.Len(t, cfg.Rules, 2)

		ruleNames := make([]string, len(cfg.Rules))
		for i, r := range cfg.Rules {
			ruleNames[i] = r.Name
		}
		assert.Contains(t, ruleNames, "Main Rule")
		assert.Contains(t, ruleNames, "Included Rule")
	})

	t.Run("circular include detection", func(t *testing.T) {
		tempDir := t.TempDir()

		fileA := filepath.Join(tempDir, "a.yaml")
		contentA := `
metadata:
  name: File A
outputs:
  - path: output.md
includes:
  - b.yaml
`
		require.NoError(t, os.WriteFile(fileA, []byte(contentA), 0o644))

		fileB := filepath.Join(tempDir, "b.yaml")
		contentB := `
includes:
  - a.yaml
`
		require.NoError(t, os.WriteFile(fileB, []byte(contentB), 0o644))

		_, err := config.LoadConfigWithIncludes(context.Background(), fileA)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circular")
	})

	t.Run("missing include file", func(t *testing.T) {
		tempDir := t.TempDir()

		mainFile := filepath.Join(tempDir, "main.yaml")
		mainContent := `
metadata:
  name: Main Config
outputs:
  - path: output.md
includes:
  - nonexistent.yaml
`
		require.NoError(t, os.WriteFile(mainFile, []byte(mainContent), 0o644))

		_, err := config.LoadConfigWithIncludes(context.Background(), mainFile)
		assert.Error(t, err)
	})
}

// ========== Config Finder Tests ==========

func TestFindConfigFile(t *testing.T) {
	t.Run("finds config in current directory", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "ai_rulez.yaml")

		require.NoError(t, os.WriteFile(configFile, []byte("test"), 0o644))

		found, err := config.FindConfigFile(tempDir)
		require.NoError(t, err)
		assert.Equal(t, configFile, found)
	})

	t.Run("finds config in parent directory", func(t *testing.T) {
		tempDir := t.TempDir()
		subDir := filepath.Join(tempDir, "subdir")
		require.NoError(t, os.MkdirAll(subDir, 0o755))

		configFile := filepath.Join(tempDir, ".ai-rulez.yaml")
		require.NoError(t, os.WriteFile(configFile, []byte("test"), 0o644))

		found, err := config.FindConfigFile(subDir)
		require.NoError(t, err)
		assert.Equal(t, configFile, found)
	})

	t.Run("no config found", func(t *testing.T) {
		tempDir := t.TempDir()
		_, err := config.FindConfigFile(tempDir)
		assert.Error(t, err)
	})
}

// ========== CRUD Operations Tests ==========

func TestAddRule(t *testing.T) {
	cfg := &config.Config{
		Rules: []config.Rule{
			{Name: "Existing", Content: "Existing content"},
		},
	}

	newRule := config.Rule{
		Name:     "New Rule",
		Content:  "New content",
		Priority: config.PriorityMedium,
	}

	cfg.Rules = append(cfg.Rules, newRule)
	assert.Len(t, cfg.Rules, 2)
	assert.Equal(t, "New Rule", cfg.Rules[1].Name)
}

func TestAddSection(t *testing.T) {
	cfg := &config.Config{
		Sections: []config.Section{},
	}

	newSection := config.Section{
		Name:     "New Section",
		Content:  "Section content",
		Priority: config.PriorityLow,
	}

	cfg.Sections = append(cfg.Sections, newSection)
	assert.Len(t, cfg.Sections, 1)
	assert.Equal(t, "New Section", cfg.Sections[0].Name)
}

func TestAddAgent(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.Agent{},
	}

	newAgent := config.Agent{
		Name:         "test-agent",
		Description:  "Test agent",
		Priority:     config.PriorityMedium,
		Tools:        []string{"read", "write"},
		SystemPrompt: "You are a test agent",
	}

	cfg.Agents = append(cfg.Agents, newAgent)
	assert.Len(t, cfg.Agents, 1)
	assert.Equal(t, "test-agent", cfg.Agents[0].Name)
}

// ========== Merge Functions Tests ==========

func TestMergeRules(t *testing.T) {
	rules1 := []config.Rule{
		{Name: "Rule1", Content: "Content1", Priority: config.PriorityMinimal},
		{Name: "Rule2", Content: "Content2", Priority: config.PriorityLow},
	}

	rules2 := []config.Rule{
		{Name: "Rule2", Content: "Updated2", Priority: config.PriorityLow},
		{Name: "Rule3", Content: "Content3", Priority: config.PriorityMedium},
	}

	merged := config.MergeRules(rules1, rules2)
	assert.Len(t, merged, 3)

	for _, r := range merged {
		if r.Name == "Rule2" {
			assert.Equal(t, "Updated2", r.Content)
			assert.Equal(t, config.PriorityLow, r.Priority)
		}
	}
}

func TestMergeSections(t *testing.T) {
	sections1 := []config.Section{
		{Name: "Section1", Content: "Content1"},
	}

	sections2 := []config.Section{
		{Name: "Section2", Content: "Content2"},
	}

	merged := config.MergeSections(sections1, sections2)
	assert.Len(t, merged, 2)
}

// ========== Validation Tests ==========

func TestValidateOutputs(t *testing.T) {
	t.Run("valid outputs", func(t *testing.T) {
		outputs := []config.Output{
			{Path: "output.md"},
		}
		err := config.ValidateOutputs(outputs)
		assert.NoError(t, err)
	})

	t.Run("empty outputs", func(t *testing.T) {
		var outputs []config.Output
		err := config.ValidateOutputs(outputs)
		assert.Error(t, err)
	})

	t.Run("output without path", func(t *testing.T) {
		outputs := []config.Output{
			{Template: "default"},
		}
		err := config.ValidateOutputs(outputs)
		assert.Error(t, err)
	})
}

func TestValidateIncludes(t *testing.T) {
	t.Run("valid local include", func(t *testing.T) {
		tempDir := t.TempDir()

		includeFile := filepath.Join(tempDir, "include.yaml")
		require.NoError(t, os.WriteFile(includeFile, []byte("rules: []"), 0o644))

		cfg := &config.Config{
			Includes: []string{"include.yaml"},
		}

		err := config.ValidateIncludes(cfg, tempDir)
		assert.NoError(t, err)
	})

	t.Run("missing include", func(t *testing.T) {
		tempDir := t.TempDir()

		cfg := &config.Config{
			Includes: []string{"missing.yaml"},
		}

		err := config.ValidateIncludes(cfg, tempDir)
		assert.Error(t, err)
	})
}

// ========== Extends Functionality Tests ==========

func TestLoadConfigWithExtends(t *testing.T) {
	t.Run("local extends", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Create base config
		baseConfigFile := filepath.Join(tempDir, "base.yaml")
		baseContent := `
metadata:
  name: Base Project
  version: 1.0.0
  description: Base description
outputs:
  - path: base-output.md
rules:
  - name: Base Rule
    content: Base rule content
    priority: medium
sections:
  - name: Base Section
    content: Base section content
    priority: low
agents:
  - name: base-agent
    description: Base agent
    system_prompt: Base prompt
    priority: medium
`
		require.NoError(t, os.WriteFile(baseConfigFile, []byte(baseContent), 0o644))
		
		// Create child config that extends base
		childConfigFile := filepath.Join(tempDir, "child.yaml")
		childContent := `
extends: base.yaml
metadata:
  name: Child Project
  version: 2.0.0
outputs:
  - path: child-output.md
rules:
  - name: Child Rule
    content: Child rule content
    priority: high
  - name: Base Rule  # Override base rule
    content: Overridden base rule
    priority: critical
`
		require.NoError(t, os.WriteFile(childConfigFile, []byte(childContent), 0o644))
		
		cfg, err := config.LoadConfigWithIncludes(context.Background(), childConfigFile)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		
		// Check metadata inheritance and overrides
		assert.Equal(t, "Child Project", cfg.Metadata.Name)     // Overridden
		assert.Equal(t, "2.0.0", cfg.Metadata.Version)         // Overridden
		assert.Equal(t, "Base description", cfg.Metadata.Description) // Inherited
		
		// Check outputs are combined
		assert.Len(t, cfg.Outputs, 2)
		outputPaths := []string{cfg.Outputs[0].Path, cfg.Outputs[1].Path}
		assert.Contains(t, outputPaths, "base-output.md")
		assert.Contains(t, outputPaths, "child-output.md")
		
		// Check rules are merged (child overrides base)
		assert.Len(t, cfg.Rules, 2)
		ruleNames := make(map[string]string)
		for _, rule := range cfg.Rules {
			ruleNames[rule.Name] = rule.Content
		}
		assert.Equal(t, "Overridden base rule", ruleNames["Base Rule"])
		assert.Equal(t, "Child rule content", ruleNames["Child Rule"])
		
		// Check sections are inherited
		assert.Len(t, cfg.Sections, 1)
		assert.Equal(t, "Base Section", cfg.Sections[0].Name)
		
		// Check agents are inherited
		assert.Len(t, cfg.Agents, 1)
		assert.Equal(t, "base-agent", cfg.Agents[0].Name)
	})
	
	t.Run("extends with includes", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Create included config
		includeConfigFile := filepath.Join(tempDir, "include.yaml")
		includeContent := `
rules:
  - name: Include Rule
    content: Include rule content
    priority: low
`
		require.NoError(t, os.WriteFile(includeConfigFile, []byte(includeContent), 0o644))
		
		// Create base config
		baseConfigFile := filepath.Join(tempDir, "base.yaml")
		baseContent := `
metadata:
  name: Base Project
outputs:
  - path: base-output.md
rules:
  - name: Base Rule
    content: Base rule content
    priority: medium
`
		require.NoError(t, os.WriteFile(baseConfigFile, []byte(baseContent), 0o644))
		
		// Create child config that extends base and includes additional content
		childConfigFile := filepath.Join(tempDir, "child.yaml")
		childContent := `
extends: base.yaml
includes:
  - include.yaml
metadata:
  name: Child Project
rules:
  - name: Child Rule
    content: Child rule content
    priority: high
`
		require.NoError(t, os.WriteFile(childConfigFile, []byte(childContent), 0o644))
		
		cfg, err := config.LoadConfigWithIncludes(context.Background(), childConfigFile)
		require.NoError(t, err)
		
		// Should have rules from base + includes + child
		assert.Len(t, cfg.Rules, 3)
		ruleNames := make([]string, len(cfg.Rules))
		for i, rule := range cfg.Rules {
			ruleNames[i] = rule.Name
		}
		assert.Contains(t, ruleNames, "Base Rule")
		assert.Contains(t, ruleNames, "Include Rule")
		assert.Contains(t, ruleNames, "Child Rule")
	})
	
	t.Run("extends not found", func(t *testing.T) {
		tempDir := t.TempDir()
		
		childConfigFile := filepath.Join(tempDir, "child.yaml")
		childContent := `
extends: nonexistent.yaml
metadata:
  name: Child Project
outputs:
  - path: output.md
`
		require.NoError(t, os.WriteFile(childConfigFile, []byte(childContent), 0o644))
		
		_, err := config.LoadConfigWithIncludes(context.Background(), childConfigFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "loading extended config")
	})
	
	t.Run("circular extends", func(t *testing.T) {
		tempDir := t.TempDir()
		
		// Create config A that extends B
		configAFile := filepath.Join(tempDir, "a.yaml")
		configAContent := `
extends: b.yaml
metadata:
  name: Config A
outputs:
  - path: a-output.md
`
		require.NoError(t, os.WriteFile(configAFile, []byte(configAContent), 0o644))
		
		// Create config B that extends A (circular)
		configBFile := filepath.Join(tempDir, "b.yaml")
		configBContent := `
extends: a.yaml
metadata:
  name: Config B
outputs:
  - path: b-output.md
`
		require.NoError(t, os.WriteFile(configBFile, []byte(configBContent), 0o644))
		
		_, err := config.LoadConfigWithIncludes(context.Background(), configAFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circular include detected")
	})
	
	t.Run("empty extends field", func(t *testing.T) {
		tempDir := t.TempDir()
		
		configFile := filepath.Join(tempDir, "config.yaml")
		configContent := `
extends: ""
metadata:
  name: Test Project
outputs:
  - path: output.md
`
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))
		
		cfg, err := config.LoadConfigWithIncludes(context.Background(), configFile)
		require.NoError(t, err)
		assert.Equal(t, "Test Project", cfg.Metadata.Name)
		assert.Empty(t, cfg.Extends) // Should be cleared
	})
}
