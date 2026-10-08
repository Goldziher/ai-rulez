package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := gitutil.CommandNoContext(dir, append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// countingCtx returns a ctx whose runner counts git invocations.
func countingCtx(t *testing.T) (context.Context, *atomic.Int64) {
	t.Helper()
	var n atomic.Int64
	r := runner.Func(func(ctx context.Context, spec runner.Spec) runner.Result {
		n.Add(1)
		return runner.Run(ctx, spec)
	})
	return runner.WithContext(t.Context(), r), &n
}

func skillResources(t *testing.T, ctx context.Context, configDir string) map[string][]string {
	t.Helper()
	tree, err := ScanContentTreeContext(ctx, configDir)
	require.NoError(t, err)
	out := map[string][]string{}
	for _, s := range tree.Skills {
		out[s.Name] = relPaths(s.Resources)
	}
	return out
}

func TestBundleVisibilityMatrix(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	gitInit(t, repo)
	skills := filepath.Join(repo, ".ai-rulez", "skills")
	for _, name := range []string{"plain", "tracked", "ignoredroot", "trackedignored", "nested"} {
		writeTree(t, filepath.Join(skills, name), "SKILL.md", "references/a.md", "scripts/gen.json", "scripts/run.sh")
	}
	require.NoError(t, os.WriteFile(filepath.Join(skills, "plain", ".gitignore"), []byte("gen.json\n"), 0o644))
	writeTree(t, filepath.Join(skills, "plain"), "assets/skip/blob.bin")
	require.NoError(t, os.WriteFile(filepath.Join(skills, "plain", ".gitignore"), []byte("gen.json\nassets/skip/\n"), 0o644))

	// tracked: everything committed, then a later ignore rule does not hide it.
	gitIn(t, repo, "add", "-f", ".ai-rulez/skills/tracked")
	gitIn(t, repo, "commit", "-q", "-m", "x")
	require.NoError(t, os.WriteFile(filepath.Join(skills, "tracked", ".gitignore"), []byte("gen.json\n"), 0o644))

	// ignoredroot: the item as a whole is ignored; everything is bundled.
	// trackedignored: SKILL.md is tracked yet matches an ignore rule, so the
	// item counts as ignored and the visibility check is off.
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".ai-rulez/skills/ignoredroot/\n.ai-rulez/skills/trackedignored/SKILL.md\n"), 0o644))
	gitIn(t, repo, "add", "-f", ".ai-rulez/skills/trackedignored/SKILL.md")
	require.NoError(t, os.WriteFile(filepath.Join(skills, "trackedignored", ".gitignore"), []byte("gen.json\n"), 0o644))

	// nested: its own repository with its own ignore rules.
	gitInit(t, filepath.Join(skills, "nested"))
	require.NoError(t, os.WriteFile(filepath.Join(skills, "nested", ".gitignore"), []byte("run.sh\n"), 0o644))

	want := map[string][]string{
		"plain":          {"references/a.md", "scripts/run.sh"},
		"tracked":        {"references/a.md", "scripts/run.sh"},
		"ignoredroot":    {"references/a.md", "scripts/gen.json", "scripts/run.sh"},
		"trackedignored": {"references/a.md", "scripts/gen.json", "scripts/run.sh"},
		"nested":         {"references/a.md", "scripts/gen.json"},
	}
	// tracked keeps gen.json: tracked files are visible whatever the rules say.
	want["tracked"] = []string{"references/a.md", "scripts/gen.json", "scripts/run.sh"}

	ctx, n := countingCtx(t)
	got := skillResources(t, ctx, filepath.Join(repo, ".ai-rulez"))
	assert.Equal(t, want, got)
	// 5 items: batching must not cost more than the nested repo's own pair
	// plus a constant number of calls for the shared parent.
	assert.LessOrEqual(t, n.Load(), int64(2+4), "git calls")
}

func TestBundleVisibilityLinkedWorktree(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	gitInit(t, repo)
	writeTree(t, repo, ".ai-rulez/skills/demo/SKILL.md", ".ai-rulez/skills/demo/scripts/run.sh", ".ai-rulez/skills/demo/scripts/gen.json", "README.md")
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("gen.json\n"), 0o644))
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "x")
	wt := filepath.Join(t.TempDir(), "wt")
	gitIn(t, repo, "worktree", "add", "-q", "-b", "other", wt)
	writeTree(t, wt, ".ai-rulez/skills/demo/scripts/gen.json", ".ai-rulez/skills/demo/scripts/new.sh")

	got := skillResources(t, t.Context(), filepath.Join(wt, ".ai-rulez"))
	assert.Equal(t, map[string][]string{"demo": {"scripts/new.sh", "scripts/run.sh"}}, got)
}

func TestBundleVisibilityLinearGitCalls(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	gitInit(t, repo)
	skills := filepath.Join(repo, ".ai-rulez", "skills")
	const items = 40
	for i := range items {
		writeTree(t, filepath.Join(skills, fmt.Sprintf("s%02d", i)), "SKILL.md", "references/a.md")
	}
	ctx, n := countingCtx(t)
	got := skillResources(t, ctx, filepath.Join(repo, ".ai-rulez"))
	require.Len(t, got, items)
	assert.LessOrEqual(t, n.Load(), int64(4), "git calls must not grow with the number of skills")
}
