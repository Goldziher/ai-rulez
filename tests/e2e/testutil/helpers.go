package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	TestBinaryName = "ai-rulez-e2e-test"
	TestTimeout    = 30 * time.Second
)

var binaryPath string

func SetupTestBinary(t *testing.T) string {
	t.Helper()

	if binaryPath != "" {
		if _, err := os.Stat(binaryPath); err == nil {
			return binaryPath
		}

		binaryPath = ""
	}

	_, helperFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "Failed to resolve helper source path")

	testDir := filepath.Dir(helperFile)

	// Find the project root from the helper file location so tests remain stable after chdir.
	projectRoot := testDir
	for {
		gomodPath := filepath.Join(projectRoot, "go.mod")
		if _, err := os.Stat(gomodPath); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			// Reached filesystem root without finding go.mod
			require.NoError(t, fmt.Errorf("could not find project root with go.mod"))
			return ""
		}
		projectRoot = parent
	}

	binaryName := TestBinaryName
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath = filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d", binaryName, os.Getpid()))

	//nolint:gosec // G204: Test utility needs to build binary with variables
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd")
	cmd.Dir = projectRoot

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to build test binary: %s", output)

	return binaryPath
}

func CleanupTestBinary() {
	if binaryPath != "" {
		//nolint:errcheck,gosec
		os.Remove(binaryPath)
		binaryPath = ""
	}
}

func CreateTempDir(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	return tmpDir
}

func WriteFile(t *testing.T, dir, filename, content string) {
	t.Helper()

	fullPath := filepath.Join(dir, filename)
	err := os.WriteFile(fullPath, []byte(content), 0o644)
	require.NoError(t, err, "Failed to write file %s", fullPath)
}

func FileExists(t *testing.T, path string) bool {
	t.Helper()

	_, err := os.Stat(path)
	return err == nil
}

func ReadFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read file %s", path)
	return string(content)
}

// SetupBasicConfig creates a basic configuration directory structure with rules
func SetupBasicConfig(t *testing.T, workingDir string) {
	t.Helper()

	// Create .ai-rulez directory
	aiRulesDir := filepath.Join(workingDir, ".ai-rulez")
	err := os.MkdirAll(aiRulesDir, 0o755)
	require.NoError(t, err, "Failed to create .ai-rulez directory")

	// Create config.yaml
	WriteFile(t, aiRulesDir, "config.yaml", BasicConfigYAML)

	// Create rules directory
	rulesDir := filepath.Join(aiRulesDir, "rules")
	err = os.MkdirAll(rulesDir, 0o755)
	require.NoError(t, err, "Failed to create rules directory")

	// Create rule files
	WriteFile(t, rulesDir, "basic-rule.md", BasicRuleMarkdown)
	WriteFile(t, rulesDir, "high-priority-rule.md", HighPriorityRuleMarkdown)

	// Create context directory
	contextDir := filepath.Join(aiRulesDir, "context")
	err = os.MkdirAll(contextDir, 0o755)
	require.NoError(t, err, "Failed to create context directory")

	// Create context file
	WriteFile(t, contextDir, "project-info.md", ProjectContextMarkdown)
}

// SetupConfigWithMCPServers creates a configuration with MCP server settings
func SetupConfigWithMCPServers(t *testing.T, workingDir string) {
	t.Helper()

	// Create .ai-rulez directory
	aiRulesDir := filepath.Join(workingDir, ".ai-rulez")
	err := os.MkdirAll(aiRulesDir, 0o755)
	require.NoError(t, err, "Failed to create .ai-rulez directory")

	// Create config.yaml (MCP servers are inlined)
	WriteFile(t, aiRulesDir, "config.yaml", ConfigWithMCPServersYAML)

	// Create empty rules directory
	rulesDir := filepath.Join(aiRulesDir, "rules")
	err = os.MkdirAll(rulesDir, 0o755)
	require.NoError(t, err, "Failed to create rules directory")
}

// SetupMultiPresetConfig creates a configuration with multiple output presets
func SetupMultiPresetConfig(t *testing.T, workingDir string) {
	t.Helper()

	// Create .ai-rulez directory
	aiRulesDir := filepath.Join(workingDir, ".ai-rulez")
	err := os.MkdirAll(aiRulesDir, 0o755)
	require.NoError(t, err, "Failed to create .ai-rulez directory")

	// Create config.yaml
	WriteFile(t, aiRulesDir, "config.yaml", ConfigWithMultiplePresetsYAML)

	// Create rules directory
	rulesDir := filepath.Join(aiRulesDir, "rules")
	err = os.MkdirAll(rulesDir, 0o755)
	require.NoError(t, err, "Failed to create rules directory")

	// Create a sample rule
	WriteFile(t, rulesDir, "sample-rule.md", BasicRuleMarkdown)
}

// SetupInvalidConfig creates a configuration with invalid content
func SetupInvalidConfig(t *testing.T, workingDir string) {
	t.Helper()

	// Create .ai-rulez directory
	aiRulesDir := filepath.Join(workingDir, ".ai-rulez")
	err := os.MkdirAll(aiRulesDir, 0o755)
	require.NoError(t, err, "Failed to create .ai-rulez directory")

	// Create invalid config.yaml
	WriteFile(t, aiRulesDir, "config.yaml", InvalidConfigYAML)
}
