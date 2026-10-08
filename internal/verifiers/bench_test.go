package verifiers

import (
	"fmt"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// BenchmarkRegexFiles selects the files of an any-file regex predicate from a
// tree of each size, with exclude globs on the spec.
func BenchmarkRegexFiles(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		tree := make([]string, 0, s.Files*5)
		for i := range s.Files * 5 {
			tree = append(tree, fmt.Sprintf("pkg%03d/sub/file%05d.go", i%97, i))
		}
		c := &evalCtx{
			env:  &Env{scope: &scopeData{tree: tree}},
			spec: &Spec{Exclude: []string{"**/*_test.go", "vendor/**", "**/generated/**"}},
		}
		p := &RegexPred{Regex: "TODO", In: inAnyFile, Files: "**/*.go"}
		b.Run(s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := c.regexFiles(p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
