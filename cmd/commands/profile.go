package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/spf13/cobra"
)

var (
	profileSetDefault bool
	profileForce      bool
	profileJSON       bool
	profileLocal      bool
)

var ProfileCmd = &cobra.Command{
	Use:   "profile",
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

Use --set-default to make this profile the default for generation.`,
	Args: cobra.MinimumNArgs(2),
	Run:  runProfileAdd,
}

var profileRemoveCmd = &cobra.Command{
	Use:   cmdUseRemoveName,
	Short: "Remove a profile",
	Long: `Remove a profile from the configuration.

Use --yes to skip confirmation prompts.
Note: Cannot remove the default profile.`,
	Args: cobra.ExactArgs(1),
	Run:  runProfileRemove,
}

var profileSetDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set the default profile",
	Long:  `Set which profile should be used as the default during generation.`,
	Args:  cobra.ExactArgs(1),
	Run:   runProfileSetDefault,
}

var profileListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List all profiles",
	Long:  `List all configured profiles and show which is default.` + localListHint,
	Args:  cobra.NoArgs,
	Run:   runProfileList,
}

func init() {
	ProfileCmd.AddCommand(profileAddCmd)
	ProfileCmd.AddCommand(profileRemoveCmd)
	ProfileCmd.AddCommand(profileSetDefaultCmd)
	ProfileCmd.AddCommand(profileListCmd)

	for _, c := range []*cobra.Command{profileAddCmd, profileRemoveCmd, profileSetDefaultCmd} {
		c.Flags().BoolVar(&profileLocal, "local", false, localFlagUsage)
	}

	// Add flags for profile add
	profileAddCmd.Flags().BoolVarP(&profileSetDefault, "set-default", "s", false, "Set this profile as the default")

	// Add flags for profile remove
	profileRemoveCmd.Flags().BoolVarP(&profileForce, "yes", "y", false, "Skip confirmation prompts")

	// Add flags for profile list
	addJSONFormat(profileListCmd.Flags(), &profileJSON, "j")
}

func runProfileAdd(cmd *cobra.Command, args []string) {
	name := args[0]
	domains := args[1:]

	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}
	if profileLocal {
		op = op.Local()
	}

	if err := op.AddProfile(ctx, name, domains); err != nil {
		logger.Error("Failed to add profile", "error", err)
		os.Exit(1)
	}

	logger.Info("Profile added successfully",
		"name", name,
		"domains", strings.Join(domains, ", "),
	)

	// Set as default if requested
	if profileSetDefault {
		if err := op.SetDefaultProfile(ctx, name); err != nil {
			logger.Error("Failed to set default profile", "error", err)
			os.Exit(1)
		}
		logger.Info("Profile set as default", "name", name)
	}
}

func runProfileRemove(cmd *cobra.Command, args []string) {
	name := args[0]

	// Confirm removal unless --yes is specified
	if !profileForce {
		if !confirmRemoval("profile", name) {
			logger.Info("Operation canceled")
			return
		}
	}

	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}
	if profileLocal {
		op = op.Local()
	}

	if err := op.RemoveProfile(ctx, name); err != nil {
		logger.Error("Failed to remove profile", "error", err)
		os.Exit(1)
	}

	logger.Info("Profile removed successfully", "name", name)
}

func runProfileSetDefault(cmd *cobra.Command, args []string) {
	name := args[0]

	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}
	if profileLocal {
		op = op.Local()
	}

	if err := op.SetDefaultProfile(ctx, name); err != nil {
		logger.Error("Failed to set default profile", "error", err)
		os.Exit(1)
	}

	logger.Info("Default profile set successfully", "name", name)
}

func runProfileList(cmd *cobra.Command, args []string) {
	ctx := cmdContext()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	profiles, err := op.ListProfiles(ctx)
	if err != nil {
		logger.Error("Failed to list profiles", "error", err)
		os.Exit(1)
	}

	if len(profiles) == 0 && !profileJSON {
		logger.Info("No profiles found")
		logLocalEntriesHint("profiles")
		return
	}

	if profileJSON {
		// Output as JSON
		output := make([]map[string]interface{}, len(profiles))
		for i, profile := range profiles {
			output[i] = map[string]interface{}{
				keyName:      profile.Name,
				"domains":    profile.Domains,
				"is_default": profile.IsDefault,
			}
		}
		data, err := jsondoc.Marshal(output)
		if err != nil {
			logger.Error("Failed to marshal JSON", "error", err)
			os.Exit(1)
		}
		fmt.Print(string(data))
	} else {
		// Output as human-readable table
		logger.Info("Profiles:")
		for _, profile := range profiles {
			marker := ""
			if profile.IsDefault {
				marker = " (default)"
			}
			logger.Info(fmt.Sprintf("  • %s%s", profile.Name, marker))
			logger.Debug(fmt.Sprintf("    Domains: %v", profile.Domains))
		}
		logLocalEntriesHint("profiles")
	}
}
