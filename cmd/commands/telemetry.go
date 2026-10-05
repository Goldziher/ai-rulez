package commands

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Goldziher/ai-rulez/internal/mcp"
	"github.com/Goldziher/ai-rulez/internal/telemetry"
	"github.com/Goldziher/ai-rulez/internal/usage"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	telHarness    string
	telRole       string
	telRoot       string
	telConfigDir  string
	telFormat     string
	telExecutable string
	telOutput     string
	telBackground bool
	telJSON       bool
	telTimeout    time.Duration
)

// roleEnv names the environment variable that supplies the role when --role is
// not given (the MCP server has no flag of its own for it).
const roleEnv = "AI_RULEZ_ROLE"

// TelemetryCmd groups item-load telemetry commands. Nothing here records unless
// [telemetry] enabled is set, and nothing uses the network unless the user's own
// scope allows it.
var TelemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Item-load telemetry: record rule, agent and context loads; optional OTLP export",
	Long: `Record which rules, agents, context files and skills an AI harness loaded, as
identifier-only events, and optionally export them to an OpenTelemetry collector.

  ai-rulez telemetry hook     print the hooks that record loads (Claude Code: InstructionsLoaded,
                              SubagentStart/Stop, plus the Skill hooks of "usage hook")
  ai-rulez telemetry record   the command those hooks run; reads one hook event on stdin
  ai-rulez telemetry flush    ship the local outbox to the collector now
  ai-rulez telemetry doctor   show the resolved configuration, consent state and buffer

Everything is off by default. See docs/telemetry.md for what is collected, what never
is, and the rule that a repository cannot enable network export.`,
}

var telemetryHookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Print the hooks that record skill, rule, context and agent loads",
	Long: `Print the hook configuration for a harness. For --harness claude (the default) the
output is a "hooks" block for .claude/settings.json (--format json) or [[hooks]] groups for
config.toml (--format toml), so "ai-rulez generate" writes them into .claude/settings.json and
owns them. Handlers are async with a 5 second timeout: they only append to a local file.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out, err := telemetry.HookTemplate(telemetry.TemplateOptions{Executable: telExecutable, Harness: telHarness, Role: telRole, Format: telFormat})
		if err != nil {
			return err
		}
		if telOutput == "" {
			_, err = cmd.OutOrStdout().Write(out)
			return oops.Wrapf(err, "write hook template")
		}
		if err := os.MkdirAll(filepath.Dir(telOutput), 0o750); err != nil {
			return oops.Wrapf(err, "create output directory")
		}
		return oops.Wrapf(os.WriteFile(telOutput, out, 0o600), "write hook template")
	},
}

var telemetryRecordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record one item load from a hook event on stdin",
	Long: `Read one hook event from standard input. InstructionsLoaded, SubagentStart and
SubagentStop become item events; any other event is ignored. The command prints nothing
on standard output, never fails the session (errors go to standard error and the exit
status stays 0) and never waits on the network.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		if err := runTelemetryRecord(cmd.InOrStdin()); err != nil {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "ai-rulez telemetry record:", err) //nolint:errcheck // a hook must not fail on a closed stderr
		}
	},
}

func runTelemetryRecord(in io.Reader) error {
	data, err := io.ReadAll(io.LimitReader(in, 4<<20))
	if err != nil {
		return oops.Wrapf(err, "read hook event")
	}
	var peek struct {
		CWD string `json:"cwd"`
	}
	_ = json.Unmarshal(data, &peek) //nolint:errcheck // the handler reports a parse error itself
	p := newTelemetryPipeline(peek.CWD, true)
	if !p.Settings.RecordActive() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = p.HandleHook(ctx, bytes.NewReader(data), telemetry.HookOptions{Harness: telHarness, Role: telemetryRole()})
	return err
}

func telemetryRole() string {
	if telRole != "" {
		return telRole
	}
	return os.Getenv(roleEnv)
}

var telemetryFlushCmd = &cobra.Command{
	Use:   "flush",
	Short: "Send the local outbox to the OTLP collector",
	Long: `Ship pending events to the configured collector in batches, with retry and backoff.
Events that cannot be delivered stay in the outbox (bounded, oldest dropped first). Only one
flush runs at a time. --background is what the hooks start: it is silent and exits 0.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		p := newTelemetryPipeline("", false)
		if !p.Settings.ExportActive() {
			if telBackground {
				return nil
			}
			return oops.Errorf("OTLP export is not active (run `ai-rulez telemetry doctor`)")
		}
		timeout := telTimeout
		if timeout <= 0 {
			timeout = telemetry.FlushTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		result, err := p.Exporter.Flush(ctx)
		if telBackground {
			return nil
		}
		switch {
		case result.Skipped:
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "another flush is running") //nolint:errcheck // informational
		case err != nil:
			return oops.Wrapf(err, "flush (sent %d, rejected %d)", result.Sent, result.Rejected)
		default:
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "sent %d events in %d batches\n", result.Sent, result.Batches) //nolint:errcheck // informational
		}
		return nil
	},
}

var telemetryDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Show the resolved telemetry configuration, consent state and buffer",
	Long: `Print where each [telemetry] setting came from (default, repo, user, env), which repository
keys were ignored by the trust rule, why export is off, the endpoint host (never the path or
any header), the outbox size and the last flush. Prints no event content.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		p := newTelemetryPipeline("", false)
		report := telemetry.Diagnose(&p.Settings, telemetry.LocalDir(p.Root, telemetryConfigDirName()), nil)
		if telJSON {
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return oops.Wrapf(encoder.Encode(report), "encode doctor report")
		}
		report.Render(cmd.OutOrStdout())
		return nil
	},
}

func telemetryConfigDirName() string {
	if telConfigDir != "" {
		return telConfigDir
	}
	return configDirName()
}

// telemetryRoot finds the project root: --root, else CLAUDE_PROJECT_DIR, else the
// nearest ancestor of hint (default the working directory) holding the config
// directory, else hint itself.
func telemetryRoot(hint string) string {
	if telRoot != "" {
		abs, err := filepath.Abs(telRoot)
		if err == nil {
			return abs
		}
		return telRoot
	}
	if dir := os.Getenv("CLAUDE_PROJECT_DIR"); dir != "" {
		return dir
	}
	start := hint
	if start == "" {
		start, _ = os.Getwd() //nolint:errcheck // an empty start resolves to "."
	}
	for dir := start; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, telemetryConfigDirName())); err == nil && info.IsDir() {
			return dir
		}
		if filepath.Dir(dir) == dir {
			return start
		}
	}
}

// newTelemetryPipeline resolves the settings for the project and builds the
// pipeline. localLog adds the usage-log JSONL emitter.
func newTelemetryPipeline(cwdHint string, localLog bool) *telemetry.Pipeline {
	root := telemetryRoot(cwdHint)
	name := telemetryConfigDirName()
	settings := telemetry.ResolveFor(root, name, nil)
	return telemetry.Build(settings, telemetry.BuildOptions{Root: root, ConfigDirName: name, Version: Version, LocalLog: localLog, Spawn: telemetrySpawn})
}

// telemetrySpawn overrides how a detached flush starts; nil means the default
// (re-exec of this binary). Tests set it so a test binary never re-executes itself.
var telemetrySpawn func() error

// emitUsageTelemetry forwards a skill usage line to the OTLP spool. The line is
// already in the usage log, so no local emitter is added. It does nothing unless
// export is active, and it never reports an error.
func emitUsageTelemetry(entry *usage.Entry) {
	if entry == nil {
		return
	}
	p := newTelemetryPipeline("", false)
	if !p.Settings.ExportActive() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = p.Record(ctx, telemetry.FromUsageEntry(entry)) //nolint:errcheck // a skill hook must not fail on telemetry
}

// wireMCPTelemetry turns on item telemetry for an MCP server when the settings
// allow it and returns the function to call on shutdown (a final, bounded flush).
// With telemetry off it changes nothing and returns a no-op.
func wireMCPTelemetry(srv *mcp.Server) func() {
	p := newTelemetryPipeline("", true)
	if !p.Settings.RecordActive() {
		return func() {}
	}
	raw := make([]byte, 16)
	_, _ = rand.Read(raw) //nolint:errcheck // crypto/rand does not fail on supported platforms
	srv.SetTelemetry(p, mcp.TelemetryOptions{Role: telemetryRole(), Session: p.Session(hex.EncodeToString(raw))})
	p.Start()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = p.Close(ctx) //nolint:errcheck // shutdown flush is best effort
	}
}

func init() {
	RootCmd.AddCommand(TelemetryCmd)
	TelemetryCmd.AddCommand(telemetryHookCmd, telemetryRecordCmd, telemetryFlushCmd, telemetryDoctorCmd)
	for _, c := range []*cobra.Command{telemetryHookCmd, telemetryRecordCmd} {
		c.Flags().StringVar(&telHarness, "harness", "", "Harness: claude (default), codex or cursor")
		c.Flags().StringVar(&telRole, flagRole, "", "Role active in this session (recorded as given; else $"+roleEnv+")")
	}
	for _, c := range []*cobra.Command{telemetryRecordCmd, telemetryFlushCmd, telemetryDoctorCmd} {
		c.Flags().StringVar(&telRoot, "root", "", "Project root (default $CLAUDE_PROJECT_DIR, else the nearest directory holding the config directory)")
		c.Flags().StringVarP(&telConfigDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	}
	telemetryHookCmd.Flags().StringVar(&telExecutable, "executable", "ai-rulez", "Command the hook runs")
	telemetryHookCmd.Flags().StringVar(&telFormat, "format", "json", "Output: json (a hooks block) or toml ([[hooks]] groups for config.toml)")
	telemetryHookCmd.Flags().StringVarP(&telOutput, "output", "o", "", "Write the template to this file instead of stdout")
	telemetryFlushCmd.Flags().BoolVar(&telBackground, "background", false, "Silent mode used by hooks: exit 0 whatever happens")
	telemetryFlushCmd.Flags().DurationVar(&telTimeout, "timeout", 0, "Overall flush deadline (default 8s)")
	telemetryDoctorCmd.Flags().BoolVarP(&telJSON, "json", "j", false, "Emit the report as JSON")
}
