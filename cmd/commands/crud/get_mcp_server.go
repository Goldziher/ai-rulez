package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

var GetMCPServerCmd = &cobra.Command{
	Use:   "mcp-server [name]",
	Short: "Get details of a specific MCP server",
	Long:  `Retrieves and displays the configuration of a specific MCP server from your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		crud.GetElement("mcp_servers", name)
	},
}
