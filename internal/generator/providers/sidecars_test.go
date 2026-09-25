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

	// Only mcpServers changed: the generated server replaces the stale entry.
	assert.Equal(t, "npx", parsed.MCPServers["generated"].Command)
	assert.Equal(t, []string{"-y", "generated-server"}, parsed.MCPServers["generated"].Args)
	assert.NotContains(t, parsed.MCPServers, "stale-hand-authored",
		"ai-rulez owns the mcpServers key outright")
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
