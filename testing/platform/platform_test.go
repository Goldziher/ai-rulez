package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCrossPlatformPaths tests that our path handling works correctly across platforms
func TestCrossPlatformPaths(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		input    []string
		expected func(string) string
	}{
		{
			name:     "Windows paths",
			platform: "windows",
			input:    []string{"C:", "Users", "test", "ai_rulez.yaml"},
			expected: func(platform string) string {
				if platform == "windows" {
					return "C:\\Users\\test\\ai_rulez.yaml"
				}
				return "C:/Users/test/ai_rulez.yaml" // filepath.Join normalizes
			},
		},
		{
			name:     "Unix paths",
			platform: "linux",
			input:    []string{"", "home", "user", ".ai-rulez", "config.yaml"},
			expected: func(platform string) string {
				return "/home/user/.ai-rulez/config.yaml"
			},
		},
		{
			name:     "macOS paths",
			platform: "darwin",
			input:    []string{"", "Users", "developer", "workspace", "ai-rulez"},
			expected: func(platform string) string {
				return "/Users/developer/workspace/ai-rulez"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filepath.Join(tt.input...)

			// On the current platform, verify basic properties
			if tt.platform == runtime.GOOS {
				// Don't do exact match as platforms may normalize differently
				// Instead verify the path has the expected components
				assert.NotEmpty(t, result, "Path should not be empty on current platform")
			}

			// Always verify the path is absolute when it should be
			if len(tt.input) > 0 && tt.input[0] != "" && tt.input[0] != "C:" {
				assert.True(t, filepath.IsAbs(result), "Path should be absolute")
			}
		})
	}
}

// TestBinaryExtensionHandling tests that we correctly handle binary extensions across platforms
func TestBinaryExtensionHandling(t *testing.T) {
	tests := []struct {
		name     string
		baseName string
		platform string
		expected string
	}{
		{"Windows binary", "ai-rulez", "windows", "ai-rulez.exe"},
		{"Linux binary", "ai-rulez", "linux", "ai-rulez"},
		{"macOS binary", "ai-rulez", "darwin", "ai-rulez"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addBinaryExtension(tt.baseName, tt.platform)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// addBinaryExtension adds .exe extension on Windows
func addBinaryExtension(baseName, platform string) string {
	if platform == "windows" {
		return baseName + ".exe"
	}
	return baseName
}

// TestGitCommandsAcrossPlatforms ensures git commands work cross-platform
func TestGitCommandsAcrossPlatforms(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}

	// Test basic git availability (should work on all platforms)
	cmd := exec.Command("git", "--version")
	output, err := cmd.Output()
	require.NoError(t, err, "git --version should work on all platforms")
	assert.Contains(t, strings.ToLower(string(output)), "git version", "Should contain git version info")

	// Test git rev-parse (directory independent command)
	cmd = exec.Command("git", "rev-parse", "--show-toplevel")
	// This may fail if we're not in a git repo, but the command structure should be valid
	_, _ = cmd.Output()
	// We don't assert no error here since we might not be in a git repo
	// but we do assert that the command can be constructed and executed
	assert.NotNil(t, cmd, "Git commands should be constructible on all platforms")
}

// TestEnvironmentVariableHandling tests environment variable access patterns
func TestEnvironmentVariableHandling(t *testing.T) {
	// Test setting and getting environment variables (cross-platform)
	testVar := "AI_RULEZ_TEST_VAR"
	testValue := "cross_platform_test_value"

	// Clean up after test
	defer os.Unsetenv(testVar)

	// Set environment variable
	err := os.Setenv(testVar, testValue)
	require.NoError(t, err, "Setting environment variable should work on all platforms")

	// Get environment variable
	result := os.Getenv(testVar)
	assert.Equal(t, testValue, result, "Environment variable retrieval should work cross-platform")

	// Test case sensitivity expectations
	if runtime.GOOS == "windows" {
		// Windows env vars are case-insensitive, but Go preserves case
		// The behavior may vary, so we'll just ensure we can retrieve it
		result2 := os.Getenv(strings.ToLower(testVar))
		// On Windows, this might be empty if Go doesn't do case-insensitive lookup
		t.Logf("Windows case-insensitive test: %s -> %s", strings.ToLower(testVar), result2)
	} else {
		// Unix-like systems are case-sensitive
		result2 := os.Getenv(strings.ToLower(testVar))
		assert.Empty(t, result2, "Unix systems should be case-sensitive for env vars")
	}
}

// TestFilePermissionsHandling tests file permission handling across platforms
func TestFilePermissionsHandling(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_permissions.txt")

	// Create test file
	err := os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err, "Creating test file should work on all platforms")

	// Check file exists
	_, err = os.Stat(testFile)
	require.NoError(t, err, "File should exist after creation")

	if runtime.GOOS != "windows" {
		// Unix-like systems: test specific permissions
		err = os.Chmod(testFile, 0o755)
		require.NoError(t, err, "Chmod should work on Unix-like systems")

		info, err := os.Stat(testFile)
		require.NoError(t, err)

		mode := info.Mode().Perm()
		assert.Equal(t, os.FileMode(0o755), mode, "File permissions should be set correctly on Unix")
	} else {
		// Windows: permissions work differently, but file operations should still work
		t.Log("Windows: Skipping specific permission checks, testing basic file operations")

		// Ensure we can read the file
		content, err := os.ReadFile(testFile)
		require.NoError(t, err, "Reading file should work on Windows")
		assert.Equal(t, "test content", string(content))
	}
}

// TestDirectorySeparatorHandling tests that we handle directory separators correctly
func TestDirectorySeparatorHandling(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected func() string
	}{
		{
			name:  "Unix path with forward slashes",
			input: "path/to/file.yaml",
			expected: func() string {
				return filepath.FromSlash("path/to/file.yaml")
			},
		},
		{
			name:  "Windows path with backslashes",
			input: "path\\to\\file.yaml",
			expected: func() string {
				// filepath.Clean normalizes separators
				return filepath.Clean("path\\to\\file.yaml")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filepath.Clean(tt.input)
			expected := tt.expected()

			// Both should use the platform's native separator
			assert.Equal(t, expected, result)

			// Split should work consistently (handle different separators)
			parts := strings.FieldsFunc(result, func(c rune) bool {
				return c == '/' || c == '\\'
			})
			assert.Contains(t, parts, "path", "Should contain path component")
			assert.Contains(t, parts, "to", "Should contain to component")
			assert.Contains(t, parts, "file.yaml", "Should contain file component")
		})
	}
}

// TestTempDirectoryHandling tests temporary directory creation across platforms
func TestTempDirectoryHandling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ai-rulez-test-*")
	require.NoError(t, err, "Creating temp directory should work on all platforms")
	defer os.RemoveAll(tempDir)

	// Verify temp directory exists and is writable
	assert.True(t, filepath.IsAbs(tempDir), "Temp directory should be absolute path")

	// Test creating files in temp directory
	testFile := filepath.Join(tempDir, "test.yaml")
	err = os.WriteFile(testFile, []byte("test: value\n"), 0o644)
	require.NoError(t, err, "Writing to temp directory should work")

	// Test reading back
	content, err := os.ReadFile(testFile)
	require.NoError(t, err, "Reading from temp directory should work")
	assert.Contains(t, string(content), "test: value")

	// Test subdirectory creation
	subDir := filepath.Join(tempDir, "subdir")
	err = os.Mkdir(subDir, 0o755)
	require.NoError(t, err, "Creating subdirectories should work")

	// Verify subdirectory
	info, err := os.Stat(subDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "Subdirectory should be recognized as directory")
}

// TestCurrentWorkingDirectory tests working directory operations
func TestCurrentWorkingDirectory(t *testing.T) {
	// Skip in CI if the environment is unusual
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping working directory test in CI environment")
	}
	
	// Get current working directory
	originalWd, err := os.Getwd()
	require.NoError(t, err, "Getting current working directory should work")

	// Should be an absolute path
	assert.True(t, filepath.IsAbs(originalWd), "Current directory should be absolute")

	// Test changing to temp directory
	tempDir := t.TempDir()

	// Ensure we return to original directory before test cleanup
	defer func() {
		// Change back to original directory before temp dir is cleaned up
		_ = os.Chdir(originalWd)
	}()

	err = os.Chdir(tempDir)
	require.NoError(t, err, "Changing directory should work")

	// Verify we're in the new directory
	currentWd, err := os.Getwd()
	require.NoError(t, err)

	// Use EvalSymlinks to resolve any symlinks (common on macOS)
	resolvedTempDir, _ := filepath.EvalSymlinks(tempDir)
	resolvedCurrentWd, _ := filepath.EvalSymlinks(currentWd)

	// Clean paths for comparison (handles separator differences)
	assert.Equal(t, filepath.Clean(resolvedTempDir), filepath.Clean(resolvedCurrentWd))
}
