package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const serveSettingsToml = `version = "5.0"
name = "proj"
presets = ["claude"]

[mcp]
serve_skills = true
read_only = true
daemon = true
no_watch = true
serve_user_invoked_skills = true
allow = ["review-*"]
deny = ["draft-*"]
tools = ["find_skill", "load_skill"]
`

func writeServeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestMCPServeSettings_LoadFromEachLayout(t *testing.T) {
	tests := []struct {
		name string
		rel  string
	}{
		{name: "config directory", rel: ".ai-rulez/config.toml"},
		{name: "root ai-rulez.toml", rel: "ai-rulez.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeServeFile(t, dir, tt.rel, serveSettingsToml)

			cfg, err := LoadConfigFromFile(context.Background(), path)

			require.NoError(t, err)
			require.NotNil(t, cfg.MCP)
			require.NotNil(t, cfg.MCP.ServeSkills)
			assert.True(t, *cfg.MCP.ServeSkills)
			require.NotNil(t, cfg.MCP.ReadOnly)
			assert.True(t, *cfg.MCP.ReadOnly)
			require.NotNil(t, cfg.MCP.Daemon)
			assert.True(t, *cfg.MCP.Daemon)
			require.NotNil(t, cfg.MCP.NoWatch)
			assert.True(t, *cfg.MCP.NoWatch)
			require.NotNil(t, cfg.MCP.ServeUserInvokedSkills)
			assert.True(t, *cfg.MCP.ServeUserInvokedSkills)
			assert.Equal(t, []string{"review-*"}, cfg.MCP.Allow)
			assert.Equal(t, []string{"draft-*"}, cfg.MCP.Deny)
			assert.Equal(t, []string{"find_skill", "load_skill"}, cfg.MCP.Tools)
			findings, err := SchemaFindings(cfg)
			require.NoError(t, err)
			assert.Empty(t, findings, "the keys are in the schema")
		})
	}
}

func TestMCPServeSettings_ALocalOverlayOverridesPerKey(t *testing.T) {
	dir := t.TempDir()
	writeServeFile(t, dir, ".ai-rulez/config.toml", serveSettingsToml)
	path := writeServeFile(t, dir, ".ai-rulez/config.local.toml", "[mcp]\nread_only = false\ntools = [\"find_skill\"]\n")
	require.NotEmpty(t, path)

	cfg, err := LoadConfigFromFile(context.Background(), filepath.Join(dir, ".ai-rulez", "config.toml"))

	require.NoError(t, err)
	require.NotNil(t, cfg.MCP.ReadOnly)
	assert.False(t, *cfg.MCP.ReadOnly, "the local overlay wins")
	assert.Equal(t, []string{"find_skill"}, cfg.MCP.Tools, "a list is replaced, not appended to")
	require.NotNil(t, cfg.MCP.Daemon)
	assert.True(t, *cfg.MCP.Daemon, "keys the overlay does not set stay")
	findings, err := SchemaFindings(cfg)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestMCPServeSettings_SurviveARewrite(t *testing.T) {
	dir := t.TempDir()
	path := writeServeFile(t, dir, ".ai-rulez/config.toml", serveSettingsToml)
	cfg, err := LoadConfigFromFile(context.Background(), path)
	require.NoError(t, err)

	out, err := MarshalTOML(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o600))
	again, err := LoadConfigFromFile(context.Background(), path)

	require.NoError(t, err)
	assert.Equal(t, cfg.MCP, again.MCP)
}

func TestMCPServeSettings_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mcp     MCPConfig
		wantErr string
	}{
		{name: "empty", mcp: MCPConfig{}},
		{name: "values", mcp: MCPConfig{Allow: []string{"a-*"}, Deny: []string{"b"}, Tools: []string{"find_skill"}}},
		{name: "blank allow entry", mcp: MCPConfig{Allow: []string{" "}}, wantErr: "mcp.allow"},
		{name: "blank deny entry", mcp: MCPConfig{Deny: []string{""}}, wantErr: "mcp.deny"},
		{name: "bad glob", mcp: MCPConfig{Allow: []string{"[a"}}, wantErr: "mcp.allow"},
		{name: "blank tool", mcp: MCPConfig{Tools: []string{""}}, wantErr: "mcp.tools"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Version: "5.0", Name: "test", Presets: []Preset{{BuiltIn: "claude"}}, MCP: &tt.mcp}

			err := cfg.Validate()

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
