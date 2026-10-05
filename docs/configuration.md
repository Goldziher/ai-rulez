# Configuration Reference

V4 configuration reference for `.ai-rulez/config.toml` (defaults to TOML format).

## File-Based Configuration

V4 uses a file-based approach where you edit files directly with your editor or use CRUD commands:

- **Configuration**: Edit `.ai-rulez/config.toml` (TOML format) with any text editor
- **Rules**: Add/edit `.ai-rulez/rules/*.md` files or use `ai-rulez add rule`
- **Context**: Add/edit `.ai-rulez/context/*.md` files or use `ai-rulez add context`
- **Skills**: Add/edit `.ai-rulez/skills/{name}/SKILL.md` files or use `ai-rulez add skill`
- **Commands**: Add/edit `.ai-rulez/commands/{name}.md` (flat form) or `.ai-rulez/commands/{name}/COMMAND.md` (directory form with optional `references/` subdirectory)
- **Checks**: Add/edit `.ai-rulez/checks/{name}.md` code-review guidelines (frontmatter `description`, `severity`, `tools`, `targets`) or use `ai-rulez add check`; see [Checks](checks.md)
- **Agents**: Add/edit `.ai-rulez/agents/*.md` files or use `ai-rulez add agent` (`add command` likewise creates commands). The `claude` preset passes the documented Claude Code subagent keys (`disallowedTools`, `permissionMode`, `memory`, `maxTurns`, `mcpServers`, `hooks`, `background`, `isolation`, `color`, `initialPrompt`, `omitClaudeMd`) through with their YAML types; other presets ignore them. `validate` and `generate` warn about an agent key no tool reads (with a suggestion) and about a `skills:` entry that names no skill; `validate --strict` reports the same findings as `AR303` and `AR302`
- **Domains**: Add/edit `.ai-rulez/domains/{name}/{rules,context,skills,agents,commands,checks}/*.md` files or use `ai-rulez domain add`
- **MCP Servers**: Inline in `.ai-rulez/config.toml` (no separate mcp.yaml file)
- **Machine-local configuration**: Personal content under `.ai-rulez/local/` and a `config.local.*` overlay, both gitignored. See [Local overlay](#local-overlay)

You can either directly edit files with your editor or use CRUD commands for programmatic modification. After changes, run `ai-rulez generate` to create tool-specific outputs.

## Configuration Discovery

When no explicit path is given, the CLI discovers configuration by walking up from the current directory and trying, in order:

1. `.ai-rulez/config.{toml,yaml,yml,json}` — the tool-specific directory (default)
2. `.config/ai-rulez/config.{toml,yaml,yml,json}` — the project-level [`.config/` convention](https://github.com/pi0/config-dir)
3. Legacy V2 flat files: `ai-rulez.yaml`, `.ai-rulez.yaml`, and their `.yml` / underscore variants

`.ai-rulez/` wins when both directory layouts exist at the same level. `--config <file>` selects an exact file, and `--config-dir <path>` selects a non-default directory (for example `--config-dir .config/ai-rulez`). To scaffold the `.config/` layout, run `ai-rulez init --config-dir .config/ai-rulez`; generated outputs and the managed `.gitignore` block then reference `.config/ai-rulez/` instead of `.ai-rulez/`.

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

gitignore = true

[profiles]
full = ["backend", "frontend", "qa"]
backend = ["backend", "qa"]
frontend = ["frontend", "qa"]

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
name = "acme-platform"
```

Every project has one name; the value is used verbatim in generated headers.

## Content Layout

### Skills

Skills use a directory form with supporting resources:

```text
.ai-rulez/skills/deployment-checklist/
├── SKILL.md              # Main skill content
├── references/           # Markdown documentation (optional)
│   └── api-endpoints.md
├── scripts/              # Executable scripts (optional)
│   └── deploy.sh
└── assets/               # Binary assets (optional)
    └── diagram.png
```

The skill directory name becomes the skill id. Resources under `references/`, `scripts/`, and `assets/` are emitted as separate files in the generated output, preserving the Agent Skills progressive-disclosure model. Subdirectories outside these three produce a warning naming the skill and the unrecognized directory. Build artifacts (`.venv*`, `__pycache__`, `node_modules`, anything `.gitignore` ignores, ...) are not bundled; see [`bundle_exclude`](#bundle_exclude).

### Commands

Commands support both flat and directory forms:

**Flat form** (single file):

```text
.ai-rulez/commands/
├── deploy.md
└── review.md
```

**Directory form** (with supporting resources):

```text
.ai-rulez/commands/deploy/
├── COMMAND.md            # Main command content
└── references/           # Markdown documentation (optional)
    └── deployment-guide.md
```

The directory form mirrors the skill layout and supports `references/`, `scripts/`, and `assets/` subdirectories. Use it when a command needs bundled reference material.

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
  "claude",       # → CLAUDE.md, .claude/ (rules/, skills/, agents/)
  "cursor",       # → .cursor/rules/, .cursor/commands/, .cursor/agents/, .agents/skills/
  "gemini",       # → GEMINI.md, .gemini/ (settings.json, agents/), .agents/skills/
  "copilot",      # → .github/copilot-instructions.md, .github/instructions/, .github/{skills,agents,commands}/
  "devin",        # → .devin/
  "cline",        # → .clinerules/, .cline/
  "codex",        # → AGENTS.md, .agents/skills/ and .codex/
  "amp",          # → AGENTS.md and .agents/ (.amp/settings.json when an effort resolves)
  "junie",        # → AGENTS.md and .junie/ (rules/, skills/, agents/)
  "opencode",     # → AGENTS.md, .opencode/, opencode.json (when MCP servers are set)
  "hermes",       # → .hermes.md
  "antigravity",  # → .agents/ (rules/, skills/, agents/), GEMINI.md
  "xum",          # → AGENTS.md, .xum/skills, .xum/agents, .xum/mcp.jsonc (stdio with env as a shell prefix, http and sse MCP servers)
  "pi"            # → AGENTS.md, .agents/skills, .pi/agents, .pi/mcp.json (stdio and http MCP servers)
  "baz"           # → AGENTS.md (root and nested), .agents/skills, .claude/agents; see baz.md
]
```

With `agents_md = true`, one shared `AGENTS.md` and `.agents/skills/` replace the per-tool copies; see
[`agents_md`](#agents_md).

With the default `[rules] mode = "split"`, rules are written to each tool's native rules folder
(`.claude/rules/`, `.cursor/rules/`, `.github/instructions/`, `.junie/rules/`, `.agents/rules/`, and so
on) and the root file keeps context and the agent roster. See [Rules](rules.md#rules-mode) for what each
preset writes in each mode.

`mcp` is a built-in preset too, but it is a shared utility: it writes the generic `.mcp.json` and is
invoked automatically when MCP servers are configured, so you normally do not name it. It is the only
preset not produced by a coding-tool adapter, and `presets = ["mcp"]` alone generates nothing else.

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

The template uses Go's `text/template` with only its built-in functions; a helper such as `where`,
`truncate`, or `now` is not defined and fails to parse. See [Custom Presets](profiles.md) for the
available template data.

#### Provider-backed Presets (full parity)

A custom preset may reference a declarative **provider spec** instead of a
template. A provider spec has the same expressive power as the built-in presets
— root instructions file, skills/agents/commands, frontmatter, effort/model, and
MCP sidecars — and is validated against
[`schema/provider.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/provider.schema.json).

```toml
[[presets]]
name = "my-tool"
provider = ".ai-rulez/providers/my-tool.toml"
```

The `provider` path is relative to the project root and must not escape it. The
spec's `name` must match the preset's `name`. Example spec:

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
emit_when = "has_mcp_servers"   # also: always, has_plugins, has_resolved_effort, has_mcp_json_entries, has_mcp_servers_or_plugin_settings, has_resolved_effort_or_mcp_servers
```

Built-in presets are written as plain strings (`presets = ["claude", "xum"]`);
provider-backed presets use the inline-table form shown above. TOML, YAML, and
JSON configs all accept both.

##### Native tool and model names in a provider spec

A tool with its own tool and model vocabulary can be given a translation instead of Claude's names.
These keys go under `[outputs.<kind>.frontmatter]`:

| Key                 | Description |
| ------------------- | ----------- |
| `tool_names`        | Table of Claude tool name to the tool's own name for the `tools` list (matched ignoring case). A tool the table does not name is dropped; an agent left with none writes no `tools` key |
| `tool_case`         | `"lower"` writes every tool name lower-case (after `tool_names`) |
| `model_aliases`     | Table of resolved model (matched ignoring case) to the tool's own id; `""` drops the model |
| `drop_bare_aliases` | Drops a model that is `sonnet`, `opus`, `haiku` or `inherit`, so the agent inherits the session model; runs after `model_aliases` |

`[outputs.<kind>.body]` takes `replace` (a table of placeholder to replacement in the item's content, such as
`{ "$ARGUMENTS" = "$prompt" }`) and `replace_flag` (a frontmatter key written `true` when a replacement happened).
An `mcp` sidecar takes `transports` (`["stdio"]` leaves remote servers out), `env_ref_syntax` (`"dollar"` or
`"env_prefix"`: a value that came from a `${VAR}` placeholder is written as `$VAR` or `${env:VAR}` instead of the
resolved secret), and `elements.project_only` (the elements are project-relative and skipped in the user scope).

##### Split rule files in a provider spec

A provider can honour the `[rules] mode` setting like the built-in `claude` and `junie` presets by
adding a `split` rules output:

```toml
[root]
file = "MY_TOOL.md"
sections = ["title", "rules_inline", "context_inline"]

[outputs.rules]
mode = "per_item_file"
dir = ".my-tool/rules"
filename = "{id}.md"
split = true
dialect = "claude"          # claude | cursor | trigger | copilot | cline | continue | junie
inline_filter = "path_scoped"
```

| Field           | Description |
| --------------- | ----------- |
| `split`         | When `true`, the output follows `[rules] mode`: split mode writes every rule (and path-scoped context item) to a file, inline mode writes only the items `inline_filter` selects |
| `dialect`       | Required with `split`. The frontmatter vocabulary of the rule files; cannot be combined with `body` or `frontmatter` |
| `inline_filter` | Optional, `path_scoped` only. Items that still get a file in inline mode; omit to keep everything inline |
| `dir`           | Required with `split`: a relative path inside the project |
| `filename`      | Flat `{id}<ext>` template, no `/` |

`split` also requires `rules_inline` in `root.sections` and cannot be combined with `filter`. The
folder is treated like the built-in rules folders: hand-written files are never overwritten, hashes go
in the banner, and generated files are gitignored one by one. A provider without `split` inlines all
rules in its root file. Custom providers get no [local](local-overrides.md) output.

### `default`

The default profile name used when `ai-rulez generate` is run without `--profile`.

```toml
default = "full"
```

It may name several profiles to compose, the same as `--profile`:

```toml
default = "base,backend"
```

When `default` is not set, the built-in `default` profile applies and its meaning depends on whether
any profiles are defined:

- If **no profiles** are defined at all, `default` includes root content and **all** domains.
- If profiles are defined and one is named `default`, that definition is used.
- If profiles are defined but none is named `default`, `default` includes root content, globally-active
  builtin domains, and domains sourced from external includes — but not profile-only domains.

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

1. All root content (`.ai-rulez/rules/`, `.ai-rulez/context/`, `.ai-rulez/skills/`, `.ai-rulez/agents/`, `.ai-rulez/commands/`) is included
2. Content from the specified domains (`.ai-rulez/domains/{name}/`) is included
3. Globally-active builtin domains and every domain sourced from an external include are always included, whatever the profile
4. A builtin named as `builtin:<name>` in the profile is loaded for that profile only

Several profiles can be selected at once by separating them with commas — `--profile
base,backend`, or `default = "base,backend"` — which generates the union of their
domains. A profile name may therefore not contain a comma, and a profile's value lists
domains only, never other profiles. See
[Domains and Profiles](domains.md#composing-profiles).

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

A scoped file contains **only** the scope's own content (its profile's domains that the root run does
not already render), not the root rules and context: the target tools load a subdirectory `CLAUDE.md`/`AGENTS.md` on top of the root file, so
repeating the root content would duplicate the always-loaded text. The default scoped presets are
`codex` and `claude`, producing subfolder `AGENTS.md` and `CLAUDE.md` files.

A domain that the root output already contains (a builtin, an include, or one selected by the root profile)
is never repeated in a scope, and a scope left with nothing writes no files. Scope paths must be
relative, stay inside the project and contain no glob characters (`* ? [ ] { } , !`); `validate` rejects
anything else.

Rule files of a scope are written to the root rules folders with the scope path as a qualifier and glob
prefix, not into the scope directory; see [Scoped Rule Files](monorepo.md#scoped-rule-files).

### `gitignore`

Controls whether `ai-rulez` automatically updates `.gitignore` with generated output patterns.

```toml
gitignore = true   # Default: update .gitignore automatically
# gitignore = false  # Manage .gitignore yourself
```

When `true`, ai-rulez adds the specific generated files and owned subdirectories to `.gitignore` —
`AGENTS.md`/`CLAUDE.md`/`.mcp.json` and owned subtrees such as `.claude/skills/`, `.claude/agents/`,
`.codex/`, `.opencode/skills/`. It never ignores an assistant directory wholesale, so a hand-authored
`.opencode/settings.json` beside the generated files stays tracked. GitHub output is narrower:
ai-rulez ignores generated `.github/copilot-instructions.md`, `.github/agents/`, `.github/prompts/`,
and `.github/skills/` without ignoring all of `.github/`.

ai-rulez adds only what git does not already ignore. Before writing, it asks git (`git check-ignore`) about each
entry against every ignore source (all `.gitignore` files, `.git/info/exclude`, `core.excludesFile`) with its own
block left out, and skips an entry when a rule of yours already ignores it, or when your last matching rule is a
negation (`!CLAUDE.md`), which is read as a deliberate override and never re-ignored. The managed
`# BEGIN ai-rulez` / `# END ai-rulez` block holds only the remainder and is removed when nothing is left. A negated
machine-local or secret output (`*.local.*`, `config.local.*`, `.ai-rulez/local/`, a generated MCP config holding
secrets) is also left out of the block, with a warning; generation still refuses to write a secret-bearing MCP config
that ends up unignored. Outside a git repository, or when git is unavailable, every entry is added.

Machine-local outputs and sources are **always** gitignored, even when `gitignore = false`: the
local outputs (`CLAUDE.local.md`, `AGENTS.local.md`, `GEMINI.local.md`, `AGENTS.override.md`,
`<rulesdir>/*.local.*` including `ai-rulez.local.*`), the `.ai-rulez/local/` source tree, the
`config.local.*` overlay with its `.config.local.*` lock and temp files, and
`.ai-rulez/.generated-manifest.local.json`. Overlay-derived outputs whose names differ per machine are
excluded through `.git/info/exclude` instead. See [Local Configuration](local-overrides.md).

`gitignore` defaults to `true`, so generated files are normally not committed; set it to `false` when
the team commits generated output.

### `compact`

Controls whether generated inline rule sections omit per-rule `**Priority:**` annotations.

```toml
compact = false  # Default: include Priority lines
# compact = true   # Omit Priority annotations, reduce output size
```

When `true`, generated presets (CLAUDE.md, GEMINI.md, copilot-instructions.md, etc.) omit the per-rule
`**Priority:**` lines, reducing file size for large rule sets. Applies to all configured presets.

### `agents_md`

Renders the files several tools share once instead of once per tool.

```toml
agents_md = false  # Default: every preset writes its own files
# agents_md = true
```

When `true`, always-on rules and context go into one `AGENTS.md` (plus a nested `<scope>/AGENTS.md` for each
`[[scopes]]` entry) and skills go into one `.agents/skills/<name>/SKILL.md`. The presets that read these files
(`codex`, `opencode`, `amp`, `xum`, `pi`, `claude`, `gemini`, `antigravity`, `hermes`, `cursor`, `copilot`, `devin`,
`cline` and `junie`) stop writing their own `AGENTS.md` copy, root file and skills directory.
Turning the flag off regenerates the per-tool files and removes the shared ones that no preset writes itself.

Quick reference:

- Rules folders (`.cursor/rules`, `.claude/rules`, ...) keep only the items that are not always-on; the scoped items
  are also inlined into `AGENTS.md` when a configured preset has no folder for them (duplicated for tools that have
  one).
- `claude` keeps `CLAUDE.md` as a banner plus `@AGENTS.md`; `gemini` gets `AGENTS.md` added to `context.fileName` in
  `.gemini/settings.json`; `GEMINI.md`, `.hermes.md` and `.github/copilot-instructions.md`
  are not written.
- Skills with `targets`, machine-local files and agents, commands and MCP files stay per-preset.
- If another preset (including a custom provider) also writes `AGENTS.md` or a file under `.agents/skills`, the
  shared output wins.

Tool support, per-preset file lists, the duplication trade-off, `targets`, scopes and the on/off behavior are in
[AGENTS.md and .agents/skills](agents-md.md).

### `codex_skills_dir`

Where the `codex` preset writes skills, relative to the project root. The default is `.agents/skills`, the
directory Codex documents for repository skills (it scans `.agents/skills` from the working directory up to the
repository root; `.codex/skills` is not read). Releases before 4.24 wrote `.codex/skills`; set it back to keep that
location:

```toml
codex_skills_dir = ".codex/skills"  # Default: ".agents/skills"
```

After the default changes, `generate` writes the new tree and removes the files it previously wrote to
`.codex/skills` (only manifest-tracked files; hand-written skills stay). The path must stay inside the project.
With `agents_md` on, skills are written once to `.agents/skills` whatever this is set to.

### `bundle_exclude`

Extra patterns for files that are left out of a skill's or command's bundled resources (`references/`, `scripts/`,
`assets/`): they are neither listed in the generated `SKILL.md` `## Resources` section nor copied. Independently of
this key the bundle skips `.git`, `.venv*`, `venv`, `__pycache__`, `*.pyc` and `node_modules`, and, when the project
is in a git work tree, everything `.gitignore` ignores (tracked files and untracked files that are not ignored are
kept). A skill directory that is itself gitignored is bundled without the `.gitignore` filter.

```toml
bundle_exclude = ["*.log", "scripts/build/", "assets/raw/*.psd"]
```

A pattern without `/` matches any path segment (`*.log`, `tmp`); one with `/` matches the path relative to the skill
directory, or a directory on it. Patterns use glob syntax (`*`, `?`, `[a-z]`). Skills fetched through `includes` and
`installed_skills` get the built-in list and `.gitignore` only.

### `mcp`

Project-level MCP generation options.

```toml
[mcp]
self_server = true                # add ai-rulez's own MCP server to .mcp.json
# self_server_version = "4.19.0"  # default: the running binary's version ("latest" for a dev build)
# self_server_command = ["ai-rulez", "mcp"]  # replace the whole launch command instead of npx
```

| Field                 | Description                                                                                                                                              |
| --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `self_server`         | When `true`, `generate` adds `"ai-rulez": {"type": "stdio", "command": "npx", "args": ["-y", "ai-rulez@<version>", "mcp"]}` to the project `.mcp.json`. |
| `self_server_version` | Version (or dist-tag) to pin. Defaults to the version of the ai-rulez binary running `generate`, or `latest` for a dev build. Needs `self_server`.       |
| `self_server_command` | Executable followed by its arguments, replacing the npx launch. Mutually exclusive with `self_server_version`. Needs `self_server`.                      |

Behavior:

- **Merged, not replaced**: with `self_server` and no `[[mcp_servers]]`, ai-rulez owns only the
  `mcpServers.ai-rulez` entry. Other servers in an existing `.mcp.json` (and every other key)
  are preserved, so running `generate` twice produces no diff.
- **Only `.mcp.json`**: `.claude/settings.json` and the other presets' MCP files are not written
  for `self_server` alone, so hand-authored settings are never touched.
- **With `[[mcp_servers]]`**: declared servers keep their existing behavior (ai-rulez owns the whole
  `mcpServers` object, see [Settings document merge behavior](#settings-document-merge-behavior)),
  and the ai-rulez entry is added to it unless you declare a server named `ai-rulez` yourself, which
  wins.
- **Per root**: each config root decides independently, including under `generate --recursive`.
- The pinned version changes when you upgrade ai-rulez, so the generated `.mcp.json` changes with it.
- **Turning it off**: removing `self_server` stops ai-rulez from owning the entry but does not
  delete it; remove `mcpServers.ai-rulez` from `.mcp.json` yourself. `ai-rulez clean` likewise leaves a
  `.mcp.json` that holds hand-authored servers in place.

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
| `command`     | No       | Command to run for local `stdio` servers (npx, uvx, ai-rulez, etc.). May contain `${PROJECT_ROOT}`. |
| `args`        | No       | Array of command arguments for local servers. Elements may contain `${PROJECT_ROOT}`. |
| `env`         | No       | Environment variables as key-value pairs. Values may contain `${VAR}` placeholders. |
| `transport`   | No       | `stdio`, `http`, or `sse`. Defaults to `stdio`.                                     |
| `url`         | No       | Remote MCP server URL for `http` or `sse` transports.                               |
| `headers`     | No       | HTTP headers for `http` or `sse` servers (e.g. auth). Values may contain `${VAR}` placeholders. |
| `enabled`     | No       | Set to `false` to skip the server in generated MCP outputs. Defaults to `true`.     |
| `profiles`    | No       | Restrict the server to the named profiles. Omit to include it in every profile.     |

Local `stdio` servers normally need `command`; remote `http` and `sse` servers normally use `url`,
plus `headers` when the server needs auth:

```toml
[[mcp_servers]]
name = "remote-api"
transport = "http"
url = "https://mcp.example.com/mcp"
headers = { Authorization = "Bearer ${REMOTE_API_TOKEN}", X-Team = "platform" }
```

`headers` is rejected on a `stdio` server, header names must be valid HTTP tokens (and unique ignoring
case), and values must not contain line breaks. Each preset writes them where its tool reads them
(`headers` in every MCP file ai-rulez generates). Headers are not included in distributable plugin
bundles: `generate --plugin` warns for each affected server and does not require header placeholders
to resolve.

A `command` or `args` value may use the `${PROJECT_ROOT}` placeholder, which resolves to the
project root (the directory containing `.ai-rulez/`) during `ai-rulez generate`:

```toml
[[mcp_servers]]
name = "repo-tools"
command = "node"
args = ["${PROJECT_ROOT}/scripts/mcp.js"]
```

This is the portable way to pass an absolute project path to a server that requires one, without
hardcoding a machine-specific path. Because it resolves to an absolute path, the generated file
carrying it is machine-specific: keep the output gitignored or regenerate it per machine. In `env`
values, `${PROJECT_ROOT}` also resolves this way unless a real `PROJECT_ROOT` is supplied via
`--env`, the process environment, or a dotenv file, in which case that value wins.

For Claude Code specifically, a project-scoped `.mcp.json` can instead use Claude's own
`${CLAUDE_PROJECT_DIR:-.}` token in `command`/`args`; ai-rulez passes it through unchanged, and it
stays portable across machines.

MCP env and header placeholders use `${VAR}` syntax, where `VAR` must match `[A-Za-z_][A-Za-z0-9_]*`. They are
resolved during `ai-rulez generate` from repeated `--env KEY=VALUE` flags, process environment
variables, then dotenv files. By default, `ai-rulez` loads `.env` from the generation base directory.
If any `--env-file PATH` flags are supplied, the default `.env` is not loaded; files are merged in
flag order, with later files winning. Generation fails if a placeholder cannot be resolved.

Generated MCP config files contain resolved values. If any resolved MCP env or header value comes
from a placeholder, or if an env key or header name looks sensitive (`TOKEN`, `SECRET`, `PASSWORD`,
`KEY`, `CREDENTIAL`, or the `Authorization`, `Proxy-Authorization` and `Cookie` headers), generation
fails before writing unless the generated MCP config path is covered by `.gitignore` or by the
patterns `ai-rulez` is about to add. Protected MCP output paths are `.mcp.json`,
`.claude/settings.json`, `.gemini/settings.json`, `.agents/mcp_config.json`, and `opencode.json`,
including scoped variants such as `packages/web/.claude/settings.json`. Resolved secret values are redacted before
source-hash calculation, but generated MCP config files contain the actual resolved values.

#### Settings document merge behavior

Files such as `.claude/settings.json`, `.mcp.json`, `.amp/settings.json`, `.gemini/settings.json`,
`.agents/mcp_config.json` and `.pi/mcp.json` are **shared documents**: ai-rulez owns specific top-level keys
(`mcpServers` for MCP config, `amp.anthropic.effort` for Amp) and the consumer owns everything else.
Generation replaces only the owned keys and preserves every other member byte-for-byte, including
the document's original indentation.

**Important implications**:

- **MCP servers are owned one by one**: ai-rulez owns the servers it writes into `mcpServers`
  (`mcp.<name>` in `opencode.json`, `servers` in `.xum/mcp.jsonc`), not the whole object. A server
  you added by hand under a name that is not in `config.toml` survives `generate` and `clean`. A
  server dropped from `config.toml` is removed on the next `generate`, and only while it still holds
  the value ai-rulez wrote: an entry you edited is yours, stays, and is reported once. If a
  hand-written server has the same name as a configured one, the configured value wins on
  `generate` and is the one recorded.
- **Hooks, permissions and managed keys**: top-level `[[hooks]]`, `[permissions]` and
  `[claude.settings.managed]` add `hooks`, `permissions.allow|ask|deny`, `env` and `skillOverrides`
  to the shared documents, owned element by element (hook groups, rules) or entry by entry (env,
  skill overrides). Hand-authored keys, groups and rules in the same arrays survive `generate` and
  `clean`. See [Hooks and permissions](settings.md).
- **Plugin keys of `.claude/settings.json`**: with `[claude.settings] manage = true`, ai-rulez also
  owns `extraKnownMarketplaces.<marketplace.name>` and the listed `enabledPlugins.<plugin>@<marketplace>`
  entries, each entry on its own; other marketplaces and plugins in those objects survive.
- **Written only when there is something to contribute**: A settings document is emitted only when
  the config declares MCP servers (or, for Amp, a resolved effort tier; for Claude, managed plugin keys). The `gemini` and
  `antigravity` presets previously wrote their settings document on every run purely to self-register
  the ai-rulez MCP server; they no longer do, so a project with no `[[mcp_servers]]` keeps whatever
  is already at `.agents/mcp_config.json` untouched. The exceptions are `.gemini/settings.json`
  (`context.fileName`) and `opencode.json` (`instructions`), which carry the entry that loads
  machine-local content.
- **JSONC**: A document containing comments or trailing commas is not valid JSON, and rewriting it
  would delete the comments. When MCP servers must be written into it, generation fails with a hint
  naming the path. When only the Gemini `context.fileName` or OpenCode `instructions` entry would be
  written, the document is left untouched with a warning and generation continues.
- **Taking keys back out**: `ai-rulez clean`, and `generate` after a preset or server is removed,
  remove the keys ai-rulez merged in, as long as they still hold the value it wrote, and keep the rest
  of the document; see
  [Settings documents shared with you](local-overrides.md#settings-documents-shared-with-you).
- **Gitignore behavior**: A document still holding keys ai-rulez does not own is treated as the
  user's file: it is NOT added to the managed `.gitignore` block and is NOT deleted as stale. This
  preserves hand-authored settings such as Claude's `permissions`, `env`, `model`, and `statusLine`.
  A document holding only ai-rulez's own keys remains a generated artifact and is still gitignored
  (keeping resolved MCP secret values out of git).

### `plugins`

Plugins to install from a configured marketplace. This is the **consumer** side of the plugin
config: ai-rulez emits these declarations into `.claude/plugins.json` and `.codex/plugins.json`,
and the target tool performs the install. It is distinct from the **producer** `[plugin]` block
(see [Authoring Plugins](plugins.md)) that packages this project as a plugin.

```toml
[[plugins]]
marketplace = "official"   # name of a [[marketplaces]] entry
name = "my-plugin"
scope = "project"          # project or user; defaults to project
enabled = true             # defaults to true
```

!!! warning "Deprecated"
    `.claude/plugins.json` and `.codex/plugins.json` are not read by Claude Code or Codex (checked
    2026-10-04). Claude Code records installs in `.claude/settings.json` (`enabledPlugins`,
    `extraKnownMarketplaces`); Codex enables plugins with `[plugins."name@marketplace"] enabled = true`
    in `config.toml`. `generate` keeps writing the files and now warns when `[[plugins]]` is set
    together with the `claude` or `codex` preset. Migrate to [`[claude.settings]`](#claudesettings)
    for Claude Code, and to a `config.toml` entry you keep yourself for Codex; ai-rulez does not write
    `.codex/config.toml` plugin entries because it owns that file outright. The Copilot
    `enabledPlugins` settings syntax is not shown in the vendor documentation, so it is not generated.

### `marketplaces`

Plugin marketplaces to register. ai-rulez records these sources but does not currently emit any
marketplace output for them; a `[[plugins]]` entry references a marketplace by `name`.

```toml
[[marketplaces]]
name = "official"
source = "https://github.com/org/marketplace"   # GitHub repo, git URL, local path, or URL
type = "github"                                  # github, git, local, or url
```

### `placement`

Decides whether skills and commands are generated into `.claude/skills` (`core`, the default) or
shipped only through a plugin. Absent block: nothing changes. See
[Keep a skill out of `.claude/skills`](plugins.md#keep-a-skill-out-of-claudeskills).

```toml
[placement]
default = "core"
plugin = ["domains/*"]
core = ["domains/backend/python-conventions"]
honor_targets = false
```

### `claude.settings`

Opt-in ownership of `extraKnownMarketplaces.<marketplace.name>` and `enabledPlugins.<plugin>@<marketplace>`
in `.claude/settings.json`, merged like `mcpServers` (see
[Settings document merge behavior](#settings-document-merge-behavior)). See
[Register the marketplace in `.claude/settings.json`](plugins.md#register-the-marketplace-in-claudesettingsjson).

```toml
[claude.settings]
manage = true
enable_plugins = ["acme-essentials"]
```

### `claude.skills`

By default a skill is user-invocable and appears in Claude Code's `/` menu. `hide_from_menu = true` writes
`user-invocable: false` on every skill that does not set the key itself, so only the model loads it. A `user-invocable`
value in a skill's frontmatter always wins. See [Skill Frontmatter](skills.md#invocation-keys).

```toml
[claude.skills]
hide_from_menu = true
```

### `codex`

`project_doc_max_bytes` is the limit your Codex is configured with (default 32 KiB). `generate` warns when the
`AGENTS.md` files Codex concatenates exceed it, naming the largest sections; `0` turns the warning off. See
[Rules: size limits](rules.md#size-limits).

```toml
[codex]
project_doc_max_bytes = 65536
```

`[claude.settings.managed]` additionally owns `env.<NAME>` and `skillOverrides.<skill>` entries without
`manage = true`; `[[hooks]]` and `[permissions]` render the hooks and permission rules. See
[Hooks and permissions](settings.md).

### `hooks` and `permissions`

Top-level `[[hooks]]` (lifecycle hooks for `claude`, `codex`, `cursor`, `gemini` and `copilot`) and
`[permissions]` (`allow`, `ask`, `deny` rules for `claude`) are rendered into each harness's native
settings file outside any plugin, merged key by key so hand-authored content survives. See
[Hooks and permissions](settings.md).

### `builtins`

Enables built-in domains that ship embedded in the `ai-rulez` binary. These provide opinionated rules,
skills, agents, context, and commands without needing external includes.

**Omit the `builtins` field entirely and no builtin content is loaded**, auto-included domains included.
Auto-inclusion applies only once the field is present, so the minimum opt-in is `builtins = true` or any
array value.

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

#### Scope a builtin to a profile

A builtin pack can be named in a profile's domain list with the `builtin:` prefix. The pack
is then loaded for that profile only, instead of every profile:

```toml
[profiles]
backend = ["backend", "builtin:docker"]
frontend = ["frontend"]
```

`backend` sees the `docker` pack; `frontend` does not. This works even when the root
`builtins` field is absent or set to `false`, because a profile reference is an explicit
opt-in. A pack the root `builtins` field already enables stays global — the prefix selects
a pack, it does not un-globalize one. Prefixing also keeps a builtin (`builtin:rust`) from
colliding with a local domain of the same name.

#### Exclude auto-included builtins

`ai-governance` is auto-included whenever builtins are configured. Exclude it with `!`:

```toml
builtins = ["rust", "!ai-governance"]
```

#### Drop the agents roster from root files

`agent-delegation` is auto-included, and it is what makes every root instructions file
(`CLAUDE.md`, `AGENTS.md`, …) end in an `## Agents` section listing each agent's name and
description. Exclude the domain to drop that section:

```toml
builtins = ["!agent-delegation"]
```

The per-agent files (`.claude/agents/*.md`) are still generated, so no capability is lost — the
roster is a second copy of text the agent files already carry, and the root file is the copy read
on every request. On a tree with 32 agents the roster measures about 1,100 always-loaded tokens;
`ai-rulez tokens` reports it as the `agents_delegation` line so you can see the figure for your own
tree before deciding.

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

| Domain              | Description                                                        |
| ------------------- | ------------------------------------------------------------------ |
| `ai-governance`     | AI agent behavior governance (auto-included)                       |
| `agent-delegation`  | Agent delegation instructions and listing (auto-included)          |
| `security`          | Security best practices and OWASP reference (auto-included)        |
| `git-workflow`      | Git workflow and commit conventions (auto-included)                |
| `code-quality`      | Code readability, error handling, and complexity (auto-included)   |
| `testing`           | Testing conventions and best practices (auto-included)             |
| `token-efficiency`  | Output efficiency and task automation (auto-included)              |
| `cicd`              | CI/CD pipeline standards and GitHub workflow conventions           |
| `docker`            | Container build, security, and deployment best practices           |
| `observability`     | Logging, metrics, health checks, and observability standards       |
| `documentation`     | Documentation standards and maintenance                            |
| `polyglot-bindings` | Cross-language binding and native FFI conventions                  |
| `default-commands`  | Built-in slash commands (`/iterate`, `/parallelize`)               |

**Languages** (per-language conventions):

`rust`, `python`, `typescript`, `go`, `java`, `ruby`, `php`, `elixir`, `csharp`, `r`

**Bindings** (FFI binding conventions):

`pyo3`, `napi-rs`, `magnus`, `ext-php-rs`, `rustler`, `wasm`, `jni-rs`, `extendr`, `cgo`, `vite-plus`

#### What Builtins Provide

**Universal builtins** carry opinionated content for their domain, split between always-on rules
(under the default split mode, written to `.claude/rules/*.md`; with `[rules] mode = "inline"`,
inlined into `CLAUDE.md`) and on-demand skills (loaded only when relevant):

- `ai-governance` — rules: read-before-write, minimal changes, verification before completion, systematic debugging, agent workflow, communication style, no AI signatures, explain reasoning
- `security` — rules: input validation, secrets handling, least privilege. Skills: `owasp-quick-reference`, `dependency-awareness` (with per-language audit tool recommendations)
- `git-workflow` — rules: conventional commits, atomic commits, branch hygiene, safe operations
- `code-quality` — skills only: `code-quality-standards` (readability, complexity limits, anti-patterns, duplication, dead code), `error-handling`
- `testing` — rule: tests ship with the behaviour change. Skills: `tdd-workflow`, `testing-conventions` (naming, assertions, independence, anti-patterns)
- `token-efficiency` — rules: context preservation, output awareness. Skills: `task-runner`, `incremental-approach`
- `agent-delegation` — context: delegation instructions and the generated agent listing
- `cicd` — rules: pipeline standards, GitHub workflow conventions
- `docker` — skill only: `container-standards`
- `observability` — skill only: `observability-standards`
- `documentation` — rules: inline docs, README standards, docs-with-code updates
- `polyglot-bindings` — skills only: Rust-core/native ABI boundaries, FFI ownership, cross-language error conversion, binding parity
- `default-commands` — commands: `/iterate` (implementation + review cycles) and `/parallelize` (subagent task splitting)

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

#### Builtins as On-Demand Agent Skills

Convention-heavy builtins no longer put their content into `CLAUDE.md`, `.claude/rules/` (or the other
always-loaded surfaces). Instead they emit as **Agent Skills** — `.claude/skills/<id>/SKILL.md` files with YAML
`name:` / `description:` frontmatter — that the assistant loads on demand only when the work is
relevant. This keeps the always-loaded governance file small while still shipping the full
conventions.

Builtins that emit as skills:

- **Language** domains: `rust`, `python`, `typescript`, `go`, `java`, `ruby`, `php`, `elixir`, `csharp`, `r`
- **Binding** domains: `pyo3`, `napi-rs`, `magnus`, `ext-php-rs`, `rustler`, `wasm`, `jni-rs`, `extendr`, `cgo`, `vite-plus`
- **`polyglot-bindings`** conventions
- **`security`**: the `owasp-quick-reference` and `dependency-awareness` entries
- **`code-quality`**, entirely: `code-quality-standards` and `error-handling`
- **`testing`**, all but one rule: `tdd-workflow` and `testing-conventions`
- **`token-efficiency`**: `task-runner` and `incremental-approach`
- **`docker`** and **`observability`**, entirely: `container-standards` and `observability-standards`

What stays inline is behavioural governance that has to land before the first file is read, where a skill
loaded after the fact is too late: all of `ai-governance` and `git-workflow`, `security`'s
`secrets-handling`, `input-validation` and `least-privilege`, `token-efficiency`'s `context-preservation`
and `output-awareness`, and `testing`'s `test-alongside-code`. Guidance that only matters once you are
already inside a specific activity — writing a test, writing an error path, writing a Dockerfile — is a
skill, because its description can name that trigger and its body then costs nothing until it fires.

Exclusions still work exactly the same for these now-skill entries. `!domain` drops a whole domain and
`!domain/name` drops a single rule/skill within it (array form of `builtins` only):

```toml
builtins = ["rust", "security", "!security/owasp-quick-reference"]
```

#### Merge Priority

Builtins have the **lowest** priority. Content is merged in this order:

1. **Builtins** (lowest) — embedded in binary
2. **Includes** — from git repos or local paths
3. **Project content** (highest) — in your `.ai-rulez/` directory (not to be confused with
   machine-local content under `.ai-rulez/local/`, see [Local overlay](#local-overlay))

If a local domain has the same name as a builtin, the local domain is used and the builtin is skipped entirely.

### `defaults`

Top-level defaults that propagate into generated outputs when individual content files do not override them.

```toml
[defaults]
effort = "medium"  # low | medium | high | xhigh | max | inherit

# Suppress agent frontmatter fields that a target tool would reject.
omit_agent_fields = ["model", "tools"]

[defaults.effort_by_preset]
codex = "high"
claude = "xhigh"
amp = "max"

[defaults.model_by_preset]
claude = "opus"
copilot = "gpt-5"
cursor = "claude-3.7-sonnet"
```

**`defaults.omit_agent_fields`** suppresses named agent frontmatter fields for every preset, so an
agent stays loadable where a field would be invalid — an unconfigured model or provider, or a tool
name the target tool does not recognize. Recognized values are `model`, `effort`, `tools`, and
`description`; an omitted field is simply not written. This trades strictness for loadability, which
is usually the right call when the same agent is generated for many tools.

**`defaults.effort`** sets the reasoning effort applied to every preset that supports it. Per-agent overrides (via agent frontmatter) win where the preset accepts per-agent effort; if neither is set, the field is omitted entirely.

**`defaults.effort_by_preset`** lets you override `defaults.effort` for specific presets. Per-agent metadata still wins. Useful when, for example, you want Codex to reason harder than Claude on the same project.

**`defaults.model_by_preset`** sets the agent `model` value per preset. Model strings are provider-specific (`opus` makes sense for Claude, `gpt-5` for Copilot) so there is no provider-neutral `defaults.model` scalar — every entry is preset-scoped. Per-agent `<preset>_model` frontmatter still wins; the legacy single-value `model` field on an agent acts as the lowest-priority fallback. Presets that do not emit a per-agent model frontmatter (`codex`, `antigravity`) ignore entries for their preset.

**OpenCode models must be `provider/model`** (for example `anthropic/claude-sonnet-4-5`). OpenCode reads a bare alias such as `sonnet` as a provider with no model: it drops the agent file or fails the session with `Model not found`. When the resolved model for `opencode` is not provider-qualified, ai-rulez warns, omits `model`, and the agent inherits the session's model. To pin a model for OpenCode only, set `opencode_model` in the agent frontmatter or `[defaults.model_by_preset] opencode = "provider/model"`; either wins over the shared `model` field, so the same agent can keep `model: sonnet` for Claude. OpenCode agent `hidden` is written as a boolean, and `temperature` and `top_p` as top-level numbers; values that do not parse are omitted with a warning.

**Resolution order** (per preset, per agent):

1. Per-agent `effort` in agent frontmatter (Claude, Codex, Devin, Opencode, Xum — presets that support per-agent effort)
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
| `claude`                                                                       | `.claude/agents/<id>.md` frontmatter                                            | `effort`                 | Per-agent. Full vocabulary including `max`. `inherit` is not a Claude effort value and is dropped.                                                                                  |
| `codex`                                                                        | `.codex/agents/<id>.toml` (per-agent) and `.codex/config.toml` (global default) | `model_reasoning_effort` | Per-agent override beats global `.codex/config.toml`. `max` → `high`; `inherit` dropped.                                                                                           |
| `amp`                                                                          | `.amp/settings.json`                                                            | `amp.anthropic.effort`   | Global only. `xhigh` → `high`.                                                                                                                                                     |
| `devin`                                                                        | `.devin/agents/<id>.md` frontmatter                                             | `reasoning_effort`       | Per-agent. `max` → `high`; `inherit` dropped.                                                                                                                                      |
| `opencode`                                                                     | `.opencode/agents/<id>.md` frontmatter                                          | `variant`                | Per-agent. A separate `variant:` key beside a plain `provider/model` (the `model#variant` form is `opencode.json` only); a `#variant` in the source model is split off. `xhigh` and `max` → `high`; `inherit` dropped.                                       |
| `xum`                                                                         | `.xum/agents/<id>.md` frontmatter                                               | `ai.thinkingLevel`       | Per-agent. `xhigh` and `max` → `high`; `inherit` dropped.                                                                                                                          |
| `pi`                                                                          | `.pi/agents/<id>.md` frontmatter                                                | `thinking`               | Per-agent. Full vocabulary; `inherit` dropped.                                                                                                                                     |
| `cursor`, `copilot`, `gemini`, `junie`, `antigravity`, `cline`                 | —                                                                               | —                        | These tools either gate effort behind UI toggles or read it from user-managed config files. ai-rulez does not emit anything for them; configure effort in the tool's own settings. |

**Per-preset model matrix** — presets that emit a `model` value in their agent frontmatter:

| Preset         | Per-agent frontmatter key | Emitted field                                |
| -------------- | ------------------------- | -------------------------------------------- |
| `claude`       | `claude_model`            | `model` in `.claude/agents/<id>.md`          |
| `copilot`      | `copilot_model`           | `model` in `.github/agents/<id>.agent.md`    |
| `cursor`       | `cursor_model`            | `model` in `.cursor/agents/<id>.md`          |
| `cline`        | `cline_model`             | `modelId` in `.cline/agents/<id>.yaml`         |
| `amp`          | `amp_model`               | `model` in `.agents/agents/<id>.md` (Amp)    |
| `junie`        | `junie_model`             | `model` in `.junie/agents/<id>.md`           |
| `opencode`     | `opencode_model`          | `model` in `.opencode/agents/<id>.md`        |
| `devin`        | `devin_model`             | `model` in `.devin/agents/<id>.md`           |
| `gemini`       | `gemini_model`            | `model` in `.gemini/agents/<id>.md` (Gemini) |
| `xum`          | `xum_model`               | `ai.model` in `.xum/agents/<id>.md`          |
| `pi`           | `pi_model`                | `model` in `.pi/agents/<id>.md`              |

A bare Claude alias (`sonnet`, `opus`, `haiku`) is not a Gemini model, so `gemini` omits it with a warning and the agent inherits the session model; `gemini_model` and `defaults.model_by_preset.gemini` are written as given.

### `rules`

Controls how rules are written to generated outputs.

```toml
[rules]
mode = "split"           # split (default) | inline

[rules.mode_by_preset]
claude = "inline"
```

- **`rules.mode`**: `split` (the default since 4.22.0) writes one file per rule in the tool's native rules folder; `inline` embeds rules in the root file, moving only path-scoped rules to the folder.
- **`rules.mode_by_preset`**: per-preset override that beats `rules.mode`. Keys are built-in, custom, or provider preset names.
- **`rules.baz_scoped`**: `nested` (the default) writes the path-scoped rules and context of the `baz` preset to the `AGENTS.md` of the directory their globs point into; `root` keeps them in the root `AGENTS.md`. See [Baz](baz.md).

Set `mode = "inline"` to restore the pre-4.22.0 output. Per-tool output, fallbacks and caveats are in [Rules and native rules folders](rules.md).

### `lint`

Tunes `ai-rulez validate --strict`: severities, ignores, allow-lists, description bounds, size budgets and
required frontmatter keys.

```toml
[lint]
fail_on = "warning"
ignore = ["AR803"]

[lint.severity]
AR401 = "error"

[lint.budgets.skill]
max_lines = 400

[lint.require_metadata]
skill = ["owner"]
```

Every key, the finding codes and the exit codes are in [Strict validation](strict-validation.md).

### `header`

Configures the style of headers in generated files. Headers provide context about ai-rulez, explain the folder structure, and instruct AI agents on proper usage.

```toml
[header]
style = "minimal"   # Default: bare minimum header
# style = "compact"  # Shorter header with key information
# style = "detailed" # Comprehensive header with full documentation
# timestamp = true  # Emit the "Generated:" line (default: false)
# text = "..."      # Override the generated prose (see "Custom header")
# hashes = "content" # Freshness lines: "full" (default), "content", or "none"
```

The default is `minimal`. Every style — including `minimal` — carries the "DO NOT EDIT"
warning and the injected `Content-Hash` / `Source-Hash` freshness lines (see [`hashes`](#hashes)) that ai-rulez uses to
detect whether a generated file (or its sources) changed since the last `generate`. Only the
amount of explanatory prose differs between styles.

#### `hashes`

Every generated file carries freshness lines in its header (a YAML comment inside the frontmatter for
skills and agents, an HTML comment in `CLAUDE.md`). `hashes` selects which ones:

| Value               | Lines written                 | Effect                                                                                      |
| ------------------- | ----------------------------- | ------------------------------------------------------------------------------------------- |
| `full` (default)    | `Content-Hash`, `Source-Hash` | Unchanged behaviour.                                                                        |
| `content`           | `Content-Hash`                | Each header depends only on that file's own body.                                           |
| `none`              | neither                       | No hash lines at all.                                                                       |

`Content-Hash` is a hash of the file's own body and is stable. `Source-Hash` is a hash of the entire
source set (config, every rule, skill, agent, command, MCP server), so under `full` one edit changes the
`Source-Hash` line in every generated file. That is harmless for gitignored output, but when generated
files are committed it makes every change to `.ai-rulez/` rewrite hundreds of files and makes concurrent
branches conflict. With `content` or `none`, editing one skill changes only that skill's output (and
`CLAUDE.md` when the edit changes what `CLAUDE.md` renders).

With `content` or `none`, `generate` decides whether to rewrite a file by comparing the whole rendered
file to what is on disk (ignoring the `Generated:` text when `timestamp = true`), so a changed header
style, custom header or config directory still re-renders. Consequently a formatter that edits generated
files is undone by the next `generate`, as it would be for any committed-output check. `clean` does not
read these lines in any mode. `verify --plugin` uses its own per-bundle provenance (the
`.ai-rulez-generated.json` sidecar) and is independent of this setting. Switching modes rewrites every
file once. Any other value is rejected by `validate`.

#### `timestamp`

Headers carry no `Generated:` line by default, so generated output is byte-reproducible: the same
sources generate the same bytes, output can be verified by content hash, and two files rendered from
the same sources — `CLAUDE.md` and `AGENTS.md`, which are otherwise identical — cannot drift apart.
Set `timestamp = true` to stamp each header with the time of the run:

```toml
[header]
timestamp = true
```

One run resolves the timestamp once and stamps every file it writes with that value, so sibling
outputs always agree even when a large project takes more than a second to render. Set
[`SOURCE_DATE_EPOCH`](https://reproducible-builds.org/docs/source-date-epoch/) to a Unix second count
to pin it, keeping output reproducible with the line in place:

```bash
SOURCE_DATE_EPOCH=$(git log -1 --pretty=%ct) ai-rulez generate
```

A value that is not a parsable integer is ignored and the wall clock is used.

#### Header Styles

**`minimal`** (default)

- Only critical information
- Brief "DO NOT EDIT" warning
- MCP server reference
- `Content-Hash` / `Source-Hash` freshness lines
- Best for: keeping always-loaded files small (the default)
- Size: ~10 lines

**`compact`**

- Condensed version with essential information
- Uses symbols (✗/✓) for clarity
- Brief structure overview
- Best for: projects that want a short structure overview
- Size: ~20 lines

**`detailed`**

- Comprehensive explanation of ai-rulez
- Complete folder organization documentation
- Full AI agent instructions with MCP server promotion
- Best for: projects where AI agents need thorough context
- Size: ~50 lines

#### Custom header

When the predefined styles do not match your environment — for example the default banner tells
agents to use `npx`, but your project manages tools with `mise` — set `text` to replace the
generated prose with your own. It overrides `style` entirely (the `style` value is ignored while
`text` is set) and is written verbatim, wrapped in the output's comment syntax:

```toml
[header]
text = """
Built by the platform team.
Set up with: mise run setup
Regenerate with: ai-rulez generate
"""
```

A few details worth knowing:

- The `Content-Hash` / `Source-Hash` freshness lines are still appended inside the banner, so
  hash-based skip detection keeps working. Do not add a closing comment marker of your own.
- `timestamp = true` still does nothing to a custom header; add a `Generated:` line in `text`
  yourself if you want one.
- Surrounding blank lines are trimmed; interior line breaks are preserved. Standard TOML multi-line
  escaping applies (`"""` for a literal block).

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

## Local overlay

A `config.local.{toml,yaml,yml,json}` file beside `config.toml` is a machine-local, gitignored overlay
that is merged onto this configuration in memory at load time. Use it for personal presets, profiles,
MCP servers and secrets. It accepts the same keys as `config.toml` (it is validated against
`schema/ai-rules-local.schema.json`; see [Schema Reference](schema.md)) and merges as follows: scalars
and lists are replaced by the local value, tables merge per key, `presets` is an ordered union where
`"!name"` drops a shared preset, and the named lists (`mcp_servers`, `plugins`, `includes`,
`installed_skills`, `marketplaces`, `scopes`) merge by name, with `remove = true` deleting a shared
entry. Unknown keys, a mismatched `version` and more than one overlay file are errors, and plugin
bundles (`generate --plugin`) ignore the overlay.

```toml
# .ai-rulez/config.local.toml
presets = ["codex", "!cursor"]
```

The full merge table, the `ai-rulez local` commands, `--local` content under `.ai-rulez/local/` and
the drift guard are documented in [Local Configuration](local-overrides.md), which is the single
source of truth for this feature.

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
  - claude
  - ".cursor/rules/"
custom_field: value
---

# Rule or Context Title

Your content here. Can include any markdown formatting.
```

### Frontmatter Fields

**`priority`** (optional, string)

- Values: `critical`, `high`, `medium`, `low`, `minimal`
- Default: `medium`
- Controls the order of the **Rules** and **Context** sections in generated files (higher priority
  first, name order breaking ties)

```yaml
---
priority: critical
---
```

**`targets`** (optional, array of strings)

- Selects which generated **outputs** include this content (not which source files it applies to; use `globs` / `paths` for that): preset names, root files, file paths or base
  names, directory prefixes (`.cursor/rules/`), globs, or `*`. Applies to rules-folder files and root files.
- If empty, included in all outputs
- See [Targets](rules.md#targets) for the match rules

```yaml
---
targets:
  - "CLAUDE.md"
  - ".cursor/rules/*"
---
```

**`globs` / `paths`** (optional, array of strings — rules)

- The files a rule applies to. The two keys are synonyms; either sets the same path scope. A
  comma-separated string is split into several globs (commas inside `{}` are kept).
- A **path-scoped rule is not inlined into the root instructions file**. Each preset with a rules
  folder (`.claude/rules`, `.cursor/rules`, `.github/instructions`, ...) writes it there with native
  frontmatter; presets without one keep it inline under an `_Applies to: ..._` line.
- `activation` (`always`, `glob`, `auto`, `manual`) and `description` select other modes. See
  [Rules and native rules folders](rules.md).

```yaml
---
globs:
  - "**/*.tsx"
---
```

**`effort`** (optional, string — agents only)

- Values: `low`, `medium`, `high`, `xhigh`, `max`, `inherit`
- Sets the reasoning effort for a Claude Code subagent in its generated `.claude/agents/<name>.md` frontmatter
- Available levels depend on the model
- Falls back to `defaults.effort` in `config.toml` when not set, then to the session-level default
- Presets that don't emit effort — `cursor`, `copilot`, `gemini`, `junie`, `cline`, `antigravity` — omit it from their outputs. See the [per-preset support matrix](#defaults).

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

When the same rule, context, skill, agent or command **name** appears in multiple sources, the generated output includes it only **once**. Precedence (highest to lowest):

1. **Root content** (`.ai-rulez/rules/`, `.ai-rulez/context/`, etc.)
2. **Domain content** (`.ai-rulez/domains/{name}/rules/`, etc.)
3. **Include-sourced content** (`FromInclude` domains from external includes)
4. **Builtin content** (auto-included domains)

Example: If both a builtin `git-workflow` domain and a local rule define `commit-messages`, the local version is used and the builtin version is dropped.

Rules and context are *inlined*, so a duplicate there would render the same section twice. Skills and
commands are *per-item files* (`.claude/skills/{id}/SKILL.md`), so a duplicate is worse than doubled
output: both copies are written to one path and whichever lands last wins. Deduplication makes the
precedence above the one that actually reaches disk.

Shadowing a builtin skill with a project skill of the same name is a supported, intended pattern — it
is a warning, never an error.

During `ai-rulez generate` and `ai-rulez validate`, a warning is logged for each deduplicated item:

```text
Duplicate rule collapsed name=commit-messages kept=.ai-rulez/rules/commit-messages.md dropped=builtin://universal/git-workflow/rules/commit-messages.md
Duplicate skill collapsed name=go-conventions kept=.ai-rulez/skills/go-conventions/SKILL.md dropped=builtin://languages/go/skills/go-conventions/SKILL.md
```

### Name Collision Handling

If a name appears in both root and a domain:

```text
.ai-rulez/rules/testing.md
.ai-rulez/domains/backend/rules/testing.md
```

The root version takes precedence and the domain copy is dropped — the same precedence order as
above. To keep a domain-specific version, give it a name no other layer uses, or remove the root copy.

### Output ID Collisions

Deduplication resolves collisions *across* precedence layers. It cannot resolve two items in the
**same** layer: there is no precedence between them. Skills and commands both render to
`.claude/skills/{id}/SKILL.md`, differing only in the `user-invocable` frontmatter a command gets, so two
items in one scope resolving to one id means one silently overwrites the other. `ai-rulez validate`
(and `generate`) refuse instead, with the two source paths named.

Two skills, or two commands, in the same directory:

```text
duplicate output ids: command "deploy" (.ai-rulez/commands/deploy.md) vs command "deploy" (.ai-rulez/commands/deploy/COMMAND.md)
```

The flat form (`commands/deploy.md`) and the directory form (`commands/deploy/COMMAND.md`) resolve to
the same id, so keeping both is the most common way to hit this. Delete one, or rename one side.

A skill and a command sharing an id:

```text
skill and command ids collide in the output namespace: skill "review" (.ai-rulez/skills/review/SKILL.md) vs command "review" (.ai-rulez/domains/qa/commands/review.md)
```

This check pools root and every domain, because the output layout has no domain segment: a skill in
one domain and a command in another still land on the same path for any profile that activates both.
Ids are compared case-insensitively, since a macOS or Windows checkout treats `Review/` and `review/`
as one directory.

Root shadowing a domain is *not* an error — it is the documented resolution above, and it applies to
a command regardless of which form each side uses.

## Extending Agents

Built-in and shared-module agents (`code-reviewer`, `docs-writer`, `security-auditor`, language
specialists, ...) give you a solid base. When you only need to *add* project-specific guidance, don't
copy the whole agent — **extend** it.

### Agent precedence

Same-named agents resolve deterministically by layer, highest precedence first:

1. **Project** — agents under your `.ai-rulez/agents/`
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

- A **project** agent extends an **include** or **builtin** agent.
- An **include** agent extends a **builtin** agent.
- Chains compose in resolution order (builtin → include → project), across as many layers as extend one
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
gitignore = true

[profiles]
full = ["backend", "frontend", "qa", "devops"]
backend = ["backend", "qa"]
frontend = ["frontend", "qa"]
qa = ["qa"]
devops = ["devops"]
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

presets = [
  "claude",
  "cursor",
  "gemini",
  "devin",
  { name = "internal-guide", type = "markdown", path = "docs/AI_DEVELOPMENT_GUIDE.md" },
]

default = "full"
gitignore = true

[profiles]
full = ["research", "ml-ops", "infrastructure", "frontend"]
research = ["research"]
ml-ops = ["ml-ops", "infrastructure"]
frontend = ["frontend"]
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

For V4 configurations `ai-rulez validate` checks the raw config file against the JSON schema
(`schema/ai-rules.schema.json`) — so an unknown key or a value outside an enum is reported rather
than silently dropped — and then runs the structural checks below. TOML is converted to JSON for
the schema check. V3 configurations are still accepted (`version = "3.0"`) and get the structural
checks only, since the schema is V4-shaped.

```bash
ai-rulez validate
```

The structural checks cover:

- `version` is `"4.0"` or `"3.0"` (for backward compatibility)
- `name` is present and non-empty
- All preset names are valid
- `builtin:<name>` references in profiles name a real builtin
- Profile references resolve (a missing domain is a warning, not a failure)
- File paths are valid
- MCP server definitions are well-formed
- A `config.local.*` overlay, if present, passes `schema/ai-rules-local.schema.json` and merges into a valid config (`--no-local` skips this)

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
├── skills/          # Domain-specific AI skills
├── agents/          # Domain-specific agents
└── commands/        # Domain-specific commands
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
ai-rulez add agent <name>            # Create an agent
ai-rulez add command <name>          # Create a command

# Remove content
ai-rulez remove rule <name>          # Delete a rule
ai-rulez remove context <name>       # Delete context
ai-rulez remove skill <name>         # Delete a skill
ai-rulez remove agent <name>         # Delete an agent
ai-rulez remove command <name>       # Delete a command

# List content
ai-rulez list rules                  # List all rules
ai-rulez list context                # List all context
ai-rulez list skills                 # List all skills
ai-rulez list agents                 # List all agents
ai-rulez list commands               # List all commands
```

Add `--local` to any `add`, `remove` or `list` command to work on the machine-local tree
`.ai-rulez/local/` instead (see [Local Configuration](local-overrides.md)).

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

`include add|remove`, `skill install|remove` and `profile add|remove|set-default` also take `--local`,
which writes to the `config.local.*` overlay.

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
6. **Commit sources, and outputs only if you set `gitignore = false`**: `gitignore` defaults to `true`, so generated files are normally ignored; commit `.ai-rulez/` always, and generated files only when the team chooses to version them

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

# Commit (add generated files too only if gitignore = false)
git add .ai-rulez/
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
# Good: one responsibility per domain
[profiles]
full = ["api", "frontend", "infrastructure"]
```

```toml
# Bad: domains that bundle unrelated concerns
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
