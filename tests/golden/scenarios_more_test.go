package golden

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func merge(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func localScenarios(multi []string) []scenario {
	local := map[string]string{
		".ai-rulez/config.local.toml": `presets = ["claude", "cursor", "codex", "gemini", "copilot", "opencode", "kilo"]

[[mcp_servers]]
name = "personal"
command = "personal-mcp"
args = ["--token", "${GOLDEN_TOKEN}"]
`,
		".ai-rulez/local/rules/mine.md":              "---\npriority: high\n---\n# Mine\n\nA personal rule.\n",
		".ai-rulez/local/context/notes.md":           "# Notes\n\nScratch notes.\n",
		".ai-rulez/local/skills/scratch/SKILL.md":    "---\nname: scratch\ndescription: Scratch skill\n---\n# Scratch\n",
		".ai-rulez/local/agents/helper.md":           "---\nname: helper\ndescription: Local helper\n---\n# Helper\n",
		".ai-rulez/local/commands/quick.md":          "---\nname: quick\ndescription: Quick local command\n---\nDo it.\n",
		".ai-rulez/local/domains/lab/rules/lab.md":   "# Lab\n\nLocal domain rule.\n",
		".ai-rulez/local/domains/lab/context/lab.md": "# Lab context\n",
	}
	return []scenario{
		{
			name:  "local-overlay",
			files: merge(richFiles(multi, ""), local),
			exec:  scriptExec(),
			git:   true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				runEnv(goldenEnv, "generate", "--yes", "--allow-local-drift"),
				runEnv(goldenEnv, "generate", "--yes", "--allow-local-drift"),
				runEnv(goldenEnv, "generate", "--check"),
				runEnv(goldenEnv, "generate", "--yes", "--no-local"),
				runEnv(goldenEnv, "generate", "--dry-run", "--allow-local-drift"),
				run("local", "show"),
				run("clean", "--yes"),
			},
		},
		{
			name:  "local-overlay-full",
			files: merge(richFiles([]string{"claude", "cursor", "codex"}, ""), local),
			exec:  scriptExec(),
			git:   true,
			full:  true,
			steps: []step{runEnv(goldenEnv, "generate", "--yes", "--allow-local-drift")},
		},
	}
}

func lifecycleScenarios(multi []string) []scenario {
	two := []string{"claude", "cursor"}
	oneConfig := richConfig([]string{"claude"}, "")
	userJSONC := `// my settings
{
  "theme": "dark", // keep
  "permissions": {"allow": ["Bash(ls)"]},
  "env": {"MINE": "1"}
}
`
	return []scenario{
		{
			name:  "stale-removal",
			files: richFiles(two, ""),
			exec:  scriptExec(),
			git:   true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				{kind: stepWrite, path: ".ai-rulez/config.toml", data: oneConfig},
				runEnv(goldenEnv, "generate", "--yes"),
				{kind: stepRemove, path: ".ai-rulez/rules/always.md"},
				{kind: stepRemove, path: ".ai-rulez/agents"},
				runEnv(goldenEnv, "generate", "--yes"),
				{kind: stepWrite, path: ".ai-rulez/config.toml", data: richConfig(two, "")},
				runEnv(goldenEnv, "generate", "--yes"),
			},
		},
		{
			name:  "drift-detect",
			files: richFiles(two, ""),
			exec:  scriptExec(),
			git:   true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				{kind: stepAppend, path: "CLAUDE.md", data: "\nhand edit\n"},
				{kind: stepRemove, path: ".cursor/rules/always.mdc"},
				runEnv(goldenEnv, "generate", "--check"),
				run("doctor"),
				run("verify"),
				runEnv(goldenEnv, "generate", "--dry-run"),
				runEnv(goldenEnv, "generate", "--yes"),
				runEnv(goldenEnv, "generate", "--check"),
				{kind: stepAppend, path: ".ai-rulez/rules/always.md", data: "\nmore\n"},
				runEnv(goldenEnv, "generate", "--check"),
				run("doctor", "--format", "json"),
			},
		},
		{
			name: "merged-documents",
			files: merge(publicMCP(richFiles([]string{"claude", "opencode", "codex", "gemini"}, "")), map[string]string{
				".claude/settings.json":   userJSONC,
				"opencode.json":           "{\n  // mine\n  \"$schema\": \"https://opencode.ai/config.json\",\n  \"model\": \"mine\"\n}\n",
				".codex/config.toml":      "# my codex config\nmodel = \"mine\"\n",
				".gemini/settings.json":   "{\"mine\": true}\n",
				".claude/CLAUDE.local.md": "mine\n",
				".claude/rules/always.md": "# my own rule\n",
				".mcp.json":               "{\"mcpServers\": {\"mine\": {\"command\": \"x\"}}}\n",
			}),
			exec: scriptExec(),
			git:  true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				{kind: stepWrite, path: ".ai-rulez/config.toml", data: publicMCP(richFiles([]string{"claude", "opencode"}, ""))[".ai-rulez/config.toml"]},
				runEnv(goldenEnv, "generate", "--yes"),
				runEnv(goldenEnv, "generate", "--yes", "--dry-run"),
			},
			full: true,
		},
		{
			name: "merged-documents-clean",
			files: merge(publicMCP(richFiles([]string{"claude", "opencode", "codex", "gemini"}, "")), map[string]string{
				".claude/settings.json": userJSONC,
				"opencode.json":         "{\n  // mine\n  \"model\": \"mine\"\n}\n",
				".codex/config.toml":    "# my codex config\nmodel = \"mine\"\n",
			}),
			exec: scriptExec(),
			git:  true,
			full: true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				run("clean", "--dry-run"),
				run("clean", "--yes"),
			},
		},
		{
			name:  "mcp-placeholders",
			files: merge(richFiles([]string{"claude", "cursor"}, ""), map[string]string{".env.golden": "GOLDEN_TOKEN=from-env-file\n"}),
			exec:  scriptExec(),
			git:   true,
			steps: []step{
				run("generate", "--yes"),
				run("generate", "--yes", "--env", "GOLDEN_TOKEN=from-flag"),
				run("generate", "--yes", "--env-file", ".env.golden"),
				run("validate"),
				run("doctor"),
			},
		},
		{
			name:  "gitignore-off",
			files: richFilesTop(two, "gitignore = false", ""),
			exec:  scriptExec(),
			steps: []step{runEnv(goldenEnv, "generate", "--yes"), runEnv(goldenEnv, "generate", "--gitignore", "--yes")},
		},
		{
			name: "config-dir",
			files: func() map[string]string {
				f := map[string]string{}
				for k, v := range richFiles(two, "") {
					if len(k) > 10 && k[:10] == ".ai-rulez/" {
						f[".config/ai-rulez/"+k[10:]] = v
					} else {
						f[k] = v
					}
				}
				return f
			}(),
			exec:  scriptExec(),
			git:   true,
			steps: []step{runEnv(goldenEnv, "generate", "--yes"), run("validate")},
		},
		{
			// ${PROJECT_ROOT} resolves to the scratch path, so the files that embed it
			// (and the manifests that digest them) are left out of the snapshot.
			name: "mcp-project-root",
			files: map[string]string{
				".ai-rulez/config.toml": `version = "5.0"
agents_md = false
name = "root"
presets = ["claude", "cursor"]
gitignore = true

[[mcp_servers]]
name = "rooted"
command = "${PROJECT_ROOT}/bin/serve"
args = ["--root", "${PROJECT_ROOT}"]
`,
			},
			git:   true,
			omit:  []string{".ai-rulez/.generated-manifest.local.json", ".ai-rulez/.generated-manifest.json"},
			steps: []step{run("generate", "--yes"), run("generate", "--check")},
		},
		{
			name:  "gitignore-off-public",
			files: publicMCP(richFilesTop(two, "gitignore = false", "")),
			exec:  scriptExec(),
			steps: []step{run("generate", "--yes")},
		},
		{
			name:  "no-git",
			files: richFiles(two, ""),
			exec:  scriptExec(),
			steps: []step{runEnv(goldenEnv, "generate", "--yes"), runEnv(goldenEnv, "generate", "--check")},
		},
		{
			name: "invalid-config",
			files: map[string]string{
				".ai-rulez/config.toml": "version = \"5.0\"\nagents_md = false\nname = \"bad\"\npresets = [\"claude\", \"not-a-preset\"]\n",
			},
			git:   true,
			steps: []step{run("generate", "--yes"), run("validate"), run("validate", "--strict")},
		},
	}
}

func userScenarios(all []string) []scenario {
	userConfig := func(presets []string) string {
		return `version = "5.0"
agents_md = false
name = "me"
presets = ` + tomlList(presets) + `

[permissions]
allow = ["Bash(npm run test:*)"]
deny = ["Bash(rm -rf:*)"]

[[hooks]]
event = "SessionStart"
[[hooks.hooks]]
command = "echo hi"

[profiles]
backend = ["backend"]

[[roles]]
name = "dev"
domains = ["backend"]
`
	}
	userHome := func(presets []string) map[string]string {
		return map[string]string{
			".config/ai-rulez/config.toml":                        userConfig(presets),
			".config/ai-rulez/rules/mine.md":                      "---\npriority: high\n---\n# Mine\n\nPersonal rule.\n",
			".config/ai-rulez/context/me.md":                      "# Me\n\nPersonal context.\n",
			".config/ai-rulez/skills/helper/SKILL.md":             "---\nname: helper\ndescription: Helps\n---\n# Helper\n",
			".config/ai-rulez/agents/buddy.md":                    "---\nname: buddy\ndescription: Buddy\n---\n# Buddy\n",
			".config/ai-rulez/domains/backend/rules/be.md":        "# Backend\n\nBackend rule.\n",
			".config/ai-rulez/domains/backend/skills/db/SKILL.md": "---\nname: db\ndescription: DB\n---\n# DB\n",
		}
	}
	few := []string{"claude", "codex", "gemini", "opencode", "cursor", "copilot"}
	handWritten := map[string]string{
		".claude/CLAUDE.md":     "# my own file\n",
		".claude/settings.json": "{\"mine\": 1}\n",
	}
	return []scenario{
		{
			name: "user-all",
			home: merge(userHome(all), handWritten),
			steps: []step{
				run("generate", "--user", "--dry-run"),
				run("generate", "--user", "--yes"),
				run("generate", "--user", "--yes"),
				run("generate", "--user", "--yes", "--profile", "backend"),
				run("clean", "--user", "--dry-run"),
				run("clean", "--user", "--yes"),
			},
		},
		{
			name:  "user-full",
			home:  merge(userHome(few), handWritten),
			full:  true,
			steps: []step{run("generate", "--user", "--yes")},
		},
		{
			name:  "user-role-full",
			home:  userHome(few),
			full:  true,
			steps: []step{run("generate", "--user", "--yes", "--role", "dev")},
		},
		{
			name:  "user-relocated-home",
			home:  userHome(few),
			steps: []step{runEnv([]string{"CLAUDE_CONFIG_DIR={{HOME}}/elsewhere/claude", "CODEX_HOME={{HOME}}/elsewhere/codex"}, "generate", "--user", "--yes")},
		},
	}
}

func gitInclude(t *testing.T, base, _ string) {
	t.Helper()
	repo := filepath.Join(base, "shared")
	files := map[string]string{
		".ai-rulez/config.toml":        "version = \"5.0\"\nagents_md = false\nname = \"shared\"\npresets = []\n",
		".ai-rulez/rules/shared.md":    "---\npriority: high\n---\n# Shared\n\nShared rule.\n",
		".ai-rulez/context/shared.md":  "# Shared context\n",
		".ai-rulez/skills/sk/SKILL.md": "---\nname: sk\ndescription: Shared skill\n---\n# SK\n",
	}
	for rel, content := range files {
		path := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // fixture
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q", "--template=", "-b", "main"}, {"add", "-A"}, {"-c", "commit.gpgsign=false", "commit", "-q", "-m", "init"}, {"tag", "v1.0.0"}} {
		cmd := exec.Command("git", args...) //nolint:gosec // fixed arguments
		cmd.Dir = repo
		cmd.Env = baseEnv(filepath.Join(base, "home"), []string{"GIT_AUTHOR_DATE=2020-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2020-01-01T00:00:00Z"})
		if msg, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, msg)
		}
	}
}

func miscScenarios(multi []string) []scenario {
	two := []string{"claude", "cursor"}
	scopes := richFiles([]string{"claude", "codex", "cursor", "copilot"}, `
[profiles]
web = ["frontend"]
api = ["backend"]

[[scopes]]
path = "packages/web"
profile = "web"
presets = ["codex", "claude"]

[[scopes]]
path = "packages/api"
profile = "api"
`)
	for k, v := range domainFiles(nil) {
		if len(k) > 20 && k[:20] == ".ai-rulez/domains/ba" || len(k) > 20 && k[:20] == ".ai-rulez/domains/fr" {
			scopes[k] = v
		}
	}
	pluginCfg := richConfig(two, `
[plugin]
name = "golden-plugin"
description = "Golden plugin"
version = "1.2.3"
license = "MIT"
keywords = ["golden"]

[plugin.author]
name = "Golden"
email = "golden@example.invalid"

[plugin.interface]
display_name = "Golden"
short_description = "Golden plugin"
long_description = "A golden plugin for tests"
developer_name = "Golden"
category = "Productivity"
capabilities = ["Read"]
default_prompt = ["Do the thing"]

[[plugin.mcp]]
name = "golden"
command = "${PLUGIN_ROOT}/run.sh"
args = ["serve"]
`)
	return []scenario{
		{
			name:  "scopes",
			files: scopes,
			exec:  scriptExec(),
			git:   true,
			steps: []step{runEnv(goldenEnv, "generate", "--yes"), runEnv(goldenEnv, "generate", "--check"), run("clean", "--yes")},
		},
		{
			name:  "plugin",
			files: merge(richFiles(two, ""), map[string]string{".ai-rulez/config.toml": pluginCfg}),
			exec:  scriptExec(),
			git:   true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--plugin", "--dry-run"),
				runEnv(goldenEnv, "generate", "--plugin", "--yes"),
				runEnv(goldenEnv, "generate", "--plugin", "--yes"),
				runEnv(goldenEnv, "verify", "--plugin"),
			},
		},
		{
			name: "include-local-path",
			files: merge(richFilesTop(two, "", "\n[[includes]]\nname = \"team\"\nsource = \"./vendor/team\"\ninclude = [\"rules\", \"context\", \"skills\"]\n"), map[string]string{
				"vendor/team/rules/team.md":      "---\npriority: high\n---\n# Team\n\nTeam rule.\n",
				"vendor/team/context/team.md":    "# Team context\n",
				"vendor/team/skills/tk/SKILL.md": "---\nname: tk\ndescription: Team skill\n---\n# TK\n",
			}),
			exec:  scriptExec(),
			git:   true,
			steps: []step{runEnv(goldenEnv, "generate", "--yes"), run("include", "list")},
		},
		{
			name:        "include-git-file",
			files:       richFilesTop(two, "", "\n[[includes]]\nname = \"shared\"\nsource = \"file://{{BASE}}/shared\"\nref = \"v1.0.0\"\ninclude = [\"rules\", \"context\", \"skills\"]\n"),
			exec:        scriptExec(),
			git:         true,
			prepare:     gitInclude,
			maskDigests: true,
			omit:        []string{".ai-rulez/ai-rulez.lock", ".ai-rulez/.generated-manifest.local.json", ".ai-rulez/.generated-manifest.json"},
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				runEnv(goldenEnv, "generate", "--yes", "--offline"),
				runEnv(goldenEnv, "lock"),
				runEnv(goldenEnv, "lock", "--check"),
				runEnv(goldenEnv, "generate", "--yes", "--locked"),
			},
		},
		{
			// --emit-plan applies nothing: the only new file is the plan, and the
			// plan is the same before and after a real run except for what the run
			// removed or wrote.
			name:  "emit-plan",
			files: richFiles(two, ""),
			exec:  scriptExec(),
			git:   true,
			full:  true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--emit-plan", "plan-before.json"),
				runEnv(goldenEnv, "generate", "--emit-plan", "-"),
				runEnv(goldenEnv, "generate", "--yes"),
				runEnv(goldenEnv, "generate", "--emit-plan", "plan-after.json", "--profile", "default"),
				{kind: stepWrite, path: ".ai-rulez/config.toml", data: richConfig([]string{"claude"}, "")},
				runEnv(goldenEnv, "generate", "--emit-plan", "plan-stale.json"),
			},
		},
		{
			name:  "lock-content",
			files: richFiles(two, ""),
			exec:  scriptExec(),
			git:   true,
			steps: []step{
				runEnv(goldenEnv, "lock", "--content-only"),
				runEnv(goldenEnv, "lock", "--check"),
				{kind: stepAppend, path: ".ai-rulez/rules/always.md", data: "\nchanged\n"},
				runEnv(goldenEnv, "lock", "--check"),
				runEnv(goldenEnv, "lock", "--diff"),
			},
		},
		{
			name:  "reports",
			files: richFiles(multi, ""),
			exec:  scriptExec(),
			git:   true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				runEnv(goldenEnv, "tokens"),
				runEnv(goldenEnv, "cost"),
				runEnv(goldenEnv, "catalog"),
				runEnv(goldenEnv, "sbom"),
				run("list", "rules"),
				run("list", "skills"),
				run("list", "checks"),
				run("validate", "--strict"),
				run("doctor"),
			},
		},
		{
			name: "recursive",
			files: map[string]string{
				"a/.ai-rulez/config.toml":    "version = \"5.0\"\nagents_md = false\nname = \"a\"\npresets = [\"claude\"]\n",
				"a/.ai-rulez/rules/r.md":     "# A rule\n",
				"b/c/.ai-rulez/config.toml":  "version = \"5.0\"\nagents_md = false\nname = \"c\"\npresets = [\"cursor\", \"codex\"]\n",
				"b/c/.ai-rulez/context/c.md": "# C\n",
			},
			git:   true,
			steps: []step{run("-q", "generate", "--yes", "--recursive"), run("-q", "generate", "--recursive", "--check")},
		},
		{
			name: "rule-collision",
			files: merge(richFiles([]string{"claude", "cursor", "copilot"}, ""), map[string]string{
				".claude/rules/always.md":                         "# hand written\n",
				".cursor/rules/targeted.mdc":                      "hand written\n",
				".github/instructions/typescript.instructions.md": "hand written\n",
			}),
			exec:  scriptExec(),
			git:   true,
			steps: []step{runEnv(goldenEnv, "generate", "--yes"), runEnv(goldenEnv, "generate", "--yes"), run("clean", "--yes")},
			full:  false,
		},
	}
}
