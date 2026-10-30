package includes

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// sharedIncludeRemote is a remote holding one include rule whose body reads
// "version <n>", with a helper to build checkouts that pin one of its commits.
type sharedIncludeRemote struct {
	dir  string
	rule string
}

func newSharedIncludeRemote(t *testing.T, version string) *sharedIncludeRemote {
	t.Helper()
	dir := t.TempDir()
	testutil.Git(t, dir, "init", "-q", "-b", "main")
	r := &sharedIncludeRemote{dir: dir, rule: filepath.Join(dir, ".ai-rulez", "rules", "shared.md")}
	r.commit(t, version)
	return r
}

// commit sets the rule to version and commits it, returning the commit.
func (r *sharedIncludeRemote) commit(t *testing.T, version string) string {
	t.Helper()
	writeTestFile(t, r.rule, "# Shared\n\nversion "+version+"\n")
	testutil.Git(t, r.dir, "add", "-A")
	testutil.Git(t, r.dir, "commit", "-qm", version)
	return testutil.Git(t, r.dir, "rev-parse", "HEAD")
}

// url is the include source as a project writes it.
func (r *sharedIncludeRemote) url() string { return "file://" + filepath.ToSlash(r.dir) }

// checkout writes a project that includes the remote and locks it to the
// remote's current HEAD, the way a checkout on that commit carries it.
func (r *sharedIncludeRemote) checkout(t *testing.T) (project, commit string) {
	t.Helper()
	project = t.TempDir()
	writeTestFile(t, filepath.Join(project, ".ai-rulez", "config.toml"), `version = "5.0"
name = "p"
presets = ["claude"]
gitignore = false

[[includes]]
name = "shared"
source = "`+r.url()+`"
`)
	lockPolicy.Mode, lockPolicy.Refresh, lockPolicy.Offline = LockRefresh, nil, false
	cfg, err := loadWithResolvers(context.Background(), project, config.WithoutLocal())
	require.NoError(t, err)
	lockPolicy.Mode, lockPolicy.Refresh, lockPolicy.Offline = LockAuto, nil, false
	lock, problems := BuildLock(cfg, nil)
	require.Empty(t, problems)
	require.Len(t, lock.Include, 1)
	require.NoError(t, lockfile.Save(cfg.ConfigDir, lock))
	return project, lock.Include[0].Commit
}

// load reads a checkout under its lock, from the cache.
func (r *sharedIncludeRemote) load(t *testing.T, project string) (*config.Config, error) {
	t.Helper()
	return loadWithResolvers(context.Background(), project, config.WithoutLocal())
}

// TestPinnedIncludeResolvesItsOwnCommitFromASharedCache guards #524: two
// checkouts whose locks pin different commits of one include share the include
// cache, so each must read the commit its own lock records, whatever order they
// load in.
func TestPinnedIncludeResolvesItsOwnCommitFromASharedCache(t *testing.T) {
	tests := []struct {
		name    string
		online  func(first, second string) []string
		offline func(first, second string) []string
	}{
		{
			name:    "the older pin loads last",
			online:  func(first, second string) []string { return []string{second, first} },
			offline: func(first, second string) []string { return []string{first, second} },
		},
		{
			name:    "the older pin loads first",
			online:  func(first, second string) []string { return []string{first, second} },
			offline: func(first, second string) []string { return []string{second, first} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			isolateHome(t)
			remote := newSharedIncludeRemote(t, "one")
			projectFirst, first := remote.checkout(t)
			second := remote.commit(t, "two")
			projectSecond, lockedSecond := remote.checkout(t)
			require.Equal(t, second, lockedSecond, "the second checkout locks the newer commit")
			require.NotEqual(t, first, second)
			projects := map[string]string{first: projectFirst, second: projectSecond}
			body := map[string]string{first: "version one", second: "version two"}

			// Act: both checkouts load online, then offline the way `lock --check`
			// and the MCP settings load do.
			for _, commit := range tt.online(first, second) {
				cfg, err := remote.load(t, projects[commit])
				require.NoError(t, err, "online load of the checkout pinning %s", commit)
				assert.Contains(t, ruleBody(cfg), body[commit], "online load of the checkout pinning %s", commit)
			}
			lockPolicy.Offline = true
			defer func() { lockPolicy.Offline = false }()
			for _, commit := range tt.offline(first, second) {
				cfg, err := remote.load(t, projects[commit])
				require.NoError(t, err, "offline load of the checkout pinning %s", commit)
				assert.Contains(t, ruleBody(cfg), body[commit], "offline load of the checkout pinning %s", commit)
			}
		})
	}
}

// TestPinnedIncludeCacheLeftAtAnotherCommitIsCorrected guards the second half of
// #524: when the shared cache directory holds another commit (another checkout
// fetched it, or an older ai-rulez keyed it without the commit), a lock-pinned
// load fetches its own commit instead of reporting a lock violation.
func TestPinnedIncludeCacheLeftAtAnotherCommitIsCorrected(t *testing.T) {
	// Arrange
	isolateHome(t)
	remote := newSharedIncludeRemote(t, "one")
	pinnedFirst, first := remote.checkout(t)
	cfg, err := remote.load(t, pinnedFirst)
	require.NoError(t, err)
	require.Contains(t, ruleBody(cfg), "version one")
	second := remote.commit(t, "two")
	pinnedSecond, lockedSecond := remote.checkout(t)
	require.Equal(t, second, lockedSecond)
	_, err = remote.load(t, pinnedSecond)
	require.NoError(t, err)
	shared := remote.legacyCacheDir(t)
	require.Equal(t, second, cachedCommit(shared), "precondition: another checkout left the shared cache at its commit")

	// Act
	reloaded, err := remote.load(t, pinnedFirst)

	// Assert
	require.NoError(t, err, "a cache left at another commit is fetched again, not a lock violation")
	assert.Contains(t, ruleBody(reloaded), "version one")
	assert.Equal(t, first, cachedCommit(remote.ownCacheDir(t, first)), "the checkout has a tree of its own commit")
	lockPolicy.Offline = true
	defer func() { lockPolicy.Offline = false }()
	offline, err := remote.load(t, pinnedFirst)
	require.NoError(t, err, "the corrected tree serves the next offline read")
	assert.Contains(t, ruleBody(offline), "version one")
}

// legacyCacheDir is the commit-less directory the include was cached in before
// the commit became part of the identity, and by a fetch that follows a ref.
func (r *sharedIncludeRemote) legacyCacheDir(t *testing.T) string {
	t.Helper()
	dir, err := getIncludeCacheDir(ambient.Host{}, "shared", r.url(), "", "")
	require.NoError(t, err)
	return dir
}

// ownCacheDir is the directory a checkout pinning commit reads and writes.
func (r *sharedIncludeRemote) ownCacheDir(t *testing.T, commit string) string {
	t.Helper()
	dir, err := getIncludeCacheDir(ambient.Host{}, "shared", r.url(), "", commit)
	require.NoError(t, err)
	return dir
}
