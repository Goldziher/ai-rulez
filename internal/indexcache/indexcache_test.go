package indexcache

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// project makes a project root with a .ai-rulez directory and isolates the home.
func project(t *testing.T) (root, configDir string) {
	t.Helper()
	testutil.IsolateHome(t)
	root = t.TempDir()
	configDir = filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	return root, configDir
}

func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func opts(root, configDir, mode string) Options {
	return Options{Root: root, ConfigDir: configDir, Mode: mode, Key: testKey(1), Fingerprint: "fp1", Tree: "tree1"}
}

func TestLocate(t *testing.T) {
	root, configDir := project(t)
	otherRoot := t.TempDir()

	tests := []struct {
		name         string
		o            Options
		wantKind     string
		wantContains string
		wantFallback bool
	}{
		{name: "user", o: opts(root, configDir, config.IndexCacheUser), wantKind: KindUser, wantContains: filepath.Join("ai-rulez", "monorepo-index")},
		{name: "default is user", o: opts(root, configDir, ""), wantKind: KindUser, wantContains: filepath.Join("ai-rulez", "monorepo-index")},
		{name: "repo", o: opts(root, configDir, config.IndexCacheRepo), wantKind: KindRepo, wantContains: filepath.Join(".ai-rulez", ".cache", "monorepo-index")},
		{name: "root layout", o: opts(root, "", config.IndexCacheRepo), wantKind: KindRepo, wantContains: filepath.Join(".ai-rulez-cache", "monorepo-index")},
		{name: "off", o: opts(root, configDir, config.IndexCacheOff), wantKind: KindOff},
		{name: "other root", o: opts(otherRoot, "", config.IndexCacheUser), wantKind: KindUser, wantContains: "monorepo-index"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := Locate(context.Background(), tt.o)

			require.NoError(t, err)
			assert.Equal(t, tt.wantKind, loc.Kind)
			assert.Equal(t, tt.wantFallback, loc.Fallback)
			if tt.wantKind == KindOff {
				assert.Empty(t, loc.Dir)
				return
			}
			assert.Contains(t, loc.Dir, tt.wantContains)
			info, statErr := os.Stat(loc.Dir)
			require.NoError(t, statErr)
			assert.True(t, info.IsDir())
		})
	}
}

func TestLocate_UserDirectoryIsKeyedByTheProjectRoot(t *testing.T) {
	root, configDir := project(t)
	other := t.TempDir()

	a, err := Locate(context.Background(), opts(root, configDir, config.IndexCacheUser))
	require.NoError(t, err)
	again, err := Locate(context.Background(), opts(root, configDir, config.IndexCacheUser))
	require.NoError(t, err)
	b, err := Locate(context.Background(), opts(other, "", config.IndexCacheUser))
	require.NoError(t, err)

	assert.Equal(t, a.Dir, again.Dir)
	assert.NotEqual(t, a.Dir, b.Dir)
	assert.Equal(t, filepath.Dir(a.Dir), filepath.Dir(b.Dir))
}

func TestLocate_FallsBackToTheProjectWhenTheUserCacheIsNotWritable(t *testing.T) {
	root, configDir := project(t)
	home, err := ambient.OS().UserHomeDir()
	require.NoError(t, err)
	// A file where the cache directory should be makes it impossible to create.
	require.NoError(t, os.MkdirAll(home, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".cache"), []byte("x"), 0o600))

	loc, err := Locate(context.Background(), opts(root, configDir, config.IndexCacheUser))

	require.NoError(t, err)
	assert.Equal(t, KindRepo, loc.Kind)
	assert.True(t, loc.Fallback)
	assert.Contains(t, loc.Warning, "user cache")
	assert.Equal(t, filepath.Join(configDir, ".cache", "monorepo-index"), loc.Dir)
}

func TestLocate_RepoCacheIsExcludedFromGit(t *testing.T) {
	root, configDir := project(t)
	testutil.Git(t, root, "init", "-q")

	_, err := Locate(context.Background(), opts(root, configDir, config.IndexCacheRepo))
	require.NoError(t, err)
	_, err = Locate(context.Background(), opts(root, configDir, config.IndexCacheRepo))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), "/.ai-rulez/.cache/"), "the entry is added once")
}

func entry(hash string) repoindex.CachedItem {
	return repoindex.CachedItem{Name: "n", Description: "d", Hash: hash}
}

func TestStore_RoundTripsEntriesAndVerdicts(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	o := opts(root, configDir, config.IndexCacheUser)
	first, err := Open(ctx, o)
	require.NoError(t, err)
	first.Store("a/AGENTS.md", 10, 1000, entry("sha256:aa"))
	first.MarkClean("verdict-1")
	require.NoError(t, first.Save(ctx))

	second, err := Open(ctx, o)
	require.NoError(t, err)

	got, ok := second.Lookup("a/AGENTS.md", 10, 1000)
	require.True(t, ok)
	assert.Equal(t, entry("sha256:aa"), got)
	assert.True(t, second.Clean("verdict-1"))
	assert.False(t, second.Clean("verdict-2"))
	assert.Empty(t, second.Stats().Discarded)
}

func TestStore_FileModeAndNoBodies(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	s, err := Open(ctx, opts(root, configDir, config.IndexCacheUser))
	require.NoError(t, err)
	s.Store("AGENTS.md", 1, 1, entry("sha256:aa"))
	require.NoError(t, s.Save(ctx))

	entries, err := os.ReadDir(s.Location().Dir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	for _, e := range entries {
		info, infoErr := e.Info()
		require.NoError(t, infoErr)
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), e.Name())
		}
		assert.False(t, strings.HasSuffix(e.Name(), ".tmp"), "no temporary file is left behind")
	}
}

func TestStore_LookupRequiresSizeAndModificationTimeToMatch(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	o := opts(root, configDir, config.IndexCacheUser)
	first, err := Open(ctx, o)
	require.NoError(t, err)
	first.Store("AGENTS.md", 10, 5_000_000_000, entry("sha256:aa"))
	require.NoError(t, first.Save(ctx))
	second, err := Open(ctx, o)
	require.NoError(t, err)

	tests := []struct {
		name  string
		size  int64
		mtime int64
		want  bool
	}{
		{name: "same", size: 10, mtime: 5_000_000_000, want: true},
		{name: "size changed", size: 11, mtime: 5_000_000_000},
		{name: "mtime changed", size: 10, mtime: 5_000_000_001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := second.Lookup("AGENTS.md", tt.size, tt.mtime)

			assert.Equal(t, tt.want, ok)
		})
	}
}

func TestStore_DoesNotTrustAnEntryModifiedInTheSecondBeforeItWasSaved(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	o := opts(root, configDir, config.IndexCacheUser)
	now := int64(1_800_000_000_000_000_000)
	o.Host = ambient.Host{Clock: func() time.Time { return time.Unix(0, now) }}
	first, err := Open(ctx, o)
	require.NoError(t, err)
	first.Store("racy.md", 1, now-100, entry("sha256:aa"))
	first.Store("old.md", 1, now-int64(1e11), entry("sha256:bb"))
	require.NoError(t, first.Save(ctx))
	second, err := Open(ctx, o)
	require.NoError(t, err)

	_, racy := second.Lookup("racy.md", 1, now-100)
	_, old := second.Lookup("old.md", 1, now-int64(1e11))

	assert.False(t, racy, "a file written while the cache was saved may have changed again within the timestamp granularity")
	assert.True(t, old)
}

func TestStore_TreeChangeDropsEntriesButKeepsVerdicts(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	o := opts(root, configDir, config.IndexCacheUser)
	first, err := Open(ctx, o)
	require.NoError(t, err)
	first.Store("AGENTS.md", 1, 1, entry("sha256:aa"))
	first.MarkClean("v")
	require.NoError(t, first.Save(ctx))
	o.Tree = "tree2"

	second, err := Open(ctx, o)
	require.NoError(t, err)

	_, ok := second.Lookup("AGENTS.md", 1, 1)
	assert.False(t, ok)
	assert.True(t, second.Clean("v"), "a verdict is keyed by the content, which a checkout does not change")
}

func TestStore_FingerprintChangeDropsVerdictsButKeepsEntries(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	o := opts(root, configDir, config.IndexCacheUser)
	first, err := Open(ctx, o)
	require.NoError(t, err)
	first.Store("AGENTS.md", 1, 1, entry("sha256:aa"))
	first.MarkClean("v")
	require.NoError(t, first.Save(ctx))
	o.Fingerprint = "fp2"

	second, err := Open(ctx, o)
	require.NoError(t, err)

	_, ok := second.Lookup("AGENTS.md", 1, 1)
	assert.True(t, ok)
	assert.False(t, second.Clean("v"), "another scanner or scan setting must scan again")
}

func TestStore_DiscardsACacheItCannotTrust(t *testing.T) {
	tests := []struct {
		name       string
		damage     func(t *testing.T, file string)
		wantReason string
	}{
		{name: "not JSON", damage: func(t *testing.T, file string) { writeFile(t, file, "not json at all") }, wantReason: ReasonCorrupt},
		{name: "empty", damage: func(t *testing.T, file string) { writeFile(t, file, "") }, wantReason: ReasonCorrupt},
		{name: "other version", damage: func(t *testing.T, file string) { rewrite(t, file, `"version":2`, `"version":99`) }, wantReason: ReasonVersion},
		{name: "edited verdict", damage: func(t *testing.T, file string) { rewrite(t, file, `"v"`, `"w"`) }, wantReason: ReasonIntegrity},
		{name: "edited hash", damage: func(t *testing.T, file string) { rewrite(t, file, `sha256:aa`, `sha256:bb`) }, wantReason: ReasonIntegrity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, configDir := project(t)
			ctx := context.Background()
			o := opts(root, configDir, config.IndexCacheUser)
			first, err := Open(ctx, o)
			require.NoError(t, err)
			first.Store("AGENTS.md", 1, 1, entry("sha256:aa"))
			first.MarkClean("v")
			require.NoError(t, first.Save(ctx))
			tt.damage(t, filepath.Join(first.Location().Dir, indexFile))

			second, err := Open(ctx, o)

			require.NoError(t, err)
			_, ok := second.Lookup("AGENTS.md", 1, 1)
			assert.False(t, ok)
			assert.False(t, second.Clean("v"))
			assert.Equal(t, tt.wantReason, second.Stats().Discarded)
			require.NoError(t, second.Save(ctx), "the cache is rebuilt")
			third, err := Open(ctx, o)
			require.NoError(t, err)
			assert.Empty(t, third.Stats().Discarded)
		})
	}
}

func TestStore_ForgedVerdictFileWithAnotherKeyIsDiscarded(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	o := opts(root, configDir, config.IndexCacheUser)
	first, err := Open(ctx, o)
	require.NoError(t, err)
	first.MarkClean("v")
	require.NoError(t, first.Save(ctx))
	dir := first.Location().Dir
	// An attacker regenerates the whole cache with a key of their own.
	other := t.TempDir()
	forged := Options{Root: other, ConfigDir: filepath.Join(other, ".ai-rulez"), Mode: config.IndexCacheRepo, Key: testKey(2), Fingerprint: "fp1", Tree: "tree1"}
	require.NoError(t, os.MkdirAll(forged.ConfigDir, 0o755))
	fs, err := Open(ctx, forged)
	require.NoError(t, err)
	fs.MarkClean("forged")
	require.NoError(t, fs.Save(ctx))
	data, err := os.ReadFile(filepath.Join(fs.Location().Dir, indexFile))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, indexFile), data, 0o600))

	second, err := Open(ctx, o)

	require.NoError(t, err)
	assert.False(t, second.Clean("forged"))
	assert.Equal(t, ReasonIntegrity, second.Stats().Discarded)
}

func TestStore_SeveralStoresOnOneRepositoryMergeTheirEntries(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	o := opts(root, configDir, config.IndexCacheUser)
	a, err := Open(ctx, o)
	require.NoError(t, err)
	b, err := Open(ctx, o)
	require.NoError(t, err)
	a.Store("a/AGENTS.md", 1, 1, entry("sha256:aa"))
	a.MarkClean("va")
	b.Store("b/AGENTS.md", 2, 2, entry("sha256:bb"))
	b.MarkClean("vb")

	require.NoError(t, a.Save(ctx))
	require.NoError(t, b.Save(ctx))

	merged, err := Open(ctx, o)
	require.NoError(t, err)
	_, okA := merged.Lookup("a/AGENTS.md", 1, 1)
	_, okB := merged.Lookup("b/AGENTS.md", 2, 2)
	assert.True(t, okA)
	assert.True(t, okB)
	assert.True(t, merged.Clean("va"))
	assert.True(t, merged.Clean("vb"))
}

func TestStore_ConcurrentSavesLeaveAReadableCache(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	o := opts(root, configDir, config.IndexCacheUser)
	const writers = 8
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := Open(ctx, o)
			if err != nil {
				errs[i] = err
				return
			}
			s.Store(filepath.ToSlash(filepath.Join("pkg", string(rune('a'+i)), "AGENTS.md")), int64(i), int64(i), entry("sha256:aa"))
			s.MarkClean(string(rune('a' + i)))
			errs[i] = s.Save(ctx)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	final, err := Open(ctx, o)

	require.NoError(t, err)
	assert.Empty(t, final.Stats().Discarded)
	assert.Equal(t, writers, final.Stats().Entries)
	assert.Equal(t, writers, final.Stats().Verdicts)
}

func TestStore_OffAndNilStoresAreInert(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	off, err := Open(ctx, opts(root, configDir, config.IndexCacheOff))
	require.NoError(t, err)
	var none *Store

	for _, s := range []*Store{off, none} {
		s.Store("AGENTS.md", 1, 1, entry("sha256:aa"))
		s.MarkClean("v")
		_, ok := s.Lookup("AGENTS.md", 1, 1)
		assert.False(t, ok)
		assert.False(t, s.Clean("v"))
		require.NoError(t, s.Save(ctx))
		assert.Equal(t, KindOff, s.Location().Kind)
	}
}

func TestInspectAndClear(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	o := opts(root, configDir, config.IndexCacheUser)

	before, err := Inspect(ctx, o)
	require.NoError(t, err)
	assert.False(t, before.Exists)
	assert.Equal(t, KindUser, before.Kind)

	s, err := Open(ctx, o)
	require.NoError(t, err)
	s.Store("AGENTS.md", 1, 1, entry("sha256:aa"))
	s.Store("b/AGENTS.md", 1, 1, entry("sha256:bb"))
	s.MarkClean("v")
	require.NoError(t, s.Save(ctx))

	after, err := Inspect(ctx, o)
	require.NoError(t, err)
	assert.True(t, after.Exists)
	assert.Equal(t, s.Location().Dir, after.Dir)
	assert.Equal(t, 2, after.Entries)
	assert.Equal(t, 1, after.Verdicts)
	assert.Positive(t, after.Bytes)
	assert.Empty(t, after.Warnings)

	cleared, err := Clear(ctx, o)
	require.NoError(t, err)
	assert.Equal(t, []string{s.Location().Dir}, cleared.Removed)
	assert.Equal(t, after.Bytes, cleared.Bytes)
	_, statErr := os.Stat(s.Location().Dir)
	assert.True(t, os.IsNotExist(statErr))
	again, err := Clear(ctx, o)
	require.NoError(t, err)
	assert.Empty(t, again.Removed)
}

func TestInspect_ReportsACorruptCache(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	o := opts(root, configDir, config.IndexCacheUser)
	s, err := Open(ctx, o)
	require.NoError(t, err)
	s.MarkClean("v")
	require.NoError(t, s.Save(ctx))
	writeFile(t, filepath.Join(s.Location().Dir, indexFile), "{")

	rep, err := Inspect(ctx, o)

	require.NoError(t, err)
	require.Len(t, rep.Warnings, 1)
	assert.Contains(t, rep.Warnings[0], "corrupt")
}

func TestInspect_FindsTheProjectFallbackWhenTheUserCacheIsNotUsed(t *testing.T) {
	root, configDir := project(t)
	ctx := context.Background()
	repoOpts := opts(root, configDir, config.IndexCacheRepo)
	s, err := Open(ctx, repoOpts)
	require.NoError(t, err)
	s.MarkClean("v")
	require.NoError(t, s.Save(ctx))

	rep, err := Inspect(ctx, opts(root, configDir, config.IndexCacheUser))

	require.NoError(t, err)
	assert.True(t, rep.Exists)
	assert.Equal(t, KindRepo, rep.Kind)
	assert.Equal(t, 1, rep.Verdicts)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func rewrite(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), old)
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o600))
}

func TestLocate_WithoutASecretThereIsNoCache(t *testing.T) {
	root, configDir := project(t)
	o := opts(root, configDir, config.IndexCacheUser)
	o.Key = nil

	loc, err := Locate(context.Background(), o)
	require.NoError(t, err)
	s, err := Open(context.Background(), o)
	require.NoError(t, err)

	assert.Equal(t, KindOff, loc.Kind)
	assert.Contains(t, loc.Warning, "secret")
	assert.Equal(t, KindOff, s.Location().Kind)
}

func TestVerdictKey(t *testing.T) {
	a := SumOfContent("SKILL.md", []byte("one"))
	b := SumOfContent("notes.md", []byte("two"))
	base := VerdictKey("warn", []FileSum{a, b})

	tests := []struct {
		name string
		key  string
		same bool
	}{
		{name: "same inputs", key: VerdictKey("warn", []FileSum{a, b}), same: true},
		{name: "other level", key: VerdictKey("error", []FileSum{a, b})},
		{name: "other content", key: VerdictKey("warn", []FileSum{SumOfContent("SKILL.md", []byte("uno")), b})},
		{name: "other name", key: VerdictKey("warn", []FileSum{SumOfContent("renamed.md", []byte("one")), b})},
		{name: "other order", key: VerdictKey("warn", []FileSum{b, a})},
		{name: "fewer files", key: VerdictKey("warn", []FileSum{a})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.same, tt.key == base)
		})
	}
}

func TestSumOfHash_MatchesTheSumOfTheContent(t *testing.T) {
	content := []byte("# heading\n")
	want := SumOfContent("AGENTS.md", content)

	got, ok := SumOfHash("AGENTS.md", repoindex.Hash(content))

	require.True(t, ok)
	assert.Equal(t, want, got)
	for _, bad := range []string{"", "sha256:", "sha256:zz", "md5:abcd", "sha256:abcd"} {
		_, ok := SumOfHash("AGENTS.md", bad)
		assert.False(t, ok, bad)
	}
}

func TestLocate_CacheDirOptionReplacesTheUserCacheRoot(t *testing.T) {
	root, configDir := project(t)
	base := filepath.Join(t.TempDir(), "base")
	o := opts(root, configDir, config.IndexCacheUser)
	o.CacheDir = base

	loc, err := Locate(context.Background(), o)

	require.NoError(t, err)
	assert.Equal(t, KindUser, loc.Kind)
	assert.Equal(t, filepath.Join(base, "monorepo-index", RepoID(root)), loc.Dir)
}
