package lint

import (
	"fmt"
	"path"
	"strings"
	"testing"
)

// numberedPrefixTree is a tree of n files, among them the numbered ADRs.
func numberedPrefixTree(n int) *Tree {
	t := &Tree{files: map[string]uint32{"adrs/0065-license.md": 0, "adrs/0070.md": 0, "docs/adrs/0001-first.md": 0}}
	for i := range n {
		t.files[fmt.Sprintf("src/pkg%d/file%d.go", i%50, i)] = 0
	}
	return t
}

// numberedPrefixScan is the per-call scan of every path that HasPathPrefix replaces.
func numberedPrefixScan(t *Tree, baseRel, rel string) bool {
	dir, base := path.Split(strings.TrimSuffix(rel, "/"))
	roots := []string{""}
	if baseRel != "" {
		roots = append(roots, baseRel+"/")
	}
	for _, p := range t.Paths() {
		for _, root := range roots {
			if strings.HasPrefix(p, root+dir+base+"-") || strings.HasPrefix(p, root+dir+base+".") {
				return true
			}
		}
	}
	return false
}

func TestNumberedPrefixExists_MatchesTheFullScan(t *testing.T) {
	tests := []struct {
		rel, baseRel string
	}{
		{"adrs/0065", ""}, {"adrs/0070", ""}, {"adrs/0099", ""}, {"adrs/0001", "docs"},
		{"adrs/0001", ""}, {"adrs/006", ""}, {"adrs/0065/", ""}, {"0065", ""},
	}
	for _, tt := range tests {
		t.Run(tt.rel+"@"+tt.baseRel, func(t *testing.T) {
			tree := numberedPrefixTree(200)
			r := &runner{tree: tree, baseRel: tt.baseRel}
			if got, want := r.numberedPrefixExists(tt.rel), numberedPrefixScan(tree, tt.baseRel, tt.rel); got != want {
				t.Errorf("numberedPrefixExists(%q) = %v, full scan says %v", tt.rel, got, want)
			}
		})
	}
}

func BenchmarkNumberedPrefixExists(b *testing.B) {
	tree := numberedPrefixTree(20000)
	r := &runner{tree: tree, baseRel: "docs"}
	b.ReportAllocs()
	for range b.N {
		for i := range 100 {
			r.numberedPrefixExists(fmt.Sprintf("adrs/%04d", i))
		}
	}
}
