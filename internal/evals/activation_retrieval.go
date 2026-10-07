package evals

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"gopkg.in/yaml.v3"
)

// ActivationOptions configures RunActivationRetrieval.
type ActivationOptions struct {
	// ConfigDir is the absolute .ai-rulez directory.
	ConfigDir string
	// Skills limits the run to these ids; empty means every skill with cases.
	Skills []string
	// Changed, when non-nil, limits the run to the ids it holds.
	Changed map[string]bool
	// Scope is ScopeDomain (default) or ScopeAll.
	Scope string
	// Date is recorded in the results; the caller supplies it.
	Date string
	// PassThreshold is the share of prompts a skill needs to pass; nil means 1.
	PassThreshold *float64
	// Store, when non-nil, receives each measured skill's activation record.
	Store *Store
	// Description, with DescriptionSkill, replaces that skill's description for
	// this run only (--description-from). Nothing is recorded in the store.
	Description      string
	DescriptionSkill string
	// dirOverride maps a skill id to the directory the run reads it from.
	dirOverride map[string]string

	// Surface is SurfaceRetrieval (default) or SurfaceNative. The settings below
	// belong to the native surface.
	Surface string
	// Runner drives the harness; it must declare the activation capability and the
	// native surface.
	Runner  Runner
	Harness string
	Model   string
	// Runs is how often each prompt is repeated (default DefaultActivationRuns).
	Runs int
	// DryRun estimates and calls no runner.
	DryRun bool
	// Force repeats a measurement the store already holds.
	Force bool
	// Timeout bounds one runner call (one skill); zero means no limit here.
	Timeout time.Duration
	// MaxCostUSD refuses a run whose estimate exceeds it (the figure MaxCostMode
	// names) and stops between skills once spend reaches it. Zero means no limit.
	MaxCostUSD float64
	// MaxCostMode is CostModeHigh (default for activation) or CostModeExpected.
	MaxCostMode string
	Price       Price
	Params      EstimateParams
	Counter     tokens.Counter
	ToolVersion string
}

// skillMeta is the searchable text of one skill read from its SKILL.md.
type skillMeta struct {
	doc    skillsearch.Doc
	digest string
}

// RunActivationRetrieval runs the activation cases of the selected skills against
// the offline find_skill ranker. It calls no model and no network, so it is free
// and deterministic.
func RunActivationRetrieval(ctx context.Context, opts *ActivationOptions) (*ActivationReport, error) {
	scope, threshold, all, selected, err := activationSetup(opts)
	if err != nil {
		return nil, err
	}
	report := &ActivationReport{
		SchemaVersion: ActivationSchemaVersion, Mode: ModeActivation, Surface: SurfaceRetrieval, Scope: scope,
		Note: activationNote, Date: opts.Date, Skills: []ActivationSkill{}, Confusion: map[string]map[string]int{},
	}
	metas, metaProblems := loadSkillMetas(all)
	env := &activationEnv{all: all, metas: metas, metaProblems: metaProblems, scope: scope, threshold: threshold, opts: opts, report: report}
	for i := range selected {
		if err := ctx.Err(); err != nil {
			report.Failed = true
			return report, fmt.Errorf("interrupted before %s: %w", selected[i].ID, err)
		}
		run := env.activate(&selected[i])
		if run.Status == RunInvalid || run.Status == RunError || (run.Status == RunRan && !run.Passing) {
			report.Failed = true
		}
		report.Skills = append(report.Skills, run)
		if opts.Store != nil && run.Status == RunRan {
			opts.Store.PutActivation(run.ID, run.Digest, run.Record(SurfaceRetrieval, scope, opts.Date))
		}
	}
	return report, nil
}

// activationSetup validates the scope and the pass threshold and lists the
// skills: every skill, and the ones selected for the run.
func activationSetup(opts *ActivationOptions) (scope string, threshold float64, all, selected []Skill, err error) {
	scope = opts.Scope
	if scope == "" {
		scope = ScopeDomain
	}
	if scope != ScopeDomain && scope != ScopeAll {
		return "", 0, nil, nil, fmt.Errorf("unknown scope %q (use %s or %s)", opts.Scope, ScopeDomain, ScopeAll)
	}
	threshold = defaultThreshold
	if opts.PassThreshold != nil {
		threshold = *opts.PassThreshold
		if threshold < 0 || threshold > 1 {
			return "", 0, nil, nil, fmt.Errorf("pass threshold must be between 0 and 1, got %v", threshold)
		}
	}
	if all, err = FindSkills(opts.ConfigDir); err != nil {
		return "", 0, nil, nil, err
	}
	for i := range all {
		if dir, ok := opts.dirOverride[all[i].ID]; ok {
			all[i].Dir = dir
		}
	}
	if selected, err = selectSkills(all, opts.Skills); err != nil {
		return "", 0, nil, nil, err
	}
	return scope, threshold, all, selected, nil
}

// activationEnv is what one retrieval run shares between its skills.
type activationEnv struct {
	all          []Skill
	metas        map[string]skillMeta
	metaProblems map[string]string
	scope        string
	threshold    float64
	opts         *ActivationOptions
	report       *ActivationReport
}

// activationPlan is one skill after preparation: its provisional run record and,
// when the skill has something to measure, the cases and the competing set.
type activationPlan struct {
	skill     *Skill
	run       ActivationSkill
	cases     []Case
	competing []string
	docs      []skillsearch.Doc
	digests   map[string]string
	// ready is false when the run record is already final (not changed, no cases,
	// invalid, unreadable).
	ready bool
}

// prepare loads the skill's cases and its competing set.
func (e *activationEnv) prepare(skill *Skill) *activationPlan {
	all, metas, metaProblems, scope, opts := e.all, e.metas, e.metaProblems, e.scope, e.opts
	plan := &activationPlan{skill: skill, run: ActivationSkill{ID: skill.ID}}
	run := &plan.run
	if opts.Changed != nil && !opts.Changed[skill.ID] {
		run.Status = RunNotChanged
		return plan
	}
	authored, problems := LoadCases(skill)
	switch {
	case len(authored) == 0 && len(problems) == 0:
		run.Status = RunNoCases
		return plan
	case len(problems) > 0:
		run.Status, run.Problems = RunInvalid, relativeProblems(opts.ConfigDir, problems)
		return plan
	}
	if why, bad := metaProblems[skill.ID]; bad {
		run.Status, run.Error = RunError, why
		return plan
	}
	plan.digests = map[string]string{}
	for i := range all {
		other := &all[i]
		if scope == ScopeDomain && other.Domain != skill.Domain && other.Domain != "" {
			continue
		}
		meta, ok := metas[other.ID]
		if !ok {
			run.Warnings = append(run.Warnings, fmt.Sprintf("skill %q was left out of the competing set: %s", other.ID, metaProblems[other.ID]))
			continue
		}
		plan.competing = append(plan.competing, other.ID)
		plan.docs = append(plan.docs, meta.doc)
		plan.digests[other.ID] = meta.digest
	}
	run.Status, run.Competing = RunRan, plan.competing
	run.Digest, run.SetDigest = plan.digests[skill.ID], setDigest(plan.competing, plan.digests)
	if len(plan.competing) < 2 {
		run.Warnings = append(run.Warnings, "no other skill competes for these prompts, so a stolen trigger cannot be measured")
	}
	plan.cases, plan.ready = Expand(authored), true
	return plan
}

// activate measures one skill with the offline ranker and adds its positive
// prompts to the report's confusion matrix.
func (e *activationEnv) activate(skill *Skill) ActivationSkill {
	plan := e.prepare(skill)
	run := plan.run
	if !plan.ready {
		return run
	}
	measure(&run, skill, plan.cases, plan.competing, plan.docs, e.threshold)
	report := e.report
	for _, st := range run.StolenBy {
		addConfusion(report.Confusion, skill.ID, st.Skill, st.Prompts)
	}
	for i := range run.Prompts {
		p := &run.Prompts[i]
		if p.Expect && p.Winner == skill.ID {
			addConfusion(report.Confusion, skill.ID, skill.ID, 1)
		} else if p.Expect && p.Winner == activationNone {
			addConfusion(report.Confusion, skill.ID, activationNone, 1)
		}
	}
	return run
}

func addConfusion(m map[string]map[string]int, expected, won string, n int) {
	if m[expected] == nil {
		m[expected] = map[string]int{}
	}
	m[expected][won] += n
}

// measure ranks every prompt of cases against the competing skills and fills in
// the skill's prompts and figures.
func measure(run *ActivationSkill, skill *Skill, cases []Case, competing []string, docs []skillsearch.Doc, threshold float64) {
	var tp, fp, fn, positives, negatives, atLow, atHigh int
	var rr float64
	stolen := map[string]int{}
	for i := range cases {
		c := &cases[i]
		if len(c.Files) > 0 || len(c.Assertions) > 0 || c.HasRubric() {
			run.Ignored++
		}
		p := rankPrompt(skill, c, competing, docs)
		fired := p.Winner == skill.ID
		switch {
		case p.Expect && fired:
			tp++
		case p.Expect:
			fn++
			if p.Winner != activationNone {
				stolen[p.Winner]++
			}
		case fired:
			fp++
		}
		if p.Expect {
			positives++
			if p.Rank != nil {
				rr += 1 / float64(*p.Rank)
				atLow += btoi(*p.Rank <= recallAtLow)
				atHigh += btoi(*p.Rank <= recallAtHigh)
			}
		} else {
			negatives++
		}
		p.Status = promptStatus(&p)
		run.Prompts = append(run.Prompts, p)
	}
	run.Recall = newRate(tp, positives)
	run.Precision = newRate(tp, tp+fp)
	run.FalseActivation = newRate(fp, negatives)
	if positives > 0 {
		run.RecallAt1 = ptr(round(float64(atLow) / float64(positives)))
		run.RecallAt3 = ptr(round(float64(atHigh) / float64(positives)))
		run.MRR = ptr(round(rr / float64(positives)))
	}
	run.StolenBy = stolenList(stolen, positives)
	run.Passing = passingShare(run.Prompts, threshold)
}

// passingShare says whether enough prompts passed to reach the threshold.
func passingShare(prompts []ActivationPrompt, threshold float64) bool {
	passed := 0
	for i := range prompts {
		if prompts[i].Status == PromptPassed {
			passed++
		}
	}
	return len(prompts) > 0 && float64(passed)/float64(len(prompts)) >= threshold
}

// rankPrompt ranks one prompt against the competing skills and records the
// winner and where the skill under test landed.
func rankPrompt(skill *Skill, c *Case, competing []string, docs []skillsearch.Doc) ActivationPrompt {
	hits := skillsearch.Rank(docs, c.Prompt)
	p := ActivationPrompt{Case: c.ID, Expect: c.Expects(), NearMiss: c.NearMissOf != "", Runs: 1, Winner: activationNone}
	if len(hits) > 0 {
		p.Winner, p.TopScore = competing[hits[0].Index], hits[0].Score
	}
	for rank, h := range hits {
		if competing[h.Index] == skill.ID {
			r := rank + 1
			p.Rank = &r
			break
		}
	}
	if p.Winner == skill.ID {
		p.Rate = 1
	}
	return p
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func ptr(v float64) *float64 { return &v }

// promptStatus judges a prompt by its activation rate against the thresholds.
func promptStatus(p *ActivationPrompt) string {
	if p.Expect {
		if p.Rate >= activationPositiveMin {
			return PromptPassed
		}
		return PromptFailed
	}
	if p.Rate <= activationNegativeMax {
		return PromptPassed
	}
	return PromptFailed
}

func stolenList(stolen map[string]int, positives int) []StolenBy {
	out := make([]StolenBy, 0, len(stolen))
	for skill, n := range stolen {
		out = append(out, StolenBy{Skill: skill, Prompts: n, Share: round(float64(n) / float64(positives))})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Prompts != out[b].Prompts {
			return out[a].Prompts > out[b].Prompts
		}
		return out[a].Skill < out[b].Skill
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

// relativeProblems makes problem paths relative to the project.
func relativeProblems(configDir string, problems []Problem) []Problem {
	root := filepath.Dir(configDir)
	out := make([]Problem, len(problems))
	for i, p := range problems {
		if rel, err := filepath.Rel(root, p.File); err == nil && within(root, p.File) {
			p.File = filepath.ToSlash(rel)
		}
		out[i] = p
	}
	return out
}

// loadSkillMetas reads the searchable text and digest of every skill. A skill
// that cannot be read is left out and its reason returned.
func loadSkillMetas(skills []Skill) (metas map[string]skillMeta, bad map[string]string) {
	metas = map[string]skillMeta{}
	bad = map[string]string{}
	for i := range skills {
		s := &skills[i]
		doc, err := readSkillDoc(s)
		if err != nil {
			bad[s.ID] = err.Error()
			continue
		}
		digest, err := SkillDigest(s.Dir)
		if err != nil {
			bad[s.ID] = "cannot compute the skill digest: " + err.Error()
			continue
		}
		metas[s.ID] = skillMeta{doc: doc, digest: digest}
	}
	return metas, bad
}

// readSkillDoc reads name, description, triggers and keywords from a SKILL.md,
// the fields the find_skill ranker searches. A symlinked SKILL.md is refused.
func readSkillDoc(skill *Skill) (skillsearch.Doc, error) {
	file := filepath.Join(skill.Dir, "SKILL.md")
	info, err := os.Lstat(file)
	if err != nil {
		return skillsearch.Doc{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	if !info.Mode().IsRegular() {
		return skillsearch.Doc{}, fmt.Errorf("SKILL.md is not a regular file")
	}
	data, err := readBounded(file)
	if err != nil {
		return skillsearch.Doc{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	front, err := frontmatterOf(data)
	if err != nil {
		return skillsearch.Doc{}, fmt.Errorf("SKILL.md frontmatter: %w", err)
	}
	doc := skillsearch.Doc{Name: stringOf(front["name"]), Description: stringOf(front[frontmatterDescription]),
		Triggers: listOf(front["triggers"]), Keywords: listOf(front["keywords"])}
	if doc.Name == "" {
		doc.Name = skill.ID
	}
	if strings.TrimSpace(doc.Description) == "" {
		doc.Description = doc.Name // as find_skill does: a skill without a description stays findable by name
	}
	return doc, nil
}

// frontmatterOf returns the YAML frontmatter of a markdown file; none is an empty map.
func frontmatterOf(data []byte) (map[string]any, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	front := map[string]any{}
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return front, nil
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return front, nil
	}
	if err := yaml.NewDecoder(bytes.NewReader([]byte(rest[:end]))).Decode(&front); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if front == nil {
		front = map[string]any{}
	}
	return front, nil
}

func stringOf(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// listOf reads a frontmatter value that is a list of strings or one comma-separated string.
func listOf(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case string:
		for _, part := range strings.Split(t, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
