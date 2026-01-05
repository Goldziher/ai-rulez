package presets

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

func init() {
	config.RegisterPresetV3("gemini", &GeminiPresetGenerator{})
}

// GeminiPresetGenerator generates Gemini preset files
type GeminiPresetGenerator struct{}

// generatePresetHeader creates a header for Gemini preset files
func generateGeminiPresetHeader(cfg *config.ConfigV3, outputPath string, ruleCount, sectionCount, agentCount int) string {
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

func (g *GeminiPresetGenerator) GetName() string {
	return "gemini"
}

func (g *GeminiPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".gemini"),
		filepath.Join(baseDir, "GEMINI.md"),
	}
}

func (g *GeminiPresetGenerator) Generate(content *config.ContentTreeV3, baseDir string, cfg *config.ConfigV3) ([]config.OutputFileV3, error) {
	var outputs []config.OutputFileV3

	// Create .gemini directory
	outputs = append(outputs, config.OutputFileV3{
		Path:  filepath.Join(baseDir, ".gemini"),
		IsDir: true,
	})

	// Generate settings.json with MCP configuration
	settingsContent, err := g.renderSettingsJSON(cfg)
	if err != nil {
		return nil, fmt.Errorf("render settings.json: %w", err)
	}

	outputs = append(outputs, config.OutputFileV3{
		Path:    filepath.Join(baseDir, ".gemini", "settings.json"),
		Content: settingsContent,
	})

	// Generate GEMINI.md with all rules and context
	geminiMD := g.renderGeminiMarkdown(content, cfg)
	outputs = append(outputs, config.OutputFileV3{
		Path:    filepath.Join(baseDir, "GEMINI.md"),
		Content: geminiMD,
	})

	return outputs, nil
}

func (g *GeminiPresetGenerator) renderSettingsJSON(cfg *config.ConfigV3) (string, error) {
	// Generate MCP settings for Gemini
	settings := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"ai-rulez": map[string]interface{}{
				"command": "npx",
				"args": []string{
					"-y",
					"ai-rulez@latest",
					"mcp",
				},
			},
		},
	}

	jsonData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal JSON: %w", err)
	}

	return string(jsonData), nil
}

func (g *GeminiPresetGenerator) renderGeminiMarkdown(content *config.ContentTreeV3, cfg *config.ConfigV3) string {
	var builder strings.Builder

	// Calculate content counts
	allRules := combineContentFiles(content.Rules, getAllDomainRules(content))
	allAgents := combineContentFiles(content.Agents, getAllDomainAgents(content))

	// Add header before title
	header := generateGeminiPresetHeader(cfg, "GEMINI.md", len(allRules), 0, len(allAgents))
	builder.WriteString(header)

	// Add title
	builder.WriteString("# ")
	builder.WriteString(cfg.Name)
	builder.WriteString("\n\n")

	if cfg.Description != "" {
		builder.WriteString(cfg.Description)
		builder.WriteString("\n\n")
	}

	// Add rules section
	if len(allRules) > 0 {
		builder.WriteString("## Rules\n\n")
		for _, rule := range allRules {
			builder.WriteString("### ")
			builder.WriteString(rule.Name)
			builder.WriteString("\n\n") // Add blank line after heading

			if rule.Metadata != nil && rule.Metadata.Priority != "" {
				builder.WriteString("**Priority:** ")
				builder.WriteString(rule.Metadata.Priority)
				builder.WriteString("\n\n")
			}

			processedContent := markdown.ProcessEmbeddedContent(rule.Content)
			builder.WriteString(processedContent)
			builder.WriteString("\n\n")
		}
	}

	// Add context section
	allContext := combineContentFiles(content.Context, getAllDomainContext(content))
	if len(allContext) > 0 {
		builder.WriteString("## Context\n\n")
		for _, ctx := range allContext {
			builder.WriteString("### ")
			builder.WriteString(ctx.Name)
			builder.WriteString("\n\n")

			processedContent := markdown.ProcessEmbeddedContent(ctx.Content)
			builder.WriteString(processedContent)
			builder.WriteString("\n\n")
		}
	}

	// Skills should not be inlined in GEMINI.md

	return builder.String()
}
