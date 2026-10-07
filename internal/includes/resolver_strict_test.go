package includes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestResolveIncludes_FailingIncludeIsAnErrorUnlessOffline(t *testing.T) {
	tests := []struct {
		name    string
		mode    LockMode
		require bool
		lock    bool
		wantErr bool
	}{
		{"default fails", LockAuto, false, false, true},
		{"enforced lock without generate policy fails", LockAuto, false, true, true},
		{"--locked", LockRequire, true, false, true},
		{"--frozen", LockFrozen, true, false, true},
		{"enforced lock under generate", LockAuto, true, true, true},
		{"generate without lock", LockAuto, true, false, true},
		{"--no-fetch keeps the warning", LockAuto, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			oldMode, oldReq := Mode, RequireWhenEnforced
			t.Cleanup(func() { Mode, RequireWhenEnforced = oldMode, oldReq })
			Mode, RequireWhenEnforced = tt.mode, tt.require
			oldSkip := SkipFetch
			t.Cleanup(func() { SkipFetch = oldSkip })
			SkipFetch = tt.name == "--no-fetch keeps the warning"
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
				assert.True(t, errors.Is(err, config.ErrLockViolation) || errors.Is(err, config.ErrIncludeUnresolved),
					"the loader only propagates these sentinels")
				return
			}
			require.NoError(t, err)
		})
	}
}
