package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

func TestCacheDirIn_HonorsTheUserCacheRoot(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home", "dev")
	xdg := filepath.Join(root, "cache", "dev")
	tests := []struct {
		name    string
		env     ambient.Env
		want    string
		wantErr string
	}{
		{
			name: "absolute XDG_CACHE_HOME wins over the home directory",
			env:  ambient.MapEnv{Home: home, Vars: map[string]string{"XDG_CACHE_HOME": xdg}},
			want: filepath.Join(xdg, "ai-rulez", "includes", "src-abc"),
		},
		{
			name: "absolute XDG_CACHE_HOME works without a home directory on a read-only home",
			env:  ambient.MapEnv{Vars: map[string]string{"XDG_CACHE_HOME": xdg}},
			want: filepath.Join(xdg, "ai-rulez", "includes", "src-abc"),
		},
		{
			name: "a relative XDG_CACHE_HOME is ignored",
			env:  ambient.MapEnv{Home: home, Vars: map[string]string{"XDG_CACHE_HOME": "cache"}},
			want: filepath.Join(home, ".cache", "ai-rulez", "includes", "src-abc"),
		},
		{
			name: "no XDG_CACHE_HOME falls back to the home directory",
			env:  ambient.MapEnv{Home: home},
			want: filepath.Join(home, ".cache", "ai-rulez", "includes", "src-abc"),
		},
		{
			name:    "neither XDG_CACHE_HOME nor a home directory is an error",
			env:     ambient.MapEnv{},
			wantErr: "no cache directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CacheDirIn(tt.env, "includes", "src-abc")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUserConfigDir_HonorsAnAbsoluteXDGConfigHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home", "dev")
	xdg := filepath.Join(root, "config", "dev")

	assert.Equal(t, filepath.Join(xdg, "ai-rulez"), UserConfigDir(func(name string) string {
		if name == "XDG_CONFIG_HOME" {
			return xdg
		}
		return ""
	}, home))
	assert.Equal(t, filepath.Join(home, ".config", "ai-rulez"), UserConfigDir(func(string) string { return "" }, home))
}
