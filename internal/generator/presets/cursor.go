package presets

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	config.RegisterPresetV3("cursor", &CursorPresetGenerator{})
}

const presetNameCursor = "cursor"

// CursorPresetGenerator generates Cursor preset files
type CursorPresetGenerator struct{}

func (g *CursorPresetGenerator) GetName() string {
	return presetNameCursor
}

func (g *CursorPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".cursor"),
		filepath.Join(baseDir, ".cursor", "rules"),
		filepath.Join(baseDir, ".cursor", "skills"),
		filepath.Join(baseDir, ".cursor", "commands"),
	}
}

func (g *CursorPresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .cursor directory structure
	outputs = append(outputs,
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".cursor"),
			IsDir: true,
		},
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".cursor", "rules"),
			IsDir: true,
		},
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".cursor", "skills"),
			IsDir: true,
		},
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".cursor", "commands"),
			IsDir: true,
		},
	)

	// Combine all rules from root and domains
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))

	// Generate rule files
	for _, rule := range allRules {
		ruleContent := g.renderRuleFile(rule)
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".cursor", "rules", sanitized+".mdc"),
			Content: ruleContent,
		})
	}

	// Combine all commands from root and domains
	allCommands := combineContentFiles(content.Commands, getAllDomainCommands(content))

	// Generate command files to .cursor/commands/
	for _, command := range allCommands {
		// Check if command should be included (enabled and targets Cursor if specified)
		if g.shouldIncludeCommand(command) {
			commandContent := g.renderCommandFile(command)
			sanitized := sanitizeName(command.Name)

			outputs = append(outputs, config.OutputFileV3{
				Path:    filepath.Join(baseDir, ".cursor", "commands", sanitized+".md"),
				Content: commandContent,
			})
		}
	}

	// Combine all skills from root and domains
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))

	// Generate skill files to .cursor/skills/
	for _, skill := range allSkills {
		// Check if skill should be included (enabled and targets Cursor if specified)
		if !g.shouldIncludeSkill(skill) {
			continue
		}

		skillID := extractSkillID(skill.Path)

		// Create skill directory
		skillDir := filepath.Join(baseDir, ".cursor", "skills", skillID)
		outputs = append(outputs, config.OutputFileV3{
			Path:  skillDir,
			IsDir: true,
		})

		// Generate SKILL.md file
		skillContent := g.renderSkillFile(skill, cfg)
		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(skillDir, "SKILL.md"),
			Content: skillContent,
		})
	}

	return outputs, nil
}

func (g *CursorPresetGenerator) renderRuleFile(rule config.ContentFile) string {
	var builder strings.Builder

	// Add title
	builder.WriteString("# ")
	builder.WriteString(rule.Name)
	builder.WriteString("\n\n")

	// Add priority if present
	if rule.Metadata != nil && rule.Metadata.Priority != "" {
		builder.WriteString("**Priority:** ")
		builder.WriteString(rule.Metadata.Priority)
		builder.WriteString("\n\n")
	}

	// Add content
	builder.WriteString(rule.Content)

	return builder.String()
}

// shouldIncludeCommand checks if a command should be included in the Cursor preset
func (g *CursorPresetGenerator) shouldIncludeCommand(command config.ContentFile) bool {
	// Include if no metadata (no restrictions)
	if command.Metadata == nil {
		return true
	}

	// If targets are specified, only include if Cursor is in targets
	if len(command.Metadata.Targets) > 0 {
		for _, target := range command.Metadata.Targets {
			if target == presetNameCursor {
				return true
			}
		}
		return false
	}

	// No targets specified, include by default
	return true
}

// renderCommandFile renders a command file in Markdown format for Cursor commands
func (g *CursorPresetGenerator) renderCommandFile(command config.ContentFile) string {
	var builder strings.Builder

	// Add title
	builder.WriteString("# /")
	builder.WriteString(command.Name)
	builder.WriteString("\n\n")

	// Add description if present in metadata
	if command.Metadata != nil && command.Metadata.Extra != nil {
		if desc, ok := command.Metadata.Extra["description"]; ok && desc != "" {
			builder.WriteString("**Description:** ")
			builder.WriteString(desc)
			builder.WriteString("\n\n")
		}
	}

	// Add usage if present in metadata
	if command.Metadata != nil && command.Metadata.Usage != "" {
		builder.WriteString("**Usage:** `")
		builder.WriteString(command.Metadata.Usage)
		builder.WriteString("`\n\n")
	}

	// Add aliases if present in metadata
	if command.Metadata != nil && len(command.Metadata.Aliases) > 0 {
		builder.WriteString("**Aliases:** ")
		for i, alias := range command.Metadata.Aliases {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString("`/")
			builder.WriteString(alias)
			builder.WriteString("`")
		}
		builder.WriteString("\n\n")
	}

	// Add command content
	builder.WriteString(command.Content)

	return builder.String()
}

// shouldIncludeSkill checks if a skill should be included in the Cursor preset
func (g *CursorPresetGenerator) shouldIncludeSkill(skill config.ContentFile) bool {
	// Include if no metadata (no restrictions)
	if skill.Metadata == nil {
		return true
	}

	// If targets are specified, only include if Cursor is in targets
	if len(skill.Metadata.Targets) > 0 {
		for _, target := range skill.Metadata.Targets {
			if target == presetNameCursor {
				return true
			}
		}
		return false
	}

	// No targets specified, include by default
	return true
}

// renderSkillFile renders a skill file in SKILL.md format for Cursor
func (g *CursorPresetGenerator) renderSkillFile(skill config.ContentFile, cfg *config.ConfigV3) string {
	var builder strings.Builder

	// Add YAML frontmatter
	builder.WriteString("---\n")
	builder.WriteString("name: ")
	builder.WriteString(skill.Name)
	builder.WriteString("\n")

	if skill.Metadata != nil {
		if skill.Metadata.Priority != "" {
			builder.WriteString("priority: ")
			builder.WriteString(skill.Metadata.Priority)
			builder.WriteString("\n")
		}

		if desc, ok := skill.Metadata.Extra["description"]; ok && desc != "" {
			builder.WriteString("description: ")
			builder.WriteString(desc)
			builder.WriteString("\n")
		}
	}

	builder.WriteString("---\n\n")

	// Add skill content
	builder.WriteString(skill.Content)

	return builder.String()
}
