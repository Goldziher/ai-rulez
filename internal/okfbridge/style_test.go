package okfbridge_test

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exportStyle(t *testing.T, root, style string) *okfbridge.ExportResult {
	t.Helper()
	res, err := okfbridge.Export(loadTree(t, root), okfbridge.ExportOptions{IndexStyle: style})
	require.NoError(t, err)
	return res
}

func byPath(files []okf.File) map[string]string {
	out := map[string]string{}
	for _, f := range files {
		out[f.Path] = string(f.Data)
	}
	return out
}

func TestIndexStylesDifferOnlyInIndexFiles(t *testing.T) {
	// Arrange
	root := sampleProject(t)
	// Act
	body := byPath(exportStyle(t, root, okf.StyleBody).Files)
	front := byPath(exportStyle(t, root, okf.StyleFrontmatter).Files)
	// Assert
	assert.Equal(t, body, byPath(exportStyle(t, root, "").Files), "the default is the body style")
	require.Equal(t, len(body), len(front))
	for p, data := range body {
		if path.Base(p) == okf.IndexFile {
			assert.NotEqual(t, data, front[p], p)
			continue
		}
		assert.Equal(t, data, front[p], p)
	}
}

// Regenerate with: UPDATE_GOLDEN=1 go test ./internal/okfbridge -run IndexGolden
func TestFrontmatterIndexGolden(t *testing.T) {
	// Arrange
	res := exportStyle(t, sampleProject(t), okf.StyleFrontmatter)
	golden := filepath.Join("testdata", "golden-frontmatter")
	var got []okf.File
	for _, f := range res.Files {
		if path.Base(f.Path) == okf.IndexFile {
			got = append(got, f)
		}
	}
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.RemoveAll(golden))
		for _, f := range got {
			p := filepath.Join(golden, filepath.FromSlash(f.Path))
			require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
			require.NoError(t, os.WriteFile(p, f.Data, 0o644))
		}
	}
	// Act and assert
	var want, listed []string
	require.NoError(t, filepath.WalkDir(golden, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(golden, p)
		want = append(want, filepath.ToSlash(rel))
		return relErr
	}))
	sort.Strings(want)
	for _, f := range got {
		listed = append(listed, f.Path)
		data, err := os.ReadFile(filepath.Join(golden, filepath.FromSlash(f.Path)))
		require.NoError(t, err, f.Path)
		assert.Equal(t, string(data), string(f.Data), f.Path)
	}
	assert.Equal(t, want, listed)
}

func TestBothStylesRoundTripTheSameSourceTree(t *testing.T) {
	for _, style := range []string{okf.StyleBody, okf.StyleFrontmatter} {
		t.Run(style, func(t *testing.T) {
			// Arrange
			root := sampleProject(t)
			first := exportStyle(t, root, style)
			dir := filepath.Join(t.TempDir(), "b")
			writeBundle(t, dir, first.Files)
			b, err := okf.Load(os.DirFS(dir))
			require.NoError(t, err)
			fresh := t.TempDir()
			write(t, fresh, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"sample\"\npresets = [\"claude\"]\n")
			// Act
			res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(fresh, ".ai-rulez"), Scan: testScan})
			require.NoError(t, err)
			second := exportStyle(t, fresh, style)
			// Assert
			assert.Equal(t, style, res.IndexStyle)
			assert.Equal(t, byPath(first.Files), byPath(second.Files))
			for _, f := range b.Validate() {
				assert.NotEqual(t, okf.SeverityError, f.Severity, "%s %s %s", f.Code, f.Path, f.Message)
				assert.NotEqual(t, okf.SeverityWarning, f.Severity, "%s %s %s", f.Code, f.Path, f.Message)
			}
		})
	}
}

func TestImportYieldsTheSameTreeForEitherIndexStyle(t *testing.T) {
	// Arrange
	root := sampleProject(t)
	trees := map[string]map[string]string{}
	for _, style := range []string{okf.StyleBody, okf.StyleFrontmatter} {
		dir := filepath.Join(t.TempDir(), "b")
		writeBundle(t, dir, exportStyle(t, root, style).Files)
		b, err := okf.Load(os.DirFS(dir))
		require.NoError(t, err)
		cfgDir := filepath.Join(t.TempDir(), ".ai-rulez")
		// Act
		_, err = okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan})
		require.NoError(t, err)
		tree := map[string]string{}
		require.NoError(t, filepath.WalkDir(cfgDir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(cfgDir, p)
			data, readErr := os.ReadFile(p)
			tree[filepath.ToSlash(rel)] = string(data)
			return readErr
		}))
		trees[style] = tree
	}
	// Assert
	assert.NotEmpty(t, trees[okf.StyleBody])
	assert.Equal(t, trees[okf.StyleBody], trees[okf.StyleFrontmatter])
}

func TestExportRejectsAnUnknownIndexStyle(t *testing.T) {
	_, err := okfbridge.Export(&config.ContentTree{}, okfbridge.ExportOptions{IndexStyle: "yaml"})
	assert.ErrorContains(t, err, `unknown index style "yaml"`)
}
