package okfbridge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportKeepsItemsNamedLikeReservedOKFFiles(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".ai-rulez/config.yaml", "version: \"4.0\"\nname: r\npresets:\n  - claude\n")
	write(t, root, ".ai-rulez/rules/index.md", "RULE INDEX BODY\n")
	write(t, root, ".ai-rulez/context/index.md", "CONTEXT INDEX BODY\n")
	write(t, root, ".ai-rulez/context/log.md", "CONTEXT LOG BODY\n")
	first := exportProject(t, root)

	seen := map[string]int{}
	for _, f := range first.Files {
		seen[f.Path]++
	}
	for p, n := range seen {
		assert.Equal(t, 1, n, "duplicate path %s", p)
	}
	dir := t.TempDir() + "/b"
	writeBundle(t, dir, first.Files)
	b, err := okf.Load(os.DirFS(dir))
	require.NoError(t, err)
	assert.Empty(t, b.Validate(), "bundle stays conformant")
	assert.Equal(t, 3, len(b.Concepts), "all three items are concepts")

	fresh := t.TempDir()
	write(t, fresh, ".ai-rulez/config.yaml", "version: \"4.0\"\nname: r\npresets:\n  - claude\n")
	_, err = okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(fresh, ".ai-rulez"), Scan: testScan})
	require.NoError(t, err)
	for rel, body := range map[string]string{"rules/index.md": "RULE INDEX BODY\n", "context/index.md": "CONTEXT INDEX BODY\n", "context/log.md": "CONTEXT LOG BODY\n"} {
		got, err := os.ReadFile(filepath.Join(fresh, ".ai-rulez", rel))
		require.NoError(t, err, rel)
		assert.Equal(t, body, string(got), rel)
	}
	second := exportProject(t, fresh)
	require.Equal(t, len(first.Files), len(second.Files))
	for i := range first.Files {
		assert.Equal(t, string(first.Files[i].Data), string(second.Files[i].Data), first.Files[i].Path)
	}
}
