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
				return "C:/Users/test/ai_rulez.yaml"
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

			if tt.platform == runtime.GOOS {
				assert.NotEmpty(t, result, "Path should not be empty on current platform")
			}

			if len(tt.input) > 0 && tt.input[0] != "" && tt.input[0] != "C:" {
				assert.True(t, filepath.IsAbs(result), "Path should be absolute")
			}
		})
	}
}

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

func addBinaryExtension(baseName, platform string) string {
	if platform == "windows" {
		return baseName + ".exe"
	}
	return baseName
}

func TestGitCommandsAcrossPlatforms(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}

	cmd := exec.Command("git", "--version")
	output, err := cmd.Output()
	require.NoError(t, err, "git --version should work on all platforms")
	assert.Contains(t, strings.ToLower(string(output)), "git version", "Should contain git version info")

	cmd = exec.Command("git", "rev-parse", "--show-toplevel")
	_, _ = cmd.Output()
	assert.NotNil(t, cmd, "Git commands should be constructible on all platforms")
}

func TestEnvironmentVariableHandling(t *testing.T) {
	testVar := "AI_RULEZ_TEST_VAR"
	testValue := "cross_platform_test_value"

	defer os.Unsetenv(testVar)

	err := os.Setenv(testVar, testValue)
	require.NoError(t, err, "Setting environment variable should work on all platforms")

	result := os.Getenv(testVar)
	assert.Equal(t, testValue, result, "Environment variable retrieval should work cross-platform")

	if runtime.GOOS == "windows" {
		result2 := os.Getenv(strings.ToLower(testVar))
		t.Logf("Windows case-insensitive test: %s -> %s", strings.ToLower(testVar), result2)
	} else {
		result2 := os.Getenv(strings.ToLower(testVar))
		assert.Empty(t, result2, "Unix systems should be case-sensitive for env vars")
	}
}

func TestFilePermissionsHandling(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_permissions.txt")

	err := os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err, "Creating test file should work on all platforms")

	_, err = os.Stat(testFile)
	require.NoError(t, err, "File should exist after creation")

	if runtime.GOOS != "windows" {
		err = os.Chmod(testFile, 0o755)
		require.NoError(t, err, "Chmod should work on Unix-like systems")

		info, err := os.Stat(testFile)
		require.NoError(t, err)

		mode := info.Mode().Perm()
		assert.Equal(t, os.FileMode(0o755), mode, "File permissions should be set correctly on Unix")
	} else {
		t.Log("Windows: Skipping specific permission checks, testing basic file operations")

		content, err := os.ReadFile(testFile)
		require.NoError(t, err, "Reading file should work on Windows")
		assert.Equal(t, "test content", string(content))
	}
}

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
				return filepath.Clean("path\\to\\file.yaml")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filepath.Clean(tt.input)
			expected := tt.expected()

			assert.Equal(t, expected, result)

			parts := strings.FieldsFunc(result, func(c rune) bool {
				return c == '/' || c == '\\'
			})
			assert.Contains(t, parts, "path", "Should contain path component")
			assert.Contains(t, parts, "to", "Should contain to component")
			assert.Contains(t, parts, "file.yaml", "Should contain file component")
		})
	}
}

func TestTempDirectoryHandling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ai-rulez-test-*")
	require.NoError(t, err, "Creating temp directory should work on all platforms")
	defer os.RemoveAll(tempDir)

	assert.True(t, filepath.IsAbs(tempDir), "Temp directory should be absolute path")

	testFile := filepath.Join(tempDir, "test.yaml")
	err = os.WriteFile(testFile, []byte("test: value\n"), 0o644)
	require.NoError(t, err, "Writing to temp directory should work")

	content, err := os.ReadFile(testFile)
	require.NoError(t, err, "Reading from temp directory should work")
	assert.Contains(t, string(content), "test: value")

	subDir := filepath.Join(tempDir, "subdir")
	err = os.Mkdir(subDir, 0o755)
	require.NoError(t, err, "Creating subdirectories should work")

	info, err := os.Stat(subDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "Subdirectory should be recognized as directory")
}

func TestCurrentWorkingDirectory(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping working directory test in CI environment")
	}

	originalWd, err := os.Getwd()
	require.NoError(t, err, "Getting current working directory should work")

	assert.True(t, filepath.IsAbs(originalWd), "Current directory should be absolute")

	tempDir := t.TempDir()

	defer func() {
		_ = os.Chdir(originalWd)
	}()

	err = os.Chdir(tempDir)
	require.NoError(t, err, "Changing directory should work")

	currentWd, err := os.Getwd()
	require.NoError(t, err)

	resolvedTempDir, _ := filepath.EvalSymlinks(tempDir)
	resolvedCurrentWd, _ := filepath.EvalSymlinks(currentWd)

	assert.Equal(t, filepath.Clean(resolvedTempDir), filepath.Clean(resolvedCurrentWd))
}
