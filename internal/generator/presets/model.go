package presets

import "github.com/Goldziher/ai-rulez/internal/config"

// ResolveAgentModel returns the model string to emit in an agent frontmatter for the
// given preset, or "" to omit. Model strings are preset-specific (each provider has its
// own model namespace) so the resolver passes values through verbatim; there is no
// MapModel translation step analogous to MapEffort.
//
// Resolution order:
//  1. agent.Metadata.Extra["<preset>_model"] (per-agent, preset-specific)
//  2. cfg.Defaults.ModelByPreset[preset]      (global default for that preset)
//  3. agent.Metadata.Extra["model"]           (legacy single field, preset-agnostic)
//  4. ""                                      (omit the model frontmatter)
func ResolveAgentModel(preset string, agent config.ContentFile, cfg *config.Config) string {
	if agent.Metadata != nil {
		if v, ok := agent.Metadata.Extra[preset+"_model"]; ok && v != "" {
			return v
		}
	}
	if v := ResolveGlobalModel(preset, cfg); v != "" {
		return v
	}
	if agent.Metadata != nil {
		if v, ok := agent.Metadata.Extra["model"]; ok && v != "" {
			return v
		}
	}
	return ""
}

// ResolveGlobalModel returns the per-preset global model default, or "".
func ResolveGlobalModel(preset string, cfg *config.Config) string {
	if cfg == nil || cfg.Defaults == nil {
		return ""
	}
	if v, ok := cfg.Defaults.ModelByPreset[preset]; ok && v != "" {
		return v
	}
	return ""
}
