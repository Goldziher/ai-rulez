package crud_test

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/stretchr/testify/assert"
)

// TestGenerateFrontmatter tests YAML frontmatter generation
func TestGenerateFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		priority string
		targets  []string
		checkFn  func(t *testing.T, result string)
	}{
		{
			name:     "with priority and targets",
			priority: "high",
			targets:  []string{"claude", "cursor"},
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "---")
				assert.Contains(t, result, "priority: high")
				assert.Contains(t, result, "- claude")
				assert.Contains(t, result, "- cursor")
				assert.Contains(t, result, "targets:")
				// Verify structure
				lines := strings.Split(result, "\n")
				assert.Equal(t, "---", lines[0])
				assert.True(t, strings.HasPrefix(lines[1], "priority:") || strings.HasPrefix(lines[2], "priority:"))
			},
		},
		{
			name:     "empty priority defaults to medium",
			priority: "",
			targets:  []string{},
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "priority: medium")
				assert.Contains(t, result, "---")
			},
		},
		{
			name:     "no targets",
			priority: "critical",
			targets:  []string{},
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "priority: critical")
				assert.NotContains(t, result, "targets:")
			},
		},
		{
			name:     "single target",
			priority: "low",
			targets:  []string{"claude"},
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "- claude")
				assert.Contains(t, result, "targets:")
			},
		},
		{
			name:     "multiple targets",
			priority: "medium",
			targets:  []string{"claude", "cursor", "vim"},
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "- claude")
				assert.Contains(t, result, "- cursor")
				assert.Contains(t, result, "- vim")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := crud.GenerateFrontmatter(tt.priority, tt.targets)

			// Basic structure checks
			assert.True(t, strings.HasPrefix(result, "---"))
			assert.Contains(t, result, "---")

			// Run custom checks
			tt.checkFn(t, result)
		})
	}
}

// TestGenerateRuleTemplate tests rule template generation
func TestGenerateRuleTemplate(t *testing.T) {
	tests := []struct {
		name     string
		ruleName string
		priority string
		targets  []string
		content  string
		checkFn  func(t *testing.T, result string)
	}{
		{
			name:     "with custom content",
			ruleName: "coding-standard",
			priority: "high",
			targets:  []string{"claude"},
			content:  "Follow PEP 8 guidelines",
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "priority: high")
				assert.Contains(t, result, "Follow PEP 8 guidelines")
				assert.Contains(t, result, "---")
			},
		},
		{
			name:     "with generated content",
			ruleName: "my-rule",
			priority: "medium",
			targets:  []string{},
			content:  "",
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "priority: medium")
				// Should have auto-generated heading
				assert.Contains(t, result, "#")
				assert.Contains(t, result, "My Rule")
			},
		},
		{
			name:     "rule name normalization",
			ruleName: "test-rule-name",
			priority: "critical",
			targets:  []string{"cursor"},
			content:  "",
			checkFn: func(t *testing.T, result string) {
				// Should convert hyphens to spaces in heading
				assert.Contains(t, result, "Test Rule Name")
				assert.Contains(t, result, "priority: critical")
			},
		},
		{
			name:     "with content already containing frontmatter",
			ruleName: "test",
			priority: "high",
			targets:  []string{},
			content:  "---\npriority: low\n---\nCustom content",
			checkFn: func(t *testing.T, result string) {
				// Should preserve provided frontmatter structure
				assert.Contains(t, result, "Custom content")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := crud.GenerateRuleTemplate(tt.ruleName, tt.priority, tt.targets, tt.content)

			assert.NotEmpty(t, result)
			assert.True(t, strings.HasPrefix(result, "---"))

			tt.checkFn(t, result)
		})
	}
}

// TestGenerateContextTemplate tests context template generation
func TestGenerateContextTemplate(t *testing.T) {
	tests := []struct {
		name     string
		ctxName  string
		priority string
		targets  []string
		content  string
		checkFn  func(t *testing.T, result string)
	}{
		{
			name:     "with custom content",
			ctxName:  "project-structure",
			priority: "medium",
			targets:  []string{"claude"},
			content:  "The project has a modular structure",
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "priority: medium")
				assert.Contains(t, result, "The project has a modular structure")
			},
		},
		{
			name:     "with generated content",
			ctxName:  "team-context",
			priority: "low",
			targets:  []string{},
			content:  "",
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "priority: low")
				// Should have auto-generated heading
				assert.Contains(t, result, "Team Context")
				assert.Contains(t, result, "#")
			},
		},
		{
			name:     "context name with hyphens",
			ctxName:  "backend-api-docs",
			priority: "high",
			targets:  []string{"claude", "cursor"},
			content:  "",
			checkFn: func(t *testing.T, result string) {
				// Should convert hyphens to spaces
				assert.Contains(t, result, "Backend Api Docs")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := crud.GenerateContextTemplate(tt.ctxName, tt.priority, tt.targets, tt.content)

			assert.NotEmpty(t, result)
			tt.checkFn(t, result)
		})
	}
}

// TestGenerateSkillTemplate tests skill template generation
func TestGenerateSkillTemplate(t *testing.T) {
	tests := []struct {
		name        string
		skillName   string
		description string
		priority    string
		targets     []string
		content     string
		checkFn     func(t *testing.T, result string)
	}{
		{
			name:        "with description",
			skillName:   "code-review",
			description: "Reviews code for quality and best practices",
			priority:    "high",
			targets:     []string{"claude"},
			content:     "Use this skill to review pull requests",
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "priority: high")
				assert.Contains(t, result, "description:")
				assert.Contains(t, result, "Reviews code for quality and best practices")
				assert.Contains(t, result, "Use this skill to review pull requests")
			},
		},
		{
			name:        "without description",
			skillName:   "testing-skill",
			description: "",
			priority:    "medium",
			targets:     []string{},
			content:     "",
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "priority: medium")
				// Should have auto-generated content
				assert.Contains(t, result, "Testing Skill")
			},
		},
		{
			name:        "with multiple targets",
			skillName:   "refactor-code",
			description: "Refactors code for better quality",
			priority:    "critical",
			targets:     []string{"claude", "cursor", "vim"},
			content:     "Skill content",
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "- claude")
				assert.Contains(t, result, "- cursor")
				assert.Contains(t, result, "- vim")
				assert.Contains(t, result, "priority: critical")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := crud.GenerateSkillTemplate(tt.skillName, tt.description, tt.priority, tt.targets, tt.content)

			assert.NotEmpty(t, result)
			tt.checkFn(t, result)
		})
	}
}

// TestEnsureTrailingNewline tests ensuring files end with newline
func TestEnsureTrailingNewline(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no trailing newline", "content", "content\n"},
		{"with trailing newline", "content\n", "content\n"},
		{"multiple lines no newline", "line1\nline2", "line1\nline2\n"},
		{"multiple lines with newline", "line1\nline2\n", "line1\nline2\n"},
		{"empty string", "", ""},
		{"whitespace only", "   ", "   \n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := crud.EnsureTrailingNewline(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNormalizeTargets tests target normalization
func TestNormalizeTargets(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			"single target",
			[]string{"claude"},
			[]string{"claude"},
		},
		{
			"multiple targets",
			[]string{"claude", "cursor", "vim"},
			[]string{"claude", "cursor", "vim"},
		},
		{
			"empty array",
			[]string{},
			[]string{},
		},
		{
			"nil array",
			nil,
			[]string{},
		},
		{
			"with duplicates",
			[]string{"claude", "claude", "cursor"},
			[]string{"claude", "cursor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := crud.NormalizeTargets(tt.input)
			// Check if same length and contents match
			assert.Equal(t, len(tt.expected), len(result))
			for _, exp := range tt.expected {
				assert.Contains(t, result, exp)
			}
		})
	}
}

// TestFrontmatterStructure tests the structure of generated frontmatter
func TestFrontmatterStructure(t *testing.T) {
	frontmatter := crud.GenerateFrontmatter("high", []string{"claude", "cursor"})

	lines := strings.Split(frontmatter, "\n")

	// Should start and end with ---
	assert.Equal(t, "---", lines[0])

	// Should have closing --- before empty line
	foundClosing := false
	for i, line := range lines {
		if line == "---" && i > 0 {
			foundClosing = true
			break
		}
	}
	assert.True(t, foundClosing)

	// Priority should be present
	assert.True(t, func() bool {
		for _, line := range lines {
			if strings.Contains(line, "priority:") {
				return true
			}
		}
		return false
	}())
}

// TestTemplateContentConsistency ensures generated templates are consistent
func TestTemplateContentConsistency(t *testing.T) {
	// Generate multiple templates with same inputs (no custom content)
	rule1 := crud.GenerateRuleTemplate("my-rule", "high", []string{"claude"}, "")
	rule2 := crud.GenerateRuleTemplate("my-rule", "high", []string{"claude"}, "")

	// Should be identical
	assert.Equal(t, rule1, rule2)

	// Different rule names should produce different outputs (due to heading)
	rule3 := crud.GenerateRuleTemplate("different-rule", "high", []string{"claude"}, "")
	assert.NotEqual(t, rule1, rule3)
}
