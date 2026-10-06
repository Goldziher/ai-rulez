package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
)

// Codes of the activation checks that read the eval cases themselves, not a
// recorded result (docs/evals.md#activation-mode).
const (
	CodeActivationPolicyConflict = "AR9A3"
	CodeActivationPromptNames    = "AR9A4"
	// CodeEvalImportUnmapped is reported by `ai-rulez eval import`, never by validate.
	CodeEvalImportUnmapped = "AR9A5"
)

// Frontmatter keys that stop the model from loading a skill on its own: Claude
// Code's disable-model-invocation (true) and Codex's allow_implicit_invocation
// (false). The underscore spelling of the first is a harness trap (AR9C*) but is
// still read here, so the conflict is reported next to it.
const (
	keyDisableModelUnderscore = "disable_model_invocation"
	keyAllowImplicit          = "allow_implicit_invocation"
)

func init() {
	registerRules(
		RuleInfo{CodeActivationPolicyConflict, "activation-policy-conflict", SeverityWarning, "an eval case expects a skill to trigger although the skill's frontmatter stops the model from invoking it (disable-model-invocation: true or allow_implicit_invocation: false)"},
		RuleInfo{CodeActivationPromptNames, "activation-prompt-names-skill", SeverityOff, "a positive eval prompt contains the skill's name, so it tests an explicit invocation, not whether the model chooses the skill (off by default; enable it in [lint.severity])"},
		RuleInfo{CodeEvalImportUnmapped, "eval-import-unmapped", SeverityInfo, "fields of an imported eval scenario that have no counterpart in the case format (reported by `ai-rulez eval import`, never by `validate`)"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeActivationPolicyConflict: {
			Why:  "A case that expects a trigger for a skill the model is not allowed to start can never pass, so the eval measures nothing and fails for a reason no edit to the description fixes.",
			Bad:  "A case with `expect_trigger: true` for a skill with `disable-model-invocation: true`",
			Good: "Drop the case (or make it a negative one), or allow model invocation in the skill",
		},
		CodeActivationPromptNames: {
			Why:  "A prompt that names the skill (\"use deploy-staging to ...\") fires it by explicit invocation. The activation rate then measures that the model can follow a name, not that the description makes it choose the skill.",
			Bad:  "`prompt: Use the deploy-staging skill to ship billing` for the skill `deploy-staging`",
			Good: "`prompt: Ship the billing service to staging`",
		},
		CodeEvalImportUnmapped: {
			Why:  "An importer that drops what it cannot map without saying so makes an imported eval look complete. The report lists every input field that was not imported and where it belongs in ai-rulez.",
			Bad:  "A scenario whose `baseline` and `repeats` fields vanished on import",
			Good: "`$.baseline` and `$.repeats` listed as unmapped, with `eval run --ablation` and `--runs N` as the places they belong",
		},
	})
	SetAnalyzer(CodeActivationPolicyConflict, AnalyzerEvals, ScopeItem)
	SetAnalyzer(CodeActivationPromptNames, AnalyzerEvals, ScopeItem)
	SetAnalyzer(CodeEvalImportUnmapped, AnalyzerEvals, ScopeItem)
	registerItemCheck(checkActivationCases, AnalyzerEvals)
}

// checkActivationCases reads the skill's authored eval cases against its
// frontmatter: AR9A3 for a positive case the skill's invocation policy rules out,
// AR9A4 for a positive prompt that names the skill. Cases that do not load are
// AR996's business.
func checkActivationCases(r *runner, it *item, _ doc, fm frontmatter) {
	if r.cfg.ConfigDir == "" || !it.owned || it.isDoc || it.kind != kindSkill || it.itemDir == "" {
		return
	}
	conflictOn, namesOn := r.sev[CodeActivationPolicyConflict] != SeverityOff, r.sev[CodeActivationPromptNames] != SeverityOff
	if !conflictOn && !namesOn {
		return
	}
	id := config.SkillID(it.cf)
	skill := evals.Skill{ID: id, Dir: it.itemDir}
	for _, dir := range []string{filepath.Join(it.itemDir, config.SkillKindEvals), filepath.Join(r.cfg.ConfigDir, config.EvalsDirName, id)} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			skill.EvalDirs = append(skill.EvalDirs, dir)
		}
	}
	if len(skill.EvalDirs) == 0 {
		return
	}
	cases, _ := evals.LoadCases(&skill)
	why := invocationBlocked(fm)
	names := skillNameMatcher(id, it.cf)
	for i := range cases {
		c := &cases[i]
		if !c.Expects() {
			continue
		}
		if conflictOn && why != "" {
			r.add(CodeActivationPolicyConflict, c.File, c.Line, "skill %q: case %q expects a trigger (expect_trigger: true), but the skill sets %s, so the model never starts it and the case can never pass", id, c.ID, why)
		}
		if namesOn && names != nil && names.MatchString(c.Prompt) {
			r.add(CodeActivationPromptNames, c.File, c.Line, "skill %q: the prompt of case %q names the skill, so it tests an explicit invocation, not whether the model chooses it", id, c.ID)
		}
	}
}

// invocationBlocked returns the frontmatter setting that stops the model from
// loading the skill by itself, or "".
func invocationBlocked(fm frontmatter) string {
	for _, name := range []string{keyDisableModel, keyDisableModelUnderscore} {
		if k, ok := fm.top(name); ok && isTrue(k.Value) {
			return name + ": true"
		}
	}
	if k, ok := fm.top(keyAllowImplicit); ok && isFalse(k.Value) {
		return keyAllowImplicit + ": false"
	}
	return ""
}

func isTrue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	}
	return false
}

func isFalse(v any) bool {
	switch t := v.(type) {
	case bool:
		return !t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "false")
	}
	return false
}

// skillNameMatcher matches the skill's id or frontmatter name as a whole word,
// case-insensitively (so "deploy-staging" in "use deploy-staging now" matches, and
// "deploy" in "deployment" does not).
func skillNameMatcher(id string, cf config.ContentFile) *regexp.Regexp {
	names := []string{id}
	if n := strings.TrimSpace(cf.Name); n != "" && n != id {
		names = append(names, n)
	}
	var alts []string
	for _, n := range names {
		if n != "" {
			alts = append(alts, regexp.QuoteMeta(n))
		}
	}
	if len(alts) == 0 {
		return nil
	}
	re, err := regexp.Compile(fmt.Sprintf(`(?i)(?:^|[^a-z0-9_-])(?:%s)(?:$|[^a-z0-9_-])`, strings.Join(alts, "|")))
	if err != nil {
		return nil
	}
	return re
}
