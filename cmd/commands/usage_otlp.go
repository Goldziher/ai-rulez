package commands

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	usageExportWithEvals  bool
	usageExportAll        bool
	usageExportMaxBatches int
	usageExportEvalsFile  string
)

// defaultEvalStorePath is the project's eval results file: under the project root
// and configuration directory --root and --config-dir select, like the rest of the
// telemetry commands, not the working directory.
func defaultEvalStorePath() string {
	return evals.DefaultStorePath(filepath.Join(telemetryRoot(""), telemetryConfigDirName()))
}

// loadEvalEvents turns the project's verified eval results into eval_result
// events. A default results file that does not exist is no events; a named one
// must exist. Unsigned or foreign-signed records and activation-only records are
// not results and are never exported.
func loadEvalEvents(path string) ([]telemetry.Event, error) {
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return nil, oops.With("path", path).Hint("Pass the results file that eval run wrote, or omit --evals.").Wrapf(err, "read eval results")
		}
	} else {
		path = defaultEvalStorePath()
	}
	store, err := evals.LoadStoreKeyed(path, evals.UserKey())
	if err != nil {
		return nil, err
	}
	results := make([]telemetry.EvalResult, 0, len(store.Skills))
	for i := range store.Skills {
		r := &store.Skills[i]
		if !r.HasRun() || !r.Verified() {
			continue
		}
		results = append(results, telemetry.EvalResult{
			SkillID: r.ID, LockDigest: r.LockDigest, Harness: r.Harness, Date: r.Date, PassRate: r.Score.PassRate,
			TriggerPrecision: r.Score.TriggerPrecision, TriggerRecall: r.Score.TriggerRecall, AblationDelta: r.Score.AblationDelta,
		})
	}
	events, _ := telemetry.EvalEvents(results)
	return events, nil
}

// runUsageExportOTLP pushes the usage log past the export cursor (and with
// --with-evals the recorded eval results) to the consented collector. It never
// runs without consent: the same gates as the background flush apply. A delivery
// failure is an error here (exit 1) so CI notices; the background flush stays silent.
func runUsageExportOTLP(out io.Writer) error {
	root, name := telemetryRoot(""), telemetryConfigDirName()
	settings := telemetry.ResolveFor(root, name, nil, activePolicy)
	logPath := usageLog
	if logPath == "" {
		logPath = defaultUsageLogPath()
	}
	if !settings.ExportActive() {
		return oops.Hint("Run `ai-rulez telemetry enable --endpoint URL`, or use --to file for an air-gapped copy.").
			Errorf("OTLP export is not active: %s", strings.Join(settings.ExportBlockers(), "; "))
	}
	if usageExportMaxBatches < 1 {
		return oops.Errorf("--max-batches must be at least 1")
	}
	p := telemetry.Build(settings, telemetry.BuildOptions{Root: root, ConfigDirName: name, Version: Version, LogPath: logPath, Spawn: telemetrySpawn})
	w := reportWriter{out}

	var evalEvents []telemetry.Event
	if usageExportWithEvals {
		var err error
		if evalEvents, err = loadEvalEvents(usageExportEvalsFile); err != nil {
			return err
		}
		if evalEvents, err = p.Spool.Unsent(evalEvents); err != nil {
			return err
		}
	}
	if usageExportDryRun {
		res, err := p.Spool.CatchUp(logPath, telemetry.CatchUpOptions{Sample: settings.Sample, All: usageExportAll, DryRun: true})
		if err != nil {
			return err
		}
		pending, _, err := p.Spool.Pending()
		if err != nil {
			return err
		}
		w.printf("would queue %d events from the usage log (%d already queued or delivered)%s, %d eval results; %d events already wait in the outbox (nothing sent)\n",
			res.Queued, res.Skipped, cursorNote(res), len(evalEvents), len(pending))
		return nil
	}

	var queued, skipped, batches, sent int
	if err := p.Spool.AppendMany(evalEvents); err != nil {
		return err
	}
	for round := 0; round < usageExportMaxBatches; round++ {
		res, err := p.Spool.CatchUp(logPath, telemetry.CatchUpOptions{Sample: settings.Sample, All: usageExportAll && round == 0})
		if err != nil {
			return err
		}
		queued += res.Queued
		skipped += res.Skipped
		if res.Initialized {
			w.printf("export cursor placed at the end of the usage log: events from before export was on are not sent (use --all to send history)\n")
		}
		ctx, cancel := context.WithTimeout(cmdContext(), telemetry.MaxFlushTimeout)
		flushed, err := p.Exporter.Flush(ctx)
		cancel()
		sent += flushed.Sent
		batches += flushed.Batches
		if flushed.Skipped {
			w.printf("another flush is running; this one queued %d events for it\n", res.Queued)
			return nil
		}
		if err != nil {
			return oops.Wrapf(err, "export (queued %d, delivered %d; the rest stays in the outbox)", queued, sent)
		}
		if !res.More {
			break
		}
	}
	w.printf("queued %d events from the usage log and %d eval results (%d already queued or delivered); delivered %d in %d batches\n",
		queued, len(evalEvents), skipped, sent, batches)
	return nil
}

func cursorNote(res telemetry.CatchUpResult) string {
	switch {
	case res.Initialized:
		return ", no cursor yet: a real run places it at the end of the log"
	case res.Reset:
		return ", the log changed since the cursor: reading from the start"
	case res.More:
		return ", more remain beyond one run's bound"
	}
	return ""
}

func addUsageExportOTLPFlags(c *cobra.Command) {
	c.Flags().BoolVar(&usageExportWithEvals, "with-evals", false, "Also export the recorded eval results (eval_result events and gauges); needs verified results")
	c.Flags().StringVar(&usageExportEvalsFile, "evals", "", "Eval results file for --with-evals (default <config dir>/eval-results.json)")
	c.Flags().BoolVar(&usageExportAll, "all", false, "With --to otlp, start from the beginning of the usage log instead of the export cursor")
	c.Flags().IntVar(&usageExportMaxBatches, "max-batches", 10, "With --to otlp, stop after this many catch-up rounds of up to 2000 events")
}
