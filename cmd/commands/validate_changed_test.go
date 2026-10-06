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

func TestSinceDepthFlag(t *testing.T) {
	tests := []struct {
		value   string
		want    int
		wantErr bool
	}{
		{"", 1, false}, {"1", 1, false}, {"3", 3, false}, {"all", lint.DepthAll, false}, {" ALL ", lint.DepthAll, false},
		{"0", 0, true}, {"-2", 0, true}, {"deep", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			old := validateSinceDepth
			t.Cleanup(func() { validateSinceDepth = old })
			validateSinceDepth = tt.value
			got, err := sinceDepth()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSinceDepthNeedsSince(t *testing.T) {
	resetStrictFlags(t)
	t.Cleanup(func() { validateSinceDepth, validateSinceMax, validateSince = "1", 0, "" })
	validateSinceDepth = "2"
	assert.Error(t, checkStrictFlags(), "--since-depth without --since")
	validateSince = "main"
	assert.NoError(t, checkStrictFlags())
	validateSinceDepth, validateSinceMax = "x", 0
	assert.Error(t, checkStrictFlags(), "an invalid depth")
	validateSinceDepth, validateSinceMax = "all", -1
	assert.Error(t, checkStrictFlags(), "a negative cap")
}

func TestChangedOnlyFollowsTransitiveDependents(t *testing.T) {
	resetStrictFlags(t)
	root := changedRepo(t)
	// d.md refers to c.md, which refers to the changed b.md: two hops.
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "d.md"), "---\ndescription: d\n---\nSee [c](c.md) and [gone](nope-d.md).\n")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "d")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "b.md"), strings.ReplaceAll(brokenLinkRule, "docs/missing.md", "docs/other.md")+"\nedited\n")
	t.Cleanup(func() { validateSinceDepth = "1" })
	run := func(depth string) lint.Combined {
		validateSinceDepth = depth
		strictTreeCache = lint.Loader{}
		cfg := loadStrictProject(t, root)
		report := lintProject(t, cfg)
		require.NoError(t, narrowToChanged([]*lint.Report{report}, []*config.Config{cfg}))
		return lint.Combine([]*lint.Report{report})
	}
	validateChanged = true

	one, two, all := run("1"), run("2"), run("all")

	hops := func(c lint.Combined) map[string]string {
		out := map[string]string{}
		for i := range c.Findings {
			out[c.Findings[i].RepoPath()] = c.Findings[i].Hop()
		}
		return out
	}
	assert.NotContains(t, hops(one), ".ai-rulez/rules/d.md", "depth 1 keeps today's behaviour")
	assert.Equal(t, "transitive(2)", hops(two)[".ai-rulez/rules/d.md"])
	assert.Equal(t, hops(two), hops(all))
	assert.Equal(t, 2, two.ChangedOnly.Depth)
	assert.Equal(t, 1, two.ChangedOnly.Transitive)
	var sb strings.Builder
	require.NoError(t, lint.Write(&sb, lint.FormatText, two, lint.WriteOptions{}))
	assert.Contains(t, sb.String(), "changed-only since HEAD (depth 2)")
}
