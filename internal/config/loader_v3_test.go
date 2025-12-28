package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDetectConfigVersion(t *testing.T) {
	t.Run("detects v3 config", func(t *testing.T) {
		tempDir := t.TempDir()
		v3Dir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(v3Dir, 0o755))

		version, err := DetectConfigVersion(tempDir)
		require.NoError(t, err)
		assert.Equal(t, "v3", version)
	})

	t.Run("detects v2 config with yaml", func(t *testing.T) {
		tempDir := t.TempDir()
		v2File := filepath.Join(tempDir, "ai-rulez.yaml")
		require.NoError(t, os.WriteFile(v2File, []byte("metadata:\n  name: test\n"), 0o644))

		version, err := DetectConfigVersion(tempDir)
		require.NoError(t, err)
		assert.Equal(t, "v2", version)
	})

	t.Run("detects v2 config with yml", func(t *testing.T) {
		tempDir := t.TempDir()
		v2File := filepath.Join(tempDir, "ai-rulez.yml")
		require.NoError(t, os.WriteFile(v2File, []byte("metadata:\n  name: test\n"), 0o644))

		version, err := DetectConfigVersion(tempDir)
		require.NoError(t, err)
		assert.Equal(t, "v2", version)
	})

	t.Run("returns empty string when no config found", func(t *testing.T) {
		tempDir := t.TempDir()

		version, err := DetectConfigVersion(tempDir)
		require.NoError(t, err)
		assert.Equal(t, "", version)
	})

	t.Run("prefers v3 when both exist", func(t *testing.T) {
		tempDir := t.TempDir()
		v3Dir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(v3Dir, 0o755))
		v2File := filepath.Join(tempDir, "ai-rulez.yaml")
		require.NoError(t, os.WriteFile(v2File, []byte("metadata:\n  name: test\n"), 0o644))

		version, err := DetectConfigVersion(tempDir)
		require.NoError(t, err)
		assert.Equal(t, "v3", version)
	})
}

func TestLoadConfigV3_YAML(t *testing.T) {
	t.Run("loads minimal YAML config", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: test-project
presets:
  - claude
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "3.0", config.Version)
		assert.Equal(t, "test-project", config.Name)
		assert.Len(t, config.Presets, 1)
		assert.True(t, config.Presets[0].IsBuiltIn())
		assert.Equal(t, "claude", config.Presets[0].BuiltIn)
	})

	t.Run("loads full YAML config with profiles", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: my-project
description: A test project
presets:
  - claude
  - cursor
  - name: custom
    type: markdown
    path: CUSTOM.md
default: full
profiles:
  full:
    - backend
    - frontend
  backend:
    - backend
gitignore: true
`
		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "3.0", config.Version)
		assert.Equal(t, "my-project", config.Name)
		assert.Equal(t, "A test project", config.Description)
		assert.Len(t, config.Presets, 3)
		assert.True(t, config.Presets[0].IsBuiltIn())
		assert.Equal(t, "claude", config.Presets[0].BuiltIn)
		assert.True(t, config.Presets[1].IsBuiltIn())
		assert.Equal(t, "cursor", config.Presets[1].BuiltIn)
		assert.False(t, config.Presets[2].IsBuiltIn())
		assert.Equal(t, "custom", config.Presets[2].Name)
		assert.Equal(t, PresetTypeMarkdown, config.Presets[2].Type)
		assert.Equal(t, "CUSTOM.md", config.Presets[2].Path)
		assert.Equal(t, "full", config.Default)
		assert.Len(t, config.Profiles, 2)
		assert.Equal(t, []string{"backend", "frontend"}, config.Profiles["full"])
		assert.Equal(t, []string{"backend"}, config.Profiles["backend"])
		assert.True(t, *config.Gitignore)
	})

	t.Run("returns error when .ai-rulez not found", func(t *testing.T) {
		tempDir := t.TempDir()

		_, err := LoadConfigV3(context.Background(), tempDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ".ai-rulez directory not found")
	})

	t.Run("returns error when .ai-rulez is a file", func(t *testing.T) {
		tempDir := t.TempDir()
		notADir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.WriteFile(notADir, []byte("not a directory"), 0o644))

		_, err := LoadConfigV3(context.Background(), tempDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})

	t.Run("returns error when no config file found", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		_, err := LoadConfigV3(context.Background(), tempDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no config file found")
	})

	t.Run("returns error on invalid YAML", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configFile := filepath.Join(configDir, configYAMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte("invalid: yaml: syntax:"), 0o644))

		_, err := LoadConfigV3(context.Background(), tempDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse YAML config")
	})
}

func TestLoadConfigV3_JSON(t *testing.T) {
	t.Run("loads minimal JSON config", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configData := map[string]interface{}{
			"version": "3.0",
			"name":    "test-project",
			"presets": []string{"claude"},
		}
		configBytes, err := json.MarshalIndent(configData, "", "  ")
		require.NoError(t, err)

		configFile := filepath.Join(configDir, configJSONFilename)
		require.NoError(t, os.WriteFile(configFile, configBytes, 0o644))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "3.0", config.Version)
		assert.Equal(t, "test-project", config.Name)
		assert.Len(t, config.Presets, 1)
	})

	t.Run("loads full JSON config", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configData := map[string]interface{}{
			"version":     "3.0",
			"name":        "my-project",
			"description": "A test project",
			"presets": []interface{}{
				"claude",
				"cursor",
				map[string]string{
					"name": "custom",
					"type": "markdown",
					"path": "CUSTOM.md",
				},
			},
			"default": "full",
			"profiles": map[string][]string{
				"full":    {"backend", "frontend"},
				"backend": {"backend"},
			},
		}
		configBytes, err := json.MarshalIndent(configData, "", "  ")
		require.NoError(t, err)

		configFile := filepath.Join(configDir, configJSONFilename)
		require.NoError(t, os.WriteFile(configFile, configBytes, 0o644))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "3.0", config.Version)
		assert.Equal(t, "my-project", config.Name)
		assert.Len(t, config.Presets, 3)
		assert.Equal(t, "full", config.Default)
		assert.Len(t, config.Profiles, 2)
	})

	t.Run("returns error on invalid JSON", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configFile := filepath.Join(configDir, configJSONFilename)
		require.NoError(t, os.WriteFile(configFile, []byte("{invalid json}"), 0o644))

		_, err := LoadConfigV3(context.Background(), tempDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse JSON config")
	})
}

func TestScanContentTree(t *testing.T) {
	t.Run("scans empty directories", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		tree, err := scanContentTree(configDir)
		require.NoError(t, err)
		assert.NotNil(t, tree)
		assert.Empty(t, tree.Rules)
		assert.Empty(t, tree.Context)
		assert.Empty(t, tree.Skills)
		assert.Empty(t, tree.Agents)
		assert.Empty(t, tree.Domains)
	})

	t.Run("scans rules directory", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		rulesPath := filepath.Join(configDir, rulesDir)
		require.NoError(t, os.MkdirAll(rulesPath, 0o755))

		// Create test rule files
		rule1 := filepath.Join(rulesPath, "rule1.md")
		require.NoError(t, os.WriteFile(rule1, []byte("# Rule 1\nContent"), 0o644))
		rule2 := filepath.Join(rulesPath, "rule2.md")
		require.NoError(t, os.WriteFile(rule2, []byte("# Rule 2\nContent"), 0o644))

		tree, err := scanContentTree(configDir)
		require.NoError(t, err)
		assert.Len(t, tree.Rules, 2)
		assert.Equal(t, "rule1", tree.Rules[0].Name)
		assert.Equal(t, "# Rule 1\nContent", tree.Rules[0].Content)
		assert.Equal(t, "rule2", tree.Rules[1].Name)
	})

	t.Run("scans context directory", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		contextPath := filepath.Join(configDir, contextDir)
		require.NoError(t, os.MkdirAll(contextPath, 0o755))

		context1 := filepath.Join(contextPath, "architecture.md")
		require.NoError(t, os.WriteFile(context1, []byte("# Architecture\nDetails"), 0o644))

		tree, err := scanContentTree(configDir)
		require.NoError(t, err)
		assert.Len(t, tree.Context, 1)
		assert.Equal(t, "architecture", tree.Context[0].Name)
		assert.Equal(t, "# Architecture\nDetails", tree.Context[0].Content)
	})

	t.Run("scans skills directory", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		skillsPath := filepath.Join(configDir, skillsDir)
		require.NoError(t, os.MkdirAll(skillsPath, 0o755))

		// Create skill with SKILL.md
		skill1Path := filepath.Join(skillsPath, "code-review")
		require.NoError(t, os.MkdirAll(skill1Path, 0o755))
		skill1File := filepath.Join(skill1Path, skillMarkerFile)
		require.NoError(t, os.WriteFile(skill1File, []byte("# Code Review Skill"), 0o644))

		// Create another skill
		skill2Path := filepath.Join(skillsPath, "debugging")
		require.NoError(t, os.MkdirAll(skill2Path, 0o755))
		skill2File := filepath.Join(skill2Path, skillMarkerFile)
		require.NoError(t, os.WriteFile(skill2File, []byte("# Debugging Skill"), 0o644))

		tree, err := scanContentTree(configDir)
		require.NoError(t, err)
		assert.Len(t, tree.Skills, 2)
	})

	t.Run("scans domains directory", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		domainsPath := filepath.Join(configDir, domainsDir)
		require.NoError(t, os.MkdirAll(domainsPath, 0o755))

		// Create backend domain
		backendPath := filepath.Join(domainsPath, "backend")
		backendRulesPath := filepath.Join(backendPath, rulesDir)
		require.NoError(t, os.MkdirAll(backendRulesPath, 0o755))
		backendRule := filepath.Join(backendRulesPath, "api.md")
		require.NoError(t, os.WriteFile(backendRule, []byte("# API Rules"), 0o644))

		// Create frontend domain
		frontendPath := filepath.Join(domainsPath, "frontend")
		frontendContextPath := filepath.Join(frontendPath, contextDir)
		require.NoError(t, os.MkdirAll(frontendContextPath, 0o755))
		frontendContext := filepath.Join(frontendContextPath, "ui.md")
		require.NoError(t, os.WriteFile(frontendContext, []byte("# UI Context"), 0o644))

		tree, err := scanContentTree(configDir)
		require.NoError(t, err)
		assert.Len(t, tree.Domains, 2)
		assert.Contains(t, tree.Domains, "backend")
		assert.Contains(t, tree.Domains, "frontend")
		assert.Len(t, tree.Domains["backend"].Rules, 1)
		assert.Equal(t, "api", tree.Domains["backend"].Rules[0].Name)
		assert.Len(t, tree.Domains["frontend"].Context, 1)
		assert.Equal(t, "ui", tree.Domains["frontend"].Context[0].Name)
	})

	t.Run("ignores non-markdown files", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		rulesPath := filepath.Join(configDir, rulesDir)
		require.NoError(t, os.MkdirAll(rulesPath, 0o755))

		// Create markdown file
		require.NoError(t, os.WriteFile(filepath.Join(rulesPath, "rule.md"), []byte("content"), 0o644))
		// Create non-markdown file
		require.NoError(t, os.WriteFile(filepath.Join(rulesPath, "readme.txt"), []byte("text"), 0o644))

		tree, err := scanContentTree(configDir)
		require.NoError(t, err)
		assert.Len(t, tree.Rules, 1)
		assert.Equal(t, "rule", tree.Rules[0].Name)
	})
}

func TestParseFrontmatter(t *testing.T) {
	t.Run("parses frontmatter with priority", func(t *testing.T) {
		content := `---
priority: high
---

# Rule Content

Some content here.`

		metadata, actualContent := parseFrontmatter(content)
		require.NotNil(t, metadata)
		assert.Equal(t, "high", metadata.Priority)
		assert.Equal(t, "# Rule Content\n\nSome content here.", actualContent)
	})

	t.Run("parses frontmatter with targets", func(t *testing.T) {
		content := `---
priority: medium
targets:
  - "*.py"
  - "backend/*"
---

Content here.`

		metadata, actualContent := parseFrontmatter(content)
		require.NotNil(t, metadata)
		assert.Equal(t, "medium", metadata.Priority)
		assert.Equal(t, []string{"*.py", "backend/*"}, metadata.Targets)
		assert.Equal(t, "Content here.", actualContent)
	})

	t.Run("returns nil metadata when no frontmatter", func(t *testing.T) {
		content := "# Just regular content"

		metadata, actualContent := parseFrontmatter(content)
		assert.Nil(t, metadata)
		assert.Equal(t, content, actualContent)
	})

	t.Run("returns nil metadata when frontmatter not closed", func(t *testing.T) {
		content := `---
priority: high

No closing marker`

		metadata, actualContent := parseFrontmatter(content)
		assert.Nil(t, metadata)
		assert.Equal(t, content, actualContent)
	})

	t.Run("handles invalid frontmatter YAML", func(t *testing.T) {
		content := `---
invalid: yaml: syntax:
---

Content`

		metadata, actualContent := parseFrontmatter(content)
		assert.Nil(t, metadata)
		assert.Equal(t, content, actualContent)
	})

	t.Run("handles extra frontmatter fields", func(t *testing.T) {
		content := `---
priority: low
custom_field: value
another: data
---

Content`

		metadata, actualContent := parseFrontmatter(content)
		require.NotNil(t, metadata)
		assert.Equal(t, "low", metadata.Priority)
		assert.Equal(t, "Content", actualContent)
	})
}

func TestValidateV3(t *testing.T) {
	t.Run("validates minimal config", func(t *testing.T) {
		config := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{
				{BuiltIn: "claude"},
			},
		}

		err := config.ValidateV3()
		assert.NoError(t, err)
	})

	t.Run("fails on invalid version", func(t *testing.T) {
		config := &ConfigV3{
			Version: "2.0",
			Name:    "test",
			Presets: []PresetV3{
				{BuiltIn: "claude"},
			},
		}

		err := config.ValidateV3()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid version")
	})

	t.Run("fails on missing name", func(t *testing.T) {
		config := &ConfigV3{
			Version: "3.0",
			Name:    "",
			Presets: []PresetV3{
				{BuiltIn: "claude"},
			},
		}

		err := config.ValidateV3()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required field 'name'")
	})

	t.Run("fails on missing presets", func(t *testing.T) {
		config := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{},
		}

		err := config.ValidateV3()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one preset is required")
	})

	t.Run("fails on invalid built-in preset", func(t *testing.T) {
		config := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{
				{BuiltIn: "invalid-preset"},
			},
		}

		err := config.ValidateV3()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown built-in preset")
	})

	t.Run("fails on custom preset without name", func(t *testing.T) {
		config := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{
				{
					Type: PresetTypeMarkdown,
					Path: "custom.md",
				},
			},
		}

		err := config.ValidateV3()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required field 'name'")
	})

	t.Run("fails on custom preset without type", func(t *testing.T) {
		config := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{
				{
					Name: "custom",
					Path: "custom.md",
				},
			},
		}

		err := config.ValidateV3()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required field 'type'")
	})

	t.Run("fails on custom preset without path", func(t *testing.T) {
		config := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{
				{
					Name: "custom",
					Type: PresetTypeMarkdown,
				},
			},
		}

		err := config.ValidateV3()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required field 'path'")
	})

	t.Run("fails on custom preset with invalid type", func(t *testing.T) {
		config := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{
				{
					Name: "custom",
					Type: "invalid",
					Path: "custom.md",
				},
			},
		}

		err := config.ValidateV3()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid type")
	})

	t.Run("fails when default specified without profiles", func(t *testing.T) {
		config := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{
				{BuiltIn: "claude"},
			},
			Default: "full",
		}

		err := config.ValidateV3()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no profiles defined")
	})

	t.Run("fails when default profile doesn't exist", func(t *testing.T) {
		config := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{
				{BuiltIn: "claude"},
			},
			Default: "missing",
			Profiles: map[string][]string{
				"backend": {"backend"},
			},
		}

		err := config.ValidateV3()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist in profiles")
	})

	t.Run("validates full config", func(t *testing.T) {
		config := &ConfigV3{
			Version: "3.0",
			Name:    "test",
			Presets: []PresetV3{
				{BuiltIn: "claude"},
				{
					Name: "custom",
					Type: PresetTypeMarkdown,
					Path: "CUSTOM.md",
				},
			},
			Default: "full",
			Profiles: map[string][]string{
				"full":    {"backend", "frontend"},
				"backend": {"backend"},
			},
		}

		err := config.ValidateV3()
		assert.NoError(t, err)
	})
}

func TestPresetV3Marshaling(t *testing.T) {
	t.Run("unmarshals built-in preset from YAML", func(t *testing.T) {
		yamlContent := `presets:
  - claude
  - cursor
`
		var config struct {
			Presets []PresetV3 `yaml:"presets"`
		}
		err := yaml.Unmarshal([]byte(yamlContent), &config)
		require.NoError(t, err)
		assert.Len(t, config.Presets, 2)
		assert.True(t, config.Presets[0].IsBuiltIn())
		assert.Equal(t, "claude", config.Presets[0].BuiltIn)
		assert.True(t, config.Presets[1].IsBuiltIn())
		assert.Equal(t, "cursor", config.Presets[1].BuiltIn)
	})

	t.Run("unmarshals custom preset from YAML", func(t *testing.T) {
		yamlContent := `presets:
  - name: custom
    type: markdown
    path: CUSTOM.md
    template: "Custom template"
`
		var config struct {
			Presets []PresetV3 `yaml:"presets"`
		}
		err := yaml.Unmarshal([]byte(yamlContent), &config)
		require.NoError(t, err)
		assert.Len(t, config.Presets, 1)
		assert.False(t, config.Presets[0].IsBuiltIn())
		assert.Equal(t, "custom", config.Presets[0].Name)
		assert.Equal(t, PresetTypeMarkdown, config.Presets[0].Type)
		assert.Equal(t, "CUSTOM.md", config.Presets[0].Path)
		assert.Equal(t, "Custom template", config.Presets[0].Template)
	})

	t.Run("unmarshals mixed presets from YAML", func(t *testing.T) {
		yamlContent := `presets:
  - claude
  - name: custom
    type: markdown
    path: CUSTOM.md
  - cursor
`
		var config struct {
			Presets []PresetV3 `yaml:"presets"`
		}
		err := yaml.Unmarshal([]byte(yamlContent), &config)
		require.NoError(t, err)
		assert.Len(t, config.Presets, 3)
		assert.True(t, config.Presets[0].IsBuiltIn())
		assert.Equal(t, "claude", config.Presets[0].BuiltIn)
		assert.False(t, config.Presets[1].IsBuiltIn())
		assert.Equal(t, "custom", config.Presets[1].Name)
		assert.True(t, config.Presets[2].IsBuiltIn())
		assert.Equal(t, "cursor", config.Presets[2].BuiltIn)
	})

	t.Run("unmarshals built-in preset from JSON", func(t *testing.T) {
		jsonContent := `{
  "presets": ["claude", "cursor"]
}`
		var config struct {
			Presets []PresetV3 `json:"presets"`
		}
		err := json.Unmarshal([]byte(jsonContent), &config)
		require.NoError(t, err)
		assert.Len(t, config.Presets, 2)
		assert.True(t, config.Presets[0].IsBuiltIn())
		assert.Equal(t, "claude", config.Presets[0].BuiltIn)
	})

	t.Run("unmarshals custom preset from JSON", func(t *testing.T) {
		jsonContent := `{
  "presets": [
    {
      "name": "custom",
      "type": "markdown",
      "path": "CUSTOM.md"
    }
  ]
}`
		var config struct {
			Presets []PresetV3 `json:"presets"`
		}
		err := json.Unmarshal([]byte(jsonContent), &config)
		require.NoError(t, err)
		assert.Len(t, config.Presets, 1)
		assert.False(t, config.Presets[0].IsBuiltIn())
		assert.Equal(t, "custom", config.Presets[0].Name)
	})
}

func TestScanAgents(t *testing.T) {
	t.Run("scans agents directory", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		agentsPath := filepath.Join(configDir, agentsDir)
		require.NoError(t, os.MkdirAll(agentsPath, 0o755))

		// Create test agent files
		agent1 := filepath.Join(agentsPath, "agent1.md")
		require.NoError(t, os.WriteFile(agent1, []byte("# Agent 1\nYou are an agent."), 0o644))
		agent2 := filepath.Join(agentsPath, "agent2.md")
		require.NoError(t, os.WriteFile(agent2, []byte("# Agent 2\nYou are another agent."), 0o644))

		agents, err := scanAgents(agentsPath)
		require.NoError(t, err)
		assert.Len(t, agents, 2)
		assert.Equal(t, "agent1", agents[0].Name)
		assert.Equal(t, "# Agent 1\nYou are an agent.", agents[0].Content)
		assert.Equal(t, "agent2", agents[1].Name)
	})

	t.Run("scans agents with metadata", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		agentsPath := filepath.Join(configDir, agentsDir)
		require.NoError(t, os.MkdirAll(agentsPath, 0o755))

		// Create agent with metadata
		agentContent := `---
name: test-agent
description: A test agent
model: claude-3-sonnet
tools: code_execution,file_search
permission_mode: safe
---

You are a helpful assistant.`

		agentPath := filepath.Join(agentsPath, "test.md")
		require.NoError(t, os.WriteFile(agentPath, []byte(agentContent), 0o644))

		agents, err := scanAgents(agentsPath)
		require.NoError(t, err)
		assert.Len(t, agents, 1)
		assert.Equal(t, "test", agents[0].Name)
		assert.NotNil(t, agents[0].Metadata)
		assert.Equal(t, "A test agent", agents[0].Metadata.Extra["description"])
		assert.Equal(t, "claude-3-sonnet", agents[0].Metadata.Extra["model"])
	})

	t.Run("ignores non-markdown files in agents", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		agentsPath := filepath.Join(configDir, agentsDir)
		require.NoError(t, os.MkdirAll(agentsPath, 0o755))

		// Create markdown file
		require.NoError(t, os.WriteFile(filepath.Join(agentsPath, "agent.md"), []byte("content"), 0o644))
		// Create non-markdown file
		require.NoError(t, os.WriteFile(filepath.Join(agentsPath, "readme.txt"), []byte("text"), 0o644))
		// Create subdirectory (should be ignored)
		require.NoError(t, os.MkdirAll(filepath.Join(agentsPath, "subdir"), 0o755))

		agents, err := scanAgents(agentsPath)
		require.NoError(t, err)
		assert.Len(t, agents, 1)
		assert.Equal(t, "agent", agents[0].Name)
	})

	t.Run("returns empty slice for non-existent directory", func(t *testing.T) {
		nonExistentPath := "/non/existent/path"
		agents, err := scanAgents(nonExistentPath)
		require.NoError(t, err)
		assert.Empty(t, agents)
	})

	t.Run("scans agents in content tree", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create agents directory with content
		agentsPath := filepath.Join(configDir, agentsDir)
		require.NoError(t, os.MkdirAll(agentsPath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(agentsPath, "agent1.md"), []byte("content1"), 0o644))

		tree, err := scanContentTree(configDir)
		require.NoError(t, err)
		assert.Len(t, tree.Agents, 1)
		assert.Equal(t, "agent1", tree.Agents[0].Name)
	})

	t.Run("scans domain agents", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		// Create backend domain with agents
		backendPath := filepath.Join(configDir, domainsDir, "backend")
		backendAgentsPath := filepath.Join(backendPath, agentsDir)
		require.NoError(t, os.MkdirAll(backendAgentsPath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(backendAgentsPath, "api-agent.md"), []byte("API agent"), 0o644))

		tree, err := scanContentTree(configDir)
		require.NoError(t, err)
		assert.Len(t, tree.Domains, 1)
		assert.Contains(t, tree.Domains, "backend")
		assert.Len(t, tree.Domains["backend"].Agents, 1)
		assert.Equal(t, "api-agent", tree.Domains["backend"].Agents[0].Name)
	})
}
