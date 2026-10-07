package lint

import (
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
)

// Codes of the activation checks (docs/evals.md#activation-mode). They judge the
// activation record `ai-rulez eval run --mode activation` stores in
// .ai-rulez/eval-results.json; both are off until a [lint.evals] threshold is set.
const (
	CodeActivationLow   = "AR9A1"
	CodeSkillConfusable = "AR9A2"
)

func registerActivationcodes(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeActivationLow, "activation-low", SeverityOff, "a skill's recorded activation recall or precision is below lint.evals.min_activation_recall or min_activation_precision (enabled by setting either)"},
		RuleInfo{CodeSkillConfusable, "skill-confusable", SeverityOff, "a sibling skill won at least lint.evals.confusion_threshold of this skill's positive activation prompts (enabled by setting it)"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeActivationLow: {
			Why:  "A skill that does not fire for the prompts it exists for, or fires for prompts it should not, is invisible or noisy no matter how good its body is.",
			Bad:  "A skill whose recorded activation recall is 50% with `min_activation_recall = 0.8`",
			Good: "Reword the description, triggers and keywords, then re-run `ai-rulez eval run --mode activation --surface retrieval`",
		},
		CodeSkillConfusable: {
			Why:  "When a sibling ranks first for a share of a skill's own prompts, the agent loads the wrong skill and the right one never gets its turn.",
			Bad:  "A deploy skill whose prompts a release-notes skill wins a third of the time with `confusion_threshold = 0.25`",
			Good: "Separate the two descriptions, or merge the skills",
		},
	})
	SetAnalyzer(CodeActivationLow, AnalyzerEvals, ScopeItem)
	SetAnalyzer(CodeSkillConfusable, AnalyzerEvals, ScopeItem)
}

// checkActivationRecord reports a skill's recorded activation figures against the
// [lint.evals] thresholds. A record that is not signed with this user's key, or
// that was measured on a different skill or competing set than the one on disk,
// is not evidence either way and is skipped.
func (r *runner) checkActivationRecord(it *item, record *evals.SkillRecord, id string) {
	act := record.Activation
	if act == nil || r.lc.Evals == nil {
		return
	}
	lowOn, confusableOn := r.sev[CodeActivationLow] != SeverityOff, r.sev[CodeSkillConfusable] != SeverityOff
	if !lowOn && !confusableOn {
		return
	}
	if !record.Verified() {
		const why = "its recorded activation result is unverified (not signed with your own key) and is ignored; re-run `ai-rulez eval run --mode activation --surface %s %s`"
		if lowOn {
			r.add(CodeActivationLow, it.abs, 1, "skill %q: "+why, id, act.Surface, id)
		}
		return
	}
	if digest, err := evals.SkillDigest(it.itemDir); err != nil || digest != act.Digest {
		return
	}
	cfg := r.lc.Evals
	if lowOn {
		if floor := cfg.MinActivationRecall; floor > 0 && act.Recall != nil && *act.Recall < floor {
			r.add(CodeActivationLow, it.abs, 1, "skill %q has a recorded activation recall of %.0f%%, below lint.evals.min_activation_recall %.0f%%",
				id, *act.Recall*100, floor*100)
		}
		if floor := cfg.MinActivationPrecision; floor > 0 && act.Precision != nil && *act.Precision < floor {
			r.add(CodeActivationLow, it.abs, 1, "skill %q has a recorded activation precision of %.0f%%, below lint.evals.min_activation_precision %.0f%%",
				id, *act.Precision*100, floor*100)
		}
	}
	if confusableOn {
		for _, st := range act.StolenBy {
			if st.Share >= cfg.ConfusionThreshold {
				r.add(CodeSkillConfusable, it.abs, 1, "skill %q: %q won %d of its positive activation prompts (%.0f%%, lint.evals.confusion_threshold %.0f%%)",
					id, st.Skill, st.Prompts, st.Share*100, cfg.ConfusionThreshold*100)
			}
		}
	}
}
