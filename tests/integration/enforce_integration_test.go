package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test configuration
var (
	// Agents to test - will skip if not available
	testAgents = []string{
		"claude",
		"gemini",
		"amp",
		"continue-dev",
		"cursor",
		"windsurf",
		"github-copilot",
	}

	// Default timeout per agent operation
	defaultAgentTimeout = "30s"

	// Path to ai-rulez binary (built before tests)
	aiRulezBinary = "../../ai-rulez"

	// Test fixtures path
	fixturesPath = "../fixtures"
)

// EnforcementResult represents the JSON output from enforcement
type EnforcementResult struct {
	ConfigPath    string      `json:"config_path"`
	TotalRules    int         `json:"total_rules"`
	EnforcedRules int         `json:"enforced_rules"`
	FilesScanned  int         `json:"files_scanned"`
	Violations    []Violation `json:"violations"`
	FixesApplied  int         `json:"fixes_applied"`
	Duration      string      `json:"duration"`
	Success       bool        `json:"success"`
	AgentUsed     string      `json:"agent_used,omitempty"`
	ErrorMessage  string      `json:"error_message,omitempty"`
}

type Violation struct {
	FilePath     string `json:"file_path"`
	LineNumber   int    `json:"line_number"`
	ColumnNumber int    `json:"column_number"`
	RuleName     string `json:"rule_name"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Context      string `json:"context,omitempty"`
	Suggestion   string `json:"suggestion,omitempty"`
}

// TestEnforcementWithRealAgents tests each available agent
func TestEnforcementWithRealAgents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Build ai-rulez binary first
	buildAIRulez(t)

	for _, agent := range testAgents {
		t.Run(fmt.Sprintf("Agent_%s", agent), func(t *testing.T) {
			if !isAgentAvailable(agent) {
				t.Skipf("Agent %s not available", agent)
			}

			t.Run("ReadOnlyMode", func(t *testing.T) {
				testReadOnlyMode(t, agent)
			})

			t.Run("FixMode", func(t *testing.T) {
				testFixMode(t, agent)
			})

			t.Run("ViolationDetection", func(t *testing.T) {
				testViolationDetection(t, agent)
			})
		})
	}
}

// TestReadOnlyMode verifies no changes are made without --fix flag
func testReadOnlyMode(t *testing.T, agent string) {
	// Create temp directory with test files
	tempDir := setupTestProject(t, "javascript")
	defer os.RemoveAll(tempDir)

	// Run enforcement without --fix
	result := runEnforcement(t,
		"--agent", agent,
		"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
		"--include-files", filepath.Join(tempDir, "*.js"),
		"--format", "json",
		"--timeout", defaultAgentTimeout,
	)

	// Verify no fixes were applied
	assert.Equal(t, 0, result.FixesApplied, "No fixes should be applied in read-only mode")
	assert.True(t, result.FilesScanned > 0, "Should scan files")

	// Verify original files are unchanged
	verifyFilesUnchanged(t, tempDir)
}

// TestFixMode verifies fixes are applied correctly
func testFixMode(t *testing.T, agent string) {
	// Create temp directory with test files
	tempDir := setupTestProject(t, "javascript")
	defer os.RemoveAll(tempDir)

	// Run enforcement with --fix
	result := runEnforcement(t,
		"--agent", agent,
		"--fix",
		"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
		"--include-files", filepath.Join(tempDir, "*.js"),
		"--format", "json",
		"--timeout", defaultAgentTimeout,
	)

	// Verify fixes were attempted
	if len(result.Violations) > 0 {
		// Some violations should have been fixed
		assert.True(t, result.FixesApplied > 0 || result.ErrorMessage != "",
			"Should apply fixes or report why not")
	}
}

// TestViolationDetection verifies agents detect known violations
func testViolationDetection(t *testing.T, agent string) {
	// Test with JavaScript violations
	t.Run("JavaScript", func(t *testing.T) {
		result := runEnforcement(t,
			"--agent", agent,
			"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
			"--include-files", filepath.Join(fixturesPath, "violations/javascript/*.js"),
			"--format", "json",
			"--timeout", defaultAgentTimeout,
		)

		// Should detect violations in test files
		assert.True(t, len(result.Violations) > 0,
			"Agent %s should detect violations in JavaScript files", agent)

		// Check for specific violation types
		hasConsoleLog := false
		hasErrorHandling := false
		for _, v := range result.Violations {
			if strings.Contains(strings.ToLower(v.Message), "console") {
				hasConsoleLog = true
			}
			if strings.Contains(strings.ToLower(v.Message), "error") {
				hasErrorHandling = true
			}
		}

		t.Logf("Agent %s detected %d violations", agent, len(result.Violations))
		t.Logf("Console.log violations: %v", hasConsoleLog)
		t.Logf("Error handling violations: %v", hasErrorHandling)
	})
}

// TestOutputFormats tests all output formats with real data
func TestOutputFormats(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	agent := getFirstAvailableAgent(t)
	if agent == "" {
		t.Skip("No agents available for testing")
	}

	formats := []string{"json", "csv", "table", "summary"}

	for _, format := range formats {
		t.Run(fmt.Sprintf("Format_%s", format), func(t *testing.T) {
			output := runEnforcementRaw(t,
				"--agent", agent,
				"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
				"--include-files", filepath.Join(fixturesPath, "violations/javascript/*.js"),
				"--format", format,
				"--timeout", "10s",
			)

			// Verify output is not empty
			assert.NotEmpty(t, output, "Output should not be empty for format %s", format)

			// Format-specific validation
			switch format {
			case "json":
				var result EnforcementResult
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err, "JSON output should be valid")
			case "csv":
				assert.Contains(t, output, "file_path", "CSV should have headers")
			case "table":
				assert.Contains(t, output, "Enforcement Report", "Table should have title")
			case "summary":
				assert.Contains(t, output, "Enforcement Summary", "Summary should have title")
			}
		})
	}
}

// TestFileFiltering tests include/exclude patterns
func TestFileFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	agent := getFirstAvailableAgent(t)
	if agent == "" {
		t.Skip("No agents available for testing")
	}

	// Test include patterns
	t.Run("IncludePattern", func(t *testing.T) {
		result := runEnforcement(t,
			"--agent", agent,
			"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
			"--include-files", filepath.Join(fixturesPath, "violations/javascript/console*.js"),
			"--format", "json",
			"--timeout", "10s",
		)

		// Should only scan matching files
		for _, v := range result.Violations {
			assert.Contains(t, v.FilePath, "console",
				"Should only include files matching pattern")
		}
	})

	// Test exclude patterns
	t.Run("ExcludePattern", func(t *testing.T) {
		result := runEnforcement(t,
			"--agent", agent,
			"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
			"--include-files", filepath.Join(fixturesPath, "violations/javascript/*.js"),
			"--exclude-files", "*console*",
			"--format", "json",
			"--timeout", "10s",
		)

		// Should not scan excluded files
		for _, v := range result.Violations {
			assert.NotContains(t, v.FilePath, "console",
				"Should exclude files matching pattern")
		}
	})
}

// TestEnforcementLevels tests different enforcement levels
func TestEnforcementLevels(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	agent := getFirstAvailableAgent(t)
	if agent == "" {
		t.Skip("No agents available for testing")
	}

	levels := []string{"warn", "error", "strict"}

	for _, level := range levels {
		t.Run(fmt.Sprintf("Level_%s", level), func(t *testing.T) {
			result := runEnforcement(t,
				"--agent", agent,
				"--level", level,
				"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
				"--include-files", filepath.Join(fixturesPath, "violations/javascript/*.js"),
				"--format", "json",
				"--timeout", "10s",
			)

			t.Logf("Level %s: %d violations detected", level, len(result.Violations))

			// Strict should fail if there are violations
			if level == "strict" && len(result.Violations) > 0 {
				assert.False(t, result.Success, "Strict mode should fail with violations")
			}
		})
	}
}

// TestAgentTimeout verifies timeout handling
func TestAgentTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	agent := getFirstAvailableAgent(t)
	if agent == "" {
		t.Skip("No agents available for testing")
	}

	// Use very short timeout to trigger timeout
	output := runEnforcementRaw(t,
		"--agent", agent,
		"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
		"--include-files", filepath.Join(fixturesPath, "violations/**/*.js"),
		"--timeout", "1ms", // Extremely short timeout
		"--format", "json",
	)

	// Should handle timeout gracefully
	assert.NotEmpty(t, output, "Should produce output even on timeout")
}

// Helper Functions

func buildAIRulez(t *testing.T) {
	cmd := exec.Command("go", "build", "-o", aiRulezBinary, "./cmd")
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to build ai-rulez: %s", output)
}

func isAgentAvailable(agent string) bool {
	cmd := exec.Command("which", agent)
	err := cmd.Run()
	return err == nil
}

func getFirstAvailableAgent(t *testing.T) string {
	for _, agent := range testAgents {
		if isAgentAvailable(agent) {
			return agent
		}
	}
	return ""
}

func setupTestProject(t *testing.T, language string) string {
	tempDir, err := os.MkdirTemp("", "ai-rulez-test-*")
	require.NoError(t, err)

	// Copy test files to temp directory
	sourceDir := filepath.Join(fixturesPath, "violations", language)
	files, err := filepath.Glob(filepath.Join(sourceDir, "*"))
	require.NoError(t, err)

	for _, file := range files {
		content, err := os.ReadFile(file)
		require.NoError(t, err)

		destFile := filepath.Join(tempDir, filepath.Base(file))
		err = os.WriteFile(destFile, content, 0644)
		require.NoError(t, err)
	}

	return tempDir
}

func runEnforcement(t *testing.T, args ...string) *EnforcementResult {
	output := runEnforcementRaw(t, args...)

	// Parse JSON output
	var result EnforcementResult
	err := json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "Failed to parse enforcement output: %s", output)

	return &result
}

func runEnforcementRaw(t *testing.T, args ...string) string {
	cmdArgs := append([]string{"enforce"}, args...)
	cmd := exec.Command(aiRulezBinary, cmdArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Log output for debugging
	t.Logf("Command: %s %s", aiRulezBinary, strings.Join(cmdArgs, " "))
	t.Logf("Stdout: %s", stdout.String())
	if stderr.Len() > 0 {
		t.Logf("Stderr: %s", stderr.String())
	}

	// Don't fail on non-zero exit codes (enforcement may fail legitimately)
	if err != nil && !strings.Contains(err.Error(), "exit status") {
		t.Fatalf("Command failed: %v", err)
	}

	return stdout.String()
}

func verifyFilesUnchanged(t *testing.T, dir string) {
	// Read original fixture files and compare
	// This is simplified - in production, you'd calculate checksums
	files, err := filepath.Glob(filepath.Join(dir, "*.js"))
	require.NoError(t, err)

	for _, file := range files {
		originalName := filepath.Base(file)
		originalPath := filepath.Join(fixturesPath, "violations/javascript", originalName)

		original, err := os.ReadFile(originalPath)
		require.NoError(t, err)

		current, err := os.ReadFile(file)
		require.NoError(t, err)

		assert.Equal(t, string(original), string(current),
			"File %s should be unchanged in read-only mode", originalName)
	}
}
