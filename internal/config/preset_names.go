package config

import "sort"

type PresetName string

const (
	PresetClaude      PresetName = "claude"
	PresetCursor      PresetName = "cursor"
	PresetDevin       PresetName = "devin"
	PresetCopilot     PresetName = "copilot"
	PresetGemini      PresetName = "gemini"
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
//
// The declarative provider presets are read from the specs embedded in
// internal/generator/providers/builtin, so the result is the same in every program.
func AllPresetNames() []string {
	return sortedPresetNames(false)
}

// IsBuiltInPresetName reports whether name is a built-in preset.
func IsBuiltInPresetName(name string) bool {
	return isValidBuiltInPreset(name)
}

// IndividualPresetNames returns the per-tool presets, excluding the shared `mcp`
// config preset and the `okf` knowledge-bundle preset (neither targets a coding
// tool), sorted.
func IndividualPresetNames() []string {
	return sortedPresetNames(true)
}

func sortedPresetNames(excludeMCP bool) []string {
	set := builtInPresetSet()
	names := make([]string, 0, len(set))
	for name := range set {
		if excludeMCP && (name == string(PresetMCP) || name == PresetOKF) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
