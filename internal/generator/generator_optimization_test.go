package generator_test

import (
	"testing"

	"github.com/Goldziher/airules/internal/config"
	"github.com/Goldziher/airules/internal/generator"
)

func BenchmarkGenerateAllOriginal(b *testing.B) {
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Benchmark Project",
			Version:     "1.0.0",
			Description: "Test project for benchmarking",
		},
		Outputs: []config.Output{
			{File: "output1.md"},
			{File: "output2.md", Template: "documentation"},
			{File: "output3.md"},
			{File: "output4.md"},
			{File: "output5.md"},
		},
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: 10, Content: "Content 1"},
			{Name: "Rule 2", Priority: 5, Content: "Content 2"},
			{Name: "Rule 3", Priority: 1, Content: "Content 3"},
		},
	}

	tempDir := b.TempDir()
	gen := generator.NewWithBaseDir(tempDir)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := gen.GenerateAll(cfg); err != nil {
			b.Fatalf("Failed to generate: %v", err)
		}
	}
}

func BenchmarkGenerateAllOptimized(b *testing.B) {
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Benchmark Project",
			Version:     "1.0.0",
			Description: "Test project for benchmarking",
		},
		Outputs: []config.Output{
			{File: "output1.md"},
			{File: "output2.md", Template: "documentation"},
			{File: "output3.md"},
			{File: "output4.md"},
			{File: "output5.md"},
		},
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: 10, Content: "Content 1"},
			{Name: "Rule 2", Priority: 5, Content: "Content 2"},
			{Name: "Rule 3", Priority: 1, Content: "Content 3"},
		},
	}

	tempDir := b.TempDir()
	gen := generator.NewWithBaseDir(tempDir)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := gen.GenerateAllConcurrent(cfg); err != nil {
			b.Fatalf("Failed to generate: %v", err)
		}
	}
}
func BenchmarkComputeContentHashPooled(b *testing.B) {
	content := "This is a test string that will be hashed repeatedly during the benchmark"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = generator.ComputeContentHashPooled(content)
	}
}
