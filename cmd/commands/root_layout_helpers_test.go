package commands

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const (
	rootLayoutConfig = "version = \"5.0\"\nname = \"solo\"\npresets = [\"claude\"]\n"
	rootLayoutSkill  = "---\nname: demo\ndescription: A demo skill installed from a local source for tests\n---\n\nDo the demo.\n"
)

// rootLayoutProject is a project whose whole configuration is ai-rulez.toml,
// beside a local skill source ./shared that the configuration can install.
func rootLayoutProject(t *testing.T, body string) string {
	t.Helper()
	testutil.ScrubCIEnv(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ai-rulez.toml"), body)
	writeFile(t, filepath.Join(root, "shared", "skills", "demo", "SKILL.md"), rootLayoutSkill)
	chdir(t, root)
	return root
}

// cliResult is what one in-process run of the command line produced.
type cliResult struct {
	stdout, stderr string
	code           int
}

// runCLI runs the command line in the working directory the way main does and
// reports the exit code ReportError maps the outcome to. Every flag the run
// touched is put back afterwards.
func runCLI(t *testing.T, args ...string) cliResult {
	t.Helper()
	resetAllFlags()
	t.Cleanup(func() {
		resetAllFlags()
		progress.SetQuiet(false)
		strictTreeCache = &lint.Loader{}                  // validate --strict caches the loaded tree
		validateStrict, strictSecurityOnly = false, false // scan sets both from RunE, not from a flag
	})
	var res cliResult
	var out, errOut bytes.Buffer
	res.stdout, res.stderr = capture(t, func() {
		RootCmd.SetArgs(args)
		RootCmd.SetOut(&out)
		RootCmd.SetErr(&errOut)
		cmd, err := execute()
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
		if err != nil {
			res.code = ReportError(&out, &errOut, commandFormat(cmd), err)
		}
	})
	res.stdout += out.String()
	res.stderr += errOut.String()
	return res
}

// resetAllFlags puts the flags of every command back to their initial values.
func resetAllFlags() {
	walkCommands(RootCmd, resetFlags)
	// The format of the last command that ran decides how helpers print; a later test must not inherit it.
	delete(RootCmd.Annotations, activeFormatKey)
	delete(RootCmd.Annotations, errorDocWritten)
}
