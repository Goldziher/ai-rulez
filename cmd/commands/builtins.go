package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/builtins"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var builtinsJSON bool
var builtinsShowJSON bool

var BuiltinsCmd = &cobra.Command{
	Use:   "builtins",
	Short: "Manage built-in domains",
	Long:  `Manage built-in domains that ship with ai-rulez.`,
}

var builtinsListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List all available built-in domains",
	Long: `List all available built-in domains that can be enabled via the 'builtins' config field.

Categories:
  universal  — Language-agnostic governance (security, git, code quality, etc.)
  language   — Per-language conventions (rust, python, typescript, etc.)
  binding    — FFI binding conventions (pyo3, napi-rs, magnus, etc.)

ai-governance is auto-included unless explicitly excluded with "!ai-governance".`,
	Args: cobra.NoArgs,
	Run:  runBuiltinsList,
}

var builtinsShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show the full content of a built-in domain",
	Long:  `Show all rules, context, skills, agents, and commands for a built-in domain.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runBuiltinsShow,
}

func init() {
	BuiltinsCmd.AddCommand(builtinsListCmd)
	BuiltinsCmd.AddCommand(builtinsShowCmd)
	builtinsListCmd.Flags().BoolVarP(&builtinsJSON, "json", "j", false, "Output as JSON")
	builtinsShowCmd.Flags().BoolVarP(&builtinsShowJSON, "json", "j", false, "Output as JSON")
}

func runBuiltinsList(cmd *cobra.Command, args []string) {
	domains := builtins.List()

	if builtinsJSON {
		outputBuiltinsJSON(domains)
	} else {
		outputBuiltinsTable(domains)
	}
}

func outputBuiltinsJSON(domains []builtins.BuiltinDomain) {
	output := make([]map[string]interface{}, len(domains))
	for i, d := range domains {
		output[i] = map[string]interface{}{
			keyName:        d.Name,
			"category":     string(d.Category),
			"auto_include": d.AutoInclude,
			"description":  d.Description,
		}
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		logger.Error("Failed to marshal JSON", "error", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func outputBuiltinsTable(domains []builtins.BuiltinDomain) {
	var currentCategory builtins.Category

	for _, d := range domains {
		if d.Category != currentCategory {
			currentCategory = d.Category
			fmt.Printf("\n%s:\n", categoryLabel(currentCategory))
		}

		autoTag := ""
		if d.AutoInclude {
			autoTag = " (auto-included)"
		}
		fmt.Printf("  %-16s %s%s\n", d.Name, d.Description, autoTag)
	}
	fmt.Println()
}

func runBuiltinsShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	if !builtins.IsValid(name) {
		return fmt.Errorf("unknown builtin domain: %s", name)
	}

	entries, err := builtins.LoadDomainContent(name)
	if err != nil {
		return fmt.Errorf("failed to load builtin domain content: %w", err)
	}

	if builtinsShowJSON {
		outputShowJSON(name, entries)
		return nil
	}

	outputShowFormatted(name, entries)
	return nil
}

func outputShowJSON(name string, entries []builtins.ContentEntry) {
	grouped := map[string][]map[string]string{}
	for _, e := range entries {
		entry := map[string]string{
			keyName:   e.Name,
			"content": e.Content,
		}
		if e.Priority != "" {
			entry["priority"] = e.Priority
		}
		grouped[e.Type] = append(grouped[e.Type], entry)
	}

	domain, _ := builtins.Get(name)
	output := map[string]interface{}{
		keyName:    name,
		"category": string(domain.Category),
		"content":  grouped,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		logger.Error("Failed to marshal JSON", "error", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func outputShowFormatted(name string, entries []builtins.ContentEntry) {
	domain, _ := builtins.Get(name)

	fmt.Printf("Domain: %s\n", name)
	fmt.Printf("Category: %s\n", domain.Category)

	// Group entries by type
	grouped := map[string][]builtins.ContentEntry{}
	for _, e := range entries {
		grouped[e.Type] = append(grouped[e.Type], e)
	}

	contentTypes := []string{crud.ContentTypeRules, crud.ContentTypeContext, crud.ContentTypeSkills, "agents", "commands"}
	for _, ct := range contentTypes {
		items, ok := grouped[ct]
		if !ok {
			continue
		}
		fmt.Printf("\n%s:\n", titleCase(ct))
		for _, item := range items {
			if item.Priority != "" {
				fmt.Printf("  %s (priority: %s)\n", item.Name, item.Priority)
			} else {
				fmt.Printf("  %s\n", item.Name)
			}
			// Print first non-empty, non-frontmatter line as summary
			summary := extractSummary(item.Content)
			if summary != "" {
				fmt.Printf("    %s\n", summary)
			}
		}
	}
	fmt.Println()
}

func extractSummary(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip frontmatter delimiters, empty lines, and headings
		if trimmed == "" || trimmed == "---" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Skip frontmatter key-value lines (between --- delimiters)
		if isFrontmatterLine(lines, line) {
			continue
		}
		// Truncate long summaries
		if len(trimmed) > 100 {
			return trimmed[:100] + "..."
		}
		return trimmed
	}
	return ""
}

func isFrontmatterLine(lines []string, line string) bool {
	// Check if we're within frontmatter (between first --- and second ---)
	inFrontmatter := false
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			return false // Past frontmatter
		}
		if inFrontmatter && strings.TrimSpace(l) == strings.TrimSpace(line) {
			return true
		}
	}
	return false
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func categoryLabel(c builtins.Category) string {
	switch c {
	case builtins.CategoryBinding:
		return "Bindings"
	case builtins.CategoryLanguage:
		return "Languages"
	case builtins.CategoryUniversal:
		return "Universal"
	default:
		return string(c)
	}
}
