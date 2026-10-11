package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsource"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// servedConfig is a project whose skills are all served, so a build has a
// non-empty catalog to cache.
const servedConfig = baseConfig + "\n[skills]\ndelivery = \"served\"\n"

func cacheProject(t *testing.T) string {
	t.Helper()
	return project(t, servedConfig, map[string]string{
		"skills/a/SKILL.md": skillFile("a", "Skill a", ""),
		"skills/b/SKILL.md": skillFile("b", "Skill b", ""),
	})
}

// The second start of the same project restores the catalog from the disk cache
// instead of rendering and scanning every skill again.
func TestServeCache_SecondStartRestoresTheCatalogWithoutRebuilding(t *testing.T) {
	root := cacheProject(t)
	cacheDir := t.TempDir()

	first := &testutil.LogRecorder{}
	srv1, err := (&ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: cacheDir, Log: first}).NewServer(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, catalogNames(srv1.Catalog()))
	assert.NotContains(t, first.String(), "Restored the served catalog from the cache", "the first start has nothing to restore")

	second := &testutil.LogRecorder{}
	srv2, err := (&ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: cacheDir, Log: second}).NewServer(context.Background())
	require.NoError(t, err)
	assert.Contains(t, second.String(), "Restored the served catalog from the cache")
	assert.Equal(t, []string{"a", "b"}, catalogNames(srv2.Catalog()))

	// The restored catalog is complete: lookups, resources and provenance.
	a, ok := srv2.Catalog().Lookup("a")
	require.True(t, ok)
	assert.Equal(t, "Skill a", a.Description)
	assert.Equal(t, "skill://a/SKILL.md", a.URI)
	assert.Equal(t, srv1.Catalog().byName["a"].Digest, a.Digest)
	_, ok = srv2.Catalog().File("skill://a/SKILL.md")
	assert.True(t, ok, "the resource index is rebuilt from the cache")
}

// A restored catalog can be admitted again, so an approval or signing key that
// expires later is still caught without a file change.
func TestServeCache_RestoredCatalogCanBeReAdmitted(t *testing.T) {
	root := cacheProject(t)
	cacheDir := t.TempDir()
	ctx := context.Background()

	first, err := (&ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: cacheDir}).build(ctx, buildOptions{admit: true})
	require.NoError(t, err)
	require.NotNil(t, first.readmit)

	second, err := (&ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: cacheDir}).build(ctx, buildOptions{admit: true})
	require.NoError(t, err)
	require.NotNil(t, second.readmit)
	readmitted := second.readmit()
	require.NotNil(t, readmitted)
	assert.Equal(t, catalogNames(second.catalog), catalogNames(readmitted))
}

// An edit changes the content digest, so the stale entry is not served.
func TestServeCache_ContentChangeInvalidatesTheEntry(t *testing.T) {
	root := cacheProject(t)
	cacheDir := t.TempDir()

	_, err := (&ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: cacheDir}).NewServer(context.Background())
	require.NoError(t, err)

	writeFile(t, root, ".ai-rulez/skills/a/SKILL.md", skillFile("a", "Skill a, edited", ""))
	rec := &testutil.LogRecorder{}
	srv, err := (&ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: cacheDir, Log: rec}).NewServer(context.Background())
	require.NoError(t, err)
	assert.NotContains(t, rec.String(), "Restored the served catalog from the cache")
	a, _ := srv.Catalog().Lookup("a")
	assert.Equal(t, "Skill a, edited", a.Description)
}

// A different view of the same files is a different catalog, so it does not
// restore another view's entry.
func TestServeCache_ADifferentViewDoesNotRestore(t *testing.T) {
	root := cacheProject(t)
	cacheDir := t.TempDir()

	_, err := (&ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: cacheDir}).NewServer(context.Background())
	require.NoError(t, err)

	rec := &testutil.LogRecorder{}
	srv, err := (&ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: cacheDir, Log: rec,
		Filter: SkillFilter{Deny: []string{"b"}}}).NewServer(context.Background())
	require.NoError(t, err)
	assert.NotContains(t, rec.String(), "Restored the served catalog from the cache")
	assert.Equal(t, []string{"a"}, catalogNames(srv.Catalog()))
}

// A corrupt or truncated entry counts as a miss and is rebuilt.
func TestServeCache_CorruptEntryIsRebuilt(t *testing.T) {
	root := cacheProject(t)
	cacheDir := t.TempDir()

	_, err := (&ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: cacheDir}).NewServer(context.Background())
	require.NoError(t, err)

	entry := filepath.Join(cacheDir, serveCacheFileName)
	require.NoError(t, os.WriteFile(entry, []byte(`{"version":1,"key":"x","payload":`), 0o600))

	rec := &testutil.LogRecorder{}
	srv, err := (&ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: cacheDir, Log: rec}).NewServer(context.Background())
	require.NoError(t, err)
	assert.NotContains(t, rec.String(), "Restored the served catalog from the cache")
	assert.Equal(t, []string{"a", "b"}, catalogNames(srv.Catalog()))

	// The rebuilt entry is usable: a third start restores it.
	again := &testutil.LogRecorder{}
	_, err = (&ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: cacheDir, Log: again}).NewServer(context.Background())
	require.NoError(t, err)
	assert.Contains(t, again.String(), "Restored the served catalog from the cache")
}

// An entry whose payload was edited fails its MAC and is rebuilt.
func TestServeCache_TamperedPayloadIsRejected(t *testing.T) {
	dir := t.TempDir()
	c := newServeCache(nil, dir, filepath.Join(dir, ".key"))
	require.NotNil(t, c)
	require.NoError(t, c.store("k", &serveCacheEntry{Raw: &catalogDTO{}, Admitted: &catalogDTO{}}))
	_, ok := c.load("k")
	require.True(t, ok)

	entry := filepath.Join(dir, serveCacheFileName)
	data, err := os.ReadFile(entry)
	require.NoError(t, err)
	var file serveCacheFile
	require.NoError(t, json.Unmarshal(data, &file))
	// A different payload under the old MAC: only the secret can produce a
	// matching MAC, so this must fail.
	file.Payload = json.RawMessage(`{"version":1,"key":"k","raw":{},"admitted":{"skills":[{"name":"evil"}]}}`)
	tampered, err := json.Marshal(file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(entry, tampered, 0o600))
	_, ok = c.load("k")
	assert.False(t, ok, "a forged entry must not load")
}

// A symlinked entry file is ignored, never read through.
func TestServeCache_SymlinkedEntryIsIgnored(t *testing.T) {
	dir := t.TempDir()
	c := newServeCache(nil, dir, filepath.Join(dir, ".key"))
	require.NoError(t, c.store("k", &serveCacheEntry{Raw: &catalogDTO{}, Admitted: &catalogDTO{}}))
	_, ok := c.load("k")
	require.True(t, ok)

	entry := filepath.Join(dir, serveCacheFileName)
	real := filepath.Join(dir, "real.json")
	require.NoError(t, os.Rename(entry, real))
	require.NoError(t, os.Symlink(real, entry))
	_, ok = c.load("k")
	assert.False(t, ok, "a symlinked entry is not read")
}

// The catalog survives a store/load round trip, including files, digests,
// refusals and scan reports.
func TestServeCache_CatalogRoundTrips(t *testing.T) {
	served := []generator.ServedSkill{
		servedSkill("a", "", "Skill a", []string{"a"}),
		servedSkill("b", "", "Skill b", []string{"b"}),
	}
	cat, err := BuildCatalog("p", "claude", served, SkillFilter{})
	require.NoError(t, err)
	cat.refused = map[string]Refusal{"z": {Name: "z", Code: "AR005", Reason: "blocked"}}
	cat.reports = []ScanReport{{Skill: "a", Level: "warn", Findings: []lint.Finding{{
		Code: "AR989", Name: "unscannable", Severity: lint.SeverityWarning, File: "skill://a/x.bin", Message: "binary",
	}}}}

	back := dtoToCatalog(catalogToDTO(cat))
	require.NotNil(t, back)
	assert.Equal(t, catalogToDTO(cat), catalogToDTO(back))
	assert.Equal(t, cat.Profile, back.Profile)
	require.Contains(t, back.byName, "a")
	assert.Equal(t, cat.byName["a"].Digest, back.byName["a"].Digest)
	require.Len(t, back.byName["a"].Files, len(cat.byName["a"].Files))
	r, ok := back.Refusal("z")
	require.True(t, ok)
	assert.Equal(t, "AR005", r.Code)
	require.Len(t, back.ScanReports(), 1)
	assert.Equal(t, "AR989", back.ScanReports()[0].Findings[0].Code)
}

// The key changes with the view and the filter, and with what each source
// resolved to, so a moved remote ref never reuses a stale entry.
func TestServeCacheKey_ViewFilterAndSourceResolutionChangeTheKey(t *testing.T) {
	fp := "abc"
	base := serveCacheKey(fp, &ServeSetup{}, nil)
	assert.Equal(t, base, serveCacheKey(fp, &ServeSetup{}, nil), "the same build has the same key")
	assert.NotEqual(t, base, serveCacheKey(fp, &ServeSetup{Role: "backend"}, nil))
	assert.NotEqual(t, base, serveCacheKey(fp, &ServeSetup{Filter: SkillFilter{Deny: []string{"x"}}}, nil))

	one := []*skillsource.Resolved{{Spec: skillsource.Spec{Name: "team"}, Commit: "c1", Digest: "sha256:1"}}
	two := []*skillsource.Resolved{{Spec: skillsource.Spec{Name: "team"}, Commit: "c2", Digest: "sha256:2"}}
	assert.Equal(t, serveCacheKey(fp, &ServeSetup{}, one), serveCacheKey(fp, &ServeSetup{}, one))
	assert.NotEqual(t, serveCacheKey(fp, &ServeSetup{}, one), serveCacheKey(fp, &ServeSetup{}, two))
}
