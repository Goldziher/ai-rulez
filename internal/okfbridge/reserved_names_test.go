package okfbridge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
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

func TestExportSkipsItemsMergedInFromIncludes(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".ai-rulez/config.yaml", "version: \"4.0\"\nname: r\npresets:\n  - claude\n")
	write(t, root, ".ai-rulez/rules/own.md", "own rule\n")
	tree := loadTree(t, root)
	elsewhere := t.TempDir()
	tree.Rules = append(tree.Rules, config.ContentFile{Name: "always-use-x", Path: filepath.Join(elsewhere, "rules", "always-use-x.md"), Content: "from an include\n"})

	res, err := okfbridge.Export(tree, okfbridge.ExportOptions{LocalDir: filepath.Join(root, ".ai-rulez")})
	require.NoError(t, err)
	var paths []string
	for _, f := range res.Files {
		paths = append(paths, f.Path)
	}
	assert.Contains(t, paths, "rules/own.md")
	assert.NotContains(t, paths, "rules/always-use-x.md")
	assert.Contains(t, res.Notes[0], "includes")
	assert.Equal(t, 1, res.Counts[okfbridge.KindRule])

	all, err := okfbridge.Export(tree, okfbridge.ExportOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, all.Counts[okfbridge.KindRule], "no filter without a local dir")
}

func TestRoundTripIsStableForNamelessSkillsAndUppercaseResources(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".ai-rulez/config.yaml", "version: \"4.0\"\nname: r\npresets:\n  - claude\n")
	write(t, root, ".ai-rulez/skills/plain/SKILL.md", "---\ndescription: No name key\n---\n\nBody.\n")
	write(t, root, ".ai-rulez/skills/plain/references/upper.MD", "# Upper\n")
	write(t, root, ".ai-rulez/skills/plain/references/lower.md", "# Lower\n")

	cur := root
	var prev *okfbridge.ExportResult
	for cycle := 0; cycle < 3; cycle++ {
		res := exportProject(t, cur)
		if prev != nil {
			require.Equal(t, len(prev.Files), len(res.Files), "cycle %d", cycle)
			for i := range prev.Files {
				assert.Equal(t, prev.Files[i].Path, res.Files[i].Path, "cycle %d", cycle)
				assert.Equal(t, string(prev.Files[i].Data), string(res.Files[i].Data), "cycle %d %s", cycle, prev.Files[i].Path)
			}
		}
		prev = res
		dir := t.TempDir() + "/b"
		writeBundle(t, dir, res.Files)
		b, err := okf.Load(os.DirFS(dir))
		require.NoError(t, err)
		next := t.TempDir()
		write(t, next, ".ai-rulez/config.yaml", "version: \"4.0\"\nname: r\npresets:\n  - claude\n")
		_, err = okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(next, ".ai-rulez"), Scan: testScan})
		require.NoError(t, err)
		cur = next
	}
	got, err := os.ReadFile(filepath.Join(cur, ".ai-rulez/skills/plain/references/upper.MD"))
	require.NoError(t, err)
	assert.Equal(t, "# Upper\n", string(got))
}
