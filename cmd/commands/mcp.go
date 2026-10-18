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

// flagServeProfile and flagServeTargets name the --profile and --targets flags.
const (
	flagServeProfile = "profile"
	flagServeTargets = "targets"
)

// Flags that set which directories the authoring tools may use.
const (
	flagMCPRoot     = "root"
	flagAllowAnyDir = "allow-any-dir"
	flagMCPReadOnly = "read-only"
)

var MCPCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start Model Context Protocol (MCP) server",
	Long: `Start an MCP server that allows AI assistants to interact with
ai-rulez configuration dynamically. The server provides tools for reading,
creating, updating, and deleting configuration elements.

The MCP server communicates via stdin/stdout and is designed to be integrated
with AI assistants that support the Model Context Protocol.

With --serve-skills the server serves the skills of one profile (MCP Skills
extension, SEP-2640) as skill:// resources, answers skills/list and skills/get,
and adds find_skill, load_skill and the other serving tools. It also registers
the authoring tools, so one process serves both and a project needs one MCP
entry. Pass --read-only to register only the read-only config and report tools,
so the server can inspect a project but never change it. --tools limits the
server to the tools named. The [mcp] table of the project can set serve_skills,
read_only, tools, daemon, no_watch, allow and deny; a flag given on the command line wins,
and an organization policy that requires read-only cannot be loosened.`,
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
	settings, err := loadMCPSettings(ctx, cmd.Flags())
	if err != nil {
		return err
	}
	serve := settings.serveSkills
	if !serve {
		serveOnly := append([]string{flagServeProfile, flagServeTargets, flagServeDomain, "allow", "deny"}, dynamicServeFlagNames...)
		for _, name := range serveOnly {
			if cmd.Flags().Changed(name) {
				return oops.Errorf("--%s requires --serve-skills (or [mcp] serve_skills = true)", name)
			}
		}
		if msg := ignoredSkillsKeysWarning(settings.ignoredSkillsKeys); msg != "" {
			logger.Warn(msg)
		}
	}
	if serve {
		if done, err := runMCPDaemon(ctx, cmd, settings); done {
			return err
		}
	}
	var (
		srv       *mcp.Server
		transport = mcp.NewGuardedStdioTransport(os.Stdin, os.Stdout, nil)
	)
	if serve {
		if srv, err = buildSkillServer(ctx, cmd, settings); err != nil {
			return oops.Wrapf(err, "MCP: build skills server")
		}
		transport = srv.WrapTransport(transport)
	} else {
		srv = NewAuthoringMCPServer(append(authoringOptions(cmd), settings.options()...)...)
		transport = mcp.GuardLifecycle(transport)
	}

	closeTelemetry := wireMCPTelemetry(srv)
	run := func(ctx context.Context) error { return srv.GetMCPServer().Run(ctx, transport) }
	if err := serveUntilDone(ctx, run, closeTelemetry, srv.Close); err != nil {
		return oops.Wrapf(err, "MCP: start MCP server")
	}
	return nil
}

// runMCPDaemon handles the shared-daemon mode of `mcp --serve-skills`. The
// spawned child (--daemon-server, hidden) runs the daemon itself; every other
// process is a thin proxy to it. done is true when the session was served (by
// the daemon or through a proxy to it); false means no daemon was asked for, or
// none could be reached, so the caller serves in-process exactly as without
// --daemon — a missing or broken daemon never fails a session.
func runMCPDaemon(ctx context.Context, cmd *cobra.Command, settings mcpSettings) (done bool, err error) {
	asServer, _ := cmd.Flags().GetBool(flagServeDaemonServer) //nolint:errcheck // the flag is registered in init
	if !asServer && !settings.daemon {
		return false, nil
	}
	setup, err := serveSetupFor(cmd, settings)
	if err != nil {
		return true, err
	}
	if asServer {
		return true, oops.Wrapf(mcp.RunDaemon(ctx, setup, mcp.DaemonOptions{}), "MCP: run the shared daemon")
	}
	handled, herr := mcp.ServeViaDaemon(ctx, setup, os.Stdin, os.Stdout, mcp.ProxyOptions{StartDaemon: startDaemonProcess})
	if handled {
		return true, oops.Wrapf(herr, "MCP: serve via the shared daemon")
	}
	return false, nil
}

// authoringOptions are the options of the authoring server that come from the
// command line: the directory the tools are confined to (the engines are wired
// by NewAuthoringMCPServer). They are also handed to the merged server
// `mcp --serve-skills` builds.
func authoringOptions(cmd *cobra.Command) []mcp.Option {
	opts := []mcp.Option{mcp.WithRoot(workingDir())}
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
func buildSkillServer(ctx context.Context, cmd *cobra.Command, settings mcpSettings) (*mcp.Server, error) {
	return buildDynamicSkillServer(ctx, cmd, settings)
}

func init() {
	MCPCmd.Flags().Bool(flagServeSkills, false, "Serve skills read-only over the MCP Skills extension instead of the authoring tools (default: [mcp] serve_skills, else off)")
	MCPCmd.Flags().String("profile", "", "Profile whose skills to serve (default: the configured default profile; requires --serve-skills)")
	MCPCmd.Flags().String("targets", "", "Preset whose rendering of the skills to serve (default: first configured preset with skills; requires --serve-skills)")
	MCPCmd.Flags().StringSlice(flagServeDomain, nil, "Only serve skills of these domains; 'root' selects skills in no domain (requires --serve-skills)")
	MCPCmd.Flags().StringSlice("allow", nil, "Only serve skills whose name matches one of these glob patterns (requires --serve-skills)")
	MCPCmd.Flags().StringSlice("deny", nil, "Never serve skills whose name matches one of these glob patterns; wins over --allow (requires --serve-skills)")
	registerDynamicServeFlags(MCPCmd)
	MCPCmd.Flags().String(flagMCPRoot, "", "Directory the authoring tools may read and write; working_directory must lie inside it (default: the current directory)")
	MCPCmd.Flags().Bool(flagAllowAnyDir, false, "Let the authoring tools use any working_directory, not only the root")
	MCPCmd.Flags().StringSlice(flagMCPTools, nil, "Register only these tools, repeatable or comma separated (default: [mcp] tools, else every tool); an unknown name is an error that lists the valid ones")
	MCPCmd.Flags().Bool(flagMCPReadOnly, false, "Register only the read-only authoring tools, so the server can inspect a project but not change it (with --serve-skills, keeps the merged server read-only)")
}
