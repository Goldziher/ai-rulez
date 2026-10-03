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
	// RulesFolder marks a preset whose own rules folder holds the rules and
	// context that are not always-on, so the shared AGENTS.md does not have to
	// carry them for it.
	RulesFolder bool
	// FolderNeedsSplit marks a RulesFolder preset whose folder only takes those
	// items in the "split" rules mode; in "inline" mode they stay in a root file
	// that agents_md no longer writes, so AGENTS.md must carry them.
	FolderNeedsSplit bool
	// FolderSkipsAutoManual marks a RulesFolder preset whose folder cannot hold
	// auto and manual items (copilot applies a file only through applyTo), so
	// AGENTS.md carries those.
	FolderSkipsAutoManual bool
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
	string(PresetClaude): {ImportsAgentsMD: true, RulesFolder: true, FolderNeedsSplit: true},
	// Gemini CLI reads AGENTS.md through .gemini/settings.json context.fileName
	// and .agents/skills natively.
	string(PresetGemini): {Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnRootFile: "GEMINI.md"},
	// Antigravity reads AGENTS.md and .agents/skills natively and keeps its
	// .agents/rules folder for scoped rules.
	string(PresetAntigravity): {
		Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnRootFile: "GEMINI.md",
		RulesFolder: true, FolderNeedsSplit: true,
	},
	// Cursor reads AGENTS.md and .agents/skills natively; .cursor/rules keeps the
	// rules that are not always-on.
	string(PresetCursor): {Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, RulesFolder: true},
	// Copilot reads AGENTS.md, which also stops copilot-instructions.md from
	// shadowing it elsewhere, and .agents/skills. .github/instructions keeps the
	// applyTo-scoped items; auto and manual ones have no file Copilot applies.
	string(PresetCopilot): {
		Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnSkillsDir: ".github/skills",
		OwnRootFile: ".github/copilot-instructions.md", RulesFolder: true, FolderSkipsAutoManual: true,
	},
	// Junie prefers AGENTS.md over .junie/guidelines.md and reads .agents/skills.
	string(PresetJunie): {
		Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnSkillsDir: ".junie/skills",
		OwnRootFile: ".junie/guidelines.md", RulesFolder: true, FolderNeedsSplit: true,
	},
	string(PresetWindsurf): {
		Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnSkillsDir: ".windsurf/skills", RulesFolder: true,
	},
	string(PresetCline): {
		Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnSkillsDir: ".cline/skills", RulesFolder: true,
	},
	// Continue reads AGENTS.md at the repository root but not .agents/skills, so
	// its prompts file keeps carrying skills.
	string(PresetContinue): {Outputs: []SharedOutput{SharedAgentsMD}, RulesFolder: true},
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

// ReadsSharedAgentsMD reports whether agents_md is on and the built-in preset
// relies on the shared AGENTS.md in place of its own root instructions file.
func (c *Config) ReadsSharedAgentsMD(preset string) bool {
	if c == nil || !c.AgentsMD {
		return false
	}
	consumer, ok := SharedOutputConsumerFor(preset)
	return ok && consumer.NeedsAgentsMD()
}

// AgentsMDInlining says which items beyond the always-on ones the shared AGENTS.md
// inlines. Scoped covers glob, auto and manual rules and glob context;
// AutoManual only the auto and manual ones.
type AgentsMDInlining struct {
	Scoped     bool
	AutoManual bool
}

// SharedAgentsMDInlining decides what AGENTS.md carries beyond always-on rules
// and context: scoped items only when a configured preset relying on the file
// has no rules folder that holds them (the folder presets keep their own).
func SharedAgentsMDInlining(cfg *Config) AgentsMDInlining {
	var inlining AgentsMDInlining
	for _, preset := range cfg.Presets {
		if !preset.IsBuiltIn() {
			continue
		}
		consumer, ok := SharedOutputConsumerFor(preset.BuiltIn)
		if !ok || !consumer.NeedsAgentsMD() {
			continue
		}
		switch {
		case !consumer.RulesFolder, consumer.FolderNeedsSplit && cfg.RulesModeFor(preset.BuiltIn) != RulesModeSplit:
			inlining.Scoped = true
		case consumer.FolderSkipsAutoManual:
			inlining.AutoManual = true
		}
	}
	// Scoped covers auto and manual items, so equal output means equal values.
	inlining.AutoManual = inlining.AutoManual || inlining.Scoped
	return inlining
}
