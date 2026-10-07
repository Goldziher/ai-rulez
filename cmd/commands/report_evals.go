package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var reportEvalsFlags struct {
	// usageLogs and usageLogsAlias hold --usage and --usage-log; both repeat and
	// are read together (a flag set cannot share one slice between two flags).
	usageLogs      []string
	usageLogsAlias []string
	fromOTLP       bool
	feedback       string
	results        string
	minPass        float64
	minTrigger     float64
	json           bool
}

// evalsUsagePaths is every usage log named by --usage and --usage-log, in order.
func evalsUsagePaths() []string {
	return append(append([]string(nil), reportEvalsFlags.usageLogs...), reportEvalsFlags.usageLogsAlias...)
}

var reportEvalsCmd = &cobra.Command{
	Use:   "evals",
	Short: "Rank skills to prune or rewrite from eval scores joined with usage",
	Long: `Join the recorded eval scores (.ai-rulez/eval-results.json) with the usage log and
feedback log and recommend an action per skill:

  rewrite  pass rate, trigger precision or recall below its floor, a negative ablation delta,
           edited since its last passing eval, or bad feedback outweighing "great"
  prune    never used in the log and evals do not show it helping
  review   unused but evals show value, or no eval results yet
  keep     everything else

With a usage log each row also says how the uses join with the eval record:

  exact    a use was logged at the skill digest the eval ran on (record lock_digest)
  stale    uses carry a digest, none the evaluated one: the evidence is about another version
  legacy   no canonical digest on the record or the uses (older logs, older results): by id only
  none     the skill has no logged use, or no eval record

A record that is unsigned or signed with another key (committed from another machine, or
hand-edited) is marked "unverified" and ignored: the skill counts as having no eval results.
"ai-rulez eval run" re-runs and signs it.

--usage may repeat (--usage-log is the same flag): the logs of several repositories or machines are
merged and an event that appears in more than one counts once, by its event id. --from-otlp reads every
--usage file as OTLP JSON (what "usage export --to file" writes or a collector's file exporter produces)
instead of a native usage log; the digest scheme travels in the export, so the join classes are the same.

Without a usage log nothing is concluded about use. The command reports and exits 0.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runReportEvals(cmd.OutOrStdout())
	},
}

func init() {
	f := reportEvalsCmd.Flags()
	f.StringArrayVar(&reportEvalsFlags.usageLogs, "usage", nil, "Usage log; repeat for several (default <config dir>/local/usage.jsonl, when present)")
	f.StringArrayVar(&reportEvalsFlags.usageLogsAlias, "usage-log", nil, "Same as --usage")
	f.BoolVar(&reportEvalsFlags.fromOTLP, "from-otlp", false, "Read every --usage file as OTLP JSON (usage export --to file output) instead of a native log")
	f.StringVar(&reportEvalsFlags.feedback, "feedback", "", "Feedback log (default feedback.jsonl beside the first usage log, when present)")
	f.StringVar(&reportEvalsFlags.results, "results", "", "Eval results (default <config dir>/eval-results.json)")
	f.Float64Var(&reportEvalsFlags.minPass, "min-pass-rate", evals.DefaultMinPassRate, "Pass rate below which a skill is a rewrite candidate")
	f.Float64Var(&reportEvalsFlags.minTrigger, "min-trigger", evals.DefaultMinTrigger, "Trigger precision and recall below which a skill is a rewrite candidate")
	addJSONFormat(f, &reportEvalsFlags.json, "j")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	telemetryReportCmd.AddCommand(reportEvalsCmd)
}

func runReportEvals(out io.Writer) error {
	cfgDir, err := filepath.Abs(configDirName())
	if err != nil {
		return oops.Wrapf(err, "resolve config directory")
	}
	resultsPath := reportEvalsFlags.results
	if resultsPath == "" {
		resultsPath = evals.DefaultStorePath(cfgDir)
	}
	store, err := evals.LoadStoreKeyed(resultsPath, evals.UserKey())
	if err != nil {
		return err
	}
	skills, err := evals.FindSkills(cfgDir)
	if err != nil {
		return err
	}
	counter, err := tokens.New("")
	if err != nil {
		return oops.Wrapf(err, "token counter")
	}
	in := evals.RankInput{Store: store, MinPassRate: reportEvalsFlags.minPass, MinTrigger: reportEvalsFlags.minTrigger}
	for i := range skills {
		rs := rankSkillFor(skills[i].ID, skills[i].Dir, counter)
		in.Skills = append(in.Skills, rs)
	}
	src, err := joinRankUsage(&in, cfgDir)
	if err != nil {
		return err
	}
	rows := evals.Rank(in)

	if reportEvalsFlags.json {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		doc := map[string]any{"schema_version": 1, "usage_log": in.Uses != nil, "skills": rows}
		if src.logs > 0 {
			doc["usage_sources"] = map[string]int{"logs": src.logs, "events": src.events, "duplicates": src.duplicates}
		}
		return oops.Wrapf(encoder.Encode(doc), "encode evals report")
	}
	if src.logs > 1 || src.duplicates > 0 {
		reportWriter{out}.printf("Usage: %d logs, %d events (%d duplicates removed by event id)\n", src.logs, src.events, src.duplicates)
	}
	writeEvalsReport(reportWriter{out}, rows, in.Uses != nil)
	return nil
}

// rankSkillFor measures one skill for the ranking. What cannot be measured is
// recorded as a note on the skill, not dropped: a skill without a digest is never
// reported stale, and the reader should know why.
func rankSkillFor(id, dir string, counter tokens.Counter) evals.RankSkill {
	rs := evals.RankSkill{ID: id}
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md")) //nolint:gosec // the project's own skill
	if err != nil {
		rs.Notes = append(rs.Notes, "could not read SKILL.md, so its token cost is unknown: "+err.Error())
	} else {
		rs.SkillTokens = counter.Count(string(data))
	}
	digest, err := evals.SkillDigest(dir)
	if err != nil {
		rs.Notes = append(rs.Notes, "could not digest the skill, so staleness was not checked: "+err.Error())
	} else {
		rs.Digest = digest
	}
	return rs
}

// usageSources says what the usage input held, for the report header.
type usageSources struct{ logs, events, duplicates int }

// joinRankUsage reads the usage and feedback logs into the ranking input. A log
// left at its default location is optional; one named by a flag must exist.
// Several logs are merged by event id, so a repository log and its export, or two
// machines' copies, count each event once.
func joinRankUsage(in *evals.RankInput, cfgDir string) (usageSources, error) {
	var src usageSources
	paths := evalsUsagePaths()
	explicit := len(paths) > 0
	if !explicit {
		paths = []string{filepath.Join(cfgDir, "local", "usage.jsonl")}
	}
	var logs [][]usage.Entry
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil && !explicit {
			continue // the default log is optional
		}
		entries, err := readUsageEvidence(path, reportEvalsFlags.fromOTLP)
		if err != nil {
			return src, err
		}
		logs = append(logs, entries)
		src.logs++
	}
	if src.logs > 0 {
		merged, duplicates := usage.MergeEntries(logs...)
		src.events, src.duplicates = len(merged), duplicates
		in.Uses, in.UseDigests = map[string]int{}, map[string]map[string]int{}
		for i := range merged {
			if merged[i].Resource || merged[i].ID == telemetry.ListID {
				continue // a supporting file of a skill, or a listing older releases logged as a skill, is not a use
			}
			id := merged[i].ID
			in.Uses[id]++
			if in.UseDigests[id] == nil {
				in.UseDigests[id] = map[string]int{}
			}
			// Only a digest in the canonical scheme can join with an eval record's
			// lock_digest; a v2 line, or a served digest, joins by id only.
			digest := ""
			if merged[i].DigestScheme == usage.DigestSchemeSkill {
				digest = merged[i].Digest
			}
			in.UseDigests[id][digest]++
		}
	}
	feedbackPath := reportEvalsFlags.feedback
	explicitFeedback := feedbackPath != ""
	if !explicitFeedback {
		feedbackPath = filepath.Join(filepath.Dir(paths[0]), usage.FeedbackFileName)
	}
	if _, err := os.Stat(feedbackPath); err == nil || explicitFeedback {
		entries, _, err := usage.ReadFeedback(feedbackPath)
		if err != nil {
			return src, err
		}
		in.Feedback = usage.FeedbackCounts(entries)
	}
	return src, nil
}

// readUsageEvidence reads one usage input: a native usage log, or with fromOTLP
// an OTLP JSON file.
func readUsageEvidence(path string, fromOTLP bool) ([]usage.Entry, error) {
	if fromOTLP {
		read, err := telemetry.ReadOTLPFile(path)
		return read.Entries, err
	}
	entries, _, err := usage.ReadLog(path)
	return entries, err
}

func writeEvalsReport(w reportWriter, rows []evals.RankRow, haveUsage bool) {
	w.printf("Skill evals joined with usage: %s\n", evals.SummaryLine(rows))
	if !haveUsage {
		w.printf("No usage log found: nothing is concluded about use (pass --usage).\n")
	}
	current := ""
	for i := range rows {
		row := &rows[i]
		if row.Action != current {
			current = row.Action
			w.printf("\n%s\n", strings.ToUpper(current[:1])+current[1:])
		}
		w.printf("  %-36s pass %-5s prec %-5s recall %-5s delta %-9s", truncate(row.ID, 36),
			evals.Percent(row.PassRate), evals.Percent(row.TriggerPrecision), evals.Percent(row.TriggerRecall), rankDelta(row.AblationDelta))
		if row.Uses != nil {
			w.printf(" used %d", *row.Uses)
			if row.Join != "" {
				w.printf(" join %s", row.Join)
			}
		}
		w.printf("\n")
		for _, reason := range row.Reasons {
			w.printf("      - %s\n", reason)
		}
		for _, note := range row.Notes {
			w.printf("      note: %s\n", note)
		}
	}
}

func rankDelta(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%+.0f pts", *v*100)
}

// loadEvalSummaries reads the eval results for `telemetry report`. An empty path means
// the project's default file, which is optional.
func loadEvalSummaries(path string) (map[string]usage.EvalSummary, error) {
	explicit := path != ""
	if !explicit {
		abs, err := filepath.Abs(configDirName())
		if err != nil {
			return nil, oops.Wrapf(err, "resolve config directory")
		}
		path = evals.DefaultStorePath(abs)
	}
	if _, err := os.Stat(path); err != nil && !explicit {
		return nil, nil //nolint:nilerr // the default results file is optional
	}
	store, err := evals.LoadStoreKeyed(path, evals.UserKey())
	if err != nil {
		return nil, err
	}
	out := make(map[string]usage.EvalSummary, len(store.Skills))
	for i := range store.Skills {
		r := &store.Skills[i]
		if !r.HasRun() {
			continue // an activation measurement alone is no eval result
		}
		if !r.Verified() {
			continue // unsigned or foreign-signed records are never reported as results
		}
		out[r.ID] = usage.EvalSummary{
			PassRate: r.Score.PassRate, TriggerPrecision: r.Score.TriggerPrecision, TriggerRecall: r.Score.TriggerRecall,
			AblationDelta: r.Score.AblationDelta, Passing: r.Passing, Date: r.Date,
		}
	}
	return out, nil
}
