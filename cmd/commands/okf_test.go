package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func resetOKFFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		okfOut, okfProfile, okfRole, okfInclude, okfCheck, okfFormat = "", "", "", nil, false, ""
		okfFailOn, okfInto, okfDomain, okfForce, okfDryRun = "error", "", "", false, false
		okfIndexStyle = ""
		noLocal, configDir = false, ""
	}
	reset()
	t.Cleanup(reset)
}

func okfProject(t *testing.T) string {
	t.Helper()
	resetOKFFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "---\ndescription: Style\n---\nUse gofmt.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "context", "arch.md"), "---\ndescription: Arch\n---\nLayers.\n")
	chdir(t, root)
	return root
}

func exportRun(t *testing.T, check bool) (int, string) {
	t.Helper()
	okfCheck = check
	defer func() { okfCheck = false }()
	var out bytes.Buffer
	code := runOKFExport(context.Background(), nil, &out)
	return code, out.String()
}

func TestOKFExportWritesChecksAndDetectsDrift(t *testing.T) {
	root := okfProject(t)

	code, out := exportRun(t, true)
	assert.Equal(t, exitOKFProblems, code, "a missing bundle is drift: %s", out)
	assert.Contains(t, out, "missing: index.md")

	code, out = exportRun(t, false)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "Wrote")
	bundle := filepath.Join(root, "docs", "okf")
	require.FileExists(t, filepath.Join(bundle, "index.md"))
	require.FileExists(t, filepath.Join(bundle, "rules", "style.md"))

	code, out = exportRun(t, true)
	assert.Equal(t, 0, code, out)

	writeFile(t, filepath.Join(bundle, "rules", "style.md"), "edited\n")
	code, out = exportRun(t, true)
	assert.Equal(t, exitOKFProblems, code)
	assert.Contains(t, out, "changed: rules/style.md")
}

func TestOKFExportOutIncludeAndRefusal(t *testing.T) {
	root := okfProject(t)
	okfOut = filepath.Join(root, "elsewhere")
	okfInclude = []string{"rules"}
	code, out := exportRun(t, false)
	require.Equal(t, 0, code, out)
	assert.FileExists(t, filepath.Join(root, "elsewhere", "rules", "style.md"))
	assert.NoFileExists(t, filepath.Join(root, "elsewhere", "context", "arch.md"))

	// A directory that holds foreign files is never cleaned.
	foreign := filepath.Join(root, "foreign")
	writeFile(t, filepath.Join(foreign, "keep.txt"), "mine")
	okfOut = foreign
	code, _ = exportRun(t, false)
	assert.Equal(t, exitOKFCannotRun, code)
	assert.FileExists(t, filepath.Join(foreign, "keep.txt"))

	okfOut, okfInclude = "", []string{"nonsense"}
	code, _ = exportRun(t, false)
	assert.Equal(t, exitOKFCannotRun, code)
}

func TestOKFValidateRejectsNonBundleDirectory(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
	}{
		{"empty directory", nil},
		{"only notes.txt", map[string]string{"notes.txt": "hello\n"}},
		{"index without okf_version", map[string]string{"index.md": "# Concepts\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			okfProject(t)
			dir := t.TempDir()
			for name, content := range tt.files {
				writeFile(t, filepath.Join(dir, name), content)
			}

			// Act
			var out bytes.Buffer
			code := runOKFValidate(context.Background(), dir, &out)

			// Assert
			assert.Equal(t, exitOKFProblems, code, out.String())
			assert.Contains(t, out.String(), "okf_version")
		})
	}
}

func TestOKFValidateCommand(t *testing.T) {
	root := okfProject(t)
	require.Equal(t, 0, mustExport(t))
	bundle := filepath.Join(root, "docs", "okf")

	var out bytes.Buffer
	assert.Equal(t, 0, runOKFValidate(context.Background(), bundle, &out), out.String())

	writeFile(t, filepath.Join(bundle, "rules", "bad.md"), "no frontmatter\n")
	out.Reset()
	assert.Equal(t, exitOKFProblems, runOKFValidate(context.Background(), bundle, &out))
	assert.Contains(t, out.String(), "AR9B1")

	okfFormat = "json"
	out.Reset()
	assert.Equal(t, exitOKFProblems, runOKFValidate(context.Background(), bundle, &out))
	var decoded struct {
		Findings []struct{ Code string } `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	assert.NotEmpty(t, decoded.Findings)

	okfFailOn = "none"
	assert.Equal(t, 0, runOKFValidate(context.Background(), bundle, &bytes.Buffer{}))
	okfFailOn = "bogus"
	assert.Equal(t, exitOKFCannotRun, runOKFValidate(context.Background(), bundle, &bytes.Buffer{}))
	okfFailOn = "error"
	assert.Equal(t, exitOKFCannotRun, runOKFValidate(context.Background(), filepath.Join(root, "missing"), &bytes.Buffer{}))
}

func mustExport(t *testing.T) int {
	t.Helper()
	code, _ := exportRun(t, false)
	return code
}

func TestOKFImportCommand(t *testing.T) {
	root := okfProject(t)
	require.Equal(t, 0, mustExport(t))
	bundle := filepath.Join(root, "docs", "okf")

	target := t.TempDir()
	writeFile(t, filepath.Join(target, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"y\"\npresets = [\"claude\"]\n")
	chdir(t, target)

	okfDryRun = true
	var out bytes.Buffer
	require.Equal(t, 0, runOKFImport(context.Background(), bundle, &out), out.String())
	assert.Contains(t, out.String(), "would create")
	assert.NoFileExists(t, filepath.Join(target, ".ai-rulez", "rules", "style.md"))

	okfDryRun = false
	out.Reset()
	require.Equal(t, 0, runOKFImport(context.Background(), bundle, &out), out.String())
	got, err := os.ReadFile(filepath.Join(target, ".ai-rulez", "rules", "style.md"))
	require.NoError(t, err)
	assert.Equal(t, "---\ndescription: Style\n---\n\nUse gofmt.\n", string(got))

	out.Reset()
	require.Equal(t, 0, runOKFImport(context.Background(), bundle, &out))
	assert.Contains(t, out.String(), "2 unchanged")

	writeFile(t, filepath.Join(target, ".ai-rulez", "rules", "style.md"), "mine\n")
	out.Reset()
	assert.Equal(t, exitOKFProblems, runOKFImport(context.Background(), bundle, &out))
	again, _ := os.ReadFile(filepath.Join(target, ".ai-rulez", "rules", "style.md"))
	assert.Equal(t, "mine\n", string(again))
	okfForce = true
	assert.Equal(t, 0, runOKFImport(context.Background(), bundle, &bytes.Buffer{}))

	okfInto = "nonsense"
	assert.Equal(t, exitOKFCannotRun, runOKFImport(context.Background(), bundle, &bytes.Buffer{}))
	okfInto = ""
	chdir(t, t.TempDir())
	assert.Equal(t, exitOKFCannotRun, runOKFImport(context.Background(), bundle, &bytes.Buffer{}), "no .ai-rulez directory")
}

func TestOKFImportRefusesSecrets(t *testing.T) {
	okfProject(t)
	bundle := t.TempDir()
	writeFile(t, filepath.Join(bundle, "a.md"), "---\ntype: Decision\n---\nkey AKIAABCDEFGHIJKLMNOP\n")
	target := t.TempDir()
	writeFile(t, filepath.Join(target, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"y\"\npresets = [\"claude\"]\n")
	chdir(t, target)
	var out bytes.Buffer
	assert.Equal(t, exitOKFProblems, runOKFImport(context.Background(), bundle, &out))
	assert.Contains(t, out.String(), "AR001")
	assert.NoDirExists(t, filepath.Join(target, ".ai-rulez", "rules"))
}

func TestStrictValidateLintsTheOKFBundle(t *testing.T) {
	root := okfProject(t)
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\", \"okf\"]\n")
	require.Equal(t, 0, runRecursiveGenerate())

	strictCodes := func() map[string]string {
		cfg, err := loadConfigForCommand(context.Background(), nil)
		require.NoError(t, err)
		report, err := strictLint(t.Context(), cfg)
		require.NoError(t, err)
		got := map[string]string{}
		for _, f := range report.Findings {
			if strings.HasPrefix(f.Code, "AR9B") {
				got[f.Code+" "+f.File] = string(f.Severity)
			}
		}
		return got
	}
	assert.Empty(t, strictCodes(), "a freshly generated bundle is clean")

	bundle := filepath.Join(root, "docs", "okf")
	writeFile(t, filepath.Join(bundle, "rules", "style.md"), "---\ntype: Decision\n---\n[gone](nope.md)\n")
	got := strictCodes()
	assert.Equal(t, "error", got["AR9B5 docs/okf/rules/style.md"])
	assert.Equal(t, "warning", got["AR9B2 docs/okf/rules/style.md"])

	require.NoError(t, os.Remove(filepath.Join(bundle, "rules", "style.md")))
	got = strictCodes()
	assert.Equal(t, "error", got["AR9B5 docs/okf/rules/style.md"])
	assert.Equal(t, "warning", got["AR9B0 docs/okf/rules/index.md"])
}

func TestDoctorReportsOKFBundleProblems(t *testing.T) {
	root := okfProject(t)
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\", \"okf\"]\n")
	require.Equal(t, 0, runRecursiveGenerate())
	writeFile(t, filepath.Join(root, "docs", "okf", "rules", "notype.md"), "plain text\n")
	t.Cleanup(func() { doctorStrict, doctorJSON, doctorProfile = false, false, "" })
	var out bytes.Buffer
	code := runDoctor(context.Background(), nil, &out)
	assert.Equal(t, exitDoctorFindings, code, out.String())
	assert.Contains(t, out.String(), "AR9B1")
}

func TestOKFBundleAsInclude(t *testing.T) {
	root := okfProject(t)
	bundle := filepath.Join(t.TempDir(), "kb")
	writeFile(t, filepath.Join(bundle, "decisions", "use-go.md"), "---\ntype: Decision\ndescription: Use Go\n---\nWe write Go.\n")
	writeFile(t, filepath.Join(bundle, "concepts", "arch.md"), "---\ntype: Concept\n---\nLayers.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\nagents_md = false\n\n[[includes]]\nname = \"kb\"\nsource = \""+filepath.ToSlash(bundle)+"\"\nformat = \"okf\"\ninclude = [\"rules\"]\n")
	require.Equal(t, 0, runRecursiveGenerate())
	got, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "decisions-use-go.md"))
	require.NoError(t, err)
	assert.Contains(t, string(got), "We write Go.")
	assert.NoFileExists(t, filepath.Join(root, ".claude", "rules", "concepts-arch.md"), "include filter applies")

	writeFile(t, filepath.Join(bundle, "decisions", "evil.md"), "---\ntype: Decision\n---\nkey AKIAABCDEFGHIJKLMNOP\n")
	runRecursiveGenerate() // a refused include is skipped with an error, as any failing include is
	assert.NoFileExists(t, filepath.Join(root, ".claude", "rules", "decisions-evil.md"), "the security scan refuses the whole bundle")
}

const okfRolesConfig = `version = "5.0"
name = "x"
presets = ["claude", "okf"]

[skills]
delivery = "served"

[[roles]]
name = "backend"
domains = ["backend"]
[roles.rules]
exclude = ["secret-*"]
[roles.checks]
include = ["review-*"]
`

// okfRolesProject has root content, a backend and a frontend domain, a served
// skill and two checks, and one role that selects part of it.
func okfRolesProject(t *testing.T) string {
	t.Helper()
	root := okfProject(t)
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), okfRolesConfig)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "secret-plan.md"), "---\ndescription: Plan\n---\nInternal plan.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "domains", "backend", "rules", "db.md"), "---\ndescription: DB\n---\nUse transactions.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "domains", "frontend", "rules", "css.md"), "---\ndescription: CSS\n---\nUse tokens.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "domains", "backend", "skills", "migrate", "SKILL.md"),
		"---\nname: migrate\ndescription: Use when you migrate the database.\n---\nRun the migration.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "checks", "review-security.md"), "---\ndescription: Security review\n---\nCheck authz.\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "checks", "lint-style.md"), "---\ndescription: Style check\n---\nCheck naming.\n")
	return root
}

func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p) //nolint:errcheck // both paths are below dir
			out = append(out, filepath.ToSlash(rel))
		}
		return err
	}))
	return out
}

func TestOKFExportRoleFiltersLikeGenerate(t *testing.T) {
	okfRolesProject(t)
	okfRole = "backend"
	out := filepath.Join(t.TempDir(), "bundle")
	okfOut = out
	code, msg := exportRun(t, false)
	require.Equal(t, 0, code, msg)

	files := listFiles(t, out)
	joined := strings.Join(files, " ")
	assert.Contains(t, joined, "db.md", "the role's domain is exported")
	assert.Contains(t, joined, "review-security.md", "a check the role includes is exported")
	assert.Contains(t, joined, "migrate", "a served skill is still knowledge and is exported")
	assert.NotContains(t, joined, "secret-plan", "the role excludes it")
	assert.NotContains(t, joined, "css.md", "a domain the role does not select")
	assert.NotContains(t, joined, "lint-style", "a check outside the role include list")
	assert.Contains(t, joined, "rules/style.md", "root content stays unless a selector drops it")

	// The full export still has everything.
	okfRole = ""
	full := filepath.Join(t.TempDir(), "full")
	okfOut = full
	code, msg = exportRun(t, false)
	require.Equal(t, 0, code, msg)
	all := strings.Join(listFiles(t, full), " ")
	for _, want := range []string{"secret-plan", "css.md", "lint-style", "review-security"} {
		assert.Contains(t, all, want)
	}
}

func TestOKFExportRoleErrors(t *testing.T) {
	okfRolesProject(t)
	okfRole = "nobody"
	code, _ := exportRun(t, false)
	assert.Equal(t, exitOKFCannotRun, code, "unknown role")

	okfRole, okfProfile = "backend", "default"
	code, _ = exportRun(t, false)
	assert.Equal(t, exitOKFCannotRun, code, "--role and --profile are exclusive")
}

func TestOKFExportRoleCheckCompares(t *testing.T) {
	okfRolesProject(t)
	okfRole = "backend"
	dir := filepath.Join(t.TempDir(), "bundle")
	okfOut = dir
	require.Equal(t, 0, mustExport(t))
	code, msg := exportRun(t, true)
	assert.Equal(t, 0, code, msg)
	okfRole = ""
	code, _ = exportRun(t, true)
	assert.Equal(t, exitOKFProblems, code, "the full export differs from the role's bundle")
}

func TestOKFRoleBundleRoundTripsChecksAndDelivery(t *testing.T) {
	okfRolesProject(t)
	okfRole = "backend"
	bundle := filepath.Join(t.TempDir(), "bundle")
	okfOut = bundle
	require.Equal(t, 0, mustExport(t))
	okfRole, okfOut = "", ""

	target := t.TempDir()
	writeFile(t, filepath.Join(target, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"y\"\npresets = [\"claude\"]\n")
	chdir(t, target)
	var out bytes.Buffer
	require.Equal(t, 0, runOKFImport(context.Background(), bundle, &out), out.String())

	imported := strings.Join(listFiles(t, filepath.Join(target, ".ai-rulez")), " ")
	assert.Contains(t, imported, "checks/review-security.md", "a check comes back as a check")
	assert.Contains(t, imported, "skills/migrate/SKILL.md")
	assert.NotContains(t, imported, "lint-style")
}

func TestGenerateWithRoleLeavesTheCommittedOKFBundleAlone(t *testing.T) {
	root := okfRolesProject(t)
	require.Equal(t, 0, runRecursiveGenerate())
	bundle := filepath.Join(root, "docs", "okf")
	before := listFiles(t, bundle)
	require.Contains(t, strings.Join(before, " "), "css.md")

	generateRole = "backend"
	t.Cleanup(func() { generateRole = "" })
	require.Equal(t, 0, runRecursiveGenerate())
	assert.Equal(t, before, listFiles(t, bundle), "generate --role must not shrink the project-wide bundle")
}

func TestOKFIndexStyleFlagConfigAndDetection(t *testing.T) {
	root := okfProject(t)
	bundle := filepath.Join(root, "docs", "okf")

	// --index-style frontmatter writes the frontmatter scheme; validate reports it.
	okfIndexStyle = "frontmatter"
	require.Equal(t, 0, mustExport(t))
	index, err := os.ReadFile(filepath.Join(bundle, "index.md"))
	require.NoError(t, err)
	assert.Contains(t, string(index), "version: 0.1.0\nentries:\n")
	var out bytes.Buffer
	assert.Equal(t, 0, runOKFValidate(context.Background(), bundle, &out), out.String())
	assert.Contains(t, out.String(), "index style: frontmatter")
	assert.Contains(t, out.String(), "AR9B3")

	// --check compares the requested style.
	code, out2 := exportRun(t, true)
	assert.Equal(t, 0, code, out2)
	okfIndexStyle = "body"
	code, out2 = exportRun(t, true)
	assert.Equal(t, exitOKFProblems, code)
	assert.Contains(t, out2, "changed: index.md")

	// The configured style is the default; the flag overrides it.
	okfIndexStyle = ""
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n\n[okf]\nindex_style = \"frontmatter\"\n")
	code, out2 = exportRun(t, true)
	assert.Equal(t, 0, code, out2)

	// The body style is detected too, and JSON carries it.
	okfIndexStyle = "body"
	require.Equal(t, 0, mustExport(t))
	okfFormat = "json"
	out.Reset()
	assert.Equal(t, 0, runOKFValidate(context.Background(), bundle, &out))
	var decoded struct {
		IndexStyle string `json:"index_style"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	assert.Equal(t, "body", decoded.IndexStyle)

	// An unknown style cannot run.
	okfIndexStyle = "yaml"
	assert.Equal(t, exitOKFCannotRun, mustExport(t))
}

func TestOKFImportJSONPrintsEmptyLists(t *testing.T) {
	// Arrange
	root := okfProject(t)
	require.Equal(t, 0, mustExport(t))
	bundle := filepath.Join(root, "docs", "okf")
	target := t.TempDir()
	writeFile(t, filepath.Join(target, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"y\"\npresets = [\"claude\"]\n")
	chdir(t, target)
	okfDryRun, okfFormat = true, formatJSON
	t.Cleanup(func() { okfFormat = formatText })

	// Act
	var out bytes.Buffer
	code := runOKFImport(context.Background(), bundle, &out)

	// Assert
	require.Equal(t, 0, code, out.String())
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	for _, key := range []string{"actions", "findings", "security", "skipped"} {
		assert.NotEqual(t, "null", string(doc[key]), "%s must be a list", key)
	}
	assert.JSONEq(t, "[]", string(doc["security"]))
}

func TestOKFExportRefusesAnOutputThatOverlapsTheConfigDirectory(t *testing.T) {
	root := okfProject(t)
	cfgDir := filepath.Join(root, ".ai-rulez")
	// A migrated config dir looks like a bundle, which the old guard accepted.
	writeFile(t, filepath.Join(cfgDir, "index.md"), "---\nokf_version: \"0.2\"\n---\n")
	writeFile(t, filepath.Join(cfgDir, "ai-rulez.lock"), "lock\n")
	writeFile(t, filepath.Join(cfgDir, "local", "mine.md"), "local\n")
	link := filepath.Join(root, "alias")
	testutil.SymlinkOrSkip(t, cfgDir, link)

	for name, out := range map[string]string{
		"the config dir":          cfgDir,
		"a directory inside it":   filepath.Join(cfgDir, "rules"),
		"its parent":              root,
		"a symlink to it":         link,
		"a path through the link": filepath.Join(link, "rules"),
	} {
		t.Run(name, func(t *testing.T) {
			okfOut = out
			code, _ := exportRun(t, false)
			assert.Equal(t, exitOKFCannotRun, code)
			assert.FileExists(t, filepath.Join(cfgDir, "config.toml"))
			assert.FileExists(t, filepath.Join(cfgDir, "ai-rulez.lock"))
			assert.FileExists(t, filepath.Join(cfgDir, "local", "mine.md"))
			assert.FileExists(t, filepath.Join(cfgDir, "rules", "style.md"))
		})
	}
}

func TestOKFExportRefusesAnyDirectoryHoldingProjectFiles(t *testing.T) {
	root := okfProject(t)
	other := filepath.Join(root, "other")
	writeFile(t, filepath.Join(other, "index.md"), "---\nokf_version: \"0.2\"\n---\n")
	writeFile(t, filepath.Join(other, "config.toml"), "x = 1\n")
	okfOut = other
	code, _ := exportRun(t, false)
	assert.Equal(t, exitOKFCannotRun, code)
	assert.FileExists(t, filepath.Join(other, "config.toml"))
}

func TestOKFExportNeverDeletesAFileItDidNotWrite(t *testing.T) {
	root := okfProject(t)
	code, out := exportRun(t, false)
	require.Equal(t, 0, code, out)
	bundle := filepath.Join(root, "docs", "okf")
	writeFile(t, filepath.Join(bundle, "NOTES.md"), "mine\n")
	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", "context", "arch.md")))

	code, out = exportRun(t, false)
	require.Equal(t, 0, code, out)

	assert.FileExists(t, filepath.Join(bundle, "NOTES.md"))
	assert.NoFileExists(t, filepath.Join(bundle, "context", "arch.md"))
}
