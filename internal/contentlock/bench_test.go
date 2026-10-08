package contentlock

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func benchConfig(b *testing.B, files int) *config.Config {
	b.Helper()
	root := testutil.BuildBenchTree(b, files, testutil.BenchTreeOptions{Git: true})
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		b.Fatal(err)
	}
	return cfg
}

// BenchmarkCompute digests the authored content (the work behind `lock`).
func BenchmarkCompute(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		cfg := benchConfig(b, s.Files)
		b.Run(s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := Compute(cfg, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkBuildAndCompare builds a lock from a snapshot and verifies it
// against a fresh snapshot (the work behind `lock --check`).
func BenchmarkBuildAndCompare(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		cfg := benchConfig(b, s.Files)
		snap, err := Compute(cfg, Options{})
		if err != nil {
			b.Fatal(err)
		}
		b.Run("Build/"+s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				var f lockfile.File
				Build(&f, snap)
			}
		})
		var lock lockfile.File
		Build(&lock, snap)
		b.Run("Compare/"+s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if d := Compare(&lock, snap); !d.InSync {
					b.Fatalf("unexpected drift: %d changes", len(d.Changes))
				}
			}
		})
	}
}
