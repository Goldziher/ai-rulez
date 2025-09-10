package agents

import (
	"fmt"
	"strings"
)

func buildProjectAnalysisPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Generate a project description for '%s'\n\n", context.ProjectName))

	prompt.WriteString("CRITICAL INSTRUCTIONS - JSON OUTPUT ONLY:\n")
	prompt.WriteString("You must output a valid JSON object. Do NOT execute any commands.\n")
	prompt.WriteString("Your response must contain ONLY the JSON object, no explanations.\n\n")

	prompt.WriteString("EXAMPLE INPUT: React web app for managing tasks\n")
	prompt.WriteString("EXAMPLE OUTPUT:\n")
	prompt.WriteString(`{
  "description": "React-based task management web application with real-time updates and team collaboration features"
}`)
	prompt.WriteString("\n\n")

	prompt.WriteString("YOUR TASK:\n")
	prompt.WriteString(fmt.Sprintf("Generate a JSON response with a description for the '%s' project.\n", context.ProjectName))
	prompt.WriteString("The description should be 1-2 sentences, clear and concise.\n\n")

	addExistingConfigContext(&prompt, context)

	addProjectContext(&prompt, context)

	prompt.WriteString("\nAnalyze the project and create a description that:\n")
	prompt.WriteString("- Clearly states the primary purpose and main functionality\n")
	prompt.WriteString("- Lists the key technologies, frameworks, and libraries used\n")
	prompt.WriteString("- Identifies the project type (CLI tool, library, web app, etc.)\n")
	prompt.WriteString("- Is specific to THIS project, not generic\n")
	prompt.WriteString("- Is concise but informative (50-100 words)\n")
	prompt.WriteString("\nOutput ONLY the JSON object with the description field.\n")

	return prompt.String()
}

func buildDocumentationPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Generate documentation sections for '%s'\n\n", context.ProjectName))

	prompt.WriteString("CRITICAL INSTRUCTIONS - JSON OUTPUT ONLY:\n")
	prompt.WriteString("You must output a valid JSON object. Do NOT execute any commands.\n")
	prompt.WriteString("Your response must contain ONLY the JSON object, no explanations.\n\n")

	prompt.WriteString("EXAMPLE INPUT: Python web API project\n")
	prompt.WriteString("EXAMPLE OUTPUT:\n")
	prompt.WriteString(`{
  "sections": [
    {
      "name": "Setup Guide",
      "priority": "high",
      "content": "## Prerequisites\n- Python 3.8+\n- pip\n\n## Installation\n1. Clone repository\n2. Run: pip install -r requirements.txt"
    },
    {
      "name": "API Documentation",
      "priority": "medium",
      "content": "## Endpoints\n- GET /api/users - List all users\n- POST /api/users - Create new user"
    }
  ]
}`)
	prompt.WriteString("\n\n")

	prompt.WriteString("YOUR TASK:\n")
	prompt.WriteString(fmt.Sprintf("Generate 2-3 documentation sections for the '%s' project that:\n", context.ProjectName))
	prompt.WriteString("- Provide PRACTICAL guidance specific to this codebase\n")
	prompt.WriteString("- Include actual commands, file paths, and package names from the project\n")
	prompt.WriteString("- Cover setup, development workflow, architecture, or testing\n")
	prompt.WriteString("- Use clear markdown formatting with code examples where helpful\n")
	prompt.WriteString("- Priority must be: high (essential), medium (important), or low (nice-to-have)\n")
	prompt.WriteString("- Avoid generic content that could apply to any project\n")

	addExistingConfigContext(&prompt, context)

	addProjectContext(&prompt, context)

	if len(context.MarkdownFiles) > 0 {
		prompt.WriteString("\nEXISTING DOCUMENTATION:\n")
		for _, doc := range context.MarkdownFiles {
			if doc.Category == "root" || doc.Category == "docs" {
				fmt.Fprintf(&prompt, "- @%s - %s\n", doc.RelativePath, doc.Title)
			}
		}
	}

	prompt.WriteString("\nCreate 2-3 sections covering essential topics like:\n")
	prompt.WriteString("- Setup/Installation, Development Workflow, Architecture, Testing, etc.\n")
	prompt.WriteString("Keep content concise but helpful. Reference existing docs when relevant.\n")

	return prompt.String()
}

func buildStandardsPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Generate coding standards for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("CRITICAL INSTRUCTIONS - JSON OUTPUT ONLY:\n")
	prompt.WriteString("You must output a valid JSON object. Do NOT execute any commands.\n")
	prompt.WriteString("Your response must contain ONLY the JSON object, no explanations.\n\n")

	prompt.WriteString("EXAMPLE INPUT: Go CLI tool project\n")
	prompt.WriteString("EXAMPLE OUTPUT:\n")
	prompt.WriteString(`{
  "rules": [
    {
      "name": "Error Handling",
      "priority": "high",
      "content": "Always handle errors explicitly. Use the oops library for rich error context."
    },
    {
      "name": "Testing",
      "priority": "critical",
      "content": "Write unit tests for all new functions. Aim for 80% code coverage minimum."
    },
    {
      "name": "Comments",
      "priority": "medium",
      "content": "Document all exported functions with godoc comments."
    },
    {
      "name": "Naming",
      "priority": "medium",
      "content": "Use clear, descriptive names. Avoid abbreviations except well-known ones."
    },
    {
      "name": "Dependencies",
      "priority": "low",
      "content": "Minimize external dependencies. Justify each new dependency added."
    }
  ]
}`)
	prompt.WriteString("\n\n")

	prompt.WriteString("YOUR TASK:\n")
	prompt.WriteString(fmt.Sprintf("Generate 5-7 coding rules for the '%s' project that:\n", context.ProjectName))
	prompt.WriteString("- Are SPECIFIC to this codebase's actual patterns and technologies\n")
	prompt.WriteString("- Reference actual packages, libraries, and tools used in the project\n")
	prompt.WriteString("- Include concrete, actionable guidance (not generic advice)\n")
	prompt.WriteString("- Cover error handling, testing, security, performance, and code organization\n")
	prompt.WriteString("- Use priority levels: critical, high, medium, low, or minimal\n")
	prompt.WriteString("- Avoid duplicate rules with different names\n")

	addExistingConfigContext(&prompt, context)

	addProjectContext(&prompt, context)

	prompt.WriteString("\nFocus on critical practices that prevent bugs and maintain code quality.\n")
	prompt.WriteString("Base standards on the detected technologies, commands, and project structure.\n")

	return prompt.String()
}

func addProjectContext(prompt *strings.Builder, context *ProjectContext) {
	addBasicProjectInfo(prompt, context)
	addGitHistoryInfo(prompt, context)
	addCodebaseInfo(prompt, context)
	addMonorepoInfo(prompt, context)
	addProjectStructure(prompt, context)
}

func addBasicProjectInfo(prompt *strings.Builder, context *ProjectContext) {
	prompt.WriteString("✓ VERIFIED PROJECT CONTEXT (from actual code analysis):\n")
	fmt.Fprintf(prompt, "- Repository Type: %s\n", context.RepoType)
	fmt.Fprintf(prompt, "- Root Path: %s\n", context.RootPath)
}

func addGitHistoryInfo(prompt *strings.Builder, context *ProjectContext) {
	if context.GitHistory == nil || !context.GitHistory.HasGit || context.GitHistory.CommitCount == 0 {
		return
	}

	prompt.WriteString("\n📚 GIT HISTORY ANALYSIS:\n")
	fmt.Fprintf(prompt, "- Total Commits: %d\n", context.GitHistory.CommitCount)

	if len(context.GitHistory.CommonPatterns) > 0 {
		prompt.WriteString("- Commit Patterns:\n")
		for _, pattern := range context.GitHistory.CommonPatterns {
			fmt.Fprintf(prompt, "  • %s\n", pattern)
		}
	}

	if len(context.GitHistory.CodingConventions) > 0 {
		prompt.WriteString("- Detected Conventions:\n")
		for _, convention := range context.GitHistory.CodingConventions {
			fmt.Fprintf(prompt, "  • %s\n", convention)
		}
	}
}

func addCodebaseInfo(prompt *strings.Builder, context *ProjectContext) {
	if context.CodebaseInfo == nil {
		return
	}

	info := context.CodebaseInfo
	prompt.WriteString("\n📊 DETECTED FROM CODE (Highly Reliable):\n")

	addLanguageInfo(prompt, info)
	addDevelopmentCommands(prompt, info)
	addCapabilities(prompt, info)
}

func addLanguageInfo(prompt *strings.Builder, info *CodebaseInfo) {
	if info.MainLanguage != "" {
		fmt.Fprintf(prompt, "- Primary Language: %s (detected from files)\n", info.MainLanguage)
	}
	if len(info.TechStack) > 0 {
		fmt.Fprintf(prompt, "- Technologies: %s (found in dependencies)\n", strings.Join(info.TechStack, ", "))
	}
	fmt.Fprintf(prompt, "- Project Type: %s\n", info.ProjectType)
}

func addDevelopmentCommands(prompt *strings.Builder, info *CodebaseInfo) {
	if info.BuildCommand != "" {
		fmt.Fprintf(prompt, "- Build Command: %s (from package files)\n", info.BuildCommand)
	}
	if info.TestCommand != "" {
		fmt.Fprintf(prompt, "- Test Command: %s (from package files)\n", info.TestCommand)
	}
	if info.LintCommand != "" {
		fmt.Fprintf(prompt, "- Lint Command: %s (from package files)\n", info.LintCommand)
	}
}

func addCapabilities(prompt *strings.Builder, info *CodebaseInfo) {
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

func addMonorepoInfo(prompt *strings.Builder, context *ProjectContext) {
	if context.RepoType != repoTypeMonorepo {
		return
	}

	if len(context.PackageLocations) > 0 {
		fmt.Fprintf(prompt, "- Packages (%d): %s\n", len(context.PackageLocations), strings.Join(context.PackageLocations, ", "))
	}
	if len(context.AppLocations) > 0 {
		fmt.Fprintf(prompt, "- Applications (%d): %s\n", len(context.AppLocations), strings.Join(context.AppLocations, ", "))
	}
}

func addProjectStructure(prompt *strings.Builder, context *ProjectContext) {
	structure := context.GenerateStructureTree()
	if structure == "" {
		return
	}

	prompt.WriteString("\nPROJECT STRUCTURE:\n```\n")
	prompt.WriteString(structure)
	prompt.WriteString("```\n")
}

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

	configOrder := []string{"README", "CLAUDE", "GEMINI", "AMP", "CONTINUE", "CURSOR", "AI_RULEZ"}

	for _, key := range configOrder {
		if content, exists := context.ExistingConfigs[key]; exists {
			fmt.Fprintf(prompt, "=== %s (UNVERIFIED) ===\n", key)
			if len(content) > 2000 {
				content = content[:2000] + "\n... (truncated)"
			}
			prompt.WriteString(content)
			prompt.WriteString("\n\n")
		}
	}

	prompt.WriteString("IMPORTANT: Use these as hints only. Actual code structure takes precedence over documentation.\n\n")
}

func buildProjectAnalysisFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Set project description for '%s'\n\n", context.ProjectName))

	prompt.WriteString("Output ONLY this JSON format:\n")
	prompt.WriteString(`{"description": "A brief description of the project"}` + "\n\n")

	prompt.WriteString("Replace the description with one for this project.\n")
	prompt.WriteString("Output ONLY the JSON, nothing else.\n\n")
	return prompt.String()
}

func buildStandardsFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Add 3 basic coding rules for '%s'\n\n", context.ProjectName))

	prompt.WriteString("Output ONLY this JSON format:\n")
	prompt.WriteString(`{
  "rules": [
    {"name": "Error Handling", "priority": "high", "content": "Handle all errors explicitly"},
    {"name": "Testing", "priority": "high", "content": "Write tests for new features"},
    {"name": "Code Style", "priority": "medium", "content": "Follow project conventions"}
  ]
}` + "\n\n")

	prompt.WriteString("Update the rules for this specific project.\n")
	prompt.WriteString("Output ONLY the JSON, nothing else.\n\n")
	return prompt.String()
}

func buildDocumentationFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Add 2 documentation sections for '%s'\n\n", context.ProjectName))

	prompt.WriteString("Output ONLY this JSON format:\n")
	prompt.WriteString(`{
  "sections": [
    {"name": "Setup Guide", "priority": "high", "content": "## Installation\n1. Clone repo\n2. Install deps"},
    {"name": "API Docs", "priority": "medium", "content": "## Endpoints\n- GET /api/status"}
  ]
}` + "\n\n")

	prompt.WriteString("Update the sections for this specific project.\n")
	prompt.WriteString("Output ONLY the JSON, nothing else.\n\n")
	return prompt.String()
}

func buildAgentDefinitionsPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Generate AI agent definitions for '%s'\n\n", context.ProjectName))

	prompt.WriteString("CRITICAL INSTRUCTIONS - JSON OUTPUT ONLY:\n")
	prompt.WriteString("You must output a valid JSON object. Do NOT execute any commands.\n")
	prompt.WriteString("Your response must contain ONLY the JSON object, no explanations.\n\n")

	prompt.WriteString("EXAMPLE INPUT: Node.js REST API project\n")
	prompt.WriteString("EXAMPLE OUTPUT:\n")
	prompt.WriteString(`{
  "agents": [
    {
      "name": "backend-dev",
      "role": "Node.js backend developer",
      "expertise": "Express, MongoDB, JWT authentication, REST API design"
    },
    {
      "name": "test-engineer",
      "role": "Testing specialist",
      "expertise": "Jest, Supertest, integration testing, TDD practices"
    },
    {
      "name": "devops",
      "role": "DevOps engineer",
      "expertise": "Docker, Kubernetes, CI/CD pipelines, AWS deployment"
    }
  ]
}`)
	prompt.WriteString("\n\n")

	prompt.WriteString("YOUR TASK:\n")
	prompt.WriteString(fmt.Sprintf("Generate 2-4 agent definitions for the '%s' project that:\n", context.ProjectName))
	prompt.WriteString("- Are specialized for THIS project's specific technologies and patterns\n")
	prompt.WriteString("- Have expertise that references actual tools, frameworks, and libraries used\n")
	prompt.WriteString("- Cover different aspects: architecture, testing, documentation, operations\n")
	prompt.WriteString("- Have unique names (lowercase with hyphens, e.g., backend-dev)\n")
	prompt.WriteString("- Provide clear, distinct roles that don't overlap\n")

	addExistingConfigContext(&prompt, context)

	addProjectContext(&prompt, context)

	prompt.WriteString("\nCreate agents relevant to this project type, such as:\n")
	prompt.WriteString("- Code reviewer, architect, tester, documentation specialist, etc.\n")
	prompt.WriteString("- Keep expertise specific to the project's technology stack\n")
	prompt.WriteString("- Focus on roles that would genuinely help this project\n")

	return prompt.String()
}

func buildAgentDefinitionsFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add 2 basic AI agents for '%s'\n\n", context.ProjectName))

	prompt.WriteString("Output ONLY this JSON format:\n")
	prompt.WriteString(`{
  "agents": [
    {"name": "code-reviewer", "role": "Code review specialist", "expertise": "Code quality, best practices, refactoring"},
    {"name": "test-engineer", "role": "Testing specialist", "expertise": "Unit tests, integration tests, TDD"}
  ]
}` + "\n\n")

	prompt.WriteString("Update the agents for this specific project.\n")
	prompt.WriteString("Output ONLY the JSON, nothing else.\n\n")

	return prompt.String()
}
