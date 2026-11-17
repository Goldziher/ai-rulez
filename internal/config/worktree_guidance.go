package config

import (
	"strings"
)

// WorktreeGuidance is the one short instruction the generated root guidance
// and the default session-start briefing carry: run the worktree setup
// command from the source checkout after git or the agent's host creates a
// linked worktree, or the create command when the agent owns creation.
// Carrying machine-local files is explicit consent, never a default.
//
// The instruction is independent of served skills and identical across root
// files several presets write. The wording here is CLI-only; the MCP tool's
// change adds the worktree_setup preference where the tool is actually
// registered ([mcp] tools allowlist and read_only respected).
func (c *Config) WorktreeGuidance() (text string, show bool) {
	if c == nil {
		return "", false
	}
	return "## Worktrees\n\n" +
		"After git or your host creates a linked worktree, run `ai-rulez worktree setup <path>` from this checkout to provision it, " +
		"or `ai-rulez worktree create <path> --branch <branch>` when you create the worktree yourself. " +
		"Carrying machine-local files into the worktree is opt-in with `--copy-local`; consent is explicit, never a default.", true
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
