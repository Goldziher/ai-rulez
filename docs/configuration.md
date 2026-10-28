# Configuration Reference

Configuration reference for `.ai-rulez/config.toml` (YAML and JSON configs are no longer read; `ai-rulez migrate v5` converts them).

## File-Based Configuration

ai-rulez uses a file-based approach where you edit files directly with your editor or use CRUD commands:

- **Configuration**: Edit `.ai-rulez/config.toml` (TOML format) with any text editor
- **Rules**: Add/edit `.ai-rulez/rules/*.md` files or use `ai-rulez add rule`
- **Context**: Add/edit `.ai-rulez/context/*.md` files or use `ai-rulez add context`
- **Skills**: Add/edit `.ai-rulez/skills/{name}/SKILL.md` files or use `ai-rulez add skill`
- **Commands**: Add/edit `.ai-rulez/commands/{name}.md` (flat form) or `.ai-rulez/commands/{name}/COMMAND.md` (directory form with optional `references/` subdirectory)
- **Checks**: Add/edit `.ai-rulez/checks/{name}.md` code-review guidelines (frontmatter `description`, `severity`, `tools`, `targets`) or use `ai-rulez add check`; see [Checks](checks.md)
- **Agents**: Add/edit `.ai-rulez/agents/*.md` files or use `ai-rulez add agent` (`add command` likewise creates commands). The `claude` preset passes the documented Claude Code subagent keys (`disallowedTools`, `permissionMode`, `memory`, `maxTurns`, `mcpServers`, `hooks`, `background`, `isolation`, `color`, `initialPrompt`, `omitClaudeMd`) through with their YAML types; other presets ignore them. `validate` and `generate` warn about an agent key no tool reads (with a suggestion) and about a `skills:` entry that names no skill; `validate` reports the same findings as `AR303` and `AR302`
- **Domains**: Add/edit `.ai-rulez/domains/{name}/{rules,context,skills,agents,commands,checks}/*.md` files or use `ai-rulez domain add`
- **MCP Servers**: Inline in `.ai-rulez/config.toml`
- **Machine-local configuration**: Personal content under `.ai-rulez/local/` and a `config.local.toml` overlay, both gitignored. See [Local overlay](#local-overlay)

You can either directly edit files with your editor or use CRUD commands for programmatic modification. After changes, run `ai-rulez generate` to create tool-specific outputs.

## Configuration Discovery

When no explicit path is given, the CLI discovers configuration by walking up from the current directory to the nearest directory holding a configuration, stopping at the repository root (never above the home directory), and trying in each directory, in order:

1. `.agents/ai-rulez.toml` — the configuration of a project whose content is [`.agents` and `AGENTS.md`](agents-source.md)
2. `.ai-rulez/config.toml` — the tool-specific directory (default)
3. `.config/ai-rulez/config.toml` — the project-level [`.config/` convention](https://github.com/pi0/config-dir)
4. `ai-rulez.toml` at the project root — a [configuration without a `.ai-rulez/` folder](config-without-folder.md); a config directory at the same level wins, with a warning that the file is ignored

`config.toml` is the only config format read, and its `version` must be `"5.0"`. A project that has only a
`config.yaml`, `config.yml` or `config.json` (or a `config.local.yaml`, `.yml` or `.json` overlay), a flat `ai-rulez.yaml`
(also `.ai-rulez.yaml`, `ai_rulez.yaml` and their `.yml` forms), or a `config.toml` with an older `version`, fails with an
error that names the file; run `ai-rulez migrate v5` to convert it (a 2.x or 3.x project goes through ai-rulez 4.x
first). See [Migrating to v5](migration-v5.md).

`.agents/ai-rulez.toml` wins over `.ai-rulez/` when both exist at the same level, and `.ai-rulez/` wins over `.config/ai-rulez/`. `--config <file>` selects an exact file, and `--config-dir <path>` selects a non-default directory (for example `--config-dir .config/ai-rulez`). To scaffold the `.config/` layout, run `ai-rulez init --config-dir .config/ai-rulez`; generated outputs and the managed `.gitignore` block then reference `.config/ai-rulez/` instead of `.ai-rulez/`.

## Checks `generate` runs before it writes

`generate` runs the same schema check as `ai-rulez validate` over `config.toml` and the machine-local
`config.local.toml`. A key the schema does not know (`delivry`, `[lock] enforc`) has no effect, so `generate`
prints a warning naming the file and the key, with the nearest known key when there is one:

```text
WARN  Configuration problem: .ai-rulez/config.toml: unknown key "lock.enforc" (did you mean "enforce"?)
```

`generate --strict-config`, or `AI_RULEZ_STRICT=1` in CI, fails with exit code 1 instead. `validate` and `generate` report
the same unknown keys.

`--strict` means something different on each command, and they do not imply one another:

| Command | `--strict` adds |
| --- | --- |
| `generate --strict-config` | Schema check only: an unknown or invalid configuration key fails the run instead of warning. |
| `validate` | Deep content checks (dead links and references, globs that match nothing, size and duplicate checks, findings `AR###`). Unknown keys already fail `validate` without it. |

Run both in CI: `ai-rulez validate && ai-rulez generate --strict-config`.

`generate` also lists the commands it is about to write that your tools will run: `[[hooks]]` commands (and the
content digest of a hook `script` file, so a changed script behind the same path is listed again), `http` and `prompt`
hooks, command-based `[[mcp_servers]]`, `[permissions] allow` rules for every harness that takes them,
`[claude.settings.managed] env` entries (`NODE_OPTIONS`, `ANTHROPIC_BASE_URL`, ...), and plugin enablement and
marketplace registration. Only commands that are new or changed since the previous run on this machine are listed
(every command on a first run in a fresh clone), one item per line. It only warns, it is printed even with `--quiet`,
and `--yes` or `AI_RULEZ_ACK_COMMANDS=1` silences it. The previous run is remembered in `~/.cache/ai-rulez/commands/`,
never in the repository.

The text is repository-controlled, so it is made safe to print: control characters (ESC, CR, LF, other C0/C1 codes),
bidirectional overrides and zero-width characters appear as visible escapes (`\x1b`, `\u202e`), and anything that
looks like a credential is replaced by `<redacted>` (secret-named flags such as `--token=...`, `Bearer`/`Authorization`
values, URL user information and query values, `NAME=value` with a secret-looking name, well-known token prefixes, long
base64 or hex runs). An env value is shown unless its name or value looks like a credential. `generate --watch` prints
the same list on every regeneration that changes a command, and the MCP `generate_outputs` tool returns it as
`new_commands` (and writes it to stderr) because it has no terminal.

## Basic Structure

The minimal valid configuration:

```toml
version = "5.0"
name = "my-project"
```

A typical production configuration:

```toml
version = "5.0"
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

The config schema version. Must be `"5.0"`; `"4.0"` and older are rejected with a pointer to `ai-rulez migrate v5`.

```toml
version = "5.0"
```

### `name`

The project name. Used in generated files and displayed in headers.

```toml
name = "acme-platform"
```

Every project has one name; the value is used verbatim in generated headers.

## Lint-only configuration

`presets` is optional. A configuration with no preset (omitted or `presets = []`) and no `[plugin]` block is a lint-only configuration: `validate`, `scan` and `[[lint.external]]` run against the files already in the repository, while `generate` prints `No presets are configured: nothing to generate`, writes nothing (not `AGENTS.md`, not `.mcp.json`) and exits 0. `generate --check` and `doctor` report no drift. It works in the directory layout and in the [root `ai-rulez.toml` layout](config-without-folder.md#lint-only-configuration).

```toml
version = "5.0"
name = "lint"

[lint]
profile = "strict"
```

## Content Layout

### Symlinks in content

A symlinked file or directory under your project's `.ai-rulez/` (including `domains/` and `local/`) is followed only
when its fully resolved target is inside the project (the git top-level, else the directory holding `.ai-rulez/`). Any
other symlink, including a dangling one, is refused: `generate` fails with an error naming it and
`ai-rulez validate` reports it as an error. Content from includes, installed skills, skill sources and OKF bundles
never follows symlinks; see [Includes](includes.md).

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
  "devin",        # → AGENTS.md, .devin/ (rules/, skills/, agents/, mcp_config.json)
  "cline",        # → .clinerules/, .cline/
  "codex",        # → AGENTS.md, .agents/skills/ and .codex/
  "amp",          # → AGENTS.md and .agents/ (.amp/settings.json when an effort resolves)
  "junie",        # → AGENTS.md and .junie/ (rules/, skills/, agents/)
  "opencode",     # → AGENTS.md, .opencode/, opencode.json (when MCP servers are set)
  "hermes",       # → .hermes.md
  "antigravity",  # → .agents/ (rules/, skills/, agents/), GEMINI.md
  "xum",          # → AGENTS.md, .xum/skills, .xum/agents, .xum/mcp.jsonc (stdio with env as a shell prefix, http and sse MCP servers)
  "pi",           # → AGENTS.md, .agents/skills, .pi/agents, .pi/mcp.json (stdio and http MCP servers)
  "baz",          # → AGENTS.md (root and nested), .agents/skills, .claude/agents; see baz.md
  "okf"           # → docs/okf/, an Open Knowledge Format bundle of the content; see okf.md
]
```

These are 15 of the 52 built-in presets. The complete list, with the features each supports, is in
[Supported harnesses](harnesses.md); the names are:

`aiassistant`, `amp`, `antigravity`, `augment`, `baz`, `bob`, `claude`, `cline`, `codebuddy`, `codebuff`,
`codewhale`, `codex`, `commandcode`, `copilot`, `copilot-cli`, `cortex`, `crush`, `cursor`, `deepagents`, `devin`,
`dsh`, `factory`, `gemini`, `gitlab-duo`, `goose`, `grok`, `hermes`, `junie`, `kilo`, `kimi`, `kiro`, `letta`,
`mimocode`, `muse`, `omp`, `openclaw`, `opencode`, `pi`, `poolside`, `qoder`, `qwen`, `reasonix`, `replit`,
`rovodev`, `takt`, `trae`, `vibe`, `warp`, `xum`, `zcode`, `zed`, `zoocode`.

The `windsurf` preset is now `devin` and `continue-dev` was removed; `ai-rulez doctor` reports both. See
[Migrating to v5](migration-v5.md).

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
emit_when = "has_mcp_servers"   # also: always, has_plugins, has_resolved_effort, has_mcp_json_entries, has_mcp_servers_or_plugin_settings, has_resolved_effort_or_mcp_servers, has_hooks
```

Built-in presets are written as plain strings (`presets = ["claude", "xum"]`);
provider-backed presets use the inline-table form shown above.

##### Output path rules

A configuration comes from the repository, so every output path it names (a
custom preset `path`, a provider spec's `root.file`, `outputs.*.dir`,
`sidecars[].path` and `directories`, `okf.dir`, `marketplace.output_dir`,
`codex_skills_dir`) must be a relative path inside the project. `validate` and
`generate` reject an absolute path, a `..` segment, a drive or UNC prefix, any
`.git` segment (any case), and a path under `.ai-rulez/`, `.config/ai-rulez`,
`.hg` or `.svn`. A custom preset that writes a file CI or a developer tool runs
(`.github/workflows/`, `Makefile`, `.vscode/tasks.json`, ...) is allowed, and the
`generate` command summary lists it as `exec-file` before writing. Generation
also refuses any write that resolves outside the project or inside `.git`.

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
An `mcp` sidecar takes `transports` (`["stdio"]` leaves remote servers out), `env_ref_syntax` (`"dollar"`,
`"env_prefix"`, `"braced"` or `"opencode_env"`: a value that came from a `${VAR}` placeholder is written as `$VAR`,
`${env:VAR}`, `${VAR}` or `{env:VAR}` instead of the resolved secret), and `elements.project_only` (the elements are project-relative and skipped in the user scope).

A `hooks` sidecar renders the top-level `[[hooks]]` into the tool's own hooks file. Its `dialect` names the
harness whose hook format is used (`claude`, `qwen`, `factory`, `kiro`, `vibe`, ...; the harnesses and what
each writes are in [Hooks and permissions](settings.md)), which fixes the key and the layout, so a hooks
sidecar takes no `key`; it is emitted when `[[hooks]]` are declared (`emit_when = "has_hooks"`):

```toml
[[sidecars]]
kind = "hooks"
dialect = "qwen"
path = ".qwen/settings.json"
global_path = ".qwen/settings.json"
```

Sidecars that name one `path` (an `mcp` and a `hooks` sidecar of one settings file) are merged into a single
document, so neither replaces the other.

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
in the banner, and, with `gitignore = true`, generated files are gitignored one by one. A provider without `split` inlines all
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

An entry may also be the wildcard `*`, which selects every domain the config knows, or an exclusion
`!name`, which removes a domain from the selection. Resolution starts from the positive entries
(expanding `*`), then removes the negated ones, so order does not matter and the result is
deterministic. This is how a monorepo keeps most domains in the root output while a `[[scopes]]`
subdirectory renders the rest:

```toml
[profiles]
# Every domain except "s"; "s" is rendered only in its own scope.
all-but-s = ["*", "!s"]
```

A profile made only of exclusions — `["!a"]` with no positive entry — is an error, because there is
nothing to subtract from; write `["*", "!a"]` to select everything except `a`. An exclusion that
names an unknown domain is a warning, exactly as an unknown positive name is, and is ignored.

Several profiles can be selected at once by separating them with commas — `--profile
base,backend`, or `default = "base,backend"` — which generates the union of their
domains. A profile name may therefore not contain a comma, and a profile's value lists
domains only, never other profiles. See
[Domains and Profiles](domains.md#composing-profiles) and, for how profiles differ from
roles, [Profiles vs roles](roles.md#profiles-vs-roles).

### `roles` and `role_manifest`

`[[roles]]` maps a job to a slice of the content (domains, per-kind include/exclude selectors, inheritance, a
per-skill `skill_mode` rendered as Claude Code `skillOverrides`), selected with `ai-rulez generate --role <name>`.
Like `--profile`, `--role` accepts a comma-separated list to compose several roles; see
[Composing roles](roles.md#composing-roles). `[role_manifest] enabled = true` writes `<config dir>/roles.json`; `skill_mode_fallback = "drop" | "serve"` says what an `off` skill does on a harness with no setting for it. See [Roles](roles.md).

```toml
[role_manifest]
enabled = true

[[roles]]
name = "backend"
domains = ["backend"]
[roles.skills]
exclude = ["deploy-*"]
[roles.skill_mode]
migrate = "name-only"
```

### `lock`

`[lock]` tunes how strictly `ai-rulez.lock` is enforced: `enforce` (default `true` whenever `ai-rulez.lock` exists, `enforce = false` opts out) makes `validate`
report content drift (or an unreadable lock) as `AR981` / `AR982`, makes `generate` refuse a remote include or installed skill the lock does not pin (and `AR010` an error), and makes `generate --locked` require content pins (`lock --check` always does); `include_outputs`
(default `true`) pins generated outputs; `scope` is `all` (default) or `skills`. See [Lock file](lockfile.md).
An [organization policy](policy.md) can force `enforce` and `include_outputs` on.

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

Opt-in: controls whether `ai-rulez` maintains a managed block in `.gitignore` with generated output patterns.
It is off by default, because committed outputs should not flip-flop in and out of the ignore block.

```toml
gitignore = true     # Maintain the managed .gitignore block (or pass `generate --gitignore`)
# gitignore = false  # Default: leave .gitignore alone
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

Your lines are yours. ai-rulez edits only between `# BEGIN ai-rulez` and `# END ai-rulez`; every byte outside
the block stays as it was, blank lines included (nothing is inserted under `# END ai-rulez`). A negation of a
directory entry (`!.claude/skills/`, `!/.claude/skills/`) is honored the same way as one of a file entry
(`!CLAUDE.md`): the entry is left out of the block. `ai-rulez doctor` does not report an output you re-included this
way, since `generate` leaves it alone; it still reports a machine-local or secret output a rule un-ignores, and says
that `generate` will not re-ignore it.

With `gitignore = true` the block is the same on every machine, whether or not `config.local.*` or `.ai-rulez/local/`
exist and with or without `generate --no-local`: it always lists `.ai-rulez/config.local.*` with its
`.config.local.*` lock and temp files, `.ai-rulez/local/` and `.ai-rulez/.generated-manifest.local.json`. A path that
exists only because this machine's local inputs add it (`CLAUDE.local.md`, `AGENTS.local.md`, `GEMINI.local.md`,
`AGENTS.override.md`, `<rulesdir>/*.local.*`, a `.claude/settings.json` that only the overlay's hooks create, local
skills) is never listed: it is excluded through `.git/info/exclude`, which is not committed.

With `gitignore = false` the machine-local inputs that exist (the `config.local.*` overlay, the `.ai-rulez/local/`
tree) are still gitignored in the project's `.gitignore`. The machine-local manifest alone does not make `generate`
write a tracked `.gitignore`: it is excluded through `.git/info/exclude` instead. See
[Local Configuration](local-overrides.md).

`gitignore` defaults to `false`, so generated files are normally committed; set it to `true` when the team
keeps generated output out of git and regenerates it locally.

### `gitignore_exclude`

Keeps generated outputs you commit on purpose out of the managed `.gitignore` block, while every other entry stays
managed. Use it instead of a hand-written `!pattern` after the block.

```toml
gitignore = true
gitignore_exclude = ["AGENTS.md", "**/CLAUDE.md", ".claude/skills/", ".claude/agents/", ".opencode/skills/"]
```

Entries use gitignore notation, matched against the patterns the block would list: an entry without `/`
(`AGENTS.md`, `*.md`) matches at any depth, one with a `/` is anchored at the root, `**` crosses directories
(`**/AGENTS.md` also reaches the nested `svc/AGENTS.md` of a `[[scopes]]` entry), and a trailing `/` names a directory
and covers what is below it. An entry is a plain pattern: an empty one, a comment or a `!negation` fails validation.
The knob has no effect unless the block is written (`gitignore = true` or `generate --gitignore`); `gitignore` itself stays a
plain boolean. Machine-local and secret outputs (`*.local.*`, `config.local.*`, `.ai-rulez/local/`,
`.ai-rulez/.generated-manifest.local.json`, an MCP config holding secrets) are never excluded; an entry that only
reaches them is ignored with a warning. In a recursive run each root applies its own list to its own outputs.
Excluded paths are not reported by `ai-rulez doctor`.

`generate` warns instead of letting the block quietly win or lose:

- A tracked file matches a line of the block: git keeps tracking it, but new files next to it (a new skill under
  `.claude/skills/`) are ignored and need `git add -f`. The warning names the files and the fix, a `gitignore_exclude`
  entry or `git rm --cached`, and calls out an unanchored `AGENTS.md`/`CLAUDE.md` line that also hides every nested
  root's file.
- A `!pattern` of yours un-ignores a line the block would list: it wins, as documented above, and the warning names the
  negation's `file:line`. A `gitignore_exclude` entry for the path states the intent and silences it.

### `compact`

Controls whether generated inline rule sections omit per-rule `**Priority:**` annotations.

```toml
compact = false  # Default: include Priority lines
# compact = true   # Omit Priority annotations, reduce output size
```

When `true`, generated presets (CLAUDE.md, GEMINI.md, copilot-instructions.md, etc.) omit the per-rule
`**Priority:**` lines, reducing file size for large rule sets. Applies to all configured presets.

### `source`

Where the content is read from.

```toml
source = "ai-rulez"  # Default: the .ai-rulez content tree
# source = "agents"  # AGENTS.md and the .agents directory
```

`"agents"` reads the open standards instead of the `.ai-rulez` content tree: the root `AGENTS.md`, Agent Skills in
`.agents/skills/<name>/SKILL.md`, and the ai-rulez layout under `.agents/` for what the standards lack. ai-rulez
translates that content to every other tool and never writes `AGENTS.md` or anything under `.agents/`. A configuration
kept in `.agents/ai-rulez.toml` implies `source = "agents"`. It forces per-tool files (`agents_md` off) and rejects an explicit `agents_md = true`. See [`.agents` as a source](agents-source.md).

### `agents_md_split`

With the agents source, reads the root `AGENTS.md` as one context item per `##` heading instead of one item.

```toml
source = "agents"
agents_md_split = true  # Default false: AGENTS.md is one item named agents
```

Items are named `agents-<heading slug>`, and `list context` and `remove context` address them. It needs
`source = "agents"` or a configuration in `.agents/ai-rulez.toml`. See
[Splitting AGENTS.md](agents-source.md#splitting-agentsmd).

### `agents_md`

Renders the files several tools share once instead of once per tool. On by default: `AGENTS.md` is the canonical
instruction file and `CLAUDE.md` imports `@AGENTS.md`.

```toml
agents_md = true     # Default: one shared AGENTS.md and .agents/skills
# agents_md = false  # Every preset writes its own files
```

When `true` (the default), always-on rules and context go into one `AGENTS.md` (plus a nested `<scope>/AGENTS.md` for each
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
  Set [`[claude] claude_md = "none"`](#claude_md) to drop the `CLAUDE.md` shim and leave `AGENTS.md` as Claude's only file.
- Skills with `targets`, machine-local files and agents, commands and MCP files stay per-preset.
- If another preset (including a custom provider) also writes `AGENTS.md` or a file under `.agents/skills`, the
  shared output wins.

Tool support, per-preset file lists, the duplication trade-off, `targets`, scopes and the on/off behavior are in
[AGENTS.md and .agents/skills](agents-md.md).

### `skip_outputs`

Output kinds ai-rulez never writes, overwrites or deletes, so a project can hand-write them and still let ai-rulez emit
the rest (installed skills, plugins, MCP servers, hooks, permissions). The default is empty: nothing is skipped.

```toml
skip_outputs = ["instructions"]
```

| Value          | Skips |
| -------------- | ----- |
| `instructions` | Every root instruction file: `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` and the other presets' root files, the `*.local.md` and `AGENTS.override.md` variants, and scope roots. Overrides `agents_md` and `claude_md`. |
| `agents_md`    | Only the shared `AGENTS.md` (and `AGENTS.override.md`) and the shared `.agents/skills` tree. |
| `claude_md`    | Only `CLAUDE.md` (root and scopes). |
| `rules`        | Rule files in every preset (`.claude/rules`, `.cursor/rules`, ...). |
| `skills`       | Skill directories in every preset, installed skills included. A command a harness runs as a skill (`.claude/skills/<command>`, `.agents/skills/<command>`) is a command, not a skill. |
| `agents`       | Agent definition files. |
| `commands`     | Command and prompt files, and the skills that carry a command (including `agents/openai.yaml` for Codex). |

An unknown value fails validation and names the valid ones. `config.local.toml` accepts the key too. Skipped files leave
the manifest, the managed `.gitignore` block, `generate --check`, the lock output digests and `clean`, so none of them
reads or removes a hand-written file. A file an earlier run generated and that is now skipped is left in place and
reported once as no longer managed. See [Managing only remote skills and plugins](managing-remote-skills.md).

### `claude_md`

Controls whether the `claude` preset writes its `CLAUDE.md` when `agents_md` is on. The default, `"shim"`, keeps the
`CLAUDE.md` that imports `@AGENTS.md`; `"none"` omits it and writes only `AGENTS.md`. It is a key of the `[claude]`
table, alongside `[claude.settings]` and `[claude.skills]`.

```toml
[claude]
claude_md = "shim"  # Default: CLAUDE.md imports @AGENTS.md
# claude_md = "none"  # Write only AGENTS.md; no CLAUDE.md at the root or in a scope
```

Claude Code reads `AGENTS.md` itself from **2.1.277** on, so `"none"` lets a repository use `AGENTS.md` as its only
instruction file. With the option set, `generate` writes `AGENTS.md` (root and each `[[scopes]]` directory) and no
`CLAUDE.md`, and it removes a `CLAUDE.md` a previous run generated — even when the generated manifest is gone, because
the shim's own `Content-Hash` header proves ai-rulez wrote it. A hand-authored `CLAUDE.md` carries no such header and is
left alone. `generate --check` reports a leftover generated shim as an `orphan`, and `clean` removes it.

Shadowing caveats: Claude Code reads `AGENTS.md` only when no `CLAUDE.md`, `.claude/CLAUDE.md` or `CLAUDE.local.md`
exists in the working directory or any ancestor. A hand-authored `CLAUDE.md` anywhere on that path, or a
`CLAUDE.local.md` (which ai-rulez writes for machine-local content under `.ai-rulez/local/`), therefore shadows
`AGENTS.md` and makes the option ineffective for that tree. Remove those files, or do not use machine-local content,
if `AGENTS.md` must be the only instruction file. Nested `AGENTS.md` files are read lazily, when Claude Code works
inside that directory.

### `codex_skills_dir`

Where the `codex` preset writes skills, relative to the project root. The default is `.agents/skills`, the
directory Codex documents for repository skills (it scans `.agents/skills` from the working directory up to the
repository root; `.codex/skills` is not read). Set it to `.codex/skills` to write there instead:

```toml
codex_skills_dir = ".codex/skills"  # Default: ".agents/skills"
```

After the default changes, `generate` writes the new tree and removes the files it previously wrote to
`.codex/skills` (only manifest-tracked files; hand-written skills stay). The path must stay inside the project.
With `agents_md` on (the default), skills are written once to `.agents/skills` whatever this is set to.

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
self_server = true                # default; add ai-rulez's own MCP server to .mcp.json
# self_server_version = "5.0.0"   # default: latest, so a project picks up new releases
# self_server_launcher = "binary-then-npx"  # use an installed ai-rulez, else npx
# self_server_command = ["ai-rulez", "mcp"]  # replace the whole launch command instead of npx
# skills_instructions = false                # leave the "Unlisted skills" section out of instruction files
# skills_instructions_text = "..."           # replace that section; {{server}}, {{skills_dir}}
# serve_skills = true                       # a bare `ai-rulez mcp` serves skills, as --serve-skills does
# discover_skills = "repo"                   # mcp --serve-skills also serves the skills already in the repository
# index_cache = "user"                       # "user" (default), "repo" or "off": where the repository index is cached
# read_only = true                           # mcp registers only its read-only tools (--read-only)
# tools = ["find_skill", "load_skill"]       # mcp registers only these tools (--tools)
# allow = ["review-*"]                       # mcp --serve-skills serves only matching skills (--allow)
# deny = ["draft-*"]                         # mcp --serve-skills never serves matching skills (--deny)
# daemon = true                              # mcp --serve-skills shares one server per repository (--daemon)
# no_watch = true                            # mcp --serve-skills does not reload on file changes (--no-watch)
# daemon_idle_timeout = "15m"                # the shared daemon exits this long after its last client
# authoring = true                           # keep the authoring tools on a read-only skills server
# serve_user_invoked_skills = true           # also serve skills that set disable-model-invocation (left out by default)
# exclude = ["third_party/**"]               # globs of paths repository discovery never reads (--exclude adds to them)
```

| Field                      | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `self_server`              | When `true` (the **default in v5**), `generate` adds `"ai-rulez": {"type": "stdio", "command": "npx", "args": ["-y", "ai-rulez@latest", "mcp"]}` to the project `.mcp.json`, so a project is MCP-ready out of the box. When the project serves skills (a `delivery` other than `static`, a `[[skill_sources]]` entry or a role delivery) the entry runs `mcp --serve-skills` instead: one process serves the skills and the authoring tools, so no separate skills server is needed. Set `false` (or pass `generate --no-self-mcp`) to opt out. |
| `self_server_version`      | Version (or dist-tag) to pin. Defaults to `latest`, so a project picks up new releases without regenerating. Only applies while the self server is on.                                                                                                                                                                                                                                                                                                                                                                                          |
| `self_server_launcher`     | `npx` (the default) or `binary-then-npx`: run an installed `ai-rulez` when one is on `PATH`, otherwise npx (pinned by `self_server_version`). Generated as a `sh -c` script with `--serve-skills` passed as an argument. Mutually exclusive with `self_server_command`. |
| `self_server_command`      | Executable followed by its arguments, replacing the npx launch. When the project serves skills, `--serve-skills` is appended unless it is already there; a shell launch (`sh -c <script>`) is never appended to, because the flag would become `$0`, so put `--serve-skills` in the script and end the command with a dummy `"sh"` argument (`exec ai-rulez mcp "$@"`). Mutually exclusive with `self_server_version`. Only applies while the self server is on.                                                                                                                                                                                                                                                                                               |
| `skills_instructions`      | While skills are served, every instruction file gets a short "Unlisted skills" section telling the agent to call `find_skill`, then `load_skill`, on the MCP server that serves them, with the on-disk fallback (`.ai-rulez/skills/<name>/SKILL.md`). Default `true`; `false` leaves it out. Written when a configured preset can call MCP, and only into the root file of a harness that can; the shared `AGENTS.md` carries it once.                                                                                                          |
| `skills_instructions_text` | Replaces the default section (write your own heading). `{{server}}` is the MCP server that serves the skills, `{{skills_dir}}` the config directory's skills folder (`.ai-rulez/skills`). Unknown variables, blank text, and text beside `skills_instructions = false` are configuration errors.                                                                                                                                                                                                                                                |
| `serve_skills`             | `true` makes a bare `ai-rulez mcp` behave as `mcp --serve-skills`, so a client entry needs no flag (default `false`). The flag overrides it: `--serve-skills=false` turns it off. `generate` still writes `--serve-skills` into the self-server entry when the project serves skills; the flag and the key agree. Skills-only keys (`discover_skills`, `allow`, `deny`, `exclude`, `no_watch`, `daemon`) that are set while skills are not served are ignored, and the server logs one warning naming them at startup. |
| `discover_skills`          | `off` (the default), `repo` or `all`: whether `mcp --serve-skills` also serves the skills the repository already keeps (`skills/`, `.claude/skills` at any depth, the configured presets' skill directories). `all` adds the installed and included skills whatever their delivery. The `--discover-skills` flag overrides it. See [Discovering skills already in the repository](mcp-server.md#discovering-skills-already-in-the-repository). |
| `exclude`                  | Globs, relative to the project root, of the paths repository discovery never reads: skill directories and the context index (`CLAUDE.md`, `AGENTS.md`, rules, agents) alike. `**` crosses directories, a pattern without a slash matches at any depth. The `mcp --exclude` flag (repeatable) adds to the list. See [Excluding paths](mcp-server.md#excluding-paths). |
| `index_cache`              | `user` (the default), `repo` or `off`: where the repository index of the discovery keeps its cache of item summaries and scan verdicts, never file bodies. `user` is the user cache directory, with a gitignored directory in the project (`.ai-rulez/.cache/`, or `.ai-rulez-cache/` in the root layout) as the fallback when it is not writable; `repo` always uses that directory; `off` keeps none. See [The index cache](mcp-server.md#the-index-cache). |
| `read_only`                | `true` registers only the read-only tools, as `mcp --read-only` does; the server instructions and prompts follow. The `--read-only` flag overrides it. An organization policy with `[mcp] read_only = true` cannot be loosened by it (`AR740`). |
| `tools`                    | The only tools the server registers, as `mcp --tools` does, for example `["find_skill", "load_skill", "find_context", "load_context"]`. An unknown name stops the server and lists the valid ones. The `--tools` flag overrides it. |
| `allow`, `deny`            | Glob patterns on the served skill names (`mcp --serve-skills --allow` and `--deny`); `deny` wins. The flags override them. |
| `daemon_idle_timeout`      | How long the shared daemon outlives its last client, as a Go duration (`"2m"`); default 15 minutes. The daemon also exits when its repository directory is removed. |
| `authoring`                | `true` keeps the authoring tools on a read-only skills server. Without it, `read_only` with `serve_skills` registers only `find_skill`, `load_skill`, `search_skills`, `read_skill_file` and `list_skill_resources`, a few KB of schema instead of about 92 KB. `tools` wins over both. |
| `serve_user_invoked_skills` | `true` serves skills that set `disable-model-invocation: true` to the model too. By default `find_skill`, `search_skills` and the `skill://` resources leave them out and `load_skill` refuses them (`<name> is user-invoked only`); on a server that also carries the authoring tools, `read_skill`, `list_skills`, `catalog` and the `ai-rulez://` resources leave them out too. The plain `ai-rulez mcp` server is not affected. A running `--daemon` reads the setting when it starts. |
| `daemon`, `no_watch`       | `mcp --serve-skills --daemon` (one shared server per repository) and `--no-watch` (no live reload). The flags override them. |

The `mcp` serve keys are read when the server starts. See [Settings in the configuration](mcp-server.md#settings-in-the-configuration).

Behavior:

- **On by default**: a project gets the `ai-rulez` MCP server without declaring anything;
  `self_server = false` or `generate --no-self-mcp` turns it off.
- **Merged, not replaced**: with `self_server` and no `[[mcp_servers]]`, ai-rulez owns only the
  `mcpServers.ai-rulez` entry. Other servers in an existing `.mcp.json` (and every other key)
  are preserved, so running `generate` twice produces no diff.
- **Every MCP writer agrees**: the root `.mcp.json` is rendered identically by the `mcp` preset and
  the cursor/copilot writers, and the `antigravity` preset adds the same entry to
  `.agents/mcp_config.json`.
- **With `[[mcp_servers]]`**: declared servers keep their existing behavior (ai-rulez owns the whole
  `mcpServers` object, see [Settings document merge behavior](#settings-document-merge-behavior)),
  and the ai-rulez entry is added to it unless you declare a server named `ai-rulez` yourself, which
  wins.
- **Per root**: each config root decides independently, including under `generate --recursive`.
- The pinned version changes when you upgrade ai-rulez, so the generated `.mcp.json` changes with it.
- **Turning it off**: `self_server = false` (or `generate --no-self-mcp`) stops ai-rulez from owning
  the entry but does not delete it; remove `mcpServers.ai-rulez` from `.mcp.json` yourself.
  `ai-rulez clean` likewise leaves a `.mcp.json` that holds hand-authored servers in place.

### Keeping directories out of `generate --recursive`

`generate --recursive` (and `validate --recursive`) searches every directory below the working directory for
configuration, except build output, dependency and hidden directories and `testdata`. A repository that keeps
deliberately invalid configurations elsewhere (test fixtures, golden corpora) lists them in the configuration of the
directory the run starts from (`.ai-rulez/config.toml`, `.agents/ai-rulez.toml` or `.config/ai-rulez/config.toml`):

```toml
recursive_exclude = ["tests/fixtures", "svc/*/gen"]   # directories or globs, relative to the project root
```

`generate --recursive --exclude <dir>` adds to the list for one run.

### `mcp_servers`

Inline MCP (Model Context Protocol) server definitions. A separate `mcp.yaml`, `mcp.toml` or `mcp.json` is no longer read.

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
| `type`        | No       | The harness's `type` value (`stdio`, `http`, or `sse`), an alias of `transport`. When both are set they must agree; an unset `type` falls back to `transport`. |
| `url`         | No       | Remote MCP server URL for `http` or `sse` transports.                               |
| `headers`     | No       | HTTP headers for `http` or `sse` servers (e.g. auth). Values may contain `${VAR}` placeholders. |
| `oauth`       | No       | Pre-registered OAuth client of a hosted (`http`/`sse`) server: `client_id`, `callback_port` (1–65535) and `scopes`. Rendered only by harnesses whose MCP format has an `oauth` block (Claude Code's `.mcp.json`, VS Code's `.vscode/mcp.json`, Codex's `.codex/config.toml`); a harness that cannot express it skips the server with a warning. |
| `enabled`     | No       | Set to `false` to skip the server in generated MCP outputs. Defaults to `true`.     |
| `profiles`    | No       | Restrict the server to the named profiles. Omit to include it in every profile.     |
| `domain`      | No       | Restrict the server to a domain: it is included only when the active profile or role selects that domain. Omit to include it in every selection. |

Local `stdio` servers normally need `command`; remote `http` and `sse` servers normally use `url`,
plus `headers` when the server needs auth:

```toml
[[mcp_servers]]
name = "remote-api"
transport = "http"
url = "https://mcp.example.com/mcp"
headers = { Authorization = "Bearer ${REMOTE_API_TOKEN}", X-Team = "platform" }
```

A hosted server that needs a pre-registered OAuth client (Slack, for example) carries an `oauth` block. The
`type` key is the same setting as `transport`, spelled the way a harness's file spells it:

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

`oauth` is written as `{ "clientId": ..., "callbackPort": ..., "scopes": [...] }` in Claude Code's `.mcp.json`
and VS Code's `.vscode/mcp.json`, and as an `[mcp_servers.<name>.oauth]` table with `client_id` and
`callback_port` (plus a sibling `scopes` array) in Codex's `.codex/config.toml`. A harness whose MCP format
has no `oauth` block (Gemini, OpenCode, Zed, ...) leaves the server out of its file and warns, rather than
writing a server that could not authenticate. `oauth` requires a remote transport; a `client_id` is not
required by `validate` (a callback-port-only block relies on dynamic client registration), but `scan` reports
one that is missing as `AR602`.

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
patterns `ai-rulez` is about to add. The check looks at what is rendered: an MCP config file
(`.mcp.json`, `.gemini/settings.json`, `.agents/mcp_config.json`, `opencode.json`, a scoped variant such as
`packages/web/.mcp.json`, ...) is protected only when it holds a resolved secret value. A file that only refers to the
variable (`${env:TOKEN}`) and a merged document that is not an MCP config (a review check file) need no ignore entry.
Resolved secret values are redacted before source-hash calculation, but generated MCP config files contain the actual
resolved values. Claude Code reads project servers from `.mcp.json`, which references `${VAR}`; `.claude/settings.json`
carries no `mcpServers`, and a `mcpServers` entry an earlier version wrote there is removed on the next `generate`.

**Environment references.** Where a tool expands environment references in its own MCP config, a value that
came from a `${VAR}` placeholder resolved from the process environment is written as that tool's reference
instead of the secret. A value from `--env`, `--env-file`, `.env`, `${PROJECT_ROOT}` or an unresolved lenient
placeholder is always written resolved, and a file that holds only references is not made owner-only.

| Tool (preset) | Reference | Where | Documentation |
| --- | --- | --- | --- |
| Claude Code and the readers of the root `.mcp.json` (`mcp`, `codebuddy`, `commandcode`, `reasonix`, `cursor`, `copilot`) | `${VAR}` | `.mcp.json` env and headers | <https://code.claude.com/docs/en/mcp> |
| Gemini CLI (`gemini`) | `${VAR}` | `.gemini/settings.json` env and headers | <https://geminicli.com/docs/tools/mcp-server/> |
| Amp (`amp`) | `${VAR}` | `.amp/settings.json` env and headers | <https://ampcode.com/docs/customize/mcp> |
| Pi (`pi`) | `${VAR}` | `.pi/mcp.json` env and headers | <https://pi.dev/docs/latest/mcp> |
| Factory Droid (`factory`) | `${VAR}` | `.factory/mcp.json` env and headers (not `url`, `args`) | <https://docs.factory.com/cli/configuration/mcp> |
| OpenCode (`opencode`), Kilo (`kilo`) | `{env:VAR}` | `environment` and `headers` | <https://opencode.ai/docs/config/> |
| Cursor (`cursor`), Devin (`devin`) | `${env:VAR}` | `.cursor/mcp.json`, `.devin/mcp_config.json` | <https://docs.devin.ai/desktop/cascade/mcp> |
| Codex (`codex`) | `env_vars`, `env_http_headers`, `bearer_token_env_var` | `~/.codex/config.toml` / `.codex/config.toml` | Codex configuration reference |
| Codebuff (`codebuff`), Crush (`crush`) | `$VAR` (whole value only) | env and headers | tool documentation |

A reference with a delimiter (`${VAR}`, `${env:VAR}`, `{env:VAR}`) may sit inside a longer value
(`Bearer ${TOKEN}`); `$VAR` is written only when the value is exactly one placeholder, because `$TOKEN_v2` would
read another variable.

The root `.mcp.json` is one file shared by several presets, so every writer renders the same bytes. It carries
`${VAR}` references only for upper-case names (CodeBuddy expands no others). `qoder` documents no expansion, so it
writes the resolved value (owner-only). Alone, or with presets that do not read `.mcp.json`, the whole file stays
resolved. Together with a preset that expands references (`claude`, `cursor`, `copilot`, `codebuddy`, `commandcode`,
`reasonix`), the two renderings differ and `generate` fails naming both presets, rather than putting a resolved secret
into a file a reference-reading tool shares; supply the secret with `--env` or `.env` to use both. Tools whose documentation
promises no expansion (`junie`, `antigravity`, `zed`, `trae`, `cline`, `xum`, Claude Desktop, VS Code's `.vscode/mcp.json`)
and tools that expand only after an opt-in (`kiro`: "Mcp Approved Env Vars") keep the resolved value.

#### Settings document merge behavior

Files such as `.claude/settings.json`, `.mcp.json`, `.amp/settings.json`, `.gemini/settings.json`,
`.agents/mcp_config.json` and `.pi/mcp.json` are **shared documents**: ai-rulez owns specific top-level keys
(`mcpServers` for MCP config, `amp.anthropic.effort` for Amp) and the consumer owns everything else.
Generation replaces only the owned keys and preserves every other member byte-for-byte, including
the document's original indentation. A hand-formatted document (a one-line nested object, for example) is edited in
place, so `clean` restores its original bytes. The manifest records what ai-rulez merged as digests, never as values,
so a resolved secret in a merged server entry is not copied into `.generated-manifest.local.json`.

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
- **JSONC**: A document with comments or trailing commas is merged in place: ai-rulez adds or updates
  only the keys it owns (the MCP servers, the Gemini `context.fileName` or OpenCode `instructions`
  entry) and every comment and the rest of the document stay as written.
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
config. It is distinct from the **producer** `[plugin]` block
(see [Authoring Plugins](plugins.md)) that packages this project as a plugin.

```toml
[[plugins]]
marketplace = "official"   # name of a [[marketplaces]] entry
name = "my-plugin"
scope = "project"          # project or user; defaults to project
enabled = true             # defaults to true
```

!!! warning "No effect"
    `[[plugins]]` no longer writes `.claude/plugins.json` or `.codex/plugins.json`: Claude Code and Codex do not
    read them. The table is still accepted, and `generate` warns when it is set. Claude Code records installs in
    `.claude/settings.json` (`enabledPlugins`, `extraKnownMarketplaces`); Codex enables plugins with
    `[plugins."name@marketplace"] enabled = true` in `.codex/config.toml`. Use
    [`[claude.settings]`](#claudesettings) (`manage = true` with `enable_plugins`) for Claude Code, and keep the
    Codex entry yourself; ai-rulez does not write `.codex/config.toml` plugin entries because it owns that file
    outright. Delete the files an earlier version left behind. The Copilot
    `enabledPlugins` settings syntax is not shown in the vendor documentation, so it is not generated.

### `plugin`

The **producer** block: packaging metadata for a distributable plugin bundle (`generate --plugin`, `publish`,
`--emit agent-plugins`). Skills, commands and agents come from the content tree; this block only names and describes
the package. `name` and `version` are required. See [Authoring Plugins](plugins.md) and
[Agent Plugins](agent-plugins.md).

```toml
[plugin]
name = "acme.tools"            # required
version = "1.2.0"              # required, semantic version
description = "Acme review tooling."
runtimes = ["agent-plugins"]   # default: all runtimes
spec = "1.1.0"                 # Agent Plugins version: "1.0.0" (default) or "1.1.0"
```

| Key | Default | Meaning |
| --- | --- | --- |
| `name`, `version` | required | Package name and semantic version |
| `display_name`, `description`, `homepage`, `repository`, `license`, `category`, `brand_color`, `icon`, `logo` | empty | Manifest metadata |
| `keywords`, `tags` | empty | Search terms |
| `runtimes` | all | Runtimes to build: `claude`, `cursor`, `codex`, `gemini`, `kimi`, `opencode`, `factory`, `hermes`, `agent-plugins`, `copilot` |
| `spec` | `1.0.0` | Agent Plugins specification of the `agent-plugins`, `copilot` and root-layout `codex` packages: `1.0.0` or `1.1.0` |
| `content_root` | content tree | Project-relative directory holding plugin-only `skills/`, `commands/` and `agents/` |
| `include_domains` | none | Domains (names or globs) whose skills, commands and agents are bundled; a root item wins over a same-named domain item |
| `include_evals` | `false` | Bundle each skill's `evals/` directory and the project-level `evals/` tree |
| `skills_dir_owned` | `false` | Declare the bundle's skills directory fully owned: `verify --plugin` reports entries it did not generate as `extra` and `generate --plugin` removes them |

To publish several plugins from one root, use `[[plugins_authoring]]` instead of `[plugin]` (and
`[[marketplaces_authoring]]` for several marketplace indexes); see
[Several plugins from one root](plugins.md#several-plugins-from-one-root). A definition file
`.ai-rulez/plugins/<name>.toml` or `.ai-rulez/marketplaces/<name>.toml` holds one entry's keys.

Sub-tables: `[plugin.author]` (`name`, `email`, `url`), `[[plugin.mcp]]`, `[[plugin.hooks]]`, `[plugin.statusline]`,
`[plugin.codex]` (`manifest` = `legacy`, `root` or `both`; `marketplace`), `[plugin.cursor]`, `[plugin.gemini]`,
`[plugin.kimi]`, `[plugin.hermes]` and `[plugin.interface]`.

#### `plugin.interface`

The rich UI block of the Codex and Kimi manifests (the Codex root layout writes it under
`extensions["com.openai"].interface`, keys sorted). No key is required and unknown keys are rejected.

| Key | Type |
| --- | --- |
| `display_name`, `short_description`, `long_description` | string |
| `developer_name`, `category` | string |
| `capabilities`, `default_prompt`, `screenshots` | list of strings |
| `website_url`, `privacy_policy_url`, `terms_of_service_url` | string |
| `brand_color`, `composer_icon`, `logo`, `logo_dark` | string |

### `llms_txt`

Controls the `llms.txt` written by the opt-in `llms-txt` preset and linted by `validate --strict` (`AR9P0` to
`AR9P6`). The table has no effect without the preset. See [llms.txt](llms-txt.md).

```toml
presets = ["llms-txt"]

[llms_txt]
dir = ""                                     # default: the project root
title = "Acme"                               # default: name
summary = "Acme coding rules."               # default: description
full = true                                  # also write llms-full.txt (default: false)
include = ["rules", "context", "skills"]     # default; also agents, commands
```

`agents` and `commands` are listed in the `Optional` section. `dir` must not be inside `.git` or the configuration
directory.

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
enable_plugins = ["acme-essentials"]    # or a qualified "name@marketplace" of another marketplace
```

### `enabled_plugins`

Enables a plugin from another marketplace for the `claude`, `codex` and `opencode` presets from one entry:
`.claude/settings.json` gets `enabledPlugins` (and `extraKnownMarketplaces` with `marketplace`), `.codex/config.toml`
a `[plugins."<id>"]` table and `opencode.json` the `opencode` npm package in `plugin`. Merged like the other
settings documents (see [Settings document merge behavior](#settings-document-merge-behavior)). See
[Enable a plugin from another marketplace](plugins.md#enable-a-plugin-from-another-marketplace).

```toml
[[enabled_plugins]]
id = "basemind@basemind"
marketplace = { name = "basemind", source = "Goldziher/basemind", type = "github" }
opencode = "basemind-opencode"
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

Top-level `[[hooks]]` (lifecycle hooks for 36 harnesses, from `claude`, `codex`, `cursor`, `gemini` and `copilot` to
`qwen`, `kiro` and the plugin-based `opencode`, `kilo`, `pi` and `amp`) and
`[permissions]` (`allow`, `ask`, `deny` rules, translated for 24 harnesses with a native permission
surface) are rendered into each harness's native settings file outside any plugin, merged key by key so
hand-authored content survives, comments in JSONC, TOML and YAML included. See
[Hooks and permissions](settings.md), [Permissions](permissions.md) and the per-harness
[feature matrix](harnesses.md).

`ai-rulez generate --user` renders a user config into the per-user directories each harness reads; see
[User-level configuration](user-scope.md).

### `session_start`

`[session_start]` is on by default for a project that generates for Claude Code: `generate` adds a `SessionStart` hook
to `.claude/settings.json` that runs `ai-rulez session-start`. In a remote (cloud) session, where `CLAUDE_CODE_REMOTE`
is `true`, it regenerates the outputs of the fresh clone and injects a short briefing; in a local session it does
nothing. Keys: `generate` (false removes it), `when` (`remote` or `always`), `mode` (`auto`, `locked`, `frozen`,
`plain`), `timeout`, `version`, `command`, `prompt` and `prompt_text`. See
[Run generate when a remote session starts](session-start.md).

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

An unknown positive entry (a typo such as `builtins = ["rsut"]`) fails `validate` and the load with the valid
choices; unknown `!exclusions` stay warnings.

#### Scope a builtin to a profile

A builtin pack can be named in a profile's domain list with the `builtin:` prefix. The pack
is then loaded for that profile only, instead of every profile:

```toml
[profiles]
backend = ["backend", "builtin:docker"]
frontend = ["frontend"]
```

`backend` sees the `docker` pack; `frontend` does not. `builtin:docker/container-standards` scopes just that item. This works even when the root
`builtins` field is absent or set to `false`, because a profile reference is an explicit
opt-in. A pack the root `builtins` field already enables stays global — the prefix selects
a pack, it does not un-globalize one. Prefixing also keeps a builtin (`builtin:rust`) from
colliding with a local domain of the same name.

#### Precedence over a builtin skill

A builtin ranks below every skill the project has. A skill under `.ai-rulez/skills/` replaces the builtin of the same
name; so does a hand-written repository skill such as `.claude/skills/testing-conventions` when
`mcp --serve-skills --discover-skills` serves it. `validate` reports either case as `AR703` (silence it with
`lint.allow_overrides = ["testing-conventions"]`); `generate` logs the same collapse as a `Duplicate skill collapsed`
warning and honours the same list. `generate` never overwrites the hand-written file: it stops and names
the file, so the project skill is not clobbered by the generated builtin; to use the generated builtin, remove the
hand-written copy, otherwise exclude the builtin item (`!<domain>/<item>`).

Skills and commands share one output namespace (`.claude/skills/{id}/SKILL.md`), so the same holds across the two kinds:
a project skill replaces a builtin command of the same id, and a project command replaces a builtin skill. The builtin
is dropped, `generate` logs `Duplicate command collapsed` (or `skill`) and `validate` reports `AR703`, both silenced by
`lint.allow_overrides`. A skill and a command that are both project items, or both builtins, with one id remain an error.

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

#### Exclude specific items from a builtin domain

When using the array form of `builtins`, you can exclude a single item from a domain while keeping the rest:

```toml
builtins = ["git-workflow", "!git-workflow/commit-messages"]
```

This loads everything from `git-workflow` except the `commit-messages` rule. The syntax is `!domain/name`, and
`name` matches an item of any type (rule, skill, agent, command or context file) by its bare name;
`ai-rulez builtins show <domain>` lists the names. The exclusion also applies to a pack loaded through a
`builtin:<domain>` profile reference. Per-item selection only works with the array form; `builtins = true` is a
boolean, so use the array form with explicit pack names instead.

#### Select a single item from a builtin domain

A positive `domain/name` loads only that item of the pack:

```toml
builtins = ["!ai-governance", "!agent-delegation", "!security", "!git-workflow", "!code-quality",
            "!testing", "!token-efficiency", "docker/container-standards"]
```

This emits the `container-standards` skill and nothing else; `docker` is not loaded as a pack. The rules, in order:

- `domain/name` loads only that item of a pack that is not otherwise loaded whole.
- `!domain` removes the pack, and a `domain/name` entry still applies afterwards, in either order:
  `["security/secrets-handling", "!security"]` loads only `secrets-handling`.
- `domain` wins over `domain/name`: the whole pack loads. An auto-included pack (`security`) loads whole unless it is
  excluded with `!domain`.
- `!domain/name` always removes the item, whichever entry selected it.
- A positive entry that names no pack or item (`builtins = ["nope"]`, `"docker/container-standard"`) fails
  `validate` with the valid choices and a did-you-mean. An unknown *exclusion* stays a warning.

In a profile, `builtin:<domain>/<name>` scopes one item the same way (`default = ["builtin:docker/container-standards"]`);
`builtin:<domain>` wins over item references to the same pack. A pack the root `builtins` field selects, even item by
item, stays global and wins over a profile reference to the same pack.

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

- `ai-governance` — rules: read-before-write, minimal changes, verification before completion, systematic debugging, agent workflow, generated files, communication style, no AI signatures, explain reasoning. Skills: `config-placement`, `skill-authoring`
- `security` — rules: input validation, secrets handling, least privilege, customer data boundary. Skills: `owasp-quick-reference`, `dependency-awareness` (with per-language audit tool recommendations)
- `git-workflow` — rules: conventional commits, atomic commits, branch hygiene, safe operations
- `code-quality` — skills only: `code-quality-standards` (readability, complexity limits, anti-patterns, duplication, dead code, suppressions, markers), `error-handling`
- `testing` — rule: tests ship with the behaviour change. Skills: `tdd-workflow`, `testing-conventions` (naming, assertions, independence, anti-patterns)
- `token-efficiency` — rules: context preservation, output awareness. Skills: `task-runner`, `incremental-approach`
- `agent-delegation` — context: delegation instructions and the generated agent listing
- `cicd` — rules: pipeline standards, GitHub workflow conventions
- `docker` — skill only: `container-standards`
- `observability` — skill only: `observability-standards`
- `documentation` — rules: inline docs, README standards, docs-with-code updates, ADR immutability. Skill: `adr-workflow`
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
- **`ai-governance`**: the `config-placement` and `skill-authoring` entries
- **`documentation`**: the `adr-workflow` entry
- **`security`**: the `owasp-quick-reference` and `dependency-awareness` entries
- **`code-quality`**, entirely: `code-quality-standards` and `error-handling`
- **`testing`**, all but one rule: `tdd-workflow` and `testing-conventions`
- **`token-efficiency`**: `task-runner` and `incremental-approach`
- **`docker`** and **`observability`**, entirely: `container-standards` and `observability-standards`

What stays inline is behavioural governance that has to land before the first file is read, where a skill
loaded after the fact is too late: the rules of `ai-governance`, all of `git-workflow`, `security`'s
`secrets-handling`, `input-validation`, `least-privilege` and `customer-data-boundary`, `token-efficiency`'s `context-preservation`
and `output-awareness`, and `testing`'s `test-alongside-code`. Guidance that only matters once you are
already inside a specific activity — writing a test, writing an error path, writing a Dockerfile — is a
skill, because its description can name that trigger and its body then costs nothing until it fires.

Exclusions still work exactly the same for these now-skill entries. `!domain` drops a whole domain and
`!domain/name` drops a single item within it (array form of `builtins` only):

```toml
builtins = ["rust", "security", "!security/owasp-quick-reference"]
```

#### Merge Priority

Builtins have the **lowest** priority. Content is merged in this order:

1. **Builtins** (lowest) — embedded in binary
2. **Includes** — from git repos or local paths
3. **Project content** (highest) — in your `.ai-rulez/` directory (not to be confused with
   machine-local content under `.ai-rulez/local/`, see [Local overlay](#local-overlay))

If a local domain has the same name as a builtin, the local domain is used and the builtin is skipped entirely. The builtin pack is not merged into the
domain, so its other items are lost; `generate` and `validate` log a `Builtin pack skipped` warning once per pack that names
them. To get a pack item, define it in your own domain or rename the domain.

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

1. Per-agent `effort` in agent frontmatter (Claude, Codex, Devin, Opencode, Xum, pi — presets that support per-agent effort)
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

- **`rules.mode`**: `split` (the default) writes one file per rule in the tool's native rules folder; `inline` embeds rules in the root file, moving only path-scoped rules to the folder.
- **`rules.mode_by_preset`**: per-preset override that beats `rules.mode`. Keys are built-in, custom, or provider preset names.
- **`rules.scoped_fallback`**: how the shared `AGENTS.md` (`agents_md = true`) serves glob-scoped rules and context to presets without a rules folder (`codex`, `opencode`, `pi`, ...). `inline` (the default) inlines them with an `_Applies to:_` line, so every preset that reads or imports `AGENTS.md` loads them at start; `skill` writes each one to `.agents/skills` as a skill gated by its `paths`. See [Rules](rules.md#glob-scoped-rules-and-agentsmd).
- **`rules.scoped_fallback_group`**: with `scoped_fallback = "skill"`, folds the fallback skills so the skill listing stays short. `none` (the default) writes one skill per item; `include` writes one `scoped-<include>` skill per include or builtin pack; `directory` writes one `scoped-<dir>` skill per directory the globs point into. A grouped skill's `paths` are the union of its items' globs. A rule or context item with `fallback: false` in its frontmatter stays out of the fallback. See [Keeping the skill listing short](rules.md#keeping-the-skill-listing-short).
- **`rules.baz_scoped`**: `nested` (the default) writes the path-scoped rules and context of the `baz` preset to the `AGENTS.md` of the directory their globs point into; `root` keeps them in the root `AGENTS.md`. See [Baz](baz.md).
- **`rules.baz_default`**: `all` (the default) nests every path-scoped item into the Baz directory files; `none` nests only items whose `targets` select `baz`, leaving the rest in the root `AGENTS.md`.
- **`rules.baz.include`**: with `baz_default = "none"`, the rule and context names (list) that are nested for Baz in addition to the items whose `targets` select `baz`; `baz: true` in an item's frontmatter does the same. A `!baz` entry in `targets` always keeps an item out of the Baz nested files. See [Baz](baz.md#per-rule-opt-in-and-opt-out).
- **`rules.baz_rest`**: `root` (the default) leaves a glob-activated item that baz does not nest (unselected under `baz_default = "none"`, or excluded by `!baz`) inlined in the root `AGENTS.md`; `skip` leaves it out of that file when `baz` is the only preset writing it, so an opt-in baz surface stays small. See [Baz](baz.md).
- **`rules.baz_unscoped`**: `root` (the default) inlines a glob-activated item that cannot be nested (`**/*.go`, `**/BUILD`, `.mcp.json`) into the root `AGENTS.md`; `skip` leaves it out of that file when `baz` is the only preset writing it. See [Baz](baz.md#path-scoped-rules-with-no-directory).
- **`rules.baz_dedupe`**: `true` writes an item whose globs point into several sibling directories under one parent once, to that parent's `AGENTS.md`, instead of one copy per directory. Default `false`.
- **`rules.baz_dedupe_max`**: when above `0`, an item that would be copied into more than that many directories is written once to the directory covering all of them, up to the repository root (the root `AGENTS.md`). Works without `baz_dedupe`; with `baz_unscoped = "skip"` the root fallback is not used and the copies stay. Default `0` (off).
- **`rules.baz_budget`**: byte budget of the Baz `AGENTS.md` surface (root plus nested); `generate` warns when it is exceeded, naming the largest files. Default `0` (off).

Set `mode = "inline"` to restore the pre-4.22.0 output. Per-tool output, fallbacks and caveats are in [Rules and native rules folders](rules.md).

### `okf`

Settings for the opt-in `okf` preset, which keeps an [OKF](okf.md) bundle in sync with the sources:

```toml
presets = ["claude", "okf"]

[okf]
dir = "docs/okf"                                # bundle directory, relative to the project root (default)
include = ["rules", "context", "skills"]        # kinds exported; default all of rules, context, skills, agents, commands, checks
spec = "0.2"                                    # the only OKF spec version implemented
```

`dir` must stay inside the project and outside the configuration directory and `.git`. The bundle is committed
documentation, so `gitignore` never ignores it, and every file is written without a generated-by banner because
OKF requires frontmatter at the start of each concept.

### `lint`

Tunes `ai-rulez validate`: severities, ignores, allow-lists, description bounds, size budgets
(`[lint.budgets.<kind>]`), tolerated findings per rule (`[lint.ratchet]`, formerly `[lint.budget]` and `[lint.tolerate]`) and required
frontmatter keys.

```toml
[lint]
fail_on = "warning"
analyzers = ["security", "references"]   # run only these analyzers
ignore = ["AR803"]

[lint.severity]
AR401 = "error"

[lint.ratchet]                          # tolerated findings per rule: AR401 may have up to 12
AR401 = 12

[lint.budgets.skill]                     # size budget of a content kind
max_lines = 400

[lint.require_metadata]
skill = ["owner"]
```

Every key, the finding codes and the exit codes are in [Strict validation](strict-validation.md).
An [organization policy](policy.md) bounds `severity`, `ignore` and `security` from outside the repository: a repository can raise them, not lower them.

### `verifiers`

Deterministic, read-only repo checks, run by `ai-rulez verifiers run`. One `[[verifiers]]` table per check; the guide with
semantics and examples is [Verifiers](verifiers.md).

```toml
[[verifiers]]
name = "node-version"
description = "package.json pins Node"
type = "key_equals"
path = "package.json"
key = "engines.node"
equals = ">=20"
severity = "error"
```

| Field | Applies to | Meaning |
| --- | --- | --- |
| `name` | all | Required. Letters, digits, `.`, `_`, `-`; unique. The merge key of the machine-local overlay |
| `type` | all | Required: `file_exists`, `file_absent`, `glob_count`, `regex`, `forbid`, `key_equals`, `generated_in_sync` |
| `description` | all | Free text shown by `list` and in reports |
| `severity` | all | `error` (default), `warning` or `info`. Only errors, and warnings with `--strict`, fail the run |
| `path` | `file_exists`, `file_absent`, `key_equals` | Project-relative path that stays inside the root |
| `glob` | `glob_count`, `regex`, `forbid` | Files to select; `**`, `*`, `?`, `{a,b}` (at most 64 expanded alternatives per glob) |
| `exclude` | `glob_count`, `regex`, `forbid` | Globs removed from the selection |
| `pattern` | `regex`, `forbid` | RE2 expression, matched against the whole file |
| `min`, `max` | `glob_count` | Inclusive, non-negative bounds on the match count; at least one is required |
| `key` | `key_equals` | Dotted path; `\.` is a literal dot, `["a.b"]` quotes a segment, `[0]` or `.0` indexes a list |
| `equals` | `key_equals` | Expected scalar, compared as text |
| `profile` | `generated_in_sync` | A defined profile (or `default`); empty selects the default |

A field that does not apply to the type is a configuration error, as is a glob that does not compile, a `key_equals` path
that is not `.json`, `.yaml`, `.yml` or `.toml`, and a `profile` that is not defined. `config.local.toml` may declare
`[[verifiers]]` too; entries merge by `name`.

### `verifiers_settings`

Limits and policy of `ai-rulez verifiers run` (the name differs from the array because TOML cannot use one key as both).

```toml
[verifiers_settings]
max_timeout_s = 300           # cap on a command predicate's timeout_s (1 to 900)
max_file_bytes = 5242880      # largest file a content predicate reads (default 5 MiB)
require_examples = false      # AR9H6 for a spec verifier without examples
warn_dead = false             # AR9H5 for a when_changed that matches no file, on every run
trust_exec_from = []          # includes whose verifiers may use the command predicate (also pin them in the lock)
command_env = []              # extra environment variable names passed to command predicates
```

Credential-looking and proxy names in `command_env` and unknown includes in `trust_exec_from` are configuration errors.
The table is pinned in the lock. See [Verifiers](verifiers.md#settings).

### `exposure`

Describes the areas of a repository (the directories each one claims, its owner and the words that name it) for the
session exposure checks. The map is inline or in `areas_file`, not both; `validate` reports a map it cannot use as
`AR9W5`, area terms in the text a session sees at its start as `AR9W0` and `AR9W1` (accepted with `[[lint.accept]]`),
and `skill_areas` keys that name no skill as `AR9W6`; `collision_min_score` turns on `AR9W2` for start-listed skills
and agents of different areas whose listings overlap (the 20 highest pairs, then one finding for the rest). `start_budget` caps the estimated tokens a session loads at
start (`AR9W3`), and `context_window` sizes the skill listing budget `AR964` checks. Without the table nothing
changes. See
[Session exposure and areas](exposure.md).

```toml
[exposure]
areas_file = "tools/context/areas.toml"   # [[area]] tables and [skill_areas], relative to the project root
collision_min_score = 0.2                 # AR9W2 at warning; 0 (default) is off
context_window = 200000                   # tokens; the skill listing budget is 1% of it at 4 characters per token
start_budget = 6000                       # AR9W3 above 6000 estimated start tokens; 0 (default) is off
```

### `header`

Configures the style of headers in generated files. Headers provide context about ai-rulez, explain the folder structure, and instruct AI agents on proper usage.

```toml
[header]
style = "minimal"   # Default: bare minimum header
# style = "compact"  # Shorter header with key information
# style = "detailed" # Comprehensive header with full documentation
# timestamp = true  # Emit the "Generated:" line (default: false)
# text = "..."      # Override the generated prose (see "Custom header")
# hashes = "full"    # Freshness lines: "content" (default), "full", or "none"
# mcp_line = false  # Drop the "Use MCP server: ..." line (default: true)
# nested = "compact" # Header of a root inside another root: "compact" (default), "omit" or "inherit"
```

The default is `minimal`. Every style — including `minimal` — carries the "DO NOT EDIT"
warning and the injected `Content-Hash` freshness line (plus `Source-Hash` with `hashes = "full"`; see
[`hashes`](#hashes)) that ai-rulez uses to detect whether a generated file changed since the last `generate`.
Only the amount of explanatory prose differs between styles.

#### `hashes`

Every generated file carries freshness lines in its header (a YAML comment inside the frontmatter for
skills and agents, an HTML comment in `CLAUDE.md`). `hashes` selects which ones:

| Value               | Lines written                 | Effect                                                                                      |
| ------------------- | ----------------------------- | ------------------------------------------------------------------------------------------- |
| `content` (default) | `Content-Hash`                | Each header depends only on that file's own body.                                           |
| `full`              | `Content-Hash`, `Source-Hash` | Adds the project-wide `Source-Hash`.                                                        |
| `none`              | neither                       | No hash lines at all.                                                                       |

`Content-Hash` is a hash of the file's own body and is stable. `Source-Hash` is a hash of the entire
source set (config, every rule, skill, agent, command, MCP server), so under `full` one edit changes the
`Source-Hash` line in every generated file. That is harmless for gitignored output, but when generated
files are committed it makes every change to `.ai-rulez/` rewrite hundreds of files and makes concurrent
branches conflict, which is why it is not the default. With `content` or `none`, editing one skill changes only
that skill's output (and `AGENTS.md`/`CLAUDE.md` when the edit changes what they render).

With `content` or `none`, `generate` decides whether to rewrite a file by comparing the whole rendered
file to what is on disk (ignoring the `Generated:` text when `timestamp = true`), so a changed header
style, custom header or config directory still re-renders. Consequently a formatter that edits generated
files is undone by the next `generate`, as it would be for any committed-output check. `clean` does not
read these lines in any mode. `verify --plugin` uses its own per-bundle provenance (the
`.ai-rulez-generated.json` sidecar) and is independent of this setting. Switching modes rewrites every
file once. Any other value is rejected by `validate`.

#### `mcp_line`

`minimal`, `compact` and `detailed` headers tell agents which MCP server to use. The command is derived
from [`[mcp]`](#mcp), the same settings that write the `.mcp.json` entry, so it matches the server the
project actually launches: `npx -y ai-rulez@latest mcp` by default, `npx -y ai-rulez@4.1.0 mcp` with
`self_server_version`, the `self_server_command` as written (`ai-rulez mcp`), and `--serve-skills` appended
whenever skills are served. The line is left out while `self_server = false`, because no ai-rulez server is
configured to point at. Set `mcp_line = false` to drop it from every header. A custom `text` is
never touched.

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
- `Content-Hash` freshness line (and `Source-Hash` with `hashes = "full"`)
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

- The freshness lines selected by `hashes` are still appended inside the banner, so
  hash-based skip detection keeps working. Do not add a closing comment marker of your own.
- `timestamp = true` still does nothing to a custom header; add a `Generated:` line in `text`
  yourself if you want one.
- Surrounding blank lines are trimmed; interior line breaks are preserved. Standard TOML multi-line
  escaping applies (`"""` for a literal block).

#### Nested roots

A root below the repository root names its own config in the header by its path from the git work tree
root, for example `Source: products/em/webapps/.ai-rulez/config.toml`, so "edit the source" leads to the
right file. The path comes from the work tree (the nearest ancestor holding a `.git` entry), so
`generate`, `generate --recursive` and `generate --check` render the same header. The repository root,
and a project outside a git work tree, keep the bare `.ai-rulez/config.toml`.

#### `nested`

A root that sits inside another ai-rulez root of the same git repository (some directory between it and
the repository root holds a config) would otherwise repeat the outer root's whole header in every
instruction file an agent loads. `nested` chooses what the inner root writes:

| Value               | Header of the nested root                                                              |
| ------------------- | -------------------------------------------------------------------------------------- |
| `compact` (default) | One line: `Generated by ai-rulez from <path to its config>; do not edit`.              |
| `omit`              | No header block.                                                                       |
| `inherit`           | The header of the configured `style`, as for the outer root.                           |

The outer root is never affected. `compact` still carries the freshness lines selected by `hashes`, so
the file keeps its `Content-Hash`. A custom [`text`](#custom-header) is kept for nested roots unless
`nested` is set explicitly. With `omit` the file has no `Content-Hash`: `generate --check` still compares
the whole rendered file and `generate` recognises its own output by the recorded manifest, but a
hand-edited file is then reported as blocked instead of edited.

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
| `source`         | Yes      | Git URL, `owner/repo` or `github:owner/repo`, `file://` URL, or local directory (`~/path` only from `config.local.toml` or the user config); see [source forms](installed-skills.md#source-forms) |
| `path`           | No       | Path within repo to the skill directory, or to a directory of skill directories (defaults to `skills/<name>`; when omitted and nothing is there, the conventional harness skill directories are searched) |
| `ref`            | No       | Git ref (branch, tag, commit)                                     |
| `local_override` | No       | Local path override for development                               |
| `domain`         | No       | Tie the skill to a domain so profiles and roles select it by domain like local content. Omit for global. |
| `include`        | No       | Keep only skills whose directory name matches one of these globs  |
| `exclude`        | No       | Drop skills whose directory name matches one of these globs; wins over `include` |
| `name_prefix`    | No       | Prepend this to every resolved skill name and output directory    |
| `resources`      | No       | What is bundled besides `references/`, `scripts/` and `assets/`: the whole skill directory by default (not `SKILL.md`, `evals/`, `agents/`, hidden files); setting it replaces that. See [Installed skills](installed-skills.md#what-is-bundled-with-a-skill) |
| `trust`          | No       | Scan level of the imported skills: `error` (default) or `warn`    |

Manage via CLI: `ai-rulez skill install/remove/list`.

### `installed_marketplaces` and `installed_plugins`

Plugin marketplaces and the plugins imported from them (or straight from a repository). Skills, agents, commands,
rules, hooks and MCP servers become ordinary content, pinned in `ai-rulez.lock`. See
[Installed Plugins](installed-plugins.md) for the formats, source forms, hook and MCP policy and the lock flow.

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

| Field (marketplace) | Description |
| ------------------- | ----------- |
| `name`, `source` | Required. `source` is a repository URL, GitHub `owner/repo`, a local directory inside the project or an https `marketplace.json` URL |
| `path`, `ref`, `version`, `tag_prefix`, `include_prerelease`, `min_release_age` | As on `includes` |
| `trust` | Scan level of the plugins read through it: `error` (default) or `warn` |

| Field (plugin) | Description |
| -------------- | ----------- |
| `name` | Required, unique |
| `marketplace`, `plugin` | Install the marketplace entry `plugin` (default `name`) from an installed marketplace; exclusive with `source` |
| `source`, `path`, `ref`, `version`, `tag_prefix`, `include_prerelease`, `min_release_age` | A plugin repository or directory, as on `installed_skills` |
| `format` | Force the layout: `claude`, `codex`, `gemini`, `cursor`, `agent-plugins` |
| `include`, `exclude` | Component kinds: `skills`, `agents`, `commands`, `hooks`, `mcp`, `rules` |
| `name_prefix`, `domain`, `profiles`, `trust` | As on `installed_skills` |
| `enable_hooks` | Import the plugin's hooks (default `false`) |
| `enable_mcp` | Import the plugin's MCP servers (default `true`) |

Manage via CLI: `ai-rulez marketplace add/remove/list/update`, `ai-rulez plugin install/remove/list/update`.

### `[skills]`, `[domains.<name>]`, `[[skill_sources]]`, `[lock]`

Dynamic skill loading (see [Dynamic skill loading](mcp-server.md#dynamic-skill-loading)). All optional.

| Key | Meaning |
| --- | ------- |
| `[skills] delivery` | Global default delivery: `static` (default), `served` or `both`. |
| `[domains.<name>] delivery` | Default delivery of a domain's skills. A skill's `delivery` frontmatter wins. |
| `[[skill_sources]]` | `name`, `url`, `ref`, `path`, `include`, `exclude`, `name_prefix`, `trust` (`error` or `warn`), `domain` (served only when the active profile or role selects it), `max_skills` (default 200), `max_bytes` (default 64 MiB), `max_clone_bytes` (default 256 MiB), `max_clone_files` (default 20000; each entry counts as at least 4 KiB toward `max_clone_bytes`; `AI_RULEZ_MAX_CLONE_FILES` sets it globally): skills served from a git repository or directory. A source over a limit is an error. |
| `[lock] enforce` | The skills server refuses a served skill whose digest is not pinned in `ai-rulez.lock`. |
| `[delivery.stub]` | Customises the generated `dynamic-skills` stub, see below. |

#### `[delivery.stub]`

The stub skill that tells an MCP-capable agent to call `find_skill` and `load_skill` is generated, and
its description is listed in every session. Each field is optional; an unset one keeps the built-in text:

```toml
[delivery.stub]
description = "Find and load team skills on demand."   # frontmatter description
body_file = "stub.md"                                   # relative to .ai-rulez/, or use body = "..."
```

| Key | Meaning |
| --- | ------- |
| `description` | Replaces the frontmatter description. Must not be whitespace-only; an empty string is the same as leaving the key out. |
| `body` | Replaces the stub's text. Multi-line TOML (`"""`) works. |
| `body_file` | Reads the text from a file relative to the config directory (`.ai-rulez/`); the path must stay inside it. A missing file is a load error. Mutually exclusive with `body`. |

The body may use one variable, `{{server}}`: the MCP server that serves the skills (the `ai-rulez` self
entry, or the `[[mcp_servers]]` entry running `--serve-skills`). Other `{{...}}` names are rejected, and the
body must still name `find_skill` and `load_skill`, since the stub is how an agent learns to call them.
Use it for local guidance such as which domains are served or how to rank matches. A skill you author
named `dynamic-skills` still replaces the stub, and `AR991` is unaffected: the customised stub is the
same skill. Editing the file behind `body_file` shows as drift in `generate --check`.

### `[ard]`

The [Agentic Resource Discovery](ard.md) manifest written by `publish --emit ard`. Optional.

| Key | Meaning |
| --- | ------- |
| `publisher` | Required. Fully qualified domain name the identifiers are anchored to. |
| `namespace` | Required. Identifier segment between publisher and name; colons separate sub-segments. |
| `base_url` | https location of `skills/<name>/SKILL.md` (written next to `ard.json`). |
| `plugin_type` | Media type of plugin entries. |
| `queries` | `representativeQueries` (1 to 5 each) by resource name. |

### `[publish]`

What [`ai-rulez publish`](publish.md) ships and its policy gates. All optional; it has no credential keys.

| Key | Meaning |
| --- | ------- |
| `runtimes` | Plugin runtimes to publish (default: the `[plugin]` runtimes). `--runtime` overrides it. |
| `require_signature` | Fail unless the archive is signed (`AR9N7`). |
| `require_approved` | Fail unless every item `[governance]` selects has a valid approval (`AR9N8`). |
| `allow_dirty` | Publish from a tree with uncommitted changes. |
| `[publish.github_release] repo` | `OWNER/REPO` of the release. |
| `[publish.oci] ref` | Repository `host/path` (no tag) of the OCI target. |
| `[publish.npm]` | `scope` (`@acme`, required for the npm target), `access` (`restricted` or `public`), `registry` (https). |
| `[publish.marketplace.channels]` | Channel name to the git ref its pinned marketplace index points at. |
| `[[publish.emitters]]` | `name` (`template`, `cursor-team-marketplace`, `port`, `aws-agent-registry`, `kiro-steering`, `agent-plugins`, `ard`, `mcp-server-card`), `template` and `output` for the template emitter, `options` (for example `blueprint` for `port`). |

## Layered configuration

Split the configuration across `config.<name>.toml` files beside `config.toml` and choose which apply. The active
layers are merged over `config.toml` in the order they are named, so a later layer wins; `config.local.toml` is applied
after all of them. `config.toml` always loads.

```toml
# .ai-rulez/config.ai.toml
[[mcp_servers]]
name = "basemind"
command = "basemind"
args = ["serve"]

[[installed_skills]]
name = "cloudflare"
source = "https://github.com/cloudflare/skills"
include = ["cloudflare", "workers-best-practices"]
```

```toml
# .ai-rulez/config.custom.toml
[[installed_skills]]
name = "cloudflare"
source = "https://github.com/cloudflare/skills"
include = ["cloudflare*"]
```

```bash
AI_RULEZ_LAYERS=ai,custom ai-rulez generate   # custom layers after ai, so its include wins
ai-rulez config layers                        # list the layers and which are active
```

Layers become active in this order, each merged over the one before:

1. `default_layers` in `config.toml`: committed, always active, no environment variable needed. A layer listed there must
   have its `config.<name>.toml`; a missing file is an error that names it. The key is read from `config.toml` only and
   is ignored (with a warning) inside a layer or the local overlay.
2. `AI_RULEZ_LAYERS`, a comma-separated list, or `AI_RULEZ_PROFILE` when that is not set, so one value can pick a profile
   and the layers named like it. The `--profile` flag does not select layers. A name with no file is skipped (so
   `default` and profile names without a layer are harmless).
3. `config.local.toml`, the machine-local overlay, always last. `local` is a reserved layer name.

A name repeated across these keeps its first position. Use `default_layers` to give a team-owned concern (lint
allow-lists, builtin exclusions, MCP servers) its own file and CODEOWNERS entry:

```toml
# .ai-rulez/config.toml
default_layers = ["lint", "mcp"]
```

`ai-rulez config layers` prints every layer with its position and origin (`default`, `env`), in text and with
`--format json`. Errors for a layer name the layer file (`unknown key(s) in layer config.lint.toml: bogus`).

Layers merge like the local overlay: tables merge per key, named lists (`mcp_servers`, `installed_skills`, `plugins`,
`includes`, `marketplaces`, `scopes`, `roles`, `verifiers`, `plugins_authoring`, `marketplaces_authoring`) merge entry by
entry on `name` (`remove = true` drops one), `[[marketplace.plugins]]` merges the same way inside the `[marketplace]`
table, and any other value is replaced. `[governance]` and `[signing]` are shared policy and are ignored in a layer. A layered configuration is a
merged view and is never written back by a command that edits `config.toml`.

## Local overlay

A `config.local.toml` file beside `config.toml` is a machine-local, gitignored overlay
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
version = "5.0"
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
version = "5.0"
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
version = "5.0"
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
version = "5.0"
name = "simple-project"
presets = ["claude", "cursor"]
```

### Multi-Team Monorepo

For monorepos with multiple independent teams:

```toml
version = "5.0"
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
version = "5.0"
name = "saas-app"
presets = ["claude"]

default = "production"

[profiles]
development = ["dev-guidelines"]
staging = ["staging-guidelines"]
production = ["production-guidelines", "security-hardened"]
```

## Validation

`ai-rulez validate` checks the raw config file against the JSON schema
(`schema/ai-rules.schema.json`) — so an unknown key or a value outside an enum is reported rather
than silently dropped — and then runs the structural checks below. TOML is converted to JSON for
the schema check.

```bash
ai-rulez validate
ai-rulez validate --config-only
```

The structural checks cover:

- `version` is `"5.0"`
- `name` is present and non-empty
- All preset names are valid
- `builtin:<name>` and `builtin:<name>/<item>` references in profiles name a real builtin or pack item
- Positive `builtins` entries (`<name>` or `<name>/<item>`) name a real builtin or pack item
- Profile references resolve (a missing domain is a warning, not a failure)
- File paths are valid
- MCP server definitions are well-formed
- A `config.local.*` overlay, if present, passes `schema/ai-rules-local.schema.json` and merges into a valid config (`--no-local` skips this)

## Programmatic Modification with CRUD Operations

ai-rulez provides CRUD (Create, Read, Update, Delete) commands to programmatically modify your configuration. This is useful for:

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
6. **Commit sources and outputs together**: `gitignore` defaults to `false`, so commit `.ai-rulez/` and the generated files; set `gitignore = true` only when the team keeps generated files out of git

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

# Commit (generated files too, unless gitignore = true)
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
