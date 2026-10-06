package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// exitScannersUnhealthy is the exit status of `scanners doctor` when a scanner has a problem.
const exitScannersUnhealthy = 2

var scannersAll bool

// ScannersCmd groups the commands that inspect the [[lint.external]] scanners.
var ScannersCmd = &cobra.Command{
	Use:   "scanners",
	Short: "Inspect the external scanners configured in [[lint.external]]",
	Long: `Show and check the third-party scanners that "scan --external" and
"validate --strict --external" run. "list" only looks the binaries up; "doctor"
also starts the ones you name to ask their version.`,
}

// ScannersListCmd lists the configured scanners.
var ScannersListCmd = &cobra.Command{
	Use:   "list [config-file]",
	Short: "List the configured scanners with their egress declaration and whether they are installed",
	Long: `List every [[lint.external]] entry: its egress declaration, whether its binary is on
PATH, and what it needs to run. Nothing is executed: the commands come from the
repository, so they run only with "scan --external".`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		infos, err := loadScanners(cmd.Context(), args)
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}
		writeScannerList(os.Stdout, infos)
	},
}

// ScannersDoctorCmd diagnoses scanners.
var ScannersDoctorCmd = &cobra.Command{
	Use:   "doctor <name>... | --all",
	Short: "Check named scanners: binary, version, egress, environment and configuration",
	Long: `For each named scanner (or every one with --all) report whether its binary is on
PATH, its version (the scanner is started once with --version, with a scrubbed
environment and a 10 second timeout), its egress declaration, the environment
variables it may receive, its timeout and staged inputs, and any configuration
problem that stops "scan --external" from running it (AR9E0, AR9E4).

Because doctor starts a program named in the repository's configuration, it runs
only the scanners you name. Exit 0 when every checked scanner is healthy, 2 when
one is not installed, is misconfigured, or has a network flag on an egress = false
entry, 1 when the configuration cannot be loaded or a name is unknown.`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if code := runScannersDoctor(cmd.Context(), args, os.Stdout); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	ScannersCmd.AddCommand(ScannersListCmd, ScannersDoctorCmd)
	ScannersDoctorCmd.Flags().BoolVar(&scannersAll, "all", false, "Check every configured scanner (starts each one to ask its version)")
	for _, c := range []*cobra.Command{ScannersListCmd, ScannersDoctorCmd} {
		c.Flags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
		c.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	}
}

// loadScanners loads the configuration and inspects its scanners.
func loadScanners(ctx context.Context, args []string) ([]lint.ScannerInfo, error) {
	cfg, err := loadConfigForCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	base, _ := filepath.Abs(cfg.BaseDir) //nolint:errcheck // falls back to the empty path
	return lint.InspectScanners(cfg.Lint, base), nil
}

func scannerStatus(s lint.ScannerInfo) string {
	switch {
	case len(s.Problems) > 0:
		return "invalid configuration (AR9E0)"
	case s.EgressFlag != "":
		return "blocked: " + s.EgressFlag + " on an egress = false scanner (AR9E4)"
	case !s.Found():
		return "not found on PATH"
	case s.NeedsAllow():
		return "found; needs --allow-egress=" + s.Name
	}
	return "found"
}

func writeScannerList(out io.Writer, infos []lint.ScannerInfo) {
	if len(infos) == 0 {
		fmt.Fprintln(out, "No scanners configured. Add a [[lint.external]] entry to .ai-rulez/config.toml.")
		return
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tEGRESS\tINPUTS\tSTATUS")
	for _, s := range infos {
		inputs := "-"
		if len(s.Inputs) > 0 {
			inputs = strings.Join(s.Inputs, ",")
		}
		egress := s.Egress
		if egress == "true" {
			egress = "YES"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, egress, inputs, scannerStatus(s))
	}
	_ = w.Flush() //nolint:errcheck // a closed stdout has no better handling
}

// runScannersDoctor checks the named scanners and returns the exit code.
func runScannersDoctor(ctx context.Context, args []string, out io.Writer) int {
	if len(args) == 0 && !scannersAll {
		fmtError(oops.Hint("Name a scanner, or pass --all to check every one").Errorf("doctor needs a scanner name or --all"))
		return 1
	}
	// Every argument is a scanner name, so the configuration is found by discovery, --config or --config-dir.
	infos, err := loadScanners(ctx, nil)
	if err != nil {
		fmtError(err)
		return 1
	}
	selected, unknown := selectScanners(infos, args, scannersAll)
	if len(unknown) > 0 {
		fmtError(oops.Errorf("unknown scanner %s (configured: %s)", strings.Join(unknown, ", "), scannerNames(infos)))
		return 1
	}
	code := 0
	for i, s := range selected {
		if i > 0 {
			fmt.Fprintln(out)
		}
		if !writeDoctor(ctx, out, s) {
			code = exitScannersUnhealthy
		}
	}
	return code
}

func scannerNames(infos []lint.ScannerInfo) string {
	if len(infos) == 0 {
		return "none"
	}
	names := make([]string, len(infos))
	for i, s := range infos {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}

// selectScanners picks the scanners by name (all of them with all) and returns
// the names that match none.
func selectScanners(infos []lint.ScannerInfo, names []string, all bool) (selected []lint.ScannerInfo, unknown []string) {
	if all {
		return infos, nil
	}
	for _, name := range names {
		found := false
		for _, s := range infos {
			if s.Name == name {
				selected, found = append(selected, s), true
			}
		}
		if !found {
			unknown = append(unknown, name)
		}
	}
	return selected, unknown
}

// writeDoctor prints one scanner's report and reports whether it is healthy.
func writeDoctor(ctx context.Context, out io.Writer, s lint.ScannerInfo) bool {
	row := func(key, value string) { fmt.Fprintf(out, "  %-9s %s\n", key, value) }
	fmt.Fprintln(out, s.Name)
	if s.Found() {
		version := lint.ProbeScannerVersion(ctx, s)
		if version == "" {
			version = "unknown (--version printed nothing usable)"
		}
		row("binary", s.Path)
		row("version", version)
	} else {
		row("binary", s.Command+": not found on PATH (or only through a relative PATH entry)")
	}
	switch s.Egress {
	case "true":
		row("egress", "true: content may leave the machine; runs only with --allow-egress="+s.Name)
	case "false":
		row("egress", "false: scrubbed environment; network flags are rejected")
	default:
		row("egress", "undeclared (AR9E1): runs with the full environment unless inputs is set")
	}
	if s.EgressFlag != "" {
		row("", "network flag "+s.EgressFlag+" on an egress = false scanner (AR9E4): the scanner is not run")
	}
	if len(s.EnvPass) > 0 {
		row("env_pass", strings.Join(s.EnvPass, ", "))
	} else {
		row("env_pass", "none (PATH, HOME, USER, locale and temp variables only)")
	}
	if len(s.Inputs) > 0 {
		row("inputs", strings.Join(s.Inputs, ", ")+": a read-only staged copy; HOME and TMPDIR are scratch directories")
	} else {
		row("inputs", "none: runs in the project root with the scanned file paths appended")
	}
	row("format", s.Format)
	row("timeout", s.Timeout.String())
	for _, p := range s.Problems {
		row("problem", p+" (AR9E0)")
	}
	verdict := "ok"
	if !s.Healthy() {
		verdict = "not healthy"
	}
	row("result", verdict)
	return s.Healthy()
}
