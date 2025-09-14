package config

var PresetRegistry = map[string][]Output{
	"claude": {
		{Path: "CLAUDE.md"},
		{Path: ".claude/agents/", Type: "agent", NamingScheme: "{name}.md"},
		{Path: ".mcp.json", Template: Template{Type: TemplateBuiltin, Value: "claude-code-mcp"}},
	},
	"cursor": {
		{Path: ".cursor/rules/", Type: "rule", NamingScheme: "{name}.mdc"},
	},
	"copilot": {
		{Path: ".github/copilot-instructions.md"},
	},
	"gemini": {
		{Path: "GEMINI.md"},
		{Path: ".gemini/settings.json", Template: Template{Type: TemplateBuiltin, Value: "gemini-mcp"}},
	},
	"windsurf": {
		{Path: ".windsurf/", Type: "rule", NamingScheme: "{name}.md"},
	},
	"cline": {
		{Path: ".clinerules/", Type: "rule", NamingScheme: "{name}.md"},
	},
	"amp": {
		{Path: "AGENTS.md"},
	},
	"codex": {
		{Path: "AGENTS.md"},
	},
	"opencode": {
		{Path: "AGENTS.md"},
	},
	"continue-dev": {
		{Path: ".continue/rules/", Type: "rule", NamingScheme: "{name}.md"},
		{Path: ".continue/prompts/ai_rulez_prompts.yaml"},
	},
	"popular": {
		{Path: "CLAUDE.md"},
		{Path: "AGENTS.md"},
		{Path: ".mcp.json", Template: Template{Type: TemplateBuiltin, Value: "claude-code-mcp"}},
		{Path: ".cursor/rules/", Type: "rule", NamingScheme: "{name}.mdc"},
		{Path: ".windsurf/", Type: "rule", NamingScheme: "{name}.md"},
		{Path: ".gemini/settings.json", Template: Template{Type: TemplateBuiltin, Value: "gemini-mcp"}},
		{Path: ".github/copilot-instructions.md"},
		{Path: "GEMINI.md"},
		{Path: ".claude/agents/", Type: "agent", NamingScheme: "{name}.md"},
	},
}

func ExpandPresets(presets []string) ([]Output, error) {
	var outputs []Output

	for _, preset := range presets {
		presetOutputs, exists := PresetRegistry[preset]
		if !exists {
			return nil, ErrInvalidPreset{Preset: preset}
		}
		outputs = append(outputs, presetOutputs...)
	}

	return outputs, nil
}

type ErrInvalidPreset struct {
	Preset string
}

func (e ErrInvalidPreset) Error() string {
	return "unknown preset: " + e.Preset
}
