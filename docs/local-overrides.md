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
Every file below is one the tool loads on its own; a `.local.md` file a tool never reads is not written.

- Local rules the preset routes to rule files are written as `<rulesdir>/<id>.local<ext>`, the same
  routing shared rules use, so the tool loads them natively.
- Everything else (local context, and rules the preset keeps inline) goes to the file the tool loads for
  machine-local instructions, listed below. It is written only when there is something to put in it.
- Local skills, agents and commands are written per item, to the same paths as shared ones (for
  example `.claude/skills/<name>/SKILL.md`). A preset that aggregates them into a shared file, or has no
  output for them, gets none; `generate` warns once per preset.
- Content a preset has no place for (local context with Cursor, say) is not written and is reported in
  one warning per preset.
- Custom providers (`.ai-rulez/providers/`) get no local output at all.

| Preset | Split mode (default) | Inline mode | Loaded by |
| ------ | -------------------- | ----------- | --------- |
| `claude` | Rules: `.claude/rules/<id>.local.md`. Context: `CLAUDE.local.md` | Path-scoped rules: `.claude/rules/<id>.local.md`. Other rules and context: `CLAUDE.local.md` | Claude Code reads `CLAUDE.local.md` after `CLAUDE.md` and the rules folder natively |
| `junie` | Rules: `.junie/rules/<id>.local.md`. Context: `.junie/rules/ai-rulez.local.md` | Rules and context: `.junie/rules/ai-rulez.local.md` | Junie loads `.junie/rules/*.md` only on its `AGENTS.md` discovery path: a root `AGENTS.md` combined with `.junie/playbook.md` and every `.junie/rules/*.md`. A `.junie/AGENTS.md` takes precedence and ends the search, and the legacy `.junie/guidelines.md` layout does not load the rules folder, so with either of those the local file is not read. Based on JetBrains' documentation; not tested against Junie |
| `cursor` | Rules: `.cursor/rules/<id>.local.mdc`. No place for context | Same | Cursor loads its rules folder |
| `devin` | Rules: `.devin/rules/<id>.local.md`. No place for context | Same | Devin loads its rules folder |
| `cline` | Rules: `.clinerules/<id>.local.md`. No place for context | Same | Cline loads its rules folder |
| `copilot` | Routed rules: `.github/instructions/<id>.local.instructions.md`. Rules Copilot keeps inline (`auto`, `manual`, negated-only globs) and context: `.github/instructions/ai-rulez.local.instructions.md` (`applyTo: "**"`) | Same routing as shared rules; the remainder goes to `ai-rulez.local.instructions.md` | Copilot loads path-specific instructions files |
| `antigravity` | Rules: `.agents/rules/<id>.local.md`. Context: `.agents/rules/ai-rulez.local.md` (`trigger: always_on`) | Path-scoped rules: `.agents/rules/<id>.local.md`. Other rules and context: `.agents/rules/ai-rulez.local.md`. With `gemini` also enabled and no explicit `mode_by_preset`, all rules stay in `ai-rulez.local.md` | Antigravity loads every `.agents/rules/*.md` with a `trigger`; it never reads `GEMINI.local.md` |
| `gemini` | Rules and context: `GEMINI.local.md` | Same | Gemini CLI loads it because `.gemini/settings.json` `context.fileName` lists it (written whether or not local content exists). A `context.fileName` you wrote gets `GEMINI.local.md` appended; see [Settings documents shared with you](#settings-documents-shared-with-you) |
| `opencode` | Rules and context: `AGENTS.local.md` | Same | OpenCode loads it because `opencode.json` `instructions` lists it (written whether or not local content exists; `./AGENTS.local.md` counts as the same entry) |
| `xum` | Rules and context: `AGENTS.local.md` (shared with `opencode`) | Same | xum appends `AGENTS.local.md` to `AGENTS.md` |
| `pi` | Nothing written, one warning | Same | pi reads no project-local `AGENTS.local.md`; put personal guidance in pi's user config |
| `codex` | Rules and context: `AGENTS.override.md` (root only) | Same | Codex loads `AGENTS.override.md` instead of `AGENTS.md` in the same directory, so the file repeats the shared `AGENTS.md` and appends the local sections |
| `hermes` | With `agents_md`: `AGENTS.override.md`, as for Codex. Without it: nothing written, one warning | Same | Hermes loads `AGENTS.override.md` instead of `AGENTS.md` in the AGENTS chain; `.hermes.md` (without `agents_md`) beats the chain and has no local counterpart |
| `amp` | Nothing written, one warning | Same | Amp has no project-local file; put personal guidance in `~/.config/amp/AGENTS.md` |

`AGENTS.override.md` is generated, git-ignored and listed in the local manifest, so it is removed when the local
content goes away. Do not edit it: it replaces `AGENTS.md` for those tools and is rebuilt from `AGENTS.md` on every
`generate`. It repeats the `AGENTS.md` body that run actually wrote (without that file's generated banner, which keeps a
`[header] timestamp` from rewriting it every run). An `AGENTS.override.md` that is not in the local manifest and has no
generated banner is yours: it is never overwritten or deleted, `generate` warns naming it, and Codex then reads it
instead of the local content. If local content exists but no `AGENTS.md` is produced, the file is not written and
`generate` warns. A local rule named `ai-rulez` would map to the same path as the generated `ai-rulez.local.*` root file of
`junie`, `antigravity` and `copilot`, so it is written as `ai-rulez-<hash>.local<ext>` instead.

### Bookkeeping

- **Gitignore.** Local rule files share a folder with committed rules, so one pattern per folder is
  written (for example `.claude/rules/*.local.*`) before the files themselves. The managed block also
  lists `.ai-rulez/local/`, `.ai-rulez/config.local.*`, `.ai-rulez/.config.local.*` (lock and temp
  files) and `.ai-rulez/.generated-manifest.local.json`.
- **Symlinked `.gitignore`.** Git does not read a `.gitignore` that is a symbolic link, and ai-rulez never
  writes through it: every ignore entry goes to a per-project block in `.git/info/exclude` instead.
- **Fail closed.** `generate` stops, listing the paths, when a machine-local or secret-bearing output, the
  `config.local.*` overlay or the `local/` tree would not be git-ignored after the entries are written (for
  example because a `.gitignore` line such as `!.ai-rulez/local/` un-ignores it). Narrow the rule, or use
  `--no-local`.
- **Per-clone excludes.** Local-only outputs whose names do not contain `.local.` (a local skill's
  `SKILL.md`, `AGENTS.override.md`, an output of an overlay-defined preset) differ per machine. They are listed in a block of
  this project's `.git/info/exclude`, keyed by the project's config directory, so they stay out of the
  shared `.gitignore`. Outside a git repository they fall back to the managed `.gitignore` block.
  `clean`, or a run with no local inputs left, removes the block.
- **Local manifest.** Local outputs are tracked in `.ai-rulez/.generated-manifest.local.json`, not in
  the committed manifest. A teammate's `generate` never deletes your local files; `clean` removes them
  through the local manifest. It also records what ai-rulez merged into settings documents you share with
  it (below), because the server names can come from your overlay. It is ignored like the other local files.
- **Permissions.** Generated files that contain a resolved MCP secret are written `0600`, whichever
  preset produced them. The overlay itself is written `0600`.
- **Files from earlier versions.** `AGENTS.local.md` (codex and amp only setups), `.hermes.local.md`,
  `.junie/guidelines.local.md` and Antigravity's `GEMINI.local.md` were never read by their tools. The first
  `generate` after upgrading removes them through the local manifest.
- **Reserved names.** `*.local.*` in a rules folder is reserved. A hand-written file with such a name is
  skipped with a warning.

### Settings documents shared with you

`.claude/settings.json`, `.gemini/settings.json`, `opencode.json`, `.mcp.json`, `.agents/settings.json` and
`.xum/mcp.jsonc` can hold your own settings beside what ai-rulez writes. ai-rulez records, per document, the MCP
server entries, array elements and scalar keys it merged in, each with a digest of the value it wrote (never the
value, which may be a secret). A document it wrote whole is recorded in the committed manifest, a document shared
with you or carrying overlay content in the local manifest.

- **`clean`** removes exactly those entries, only while they still hold the value ai-rulez wrote, and keeps every
  other key and the document's formatting. An entry you edited is yours: it stays and `clean` warns once, naming
  the file and key. A key or object the removal leaves empty is dropped, and a document with nothing else in it is
  deleted (a document made only of ai-rulez's keys, such as one it created, is therefore deleted by `clean`). This
  includes an `mcpServers` entry that holds a resolved secret (a header or env value from your overlay), which
  previously outlived the overlay in a hand-written `.claude/settings.json`.
- **`generate`** takes back what an earlier run recorded and this one no longer produces: a preset removed from the
  config, or an MCP server removed. It works after the overlay is deleted, because the record is the local
  manifest. `generate --dry-run` lists these as `unmerge:` lines.
- **No record.** A document merged by 4.23.0 or earlier, or a fresh clone, has no record. The fallback runs on
  `clean` only, never on `generate`, and is narrow: it removes an MCP server your config declares by name only when
  its value equals what the config would render for that document (a hand-written server of the same name with
  another value stays, with a warning, even if the preset that would write it is off), the ai-rulez
  self-registration when it equals what ai-rulez writes, a `context.fileName` that exactly equals a value ai-rulez
  wrote, `AGENTS.local.md` entries of `instructions`, and an OpenCode `$schema` that is the document's only key. A
  hand-written value that happens to equal what ai-rulez would render is indistinguishable from its own and is
  removed. A server removed from the config before the first run that records it is not recognized and stays;
  delete it by hand.
- **Hand-written servers.** A server whose name is not in your config is never touched. One with the same name as
  a configured server is replaced by the configured value on `generate`.
- **Gemini `context.fileName`.** A value that exactly equals one of ai-rulez's forms (`["AGENTS.md"]`,
  `["AGENTS.md", "GEMINI.local.md"]`, `["GEMINI.md", "GEMINI.local.md"]`) is ai-rulez's, with or without a
  manifest, and is rewritten to the current form (this is how `agents_md` toggles it). Any other value is yours: it
  is kept, a single string becomes a list, and `GEMINI.local.md` (and `AGENTS.md` under `agents_md`) is appended
  when missing, without a warning. Only the names ai-rulez added are taken back: `clean` on `"MY.md"` leaves
  `["MY.md"]` (a list; the string form is not restored), and turning `agents_md` off removes only the `AGENTS.md`
  it appended. An `AGENTS.md` you listed yourself stays. With `agents_md` off, a list without `GEMINI.md` still
  gets a warning, because Gemini then ignores the generated file.
- **OpenCode.** `AGENTS.local.md` is appended to your `instructions` (`./AGENTS.local.md` counts as the same entry)
  and taken back by `clean`. `$schema` is written only when ai-rulez creates `opencode.json`, never added to yours.
- **Comments (JSONC).** Gemini CLI and OpenCode accept comments in these files, but rewriting would delete them.
  A document with comments or trailing commas is therefore left untouched: when only the `context.fileName` or
  `instructions` registration would be written, `generate` warns (and `GEMINI.local.md` or `AGENTS.local.md` is not
  loaded until you add it by hand); generation does not stop. When MCP servers must be written into such a document,
  `generate` still stops with a hint, as before. `clean` leaves a commented document alone with a warning.
  This includes `.xum/mcp.jsonc`, which is JSONC by definition: once you add a comment, ai-rulez can no longer
  edit it, so the servers it merged stay after a preset or server is removed or after `clean`. It warns once per
  run; remove those entries by hand.
- `clean` does not print the Gemini `context.fileName` advice.

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
