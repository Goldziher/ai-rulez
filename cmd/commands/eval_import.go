package commands

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/evals/evalimport"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var evalImportFlags struct {
	from           string
	skill          string
	out            string
	dryRun         bool
	liftAssertions bool
	rubricMode     string
	report         string
	force          bool
	format         string
}

var evalImportCmd = &cobra.Command{
	Use:   "import <path>...",
	Short: "Import eval scenarios from another tool as eval cases",
	Long: `Import scenarios written for another tool as eval case files. The importer is offline:
it reads local files only, never contacts a service, and treats the input as untrusted text.

  --from tessl   a scenario directory (task.md plus criteria.json), a criteria.json file, or a
                 directory of scenario directories. The task becomes the case prompt and the weighted
                 checklist a rubric (--rubric-mode single, weights kept as text) or rubric_items
                 (--rubric-mode items, weights kept); the pass threshold becomes rubric_min_score;
                 fixtures become files. Every field that has no counterpart is listed as unmapped
                 (AR9A5, informational) with a hint where it belongs in ai-rulez.

The cases are written to .ai-rulez/skills/<skill>/evals/ (--skill) or --out, as <id>.eval.yaml with
the task beside it as <id>.task.md. Nothing is written unless every scenario maps, and an existing file
is not overwritten without --force. --lift-assertions also turns criteria in a fixed phrasing ("The file
"x" exists", "The output contains "y"") into deterministic assertions, never dropping the criterion from
the rubric; every lift is listed for review. Text with hidden characters or credentials is refused;
instruction-override phrases are flagged. See docs/evals.md.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runEvalImport,
}

func init() {
	f := evalImportCmd.Flags()
	f.StringVar(&evalImportFlags.from, "from", "", "Format of the input: tessl (required)")
	f.StringVar(&evalImportFlags.skill, "skill", "", "Skill the cases belong to: they are written to its evals/ directory")
	f.StringVar(&evalImportFlags.out, "out", "", "Directory to write the cases to (instead of the skill's evals/ directory)")
	f.BoolVar(&evalImportFlags.dryRun, "dry-run", false, "Map and report; write nothing")
	f.BoolVar(&evalImportFlags.liftAssertions, "lift-assertions", false, "Turn criteria in a fixed phrasing into deterministic assertions (the criterion stays in the rubric)")
	f.StringVar(&evalImportFlags.rubricMode, "rubric-mode", evalimport.RubricSingle, "single: one free-text rubric with the weights as text; items: rubric_items with the weights kept")
	f.StringVar(&evalImportFlags.report, "report", "", "Also write the machine-readable report (JSON) to this file")
	f.BoolVar(&evalImportFlags.force, "force", false, "Overwrite existing files")
	addFormatFlag(f, &evalImportFlags.format, formatText, formatText, formatText, formatJSON)
	addJSONFlagAlias(f)
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	EvalCmd.AddCommand(evalImportCmd)
}

func runEvalImport(cmd *cobra.Command, paths []string) error {
	flags := &evalImportFlags
	if flags.from != evalimport.SourceTessl {
		return oops.Hint("--from tessl is the only supported format").Errorf("unknown or missing --from %q", flags.from)
	}
	switch flags.rubricMode {
	case evalimport.RubricSingle, evalimport.RubricItems:
	default:
		return oops.Errorf("unknown --rubric-mode %q (use %s or %s)", flags.rubricMode, evalimport.RubricSingle, evalimport.RubricItems)
	}
	if flags.format != formatText && flags.format != formatJSON {
		return oops.Errorf("unknown --format %q (use text or json)", flags.format)
	}
	out, err := importOutDir(cmd)
	if err != nil {
		return err
	}
	res, runErr := evalimport.Run(&evalimport.Options{
		Source: evalimport.Tessl{}, Paths: paths, OutDir: out, DryRun: flags.dryRun, Force: flags.force,
		Map: evalimport.MapOptions{RubricMode: flags.rubricMode, LiftAssertions: flags.liftAssertions},
	})
	if runErr != nil && res == nil {
		return oops.Wrapf(runErr, "import scenarios")
	}
	if flags.report != "" {
		if err := writeImportReport(flags.report, res); err != nil {
			return err
		}
	}
	if flags.format == formatJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			return oops.Wrapf(err, "write report")
		}
	} else if err := res.WriteText(cmd.OutOrStdout()); err != nil {
		return oops.Wrapf(err, "write report")
	}
	return oops.Wrapf(runErr, "import scenarios")
}

// importOutDir resolves where the cases go: --out, else the skill's evals directory.
func importOutDir(cmd *cobra.Command) (string, error) {
	flags := &evalImportFlags
	if flags.out != "" {
		return flags.out, nil
	}
	if flags.skill == "" {
		return "", oops.Hint("Name the skill with --skill, or a directory with --out").Errorf("nowhere to write the cases")
	}
	cfg, err := loadConfigForCommand(commandContext(cmd), nil)
	if err != nil {
		return "", err
	}
	absDir, err := filepath.Abs(cfg.ConfigDir)
	if err != nil {
		return "", oops.Wrapf(err, "resolve config directory")
	}
	skills, err := evals.FindSkills(absDir)
	if err != nil {
		return "", oops.Wrapf(err, "list skills")
	}
	for i := range skills {
		if skills[i].ID == flags.skill {
			return filepath.Join(skills[i].Dir, "evals"), nil
		}
	}
	return "", oops.Errorf("unknown skill %q", flags.skill)
}

// writeImportReport writes the JSON report to path.
func writeImportReport(path string, res *evalimport.Result) error {
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "encode report")
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil { //nolint:gosec // a report the user asked for
		return oops.Wrapf(err, "write report")
	}
	return nil
}
