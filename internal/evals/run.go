package evals

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/internal/tokens"
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
	// PassThreshold is the pass rate a skill needs to count as passing. Default 1.
	PassThreshold float64
	Grade         GradeOptions
	Counter       tokens.Counter
	// Price overrides the model price tier for estimates when non-zero.
	Price Price
	// EstimateRuns is the agent runs per case and arm assumed by the estimate.
	EstimateRuns int
	// Store holds earlier results (for the cache) and receives new ones.
	Store *Store
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
	e := &engine{opts: opts, counter: opts.Counter, threshold: opts.PassThreshold, price: opts.Price, store: opts.Store, model: opts.Model, runnerName: "none"}
	if e.counter == nil {
		c, err := tokens.New("")
		if err != nil {
			return nil, fmt.Errorf("token counter: %w", err)
		}
		e.counter = c
	}
	if e.threshold <= 0 {
		e.threshold = defaultThreshold
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

	// Plan first, so the pre-flight cost check sees the whole run.
	plans := make([]plannedSkill, 0, len(selected))
	for i := range selected {
		p, err := e.plan(&selected[i])
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
		if p.run.Status == RunCached {
			continue
		}
		if p.run.Estimate != nil {
			e.report.Estimate = e.report.Estimate.Add(*p.run.Estimate)
		}
	}
	if !opts.DryRun && opts.MaxCostUSD > 0 && e.report.Estimate.CostUSD > opts.MaxCostUSD {
		return nil, fmt.Errorf("estimated cost $%.2f exceeds --max-cost $%.2f (%d agent runs); narrow the skills, lower runs, or raise the limit",
			e.report.Estimate.CostUSD, opts.MaxCostUSD, e.report.Estimate.AgentRuns)
	}

	for i := range plans {
		run := e.execute(ctx, &plans[i])
		if failedRun(&run) {
			e.report.Failed = true
		}
		e.report.Skills = append(e.report.Skills, run)
	}
	return e.report, nil
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
		p.run.Status, p.run.Problems = RunInvalid, problems
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
	if old, ok := e.store.Get(skill.ID); ok && !e.opts.Force && old.CacheKey == e.cacheKey(&p.run) {
		p.run.Status = RunCached
	}
	return p, nil
}

func (e *engine) cacheKey(run *SkillRun) string {
	return CacheKey(run.Digest, run.CasesDigest, e.runnerName, e.opts.Harness, e.model, e.opts.Ablation)
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
			return run
		}
		score, cases := Score(p.req.Cases, resp, ScoreOptions{Grade: e.opts.Grade, SkillTokens: skillTokens(p.skill, e.counter)})
		e.report.CostUSD = round(e.report.CostUSD + score.CostUSD)
		run.Status, run.Score, run.Cases = RunRan, &score, cases
		run.Passing = score.Scored > 0 && score.PassRate >= e.threshold
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
