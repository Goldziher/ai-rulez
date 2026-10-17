package config

import (
	"slices"
	"strings"

	"github.com/samber/oops"
)

// Values of the skip_outputs option: output kinds ai-rulez never writes,
// overwrites or deletes, so a project can hand-write them.
const (
	// SkipInstructions skips every root instruction file (CLAUDE.md, AGENTS.md,
	// GEMINI.md, ...), their machine-local variants and the scope roots. It
	// overrides agents_md and claude_md.
	SkipInstructions = "instructions"
	// SkipAgentsMD skips only the shared AGENTS.md (and AGENTS.override.md) and
	// the shared .agents/skills tree.
	SkipAgentsMD = "agents_md"
	// SkipClaudeMD skips only CLAUDE.md.
	SkipClaudeMD = "claude_md"
	// SkipRules skips rule files (.cursor/rules, .claude/rules, ...).
	SkipRules = "rules"
	// SkipSkills skips skill directories.
	SkipSkills = "skills"
	// SkipAgents skips agent definition files.
	SkipAgents = "agents"
	// SkipCommands skips command and prompt files.
	SkipCommands = "commands"
)

// SkipOutputValues lists the values skip_outputs accepts.
func SkipOutputValues() []string {
	return []string{SkipInstructions, SkipAgentsMD, SkipClaudeMD, SkipRules, SkipSkills, SkipAgents, SkipCommands}
}

func (c *Config) validateSkipOutputs() error {
	valid := SkipOutputValues()
	for i, entry := range c.SkipOutputs {
		if slices.Contains(valid, entry) {
			continue
		}
		return oops.
			With("field", "skip_outputs").
			With("index", i).
			With("actual_value", entry).
			With("valid_values", valid).
			Hint("Use one of: "+strings.Join(valid, ", ")+".").
			Errorf("invalid skip_outputs value %q", entry)
	}
	return nil
}

// SkipsOutput reports whether skip_outputs lists kind. Safe on a nil receiver.
func (c *Config) SkipsOutput(kind string) bool {
	return c != nil && slices.Contains(c.SkipOutputs, kind)
}

// HasSkipOutputs reports whether skip_outputs is set. Safe on a nil receiver.
func (c *Config) HasSkipOutputs() bool {
	return c != nil && len(c.SkipOutputs) > 0
}

// SkipsAgentsMD reports whether no shared AGENTS.md is written on purpose,
// through "instructions" or "agents_md". Safe on a nil receiver.
func (c *Config) SkipsAgentsMD() bool {
	return c.SkipsOutput(SkipInstructions) || c.SkipsOutput(SkipAgentsMD)
}
