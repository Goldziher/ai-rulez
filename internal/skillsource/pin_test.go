package skillsource

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_LockedRefThatMovedIsFetchedByCommitIntoAnEmptyCache(t *testing.T) {
	for _, ref := range []string{"v1.0.0", "main", ""} {
		t.Run("ref="+ref, func(t *testing.T) {
			f := newFixture(t)
			spec, lock, pinned, moved := lockedThenMoved(t, f, ref)
			require.NotEqual(t, pinned, moved)

			res, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir(), Lock: lock})
			require.NoError(t, err, "the pinned commit is fetched even though %q moved", ref)
			assert.Equal(t, pinned, res.Commit)
			assert.True(t, res.Locked)
		})
	}
}

func TestResolve_LockedCommitThatIsGoneFailsClosed(t *testing.T) {
	f := newFixture(t)
	spec, lock, _, _ := lockedThenMoved(t, f, "main")
	entry := lock.Find(lockfile.KindSource, "team")
	entry.Commit = "0123456789abcdef0123456789abcdef01234567"
	_, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir(), Lock: lock})
	require.Error(t, err)
}

// legacyProtocol makes git speak protocol v0, where a server refuses to serve a
// commit that no ref advertises: the fetch by SHA fails and the ref fallback runs.
func legacyProtocol(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.version")
	t.Setenv("GIT_CONFIG_VALUE_0", "0")
}

func TestResolve_LockedCommitBehindTheRefIsFoundThroughTheRefWhenSHAFetchIsRefused(t *testing.T) {
	legacyProtocol(t)
	f := newFixture(t)
	spec, lock, pinned, _ := lockedThenMoved(t, f, "main")
	res, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir(), Lock: lock})
	require.NoError(t, err)
	assert.Equal(t, pinned, res.Commit)
}

func TestResolve_LockedCommitRewrittenAwayFailsClosed(t *testing.T) {
	legacyProtocol(t)
	f := newFixture(t)
	spec, lock, pinned, _ := lockedThenMoved(t, f, "main")
	// Rewrite history: an orphan commit replaces main, so the pinned commit is unreachable.
	git(t, f.work, "checkout", "--quiet", "--orphan", "rewritten")
	write(t, f.work, "skills/pdf/SKILL.md", skillMD("pdf", "Rewritten"))
	git(t, f.work, "add", "-A")
	git(t, f.work, "commit", "--quiet", "-m", "rewrite")
	git(t, f.work, "push", "--quiet", "--force", f.bare, "rewritten:main")
	git(t, f.bare, "gc", "--quiet", "--prune=now")

	_, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir(), Lock: lock})
	require.Error(t, err)
	assert.Contains(t, err.Error(), pinned)
}

func TestResolve_NoHomeMeansNoWorldWritableCacheFallback(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("home", "")
	f := newFixture(t)
	_, err := Resolve(context.Background(), Spec{Name: "t", URL: "git+" + f.url, Ref: "v1.0.0"}, Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache")
}

func TestResolve_ACompetingProcessStoringTheSameTreeIsTolerated(t *testing.T) {
	f := newFixture(t)
	cache := t.TempDir()
	spec := Spec{Name: "t", URL: "git+" + f.url, Ref: "v1.0.0", Path: "skills"}
	first, err := Resolve(context.Background(), spec, Options{CacheDir: cache})
	require.NoError(t, err)
	// Simulate the loser of a race: the tree exists although this process meant to store it.
	var fetched bool
	treeDir := cacheTree(cache, "git+"+f.url, first.Commit)
	require.NoError(t, fetchInto(context.Background(), gitURL("git+"+f.url), "v1.0.0", kindTag, first.Commit, treeDir, &fetched))
}
