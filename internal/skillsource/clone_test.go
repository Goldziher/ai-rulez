package skillsource

import (
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// randomBytes is incompressible, so a blob of n bytes costs about n bytes in a pack.
func randomBytes(n int) string {
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic test data
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.IntN(256))
	}
	return string(b)
}

// bigFixture is a remote with small skills under skills/ and a 2 MiB blob under
// other/. The remote allows partial clones, like the hosting services do.
func bigFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	write(t, f.work, "other/huge.bin", randomBytes(2<<20))
	git(t, f.work, "add", "-A")
	git(t, f.work, "commit", "--quiet", "-m", "big")
	git(t, f.work, "push", "--quiet", f.bare, "main")
	git(t, f.bare, "config", "uploadpack.allowFilter", "true")
	git(t, f.bare, "config", "uploadpack.allowAnySHA1InWant", "true")
	return f
}

func cacheEntries(t *testing.T, cache string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.WalkDir(cache, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if rel, relErr := filepath.Rel(cache, p); relErr == nil && rel != "." && rel != "refs.json" && filepath.Base(rel) != "refs.json" {
			out = append(out, rel)
		}
		return nil
	}))
	return out
}

func TestResolve_CloneOverTheLimitFailsWithADistinctErrorAndLeavesNoCacheEntry(t *testing.T) {
	// Arrange: no path, so the 2 MiB blob is part of the checkout.
	f := bigFixture(t)
	cache := t.TempDir()
	spec := Spec{Name: "big", URL: "git+" + f.url, Ref: "main", MaxCloneBytes: 256 << 10}

	// Act
	res, err := Resolve(context.Background(), spec, Options{CacheDir: cache})

	// Assert
	require.Error(t, err)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, ErrCloneTooLarge)
	assert.Contains(t, err.Error(), "max_clone_bytes")
	assert.Contains(t, err.Error(), `"big"`)
	for _, e := range cacheEntries(t, cache) {
		assert.NotContains(t, e, string(filepath.Separator), "nothing below the per-repository directory (no commit directory, tree or partial checkout): %s", e)
	}
}

func TestResolve_SourcePathDownloadsOnlyTheSubtree(t *testing.T) {
	// Arrange: the limit is far below the 2 MiB blob outside the path.
	f := bigFixture(t)
	cache := t.TempDir()
	spec := Spec{Name: "team", URL: "git+" + f.url, Ref: "main", Path: "skills", MaxCloneBytes: 256 << 10}

	// Act
	res, err := Resolve(context.Background(), spec, Options{CacheDir: cache})

	// Assert
	require.NoError(t, err, "only skills/ is downloaded, so the 2 MiB blob does not count")
	assert.Equal(t, []string{"pdf", "sql", "wip-draft"}, names(res))
	tree := cacheTree(cache, spec.URL, res.Commit, "skills")
	assert.NoFileExists(t, filepath.Join(tree, "other", "huge.bin"), "files outside the path are not checked out")
	assert.DirExists(t, filepath.Join(tree, "skills", "pdf"))
}

func TestResolve_ServerWithoutPartialCloneSupportStillHonoursTheLimit(t *testing.T) {
	// Arrange: the filter is ignored by a server that does not allow it, so the
	// size watch has to stop the clone.
	f := bigFixture(t)
	git(t, f.bare, "config", "uploadpack.allowFilter", "false")
	spec := Spec{Name: "big", URL: "git+" + f.url, Ref: "main", MaxCloneBytes: 256 << 10}

	// Act
	_, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir()})

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCloneTooLarge)
}

func TestResolve_CloneWithinTheLimitIsCached(t *testing.T) {
	// Arrange
	f := newFixture(t)
	spec := Spec{Name: "team", URL: "git+" + f.url, Ref: "v1.0.0", Path: "skills", MaxCloneBytes: 10 << 20}

	// Act
	res, err := Resolve(context.Background(), spec, Options{CacheDir: t.TempDir()})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"pdf", "sql", "wip-draft"}, names(res))
}

func TestMaxCloneBytes_SourceThenGlobalThenEnvironmentThenDefault(t *testing.T) {
	tests := []struct {
		name   string
		spec   int64
		global int64
		env    string
		want   int64
	}{
		{"default", 0, 0, "", DefaultMaxCloneBytes},
		{"environment", 0, 0, "1000", 1000},
		{"invalid environment falls back to the default", 0, 0, "lots", DefaultMaxCloneBytes},
		{"global beats the environment", 0, 2000, "1000", 2000},
		{"source beats the global limit", 3000, 2000, "1000", 3000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			t.Setenv(EnvMaxCloneBytes, tt.env)

			// Act
			got := Spec{MaxCloneBytes: tt.spec}.maxCloneBytes(tt.global)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
	assert.Equal(t, int64(256<<20), DefaultMaxCloneBytes)
}
