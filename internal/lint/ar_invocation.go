package lint

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// CodeAutoInvocation reports items the model can start on its own that then run
// with far more authority than the user granted for that one request.
const CodeAutoInvocation = "AR013"

func registerArInvocation(s *ruleSet) {
	s.addRules(RuleInfo{CodeAutoInvocation, "auto-invocation-danger", SeverityWarning, "a skill the model can invoke by itself has unrestricted Bash and ships scripts, or a subagent runs with permissionMode bypassPermissions"})
	s.addItemCheck(checkAutoInvocation, AnalyzerSecurity)
}

func checkAutoInvocation(r *runner, it *item, _ doc, fm frontmatter) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	switch it.kind {
	case kindSkill, kindCommand:
		if k, ok := fm.top(keyDisableModel); ok && k.Value == true {
			return
		}
		tools, ok := fm.top(keyAllowedTools)
		if !ok {
			return
		}
		allow := map[string]bool{}
		for _, a := range r.security().AllowedTools {
			allow[strings.TrimSpace(a)] = true
		}
		broad := ""
		for _, tool := range splitTools(tools.Value) {
			if broadTool(tool) && toolBase(tool) != "*" && !allow[tool] {
				broad = tool
				break
			}
		}
		scripts := 0
		for _, res := range it.cf.Resources {
			if res.Kind == config.SkillKindScripts {
				scripts++
			}
		}
		if broad != "" && scripts > 0 {
			r.add(CodeAutoInvocation, it.abs, tools.Line,
				"%s %q can be invoked by the model on its own, is allowed %q and ships %d script(s); text the model reads could start them without a request. Set disable-model-invocation: true or narrow allowed-tools",
				it.kind, itemID(it.kind, it.cf), broad, scripts)
		}
	case kindAgent:
		if k, ok := fm.top("permissionMode"); ok && scalar(k.Value) == "bypassPermissions" {
			r.add(CodeAutoInvocation, it.abs, k.Line,
				"agent %q runs with permissionMode bypassPermissions, and the model can delegate to it without asking; use acceptEdits or default", itemID(it.kind, it.cf))
		}
	}
}
