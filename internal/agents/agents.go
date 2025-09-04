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
	{ID: "codex", Command: "codex", Display: "Codex (OpenAI)"},
	{ID: "cursor", Command: "cursor-agent", Display: "Cursor Agent (Cursor)"},
	{ID: "gemini", Command: "gemini", Display: "Gemini (Google)"},
}

func detectAvailableAgents() ([]AgentInfo, error) {
	var available []AgentInfo

	for _, agent := range supportedAgents {
		if isCommandAvailable(agent.Command) {
			available = append(available, agent)
		}
	}

	return available, nil
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
		cmd = exec.CommandContext(ctx, agent.Command, "--print", prompt) //nolint:gosec // Intentional subprocess execution

	case "amp":
		cmd = exec.CommandContext(ctx, agent.Command, "--execute", prompt) //nolint:gosec // Intentional subprocess execution

	case "codex":
		cmd = exec.CommandContext(ctx, agent.Command, "exec", prompt) //nolint:gosec // Intentional subprocess execution

	case "cursor":
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

	logger.Info("📝 Agent response:")

	buffer := make([]byte, 1024)
	for {
		n, err := stdout.Read(buffer)
		if n > 0 {
			chunk := string(buffer[:n])
			outputBuilder.WriteString(chunk)
			fmt.Print(chunk)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("error reading stdout: %w", err)
		}
	}

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

	fmt.Println()
	logger.Info("✅ Agent completed successfully")

	return outputBuilder.String(), nil
}

func ListAvailableAgents() {
	logger.Info("Available AI agents for configuration generation:")
	logger.Info("")

	available, _ := detectAvailableAgents() //nolint:errcheck // Error intentionally ignored for display purposes

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

	available, _ := detectAvailableAgents() //nolint:errcheck // Error intentionally ignored - fail safe to false
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
	if config.Codex {
		enabledCount++
		lastEnabled = "codex"
	}
	if config.Cursor {
		enabledCount++
		lastEnabled = "cursor"
	}

	if enabledCount == 1 {
		return lastEnabled
	}

	return claudeAgentID
}

func HandleAgentGeneration(cmd *cobra.Command, projectName string, config templates.ProviderConfig, useAgent string, autoYes bool) (string, bool) { //nolint:gocyclo // Agent selection logic has multiple fallback scenarios
	var selectedAgent *AgentInfo

	if useAgent != "" {
		agent, err := getAgentByID(useAgent)
		if err != nil {
			logger.Error("Unknown agent", "agent", useAgent)
			return "", false
		}
		selectedAgent = agent
	} else {
		available, _ := detectAvailableAgents() //nolint:errcheck // Error intentionally ignored
		if len(available) == 0 {
			return "", false
		}

		preferredAgent := getPreferredAgent(config)
		for _, agent := range available {
			if agent.ID == preferredAgent {
				selectedAgent = &agent
				break
			}
		}

		if selectedAgent == nil {
			for _, agent := range available {
				if agent.ID == claudeAgentID {
					selectedAgent = &agent
					break
				}
			}
		}

		if selectedAgent == nil {
			selectedAgent = &available[0]
		}
	}

	prompt := buildAgentPrompt(projectName, config)

	var response string
	if autoYes {
		logger.Info(fmt.Sprintf("🤖 Using %s to generate your configuration (--yes)", selectedAgent.Display))
		response = "y"
	} else {
		logger.Info(fmt.Sprintf("🤖 Would you like to use %s to generate your configuration? (y/N): ", selectedAgent.Display))
		reader := bufio.NewReader(os.Stdin)
		userResponse, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			logger.LogError("Failed to read user input", err)
			return "", false
		}
		response = strings.TrimSpace(strings.ToLower(userResponse))
	}

	if response != "y" && response != "yes" {
		return "", false
	}

	logger.Info("🤖 Generating configuration...", "agent", selectedAgent.ID)

	result, err := invokeAgent(*selectedAgent, prompt, 120*time.Second)
	if err != nil {
		logger.LogError("Failed to generate configuration", err, "agent", selectedAgent.ID)
		return "", false
	}

	result = cleanAgentOutput(result)

	return strings.TrimSpace(result), true
}

func cleanAgentOutput(result string) string {
	// Remove markdown code blocks
	result = strings.TrimPrefix(result, "```yaml\n")
	result = strings.TrimPrefix(result, "```yml\n")
	result = strings.TrimPrefix(result, "```\n")
	result = strings.TrimSuffix(result, "\n```")
	result = strings.TrimSuffix(result, "```")

	// Find YAML content between lines that start with version: or $schema:
	lines := strings.Split(result, "\n")
	startIdx := -1
	endIdx := len(lines)

	// Find the start of YAML content
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "version:") ||
			strings.HasPrefix(trimmed, "$schema:") ||
			strings.HasPrefix(trimmed, "metadata:") {
			startIdx = i
			break
		}
	}

	// Find the end of YAML content (look for markdown blocks or other non-YAML content)
	if startIdx >= 0 {
		for i := startIdx + 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			// Stop if we hit markdown blocks or other non-YAML patterns
			if strings.HasPrefix(line, "```") ||
				strings.HasPrefix(line, "I am unable to") ||
				strings.HasPrefix(line, "# ") {
				endIdx = i
				break
			}
		}

		// Extract just the YAML portion
		if startIdx < endIdx {
			result = strings.Join(lines[startIdx:endIdx], "\n")
		}
	}

	return result
}

func buildAgentPrompt(projectName string, config templates.ProviderConfig) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Generate an ai-rulez configuration file (ai_rulez.yaml) for a project called '%s'. ", projectName))
	prompt.WriteString("The configuration MUST follow this exact schema format:\n\n")

	prompt.WriteString("$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json\n\n")
	prompt.WriteString("metadata:\n  name: \"ProjectName\"\n  version: \"1.0.0\"\n\n")
	prompt.WriteString("outputs:\n  - path: \"FILENAME.md\"\n\n")
	prompt.WriteString("rules:\n  - name: \"Rule Name\"\n    priority: critical|high|medium|low\n    content: |\n      Rule description\n\n")
	prompt.WriteString("sections:\n  - name: \"Section Name\"\n    priority: critical|high|medium|low\n    content: |\n      Section content\n\n")
	prompt.WriteString("Include output configurations for:\n")

	if config.Claude {
		prompt.WriteString("   - Claude (CLAUDE.md and .claude/agents/)\n")
	}
	if config.Cursor {
		prompt.WriteString("   - Cursor (.cursor/rules/)\n")
	}
	if config.Windsurf {
		prompt.WriteString("   - Windsurf (.windsurf/)\n")
	}
	if config.Copilot {
		prompt.WriteString("   - GitHub Copilot (.github/copilot-instructions.md)\n")
	}
	if config.Gemini {
		prompt.WriteString("   - Gemini (GEMINI.md)\n")
	}
	if config.Amp || config.Codex {
		prompt.WriteString("   - AMP/Codex (AGENTS.md)\n")
	}
	if config.Cline {
		prompt.WriteString("   - Cline (.clinerules/)\n")
	}
	if config.ContinueDev {
		prompt.WriteString("   - Continue.dev (.continue/rules/)\n")
	}

	prompt.WriteString("\n3. At least 5 high-quality rules for software development best practices\n")
	prompt.WriteString("4. At least 3 documentation sections\n")

	if config.Claude {
		prompt.WriteString("5. At least 2 agent configurations for Claude\n")
	}

	prompt.WriteString("\nGenerate only the YAML content, no explanations or markdown code blocks.")

	return prompt.String()
}
