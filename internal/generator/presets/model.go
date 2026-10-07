package presets

import (
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// providerQualifiedModel matches OpenCode's "provider/model" reference with an
// optional "#variant" suffix, the same shape its config schema accepts.
var providerQualifiedModel = regexp.MustCompile(`^[^/#\s]+(/[^/#\s]+)+(#[^#\s]+)?$`)

// IsProviderQualifiedModel reports whether model is a "provider/model" reference
// (optionally "#variant"), the only form OpenCode resolves. Tool-neutral aliases
// such as "sonnet" are not: OpenCode reads them as a provider with no model.
func IsProviderQualifiedModel(model string) bool {
	return providerQualifiedModel.MatchString(model)
}

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
	if cfg.OmitsAgentField("model") {
		return ""
	}
	if agent.Metadata != nil {
		if v := strings.TrimSpace(agent.Metadata.Extra[preset+"_model"]); v != "" {
			return v
		}
	}
	if v := ResolveGlobalModel(preset, cfg); v != "" {
		return v
	}
	if agent.Metadata != nil {
		if v := strings.TrimSpace(agent.Metadata.Extra["model"]); v != "" {
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
	return strings.TrimSpace(cfg.Defaults.ModelByPreset[preset])
}

// bareModelAliases are the tool-neutral Claude model aliases and the inherit
// marker, none of which is a model id in another tool's namespace.
var bareModelAliases = map[string]bool{"sonnet": true, "opus": true, "haiku": true, effortInherit: true}

// IsBareModelAlias reports whether model is one of the bare Claude aliases
// (sonnet, opus, haiku) or the inherit marker.
func IsBareModelAlias(model string) bool {
	return bareModelAliases[strings.ToLower(strings.TrimSpace(model))]
}

// ResolveNativeAgentModel is ResolveAgentModel for a tool with its own model
// namespace: a bare Claude alias is dropped (the agent then inherits the session
// model), while an explicit <preset>_model or per-preset default is a native id
// and kept.
func ResolveNativeAgentModel(preset string, agent config.ContentFile, cfg *config.Config) string {
	model := ResolveAgentModel(preset, agent, cfg)
	if IsBareModelAlias(model) {
		return ""
	}
	return model
}
