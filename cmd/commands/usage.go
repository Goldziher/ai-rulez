package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	usageLog         string
	usageSinkCommand string
	usageIndex       string
	reportJSON       bool

	usageOutcome   string
	usageServed    bool
	usageSalt      string
	feedbackKind   string
	feedbackNote   string
	reportFeedback string
	reportEvals    string

	usageExportTo     string
	usageExportFile   string
	usageExportDryRun bool
)

func runUsageRecord(in io.Reader) error {
	logPath := usageLog
	if logPath == "" && usageSinkCommand == "" {
		root := os.Getenv("CLAUDE_PROJECT_DIR")
		if root == "" {
			root, _ = os.Getwd() //nolint:errcheck // an empty root falls back to a relative path
		}
		logPath = filepath.Join(root, ".ai-rulez", "local", "usage.jsonl")
	}
	saltPath := usageSalt
	if saltPath == "" {
		// Hash sessions with the same salt the telemetry pipeline uses, so skill
		// events and rule/agent events of one session agree.
		saltPath = telemetry.ResolveFor(telemetryRoot(""), telemetryConfigDirName(), nil, activePolicy).SaltFile
	}
	entry, err := usage.Record(in, usage.RecordOptions{
		LogPath:     logPath,
		SinkCommand: usageSinkCommand,
		IndexPath:   usageIndex,
		Harness:     telHarness,
		Outcome:     usageOutcome,
		Role:        telRole,
		Served:      usageServed,
		SaltPath:    saltPath,
	})
	emitUsageTelemetry(entry)
	return err
}

var telemetryExportCmd = &cobra.Command{
	Use:   "export [path]",
	Short: "Write the usage log as an OTLP JSON file, or push it to the collector",
	Long: `--to file writes the events of the usage log to a file, one OTLP logs request per line, the
format the OpenTelemetry Collector's otlpjsonfile receiver reads. Nothing is sent over a network, and
the file needs no consent: it is a local copy you move yourself.

  ai-rulez telemetry export --to file usage.ndjson
  ai-rulez telemetry export --to file --file usage.ndjson --log other/usage.jsonl
  ai-rulez telemetry export --to file --with-evals usage.ndjson

--to otlp pushes the log past the export cursor to the collector you consented to (see
"telemetry enable"): it queues what the outbox does not hold, sends it with retry, and moves the
cursor. It refuses without consent and exits 1 when delivery fails, so CI notices (the background
flush stays silent). --all starts at the beginning of the log; --dry-run counts without sending.
--with-evals adds one eval_result event per verified eval result and the eval gauges.

Only allowlisted, identifier-only fields are written (see "ai-rulez telemetry preview" for the
list and for the exact bytes): unlisted keys in a log line are dropped, raw version 1 session
ids are never exported, and the session and path fields stay out unless include_session and
include_paths are on in your user configuration. Skill lines and rule, agent and context events
are both exported, in log order, each once per event_id. The output is deterministic: the same
log gives the same file, which is replaced atomically. --dry-run reads and encodes the log and
reports what would be written without creating the file.

Logs only: the receiver reads one signal per file, and counts can be derived from the log records.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUsageExport(cmd.OutOrStdout(), args)
	},
}

func runUsageExport(out io.Writer, args []string) error {
	switch usageExportTo {
	case "file":
	case "otlp":
		if len(args) > 0 || usageExportFile != "" {
			return oops.Errorf("--to otlp sends to the consented collector; a destination path belongs to --to file")
		}
		return runUsageExportOTLP(out)
	default:
		return oops.Hint("Use --to file and a destination path, or --to otlp.").Errorf("--to must be file or otlp, got %q", usageExportTo)
	}
	dest := usageExportFile
	if len(args) == 1 {
		if dest != "" && dest != args[0] {
			return oops.Errorf("give the destination once: as an argument or --file, not both")
		}
		dest = args[0]
	}
	if dest == "" {
		return oops.Hint("Pass the destination: `usage export --to file out.ndjson`.").Errorf("a destination path is required")
	}
	logPath := usageLog
	if logPath == "" {
		logPath = defaultUsageLogPath()
	}
	if same, err := sameFile(logPath, dest); err != nil {
		return err
	} else if same {
		return oops.Errorf("the destination %s is the usage log; choose another path", dest)
	}
	read, err := telemetry.ReadLogEvents(logPath)
	if err != nil {
		return err
	}
	events := read.Events
	if usageExportWithEvals {
		evalEvents, err := loadEvalEvents(usageExportEvalsFile)
		if err != nil {
			return err
		}
		events = append(events[:len(events):len(events)], evalEvents...)
	}
	settings := telemetry.ResolveFor(telemetryRoot(""), telemetryConfigDirName(), nil, activePolicy)
	encoder := settings.Encoder(Version)
	file, err := encoder.EncodeFile(events)
	if err != nil {
		return err
	}
	w := reportWriter{out}
	if usageExportDryRun {
		w.printf("would write %d events in %d batches to %s (nothing written)\n", file.Events, file.Batches, dest)
	} else {
		if err := safefs.WriteFileAtomic(dest, file.Data); err != nil {
			return oops.Wrapf(err, "write %s", dest)
		}
		w.printf("wrote %d events in %d batches to %s\n", file.Events, file.Batches, dest)
	}
	if read.Rejected > 0 {
		w.printf("left out %d lines that failed validation\n", read.Rejected)
	}
	return nil
}

// sameFile reports whether two paths name the same existing file.
func sameFile(a, b string) (bool, error) {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false, nil //nolint:nilerr // a missing file cannot be the same file
	}
	return os.SameFile(infoA, infoB), nil
}

var telemetryFeedbackCmd = &cobra.Command{
	Use:   "feedback <skill>",
	Short: "Record that a skill misled you, is stale, wrong or great",
	Long: `Append one identifier-only feedback record for a skill to the feedback log (default
.ai-rulez/local/feedback.jsonl, machine-local): the skill, its current hash, the kind and a
timestamp. With --note-file the text of a note is copied to feedback-notes/ next to the log
(mode 0600) and only the note's file name is recorded. Notes stay on this machine: they are
not in the log line, the skills index, the eval results or any hash.

  ai-rulez telemetry feedback deploy-staging --kind stale --note-file ./why.txt`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logPath := usageLog
		if logPath == "" {
			logPath = filepath.Join(defaultLocalDir(), usage.FeedbackFileName)
		}
		indexPath := usageIndex
		if indexPath == "" {
			indexPath = usage.DefaultIndexPath(".", configDirName())
		}
		entry, err := usage.RecordFeedback(args[0], feedbackKind, usage.FeedbackOptions{
			LogPath: logPath, IndexPath: indexPath, NoteFile: feedbackNote, Harness: telHarness, Role: telRole,
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Recorded %s feedback for %s in %s\n", entry.Kind, entry.ID, logPath)
		return oops.Wrapf(err, "write confirmation")
	},
}

// defaultUsageLogPath is the usage log `telemetry export` and `telemetry preview` read
// by default: the same project root and config directory resolution for both.
func defaultUsageLogPath() string {
	return filepath.Join(telemetry.LocalDir(telemetryRoot(""), telemetryConfigDirName()), "usage.jsonl")
}

// defaultLocalDir is the machine-local directory for logs: the Claude project
// directory when set, else the working directory.
func defaultLocalDir() string {
	root := os.Getenv("CLAUDE_PROJECT_DIR")
	if root == "" {
		root, _ = os.Getwd() //nolint:errcheck // an empty root falls back to a relative path
	}
	return filepath.Join(root, configDirName(), "local")
}

func configDirName() string {
	if configDir != "" {
		return configDir
	}
	return defaultConfigDirName
}

var telemetryReportCmd = &cobra.Command{
	Use:   "report [log]",
	Short: "List never-used skills and skills edited since they were used",
	Long: `Join a usage log (written by "ai-rulez telemetry record") with the skills index and list:

  used          skills with at least one logged invocation, most used first
  never used    indexed skills with no logged invocation: candidates for retirement
  changed       skills whose logged content hash differs from the current one, so the
                evidence may describe an older version
  unknown       logged skills that are not in the index (renamed, removed, external)

The command reports and exits 0; it does not gate anything. Without a log argument it reads
<config dir>/local/usage.jsonl. "telemetry report evals" ranks skills by eval scores joined with usage.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logPath := filepath.Join(defaultLocalDir(), "usage.jsonl")
		if len(args) == 1 {
			logPath = args[0]
		}
		return runReportUsage(cmd.OutOrStdout(), logPath)
	},
}

func runReportUsage(out io.Writer, logPath string) error {
	indexPath := usageIndex
	if indexPath == "" {
		name := configDir
		if name == "" {
			name = ".ai-rulez"
		}
		indexPath = usage.DefaultIndexPath(".", name)
	}
	index, err := usage.LoadIndex(indexPath)
	if err != nil {
		return err
	}
	entries, skipped, err := usage.ReadLog(logPath)
	if err != nil {
		return err
	}
	report := usage.BuildReport(index, entries, skipped)
	if err := joinUsageExtras(report, logPath); err != nil {
		return err
	}

	items, err := itemsSection(logPath)
	if err != nil {
		return err
	}
	if reportJSON {
		return jsondoc.Write(out, usageReportJSON{Report: report, Items: items})
	}
	writeUsageReport(reportWriter{out}, report)
	if items != nil {
		writeItemsReport(reportWriter{out}, items)
	}
	return nil
}

// joinUsageExtras adds feedback counts and eval scores. The feedback log defaults
// to feedback.jsonl beside the usage log and the eval results to the project's
// eval-results.json; a default that does not exist is skipped, a named one is an error.
func joinUsageExtras(report *usage.Report, logPath string) error {
	var feedback []usage.FeedbackEntry
	feedbackPath := reportFeedback
	if feedbackPath == "" {
		feedbackPath = filepath.Join(filepath.Dir(logPath), usage.FeedbackFileName)
	}
	if _, statErr := os.Stat(feedbackPath); statErr == nil || reportFeedback != "" {
		var err error
		if feedback, _, err = usage.ReadFeedback(feedbackPath); err != nil {
			return err
		}
	}
	scores, err := loadEvalSummaries(reportEvals)
	if err != nil {
		return err
	}
	report.Join(feedback, scores)
	return nil
}

func writeUsageReport(w reportWriter, report *usage.Report) {
	w.printf("Skill usage: %d events", report.Events)
	if report.Skipped > 0 {
		w.printf(" (%d unreadable lines skipped)", report.Skipped)
	}
	w.printf("\n")

	section := func(title string, rows []usage.SkillUsage, counts bool) {
		w.printf("\n%s (%d)\n", title, len(rows))
		for _, row := range rows {
			label := row.ID
			if row.Domain != "" {
				label += " [" + row.Domain + "]"
			}
			switch {
			case counts:
				w.printf("  %-40s %6d  last %s", truncate(label, 40), row.Count, row.LastSeen)
			case row.Owner != "":
				w.printf("  %-40s owner %s", truncate(label, 40), row.Owner)
			default:
				w.printf("  %s", label)
			}
			w.printf("%s\n", usageExtras(row))
		}
	}
	section("Used", report.Used, true)
	section("Never used", report.Never, false)
	w.printf("\nChanged since used (%d)\n", len(report.Changed))
	for _, changed := range report.Changed {
		w.printf("  %-40s logged %s, now %s\n", truncate(changed.ID, 40), strings.Join(changed.LoggedHash, ", "), changed.CurrentHash)
	}
	section("Not in the index", report.Unknown, true)
}

// usageExtras renders the feedback counts and eval score of a row.
func usageExtras(row usage.SkillUsage) string {
	var out string
	if text := usage.FeedbackText(row.Feedback); text != "" {
		out += "  feedback: " + text
	}
	if row.Eval != nil {
		out += "  eval: " + evals.Percent(&row.Eval.PassRate)
		if !row.Eval.Passing {
			out += " (failing)"
		}
	}
	return out
}

func init() {
	TelemetryCmd.AddCommand(telemetryFeedbackCmd, telemetryExportCmd, telemetryReportCmd)
	telemetryExportCmd.Flags().StringVar(&usageExportTo, "to", "", "Destination kind: file or otlp (required)")
	addUsageExportOTLPFlags(telemetryExportCmd)
	telemetryExportCmd.Flags().StringVar(&usageExportFile, "file", "", "Destination path (or pass it as the argument)")
	telemetryExportCmd.Flags().StringVar(&usageLog, "log", "", "Usage log to export (default <config dir>/local/usage.jsonl)")
	telemetryExportCmd.Flags().BoolVar(&usageExportDryRun, "dry-run", false, "Encode the log and report the result without writing the file (--to otlp: without queueing or sending)")
	telemetryExportCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	if err := telemetryExportCmd.MarkFlagRequired("to"); err != nil {
		panic(err)
	}
	telemetryFeedbackCmd.Flags().StringVar(&telHarness, "harness", "", "Harness the feedback is about (recorded as given)")
	telemetryFeedbackCmd.Flags().StringVar(&telRole, flagRole, "", "Role active when the skill loaded, a role of [[roles]] (recorded as given)")
	telemetryFeedbackCmd.Flags().StringVar(&feedbackKind, "kind", "", "Feedback kind: "+strings.Join(usage.FeedbackKinds, ", ")+" (required)")
	telemetryFeedbackCmd.Flags().StringVar(&feedbackNote, "note-file", "", "File whose text is kept as a local note (never logged or hashed)")
	telemetryFeedbackCmd.Flags().StringVar(&usageLog, "log", "", "Feedback log to append to (default .ai-rulez/local/feedback.jsonl)")
	telemetryFeedbackCmd.Flags().StringVar(&usageIndex, "index", "", "Skills index used to resolve the current hash")
	telemetryFeedbackCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	if err := telemetryFeedbackCmd.MarkFlagRequired("kind"); err != nil {
		panic(err)
	}

	telemetryReportCmd.Flags().StringVar(&usageIndex, "index", "", "Skills index to join against (default <config dir>/skills-index.json)")
	telemetryReportCmd.Flags().StringVar(&reportFeedback, "feedback", "", "Feedback log to join (default feedback.jsonl beside the usage log, when present)")
	telemetryReportCmd.Flags().StringVar(&reportEvals, "evals", "", "Eval results to join (default <config dir>/eval-results.json, when present)")
	addJSONFormat(telemetryReportCmd.Flags(), &reportJSON, "")
	telemetryReportCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}
