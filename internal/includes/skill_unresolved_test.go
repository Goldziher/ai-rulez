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

// TestInstalledSkill_UnresolvedFailsTheLoad is RV-ENGINE-2: an installed skill
// that cannot be resolved fails the load like an include does. Dropping it
// silently made generate delete its outputs as stale and `generate --check`
// pass. Only an explicit offline run, a command that inventories sources (lock,
// sbom, CRUD) and a `lock` refresh go on without it.
func TestInstalledSkill_UnresolvedFailsTheLoad(t *testing.T) {
	tests := []struct {
		name    string
		ctx     func(context.Context) context.Context
		policy  config.LockPolicy
		wantErr bool
	}{
		{name: "a plain load fails", wantErr: true},
		{name: "generate (an enforced lock is required) fails", policy: config.LockPolicy{RequireWhenEnforced: true}, wantErr: true},
		{name: "--no-fetch keeps the warning", policy: config.LockPolicy{Offline: true}},
		{name: "an inventory command keeps the warning", ctx: config.WithUnresolvedIncludesTolerated},
		{name: "a lock refresh keeps the warning", policy: config.LockPolicy{Mode: LockRefresh}},
		{name: "--locked fails even when tolerated", ctx: config.WithUnresolvedIncludesTolerated, policy: config.LockPolicy{Mode: LockRequire}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			t.Setenv("HOME", t.TempDir())
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n\n[[installed_skills]]\nname = \"foo\"\nsource = \"vendor\"\npath = \"foo\"\n")
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o755)) // the skill itself is gone
			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx(ctx)
			}

			// Act
			cfg, err := loadWithResolvers(ctx, dir, config.WithoutLocal(), config.WithLockPolicy(tt.policy))

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, config.ErrSkillUnresolved) || errors.Is(err, config.ErrLockViolation), err.Error())
				assert.Contains(t, err.Error(), "foo")
				return
			}
			require.NoError(t, err)
			for _, s := range cfg.Content.Skills {
				assert.NotEqual(t, "foo", s.Name)
			}
		})
	}
}
