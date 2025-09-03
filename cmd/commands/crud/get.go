package crud

import (
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// GetRuleCmd gets a specific rule by name
var GetRuleCmd = &cobra.Command{
	Use:   "rule <name>",
	Short: "Get a specific rule by name",
	Long:  `Retrieve and display information about a specific rule from your ai-rulez configuration.`,
	Args:  cobra.ExactArgs(1),
	Run:   runGetRule,
}

// GetSectionCmd gets a specific section by name
var GetSectionCmd = &cobra.Command{
	Use:   "section <name>",
	Short: "Get a specific section by name",
	Long:  `Retrieve and display information about a specific section from your ai-rulez configuration.`,
	Args:  cobra.ExactArgs(1),
	Run:   runGetSection,
}

// GetAgentCmd gets a specific agent by name
var GetAgentCmd = &cobra.Command{
	Use:   "agent <name>",
	Short: "Get a specific agent by name",
	Long:  `Retrieve and display information about a specific agent from your ai-rulez configuration.`,
	Args:  cobra.ExactArgs(1),
	Run:   runGetAgent,
}

// GetOutputCmd gets a specific output by index
var GetOutputCmd = &cobra.Command{
	Use:   "output <index>",
	Short: "Get a specific output by index",
	Long:  `Retrieve and display information about a specific output by its index (0-based) from your ai-rulez configuration.`,
	Args:  cobra.ExactArgs(1),
	Run:   runGetOutput,
}

func runGetRule(cmd *cobra.Command, args []string) {
	_, cfg := loadConfiguration()
	ruleName := args[0]

	// Find the rule by name
	for _, rule := range cfg.Rules {
		if rule.Name != ruleName {
			continue
		}
		fmt.Printf("🔍 Rule: %s\n", rule.Name)
		fmt.Printf("  Priority: %s (%d)\n", rule.Priority, rule.Priority.ToInt())
		fmt.Printf("  Content: %s\n", formatContent(rule.Content))
		if len(rule.Targets) > 0 {
			fmt.Printf("  Targets: %s\n", strings.Join(rule.Targets, ", "))
		}
		if rule.ID != "" {
			fmt.Printf("  ID: %s\n", rule.ID)
		}
		return
	}

	FmtError(oops.
		With("rule_name", ruleName).
		With("available_rules", getRuleNames(cfg.Rules)).
		Hint(fmt.Sprintf("Available rules: %s\nUse 'ai-rulez list rules' to see all rules", strings.Join(getRuleNames(cfg.Rules), ", "))).
		Errorf("rule '%s' not found", ruleName))
}

func runGetSection(cmd *cobra.Command, args []string) {
	_, cfg := loadConfiguration()
	sectionName := args[0]

	// Find the section by name
	for _, section := range cfg.Sections {
		if section.Name != sectionName {
			continue
		}
		fmt.Printf("📄 Section: %s\n", section.Name)
		fmt.Printf("  Priority: %s (%d)\n", section.Priority, section.Priority.ToInt())
		fmt.Printf("  Content: %s\n", formatContent(section.Content))
		if len(section.Targets) > 0 {
			fmt.Printf("  Targets: %s\n", strings.Join(section.Targets, ", "))
		}
		if section.ID != "" {
			fmt.Printf("  ID: %s\n", section.ID)
		}
		return
	}

	FmtError(oops.
		With("section_name", sectionName).
		With("available_sections", getSectionNames(cfg.Sections)).
		Hint(fmt.Sprintf("Available sections: %s\nUse 'ai-rulez list sections' to see all sections", strings.Join(getSectionNames(cfg.Sections), ", "))).
		Errorf("section '%s' not found", sectionName))
}

func runGetAgent(cmd *cobra.Command, args []string) {
	_, cfg := loadConfiguration()
	agentName := args[0]

	// Find the agent by name
	for i := range cfg.Agents {
		agent := &cfg.Agents[i]
		if agent.Name != agentName {
			continue
		}
		fmt.Printf("🤖 Agent: %s\n", agent.Name)
		fmt.Printf("  Description: %s\n", agent.Description)
		fmt.Printf("  Priority: %s (%d)\n", agent.Priority, agent.Priority.ToInt())
		if len(agent.Tools) > 0 {
			fmt.Printf("  Tools: %s\n", strings.Join(agent.Tools, ", "))
		}
		if agent.SystemPrompt != "" {
			fmt.Printf("  System Prompt: %s\n", formatContent(agent.SystemPrompt))
		}
		if agent.Template != nil {
			template, err := agent.GetTemplate()
			if err == nil && template != nil {
				fmt.Printf("  Template: %s:%s\n", template.Type, template.Value)
			}
		}
		if len(agent.Targets) > 0 {
			fmt.Printf("  Targets: %s\n", strings.Join(agent.Targets, ", "))
		}
		if agent.ID != "" {
			fmt.Printf("  ID: %s\n", agent.ID)
		}
		return
	}

	FmtError(oops.
		With("agent_name", agentName).
		With("available_agents", getAgentNames(cfg.Agents)).
		Hint(fmt.Sprintf("Available agents: %s\nUse 'ai-rulez list agents' to see all agents", strings.Join(getAgentNames(cfg.Agents), ", "))).
		Errorf("agent '%s' not found", agentName))
}

func runGetOutput(cmd *cobra.Command, args []string) {
	_, cfg := loadConfiguration()
	indexStr := args[0]

	// Parse the index
	var index int
	if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
		FmtError(oops.
			With("provided_index", indexStr).
			Hint("Index must be a number (0-based)\nExample: ai-rulez get output 0").
			Errorf("invalid index: %s", indexStr))
		return
	}

	// Check bounds
	if index < 0 || index >= len(cfg.Outputs) {
		FmtError(oops.
			With("provided_index", index).
			With("max_index", len(cfg.Outputs)-1).
			Hint(fmt.Sprintf("Index must be between 0 and %d\nUse 'ai-rulez list outputs' to see all outputs", len(cfg.Outputs)-1)).
			Errorf("index %d is out of bounds", index))
		return
	}

	output := cfg.Outputs[index]
	fmt.Printf("📤 Output [%d]:\n", index)
	fmt.Printf("  Path: %s\n", output.Path)
	if output.Type != "" {
		fmt.Printf("  Type: %s\n", output.Type)
	}
	if output.NamingScheme != "" {
		fmt.Printf("  Naming Scheme: %s\n", output.NamingScheme)
	}
	if output.Template != nil {
		template, err := output.GetTemplate()
		if err == nil && template != nil {
			fmt.Printf("  Template: %s:%s\n", template.Type, template.Value)
		}
	}
	fmt.Printf("  Is Directory: %t\n", output.IsDirectory())
}

// Helper functions to extract names for error messages
func getRuleNames(rules []config.Rule) []string {
	names := make([]string, len(rules))
	for i, rule := range rules {
		names[i] = rule.Name
	}
	return names
}

func getSectionNames(sections []config.Section) []string {
	names := make([]string, len(sections))
	for i, section := range sections {
		names[i] = section.Name
	}
	return names
}

func getAgentNames(agents []config.Agent) []string {
	names := make([]string, len(agents))
	for i := range agents {
		names[i] = agents[i].Name
	}
	return names
}

// formatContent truncates long content and adds preview
func formatContent(content string) string {
	const maxLen = 200
	if len(content) <= maxLen {
		return content
	}

	// Find the last complete word within the limit
	truncated := content[:maxLen]
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace != -1 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}
