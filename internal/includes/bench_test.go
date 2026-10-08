package includes

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// BenchmarkLoadWithLocalInclude loads a project that merges a local include
// through the include resolver.
func BenchmarkLoadWithLocalInclude(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		root := testutil.BuildBenchTree(b, s.Files, testutil.BenchTreeOptions{Include: true})
		b.Run(s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_, err := config.LoadConfig(context.Background(), root,
					config.WithoutLocal(), config.WithoutRemote(), config.WithResolvers(Resolvers("", nil)))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
