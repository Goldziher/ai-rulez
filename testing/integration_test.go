package integration_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	binaryName = "airules-test"
	testTimeout = 30 * time.Second
)


type TestCase struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	WorkingDir  string            `json:"working_dir"`
	Command     []string          `json:"command"`
	Input       string            `json:"input,omitempty"`
	ExpectedOut string            `json:"expected_out,omitempty"`
	ExpectedErr string            `json:"expected_err,omitempty"`
	ExitCode    int               `json:"exit_code"`
	Files       map[string]string `json:"files,omitempty"`
	Setup       []string          `json:"setup,omitempty"`
	Cleanup     []string          `json:"cleanup,omitempty"`
}


type TestSuite struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Setup       []string   `json:"setup,omitempty"`
	Cleanup     []string   `json:"cleanup,omitempty"`
	Tests       []TestCase `json:"tests"`
}

var binaryPath string

func TestMain(m *testing.M) {
	
	if err := buildBinary(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build binary: %v\n", err)
		os.Exit(1)
	}

	
	code := m.Run()

	
	cleanupBinary()

	os.Exit(code)
}

func buildBinary() error {
	
	testingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	
	projectRoot := filepath.Dir(testingDir)
	binaryPath = filepath.Join(testingDir, binaryName)
	
	
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = projectRoot
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build binary: %w\nOutput: %s", err, output)
	}
	
	return nil
}


func cleanupBinary() {
	if binaryPath != "" {
		_ = os.Remove(binaryPath)
	}
}

func runCommand(t *testing.T, testCase TestCase, testDir string) (string, string, int) {
	t.Helper()
	
	
	cmd := exec.Command(binaryPath, testCase.Command...)
	
	
	if testCase.WorkingDir != "" {
		cmd.Dir = filepath.Join(testDir, testCase.WorkingDir)
	} else {
		cmd.Dir = testDir
	}
	
	
	if testCase.Input != "" {
		cmd.Stdin = strings.NewReader(testCase.Input)
	}
	
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()
	
	select {
	case err := <-done:
		exitCode := 0
		if err != nil {
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				exitCode = exitError.ExitCode()
			} else {
				t.Fatalf("Command failed to run: %v", err)
			}
		}
		return stdout.String(), stderr.String(), exitCode
		
	case <-time.After(testTimeout):
		_ = cmd.Process.Kill()
		t.Fatalf("Command timed out after %v", testTimeout)
		return "", "", -1
	}
}

func setupTestCase(t *testing.T, testCase TestCase, testDir string) {
	t.Helper()
	
	
	needsScenarios := false
	for _, arg := range testCase.Command {
		if strings.Contains(arg, "scenarios/") {
			needsScenarios = true
			break
		}
	}
	
	if needsScenarios {
		
		currentTestingDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get current directory: %v", err)
		}
		
		scenarios := []string{
			"scenarios/basic",
			"scenarios/minimal", 
			"scenarios/with-includes",
			"scenarios/nested-includes",
			"scenarios/invalid",
			"scenarios/circular",
			"scenarios/custom-template",
			"scenarios/empty-project",
			"scenarios/inline-template",
			"includes",
			"templates",
		}
		
		for _, scenario := range scenarios {
			srcPath := filepath.Join(currentTestingDir, scenario)
			destPath := filepath.Join(testDir, scenario)
			
			if _, err := os.Stat(srcPath); err == nil {
				if err := copyDir(srcPath, destPath); err != nil {
					t.Fatalf("Failed to copy scenario %s: %v", scenario, err)
				}
			}
		}
	}
	
	
	for _, setupCmd := range testCase.Setup {
		parts := strings.Fields(setupCmd)
		if len(parts) == 0 {
			continue
		}
		
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = testDir
		
		if err := cmd.Run(); err != nil {
			t.Fatalf("Setup command failed: %v", err)
		}
	}
	
	
	for filePath, content := range testCase.Files {
		fullPath := filepath.Join(testDir, filePath)
		
		
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("Failed to create directory for %s: %v", filePath, err)
		}
		
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", filePath, err)
		}
	}
}

func cleanupTestCase(t *testing.T, testCase TestCase, testDir string) {
	t.Helper()
	
	
	for _, cleanupCmd := range testCase.Cleanup {
		parts := strings.Fields(cleanupCmd)
		if len(parts) == 0 {
			continue
		}
		
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = testDir
		_ = cmd.Run() // ~keep Ignore errors in cleanup
	}
}

func normalizeOutput(output string) string {
	
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	output = ansiRegex.ReplaceAllString(output, "")
	
	
	lines := strings.Split(output, "\n")
	var normalizedLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			normalizedLines = append(normalizedLines, line)
		}
	}
	
	return strings.Join(normalizedLines, "\n")
}

func loadTestSuite(t *testing.T, suitePath string) TestSuite {
	t.Helper()
	
	data, err := os.ReadFile(suitePath)
	require.NoError(t, err, "Failed to read test suite file")
	
	var suite TestSuite
	err = json.Unmarshal(data, &suite)
	require.NoError(t, err, "Failed to parse test suite JSON")
	
	return suite
}

func runTestSuite(t *testing.T, suite TestSuite, testDir string) {
	t.Helper()
	
	
	for _, setupCmd := range suite.Setup {
		parts := strings.Fields(setupCmd)
		if len(parts) == 0 {
			continue
		}
		
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = testDir
		
		if err := cmd.Run(); err != nil {
			t.Fatalf("Suite setup command failed: %v", err)
		}
	}
	
	
	for _, testCase := range suite.Tests {
		t.Run(testCase.Name, func(t *testing.T) {
			
			caseDir := filepath.Join(testDir, fmt.Sprintf("case_%s", strings.ReplaceAll(testCase.Name, " ", "_")))
			err := os.MkdirAll(caseDir, 0755)
			require.NoError(t, err)
			
			
			setupTestCase(t, testCase, caseDir)
			
			
			stdout, stderr, exitCode := runCommand(t, testCase, caseDir)
			
			
			if exitCode != testCase.ExitCode {
				t.Logf("Command failed: %v %v", binaryPath, testCase.Command)
				t.Logf("Working dir: %s", caseDir)
				t.Logf("Stdout: %s", stdout)
				t.Logf("Stderr: %s", stderr)
			}
			
			
			assert.Equal(t, testCase.ExitCode, exitCode, "Exit code mismatch")
			
			
			if testCase.ExpectedOut != "" {
				normalizedOut := normalizeOutput(stdout)
				if strings.Contains(testCase.ExpectedOut, "*") || strings.Contains(testCase.ExpectedOut, "?") {
					
					pattern := regexp.QuoteMeta(testCase.ExpectedOut)
					pattern = strings.ReplaceAll(pattern, "\\*", ".*")
					pattern = strings.ReplaceAll(pattern, "\\?", ".")
					
					matched, err := regexp.MatchString(pattern, normalizedOut)
					assert.NoError(t, err, "Invalid regex pattern")
					assert.True(t, matched, 
						"stdout pattern mismatch\nExpected pattern: %s\nActual: %s", testCase.ExpectedOut, normalizedOut)
				} else {
					assert.Contains(t, normalizedOut, testCase.ExpectedOut, 
						"stdout content mismatch\nFull output: %s", normalizedOut)
				}
			}
			
			
			if testCase.ExpectedErr != "" {
				normalizedErr := normalizeOutput(stderr)
				if strings.Contains(testCase.ExpectedErr, "*") || strings.Contains(testCase.ExpectedErr, "?") {
					
					pattern := regexp.QuoteMeta(testCase.ExpectedErr)
					pattern = strings.ReplaceAll(pattern, "\\*", ".*")
					pattern = strings.ReplaceAll(pattern, "\\?", ".")
					
					matched, err := regexp.MatchString(pattern, normalizedErr)
					assert.NoError(t, err, "Invalid regex pattern")
					assert.True(t, matched, 
						"stderr pattern mismatch\nExpected pattern: %s\nActual: %s", testCase.ExpectedErr, normalizedErr)
				} else {
					assert.Contains(t, normalizedErr, testCase.ExpectedErr, 
						"stderr content mismatch\nFull output: %s", normalizedErr)
				}
			}
			
			
			cleanupTestCase(t, testCase, caseDir)
		})
	}
	
	
	for _, cleanupCmd := range suite.Cleanup {
		parts := strings.Fields(cleanupCmd)
		if len(parts) == 0 {
			continue
		}
		
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = testDir
		_ = cmd.Run() // ~keep Ignore errors in cleanup
	}
}