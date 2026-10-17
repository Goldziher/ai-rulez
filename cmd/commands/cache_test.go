package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/indexcache"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const cacheProjectConfig = "version = \"5.0\"\nname = \"cached\"\npresets = [\"claude\"]\n"

// cacheProject is a project with a populated index cache in the isolated user
// cache. It returns the project root.
func cacheProject(t *testing.T) string {
	t.Helper()
	testutil.IsolateHome(t)
	testutil.ScrubCIEnv(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), cacheProjectConfig)
	chdir(t, root)
	cfg := loadCacheConfig(t)
	opts, _ := (&mcp.ServeSetup{}).IndexCacheOptions(t.Context(), cfg, true)
	store, err := indexcache.Open(t.Context(), opts)
	require.NoError(t, err)
	store.Store("AGENTS.md", 1, 1, repoindex.CachedItem{Name: "AGENTS.md", Hash: "sha256:aa"})
	store.Store("pkg/AGENTS.md", 1, 1, repoindex.CachedItem{Name: "AGENTS.md", Hash: "sha256:bb"})
	store.MarkClean("verdict")
	require.NoError(t, store.Save(t.Context()))
	return root
}

func TestCacheStatus_ReportsTheLocationSizeAndEntries(t *testing.T) {
	cacheProject(t)

	res := runCLI(t, "cache", "status", "--format", "json")

	require.Equal(t, 0, res.code, res.stderr)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &doc), res.stdout)
	assert.EqualValues(t, 1, doc["schema_version"])
	assert.Equal(t, "user", doc["mode"])
	assert.Equal(t, "user", doc["kind"])
	assert.Equal(t, true, doc["exists"])
	assert.EqualValues(t, 2, doc["entries"])
	assert.EqualValues(t, 1, doc["verdicts"])
	assert.Positive(t, doc["bytes"])
	assert.Contains(t, doc["dir"], "monorepo-index")
	assert.Empty(t, doc["warnings"])
}

func TestCacheStatus_TextNamesWhereTheCacheIs(t *testing.T) {
	cacheProject(t)

	res := runCLI(t, "cache", "status")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, "monorepo-index")
	assert.Contains(t, res.stdout, "2 entries")
	assert.Contains(t, res.stdout, "1 verdict")
}

func TestCacheStatus_WithoutACacheSaysSo(t *testing.T) {
	testutil.IsolateHome(t)
	testutil.ScrubCIEnv(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), cacheProjectConfig)
	chdir(t, root)

	res := runCLI(t, "cache", "status")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, "No index cache")
}

func TestCacheStatus_ReportsACorruptCache(t *testing.T) {
	cacheProject(t)
	res := runCLI(t, "cache", "status", "--format", "json")
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &doc))
	require.NoError(t, os.WriteFile(filepath.Join(doc["dir"].(string), "index.json"), []byte("{"), 0o600))

	res = runCLI(t, "cache", "status", "--format", "json")

	require.Equal(t, 0, res.code, res.stderr)
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &doc))
	warnings, _ := doc["warnings"].([]any)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "corrupt")
}

func TestCacheClear(t *testing.T) {
	tests := []struct {
		name string
		args []string
		text []string
	}{
		{name: "text", args: []string{"cache", "clear"}, text: []string{"Removed", "monorepo-index"}},
		{name: "json", args: []string{"cache", "clear", "--format", "json"}},
		{name: "all projects", args: []string{"cache", "clear", "--all"}, text: []string{"Removed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheProject(t)
			before := runCLI(t, "cache", "status", "--format", "json")
			var doc map[string]any
			require.NoError(t, json.Unmarshal([]byte(before.stdout), &doc))
			dir := doc["dir"].(string)

			res := runCLI(t, tt.args...)

			require.Equal(t, 0, res.code, res.stderr)
			for _, want := range tt.text {
				assert.Contains(t, res.stdout, want)
			}
			if strings.Contains(strings.Join(tt.args, " "), "json") {
				var cleared map[string]any
				require.NoError(t, json.Unmarshal([]byte(res.stdout), &cleared), res.stdout)
				assert.EqualValues(t, 1, cleared["schema_version"])
				assert.Equal(t, []any{dir}, cleared["removed"])
				assert.Positive(t, cleared["bytes"])
			}
			_, err := os.Stat(dir)
			assert.True(t, os.IsNotExist(err), "the cache directory is gone")
			after := runCLI(t, "cache", "status")
			assert.Contains(t, after.stdout, "No index cache")
		})
	}
}

func TestCacheClear_WithNothingToClearSaysSo(t *testing.T) {
	testutil.IsolateHome(t)
	testutil.ScrubCIEnv(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), cacheProjectConfig)
	chdir(t, root)

	res := runCLI(t, "cache", "clear")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, "Nothing to remove")
}

func TestCache_WorksWithoutAProjectConfiguration(t *testing.T) {
	testutil.IsolateHome(t)
	testutil.ScrubCIEnv(t)
	chdir(t, t.TempDir())

	status := runCLI(t, "cache", "status")
	clear := runCLI(t, "cache", "clear")

	assert.Equal(t, 0, status.code, status.stderr)
	assert.Equal(t, 0, clear.code, clear.stderr)
}

func loadCacheConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig(t.Context(), ".", config.WithoutLocal(), config.WithoutRemote())
	require.NoError(t, err)
	return cfg
}
