package golden

// enabledPluginScenarios enables one external plugin for the claude, codex and
// opencode presets from a single [[enabled_plugins]] entry, on top of settings
// files the user already edited, and a second scenario drops the entry again:
// the user's keys survive both runs and only the plugin entries ai-rulez wrote
// leave.
func enabledPluginScenarios() []scenario {
	const head = `version = "5.0"
name = "golden-enabled-plugins"
presets = ["claude", "codex", "opencode"]
gitignore = false
`
	const entry = `
[[enabled_plugins]]
id = "basemind@basemind"
marketplace = { name = "basemind", source = "Goldziher/basemind", type = "github" }
opencode = "basemind-opencode"
`
	files := map[string]string{
		".ai-rulez/config.toml":   head + entry,
		".ai-rulez/rules/base.md": "---\npriority: high\n---\n# Base\n\nKeep it simple.\n",
		".claude/settings.json": "{\n  \"model\": \"mine\",\n  \"extraKnownMarketplaces\": {\"theirs\": {\"source\": {\"source\": \"github\", \"repo\": \"o/r\"}}},\n" +
			"  \"enabledPlugins\": {\"theirs-plugin@theirs\": true}\n}\n",
		".codex/config.toml": "# my codex config\nmodel = \"mine\"\n\n[plugins.\"theirs@theirs\"]\nenabled = true\n",
		"opencode.json":      "{\n  // mine\n  \"model\": \"mine\",\n  \"plugin\": [\"theirs-plugin\"]\n}\n",
	}
	return []scenario{
		{
			name:  "enabled-plugins",
			files: files,
			exec:  scriptExec(),
			git:   true,
			full:  true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				runEnv(goldenEnv, "generate", "--check"),
			},
		},
		{
			name:  "enabled-plugins-dropped",
			files: files,
			exec:  scriptExec(),
			git:   true,
			full:  true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				{kind: stepWrite, path: ".ai-rulez/config.toml", data: head},
				runEnv(goldenEnv, "generate", "--yes"),
			},
		},
	}
}
