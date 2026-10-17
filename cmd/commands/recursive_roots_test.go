package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

const rootCount = 8

// gitShim puts a `git` first on PATH that appends its arguments to a log and then
// runs the real git. The returned func reads the calls made so far.
func gitShim(t *testing.T) func() []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the shim is a shell script")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" >> %q\nexec %q \"$@\"\n", logPath, realGit)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755)) //nolint:gosec // a test shim must be executable
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		data, err := os.ReadFile(logPath)
		if os.IsNotExist(err) {
			return nil
		}
		require.NoError(t, err)
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
}

// gitMonorepo builds a git repository with n roots (svc00 ... svcNN), makes it the
// working directory and resets the flags the recursive commands read.
func gitMonorepo(t *testing.T, n int) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	for i := range n {
		writeFile(t, filepath.Join(root, fmt.Sprintf("svc%02d", i), ".ai-rulez", "config.toml"), validRootConfig)
		writeFile(t, filepath.Join(root, fmt.Sprintf("svc%02d", i), ".ai-rulez", "rules", "r.md"), "# r\n\nbody\n")
	}
	require.NoError(t, gitutil.CommandNoContext(root, "init", "-q").Run())
	chdir(t, root)
	progress.SetQuiet(true)
	strictTreeCache = &lint.Loader{}
	t.Cleanup(func() {
		progress.SetQuiet(false)
		strictTreeCache = &lint.Loader{}
		pluginMode, dryRun, profile, validateStrict = false, false, "", false
	})
	pluginMode, dryRun, profile = false, false, ""
	delete(RootCmd.Annotations, activeFormatKey) // a command that ran earlier may have left --format json behind
	return root
}

func rootDir(i int) string { return fmt.Sprintf("svc%02d", i) }

func lsFilesCalls(calls []string) (indexReads []string) {
	for _, c := range calls {
		if strings.Contains(c, " ls-files ") {
			indexReads = append(indexReads, c)
		}
	}
	return indexReads
}

// #376: output of a recursive drift check does not depend on which root finishes
// first, and the exit code is the same on every run.
func TestRunRecursiveDrift_OutputIsDeterministic(t *testing.T) {
	// Arrange: generated files of roots 1, 4 and 6 differ from their sources.
	root := gitMonorepo(t, rootCount)
	require.Equal(t, 0, runRecursiveGenerate())
	for _, i := range []int{1, 4, 6} {
		path := filepath.Join(root, rootDir(i), "CLAUDE.md")
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, append(data, []byte("\nedited by hand\n")...), 0o644))
	}

	var wantOut, wantErr string
	for run := range 6 {
		// Act
		var code int
		stdout, stderr := capture(t, func() { code = runDriftCheck(true) })

		// Assert
		require.Equal(t, exitDrift, code, "run %d", run)
		if run == 0 {
			wantOut, wantErr = stdout, stderr
			continue
		}
		assert.Equal(t, wantOut, stdout, "run %d stdout", run)
		assert.Equal(t, wantErr, stderr, "run %d stderr", run)
	}
	var drifted []string
	for _, line := range strings.Split(strings.TrimSpace(wantOut), "\n") {
		drifted = append(drifted, line[strings.Index(line, ": ")+2:])
	}
	assert.Equal(t, []string{"svc01/CLAUDE.md", "svc04/CLAUDE.md", "svc06/CLAUDE.md"}, drifted, "reported in root order")
}

func TestRunRecursiveDrift_FailureExitCodeIsStable(t *testing.T) {
	root := gitMonorepo(t, rootCount)
	require.Equal(t, 0, runRecursiveGenerate())
	writeFile(t, filepath.Join(root, rootDir(2), ".ai-rulez", "config.toml"), brokenRootConfig)
	writeFile(t, filepath.Join(root, rootDir(5), ".ai-rulez", "config.toml"), brokenRootConfig)

	var first string
	for run := range 4 {
		var code int
		_, stderr := capture(t, func() { code = runDriftCheck(true) })
		require.Equal(t, 1, code, "a root that cannot load is a tool failure, not drift")
		if run == 0 {
			first = stderr
			require.Equal(t, 2, strings.Count(stderr, "Error:"), "both broken roots are reported: %q", stderr)
			continue
		}
		assert.Equal(t, first, stderr)
	}
}

// #376: validate --recursive prints the same thing, in root order, on every run.
func TestRunRecursiveValidate_OutputIsDeterministic(t *testing.T) {
	root := gitMonorepo(t, rootCount)
	writeFile(t, filepath.Join(root, rootDir(2), ".ai-rulez", "config.toml"), brokenRootConfig)
	writeFile(t, filepath.Join(root, rootDir(5), ".ai-rulez", "config.toml"), brokenRootConfig)
	writeFile(t, filepath.Join(root, rootDir(6), ".ai-rulez", "rules", "bad.md"), brokenLinkRule)
	validateStrict = true
	progress.SetQuiet(false)

	var wantOut, wantErr string
	for run := range 5 {
		var code int
		stdout, stderr := capture(t, func() { code = runRecursiveValidate() })
		require.Equal(t, 1, code)
		if run == 0 {
			wantOut, wantErr = stdout, stderr
			continue
		}
		assert.Equal(t, wantOut, stdout, "run %d stdout", run)
		// The per-root progress lines carry a duration, which differs between runs.
		assert.Equal(t, dropProgressLines(wantErr), dropProgressLines(stderr), "run %d stderr", run)
	}
	var ok []string
	for _, line := range strings.Split(wantOut, "\n") {
		if strings.HasPrefix(line, "✅ ") {
			ok = append(ok, filepath.Dir(filepath.Dir(strings.TrimPrefix(line, "✅ "))))
		}
	}
	assert.Equal(t, []string{"svc00", "svc01", "svc03", "svc04", "svc06", "svc07"}, ok, "valid roots in root order")
	assert.Less(t, strings.Index(wantErr, "svc02"), strings.Index(wantErr, "svc05"))
}

func dropProgressLines(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "[") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// #376: the roots of one repository share one view of its index, not one read each.
func TestRunRecursiveDrift_ListsTheIndexOnce(t *testing.T) {
	calls := gitShim(t)
	gitMonorepo(t, rootCount)
	require.Equal(t, 0, runRecursiveGenerate())
	before := len(calls())

	_, _ = capture(t, func() { require.Equal(t, 0, runDriftCheck(true)) })

	listings := lsFilesCalls(calls()[before:])
	// One listing of the tracked files and one of the .gitignore files, not one of each per root.
	assert.LessOrEqual(t, len(listings), 2, "got %v", listings)
}

func TestRunRecursiveValidate_ListsTheIndexOncePerPurpose(t *testing.T) {
	calls := gitShim(t)
	gitMonorepo(t, rootCount)
	validateStrict = true

	_, _ = capture(t, func() { require.Equal(t, 0, runRecursiveValidate()) })

	// One listing answers "is the config tracked" for every root; the strict run's
	// own `ls-files -s` indexes the repository once; the files nobody generated are
	// found from one more listing that every root shares.
	assert.LessOrEqual(t, len(lsFilesCalls(calls())), 3, "got %v", lsFilesCalls(calls()))
}

func TestRecursiveRuns_PrintAProgressLinePerRoot(t *testing.T) {
	gitMonorepo(t, 3)
	require.Equal(t, 0, runRecursiveGenerate())
	progress.SetQuiet(false)

	var code int
	_, stderr := capture(t, func() { code = runDriftCheck(true) })

	require.Equal(t, 0, code)
	var lines []string
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "[") {
			lines = append(lines, line)
		}
	}
	require.Len(t, lines, 3, "stderr: %q", stderr)
	for i, line := range lines {
		assert.Contains(t, line, fmt.Sprintf("[%d/3] %s", i+1, filepath.Join(rootDir(i), ".ai-rulez", "config.toml")))
	}

	validateStrict = true
	_, stderr = capture(t, func() { require.Equal(t, 0, runRecursiveValidate()) })
	assert.Equal(t, 3, strings.Count("\n"+stderr, "\n[1/3]")+strings.Count(stderr, "\n[2/3]")+strings.Count(stderr, "\n[3/3]"),
		"validate prints one per root too: %q", stderr)

	progress.SetQuiet(true)
	_, stderr = capture(t, func() { require.Equal(t, 0, runDriftCheck(true)) })
	assert.NotContains(t, stderr, "[1/3]", "--quiet silences the progress lines")
}

// #376: the shared toplevel .gitignore block is the same whether the roots run one at
// a time or side by side, and `generate --check` judges it the same way.
func TestRecursiveGitignoreBlockIsIndependentOfParallelism(t *testing.T) {
	type result struct {
		block, driftOut, driftErr string
		code                      int
	}
	run := func(procs int) result {
		prev := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(prev)
		root := gitMonorepo(t, rootCount)
		for i := range rootCount {
			writeFile(t, filepath.Join(root, rootDir(i), ".ai-rulez", "config.toml"),
				"version = \"5.0\"\nname = \"ok\"\npresets = [\"claude\"]\ngitignore = true\nagents_md = false\n")
			writeFile(t, filepath.Join(root, rootDir(i), ".ai-rulez", "skills", "demo", "SKILL.md"),
				"---\nname: demo\ndescription: Demo.\n---\n\nDo it.\n")
		}
		recursive, updateGitignore = true, true
		t.Cleanup(func() { recursive, updateGitignore = false, false })
		require.Equal(t, 0, runRecursiveGenerate())
		ignorePath := filepath.Join(root, ".gitignore")
		block := readIgnoreFile(t, ignorePath)
		require.Contains(t, block, "svc07/CLAUDE.md")

		// Drop one root's line so the block is stale.
		stale := strings.Replace(block, "svc03/CLAUDE.md\n", "", 1)
		require.NotEqual(t, block, stale)
		require.NoError(t, os.WriteFile(ignorePath, []byte(stale), 0o644))
		r := result{block: block}
		r.driftOut, r.driftErr = capture(t, func() { r.code = runDriftCheck(true) })
		return r
	}

	seq, par := run(1), run(8)

	assert.Equal(t, seq.block, par.block, "byte-identical block")
	assert.Equal(t, exitDrift, seq.code)
	assert.Equal(t, seq.code, par.code)
	assert.Equal(t, seq.driftOut, par.driftOut)
	assert.Contains(t, par.driftOut, ".gitignore")
	assert.Equal(t, seq.driftErr, par.driftErr)
}
