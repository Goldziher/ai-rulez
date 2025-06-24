package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func BenchmarkBasicGeneration(b *testing.B) {
	// Build binary first
	if err := buildBinary(); err != nil {
		b.Fatalf("Failed to build binary: %v", err)
	}
	defer cleanupBinary()
	
	// Create temp directory and copy scenarios
	tempDir := b.TempDir()
	if err := copyTestScenariosForBench(b, tempDir); err != nil {
		b.Fatalf("Failed to copy scenarios: %v", err)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(binaryPath, "generate", "scenarios/basic/ai_rules.yaml")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("Command failed: %v\nOutput: %s", err, output)
		}
	}
}

func BenchmarkDryRunGeneration(b *testing.B) {
	// Build binary first
	if err := buildBinary(); err != nil {
		b.Fatalf("Failed to build binary: %v", err)
	}
	defer cleanupBinary()
	
	// Create temp directory and copy scenarios
	tempDir := b.TempDir()
	if err := copyTestScenariosForBench(b, tempDir); err != nil {
		b.Fatalf("Failed to copy scenarios: %v", err)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(binaryPath, "generate", "scenarios/basic/ai_rules.yaml", "--dry-run")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("Command failed: %v\nOutput: %s", err, output)
		}
	}
}

func BenchmarkValidation(b *testing.B) {
	// Build binary first
	if err := buildBinary(); err != nil {
		b.Fatalf("Failed to build binary: %v", err)
	}
	defer cleanupBinary()
	
	// Create temp directory and copy scenarios
	tempDir := b.TempDir()
	if err := copyTestScenariosForBench(b, tempDir); err != nil {
		b.Fatalf("Failed to copy scenarios: %v", err)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(binaryPath, "validate", "scenarios/basic/ai_rules.yaml")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("Command failed: %v\nOutput: %s", err, output)
		}
	}
}

func BenchmarkRecursiveGeneration(b *testing.B) {
	// Build binary first
	if err := buildBinary(); err != nil {
		b.Fatalf("Failed to build binary: %v", err)
	}
	defer cleanupBinary()
	
	// Create temp directory and copy scenarios
	tempDir := b.TempDir()
	if err := copyTestScenariosForBench(b, tempDir); err != nil {
		b.Fatalf("Failed to copy scenarios: %v", err)
	}
	
	// Create some configs for recursive testing
	configs := []string{
		"project1/.airules.yaml",
		"project2/airules.yaml",
	}
	
	configContent := `metadata:
  name: "Benchmark Project"
outputs:
  - file: "test.md"
rules:
  - name: "Test Rule"
    content: "Test content"`
	
	for _, config := range configs {
		fullPath := filepath.Join(tempDir, config)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			b.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(configContent), 0644); err != nil {
			b.Fatalf("Failed to create config: %v", err)
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(binaryPath, "generate", "--recursive", "--dry-run")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("Command failed: %v\nOutput: %s", err, output)
		}
	}
}

func BenchmarkWithIncludes(b *testing.B) {
	// Build binary first
	if err := buildBinary(); err != nil {
		b.Fatalf("Failed to build binary: %v", err)
	}
	defer cleanupBinary()
	
	// Create temp directory and copy scenarios
	tempDir := b.TempDir()
	if err := copyTestScenariosForBench(b, tempDir); err != nil {
		b.Fatalf("Failed to copy scenarios: %v", err)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(binaryPath, "generate", "scenarios/with-includes/ai_rules.yaml")
		cmd.Dir = tempDir
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("Command failed: %v\nOutput: %s", err, output)
		}
	}
}

func BenchmarkConfigDiscovery(b *testing.B) {
	// Build binary first
	if err := buildBinary(); err != nil {
		b.Fatalf("Failed to build binary: %v", err)
	}
	defer cleanupBinary()
	
	// Create temp directory and copy scenarios
	tempDir := b.TempDir()
	configDir := filepath.Join(tempDir, "deep", "nested", "directory")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		b.Fatalf("Failed to create nested directory: %v", err)
	}
	
	// Copy a basic config to root
	configContent := `metadata:
  name: "Benchmark Test"
outputs:
  - file: "test.md"
rules:
  - name: "Test Rule"
    content: "Test content"`
	
	if err := os.WriteFile(filepath.Join(tempDir, ".airules.yaml"), []byte(configContent), 0644); err != nil {
		b.Fatalf("Failed to create config file: %v", err)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(binaryPath, "generate", "--dry-run")
		cmd.Dir = configDir // Run from deep nested directory
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("Command failed: %v\nOutput: %s", err, output)
		}
	}
}

// Helper function for benchmarks
func copyTestScenariosForBench(b *testing.B, destDir string) error {
	b.Helper()
	
	// Get current testing directory
	testingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	
	scenarios := []string{
		"scenarios/basic",
		"scenarios/minimal", 
		"scenarios/with-includes",
		"scenarios/nested-includes",
		"scenarios/invalid",
		"includes",
		"templates",
	}
	
	for _, scenario := range scenarios {
		srcPath := filepath.Join(testingDir, scenario)
		destPath := filepath.Join(destDir, scenario)
		
		if _, err := os.Stat(srcPath); err == nil {
			if err := copyDir(srcPath, destPath); err != nil {
				return err
			}
		}
	}
	
	return nil
}