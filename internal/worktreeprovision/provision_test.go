package worktreeprovision

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixture is one repository with a committed AI-Rulez configuration and a
// managed ignore block, and one linked worktree cut from it.
type fixture struct {
	root, src, linked, branch string
	// initRef is the first, empty commit: the ref a worktree with no
	// configuration checks out.
	initRef       string
	generateCalls []generateCall
}

type generateCall struct {
	projectDir string
	dryRun     bool
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func write(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// newFixture builds the source checkout and a linked worktree, both holding
// the same committed configuration.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	src := filepath.Join(root, "main")
	write(t, filepath.Join(src, ".ai-rulez/config.toml"),
		"version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n")
	// The managed .gitignore block a real generate run writes and the project
	// commits, so the destination inherits the ignore protection.
	write(t, filepath.Join(src, ".gitignore"),
		"# BEGIN ai-rulez (DO NOT EDIT - managed by ai-rulez)\n"+
			".ai-rulez/config.local.toml\n"+
			".ai-rulez/local/\n"+
			"CLAUDE.local.md\n"+
			".claude/settings.local.json\n"+
			"# END ai-rulez\n")
	gitRun(t, src, "init", "-q")
	gitRun(t, src, "commit", "-q", "--allow-empty", "-m", "init")
	initRef := gitOut(t, src, "rev-parse", "HEAD")
	gitRun(t, src, "add", ".ai-rulez/config.toml", ".gitignore")
	gitRun(t, src, "commit", "-q", "-m", "config")
	linked := filepath.Join(root, "linked")
	gitRun(t, src, "worktree", "add", "-q", linked, "-b", "feature")
	return &fixture{root: root, src: src, linked: linked, branch: "feature", initRef: initRef}
}

// gitOut runs git and returns its trimmed standard output.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
	out, err := cmd.Output()
	require.NoError(t, err, "git "+strings.Join(args, " "))
	return strings.TrimSpace(string(out))
}

// generate records every call and returns fixed plan lines, as the CLI's
// generate step would.
func (f *fixture) generate(_ context.Context, projectDir string, dryRun bool) ([]string, error) {
	f.generateCalls = append(f.generateCalls, generateCall{projectDir: projectDir, dryRun: dryRun})
	return []string{"generate-plan-line"}, nil
}

func (f *fixture) opts(t *testing.T, dryRun bool) *Options {
	t.Helper()
	return &Options{
		Source:      f.src,
		Destination: f.linked,
		DryRun:      dryRun,
		Generate:    f.generate,
	}
}

func TestSetupProvisionsAnExistingWorktree(t *testing.T) {
	f := newFixture(t)

	res, err := Setup(context.Background(), f.opts(t, false))

	require.NoError(t, err)
	assert.False(t, res.DryRun)
	assert.Empty(t, res.Copies, "no local inputs exist yet, nothing to carry")
	assert.True(t, res.Generated)
	require.Len(t, f.generateCalls, 1)
	assert.Equal(t, f.linked, f.generateCalls[0].projectDir)
	assert.False(t, f.generateCalls[0].dryRun)
}

func TestSetupCarriesTheLocalInputs(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, ".ai-rulez/config.local.toml"), "name = \"local\"\n")
	require.NoError(t, os.MkdirAll(filepath.Join(f.src, ".ai-rulez/local/rules"), 0o700))
	write(t, filepath.Join(f.src, ".ai-rulez/local/rules/scratch.md"), "# scratch\n")
	write(t, filepath.Join(f.src, "CLAUDE.local.md"), "# local\n")
	write(t, filepath.Join(f.src, ".claude/settings.local.json"), "{}\n")

	opts := f.opts(t, false)
	opts.CopyLocal = true
	res, err := Setup(context.Background(), opts)

	require.NoError(t, err)
	got := map[string]string{}
	for _, e := range res.Copies {
		got[e.Destination] = e.Action
	}
	assert.Equal(t, ActionCopy, got[".ai-rulez/config.local.toml"])
	assert.Equal(t, ActionCopy, got[".ai-rulez/local/rules/scratch.md"])
	assert.Equal(t, ActionCopy, got["CLAUDE.local.md"])
	assert.Equal(t, ActionCopy, got[".claude/settings.local.json"])
	for _, rel := range []string{".ai-rulez/config.local.toml", "CLAUDE.local.md"} {
		data, err := os.ReadFile(filepath.Join(f.linked, filepath.FromSlash(rel)))
		require.NoError(t, err, rel)
		assert.NotEmpty(t, data)
	}
	// Owner-only permissions on everything the run copied.
	info, err := os.Stat(filepath.Join(f.linked, ".ai-rulez/config.local.toml"))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
	dirInfo, err := os.Stat(filepath.Join(f.linked, ".ai-rulez/local/rules"))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o700), dirInfo.Mode().Perm())
}

func TestSetupWithoutConsentCarriesNothing(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, "CLAUDE.local.md"), "# local\n")

	res, err := Setup(context.Background(), f.opts(t, false))

	require.NoError(t, err)
	assert.Empty(t, res.Copies)
	_, err = os.Stat(filepath.Join(f.linked, "CLAUDE.local.md"))
	assert.True(t, os.IsNotExist(err))
}

func TestSetupDryRunWritesNothing(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, "CLAUDE.local.md"), "# local\n")

	opts := f.opts(t, true)
	opts.CopyLocal = true
	res, err := Setup(context.Background(), opts)

	require.NoError(t, err)
	assert.True(t, res.DryRun)
	require.Len(t, res.Copies, 1)
	assert.Equal(t, ActionCopy, res.Copies[0].Action)
	assert.Equal(t, "CLAUDE.local.md", res.Copies[0].Destination)
	assert.True(t, f.generateCalls[0].dryRun)
	_, err = os.Stat(filepath.Join(f.linked, "CLAUDE.local.md"))
	assert.True(t, os.IsNotExist(err), "a dry run writes nothing")
}

func TestSetupTreatsAnIdenticalFileAsANoOp(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, "CLAUDE.local.md"), "# local\n")
	write(t, filepath.Join(f.linked, "CLAUDE.local.md"), "# local\n")

	opts := f.opts(t, false)
	opts.CopyLocal = true
	res, err := Setup(context.Background(), opts)

	require.NoError(t, err)
	require.Len(t, res.Copies, 1)
	assert.Equal(t, ActionIdentical, res.Copies[0].Action)
	data, err := os.ReadFile(filepath.Join(f.linked, "CLAUDE.local.md"))
	require.NoError(t, err)
	assert.Equal(t, "# local\n", string(data))
}

func TestSetupRefusesAConflictingDestinationFile(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, "CLAUDE.local.md"), "# source\n")
	write(t, filepath.Join(f.linked, "CLAUDE.local.md"), "# the agent's own edit\n")

	opts := f.opts(t, false)
	opts.CopyLocal = true
	_, err := Setup(context.Background(), opts)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLAUDE.local.md")
	data, err := os.ReadFile(filepath.Join(f.linked, "CLAUDE.local.md"))
	require.NoError(t, err)
	assert.Equal(t, "# the agent's own edit\n", string(data), "the destination edit is preserved")
}

func TestSetupRefusesAnUnrelatedRepository(t *testing.T) {
	f := newFixture(t)
	other := filepath.Join(f.root, "other")
	write(t, filepath.Join(other, "README"), "x\n")
	gitRun(t, other, "init", "-q")
	otherLinked := filepath.Join(f.root, "other-linked")
	gitRun(t, other, "worktree", "add", "-q", otherLinked, "-b", "o")

	_, err := Setup(context.Background(), &Options{Source: f.src, Destination: otherLinked, Generate: f.generate})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a registered linked worktree")
}

func TestSetupRefusesAWorktreeGitDoesNotRegister(t *testing.T) {
	f := newFixture(t)
	stray := filepath.Join(f.root, "stray")
	require.NoError(t, os.MkdirAll(stray, 0o755))
	write(t, filepath.Join(stray, ".ai-rulez/config.toml"), "version = \"5.0\"\nname = \"t\"\n")

	_, err := Setup(context.Background(), &Options{Source: f.src, Destination: stray, Generate: f.generate})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a registered linked worktree")
}

func TestSetupRefusesADestinationWithoutConfiguration(t *testing.T) {
	f := newFixture(t)
	// A worktree whose branch has no configuration yet: cut from the empty
	// init commit, before the configuration was made.
	plain := filepath.Join(f.root, "plain")
	gitRun(t, f.src, "worktree", "add", "-q", plain, "--detach", f.initRef)

	_, err := Setup(context.Background(), &Options{Source: f.src, Destination: plain, Generate: f.generate})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "holds no AI-Rulez configuration")
}

func TestSetupRefusesATrackedCopyTarget(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, "tracked.local.md"), "# tracked\n")
	write(t, filepath.Join(f.linked, "tracked.local.md"), "") // staged empty
	gitRun(t, f.linked, "add", "tracked.local.md")
	gitRun(t, f.linked, "commit", "-q", "-m", "tracked local file")

	_, err := Setup(context.Background(), &Options{
		Source: f.src, Destination: f.linked, Generate: f.generate,
		CopyExtra: []string{"tracked.local.md"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "is tracked by git in the destination")
}

func TestSetupRefusesAnUnignoredCopyTarget(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, "unignored.md"), "# unignored\n")

	_, err := Setup(context.Background(), &Options{
		Source: f.src, Destination: f.linked, Generate: f.generate,
		CopyExtra: []string{"unignored.md"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not ignored by git in the destination")
}

func TestSetupRefusesASymlinkedExtra(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, "real.md"), "# real\n")
	require.NoError(t, os.Symlink("real.md", filepath.Join(f.src, "link.local.md")))
	// The managed block ignores the link's path, but a symlink is never carried.
	gitRun(t, f.linked, "config", "core.symlinks", "true")

	_, err := Setup(context.Background(), &Options{
		Source: f.src, Destination: f.linked, Generate: f.generate,
		CopyExtra: []string{"link.local.md"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a symlink")
}

func TestSetupRefusesAnExtraOutsideTheProject(t *testing.T) {
	f := newFixture(t)

	_, err := Setup(context.Background(), &Options{
		Source: f.src, Destination: f.linked, Generate: f.generate,
		CopyExtra: []string{"../outside.md"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a relative path inside the project")
}

func TestSetupRefusesAMissingExtra(t *testing.T) {
	f := newFixture(t)

	_, err := Setup(context.Background(), &Options{
		Source: f.src, Destination: f.linked, Generate: f.generate,
		CopyExtra: []string{"missing.local.md"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read the extra missing.local.md")
}

func TestSetupCarriesANamedExtra(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, "notes.local.md"), "# notes\n")
	// The destination is what check-ignore consults, so the ignore rule is
	// committed on the branch the linked worktree holds.
	write(t, filepath.Join(f.linked, ".gitignore"), strings.Join([]string{
		"# BEGIN ai-rulez (DO NOT EDIT - managed by ai-rulez)",
		".ai-rulez/config.local.toml", ".ai-rulez/local/", "CLAUDE.local.md",
		".claude/settings.local.json", "notes.local.md", "# END ai-rulez\n",
	}, "\n"))
	gitRun(t, f.linked, "add", ".gitignore")
	gitRun(t, f.linked, "commit", "-q", "-m", "ignore notes")

	res, err := Setup(context.Background(), &Options{
		Source: f.src, Destination: f.linked, Generate: f.generate,
		CopyExtra: []string{"notes.local.md"},
	})

	require.NoError(t, err)
	require.Len(t, res.Copies, 1)
	assert.Equal(t, "notes.local.md", res.Copies[0].Destination)
	data, err := os.ReadFile(filepath.Join(f.linked, "notes.local.md"))
	require.NoError(t, err)
	assert.Equal(t, "# notes\n", string(data))
}

func TestSetupRefusesALayoutMismatchWhenTheOverlayWouldStrand(t *testing.T) {
	f := newFixture(t)
	// A source whose configuration lives in .config/ai-rulez with an overlay:
	// the destination's .ai-rulez layout would strand the copied overlay.
	require.NoError(t, os.RemoveAll(filepath.Join(f.src, ".ai-rulez")))
	write(t, filepath.Join(f.src, ".config/ai-rulez/config.toml"),
		"version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n")
	write(t, filepath.Join(f.src, ".config/ai-rulez/config.local.toml"), "name = \"local\"\n")
	// The destination keeps .ai-rulez (inherited), so the layouts differ.

	_, err := Setup(context.Background(), &Options{
		Source: f.src, Destination: f.linked, Generate: f.generate, CopyLocal: true,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "configuration layout differs")
}

func TestCreateMakesTheWorktreeAndProvisionsIt(t *testing.T) {
	f := newFixture(t)
	dest := filepath.Join(f.root, "created")

	opts := f.opts(t, false)
	opts.Destination = dest
	res, err := Create(context.Background(), opts, "provisioned", "")

	require.NoError(t, err)
	assert.Equal(t, dest, res.Destination)
	g := gitutil.Git{}
	assert.True(t, g.BranchExists(context.Background(), f.src, "provisioned"))
	infos, err := g.WorktreesContext(context.Background(), f.src)
	require.NoError(t, err)
	registered := false
	resolved, err := filepath.EvalSymlinks(dest)
	require.NoError(t, err)
	for _, info := range infos {
		if info.Path == resolved {
			registered = true
		}
	}
	assert.True(t, registered)
	require.Len(t, f.generateCalls, 1)
	assert.Equal(t, dest, f.generateCalls[0].projectDir)
}

func TestCreateUsesTheStartRef(t *testing.T) {
	f := newFixture(t)
	gitRun(t, f.src, "commit", "-q", "--allow-empty", "-m", "second")
	dest := filepath.Join(f.root, "at-old-ref")

	opts := f.opts(t, false)
	opts.Destination = dest
	_, err := Create(context.Background(), opts, "from-old", "HEAD~1")

	require.NoError(t, err)
	head, err := gitutil.Git{}.Output(context.Background(), dest, "rev-parse", "HEAD")
	require.NoError(t, err)
	base, err := gitutil.Git{}.Output(context.Background(), f.src, "rev-parse", "HEAD~1")
	require.NoError(t, err)
	assert.Equal(t, base, head, "the worktree starts at the named ref, not the source HEAD")
	_ = dest
}

func TestCreateRefusesAnExistingBranch(t *testing.T) {
	f := newFixture(t)
	dest := filepath.Join(f.root, "created")

	_, err := Create(context.Background(), f.opts(t, false), "feature", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	_, statErr := os.Stat(dest)
	assert.True(t, os.IsNotExist(statErr), "the worktree is not created")
}

func TestCreateRefusesABadBranchName(t *testing.T) {
	f := newFixture(t)

	_, err := Create(context.Background(), f.opts(t, false), "--evil", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
}

func TestSetupRefusesAMissingGenerateStep(t *testing.T) {
	f := newFixture(t)

	_, err := Setup(context.Background(), &Options{Source: f.src, Destination: f.linked})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no generate step")
}

func TestCreateDryRunMutatesNothing(t *testing.T) {
	f := newFixture(t)
	dest := filepath.Join(f.root, "planned")
	g := gitutil.Git{}
	ctx := context.Background()

	opts := f.opts(t, true)
	opts.Destination = dest
	opts.CopyLocal = true
	write(t, filepath.Join(f.src, "CLAUDE.local.md"), "# local\n")
	res, err := Create(ctx, opts, "planned-branch", "")

	require.NoError(t, err)
	assert.True(t, res.DryRun)
	assert.Empty(t, f.generateCalls, "a dry run does not generate")
	assert.False(t, g.BranchExists(ctx, f.src, "planned-branch"), "no branch is created")
	_, err = os.Stat(dest)
	assert.True(t, os.IsNotExist(err), "no worktree directory is created")
	infos, err := g.WorktreesContext(ctx, f.src)
	require.NoError(t, err)
	for _, info := range infos {
		assert.NotEqual(t, dest, info.Path, "no worktree is registered")
	}
}

func TestCreateDryRunStillChecksTheBranchAndStartRef(t *testing.T) {
	f := newFixture(t)
	dest := filepath.Join(f.root, "planned")

	opts := f.opts(t, true)
	opts.Destination = dest
	_, err := Create(context.Background(), opts, "feature", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists", "the branch check runs without mutation")

	opts2 := f.opts(t, true)
	opts2.Destination = dest
	_, err = Create(context.Background(), opts2, "planned", "no-such-ref")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a commit")
}

func TestSetupDryRunNamesTheGeneratePreviewLimit(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, ".ai-rulez/config.local.toml"), "# overlay\n")
	write(t, filepath.Join(f.src, "CLAUDE.local.md"), "# local\n")

	opts := f.opts(t, true)
	opts.CopyLocal = true
	res, err := Setup(context.Background(), opts)

	require.NoError(t, err)
	assert.NotEmpty(t, res.Plan)
	found := false
	for _, line := range res.Plan {
		if strings.Contains(line, "machine-local") && strings.Contains(line, "not yet carried") {
			found = true
		}
	}
	assert.True(t, found, "the dry run names the overlay limitation: %v", res.Plan)
}

// generatedLocalMD is a generated local file: a leading banner comment, the
// shape genmark detects without needing a verifiable hash in the test.
func generatedLocalMD(name, body string) string {
	return "<!--\n" +
		"\U0001F916 AI-RULEZ :: GENERATED FILE — DO NOT EDIT\n" +
		"Project: t\n" +
		"Source: .ai-rulez/config.toml\n" +
		"NEVER edit this file - modify .ai-rulez/ content instead\n" +
		"-->\n\n" + body
}

func TestSetupSkipsGeneratedAllowlistFiles(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, "CLAUDE.local.md"), generatedLocalMD("CLAUDE.local.md", "# generated\n"))
	write(t, filepath.Join(f.src, "AGENTS.local.md"), "# authored\n")
	write(t, filepath.Join(f.linked, ".gitignore"), strings.Join([]string{
		"# BEGIN ai-rulez (DO NOT EDIT - managed by ai-rulez)",
		".ai-rulez/config.local.toml", ".ai-rulez/local/", "CLAUDE.local.md",
		"AGENTS.local.md", "# END ai-rulez\n",
	}, "\n"))
	gitRun(t, f.linked, "add", ".gitignore")
	gitRun(t, f.linked, "commit", "-q", "-m", "ignore the local files")

	opts := f.opts(t, false)
	opts.CopyLocal = true
	res, err := Setup(context.Background(), opts)

	require.NoError(t, err)
	got := map[string]string{}
	for _, e := range res.Copies {
		got[e.Destination] = e.Action
	}
	assert.NotContains(t, got, "CLAUDE.local.md", "a generated file is regenerated, not copied")
	assert.Equal(t, ActionCopy, got["AGENTS.local.md"], "an authored file is carried")
	assert.NoFileExists(t, filepath.Join(f.linked, "CLAUDE.local.md"))
}

func TestSetupRefusesAGeneratedExtra(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, "carry-me.local.md"), generatedLocalMD("carry-me.local.md", "# generated\n"))
	// The destination is what the ignore check consults, so the rule is
	// committed on the branch the linked worktree holds.
	write(t, filepath.Join(f.linked, ".gitignore"), strings.Join([]string{
		"# BEGIN ai-rulez (DO NOT EDIT - managed by ai-rulez)",
		".ai-rulez/config.local.toml", ".ai-rulez/local/", "CLAUDE.local.md",
		"carry-me.local.md", "# END ai-rulez\n",
	}, "\n"))
	gitRun(t, f.linked, "add", ".gitignore")
	gitRun(t, f.linked, "commit", "-q", "-m", "ignore carry-me")

	_, err := Setup(context.Background(), &Options{
		Source: f.src, Destination: f.linked, Generate: f.generate,
		CopyExtra: []string{"carry-me.local.md"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "generated")
}

func TestSetupDedupesOverlappingExtras(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.src, "notes.local.md"), "# notes\n")
	write(t, filepath.Join(f.src, "extra.d/extra.local.md"), "# extra\n")
	// The destination is what the ignore check consults, so the rule is
	// committed on the branch the linked worktree holds.
	write(t, filepath.Join(f.linked, ".gitignore"), strings.Join([]string{
		"# BEGIN ai-rulez (DO NOT EDIT - managed by ai-rulez)",
		".ai-rulez/config.local.toml", ".ai-rulez/local/", "CLAUDE.local.md",
		"notes.local.md", "extra.d/", "# END ai-rulez\n",
	}, "\n"))
	gitRun(t, f.linked, "add", ".gitignore")
	gitRun(t, f.linked, "commit", "-q", "-m", "ignore extras")

	opts := &Options{
		Source: f.src, Destination: f.linked, Generate: f.generate,
		// The directory and the file it contains overlap; the plan dedupes.
		CopyExtra: []string{"notes.local.md", "extra.d", "extra.d/extra.local.md"},
	}
	_, err := Setup(context.Background(), opts)

	require.NoError(t, err)
}

func TestSetupMapsANestedSourceProjectIntoTheWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	// The project lives in services/billing below the repository top.
	proj := filepath.Join(root, "services", "billing")
	write(t, filepath.Join(proj, ".ai-rulez/config.toml"),
		"version = \"5.0\"\nname = \"nested\"\ngitignore = false\npresets = [\"claude\"]\n")
	write(t, filepath.Join(proj, "services-billing.gitignore"),
		"# BEGIN ai-rulez (DO NOT EDIT - managed by ai-rulez)\n.ai-rulez/config.local.toml\n.ai-rulez/local/\nCLAUDE.local.md\n# END ai-rulez\n")
	require.NoError(t, os.Rename(filepath.Join(proj, "services-billing.gitignore"), filepath.Join(proj, ".gitignore")))
	write(t, filepath.Join(proj, "CLAUDE.local.md"), "# local\n")
	gitRun(t, root, "init", "-q")
	gitRun(t, root, "commit", "-q", "--allow-empty", "-m", "init")
	gitRun(t, root, "add", "services/billing/.ai-rulez/config.toml", "services/billing/.gitignore")
	gitRun(t, root, "commit", "-q", "-m", "config")
	linked := filepath.Join(root, "linked")
	gitRun(t, root, "worktree", "add", "-q", linked, "-b", "feature")

	res, err := Setup(context.Background(), &Options{
		Source: proj, Destination: linked, CopyLocal: true, Generate: f_generate,
	})

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(linked, "services", "billing"), res.Destination,
		"the destination project is the worktree's copy of the nested project")
	data, err := os.ReadFile(filepath.Join(linked, "services", "billing", "CLAUDE.local.md"))
	require.NoError(t, err)
	assert.Equal(t, "# local\n", string(data))
}

// f_generate is a generate stub the nested-project test shares.
func f_generate(_ context.Context, _ string, _ bool) ([]string, error) { return nil, nil }

func TestSetupRefusesADestinationWithoutTheNestedProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	proj := filepath.Join(root, "services", "billing")
	write(t, filepath.Join(proj, ".ai-rulez/config.toml"),
		"version = \"5.0\"\nname = \"nested\"\ngitignore = false\npresets = [\"claude\"]\n")
	gitRun(t, root, "init", "-q")
	gitRun(t, root, "commit", "-q", "--allow-empty", "-m", "init")
	// The configuration is committed only in the project subtree of a LATER
	// commit; the worktree checks out the init commit without it.
	gitRun(t, root, "add", "services/billing/.ai-rulez/config.toml")
	gitRun(t, root, "commit", "-q", "-m", "config")
	linked := filepath.Join(root, "linked")
	gitRun(t, root, "worktree", "add", "-q", linked, "--detach", "HEAD~1")

	_, err := Setup(context.Background(), &Options{
		Source: proj, Destination: linked, Generate: f_generate,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "services/billing")
}
