package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

// UpdateMCPServerCmd represents the command to update an existing MCP server
var UpdateMCPServerCmd = &cobra.Command{
	Use:   "mcp-server [name]",
	Short: "Update an existing MCP server in the configuration",
	Long:  `Updates an existing Model Context Protocol (MCP) server in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		updates := make(map[string]interface{})
		if cmd.Flags().Changed("id") {
			updates["ID"] = mcpID
		}
		if cmd.Flags().Changed("description") {
			updates["Description"] = mcpDescription
		}
		if cmd.Flags().Changed("command") {
			updates["Command"] = mcpCommand
		}
		if cmd.Flags().Changed("arg") {
			updates["Args"] = mcpArgs
		}
		if cmd.Flags().Changed("env") {
			updates["Env"] = crud.ParseEnvVars(mcpEnv)
		}
		if cmd.Flags().Changed("transport") {
			updates["Transport"] = mcpTransport
		}
		if cmd.Flags().Changed("url") {
			updates["URL"] = mcpURL
		}
		if cmd.Flags().Changed("enabled") {
			updates["Enabled"] = mcpEnabled
		}
		if cmd.Flags().Changed("target") {
			updates["Targets"] = mcpTargets
		}

		crud.UpdateElement("mcp_servers", name, updates)
	},
}

func init() {
	UpdateMCPServerCmd.Flags().StringVar(&mcpID, "id", "", "New unique identifier for the MCP server")
	UpdateMCPServerCmd.Flags().StringVarP(&mcpDescription, "description", "d", "", "New description for the MCP server")
	UpdateMCPServerCmd.Flags().StringVarP(&mcpCommand, "command", "c", "", "New command for the MCP server")
	UpdateMCPServerCmd.Flags().StringSliceVarP(&mcpArgs, "arg", "a", []string{}, "New set of command line arguments")
	UpdateMCPServerCmd.Flags().StringSliceVarP(&mcpEnv, "env", "e", []string{}, "New set of environment variables in KEY=VALUE format")
	UpdateMCPServerCmd.Flags().StringVarP(&mcpTransport, "transport", "t", "", "New transport protocol (stdio, sse, http)")
	UpdateMCPServerCmd.Flags().StringVarP(&mcpURL, "url", "u", "", "New URL for the MCP server")
	UpdateMCPServerCmd.Flags().BoolVar(&mcpEnabled, "enabled", true, "Set the enabled state of the MCP server")
	UpdateMCPServerCmd.Flags().StringSliceVar(&mcpTargets, "target", []string{}, "New set of output targets")
}
