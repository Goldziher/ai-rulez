package doctor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/indexcache"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const indexCacheBase = baseConfig + "\n[mcp]\ndiscover_skills = \"repo\"\n"

func cacheOptionsFor(cacheDir string) func(*config.Config) indexcache.Options {
	return func(cfg *config.Config) indexcache.Options {
		return indexcache.Options{
			Root: cfg.BaseDir, ConfigDir: cfg.ConfigDir, CacheDir: cacheDir, Mode: cfg.IndexCacheMode(),
			Key: bytes.Repeat([]byte{7}, 32), Fingerprint: "fp", Tree: "t",
		}
	}
}

func indexCacheFindings(t *testing.T, dir string, cacheDir string) []Finding {
	t.Helper()
	opts := Options{Load: loader(dir), IndexCache: cacheOptionsFor(cacheDir)}
	return byCheck(Run(context.Background(), opts), CheckIndexCache)
}

func populate(t *testing.T, dir, cacheDir string) string {
	t.Helper()
	cfg, err := loader(dir)(context.Background())
	require.NoError(t, err)
	store, err := indexcache.Open(context.Background(), cacheOptionsFor(cacheDir)(cfg))
	require.NoError(t, err)
	store.Store("AGENTS.md", 1, 1, repoindex.CachedItem{Name: "AGENTS.md", Hash: "sha256:aa"})
	require.NoError(t, store.Save(context.Background()))
	return store.Location().Dir
}

func TestCheckIndexCache_SilentWhenDiscoveryIsOffAndNoCacheExists(t *testing.T) {
	testutil.IsolateHome(t)
	dir := project(t, map[string]string{".ai-rulez/config.toml": baseConfig})

	assert.Empty(t, indexCacheFindings(t, dir, t.TempDir()))
}

func TestCheckIndexCache_SilentWithoutTheHook(t *testing.T) {
	dir := project(t, map[string]string{".ai-rulez/config.toml": indexCacheBase})

	got := byCheck(Run(context.Background(), Options{Load: loader(dir)}), CheckIndexCache)

	assert.Empty(t, got)
}

func TestCheckIndexCache_ReportsLocationAndSize(t *testing.T) {
	testutil.IsolateHome(t)
	dir := project(t, map[string]string{".ai-rulez/config.toml": indexCacheBase})
	cacheDir := t.TempDir()
	where := populate(t, dir, cacheDir)

	got := indexCacheFindings(t, dir, cacheDir)

	require.Len(t, got, 1)
	assert.Equal(t, SeverityInfo, got[0].Severity)
	assert.Contains(t, got[0].Message, where)
	assert.Contains(t, got[0].Message, "user")
	assert.Contains(t, got[0].Message, "1 entries")
}

func TestCheckIndexCache_WarnsOnACorruptIndex(t *testing.T) {
	testutil.IsolateHome(t)
	dir := project(t, map[string]string{".ai-rulez/config.toml": indexCacheBase})
	cacheDir := t.TempDir()
	where := populate(t, dir, cacheDir)
	require.NoError(t, os.WriteFile(filepath.Join(where, "index.json"), []byte("{"), 0o600))

	got := indexCacheFindings(t, dir, cacheDir)

	var warnings []Finding
	for _, f := range got {
		if f.Severity == SeverityWarning {
			warnings = append(warnings, f)
		}
	}
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Message, "corrupt")
	assert.NotEmpty(t, warnings[0].Hint)
}

func TestCheckIndexCache_WarnsWhenTheUserCacheIsNotWritable(t *testing.T) {
	testutil.IsolateHome(t)
	dir := project(t, map[string]string{".ai-rulez/config.toml": indexCacheBase})
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	got := indexCacheFindings(t, dir, filepath.Join(blocker, "cache"))

	var warnings []Finding
	for _, f := range got {
		if f.Severity == SeverityWarning {
			warnings = append(warnings, f)
		}
	}
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Message, "not writable")
}
