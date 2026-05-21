package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerator_Basic(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Create generator
	gen := NewGenerator(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify .claude directory and files were created
	assert.DirExists(t, filepath.Join(tempDir, ".claude"))
	assert.DirExists(t, filepath.Join(tempDir, ".claude", "skills"))

	// Verify main file has content
	mainFilePath := filepath.Join(tempDir, ".claude", "main.md")
	if _, err := os.Stat(mainFilePath); err == nil {
		content, err := os.ReadFile(mainFilePath)
		require.NoError(t, err)
		assert.NotEmpty(t, content)
	}
}

func TestGenerator_MultiPreset(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "multi-preset")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGenerator(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify output directories exist
	// Claude preset creates .claude directory
	assert.DirExists(t, filepath.Join(tempDir, ".claude"))

	// Cursor preset creates .cursor directory
	assert.DirExists(t, filepath.Join(tempDir, ".cursor"))

	// Windsurf preset creates .windsurf directory
	assert.DirExists(t, filepath.Join(tempDir, ".windsurf"))
}

func TestGenerator_WithDomains_DefaultProfile(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "with-domains")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGenerator(cfg)

	// Generate with default profile (should be "backend" according to config)
	err = gen.Generate("")
	require.NoError(t, err)

	// Verify output exists
	assert.DirExists(t, filepath.Join(tempDir, ".claude"))
}

func TestGenerator_WithDomains_FrontendProfile(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "with-domains")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGenerator(cfg)

	// Generate with frontend profile
	err = gen.Generate("frontend")
	require.NoError(t, err)

	// Verify output exists
	assert.DirExists(t, filepath.Join(tempDir, ".claude"))
}

func TestGenerator_WithDomains_FullProfile(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "with-domains")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGenerator(cfg)

	// Generate with full profile
	err = gen.Generate("full")
	require.NoError(t, err)

	// Verify output exists
	assert.DirExists(t, filepath.Join(tempDir, ".claude"))
}

func TestGenerator_InvalidProfile(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "with-domains")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGenerator(cfg)

	// Generate with invalid profile
	err = gen.Generate("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "profile not found")
}

func TestGenerator_Gitignore_Disabled(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config (gitignore is disabled in this fixture)
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGenerator(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify .gitignore was NOT created
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	assert.NoFileExists(t, gitignorePath)
}

func TestGenerator_Gitignore_Enabled(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Override gitignore setting to enable it
	enabled := true
	cfg.Gitignore = &enabled

	// Create generator
	gen := NewGenerator(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify .gitignore was created
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	assert.FileExists(t, gitignorePath)

	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	contentStr := string(content)

	// Generated files are listed individually; assistant directories can
	// contain user-owned files and must not be treated as fully managed.
	assert.Contains(t, contentStr, "CLAUDE.md")
}

func TestGenerator_CustomPreset_Markdown(t *testing.T) {
	// Setup
	tempDir := t.TempDir()

	// Create minimal config with custom preset
	aiRulezDir := filepath.Join(tempDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(aiRulezDir, "rules"), 0o755))

	// Create config
	configContent := `version: "3.0"
name: custom-preset-test
presets:
  - name: custom
    type: markdown
    path: custom-output.md
gitignore: false
`
	require.NoError(t, os.WriteFile(filepath.Join(aiRulezDir, "config.yaml"), []byte(configContent), 0o644))

	// Create content
	ruleContent := "# Custom Rule\n\nThis is a custom rule."
	require.NoError(t, os.WriteFile(filepath.Join(aiRulezDir, "rules", "custom.md"), []byte(ruleContent), 0o644))

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGenerator(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify output
	customPath := filepath.Join(tempDir, "custom-output.md")
	assert.FileExists(t, customPath)

	content, err := os.ReadFile(customPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Custom Rule")
}

func TestGenerator_CustomPreset_Directory(t *testing.T) {
	// Setup
	tempDir := t.TempDir()

	// Create minimal config with custom preset
	aiRulezDir := filepath.Join(tempDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(aiRulezDir, "rules"), 0o755))

	// Create config
	configContent := `version: "3.0"
name: custom-preset-test
presets:
  - name: custom-dir
    type: directory
    path: output-dir
gitignore: false
`
	require.NoError(t, os.WriteFile(filepath.Join(aiRulezDir, "config.yaml"), []byte(configContent), 0o644))

	// Create content
	ruleContent := "# Directory Rule\n\nThis is a rule for directory output."
	require.NoError(t, os.WriteFile(filepath.Join(aiRulezDir, "rules", "dir-rule.md"), []byte(ruleContent), 0o644))

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGenerator(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify output directory was created
	outputDir := filepath.Join(tempDir, "output-dir")
	assert.DirExists(t, outputDir)
}

func TestGenerator_MCPAutoGeneration(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "with-mcp")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGenerator(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify .mcp.json was automatically generated (not in presets list)
	mcpPath := filepath.Join(tempDir, ".mcp.json")
	assert.FileExists(t, mcpPath, ".mcp.json should be auto-generated even though 'mcp' is not in presets")

	// Verify content
	content, err := os.ReadFile(mcpPath)
	require.NoError(t, err)
	contentStr := string(content)

	// Should contain MCP server configuration
	assert.Contains(t, contentStr, "test-server")
	assert.Contains(t, contentStr, "mcpServers")
}

func TestGenerator_MCPAutoGeneration_NoServers(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGenerator(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify .mcp.json was NOT generated (no MCP servers configured)
	mcpPath := filepath.Join(tempDir, ".mcp.json")
	assert.NoFileExists(t, mcpPath, ".mcp.json should not be generated when no MCP servers exist")
}

func TestGenerator_Gitignore_NoAbsolutePaths(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Override gitignore setting to enable it
	enabled := true
	cfg.Gitignore = &enabled

	// Create generator
	gen := NewGenerator(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify .gitignore was created
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	assert.FileExists(t, gitignorePath)

	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	contentStr := string(content)

	// Check that no absolute paths were added
	// Absolute paths would start with / on Unix or C:\ on Windows
	lines := strings.Split(contentStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Check for absolute paths
		assert.False(t, filepath.IsAbs(line), "Found absolute path in .gitignore: %s", line)
	}
}

func TestGenerator_Gitignore_SkipsAiRulezFolder(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Override gitignore setting to enable it
	enabled := true
	cfg.Gitignore = &enabled

	// Create generator
	gen := NewGenerator(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify .gitignore was created
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	assert.FileExists(t, gitignorePath)

	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	contentStr := string(content)

	// Check that .ai-rulez is NOT in the gitignore
	assert.NotContains(t, contentStr, ".ai-rulez", ".ai-rulez folder should not be added to .gitignore")
	assert.NotContains(t, contentStr, ".ai-rulez/", ".ai-rulez/ folder should not be added to .gitignore")
}

func TestGenerator_Gitignore_IncludesMCPJSON(t *testing.T) {
	// Setup: with-mcp fixture has MCP servers configured
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "with-mcp")
	tempDir := t.TempDir()
	copyFixture(t, fixtureDir, tempDir)

	// Load config and add cursor preset (which emits .mcp.json when MCP servers exist)
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	cfg.Presets = append(cfg.Presets, config.Preset{BuiltIn: "cursor"})
	enabled := true
	cfg.Gitignore = &enabled

	// Generate
	gen := NewGenerator(cfg)
	require.NoError(t, gen.Generate("default"))

	// Verify .mcp.json was generated and gitignored
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	assert.FileExists(t, filepath.Join(tempDir, ".mcp.json"))
	assert.FileExists(t, gitignorePath)

	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	contentStr := string(content)

	// .mcp.json must appear inside the managed fence
	assert.Contains(t, contentStr, ".mcp.json", ".gitignore should list .mcp.json")
	assert.Contains(t, contentStr, "# BEGIN ai-rulez", "managed fence missing")
	assert.Contains(t, contentStr, "# END ai-rulez", "managed fence missing")
}

func TestGenerator_Gitignore_NoCrossDuplication(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic")
	tempDir := t.TempDir()
	copyFixture(t, fixtureDir, tempDir)

	// Pre-populate .gitignore with patterns the generator would otherwise add
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	preExisting := "node_modules/\n.claude/\nCLAUDE.md\n"
	require.NoError(t, os.WriteFile(gitignorePath, []byte(preExisting), 0o644))

	// Load config and enable gitignore
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)
	enabled := true
	cfg.Gitignore = &enabled

	// Generate
	gen := NewGenerator(cfg)
	require.NoError(t, gen.Generate("default"))

	// Verify no cross-fence duplication
	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	contentStr := string(content)

	// Each pre-existing pattern should appear exactly once total
	assert.Equal(t, 1, strings.Count(contentStr, ".claude/"),
		".claude/ should not be duplicated across pre-existing entries and managed fence")

	// User's pre-existing entries should still be present
	assert.Contains(t, contentStr, "node_modules/", "pre-existing entries must be preserved")
}

// Helper function to copy fixture directory to temp directory
func copyFixture(t *testing.T, src, dst string) {
	t.Helper()

	// Walk the source directory
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Calculate destination path
		dstPath := filepath.Join(dst, relPath)

		// Create directory or copy file
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Read source file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Write to destination
		return os.WriteFile(dstPath, data, info.Mode())
	})

	require.NoError(t, err)
}

// Benchmark tests
func BenchmarkGenerator_Basic(b *testing.B) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic")
	tempDir := b.TempDir()
	copyFixtureBench(b, fixtureDir, tempDir)

	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(b, err)

	gen := NewGenerator(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err = gen.Generate("default")
		require.NoError(b, err)
	}
}

func copyFixtureBench(b *testing.B, src, dst string) {
	b.Helper()

	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, data, info.Mode())
	})

	require.NoError(b, err)
}

// Test helper functions
func TestHelperFunctions(t *testing.T) {
	t.Run("splitLines", func(t *testing.T) {
		assert.Equal(t, []string{"a", "b", "c"}, splitLines("a\nb\nc"))
		assert.Equal(t, []string{"a"}, splitLines("a"))
		// Empty string returns empty slice
		result := splitLines("")
		assert.Empty(t, result)
	})

	t.Run("trimSpace", func(t *testing.T) {
		assert.Equal(t, "hello", trimSpace("  hello  "))
		assert.Equal(t, "hello", trimSpace("hello"))
		assert.Equal(t, "", trimSpace("   "))
	})

	t.Run("hasPrefix", func(t *testing.T) {
		assert.True(t, hasPrefix("hello", "hel"))
		assert.False(t, hasPrefix("hello", "world"))
		assert.True(t, hasPrefix("hello", ""))
	})

	t.Run("hasSuffix", func(t *testing.T) {
		assert.True(t, hasSuffix("hello", "llo"))
		assert.False(t, hasSuffix("hello", "world"))
		assert.True(t, hasSuffix("hello", ""))
	})

	t.Run("trimPrefix", func(t *testing.T) {
		assert.Equal(t, "world", trimPrefix("hello world", "hello "))
		assert.Equal(t, "hello", trimPrefix("hello", "world"))
	})

	t.Run("trimSuffix", func(t *testing.T) {
		assert.Equal(t, "hello", trimSuffix("hello world", " world"))
		assert.Equal(t, "hello", trimSuffix("hello", "world"))
	})

	t.Run("contains", func(t *testing.T) {
		assert.True(t, contains("hello world", "llo"))
		assert.False(t, contains("hello", "world"))
		assert.True(t, contains("hello", ""))
	})
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		pattern  string
		want     bool
	}{
		{"exact match", "file.txt", "file.txt", true},
		{"no match", "file.txt", "other.txt", false},
		{"directory pattern", "dir/file.txt", "dir/", true},
		{"glob pattern", "file.txt", "*.txt", true},
		{"glob no match", "file.txt", "*.md", false},
		{"substring", "path/to/file.txt", "file.txt", true},
		{"absolute pattern", "file.txt", "/file.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesPattern(tt.filename, tt.pattern)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Test for Issue #12: Gitignore Path Extraction Bug
func TestGitignorePathExtraction(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"single level directory", ".claude", ".claude"},
		{"nested path", ".claude/skills/test-skill", ".claude"},
		{"nested path with multiple levels", ".claude/skills/test-skill/subcategory", ".claude"},
		{"cursor directory", ".cursor", ".cursor"},
		{"custom nested path", "output/nested/path", "output"},
		{"simple file", "file.md", "file.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the fixed logic
			parts := strings.Split(filepath.Clean(tt.path), string(filepath.Separator))
			var topLevel string
			if len(parts) > 0 {
				topLevel = parts[0]
			}
			assert.Equal(t, tt.expected, topLevel, "path: %s", tt.path)
		})
	}
}

func TestIsIgnored(t *testing.T) {
	patterns := []string{"*.log", "tmp/", ".claude"}

	tests := []struct {
		filename string
		want     bool
	}{
		{"test.log", true},
		{"tmp/file.txt", true},
		{".claude", true},
		{"file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := isIgnored(tt.filename, patterns)
			assert.Equal(t, tt.want, got, "filename: %s", tt.filename)
		})
	}
}

// Test for Issue #13 (updated): default profile domains behavior with/without profiles
func TestGenerator_DefaultProfileDomainsLogic_WithAndWithoutProfiles(t *testing.T) {
	t.Run("default profile includes all domains when no profiles defined", func(t *testing.T) {
		cfg := &config.Config{
			Content: &config.ContentTree{
				Domains: map[string]*config.Domain{
					"php": {Name: "php"},
				},
			},
			Profiles: map[string][]string{},
		}

		gen := &Generator{config: cfg}

		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Len(t, content.Domains, 1)
		_, ok := content.Domains["php"]
		assert.True(t, ok, "expected php domain to be present for default profile when no profiles are defined")
	})

	t.Run("default profile only includes builtin domains when profiles are defined", func(t *testing.T) {
		cfg := &config.Config{
			Content: &config.ContentTree{
				Domains: map[string]*config.Domain{
					"php": {Name: "php"},
					"go":  {Name: "go", Builtin: true},
				},
			},
			Profiles: map[string][]string{
				"backend": {"php"},
			},
		}

		gen := &Generator{config: cfg}

		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)
		require.NotNil(t, content)

		// Should only see builtin domains on default when profiles exist
		assert.Len(t, content.Domains, 1)
		_, hasGo := content.Domains["go"]
		_, hasPHP := content.Domains["php"]
		assert.True(t, hasGo, "expected builtin go domain to be present for default profile when profiles are defined")
		assert.False(t, hasPHP, "expected non-builtin php domain to be excluded for default profile when profiles are defined")
	})

	t.Run("explicitly defined profiles[default] is honored like a named profile", func(t *testing.T) {
		cfg := &config.Config{
			Content: &config.ContentTree{
				Domains: map[string]*config.Domain{
					"php": {Name: "php"},
					"go":  {Name: "go", Builtin: true},
				},
			},
			// User explicitly wrote profiles: { "default": ["php"] }
			Profiles: map[string][]string{
				"default": {"php"},
			},
		}

		gen := &Generator{config: cfg}

		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)
		require.NotNil(t, content)

		// Explicit profiles["default"] wins: php should be present, go (builtin) also
		// present because GetContentForProfile always includes root + profile domains.
		_, hasPHP := content.Domains["php"]
		assert.True(t, hasPHP, "expected explicitly listed php domain to be present when profiles[default] is defined")
	})

	t.Run("default profile includes FromInclude domains even when profiles are defined", func(t *testing.T) {
		cfg := &config.Config{
			Content: &config.ContentTree{
				Domains: map[string]*config.Domain{
					"php":    {Name: "php"},                       // non-builtin, referenced by a profile
					"go":     {Name: "go", Builtin: true},         // builtin domain
					"python": {Name: "python", FromInclude: true}, // from external include
				},
			},
			Profiles: map[string][]string{
				"backend": {"php"},
			},
		}

		gen := &Generator{config: cfg}

		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)
		require.NotNil(t, content)

		// Should see builtin and FromInclude domains on default when profiles exist
		assert.Len(t, content.Domains, 2)
		_, hasGo := content.Domains["go"]
		_, hasPHP := content.Domains["php"]
		_, hasPython := content.Domains["python"]
		assert.True(t, hasGo, "expected builtin go domain to be present for default profile when profiles are defined")
		assert.False(t, hasPHP, "expected non-builtin php domain to be excluded for default profile when profiles are defined")
		assert.True(t, hasPython, "expected FromInclude python domain to be present for default profile when profiles are defined")
	})
}

// Test MCP server collection from root config.
func TestGenerator_DefaultProfile_MCPServers_WithAndWithoutProfiles(t *testing.T) {
	t.Run("collectMCPServersForContent includes enabled root servers", func(t *testing.T) {
		rootEnabled := true

		cfg := &config.Config{
			MCPServers: map[string]*config.MCPServer{
				"root-server": {
					Name:    "root-server",
					Enabled: &rootEnabled,
				},
			},
			Content: &config.ContentTree{
				Domains: map[string]*config.Domain{
					"php": {
						Name: "php",
					},
				},
			},
			Profiles: map[string][]string{},
		}

		gen := &Generator{config: cfg}

		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)

		servers := gen.collectMCPServersForContent(content)
		require.NotNil(t, servers)

		// Should contain root MCP servers
		_, hasRoot := servers["root-server"]
		assert.True(t, hasRoot, "expected root MCP server to be included")
	})

	t.Run("collectMCPServersForContent excludes disabled root servers", func(t *testing.T) {
		rootEnabled := false

		cfg := &config.Config{
			MCPServers: map[string]*config.MCPServer{
				"root-server": {
					Name:    "root-server",
					Enabled: &rootEnabled,
				},
			},
			Content: &config.ContentTree{
				Domains: map[string]*config.Domain{
					"php": {
						Name: "php",
					},
				},
			},
			Profiles: map[string][]string{},
		}

		gen := &Generator{config: cfg}

		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)

		servers := gen.collectMCPServersForContent(content)
		require.NotNil(t, servers)

		// Should not contain disabled server
		_, hasRoot := servers["root-server"]
		assert.False(t, hasRoot, "expected disabled MCP server to be excluded")
	})

	t.Run("collectMCPServersForContent collects multiple enabled servers", func(t *testing.T) {
		enabled := true

		cfg := &config.Config{
			MCPServers: map[string]*config.MCPServer{
				"github": {
					Name:    "github",
					Enabled: &enabled,
				},
				"postgres": {
					Name:    "postgres",
					Enabled: &enabled,
				},
			},
			Content: &config.ContentTree{
				Domains: map[string]*config.Domain{},
			},
			Profiles: map[string][]string{},
		}

		gen := &Generator{config: cfg}
		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)

		servers := gen.collectMCPServersForContent(content)
		require.NotNil(t, servers)

		// Should contain all enabled servers
		assert.Len(t, servers, 2, "expected both enabled servers to be collected")
		_, hasGitHub := servers["github"]
		_, hasPostgres := servers["postgres"]
		assert.True(t, hasGitHub, "expected github server to be included")
		assert.True(t, hasPostgres, "expected postgres server to be included")
	})
}

func TestGenerator_InvalidProfile_SortedHint(t *testing.T) {
	cfg := &config.Config{
		Profiles: map[string][]string{
			"zebra":  {},
			"alpha":  {},
			"middle": {},
		},
		Content: &config.ContentTree{
			Rules: []config.ContentFile{
				{Path: "rule.md", Content: "content"},
			},
		},
	}

	gen := NewGenerator(cfg)
	err := gen.Generate("nonexistent")
	require.Error(t, err)

	var oopsErr oops.OopsError
	require.ErrorAs(t, err, &oopsErr)
	hint := oopsErr.Hint()
	// Profiles should appear in sorted order in the hint
	alphaIdx := strings.Index(hint, "alpha")
	middleIdx := strings.Index(hint, "middle")
	zebraIdx := strings.Index(hint, "zebra")
	assert.True(t, alphaIdx < middleIdx && middleIdx < zebraIdx, "available profiles in hint should be sorted alphabetically, got: %s", hint)
}

func TestExtractContentHash(t *testing.T) {
	t.Run("should_return_hash_from_html_comment_header", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.md")
		content := "<!--\nProject: test\nContent-Hash: sha256:abc123\n-->\n\n# Body"
		require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o644))

		hash := extractContentHash(tmpFile)
		assert.Equal(t, "sha256:abc123", hash)
	})

	t.Run("should_return_hash_from_hash_comment_header", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.yaml")
		content := "# Project: test\n# Content-Hash: sha256:def456\n\nbody"
		require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o644))

		hash := extractContentHash(tmpFile)
		assert.Equal(t, "sha256:def456", hash)
	})

	t.Run("should_return_hash_from_slash_comment_header", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.json")
		content := "// Project: test\n// Content-Hash: sha256:ghi789\n\n{}"
		require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o644))

		hash := extractContentHash(tmpFile)
		assert.Equal(t, "sha256:ghi789", hash)
	})

	t.Run("should_return_empty_when_no_hash_present", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.md")
		content := "<!--\nProject: test\n-->\n\n# Body"
		require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o644))

		hash := extractContentHash(tmpFile)
		assert.Equal(t, "", hash)
	})

	t.Run("should_return_empty_when_file_does_not_exist", func(t *testing.T) {
		hash := extractContentHash(filepath.Join(t.TempDir(), "nonexistent.md"))
		assert.Equal(t, "", hash)
	})
}

func TestStripHeader(t *testing.T) {
	t.Run("should_strip_html_comment_header_from_markdown", func(t *testing.T) {
		content := "<!--\nHeader line\n-->\n\n# Body content"
		body := stripHeader(content, "output.md")
		assert.Equal(t, "# Body content", body)
	})

	t.Run("should_strip_hash_comment_header_from_yaml", func(t *testing.T) {
		content := "# Comment 1\n# Comment 2\n\nbody: value"
		body := stripHeader(content, "output.yaml")
		assert.Equal(t, "body: value", body)
	})

	t.Run("should_strip_slash_comment_header_from_json", func(t *testing.T) {
		content := "// Comment 1\n// Comment 2\n\n{\"key\": \"value\"}"
		body := stripHeader(content, "output.json")
		assert.Equal(t, "{\"key\": \"value\"}", body)
	})

	t.Run("should_return_full_content_when_no_header", func(t *testing.T) {
		content := "# Body content"
		body := stripHeader(content, "output.md")
		assert.Equal(t, "# Body content", body)
	})
}

func TestInjectContentHash(t *testing.T) {
	t.Run("should_inject_hash_into_html_comment_header", func(t *testing.T) {
		content := "<!--\nProject: test\n-->\n\n# Body"
		result := injectContentHash(content, "output.md", "sha256:abc123")
		assert.Contains(t, result, "Content-Hash: sha256:abc123")
		assert.Contains(t, result, "# Body")
	})

	t.Run("should_inject_hash_into_hash_comment_header", func(t *testing.T) {
		content := "# Project: test\n\nbody"
		result := injectContentHash(content, "output.yaml", "sha256:def456")
		assert.Contains(t, result, "# Content-Hash: sha256:def456")
		assert.Contains(t, result, "body")
	})

	t.Run("should_inject_hash_into_slash_comment_header", func(t *testing.T) {
		content := "// Project: test\n\n{}"
		result := injectContentHash(content, "output.json", "sha256:ghi789")
		assert.Contains(t, result, "// Content-Hash: sha256:ghi789")
		assert.Contains(t, result, "{}")
	})

	t.Run("should_roundtrip_inject_then_extract_for_html", func(t *testing.T) {
		content := "<!--\nProject: test\nGenerated: 2025\n-->\n\n# Body"
		hash := "sha256:roundtrip123"
		injected := injectContentHash(content, "output.md", hash)

		tmpFile := filepath.Join(t.TempDir(), "output.md")
		require.NoError(t, os.WriteFile(tmpFile, []byte(injected), 0o644))

		extracted := extractContentHash(tmpFile)
		assert.Equal(t, hash, extracted, "round-trip: inject then extract should return the same hash")
	})

	t.Run("should_roundtrip_inject_then_extract_for_yaml", func(t *testing.T) {
		content := "# Project: test\n# Generated: 2025\n\nbody: value"
		hash := "sha256:roundtrip456"
		injected := injectContentHash(content, "output.yaml", hash)

		tmpFile := filepath.Join(t.TempDir(), "output.yaml")
		require.NoError(t, os.WriteFile(tmpFile, []byte(injected), 0o644))

		extracted := extractContentHash(tmpFile)
		assert.Equal(t, hash, extracted, "round-trip: inject then extract should return the same hash")
	})
}

func TestGenerator_SkipUnchangedFiles(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)

	// Create generator and generate first time
	gen := NewGenerator(cfg)
	err = gen.Generate("default")
	require.NoError(t, err)

	// Find a generated file and record its mod time
	mainFilePath := filepath.Join(tempDir, ".claude", "main.md")
	if _, statErr := os.Stat(mainFilePath); statErr != nil {
		// If main.md doesn't exist, look for CLAUDE.md
		mainFilePath = filepath.Join(tempDir, "CLAUDE.md")
	}

	firstContent, err := os.ReadFile(mainFilePath)
	require.NoError(t, err)
	assert.Contains(t, string(firstContent), "Content-Hash: blake3:")

	// Extract the hash from the first generation
	firstHash := extractContentHash(mainFilePath)
	require.NotEmpty(t, firstHash, "first generation should contain a content hash")

	// Re-load config and generate again (content unchanged)
	cfg2, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)
	gen2 := NewGenerator(cfg2)
	err = gen2.Generate("default")
	require.NoError(t, err)

	// The file content should be identical (hash matches, so skip)
	secondContent, err := os.ReadFile(mainFilePath)
	require.NoError(t, err)
	assert.Equal(t, string(firstContent), string(secondContent),
		"file should not be rewritten when content hash matches")
}

// TestComputeSourceHash_IncludesSkillResources verifies that changing a
// skill's bundled reference busts the source hash. Without this, the
// generator's "skip unchanged" optimisation would leave the SKILL.md
// resource index stale after a reference is added or modified.
func TestComputeSourceHash_IncludesSkillResources(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Name: "x"}
	skill := config.ContentFile{Name: "demo", Path: "skills/demo/SKILL.md", Content: "body"}

	withRef := skill
	withRef.Resources = []config.SkillResource{{
		Kind: config.SkillKindReferences, RelPath: "references/api.md",
		Content: []byte("v1\n"),
	}}

	updatedRef := skill
	updatedRef.Resources = []config.SkillResource{{
		Kind: config.SkillKindReferences, RelPath: "references/api.md",
		Content: []byte("v2\n"),
	}}

	bare := computeSourceHash(cfg, &config.ContentTree{Skills: []config.ContentFile{skill}})
	withV1 := computeSourceHash(cfg, &config.ContentTree{Skills: []config.ContentFile{withRef}})
	withV2 := computeSourceHash(cfg, &config.ContentTree{Skills: []config.ContentFile{updatedRef}})

	assert.NotEqual(t, bare, withV1, "adding a resource must change the hash")
	assert.NotEqual(t, withV1, withV2, "changing resource content must change the hash")
}

// TestComputeSourceHash_ResistsResourceDelimiterCollision guards against a
// class of collision where one resource's body could spoof a second
// resource record by happening to contain the inter-record delimiter.
// Resolution is to length-prefix each resource's content during hashing.
func TestComputeSourceHash_ResistsResourceDelimiterCollision(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Name: "x"}

	// Single resource whose content spells out a fake second resource
	// using the on-disk hash format.
	one := config.ContentFile{
		Name: "demo", Path: "skills/demo/SKILL.md", Content: "body",
		Resources: []config.SkillResource{{
			Kind: config.SkillKindReferences, RelPath: "a",
			Content: []byte("b|res=references:c:d"),
		}},
	}
	// Two resources whose serialization, without length prefixing, would
	// look identical to `one` above.
	two := config.ContentFile{
		Name: "demo", Path: "skills/demo/SKILL.md", Content: "body",
		Resources: []config.SkillResource{
			{Kind: config.SkillKindReferences, RelPath: "a", Content: []byte("b")},
			{Kind: config.SkillKindReferences, RelPath: "c", Content: []byte("d")},
		},
	}

	hashOne := computeSourceHash(cfg, &config.ContentTree{Skills: []config.ContentFile{one}})
	hashTwo := computeSourceHash(cfg, &config.ContentTree{Skills: []config.ContentFile{two}})
	assert.NotEqual(t, hashOne, hashTwo,
		"adjacent resource records must not be spoofable from another resource's content")
}

// TestGenerator_CleansStaleSkillResource verifies that when a skill drops a
// reference file between two generations, the stale file is removed from the
// rendered skill directory. Manifest ownership keeps this cleanup scoped to
// files ai-rulez previously generated.
func TestGenerator_CleansStaleSkillResource(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	skillRoot := filepath.Join(tempDir, ".ai-rulez", "skills", "demo")
	refsDir := filepath.Join(skillRoot, "references")
	require.NoError(t, os.MkdirAll(refsDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(skillRoot, "SKILL.md"),
		[]byte("---\ndescription: demo\n---\nbody\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(refsDir, "keep.md"), []byte("keep\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(refsDir, "drop.md"), []byte("drop\n"), 0o644))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, ".ai-rulez", "config.yaml"),
		[]byte("version: \"3.0\"\nname: x\npresets:\n  - claude\ngitignore: false\n"), 0o644))

	ctx := context.Background()
	cfg, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate("default"))

	// First generation: both reference files present.
	keepOut := filepath.Join(tempDir, ".claude", "skills", "demo", "references", "keep.md")
	dropOut := filepath.Join(tempDir, ".claude", "skills", "demo", "references", "drop.md")
	require.FileExists(t, keepOut)
	require.FileExists(t, dropOut)

	// Drop one reference at the source.
	require.NoError(t, os.Remove(filepath.Join(refsDir, "drop.md")))

	// Re-load and regenerate.
	cfg2, err := config.LoadConfig(ctx, tempDir)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg2).Generate("default"))

	// Stale reference must be cleaned up; the surviving one must remain.
	require.FileExists(t, keepOut)
	_, statErr := os.Stat(dropOut)
	assert.True(t, os.IsNotExist(statErr),
		"expected stale reference to be removed, but it still exists at %s", dropOut)
}

func TestGenerator_PreservesUserOwnedAssistantFiles(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "config.yaml"),
		[]byte("version: \"4.0\"\nname: x\npresets:\n  - claude\n  - codex\ngitignore: false\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "rules", "keep.md"),
		[]byte("---\npriority: high\n---\n# Keep\n\nGenerated rule\n"),
		0o644,
	))

	userFiles := []string{
		filepath.Join(tempDir, ".claude", "settings.local.json"),
		filepath.Join(tempDir, ".claude", "hooks", "pre-tool-use.sh"),
		filepath.Join(tempDir, ".claude", "skills", "manual", "SKILL.md"),
		filepath.Join(tempDir, ".codex", "hooks", "local-tool", "script.sh"),
	}
	for _, path := range userFiles {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("user-owned\n"), 0o644))
	}

	cfg, err := config.LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate("default"))

	for _, path := range userFiles {
		require.FileExists(t, path)
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "user-owned\n", string(content))
	}
	require.FileExists(t, filepath.Join(configDir, generatedManifestName))
}

func TestGenerator_DryRunPlansWritesAndDeletesWithoutMutation(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "config.yaml"),
		[]byte("version: \"4.0\"\nname: x\npresets:\n  - codex\ngitignore: false\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "rules", "current.md"),
		[]byte("---\npriority: high\n---\n# Current\n\nCurrent rule\n"),
		0o644,
	))

	stalePath := filepath.Join(tempDir, "STALE.md")
	require.NoError(t, os.WriteFile(stalePath, []byte("stale\n"), 0o644))
	manifest := generatedManifest{Version: "1", Files: []string{"STALE.md"}}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, generatedManifestName), data, 0o644))

	cfg, err := config.LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)
	plan, err := NewGenerator(cfg).DryRun("default")
	require.NoError(t, err)

	joined := strings.Join(plan, "\n")
	assert.Contains(t, joined, "write-file: AGENTS.md")
	assert.Contains(t, joined, "delete-stale: STALE.md")
	require.FileExists(t, stalePath)
	assert.NoFileExists(t, filepath.Join(tempDir, "AGENTS.md"))
}

func TestGenerator_ScopedOutputsUseScopedProfiles(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "rules"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "domains", "frontend", "rules"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "config.toml"),
		[]byte(`version = "4.0"
name = "scoped"
presets = ["codex"]
gitignore = false

[profiles]
frontend = ["frontend"]

[[scopes]]
path = "apps/web"
profile = "frontend"
presets = ["codex", "claude"]
`),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "rules", "root.md"),
		[]byte("---\npriority: high\n---\n# Root Rule\n\nROOT_ONLY_RULE\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "domains", "frontend", "rules", "component.md"),
		[]byte("---\npriority: high\n---\n# Component Rule\n\nFRONTEND_SCOPED_RULE\n"),
		0o644,
	))

	cfg, err := config.LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate("default"))

	rootAgents := filepath.Join(tempDir, "AGENTS.md")
	scopedAgents := filepath.Join(tempDir, "apps", "web", "AGENTS.md")
	scopedClaude := filepath.Join(tempDir, "apps", "web", "CLAUDE.md")
	require.FileExists(t, rootAgents)
	require.FileExists(t, scopedAgents)
	require.FileExists(t, scopedClaude)

	rootContent, err := os.ReadFile(rootAgents)
	require.NoError(t, err)
	assert.Contains(t, string(rootContent), "ROOT_ONLY_RULE")
	assert.NotContains(t, string(rootContent), "FRONTEND_SCOPED_RULE")

	scopedContent, err := os.ReadFile(scopedAgents)
	require.NoError(t, err)
	assert.Contains(t, string(scopedContent), "ROOT_ONLY_RULE")
	assert.Contains(t, string(scopedContent), "FRONTEND_SCOPED_RULE")
}

// TestGenerator_writeOutput_RawContent_SkipsHeaderInjection verifies that
// OutputFile.RawContent bypasses the standard header banner and
// trailing-newline normalization. Skill resources (Python scripts,
// binary assets) must be written verbatim — anything else corrupts them.
func TestGenerator_writeOutput_RawContent_SkipsHeaderInjection(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	gen := &Generator{config: &config.Config{BaseDir: tempDir, SourceHash: "blake3:test"}}

	t.Run("text payload bytes round-trip exactly", func(t *testing.T) {
		t.Parallel()
		// No trailing newline — writer must not add one in raw mode.
		payload := []byte("#!/bin/sh\necho hi")
		path := filepath.Join(tempDir, "scripts", "run.sh")
		require.NoError(t, gen.writeOutput(config.OutputFile{
			Path:       path,
			RawContent: payload,
		}))

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, payload, got)
		assert.NotContains(t, string(got), "AI-RULEZ",
			"raw outputs must not have the generated-file banner")
	})

	t.Run("binary payload round-trips byte-for-byte", func(t *testing.T) {
		t.Parallel()
		payload := []byte{0x00, 0xff, 0x10, 0x20, 0x00, 0xfe}
		path := filepath.Join(tempDir, "assets", "blob.bin")
		require.NoError(t, gen.writeOutput(config.OutputFile{
			Path:       path,
			RawContent: payload,
		}))

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, payload, got)
	})

	t.Run("creates parent directories on demand", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(tempDir, "deep", "nested", "x.txt")
		require.NoError(t, gen.writeOutput(config.OutputFile{
			Path:       path,
			RawContent: []byte("ok"),
		}))
		_, err := os.Stat(path)
		assert.NoError(t, err)
	})

	t.Run("preserves explicit file mode", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(tempDir, "exec", "run.sh")
		require.NoError(t, gen.writeOutput(config.OutputFile{
			Path:       path,
			RawContent: []byte("#!/bin/sh\n"),
			Mode:       0o755,
		}))
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})

	t.Run("zero mode falls back to 0o644", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(tempDir, "default-mode", "x.md")
		require.NoError(t, gen.writeOutput(config.OutputFile{
			Path:       path,
			RawContent: []byte("ok"),
		}))
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	})

	t.Run("skips write when raw content and mode are unchanged", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(tempDir, "idempotent", "x.bin")
		require.NoError(t, gen.writeOutput(config.OutputFile{
			Path: path, RawContent: []byte("payload"), Mode: 0o644,
		}))

		// Pin the file's mtime to a sentinel value far in the past. If the
		// writer rewrites, the mtime jumps to "now". Comparing equality of
		// the sentinel to the mtime after the second call is robust on
		// filesystems with second-level mtime resolution (where two
		// back-to-back writes would otherwise produce identical mtimes).
		sentinel := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		require.NoError(t, os.Chtimes(path, sentinel, sentinel))

		require.NoError(t, gen.writeOutput(config.OutputFile{
			Path: path, RawContent: []byte("payload"), Mode: 0o644,
		}))
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.True(t, info.ModTime().Equal(sentinel),
			"unchanged raw output should not be rewritten; mtime moved from %s to %s",
			sentinel, info.ModTime())
	})

	t.Run("rewrites when raw content changes", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(tempDir, "changed-content", "x.bin")
		require.NoError(t, gen.writeOutput(config.OutputFile{
			Path: path, RawContent: []byte("first"), Mode: 0o644,
		}))
		require.NoError(t, gen.writeOutput(config.OutputFile{
			Path: path, RawContent: []byte("second"), Mode: 0o644,
		}))
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, []byte("second"), got)
	})

	t.Run("rewrites when only mode changes", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(tempDir, "changed-mode", "x.sh")
		require.NoError(t, gen.writeOutput(config.OutputFile{
			Path: path, RawContent: []byte("ok"), Mode: 0o644,
		}))
		require.NoError(t, gen.writeOutput(config.OutputFile{
			Path: path, RawContent: []byte("ok"), Mode: 0o755,
		}))
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})
}
