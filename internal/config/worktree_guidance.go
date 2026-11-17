package config

import (
	"fmt"
	"strings"
)

// WorktreeGuidance is the one short instruction the generated root guidance
// and the default session-start briefing carry: run the worktree setup
// command from the source checkout after git or the agent's host creates a
// linked worktree, or the create command when the agent owns creation.
// Carrying machine-local files is explicit consent, never a default.
//
// The instruction is independent of served skills and identical across root
// files several presets write. When the configuration's server registers the
// worktree_setup tool, the wording prefers the tool at runtime and names the
// CLI command as the fallback; otherwise it is CLI-only.
func (c *Config) WorktreeGuidance() (text string, show bool) {
	if c == nil {
		return "", false
	}
	base := "## Worktrees\n\n" +
		"After git or your host creates a linked worktree, run `ai-rulez worktree setup <path>` from this checkout to provision it, " +
		"or `ai-rulez worktree create <path> --branch <branch>` when you create the worktree yourself. " +
		"Carrying machine-local files into the worktree is opt-in with `--copy-local`; consent is explicit, never a default."
	if c.HasSelfServer() && c.hasMCPPreset() && c.selfServerExposesWorktreeSetup() {
		base += fmt.Sprintf(" At runtime prefer the `worktree_setup` tool on the %#q MCP server when it is exposed; the CLI command is the fallback.", c.skillsServerName())
	}
	return base, true
}

// selfServerExposesWorktreeSetup reports whether the server this configuration
// writes actually registers the worktree_setup tool: the [mcp] tools
// allowlist, when set, is the only list the server registers, and read_only
// servers register no write tool.
func (c *Config) selfServerExposesWorktreeSetup() bool {
	if c.MCP == nil {
		return true
	}
	if c.MCP.ReadOnly != nil && *c.MCP.ReadOnly {
		return false
	}
	if len(c.MCP.Tools) == 0 {
		return true
	}
	for _, name := range c.MCP.Tools {
		if name == "worktree_setup" {
			return true
		}
	}
	return false
}

// ShowWorktreeGuidance reports whether a generated root file the preset names
// may carry the worktree guidance. The authored AGENTS.md of the "agents"
// source is never written, so it never carries the guidance from here; a
// generated harness file that is actually loaded (for example the CLAUDE.md
// shim) still does.
func (c *Config) ShowWorktreeGuidance() bool {
	return c != nil
}

// WorktreeBriefing is the worktree guidance as one line for the default
// session-start briefing. It names no placeholder in angle brackets: the
// briefing context is kept free of them, so markup a launcher or the network
// chose cannot pass as generated text.
func (c *Config) WorktreeBriefing() (text string, show bool) {
	guidance, show := c.WorktreeGuidance()
	if !show {
		return "", false
	}
	line := strings.ReplaceAll(strings.TrimPrefix(guidance, "## Worktrees\n\n"), "`--copy-local`", "--copy-local")
	line = strings.ReplaceAll(line, "`ai-rulez worktree setup <path>`", "`ai-rulez worktree setup` with the worktree's path")
	line = strings.ReplaceAll(line, "`ai-rulez worktree create <path> --branch <branch>`", "`ai-rulez worktree create` with the path and a new `--branch`")
	return line, true
}
