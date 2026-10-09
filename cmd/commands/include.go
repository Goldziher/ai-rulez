package commands

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	incl "github.com/Goldziher/ai-rulez/v5/internal/includes"
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
	includeLocal      bool
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
	RunE: runIncludeAdd,
}

var includeRemoveCmd = &cobra.Command{
	Use:   cmdUseRemoveName,
	Short: "Remove an include source",
	Long: `Remove an include source from the configuration.

Use --yes to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	RunE: runIncludeRemove,
}

var includeListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List all includes",
	Long:  `List all configured include sources.` + localListHint,
	Args:  cobra.NoArgs,
	RunE:  runIncludeList,
}

func init() {
	IncludeCmd.AddCommand(includeAddCmd)
	IncludeCmd.AddCommand(includeRemoveCmd)
	IncludeCmd.AddCommand(includeListCmd)

	includeAddCmd.Flags().BoolVar(&includeLocal, "local", false, localFlagUsage)
	includeRemoveCmd.Flags().BoolVar(&includeLocal, "local", false, localFlagUsage)

	// Add flags for include add
	includeAddCmd.Flags().StringVarP(&includePath, "path", "p", "", "Subdirectory within git repository (git only)")
	includeAddCmd.Flags().StringVarP(&includeRef, "ref", "r", "", "Branch, tag, or commit to use (git only)")
	includeAddCmd.Flags().StringVarP(&includeTypes, "include", "i", "rules,context,skills", "Content types to include (comma-separated)")
	includeAddCmd.Flags().StringVarP(&includeMergeStrat, "merge-strategy", "m", "", "Merge strategy: local-override (default), include-override, or error")
	includeAddCmd.Flags().StringVarP(&includeInstallTo, "install-to", "t", "", "Installation path (optional)")

	addResultFormat(includeAddCmd.Flags())
	addResultFormat(includeRemoveCmd.Flags())

	// Add flags for include remove
	addYesFlag(includeRemoveCmd.Flags(), &includeForce, "Skip confirmation prompts")

	// Add flags for include list
	addJSONFormat(includeListCmd.Flags(), &includeJSON, "j")
}

func runIncludeAdd(cmd *cobra.Command, args []string) error {
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
		includeList = []string{crud.ContentTypeRules, crud.ContentTypeContext, crud.ContentTypeSkills}
	}

	out := outFor(cmd)
	ctx := cmdContext()
	op, err := newContentOperator(includeLocal)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
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
		return failMsg("Failed to add include", err)
	}

	return reportChange(out, out.JSON(), changeResult{
		Status: statusCreated, Type: "include", Name: name, Source: incl.RedactURL(source),
		Path: op.ConfigFile(), Local: includeLocal,
	}, false)
}

func runIncludeRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := confirmRemovalUnlessYes(includeForce, "include", name, "Operation canceled"); err != nil {
		return fail(err)
	}

	out := outFor(cmd)
	ctx := cmdContext()
	op, err := newContentOperator(includeLocal)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	if err := op.RemoveInclude(ctx, name); err != nil {
		return failMsg("Failed to remove include", err)
	}

	return reportChange(out, out.JSON(), changeResult{
		Status: statusRemoved, Type: "include", Name: name, Path: op.ConfigFile(), Local: includeLocal,
	}, false)
}

func runIncludeList(cmd *cobra.Command, _ []string) error {
	out := outFor(cmd)
	ctx := cmdContext()
	op, err := newContentOperator(false)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	includes, err := op.ListIncludes(ctx)
	if err != nil {
		return failMsg("Failed to list includes", err)
	}

	if includeJSON {
		output := make([]map[string]interface{}, len(includes))
		for i, inc := range includes {
			output[i] = map[string]interface{}{
				keyName:   inc.Name,
				keySource: incl.RedactURL(inc.Source),
				keyType:   inc.Type,
			}
		}
		return writeListJSON(out.Stdout(), output)
	}
	if len(includes) == 0 {
		out.Info("No includes found\n")
		logLocalEntriesHint("includes")
		return nil
	}
	out.Result("Includes:\n")
	for _, inc := range includes {
		out.Result("  • %s [%s]\n", inc.Name, inc.Type)
		out.Result("    Source: %s\n", incl.RedactURL(inc.Source))
	}
	logLocalEntriesHint("includes")
	return nil
}
