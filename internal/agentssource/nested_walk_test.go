package agentssource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func nestedDirs(found []nestedFile) []string {
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, f.dir)
	}
	return out
}

func TestNestedWalkStopsAtTheEntryBound(t *testing.T) {
	// Arrange: each directory holds one AGENTS.md, so the walk meets 2 entries per directory
	fsys := fstest.MapFS{}
	for _, d := range []string{"a", "b", "c", "d", "e", "f"} {
		fsys[d+"/AGENTS.md"] = &fstest.MapFile{Data: []byte("# " + d + "\n")}
	}
	tests := []struct {
		name         string
		maxEntries   int
		wantComplete bool
		wantFound    []string
	}{
		{"under the bound reads everything", 100, true, []string{"a", "b", "c", "d", "e", "f"}},
		{"exactly the entries of the tree is still complete", 13, true, []string{"a", "b", "c", "d", "e", "f"}},
		{"over the bound stops early", 5, false, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			found, complete := nestedWalker{maxEntries: tt.maxEntries}.find(fsys)

			// Assert
			assert.Equal(t, tt.wantComplete, complete)
			assert.Equal(t, tt.wantFound, nestedDirs(found))
		})
	}
}

func TestReadNestedWarnsWhenTheWalkIsCutShort(t *testing.T) {
	// Arrange
	root := t.TempDir()
	for _, d := range []string{"a", "b", "c", "d"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, d), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, d, "AGENTS.md"), []byte("# "+d+"\n\nTEXT "+d+"\n"), 0o600))
	}
	rec := &testutil.LogRecorder{}
	cfg := &config.Config{BaseDir: root, Host: ambient.Host{Log: rec}}

	// Act
	got := readNestedWith(context.Background(), cfg, os.DirFS(root), map[string]bool{}, nestedWalker{maxEntries: 4})

	// Assert
	require.Len(t, rec.Level("WARN"), 1)
	assert.Contains(t, rec.Level("WARN")[0], "stopped looking for nested AGENTS.md files")
	var names []string
	for _, f := range got {
		names = append(names, f.Name)
	}
	assert.Equal(t, []string{"agents-a"}, names)
}

func TestReadNestedDoesNotWarnUnderTheBound(t *testing.T) {
	// Arrange
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "a"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a", "AGENTS.md"), []byte("# a\n\nTEXT\n"), 0o600))
	rec := &testutil.LogRecorder{}
	cfg := &config.Config{BaseDir: root, Host: ambient.Host{Log: rec}}

	// Act
	got := readNested(context.Background(), cfg, os.DirFS(root), map[string]bool{})

	// Assert
	assert.Empty(t, rec.Level("WARN"))
	require.Len(t, got, 1)
	assert.Equal(t, "agents-a", got[0].Name)
}

func TestNestedWalkDoesNotFollowSymlinks(t *testing.T) {
	// Arrange
	root := t.TempDir()
	for rel, content := range map[string]string{
		"real/AGENTS.md":   "# Real\n",
		"shared/AGENTS.md": "# Target\n",
		"other/notes.md":   "# Notes\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(content), 0o600))
	}
	testutil.SymlinkOrSkip(t, "../real/AGENTS.md", filepath.Join(root, "other", "AGENTS.md"))
	testutil.SymlinkOrSkip(t, "shared", filepath.Join(root, "linked"))

	// Act
	found, complete := findNested(os.DirFS(root))

	// Assert: the symlinked file is not listed and the symlinked directory is not entered
	assert.True(t, complete)
	assert.Equal(t, []string{"real", "shared"}, nestedDirs(found))
}
