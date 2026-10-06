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
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var reportEvalsFlags struct {
	usageLog   string
	feedback   string
	results    string
	minPass    float64
	minTrigger float64
	json       bool
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

Without a usage log nothing is concluded about use. The command reports and exits 0.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runReportEvals(cmd.OutOrStdout())
	},
}

func init() {
	f := reportEvalsCmd.Flags()
	f.StringVar(&reportEvalsFlags.usageLog, "usage-log", "", "Usage log (default <config dir>/local/usage.jsonl, when present)")
	f.StringVar(&reportEvalsFlags.feedback, "feedback", "", "Feedback log (default feedback.jsonl beside the usage log, when present)")
	f.StringVar(&reportEvalsFlags.results, "results", "", "Eval results (default <config dir>/eval-results.json)")
	f.Float64Var(&reportEvalsFlags.minPass, "min-pass-rate", evals.DefaultMinPassRate, "Pass rate below which a skill is a rewrite candidate")
	f.Float64Var(&reportEvalsFlags.minTrigger, "min-trigger", evals.DefaultMinTrigger, "Trigger precision and recall below which a skill is a rewrite candidate")
	addJSONFormat(f, &reportEvalsFlags.json, "j")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	ReportCmd.AddCommand(reportEvalsCmd)
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
	store, err := evals.LoadStoreKeyed(resultsPath, evalResultsKey())
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
	if err := joinRankUsage(&in, cfgDir); err != nil {
		return err
	}
	rows := evals.Rank(in)

	if reportEvalsFlags.json {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return oops.Wrapf(encoder.Encode(map[string]any{"schema_version": 1, "usage_log": in.Uses != nil, "skills": rows}), "encode evals report")
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

// joinRankUsage reads the usage and feedback logs into the ranking input. A log
// left at its default location is optional; one named by a flag must exist.
func joinRankUsage(in *evals.RankInput, cfgDir string) error {
	usagePath := reportEvalsFlags.usageLog
	explicit := usagePath != ""
	if !explicit {
		usagePath = filepath.Join(cfgDir, "local", "usage.jsonl")
	}
	if _, err := os.Stat(usagePath); err == nil || explicit {
		entries, _, err := usage.ReadLog(usagePath)
		if err != nil {
			return err
		}
		in.Uses, in.UseDigests = map[string]int{}, map[string]map[string]int{}
		for i := range entries {
			id := entries[i].ID
			in.Uses[id]++
			if in.UseDigests[id] == nil {
				in.UseDigests[id] = map[string]int{}
			}
			// Only a digest in the canonical scheme can join with an eval record's
			// lock_digest; a v2 line, or a served digest, joins by id only.
			digest := ""
			if entries[i].DigestScheme == usage.DigestSchemeSkill {
				digest = entries[i].Digest
			}
			in.UseDigests[id][digest]++
		}
	}
	feedbackPath := reportEvalsFlags.feedback
	explicitFeedback := feedbackPath != ""
	if !explicitFeedback {
		feedbackPath = filepath.Join(filepath.Dir(usagePath), usage.FeedbackFileName)
	}
	if _, err := os.Stat(feedbackPath); err == nil || explicitFeedback {
		entries, _, err := usage.ReadFeedback(feedbackPath)
		if err != nil {
			return err
		}
		in.Feedback = usage.FeedbackCounts(entries)
	}
	return nil
}

func writeEvalsReport(w reportWriter, rows []evals.RankRow, haveUsage bool) {
	w.printf("Skill evals joined with usage: %s\n", evals.SummaryLine(rows))
	if !haveUsage {
		w.printf("No usage log found: nothing is concluded about use (pass --usage-log).\n")
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

// loadEvalSummaries reads the eval results for `report usage`. An empty path means
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
	store, err := evals.LoadStoreKeyed(path, evalResultsKey())
	if err != nil {
		return nil, err
	}
	out := make(map[string]usage.EvalSummary, len(store.Skills))
	for i := range store.Skills {
		r := &store.Skills[i]
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
