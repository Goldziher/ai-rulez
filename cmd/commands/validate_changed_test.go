package commands

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	cmd := gitutil.Command(context.Background(), dir, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(cmd.Env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func changedRepo(t *testing.T) string {
	t.Helper()
	root, _ := strictProject(t, "", map[string]string{
		".ai-rulez/rules/a.md": brokenLinkRule,
		".ai-rulez/rules/b.md": strings.ReplaceAll(brokenLinkRule, "docs/missing.md", "docs/other.md"),
		".ai-rulez/rules/c.md": "---\ndescription: c\n---\nSee [b](b.md).\n",
	})
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "init")
	return root
}

func TestChangedOnlyNarrowsFindings(t *testing.T) {
	resetStrictFlags(t)
	root := changedRepo(t)
	cfg := loadStrictProject(t, root)
	run := func() lint.Combined {
		strictTreeCache = lint.Loader{}
		report := lintProject(t, cfg)
		require.NoError(t, narrowToChanged([]*lint.Report{report}, []*config.Config{cfg}))
		return lint.Combine([]*lint.Report{report})
	}

	validateChanged = true
	none := run()
	assert.Empty(t, none.Findings, "nothing changed since HEAD")
	require.NotNil(t, none.ChangedOnly)
	assert.Equal(t, "HEAD", none.ChangedOnly.Since)

	// Touch b.md: its own finding stays, and so does c.md's link to it; a.md is left out.
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "b.md"), strings.ReplaceAll(brokenLinkRule, "docs/missing.md", "docs/other.md")+"\nedited\n")
	cfg = loadStrictProject(t, root)
	got := run()
	files := map[string]bool{}
	for i := range got.Findings {
		files[got.Findings[i].RepoPath()] = true
	}
	assert.True(t, files[".ai-rulez/rules/b.md"])
	assert.False(t, files[".ai-rulez/rules/a.md"], "unrelated file's finding is not shown")
	assert.Equal(t, 1, got.ChangedOnly.Changed)
	assert.Equal(t, 1, got.ChangedOnly.Dependents, "c.md links to the changed b.md")
	assert.Positive(t, got.ChangedOnly.Dropped)
}

func TestChangedOnlyExitCodeIgnoresUntouchedFindings(t *testing.T) {
	resetStrictFlags(t)
	root := changedRepo(t)
	cfg := loadStrictProject(t, root)
	validateChanged = true
	assert.Equal(t, 0, runStrict(t, root, cfg), "every finding sits in an untouched file")

	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "a.md"), brokenLinkRule+"\nedited\n")
	assert.Equal(t, exitStrictFindings, runStrict(t, root, loadStrictProject(t, root)))
}

func TestChangedOnlyIgnoresInheritedGitEnvironment(t *testing.T) {
	resetStrictFlags(t)
	root := changedRepo(t)
	other := t.TempDir()
	gitIn(t, other, "init", "-q")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))

	validateChanged = true
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "a.md"), brokenLinkRule+"\nedited\n")
	assert.Equal(t, exitStrictFindings, runStrict(t, root, loadStrictProject(t, root)), "git must look at the project, not at GIT_DIR")
}

func TestChangedFlagValidation(t *testing.T) {
	resetStrictFlags(t)
	validateStrict = false
	validateSince = "main"
	assert.Error(t, checkStrictFlags())
	validateStrict = true
	validateChanged = true
	assert.Error(t, checkStrictFlags(), "--since with --changed")
	validateChanged, validateUpdateBaseline = false, true
	assert.Error(t, checkStrictFlags(), "--update-baseline with --since")
	t.Cleanup(func() { validateSince, validateChanged = "", false })
}

func TestChangedOnlyUnknownRevisionFails(t *testing.T) {
	resetStrictFlags(t)
	root := changedRepo(t)
	validateSince = "no-such-rev"
	t.Cleanup(func() { validateSince = "" })
	assert.Equal(t, 1, runStrict(t, root, loadStrictProject(t, root)))
}
