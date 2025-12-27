package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var (
	includePath       string
	includeRef        string
	includeTypes      string
	includeMergeStrat string
	includeInstallTo  string
	includeForce      bool
	includeJSON       bool
)

var IncludeCmd = &cobra.Command{
	Use:   "include",
	Short: "Manage includes",
	Long:  `Manage include sources in your .ai-rulez/ configuration.`,
}

var includeAddCmd = &cobra.Command{
	Use:   "add <name> <source>",
	Short: "Add an include source",
	Long: `Add a new include source (git repository or local path).

Sources can be:
  - Git URLs: https://github.com/user/repo or git@github.com:user/repo
  - Local paths: /path/to/local/repo or ~/relative/path

For git sources, you can specify:
  - --ref: Branch, tag, or commit to use
  - --path: Subdirectory within the repository

By default, all content types (rules, context, skills) are included.
Use --include to specify which types: rules,context,skills`,
	Args: cobra.ExactArgs(2),
	Run:  runIncludeAdd,
}

var includeRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an include source",
	Long: `Remove an include source from the configuration.

Use --force to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	Run:  runIncludeRemove,
}

var includeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all includes",
	Long:  `List all configured include sources.`,
	Args:  cobra.NoArgs,
	Run:   runIncludeList,
}

func init() {
	IncludeCmd.AddCommand(includeAddCmd)
	IncludeCmd.AddCommand(includeRemoveCmd)
	IncludeCmd.AddCommand(includeListCmd)

	// Add flags for include add
	includeAddCmd.Flags().StringVar(&includePath, "path", "", "Subdirectory within git repository (git only)")
	includeAddCmd.Flags().StringVar(&includeRef, "ref", "", "Branch, tag, or commit to use (git only)")
	includeAddCmd.Flags().StringVar(&includeTypes, "include", "rules,context,skills", "Content types to include (comma-separated)")
	includeAddCmd.Flags().StringVar(&includeMergeStrat, "merge-strategy", "default", "Merge strategy: default|override|append")
	includeAddCmd.Flags().StringVar(&includeInstallTo, "install-to", "", "Installation path (optional)")

	// Add flags for include remove
	includeRemoveCmd.Flags().BoolVar(&includeForce, "force", false, "Skip confirmation prompts")

	// Add flags for include list
	includeListCmd.Flags().BoolVar(&includeJSON, "json", false, "Output as JSON")
}

func runIncludeAdd(cmd *cobra.Command, args []string) {
	name := args[0]
	source := args[1]

	// Parse include types
	var includeList []string
	for _, t := range strings.Split(includeTypes, ",") {
		if t = strings.TrimSpace(t); t != "" {
			includeList = append(includeList, t)
		}
	}
	if len(includeList) == 0 {
		includeList = []string{"rules", "context", "skills"}
	}

	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	req := &crud.AddIncludeRequest{
		Name:          name,
		Source:        source,
		Path:          includePath,
		Ref:           includeRef,
		Include:       includeList,
		MergeStrategy: includeMergeStrat,
		InstallTo:     includeInstallTo,
	}

	if err := op.AddInclude(ctx, req); err != nil {
		logger.Error("Failed to add include", "error", err)
		os.Exit(1)
	}

	logger.Info("Include added successfully",
		"name", name,
		"source", source,
	)
}

func runIncludeRemove(cmd *cobra.Command, args []string) {
	name := args[0]

	// Confirm removal unless --force is specified
	if !includeForce {
		if !confirmRemoval("include", name) {
			logger.Info("Operation canceled")
			return
		}
	}

	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	if err := op.RemoveInclude(ctx, name); err != nil {
		logger.Error("Failed to remove include", "error", err)
		os.Exit(1)
	}

	logger.Info("Include removed successfully", "name", name)
}

func runIncludeList(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	includes, err := op.ListIncludes(ctx)
	if err != nil {
		logger.Error("Failed to list includes", "error", err)
		os.Exit(1)
	}

	if len(includes) == 0 {
		logger.Info("No includes found")
		return
	}

	if includeJSON {
		// Output as JSON
		output := make([]map[string]interface{}, len(includes))
		for i, inc := range includes {
			output[i] = map[string]interface{}{
				"name":   inc.Name,
				"source": inc.Source,
				"type":   inc.Type,
			}
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			logger.Error("Failed to marshal JSON", "error", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		// Output as human-readable table
		logger.Info("Includes:")
		for _, inc := range includes {
			sourceInfo := fmt.Sprintf("[%s]", inc.Type)
			logger.Info(fmt.Sprintf("  • %s %s", inc.Name, sourceInfo))
			logger.Debug(fmt.Sprintf("    Source: %s", inc.Source))
		}
	}
}
