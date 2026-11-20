package llm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

func TestEnvOptionControlsHomeAndSecretLocations(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	xdg := filepath.Join(t.TempDir(), "xdg")
	xdgCache := filepath.Join(t.TempDir(), "xdg-cache")
	tests := []struct {
		name       string
		env        ambient.Env
		wantSecret string
		wantCache  string
	}{
		{
			name:       "home only",
			env:        ambient.MapEnv{Home: home},
			wantSecret: filepath.Join(home, ".config", "ai-rulez", cacheSecretFile),
			wantCache:  filepath.Join(home, ".cache", "ai-rulez", "llm"),
		},
		{
			name:       "xdg wins for the secret",
			env:        ambient.MapEnv{Home: home, Vars: map[string]string{"XDG_CONFIG_HOME": xdg}},
			wantSecret: filepath.Join(xdg, "ai-rulez", cacheSecretFile),
			wantCache:  filepath.Join(home, ".cache", "ai-rulez", "llm"),
		},
		{
			name:       "xdg cache home wins for the cache",
			env:        ambient.MapEnv{Home: home, Vars: map[string]string{"XDG_CACHE_HOME": xdgCache}},
			wantSecret: filepath.Join(home, ".config", "ai-rulez", cacheSecretFile),
			wantCache:  filepath.Join(xdgCache, "ai-rulez", "llm"),
		},
		{
			name:       "xdg cache home works without a home directory",
			env:        ambient.MapEnv{Vars: map[string]string{"XDG_CACHE_HOME": xdgCache}},
			wantSecret: "",
			wantCache:  filepath.Join(xdgCache, "ai-rulez", "llm"),
		},
		{
			name:       "no home",
			env:        ambient.MapEnv{},
			wantSecret: "",
			wantCache:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			opts := Options{Env: tt.env, ConfigDir: t.TempDir()}
			secret, cache := secretPathFor(opts), CacheDirFor(opts)

			// Assert
			if secret != tt.wantSecret {
				t.Errorf("secret path = %q, want %q", secret, tt.wantSecret)
			}
			if tt.wantCache == "" && cache != "" || tt.wantCache != "" && filepath.Dir(cache) != tt.wantCache {
				t.Errorf("cache dir = %q, want under %q", cache, tt.wantCache)
			}
		})
	}
}

func TestSecretDirTooOpen(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		goos string
		want bool
	}{
		{"private dir on unix", 0o700, "linux", false},
		{"group writable on unix", 0o770, "linux", true},
		{"world writable on darwin", 0o707, "darwin", true},
		{"windows reports 0777 for every directory", 0o777, "windows", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := secretDirTooOpen(tt.mode, tt.goos)

			// Assert
			if got != tt.want {
				t.Fatalf("secretDirTooOpen(%o, %s) = %v, want %v", tt.mode, tt.goos, got, tt.want)
			}
		})
	}
}
