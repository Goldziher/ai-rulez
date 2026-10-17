package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mcpExcludeBody = "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n[mcp]\nexclude = [\"vendor/**\", \"**/fixtures/\"]\n"

func TestMCPExclude_LoadsFromEveryLayout(t *testing.T) {
	tests := []struct {
		name string
		rel  string
	}{
		{name: "the .ai-rulez directory", rel: ".ai-rulez/config.toml"},
		{name: "the root ai-rulez.toml", rel: "ai-rulez.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeConfigFile(t, filepath.Join(root, filepath.FromSlash(tt.rel)), mcpExcludeBody)

			// Act
			cfg, err := LoadConfig(context.Background(), root)
			require.NoError(t, err)
			findings, ferr := SchemaFindings(cfg)

			// Assert
			require.NotNil(t, cfg.MCP)
			assert.Equal(t, []string{"vendor/**", "**/fixtures/"}, cfg.MCP.Exclude)
			require.NoError(t, ferr)
			assert.Empty(t, findings, "exclude is a known key")
		})
	}
}

func TestMCPExclude_SurvivesARewrite(t *testing.T) {
	cfg := &Config{Version: "5.0", Name: "t", Presets: []Preset{{BuiltIn: "claude"}}, MCP: &MCPConfig{Exclude: []string{"vendor/**"}}}

	out, err := MarshalTOML(cfg)

	require.NoError(t, err)
	assert.Contains(t, string(out), "exclude = ['vendor/**']")
}

func TestValidate_MCPExclude(t *testing.T) {
	tests := []struct {
		name    string
		exclude []string
		wantErr string
	}{
		{name: "globs are accepted", exclude: []string{"vendor/**", "*.md", "a/{b,c}/"}},
		{name: "an empty entry is rejected", exclude: []string{"vendor/**", " "}, wantErr: "mcp.exclude"},
		{name: "a glob that expands past the limit is rejected", exclude: []string{"{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}"}, wantErr: "mcp.exclude"},
		{name: "a glob that climbs out of the project is rejected", exclude: []string{"../outside/**"}, wantErr: "mcp.exclude"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &Config{Version: "5.0", Name: "t", Presets: []Preset{{BuiltIn: "claude"}}, MCP: &MCPConfig{Exclude: tt.exclude}}

			// Act
			err := cfg.Validate()

			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
