package templates_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

// Helper function to create TemplateData with required fields
func createTemplateData(projectName string, cfgV3 *config.ConfigV3) *templates.TemplateData {
	return &templates.TemplateData{
		ProjectName:    projectName,
		RuleCount:      3,
		SectionCount:   2,
		AgentCount:     1,
		MCPServerCount: 1,
		CommandCount:   0,
		ConfigFile:     "config.yaml",
		OutputFile:     "CLAUDE.md",
		Timestamp:      time.Date(2025, 11, 1, 15, 7, 23, 0, time.UTC),
		Config:         cfgV3,
	}
}

// TestBuildDetailedHeader verifies the detailed header structure and content
func TestBuildDetailedHeader(t *testing.T) {
	data := createTemplateData("TestProject", nil)

	// Call GenerateHeader to get the full header with wrapping
	header := templates.GenerateHeader(data)

	// Verify header is wrapped in HTML comment
	assert.True(t, strings.HasPrefix(header, "<!--"), "header should start with HTML comment")
	assert.True(t, strings.Contains(header, "-->"), "header should contain closing HTML comment")

	// Verify banner section
	assert.Contains(t, header, "🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY")
	assert.Contains(t, header, "Project: TestProject")
	assert.Contains(t, header, "Generated: 2025-11-01 15:07:23")
	assert.Contains(t, header, "Source: .ai-rulez/config.yaml")
	assert.Contains(t, header, "Target: CLAUDE.md")
	assert.Contains(t, header, "Content: rules=3, sections=2, agents=1")

	// Verify "WHAT IS AI-RULEZ" section
	assert.Contains(t, header, "WHAT IS AI-RULEZ")
	assert.Contains(t, header, "AI-Rulez is a directory-based AI governance tool")
	assert.Contains(t, header, "auto-generated from source files")

	// Verify folder organization section
	assert.Contains(t, header, ".AI-RULEZ FOLDER ORGANIZATION")
	assert.Contains(t, header, ".ai-rulez/config.yaml")
	assert.Contains(t, header, ".ai-rulez/rules/")
	assert.Contains(t, header, ".ai-rulez/context/")
	assert.Contains(t, header, ".ai-rulez/skills/")
	assert.Contains(t, header, ".ai-rulez/agents/")
	assert.Contains(t, header, "Domain content (profile-specific)")
	assert.Contains(t, header, ".ai-rulez/domains/{name}/")

	// Verify instructions section
	assert.Contains(t, header, "INSTRUCTIONS FOR AI AGENTS")
	assert.Contains(t, header, "NEVER edit this file (CLAUDE.md)")
	assert.Contains(t, header, "ALWAYS edit files in .ai-rulez/ instead")
	assert.Contains(t, header, "PREFER using the MCP Server")
	assert.Contains(t, header, "npx -y ai-rulez@latest mcp")

	// Verify help section
	assert.Contains(t, header, "Documentation: https://github.com/Goldziher/ai-rulez")

	// Verify approximate line count (detailed should be around 50 lines)
	lines := strings.Split(strings.TrimSpace(header), "\n")
	assert.Greater(t, len(lines), 40, "detailed header should have roughly 50+ lines")
}

// TestBuildCompactHeader verifies the compact header structure and content
func TestBuildCompactHeader(t *testing.T) {
	data := createTemplateData("MyProject", nil)

	// For compact header, we need to set the style in the config
	cfgV3 := &config.ConfigV3{
		Name:   "MyProject",
		Header: &config.HeaderConfig{Style: "compact"},
	}
	data.Config = cfgV3

	header := templates.GenerateHeader(data)

	// Verify header is wrapped in HTML comment
	assert.True(t, strings.HasPrefix(header, "<!--"))
	assert.True(t, strings.Contains(header, "-->"))

	// Verify banner section
	assert.Contains(t, header, "🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT")
	assert.Contains(t, header, "Project: MyProject")
	assert.Contains(t, header, "Generated: 2025-11-01 15:07:23")
	assert.Contains(t, header, "Content: rules=3, sections=2, agents=1")

	// Verify "WHAT IS AI-RULEZ" section exists but is concise
	assert.Contains(t, header, "WHAT IS AI-RULEZ")
	assert.Contains(t, header, "Directory-based AI governance")

	// Verify structure section
	assert.Contains(t, header, "STRUCTURE:")
	assert.Contains(t, header, ".ai-rulez/config.yaml")
	assert.Contains(t, header, ".ai-rulez/domains/{name}/")

	// Verify AI AGENT INSTRUCTIONS with symbols
	assert.Contains(t, header, "AI AGENT INSTRUCTIONS:")
	assert.Contains(t, header, "✗ NEVER edit")
	assert.Contains(t, header, "✓ EDIT")
	assert.Contains(t, header, "✓ USE MCP server")
	assert.Contains(t, header, "✓ REGENERATE")
	assert.Contains(t, header, "✓ COMMIT")
	assert.Contains(t, header, "npx -y ai-rulez@latest mcp")

	// Verify help section
	assert.Contains(t, header, "Docs: https://github.com/Goldziher/ai-rulez")

	// Verify approximate line count (compact should be around 20 lines)
	lines := strings.Split(strings.TrimSpace(header), "\n")
	assert.Less(t, len(lines), 40, "compact header should have roughly 20 lines")
	assert.Greater(t, len(lines), 10, "compact header should have at least 10 lines")
}

// TestBuildMinimalHeader verifies the minimal header structure and content
func TestBuildMinimalHeader(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "SimpleProject",
		Header: &config.HeaderConfig{Style: "minimal"},
	}
	data := createTemplateData("SimpleProject", cfgV3)

	header := templates.GenerateHeader(data)

	// Verify header is wrapped in HTML comment
	assert.True(t, strings.HasPrefix(header, "<!--"))
	assert.True(t, strings.Contains(header, "-->"))

	// Verify banner section with minimal content
	assert.Contains(t, header, "🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT")
	assert.Contains(t, header, "Project: SimpleProject")
	assert.Contains(t, header, "Generated: 2025-11-01 15:07:23")
	assert.Contains(t, header, "Source: .ai-rulez/config.yaml")

	// Verify core instructions
	assert.Contains(t, header, "NEVER edit this file")
	assert.Contains(t, header, "modify .ai-rulez/ content instead")
	assert.Contains(t, header, "Use MCP server: npx -y ai-rulez@latest mcp")
	assert.Contains(t, header, "Regenerate: ai-rulez generate")

	// Verify help section
	assert.Contains(t, header, "Docs: https://github.com/Goldziher/ai-rulez")

	// Verify approximate line count (minimal should be around 10 lines)
	lines := strings.Split(strings.TrimSpace(header), "\n")
	assert.Less(t, len(lines), 20, "minimal header should have roughly 10 lines")
	assert.Greater(t, len(lines), 5, "minimal header should have at least 5 lines")

	// Verify it doesn't contain detailed section titles
	assert.NotContains(t, header, ".AI-RULEZ FOLDER ORGANIZATION")
	assert.NotContains(t, header, "INSTRUCTIONS FOR AI AGENTS")
}

// TestBuildHeaderLines_DefaultsToDetailed verifies that without config, detailed is used
func TestBuildHeaderLines_DefaultsToDetailed(t *testing.T) {
	data := createTemplateData("DefaultProject", nil)

	header := templates.GenerateHeader(data)

	// Should contain detailed header markers
	assert.Contains(t, header, "🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY")
	assert.Contains(t, header, ".AI-RULEZ FOLDER ORGANIZATION")
	assert.Contains(t, header, "INSTRUCTIONS FOR AI AGENTS")

	// Count lines - should be longer than compact/minimal
	lines := strings.Split(strings.TrimSpace(header), "\n")
	assert.Greater(t, len(lines), 40)
}

// TestBuildHeaderLines_WithCompactStyle verifies compact style selection
func TestBuildHeaderLines_WithCompactStyle(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "CompactProject",
		Header: &config.HeaderConfig{Style: "compact"},
	}
	data := createTemplateData("CompactProject", cfgV3)

	header := templates.GenerateHeader(data)

	// Should contain compact header markers
	assert.Contains(t, header, "🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT")
	assert.Contains(t, header, "WHAT IS AI-RULEZ:")
	assert.Contains(t, header, "STRUCTURE:")
	assert.Contains(t, header, "AI AGENT INSTRUCTIONS:")

	// Should not contain detailed section titles
	assert.NotContains(t, header, ".AI-RULEZ FOLDER ORGANIZATION")

	// Should contain checkmarks
	assert.Contains(t, header, "✓ EDIT")
	assert.Contains(t, header, "✗ NEVER edit")
}

// TestBuildHeaderLines_WithMinimalStyle verifies minimal style selection
func TestBuildHeaderLines_WithMinimalStyle(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "MinimalProject",
		Header: &config.HeaderConfig{Style: "minimal"},
	}
	data := createTemplateData("MinimalProject", cfgV3)

	header := templates.GenerateHeader(data)

	// Should contain minimal header markers
	assert.Contains(t, header, "🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT")
	assert.Contains(t, header, "Project: MinimalProject")
	assert.Contains(t, header, "NEVER edit this file")

	// Should not contain detailed section titles
	assert.NotContains(t, header, ".AI-RULEZ FOLDER ORGANIZATION")
	assert.NotContains(t, header, "AI AGENT INSTRUCTIONS:")

	// Should not contain checkmarks or detailed instructions
	assert.NotContains(t, header, "✓ EDIT")
	assert.NotContains(t, header, "Root content (always included)")
}

// TestBuildHeaderLines_WithDetailedStyle verifies explicit detailed style selection
func TestBuildHeaderLines_WithDetailedStyle(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "DetailedProject",
		Header: &config.HeaderConfig{Style: "detailed"},
	}
	data := createTemplateData("DetailedProject", cfgV3)

	header := templates.GenerateHeader(data)

	// Should contain all detailed sections
	assert.Contains(t, header, "🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY")
	assert.Contains(t, header, "WHAT IS AI-RULEZ")
	assert.Contains(t, header, ".AI-RULEZ FOLDER ORGANIZATION")
	assert.Contains(t, header, "INSTRUCTIONS FOR AI AGENTS")
	assert.Contains(t, header, "Root content (always included):")
	assert.Contains(t, header, "Domain content (profile-specific):")

	// Line count should be significant
	lines := strings.Split(strings.TrimSpace(header), "\n")
	assert.Greater(t, len(lines), 40)
}

// TestGenerateHeader_AllStyles verifies that GenerateHeader properly wraps headers
func TestGenerateHeader_AllStyles(t *testing.T) {
	styles := []string{"detailed", "compact", "minimal"}

	for _, style := range styles {
		t.Run("style_"+style, func(t *testing.T) {
			cfgV3 := &config.ConfigV3{
				Name:   "TestProject",
				Header: &config.HeaderConfig{Style: style},
			}
			data := createTemplateData("TestProject", cfgV3)

			header := templates.GenerateHeader(data)

			// All headers should be wrapped in HTML comments for .md files
			assert.True(t, strings.HasPrefix(header, "<!--"), "header should start with HTML comment opening")
			assert.True(t, strings.Contains(header, "-->"), "header should contain HTML comment closing")

			// All headers should contain essential info
			assert.Contains(t, header, "🤖 AI-RULEZ :: GENERATED FILE")
			assert.Contains(t, header, "Project: TestProject")
			assert.Contains(t, header, "Generated:")
			assert.Contains(t, header, "https://github.com/Goldziher/ai-rulez")

			// All headers should mention not editing
			assert.Contains(t, header, "DO NOT EDIT")
		})
	}
}

// TestGenerateHeader_WithDifferentFileTypes verifies comment wrapping per file type
func TestGenerateHeader_WithDifferentFileTypes(t *testing.T) {
	testCases := []struct {
		outputFile    string
		shouldHave    string
		shouldNotHave string
	}{
		{
			outputFile:    "CLAUDE.md",
			shouldHave:    "<!--",
			shouldNotHave: "# ",
		},
		{
			outputFile:    "cursor.json",
			shouldHave:    "// ",
			shouldNotHave: "<!--",
		},
		{
			outputFile:    "rules.py",
			shouldHave:    "# ",
			shouldNotHave: "<!--",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.outputFile, func(t *testing.T) {
			cfgV3 := &config.ConfigV3{
				Name:   "TestProject",
				Header: &config.HeaderConfig{Style: "minimal"},
			}
			data := createTemplateData("TestProject", cfgV3)
			data.OutputFile = tc.outputFile

			header := templates.GenerateHeader(data)

			if tc.shouldHave != "" {
				assert.True(t, strings.Contains(header, tc.shouldHave),
					"header for %s should contain %s", tc.outputFile, tc.shouldHave)
			}
			if tc.shouldNotHave != "" {
				assert.False(t, strings.Contains(header, tc.shouldNotHave),
					"header for %s should not contain %s", tc.outputFile, tc.shouldNotHave)
			}
		})
	}
}

// TestHeaderContent_WithEmptyConfigPath verifies default config path handling
func TestHeaderContent_WithEmptyConfigPath(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "TestProject",
		Header: &config.HeaderConfig{Style: "minimal"},
	}
	data := createTemplateData("TestProject", cfgV3)
	data.ConfigFile = "" // Empty config file

	header := templates.GenerateHeader(data)

	// Should default to "ai-rulez.yaml"
	assert.Contains(t, header, "Source: .ai-rulez/ai-rulez.yaml")
}

// TestHeaderContent_WithEmptyOutputPath verifies default output path handling
func TestHeaderContent_WithEmptyOutputPath(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "TestProject",
		Header: &config.HeaderConfig{Style: "compact"},
	}
	data := createTemplateData("TestProject", cfgV3)
	data.OutputFile = "" // Empty output file

	header := templates.GenerateHeader(data)

	// Should default to "(preview output)" in compact header which shows the output path
	assert.Contains(t, header, "(preview output)")
}

// TestHeaderContent_TimestampFormatting verifies correct timestamp format
func TestHeaderContent_TimestampFormatting(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "TestProject",
		Header: &config.HeaderConfig{Style: "minimal"},
	}
	data := createTemplateData("TestProject", cfgV3)
	data.Timestamp = time.Date(2024, 12, 25, 10, 30, 45, 0, time.UTC)

	header := templates.GenerateHeader(data)

	// Should contain formatted timestamp
	assert.Contains(t, header, "2024-12-25 10:30:45")
}

// TestHeaderContent_CountsDisplay verifies content counts are displayed correctly
func TestHeaderContent_CountsDisplay(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "TestProject",
		Header: &config.HeaderConfig{Style: "compact"},
	}
	data := createTemplateData("TestProject", cfgV3)
	data.RuleCount = 5
	data.SectionCount = 3
	data.AgentCount = 2

	header := templates.GenerateHeader(data)

	assert.Contains(t, header, "rules=5")
	assert.Contains(t, header, "sections=3")
	assert.Contains(t, header, "agents=2")
}

// TestHeaderContent_MCPServerCommand verifies MCP command in all styles
func TestHeaderContent_MCPServerCommand(t *testing.T) {
	styles := []string{"detailed", "compact", "minimal"}

	for _, style := range styles {
		t.Run("style_"+style, func(t *testing.T) {
			cfgV3 := &config.ConfigV3{
				Name:   "TestProject",
				Header: &config.HeaderConfig{Style: style},
			}
			data := createTemplateData("TestProject", cfgV3)

			header := templates.GenerateHeader(data)

			// All styles should mention the MCP server command
			assert.Contains(t, header, "npx -y ai-rulez@latest mcp",
				"header style %s should contain MCP command", style)
		})
	}
}

// TestHeaderContent_WorkflowInstructions verifies workflow steps are present
func TestHeaderContent_WorkflowInstructions(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "TestProject",
		Header: &config.HeaderConfig{Style: "detailed"},
	}
	data := createTemplateData("TestProject", cfgV3)

	header := templates.GenerateHeader(data)

	// Detailed header should have complete workflow
	assert.Contains(t, header, "ai-rulez generate")
	assert.Contains(t, header, "Commit both .ai-rulez/ and generated files")
	assert.Contains(t, header, "NEVER edit this file")
	assert.Contains(t, header, "ALWAYS edit files in .ai-rulez/ instead")
}

// TestHeaderContent_WithLargeContentCounts verifies large numbers display correctly
func TestHeaderContent_WithLargeContentCounts(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "TestProject",
		Header: &config.HeaderConfig{Style: "compact"},
	}
	data := createTemplateData("TestProject", cfgV3)
	data.RuleCount = 150
	data.SectionCount = 50
	data.AgentCount = 25
	data.MCPServerCount = 10
	data.CommandCount = 30

	header := templates.GenerateHeader(data)

	assert.Contains(t, header, "rules=150")
	assert.Contains(t, header, "sections=50")
	assert.Contains(t, header, "agents=25")
}

// TestGetHeaderStyle_DefaultBehavior verifies HeaderConfig.GetHeaderStyle() default
func TestGetHeaderStyle_DefaultBehavior(t *testing.T) {
	// Nil config should default to "detailed"
	var nilConfig *config.HeaderConfig
	assert.Equal(t, "detailed", nilConfig.GetHeaderStyle())

	// Empty style should default to "detailed"
	emptyConfig := &config.HeaderConfig{}
	assert.Equal(t, "detailed", emptyConfig.GetHeaderStyle())

	// Set styles should be returned as-is
	detailedConfig := &config.HeaderConfig{Style: "detailed"}
	assert.Equal(t, "detailed", detailedConfig.GetHeaderStyle())

	compactConfig := &config.HeaderConfig{Style: "compact"}
	assert.Equal(t, "compact", compactConfig.GetHeaderStyle())

	minimalConfig := &config.HeaderConfig{Style: "minimal"}
	assert.Equal(t, "minimal", minimalConfig.GetHeaderStyle())
}

// TestGenerateHeader_AllStylesWithMarkdownFile verifies HTML wrapping for all styles
func TestGenerateHeader_AllStylesWithMarkdownFile(t *testing.T) {
	styles := []string{"detailed", "compact", "minimal"}

	for _, style := range styles {
		t.Run("style_"+style, func(t *testing.T) {
			cfgV3 := &config.ConfigV3{
				Name:   "TestProject",
				Header: &config.HeaderConfig{Style: style},
			}
			data := createTemplateData("TestProject", cfgV3)
			data.OutputFile = "output.md"

			header := templates.GenerateHeader(data)

			// HTML comment wrapping for markdown files
			assert.True(t, strings.HasPrefix(header, "<!--"))
			assert.True(t, strings.Contains(header, "-->"))
		})
	}
}

// TestGenerateHeader_CompactHeaderLineCount verifies line count is appropriate
func TestGenerateHeader_CompactHeaderLineCount(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "TestProject",
		Header: &config.HeaderConfig{Style: "compact"},
	}
	data := createTemplateData("TestProject", cfgV3)

	header := templates.GenerateHeader(data)

	// Count non-empty lines in the header comment
	lines := strings.Split(strings.TrimSpace(header), "\n")
	// Filter out comment markers and count meaningful lines
	contentLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "<!--" && trimmed != "-->" && trimmed != "" {
			contentLines++
		}
	}

	// Compact should have roughly 15-25 content lines
	assert.Greater(t, contentLines, 10, "compact header should have at least 10 content lines")
	assert.Less(t, contentLines, 30, "compact header should have fewer than 30 content lines")
}

// TestGenerateHeader_MinimalHeaderLineCount verifies line count is minimal
func TestGenerateHeader_MinimalHeaderLineCount(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "TestProject",
		Header: &config.HeaderConfig{Style: "minimal"},
	}
	data := createTemplateData("TestProject", cfgV3)

	header := templates.GenerateHeader(data)

	// Count non-empty lines
	lines := strings.Split(strings.TrimSpace(header), "\n")
	contentLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "<!--" && trimmed != "-->" && trimmed != "" {
			contentLines++
		}
	}

	// Minimal should have roughly 7-12 content lines
	assert.Greater(t, contentLines, 5, "minimal header should have at least 5 content lines")
	assert.Less(t, contentLines, 15, "minimal header should have fewer than 15 content lines")
}

// TestGenerateHeader_DetailedHeaderLineCount verifies detailed header is comprehensive
func TestGenerateHeader_DetailedHeaderLineCount(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "TestProject",
		Header: &config.HeaderConfig{Style: "detailed"},
	}
	data := createTemplateData("TestProject", cfgV3)

	header := templates.GenerateHeader(data)

	// Count non-empty lines
	lines := strings.Split(strings.TrimSpace(header), "\n")
	contentLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "<!--" && trimmed != "-->" && trimmed != "" {
			contentLines++
		}
	}

	// Detailed should have 35+ content lines (approximately 50+ total with comments)
	assert.Greater(t, contentLines, 30, "detailed header should have 30+ content lines")
	assert.Greater(t, len(lines), 40, "detailed header should have 40+ total lines with HTML comment markers")
}

// TestHeaderContent_ProjectNameDisplay verifies project name in header
func TestHeaderContent_ProjectNameDisplay(t *testing.T) {
	projectNames := []string{"MyProject", "AI-Rulez", "Test123", "my-awesome-project"}

	for _, projName := range projectNames {
		t.Run(projName, func(t *testing.T) {
			cfgV3 := &config.ConfigV3{
				Name:   projName,
				Header: &config.HeaderConfig{Style: "minimal"},
			}
			data := createTemplateData(projName, cfgV3)

			header := templates.GenerateHeader(data)

			assert.Contains(t, header, "Project: "+projName)
		})
	}
}

// TestHeaderContent_ConfigPathAndOutputPath verifies paths are correctly shown
func TestHeaderContent_ConfigPathAndOutputPath(t *testing.T) {
	cfgV3 := &config.ConfigV3{
		Name:   "TestProject",
		Header: &config.HeaderConfig{Style: "compact"},
	}
	data := createTemplateData("TestProject", cfgV3)
	data.ConfigFile = "config/custom-config.yaml"
	data.OutputFile = "docs/output/CURSOR.md"

	header := templates.GenerateHeader(data)

	assert.Contains(t, header, "Source: .ai-rulez/config/custom-config.yaml")
	assert.Contains(t, header, "Target: docs/output/CURSOR.md")
}
