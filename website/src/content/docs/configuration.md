---
title: "Configuration reference"
description: ".ai-rulez/config.toml declares which tools ai-rulez generates for, which content each profile selects, and how the generated files look."
---

`.ai-rulez/config.toml` declares which tools ai-rulez generates for, which content each profile selects, and how the generated files look. Keys are grouped by what they control; features with a page of their own get a short summary, their keys and a link.

## Minimal configuration

`version` and `name` are the only required keys. `presets` names the tools to generate for:

```toml
version = "5.0"
name = "acme"
presets = ["claude", "cursor", "codex", "copilot"]
```

```bash
ai-rulez generate
```

With one rule in `.ai-rulez/rules/`, this writes a shared `AGENTS.md` (the rules and context), a `CLAUDE.md` that imports `@AGENTS.md`, the `ai-rulez` MCP server entry in `.mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json` and `.codex/config.toml`, and a `SessionStart` hook in `.claude/settings.json`. Commit the generated files with the sources; set [`gitignore = true`](#gitignore) to keep them out of git instead.

`ai-rulez init` scaffolds the directory and a commented `config.toml`. To keep only a configuration file and no content tree, see [Config without a .ai-rulez folder](/ai-rulez/config-without-folder/).

## Content layout

The configuration sits in the config directory next to the content it renders:

```text
.ai-rulez/
├── index.md             # OKF index (okf_version), one per directory
├── config.toml          # this reference
├── config.local.toml    # optional machine-local overlay (gitignored)
├── rules/*.md           # always-on or path-scoped instructions
├── context/*.md         # reference material
├── skills/<name>/SKILL.md
├── agents/*.md
├── commands/<name>.md   # or commands/<name>/COMMAND.md
├── checks/*.md          # code-review guidelines
├── domains/<name>/      # same folders, selected by profiles
└── local/               # machine-local content (gitignored)
```

| Directory | Holds | See |
| --- | --- | --- |
| `rules/` | Instructions, always-on or scoped with `globs`/`paths` | [Rules](/ai-rulez/rules/) |
| `context/` | Reference documentation rendered beside the rules | [Rules](/ai-rulez/rules/) |
| `skills/<name>/` | `SKILL.md` plus optional `references/`, `scripts/` and `assets/` | [Skill frontmatter](/ai-rulez/skills/) |
| `agents/` | Subagent definitions | [Agents](#agents) |
| `commands/` | Slash commands, flat (`deploy.md`) or directory form (`deploy/COMMAND.md` with resources) | [Commands](#commands) |
| `checks/` | Code-review guidelines | [Checks](/ai-rulez/checks/) |
| `domains/<name>/` | The same folders for one area, included when a profile selects the domain | [Domains and profiles](/ai-rulez/domains/) |
| `local/` | Personal content that is never committed | [Local configuration](/ai-rulez/local-overrides/) |

`ai-rulez init` creates the tree as an [OKF](/ai-rulez/okf/) bundle, with an `index.md` in each directory and item frontmatter under `x-ai-rulez.metadata`; `add`, `remove` and the MCP tools keep the indexes current. A tree without a root `index.md` still loads, with a deprecation notice on every command: run `ai-rulez migrate okf` (see [The native tree](/ai-rulez/okf/#the-native-tree)).

The skill or command directory name is its id. Files under `references/`, `scripts/` and `assets/` are copied next to the generated `SKILL.md` and listed in its `## Resources` section; another subdirectory produces a warning. Build artifacts are left out; see [`bundle_exclude`](#bundle_exclude).

Instead of `.ai-rulez/`, content can come from `AGENTS.md` and `.agents/` ([`source = "agents"`](#source)), and the config directory can be `.config/ai-rulez/` (`ai-rulez init --config-dir .config/ai-rulez`).

### Symlinks in content

A symlinked file or directory under `.ai-rulez/` (including `domains/` and `local/`) is followed only when its resolved target is inside the project (the git top-level, otherwise the directory holding `.ai-rulez/`). Any other symlink, including a dangling one, is refused: `generate` fails naming it and `validate` reports it as an error. Content read from includes, installed skills, skill sources and OKF bundles never follows symlinks; see [Includes](/ai-rulez/includes/).

### Agents

An agent is a Markdown file in `agents/` (or `domains/<name>/agents/`) whose frontmatter configures it and whose body is its prompt. Like every file in the tree it uses the [OKF form](/ai-rulez/okf/#the-native-tree): `type`, then the agent's keys under `x-ai-rulez.metadata`; `description` stays at the top level. Each preset that supports subagents writes it in its own format (`.claude/agents/<id>.md`, `.codex/agents/<id>.toml`, `.cursor/agents/<id>.md`, ...).

```markdown
<!-- .ai-rulez/agents/security-reviewer.md -->
---
type: Reference
title: Security Reviewer
description: Reviews diffs for security regressions
x-ai-rulez:
  kind: agent
  id: security-reviewer
  metadata:
    tools: [Read, Grep, Glob]
    model: sonnet
    claude_model: opus
    effort: high
    skills: [secure-coding]
---

Review the diff for injection, leaked secrets and unsafe deserialization.
```

| Key | Description |
| --- | --- |
| `x-ai-rulez.id` | Agent id. Defaults to the file name. |
| `description` | When to use the agent; written to every preset and listed in the `## Agents` roster. |
| `tools` | Tool names (Claude Code names; presets translate or drop them). |
| `model` | Model used for every preset that writes one, unless something more specific is set. |
| `<preset>_model` | Model for one preset (`claude_model`, `copilot_model`, `opencode_model`, ...); wins over [`[defaults] model_by_preset`](#defaults) and `model`. |
| `effort` | `low`, `medium`, `high`, `xhigh`, `max` or `inherit`; see [`[defaults]`](#defaults) for how each preset maps it. |
| `skills` | Skills the agent preloads. `validate` reports a name that is no skill as `AR302`. |
| `extends` | Append this agent to a lower-precedence agent of that name; see [Extending agents](#extending-agents). |
| `priority`, `targets` | Ordering and output selection, as for rules ([Targets](/ai-rulez/rules/#targets)). |

The `claude` preset also passes Claude Code's subagent keys through with their YAML types: `disallowedTools`, `permissionMode`, `memory`, `maxTurns`, `mcpServers`, `hooks`, `background`, `isolation`, `color`, `initialPrompt`, `omitClaudeMd`; other presets ignore them. For OpenCode, `hidden` is written as a boolean and `temperature` and `top_p` as numbers. `validate` reports a key no tool reads as `AR303`; list a custom key in [`[lint] allowed_keys`](/ai-rulez/strict-validation/) to silence it.

### Commands

A command is a slash command, in one of two forms:

```text
.ai-rulez/commands/
├── pr-summary.md            # flat form
└── deploy/                  # directory form
    ├── COMMAND.md
    └── references/
        └── runbook.md
```

```markdown
<!-- .ai-rulez/commands/deploy/COMMAND.md -->
---
type: Reference
title: Deploy
description: Deploy the current branch to staging
x-ai-rulez:
  kind: command
  id: deploy
  metadata:
    argument-hint: "[service]"
---

Deploy $ARGUMENTS to staging, following references/runbook.md.
```

The directory form takes `references/`, `scripts/` and `assets/` like a skill. Keeping both `deploy.md` and `deploy/COMMAND.md` is an [output ID collision](#output-id-collisions). The frontmatter (under `x-ai-rulez.metadata`) takes the Claude Code skill and command keys (`description`, `argument-hint`, `arguments`, `allowed-tools`, `model`, `disable-model-invocation`, ...) plus `priority` and `targets`. Harnesses that run commands as skills get one (`.claude/skills/deploy/SKILL.md` with its `references/`, `.agents/skills/deploy/` for Codex); others get their command file (`.cursor/commands/deploy.md`). [Supported harnesses](/ai-rulez/harnesses/) lists where each preset writes commands.

## Configuration discovery

Without `--config`, the CLI walks up from the working directory, stopping at the repository root and never going above the home directory. In each directory it tries, in order:

1. `.agents/ai-rulez.toml`: a project whose content is [`AGENTS.md` and `.agents/`](/ai-rulez/agents-source/)
2. `.ai-rulez/config.toml`: the default layout
3. `.config/ai-rulez/config.toml`: the [`.config/` convention](https://github.com/pi0/config-dir)
4. `ai-rulez.toml`: a [configuration without a `.ai-rulez/` folder](/ai-rulez/config-without-folder/)

The first match wins; when a config directory and `ai-rulez.toml` sit side by side, the file is ignored with a warning. `--config <file or dir>` selects a configuration exactly, and `--config-dir <name>` selects a config directory name below the working directory.

Only `config.toml` (TOML) is read, and its `version` must be `"5.0"`. A project with only a `config.yaml`, `config.yml`, `config.json`, a flat `ai-rulez.yaml`, or a `config.toml` with an older `version` fails with an error naming the file; run `ai-rulez migrate v5`. See [Migrating to v5](/ai-rulez/migration-v5/).

## Checks `generate` runs before it writes

`generate` checks `config.toml`, its layers and `config.local.toml` against the [JSON schema](/ai-rulez/schema/), as `validate` does. An unknown key has no effect, so `generate` warns and names the nearest known key:

```text
WARN  Configuration problem: .ai-rulez/config.toml: unknown key "lock.enforc" (did you mean "enforce"?)
WARN  These keys have no effect; `ai-rulez validate` fails on them, `generate --strict-config` (or AI_RULEZ_STRICT=1) does too
```

`validate` always fails on an unknown key. `generate --strict-config`, or `AI_RULEZ_STRICT=1`, makes `generate` exit 1 too. `validate --strict` is a different switch: it makes warnings fail as well as errors (`--fail-on warning`); see [Strict validation](/ai-rulez/strict-validation/). In CI, run:

```bash
ai-rulez generate --check   # committed outputs match the sources
ai-rulez validate           # unknown keys, content and security checks
```

See [GitHub Actions](/ai-rulez/github-actions/) for a ready-made workflow.

`generate` also lists the commands it is about to write that your tools will run: `[[hooks]]` commands (with the digest of a hook `script`), `http` and `prompt` hooks, command-based `[[mcp_servers]]`, `[permissions] allow` rules, `[claude.settings.managed] env` entries, and plugin enablement and marketplace registration:

```text
Warning: generate will write 1 new or changed command(s) that your tools will run (from .ai-rulez/config.toml):
  hook       SessionStart: sh -c 'case "${CLAUDE_CODE_REMOTE:-}" in ...  -> claude
Review them before you start a tool in this project. Silence with --yes or AI_RULEZ_ACK_COMMANDS=1.
```

Only commands that are new or changed since the last run on this machine are listed (every command on a fresh clone). The previous run is remembered in the user cache directory, never in the repository. The list is printed even with `--quiet`; control characters and anything that looks like a credential are escaped or shown as `<redacted>`. `--yes` or `AI_RULEZ_ACK_COMMANDS=1` silences it, and the MCP `generate_outputs` tool returns it as `new_commands`.

## Key index

Every top-level key and the section that documents it.

| Key | Type | Section |
| --- | --- | --- |
| `version`, `name`, `description`, `schema`, `$comment` | string | [Project](#project) |
| `presets` | array | [Presets](#presets) |
| `default`, `profiles` | string, table | [`default` and `profiles`](#default-and-profiles) |
| `scopes` | array of tables | [`[[scopes]]`](#scopes) |
| `roles`, `role_manifest` | array of tables, table | [`[[roles]]` and `[role_manifest]`](#roles-and-role_manifest) |
| `source`, `agents_md_split` | string, boolean | [`source`](#source) |
| `agents_md` | boolean | [`agents_md`](#agents_md) |
| `skip_outputs` | array | [`skip_outputs`](#skip_outputs) |
| `claude` | table | [`[claude]`](#claude) |
| `codex`, `codex_skills_dir` | table, string | [`[codex]` and `codex_skills_dir`](#codex-and-codex_skills_dir) |
| `compact` | boolean | [`compact`](#compact) |
| `bundle_exclude` | array | [`bundle_exclude`](#bundle_exclude) |
| `recursive_exclude` | array | [`recursive_exclude`](#recursive_exclude) |
| `gitignore` | boolean | [`gitignore`](#gitignore) |
| `gitignore_exclude` | array | [`gitignore_exclude`](#gitignore_exclude) |
| `header` | table | [`[header]`](#header) |
| `rules` | table | [`[rules]`](#rules) |
| `defaults` | table | [`[defaults]`](#defaults) |
| `builtins` | boolean or array | [`builtins`](#builtins) |
| `mcp` | table | [`[mcp]`](#mcp) |
| `mcp_servers` | array of tables | [`[[mcp_servers]]`](#mcp_servers) |
| `hooks`, `permissions`, `guard`, `session_start` | array, table, table, table | [Hooks, permissions, guard and session_start](#hooks-permissions-guard-and-session_start) |
| `includes` | array of tables | [`[[includes]]`](#includes) |
| `installed_skills` | array of tables | [`[[installed_skills]]`](#installed_skills) |
| `installed_marketplaces`, `installed_plugins` | array of tables | [`[[installed_marketplaces]]` and `[[installed_plugins]]`](#installed_marketplaces-and-installed_plugins) |
| `skills`, `domains`, `skill_sources` | table, table, array | [Served skills](#served-skills) |
| `delivery` | table | [`[delivery.stub]`](#deliverystub) |
| `lock` | table | [`[lock]`](#lock) |
| `plugins`, `marketplaces` | array of tables | [`[[plugins]]`](#plugins) |
| `enabled_plugins` | array of tables | [`[[enabled_plugins]]`](#enabled_plugins) |
| `placement` | table | [`[placement]`](#placement) |
| `plugin`, `plugins_authoring`, `marketplace`, `marketplaces_authoring` | table, array, table, array | [`[plugin]`](#plugin) |
| `lint` | table | [`[lint]`](#lint) |
| `verifiers`, `verifiers_settings` | array, table | [`[[verifiers]]`](#verifiers) |
| `exposure` | table | [`[exposure]`](#exposure) |
| `okf` | table | [`[okf]`](#okf) |
| `llms_txt` | table | [`[llms_txt]`](#llms_txt) |
| `ard`, `publish`, `catalog` | table | [`[ard]`, `[publish]` and `[catalog]`](#ard-publish-and-catalog) |
| `governance`, `signing` | table | [Governance and signing](#governance-and-signing) |
| `llm`, `search`, `review`, `judge`, `improve` | table | [Model features](#model-features) |
| `telemetry`, `usage` | table | [Telemetry and usage](#telemetry-and-usage) |
| `default_layers` | array | [Layered configuration](#layered-configuration) |

## Project

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `version` | string | required | Configuration version. Must be `"5.0"`. |
| `name` | string | required | Project name, shown in generated headers and used as the title of root instruction files. |
| `description` | string | none | Project description, written under the title. |
| `schema` | string | none | JSON schema reference for editors (the TOML spelling of `$schema`). |
| `$comment` | string | none | Free-form note, ignored. |

```toml
version = "5.0"
name = "acme-platform"
description = "Billing platform: Go services and a React front end"
```

## Presets

`presets` names the tools to generate for: built-in preset names, custom presets as inline tables, or both.

```toml
presets = [
  "claude",
  "cursor",
  "gemini",
  "codex",
  { name = "team-guide", type = "markdown", path = "docs/AI_GUIDE.md" },
]
```

`presets` is optional. A configuration without presets (and without a `[plugin]` block) generates nothing; see [Lint-only configuration](/ai-rulez/config-without-folder/#lint-only-configuration).

### Built-in presets

There are 52 built-in presets. [Supported harnesses](/ai-rulez/harnesses/) lists the files and features each one writes:

`aiassistant`, `amp`, `antigravity`, `augment`, `baz`, `bob`, `claude`, `cline`, `codebuddy`, `codebuff`, `codewhale`, `codex`, `commandcode`, `copilot`, `copilot-cli`, `cortex`, `crush`, `cursor`, `deepagents`, `devin`, `dsh`, `factory`, `gemini`, `gitlab-duo`, `goose`, `grok`, `hermes`, `junie`, `kilo`, `kimi`, `kiro`, `letta`, `mimocode`, `muse`, `omp`, `openclaw`, `opencode`, `pi`, `poolside`, `qoder`, `qwen`, `reasonix`, `replit`, `rovodev`, `takt`, `trae`, `vibe`, `warp`, `xum`, `zcode`, `zed`, `zoocode`.

Three more presets write no tool configuration:

| Preset | Writes |
| --- | --- |
| `okf` | An [Open Knowledge Format](/ai-rulez/okf/) bundle of the content; see [`[okf]`](#okf). |
| `llms-txt` | An [`llms.txt`](/ai-rulez/llms-txt/) index; see [`[llms_txt]`](#llms_txt). |
| `mcp` | The generic `.mcp.json`. It runs automatically whenever MCP servers are configured, so you do not name it. |

`windsurf` and `continue-dev` are not presets: `validate` and `generate` fail on them (`unknown built-in preset: "windsurf" (renamed to devin)`), and `ai-rulez doctor` reports them. Use `devin` for Windsurf. See [Migrating to v5](/ai-rulez/migration-v5/).

### Custom template presets

A preset table with `name`, `type` (`markdown`, `directory` or `json`), `path` and an optional Go `template` writes your rules and context to a file or folder of your choice:

```toml
[[presets]]
name = "team-guide"
type = "markdown"
path = "docs/AI_GUIDE.md"
```

The keys, preset types, template data and allowed output paths are documented in [Custom presets](/ai-rulez/profiles/). For a tool that needs skills, agents, MCP or settings files, use a provider spec (next section).

### Provider-backed presets (full parity)

A preset can point at a declarative provider spec instead of a template. A provider spec has the same reach as a built-in preset (root instruction file, skills, agents, commands, rule files, frontmatter, effort and model, MCP and hooks sidecars) and is validated against [`schema/provider.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/provider.schema.json).

```toml
[[presets]]
name = "my-tool"
provider = ".ai-rulez/providers/my-tool.toml"
```

The path is relative to the project root and must stay inside it; the spec's `name` must equal the preset's `name`. A spec:

```toml
name = "my-tool"
directories = [".my-tool"]

[root]
file = "MY_TOOL.md"
sections = ["title", "rules_inline", "context_inline"]

[outputs.skills]
mode = "per_item_file"
dir = ".my-tool/skills"
filename = "{id}/SKILL.md"

[[sidecars]]
kind = "mcp_json"
path = ".my-tool/mcp.json"
emit_when = "has_mcp_servers"
```

`emit_when` takes `always`, `has_mcp_servers`, `has_plugins`, `has_resolved_effort`, `has_mcp_json_entries`, `has_mcp_servers_or_plugin_settings`, `has_resolved_effort_or_mcp_servers`, `has_claude_settings`, `has_permissions` or `has_hooks`; it defaults to `has_mcp_servers` for an `mcp` sidecar, `has_hooks` for `hooks`, `has_permissions` for `permissions` and `always` otherwise. A provider without a `split` rules output (below) inlines every rule into its root file, and custom providers get no [machine-local](/ai-rulez/local-overrides/) output.

#### Output path rules

Every output path a configuration names (a custom preset `path`, a provider spec's `root.file`, `outputs.*.dir`, `sidecars[].path` and `directories`, `okf.dir`, `marketplace.output_dir`, `codex_skills_dir`) follows the rules in [Output paths](/ai-rulez/profiles/#output-paths).

#### Tool and model names in a provider spec

These keys under `[outputs.<kind>.frontmatter]` translate the canonical (Claude Code) tool and model names into the tool's own:

| Key | Description |
| --- | --- |
| `tool_names` | Table of canonical tool name to the tool's name (matched ignoring case). A tool the table does not name is dropped; an agent left with no tools writes no `tools` key. |
| `tool_case` | `"lower"` lower-cases every tool name after `tool_names`. |
| `model_aliases` | Table of resolved model (matched ignoring case) to the tool's model id; `""` drops the model. |
| `drop_bare_aliases` | Drops `sonnet`, `opus`, `haiku` and `inherit` after `model_aliases`, so the agent inherits the session model. |

`[outputs.<kind>.body]` takes `replace` (placeholder to replacement in the item body, such as `{ "$ARGUMENTS" = "$prompt" }`) and `replace_flag` (a frontmatter key set to `true` when a replacement happened).

An `mcp` sidecar takes `transports` (`["stdio"]` leaves remote servers out), `env_ref_syntax` (`"dollar"`, `"env_prefix"`, `"braced"` or `"opencode_env"`: a value from a `${VAR}` placeholder is written as `$VAR`, `${env:VAR}`, `${VAR}` or `{env:VAR}` instead of the resolved secret) and `elements.project_only`. A `hooks` sidecar renders the top-level `[[hooks]]` in the format of the harness its `dialect` names (`claude`, `qwen`, `factory`, `kiro`, `vibe`, ...; see [Hooks and settings](/ai-rulez/settings/)):

```toml
[[sidecars]]
kind = "hooks"
dialect = "qwen"
path = ".qwen/settings.json"
global_path = ".qwen/settings.json"
```

Sidecars that name the same `path` are merged into one document.

#### Split rule files in a provider spec

A `split` rules output makes a provider follow [`[rules] mode`](#rules) like the built-in presets:

```toml
[root]
file = "MY_TOOL.md"
sections = ["title", "rules_inline", "context_inline"]

[outputs.rules]
mode = "per_item_file"
dir = ".my-tool/rules"
filename = "{id}.md"
split = true
dialect = "claude"          # claude, cursor, trigger, copilot, cline, continue or junie
inline_filter = "path_scoped"
```

| Key | Description |
| --- | --- |
| `split` | `true`: split mode writes every rule (and path-scoped context item) to a file; inline mode writes only the items `inline_filter` selects. Requires `rules_inline` in `root.sections`; cannot be combined with `filter`. |
| `dialect` | Required with `split`. The frontmatter vocabulary of the rule files; cannot be combined with `body` or `frontmatter`. |
| `inline_filter` | `path_scoped`: the items that still get a file in inline mode. Omit to keep everything inline. |
| `dir` | Required with `split`; a relative path inside the project. |
| `filename` | Flat `{id}<ext>` template without `/`. |

The folder behaves like a built-in rules folder: hand-written files in it are never overwritten, and with `gitignore = true` generated files are ignored one by one.

## Profiles and scopes

### `default` and `profiles`

A profile is a named list of domains. `generate --profile <name>` (or `default`) renders the root content plus the domains the profile selects.

```toml
default = "full"

[profiles]
full = ["backend", "frontend", "qa"]
backend = ["backend", "qa", "builtin:docker"]
all-but-legacy = ["*", "!legacy"]
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `default` | string | `"default"` | Profile used without `--profile`. A comma-separated value (`"base,backend"`) composes several profiles. |
| `profiles` | table of string arrays | none | Profile name to domain entries. |

A profile entry is one of:

| Entry | Selects |
| --- | --- |
| `backend` | A local or included domain. |
| `builtin:docker` | A [builtin](#builtins) pack, for this profile only. |
| `builtin:docker/container-standards` | One item of a builtin pack, for this profile only. |
| `*` | Every domain the configuration knows. |
| `!legacy` | Removes a domain from the selection. Exclusions apply after the positive entries, whatever their order. |

Root content, globally enabled builtins and domains that come from includes are always selected. A profile made only of exclusions is an error (write `["*", "!legacy"]`); an unknown domain name is a warning. Profile names cannot contain a comma, and a profile lists domains only, never other profiles.

Without a `default` key the built-in `default` profile applies: a profile you define as `default`; otherwise every domain when no profiles are defined; otherwise only root content, global builtins and included domains. See [Domains and profiles](/ai-rulez/domains/) and [Profiles vs roles](/ai-rulez/roles/#profiles-vs-roles).

### `[[scopes]]`

A scope generates extra instruction files in a subdirectory, holding only that subdirectory's content. Tools load a subdirectory `AGENTS.md` or `CLAUDE.md` on top of the root one, so the root content is not repeated.

```toml
[profiles]
web = ["frontend"]

[[scopes]]
path = "packages/web"
profile = "web"
presets = ["codex", "claude"]
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `path` | string | required | Subdirectory, relative to the project root, without glob characters (`* ? [ ] { } , !`). |
| `profile` | string | the active profile | Profile whose domains the scope renders. |
| `presets` | array | `["claude", "codex"]` | Presets written in the scope. |
| `name` | string | none | Display name. |

A domain the root output already renders (a builtin, an include, or one the root profile selects) is not repeated in a scope, and a scope left with no content writes nothing. Rule files of a scope go to the root rules folders with the scope path as a glob prefix; see [Monorepo](/ai-rulez/monorepo/).

### `[[roles]]` and `[role_manifest]`

A role maps a job to a slice of the content: domains, include and exclude selectors per kind, inheritance, and a per-skill `skill_mode`. Select one with `generate --role <name>`; a comma-separated list composes roles. See [Roles](/ai-rulez/roles/).

```toml
[role_manifest]
enabled = true                  # write <config dir>/roles.json

[[roles]]
name = "backend"
domains = ["backend"]
[roles.skills]
exclude = ["deploy-*"]
[roles.skill_mode]
migrate = "name-only"
```

`[[roles]]` keys: `name`, `description`, `extends`, `domains`, `skills`, `rules`, `agents`, `commands`, `checks`, `pin`, `skill_mode`, `delivery`, `match`. `[role_manifest]` keys: `enabled`, `skill_mode_fallback` (`drop` (default) or `serve`), which says what `generate --role` does with a skill whose `skill_mode` is `off` on a harness with no per-skill setting.

## Output files

### `source`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `source` | `ai-rulez`, `agents` | `ai-rulez` | Where content is read from. `agents` reads the root `AGENTS.md` and `.agents/` (Agent Skills in `.agents/skills/<name>/SKILL.md`, plus `.agents/rules`, `agents`, `commands`, `context`, `checks` and `domains/<name>`) and never writes to them. |
| `agents_md_split` | boolean | `false` | With `source = "agents"`, read `AGENTS.md` as one context item per `##` heading (`agents-<heading slug>`) instead of one item named `agents`. |

A configuration in `.agents/ai-rulez.toml` implies `source = "agents"`. The agents source forces per-tool files and rejects `agents_md = true`. See [`.agents` as a source](/ai-rulez/agents-source/).

```toml
source = "agents"
agents_md_split = true
```

### `agents_md`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `agents_md` | boolean | `true` | Render shared files once: always-on rules and context into one `AGENTS.md` (and `<scope>/AGENTS.md` for each scope), skills into `.agents/skills/<name>/SKILL.md`. |

With `agents_md` on, the presets that read these files (for example `codex`, `opencode`, `claude`, `gemini`, `cursor`, `copilot`; [Supported harnesses](/ai-rulez/harnesses/#instructions-rules-and-skills) has the full list) stop writing their own root file and skills directory:

- `claude` writes a `CLAUDE.md` that imports `@AGENTS.md` (or nothing; see [`[claude] claude_md`](#claude)).
- `gemini` gets `AGENTS.md` added to `context.fileName` in `.gemini/settings.json`; `GEMINI.md`, `.hermes.md` and `.github/copilot-instructions.md` are not written.
- Rules folders (`.cursor/rules`, `.claude/rules`, ...) keep only the items that are not always-on.
- Skills with `targets`, agents, commands, MCP files and machine-local files stay per preset.

`agents_md = false` writes each preset's own files and removes the shared ones no preset still writes. Tool support and trade-offs are in [AGENTS.md and .agents/skills](/ai-rulez/agents-md/).

### `skip_outputs`

Output kinds ai-rulez never writes, overwrites or deletes, so you can hand-write them and let ai-rulez write the rest (installed skills, plugins, MCP servers, hooks, permissions). Default: empty.

```toml
skip_outputs = ["instructions"]
```

| Value | Skips |
| --- | --- |
| `instructions` | Every root instruction file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md` and the other presets' root files, their `.local.md` and `AGENTS.override.md` variants, scope roots). Overrides `agents_md` and `claude_md`. |
| `agents_md` | The shared `AGENTS.md` (and `AGENTS.override.md`) and the shared `.agents/skills` tree. |
| `claude_md` | `CLAUDE.md`, at the root and in scopes. |
| `rules` | Rule files in every preset. |
| `skills` | Skill directories in every preset, installed skills included. A command written as a skill counts as a command. |
| `agents` | Agent files. |
| `commands` | Command and prompt files, including the skills that carry a command. |

Skipped files are left out of the manifest, the managed `.gitignore` block, `generate --check`, the lock's output digests and `clean`. A file an earlier run generated and that is now skipped stays where it is and is reported once as no longer managed. `config.local.toml` accepts the key too. See [Managing only remote skills and plugins](/ai-rulez/managing-remote-skills/).

### `[claude]`

Claude Code options.

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `claude_md` | `shim`, `none` | `shim` | With `agents_md` on: `shim` writes a `CLAUDE.md` that imports `@AGENTS.md`; `none` writes no `CLAUDE.md` (root or scopes) and removes one a previous run generated. |
| `skills.hide_from_menu` | boolean | `false` | Write `user-invocable: false` on every skill that does not set it, so the skill is hidden from the `/` menu. A skill's own `user-invocable` wins. See [Skill frontmatter](/ai-rulez/skills/#invocation-keys). |
| `settings` | table | none | Opt-in ownership of plugin keys in `.claude/settings.json`: `manage`, `register_marketplace`, `marketplace_source`, `auto_update`, `enable_plugins`, `disable_plugins`. See [Plugins](/ai-rulez/plugins/). |
| `settings.managed` | table | none | `env` and `skill_overrides` entries of `.claude/settings.json`, owned entry by entry. See [Hooks and settings](/ai-rulez/settings/). |

```toml
[claude]
claude_md = "none"

[claude.skills]
hide_from_menu = true
```

Claude Code reads `AGENTS.md` directly from version 2.1.277, so `claude_md = "none"` makes `AGENTS.md` the only instruction file. Claude Code reads `AGENTS.md` only when no `CLAUDE.md`, `.claude/CLAUDE.md` or `CLAUDE.local.md` exists in the working directory or an ancestor: a hand-written `CLAUDE.md`, or the `CLAUDE.local.md` ai-rulez writes for [machine-local content](/ai-rulez/local-overrides/), shadows it. A hand-written `CLAUDE.md` (one without ai-rulez's `Content-Hash` header) is never removed.

### `[codex]` and `codex_skills_dir`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `codex.project_doc_max_bytes` | integer | `32768` | The limit your Codex uses. `generate` warns when the `AGENTS.md` files Codex concatenates exceed it, naming the largest sections; `0` turns the warning off. See [Rules: size limits](/ai-rulez/rules/#size-limits). |
| `codex_skills_dir` | string | `.agents/skills` | Where the `codex` preset writes skills when `agents_md` is off. Set `.codex/skills` to write there instead. |

```toml
codex_skills_dir = ".codex/skills"

[codex]
project_doc_max_bytes = 65536
```

When the directory changes, `generate` writes the new tree and removes the files it wrote to the old one; hand-written skills stay. With `agents_md` on, skills always go to `.agents/skills`.

### `compact`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `compact` | boolean | `false` | Omit the per-rule `**Priority:**` lines from inline rule sections. |

### `bundle_exclude`

Extra patterns for files left out of a skill's or command's bundled resources: they are not copied and not listed in `## Resources`. The bundle always skips `.git`, `.venv*`, `venv`, `__pycache__`, `*.pyc` and `node_modules`, and, in a git work tree, everything `.gitignore` ignores.

```toml
bundle_exclude = ["*.log", "scripts/build/", "assets/raw/*.psd"]
```

A pattern without `/` matches any path segment; one with `/` matches the path relative to the skill directory, or a directory on it. Patterns use glob syntax (`*`, `?`, `[a-z]`). Installed and included skills get the built-in list and `.gitignore` only.

### `recursive_exclude`

`generate --recursive` and `validate --recursive` search every directory below the working directory for configurations, except build output, dependency and hidden directories and `testdata`. List further directories (test fixtures, golden corpora) in the configuration of the directory the run starts from:

```toml
recursive_exclude = ["tests/fixtures", "svc/*/gen"]
```

Entries are directories or globs relative to the project root. `generate --recursive --exclude <dir>` adds to the list for one run.

## Git ignore

### `gitignore`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `gitignore` | boolean | `false` | Maintain a managed block in `.gitignore` that lists the generated outputs. `generate --gitignore` does the same for one run. |

By default generated files are committed. Set `gitignore = true` when the team keeps them out of git and regenerates locally:

```toml
gitignore = true
```

The block lists generated files and the subdirectories ai-rulez owns (`AGENTS.md`, `CLAUDE.md`, `.mcp.json`, `.claude/skills/`, `.claude/agents/`, `.codex/`, `.github/skills/`, ...), never a whole tool directory, so a hand-written `.opencode/settings.json` stays tracked. ai-rulez edits only the lines between `# BEGIN ai-rulez` and `# END ai-rulez`; everything outside the block is left byte for byte.

Before writing, ai-rulez asks git (`git check-ignore`) about each entry and leaves out what your own rules already ignore. A negation of yours (`!CLAUDE.md`, `!.claude/skills/`) is a deliberate override: that entry is left out of the block and never re-ignored. Outside a git repository every entry is written.

Machine-local files are ignored whatever this key says:

- With `gitignore = true`, the block always lists `.ai-rulez/config.local.*` (and its lock and temp files), `.ai-rulez/local/` and `.ai-rulez/.generated-manifest.local.json`, so the block is the same on every machine.
- With `gitignore = false`, the machine-local inputs that exist are still added to `.gitignore`.
- Outputs that exist only because of your local inputs (`CLAUDE.local.md`, `AGENTS.override.md`, `<rulesdir>/*.local.*`, ...) go to `.git/info/exclude`, which is not committed.

`generate` refuses to write a machine-local or secret-bearing output that would end up unignored. See [Local configuration](/ai-rulez/local-overrides/).

### `gitignore_exclude`

Generated outputs you commit on purpose, kept out of the managed block while every other entry stays managed. Use it instead of a hand-written `!pattern`.

```toml
gitignore = true
gitignore_exclude = ["AGENTS.md", "**/CLAUDE.md", ".claude/skills/", ".opencode/skills/"]
```

Entries use gitignore notation, matched against the patterns the block would list: an entry without `/` matches at any depth, one with `/` is anchored at the root, `**` crosses directories (`**/AGENTS.md` also reaches a scope's `svc/AGENTS.md`), and a trailing `/` names a directory and what is below it. An empty entry, a comment or a `!negation` fails validation. The key has no effect unless the block is written. Machine-local and secret outputs are never excluded; an entry that only reaches them is ignored with a warning. In a recursive run each root applies its own list.

`generate` warns when the block and your files disagree:

- A tracked file matches a line of the block: git keeps tracking it, but new files next to it (a new skill under `.claude/skills/`) are ignored. The warning names the files and the fix (`gitignore_exclude` or `git rm --cached`).
- A `!pattern` of yours un-ignores a line the block lists. The warning names the negation's `file:line`; a `gitignore_exclude` entry for the path silences it.

## `[header]`

Every generated file starts with a banner that says it is generated and carries a `Content-Hash` freshness line. `[header]` controls the banner.

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `style` | `minimal`, `compact`, `detailed` | `minimal` | Amount of prose: about 10, 20 or 50 lines. |
| `text` | string | none | Your own banner text, written verbatim inside the output's comment syntax. Replaces `style`. |
| `timestamp` | boolean | `false` | Write a `Generated:` line. |
| `hashes` | `content`, `full`, `none` | `content` | Freshness lines: `Content-Hash` only, plus the project-wide `Source-Hash`, or none. |
| `mcp_line` | boolean | `true` | Name the ai-rulez MCP server in the banner. |
| `nested` | `compact`, `omit`, `inherit` | `compact` | Banner of a root inside another ai-rulez root of the same repository. |

The default `minimal` banner of `AGENTS.md`:

```html
<!--
🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT
Project: acme
Source: .ai-rulez/config.toml

NEVER edit this file - modify .ai-rulez/ content instead
Use MCP server: npx -y ai-rulez@latest mcp
Regenerate: ai-rulez generate

Docs: https://github.com/Goldziher/ai-rulez
Content-Hash: blake3:8d214e1bfea92a79aca55fbdbadb174da22cedbec8106e36fa020e1881a89771
-->
```

`style = "compact"` adds the target file, item counts, the directory layout and a short checklist for agents; `detailed` explains the folder layout and the edit-and-regenerate workflow in full.

- **`hashes`.** Keep the default `content` when generated files are committed: `full` adds a `Source-Hash` of the whole source set, so one edit rewrites every generated file.
- **`timestamp`.** Pin the time with [`SOURCE_DATE_EPOCH`](https://reproducible-builds.org/docs/source-date-epoch/) to keep output reproducible: `SOURCE_DATE_EPOCH=$(git log -1 --pretty=%ct) ai-rulez generate`.
- **`mcp_line`.** The command follows [`[mcp]`](#mcp); the line is left out while `self_server = false`.
- **`text`.** The hash lines are still appended; `timestamp` does not apply.
- **`nested`.** `compact` writes one line, `Generated by ai-rulez from <path to its config>; do not edit`, plus the hash lines. A custom `text` is kept for nested roots unless `nested` is set.

```toml
[header]
text = """
Built by the platform team.
Set up with: mise run setup
Regenerate with: ai-rulez generate
"""
```

## `[rules]`

How rules are written. See [Rules and native rules folders](/ai-rulez/rules/) for what each preset writes.

```toml
[rules]
mode = "split"

[rules.mode_by_preset]
claude = "inline"
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `mode` | `split`, `inline` | `split` | `split` writes one file per rule in the tool's rules folder; `inline` embeds rules in the root file and moves only path-scoped rules to the folder. |
| `mode_by_preset` | table | none | Preset name (built-in, custom or provider) to `split` or `inline`; beats `mode`. |
| `scoped_fallback` | `inline`, `skill` | `inline` | How the shared `AGENTS.md` serves glob-scoped rules and context to presets without a rules folder: inlined with an `_Applies to:_` line, or written to `.agents/skills` as skills gated by their paths. See [Glob-scoped rules and AGENTS.md](/ai-rulez/rules/#glob-scoped-rules-and-agentsmd). |
| `scoped_fallback_group` | `none`, `include`, `directory` | `none` | With `scoped_fallback = "skill"`: one skill per item, per include or builtin pack, or per directory. |
| `baz_scoped` | `nested`, `root` | `nested` | `baz` preset: write path-scoped items to the `AGENTS.md` of the directory their globs point into, or keep them in the root `AGENTS.md`. |
| `baz_default` | `all`, `none` | `all` | `baz` preset: nest every path-scoped item, or only those selected by `targets`, `baz: true` or `baz.include`. |
| `baz.include` | array | none | Rule and context names nested for `baz` when `baz_default = "none"`. |
| `baz_rest` | `root`, `skip` | `root` | `baz` preset: what happens to a glob-activated item it does not nest. |
| `baz_unscoped` | `root`, `skip` | `root` | `baz` preset: what happens to a glob-activated item that names no directory (`**/*.go`). |
| `baz_dedupe` | boolean | `false` | `baz` preset: write an item that targets several sibling directories once, to their parent. |
| `baz_dedupe_max` | integer | `0` | `baz` preset: above `0`, an item copied into more directories than this is written once to the directory covering them all. |
| `baz_budget` | integer | `0` | `baz` preset: byte budget of its `AGENTS.md` files; `generate` warns above it. |

The `baz_*` keys are explained in [Baz](/ai-rulez/baz/).

## `[defaults]`

Agent defaults applied when an agent's frontmatter does not set them.

```toml
[defaults]
effort = "medium"
omit_agent_fields = ["tools"]

[defaults.effort_by_preset]
codex = "high"
claude = "xhigh"

[defaults.model_by_preset]
claude = "opus"
copilot = "gpt-5"
opencode = "anthropic/claude-sonnet-4-5"
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `effort` | `low`, `medium`, `high`, `xhigh`, `max`, `inherit` | none | Reasoning effort for every preset that supports it. |
| `effort_by_preset` | table | none | Preset name to effort; beats `effort`. |
| `model_by_preset` | table | none | Preset name to the agent `model` value. Model names are tool-specific, so there is no global model key. |
| `omit_agent_fields` | array of `model`, `effort`, `tools`, `description` | none | Agent frontmatter fields never written, so an agent stays loadable where a value would be rejected. |

Effort resolves per preset and agent: the agent's `effort` frontmatter (for presets with per-agent effort), then `effort_by_preset.<preset>`, then `effort`; otherwise nothing is written. Each preset maps the value to its own vocabulary; `inherit` is never written:

| Preset | Written to | Field | `xhigh` | `max` |
| --- | --- | --- | --- | --- |
| `claude` | `.claude/agents/<id>.md` | `effort` | `xhigh` | `max` |
| `codex` | `.codex/agents/<id>.toml` and `.codex/config.toml` | `model_reasoning_effort` | `xhigh` | `high` |
| `devin` | `.devin/agents/<id>.md` | `reasoning_effort` | `xhigh` | `high` |
| `pi` | `.pi/agents/<id>.md` | `thinking` | `xhigh` | `max` |
| `opencode` | `.opencode/agents/<id>.md` | `variant` | `high` | `high` |
| `xum` | `.xum/agents/<id>.md` | `ai.thinkingLevel` | `high` | `high` |
| `factory` | `.factory/droids/<id>.md` | `reasoningEffort` | `high` | `high` |
| `amp` | `.amp/settings.json` (global only) | `amp.anthropic.effort` | `high` | `max` |
| `reasonix` | `.reasonix/skills/<id>/SKILL.md` | `effort` | `high` | `max` |

`cursor`, `copilot`, `gemini`, `junie`, `antigravity` and `cline` take effort from their own settings; ai-rulez writes none.

The model resolves as: the agent's `<preset>_model` frontmatter (`claude_model`, `copilot_model`, ...), then `model_by_preset.<preset>`, then the agent's `model`; otherwise nothing is written. Most presets that write agent files write a model. Where it lands for the main ones:

| Preset | Agent file | Field |
| --- | --- | --- |
| `claude` | `.claude/agents/<id>.md` | `model` |
| `copilot` | `.github/agents/<id>.agent.md` | `model` |
| `cursor` | `.cursor/agents/<id>.md` | `model` |
| `cline` | `.cline/agents/<id>.yaml` | `modelId` |
| `junie` | `.junie/agents/<id>.md` | `model` |
| `opencode` | `.opencode/agents/<id>.md` | `model` |
| `devin` | `.devin/agents/<id>.md` | `model` |
| `gemini` | `.gemini/agents/<id>.md` | `model` |
| `xum` | `.xum/agents/<id>.md` | `ai.model` |
| `pi` | `.pi/agents/<id>.md` | `model` |
| `factory` | `.factory/droids/<id>.md` | `model` |

OpenCode needs a `provider/model` id (`anthropic/claude-sonnet-4-5`). When the resolved model is not provider-qualified, ai-rulez warns and omits it, and the agent uses the session model; set `opencode_model` or `model_by_preset.opencode` to keep `model: sonnet` for other tools. Likewise `gemini` omits a bare Claude alias (`sonnet`, `opus`, `haiku`) with a warning.

## `builtins`

Built-in domains embedded in the binary: rules, skills, agents, context and commands you enable without an include. With the key absent no builtin content is loaded.

| Value | Loads |
| --- | --- |
| absent | Nothing. |
| `true` | Every builtin pack. |
| `false` | Nothing, auto-included packs included. |
| `["rust", "docker"]` | The auto-included packs plus the named packs. |
| `["!ai-governance"]` | The auto-included packs minus `ai-governance`. |
| `["!git-workflow/commit-messages"]` | Removes one item (rule, skill, agent, command or context file, by name). |
| `["docker/container-standards"]` | Only that item of the `docker` pack. |

```toml
builtins = ["rust", "python", "!ai-governance", "!git-workflow/commit-messages"]
```

How entries combine:

- `domain` wins over `domain/name`: the whole pack loads. An auto-included pack loads whole unless excluded with `!domain`.
- `domain/name` still applies after `!domain`, in either order: `["security/secrets-handling", "!security"]` loads only `secrets-handling`.
- `!domain/name` always removes the item.
- A positive entry that names no pack or item fails `validate` with the valid choices; an unknown exclusion is a warning.
- Item selection needs the array form.

`ai-rulez builtins list` lists the packs and `ai-rulez builtins show <domain>` the item names. A profile can enable a pack for itself with [`builtin:<name>`](#default-and-profiles), even when `builtins` is absent or `false`; a pack the root `builtins` enables stays global.

Universal packs:

| Pack | Auto | Content |
| --- | --- | --- |
| `ai-governance` | yes | Rules: read before write, minimal changes, verification before completion, systematic debugging, agent workflow, generated files, communication style, no AI signatures, explain reasoning. Agents: `code-reviewer`, `docs-writer`. Skills: `config-placement`, `skill-authoring`. |
| `agent-delegation` | yes | The `## Agents` roster in root instruction files. |
| `security` | yes | Rules: input validation, secrets handling, least privilege, customer data boundary. Skills: `owasp-quick-reference`, `dependency-awareness`. Agent: `security-auditor`. |
| `git-workflow` | yes | Rules: conventional commits, atomic commits, branch hygiene, safe operations. |
| `code-quality` | yes | Skills: `code-quality-standards`, `error-handling`. |
| `testing` | yes | Rule: tests ship with the change. Skills: `tdd-workflow`, `testing-conventions`. Agent: `test-writer`. |
| `token-efficiency` | yes | Rules: context preservation, output awareness. Skills: `task-runner`, `incremental-approach`. |
| `cicd` | no | Rules: pipeline standards, GitHub workflow conventions. Agents: `devops-engineer`, `release-engineer`. |
| `docker` | no | Skill: `container-standards`. |
| `observability` | no | Skill: `observability-standards`. |
| `documentation` | no | Rules: inline docs, README standards, docs with code, ADR immutability. Skill: `adr-workflow`. |
| `polyglot-bindings` | no | Skills: `bindings`, `ffi-and-language-interop`, `rust-polyglot-conventions`. Agents: `ffi-engineer`, `polyglot-architect`. |
| `default-commands` | no | Commands: `/iterate`, `/parallelize`. |

Language packs (`rust`, `python`, `typescript`, `go`, `java`, `ruby`, `php`, `elixir`, `csharp`, `r`) and binding packs (`pyo3`, `napi-rs`, `magnus`, `ext-php-rs`, `rustler`, `wasm`, `jni-rs`, `extendr`, `cgo`, `vite-plus`) are skills: toolchain, linting, type checking, security scanning, testing and anti-patterns for that language or binding. Skills load on demand, so they cost nothing in the always-loaded files; only behavioral rules are written inline.

### Drop the agents roster from root files

The auto-included `agent-delegation` pack adds an `## Agents` section, listing each agent's name and description, to every root instruction file. Exclude it to drop the section:

```toml
builtins = ["!agent-delegation"]
```

The agent files themselves (`.claude/agents/*.md`, ...) are still generated. `ai-rulez tokens` reports the roster's always-loaded size as the `agents_delegation` line.

### Precedence over builtins

Builtins rank below every project item: a project rule, skill, agent or command with the name of a builtin item replaces it (see [Deduplication by name](#deduplication-by-name)). Skills and commands share one output namespace, so a project skill also replaces a builtin command of the same id, and the other way round. For skills and commands `generate` logs `Duplicate skill collapsed` (or `command`) and `validate` reports `AR703`; list the name in `[lint] allow_overrides` to silence both. When a hand-written skill (such as `.claude/skills/testing-conventions/`) sits where a builtin would be written, `generate` stops and names the file instead of overwriting it.

A local domain with the name of a builtin pack replaces the whole pack: none of the pack's items are loaded, and `generate` and `validate` warn `Builtin pack skipped`.

## MCP servers

### `[mcp]`

How ai-rulez writes its own MCP server entry, and the defaults of `ai-rulez mcp`.

```toml
[mcp]
self_server = true
self_server_launcher = "binary-then-npx"
serve_skills = true
discover_skills = "repo"
exclude = ["third_party/**"]
```

By default `generate` adds this entry to `.mcp.json` and to the project MCP file of the configured presets that have one, such as `.cursor/mcp.json`, `.vscode/mcp.json` (`copilot`), `.codex/config.toml`, `opencode.json`, `.devin/mcp_config.json`, `.agents/mcp_config.json` (`antigravity`) and `.xum/mcp.jsonc`. Presets whose MCP file is written only for declared servers (`gemini`, `qwen`, `zed`, `vibe`, ...) get the entry only when `[[mcp_servers]]` declares some:

```json
{
  "mcpServers": {
    "ai-rulez": { "type": "stdio", "command": "npx", "args": ["-y", "ai-rulez@latest", "mcp"] }
  }
}
```

When the project serves skills (a `delivery` other than `static`, a `[[skill_sources]]` entry or a role delivery), the entry runs `mcp --serve-skills`, so one server serves the skills and the authoring tools.

Generation keys:

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `self_server` | boolean | `true` | Write the `ai-rulez` entry. `generate --no-self-mcp` turns it off for one run. |
| `self_server_version` | string | `latest` | Version or dist-tag the npx launch pins. Exclusive with `self_server_command`. |
| `self_server_launcher` | `npx`, `binary-then-npx` | `npx` | `binary-then-npx` runs an installed `ai-rulez` from `PATH` and falls back to npx, through a `sh -c` script. Exclusive with `self_server_command`. |
| `self_server_command` | array | none | Executable and arguments replacing the npx launch, such as `["ai-rulez", "mcp"]`. `--serve-skills` is appended when skills are served, except to a `sh -c` command, where you put it in the script. |
| `skills_instructions` | boolean | `true` | While skills are served, add an "Unlisted skills" section to instruction files telling the agent to call `find_skill`, then `load_skill`. |
| `skills_instructions_text` | string | none | Replaces that section. `{{server}}` is the serving MCP server, `{{skills_dir}}` the skills folder relative to the instruction file. |

The other `[mcp]` keys are defaults of `ai-rulez mcp`, read when the server starts, with the matching flag winning: `serve_skills`, `discover_skills`, `exclude`, `index_cache`, `read_only`, `tools`, `authoring`, `allow`, `deny`, `serve_user_invoked_skills`, `daemon`, `daemon_idle_timeout`, `no_watch`. They are documented in [Settings in the configuration](/ai-rulez/mcp-server/#settings-in-the-configuration).

`self_server_version`, `self_server_launcher` and `self_server_command` need `self_server = true`. The self entry is merged into existing files: other servers and keys survive, and a server you declare named `ai-rulez` wins. Each root decides on its own, also under `generate --recursive`. MCP output is project-only; the user scope never carries the entry. Turning `self_server` off removes the entry on the next `generate` (and `.mcp.json` with it, when nothing else is left). On a clone whose local manifest never recorded the entry, it can survive; remove it by hand.

### `[[mcp_servers]]`

MCP servers written into the MCP file of every configured preset.

```toml
[[mcp_servers]]
name = "grafana"
command = "uvx"
args = ["mcp-grafana"]
env = { GRAFANA_URL = "http://localhost:3000", GRAFANA_SERVICE_ACCOUNT_TOKEN = "${GRAFANA_SERVICE_ACCOUNT_TOKEN}" }

[[mcp_servers]]
name = "remote-api"
transport = "http"
url = "https://mcp.example.com/mcp"
headers = { Authorization = "Bearer ${REMOTE_API_TOKEN}" }
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `name` | string | required | Unique server name. |
| `description` | string | none | Description. |
| `command` | string | none | Executable of a `stdio` server. May contain `${PROJECT_ROOT}`. |
| `args` | array | none | Arguments of a `stdio` server. May contain `${PROJECT_ROOT}`. |
| `env` | table | none | Environment variables. Values may contain `${VAR}` placeholders. |
| `transport` | `stdio`, `http`, `sse` | `stdio` | Transport. |
| `type` | `stdio`, `http`, `sse` | `transport` | Alias of `transport`, spelled as harness files spell it; both must agree when set. |
| `url` | string | none | `http(s)://` URL of an `http` or `sse` server. |
| `headers` | table | none | HTTP headers of a remote server. Values may contain `${VAR}`. Rejected on `stdio`. |
| `oauth` | table | none | Pre-registered OAuth client of a remote server: `client_id`, `callback_port` (1 to 65535), `scopes`. |
| `enabled` | boolean | `true` | `false` leaves the server out. |
| `profiles` | array | all | Profiles the server is written for. |
| `domain` | string | none | Write the server only when the active profile or role selects this domain. With `profiles`, both must match. |
| `package` | string | none | Package URL (`pkg:npm/%40scope/server@1.4.2`) that `ai-rulez sbom` reports; not written to tool files. |

`oauth` is written to Claude Code's `.mcp.json`, VS Code's `.vscode/mcp.json` and Codex's `.codex/config.toml`. A harness whose MCP format has no OAuth block leaves the server out with a warning. `validate` and `scan` report an `oauth` block without `client_id` as `AR602`, and also a server name that resolves to a different endpoint or transport in another file (a hand-written `.mcp.json`, say), since a harness reads only one of them.

```toml
[[mcp_servers]]
name = "slack"
type = "http"
url = "https://mcp.slack.com/mcp"
[mcp_servers.oauth]
client_id = "1234567890.1234567890"
callback_port = 8080
scopes = ["channels:read", "chat:write"]
```

Header names must be valid HTTP tokens, unique ignoring case, and values may not contain line breaks. Distributable plugin bundles (`generate --plugin`) carry no headers; `generate --plugin` warns for each affected server.

#### Placeholders and secrets

`${PROJECT_ROOT}` in `command`, `args` or `env` resolves to the project root (the directory holding `.ai-rulez/`). The result is a machine-specific absolute path, so keep that output gitignored or regenerate it per machine; in `env`, a real `PROJECT_ROOT` from `--env`, the environment or a dotenv file wins. For Claude Code, `${CLAUDE_PROJECT_DIR:-.}` in `command` or `args` is passed through unchanged and stays portable.

Other `${VAR}` placeholders (`VAR` matching `[A-Za-z_][A-Za-z0-9_]*`) in `env` and `headers` resolve at `generate` time from repeated `--env KEY=VALUE` flags, then the process environment, then dotenv files. `.env` in the generation directory is read by default; `--env-file PATH` (repeatable, later files win) replaces it. An unresolved placeholder fails generation.

A generated MCP file that holds a resolved secret (a value from a placeholder, or an env key or header name that looks sensitive: `TOKEN`, `SECRET`, `PASSWORD`, `KEY`, `CREDENTIAL`, `Authorization`, `Proxy-Authorization`, `Cookie`) must be gitignored; otherwise `generate` fails before writing. Such a file is written owner-only (`0600`). A file that only holds environment references needs no ignore entry.

#### Environment references

Where a tool expands environment references itself, a value that came from a `${VAR}` placeholder resolved from the process environment is written as that tool's reference instead of the secret. Values from `--env`, `--env-file`, `.env` or `${PROJECT_ROOT}` are always written resolved.

| Tool | Reference | Where |
| --- | --- | --- |
| Readers of the root `.mcp.json` (`claude`, `cursor`, `copilot`, `codebuddy`, `commandcode`, `reasonix`) | `${VAR}` | `.mcp.json` env and headers |
| `gemini` | `${VAR}` | `.gemini/settings.json` |
| `amp` | `${VAR}` | `.amp/settings.json` |
| `pi` | `${VAR}` | `.pi/mcp.json` |
| `factory` | `${VAR}` | `.factory/mcp.json` env and headers |
| `opencode`, `kilo` | `{env:VAR}` | `environment` and `headers` |
| `cursor`, `devin` | `${env:VAR}` | `.cursor/mcp.json`, `.devin/mcp_config.json` |
| `codex` | `env_vars`, `env_http_headers`, `bearer_token_env_var` | `.codex/config.toml` |
| `codebuff`, `crush` | `$VAR` (whole value only) | env and headers |

- `${VAR}`, `${env:VAR}` and `{env:VAR}` can sit inside a longer value (`Bearer ${TOKEN}`); `$VAR` is used only for a whole value.
- The root `.mcp.json` uses `${VAR}` only for upper-case names.
- Tools that document no expansion (`copilot` in `.vscode/mcp.json`, `qoder`, `junie`, `antigravity`, `zed`, `trae`, `cline`, `xum`, `kiro` without its opt-in) get the resolved value, written owner-only.
- When a preset that needs the resolved value and one that reads references share `.mcp.json`, `generate` fails naming both; pass the secret with `--env` or `.env` to use both.

### Settings document merge behavior

`.mcp.json`, `.claude/settings.json`, `.gemini/settings.json`, `opencode.json` and similar files are shared documents: ai-rulez owns the entries it writes (each MCP server, hook group, permission rule and managed key on its own) and you own the rest, which `generate` and `clean` keep byte for byte. See [Shared settings documents](/ai-rulez/settings/#shared-settings-documents) for the ownership rules.

## Hooks, permissions, guard and session_start

These keys write lifecycle hooks and permission rules into each harness's native settings files, outside any plugin. Everything is documented in [Hooks and settings](/ai-rulez/settings/) and [Permissions](/ai-rulez/permissions/); the [feature matrix](/ai-rulez/harnesses/) shows which harness supports what.

| Key | Type | Description |
| --- | --- | --- |
| `[[hooks]]` | array of tables | Lifecycle hooks with Claude Code event names, translated per harness. Keys: `event`, `matcher`, `matchers` (per harness), `targets`, `hooks` (handlers with `type` (only `command`), exactly one of `command` or `script`, `args`, `timeout`, `async`, `if`, `status_message`). |
| `[permissions]` | table | `allow`, `ask` and `deny` rules in Claude Code permission-rule syntax, translated for every harness with a permission file. |
| `[guard]` | table | `generated = true` adds a `PreToolUse` hook that blocks an agent's edit to a generated file. Keys: `generated` (default `false`), `version`, `command`. |
| `[session_start]` | table | A `SessionStart` hook for Claude Code that regenerates outputs in a remote session. On by default. Keys: `generate` (default `true`), `when` (`remote` or `always`), `mode` (`auto`, `locked`, `frozen`, `plain`), `timeout` (default 120 s), `version`, `command`, `prompt`, `prompt_text`. See [Run generate in remote sessions](/ai-rulez/session-start/). |

```toml
[[hooks]]
event = "PreToolUse"
matcher = "Bash"
[[hooks.hooks]]
script = "scripts/guard.sh"
timeout = 10

[permissions]
allow = ["Bash(git status)", "Bash(git diff *)"]
deny = ["Read(./.env)"]

[guard]
generated = true

[session_start]
when = "always"
```

`ai-rulez generate --user` renders a user-level configuration into the per-user directories each harness reads; see [User-level configuration](/ai-rulez/user-scope/).

## Remote content

Content fetched from other repositories or directories and merged into the project. Every remote source can be pinned in `ai-rulez.lock`, and every git source takes either a `ref` or an npm-style `version` constraint (`^1.2`, `~2.1.0`) with `tag_prefix`, `include_prerelease` and `min_release_age`.

### `[[includes]]`

Rules, context, skills, agents, commands and checks from another ai-rulez project. See [Includes](/ai-rulez/includes/).

```toml
[[includes]]
name = "platform-standards"
source = "https://github.com/acme/ai-standards"
version = "^2.0"
include = ["rules", "skills"]
install_to = "platform"
```

Keys: `name`, `source` (required), `path`, `ref`, `version`, `tag_prefix`, `include_prerelease`, `min_release_age`, `include` (`rules`, `context`, `skills`, `agents`, `commands`, `checks`; default all), `install_to` (a domain; default root), `local_override`, `format` (`okf`), `merge_strategy` (`local-override` (default), `include-override`, `error`).

### `[[installed_skills]]`

Skills installed from another repository into every preset's skill directory. See [Installed skills](/ai-rulez/installed-skills/).

```toml
[[installed_skills]]
name = "kreuzberg"
source = "kreuzberg-dev/kreuzberg"
include = ["xberg*"]
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `name` | string | required | Unique entry name. |
| `source` | string | required | Git URL, `owner/repo`, `github:owner/repo`, `file://` URL or a directory inside the project (`~/path` only from `config.local.toml` or the user config). |
| `path` | string | `skills/<name>`, or `skills` with `include`/`exclude` | Skill directory, or a directory of skill directories, inside the source. |
| `ref`, `version`, `tag_prefix`, `include_prerelease`, `min_release_age` | | | Version selection, as above. |
| `local_override` | string | none | Local path used instead of `source` when present. |
| `include`, `exclude` | array | none | Globs on skill directory names; `exclude` wins. |
| `name_prefix` | string | none | Prefix for every resolved skill name and output directory. |
| `domain` | string | none | Place the skills in a domain, so profiles and roles select them. |
| `profiles` | array | all | Profiles the skills are written for. |
| `resources` | array | whole directory | What is bundled besides `references/`, `scripts/` and `assets/`. |
| `trust` | `error`, `warn` | `error` | Security-scan level of the imported skills. |

`ai-rulez skill install`, `skill remove` and `skill list` edit the entries.

### `[[installed_marketplaces]]` and `[[installed_plugins]]`

Plugins imported from Claude Code, Codex, Cursor or Gemini marketplaces, or straight from a plugin repository: their skills, agents, commands, rules, MCP servers and (opt-in) hooks become ordinary content. See [Installed plugins](/ai-rulez/installed-plugins/).

```toml
[[installed_marketplaces]]
name = "acme"
source = "acme/plugins"
version = "^1.2"

[[installed_plugins]]
name = "formatter"
marketplace = "acme"
exclude = ["hooks"]
```

Marketplace keys: `name`, `source` (required), `path`, `ref`, `version`, `tag_prefix`, `include_prerelease`, `min_release_age`, `trust`. Plugin keys: `name` (required), `marketplace` and `plugin`, or `source`, `path`, `ref` and the version keys; `format` (`claude`, `codex`, `gemini`, `cursor`, `agent-plugins`); `include` and `exclude` (`skills`, `agents`, `commands`, `hooks`, `mcp`, `rules`); `name_prefix`, `domain`, `profiles`, `trust`; `enable_hooks` (default `false`); `enable_mcp` (default `true`).

## Served skills

Skills can be served on demand by `ai-rulez mcp --serve-skills` instead of, or as well as, being written into every tool's skill folder. See [Dynamic skill loading](/ai-rulez/mcp-server/#dynamic-skill-loading).

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `[skills] delivery` | `static`, `served`, `both` | `static` | Default delivery of every skill. A skill's `delivery` frontmatter, a role, and `[domains.<name>]` win. |
| `[domains.<name>] delivery` | `static`, `served`, `both` | `[skills] delivery` | Default delivery of one domain's skills. |
| `[[skill_sources]]` | array of tables | none | Skills served from a git repository or directory, never written to disk. |
| `[delivery.stub]` | table | built-in text | The generated `dynamic-skills` stub skill; see below. |

```toml
[skills]
delivery = "served"

[domains.backend]
delivery = "both"

[[skill_sources]]
name = "team-skills"
url = "https://github.com/acme/skills"
ref = "v1.4.0"
include = ["review-*"]
```

`[[skill_sources]]` keys: `name` and `url` (required), `ref` (a tag or full commit; a branch is reported as unpinned, `AR010`), `version`, `tag_prefix`, `include_prerelease`, `min_release_age`, `path`, `include`, `exclude`, `name_prefix`, `trust`, `domain`, and the limits `max_skills` (default 200), `max_bytes` (64 MiB), `max_clone_bytes` (256 MiB; `AI_RULEZ_MAX_CLONE_BYTES`) and `max_clone_files` (20000; `AI_RULEZ_MAX_CLONE_FILES`). A source over a limit is an error.

### `[delivery.stub]`

When skills are served, `generate` writes a `dynamic-skills` stub skill that tells the agent to call `find_skill` and `load_skill`. Its description is listed in every session.

```toml
[delivery.stub]
description = "Find and load team skills on demand."
body_file = "stub.md"
```

| Key | Type | Description |
| --- | --- | --- |
| `description` | string | Replaces the frontmatter description. Must not be only whitespace. |
| `body` | string | Replaces the text. |
| `body_file` | string | Reads the text from a file relative to the config directory and inside it. Exclusive with `body`; a missing file is an error. |

The body may use `{{server}}` (the MCP server that serves the skills) and no other variable, and must still name `find_skill` and `load_skill`. A skill you write named `dynamic-skills` replaces the stub.

## `[lock]`

How `ai-rulez.lock` is enforced. See [Lock file](/ai-rulez/lockfile/); an [organization policy](/ai-rulez/policy/) can force `enforce` and `include_outputs` on.

```toml
[lock]
enforce = true          # validate reports AR981 until `ai-rulez lock` has written content pins
min_release_age = "7d"
verify_tags = true
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `enforce` | boolean | `true` when `ai-rulez.lock` exists | `validate` reports content drift (`AR981`, `AR982`), `generate` refuses a remote source the lock does not pin, and `lock --check` and `generate --locked` require content pins; the skills server refuses a served skill whose digest the lock does not record. |
| `include_outputs` | boolean | `true` | Also pin digests of generated files. |
| `scope` | `all`, `skills` | `all` | Pin every authored item, or only skills and remote sources. |
| `min_release_age` | string | none | Default minimum age (`7d`, `12h`, `2w`) of a tag picked by a `version` constraint (`AR733`). |
| `min_release_age_source` | `auto`, `forge`, `first-seen`, `commit` | `auto` | Where a tag's release time comes from. |
| `refresh_on_generate` | boolean | `false` | `generate` re-resolves remote sources and rewrites the lock first, as `generate --refresh-remote` does. |
| `verify_tags` | boolean | `false` | Ask the remotes whether a pinned tag moved (`AR732`) or was deleted (`AR735`). |

## Plugins and marketplaces

### `[[plugins]]`

Declares plugins to install from a `[[marketplaces]]` entry. It writes nothing: Claude Code and Codex do not read a plugin list file, and `generate` warns that the table has no effect. Use [`[[enabled_plugins]]`](#enabled_plugins) or `[claude.settings] enable_plugins` instead.

Keys: `marketplace`, `name` (required), `scope` (`project` or `user`; default `project`), `enabled` (default `true`). `[[marketplaces]]` keys: `name`, `source` (required), `type` (`github`, `git`, `local`, `url`); they are recorded and write nothing.

### `[[enabled_plugins]]`

Enables a plugin from a marketplace this project does not author, for `claude` (`enabledPlugins` and `extraKnownMarketplaces` in `.claude/settings.json`), `codex` (`[plugins."<id>"]` in `.codex/config.toml`) and `opencode` (the npm package in `opencode.json`). See [Plugins](/ai-rulez/plugins/#enable-a-plugin-from-another-marketplace).

```toml
[[enabled_plugins]]
id = "formatter@acme"
marketplace = { name = "acme", source = "acme/plugins", type = "github" }
opencode = "acme-formatter-opencode"
```

| Key | Type | Description |
| --- | --- | --- |
| `id` | string | Required. `<name>@<marketplace>`. |
| `marketplace` | table | `name`, `source`, `type` (`github`, `git`, `url`, `directory`) and `ref`. Omit when the harness already knows the marketplace. |
| `opencode` | string | npm package of the plugin for OpenCode. Without it OpenCode is left alone. |

### `[placement]`

Whether skills and commands are written to `.claude/skills` (`core`) or shipped only through a plugin. Without the table every item is core. See [Plugins](/ai-rulez/plugins/#keep-a-skill-out-of-claudeskills).

```toml
[placement]
default = "core"
plugin = ["domains/*"]
core = ["domains/backend/python-conventions"]
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `default` | `core`, `plugin` | `core` | Placement of items no pattern decides. |
| `core` | array | none | Names or globs (`<name>` or `domains/<domain>/<name>`) kept in `.claude/skills`. Wins over `plugin`. |
| `plugin` | array | none | Names or globs left out of `.claude/skills`. |
| `honor_targets` | boolean | `false` | Apply frontmatter `targets` to skills in `.claude/skills`. |

### `[plugin]`

The producer side: packaging metadata for a distributable plugin bundle, built by `generate --plugin` and `publish`. Skills, commands and agents come from the content tree. See [Authoring plugins](/ai-rulez/plugins/) and [Agent Plugins](/ai-rulez/agent-plugins/).

```toml
[plugin]
name = "acme-tools"
version = "1.2.0"
description = "Acme review tooling."
runtimes = ["claude", "agent-plugins"]
```

`name` and `description` are required, and so is exactly one of `version` and `version_from`. Other keys: `display_name`, `version_from`, `author` (`name`, `email`, `url`), `homepage`, `repository`, `license`, `category`, `brand_color`, `icon`, `logo`, `keywords`, `tags`, `runtimes` (default: `claude`, `cursor`, `codex`, `gemini`, `kimi`, `opencode`, `factory` and `hermes`; `agent-plugins`, `copilot` and `pi` are written only when named, and the `codex` runtime also needs `[plugin.author]` `name` and `[plugin.interface]`), `spec` (Agent Plugins `1.0.0` (default) or `1.1.0`), `content_root`, `include_domains`, `include`, `include_evals`, `skills_dir_owned`, `provenance_headers`, `shared_resources`, `package_manager`, `opencode_dependency`, `opencode_dependency_enabled`, and the sub-tables `[[plugin.mcp]]`, `[[plugin.hooks]]`, `[plugin.statusline]`, `[plugin.interface]`, `[plugin.codex]`, `[plugin.cursor]`, `[plugin.gemini]`, `[plugin.kimi]`, `[plugin.hermes]`, `[plugin.pi]`.

To publish several plugins from one root, use `[[plugins_authoring]]` instead of `[plugin]`, and `[[marketplaces_authoring]]` for several marketplace indexes; `.ai-rulez/plugins/<name>.toml` and `.ai-rulez/marketplaces/<name>.toml` hold one entry each. `[marketplace]` describes the marketplace index written for the plugins (`name`, `description`, `owner`, `members`, `output_dir`, `from_domains`, `plugins`, `cursor_index`, `catalog_skill`). See [Several plugins from one root](/ai-rulez/plugins/#several-plugins-from-one-root).

## Validation and checks

### `[lint]`

Settings of `ai-rulez validate`: a profile (`default`, `strict`, `permissive`), the failure threshold, ignores, per-rule severities, size budgets, required frontmatter keys, external linters and more. Every key and finding code is in [Strict validation](/ai-rulez/strict-validation/). An [organization policy](/ai-rulez/policy/) can raise `severity`, `ignore` and `security` settings, and a repository cannot lower them.

```toml
[lint]
profile = "strict"
ignore = ["AR803"]
allow_overrides = ["testing-conventions"]

[lint.severity]
AR401 = "error"

[lint.budgets.skill]
max_lines = 400
```

Keys: `profile`, `fail_on`, `ignore`, `ignore_paths`, `ignore_rules`, `accept`, `tracked_only`, `severity`, `analyzers`, `ratchet`, `budgets`, `load_budgets`, `require_metadata`, `allowed_keys`, `metadata`, `allow_overrides`, `allow_paths`, `example_paths`, `known_names`, `builtin_names`, `skill_roots`, `description`, `risk`, `security`, `capability`, `external`, `scanner_policy`, `traps`, `agents_md_only`, `agents_md_only_allow`, `lint_outputs`, `paths`, `evals`.

### `[[verifiers]]`

Deterministic, read-only repository checks run by `ai-rulez verifiers run`. See [Verifiers](/ai-rulez/verifiers/).

```toml
[[verifiers]]
name = "node-version"
description = "package.json pins Node"
type = "key_equals"
path = "package.json"
key = "engines.node"
equals = ">=20"
```

Keys: `name` and `type` (required; one of `file_exists`, `file_absent`, `glob_count`, `regex`, `forbid`, `key_equals`, `generated_in_sync`), `description`, `severity` (`error` (default), `warning`, `info`), `path`, `glob`, `exclude`, `pattern`, `min`, `max`, `key`, `equals`, `profile`, `rule`, `skill`, `agent`, `command`, `anchor`, `message`, `fix`, `when_changed`, `require`, `examples`. A key that does not apply to the `type` is a configuration error. `config.local.toml` may add verifiers; entries merge by `name`.

`[verifiers_settings]` holds the limits of a run: `max_timeout_s` (default 300), `max_file_bytes` (default 5 MiB), `require_examples`, `warn_dead`, `trust_exec_from`, `command_env`. It is a separate table because TOML cannot use `verifiers` as both an array and a table.

### `[exposure]`

The repository's areas (the directories each one claims, its owner and the terms that name it) and the session exposure checks built on them. `harness` picks the load model: `"claude"` (default) or `"codex"`. Without the table nothing changes. See [Session Exposure and Areas](/ai-rulez/exposure/).

```toml
[exposure]
areas_file = "tools/context/areas.toml"
start_budget = 6000
```

Keys: `harness`, `context_window`, `plugins`, `areas_file`, `areas`, `skill_areas`, `start_budget`, `codex_start_budget`, `mcp_server_tokens`, `collision_min_score`.

## Exports and publishing

### `[okf]`

Settings of the `okf` preset, which keeps an [Open Knowledge Format](/ai-rulez/okf/) bundle of the content in sync.

```toml
presets = ["claude", "okf"]

[okf]
dir = "docs/okf"
include = ["rules", "context", "skills"]
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `dir` | string | `docs/okf` | Bundle directory inside the project, outside the config directory and `.git`. |
| `include` | array | all | Kinds exported: `rules`, `context`, `skills`, `agents`, `commands`, `checks`. |
| `index_style` | `body`, `frontmatter` | `body` | Where `index.md` files list their entries. |
| `spec` | `0.2` | `0.2` | OKF version written. |

The bundle is committed documentation: it is never gitignored, and its files carry no generated banner.

### `[llms_txt]`

Settings of the `llms-txt` preset. See [llms.txt](/ai-rulez/llms-txt/).

```toml
presets = ["claude", "llms-txt"]

[llms_txt]
title = "Acme"
full = true
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `dir` | string | project root | Output directory, outside the config directory and `.git`. |
| `title` | string | `name` | H1 of the file. |
| `summary` | string | `description` | Blockquote under the title. |
| `full` | boolean | `false` | Also write `llms-full.txt` with the full text of every item. |
| `include` | array | `rules`, `context`, `skills` | Kinds listed; `agents` and `commands` go to the `Optional` section. |

### `[ard]`, `[publish]` and `[catalog]`

| Table | Purpose | Keys |
| --- | --- | --- |
| `[ard]` | The [Agentic Resource Discovery](/ai-rulez/ard/) manifest written by `publish --emit ard`. | `publisher`, `namespace` (required), `base_url`, `plugin_type`, `queries` |
| `[publish]` | What [`ai-rulez publish`](/ai-rulez/publish/) ships and its gates. It holds no credentials. | `runtimes`, `require_signature`, `require_approved`, `allow_dirty`, `github_release`, `oci`, `npm`, `marketplace`, `emitters` |
| `[catalog]` | Defaults of [`ai-rulez catalog`](/ai-rulez/catalog/) and its static site. | `title`, `include_excerpt`, `exclude_owners`, `indexable`, `max_items_per_page`, `render_markdown` |

```toml
[publish]
runtimes = ["claude", "agent-plugins"]
require_signature = true

[publish.github_release]
repo = "acme/ai-tools"
```

## Governance and signing

| Table | Purpose | Keys |
| --- | --- | --- |
| `[governance]` | Which pinned content needs a recorded reviewer approval. See [Approvals](/ai-rulez/approvals/). | `require_approval`, `exempt`, `min_approvers`, `approvers`, `approvers_from`, `teams`, `min_assurance`, `forbid_self_approval`, `max_age`, `enforce` |
| `[signing]` | Who may sign `ai-rulez.lock` and how fresh the signature must be. See [Signing](/ai-rulez/signing/). | `require`, `tlog`, `max_age`, `trusted_root`, `min_hash_version`, `attestation`, `identity`, `issuer`, `key_file`, `trust`, `thresholds`, `require_provenance`, `builders` |

Both are shared policy: a layer or `config.local.toml` cannot change them.

## Model features

Features that call a language model. Nothing calls out unless `[llm] allow_network` is true, and the network, endpoint and credential keys (`allow_network`, `base_url`, `api_key_env`, `allow_plain_http`, `plain_http_hosts`, price overrides) are honored only from the user configuration (`~/.config/ai-rulez/config.toml`) or `AI_RULEZ_LLM_*` variables, never from `config.toml`.

| Table | Purpose | Page |
| --- | --- | --- |
| `[llm]` | Provider, model, budgets and cache: `provider`, `model`, `base_url`, `api_key_env`, `embedding_model`, `max_cost_usd`, `max_tokens`, `max_calls`, `cache`, `allow_network`, `timeout_seconds`, `max_retries`, `price_input_per_mtok`, `price_output_per_mtok`, `allow_plain_http`, `plain_http_hosts` | [LLM Access](/ai-rulez/llm/) |
| `[search]` | Ranking of `ai-rulez search` and `find_skill`: lexical by default, optionally hybrid with embeddings | [Skill search](/ai-rulez/search/) |
| `[review]` | `ai-rulez review`: rubric, content sent, exclusions, spend ceilings, gate | [Review](/ai-rulez/review/) |
| `[judge]` | `ai-rulez judge`: categories, gating, attached conventions, rubric skills, budgets | [Judge](/ai-rulez/judge/) |
| `[improve]` | `ai-rulez improve run`: held-out split, acceptance gate, limits, optimizer | [Improve](/ai-rulez/improve/) |

## Telemetry and usage

| Table | Purpose | Page |
| --- | --- | --- |
| `[telemetry]` | Opt-in item-load telemetry: local recording with `enabled`, OTLP export with `allow_network` and `otlp_endpoint`. Export, endpoint, header, path and resource keys are user scope only. | [Item-Load Telemetry](/ai-rulez/telemetry/) |
| `[usage]` | Usage telemetry support (no network): `skills_index`. | [Usage Telemetry](/ai-rulez/usage-telemetry/) |

## Layered configuration

Split the configuration across `config.<name>.toml` files beside `config.toml` and choose which apply. Active layers merge over `config.toml` in the order they are named, so a later layer wins.

```toml
# .ai-rulez/config.toml
default_layers = ["lint", "mcp"]
```

```toml
# .ai-rulez/config.mcp.toml
[[mcp_servers]]
name = "grafana"
command = "uvx"
args = ["mcp-grafana"]
```

```bash
AI_RULEZ_LAYERS=ai,custom ai-rulez generate   # add config.ai.toml, then config.custom.toml
ai-rulez config layers                        # list the layers and which are active
```

Layers become active in this order, each merged over the one before:

1. `default_layers` in `config.toml`: committed and always active. A listed layer without its file is an error; `default` is not a layer name and is skipped. The key is ignored, with a warning, inside a layer or the local overlay.
2. `AI_RULEZ_LAYERS` (comma-separated), or `AI_RULEZ_PROFILE` when it is not set, so one value can select a profile and the layer named like it. A name without a file is skipped. The `--profile` flag does not select layers.
3. `config.local.toml`, always last. `local` is a reserved layer name.

A name repeated across these keeps its first position. A layer per team-owned concern (lint settings, builtin exclusions, MCP servers) can have its own CODEOWNERS entry. Layers are not supported for a root `ai-rulez.toml`.

Layers merge like the local overlay ([merge rules](/ai-rulez/local-overrides/#merge-semantics)): tables merge per key, named lists (`mcp_servers`, `installed_skills`, `includes`, `scopes`, `roles`, `verifiers`, ...) merge entry by entry on `name` with `remove = true` dropping one, and other values are replaced. `[governance]` and `[signing]` are ignored in a layer. Commands that edit `config.toml` never write the merged view back. `ai-rulez config layers` prints every layer with its origin (`default`, `env`), also with `--format json`.

## Local overlay

`config.local.toml` beside `config.toml` is a gitignored, machine-local overlay merged onto the configuration at load time, after every layer. Use it for personal presets, profiles, MCP servers and secrets. It accepts the same keys as `config.toml` and is validated against `schema/ai-rules-local.schema.json`; `presets` merges as an ordered union where `"!name"` drops a shared preset.

```toml
# .ai-rulez/config.local.toml
presets = ["codex"]
```

Use the overlay to add: dropping a shared preset removes that preset's committed outputs; see the [warning](/ai-rulez/local-overrides/#config-overlay).

Merge rules, the `ai-rulez local` commands, the `.ai-rulez/local/` content tree and the drift guard are in [Local configuration](/ai-rulez/local-overrides/).

## Content precedence

### Deduplication by name

When a rule, context item, skill, agent, command or check name comes from several sources, the output contains it once. Precedence, highest first:

1. Project content (`.ai-rulez/rules/`, `.ai-rulez/skills/`, ...)
2. Domain content (`.ai-rulez/domains/<name>/...`)
3. Content from includes
4. Builtins

A project rule named `commit-messages` therefore replaces the builtin `git-workflow` rule of that name. `generate` and `validate` log each collapse:

```text
Duplicate rule collapsed name=commit-messages kept=.ai-rulez/rules/commit-messages.md dropped=builtin://universal/git-workflow/rules/commit-messages.md
```

A root item shadowing a domain item of the same name is resolved the same way. To keep both, give the domain item a different name.

### Output ID collisions

Two items in the same layer have no precedence between them. Skills and commands both render to `<skills dir>/<id>/SKILL.md`, so two that resolve to one id would overwrite each other; `validate` and `generate` refuse instead and name both files:

```text
duplicate output ids: command "deploy" (.ai-rulez/commands/deploy.md) vs command "deploy" (.ai-rulez/commands/deploy/COMMAND.md)
skill and command ids collide in the output namespace: skill "review" (.ai-rulez/skills/review/SKILL.md) vs command "review" (.ai-rulez/domains/qa/commands/review.md)
```

The check pools the root and every domain, because the output paths have no domain segment, and compares ids ignoring case. Keeping both the flat and the directory form of one command is the common cause: delete or rename one.

## Extending agents

Builtin and included agents (`code-reviewer`, `docs-writer`, `security-auditor`, ...) can be extended instead of copied: your agent appends to the base.

### Agent precedence

Same-named agents resolve by layer, highest first:

1. Project: `.ai-rulez/agents/`
2. Include: agents from includes
3. Builtin: agents from builtin packs

A higher agent replaces a lower one of the same name unless it sets `extends`.

### Append a message with `extends`

Create `.ai-rulez/agents/<name>.md` with the same id, set `extends` to that name, and write the additions:

```markdown
<!-- .ai-rulez/agents/code-reviewer.md -->
---
type: Reference
title: Code Reviewer
x-ai-rulez:
  kind: agent
  id: code-reviewer
  metadata:
    extends: code-reviewer
---

Also enforce, for this repository:

- No `.unwrap()` or `.expect()` in library code.
- Every `unsafe` block carries a SAFETY comment.
```

The generated `code-reviewer` is the base agent's instructions followed by your text. Frontmatter you set wins over the base; fields you omit are inherited:

```markdown
---
type: Reference
title: Docs Writer
x-ai-rulez:
  kind: agent
  id: docs-writer
  metadata:
    extends: docs-writer
    model: opus
    effort: high
---

When documenting the public API, include a runnable example in every supported language.
```

### How resolution works

`extends: <name>` binds to the agent of that name resolved from the layers below the current one: a project agent extends an include or builtin agent, an include agent extends a builtin agent, and chains compose in order (builtin, include, project).

When no lower agent of that name exists, or the chain has a cycle, the agent is generated with your body only and `generate` warns naming the missing target.

### Extend vs. redefine

- **Extend** (`extends`): the base plus your additions.
- **Redefine**: omit `extends` and write a complete agent; it replaces the base of that name.

## Validating the configuration

`ai-rulez validate` checks `config.toml`, its layers and `config.local.toml` against [`schema/ai-rules.schema.json`](/ai-rulez/schema/), then the structure: preset and builtin names exist, profile references resolve (an unknown domain is a warning), paths stay inside the project, MCP servers are well-formed, and the overlay merges into a valid configuration (`--no-local` skips it).

```bash
ai-rulez validate                 # configuration and content
ai-rulez validate --config-only   # configuration only
ai-rulez validate --strict        # warnings fail too
```

For editor completion, start `config.toml` with a schema reference:

```toml
#:schema https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules.schema.json
version = "5.0"
name = "acme"
```

## See also

- [Domains and profiles](/ai-rulez/domains/): organizing content by team or area
- [CLI Commands](/ai-rulez/cli/): `add`, `remove`, `list`, `domain`, `profile` and the other editing commands
- [Schema](/ai-rulez/schema/): the JSON schemas behind validation
