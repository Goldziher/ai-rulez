package improve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

var (
	unsafeBranchChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	dotRuns           = regexp.MustCompile(`\.{2,}`)
)

// branchName is the branch of a run: the skill id made safe for a ref name (no "..", no leading or trailing
// "-" or ".", never ending in ".lock") and the first eight hex digits of the candidate digest.
func branchName(skill, digest string) string {
	id := dotRuns.ReplaceAllString(unsafeBranchChars.ReplaceAllString(skill, "-"), "-")
	id = strings.TrimSuffix(strings.Trim(id, "-."), ".lock")
	if id == "" {
		id = "skill"
	}
	hex := strings.TrimPrefix(digest, "sha256:")
	return fmt.Sprintf("ai-rulez/improve/%s-%s", id, hex[:min(8, len(hex))])
}

// PR turns an accepted run into a branch and a pull request without touching the user's checkout. It
// creates a linked git worktree from the base, applies the candidate there, refreshes the lock (and,
// on request, the eval results), commits on a new branch, and, when git has the remote and gh is
// installed, pushes and opens the pull request with fixed gh arguments. improve pr itself makes no network
// call; the generate and lock it runs may fetch remote includes. The change is never approved: the pull request says so.
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
	// The body needs only the report. Writing it before the worktree and the commit means a failure here
	// leaves no branch behind.
	bodyFile := filepath.Join(p.dir, "pr-body.md")
	if err := safeWriteBody(bodyFile, prBody(p.report, opts.RunID)); err != nil {
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
	branch := branchName(p.report.Skill, p.report.CandidateDigest)
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

	relProject := gitutil.RepoRelative(top, opts.RepoDir)
	if relProject == "" {
		return nil, refuse(CodePRRefused, "%s is not inside the repository %s", opts.RepoDir, top)
	}
	relConfig, err := filepath.Rel(gitutil.Resolve(opts.RepoDir), gitutil.Resolve(opts.ConfigDir))
	if err != nil || !filepath.IsLocal(relConfig) {
		return nil, refuse(CodePRRefused, "the config directory %s is not inside the project %s", opts.ConfigDir, opts.RepoDir)
	}
	p.configRel = relConfig
	projDir := filepath.Join(wt, relProject)
	skillDir := filepath.Join(projDir, filepath.FromSlash(p.report.SkillPath))
	if err := p.applyAt(wt, skillDir, base); err != nil {
		return nil, err
	}
	res = &PRResult{Branch: branch, Base: base, BodyFile: bodyFile}
	if err := p.refresh(ctx, opts, projDir, relProject, res); err != nil {
		return nil, err
	}
	if err := p.commit(ctx, opts, wt, relProject, res); err != nil {
		return nil, err
	}
	committed = true
	fmt.Fprintf(opts.Out, "Committed %s on %s (from %s); your checkout was not touched.\n", res.Commit[:min(12, len(res.Commit))], branch, base)
	return res, p.publish(ctx, opts, top, res)
}

// prRun is a verified, accepted run.
type prRun struct {
	report *Report
	dir    string
	cand   *Tree
	opts   *PROptions
	// generated are the repository-relative generated outputs the project commits, refreshed by
	// `ai-rulez generate` in the worktree and staged with the skill.
	generated []string
	// configRel is the config directory relative to the project root (".ai-rulez", ".config/ai-rulez").
	configRel string
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
func (p *prRun) applyAt(wt, skillDir, base string) error {
	r, o := p.report, p.opts
	if err := plainPath(wt, skillDir); err != nil {
		return refuse(CodePRRefused, "the base %s has a symlink on the path to %s (%v): improve pr will not write through it", base, r.SkillPath, err)
	}
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
	constraints := ConstraintsFor(counter.Count(string(orig.Files[skillFile].Data)), r.Gate.MaxSkillGrowth, o.AllowFrontmatter, o.AllowScripts)
	if vs, _ := CheckDiff(&PolicyInput{Original: orig, Candidate: p.cand, Constraints: constraints, AllowScripts: o.AllowScripts, Counter: counter}); len(vs) > 0 {
		return refuse(CodePolicyViolation, "the candidate of run %s breaks the diff policy now: %s", o.RunID, vs[0])
	}
	if _, _, err := writeTree(skillDir, orig, p.cand); err != nil {
		return err
	}
	return nil
}

// plainPath checks that no component of target below root (the skill directory and every parent up to
// the worktree) is a symlink, so a committed link cannot redirect the write outside the worktree.
func plainPath(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil || !filepath.IsLocal(rel) {
		return fmt.Errorf("%s is not below the worktree", target)
	}
	cur := root
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		switch {
		case os.IsNotExist(err):
			return nil // the caller reports the missing directory
		case err != nil:
			return err //nolint:wrapcheck // named in the refusal
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("%s is a symlink", part)
		}
	}
	return nil
}

// refresh brings the generated outputs, the lock (and, on request, the eval results) up to date inside the
// worktree, so the lock check and AR997 pass on the pull request. Outputs come first: the lock pins them, so a
// lock made over stale outputs would not verify. A failure stops the pull request, except a failed generate in a
// project without a lock, which only warns.
func (p *prRun) refresh(ctx context.Context, opts *PROptions, projDir, relProject string, res *PRResult) error {
	if len(opts.Self) == 0 {
		res.Refreshed = nil
		fmt.Fprintln(opts.Out, "note: ai-rulez was not found to re-run itself, so the generated outputs, the lock and the eval results were not refreshed: run `ai-rulez generate` and `ai-rulez lock` on the branch")
		return nil
	}
	_, lockErr := os.Stat(filepath.Join(projDir, p.configRel, lockfile.FileName))
	hasLock := lockErr == nil
	steps := [][]string{{"generate", "--yes"}}
	if hasLock {
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
			why := fmt.Sprintf("`ai-rulez %s` failed in the worktree (%s, exit %d): %s", strings.Join(args, " "), r.Status, r.ExitCode, Sanitize(string(r.Stderr)+string(r.Stdout), 600))
			if args[0] == "generate" && !hasLock {
				fmt.Fprintf(opts.Out, "warning: %s; the generated outputs were not refreshed: run `ai-rulez generate` on the branch if the project commits them\n", why)
				continue
			}
			return refuse(CodePRRefused, "%s", why)
		}
		res.Refreshed = append(res.Refreshed, "ai-rulez "+strings.Join(args, " "))
		if args[0] == "generate" {
			p.generated = p.committedOutputs(ctx, opts, projDir, relProject)
		}
	}
	return nil
}

// manifestName is the file `generate` writes next to the config to list the outputs it made.
const manifestName = ".generated-manifest.json"

// maxManifestBytes bounds the manifest read from the worktree.
const maxManifestBytes = 4 << 20

// committedOutputs returns the repository-relative generated outputs to stage: the files of the generate
// manifest, when git tracks at least one of them at the base (the project commits its outputs). A project that
// gitignores them gets none, so a pull request never adds generated files the project does not commit.
func (p *prRun) committedOutputs(ctx context.Context, opts *PROptions, projDir, relProject string) []string {
	data, err := readBounded(filepath.Join(projDir, p.configRel, manifestName), maxManifestBytes)
	if err != nil {
		return nil
	}
	var m struct {
		Files []string `json:"files"`
	}
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	var files []string
	for _, f := range m.Files {
		if f != "" && filepath.IsLocal(filepath.FromSlash(f)) {
			files = append(files, filepath.ToSlash(filepath.Clean(filepath.FromSlash(f))))
		}
	}
	manifestRel := filepath.ToSlash(filepath.Join(p.configRel, manifestName))
	tracked, err := opts.Git.TrackedAmong(projDir, append(files, manifestRel))
	if err != nil {
		return nil
	}
	committed := false
	for _, f := range files {
		committed = committed || tracked[f]
	}
	if !committed {
		return nil
	}
	// git add refuses a gitignored path, so an output the project does not commit (next to ones it does) is
	// dropped; a tracked file stays even when a pattern matches it.
	ignored, err := opts.Git.IgnoredAmong(projDir, files)
	if err != nil {
		return nil
	}
	var out []string
	for _, f := range files {
		if ignored[f] && !tracked[f] {
			continue
		}
		if _, err := os.Lstat(filepath.Join(projDir, filepath.FromSlash(f))); err == nil || tracked[f] {
			out = append(out, filepath.ToSlash(filepath.Join(relProject, filepath.FromSlash(f))))
		}
	}
	if tracked[manifestRel] {
		out = append(out, filepath.ToSlash(filepath.Join(relProject, filepath.FromSlash(manifestRel))))
	}
	return out
}

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // a file of the worktree this run created
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller ignores the cause
	}
	defer f.Close() //nolint:errcheck // read only
	return io.ReadAll(io.LimitReader(f, limit))
}

// commit stages the skill, the lock and the eval results and commits them.
func (p *prRun) commit(ctx context.Context, opts *PROptions, wt, relProject string, res *PRResult) error {
	g := opts.Git
	relConfig := filepath.Join(relProject, p.configRel)
	paths := []string{filepath.ToSlash(filepath.Join(relProject, filepath.FromSlash(p.report.SkillPath)))}
	for _, name := range []string{lockfile.FileName, "eval-results.json"} {
		if _, err := os.Stat(filepath.Join(wt, relConfig, name)); err == nil {
			paths = append(paths, filepath.ToSlash(filepath.Join(relConfig, name)))
		}
	}
	paths = append(paths, p.generated...)
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
	prArgs := []string{"gh", "pr", "create"}
	// Without --repo gh picks the repository itself, and for a fork it picks the upstream, where the pushed
	// branch does not exist. Name the repository the branch was pushed to when its URL says which it is.
	if repo := remoteRepo(ctx, g, top, opts.Remote); repo != "" {
		prArgs = append(prArgs, "--repo", repo)
	}
	prArgs = append(prArgs, "--base", strings.TrimPrefix(res.Base, opts.Remote+"/"), "--head", res.Branch, "--title", title, "--body-file", res.BodyFile)
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
	}
	if why := baseProblem(ctx, g, top, opts.Remote, res.Base); why != "" {
		return manual(why + " Nothing was pushed.")
	}
	if !opts.Yes && (opts.Confirm == nil || !opts.Confirm(fmt.Sprintf("Push %s to %s and open a pull request with gh?", res.Branch, opts.Remote))) {
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

// baseProblem says why the pull request cannot target base on the remote: it is not a branch the remote
// has (gh --base needs a branch, so a commit id fails after the push), or the local base holds commits the
// remote lacks (the pull request would carry them). It reads local refs only; "" means no problem found.
func baseProblem(ctx context.Context, g gitutil.Git, top, remote, base string) string {
	branch := strings.TrimPrefix(base, remote+"/")
	remoteRef := "refs/remotes/" + remote + "/" + branch
	if _, err := g.Output(ctx, top, "rev-parse", "--verify", "--quiet", "--end-of-options", remoteRef+"^{commit}"); err != nil {
		return fmt.Sprintf("The base %s is not a branch of %s that this checkout knows (gh needs a branch to target: run `git fetch %s`, or pass --base BRANCH).", Sanitize(base, 100), remote, remote)
	}
	out, err := g.Output(ctx, top, "rev-list", "--count", "--end-of-options", remoteRef+".."+base)
	if err != nil {
		return fmt.Sprintf("Could not compare %s with %s/%s: %s.", Sanitize(base, 100), remote, Sanitize(branch, 100), Sanitize(err.Error(), 200))
	}
	if n, _ := strconv.Atoi(out); n > 0 { //nolint:errcheck // a non-number counts as none
		return fmt.Sprintf("The base %s has %d commit(s) that %s/%s does not, so the pull request would carry them: push the base first.", Sanitize(base, 100), n, remote, Sanitize(branch, 100))
	}
	return ""
}

// remoteRepo names the repository a remote points at, in the form gh --repo takes (OWNER/REPO, or
// HOST/OWNER/REPO off github.com), or "" when the URL is not a forge URL of that shape.
func remoteRepo(ctx context.Context, g gitutil.Git, top, remote string) string {
	url, err := g.Output(ctx, top, "config", "--get", "remote."+remote+".url")
	if err != nil {
		return ""
	}
	return parseRepoURL(url)
}

var (
	urlRemote  = regexp.MustCompile(`^(?:https?|ssh|git\+ssh)://(?:[^@/]+@)?([^/:@]+)(?::[0-9]+)?/(.+?)(?:\.git)?/?$`)
	scpRemote  = regexp.MustCompile(`^(?:[^@/:]+@)?([^/:@]+):([^/].*?)(?:\.git)?/?$`)
	repoSegmnt = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
)

func parseRepoURL(raw string) string {
	raw = strings.TrimSpace(raw)
	m := urlRemote.FindStringSubmatch(raw)
	if m == nil {
		m = scpRemote.FindStringSubmatch(raw)
	}
	if m == nil {
		return ""
	}
	host, path := m[1], strings.Split(m[2], "/")
	if len(path) != 2 || !repoSegmnt.MatchString(host) || !repoSegmnt.MatchString(path[0]) || !repoSegmnt.MatchString(path[1]) {
		return ""
	}
	if strings.EqualFold(host, "github.com") {
		return path[0] + "/" + path[1]
	}
	return host + "/" + path[0] + "/" + path[1]
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
