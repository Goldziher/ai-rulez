package okfbridge_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testScan(texts map[string]string) []okfbridge.SecurityFinding {
	var out []okfbridge.SecurityFinding
	for _, f := range lint.ScanTexts(nil, texts) {
		out = append(out, okfbridge.SecurityFinding{Code: f.Code, Severity: string(f.Severity), File: f.File, Line: f.Line, Message: f.Message})
	}
	return out
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func sampleProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	ar := ".ai-rulez/"
	write(t, root, ar+"config.toml", "version = \"5.0\"\nname = \"sample\"\npresets = [\"claude\"]\n")
	write(t, root, ar+"rules/testing.md", "---\npriority: high\nglobs:\n  - \"**/*_test.go\"\ndescription: How we test\nowner: platform-team\nversion: 1.2\n---\n\n# Testing\n\nWrite table tests.\n")
	write(t, root, ar+"rules/plain.md", "No frontmatter, just text.\n")
	write(t, root, ar+"context/architecture.md", "---\ndescription: System layout\nokf:\n  tags: [arch, core]\n  status: stable\n---\nMonolith with plugins.\n")
	write(t, root, ar+"skills/release/SKILL.md", "---\nname: release\ndescription: Cut a release\nallowed-tools: Bash(git tag:*)\n---\n\nSteps go here.\n")
	write(t, root, ar+"skills/release/references/checklist.md", "---\ndescription: Checklist\n---\n- tag\n- publish\n")
	write(t, root, ar+"skills/release/references/index.md", "# reserved name\n")
	write(t, root, ar+"skills/release/scripts/tag.sh", "#!/bin/sh\necho tag\n")
	require.NoError(t, os.Chmod(filepath.Join(root, ar, "skills/release/scripts/tag.sh"), 0o755))
	write(t, root, ar+"agents/reviewer.md", "---\ndescription: Reviews code\ntools: [Read]\n---\nYou review.\n")
	write(t, root, ar+"commands/ship.md", "---\ndescription: Ship it\n---\nRun ship.\n")
	write(t, root, ar+"checks/no-todo.md", "---\nseverity: high\ndescription: No TODOs\n---\nFlag TODO comments.\n")
	write(t, root, ar+"domains/backend/rules/db.md", "---\ndescription: DB rules\n---\nUse migrations.\n")
	return root
}

func loadTree(t *testing.T, root string) *config.ContentTree {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	require.NotNil(t, cfg.Content)
	return cfg.Content
}

func exportProject(t *testing.T, root string) *okfbridge.ExportResult {
	t.Helper()
	res, err := okfbridge.Export(loadTree(t, root), okfbridge.ExportOptions{})
	require.NoError(t, err)
	return res
}

func writeBundle(t *testing.T, dir string, files []okf.File) {
	t.Helper()
	require.NoError(t, okf.WriteFiles(dir, files, true))
}

func TestExportProducesConformantBundle(t *testing.T) {
	res := exportProject(t, sampleProject(t))
	dir := t.TempDir() + "/bundle"
	writeBundle(t, dir, res.Files)
	b, err := okf.Load(os.DirFS(dir))
	require.NoError(t, err)
	assert.Empty(t, b.Validate(), "exported bundle must be clean")

	paths := map[string]bool{}
	for _, f := range res.Files {
		paths[f.Path] = true
	}
	for _, want := range []string{
		"index.md", "rules/testing.md", "rules/plain.md", "context/architecture.md",
		"skills/release/SKILL.md", "skills/release/references/checklist.md",
		"skills/release/references/index_.md", "skills/release/scripts/tag.sh",
		"agents/reviewer.md", "commands/ship.md", "checks/no-todo.md",
		"domains/backend/rules/db.md", "domains/backend/rules/index.md",
	} {
		assert.True(t, paths[want], want)
	}
	assert.Equal(t, 3, res.Counts[okfbridge.KindRule], "two root rules and one domain rule")
	rule := b.Concepts["rules/testing.md"]
	assert.Equal(t, "Decision", rule.Type())
	assert.Equal(t, "How we test", rule.Description())
	assert.Equal(t, "Playbook", b.Concepts["skills/release/SKILL.md"].Type())
	assert.Equal(t, "Reference", b.Concepts["agents/reviewer.md"].Type())
}

func TestExportIsDeterministic(t *testing.T) {
	root := sampleProject(t)
	a, b := exportProject(t, root), exportProject(t, root)
	assert.Equal(t, a.Files, b.Files)
}

func TestRoundTripIsByteIdentical(t *testing.T) {
	root := sampleProject(t)
	first := exportProject(t, root)
	bundleDir := t.TempDir() + "/b"
	writeBundle(t, bundleDir, first.Files)

	b, err := okf.Load(os.DirFS(bundleDir))
	require.NoError(t, err)
	fresh := t.TempDir()
	write(t, fresh, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"sample\"\npresets = [\"claude\"]\n")
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(fresh, ".ai-rulez"), Scan: testScan})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Count(okfbridge.StatusConflict))
	assert.Equal(t, 0, res.Count(okfbridge.StatusUnchanged))

	second := exportProject(t, fresh)
	require.Equal(t, len(first.Files), len(second.Files))
	for i := range first.Files {
		assert.Equal(t, first.Files[i].Path, second.Files[i].Path)
		assert.Equal(t, string(first.Files[i].Data), string(second.Files[i].Data), first.Files[i].Path)
		assert.Equal(t, first.Files[i].Mode, second.Files[i].Mode, first.Files[i].Path)
	}

	// Source files come back byte-identical for content that has a canonical form.
	for _, rel := range []string{"rules/plain.md", "skills/release/scripts/tag.sh", "skills/release/references/index.md"} {
		want, err := os.ReadFile(filepath.Join(root, ".ai-rulez", rel))
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(fresh, ".ai-rulez", rel))
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), rel)
	}

	// Importing again changes nothing.
	again, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(fresh, ".ai-rulez"), Scan: testScan})
	require.NoError(t, err)
	assert.Equal(t, len(res.Actions), again.Count(okfbridge.StatusUnchanged))
}

func TestInclude(t *testing.T) {
	kinds, err := okfbridge.ParseKinds([]string{"rules,skills"})
	require.NoError(t, err)
	res, err := okfbridge.Export(loadTree(t, sampleProject(t)), okfbridge.ExportOptions{Include: kinds})
	require.NoError(t, err)
	for _, f := range res.Files {
		assert.NotContains(t, f.Path, "agents/")
		assert.NotContains(t, f.Path, "context/")
	}
	_, err = okfbridge.ParseKinds([]string{"nonsense"})
	assert.Error(t, err)
}

func foreignBundle(files map[string]string) *okf.Bundle {
	dir, _ := os.MkdirTemp("", "okf")
	for p, c := range files {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		_ = os.WriteFile(filepath.Join(dir, p), []byte(c), 0o644)
	}
	b, _ := okf.Load(os.DirFS(dir))
	return b
}

func TestImportForeignBundleMapsByType(t *testing.T) {
	b := foreignBundle(map[string]string{
		"index.md":            "* [x](a.md)\n",
		"decisions/use-go.md": "---\ntype: Decision\ntitle: Use Go\ndescription: We use Go\ntags: [lang]\n---\nBecause.\n",
		"runbooks/deploy.md":  "---\ntype: Runbook\ntitle: Deploy\n---\nDo it.\n",
		"metrics/mrr.md":      "---\ntype: Metric\ntitle: MRR\nsources:\n  - resource: https://x.y\n---\nFormula.\n",
		"weird.md":            "---\ntype: Totally Unknown\nextra: {a: 1}\n---\nx\n",
		"notype.md":           "just text\n",
		"broken.md":           "---\ntype: [\n---\n",
	})
	cfgDir := filepath.Join(t.TempDir(), ".ai-rulez")
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan})
	require.NoError(t, err)
	got := map[string]okfbridge.Kind{}
	for _, a := range res.Actions {
		got[a.Path] = a.Kind
	}
	assert.Equal(t, okfbridge.KindRule, got["rules/decisions-use-go.md"])
	assert.Equal(t, okfbridge.KindSkill, got["skills/runbooks-deploy/SKILL.md"])
	assert.Equal(t, okfbridge.KindContext, got["context/metrics-mrr.md"])
	assert.Equal(t, okfbridge.KindContext, got["context/weird.md"])
	assert.Equal(t, okfbridge.KindContext, got["context/notype.md"])
	require.Len(t, res.Skipped, 1)
	assert.Contains(t, res.Skipped[0], "broken.md")

	text, err := os.ReadFile(filepath.Join(cfgDir, "rules/decisions-use-go.md"))
	require.NoError(t, err)
	assert.Contains(t, string(text), "description: We use Go")
	assert.Contains(t, string(text), "okf:")
	assert.Contains(t, string(text), "tags:")

	// Loads as a normal ai-rulez project and exports again with the foreign keys back.
	write(t, filepath.Dir(cfgDir), ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"imported\"\npresets = [\"claude\"]\n")
	out := exportProject(t, filepath.Dir(cfgDir))
	var ruleFile string
	for _, f := range out.Files {
		if f.Path == "rules/decisions-use-go.md" {
			ruleFile = string(f.Data)
		}
	}
	assert.Contains(t, ruleFile, "type: Decision")
	assert.Contains(t, ruleFile, "title: Use Go")
	assert.Contains(t, ruleFile, "tags:")
}

func TestImportIntoForcesKindAndDomain(t *testing.T) {
	b := foreignBundle(map[string]string{"a.md": "---\ntype: Decision\n---\nx\n"})
	cfgDir := filepath.Join(t.TempDir(), ".ai-rulez")
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan, Into: okfbridge.KindContext, Domain: "team"})
	require.NoError(t, err)
	require.Len(t, res.Actions, 1)
	assert.Equal(t, "domains/team/context/a.md", res.Actions[0].Path)
}

func TestImportNeverOverwritesWithoutForce(t *testing.T) {
	b := foreignBundle(map[string]string{"a.md": "---\ntype: Decision\n---\nnew\n"})
	cfgDir := filepath.Join(t.TempDir(), ".ai-rulez")
	write(t, cfgDir, "rules/a.md", "mine\n")
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Count(okfbridge.StatusConflict))
	got, _ := os.ReadFile(filepath.Join(cfgDir, "rules/a.md"))
	assert.Equal(t, "mine\n", string(got))

	res, err = okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan, Force: true})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Count(okfbridge.StatusOverwritten))
	got, _ = os.ReadFile(filepath.Join(cfgDir, "rules/a.md"))
	assert.Contains(t, string(got), "new")
}

func TestImportDryRunWritesNothing(t *testing.T) {
	b := foreignBundle(map[string]string{"a.md": "---\ntype: Decision\n---\nx\n"})
	cfgDir := filepath.Join(t.TempDir(), ".ai-rulez")
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan, DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Count(okfbridge.StatusCreated))
	_, statErr := os.Stat(cfgDir)
	assert.True(t, os.IsNotExist(statErr))
}

func TestImportRefusesSecretsAndHiddenText(t *testing.T) {
	b := foreignBundle(map[string]string{
		"ok.md":  "---\ntype: Decision\n---\nfine\n",
		"bad.md": "---\ntype: Decision\n---\nkey AKIAABCDEFGHIJKLMNOP here\n",
	})
	cfgDir := filepath.Join(t.TempDir(), ".ai-rulez")
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan})
	var sec *okfbridge.SecurityError
	require.ErrorAs(t, err, &sec)
	assert.NotEmpty(t, res.Security)
	_, statErr := os.Stat(cfgDir)
	assert.True(t, os.IsNotExist(statErr), "nothing is written when the scan fails")

	hidden := foreignBundle(map[string]string{"h.md": "---\ntype: Decision\n---\nzero\u200bwidth\n"})
	_, err = okfbridge.Import(hidden, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan})
	assert.ErrorAs(t, err, &sec)

	inline := foreignBundle(map[string]string{"i.md": "---\ntype: Decision\n---\n<!-- ai-rulez-lint-ignore -->\nkey AKIAABCDEFGHIJKLMNOP\n"})
	_, err = okfbridge.Import(inline, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan})
	assert.ErrorAs(t, err, &sec, "imported text cannot silence its own findings")
}

func TestImportRejectsHostileMetadata(t *testing.T) {
	b := foreignBundle(map[string]string{
		"a.md":                         "---\ntype: Decision\nx-ai-rulez:\n  kind: rule\n  id: ../../../etc/passwd\n---\nx\n",
		"skills/s/SKILL.md":            "---\ntype: Playbook\nx-ai-rulez:\n  kind: skill\n  id: s\n---\nx\n",
		"skills/s/references/evil.md":  "---\ntype: Reference\nx-ai-rulez:\n  kind: skill-resource\n  id: s\n  path: ../../../../evil.md\n---\nx\n",
		"skills/s/references/evil2.md": "---\ntype: Reference\nx-ai-rulez:\n  kind: skill-resource\n  id: ../x\n  path: references/e.md\n---\nx\n",
	})
	cfgDir := filepath.Join(t.TempDir(), "root", ".ai-rulez")
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan})
	require.NoError(t, err)
	for _, a := range res.Actions {
		assert.NotContains(t, a.Path, "..")
		full := filepath.Join(cfgDir, a.Path)
		assert.True(t, filepath.IsLocal(a.Path), full)
	}
	assert.NotEmpty(t, res.Skipped)
	_, statErr := os.Stat(filepath.Join(filepath.Dir(cfgDir), "evil.md"))
	assert.True(t, os.IsNotExist(statErr))
	assert.Equal(t, "rules/a.md", res.Actions[0].Path, "unsafe id falls back to the file name")
}

func TestImportUnknownXAIRulezKindFallsBackToType(t *testing.T) {
	b := foreignBundle(map[string]string{"a.md": "---\ntype: Playbook\nx-ai-rulez:\n  kind: spaceship\n---\nx\n"})
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(t.TempDir(), ".ai-rulez"), DryRun: true, Scan: testScan})
	require.NoError(t, err)
	assert.Equal(t, okfbridge.KindSkill, res.Actions[0].Kind)
	found := false
	for _, f := range res.Findings {
		found = found || f.Code == okf.CodeLossyMapping
	}
	assert.True(t, found)
}

func TestImportOfficialAcmeRetail(t *testing.T) {
	b, err := okf.Load(os.DirFS("../okf/testdata/acme_retail"))
	require.NoError(t, err)
	cfgDir := filepath.Join(t.TempDir(), ".ai-rulez")
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan})
	require.NoError(t, err)
	assert.Empty(t, res.Security)
	assert.Equal(t, 9, len(res.Actions))
	write(t, filepath.Dir(cfgDir), ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"acme\"\npresets = [\"claude\"]\n")
	out := exportProject(t, filepath.Dir(cfgDir))
	dir := t.TempDir() + "/b"
	writeBundle(t, dir, out.Files)
	nb, err := okf.Load(os.DirFS(dir))
	require.NoError(t, err)
	for _, f := range nb.Validate() {
		assert.NotEqual(t, okf.SeverityError, f.Severity, "%s %s %s", f.Code, f.Path, f.Message)
	}
}

func TestRoundTripAddsNoKeysToASkillWithoutNameOrDescription(t *testing.T) {
	// Arrange: a skill whose frontmatter has neither key.
	root := t.TempDir()
	write(t, root, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"sample\"\npresets = [\"claude\"]\n")
	write(t, root, ".ai-rulez/skills/bare/SKILL.md", "---\nallowed-tools: Read\n---\n\nSteps.\n")
	first := exportProject(t, root)
	bundleDir := t.TempDir() + "/b"
	writeBundle(t, bundleDir, first.Files)
	b, err := okf.Load(os.DirFS(bundleDir))
	require.NoError(t, err)
	fresh := t.TempDir()
	write(t, fresh, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"sample\"\npresets = [\"claude\"]\n")

	// Act
	_, err = okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(fresh, ".ai-rulez"), Scan: testScan})
	require.NoError(t, err)
	second := exportProject(t, fresh)

	// Assert: the second export is the first, and the source file is unchanged.
	require.Equal(t, len(first.Files), len(second.Files))
	for i := range first.Files {
		assert.Equal(t, string(first.Files[i].Data), string(second.Files[i].Data), first.Files[i].Path)
	}
	got, err := os.ReadFile(filepath.Join(fresh, ".ai-rulez/skills/bare/SKILL.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(got), "name:")
	assert.NotContains(t, string(got), "description:")
}

func TestExportConceptsNeverClaimReservedNames(t *testing.T) {
	// Arrange
	root := sampleProject(t)
	write(t, root, ".ai-rulez/rules/index.md", "---\ndescription: A rule named index\n---\nbody index\n")
	write(t, root, ".ai-rulez/rules/log.md", "---\ndescription: A rule named log\n---\nbody log\n")

	// Act
	res := exportProject(t, root)

	// Assert
	byPath := map[string]string{}
	for _, f := range res.Files {
		byPath[f.Path] = string(f.Data)
	}
	assert.Contains(t, byPath["rules/index.md"], "# Concepts", "rules/index.md stays the generated index")
	assert.NotContains(t, byPath["rules/index.md"], "body index")
	assert.Contains(t, byPath["rules/index_.md"], "body index")
	assert.Contains(t, byPath["rules/log_.md"], "body log")
	dir := t.TempDir() + "/bundle"
	writeBundle(t, dir, res.Files)
	b, err := okf.Load(os.DirFS(dir))
	require.NoError(t, err)
	assert.Empty(t, b.Validate())

	// And the real names come back on import.
	fresh := t.TempDir()
	write(t, fresh, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"sample\"\npresets = [\"claude\"]\n")
	_, err = okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(fresh, ".ai-rulez"), Scan: testScan})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(fresh, ".ai-rulez/rules/index.md"))
	assert.FileExists(t, filepath.Join(fresh, ".ai-rulez/rules/log.md"))
}

func TestRoundTripKeepsNonASCIINames(t *testing.T) {
	// Arrange
	root := t.TempDir()
	write(t, root, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"sample\"\npresets = [\"claude\"]\n")
	write(t, root, ".ai-rulez/rules/résumé.md", "---\ndescription: Accents\n---\nbody\n")
	write(t, root, ".ai-rulez/rules/日本語.md", "---\ndescription: CJK\n---\nbody2\n")
	first := exportProject(t, root)
	dir := t.TempDir() + "/b"
	writeBundle(t, dir, first.Files)
	b, err := okf.Load(os.DirFS(dir))
	require.NoError(t, err)
	fresh := t.TempDir()
	write(t, fresh, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"sample\"\npresets = [\"claude\"]\n")

	// Act
	_, err = okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: filepath.Join(fresh, ".ai-rulez"), Scan: testScan})

	// Assert
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(fresh, ".ai-rulez/rules/résumé.md"))
	assert.FileExists(t, filepath.Join(fresh, ".ai-rulez/rules/日本語.md"))
}

func TestImportReportsAnUnsafeDomain(t *testing.T) {
	// Arrange
	b := foreignBundle(map[string]string{"a.md": "---\ntype: Decision\nx-ai-rulez:\n  kind: rule\n  id: a\n  domain: \"../etc\"\n---\nx\n"})
	cfgDir := filepath.Join(t.TempDir(), ".ai-rulez")

	// Act
	res, err := okfbridge.Import(b, okfbridge.ImportOptions{ConfigDir: cfgDir, Scan: testScan})

	// Assert
	require.NoError(t, err)
	require.Len(t, res.Actions, 1)
	assert.Equal(t, "rules/a.md", res.Actions[0].Path)
	found := false
	for _, f := range res.Findings {
		found = found || (f.Code == okf.CodeLossyMapping && strings.Contains(f.Message, "../etc"))
	}
	assert.True(t, found, "the dropped domain is reported")
}
