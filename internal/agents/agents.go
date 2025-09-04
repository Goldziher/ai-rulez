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

type AgentInfo struct {
	ID      string
	Command string
	Display string
}

var supportedAgents = []AgentInfo{
	{ID: "amp", Command: "amp", Display: "AMP (Sourcegraph)"},
	{ID: "claude", Command: "claude", Display: "Claude (Anthropic)"},
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

func invokeAgent(agent AgentInfo, prompt string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd

	switch agent.ID {
	case "claude":
		cmd = exec.CommandContext(ctx, agent.Command, "--no-conversation", "--max-tokens", "4000") //nolint:gosec // Intentional subprocess execution
		cmd.Stdin = strings.NewReader(prompt)

	case "amp", "codex", "cursor":
		cmd = exec.CommandContext(ctx, agent.Command, prompt) //nolint:gosec // Intentional subprocess execution

	case "gemini":
		cmd = exec.CommandContext(ctx, agent.Command, prompt) //nolint:gosec // Intentional subprocess execution

	default:
		cmd = exec.CommandContext(ctx, agent.Command) //nolint:gosec // Intentional subprocess execution
		cmd.Stdin = strings.NewReader(prompt)
	}

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("agent timed out after %v", timeout)
		}
		return "", fmt.Errorf("agent failed: %w", err)
	}

	return string(output), nil
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

func HandleAgentGeneration(cmd *cobra.Command, projectName string, config templates.ProviderConfig, useAgent string) (string, bool) {
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

		for _, agent := range available {
			if agent.ID == "claude" {
				selectedAgent = &agent
				break
			}
		}
		if selectedAgent == nil {
			selectedAgent = &available[0]
		}
	}

	prompt := buildAgentPrompt(projectName, config)

	logger.Info(fmt.Sprintf("🤖 Would you like to use %s to generate your configuration? (y/N): ", selectedAgent.Display))
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		logger.LogError("Failed to read user input", err)
		return "", false
	}
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		return "", false
	}

	logger.Info("🤖 Generating configuration...", "agent", selectedAgent.ID)

	result, err := invokeAgent(*selectedAgent, prompt, 60*time.Second)
	if err != nil {
		logger.LogError("Failed to generate configuration", err, "agent", selectedAgent.ID)
		return "", false
	}

	result = strings.TrimPrefix(result, "```yaml\n")
	result = strings.TrimPrefix(result, "```yml\n")
	result = strings.TrimPrefix(result, "```\n")
	result = strings.TrimSuffix(result, "\n```")
	result = strings.TrimSuffix(result, "```")

	return strings.TrimSpace(result), true
}

func buildAgentPrompt(projectName string, config templates.ProviderConfig) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Generate an ai-rulez configuration file (ai_rulez.yaml) for a project called '%s'. ", projectName))
	prompt.WriteString("The configuration should follow the ai-rulez schema and include:\n\n")

	prompt.WriteString("1. Metadata section with project name and version\n")
	prompt.WriteString("2. Output configurations for:\n")

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
