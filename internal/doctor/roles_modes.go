package doctor

import (
	"context"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/roles"
)

// CheckRoleModes is the name of the role skill_mode check.
const CheckRoleModes = "roles"

// checkRoleModes reports, per role, the skill modes a configured harness cannot
// honor and what generate --role does instead (see roles.PlanSkillModes). It is
// advisory: the role still renders, with the documented fallback.
func checkRoleModes(_ context.Context, s *state) []Finding {
	if s.cfg == nil {
		return nil
	}
	var out []Finding
	for _, name := range s.cfg.RoleNames() {
		res, err := s.cfg.ResolveRole(name)
		if err != nil {
			continue // reported by the config check and validate --strict (AR972)
		}
		for _, o := range roles.PlanSkillModes(s.cfg, res) {
			if len(o.Degraded) == 0 {
				continue
			}
			out = append(out, Finding{
				Check: CheckRoleModes, Severity: SeverityInfo,
				Message: "role " + name + ": skill_mode " + o.Mode + " of " + o.Key() + " is not honored on " +
					strings.Join(o.Degraded, ", ") + "; " + degradedEffect(o),
				Hint: "see docs/roles.md#skill_mode-on-other-harnesses; [role_manifest] skill_mode_fallback picks drop or serve for off",
			})
		}
	}
	return out
}

func degradedEffect(o roles.SkillOutcome) string {
	switch o.Action {
	case roles.ActionDrop:
		return "the skill is left out of the render"
	case roles.ActionServe:
		return "the skill is served over MCP instead"
	}
	return "the skill stays listed there"
}
