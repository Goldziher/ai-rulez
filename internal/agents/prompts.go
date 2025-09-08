package agents

import (
	"fmt"
	"strings"
)

// Task-specific prompt functions for direct YAML editing

func buildProjectAnalysisPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Analyze the '%s' project and update the description in ai_rulez.yaml\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the current ai_rulez.yaml file to understand the existing configuration\n")
	prompt.WriteString("2. Analyze the project structure, technologies, and architecture\n")
	prompt.WriteString("3. Use the Edit tool to update the metadata.description field with a concise project description (2-3 sentences)\n\n")

	// Add rich project context
	addProjectContext(&prompt, context)

	prompt.WriteString("\nFocus on what the project does and its key technical characteristics.\n")

	return prompt.String()
}

func buildDocumentationPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add documentation sections to ai_rulez.yaml for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the current ai_rulez.yaml file\n")
	prompt.WriteString("2. Add 4-6 key documentation sections to the `sections:` array\n")
	prompt.WriteString("3. Use the Edit tool to modify ai_rulez.yaml directly\n\n")

	prompt.WriteString("Each section should have:\n")
	prompt.WriteString("- name: \"Section Title\"\n")
	prompt.WriteString("- priority: critical|high|medium|low\n")
	prompt.WriteString("- content: |\n")
	prompt.WriteString("    Multi-line description of what developers need to know\n\n")

	// Add rich project context
	addProjectContext(&prompt, context)

	// Add existing documentation context
	if len(context.MarkdownFiles) > 0 {
		prompt.WriteString("\nEXISTING DOCUMENTATION:\n")
		for _, doc := range context.MarkdownFiles {
			if doc.Category == "root" || doc.Category == "docs" {
				prompt.WriteString(fmt.Sprintf("- @%s - %s\n", doc.RelativePath, doc.Title))
			}
		}
	}

	prompt.WriteString("\nFocus on essential areas like architecture, development workflow, testing, deployment, etc.\n")
	prompt.WriteString("Reference existing documentation with @ notation when relevant.\n")

	return prompt.String()
}

func buildStandardsPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add coding standards to ai_rulez.yaml for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the current ai_rulez.yaml file\n")
	prompt.WriteString("2. Add 5-7 important coding standards to the `rules:` array\n")
	prompt.WriteString("3. Use the Edit tool to modify ai_rulez.yaml directly\n\n")

	prompt.WriteString("Each rule should be a separate array entry:\n")
	prompt.WriteString("- name: \"Specific Standard Name\"\n")
	prompt.WriteString("- priority: critical|high|medium|low\n")
	prompt.WriteString("- content: \"Clear, actionable description with examples if helpful\"\n\n")

	// Add rich project context
	addProjectContext(&prompt, context)

	prompt.WriteString("\nFocus on critical practices that prevent bugs and maintain code quality.\n")
	prompt.WriteString("Base standards on the detected technologies, commands, and project structure.\n")

	return prompt.String()
}

func buildSpecialistPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add specialized AI agents to ai_rulez.yaml for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the current ai_rulez.yaml file\n")
	prompt.WriteString("2. Add 3-4 specialized AI agents to the `agents:` array\n")
	prompt.WriteString("3. Use the Edit tool to modify ai_rulez.yaml directly\n\n")

	prompt.WriteString("Each agent should be a separate array entry:\n")
	prompt.WriteString("- name: \"agent-role-name\"\n")
	prompt.WriteString("- description: \"One sentence describing their specialty\"\n")
	prompt.WriteString("- priority: high|medium|low\n")
	prompt.WriteString("- system_prompt: |\n")
	prompt.WriteString("    Multi-line prompt describing their role and expertise\n\n")

	// Add rich project context
	addProjectContext(&prompt, context)

	// Add agent suggestions based on project characteristics
	addAgentSuggestions(&prompt, context)

	prompt.WriteString("\nFocus on agents that address complex, domain-specific tasks for this project.\n")
	prompt.WriteString("Reference actual project tools, commands, and file paths in agent system prompts.\n")

	return prompt.String()
}

// addProjectContext adds comprehensive project context information to prompts
func addProjectContext(prompt *strings.Builder, context *ProjectContext) {
	prompt.WriteString("PROJECT CONTEXT:\n")
	fmt.Fprintf(prompt, "- Repository Type: %s\n", context.RepoType)
	fmt.Fprintf(prompt, "- Root Path: %s\n", context.RootPath)

	if context.CodebaseInfo != nil {
		info := context.CodebaseInfo
		if info.MainLanguage != "" {
			fmt.Fprintf(prompt, "- Primary Language: %s\n", info.MainLanguage)
		}
		if len(info.TechStack) > 0 {
			fmt.Fprintf(prompt, "- Technologies: %s\n", strings.Join(info.TechStack, ", "))
		}
		fmt.Fprintf(prompt, "- Project Type: %s\n", info.ProjectType)

		// Development commands
		if info.BuildCommand != "" {
			fmt.Fprintf(prompt, "- Build Command: %s\n", info.BuildCommand)
		}
		if info.TestCommand != "" {
			fmt.Fprintf(prompt, "- Test Command: %s\n", info.TestCommand)
		}
		if info.LintCommand != "" {
			fmt.Fprintf(prompt, "- Lint Command: %s\n", info.LintCommand)
		}

		// Capabilities
		if info.HasDatabase {
			prompt.WriteString("- Uses Database\n")
		}
		if info.HasDocker {
			prompt.WriteString("- Uses Docker\n")
		}
		if info.HasMCP {
			fmt.Fprintf(prompt, "- MCP Server: %s\n", info.MCPCommand)
		}
	}

	// Monorepo structure
	if context.RepoType == "monorepo" {
		if len(context.PackageLocations) > 0 {
			fmt.Fprintf(prompt, "- Packages (%d): %s\n", len(context.PackageLocations), strings.Join(context.PackageLocations, ", "))
		}
		if len(context.AppLocations) > 0 {
			fmt.Fprintf(prompt, "- Applications (%d): %s\n", len(context.AppLocations), strings.Join(context.AppLocations, ", "))
		}
	}

	// Project structure tree
	if structure := context.GenerateStructureTree(); structure != "" {
		prompt.WriteString("\nPROJECT STRUCTURE:\n```\n")
		prompt.WriteString(structure)
		prompt.WriteString("```\n")
	}
}

// addAgentSuggestions suggests relevant agent types based on project characteristics
func addAgentSuggestions(prompt *strings.Builder, context *ProjectContext) {
	prompt.WriteString("\nSUGGESTED AGENT TYPES based on your project:\n")

	if context.RepoType == "monorepo" {
		prompt.WriteString("- Monorepo architect (cross-package dependencies, workspace management)\n")
	}

	if context.CodebaseInfo != nil {
		info := context.CodebaseInfo

		if info.HasDatabase {
			prompt.WriteString("- Database specialist (schema, migrations, queries)\n")
		}
		if info.HasDocker {
			prompt.WriteString("- DevOps engineer (deployment, infrastructure, containerization)\n")
		}

		// Language-specific agents
		switch info.MainLanguage {
		case "Go":
			prompt.WriteString("- Go developer (concurrency, performance, idioms)\n")
		case "JavaScript", "TypeScript":
			prompt.WriteString("- Frontend specialist (React/Next.js, state management)\n")
		case "Python":
			prompt.WriteString("- Python specialist (async patterns, data processing)\n")
		}

		// Framework-specific
		for _, tech := range info.TechStack {
			if strings.Contains(tech, "React") || strings.Contains(tech, "Next") {
				prompt.WriteString("- React specialist (components, hooks, performance)\n")
				break
			}
		}
	}
}
