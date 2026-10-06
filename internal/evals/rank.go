package evals

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Actions a ranking recommends, in display order.
const (
	ActionRewrite = "rewrite"
	ActionPrune   = "prune"
	ActionReview  = "review"
	ActionKeep    = "keep"
)

// Defaults of the ranking thresholds.
const (
	DefaultMinPassRate = 0.8
	DefaultMinTrigger  = 0.8
	// MinUsefulDelta is the ablation delta under which a skill is not shown to help.
	MinUsefulDelta = 0.05
	// MinAblationCases is how many with/without case pairs an ablation delta needs
	// before it counts as evidence: one flaky case is a swing of 100 points.
	MinAblationCases = 3
)

// RankSkill is what the ranking knows about one skill.
type RankSkill struct {
	ID          string
	SkillTokens int
	// Digest is the skill's current content digest, used to detect staleness.
	Digest string
	// Notes are things the caller could not compute for this skill (a digest, a
	// token count); they are shown with the row, never silently dropped.
	Notes []string
}

// RankInput joins eval results with usage data.
type RankInput struct {
	Skills []RankSkill
	Store  *Store
	// Uses is the invocation count per skill id; nil means no usage log was given,
	// so "never used" cannot be concluded.
	Uses map[string]int
	// Feedback counts feedback kinds per skill id.
	Feedback map[string]map[string]int
	// UseDigests counts, per skill id, the logged uses by canonical skill digest
	// (scheme "ai-rulez/skill/v1"); the empty key counts uses logged without one.
	// nil means the log carried no digests, so every use joins as legacy.
	UseDigests  map[string]map[string]int
	MinPassRate float64
	MinTrigger  float64
}

// RankRow is one skill's recommendation.
type RankRow struct {
	ID               string         `json:"id"`
	Action           string         `json:"action"`
	Reasons          []string       `json:"reasons"`
	Uses             *int           `json:"uses"`
	PassRate         *float64       `json:"pass_rate"`
	TriggerPrecision *float64       `json:"trigger_precision"`
	TriggerRecall    *float64       `json:"trigger_recall"`
	AblationDelta    *float64       `json:"ablation_delta"`
	AblationCases    int            `json:"ablation_cases,omitempty"`
	SkillTokens      int            `json:"skill_tokens"`
	Feedback         map[string]int `json:"feedback,omitempty"`
	Stale            bool           `json:"stale,omitempty"`
	// Unverified says the skill has an eval record that is unsigned or signed by
	// another key. Its numbers are not used: it counts as "no eval results".
	Unverified bool `json:"unverified,omitempty"`
	// Join says how far the usage evidence is tied to the evaluated skill: see
	// the Join* constants. Empty when no usage log was given.
	Join string `json:"join,omitempty"`
	// JoinUses counts the uses behind Join by class.
	JoinUses map[string]int `json:"join_uses,omitempty"`
	// Notes list inputs that could not be computed for the skill, so a missing
	// staleness check or token count is visible.
	Notes []string `json:"notes,omitempty"`
}

// Join classes of usage evidence against an eval record.
const (
	// JoinExact: a use was logged at the skill digest the eval record covers.
	JoinExact = "exact"
	// JoinStale: uses carry a digest, none the one the eval ran on, so the
	// evidence describes another version of the skill.
	JoinStale = "stale"
	// JoinLegacy: no canonical digest on the eval record or on the uses, so the
	// two match by skill id only.
	JoinLegacy = "legacy"
	// JoinNone: nothing to join, a skill without logged uses or without an eval record.
	JoinNone = "none"
)

// joinClass classifies the uses of a skill against its eval record.
func joinClass(in *RankInput, id string, record *SkillRecord) (string, map[string]int) {
	uses := in.Uses[id]
	if uses == 0 || record == nil {
		return JoinNone, nil
	}
	counts := map[string]int{}
	if record.LockDigest == "" || in.UseDigests == nil {
		counts[JoinLegacy] = uses
		return JoinLegacy, counts
	}
	digests := in.UseDigests[id]
	exact, legacy := digests[record.LockDigest], digests[""]
	counts[JoinExact], counts[JoinLegacy] = exact, legacy
	counts[JoinStale] = max(uses-exact-legacy, 0)
	for class, n := range counts {
		if n == 0 {
			delete(counts, class)
		}
	}
	switch {
	case exact > 0:
		return JoinExact, counts
	case counts[JoinStale] > 0:
		return JoinStale, counts
	}
	return JoinLegacy, counts
}

// Rank recommends, per skill, whether to rewrite, prune, review or keep it. The
// rules are fixed and documented in docs/evals.md:
//
//	rewrite  the recorded pass rate, trigger precision or recall is below its floor,
//	         the ablation delta is negative, the skill changed after its last passing
//	         eval, or bad feedback (misled, wrong, stale) outweighs "great".
//	prune    not rewrite, a usage log was given, the skill was never used, and evals
//	         do not show it helping (no record, or ablation delta <= 5 points).
//	review   not rewrite or prune, but unused while evals show value, or no eval record.
//	keep     everything else.
//
// Rows are ordered by action, then by the number of reasons, then by pass rate
// (worst first), then by skill tokens (largest first), then by id.
func Rank(in RankInput) []RankRow {
	if in.MinPassRate <= 0 {
		in.MinPassRate = DefaultMinPassRate
	}
	if in.MinTrigger <= 0 {
		in.MinTrigger = DefaultMinTrigger
	}
	rows := make([]RankRow, 0, len(in.Skills))
	for _, skill := range in.Skills {
		rows = append(rows, rankOne(&in, skill))
	}
	order := map[string]int{ActionRewrite: 0, ActionPrune: 1, ActionReview: 2, ActionKeep: 3}
	sort.SliceStable(rows, func(a, b int) bool {
		x, y := &rows[a], &rows[b]
		switch {
		case order[x.Action] != order[y.Action]:
			return order[x.Action] < order[y.Action]
		case len(x.Reasons) != len(y.Reasons):
			return len(x.Reasons) > len(y.Reasons)
		case rateOrInf(x.PassRate) != rateOrInf(y.PassRate):
			return rateOrInf(x.PassRate) < rateOrInf(y.PassRate)
		case x.SkillTokens != y.SkillTokens:
			return x.SkillTokens > y.SkillTokens
		}
		return x.ID < y.ID
	})
	return rows
}

func rateOrInf(v *float64) float64 {
	if v == nil {
		return 2
	}
	return *v
}

func rankOne(in *RankInput, skill RankSkill) RankRow {
	row := RankRow{ID: skill.ID, SkillTokens: skill.SkillTokens, Feedback: in.Feedback[skill.ID], Notes: skill.Notes}
	var record *SkillRecord
	if in.Store != nil {
		record, _ = in.Store.Get(skill.ID)
	}
	if record != nil && !record.Verified() {
		row.Unverified = true
		row.Notes = append(slices.Clone(row.Notes), "the eval record is unverified (unsigned, or signed with another key): ignored; run `ai-rulez eval run` to record a signed result")
		record = nil
	}
	if record != nil && !record.HasRun() {
		record = nil // only an activation measurement: there is no eval result to rank on
	}
	if in.Uses != nil {
		n := in.Uses[skill.ID]
		row.Uses = &n
		row.Join, row.JoinUses = joinClass(in, skill.ID, record)
	}
	if record != nil {
		sc := record.Score
		row.PassRate, row.TriggerPrecision, row.TriggerRecall, row.AblationDelta = &sc.PassRate, sc.TriggerPrecision, sc.TriggerRecall, sc.AblationDelta
		row.AblationCases = sc.AblationCases
	}
	if reasons := rewriteReasons(in, skill, record, &row); len(reasons) > 0 {
		row.Action, row.Reasons = ActionRewrite, reasons
		return row
	}

	// "Never used" needs a log that recorded something, or an empty or wrong log
	// would mark every skill for pruning.
	unused := row.Uses != nil && *row.Uses == 0 && totalUses(in.Uses) > 0
	showsValue := record != nil && reliableDelta(&record.Score) && *row.AblationDelta > MinUsefulDelta
	switch {
	case unused && !showsValue:
		row.Action = ActionPrune
		row.Reasons = pruneReasons(&row, record != nil)
	case unused:
		row.Action = ActionReview
		row.Reasons = []string{"never used in the usage log, but evals show a benefit (" + deltaText(row.AblationDelta) + "): check that it triggers in practice"}
	case record == nil:
		row.Action = ActionReview
		row.Reasons = []string{"no eval results recorded: add cases and run `ai-rulez eval run`"}
	default:
		row.Action = ActionKeep
		row.Reasons = []string{}
	}
	return row
}

// reliableDelta says whether the score carries an ablation delta measured over
// enough cases to act on.
func reliableDelta(sc *SkillScore) bool {
	return sc.AblationDelta != nil && sc.AblationCases >= MinAblationCases
}

func totalUses(uses map[string]int) int {
	total := 0
	for _, n := range uses {
		total += n
	}
	return total
}

func pruneReasons(row *RankRow, hasRecord bool) []string {
	reasons := []string{"never used in the usage log"}
	if !hasRecord || row.AblationDelta == nil || row.AblationCases < MinAblationCases {
		reasons = append(reasons, "no ablation evidence that it helps")
	} else {
		reasons = append(reasons, "ablation delta "+deltaText(row.AblationDelta)+" shows no benefit")
	}
	if row.SkillTokens > 0 {
		reasons = append(reasons, fmt.Sprintf("costs about %d tokens when loaded", row.SkillTokens))
	}
	return reasons
}

// rewriteReasons lists why a skill should be rewritten; it sets row.Stale.
func rewriteReasons(in *RankInput, skill RankSkill, record *SkillRecord, row *RankRow) []string {
	var reasons []string
	if record != nil {
		sc := &record.Score
		if sc.Scored > 0 && sc.PassRate < in.MinPassRate {
			reasons = append(reasons, fmt.Sprintf("pass rate %s is below %s", Percent(&sc.PassRate), pct(in.MinPassRate)))
		}
		if sc.TriggerPrecision != nil && *sc.TriggerPrecision < in.MinTrigger {
			reasons = append(reasons, fmt.Sprintf("trigger precision %s is below %s: it fires when it should not", Percent(sc.TriggerPrecision), pct(in.MinTrigger)))
		}
		if sc.TriggerRecall != nil && *sc.TriggerRecall < in.MinTrigger {
			reasons = append(reasons, fmt.Sprintf("trigger recall %s is below %s: it misses prompts it should handle", Percent(sc.TriggerRecall), pct(in.MinTrigger)))
		}
		if reliableDelta(sc) && *sc.AblationDelta < 0 {
			reasons = append(reasons, fmt.Sprintf("ablation delta %s: outcomes are worse with the skill", deltaText(sc.AblationDelta)))
		}
		if last, stale := in.Store.Stale(skill.ID, skill.Digest); stale && skill.Digest != "" {
			row.Stale = true
			reasons = append(reasons, "changed after its last passing eval ("+dateOrUndated(last.Date)+")")
		}
	}
	bad := row.Feedback["misled"] + row.Feedback["wrong"] + row.Feedback["stale"]
	if bad > 0 && bad > row.Feedback["great"] {
		reasons = append(reasons, fmt.Sprintf("feedback: %d misled/wrong/stale against %d great", bad, row.Feedback["great"]))
	}
	return reasons
}

func pct(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }

func dateOrUndated(date string) string {
	if date == "" {
		return "undated"
	}
	return date
}

// SummaryLine counts rows per action, for the report header.
func SummaryLine(rows []RankRow) string {
	counts := map[string]int{}
	for i := range rows {
		counts[rows[i].Action]++
	}
	var parts []string
	for _, action := range []string{ActionRewrite, ActionPrune, ActionReview, ActionKeep} {
		parts = append(parts, fmt.Sprintf("%d %s", counts[action], action))
	}
	return strings.Join(parts, ", ")
}
