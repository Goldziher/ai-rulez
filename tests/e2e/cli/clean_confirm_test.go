package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

func cleanProject(t *testing.T) string {
	t.Helper()
	dir := testutil.CreateTempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez", "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(exitCodeConfig), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "rules", "style.md"), []byte(goodRule), 0o644))
	t.Cleanup(testutil.CleanupTestBinary)
	require.Equal(t, 0, testutil.RunCLI(t, dir, "generate").ExitCode)
	return dir
}

func TestCleanYesKeepsHandEditedFiles(t *testing.T) {
	dir := cleanProject(t)
	agents := filepath.Join(dir, "AGENTS.md")
	f, err := os.OpenFile(agents, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("my hand edit\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	res := testutil.RunCLI(t, dir, "clean", "--yes")

	assert.Equal(t, 0, res.ExitCode, res.Stderr)
	assert.FileExists(t, agents, "--yes only skips the prompt; it must not delete hand edits")

	res = testutil.RunCLI(t, dir, "clean", "--yes", "--include-edited")
	assert.Equal(t, 0, res.ExitCode, res.Stderr)
	assert.NoFileExists(t, agents)
}

func TestDeclinedConfirmationExitsNonZero(t *testing.T) {
	dir := cleanProject(t)

	clean := testutil.RunCLI(t, dir, "clean")
	assert.Equal(t, exitCannot, clean.ExitCode, "non-interactive clean without --yes did nothing")
	assert.FileExists(t, filepath.Join(dir, "AGENTS.md"))

	remove := testutil.RunCLI(t, dir, "remove", "rule", "style")
	assert.Equal(t, exitCannot, remove.ExitCode, remove.Stdout+remove.Stderr)
	assert.FileExists(t, filepath.Join(dir, ".ai-rulez", "rules", "style.md"))
}

func TestCleanDoesNotWarnAboutAFileTheLastRunRemoved(t *testing.T) {
	dir := cleanProject(t)
	agents := filepath.Join(dir, "AGENTS.md")
	f, err := os.OpenFile(agents, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("my hand edit\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	first := testutil.RunCLI(t, dir, "clean", "--yes")
	require.Equal(t, 0, first.ExitCode, first.Stderr)
	require.NoFileExists(t, filepath.Join(dir, "CLAUDE.md"), "the first run removed it")

	second := testutil.RunCLI(t, dir, "clean", "--yes", "--include-edited")

	assert.Equal(t, 0, second.ExitCode, second.Stderr)
	assert.NotContains(t, second.Stdout+second.Stderr, "CLAUDE.md", "a file that is gone is not kept")
	assert.NoFileExists(t, agents)
}
