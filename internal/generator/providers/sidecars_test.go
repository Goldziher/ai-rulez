package providers_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handAuthoredClaudeSettings mirrors a real, version-controlled
// .claude/settings.json: every top-level key here is owned by the human, not by
// ai-rulez, except `mcpServers`. `skillOverrides.init: "off"` is the canary —
// losing it silently re-enables a command a consumer deliberately disabled.
const handAuthoredClaudeSettings = `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(git status:*)",
      "Read"
    ],
    "deny": [
      "Bash(rm -rf:*)"
    ]
  },
  "env": {
    "AI_RULEZ_LOG": "debug"
  },
  "model": "opus",
  "outputStyle": "Explanatory",
  "statusLine": {
    "type": "command",
    "command": "npx ccstatusline@latest"
  },
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit",
        "hooks": [
          {
            "type": "command",
            "command": "task fmt"
          }
        ]
      }
    ]
  },
  "skillOverrides": {
    "init": "off"
  },
  "mcpServers": {
    "stale-hand-authored": {
      "command": "this-entry-is-replaced"
    }
  }
}
`

type rawMember struct {
	key string
	raw string
}

func jsonMembers(t *testing.T, doc string) []rawMember {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader([]byte(doc)))
	open, err := decoder.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), open)

	var members []rawMember
	for decoder.More() {
		keyToken, err := decoder.Token()
		require.NoError(t, err)
		key, ok := keyToken.(string)
		require.True(t, ok, "object key must be a string")
		var raw json.RawMessage
		require.NoError(t, decoder.Decode(&raw))
		members = append(members, rawMember{key: key, raw: string(raw)})
	}
	return members
}

func rawValue(t *testing.T, doc, key string) string {
	t.Helper()
	for _, member := range jsonMembers(t, doc) {
		if member.key == key {
			return member.raw
		}
	}
	t.Fatalf("key %q missing from document:\n%s", key, doc)
	return ""
}

// writeFixture materializes an existing on-disk file under baseDir and returns
// baseDir, so a test reads as "given this repo state".
func writeFixture(t *testing.T, relPath, content string) string {
	t.Helper()
	baseDir := t.TempDir()
	absPath := filepath.Join(baseDir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0o755))
	require.NoError(t, os.WriteFile(absPath, []byte(content), 0o644))
	return baseDir
}

func oneMCPServerConfig(baseDir string) *config.Config {
	return &config.Config{
		Name:    "test",
		BaseDir: baseDir,
		MCPServers: map[string]*config.MCPServer{
			"generated": {Command: "npx", Args: []string{"-y", "generated-server"}},
		},
	}
}

// TestClaudeSettingsSidecar_PreservesHandAuthoredKeys is the regression test for
// GitHub issue #185: emitting the settings sidecar used to replace the whole
// document with {"mcpServers": ...}, destroying every hand-authored key in a
// tracked file.
func TestClaudeSettingsSidecar_PreservesHandAuthoredKeys(t *testing.T) {
	t.Parallel()

	relPath := filepath.Join(".claude", "settings.json")
	baseDir := writeFixture(t, relPath, handAuthoredClaudeSettings)

	gen := claudeGen(t)
	outputs, err := gen.Generate(&config.ContentTree{}, baseDir, oneMCPServerConfig(baseDir))
	require.NoError(t, err)

	settings := requireFile(t, outputs, relPath)

	for _, key := range []string{
		"$schema", "permissions", "env", "model", "outputStyle", "statusLine", "hooks", "skillOverrides",
	} {
		assert.Equal(t,
			rawValue(t, handAuthoredClaudeSettings, key),
			rawValue(t, settings.Content, key),
			"user-owned key %q must survive byte-for-byte", key)
	}

	// The canary, asserted by name: a consumer that switched `init` off must not
	// have it silently switched back on.
	var parsed struct {
		SkillOverrides map[string]string `json:"skillOverrides"`
		MCPServers     map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(settings.Content), &parsed))
	assert.Equal(t, "off", parsed.SkillOverrides["init"], "skillOverrides.init must still be off")

	// Only mcpServers changed: the generated server joins the hand-written one.
	assert.Equal(t, "npx", parsed.MCPServers["generated"].Command)
	assert.Equal(t, []string{"-y", "generated-server"}, parsed.MCPServers["generated"].Args)
	assert.Contains(t, parsed.MCPServers, "stale-hand-authored",
		"a server ai-rulez did not write is the consumer's")
}

// TestClaudeSettingsSidecar_CreatesFileWhenAbsent keeps the greenfield output
// byte-identical to the pre-merge renderer so first-run output is unchanged.
func TestClaudeSettingsSidecar_CreatesFileWhenAbsent(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	gen := claudeGen(t)
	outputs, err := gen.Generate(&config.ContentTree{}, baseDir, oneMCPServerConfig(baseDir))
	require.NoError(t, err)

	settings := requireFile(t, outputs, filepath.Join(".claude", "settings.json"))
	assert.Equal(t, `{
  "mcpServers": {
    "generated": {
      "args": [
        "-y",
        "generated-server"
      ],
      "command": "npx"
    }
  }
}
`, settings.Content)
}

// TestMCPJSONSidecar_PreservesUnownedKeys proves the merge is generic to the
// JSON-object sidecars, not special-cased to .claude/settings.json. A tracked,
// hand-authored /.mcp.json is a real pattern and cannot be opted out of (the
// mcp generator runs whenever servers exist).
func TestMCPJSONSidecar_PreservesUnownedKeys(t *testing.T) {
	t.Parallel()

	const existing = `{
  "mcpServers": {
    "hand-authored": {
      "command": "old"
    }
  },
  "somethingElse": true
}
`
	baseDir := writeFixture(t, ".mcp.json", existing)

	gen, err := providers.LoadBuiltin("mcp")
	require.NoError(t, err)
	outputs, err := gen.Generate(&config.ContentTree{}, baseDir, oneMCPServerConfig(baseDir))
	require.NoError(t, err)

	mcpFile := requireFile(t, outputs, ".mcp.json")
	assert.Equal(t, "true", rawValue(t, mcpFile.Content, "somethingElse"))
	assert.Contains(t, mcpFile.Content, "generated")
}

// TestAmpSettingsSidecar_PreservesUnownedKeys covers the third JSON-object
// sidecar: .amp/settings.json carries arbitrary user Amp settings and ai-rulez
// owns only the effort key.
func TestAmpSettingsSidecar_PreservesUnownedKeys(t *testing.T) {
	t.Parallel()

	const existing = `{
  "amp.anthropic.effort": "low",
  "amp.mcpServers": {
    "local": {
      "command": "amp-mcp"
    }
  }
}
`
	relPath := filepath.Join(".amp", "settings.json")
	baseDir := writeFixture(t, relPath, existing)

	gen, err := providers.LoadBuiltin("amp")
	require.NoError(t, err)
	cfg := &config.Config{
		Name:     "test",
		BaseDir:  baseDir,
		Defaults: &config.DefaultsConfig{Effort: "high"},
	}
	outputs, err := gen.Generate(&config.ContentTree{}, baseDir, cfg)
	require.NoError(t, err)

	ampSettings := requireFile(t, outputs, relPath)
	assert.Equal(t, `"high"`, rawValue(t, ampSettings.Content, "amp.anthropic.effort"))
	assert.Equal(t, rawValue(t, existing, "amp.mcpServers"), rawValue(t, ampSettings.Content, "amp.mcpServers"),
		"unowned Amp settings must survive byte-for-byte")
}

// TestJSONSidecar_PreservesFourSpaceIndent checks the merge adapts to the
// existing file's top-level indentation instead of forcing two spaces onto a
// document whose nested values are indented differently.
func TestJSONSidecar_PreservesFourSpaceIndent(t *testing.T) {
	t.Parallel()

	const existing = "{\n    \"model\": \"opus\",\n    \"env\": {\n        \"A\": \"b\"\n    }\n}\n"
	relPath := filepath.Join(".claude", "settings.json")
	baseDir := writeFixture(t, relPath, existing)

	gen := claudeGen(t)
	outputs, err := gen.Generate(&config.ContentTree{}, baseDir, oneMCPServerConfig(baseDir))
	require.NoError(t, err)

	settings := requireFile(t, outputs, relPath)
	assert.Contains(t, settings.Content, "\n    \"model\": \"opus\",")
	assert.Contains(t, settings.Content, "\n    \"mcpServers\": {\n        \"generated\"")
}

func pluginSettingsConfig(baseDir string, settings *config.ClaudeSettings) *config.Config {
	return &config.Config{
		Name:        "test",
		BaseDir:     baseDir,
		Marketplace: &config.MarketplaceAuthoring{Name: "mk", OutputDir: "tools/mkt"},
		Claude:      &config.ClaudeConfig{Settings: settings},
	}
}

func TestClaudeSettingsSidecar_PluginKeys(t *testing.T) {
	t.Parallel()

	const existing = `{
  "permissions": {"allow": ["Read"]},
  "extraKnownMarketplaces": {
    "theirs": {"source": {"source": "github", "repo": "o/r"}}
  },
  "enabledPlugins": {
    "theirs-plugin@theirs": true,
    "demo-a@mk": false
  }
}
`
	relPath := filepath.Join(".claude", "settings.json")
	tests := []struct {
		name     string
		settings *config.ClaudeSettings
		assertFn func(t *testing.T, doc string)
	}{
		{
			name:     "enable and disable are owned entry by entry",
			settings: &config.ClaudeSettings{Manage: true, EnablePlugins: []string{"demo-b"}, DisablePlugins: []string{"demo-a"}},
			assertFn: func(t *testing.T, doc string) {
				var got struct {
					EnabledPlugins map[string]bool           `json:"enabledPlugins"`
					Marketplaces   map[string]map[string]any `json:"extraKnownMarketplaces"`
				}
				require.NoError(t, json.Unmarshal([]byte(doc), &got))
				assert.Equal(t, map[string]bool{"theirs-plugin@theirs": true, "demo-a@mk": false, "demo-b@mk": true}, got.EnabledPlugins)
				assert.Contains(t, got.Marketplaces, "theirs")
				assert.Equal(t, map[string]any{"source": "directory", "path": "./tools/mkt"}, got.Marketplaces["mk"]["source"])
			},
		},
		{
			name: "explicit source and auto_update",
			settings: &config.ClaudeSettings{
				Manage: true, AutoUpdate: boolRef(true),
				MarketplaceSource: &config.MarketplaceSource{Source: "github", Repo: "org/market", Ref: "main"},
			},
			assertFn: func(t *testing.T, doc string) {
				var got struct {
					Marketplaces map[string]map[string]any `json:"extraKnownMarketplaces"`
				}
				require.NoError(t, json.Unmarshal([]byte(doc), &got))
				assert.Equal(t, map[string]any{"source": "github", "repo": "org/market", "ref": "main"}, got.Marketplaces["mk"]["source"])
				assert.Equal(t, true, got.Marketplaces["mk"]["autoUpdate"])
			},
		},
		{
			name:     "register_marketplace false leaves that key alone",
			settings: &config.ClaudeSettings{Manage: true, RegisterMarketplace: boolRef(false), EnablePlugins: []string{"demo-b"}},
			assertFn: func(t *testing.T, doc string) {
				assert.NotContains(t, doc, `"mk": {`)
				assert.Contains(t, doc, `"demo-b@mk": true`)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			baseDir := writeFixture(t, relPath, existing)

			// Act
			outputs, err := claudeGen(t).Generate(&config.ContentTree{}, baseDir, pluginSettingsConfig(baseDir, tt.settings))

			// Assert
			require.NoError(t, err)
			settings := requireFile(t, outputs, relPath)
			assert.True(t, settings.PartiallyOwned, "the file carries keys ai-rulez does not own")
			assert.JSONEq(t, `{"allow": ["Read"]}`, rawValue(t, settings.Content, "permissions"))
			assert.NotContains(t, settings.Content, "mcpServers", "no MCP servers means no mcpServers key")
			tt.assertFn(t, settings.Content)
		})
	}
}

func TestClaudeSettingsSidecar_NotEmittedUnlessManaged(t *testing.T) {
	t.Parallel()

	// Arrange: a marketplace and plugin lists, but manage is off.
	baseDir := t.TempDir()
	cfg := pluginSettingsConfig(baseDir, &config.ClaudeSettings{EnablePlugins: []string{"demo-b"}})

	// Act
	outputs, err := claudeGen(t).Generate(&config.ContentTree{}, baseDir, cfg)

	// Assert
	require.NoError(t, err)
	assert.False(t, hasOutputPathSuffix(outputs, filepath.Join(".claude", "settings.json")))
}

func boolRef(b bool) *bool { return &b }
