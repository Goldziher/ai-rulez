package agents

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Task-specific prompt functions for direct YAML editing

func buildProjectAnalysisPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Update the project description in ai-rulez.yaml for '%s'\n\n", context.ProjectName))

	prompt.WriteString("SIMPLE INSTRUCTIONS:\n")
	prompt.WriteString("1. Use Read tool to read ai-rulez.yaml\n")
	prompt.WriteString("2. Find the line with: description: \"\" # Will be updated by project-agent\n")
	prompt.WriteString("3. Use Edit tool to replace the empty string with a real description\n")
	prompt.WriteString("4. Keep the format as: description: \"Your 2-3 sentence project description\"\n\n")

	prompt.WriteString("IMPORTANT: Only update the description value. Do not change anything else.\n\n")

	// Add existing AI configs as context
	addExistingConfigContext(&prompt, context)

	// Add rich project context
	addProjectContext(&prompt, context)

	prompt.WriteString("\nFocus on:\n")
	prompt.WriteString("- Project organization and file structure patterns\n")
	prompt.WriteString("- Code style and naming conventions\n")
	prompt.WriteString("- Module/package boundaries and dependencies\n")
	prompt.WriteString("- Development workflow and contribution guidelines\n")

	return prompt.String()
}

func buildInfrastructurePrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add a deployment/infrastructure section to ai-rulez.yaml for '%s'\n\n", context.ProjectName))

	prompt.WriteString("SIMPLE INSTRUCTIONS:\n")
	prompt.WriteString("1. Use Read tool to read ai-rulez.yaml\n")
	prompt.WriteString("2. Find the line: sections: []\n")
	prompt.WriteString("3. Use Edit tool to replace it with a sections array containing ONE infrastructure section\n")
	prompt.WriteString("4. Example format:\n")
	prompt.WriteString("sections:\n")
	prompt.WriteString("  - name: \"Deployment & Infrastructure\"\n")
	prompt.WriteString("    priority: high\n")
	prompt.WriteString("    content: |\n")
	prompt.WriteString("      Brief description of deployment process,\n")
	prompt.WriteString("      CI/CD pipeline, and infrastructure setup.\n\n")

	// Add existing AI configs as context
	addExistingConfigContext(&prompt, context)

	// Add rich project context
	addProjectContext(&prompt, context)

	prompt.WriteString("\nFocus on:\n")
	prompt.WriteString("- CI/CD pipelines and automation\n")
	prompt.WriteString("- Deployment strategies and environments\n")
	prompt.WriteString("- Infrastructure as code (Docker, Kubernetes, Terraform)\n")
	prompt.WriteString("- Monitoring, logging, and observability\n")
	prompt.WriteString("- Security and compliance requirements\n")
	prompt.WriteString("- Performance and scalability considerations\n")

	return prompt.String()
}

func buildDocumentationPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add documentation sections to ai-rulez.yaml for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the current ai-rulez.yaml file\n")
	prompt.WriteString("2. Replace the empty `sections: []` array with 4-6 key documentation sections\n")
	prompt.WriteString("3. Use the Edit tool to modify ai-rulez.yaml directly\n")
	prompt.WriteString("4. Replace the entire line `sections: []` with a proper YAML array\n\n")

	prompt.WriteString("Each section should have:\n")
	prompt.WriteString("- name: \"Section Title\"\n")
	prompt.WriteString("- priority: critical|high|medium|low\n")
	prompt.WriteString("- content: |\n")
	prompt.WriteString("    Multi-line description of what developers need to know\n\n")

	// Add existing AI configs as context
	addExistingConfigContext(&prompt, context)

	// Add rich project context
	addProjectContext(&prompt, context)

	// Add existing documentation context
	if len(context.MarkdownFiles) > 0 {
		prompt.WriteString("\nEXISTING DOCUMENTATION:\n")
		for _, doc := range context.MarkdownFiles {
			if doc.Category == "root" || doc.Category == "docs" {
				fmt.Fprintf(&prompt, "- @%s - %s\n", doc.RelativePath, doc.Title)
			}
		}
	}

	prompt.WriteString("\nFocus on essential areas like architecture, development workflow, testing, deployment, etc.\n")
	prompt.WriteString("Reference existing documentation with @ notation when relevant.\n")

	return prompt.String()
}

func buildStandardsPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add coding standards to ai-rulez.yaml for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the current ai-rulez.yaml file\n")
	prompt.WriteString("2. Replace the empty `rules: []` array with 5-7 important coding standards\n")
	prompt.WriteString("3. Use the Edit tool to modify ai-rulez.yaml directly\n")
	prompt.WriteString("4. Replace the entire line `rules: []` with a proper YAML array\n\n")

	prompt.WriteString("Each rule should be a separate array entry:\n")
	prompt.WriteString("- name: \"Specific Standard Name\"\n")
	prompt.WriteString("- priority: critical|high|medium|low\n")
	prompt.WriteString("- content: \"Clear, actionable description with examples if helpful\"\n\n")

	// Add existing AI configs as context
	addExistingConfigContext(&prompt, context)

	// Add rich project context
	addProjectContext(&prompt, context)

	prompt.WriteString("\nFocus on critical practices that prevent bugs and maintain code quality.\n")
	prompt.WriteString("Base standards on the detected technologies, commands, and project structure.\n")

	return prompt.String()
}

func buildSpecialistPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add specialized AI agents to ai-rulez.yaml for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the current ai-rulez.yaml file\n")
	prompt.WriteString("2. Replace the empty `agents: []` array with 3-4 specialized AI agents\n")
	prompt.WriteString("3. Use the Edit tool to modify ai-rulez.yaml directly\n")
	prompt.WriteString("4. Replace the entire line `agents: []` with a proper YAML array\n\n")

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

func buildToolingPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add commands and MCP server configuration to ai-rulez.yaml for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the current ai-rulez.yaml file\n")
	prompt.WriteString("2. Replace `commands: []` with 3-5 common project commands\n")
	prompt.WriteString("3. Replace `mcp_servers: []` with the ai-rulez MCP server configuration\n")
	prompt.WriteString("4. Use the Edit tool to modify ai-rulez.yaml directly\n")
	prompt.WriteString("5. Replace the entire lines with proper YAML arrays\n\n")

	prompt.WriteString("Commands section format:\n")
	prompt.WriteString("commands:\n")
	prompt.WriteString("  - name: \"command-name\"\n")
	prompt.WriteString("    description: \"What this command does\"\n")
	prompt.WriteString("    command: \"actual command to run\"\n\n")

	prompt.WriteString("MCP servers section format:\n")
	prompt.WriteString("mcp_servers:\n")
	prompt.WriteString("  - name: \"ai-rulez\"\n")
	prompt.WriteString("    command: \"ai-rulez\"\n")
	prompt.WriteString("    args: [\"mcp\"]\n")
	prompt.WriteString("    description: \"AI-Rulez MCP server for configuration management\"\n\n")

	// Add rich project context
	addProjectContext(&prompt, context)

	prompt.WriteString("\nSuggested commands based on detected project tools:\n")
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
		if info.HasDocker {
			prompt.WriteString("- Docker commands (docker-compose up, etc.)\n")
		}
		if context.RepoType == "monorepo" {
			prompt.WriteString("- Monorepo commands (workspace management, etc.)\n")
		}
	}

	prompt.WriteString("\nFocus on commands that developers use daily for this project.\n")
	prompt.WriteString("Always include the ai-rulez MCP server for Claude integration.\n")

	return prompt.String()
}

// Specialized documentation prompts for large projects

func buildCoreDocumentationPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add CORE documentation sections to ai-rulez.yaml for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the current ai-rulez.yaml file\n")
	prompt.WriteString("2. Focus on README, CONTRIBUTING, and main documentation\n")
	prompt.WriteString("3. Add 2-3 essential sections about project overview and setup\n")
	prompt.WriteString("4. Use the Edit tool to modify ai-rulez.yaml directly\n\n")

	// Add targeted context
	addProjectContext(&prompt, context)

	// List relevant core docs
	prompt.WriteString("\nFOCUS ON THESE CORE DOCUMENTS:\n")
	for _, doc := range context.MarkdownFiles {
		name := strings.ToLower(filepath.Base(doc.Path))
		if name == "readme.md" || name == "contributing.md" || name == "license.md" || name == "code_of_conduct.md" {
			fmt.Fprintf(&prompt, "- @%s - %s\n", doc.RelativePath, doc.Title)
		}
	}

	prompt.WriteString("\nCreate sections for: Project Overview, Setup & Installation, Development Workflow\n")

	return prompt.String()
}

func buildAPIDocumentationPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add API documentation sections to ai-rulez.yaml for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the current ai-rulez.yaml file\n")
	prompt.WriteString("2. Focus on API documentation, endpoints, and schemas\n")
	prompt.WriteString("3. Add 2-3 sections about API usage and patterns\n")
	prompt.WriteString("4. Use the Edit tool to modify ai-rulez.yaml directly\n\n")

	// Add targeted context
	addProjectContext(&prompt, context)

	// List relevant API docs
	prompt.WriteString("\nFOCUS ON THESE API DOCUMENTS:\n")
	for _, doc := range context.MarkdownFiles {
		name := strings.ToLower(filepath.Base(doc.Path))
		if strings.Contains(name, "api") || strings.Contains(name, "reference") || strings.Contains(name, "endpoint") || strings.Contains(name, "schema") {
			fmt.Fprintf(&prompt, "- @%s - %s\n", doc.RelativePath, doc.Title)
		}
	}

	prompt.WriteString("\nCreate sections for: API Patterns, Authentication, Error Handling\n")

	return prompt.String()
}

func buildArchDocumentationPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add architecture documentation sections to ai-rulez.yaml for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Read the current ai-rulez.yaml file\n")
	prompt.WriteString("2. Focus on architecture, design patterns, and system design\n")
	prompt.WriteString("3. Add 2-3 sections about technical architecture\n")
	prompt.WriteString("4. Use the Edit tool to modify ai-rulez.yaml directly\n\n")

	// Add targeted context
	addProjectContext(&prompt, context)

	// List relevant architecture docs
	prompt.WriteString("\nFOCUS ON THESE ARCHITECTURE DOCUMENTS:\n")
	for _, doc := range context.MarkdownFiles {
		name := strings.ToLower(filepath.Base(doc.Path))
		if strings.Contains(name, "architect") || strings.Contains(name, "design") || strings.Contains(name, "structure") || strings.Contains(name, "technical") {
			fmt.Fprintf(&prompt, "- @%s - %s\n", doc.RelativePath, doc.Title)
		}
	}

	prompt.WriteString("\nCreate sections for: System Architecture, Data Flow, Key Components\n")

	return prompt.String()
}

// addProjectContext adds comprehensive project context information to prompts
func addProjectContext(prompt *strings.Builder, context *ProjectContext) {
	prompt.WriteString("✓ VERIFIED PROJECT CONTEXT (from actual code analysis):\n")
	fmt.Fprintf(prompt, "- Repository Type: %s\n", context.RepoType)
	fmt.Fprintf(prompt, "- Root Path: %s\n", context.RootPath)

	if context.CodebaseInfo != nil {
		info := context.CodebaseInfo
		prompt.WriteString("\n📊 DETECTED FROM CODE (Highly Reliable):\n")
		if info.MainLanguage != "" {
			fmt.Fprintf(prompt, "- Primary Language: %s (detected from files)\n", info.MainLanguage)
		}
		if len(info.TechStack) > 0 {
			fmt.Fprintf(prompt, "- Technologies: %s (found in dependencies)\n", strings.Join(info.TechStack, ", "))
		}
		fmt.Fprintf(prompt, "- Project Type: %s\n", info.ProjectType)

		// Development commands
		if info.BuildCommand != "" {
			fmt.Fprintf(prompt, "- Build Command: %s (from package files)\n", info.BuildCommand)
		}
		if info.TestCommand != "" {
			fmt.Fprintf(prompt, "- Test Command: %s (from package files)\n", info.TestCommand)
		}
		if info.LintCommand != "" {
			fmt.Fprintf(prompt, "- Lint Command: %s (from package files)\n", info.LintCommand)
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

// addExistingConfigContext adds existing AI configuration as context
func addExistingConfigContext(prompt *strings.Builder, context *ProjectContext) {
	if len(context.ExistingConfigs) == 0 {
		return
	}

	prompt.WriteString("⚠️ EXISTING DOCUMENTATION (POTENTIALLY OUTDATED):\n")
	prompt.WriteString("The following files exist but MAY BE OUTDATED OR INCORRECT:\n")
	prompt.WriteString("- README files often contain outdated setup instructions\n")
	prompt.WriteString("- AI config files might be copied from other projects\n")
	prompt.WriteString("- ALWAYS verify claims against actual code structure\n\n")

	prompt.WriteString("VERIFICATION REQUIRED:\n")
	prompt.WriteString("✓ Cross-check any claims with actual project files\n")
	prompt.WriteString("✓ Trust package.json/go.mod/requirements.txt over README claims\n")
	prompt.WriteString("✓ Verify that mentioned features actually exist in code\n")
	prompt.WriteString("✓ Check if build/test commands actually work\n\n")

	// Prioritize certain configs
	configOrder := []string{"README", "CLAUDE", "GEMINI", "AMP", "CONTINUE", "CURSOR", "AI_RULEZ"}

	for _, key := range configOrder {
		if content, exists := context.ExistingConfigs[key]; exists {
			fmt.Fprintf(prompt, "=== %s (UNVERIFIED) ===\n", key)
			// Only include first 2000 chars to avoid overwhelming context
			if len(content) > 2000 {
				content = content[:2000] + "\n... (truncated)"
			}
			prompt.WriteString(content)
			prompt.WriteString("\n\n")
		}
	}

	prompt.WriteString("IMPORTANT: Use these as hints only. Actual code structure takes precedence over documentation.\n\n")
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

// Fallback prompt functions - simplified versions for retries

func buildProjectAnalysisFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Update project description in ai-rulez.yaml for '%s'\n\n", context.ProjectName))
	prompt.WriteString("SIMPLE STEPS:\n")
	prompt.WriteString("1. Read ai-rulez.yaml file\n")
	prompt.WriteString("2. Replace 'description: \"\"' with a 1-2 sentence description\n")
	prompt.WriteString("3. Keep it brief and factual\n\n")
	return prompt.String()
}

func buildInfrastructureFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Add one infrastructure section to ai-rulez.yaml for '%s'\n\n", context.ProjectName))
	prompt.WriteString("SIMPLE STEPS:\n")
	prompt.WriteString("1. Read ai-rulez.yaml\n")
	prompt.WriteString("2. Replace 'sections: []' with:\n")
	prompt.WriteString("sections:\n")
	prompt.WriteString("  - name: \"Infrastructure\"\n")
	prompt.WriteString("    priority: medium\n")
	prompt.WriteString("    content: |\n")
	prompt.WriteString("      Basic deployment and infrastructure setup.\n\n")
	return prompt.String()
}

func buildStandardsFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Add 3 basic coding rules to ai-rulez.yaml for '%s'\n\n", context.ProjectName))
	prompt.WriteString("SIMPLE STEPS:\n")
	prompt.WriteString("1. Read ai-rulez.yaml\n")
	prompt.WriteString("2. Replace 'rules: []' with 3 simple rules\n")
	prompt.WriteString("3. Use: Error Handling, Code Style, Testing\n")
	prompt.WriteString("4. Keep each rule 1-2 sentences\n\n")
	return prompt.String()
}

func buildSpecialistFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Add 2 simple agents to ai-rulez.yaml for '%s'\n\n", context.ProjectName))
	prompt.WriteString("SIMPLE STEPS:\n")
	prompt.WriteString("1. Read ai-rulez.yaml\n")
	prompt.WriteString("2. Replace 'agents: []' with 2 basic agents\n")
	prompt.WriteString("3. Use: code-reviewer and test-writer\n")
	prompt.WriteString("4. Keep descriptions brief\n\n")
	return prompt.String()
}

func buildToolingFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Add basic commands to ai-rulez.yaml for '%s'\n\n", context.ProjectName))
	prompt.WriteString("SIMPLE STEPS:\n")
	prompt.WriteString("1. Read ai-rulez.yaml\n")
	prompt.WriteString("2. Replace 'commands: []' with 2-3 basic commands\n")
	prompt.WriteString("3. Add MCP server if 'mcp_servers: []' exists\n")
	prompt.WriteString("4. Keep it simple\n\n")
	return prompt.String()
}

func buildDocumentationFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Add 2 documentation sections to ai-rulez.yaml for '%s'\n\n", context.ProjectName))
	prompt.WriteString("SIMPLE STEPS:\n")
	prompt.WriteString("1. Read ai-rulez.yaml\n")
	prompt.WriteString("2. If 'sections: []' exists, add 2 basic sections\n")
	prompt.WriteString("3. Use: Setup Guide and API Documentation\n")
	prompt.WriteString("4. Keep content brief\n\n")
	return prompt.String()
}
