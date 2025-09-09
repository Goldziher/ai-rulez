package agents

import (
	"fmt"
	"strings"
)

// Task-specific prompt functions for direct YAML editing

func buildProjectAnalysisPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Update the project description in ai-rulez.yaml for '%s'\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Use Read tool to read ai-rulez.yaml\n")
	prompt.WriteString("2. Find the description field and update it with a proper description\n")
	prompt.WriteString("3. Use Edit tool to replace ONLY the description value\n")
	prompt.WriteString("4. Keep it brief (1-2 sentences) and informative\n")
	prompt.WriteString("5. Do NOT change any other parts of the YAML\n\n")

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
	prompt.WriteString("1. Use Read tool to read ai-rulez.yaml\n")
	prompt.WriteString("2. Find 'sections: []' and replace with 2-3 essential documentation sections\n")
	prompt.WriteString("3. Use Edit tool to replace the entire line with proper YAML\n")
	prompt.WriteString("4. Use correct YAML formatting with proper indentation\n")
	prompt.WriteString("5. Each section needs: name, priority, and content fields\n\n")

	prompt.WriteString("Section format:\n")
	prompt.WriteString("sections:\n")
	prompt.WriteString("  - name: \"Section Name\"\n")
	prompt.WriteString("    priority: high  # or medium, low\n")
	prompt.WriteString("    content: |\n")
	prompt.WriteString("      Multi-line markdown content\n")
	prompt.WriteString("      explaining this aspect\n\n")

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

// buildRepairPrompt creates a prompt for fixing validation errors
func buildRepairPrompt(validationErr error, context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Fix validation errors in ai-rulez.yaml for '%s'\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Use Read tool to read the current ai-rulez.yaml file\n")
	prompt.WriteString("2. Analyze the specific validation errors provided below\n")
	prompt.WriteString("3. Use Edit tool to fix ONLY the problematic parts\n")
	prompt.WriteString("4. Keep all existing valid content unchanged\n")
	prompt.WriteString("5. Ensure proper YAML syntax and structure\n")
	prompt.WriteString("6. Do NOT add any comments to the YAML\n\n")

	prompt.WriteString("VALIDATION ERRORS TO FIX:\n")
	prompt.WriteString(fmt.Sprintf("%v\n\n", validationErr))

	prompt.WriteString("COMMON FIXES:\n")
	prompt.WriteString("- Fix YAML syntax errors (indentation, quotes, colons)\n")
	prompt.WriteString("- Remove invalid fields or values\n")
	prompt.WriteString("- Add missing required fields\n")
	prompt.WriteString("- Fix array/object structure mismatches\n")
	prompt.WriteString("- Ensure priority values are: critical, high, medium, low, minimal\n")
	prompt.WriteString("- Make sure all strings are properly quoted\n\n")

	prompt.WriteString("IMPORTANT CONSTRAINTS:\n")
	prompt.WriteString("- Do NOT change the overall structure if it's correct\n")
	prompt.WriteString("- Do NOT add new sections unless required by validation\n")
	prompt.WriteString("- Keep content concise and meaningful\n")
	prompt.WriteString("- Commands are AI slash commands (/task, /generate) - leave empty if unsure\n")

	return prompt.String()
}

// buildAgentDefinitionsPrompt creates a prompt for generating AI agent definitions
func buildAgentDefinitionsPrompt(context *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("TASK: Add AI agent definitions to ai-rulez.yaml for '%s'\n\n", context.ProjectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. Use Read tool to read the current ai-rulez.yaml file\n")
	prompt.WriteString("2. Replace 'agents: []' with 2-4 useful AI agent definitions\n")
	prompt.WriteString("3. Use Edit tool to modify ai-rulez.yaml directly\n")
	prompt.WriteString("4. Each agent should have name, role, and expertise fields\n")
	prompt.WriteString("5. Use proper YAML array format\n\n")

	prompt.WriteString("Agent format:\n")
	prompt.WriteString("agents:\n")
	prompt.WriteString("  - name: \"Agent Name\"\n")
	prompt.WriteString("    role: \"Brief role description\"\n")
	prompt.WriteString("    expertise: \"Specific expertise areas\"\n\n")

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

	prompt.WriteString(fmt.Sprintf("TASK: Add 2 basic AI agents to ai-rulez.yaml for '%s'\n\n", context.ProjectName))

	prompt.WriteString("SIMPLE STEPS:\n")
	prompt.WriteString("1. Read ai-rulez.yaml\n")
	prompt.WriteString("2. Replace 'agents: []' with 2 basic agents\n")
	prompt.WriteString("3. Use: Code Reviewer and Documentation Specialist\n")
	prompt.WriteString("4. Keep it simple with name, role, expertise fields\n\n")

	return prompt.String()
}
