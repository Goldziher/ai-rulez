package crud

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/spf13/cobra"
)

var (
	listDetailed bool
	listJSON     bool
	listFilter   string
)

// ListRulesCmd lists all rules from configuration
var ListRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "List all rules from configuration",
	Long:  `Display all rules from your ai-rulez configuration with names, priorities, and content previews.`,
	Run:   runListRules,
}

// ListSectionsCmd lists all sections from configuration
var ListSectionsCmd = &cobra.Command{
	Use:   "sections",
	Short: "List all sections from configuration",
	Long:  `Display all sections from your ai-rulez configuration with titles, priorities, and content previews.`,
	Run:   runListSections,
}

// ListAgentsCmd lists all agents from configuration
var ListAgentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List all agents from configuration",
	Long:  `Display all agents from your ai-rulez configuration with names, descriptions, tools, and priorities.`,
	Run:   runListAgents,
}

// ListOutputsCmd lists all outputs from configuration
var ListOutputsCmd = &cobra.Command{
	Use:   "outputs",
	Short: "List all outputs from configuration",
	Long:  `Display all configured outputs with paths, types, and naming schemes.`,
	Run:   runListOutputs,
}

func init() {
	// Add flags to all list commands
	for _, cmd := range []*cobra.Command{ListRulesCmd, ListSectionsCmd, ListAgentsCmd, ListOutputsCmd} {
		cmd.Flags().BoolVar(&listDetailed, "detailed", false, "Show detailed information")
		cmd.Flags().BoolVar(&listJSON, "json", false, "Output in JSON format")
		cmd.Flags().StringVar(&listFilter, "filter", "", "Filter by name/title (case insensitive)")
	}
}

func runListRules(cmd *cobra.Command, args []string) {
	_, cfg := loadConfiguration()

	if len(cfg.Rules) == 0 {
		fmt.Println("No rules found in configuration")
		return
	}

	// Filter rules if needed
	var filteredRules []config.Rule
	for _, rule := range cfg.Rules {
		if listFilter == "" || strings.Contains(strings.ToLower(rule.Name), strings.ToLower(listFilter)) {
			filteredRules = append(filteredRules, rule)
		}
	}

	if listJSON {
		output, _ := json.MarshalIndent(filteredRules, "", "  ")
		fmt.Println(string(output))
		return
	}

	if len(filteredRules) == 0 {
		fmt.Printf("No rules found matching filter: %s\n", listFilter)
		return
	}

	fmt.Printf("Found %d rule(s):\n\n", len(filteredRules))

	if listDetailed {
		for i, rule := range filteredRules {
			fmt.Printf("Rule %d:\n", i+1)
			fmt.Printf("  Name: %s\n", rule.Name)
			fmt.Printf("  Priority: %d\n", rule.Priority)
			fmt.Printf("  Content:\n%s\n", indentContent(rule.Content, "    "))
			if i < len(filteredRules)-1 {
				fmt.Println()
			}
		}
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		//nolint:errcheck
		fmt.Fprintln(w, "NAME\tPRIORITY\tCONTENT PREVIEW")
		//nolint:errcheck
		fmt.Fprintln(w, "----\t--------\t---------------")

		for _, rule := range filteredRules {
			preview := truncateString(rule.Content, 50)
			//nolint:errcheck
			fmt.Fprintf(w, "%s\t%d\t%s\n", rule.Name, rule.Priority, preview)
		}
		//nolint:errcheck,gosec
		w.Flush()
	}
}

func runListSections(cmd *cobra.Command, args []string) {
	_, cfg := loadConfiguration()

	if len(cfg.Sections) == 0 {
		fmt.Println("No sections found in configuration")
		return
	}

	// Filter sections if needed
	var filteredSections []config.Section
	for _, section := range cfg.Sections {
		if listFilter == "" || strings.Contains(strings.ToLower(section.Name), strings.ToLower(listFilter)) {
			filteredSections = append(filteredSections, section)
		}
	}

	if listJSON {
		output, _ := json.MarshalIndent(filteredSections, "", "  ")
		fmt.Println(string(output))
		return
	}

	if len(filteredSections) == 0 {
		fmt.Printf("No sections found matching filter: %s\n", listFilter)
		return
	}

	fmt.Printf("Found %d section(s):\n\n", len(filteredSections))

	if listDetailed {
		for i, section := range filteredSections {
			fmt.Printf("Section %d:\n", i+1)
			fmt.Printf("  Name: %s\n", section.Name)
			fmt.Printf("  Priority: %d\n", section.Priority)
			fmt.Printf("  Content:\n%s\n", indentContent(section.Content, "    "))
			if i < len(filteredSections)-1 {
				fmt.Println()
			}
		}
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		//nolint:errcheck
		fmt.Fprintln(w, "NAME\tPRIORITY\tCONTENT PREVIEW")
		//nolint:errcheck
		fmt.Fprintln(w, "----\t--------\t---------------")

		for _, section := range filteredSections {
			preview := truncateString(section.Content, 50)
			//nolint:errcheck
			fmt.Fprintf(w, "%s\t%d\t%s\n", section.Name, section.Priority, preview)
		}
		//nolint:errcheck,gosec
		w.Flush()
	}
}

func runListAgents(cmd *cobra.Command, args []string) {
	_, cfg := loadConfiguration()

	if len(cfg.Agents) == 0 {
		fmt.Println("No agents found in configuration")
		return
	}

	// Filter agents if needed
	var filteredAgents []config.Agent
	for i := range cfg.Agents {
		agent := &cfg.Agents[i]
		if listFilter == "" || strings.Contains(strings.ToLower(agent.Name), strings.ToLower(listFilter)) {
			filteredAgents = append(filteredAgents, *agent)
		}
	}

	if listJSON {
		output, _ := json.MarshalIndent(filteredAgents, "", "  ")
		fmt.Println(string(output))
		return
	}

	if len(filteredAgents) == 0 {
		fmt.Printf("No agents found matching filter: %s\n", listFilter)
		return
	}

	fmt.Printf("Found %d agent(s):\n\n", len(filteredAgents))

	if listDetailed {
		for i := range filteredAgents {
			agent := &filteredAgents[i]
			fmt.Printf("Agent %d:\n", i+1)
			fmt.Printf("  Name: %s\n", agent.Name)
			fmt.Printf("  Description: %s\n", agent.Description)
			fmt.Printf("  Priority: %d\n", agent.Priority)
			fmt.Printf("  Tools: %s\n", strings.Join(agent.Tools, ", "))
			if agent.SystemPrompt != "" {
				fmt.Printf("  System Prompt:\n%s\n", indentContent(agent.SystemPrompt, "    "))
			}
			if i < len(filteredAgents)-1 {
				fmt.Println()
			}
		}
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		//nolint:errcheck
		fmt.Fprintln(w, "NAME\tPRIORITY\tDESCRIPTION\tTOOLS")
		//nolint:errcheck
		fmt.Fprintln(w, "----\t--------\t-----------\t-----")

		for i := range filteredAgents {
			agent := &filteredAgents[i]
			tools := strings.Join(agent.Tools, ",")
			if len(tools) > 20 {
				tools = tools[:17] + "..."
			}
			description := truncateString(agent.Description, 30)
			//nolint:errcheck
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", agent.Name, agent.Priority, description, tools)
		}
		//nolint:errcheck,gosec
		w.Flush()
	}
}

func runListOutputs(cmd *cobra.Command, args []string) {
	_, cfg := loadConfiguration()

	if len(cfg.Outputs) == 0 {
		fmt.Println("No outputs found in configuration")
		return
	}

	filteredOutputs := filterOutputs(cfg.Outputs)

	if listJSON {
		printOutputsJSON(filteredOutputs)
		return
	}

	if len(filteredOutputs) == 0 {
		fmt.Printf("No outputs found matching filter: %s\n", listFilter)
		return
	}

	fmt.Printf("Found %d output(s):\n\n", len(filteredOutputs))

	if listDetailed {
		printOutputsDetailed(filteredOutputs)
	} else {
		printOutputsTable(filteredOutputs)
	}
}

// filterOutputs filters outputs based on the list filter
func filterOutputs(outputs []config.Output) []config.Output {
	var filteredOutputs []config.Output
	for _, output := range outputs {
		path := output.GetPath()
		if listFilter == "" || strings.Contains(strings.ToLower(path), strings.ToLower(listFilter)) {
			filteredOutputs = append(filteredOutputs, output)
		}
	}
	return filteredOutputs
}

// printOutputsJSON prints outputs in JSON format
func printOutputsJSON(outputs []config.Output) {
	output, _ := json.MarshalIndent(outputs, "", "  ")
	fmt.Println(string(output))
}

// printOutputsDetailed prints outputs in detailed format
func printOutputsDetailed(outputs []config.Output) {
	for i, output := range outputs {
		fmt.Printf("Output %d:\n", i+1)
		fmt.Printf("  Path: %s\n", output.GetPath())
		if output.Type != "" {
			fmt.Printf("  Type: %s\n", output.Type)
		}
		if output.NamingScheme != "" {
			fmt.Printf("  Naming Scheme: %s\n", output.NamingScheme)
		}
		if output.Template != "" {
			fmt.Printf("  Template: %s\n", truncateString(output.Template, 50))
		}
		fmt.Printf("  Is Directory: %t\n", output.IsDirectory())
		if output.File != "" {
			fmt.Printf("  [DEPRECATED] File: %s (use path instead)\n", output.File)
		}
		if i < len(outputs)-1 {
			fmt.Println()
		}
	}
}

// printOutputsTable prints outputs in table format
func printOutputsTable(outputs []config.Output) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	//nolint:errcheck
	fmt.Fprintln(w, "PATH\tTYPE\tNAMING SCHEME\tIS DIRECTORY")
	//nolint:errcheck
	fmt.Fprintln(w, "----\t----\t-------------\t------------")

	for _, output := range outputs {
		path := output.GetPath()
		outputType := output.Type
		if outputType == "" {
			outputType = "-"
		}
		namingScheme := output.NamingScheme
		if namingScheme == "" {
			namingScheme = "-"
		}
		isDir := "No"
		if output.IsDirectory() {
			isDir = "Yes"
		}
		//nolint:errcheck
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", path, outputType, namingScheme, isDir)
	}
	//nolint:errcheck,gosec
	w.Flush()
}

// Helper functions
func truncateString(s string, maxLen int) string {
	// Remove newlines and replace with spaces
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)

	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func indentContent(content, indent string) string {
	lines := strings.Split(content, "\n")
	var indentedLines []string
	for _, line := range lines {
		indentedLines = append(indentedLines, indent+line)
	}
	return strings.Join(indentedLines, "\n")
}
