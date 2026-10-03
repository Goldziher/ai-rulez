package config

// SharedOutput names an output several presets can read and that the agents_md
// flag therefore renders once instead of once per preset.
type SharedOutput string

const (
	// SharedAgentsMD is the root AGENTS.md (nested <scope>/AGENTS.md for scopes).
	SharedAgentsMD SharedOutput = "AGENTS.md"
	// SharedAgentSkills is the .agents/skills directory of Agent Skills.
	SharedAgentSkills SharedOutput = ".agents/skills"
)

// SharedOutputConsumer describes how a preset takes part in the shared outputs
// when agents_md is on: which ones it reads, and the preset-specific skills
// directory it stops writing because .agents/skills replaces it.
type SharedOutputConsumer struct {
	Outputs []SharedOutput
	// OwnSkillsDir is the preset's own skills directory, relative to the output
	// base dir. Empty when the preset has none or already writes .agents/skills.
	OwnSkillsDir string
	// OwnRootFile is the preset's own root instruction file, relative to the
	// output base dir, that it stops writing because AGENTS.md replaces it
	// (GEMINI.md, .hermes.md). Empty when the preset keeps its root file.
	OwnRootFile string
	// ImportsAgentsMD marks a preset that does not read AGENTS.md itself and
	// instead imports it from its own root file (CLAUDE.md with "@AGENTS.md").
	// Such a preset still needs the shared AGENTS.md rendered, and owns it for
	// frontmatter targets.
	ImportsAgentsMD bool
}

// sharedOutputConsumers is the single place that decides which presets
// participate. A table rather than a generator interface: amp is a declarative
// provider with no Go type to implement one, and the participation of a preset
// is a fact about the tool, not about how its generator renders.
var sharedOutputConsumers = map[string]SharedOutputConsumer{
	string(PresetCodex):    {Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnSkillsDir: ".codex/skills"},
	string(PresetOpenCode): {Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnSkillsDir: ".opencode/skills"},
	string(PresetXum):      {Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnSkillsDir: ".xum/skills"},
	string(PresetAmp):      {Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}},
	// Claude Code reads CLAUDE.md only; its CLAUDE.md becomes an "@AGENTS.md"
	// shim. It does not read .agents/skills, so it keeps .claude/skills.
	string(PresetClaude): {ImportsAgentsMD: true},
	// Gemini CLI reads AGENTS.md through .gemini/settings.json context.fileName
	// and .agents/skills natively.
	string(PresetGemini): {Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnRootFile: "GEMINI.md"},
	// Antigravity reads AGENTS.md and .agents/skills natively and keeps its
	// .agents/rules folder for scoped rules.
	string(PresetAntigravity): {Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnRootFile: "GEMINI.md"},
	// Cursor reads .agents/skills and already writes its skills there.
	string(PresetCursor): {Outputs: []SharedOutput{SharedAgentSkills}},
	// Hermes would let .hermes.md shadow AGENTS.md, so the preset stops writing it.
	string(PresetHermes): {Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnRootFile: ".hermes.md"},
}

// SharedOutputConsumerFor returns the shared-output participation of a built-in
// preset, and false when the preset takes no part.
func SharedOutputConsumerFor(preset string) (SharedOutputConsumer, bool) {
	consumer, ok := sharedOutputConsumers[preset]
	return consumer, ok
}

// Reads reports whether the consumer reads the shared output.
func (c SharedOutputConsumer) Reads(output SharedOutput) bool {
	for _, o := range c.Outputs {
		if o == output {
			return true
		}
	}
	return false
}

// NeedsAgentsMD reports whether the preset reads or imports the shared AGENTS.md.
func (c SharedOutputConsumer) NeedsAgentsMD() bool {
	return c.ImportsAgentsMD || c.Reads(SharedAgentsMD)
}
