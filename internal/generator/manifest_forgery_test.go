package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forgeManifestFiles appends entries to the committed manifest's files list, the
// way a hostile commit to .ai-rulez/.generated-manifest.json would.
func forgeManifestFiles(t *testing.T, dir string, entries ...string) {
	t.Helper()
	path := filepath.Join(dir, ".ai-rulez", generatedManifestName)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	files, _ := raw["files"].([]any)
	for _, entry := range entries {
		files = append(files, entry)
	}
	raw["files"] = files
	out, err := json.MarshalIndent(raw, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

func TestGenerate_ForgedManifestEntriesDoNotDeleteUserFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		rel   string
		body  string
		extra string // appended to the config
	}{
		{name: "unrelated file", rel: "secret.txt", body: "do not delete\n"},
		{name: "source file", rel: "a.go", body: "package a\n"},
		{name: "workflow beside a preset root", rel: ".github/workflows/ci.yml", body: "on: push\n"},
		{name: "hand-written skill under a generated dir", rel: ".claude/skills/mine/SKILL.md", body: "---\ndescription: mine\n---\nmine\n"},
		{name: "hand-written agent under a generated dir", rel: ".claude/agents/mine.md", body: "my agent\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			dir := narrowingProject(t)
			generateProfile(t, dir, "full")
			abs := filepath.Join(dir, filepath.FromSlash(tt.rel))
			require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
			require.NoError(t, os.WriteFile(abs, []byte(tt.body), 0o644))
			forgeManifestFiles(t, dir, tt.rel)

			// Act
			generateProfile(t, dir, "full")

			// Assert
			data, err := os.ReadFile(abs)
			require.NoError(t, err, "a file ai-rulez cannot prove it wrote must survive")
			assert.Equal(t, tt.body, string(data))
		})
	}
}

func TestGenerate_ForgedManifestDigestDoesNotAuthorizeUnknownPath(t *testing.T) {
	t.Parallel()

	// Arrange: the attacker also supplies a matching digest for the file.
	dir := narrowingProject(t)
	generateProfile(t, dir, "full")
	abs := filepath.Join(dir, "notes.json")
	body := []byte("{\"keep\":true}\n")
	require.NoError(t, os.WriteFile(abs, body, 0o644))
	path := filepath.Join(dir, ".ai-rulez", generatedManifestName)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	raw["files"] = append(raw["files"].([]any), "notes.json")
	raw["digests"] = map[string]string{"notes.json": fileDigest(body)}
	out, err := json.Marshal(raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))

	// Act
	generateProfile(t, dir, "full")

	// Assert
	assert.FileExists(t, abs)
}

func TestGenerate_StaleGeneratedFileIsStillRemoved(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := narrowingProject(t)
	generateProfile(t, dir, "full")
	dropped := filepath.Join(dir, ".claude", "skills", "dropped", "SKILL.md")
	require.FileExists(t, dropped)

	// Act
	generateProfile(t, dir, "narrow")

	// Assert
	assert.NoFileExists(t, dropped)
}

func TestGenerate_StaleHandEditedFileIsKept(t *testing.T) {
	t.Parallel()

	// Arrange: a generated file the user edited no longer proves ai-rulez wrote it.
	dir := narrowingProject(t)
	generateProfile(t, dir, "full")
	dropped := filepath.Join(dir, ".claude", "skills", "dropped", "SKILL.md")
	data, err := os.ReadFile(dropped)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dropped, append(data, []byte("\nmy own notes\n")...), 0o644))

	// Act
	generateProfile(t, dir, "narrow")

	// Assert
	assert.FileExists(t, dropped)
}

func TestClean_ForgedManifestEntryDoesNotDeleteUserFile(t *testing.T) {
	t.Parallel()

	// Arrange
	dir := narrowingProject(t)
	generateProfile(t, dir, "full")
	secret := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("keep\n"), 0o644))
	forgeManifestFiles(t, dir, "secret.txt")
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)

	// Act
	plan, err := NewGenerator(cfg).Clean("full", CleanOptions{})
	require.NoError(t, err)

	// Assert
	assert.FileExists(t, secret)
	assert.NotContains(t, plan.Files, secret)
}

func TestGenerate_HeaderlessGeneratedFileNeedsRecordedDigest(t *testing.T) {
	t.Parallel()

	// Arrange: header_hashes = none leaves no in-file proof; the manifest digest is it.
	dir := narrowingProject(t)
	cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
	cfgData, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, append(cfgData, []byte("\n[header]\nhashes = \"none\"\n")...), 0o644))
	generateProfile(t, dir, "full")
	dropped := filepath.Join(dir, ".claude", "skills", "dropped", "SKILL.md")
	require.FileExists(t, dropped)

	// Act
	generateProfile(t, dir, "narrow")

	// Assert
	assert.NoFileExists(t, dropped)
}

// hashedFixture returns content stamped with the Content-Hash of its own body, the
// shape of a file an earlier ai-rulez wrote; stale cleanup removes only such files
// (or ones whose digest the manifest recorded).
func hashedFixture(rel, content string) string {
	body := strings.TrimRight(stripHeader(content, rel), "\n")
	return injectContentHash(content, rel, templates.HashContent(body+"\n"))
}

// forgeMerged sets the committed manifest's merged claims.
func forgeMerged(t *testing.T, dir string, merged map[string][]map[string]any) {
	t.Helper()
	path := filepath.Join(dir, ".ai-rulez", generatedManifestName)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	raw["merged"] = merged
	out, err := json.MarshalIndent(raw, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

func TestGenerate_ForgedMergedClaimsDoNotTouchUserDocuments(t *testing.T) {
	t.Parallel()

	guard := func(v any) map[string]any {
		return map[string]any{"path": []string{"a"}, "equals": v, "sum": jsonmerge.Digest(v)}
	}
	tests := []struct {
		name   string
		rel    string
		body   string
		claims []map[string]any
	}{
		{name: "document no preset merges into", rel: "only.json", body: "{\"a\":1}\n", claims: []map[string]any{guard(1)}},
		{name: "package manifest", rel: "package.json", body: "{\"a\":1}\n", claims: []map[string]any{guard(1)}},
		{name: "known document, claim empties it", rel: ".claude/settings.json", body: "{\"a\":1}\n", claims: []map[string]any{guard(1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			dir := narrowingProject(t)
			generateProfile(t, dir, "full")
			abs := filepath.Join(dir, filepath.FromSlash(tt.rel))
			require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
			require.NoError(t, os.WriteFile(abs, []byte(tt.body), 0o644))
			forgeMerged(t, dir, map[string][]map[string]any{tt.rel: tt.claims})

			// Act
			generateProfile(t, dir, "full")
			cfg, err := config.LoadConfig(context.Background(), dir)
			require.NoError(t, err)
			_, cleanErr := NewGenerator(cfg).Clean("full", CleanOptions{})
			require.NoError(t, cleanErr)

			// Assert
			require.FileExists(t, abs, "a forged claim must never delete a document")
			if tt.rel != ".claude/settings.json" {
				data, readErr := os.ReadFile(abs)
				require.NoError(t, readErr)
				assert.Equal(t, tt.body, string(data))
			}
		})
	}
}
