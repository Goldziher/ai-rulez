package govview

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
)

// ItemEval is the recorded eval result of a skill, reduced to an allowlist of
// aggregate figures. Case prompts, transcripts, runner and model never enter
// the catalog.
type ItemEval struct {
	Cases    int     `json:"cases"`
	PassRate float64 `json:"pass_rate"`
	// Passing says the run met the pass threshold.
	Passing bool `json:"passing"`
	// AblationDelta is the outcome gain from having the skill; null without ablation data.
	AblationDelta    *float64 `json:"ablation_delta"`
	TriggerPrecision *float64 `json:"trigger_precision"`
	TriggerRecall    *float64 `json:"trigger_recall"`
	// Stale says the skill changed after the run: its lock digest differs from the
	// one the run recorded.
	Stale bool `json:"stale"`
	// Verified says the record carries a valid MAC of this user's key; a record
	// from another machine is shown but unverified.
	Verified bool   `json:"verified"`
	Date     string `json:"date,omitempty"`
}

// ItemUsage is the recorded use of a skill: a count and the last day. Sessions,
// harnesses, outcomes and feedback are not included.
type ItemUsage struct {
	Invocations int    `json:"invocations"`
	LastSeen    string `json:"last_seen,omitempty"`
}

// evalOf reduces a store record to the allowlisted fields.
func evalOf(rec *evals.SkillRecord, itemDigest string) *ItemEval {
	if rec == nil || !rec.HasRun() {
		return nil
	}
	return &ItemEval{
		Cases: rec.Score.Scored, PassRate: rec.Score.PassRate, Passing: rec.Passing,
		AblationDelta: rec.Score.AblationDelta, TriggerPrecision: rec.Score.TriggerPrecision, TriggerRecall: rec.Score.TriggerRecall,
		Stale:    rec.LockDigest != "" && itemDigest != "" && rec.LockDigest != itemDigest,
		Verified: rec.Verified(), Date: rec.Date,
	}
}

// usageTallies counts the skill invocations per skill id and their last day.
func usageTallies(entries []usage.Entry) map[string]*ItemUsage {
	out := map[string]*ItemUsage{}
	for i := range entries {
		e := &entries[i]
		if e.Resource || e.ID == "" {
			continue // a supporting file of a skill already counted
		}
		u := out[e.ID]
		if u == nil {
			u = &ItemUsage{}
			out[e.ID] = u
		}
		u.Invocations++
		day, _, _ := strings.Cut(e.Time, "T")
		if day > u.LastSeen {
			u.LastSeen = day
		}
	}
	return out
}

// attachSignals adds the eval and usage sections to the skills of doc, and says
// in the notes why a requested one is missing. A usage log names skills by id
// only, so an id shared by two skills is left unattributed rather than guessed.
func attachSignals(doc *CatalogDocV2, opts *CatalogOptions) {
	if opts.WithEval {
		if opts.Eval == nil {
			doc.Notes = append(doc.Notes, reasonOr(opts.EvalNote, "eval results were not found")+": eval fields are omitted")
		} else {
			for i := range doc.Items {
				if it := &doc.Items[i]; it.Kind == config.RoleKindSkill {
					rec, _ := opts.Eval.Get(it.ID)
					it.Eval = evalOf(rec, it.Digest)
				}
			}
			doc.Notes = append(doc.Notes, "eval results are shown as recorded; a result without a valid signature from this machine is marked unverified")
		}
	}
	if !opts.WithUsage {
		return
	}
	if !opts.UsageLoaded {
		doc.Notes = append(doc.Notes, reasonOr(opts.UsageNote, "the usage log was not found")+": usage fields are omitted")
		return
	}
	tallies := usageTallies(opts.Usage)
	count := map[string]int{}
	for i := range doc.Items {
		if doc.Items[i].Kind == config.RoleKindSkill {
			count[doc.Items[i].ID]++
		}
	}
	ambiguous := false
	for i := range doc.Items {
		it := &doc.Items[i]
		if it.Kind != config.RoleKindSkill {
			continue
		}
		if count[it.ID] > 1 {
			ambiguous = ambiguous || tallies[it.ID] != nil
			continue
		}
		if u := tallies[it.ID]; u != nil {
			c := *u
			it.Usage = &c
		}
	}
	if ambiguous {
		doc.Notes = append(doc.Notes, "usage of skills that share an id across domains is not attributed: the log names a skill by id only")
	}
}

func reasonOr(reason, fallback string) string {
	if reason != "" {
		return reason
	}
	return fallback
}
