package lint

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
)

// Codes of the eval runner checks (see docs/evals.md).
const (
	CodeEvalCaseInvalid    = "AR996"
	CodeEvalStale          = "AR997"
	CodeEvalScoreLow       = "AR998"
	CodeEvalResultsInvalid = "AR9A0"
)

// Values of [lint.evals] require_fresh.
const (
	freshWarn  = "warn"
	freshError = "error"
	freshOff   = "off"
)

// The eval checks register themselves so this file is the only place to touch.
func init() {
	registerRules(
		RuleInfo{CodeEvalCaseInvalid, "eval-case-invalid", SeverityError, "an eval case file (`*.eval.yaml`, `*.eval.yml`, `*.eval.json`) is malformed: unknown field, missing expect_trigger or prompt, bad assertion, unsafe path"},
		RuleInfo{CodeEvalStale, "eval-stale", SeverityOff, "a skill changed after its last recorded passing eval run (enabled by lint.evals.require_fresh)"},
		RuleInfo{CodeEvalScoreLow, "eval-score-low", SeverityOff, "a skill's recorded eval pass rate is below lint.evals.min_pass_rate (enabled by setting it)"},
		RuleInfo{CodeEvalResultsInvalid, "eval-results-invalid", SeverityError, ".ai-rulez/eval-results.json cannot be read or has an unsupported schema_version"},
	)
}

// evalSettings applies the [lint.evals] settings to the rule severities.
func (r *runner) evalSettings() {
	if r.lc.Evals == nil {
		return
	}
	switch r.lc.Evals.RequireFresh {
	case freshWarn:
		r.sev[CodeEvalStale] = SeverityWarning
	case freshError:
		r.sev[CodeEvalStale] = SeverityError
	}
	if r.lc.Evals.MinPassRate > 0 {
		r.sev[CodeEvalScoreLow] = SeverityError
	}
	if r.lc.Evals.MinActivationRecall > 0 || r.lc.Evals.MinActivationPrecision > 0 {
		r.sev[CodeActivationLow] = SeverityError
	}
	if r.lc.Evals.ConfusionThreshold > 0 {
		r.sev[CodeSkillConfusable] = SeverityWarning
	}
}

// validateEvalSettings reports invalid [lint.evals] values.
func validateEvalSettings(lc *config.LintConfig) []string {
	if lc.Evals == nil {
		return nil
	}
	var problems []string
	switch lc.Evals.RequireFresh {
	case "", freshOff, freshWarn, freshError:
	default:
		problems = append(problems, fmt.Sprintf("lint.evals.require_fresh: unknown value %q (use warn, error or off)", lc.Evals.RequireFresh))
	}
	if lc.Evals.MinPassRate < 0 || lc.Evals.MinPassRate > 1 {
		problems = append(problems, "lint.evals.min_pass_rate: must be between 0 and 1")
	}
	for _, f := range []struct {
		name string
		v    float64
	}{
		{"min_activation_recall", lc.Evals.MinActivationRecall}, {"min_activation_precision", lc.Evals.MinActivationPrecision},
		{"confusion_threshold", lc.Evals.ConfusionThreshold},
	} {
		if f.v < 0 || f.v > 1 {
			problems = append(problems, "lint.evals."+f.name+": must be between 0 and 1")
		}
	}
	return problems
}

// checkEvalRunner reports malformed eval case files (AR996) and, from the results
// store, skills edited after their last passing run (AR997) and skills scoring
// below the floor (AR998).
func (r *runner) checkEvalRunner() {
	if r.cfg.ConfigDir == "" {
		return
	}
	store, storeErr := evals.LoadStoreKeyed(evals.DefaultStorePath(r.cfg.ConfigDir), evals.ExistingUserKey())
	if storeErr != nil {
		store = nil
		r.add(CodeEvalResultsInvalid, evals.DefaultStorePath(r.cfg.ConfigDir), 1, "%v", storeErr)
	}
	for i := range r.items {
		it := &r.items[i]
		if !it.owned || it.kind != kindSkill || it.itemDir == "" {
			continue
		}
		id := config.SkillID(it.cf)
		skill := evals.Skill{ID: id, Dir: it.itemDir}
		for _, dir := range []string{filepath.Join(it.itemDir, config.SkillKindEvals), filepath.Join(r.cfg.ConfigDir, config.EvalsDirName, id)} {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				skill.EvalDirs = append(skill.EvalDirs, dir)
			}
		}
		r.checkEvalCases(&skill)
		if store != nil {
			r.checkEvalRecord(it, store, id)
		}
	}
}

func (r *runner) checkEvalCases(skill *evals.Skill) {
	if len(skill.EvalDirs) == 0 {
		return
	}
	_, problems := evals.LoadCases(skill)
	for _, p := range problems {
		r.add(CodeEvalCaseInvalid, p.File, p.Line, "skill %q: %s", skill.ID, p.Message)
	}
}

func (r *runner) checkEvalRecord(it *item, store *evals.Store, id string) {
	record, ok := store.Get(id)
	if !ok {
		return
	}
	r.checkActivationRecord(it, record, id)
	if !record.HasRun() {
		return // only an activation measurement: no case run to judge
	}
	if !record.Verified() {
		// A record not signed with this user's key (committed from another
		// machine, hand edited, or forged) is no evidence of a passing run.
		const why = "its recorded eval result is unverified (not signed with your own key, so it may be forged or from another machine) and does not count as passing; re-run `ai-rulez eval run %s`"
		if r.sev[CodeEvalStale] != SeverityOff {
			r.add(CodeEvalStale, it.abs, 1, "skill %q: "+why, id, id)
		}
		if r.minPassRate() > 0 {
			r.add(CodeEvalScoreLow, it.abs, 1, "skill %q: "+why, id, id)
		}
		return
	}
	if r.sev[CodeEvalStale] != SeverityOff {
		digest, err := evals.SkillDigest(it.itemDir)
		if err != nil {
			r.add(CodeEvalStale, it.abs, 1, "skill %q: cannot compute its digest: %v", id, err)
		} else if last, stale := store.Stale(id, digest); stale {
			r.add(CodeEvalStale, it.abs, 1, "skill %q changed after its last passing eval (%s, %s); re-run `ai-rulez eval run %s`",
				id, dateOrUnknown(last.Date), shortDigest(last.Digest), id)
		}
	}
	if floor := r.minPassRate(); floor > 0 && record.Score.Scored > 0 && record.Score.PassRate < floor {
		r.add(CodeEvalScoreLow, it.abs, 1, "skill %q has a recorded eval pass rate of %.0f%%, below lint.evals.min_pass_rate %.0f%%",
			id, record.Score.PassRate*100, floor*100)
	}
}

func (r *runner) minPassRate() float64 {
	if r.lc.Evals == nil {
		return 0
	}
	return r.lc.Evals.MinPassRate
}

func dateOrUnknown(date string) string {
	if date == "" {
		return "undated"
	}
	return date
}

func shortDigest(digest string) string {
	const keep = len("sha256:") + 12
	if len(digest) > keep {
		return digest[:keep]
	}
	return digest
}
