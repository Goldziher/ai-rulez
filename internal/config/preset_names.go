package config

import "sort"

type PresetName string

const (
	PresetClaude      PresetName = "claude"
	PresetCursor      PresetName = "cursor"
	PresetWindsurf    PresetName = "windsurf"
	PresetCopilot     PresetName = "copilot"
	PresetGemini      PresetName = "gemini"
	PresetContinue    PresetName = "continue-dev"
	PresetCline       PresetName = "cline"
	PresetAmp         PresetName = "amp"
	PresetCodex       PresetName = "codex"
	PresetJunie       PresetName = "junie"
	PresetHermes      PresetName = "hermes"
	PresetOpenCode    PresetName = "opencode"
	PresetAntigravity PresetName = "antigravity"
	PresetMCP         PresetName = "mcp"
	PresetXum         PresetName = "xum"
	PresetPi          PresetName = "pi"
	PresetBaz         PresetName = "baz"
)

// AllPresetNames returns every built-in preset name, sorted. It is derived from
// the builtInPresets registry so it cannot drift from validation.
func AllPresetNames() []string {
	names := make([]string, 0, len(builtInPresets))
	for name := range builtInPresets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// IndividualPresetNames returns the per-tool presets, excluding the shared `mcp`
// config preset, sorted.
func IndividualPresetNames() []string {
	names := make([]string, 0, len(builtInPresets))
	for name := range builtInPresets {
		if name == string(PresetMCP) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
