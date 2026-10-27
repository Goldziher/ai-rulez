package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, exec.Command("git", "-C", dir, "init", "-q").Run())
}

func writeRoot(t *testing.T, dir, name string, gitignore bool) {
	t.Helper()
	cfg := "version = \"5.0\"\nname = \"" + name + "\"\ngitignore = false\npresets = [\"claude\"]\n"
	if gitignore {
		cfg = strings.Replace(cfg, "gitignore = false", "gitignore = true", 1)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
	testutil.WriteFile(t, filepath.Join(dir, ".ai-rulez"), "config.toml", cfg)
}

func readIfExists(t *testing.T, path string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		require.True(t, os.IsNotExist(err), err)
		return "", false
	}
	return string(data), true
}

// #499: the managed block must not depend on which machine-local files exist.
func TestGitignoreBlockIsIndependentOfLocalState(t *testing.T) {
	dir := testutil.CreateTempDir(t)
	gitInit(t, dir)
	writeRoot(t, dir, "solo", true)

	testutil.RunCLIExpectSuccess(t, dir, "generate", "--yes")
	bare := testutil.ReadFile(t, filepath.Join(dir, ".gitignore"))
	for _, want := range []string{".ai-rulez/config.local.*", ".ai-rulez/local/", ".ai-rulez/.generated-manifest.local.json"} {
		assert.Contains(t, bare, want)
	}

	testutil.WriteFile(t, filepath.Join(dir, ".ai-rulez"), "config.local.toml",
		"version = \"5.0\"\n\n[[hooks]]\nevent = \"PreToolUse\"\nmatcher = \"Bash\"\ncommand = \"echo hi\"\n")

	for _, args := range [][]string{{"generate", "--yes"}, {"generate", "--yes", "--no-local"}} {
		testutil.RunCLIExpectSuccess(t, dir, args...)
		assert.Equal(t, bare, testutil.ReadFile(t, filepath.Join(dir, ".gitignore")), strings.Join(args, " "))
	}

	require.NoError(t, os.Remove(filepath.Join(dir, ".ai-rulez", "config.local.toml")))
	testutil.RunCLIExpectSuccess(t, dir, "generate", "--yes")
	assert.Equal(t, bare, testutil.ReadFile(t, filepath.Join(dir, ".gitignore")))

	check := testutil.RunCLI(t, dir, "generate", "--check", "--no-local")
	assert.Equal(t, 0, check.ExitCode, check.Stdout+check.Stderr)
}

// #497: a root's patterns go to the .gitignore of its own repository, and only
// roots with gitignore = true contribute.
func TestRecursiveGitignoreStaysInEachRepository(t *testing.T) {
	parent := testutil.CreateTempDir(t)
	gitInit(t, parent)
	writeRoot(t, parent, "parent", false)
	child := filepath.Join(parent, "child")
	gitInit(t, child)
	writeRoot(t, child, "child", true)
	plain := filepath.Join(parent, "plain")
	writeRoot(t, plain, "plain", false)

	testutil.RunCLIExpectSuccess(t, parent, "generate", "--recursive", "--no-local", "--yes")

	_, ok := readIfExists(t, filepath.Join(parent, ".gitignore"))
	assert.False(t, ok, "the parent sets gitignore = false and must not gain a .gitignore")
	childIgnore, ok := readIfExists(t, filepath.Join(child, ".gitignore"))
	require.True(t, ok, "the nested repository gets its own block")
	assert.Contains(t, childIgnore, "CLAUDE.md")
	assert.NotContains(t, childIgnore, "child/")
	_, ok = readIfExists(t, filepath.Join(plain, ".gitignore"))
	assert.False(t, ok)

	check := testutil.RunCLI(t, parent, "generate", "--recursive", "--check", "--no-local")
	assert.Equal(t, 0, check.ExitCode, check.Stdout+check.Stderr)
}

// A nested root without its own repository contributes to the toplevel block
// when it sets gitignore = true; a sibling with gitignore = false does not.
func TestRecursiveGitignoreSharedToplevelOnlyForEnabledRoots(t *testing.T) {
	top := testutil.CreateTempDir(t)
	gitInit(t, top)
	writeRoot(t, top, "top", true)
	writeRoot(t, filepath.Join(top, "on"), "on", true)
	writeRoot(t, filepath.Join(top, "off"), "off", false)

	testutil.RunCLIExpectSuccess(t, top, "generate", "--recursive", "--no-local", "--yes")

	got := testutil.ReadFile(t, filepath.Join(top, ".gitignore"))
	assert.Contains(t, got, "on/.claude/skills/")
	assert.NotContains(t, got, "off/")
}
