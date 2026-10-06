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
