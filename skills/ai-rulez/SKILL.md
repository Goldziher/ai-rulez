---
name: ai-rulez
description: >-
  Manage AI assistant governance rules across 52 harnesses (Claude Code,
  Cursor, Codex, Copilot, Gemini CLI, OpenCode, Devin, and more) using
  ai-rulez. Use when configuring rules, context, skills, checks, hooks,
  permissions, domains, profiles, includes, plugins, or generating
  tool-specific outputs.
license: MIT
metadata:
  author: Goldziher
  version: "5.0.0"
  repository: https://github.com/Goldziher/ai-rulez
---

# AI-Rulez Governance

AI-Rulez centralizes AI assistant governance in a config directory (default `.ai-rulez/`) and generates tool-specific outputs for 52 built-in harness presets (Claude Code, Cursor, Codex, Copilot, Gemini CLI, OpenCode, Devin, and others).

Use this skill when:

- Setting up or modifying `.ai-rulez/` configuration
- Writing rules, context, skills, agents, commands, or checks for AI assistants
- Declaring `[[hooks]]` or `[permissions]` once for every harness
- Generating a personal config into the home directory with `generate --user`
- Configuring domains, profiles, or includes
- Using custom config directory names via `--config-dir`
- Generating outputs for specific AI tools
- Auditing planned writes/deletes with `--dry-run`
- Installing or managing external skills

## Installation

```bash
# npm (no install)
npx ai-rulez@latest

# Python (no install)
uvx ai-rulez

# Go (the module path needs /v5; without it @latest resolves to an old 1.x build)
go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest
```

## Quick Start

```bash
# Initialize a new project
ai-rulez init

# Add rules and context
ai-rulez add rule my-rule --priority high
ai-rulez add context my-context

# Generate outputs for all configured presets
ai-rulez generate

# Regenerate whenever .ai-rulez/ changes
ai-rulez generate --watch

# Validate configuration
ai-rulez validate

# Read-only diagnostics: drift, removed presets, missing tools
ai-rulez doctor
```

## Configuration Structure

```text
.ai-rulez/
  config.toml          # Main configuration
  rules/               # Governance rules (.md files with frontmatter)
  context/             # Contextual information (.md files)
  skills/              # Specialized capabilities (name/SKILL.md plus resources)
  agents/              # Agent definitions (.md files)
  commands/            # Custom commands (.md files)
  checks/              # Code-review guidelines (.md files)
  domains/             # Domain-scoped content
    backend/
      rules/
      context/
      skills/
```

## config.toml

```toml
version = "5.0"
name = "my-project"
description = "Project description"

presets = ["claude", "cursor", "gemini"]
default = "backend"
builtins = ["go", "security", "testing"]

[profiles]
backend = ["backend", "shared"]
frontend = ["frontend", "shared"]

[[includes]]
name = "shared-rules"
source = "https://github.com/org/shared-rules"

[[installed_skills]]
name = "kreuzberg"
source = "https://github.com/kreuzberg-dev/kreuzberg"

[[mcp_servers]]
name = "ai-rulez"
command = "npx"
args = ["-y", "ai-rulez@latest", "mcp"]

[[scopes]]
path = "packages/web"
profile = "frontend"
presets = ["codex", "claude"]
```

## Content Frontmatter

Rules, context, skills, and agents support YAML frontmatter:

```yaml
---
priority: high # critical, high, medium, low, minimal
targets: # Limit to specific presets
  - CLAUDE.md
  - .cursor/rules/*
description: Brief description
---
```

### Agent-only fields

Agent files under `.ai-rulez/agents/` accept additional frontmatter that maps to
Claude Code's subagent spec. Each is optional and only emitted into
`.claude/agents/*.md` (other presets skip fields they do not support):

```yaml
---
name: security-reviewer
description: Reviews code for security regressions
model: opus # haiku | sonnet | opus | inherit
effort: high # low | medium | high | xhigh | max | inherit
permissionMode: default
tools: # Restrict tool access
  - Read
  - Grep
  - Glob
---
```

`effort` controls reasoning depth. Set a project-wide default and per-preset
overrides in `config.toml`:

```toml
[defaults]
effort = "medium"

[defaults.effort_by_preset]
codex = "high"      # Codex applies it globally and per agent
claude = "xhigh"    # Claude applies it as `effort` per agent
amp = "max"         # Amp writes amp.anthropic.effort to .amp/settings.json
```

Resolution order (per preset, per agent): per-agent `effort` →
`defaults.effort_by_preset[<preset>]` → `defaults.effort` → omit.

Per-preset support:

- **Claude**: per-agent in `.claude/agents/*.md` (full vocabulary including `max`; `inherit` is not a Claude value and is dropped)
- **Codex**: global in `.codex/config.toml` and per-agent in `.codex/agents/*.toml` (`max` → `high`; `inherit` dropped)
- **Amp**: global in `.amp/settings.json` (`xhigh` → `high`)
- **Devin**: per-agent in `.devin/agents/*.md` frontmatter (`max` → `high`)
- **Opencode**: per-agent `variant` in `.opencode/agents/*.md` (joins the agent's `model` as `model#variant`)
- Cursor, Copilot, Gemini, Junie, Antigravity, Cline: silently skipped (those tools expose effort via UI toggles or user-managed config files we don't generate).

## Domains and Profiles

Domains group content for different teams or concerns. Profiles select which domains are active.

```bash
ai-rulez domain add backend
ai-rulez add rule api-standards --domain backend
ai-rulez profile add backend-team backend shared
ai-rulez generate --profile backend-team
```

## Includes

Share rules across projects using git repositories or local paths:

```bash
ai-rulez include add shared-rules https://github.com/org/shared-rules
ai-rulez include add local-rules ./path/to/local --merge-strategy include-override
```

## Installed Skills

Install named skills from external repositories:

```bash
ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg
ai-rulez skill install ai-rulez --source https://github.com/Goldziher/ai-rulez
ai-rulez skill list
ai-rulez skill remove kreuzberg
```

Skills are fetched dynamically at generation time and included in outputs.
Skill `references/`, `scripts/`, and `assets/` directories are preserved as separate generated resource files when the target preset supports skill directories.

## Built-in Presets

52 presets are available (`ai-rulez init --help` prints them): `aiassistant`, `amp`, `antigravity`, `augment`, `baz`, `bob`, `claude`, `cline`, `codebuddy`, `codebuff`, `codewhale`, `codex`, `commandcode`, `copilot`, `copilot-cli`, `cortex`, `crush`, `cursor`, `deepagents`, `devin`, `dsh`, `factory`, `gemini`, `gitlab-duo`, `goose`, `grok`, `hermes`, `junie`, `kilo`, `kimi`, `kiro`, `letta`, `mimocode`, `muse`, `omp`, `openclaw`, `opencode`, `pi`, `poolside`, `qoder`, `qwen`, `reasonix`, `replit`, `rovodev`, `takt`, `trae`, `vibe`, `warp`, `xum`, `zcode`, `zed`, `zoocode`, plus the opt-in `okf` preset (an Open Knowledge Format bundle in `docs/okf/`, see `ai-rulez export okf`). The `mcp` preset is a shared utility (the generic `.mcp.json`) invoked automatically when MCP servers are configured.

`windsurf` is now `devin` and `continue-dev` was removed; `ai-rulez doctor` reports both. `docs/harnesses.md` has the per-preset feature matrix (rules folder, skills, agents, commands, MCP, hooks, permissions, checks, user scope).

## Hooks, Permissions, Checks and User Scope

- `[[hooks]]` in `config.toml` renders into the native hooks file of 36 harnesses (a generated plugin module for `opencode`, `kilo`, `mimocode`, `pi` and `amp`). Events, matchers and timeouts are translated per harness; a group a harness cannot express is skipped with a warning. See `docs/settings.md`.
- `[permissions]` (`allow`, `ask`, `deny`, Claude Code rule syntax) is translated for 24 harnesses. A rule that cannot be expressed is skipped, never widened, and an unenforced deny is reported. See `docs/permissions.md`.
- Checks are code-review guidelines in `.ai-rulez/checks/<name>.md` (frontmatter `description`, `severity`, `tools`, `targets`), rendered for `cursor`, `kilo`, `qwen`, `factory`, `rovodev`, `amp`, `augment` and `gitlab-duo`. Manage them with `ai-rulez add|remove|list check`. See `docs/checks.md`.
- `ai-rulez generate --user` renders a user config (`~/.config/ai-rulez`) into the home directories of 47 harnesses; `clean --user` removes what it wrote. See `docs/user-scope.md`.
- Settings files you also edit (JSON, JSONC, TOML, YAML) are merged key by key, so your own keys and comments survive `generate` and `clean`.
- `[guard] generated = true` adds a PreToolUse hook (`ai-rulez guard`) that blocks agent edits to generated files and points at the source under `.ai-rulez/`.

## Lock, Updates and Verification

- `ai-rulez lock` writes `ai-rulez.lock`: commits and digests of remote includes, installed skills and skill sources, plus every authored item and generated output. It is enforced whenever the file exists (`[lock] enforce = false` opts out). `lock --check` verifies offline, `--diff` previews, `--subject` prints the digest to sign with cosign, `generate --locked`/`--frozen` are the CI modes. Exit codes: `0` ok, `1` could not run, `2` drift, `3` served skills left unpinned by the security scan. See `docs/lockfile.md`.
- Sources accept `version = "^1.2"`; `ai-rulez lock --outdated` lists newer tags and `ai-rulez update` moves range pins (codes `AR730`, `AR731`, `AR732`, `AR735`).
- `ai-rulez validate --strict` runs deep content checks with stable `AR` codes (`--fix`, `--since`, `--baseline`, `--format json|sarif|github|junit|markdown`); `ai-rulez scan` runs only the security checks; `validate --explain AR001` explains a rule. See `docs/strict-validation.md`.
- `[[verifiers]]` and `.ai-rulez/verifiers/*.toml` declare deterministic repo checks run by `ai-rulez verifiers run` (`--since`, `--staged`, `--format sarif|junit`).
- `ai-rulez sbom` prints a CycloneDX bill of materials, `ai-rulez catalog` lists every item with owner, version, tokens and lock status (`--html <dir>` writes a static site), `ai-rulez tokens` and `cost` report prompt-token cost, `ai-rulez search <query>` ranks skills.
- `ai-rulez convert` imports existing tool files, a rulesync project or a `skills-lock.json` into `.ai-rulez/` with a lossiness report.

## Roles and Dynamic Skills

- `[[roles]]` map a job to a slice of the content (`domains`, `include`/`exclude` selectors, `skill_mode`); `generate --role <name>` renders it and `ai-rulez roles list|show|resolve` inspects it. See `docs/roles.md`.
- `delivery: static|served|both` on a skill (or `[skills] delivery`) serves a skill over MCP on demand instead of writing it to the harness trees: run `ai-rulez mcp --serve-skills` (`find_skill`, `load_skill`, `list_skill_resources`). `[[skill_sources]]` serve skills from a git repository or directory. See `docs/mcp-server.md`.

A tool that isn't built in can be supported at full parity with a provider-backed custom preset: set `provider = "<project-relative spec.toml>"` and point it at a declarative spec validated against `schema/provider.schema.json`. See the `ai-rulez` references for the spec shape.

## MCP Integration

MCP servers are configured inline in `config.toml` with `[[mcp_servers]]`. ai-rulez exposes an MCP server for AI assistants to read, create, update, validate, dry-run, and generate configuration:

```bash
ai-rulez mcp
```

Configure in your AI tool's MCP settings to enable CRUD operations from within the assistant.

MCP server env values can use `${VAR}` placeholders. `ai-rulez generate` resolves them from `--env`,
process env, and dotenv files, then refuses to write secret-bearing generated MCP configs unless the
target paths are gitignored (`gitignore = true` or `generate --gitignore` adds them to the managed block).

## Plugins and Marketplaces

ai-rulez can package the same source (skills, commands, agents, MCP servers) as distributable plugin bundles and a marketplace index for Claude, Cursor, Codex, Gemini, Kimi, OpenCode, Factory, and Hermes via `ai-rulez generate --plugin`. Consumer `[[plugins]]`/`[[marketplaces]]` arrays declare plugins to install from a marketplace (no output file is written for them: `generate` warns that `[[plugins]]` has no effect; enable plugins through `[claude.settings]` for Claude Code and `.codex/config.toml` for Codex).
