package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func benchSession(b *testing.B, srv *Server) *sdkmcp.ClientSession {
	b.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	b.Cleanup(cancel)
	serverT, clientT := sdkmcp.NewInMemoryTransports()
	go func() { _ = srv.GetMCPServer().Run(ctx, serverT) }()
	session, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "bench", Version: "1"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = session.Close() })
	return session
}

func benchCall(b *testing.B, s *sdkmcp.ClientSession, tool string, args map[string]any) {
	b.Helper()
	res, err := s.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil || res.IsError {
		b.Fatalf("%s: err=%v result=%+v", tool, err, res)
	}
}

// BenchmarkProjectTools dispatches the project (CRUD) tools over an in-memory
// transport against a synthetic project.
func BenchmarkProjectTools(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		root := testutil.BuildBenchTree(b, s.Files, testutil.BenchTreeOptions{})
		session := benchSession(b, NewServer("bench", WithAnyDirectory()))
		for _, tc := range []struct {
			tool string
			args map[string]any
		}{
			{"list_rules", map[string]any{"working_directory": root}},
			{"read_rule", map[string]any{"name": "rule-0000", "working_directory": root}},
			{"list_skills", map[string]any{"working_directory": root}},
		} {
			b.Run(tc.tool+"/"+s.Name, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					benchCall(b, session, tc.tool, tc.args)
				}
			})
		}
	}
}

// benchSkillServer is a served-skills server whose load_skill session budget
// is unlimited: the benchmark loads one skill more often than the default 256 KiB
// budget allows, after which every call is refused.
func benchSkillServer(cat *Catalog) *Server {
	return NewSkillServerWith("bench", cat, ServeOptions{BudgetBytes: -1})
}

// BenchmarkSkillServer dispatches the served-skills tools over a catalog of each size.
func BenchmarkSkillServer(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		served := make([]generator.ServedSkill, 0, s.Files)
		for i := range s.Files {
			served = append(served, servedSkill(fmt.Sprintf("skill-%04d", i), "",
				fmt.Sprintf("Handle synthetic benchmark task number %d for the billing service", i), []string{"billing", fmt.Sprintf("k%d", i%17)}))
		}
		cat, err := BuildCatalog("p", "claude", served, SkillFilter{})
		if err != nil {
			b.Fatal(err)
		}
		session := benchSession(b, benchSkillServer(cat))
		b.Run("BuildCatalog/"+s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := BuildCatalog("p", "claude", served, SkillFilter{}); err != nil {
					b.Fatal(err)
				}
			}
		})
		for _, tc := range []struct {
			tool string
			args map[string]any
		}{
			{"search_skills", map[string]any{"query": "billing task number 7"}},
			{"load_skill", map[string]any{"name": "skill-0000"}},
		} {
			b.Run(tc.tool+"/"+s.Name, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					benchCall(b, session, tc.tool, tc.args)
				}
			})
		}
	}
}

const benchDiscoverConfig = "version = \"5.0\"\nname = \"bench\"\ngitignore = false\npresets = [\"claude\"]\n"

// benchMonorepo writes the monorepo of the given scope count with an
// ai-rulez configuration, staged in git.
func benchMonorepo(b *testing.B, files, scopes int) string {
	b.Helper()
	return testutil.BuildMonorepo(b, testutil.MonorepoSpec{
		Files: files, Scopes: scopes, Git: true,
		Extra: map[string]string{".ai-rulez/config.toml": benchDiscoverConfig},
	})
}

func benchServer(b *testing.B, root string) *Server {
	b.Helper()
	setup := &ServeSetup{WorkDir: root, DiscoverSkills: "repo", NoWatch: true, CacheDir: b.TempDir()}
	srv, err := setup.NewServer(context.Background())
	if err != nil {
		b.Fatal(err)
	}
	return srv
}

// BenchmarkDiscoveryCatalog times building the served catalog of a monorepo
// with repository discovery on: the config load, the repository index and one
// served skill per discovered directory (up to the discovery cap).
func BenchmarkDiscoveryCatalog(b *testing.B) {
	for _, scopes := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("scopes=%d", scopes), func(b *testing.B) {
			root := benchMonorepo(b, testutil.MonorepoLargest(), scopes)
			b.ResetTimer()
			b.ReportAllocs()
			for range b.N {
				benchServer(b, root)
			}
		})
	}
}

// ageBenchTree puts every file's modification time an hour back, so the index
// cache trusts it (a file changed within two seconds of a save is read again).
func ageBenchTree(b *testing.B, root string) {
	b.Helper()
	old := time.Now().Add(-time.Hour)
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return os.Chtimes(p, old, old)
	})
	if err != nil {
		b.Fatal(err)
	}
}

// BenchmarkDiscoveryCatalogCache compares a start without the index cache, a
// cold start that fills it, and a warm start that reuses it, on the largest
// monorepo.
func BenchmarkDiscoveryCatalogCache(b *testing.B) {
	for _, scopes := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("scopes=%d", scopes), func(b *testing.B) {
			root := benchMonorepo(b, testutil.MonorepoLargest(), scopes)
			ageBenchTree(b, root)
			configPath := filepath.Join(root, ".ai-rulez", "config.toml")
			on := benchDiscoverConfig
			off := benchDiscoverConfig + "\n[mcp]\nindex_cache = \"off\"\n"
			start := func(b *testing.B, cacheDir string) {
				setup := &ServeSetup{WorkDir: root, DiscoverSkills: "repo", NoWatch: true, CacheDir: cacheDir}
				if _, err := setup.NewServer(context.Background()); err != nil {
					b.Fatal(err)
				}
			}
			b.Run("off", func(b *testing.B) {
				if err := os.WriteFile(configPath, []byte(off), 0o600); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				for range b.N {
					start(b, b.TempDir())
				}
			})
			if err := os.WriteFile(configPath, []byte(on), 0o600); err != nil {
				b.Fatal(err)
			}
			b.Run("cold", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					start(b, b.TempDir())
				}
			})
			b.Run("warm", func(b *testing.B) {
				cacheDir := b.TempDir()
				start(b, cacheDir)
				b.ResetTimer()
				b.ReportAllocs()
				for range b.N {
					start(b, cacheDir)
				}
			})
		})
	}
}

// BenchmarkRepositoryQueries times find_skill and find_context over the
// in-memory transport against the largest monorepo, once per scope count.
func BenchmarkRepositoryQueries(b *testing.B) {
	for _, scopes := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("scopes=%d", scopes), func(b *testing.B) {
			root := benchMonorepo(b, testutil.MonorepoLargest(), scopes)
			session := benchSession(b, benchServer(b, root))
			deep := testutil.MonorepoScopeDir(scopes-1) + "/src/pkg001/file.go"
			for _, tc := range []struct {
				name string
				tool string
				args map[string]any
			}{
				{"find_skill/task", "find_skill", map[string]any{"task": "work in a service of the monorepo"}},
				{"find_skill/path", "find_skill", map[string]any{"task": "work in a service of the monorepo", "path": deep}},
				{"find_skill/scope", "find_skill", map[string]any{"task": "synthetic monorepo", "scope": testutil.MonorepoScopeDir(1), "recursive": true}},
				{"find_context/path", "find_context", map[string]any{"path": deep}},
				{"find_context/task", "find_context", map[string]any{"task": "conventions of the service", "path": deep}},
				{"find_context/root-recursive", "find_context", map[string]any{"scope": ".", "recursive": true}},
			} {
				b.Run(tc.name, func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						benchCall(b, session, tc.tool, tc.args)
					}
				})
			}
		})
	}
}
