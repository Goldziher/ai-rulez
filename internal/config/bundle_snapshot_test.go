package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// countingRunner runs the real command and counts `git ls-files` calls.
type countingRunner struct {
	lsFiles *atomic.Int32
	// all, when set, counts every git process.
	all *atomic.Int32
}

func (c countingRunner) Run(ctx context.Context, spec runner.Spec) runner.Result {
	if slicesContains(spec.Argv, "ls-files") {
		c.lsFiles.Add(1)
	}
	if c.all != nil {
		c.all.Add(1)
	}
	return runner.Exec{}.Run(ctx, spec)
}

func slicesContains(argv []string, want string) bool {
	for _, a := range argv {
		if a == want {
			return true
		}
	}
	return false
}

// #376: the skills and commands of every domain share one `git ls-files` listing
// taken at the configuration directory, not one per directory.
func TestScanContentTree_ListsGitFilesOncePerConfigDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	require.NoError(t, gitutil.CommandNoContext(root, "init", "-q").Run())
	cfgDir := filepath.Join(root, ".ai-rulez")
	for _, domain := range []string{"a", "b", "c", "d"} {
		skill := filepath.Join(cfgDir, "domains", domain, "skills", "s-"+domain)
		require.NoError(t, os.MkdirAll(filepath.Join(skill, "references"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: s-"+domain+"\ndescription: d\n---\n# s\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(skill, "references", "r.md"), []byte("# ref\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(skill, "references", "ignored.md"), []byte("# ig\n"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.md\n"), 0o644))

	calls := &atomic.Int32{}
	scanner := newProjectScanner(context.Background(), osView(cfgDir))
	scanner.git = gitutil.New(countingRunner{lsFiles: calls})
	scanner.snapshots.anchor = filepath.Clean(cfgDir)
	tree, err := scanContentTree(scanner, cfgDir, nil)
	require.NoError(t, err)

	assert.Equal(t, int32(1), calls.Load(), "one git ls-files for all domains")
	require.Len(t, tree.Domains, 4)
	for name, domain := range tree.Domains {
		require.Len(t, domain.Skills, 1, name)
		var got []string
		for _, r := range domain.Skills[0].Resources {
			got = append(got, filepath.Base(r.RelPath))
		}
		assert.Equal(t, []string{"r.md"}, got, "%s: the gitignored resource is left out", name)
	}
}

// The skills of a lint run share one set of git answers (WithBundleSnapshots).
// Each skill's resources must be what loading it on its own gives, whatever
// state its entry file is in: tracked, untracked, ignored, or ignored though
// tracked. The shared set asks git once for the entry files of the repository
// instead of two processes per skill.
func TestLoadSkillDirSharedSnapshotsMatchPerSkillGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	require.NoError(t, gitutil.CommandNoContext(root, "init", "-q").Run())
	write := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	skill := func(dir string) {
		write(dir+"/SKILL.md", "---\nname: "+filepath.Base(dir)+"\ndescription: d\n---\n# s\n")
		write(dir+"/references/r.md", "# ref\n")
		write(dir+"/references/skip.log", "log\n")
	}
	write(".gitignore", "*.log\nhidden-skill/\ntracked-ignored/SKILL.md\n")
	for _, dir := range []string{
		".claude/skills/tracked", ".claude/skills/untracked", ".claude/skills/hidden-skill",
		".claude/skills/tracked-ignored", "pkg/.claude/skills/other",
	} {
		skill(dir)
	}
	require.NoError(t, gitutil.CommandNoContext(root, "add", ".gitignore", ".claude/skills/tracked").Run())
	require.NoError(t, gitutil.CommandNoContext(root, "add", "-f", ".claude/skills/tracked-ignored", "pkg").Run())

	shared := WithBundleSnapshots(context.Background(), root)
	files, ok, err := gitutil.Git{}.ListFilesContext(context.Background(), root)
	require.NoError(t, err)
	require.True(t, ok)
	SeedBundleListing(shared, files)

	for _, dir := range []string{
		".claude/skills/tracked", ".claude/skills/untracked", ".claude/skills/hidden-skill",
		".claude/skills/tracked-ignored", "pkg/.claude/skills/other",
	} {
		abs := filepath.Join(root, filepath.FromSlash(dir))
		want, wantOK := LoadSkillDir(context.Background(), root, abs)
		got, gotOK := LoadSkillDir(shared, root, abs)
		require.Equal(t, wantOK, gotOK, dir)
		var wantRes, gotRes []string
		for _, r := range want.Resources {
			wantRes = append(wantRes, r.RelPath)
		}
		for _, r := range got.Resources {
			gotRes = append(gotRes, r.RelPath)
		}
		assert.Equal(t, wantRes, gotRes, "resources of %s", dir)
	}
}

// Loading many skills under shared snapshots costs a fixed number of git
// processes, not two per skill.
func TestLoadSkillDirSharedSnapshotsAskGitOnce(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	require.NoError(t, gitutil.CommandNoContext(root, "init", "-q").Run())
	var dirs []string
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		dir := filepath.Join(root, ".claude", "skills", name)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "references"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: d\n---\n# s\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "references", "r.md"), []byte("# r\n"), 0o644))
		dirs = append(dirs, dir)
	}
	require.NoError(t, gitutil.CommandNoContext(root, "add", ".").Run())

	all := &atomic.Int32{}
	ctx := runner.WithContext(context.Background(), countingRunner{lsFiles: &atomic.Int32{}, all: all})
	ctx = WithBundleSnapshots(ctx, root)
	files, ok, err := gitutil.New(runner.FromContext(ctx)).ListFilesContext(ctx, root)
	require.NoError(t, err)
	require.True(t, ok)
	SeedBundleListing(ctx, files)
	before := all.Load()
	for _, dir := range dirs {
		skill, found := LoadSkillDir(ctx, root, dir)
		require.True(t, found)
		require.Len(t, skill.Resources, 1)
	}
	assert.LessOrEqual(t, all.Load()-before, int32(1), "one batched check-ignore for all %d skills", len(dirs))
}

// An entry file that is on disk but missing from the git listing is not thereby
// ignored: on a case-insensitive filesystem the listing can name it with another
// case (skill.md for SKILL.md). The shared snapshots ask git about such a file
// instead of assuming, so the skill's resources are those loading it alone gives.
func TestLoadSkillDirSharedSnapshotsAskGitAboutAnUnlistedEntryFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	require.NoError(t, gitutil.CommandNoContext(root, "init", "-q").Run())
	dir := filepath.Join(root, ".claude", "skills", "odd")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "references"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: odd\ndescription: d\n---\n# s\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "references", "r.md"), []byte("# r\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "references", "skip.log"), []byte("log\n"), 0o644))
	require.NoError(t, gitutil.CommandNoContext(root, "add", ".").Run())

	shared := WithBundleSnapshots(context.Background(), root)
	files, ok, err := gitutil.Git{}.ListFilesContext(context.Background(), root)
	require.NoError(t, err)
	require.True(t, ok)
	var unlisted []string
	for _, f := range files {
		if filepath.Base(f) != "SKILL.md" {
			unlisted = append(unlisted, f)
		}
	}
	require.Less(t, len(unlisted), len(files), "the entry file is removed from the listing")
	SeedBundleListing(shared, unlisted)

	want, wantOK := LoadSkillDir(context.Background(), root, dir)
	got, gotOK := LoadSkillDir(shared, root, dir)

	require.True(t, wantOK)
	require.Equal(t, wantOK, gotOK)
	paths := func(s ContentFile) (out []string) {
		for _, r := range s.Resources {
			out = append(out, r.RelPath)
		}
		return out
	}
	assert.Equal(t, paths(want), paths(got))
	assert.NotContains(t, paths(got), "references/skip.log")
}
