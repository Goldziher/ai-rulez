package agents

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/progress"
	"github.com/Goldziher/ai-rulez/internal/templates"
)

type ChainPhase struct {
	Number      int
	Name        string
	Description string
	Prompt      func(context *ProjectContext, previousOutputs []string) string
}

var initChainPhases = []ChainPhase{
	{
		Number:      1,
		Name:        "Analyzing project structure",
		Description: "Understanding repository organization and architecture",
		Prompt:      buildPhase1Prompt,
	},
	{
		Number:      2,
		Name:        "Generating documentation sections",
		Description: "Creating comprehensive documentation about the codebase",
		Prompt:      buildPhase2Prompt,
	},
	{
		Number:      3,
		Name:        "Creating coding rules",
		Description: "Defining project-specific coding standards and patterns",
		Prompt:      buildPhase3Prompt,
	},
	{
		Number:      4,
		Name:        "Configuring specialized agents",
		Description: "Setting up AI agents for domain-specific tasks",
		Prompt:      buildPhase4Prompt,
	},
	{
		Number:      5,
		Name:        "Finalizing configuration",
		Description: "Adding commands and final adjustments",
		Prompt:      buildPhase5Prompt,
	},
}

func ExecuteInitChain(agent AgentInfo, context *ProjectContext, providerConfig templates.ProviderConfig) (string, error) {
	logger.Info("🔗 Starting multi-phase configuration generation...")
	logger.Info("")

	var outputs []string
	totalPhases := len(initChainPhases)

	// Create overall progress bar
	progressBar := progress.New(totalPhases, "Generating configuration")
	defer func() {
		if err := progressBar.Finish(); err != nil {
			logger.Warn("Failed to finish progress bar", "error", err)
		}
	}()

	for _, phase := range initChainPhases {
		result, err := executePhase(phase, agent, context, outputs, providerConfig, totalPhases)
		if err != nil {
			// If we have at least 2 successful phases, try to salvage
			if len(outputs) >= 2 {
				logger.Info("  Attempting to generate configuration from completed phases...")
				return mergePhaseOutputs(outputs, context, providerConfig), nil
			}
			return "", fmt.Errorf("chain failed at phase %d: %w", phase.Number, err)
		}

		// Clean and store output
		cleanedOutput := cleanAgentOutput(result)
		outputs = append(outputs, cleanedOutput)

		// Write incremental update to ai_rulez.yaml file
		if err := writeIncrementalUpdate(phase.Number, cleanedOutput, context, providerConfig); err != nil {
			logger.Warn(fmt.Sprintf("Failed to write incremental update for phase %d: %v", phase.Number, err))
		}

		// Clean completion message
		fmt.Printf("✓ Phase %d completed - updated ai_rulez.yaml\n", phase.Number)

		// Update main progress bar
		if err := progressBar.Add(1); err != nil {
			logger.Warn("Failed to update progress bar", "error", err)
		}

		// Small delay between phases to avoid rate limiting
		if phase.Number < totalPhases {
			time.Sleep(2 * time.Second)
		}
	}

	logger.Info("")
	logger.Success("✅ All phases completed successfully")

	// Merge all outputs into final configuration
	return mergePhaseOutputs(outputs, context, providerConfig), nil
}

func executePhase(phase ChainPhase, agent AgentInfo, context *ProjectContext, outputs []string, providerConfig templates.ProviderConfig, totalPhases int) (string, error) {
	// Clean phase display - just show what we're doing
	fmt.Printf("\n[%d/%d] %s\n", phase.Number, totalPhases, phase.Name)
	logger.Info(fmt.Sprintf("Phase %d: %s", phase.Number, phase.Description))

	// Create simple spinner
	spinner := progress.NewSpinner("  Working...")

	// Build prompt for this phase
	prompt := phase.Prompt(context, outputs)

	// Add provider context to first phase
	if phase.Number == 1 {
		prompt = addProviderContext(prompt, providerConfig)
	}

	// Execute agent call with retries and longer timeout
	result, err := executeAgentWithRetries(agent, prompt, 180*time.Second, spinner, phase.Number, totalPhases)
	if err != nil {
		if finishErr := spinner.Finish(); finishErr != nil {
			logger.Warn("Failed to finish spinner", "error", finishErr)
		}

		result, err = tryFallbackAgent(agent, prompt, spinner, phase, totalPhases, err)
		if err != nil {
			logger.Warn(fmt.Sprintf("  ⚠️  Phase %d failed: %v", phase.Number, err))
			return "", err
		}
	}

	if err := spinner.Finish(); err != nil {
		logger.Warn("Failed to finish spinner", "error", err)
	}

	return result, nil
}

func tryFallbackAgent(agent AgentInfo, prompt string, spinner *progress.Bar, phase ChainPhase, totalPhases int, originalErr error) (string, error) {
	// Try fallback agent if primary failed with policy violation
	if !strings.Contains(originalErr.Error(), "Usage Policy") {
		return "", originalErr
	}

	logger.Info(fmt.Sprintf("  🔄 Trying fallback agent for Phase %d due to policy violation...", phase.Number))
	fallbackAgent := getFallbackAgent(agent)
	if fallbackAgent == nil {
		return "", originalErr
	}

	fallbackSpinner := progress.NewSpinner(fmt.Sprintf("  [%d/%d] %s (fallback)...", phase.Number, totalPhases, phase.Name))
	result, err := executeAgentWithRetries(*fallbackAgent, prompt, 180*time.Second, fallbackSpinner, phase.Number, totalPhases)

	if finishErr := fallbackSpinner.Finish(); finishErr != nil {
		logger.Warn("Failed to finish fallback spinner", "error", finishErr)
	}

	if err == nil {
		logger.Success(fmt.Sprintf("  ✓ Phase %d completed with fallback agent (%s)", phase.Number, fallbackAgent.ID))
		return result, nil
	}

	return "", err
}

func addProviderContext(prompt string, config templates.ProviderConfig) string {
	var providers []string

	if config.Claude {
		providers = append(providers, "Claude")
	}
	if config.Cursor {
		providers = append(providers, "Cursor")
	}
	if config.Windsurf {
		providers = append(providers, "Windsurf")
	}
	if config.Copilot {
		providers = append(providers, "GitHub Copilot")
	}
	if config.Gemini {
		providers = append(providers, "Gemini")
	}
	if config.Amp {
		providers = append(providers, "AMP")
	}
	if config.Codex {
		providers = append(providers, "Codex")
	}
	if config.Cline {
		providers = append(providers, "Cline")
	}
	if config.ContinueDev {
		providers = append(providers, "Continue.dev")
	}

	if len(providers) > 0 {
		prompt += fmt.Sprintf("\n\nTarget AI Assistants: %s\n", strings.Join(providers, ", "))
	}

	return prompt
}

func mergePhaseOutputs(outputs []string, context *ProjectContext, providerConfig templates.ProviderConfig) string {
	var sb strings.Builder

	// Build basic structure
	buildConfigHeader(&sb, context, providerConfig, outputs)
	buildOutputsSection(&sb, providerConfig)

	// Add content sections
	addPhaseContent(&sb, outputs, 3, "agents:", extractAgents)     // Phase 4 agents
	addPhaseContent(&sb, outputs, 2, "rules:", extractRules)       // Phase 3 rules
	addPhaseContent(&sb, outputs, 1, "sections:", extractSections) // Phase 2 sections

	// Add commands and MCP from phase 5
	if len(outputs) > 4 {
		addPhaseContent(&sb, outputs, 4, "commands:", extractCommands)
		addMCPServers(&sb, outputs[4])
	}

	return sb.String()
}

func buildConfigHeader(sb *strings.Builder, context *ProjectContext, providerConfig templates.ProviderConfig, outputs []string) {
	sb.WriteString("$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json\n\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(sb, "  name: %q\n", context.ProjectName)
	sb.WriteString("  version: \"1.0.0\"\n")

	// Extract description from phase outputs
	description := extractDescription(outputs)
	if description != "" {
		fmt.Fprintf(sb, "  description: %q\n", description)
	}
	sb.WriteString("\n")
}

func buildOutputsSection(sb *strings.Builder, providerConfig templates.ProviderConfig) {
	sb.WriteString("outputs:\n")

	if providerConfig.Claude {
		sb.WriteString("  - path: \"CLAUDE.md\"\n")
		sb.WriteString("  - path: \".claude/agents/\"\n")
		sb.WriteString("    type: \"agent\"\n")
		sb.WriteString("    naming_scheme: \"{name}.md\"\n")
	}
	if providerConfig.Cursor {
		sb.WriteString("  - path: \".cursor/rules/\"\n")
		sb.WriteString("    type: \"section\"\n")
		sb.WriteString("    naming_scheme: \"{name}.md\"\n")
	}
	if providerConfig.Windsurf {
		sb.WriteString("  - path: \".windsurfrules\"\n")
	}
	if providerConfig.Copilot {
		sb.WriteString("  - path: \".github/copilot-instructions.md\"\n")
	}
	if providerConfig.Gemini {
		sb.WriteString("  - path: \"GEMINI.md\"\n")
	}
	if providerConfig.Amp || providerConfig.Codex {
		sb.WriteString("  - path: \"AGENTS.md\"\n")
	}
	if providerConfig.Cline {
		sb.WriteString("  - path: \".clinerules/\"\n")
		sb.WriteString("    type: \"section\"\n")
		sb.WriteString("    naming_scheme: \"{name}.md\"\n")
	}
	if providerConfig.ContinueDev {
		sb.WriteString("  - path: \".continue/rules/\"\n")
		sb.WriteString("    type: \"section\"\n")
		sb.WriteString("    naming_scheme: \"{name}.md\"\n")
		sb.WriteString("  - path: \".continue/prompts/ai_rulez_prompts.yaml\"\n")
		sb.WriteString("    template:\n")
		sb.WriteString("      type: \"builtin\"\n")
		sb.WriteString("      value: \"continue-prompts\"\n")
	}
	sb.WriteString("\n")
}

func addPhaseContent(sb *strings.Builder, outputs []string, phaseIndex int, sectionName string, extractFn func(string) string) {
	if len(outputs) > phaseIndex {
		content := extractFn(outputs[phaseIndex])
		if content != "" {
			sb.WriteString(sectionName + "\n")
			sb.WriteString(content)
			sb.WriteString("\n")
		}
	}
}

func addMCPServers(sb *strings.Builder, phaseOutput string) {
	mcpServers := extractMCPServers(phaseOutput)
	if mcpServers != "" {
		sb.WriteString("\nmcp_servers:\n")
		sb.WriteString(mcpServers)
	}
}

func writeIncrementalUpdate(phaseNumber int, output string, context *ProjectContext, providerConfig templates.ProviderConfig) error {
	configPath := "ai_rulez.yaml"

	var existingContent string
	if data, err := os.ReadFile(configPath); err == nil {
		existingContent = string(data)
	}

	// Build incremental content based on phase
	var newContent string
	switch phaseNumber {
	case 1:
		// Phase 1: Initial structure with metadata and outputs
		newContent = buildInitialConfig(context, providerConfig, output)
	case 2:
		// Phase 2: Add sections
		sections := extractSections(output)
		if sections != "" {
			newContent = addSectionsToConfig(existingContent, sections)
		} else {
			return nil // No sections to add
		}
	case 3:
		// Phase 3: Add rules
		rules := extractRules(output)
		if rules != "" {
			newContent = addRulesToConfig(existingContent, rules)
		} else {
			return nil // No rules to add
		}
	case 4:
		// Phase 4: Add agents
		agents := extractAgents(output)
		if agents != "" {
			newContent = addAgentsToConfig(existingContent, agents)
		} else {
			return nil // No agents to add
		}
	case 5:
		// Phase 5: Add commands and MCP servers
		commands := extractCommands(output)
		mcpServers := extractMCPServers(output)
		newContent = addCommandsAndMCPToConfig(existingContent, commands, mcpServers)
	default:
		return fmt.Errorf("unknown phase number: %d", phaseNumber)
	}

	if newContent == "" {
		return nil // Nothing to write
	}

	// Write the updated content
	return os.WriteFile(configPath, []byte(newContent), 0o644)
}

func buildInitialConfig(context *ProjectContext, providerConfig templates.ProviderConfig, phaseOutput string) string {
	var sb strings.Builder

	// Schema and metadata
	sb.WriteString("$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json\n\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: %q\n", context.ProjectName)
	sb.WriteString("  version: \"1.0.0\"\n")

	// Extract description from phase 1 output
	description := extractDescription([]string{phaseOutput})
	if description != "" {
		fmt.Fprintf(&sb, "  description: %q\n", description)
	}
	sb.WriteString("\n")

	// Add outputs configuration
	sb.WriteString("outputs:\n")
	if providerConfig.Claude {
		sb.WriteString("  - path: \"CLAUDE.md\"\n")
		sb.WriteString("  - path: \".claude/agents/\"\n")
		sb.WriteString("    type: \"agent\"\n")
		sb.WriteString("    naming_scheme: \"{name}.md\"\n")
	}
	if providerConfig.Cursor {
		sb.WriteString("  - path: \".cursor/rules/\"\n")
		sb.WriteString("    type: \"section\"\n")
		sb.WriteString("    naming_scheme: \"{name}.md\"\n")
	}
	if providerConfig.Windsurf {
		sb.WriteString("  - path: \".windsurfrules\"\n")
	}
	if providerConfig.Copilot {
		sb.WriteString("  - path: \".github/copilot-instructions.md\"\n")
	}
	if providerConfig.Gemini {
		sb.WriteString("  - path: \"GEMINI.md\"\n")
	}
	if providerConfig.Amp || providerConfig.Codex {
		sb.WriteString("  - path: \"AGENTS.md\"\n")
	}
	if providerConfig.Cline {
		sb.WriteString("  - path: \".clinerules/\"\n")
		sb.WriteString("    type: \"section\"\n")
		sb.WriteString("    naming_scheme: \"{name}.md\"\n")
	}
	if providerConfig.ContinueDev {
		sb.WriteString("  - path: \".continue/rules/\"\n")
		sb.WriteString("    type: \"section\"\n")
		sb.WriteString("    naming_scheme: \"{name}.md\"\n")
		sb.WriteString("  - path: \".continue/prompts/ai_rulez_prompts.yaml\"\n")
		sb.WriteString("    template:\n")
		sb.WriteString("      type: \"builtin\"\n")
		sb.WriteString("      value: \"continue-prompts\"\n")
	}
	sb.WriteString("\n")

	return sb.String()
}

func addSectionsToConfig(existingContent, sections string) string {
	if sections == "" {
		return existingContent
	}

	// Find where to insert sections (before rules/agents/commands if they exist)
	lines := strings.Split(existingContent, "\n")
	var result []string
	sectionsAdded := false

	for _, line := range lines {
		// Look for first top-level key after outputs
		if !sectionsAdded && (strings.HasPrefix(line, "rules:") ||
			strings.HasPrefix(line, "agents:") ||
			strings.HasPrefix(line, "commands:") ||
			strings.HasPrefix(line, "mcp_servers:")) {
			// Insert sections before this key
			result = append(result, "sections:")
			sectionLines := strings.Split(sections, "\n")
			result = append(result, sectionLines...)
			result = append(result, "")
			sectionsAdded = true
		}
		result = append(result, line)
	}

	// If sections weren't inserted, add at end
	if !sectionsAdded {
		result = append(result, "sections:")
		sectionLines := strings.Split(sections, "\n")
		result = append(result, sectionLines...)
	}

	return strings.Join(result, "\n")
}

func addRulesToConfig(existingContent, rules string) string {
	if rules == "" {
		return existingContent
	}

	lines := strings.Split(existingContent, "\n")
	var result []string
	rulesAdded := false

	for _, line := range lines {
		if !rulesAdded && (strings.HasPrefix(line, "agents:") ||
			strings.HasPrefix(line, "commands:") ||
			strings.HasPrefix(line, "mcp_servers:")) {
			// Insert rules before this key
			result = append(result, "rules:")
			ruleLines := strings.Split(rules, "\n")
			result = append(result, ruleLines...)
			result = append(result, "")
			rulesAdded = true
		}
		result = append(result, line)
	}

	// If rules weren't inserted, add at end
	if !rulesAdded {
		result = append(result, "rules:")
		ruleLines := strings.Split(rules, "\n")
		result = append(result, ruleLines...)
	}

	return strings.Join(result, "\n")
}

func addAgentsToConfig(existingContent, agents string) string {
	if agents == "" {
		return existingContent
	}

	lines := strings.Split(existingContent, "\n")
	var result []string
	agentsAdded := false

	for _, line := range lines {
		if !agentsAdded && (strings.HasPrefix(line, "commands:") ||
			strings.HasPrefix(line, "mcp_servers:")) {
			// Insert agents before this key
			result = append(result, "agents:")
			agentLines := strings.Split(agents, "\n")
			result = append(result, agentLines...)
			result = append(result, "")
			agentsAdded = true
		}
		result = append(result, line)
	}

	// If agents weren't inserted, add at end
	if !agentsAdded {
		result = append(result, "agents:")
		agentLines := strings.Split(agents, "\n")
		result = append(result, agentLines...)
	}

	return strings.Join(result, "\n")
}

func addCommandsAndMCPToConfig(existingContent, commands, mcpServers string) string {
	lines := strings.Split(existingContent, "\n")
	var result []string

	// Add existing content
	result = append(result, lines...)

	// Add commands if they exist
	if commands != "" {
		result = append(result, "", "commands:")
		commandLines := strings.Split(commands, "\n")
		result = append(result, commandLines...)
	}

	// Add MCP servers if they exist
	if mcpServers != "" {
		result = append(result, "", "mcp_servers:")
		mcpLines := strings.Split(mcpServers, "\n")
		result = append(result, mcpLines...)
	}

	return strings.Join(result, "\n")
}

func extractDescription(outputs []string) string {
	if len(outputs) == 0 {
		return ""
	}

	// Look for description in first phase output
	lines := strings.Split(outputs[0], "\n")
	for _, line := range lines {
		if strings.Contains(line, "description:") || strings.Contains(line, "Description:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				desc := strings.TrimSpace(parts[1])
				desc = strings.Trim(desc, "\"'")
				return desc
			}
		}
	}

	return ""
}

func extractSections(output string) string {
	return extractYAMLSection(output, "sections:", "rules:", "agents:", "commands:")
}

func extractRules(output string) string {
	return extractYAMLSection(output, "rules:", "sections:", "agents:", "commands:")
}

func extractAgents(output string) string {
	return extractYAMLSection(output, "agents:", "rules:", "sections:", "commands:")
}

func extractCommands(output string) string {
	return extractYAMLSection(output, "commands:", "mcp_servers:", "rules:", "sections:")
}

func extractMCPServers(output string) string {
	return extractYAMLSection(output, "mcp_servers:", "commands:", "rules:", "sections:")
}

func extractYAMLSection(content string, startMarker string, endMarkers ...string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inSection := false

	for _, line := range lines {
		// Check if we're starting the target section
		if strings.TrimSpace(line) == strings.TrimSpace(startMarker) {
			inSection = true
			continue
		}

		// Check if we've hit another section
		if inSection {
			trimmed := strings.TrimSpace(line)
			for _, endMarker := range endMarkers {
				if trimmed == strings.TrimSpace(endMarker) {
					inSection = false
					break
				}
			}

			// Also stop if we hit a top-level key (no indentation)
			if line != "" && line[0] != ' ' && line[0] != '\t' && strings.Contains(line, ":") {
				inSection = false
			}
		}

		// Collect lines in section
		if inSection && strings.TrimSpace(line) != "" {
			// Ensure proper indentation (2 spaces for items)
			if !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "\t") {
				line = "  " + line
			}
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}
