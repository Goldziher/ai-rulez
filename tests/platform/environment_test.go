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

func TestShellDetection(t *testing.T) {
	tests := []struct {
		name           string
		platform       string
		expectedShells []string
	}{
		{
			name:           "Windows shells",
			platform:       "windows",
			expectedShells: []string{"powershell", "cmd", "pwsh"},
		},
		{
			name:           "Unix shells",
			platform:       "linux",
			expectedShells: []string{"bash", "sh", "zsh"},
		},
		{
			name:           "macOS shells",
			platform:       "darwin",
			expectedShells: []string{"bash", "zsh", "sh"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.platform != runtime.GOOS {
				t.Skipf("Skipping %s test on %s platform", tt.platform, runtime.GOOS)
			}

			availableShells := []string{}
			for _, shell := range tt.expectedShells {
				if _, err := exec.LookPath(shell); err == nil {
					availableShells = append(availableShells, shell)
				}
			}

			assert.NotEmpty(t, availableShells, "At least one shell should be available")
			t.Logf("Available shells on %s: %v", runtime.GOOS, availableShells)

			for _, shell := range availableShells {
				t.Run("shell: "+shell, func(t *testing.T) {
					testBasicShellCommand(t, shell)
				})
			}
		})
	}
}

func testBasicShellCommand(t *testing.T, shell string) {
	var cmd *exec.Cmd

	switch shell {
	case "powershell", "pwsh":
		cmd = exec.Command(shell, "-Command", "echo 'test'")
	case "cmd":
		cmd = exec.Command(shell, "/c", "echo test")
	case "bash", "zsh":
		cmd = exec.Command(shell, "-c", "echo 'test'")
	case "sh":
		cmd = exec.Command(shell, "-c", "echo test")
	default:
		t.Skipf("Unknown shell: %s", shell)
		return
	}

	output, err := cmd.Output()
	require.NoError(t, err, "Basic command should work in %s", shell)

	result := strings.TrimSpace(string(output))
	assert.Contains(t, result, "test", "Should contain expected output")
}

func TestEnvironmentVariablePatterns(t *testing.T) {
	tests := []struct {
		name     string
		varName  string
		varValue string
		platform string
	}{
		{
			name:     "AI_RULEZ debug flag",
			varName:  "AI_RULEZ_DEBUG",
			varValue: "1",
			platform: "all",
		},
		{
			name:     "AI_RULEZ max agents",
			varName:  "AI_RULEZ_MAX_AGENTS",
			varValue: "5",
			platform: "all",
		},
		{
			name:     "PATH variable",
			varName:  "PATH",
			varValue: "",
			platform: "all",
		},
		{
			name:     "HOME variable",
			varName:  "HOME",
			varValue: "",
			platform: "unix",
		},
		{
			name:     "USERPROFILE variable",
			varName:  "USERPROFILE",
			varValue: "",
			platform: "windows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.platform == "unix" && runtime.GOOS == "windows" {
				t.Skip("Skipping Unix test on Windows")
			}
			if tt.platform == "windows" && runtime.GOOS != "windows" {
				t.Skip("Skipping Windows test on non-Windows")
			}

			if tt.varValue != "" {
				originalValue := os.Getenv(tt.varName)
				defer func() {
					if originalValue != "" {
						os.Setenv(tt.varName, originalValue)
					} else {
						os.Unsetenv(tt.varName)
					}
				}()

				err := os.Setenv(tt.varName, tt.varValue)
				require.NoError(t, err, "Should be able to set environment variable")

				retrievedValue := os.Getenv(tt.varName)
				assert.Equal(t, tt.varValue, retrievedValue,
					"Environment variable should be retrievable")
			} else {
				value := os.Getenv(tt.varName)
				if tt.varName == "PATH" {
					assert.NotEmpty(t, value, "PATH should not be empty")
					assert.Contains(t, value, string(os.PathListSeparator),
						"PATH should contain path separator")
				}
				t.Logf("%s = %s", tt.varName, value)
			}
		})
	}
}

func TestCommandLineEscaping(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
		platform string
	}{
		{
			name:     "Simple argument",
			args:     []string{"echo", "hello"},
			expected: "hello",
			platform: "all",
		},
		{
			name:     "Argument with spaces",
			args:     []string{"echo", "hello world"},
			expected: "hello world",
			platform: "all",
		},
		{
			name:     "Argument with quotes",
			args:     []string{"echo", `"quoted string"`},
			expected: `"quoted string"`,
			platform: "unix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.platform == "unix" && runtime.GOOS == "windows" {
				t.Skip("Skipping Unix test on Windows")
			}

			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				cmd = exec.Command("cmd", "/c", strings.Join(tt.args, " "))
			} else {
				cmd = exec.Command(tt.args[0], tt.args[1:]...)
			}

			output, err := cmd.Output()
			require.NoError(t, err, "Command should execute successfully")

			result := strings.TrimSpace(string(output))
			assert.Contains(t, result, strings.TrimSpace(tt.expected),
				"Output should contain expected content")
		})
	}
}

func TestProcessExecution(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		timeout bool
	}{
		{
			name:    "Git version check",
			command: "git",
			args:    []string{"--version"},
			timeout: false,
		},
		{
			name:    "Echo command",
			command: getEchoCommand(),
			args:    getEchoArgs("test output"),
			timeout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := exec.LookPath(tt.command)
			if err != nil {
				t.Skipf("Command not available: %s", tt.command)
			}

			cmd := exec.Command(tt.command, tt.args...)
			output, err := cmd.Output()

			require.NoError(t, err, "Command should execute: %s %v", tt.command, tt.args)
			assert.NotEmpty(t, output, "Command should produce output")

			t.Logf("Command '%s %s' output: %s",
				tt.command, strings.Join(tt.args, " "),
				strings.TrimSpace(string(output)))
		})
	}
}

func getEchoCommand() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "echo"
}

func getEchoArgs(message string) []string {
	if runtime.GOOS == "windows" {
		return []string{"/c", "echo " + message}
	}
	return []string{message}
}

func TestFileSystemPermissions(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		mode        os.FileMode
		expectError bool
	}{
		{
			name:        "Read-write file",
			mode:        0o644,
			expectError: false,
		},
		{
			name:        "Executable file",
			mode:        0o755,
			expectError: false,
		},
		{
			name:        "Read-only file",
			mode:        0o444,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := filepath.Join(tempDir, "test_"+tt.name+".txt")

			err := os.WriteFile(testFile, []byte("test content"), tt.mode)
			require.NoError(t, err, "Should create file with permissions")

			info, err := os.Stat(testFile)
			require.NoError(t, err, "File should exist")

			if runtime.GOOS != "windows" {
				assert.Equal(t, tt.mode, info.Mode().Perm(),
					"File should have expected permissions")
			} else {
				assert.False(t, info.IsDir(), "Should be a file, not directory")

				content, err := os.ReadFile(testFile)
				require.NoError(t, err, "Should be able to read file")
				assert.Equal(t, "test content", string(content))
			}
		})
	}
}

func TestPlatformSpecificPaths(t *testing.T) {
	tests := []struct {
		name         string
		pathElements []string
		platform     string
	}{
		{
			name:         "Windows config path",
			pathElements: []string{"C:", "Users", "test", ".ai-rulez", "config.yaml"},
			platform:     "windows",
		},
		{
			name:         "Unix config path",
			pathElements: []string{"", "home", "user", ".config", "ai-rulez", "config.yaml"},
			platform:     "linux",
		},
		{
			name:         "macOS config path",
			pathElements: []string{"", "Users", "developer", ".ai-rulez", "config.yaml"},
			platform:     "darwin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(tt.pathElements...)

			assert.NotEmpty(t, path, "Path should not be empty")

			dir := filepath.Dir(path)
			base := filepath.Base(path)

			assert.NotEmpty(t, dir, "Directory should not be empty")
			assert.NotEmpty(t, base, "Filename should not be empty")

			if tt.platform == runtime.GOOS {
				ext := filepath.Ext(path)
				assert.Equal(t, ".yaml", ext, "Should have correct extension")

				cleaned := filepath.Clean(path)
				assert.NotEmpty(t, cleaned, "Cleaned path should not be empty")
			}
		})
	}
}
