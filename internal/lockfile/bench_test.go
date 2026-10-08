package lockfile

import (
	"fmt"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func benchFile(items int) *File {
	f := &File{Version: 1, Tree: "sha256:" + fmt.Sprintf("%064d", 0)}
	for i := range items {
		f.Item = append(f.Item, Item{Kind: "rule", ID: fmt.Sprintf("rule-%05d", i), Path: fmt.Sprintf(".ai-rulez/rules/rule-%05d.md", i), Digest: fmt.Sprintf("sha256:%064d", i)})
	}
	return f
}

// BenchmarkSaveLoad round-trips a lock file of each size through disk.
func BenchmarkSaveLoad(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		f := benchFile(s.Files)
		dir := b.TempDir()
		b.Run("Save/"+s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if err := Save(dir, f); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("Load/"+s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := Load(dir); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
