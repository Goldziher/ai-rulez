package commands

import (
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/spf13/cobra"
)

var (
	profileSetDefault bool
	profileForce      bool
	profileJSON       bool
	profileLocal      bool
)

var ProfileCmd = &cobra.Command{
	Use:   flagServeProfile,
	Short: "Manage profiles",
	Long:  `Manage profiles in your .ai-rulez/ configuration.`,
}

var profileAddCmd = &cobra.Command{
	Use:   "add <name> <domain...>",
	Short: "Add a new profile",
	Long: `Add a new profile with one or more domains.

Profiles are named collections of domains that can be used during generation.
For example: ai-rulez profile add backend backend-services database
            ai-rulez profile add frontend frontend-ui frontend-api

Use --set-default to make this profile the default for generation. --format json
prints a document with the name, domains and the config file that was changed.`,
	Args: cobra.MinimumNArgs(2),
	RunE: runProfileAdd,
}

var profileRemoveCmd = &cobra.Command{
	Use:   cmdUseRemoveName,
	Short: "Remove a profile",
	Long: `Remove a profile from the configuration.

Use --yes to skip confirmation prompts.
Note: Cannot remove the default profile.`,
	Args: cobra.ExactArgs(1),
	RunE: runProfileRemove,
}

var profileSetDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set the default profile",
	Long:  `Set which profile should be used as the default during generation.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileSetDefault,
}

var profileListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List all profiles",
	Long:  `List all configured profiles and show which is default.` + localListHint,
	Args:  cobra.NoArgs,
	RunE:  runProfileList,
}

func init() {
	ProfileCmd.AddCommand(profileAddCmd)
	ProfileCmd.AddCommand(profileRemoveCmd)
	ProfileCmd.AddCommand(profileSetDefaultCmd)
	ProfileCmd.AddCommand(profileListCmd)

	for _, c := range []*cobra.Command{profileAddCmd, profileRemoveCmd, profileSetDefaultCmd} {
		specLocal.Bool(c.Flags(), &profileLocal, localFlagUsage)
	}

	for _, c := range []*cobra.Command{profileAddCmd, profileRemoveCmd, profileSetDefaultCmd} {
		addResultFormat(c.Flags())
	}

	// Add flags for profile add
	profileAddCmd.Flags().BoolVar(&profileSetDefault, "set-default", false, "Set this profile as the default")

	// Add flags for profile remove
	addYesFlag(profileRemoveCmd.Flags(), &profileForce, "Skip confirmation prompts")

	// Add flags for profile list
	addJSONFormat(profileListCmd.Flags(), &profileJSON, "j")
}

// profileOperator opens the operator for the profile commands: the shared
// config, or the machine-local overlay with --local.
func profileOperator() (*crud.OperatorImpl, error) {
	op, err := newContentOperator(false)
	if err != nil {
		return nil, failMsg("Failed to create CRUD operator", err)
	}
	if profileLocal {
		op = op.Local()
	}
	return op, nil
}

func runProfileAdd(cmd *cobra.Command, args []string) error {
	out := outFor(cmd)
	name := args[0]
	domains := args[1:]

	ctx := cmdContext()
	op, err := profileOperator()
	if err != nil {
		return err
	}

	if err := op.AddProfile(ctx, name, domains); err != nil {
		return failMsg("Failed to add profile", err)
	}

	// Set as default if requested
	if profileSetDefault {
		if err := op.SetDefaultProfile(ctx, name); err != nil {
			return failMsg("Failed to set default profile", err)
		}
	}
	return reportChange(out, out.JSON(), changeResult{
		Status: statusCreated, Type: flagServeProfile, Name: name, Domains: domains,
		Default: profileSetDefault, Path: op.ConfigFile(), Local: profileLocal,
	}, false)
}

func runProfileRemove(cmd *cobra.Command, args []string) error {
	out := outFor(cmd)
	name := args[0]

	if err := confirmRemovalUnlessYes(profileForce, "profile", name, "Operation canceled"); err != nil {
		return fail(err)
	}

	op, err := profileOperator()
	if err != nil {
		return err
	}
	if err := op.RemoveProfile(cmdContext(), name); err != nil {
		return failMsg("Failed to remove profile", err)
	}
	return reportChange(out, out.JSON(), changeResult{
		Status: statusRemoved, Type: flagServeProfile, Name: name, Path: op.ConfigFile(), Local: profileLocal,
	}, false)
}

func runProfileSetDefault(cmd *cobra.Command, args []string) error {
	out := outFor(cmd)
	name := args[0]

	op, err := profileOperator()
	if err != nil {
		return err
	}
	if err := op.SetDefaultProfile(cmdContext(), name); err != nil {
		return failMsg("Failed to set default profile", err)
	}
	return reportChange(out, out.JSON(), changeResult{
		Status: statusUpdated, Type: flagServeProfile, Name: name, Default: true, Path: op.ConfigFile(), Local: profileLocal,
	}, false)
}

func runProfileList(cmd *cobra.Command, _ []string) error {
	out := outFor(cmd)
	ctx := cmdContext()
	op, err := newContentOperator(false)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	profiles, err := op.ListProfiles(ctx)
	if err != nil {
		return failMsg("Failed to list profiles", err)
	}

	if profileJSON {
		output := make([]map[string]interface{}, len(profiles))
		for i, profile := range profiles {
			output[i] = map[string]interface{}{
				keyName:      profile.Name,
				"domains":    profile.Domains,
				"is_default": profile.IsDefault,
			}
		}
		return writeListJSON(out.Stdout(), output)
	}
	if len(profiles) == 0 {
		out.Info("No profiles found\n")
		logLocalEntriesHint("profiles")
		return nil
	}
	out.Result("Profiles:\n")
	for _, profile := range profiles {
		marker := ""
		if profile.IsDefault {
			marker = " (default)"
		}
		out.Result("  • %s%s\n", profile.Name, marker)
		out.Result("    Domains: %v\n", profile.Domains)
	}
	logLocalEntriesHint("profiles")
	return nil
}
