package commands

import (
	"context"
	"io"
	"os"
	"text/tabwriter"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// Exit codes of `verifiers run`, consistent with doctor: 0 every verifier
// passed (at the failing severity), 2 at least one failed (even if another
// verifier could not be evaluated), 1 the run could not complete and nothing
// failed (the configuration does not load or validate, an unknown --name, or a
// verifier that could not be evaluated).
const (
	exitVerifiersFindings  = 2
	exitVerifiersCannotRun = 1
)

var (
	verifiersStrict  bool
	verifiersJSON    bool
	verifiersNames   []string
	verifiersProfile string
)

// VerifiersCmd groups the deterministic repo checks declared as [[verifiers]].
var VerifiersCmd = &cobra.Command{
	Use:   "verifiers",
	Short: "Run the deterministic repo checks declared as [[verifiers]]",
	Long: `Verifiers are read-only, deterministic checks over the repository that you
declare in .ai-rulez/config.toml as [[verifiers]]: a file exists or is absent, a
glob matches a bounded number of files, a regex is present in (or forbidden from)
files, a JSON, YAML or TOML key has a value, generated files match their sources.
They never use the network and never start a process.`,
}

// VerifiersRunCmd evaluates the verifiers.
var VerifiersRunCmd = &cobra.Command{
	Use:   "run [config-file]",
	Short: "Evaluate the verifiers and report pass or fail for each",
	Long: `Evaluate every [[verifiers]] entry (or only those named with --name) and print a
table, or JSON with --json.

Exit codes: 0 no verifier failed at error severity (with --strict, also none at
warning severity), 2 at least one failed (even if another could not be evaluated),
1 the run could not complete and nothing failed: the configuration does not load or
validate, a --name is unknown, or a verifier could not be evaluated.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if code := runVerifiers(watchParentContext(cmd), args, os.Stdout); code != 0 {
			os.Exit(code)
		}
	},
}

// VerifiersListCmd lists the declared verifiers without evaluating them.
var VerifiersListCmd = &cobra.Command{
	Use:   "list [config-file]",
	Short: "List the declared verifiers",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if code := listVerifiers(watchParentContext(cmd), args, os.Stdout); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	VerifiersCmd.AddCommand(VerifiersRunCmd, VerifiersListCmd)
	VerifiersRunCmd.Flags().BoolVar(&verifiersStrict, "strict", false, "Also exit non-zero when a warning-severity verifier fails")
	VerifiersRunCmd.Flags().BoolVar(&verifiersJSON, "json", false, "Print the report as JSON")
	VerifiersRunCmd.Flags().StringSliceVar(&verifiersNames, "name", nil, "Run only the named verifier (repeatable)")
	VerifiersRunCmd.Flags().StringVarP(&verifiersProfile, "profile", "p", "", "Profile for generated_in_sync verifiers that name none (default: from config)")
	for _, c := range []*cobra.Command{VerifiersRunCmd, VerifiersListCmd} {
		c.Flags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
		c.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	}
}

// loadVerifierConfig loads and validates the configuration for the verifiers commands.
func loadVerifierConfig(ctx context.Context, args []string) (*config.Config, error) {
	cfg, err := loadConfigForCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, oops.Wrap(err)
	}
	return cfg, nil
}

// runVerifiers evaluates the verifiers, prints the report to out and returns
// the process exit code.
func runVerifiers(ctx context.Context, args []string, out io.Writer) int {
	progress.SetQuiet(true) // keep stdout to the report, so --json stays parseable
	defer progress.SetQuiet(false)

	cfg, err := loadVerifierConfig(ctx, args)
	if err != nil {
		fmtError(err)
		return exitVerifiersCannotRun
	}
	opts := verifiers.Options{Names: verifiersNames}
	if verifiersProfile != "" {
		for i := range cfg.Verifiers {
			if cfg.Verifiers[i].Type == config.VerifierGeneratedInSync && cfg.Verifiers[i].Profile == "" {
				cfg.Verifiers[i].Profile = verifiersProfile
			}
		}
	}
	report := verifiers.Run(ctx, cfg, opts)
	if report.Err != nil {
		fmtError(report.Err)
		return exitVerifiersCannotRun
	}
	if verifiersJSON {
		err = verifiers.WriteJSON(out, report)
	} else {
		err = verifiers.WriteText(out, report)
	}
	if err != nil {
		fmtError(err)
		return exitVerifiersCannotRun
	}
	// A failure outranks a verifier that could not be evaluated: the failure is
	// real and actionable, and exit 1 must not hide it. Both are in the report.
	if report.Failed(verifiersStrict) {
		return exitVerifiersFindings
	}
	if report.CannotRun() {
		return exitVerifiersCannotRun
	}
	return 0
}

// listVerifiers prints the declared verifiers.
func listVerifiers(ctx context.Context, args []string, out io.Writer) int {
	cfg, err := loadVerifierConfig(ctx, args)
	if err != nil {
		fmtError(err)
		return exitVerifiersCannotRun
	}
	if len(cfg.Verifiers) == 0 {
		_, _ = io.WriteString(out, "No verifiers configured.\n")
		return 0
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = io.WriteString(tw, "NAME\tTYPE\tSEVERITY\tDESCRIPTION\n")
	for _, v := range cfg.Verifiers {
		sev := v.Severity
		if sev == "" {
			sev = "error"
		}
		_, _ = io.WriteString(tw, v.Name+"\t"+v.Type+"\t"+sev+"\t"+v.Description+"\n")
	}
	if err := tw.Flush(); err != nil {
		fmtError(oops.Wrapf(err, "write verifiers list"))
		return exitVerifiersCannotRun
	}
	return 0
}
