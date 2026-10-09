package commands

import (
	"io"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/builtins"
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/samber/oops"
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
	RunE: runBuiltinsList,
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
	addJSONFormat(builtinsListCmd.Flags(), &builtinsJSON, "j")
	addJSONFormat(builtinsShowCmd.Flags(), &builtinsShowJSON, "j")
}

func runBuiltinsList(cmd *cobra.Command, _ []string) error {
	domains := builtins.List()
	w := outFor(cmd).Stdout()

	if builtinsJSON {
		return outputBuiltinsJSON(w, domains)
	}
	outputBuiltinsTable(w, domains)
	return nil
}

func outputBuiltinsJSON(w io.Writer, domains []builtins.BuiltinDomain) error {
	output := make([]map[string]interface{}, len(domains))
	for i, d := range domains {
		output[i] = map[string]interface{}{
			keyName:        d.Name,
			"category":     string(d.Category),
			"auto_include": d.AutoInclude,
			"description":  d.Description,
		}
	}
	data, err := jsondoc.Marshal(output)
	if err != nil {
		return failMsg("Failed to marshal JSON", err)
	}
	_, err = w.Write(data)
	return err //nolint:wrapcheck // a write failure
}

func outputBuiltinsTable(w io.Writer, domains []builtins.BuiltinDomain) {
	var currentCategory builtins.Category

	for _, d := range domains {
		if d.Category != currentCategory {
			currentCategory = d.Category
			writef(w, "\n%s:\n", categoryLabel(currentCategory))
		}

		autoTag := ""
		if d.AutoInclude {
			autoTag = " (auto-included)"
		}
		writef(w, "  %-16s %s%s\n", d.Name, d.Description, autoTag)
	}
	writeln(w)
}

func runBuiltinsShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	if !builtins.IsValid(name) {
		return fail(oops.Hint("Run 'ai-rulez builtins list' to see the built-in domains.").Errorf("unknown builtin domain: %s", name))
	}

	entries, err := builtins.LoadDomainContent(name)
	if err != nil {
		return failMsg("failed to load builtin domain content", err)
	}

	if builtinsShowJSON {
		return outputShowJSON(outFor(cmd).Stdout(), name, entries)
	}

	outputShowFormatted(outFor(cmd).Stdout(), name, entries)
	return nil
}

func outputShowJSON(w io.Writer, name string, entries []builtins.ContentEntry) error {
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

	data, err := jsondoc.Marshal(output)
	if err != nil {
		return failMsg("Failed to marshal JSON", err)
	}
	_, err = w.Write(data)
	return err //nolint:wrapcheck // a write failure
}

func outputShowFormatted(w io.Writer, name string, entries []builtins.ContentEntry) {
	domain, _ := builtins.Get(name)

	writef(w, "Domain: %s\n", name)
	writef(w, "Category: %s\n", domain.Category)

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
		writef(w, "\n%s:\n", titleCase(ct))
		for _, item := range items {
			if item.Priority != "" {
				writef(w, "  %s (priority: %s)\n", item.Name, item.Priority)
			} else {
				writef(w, "  %s\n", item.Name)
			}
			// Print first non-empty, non-frontmatter line as summary
			summary := extractSummary(item.Content)
			if summary != "" {
				writef(w, "    %s\n", summary)
			}
		}
	}
	writeln(w)
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
