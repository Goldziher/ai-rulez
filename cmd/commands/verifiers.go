package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
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
	verifiersNames   []string
	verifiersProfile string
	verifiersSince   string
	verifiersStaged  bool
	verifiersAll     bool
	verifiersRule    string
	verifiersFormat  string
	verifiersFailOn  string
	verifiersOut     string
	verifiersDead    bool
	verifiersJSON    bool
	verifiersExec    bool
	verifiersRole    string
)

// verifiersAllowExecEnv is the CI spelling of --allow-exec.
const verifiersAllowExecEnv = "AI_RULEZ_VERIFIERS_ALLOW_EXEC"

// allowExec reports whether command predicates may run: --allow-exec or the
// environment variable set to 1 or true. Nothing else implies it.
func allowExec() bool {
	if verifiersExec {
		return true
	}
	switch strings.ToLower(os.Getenv(verifiersAllowExecEnv)) {
	case "1", "true":
		return true
	}
	return false
}

// VerifiersCmd groups the deterministic repo checks declared as [[verifiers]].
var VerifiersCmd = &cobra.Command{
	Use:   "verifiers",
	Short: "Run the deterministic repo checks declared as [[verifiers]]",
	Long: `Verifiers are read-only, deterministic checks over the repository that you
declare in .ai-rulez/config.toml as [[verifiers]]: a file exists or is absent, a
glob matches a bounded number of files, a regex is present in (or forbidden from)
files, a JSON, YAML or TOML key has a value, generated files match their sources.
They never use the network and never start a process, except two predicates of a
rule-linked verifier that run only when you opt in: command (--allow-exec) and llm
(--allow-llm, which sends the changed lines to the configured model).

Larger sets, rule-linked verifiers (failures name the rule or skill they enforce), the
paired predicate and all/any/not combinators live in .ai-rulez/verifiers/*.toml
(see docs/verifiers.md).`,
}

// VerifiersRunCmd evaluates the verifiers.
var VerifiersRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Evaluate the verifiers and report pass or fail for each",
	Long: `Evaluate every [[verifiers]] entry (or only those named with --name) and print a
table, or JSON with --format json.

--since REV evaluates only the files changed since the merge base of REV and HEAD
(plus uncommitted and untracked files); --staged only what is staged. A base that does
not exist or share history with HEAD (a shallow clone) is an error, never a pass.
Formats: text (default), json, sarif and junit; --output writes the report to a file.

Exit codes: 0 no verifier failed at the --fail-on severity (error by default; --strict
means warning), 2 at least one failed (even if another could not be evaluated),
1 the run could not complete and nothing failed: the configuration does not load or
validate, a --name is unknown, or a verifier could not be evaluated.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runVerifiers(watchParentContext(cmd), os.Stdout)
	},
}

// VerifiersExplainCmd describes one verifier.
var VerifiersExplainCmd = &cobra.Command{
	Use:   "explain <name>",
	Short: "Explain what a verifier checks, the rule it enforces and how to fix it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return explainVerifier(watchParentContext(cmd), args[0], os.Stdout)
	},
}

// VerifiersTestCmd runs the self-test examples of the verifiers.
var VerifiersTestCmd = &cobra.Command{
	Use:   "test [name...]",
	Short: "Run the self-test examples of the verifiers offline",
	Long: `Run the [[verifiers.examples]] of verifiers declared under .ai-rulez/verifiers/.
Each example gets a temporary directory with its synthetic files, in which the listed
changed files count as entirely added; git and the real project are never touched.

Exit codes: 0 every example produced its expected outcome, 2 one did not (or a
declaration is invalid), 1 the configuration does not load or a name is unknown.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return testVerifiers(watchParentContext(cmd), args, os.Stdout)
	},
}

// VerifiersListCmd lists the declared verifiers without evaluating them.
var VerifiersListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List the declared verifiers",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return listVerifiers(watchParentContext(cmd), os.Stdout)
	},
}

func init() {
	VerifiersCmd.AddCommand(VerifiersRunCmd, VerifiersListCmd, VerifiersExplainCmd, VerifiersTestCmd)
	VerifiersRunCmd.Flags().BoolVar(&verifiersStrict, "strict", false, "Also exit non-zero when a warning-severity verifier fails")
	VerifiersRunCmd.Flags().StringSliceVar(&verifiersNames, "name", nil, "Run only the named verifier (repeatable)")
	specProfile.String(VerifiersRunCmd.Flags(), &verifiersProfile, "Active profile: sets the profile of generated_in_sync verifiers that name none, and which rules count as active (default: from config)")
	f := VerifiersRunCmd.Flags()
	specRole.String(f, &verifiersRole, "Active role: verifiers whose rule or skill the role does not keep are reported inactive")
	f.StringVar(&verifiersSince, "since", "", "Evaluate only files changed since the merge base of REV and HEAD (plus uncommitted and untracked)")
	f.BoolVar(&verifiersStaged, "staged", false, "Evaluate only staged changes")
	f.BoolVar(&verifiersAll, "all", false, "Evaluate every file (the default)")
	f.StringVar(&verifiersRule, "rule", "", "Run only the verifiers that enforce this rule, skill, agent or command")
	addFormatFlag(f, &verifiersFormat, "", formatText, formatText, formatJSON, "sarif", "junit")
	f.StringVar(&verifiersFailOn, "fail-on", "", "Lowest failing severity: error (default), warning, info or none")
	specOutput.String(f, &verifiersOut, "Write the report to this file instead of stdout")
	f.BoolVar(&verifiersDead, "strict-applicability", false, "Report a verifier whose when_changed matches no file (AR9H5)")
	f.BoolVar(&verifiersExec, "allow-exec", false, "Let command predicates run a program (or set "+verifiersAllowExecEnv+"=1); never implied by another flag")
	f.BoolVar(&verifiersAllowLLM, "allow-llm", false, "Evaluate llm verifiers: sends the changed lines to the configured model (needs allow_network in the user config)")
	f.BoolVar(&verifiersGateLLM, "gate-llm", false, "Let a failing llm verifier of error severity fail the run when its calibration record (verifiers calibrate) is current and meets the bar; otherwise every llm verdict stays a warning")
	f.Float64Var(&verifiersMaxCost, "max-cost", defaultVerifiersMaxCost, "Most an llm verifier run may cost in USD (0 removes this cap; [llm] limits still apply)")
	f.BoolVar(&verifiersEstimate, "estimate", false, "Print which files and how many bytes llm verifiers would send and the cost bound, and call nothing")
	VerifiersTestCmd.Flags().BoolVar(&verifiersExec, "allow-exec", false, "Let command predicates of the examples run a program (or set "+verifiersAllowExecEnv+"=1)")
	addJSONFormat(VerifiersListCmd.Flags(), &verifiersJSON, "")
	addJSONFormat(VerifiersExplainCmd.Flags(), &verifiersJSON, "")
	addJSONFormat(VerifiersTestCmd.Flags(), &verifiersJSON, "")
	for _, c := range []*cobra.Command{VerifiersRunCmd, VerifiersListCmd, VerifiersExplainCmd, VerifiersTestCmd} {
		specNoLocal.Bool(c.Flags(), &noLocal, "Ignore the machine-local config.local.* overlay and local/ content")
	}
}

// loadVerifierConfig loads and validates the configuration for the verifiers commands.
func loadVerifierConfig(ctx context.Context) (*config.Config, error) {
	cfg, err := loadConfigForCommand(ctx)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, oops.Wrap(err)
	}
	return cfg, nil
}

// runVerifiers evaluates the verifiers and prints the report to out. It returns
// nil, an ExitError with the findings code, or the failure.
func runVerifiers(ctx context.Context, out io.Writer) error {
	progress.SetQuiet(true) // keep stdout to the report, so --format json stays parseable
	defer progress.SetQuiet(false)

	cfg, err := loadVerifierConfig(ctx)
	if err != nil {
		return fail(err)
	}
	opts, format, failOn, err := verifierRunOptions()
	if err != nil {
		return fail(err)
	}
	applyVerifierProfile(cfg)
	if verifiersMaxCost < 0 {
		return fail(oops.Errorf("--max-cost must not be negative"))
	}
	// The user's [llm] settings are read only when an llm verifier will run, so a
	// broken one cannot break a project that has none.
	if verifiers.UsesLLM(cfg, opts) {
		var releaseLLM func()
		if opts.LLM, releaseLLM, err = verifierLLMOptions(ctx, cfg); err != nil {
			return fail(err)
		}
		defer releaseLLM()
	}
	report := verifiers.Run(ctx, cfg, opts)
	if report.Err != nil {
		return fail(report.Err)
	}
	var buf bytes.Buffer
	switch format {
	case "json":
		err = verifiers.WriteJSON(&buf, report)
	case "sarif":
		err = verifiers.WriteSARIF(&buf, report, Version)
	case "junit":
		err = verifiers.WriteJUnit(&buf, report, failOn)
	default:
		err = verifiers.WriteText(&buf, report)
	}
	if err == nil {
		err = emitReport(out, buf.Bytes())
	}
	if err != nil {
		return fail(err)
	}
	// A failure outranks a verifier that could not be evaluated: the failure is
	// real and actionable, and exit 1 must not hide it. Both are in the report.
	if report.FailedAt(failOn) {
		return exitStatus(exitVerifiersFindings)
	}
	if report.CannotRun() {
		return exitStatus(exitVerifiersCannotRun)
	}
	return nil
}

// applyVerifierProfile sets the active profile on the generated_in_sync
// verifiers that name none.
func applyVerifierProfile(cfg *config.Config) {
	if verifiersProfile == "" {
		return
	}
	for i := range cfg.Verifiers {
		if cfg.Verifiers[i].Type == config.VerifierGeneratedInSync && cfg.Verifiers[i].Profile == "" {
			cfg.Verifiers[i].Profile = verifiersProfile
		}
	}
}

// verifierRunOptions validates the run flags and builds the options.
func verifierRunOptions() (opts verifiers.Options, format, failOn string, err error) {
	modes := 0
	for _, set := range []bool{verifiersSince != "", verifiersStaged, verifiersAll} {
		if set {
			modes++
		}
	}
	if modes > 1 {
		return opts, "", "", oops.New("use only one of --since, --staged and --all")
	}
	format = verifiersFormat
	switch format {
	case "", "text", "json", "sarif", "junit":
	default:
		return opts, "", "", oops.Hint("Use text, json, sarif or junit.").Errorf("unknown --format %q", format)
	}
	failOn = verifiersFailOn
	if failOn == "" {
		failOn = "error"
		if verifiersStrict {
			failOn = "warning"
		}
	}
	switch failOn {
	case failOnError, failOnWarning, failOnInfo, valueNone:
	default:
		return opts, "", "", oops.Hint("Use error, warning, info or none.").Errorf("unknown --fail-on %q", failOn)
	}
	if verifiersSince != "" {
		if err := gitutil.CheckArg("--since", verifiersSince); err != nil {
			return opts, "", "", oops.Wrap(err)
		}
	}
	return verifiers.Options{
		Names: verifiersNames, Since: verifiersSince, Staged: verifiersStaged,
		Rule: verifiersRule, StrictApplicability: verifiersDead, AllowExec: allowExec(),
		Profile: verifiersProfile, Role: verifiersRole,
	}, format, failOn, nil
}

// emitReport writes the report to --output (atomically) or to out.
func emitReport(out io.Writer, data []byte) error {
	if verifiersOut == "" {
		_, err := out.Write(data)
		return oops.Wrapf(err, "write report")
	}
	return oops.Wrapf(gitutil.WriteFileAtomic(verifiersOut, data, 0o644), "write %s", verifiersOut)
}

// explainVerifier prints what a verifier checks.
func explainVerifier(ctx context.Context, name string, out io.Writer) error {
	cfg, err := loadVerifierConfig(ctx)
	if err != nil {
		return fail(err)
	}
	if verifiersJSON {
		doc, err := verifiers.ExplainInfo(cfg, name)
		if err != nil {
			return fail(err)
		}
		return fail(writeJSON(out, doc))
	}
	return fail(verifiers.Explain(out, cfg, name))
}

// testVerifiers runs the self-test examples and prints one line per example.
func testVerifiers(ctx context.Context, names []string, out io.Writer) error {
	cfg, err := loadVerifierConfig(ctx)
	if err != nil {
		return fail(err)
	}
	report, err := verifiers.RunExamplesWith(ctx, cfg, names, verifiers.Options{AllowExec: allowExec()})
	if err != nil {
		return fail(err)
	}
	if verifiersJSON {
		return writeVerifiersTestJSON(out, report)
	}
	var sb strings.Builder
	passed := 0
	for _, r := range report.Results {
		status := "ok  "
		if r.OK {
			passed++
		} else {
			status = "FAIL"
		}
		sb.WriteString(status + " " + r.Verifier + ": " + r.Example)
		if !r.OK {
			sb.WriteString(" (expected " + r.Want + ", got " + string(r.Got))
			if r.Message != "" {
				sb.WriteString(": " + r.Message)
			}
			sb.WriteString(")")
		}
		sb.WriteString("\n")
	}
	for _, p := range report.Problems {
		id := p.ID
		if id == "" {
			id = p.File
		}
		sb.WriteString("FAIL " + verifiers.CodeVerifierInvalid + " " + id + ": " + p.Message + "\n")
	}
	fmt.Fprintf(&sb, "%d of %d examples passed\n", passed, len(report.Results))
	if len(report.Untested) > 0 {
		sb.WriteString("no examples: " + strings.Join(report.Untested, ", ") + "\n")
	}
	if _, err := io.WriteString(out, sb.String()); err != nil {
		return fail(oops.Wrapf(err, "write test report"))
	}
	if report.Failed() {
		return exitStatus(exitVerifiersFindings)
	}
	return nil
}

// verifiersTestDoc is the `verifiers test --format json` document.
type verifiersTestDoc struct {
	SchemaVersion int                       `json:"schema_version"`
	OK            bool                      `json:"ok"`
	Passed        int                       `json:"passed"`
	Total         int                       `json:"total"`
	Results       []verifiers.ExampleResult `json:"results"`
	Untested      []string                  `json:"untested"`
	Problems      []verifiersTestProblem    `json:"problems"`
}

type verifiersTestProblem struct {
	Code    string `json:"code"`
	ID      string `json:"id,omitempty"`
	File    string `json:"file,omitempty"`
	Message string `json:"message"`
}

func writeVerifiersTestJSON(out io.Writer, report *verifiers.TestReport) error {
	doc := verifiersTestDoc{SchemaVersion: 1, Total: len(report.Results), Results: append([]verifiers.ExampleResult{}, report.Results...),
		Untested: append([]string{}, report.Untested...), Problems: []verifiersTestProblem{}}
	for _, r := range report.Results {
		if r.OK {
			doc.Passed++
		}
	}
	for _, p := range report.Problems {
		doc.Problems = append(doc.Problems, verifiersTestProblem{Code: verifiers.CodeVerifierInvalid, ID: p.ID, File: p.File, Message: p.Message})
	}
	doc.OK = !report.Failed()
	if err := writeJSON(out, doc); err != nil {
		return fail(err)
	}
	if !doc.OK {
		return exitStatus(exitVerifiersFindings)
	}
	return nil
}

// listVerifiers prints the declared verifiers.
func listVerifiers(ctx context.Context, out io.Writer) error {
	cfg, err := loadVerifierConfig(ctx)
	if err != nil {
		return fail(err)
	}
	rows := verifiers.List(cfg)
	if verifiersJSON {
		return fail(writeJSON(out, rows))
	}
	if len(rows) == 0 {
		_, _ = io.WriteString(out, "No verifiers configured.\n") //nolint:errcheck // terminal output
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = io.WriteString(tw, "NAME\tTYPE\tSEVERITY\tENFORCES\tDESCRIPTION\n") //nolint:errcheck // flushed below
	for _, r := range rows {
		target := "-"
		if r.Target != "" {
			target = r.Target
		}
		_, _ = io.WriteString(tw, r.Name+"\t"+r.Type+"\t"+r.Severity+"\t"+target+"\t"+r.Description+"\n") //nolint:errcheck // flushed below
	}
	return fail(oops.Wrapf(tw.Flush(), "write verifiers list"))
}
