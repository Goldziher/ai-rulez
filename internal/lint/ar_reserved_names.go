package lint

import "strings"

// reservedPreset is the built-in slash commands and bundled skills of one
// harness. A project skill or command with one of these names replaces the
// built-in (Claude Code) or hides it, so the built-in stops working (AR808).
type reservedPreset struct {
	// Title names the harness in the message.
	Title string
	// Source is the vendor page the names were taken from, verified on Verified.
	Source, Verified string
	// Names are lower-case with hyphens; a skill's name is normalized the same way.
	Names []string
	// SkillRoots and CommandRoots are the directories (slash paths below any project directory)
	// the harness loads project skills and commands from. A hand-written skill or command
	// elsewhere cannot shadow a built-in. Plugin says the harness also loads the skills/ and
	// commands/ directories of a plugin.
	SkillRoots, CommandRoots []string
	Plugin                   bool
}

// reservedPresetTable is the table, keyed by preset name. Add a harness by
// adding an entry, and list a name only when the vendor documents it.
func reservedPresetTable() map[string]reservedPreset {
	return map[string]reservedPreset{
		presetClaude: {
			Title:        "Claude Code",
			Source:       "https://code.claude.com/docs/en/commands",
			Verified:     "2026-10-17",
			SkillRoots:   []string{".claude/skills"},
			CommandRoots: []string{".claude/commands"},
			Plugin:       true,
			Names: strings.Fields(`
				add-dir advisor agents artifact-capabilities artifact-diagramming artifacts auto-mode-setup
				autocompact autofix-pr background batch branch btw bug cd chrome claude-api claude-in-chrome clear
				code-review color compact config context copy cost dataviz debug deep-research design design-login
				design-sync desktop diff doctor effort exit export fast feedback fewer-permission-prompts focus
				fork goal heapdump help hooks ide import init insights install-github-app install-slack-app
				keybindings list-agents login logout loop mcp memory mobile model output-style passes permissions
				plan plugin plugin-authoring powerup pr-comments privacy-settings radio rate-limit-options recap
				release-notes reload-plugins reload-skills remote-control remote-env rename resume review rewind
				run run-skill-generator sandbox schedule scroll-speed security-review setup-bedrock setup-vertex
				simplify skill-doctor skills slides stats status statusline stickers stop subtask tasks
				team-onboarding teleport terminal-setup theme tui ultraplan ultrareview update-config upgrade
				usage usage-credits verify vim voice web-setup workflow-authoring workflows
			`),
		},
	}
}
