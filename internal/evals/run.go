package evals

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

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
	// a skill is skipped once actual spend reaches it. Zero means no limit.
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
	ID          string       `json:"id"`
	Status      string       `json:"status"`
	Digest      string       `json:"digest,omitempty"`
	CasesDigest string       `json:"cases_digest,omitempty"`
	Passing     bool         `json:"passing"`
	Score       *SkillScore  `json:"score,omitempty"`
	Cases       []CaseScore  `json:"cases,omitempty"`
	Estimate    *Estimate    `json:"estimate,omitempty"`
	CaseCount   int          `json:"case_count"`
	Error       string       `json:"error,omitempty"`
	Problems    []Problem    `json:"problems,omitempty"`
	Record      *SkillRecord `json:"-"`
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
	// CostUSD is the actual cost the runner reported.
	CostUSD float64 `json:"cost_usd"`
	// Failed is set when a skill failed its threshold, errored or had invalid cases.
	Failed bool `json:"failed"`
}

// engine holds the resolved settings of one Run call.
type engine struct {
	opts       *RunOptions
	counter    tokens.Counter
	threshold  float64
	price      Price
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
	if e.price == (Price{}) {
		e.price = PriceFor(opts.Model)
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
	e.report = &RunReport{Runner: e.runnerName, Harness: opts.Harness, Model: e.model, Date: opts.Date, Ablation: opts.Ablation, DryRun: opts.DryRun}
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
	if !e.opts.DryRun && e.opts.MaxCostUSD > 0 && e.report.Estimate.CostUSD > e.opts.MaxCostUSD {
		return nil, fmt.Errorf("estimated cost $%.2f exceeds --max-cost $%.2f (%d agent runs); narrow the skills, lower runs, or raise the limit",
			e.report.Estimate.CostUSD, e.opts.MaxCostUSD, e.report.Estimate.AgentRuns)
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
	p.req = &Request{
		Version: ProtocolVersion, Harness: e.opts.Harness, Model: e.opts.Model, Ablation: e.opts.Ablation,
		Skill: SkillRef{ID: skill.ID, Dir: skill.Dir, Digest: digest}, Cases: cases,
	}
	est := EstimateRun(p.req, skillTokens(skill, e.counter), e.opts.EstimateRuns, e.price, e.counter)
	p.run.Estimate = &est
	if old, ok := e.store.Get(skill.ID); ok && !e.opts.Force && cacheable(&old.Score) && old.CacheKey == e.cacheKey(&p.run) {
		p.run.Status = RunCached
	}
	return p, nil
}

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
		ToolVersion: e.opts.ToolVersion,
	})
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
		old, _ := e.store.Get(p.skill.ID)
		score := old.Score
		run.Score, run.Passing = &score, score.Scored > 0 && score.PassRate >= e.threshold
		old.Passing = run.Passing
		if run.Passing {
			old.LastPass = &PassMark{Digest: old.Digest, Date: old.Date}
		}
	case e.opts.DryRun:
		run.Status = RunDryRun
	case e.opts.MaxCostUSD > 0 && e.report.CostUSD >= e.opts.MaxCostUSD:
		run.Status = RunOverBudget
		run.Error = fmt.Sprintf("spend $%.2f reached --max-cost $%.2f", e.report.CostUSD, e.opts.MaxCostUSD)
	default:
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
		score, cases := Score(p.req.Cases, resp, ScoreOptions{Grade: e.opts.Grade, SkillTokens: skillTokens(p.skill, e.counter), Price: e.price}) //nolint:contextcheck // local grading is bounded by GradeOptions.CommandTimeout
		e.report.CostUSD = round(e.report.CostUSD + score.CostUSD)
		run.Status, run.Score, run.Cases = RunRan, &score, cases
		run.Passing = score.Scored > 0 && score.PassRate >= e.threshold
		if !cacheable(&score) {
			return run // never store an infrastructure failure: it would mask the last good result
		}
		e.store.Put(SkillRecord{
			ID: p.skill.ID, Digest: run.Digest, CasesDigest: run.CasesDigest, CacheKey: e.cacheKey(&run),
			Runner: e.runnerName, Harness: e.opts.Harness, Model: e.model, Ablation: e.opts.Ablation,
			Date: e.opts.Date, Passing: run.Passing, Score: score,
		})
	}
	return run
}

func remaining(limit, spent float64) float64 {
	if limit <= 0 {
		return 0
	}
	return round(limit - spent)
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
