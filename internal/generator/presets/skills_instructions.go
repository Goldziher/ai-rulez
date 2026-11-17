package presets

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
)

// WriteRootGuidance appends the served-skill and worktree instructions to a
// full root document.
func WriteRootGuidance(builder *strings.Builder, cfg *config.Config, preset, rootFile string) {
	WriteSkillsInstructions(builder, cfg, preset, rootFile)
	WriteWorktreeGuidance(builder, cfg, preset, rootFile)
}

// WriteSkillsInstructions describes the opt-in served-skill workflow.
func WriteSkillsInstructions(builder *strings.Builder, cfg *config.Config, preset, rootFile string) {
	if rulefiles.SharedRootFile(rootFile) {
		preset = ""
	}
	if text, ok := cfg.SkillsInstructionsFor(preset); ok {
		builder.WriteString(text)
		builder.WriteString("\n\n")
	}
}

// WriteWorktreeGuidance uses CLI instructions in root documents shared across
// presets. The MCP server advertises the tool to connected clients.
func WriteWorktreeGuidance(builder *strings.Builder, cfg *config.Config, preset, rootFile string) {
	if rulefiles.SharedRootFile(rootFile) {
		preset = ""
	}
	if text, ok := cfg.WorktreeGuidanceFor(preset); ok {
		builder.WriteString(text)
		builder.WriteString("\n\n")
	}
}
