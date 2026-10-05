package presets

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCursorMCPEntry_WritesEnvReferences(t *testing.T) {
	tests := []struct {
		name   string
		server *config.MCPServer
		want   map[string]any
	}{
		{"header and env placeholders become ${env:}", &config.MCPServer{
			Transport: "http", URL: "https://x/mcp",
			Headers:    map[string]string{"Authorization": "Bearer s3cret", "X-Static": "1"},
			HeaderRefs: map[string]string{"Authorization": "Bearer ${TOK}"},
		}, map[string]any{"url": "https://x/mcp", "headers": map[string]string{
			"Authorization": "Bearer ${env:TOK}", "X-Static": "1",
		}}},
		{"stdio env", &config.MCPServer{
			Command: "npx", Env: map[string]string{"K": "abc"}, EnvRefs: map[string]string{"K": "${KEY}"},
		}, map[string]any{"command": "npx", "env": map[string]string{"K": "${env:KEY}"}}},
		{"no references keeps resolved values", &config.MCPServer{
			Command: "npx", Env: map[string]string{"K": "abc"},
		}, map[string]any{"command": "npx", "env": map[string]string{"K": "abc"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := cursorMCPEntry(tt.server)

			// Assert
			assert.Equal(t, tt.want, got)
			if len(tt.server.Env) > 0 {
				assert.NotContains(t, tt.server.Env["K"], "${env:", "the server's own map must not be rewritten")
			}
		})
	}
}

func TestResolveNativeAgentModel(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]string
		cfg   *config.Config
		want  string
	}{
		{"bare alias dropped", map[string]string{"model": "Sonnet"}, &config.Config{}, ""},
		{"inherit dropped", map[string]string{"model": "inherit"}, &config.Config{}, ""},
		{"native id kept", map[string]string{"model": "anthropic/claude-sonnet-4"}, &config.Config{}, "anthropic/claude-sonnet-4"},
		{"per-preset model wins", map[string]string{"model": "opus", "cline_model": "gpt-5"}, &config.Config{}, "gpt-5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := config.ContentFile{Name: "a", Metadata: &config.Metadata{Extra: tt.extra}}
			assert.Equal(t, tt.want, ResolveNativeAgentModel("cline", agent, tt.cfg))
		})
	}
}

func envRefConfig() *config.Config {
	return &config.Config{MCPServers: map[string]*config.MCPServer{
		"local": {
			Command: "npx", Env: map[string]string{"TOK": "s3cret-env"}, EnvRefs: map[string]string{"TOK": "${TOK}"},
		},
		"remote": {
			Transport: "http", URL: "https://x/mcp",
			Headers:    map[string]string{"Authorization": "Bearer s3cret-hdr"},
			HeaderRefs: map[string]string{"Authorization": "Bearer ${HDR}"},
		},
	}}
}

func TestMCPEnvReferences_PerTool(t *testing.T) {
	tests := []struct {
		name       string
		build      func(*config.Config) map[string]any
		wantEnv    string
		wantHeader string
	}{
		{"gemini ${VAR}", func(c *config.Config) map[string]any { return (&GeminiPresetGenerator{}).mcpServersValue(c) },
			"${TOK}", "Bearer ${HDR}"},
		{"opencode {env:VAR}", func(c *config.Config) map[string]any { return (&OpencodePresetGenerator{}).mcpServersValue(c) },
			"{env:TOK}", "Bearer {env:HDR}"},
		{"devin ${env:VAR}", func(c *config.Config) map[string]any { return mcpEntries(c, devinMCPEntry) },
			"${env:TOK}", "Bearer ${env:HDR}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := envRefConfig()

			// Act
			servers := tt.build(cfg)

			// Assert
			local, _ := servers["local"].(map[string]any)
			remote, _ := servers["remote"].(map[string]any)
			envKey := "env"
			if _, ok := local["environment"]; ok {
				envKey = "environment"
			}
			assert.Equal(t, tt.wantEnv, local[envKey].(map[string]string)["TOK"])
			assert.Equal(t, tt.wantHeader, remote["headers"].(map[string]string)["Authorization"])
			assert.Equal(t, "s3cret-env", cfg.MCPServers["local"].Env["TOK"], "the server's own map is not rewritten")
		})
	}
}

func TestRenderSharedMCPJSON_WritersAgree(t *testing.T) {
	// Arrange
	cfg := envRefConfig()

	// Act: cursor and copilot render the same root document as the mcp preset.
	cursor, err := (&CursorPresetGenerator{}).renderMCPJSON("", cfg)
	require.NoError(t, err)
	copilot, err := (&CopilotPresetGenerator{}).renderMCPJSON("", cfg)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, cursor.Body, copilot.Body)
	assert.Contains(t, cursor.Body, "${TOK}")
	assert.NotContains(t, cursor.Body, "s3cret-env")

	// Arrange: a writer whose tool does not expand references.
	cfg.Presets = []config.Preset{{BuiltIn: "qoder"}}

	// Act
	resolved, err := (&CursorPresetGenerator{}).renderMCPJSON("", cfg)
	require.NoError(t, err)

	// Assert
	assert.Contains(t, resolved.Body, "s3cret-env")
	assert.NotContains(t, resolved.Body, "${TOK}")
}
