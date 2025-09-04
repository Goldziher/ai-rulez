package crud

import (
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/Goldziher/ai-rulez/internal/utils"
	"github.com/spf13/cobra"
)

var (
	cmdID           string
	cmdName         string
	cmdDescription  string
	cmdAliases      []string
	cmdUsage        string
	cmdSystemPrompt string
	cmdShortcut     string
	cmdEnabled      bool
	cmdTargets      []string
)

// AddCommandCmd represents the command to add a new custom command
var AddCommandCmd = &cobra.Command{
	Use:   "command [name]",
	Short: "Add a new custom command to the configuration",
	Long:  `Adds a new custom slash command to the commands list in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cmdName = args[0]

		newCmd := &config.Command{
			ID:           cmdID,
			Name:         cmdName,
			Description:  cmdDescription,
			Aliases:      cmdAliases,
			Usage:        cmdUsage,
			SystemPrompt: cmdSystemPrompt,
			Shortcut:     cmdShortcut,
			Enabled:      &cmdEnabled,
			Targets:      cmdTargets,
		}

		crud.AddElement("commands", newCmd)
	},
}

func init() {
	AddCommandCmd.Flags().StringVar(&cmdID, "id", "", "Optional unique identifier for the command")
	AddCommandCmd.Flags().StringVarP(&cmdDescription, "description", "d", "", "Description of what the command does (required)")
	utils.LogIfErr(AddCommandCmd.MarkFlagRequired("description"))
	AddCommandCmd.Flags().StringSliceVarP(&cmdAliases, "alias", "a", []string{}, "Alternative name for the command (can be specified multiple times)")
	AddCommandCmd.Flags().StringVarP(&cmdUsage, "usage", "u", "", "Usage example or pattern (e.g., '/newtask <description>')")
	AddCommandCmd.Flags().StringVarP(&cmdSystemPrompt, "system-prompt", "s", "", "System prompt or instructions for the AI when this command is used")
	AddCommandCmd.Flags().StringVar(&cmdShortcut, "shortcut", "", "Keyboard shortcut for the command")
	AddCommandCmd.Flags().BoolVar(&cmdEnabled, "enabled", true, "Whether this command is enabled")
	AddCommandCmd.Flags().StringSliceVar(&cmdTargets, "target", []string{}, "Output target for this command (can be specified multiple times)")
}
