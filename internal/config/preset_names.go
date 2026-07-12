package config

type PresetName string

const (
	PresetPopular     PresetName = "popular"
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
	PresetAntigravity PresetName = "antigravity"
)

func AllPresetNames() []string {
	return []string{
		string(PresetPopular),
		string(PresetClaude),
		string(PresetCursor),
		string(PresetWindsurf),
		string(PresetCopilot),
		string(PresetGemini),
		string(PresetContinue),
		string(PresetCline),
		string(PresetAmp),
		string(PresetCodex),
		string(PresetJunie),
		string(PresetHermes),
		string(PresetAntigravity),
	}
}

func IndividualPresetNames() []string {
	return []string{
		string(PresetClaude),
		string(PresetCursor),
		string(PresetWindsurf),
		string(PresetCopilot),
		string(PresetGemini),
		string(PresetContinue),
		string(PresetCline),
		string(PresetAmp),
		string(PresetCodex),
		string(PresetJunie),
		string(PresetHermes),
		string(PresetAntigravity),
	}
}
