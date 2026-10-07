package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindLegacyConfig(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string // relative to the project, "" for none
	}{
		{"V3 directory config.yaml", map[string]string{".ai-rulez/config.yaml": "version: \"3.0\"\n"}, ".ai-rulez/config.yaml"},
		{"V3 directory config.yml", map[string]string{".ai-rulez/config.yml": "version: \"3.0\"\n"}, ".ai-rulez/config.yml"},
		{"V3 directory config.json", map[string]string{".ai-rulez/config.json": "{}"}, ".ai-rulez/config.json"},
		{"V2 flat ai-rulez.yaml", map[string]string{"ai-rulez.yaml": "metadata:\n  name: x\n"}, "ai-rulez.yaml"},
		{"V2 flat .ai-rulez.yml", map[string]string{".ai-rulez.yml": "metadata:\n  name: x\n"}, ".ai-rulez.yml"},
		{"V2 flat ai_rulez.yaml", map[string]string{"ai_rulez.yaml": "metadata:\n  name: x\n"}, "ai_rulez.yaml"},
		{"config.toml wins over a stale config.yaml", map[string]string{
			".ai-rulez/config.toml": "version = \"4.0\"\n", ".ai-rulez/config.yaml": "x: 1\n",
		}, ""},
		{"nothing", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			for rel, body := range tt.files {
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644))
			}

			// Act
			got := FindLegacyConfig(root)

			// Assert
			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, filepath.Join(root, filepath.FromSlash(tt.want)), got)
		})
	}
}

// TestLoadConfig_LegacyFilesAreRefused covers every way a V2/V3-only project is
// reached: the error names the file and the 4.x migration, and wraps ErrLegacyConfig.
func TestLoadConfig_LegacyFilesAreRefused(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		load     func(root string) error
		wantHint string
	}{
		{"V3 config.yaml via LoadConfig", ".ai-rulez/config.yaml", func(root string) error {
			_, err := LoadConfig(context.Background(), root)
			return err
		}, ""},
		{"V3 config.json via LoadConfig", ".ai-rulez/config.json", func(root string) error {
			_, err := LoadConfig(context.Background(), root)
			return err
		}, ""},
		{"V2 ai-rulez.yaml via LoadConfig", "ai-rulez.yaml", func(root string) error {
			_, err := LoadConfig(context.Background(), root)
			return err
		}, "move it to .ai-rulez/config.yaml first"},
		{"V3 config.yaml via an explicit file path", ".ai-rulez/config.yaml", func(root string) error {
			_, err := LoadConfigFromFile(context.Background(), filepath.Join(root, ".ai-rulez", "config.yaml"))
			return err
		}, ""},
		{"V3 config.yaml via the config directory path", ".ai-rulez/config.yaml", func(root string) error {
			_, err := LoadConfigFromFile(context.Background(), filepath.Join(root, ".ai-rulez"))
			return err
		}, ""},
		{"V3 config.yaml found by discovery", ".ai-rulez/config.yaml", func(root string) error {
			_, err := FindConfigFile(root)
			return err
		}, ""},
		{"V3 config.local.yaml beside a config.toml", ".ai-rulez/config.local.yaml", func(root string) error {
			require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "config.toml"),
				[]byte("version = \"4.0\"\nname = \"p\"\npresets = [\"claude\"]\n"), 0o644))
			_, err := LoadConfig(context.Background(), root)
			return err
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, tt.file)), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, tt.file), []byte("version: \"3.0\"\nname: p\n"), 0o644))

			// Act
			err := tt.load(root)

			// Assert
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrLegacyConfig)
			assert.Contains(t, err.Error(), filepath.Base(tt.file))
			assert.Contains(t, err.Error(), "npx ai-rulez@4 migrate v4")
			var oe oops.OopsError
			require.ErrorAs(t, err, &oe)
			assert.Contains(t, oe.Hint(), tt.wantHint)
		})
	}
}

func TestLoadConfig_TOML(t *testing.T) {
	t.Run("loads minimal TOML config", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version = "5.0"
name = "test-project"
presets = ["claude"]
`
		configFile := filepath.Join(configDir, configTOMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		config, err := LoadConfig(context.Background(), tempDir)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "5.0", config.Version)
		assert.Equal(t, "test-project", config.Name)
		assert.Len(t, config.Presets, 1)
		assert.True(t, config.Presets[0].IsBuiltIn())
		assert.Equal(t, "claude", config.Presets[0].BuiltIn)
	})

	t.Run("loads full TOML config with profiles", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version = "4.0"
name = "my-project"
description = "A test project"
presets = ["claude", "cursor", { name = "custom", type = "markdown", path = "CUSTOM.md" }]
default = "full"
gitignore = true

[profiles]
full = ["backend", "frontend"]
backend = ["backend"]
`
		configFile := filepath.Join(configDir, configTOMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

		config, err := LoadConfig(context.Background(), tempDir)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "5.0", config.Version)
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

		_, err := LoadConfig(context.Background(), tempDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ".ai-rulez directory not found")
	})

	t.Run("returns error when .ai-rulez is a file", func(t *testing.T) {
		tempDir := t.TempDir()
		notADir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.WriteFile(notADir, []byte("not a directory"), 0o644))

		_, err := LoadConfig(context.Background(), tempDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})

	t.Run("returns error when no config file found", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		_, err := LoadConfig(context.Background(), tempDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no config file found")
	})

	t.Run("returns error on invalid TOML", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configFile := filepath.Join(configDir, configTOMLFilename)
		require.NoError(t, os.WriteFile(configFile, []byte("invalid = = toml"), 0o644))

		_, err := LoadConfig(context.Background(), tempDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse TOML config")
	})

	for _, name := range []string{"config.yaml", "config.yml"} {
		t.Run("rejects "+name+" with the migrate hint", func(t *testing.T) {
			tempDir := t.TempDir()
			configDir := filepath.Join(tempDir, aiRulezDirName)
			require.NoError(t, os.MkdirAll(configDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(configDir, name), []byte("version: \"5.0\"\nname: x\n"), 0o644))

			_, err := LoadConfig(context.Background(), tempDir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "YAML configs are no longer loaded")
			assert.Contains(t, err.Error(), "ai-rulez migrate v5")
		})
	}

	for _, version := range []string{"3.0", "4.0", "4.1"} {
		t.Run("rejects version "+version+" with the migrate hint", func(t *testing.T) {
			tempDir := t.TempDir()
			configDir := filepath.Join(tempDir, aiRulezDirName)
			require.NoError(t, os.MkdirAll(configDir, 0o755))
			body := "version = \"" + version + "\"\nname = \"x\"\npresets = [\"claude\"]\n"
			require.NoError(t, os.WriteFile(filepath.Join(configDir, configTOMLFilename), []byte(body), 0o644))

			_, err := LoadConfig(context.Background(), tempDir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ai-rulez migrate v5")
		})
	}
}

func TestLoadConfigFromFile_ExactPathAndCustomConfigDir(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".rules")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, rulesDir), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, configTOMLFilename),
		[]byte("version = \"5.0\"\nname = \"custom-dir\"\npresets = [\"codex\"]\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, rulesDir, "custom.md"),
		[]byte("---\npriority: high\n---\n# Custom\n\nExact config path\n"),
		0o644,
	))

	cfg, err := LoadConfigFromFile(context.Background(), filepath.Join(configDir, configTOMLFilename))
	require.NoError(t, err)

	assert.Equal(t, tempDir, cfg.BaseDir)
	assert.Equal(t, configDir, cfg.ConfigDir)
	assert.Equal(t, ".rules", cfg.ConfigDirName)
	require.Len(t, cfg.Content.Rules, 1)
	assert.Equal(t, "custom", cfg.Content.Rules[0].Name)
}

func TestLoadConfigFromFile_RootConfigRequiresDirectoryLayout(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))
	configFile := filepath.Join(tempDir, configTOMLFilename)
	require.NoError(t, os.WriteFile(
		configFile,
		[]byte("version = \"5.0\"\nname = \"root-config\"\npresets = [\"codex\"]\n"),
		0o644,
	))

	_, err := LoadConfigFromFile(context.Background(), configFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "directory layout required")
}

func TestLoadConfigTOML_LoadsDefaults(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, aiRulezDirName)
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	configContent := `version = "5.0"
name = "with-defaults"
presets = ["claude", "copilot"]

[defaults]
effort = "high"

[defaults.effort_by_preset]
claude = "xhigh"

[defaults.model_by_preset]
claude = "opus"
copilot = "gpt-5"
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, configTOMLFilename), []byte(configContent), 0o644))

	cfg, err := LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Defaults, "defaults table must be loaded from TOML")
	assert.Equal(t, "high", cfg.Defaults.Effort)
	assert.Equal(t, "xhigh", cfg.Defaults.EffortByPreset["claude"])
	assert.Equal(t, "opus", cfg.Defaults.ModelByPreset["claude"])
	assert.Equal(t, "gpt-5", cfg.Defaults.ModelByPreset["copilot"])
}

func TestScanContentTree(t *testing.T) {
	t.Run("scans empty directories", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		tree, err := ScanContentTree(configDir)
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

		tree, err := ScanContentTree(configDir)
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

		tree, err := ScanContentTree(configDir)
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

		tree, err := ScanContentTree(configDir)
		require.NoError(t, err)
		assert.Len(t, tree.Skills, 2)
	})

	t.Run("loads skill resources alongside SKILL.md", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		skillRoot := filepath.Join(configDir, skillsDir, "with-refs")
		require.NoError(t, os.MkdirAll(skillRoot, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skillRoot, skillMarkerFile),
			[]byte("# Body\n"), 0o644))

		require.NoError(t, os.MkdirAll(filepath.Join(skillRoot, "references"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skillRoot, "references", "api.md"),
			[]byte("---\ndescription: API\n---\n\nbody\n"), 0o644))

		require.NoError(t, os.MkdirAll(filepath.Join(skillRoot, "scripts"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skillRoot, "scripts", "x.sh"),
			[]byte("#!/bin/sh\n"), 0o755))

		tree, err := ScanContentTree(configDir)
		require.NoError(t, err)
		require.Len(t, tree.Skills, 1)
		// Body must NOT contain inlined reference text — references stay separate.
		assert.NotContains(t, tree.Skills[0].Content, "body")
		require.Len(t, tree.Skills[0].Resources, 2)
		assert.Equal(t, "references/api.md", tree.Skills[0].Resources[0].RelPath)
		assert.Equal(t, "API", tree.Skills[0].Resources[0].Description)
		assert.Equal(t, "scripts/x.sh", tree.Skills[0].Resources[1].RelPath)
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

		tree, err := ScanContentTree(configDir)
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

		tree, err := ScanContentTree(configDir)
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

		metadata, actualContent, _ := parseFrontmatter(content)
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

		metadata, actualContent, _ := parseFrontmatter(content)
		require.NotNil(t, metadata)
		assert.Equal(t, "medium", metadata.Priority)
		assert.Equal(t, []string{"*.py", "backend/*"}, metadata.Targets)
		assert.Equal(t, "Content here.", actualContent)
	})

	t.Run("returns nil metadata when no frontmatter", func(t *testing.T) {
		content := "# Just regular content"

		metadata, actualContent, _ := parseFrontmatter(content)
		assert.Nil(t, metadata)
		assert.Equal(t, content, actualContent)
	})

	t.Run("returns nil metadata when frontmatter not closed", func(t *testing.T) {
		content := `---
priority: high

No closing marker`

		metadata, actualContent, _ := parseFrontmatter(content)
		assert.Nil(t, metadata)
		assert.Equal(t, content, actualContent)
	})

	t.Run("strips malformed frontmatter block instead of leaking it (#156)", func(t *testing.T) {
		// A delimited frontmatter block whose YAML is unparseable must be
		// stripped, not returned verbatim — otherwise generate re-emits the raw
		// block after the generated frontmatter, producing two blocks. The
		// block must also be flagged malformed so validate() can fail (#175).
		content := `---
invalid: yaml: syntax:
---

Content`

		metadata, actualContent, malformed := parseFrontmatter(content)
		assert.Nil(t, metadata)
		assert.True(t, malformed)
		assert.Equal(t, "Content", actualContent)
	})

	t.Run("handles extra frontmatter fields", func(t *testing.T) {
		content := `---
priority: low
custom_field: value
another: data
---

Content`

		metadata, actualContent, _ := parseFrontmatter(content)
		require.NotNil(t, metadata)
		assert.Equal(t, "low", metadata.Priority)
		assert.Equal(t, "Content", actualContent)
	})

	t.Run("parses effort via direct unmarshal", func(t *testing.T) {
		content := `---
name: security-reviewer
description: Reviews security concerns
effort: high
---

Body`

		metadata, body, _ := parseFrontmatter(content)
		require.NotNil(t, metadata)
		assert.Equal(t, "high", metadata.Effort)
		assert.NotContains(t, metadata.Extra, "effort", "effort should not also leak into Extra")
		assert.Equal(t, "Body", body)
	})

	t.Run("parses effort via raw-map fallback when nested YAML present", func(t *testing.T) {
		// hooks: nested mapping forces the raw-map fallback path
		content := `---
name: docs-writer
description: Writes documentation
effort: low
hooks:
  PostToolUse:
    - command: echo done
---

Body`

		metadata, body, _ := parseFrontmatter(content)
		require.NotNil(t, metadata)
		assert.Equal(t, "low", metadata.Effort)
		assert.NotContains(t, metadata.Extra, "effort", "effort should not also leak into Extra in fallback")
		assert.Equal(t, "Body", body)
	})
}

func TestValidate(t *testing.T) {
	t.Run("validates minimal config", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{
				{BuiltIn: "claude"},
			},
		}

		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("fails on invalid version", func(t *testing.T) {
		config := &Config{
			Version: "6.0",
			Name:    "test",
			Presets: []Preset{
				{BuiltIn: "claude"},
			},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid version")
	})

	t.Run("fails on a v4 version with the migrate hint", func(t *testing.T) {
		config := &Config{Version: "4.0", Name: "test", Presets: []Preset{{BuiltIn: "claude"}}}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ai-rulez migrate v5")
	})

	t.Run("fails on missing name", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "",
			Presets: []Preset{
				{BuiltIn: "claude"},
			},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required field 'name'")
	})

	t.Run("fails on missing presets", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one preset is required")
	})

	t.Run("fails on invalid built-in preset", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{
				{BuiltIn: "invalid-preset"},
			},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown built-in preset")
	})

	t.Run("fails on custom preset without name", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{
				{
					Type: PresetTypeMarkdown,
					Path: "custom.md",
				},
			},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required field 'name'")
	})

	t.Run("fails on custom preset without type", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{
				{
					Name: "custom",
					Path: "custom.md",
				},
			},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required field 'type'")
	})

	t.Run("fails on custom preset without path", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{
				{
					Name: "custom",
					Type: PresetTypeMarkdown,
				},
			},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required field 'path'")
	})

	t.Run("fails on custom preset with invalid type", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{
				{
					Name: "custom",
					Type: "invalid",
					Path: "custom.md",
				},
			},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid type")
	})

	t.Run("warns but does not fail when skill description is missing", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{
				{BuiltIn: "codex"},
			},
			Content: &ContentTree{
				Skills: []ContentFile{
					{
						Name:    "core-principles",
						Path:    "/tmp/.ai-rulez/skills/core-principles/SKILL.md",
						Content: "# Core Principles",
					},
				},
			},
		}

		// Since 3.13.1, missing skill description is a warning, not an error.
		// The skill name is used as a fallback description.
		err := config.Validate()
		require.NoError(t, err)
	})

	t.Run("accepts skill description when present", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{
				{BuiltIn: "codex"},
			},
			Content: &ContentTree{
				Skills: []ContentFile{
					{
						Name:    "core-principles",
						Path:    "/tmp/.ai-rulez/skills/core-principles/SKILL.md",
						Content: "# Core Principles",
						Metadata: &Metadata{
							Extra: map[string]string{
								"description": "Project-wide engineering standards.",
							},
						},
					},
				},
			},
		}

		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("fails when default specified without profiles", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{
				{BuiltIn: "claude"},
			},
			Default: "full",
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no profiles defined")
	})

	t.Run("fails when default profile doesn't exist", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{
				{BuiltIn: "claude"},
			},
			Default: "missing",
			Profiles: map[string][]string{
				"backend": {"backend"},
			},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist in profiles")
	})

	t.Run("validates full config", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{
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

		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("fails on installed skill with empty name", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
			InstalledSkills: []InstalledSkillConfig{
				{Name: "", Source: "https://github.com/example/repo"},
			},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required field 'name'")
	})

	t.Run("fails on installed skill with empty source", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
			InstalledSkills: []InstalledSkillConfig{
				{Name: "test-skill", Source: ""},
			},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required field 'source'")
	})

	t.Run("fails on duplicate installed skill names", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
			InstalledSkills: []InstalledSkillConfig{
				{Name: "dup", Source: "https://github.com/example/a"},
				{Name: "dup", Source: "https://github.com/example/b"},
			},
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate installed skill name")
	})

	t.Run("accepts valid installed skills", func(t *testing.T) {
		config := &Config{
			Version: "5.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
			InstalledSkills: []InstalledSkillConfig{
				{Name: "skill-a", Source: "https://github.com/example/a"},
				{Name: "skill-b", Source: "/local/path"},
			},
		}

		err := config.Validate()
		assert.NoError(t, err)
	})
}

func TestSaveConfig_WritesTOML(t *testing.T) {
	t.Parallel()

	t.Run("removes dropped fields", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		aiRulezDir := filepath.Join(dir, ".ai-rulez")
		require.NoError(t, os.MkdirAll(aiRulezDir, 0o755))

		original := `version = "4.0"
name = "my-project"
presets = ["claude"]

[[installed_skills]]
name = "old-skill"
source = "https://github.com/example/old"
`
		tomlPath := filepath.Join(aiRulezDir, "config.toml")
		require.NoError(t, os.WriteFile(tomlPath, []byte(original), 0o644))

		cfg, err := loadConfigTOML(osView(filepath.Dir(tomlPath)), tomlPath)
		require.NoError(t, err)
		cfg.InstalledSkills = nil
		require.NoError(t, SaveConfig(cfg, aiRulezDir))

		err = SaveConfig(cfg, aiRulezDir)
		require.NoError(t, err)

		saved, err := os.ReadFile(tomlPath)
		require.NoError(t, err)
		content := string(saved)

		assert.NotContains(t, content, "installed_skills")
		assert.NotContains(t, content, "old-skill")
		assert.Contains(t, content, "presets")
	})

	t.Run("never creates a V3 file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfg := &Config{Version: "4.0", Name: "p", Presets: []Preset{{BuiltIn: "claude"}}}

		require.NoError(t, SaveConfig(cfg, dir))

		assert.FileExists(t, filepath.Join(dir, "config.toml"))
		assert.NoFileExists(t, filepath.Join(dir, "config.yaml"))
	})
}

func TestPresetMarshaling(t *testing.T) {
	t.Run("unmarshals built-in preset from JSON", func(t *testing.T) {
		jsonContent := `{
  "presets": ["claude", "cursor"]
}`
		var config struct {
			Presets []Preset `json:"presets"`
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
			Presets []Preset `json:"presets"`
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

		tree, err := ScanContentTree(configDir)
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

		tree, err := ScanContentTree(configDir)
		require.NoError(t, err)
		assert.Len(t, tree.Domains, 1)
		assert.Contains(t, tree.Domains, "backend")
		assert.Len(t, tree.Domains["backend"].Agents, 1)
		assert.Equal(t, "api-agent", tree.Domains["backend"].Agents[0].Name)
	})
}

func TestSaveConfig_PreservesFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	tests := []struct {
		name     string
		existing os.FileMode
		create   bool
		want     os.FileMode
	}{
		{name: "keeps 0600", existing: 0o600, create: true, want: 0o600},
		{name: "keeps 0664", existing: 0o664, create: true, want: 0o664},
		{name: "new file is 0644", want: 0o644},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), ".ai-rulez")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			path := filepath.Join(dir, "config.toml")
			if tt.create {
				require.NoError(t, os.WriteFile(path, []byte("version = \"5.0\"\nname = \"p\"\n"), tt.existing))
				require.NoError(t, os.Chmod(path, tt.existing))
			}
			cfg := &Config{Version: "5.0", Name: "p", ConfigFile: "config.toml"}

			require.NoError(t, SaveConfig(cfg, dir))

			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, info.Mode().Perm())
		})
	}
}
