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
	agentPriority     int
	agentTools        []string
	agentSystemPrompt string
	agentID           string
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
	// Add agent flags
	AddAgentCmd.Flags().StringVarP(&agentDescription, "description", "d", "", "Description of the agent")
	AddAgentCmd.Flags().IntVarP(&agentPriority, "priority", "p", 5, "Priority level for the agent (1-10)")
	AddAgentCmd.Flags().StringSliceVarP(&agentTools, "tools", "t", []string{}, "Comma-separated list of tools the agent can use")
	AddAgentCmd.Flags().StringVarP(&agentSystemPrompt, "system-prompt", "s", "", "System prompt for the agent (will prompt via stdin if not provided)")
	AddAgentCmd.Flags().StringVar(&agentID, "id", "", "Optional unique identifier for the agent")

	// Update agent flags
	UpdateAgentCmd.Flags().StringVarP(&agentDescription, "description", "d", "", "New description for the agent")
	UpdateAgentCmd.Flags().IntVarP(&agentPriority, "priority", "p", 0, "New priority level for the agent (1-10)")
	UpdateAgentCmd.Flags().StringSliceVarP(&agentTools, "tools", "t", []string{}, "New comma-separated list of tools the agent can use")
	UpdateAgentCmd.Flags().StringVarP(&agentSystemPrompt, "system-prompt", "s", "", "New system prompt for the agent (will prompt via stdin if not provided)")
}

func runAddAgent(cmd *cobra.Command, args []string) {
	agentName := args[0]
	configPath, cfg := loadConfiguration()

	// Check for duplicate
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == agentName {
			fmt.Fprintf(os.Stderr, "Error: Agent '%s' already exists in configuration\n", agentName)
			os.Exit(1)
		}
	}

	// Get system prompt if not provided
	if agentSystemPrompt == "" {
		fmt.Println("Enter agent system prompt (press Ctrl+D when done):")
		content, err := readFromStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading system prompt: %v\n", err)
			os.Exit(1)
		}
		agentSystemPrompt = content
	}

	// Add new agent
	newAgent := config.Agent{
		ID:           agentID,
		Name:         agentName,
		Description:  agentDescription,
		Priority:     agentPriority,
		Tools:        agentTools,
		SystemPrompt: agentSystemPrompt,
	}
	cfg.Agents = append(cfg.Agents, newAgent)

	// Save configuration
	if err := config.SaveConfig(cfg, configPath); err != nil {
		FmtError(err)
		os.Exit(1)
	}

	fmt.Printf("✅ Added agent '%s'", agentName)
	if agentPriority > 0 {
		fmt.Printf(" with priority %d", agentPriority)
	}
	fmt.Printf(" to %s\n", configPath)
}

func runUpdateAgent(cmd *cobra.Command, args []string) {
	agentName := args[0]
	configPath, cfg := loadConfiguration()

	// Find agent
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

	// If no flags provided, prompt for system prompt
	if agentSystemPrompt == "" && agentDescription == "" && agentPriority == 0 && len(agentTools) == 0 {
		fmt.Printf("Current system prompt: %s\n", cfg.Agents[agentIndex].SystemPrompt)
		fmt.Println("Enter new system prompt (press Ctrl+D when done, or press Enter to keep current):")
		content, err := readFromStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading system prompt: %v\n", err)
			os.Exit(1)
		}
		if strings.TrimSpace(content) != "" {
			agentSystemPrompt = content
		}
	}

	// Update fields
	if agentDescription != "" {
		cfg.Agents[agentIndex].Description = agentDescription
	}
	if agentPriority > 0 {
		cfg.Agents[agentIndex].Priority = agentPriority
	}
	if len(agentTools) > 0 {
		cfg.Agents[agentIndex].Tools = agentTools
	}
	if agentSystemPrompt != "" {
		cfg.Agents[agentIndex].SystemPrompt = agentSystemPrompt
	}

	// Save configuration
	if err := config.SaveConfig(cfg, configPath); err != nil {
		FmtError(err)
		os.Exit(1)
	}

	fmt.Printf("✅ Updated agent '%s' in %s\n", agentName, configPath)
}

func runDeleteAgent(cmd *cobra.Command, args []string) {
	agentName := args[0]
	configPath, cfg := loadConfiguration()

	// Find and remove agent
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

	// Remove the agent
	cfg.Agents = append(cfg.Agents[:agentIndex], cfg.Agents[agentIndex+1:]...)

	// Save configuration
	if err := config.SaveConfig(cfg, configPath); err != nil {
		FmtError(err)
		os.Exit(1)
	}

	fmt.Printf("✅ Deleted agent '%s' from %s\n", agentName, configPath)
}
