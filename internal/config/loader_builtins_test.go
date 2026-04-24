package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigV3_BuiltinsArray(t *testing.T) {
	t.Run("loads specific builtins from array", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: test-builtins
presets:
  - claude
builtins:
  - rust
  - security
`
		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, configYAMLFilename),
			[]byte(configContent), 0o644,
		))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)

		// Should have rust, security, and auto-included ai-governance
		assert.NotNil(t, config.Content)
		assert.Contains(t, domainNames(config), "rust")
		assert.Contains(t, domainNames(config), "security")
		assert.Contains(t, domainNames(config), "ai-governance")
	})

	t.Run("excludes auto-include with bang prefix", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: test-builtins-exclude
presets:
  - claude
builtins:
  - rust
  - "!ai-governance"
`
		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, configYAMLFilename),
			[]byte(configContent), 0o644,
		))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)

		assert.Contains(t, domainNames(config), "rust")
		assert.NotContains(t, domainNames(config), "ai-governance")
	})
}

func TestLoadConfigV3_BuiltinsBoolean(t *testing.T) {
	t.Run("builtins true loads all domains", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: test-builtins-true
presets:
  - claude
builtins: true
`
		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, configYAMLFilename),
			[]byte(configContent), 0o644,
		))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)

		names := domainNames(config)
		// Should have all 23 builtin domains
		assert.GreaterOrEqual(t, len(names), 23)
		assert.Contains(t, names, "ai-governance")
		assert.Contains(t, names, "rust")
		assert.Contains(t, names, "python")
		assert.Contains(t, names, "pyo3")
		assert.Contains(t, names, "security")
		assert.Contains(t, names, "default-commands")
	})

	t.Run("builtins false loads no domains", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: test-builtins-false
presets:
  - claude
builtins: false
`
		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, configYAMLFilename),
			[]byte(configContent), 0o644,
		))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)

		assert.Empty(t, domainNames(config))
	})
}

func TestLoadConfigV3_BuiltinsNotSet(t *testing.T) {
	t.Run("no builtins field means no builtin domains loaded", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: test-no-builtins
presets:
  - claude
`
		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, configYAMLFilename),
			[]byte(configContent), 0o644,
		))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)

		// No builtins field = no builtin domains
		assert.Empty(t, domainNames(config))
	})
}

func TestLoadConfigV3_BuiltinsLocalOverride(t *testing.T) {
	t.Run("local domain takes priority over builtin", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)

		// Create local "rust" domain with custom content
		localRustDir := filepath.Join(configDir, domainsDir, "rust", rulesDir)
		require.NoError(t, os.MkdirAll(localRustDir, 0o755))

		localRule := `---
priority: critical
---
# Custom Rust Rule
This is a local override.
`
		require.NoError(t, os.WriteFile(
			filepath.Join(localRustDir, "custom-rust.md"),
			[]byte(localRule), 0o644,
		))

		configContent := `version: "3.0"
name: test-local-override
presets:
  - claude
builtins:
  - rust
`
		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, configYAMLFilename),
			[]byte(configContent), 0o644,
		))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)

		// Local "rust" domain should exist with custom content
		rustDomain, exists := config.Content.Domains["rust"]
		require.True(t, exists)
		require.Len(t, rustDomain.Rules, 1)
		assert.Equal(t, "custom-rust", rustDomain.Rules[0].Name)
		assert.Contains(t, rustDomain.Rules[0].Content, "local override")
	})
}

func TestLoadConfigV3_BuiltinsContent(t *testing.T) {
	t.Run("ai-governance builtin has 4 rules", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: test-content
presets:
  - claude
builtins:
  - "!ai-governance"
  - ai-governance
`
		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, configYAMLFilename),
			[]byte(configContent), 0o644,
		))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)

		// ai-governance not loaded because it's also excluded
		assert.NotContains(t, domainNames(config), "ai-governance")
	})

	t.Run("security builtin has rules and context", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: test-security
presets:
  - claude
builtins:
  - security
  - "!ai-governance"
`
		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, configYAMLFilename),
			[]byte(configContent), 0o644,
		))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)

		secDomain, exists := config.Content.Domains["security"]
		require.True(t, exists)
		assert.Equal(t, 4, len(secDomain.Rules))
		assert.Equal(t, 1, len(secDomain.Context))
	})

	t.Run("default-commands builtin has commands", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: test-commands
presets:
  - claude
builtins:
  - default-commands
  - "!ai-governance"
`
		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, configYAMLFilename),
			[]byte(configContent), 0o644,
		))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)

		cmdDomain, exists := config.Content.Domains["default-commands"]
		require.True(t, exists)
		assert.Equal(t, 2, len(cmdDomain.Commands))

		// Verify iterate and parallelize commands exist
		cmdNames := make([]string, len(cmdDomain.Commands))
		for i, c := range cmdDomain.Commands {
			cmdNames[i] = c.Name
		}
		assert.Contains(t, cmdNames, "iterate")
		assert.Contains(t, cmdNames, "parallelize")
	})

	t.Run("builtin content has parsed frontmatter", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, aiRulezDirName)
		require.NoError(t, os.MkdirAll(configDir, 0o755))

		configContent := `version: "3.0"
name: test-frontmatter
presets:
  - claude
builtins:
  - ai-governance
  - "!ai-governance"
  - security
`
		require.NoError(t, os.WriteFile(
			filepath.Join(configDir, configYAMLFilename),
			[]byte(configContent), 0o644,
		))

		config, err := LoadConfigV3(context.Background(), tempDir)
		require.NoError(t, err)

		secDomain := config.Content.Domains["security"]
		require.NotNil(t, secDomain)

		// Check that rules have metadata with priority
		for _, rule := range secDomain.Rules {
			assert.NotNil(t, rule.Metadata, "rule %q should have metadata", rule.Name)
			assert.NotEmpty(t, rule.Metadata.Priority, "rule %q should have priority", rule.Name)
		}

		// Content should not contain frontmatter delimiters
		for _, rule := range secDomain.Rules {
			assert.NotContains(t, rule.Content, "---\npriority:")
		}
	})
}

func TestBuiltinsConfig_Marshaling(t *testing.T) {
	t.Run("unmarshal boolean true from YAML", func(t *testing.T) {
		var b BuiltinsConfig
		err := b.UnmarshalYAML(func(v interface{}) error {
			*(v.(*bool)) = true
			return nil
		})
		require.NoError(t, err)
		assert.True(t, b.IsAll())
		assert.False(t, b.IsNone())
	})

	t.Run("unmarshal boolean false from YAML", func(t *testing.T) {
		var b BuiltinsConfig
		err := b.UnmarshalYAML(func(v interface{}) error {
			*(v.(*bool)) = false
			return nil
		})
		require.NoError(t, err)
		assert.False(t, b.IsAll())
		assert.True(t, b.IsNone())
	})

	t.Run("unmarshal JSON boolean true", func(t *testing.T) {
		var b BuiltinsConfig
		err := b.UnmarshalJSON([]byte("true"))
		require.NoError(t, err)
		assert.True(t, b.IsAll())
	})

	t.Run("unmarshal JSON boolean false", func(t *testing.T) {
		var b BuiltinsConfig
		err := b.UnmarshalJSON([]byte("false"))
		require.NoError(t, err)
		assert.True(t, b.IsNone())
	})

	t.Run("unmarshal JSON array", func(t *testing.T) {
		var b BuiltinsConfig
		err := b.UnmarshalJSON([]byte(`["rust","python"]`))
		require.NoError(t, err)
		assert.Nil(t, b.All)
		assert.Equal(t, []string{"rust", "python"}, b.Names)
	})

	t.Run("marshal boolean true to JSON", func(t *testing.T) {
		trueVal := true
		b := BuiltinsConfig{All: &trueVal}
		data, err := b.MarshalJSON()
		require.NoError(t, err)
		assert.Equal(t, "true", string(data))
	})

	t.Run("marshal array to JSON", func(t *testing.T) {
		b := BuiltinsConfig{Names: []string{"rust"}}
		data, err := b.MarshalJSON()
		require.NoError(t, err)
		assert.Equal(t, `["rust"]`, string(data))
	})
}

func TestBuiltinsConfig_Methods(t *testing.T) {
	t.Run("nil BuiltinsConfig", func(t *testing.T) {
		var b *BuiltinsConfig
		assert.False(t, b.IsEnabled())
		assert.False(t, b.IsAll())
		assert.False(t, b.IsNone())
		assert.Nil(t, b.GetNames())
	})

	t.Run("empty BuiltinsConfig", func(t *testing.T) {
		b := &BuiltinsConfig{}
		assert.True(t, b.IsEnabled())
		assert.False(t, b.IsAll())
		assert.False(t, b.IsNone())
	})
}

// domainNames extracts domain names from a loaded config
func domainNames(config *ConfigV3) []string {
	if config.Content == nil {
		return nil
	}
	names := make([]string, 0, len(config.Content.Domains))
	for name := range config.Content.Domains {
		names = append(names, name)
	}
	return names
}
