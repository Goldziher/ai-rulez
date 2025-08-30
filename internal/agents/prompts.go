package agents

import (
	"fmt"
	"strings"
)

type PromptTemplate struct {
	Task        string
	Constraints []string
	Example     string
	Format      string
	MaxRetries  int
}

func GeneratePrompt(agent AgentInfo, projectName string, providers []string) string {
	template := getPromptTemplate(agent)

	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Generate an ai_rulez.yaml configuration file for project '%s'.\n\n", projectName))

	prompt.WriteString("Requirements:\n")
	for _, constraint := range template.Constraints {
		prompt.WriteString(fmt.Sprintf("- %s\n", constraint))
	}
	prompt.WriteString("\n")

	if len(providers) > 0 {
		prompt.WriteString("Include output configurations for:\n")
		for _, provider := range providers {
			prompt.WriteString(fmt.Sprintf("- %s\n", provider))
		}
		prompt.WriteString("\n")
	}

	if template.Example != "" {
		prompt.WriteString("Example structure:\n```yaml\n")
		prompt.WriteString(template.Example)
		prompt.WriteString("\n```\n\n")
	}

	prompt.WriteString(fmt.Sprintf("Output format: %s\n", template.Format))
	prompt.WriteString("Respond with ONLY the YAML content, no explanations or markdown code blocks.\n")

	return prompt.String()
}

func getPromptTemplate(agent AgentInfo) PromptTemplate {
	baseConstraints := []string{
		"Must be valid YAML",
		"Must include $schema field pointing to https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json",
		"Must include metadata section with name, version, and description",
		"Must include outputs section with appropriate file/path configurations",
		"Must include at least 5 practical rules",
		"Rules must have name, priority (1-10), and content fields",
		"Content should be specific and actionable",
	}

	example := `$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json

metadata:
  name: "Project Name"
  version: "1.0.0"
  description: "AI assistant rules configuration"

outputs:
  - file: "CLAUDE.md"
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{name}.md"

agents:
  - name: "test-writer"
    description: "Specialized agent for writing tests"
    system_prompt: "You are a test writing specialist..."

rules:
  - name: "code-style"
    priority: 10
    content: "Follow the project's existing code style and conventions"
  - name: "error-handling"
    priority: 9
    content: "Always implement proper error handling with meaningful messages"`

	switch agent.ID {
	case "claude":
		return PromptTemplate{
			Task:        "Generate ai_rulez.yaml configuration",
			Constraints: append(baseConstraints, "Include Claude-specific agent configurations if requested"),
			Example:     example,
			Format:      "yaml",
			MaxRetries:  3,
		}

	case "amp", "codex", "cursor":
		return PromptTemplate{
			Task:        "Generate ai_rulez.yaml configuration",
			Constraints: append(baseConstraints, "Include AGENTS.md in outputs for tool integration"),
			Example:     example,
			Format:      "yaml",
			MaxRetries:  3,
		}

	case "gemini":
		return PromptTemplate{
			Task:        "Generate ai_rulez.yaml configuration",
			Constraints: append(baseConstraints, "Include GEMINI.md in outputs"),
			Example:     example,
			Format:      "yaml",
			MaxRetries:  3,
		}

	default:
		return PromptTemplate{
			Task:        "Generate ai_rulez.yaml configuration",
			Constraints: baseConstraints,
			Example:     example,
			Format:      "yaml",
			MaxRetries:  2,
		}
	}
}

func GenerateFeedbackPrompt(originalPrompt string, previousOutput string, validationErrors []string) string {
	var prompt strings.Builder

	prompt.WriteString("The previous configuration had validation errors. Please fix them.\n\n")

	prompt.WriteString("Validation errors:\n")
	for _, err := range validationErrors {
		prompt.WriteString(fmt.Sprintf("- %s\n", err))
	}
	prompt.WriteString("\n")

	prompt.WriteString("Previous output:\n```yaml\n")
	prompt.WriteString(previousOutput)
	prompt.WriteString("\n```\n\n")

	prompt.WriteString("Please generate a corrected version that addresses all validation errors.\n")
	prompt.WriteString("Respond with ONLY the corrected YAML content.\n")

	return prompt.String()
}
