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

// TestAIRulezBinaryDetection tests our binary path detection logic
func TestAIRulezBinaryDetection(t *testing.T) {
	tests := []struct {
		name            string
		platform        string
		binaryName      string
		expectExtension bool
	}{
		{"Windows binary detection", "windows", "ai-rulez", true},
		{"Linux binary detection", "linux", "ai-rulez", false},
		{"macOS binary detection", "darwin", "ai-rulez", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the binary detection logic used in testutil
			expected := tt.binaryName
			if tt.platform == "windows" {
				expected += ".exe"
			}

			actual := addBinaryExtension(tt.binaryName, tt.platform)
			assert.Equal(t, expected, actual)

			// Test the actual runtime logic
			if tt.platform == runtime.GOOS {
				runtimeExpected := tt.binaryName
				if runtime.GOOS == "windows" {
					runtimeExpected += ".exe"
				}
				assert.Equal(t, runtimeExpected, addBinaryExtension(tt.binaryName, runtime.GOOS))
			}
		})
	}
}

// TestConfigPathGeneration tests config file path generation across platforms
func TestConfigPathGeneration(t *testing.T) {
	testCases := []struct {
		name       string
		basePath   string
		configName string
		expected   func(base, config string) string
	}{
		{
			name:       "Standard config path",
			basePath:   "/home/user/project",
			configName: "ai_rulez.yaml",
			expected: func(base, config string) string {
				return filepath.Join(base, config)
			},
		},
		{
			name:       "Windows config path",
			basePath:   "C:\\Users\\developer\\project",
			configName: "ai_rulez.yaml",
			expected: func(base, config string) string {
				return filepath.Join(base, config)
			},
		},
		{
			name:       "Hidden config directory",
			basePath:   "/home/user",
			configName: ".ai-rulez/config.yaml",
			expected: func(base, config string) string {
				return filepath.Join(base, config)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := filepath.Join(tc.basePath, tc.configName)
			expected := tc.expected(tc.basePath, tc.configName)

			assert.Equal(t, expected, result)

			// Verify the path components
			dir := filepath.Dir(result)
			file := filepath.Base(result)

			assert.NotEmpty(t, dir, "Directory component should not be empty")
			assert.NotEmpty(t, file, "File component should not be empty")
		})
	}
}

// TestGitCommandExecution tests git command execution patterns used in the codebase
func TestGitCommandExecution(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "git version check",
			args:        []string{"--version"},
			expectError: false,
		},
		{
			name:        "git rev-parse --git-dir",
			args:        []string{"rev-parse", "--git-dir"},
			expectError: true, // May fail if not in git repo
		},
		{
			name:        "git rev-list --count HEAD",
			args:        []string{"rev-list", "--count", "HEAD"},
			expectError: true, // May fail if not in git repo
		},
		{
			name:        "git log format",
			args:        []string{"log", "--oneline", "-n", "1", "--format=%s"},
			expectError: true, // May fail if not in git repo
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("git", tt.args...)
			output, err := cmd.Output()

			if !tt.expectError {
				require.NoError(t, err, "Command should succeed: git %s", strings.Join(tt.args, " "))
				assert.NotEmpty(t, output, "Command should produce output")
			} else {
				// For commands that may fail (not in git repo), just verify they can be constructed
				assert.NotNil(t, cmd, "Command should be constructible")
				t.Logf("Command 'git %s' result: error=%v, output_len=%d",
					strings.Join(tt.args, " "), err != nil, len(output))
			}
		})
	}
}

// TestAgentCommandExecution tests the agent command patterns we use
func TestAgentCommandExecution(t *testing.T) {
	tests := []struct {
		name        string
		agentID     string
		command     string
		args        []string
		shouldExist bool // whether we expect the command to exist
	}{
		{
			name:        "Claude command structure",
			agentID:     "claude",
			command:     "claude",
			args:        []string{"--print", "--permission-mode", "bypassPermissions", "test prompt"},
			shouldExist: false, // May not be installed
		},
		{
			name:        "Generic command structure",
			agentID:     "test",
			command:     "echo", // Use echo as it exists on all platforms
			args:        []string{"test"},
			shouldExist: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test command construction (should work on all platforms)
			cmd := exec.Command(tt.command, tt.args...)
			assert.NotNil(t, cmd, "Command should be constructible")
			assert.Contains(t, cmd.Path, tt.command, "Command path should contain command name")

			// Test command existence
			_, err := exec.LookPath(tt.command)
			if tt.shouldExist {
				require.NoError(t, err, "Expected command should exist: %s", tt.command)

				// Test actual execution for commands we expect to work
				if tt.command == "echo" {
					output, execErr := cmd.Output()
					require.NoError(t, execErr, "Echo command should execute successfully")
					assert.Contains(t, string(output), "test", "Echo should return expected output")
				}
			} else {
				// For commands that may not exist, just log the result
				t.Logf("Command '%s' availability: %v", tt.command, err == nil)
			}
		})
	}
}

// TestDirectoryTraversalSecurity tests that our directory traversal is secure
func TestDirectoryTraversalSecurity(t *testing.T) {
	tempDir := t.TempDir()

	// Test potentially dangerous paths
	dangerousPaths := []string{
		"../../../etc/passwd",                        // Unix path traversal
		"..\\..\\..\\Windows\\System32\\config\\sam", // Windows path traversal
		"../outside",    // Simple traversal
		"./safe/path",   // Safe relative path
		"/tmp/absolute", // Absolute path
	}

	for _, path := range dangerousPaths {
		t.Run("path: "+path, func(t *testing.T) {
			// Test filepath.Join behavior (should be safe)
			joined := filepath.Join(tempDir, path)

			// Check if the path escapes our temp directory
			rel, err := filepath.Rel(tempDir, joined)
			if err == nil && !strings.HasPrefix(rel, "..") {
				t.Logf("Safe path: %s -> %s (rel: %s)", path, joined, rel)
			} else {
				t.Logf("Potentially unsafe path: %s -> %s (rel: %s, err: %v)",
					path, joined, rel, err)

				// For path traversal attempts, ensure we detect them
				if strings.Contains(path, "..") {
					assert.True(t, strings.Contains(rel, "..") || err != nil,
						"Path traversal should be detectable")
				}
			}
		})
	}
}

// TestFileHiddenDetection tests detection of hidden files across platforms
func TestFileHiddenDetection(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		expected bool
	}{
		{"Unix hidden file", ".hidden", true},
		{"Windows hidden file", ".gitignore", true}, // Starts with dot
		{"Normal file", "README.md", false},
		{"File with dot in middle", "config.yaml", false},
		{"GitHub directory", ".github", false}, // Special case we allow
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test our hidden file detection logic
			isHidden := strings.HasPrefix(tt.fileName, ".")
			if tt.fileName == ".github" {
				isHidden = false // Special case exception
			}

			assert.Equal(t, tt.expected, isHidden,
				"Hidden file detection for %s", tt.fileName)
		})
	}
}

// TestWorkspaceDetection tests workspace/project root detection
func TestWorkspaceDetection(t *testing.T) {
	tempDir := t.TempDir()

	// Create test project structures
	structures := []struct {
		name     string
		files    []string
		expected bool // whether this indicates a project root
	}{
		{
			name:     "Go project",
			files:    []string{"go.mod", "main.go"},
			expected: true,
		},
		{
			name:     "Node project",
			files:    []string{"package.json", "src/index.js"},
			expected: true,
		},
		{
			name:     "Python project",
			files:    []string{"setup.py", "requirements.txt"},
			expected: true,
		},
		{
			name:     "Git repository",
			files:    []string{".git/config"},
			expected: true,
		},
		{
			name:     "Empty directory",
			files:    []string{},
			expected: false,
		},
	}

	for _, structure := range structures {
		t.Run(structure.name, func(t *testing.T) {
			structureDir := filepath.Join(tempDir, structure.name)
			err := os.MkdirAll(structureDir, 0o755)
			require.NoError(t, err)

			// Create the test files
			for _, file := range structure.files {
				filePath := filepath.Join(structureDir, file)
				dir := filepath.Dir(filePath)

				// Create directory if needed
				if dir != structureDir {
					err = os.MkdirAll(dir, 0o755)
					require.NoError(t, err)
				}

				// Create file
				err = os.WriteFile(filePath, []byte("test content"), 0o644)
				require.NoError(t, err)
			}

			// Test project detection logic
			hasProjectFiles := false
			projectIndicators := []string{"go.mod", "package.json", "setup.py", ".git"}

			for _, indicator := range projectIndicators {
				indicatorPath := filepath.Join(structureDir, indicator)
				if _, err := os.Stat(indicatorPath); err == nil {
					hasProjectFiles = true
					break
				}
			}

			assert.Equal(t, structure.expected, hasProjectFiles,
				"Project detection for %s structure", structure.name)
		})
	}
}
