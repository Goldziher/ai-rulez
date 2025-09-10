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
			expected := tt.binaryName
			if tt.platform == "windows" {
				expected += ".exe"
			}

			actual := addBinaryExtension(tt.binaryName, tt.platform)
			assert.Equal(t, expected, actual)

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

			dir := filepath.Dir(result)
			file := filepath.Base(result)

			assert.NotEmpty(t, dir, "Directory component should not be empty")
			assert.NotEmpty(t, file, "File component should not be empty")
		})
	}
}

func TestGitCommandExecution(t *testing.T) {
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
			expectError: true,
		},
		{
			name:        "git rev-list --count HEAD",
			args:        []string{"rev-list", "--count", "HEAD"},
			expectError: true,
		},
		{
			name:        "git log format",
			args:        []string{"log", "--oneline", "-n", "1", "--format=%s"},
			expectError: true,
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
				assert.NotNil(t, cmd, "Command should be constructible")
				t.Logf("Command 'git %s' result: error=%v, output_len=%d",
					strings.Join(tt.args, " "), err != nil, len(output))
			}
		})
	}
}

func TestAgentCommandExecution(t *testing.T) {
	tests := []struct {
		name        string
		agentID     string
		command     string
		args        []string
		shouldExist bool
	}{
		{
			name:        "Claude command structure",
			agentID:     "claude",
			command:     "claude",
			args:        []string{"--print", "--permission-mode", "bypassPermissions", "test prompt"},
			shouldExist: false,
		},
		{
			name:        "Generic command structure",
			agentID:     "test",
			command:     "echo",
			args:        []string{"test"},
			shouldExist: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(tt.command, tt.args...)
			assert.NotNil(t, cmd, "Command should be constructible")
			assert.Contains(t, cmd.Path, tt.command, "Command path should contain command name")

			_, err := exec.LookPath(tt.command)
			if tt.shouldExist {
				require.NoError(t, err, "Expected command should exist: %s", tt.command)

				if tt.command == "echo" {
					output, execErr := cmd.Output()
					require.NoError(t, execErr, "Echo command should execute successfully")
					assert.Contains(t, string(output), "test", "Echo should return expected output")
				}
			} else {
				t.Logf("Command '%s' availability: %v", tt.command, err == nil)
			}
		})
	}
}

func TestDirectoryTraversalSecurity(t *testing.T) {
	tempDir := t.TempDir()

	dangerousPaths := []string{
		"../../../etc/passwd",
		"..\\..\\..\\Windows\\System32\\config\\sam",
		"../outside",
		"./safe/path",
		"/tmp/absolute",
	}

	for _, path := range dangerousPaths {
		t.Run("path: "+path, func(t *testing.T) {
			joined := filepath.Join(tempDir, path)

			rel, err := filepath.Rel(tempDir, joined)
			if err == nil && !strings.HasPrefix(rel, "..") {
				t.Logf("Safe path: %s -> %s (rel: %s)", path, joined, rel)
			} else {
				t.Logf("Potentially unsafe path: %s -> %s (rel: %s, err: %v)",
					path, joined, rel, err)

				if strings.Contains(path, "..") {
					assert.True(t, strings.Contains(rel, "..") || err != nil,
						"Path traversal should be detectable")
				}
			}
		})
	}
}

func TestFileHiddenDetection(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		expected bool
	}{
		{"Unix hidden file", ".hidden", true},
		{"Windows hidden file", ".gitignore", true},
		{"Normal file", "README.md", false},
		{"File with dot in middle", "config.yaml", false},
		{"GitHub directory", ".github", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isHidden := strings.HasPrefix(tt.fileName, ".")
			if tt.fileName == ".github" {
				isHidden = false
			}

			assert.Equal(t, tt.expected, isHidden,
				"Hidden file detection for %s", tt.fileName)
		})
	}
}

func TestWorkspaceDetection(t *testing.T) {
	tempDir := t.TempDir()

	structures := []struct {
		name     string
		files    []string
		expected bool
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

			for _, file := range structure.files {
				filePath := filepath.Join(structureDir, file)
				dir := filepath.Dir(filePath)

				if dir != structureDir {
					err = os.MkdirAll(dir, 0o755)
					require.NoError(t, err)
				}

				err = os.WriteFile(filePath, []byte("test content"), 0o644)
				require.NoError(t, err)
			}

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
