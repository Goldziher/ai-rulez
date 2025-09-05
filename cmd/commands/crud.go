package commands

import (
	"github.com/spf13/cobra"
)

var AddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add configuration elements (rules, sections, outputs, agents)",
	Long:  `Add new configuration elements to your ai-rulez configuration file.`,
}

var UpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update configuration elements (rules, sections, outputs, agents)",
	Long:  `Update existing configuration elements in your ai-rulez configuration file.`,
}

var DeleteCmd = &cobra.Command{
	Use:     "delete",
	Short:   "Delete configuration elements (rules, sections, outputs, agents)",
	Long:    `Delete configuration elements from your ai-rulez configuration file.`,
	Aliases: []string{"del", "remove", "rm"},
}

var GetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get configuration elements (rules, sections, outputs, agents)",
	Long:  `Retrieve and display information about specific configuration elements from your ai-rulez configuration file.`,
}

var SetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set top-level configuration properties (metadata, extends, includes)",
	Long:  `Set or update top-level singleton properties in your ai-rulez configuration file.`,
}
