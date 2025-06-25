package generator_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
)

func BenchmarkGenerateAll(b *testing.B) {
	// Create a test config
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
		},
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: 10, Content: "Content 1"},
			{Name: "Rule 2", Priority: 5, Content: "Content 2"},
			{Name: "Rule 3", Priority: 1, Content: "Content 3"},
		},
		Sections: []config.Section{
			{Title: "Section 1", Priority: 20, Content: "# Section Content\nThis is a section."},
			{Title: "Section 2", Priority: 15, Content: "## Another Section\nMore content here."},
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

func BenchmarkGenerateAllParallel(b *testing.B) {
	// Create a test config
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
			{Name: "Rule 4", Priority: 8, Content: "Content 4"},
			{Name: "Rule 5", Priority: 3, Content: "Content 5"},
		},
	}

	tempDir := b.TempDir()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Each goroutine gets its own subdirectory
			subDir := filepath.Join(tempDir, filepath.Base(b.TempDir()))
			_ = os.MkdirAll(subDir, 0755)

			gen := generator.NewWithBaseDir(subDir)
			if err := gen.GenerateAll(cfg); err != nil {
				b.Fatalf("Failed to generate: %v", err)
			}
		}
	})
}

func BenchmarkTemplateRendering(b *testing.B) {
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:    "Template Benchmark",
			Version: "1.0.0",
		},
		Outputs: []config.Output{
			{File: "test.md"},
		},
		Rules: make([]config.Rule, 100), // 100 rules to make template rendering more significant
	}

	// Fill rules
	for i := 0; i < 100; i++ {
		cfg.Rules[i] = config.Rule{
			Name:     fmt.Sprintf("Rule %d", i),
			Priority: i % 10,
			Content:  fmt.Sprintf("This is the content for rule %d with some longer text to make it more realistic", i),
		}
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

func BenchmarkIncrementalGeneration(b *testing.B) {
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Incremental Test",
		},
		Outputs: []config.Output{
			{File: "output.md"},
		},
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: 1, Content: "Content 1"},
		},
	}

	tempDir := b.TempDir()
	gen := generator.NewWithBaseDir(tempDir)

	// Generate once to create the file
	if err := gen.GenerateAll(cfg); err != nil {
		b.Fatalf("Initial generation failed: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Benchmark incremental generation (should skip unchanged files)
	for i := 0; i < b.N; i++ {
		if err := gen.GenerateAll(cfg); err != nil {
			b.Fatalf("Failed to generate: %v", err)
		}
	}
}

func BenchmarkGenerateAllConcurrent(b *testing.B) {
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Concurrent Benchmark",
			Version:     "1.0.0",
			Description: "Test concurrent generation",
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

// Removed - can't directly test unexported functions from _test package
