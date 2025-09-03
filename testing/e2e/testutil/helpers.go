package testutil

import (
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

// SetupTestBinary builds the ai-rulez binary for testing
func SetupTestBinary(t *testing.T) string {
	t.Helper()

	if binaryPath != "" {
		return binaryPath
	}

	testDir, err := os.Getwd()
	require.NoError(t, err)

	projectRoot := filepath.Join(testDir, "..", "..", "..")
	binaryName := TestBinaryName
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath = filepath.Join(testDir, binaryName)

	//nolint:gosec // G204: Test utility needs to build binary with variables
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd")
	cmd.Dir = projectRoot

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to build test binary: %s", output)

	return binaryPath
}

// CleanupTestBinary removes the test binary
func CleanupTestBinary() {
	if binaryPath != "" {
		//nolint:errcheck,gosec
		os.Remove(binaryPath)
		binaryPath = ""
	}
}

// CreateTempDir creates a temporary directory for testing
func CreateTempDir(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	return tmpDir
}

// WriteFile writes content to a file in the given directory
func WriteFile(t *testing.T, dir, filename, content string) {
	t.Helper()

	fullPath := filepath.Join(dir, filename)
	err := os.WriteFile(fullPath, []byte(content), 0o644)
	require.NoError(t, err, "Failed to write file %s", fullPath)
}

// FileExists checks if a file exists at the given path
func FileExists(t *testing.T, path string) bool {
	t.Helper()

	_, err := os.Stat(path)
	return err == nil
}

// ReadFile reads and returns file content
func ReadFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read file %s", path)
	return string(content)
}

// AssertFileContent checks that a file exists and contains expected content
func AssertFileContent(t *testing.T, path, expectedContent string) {
	t.Helper()

	require.True(t, FileExists(t, path), "File should exist: %s", path)
	content := ReadFile(t, path)
	require.Contains(t, content, expectedContent, "File content mismatch in %s", path)
}

// AssertFileNotExists checks that a file does not exist
func AssertFileNotExists(t *testing.T, path string) {
	t.Helper()

	require.False(t, FileExists(t, path), "File should not exist: %s", path)
}
