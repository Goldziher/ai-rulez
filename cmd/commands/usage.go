package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
)

// UsageCmd groups the usage-telemetry commands. Nothing here runs unless a user
// wires the recorder into their harness, and nothing makes a network call.
var UsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Usage telemetry: generate a skill-usage hook and record skill invocations",
	Long: `Support for finding out which generated skills are ever used.

  ai-rulez usage hook      print a Claude Code hooks block that records skill invocations
  ai-rulez usage record    the command that block runs; appends one identifier-only JSON line
  ai-rulez report usage    join a log with the skills index

Set [usage] skills_index = true so generate writes .ai-rulez/skills-index.json, which gives
every logged skill its content hash. The log holds the skill name, a timestamp, the session
id and the hash. It never holds prompts, arguments or file contents, and ai-rulez makes no
network call; a --sink-command is the only way a line leaves the machine, and only if you
write one.`,
}

var usageHookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Print the Claude Code hooks block that records skill invocations",
	Long: `Print a "hooks" block for .claude/settings.json (or a plugin hooks file) that runs
"ai-rulez usage record" when the model calls the Skill tool and when the user types a skill's
slash command. The block is a template: merge it into your settings yourself. Nothing is
enabled by default.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		template, err := usage.HookTemplate(usage.HookTemplateOptions{
			Executable:  usageExecutable,
			LogPath:     usageLog,
			SinkCommand: usageSinkCommand,
			IndexPath:   usageIndex,
		})
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
	_, err := usage.Record(in, usage.RecordOptions{
		LogPath:     logPath,
		SinkCommand: usageSinkCommand,
		IndexPath:   usageIndex,
	})
	return err
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

	if reportJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return oops.Wrapf(encoder.Encode(report), "encode usage report")
	}
	writeUsageReport(reportWriter{out}, report)
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
				w.printf("  %-40s %6d  last %s\n", truncate(label, 40), row.Count, row.LastSeen)
			case row.Owner != "":
				w.printf("  %-40s owner %s\n", truncate(label, 40), row.Owner)
			default:
				w.printf("  %s\n", label)
			}
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

func init() {
	UsageCmd.AddCommand(usageHookCmd, usageRecordCmd)
	for _, c := range []*cobra.Command{usageHookCmd, usageRecordCmd} {
		c.Flags().StringVar(&usageLog, "log", "", "Usage log file to append to (default .ai-rulez/local/usage.jsonl)")
		c.Flags().StringVar(&usageSinkCommand, "sink-command", "", "Shell command that receives each log line on stdin")
		c.Flags().StringVar(&usageIndex, "index", "", "Skills index used to resolve content hashes")
	}
	usageHookCmd.Flags().StringVar(&usageExecutable, "executable", "ai-rulez", "Command the hook runs")
	usageHookCmd.Flags().StringVarP(&usageOutput, "output", "o", "", "Write the template to this file instead of stdout")

	ReportCmd.AddCommand(reportUsageCmd)
	reportUsageCmd.Flags().StringVar(&usageIndex, "index", "", "Skills index to join against (default <config dir>/skills-index.json)")
	reportUsageCmd.Flags().BoolVarP(&reportJSON, "json", "j", false, "Emit the report as JSON")
	reportUsageCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}
