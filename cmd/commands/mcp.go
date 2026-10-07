package commands

import (
	"context"
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// flagServeDomain names the --domain flag of the skills server.
const flagServeDomain = "domain"

var MCPCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start Model Context Protocol (MCP) server",
	Long: `Start an MCP server that allows AI assistants to interact with
ai-rulez configuration dynamically. The server provides tools for reading,
creating, updating, and deleting configuration elements.

The MCP server communicates via stdin/stdout and is designed to be integrated
with AI assistants that support the Model Context Protocol.

With --serve-skills the server instead becomes a read-only skills server
(MCP Skills extension, SEP-2640): it serves the skills of one profile as
skill:// resources, answers skills/list and skills/get, and adds search_skills,
get_skill and read_skill_file tools. The authoring tools are not registered in
that mode, so it is safe to hand to an unattended agent.`,
	Run: runMCPServer,
}

func runMCPServer(cmd *cobra.Command, args []string) {
	ctx := config.WithPolicyContext(context.Background(), activePolicy)
	serveOnly := append([]string{"profile", "targets", flagServeDomain, "allow", "deny"}, dynamicServeFlagNames...)
	if serve, _ := cmd.Flags().GetBool("serve-skills"); !serve {
		for _, name := range serveOnly {
			if cmd.Flags().Changed(name) {
				fmtError(oops.Errorf("--%s requires --serve-skills", name))
				os.Exit(1)
			}
		}
	}
	var (
		srv       *mcp.Server
		transport sdkmcp.Transport = mcp.NewGuardedStdioTransport(os.Stdin, os.Stdout, nil)
	)
	if serve, _ := cmd.Flags().GetBool("serve-skills"); serve {
		var err error
		srv, err = buildSkillServer(ctx, cmd)
		if err != nil {
			fmtError(oops.Wrapf(err, "MCP: build skills server"))
			os.Exit(1)
		}
		transport = srv.WrapTransport(transport)
	} else {
		srv = mcp.NewServer(Version)
	}

	closeTelemetry := wireMCPTelemetry(srv)
	err := srv.GetMCPServer().Run(ctx, transport)
	closeTelemetry()
	srv.Close()
	if err != nil {
		fmtError(oops.Wrapf(err, "MCP: start MCP server"))
		os.Exit(1)
	}
}

// buildSkillServer builds the read-only serving server; see mcp_serve.go.
func buildSkillServer(ctx context.Context, cmd *cobra.Command) (*mcp.Server, error) {
	return buildDynamicSkillServer(ctx, cmd)
}

func init() {
	MCPCmd.Flags().Bool("serve-skills", false, "Serve skills read-only over the MCP Skills extension instead of the authoring tools")
	MCPCmd.Flags().String("profile", "", "Profile whose skills to serve (default: the configured default profile; requires --serve-skills)")
	MCPCmd.Flags().String("targets", "", "Preset whose rendering of the skills to serve (default: first configured preset with skills; requires --serve-skills)")
	MCPCmd.Flags().StringSlice(flagServeDomain, nil, "Only serve skills of these domains; 'root' selects skills in no domain (requires --serve-skills)")
	MCPCmd.Flags().StringSlice("allow", nil, "Only serve skills whose name matches one of these glob patterns (requires --serve-skills)")
	MCPCmd.Flags().StringSlice("deny", nil, "Never serve skills whose name matches one of these glob patterns; wins over --allow (requires --serve-skills)")
	registerDynamicServeFlags(MCPCmd)
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
