package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// writeSkillBundle writes a skill directory of files supporting files below dir:
// references, scripts and assets, plus documents beside SKILL.md and in a
// folder of their own, each a few hundred bytes.
func writeSkillBundle(tb testing.TB, dir string, files int) {
	tb.Helper()
	write := func(rel, body string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil { //nolint:gosec // benchmark fixture
			tb.Fatal(err)
		}
	}
	write("SKILL.md", "---\nname: bundle\ndescription: Use when exercising the bundle loader.\n---\n# Bundle\n")
	body := testutil.BenchBody(8)
	dirs := []string{"references", "scripts", "assets", "docs", "."}
	for i := range files {
		write(fmt.Sprintf("%s/file-%04d.md", dirs[i%len(dirs)], i), fmt.Sprintf("---\ndescription: Document %d of the bundle.\n---\n# Document %d\n%s", i, i, body))
	}
}

// BenchmarkLoadSkillResourcesContext loads the whole bundle of one installed
// skill with 1, 100 and 2000 supporting files.
func BenchmarkLoadSkillResourcesContext(b *testing.B) {
	for _, n := range []int{1, 100, 2000} {
		dir := b.TempDir()
		writeSkillBundle(b, dir, n)
		b.Run(fmt.Sprintf("files=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				res, err := LoadSkillResourcesContext(context.Background(), dir)
				if err != nil {
					b.Fatal(err)
				}
				if len(res) == 0 {
					b.Fatal("no resources loaded")
				}
			}
		})
	}
}

// BenchmarkLoadSkillResourcesCatalog loads 200 installed skills of 10
// supporting files each, one after the other as an include of that size would.
func BenchmarkLoadSkillResourcesCatalog(b *testing.B) {
	const skills, files = 200, 10
	root := b.TempDir()
	dirs := make([]string, skills)
	for i := range skills {
		dirs[i] = filepath.Join(root, fmt.Sprintf("skill-%03d", i))
		writeSkillBundle(b, dirs[i], files)
	}
	b.ReportAllocs()
	for range b.N {
		for _, dir := range dirs {
			if _, err := LoadSkillResourcesContext(context.Background(), dir); err != nil {
				b.Fatal(err)
			}
		}
	}
}
