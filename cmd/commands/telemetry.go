package commands

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
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
	telLog        string
	telLimit      int
	// telWithEvals adds the recorded eval results to a preview.
	telWithEvals bool
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

  ai-rulez telemetry hook      print the hooks that record loads (Claude Code: InstructionsLoaded,
                               SubagentStart/Stop and the Skill hooks; codex and cursor: the skill hook)
  ai-rulez telemetry record    the command those hooks run; reads one hook event on stdin
  ai-rulez telemetry feedback  record that a skill misled you, is stale, wrong or great (notes stay local)
  ai-rulez telemetry report    join the usage log with the skills index, feedback and eval scores;
                               "report evals" ranks skills to prune or rewrite
  ai-rulez telemetry enable    consent to export to one collector (stored per user; a repo cannot)
  ai-rulez telemetry status    on or off, consent, pending events, delivery failures
  ai-rulez telemetry disable   withdraw consent
  ai-rulez telemetry flush     ship the local outbox to the collector now
  ai-rulez telemetry doctor    show the resolved configuration, consent state and buffer
  ai-rulez telemetry preview   print exactly what an export would send (no network)

Skill usage lines go to a machine-local log (default .ai-rulez/local/usage.jsonl) that holds the
skill name, a timestamp, a salted hash of the session id, the content hash, the harness and an
outcome; never prompts, arguments or file contents. Set [usage] skills_index = true so generate
writes .ai-rulez/skills-index.json, which gives every logged skill its content hash.

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
		out, err := telemetry.HookTemplate(telemetry.TemplateOptions{
			Executable: telExecutable, Harness: telHarness, Role: telRole, Format: telFormat,
			LogPath: usageLog, SinkCommand: usageSinkCommand, IndexPath: usageIndex,
		})
		var unsupported *usage.UnsupportedHarnessError
		if errors.As(err, &unsupported) {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning:", unsupported) //nolint:errcheck // a warning on a closed stderr is not actionable
			return nil
		}
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
	Short: "Record one skill or item load from a hook event on stdin",
	Long: `Read one hook event from standard input. A skill invocation is appended to the usage
log (ts, skill, id, hash, session, invocation); for codex and cursor only a read of
skills/<id>/SKILL.md counts, and their payload is inferred, not verified. InstructionsLoaded,
SubagentStart and SubagentStop become item events when [telemetry] enabled is set; any other
event is ignored. The command prints nothing
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
	// The usage recorder ignores events that are not skill loads and the item
	// handler ignores the rest, so one event feeds whichever applies.
	usageErr := runUsageRecord(bytes.NewReader(data))
	p := newTelemetryPipeline(peek.CWD, true)
	if !p.Settings.RecordActive() {
		return usageErr
	}
	ctx, cancel := context.WithTimeout(cmdContext(), 2*time.Second)
	defer cancel()
	if _, err = p.HandleHook(ctx, bytes.NewReader(data), telemetry.HookOptions{Harness: telHarness, Role: telemetryRole()}); err != nil {
		return err
	}
	return usageErr
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
First it queues the usage-log events past the export cursor that the outbox does not already hold
(see "usage export --to otlp"). Events that cannot be delivered stay in the outbox (bounded, oldest
dropped first). Only one flush runs at a time. --background is what the hooks start: it is silent,
bounded by the flush deadline and exits 0; failures are counted in "telemetry status".`,
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
		if timeout > telemetry.MaxFlushTimeout {
			return oops.Errorf("--timeout %s exceeds the maximum %s (the flush lock would look stale and be taken over)", timeout, telemetry.MaxFlushTimeout)
		}
		ctx, cancel := context.WithTimeout(cmdContext(), timeout)
		defer cancel()
		result, err := p.Flush(ctx)
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
			return jsondoc.Write(cmd.OutOrStdout(), report)
		}
		report.Render(cmd.OutOrStdout())
		return nil
	},
}

var telemetryPreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Print exactly what an export would send, without sending anything",
	Long: `Encode pending events with the allowlisted encoder an export uses and print the requests that
would be made: the destination, the exact JSON body of each request, and which fields are exported
and which are withheld. Nothing is sent and nothing is written; no network connection is opened,
and this works whether or not export is enabled or consented to.

The events come from the outbox when export is active, otherwise from the usage log (--log FILE
picks another log). Sampling applies to a log, as it would when recording. --with-evals adds the
eval_result events and gauges that "usage export --with-evals" would send. A request shows only the
scheme, host and path of the endpoint, never a header or credential. --limit N previews the first N
events (default 5, 0 for all).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runTelemetryPreview(cmd.OutOrStdout())
	},
}

// previewSource is where a preview's events came from.
type previewSource struct {
	label  string
	events []telemetry.Event
	// rejected counts log lines that failed validation.
	rejected int
}

func runTelemetryPreview(out io.Writer) error {
	if telLimit < 0 {
		return oops.Errorf("--limit must not be negative")
	}
	root, name := telemetryRoot(""), telemetryConfigDirName()
	settings := telemetry.ResolveFor(root, name, nil, activePolicy)
	source, err := previewEvents(&settings, root, name)
	if err != nil {
		return err
	}
	if telWithEvals {
		evalEvents, evalErr := loadEvalEvents("")
		if evalErr != nil {
			return evalErr
		}
		source.events = append(source.events[:len(source.events):len(source.events)], evalEvents...)
	}
	w := reportWriter{out}
	total := len(source.events)
	shown := source.events
	if telLimit > 0 && total > telLimit {
		shown = shown[:telLimit]
	}
	encoder := settings.Encoder(Version)
	plan, err := encoder.Plan(shown, telemetry.DefaultBatchMax, true)
	if err != nil {
		return err
	}
	exported, withheld := encoder.Fields()

	w.printf("source: %s (%d events, previewing %d)\n", source.label, total, len(shown))
	if source.rejected > 0 {
		w.printf("left out: %d lines that failed validation\n", source.rejected)
	}
	if blockers := settings.ExportBlockers(); len(blockers) > 0 {
		w.printf("export: off (%s); this is what would be sent once it is on\n", strings.Join(blockers, "; "))
	} else {
		w.printf("export: on\n")
	}
	if len(plan) == 0 {
		w.printf("\nNothing to send.\n")
		return nil
	}
	w.printf("timestamps: the observation and metric times below are the newest previewed event's; a flush stamps its own clock\n")
	for i := range plan {
		request := &plan[i]
		target := telemetry.DisplayURL(settings.Endpoint, request.Path)
		if target == "" {
			target = "<no endpoint configured>" + request.Path
		}
		w.printf("\nPOST %s  (%d events, gzip, %d bytes)\n%s\n", target, request.Events, request.GzipBytes, request.Body)
	}
	w.printf("\nfields exported: %s\n", strings.Join(exported, ", "))
	w.printf("fields withheld: %s\n", orNone(withheld))
	w.printf("\nNothing was sent.\n")
	return nil
}

// previewEvents loads the events to preview: the outbox when export is active
// and no log was named, else the usage log.
func previewEvents(settings *telemetry.Settings, root, name string) (previewSource, error) {
	localDir := telemetry.LocalDir(root, name)
	if telLog == "" && settings.ExportActive() {
		spool := &telemetry.Spool{Dir: localDir}
		events, corrupt, err := spool.Pending()
		if err != nil {
			return previewSource{}, err
		}
		return previewSource{label: "outbox " + filepath.Join(name, telemetry.LocalDirName, telemetry.OutboxFileName), events: events, rejected: corrupt}, nil
	}
	path, label := telLog, telLog
	if path == "" {
		path = defaultUsageLogPath()
		label = filepath.Join(name, telemetry.LocalDirName, "usage.jsonl")
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return previewSource{label: label + " (missing)"}, nil
		}
	}
	read, err := telemetry.ReadLogEvents(path)
	if err != nil {
		return previewSource{}, err
	}
	kept := read.Events[:0:0]
	for i := range read.Events {
		if telemetry.Sampled(settings.Sample, &read.Events[i]) {
			kept = append(kept, read.Events[i])
		}
	}
	return previewSource{label: label + " (usage log)", events: kept, rejected: read.Rejected}, nil
}

func orNone(list []string) string {
	if len(list) == 0 {
		return "none"
	}
	return strings.Join(list, ", ")
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
	settings := telemetry.ResolveFor(root, name, nil, activePolicy)
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
	ctx, cancel := context.WithTimeout(cmdContext(), time.Second)
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
		ctx, cancel := context.WithTimeout(cmdContext(), 3*time.Second)
		defer cancel()
		_ = p.Close(ctx) //nolint:errcheck // shutdown flush is best effort
	}
}

func init() {
	RootCmd.AddCommand(TelemetryCmd)
	TelemetryCmd.AddCommand(telemetryHookCmd, telemetryRecordCmd, telemetryFlushCmd, telemetryDoctorCmd, telemetryPreviewCmd)
	for _, c := range []*cobra.Command{telemetryHookCmd, telemetryRecordCmd} {
		c.Flags().StringVar(&usageLog, "log", "", "Usage log file to append skill loads to (default .ai-rulez/local/usage.jsonl)")
		c.Flags().StringVar(&usageSinkCommand, "sink-command", "", "Shell command that receives each usage log line on stdin")
		c.Flags().StringVar(&usageIndex, "index", "", "Skills index used to resolve content hashes")
	}
	telemetryRecordCmd.Flags().StringVar(&usageOutcome, "outcome", "", "Outcome to record for a skill load: loaded (default), used or abandoned")
	telemetryRecordCmd.Flags().BoolVar(&usageServed, "served", false, "Mark the skill load as served by the MCP server")
	telemetryRecordCmd.Flags().StringVar(&usageSalt, "salt-file", "", "File holding the session-hash salt (default usage.salt beside the log; $AI_RULEZ_USAGE_SALT wins)")
	for _, c := range []*cobra.Command{telemetryHookCmd, telemetryRecordCmd} {
		c.Flags().StringVar(&telHarness, "harness", "", "Harness: claude (default), codex or cursor")
		c.Flags().StringVar(&telRole, flagRole, "", "Role active in this session (recorded as given; else $"+roleEnv+")")
	}
	for _, c := range []*cobra.Command{telemetryRecordCmd, telemetryFlushCmd, telemetryDoctorCmd, telemetryPreviewCmd} {
		c.Flags().StringVar(&telRoot, "root", "", "Project root (default $CLAUDE_PROJECT_DIR, else the nearest directory holding the config directory)")
		c.Flags().StringVarP(&telConfigDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	}
	telemetryHookCmd.Flags().StringVar(&telExecutable, "executable", "ai-rulez", "Command the hook runs")
	telemetryHookCmd.Flags().StringVar(&telFormat, "format", "json", "Output: json (a hooks block) or toml ([[hooks]] groups for config.toml)")
	telemetryHookCmd.Flags().StringVarP(&telOutput, "output", "o", "", "Write the template to this file instead of stdout")
	telemetryFlushCmd.Flags().BoolVar(&telBackground, "background", false, "Silent mode used by hooks: exit 0 whatever happens")
	telemetryFlushCmd.Flags().DurationVar(&telTimeout, "timeout", 0, "Overall flush deadline (default 8s, at most 30s)")
	telemetryPreviewCmd.Flags().StringVar(&telLog, "log", "", "Usage log to preview instead of the outbox (default <config dir>/local/usage.jsonl)")
	telemetryPreviewCmd.Flags().IntVar(&telLimit, "limit", 5, "Preview the first N events (0 for all)")
	telemetryPreviewCmd.Flags().BoolVar(&telWithEvals, "with-evals", false, "Also preview the eval results that telemetry export --with-evals would send")
	addJSONFormat(telemetryDoctorCmd.Flags(), &telJSON, "j")
}
