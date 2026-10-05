# Configuration Reference

## config.toml Schema

```toml
version = "4.0"              # Required, must be "4.0"
name = "my-project"           # Required, project name

description = ""              # Optional project description

presets = [                   # Built-in names and/or custom preset tables
  "claude",
  "cursor",
  { name = "custom", type = "markdown", path = "docs/AI_GUIDE.md" },
]

default = ""                  # Default profile name

gitignore = true              # Auto-update .gitignore (default: true)

builtins = true               # Enable all built-in domains
# builtins = false            # Disable all built-in domains
# builtins = ["go", "security", "!ai-governance"]  # Enable specific builtins

[profiles]                    # Profile → domain mappings
backend = ["backend", "shared", "builtin:docker"]  # builtin:<name> scopes a pack to this profile
frontend = ["frontend", "shared"]

[[scopes]]                    # Optional scoped outputs in subfolders
path = "packages/web"
profile = "frontend"
presets = ["codex", "claude"] # Defaults to codex + claude when omitted

[[installed_skills]]          # External skills to pull at generate time
name = "kreuzberg"
source = "https://github.com/kreuzberg-dev/kreuzberg"
path = "skills/kreuzberg"     # Optional (defaults to skills/<name>)
ref = "main"                  # Optional git ref
local_override = "../local"   # Optional local dev path

[[includes]]                  # External content sources
name = "shared-rules"
source = "https://github.com/org/shared-rules"
path = ""                     # Subdirectory within repo
ref = "main"                  # Git ref
include = ["rules", "context", "skills"]  # Content types to import
merge_strategy = "local-override"  # local-override | include-override | error
install_to = ""               # Install as domain (e.g., "domains/backend")
local_override = ""           # Local dev path override

[defaults]
effort = "medium"             # low | medium | high | xhigh | max | inherit
# omit_agent_fields = ["model", "tools"]  # model | effort | tools | description

[defaults.effort_by_preset]   # Per-preset overrides (beat defaults.effort)
codex = "high"
claude = "xhigh"
amp = "max"

[header]
style = "detailed"            # detailed | compact | minimal

[[mcp_servers]]
name = "ai-rulez"
command = "npx"
args = ["-y", "ai-rulez@latest", "mcp"]

[[mcp_servers]]
name = "grafana"
command = "uvx"
args = ["mcp-grafana"]
env = { GRAFANA_URL = "http://localhost:3000", GRAFANA_SERVICE_ACCOUNT_TOKEN = "${GRAFANA_SERVICE_ACCOUNT_TOKEN}" }
```

## Built-in Presets

| Preset       | Output Path                     |
| ------------ | ------------------------------- |
| claude       | CLAUDE.md                       |
| cursor       | .cursor/rules/, .cursor/commands/, .cursor/agents/, .agents/skills/ |
| gemini       | GEMINI.md, .gemini/, .agents/   |
| copilot      | .github/copilot-instructions.md, .github/{skills,agents,commands}/ |
| devin        | AGENTS.md, .devin/              |
| cline        | .clinerules/, .cline/           |
| codex        | AGENTS.md, .agents/skills/ and .codex/ |
| amp          | AGENTS.md and .agents/ (.amp/settings.json) |
| junie        | AGENTS.md and .junie/           |
| opencode     | AGENTS.md, .opencode/, opencode.json |
| hermes       | .hermes.md                      |
| antigravity  | .agents/, GEMINI.md             |
| xum          | AGENTS.md and .xum/             |
| pi           | AGENTS.md, .agents/skills, .pi/agents, .pi/mcp.json |
| baz          | AGENTS.md (root and nested), .agents/skills, .claude/agents |

These are 15 of the 52 built-in presets. The rest: `aiassistant`, `augment`, `bob`, `codebuddy`, `codebuff`,
`codewhale`, `commandcode`, `copilot-cli`, `cortex`, `crush`, `deepagents`, `dsh`, `factory`, `gitlab-duo`, `goose`,
`grok`, `kilo`, `kimi`, `kiro`, `letta`, `mimocode`, `muse`, `omp`, `openclaw`, `poolside`, `qoder`, `qwen`,
`reasonix`, `replit`, `rovodev`, `takt`, `trae`, `vibe`, `warp`, `zcode`, `zed`, `zoocode`. See
`docs/harnesses.md` for the feature matrix. `windsurf` was renamed `devin`; `continue-dev` was removed.

## Hooks and Permissions

```toml
[[hooks]]
event = "PreToolUse"
matcher = "Bash"
[[hooks.hooks]]
script = "scripts/guard.sh"          # or: command = "..."

[permissions]
allow = ["Bash(npm run test:*)"]
ask   = ["Bash(git push:*)"]
deny  = ["Read(./.env)"]
```

Both use Claude Code names and syntax and are translated per harness (`docs/settings.md`, `docs/permissions.md`).
`generate --user` renders a user config into home directories (`docs/user-scope.md`).

## Available Builtins

Language domains: `rust`, `python`, `typescript`, `go`, `java`, `ruby`, `php`, `elixir`, `csharp`, `r`

Cross-language binding domains: `pyo3`, `napi-rs`, `magnus`, `ext-php-rs`, `rustler`, `wasm`, `jni-rs`, `extendr`, `cgo`, `vite-plus`

Practice domains: `ai-governance` (auto), `agent-delegation` (auto), `code-quality` (auto), `testing` (auto), `git-workflow` (auto), `security` (auto), `token-efficiency` (auto), `documentation`, `polyglot-bindings`, `default-commands`, `cicd`, `observability`, `docker`

## Content Frontmatter Fields

```yaml
---
priority: medium # critical | high | medium | low | minimal
targets: # Limit to specific output targets
  - CLAUDE.md
  - .cursor/rules/*
globs: # Path scope (rules); synonym: paths. Keeps the rule out of the root
  - "**/*.tsx" # file and emits it as .claude/rules/ or a Cursor .mdc rule.
aliases: # Alternative names (commands)
  - alias1
usage: "Usage text" # Command usage info
shortcut: "Ctrl+K" # Keyboard shortcut (commands)
category: "Category" # Content category
description: "Desc" # Description (skills, agents)
---
```

## Merge Strategies

- **local-override** (default): Local content wins on name conflicts
- **include-override**: Included content wins on conflicts
- **error**: Fail immediately on any name conflict

## MCP Environment Placeholders

MCP server env values may use `${VAR}` placeholders. Placeholder names must match
`[A-Za-z_][A-Za-z0-9_]*` and are resolved during `ai-rulez generate` from repeated
`--env KEY=VALUE` flags, process environment variables, then dotenv files. By default,
`.env` is loaded from the generation base directory. If `--env-file` is supplied, the
default `.env` is skipped; multiple env files are merged in flag order, with later files
winning.

Generated MCP configs contain resolved values. If an env value came from a placeholder, or
if an env key contains `TOKEN`, `SECRET`, `PASSWORD`, `KEY`, or `CREDENTIAL`, generation
fails before writing unless the generated MCP config path is gitignored or covered by
planned `--gitignore` patterns. Secret values are redacted before source-hash calculation.

## Profile Resolution

1. If `profiles.default` is explicitly defined → use it
2. If no profiles are defined → include root + all domains
3. If profiles exist but `default` is not among them → include root + builtin + FromInclude domains

## Config Directory and Generated Manifest

- `.ai-rulez/` remains the default config root.
- Config discovery also accepts the project-level `.config/` convention: `.config/ai-rulez/config.{toml,yaml,yml,json}`. `.ai-rulez/` wins when both exist at the same level.
- `ai-rulez init --config-dir .config/ai-rulez` scaffolds the `.config/` layout; generated outputs and the managed `.gitignore` block then reference `.config/ai-rulez/`.
- `ai-rulez generate --config-dir <name>` and `validate --config-dir <name>` use another config directory (nested values such as `.config/ai-rulez` are supported).
- `ai-rulez generate <path/to/config.toml>` and `--config <path/to/config.toml>` load that exact file.
- Generation writes `.generated-manifest.json` under the active config directory and removes only stale files listed in that manifest.
- Assistant directories such as `.claude/`, `.codex/`, `.cursor/`, `.gemini/`, `.devin/`, and `.agents/` are not considered fully owned; user settings, hooks, personal skills, and other local files are preserved.

## Scoped Outputs

Use `[[scopes]]` to generate subfolder-specific `AGENTS.md` and `CLAUDE.md` files with separate profile content:

```toml
[profiles]
frontend = ["frontend"]

[[scopes]]
path = "packages/web"
profile = "frontend"
presets = ["codex", "claude"]
```

Root output and scoped output are generated independently. `--dry-run` shows the target paths for both.
