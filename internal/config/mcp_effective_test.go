package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectiveMCPServers(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   []string
	}{
		{
			name:   "raw keeps authored order and drops the resolved values",
			config: "\n[[mcp_servers]]\nname = \"zeta\"\ncommand = \"z\"\n\n[[mcp_servers]]\nname = \"alpha\"\ncommand = \"a\"\n",
			want:   []string{"zeta", "alpha"},
		},
		{name: "none", want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			dir := filepath.Join(root, ".ai-rulez")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte("version = \"4.0\"\nname = \"p\"\npresets = [\"claude\"]\n"+tt.config), 0o644))
			cfg, err := LoadConfig(context.Background(), root)
			require.NoError(t, err)
			// A render resolves placeholders in the working copy.
			for _, s := range cfg.MCPServers {
				s.Env = map[string]string{"K": "resolved-secret"}
			}

			// Act
			got := cfg.EffectiveMCPServers()

			// Assert
			names := make([]string, 0, len(got))
			for _, s := range got {
				names = append(names, s.Name)
				assert.NotContains(t, s.Env, "K", "the as-written server must not carry a resolved value")
			}
			assert.Equal(t, tt.want, names)
		})
	}
}

func TestLoadConfig_IgnoresSeparateMCPFiles(t *testing.T) {
	for _, name := range []string{"mcp.toml", "mcp.yaml", "mcp.json"} {
		t.Run(name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			dir := filepath.Join(root, ".ai-rulez")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"),
				[]byte("version = \"4.0\"\nname = \"p\"\npresets = [\"claude\"]\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(`{"mcp_servers":[{"name":"legacy","command":"x"}]}`), 0o644))

			// Act
			cfg, err := LoadConfig(context.Background(), root)

			// Assert
			require.NoError(t, err)
			assert.Empty(t, cfg.MCPServers, "a separate MCP file is no longer read")
			assert.Empty(t, cfg.EffectiveMCPServers())
		})
	}
}
