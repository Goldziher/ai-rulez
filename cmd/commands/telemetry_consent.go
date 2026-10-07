package commands

import (
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	telEnableEndpoint string
	telEnableProtocol string
	telEnableSession  bool
	telEnablePaths    bool
	telEnableBackfill bool
	telStatusJSON     bool
)

var telemetryEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Consent to sending telemetry to a collector (stored per user)",
	Long: `Record your consent to export identifier-only events to one OTLP collector. The record
is a small file in your user config directory (mode 0600) naming the endpoint, the protocol and
the exact set of exported fields you agreed to:

  ai-rulez telemetry enable --endpoint https://collector.example.org:4318
  ai-rulez telemetry enable --endpoint collector.internal:4317 --protocol grpc   (gRPC: host[:port])

Consent is per machine and per endpoint, and only you can give it: a repository config cannot,
whatever it says. It stops covering a different endpoint, a different protocol, or a wider field set
(--include-session, --include-paths, or an allowlist that grew in a newer release); ` + "`telemetry status`" + `
then says it is stale and you run enable again. Enabling also turns local recording on for you.

Consent is not retroactive: the export cursor is placed at the end of the current usage log, so only
events recorded from now on are sent. --backfill places it at the start to send the existing
history too. Nothing is sent by this command; see ` + "`telemetry preview`" + ` for the exact payload.

AI_RULEZ_TELEMETRY=off and DO_NOT_TRACK=1 still win over a record, and so does an organization policy.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runTelemetryEnable(cmd.OutOrStdout())
	},
}

func runTelemetryEnable(out io.Writer) error {
	root, name := telemetryRoot(""), telemetryConfigDirName()
	if config.PolicyLocksIn(activePolicy, "telemetry", root) {
		return oops.Hint("Ask the owner of the organization policy.").Errorf("an organization policy forbids telemetry export")
	}
	settings := telemetry.ResolveFor(root, name, nil, activePolicy)

	endpoint := telEnableEndpoint
	if endpoint == "" {
		endpoint = settings.Endpoint
	}
	if endpoint == "" {
		return oops.Hint("Pass --endpoint, for example https://collector.example.org:4318.").Errorf("no collector endpoint to consent to")
	}
	protocol := telEnableProtocol
	if protocol == "" {
		protocol = settings.Protocol
	}
	// A gate counts when the user turned it on in config, env or a flag; a gate that only
	// the previous record carried is not carried over (re-enabling is a fresh decision).
	fromConfig := func(key string, on bool) bool { return on && settings.Sources[key] != telemetry.ScopeConsent }
	includeSession := telEnableSession || fromConfig("include_session", settings.IncludeSession)
	includePaths := telEnablePaths || fromConfig("include_paths", settings.IncludePaths)

	endpoint = config.NormalizeTelemetryEndpoint(endpoint, protocol)
	candidate := config.TelemetryConfig{OTLPEndpoint: endpoint, OTLPProtocol: protocol}
	if problems := candidate.Validate(); len(problems) > 0 {
		return oops.Hint("See docs/telemetry.md for the accepted endpoint and protocol forms.").Errorf("%s", strings.Join(problems, "; "))
	}

	record := telemetry.NewConsent(endpoint, protocol, includePaths, includeSession, telemetry.SystemClock(), Version)
	path := telemetry.ConsentPath(nil)
	if err := telemetry.SaveConsent(path, &record); err != nil {
		return err
	}

	w := reportWriter{out}
	w.printf("consent recorded for %s (%s)\n", displayEndpoint(endpoint), protocol)
	w.printf("  stored in %s (mode 0600)\n", path)
	w.printf("  exports: %s\n", strings.Join(record.Scope.Fields, ", "))
	if withheld := withheldFields(includePaths, includeSession); withheld != "" {
		w.printf("  withheld: %s\n", withheld)
	}
	placeCursor(w, root, name)

	after := telemetry.ResolveFor(root, name, nil, activePolicy)
	switch {
	case after.ExportActive():
		w.printf("export is on. Hooks record events (ai-rulez telemetry hook); preview what is sent with `ai-rulez telemetry preview`.\n")
	default:
		w.printf("export is still off: %s\n", strings.Join(after.ExportBlockers(), "; "))
	}
	return nil
}

// placeCursor puts the export cursor on the project's usage log: at the end unless
// --backfill. Without a project (no config directory) there is no log to place it on.
func placeCursor(w reportWriter, root, name string) {
	if info, err := os.Stat(filepath.Join(root, name)); err != nil || !info.IsDir() {
		w.printf("  no project here: the cursor is placed on the first flush inside a project\n")
		return
	}
	spool := &telemetry.Spool{Dir: telemetry.LocalDir(root, name)}
	if err := spool.PlaceCursor(defaultUsageLogPath(), telEnableBackfill); err != nil {
		w.printf("  could not place the export cursor (%v); the first flush will\n", err)
		return
	}
	if telEnableBackfill {
		w.printf("  cursor: at the start of the usage log, so existing history is exported\n")
		return
	}
	w.printf("  cursor: at the end of the usage log; only events recorded from now on are exported (--backfill sends history)\n")
}

func withheldFields(includePaths, includeSession bool) string {
	encoder := telemetry.Encoder{IncludePaths: includePaths, IncludeSession: includeSession}
	_, withheld := encoder.Fields()
	return strings.Join(withheld, ", ")
}

// displayEndpoint shows scheme and host only: never a path, query or credential.
func displayEndpoint(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "the endpoint"
	}
	return parsed.Scheme + "://" + parsed.Host
}

var telemetryDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Withdraw telemetry consent",
	Long: `Delete your consent record: network export stops at once, and so does the local recording the
record turned on; recording that your user config, the repository config or AI_RULEZ_TELEMETRY
turns on continues. The usage log and events already waiting in the outbox are kept (delete
.ai-rulez/local/telemetry-* to discard the outbox). Export also stays off while allow_network or
AI_RULEZ_TELEMETRY_ALLOW_NETWORK is unset; if either is set it still grants export and this command
tells you so.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runTelemetryDisable(cmd.OutOrStdout())
	},
}

func runTelemetryDisable(out io.Writer) error {
	removed, err := telemetry.RemoveConsent(telemetry.ConsentPath(nil))
	if err != nil {
		return err
	}
	w := reportWriter{out}
	if removed {
		w.printf("consent withdrawn: the record is deleted\n")
	} else {
		w.printf("there was no consent record\n")
	}
	after := telemetry.ResolveFor(telemetryRoot(""), telemetryConfigDirName(), nil, activePolicy)
	if after.ExportActive() {
		w.printf("export is still on because %s grants it; remove allow_network from your user config or unset the variable\n", consentSourceName(&after))
		return nil
	}
	w.printf("export is off; %s; the usage log and the outbox are kept\n", recordingState(&after))
	return nil
}

// recordingState says whether local recording continues once the consent record
// is gone: the record turned recording on, so it stops unless a config or the
// environment turns it on too.
func recordingState(s *telemetry.Settings) string {
	if !s.RecordActive() {
		return "local recording is off"
	}
	return "local recording stays on (" + recordingSource(s.Sources["enabled"]) + " enables it)"
}

func recordingSource(scope string) string {
	switch scope {
	case telemetry.ScopeEnv:
		return telemetry.EnvEnabled
	case telemetry.ScopeUser:
		return "your user config"
	case telemetry.ScopeRepo:
		return "the repository config"
	}
	return "the configuration"
}

func consentSourceName(s *telemetry.Settings) string {
	switch s.ConsentState {
	case telemetry.ConsentEnv:
		return telemetry.EnvAllowNetwork
	case telemetry.ConsentConfig:
		return "allow_network in your user config"
	}
	return "your configuration"
}

var telemetryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether telemetry is on, who consented, and whether delivery works",
	Long: `Print a short status: recording and export on or off and why, the consent state (record, user
config, environment, stale, none), the collector host and protocol (never the path or any header),
the fields that would be exported and withheld, how many events wait in the outbox and in the usage
log past the export cursor, and the delivery totals including failed flushes. Failed flushes are
silent to your harness by design; this is where they show. Nothing is written or sent.
` + "`telemetry doctor`" + ` prints every setting and where it came from.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		root, name := telemetryRoot(""), telemetryConfigDirName()
		settings := telemetry.ResolveFor(root, name, nil, activePolicy)
		status := telemetry.BuildStatus(&settings, telemetry.LocalDir(root, name), defaultUsageLogPath())
		if telStatusJSON {
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return oops.Wrapf(encoder.Encode(status), "encode status")
		}
		status.Render(cmd.OutOrStdout())
		return nil
	},
}

func init() {
	TelemetryCmd.AddCommand(telemetryEnableCmd, telemetryDisableCmd, telemetryStatusCmd)
	f := telemetryEnableCmd.Flags()
	f.StringVar(&telEnableEndpoint, "endpoint", "", "Collector endpoint, https (http only for loopback); for grpc host[:port] (default: the configured one)")
	f.StringVar(&telEnableProtocol, "protocol", "", "http/json (default), http/protobuf or grpc")
	f.BoolVar(&telEnableSession, "include-session", false, "Also export the salted session hash (pseudonymous)")
	f.BoolVar(&telEnablePaths, "include-paths", false, "Also export the repository-relative path of loaded items")
	f.BoolVar(&telEnableBackfill, "backfill", false, "Also export the events already in the usage log")
	for _, c := range []*cobra.Command{telemetryEnableCmd, telemetryDisableCmd, telemetryStatusCmd} {
		c.Flags().StringVar(&telRoot, "root", "", "Project root (default $CLAUDE_PROJECT_DIR, else the nearest directory holding the config directory)")
		c.Flags().StringVarP(&telConfigDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	}
	addJSONFormat(telemetryStatusCmd.Flags(), &telStatusJSON, "j")
}
