package govview

import (
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/roles"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// RoleResolution is the JSON of `ai-rulez roles resolve <name> --format json`.
// The fields are declared in key order (the CLI used to print a sorted map).
type RoleResolution struct {
	Role          *roles.Role          `json:"role"`
	SchemaVersion int                  `json:"schema_version"`
	SkillModes    []roles.SkillOutcome `json:"skill_modes"`
	Tokenizer     string               `json:"tokenizer"`
}

// ModeOutcomes is what each skill_mode of the role comes to on the configured
// harnesses (see roles.PlanSkillModes).
func ModeOutcomes(cfg *config.Config, name string) ([]roles.SkillOutcome, error) {
	res, err := cfg.ResolveRole(name)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	outcomes := roles.PlanSkillModes(cfg, res)
	if outcomes == nil {
		outcomes = []roles.SkillOutcome{}
	}
	return outcomes, nil
}

// ResolveRole builds what a person holding the named role gets.
func ResolveRole(cfg *config.Config, name string, counter tokens.Counter) (*RoleResolution, error) {
	role, err := roles.BuildRole(cfg, name, counter)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	outcomes, err := ModeOutcomes(cfg, name)
	if err != nil {
		return nil, err
	}
	return &RoleResolution{Role: role, SchemaVersion: roles.SchemaVersion, SkillModes: outcomes, Tokenizer: counter.Name()}, nil
}

// ResolutionView is a RoleResolution with the item list capped at a limit. Under
// the limit it serialises exactly as RoleResolution.
type ResolutionView struct {
	*RoleResolution
	TotalItems int  `json:"total_items,omitempty"`
	Truncated  bool `json:"truncated,omitempty"`
}

// ViewResolution keeps at most limit items of the role (see ClampLimit); the
// totals still count every item.
func ViewResolution(res *RoleResolution, limit int) *ResolutionView {
	view := &ResolutionView{RoleResolution: res}
	if limit = ClampLimit(limit); len(res.Role.Items) > limit {
		role := *res.Role
		view.TotalItems, view.Truncated = len(role.Items), true
		role.Items = role.Items[:limit]
		cut := *res
		cut.Role = &role
		view.RoleResolution = &cut
	}
	return view
}
