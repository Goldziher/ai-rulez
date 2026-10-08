package mcp

import (
	"context"
	"fmt"
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
		session := benchSession(b, NewServer("bench"))
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
		session := benchSession(b, NewSkillServer("bench", cat))
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
