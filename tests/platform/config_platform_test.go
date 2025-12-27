package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConfigPathHandling(t *testing.T) {
	tests := []struct {
		name          string
		configPath    string
		expectedValid bool
		platform      string
	}{
		{
			name:          "Unix absolute path",
			configPath:    "/home/user/project/ai_rulez.yaml",
			expectedValid: true,
			platform:      "linux",
		},
		{
			name:          "Windows absolute path",
			configPath:    "C:\\Users\\User\\project\\ai_rulez.yaml",
			expectedValid: true,
			platform:      "windows",
		},
		{
			name:          "macOS absolute path",
			configPath:    "/Users/developer/workspace/ai_rulez.yaml",
			expectedValid: true,
			platform:      "darwin",
		},
		{
			name:          "Relative path",
			configPath:    "./ai_rulez.yaml",
			expectedValid: true,
			platform:      "any",
		},
		{
			name:          "Current directory",
			configPath:    "ai_rulez.yaml",
			expectedValid: true,
			platform:      "any",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.platform == "any" || tt.platform == runtime.GOOS {
				dir := filepath.Dir(tt.configPath)
				base := filepath.Base(tt.configPath)

				assert.NotEmpty(t, dir, "Directory component should not be empty")
				assert.NotEmpty(t, base, "Base filename should not be empty")
				assert.True(t, strings.HasSuffix(base, ".yaml"), "Should have .yaml extension")

				cleaned := filepath.Clean(tt.configPath)
				assert.NotEmpty(t, cleaned, "Cleaned path should not be empty")
			}
		})
	}
}

func TestOutputPathGeneration(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name       string
		outputPath string
		content    string
	}{
		{
			name:       "CLAUDE.md in root",
			outputPath: "CLAUDE.md",
			content:    "# AI Assistant Rules\n",
		},
		{
			name:       "Cursor rules directory",
			outputPath: ".cursor/rules/general.md",
			content:    "# General Rules\n",
		},
		{
			name:       "Continue.dev prompts",
			outputPath: ".continue/prompts/ai_rulez_prompts.yaml",
			content:    "prompts:\n  - test\n",
		},
		{
			name:       "Nested agent files",
			outputPath: ".claude/agents/specialist.md",
			content:    "# Specialist Agent\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullPath := filepath.Join(tempDir, tt.outputPath)

			dir := filepath.Dir(fullPath)
			err := os.MkdirAll(dir, 0o755)
			require.NoError(t, err, "Should be able to create directory structure")

			err = os.WriteFile(fullPath, []byte(tt.content), 0o644)
			require.NoError(t, err, "Should be able to write output file")

			content, err := os.ReadFile(fullPath)
			require.NoError(t, err, "Should be able to read output file")
			assert.Equal(t, tt.content, string(content), "File content should match")

			assert.True(t, filepath.IsAbs(fullPath), "Full path should be absolute")

			rel, err := filepath.Rel(tempDir, fullPath)
			require.NoError(t, err)
			expectedPath := filepath.FromSlash(tt.outputPath)
			assert.Equal(t, expectedPath, rel, "Relative path should match original")
		})
	}
}

func TestConfigSerializationAcrossPlatforms(t *testing.T) {
	testConfig := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":        "Cross Platform Test",
			"version":     "1.0.0",
			"description": "Testing cross-platform config handling",
		},
		"rules": []map[string]interface{}{
			{
				"name":     "File Path Handling",
				"priority": "critical",
				"content":  "Always use filepath.Join() for cross-platform path handling",
			},
		},
		"outputs": []map[string]interface{}{
			{
				"path": "CLAUDE.md",
			},
		},
	}

	yamlData, err := yaml.Marshal(testConfig)
	require.NoError(t, err, "YAML marshaling should work on all platforms")

	var parsedConfig map[string]interface{}
	err = yaml.Unmarshal(yamlData, &parsedConfig)
	require.NoError(t, err, "YAML unmarshaling should work on all platforms")

	metadata := parsedConfig["metadata"].(map[string]interface{})
	assert.Equal(t, "Cross Platform Test", metadata["name"])

	rules := parsedConfig["rules"].([]interface{})
	assert.Len(t, rules, 1)

	outputs := parsedConfig["outputs"].([]interface{})
	assert.Len(t, outputs, 1)

	tempFile := filepath.Join(t.TempDir(), "test_config.yaml")

	err = os.WriteFile(tempFile, yamlData, 0o644)
	require.NoError(t, err, "Writing config file should work")

	readData, err := os.ReadFile(tempFile)
	require.NoError(t, err, "Reading config file should work")

	var readConfig map[string]interface{}
	err = yaml.Unmarshal(readData, &readConfig)
	require.NoError(t, err, "Parsing read config should work")

	readMetadata := readConfig["metadata"].(map[string]interface{})
	assert.Equal(t, "Cross Platform Test", readMetadata["name"])
}

func TestConfigLoadingWithPlatformPaths(t *testing.T) {
	tempDir := t.TempDir()

	configDir := filepath.Join(tempDir, "configs", "test")
	err := os.MkdirAll(configDir, 0o755)
	require.NoError(t, err)

	configPath := filepath.Join(configDir, "ai_rulez.yaml")
	configData := `
metadata:
  name: Platform Test
  version: "1.0.0"

rules:
  - name: Cross Platform Rule
    priority: high
    content: Test rule content

outputs:
  - path: CLAUDE.md
`

	err = os.WriteFile(configPath, []byte(configData), 0o644)
	require.NoError(t, err)

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(configDir)
	require.NoError(t, err)

	pathTests := []string{
		"ai_rulez.yaml",
		"./ai_rulez.yaml",
		filepath.Join(".", "ai_rulez.yaml"),
	}

	for _, testPath := range pathTests {
		t.Run("path: "+testPath, func(t *testing.T) {
			cfg, err := config.LoadConfig(testPath)
			require.NoError(t, err, "Config loading should work with path: %s", testPath)

			assert.Equal(t, "Platform Test", cfg.Metadata.Name)
			assert.Len(t, cfg.Rules, 1)
			assert.Equal(t, "Cross Platform Rule", cfg.Rules[0].Name)
			assert.Len(t, cfg.Outputs, 1)
		})
	}
}

func TestHiddenDirectoryHandling(t *testing.T) {
	tempDir := t.TempDir()

	directories := []struct {
		name     string
		isHidden bool
	}{
		{".git", true},
		{".github", true},
		{".cursor", true},
		{".continue", true},
		{".claude", true},
		{"src", false},
		{"docs", false},
		{".DS_Store", true},
	}

	for _, dir := range directories {
		dirPath := filepath.Join(tempDir, dir.name)
		err := os.MkdirAll(dirPath, 0o755)
		require.NoError(t, err)

		testFile := filepath.Join(dirPath, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0o644)
		require.NoError(t, err)
	}

	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	var hiddenDirs []string
	var visibleDirs []string

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if strings.HasPrefix(name, ".") && name != ".github" {
				hiddenDirs = append(hiddenDirs, name)
			} else {
				visibleDirs = append(visibleDirs, name)
			}
		}
	}

	assert.Contains(t, hiddenDirs, ".git", "Should detect .git as hidden")
	assert.Contains(t, hiddenDirs, ".cursor", "Should detect .cursor as hidden")
	assert.Contains(t, visibleDirs, ".github", "Should treat .github as visible")
	assert.Contains(t, visibleDirs, "src", "Should detect src as visible")

	if runtime.GOOS == "darwin" {
		assert.Contains(t, hiddenDirs, ".DS_Store", "Should detect macOS .DS_Store as hidden")
	}
}

func TestTemplatePathHandling(t *testing.T) {
	tempDir := t.TempDir()

	templateDir := filepath.Join(tempDir, "templates")
	err := os.MkdirAll(templateDir, 0o755)
	require.NoError(t, err)

	templates := []struct {
		name    string
		path    string
		content string
	}{
		{
			name:    "Unix style path",
			path:    "unix/claude.md",
			content: "# Unix Template\n{{.Rules}}\n",
		},
		{
			name:    "Windows style path",
			path:    filepath.Join("windows", "claude.md"),
			content: "# Windows Template\r\n{{.Rules}}\r\n",
		},
		{
			name:    "Nested template",
			path:    filepath.Join("nested", "deep", "template.md"),
			content: "# Nested Template\n{{.Sections}}\n",
		},
	}

	for _, tmpl := range templates {
		t.Run(tmpl.name, func(t *testing.T) {
			tmplPath := filepath.Join(templateDir, tmpl.path)
			tmplDir := filepath.Dir(tmplPath)

			err := os.MkdirAll(tmplDir, 0o755)
			require.NoError(t, err, "Should create template directory structure")

			err = os.WriteFile(tmplPath, []byte(tmpl.content), 0o644)
			require.NoError(t, err, "Should write template file")

			content, err := os.ReadFile(tmplPath)
			require.NoError(t, err, "Should read template file")
			assert.Equal(t, tmpl.content, string(content), "Template content should match")

			relPath, err := filepath.Rel(templateDir, tmplPath)
			require.NoError(t, err)

			expectedRel := filepath.FromSlash(tmpl.path)
			assert.Equal(t, expectedRel, relPath, "Relative path should be normalized correctly")
		})
	}
}
