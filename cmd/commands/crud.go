package commands

import (
	"github.com/spf13/cobra"
)

// AddCmd represents the add command group
var AddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add configuration elements (rules, sections, outputs, agents)",
	Long:  `Add new configuration elements to your ai-rulez configuration file.`,
}

// UpdateCmd represents the update command group
var UpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update configuration elements (rules, sections, outputs, agents)",
	Long:  `Update existing configuration elements in your ai-rulez configuration file.`,
}

// DeleteCmd represents the delete command group
var DeleteCmd = &cobra.Command{
	Use:     "delete",
	Short:   "Delete configuration elements (rules, sections, outputs, agents)",
	Long:    `Delete configuration elements from your ai-rulez configuration file.`,
	Aliases: []string{"del", "remove", "rm"},
}

// GetCmd represents the get command group
var GetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get configuration elements (rules, sections, outputs, agents)",
	Long:  `Retrieve and display information about specific configuration elements from your ai-rulez configuration file.`,
}
