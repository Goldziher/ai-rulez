package repoindex

import (
	"context"
	"fmt"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// monorepoShapes are the file and scope counts the index benchmarks cover. A
// shape needs at least three files per scope for the configuration files.
func monorepoShapes() []testutil.MonorepoSpec {
	var out []testutil.MonorepoSpec
	for _, files := range testutil.MonorepoSizes() {
		for _, scopes := range []int{10, 100, 500} {
			if files >= 4*scopes {
				out = append(out, testutil.MonorepoSpec{Files: files, Scopes: scopes})
			}
		}
	}
	return out
}

func shapeName(s testutil.MonorepoSpec) string {
	return fmt.Sprintf("files=%d/scopes=%d", s.Files, s.Scopes)
}

// BenchmarkBuild times a cold index build on a synthetic monorepo. The git
// variant lists files with `git ls-files`; the walk variant has no repository.
// Each shape's tree lives only while its sub-benchmarks run.
func BenchmarkBuild(b *testing.B) {
	for _, shape := range monorepoShapes() {
		b.Run(shapeName(shape), func(b *testing.B) {
			for _, mode := range []struct {
				name string
				git  bool
			}{{"git", true}, {"walk", false}} {
				spec := shape
				spec.Git = mode.git
				root := testutil.BuildMonorepo(b, spec)
				b.Run(mode.name, func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						if _, err := Build(context.Background(), Options{Root: root, Presets: []string{"claude"}}); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// BenchmarkBuildPreListed times Build when the caller supplies the file
// listing, which leaves out the listing and measures classification, reading
// and grouping alone.
func BenchmarkBuildPreListed(b *testing.B) {
	for _, shape := range monorepoShapes() {
		b.Run(shapeName(shape), func(b *testing.B) {
			shape.Git = true
			root := testutil.BuildMonorepo(b, shape)
			files := listFiles(context.Background(), root)
			b.ResetTimer()
			b.ReportAllocs()
			for range b.N {
				if _, err := Build(context.Background(), Options{Root: root, Presets: []string{"claude"}, Files: files}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkListFiles times the file listing alone on the largest tree.
func BenchmarkListFiles(b *testing.B) {
	for _, mode := range []struct {
		name string
		git  bool
	}{{"git", true}, {"walk", false}} {
		b.Run(mode.name, func(b *testing.B) {
			root := testutil.BuildMonorepo(b, testutil.MonorepoSpec{Files: testutil.MonorepoLargest(), Scopes: 100, Git: mode.git})
			b.ResetTimer()
			b.ReportAllocs()
			for range b.N {
				if len(listFiles(context.Background(), root)) == 0 {
					b.Fatal("no files listed")
				}
			}
		})
	}
}

// BenchmarkResolve times Resolve for a deep path on the largest index.
func BenchmarkResolve(b *testing.B) {
	for _, scopes := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("scopes=%d", scopes), func(b *testing.B) {
			root := testutil.BuildMonorepo(b, testutil.MonorepoSpec{Files: testutil.MonorepoLargest(), Scopes: scopes, Git: true})
			ix, err := Build(context.Background(), Options{Root: root, Presets: []string{"claude"}})
			if err != nil {
				b.Fatal(err)
			}
			deep := testutil.MonorepoScopeDir(scopes-1) + "/src/pkg001/a/b/c/file.go"
			b.ResetTimer()
			b.ReportAllocs()
			for range b.N {
				if _, err := ix.Resolve(deep); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkWithin times listing a scope and its descendants on the largest
// index with 500 scopes.
func BenchmarkWithin(b *testing.B) {
	root := testutil.BuildMonorepo(b, testutil.MonorepoSpec{Files: testutil.MonorepoLargest(), Scopes: 500, Git: true})
	ix, err := Build(context.Background(), Options{Root: root, Presets: []string{"claude"}})
	if err != nil {
		b.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		scope     string
		recursive bool
	}{{"leaf", testutil.MonorepoScopeDir(7), false}, {"root-recursive", ".", true}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := ix.Within(tc.scope, tc.recursive); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
