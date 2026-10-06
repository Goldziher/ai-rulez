package includes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestResolveIncludes_FailingIncludeIsAnErrorWhenLockIsStrict(t *testing.T) {
	tests := []struct {
		name    string
		mode    LockMode
		require bool
		lock    bool
		wantErr bool
	}{
		{"default only warns", LockAuto, false, false, false},
		{"enforced lock without generate policy only warns", LockAuto, false, true, false},
		{"--locked", LockRequire, true, false, true},
		{"--frozen", LockFrozen, true, false, true},
		{"enforced lock under generate", LockAuto, true, true, true},
		{"generate without lock", LockAuto, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			oldMode, oldReq := Mode, RequireWhenEnforced
			t.Cleanup(func() { Mode, RequireWhenEnforced = oldMode, oldReq })
			Mode, RequireWhenEnforced = tt.mode, tt.require
			dir := t.TempDir()
			cfgDir := filepath.Join(dir, ".ai-rulez")
			require.NoError(t, os.MkdirAll(cfgDir, 0o755))
			if tt.lock {
				require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "ai-rulez.lock"), []byte("version = 1\n"), 0o644))
			}
			cfg := &config.Config{
				BaseDir: dir, ConfigDir: cfgDir,
				Includes: []config.IncludeConfig{{Name: "gone", Source: filepath.Join(dir, "missing")}},
				Content:  &config.ContentTree{Domains: map[string]*config.Domain{}},
			}

			// Act
			_, err := NewResolver(dir, "").ResolveIncludes(context.Background(), cfg)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "gone")
				return
			}
			require.NoError(t, err)
		})
	}
}
