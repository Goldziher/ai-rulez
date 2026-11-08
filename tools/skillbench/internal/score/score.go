// Package score turns verdicts into the published numbers: confusion counts from each tool's own
// verdict, a severity sweep, clustered bootstrap intervals and paired exact McNemar tests.
package score

import (
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
	"slices"

	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/dataset"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/run"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/scanner"
)

// Counts is a confusion matrix. A scanner error sits in FN or TN: it detected nothing.
type Counts struct {
	TP int `json:"tp"`
	FN int `json:"fn"`
	FP int `json:"fp"`
	TN int `json:"tn"`
}

// Rates are the derived measures.
type Rates struct {
	Recall    float64 `json:"recall"`
	FPR       float64 `json:"fpr"`
	Precision float64 `json:"precision"`
	F1        float64 `json:"f1"`
}

// Interval is a 95% percentile bootstrap interval.
type Interval struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// ToolResult is one tool on one track.
type ToolResult struct {
	Tool       string            `json:"tool"`
	Version    string            `json:"version"`
	Counts     Counts            `json:"counts"`
	Rates      Rates             `json:"rates"`
	RecallCI   Interval          `json:"recall_ci95"`
	FPRCI      Interval          `json:"fpr_ci95"`
	F1CI       Interval          `json:"f1_ci95"`
	Errors     map[string]int    `json:"errors"`
	Sweep      map[string]Rates  `json:"severity_sweep"`
	BySource   map[string]Counts `json:"by_source"`
	ByEvidence map[string]Counts `json:"by_evidence"`
	TopRules   []RuleCount       `json:"top_rules"`
	Unstable   int               `json:"unstable_samples"`
}

// RuleCount is how often a rule fired, and on how many malicious samples.
type RuleCount struct {
	Rule      string `json:"rule"`
	Samples   int    `json:"samples"`
	Malicious int    `json:"malicious"`
}

// Paired compares two tools on the same samples.
type Paired struct {
	A      string  `json:"a"`
	B      string  `json:"b"`
	OnlyA  int     `json:"only_a_correct"`
	OnlyB  int     `json:"only_b_correct"`
	PValue float64 `json:"mcnemar_exact_p"`
}

// Summary is the published result of one track.
type Summary struct {
	Protocol   string          `json:"protocol_sha256"`
	Claim      string          `json:"claim"`
	Dataset    string          `json:"dataset"`
	Revision   string          `json:"revision"`
	Track      string          `json:"track"`
	Population string          `json:"population_sha256"`
	Samples    int             `json:"samples"`
	Malicious  int             `json:"malicious"`
	Benign     int             `json:"benign"`
	Runs       int             `json:"runs"`
	Dropped    dataset.Dropped `json:"dropped"`
	Tools      []ToolResult    `json:"tools"`
	Paired     []Paired        `json:"paired"`
}

// Run is one complete verdict set: a repetition on one machine.
type Run struct {
	Records []run.Record
}

// Compute builds the summary. Every run of a tool must cover the population exactly; disagreement
// between runs is counted per sample, and the first run is the one scored.
func Compute(p *dataset.Protocol, m *dataset.Materialized, runs []Run, versions map[string]string) (*Summary, error) {
	s := &Summary{Protocol: p.SHA256, Claim: p.Claim, Dataset: p.Dataset.Name, Revision: p.Dataset.Revision,
		Track: m.Track, Population: m.Digest, Samples: len(m.Samples), Runs: len(runs), Dropped: m.Dropped}
	for _, x := range m.Samples {
		if x.Malicious {
			s.Malicious++
		} else {
			s.Benign++
		}
	}
	byTool := map[string][]map[string]run.Record{}
	for _, r := range runs {
		perTool := map[string]map[string]run.Record{}
		for _, rec := range r.Records {
			if perTool[rec.Tool] == nil {
				perTool[rec.Tool] = map[string]run.Record{}
			}
			if _, dup := perTool[rec.Tool][rec.ID]; dup {
				return nil, fmt.Errorf("%s: %s appears twice in one run", rec.Tool, rec.ID)
			}
			perTool[rec.Tool][rec.ID] = rec
		}
		for tool, recs := range perTool {
			byTool[tool] = append(byTool[tool], recs)
		}
	}
	verdicts := map[string]map[string]run.Record{}
	for _, tool := range scanner.Names {
		sets, ok := byTool[tool]
		if !ok {
			continue
		}
		for _, set := range sets {
			if len(set) != len(m.Samples) {
				return nil, fmt.Errorf("%s: a run covers %d of %d samples", tool, len(set), len(m.Samples))
			}
		}
		verdicts[tool] = sets[0]
		res := toolResult(p, m, tool, sets)
		res.Version = versions[tool]
		s.Tools = append(s.Tools, res)
	}
	for i := range s.Tools {
		for j := i + 1; j < len(s.Tools); j++ {
			s.Paired = append(s.Paired, paired(m, s.Tools[i].Tool, s.Tools[j].Tool, verdicts))
		}
	}
	return s, nil
}

func toolResult(p *dataset.Protocol, m *dataset.Materialized, tool string, sets []map[string]run.Record) ToolResult {
	first := sets[0]
	res := ToolResult{Tool: tool, Errors: map[string]int{}, Sweep: map[string]Rates{},
		BySource: map[string]Counts{}, ByEvidence: map[string]Counts{}}
	rules := map[string]*RuleCount{}
	for _, x := range m.Samples {
		r := first[x.ID]
		res.Counts.add(x.Malicious, r.Flagged)
		c := res.BySource[x.Source]
		c.add(x.Malicious, r.Flagged)
		res.BySource[x.Source] = c
		e := res.ByEvidence[x.Evidence]
		e.add(x.Malicious, r.Flagged)
		res.ByEvidence[x.Evidence] = e
		if r.Error != "" {
			res.Errors[r.Error]++
		}
		for _, name := range r.Rules {
			rc := rules[name]
			if rc == nil {
				rc = &RuleCount{Rule: name}
				rules[name] = rc
			}
			rc.Samples++
			if x.Malicious {
				rc.Malicious++
			}
		}
		for _, other := range sets[1:] {
			if !same(r, other[x.ID]) {
				res.Unstable++
				break
			}
		}
	}
	res.Rates = res.Counts.rates()
	for _, level := range p.Scoring.SeveritySweep {
		var c Counts
		for _, x := range m.Samples {
			c.add(x.Malicious, scanner.SeverityRank(first[x.ID].MaxSeverity) >= scanner.SeverityRank(level))
		}
		res.Sweep[level] = c.rates()
	}
	for _, rc := range rules {
		res.TopRules = append(res.TopRules, *rc)
	}
	slices.SortFunc(res.TopRules, func(a, b RuleCount) int {
		if a.Samples != b.Samples {
			return b.Samples - a.Samples
		}
		if a.Rule < b.Rule {
			return -1
		}
		return 1
	})
	if len(res.TopRules) > 15 {
		res.TopRules = res.TopRules[:15]
	}
	res.RecallCI, res.FPRCI, res.F1CI = bootstrap(p, m, first)
	return res
}

func same(a, b run.Record) bool {
	return a.Flagged == b.Flagged && a.MaxSeverity == b.MaxSeverity && a.Error == b.Error &&
		a.Findings == b.Findings && slices.Equal(a.Rules, b.Rules)
}

func (c *Counts) add(malicious, flagged bool) {
	switch {
	case malicious && flagged:
		c.TP++
	case malicious:
		c.FN++
	case flagged:
		c.FP++
	default:
		c.TN++
	}
}

func (c Counts) rates() Rates {
	r := Rates{Recall: ratio(c.TP, c.TP+c.FN), FPR: ratio(c.FP, c.FP+c.TN), Precision: ratio(c.TP, c.TP+c.FP)}
	if r.Recall+r.Precision > 0 {
		r.F1 = 2 * r.Recall * r.Precision / (r.Recall + r.Precision)
	}
	return r
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// bootstrap resamples whole clusters, because near-duplicate samples are not independent evidence.
func bootstrap(p *dataset.Protocol, m *dataset.Materialized, verdicts map[string]run.Record) (recall, fpr, f1 Interval) {
	clusters := map[string]*Counts{}
	var keys []string
	for _, x := range m.Samples {
		c := clusters[x.Cluster]
		if c == nil {
			c = &Counts{}
			clusters[x.Cluster] = c
			keys = append(keys, x.Cluster)
		}
		c.add(x.Malicious, verdicts[x.ID].Flagged)
	}
	slices.Sort(keys)
	if len(keys) == 0 {
		return recall, fpr, f1
	}
	n := p.Scoring.BootstrapResamples
	rng := rand.New(rand.NewPCG(p.Scoring.BootstrapSeed, 0)) //nolint:gosec // reproducible by design
	rs, fs, fp := make([]float64, n), make([]float64, n), make([]float64, n)
	for i := range n {
		var c Counts
		for range keys {
			k := clusters[keys[rng.IntN(len(keys))]]
			c.TP += k.TP
			c.FN += k.FN
			c.FP += k.FP
			c.TN += k.TN
		}
		r := c.rates()
		rs[i], fp[i], fs[i] = r.Recall, r.FPR, r.F1
	}
	return percentile(rs), percentile(fp), percentile(fs)
}

func percentile(v []float64) Interval {
	slices.Sort(v)
	at := func(q float64) float64 { return v[int(q*float64(len(v)-1))] }
	return Interval{Low: at(0.025), High: at(0.975)}
}

// paired runs an exact McNemar test on correctness.
func paired(m *dataset.Materialized, a, b string, verdicts map[string]map[string]run.Record) Paired {
	out := Paired{A: a, B: b}
	for _, x := range m.Samples {
		ca := verdicts[a][x.ID].Flagged == x.Malicious
		cb := verdicts[b][x.ID].Flagged == x.Malicious
		switch {
		case ca && !cb:
			out.OnlyA++
		case cb && !ca:
			out.OnlyB++
		}
	}
	out.PValue = mcnemar(out.OnlyA, out.OnlyB)
	return out
}

// mcnemar is the two-sided exact binomial test on the discordant pairs.
func mcnemar(b, c int) float64 {
	n := b + c
	if n == 0 {
		return 1
	}
	tail := new(big.Int)
	for i := range min(b, c) + 1 {
		tail.Add(tail, new(big.Int).Binomial(int64(n), int64(i)))
	}
	f := new(big.Float).SetInt(tail)
	f.Mul(f, big.NewFloat(2))
	f.Quo(f, new(big.Float).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(n)))) //nolint:gosec // n is non-negative
	p, _ := f.Float64()
	return math.Min(p, 1)
}
