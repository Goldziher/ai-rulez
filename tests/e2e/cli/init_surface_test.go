package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

func TestMigrateTargetsAreSubcommands(t *testing.T) {
	dir := crudProject(t)
	for _, args := range [][]string{{"migrate", "banana"}, {"migrate", "v4"}} {
		res := testutil.RunCLI(t, dir, args...)
		assert.Equal(t, exitCannot, res.ExitCode, args)
		assert.Contains(t, res.Stderr, "unknown command", args)
	}
	// both flag positions keep working
	for _, args := range [][]string{{"migrate", "v5", "--check"}, {"migrate", "--check", "v5"}} {
		res := testutil.RunCLI(t, dir, args...)
		assert.Contains(t, []int{0, 2}, res.ExitCode, "%v: %s", args, res.Stderr)
	}
	okf := testutil.RunCLI(t, dir, "migrate", "okf", "--check", "--format", "json")
	assert.Contains(t, []int{0, 2}, okf.ExitCode, okf.Stderr)
	assert.Contains(t, okf.Stdout, "\"changes\"")
	// v5-only flags are not accepted by okf
	bad := testutil.RunCLI(t, dir, "migrate", "okf", "--recursive")
	assert.Equal(t, exitCannot, bad.ExitCode)
}

func TestRemovedAliasesAndDeadMCPFlags(t *testing.T) {
	dir := crudProject(t)
	for _, alias := range []string{"v", "check", "g", "clear"} {
		res := testutil.RunCLI(t, dir, alias)
		assert.Equal(t, exitCannot, res.ExitCode, alias)
		assert.Contains(t, res.Stderr, "unknown command", alias)
	}
	for _, alias := range []string{"val", "gen"} {
		assert.NotContains(t, testutil.RunCLI(t, dir, alias, "--help").Stderr, "unknown command", alias)
	}
	list := testutil.RunCLI(t, dir, "list", "check")
	assert.Equal(t, exitCannot, list.ExitCode)
	for _, flag := range []string{"--transport", "--address", "--port"} {
		res := testutil.RunCLI(t, dir, "mcp", flag, "x")
		assert.Equal(t, exitCannot, res.ExitCode, flag)
		assert.Contains(t, res.Stderr, "unknown flag", flag)
	}
}

func TestGuardIsListedInRootHelp(t *testing.T) {
	res := testutil.RunCLI(t, crudProject(t), "--help")
	assert.Contains(t, res.Stdout, "guard")
}

func TestInitDoesNotDestroyAndUsesGlobalConfigDir(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)

	res := testutil.RunCLI(t, dir, "--config-dir", ".config/ai-rulez", "init", "--yes")

	require.Equal(t, 0, res.ExitCode, res.Stderr)
	assert.FileExists(t, filepath.Join(dir, ".config", "ai-rulez", "config.toml"))
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
	assert.Equal(t, ".config/ai-rulez\n", filepath.ToSlash(res.Stdout), "the created directory is the result")
	assert.NotContains(t, res.Stderr, "INFO  \n", "no empty log lines")

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".config", "ai-rulez", "rules", "mine.md"), []byte("keep"), 0o644))
	again := testutil.RunCLI(t, dir, "--config-dir", ".config/ai-rulez", "init", "--yes")
	assert.Equal(t, exitCannot, again.ExitCode)
	assert.FileExists(t, filepath.Join(dir, ".config", "ai-rulez", "rules", "mine.md"))
}

func TestInitJSON(t *testing.T) {
	t.Cleanup(testutil.CleanupTestBinary)
	dir := testutil.CreateTempDir(t)

	doc := decodeDoc(t, testutil.RunCLI(t, dir, "init", "--yes", "--format", "json"))

	assert.Equal(t, "created", doc["status"])
	assert.Equal(t, ".ai-rulez", doc["path"])
}
