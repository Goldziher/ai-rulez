package score

import (
	"math"
	"testing"

	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/dataset"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/run"
	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/scanner"
)

func protocol() *dataset.Protocol {
	return &dataset.Protocol{Scoring: dataset.Scoring{
		BootstrapResamples: 2000, BootstrapSeed: 7, SeveritySweep: []string{"medium", "high"},
	}}
}

func population() *dataset.Materialized {
	return &dataset.Materialized{Track: "test", Samples: []dataset.Sample{
		{ID: "m1", Malicious: true, Source: "A", Evidence: "static", Cluster: "c1"},
		{ID: "m2", Malicious: true, Source: "A", Evidence: "static", Cluster: "c1"},
		{ID: "m3", Malicious: true, Source: "B", Evidence: "static", Cluster: "c2"},
		{ID: "m4", Malicious: true, Source: "B", Evidence: "static", Cluster: "c3"},
		{ID: "b1", Source: "A", Evidence: "static", Cluster: "c4"},
		{ID: "b2", Source: "B", Evidence: "static", Cluster: "c5"},
	}}
}

func rec(id, tool string, flagged bool, sev string) run.Record {
	return run.Record{ID: id, Tool: tool, Verdict: scanner.Verdict{Flagged: flagged, MaxSeverity: sev, Rules: []string{"R1"}}}
}

func TestComputeCounts(t *testing.T) {
	records := []run.Record{
		rec("m1", "ai-rulez", true, "high"), rec("m2", "ai-rulez", true, "medium"),
		rec("m3", "ai-rulez", true, "medium"), rec("m4", "ai-rulez", false, ""),
		rec("b1", "ai-rulez", true, "medium"), rec("b2", "ai-rulez", false, ""),
	}
	s, err := Compute(protocol(), population(), []Run{{Records: records}}, map[string]string{"ai-rulez": "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	r := s.Tools[0]
	if r.Counts != (Counts{TP: 3, FN: 1, FP: 1, TN: 1}) {
		t.Fatalf("counts = %+v", r.Counts)
	}
	if r.Rates.Recall != 0.75 || r.Rates.FPR != 0.5 || r.Rates.Precision != 0.75 || r.Rates.F1 != 0.75 {
		t.Fatalf("rates = %+v", r.Rates)
	}
	if r.Sweep["high"].Recall != 0.25 {
		t.Fatalf("sweep = %+v", r.Sweep)
	}
	if r.BySource["B"] != (Counts{TP: 1, FN: 1, TN: 1}) {
		t.Fatalf("by source = %+v", r.BySource)
	}
	if r.Version != "1.0" || s.Malicious != 4 || s.Benign != 2 {
		t.Fatalf("summary = %+v", s)
	}
}

func TestErrorsStayInTheDenominator(t *testing.T) {
	m := &dataset.Materialized{Samples: []dataset.Sample{{ID: "m1", Malicious: true, Cluster: "c"}}}
	r := run.Record{ID: "m1", Tool: "ai-rulez", Verdict: scanner.Verdict{Error: "timeout"}}
	s, err := Compute(protocol(), m, []Run{{Records: []run.Record{r}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Tools[0].Counts.FN != 1 || s.Tools[0].Errors["timeout"] != 1 {
		t.Fatalf("result = %+v", s.Tools[0])
	}
}

func TestIncompleteRunFails(t *testing.T) {
	r := Run{Records: []run.Record{rec("m1", "ai-rulez", true, "high")}}
	if _, err := Compute(protocol(), population(), []Run{r}, nil); err == nil {
		t.Fatal("a shrunken denominator must fail")
	}
}

func TestDuplicateFails(t *testing.T) {
	r := Run{Records: []run.Record{rec("m1", "ai-rulez", true, "high"), rec("m1", "ai-rulez", true, "high")}}
	if _, err := Compute(protocol(), population(), []Run{r}, nil); err == nil {
		t.Fatal("a duplicate must fail")
	}
}

func TestUnstableSamplesAreCounted(t *testing.T) {
	m := &dataset.Materialized{Samples: []dataset.Sample{{ID: "m1", Malicious: true, Cluster: "c"}}}
	a := Run{Records: []run.Record{rec("m1", "ai-rulez", true, "high")}}
	b := Run{Records: []run.Record{rec("m1", "ai-rulez", false, "")}}
	s, err := Compute(protocol(), m, []Run{a, b}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Tools[0].Unstable != 1 {
		t.Fatalf("unstable = %d", s.Tools[0].Unstable)
	}
}

func TestBootstrapIsReproducibleAndClustered(t *testing.T) {
	var records []run.Record
	for _, x := range population().Samples {
		records = append(records, rec(x.ID, "ai-rulez", x.Malicious, "high"))
	}
	v := map[string]run.Record{}
	for _, r := range records {
		v[r.ID] = r
	}
	a, _, _ := bootstrap(protocol(), population(), v)
	b, _, _ := bootstrap(protocol(), population(), v)
	if a != b {
		t.Fatalf("a fixed seed must repeat: %+v %+v", a, b)
	}
	if a.Low != 1 || a.High != 1 {
		t.Fatalf("perfect recall stays perfect: %+v", a)
	}
}

func TestMcNemar(t *testing.T) {
	for _, tc := range []struct {
		b, c int
		want float64
	}{{0, 0, 1}, {1, 9, 22.0 / 1024}, {5, 5, 1}, {0, 10, 2.0 / 1024}} {
		if got := mcnemar(tc.b, tc.c); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("mcnemar(%d, %d) = %v, want %v", tc.b, tc.c, got, tc.want)
		}
	}
}

func TestPaired(t *testing.T) {
	var records []run.Record
	for _, x := range population().Samples {
		records = append(records, rec(x.ID, "ai-rulez", x.Malicious, "high"), rec(x.ID, "skillspector", false, ""))
	}
	s, err := Compute(protocol(), population(), []Run{{Records: records}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p := s.Paired[0]; p.OnlyA != 4 || p.OnlyB != 0 {
		t.Fatalf("paired = %+v", p)
	}
}
