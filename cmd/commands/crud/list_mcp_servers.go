package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

// ListMCPServersCmd represents the command to list all MCP servers
var ListMCPServersCmd = &cobra.Command{
	Use:     "mcp-servers",
	Short:   "List all configured MCP servers",
	Long:    `Lists all the Model Context Protocol (MCP) servers currently defined in your ai_rulez.yaml file.`,
	Aliases: []string{"mcp-server"},
	Run: func(cmd *cobra.Command, args []string) {
		crud.ListElements("mcp_servers")
	},
}
