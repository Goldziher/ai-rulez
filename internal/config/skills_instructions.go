package config

import (
	"fmt"
	"path"
	"strings"
)

// Variables a [mcp] skills_instructions_text may use.
const (
	varServer    = "server"
	varSkillsDir = "skills_dir"
)

// ShowSkillsInstructions reports whether the skills section may be written; it
// is on unless [mcp] skills_instructions = false.
func (m *MCPConfig) ShowSkillsInstructions() bool {
	return m == nil || m.SkillsInstructions == nil || *m.SkillsInstructions
}

// SkillsInstructions returns the section that tells an agent how to load served
// skills, and whether to write it into the instruction files. Harnesses that do
// not list the dynamic-skills stub (Codex, Cursor, a subagent with a tool
// allowlist) get no other pointer to skills that exist only over MCP.
//
// It applies when skills are served, a configured preset can call MCP, and
// [mcp] skills_instructions is not false. The text is
// [mcp] skills_instructions_text with {{server}} and {{skills_dir}} replaced, or
// a short default naming find_skill, load_skill, the server and the on-disk
// fallback. The result does not depend on the preset: a root file several
// presets write (AGENTS.md) must render identically whichever writes it last.
func (c *Config) SkillsInstructions() (text string, show bool) {
	return c.SkillsInstructionsFor("")
}

// SkillsInstructionsFor is SkillsInstructions for a root file that only preset
// writes: a preset whose harness cannot call MCP gets no section, because the
// served skills are written to its skills folder instead. An empty preset names
// a root file several presets share.
func (c *Config) SkillsInstructionsFor(preset string) (text string, show bool) {
	if c == nil || !c.MCP.ShowSkillsInstructions() || !c.servesSkills() || !c.hasMCPPreset() {
		return "", false
	}
	if preset != "" && !HarnessSupportsMCP(preset) {
		return "", false
	}
	server := c.skillsServerName()
	dir := path.Join(c.skillsInstructionsDirName(), "skills")
	if custom := strings.TrimSpace(c.mcpSkillsInstructionsText()); custom != "" {
		return expandTemplateVars(custom, map[string]string{varServer: server, varSkillsDir: dir}), true
	}
	return fmt.Sprintf("## Unlisted skills\n\n"+
		"Some skills are served over MCP and are not listed in this session. "+
		"Call `find_skill` with a short description of the task, then `load_skill` with the chosen name, "+
		"on the `%s` MCP server. If that server is unavailable, read `%s/<name>/SKILL.md` instead.", server, dir), true
}

func (c *Config) mcpSkillsInstructionsText() string {
	if c.MCP == nil {
		return ""
	}
	return c.MCP.SkillsInstructionsText
}

// skillsInstructionsDirName is the config directory the on-disk skills live in,
// relative to the instruction file.
func (c *Config) skillsInstructionsDirName() string {
	if name := strings.TrimSpace(c.ConfigDirName); name != "" {
		return name
	}
	return aiRulezDirName
}

// hasMCPPreset reports whether some configured preset can call MCP tools.
func (c *Config) hasMCPPreset() bool {
	for i := range c.Presets {
		if HarnessSupportsMCP(c.Presets[i].GetName()) {
			return true
		}
	}
	return false
}
