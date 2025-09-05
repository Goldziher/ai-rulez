package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
)

func (g *Generator) writeOutputFile(output *config.Output, data *templates.TemplateData) error {
	if output.IsDirectory() {
		return g.writeDirectoryOutput(output, data)
	}
	return g.writeSingleFile(output, data)
}

func (g *Generator) writeSingleFile(output *config.Output, data *templates.TemplateData) error {
	localData := *data
	localData.ConfigFile = g.configFile
	localData.OutputFile = output.Path
	data = &localData

	content, err := g.renderTemplate(output, data)
	if err != nil {
		return err
	}

	if g.isMCPTemplate(output) {
		return g.writeFileWithoutHeader(output.Path, content)
	}

	return g.writeContentToFile(output.Path, content, data)
}

func (g *Generator) writeDirectoryOutput(output *config.Output, data *templates.TemplateData) error {
	outputType := output.GetOutputType()
	namingScheme := output.GetNamingScheme()
	dirPath := output.Path

	fullDirPath := filepath.Join(g.baseDir, dirPath)
	if err := os.MkdirAll(fullDirPath, 0o755); err != nil {
		return oops.
			With("path", fullDirPath).
			With("operation", "create directory").
			Hint(fmt.Sprintf("Ensure you have write permissions for %s", fullDirPath)).
			Wrapf(err, "failed to create directory")
	}

	switch outputType {
	case "agent":
		return g.writeAgentFiles(dirPath, namingScheme, output, data)
	case "rule":
		return g.writeRulesFile(dirPath, namingScheme, output, data)
	default:
		return g.writeRulesFile(dirPath, namingScheme, output, data)
	}
}

func (g *Generator) writeAgentFiles(dirPath, namingScheme string, output *config.Output, data *templates.TemplateData) error {
	for i := range data.Agents {
		agent := &data.Agents[i]
		sanitizedName := sanitizeFilename(agent.Name)

		filename := strings.ReplaceAll(namingScheme, "{name}", sanitizedName)
		filename = strings.ReplaceAll(filename, "{index:03d}", fmt.Sprintf("%03d", i+1))
		filename = strings.ReplaceAll(filename, "{index:02d}", fmt.Sprintf("%02d", i+1))
		filename = strings.ReplaceAll(filename, "{index}", fmt.Sprintf("%d", i+1))
		filename = strings.ReplaceAll(filename, "{type}", "agent")
		filePath := filepath.Join(dirPath, filename)

		content, err := g.renderAgentTemplate(output, agent, data)
		if err != nil {
			return oops.
				With("path", filePath).
				With("agent", agent.Name).
				Hint("Check the agent template syntax").
				Wrapf(err, "render agent template")
		}

		if err := g.writeAgentContentToFile(filePath, content); err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) writeRulesFile(dirPath, namingScheme string, output *config.Output, data *templates.TemplateData) error {
	hasPlaceholders := strings.Contains(namingScheme, "{name}") ||
		strings.Contains(namingScheme, "{priority}") ||
		strings.Contains(namingScheme, "{type}") ||
		strings.Contains(namingScheme, "{index")

	if !hasPlaceholders {
		filePath := filepath.Join(dirPath, namingScheme)
		localData := *data
		localData.ConfigFile = g.configFile
		localData.OutputFile = filePath

		content, err := g.renderTemplate(output, &localData)
		if err != nil {
			return err
		}

		return g.writeContentToFile(filePath, content, &localData)
	}

	for i := range data.Rules {
		rule := &data.Rules[i]
		sanitizedName := sanitizeFilename(rule.Name)

		filename := strings.ReplaceAll(namingScheme, "{name}", sanitizedName)
		filename = strings.ReplaceAll(filename, "{priority}", fmt.Sprintf("%d", rule.Priority.ToInt()))
		filename = strings.ReplaceAll(filename, "{index:02d}", fmt.Sprintf("%02d", i+1))
		filename = strings.ReplaceAll(filename, "{index}", fmt.Sprintf("%d", i+1))
		filename = strings.ReplaceAll(filename, "{type}", "rule")
		filePath := filepath.Join(dirPath, filename)

		content, err := g.renderRuleTemplate(output, rule, data)
		if err != nil {
			return oops.
				With("path", filePath).
				With("rule", rule.Name).
				Hint("Check the rule template syntax").
				Wrapf(err, "render rule template")
		}

		if err := g.writeContentToFile(filePath, content, data); err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) writeContentToFile(filePath, content string, data *templates.TemplateData) error {
	header := templates.GenerateHeader(data)
	finalContent := header + content

	shouldWrite, err := g.shouldWriteFile(filePath, finalContent)
	if err != nil {
		return err
	}
	if !shouldWrite {
		return nil
	}

	return g.writeFile(filePath, finalContent)
}

func (g *Generator) writeAgentContentToFile(filePath, content string) error {
	shouldWrite, err := g.shouldWriteFile(filePath, content)
	if err != nil {
		return err
	}
	if !shouldWrite {
		return nil
	}
	return g.writeFile(filePath, content)
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"<", "-",
		">", "-",
		"|", "-",
		"\"", "-",
	)
	return replacer.Replace(name)
}

func (g *Generator) isMCPTemplate(output *config.Output) bool {
	template, err := output.GetTemplate()
	if err != nil {
		return false
	}

	if template == nil {
		return false
	}

	if template.Type == config.TemplateBuiltin {
		mcpTemplates := []string{
			"claude-code-mcp",
			"cursor-mcp",
			"windsurf-mcp",
			"vscode-mcp",
			"continuedev-mcp",
			"cline-mcp",
		}

		for _, mcpTemplate := range mcpTemplates {
			if template.Value == mcpTemplate {
				return true
			}
		}
	}

	return false
}

func (g *Generator) writeFileWithoutHeader(filePath, content string) error {
	shouldWrite, err := g.shouldWriteFile(filePath, content)
	if err != nil {
		return err
	}
	if !shouldWrite {
		return nil
	}

	return g.writeFile(filePath, content)
}
