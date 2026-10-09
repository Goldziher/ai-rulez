package commands

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/cost"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	costFormat         string
	costTarget         string
	costTop            int
	costBudget         int
	costOnDemandBudget int
)

// CostCmd reports which content items cost the most context.
var CostCmd = &cobra.Command{
	Use:   "cost",
	Short: "Report which skills, rules and context cost the most prompt tokens",
	Long: `Report the context cost of the configured content: what is paid on every request
(always loaded: rule and context bodies, and the name and description of every
listed skill, agent and command) and what is paid only when an item is opened
(on demand: bodies), with the biggest offenders first.

The runtime totals are the ones "ai-rulez tokens" prints for the target; the
per-item table is estimated from the sources. Use "tokens" for the full
per-runtime breakdown and "cost" to find what to trim. --budget and
--on-demand-budget set ceilings; exceeding one exits 2 and names the three
biggest offenders.

Counts are approximations (Claude's tokenizer is not published).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		exceeded, err := runCost(cmd)
		if err != nil {
			return fail(err)
		}
		if exceeded {
			return exitStatus(budgetExceededExitCode)
		}
		return nil
	},
}

func init() {
	f := CostCmd.Flags()
	addFormatFlag(f, &costFormat, formatText, formatText, formatText, formatJSON, "markdown")
	f.StringVar(&costTarget, "target", "", "Preset whose runtime totals to report (default: the runtime with the largest always-loaded surface)")
	f.IntVar(&costTop, "top", 10, "How many top offenders to list")
	f.IntVar(&costBudget, "budget", 0, "Exit 2 when the always-loaded tokens of the target exceed this ceiling")
	f.IntVar(&costOnDemandBudget, "on-demand-budget", 0, "Exit 2 when the on-demand tokens of the target exceed this ceiling")
	specNoLocal.Bool(f, &noLocal, "Ignore the machine-local config.local.* overlay and local/ content")
	specProfile.String(f, &profile, "Profile to report on, or a comma-separated list to compose several")
	f.StringVar(&tokensTokenizer, "tokenizer", tokens.CounterCL100KBase, "Token counter to use: "+strings.Join(tokens.Names(), " or "))
}

func runCost(cmd *cobra.Command) (exceeded bool, err error) {
	switch costFormat {
	case cost.FormatText, cost.FormatJSON, cost.FormatMarkdown:
	default:
		return false, oops.Errorf("unknown --format %q (use text, json or markdown)", costFormat)
	}
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
	report, err := cost.Build(cfg, cost.Options{
		Profile: profile, Target: costTarget, Top: costTop, Counter: counter,
		AlwaysBudget: costBudget, OnDemandBudget: costOnDemandBudget,
	})
	if err != nil {
		return false, err //nolint:wrapcheck // already contextual
	}
	if err := cost.Write(cmd.OutOrStdout(), report, costFormat); err != nil {
		return false, oops.Wrapf(err, "write cost report")
	}
	return report.Exceeded(), nil
}
