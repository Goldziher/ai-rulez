package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

var UpdateCommandCmd = &cobra.Command{
	Use:   "command [name]",
	Short: "Update an existing custom command in the configuration",
	Long:  `Updates an existing custom slash command in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		updates := make(map[string]interface{})
		if cmd.Flags().Changed("id") {
			updates["ID"] = cmdID
		}
		if cmd.Flags().Changed("description") {
			updates["Description"] = cmdDescription
		}
		if cmd.Flags().Changed("alias") {
			updates["Aliases"] = cmdAliases
		}
		if cmd.Flags().Changed("usage") {
			updates["Usage"] = cmdUsage
		}
		if cmd.Flags().Changed("system-prompt") {
			updates["SystemPrompt"] = cmdSystemPrompt
		}
		if cmd.Flags().Changed("shortcut") {
			updates["Shortcut"] = cmdShortcut
		}
		if cmd.Flags().Changed("enabled") {
			updates["Enabled"] = cmdEnabled
		}
		if cmd.Flags().Changed("target") {
			updates["Targets"] = cmdTargets
		}

		crud.UpdateElement("commands", name, updates)
	},
}

func init() {
	UpdateCommandCmd.Flags().StringVar(&cmdID, "id", "", "New unique identifier for the command")
	UpdateCommandCmd.Flags().StringVarP(&cmdDescription, "description", "d", "", "New description for the command")
	UpdateCommandCmd.Flags().StringSliceVarP(&cmdAliases, "alias", "a", []string{}, "New set of alternative names for the command")
	UpdateCommandCmd.Flags().StringVarP(&cmdUsage, "usage", "u", "", "New usage example or pattern")
	UpdateCommandCmd.Flags().StringVarP(&cmdSystemPrompt, "system-prompt", "s", "", "New system prompt for the command")
	UpdateCommandCmd.Flags().StringVar(&cmdShortcut, "shortcut", "", "New keyboard shortcut for the command")
	UpdateCommandCmd.Flags().BoolVar(&cmdEnabled, "enabled", true, "Set the enabled state of the command")
	UpdateCommandCmd.Flags().StringSliceVar(&cmdTargets, "target", []string{}, "New set of output targets for the command")
}
