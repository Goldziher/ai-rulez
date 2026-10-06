package improve

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// DefaultRemote is the remote `improve pr` pushes to.
const DefaultRemote = "origin"

// refreshTimeout bounds one ai-rulez command run in the worktree (lock, eval run).
const refreshTimeout = 30 * time.Minute

// PROptions configure PR.
type PROptions struct {
	ConfigDir string
	// RepoDir is the project root (the directory holding the config directory).
	RepoDir string
	RunID   string
	// Base is the branch or commit the worktree starts from and the pull request targets; empty
	// uses the branch checked out in RepoDir.
	Base   string
	Remote string
	Draft  bool
	// NoPush stops after the commit: no push, no pull request.
	NoPush bool
	// Yes skips the confirmation before the push; otherwise Confirm decides (nil answers no).
	Yes     bool
	Confirm func(question string) bool
	Out     io.Writer
	Git     gitutil.Git
	// Exec starts gh and ai-rulez itself; nil runs real processes.
	Exec runner.Runner
	// LookPath finds gh; nil uses PATH.
	LookPath func(string) (string, error)
	// GHEnv is the complete environment of gh (the caller scrubs it); Env is the environment of the
	// ai-rulez commands run in the worktree.
	GHEnv []string
	Env   []string
	// Self is the argv prefix that starts ai-rulez. Empty skips the lock and eval refresh and says so.
	Self []string
	// RunEvals also runs `eval run <skill> --changed-only` in the worktree so AR997 holds; it calls
	// the eval runner and spends money, so it is off by default. EvalArgs are passed to it (for
	// example --max-cost).
	RunEvals bool
	EvalArgs []string
	Counter  tokens.Counter
	// AllowScripts and AllowFrontmatter widen the diff policy re-checked at the base, as for apply.
	AllowScripts     bool
	AllowFrontmatter bool
}

// PRResult says what PR did.
type PRResult struct {
	Branch string `json:"branch"`
	Base   string `json:"base"`
	Commit string `json:"commit"`
	// Refreshed lists the commands run in the worktree to bring the lock and eval results up to date.
	Refreshed []string `json:"refreshed,omitempty"`
	Pushed    bool     `json:"pushed"`
	URL       string   `json:"url,omitempty"`
	// Commands are the commands to run by hand when the push or the pull request was not made.
	Commands []string `json:"commands,omitempty"`
	BodyFile string   `json:"body_file"`
}

var unsafeBranchChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// PR turns an accepted run into a branch and a pull request without touching the user's checkout. It
// creates a linked git worktree from the base, applies the candidate there, refreshes the lock (and,
// on request, the eval results), commits on a new branch, and, when git has the remote and gh is
// installed, pushes and opens the pull request with fixed gh arguments. ai-rulez makes no network
// call itself. The change is never approved: the pull request says so.
func PR(ctx context.Context, opts *PROptions) (res *PRResult, err error) {
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.Remote == "" {
		opts.Remote = DefaultRemote
	}
	p, err := loadPRRun(opts)
	if err != nil {
		return nil, err
	}
	g := opts.Git
	top := g.TopLevel(opts.RepoDir)
	if top == "" {
		return nil, refuse(CodePRRefused, "%s is not inside a git repository: improve pr needs one to make a worktree", opts.RepoDir)
	}
	base := opts.Base
	if base == "" {
		if base = g.CurrentBranch(ctx, opts.RepoDir); base == "" {
			return nil, refuse(CodePRRefused, "HEAD is detached: pass --base BRANCH")
		}
	}
	if _, err := g.CommitOf(ctx, top, base); err != nil {
		return nil, refuse(CodePRRefused, "the base %q is not a commit of this repository: %v", base, err)
	}
	branch := fmt.Sprintf("ai-rulez/improve/%s-%s", strings.Trim(unsafeBranchChars.ReplaceAllString(p.report.Skill, "-"), "-."), strings.TrimPrefix(p.report.CandidateDigest, "sha256:")[:8])
	if g.BranchExists(ctx, top, branch) {
		return nil, refuse(CodePRRefused, "the branch %s already exists: delete it (git branch -D %s) or open the pull request from it", branch, branch)
	}
	scratch, err := os.MkdirTemp("", "ai-rulez-improve-pr-")
	if err != nil {
		return nil, fmt.Errorf("create the worktree directory: %w", err)
	}
	wt := filepath.Join(scratch, "wt")
	if err := g.WorktreeAdd(ctx, top, wt, branch, base); err != nil {
		_ = os.RemoveAll(scratch) //nolint:errcheck // a temp directory
		return nil, refuse(CodePRRefused, "create the worktree: %v", err)
	}
	committed := false
	defer func() {
		_ = g.WorktreeRemove(ctx, top, wt) //nolint:errcheck // best effort; the directory is removed next
		_ = os.RemoveAll(scratch)          //nolint:errcheck // a temp directory
		if !committed {
			_ = g.BranchDelete(ctx, top, branch) //nolint:errcheck // leave no trace of a pull request that was not made
		}
		_ = g.WorktreePrune(ctx, top) //nolint:errcheck // drops a stale entry
	}()

	relProject, err := filepath.Rel(top, gitutil.Resolve(opts.RepoDir))
	if err != nil {
		return nil, fmt.Errorf("locate the project in the repository: %w", err)
	}
	projDir := filepath.Join(wt, relProject)
	skillDir := filepath.Join(projDir, filepath.FromSlash(p.report.SkillPath))
	if err := p.applyAt(skillDir, base); err != nil {
		return nil, err
	}
	res = &PRResult{Branch: branch, Base: base, BodyFile: filepath.Join(p.dir, "pr-body.md")}
	if err := p.refresh(ctx, opts, projDir, res); err != nil {
		return nil, err
	}
	if err := p.commit(ctx, opts, wt, relProject, res); err != nil {
		return nil, err
	}
	committed = true
	fmt.Fprintf(opts.Out, "Committed %s on %s (from %s); your checkout was not touched.\n", res.Commit[:min(12, len(res.Commit))], branch, base)
	if err := safeWriteBody(res.BodyFile, prBody(p.report, opts.RunID)); err != nil {
		return res, err
	}
	return res, p.publish(ctx, opts, top, res)
}

// prRun is a verified, accepted run.
type prRun struct {
	report *Report
	dir    string
	cand   *Tree
	opts   *PROptions
}

func loadPRRun(opts *PROptions) (*prRun, error) {
	report, dir, raw, err := loadReport(opts.ConfigDir, opts.RunID)
	if err != nil {
		return nil, err
	}
	switch {
	case !verifyReport(dir, opts.RunID, raw):
		return nil, refuse(CodePRRefused, "run %s is not signed by this machine's user key (missing, edited or foreign report): refusing to make a pull request from it", opts.RunID)
	case !report.Accepted():
		return nil, refuse(CodePRRefused, "run %s has no accepted candidate (%s): nothing to propose", opts.RunID, report.Reason)
	}
	candDir := filepath.Join(dir, "rounds", fmt.Sprint(report.AcceptedRound), "candidate", report.Skill)
	if got, err := evals.SkillDigest(candDir); err != nil || got != report.CandidateDigest {
		return nil, refuse(CodePRRefused, "the candidate files of run %s changed after the run (digest mismatch): refusing to propose them", opts.RunID)
	}
	cand, err := ReadTree(candDir)
	if err != nil {
		return nil, err
	}
	return &prRun{report: report, dir: dir, cand: cand, opts: opts}, nil
}

// applyAt writes the candidate into the skill at the base, after checking that the base holds the same
// skill the run measured and that the candidate still passes the diff policy.
func (p *prRun) applyAt(skillDir, base string) error {
	r, o := p.report, p.opts
	if info, err := os.Lstat(skillDir); err != nil || !info.IsDir() {
		return refuse(CodePRRefused, "the base %s has no skill directory at %s", base, r.SkillPath)
	}
	if now, err := evals.SkillDigest(skillDir); err != nil || now != r.OriginalDigest {
		return refuse(CodeRunStale, "%s at %s differs from what run %s measured: re-run `ai-rulez improve run %s` on that base", r.Skill, base, o.RunID, r.Skill)
	}
	orig, err := ReadTree(skillDir)
	if err != nil {
		return err
	}
	if len(orig.Odd) > 0 {
		return refuse(CodePRRefused, "%s contains symlinks, hard links or oversized files: improve pr will not write into it", r.SkillPath)
	}
	counter := o.Counter
	if counter == nil {
		if counter, err = tokens.New(""); err != nil {
			return fmt.Errorf("token counter: %w", err)
		}
	}
	constraints := DefaultConstraints(counter.Count(string(orig.Files[skillFile].Data)), o.AllowFrontmatter, o.AllowScripts)
	if vs, _ := CheckDiff(&PolicyInput{Original: orig, Candidate: p.cand, Constraints: constraints, AllowScripts: o.AllowScripts, Counter: counter}); len(vs) > 0 {
		return refuse(CodePolicyViolation, "the candidate of run %s breaks the diff policy now: %s", o.RunID, vs[0])
	}
	if _, _, err := writeTree(skillDir, orig, p.cand); err != nil {
		return err
	}
	return nil
}

// refresh brings the lock (and, on request, the eval results) up to date inside the worktree, so the
// lock check and AR997 pass on the pull request. A failure stops the pull request.
func (p *prRun) refresh(ctx context.Context, opts *PROptions, projDir string, res *PRResult) error {
	if len(opts.Self) == 0 {
		res.Refreshed = nil
		fmt.Fprintln(opts.Out, "note: ai-rulez was not found to re-run itself, so the lock and the eval results were not refreshed: run `ai-rulez lock` on the branch")
		return nil
	}
	steps := [][]string{}
	if _, err := os.Stat(filepath.Join(projDir, filepath.Base(opts.ConfigDir), lockfile.FileName)); err == nil {
		steps = append(steps, []string{"lock"})
	}
	if opts.RunEvals {
		steps = append(steps, append([]string{"eval", "run", p.report.Skill, "--changed-only"}, opts.EvalArgs...))
	} else {
		fmt.Fprintf(opts.Out, "note: eval results were not refreshed (they cost model calls): run `ai-rulez eval run %s --changed-only` on the branch, or pass --run-evals\n", p.report.Skill)
	}
	run := runner.Or(opts.Exec)
	for _, args := range steps {
		argv := append(append([]string(nil), opts.Self...), args...)
		r := run.Run(ctx, runner.Spec{Argv: argv, Dir: projDir, Env: opts.Env, Timeout: refreshTimeout})
		if r.Status != runner.StatusOK {
			return refuse(CodePRRefused, "`ai-rulez %s` failed in the worktree (%s, exit %d): %s", strings.Join(args, " "), r.Status, r.ExitCode, Sanitize(string(r.Stderr)+string(r.Stdout), 600))
		}
		res.Refreshed = append(res.Refreshed, "ai-rulez "+strings.Join(args, " "))
	}
	return nil
}

// commit stages the skill, the lock and the eval results and commits them.
func (p *prRun) commit(ctx context.Context, opts *PROptions, wt, relProject string, res *PRResult) error {
	g := opts.Git
	relConfig := filepath.Join(relProject, filepath.Base(opts.ConfigDir))
	paths := []string{filepath.ToSlash(filepath.Join(relProject, filepath.FromSlash(p.report.SkillPath)))}
	for _, name := range []string{lockfile.FileName, "eval-results.json"} {
		if _, err := os.Stat(filepath.Join(wt, relConfig, name)); err == nil {
			paths = append(paths, filepath.ToSlash(filepath.Join(relConfig, name)))
		}
	}
	if err := g.Add(ctx, wt, paths...); err != nil {
		return refuse(CodePRRefused, "stage the change: %v", err)
	}
	staged, err := g.HasStaged(ctx, wt)
	switch {
	case err != nil:
		return refuse(CodePRRefused, "check the staged change: %v", err)
	case !staged:
		return refuse(CodePRRefused, "the candidate changes nothing at %s", res.Base)
	}
	sha, err := g.Commit(ctx, wt, commitMessage(p.report, opts.RunID))
	if err != nil {
		return refuse(CodePRRefused, "commit: %v", err)
	}
	res.Commit = sha
	return nil
}

// publish pushes the branch and opens the pull request when git has the remote and gh is installed and
// the user agrees; otherwise it prints the commands.
func (p *prRun) publish(ctx context.Context, opts *PROptions, top string, res *PRResult) error {
	g := opts.Git
	title := prTitle(p.report)
	prArgs := []string{"gh", "pr", "create", "--base", strings.TrimPrefix(res.Base, opts.Remote+"/"), "--head", res.Branch, "--title", title, "--body-file", res.BodyFile}
	if opts.Draft {
		prArgs = append(prArgs, "--draft")
	}
	res.Commands = []string{"git push --set-upstream " + opts.Remote + " " + res.Branch, shellQuote(prArgs)}
	manual := func(why string) error {
		fmt.Fprintf(opts.Out, "%s Run by hand:\n  %s\n  %s\n", why, res.Commands[0], res.Commands[1])
		return nil
	}
	look := opts.LookPath
	if look == nil {
		look = runner.LookPath
	}
	ghPath, ghErr := look("gh")
	switch {
	case opts.NoPush:
		return manual("--no-push: nothing was pushed.")
	case !g.HasRemote(ctx, top, opts.Remote):
		return manual("The remote " + opts.Remote + " does not exist.")
	case ghErr != nil:
		return manual("gh (the GitHub CLI) was not found on PATH.")
	case !opts.Yes && (opts.Confirm == nil || !opts.Confirm(fmt.Sprintf("Push %s to %s and open a pull request with gh?", res.Branch, opts.Remote))):
		return manual("Not confirmed: nothing was pushed.")
	}
	if err := g.Push(ctx, top, opts.Remote, res.Branch); err != nil {
		return refuse(CodePRRefused, "push %s to %s: %v (the commit is on the local branch)", res.Branch, opts.Remote, err)
	}
	res.Pushed = true
	prArgs[0] = ghPath
	out := runner.Or(opts.Exec).Run(ctx, runner.Spec{Argv: prArgs, Dir: top, Env: opts.GHEnv, Timeout: 2 * time.Minute})
	if out.Status != runner.StatusOK {
		return refuse(CodePRRefused, "gh pr create failed (%s): %s; the branch is pushed, open the pull request by hand", out.Status, Sanitize(string(out.Stderr)+string(out.Stdout), 600))
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out.Stdout)), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "https://") {
			res.URL = Sanitize(line, 300)
		}
	}
	fmt.Fprintf(opts.Out, "Opened the pull request: %s\nIt is not approved: review the full diff.\n", orElse(res.URL, "(gh printed no URL)"))
	return nil
}

func orElse(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func shellQuote(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a == "" || strings.ContainsAny(a, " \t\n\"'$`\\!*?()<>|&;") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		parts[i] = a
	}
	return strings.Join(parts, " ")
}

func prTitle(r *Report) string {
	return "chore(skills): improve " + Sanitize(r.Skill, 80) + " with an eval-gated candidate"
}

func commitMessage(r *Report, runID string) string {
	var b strings.Builder
	b.WriteString(prTitle(r))
	fmt.Fprintf(&b, "\n\nCandidate of `ai-rulez improve run` %s, round %d, accepted by the held-out gate.", runID, r.AcceptedRound)
	if rd := acceptedRound(r); rd != nil && rd.Held != nil {
		fmt.Fprintf(&b, " Held-out pass rate %.0f%% -> %.0f%% (%+.1f points, %d win(s), %d loss(es)).", rd.Held.Base.PassRate*100, rd.Held.Cand.PassRate*100, rd.Held.Gain*100, len(rd.Held.Wins), len(rd.Held.Losses))
	}
	b.WriteString("\n\nNot approved: a human reviews the full diff.\n")
	return b.String()
}

func safeWriteBody(path, body string) error {
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write the pull request body: %w", err)
	}
	return nil
}
