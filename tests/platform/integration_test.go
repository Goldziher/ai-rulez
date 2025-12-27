package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAIRulezCrossplatformBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping build test in short mode")
	}

	projectRoot := findProjectRoot(t)

	cmd := exec.Command("go", "build", "-o", getBinaryName("ai-rulez-test"), "./cmd")
	cmd.Dir = projectRoot

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Building ai-rulez should succeed on %s: %s", runtime.GOOS, string(output))

	binaryPath := filepath.Join(projectRoot, getBinaryName("ai-rulez-test"))
	_, err = os.Stat(binaryPath)
	require.NoError(t, err, "Binary should exist after build")

	defer os.Remove(binaryPath)

	cmd = exec.Command(binaryPath, "version")
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Binary should execute successfully, output: %s", string(output))

	outputStr := string(output)
	assert.Contains(t, outputStr, "ai-rulez", "Version output should contain ai-rulez, actual: %q", outputStr)
}

func TestAIRulezBasicCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectRoot := findProjectRoot(t)
	tempDir := t.TempDir()

	binaryName := getBinaryName("ai-rulez-platform-test")
	binaryPath := filepath.Join(tempDir, binaryName)

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd")
	cmd.Dir = projectRoot

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Should build ai-rulez: %s", string(output))

	err = os.Chdir(tempDir)
	require.NoError(t, err, "Should change to temp directory")
	defer os.Chdir(projectRoot)

	testCases := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "Version command",
			args:        []string{"version"},
			expectError: false,
		},
		{
			name:        "Help command",
			args:        []string{"--help"},
			expectError: false,
		},
		{
			name:        "Validate without config",
			args:        []string{"validate"},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(binaryPath, tc.args...)
			output, err := cmd.CombinedOutput()

			if tc.expectError {
				assert.Error(t, err, "Command should fail: %s", tc.name)
			} else {
				assert.NoError(t, err, "Command should succeed: %s, output: %s", tc.name, string(output))
				assert.NotEmpty(t, output, "Command should produce output")
			}

			t.Logf("Command '%s %s' output: %s",
				binaryName, strings.Join(tc.args, " "), string(output))
		})
	}
}

func TestConfigFileHandlingCrossPlatform(t *testing.T) {
	tempDir := t.TempDir()

	configs := []struct {
		name     string
		filename string
		content  string
	}{
		{
			name:     "Standard config",
			filename: "ai_rulez.yaml",
			content: `
metadata:
  name: Test Project
  version: "1.0.0"

rules:
  - name: Test Rule
    priority: medium
    content: Test rule content

outputs:
  - path: CLAUDE.md
`,
		},
		{
			name:     "Config with Windows paths",
			filename: "windows_config.yaml",
			content: `
metadata:
  name: Windows Test

outputs:
  - path: "C:\\output\\CLAUDE.md"
  - path: ".cursor\\rules\\"
    type: section
    naming_scheme: "{name}.md"
`,
		},
		{
			name:     "Config with Unix paths",
			filename: "unix_config.yaml",
			content: `
metadata:
  name: Unix Test

outputs:
  - path: "/tmp/output/CLAUDE.md"
  - path: ".cursor/rules/"
    type: section
    naming_scheme: "{name}.md"
`,
		},
	}

	for _, config := range configs {
		t.Run(config.name, func(t *testing.T) {
			configPath := filepath.Join(tempDir, config.filename)

			err := os.WriteFile(configPath, []byte(config.content), 0o644)
			require.NoError(t, err, "Should write config file")

			content, err := os.ReadFile(configPath)
			require.NoError(t, err, "Should read config file")

			assert.Equal(t, strings.TrimSpace(config.content),
				strings.TrimSpace(string(content)), "Content should match")

			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.Contains(line, "path:") && !strings.Contains(config.name, "Windows") {
					assert.NotContains(t, line, "\\\\", "Paths should not have double backslashes")
				}
			}
		})
	}
}

func TestOutputDirectoryCreation(t *testing.T) {
	tempDir := t.TempDir()

	outputPaths := []struct {
		name string
		path string
	}{
		{
			name: "Simple file",
			path: "CLAUDE.md",
		},
		{
			name: "Hidden directory",
			path: ".cursor/rules/general.md",
		},
		{
			name: "Nested structure",
			path: "docs/ai-rules/sections/setup.md",
		},
		{
			name: "Mixed separators",
			path: filepath.Join(".continue", "prompts", "ai_rulez_prompts.yaml"),
		},
	}

	for _, output := range outputPaths {
		t.Run(output.name, func(t *testing.T) {
			fullPath := filepath.Join(tempDir, output.path)
			dir := filepath.Dir(fullPath)

			err := os.MkdirAll(dir, 0o755)
			require.NoError(t, err, "Should create directory structure")

			content := "# Test Content\nGenerated on " + runtime.GOOS
			err = os.WriteFile(fullPath, []byte(content), 0o644)
			require.NoError(t, err, "Should write output file")

			readContent, err := os.ReadFile(fullPath)
			require.NoError(t, err, "Should read output file")
			assert.Equal(t, content, string(readContent), "Content should match")

			assert.True(t, filepath.IsAbs(fullPath), "Full path should be absolute")

			rel, err := filepath.Rel(tempDir, fullPath)
			require.NoError(t, err, "Should compute relative path")

			expectedRel := filepath.FromSlash(output.path)
			assert.Equal(t, expectedRel, rel, "Relative path should match")
		})
	}
}

func TestGitIntegrationCrossPlatform(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tempDir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Cleanup(func() {
			time.Sleep(100 * time.Millisecond)
			runtime.GC()
		})
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = tempDir
	_, err := cmd.Output()
	require.NoError(t, err, "Should initialize git repo")

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tempDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tempDir
	cmd.Run()

	testFile := filepath.Join(tempDir, "README.md")
	err = os.WriteFile(testFile, []byte("# Test Repo"), 0o644)
	require.NoError(t, err)

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = tempDir
	_, err = cmd.Output()
	require.NoError(t, err, "Should add file to git")

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tempDir
	_, err = cmd.Output()
	require.NoError(t, err, "Should commit file")

	gitCommands := []struct {
		name string
		args []string
	}{
		{
			name: "Check git dir",
			args: []string{"rev-parse", "--git-dir"},
		},
		{
			name: "Count commits",
			args: []string{"rev-list", "--count", "HEAD"},
		},
		{
			name: "Recent commits",
			args: []string{"log", "--oneline", "-n", "1", "--format=%s"},
		},
	}

	for _, gitCmd := range gitCommands {
		t.Run(gitCmd.name, func(t *testing.T) {
			cmd := exec.Command("git", gitCmd.args...)
			cmd.Dir = tempDir
			output, err := cmd.Output()
			require.NoError(t, err, "Git command should work: %s", strings.Join(gitCmd.args, " "))
			assert.NotEmpty(t, output, "Git command should produce output")

			t.Logf("Git command '%s' output: %s",
				strings.Join(gitCmd.args, " "), strings.TrimSpace(string(output)))
		})
	}
}

func findProjectRoot(t *testing.T) string {
	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find project root (no go.mod found)")
		}
		dir = parent
	}
}

func getBinaryName(baseName string) string {
	if runtime.GOOS == "windows" {
		return baseName + ".exe"
	}
	return baseName
}
