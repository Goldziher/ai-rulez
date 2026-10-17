package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIndexCache(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: IndexCacheUser},
		{in: "user", want: IndexCacheUser},
		{in: " Repo ", want: IndexCacheRepo},
		{in: "OFF", want: IndexCacheOff},
		{in: "disk", wantErr: true},
		{in: "true", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseIndexCache(tt.in)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "index_cache")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidate_RejectsAnUnknownIndexCacheMode(t *testing.T) {
	cfg := &Config{
		Version: "5.0", Name: "test", Presets: []Preset{{BuiltIn: "claude"}},
		MCP: &MCPConfig{IndexCache: "somewhere"},
	}

	err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "index_cache")
}

func TestIndexCache_LoadsAndSurvivesARewrite(t *testing.T) {
	src := "version = \"5.0\"\nname = \"proj\"\npresets = [\"claude\"]\n\n[mcp]\nindex_cache = \"repo\"\n"
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
	path := filepath.Join(dir, ".ai-rulez", "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	cfg, err := LoadConfigFromFile(context.Background(), path)
	require.NoError(t, err)
	require.NotNil(t, cfg.MCP)
	assert.Equal(t, "repo", cfg.MCP.IndexCache)

	out, err := MarshalTOML(cfg)
	require.NoError(t, err)
	assert.Contains(t, string(out), "index_cache = 'repo'")
	require.NoError(t, os.WriteFile(path, out, 0o644))
	again, err := LoadConfigFromFile(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, "repo", again.MCP.IndexCache)
}

func TestIndexCacheMode_DefaultsToUser(t *testing.T) {
	var c *Config
	assert.Equal(t, IndexCacheUser, c.IndexCacheMode())
	assert.Equal(t, IndexCacheUser, (&Config{}).IndexCacheMode())
	assert.Equal(t, IndexCacheOff, (&Config{MCP: &MCPConfig{IndexCache: "off"}}).IndexCacheMode())
}
