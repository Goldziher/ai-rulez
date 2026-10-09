package commands

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// exitScannersUnhealthy is the exit status of `scanners doctor` when a scanner has a problem.
const exitScannersUnhealthy = 2

var (
	scannersAll    bool
	scannersProbe  bool
	scannersFormat string
)

// ScannersCmd groups the commands that inspect the [[lint.external]] scanners.
var ScannersCmd = &cobra.Command{
	Use:   "scanners",
	Short: "Inspect the external scanners configured in [[lint.external]]",
	Long: `Show and check the third-party scanners that "scan --external" and
"validate --external" run. "list" only looks the binaries up; "doctor"
starts a scanner to ask its version only with --external.`,
}

// ScannersListCmd lists the configured scanners.
var ScannersListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List the configured scanners with their egress declaration and whether they are installed",
	Long: `List every [[lint.external]] entry: its egress declaration, whether its binary is on
PATH, and what it needs to run. Nothing is executed: the commands come from the
repository, so they run only with "scan --external".`,
	Args:    cobra.NoArgs,
	PreRunE: checkScannersFormat,
	RunE: func(cmd *cobra.Command, args []string) error {
		infos, err := loadScanners(cmd.Context())
		if err != nil {
			return fail(err)
		}
		if scannersFormat == formatJSON {
			return fail(writeScannersJSON(os.Stdout, infos, false, nil))
		}
		writeScannerList(os.Stdout, infos)
		return nil
	},
}

// ScannersDoctorCmd diagnoses scanners.
var ScannersDoctorCmd = &cobra.Command{
	Use:   "doctor <name>... | --all",
	Short: "Check named scanners: binary, version, egress, environment and configuration",
	Long: `For each named scanner (or every one with --all) report whether its binary is on
PATH, its version (only with --external: the scanner is then started once with
--version, with a scrubbed environment and a 10 second timeout), its egress declaration, the environment
variables it may receive, its timeout and staged inputs, and any configuration
problem that stops "scan --external" from running it (AR9E0, AR9E4).

A scanner command comes from the repository's configuration, so doctor starts
nothing unless you pass --external (the same consent "scan --external" needs), and
then only the scanners you name or --all. Exit 0 when every checked scanner is healthy, 2 when
one is not installed, is misconfigured, or has a network flag on an egress = false
entry, 1 when the configuration cannot be loaded or a name is unknown.`,
	Args:    cobra.ArbitraryArgs,
	PreRunE: checkScannersFormat,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runScannersDoctor(cmd.Context(), args, os.Stdout)
	},
}

func init() {
	ScannersCmd.AddCommand(ScannersListCmd, ScannersDoctorCmd)
	ScannersDoctorCmd.Flags().BoolVar(&scannersAll, "all", false, "Check every configured scanner")
	ScannersDoctorCmd.Flags().BoolVar(&scannersProbe, "external", false, "Start each checked scanner once with --version (it is a program the repository named)")
	for _, c := range []*cobra.Command{ScannersListCmd, ScannersDoctorCmd} {
		addFormatFlag(c.Flags(), &scannersFormat, formatText, formatText, formatText, formatJSON)
		specNoLocal.Bool(c.Flags(), &noLocal, "Ignore the machine-local config.local.* overlay and local/ content")
	}
}

// loadScanners loads the configuration and inspects its scanners.
func loadScanners(ctx context.Context) ([]lint.ScannerInfo, error) {
	cfg, err := loadConfigForCommand(ctx)
	if err != nil {
		return nil, err
	}
	base, _ := filepath.Abs(cfg.BaseDir) //nolint:errcheck // falls back to the empty path
	return lint.InspectScanners(cfg, base), nil
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
		reportWriter{out}.printf("No scanners configured. Add a [[lint.external]] entry to .ai-rulez/config.toml.\n")
		return
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	reportWriter{w}.printf("NAME\tEGRESS\tINPUTS\tPRESET\tSTATUS\n")
	for i := range infos {
		s := &infos[i]
		inputs := "-"
		if len(s.Inputs) > 0 {
			inputs = strings.Join(s.Inputs, ",")
		}
		egress := s.Egress
		if egress == valueTrue {
			egress = "YES"
		}
		reportWriter{w}.printf("%s\t%s\t%s\t%s\t%s\n", s.Name, egress, inputs, presetColumn(*s), scannerStatus(*s))
	}
	_ = w.Flush() //nolint:errcheck // a closed stdout has no better handling
}

// presetColumn names the presets that include a scanner's profile ("-" for none).
func presetColumn(s lint.ScannerInfo) string {
	if len(s.Presets) == 0 {
		return "-"
	}
	return strings.Join(s.Presets, ",")
}

// runScannersDoctor checks the named scanners. It returns nil, an ExitError with
// exitScannersUnhealthy when one has a problem, or the failure.
func runScannersDoctor(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 && !scannersAll {
		return fail(oops.Hint("Name a scanner, or pass --all to check every one").Errorf("doctor needs a scanner name or --all"))
	}
	// Every argument is a scanner name, so the configuration is found by discovery, --config or --config-dir.
	infos, err := loadScanners(ctx)
	if err != nil {
		return fail(err)
	}
	selected, unknown := selectScanners(infos, args, scannersAll)
	if len(unknown) > 0 {
		return fail(oops.Errorf("unknown scanner %s (configured: %s)", strings.Join(unknown, ", "), scannerNames(infos)))
	}
	if scannersFormat == formatJSON {
		return doctorScannersJSON(ctx, out, selected)
	}
	if len(selected) == 0 {
		reportWriter{out}.printf("No scanners configured. Add a [[lint.external]] entry to .ai-rulez/config.toml.\n")
		return nil
	}
	code := 0
	for i := range selected {
		if i > 0 {
			reportWriter{out}.println()
		}
		if !writeDoctor(ctx, out, selected[i], scannersProbe) {
			code = exitScannersUnhealthy
		}
	}
	return exitStatus(code)
}

// doctorScannersJSON prints the --format json document of scanners doctor.
func doctorScannersJSON(ctx context.Context, out io.Writer, selected []lint.ScannerInfo) error {
	versions := map[string]string{}
	for i := range selected {
		if s := &selected[i]; s.Found() && scannersProbe {
			versions[s.Name], _ = lint.ProbeScannerVersion(ctx, *s) //nolint:errcheck // a refused probe leaves the version empty
		}
	}
	if err := writeScannersJSON(out, selected, true, versions); err != nil {
		return fail(err)
	}
	for i := range selected {
		if !selected[i].Healthy() {
			return exitStatus(exitScannersUnhealthy)
		}
	}
	return nil
}

func scannerNames(infos []lint.ScannerInfo) string {
	if len(infos) == 0 {
		return valueNone
	}
	names := make([]string, len(infos))
	for i := range infos {
		names[i] = infos[i].Name
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
		for i := range infos {
			if infos[i].Name == name {
				selected, found = append(selected, infos[i]), true
			}
		}
		if !found {
			unknown = append(unknown, name)
		}
	}
	return selected, unknown
}

// scannerVersionText is the version row of a scanner found on PATH: probed with
// --version only when probe is set.
func scannerVersionText(ctx context.Context, s lint.ScannerInfo, probe bool) string {
	if !probe {
		return "not probed (pass --external)"
	}
	v, err := lint.ProbeScannerVersion(ctx, s)
	switch {
	case err != nil:
		return "not probed (" + err.Error() + ")"
	case v == "":
		return "unknown (--version printed nothing usable)"
	}
	return v
}

// writeDoctor prints one scanner's report and reports whether it is healthy.
func writeDoctor(ctx context.Context, out io.Writer, s lint.ScannerInfo, probe bool) bool {
	rw := reportWriter{out}
	row := func(key, value string) { rw.printf("  %-9s %s\n", key, value) }
	rw.printf("%s\n", s.Name)
	if s.Found() {
		row("binary", s.Path)
		row("version", scannerVersionText(ctx, s, probe))
	} else {
		row("binary", s.Command+": not found on PATH (or only through a relative PATH entry)")
	}
	if s.Profile != "" {
		row("profile", s.Profile+" (embedded; presets: "+presetColumn(s)+")")
	}
	if s.FromPreset {
		row("preset", "added by [lint.scanner_policy] preset; runs with --external")
	}
	if s.Required {
		row("required", "yes: missing, outdated or failing is an error")
	}
	if s.Version != "" {
		row("range", s.Version+" (checked against --version before every run)")
	}
	switch s.Egress {
	case valueTrue:
		row("egress", "true: content may leave the machine; runs only with --allow-egress="+s.Name)
		if len(s.DataSent) > 0 {
			row("", "the vendor documents receiving: "+strings.Join(s.DataSent, "; "))
		}
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
	row("isolation", isolationText(s))
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

// isolationText says how this system would confine the scanner.
func isolationText(s lint.ScannerInfo) string {
	switch {
	case s.Isolation == valueNone:
		return "none (isolation = \"none\")"
	case len(s.Inputs) == 0 && s.Isolation == "require":
		return "require: refused, a scanner without inputs runs in the project root and cannot be confined"
	case len(s.Inputs) == 0:
		return "none: a scanner without inputs runs in the project root"
	case s.Backend == "" && s.Isolation == "require":
		return "require: refused, this system has no isolation backend"
	case s.Backend == "":
		return "unavailable on this system (isolation = \"" + s.Isolation + "\"): runs with a scrubbed environment only (AR9E7)"
	}
	return s.Backend + " (isolation = \"" + s.Isolation + "\"): no network unless egress = true, writes only in the scratch directory"
}

func checkScannersFormat(_ *cobra.Command, _ []string) error {
	if scannersFormat != formatText && scannersFormat != formatJSON {
		return oops.Hint("Use text or json").Errorf("unknown --format %q", scannersFormat)
	}
	return nil
}

// scannerJSON is one scanner in the --format json output of scanners list|doctor.
type scannerJSON struct {
	Name       string   `json:"name"`
	Command    string   `json:"command"`
	Path       string   `json:"path,omitempty"`
	Found      bool     `json:"found"`
	Egress     string   `json:"egress"`
	Format     string   `json:"format,omitempty"`
	Inputs     []string `json:"inputs"`
	EnvPass    []string `json:"env_pass"`
	TimeoutSec float64  `json:"timeout_seconds"`
	Problems   []string `json:"problems"`
	EgressFlag string   `json:"egress_flag,omitempty"`
	Status     string   `json:"status"`
	Healthy    bool     `json:"healthy"`
	Profile    string   `json:"profile,omitempty"`
	Presets    []string `json:"presets"`
	FromPreset bool     `json:"from_preset"`
	Required   bool     `json:"required"`
	// VersionRange is the version range the scanner must satisfy ("" for none).
	VersionRange string   `json:"version_range,omitempty"`
	DataSent     []string `json:"data_sent,omitempty"`
	Isolation    string   `json:"isolation"`
	// IsolationBackend is the confinement tool this system would use ("" for none).
	IsolationBackend string `json:"isolation_backend,omitempty"`
	// Version is set by doctor for an installed scanner ("" when it printed nothing usable).
	Version string `json:"version,omitempty"`
}

// writeScannersJSON prints {"scanners":[...]}. doctor adds the probed versions.
func writeScannersJSON(out io.Writer, infos []lint.ScannerInfo, doctor bool, versions map[string]string) error {
	rows := make([]scannerJSON, 0, len(infos))
	for i := range infos {
		s := &infos[i]
		row := scannerJSON{
			Name: s.Name, Command: s.Command, Path: s.Path, Found: s.Found(), Egress: s.Egress, Format: s.Format,
			Inputs: nonNil(s.Inputs), EnvPass: nonNil(s.EnvPass), TimeoutSec: s.Timeout.Seconds(), Problems: nonNil(s.Problems),
			EgressFlag: s.EgressFlag, Status: scannerStatus(*s), Healthy: s.Healthy(),
			Profile: s.Profile, Presets: nonNil(s.Presets), FromPreset: s.FromPreset, Required: s.Required,
			VersionRange: s.Version, DataSent: s.DataSent, Isolation: s.Isolation, IsolationBackend: s.Backend,
		}
		if doctor {
			row.Version = versions[s.Name]
		}
		rows = append(rows, row)
	}
	return jsondoc.Write(out, map[string]any{"scanners": rows}) //nolint:wrapcheck // a closed stdout has no better handling
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
