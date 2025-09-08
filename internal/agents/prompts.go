package agents

import (
	"fmt"
	"os"
	"strings"
)

func buildPhase1Prompt(context *ProjectContext, previousOutputs []string) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Please analyze the software project '%s' and provide a technical overview.\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("- Focus on technical architecture and development patterns\n")
	prompt.WriteString("- Provide a brief project summary (1-2 sentences)\n")
	prompt.WriteString("- Describe the software architecture type\n")
	prompt.WriteString("- List key technical decisions and frameworks\n")
	prompt.WriteString("- Keep the analysis professional and technical\n\n")

	prompt.WriteString("DETECTED PROJECT DETAILS:\n")
	prompt.WriteString(fmt.Sprintf("- Repository Type: %s\n", context.RepoType))

	if context.CodebaseInfo != nil {
		info := context.CodebaseInfo
		if len(info.TechStack) > 0 {
			prompt.WriteString(fmt.Sprintf("- Tech Stack: %s\n", strings.Join(info.TechStack, ", ")))
		}
		if info.MainLanguage != "" {
			prompt.WriteString(fmt.Sprintf("- Main Language: %s\n", info.MainLanguage))
		}
		prompt.WriteString(fmt.Sprintf("- Project Type: %s\n", info.ProjectType))

		if info.BuildCommand != "" {
			prompt.WriteString(fmt.Sprintf("- Build Command: %s\n", info.BuildCommand))
		}
		if info.TestCommand != "" {
			prompt.WriteString(fmt.Sprintf("- Test Command: %s\n", info.TestCommand))
		}
		if info.LintCommand != "" {
			prompt.WriteString(fmt.Sprintf("- Lint Command: %s\n", info.LintCommand))
		}
	}

	if context.RepoType == repoTypeMonorepo {
		prompt.WriteString("\nMONOREPO STRUCTURE:\n")
		if len(context.PackageLocations) > 0 {
			prompt.WriteString(fmt.Sprintf("- Packages (%d): %s\n", len(context.PackageLocations), strings.Join(context.PackageLocations, ", ")))
		}
		if len(context.AppLocations) > 0 {
			prompt.WriteString(fmt.Sprintf("- Applications (%d): %s\n", len(context.AppLocations), strings.Join(context.AppLocations, ", ")))
		}
	}

	prompt.WriteString("\nPROJECT STRUCTURE:\n```\n")
	prompt.WriteString(context.GenerateStructureTree())
	prompt.WriteString("```\n\n")

	prompt.WriteString("DOCUMENTATION FILES:\n")
	prompt.WriteString(context.GetDocumentationSummary())
	prompt.WriteString("\n")

	prompt.WriteString("TASK: Provide a detailed analysis of this project including:\n")
	prompt.WriteString("1. A one-line description of what this project does\n")
	prompt.WriteString("2. The overall architecture pattern (monolithic, microservices, monorepo, etc.)\n")
	prompt.WriteString("3. Key technical decisions and patterns observed\n")
	prompt.WriteString("4. Development workflow based on available commands\n")
	prompt.WriteString("5. Any notable configurations or setups\n\n")

	prompt.WriteString("Format your response as structured text that will be used in the next phases.\n")
	prompt.WriteString("Be concise but comprehensive. Focus on facts from the actual codebase.\n")

	return prompt.String()
}

func buildPhase2Prompt(context *ProjectContext, previousOutputs []string) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Generate documentation sections for the '%s' project.\n\n", context.ProjectName))

	// Read existing config from file
	existingConfig := readExistingConfig()
	if existingConfig != "" {
		prompt.WriteString("CURRENT ai_rulez.yaml CONFIGURATION:\n")
		prompt.WriteString("```yaml\n")
		prompt.WriteString(existingConfig)
		prompt.WriteString("```\n\n")
	}

	if len(previousOutputs) > 0 {
		prompt.WriteString("PROJECT ANALYSIS FROM PHASE 1:\n")
		prompt.WriteString(previousOutputs[0])
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("EXISTING DOCUMENTATION:\n")
	for _, doc := range context.MarkdownFiles {
		if doc.Category == categoryRoot || doc.Category == categoryDocs {
			prompt.WriteString(fmt.Sprintf("- @%s - %s\n", doc.RelativePath, doc.Title))
		}
	}

	if context.RepoType == repoTypeMonorepo {
		prompt.WriteString("\nPACKAGE DOCUMENTATION:\n")
		for _, doc := range context.MarkdownFiles {
			if doc.Category == categoryPackage {
				prompt.WriteString(fmt.Sprintf("- @%s - %s\n", doc.RelativePath, doc.Title))
			}
		}

		prompt.WriteString("\nAPPLICATION DOCUMENTATION:\n")
		for _, doc := range context.MarkdownFiles {
			if doc.Category == categoryApp {
				prompt.WriteString(fmt.Sprintf("- @%s - %s\n", doc.RelativePath, doc.Title))
			}
		}
	}

	prompt.WriteString("\n")
	prompt.WriteString("TASK: Create YAML sections that document this codebase comprehensively.\n")
	prompt.WriteString("Each section should reference existing documentation using @ notation.\n\n")

	prompt.WriteString("REQUIREMENTS:\n")
	prompt.WriteString("1. Create a 'Project Structure' or 'Monorepo Structure' section (priority: high)\n")
	prompt.WriteString("2. Create a 'Documentation Hub' section listing all docs with @ references (priority: high)\n")
	prompt.WriteString("3. Add sections for key architectural components\n")
	prompt.WriteString("4. Include sections for development workflow if complex\n")
	prompt.WriteString("5. Reference actual README and doc files using @path notation\n\n")

	prompt.WriteString("FORMAT each section as:\n")
	prompt.WriteString("```yaml\n")
	prompt.WriteString("sections:\n")
	prompt.WriteString("  - name: \"Section Name\"\n")
	prompt.WriteString("    priority: high|medium|low\n")
	prompt.WriteString("    content: |\n")
	prompt.WriteString("      Content here with @references to docs\n")
	prompt.WriteString("```\n\n")

	if context.RepoType == repoTypeMonorepo {
		prompt.WriteString("For MONOREPO, ensure you:\n")
		prompt.WriteString("- Explain the monorepo organization\n")
		prompt.WriteString("- List all packages with their purposes\n")
		prompt.WriteString("- List all applications with their roles\n")
		prompt.WriteString("- Reference package-specific READMEs\n\n")
	}

	prompt.WriteString("Create 3-6 high-quality sections. Be specific to this project.\n")
	prompt.WriteString("Always link to existing documentation rather than duplicating content.\n")

	return prompt.String()
}

func buildPhase3Prompt(context *ProjectContext, previousOutputs []string) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Generate coding rules for the '%s' project.\n\n", context.ProjectName))

	// Read existing config from file
	existingConfig := readExistingConfig()
	if existingConfig != "" {
		prompt.WriteString("CURRENT ai_rulez.yaml CONFIGURATION:\n")
		prompt.WriteString("```yaml\n")
		prompt.WriteString(existingConfig)
		prompt.WriteString("```\n\n")
		prompt.WriteString("BUILD ON: Add rules that complement the existing sections and project structure.\n\n")
	}

	if len(previousOutputs) > 0 {
		prompt.WriteString("PROJECT ANALYSIS:\n")
		prompt.WriteString(truncateForContext(previousOutputs[0], 1000))
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("DETECTED CONFIGURATION:\n")
	if context.CodebaseInfo != nil {
		info := context.CodebaseInfo
		prompt.WriteString(fmt.Sprintf("- Language: %s\n", info.MainLanguage))
		prompt.WriteString(fmt.Sprintf("- Tech Stack: %s\n", strings.Join(info.TechStack, ", ")))
		prompt.WriteString(fmt.Sprintf("- Build: %s\n", info.BuildCommand))
		prompt.WriteString(fmt.Sprintf("- Test: %s\n", info.TestCommand))
		prompt.WriteString(fmt.Sprintf("- Lint: %s\n", info.LintCommand))
	}

	prompt.WriteString("\nTASK: Create project-specific coding rules based on the actual codebase.\n\n")

	prompt.WriteString("REQUIREMENTS:\n")
	prompt.WriteString("1. Create a 'Development Workflow' rule if complex commands exist (priority: high)\n")
	prompt.WriteString("2. Add language-specific best practices observed in the code\n")
	prompt.WriteString("3. Include testing requirements if test setup exists\n")
	prompt.WriteString("4. Add linting/formatting rules if tools are configured\n")
	prompt.WriteString("5. Include any security or performance guidelines\n\n")

	prompt.WriteString("FORMAT as:\n")
	prompt.WriteString("```yaml\n")
	prompt.WriteString("rules:\n")
	prompt.WriteString("  - name: \"Rule Name\"\n")
	prompt.WriteString("    priority: critical|high|medium|low|minimal\n")
	prompt.WriteString("    content: |\n")
	prompt.WriteString("      Rule content here\n")
	prompt.WriteString("```\n\n")

	prompt.WriteString("Create 3-8 rules that are SPECIFIC to this project.\n")
	prompt.WriteString("Include actual commands, file paths, and patterns from the codebase.\n")
	prompt.WriteString("Avoid generic advice - be precise and actionable.\n")

	return prompt.String()
}

func buildPhase4Prompt(context *ProjectContext, previousOutputs []string) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Configure specialized AI agents for the '%s' project.\n\n", context.ProjectName))

	// Read existing config from file
	existingConfig := readExistingConfig()
	if existingConfig != "" {
		prompt.WriteString("CURRENT ai_rulez.yaml CONFIGURATION:\n")
		prompt.WriteString("```yaml\n")
		prompt.WriteString(existingConfig)
		prompt.WriteString("```\n\n")
		prompt.WriteString("BUILD ON: Create agents that work with the existing sections and rules.\n\n")
	}

	if len(previousOutputs) > 0 {
		prompt.WriteString("PROJECT CONTEXT:\n")
		prompt.WriteString(truncateForContext(previousOutputs[0], 800))
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("PROJECT TYPE: " + context.RepoType + "\n")
	if context.CodebaseInfo != nil {
		prompt.WriteString("MAIN TECH: " + strings.Join(context.CodebaseInfo.TechStack, ", ") + "\n")
	}

	prompt.WriteString("\nTASK: Create specialized AI agents for complex domain-specific tasks.\n\n")

	prompt.WriteString("AGENT GUIDELINES:\n")
	prompt.WriteString("1. Only create agents for complex, specialized tasks\n")
	prompt.WriteString("2. Each agent should have deep domain knowledge\n")
	prompt.WriteString("3. Agents should reference actual tools, commands, and patterns\n")
	prompt.WriteString("4. Include specific file paths and configurations\n")
	prompt.WriteString("5. Limit to 2-5 most valuable agents\n\n")

	prompt.WriteString("SUGGESTED AGENT TYPES based on project:\n")

	if context.RepoType == repoTypeMonorepo {
		prompt.WriteString("- monorepo-architect: Cross-package dependencies and builds\n")
	}

	if context.CodebaseInfo != nil {
		tech := context.CodebaseInfo.TechStack
		for _, t := range tech {
			switch {
			case strings.Contains(t, "React") || strings.Contains(t, "Next"):
				prompt.WriteString("- frontend-engineer: React/Next.js specialist\n")
			case strings.Contains(t, "Node") || strings.Contains(t, "Express") || strings.Contains(t, "Fastify"):
				prompt.WriteString("- backend-engineer: Node.js API specialist\n")
			case t == "Go":
				prompt.WriteString("- go-specialist: Go patterns and performance\n")
			case t == "Python":
				prompt.WriteString("- python-specialist: Python best practices\n")
			case t == "TypeScript":
				prompt.WriteString("- typescript-expert: Type system and patterns\n")
			}
		}

		if context.CodebaseInfo.HasDatabase {
			prompt.WriteString("- database-specialist: Schema and query optimization\n")
		}

		if context.CodebaseInfo.HasDocker {
			prompt.WriteString("- devops-engineer: Docker and deployment\n")
		}
	}

	prompt.WriteString("\nFORMAT as:\n")
	prompt.WriteString("```yaml\n")
	prompt.WriteString("agents:\n")
	prompt.WriteString("  - name: \"agent-name\"\n")
	prompt.WriteString("    description: \"What this agent specializes in\"\n")
	prompt.WriteString("    priority: high|medium|low\n")
	prompt.WriteString("    system_prompt: |\n")
	prompt.WriteString("      You are a [role] for [project].\n")
	prompt.WriteString("      Tech stack: [specific versions and tools]\n")
	prompt.WriteString("      Focus on: [key responsibilities]\n")
	prompt.WriteString("      Commands: [actual commands from project]\n")
	prompt.WriteString("      Always: [key principles]\n")
	prompt.WriteString("```\n\n")

	prompt.WriteString("Create only the most valuable agents for this specific project.\n")
	prompt.WriteString("Each agent must reference actual project details, not generic advice.\n")

	return prompt.String()
}

func buildPhase5Prompt(context *ProjectContext, previousOutputs []string) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Finalize configuration for the '%s' project with commands and integrations.\n\n", context.ProjectName))

	// Read existing config from file
	existingConfig := readExistingConfig()
	if existingConfig != "" {
		prompt.WriteString("CURRENT ai_rulez.yaml CONFIGURATION:\n")
		prompt.WriteString("```yaml\n")
		prompt.WriteString(existingConfig)
		prompt.WriteString("```\n\n")
		prompt.WriteString("BUILD ON: Add commands and MCP servers that complement the existing configuration.\n\n")
	}

	prompt.WriteString("DETECTED COMMANDS:\n")
	if context.CodebaseInfo != nil {
		info := context.CodebaseInfo
		if info.BuildCommand != "" {
			prompt.WriteString(fmt.Sprintf("- Build: %s\n", info.BuildCommand))
		}
		if info.TestCommand != "" {
			prompt.WriteString(fmt.Sprintf("- Test: %s\n", info.TestCommand))
		}
		if info.LintCommand != "" {
			prompt.WriteString(fmt.Sprintf("- Lint: %s\n", info.LintCommand))
		}
	}

	if context.CodebaseInfo != nil && context.CodebaseInfo.HasMCP {
		prompt.WriteString(fmt.Sprintf("\nMCP CAPABILITY: %s\n", context.CodebaseInfo.MCPCommand))
	}

	prompt.WriteString("\nTASK: Add useful commands and MCP server configuration if applicable.\n\n")

	prompt.WriteString("REQUIREMENTS:\n")
	prompt.WriteString("1. Add frequently used development commands\n")
	prompt.WriteString("2. Include build, test, and lint commands if they exist\n")
	prompt.WriteString("3. Add any deployment or release commands\n")
	prompt.WriteString("4. Configure MCP server for ai-rulez if MCP is available\n")
	prompt.WriteString("5. Keep descriptions concise and clear\n\n")

	prompt.WriteString("FORMAT commands as:\n")
	prompt.WriteString("```yaml\n")
	prompt.WriteString("commands:\n")
	prompt.WriteString("  - name: \"build\"\n")
	prompt.WriteString("    command: \"actual build command\"\n")
	prompt.WriteString("    description: \"Build the project\"\n")
	prompt.WriteString("```\n\n")

	if context.CodebaseInfo != nil && context.CodebaseInfo.HasMCP {
		prompt.WriteString("FORMAT MCP server as:\n")
		prompt.WriteString("```yaml\n")
		prompt.WriteString("mcp_servers:\n")
		prompt.WriteString("  - name: \"ai-rulez\"\n")
		prompt.WriteString("    command: \"" + context.CodebaseInfo.MCPCommand + " ai-rulez\"\n")
		prompt.WriteString("    description: \"AI-Rulez MCP server for configuration management\"\n")
		prompt.WriteString("```\n\n")
	}

	prompt.WriteString("Only include commands that actually exist in the project.\n")
	prompt.WriteString("Use the exact command syntax from the project's setup.\n")

	return prompt.String()
}

func truncateForContext(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}
	return text[:maxLength] + "..."
}

func readExistingConfig() string {
	if data, err := os.ReadFile("ai_rulez.yaml"); err == nil {
		return string(data)
	}
	return ""
}
