package skillsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/oops"
)

// ResultSchemaVersion versions the JSON `search --eval` prints.
const ResultSchemaVersion = 1

const (
	// ModeLexical is BM25F alone: offline, deterministic, the reference and the fallback.
	ModeLexical = "lexical"

	metricTop1   = "top1"
	metricRecall = "recall"
	metricHit    = "hit"
	metricMRR    = "mrr"
	metricNDCG   = "ndcg"
	keyRecallAtK = "recall_at_k"
	keyNDCG      = "ndcg_at_k"

	bootstrapResamples = 1000
	bootstrapSeed      = 1
)

// Metrics are the retrieval scores of one set of cases. NDCG is present when a
// case is graded; AvoidTop1 when a case lists skills that must not rank first
// (the share of those cases where one did; lower is better).
type Metrics struct {
	N        int      `json:"n"`
	Top1     float64  `json:"top1"`
	RecallAt float64  `json:"recall_at_k"`
	HitAt    float64  `json:"hit_at_k"`
	MRR      float64  `json:"mrr"`
	NDCG     *float64 `json:"ndcg_at_k,omitempty"`
	AvoidTop *float64 `json:"avoid_top1,omitempty"`
}

// Interval is a 95% bootstrap interval; it is informational and never gated on.
type Interval struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// Diff is a paired difference between two modes with its 95% bootstrap
// interval (over the same cases): an interval that excludes 0 is a real change.
type Diff struct {
	Diff float64 `json:"diff"`
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// CaseResult is the outcome of one positive case. Rank is the 1-based rank of
// the first relevant skill, null when none was retrieved. Degraded is set when
// a vector mode fell back to lexical for the case.
type CaseResult struct {
	ID       string   `json:"id"`
	Query    string   `json:"query,omitempty"`
	Role     string   `json:"role,omitempty"`
	Expected []string `json:"expected"`
	Got      []string `json:"got"`
	Rank     *int     `json:"rank"`
	Hit      bool     `json:"hit"`
	Violated bool     `json:"avoid_violated,omitempty"`
	Degraded string   `json:"degraded,omitempty"`
	Tags     []string `json:"tags,omitempty"`

	top1, recall, ndcg float64
	graded, hasAvoid   bool
}

// NegativeResult is a case with nothing relevant: it reports what ranked first.
// Score is the ranker's own (BM25F, RRF or cosine), informational only.
type NegativeResult struct {
	ID       string   `json:"id"`
	Top      string   `json:"top,omitempty"`
	TopScore float64  `json:"top_score"`
	Avoid    []string `json:"avoid,omitempty"`
	Violated bool     `json:"avoid_violated,omitempty"`
	Degraded string   `json:"degraded,omitempty"`
	Tags     []string `json:"tags,omitempty"`

	hasAvoid bool
}

// Flips are the cases whose hit@k changed against a baseline.
type Flips struct {
	Regressed []string `json:"regressed"`
	Fixed     []string `json:"fixed"`
}

// ModeResult is the full outcome of one ranking mode.
type ModeResult struct {
	Metrics       Metrics             `json:"metrics"`
	Tags          map[string]Metrics  `json:"tags,omitempty"`
	CI95          map[string]Interval `json:"ci95"`
	Cases         []CaseResult        `json:"cases"`
	Misses        []CaseResult        `json:"misses"`
	Negatives     []NegativeResult    `json:"negatives,omitempty"`
	DegradedCases int                 `json:"degraded_cases,omitempty"`
}

// IndexInfo says which vectors a hybrid or vector evaluation used.
type IndexInfo struct {
	Model   string `json:"model"`
	Dims    int    `json:"dims"`
	DType   string `json:"dtype"`
	Rows    int    `json:"rows"`
	Stale   int    `json:"stale_items"`
	Missing int    `json:"missing_items"`
}

// Result is the document `search --eval --format json` prints, and the file a
// later run takes as its --baseline. The top-level cases, tags, intervals,
// misses and negatives are those of Mode, the first mode asked for; ByMode holds
// every mode evaluated, and Paired the difference of each against lexical when
// both were.
type Result struct {
	SchemaVersion int                        `json:"schema_version"`
	K             int                        `json:"k"`
	N             int                        `json:"n"`
	NNegative     int                        `json:"n_negative"`
	Mode          string                     `json:"mode,omitempty"`
	Modes         map[string]Metrics         `json:"modes"`
	Tags          map[string]Metrics         `json:"tags,omitempty"`
	CI95          map[string]Interval        `json:"ci95"`
	Cases         []CaseResult               `json:"cases"`
	Misses        []CaseResult               `json:"misses"`
	Negatives     []NegativeResult           `json:"negatives,omitempty"`
	ByMode        map[string]*ModeResult     `json:"by_mode,omitempty"`
	Paired        map[string]map[string]Diff `json:"paired_vs_lexical,omitempty"`
	Index         *IndexInfo                 `json:"index,omitempty"`
	Flips         *Flips                     `json:"flips_vs_baseline,omitempty"`
	GateFailures  []string                   `json:"gate_failures,omitempty"`
}

// EvalEnv is what an evaluation ranks against.
type EvalEnv struct {
	Items    []Item
	Cfg      Config
	Index    *Index
	Embedder Embedder
	// Scope resolves a case's role to an item filter; nil rejects cases with a role.
	Scope func(role string) (allow func(item int) bool, err error)
}

// Eval ranks every case of f in each mode and scores the result at k. The first
// mode is the primary one: it fills the top-level fields and is what the gates
// check. It returns an AR9D2 error for a case whose role is unknown or whose
// expected skill is outside its role.
func Eval(ctx context.Context, env *EvalEnv, f *CaseFile, k int, modes []string) (*Result, error) {
	if len(modes) == 0 {
		modes = []string{ModeLexical}
	}
	scopes, err := resolveScopes(env, f)
	if err != nil {
		return nil, err
	}
	ids := IDs(env.Items)
	ranker := &Ranker{Items: env.Items, Cfg: env.Cfg, Index: env.Index, Embedder: env.Embedder}
	res := &Result{SchemaVersion: ResultSchemaVersion, K: k, Mode: modes[0], Modes: map[string]Metrics{}, Misses: []CaseResult{}}
	byMode := map[string]*ModeResult{}
	for _, mode := range modes {
		if _, dup := byMode[mode]; dup {
			continue
		}
		mr := &ModeResult{Cases: []CaseResult{}, Misses: []CaseResult{}}
		for i := range f.Cases {
			c := &f.Cases[i]
			sr := ranker.SearchMode(ctx, mode, c.Query, scopes[c.Role])
			if err := ctx.Err(); err != nil {
				return nil, oops.Wrapf(err, "evaluation interrupted")
			}
			if sr.Degraded != "" && mode != ModeLexical {
				mr.DegradedCases++
			}
			ranked := make([]string, len(sr.Hits))
			scores := make([]float64, len(sr.Hits))
			for j, h := range sr.Hits {
				ranked[j], scores[j] = ids[h.Index], h.Score
			}
			if len(c.Expect) == 0 {
				neg := NegativeResult{ID: c.ID, Tags: c.Tags, Avoid: c.Avoid, Degraded: sr.Degraded, hasAvoid: len(c.Avoid) > 0}
				if len(ranked) > 0 {
					neg.Top, neg.TopScore = ranked[0], scores[0]
					neg.Violated = len(ranked) > 0 && contains(c.Avoid, ranked[0])
				}
				mr.Negatives = append(mr.Negatives, neg)
				continue
			}
			cr := scoreCase(c, ranked, k)
			cr.Degraded = sr.Degraded
			mr.Cases = append(mr.Cases, cr)
		}
		mr.Metrics = aggregate(mr.Cases, mr.Negatives)
		mr.Tags = tagMetrics(mr.Cases)
		mr.CI95 = bootstrap(mr.Cases)
		for i := range mr.Cases {
			if !mr.Cases[i].Hit {
				mr.Misses = append(mr.Misses, mr.Cases[i])
			}
		}
		byMode[mode] = mr
		res.Modes[mode] = mr.Metrics
	}
	p := byMode[modes[0]]
	res.N, res.NNegative = len(p.Cases), len(p.Negatives)
	res.Tags, res.CI95, res.Cases, res.Misses, res.Negatives = p.Tags, p.CI95, p.Cases, p.Misses, p.Negatives
	res.ByMode = byMode
	if len(byMode) > 1 {
		res.Paired = paired(byMode)
	}
	if env.Index != nil && (byMode[ModeHybrid] != nil || byMode[ModeVector] != nil) {
		st := env.Index.Check(env.Items, env.Cfg)
		res.Index = &IndexInfo{
			Model: env.Index.Manifest.Model, Dims: env.Index.Manifest.Dims, DType: env.Index.Manifest.DType,
			Rows: env.Index.Len(), Stale: len(st.Stale), Missing: len(st.Missing),
		}
	}
	return res, nil
}

// resolveScopes resolves the role of every case once.
func resolveScopes(env *EvalEnv, f *CaseFile) (map[string]func(int) bool, error) {
	scopes := map[string]func(int) bool{"": nil}
	var problems []string
	for i := range f.Cases {
		c := &f.Cases[i]
		if _, done := scopes[c.Role]; done {
			continue
		}
		if env.Scope == nil {
			problems = append(problems, fmt.Sprintf("case %q sets role %q, but no roles are configured", c.ID, c.Role))
			scopes[c.Role] = nil
			continue
		}
		allow, err := env.Scope(c.Role)
		if err != nil {
			problems = append(problems, fmt.Sprintf("case %q: %v", c.ID, err))
			scopes[c.Role] = nil
			continue
		}
		scopes[c.Role] = allow
	}
	index := map[string]int{}
	for i := range env.Items {
		index[env.Items[i].ID] = i
	}
	for i := range f.Cases {
		c := &f.Cases[i]
		allow := scopes[c.Role]
		if c.Role == "" || allow == nil {
			continue
		}
		for _, id := range c.ExpectIDs() {
			if at, ok := index[id]; ok && !allow(at) {
				problems = append(problems, fmt.Sprintf("case %q expects %q, which role %q does not serve", c.ID, id, c.Role))
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, oops.Code(CodeCasesInvalid).Errorf("%s: invalid cases file: %s", CodeCasesInvalid, strings.Join(problems, "; "))
	}
	return scopes, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func scoreCase(c *Case, ranked []string, k int) CaseResult {
	want := map[string]int{}
	graded := false
	for _, r := range c.Expect {
		want[r.ID] = r.Grade
		graded = graded || r.Graded
	}
	out := CaseResult{
		ID: c.ID, Query: c.Query, Role: c.Role, Expected: c.ExpectIDs(), Got: []string{}, Tags: c.Tags,
		graded: graded, hasAvoid: len(c.Avoid) > 0,
	}
	found := 0
	var dcg float64
	for pos, id := range ranked {
		g, relevant := want[id]
		if pos < k {
			out.Got = append(out.Got, id)
			if relevant {
				found++
				dcg += float64(g) / math.Log2(float64(pos)+2)
			}
		}
		if out.Rank == nil && relevant {
			rank := pos + 1
			out.Rank = &rank
		}
	}
	out.recall = float64(found) / float64(len(want))
	out.ndcg = ndcg(dcg, c.Expect, k)
	if out.Rank != nil {
		out.Hit = *out.Rank <= k
		if *out.Rank == 1 {
			out.top1 = 1
		}
	}
	out.Violated = len(ranked) > 0 && contains(c.Avoid, ranked[0])
	return out
}

// ndcg normalises dcg by the ideal ordering of the case's grades.
func ndcg(dcg float64, expect []Relevant, k int) float64 {
	grades := make([]int, len(expect))
	for i, r := range expect {
		grades[i] = r.Grade
	}
	sort.Sort(sort.Reverse(sort.IntSlice(grades)))
	var ideal float64
	for i, g := range grades {
		if i >= k {
			break
		}
		ideal += float64(g) / math.Log2(float64(i)+2)
	}
	if ideal == 0 {
		return 0
	}
	return dcg / ideal
}

func (c *CaseResult) reciprocal() float64 {
	if c.Rank == nil {
		return 0
	}
	return 1 / float64(*c.Rank)
}

func aggregate(cases []CaseResult, negatives []NegativeResult) Metrics {
	m := Metrics{N: len(cases)}
	anyGraded := false
	var avoidN, avoidHit float64
	for i := range cases {
		c := &cases[i]
		m.Top1 += c.top1
		m.RecallAt += c.recall
		if c.Hit {
			m.HitAt++
		}
		m.MRR += c.reciprocal()
		anyGraded = anyGraded || c.graded
		if c.hasAvoid {
			avoidN++
			if c.Violated {
				avoidHit++
			}
		}
	}
	for i := range negatives {
		if negatives[i].hasAvoid {
			avoidN++
			if negatives[i].Violated {
				avoidHit++
			}
		}
	}
	if len(cases) > 0 {
		n := float64(len(cases))
		m.Top1, m.RecallAt, m.HitAt, m.MRR = round(m.Top1/n), round(m.RecallAt/n), round(m.HitAt/n), round(m.MRR/n)
		if anyGraded {
			var sum float64
			for i := range cases {
				sum += cases[i].ndcg
			}
			v := round(sum / n)
			m.NDCG = &v
		}
	}
	if avoidN > 0 {
		v := round(avoidHit / avoidN)
		m.AvoidTop = &v
	}
	return m
}

func tagMetrics(cases []CaseResult) map[string]Metrics {
	by := map[string][]CaseResult{}
	for i := range cases {
		for _, t := range cases[i].Tags {
			by[t] = append(by[t], cases[i])
		}
	}
	if len(by) == 0 {
		return nil
	}
	out := make(map[string]Metrics, len(by))
	for t, cs := range by {
		out[t] = aggregate(cs, nil)
	}
	return out
}

// series are the per-case values of the metrics that have an interval.
func series(cases []CaseResult) map[string][]float64 {
	out := map[string][]float64{metricTop1: {}, keyRecallAtK: {}, metricMRR: {}}
	anyGraded := false
	for i := range cases {
		c := &cases[i]
		out[metricTop1] = append(out[metricTop1], c.top1)
		out[keyRecallAtK] = append(out[keyRecallAtK], c.recall)
		out[metricMRR] = append(out[metricMRR], c.reciprocal())
		anyGraded = anyGraded || c.graded
	}
	if anyGraded {
		out[keyNDCG] = []float64{}
		for i := range cases {
			out[keyNDCG] = append(out[keyNDCG], cases[i].ndcg)
		}
	}
	return out
}

// bootstrap resamples the cases with a fixed seed, so the interval is the same
// on every run over the same data.
func bootstrap(cases []CaseResult) map[string]Interval {
	if len(cases) == 0 {
		return map[string]Interval{}
	}
	out := map[string]Interval{}
	for name, vals := range series(cases) {
		means := resampleMeans(vals)
		out[name] = Interval{Low: round(means[int(0.025*float64(len(means)))]), High: round(means[int(0.975*float64(len(means)))-1])}
	}
	return out
}

// resampleMeans is the sorted bootstrap distribution of the mean of vals.
func resampleMeans(vals []float64) []float64 {
	rng := rand.New(rand.NewSource(bootstrapSeed)) //nolint:gosec // deterministic resampling, not security
	means := make([]float64, bootstrapResamples)
	n := float64(len(vals))
	for r := range means {
		var sum float64
		for range vals {
			sum += vals[rng.Intn(len(vals))]
		}
		means[r] = sum / n
	}
	sort.Float64s(means)
	return means
}

// paired compares every mode with lexical over the same cases: the mean
// difference per metric and its bootstrap interval.
func paired(byMode map[string]*ModeResult) map[string]map[string]Diff {
	base := byMode[ModeLexical]
	if base == nil {
		return nil
	}
	out := map[string]map[string]Diff{}
	bs := series(base.Cases)
	for mode, mr := range byMode {
		if mode == ModeLexical || len(mr.Cases) != len(base.Cases) || len(mr.Cases) == 0 {
			continue
		}
		diffs := map[string]Diff{}
		for name, vals := range series(mr.Cases) {
			ref, ok := bs[name]
			if !ok {
				continue
			}
			delta := make([]float64, len(vals))
			var mean float64
			for i := range vals {
				delta[i] = vals[i] - ref[i]
				mean += delta[i]
			}
			means := resampleMeans(delta)
			diffs[name] = Diff{
				Diff: round(mean / float64(len(vals))),
				Low:  round(means[int(0.025*float64(len(means)))]), High: round(means[int(0.975*float64(len(means)))-1]),
			}
		}
		out[mode] = diffs
	}
	return out
}

func round(v float64) float64 { return math.Round(v*1e4) / 1e4 }

// LoadBaseline reads a Result written by an earlier run.
func LoadBaseline(path string) (*Result, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // an explicit user-supplied path
	if err != nil {
		return nil, oops.Wrapf(err, "read baseline")
	}
	var base Result
	if err := json.Unmarshal(raw, &base); err != nil {
		return nil, oops.Wrapf(err, "parse baseline %s", path)
	}
	if base.SchemaVersion != ResultSchemaVersion {
		return nil, oops.Errorf("baseline %s has schema_version %d; this build reads %d", path, base.SchemaVersion, ResultSchemaVersion)
	}
	return &base, nil
}

// CompareBaseline records which cases went from hit to miss and back. Cases
// present in only one of the two runs are ignored: they are not comparable. It
// fails when the two runs used different cut-offs (a hit at k=10 is not a hit at
// k=3) or ranked in different modes (a hybrid hit is not a lexical hit), so every
// flip would be an artifact of the flag.
func (r *Result) CompareBaseline(base *Result) error {
	if bm, rm := baselineMode(base.Mode), baselineMode(r.Mode); bm != rm {
		return oops.Hint("rerun with --mode "+bm+", or save a new baseline with --out").
			Errorf("the baseline was ranked in %s mode but this run uses %s", bm, rm)
	}
	if base.K != r.K {
		return oops.Hint("rerun with --k "+strconv.Itoa(base.K)+", or save a new baseline with --out").
			Errorf("the baseline was evaluated at k=%d but this run uses k=%d", base.K, r.K)
	}
	was := map[string]bool{}
	for i := range base.Cases {
		was[base.Cases[i].ID] = base.Cases[i].Hit
	}
	flips := &Flips{Regressed: []string{}, Fixed: []string{}}
	for i := range r.Cases {
		c := &r.Cases[i]
		prev, ok := was[c.ID]
		switch {
		case !ok:
		case prev && !c.Hit:
			flips.Regressed = append(flips.Regressed, c.ID)
		case !prev && c.Hit:
			flips.Fixed = append(flips.Fixed, c.ID)
		}
	}
	sort.Strings(flips.Regressed)
	sort.Strings(flips.Fixed)
	r.Flips = flips
	return nil
}

// baselineMode names the mode of a result; one written before modes existed is lexical.
func baselineMode(m string) string {
	if m == "" {
		return ModeLexical
	}
	return m
}

// Minimums are absolute floors, keyed by metric name.
type Minimums map[string]float64

// ParseMinimums parses `top1=0.6,mrr=0.7`. Metrics: top1, recall (recall@k),
// hit (hit@k), mrr and ndcg (ndcg@k).
func ParseMinimums(spec string) (Minimums, error) {
	out := Minimums{}
	if strings.TrimSpace(spec) == "" {
		return out, nil
	}
	for _, part := range strings.Split(spec, ",") {
		key, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		key = canonicalMetric(key)
		if !ok || key == "" {
			return nil, oops.Errorf("--min %q: want metric=value pairs such as top1=0.6,mrr=0.7 (metrics: top1, recall, hit, mrr, ndcg)", part)
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil || f < 0 || f > 1 {
			return nil, oops.Errorf("--min %q: value must be a number between 0 and 1", part)
		}
		out[key] = f
	}
	return out, nil
}

func canonicalMetric(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case metricTop1, "top-1":
		return metricTop1
	case metricRecall, "recall@k", keyRecallAtK:
		return metricRecall
	case metricHit, "hit@k", "hit_at_k":
		return metricHit
	case metricMRR:
		return metricMRR
	case metricNDCG, "ndcg@k", keyNDCG:
		return metricNDCG
	}
	return ""
}

// Gate lists every failed gate, as AR9D4 messages; empty means the run passes.
// maxFlips < 0 disables the flip gate.
func (r *Result) Gate(mins Minimums, maxFlips int) []string {
	mode := r.Mode
	if mode == "" {
		mode = ModeLexical
	}
	m := r.Modes[mode]
	have := map[string]float64{metricTop1: m.Top1, metricRecall: m.RecallAt, metricHit: m.HitAt, metricMRR: m.MRR}
	if m.NDCG != nil {
		have[metricNDCG] = *m.NDCG
	}
	var failed []string
	names := make([]string, 0, len(mins))
	for name := range mins {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, ok := have[name]; !ok {
			failed = append(failed, fmt.Sprintf("%s: %s cannot be checked: no case is graded", CodeEvalRegression, name))
			continue
		}
		if have[name] < mins[name] {
			failed = append(failed, fmt.Sprintf("%s: %s is %.4f, below the minimum %.4f", CodeEvalRegression, name, have[name], mins[name]))
		}
	}
	if r.Flips != nil && maxFlips >= 0 && len(r.Flips.Regressed) > maxFlips {
		failed = append(failed, fmt.Sprintf("%s: %d cases regressed against the baseline (max %d): %s",
			CodeEvalRegression, len(r.Flips.Regressed), maxFlips, strings.Join(r.Flips.Regressed, ", ")))
	}
	return failed
}
