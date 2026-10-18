package lint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// stagedSkillsTree writes a project of n skills with three supporting files
// each, without a git repository.
func stagedSkillsTree(tb testing.TB, n int) string {
	tb.Helper()
	root := tb.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil { //nolint:gosec // benchmark fixture
			tb.Fatal(err)
		}
	}
	write(".ai-rulez/config.toml", baseConfig)
	body := testutil.BenchBody(20)
	for i := range n {
		dir := fmt.Sprintf(".ai-rulez/skills/skill-%04d", i)
		write(dir+"/SKILL.md", fmt.Sprintf("---\nname: skill-%04d\ndescription: Use when staging skill %d for a scanner.\n---\n# Skill %d\n%s", i, i, i, body))
		write(dir+"/references/guide.md", "# Guide\n"+body)
		write(dir+"/scripts/run.sh", "#!/bin/sh\necho run\n")
		write(dir+"/assets/template.txt", body)
	}
	return root
}

// BenchmarkScannerStage times what an external scanner run needs before it
// starts anything: listing the files staged for its inputs, digesting them and
// computing the cache key.
func BenchmarkScannerStage(b *testing.B) {
	bin, err := os.Executable()
	if err != nil {
		b.Fatal(err)
	}
	for _, n := range []int{50, 500} {
		root := stagedSkillsTree(b, n)
		cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
		if err != nil {
			b.Fatal(err)
		}
		tree, err := LoadTree(root)
		if err != nil {
			b.Fatal(err)
		}
		r := &runner{cfg: cfg, tree: tree, docs: map[string]doc{}}
		r.collect()
		sc := resolvedScanner{LintExternal: config.LintExternal{Name: "bench", Command: []string{bin, "{stage}"}, Inputs: []string{inputSkills}}}
		b.Run(fmt.Sprintf("skills=%d", n), func(b *testing.B) {
			b.Run("stage", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if len(r.stageFilesFor(sc)) == 0 {
						b.Fatal("nothing staged")
					}
				}
			})
			files := r.stageFilesFor(sc)
			b.Run("digest", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					_ = digestStage(files)
				}
			})
			digest := digestStage(files)
			b.Run("key", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if _, ok := scanKeyFor(sc, bin, digest, nil, false, "none/"); !ok {
						b.Fatal("no key")
					}
				}
			})
		})
	}
}

// BenchmarkScannerStageChanged compares listing the staged files of a per-skill
// scanner for a full run with the listing of a --changed run that touched one skill.
func BenchmarkScannerStageChanged(b *testing.B) {
	bin, err := os.Executable()
	if err != nil {
		b.Fatal(err)
	}
	root := stagedSkillsTree(b, 300)
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutRemote())
	if err != nil {
		b.Fatal(err)
	}
	tree, err := LoadTree(root)
	if err != nil {
		b.Fatal(err)
	}
	opts := Options{Scanner: ScannerOptions{ChangedFiles: []string{".ai-rulez/skills/skill-0007/references/guide.md"}}}
	r := &runner{cfg: cfg, tree: tree, docs: map[string]doc{}, opts: opts}
	r.collect()
	sc := resolvedScanner{LintExternal: config.LintExternal{Name: "bench", Command: []string{bin, "{skill_dirs}"}, Inputs: []string{inputSkills}}}
	b.Run("full", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			r.skillFiles.Clear()
			_ = r.stageFilesFor(sc)
		}
	})
	b.Run("changed", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			r.skillFiles.Clear()
			_ = r.stageFilesToScan(sc)
		}
	})
}
