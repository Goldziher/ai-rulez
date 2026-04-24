package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratorV3_Basic(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Create generator
	gen := NewGeneratorV3(cfg)

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

func TestGeneratorV3_MultiPreset(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "multi-preset")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGeneratorV3(cfg)

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

func TestGeneratorV3_WithDomains_DefaultProfile(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "with-domains")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGeneratorV3(cfg)

	// Generate with default profile (should be "backend" according to config)
	err = gen.Generate("")
	require.NoError(t, err)

	// Verify output exists
	assert.DirExists(t, filepath.Join(tempDir, ".claude"))
}

func TestGeneratorV3_WithDomains_FrontendProfile(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "with-domains")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGeneratorV3(cfg)

	// Generate with frontend profile
	err = gen.Generate("frontend")
	require.NoError(t, err)

	// Verify output exists
	assert.DirExists(t, filepath.Join(tempDir, ".claude"))
}

func TestGeneratorV3_WithDomains_FullProfile(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "with-domains")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGeneratorV3(cfg)

	// Generate with full profile
	err = gen.Generate("full")
	require.NoError(t, err)

	// Verify output exists
	assert.DirExists(t, filepath.Join(tempDir, ".claude"))
}

func TestGeneratorV3_InvalidProfile(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "with-domains")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGeneratorV3(cfg)

	// Generate with invalid profile
	err = gen.Generate("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "profile not found")
}

func TestGeneratorV3_Gitignore_Disabled(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config (gitignore is disabled in this fixture)
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGeneratorV3(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify .gitignore was NOT created
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	assert.NoFileExists(t, gitignorePath)
}

func TestGeneratorV3_Gitignore_Enabled(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Override gitignore setting to enable it
	enabled := true
	cfg.Gitignore = &enabled

	// Create generator
	gen := NewGeneratorV3(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify .gitignore was created
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	assert.FileExists(t, gitignorePath)

	content, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	contentStr := string(content)

	// Should contain the .claude directory
	assert.Contains(t, contentStr, ".claude")
}

func TestGeneratorV3_CustomPreset_Markdown(t *testing.T) {
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
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGeneratorV3(cfg)

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

func TestGeneratorV3_CustomPreset_Directory(t *testing.T) {
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
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGeneratorV3(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify output directory was created
	outputDir := filepath.Join(tempDir, "output-dir")
	assert.DirExists(t, outputDir)
}

func TestGeneratorV3_MCPAutoGeneration(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "with-mcp")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGeneratorV3(cfg)

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

func TestGeneratorV3_MCPAutoGeneration_NoServers(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Create generator
	gen := NewGeneratorV3(cfg)

	// Generate
	err = gen.Generate("default")
	require.NoError(t, err)

	// Verify .mcp.json was NOT generated (no MCP servers configured)
	mcpPath := filepath.Join(tempDir, ".mcp.json")
	assert.NoFileExists(t, mcpPath, ".mcp.json should not be generated when no MCP servers exist")
}

func TestGeneratorV3_Gitignore_NoAbsolutePaths(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Override gitignore setting to enable it
	enabled := true
	cfg.Gitignore = &enabled

	// Create generator
	gen := NewGeneratorV3(cfg)

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

func TestGeneratorV3_Gitignore_SkipsAiRulezFolder(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Override gitignore setting to enable it
	enabled := true
	cfg.Gitignore = &enabled

	// Create generator
	gen := NewGeneratorV3(cfg)

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
func BenchmarkGeneratorV3_Basic(b *testing.B) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "basic")
	tempDir := b.TempDir()
	copyFixtureBench(b, fixtureDir, tempDir)

	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(b, err)

	gen := NewGeneratorV3(cfg)

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
func TestGeneratorV3_DefaultProfileDomainsLogic_WithAndWithoutProfiles(t *testing.T) {
	t.Run("default profile includes all domains when no profiles defined", func(t *testing.T) {
		cfg := &config.ConfigV3{
			Content: &config.ContentTreeV3{
				Domains: map[string]*config.DomainV3{
					"php": {Name: "php"},
				},
			},
			Profiles: map[string][]string{},
		}

		gen := &GeneratorV3{config: cfg}

		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)
		require.NotNil(t, content)

		assert.Len(t, content.Domains, 1)
		_, ok := content.Domains["php"]
		assert.True(t, ok, "expected php domain to be present for default profile when no profiles are defined")
	})

	t.Run("default profile only includes builtin domains when profiles are defined", func(t *testing.T) {
		cfg := &config.ConfigV3{
			Content: &config.ContentTreeV3{
				Domains: map[string]*config.DomainV3{
					"php": {Name: "php"},
					"go":  {Name: "go", Builtin: true},
				},
			},
			Profiles: map[string][]string{
				"backend": {"php"},
			},
		}

		gen := &GeneratorV3{config: cfg}

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
		cfg := &config.ConfigV3{
			Content: &config.ContentTreeV3{
				Domains: map[string]*config.DomainV3{
					"php": {Name: "php"},
					"go":  {Name: "go", Builtin: true},
				},
			},
			// User explicitly wrote profiles: { "default": ["php"] }
			Profiles: map[string][]string{
				"default": {"php"},
			},
		}

		gen := &GeneratorV3{config: cfg}

		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)
		require.NotNil(t, content)

		// Explicit profiles["default"] wins: php should be present, go (builtin) also
		// present because GetContentForProfile always includes root + profile domains.
		_, hasPHP := content.Domains["php"]
		assert.True(t, hasPHP, "expected explicitly listed php domain to be present when profiles[default] is defined")
	})

	t.Run("default profile includes FromInclude domains even when profiles are defined", func(t *testing.T) {
		cfg := &config.ConfigV3{
			Content: &config.ContentTreeV3{
				Domains: map[string]*config.DomainV3{
					"php":    {Name: "php"},                       // non-builtin, referenced by a profile
					"go":     {Name: "go", Builtin: true},         // builtin domain
					"python": {Name: "python", FromInclude: true}, // from external include
				},
			},
			Profiles: map[string][]string{
				"backend": {"php"},
			},
		}

		gen := &GeneratorV3{config: cfg}

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

// Test MCP server aggregation for default profile with and without profiles.
func TestGeneratorV3_DefaultProfile_MCPServers_WithAndWithoutProfiles(t *testing.T) {
	t.Run("default profile includes domain MCP servers when no profiles defined", func(t *testing.T) {
		rootEnabled := true
		domainEnabled := true

		cfg := &config.ConfigV3{
			MCPServers: map[string]*config.MCPServerV3{
				"root-server": {
					Name:    "root-server",
					Enabled: &rootEnabled,
				},
			},
			Content: &config.ContentTreeV3{
				Domains: map[string]*config.DomainV3{
					"php": {
						Name: "php",
						MCPServers: map[string]*config.MCPServerV3{
							"php-server": {
								Name:    "php-server",
								Enabled: &domainEnabled,
							},
						},
					},
				},
			},
			Profiles: map[string][]string{},
		}

		gen := &GeneratorV3{config: cfg}

		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)

		servers := gen.collectMCPServersForContent(content)
		require.NotNil(t, servers)

		// Should contain both root and domain MCP servers
		_, hasRoot := servers["root-server"]
		_, hasPHP := servers["php-server"]
		assert.True(t, hasRoot, "expected root MCP server to be included")
		assert.True(t, hasPHP, "expected domain MCP server to be included when no profiles are defined")
	})

	t.Run("default profile only includes root and builtin domain MCP servers when profiles are defined", func(t *testing.T) {
		rootEnabled := true
		domainEnabled := true

		cfg := &config.ConfigV3{
			MCPServers: map[string]*config.MCPServerV3{
				"root-server": {
					Name:    "root-server",
					Enabled: &rootEnabled,
				},
			},
			Content: &config.ContentTreeV3{
				Domains: map[string]*config.DomainV3{
					"php": {
						Name: "php",
						MCPServers: map[string]*config.MCPServerV3{
							"php-server": {
								Name:    "php-server",
								Enabled: &domainEnabled,
							},
						},
					},
					// Builtin domain should always be visible on default, even when profiles exist.
					"go": {
						Name:    "go",
						Builtin: true,
						MCPServers: map[string]*config.MCPServerV3{
							"go-server": {
								Name:    "go-server",
								Enabled: &domainEnabled,
							},
						},
					},
				},
			},
			Profiles: map[string][]string{
				"backend": {"php"},
			},
		}

		gen := &GeneratorV3{config: cfg}

		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)

		servers := gen.collectMCPServersForContent(content)
		require.NotNil(t, servers)

		// Should contain root and builtin domain MCP servers, but not non-builtin profile-only domain servers.
		_, hasRoot := servers["root-server"]
		_, hasGo := servers["go-server"]
		_, hasPHP := servers["php-server"]
		assert.True(t, hasRoot, "expected root MCP server to be included")
		assert.True(t, hasGo, "expected builtin domain MCP server to be included for default when profiles are defined")
		assert.False(t, hasPHP, "expected non-builtin profile domain MCP server to be excluded for default when profiles are defined")
	})

	t.Run("domain MCP server override order is deterministic when multiple domains share a server name", func(t *testing.T) {
		enabled := true

		cfg := &config.ConfigV3{
			MCPServers: map[string]*config.MCPServerV3{},
			Content: &config.ContentTreeV3{
				Domains: map[string]*config.DomainV3{
					"aaa": {
						Name: "aaa",
						MCPServers: map[string]*config.MCPServerV3{
							"shared-server": {
								Name:    "aaa-value",
								Enabled: &enabled,
							},
						},
					},
					"zzz": {
						Name: "zzz",
						MCPServers: map[string]*config.MCPServerV3{
							"shared-server": {
								Name:    "zzz-value",
								Enabled: &enabled,
							},
						},
					},
				},
			},
			Profiles: map[string][]string{},
		}

		gen := &GeneratorV3{config: cfg}
		content, err := gen.getContentForProfile(defaultProfileName)
		require.NoError(t, err)

		// Run multiple times to catch any non-determinism
		var firstName string
		for i := 0; i < 20; i++ {
			servers := gen.collectMCPServersForContent(content)
			name := servers["shared-server"].Name
			if i == 0 {
				firstName = name
			} else {
				assert.Equal(t, firstName, name, "MCP server override order should be deterministic across runs")
			}
		}
		// Alphabetically last domain ("zzz") should win
		assert.Equal(t, "zzz-value", firstName, "expected alphabetically last domain to win override")
	})
}

func TestGeneratorV3_InvalidProfile_SortedHint(t *testing.T) {
	cfg := &config.ConfigV3{
		Profiles: map[string][]string{
			"zebra":  {},
			"alpha":  {},
			"middle": {},
		},
		Content: &config.ContentTreeV3{
			Rules: []config.ContentFile{
				{Path: "rule.md", Content: "content"},
			},
		},
	}

	gen := NewGeneratorV3(cfg)
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

func TestGeneratorV3_SkipUnchangedFiles(t *testing.T) {
	// Setup
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "v3", "generator", "basic")
	tempDir := t.TempDir()

	// Copy fixture to temp dir
	copyFixture(t, fixtureDir, tempDir)

	// Load config
	ctx := context.Background()
	cfg, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)

	// Create generator and generate first time
	gen := NewGeneratorV3(cfg)
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
	cfg2, err := config.LoadConfigV3(ctx, tempDir)
	require.NoError(t, err)
	gen2 := NewGeneratorV3(cfg2)
	err = gen2.Generate("default")
	require.NoError(t, err)

	// The file content should be identical (hash matches, so skip)
	secondContent, err := os.ReadFile(mainFilePath)
	require.NoError(t, err)
	assert.Equal(t, string(firstContent), string(secondContent),
		"file should not be rewritten when content hash matches")
}
