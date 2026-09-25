package presets

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The legacy preset generators below each emit a settings-style JSON document
// that the consumer also owns keys in. They used to render it from a fresh Go map
// on every run, destroying everything ai-rulez does not own (#185). These tests
// pin the merge: the hand-authored keys survive byte-for-byte, the owned
// mcpServers key is still written, and the emitted OutputFile is flagged
// PartiallyOwned so the pipeline neither gitignores nor deletes the file.

// handAuthoredMCPDocument is a settings/MCP document with a hand-authored sibling
// key and a stale mcpServers entry. `canaryKey` is the thing a clobbering
// generator silently deleted.
const handAuthoredMCPDocument = `{
  "canaryKey": {
    "nested": [
      1,
      2
    ]
  },
  "mcpServers": {
    "stale-hand-authored": {
      "command": "this-entry-is-replaced"
    }
  },
  "trailingUserKey": "kept"
}
`

// mergeFixtureConfig is a config with exactly one MCP server, which is what makes
// every one of these presets emit its JSON document.
func mergeFixtureConfig() *config.Config {
	return &config.Config{
		Name: "test-project",
		MCPServers: map[string]*config.MCPServer{
			"configured": {Command: "npx", Args: []string{"-y", "configured-server"}},
		},
	}
}

// writeDocumentFixture materializes an existing on-disk document under a fresh
// base dir and returns that base dir, so a test reads as "given this repo state".
func writeDocumentFixture(t *testing.T, relPath, content string) string {
	t.Helper()
	baseDir := t.TempDir()
	absPath := filepath.Join(baseDir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0o750))
	require.NoError(t, os.WriteFile(absPath, []byte(content), 0o600))
	return baseDir
}

func requireOutput(t *testing.T, outputs []config.OutputFile, absPath string) config.OutputFile {
	t.Helper()
	for _, output := range outputs {
		if output.Path == absPath {
			return output
		}
	}
	t.Fatalf("output %q not generated", absPath)
	return config.OutputFile{}
}

func findOutput(outputs []config.OutputFile, absPath string) (config.OutputFile, bool) {
	for _, output := range outputs {
		if output.Path == absPath {
			return output, true
		}
	}
	return config.OutputFile{}, false
}

// rawJSONValue returns a top-level member's value as its exact source bytes, so a
// test can assert a key survived byte-for-byte rather than merely round-tripped.
func rawJSONValue(t *testing.T, doc, key string) string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader([]byte(doc)))
	open, err := decoder.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), open)

	for decoder.More() {
		keyToken, err := decoder.Token()
		require.NoError(t, err)
		name, ok := keyToken.(string)
		require.True(t, ok, "object key must be a string")
		var raw json.RawMessage
		require.NoError(t, decoder.Decode(&raw))
		if name == key {
			return string(raw)
		}
	}
	t.Fatalf("key %q missing from document:\n%s", key, doc)
	return ""
}

// mcpServerEntries parses the owned mcpServers key of a rendered document.
func mcpServerEntries(t *testing.T, doc string) map[string]map[string]any {
	t.Helper()
	var parsed struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(doc), &parsed))
	return parsed.MCPServers
}

// assertPreservedHandAuthoredKeys is the shared body of the four regression
// tests: the sibling keys are byte-identical, the stale owned entry is gone, and
// the file is reported as shared with the consumer.
func assertPreservedHandAuthoredKeys(t *testing.T, output config.OutputFile) map[string]map[string]any {
	t.Helper()

	assert.Equal(t,
		rawJSONValue(t, handAuthoredMCPDocument, "canaryKey"),
		rawJSONValue(t, output.Content, "canaryKey"),
		"hand-authored key must survive byte-for-byte")
	assert.Equal(t, `"kept"`, rawJSONValue(t, output.Content, "trailingUserKey"))
	assert.True(t, output.PartiallyOwned,
		"a document holding hand-authored keys must not be gitignored or deleted as stale")

	servers := mcpServerEntries(t, output.Content)
	assert.NotContains(t, servers, "stale-hand-authored", "ai-rulez owns the mcpServers key outright")
	return servers
}

func TestGeminiPreset_SettingsJSON_PreservesHandAuthoredKeys(t *testing.T) {
	relPath := filepath.Join(".gemini", "settings.json")
	baseDir := writeDocumentFixture(t, relPath, handAuthoredMCPDocument)

	g := &GeminiPresetGenerator{}
	outputs, err := g.Generate(&config.ContentTree{}, baseDir, mergeFixtureConfig())
	require.NoError(t, err)

	settings := requireOutput(t, outputs, filepath.Join(baseDir, relPath))
	servers := assertPreservedHandAuthoredKeys(t, settings)

	assert.Equal(t, "npx", servers["configured"]["command"])
	assert.Equal(t, []any{"-y", "configured-server"}, servers["configured"]["args"])
	// Gemini injects its own self-registration entry; that behavior is unchanged.
	assert.Equal(t, "npx", servers["ai-rulez"]["command"])
	assert.Equal(t, []any{"-y", "ai-rulez@latest", "mcp"}, servers["ai-rulez"]["args"])
}

// TestGeminiPreset_SettingsJSON_SkippedWithoutMCPServers pins the other half of
// the #185 fix: the settings document is no longer written on every run. With no
// configured servers there is nothing to contribute but the self-registration
// entry, which is not worth creating (or rewriting the owned key of) a settings
// file for.
func TestGeminiPreset_SettingsJSON_SkippedWithoutMCPServers(t *testing.T) {
	relPath := filepath.Join(".gemini", "settings.json")
	baseDir := writeDocumentFixture(t, relPath, handAuthoredMCPDocument)

	g := &GeminiPresetGenerator{}
	outputs, err := g.Generate(&config.ContentTree{}, baseDir, &config.Config{Name: "test-project"})
	require.NoError(t, err)

	_, found := findOutput(outputs, filepath.Join(baseDir, relPath))
	assert.False(t, found, "no MCP servers configured, so no settings.json output")

	onDisk, readErr := os.ReadFile(filepath.Join(baseDir, relPath))
	require.NoError(t, readErr)
	assert.Equal(t, handAuthoredMCPDocument, string(onDisk), "the existing document is left untouched")
}

func TestAntigravityPreset_SettingsJSON_PreservesHandAuthoredKeys(t *testing.T) {
	relPath := filepath.Join(".agents", "settings.json")
	baseDir := writeDocumentFixture(t, relPath, handAuthoredMCPDocument)

	g := &AntigravityPresetGenerator{}
	outputs, err := g.Generate(&config.ContentTree{}, baseDir, mergeFixtureConfig())
	require.NoError(t, err)

	settings := requireOutput(t, outputs, filepath.Join(baseDir, relPath))
	servers := assertPreservedHandAuthoredKeys(t, settings)

	assert.Equal(t, "npx", servers["configured"]["command"])
	assert.Equal(t, []any{"-y", "configured-server"}, servers["configured"]["args"])
	assert.Equal(t, "npx", servers["ai-rulez"]["command"])
	assert.Equal(t, []any{"-y", "ai-rulez@latest", "mcp"}, servers["ai-rulez"]["args"])
}

func TestAntigravityPreset_SettingsJSON_SkippedWithoutMCPServers(t *testing.T) {
	relPath := filepath.Join(".agents", "settings.json")
	baseDir := writeDocumentFixture(t, relPath, handAuthoredMCPDocument)

	g := &AntigravityPresetGenerator{}
	outputs, err := g.Generate(&config.ContentTree{}, baseDir, &config.Config{Name: "test-project"})
	require.NoError(t, err)

	_, found := findOutput(outputs, filepath.Join(baseDir, relPath))
	assert.False(t, found, "no MCP servers configured, so no settings.json output")

	onDisk, readErr := os.ReadFile(filepath.Join(baseDir, relPath))
	require.NoError(t, readErr)
	assert.Equal(t, handAuthoredMCPDocument, string(onDisk), "the existing document is left untouched")
}

func TestCursorPreset_MCPJSON_PreservesHandAuthoredKeys(t *testing.T) {
	baseDir := writeDocumentFixture(t, ".mcp.json", handAuthoredMCPDocument)

	g := &CursorPresetGenerator{}
	outputs, err := g.Generate(&config.ContentTree{}, baseDir, mergeFixtureConfig())
	require.NoError(t, err)

	mcpFile := requireOutput(t, outputs, filepath.Join(baseDir, ".mcp.json"))
	servers := assertPreservedHandAuthoredKeys(t, mcpFile)

	assert.Equal(t, "npx", servers["configured"]["command"])
	assert.Equal(t, []any{"-y", "configured-server"}, servers["configured"]["args"])
	assert.Equal(t, false, servers["configured"]["disabled"])
	assert.NotContains(t, servers, "ai-rulez", "cursor does not self-register")
}

func TestCopilotPreset_MCPJSON_PreservesHandAuthoredKeys(t *testing.T) {
	baseDir := writeDocumentFixture(t, ".mcp.json", handAuthoredMCPDocument)

	g := &CopilotPresetGenerator{}
	outputs, err := g.Generate(&config.ContentTree{}, baseDir, mergeFixtureConfig())
	require.NoError(t, err)

	mcpFile := requireOutput(t, outputs, filepath.Join(baseDir, ".mcp.json"))
	servers := assertPreservedHandAuthoredKeys(t, mcpFile)

	assert.Equal(t, "npx", servers["configured"]["command"])
	assert.Equal(t, []any{"-y", "configured-server"}, servers["configured"]["args"])
	assert.Equal(t, false, servers["configured"]["disabled"])
	assert.NotContains(t, servers, "ai-rulez", "copilot does not self-register")
}

// TestLegacyPresets_JSONDocument_GreenfieldIsWhollyOwned pins the flag for the
// first-run case across all four presets: a document ai-rulez created holds
// nothing but the owned key, so it stays gitignorable and deletable — which is
// what keeps resolved MCP secrets out of git.
func TestLegacyPresets_JSONDocument_GreenfieldIsWhollyOwned(t *testing.T) {
	for name, testCase := range map[string]struct {
		generator config.PresetGenerator
		relPath   string
	}{
		"gemini":      {generator: &GeminiPresetGenerator{}, relPath: filepath.Join(".gemini", "settings.json")},
		"antigravity": {generator: &AntigravityPresetGenerator{}, relPath: filepath.Join(".agents", "settings.json")},
		"cursor":      {generator: &CursorPresetGenerator{}, relPath: ".mcp.json"},
		"copilot":     {generator: &CopilotPresetGenerator{}, relPath: ".mcp.json"},
	} {
		t.Run(name, func(t *testing.T) {
			baseDir := t.TempDir()
			outputs, err := testCase.generator.Generate(&config.ContentTree{}, baseDir, mergeFixtureConfig())
			require.NoError(t, err)

			output := requireOutput(t, outputs, filepath.Join(baseDir, testCase.relPath))
			assert.False(t, output.PartiallyOwned, "a freshly created document is wholly generated")
			assert.Equal(t, []string{"mcpServers"}, topLevelKeys(t, output.Content))
		})
	}
}

// topLevelKeys returns the top-level keys of a JSON object in source order.
func topLevelKeys(t *testing.T, doc string) []string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader([]byte(doc)))
	open, err := decoder.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), open)

	keys := make([]string, 0, 4)
	for decoder.More() {
		keyToken, err := decoder.Token()
		require.NoError(t, err)
		key, ok := keyToken.(string)
		require.True(t, ok, "object key must be a string")
		var raw json.RawMessage
		require.NoError(t, decoder.Decode(&raw))
		keys = append(keys, key)
	}
	return keys
}
