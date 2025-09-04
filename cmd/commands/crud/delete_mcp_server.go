package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

var DeleteMCPServerCmd = &cobra.Command{
	Use:     "mcp-server [name]",
	Short:   "Delete an MCP server from the configuration",
	Long:    `Deletes a Model Context Protocol (MCP) server from the mcp_servers list in your ai_rulez.yaml file.`,
	Aliases: []string{"mcp"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		crud.DeleteElement("mcp_servers", name)
	},
}
