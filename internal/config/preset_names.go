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
// The declarative provider presets register themselves from the init of
// internal/generator/providers, so the result is complete only in a binary that
// links that package (any program importing internal/generator does). A program
// that imports config alone sees just the Go-implemented presets.
func AllPresetNames() []string {
	return sortedPresetNames(false)
}

// IsBuiltInPresetName reports whether name is a built-in preset.
func IsBuiltInPresetName(name string) bool {
	return isValidBuiltInPreset(name)
}

// IndividualPresetNames returns the per-tool presets, excluding the shared `mcp`
// config preset, sorted.
func IndividualPresetNames() []string {
	return sortedPresetNames(true)
}

func sortedPresetNames(excludeMCP bool) []string {
	builtInPresetsMu.RLock()
	names := make([]string, 0, len(builtInPresets))
	for name := range builtInPresets {
		if excludeMCP && name == string(PresetMCP) {
			continue
		}
		names = append(names, name)
	}
	builtInPresetsMu.RUnlock()
	sort.Strings(names)
	return names
}
