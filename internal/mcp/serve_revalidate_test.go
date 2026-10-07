package mcp

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// An approval that expires while the server runs must stop the skill being
// served without any file changing: admission is judged against the clock, so
// it is re-evaluated on a timer, not only when the catalog is rebuilt.
func TestWatch_RevalidatesApprovalExpiryWithoutAFileChange(t *testing.T) {
	tests := []struct {
		name        string
		noWatch     bool
		wantRefused bool
	}{
		{name: "live reload on", wantRefused: true},
		{name: "live reload off", noWatch: true, wantRefused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := project(t, baseConfig, map[string]string{"skills/core/SKILL.md": skillFile("core", "Core conventions", "")})
			probe := newServerFor(t, &ServeSetup{WorkDir: root})
			core, ok := probe.Catalog().Lookup("core")
			require.True(t, ok)
			writeFile(t, root, ".ai-rulez/config.toml", baseConfig+"\n[governance]\nrequire_approval = [\"kind:served\"]\nenforce = true\n")
			require.NoError(t, lockfile.Save(filepath.Join(root, ".ai-rulez"), &lockfile.File{
				Version: lockfile.Version,
				Served:  []lockfile.Entry{{Name: "core", Source: core.Source, Digest: core.LockDigest}},
				Approval: []lockfile.Approval{{
					Kind: "served", ID: "core", Digest: core.LockDigest, Reviewer: "alice",
					Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-10-01T12:00:00Z", Expires: "2026-10-10",
				}},
			}))
			var now atomic.Value
			now.Store(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
			setup := &ServeSetup{
				WorkDir: root, NoWatch: tt.noWatch, PollInterval: 10 * time.Millisecond, RevalidateInterval: 20 * time.Millisecond,
				Clock: func() time.Time { return now.Load().(time.Time) }, CacheDir: filepath.Join(t.TempDir(), "cache"),
			}
			srv, err := setup.NewServer(context.Background())
			require.NoError(t, err)
			_, served := srv.Catalog().Lookup("core")
			require.True(t, served, "the approval is valid on its expiry date")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go srv.Watch(ctx)

			// Act
			now.Store(time.Date(2026, 10, 11, 0, 0, 1, 0, time.UTC))

			// Assert
			require.Eventually(t, func() bool {
				r, refused := srv.Catalog().Refusal("core")
				return refused && r.Code != ""
			}, 5*time.Second, 10*time.Millisecond, "the expired approval still serves the skill")
			_, served = srv.Catalog().Lookup("core")
			require.False(t, served)
		})
	}
}
