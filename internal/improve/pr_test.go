package improve

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// gitIn runs git in dir with a world that cannot sign or read the user's configuration.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := gitutil.CommandNoContext("", append([]string{"-C", dir, "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "user.email=t@example.test", "-c", "user.name=t"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

// prWorld is an accepted run inside a committed git repository with a bare origin and a fake gh.
type prWorld struct {
	root, configDir, remote, ghLog, ghBin string
	plan                                  *Plan
	report                                *Report
	opts                                  PROptions
	out                                   *strings.Builder
	// looked are the programs PR asked LookPath for; the fake gh must be what it resolved.
	looked []string
}

func newPRWorld(t *testing.T) *prWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root, configDir, plan, report := acceptedRun(t)
	gitIn(t, root, "init", "-q", "-b", "main")
	gitIn(t, root, "add", ".ai-rulez/config.toml", ".ai-rulez/skills")
	gitIn(t, root, "commit", "-q", "-m", "init")
	remote := filepath.Join(t.TempDir(), "origin.git")
	gitIn(t, root, "init", "-q", "--bare", remote)
	gitIn(t, root, "remote", "add", "origin", remote)
	gitIn(t, root, "push", "-q", "origin", "main") // the base is a branch the remote has, as in a real checkout

	bin := t.TempDir()
	ghLog := filepath.Join(t.TempDir(), "gh.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$GH_LOG\"\necho 'https://github.com/example/repo/pull/7'\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755)) //nolint:gosec // a test script
	w := &prWorld{root: root, configDir: configDir, remote: remote, ghLog: ghLog, ghBin: filepath.Join(bin, "gh"), plan: plan, report: report, out: &strings.Builder{}}
	w.opts = PROptions{
		ConfigDir: configDir, RepoDir: root, RunID: plan.RunID, Out: w.out, Yes: true,
		Exec: runner.Exec{}, LookPath: func(name string) (string, error) { w.looked = append(w.looked, name); return w.ghBin, nil },
		GHEnv: []string{"PATH=/usr/bin:/bin", "GH_LOG=" + ghLog}, Env: []string{"PATH=/usr/bin:/bin"},
		Git: gitutil.Git{},
	}
	return w
}

func (w *prWorld) status(t *testing.T) string {
	t.Helper()
	return gitIn(t, w.root, "status", "--porcelain", "--", ".ai-rulez/skills", ".ai-rulez/config.toml")
}

func TestPR_OpensAPullRequestFromAnIsolatedWorktree(t *testing.T) {
	// Arrange
	w := newPRWorld(t)
	headBefore := gitIn(t, w.root, "rev-parse", "HEAD")
	skillBefore := readFileString(t, filepath.Join(w.configDir, "skills/deploy/SKILL.md"))

	// Act
	res, err := PR(context.Background(), &w.opts)

	// Assert: the user's checkout is untouched.
	require.NoError(t, err, w.out.String())
	assert.Equal(t, headBefore, gitIn(t, w.root, "rev-parse", "HEAD"))
	assert.Equal(t, "main", gitIn(t, w.root, "symbolic-ref", "--short", "HEAD"))
	assert.Empty(t, w.status(t), "the working tree and index of the checkout did not change")
	assert.Equal(t, skillBefore, readFileString(t, filepath.Join(w.configDir, "skills/deploy/SKILL.md")))
	assert.Equal(t, 1, strings.Count(gitIn(t, w.root, "worktree", "list"), "\n")+1, "the worktree is gone")

	// the branch carries one commit with only the skill files
	assert.Equal(t, "ai-rulez/improve/deploy-"+strings.TrimPrefix(w.report.CandidateDigest, "sha256:")[:8], res.Branch)
	assert.Equal(t, "main", res.Base)
	files := gitIn(t, w.root, "show", "--name-only", "--format=", res.Branch)
	assert.ElementsMatch(t, []string{".ai-rulez/skills/deploy/SKILL.md", ".ai-rulez/skills/deploy/references/extra.md"}, strings.Split(files, "\n"))
	assert.Contains(t, gitIn(t, w.root, "show", res.Branch+":.ai-rulez/skills/deploy/SKILL.md"), "GOOD advice.")
	msg := gitIn(t, w.root, "log", "-1", "--format=%B", res.Branch)
	assert.True(t, strings.HasPrefix(msg, "chore(skills): improve deploy"), msg)
	assert.Contains(t, msg, "Not approved")
	assert.NotContains(t, strings.ToLower(msg), "co-authored-by")

	// pushed and opened with fixed gh arguments
	assert.True(t, res.Pushed)
	assert.Equal(t, []string{"gh"}, w.looked, "gh was resolved through the injected lookup, which returned the fake")
	assert.Equal(t, "https://github.com/example/repo/pull/7", res.URL)
	assert.Equal(t, res.Commit, gitIn(t, w.remote, "rev-parse", "refs/heads/"+res.Branch))
	args := strings.Split(strings.TrimSpace(readFileString(t, w.ghLog)), "\n")
	assert.Equal(t, []string{
		"pr", "create", "--base", "main", "--head", res.Branch,
		"--title", "chore(skills): improve deploy with an eval-gated candidate", "--body-file", res.BodyFile,
	}, args)
	body := readFileString(t, res.BodyFile)
	for _, want := range []string{"**Not approved**", "Reviewer checklist", "Read the full diff", "Wins:", "95% bootstrap interval", "Sibling trigger guard"} {
		assert.Contains(t, body, want)
	}
}

func TestPR_DraftFlagAndCustomBase(t *testing.T) {
	// Arrange
	w := newPRWorld(t)
	gitIn(t, w.root, "branch", "release")
	gitIn(t, w.root, "push", "-q", "origin", "release")
	w.opts.Base, w.opts.Draft = "release", true

	// Act
	res, err := PR(context.Background(), &w.opts)

	// Assert
	require.NoError(t, err, w.out.String())
	args := strings.Split(strings.TrimSpace(readFileString(t, w.ghLog)), "\n")
	assert.Equal(t, []string{"--base", "release"}, args[2:4])
	assert.Equal(t, "--draft", args[len(args)-1])
	assert.Equal(t, "release", res.Base)
}

func TestPR_PrintsCommandsInsteadOfPushing(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(w *prWorld)
		want   string
	}{
		{"gh missing", func(w *prWorld) {
			w.opts.LookPath = func(string) (string, error) { return "", errors.New("not found") }
		}, "gh (the GitHub CLI) was not found"},
		{"no push", func(w *prWorld) { w.opts.NoPush = true }, "--no-push"},
		{"no remote", func(w *prWorld) { gitIn(t, w.root, "remote", "remove", "origin") }, "does not exist"},
		{"not confirmed", func(w *prWorld) { w.opts.Yes, w.opts.Confirm = false, func(string) bool { return false } }, "Not confirmed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			w := newPRWorld(t)
			tt.mutate(w)

			// Act
			res, err := PR(context.Background(), &w.opts)

			// Assert
			require.NoError(t, err, w.out.String())
			assert.False(t, res.Pushed)
			assert.Contains(t, w.out.String(), tt.want)
			assert.Contains(t, w.out.String(), "git push --set-upstream origin "+res.Branch)
			assert.Contains(t, w.out.String(), "gh pr create --base main --head "+res.Branch)
			assert.NoFileExists(t, w.ghLog, "gh was never run")
			assert.Contains(t, gitIn(t, w.root, "branch", "--list", res.Branch), res.Branch, "the commit stays on the local branch")
		})
	}
}

func TestPR_Refusals(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(t *testing.T, w *prWorld)
		code     string
		text     string
		noBranch bool
	}{
		{"unsigned run", func(t *testing.T, w *prWorld) {
			require.NoError(t, os.Remove(filepath.Join(w.plan.RunDir(), ReportMACFile)))
		}, CodePRRefused, "not signed", false},
		{"unknown base", func(_ *testing.T, w *prWorld) { w.opts.Base = "nope" }, CodePRRefused, "not a commit", false},
		{"option as base", func(_ *testing.T, w *prWorld) { w.opts.Base = "--all" }, CodePRRefused, "not a commit", false},
		{"detached head without a base", func(t *testing.T, w *prWorld) { gitIn(t, w.root, "checkout", "-q", "--detach") }, CodePRRefused, "detached", false},
		{"branch exists", func(t *testing.T, w *prWorld) {
			gitIn(t, w.root, "branch", "ai-rulez/improve/deploy-"+strings.TrimPrefix(w.report.CandidateDigest, "sha256:")[:8])
		}, CodePRRefused, "already exists", false},
		{"skill differs at the base", func(t *testing.T, w *prWorld) {
			appendSkill(t, filepath.Join(w.configDir, "skills/deploy"), "\nlater edit\n")
			gitIn(t, w.root, "commit", "-q", "-am", "edit the skill")
		}, CodeRunStale, "differs", true},
		{"candidate tampered", func(t *testing.T, w *prWorld) {
			p := filepath.Join(w.plan.RunDir(), "rounds", "1", "candidate", "deploy", "SKILL.md")
			require.NoError(t, os.WriteFile(p, []byte("tampered"), 0o600))
		}, CodePRRefused, "digest mismatch", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			w := newPRWorld(t)
			tt.mutate(t, w)

			// Act
			_, err := PR(context.Background(), &w.opts)

			// Assert
			var refusal *Refusal
			require.ErrorAs(t, err, &refusal)
			assert.Equal(t, tt.code, refusal.Code)
			assert.Contains(t, refusal.Error(), tt.text)
			assert.NoFileExists(t, w.ghLog)
			if tt.noBranch {
				assert.Empty(t, gitIn(t, w.root, "branch", "--list", "ai-rulez/*"), "a refused pull request leaves no branch behind")
				assert.NotContains(t, gitIn(t, w.root, "worktree", "list"), "ai-rulez-improve-pr")
			}
		})
	}
}

func TestPR_NotAGitRepository(t *testing.T) {
	// Arrange: an accepted run in a project that is not a repository.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(os.TempDir()))
	root, configDir, plan, _ := acceptedRun(t)
	opts := PROptions{ConfigDir: configDir, RepoDir: root, RunID: plan.RunID, Git: gitutil.Git{}, Yes: true}

	// Act
	_, err := PR(context.Background(), &opts)

	// Assert
	var refusal *Refusal
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, CodePRRefused, refusal.Code)
	assert.Contains(t, refusal.Error(), "not inside a git repository")
}

func TestPR_RefreshesTheLockAndEvalResultsInTheWorktree(t *testing.T) {
	// Arrange: a committed lock and eval results, and a stand-in for ai-rulez that edits them.
	w := newPRWorld(t)
	lock := filepath.Join(w.configDir, "ai-rulez.lock")
	require.NoError(t, os.WriteFile(lock, []byte("version = 1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(w.configDir, "eval-results.json"), []byte("{}\n"), 0o600))
	gitIn(t, w.root, "add", ".ai-rulez/ai-rulez.lock", ".ai-rulez/eval-results.json")
	gitIn(t, w.root, "commit", "-q", "-m", "lock")
	self := filepath.Join(t.TempDir(), "fake-ai-rulez")
	script := "#!/bin/sh\ncase \"$1\" in\n  lock) echo \"# lock refreshed\" >> .ai-rulez/ai-rulez.lock ;;\n  eval) echo \"$@\" > .ai-rulez/eval-args.txt; echo '{\"refreshed\":true}' > .ai-rulez/eval-results.json ;;\nesac\n"
	require.NoError(t, os.WriteFile(self, []byte(script), 0o755)) //nolint:gosec // a test script
	w.opts.Self, w.opts.RunEvals, w.opts.EvalArgs = []string{self}, true, []string{"--max-cost", "2"}

	// Act
	res, err := PR(context.Background(), &w.opts)

	// Assert
	require.NoError(t, err, w.out.String())
	assert.Equal(t, []string{"ai-rulez generate --yes", "ai-rulez lock", "ai-rulez eval run deploy --changed-only --max-cost 2"}, res.Refreshed)
	files := strings.Split(gitIn(t, w.root, "show", "--name-only", "--format=", res.Branch), "\n")
	assert.Contains(t, files, ".ai-rulez/ai-rulez.lock")
	assert.Contains(t, files, ".ai-rulez/eval-results.json")
	assert.NotContains(t, files, ".ai-rulez/eval-args.txt", "only the lock, the eval results and the skill are staged")
	assert.Contains(t, gitIn(t, w.root, "show", res.Branch+":.ai-rulez/ai-rulez.lock"), "# lock refreshed")
	assert.Equal(t, "version = 1", strings.TrimSpace(readFileString(t, lock)), "the checkout's own lock is untouched")
}

func TestPR_AFailingRefreshStopsAndLeavesNothing(t *testing.T) {
	// Arrange
	w := newPRWorld(t)
	require.NoError(t, os.WriteFile(filepath.Join(w.configDir, "ai-rulez.lock"), []byte("version = 1\n"), 0o600))
	gitIn(t, w.root, "add", ".ai-rulez/ai-rulez.lock")
	gitIn(t, w.root, "commit", "-q", "-m", "lock")
	self := filepath.Join(t.TempDir(), "fake-ai-rulez")
	require.NoError(t, os.WriteFile(self, []byte("#!/bin/sh\necho 'lock drift' >&2\nexit 2\n"), 0o755)) //nolint:gosec // a test script
	w.opts.Self = []string{self}

	// Act
	_, err := PR(context.Background(), &w.opts)

	// Assert
	var refusal *Refusal
	require.ErrorAs(t, err, &refusal)
	assert.Contains(t, refusal.Error(), "lock drift")
	assert.Empty(t, gitIn(t, w.root, "branch", "--list", "ai-rulez/*"))
	assert.NoFileExists(t, w.ghLog)
}

func TestPRBody_UntrustedTextCannotBreakOutOfItsFence(t *testing.T) {
	// Arrange
	report := &Report{
		Skill: "deploy", RunID: "imp-12345678", AcceptedRound: 1, Gate: GateReport{MaxRounds: 1}, Runs: 1, Optimizer: "evil`opt", EvalRunner: "fake", Harness: "claude",
		Rounds: []RoundReport{{
			Round: 1, Decision: "accepted", Summary: "```\n# injected heading\n[click](https://evil.example)\x1b[2J", Notes: "<script>x</script> @everyone",
			Description: &DescChange{Before: "a ``` b", After: "[x](https://evil.example) ````` y"},
			Held:        &Comparison{Table: []PairRow{}, Wins: []string{"w`1"}, Losses: []string{}},
		}},
	}

	// Act
	body := prBody(report, "imp-12345678")

	// Assert
	assert.NotContains(t, body, "\x1b")
	assert.Contains(t, body, "````text\n```\n# injected heading", "the summary sits in a fence longer than its own backtick run")
	assert.Contains(t, body, "``````text", "a five-backtick run in the description forces a six-backtick fence")
	assert.Contains(t, body, "``w`1``", "a code span with a backtick is delimited by a longer run")
	assert.Contains(t, body, "``evil`opt``")
	assert.Contains(t, body, "**Not approved**")
}

func TestPRBody_NamesHowManyHeldOutEvaluationsTheSelectionRanOver(t *testing.T) {
	// Arrange: round 2 was accepted after round 1 was also scored on the held-out set.
	held := &Comparison{Table: []PairRow{}, Wins: []string{}, Losses: []string{}}
	report := &Report{
		Skill: "deploy", AcceptedRound: 2, Gate: GateReport{MaxRounds: 3}, Runs: 1,
		Rounds: []RoundReport{{Round: 1, Decision: "rejected", Held: held}, {Round: 2, Decision: "accepted", Held: held}},
	}

	// Act
	body := prBody(report, "imp-12345678")

	// Assert
	assert.Contains(t, body, "selected among 2 held-out evaluations")
	assert.NotContains(t, prBody(&Report{Skill: "deploy", AcceptedRound: 1, Rounds: []RoundReport{{Round: 1, Decision: "accepted", Held: held}}}, "imp-1"), "selected among")
}

func TestParseRepoURL(t *testing.T) {
	tests := []struct{ url, want string }{
		{"https://github.com/example/repo.git", "example/repo"},
		{"https://github.com/example/repo", "example/repo"},
		{"https://user@github.com/example/repo.git/", "example/repo"},
		{"git@github.com:example/repo.git", "example/repo"},
		{"ssh://git@github.com/example/repo.git", "example/repo"},
		{"ssh://git@ghe.example.com:2222/team/tools.git", "ghe.example.com/team/tools"},
		{"git@ghe.example.com:team/tools", "ghe.example.com/team/tools"},
		{"/tmp/origin.git", ""},
		{"../origin.git", ""},
		{"file:///tmp/origin.git", ""},
		{"https://gitlab.com/group/sub/repo.git", ""},
		{"https://github.com/-evil/repo", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.want, parseRepoURL(tt.url))
		})
	}
}

func TestPR_TargetsThePushedRepositoryWithRepoFlag(t *testing.T) {
	// Arrange: the remote's URL names a forge repository, while pushes are redirected to the local bare origin.
	w := newPRWorld(t)
	gitIn(t, w.root, "config", "remote.origin.url", "https://github.com/example/fork.git")
	gitIn(t, w.root, "config", "url."+w.remote+".insteadOf", "https://github.com/example/fork.git")

	// Act
	res, err := PR(context.Background(), &w.opts)

	// Assert
	require.NoError(t, err, w.out.String())
	args := strings.Split(strings.TrimSpace(readFileString(t, w.ghLog)), "\n")
	assert.Equal(t, []string{"pr", "create", "--repo", "example/fork", "--base", "main", "--head", res.Branch}, args[:8])
	assert.Contains(t, strings.Join(res.Commands, "\n"), "--repo example/fork")
}

func TestPR_DoesNotPushWhenTheBaseCannotBeTheTargetOfAPullRequest(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, w *prWorld)
		want   string
	}{
		{"base is a commit id, not a branch", func(t *testing.T, w *prWorld) {
			w.opts.Base = gitIn(t, w.root, "rev-parse", "HEAD")
		}, "is not a branch of origin"},
		{"base is a local branch the remote lacks", func(t *testing.T, w *prWorld) {
			gitIn(t, w.root, "branch", "unpushed")
			w.opts.Base = "unpushed"
		}, "is not a branch of origin"},
		{"base is ahead of the remote", func(t *testing.T, w *prWorld) {
			require.NoError(t, os.WriteFile(filepath.Join(w.root, "README.md"), []byte("local only\n"), 0o600))
			gitIn(t, w.root, "add", "README.md")
			gitIn(t, w.root, "commit", "-q", "-m", "unpushed work")
		}, "1 commit(s) that origin/main does not"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			w := newPRWorld(t)
			tt.mutate(t, w)

			// Act
			res, err := PR(context.Background(), &w.opts)

			// Assert: the commit exists locally, nothing was pushed and gh never ran.
			require.NoError(t, err, w.out.String())
			assert.False(t, res.Pushed)
			assert.Contains(t, w.out.String(), tt.want)
			assert.Contains(t, w.out.String(), "Nothing was pushed")
			assert.Empty(t, gitIn(t, w.remote, "branch", "--list", res.Branch))
			assert.NoFileExists(t, w.ghLog)
		})
	}
}

func TestPR_AFailedBodyWriteLeavesNoBranch(t *testing.T) {
	// Arrange: the body file cannot be written because a directory sits at its path.
	w := newPRWorld(t)
	require.NoError(t, os.MkdirAll(filepath.Join(w.plan.RunDir(), "pr-body.md"), 0o750))

	// Act
	_, err := PR(context.Background(), &w.opts)

	// Assert
	require.Error(t, err)
	assert.Empty(t, gitIn(t, w.root, "branch", "--list", "ai-rulez/*"), "no branch is left behind")
	assert.NoFileExists(t, w.ghLog)
}

func TestPR_RefusesASymlinkedParentOfTheSkill(t *testing.T) {
	// Arrange: the base commit holds .ai-rulez/skills as a symlink to another directory of the repository.
	w := newPRWorld(t)
	gitIn(t, w.root, "mv", ".ai-rulez/skills", ".ai-rulez/skills-real")
	testutil.SymlinkOrSkip(t, "skills-real", filepath.Join(w.configDir, "skills"))
	gitIn(t, w.root, "add", ".ai-rulez/skills")
	gitIn(t, w.root, "commit", "-q", "-m", "link the skills directory")

	// Act
	_, err := PR(context.Background(), &w.opts)

	// Assert
	var refusal *Refusal
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, CodePRRefused, refusal.Code)
	assert.Contains(t, refusal.Error(), "symlink")
	assert.Empty(t, gitIn(t, w.root, "branch", "--list", "ai-rulez/*"))
}

// generatingSelf is a stand-in for ai-rulez: generate rewrites CLAUDE.md from the skill and the manifest lists
// it; lock records the digest of what generate wrote, so the order of the two is observable.
func generatingSelf(t *testing.T, failGenerate bool) string {
	t.Helper()
	self := filepath.Join(t.TempDir(), "fake-ai-rulez")
	gen := `skill=$(tr -d '\n' < .ai-rulez/skills/deploy/SKILL.md | cksum | cut -d' ' -f1); echo "generated $skill" > CLAUDE.md; printf '{"version":"1","files":["CLAUDE.md"]}' > .ai-rulez/.generated-manifest.json`
	if failGenerate {
		gen = `echo 'generate exploded' >&2; exit 2`
	}
	script := "#!/bin/sh\ncase \"$1\" in\n  generate) " + gen + " ;;\n  lock) cp CLAUDE.md .ai-rulez/ai-rulez.lock 2>/dev/null || echo 'lock before generate' > .ai-rulez/ai-rulez.lock ;;\nesac\n"
	require.NoError(t, os.WriteFile(self, []byte(script), 0o755)) //nolint:gosec // a test script
	return self
}

func TestPR_RegeneratesCommittedOutputsBeforeTheLock(t *testing.T) {
	// Arrange: the project commits CLAUDE.md, its manifest and a lock that pins it.
	w := newPRWorld(t)
	require.NoError(t, os.WriteFile(filepath.Join(w.root, "CLAUDE.md"), []byte("generated before\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(w.configDir, ".generated-manifest.json"), []byte(`{"version":"1","files":["CLAUDE.md"]}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(w.configDir, "ai-rulez.lock"), []byte("old lock\n"), 0o600))
	gitIn(t, w.root, "add", "CLAUDE.md", ".ai-rulez/.generated-manifest.json", ".ai-rulez/ai-rulez.lock")
	gitIn(t, w.root, "commit", "-q", "-m", "commit the outputs")
	gitIn(t, w.root, "push", "-q", "origin", "main")
	w.opts.Self = []string{generatingSelf(t, false)}

	// Act
	res, err := PR(context.Background(), &w.opts)

	// Assert: the refreshed output is in the commit, and the lock was made after it.
	require.NoError(t, err, w.out.String())
	assert.Equal(t, []string{"ai-rulez generate --yes", "ai-rulez lock"}, res.Refreshed)
	files := strings.Split(gitIn(t, w.root, "show", "--name-only", "--format=", res.Branch), "\n")
	assert.Contains(t, files, "CLAUDE.md")
	assert.Contains(t, files, ".ai-rulez/ai-rulez.lock")
	out := gitIn(t, w.root, "show", res.Branch+":CLAUDE.md")
	assert.True(t, strings.HasPrefix(out, "generated "), out)
	assert.Equal(t, out, gitIn(t, w.root, "show", res.Branch+":.ai-rulez/ai-rulez.lock"), "the lock pins what generate wrote, not the stale output")
	assert.Equal(t, "generated before", strings.TrimSpace(readFileString(t, filepath.Join(w.root, "CLAUDE.md"))), "the checkout's own output is untouched")
}

func TestPR_DoesNotCommitOutputsTheProjectIgnores(t *testing.T) {
	// Arrange: generate writes CLAUDE.md, but git tracks none of the manifest's files.
	w := newPRWorld(t)
	w.opts.Self = []string{generatingSelf(t, false)}

	// Act
	res, err := PR(context.Background(), &w.opts)

	// Assert
	require.NoError(t, err, w.out.String())
	files := strings.Split(gitIn(t, w.root, "show", "--name-only", "--format=", res.Branch), "\n")
	assert.NotContains(t, files, "CLAUDE.md")
	assert.NotContains(t, files, ".ai-rulez/.generated-manifest.json")
}

func TestPR_SkipsGitignoredOutputsNextToCommittedOnes(t *testing.T) {
	// Arrange: CLAUDE.md is committed, GEMINI.md is generated but gitignored; both are in the manifest.
	w := newPRWorld(t)
	require.NoError(t, os.WriteFile(filepath.Join(w.root, ".gitignore"), []byte("GEMINI.md\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(w.root, "CLAUDE.md"), []byte("generated before\n"), 0o600))
	manifest := `{"version":"1","files":["CLAUDE.md","GEMINI.md"]}`
	require.NoError(t, os.WriteFile(filepath.Join(w.configDir, ".generated-manifest.json"), []byte(manifest), 0o600))
	gitIn(t, w.root, "add", ".gitignore", "CLAUDE.md", ".ai-rulez/.generated-manifest.json")
	gitIn(t, w.root, "commit", "-q", "-m", "commit the outputs")
	gitIn(t, w.root, "push", "-q", "origin", "main")
	self := filepath.Join(t.TempDir(), "fake-ai-rulez")
	script := "#!/bin/sh\necho regenerated > CLAUDE.md; echo gemini > GEMINI.md; printf '" + manifest + "' > .ai-rulez/.generated-manifest.json\n"
	require.NoError(t, os.WriteFile(self, []byte(script), 0o755)) //nolint:gosec // a test script
	w.opts.Self = []string{self}

	// Act
	res, err := PR(context.Background(), &w.opts)

	// Assert
	require.NoError(t, err, w.out.String())
	files := strings.Split(gitIn(t, w.root, "show", "--name-only", "--format=", res.Branch), "\n")
	assert.Contains(t, files, "CLAUDE.md")
	assert.NotContains(t, files, "GEMINI.md")
}

func TestBranchName(t *testing.T) {
	tests := []struct{ name, skill, digest, want string }{
		{"plain", "deploy", "sha256:0123456789abcdef", "ai-rulez/improve/deploy-01234567"},
		{"dot runs would be an invalid ref", "a..b", "sha256:0123456789abcdef", "ai-rulez/improve/a-b-01234567"},
		{"a .lock suffix is refused by git", "x.lock", "sha256:0123456789abcdef", "ai-rulez/improve/x-01234567"},
		{"nothing usable", "...", "sha256:0123456789abcdef", "ai-rulez/improve/skill-01234567"},
		{"a short digest does not panic", "deploy", "sha256:abc", "ai-rulez/improve/deploy-abc"},
		{"an empty digest does not panic", "deploy", "", "ai-rulez/improve/deploy-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			got := branchName(tt.skill, tt.digest)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPR_AFailingGenerate(t *testing.T) {
	t.Run("stops the pull request when a lock pins the outputs", func(t *testing.T) {
		// Arrange
		w := newPRWorld(t)
		require.NoError(t, os.WriteFile(filepath.Join(w.configDir, "ai-rulez.lock"), []byte("old lock\n"), 0o600))
		gitIn(t, w.root, "add", ".ai-rulez/ai-rulez.lock")
		gitIn(t, w.root, "commit", "-q", "-m", "lock")
		gitIn(t, w.root, "push", "-q", "origin", "main")
		w.opts.Self = []string{generatingSelf(t, true)}

		// Act
		_, err := PR(context.Background(), &w.opts)

		// Assert
		var refusal *Refusal
		require.ErrorAs(t, err, &refusal)
		assert.Contains(t, refusal.Error(), "generate")
		assert.Contains(t, refusal.Error(), "generate exploded")
		assert.Empty(t, gitIn(t, w.root, "branch", "--list", "ai-rulez/*"))
	})
	t.Run("only warns without a lock", func(t *testing.T) {
		// Arrange
		w := newPRWorld(t)
		w.opts.Self = []string{generatingSelf(t, true)}

		// Act
		res, err := PR(context.Background(), &w.opts)

		// Assert
		require.NoError(t, err, w.out.String())
		assert.Contains(t, w.out.String(), "generated outputs were not refreshed")
		assert.Empty(t, res.Refreshed)
	})
}

func TestPR_StagesTheLockOfAConfigDirectoryNotNamedLikeItsParent(t *testing.T) {
	// Arrange: the config directory is .config/ai-rulez, so its base name is not its path below the project.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root, configDir, plan, _ := acceptedRunAt(t, ".config/ai-rulez", "\nGOOD advice.\n", nil)
	gitIn(t, root, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "ai-rulez.lock"), []byte("version = 1\n"), 0o600))
	gitIn(t, root, "add", ".config")
	gitIn(t, root, "commit", "-q", "-m", "init")
	self := filepath.Join(t.TempDir(), "fake-ai-rulez")
	script := "#!/bin/sh\ncase \"$1\" in\n  lock) echo \"# lock refreshed\" >> .config/ai-rulez/ai-rulez.lock ;;\nesac\n"
	require.NoError(t, os.WriteFile(self, []byte(script), 0o755)) //nolint:gosec // a test script
	opts := PROptions{
		ConfigDir: configDir, RepoDir: root, RunID: plan.RunID, Out: io.Discard, Yes: true, NoPush: true, Self: []string{self},
		Exec: runner.Exec{}, Git: gitutil.Git{}, Env: []string{"PATH=/usr/bin:/bin"},
	}

	// Act
	res, err := PR(context.Background(), &opts)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, res.Refreshed, "ai-rulez lock", "the lock is found at the config directory's real path")
	assert.Contains(t, gitIn(t, root, "show", res.Branch+":.config/ai-rulez/ai-rulez.lock"), "# lock refreshed")
}
