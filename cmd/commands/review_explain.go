package commands

import (
	"strconv"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	rv "github.com/Goldziher/ai-rulez/v5/internal/review"
)

var reviewExplainCmd = &cobra.Command{
	Use:   "explain CODE",
	Short: "Explain a review code (AR9G0-AR9G9): what it means and how to fix it",
	Long: `Print what a review code means, why it matters, and a bad and a good example. For a dimension code
(AR9G1-AR9G7) it also prints the question the judge answers and the pass, warn and fail definitions of
the rubric in use (--rubric, else [review] rubric, else builtin:skill-quality).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReviewExplain(cmd, args[0])
	},
}

func init() {
	reviewExplainCmd.Flags().StringVar(&reviewFlags.rubric, "rubric", "", "Rubric whose definitions to print (default [review] rubric, else builtin:skill-quality)")
	reviewExplainCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

func runReviewExplain(cmd *cobra.Command, code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !strings.HasPrefix(code, "AR9G") {
		return oops.Hint("review codes are AR9G0 to AR9G9; use `ai-rulez validate --explain` for the others").Errorf("%q is not a review code", code)
	}
	e, ok := lint.Explain(code)
	if !ok {
		return oops.Errorf("unknown review code %q", code)
	}
	var sb strings.Builder
	lint.WriteExplanation(&sb, e)
	if rb := explainRubric(cmd); rb != nil {
		for _, d := range rb.Dimensions {
			if d.Code != code {
				continue
			}
			sb.WriteString("\nDimension " + d.ID + " of rubric " + rb.Ref + "@" + strconv.Itoa(rb.Version) + "\n")
			sb.WriteString("  question: " + d.Question + "\n  pass:     " + d.Pass + "\n  warn:     " + d.Warn + "\n  fail:     " + d.Fail + "\n")
			if len(d.Twins) > 0 {
				sb.WriteString("  lint twins: " + strings.Join(d.Twins, ", ") + " (an error on one of them answers the dimension before the judge is asked)\n")
			}
			if d.NeedsBody {
				sb.WriteString("  needs the body: judged only with --content full\n")
			}
		}
	}
	_, err := cmd.OutOrStdout().Write([]byte(sb.String()))
	return oops.Wrapf(err, "write explanation")
}

// explainRubric loads the rubric to print definitions from; nil when it cannot be loaded (the
// explanation of the code itself still prints).
func explainRubric(cmd *cobra.Command) *rv.Rubric {
	cfg, err := loadConfigForCommand(commandContext(cmd), nil)
	if err != nil {
		rb, _ := rv.Load("", reviewFlags.rubric) //nolint:errcheck // built-in fallback
		return rb
	}
	ref := reviewFlags.rubric
	if ref == "" && cfg.Review != nil {
		ref = cfg.Review.Rubric
	}
	rb, err := rv.Load(cfg.ConfigDir, ref)
	if err != nil {
		return nil
	}
	return rb
}
