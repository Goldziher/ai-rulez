package includes

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// TestVersionPolicy_ConcurrentLoadsKeepTheirOwnPolicy runs `update`-like and
// `lock`-like refreshes of two projects at the same time in one process (an MCP
// server, an embedding service). The version policy (Advance, AllowDowngrade,
// AcceptMovedTag, ReleaseGate) belongs to each load: the refresh that advances
// must not make the other one advance, and the other way round.
func TestVersionPolicy_ConcurrentLoadsKeepTheirOwnPolicy(t *testing.T) {
	// Arrange: two projects, each on its own remote (the include cache keeps one
	// checkout per remote), both pinned to v1.1.0; then v1.2.0 appears on both.
	f := newVersionFixture(t, `version = "^1"`, `ref = "main"`)
	other := newVersionFixture(t, `version = "^1"`, `ref = "main"`)
	locks := map[string]*lockfile.File{f.project: f.refresh2(t), other.project: other.refresh2(t)}
	f.release(t, "release 1.2", "v1.2.0", true)
	other.release(t, "release 1.2", "v1.2.0", true)

	advance := config.LockPolicy{Mode: config.LockRefresh, VersionPolicy: config.VersionPolicy{
		Advance: func(kind, _ string) bool { return kind == lockfile.KindInclude },
	}}
	keep := config.LockPolicy{Mode: config.LockRefresh}
	refresh := func(project string, policy config.LockPolicy) (string, []string, error) {
		cfg, err := config.LoadConfig(context.Background(), project,
			config.WithResolvers(Resolvers("")), config.WithLockPolicy(policy), config.WithoutLocal())
		if err != nil {
			return "", nil, err //nolint:wrapcheck // test helper
		}
		lock, problems := BuildLock(cfg, locks[project])
		if e := lock.Find(lockfile.KindInclude, "shared"); e != nil {
			return e.Tag, problems, nil
		}
		return "", problems, nil
	}

	// Act: both refreshes, repeatedly and concurrently.
	const rounds = 4
	type outcome struct {
		tag      string
		problems []string
		err      error
	}
	advanced, kept := make([]outcome, rounds), make([]outcome, rounds)
	var wg sync.WaitGroup
	for i := range rounds {
		wg.Add(2)
		go func() {
			defer wg.Done()
			tag, problems, err := refresh(f.project, advance)
			advanced[i] = outcome{tag, problems, err}
		}()
		go func() {
			defer wg.Done()
			tag, problems, err := refresh(other.project, keep)
			kept[i] = outcome{tag, problems, err}
		}()
	}
	wg.Wait()

	// Assert: each load moved (or kept) the pin by its own policy only.
	for i := range rounds {
		require.NoError(t, advanced[i].err, "advancing load %d", i)
		require.NoError(t, kept[i].err, "keeping load %d", i)
		assert.Empty(t, advanced[i].problems, "advancing load %d", i)
		assert.Empty(t, kept[i].problems, "keeping load %d", i)
		assert.Equal(t, "v1.2.0", advanced[i].tag, "the load whose policy advances moves to the newest allowed tag (round %d)", i)
		assert.Equal(t, "v1.1.0", kept[i].tag, "the load whose policy does not advance keeps its pin (round %d)", i)
	}
}
