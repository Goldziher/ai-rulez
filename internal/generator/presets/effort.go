package presets

import "github.com/Goldziher/ai-rulez/internal/config"

// Effort tier constants — internal vocabulary used in config and frontmatter.
// Each preset translates these via MapEffort to its own accepted values.
const (
	effortLow     = "low"
	effortMedium  = "medium"
	effortHigh    = "high"
	effortXHigh   = "xhigh"
	effortMax     = "max"
	effortInherit = "inherit"
)

// ResolveAgentEffort returns the effort to use for a per-agent slot, or "" to omit.
// Resolution order: per-agent metadata → defaults.effort_by_preset[preset] → defaults.effort.
// The returned value is the internal tier; callers must run it through MapEffort to
// get the preset-specific value.
func ResolveAgentEffort(preset string, agent config.ContentFile, cfg *config.Config) string {
	if agent.Metadata != nil && agent.Metadata.Effort != "" {
		return agent.Metadata.Effort
	}
	return ResolveGlobalEffort(preset, cfg)
}

// ResolveGlobalEffort returns the effort to use for a global (non-agent) slot, or "".
// Per-preset override beats the global default.
func ResolveGlobalEffort(preset string, cfg *config.Config) string {
	if cfg == nil || cfg.Defaults == nil {
		return ""
	}
	if v, ok := cfg.Defaults.EffortByPreset[preset]; ok && v != "" {
		return v
	}
	return cfg.Defaults.Effort
}

// MapEffort translates the internal effort tier to a value accepted by the named preset.
// Returns "" when the preset does not accept the given tier (caller must omit emission).
//
// Vocabularies (sourced April 2026):
//   - claude: low, medium, high, xhigh, max, inherit (per-agent frontmatter)
//   - codex:  low, medium, high, xhigh — model_reasoning_effort in .codex/config.toml.
//     max → high (Codex tops at xhigh; max would be a silent over-promise → cap at high
//     to stay below xhigh's cost). inherit dropped (no equivalent).
//   - amp:    low, medium, high, max — amp.anthropic.effort. xhigh → high (Amp has no xhigh).
//   - windsurf: low, medium, high, xhigh — agent frontmatter reasoning_effort. max → high.
//   - antigravity: low, medium, high, xhigh — thinking_level frontmatter. max → high.
//   - continue-dev: handled out-of-band — emit reasoning: true and a budget tied to tier.
//     MapEffort still returns the canonical tier; the preset uses its own table.
func MapEffort(preset, tier string) string {
	if tier == "" {
		return ""
	}
	switch preset {
	case "claude":
		return tier // pass-through; validation already restricted the set.
	case "codex":
		return mapCodex(tier)
	case "amp":
		return mapAmp(tier)
	case "windsurf", "antigravity":
		return mapWindsurfAntigravity(tier)
	case "continue-dev":
		return mapContinueDev(tier)
	default:
		return ""
	}
}

func mapCodex(tier string) string {
	switch tier {
	case effortLow, effortMedium, effortHigh, effortXHigh:
		return tier
	case effortMax:
		return effortHigh
	default: // inherit, anything unknown
		return ""
	}
}

func mapAmp(tier string) string {
	switch tier {
	case effortLow, effortMedium, effortHigh, effortMax:
		return tier
	case effortXHigh:
		return effortHigh
	default:
		return ""
	}
}

func mapWindsurfAntigravity(tier string) string {
	switch tier {
	case effortLow, effortMedium, effortHigh, effortXHigh:
		return tier
	case effortMax:
		return effortHigh
	default:
		return ""
	}
}

func mapContinueDev(tier string) string {
	switch tier {
	case effortLow, effortMedium, effortHigh:
		return tier
	case effortXHigh, effortMax:
		return effortHigh
	default:
		return ""
	}
}
