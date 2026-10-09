package golden

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func allScenarios() []scenario {
	var all []scenario
	for _, name := range config.IndividualPresetNames() {
		all = append(all, presetScenario(name))
	}
	all = append(all, otherScenarios()...)
	all = append(all, llmsTxtScenarios()...)
	return all
}

// presetScenario renders the rich project for one preset: generate, generate again
// (a second run must be quiet), check, verify and clean.
func presetScenario(preset string) scenario {
	return scenario{
		name:  "preset-" + preset,
		files: richFiles([]string{preset}, ""),
		exec:  scriptExec(),
		git:   true,
		steps: []step{
			runEnv(goldenEnv, "generate", "--yes"),
			runEnv(goldenEnv, "generate", "--yes"),
			runEnv(goldenEnv, "generate", "--check"),
			run("clean", "--yes"),
		},
	}
}

// checkPresets are the presets that render checks.
var checkPresets = []string{"cursor", "kilo", "qwen", "factory", "rovodev", "amp", "augment", "gitlab-duo"}

func otherScenarios() []scenario {
	all := config.IndividualPresetNames()
	multi := []string{"claude", "cursor", "codex", "gemini", "copilot", "opencode"}
	var out []scenario

	out = append(out,
		scenario{
			name:  "all-presets",
			files: withoutMCP(richFiles(all, "")),
			exec:  scriptExec(),
			git:   true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				runEnv(goldenEnv, "generate", "--yes"),
				runEnv(goldenEnv, "generate", "--check"),
				runEnv(goldenEnv, "generate", "--dry-run"),
				run("clean", "--yes"),
			},
		},
		scenario{
			name:  "all-presets-mcp-conflict",
			files: richFiles(all, ""),
			exec:  scriptExec(),
			git:   true,
			steps: []step{runEnv(goldenEnv, "generate", "--yes")},
		},
		scenario{
			name:  "multi-full",
			files: richFiles(multi, ""),
			exec:  scriptExec(),
			git:   true,
			full:  true,
			steps: []step{runEnv(goldenEnv, "generate", "--yes")},
		},
		scenario{
			name:  "checks-full",
			files: richFiles(checkPresets, ""),
			exec:  scriptExec(),
			git:   true,
			full:  true,
			steps: []step{runEnv(goldenEnv, "generate", "--yes")},
		},
		scenario{
			name:  "agents-md-shared",
			files: richFilesTop([]string{"codex", "claude", "gemini", "cursor", "opencode"}, "agents_md = true", ""),
			exec:  scriptExec(),
			git:   true,
			full:  true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				{kind: stepWrite, path: ".ai-rulez/config.toml", data: richConfig([]string{"codex", "claude", "gemini", "cursor", "opencode"}, "")},
				runEnv(goldenEnv, "generate", "--yes"),
			},
		},
		scenario{
			name:  "rules-inline-compact",
			files: richFilesTop([]string{"claude", "cursor", "copilot", "junie"}, "compact = true", "\n[rules]\nmode = \"inline\"\n"),
			exec:  scriptExec(),
			git:   true,
			full:  true,
			steps: []step{runEnv(goldenEnv, "generate", "--yes")},
		},
	)

	// Header modes and a fixed clock: SOURCE_DATE_EPOCH pins the Generated stamp.
	for _, hdr := range []struct{ name, block string }{
		{"header-full-timestamp", "\n[header]\nstyle = \"detailed\"\ntimestamp = true\n"},
		{"header-content-compact", "\n[header]\nstyle = \"compact\"\nhashes = \"content\"\n"},
		{"header-none-custom", "\n[header]\nhashes = \"none\"\ntext = \"Custom header text.\"\n"},
	} {
		out = append(out, scenario{
			name:  hdr.name,
			files: richFiles([]string{"claude", "cursor", "codex"}, hdr.block),
			exec:  scriptExec(),
			git:   true,
			full:  true,
			steps: []step{
				runEnv(append([]string{"SOURCE_DATE_EPOCH=1700000000"}, goldenEnv...), "generate", "--yes"),
				runEnv(append([]string{"SOURCE_DATE_EPOCH=1800000000"}, goldenEnv...), "generate", "--yes"),
			},
		})
	}

	out = append(out,
		scenario{
			name:  "profiles-domains",
			files: domainFiles(multi),
			exec:  scriptExec(),
			git:   true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				runEnv(goldenEnv, "generate", "--yes", "--profile", "backend"),
				runEnv(goldenEnv, "generate", "--yes", "--profile", "frontend,backend"),
				runEnv(goldenEnv, "generate", "--dry-run", "--profile", "frontend"),
				runEnv(goldenEnv, "generate", "--yes", "--profile", "full"),
				runEnv(goldenEnv, "generate", "--yes", "--profile", "nope"),
				run("list", "rules"),
				run("domain", "list"),
				run("profile", "list"),
			},
		},
		scenario{
			name:  "builtins",
			files: richFilesTop([]string{"claude", "cursor"}, "builtins = [\"go\", \"security\", \"git-workflow\"]", ""),
			exec:  scriptExec(),
			git:   true,
			steps: []step{runEnv(goldenEnv, "generate", "--yes"), run("builtins", "list")},
		},
		scenario{
			name: "roles",
			files: func() map[string]string {
				f := domainFiles([]string{"claude", "codex"})
				f[".ai-rulez/config.toml"] = richConfig([]string{"claude", "codex"}, `
[profiles]
backend = ["backend"]
frontend = ["frontend"]

[role_manifest]
enabled = true

[[roles]]
name = "engineer"
description = "Writes code"
domains = ["backend"]

[roles.skills]
exclude = ["deployer"]

[roles.skill_mode]
"review*" = "name-only"

[[roles]]
name = "web"
extends = "engineer"
domains = ["frontend"]

[roles.skill_mode]
reviewer = "on"
`)
				return f
			}(),
			exec: scriptExec(),
			git:  true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes", "--role", "engineer"),
				runEnv(goldenEnv, "generate", "--yes", "--role", "web"),
				runEnv(goldenEnv, "generate", "--yes"),
				run("roles", "list"),
				run("roles", "show", "web"),
				run("roles", "resolve", "engineer", "--format", "json"),
			},
		},
	)

	out = append(out, localScenarios(multi)...)
	out = append(out, lifecycleScenarios(multi)...)
	out = append(out, userScenarios(all)...)
	out = append(out, miscScenarios(multi)...)
	return out
}
