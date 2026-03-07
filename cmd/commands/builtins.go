package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/internal/builtins"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var builtinsJSON bool

var BuiltinsCmd = &cobra.Command{
	Use:   "builtins",
	Short: "Manage built-in domains",
	Long:  `Manage built-in domains that ship with ai-rulez.`,
}

var builtinsListCmd = &cobra.Command{
	Use:   "list",
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

func init() {
	BuiltinsCmd.AddCommand(builtinsListCmd)
	builtinsListCmd.Flags().BoolVar(&builtinsJSON, "json", false, "Output as JSON")
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
			"name":         d.Name,
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
