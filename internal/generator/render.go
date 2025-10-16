package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

func (g *Generator) renderTemplate(output *config.Output, data *templates.TemplateData) (string, error) {
	template, err := output.GetTemplate()
	if err != nil {
		return "", oops.
			With("output_path", output.Path).
			Wrapf(err, "parse template configuration")
	}

	if template == nil {
		template = &config.Template{
			Type:  config.TemplateBuiltin,
			Value: "default",
		}
	}

	switch template.Type {
	case config.TemplateFile:
		return g.renderFileTemplate(template.Value, data)
	case config.TemplateInline:
		return g.renderInlineTemplate(template.Value, data)
	case config.TemplateBuiltin:
		return g.renderBuiltinTemplate(template.Value, data)
	default:
		return "", oops.
			With("template_type", string(template.Type)).
			With("template_value", template.Value).
			Errorf("unsupported template type: %s", template.Type)
	}
}

func (g *Generator) renderFileTemplate(templatePath string, data *templates.TemplateData) (string, error) {
	fullPath := filepath.Join(g.baseDir, templatePath)

	templateContent, err := os.ReadFile(fullPath)
	if err != nil {
		return "", oops.
			With("requested", templatePath).
			With("available", g.GetSupportedTemplates()).
			With("path", fullPath).
			With("template_type", "file").
			Hint(fmt.Sprintf("Use one of the available templates: %s\nUse 'default' for the standard template\nDefine a custom template inline using Go template syntax\nCheck if the template file exists: %s\nVerify the path is correct relative to %s", strings.Join(g.GetSupportedTemplates(), ", "), fullPath, g.baseDir)).
			Errorf("template file '%s' not found", templatePath)
	}

	templateID := fmt.Sprintf("file:%s", templatePath)
	if err := g.renderer.RegisterTemplate(templateID, string(templateContent)); err != nil {
		return "", oops.
			With("template", templateID).
			With("path", fullPath).
			With("template_type", "file").
			Hint("Check the template syntax - common issues: unclosed '{{', invalid dot notation\nValidate your template at: https://golang.org/pkg/text/template/\nUse a built-in template as a reference: 'default' or 'documentation'").
			Wrapf(err, "parse template")
	}

	content, err := g.renderer.Render(templateID, data)
	if err != nil {
		return "", oops.
			With("template", templateID).
			With("path", fullPath).
			With("template_type", "file").
			Hint("Check that all template variables are available in the data\nCommon issue: accessing a field that doesn't exist\nUse {{if}} checks for optional fields").
			Wrapf(err, "execute template")
	}
	return content, nil
}

func (g *Generator) renderInlineTemplate(templateContent string, data *templates.TemplateData) (string, error) {
	content, err := templates.RenderString(templateContent, data)
	if err != nil {
		return "", oops.
			With("template", "inline").
			With("template_type", "inline").
			With("template_content", templateContent).
			Hint("Check that all template variables are available in the data\nCommon issue: accessing a field that doesn't exist\nUse {{if}} checks for optional fields").
			Wrapf(err, "execute template")
	}
	return content, nil
}

func (g *Generator) renderBuiltinTemplate(templateName string, data *templates.TemplateData) (string, error) {
	if content, handled, err := templates.RenderSpecialBuiltin(templateName, data); handled {
		return content, err
	}

	content, err := g.renderer.Render(templateName, data)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", oops.
				With("requested", templateName).
				With("available", g.GetSupportedTemplates()).
				With("template_type", "builtin").
				Hint(fmt.Sprintf("Use one of the available templates: %s\nUse 'default' for the standard template\nDefine a custom template inline using Go template syntax\nReference an external template file using a file path", strings.Join(g.GetSupportedTemplates(), ", "))).
				Errorf("builtin template '%s' not found", templateName)
		}
		return "", oops.
			With("template", templateName).
			With("template_type", "builtin").
			Hint("Check that all template variables are available in the data\nCommon issue: accessing a field that doesn't exist\nUse {{if}} checks for optional fields").
			Wrapf(err, "execute template")
	}

	return content, nil
}

func (g *Generator) renderAgentTemplate(output *config.Output, agent *config.Agent, data *templates.TemplateData) (string, error) {
	frontmatterData := map[string]interface{}{
		"name":        agent.Name,
		"description": agent.Description,
	}
	if len(agent.Tools) > 0 {
		frontmatterData["tools"] = strings.Join(agent.Tools, ", ")
	}

	yamlBytes, err := yaml.Marshal(frontmatterData)
	if err != nil {
		return "", oops.
			With("agent_name", agent.Name).
			Wrapf(err, "marshal agent frontmatter")
	}

	frontmatter := "---\n" + string(yamlBytes) + "---\n\n"

	var systemPrompt string
	template, err := agent.GetTemplate()
	if err != nil {
		return "", oops.
			With("agent_name", agent.Name).
			Wrapf(err, "parse agent template configuration")
	}

	if template != nil {
		agentOutput := &config.Output{
			Template: agent.Template,
		}
		renderedPrompt, err := g.renderTemplate(agentOutput, data)
		if err != nil {
			return "", err
		}
		systemPrompt = renderedPrompt
	} else {
		systemPrompt = agent.SystemPrompt
	}

	return frontmatter + systemPrompt, nil
}

func (g *Generator) renderRuleTemplate(output *config.Output, rule *config.Rule, data *templates.TemplateData) (string, error) {
	ruleData := map[string]interface{}{
		"Name":     rule.Name,
		"Priority": rule.Priority,
		"Content":  rule.Content,
	}

	var templateContent string
	if output.Template != nil {
		template, err := output.GetTemplate()
		if err != nil {
			return "", err
		}
		if template != nil && template.Value != "" {
			templateContent = template.Value
		}
	}

	if templateContent == "" {
		templateContent = "# {{.Name}}\n\n**Priority:** {{.Priority}}\n\n{{.Content}}"
	}

	return templates.ExecuteTemplate(templateContent, ruleData)
}
