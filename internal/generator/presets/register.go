package presets

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
)

// Register adds the Go-implemented presets to r, and the factory custom presets
// are built with. Together with providers.Register it fills the default registry
// (internal/generator/registry); nothing registers itself from an init().
func Register(r *config.Registry) {
	r.Register(presetNameAntigravity, &AntigravityPresetGenerator{})
	r.Register(bazPresetName, &BazPresetGenerator{})
	r.Register(presetNameCline, &ClinePresetGenerator{alwaysFileLocalRules{target: &clineRulesTarget, routing: rulefiles.RoutingEverything}})
	r.Register(codexPresetName, &CodexPresetGenerator{})
	r.Register(presetNameCopilot, &CopilotPresetGenerator{})
	r.Register("cursor", &CursorPresetGenerator{alwaysFileLocalRules{target: &cursorRulesTarget, routing: rulefiles.RoutingEverything}})
	r.Register(devinPresetName, &DevinPresetGenerator{alwaysFileLocalRules{target: &devinRulesTarget, routing: rulefiles.RoutingEverything}})
	r.Register(presetNameGemini, &GeminiPresetGenerator{})
	r.Register(config.PresetLLMsTxt, &LLMsTxtPresetGenerator{})
	r.Register(config.PresetOKF, &OKFPresetGenerator{})
	r.Register(opencodePresetName, &OpencodePresetGenerator{})
	r.Register(xumPresetName, &XumPresetGenerator{})
	r.Custom = func(preset config.Preset) config.PresetGenerator { return NewCustomPresetGenerator(&preset) }
}
