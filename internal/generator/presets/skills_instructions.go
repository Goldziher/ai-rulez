package presets

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
)

// WriteSkillsInstructions appends the "how to load served skills" section to a
// root instruction file when the project serves skills (see
// config.Config.SkillsInstructions). rootFile is the file being written: one
// several presets share (AGENTS.md) renders the same section whichever preset
// writes it last, so it is not decided by preset.
func WriteSkillsInstructions(builder *strings.Builder, cfg *config.Config, preset, rootFile string) {
	if rulefiles.SharedRootFile(rootFile) {
		preset = ""
	}
	text, ok := cfg.SkillsInstructionsFor(preset)
	if !ok {
		return
	}
	builder.WriteString(text)
	builder.WriteString("\n\n")
}
