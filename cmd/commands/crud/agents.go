package crud

import (
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/spf13/cobra"
)

var (
	agentDescription  string
	agentPriority     string
	agentTools        []string
	agentSystemPrompt string
	agentID           string
	agentTargets      []string
)

// AddAgentCmd adds a new agent to the configuration
var AddAgentCmd = &cobra.Command{
	Use:   "agent [name]",
	Short: "Add a new agent to configuration",
	Long: `Add a new agent to your AI rules configuration file.
The agent name is provided as an argument, and you can specify
description, priority, tools, and system prompt. The system prompt
will be read from stdin if not provided via flag.`,
	Args: cobra.ExactArgs(1),
	Run:  runAddAgent,
}

// UpdateAgentCmd updates an existing agent
var UpdateAgentCmd = &cobra.Command{
	Use:   "agent [name]",
	Short: "Update an existing agent",
	Long: `Update an existing agent in your AI rules configuration file.
You can update the description, priority, tools, or system prompt.
If no system prompt is provided via flag, you'll be prompted to enter
it via stdin.`,
	Args: cobra.ExactArgs(1),
	Run:  runUpdateAgent,
}

// DeleteAgentCmd deletes an agent
var DeleteAgentCmd = &cobra.Command{
	Use:   "agent [name]",
	Short: "Delete an existing agent",
	Long: `Delete an existing agent from your AI rules configuration file.
This will permanently remove the agent and cannot be undone.`,
	Args: cobra.ExactArgs(1),
	Run:  runDeleteAgent,
}

func init() {
	AddAgentCmd.Flags().StringVarP(&agentDescription, "description", "d", "", "Description of the agent")
	AddAgentCmd.Flags().StringVarP(&agentPriority, "priority", "p", "medium", "Priority level for the agent (critical, high, medium, low, minimal)")
	AddAgentCmd.Flags().StringSliceVarP(&agentTools, "tools", "t", []string{}, "Comma-separated list of tools the agent can use")
	AddAgentCmd.Flags().StringVarP(&agentSystemPrompt, "system-prompt", "s", "", "System prompt for the agent (will prompt via stdin if not provided)")
	AddAgentCmd.Flags().StringVar(&agentID, "id", "", "Optional unique identifier for the agent")
	AddAgentCmd.Flags().StringSliceVar(&agentTargets, "targets", []string{}, "Target file patterns (glob patterns)")

	UpdateAgentCmd.Flags().StringVarP(&agentDescription, "description", "d", "", "New description for the agent")
	UpdateAgentCmd.Flags().StringVarP(&agentPriority, "priority", "p", "", "New priority level for the agent (critical, high, medium, low, minimal)")
	UpdateAgentCmd.Flags().StringSliceVarP(&agentTools, "tools", "t", []string{}, "New comma-separated list of tools the agent can use")
	UpdateAgentCmd.Flags().StringVarP(&agentSystemPrompt, "system-prompt", "s", "", "New system prompt for the agent (will prompt via stdin if not provided)")
	UpdateAgentCmd.Flags().StringSliceVar(&agentTargets, "targets", []string{}, "Target file patterns (glob patterns)")
}

func runAddAgent(cmd *cobra.Command, args []string) {
	agentName := args[0]
	configPath, cfg := loadConfiguration()

	for i := range cfg.Agents {
		if cfg.Agents[i].Name == agentName {
			fmt.Fprintf(os.Stderr, "Error: Agent '%s' already exists in configuration\n", agentName)
			os.Exit(1)
		}
	}

	if agentSystemPrompt == "" {
		fmt.Println("Enter agent system prompt (press Ctrl+D when done):")
		content, err := readFromStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading system prompt: %v\n", err)
			os.Exit(1)
		}
		agentSystemPrompt = content
	}

	newAgent := config.Agent{
		ID:           agentID,
		Name:         agentName,
		Description:  agentDescription,
		Priority:     config.Priority(agentPriority),
		Tools:        agentTools,
		SystemPrompt: agentSystemPrompt,
		Targets:      agentTargets,
	}
	cfg.Agents = append(cfg.Agents, newAgent)

	if err := config.SaveConfig(cfg, configPath); err != nil {
		FmtError(err)
		os.Exit(1)
	}

	fmt.Printf("✅ Added agent '%s'", agentName)
	if agentPriority != "" && agentPriority != "medium" {
		fmt.Printf(" with priority %s", agentPriority)
	}
	fmt.Printf(" to %s\n", configPath)
}

// findAgentIndex returns the index of the agent with the given name, or -1 if not found
func findAgentIndex(cfg *config.Config, agentName string) int {
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == agentName {
			return i
		}
	}
	return -1
}

// promptForSystemPrompt prompts the user for a new system prompt if none was provided
func promptForSystemPrompt(currentPrompt string) (string, error) {
	fmt.Printf("Current system prompt: %s\n", currentPrompt)
	fmt.Println("Enter new system prompt (press Ctrl+D when done, or press Enter to keep current):")
	content, err := readFromStdin()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) != "" {
		return content, nil
	}
	return "", nil
}

// updateAgentFields updates the agent's fields if new values were provided
func updateAgentFields(agent *config.Agent, description string, priority string, tools []string, systemPrompt string, targets []string) {
	if description != "" {
		agent.Description = description
	}
	if priority != "" {
		agent.Priority = config.Priority(priority)
	}
	if len(tools) > 0 {
		agent.Tools = tools
	}
	if systemPrompt != "" {
		agent.SystemPrompt = systemPrompt
	}
	if len(targets) > 0 {
		agent.Targets = targets
	}
}

func runUpdateAgent(cmd *cobra.Command, args []string) {
	agentName := args[0]
	configPath, cfg := loadConfiguration()

	agentIndex := findAgentIndex(cfg, agentName)
	if agentIndex == -1 {
		fmt.Fprintf(os.Stderr, "Error: Agent '%s' not found\n", agentName)
		os.Exit(1)
	}

	// If no flags provided, prompt for system prompt
	if agentSystemPrompt == "" && agentDescription == "" && agentPriority == "" && len(agentTools) == 0 && len(agentTargets) == 0 {
		newPrompt, err := promptForSystemPrompt(cfg.Agents[agentIndex].SystemPrompt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading system prompt: %v\n", err)
			os.Exit(1)
		}
		agentSystemPrompt = newPrompt
	}

	// Update agent fields
	updateAgentFields(&cfg.Agents[agentIndex], agentDescription, agentPriority, agentTools, agentSystemPrompt, agentTargets)

	if err := config.SaveConfig(cfg, configPath); err != nil {
		FmtError(err)
		os.Exit(1)
	}

	fmt.Printf("✅ Updated agent '%s' in %s\n", agentName, configPath)
}

func runDeleteAgent(cmd *cobra.Command, args []string) {
	agentName := args[0]
	configPath, cfg := loadConfiguration()

	agentIndex := -1
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == agentName {
			agentIndex = i
			break
		}
	}

	if agentIndex == -1 {
		fmt.Fprintf(os.Stderr, "Error: Agent '%s' not found\n", agentName)
		os.Exit(1)
	}

	cfg.Agents = append(cfg.Agents[:agentIndex], cfg.Agents[agentIndex+1:]...)

	if err := config.SaveConfig(cfg, configPath); err != nil {
		FmtError(err)
		os.Exit(1)
	}

	fmt.Printf("✅ Deleted agent '%s' from %s\n", agentName, configPath)
}
