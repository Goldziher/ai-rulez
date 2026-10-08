package commands

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rv "github.com/Goldziher/ai-rulez/v5/internal/review"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// RubricCmd groups the rubric tools.
var RubricCmd = &cobra.Command{
	Use:   "rubric",
	Short: "List, show and lint review rubrics",
	Long: `Inspect the rubrics "ai-rulez review" scores against: the embedded builtin:skill-quality
and the ones under .ai-rulez/rubrics/<id>/ (rubric.toml, optional system.md, golden/*.golden.yaml
and calibration.json). See docs/review.md.`,
}

var rubricFormat string

var rubricListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the built-in and project rubrics",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runRubricList(cmd, cmd.OutOrStdout())
	},
}

var rubricShowCmd = &cobra.Command{
	Use:   "show [id]",
	Short: "Print a rubric's dimensions, weights and scoring formula",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRubricShow(cmd, args, cmd.OutOrStdout())
	},
}

var rubricLintCmd = &cobra.Command{
	Use:   "lint [id...]",
	Short: "Check rubrics, golden files and calibration records (AR9G8)",
	Long: `Check the project rubrics (or the named ones, or builtin:<id>): schema version, id, weights summing
to 1, unique dimension ids and codes, twins that are registered lint rules, thresholds in range,
golden cases with two labelers and known dimensions, and calibration.json. A rubric directory or
file that is a symlink is refused. Findings are AR9G8 errors.

Exit status: 0 when clean, 2 when there are findings, 1 when a rubric could not be read.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		found, err := runRubricLint(cmd, args, cmd.OutOrStdout())
		if err != nil {
			return err
		}
		if found {
			os.Exit(exitRubricFindings)
		}
		return nil
	},
}

func runRubricList(cmd *cobra.Command, out io.Writer) error {
	cfg, err := loadConfigForCommand(commandContext(cmd), nil)
	if err != nil {
		return err
	}
	type row struct {
		Ref     string `json:"ref"`
		Version int    `json:"version,omitempty"`
		Digest  string `json:"digest,omitempty"`
		Valid   bool   `json:"valid"`
	}
	var rows []row
	for _, id := range rv.BuiltinIDs() {
		rb, lerr := rv.LoadBuiltin(id)
		if lerr != nil {
			return lerr //nolint:wrapcheck // already contextual
		}
		rows = append(rows, row{Ref: rb.Ref, Version: rb.Version, Digest: rb.Digest, Valid: true})
	}
	ids, err := rv.ListIDs(cfg.ConfigDir)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	for _, id := range ids {
		rb, lerr := rv.Load(cfg.ConfigDir, id)
		if lerr != nil {
			rows = append(rows, row{Ref: id})
			continue
		}
		rows = append(rows, row{Ref: id, Version: rb.Version, Digest: rb.Digest, Valid: true})
	}
	if rubricFormat == formatJSON {
		return writeIndentedJSON(out, rows)
	}
	w := reportWriter{out}
	for _, r := range rows {
		if r.Valid {
			w.printf("%-28s version %d  %s\n", r.Ref, r.Version, r.Digest)
		} else {
			w.printf("%-28s invalid (run: ai-rulez rubric lint %s)\n", r.Ref, r.Ref)
		}
	}
	return nil
}

func runRubricShow(cmd *cobra.Command, args []string, out io.Writer) error {
	cfg, err := loadConfigForCommand(commandContext(cmd), nil)
	if err != nil {
		return err
	}
	ref := ""
	if len(args) > 0 {
		ref = args[0]
	} else if cfg.Review != nil {
		ref = cfg.Review.Rubric
	}
	rb, err := rv.Load(cfg.ConfigDir, ref)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if rubricFormat == formatJSON {
		return writeIndentedJSON(out, rv.NewReport(rb, &rv.Results{}, nil).Rubric)
	}
	w := reportWriter{out}
	w.printf("%s  version %d  %s\napplies to: %s\n%s\n\n", rb.Ref, rb.Version, rb.Digest, strings.Join(rb.AppliesTo, ", "), rv.Formula)
	for i := range rb.Dimensions {
		d := &rb.Dimensions[i]
		w.printf("%-22s %-6s weight %.2f  %-12s ceiling %-7s twins: %s\n  %s\n", d.ID, d.Code, d.Weight, d.Group, d.Severity, orDash(strings.Join(d.Twins, ", ")), d.Question)
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// runRubricLint lints the named rubrics, or every project rubric. found is true when any finding exists.
func runRubricLint(cmd *cobra.Command, args []string, out io.Writer) (found bool, err error) {
	if !contains(rv.Formats(), rubricFormat) {
		return false, oops.Errorf("unknown --format %q", rubricFormat)
	}
	cfg, err := loadConfigForCommand(commandContext(cmd), nil)
	if err != nil {
		return false, err
	}
	refs := args
	if len(refs) == 0 {
		if refs, err = rv.ListIDs(cfg.ConfigDir); err != nil {
			return false, err //nolint:wrapcheck // already contextual
		}
	}
	var problems []rv.Problem
	for _, ref := range refs {
		var ps []rv.Problem
		if id, ok := strings.CutPrefix(ref, config.BuiltinRubricPrefix); ok {
			ps, err = rv.LintBuiltin(id)
		} else {
			ps, err = rv.LintDir(filepath.Join(cfg.ConfigDir, rv.RubricsDir, ref))
		}
		if err != nil {
			return false, oops.Wrapf(err, "lint rubric %s", ref)
		}
		problems = append(problems, ps...)
	}
	sort.SliceStable(problems, func(i, j int) bool { return problems[i].File < problems[j].File })
	rep := &lint.Report{Root: cfg.BaseDir, Findings: rv.Findings(problems, workingDir())}
	if len(refs) == 0 && rubricFormat == formatText {
		reportWriter{out}.printf("no rubrics under %s\n", filepath.Join(cfg.ConfigDir, rv.RubricsDir))
		return false, nil
	}
	if err := lint.Write(out, rubricFormat, lint.Combine([]*lint.Report{rep}), lint.WriteOptions{Version: Version, FailOn: failOnError}); err != nil {
		return false, oops.Wrapf(err, "write report")
	}
	return len(problems) > 0, nil
}
