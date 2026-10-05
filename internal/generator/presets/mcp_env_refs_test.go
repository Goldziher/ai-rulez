package presets

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
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
