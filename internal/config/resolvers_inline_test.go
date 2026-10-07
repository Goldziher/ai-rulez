package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadWithoutResolversFailsWhenAnIncludeIsConfigured(t *testing.T) {
	tests := []struct {
		name      string
		opts      []LoadOption
		wantError string
	}{
		{name: "no resolvers", wantError: "includes resolver is unavailable"},
		{name: "remote skipped", opts: []LoadOption{WithoutRemote()}},
		{
			name: "a resolver is given",
			opts: []LoadOption{WithResolvers(Resolvers{
				Includes: func(_ context.Context, cfg *Config) (*ContentTree, error) { return cfg.Content, nil },
			})},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			cfgDir := filepath.Join(dir, ".ai-rulez")
			require.NoError(t, os.MkdirAll(cfgDir, 0o755))
			body := "version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n\n[[includes]]\nname = \"shared\"\nsource = \"shared\"\n"
			require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(body), 0o644))

			// Act
			cfg, err := LoadConfig(t.Context(), dir, tt.opts...)

			// Assert
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, cfg)
		})
	}
}
