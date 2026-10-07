package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	rv "github.com/Goldziher/ai-rulez/v5/internal/review"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

var fixFlags struct {
	finding    string
	model      string
	judgeModel string
	out        string
	apply      bool
	allowSame  bool
	noCache    bool
	patch      string
	format     string
	k          int
	content    string
}

var reviewFixCmd = &cobra.Command{
	Use:   "fix [id|name|path...]",
	Short: "Propose a verified patch for the judge's findings; writes only when asked",
	Long: `Ask a model (the fixer) for edits that resolve the stable findings of "review --semantic" on
authored items, and print them as a patch. Nothing is written unless --apply is given.

A proposal is kept only when every check passes: the edits apply exactly (each old text occurs once);
the frontmatter still parses and only the description changed in it (the name and the tool list in
particular); the item grew by at most [review.fix] max_growth_percent (25%, at least 200 bytes); the
security scan finds nothing new and no link or credential was added; and a judge, a different model
from the fixer unless --allow-same-model, rates the targeted dimensions better and no other dimension
worse. Up to two attempts per item, then "no safe fix". Items from includes, installed skills,
generated outputs and machine-local content are never touched.

The patch header carries the item digest. --apply refuses a file that changed since (a stale patch),
a file with uncommitted changes in git, and never touches more than the item file; it then tells you to
run "ai-rulez lock". Re-running after --apply proposes nothing (empty patch, exit 0): the fix is
cached, so the judge sees the same text it already rated better.

  --patch FILE   take a patch written by an earlier run instead of asking a model: verify it applies,
                 re-run the security scan, and with --apply write it

Needs the same opt-in as --semantic. The fixer model is --model, else [review.fix] model; the judge is
--judge-model, else [llm] model. Exit status: 0 on success (including nothing to fix), 2 when a finding
had no safe fix, 1 when it could not run or a patch is stale.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		code, err := runFix(cmd, args, cmd.OutOrStdout())
		if err != nil {
			return err
		}
		if code != 0 {
			os.Exit(code)
		}
		return nil
	},
}

func init() {
	f := reviewFixCmd.Flags()
	f.StringVar(&fixFlags.finding, "finding", "", "Fix only the finding with this fingerprint (a prefix is enough)")
	f.StringVar(&fixFlags.model, "model", "", "Model that writes the fix (default [review.fix] model); must differ from the judge")
	f.StringVar(&fixFlags.judgeModel, "judge-model", "", "Model that judges and verifies (default [llm] model)")
	f.StringVar(&fixFlags.out, "out", "", "Write the patch to this file instead of standard output")
	f.BoolVar(&fixFlags.apply, "apply", false, "Write the verified edits to the item files (they must be clean in git)")
	f.BoolVar(&fixFlags.allowSame, "allow-same-model", false, "Let the fixer and the judge be the same model (self-preference risk)")
	f.BoolVar(&fixFlags.noCache, "no-cache", false, "Do not read or write the model response cache")
	f.StringVar(&fixFlags.patch, "patch", "", "Apply a patch written by an earlier run (verified; --apply writes it)")
	f.IntVar(&reviewFlags.concurrency, "concurrency", 0, "Items judged at once (default 4, at most 16)")
	f.IntVar(&fixFlags.k, "k", 0, "Votes of the judge (default the rubric's votes.max)")
	f.StringVar(&reviewFlags.rubric, "rubric", "", "Rubric id (default [review] rubric, else builtin:skill-quality)")
	f.StringVar(&fixFlags.content, "content", config.ReviewContentFull, "What the judge receives: full (default here: a fix needs the body) or descriptions")
	f.StringVar(&reviewFlags.since, "since", "", "Only items changed since this git revision")
	f.StringVar(&reviewFlags.role, "role", "", "Only the content slice of this role")
	f.StringVar(&reviewFlags.profile, "profile", "", "Only the content of this profile")
	f.Float64Var(&reviewFlags.maxCost, "max-cost", 0, "Spend cap in USD (default [review] max_cost_usd, else 0.50; 0 = unlimited)")
	f.IntVar(&reviewFlags.maxCalls, "max-calls", 0, "Call cap (default [review] max_calls, else 300)")
	addFormatFlag(f, &fixFlags.format, formatText, formatText, formatText, formatJSON)
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	f.BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
}

// fixOutput is the JSON of a fix run.
type fixOutput struct {
	Proposals []*rv.FixProposal `json:"proposals"`
	Patch     string            `json:"patch,omitempty"`
	Applied   []string          `json:"applied,omitempty"`
	Usage     rv.RunUsage       `json:"usage"`
}

func runFix(cmd *cobra.Command, args []string, out io.Writer) (int, error) {
	if fixFlags.patch != "" {
		return runFixFromPatch(cmd, out)
	}
	if reviewFlags.maxCost < 0 || reviewFlags.maxCalls < 0 || fixFlags.k < 0 {
		return 0, oops.Errorf("--max-cost, --max-calls and --k must not be negative")
	}
	reviewFlags.semantic, reviewFlags.estimate, reviewFlags.content = true, false, fixFlags.content
	rc, err := loadReview(cmd, args)
	if err != nil {
		return 0, err
	}
	cfg, rb := rc.cfg, rc.rb
	res := rv.Run(rc.in)
	js, err := resolveJudge(cmd, cfg, fixFlags.judgeModel, rv.DefaultMaxCostUSD, rv.DefaultMaxCalls)
	if err != nil {
		return 0, err
	}
	fixer := fixFlags.model
	if fixer == "" && cfg.Review != nil && cfg.Review.Fix != nil {
		fixer = cfg.Review.Fix.Model
	}
	judgeName := js.lc.FullModel()
	switch {
	case fixer == "" && !fixFlags.allowSame:
		return 0, oops.Hint("pass --model or set [review.fix] model; or --allow-same-model").Errorf("no fixer model: it must differ from the judge model %s", judgeName)
	case fixer == "":
		fixer = judgeName
	case rv.SameModel(fixer, judgeName) && !fixFlags.allowSame:
		return 0, oops.Hint("pass a different --model, or --allow-same-model").Errorf("the fixer and the judge are both %s: a model verifying its own edit is biased toward them", judgeName)
	}
	est := planWith(rc, res, js.lc, js.resolved, js.maxCost, js.maxCalls, false)
	if len(est.Refused) > 0 {
		return exitReviewRefused, oops.Errorf("the run is refused: %s", strings.Join(est.Refused, "; "))
	}
	if err := js.ready(cfg); err != nil {
		return 0, err
	}
	client, err := reviewClientFactory(js.lc, llm.Options{ConfigDir: cfg.ConfigDir, NoCache: fixFlags.noCache})
	if err != nil {
		return 0, err
	}
	defer client.Close() //nolint:errcheck // nothing to flush
	k := effectiveK(rb, fixFlags.k)
	content := rc.content()
	outcome, err := rv.RunSemantic(commandContext(cmd), rv.SemanticInput{Rubric: rb, Results: res, Options: rv.SemanticOptions{Client: client, K: k, Content: content, Workers: reviewFlags.concurrency}})
	if err != nil {
		return 0, oops.Wrapf(err, "the judged run stopped")
	}
	if outcome.Incomplete {
		return exitReviewRefused, oops.Errorf("the run is incomplete (%s): %s", outcome.StoppedBecause, strings.Join(outcome.Unjudged, ", "))
	}
	verifier := rv.NewJudge(rb, rv.SemanticOptions{Client: client, K: k, Content: content})

	byItem := fixableFindings(res, rb, fixFlags.finding)
	result := fixOutput{Usage: outcome.Usage}
	var proposals []*rv.FixProposal
	growth := cfg.Review.FixMaxGrowthPercent()
	for _, id := range sortedItemIDs(byItem) {
		ir := itemByID(res, id)
		if reason := unfixable(cfg, ir); reason != "" {
			proposals = append(proposals, &rv.FixProposal{Item: id, Path: ir.Path, Digest: ir.Digest, Reason: reason, Code: byItem[id][0].Code, Dimension: byItem[id][0].Dimension, Fingerprint: byItem[id][0].Fingerprint})
			continue
		}
		p, perr := rv.ProposeFix(commandContext(cmd), rv.FixInput{
			Rubric: rb, Item: *ir, Findings: byItem[id], Pool: poolOf(res), Fixer: client, FixerModel: fixer, Verifier: verifier,
			MaxGrowthPercent: growth, LintCheck: scanDelta(ir),
		})
		if p != nil {
			result.Usage.Calls += p.Usage.Calls
			result.Usage.Cached += p.Usage.Cached
			result.Usage.Tokens += p.Usage.Tokens
			result.Usage.CostUSD += p.Usage.CostUSD
			proposals = append(proposals, p)
		}
		if perr != nil {
			return 0, oops.Wrapf(perr, "propose a fix for %s", id)
		}
	}
	result.Proposals = proposals
	patch := rv.RenderPatch(proposals, rb, fixer, judgeName)
	result.Patch = patch

	exit := 0
	for _, p := range proposals {
		if !p.Verified {
			exit = exitReviewGate
		}
	}
	if fixFlags.apply {
		var writes []fileWrite
		for _, p := range proposals {
			if p.Verified {
				writes = append(writes, fileWrite{abs: itemByID(res, p.Item).Abs, digest: p.Digest, text: p.Patched, label: p.Path})
			}
		}
		applied, err := applyFiles(writes)
		if err != nil {
			return 0, oops.Wrapf(err, "apply the fixes")
		}
		result.Applied = applied
	}
	return exit, writeFixOutput(out, cmd, result, patch)
}

// fixableFindings groups the stable judge findings of the run by item: verdicts below pass, not
// baselined, optionally one fingerprint.
func fixableFindings(res *rv.Results, rb *rv.Rubric, fingerprint string) map[string][]rv.Finding {
	out := map[string][]rv.Finding{}
	for _, f := range res.Findings(rb) {
		if f.Origin != rv.OriginLLMJudge || f.Status != rv.SemJudged || f.Baselined || f.ItemID == "" {
			continue
		}
		if fingerprint != "" && !strings.HasPrefix(f.Fingerprint, fingerprint) {
			continue
		}
		out[f.ItemID] = append(out[f.ItemID], f)
	}
	return out
}

func sortedItemIDs(m map[string][]rv.Finding) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func itemByID(res *rv.Results, id string) *rv.ItemResult {
	for i := range res.Items {
		if res.Items[i].ID == id {
			return &res.Items[i]
		}
	}
	return nil
}

// poolOf lists the sendable sibling views; the results keep them unexported, so the siblings
// come from the scored items themselves.
func poolOf(res *rv.Results) []rv.Item { return res.Pool() }

// unfixable says why an item must not be edited: only authored content inside the project's
// configuration directory, outside the machine-local overlay, qualifies.
func unfixable(cfg *config.Config, ir *rv.ItemResult) string {
	switch {
	case !ir.Owned:
		return "not authored here (include, installed skill or builtin): fix it at its source"
	case ir.Redacted:
		return "the item holds a credential that was redacted before sending: remove the credential first"
	case ir.Abs == "":
		return "the item has no file of its own"
	}
	if !committedContent(cfg.ConfigDir, ir.Abs) {
		return "machine-local or outside the configuration directory: it is not committed content"
	}
	return ""
}

// committedContent reports whether abs lies inside the configuration directory, outside its
// machine-local overlay. Both paths are resolved through symlinks first and the overlay name is
// matched without regard to case, so neither a linked parent nor a case-folding file system
// (Local/ is local/ on macOS and Windows) moves a file out of the overlay.
func committedContent(configDir, abs string) bool {
	cfgReal, abs := resolvedPath(configDir), resolvedPath(abs)
	rel, err := filepath.Rel(cfgReal, abs)
	if err != nil || rel == ".." || strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return false
	}
	return !strings.EqualFold(strings.SplitN(filepath.ToSlash(rel), "/", 2)[0], "local")
}

// resolvedPath resolves the symlinks of the longest existing prefix of path.
func resolvedPath(path string) string {
	path = filepath.Clean(path)
	rest := ""
	for cur := path; ; cur = filepath.Dir(cur) {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest)
		}
		if filepath.Dir(cur) == cur {
			return path
		}
		rest = filepath.Join(filepath.Base(cur), rest)
	}
}

// patchTargets maps the resolved file path of every item a fix may edit to its result: authored,
// committed, not redacted, and a file of its own.
func patchTargets(rc *reviewContext) map[string]bool {
	in := rc.in
	in.Only, in.Selector = nil, nil
	targets := map[string]bool{}
	res := rv.Run(in)
	for i := range res.Items {
		if ir := &res.Items[i]; unfixable(rc.cfg, ir) == "" {
			targets[resolvedPath(ir.Abs)] = true
		}
	}
	return targets
}

// scanDelta is the security scan gate of a fix: the patched text must not have more findings of
// any security rule than the original. It returns what was added.
func scanDelta(ir *rv.ItemResult) func(string) ([]string, error) {
	return func(patched string) ([]string, error) {
		return scanAdded(ir.Path, ir.Raw, patched), nil
	}
}

func scanAdded(path, orig, patched string) []string {
	count := func(text string) map[string]int {
		m := map[string]int{}
		for _, f := range lint.ScanText(path, text) {
			m[f.Code]++
		}
		return m
	}
	before, after := count(orig), count(patched)
	var added []string
	for code, n := range after {
		if n > before[code] {
			added = append(added, fmt.Sprintf("%s x%d", code, n-before[code]))
		}
	}
	sort.Strings(added)
	return added
}

// fileWrite is one verified rewrite: the file, the digest it must still have, and its new text.
type fileWrite struct{ abs, digest, text, label string }

// applyFiles writes every rewrite or none of them. Each file must still be the one its patch was
// made for and be clean in git; all are checked before the first write, and a write that fails
// puts the files already written back. The writes keep each file's mode and replace it atomically.
func applyFiles(writes []fileWrite) ([]string, error) {
	type original struct {
		data []byte
		mode os.FileMode
	}
	origs := make([]original, len(writes))
	for i, w := range writes {
		data, mode, err := safefs.ReadRegularKeepMode(w.abs)
		if err != nil {
			return nil, oops.Wrapf(err, "read %s", w.label)
		}
		if got := rv.TextDigest(string(data)); got != w.digest {
			return nil, oops.Hint("run `ai-rulez review fix` again").Errorf("stale patch: %s changed since the patch was made (digest %s, patch for %s)", w.label, got, w.digest)
		}
		if err := requireCleanInGit(w.abs); err != nil {
			return nil, err
		}
		origs[i] = original{data, mode}
	}
	var applied []string
	for i, w := range writes {
		if err := safefs.WriteFileAtomicMode(w.abs, []byte(w.text), origs[i].mode); err != nil {
			for j := range i {
				_ = safefs.WriteFileAtomicMode(writes[j].abs, origs[j].data, origs[j].mode) //nolint:errcheck // best-effort rollback; the write error is the one to report
			}
			return nil, oops.Wrapf(err, "write %s", w.label)
		}
		applied = append(applied, w.label)
	}
	return applied, nil
}

// requireCleanInGit refuses a file with uncommitted changes (or one outside a git repository,
// where a bad edit could not be undone).
func requireCleanInGit(abs string) error {
	abs = resolvedPath(abs) // the top level is reported with symlinks resolved (/private/var, not /var)
	dir := filepath.Dir(abs)
	top := gitutil.TopLevel(dir)
	if top == "" {
		return oops.Hint("--apply needs the file to be tracked and clean in git so you can review and undo the change").Errorf("%s is not inside a git repository", abs)
	}
	changed, err := gitutil.ChangedSince(dir, "HEAD")
	if err != nil {
		return oops.Wrapf(err, "check %s against git", abs)
	}
	rel, err := filepath.Rel(resolvedPath(top), abs)
	if err != nil {
		return oops.Wrapf(err, "locate %s in the repository", abs)
	}
	rel = filepath.ToSlash(rel)
	for _, c := range changed {
		if c == rel {
			return oops.Hint("commit or stash your change to the file first").Errorf("%s has uncommitted changes", rel)
		}
	}
	return nil
}

func writeFixOutput(out io.Writer, cmd *cobra.Command, res fixOutput, patch string) error {
	if fixFlags.format == formatJSON {
		return writeIndentedJSON(out, res)
	}
	summary := cmd.ErrOrStderr()
	if fixFlags.out != "" {
		if err := os.WriteFile(fixFlags.out, []byte(patch), 0o644); err != nil { //nolint:gosec // a patch is meant to be shared
			return oops.Wrapf(err, "write %s", fixFlags.out)
		}
		summary = out
	} else if patch != "" {
		if _, err := io.WriteString(out, patch); err != nil {
			return oops.Wrapf(err, "write the patch")
		}
	}
	w := reportWriter{summary}
	if len(res.Proposals) == 0 {
		w.printf("nothing to fix: no stable judged finding on an authored item\n")
	}
	for _, p := range res.Proposals {
		if p.Verified {
			w.printf("%s %s %s: verified fix (%s) %v -> %v, %d attempt(s)\n", p.Code, p.Item, p.Dimension, p.Note, p.Before, p.After, p.Attempts)
		} else {
			w.printf("%s %s %s: no safe fix: %s\n", p.Code, p.Item, p.Dimension, p.Reason)
		}
	}
	if fixFlags.out != "" && patch != "" {
		w.printf("patch written to %s\n", fixFlags.out)
	}
	for _, a := range res.Applied {
		w.printf("applied: %s\n", a)
	}
	if len(res.Applied) > 0 {
		w.printf("run `ai-rulez lock` (the lock pins the changed item), then `ai-rulez generate`\n")
	}
	w.printf("calls %d, cached %d, tokens %d, cost $%.4f\n", res.Usage.Calls, res.Usage.Cached, res.Usage.Tokens, res.Usage.CostUSD)
	return nil
}

// runFixFromPatch verifies (and with --apply writes) a patch an earlier run wrote. It calls no model.
func runFixFromPatch(cmd *cobra.Command, out io.Writer) (int, error) {
	data, _, err := safefs.ReadRegularKeepMode(fixFlags.patch)
	if err != nil {
		return 0, oops.Wrapf(err, "read the patch %s", fixFlags.patch)
	}
	files, err := rv.ParsePatch(string(data))
	if err != nil {
		return 0, err //nolint:wrapcheck // already contextual
	}
	rc, err := loadReview(cmd, nil)
	if err != nil {
		return 0, err
	}
	cfg := rc.cfg
	targets := patchTargets(rc)
	growth := cfg.Review.FixMaxGrowthPercent()
	result := fixOutput{}
	var apply []fileWrite
	for _, pf := range files {
		abs := filepath.Join(cfg.BaseDir, filepath.FromSlash(pf.Path))
		// A patch is a file anyone can hand over: it may edit only what a fix run could have edited, an
		// authored item, never the config, the lock, a script or the calibration record beside them.
		if !targets[resolvedPath(abs)] {
			return 0, oops.Errorf("the patch targets %s, which is not an authored item of this project (only authored, committed items inside %s can be fixed)", pf.Path, filepath.Base(cfg.ConfigDir))
		}
		orig, _, rerr := safefs.ReadRegularKeepMode(abs)
		if rerr != nil {
			return 0, oops.Wrapf(rerr, "read %s", pf.Path)
		}
		if got := rv.TextDigest(string(orig)); got != pf.Digest {
			return 0, oops.Hint("run `ai-rulez review fix` again").Errorf("stale patch for %s: the file changed since the patch was made", pf.Path)
		}
		patched, aerr := rv.ApplyHunks(string(orig), pf.Hunks)
		if aerr != nil {
			return 0, oops.Wrapf(aerr, "apply the patch to %s", pf.Path)
		}
		if cerr := rv.CheckPatched(string(orig), patched, growth); cerr != nil {
			return 0, oops.Wrapf(cerr, "the patch for %s fails its checks", pf.Path)
		}
		if added := scanAdded(pf.Path, string(orig), patched); len(added) > 0 {
			return 0, oops.Errorf("the patch for %s adds security findings: %s", pf.Path, strings.Join(added, "; "))
		}
		result.Proposals = append(result.Proposals, &rv.FixProposal{Item: pf.Item, Path: pf.Path, Digest: pf.Digest, Verified: true, Note: "patch applies and passes its checks"})
		apply = append(apply, fileWrite{abs: abs, digest: pf.Digest, text: patched, label: pf.Path})
	}
	if fixFlags.apply {
		applied, err := applyFiles(apply)
		if err != nil {
			return 0, oops.Wrapf(err, "apply the patch")
		}
		result.Applied = applied
	}
	return 0, writeFixOutput(out, cmd, result, "")
}
