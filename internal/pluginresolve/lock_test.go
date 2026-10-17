package pluginresolve_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// writeLock runs `ai-rulez lock` over the project.
func writeLock(t *testing.T, dir string, req lockrun.Request, policy config.LockPolicy) (*lockrun.Result, error) {
	t.Helper()
	env := lockrun.Env{
		Version: "test", Policy: policy, Cwd: dir,
		Load: func(ctx context.Context, p config.LockPolicy, opts ...config.LoadOption) (*config.Config, error) {
			return loadCtx(ctx, dir, append(opts, config.WithLockPolicy(p))...)
		},
	}
	return lockrun.Write(context.Background(), req, env)
}

func mustLock(t *testing.T, dir string) *lockfile.File {
	t.Helper()
	res, err := writeLock(t, dir, lockrun.Request{}, config.LockPolicy{})
	require.NoError(t, err)
	return res.Lock
}

// withPolicy loads the project under a lock policy.
func withPolicy(t *testing.T, dir string, p config.LockPolicy) (*config.Config, error) {
	t.Helper()
	return load(t, dir, config.WithLockPolicy(p))
}

type world struct {
	market  *plugintest.Repo
	plug    *plugintest.Repo
	project string
}

// newWorld is a project installing one plugin that lives in the marketplace
// repository and one the marketplace points at another repository.
func newWorld(t *testing.T) *world {
	t.Helper()
	isolate(t)
	plug := plugintest.NewRepo(t, plugintest.ClaudePlugin("ext"))
	market := plugintest.NewRepo(t, plugintest.Merge(
		marketFiles(pluginimport.IndexClaude,
			`{"name":"inrepo","source":"./plugins/inrepo"}`,
			`{"name":"ext","source":{"source":"url","url":"`+plug.URL()+`","ref":"main"}}`),
		plugintest.Prefix("plugins/inrepo", plugintest.ClaudePlugin("inrepo"))))
	project := plugintest.Project(t, `
[[installed_marketplaces]]
name = "acme"
source = "`+market.URL()+`"

[[installed_plugins]]
name = "inrepo"
marketplace = "acme"

[[installed_plugins]]
name = "ext"
marketplace = "acme"
`)
	return &world{market: market, plug: plug, project: project}
}

func TestLock_PinsMarketplacesAndPlugins(t *testing.T) {
	// Arrange
	w := newWorld(t)

	// Act
	lock := mustLock(t, w.project)

	// Assert
	mp := lock.Find(lockfile.KindMarketplace, "acme")
	require.NotNil(t, mp)
	assert.Equal(t, w.market.Head(), mp.Commit)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, mp.Digest)

	inrepo := lock.Find(lockfile.KindPlugin, "inrepo")
	require.NotNil(t, inrepo)
	assert.Equal(t, "marketplace:acme", inrepo.Source)
	assert.Equal(t, w.market.Head(), inrepo.Commit, "a plugin in the marketplace repository shares its commit")
	assert.NotEqual(t, mp.Digest, inrepo.Digest, "and has a digest of its own directory")
	assert.Contains(t, inrepo.Origin, "#plugins/inrepo")

	ext := lock.Find(lockfile.KindPlugin, "ext")
	require.NotNil(t, ext)
	assert.Equal(t, w.plug.Head(), ext.Commit)
	assert.Equal(t, "marketplace:acme", ext.Source)
	assert.Equal(t, w.plug.URL()+"@main", ext.Origin, "the origin is the repository the index named, credential free")

	saved, err := lockfile.Load(filepath.Join(w.project, ".ai-rulez"))
	require.NoError(t, err)
	assert.ElementsMatch(t, lock.Plugin, saved.Plugin)
}

func TestLock_IsStableAcrossRuns(t *testing.T) {
	// Arrange
	w := newWorld(t)
	mustLock(t, w.project)
	first, err := os.ReadFile(filepath.Join(w.project, ".ai-rulez", lockfile.FileName))
	require.NoError(t, err)

	// Act
	mustLock(t, w.project)
	second, err := os.ReadFile(filepath.Join(w.project, ".ai-rulez", lockfile.FileName))
	require.NoError(t, err)

	// Assert
	assert.Equal(t, string(first), string(second))
}

func TestLock_AnUpstreamChangeDoesNotMoveAPin(t *testing.T) {
	tests := []struct {
		name  string
		drift func(w *world)
	}{
		{"nothing changed", func(*world) {}},
		{"marketplace commit moved", func(w *world) {
			w.market.Commit(map[string]string{"plugins/inrepo/commands/new.md": "new\n"}, "upstream")
		}},
		{"plugin repository commit moved", func(w *world) {
			w.plug.Commit(map[string]string{"commands/new.md": "new\n"}, "upstream")
		}},
		{"plugin repository content removed", func(w *world) { w.plug.Remove("drop the agent", "agents") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			w := newWorld(t)
			mustLock(t, w.project)
			tt.drift(w)

			// Act: a load that enforces the lock fetches the pinned commits
			cfg, err := withPolicy(t, w.project, config.LockPolicy{RequireWhenEnforced: true})

			// Assert: the pinned content, whatever the remote now holds
			require.NoError(t, err)
			assert.Equal(t, []string{"review-ext", "review-inrepo"}, sortedStrings(skillNames(cfg)))
			assert.Equal(t, []string{"deploy"}, fileNames(cfg.Content.Commands))
			assert.Equal(t, []string{"helper"}, fileNames(cfg.Content.Agents))
		})
	}
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestLock_RefreshAcceptsTheNewCommit(t *testing.T) {
	// Arrange
	w := newWorld(t)
	before := mustLock(t, w.project)
	w.plug.Commit(map[string]string{"commands/new.md": "new\n"}, "upstream")
	w.market.Commit(map[string]string{"plugins/inrepo/commands/new.md": "new\n"}, "upstream")

	// Act
	after := mustLock(t, w.project)

	// Assert
	assert.NotEqual(t, before.Find(lockfile.KindPlugin, "ext").Commit, after.Find(lockfile.KindPlugin, "ext").Commit)
	assert.Equal(t, w.plug.Head(), after.Find(lockfile.KindPlugin, "ext").Commit)
	assert.Equal(t, w.market.Head(), after.Find(lockfile.KindMarketplace, "acme").Commit)
	assert.NotEqual(t, before.Find(lockfile.KindPlugin, "inrepo").Digest, after.Find(lockfile.KindPlugin, "inrepo").Digest)
	_, err := withPolicy(t, w.project, config.LockPolicy{RequireWhenEnforced: true})
	require.NoError(t, err)
}

func TestLock_CacheTamperingIsCaughtOffline(t *testing.T) {
	// Arrange
	w := newWorld(t)
	mustLock(t, w.project)
	cache, err := config.CacheDirIn(nil, "plugins")
	require.NoError(t, err)
	matches, err := filepath.Glob(filepath.Join(cache, "ext-*", "commands", "deploy.md"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	require.NoError(t, os.WriteFile(matches[0], []byte("tampered\n"), 0o644))

	// Act
	_, offlineErr := withPolicy(t, w.project, config.LockPolicy{RequireWhenEnforced: true, Offline: true})
	_, onlineErr := withPolicy(t, w.project, config.LockPolicy{RequireWhenEnforced: true})

	// Assert
	require.ErrorIs(t, offlineErr, config.ErrLockViolation)
	assert.Contains(t, offlineErr.Error(), "digest")
	require.NoError(t, onlineErr, "online, the pinned commit is fetched again and the cache repaired")
}

func TestLock_RequireFrozenAndOffline(t *testing.T) {
	t.Run("a missing lock is a violation under --locked", func(t *testing.T) {
		w := newWorld(t)
		_, err := withPolicy(t, w.project, config.LockPolicy{Mode: config.LockRequire})
		require.ErrorIs(t, err, config.ErrLockViolation)
		assert.Contains(t, err.Error(), "ai-rulez lock")
	})
	t.Run("a plugin the lock does not cover is a violation", func(t *testing.T) {
		w := newWorld(t)
		mustLock(t, w.project)
		extra := plugintest.NewRepo(t, plugintest.ClaudePlugin("more"))
		require.NoError(t, os.WriteFile(filepath.Join(w.project, ".ai-rulez", "config.toml"), appendTo(t, filepath.Join(w.project, ".ai-rulez", "config.toml"),
			"\n[[installed_plugins]]\nname = \"more\"\nsource = \""+extra.URL()+"\"\n"), 0o644))
		_, err := withPolicy(t, w.project, config.LockPolicy{RequireWhenEnforced: true})
		require.ErrorIs(t, err, config.ErrLockViolation)
		assert.Contains(t, err.Error(), "more")
	})
	t.Run("frozen runs from the cache without the network", func(t *testing.T) {
		w := newWorld(t)
		mustLock(t, w.project)
		w.plug.Commit(map[string]string{"commands/new.md": "new\n"}, "upstream not yet pinned")
		cfg, err := withPolicy(t, w.project, config.LockPolicy{Mode: config.LockFrozen, Offline: true})
		require.NoError(t, err)
		assert.Equal(t, []string{"deploy"}, fileNames(cfg.Content.Commands), "the pinned content, not the new upstream commit")
	})
	t.Run("frozen with nothing cached fails", func(t *testing.T) {
		w := newWorld(t)
		mustLock(t, w.project)
		testutil.IsolateHome(t) // an empty cache
		_, err := withPolicy(t, w.project, config.LockPolicy{Mode: config.LockFrozen, Offline: true})
		require.Error(t, err)
		assert.True(t, errors.Is(err, config.ErrLockViolation) || errors.Is(err, config.ErrPluginUnresolved), err.Error())
		assert.ErrorIs(t, err, includes.ErrNotCached)
	})
	t.Run("a lock for another plugin source is stale", func(t *testing.T) {
		w := newWorld(t)
		mustLock(t, w.project)
		other := plugintest.NewRepo(t, plugintest.ClaudePlugin("other"))
		cfgPath := filepath.Join(w.project, ".ai-rulez", "config.toml")
		data, err := os.ReadFile(cfgPath)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(cfgPath, []byte(replaceOnce(string(data), w.market.URL(), other.URL())), 0o644))
		_, err = withPolicy(t, w.project, config.LockPolicy{RequireWhenEnforced: true})
		require.ErrorIs(t, err, config.ErrLockViolation)
		assert.Contains(t, err.Error(), "stale")
	})
}

func appendTo(t *testing.T, path, text string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return append(data, text...)
}

func replaceOnce(s, old, repl string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + repl + s[i+len(old):]
		}
	}
	return s
}

func TestLock_CheckAndNotCoveredReporting(t *testing.T) {
	// Arrange
	w := newWorld(t)
	lock := mustLock(t, w.project)
	cfg, err := load(t, w.project)
	require.NoError(t, err)

	// Act
	problems, cached := includes.CheckLock(cfg, lock)
	pinned := includes.Unpinned(cfg)

	// Assert
	assert.Empty(t, problems)
	assert.Positive(t, cached, "the cached trees were verified against their digests")
	assert.Empty(t, pinned)

	// Act: a lock that lost an entry
	lock.Plugin = lock.Plugin[:1]
	problems, _ = includes.CheckLock(cfg, lock)

	// Assert
	require.NotEmpty(t, problems)
	assert.Contains(t, problems[0].String(), "not covered")
}
