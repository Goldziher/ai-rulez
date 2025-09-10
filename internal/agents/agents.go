package agents

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/spf13/cobra"
)

const (
	claudeAgentID = "claude"
)

type AgentInfo struct {
	ID      string
	Command string
	Display string
}

var supportedAgents = []AgentInfo{
	{ID: "amp", Command: "amp", Display: "AMP (Sourcegraph)"},
	{ID: claudeAgentID, Command: claudeAgentID, Display: "Claude (Anthropic)"},
	{ID: "continue-dev", Command: "cn", Display: "Continue.dev"},
	{ID: "gemini", Command: "gemini", Display: "Gemini (Google)"},
}

func detectAvailableAgents() []AgentInfo {
	var available []AgentInfo

	for _, agent := range supportedAgents {
		if isCommandAvailable(agent.Command) {
			available = append(available, agent)
		}
	}

	return available
}

func isCommandAvailable(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

func getAgentByID(id string) (*AgentInfo, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, agent := range supportedAgents {
		if agent.ID == id {
			return &agent, nil
		}
	}
	return nil, fmt.Errorf("unknown agent: %s", id)
}

func invokeAgent(agent AgentInfo, prompt string, timeout time.Duration) (string, error) { //nolint:gocyclo // Agent-specific command handling requires multiple cases
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd

	switch agent.ID {
	case claudeAgentID:
		cmd = exec.CommandContext(ctx, agent.Command, "--print", "--permission-mode", "bypassPermissions", prompt) //nolint:gosec // Intentional subprocess execution

	case "amp":
		cmd = exec.CommandContext(ctx, agent.Command, "--execute", prompt) //nolint:gosec // Intentional subprocess execution

	case "continue-dev":
		cmd = exec.CommandContext(ctx, agent.Command, "--print", prompt) //nolint:gosec // Intentional subprocess execution

	case "gemini":
		cmd = exec.CommandContext(ctx, agent.Command, "--prompt", prompt) //nolint:gosec // Intentional subprocess execution

	default:
		cmd = exec.CommandContext(ctx, agent.Command, prompt) //nolint:gosec // Intentional subprocess execution
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start agent: %w", err)
	}

	var outputBuilder strings.Builder

	// Read output silently, then format it nicely
	buffer := make([]byte, 1024)
	for {
		n, err := stdout.Read(buffer)
		if n > 0 {
			chunk := string(buffer[:n])
			outputBuilder.WriteString(chunk)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("error reading stdout: %w", err)
		}
	}

	// Don't print agent output during init - too verbose
	// The output is still captured and returned for processing

	stderrOutput, err := io.ReadAll(stderr)
	if err != nil {
		logger.Warn("Failed to read stderr", "error", err.Error())
	} else if len(stderrOutput) > 0 {
		logger.Warn("Agent stderr", "output", string(stderrOutput))
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("agent timed out after %v", timeout)
		}
		return "", fmt.Errorf("agent failed: %w", err)
	}

	// Agent completed - success will be indicated by the spinner/progress bar

	return outputBuilder.String(), nil
}

func ListAvailableAgents() {
	logger.Info("Available AI agents for configuration generation:")
	logger.Info("")

	available := detectAvailableAgents()

	for _, agent := range supportedAgents {
		status := "❌ Not installed"
		for _, avail := range available {
			if avail.ID == agent.ID {
				status = "✅ Available"
				break
			}
		}
		logger.Info(fmt.Sprintf("  %-15s %-25s %s", agent.ID, agent.Display, status))
	}

	logger.Info("")
	logger.Info("To use an agent, install its CLI tool and run:")
	logger.Info("  ai-rulez init --use-agent <agent-name>")
}

func ShouldPromptForAgent() bool {
	if os.Getenv("CI") != "" || os.Getenv("NO_INTERACTIVE") != "" {
		return false
	}

	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return false
	}

	available := detectAvailableAgents()
	return len(available) > 0
}

func getPreferredAgent(config templates.ProviderConfig) string {
	enabledCount := 0
	var lastEnabled string

	if config.Claude {
		enabledCount++
		lastEnabled = claudeAgentID
	}
	if config.Gemini {
		enabledCount++
		lastEnabled = "gemini"
	}
	if config.Amp {
		enabledCount++
		lastEnabled = "amp"
	}
	if config.ContinueDev {
		enabledCount++
		lastEnabled = "continue-dev"
	}

	if enabledCount == 1 {
		return lastEnabled
	}

	return claudeAgentID
}

func HandleAgentGeneration(cmd *cobra.Command, projectName string, config templates.ProviderConfig, useAgent string, autoYes bool) (string, bool) {
	selectedAgent := selectAgent(useAgent, config)
	if selectedAgent == nil {
		return "", false
	}

	if !confirmAgentUse(selectedAgent, autoYes) {
		return "", false
	}

	prompt := buildAgentPrompt(projectName, config)
	logger.Info("🤖 Generating configuration...", "agent", selectedAgent.ID)

	result, err := invokeAgent(*selectedAgent, prompt, 120*time.Second)
	if err != nil {
		logger.LogError("Failed to generate configuration", err, "agent", selectedAgent.ID)
		return "", false
	}

	return processAgentOutput(result, selectedAgent.ID)
}

func HandleAgentGenerationWithChain(cmd *cobra.Command, projectName string, config templates.ProviderConfig, useAgent string, autoYes bool) (string, bool) {
	selectedAgent := selectAgent(useAgent, config)
	if selectedAgent == nil {
		return "", false
	}

	if !confirmAgentUse(selectedAgent, autoYes) {
		return "", false
	}

	// Gather comprehensive project context
	logger.Info("🔍 Analyzing project structure...")
	projectContext := GatherProjectContext(projectName)

	// Execute the multi-phase chain
	result, err := ExecuteInitChain(*selectedAgent, projectContext, config)
	if err != nil {
		// Log the error but don't fail - partial success is still valuable
		logger.Warn("Some agent tasks encountered issues", "error", err.Error())
	}

	// Always return success if we got this far - the file has been created and potentially modified
	return result, true
}

func selectAgent(useAgent string, config templates.ProviderConfig) *AgentInfo {
	if useAgent != "" {
		agent, err := getAgentByID(useAgent)
		if err != nil {
			logger.Error("Unknown agent", "agent", useAgent)
			return nil
		}
		return agent
	}

	available := detectAvailableAgents()
	if len(available) == 0 {
		return nil
	}

	preferredAgent := getPreferredAgent(config)
	for i := range available {
		if available[i].ID == preferredAgent {
			return &available[i]
		}
	}

	for i := range available {
		if available[i].ID == claudeAgentID {
			return &available[i]
		}
	}

	return &available[0]
}

func confirmAgentUse(agent *AgentInfo, autoYes bool) bool {
	if autoYes {
		logger.Info(fmt.Sprintf("🤖 Using %s to generate your configuration (--yes)", agent.Display))
		return true
	}

	logger.Info(fmt.Sprintf("🤖 Would you like to use %s to generate your configuration? (y/N): ", agent.Display))
	reader := bufio.NewReader(os.Stdin)
	userResponse, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		logger.LogError("Failed to read user input", err)
		return false
	}

	response := strings.TrimSpace(strings.ToLower(userResponse))
	return response == "y" || response == "yes"
}

func processAgentOutput(result string, agentID string) (string, bool) {
	result = cleanAgentOutput(result)

	if strings.Contains(result, "ERROR_MARKER:") {
		extractAndLogError(result, agentID)
		return "", false
	}

	if len(result) < 100 || !strings.Contains(result, "metadata:") {
		logger.Warn("Agent output appears invalid, using default template", "agent", agentID, "output_size", len(result))
		return "", false
	}

	return strings.TrimSpace(result), true
}

func extractAndLogError(result string, agentID string) {
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if strings.Contains(line, "ERROR_MARKER:") {
			errorMsg := strings.TrimPrefix(line, "ERROR_MARKER:")
			errorMsg = strings.TrimSpace(errorMsg)
			logger.Warn("Agent reported error, using default template", "agent", agentID, "error", errorMsg)
			break
		}
	}
}

func cleanAgentOutput(result string) string {
	result = strings.TrimPrefix(result, "```yaml\n")
	result = strings.TrimPrefix(result, "```yml\n")
	result = strings.TrimPrefix(result, "```\n")
	result = strings.TrimSuffix(result, "\n```")
	result = strings.TrimSuffix(result, "```")

	lines := strings.Split(result, "\n")
	startIdx := -1
	endIdx := len(lines)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# AI-Rulez") ||
			strings.HasPrefix(trimmed, "version:") ||
			strings.HasPrefix(trimmed, "$schema:") ||
			strings.HasPrefix(trimmed, "metadata:") {
			startIdx = i
			break
		}
	}

	if startIdx >= 0 {
		for i := startIdx + 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if strings.HasPrefix(line, "```") ||
				strings.HasPrefix(line, "I am unable to") ||
				strings.HasPrefix(line, "I cannot") ||
				strings.HasPrefix(line, "Here is") ||
				strings.HasPrefix(line, "Error:") {
				endIdx = i
				break
			}
		}

		if startIdx < endIdx {
			result = strings.Join(lines[startIdx:endIdx], "\n")
		}
	}

	return result
}

func buildAgentPrompt(projectName string, config templates.ProviderConfig) string {
	info := AnalyzeCodebase(projectName)

	template := templates.GenerateConfigTemplate(projectName, config)

	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Customize this ai-rulez.yaml template for project '%s':\n\n", projectName))

	prompt.WriteString("INSTRUCTIONS:\n")
	prompt.WriteString("1. KEEP MINIMAL: Leave metadata and outputs sections mostly as-is\n")
	prompt.WriteString("2. SELECTIVELY UNCOMMENT based on project analysis:\n")
	prompt.WriteString("   - Uncomment 2-3 agents if they would benefit this project\n")
	prompt.WriteString("   - Uncomment 3-4 most relevant rules and customize for this project\n")
	prompt.WriteString("   - Uncomment 'Codebase Structure' and 'Coding Conventions' sections\n")

	if info.BuildCommand != "" || info.TestCommand != "" {
		prompt.WriteString("   - Uncomment commands section and add detected commands\n")
	}

	if info.HasMCP {
		prompt.WriteString("   - Uncomment ai-rulez MCP server (using " + info.MCPCommand + ")\n")
	}

	prompt.WriteString("\n3. CUSTOMIZE uncommented content:\n")
	prompt.WriteString("   - Update with actual project structure and conventions\n")
	prompt.WriteString("   - Keep descriptions concise and specific\n")
	prompt.WriteString("   - Remove generic placeholders\n")

	prompt.WriteString("\n4. VALIDATE YOUR YAML:\n")
	prompt.WriteString("   - Ensure all syntax is correct\n")
	prompt.WriteString("   - Keep proper indentation\n")
	prompt.WriteString("   - Test with an online YAML validator if unsure\n")

	prompt.WriteString("\n5. PRESERVE all inline documentation comments\n")
	prompt.WriteString("6. DELETE unused commented examples that don't apply\n\n")

	prompt.WriteString("PROJECT ANALYSIS:\n")
	prompt.WriteString("Detected project information:\n")
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

	prompt.WriteString(fmt.Sprintf("- Has Database: %v\n", info.HasDatabase))
	prompt.WriteString(fmt.Sprintf("- Has Docker: %v\n", info.HasDocker))

	if len(info.ConfigFiles) > 0 {
		prompt.WriteString(fmt.Sprintf("- Config Files: %s\n", strings.Join(info.ConfigFiles, ", ")))
	}

	prompt.WriteString("\nIMPORTANT: Most of the template should remain commented. Only uncomment what adds real value.\n")
	prompt.WriteString("Return ONLY the customized YAML content, no explanations or markdown blocks.\n\n")
	prompt.WriteString("ERROR HANDLING:\n")
	prompt.WriteString("If you encounter an error or cannot complete the task, output: ERROR_MARKER: followed by the error message\n")
	prompt.WriteString("Example: ERROR_MARKER: Authentication required\n\n")
	prompt.WriteString("CONFIGURATION TEMPLATE:\n")
	prompt.WriteString("Template to customize:\n\n")
	prompt.WriteString(template)

	return prompt.String()
}
