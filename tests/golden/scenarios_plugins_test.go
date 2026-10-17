package golden

// installedPluginScenarios imports plugins from a marketplace directory and a
// plugin directory that live inside the project, so the fixture needs no
// repository: a Claude marketplace plugin with hooks enabled and a Codex plugin
// under a name prefix.
func installedPluginScenarios() []scenario {
	files := map[string]string{
		".ai-rulez/config.toml": `version = "5.0"
name = "golden-plugins"
presets = ["claude", "codex", "cursor"]
gitignore = false

[[installed_marketplaces]]
name = "acme"
source = "./vendor/market"

[[installed_plugins]]
name = "tools"
marketplace = "acme"
enable_hooks = true

[[installed_plugins]]
name = "codex-tools"
source = "./vendor/codex-tools"
name_prefix = "cx-"
`,
		".ai-rulez/rules/base.md": "---\npriority: high\n---\n# Base\n\nKeep it simple.\n",

		"vendor/market/.claude-plugin/marketplace.json":                     `{"name": "acme", "plugins": [{"name": "tools", "source": "./plugins/tools"}]}`,
		"vendor/market/plugins/tools/.claude-plugin/plugin.json":            `{"name": "tools", "version": "1.0.0", "description": "Tools"}`,
		"vendor/market/plugins/tools/skills/review/SKILL.md":                "---\nname: review\ndescription: Review a change\n---\n\n# Review\n\nRead the diff first.\n",
		"vendor/market/plugins/tools/skills/review/references/checklist.md": "# Checklist\n\n- tests\n",
		"vendor/market/plugins/tools/agents/planner.md":                     "---\nname: planner\ndescription: Plans work\ntools: Read, Grep\n---\n\nPlan before you code.\n",
		"vendor/market/plugins/tools/commands/ship.md":                      "---\ndescription: Ship it\n---\n\nShip $ARGUMENTS.\n",
		"vendor/market/plugins/tools/hooks/hooks.json": `{"hooks": {"PostToolUse": [{"matcher": "Write|Edit", "hooks": [
			{"type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/scripts/format.sh", "timeout": 10}]}]}}`,
		"vendor/market/plugins/tools/scripts/format.sh": "#!/bin/sh\nexit 0\n",
		"vendor/market/plugins/tools/.mcp.json":         `{"mcpServers": {"tools-docs": {"command": "npx", "args": ["-y", "@example/docs-server"]}}}`,

		"vendor/codex-tools/.codex-plugin/plugin.json": `{"name": "codex-tools", "version": "0.2.0", "description": "Codex tools", "skills": "./skills/"}`,
		"vendor/codex-tools/skills/explain/SKILL.md":   "---\nname: explain\ndescription: Explain code\n---\n\n# Explain\n\nExplain simply.\n",
		"vendor/codex-tools/.mcp.json":                 `{"mcpServers": {"api": {"url": "https://example.com/mcp", "type": "http"}}}`,
	}
	return []scenario{
		{
			name:  "installed-plugins",
			files: files,
			full:  true,
			git:   true,
			steps: []step{
				run("generate", "--yes"),
				run("generate", "--check"),
				run("lock"),
				run("generate", "--locked"),
				run("plugin", "list"),
				run("marketplace", "list"),
			},
		},
	}
}
