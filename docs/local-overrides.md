# Local Configuration

Personal, machine-local configuration that is never committed. Use it for scratch notes, per-machine
paths, personal presets and MCP servers, secrets, and experiments that belong to your checkout but
not to the shared configuration.

Local configuration has two layers. Both are gitignored unconditionally, even when `gitignore = false`.

| Layer | Location | Holds |
| ----- | -------- | ----- |
| Content tree | `.ai-rulez/local/` | Rules, context, skills, agents, commands and domains |
| Config overlay | `.ai-rulez/config.local.{toml,yaml,yml,json}` | Settings merged onto `config.toml`: presets, profiles, MCP servers, includes, installed skills, and so on |

Committed outputs never contain local content, so a teammate who checks out your branch sees only
the shared configuration. Generated local files are written for your tools to load, and are kept out
of git.

## Content tree

```text
.ai-rulez/
├── config.toml
├── rules/              # shared, committed
├── context/            # shared, committed
└── local/              # machine-local, gitignored
    ├── rules/
    ├── context/
    ├── skills/
    ├── agents/
    ├── commands/
    └── domains/<name>/   # same layout; selected by the active profile like shared domains
```

- It mirrors the shared layout and uses the same file formats and frontmatter.
- Local domains are selected by the active profile like shared ones. A local profile (in the overlay)
  may reference local domains.
- Local content is root-only: `[[scopes]]` runs never emit local outputs.
- Files and directories created through the CLI or MCP tools are owner-only (`0600` / `0700`).
- A local skill, agent or command with the same name as a shared one is an error: a local file may
  never replace a shared one. Skills and commands share one namespace. Local rules and context may
  reuse a shared name; see [Collisions](#collisions).

### Generated output

Where a local rule or context item lands depends on the preset and the [rules mode](rules.md#rules-mode).

- Local rules the preset routes to rule files are written as `<rulesdir>/<id>.local<ext>`, the same
  routing shared rules use, so the tool loads them natively.
- Everything else (local context, and rules the preset keeps inline) goes to the preset's `.local`
  root file. A root file is written only when there is something to put in it.
- Local skills, agents and commands are written per item, to the same paths as shared ones (for
  example `.claude/skills/<name>/SKILL.md`). A preset that aggregates them into a shared file, or has no
  output for them, gets none; `generate` warns once per preset.
- Content a preset has no place for (local context with Cursor, say) is not written and is reported in
  one warning per preset.
- Custom providers (`.ai-rulez/providers/`) get no local output at all.

| Preset | Split mode (default) | Inline mode |
| ------ | -------------------- | ----------- |
| `claude` | Rules: `.claude/rules/<id>.local.md`. Context: `CLAUDE.local.md` | Path-scoped rules: `.claude/rules/<id>.local.md`. Other rules and context: `CLAUDE.local.md` |
| `junie` | Rules: `.junie/rules/<id>.local.md`. Context: `.junie/guidelines.local.md` | Rules and context: `.junie/guidelines.local.md` |
| `cursor` | Rules: `.cursor/rules/<id>.local.mdc`. No place for context | Same |
| `windsurf` | Rules: `.windsurf/rules/<id>.local.md`. No place for context | Same |
| `cline` | Rules: `.clinerules/<id>.local.md`. No place for context | Same |
| `continue-dev` | Rules: `.continue/rules/<id>.local.md`. No place for context | Same |
| `copilot` | Routed rules: `.github/instructions/<id>.local.instructions.md`. Rules Copilot keeps inline (`auto`, `manual`, negated-only globs) and context: `.github/instructions/ai-rulez.local.instructions.md` (`applyTo: "**"`) | Same routing as shared rules; the remainder goes to `ai-rulez.local.instructions.md` |
| `antigravity` | Rules: `.agents/rules/<id>.local.md`; they stay in `GEMINI.local.md` instead when `gemini` is also enabled and `mode_by_preset` does not set the mode. Context: `GEMINI.local.md` | Path-scoped rules: `.agents/rules/<id>.local.md`. Other rules and context: `GEMINI.local.md` |
| `gemini` | Rules and context: `GEMINI.local.md` | Same |
| `codex`, `opencode`, `amp`, `xum` | Rules and context: `AGENTS.local.md` (one shared file) | Same |
| `hermes` | Rules and context: `.hermes.local.md` | Same |

The `.local` variant of a single-file root is `<root>` with `.local` before the extension. Duplicate
paths collapse, which is why four presets share one `AGENTS.local.md`.

### Bookkeeping

- **Gitignore.** Local rule files share a folder with committed rules, so one pattern per folder is
  written (for example `.claude/rules/*.local.*`) before the files themselves. The managed block also
  lists `.ai-rulez/local/`, `.ai-rulez/config.local.*`, `.ai-rulez/.config.local.*` (lock and temp
  files) and `.ai-rulez/.generated-manifest.local.json`.
- **Per-clone excludes.** Local-only outputs whose names do not contain `.local.` (a local skill's
  `SKILL.md`, an output of an overlay-defined preset) differ per machine. They are listed in a block of
  this project's `.git/info/exclude`, keyed by the project's config directory, so they stay out of the
  shared `.gitignore`. Outside a git repository they fall back to the managed `.gitignore` block.
  `clean`, or a run with no local inputs left, removes the block.
- **Local manifest.** Local outputs are tracked in `.ai-rulez/.generated-manifest.local.json`, not in
  the committed manifest. A teammate's `generate` never deletes your local files; `clean` removes them
  through the local manifest.
- **Permissions.** Generated files that contain a resolved MCP secret are written `0600`, whichever
  preset produced them. The overlay itself is written `0600`.
- **Reserved names.** `*.local.*` in a rules folder is reserved. A hand-written file with such a name is
  skipped with a warning.

### Collisions

- A local rule that has the same name as a shared rule is allowed; its file is `<id>.local<ext>`, so
  nothing is overwritten.
- Two local rules that map to the same file name are disambiguated as `<id>-<hash>.local<ext>`.
- Local skills, agents and commands that collide with a shared item fail `generate` with an error
  naming both files.

## Config overlay

`config.local.toml` (or `.yaml`, `.yml`, `.json`) sits next to `config.toml`. It is merged onto the
shared config in memory at load time and is never written into the shared config. Only one overlay
file may exist; more than one is an error. The overlay is skipped for plugin bundles
(`generate --plugin`), which are distributable.

Create one with `ai-rulez local init`, or let `local set`, `--local` and the MCP `local: true` flag
create it on first use. Overlay files are validated against `schema/ai-rules-local.schema.json` (see
[Schema Reference](schema.md)); the merged result must also be a valid config.

### Merge semantics

| Key kind | Keys | Rule |
| -------- | ---- | ---- |
| Scalars | `name`, `description`, `default`, `gitignore`, `compact`, ... | Local value wins |
| Lists | Any list not named below | Local list replaces the shared list |
| Tables | `profiles`, `header`, `defaults`, `mcp`, `rules`, `plugin`, `marketplace` | Merged per key; local keys win |
| `presets` | | Ordered union: local entries are appended, duplicates collapse. `"!name"` drops a shared preset |
| Named lists | `mcp_servers`, `plugins`, `includes`, `installed_skills`, `marketplaces`, `scopes` | Entries merge by `name` (`scopes` by `path` when unnamed). A local entry with `remove = true` deletes the shared entry; a new name appends |
| `builtins` | | Local value replaces the shared one |
| `version` | | Must equal the shared version if set |

Other rules:

- Unknown keys are an error. Error messages name keys and entry positions, never values.
- Dropping a preset or removing an entry that the shared config does not have produces a warning.
- `remove` is not valid on preset tables; use `"!name"`.

```toml
# .ai-rulez/config.local.toml
presets = ["codex", "!cursor"]   # add codex, drop cursor
default = "dev"

[profiles]
dev = ["backend", "my-local-domain"]

[[mcp_servers]]
name = "github"                  # merges onto the shared "github" server
transport = "stdio"

[mcp_servers.env]
GITHUB_TOKEN = "ghp_example"

[[includes]]
name = "team-rules"
remove = true                    # drop a shared include on this machine
```

## Managing local configuration

### Commands

The `ai-rulez local` command edits the overlay:

| Command | Action |
| ------- | ------ |
| `local init` | Create a `config.local.*` skeleton in the main config's format (commented for TOML and YAML, `{}` for JSON; gitignored) |
| `local show` | Print every key the overlay sets and the shared value it replaces |
| `local set <path> [value]` | Set a key |
| `local unset <path>` | Remove a key |
| `local path` | Print the overlay file path |

```bash
ai-rulez local set default dev
ai-rulez local set presets '["codex", "!cursor"]'
ai-rulez local set mcp_servers.github.command npx
printf %s "$TOKEN" | ai-rulez local set mcp_servers.github.env.GITHUB_TOKEN --stdin
ai-rulez local set 'mcp_servers["foo.bar"].command' npx
ai-rulez local show --json
```

- **Value parsing.** The value is parsed as a TOML literal and falls back to a plain string. Env and
  header values and known text fields (`url`, `command`, `source`, `path`, `ref`, `description`,
  `name`, `transport`, `default`, `*_version`) at their real positions are always strings. `--string`
  forces a string.
- **Secrets.** Put them on stdin with `--stdin` (the value is stored as a string) rather than on the
  command line, where they end up in shell history.
- **Dotted names.** Write a segment containing a dot in brackets with double quotes:
  `mcp_servers["foo.bar"].command`.
- **Redaction.** `local show` withholds values by default. Only an allowlist of keys known to hold no
  credentials (`name`, `description`, `default`, `presets`, `gitignore`, `compact`, `builtins`,
  `profiles.*`, `defaults.effort*`, `defaults.omit_agent_fields`, `rules.mode*`, `header.style|hashes|timestamp`,
  `mcp.self_server`, `mcp.self_server_version`, and `transport`, `enabled`, `remove` of list entries)
  is printed, and only when the value has the expected type. Everything else shows its key path and
  `<redacted>`. `--reveal` prints everything and may print secrets.
- **Safety.** Every write takes a file lock, ensures the ignore entries first, and validates the merged
  config. If validation fails the previous file is restored.
- **Location.** The subcommands honour the global `--config` and `--config-dir` / `-n`.

`profile list`, `include list` and `skill list` show the shared layer only and print how many local
entries `config.local.*` adds. Use `local show` to see them.

### Content and config CRUD with `--local`

The `--local` flag on CLI commands, and `local: true` on the MCP tools, redirect a change to the
local layer.

| Target | Commands |
| ------ | -------- |
| `.ai-rulez/local/` content | `add rule\|context\|skill\|agent\|command`, `remove rule\|context\|skill\|agent\|command`, `list rules\|context\|skills\|agents\|commands` |
| Overlay | `profile add\|remove\|set-default`, `include add\|remove`, `skill install\|remove` |

```bash
ai-rulez add rule local-paths --local                   # .ai-rulez/local/rules/local-paths.md
ai-rulez add context local-env-notes --local
ai-rulez add skill my-debug-skill --local
ai-rulez add rule team-notes --local --domain backend   # .ai-rulez/local/domains/backend/rules/
ai-rulez profile add dev backend my-local-domain --local
ai-rulez include add scratch ../scratch-rules --local
```

- `--local --domain <name>` writes to `.ai-rulez/local/domains/<name>/`, creating a local domain.
- Local profiles may use shared or local domains.
- Removing a shared include or installed skill with `--local` writes `remove = true` to the overlay.
  Shared profiles cannot be removed locally: `profile remove --local` of a shared profile is an error.
- `domain add|remove` and the MCP `create_domain`, `delete_domain`, `list_domains` tools have no local
  form; a local domain exists once local content is written to it.

## Drift guard

The overlay can change files that the team shares (an overlay MCP server adds a block to
`.mcp.json`, say). To make sure a local value never reaches a tracked file, `generate` renders the
shared view in parallel with the merged view and classifies every output:

| Class | Meaning | Result |
| ----- | ------- | ------ |
| local-only | Only the merged render produces the file | Written; kept out of git |
| drift | Both renders produce the file with different content | Written only if the file is git-ignored and untracked |
| suppressed | Only the shared render produces the file (the overlay dropped it) | Left alone |

A run is **blocked** and exits non-zero, writing nothing, when:

- a drift file is tracked by git, or is not ignored, or
- a local-only file is tracked by git, or
- git cannot answer which files are tracked (every candidate then counts as tracked).

The error names paths only, never content. If the shared render itself fails, the run fails closed.

Ways forward:

- Git-ignore the files, or untrack them.
- `generate --no-local` generates the shared view only (the view a teammate sees).
- `generate --allow-local-drift` writes anyway. This can put non-secret overlay values into tracked files.
  It does not bypass the secret guard: a generated MCP config that would carry resolved secrets (env,
  headers, URL credentials, secret flags) is still refused unless it is git-ignored. It is CLI-only: the MCP
  server cannot bypass the guard.

`generate --dry-run` prints the plan with these lines, and exits non-zero when it would be blocked:

```text
local-only: .claude/rules/scratch.local.md
drift: .mcp.json
allowed: .gemini/settings.json (drift, but git-ignored)
suppressed: .cursor/mcp.json
blocked: .mcp.json (shared output is tracked and would change)
```

Header `Source-Hash` values of shared outputs come from the shared view, so a teammate regenerating
sees no hash churn; local-only outputs carry a hash of their local inputs. The overlay enters that hash
only in redacted form.

## `--no-local`

`--no-local` ignores both layers. It is accepted by `generate`, `validate` and `tokens`; `verify` has no
flag because it always checks the shared view. A `--no-local` run neither deletes your existing local
files nor removes their ignore entries. Use it in hooks and CI so results do not depend on one
machine's overlay; see [Git hooks](poly-hooks.md).

## Workflow

1. Add local content: `ai-rulez add rule local-paths --local`, or edit the overlay with `ai-rulez local`.
2. Edit the source file under `.ai-rulez/local/rules/`.
3. Run `ai-rulez generate`. Under the default split mode this writes
   `.claude/rules/local-paths.local.md` (and the equivalent file for each configured preset), and
   ensures the ignore entries exist. `CLAUDE.local.md` appears only for local context or rules kept
   inline.
4. Commit as usual. Local files and the local manifest stay out of the commit.

Removing a file under `.ai-rulez/local/` and regenerating drops the corresponding output.

## Security notes

- The overlay and local tree may hold secrets. They are written owner-only, and ignore entries are
  written before the files themselves; writers fail closed if the entries cannot be written.
- The `local show` default withholds values; `read_config` over MCP returns key paths only.
- The overlay is hashed into generated headers only in redacted form: env and header values and args
  are replaced, and URLs (including include and skill sources) lose their credentials, query and
  fragment; scheme, host and path still contribute to the hash.
- `--allow-local-drift` is the one way to write non-secret overlay-derived values into tracked files. Do
  not use it in shared scripts. It never writes resolved secrets into an MCP config that is not git-ignored.

## Related

- [Configuration Reference](configuration.md#local-overlay) - overlay in the config reference
- [CLI Commands](cli.md#local-configuration) - `ai-rulez local` and `--local`
- [MCP Server](mcp-server.md) - the `local` tool parameter
- [Rules](rules.md) - rules mode and rule files
