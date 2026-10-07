package improve

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// maxResponseBytes bounds an optimizer's answer.
const maxResponseBytes = 1 << 20

// Run outcomes.
const (
	StatusAccepted    = "accepted"
	StatusNoCandidate = "no-candidate"
)

// ReportSchema names the report document version.
const ReportSchema = "improve-report/1"

// Skill, request and response documents of optimizer protocol v1.
type (
	optimizerSkill struct {
		ID     string `json:"id"`
		Dir    string `json:"dir"`
		Digest string `json:"digest"`
	}
	trainScores struct {
		PassRate         float64  `json:"pass_rate"`
		TriggerPrecision *float64 `json:"trigger_precision"`
		TriggerRecall    *float64 `json:"trigger_recall"`
	}
	optimizerBudget struct {
		MaxCostUSD float64 `json:"max_cost_usd"`
		TimeoutS   int     `json:"timeout_s"`
	}
	// OptimizerRequest is the document an optimizer receives on standard input.
	// It carries train cases only.
	OptimizerRequest struct {
		Version     int             `json:"version"`
		RunID       string          `json:"run_id"`
		Round       int             `json:"round"`
		Skill       optimizerSkill  `json:"skill"`
		TrainCases  []evals.Case    `json:"train_cases"`
		TrainScores *trainScores    `json:"train_scores"`
		Constraints Constraints     `json:"constraints"`
		Budget      optimizerBudget `json:"budget"`
		// History is the one-word decision on each earlier round, never per-case results.
		History []string `json:"history,omitempty"`
		// Previous is what the optimizer needs to avoid repeating its last attempt (nil in round 1).
		Previous *PreviousRound `json:"previous,omitempty"`
	}
	// PreviousRound tells a stateless optimizer how its last round ended. Reasons are given for rounds
	// decided before the held-out set was consumed (a policy violation, a no-change round, a sibling
	// regression, a failure) and never for a round the held-out gate decided, so nothing about held-out
	// cases leaks through them.
	PreviousRound struct {
		Round    int      `json:"round"`
		Decision string   `json:"decision"`
		Reasons  []string `json:"reasons,omitempty"`
		// Summary is the optimizer's own summary of that attempt, sanitized.
		Summary string `json:"summary,omitempty"`
		// WorkspaceKept says whether the workspace still holds that attempt (a round the gate
		// rejected) or was reset to the state before it (a round rejected earlier).
		WorkspaceKept bool `json:"workspace_kept"`
	}
	// OptimizerResponse is what an optimizer prints on standard output.
	OptimizerResponse struct {
		Version int      `json:"version"`
		Summary string   `json:"summary"`
		Changed []string `json:"changed"`
		CostUSD float64  `json:"cost_usd"`
		Notes   string   `json:"notes,omitempty"`
	}
)

// RoundReport records one optimizer round.
type RoundReport struct {
	Round    int    `json:"round"`
	Decision string `json:"decision"`
	// Reasons say why a round was rejected; policy violations are in Violations.
	Reasons     []string    `json:"reasons,omitempty"`
	Violations  []Violation `json:"violations,omitempty"`
	Warnings    []string    `json:"warnings,omitempty"`
	Summary     string      `json:"summary,omitempty"`
	Notes       string      `json:"notes,omitempty"`
	Changed     []string    `json:"changed,omitempty"`
	Digest      string      `json:"candidate_digest,omitempty"`
	TrainAfter  *Metrics    `json:"train_after,omitempty"`
	Held        *Comparison `json:"held_out,omitempty"`
	CostUSD     float64     `json:"cost_usd"`
	OptCostUSD  float64     `json:"optimizer_cost_usd"`
	Description *DescChange `json:"description,omitempty"`
	// Siblings is the sibling trigger guard's measurement (AR9J4 when it regressed).
	Siblings *SiblingReport `json:"siblings,omitempty"`
	// SiblingsNative is the same guard on the harness's model, present with --sibling-native.
	SiblingsNative *SiblingReport `json:"siblings_native,omitempty"`
}

// DescChange shows the description lines the optimizer rewrote.
type DescChange struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// Costs is the money side of a run.
type Costs struct {
	EvalUSD      float64 `json:"eval_usd"`
	OptimizerUSD float64 `json:"optimizer_reported_usd"`
	TotalUSD     float64 `json:"total_usd"`
	MaxUSD       float64 `json:"max_usd"`
	// OptimizerReportedNoCost flags a run whose optimizer never reported a cost.
	OptimizerReportedNoCost bool `json:"optimizer_reported_no_cost,omitempty"`
}

// SplitReport records the split by case id only.
type SplitReport struct {
	Method   string   `json:"method"`
	Tag      string   `json:"tag,omitempty"`
	Fraction float64  `json:"fraction,omitempty"`
	Train    []string `json:"train"`
	HeldOut  []string `json:"held_out"`
}

// Report is report.json (improve-report/1).
type Report struct {
	Schema          string        `json:"schema"`
	RunID           string        `json:"run_id"`
	Skill           string        `json:"skill"`
	SkillPath       string        `json:"skill_path"`
	Date            string        `json:"date,omitempty"`
	ToolVersion     string        `json:"tool_version,omitempty"`
	Status          string        `json:"status"`
	Reason          string        `json:"reason,omitempty"`
	OriginalDigest  string        `json:"original_digest"`
	CandidateDigest string        `json:"candidate_digest,omitempty"`
	AcceptedRound   int           `json:"accepted_round,omitempty"`
	Split           SplitReport   `json:"split"`
	Harness         string        `json:"harness"`
	Model           string        `json:"model,omitempty"`
	EvalRunner      string        `json:"eval_runner"`
	Optimizer       string        `json:"optimizer"`
	Runs            int           `json:"runs"`
	Gate            GateReport    `json:"gate"`
	Constraints     Constraints   `json:"constraints"`
	Baseline        *Metrics      `json:"baseline_held_out,omitempty"`
	BaselineTrain   *Metrics      `json:"baseline_train,omitempty"`
	Rounds          []RoundReport `json:"rounds"`
	Costs           Costs         `json:"costs"`
	Egress          []string      `json:"egress,omitempty"`
	EnvPass         []string      `json:"env_pass,omitempty"`
	Warnings        []string      `json:"warnings,omitempty"`
	// Adapter names the bundled adapter the optimizer was (builtin:<name>); empty for --with.
	Adapter string `json:"adapter,omitempty"`
	// Isolation records how the optimizer was confined.
	Isolation *IsolationReport `json:"isolation,omitempty"`
}

// GateReport records the thresholds the run used.
type GateReport struct {
	MinGain         float64 `json:"min_gain"`
	MaxRegressions  int     `json:"max_regressions"`
	MaxRounds       int     `json:"max_rounds"`
	MaxHoldoutEvals int     `json:"max_holdout_evals"`
	// MaxSkillGrowth is the SKILL.md growth factor the run enforced; apply and pr re-check the
	// candidate against the same factor (0 in an older report: the default).
	MaxSkillGrowth float64 `json:"max_skill_growth,omitempty"`
	// RequireCIAboveZero records --require-ci-above-zero.
	RequireCIAboveZero bool `json:"require_ci_above_zero,omitempty"`
}

// Accepted reports whether a candidate was accepted.
func (r *Report) Accepted() bool { return r.Status == StatusAccepted }

// Sanitize makes untrusted optimizer text safe to print: control characters
// (terminal escapes included) become spaces and the text is cut at max runes.
func Sanitize(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= max {
			b.WriteString("...")
			break
		}
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
		n++
	}
	return strings.TrimSpace(b.String())
}

// execution holds the state of one Execute.
type execution struct {
	p         *Plan
	ctx       context.Context
	dir       string // run directory
	workspace string // <run>/workspace
	skillWork string // <run>/workspace/<skill>
	origDir   string
	trainFile string
	trainHash string
	report    *Report
	spent     float64
	optCost   float64
	eval      Evaluator
	prev      *Tree
}

// Execute runs the loop. A nil error means the run completed (accepted or not);
// the report says which. An error means the run could not run or stopped early
// (exit 1); when the run got as far as measuring, the report written for it is
// returned with the error (status no-candidate, or accepted when an earlier round
// had already passed the gate, with the reason "stopped: ...").
func (p *Plan) Execute(ctx context.Context) (*Report, error) {
	o := &p.Opts
	x := &execution{p: p, ctx: ctx, dir: filepath.Join(o.ConfigDir, LocalDir, p.RunID)}
	x.workspace = filepath.Join(x.dir, "workspace")
	x.skillWork = filepath.Join(x.workspace, p.Skill.ID)
	x.origDir = filepath.Join(x.dir, "original", p.Skill.ID)
	x.eval = Evaluator{Runner: o.Eval, Harness: o.Harness, Model: o.Model, Runs: o.Runs, Grade: o.Grade, Price: o.Price, Counter: o.Counter}
	x.report = &Report{
		Schema: ReportSchema, RunID: p.RunID, Skill: p.Skill.ID, SkillPath: p.SkillRel, Date: o.Date, ToolVersion: o.ToolVersion,
		OriginalDigest: p.OrigDigest, Harness: o.Harness, Model: o.Model, EvalRunner: o.Eval.Name(), Optimizer: Sanitize(o.OptimizerArgv[0], 200),
		Runs: o.Runs, Constraints: p.Constraints, Egress: o.Egress, EnvPass: o.EnvPass, Warnings: append([]string(nil), p.Warnings...),
		Gate:  GateReport{MinGain: o.MinGain, MaxRegressions: o.MaxRegressions, MaxRounds: o.MaxRounds, MaxHoldoutEvals: o.MaxHoldoutEvals, MaxSkillGrowth: EffectiveGrowth(o.MaxSkillGrowth), RequireCIAboveZero: o.RequireCIAboveZero},
		Split: SplitReport{Method: p.Split.Method, Tag: p.Split.Tag, Fraction: p.Split.Fraction, Train: IDs(p.Split.Train), HeldOut: IDs(p.Split.Held)},
		Costs: Costs{MaxUSD: o.MaxCostUSD}, Adapter: o.Adapter, Isolation: p.isolation,
	}
	p.runDir = x.dir
	if err := x.setup(); err != nil {
		return nil, err
	}
	best, err := x.loop()
	if err != nil {
		// The money was spent and the rounds so far are real: keep them in a signed report
		// instead of dropping the run, and return the report with the error.
		x.report.Reason = "stopped: " + Sanitize(err.Error(), 300)
		rep, ferr := x.finish(best)
		if ferr != nil {
			return nil, errors.Join(err, ferr)
		}
		return rep, err
	}
	return x.finish(best)
}

// setup writes the run directory: the read-only original, the optimizer's
// workspace and the train cases. Held-out content is never written here.
func (x *execution) setup() error {
	p := x.p
	for _, dir := range []string{x.dir, filepath.Join(x.dir, "home"), filepath.Join(x.dir, "tmp")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := WriteTree(x.origDir, p.orig); err != nil {
		return err
	}
	if err := WriteTree(x.skillWork, p.orig); err != nil {
		return err
	}
	train, err := json.MarshalIndent(p.trainCases, "", "  ")
	if err != nil {
		return fmt.Errorf("encode train cases: %w", err)
	}
	x.trainFile = filepath.Join(x.dir, "train", "cases.json")
	if err := safefs.WriteFileAtomic(x.trainFile, train); err != nil {
		return err //nolint:wrapcheck // safefs errors name the path
	}
	sum := sha256.Sum256(train)
	x.trainHash = hex.EncodeToString(sum[:])
	plan := map[string]any{
		"schema": "improve-plan/1", "run_id": p.RunID, "skill": p.Skill.ID, "original_digest": p.OrigDigest,
		"train": IDs(p.Split.Train), "held_out_count": len(p.heldCases), "split_method": p.Split.Method,
		"estimate": p.Estimate, "max_cost_usd": p.Opts.MaxCostUSD,
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plan: %w", err)
	}
	if err := safefs.WriteFileAtomic(filepath.Join(x.dir, "plan.json"), data); err != nil {
		return err //nolint:wrapcheck // safefs errors name the path
	}
	x.prev = p.orig
	return nil
}

// nativeSiblingGuard runs the opt-in guard on the harness's model after the free one passed and before
// any held-out spend. It charges what it spent and reports whether the round was rejected.
func (x *execution) nativeSiblingGuard(rr roundResult, rep *RoundReport) (rejected bool) {
	p := x.p
	if p.Opts.SiblingNative == nil {
		return false
	}
	nat, err := p.checkSiblingsNative(x.ctx, rr.cand, x.left())
	if nat != nil {
		x.charge(nat.CostUSD, 0)
		rep.CostUSD = roundUSD(rep.CostUSD + nat.CostUSD)
		rep.SiblingsNative = nat
	}
	switch {
	case err != nil:
		rep.Decision = "rejected: sibling guard failed"
		rep.Reasons = append(rep.Reasons, "the native sibling trigger guard could not run, so the candidate cannot be cleared: "+Sanitize(err.Error(), 300))
	case len(nat.Regressions()) > 0:
		rep.Decision = "rejected: regression"
		rep.Reasons = append(rep.Reasons, nat.Reasons()...)
	default:
		return false
	}
	x.prev = rr.cand
	return true
}

func (x *execution) left() float64 { return math.Max(0, x.p.Opts.MaxCostUSD-x.spent) }

func (x *execution) overBudget() bool { return x.left() <= 0 }

func (x *execution) charge(eval, opt float64) {
	x.spent = roundUSD(x.spent + eval + opt)
	x.report.Costs.EvalUSD = roundUSD(x.report.Costs.EvalUSD + eval)
	x.report.Costs.OptimizerUSD = roundUSD(x.report.Costs.OptimizerUSD + opt)
	x.report.Costs.TotalUSD = x.spent
}

type bestRound struct {
	round  int
	gain   float64
	tree   *Tree
	digest string
}

func (x *execution) loop() (*bestRound, error) {
	p, o := x.p, &x.p.Opts
	origDigest := p.OrigDigest
	baseHeld, err := x.eval.Eval(x.ctx, p.Skill.ID, x.origDir, origDigest, p.heldCases, x.left())
	if baseHeld != nil {
		x.charge(baseHeld.CostUSD, 0)
	}
	if err != nil {
		return nil, fmt.Errorf("measure the baseline: %w", err)
	}
	if err := x.checkBaseline(baseHeld); err != nil {
		return nil, err
	}
	baseMetrics := MetricsOf(baseHeld.Outcomes, nil)
	x.report.Baseline = &baseMetrics
	if baseMetrics.PassRate >= 1-epsilon || baseMetrics.PassRate+o.MinGain > 1+epsilon {
		x.report.Status = StatusNoCandidate
		x.report.Reason = "nothing to gain on held-out: the baseline already passes every held-out case or the required gain cannot be reached"
		return nil, nil
	}
	var scores *trainScores
	if !x.overBudget() {
		baseTrain, terr := x.eval.Eval(x.ctx, p.Skill.ID, x.origDir, origDigest, p.trainCases, x.left())
		if baseTrain != nil {
			x.charge(baseTrain.CostUSD, 0)
		}
		if terr != nil {
			return nil, fmt.Errorf("measure the baseline on train cases: %w", terr)
		}
		m := MetricsOf(baseTrain.Outcomes, nil)
		x.report.BaselineTrain = &m
		scores = &trainScores{PassRate: m.PassRate, TriggerPrecision: m.TriggerPrecision, TriggerRecall: m.TriggerRecall}
	}
	var best *bestRound
	var history []string
	holdoutEvals := 0
	for round := 1; round <= o.MaxRounds; round++ {
		if x.overBudget() {
			x.report.Reason = "stopped: over budget"
			break
		}
		if holdoutEvals >= o.MaxHoldoutEvals {
			x.report.Reason = "stopped: max held-out evaluations reached"
			break
		}
		rr := x.round(round, scores, history)
		if rr.cand != nil && rr.report.Decision == "" {
			cand, cerr := x.evaluateCandidate(round, rr, baseHeld, rr.report)
			if cerr != nil {
				x.report.Rounds = append(x.report.Rounds, *rr.report)
				return best, cerr
			}
			if cand.heldEvaluated {
				holdoutEvals++
			}
			if cand.train != nil {
				scores = cand.train
			}
			if rr.report.Decision == "accepted" && (best == nil || rr.report.Held.Gain > best.gain+epsilon) {
				best = &bestRound{round: round, gain: rr.report.Held.Gain, tree: rr.cand, digest: rr.report.Digest}
			}
		}
		x.report.Rounds = append(x.report.Rounds, *rr.report)
		history = append(history, rr.report.Decision)
		if best != nil && o.StopAtFirstAccept {
			break
		}
	}
	return best, nil
}

// checkBaseline refuses when skipped cases leave too few held-out cases.
func (x *execution) checkBaseline(m *Measurement) error {
	neg := 0
	for _, o := range m.Outcomes {
		if !o.Expect {
			neg++
		}
	}
	if need := x.p.Opts.MinHoldoutCases; len(m.Outcomes) < need || neg == 0 {
		return refuse(CodeNoHoldout, "only %d held-out case(s) (%d negative) could be scored by the runner (%d skipped): improve needs %d including one negative",
			len(m.Outcomes), neg, len(m.Skipped), need)
	}
	return nil
}

type roundResult struct {
	report *RoundReport
	cand   *Tree
	dir    string
}

type candidateEval struct {
	train *trainScores
	// heldEvaluated is set when the held-out set was consumed by this round.
	heldEvaluated bool
}

// round invokes the optimizer once and applies the diff policy. A non-empty
// report.Decision means the round is already decided (rejected) and no eval was
// spent.
func (x *execution) round(round int, scores *trainScores, history []string) roundResult {
	p, o := x.p, &x.p.Opts
	rep := &RoundReport{Round: round}
	reject := func(decision string, reasons ...string) roundResult {
		rep.Decision, rep.Reasons = decision, append(rep.Reasons, reasons...)
		if err := x.reset(x.prev); err != nil {
			rep.Reasons = append(rep.Reasons, "could not restore the workspace: "+err.Error())
		}
		return roundResult{report: rep}
	}
	snap, serr := snapshotRunDir(x.dir)
	if serr != nil {
		return reject("rejected: optimizer failed", "cannot snapshot the run directory: "+serr.Error())
	}
	resp, optReason := x.runOptimizer(round, scores, history, rep)
	if reason := x.outsideWrites(snap); reason != "" {
		rep.Violations = append(rep.Violations, violation("outside-workspace", "", "%s", reason))
		return reject("rejected: policy")
	}
	if optReason != "" {
		return reject("rejected: optimizer failed", optReason)
	}
	rep.Summary, rep.Notes, rep.Changed = Sanitize(resp.Summary, 500), Sanitize(resp.Notes, 1000), sanitizeList(resp.Changed)
	cand, err := ReadTree(x.skillWork)
	if err != nil {
		return reject("rejected: optimizer failed", "cannot read the candidate: "+err.Error())
	}
	vs, warns := CheckDiff(&PolicyInput{
		Original: p.orig, Candidate: cand, Constraints: p.Constraints, AllowScripts: o.AllowScripts,
		Counter: o.Counter, HeldAssertionValues: heldValues(p.heldCases),
	})
	rep.Warnings = append(rep.Warnings, warns...)
	if len(vs) > 0 {
		rep.Violations = append(rep.Violations, vs...)
		return reject("rejected: policy")
	}
	if treesEqual(x.prev, cand) {
		return reject("rejected: no change", "the optimizer changed nothing")
	}
	dir := filepath.Join(x.dir, "rounds", fmt.Sprint(round), "candidate", p.Skill.ID)
	if err := WriteTree(dir, cand); err != nil {
		return reject("rejected: optimizer failed", "could not snapshot the candidate: "+err.Error())
	}
	digest, err := evals.SkillDigest(dir)
	if err != nil {
		return reject("rejected: optimizer failed", "could not digest the candidate: "+err.Error())
	}
	rep.Digest = digest
	rep.Description = descChange(p.orig, cand)
	return roundResult{report: rep, cand: cand, dir: dir}
}

// evaluateCandidate runs the train and held-out evals and the gate.
func (x *execution) evaluateCandidate(round int, rr roundResult, baseHeld *Measurement, rep *RoundReport) (*candidateEval, error) {
	p, o := x.p, &x.p.Opts
	out := &candidateEval{}
	digest := rep.Digest
	// The sibling guard is free (the offline ranker), so it runs before any eval spend.
	sib, serr := p.checkSiblings(x.ctx, rr.cand)
	switch {
	case serr != nil:
		rep.Decision = "rejected: sibling guard failed"
		rep.Reasons = append(rep.Reasons, "the sibling trigger guard could not run, so the candidate cannot be cleared: "+Sanitize(serr.Error(), 300))
		x.prev = rr.cand
		return out, nil
	case len(sib.Regressions()) > 0:
		rep.Siblings = sib
		rep.Decision = "rejected: regression"
		rep.Reasons = append(rep.Reasons, sib.Reasons()...)
		x.prev = rr.cand
		return out, nil
	}
	rep.Siblings = sib
	for _, u := range sib.Unmeasured {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("the sibling guard left out %s: %s", Sanitize(u.Skill, 80), Sanitize(u.Reason, 200)))
	}
	if rejected := x.nativeSiblingGuard(rr, rep); rejected {
		return out, nil
	}
	if !x.overBudget() {
		train, err := x.eval.Eval(x.ctx, p.Skill.ID, rr.dir, digest, p.trainCases, x.left())
		if train != nil {
			x.charge(train.CostUSD, 0)
			rep.CostUSD = roundUSD(rep.CostUSD + train.CostUSD)
		}
		if err != nil {
			return nil, fmt.Errorf("round %d: evaluate the candidate on train cases: %w", round, err)
		}
		m := MetricsOf(train.Outcomes, nil)
		rep.TrainAfter = &m
		out.train = &trainScores{PassRate: m.PassRate, TriggerPrecision: m.TriggerPrecision, TriggerRecall: m.TriggerRecall}
	}
	if x.overBudget() {
		rep.Decision, rep.Reasons = "rejected: over budget", append(rep.Reasons, "the budget ran out before the held-out evaluation")
		x.report.Reason = "stopped: over budget"
		return out, x.reset(x.prev)
	}
	held, err := x.eval.Eval(x.ctx, p.Skill.ID, rr.dir, digest, p.heldCases, x.left())
	if held != nil {
		x.charge(held.CostUSD, 0)
		rep.CostUSD = roundUSD(rep.CostUSD + held.CostUSD)
	}
	if err != nil {
		return nil, fmt.Errorf("round %d: evaluate the candidate on held-out cases: %w", round, err)
	}
	out.heldEvaluated = true
	cmp := Compare(baseHeld.Outcomes, held.Outcomes)
	rep.Held = &cmp
	verdict := Gate{MinGain: o.MinGain, MaxRegressions: o.MaxRegressions, RequireCIAboveZero: o.RequireCIAboveZero}.Decide(cmp)
	rep.Decision = verdict.Decision()
	rep.Reasons = append(rep.Reasons, verdict.Reasons...)
	if cmp.Underpowered && verdict.Accept { // a rejected round needs no caveat on its evidence
		rep.Warnings = append(rep.Warnings, underpoweredWarning(&cmp))
	}
	if held.NoCost {
		rep.Warnings = append(rep.Warnings, "the eval runner reported no cost; the whole remaining budget was charged")
	}
	x.prev = rr.cand // the optimizer keeps iterating from its own state
	return out, nil
}

func heldValues(held []evals.Case) []string {
	var out []string
	for i := range held {
		for _, a := range held[i].Assertions {
			if a.Value != "" {
				out = append(out, a.Value)
			}
		}
	}
	return out
}

func sanitizeList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, Sanitize(s, 200))
	}
	return out
}

func treesEqual(a, b *Tree) bool {
	if len(a.Files) != len(b.Files) {
		return false
	}
	for p, ea := range a.Files {
		eb, ok := b.Files[p]
		if !ok || ea.Exec != eb.Exec || !bytes.Equal(ea.Data, eb.Data) {
			return false
		}
	}
	return true
}

func descChange(orig, cand *Tree) *DescChange {
	before, after := Description(orig.Files[skillFile].Data), Description(cand.Files[skillFile].Data)
	if before == after {
		return nil
	}
	return &DescChange{Before: Sanitize(before, 600), After: Sanitize(after, 600)}
}

// reset replaces the workspace skill with tree (the last good state).
func (x *execution) reset(tree *Tree) error {
	if err := os.RemoveAll(x.workspace); err != nil {
		return fmt.Errorf("clear workspace: %w", err)
	}
	return WriteTree(x.skillWork, tree)
}

// runOptimizer starts the optimizer and decodes its answer. A non-empty reason
// means the round failed.
func (x *execution) runOptimizer(round int, scores *trainScores, history []string, rep *RoundReport) (*OptimizerResponse, string) {
	p, o := x.p, &x.p.Opts
	budget := roundUSD(x.left())
	req := OptimizerRequest{
		Version: ProtocolVersion, RunID: p.RunID, Round: round,
		Skill:      optimizerSkill{ID: p.Skill.ID, Dir: p.Skill.ID, Digest: digestOfTree(x.prev)},
		TrainCases: p.trainCases, TrainScores: scores, Constraints: p.Constraints, History: history,
		Previous: x.previousRound(),
		Budget:   optimizerBudget{MaxCostUSD: budget, TimeoutS: int(runner.EffectiveTimeout(o.Timeout).Seconds())},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, "cannot encode the request: " + err.Error()
	}
	extra := []string{
		"AI_RULEZ_IMPROVE_PROTOCOL=1",
		"HOME=" + filepath.Join(x.dir, "home"), "USERPROFILE=" + filepath.Join(x.dir, "home"),
		"TMPDIR=" + filepath.Join(x.dir, "tmp"), "TMP=" + filepath.Join(x.dir, "tmp"), "TEMP=" + filepath.Join(x.dir, "tmp"),
	}
	argv, err := x.confineArgv(o.OptimizerArgv)
	if err != nil {
		return nil, "the optimizer cannot be confined, so it was not started: " + Sanitize(err.Error(), 300)
	}
	res := o.Exec.Run(x.ctx, runner.Spec{
		Argv: argv, Dir: x.workspace, Stdin: body, Timeout: o.Timeout, MaxOutput: maxResponseBytes,
		Env: runner.ScrubEnv(o.HostEnv, o.EnvPass, extra),
	})
	if o.Stderr != nil && len(res.Stderr) > 0 {
		_, _ = o.Stderr.Write([]byte(Sanitize(string(res.Stderr), 4000) + "\n")) //nolint:errcheck // diagnostics only
	}
	// An optimizer that started and then failed may have spent money: it is charged
	// what it reported, or else the whole budget it was handed.
	failed := func(reason string) (*OptimizerResponse, string) {
		x.chargeFailedRound(budget, res.Stdout, rep)
		return nil, reason
	}
	switch res.Status {
	case runner.StatusOK:
	case runner.StatusTimeout:
		return failed(fmt.Sprintf("the optimizer timed out after %s", res.Timeout))
	case runner.StatusUnavailable:
		return nil, "the optimizer is not available: " + Sanitize(fmt.Sprint(res.Err), 300)
	default:
		return failed(fmt.Sprintf("the optimizer failed (%s, exit %d)", res.Status, res.ExitCode))
	}
	if res.StdoutTruncated {
		return failed("the optimizer's answer exceeded the size limit")
	}
	resp, reason := decodeOptimizerResponse(res.Stdout)
	if reason != "" {
		return failed(reason)
	}
	x.charge(0, resp.CostUSD)
	rep.OptCostUSD = resp.CostUSD
	rep.CostUSD = roundUSD(rep.CostUSD + resp.CostUSD)
	x.optCost += resp.CostUSD
	return resp, ""
}

// decodeOptimizerResponse decodes and checks an optimizer's answer; a non-empty
// reason says why it is unusable.
func decodeOptimizerResponse(stdout []byte) (*OptimizerResponse, string) {
	var resp OptimizerResponse
	dec := json.NewDecoder(bytes.NewReader(stdout))
	if err := dec.Decode(&resp); err != nil {
		return nil, "the optimizer printed invalid JSON: " + Sanitize(err.Error(), 200)
	}
	if resp.Version != ProtocolVersion {
		return nil, fmt.Sprintf("the optimizer answered protocol version %d, want %d", resp.Version, ProtocolVersion)
	}
	if resp.CostUSD < 0 || math.IsNaN(resp.CostUSD) || math.IsInf(resp.CostUSD, 0) {
		return nil, "the optimizer reported an invalid cost_usd"
	}
	return &resp, ""
}

// chargeFailedRound charges an optimizer round that started and failed: the cost
// it reported in a well-formed answer printed before failing (the bundled
// adapters print one), else the whole budget it was handed, the rule the eval
// engine applies to a runner that reports nothing. Charging nothing would hand
// the next round the same budget again.
func (x *execution) chargeFailedRound(budget float64, stdout []byte, rep *RoundReport) {
	cost := budget
	if resp, reason := decodeOptimizerResponse(stdout); reason == "" && resp.CostUSD > 0 {
		cost = resp.CostUSD
		x.optCost += cost
	} else if budget > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("the optimizer failed without reporting a cost; the whole $%.4f budget it was given was charged", budget))
	}
	x.charge(0, cost)
	rep.OptCostUSD = cost
	rep.CostUSD = roundUSD(rep.CostUSD + cost)
}

// previousRound summarises the last finished round for the next optimizer call.
func (x *execution) previousRound() *PreviousRound {
	n := len(x.report.Rounds)
	if n == 0 {
		return nil
	}
	rd := &x.report.Rounds[n-1]
	prev := &PreviousRound{Round: rd.Round, Decision: Sanitize(rd.Decision, 80), Summary: Sanitize(rd.Summary, 300), WorkspaceKept: rd.Digest != "" && rd.Decision != "rejected: over budget"}
	if rd.Held == nil { // the held-out set was not consulted: the reasons carry nothing held-out
		for _, v := range rd.Violations {
			prev.Reasons = append(prev.Reasons, v.String())
		}
		for _, r := range rd.Reasons {
			prev.Reasons = append(prev.Reasons, Sanitize(r, 400))
		}
	}
	return prev
}

func digestOfTree(t *Tree) string {
	h := sha256.New()
	for _, p := range t.Paths() {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write(t.Files[p].Data)
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// outsideWrites reports an optimizer that wrote outside its skill directory:
// into the workspace root, anywhere else in the run directory (plan, original,
// train cases, earlier rounds) or the live skill. It undoes the damage before
// returning, so a refused round leaves everything as it was.
func (x *execution) outsideWrites(snap *runSnapshot) string {
	var reasons []string
	if entries, err := os.ReadDir(x.workspace); err != nil {
		reasons = append(reasons, "the workspace is unreadable: "+err.Error())
	} else {
		for _, e := range entries {
			if e.Name() != x.p.Skill.ID {
				reasons = append(reasons, fmt.Sprintf("created %q in the workspace root; only %s/ may exist there", e.Name(), x.p.Skill.ID))
				break
			}
		}
	}
	changed, err := snap.diff()
	if err != nil {
		reasons = append(reasons, err.Error())
	}
	if len(changed) > 0 {
		reasons = append(reasons, fmt.Sprintf("changed the run directory outside the workspace: %s", Sanitize(strings.Join(changed, ", "), 300)))
		if err := snap.restore(); err != nil {
			reasons = append(reasons, "could not restore it: "+err.Error())
		}
	}
	if d, err := evals.SkillDigest(x.p.Skill.Dir); err != nil || d != x.p.OrigDigest {
		reasons = append(reasons, "the authored skill was modified")
		if err := x.restoreAuthored(); err != nil {
			reasons = append(reasons, "could not restore it: "+err.Error())
		}
	}
	return strings.Join(reasons, "; ")
}

// restoreAuthored puts the live skill back to the tree the run measured.
func (x *execution) restoreAuthored() error {
	dir := x.p.Skill.Dir
	cur, err := ReadTree(dir)
	if err != nil {
		return err
	}
	for _, odd := range cur.Odd {
		if err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash(OddPath(odd)))); err != nil {
			return fmt.Errorf("remove %s: %w", odd, err)
		}
	}
	if len(cur.Odd) > 0 {
		if cur, err = ReadTree(dir); err != nil {
			return err
		}
	}
	_, _, err = writeTree(dir, cur, x.p.orig)
	return err
}

// finish writes report.json and, for an accepted run, diff.patch.
func (x *execution) finish(best *bestRound) (*Report, error) {
	r := x.report
	r.Costs.OptimizerReportedNoCost = x.optCost == 0
	if best != nil {
		r.Status, r.AcceptedRound, r.CandidateDigest = StatusAccepted, best.round, best.digest
		patch := UnifiedDiff(x.p.SkillRel, x.p.orig, best.tree)
		if err := safefs.WriteFileAtomic(filepath.Join(x.dir, "diff.patch"), []byte(patch)); err != nil {
			return nil, err //nolint:wrapcheck // safefs errors name the path
		}
	} else {
		r.Status = StatusNoCandidate
		if r.Reason == "" {
			r.Reason = "no round was accepted"
		}
	}
	if r.Rounds == nil {
		r.Rounds = []RoundReport{}
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode report: %w", err)
	}
	if err := safefs.WriteFileAtomic(filepath.Join(x.dir, "report.json"), append(data, '\n')); err != nil {
		return nil, err //nolint:wrapcheck // safefs errors name the path
	}
	if err := sealReport(x.dir, r.RunID, append(data, '\n')); err != nil {
		return nil, err
	}
	return r, nil
}

// RunDir is where the run's files live.
func (p *Plan) RunDir() string { return filepath.Join(p.Opts.ConfigDir, LocalDir, p.RunID) }
