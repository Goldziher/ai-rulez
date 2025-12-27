package crud

import (
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Template represents a frontmatter template for content files
type Template struct {
	Priority string
	Targets  []string
	Content  string
}

// GenerateFrontmatter generates YAML frontmatter from a Template
func GenerateFrontmatter(priority string, targets []string) string {
	if priority == "" {
		priority = "medium"
	}

	// Normalize targets
	targets = NormalizeTargets(targets)

	lines := []string{
		"---",
		fmt.Sprintf("priority: %s", priority),
	}

	// Add targets if present
	if len(targets) > 0 {
		lines = append(lines, "targets:")
		for _, target := range targets {
			lines = append(lines, fmt.Sprintf("  - %s", target))
		}
	}

	lines = append(lines, "---", "")

	return strings.Join(lines, "\n")
}

// GenerateRuleTemplate generates a template for a rule file
func GenerateRuleTemplate(name, priority string, targets []string, content string) string {
	frontmatter := GenerateFrontmatter(priority, targets)
	body := EnsureTrailingNewline(content)

	if content == "" {
		caser := cases.Title(language.English)
		body = fmt.Sprintf("# %s\n\nYour rule content here...\n", caser.String(strings.ReplaceAll(name, "-", " ")))
	}

	return frontmatter + body
}

// GenerateContextTemplate generates a template for a context file
func GenerateContextTemplate(name, priority string, targets []string, content string) string {
	frontmatter := GenerateFrontmatter(priority, targets)
	body := EnsureTrailingNewline(content)

	if content == "" {
		caser := cases.Title(language.English)
		body = fmt.Sprintf("# %s\n\nYour context information here...\n", caser.String(strings.ReplaceAll(name, "-", " ")))
	}

	return frontmatter + body
}

// GenerateSkillTemplate generates a template for a skill file
func GenerateSkillTemplate(name, description, priority string, targets []string, content string) string {
	frontmatter := GenerateFrontmatter(priority, targets)

	// Add description to frontmatter if provided
	if description != "" {
		// Insert description before ---
		lines := strings.Split(frontmatter, "\n")
		// Remove the closing ---
		lines = lines[:len(lines)-2] // Remove empty line and closing ---
		lines = append(lines, fmt.Sprintf("description: %q", description), "---", "")
		frontmatter = strings.Join(lines, "\n")
	}

	body := EnsureTrailingNewline(content)

	if content == "" {
		caser := cases.Title(language.English)
		body = fmt.Sprintf("# %s\n\n## Overview\n\n%s\n\n## Usage\n\nYour skill usage information here...\n",
			caser.String(strings.ReplaceAll(name, "-", " ")),
			description)
	}

	return frontmatter + body
}

// GenerateEmptyRuleTemplate generates an empty rule template with just frontmatter
func GenerateEmptyRuleTemplate() string {
	return GenerateFrontmatter("medium", []string{})
}

// GenerateEmptyContextTemplate generates an empty context template with just frontmatter
func GenerateEmptyContextTemplate() string {
	return GenerateFrontmatter("medium", []string{})
}

// GenerateEmptySkillTemplate generates an empty skill template with just frontmatter
func GenerateEmptySkillTemplate() string {
	return GenerateFrontmatter("medium", []string{})
}
