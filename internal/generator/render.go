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
	templateName := "default"
	if output.Template != "" {
		templateName = output.Template
	}

	// Handle file-based templates
	if strings.HasPrefix(templateName, "@") {
		templatePath := strings.TrimPrefix(templateName, "@")
		fullPath := filepath.Join(g.baseDir, templatePath)

		templateContent, err := os.ReadFile(fullPath)
		if err != nil {
			return "", oops.
				With("requested", templatePath).
				With("available", g.GetSupportedTemplates()).
				With("path", fullPath).
				With("template_type", "file").
				Hint(fmt.Sprintf("Use one of the available templates: %s\nUse 'default' for the standard template\nDefine a custom template inline using Go template syntax\nCheck if the template file exists: %s\nVerify the path is correct relative to %s", strings.Join(g.GetSupportedTemplates(), ", "), fullPath, g.baseDir)).
				Errorf("template '%s' not found", templatePath)
		}

		templateID := fmt.Sprintf("file:%s", templatePath)
		if err := g.renderer.RegisterTemplate(templateID, string(templateContent)); err != nil {
			err = oops.
				With("template", templateID).
				With("path", fullPath).
				With("template_type", "file").
				Hint("Check the template syntax - common issues: unclosed '{{', invalid dot notation\nValidate your template at: https://golang.org/pkg/text/template/\nUse a built-in template as a reference: 'default' or 'documentation'").
				Wrapf(err, "parse template")

			if strings.Contains(err.Error(), "line") {
				if oopsErr, ok := oops.AsOops(err); ok {
					err = oops.With("parse_error", err.Error()).Wrap(oopsErr)
				}
			}

			return "", err
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

	// Handle inline templates
	if strings.Contains(templateName, "\n") || strings.Contains(templateName, "{{") {
		content, err := templates.RenderString(templateName, data)
		if err != nil {
			return "", oops.
				With("template", "inline").
				With("template_type", "inline").
				With("template_content", templateName).
				Hint("Check that all template variables are available in the data\nCommon issue: accessing a field that doesn't exist\nUse {{if}} checks for optional fields").
				Wrapf(err, "execute template")
		}
		return content, nil
	}

	// Handle named templates
	content, err := g.renderer.Render(templateName, data)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", oops.
				With("requested", templateName).
				With("available", g.GetSupportedTemplates()).
				With("template_type", "named").
				Hint(fmt.Sprintf("Use one of the available templates: %s\nUse 'default' for the standard template\nDefine a custom template inline using Go template syntax\nReference an external template file using '@filename'", strings.Join(g.GetSupportedTemplates(), ", "))).
				Errorf("template '%s' not found", templateName)
		}
		return "", oops.
			With("template", templateName).
			With("template_type", "named").
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
		frontmatterData["tools"] = agent.Tools
	}

	yamlBytes, err := yaml.Marshal(frontmatterData)
	if err != nil {
		return "", oops.
			With("agent_name", agent.Name).
			Wrapf(err, "marshal agent frontmatter")
	}

	frontmatter := "---\n" + string(yamlBytes) + "---\n\n"

	var systemPrompt string
	if agent.Template != "" {
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
	// Create template data with individual rule fields accessible
	ruleData := map[string]interface{}{
		"Name":     rule.Name,
		"Priority": rule.Priority,
		"Content":  rule.Content,
	}
	
	// Use the custom template if provided, otherwise use a default rule template
	templateContent := output.Template
	if templateContent == "" {
		templateContent = "# {{.Name}}\n\n**Priority:** {{.Priority}}\n\n{{.Content}}"
	}
	
	return templates.ExecuteTemplate(templateContent, ruleData)
}
