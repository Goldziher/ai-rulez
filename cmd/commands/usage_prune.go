package commands

import (
	"errors"
	"io"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	usagePruneKeepDays     int
	usagePruneDryRun       bool
	usagePruneIgnoreCursor bool
)

var usagePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete usage-log lines older than N days that were already exported",
	Long: `The usage log only grows. Prune removes the lines whose timestamp is older than --keep-days,
but only those behind the export cursor: events still waiting to be sent are kept however old, so
pruning never costs an export. Without a cursor (telemetry export was never on) age alone decides.
A line with no readable timestamp is kept. The export cursor is moved to match the rewritten log.

  ai-rulez usage prune --keep-days 90
  ai-rulez usage prune --keep-days 30 --dry-run

If the cursor belongs to another log (the file was replaced since the last flush) the prune refuses;
run ` + "`ai-rulez telemetry flush`" + ` first, or pass --ignore-cursor to prune by age alone. The log has no
lock: the prune re-reads a log that grows while it works and gives up (exit 1, nothing changed) if it
keeps changing; a line appended in the last microseconds before the file is replaced can be lost, so
prune from a quiet session.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runUsagePrune(cmd.OutOrStdout(), telemetry.SystemClock())
	},
}

func runUsagePrune(out io.Writer, now time.Time) error {
	if usagePruneKeepDays < 0 {
		return oops.Hint("Pass --keep-days 0 to remove everything already exported.").Errorf("--keep-days must not be negative")
	}
	logPath := usageLog
	if logPath == "" {
		logPath = defaultUsageLogPath()
	}
	root, name := telemetryRoot(""), telemetryConfigDirName()
	cutoff := now.AddDate(0, 0, -usagePruneKeepDays)
	var (
		res usage.PruneResult
		err error
	)
	if same, _ := sameFile(logPath, defaultUsageLogPath()); same || usageLog == "" { //nolint:errcheck // a missing file is not the same file
		// The project's own log: the export cursor protects what is not yet sent.
		spool := &telemetry.Spool{Dir: telemetry.LocalDir(root, name)}
		res, err = spool.PruneLog(logPath, telemetry.PruneOptions{Cutoff: cutoff, DryRun: usagePruneDryRun, IgnoreCursor: usagePruneIgnoreCursor})
	} else {
		// Another log has no cursor of this project: age alone decides.
		res, err = usage.PruneLog(logPath, usage.PruneOptions{Cutoff: cutoff, DryRun: usagePruneDryRun})
	}
	switch {
	case errors.Is(err, usage.ErrLogBusy):
		return oops.Hint("Retry when no harness session is writing.").Wrapf(err, "prune usage log")
	case err != nil:
		return err
	}
	w := reportWriter{out}
	verb := "removed"
	if usagePruneDryRun {
		verb = "would remove"
	}
	w.printf("%s %d of %d lines older than %d days (%d bytes); kept %d", verb, res.Removed, res.Scanned, usagePruneKeepDays, res.RemovedBytes, res.Kept)
	if res.Protected > 0 {
		w.printf(", %d old ones not yet exported", res.Protected)
	}
	if res.Unreadable > 0 {
		w.printf(", %d without a readable timestamp", res.Unreadable)
	}
	w.printf("\n")
	return nil
}

func init() {
	UsageCmd.AddCommand(usagePruneCmd)
	f := usagePruneCmd.Flags()
	f.IntVar(&usagePruneKeepDays, "keep-days", 0, "Keep lines from the last N days (required)")
	f.BoolVar(&usagePruneDryRun, "dry-run", false, "Report what would be removed without rewriting the log")
	f.BoolVar(&usagePruneIgnoreCursor, "ignore-cursor", false, "Prune by age alone, also lines not yet exported")
	f.StringVar(&usageLog, "log", "", "Usage log to prune (default <config dir>/local/usage.jsonl)")
	f.StringVar(&telRoot, "root", "", "Project root (default $CLAUDE_PROJECT_DIR, else the nearest directory holding the config directory)")
	f.StringVarP(&telConfigDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	if err := usagePruneCmd.MarkFlagRequired("keep-days"); err != nil {
		panic(err)
	}
}
