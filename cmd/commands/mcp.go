package commands

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// flagServeDomain names the --domain flag of the skills server.
const flagServeDomain = "domain"

// Flags that set which directories the authoring tools may use.
const (
	flagMCPRoot     = "root"
	flagAllowAnyDir = "allow-any-dir"
)

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
	Args: cobra.NoArgs,
	RunE: runMCPServer,
}

func runMCPServer(cmd *cobra.Command, _ []string) error {
	return fail(runMCP(cmd))
}

// runMCP runs the MCP server until the client disconnects or a signal ends it.
func runMCP(cmd *cobra.Command) error {
	// SIGTERM and Ctrl-C end the server through its context, so shutdown still
	// flushes the usage sink and telemetry instead of dropping queued records.
	ctx, stop := signal.NotifyContext(cmdContext(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serve, _ := cmd.Flags().GetBool("serve-skills") //nolint:errcheck // the flag is registered in init
	if !serve {
		serveOnly := append([]string{"profile", "targets", flagServeDomain, "allow", "deny"}, dynamicServeFlagNames...)
		for _, name := range serveOnly {
			if cmd.Flags().Changed(name) {
				return oops.Errorf("--%s requires --serve-skills", name)
			}
		}
	}
	if serve {
		for _, name := range []string{flagMCPRoot, flagAllowAnyDir} {
			if cmd.Flags().Changed(name) {
				return oops.Errorf("--%s applies to the authoring tools, which --serve-skills does not register", name)
			}
		}
	}
	var (
		srv       *mcp.Server
		transport = mcp.NewGuardedStdioTransport(os.Stdin, os.Stdout, nil)
	)
	if serve {
		var err error
		if srv, err = buildSkillServer(ctx, cmd); err != nil {
			return oops.Wrapf(err, "MCP: build skills server")
		}
		transport = srv.WrapTransport(transport)
	} else {
		srv = mcp.NewServer(Version, authoringOptions(cmd)...)
		transport = mcp.GuardLifecycle(transport)
	}

	closeTelemetry := wireMCPTelemetry(srv)
	run := func(ctx context.Context) error { return srv.GetMCPServer().Run(ctx, transport) }
	if err := serveUntilDone(ctx, run, closeTelemetry, srv.Close); err != nil {
		return oops.Wrapf(err, "MCP: start MCP server")
	}
	return nil
}

// authoringOptions are the options of the authoring server: the lint engine of
// `validate`, and the directory the tools are confined to.
func authoringOptions(cmd *cobra.Command) []mcp.Option {
	opts := []mcp.Option{mcp.WithValidator((&mcpValidator{}).validate), mcp.WithRoot(workingDir())}
	if anyDir, _ := cmd.Flags().GetBool(flagAllowAnyDir); anyDir { //nolint:errcheck // the flag is registered in init
		opts = append(opts, mcp.WithAnyDirectory())
	}
	if root, _ := cmd.Flags().GetString(flagMCPRoot); root != "" { //nolint:errcheck // the flag is registered in init
		opts = append(opts, mcp.WithRoot(root))
	}
	return opts
}

// serveUntilDone runs the server until the client disconnects or ctx ends (a
// signal), then runs closers in order. An end through ctx is a clean shutdown.
func serveUntilDone(ctx context.Context, run func(context.Context) error, closers ...func()) error {
	err := run(ctx)
	for _, closeFn := range closers {
		closeFn()
	}
	if err != nil && ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return nil
	}
	return err
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
	MCPCmd.Flags().String(flagMCPRoot, "", "Directory the authoring tools may read and write; working_directory must lie inside it (default: the current directory)")
	MCPCmd.Flags().Bool(flagAllowAnyDir, false, "Let the authoring tools use any working_directory, not only the root")
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
