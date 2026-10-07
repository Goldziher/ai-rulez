package okfbridge_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func importBundle(t *testing.T, files map[string]string, opts okfbridge.ImportOptions) (*okfbridge.ImportResult, string) {
	t.Helper()
	cfgDir := filepath.Join(t.TempDir(), ".ai-rulez")
	opts.ConfigDir, opts.Scan = cfgDir, testScan
	res, err := okfbridge.Import(foreignBundle(files), opts)
	require.NoError(t, err)
	return res, cfgDir
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

func TestImportRewritesLinksBetweenImportedConcepts(t *testing.T) {
	// Arrange
	files := map[string]string{
		"decisions/use-go.md": "---\ntype: Decision\n---\nSee [abs](/glossary/terms.md#go), [rel](./other.md), [up](../runbooks/deploy.md \"t\").\n\n```\n[code](/glossary/terms.md)\n```\nInline `[c](/glossary/terms.md)` stays. [web](https://x.dev/a.md) [top](#top)\n",
		"decisions/other.md":  "---\ntype: Decision\n---\nx\n",
		"glossary/terms.md":   "---\ntype: Concept\n---\nterms\n",
		"runbooks/deploy.md":  "---\ntype: Runbook\ntitle: Deploy\n---\nsteps\n",
	}
	// Act
	res, cfgDir := importBundle(t, files, okfbridge.ImportOptions{})
	// Assert
	got := readFile(t, cfgDir, "rules/decisions-use-go.md")
	assert.Contains(t, got, "[abs](../context/glossary-terms.md#go)")
	assert.Contains(t, got, "[rel](decisions-other.md)")
	assert.Contains(t, got, `[up](../skills/runbooks-deploy/SKILL.md "t")`)
	assert.Contains(t, got, "```\n[code](/glossary/terms.md)\n```")
	assert.Contains(t, got, "Inline `[c](/glossary/terms.md)` stays.")
	assert.Contains(t, got, "[web](https://x.dev/a.md) [top](#top)")
	assert.Empty(t, res.Findings)
}

func TestImportReportsLinksToConceptsThatWereNotImported(t *testing.T) {
	// Arrange
	files := map[string]string{
		"a.md":      "---\ntype: Decision\n---\nline\n[gone](/missing.md) [data](data.csv) [bad](/broken.md)\n",
		"broken.md": "---\ntype: [\n---\n",
		"data.csv":  "x,y\n",
	}
	// Act
	res, cfgDir := importBundle(t, files, okfbridge.ImportOptions{})
	// Assert
	assert.Contains(t, readFile(t, cfgDir, "rules/a.md"), "[gone](/missing.md) [data](data.csv) [bad](/broken.md)", "left as written")
	var lossy []okf.Finding
	for _, f := range res.Findings {
		if f.Code == okf.CodeLossyMapping {
			lossy = append(lossy, f)
		}
	}
	require.Len(t, lossy, 3)
	assert.Equal(t, "a.md", lossy[0].Path)
	assert.Equal(t, 5, lossy[0].Line, "frontmatter lines count")
	assert.Contains(t, lossy[0].Message, "/missing.md")
	assert.Equal(t, okf.SeverityInfo, lossy[0].Severity)
}

func TestImportRewritesLinksIntoDomainAndSkillResources(t *testing.T) {
	// Arrange
	files := map[string]string{
		"deploy/SKILL.md":          "---\ntype: Playbook\nx-ai-rulez:\n  kind: skill\n  id: deploy\n---\nSee [ref](references/guide.md) and [rule](/rules/r.md).\n",
		"deploy/references/g.md":   "---\ntype: Reference\nx-ai-rulez:\n  kind: skill-resource\n  id: deploy\n  owner: skill\n  path: references/guide.md\n---\nBack to [skill](../SKILL.md) and [rule](/rules/r.md).\n",
		"rules/r.md":               "---\ntype: Decision\n---\nx\n",
		"deploy/scripts/run.sh":    "#!/bin/sh\n",
		"deploy/references/x.json": "{}\n",
	}
	// Act
	_, cfgDir := importBundle(t, files, okfbridge.ImportOptions{Domain: "ops"})
	// Assert
	assert.Contains(t, readFile(t, cfgDir, "domains/ops/skills/deploy/SKILL.md"), "[ref](references/guide.md)")
	assert.Contains(t, readFile(t, cfgDir, "domains/ops/skills/deploy/SKILL.md"), "[rule](../../rules/rules-r.md)")
	res := readFile(t, cfgDir, "domains/ops/skills/deploy/references/guide.md")
	assert.Contains(t, res, "[skill](../SKILL.md)")
	assert.Contains(t, res, "[rule](../../../rules/rules-r.md)")
}

func TestExportRewritesLinksBetweenExportedItems(t *testing.T) {
	// Arrange
	root := sampleProject(t)
	write(t, root, ".ai-rulez/rules/testing.md", "---\ndescription: How we test\n---\nSee [plain](plain.md#x), [arch](../context/architecture.md), [skill](../skills/release/SKILL.md),"+
		" [check](../skills/release/references/checklist.md), [db](../domains/backend/rules/db.md), [abs](/already/bundle.md), [src](../../src/main.go).\n\n```\n[code](plain.md)\n```\n")
	write(t, root, ".ai-rulez/domains/backend/rules/db.md", "---\ndescription: DB rules\n---\nBack to [testing](../../../rules/testing.md).\n")
	// Act
	res := exportProject(t, root)
	// Assert
	files := map[string]string{}
	for _, f := range res.Files {
		files[f.Path] = string(f.Data)
	}
	got := files["rules/testing.md"]
	assert.Contains(t, got, "[plain](/rules/plain.md#x)")
	assert.Contains(t, got, "[arch](/context/architecture.md)")
	assert.Contains(t, got, "[skill](/skills/release/SKILL.md)")
	assert.Contains(t, got, "[check](/skills/release/references/checklist.md)")
	assert.Contains(t, got, "[db](/domains/backend/rules/db.md)")
	assert.Contains(t, got, "[abs](/already/bundle.md)", "bundle-absolute links are not touched")
	assert.Contains(t, got, "[src](../../src/main.go)", "files outside the export are left alone")
	assert.Contains(t, got, "```\n[code](plain.md)\n```")
	assert.Contains(t, files["domains/backend/rules/db.md"], "[testing](/rules/testing.md)")
	require.Len(t, res.Notes, 1)
	assert.Contains(t, res.Notes[0], "../../src/main.go")
}

func TestExportThenImportKeepsSourceLinksStable(t *testing.T) {
	// Arrange
	root := sampleProject(t)
	write(t, root, ".ai-rulez/rules/testing.md", "---\ndescription: How we test\n---\n[plain](plain.md) [arch](../context/architecture.md) [db](../domains/backend/rules/db.md)\n")
	first := exportProject(t, root)
	dir := filepath.Join(t.TempDir(), "b")
	writeBundle(t, dir, first.Files)
	b, err := okf.Load(os.DirFS(dir))
	require.NoError(t, err)
	assert.Empty(t, b.Validate(), "no broken links in the exported bundle")
	fresh := t.TempDir()
	write(t, fresh, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"sample\"\npresets = [\"claude\"]\n")
	// Act
	_, err = okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(fresh, ".ai-rulez"), Scan: testScan})
	require.NoError(t, err)
	second := exportProject(t, fresh)
	// Assert
	assert.Contains(t, readFile(t, fresh, ".ai-rulez/rules/testing.md"), "[plain](plain.md) [arch](../context/architecture.md) [db](../domains/backend/rules/db.md)")
	require.Equal(t, len(first.Files), len(second.Files))
	for i := range first.Files {
		assert.Equal(t, string(first.Files[i].Data), string(second.Files[i].Data), first.Files[i].Path)
	}
}

func TestAcmeRetailLinksSurviveImportAndExport(t *testing.T) {
	// Arrange
	b, err := okf.Load(os.DirFS("../okf/testdata/acme_retail"))
	require.NoError(t, err)
	before := b.Validate()
	cfgDir := filepath.Join(t.TempDir(), ".ai-rulez")
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan})
	require.NoError(t, err)
	root := filepath.Dir(cfgDir)
	write(t, root, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"acme\"\npresets = [\"claude\"]\n")
	// Act
	out := exportProject(t, root)
	dir := filepath.Join(t.TempDir(), "b")
	writeBundle(t, dir, out.Files)
	nb, err := okf.Load(os.DirFS(dir))
	require.NoError(t, err)
	// Assert: every link that resolved in the original still resolves, to the same concept.
	broken := func(fs []okf.Finding) (n int) {
		for _, f := range fs {
			if f.Code == okf.CodeLinkBroken {
				n++
			}
		}
		return n
	}
	assert.Equal(t, broken(before), broken(nb.Validate()), "links to imported concepts are not broken")
	assert.Contains(t, bodyOf(t, nb, "context/metrics-gross-margin.md"), "[Revenue](/context/metrics-revenue.md)")
	assert.Contains(t, bodyOf(t, nb, "context/metrics-gross-margin.md"), "`gross-margin-legacy`](/context/metrics-gross-margin-legacy.md)")
	assert.Contains(t, bodyOf(t, nb, "context/metrics-gross-margin.md"), "gross_margin(period) = revenue(period) - cogs_full(period)", "code fence kept")
	for _, f := range res.Findings {
		if f.Code == okf.CodeLossyMapping {
			t.Logf("%s:%d %s", f.Path, f.Line, f.Message)
		}
	}

	// Export, import and export again is a fixed point.
	fresh := t.TempDir()
	write(t, fresh, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"acme\"\npresets = [\"claude\"]\n")
	_, err = okfbridge.Import(nb, okfbridge.ImportOptions{ConfigDir: filepath.Join(fresh, ".ai-rulez"), Scan: testScan})
	require.NoError(t, err)
	again := exportProject(t, fresh)
	require.Equal(t, len(out.Files), len(again.Files))
	for i := range out.Files {
		assert.Equal(t, string(out.Files[i].Data), string(again.Files[i].Data), out.Files[i].Path)
	}
}

func bodyOf(t *testing.T, b *okf.Bundle, p string) string {
	t.Helper()
	c, ok := b.Concepts[p]
	require.True(t, ok, p)
	return c.Body
}

func TestImportSkipsSymlinksWithAWarning(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.md"), []byte("---\ntype: Decision\n---\nx\n"), 0o644))
	testutil.SymlinkOrSkip(t, filepath.Join(dir, "real.md"), filepath.Join(dir, "link.md"))
	testutil.SymlinkOrSkip(t, t.TempDir(), filepath.Join(dir, "linkdir"))
	b, err := okf.Load(os.DirFS(dir))
	require.NoError(t, err)
	cfgDir := filepath.Join(t.TempDir(), ".ai-rulez")
	// Act
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan})
	// Assert
	require.NoError(t, err)
	assert.Len(t, res.Actions, 1)
	assert.Equal(t, []string{"link.md: symlinks are not followed and not allowed in a bundle", "linkdir: symlinks are not followed and not allowed in a bundle"}, res.Skipped)
	var warned []string
	for _, f := range res.Findings {
		if f.Code == okf.CodePathUnsafe {
			assert.Equal(t, okf.SeverityWarning, f.Severity)
			warned = append(warned, f.Path)
		}
	}
	assert.Equal(t, []string{"link.md", "linkdir"}, warned)
	_, statErr := os.Stat(filepath.Join(cfgDir, "rules/link.md"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestImportStillRefusesCaseCollisions(t *testing.T) {
	// Arrange
	b, err := okf.Load(fstest.MapFS{
		"A.md": {Data: []byte("---\ntype: Decision\n---\nx\n")},
		"a.md": {Data: []byte("---\ntype: Decision\n---\ny\n")},
	})
	require.NoError(t, err)
	// Act
	_, err = okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(t.TempDir(), ".ai-rulez"), Scan: testScan})
	// Assert
	assert.ErrorContains(t, err, "unsafe paths")
}
