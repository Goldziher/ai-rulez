package benchmarks

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var (
	testAgents = []string{
		"claude",
		"gemini",
		"amp",
		"continue-dev",
		"cursor",
		"windsurf",
		"github-copilot",
	}

	aiRulezBinary = "../../ai-rulez"
	fixturesPath  = "../fixtures"
)

// BenchmarkAgentPerformance benchmarks each agent's performance
func BenchmarkAgentPerformance(b *testing.B) {
	// Build ai-rulez first
	buildAIRulez(b)

	for _, agent := range testAgents {
		if !isAgentAvailable(agent) {
			continue
		}

		b.Run(agent, func(b *testing.B) {
			b.Run("SingleFile", func(b *testing.B) {
				benchmarkSingleFile(b, agent)
			})

			b.Run("MultipleFiles", func(b *testing.B) {
				benchmarkMultipleFiles(b, agent)
			})

			b.Run("WithFixes", func(b *testing.B) {
				benchmarkWithFixes(b, agent)
			})

			b.Run("WithReview", func(b *testing.B) {
				benchmarkWithReview(b, agent)
			})
		})
	}
}

// TestAgentResponseTimes measures and compares agent response times
func TestAgentResponseTimes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	buildAIRulez(t)

	results := make(map[string]AgentPerformance)

	for _, agent := range testAgents {
		if !isAgentAvailable(agent) {
			t.Logf("Agent %s not available, skipping", agent)
			continue
		}

		perf := measureAgentPerformance(t, agent)
		results[agent] = perf
	}

	// Generate performance report
	generatePerformanceReport(t, results)
}

type AgentPerformance struct {
	Agent               string
	SingleFileTime      time.Duration
	MultipleFilesTime   time.Duration
	ViolationsDetected  int
	FixesApplied        int
	AverageResponseTime time.Duration
	TimeoutRate         float64
}

func benchmarkSingleFile(b *testing.B, agent string) {
	testFile := filepath.Join(fixturesPath, "violations/javascript/console_log.js")
	configFile := filepath.Join(fixturesPath, "configs/test_enforcement.yaml")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runEnforcementBenchmark(b, agent, configFile, testFile)
	}
}

func benchmarkMultipleFiles(b *testing.B, agent string) {
	testPattern := filepath.Join(fixturesPath, "violations/javascript/*.js")
	configFile := filepath.Join(fixturesPath, "configs/test_enforcement.yaml")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runEnforcementBenchmark(b, agent, configFile, testPattern)
	}
}

func benchmarkWithFixes(b *testing.B, agent string) {
	// Create temp directory for each iteration
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tempDir := setupTempProject(b)
		b.StartTimer()

		runEnforcementBenchmark(b, agent,
			filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
			filepath.Join(tempDir, "*.js"),
			"--fix")

		b.StopTimer()
		os.RemoveAll(tempDir)
		b.StartTimer()
	}
}

func benchmarkWithReview(b *testing.B, agent string) {
	testFile := filepath.Join(fixturesPath, "violations/javascript/console_log.js")
	configFile := filepath.Join(fixturesPath, "configs/test_enforcement.yaml")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runEnforcementBenchmark(b, agent, configFile, testFile,
			"--review", "--review-iterations", "2")
	}
}

func measureAgentPerformance(t *testing.T, agent string) AgentPerformance {
	perf := AgentPerformance{Agent: agent}

	// Measure single file performance
	start := time.Now()
	result1 := runEnforcementForPerf(t, agent,
		filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
		filepath.Join(fixturesPath, "violations/javascript/console_log.js"))
	perf.SingleFileTime = time.Since(start)

	// Measure multiple files performance
	start = time.Now()
	result2 := runEnforcementForPerf(t, agent,
		filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
		filepath.Join(fixturesPath, "violations/javascript/*.js"))
	perf.MultipleFilesTime = time.Since(start)

	// Calculate statistics
	if result1 != nil {
		perf.ViolationsDetected = len(result1.Violations)
	}
	if result2 != nil && len(result2.Violations) > 0 {
		perf.ViolationsDetected = len(result2.Violations)
	}

	// Calculate average response time
	perf.AverageResponseTime = (perf.SingleFileTime + perf.MultipleFilesTime) / 2

	// Test timeout rate (with very short timeout)
	timeouts := 0
	total := 5
	for i := 0; i < total; i++ {
		result := runEnforcementWithTimeout(t, agent, "100ms")
		if result == nil || result.ErrorMessage != "" {
			timeouts++
		}
	}
	perf.TimeoutRate = float64(timeouts) / float64(total)

	return perf
}

func generatePerformanceReport(t *testing.T, results map[string]AgentPerformance) {
	t.Log("\n=== Agent Performance Report ===\n")

	// Find best performers
	var fastestSingle, fastestMultiple string
	fastestSingleTime, fastestMultipleTime := time.Hour, time.Hour

	for agent, perf := range results {
		t.Logf("Agent: %s", agent)
		t.Logf("  Single File:      %v", perf.SingleFileTime)
		t.Logf("  Multiple Files:   %v", perf.MultipleFilesTime)
		t.Logf("  Violations Found: %d", perf.ViolationsDetected)
		t.Logf("  Avg Response:     %v", perf.AverageResponseTime)
		t.Logf("  Timeout Rate:     %.1f%%\n", perf.TimeoutRate*100)

		if perf.SingleFileTime < fastestSingleTime {
			fastestSingleTime = perf.SingleFileTime
			fastestSingle = agent
		}
		if perf.MultipleFilesTime < fastestMultipleTime {
			fastestMultipleTime = perf.MultipleFilesTime
			fastestMultiple = agent
		}
	}

	t.Log("\n=== Performance Summary ===")
	t.Logf("Fastest Single File:    %s (%v)", fastestSingle, fastestSingleTime)
	t.Logf("Fastest Multiple Files: %s (%v)", fastestMultiple, fastestMultipleTime)

	// Create comparison table
	t.Log("\n=== Relative Performance (normalized to fastest) ===")
	for agent, perf := range results {
		singleRatio := float64(perf.SingleFileTime) / float64(fastestSingleTime)
		multiRatio := float64(perf.MultipleFilesTime) / float64(fastestMultipleTime)
		t.Logf("%s: Single=%.2fx, Multiple=%.2fx", agent, singleRatio, multiRatio)
	}
}

// Helper functions

func buildAIRulez(tb testing.TB) {
	cmd := exec.Command("go", "build", "-o", aiRulezBinary, "./cmd")
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("Failed to build ai-rulez: %s", output)
	}
}

func isAgentAvailable(agent string) bool {
	cmd := exec.Command("which", agent)
	return cmd.Run() == nil
}

func setupTempProject(tb testing.TB) string {
	tempDir, err := os.MkdirTemp("", "ai-rulez-bench-*")
	if err != nil {
		tb.Fatal(err)
	}

	// Copy test files
	sourceDir := filepath.Join(fixturesPath, "violations/javascript")
	files, _ := filepath.Glob(filepath.Join(sourceDir, "*.js"))

	for _, file := range files {
		content, _ := os.ReadFile(file)
		destFile := filepath.Join(tempDir, filepath.Base(file))
		os.WriteFile(destFile, content, 0644)
	}

	return tempDir
}

func runEnforcementBenchmark(b *testing.B, agent, configFile, filePattern string, extraArgs ...string) {
	args := []string{
		"enforce",
		"--agent", agent,
		"--config", configFile,
		"--include-files", filePattern,
		"--format", "json",
		"--timeout", "30s",
	}
	args = append(args, extraArgs...)

	cmd := exec.Command(aiRulezBinary, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil // Ignore stderr for benchmarks

	if err := cmd.Run(); err != nil {
		// Don't fail benchmark on enforcement errors
		return
	}
}

type EnforcementResult struct {
	Violations   []interface{} `json:"violations"`
	FixesApplied int           `json:"fixes_applied"`
	ErrorMessage string        `json:"error_message"`
}

func runEnforcementForPerf(t *testing.T, agent, configFile, filePattern string) *EnforcementResult {
	args := []string{
		"enforce",
		"--agent", agent,
		"--config", configFile,
		"--include-files", filePattern,
		"--format", "json",
		"--timeout", "20s",
	}

	cmd := exec.Command(aiRulezBinary, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		// Log but don't fail
		t.Logf("Enforcement command failed: %v", err)
	}

	var result EnforcementResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Logf("Failed to parse result: %v", err)
		return nil
	}

	return &result
}

func runEnforcementWithTimeout(t *testing.T, agent, timeout string) *EnforcementResult {
	args := []string{
		"enforce",
		"--agent", agent,
		"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
		"--include-files", filepath.Join(fixturesPath, "violations/javascript/*.js"),
		"--format", "json",
		"--timeout", timeout,
	}

	cmd := exec.Command(aiRulezBinary, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	// Don't check error - we expect timeouts
	cmd.Run()

	var result EnforcementResult
	json.Unmarshal(stdout.Bytes(), &result)
	return &result
}
