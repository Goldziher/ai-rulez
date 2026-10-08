package tokens_test

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

func BenchmarkEstimate(b *testing.B) {
	text := strings.Repeat(testutil.BenchBody(40), 20)
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	for range b.N {
		_ = tokens.Estimate(text)
	}
}

func BenchmarkCount(b *testing.B) {
	prose := testutil.BenchBody(40)
	cases := []struct{ name, text string }{
		{"prose-small", prose},
		{"prose-large", strings.Repeat(prose, 50)},
		{"long-line", strings.Repeat("0123456789abcdef", 8192)},
		{"long-line-unicode", strings.Repeat("\u200b\u202e", 20000)},
	}
	counters := map[string]tokens.Counter{"cl100k": tokens.CL100KBase(), "ratio": tokens.ByteRatio(tokens.EstimateBytesPerToken)}
	for _, cn := range []string{"cl100k", "ratio"} {
		for _, c := range cases {
			b.Run(cn+"/"+c.name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(c.text)))
				for range b.N {
					_ = counters[cn].Count(c.text)
				}
			})
		}
	}
}
