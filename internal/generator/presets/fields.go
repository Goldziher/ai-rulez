package presets

import "github.com/Goldziher/ai-rulez/internal/config"

// EmitAgentField reports whether an agent frontmatter field should be emitted
// under `defaults.omit_agent_fields`. Presets consult it before writing the
// fields that are not already suppressed by the shared model/effort resolvers.
func EmitAgentField(cfg *config.Config, field string) bool {
	return !cfg.OmitsAgentField(field)
}
