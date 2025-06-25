package integration_test

import (
	"os"
	"os/exec"
	"runtime/pprof"
	"testing"
)

func TestProfileGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping profiling in short mode")
	}

	// Create CPU profile
	cpuFile, err := os.Create("cpu.prof")
	if err != nil {
		t.Fatalf("Failed to create CPU profile: %v", err)
	}
	defer func() { _ = cpuFile.Close() }()

	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		t.Fatalf("Failed to start CPU profile: %v", err)
	}
	defer pprof.StopCPUProfile()

	// Build binary first
	if err := buildBinary(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}
	defer cleanupBinary()

	// Create temp directory and copy scenarios
	tempDir := t.TempDir()
	if err := copyTestScenarios(t, tempDir); err != nil {
		t.Fatalf("Failed to copy scenarios: %v", err)
	}

	// Run multiple generations to get good profile data
	for i := 0; i < 10; i++ {
		// Test basic generation
		cmd := exec.Command(binaryPath, "generate", "scenarios/basic/ai_rulez.yaml")
		cmd.Dir = tempDir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Command failed: %v\nOutput: %s", err, output)
		}

		// Test with includes
		cmd = exec.Command(binaryPath, "generate", "scenarios/with-includes/ai_rulez.yaml")
		cmd.Dir = tempDir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Command failed: %v\nOutput: %s", err, output)
		}

		// Test validation
		cmd = exec.Command(binaryPath, "validate", "scenarios/basic/ai_rulez.yaml")
		cmd.Dir = tempDir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Command failed: %v\nOutput: %s", err, output)
		}
	}

	// Write memory profile
	memFile, err := os.Create("mem.prof")
	if err != nil {
		t.Fatalf("Failed to create memory profile: %v", err)
	}
	defer func() { _ = memFile.Close() }()

	if err := pprof.WriteHeapProfile(memFile); err != nil {
		t.Fatalf("Failed to write memory profile: %v", err)
	}

	t.Log("Profiling complete. Run 'go tool pprof cpu.prof' or 'go tool pprof mem.prof' to analyze")
}