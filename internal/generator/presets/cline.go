package presets

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

func init() {
	config.RegisterPresetV3("cline", &ClinePresetGenerator{})
}

// ClinePresetGenerator generates Cline preset files
type ClinePresetGenerator struct{}

// generateClinePresetHeader creates a header for Cline preset files
func generateClinePresetHeader(cfg *config.ConfigV3, outputPath string, ruleCount, sectionCount, agentCount int) string {
	// Create TemplateData for header generation
	data := &templates.TemplateData{
		ProjectName:  cfg.Name,
		Timestamp:    time.Now(),
		ConfigFile:   "config.yaml", // V3 uses config.yaml
		OutputFile:   outputPath,
		Config:       cfg,
		RuleCount:    ruleCount,
		SectionCount: sectionCount,
		AgentCount:   agentCount,
	}

	return templates.GenerateHeader(data)
}

func (g *ClinePresetGenerator) GetName() string {
	return "cline"
}

func (g *ClinePresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".clinerules"),
		filepath.Join(baseDir, ".cline"),
		filepath.Join(baseDir, ".cline", "skills"),
	}
}

func (g *ClinePresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create directory structure
	outputs = append(outputs,
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".clinerules"),
			IsDir: true,
		},
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".cline"),
			IsDir: true,
		},
		config.OutputFileV3{
			Path:  filepath.Join(baseDir, ".cline", "skills"),
			IsDir: true,
		},
	)

	// Combine all rules from root and domains
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))

	// Generate rule files
	for _, rule := range allRules {
		outputPath := filepath.Join(".clinerules", sanitizeName(rule.Name)+".md")
		ruleContent := g.renderRuleFile(rule, cfg, outputPath, len(allRules))
		sanitized := sanitizeName(rule.Name)

		outputs = append(outputs, config.OutputFileV3{
			Path:    filepath.Join(baseDir, ".clinerules", sanitized+".md"),
			Content: ruleContent,
		})
	}

	// Generate skill files to .cline/skills/
	allSkills := combineContentFiles(content.Skills, getAllDomainSkills(content))
	for _, skill := range allSkills {
		skillID := extractSkillID(skill.Path)

		skillDir := filepath.Join(baseDir, ".cline", "skills", skillID)
		outputs = append(outputs,
			config.OutputFileV3{
				Path:  skillDir,
				IsDir: true,
			},
			config.OutputFileV3{
				Path:    filepath.Join(skillDir, "SKILL.md"),
				Content: g.renderSkillFile(skill),
			},
		)
	}

	return outputs, nil
}

// renderSkillFile renders a skill file in SKILL.md format for Cline
func (g *ClinePresetGenerator) renderSkillFile(skill config.ContentFile) string {
	var builder strings.Builder

	builder.WriteString("---\n")
	builder.WriteString("name: ")
	builder.WriteString(skill.Name)
	builder.WriteString("\n")
	builder.WriteString("description: ")
	builder.WriteString(quoteYAMLString(config.SkillDescriptionForContent(skill)))
	builder.WriteString("\n")
	builder.WriteString("---\n\n")
	builder.WriteString(skill.Content)

	return builder.String()
}

func (g *ClinePresetGenerator) renderRuleFile(rule config.ContentFile, cfg *config.ConfigV3, outputPath string, ruleCount int) string {
	var builder strings.Builder

	// Generate and prepend header
	header := generateClinePresetHeader(cfg, outputPath, ruleCount, 0, 0)
	builder.WriteString(header)

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
	processedContent := markdown.ProcessEmbeddedContent(rule.Content)
	builder.WriteString(processedContent)

	return builder.String()
}
