package agents

import (
	"fmt"
	"strings"
)

// Task-specific prompt functions for direct YAML editing

func buildProjectAnalysisPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Set the project description for '%s'\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Analyze the project context provided below\n")
	prompt.WriteString("2. Create a clear, concise description (1-2 sentences)\n")
	prompt.WriteString("3. Output ONLY the command to run. DO NOT give status messages or explanations.\n")
	prompt.WriteString("4. Your response must be exactly this format:\n")
	prompt.WriteString("   ai-rulez set metadata --description \"Your description here\"\n")
	prompt.WriteString("5. Replace 'Your description here' with the actual description\n")
	prompt.WriteString("6. Do not add any other text - only the command line\n\n")

	// Add existing AI configs as context
	addExistingConfigContext(&prompt, context)

	// Add rich project context
	addProjectContext(&prompt, context)

	prompt.WriteString("\nAnalyze the project and write a clear description focusing on:\n")
	prompt.WriteString("- Primary purpose and functionality\n")
	prompt.WriteString("- Key technologies used\n")
	prompt.WriteString("- Project type (library, application, tool, etc.)\n")

	return prompt.String()
}

func buildDocumentationPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add documentation sections to ai-rulez.yaml for '%s'\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Create 2-3 essential documentation sections based on the project\n")
	prompt.WriteString("2. Output ONLY the commands to run, one per line. DO NOT give explanations.\n")
	prompt.WriteString("3. Each command must be exactly this format:\n")
	prompt.WriteString("   ai-rulez add section --name \"Section Name\" --priority PRIORITY --content \"Content here\"\n")
	prompt.WriteString("4. Priority can be: high, medium, or low\n")
	prompt.WriteString("5. Content should be markdown format\n")
	prompt.WriteString("6. Only output the commands, no other text\n\n")

	prompt.WriteString("Example output:\n")
	prompt.WriteString("ai-rulez add section --name \"Setup Guide\" --priority high --content \"## Prerequisites\\n- Node.js 18+\\n- Docker\"\n\n")

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

	prompt.WriteString("\nCreate 2-3 sections covering essential topics like:\n")
	prompt.WriteString("- Setup/Installation, Development Workflow, Architecture, Testing, etc.\n")
	prompt.WriteString("Keep content concise but helpful. Reference existing docs when relevant.\n")

	return prompt.String()
}

func buildStandardsPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add coding standards for the '%s' project\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Create 5-7 important coding standards based on the project\n")
	prompt.WriteString("2. Output ONLY the commands to run, one per line. DO NOT give explanations.\n")
	prompt.WriteString("3. Each command must be exactly this format:\n")
	prompt.WriteString("   ai-rulez add rule --name \"Rule Name\" --priority PRIORITY --content \"Rule description\"\n")
	prompt.WriteString("4. Priority can be: critical, high, medium, low, or minimal\n")
	prompt.WriteString("5. Content should be clear and actionable\n")
	prompt.WriteString("6. Only output the commands, no other text\n\n")

	prompt.WriteString("Example output:\n")
	prompt.WriteString("ai-rulez add rule --name \"Error Handling\" --priority high --content \"Always use proper exception handling with specific error types\"\n\n")

	// Add existing AI configs as context
	addExistingConfigContext(&prompt, context)

	// Add rich project context
	addProjectContext(&prompt, context)

	prompt.WriteString("\nFocus on critical practices that prevent bugs and maintain code quality.\n")
	prompt.WriteString("Base standards on the detected technologies, commands, and project structure.\n")

	return prompt.String()
}

// addProjectContext adds comprehensive project context information to prompts
func addProjectContext(prompt *strings.Builder, context *ProjectContext) {
	addBasicProjectInfo(prompt, context)
	addGitHistoryInfo(prompt, context)
	addCodebaseInfo(prompt, context)
	addMonorepoInfo(prompt, context)
	addProjectStructure(prompt, context)
}

// addBasicProjectInfo adds basic project information to the prompt
func addBasicProjectInfo(prompt *strings.Builder, context *ProjectContext) {
	prompt.WriteString("✓ VERIFIED PROJECT CONTEXT (from actual code analysis):\n")
	fmt.Fprintf(prompt, "- Repository Type: %s\n", context.RepoType)
	fmt.Fprintf(prompt, "- Root Path: %s\n", context.RootPath)
}

// addGitHistoryInfo adds git history information if available
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

// addCodebaseInfo adds codebase analysis information
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

// addLanguageInfo adds programming language and tech stack information
func addLanguageInfo(prompt *strings.Builder, info *CodebaseInfo) {
	if info.MainLanguage != "" {
		fmt.Fprintf(prompt, "- Primary Language: %s (detected from files)\n", info.MainLanguage)
	}
	if len(info.TechStack) > 0 {
		fmt.Fprintf(prompt, "- Technologies: %s (found in dependencies)\n", strings.Join(info.TechStack, ", "))
	}
	fmt.Fprintf(prompt, "- Project Type: %s\n", info.ProjectType)
}

// addDevelopmentCommands adds development command information
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

// addCapabilities adds capability information
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

// addMonorepoInfo adds monorepo structure information
func addMonorepoInfo(prompt *strings.Builder, context *ProjectContext) {
	if context.RepoType != "monorepo" {
		return
	}

	if len(context.PackageLocations) > 0 {
		fmt.Fprintf(prompt, "- Packages (%d): %s\n", len(context.PackageLocations), strings.Join(context.PackageLocations, ", "))
	}
	if len(context.AppLocations) > 0 {
		fmt.Fprintf(prompt, "- Applications (%d): %s\n", len(context.AppLocations), strings.Join(context.AppLocations, ", "))
	}
}

// addProjectStructure adds project structure tree if available
func addProjectStructure(prompt *strings.Builder, context *ProjectContext) {
	structure := context.GenerateStructureTree()
	if structure == "" {
		return
	}

	prompt.WriteString("\nPROJECT STRUCTURE:\n```\n")
	prompt.WriteString(structure)
	prompt.WriteString("```\n")
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

// Fallback prompt functions - simplified versions for retries

func buildProjectAnalysisFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Set project description for '%s'\n\n", context.ProjectName))
	prompt.WriteString("SIMPLE STEPS:\n")
	prompt.WriteString("1. Create a brief 1-2 sentence description\n")
	prompt.WriteString("2. Output ONLY this command format:\n")
	prompt.WriteString("   ai-rulez set metadata --description \"Your description\"\n")
	prompt.WriteString("3. Replace with actual description, no other text\n\n")
	return prompt.String()
}

func buildStandardsFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Add 3 basic coding rules for '%s'\n\n", context.ProjectName))
	prompt.WriteString("SIMPLE STEPS:\n")
	prompt.WriteString("1. Add these 3 rules using ai-rulez add rule command:\n")
	prompt.WriteString("2. Error Handling rule (priority: high)\n")
	prompt.WriteString("3. Code Style rule (priority: medium)\n")
	prompt.WriteString("4. Testing rule (priority: high)\n\n")
	return prompt.String()
}

func buildDocumentationFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("TASK: Add 2 documentation sections for '%s'\n\n", context.ProjectName))
	prompt.WriteString("SIMPLE STEPS:\n")
	prompt.WriteString("1. Add these 2 sections using ai-rulez add section:\n")
	prompt.WriteString("2. Setup Guide (priority: high)\n")
	prompt.WriteString("3. API Documentation (priority: medium)\n")
	prompt.WriteString("4. Keep content brief\n\n")
	return prompt.String()
}

// buildAgentDefinitionsPrompt creates a prompt for generating AI agent definitions
func buildAgentDefinitionsPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add AI agent definitions for '%s'\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Create 2-4 useful AI agent definitions based on the project\n")
	prompt.WriteString("2. Output ONLY the commands to run, one per line. DO NOT give explanations.\n")
	prompt.WriteString("3. Each command must be exactly this format:\n")
	prompt.WriteString("   ai-rulez add agent --name \"name\" --role \"role\" --expertise \"expertise\"\n")
	prompt.WriteString("4. Name should be lowercase with hyphens (e.g., backend-dev)\n")
	prompt.WriteString("5. Role should be a brief description\n")
	prompt.WriteString("6. Expertise should list specific skills\n")
	prompt.WriteString("7. Only output the commands, no other text\n\n")

	prompt.WriteString("Example output:\n")
	prompt.WriteString("ai-rulez add agent --name \"backend-dev\" --role \"Python backend developer\" --expertise \"FastAPI, PostgreSQL, Docker\"\n\n")

	// Add existing AI configs as context
	addExistingConfigContext(&prompt, context)

	// Add rich project context
	addProjectContext(&prompt, context)

	prompt.WriteString("\nCreate agents relevant to this project type, such as:\n")
	prompt.WriteString("- Code reviewer, architect, tester, documentation specialist, etc.\n")
	prompt.WriteString("- Keep expertise specific to the project's technology stack\n")
	prompt.WriteString("- Focus on roles that would genuinely help this project\n")

	return prompt.String()
}

// buildAgentDefinitionsFallbackPrompt creates a simple fallback prompt for agent definitions
func buildAgentDefinitionsFallbackPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add 2 basic AI agents for '%s'\n\n", context.ProjectName))

	prompt.WriteString("SIMPLE STEPS:\n")
	prompt.WriteString("1. Output ONLY the commands, no explanations:\n")
	prompt.WriteString("2. ai-rulez add agent --name \"code-reviewer\" --role \"Code review specialist\" --expertise \"...\"\n")
	prompt.WriteString("3. ai-rulez add agent --name \"doc-writer\" --role \"Documentation specialist\" --expertise \"...\"\n")
	prompt.WriteString("4. Replace ... with relevant expertise, output only commands\n\n")

	return prompt.String()
}
