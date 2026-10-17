package repoindex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCache records what Build asks of a cache.
type fakeCache struct {
	items   map[string]CachedItem
	stored  map[string]CachedItem
	sizes   map[string]int64
	lookups []string
}

func newFakeCache() *fakeCache {
	return &fakeCache{items: map[string]CachedItem{}, stored: map[string]CachedItem{}, sizes: map[string]int64{}}
}

func (f *fakeCache) Lookup(file string, size, _ int64) (CachedItem, bool) {
	f.lookups = append(f.lookups, file)
	ci, ok := f.items[file]
	return ci, ok
}

func (f *fakeCache) Store(file string, size, _ int64, ci CachedItem) {
	f.stored[file] = ci
	f.sizes[file] = size
}

func TestBuild_CacheMissStoresTheSummaryWithoutTheBody(t *testing.T) {
	root := layout(t, map[string]string{"AGENTS.md": "# Root conventions\n\nsecret body text\n"})
	cache := newFakeCache()

	ix, err := Build(context.Background(), Options{Root: root, Cache: cache})

	require.NoError(t, err)
	require.Len(t, ix.Items(), 1)
	got := cache.stored["AGENTS.md"]
	assert.Equal(t, ix.Items()[0].Hash, got.Hash)
	assert.Equal(t, "Root conventions", got.Description)
	assert.NotContains(t, got.Description+got.Name+got.Hash, "secret body")
	assert.Positive(t, cache.sizes["AGENTS.md"])
}

func TestBuild_CacheHitSkipsReadingTheFile(t *testing.T) {
	root := layout(t, map[string]string{"AGENTS.md": "# Real heading\n"})
	cache := newFakeCache()
	cache.items["AGENTS.md"] = CachedItem{Name: "AGENTS.md", Description: "from the cache", Hash: "sha256:cached"}

	ix, err := Build(context.Background(), Options{Root: root, Cache: cache})

	require.NoError(t, err)
	require.Len(t, ix.Items(), 1)
	assert.Equal(t, "from the cache", ix.Items()[0].Description)
	assert.Equal(t, "sha256:cached", ix.Items()[0].Hash)
	assert.Empty(t, cache.stored, "a hit stores nothing new")
}

func TestBuild_CacheIsIgnoredWhenTheFileIsGoneOrReplaced(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  int
	}{
		{name: "removed", setup: func(t *testing.T, root string) { require.NoError(t, os.Remove(filepath.Join(root, "AGENTS.md"))) }},
		{name: "directory", setup: func(t *testing.T, root string) {
			require.NoError(t, os.Remove(filepath.Join(root, "AGENTS.md")))
			require.NoError(t, os.Mkdir(filepath.Join(root, "AGENTS.md"), 0o755))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := layout(t, map[string]string{"AGENTS.md": "# x\n"})
			cache := newFakeCache()
			cache.items["AGENTS.md"] = CachedItem{Name: "AGENTS.md", Hash: "sha256:cached"}
			files := []string{"AGENTS.md"}
			tt.setup(t, root)

			ix, err := Build(context.Background(), Options{Root: root, Cache: cache, Files: files})

			require.NoError(t, err)
			assert.Len(t, ix.Items(), tt.want)
		})
	}
}

func TestBuild_CachedGeneratedFlagLeavesTheItemOut(t *testing.T) {
	root := layout(t, map[string]string{"AGENTS.md": "# x\n", "pkg/AGENTS.md": "# y\n"})
	cache := newFakeCache()
	cache.items["AGENTS.md"] = CachedItem{Name: "AGENTS.md", Hash: "sha256:a", Generated: true}
	cache.items["pkg/AGENTS.md"] = CachedItem{Name: "AGENTS.md", Hash: "sha256:b"}

	ix, err := Build(context.Background(), Options{Root: root, Cache: cache})

	require.NoError(t, err)
	require.Len(t, ix.Items(), 1)
	assert.Equal(t, "pkg", ix.Items()[0].Scope)
	assert.Equal(t, 1, ix.GeneratedSkipped(KindInstruction))
}

func TestBuild_GeneratedCallbackStillJudgesACachedItemByPath(t *testing.T) {
	root := layout(t, map[string]string{"AGENTS.md": "# x\n"})
	cache := newFakeCache()
	cache.items["AGENTS.md"] = CachedItem{Name: "AGENTS.md", Hash: "sha256:a"}
	var sawContent []byte
	gen := func(_ string, content []byte) bool { sawContent = content; return true }

	ix, err := Build(context.Background(), Options{Root: root, Cache: cache, Generated: gen})

	require.NoError(t, err)
	assert.Empty(t, ix.Items(), "a manifest entry is judged at every build, whatever the cache says")
	assert.Nil(t, sawContent)
}

func TestBuild_ScreenKnownDecidesFromTheHashOrFallsBackToTheContent(t *testing.T) {
	tests := []struct {
		name      string
		known     func(it Item) (bool, string, bool)
		wantItems int
		wantRef   int
		wantRead  bool
	}{
		{name: "known keep", known: func(Item) (bool, string, bool) { return true, "", true }, wantItems: 1},
		{name: "known drop", known: func(Item) (bool, string, bool) { return false, "", true }},
		{name: "known refusal", known: func(Item) (bool, string, bool) { return false, "flagged", true }, wantRef: 1},
		{name: "unknown reads the content", known: func(Item) (bool, string, bool) { return false, "", false }, wantItems: 1, wantRead: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := layout(t, map[string]string{"AGENTS.md": "# x\n"})
			cache := newFakeCache()
			cache.items["AGENTS.md"] = CachedItem{Name: "AGENTS.md", Hash: "sha256:a"}
			read := false
			screen := func(Item, []byte) (bool, string) { read = true; return true, "" }

			ix, err := Build(context.Background(), Options{Root: root, Cache: cache, ScreenKnown: tt.known, Screen: screen})

			require.NoError(t, err)
			assert.Len(t, ix.Items(), tt.wantItems)
			assert.Len(t, ix.Refusals(), tt.wantRef)
			assert.Equal(t, tt.wantRead, read)
		})
	}
}

func TestBuild_TheModificationTimeIsPassedToTheCache(t *testing.T) {
	root := layout(t, map[string]string{"AGENTS.md": "# x\n"})
	when := time.Unix(1_700_000_000, 123)
	require.NoError(t, os.Chtimes(filepath.Join(root, "AGENTS.md"), when, when))
	var gotMTime int64
	cache := &mtimeCache{seen: &gotMTime}

	_, err := Build(context.Background(), Options{Root: root, Cache: cache})

	require.NoError(t, err)
	assert.NotZero(t, gotMTime)
	assert.InDelta(t, when.UnixNano(), gotMTime, float64(time.Second))
}

type mtimeCache struct{ seen *int64 }

func (m *mtimeCache) Lookup(string, int64, int64) (CachedItem, bool) { return CachedItem{}, false }
func (m *mtimeCache) Store(_ string, _, mtime int64, _ CachedItem)   { *m.seen = mtime }
