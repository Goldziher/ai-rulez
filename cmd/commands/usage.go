package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
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
	usageExecutable  string
	usageOutput      string
	reportJSON       bool

	usageHarness   string
	usageOutcome   string
	usageRole      string
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

// UsageCmd groups the usage-telemetry commands. Nothing here runs unless a user
// wires the recorder into their harness, and nothing makes a network call.
var UsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Usage telemetry: generate a skill-usage hook and record skill invocations",
	Long: `Support for finding out which generated skills are ever used.

  ai-rulez usage hook      print a Claude Code hooks block that records skill invocations
  ai-rulez usage record    the command that block runs; appends one identifier-only JSON line
  ai-rulez usage feedback  record that a skill misled you, is stale, wrong or great (notes stay local)
  ai-rulez usage export    write the log as an OTLP JSON file for an air-gapped collector or your own tooling
  ai-rulez report usage    join a log with the skills index, feedback and eval scores

Set [usage] skills_index = true so generate writes .ai-rulez/skills-index.json, which gives
every logged skill its content hash. The log holds the skill name, a timestamp, a salted hash
of the session id (never the raw id), the hash, the harness and an outcome. It never holds
prompts, arguments or file contents, and ai-rulez makes no network call; a --sink-command is the only way a line leaves the machine, and only if you
write one.`,
}

var usageHookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Print the hooks block that records skill invocations (claude, codex, cursor)",
	Long: `Print a "hooks" block that runs "ai-rulez usage record" when a skill is loaded. For
--harness claude (the default) it goes in .claude/settings.json or a plugin hooks file and
covers the model calling the Skill tool and the user typing a skill's slash command. For codex
(.codex/hooks.json) and cursor (.cursor/hooks.json) it covers the PreToolUse event, where a
skill load is a read of skills/<name>/SKILL.md; the event names come from ai-rulez's hook
support, but those harnesses' payload fields are inferred, so check the log after wiring it.
Any other harness prints a warning and no template. The block is a template: merge it into
your settings yourself. Nothing is enabled by default.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		template, err := usage.HookTemplate(usage.HookTemplateOptions{
			Executable:  usageExecutable,
			LogPath:     usageLog,
			SinkCommand: usageSinkCommand,
			IndexPath:   usageIndex,
			Harness:     usageHarness,
			Role:        usageRole,
		})
		var unsupported *usage.UnsupportedHarnessError
		if errors.As(err, &unsupported) {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning:", unsupported) //nolint:errcheck // a warning on a closed stderr is not actionable
			return nil
		}
		if err != nil {
			return err
		}
		if usageOutput == "" {
			_, err = cmd.OutOrStdout().Write(template)
			return oops.Wrapf(err, "write hook template")
		}
		if err := os.MkdirAll(filepath.Dir(usageOutput), 0o750); err != nil {
			return oops.Wrapf(err, "create output directory")
		}
		return oops.Wrapf(os.WriteFile(usageOutput, template, 0o600), "write hook template")
	},
}

var usageRecordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record one skill invocation from a hook event on stdin",
	Long: `Read one hook event from standard input and, when it is a skill invocation, append a
JSON line (ts, skill, id, hash, session, invocation) to the log. Any other event is ignored.
For codex and cursor only a read of skills/<id>/SKILL.md counts (a read tool, or cat, head, sed -n and
similar); writes, git add and other mentions of the path do not. The payload is inferred, not verified.
The command never fails a session: errors are reported on standard error and the exit status
stays 0.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		if err := runUsageRecord(cmd.InOrStdin()); err != nil {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "ai-rulez usage record:", err) //nolint:errcheck // a hook must not fail on a closed stderr
		}
	},
}

func runUsageRecord(in io.Reader) error {
	logPath := usageLog
	if logPath == "" && usageSinkCommand == "" {
		root := os.Getenv("CLAUDE_PROJECT_DIR")
		if root == "" {
			root, _ = os.Getwd() //nolint:errcheck // an empty root falls back to a relative path
		}
		logPath = filepath.Join(root, ".ai-rulez", "local", "usage.jsonl")
	}
	entry, err := usage.Record(in, usage.RecordOptions{
		LogPath:     logPath,
		SinkCommand: usageSinkCommand,
		IndexPath:   usageIndex,
		Harness:     usageHarness,
		Outcome:     usageOutcome,
		Role:        usageRole,
		Served:      usageServed,
		SaltPath:    usageSalt,
	})
	emitUsageTelemetry(entry)
	return err
}

var usageExportCmd = &cobra.Command{
	Use:   "export [path]",
	Short: "Write the usage log as an OTLP JSON file (--to file)",
	Long: `Write the events of the usage log to a file, one OTLP logs request per line, the format the
OpenTelemetry Collector's otlpjsonfile receiver reads. Nothing is sent over a network, and the
file needs no consent: it is a local copy you move yourself.

  ai-rulez usage export --to file usage.ndjson
  ai-rulez usage export --to file --file usage.ndjson --log other/usage.jsonl

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
		return oops.Hint("Use `ai-rulez telemetry flush` to send the outbox to a collector.").Errorf("--to otlp is not available in `usage export`")
	default:
		return oops.Hint("Use --to file and a destination path.").Errorf("--to must be file, got %q", usageExportTo)
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
		logPath = filepath.Join(defaultLocalDir(), "usage.jsonl")
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
	settings := telemetry.ResolveFor(telemetryRoot(""), telemetryConfigDirName(), nil)
	encoder := settings.Encoder(Version)
	file, err := encoder.EncodeFile(read.Events)
	if err != nil {
		return err
	}
	w := reportWriter{out}
	if usageExportDryRun {
		w.printf("would write %d events in %d batches to %s (nothing written)\n", file.Events, file.Batches, dest)
	} else {
		if err := safefs.EnsureParent(dest); err != nil {
			return oops.Wrapf(err, "prepare destination")
		}
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

var usageFeedbackCmd = &cobra.Command{
	Use:   "feedback <skill>",
	Short: "Record that a skill misled you, is stale, wrong or great",
	Long: `Append one identifier-only feedback record for a skill to the feedback log (default
.ai-rulez/local/feedback.jsonl, machine-local): the skill, its current hash, the kind and a
timestamp. With --note-file the text of a note is copied to feedback-notes/ next to the log
(mode 0600) and only the note's file name is recorded. Notes stay on this machine: they are
not in the log line, the skills index, the eval results or any hash.

  ai-rulez usage feedback deploy-staging --kind stale --note-file ./why.txt`,
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
			LogPath: logPath, IndexPath: indexPath, NoteFile: feedbackNote, Harness: usageHarness, Role: usageRole,
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Recorded %s feedback for %s in %s\n", entry.Kind, entry.ID, logPath)
		return oops.Wrapf(err, "write confirmation")
	},
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

// ReportCmd groups report commands.
var ReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Reports over data ai-rulez collected",
}

var reportUsageCmd = &cobra.Command{
	Use:   "usage <log>",
	Short: "List never-used skills and skills edited since they were used",
	Long: `Join a usage log (written by "ai-rulez usage record") with the skills index and list:

  used          skills with at least one logged invocation, most used first
  never used    indexed skills with no logged invocation: candidates for retirement
  changed       skills whose logged content hash differs from the current one, so the
                evidence may describe an older version
  unknown       logged skills that are not in the index (renamed, removed, external)

The command reports and exits 0; it does not gate anything.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReportUsage(cmd.OutOrStdout(), args[0])
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
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return oops.Wrapf(encoder.Encode(usageReportJSON{Report: report, Items: items}), "encode usage report")
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
	UsageCmd.AddCommand(usageHookCmd, usageRecordCmd, usageFeedbackCmd, usageExportCmd)
	usageExportCmd.Flags().StringVar(&usageExportTo, "to", "", "Destination kind: file (required)")
	usageExportCmd.Flags().StringVar(&usageExportFile, "file", "", "Destination path (or pass it as the argument)")
	usageExportCmd.Flags().StringVar(&usageLog, "log", "", "Usage log to export (default <config dir>/local/usage.jsonl)")
	usageExportCmd.Flags().BoolVar(&usageExportDryRun, "dry-run", false, "Encode the log and report the result without writing the file")
	usageExportCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	if err := usageExportCmd.MarkFlagRequired("to"); err != nil {
		panic(err)
	}
	for _, c := range []*cobra.Command{usageHookCmd, usageRecordCmd} {
		c.Flags().StringVar(&usageLog, "log", "", "Usage log file to append to (default .ai-rulez/local/usage.jsonl)")
		c.Flags().StringVar(&usageSinkCommand, "sink-command", "", "Shell command that receives each log line on stdin")
		c.Flags().StringVar(&usageIndex, "index", "", "Skills index used to resolve content hashes")
	}
	for _, c := range []*cobra.Command{usageHookCmd, usageRecordCmd, usageFeedbackCmd} {
		c.Flags().StringVar(&usageHarness, "harness", "", "Harness: claude, codex or cursor (default claude; feedback records it as given)")
		c.Flags().StringVar(&usageRole, flagRole, "", "Role active when the skill loaded, a role of [[roles]] (recorded as given)")
	}
	usageRecordCmd.Flags().StringVar(&usageOutcome, "outcome", "", "Outcome to record: loaded (default), used or abandoned")
	usageRecordCmd.Flags().BoolVar(&usageServed, "served", false, "Mark the load as served by the MCP server")
	usageRecordCmd.Flags().StringVar(&usageSalt, "salt-file", "", "File holding the session-hash salt (default usage.salt beside the log; $AI_RULEZ_USAGE_SALT wins)")
	usageFeedbackCmd.Flags().StringVar(&feedbackKind, "kind", "", "Feedback kind: "+strings.Join(usage.FeedbackKinds, ", ")+" (required)")
	usageFeedbackCmd.Flags().StringVar(&feedbackNote, "note-file", "", "File whose text is kept as a local note (never logged or hashed)")
	usageFeedbackCmd.Flags().StringVar(&usageLog, "log", "", "Feedback log to append to (default .ai-rulez/local/feedback.jsonl)")
	usageFeedbackCmd.Flags().StringVar(&usageIndex, "index", "", "Skills index used to resolve the current hash")
	usageFeedbackCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	if err := usageFeedbackCmd.MarkFlagRequired("kind"); err != nil {
		panic(err)
	}
	usageHookCmd.Flags().StringVar(&usageExecutable, "executable", "ai-rulez", "Command the hook runs")
	usageHookCmd.Flags().StringVarP(&usageOutput, "output", "o", "", "Write the template to this file instead of stdout")

	ReportCmd.AddCommand(reportUsageCmd)
	reportUsageCmd.Flags().StringVar(&usageIndex, "index", "", "Skills index to join against (default <config dir>/skills-index.json)")
	reportUsageCmd.Flags().StringVar(&reportFeedback, "feedback", "", "Feedback log to join (default feedback.jsonl beside the usage log, when present)")
	reportUsageCmd.Flags().StringVar(&reportEvals, "evals", "", "Eval results to join (default <config dir>/eval-results.json, when present)")
	reportUsageCmd.Flags().BoolVarP(&reportJSON, "json", "j", false, "Emit the report as JSON")
	reportUsageCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}
