package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectSources(t *testing.T) {
	t.Run("detects CLAUDE.md", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		createFile(t, tmpDir, "CLAUDE.md", "# Test")
		importer := NewImporter(tmpDir, filepath.Join(tmpDir, ".ai-rulez"))

		// Act
		sources, err := importer.detectSources()

		// Assert
		require.NoError(t, err)
		assert.Contains(t, sources, "CLAUDE.md")
	})

	t.Run("detects multiple sources", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		createFile(t, tmpDir, "CLAUDE.md", "# Test")
		createFile(t, tmpDir, "GEMINI.md", "# Test")
		createDir(t, tmpDir, ".cursor/rules")
		createDir(t, tmpDir, ".claude/skills")
		importer := NewImporter(tmpDir, filepath.Join(tmpDir, ".ai-rulez"))

		// Act
		sources, err := importer.detectSources()

		// Assert
		require.NoError(t, err)
		assert.Contains(t, sources, "CLAUDE.md")
		assert.Contains(t, sources, "GEMINI.md")
		assert.Contains(t, sources, ".cursor/rules")
		assert.Contains(t, sources, ".claude/skills")
	})

	t.Run("returns empty when no sources found", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		importer := NewImporter(tmpDir, filepath.Join(tmpDir, ".ai-rulez"))

		// Act
		sources, err := importer.detectSources()

		// Assert
		require.NoError(t, err)
		assert.Empty(t, sources)
	})
}

func TestParseMarkdownFile(t *testing.T) {
	t.Run("parses sections from CLAUDE.md", func(t *testing.T) {
		// Arrange
		content := `# Project Rules

## Code Quality Standards

You must follow these rules:
- Write tests
- Handle errors

## Project Architecture

This describes the architecture.

### Background

Some background info.
`

		// Act
		items := parseMarkdownFile("CLAUDE.md", content)

		// Assert
		require.Len(t, items, 3)
		assert.Equal(t, "code-quality-standards", items[0].Name)
		assert.Equal(t, ContentTypeRule, items[0].Type)
		assert.Contains(t, items[0].Content, "must follow")

		assert.Equal(t, "project-architecture", items[1].Name)
		assert.Equal(t, ContentTypeContext, items[1].Type)

		assert.Equal(t, "background", items[2].Name)
		assert.Equal(t, ContentTypeContext, items[2].Type)
	})

	t.Run("classifies rules correctly", func(t *testing.T) {
		// Arrange
		content := `## Testing Rules

You must write tests.
You should never skip tests.
Always test edge cases.
`

		// Act
		items := parseMarkdownFile("CLAUDE.md", content)

		// Assert
		require.Len(t, items, 1)
		assert.Equal(t, ContentTypeRule, items[0].Type)
	})

	t.Run("classifies skills correctly", func(t *testing.T) {
		// Arrange
		content := `## Code Review Agent

You are a code review agent.
Use this command to review code.
`

		// Act
		items := parseMarkdownFile("CLAUDE.md", content)

		// Assert
		require.Len(t, items, 1)
		assert.Equal(t, ContentTypeSkill, items[0].Type)
	})
}

func TestClassifyContent(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		content      string
		expectedType ContentType
	}{
		{
			name:         "rule by filename",
			filename:     "coding-rules.md",
			content:      "Some content",
			expectedType: ContentTypeRule,
		},
		{
			name:         "rule by content keywords",
			filename:     "standards.md",
			content:      "You must always write tests. You should never skip documentation. Required to handle errors.",
			expectedType: ContentTypeRule,
		},
		{
			name:         "skill by filename",
			filename:     "code-review-agent.md",
			content:      "Some content",
			expectedType: ContentTypeSkill,
		},
		{
			name:         "context by default",
			filename:     "overview.md",
			content:      "This is a general overview of the project.",
			expectedType: ContentTypeContext,
		},
		{
			name:         "context with few rule keywords",
			filename:     "architecture.md",
			content:      "The system must be scalable. It includes various components.",
			expectedType: ContentTypeContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			result := classifyContent(tt.filename, tt.content)

			// Assert
			assert.Equal(t, tt.expectedType, result)
		})
	}
}

func TestDeduplicateContent(t *testing.T) {
	t.Run("removes exact duplicates", func(t *testing.T) {
		// Arrange
		items := []ImportedContent{
			{
				Name:    "rule1",
				Content: "Same content",
				Hash:    hashContent("Same content"),
			},
			{
				Name:    "rule2",
				Content: "Same content",
				Hash:    hashContent("Same content"),
			},
			{
				Name:    "rule3",
				Content: "Different content",
				Hash:    hashContent("Different content"),
			},
		}

		// Act
		result := deduplicateContent(items)

		// Assert
		assert.Len(t, result, 2)
	})

	t.Run("preserves unique content", func(t *testing.T) {
		// Arrange
		items := []ImportedContent{
			{
				Name:    "rule1",
				Content: "Content 1",
				Hash:    hashContent("Content 1"),
			},
			{
				Name:    "rule2",
				Content: "Content 2",
				Hash:    hashContent("Content 2"),
			},
		}

		// Act
		result := deduplicateContent(items)

		// Assert
		assert.Len(t, result, 2)
	})
}

func TestHashContent(t *testing.T) {
	t.Run("same content produces same hash", func(t *testing.T) {
		// Arrange
		content1 := "Test content"
		content2 := "Test content"

		// Act
		hash1 := hashContent(content1)
		hash2 := hashContent(content2)

		// Assert
		assert.Equal(t, hash1, hash2)
	})

	t.Run("different content produces different hash", func(t *testing.T) {
		// Arrange
		content1 := "Test content 1"
		content2 := "Test content 2"

		// Act
		hash1 := hashContent(content1)
		hash2 := hashContent(content2)

		// Assert
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("whitespace is normalized", func(t *testing.T) {
		// Arrange
		content1 := "Test content"
		content2 := "  Test content  \n"

		// Act
		hash1 := hashContent(content1)
		hash2 := hashContent(content2)

		// Assert
		assert.Equal(t, hash1, hash2)
	})
}

func TestImportClaudeSkills(t *testing.T) {
	t.Run("imports SKILL.md files", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		skillsDir := filepath.Join(tmpDir, ".claude", "skills")

		createDir(t, tmpDir, ".claude/skills/code-reviewer")
		createFile(t, tmpDir, ".claude/skills/code-reviewer/SKILL.md", `---
priority: high
---

# Code Reviewer

You are a code reviewer.
`)

		createDir(t, tmpDir, ".claude/skills/bug-hunter")
		createFile(t, tmpDir, ".claude/skills/bug-hunter/SKILL.md", "# Bug Hunter\n\nFind bugs.")

		importer := NewImporter(tmpDir, filepath.Join(tmpDir, ".ai-rulez"))

		// Act
		items, preset, err := importer.importClaudeSkills(".claude/skills", skillsDir)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "claude", preset)
		assert.Len(t, items, 2)

		// Find items by name (order not guaranteed)
		var codeReviewerItem, bugHunterItem *ImportedContent
		for i := range items {
			switch items[i].Name {
			case "code-reviewer":
				codeReviewerItem = &items[i]
			case "bug-hunter":
				bugHunterItem = &items[i]
			}
		}

		require.NotNil(t, codeReviewerItem, "code-reviewer skill not found")
		assert.Equal(t, ContentTypeSkill, codeReviewerItem.Type)
		assert.NotNil(t, codeReviewerItem.Metadata)
		assert.Equal(t, "high", codeReviewerItem.Metadata.Priority)

		require.NotNil(t, bugHunterItem, "bug-hunter skill not found")
		assert.Equal(t, ContentTypeSkill, bugHunterItem.Type)
	})

	t.Run("skips directories without SKILL.md", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		skillsDir := filepath.Join(tmpDir, ".claude", "skills")

		createDir(t, tmpDir, ".claude/skills/incomplete")
		createFile(t, tmpDir, ".claude/skills/incomplete/README.md", "# Not a skill")

		importer := NewImporter(tmpDir, filepath.Join(tmpDir, ".ai-rulez"))

		// Act
		items, preset, err := importer.importClaudeSkills(".claude/skills", skillsDir)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "claude", preset)
		assert.Empty(t, items)
	})
}

func TestImportCursorRules(t *testing.T) {
	t.Run("imports .cursor/rules files", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		rulesDir := filepath.Join(tmpDir, ".cursor", "rules")

		createDir(t, tmpDir, ".cursor/rules")
		createFile(t, tmpDir, ".cursor/rules/coding-standards.md", "# Standards\n\nYou must follow standards.")
		createFile(t, tmpDir, ".cursor/rules/testing.mdc", "# Testing\n\nAlways write tests.")

		importer := NewImporter(tmpDir, filepath.Join(tmpDir, ".ai-rulez"))

		// Act
		items, preset, err := importer.importCursorRules(".cursor/rules", rulesDir)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "cursor", preset)
		assert.Len(t, items, 2)

		assert.Equal(t, "coding-standards", items[0].Name)
		assert.Equal(t, ContentTypeRule, items[0].Type)

		assert.Equal(t, "testing", items[1].Name)
		assert.Equal(t, ".mdc", items[1].OriginalExt)
	})
}

func TestImportContinuePrompts(t *testing.T) {
	t.Run("imports YAML prompt files", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		promptsDir := filepath.Join(tmpDir, ".continue", "prompts")

		createDir(t, tmpDir, ".continue/prompts")
		createFile(t, tmpDir, ".continue/prompts/explain.yaml", `name: Explain Code
description: Explains the selected code
prompt: Please explain this code in detail.
`)

		importer := NewImporter(tmpDir, filepath.Join(tmpDir, ".ai-rulez"))

		// Act
		items, preset, err := importer.importContinuePrompts(".continue/prompts", promptsDir)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "continue-dev", preset)
		assert.Len(t, items, 1)

		assert.Equal(t, "explain", items[0].Name)
		assert.Equal(t, ContentTypeContext, items[0].Type)
		assert.Contains(t, items[0].Content, "Explain Code")
		assert.Contains(t, items[0].Content, "Please explain")
	})

	t.Run("skips invalid YAML files", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		promptsDir := filepath.Join(tmpDir, ".continue", "prompts")

		createDir(t, tmpDir, ".continue/prompts")
		createFile(t, tmpDir, ".continue/prompts/invalid.yaml", "not: valid: yaml:")

		importer := NewImporter(tmpDir, filepath.Join(tmpDir, ".ai-rulez"))

		// Act
		items, _, err := importer.importContinuePrompts(".continue/prompts", promptsDir)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, items)
	})
}

func TestDetectPresetFromSource(t *testing.T) {
	tests := []struct {
		source   string
		expected string
	}{
		{"CLAUDE.md", "claude"},
		{".claude/skills", "claude"},
		{".cursor/rules", "cursor"},
		{"GEMINI.md", "gemini"},
		{".github/copilot-instructions.md", "copilot"},
		{".continue/rules", "continue-dev"},
		{".windsurf/rules", "windsurf"},
		{".clinerules", "cline"},
		{"unknown.md", ""},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			// Act
			result := detectPresetFromSource(tt.source)

			// Assert
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWriteContent(t *testing.T) {
	t.Run("writes rules to rules/ directory", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		outputDir := filepath.Join(tmpDir, ".ai-rulez")
		importer := NewImporter(tmpDir, outputDir)

		items := []ImportedContent{
			{
				Name:    "coding-standards",
				Type:    ContentTypeRule,
				Content: "Follow coding standards.",
			},
		}

		// Act
		err := importer.writeContent(items)

		// Assert
		require.NoError(t, err)

		rulePath := filepath.Join(outputDir, "rules", "coding-standards.md")
		assert.FileExists(t, rulePath)

		content, err := os.ReadFile(rulePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "Follow coding standards")
	})

	t.Run("writes context to context/ directory", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		outputDir := filepath.Join(tmpDir, ".ai-rulez")
		importer := NewImporter(tmpDir, outputDir)

		items := []ImportedContent{
			{
				Name:    "architecture",
				Type:    ContentTypeContext,
				Content: "System architecture overview.",
			},
		}

		// Act
		err := importer.writeContent(items)

		// Assert
		require.NoError(t, err)

		contextPath := filepath.Join(outputDir, "context", "architecture.md")
		assert.FileExists(t, contextPath)
	})

	t.Run("writes skills to skills/<name>/SKILL.md", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		outputDir := filepath.Join(tmpDir, ".ai-rulez")
		importer := NewImporter(tmpDir, outputDir)

		items := []ImportedContent{
			{
				Name:    "code-reviewer",
				Type:    ContentTypeSkill,
				Content: "You are a code reviewer.",
			},
		}

		// Act
		err := importer.writeContent(items)

		// Assert
		require.NoError(t, err)

		skillPath := filepath.Join(outputDir, "skills", "code-reviewer", "SKILL.md")
		assert.FileExists(t, skillPath)

		content, err := os.ReadFile(skillPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "code reviewer")
	})

	t.Run("includes frontmatter when metadata present", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		outputDir := filepath.Join(tmpDir, ".ai-rulez")
		importer := NewImporter(tmpDir, outputDir)

		items := []ImportedContent{
			{
				Name:    "important-rule",
				Type:    ContentTypeRule,
				Content: "This is important.",
				Metadata: &config.MetadataV3{
					Priority: "critical",
					Targets:  []string{"claude", "cursor"},
				},
			},
		}

		// Act
		err := importer.writeContent(items)

		// Assert
		require.NoError(t, err)

		rulePath := filepath.Join(outputDir, "rules", "important-rule.md")
		content, err := os.ReadFile(rulePath)
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, "---")
		assert.Contains(t, contentStr, "priority: critical")
		assert.Contains(t, contentStr, "targets:")
		assert.Contains(t, contentStr, "- claude")
		assert.Contains(t, contentStr, "- cursor")
	})
}

func TestWriteConfig(t *testing.T) {
	t.Run("writes config with detected presets", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		outputDir := filepath.Join(tmpDir, ".ai-rulez")
		createDir(t, tmpDir, ".ai-rulez")

		importer := NewImporter(tmpDir, outputDir)
		detectedPresets := map[string]bool{
			"claude": true,
			"cursor": true,
		}

		// Act
		err := importer.writeConfig("test-project", detectedPresets)

		// Assert
		require.NoError(t, err)

		configPath := filepath.Join(outputDir, "config.yaml")
		assert.FileExists(t, configPath)

		content, err := os.ReadFile(configPath)
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, "version: \"3.0\"")
		assert.Contains(t, contentStr, "name: test-project")
		assert.Contains(t, contentStr, "presets:")
		assert.Contains(t, contentStr, "- claude")
		assert.Contains(t, contentStr, "- cursor")
	})

	t.Run("defaults to claude when no presets detected", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		outputDir := filepath.Join(tmpDir, ".ai-rulez")
		createDir(t, tmpDir, ".ai-rulez")

		importer := NewImporter(tmpDir, outputDir)
		detectedPresets := map[string]bool{}

		// Act
		err := importer.writeConfig("test-project", detectedPresets)

		// Assert
		require.NoError(t, err)

		configPath := filepath.Join(outputDir, "config.yaml")
		content, err := os.ReadFile(configPath)
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, "- claude")
	})
}

func TestIntegrationImport(t *testing.T) {
	t.Run("imports from multiple sources", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		outputDir := filepath.Join(tmpDir, ".ai-rulez")

		// Create CLAUDE.md
		createFile(t, tmpDir, "CLAUDE.md", `# AI Rules

## Code Quality

You must write clean code.
You should always test.
Never skip documentation.

## Project Architecture

This is a modular project.
`)

		// Create .cursor/rules
		createDir(t, tmpDir, ".cursor/rules")
		createFile(t, tmpDir, ".cursor/rules/testing.md", "# Testing\n\nAll code must be tested.")

		// Create .claude/skills
		createDir(t, tmpDir, ".claude/skills/reviewer")
		createFile(t, tmpDir, ".claude/skills/reviewer/SKILL.md", "# Code Reviewer\n\nReview code carefully.")

		importer := NewImporter(tmpDir, outputDir)

		// Act
		err := importer.Import("auto")

		// Assert
		require.NoError(t, err)

		// Check config exists
		configPath := filepath.Join(outputDir, "config.yaml")
		assert.FileExists(t, configPath)

		// Check rules directory
		rulesDir := filepath.Join(outputDir, "rules")
		assert.DirExists(t, rulesDir)

		// Check context directory
		contextDir := filepath.Join(outputDir, "context")
		assert.DirExists(t, contextDir)

		// Check skills directory
		skillsDir := filepath.Join(outputDir, "skills")
		assert.DirExists(t, skillsDir)

		// Verify presets in config
		configContent, err := os.ReadFile(configPath)
		require.NoError(t, err)
		contentStr := string(configContent)
		assert.Contains(t, contentStr, "claude")
		assert.Contains(t, contentStr, "cursor")
	})

	t.Run("handles deduplication across sources", func(t *testing.T) {
		// Arrange
		tmpDir := t.TempDir()
		outputDir := filepath.Join(tmpDir, ".ai-rulez")

		// Create duplicate content in different sources
		createFile(t, tmpDir, "CLAUDE.md", `## Testing Rules

You must write tests.
`)

		createDir(t, tmpDir, ".cursor/rules")
		createFile(t, tmpDir, ".cursor/rules/testing.md", "You must write tests.")

		importer := NewImporter(tmpDir, outputDir)

		// Act
		err := importer.Import("auto")

		// Assert
		require.NoError(t, err)

		// Should only have one file (deduplicated)
		rulesDir := filepath.Join(outputDir, "rules")
		entries, err := os.ReadDir(rulesDir)
		require.NoError(t, err)
		assert.Len(t, entries, 1)
	})
}

func TestSplitMarkdownSections(t *testing.T) {
	t.Run("splits by ## headers", func(t *testing.T) {
		// Arrange
		content := `## Section 1

Content 1

## Section 2

Content 2
`

		// Act
		sections := splitMarkdownSections(content)

		// Assert
		assert.Len(t, sections, 2)
		assert.Equal(t, "Section 1", sections[0].Header)
		assert.Contains(t, sections[0].Content, "Content 1")
		assert.Equal(t, "Section 2", sections[1].Header)
		assert.Contains(t, sections[1].Content, "Content 2")
	})

	t.Run("splits by ### headers", func(t *testing.T) {
		// Arrange
		content := `### Subsection 1

Content 1

### Subsection 2

Content 2
`

		// Act
		sections := splitMarkdownSections(content)

		// Assert
		assert.Len(t, sections, 2)
		assert.Equal(t, "Subsection 1", sections[0].Header)
		assert.Equal(t, "Subsection 2", sections[1].Header)
	})

	t.Run("handles mixed header levels", func(t *testing.T) {
		// Arrange
		content := `## Main Section

Main content

### Subsection

Sub content
`

		// Act
		sections := splitMarkdownSections(content)

		// Assert
		assert.Len(t, sections, 2)
		assert.Equal(t, "Main Section", sections[0].Header)
		assert.Equal(t, "Subsection", sections[1].Header)
	})
}

func TestImporterSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Code Quality Standards", "code-quality-standards"},
		{"Testing_Rules", "testing_rules"},
		{"API/Documentation", "apidocumentation"},
		{"Feature #123", "feature-123"},
		{"Multiple   Spaces", "multiple-spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// Act
			result := utils.SanitizeName(tt.input)

			// Assert
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseFrontmatterToMetadata(t *testing.T) {
	t.Run("parses frontmatter with priority", func(t *testing.T) {
		// Arrange
		content := `---
priority: high
---

Content here
`

		// Act
		metadata, actualContent := parseFrontmatterToMetadata(content)

		// Assert
		require.NotNil(t, metadata)
		assert.Equal(t, "high", metadata.Priority)
		assert.Equal(t, "Content here\n", actualContent)
	})

	t.Run("parses frontmatter with targets", func(t *testing.T) {
		// Arrange
		content := `---
priority: medium
targets:
  - claude
  - cursor
---

Content here
`

		// Act
		metadata, actualContent := parseFrontmatterToMetadata(content)

		// Assert
		require.NotNil(t, metadata)
		assert.Equal(t, "medium", metadata.Priority)
		assert.Equal(t, []string{"claude", "cursor"}, metadata.Targets)
		assert.Equal(t, "Content here\n", actualContent)
	})

	t.Run("returns nil when no frontmatter", func(t *testing.T) {
		// Arrange
		content := "Just content, no frontmatter"

		// Act
		metadata, actualContent := parseFrontmatterToMetadata(content)

		// Assert
		assert.Nil(t, metadata)
		assert.Equal(t, content, actualContent)
	})
}

// Helper functions

func createFile(t *testing.T, baseDir, relativePath, content string) {
	t.Helper()
	fullPath := filepath.Join(baseDir, relativePath)
	dir := filepath.Dir(fullPath)

	err := os.MkdirAll(dir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(fullPath, []byte(content), 0o644)
	require.NoError(t, err)
}

func createDir(t *testing.T, baseDir, relativePath string) {
	t.Helper()
	fullPath := filepath.Join(baseDir, relativePath)
	err := os.MkdirAll(fullPath, 0o755)
	require.NoError(t, err)
}

func TestDeduplicateContentWithMetadata(t *testing.T) {
	t.Run("merges metadata from duplicates", func(t *testing.T) {
		// Arrange
		content := "same content"
		items := []ImportedContent{
			{
				Name:    "item1",
				Type:    ContentTypeRule,
				Content: content,
				Hash:    hashContent(content),
				Metadata: &config.MetadataV3{
					Priority: "high",
					Targets:  []string{"target1"},
				},
			},
			{
				Name:    "item2",
				Type:    ContentTypeRule,
				Content: content, // Same content
				Hash:    hashContent(content),
				Metadata: &config.MetadataV3{
					Priority: "critical",
					Targets:  []string{"target2"},
				},
			},
		}

		// Act
		result := deduplicateContent(items)

		// Assert
		require.Len(t, result, 1)
		assert.Equal(t, "critical", result[0].Metadata.Priority) // Higher priority kept
		assert.Len(t, result[0].Metadata.Targets, 2)             // Both targets combined
		assert.Contains(t, result[0].Metadata.Targets, "target1")
		assert.Contains(t, result[0].Metadata.Targets, "target2")
	})

	t.Run("preserves order and removes duplicates", func(t *testing.T) {
		// Arrange
		items := []ImportedContent{
			{
				Name:    "unique1",
				Content: "content1",
				Hash:    hashContent("content1"),
			},
			{
				Name:    "dup1",
				Content: "content2",
				Hash:    hashContent("content2"),
			},
			{
				Name:    "dup1-again",
				Content: "content2", // Duplicate
				Hash:    hashContent("content2"),
			},
			{
				Name:    "unique2",
				Content: "content3",
				Hash:    hashContent("content3"),
			},
		}

		// Act
		result := deduplicateContent(items)

		// Assert
		require.Len(t, result, 3)
		assert.Equal(t, "unique1", result[0].Name)
		assert.Equal(t, "dup1", result[1].Name)
		assert.Equal(t, "unique2", result[2].Name)
	})
}

func TestImprovedContentClassification(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		content      string
		expectedType ContentType
	}{
		{
			name:         "strong rule indicators",
			filename:     "coding-rules.md",
			content:      "You must follow these rules. Always test. Never skip validation.",
			expectedType: ContentTypeRule,
		},
		{
			name:         "strong skill indicators",
			filename:     "code-agent.md",
			content:      "This is a code review agent that analyzes functions with parameters.",
			expectedType: ContentTypeSkill,
		},
		{
			name:     "context with structure and length",
			filename: "project-overview.md",
			content: `## Project Overview

This is a comprehensive project documentation with background and architecture details.
The project provides an extensive overview of how the system works and how different components interact.
It includes detailed information about project architecture and background information about the system design.`,
			expectedType: ContentTypeContext,
		},
		{
			name:         "defaults to context for ambiguous",
			filename:     "misc.md",
			content:      "Some random content",
			expectedType: ContentTypeContext,
		},
		{
			name:         "rule with keywords",
			filename:     "standards.md",
			content:      "Code must be tested. Should follow standards. Forbidden: skip error handling.",
			expectedType: ContentTypeRule,
		},
		{
			name:         "skill by command keyword",
			filename:     "helper-agent.md",
			content:      "This is a command-line tool. Usage: call this function with parameters. Example: pass a file as argument.",
			expectedType: ContentTypeSkill,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyContent(tt.filename, tt.content)
			assert.Equal(t, tt.expectedType, result)
		})
	}
}
