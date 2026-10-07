package commands

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/improve"
)

var improveCleanFlags struct {
	all    bool
	dryRun bool
}

var improveShowCmd = &cobra.Command{
	Use:   "show <run-id>",
	Short: "(experimental) Show a saved improve run: decisions, scores, costs and the diff",
	Long: `Show the report of a saved run under .ai-rulez/local/improve/<run-id>/: every round's decision, the
held-out comparison with its bootstrap interval, the sibling guard, costs and, for an accepted run, the diff.
The optimizer's own text and the candidate's text are printed with control characters replaced, so a
candidate cannot write terminal escapes. It changes nothing. A run whose report is not signed by this machine's
user key is shown with a warning; improve apply and improve pr refuse it.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), improveExperimental)
		if err := checkFormatFlag(improveFlags.format); err != nil {
			return err
		}
		cfg, err := loadConfigForCommand(commandContext(cmd), nil)
		if err != nil {
			return err
		}
		configDirAbs, err := filepath.Abs(cfg.ConfigDir)
		if err != nil {
			return oops.Wrapf(err, "resolve config directory")
		}
		res, err := improve.Show(configDirAbs, args[0])
		if err != nil {
			return oops.Wrap(err)
		}
		if improveFlags.format == formatJSON {
			return writeImproveJSON(cmd.OutOrStdout(), res)
		}
		_, werr := io.WriteString(cmd.OutOrStdout(), res.Text())
		return oops.Wrap(werr)
	},
}

var improveCleanCmd = &cobra.Command{
	Use:   "clean [run-id]",
	Short: "(experimental) Delete saved improve runs",
	Long: `Delete one saved run (by id) or every saved run (--all) under .ai-rulez/local/improve/. A run
holds copies of the skill and its train cases and the optimizer's workspace, so clean them when you are done.
Nothing outside that directory is touched; a run directory that is a symlink is unlinked, never followed.
--dry-run lists what would go; --all asks for confirmation unless --yes is given.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), improveExperimental)
		if err := checkFormatFlag(improveFlags.format); err != nil {
			return err
		}
		opts := &improve.CleanOptions{All: improveCleanFlags.all, DryRun: improveCleanFlags.dryRun}
		if len(args) == 1 {
			opts.RunID = args[0]
		}
		cfg, err := loadConfigForCommand(commandContext(cmd), nil)
		if err != nil {
			return err
		}
		if opts.ConfigDir, err = filepath.Abs(cfg.ConfigDir); err != nil {
			return oops.Wrapf(err, "resolve config directory")
		}
		if opts.All && !opts.DryRun && !improveFlags.yes && !confirmProceed("Delete every saved improve run?") {
			return oops.Hint("Re-run with --yes to skip the prompt").Errorf("not confirmed: nothing was deleted")
		}
		res, err := improve.Clean(commandContext(cmd), opts)
		if err != nil {
			return oops.Wrap(err)
		}
		if improveFlags.format == formatJSON {
			return writeImproveJSON(cmd.OutOrStdout(), res)
		}
		verb := "Removed"
		if res.DryRun {
			verb = "Would remove"
		}
		if len(res.Removed) == 0 {
			_, werr := fmt.Fprintln(cmd.OutOrStdout(), "No saved runs.")
			return oops.Wrap(werr)
		}
		_, werr := fmt.Fprintf(cmd.OutOrStdout(), "%s %d run(s): %s\n", verb, len(res.Removed), strings.Join(res.Removed, ", "))
		return oops.Wrap(werr)
	},
}

func init() {
	for _, c := range []*cobra.Command{improveShowCmd, improveCleanCmd} {
		addFormatFlag(c.Flags(), &improveFlags.format, formatText, formatText, formatText, formatJSON)
		c.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	}
	improveCleanCmd.Flags().BoolVar(&improveCleanFlags.all, "all", false, "Delete every saved run")
	improveCleanCmd.Flags().BoolVar(&improveCleanFlags.dryRun, "dry-run", false, "List the runs that would be deleted; delete nothing")
	improveCleanCmd.Flags().BoolVarP(&improveFlags.yes, "yes", "y", false, "Delete --all without the confirmation prompt")
	ImproveCmd.AddCommand(improveShowCmd, improveCleanCmd)
}
