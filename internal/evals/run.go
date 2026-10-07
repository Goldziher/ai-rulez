package evals

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// Skill run statuses.
const (
	RunRan           = "ran"
	RunCached        = "cached"
	RunDryRun        = "dry-run"
	RunNoCases       = "no-cases"
	RunInvalid       = "invalid"
	RunError         = "error"
	RunOverBudget    = "skipped-over-budget"
	RunNotChanged    = "skipped-unchanged"
	defaultThreshold = 1.0
)

// RunOptions configures Run.
type RunOptions struct {
	// ConfigDir is the absolute .ai-rulez directory.
	ConfigDir string
	// Skills limits the run to these ids; empty means every skill with cases.
	Skills []string
	// Changed, when non-nil, limits the run to the ids it holds.
	Changed map[string]bool

	Runner   Runner
	Harness  string
	Model    string
	Ablation bool
	DryRun   bool
	// Force ignores the result cache.
	Force bool
	// MaxCostUSD stops the run: a pre-flight estimate above it refuses to start, and
	// a skill is skipped once actual spend reaches it. Zero means no limit. It is an
	// advisory cap checked between skills, not a hard per-call limit: a runner is
	// handed the budget left (Request.MaxCostUSD) and may overshoot it, which is
	// reported as a warning. A runner that reports no cost at all is assumed to have
	// spent the whole remaining budget, so the run stops. RunnerTimeout, not this,
	// bounds the time of one skill.
	MaxCostUSD float64
	// Date is recorded in the results; the caller supplies it (no clock is read).
	Date string
	// PassThreshold is the pass rate a skill needs to count as passing. Nil means 1;
	// an explicit 0 records scores without gating on them. Must lie in 0..1.
	PassThreshold *float64
	// ToolVersion identifies the ai-rulez build; a different version re-runs.
	ToolVersion string
	Grade       GradeOptions
	Counter     tokens.Counter
	// Price overrides the model price tier for estimates when non-zero.
	Price Price
	// Params are the assumptions behind the estimate; zero fields keep the built-in values.
	Params EstimateParams
	// MaxCostMode is the estimate figure that must fit under MaxCostUSD before the
	// run starts: CostModeExpected (default) or CostModeHigh.
	MaxCostMode string
	// Grader, when set, grades every rubric from the runner's output instead of
	// trusting the runner's own score (--grader builtin). Its cost counts towards
	// MaxCostUSD.
	Grader RubricGrader
	// EstimateRuns is the agent runs per case and arm assumed by the estimate.
	EstimateRuns int
	// Store holds earlier results (for the cache) and receives new ones.
	Store *Store
	// OnSkill, when set, is called after each skill finishes, so a caller can save
	// the store incrementally and a crash or interrupt keeps completed skills.
	OnSkill func(run *SkillRun) error
}

// SkillRun is the outcome of one skill in a run.
type SkillRun struct {
	ID          string      `json:"id"`
	Status      string      `json:"status"`
	Digest      string      `json:"digest,omitempty"`
	CasesDigest string      `json:"cases_digest,omitempty"`
	LockDigest  string      `json:"lock_digest,omitempty"`
	Passing     bool        `json:"passing"`
	Score       *SkillScore `json:"score,omitempty"`
	Cases       []CaseScore `json:"cases,omitempty"`
	Estimate    *Estimate   `json:"estimate,omitempty"`
	// EstimateVsActual sets the estimate next to what the runner reported, for a run
	// that happened; absent when the runner reported no cost.
	EstimateVsActual *EstimateRecord `json:"estimate_vs_actual,omitempty"`
	CaseCount        int             `json:"case_count"`
	Error            string          `json:"error,omitempty"`
	// Warnings are non-fatal findings: a cost above the budget the runner was given, or a
	// runner that reported no cost under --max-cost.
	Warnings []string     `json:"warnings,omitempty"`
	Problems []Problem    `json:"problems,omitempty"`
	Record   *SkillRecord `json:"-"`
}

// RunReport is the outcome of a whole run.
type RunReport struct {
	Runner   string     `json:"runner"`
	Harness  string     `json:"harness"`
	Model    string     `json:"model"`
	Date     string     `json:"date,omitempty"`
	Ablation bool       `json:"ablation"`
	DryRun   bool       `json:"dry_run"`
	Skills   []SkillRun `json:"skills"`
	Estimate Estimate   `json:"estimate"`
	// CostUSD is the actual cost the runner reported, plus the built-in grader's.
	CostUSD float64 `json:"cost_usd"`
	// Grader names the built-in grader when one graded the rubrics, and GraderCostUSD
	// is what it cost.
	Grader        string  `json:"grader,omitempty"`
	GraderCostUSD float64 `json:"grader_cost_usd,omitempty"`
	// PriceKnown is false when the model has no entry in the price table, so the
	// estimate used the sonnet tier as a stand-in (--price-in and --price-out replace it).
	PriceKnown bool `json:"price_known"`
	// PricedAs names the tier the estimate was priced at when the run named no
	// model (the harness picks its own, so the sonnet tier stands in); empty otherwise.
	PricedAs string `json:"priced_as,omitempty"`
	// Failed is set when a skill failed its threshold, errored or had invalid cases.
	Failed bool `json:"failed"`
}

// engine holds the resolved settings of one Run call.
type engine struct {
	opts       *RunOptions
	counter    tokens.Counter
	threshold  float64
	price      Price
	priceKnown bool
	store      *Store
	model      string
	runnerName string
	report     *RunReport
}

// plannedSkill is a skill after planning: its provisional run record and, when
// there is something to execute, the request for the runner.
type plannedSkill struct {
	skill *Skill
	run   SkillRun
	req   *Request
}

func newEngine(opts *RunOptions) (*engine, error) {
	if opts.Runner == nil && !opts.DryRun {
		return nil, fmt.Errorf("no runner configured")
	}
	if err := checkOptions(opts); err != nil {
		return nil, err
	}
	e := &engine{opts: opts, counter: opts.Counter, threshold: defaultThreshold, price: opts.Price, store: opts.Store, model: opts.Model, runnerName: "none"}
	if e.counter == nil {
		c, err := tokens.New("")
		if err != nil {
			return nil, fmt.Errorf("token counter: %w", err)
		}
		e.counter = c
	}
	if opts.PassThreshold != nil {
		e.threshold = *opts.PassThreshold
	}
	e.priceKnown = true
	pricedAs := ""
	if e.price == (Price{}) {
		e.price, e.priceKnown = PriceFor(opts.Model)
		if strings.TrimSpace(opts.Model) == "" || strings.EqualFold(opts.Model, "default") {
			pricedAs = "sonnet"
		}
		if !e.priceKnown && opts.MaxCostUSD > 0 {
			return nil, fmt.Errorf("model %q has no built-in price, so --max-cost cannot be checked: pass --price-in and --price-out, or use a model the price table lists", opts.Model)
		}
	}
	if e.store == nil {
		e.store = NewStore()
	}
	if e.model == "" {
		e.model = "default"
	}
	if opts.Runner != nil {
		e.runnerName = opts.Runner.Name()
	}
	e.report = &RunReport{Grader: e.graderName(), Runner: e.runnerName, Harness: opts.Harness, Model: e.model, Date: opts.Date, Ablation: opts.Ablation, DryRun: opts.DryRun, PriceKnown: e.priceKnown, PricedAs: pricedAs}
	return e, nil
}

// checkOptions rejects settings that would silently disable a guard.
func checkOptions(opts *RunOptions) error {
	if t := opts.PassThreshold; t != nil && (math.IsNaN(*t) || *t < 0 || *t > 1) {
		return fmt.Errorf("pass threshold must be between 0 and 1, got %v", *t)
	}
	for name, v := range map[string]float64{"max cost": opts.MaxCostUSD, "price in": opts.Price.InPerMTok, "price out": opts.Price.OutPerMTok} {
		if err := checkMoney(name, v); err != nil {
			return err
		}
	}
	switch opts.MaxCostMode {
	case "", CostModeExpected, CostModeHigh:
	default:
		return fmt.Errorf("unknown max cost mode %q (use %s or %s)", opts.MaxCostMode, CostModeExpected, CostModeHigh)
	}
	return nil
}

// Run executes the eval cases of the selected skills and records the results in
// opts.Store (the caller saves it). With DryRun it only reports what would run.
func Run(ctx context.Context, opts *RunOptions) (*RunReport, error) {
	e, err := newEngine(opts)
	if err != nil {
		return nil, err
	}
	skills, err := FindSkills(opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	selected, err := selectSkills(skills, opts.Skills)
	if err != nil {
		return nil, err
	}

	plans, err := e.planAll(selected)
	if err != nil {
		return nil, err
	}
	return e.report, e.executeAll(ctx, plans)
}

// planAll plans every selected skill first, so the pre-flight cost check sees the
// whole run.
func (e *engine) planAll(selected []Skill) ([]plannedSkill, error) {
	plans := make([]plannedSkill, 0, len(selected))
	for i := range selected {
		p, err := e.plan(&selected[i])
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
		if p.run.Status != RunCached && p.run.Estimate != nil {
			e.report.Estimate = e.report.Estimate.Add(*p.run.Estimate)
		}
	}
	if !e.opts.DryRun && e.opts.MaxCostUSD > 0 {
		figure, label := e.report.Estimate.CostUSD, "estimated"
		if e.opts.MaxCostMode == CostModeHigh {
			figure, label = e.report.Estimate.CostHighUSD, "high-end estimated"
		}
		if figure > e.opts.MaxCostUSD {
			return nil, fmt.Errorf("%s cost $%.2f exceeds --max-cost $%.2f (%d agent runs); narrow the skills, lower runs, or raise the limit",
				label, figure, e.opts.MaxCostUSD, e.report.Estimate.AgentRuns)
		}
	}
	return plans, nil
}

// executeAll runs the plans in order. On an interrupt or a failing OnSkill hook it
// stops and the report keeps what finished.
func (e *engine) executeAll(ctx context.Context, plans []plannedSkill) error {
	for i := range plans {
		if err := ctx.Err(); err != nil {
			e.report.Failed = true
			return fmt.Errorf("interrupted before %s: %w", plans[i].skill.ID, err)
		}
		run := e.execute(ctx, &plans[i])
		if failedRun(&run) {
			e.report.Failed = true
		}
		e.report.Skills = append(e.report.Skills, run)
		if e.opts.OnSkill != nil {
			if err := e.opts.OnSkill(&run); err != nil {
				return err
			}
		}
	}
	return nil
}

func failedRun(run *SkillRun) bool {
	switch run.Status {
	case RunInvalid, RunError, RunOverBudget:
		return true
	case RunRan, RunCached:
		return !run.Passing
	}
	return false
}

// plan loads a skill's cases and decides whether it needs to run.
func (e *engine) plan(skill *Skill) (plannedSkill, error) {
	p := plannedSkill{skill: skill, run: SkillRun{ID: skill.ID}}
	if e.opts.Changed != nil && !e.opts.Changed[skill.ID] {
		p.run.Status = RunNotChanged
		return p, nil
	}
	authored, problems := LoadCases(skill)
	switch {
	case len(authored) == 0 && len(problems) == 0:
		p.run.Status = RunNoCases
		return p, nil
	case len(problems) > 0:
		p.run.Status, p.run.Problems = RunInvalid, e.relativize(problems)
		return p, nil
	}
	cases := Expand(authored)
	for i := range cases {
		// A runner that only knows the free-text rubric still gets the checklist,
		// rendered; rubric_items travels along for one that understands it.
		if len(cases[i].RubricItems) > 0 {
			cases[i].Rubric = cases[i].RubricText()
		}
	}
	p.run.CaseCount = len(cases)
	digest, err := SkillDigest(skill.Dir)
	if err != nil {
		return p, err
	}
	casesDigest, err := CasesDigest(skill)
	if err != nil {
		return p, err
	}
	p.run.Digest, p.run.CasesDigest = digest, casesDigest
	// A skill the lock cannot digest keeps the evals digest and joins by id only.
	if p.run.LockDigest, err = lockDigestOf(skill.Dir); err != nil {
		p.run.LockDigest = ""
		p.run.Warnings = append(p.run.Warnings, "the lock digest of the skill could not be computed ("+err.Error()+"); usage joins by skill id only")
	}
	p.req = &Request{
		Version: ProtocolVersion, Harness: e.opts.Harness, Model: e.opts.Model, Ablation: e.opts.Ablation,
		Skill: SkillRef{ID: skill.ID, Dir: skill.Dir, Digest: digest}, Cases: cases,
	}
	est := EstimateRunWith(e.opts.Params, p.req, skillTokens(skill, e.counter), e.opts.EstimateRuns, e.price, e.counter)
	p.run.Estimate = &est
	e.markCached(&p)
	return p, nil
}

// markCached marks a planned skill as replayable when the store holds a signed,
// gradable result for the same inputs.
func (e *engine) markCached(p *plannedSkill) {
	if old, ok := e.store.Get(p.skill.ID); ok && !e.opts.Force && cacheable(&old.Score) && old.CacheKey == e.cacheKey(&p.run) &&
		// The key is an unkeyed hash a committed file can carry, so also require the
		// recorded digests to match what is on disk now: an edited skill always re-runs.
		old.Digest == p.run.Digest && old.CasesDigest == p.run.CasesDigest {
		if old.Verified() {
			p.run.Status = RunCached
		} else {
			// A committed record anyone can forge: re-run instead of trusting it.
			p.run.Warnings = append(p.run.Warnings, "the stored result is unverified (not recorded and signed on this machine); re-running")
		}
	}
}

// lockDigestOf computes the lock's digest of a skill directory; a variable so a
// test can make it fail.
var lockDigestOf = contentlock.SkillDirDigest

// relativize makes problem paths relative to the project (the directory holding
// the config directory), so reports do not differ between checkouts.
func (e *engine) relativize(problems []Problem) []Problem {
	root := filepath.Dir(e.opts.ConfigDir)
	out := make([]Problem, len(problems))
	for i, p := range problems {
		if rel, err := filepath.Rel(root, p.File); err == nil && within(root, p.File) {
			p.File = filepath.ToSlash(rel)
		}
		out[i] = p
	}
	return out
}

func (e *engine) cacheKey(run *SkillRun) string {
	fingerprint := ""
	if f, ok := e.opts.Runner.(Fingerprinter); ok {
		fingerprint = f.Fingerprint()
	}
	return CacheKey(CacheInputs{
		Digest: run.Digest, CasesDigest: run.CasesDigest, Runner: e.runnerName, RunnerFingerprint: fingerprint,
		Harness: e.opts.Harness, Model: e.model, Ablation: e.opts.Ablation, AllowExec: e.opts.Grade.AllowExec,
		ToolVersion: e.opts.ToolVersion, Grader: e.graderName(),
	})
}

// graderName is the built-in grader's identity for the cache key; empty without one.
func (e *engine) graderName() string {
	if e.opts.Grader == nil {
		return ""
	}
	return e.opts.Grader.Name()
}

// cacheable says whether a score is a real grading result. A run in which a case
// errored (rate limit, missing credentials, no result) or nothing was scored says
// more about the environment than about the skill and is always repeated.
func cacheable(score *SkillScore) bool { return score.Errors == 0 && score.Scored > 0 }

func checkMoney(name string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return fmt.Errorf("%s must be a finite number >= 0, got %v", name, v)
	}
	return nil
}

// execute runs (or replays from the store) one planned skill.
func (e *engine) execute(ctx context.Context, p *plannedSkill) SkillRun {
	run := p.run
	switch {
	case p.req == nil:
		// not changed, no cases or invalid: nothing to execute
	case run.Status == RunCached:
		e.replayCached(p, &run)
	case e.opts.DryRun:
		run.Status = RunDryRun
	case exhausted(e.opts.MaxCostUSD, e.report.CostUSD):
		run.Status = RunOverBudget
		run.Error = fmt.Sprintf("spend $%.2f reached --max-cost $%.2f", e.report.CostUSD, e.opts.MaxCostUSD)
	default:
		return e.executeLive(ctx, p, run)
	}
	return run
}

// costReported says whether a runner gave any cost information (a cost, or token
// counts the price table can turn into one).
func costReported(resp *Response) bool {
	if resp.CostUSD > 0 {
		return true
	}
	for i := range resp.Results {
		r := &resp.Results[i]
		if r.CostUSD > 0 || r.InputTokens > 0 || r.OutputTokens > 0 {
			return true
		}
	}
	return false
}

// remaining is the budget left under limit (0: no limit), rounded to 1e-4 USD.
// Callers check exhausted first: a remainder that rounds to 0 would read as
// unlimited to a runner.
func remaining(limit, spent float64) float64 {
	if limit <= 0 {
		return 0
	}
	return round(limit - spent)
}

// exhausted reports a limited budget with nothing left to hand out: what remains
// rounds to 0 or less, and a runner's max_cost_usd of 0 means unlimited.
func exhausted(limit, spent float64) bool {
	return limit > 0 && round(limit-spent) <= 0
}

func selectSkills(all []Skill, ids []string) ([]Skill, error) {
	if len(ids) == 0 {
		return all, nil
	}
	byID := map[string]Skill{}
	for _, s := range all {
		byID[s.ID] = s
	}
	var out []Skill
	seen := map[string]bool{}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	for _, id := range sorted {
		s, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown skill %q", id)
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, s)
		}
	}
	return out, nil
}

// skillTokens counts the tokens of a skill's SKILL.md; 0 when it cannot be read.
func skillTokens(skill *Skill, counter tokens.Counter) int {
	data, err := os.ReadFile(filepath.Join(skill.Dir, "SKILL.md")) //nolint:gosec // the skill's own file
	if err != nil {
		return 0
	}
	return counter.Count(string(data))
}

// replayCached fills run from the stored result of an identical earlier run.
func (e *engine) replayCached(p *plannedSkill, run *SkillRun) {
	old, _ := e.store.Get(p.skill.ID)
	score := old.Score
	run.Score, run.Passing = &score, score.Scored > 0 && score.PassRate >= e.threshold
	old.Passing = run.Passing
	if run.LockDigest != "" {
		old.LockDigest = run.LockDigest // same skill content: backfill a record that predates lock_digest
	}
	if run.Passing {
		old.LastPass = &PassMark{Digest: old.Digest, Date: old.Date}
	}
}

// executeLive sends one planned skill to the runner, grades the answer and stores
// the result.
func (e *engine) executeLive(ctx context.Context, p *plannedSkill, run SkillRun) SkillRun {
	p.req.MaxCostUSD = remaining(e.opts.MaxCostUSD, e.report.CostUSD)
	resp, err := e.opts.Runner.Run(ctx, p.req)
	if err != nil {
		run.Status, run.Error = RunError, err.Error()
		if e.opts.MaxCostUSD > 0 {
			// A failed call may already have spent money and reported none of it:
			// assume the whole remaining budget so the run stops.
			e.report.CostUSD = round(math.Max(e.report.CostUSD, e.opts.MaxCostUSD))
		}
		return run
	}
	if e.opts.Grader != nil {
		before := e.opts.Grader.SpentUSD()
		warnings, refused := gradeRubrics(ctx, e.opts.Grader, p.req.Cases, resp)
		run.Warnings = append(run.Warnings, warnings...)
		graded := round(math.Max(0, e.opts.Grader.SpentUSD()-before))
		e.report.GraderCostUSD, e.report.CostUSD = round(e.report.GraderCostUSD+graded), round(e.report.CostUSD+graded)
		if refused != nil {
			run.Status, run.Error = RunError, "the built-in grader is not allowed to run: "+refused.Error()
			return run
		}
	}
	score, cases := Score(p.req.Cases, resp, ScoreOptions{Grade: e.opts.Grade, SkillTokens: skillTokens(p.skill, e.counter), Price: e.price}) //nolint:contextcheck // local grading is bounded by GradeOptions.CommandTimeout
	charged := score.CostUSD
	if budget := p.req.MaxCostUSD; budget > 0 {
		switch {
		case !costReported(resp):
			// Reporting nothing is not spending nothing: charge the whole budget this skill
			// was given, so the next skill is refused rather than run on an unknown spend.
			charged = max(charged, budget)
			run.Warnings = append(run.Warnings, fmt.Sprintf("the runner reported no cost; assumed the whole remaining --max-cost budget ($%.2f) was spent", budget))
		case score.CostUSD > budget:
			run.Warnings = append(run.Warnings, fmt.Sprintf("cost $%.2f exceeds the $%.2f budget this skill was given (--max-cost is checked between skills, not enforced inside a runner)", score.CostUSD, budget))
		}
	}
	e.report.CostUSD = round(e.report.CostUSD + charged)
	reportedCost := score.CostUSD
	score.CostUSD = round(math.Max(score.CostUSD, charged)) // show what the run was charged
	run.Status, run.Score, run.Cases = RunRan, &score, cases
	if p.run.Estimate != nil {
		run.EstimateVsActual = NewEstimateRecord(p.run.Estimate, reportedCost, score.RunTokens).WithUsage(p.run.Estimate, score.RunInputTokens, score.RunOutputTokens, e.opts.Params)
	}
	run.Passing = score.Scored > 0 && score.PassRate >= e.threshold
	if !cacheable(&score) {
		return run // never store an infrastructure failure: it would mask the last good result
	}
	stored := score
	stored.CostUSD = reportedCost // a cache hit must not replay an assumed spend
	e.store.Put(SkillRecord{
		ID: p.skill.ID, Digest: run.Digest, CasesDigest: run.CasesDigest, LockDigest: run.LockDigest, CacheKey: e.cacheKey(&run),
		Runner: e.runnerName, Harness: e.opts.Harness, Model: e.model, Ablation: e.opts.Ablation,
		Date: e.opts.Date, Passing: run.Passing, Score: stored, Estimate: run.EstimateVsActual,
	})
	return run
}
