package evals

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// Defaults and names of the native activation surface.
const (
	// DefaultActivationRuns is how often each prompt is repeated: activation runs
	// are cheap, and a rate over five runs says more than a vote.
	DefaultActivationRuns = 5
	// ActivationMaxTurns bounds one activation run: the decision to load a skill is
	// in the first turn, so the run stops there.
	ActivationMaxTurns = 1
	// CostModeExpected and CostModeHigh name the estimate figure that must fit
	// under --max-cost before a run starts.
	CostModeExpected = "expected"
	CostModeHigh     = "high"
	// minBorderlineRuns is the repetitions below which "one run from failing" says
	// nothing (a single run is always one run from the other side).
	minBorderlineRuns = 3
	// firedNone is the reserved fired_counts key for runs in which no skill loaded.
	firedNone = activationNone
)

// PromptError marks a native prompt the runner gave no usable result for.
const PromptError = "error"

// nativeNote states what the native surface measures.
const nativeNote = "native measures which skill the harness's model loaded when every competing skill was installed, " +
	"over repeated runs; a prompt passes at an activation rate of at least 0.8 (positive) or at most 0.2 (negative)"

// RunActivation runs the activation cases of the selected skills on the surface
// opts names: the offline ranker (the default) or a runner's native run.
func RunActivation(ctx context.Context, opts *ActivationOptions) (*ActivationReport, error) {
	switch opts.Surface {
	case "", SurfaceRetrieval:
		return RunActivationRetrieval(ctx, opts)
	case SurfaceNative:
		return runActivationNative(ctx, opts)
	}
	return nil, fmt.Errorf("unknown surface %q (use %s or %s)", opts.Surface, SurfaceRetrieval, SurfaceNative)
}

// nativePlan is one skill ready for a runner.
type nativePlan struct {
	*activationPlan
	req      *Request
	estimate Estimate
	cacheKey string
	cached   *ActivationRecord
}

// nativeRun holds the resolved settings of one native run.
type nativeRun struct {
	opts      *ActivationOptions
	env       *activationEnv
	report    *ActivationReport
	price     Price
	counter   tokens.Counter
	runs      int
	model     string
	skillsDir map[string]Skill
}

func runActivationNative(ctx context.Context, opts *ActivationOptions) (*ActivationReport, error) {
	n, selected, err := newNativeRun(opts)
	if err != nil {
		return nil, err
	}
	if !opts.DryRun {
		// The handshake is mandatory: a runner that never declared the surface must
		// not be sent an activation request, which an old runner would run as full
		// cases.
		if err := RequireSurface(ctx, opts.Runner, CapabilityActivation, SurfaceNative); err != nil {
			return nil, err
		}
	}
	plans := make([]*nativePlan, 0, len(selected))
	for i := range selected {
		plan, err := n.plan(&selected[i])
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
		if plan.ready && plan.cached == nil {
			est := n.report.Estimate
			sum := est.Add(plan.estimate)
			n.report.Estimate = &sum
		}
	}
	if err := n.checkBudget(); err != nil {
		return nil, err
	}
	return n.report, n.execute(ctx, plans)
}

func newNativeRun(opts *ActivationOptions) (*nativeRun, []Skill, error) {
	scope, threshold, all, selected, err := activationSetup(opts)
	if err != nil {
		return nil, nil, err
	}
	if opts.Runner == nil && !opts.DryRun {
		return nil, nil, fmt.Errorf("no runner configured for the native surface")
	}
	if opts.Runs < 0 {
		return nil, nil, fmt.Errorf("runs must be >= 0, got %d", opts.Runs)
	}
	switch opts.MaxCostMode {
	case "", CostModeExpected, CostModeHigh:
	default:
		return nil, nil, fmt.Errorf("unknown max cost mode %q (use %s or %s)", opts.MaxCostMode, CostModeExpected, CostModeHigh)
	}
	for name, v := range map[string]float64{"max cost": opts.MaxCostUSD, "price in": opts.Price.InPerMTok, "price out": opts.Price.OutPerMTok} {
		if err := checkMoney(name, v); err != nil {
			return nil, nil, err
		}
	}
	n := &nativeRun{opts: opts, runs: opts.Runs, model: opts.Model, price: opts.Price, counter: opts.Counter, skillsDir: map[string]Skill{}}
	if n.runs == 0 {
		n.runs = DefaultActivationRuns
	}
	if n.model == "" {
		n.model = "default"
	}
	if n.counter == nil {
		c, err := tokens.New("")
		if err != nil {
			return nil, nil, fmt.Errorf("token counter: %w", err)
		}
		n.counter = c
	}
	if n.price == (Price{}) {
		var known bool
		n.price, known = PriceFor(opts.Model)
		if !known && opts.MaxCostUSD > 0 {
			return nil, nil, fmt.Errorf("model %q has no built-in price, so --max-cost cannot be checked: pass --price-in and --price-out, or use a model the price table lists", opts.Model)
		}
	}
	for i := range all {
		n.skillsDir[all[i].ID] = all[i]
	}
	metas, metaProblems := loadSkillMetas(all)
	n.report = &ActivationReport{
		SchemaVersion: ActivationSchemaVersion, Mode: ModeActivation, Surface: SurfaceNative, Scope: scope,
		Note: nativeNote, Date: opts.Date, Skills: []ActivationSkill{}, Confusion: map[string]map[string]int{},
		Harness: opts.Harness, Model: n.model, Runs: n.runs, DryRun: opts.DryRun, Estimate: &Estimate{},
	}
	if opts.Runner != nil {
		n.report.Runner = opts.Runner.Name()
	}
	n.env = &activationEnv{all: all, metas: metas, metaProblems: metaProblems, scope: scope, threshold: threshold, opts: opts, report: n.report}
	return n, selected, nil
}

// plan prepares a skill's request and decides whether the store already holds the
// measurement.
func (n *nativeRun) plan(skill *Skill) (*nativePlan, error) {
	plan := &nativePlan{activationPlan: n.env.prepare(skill)}
	if !plan.ready {
		return plan, nil
	}
	cases := make([]Case, len(plan.cases))
	for i := range plan.cases {
		c := &plan.cases[i]
		if len(c.Files) > 0 || len(c.Assertions) > 0 || c.HasRubric() {
			plan.run.Ignored++
		}
		// Only what an activation decision uses: the prompt and the expectation.
		cases[i] = Case{ID: c.ID, Prompt: c.Prompt, ExpectTrigger: c.ExpectTrigger, NearMissOf: c.NearMissOf, Target: skill.ID, Runs: n.runs}
	}
	req := &Request{
		Version: ProtocolVersion, Mode: ModeActivation, Surface: SurfaceNative, Harness: n.opts.Harness, Model: n.opts.Model,
		Runs: n.runs, MaxTurns: ActivationMaxTurns, Cases: cases,
		Skill: SkillRef{ID: skill.ID, Dir: skill.Dir, Digest: plan.run.Digest},
	}
	for i, id := range plan.competing {
		req.Skills = append(req.Skills, SkillRef{ID: id, Dir: n.skillsDir[id].Dir, Digest: plan.digests[id], Description: plan.docs[i].Description})
	}
	plan.req = req
	plan.estimate = EstimateActivation(n.opts.Params, req, n.runs, n.price, n.counter)
	casesDigest, err := CasesDigest(skill)
	if err != nil {
		return nil, err
	}
	plan.cacheKey = n.cacheKey(plan, casesDigest)
	if rec := n.storedRecord(skill.ID, plan); rec != nil {
		plan.cached = rec
	}
	return plan, nil
}

func (n *nativeRun) cacheKey(plan *nativePlan, casesDigest string) string {
	fingerprint := ""
	if f, ok := n.opts.Runner.(Fingerprinter); ok {
		fingerprint = f.Fingerprint()
	}
	runner := ""
	if n.opts.Runner != nil {
		runner = n.opts.Runner.Name()
	}
	return CacheKey(CacheInputs{
		Digest: plan.run.Digest, CasesDigest: fmt.Sprintf("%s|set=%s|runs=%d|scope=%s|turns=%d", casesDigest, plan.run.SetDigest, n.runs, n.report.Scope, ActivationMaxTurns),
		Runner: runner, RunnerFingerprint: fingerprint, Harness: n.opts.Harness, Model: n.model, ToolVersion: n.opts.ToolVersion,
	})
}

// storedRecord returns the verified native measurement the store holds for the
// plan's cache key, or nil.
func (n *nativeRun) storedRecord(id string, plan *nativePlan) *ActivationRecord {
	if n.opts.Store == nil || n.opts.Force {
		return nil
	}
	old, ok := n.opts.Store.Get(id)
	if !ok || old.Activation == nil || !old.Verified() {
		return nil
	}
	act := old.Activation
	if act.Surface != SurfaceNative || act.CacheKey != plan.cacheKey || act.Digest != plan.run.Digest || act.SetDigest != plan.run.SetDigest {
		return nil
	}
	return act
}

// checkBudget refuses a run whose estimate does not fit under --max-cost.
func (n *nativeRun) checkBudget() error {
	if n.opts.DryRun || n.opts.MaxCostUSD <= 0 {
		return nil
	}
	figure, label := n.report.Estimate.CostHighUSD, CostModeHigh
	if n.opts.MaxCostMode == CostModeExpected {
		figure, label = n.report.Estimate.CostUSD, CostModeExpected
	}
	if figure > n.opts.MaxCostUSD {
		return fmt.Errorf("the %s estimate $%.2f exceeds --max-cost $%.2f (%d agent runs, range $%.2f-$%.2f); narrow the skills, lower --runs, or raise the limit",
			label, figure, n.opts.MaxCostUSD, n.report.Estimate.AgentRuns, n.report.Estimate.CostLowUSD, n.report.Estimate.CostHighUSD)
	}
	return nil
}

// execute runs the plans in order, saving nothing itself but the store records.
func (n *nativeRun) execute(ctx context.Context, plans []*nativePlan) error {
	spent := 0.0
	for _, plan := range plans {
		if err := ctx.Err(); err != nil {
			n.report.Failed = true
			return fmt.Errorf("interrupted before %s: %w", plan.skill.ID, err)
		}
		run := n.executeOne(ctx, plan, &spent)
		if run.Status == RunInvalid || run.Status == RunError || run.Status == RunOverBudget ||
			((run.Status == RunRan || run.Status == RunCached) && !run.Passing) {
			n.report.Failed = true
		}
		n.report.Skills = append(n.report.Skills, run)
	}
	n.report.Cost.ActualUSD = round(spent)
	if n.report.Estimate != nil {
		n.report.Cost.EstimateUSD = n.report.Estimate.CostUSD
	}
	return nil
}

func (n *nativeRun) executeOne(ctx context.Context, plan *nativePlan, spent *float64) ActivationSkill {
	run := plan.run
	switch {
	case !plan.ready:
		return run
	case plan.cached != nil:
		return replayActivation(run, plan.cached)
	case n.opts.DryRun:
		run.Status = RunDryRun
		return run
	case n.opts.MaxCostUSD > 0 && *spent >= n.opts.MaxCostUSD:
		run.Status = RunOverBudget
		run.Error = fmt.Sprintf("spend $%.2f reached --max-cost $%.2f", *spent, n.opts.MaxCostUSD)
		return run
	}
	budget := remaining(n.opts.MaxCostUSD, *spent)
	plan.req.MaxCostUSD = budget
	callCtx := ctx
	if n.opts.Timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, n.opts.Timeout)
		defer cancel()
	}
	resp, err := n.opts.Runner.Run(callCtx, plan.req)
	if err != nil {
		run.Status, run.Error = RunError, err.Error()
		if n.opts.MaxCostUSD > 0 {
			*spent = math.Max(*spent, n.opts.MaxCostUSD) // a failed call may have spent money and reported none
		}
		return run
	}
	if err := resp.Validate(plan.req); err != nil {
		run.Status, run.Error = RunError, err.Error()
		return run
	}
	cost, in, out := responseUsage(resp, n.price)
	charged := cost
	if budget > 0 && !costReported(resp) {
		charged = math.Max(charged, budget)
		run.Warnings = append(run.Warnings, fmt.Sprintf("the runner reported no cost; assumed the whole remaining --max-cost budget ($%.2f) was spent", budget))
	} else if budget > 0 && cost > budget {
		run.Warnings = append(run.Warnings, fmt.Sprintf("cost $%.2f exceeds the $%.2f budget this skill was given (--max-cost is checked between skills, not enforced inside a runner)", cost, budget))
	}
	*spent += charged
	run.CostUSD = round(cost)
	run.EstimateVsActual = NewEstimateRecord(&plan.estimate, cost, in+out).WithUsage(&plan.estimate, in, out)
	n.scoreNative(&run, plan, resp)
	if run.Status == RunRan && run.Error == "" && n.opts.Store != nil {
		rec := run.Record(SurfaceNative, n.report.Scope, n.opts.Date)
		rec.Runner, rec.Harness, rec.Model, rec.Runs = n.report.Runner, n.opts.Harness, n.model, n.runs
		rec.Passing, rec.CacheKey, rec.Estimate = run.Passing, plan.cacheKey, run.EstimateVsActual
		n.opts.Store.PutActivation(run.ID, run.Digest, rec)
	}
	return run
}

// replayActivation turns a stored native record back into a run (figures only:
// the store keeps rates and ids, never prompts).
func replayActivation(run ActivationSkill, rec *ActivationRecord) ActivationSkill {
	run.Status, run.Passing, run.StolenBy = RunCached, rec.Passing, rec.StolenBy
	run.Recall = replayRate(rec.Recall, rec.Positives)
	run.Precision = replayRate(rec.Precision, 0)
	run.EstimateVsActual = rec.Estimate
	run.Warnings = append(run.Warnings, "replayed from the stored measurement (same skill, competing set, cases, runner and model); --force repeats it")
	return run
}

func replayRate(v *float64, n int) *Rate {
	if v == nil {
		return nil
	}
	r := &Rate{Value: *v, N: n}
	if n > 0 {
		r.Interval = Wilson(int(math.Round(*v*float64(n))), n)
	}
	return r
}

// responseUsage sums what a runner reported: the cost (never below its own total,
// or what the reported tokens price out to) and the token counts.
func responseUsage(resp *Response, price Price) (cost float64, in, out int) {
	for i := range resp.Results {
		cost += resp.Results[i].CostUSD
		in += resp.Results[i].InputTokens
		out += resp.Results[i].OutputTokens
	}
	cost = math.Max(cost, resp.CostUSD)
	if cost == 0 && price != (Price{}) {
		cost = (float64(in)*price.InPerMTok + float64(out)*price.OutPerMTok) / 1e6
	}
	return cost, in, out
}

// scoreNative turns the runner's per-prompt counts into the skill's figures.
func (n *nativeRun) scoreNative(run *ActivationSkill, plan *nativePlan, resp *Response) {
	byCase := map[string]*Result{}
	for i := range resp.Results {
		byCase[resp.Results[i].Case] = &resp.Results[i]
	}
	skill := plan.skill
	var tp, fp, fn, positives, negatives, posFires, posRuns, negFires, negRuns, errs int
	stolen := map[string]int{}
	for i := range plan.req.Cases {
		c := &plan.req.Cases[i]
		p := ActivationPrompt{Case: c.ID, Expect: c.Expects(), NearMiss: c.NearMissOf != "", Winner: activationNone}
		res, ok := byCase[c.ID]
		switch {
		case !ok:
			p.Status, p.Error = PromptError, "the runner returned no result for this prompt"
		case res.Skipped:
			p.Status, p.Error = PromptError, "the runner skipped this prompt: "+res.Reason
		case res.Error != "":
			p.Status, p.Error = PromptError, res.Error
		}
		if p.Status == PromptError {
			errs++
			run.Prompts = append(run.Prompts, p)
			continue
		}
		k := res.FiredCounts[skill.ID]
		p.Runs, p.FiredCounts = res.Runs, res.FiredCounts
		p.Rate = round(float64(k) / float64(res.Runs))
		iv := Wilson(k, res.Runs)
		p.Interval = &iv
		p.Winner = mostFired(res.FiredCounts, skill.ID)
		p.Status = nativePromptStatus(p.Expect, k, res.Runs)
		if p.Expect {
			positives++
			posFires += k
			posRuns += res.Runs
			if p.Status == PromptFailed {
				fn++
				if p.Winner != activationNone && p.Winner != skill.ID {
					stolen[p.Winner]++
				}
			} else {
				tp++
			}
			for id, count := range res.FiredCounts {
				addConfusion(n.report.Confusion, skill.ID, id, count)
			}
		} else {
			negatives++
			negFires += k
			negRuns += res.Runs
			if p.Status == PromptFailed {
				fp++
			}
		}
		run.Prompts = append(run.Prompts, p)
	}
	run.Recall, run.Precision, run.FalseActivation = newRate(tp, positives), newRate(tp, tp+fp), newRate(fp, negatives)
	run.RunRecall, run.RunFalseActivation = newRate(posFires, posRuns), newRate(negFires, negRuns)
	run.StolenBy = stolenList(stolen, positives)
	if errs > 0 {
		run.Error = fmt.Sprintf("%d of %d prompts have no usable result; the skill is not scored", errs, len(plan.req.Cases))
		run.Passing = false
		return
	}
	passed := 0
	for i := range run.Prompts {
		if s := run.Prompts[i].Status; s == PromptPassed || s == PromptBorderline {
			passed++
		}
	}
	run.Passing = len(run.Prompts) > 0 && float64(passed)/float64(len(run.Prompts)) >= n.env.threshold
}

// nativePromptStatus judges k fires in n runs against the thresholds: a positive
// prompt passes at a rate of at least activationPositiveMin, a negative one at
// most activationNegativeMax. A prompt that passes but would fail with one run
// going the other way is borderline (over at least minBorderlineRuns runs, and
// never a perfect score): 4 of 5 is a different finding from 5 of 5.
func nativePromptStatus(expect bool, k, n int) string {
	const eps = 1e-9
	rate := func(k int) float64 { return float64(k) / float64(n) }
	if expect {
		if rate(k) < activationPositiveMin-eps {
			return PromptFailed
		}
		if n >= minBorderlineRuns && k < n && rate(k-1) < activationPositiveMin-eps {
			return PromptBorderline
		}
		return PromptPassed
	}
	if rate(k) > activationNegativeMax+eps {
		return PromptFailed
	}
	if n >= minBorderlineRuns && k > 0 && rate(k+1) > activationNegativeMax+eps {
		return PromptBorderline
	}
	return PromptPassed
}

// mostFired is the skill that loaded most often; on a tie the target wins, then
// the first id in order. "none" when nothing loaded.
func mostFired(counts map[string]int, target string) string {
	best, bestCount := activationNone, 0
	ids := make([]string, 0, len(counts))
	for id := range counts {
		if id != firedNone {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if c := counts[id]; c > bestCount || (c == bestCount && c > 0 && id == target) {
			best, bestCount = id, c
		}
	}
	return best
}

// nativeEstimateLine is the dry-run summary of the estimate.
func nativeEstimateLine(est *Estimate) string {
	return fmt.Sprintf("%d agent runs, tokens in %s-%s-%s out %s-%s-%s, cost $%.2f-$%.2f-$%.2f",
		est.AgentRuns, humanTokens(est.InputTokensLow), humanTokens(est.InputTokens), humanTokens(est.InputTokensHigh),
		humanTokens(est.OutputTokensLow), humanTokens(est.OutputTokens), humanTokens(est.OutputTokensHigh),
		est.CostLowUSD, est.CostUSD, est.CostHighUSD)
}

func humanTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.2f", float64(n)/1e6), "0") + "M"
	case n >= 1000:
		return fmt.Sprintf("%dk", int(math.Round(float64(n)/1000)))
	}
	return fmt.Sprint(n)
}
