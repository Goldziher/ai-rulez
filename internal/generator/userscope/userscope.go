// Package userscope is the table of user-level (per person, all projects)
// destinations that `generate --user` writes to.
//
// The table is deliberately closed: an output is written under the home
// directory only when a vendor documents that location for that kind of content.
// Everything else a preset renders (commands, rules folders a harness does not
// read from the home directory, MCP configuration, project-only sidecars) is
// dropped. Each row names the vendor page it was read from; VerifiedOn is the
// date the whole table was last checked against those pages.
package userscope

import (
	"path"
	"sort"
	"strings"
)

// VerifiedOn is the date the table was last checked against vendor documentation.
const VerifiedOn = "2026-10-04"

// Kind classifies what a row carries.
type Kind string

// Content kinds of a row.
const (
	KindInstructions Kind = "instructions"
	KindSkills       Kind = "skills"
	KindAgents       Kind = "agents"
	KindRules        Kind = "rules"
	KindSettings     Kind = "settings"
)

// Entry maps a project-relative output of one preset to its user-level
// destination below the home directory. Both paths are slash-separated and
// relative; From and To are a file, or a directory whose contents map one to one.
type Entry struct {
	Preset string
	Kind   Kind
	From   string
	To     string
	// Source is the vendor page the destination was read from.
	Source string
}

// Vendor documentation pages the rows cite.
const (
	srcClaudeMemory = "https://code.claude.com/docs/en/memory"
	srcClaudeSkills = "https://code.claude.com/docs/en/skills"
	srcClaudeAgents = "https://code.claude.com/docs/en/sub-agents"
	srcClaudeConfig = "https://code.claude.com/docs/en/settings"
	srcCodexAgents  = "https://learn.chatgpt.com/docs/agent-configuration/agents-md"
	srcCodexSkills  = "https://learn.chatgpt.com/docs/build-skills"
	srcCodexHooks   = "https://learn.chatgpt.com/docs/hooks"
	srcGeminiMD     = "https://geminicli.com/docs/cli/gemini-md/"
	srcGeminiSkills = "https://geminicli.com/docs/cli/skills/"
	srcGeminiAgents = "https://geminicli.com/docs/core/subagents/"
	srcGeminiHooks  = "https://geminicli.com/docs/hooks/"
	srcOpenCodeRule = "https://opencode.ai/docs/rules/"
	srcOpenCodeSkil = "https://opencode.ai/docs/skills/"
	srcOpenCodeAgnt = "https://opencode.ai/docs/agents/"
	srcCursorSkills = "https://cursor.com/docs/context/skills"
	srcCursorHooks  = "https://cursor.com/docs/hooks"
	srcCopilotSkill = "https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills"
	srcCopilotHooks = "https://docs.github.com/en/copilot/reference/hooks-configuration"
	srcPiConfig     = "https://pi.dev/docs/latest/configuration"
)

// Project-relative paths and preset names the table repeats.
const (
	agentsSkillsDir = ".agents/skills"
	claudeSkillsDir = ".claude/skills"
	agentsMD        = "AGENTS.md"

	presetClaude   = "claude"
	presetCodex    = "codex"
	presetGemini   = "gemini"
	presetOpenCode = "opencode"
	presetCursor   = "cursor"
	presetCopilot  = "copilot"
	presetPi       = "pi"
)

var table = []Entry{
	{presetClaude, KindInstructions, "CLAUDE.md", ".claude/CLAUDE.md", srcClaudeMemory},
	{presetClaude, KindRules, ".claude/rules", ".claude/rules", srcClaudeMemory},
	{presetClaude, KindSkills, claudeSkillsDir, claudeSkillsDir, srcClaudeSkills},
	{presetClaude, KindAgents, ".claude/agents", ".claude/agents", srcClaudeAgents},
	{presetClaude, KindSettings, ".claude/settings.json", ".claude/settings.json", srcClaudeConfig},

	{presetCodex, KindInstructions, agentsMD, ".codex/AGENTS.md", srcCodexAgents},
	{presetCodex, KindSkills, agentsSkillsDir, agentsSkillsDir, srcCodexSkills},
	{presetCodex, KindSettings, ".codex/hooks.json", ".codex/hooks.json", srcCodexHooks},

	{presetGemini, KindInstructions, "GEMINI.md", ".gemini/GEMINI.md", srcGeminiMD},
	{presetGemini, KindSkills, agentsSkillsDir, agentsSkillsDir, srcGeminiSkills},
	{presetGemini, KindAgents, ".gemini/agents", ".gemini/agents", srcGeminiAgents},
	{presetGemini, KindSettings, ".gemini/settings.json", ".gemini/settings.json", srcGeminiHooks},

	{presetOpenCode, KindInstructions, agentsMD, ".config/opencode/AGENTS.md", srcOpenCodeRule},
	{presetOpenCode, KindSkills, ".opencode/skills", ".config/opencode/skills", srcOpenCodeSkil},
	{presetOpenCode, KindAgents, ".opencode/agents", ".config/opencode/agents", srcOpenCodeAgnt},

	{presetCursor, KindSkills, agentsSkillsDir, agentsSkillsDir, srcCursorSkills},
	{presetCursor, KindSettings, ".cursor/hooks.json", ".cursor/hooks.json", srcCursorHooks},

	{presetCopilot, KindSkills, ".github/skills", ".copilot/skills", srcCopilotSkill},
	{presetCopilot, KindSettings, ".github/hooks/ai-rulez.json", ".copilot/hooks/ai-rulez.json", srcCopilotHooks},

	{presetPi, KindInstructions, agentsMD, ".pi/agent/AGENTS.md", srcPiConfig},
	{presetPi, KindSkills, agentsSkillsDir, ".pi/agent/skills", srcPiConfig},
}

// Entries returns the table in a stable order.
func Entries() []Entry {
	out := make([]Entry, len(table))
	copy(out, table)
	return out
}

// Presets returns the presets that have at least one user-level destination.
func Presets() []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range table {
		if !seen[e.Preset] {
			seen[e.Preset] = true
			out = append(out, e.Preset)
		}
	}
	sort.Strings(out)
	return out
}

// Supports reports whether a preset has any user-level destination.
func Supports(preset string) bool {
	for _, e := range table {
		if e.Preset == preset {
			return true
		}
	}
	return false
}

// Map returns the user-level destination of a project-relative output of preset,
// and the row that matched. ok is false for an output with no documented
// user-level location.
func Map(preset, rel string) (dest string, entry Entry, ok bool) {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	for _, e := range table {
		if e.Preset != preset {
			continue
		}
		if rel == e.From {
			return e.To, e, true
		}
		if strings.HasPrefix(rel, e.From+"/") {
			return e.To + strings.TrimPrefix(rel, e.From), e, true
		}
	}
	return "", Entry{}, false
}

// Roots returns the user-level directories below which generate owns the content
// of a directory row, sorted. clean never removes a directory above them.
func Roots() []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range table {
		if e.Kind == KindSettings || e.Kind == KindInstructions {
			continue
		}
		if !seen[e.To] {
			seen[e.To] = true
			out = append(out, e.To)
		}
	}
	sort.Strings(out)
	return out
}

// Precedence describes, per harness, which copy of a skill wins when the same name
// exists at user and project level, as the vendor documents it.
var Precedence = map[string]string{
	presetClaude:   "Claude Code runs the user-level skill (personal over project)",
	presetGemini:   "Gemini CLI runs the workspace skill (workspace over user)",
	presetCodex:    "Codex lists both; it does not merge or override same-named skills",
	presetOpenCode: "OpenCode does not document a precedence; keep names unique",
	presetCursor:   "Cursor does not document a precedence; keep names unique",
	presetCopilot:  "Copilot does not document a precedence; keep names unique",
	presetPi:       "pi does not document a precedence; keep names unique",
}

// SkillReaders lists, per harness, every user-level skill directory it reads, as
// the vendor documents it. Two of them holding the same skill name load it twice.
var SkillReaders = map[string][]string{
	presetClaude:   {claudeSkillsDir},
	presetCodex:    {agentsSkillsDir},
	presetGemini:   {".gemini/skills", agentsSkillsDir},
	presetOpenCode: {".config/opencode/skills", claudeSkillsDir, agentsSkillsDir},
	presetCursor:   {".cursor/skills", agentsSkillsDir, claudeSkillsDir, ".codex/skills"},
	presetCopilot:  {".copilot/skills", agentsSkillsDir},
}

// ReaderNames returns the harnesses of SkillReaders, sorted.
func ReaderNames() []string {
	out := make([]string, 0, len(SkillReaders))
	for name := range SkillReaders {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
