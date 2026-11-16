package config

import (
	"fmt"
	"path"
	"slices"
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
	if !c.skillSearchToolsAvailable(server) {
		return "", false
	}
	return fmt.Sprintf("## Unlisted skills\n\n"+
		"Before answering a repository-specific question or working on a repository path, "+
		"search for relevant guidance with `find_skill` on the `%s` MCP server. "+
		"Build the query from the requested operation. Read a referenced file if needed to resolve ambiguity. "+
		"Base the operation on the request and file contents. "+
		"Pass a short description as `task` and the repository-relative `path` when known. "+
		"Search before concluding that the repository has no guidance. "+
		"Inspect the match description before calling `load_skill`. "+
		"Load a skill only if it covers the specific operation requested at that path. "+
		"A matching path alone is insufficient. "+
		"If all matches cover other workflows, continue without a skill. "+
		"Skip this lookup for unrelated questions. "+
		"In Claude Code, if the tools are deferred, use `ToolSearch` to select them. "+
		"If the server is unavailable, read `%s/<name>/SKILL.md` when the skill is known.", server, dir), true
}

// skillSearchToolsAvailable checks explicit tool allowlists for the named
// launch. An absent list leaves the serving tool surface at its defaults.
func (c *Config) skillSearchToolsAvailable(server string) bool {
	var tools, args []string
	if c.MCP != nil {
		tools = c.MCP.Tools
		if server == SelfMCPServerName && len(c.MCP.SelfServerCommand) > 0 {
			args = c.MCP.SelfServerCommand[1:]
		}
	}
	if entry := c.MCPServers[server]; entry != nil {
		args = entry.Args
	}
	if override, ok := explicitTools(args); ok {
		tools = override
	}
	return len(tools) == 0 || (slices.Contains(tools, "find_skill") && slices.Contains(tools, "load_skill"))
}

// explicitTools recognizes the two native --tools launch syntaxes. The first
// occurrence overrides the configured allowlist; repeats append entries.
func explicitTools(args []string) (tools []string, seen bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if arg == "--tools" && i+1 < len(args) {
			i++
			arg = "--tools=" + args[i]
		}
		if value, ok := strings.CutPrefix(arg, "--tools="); ok {
			seen = true
			if value != "" {
				tools = append(tools, strings.Split(value, ",")...)
			}
		}
	}
	return tools, seen
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
