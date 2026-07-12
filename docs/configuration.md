# Configuration Reference

V4 configuration reference for `.ai-rulez/config.toml` (defaults to TOML format).

## File-Based Configuration

V4 uses a file-based approach where you edit files directly with your editor or use CRUD commands:

- **Configuration**: Edit `.ai-rulez/config.toml` (TOML format) with any text editor
- **Rules**: Add/edit `.ai-rulez/rules/*.md` files or use `ai-rulez add rule`
- **Context**: Add/edit `.ai-rulez/context/*.md` files or use `ai-rulez add context`
- **Skills**: Add/edit `.ai-rulez/skills/{name}/SKILL.md` files or use `ai-rulez add skill`
- **Agents**: Add/edit `.ai-rulez/agents/*.md` files or use `ai-rulez add agent`
- **Domains**: Add/edit `.ai-rulez/domains/{name}/{rules,context,skills,agents}/*.md` files or use `ai-rulez domain add`
- **MCP Servers**: Inline in `.ai-rulez/config.toml` (no separate mcp.yaml file)

You can either directly edit files with your editor or use CRUD commands for programmatic modification. After changes, run `ai-rulez generate` to create tool-specific outputs.

## Basic Structure

The minimal valid V4 configuration:

```toml
version = "4.0"
name = "my-project"
```

A typical production configuration:

```toml
version = "4.0"
name = "my-project"
description = "My project description"

presets = ["claude", "cursor", "gemini"]

default = "full"

[profiles]
full = ["backend", "frontend", "qa"]
backend = ["backend", "qa"]
frontend = ["frontend", "qa"]

gitignore = true

[[mcp_servers]]
name = "ai-rulez"
command = "npx"
args = ["-y", "ai-rulez@latest", "mcp"]
```

## Required Fields

### `version`

The V4 schema version. Must be `"4.0"` (V3 `"3.0"` is still accepted for backward compatibility).

```toml
version = "4.0"
```

### `name`

The project name. Used in generated files and displayed in headers.

```toml
name = "My Project"
name = "acme-platform"
name = "backend-api"
```

## Optional Fields

### `description`

Brief description of the project or configuration.

```toml
description = "SaaS platform with React frontend and Go backend"
```

### `presets`

Specifies which tools to generate configuration for. Can be built-in preset names or custom preset objects.

#### Built-in Presets

```toml
presets = [
  "claude",       # → CLAUDE.md and .claude/
  "cursor",       # → .cursor/rules/
  "gemini",       # → GEMINI.md and .gemini/
  "copilot",      # → .github/copilot-instructions.md
  "windsurf",     # → .windsurf/
  "continue-dev", # → .continue/
  "cline",        # → .clinerules/
  "codex",        # → AGENTS.md and .codex/
  "amp",          # → AMP.md and .amp/
  "junie",        # → .junie/
  "opencode",     # → OPENCODE.md and .opencode/
  "hermes",       # → .hermes.md
  "antigravity"   # → .agents/
]
```

#### Custom Presets

For tools not in the built-in list:

```toml
[[presets]]
name = "my-tool"
type = "markdown"  # or: directory, json
path = "docs/MY_TOOL.md"
template = """
# {{ .Name }}
{{ range .Rules }}
- **{{ .Name }}**: {{ .Content }}
{{ end }}
"""
```

### `default`

The default profile name used when `ai-rulez generate` is run without `--profile`.

```toml
default = "full"
```

If not specified, all domains are included.

### `profiles`

Named profiles that specify which domains to include in generation.

```toml
[profiles]
full = ["backend", "frontend", "qa"]
backend = ["backend", "qa"]
frontend = ["frontend", "qa"]
qa = ["qa"]
```

Each profile specifies a list of domain names. When generating with a profile:

1. All root content (`.ai-rulez/rules/`, `.ai-rulez/context/`, `.ai-rulez/skills/`, `.ai-rulez/agents/`) is included
2. Content from specified domains (`.ai-rulez/domains/{name}/`) is included

### `scopes`

Generate additional assistant files in subfolders with their own profile.

```toml
[profiles]
frontend = ["frontend"]

[[scopes]]
path = "packages/web"
profile = "frontend"
presets = ["codex", "claude"]
```

Scoped outputs keep root context separate from subfolder context. The default scoped presets are `codex` and `claude`, producing subfolder `AGENTS.md` and `CLAUDE.md` files.

### `gitignore`

Controls whether `ai-rulez` automatically updates `.gitignore` with generated output patterns.

```toml
gitignore = true   # Default: update .gitignore automatically
gitignore = false  # Manual .gitignore management
```

When `true`, generated roots such as `AGENTS.md`, `.mcp.json`, `.claude/`, `.codex/`, `.cursor/`,
and `.agents/` are added to `.gitignore` to prevent accidental commits. GitHub output is narrower:
`ai-rulez` ignores generated `.github/copilot-instructions.md`, `.github/agents/`, `.github/commands/`,
and `.github/skills/` without ignoring all of `.github/`.

### `compact`

Controls whether generated inline rule sections omit per-rule `**Priority:**` annotations.

```toml
compact = false  # Default: include Priority lines
compact = true   # Omit Priority annotations, reduce output size
```

When `true`, generated presets (CLAUDE.md, GEMINI.md, copilot-instructions.md, etc.) omit the per-rule
`**Priority:**` lines, reducing file size for large rule sets. Applies to all configured presets.

### `mcp_servers`

Inline MCP (Model Context Protocol) server definitions. No separate `mcp.yaml` file needed.

```toml
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

Each entry supports:

| Field         | Required | Description                                                                         |
| ------------- | -------- | ----------------------------------------------------------------------------------- |
| `name`        | Yes      | Unique server identifier                                                            |
| `description` | No       | Human-readable description of the server                                            |
| `command`     | No       | Command to run for local `stdio` servers (npx, uvx, ai-rulez, etc.)                 |
| `args`        | No       | Array of command arguments for local servers                                        |
| `env`         | No       | Environment variables as key-value pairs. Values may contain `${VAR}` placeholders. |
| `transport`   | No       | `stdio`, `http`, or `sse`. Defaults to `stdio`.                                     |
| `url`         | No       | Remote MCP server URL for `http` or `sse` transports.                               |
| `enabled`     | No       | Set to `false` to skip the server in generated MCP outputs. Defaults to `true`.     |

Local `stdio` servers normally need `command`; remote `http` and `sse` servers normally use `url`.

MCP env placeholders use `${VAR}` syntax, where `VAR` must match `[A-Za-z_][A-Za-z0-9_]*`. They are
resolved during `ai-rulez generate` from repeated `--env KEY=VALUE` flags, process environment
variables, then dotenv files. By default, `ai-rulez` loads `.env` from the generation base directory.
If any `--env-file PATH` flags are supplied, the default `.env` is not loaded; files are merged in
flag order, with later files winning. Generation fails if a placeholder cannot be resolved.

Generated MCP config files contain resolved values. If any resolved MCP env value comes from a
placeholder, or if an env key looks sensitive (`TOKEN`, `SECRET`, `PASSWORD`, `KEY`, `CREDENTIAL`),
generation fails before writing unless the generated MCP config path is covered by `.gitignore` or
by the patterns `ai-rulez` is about to add. Protected MCP output paths are `.mcp.json`,
`.claude/settings.json`, `.gemini/settings.json`, and `.agents/settings.json`, including scoped
variants such as `packages/web/.claude/settings.json`. Resolved secret values are redacted before
source-hash calculation, but generated MCP config files contain the actual resolved values.

### `plugins`

Plugin configuration for extending AI-Rulez functionality.

```toml
[[plugins]]
name = "my-plugin"
source = "https://github.com/org/plugin"
version = "1.0.0"
```

### `marketplaces`

Marketplace integrations for discovering and installing extensions.

```toml
[[marketplaces]]
name = "official"
url = "https://marketplace.ai-rulez.io"
enabled = true
```

### `builtins`

Enables built-in domains that ship embedded in the `ai-rulez` binary. These provide opinionated rules, context, and commands without needing external includes.

#### Enable all builtins

```toml
builtins = true
```

#### Disable all builtins (including auto-includes)

```toml
builtins = false
```

#### Enable specific builtins

```toml
builtins = ["rust", "python", "pyo3", "security", "git-workflow", "default-commands"]
```

#### Exclude auto-included builtins

`ai-governance` is auto-included whenever builtins are configured. Exclude it with `!`:

```toml
builtins = ["rust", "!ai-governance"]
```

#### Exclude specific rules from a builtin domain

When using the array form of `builtins`, you can exclude a single rule from a domain while keeping the rest:

```toml
builtins = ["git-workflow", "!git-workflow/commit-messages"]
```

This loads all rules from `git-workflow` except the `commit-messages` rule. The syntax is `!domain/rule`.
Per-rule exclusion only works with array form; `builtins = true` does not support per-rule exclusion.

#### Available Built-in Domains

Run `ai-rulez builtins list` to see all available domains.

**Universal** (language-agnostic):

| Domain              | Description                                          |
| ------------------- | ---------------------------------------------------- |
| `ai-governance`     | AI agent behavior governance (auto-included)         |
| `security`          | Security best practices and OWASP reference          |
| `git-workflow`      | Git workflow and commit conventions                  |
| `code-quality`      | Code readability, error handling, and complexity     |
| `testing`           | Testing conventions and best practices               |
| `token-efficiency`  | Output efficiency and task automation                |
| `documentation`     | Documentation standards and maintenance              |
| `polyglot-bindings` | Cross-language binding and native FFI conventions    |
| `default-commands`  | Built-in slash commands (`/iterate`, `/parallelize`) |

**Languages** (per-language conventions):

`rust`, `python`, `typescript`, `go`, `java`, `ruby`, `php`, `elixir`, `csharp`

**Bindings** (FFI binding conventions):

`pyo3`, `napi-rs`, `magnus`, `ext-php-rs`, `rustler`, `wasm`

#### What Builtins Provide

**Universal builtins** include opinionated rules for their domain:

- `ai-governance`: Read-before-write, verify-before-acting, minimal changes, explain reasoning
- `security`: Input validation, secrets handling, least privilege, dependency awareness (with per-language audit tool recommendations)
- `git-workflow`: Conventional commits, atomic commits, branch hygiene, safe operations
- `code-quality`: Complexity limits, error handling, readability, dead code removal, duplication
- `testing`: TDD workflow, test independence, meaningful assertions, descriptive test naming
- `token-efficiency`: Task runner preference, output awareness
- `documentation`: Inline docs, README standards, docs-with-code updates
- `polyglot-bindings`: Rust-core/native ABI boundaries, FFI ownership, cross-language error conversion, binding parity
- `default-commands`: `/iterate` (implementation + review cycles) and `/parallelize` (subagent task splitting) slash commands

**Language builtins** each provide a comprehensive conventions rule covering:

- Target language version and edition
- Linting and formatting tools (e.g., `ruff` for Python, `oxfmt`/`oxlint` for TypeScript, `clippy` for Rust)
- Static analysis and type checking (e.g., `mypy --strict`, `PHPStan level 9`, `Dialyzer`)
- Security/SAST tools (e.g., `bandit`, `gosec`, `cargo audit`, `bundler-audit`)
- Testing framework and coverage tools with 80%+ threshold
- Package manager and lockfile conventions
- Benchmarking and profiling tools
- Anti-patterns specific to the language

**Binding builtins** provide FFI-specific conventions for Rust binding crates:

- Macro usage patterns (e.g., `#[pyclass]`, `#[napi]`, `#[rustler::nif]`)
- Error mapping between Rust and target language
- Build and distribution workflow
- Performance considerations (GIL release, scheduler safety, bundle size)
- Anti-patterns (no panics, no blocking, thin wrapper principle)

#### Merge Priority

Builtins have the **lowest** priority. Content is merged in this order:

1. **Builtins** (lowest) — embedded in binary
2. **Includes** — from git repos or local paths
3. **Local content** (highest) — in your `.ai-rulez/` directory

If a local domain has the same name as a builtin, the local domain is used and the builtin is skipped entirely.

### `defaults`

Top-level defaults that propagate into generated outputs when individual content files do not override them.

```toml
[defaults]
effort = "medium"  # low | medium | high | xhigh | max | inherit

[defaults.effort_by_preset]
codex = "high"
claude = "xhigh"
amp = "max"

[defaults.model_by_preset]
claude = "opus"
copilot = "gpt-5"
cursor = "claude-3.7-sonnet"
```

**`defaults.effort`** sets the reasoning effort applied to every preset that supports it. Per-agent overrides (via agent frontmatter) win where the preset accepts per-agent effort; if neither is set, the field is omitted entirely.

**`defaults.effort_by_preset`** lets you override `defaults.effort` for specific presets. Per-agent metadata still wins. Useful when, for example, you want Codex to reason harder than Claude on the same project.

**`defaults.model_by_preset`** sets the agent `model` value per preset. Model strings are provider-specific (`opus` makes sense for Claude, `gpt-5` for Copilot) so there is no provider-neutral `defaults.model` scalar — every entry is preset-scoped. Per-agent `<preset>_model` frontmatter still wins; the legacy single-value `model` field on an agent acts as the lowest-priority fallback. Presets that do not emit a per-agent model frontmatter (`codex`, `amp`, `antigravity`, `junie`) ignore entries for their preset.

**Resolution order** (per preset, per agent):

1. Per-agent `effort` in agent frontmatter (Claude, Codex, Windsurf, Opencode — presets that support per-agent effort)
2. `defaults.effort_by_preset[<preset>]`
3. `defaults.effort`
4. Omit

For models the order is:

1. Per-agent `<preset>_model` in agent frontmatter (e.g. `claude_model`, `copilot_model`)
2. `defaults.model_by_preset[<preset>]`
3. Per-agent legacy `model` field
4. Omit

**Per-preset support matrix** — each preset accepts a different vocabulary, and ai-rulez maps your value to the closest tier the preset supports:

| Preset                                                                         | Where it's emitted                                                              | Field                    | Notes                                                                                                                                                                              |
| ------------------------------------------------------------------------------ | ------------------------------------------------------------------------------- | ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `claude`                                                                       | `.claude/agents/<id>.md` frontmatter                                            | `effort`                 | Per-agent. Full vocabulary including `max` and `inherit`.                                                                                                                          |
| `codex`                                                                        | `.codex/agents/<id>.toml` (per-agent) and `.codex/config.toml` (global default) | `model_reasoning_effort` | Per-agent override beats global `.codex/config.toml`. `max` → `high`; `inherit` dropped.                                                                                           |
| `amp`                                                                          | `.amp/settings.json`                                                            | `amp.anthropic.effort`   | Global only. `xhigh` → `high`.                                                                                                                                                     |
| `windsurf`                                                                     | `.windsurf/agents/<id>.md` frontmatter                                          | `reasoning_effort`       | Per-agent. `max` → `high`; `inherit` dropped.                                                                                                                                      |
| `opencode`                                                                     | `.opencode/agents/<id>.md` frontmatter                                          | `reasoningEffort`        | Per-agent. `xhigh` and `max` → `high`; `inherit` dropped.                                                                                                                          |
| `cursor`, `copilot`, `gemini`, `junie`, `antigravity`, `cline`, `continue-dev` | —                                                                               | —                        | These tools either gate effort behind UI toggles or read it from user-managed config files. ai-rulez does not emit anything for them; configure effort in the tool's own settings. |

**Per-preset model matrix** — presets that emit a `model` value in their agent frontmatter:

| Preset         | Per-agent frontmatter key | Emitted field                                |
| -------------- | ------------------------- | -------------------------------------------- |
| `claude`       | `claude_model`            | `model` in `.claude/agents/<id>.md`          |
| `copilot`      | `copilot_model`           | `model` in `.github/agents/<id>.agent.md`    |
| `cursor`       | `cursor_model`            | `model` in `.agents/agents/<id>.md`          |
| `cline`        | `cline_model`             | `model` in `.cline/agents/<id>.md`           |
| `opencode`     | `opencode_model`          | `model` in `.opencode/agents/<id>.md`        |
| `windsurf`     | `windsurf_model`          | `model` in `.windsurf/agents/<id>.md`        |
| `continue-dev` | `continue-dev_model`      | `model` in `.continue/agents/<id>.md`        |
| `gemini`       | `gemini_model`            | `model` in `.agents/agents/<id>.md` (Gemini) |

### `header`

Configures the style of headers in generated files. Headers provide context about ai-rulez, explain the folder structure, and instruct AI agents on proper usage.

```toml
[header]
style = "detailed"  # Default: comprehensive header with full documentation
# style = "compact"  # Shorter header with key information
# style = "minimal"  # Bare minimum header
```

#### Header Styles

**`detailed`** (default)

- Comprehensive explanation of ai-rulez
- Complete folder organization documentation
- Full AI agent instructions with MCP server promotion
- Best for: projects where AI agents need thorough context
- Size: ~50 lines

**`compact`**

- Condensed version with essential information
- Uses symbols (✗/✓) for clarity
- Brief structure overview
- Best for: projects where file size matters
- Size: ~20 lines

**`minimal`**

- Only critical information
- Brief "DO NOT EDIT" warning
- MCP server reference
- Best for: projects with strict file size requirements
- Size: ~10 lines

#### Header Example (detailed)

```html
<!--
🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT DIRECTLY
Project: My Project
Generated: 2026-01-03 09:27:19
Source: .ai-rulez/config.toml
Target: CLAUDE.md
Content: rules=5, sections=0, agents=2

WHAT IS AI-RULEZ
AI-Rulez is a directory-based AI governance tool. All configuration lives in
the .ai-rulez/ directory. This file is auto-generated from source files.

.AI-RULEZ FOLDER ORGANIZATION
Root content (always included):
  .ai-rulez/config.toml    Main configuration (presets, profiles)
  .ai-rulez/rules/         Mandatory rules for AI assistants
  .ai-rulez/context/       Reference documentation
  .ai-rulez/skills/        Specialized AI prompts
  .ai-rulez/agents/        Agent definitions

Domain content (profile-specific):
  .ai-rulez/domains/{name}/rules/    Domain-specific rules
  .ai-rulez/domains/{name}/context/  Domain-specific documentation
  .ai-rulez/domains/{name}/skills/   Domain-specific AI prompts

Profiles in config.toml control which domains are included.

INSTRUCTIONS FOR AI AGENTS
1. NEVER edit this file (CLAUDE.md) - it is auto-generated

2. ALWAYS edit files in .ai-rulez/ instead:
   - Add/modify rules: .ai-rulez/rules/*.md
   - Add/modify context: .ai-rulez/context/*.md
   - Update config: .ai-rulez/config.toml
   - Domain-specific: .ai-rulez/domains/{name}/rules/*.md

3. PREFER using the MCP Server (if available):
   Command: npx -y ai-rulez@latest mcp
   Provides safe CRUD tools for reading and modifying .ai-rulez/ content

4. After making changes: ai-rulez generate

5. Complete workflow:
   a. Edit source files in .ai-rulez/
   b. Run: ai-rulez generate
   c. Commit both .ai-rulez/ and generated files

Documentation: https://github.com/Goldziher/ai-rulez
-->
```

#### Header Example (compact)

```html
<!--
🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT
Project: My Project | Generated: 2026-01-03 09:28:08
Source: .ai-rulez/config.toml | Target: CLAUDE.md
Content: rules=5, sections=0, agents=2

WHAT IS AI-RULEZ: Directory-based AI governance. Config in .ai-rulez/

STRUCTURE:
  .ai-rulez/config.toml, rules/, context/, skills/, agents/ (root)
  .ai-rulez/domains/{name}/ (profile-specific)

AI AGENT INSTRUCTIONS:
✗ NEVER edit CLAUDE.md (auto-generated)
✓ EDIT .ai-rulez/rules/*.md, .ai-rulez/context/*.md, .ai-rulez/config.toml
✓ USE MCP server: npx -y ai-rulez@latest mcp (provides CRUD tools)
✓ REGENERATE: ai-rulez generate
✓ COMMIT: both .ai-rulez/ and generated files

Docs: https://github.com/Goldziher/ai-rulez
-->
```

#### Header Example (minimal)

```html
<!--
🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT
Project: My Project
Generated: 2026-01-03 09:28:27
Source: .ai-rulez/config.toml

NEVER edit this file - modify .ai-rulez/ content instead
Use MCP server: npx -y ai-rulez@latest mcp
Regenerate: ai-rulez generate

Docs: https://github.com/Goldziher/ai-rulez
-->
```

### `installed_skills`

Named skills installed from external repositories. Skills are fetched dynamically during `ai-rulez generate` and included in outputs. See [Installed Skills](installed-skills.md) for full details.

```toml
[[installed_skills]]
name = "kreuzberg"
source = "https://github.com/kreuzberg-dev/kreuzberg"

[[installed_skills]]
name = "ai-rulez"
source = "https://github.com/Goldziher/ai-rulez"
ref = "main"

[[installed_skills]]
name = "custom-lib"
source = "https://github.com/org/repo"
path = "libs/custom"       # defaults to skills/<name>
local_override = "../local" # use local path for development
```

Each entry supports:

| Field            | Required | Description                                                       |
| ---------------- | -------- | ----------------------------------------------------------------- |
| `name`           | Yes      | Unique skill identifier                                           |
| `source`         | Yes      | Git URL or local path                                             |
| `path`           | No       | Path within repo to skill directory (defaults to `skills/<name>`) |
| `ref`            | No       | Git ref (branch, tag, commit)                                     |
| `local_override` | No       | Local path override for development                               |

Manage via CLI: `ai-rulez skill install/remove/list`.

## Directory Structure

### Root Content (Always Included)

Content in these directories is always included in every generation:

```text
.ai-rulez/
├── rules/           # Mandatory rules and constraints
├── context/         # Reference documentation
├── skills/          # AI skills/prompts
└── agents/          # Agent prompts
```

**Rules directory**: `rules/`

- Files: `*.md` markdown files
- Purpose: Mandatory constraints, standards, do's and don'ts
- Included: in all generated outputs

**Context directory**: `context/`

- Files: `*.md` markdown files
- Purpose: Reference documentation, architecture, guidelines
- Included: in all generated outputs

**Skills directory**: `skills/`

- Structure: `skills/{skill-name}/SKILL.md`
- Purpose: Specialized AI prompts and expert prompts
- Included: in all generated outputs

**Agents directory**: `agents/`

- Files: `*.md` markdown files
- Purpose: Agent prompt files for supported tools
- Included: in all generated outputs

### Domain Content (Profile-Specific)

Content in domain directories is included only when that domain is in the active profile:

```text
.ai-rulez/domains/
├── backend/
│   ├── rules/
│   ├── context/
│   ├── skills/
│   └── agents/
├── frontend/
│   ├── rules/
│   ├── context/
│   ├── skills/
│   └── agents/
└── qa/
    ├── rules/
    ├── context/
    └── agents/
```

Domain directories mirror the root structure:

- `domains/{name}/rules/` - Domain-specific rules
- `domains/{name}/context/` - Domain-specific documentation
- `domains/{name}/skills/` - Domain-specific AI skills
- `domains/{name}/agents/` - Domain-specific agent prompts

## File Formats

### Markdown Files

All `.md` files are treated as content. Optional YAML frontmatter is supported:

```markdown
---
priority: high
targets:
  - "*.py"
  - "backend/*"
custom_field: value
---

# Rule or Context Title

Your content here. Can include any markdown formatting.
```

### Frontmatter Fields

**`priority`** (optional, string)

- Values: `critical`, `high`, `medium`, `low`, `minimal`
- Default: `medium`
- Controls sort order in generated files (higher priority first)

```yaml
---
priority: critical
---
```

**`targets`** (optional, array of strings)

- File glob patterns specifying which generated outputs include this content
- If empty, included in all outputs

```yaml
---
targets:
  - "CLAUDE.md"
  - ".cursor/rules/*"
---
```

**`effort`** (optional, string — agents only)

- Values: `low`, `medium`, `high`, `xhigh`, `max`, `inherit`
- Sets the reasoning effort for a Claude Code subagent in its generated `.claude/agents/<name>.md` frontmatter
- Available levels depend on the model
- Falls back to `defaults.effort` in `config.toml` when not set, then to the session-level default
- Other presets (Cursor, Windsurf, Copilot, Gemini, etc.) do not currently support this field — it is omitted from their outputs

```yaml
---
name: security-reviewer
description: Reviews code for security regressions
effort: high
---
```

**`extends`** (optional, string — agents only)

- Inherits a lower-precedence agent of the given name and appends this agent's body to it
- See [Extending Agents](#extending-agents) for resolution rules

```yaml
---
name: code-reviewer
extends: code-reviewer
---
```

**Custom fields** (optional)

- Any other YAML fields are preserved and available in custom templates

```yaml
---
priority: high
author: engineering-team
review_date: 2025-01-01
tags: [security, performance]
---
```

### SKILL.md Format

Skills should follow this structure:

```markdown
---
priority: high
description: "Code reviewer expert for quality assurance"
targets: ["CLAUDE.md"]
---

# Code Reviewer Expert

You are an expert code reviewer with deep knowledge of:

- Code quality and maintainability
- Testing best practices
- Performance optimization

## Your Responsibilities

1. Review pull requests for correctness
2. Suggest improvements and refactoring
3. Verify test coverage
```

## Content Merge Strategy

When generating with a profile, content is merged in this order:

1. **Root rules** (`.ai-rulez/rules/`)
2. **Root context** (`.ai-rulez/context/`)
3. **Root skills** (`.ai-rulez/skills/`)
4. **Domain rules** (for each domain in profile)
5. **Domain context** (for each domain in profile)
6. **Domain skills** (for each domain in profile)

Within each category, files are sorted by:

1. **Priority** (critical → high → medium → low → minimal)
2. **Filename** (alphabetical)

### Deduplication by Name

When the same rule or context **name** appears in multiple sources, the generated output includes it only **once**. Precedence (highest to lowest):

1. **Root content** (`.ai-rulez/rules/`, `.ai-rulez/context/`, etc.)
2. **Domain content** (`.ai-rulez/domains/{name}/rules/`, etc.)
3. **Include-sourced content** (`FromInclude` domains from external includes)
4. **Builtin content** (auto-included domains)

Example: If both a builtin `git-workflow` domain and a local rule define `commit-messages`, the local version is used and the builtin version is dropped.

During `ai-rulez generate` and `ai-rulez validate`, a warning is logged for each deduplicated item:

```text
Duplicate rule collapsed name=commit-messages kept=.ai-rulez/rules/commit-messages.md dropped=<builtin>/git-workflow/commit-messages.md
```

### Name Collision Handling

If a filename appears in both root and domain:

```text
.ai-rulez/rules/testing.md
.ai-rulez/domains/backend/rules/testing.md
```

The domain version takes precedence (backend gets the domain-specific version).

A warning is logged if collisions are detected:

```text
⚠️  Content collision: rules/testing.md exists in both root and backend domain
    → Using backend domain version
```

## Extending Agents

Built-in and shared-module agents (`code-reviewer`, `docs-writer`, `security-auditor`, language
specialists, ...) give you a solid base. When you only need to _add_ project-specific guidance, don't
copy the whole agent — **extend** it.

### Agent precedence

Same-named agents resolve deterministically by layer, highest precedence first:

1. **Local** — agents under your `.ai-rulez/agents/`
2. **Include** — agents pulled in from external includes
3. **Builtin** — agents shipped with ai-rulez builtin domains

A higher-precedence agent replaces a lower-precedence one of the same name unless it uses `extends`.

### Append a message with `extends`

Create `.ai-rulez/agents/<name>.md` with the same `name`, set `extends` to that name, and put your
additional instructions in the body:

```markdown
---
name: code-reviewer
extends: code-reviewer
---

Also enforce, for this repo:

- No `.unwrap()`/`.expect()` in library code.
- Every `unsafe` block carries a SAFETY comment.
```

The generated `code-reviewer` is the base agent's full instructions **plus** your message appended. You
did not restate the base checklist — you added to it.

Frontmatter fields you set win over the base; fields you omit are inherited:

```markdown
---
name: docs-writer
extends: docs-writer
model: opus # upgrade just this agent's model
effort: high
# tools omitted → inherited from the base docs-writer
---

When documenting the public API, include a runnable example in every supported language.
```

### How resolution works

`extends: <name>` binds to the same-named (or explicitly named) agent resolved from the layers **below**
the current one:

- A **local** agent extends an **include** or **builtin** agent.
- An **include** agent extends a **builtin** agent.
- Chains compose in resolution order (builtin → include → local), across as many layers as extend one
  another.

If no lower-layer agent of that name exists, the agent degrades to a plain agent (your body only) with
the `extends` directive stripped, and `generate` logs a warning naming the missing target so a typo does
not pass unnoticed. An `extends` cycle degrades the same way.

### Extend vs. redefine

- **Extend** (`extends:`) — you want the base plus a few additions. Preferred; no duplication.
- **Redefine** — omit `extends` and write a complete agent to fully replace the base of that name. A
  higher-precedence agent without `extends` always wins over the base regardless of frontmatter.

## Configuration Examples

### Small Project (Single Team)

```toml
version = "4.0"
name = "My Startup"
description = "Early-stage SaaS with React + Go"

presets = ["claude", "cursor"]

gitignore = true
```

Directory structure:

```text
.ai-rulez/
├── config.toml
├── rules/
│   ├── code-style.md
│   └── testing.md
├── context/
│   └── architecture.md
└── skills/
    └── code-reviewer/
        └── SKILL.md
```

### Medium Project (Multiple Teams)

```toml
version = "4.0"
name = "Enterprise Platform"
description = "Multi-team SaaS platform"

presets = ["claude", "cursor", "gemini"]

default = "full"

[profiles]
full = ["backend", "frontend", "qa", "devops"]
backend = ["backend", "qa"]
frontend = ["frontend", "qa"]
qa = ["qa"]
devops = ["devops"]

gitignore = true
```

Directory structure:

```text
.ai-rulez/
├── config.toml
├── rules/
│   ├── general-standards.md
│   └── security.md
└── domains/
    ├── backend/
    │   ├── rules/
    │   │   ├── api-design.md
    │   │   └── database.md
    │   └── context/
    │       └── backend-architecture.md
    ├── frontend/
    │   ├── rules/
    │   │   ├── component-guidelines.md
    │   │   └── performance.md
    │   └── context/
    │       └── design-system.md
    ├── qa/
    │   └── rules/
    │       └── testing-strategy.md
    └── devops/
        ├── rules/
        │   └── deployment.md
        └── context/
            └── infrastructure.md
```

### Complex Project (Multiple Presets)

```toml
version = "4.0"
name = "Advanced ML Platform"
description = "Research platform with team separation"

presets = ["claude", "cursor", "gemini", "windsurf"]

[[presets]]
name = "internal-guide"
type = "markdown"
path = "docs/AI_DEVELOPMENT_GUIDE.md"

default = "full"

[profiles]
full = ["research", "ml-ops", "infrastructure", "frontend"]
research = ["research"]
ml-ops = ["ml-ops", "infrastructure"]
frontend = ["frontend"]

gitignore = true
```

## Profile Design Patterns

### Single Team (No Domains)

For projects with a single team, skip domains entirely:

```toml
version = "4.0"
name = "simple-project"
presets = ["claude", "cursor"]
```

### Multi-Team Monorepo

For monorepos with multiple independent teams:

```toml
version = "4.0"
name = "platform"
presets = ["claude", "cursor"]

default = "full"

[profiles]
full = ["backend", "frontend", "mobile"]
backend = ["backend"]
frontend = ["frontend"]
mobile = ["mobile"]
```

### Environment-Based Profiles

For different behavior in dev, staging, production:

```toml
version = "4.0"
name = "saas-app"
presets = ["claude"]

default = "production"

[profiles]
development = ["dev-guidelines"]
staging = ["staging-guidelines"]
production = ["production-guidelines", "security-hardened"]
```

## Validation

V4 configurations are validated against the JSON schema:

```text
schema/ai-rules.schema.json
```

To validate your configuration:

```bash
ai-rulez validate
```

This checks:

- `version` is `"4.0"` or `"3.0"` (for backward compatibility)
- `name` is present and non-empty
- All preset names are valid
- File paths are valid
- MCP server definitions are well-formed

## Programmatic Modification with CRUD Operations

V4 provides CRUD (Create, Read, Update, Delete) commands to programmatically modify your configuration. This is useful for:

- Automation and scripting
- Integration with CI/CD pipelines
- Programmatic domain and rule management
- Integration with AI assistants via MCP tools

### Manual File Editing vs CRUD Commands

**Manual file editing:**

- Direct control over content
- Use any text editor
- Better for complex content
- Version control friendly

**CRUD commands:**

- Automated directory structure creation
- Frontmatter generation
- Validation built-in
- Easier for scripting and automation
- Better for programmatic access

### Domain Structure with CRUD

When you create a domain with `ai-rulez domain add`, the following structure is automatically created:

```text
.ai-rulez/domains/my-domain/
├── rules/           # Domain-specific rules
├── context/         # Domain-specific documentation
└── skills/          # Domain-specific AI skills
```

This mirrors the root structure and allows you to organize content by ownership.

### CRUD Command Categories

**Domain Management:**

```bash
ai-rulez domain add <name>           # Create a domain
ai-rulez domain remove <name>        # Delete a domain
ai-rulez domain list                 # List all domains
```

**Content Management:**

```bash
# Add content to root or domain
ai-rulez add rule <name>             # Create a rule
ai-rulez add context <name>          # Create context
ai-rulez add skill <name>            # Create a skill

# Remove content
ai-rulez remove rule <name>          # Delete a rule
ai-rulez remove context <name>       # Delete context
ai-rulez remove skill <name>         # Delete a skill

# List content
ai-rulez list rules                  # List all rules
ai-rulez list context                # List all context
ai-rulez list skills                 # List all skills
```

**Include Management:**

```bash
ai-rulez include add <name> <source> # Add an include source
ai-rulez include remove <name>       # Remove an include
ai-rulez include list                # List all includes
```

**Profile Management:**

```bash
ai-rulez profile add <name> <domains>      # Create a profile
ai-rulez profile remove <name>             # Delete a profile
ai-rulez profile set-default <name>        # Set default profile
ai-rulez profile list                      # List all profiles
```

### Frontmatter Generation

When adding content via CRUD commands, frontmatter is automatically generated:

```markdown
---
priority: medium
targets: []
---

Your content here...
```

You can override defaults:

```bash
ai-rulez add rule my-rule --priority high --targets claude,cursor
```

### Best Practices for Configuration Management

1. **Use CRUD for automation**: Scripts, CI/CD pipelines, and programmatic changes
2. **Use file editing for complex content**: When you need fine control over formatting
3. **Organize by domain**: Put domain-specific rules in `domains/name/` directories
4. **Validate after changes**: Run `ai-rulez validate` to check configuration
5. **Regenerate after changes**: Run `ai-rulez generate` to create tool-specific outputs
6. **Commit both sources and outputs**: Version control both `.ai-rulez/` and generated files

### Programmatic Workflow Example

```bash
#!/bin/bash
# Example: Create a new domain with rules

# Create the domain
ai-rulez domain add backend --description "Backend services"

# Add a rule
ai-rulez add rule database-standards --domain backend --priority high

# Add context
ai-rulez add context architecture --domain backend

# Validate
ai-rulez validate

# Generate
ai-rulez generate

# Commit
git add .ai-rulez/ CLAUDE.md .cursor/
git commit -m "chore: add backend domain with database standards"
```

## Best Practices

### Domain Names

Use names that indicate ownership or responsibility:

- `backend`, `frontend`, `mobile` (service boundaries)
- `api`, `database`, `queue` (technical components)
- `auth`, `payments`, `search` (feature areas)

Avoid: `team1`, `team2`, single letters, overly broad names

### Organizing Content

Put content in the domain that owns it. Example:

- `domains/backend/rules/database-standards.md`
- `domains/frontend/rules/accessibility.md`

### Single Responsibility

Each domain should represent one area:

```toml
Good:
[profiles]
full = ["api", "frontend", "infrastructure"]

Bad:
[profiles]
full = ["api-with-db", "frontend-with-build", "infrastructure-and-monitoring"]
```

### Document Domain Purposes

Add comments to `config.toml`:

```toml
# Domains:
# - backend: Go services, REST APIs
# - frontend: React web app
# - mobile: React Native apps
# - devops: Infrastructure, deployment

[profiles]
full = ["backend", "frontend", "mobile", "devops"]
```

## Troubleshooting

### "Profile not found"

```bash
# Check which profiles are defined
ai-rulez validate

# Try generating with an explicit profile
ai-rulez generate --profile backend
```

### "No presets specified"

At least one preset is recommended. Add to config:

```toml
presets = ["claude"]
```

### "Domain not included"

Check your profile configuration:

```toml
# If this profile doesn't include "backend", backend content won't appear
[profiles]
myprofile = ["frontend", "qa"] # backend is missing!
```

### Content not appearing in output

1. Verify domain is in profile:

   ```bash
   ai-rulez validate  # Check profile definitions
   ```

2. Check file location:
   - Root: `.ai-rulez/rules/`, `.ai-rulez/context/`, `.ai-rulez/skills/`
   - Domain: `.ai-rulez/domains/{name}/rules/`, etc.

3. Check for frontmatter errors:
   - Invalid YAML in `---` blocks will skip the file
   - Remove frontmatter to test

4. Regenerate explicitly:

   ```bash
   ai-rulez generate --profile your-profile
   ```

## Next Steps

- **[Getting Started](quick-start.md)**: Quick start and common patterns
- **[CLI Reference](cli.md)**: All commands and flags
- **[Domains & Profiles](domains.md)**: Team organization patterns
