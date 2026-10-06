package importer

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitSources(t *testing.T) {
	tests := []struct {
		name      string
		spec      string
		wantFrom  []string
		wantPaths []string
	}{
		{name: "auto", spec: "auto", wantFrom: []string{"auto"}},
		{name: "importer names", spec: "rulesync, apm", wantFrom: []string{"rulesync", "apm"}},
		{name: "project paths are native paths", spec: ".claude,.cursor,CLAUDE.md", wantFrom: []string{"native"}, wantPaths: []string{".claude", ".cursor", "CLAUDE.md"}},
		{name: "a mix keeps both", spec: "rulesync,.claude", wantFrom: []string{"rulesync", "native"}, wantPaths: []string{".claude"}},
		{name: "native itself is unrestricted", spec: "native,.claude", wantFrom: []string{"native"}},
		{name: "empty tokens are ignored", spec: " , ,auto,", wantFrom: []string{"auto"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, paths := SplitSources(tt.spec)
			assert.Equal(t, tt.wantFrom, from)
			assert.Equal(t, tt.wantPaths, paths)
		})
	}
}

func TestRestrictedFS_ShowsOnlyTheNamedPaths(t *testing.T) {
	// Arrange
	fsys := restrict(mapFS(map[string]string{
		"CLAUDE.md":              "a",
		"GEMINI.md":              "b",
		".claude/agents/a.md":    "c",
		".claude/skills/s/x.md":  "d",
		".cursor/rules/r.mdc":    "e",
		".cursorrules":           "f",
		"docs/.claude/hidden.md": "g",
	}), []string{".claude/agents", "CLAUDE.md", "./.cursor"})

	// Act
	var seen []string
	require.NoError(t, fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			seen = append(seen, p)
		}
		return err
	}))

	// Assert
	assert.ElementsMatch(t, []string{"CLAUDE.md", ".claude/agents/a.md", ".cursor/rules/r.mdc"}, seen)
	_, err := fs.ReadFile(fsys, "GEMINI.md")
	assert.ErrorIs(t, err, fs.ErrNotExist)
	_, err = fs.Stat(fsys, ".claude/skills")
	assert.ErrorIs(t, err, fs.ErrNotExist)
	_, err = fs.Stat(fsys, ".claude")
	assert.NoError(t, err, "the directory on the way to an allowed path is visible")
}

func TestRestrictedFS_NoPathsLeavesItUnrestricted(t *testing.T) {
	inner := mapFS(map[string]string{"a.md": "x"})

	assert.Equal(t, fs.FS(inner), restrict(inner, nil))
}

func TestRestrictedFS_StillRefusesSymlinks(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	require.NoError(t, os.WriteFile(outside, []byte("outside\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude", "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "agents", "ok.md"), []byte("Fine.\n"), 0o644))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(dir, ".claude", "agents", "link.md"))

	// Act
	p := planOf(t, nativeImporter{}, restrict(os.DirFS(dir), []string{".claude"}), Options{})

	// Assert
	assert.Equal(t, []string{"agents/ok.md"}, itemRels(p))
	assert.NotNil(t, findingFor(p, StatusDropped, ".claude/agents/link.md", ""), "the link is reported, not followed")
}
