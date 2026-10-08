package review

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// BenchmarkScore collects the items of a synthetic project and scores them
// offline against the built-in rubric from lint evidence.
func BenchmarkScore(b *testing.B) {
	rb := builtin(b)
	for _, s := range testutil.BenchSizes() {
		root := testutil.BuildBenchTree(b, s.Files, testutil.BenchTreeOptions{Git: true})
		cfg, err := config.LoadConfig(context.Background(), root, config.WithoutLocal(), config.WithoutRemote())
		if err != nil {
			b.Fatal(err)
		}
		tree, err := lint.LoadTree(root)
		if err != nil {
			b.Fatal(err)
		}
		rep, err := lint.Run(cfg, tree)
		if err != nil {
			b.Fatal(err)
		}
		rel := func(abs string) string { r, _ := filepath.Rel(root, abs); return filepath.ToSlash(r) } //nolint:errcheck // below root
		b.Run("Collect/"+s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_ = Collect(cfg, rel)
			}
		})
		items := Collect(cfg, rel)
		b.Run("Run/"+s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_ = Run(Input{Rubric: rb, Items: items, Findings: rep.Findings})
			}
		})
	}
}

// BenchmarkCalibrate measures a scripted judge against ten golden cases with
// every vote asked, plus the metamorphic probes.
func BenchmarkCalibrate(b *testing.B) {
	cases := tenCases()
	rb := builtin(b)
	rbc := *rb
	rbc.Calibration.GoldenMinItems = 8
	set, err := LoadGolden(writeGolden(b, cases), &rbc)
	if err != nil {
		b.Fatal(err)
	}
	sj := truthJudge(cases)
	b.ReportAllocs()
	for range b.N {
		client, _ := newClient(b, sj, llm.Config{})
		_, err := Calibrate(context.Background(), CalibrateInput{
			Rubric: &rbc, Golden: set, Now: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
			Options: SemanticOptions{K: 3, Client: client},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}
