package generator_test

import (
	"fmt"
	"testing"

	"github.com/Goldziher/airules/internal/config"
	"github.com/Goldziher/airules/internal/generator"
)

func BenchmarkGenerateAllLarge(b *testing.B) {
	// Create a config with many outputs
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Large Benchmark Project",
			Version:     "1.0.0",
			Description: "Test project with many outputs",
		},
		Outputs: make([]config.Output, 50),
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: 10, Content: "Content 1"},
			{Name: "Rule 2", Priority: 5, Content: "Content 2"},
			{Name: "Rule 3", Priority: 1, Content: "Content 3"},
		},
	}

	// Fill outputs
	for i := 0; i < 50; i++ {
		cfg.Outputs[i] = config.Output{File: fmt.Sprintf("output%d.md", i)}
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

func BenchmarkGenerateAllLargeSerial(b *testing.B) {
	// Create a config with many outputs
	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Large Benchmark Project",
			Version:     "1.0.0",
			Description: "Test project with many outputs",
		},
		Outputs: make([]config.Output, 50),
		Rules: []config.Rule{
			{Name: "Rule 1", Priority: 10, Content: "Content 1"},
			{Name: "Rule 2", Priority: 5, Content: "Content 2"},
			{Name: "Rule 3", Priority: 1, Content: "Content 3"},
		},
	}

	// Fill outputs
	for i := 0; i < 50; i++ {
		cfg.Outputs[i] = config.Output{File: fmt.Sprintf("output%d.md", i)}
	}

	// Force serial generation by temporarily setting outputs to less than threshold
	originalOutputs := cfg.Outputs
	cfg.Outputs = cfg.Outputs[:9] // Force serial mode

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Restore outputs for actual generation
		cfg.Outputs = originalOutputs

		// Manually call serial generation (simulated)
		// We'll create a fresh generator to avoid the concurrent path
		freshGen := generator.NewWithBaseDir(b.TempDir())
		for j := 0; j < len(cfg.Outputs); j++ {
			if err := freshGen.GenerateOutput(cfg, cfg.Outputs[j].File); err != nil {
				b.Fatalf("Failed to generate output %d: %v", j, err)
			}
		}
	}
}
