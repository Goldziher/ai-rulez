package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	tokensJSON            bool
	tokensBudget          int
	tokensCompareProfiles []string
	tokensTokenizer       string
	tokensRole            string
	tokensByRole          bool
)

// TokensCmd reports the token surface of the generated configuration.
var TokensCmd = &cobra.Command{
	Use:   "tokens",
	Short: "Report the prompt-token cost of generated artifacts",
	Long: `Report how many prompt tokens the generated configuration costs, split by
when an agent loads it.

Artifacts are measured as rendered strings in memory, never read back from disk,
so the report is correct even when no output has been generated yet.

The report separates surface paid on every request from surface paid only when an
artifact is opened, because a flat per-file count misleads: a skill's body is far
larger than its listing entry, and only the entry reaches the prompt. For every
harness that lists skills, commands or agents at session start, the always-loaded
figure includes an estimated listing: each item's name and description (and path,
where the harness adds one) plus a per-entry framing constant. --budget gates that
figure. The JSON keeps always_legacy and conditional_legacy for the numbers the
earlier name-only model reported.

Counts are approximations — Claude's tokenizer is not published — and cover only
what ai-rulez generates. The agent harness adds a fixed floor of its own that
ai-rulez cannot see, so no figure here predicts a session total.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		overBudget, err := runTokens(cmd.OutOrStdout())
		if err != nil {
			return fail(err)
		}
		if overBudget {
			return exitStatus(budgetExceededExitCode)
		}
		return nil
	},
}

func init() {
	specNoLocal.Bool(TokensCmd.Flags(), &noLocal, "Ignore the machine-local config.local.* overlay and local/ content (the view a teammate without them sees)")
	addJSONFormat(TokensCmd.Flags(), &tokensJSON, "j")
	TokensCmd.Flags().IntVar(&tokensBudget, "budget", 0,
		"Fail when the headline always-loaded count exceeds this ceiling")
	// StringArray, not StringSlice: a comma composes several profiles into one
	// value, so it cannot also mean "next entry". Repeat the flag per column.
	TokensCmd.Flags().StringArrayVar(&tokensCompareProfiles, "compare-profiles", nil,
		"Report this profile as one column of a comparison table; repeat per column")
	TokensCmd.Flags().StringVar(&tokensTokenizer, "tokenizer", tokens.CounterCL100KBase,
		"Token counter to use: "+strings.Join(tokens.Names(), " or "))
	TokensCmd.Flags().StringVar(&tokensRole, flagRole, "", "Report on this role's content slice instead of a profile (see 'ai-rulez roles list')")
	TokensCmd.Flags().BoolVar(&tokensByRole, "by-role", false, "Report every declared role as one column of a comparison table")
	specProfile.String(TokensCmd.Flags(), &profile, "Profile to report on, or a comma-separated list to compose several")
}

// budgetExceededExitCode is returned when a report exceeds --budget. Distinct
// from 1 so a hook can tell "over budget" from "the command failed".
const budgetExceededExitCode = 2

// runTokens builds and writes the report. It returns whether a configured budget
// was exceeded rather than exiting, so the behavior is testable without a
// subprocess.
func runTokens(out io.Writer) (overBudget bool, err error) {
	counter, err := tokens.New(tokensTokenizer)
	if err != nil {
		return false, oops.Hint("Accepted values: "+strings.Join(tokens.Names(), ", ")).Wrapf(err, "select tokenizer")
	}

	cfg, err := loadConfigForCommand(cmdContext())
	if err != nil {
		return false, err
	}
	if err := cfg.Validate(); err != nil {
		return false, err
	}

	targets, err := tokenTargets(cfg)
	if err != nil {
		return false, err
	}

	reports := make([]*generator.TokenReport, 0, len(targets))
	for _, target := range targets {
		// A fresh Generator per profile or role: collectOutputs writes SourceHash
		// onto the config it holds, so reusing one across reports would carry a
		// stale hash into the next.
		gen := generator.NewGenerator(cfg)
		if target.role != "" {
			if err := gen.SetRole(target.role); err != nil {
				return false, err //nolint:wrapcheck // already contextual
			}
		}
		report, err := gen.TokenReport(generator.TokenReportOptions{
			Profile: target.profile,
			Counter: counter,
			Budget:  tokensBudget,
		})
		if err != nil {
			return false, err
		}
		reports = append(reports, report)
	}

	if err := writeTokenReports(out, reports); err != nil {
		return false, err
	}

	for _, report := range reports {
		if report.Budget != nil && report.Budget.Exceeded {
			reportWriter{out}.printf(
				"\nOver budget: profile %q always-loaded surface is %s tokens, ceiling is %s.\n",
				report.Profile, humanCount(report.Budget.Actual), humanCount(report.Budget.Limit))
			overBudget = true
		}
	}
	return overBudget, nil
}

// reportWriter formats the human-readable report. Write errors are dropped on
// purpose: the destination is stdout or a test buffer, neither of which fails in a
// way a caller could act on, and threading an error out of every line of a
// formatted table would obscure the layout for no gain.
type reportWriter struct{ out io.Writer }

func (w reportWriter) printf(format string, args ...any) {
	//nolint:errcheck // Dropping the write error is the point of this type.
	fmt.Fprintf(w.out, format, args...)
}

func (w reportWriter) println() {
	//nolint:errcheck // Dropping the write error is the point of this type.
	fmt.Fprintln(w.out)
}

func writeTokenReports(out io.Writer, reports []*generator.TokenReport) error {
	if tokensJSON {
		payload := any(reports)
		if len(reports) == 1 {
			payload = reports[0]
		}
		return jsondoc.Write(out, payload)
	}

	if len(reports) > 1 {
		writeComparisonTable(reportWriter{out}, reports)
		return nil
	}
	writeTokenReport(reportWriter{out}, reports[0])
	return nil
}

func writeTokenReport(w reportWriter, report *generator.TokenReport) {
	w.printf("Token surface — profile %q\n", report.Profile)
	w.printf("Tokenizer: %s (approximate)\n\n", report.Tokenizer.Name)

	for i := range report.Runtimes {
		writeRuntime(w, &report.Runtimes[i])
	}
	for i := range report.Scoped {
		writeRuntime(w, &report.Scoped[i])
	}

	if report.HeadlinePreset != "" {
		w.printf("Headline always-loaded surface: %s tokens (%s, the largest single runtime)\n",
			humanCount(report.HeadlineAlways), report.HeadlinePreset)
		if report.HeadlineListing > 0 {
			w.printf("  of which item listing (estimate): %s tokens; pre-listing model reported %s\n",
				humanCount(report.HeadlineListing), humanCount(report.HeadlineAlwaysLegacy))
		}
	}

	if len(report.Domains) > 0 {
		w.printf("\nPer domain:\n")
		w.printf("  %-28s %12s %12s %12s\n", "domain", "always", "conditional", "on demand")
		for _, domain := range report.Domains {
			w.printf("  %-28s %12s %12s %12s\n", truncate(domain.Name, 28),
				humanCount(domain.Always), humanCount(domain.Conditional), humanCount(domain.OnDemand))
		}
	}

	if report.Budget != nil {
		status := "within budget"
		if report.Budget.Exceeded {
			status = "OVER BUDGET"
		}
		w.printf("\nBudget: %s of %s always-loaded tokens — %s\n",
			humanCount(report.Budget.Actual), humanCount(report.Budget.Limit), status)
	}

	w.printf("\nScope and caveats:\n")
	for _, note := range report.Notes {
		w.printf("  - %s\n", wrapNote(note, 92, "    "))
	}
}

func writeRuntime(w reportWriter, runtime *generator.RuntimeTokens) {
	heading := "runtime " + runtime.Preset
	if runtime.Scope != "" {
		heading += " (scope " + runtime.Scope + ")"
	}
	w.printf("%s — %s files\n", heading, humanCount(runtime.Files))
	writeBucket(w, "always loaded", runtime.Always, runtime.Entries, generator.BucketAlways)
	writeBucket(w, "conditional", runtime.Conditional, runtime.Entries, generator.BucketConditional)
	writeBucket(w, "on demand", runtime.OnDemand, runtime.Entries, generator.BucketOnDemand)
	writeBucket(w, "unmodeled", runtime.Unmodeled, runtime.Entries, generator.BucketUnmodeled)
	w.println()
}

func writeBucket(w reportWriter, label string, total int, entries []generator.Entry, bucket generator.Bucket) {
	matching := make([]generator.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.Bucket == bucket {
			matching = append(matching, entry)
		}
	}
	if len(matching) == 0 {
		return
	}
	w.printf("  %-52s %12s\n", label, humanCount(total))
	for _, entry := range matching {
		writeEntry(w, entry, 1)
	}
}

func writeEntry(w reportWriter, entry generator.Entry, depth int) {
	indent := strings.Repeat("  ", depth)
	label := entry.Label
	if entry.Artifacts > 1 {
		label = fmt.Sprintf("%s (%d)", label, entry.Artifacts)
	}
	width := 52 - len(indent)
	if width < 10 {
		width = 10
	}
	w.printf("  %s%-*s %12s\n", indent, width, truncate(label, width), humanCount(entry.Tokens))
	for _, child := range entry.Children {
		writeEntry(w, child, depth+1)
	}
}

func writeComparisonTable(w reportWriter, reports []*generator.TokenReport) {
	w.printf("Token surface comparison\n")
	w.printf("Tokenizer: %s (approximate)\n\n", reports[0].Tokenizer.Name)
	w.printf("  %-24s %-14s %10s %12s %12s\n", "profile", "runtime", "always", "conditional", "on demand")
	for _, report := range reports {
		for i := range report.Runtimes {
			runtime := &report.Runtimes[i]
			w.printf("  %-24s %-14s %10s %12s %12s\n",
				truncate(report.Profile, 24), truncate(runtime.Preset, 14),
				humanCount(runtime.Always), humanCount(runtime.Conditional), humanCount(runtime.OnDemand))
		}
	}
	w.printf("\n  %-24s %10s  %s\n", "profile", "headline", "largest runtime")
	for _, report := range reports {
		w.printf("  %-24s %10s  %s\n",
			truncate(report.Profile, 24), humanCount(report.HeadlineAlways), report.HeadlinePreset)
	}
	w.printf("\nScope and caveats:\n")
	for _, note := range reports[0].Notes {
		w.printf("  - %s\n", wrapNote(note, 92, "    "))
	}
}

// humanCount formats a token count with thousands separators so a five-figure
// number is readable at a glance.
func humanCount(value int) string {
	negative := value < 0
	if negative {
		value = -value
	}
	digits := fmt.Sprintf("%d", value)
	var groups []string
	for len(digits) > 3 {
		groups = append([]string{digits[len(digits)-3:]}, groups...)
		digits = digits[:len(digits)-3]
	}
	groups = append([]string{digits}, groups...)
	result := strings.Join(groups, ",")
	if negative {
		return "-" + result
	}
	return result
}

func truncate(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	return value[:width-1] + "…"
}

// wrapNote soft-wraps a caveat so the scope statement stays readable in a
// terminal instead of running off the right edge where nobody reads it.
func wrapNote(note string, width int, indent string) string {
	words := strings.Fields(note)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if len(current)+1+len(word) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current += " " + word
	}
	lines = append(lines, current)
	return strings.Join(lines, "\n"+indent)
}

type tokenTarget struct{ profile, role string }

// tokenTargets lists what to report on: the compared profiles, one role, or every role.
func tokenTargets(cfg *config.Config) ([]tokenTarget, error) {
	if (tokensRole != "" || tokensByRole) && (profile != "" || len(tokensCompareProfiles) > 0) {
		return nil, oops.Hint("Use --role or --by-role on its own, or --profile/--compare-profiles").
			Errorf("--role and --by-role cannot be combined with --profile or --compare-profiles")
	}
	if tokensRole != "" && tokensByRole {
		return nil, oops.Errorf("--role and --by-role are mutually exclusive")
	}
	switch {
	case tokensByRole:
		names := cfg.RoleNames()
		if len(names) == 0 {
			return nil, oops.Hint("Declare [[roles]] in config.toml").Errorf("no roles are defined")
		}
		targets := make([]tokenTarget, 0, len(names))
		for _, name := range names {
			targets = append(targets, tokenTarget{role: name})
		}
		return targets, nil
	case tokensRole != "":
		return []tokenTarget{{role: tokensRole}}, nil
	}
	profiles := tokensCompareProfiles
	if len(profiles) == 0 {
		profiles = []string{profile}
	}
	targets := make([]tokenTarget, 0, len(profiles))
	for _, name := range profiles {
		targets = append(targets, tokenTarget{profile: name})
	}
	return targets, nil
}
