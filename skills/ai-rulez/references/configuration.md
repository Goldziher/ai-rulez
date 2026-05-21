# Configuration Reference

## config.toml Schema

```toml
version = "4.0"              # Required, must be "4.0"
name = "my-project"           # Required, project name

description = ""              # Optional project description

presets = ["claude", "cursor"]  # Array of built-in presets (strings)

default = ""                  # Default profile name

gitignore = true              # Auto-update .gitignore (default: true)

builtins = true               # Enable all built-in domains
# builtins = false            # Disable all built-in domains
# builtins = ["go", "security", "!ai-governance"]  # Enable specific builtins

[profiles]                    # Profile → domain mappings
backend = ["backend", "shared"]
frontend = ["frontend", "shared"]

[[scopes]]                    # Optional scoped outputs in subfolders
path = "packages/web"
profile = "frontend"
presets = ["codex", "claude"] # Defaults to codex + claude when omitted

[[presets]]
name = "custom"               # Custom preset (object)
type = "markdown"             # markdown | directory | json
path = "docs/AI_GUIDE.md"

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
```

## Built-in Presets

| Preset | Output Path |
|--------|------------|
| claude | CLAUDE.md |
| cursor | .cursor/rules/ |
| gemini | GEMINI.md |
| copilot | .github/copilot-instructions.md |
| continue-dev | .continue/ |
| windsurf | .windsurfrules |
| cline | .clinerules |
| codex | AGENTS.md |
| amp | AMP.md |
| junie | .junie/guidelines.md |
| opencode | OPENCODE.md |

## Available Builtins

Language domains: `rust`, `python`, `typescript`, `go`, `java`, `ruby`, `php`, `elixir`, `csharp`

Cross-language binding domains: `pyo3`, `napi-rs`, `magnus`, `ext-php-rs`, `rustler`, `wasm`

Practice domains: `ai-governance` (auto), `security`, `git-workflow`, `code-quality`, `testing`, `token-efficiency`, `documentation`, `default-commands`

## Content Frontmatter Fields

```yaml
---
priority: medium         # critical | high | medium | low
targets:                 # Limit to specific output targets
  - CLAUDE.md
  - .cursor/rules/*
aliases:                 # Alternative names (commands)
  - alias1
usage: "Usage text"      # Command usage info
shortcut: "Ctrl+K"       # Keyboard shortcut (commands)
category: "Category"     # Content category
description: "Desc"      # Description (skills, agents)
---
```

## Merge Strategies

- **local-override** (default): Local content wins on name conflicts
- **include-override**: Included content wins on conflicts
- **error**: Fail immediately on any name conflict

## Profile Resolution

1. If `profiles.default` is explicitly defined → use it
2. If no profiles are defined → include root + all domains
3. If profiles exist but `default` is not among them → include root + builtin + FromInclude domains

## Config Directory and Generated Manifest

- `.ai-rulez/` remains the default config root.
- `ai-rulez generate --config-dir <name>` and `validate --config-dir <name>` use another config directory.
- `ai-rulez generate <path/to/config.toml>` and `--config <path/to/config.toml>` load that exact file.
- Generation writes `.generated-manifest.json` under the active config directory and removes only stale files listed in that manifest.
- Assistant directories such as `.claude/`, `.codex/`, `.cursor/`, `.gemini/`, `.windsurf/`, and `.agents/` are not considered fully owned; user settings, hooks, personal skills, and other local files are preserved.

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
