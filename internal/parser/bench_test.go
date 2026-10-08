package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func BenchmarkParseFrontmatter(b *testing.B) {
	body := testutil.BenchBody(40)
	cases := map[string]string{
		"none":     body,
		"small":    "---\npriority: high\n---\n" + body,
		"rich":     "---\npriority: high\ntargets: [claude, cursor]\ntools: [Read, Grep]\nskills: [a, b]\nkeywords: [x, y, z]\nextra: value\n---\n" + body,
		"crlf":     strings.ReplaceAll("---\npriority: high\n---\n"+body, "\n", "\r\n"),
		"longbody": "---\npriority: high\n---\n" + strings.Repeat(body, 50),
	}
	for _, name := range []string{"none", "small", "rich", "crlf", "longbody"} {
		content := cases[name]
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(content)))
			for range b.N {
				if _, _, err := ParseFrontmatter(content); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkParseFrontmatterBatch(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		docs := make([]string, s.Files)
		for i := range docs {
			docs[i] = fmt.Sprintf("---\npriority: medium\nname: n%d\n---\n%s", i, testutil.BenchBody(20))
		}
		b.Run(s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				for _, d := range docs {
					if _, _, err := ParseFrontmatter(d); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
