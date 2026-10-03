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
	// (GEMINI.md, .hermes.md). The preset skips rendering it. Empty when the
	// preset keeps its root file.
	OwnRootFile string
	// ImportsAgentsMD marks a preset that does not read AGENTS.md itself and
	// instead imports it from its own root file (CLAUDE.md with "@AGENTS.md").
	// Such a preset still needs the shared AGENTS.md rendered, and owns it for
	// frontmatter targets.
	ImportsAgentsMD bool
	// Folder says what the preset's own rules folder holds once the shared
	// AGENTS.md carries the always-on items; see RulesFolderKind.
	Folder RulesFolderKind
}

// RulesFolderKind describes what a preset's rules folder holds beside the shared
// AGENTS.md, which decides what AGENTS.md must carry for it.
type RulesFolderKind int

// Rules folder kinds.
const (
	// RulesFolderNone: the preset has no rules folder, so AGENTS.md carries
	// everything for it.
	RulesFolderNone RulesFolderKind = iota
	// RulesFolderAlways: every item that is not always-on becomes a file, in
	// both rules modes (cursor, windsurf, cline, continue).
	RulesFolderAlways
	// RulesFolderSplitOnly: files only in the "split" rules mode; in "inline"
	// mode the preset writes none and AGENTS.md carries the scoped items (junie).
	RulesFolderSplitOnly
	// RulesFolderScopedInInline: in "split" mode every non-always-on item is a
	// file; in "inline" mode only glob-scoped ones stay files (claude through
	// inline_filter = path_scoped, antigravity through RoutingScopedOnly), so
	// AGENTS.md carries the auto and manual items.
	RulesFolderScopedInInline
	// RulesFolderScopedOnly: only glob-scoped items are files in either mode;
	// auto and manual items have no file the tool applies (copilot).
	RulesFolderScopedOnly
)

// HasRootFile reports whether the preset has a root instructions file that the
// shared AGENTS.md stands in for: dropped (GEMINI.md, .hermes.md, ...) or
// turned into an import shim (CLAUDE.md).
func (c SharedOutputConsumer) HasRootFile() bool {
	return c.OwnRootFile != "" || c.ImportsAgentsMD
}

// ReplacedRootFile is that root file, relative to the output base dir, or ""
// when the preset has none.
func (c SharedOutputConsumer) ReplacedRootFile() string {
	if c.OwnRootFile != "" {
		return c.OwnRootFile
	}
	if c.ImportsAgentsMD {
		return "CLAUDE.md"
	}
	return ""
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
	string(PresetClaude): {ImportsAgentsMD: true, Folder: RulesFolderScopedInInline},
	// Gemini CLI reads AGENTS.md through .gemini/settings.json context.fileName
	// and .agents/skills natively.
	string(PresetGemini): {Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnRootFile: "GEMINI.md"},
	// Antigravity reads AGENTS.md and .agents/skills natively and keeps its
	// .agents/rules folder for scoped rules.
	string(PresetAntigravity): {
		Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnRootFile: "GEMINI.md",
		Folder: RulesFolderScopedInInline,
	},
	// Cursor reads AGENTS.md and .agents/skills natively; .cursor/rules keeps the
	// rules that are not always-on.
	string(PresetCursor): {Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, Folder: RulesFolderAlways},
	// Copilot reads AGENTS.md, which also stops copilot-instructions.md from
	// shadowing it elsewhere, and .agents/skills. .github/instructions keeps the
	// applyTo-scoped items; auto and manual ones have no file Copilot applies.
	string(PresetCopilot): {
		Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnSkillsDir: ".github/skills",
		OwnRootFile: ".github/copilot-instructions.md", Folder: RulesFolderScopedOnly,
	},
	// Junie prefers AGENTS.md over .junie/guidelines.md and reads .agents/skills.
	string(PresetJunie): {
		Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnSkillsDir: ".junie/skills",
		OwnRootFile: ".junie/guidelines.md", Folder: RulesFolderSplitOnly,
	},
	string(PresetWindsurf): {
		Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnSkillsDir: ".windsurf/skills", Folder: RulesFolderAlways,
	},
	string(PresetCline): {
		Outputs: []SharedOutput{SharedAgentsMD, SharedAgentSkills}, OwnSkillsDir: ".cline/skills", Folder: RulesFolderAlways,
	},
	// Continue reads AGENTS.md at the repository root but not .agents/skills, so
	// its prompts file keeps carrying skills.
	string(PresetContinue): {Outputs: []SharedOutput{SharedAgentsMD}, Folder: RulesFolderAlways},
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
// has no rules folder that holds them (the folder presets keep their own), the
// auto and manual ones when a folder cannot hold those.
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
		split := cfg.RulesModeFor(preset.BuiltIn) == RulesModeSplit
		switch consumer.Folder {
		case RulesFolderNone:
			inlining.Scoped = true
		case RulesFolderSplitOnly:
			// No files in inline mode: the glob-scoped items go to AGENTS.md.
			inlining.Scoped = inlining.Scoped || !split
		case RulesFolderScopedInInline:
			// Inline mode still writes the glob-scoped files; auto and manual
			// items have none.
			inlining.AutoManual = inlining.AutoManual || !split
		case RulesFolderScopedOnly:
			inlining.AutoManual = true
		case RulesFolderAlways:
		}
	}
	// Scoped covers auto and manual items, so equal output means equal values.
	inlining.AutoManual = inlining.AutoManual || inlining.Scoped
	return inlining
}
