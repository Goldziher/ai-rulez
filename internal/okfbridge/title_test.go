package okfbridge_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// editTitle replaces the title of a concept file in an exported bundle.
func editTitle(t *testing.T, dir, rel, title string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	lines := strings.Split(string(data), "\n")
	done := false
	for i, l := range lines {
		if strings.HasPrefix(l, "title: ") && !done {
			lines[i] = "title: " + title
			done = true
		}
	}
	require.True(t, done, rel)
	require.NoError(t, os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644))
}

func TestEditedTitleSurvivesExportEditImportExport(t *testing.T) {
	paths := []string{"rules/testing.md", "context/architecture.md", "skills/release/SKILL.md", "agents/reviewer.md", "domains/backend/rules/db.md"}
	for _, rel := range paths {
		t.Run(rel, func(t *testing.T) {
			// Arrange
			root := sampleProject(t)
			dir := filepath.Join(t.TempDir(), "b")
			writeBundle(t, dir, exportProject(t, root).Files)
			editTitle(t, dir, rel, "Edited Title")
			b, err := okf.Load(os.DirFS(dir))
			require.NoError(t, err)
			// Act
			_, err = okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(root, ".ai-rulez"), Force: true, Scan: testScan})
			require.NoError(t, err)
			again := exportProject(t, root)
			// Assert
			found := false
			for _, f := range again.Files {
				if f.Path == rel {
					found = true
					assert.Contains(t, string(f.Data), "title: Edited Title")
				}
			}
			assert.True(t, found)
		})
	}
}

func TestEditedTitleIsAConflictWithoutForce(t *testing.T) {
	// Arrange
	root := sampleProject(t)
	dir := filepath.Join(t.TempDir(), "b")
	writeBundle(t, dir, exportProject(t, root).Files)
	// The source changes the title (okf.title) while the bundle edits it differently.
	write(t, root, ".ai-rulez/rules/plain.md", "---\nokf:\n  title: Source Title\n---\nNo frontmatter, just text.\n")
	editTitle(t, dir, "rules/plain.md", "Bundle Title")
	b, err := okf.Load(os.DirFS(dir))
	require.NoError(t, err)
	// Act
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(root, ".ai-rulez"), Scan: testScan})
	// Assert
	require.NoError(t, err)
	status := map[string]string{}
	for _, a := range res.Actions {
		status[a.Path] = a.Status
	}
	assert.Equal(t, okfbridge.StatusConflict, status["rules/plain.md"])
	assert.Contains(t, readFile(t, root, ".ai-rulez/rules/plain.md"), "Source Title", "never overwritten silently")
	assert.Contains(t, string(exportFile(t, root, "rules/plain.md")), "title: Source Title", "the sources win on export")
}

func exportFile(t *testing.T, root, rel string) []byte {
	t.Helper()
	for _, f := range exportProject(t, root).Files {
		if f.Path == rel {
			return f.Data
		}
	}
	t.Fatalf("%s not exported", rel)
	return nil
}
