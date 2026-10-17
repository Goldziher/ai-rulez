package commands

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/doctor"
	"github.com/Goldziher/ai-rulez/v5/internal/indexcache"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/spf13/cobra"
)

// exitDoctorFindings is the exit code when doctor found errors (or, with
// --strict, warnings) in a project it could diagnose.
const exitDoctorFindings = 2

// exitDoctorCannotRun is the exit code when doctor could not run: the
// configuration does not load, or the report could not be written.
const exitDoctorCannotRun = 1

var (
	doctorStrict       bool
	doctorJSON         bool
	doctorProfile      string
	doctorMCPUserScope bool
)

// DoctorCmd runs read-only diagnostics over the project.
var DoctorCmd = &cobra.Command{
	Use:   cmdUseDoctor,
	Short: "Diagnose the project's ai-rulez setup (read-only)",
	Long: `Check the project for problems without writing anything: the configuration
loads and validates, preset names exist (with suggestions for typos and the
removed continue-dev and windsurf presets), generated files match their sources,
generated paths are git-ignored, shared settings documents still parse, MCP
${VAR} placeholders resolve, hook scripts exist and are executable, ai-rulez.lock
matches the config, and the tool behind each preset is on PATH (info only).

With --mcp-user-scope it also compares the project MCP servers with the
same-named servers in a configured harness's user or local scope (for example
~/.claude.json), naming both endpoints and the removal command, and reports
generated servers that differ only by name while pointing at the same url. That
check reads files outside the project, so it is off by default.

Findings are errors, warnings or info.

Exit codes: 0 no errors (and, with --strict, no warnings), 2 findings at the
failing severity, 1 the command could not run (the configuration does not load,
or the report could not be written).

Doctor never uses the network or writes the include cache: includes and
installed skills are not resolved, so the drift check is skipped when a project
declares them (run generate --check for that).

A project with no configuration file is diagnosed through an implicit one: only
the checks that need no presets, outputs or lock run, and nothing is written.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return exitStatus(runDoctor(watchParentContext(cmd), cmd.OutOrStdout()))
	},
}

func init() {
	DoctorCmd.Flags().BoolVar(&doctorStrict, "strict", false, "Also exit non-zero on warnings")
	addJSONFormat(DoctorCmd.Flags(), &doctorJSON, "")
	specProfile.String(DoctorCmd.Flags(), &doctorProfile, "Profile to render for the drift and gitignore checks (default: from config or 'default')")
	specNoLocal.Bool(DoctorCmd.Flags(), &noLocal, "Ignore the machine-local config.local.* overlay and local/ content")
	DoctorCmd.Flags().BoolVar(&doctorMCPUserScope, "mcp-user-scope", false, "Also check MCP servers against the configured harnesses' user and local scopes (reads files outside the project, for example ~/.claude.json); off by default")
}

// runDoctor runs the diagnostics, prints the report to out and returns the
// process exit code.
func runDoctor(ctx context.Context, out io.Writer) int {
	// Keep stdout to the report, so --format json stays parseable.
	progress.SetQuiet(true)
	defer progress.SetQuiet(false)

	report := doctor.Run(ctx, doctor.Options{
		Profile:     doctorProfile,
		Load:        lintLoader(),
		ReadUserMCP: doctorMCPUserScope,
		IndexCache: func(cfg *config.Config) indexcache.Options {
			opts, _ := (&mcp.ServeSetup{Version: Version}).IndexCacheOptions(ctx, cfg, false)
			return opts
		},
		Probe: probeURL,
		OnCheck: func(name string, elapsed time.Duration) { // shown under --debug
			logger.Debug("Doctor check finished", "check", name, "elapsed", elapsed.Round(time.Millisecond))
		},
	})

	var err error
	if doctorJSON {
		err = doctor.WriteJSON(out, report)
	} else {
		err = doctor.WriteText(out, report)
	}
	if err != nil {
		renderError(os.Stderr, err)
		return exitDoctorCannotRun
	}
	if report.Unloadable {
		return exitDoctorCannotRun
	}
	if report.Failed(doctorStrict) {
		return exitDoctorFindings
	}
	return 0
}

// probeURL asks url with a short GET, honoring the proxy settings of the environment, and
// returns the HTTP status. It backs the session-start check's registry probe.
func probeURL(ctx context.Context, url string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return 0, err //nolint:wrapcheck // the caller prints the first line
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // a fixed registry URL chosen by the doctor check
	if err != nil {
		return 0, err //nolint:wrapcheck // the caller prints the first line
	}
	defer resp.Body.Close() //nolint:errcheck // nothing was read
	return resp.StatusCode, nil
}
