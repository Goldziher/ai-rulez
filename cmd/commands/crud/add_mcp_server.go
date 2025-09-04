package crud

import (
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

var (
	mcpID          string
	mcpName        string
	mcpDescription string
	mcpCommand     string
	mcpArgs        []string
	mcpEnv         []string
	mcpTransport   string
	mcpURL         string
	mcpEnabled     bool
	mcpTargets     []string
)

var AddMCPServerCmd = &cobra.Command{
	Use:   "mcp-server [name]",
	Short: "Add a new MCP server to the configuration",
	Long:  `Adds a new Model Context Protocol (MCP) server to the mcp_servers list in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mcpName = args[0]

		newServer := &config.MCPServer{
			ID:          mcpID,
			Name:        mcpName,
			Description: mcpDescription,
			Command:     mcpCommand,
			Args:        mcpArgs,
			Env:         crud.ParseEnvVars(mcpEnv),
			Transport:   mcpTransport,
			URL:         mcpURL,
			Enabled:     &mcpEnabled,
			Targets:     mcpTargets,
		}

		crud.AddElement("mcp_servers", newServer)
	},
}

func init() {
	AddMCPServerCmd.Flags().StringVar(&mcpID, "id", "", "Optional unique identifier for the MCP server")
	AddMCPServerCmd.Flags().StringVarP(&mcpDescription, "description", "d", "", "Description of the MCP server's capabilities")
	AddMCPServerCmd.Flags().StringVarP(&mcpCommand, "command", "c", "", "Command to start the MCP server (e.g., 'npx', 'python')")
	AddMCPServerCmd.Flags().StringSliceVarP(&mcpArgs, "arg", "a", []string{}, "Command line argument for the MCP server (can be specified multiple times)")
	AddMCPServerCmd.Flags().StringSliceVarP(&mcpEnv, "env", "e", []string{}, "Environment variable for the MCP server in KEY=VALUE format (can be specified multiple times)")
	AddMCPServerCmd.Flags().StringVarP(&mcpTransport, "transport", "t", "stdio", "Transport protocol (stdio, sse, http)")
	AddMCPServerCmd.Flags().StringVarP(&mcpURL, "url", "u", "", "URL for remote MCP servers (when transport is sse or http)")
	AddMCPServerCmd.Flags().BoolVar(&mcpEnabled, "enabled", true, "Whether this MCP server is enabled")
	AddMCPServerCmd.Flags().StringSliceVar(&mcpTargets, "target", []string{}, "Output target for this server (can be specified multiple times)")
}
