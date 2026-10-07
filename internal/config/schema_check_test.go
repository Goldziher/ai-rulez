package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func schemaProject(t *testing.T, main, local string) *Config {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(main), 0o600))
	if local != "" {
		require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.local.toml"), []byte(local), 0o600))
	}
	cfg, err := LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	return cfg
}

func TestSchemaFindings(t *testing.T) {
	tests := []struct {
		name  string
		main  string
		local string
		want  []string // SchemaFinding.String() with the file reduced to its base name
	}{
		{name: "valid config has no finding", main: "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"},
		{
			name: "misspelled top-level keys suggest the nearest key",
			main: "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\ndelivry = \"static\"\n",
			want: []string{"config.toml: unknown key \"delivry\""},
		},
		{
			name: "nested table and array entry",
			main: "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n[lock]\nenforc = true\n[[mcp_servers]]\nname = \"a\"\ncommand = \"x\"\ncomand = \"y\"\n",
			want: []string{"config.toml: unknown key \"lock.enforc\" (did you mean \"enforce\"?)", "config.toml: unknown key \"mcp_servers.0.comand\" (did you mean \"command\"?)"},
		},
		{
			name: "the removed compression option loads and is reported, not fatal",
			main: "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\ncompression = \"moderate\"\n",
			want: []string{"config.toml: unknown key \"compression\""},
		},
		{
			name: "a top-level key after a table header is told where it landed",
			main: "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n[header]\nhashes = \"none\"\nbuiltins = [\"rust\"]\n",
			want: []string{"config.toml: unknown key \"header.builtins\" (builtins is a top-level key: a key after a [header] line belongs to that table, so move it above the first [table])"},
		},
		{
			name:  "local overlay is included",
			main:  "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n",
			local: "[lock]\nenforc = true\n",
			want:  []string{"config.local.toml: unknown key \"lock.enforc\" (did you mean \"enforce\"?)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := schemaProject(t, tt.main, tt.local)

			// Act
			findings, err := SchemaFindings(cfg)

			// Assert
			require.NoError(t, err)
			var got []string
			for _, f := range findings {
				f.File = filepath.Base(f.File)
				got = append(got, f.String())
			}
			assert.Subset(t, got, tt.want)
			if len(tt.want) == 0 {
				assert.Empty(t, got)
			}
		})
	}
}

func TestNearestKey(t *testing.T) {
	keys := []string{"delivery", "skill_mode", "enforce"}
	tests := []struct{ key, want string }{
		{"delivry", "delivery"},
		{"skil_mode", "skill_mode"},
		{"zzzzzz", ""},
		{"x", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, nearestKey(tt.key, keys), tt.key)
	}
}
