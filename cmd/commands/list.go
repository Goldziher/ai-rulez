package commands

import (
	"github.com/spf13/cobra"
)

var ListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configuration elements (rules, sections, outputs, agents)",
	Long:  `List and inspect configuration elements from your ai-rulez configuration file.`,
}
