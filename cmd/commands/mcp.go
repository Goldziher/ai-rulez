package commands

import (
	"context"
	"os"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var MCPCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start Model Context Protocol (MCP) server",
	Long: `Start an MCP server that allows AI assistants to interact with 
ai-rulez configuration dynamically. The server provides tools for reading, 
creating, updating, and deleting configuration elements.

The MCP server communicates via stdin/stdout and is designed to be integrated
with AI assistants that support the Model Context Protocol.`,
	Run: runMCPServer,
}

func runMCPServer(cmd *cobra.Command, args []string) {
	srv := mcp.NewServer(Version)

	if err := srv.GetMCPServer().Run(context.Background(), &sdkmcp.StdioTransport{}); err != nil {
		fmtError(oops.Wrapf(err, "MCP: start MCP server"))
		os.Exit(1)
	}
}

func init() {
	MCPCmd.Flags().String("transport", "stdio", "Transport method (stdio, websocket)")
	MCPCmd.Flags().String("address", "", "Address to bind to (for websocket transport)")
	MCPCmd.Flags().Int("port", 3000, "Port to bind to (for websocket transport)")

	if err := MCPCmd.Flags().MarkHidden("transport"); err != nil {
		logger.Debug("Failed to hide transport flag", "error", err)
	}
	if err := MCPCmd.Flags().MarkHidden("address"); err != nil {
		logger.Debug("Failed to hide address flag", "error", err)
	}
	if err := MCPCmd.Flags().MarkHidden("port"); err != nil {
		logger.Debug("Failed to hide port flag", "error", err)
	}
}
