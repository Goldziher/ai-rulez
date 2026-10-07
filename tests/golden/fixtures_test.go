package golden

import (
	"fmt"
	"strings"
)

// goldenSecret is the value the MCP placeholder resolves to in scenarios that pass
// it. It is a fake; it appears in the goldens because the generated MCP files
// carry resolved values.
const goldenSecret = "golden-secret-value"

var goldenEnv = []string{"GOLDEN_TOKEN=" + goldenSecret}

// goldenFileURLEnv also lets the project config use a file:// git source outside
// the project, as the scenarios serving an include from a temporary directory do.
var goldenFileURLEnv = append([]string{"AI_RULEZ_ALLOW_FILE_URLS=1"}, goldenEnv...)

func tomlList(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func gitignoreKey(top string) string {
	if strings.Contains(top, "gitignore") {
		return ""
	}
	return "gitignore = true\n"
}

// agentsMDKey pins the 4.x per-tool outputs the preset goldens describe, unless
// the scenario states agents_md itself.
func agentsMDKey(top string) string {
	if strings.Contains(top, "agents_md") {
		return ""
	}
	return "agents_md = false\n"
}

// withoutMCP drops the [[mcp_servers]] tables of a rich project: many presets write
// the same MCP file with different bytes, which generate refuses.
func withoutMCP(files map[string]string) map[string]string {
	cfg := files[".ai-rulez/config.toml"]
	start, end := strings.Index(cfg, "[[mcp_servers]]"), strings.Index(cfg, "[permissions]")
	out := merge(files)
	out[".ai-rulez/config.toml"] = cfg[:start] + cfg[end:]
	return out
}

// richConfig is a config.toml exercising MCP, hooks, permissions and the guard;
// extra is appended after the tables, so it may only hold tables.
func richConfig(presets []string, extra string) string { return richConfigTop(presets, "", extra) }

// richConfigTop is richConfig with top-level keys (before any table).
func richConfigTop(presets []string, top, extra string) string {
	return `version = "5.0"
name = "golden"
description = "Golden fixture project"
presets = ` + tomlList(presets) + `
` + gitignoreKey(top) + agentsMDKey(top) + top + `
[guard]
generated = true

[[mcp_servers]]
name = "local-tool"
description = "A stdio server"
command = "npx"
args = ["-y", "golden-mcp"]
[mcp_servers.env]
API_TOKEN = "${GOLDEN_TOKEN}"

[[mcp_servers]]
name = "remote-tool"
transport = "http"
url = "https://mcp.example.com/mcp"
[mcp_servers.headers]
Authorization = "Bearer ${GOLDEN_TOKEN}"

[permissions]
allow = ["Bash(npm run test:*)", "Read(./src/**)", "WebFetch(domain:example.com)"]
ask = ["Bash(git push:*)"]
deny = ["Bash(rm -rf:*)", "Read(./.env)"]

[[hooks]]
event = "PreToolUse"
matcher = "Bash"
[[hooks.hooks]]
script = "scripts/guard.sh"
timeout = 10

[[hooks]]
event = "SessionStart"
[[hooks.hooks]]
command = "echo started"
status_message = "Starting"
` + extra
}

// richFiles is a project with every content kind. The config is config.toml.
func richFiles(presets []string, extraConfig string) map[string]string {
	return richFilesTop(presets, "", extraConfig)
}

// richFilesTop is richFiles with top-level config keys.
func richFilesTop(presets []string, top, tables string) map[string]string {
	f := map[string]string{
		".ai-rulez/config.toml": richConfigTop(presets, top, tables),
		"scripts/guard.sh":      "#!/bin/sh\nexit 0\n",

		".ai-rulez/rules/always.md": "---\npriority: critical\n---\n# Always\n\nAlways apply this rule.\n",
		".ai-rulez/rules/typescript.md": "---\npriority: high\npaths:\n  - \"src/**/*.{ts,tsx}\"\n  - \"tests/**\"\n" +
			"activation: glob\n---\n# TypeScript\n\nUse strict mode.\n",
		".ai-rulez/rules/auto.md":     "---\nactivation: auto\ndescription: Database migrations\n---\n# Migrations\n\nWrite reversible migrations.\n",
		".ai-rulez/rules/manual.md":   "---\nactivation: manual\npriority: low\n---\n# Manual\n\nOnly when asked.\n",
		".ai-rulez/rules/targeted.md": "---\ntargets:\n  - claude\n  - cursor\n---\n# Targeted\n\nFor claude and cursor only.\n",

		".ai-rulez/context/overview.md":     "---\npriority: high\n---\n# Overview\n\nThe project overview.\n",
		".ai-rulez/context/architecture.md": "# Architecture\n\nLayers and boundaries.\n",

		".ai-rulez/skills/reviewer/SKILL.md":                "---\nname: reviewer\ndescription: Reviews pull requests\n---\n# Reviewer\n\nReview carefully.\n",
		".ai-rulez/skills/reviewer/references/checklist.md": "# Checklist\n\n- tests\n- docs\n",
		".ai-rulez/skills/reviewer/scripts/run.sh":          "#!/bin/sh\necho review\n",
		".ai-rulez/skills/deployer/SKILL.md":                "---\nname: deployer\ndescription: Deploys the service\n---\n# Deployer\n\nDeploy safely.\n",

		".ai-rulez/agents/architect.md": "---\nname: architect\ndescription: Designs systems\nmodel: sonnet\n---\n# Architect\n\nThink in layers.\n",
		".ai-rulez/agents/tester.md":    "---\nname: tester\ndescription: Writes tests\neffort: high\n---\n# Tester\n\nTest first.\n",

		".ai-rulez/commands/build.md": "---\nname: build\ndescription: Build the project\n---\nRun the build and report failures.\n",
		".ai-rulez/commands/ship.md":  "---\nname: ship\ndescription: Ship a release\n---\nCut a release.\n",

		".ai-rulez/checks/security.md": "---\ndescription: Flags common security issues\nseverity: high\ntools: [Read, Grep]\n---\n\nReview the diff for injection and secrets.\n",
		".ai-rulez/checks/style.md":    "---\ndescription: Style consistency\nseverity: low\n---\n\nKeep the style consistent.\n",
	}
	return f
}

func scriptExec() map[string]bool { return map[string]bool{"scripts/guard.sh": true} }

// domainFiles adds two domains and profiles to a rich project.
func domainFiles(presets []string) map[string]string {
	f := richFiles(presets, `
[profiles]
backend = ["backend"]
frontend = ["frontend"]
full = ["backend", "frontend"]
`)
	f[".ai-rulez/domains/backend/rules/go.md"] = "---\npriority: high\n---\n# Go\n\nWrap errors.\n"
	f[".ai-rulez/domains/backend/context/api.md"] = "# API\n\nREST conventions.\n"
	f[".ai-rulez/domains/backend/skills/migrator/SKILL.md"] = "---\nname: migrator\ndescription: Runs migrations\n---\n# Migrator\n"
	f[".ai-rulez/domains/backend/agents/dba.md"] = "---\nname: dba\ndescription: Database admin\n---\n# DBA\n"
	f[".ai-rulez/domains/frontend/rules/react.md"] = "---\npaths: [\"web/**/*.tsx\"]\n---\n# React\n\nHooks only.\n"
	f[".ai-rulez/domains/frontend/commands/storybook.md"] = "---\nname: storybook\ndescription: Start storybook\n---\nRun storybook.\n"
	f[".ai-rulez/domains/frontend/checks/a11y.md"] = "---\ndescription: Accessibility\nseverity: medium\n---\n\nCheck ARIA usage.\n"
	return f
}

// publicMCP swaps the secret-carrying MCP servers of a rich project for ones that
// carry none, so documents the user shares with the team can be merged into.
func publicMCP(files map[string]string) map[string]string {
	cfg := files[".ai-rulez/config.toml"]
	start, end := strings.Index(cfg, "[[mcp_servers]]"), strings.Index(cfg, "[permissions]")
	out := merge(files)
	out[".ai-rulez/config.toml"] = cfg[:start] + `[[mcp_servers]]
name = "public-tool"
command = "npx"
args = ["-y", "public-mcp"]

[[mcp_servers]]
name = "public-remote"
transport = "http"
url = "https://mcp.example.com/public"

` + cfg[end:]
	return out
}
