package config

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// BenchmarkLoadConfig loads a synthetic project tree (rules, context, skills,
// agents, commands, frontmatter on every file) of each size.
func BenchmarkLoadConfig(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		root := testutil.BuildBenchTree(b, s.Files, testutil.BenchTreeOptions{})
		b.Run(s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := LoadConfig(context.Background(), root, WithoutLocal(), WithoutRemote()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
