package usage

// EvalSummary is a skill's recorded eval score, as the report shows it. The eval
// package owns the full record; this is the slice `telemetry report` joins.
type EvalSummary struct {
	PassRate         float64  `json:"pass_rate"`
	TriggerPrecision *float64 `json:"trigger_precision"`
	TriggerRecall    *float64 `json:"trigger_recall"`
	AblationDelta    *float64 `json:"ablation_delta"`
	Passing          bool     `json:"passing"`
	Date             string   `json:"date,omitempty"`
}

// Join adds feedback counts and eval scores to the rows of a report. Feedback never
// creates a row: a skill that is in no row is counted only in FeedbackEvents.
func (r *Report) Join(feedback []FeedbackEntry, scores map[string]EvalSummary) {
	counts := FeedbackCounts(feedback)
	r.FeedbackEvents = len(feedback)
	fill := func(rows []SkillUsage) {
		for i := range rows {
			if c := counts[rows[i].ID]; len(c) > 0 {
				rows[i].Feedback = c
			}
			if s, ok := scores[rows[i].ID]; ok {
				summary := s
				rows[i].Eval = &summary
			}
		}
	}
	fill(r.Used)
	fill(r.Never)
	fill(r.Unknown)
}
