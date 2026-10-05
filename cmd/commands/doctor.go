package commands

import (
	"context"
	"io"
	"os"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/doctor"
	"github.com/Goldziher/ai-rulez/internal/progress"
	"github.com/spf13/cobra"
)

// exitDoctorFindings is the exit code when doctor found errors (or, with
// --strict, warnings) in a project it could diagnose.
const exitDoctorFindings = 2

// exitDoctorCannotRun is the exit code when doctor could not run: the
// configuration does not load, or the report could not be written.
const exitDoctorCannotRun = 1

var (
	doctorStrict  bool
	doctorJSON    bool
	doctorProfile string
)

// DoctorCmd runs read-only diagnostics over the project.
var DoctorCmd = &cobra.Command{
	Use:   "doctor [config-file]",
	Short: "Diagnose the project's ai-rulez setup (read-only)",
	Long: `Check the project for problems without writing anything: the configuration
loads and validates, preset names exist (with suggestions for typos and the
removed continue-dev and windsurf presets), generated files match their sources,
generated paths are git-ignored, shared settings documents still parse, MCP
${VAR} placeholders resolve, hook scripts exist and are executable, ai-rulez.lock
matches the config, and the tool behind each preset is on PATH (info only).

Findings are errors, warnings or info.

Exit codes: 0 no errors (and, with --strict, no warnings), 2 findings at the
failing severity, 1 the command could not run (the configuration does not load,
or the report could not be written).

Doctor never uses the network or writes the include cache: includes and
installed skills are not resolved, so the drift check is skipped when a project
declares them (run generate --check for that).`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if code := runDoctor(watchParentContext(cmd), args, os.Stdout); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	DoctorCmd.Flags().BoolVar(&doctorStrict, "strict", false, "Also exit non-zero on warnings")
	DoctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Print the report as JSON")
	DoctorCmd.Flags().StringVarP(&doctorProfile, "profile", "p", "", "Profile to render for the drift and gitignore checks (default: from config or 'default')")
	DoctorCmd.Flags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
	DoctorCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

// runDoctor runs the diagnostics, prints the report to out and returns the
// process exit code.
func runDoctor(ctx context.Context, args []string, out io.Writer) int {
	// Keep stdout to the report, so --json stays parseable.
	progress.SetQuiet(true)
	defer progress.SetQuiet(false)

	report := doctor.Run(ctx, doctor.Options{
		Profile: doctorProfile,
		Load: func(ctx context.Context, opts ...config.LoadOption) (*config.Config, error) {
			return loadConfigForCommand(ctx, args, opts...)
		},
	})

	var err error
	if doctorJSON {
		err = doctor.WriteJSON(out, report)
	} else {
		err = doctor.WriteText(out, report)
	}
	if err != nil {
		fmtError(err)
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
