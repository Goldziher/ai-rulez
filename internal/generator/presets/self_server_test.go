package presets

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// generateFunc is the shape every preset's Generate method has.
type generateFunc func(*config.ContentTree, string, *config.Config) ([]config.OutputFile, error)

// entryCommand flattens an MCP entry into the command line it launches, whatever
// shape the harness uses: a shell string (Xum), command plus args (Cursor,
// Devin), or one command array (OpenCode).
func entryCommand(entry any) []string {
	switch typed := entry.(type) {
	case string:
		return strings.Fields(typed)
	case map[string]any:
		if args, ok := typed["command"].([]any); ok {
			return stringSlice(args)
		}
		command, _ := typed["command"].(string)
		if command == "" {
			return nil
		}
		args, _ := typed["args"].([]any)
		return append([]string{command}, stringSlice(args)...)
	}
	return nil
}

func stringSlice(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		text, _ := value.(string)
		out = append(out, text)
	}
	return out
}

// TestPresets_RenderTheSelfServer is the parity guard: every harness ai-rulez
// writes project MCP entries for must carry ai-rulez's own server ([mcp]
// self_server, on by default), so the project is MCP-ready without a
// hand-written user-level entry that outlives the binary it names.
func TestPresets_RenderTheSelfServer(t *testing.T) {
	tests := []struct {
		name      string
		generate  generateFunc
		rel       string
		container string
	}{
		{"opencode", (&OpencodePresetGenerator{}).Generate, "opencode.json", "mcp"},
		{"cursor", (&CursorPresetGenerator{}).Generate, ".cursor/mcp.json", "mcpServers"},
		{"devin", (&DevinPresetGenerator{}).Generate, ".devin/mcp_config.json", "mcpServers"},
		{"xum", (&XumPresetGenerator{}).Generate, ".xum/mcp.jsonc", "servers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			baseDir := t.TempDir()
			cfg := &config.Config{Name: "test"}

			// Act
			outputs, err := tt.generate(&config.ContentTree{}, baseDir, cfg)

			// Assert
			require.NoError(t, err)
			output, ok := findOutput(outputs, filepath.Join(baseDir, filepath.FromSlash(tt.rel)))
			require.True(t, ok, "%s not generated", tt.rel)
			var doc map[string]any
			require.NoError(t, json.Unmarshal([]byte(output.Content), &doc))
			container, ok := doc[tt.container].(map[string]any)
			require.True(t, ok, "%s has no %s object:\n%s", tt.rel, tt.container, output.Content)
			assert.Equal(t, []string{"npx", "-y", "ai-rulez@latest", "mcp"}, entryCommand(container[config.SelfMCPServerName]))
		})
	}
}

// TestOpencodePresetGenerator_SelfServerOffLeavesMCPAlone pins the opt-out: with
// self_server = false and no declared servers the document carries no mcp key.
func TestOpencodePresetGenerator_SelfServerOffLeavesMCPAlone(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	cfg := &config.Config{Name: "test", Run: &config.RunState{Scope: &config.ScopeRun{}}}
	cfg.SetSelfServer(false)

	// Act
	outputs, err := (&OpencodePresetGenerator{}).Generate(&config.ContentTree{}, baseDir, cfg)

	// Assert
	require.NoError(t, err)
	assertNoMCPKey(t, outputs, baseDir)
}

// TestOpencodePresetGenerator_SelfServerCommandReplacesTheLaunch pins
// mcp.self_server_command: the project uses its installed binary, not npx.
func TestOpencodePresetGenerator_SelfServerCommandReplacesTheLaunch(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	cfg := &config.Config{
		Name: "test",
		MCP:  &config.MCPConfig{SelfServerCommand: []string{"ai-rulez", "mcp", "--serve-skills"}},
	}

	// Act
	outputs, err := (&OpencodePresetGenerator{}).Generate(&config.ContentTree{}, baseDir, cfg)

	// Assert
	require.NoError(t, err)
	output, ok := findOutput(outputs, filepath.Join(baseDir, "opencode.json"))
	require.True(t, ok, "opencode.json not generated")
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(output.Content), &doc))
	mcp, ok := doc["mcp"].(map[string]any)
	require.True(t, ok, "opencode.json has no mcp object:\n%s", output.Content)
	assert.Equal(t, []string{"ai-rulez", "mcp", "--serve-skills"}, entryCommand(mcp[config.SelfMCPServerName]))
}

func assertNoMCPKey(t *testing.T, outputs []config.OutputFile, baseDir string) {
	t.Helper()
	output, ok := findOutput(outputs, filepath.Join(baseDir, "opencode.json"))
	if !ok {
		return
	}
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(output.Content), &doc))
	if _, present := doc["mcp"]; present {
		t.Fatalf("opencode.json owns mcp while self_server = false:\n%s", output.Content)
	}
}

// TestCodexPresetGenerator_RendersTheSelfServer is the TOML counterpart of the
// parity guard above.
func TestCodexPresetGenerator_RendersTheSelfServer(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	cfg := &config.Config{Name: "test", Presets: []config.Preset{{BuiltIn: "codex"}}}

	// Act
	outputs, err := (&CodexPresetGenerator{}).Generate(&config.ContentTree{}, baseDir, cfg)

	// Assert
	require.NoError(t, err)
	output, ok := findOutput(outputs, filepath.Join(baseDir, ".codex", "config.toml"))
	require.True(t, ok, ".codex/config.toml not generated")
	assert.Contains(t, output.Content, "[mcp_servers.ai-rulez]")
	assert.Contains(t, output.Content, `command = "npx"`)
	assert.Contains(t, output.Content, `"ai-rulez@latest"`)
}

// TestSelfMCPServer_DeclaredServerWins keeps the author's server under the name
// ai-rulez when one is declared, rather than writing a second one over it.
func TestSelfMCPServer_DeclaredServerWins(t *testing.T) {
	// Arrange
	declared := map[string]*config.MCPServer{
		config.SelfMCPServerName: {Name: config.SelfMCPServerName, Command: "custom-server"},
	}

	// Act
	got := selfMCPServer(&config.Config{MCPServers: declared})

	// Assert
	assert.Nil(t, got)
}

// TestSelfMCPServer_MatchesTheMCPServerShape pins the conversion the parity guard
// relies on: the .mcp.json entry (command plus args) becomes one config server.
func TestSelfMCPServer_MatchesTheMCPServerShape(t *testing.T) {
	// Arrange
	cfg := &config.Config{MCP: &config.MCPConfig{SelfServerCommand: []string{"ai-rulez", "mcp"}}}

	// Act
	got := selfMCPServer(cfg)

	// Assert
	require.NotNil(t, got)
	assert.Equal(t, config.SelfMCPServerName, got.Name)
	assert.Equal(t, "ai-rulez", got.Command)
	assert.Equal(t, []string{"mcp"}, got.Args)
}
