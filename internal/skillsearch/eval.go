package skillsearch

import (
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
	// ModeLexical is the only ranking mode of this release.
	ModeLexical = "lexical"

	metricTop1   = "top1"
	metricRecall = "recall"
	metricHit    = "hit"
	metricMRR    = "mrr"
	keyRecallAtK = "recall_at_k"

	bootstrapResamples = 1000
	bootstrapSeed      = 1
)

// Metrics are the retrieval scores of one set of cases.
type Metrics struct {
	N        int     `json:"n"`
	Top1     float64 `json:"top1"`
	RecallAt float64 `json:"recall_at_k"`
	HitAt    float64 `json:"hit_at_k"`
	MRR      float64 `json:"mrr"`
}

// Interval is a 95% bootstrap interval; it is informational and never gated on.
type Interval struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// CaseResult is the outcome of one positive case. Rank is the 1-based rank of
// the first relevant skill, null when none was retrieved.
type CaseResult struct {
	ID       string   `json:"id"`
	Query    string   `json:"query,omitempty"`
	Expected []string `json:"expected"`
	Got      []string `json:"got"`
	Rank     *int     `json:"rank"`
	Hit      bool     `json:"hit"`
	Tags     []string `json:"tags,omitempty"`

	top1, recall float64
}

// NegativeResult is a case with nothing relevant: it reports what ranked first.
type NegativeResult struct {
	ID       string   `json:"id"`
	Top      string   `json:"top,omitempty"`
	TopScore float64  `json:"top_score"`
	Tags     []string `json:"tags,omitempty"`
}

// Flips are the cases whose hit@k changed against a baseline.
type Flips struct {
	Regressed []string `json:"regressed"`
	Fixed     []string `json:"fixed"`
}

// Result is the document `search --eval --format json` prints, and the file a
// later run takes as its --baseline.
type Result struct {
	SchemaVersion int                 `json:"schema_version"`
	K             int                 `json:"k"`
	N             int                 `json:"n"`
	NNegative     int                 `json:"n_negative"`
	Modes         map[string]Metrics  `json:"modes"`
	Tags          map[string]Metrics  `json:"tags,omitempty"`
	CI95          map[string]Interval `json:"ci95"`
	Cases         []CaseResult        `json:"cases"`
	Misses        []CaseResult        `json:"misses"`
	Negatives     []NegativeResult    `json:"negatives,omitempty"`
	Flips         *Flips              `json:"flips_vs_baseline,omitempty"`
	GateFailures  []string            `json:"gate_failures,omitempty"`
}

// Eval ranks every case of f against docs (ids[i] is the stable id of docs[i])
// and scores the result at k.
func Eval(docs []Doc, ids []string, f *CaseFile, k int) *Result {
	res := &Result{SchemaVersion: ResultSchemaVersion, K: k, Modes: map[string]Metrics{}, Misses: []CaseResult{}}
	for i := range f.Cases {
		c := &f.Cases[i]
		hits := Rank(docs, c.Query)
		if len(c.Expect) == 0 {
			neg := NegativeResult{ID: c.ID, Tags: c.Tags}
			if len(hits) > 0 {
				neg.Top, neg.TopScore = ids[hits[0].Index], hits[0].Score
			}
			res.Negatives = append(res.Negatives, neg)
			continue
		}
		res.Cases = append(res.Cases, scoreCase(c, ids, hits, k))
	}
	res.N, res.NNegative = len(res.Cases), len(res.Negatives)
	res.Modes[ModeLexical] = aggregate(res.Cases)
	res.Tags = tagMetrics(res.Cases)
	res.CI95 = bootstrap(res.Cases)
	for i := range res.Cases {
		if !res.Cases[i].Hit {
			res.Misses = append(res.Misses, res.Cases[i])
		}
	}
	return res
}

func scoreCase(c *Case, ids []string, hits []Hit, k int) CaseResult {
	want := map[string]bool{}
	for _, id := range c.Expect {
		want[id] = true
	}
	out := CaseResult{ID: c.ID, Query: c.Query, Expected: append([]string{}, c.Expect...), Got: []string{}, Tags: c.Tags}
	found := 0
	for pos, h := range hits {
		id := ids[h.Index]
		if pos < k {
			out.Got = append(out.Got, id)
			if want[id] {
				found++
			}
		}
		if out.Rank == nil && want[id] {
			rank := pos + 1
			out.Rank = &rank
		}
	}
	out.recall = float64(found) / float64(len(want))
	if out.Rank != nil {
		out.Hit = *out.Rank <= k
		if *out.Rank == 1 {
			out.top1 = 1
		}
	}
	return out
}

func (c *CaseResult) reciprocal() float64 {
	if c.Rank == nil {
		return 0
	}
	return 1 / float64(*c.Rank)
}

func aggregate(cases []CaseResult) Metrics {
	m := Metrics{N: len(cases)}
	if len(cases) == 0 {
		return m
	}
	for i := range cases {
		c := &cases[i]
		m.Top1 += c.top1
		m.RecallAt += c.recall
		if c.Hit {
			m.HitAt++
		}
		m.MRR += c.reciprocal()
	}
	n := float64(len(cases))
	m.Top1, m.RecallAt, m.HitAt, m.MRR = round(m.Top1/n), round(m.RecallAt/n), round(m.HitAt/n), round(m.MRR/n)
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
		out[t] = aggregate(cs)
	}
	return out
}

// bootstrap resamples the cases with a fixed seed, so the interval is the same
// on every run over the same data.
func bootstrap(cases []CaseResult) map[string]Interval {
	if len(cases) == 0 {
		return map[string]Interval{}
	}
	rng := rand.New(rand.NewSource(bootstrapSeed)) //nolint:gosec // deterministic resampling, not security
	series := map[string][]float64{metricTop1: nil, keyRecallAtK: nil, metricMRR: nil}
	for r := 0; r < bootstrapResamples; r++ {
		var t1, rc, mr float64
		for range cases {
			c := &cases[rng.Intn(len(cases))]
			t1, rc, mr = t1+c.top1, rc+c.recall, mr+c.reciprocal()
		}
		n := float64(len(cases))
		series[metricTop1] = append(series[metricTop1], t1/n)
		series[keyRecallAtK] = append(series[keyRecallAtK], rc/n)
		series[metricMRR] = append(series[metricMRR], mr/n)
	}
	out := map[string]Interval{}
	for name, vals := range series {
		sort.Float64s(vals)
		out[name] = Interval{Low: round(vals[int(0.025*float64(len(vals)))]), High: round(vals[int(0.975*float64(len(vals)))-1])}
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
// fails when the two runs used different cut-offs: a hit at k=10 is not a hit at
// k=3, so every flip would be an artifact of the flag.
func (r *Result) CompareBaseline(base *Result) error {
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

// Minimums are absolute floors, keyed by metric name.
type Minimums map[string]float64

// ParseMinimums parses `top1=0.6,mrr=0.7`. Metrics: top1, recall (recall@k),
// hit (hit@k) and mrr.
func ParseMinimums(spec string) (Minimums, error) {
	out := Minimums{}
	if strings.TrimSpace(spec) == "" {
		return out, nil
	}
	for _, part := range strings.Split(spec, ",") {
		key, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		key = canonicalMetric(key)
		if !ok || key == "" {
			return nil, oops.Errorf("--min %q: want metric=value pairs such as top1=0.6,mrr=0.7 (metrics: top1, recall, hit, mrr)", part)
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
	}
	return ""
}

// Gate lists every failed gate, as AR9D4 messages; empty means the run passes.
// maxFlips < 0 disables the flip gate.
func (r *Result) Gate(mins Minimums, maxFlips int) []string {
	m := r.Modes[ModeLexical]
	have := map[string]float64{metricTop1: m.Top1, metricRecall: m.RecallAt, metricHit: m.HitAt, metricMRR: m.MRR}
	var failed []string
	names := make([]string, 0, len(mins))
	for name := range mins {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
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
