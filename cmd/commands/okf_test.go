package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetOKFFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		okfOut, okfProfile, okfInclude, okfCheck, okfFormat = "", "", nil, false, ""
		okfFailOn, okfInto, okfDomain, okfForce, okfDryRun = "error", "", "", false, false
		noLocal, configDir = false, ""
	}
	reset()
	t.Cleanup(reset)
}

func okfProject(t *testing.T) string {
	t.Helper()
	resetOKFFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n")
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
	writeFile(t, filepath.Join(target, ".ai-rulez", "config.toml"), "version = \"4.0\"\nname = \"y\"\npresets = [\"claude\"]\n")
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
	writeFile(t, filepath.Join(target, ".ai-rulez", "config.toml"), "version = \"4.0\"\nname = \"y\"\npresets = [\"claude\"]\n")
	chdir(t, target)
	var out bytes.Buffer
	assert.Equal(t, exitOKFProblems, runOKFImport(context.Background(), bundle, &out))
	assert.Contains(t, out.String(), "AR001")
	assert.NoDirExists(t, filepath.Join(target, ".ai-rulez", "rules"))
}
