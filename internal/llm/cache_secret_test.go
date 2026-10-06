package llm

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

func TestEnvOptionControlsHomeAndSecretLocations(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	xdg := filepath.Join(t.TempDir(), "xdg")
	tests := []struct {
		name       string
		env        ambient.Env
		wantSecret string
		wantCache  string
	}{
		{"home only", ambient.MapEnv{Home: home}, filepath.Join(home, ".config", "ai-rulez", cacheSecretFile), filepath.Join(home, ".cache", "ai-rulez", "llm")},
		{"xdg wins for the secret", ambient.MapEnv{Home: home, Vars: map[string]string{"XDG_CONFIG_HOME": xdg}}, filepath.Join(xdg, "ai-rulez", cacheSecretFile), filepath.Join(home, ".cache", "ai-rulez", "llm")},
		{"no home", ambient.MapEnv{}, "", ""},
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
