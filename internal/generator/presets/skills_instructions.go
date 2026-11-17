package presets

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
)

// WriteSkillsInstructions appends the "how to load served skills" section to a
// root instruction file when the project serves skills (see
// config.Config.SkillsInstructions), and then the one short worktree
// instruction (config.Config.WorktreeGuidance). The two share this call site
// and the shared-root routing, but the guidance's decision is its own: with
// served skills off it still renders, and its text never depends on the
// preset. rootFile is the file being written: one several presets share
// (AGENTS.md) renders identically whichever preset writes it last. The
// authored AGENTS.md of the "agents" source is never written, so no call site
// reaches it.
func WriteSkillsInstructions(builder *strings.Builder, cfg *config.Config, preset, rootFile string) {
	if rulefiles.SharedRootFile(rootFile) {
		preset = ""
	}
	if text, ok := cfg.SkillsInstructionsFor(preset); ok {
		builder.WriteString(text)
		builder.WriteString("\n\n")
	}
	if text, ok := cfg.WorktreeGuidance(); ok {
		builder.WriteString(text)
		builder.WriteString("\n\n")
	}
}
