package generator

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const benchSecretMCP = `
[[mcp_servers]]
name = "tracker"
command = "npx"
args = ["-y", "tracker-mcp", "--api-key", "sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789"]
`

func benchGenerator(b *testing.B, files int, extra string) (*Generator, string) {
	b.Helper()
	root := testutil.BuildBenchTree(b, files, testutil.BenchTreeOptions{Git: true, ExtraConfig: extra})
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		b.Fatal(err)
	}
	return NewGenerator(cfg), root
}

// BenchmarkGenerate times a full generate into an empty tree and the idempotent
// second run that writes nothing.
func BenchmarkGenerate(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		b.Run("full/"+s.Name, func(b *testing.B) {
			b.ReportAllocs()
			gen, _ := benchGenerator(b, s.Files, "")
			for range b.N {
				b.StopTimer()
				if _, err := gen.Clean("", CleanOptions{RemoveEdited: true}); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if err := gen.Generate(""); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("noop/"+s.Name, func(b *testing.B) {
			b.ReportAllocs()
			gen, _ := benchGenerator(b, s.Files, "")
			if err := gen.Generate(""); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for range b.N {
				if err := gen.Generate(""); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkClean times removing the generated outputs (and the dry-run plan).
func BenchmarkClean(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		b.Run("remove/"+s.Name, func(b *testing.B) {
			b.ReportAllocs()
			gen, _ := benchGenerator(b, s.Files, "")
			for range b.N {
				b.StopTimer()
				if err := gen.Generate(""); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if _, err := gen.Clean("", CleanOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("dryrun/"+s.Name, func(b *testing.B) {
			b.ReportAllocs()
			gen, _ := benchGenerator(b, s.Files, "")
			if err := gen.Generate(""); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for range b.N {
				if _, err := gen.Clean("", CleanOptions{DryRun: true}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCheckDrift compares the rendered outputs with the ones on disk.
func BenchmarkCheckDrift(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		gen, _ := benchGenerator(b, s.Files, "")
		if err := gen.Generate(""); err != nil {
			b.Fatal(err)
		}
		b.Run(s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := gen.CheckDrift(""); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSecretGate times the "MCP config holding a secret must be gitignored"
// check over the outputs of a project that declares a secret MCP server.
func BenchmarkSecretGate(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		gen, _ := benchGenerator(b, s.Files, benchSecretMCP)
		plan, err := gen.Plan("")
		if err != nil {
			b.Skipf("plan: %v", err)
		}
		outputs := plan.Outputs
		b.Run(s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				gen.unignoredSecretOutputs(outputs)
			}
		})
	}
}

// BenchmarkGenerateLLMsTxt generates the llms.txt delivery (index and full text).
func BenchmarkGenerateLLMsTxt(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		b.Run(s.Name, func(b *testing.B) {
			root := testutil.BuildBenchTree(b, s.Files, testutil.BenchTreeOptions{
				Git: true, Presets: []string{"claude", "llms-txt"}, ExtraConfig: "\n[llms_txt]\ndir = \"docs\"\n",
			})
			cfg, err := config.LoadConfig(context.Background(), root, config.WithoutLocal(), config.WithoutRemote())
			if err != nil {
				b.Fatal(err)
			}
			gen := NewGenerator(cfg)
			if err := gen.Generate(""); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := gen.Generate(""); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
