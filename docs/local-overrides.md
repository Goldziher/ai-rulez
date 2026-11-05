# Local configuration

Machine-local configuration that is never committed: scratch notes, per-machine paths, personal presets and MCP servers, secrets and experiments that belong to your checkout but not to the shared configuration.

```bash
ai-rulez add rule local-paths --local   # .ai-rulez/local/rules/local-paths.md
ai-rulez generate
```

Your tools load the local rule (for `claude`, from `CLAUDE.local.md` or `.claude/rules/local-paths.local.md`). Every local input and output is git-ignored; the only tracked change is the managed `.gitignore` block that the first run adds for the local inputs.

Local configuration belongs to one checkout. Instructions and skills that should follow you into every repository go to your home directories instead; see [User-level configuration](user-scope.md). The committed `config.<name>.toml` layers that `default_layers` activates for everyone are a different feature; see [Layered configuration](configuration.md#layered-configuration).

There are two layers, both git-ignored even with `gitignore = false`:

| Layer | Location | Holds |
| --- | --- | --- |
| Content tree | `.ai-rulez/local/` | Rules, context, skills, agents, commands and domains |
| Config overlay | `.ai-rulez/config.local.toml` | Settings merged onto `config.toml`: presets, profiles, MCP servers, includes, installed skills, ... |

Committed outputs never contain local content, so a teammate who checks out your branch sees only the shared configuration.

## Content tree

```text
.ai-rulez/
├── config.toml
├── rules/                  # shared, committed
├── context/                # shared, committed
└── local/                  # machine-local, git-ignored
    ├── rules/
    ├── context/
    ├── skills/
    ├── agents/
    ├── commands/
    ├── checks/
    └── domains/<name>/     # selected by the active profile like shared domains
```

- It mirrors the shared layout, with the same file formats and frontmatter.
- Local domains are selected by the active profile like shared ones, and a profile defined in the overlay may name them.
- Local content is root-only: `[[scopes]]` never get local outputs.
- Files and directories the CLI or MCP tools create here are owner-only (`0600`, `0700`).
- A local skill, agent or command may not have the name of a shared one (skills and commands share one namespace); `generate` fails naming both files. A local rule or context item may reuse a shared name; see [Collisions](#collisions).

### Generated output

A local rule follows the same routing as a shared rule of the preset (the [rules mode](rules.md#rules-mode) and [`agents_md`](configuration.md#agents_md) decide): a rule the preset writes to its rules folder becomes `<rulesdir>/<id>.local<ext>`. Local context, and rules the preset keeps inline, go to the file the tool loads for machine-local instructions:

- Local skills, agents and commands are written per item to the same paths as shared ones (`.claude/skills/<name>/SKILL.md`). A preset that has no per-item output for them gets none, with one warning per preset.
- Content a preset has no place for (local context with `cursor`) is not written, with one warning per preset:

  ```text
  WARN  Machine-local content has no output for this preset and was not written preset=cursor items=context env
  ```

- Custom providers (`.ai-rulez/providers/`) get no local output.

| Preset | Local rules routed to files | Everything else | How the tool loads it |
| --- | --- | --- | --- |
| `claude` | `.claude/rules/<id>.local.md` | `CLAUDE.local.md` | Claude Code reads `CLAUDE.local.md` after `CLAUDE.md`, and its rules folder |
| `cursor` | `.cursor/rules/<id>.local.mdc` | not written | Rules folder |
| `devin` | `.devin/rules/<id>.local.md` | not written | Rules folder |
| `cline` | `.clinerules/<id>.local.md` | not written | Rules folder |
| `junie` | `.junie/rules/<id>.local.md` | `.junie/rules/ai-rulez.local.md` | `.junie/rules/*.md`, read beside a root `AGENTS.md`; a `.junie/AGENTS.md` or `.junie/guidelines.md` layout does not read the folder |
| `copilot` | `.github/instructions/<id>.local.instructions.md` | `.github/instructions/ai-rulez.local.instructions.md` (`applyTo: "**"`) | Path-specific instruction files |
| `antigravity` | `.agents/rules/<id>.local.md` | `.agents/rules/ai-rulez.local.md` (`trigger: always_on`) | Every `.agents/rules/*.md` with a `trigger` |
| `gemini` | none | `GEMINI.local.md` | Listed in `context.fileName` of `.gemini/settings.json` |
| `opencode` | none | `AGENTS.local.md` | Listed in `instructions` of `opencode.json` |
| `xum` | none | `AGENTS.local.md` | xum appends it to `AGENTS.md` |
| `pi` | none | `AGENTS.local.md` | The `.local` variant of pi's `AGENTS.md` |
| `codex` | none | `AGENTS.override.md` (root only) | Codex reads it instead of `AGENTS.md` in the same directory |
| `hermes` | none | `AGENTS.override.md` with `agents_md`; otherwise not written | Read instead of `AGENTS.md` in the AGENTS chain; `.hermes.md` has no local counterpart |
| `amp` | none | not written | Amp reads only `~/.config/amp/AGENTS.md` |

With `agents_md` on (the default), always-on rules are inlined rather than written to rule files, so `claude` puts always-on local rules into `CLAUDE.local.md`. With `rules.mode = "inline"`, only path-scoped local rules get a file. When `antigravity` and `gemini` are both configured and no `mode_by_preset` is set, Antigravity keeps all local rules in `ai-rulez.local.md`.

`AGENTS.override.md`:

- It repeats the `AGENTS.md` body of the same run and appends the local sections, because Codex reads it instead of `AGENTS.md`. Do not edit it: it is rebuilt on every `generate` and removed when the local content goes away.
- When local content exists but no `AGENTS.md` is generated, it is not written, and `generate` warns.
- One without a generated banner that is not in the local manifest is yours: it is never overwritten or deleted, and `generate` warns that Codex reads it instead of your local content.

A local rule named `ai-rulez` would collide with the `ai-rulez.local.*` file of `junie`, `antigravity` and `copilot`, so it is written as `ai-rulez-<hash>.local<ext>`.

### Bookkeeping

- **Git ignore.** The overlay, the `local/` tree and the local manifest are always ignored, whatever `gitignore` says; with `gitignore = true` the managed block lists them on every machine. See [`gitignore`](configuration.md#gitignore).
- **Per-clone excludes.** The outputs your local inputs add (`CLAUDE.local.md`, `<rulesdir>/*.local.*`, `AGENTS.override.md`, a local skill, a `.claude/settings.json` only the overlay's hooks create, the files of an overlay-defined preset) go to a block of `.git/info/exclude`, so the shared `.gitignore` does not change with each contributor's files. Outside a git repository they go to the managed `.gitignore` block. `clean`, or a run with no local inputs left, removes the block.
- **Symlinked `.gitignore`.** Git does not read a `.gitignore` that is a symbolic link, and ai-rulez never writes through one: every ignore entry goes to `.git/info/exclude` instead.
- **Fail closed.** `generate` stops, listing the paths, when a machine-local or secret-bearing output, the overlay or the `local/` tree would not be ignored after the entries are written (for example because of a `!.ai-rulez/local/` line). Narrow the rule, or use `--no-local`.
- **Local manifest.** Local outputs are tracked in `.ai-rulez/.generated-manifest.local.json`, not in the committed manifest, so a teammate's `generate` never deletes your local files, and `clean`, or deleting the source under `.ai-rulez/local/` and regenerating, removes them. It also records what ai-rulez merged into settings documents shared with you.
- **Permissions.** The overlay, and every generated file holding a resolved MCP secret, is written `0600`.
- **Reserved names.** `*.local.*` in a rules folder is reserved; a hand-written file with such a name is skipped with a warning.

### Settings documents shared with you

`.claude/settings.json`, `.gemini/settings.json`, `opencode.json`, `.mcp.json`, `.agents/settings.json` and `.xum/mcp.jsonc` can hold your settings beside what ai-rulez writes. For each document ai-rulez records the MCP servers, array elements and scalar keys it merged in, each with a digest of the value it wrote (never the value). Array elements are counted per value: if ai-rulez added one `Bash(git status)` rule, `clean` removes one, and an identical rule you wrote stays. A document ai-rulez wrote whole is recorded in the committed manifest; a document shared with you, or one carrying overlay content, in the local manifest.

`clean`, and `generate` once a preset, server or overlay entry is gone, take back exactly those entries while they still hold the value ai-rulez wrote; an entry you edited stays, with a warning. `generate --dry-run` lists these as `unmerge:` lines. The general ownership rules are in [Shared settings documents](settings.md#shared-settings-documents); what follows is specific to the local record.

- **No record** (a fresh clone, or a document merged before records existed). On `clean` only, ai-rulez removes what is identical to what it would write: a configured MCP server with the rendered value, the ai-rulez self entry, a `context.fileName` in one of its forms, `AGENTS.local.md` in `instructions`, and an OpenCode `$schema` that is the only key. A hand-written value that happens to be identical is removed too. A server dropped from the configuration before any run recorded it stays; delete it by hand.
- **Gemini `context.fileName`** is written whether or not local content exists, so Gemini loads `GEMINI.local.md` once it appears:

  | Your value | Result |
  | --- | --- |
  | One of ai-rulez's forms (`["AGENTS.md"]`, `["AGENTS.md", "GEMINI.local.md"]`, `["GEMINI.md", "GEMINI.local.md"]`) | Rewritten to the current form. |
  | Anything else | Kept (a string becomes a list); `GEMINI.local.md`, and `AGENTS.md` under `agents_md`, appended when missing. `clean` takes back only those names. |

  With `agents_md` off, a list without `GEMINI.md` gets a warning, because Gemini then ignores the generated file.
- **OpenCode.** `AGENTS.local.md` is appended to your `instructions` (`./AGENTS.local.md` counts as the same entry) and taken back by `clean`. `$schema` is written only into an `opencode.json` ai-rulez creates.
- **Comments (JSONC).** A document with comments or trailing commas is not rewritten. When only the `context.fileName` or `instructions` entry would be written, `generate` warns and the local file is not loaded until you add it by hand. When MCP servers must be written into such a document, `generate` stops with a hint. `clean` leaves it alone with a warning. `.xum/mcp.jsonc` is JSONC by definition: once you add a comment, the servers ai-rulez merged stay after `clean`, with a warning; remove them by hand.

### Collisions

- A local rule with the name of a shared rule is allowed; its file is `<id>.local<ext>`, so nothing is overwritten.
- Two local rules that map to the same file name are written as `<id>-<hash>.local<ext>`.
- A local skill, agent or command with the name of a shared one fails `generate`, naming both files.

## Config overlay

`config.local.toml` sits next to `config.toml` and is merged onto it in memory at load time, after every [layer](configuration.md#layered-configuration). It is never written into the shared configuration, and plugin bundles (`generate --plugin`) ignore it. In a [root `ai-rulez.toml` layout](config-without-folder.md) the overlay is `ai-rulez.local.toml`.

Create one with `ai-rulez local init`, or let `local set`, `--local` or the MCP `local: true` flag create it. The overlay is validated against `schema/ai-rules-local.schema.json` (see [Schema](schema.md)), and the merged result must be a valid configuration. Only TOML is read: a `config.local.yaml`, `.yml` or `.json` fails the load; see [Migrating to v5](migration-v5.md).

```toml
# .ai-rulez/config.local.toml
presets = ["codex"]              # add codex on this machine
default = "dev"

[profiles]
dev = ["backend", "my-local-domain"]

[[mcp_servers]]
name = "github"                  # merges onto the shared "github" server
transport = "stdio"

[mcp_servers.env]
GITHUB_TOKEN = "${GITHUB_TOKEN}"   # see Environment references below

[[includes]]
name = "team-rules"
remove = true                    # drop a shared include on this machine
```

`${GITHUB_TOKEN}` resolves at `generate` time from `--env`, the environment or `.env`. A tool that expands references itself, such as a reader of `.mcp.json`, gets the reference `${GITHUB_TOKEN}` instead of the value; see [Environment references](configuration.md#environment-references).

!!! warning "Add presets, do not drop shared ones"
    The overlay is meant to add. Dropping a shared preset (`presets = ["!cursor"]`) currently deletes that preset's committed outputs (`.cursor/mcp.json`, ...) and rewrites the committed `.ai-rulez/.generated-manifest.json`; the [drift guard](#drift-guard) does not stop it. An added preset also records its MCP entries in the committed manifest. Do not commit those changes; restore them with `git checkout`, or run `generate --no-local` before committing.

### Merge semantics

| Key kind | Keys | Rule |
| --- | --- | --- |
| Scalars | `name`, `description`, `default`, `gitignore`, `compact`, `source`, `agents_md`, `codex_skills_dir`, ... | The local value wins. |
| Lists | Any list not named below (`skill_sources`, `bundle_exclude`, `skip_outputs`, `hooks`, the domain lists of `profiles`, ...) | The local list replaces the shared one. |
| Tables | `profiles`, `header`, `defaults`, `mcp`, `rules`, `claude`, `lint`, `lock`, `permissions`, `guard`, `session_start`, `role_manifest`, `delivery`, `placement`, `plugin`, `catalog`, `llm`, `verifiers_settings` | Merged per key, recursively; lists inside are replaced. A local value that is not a table (including `null`) is an error. |
| Other tables | `codex`, `domains`, `skills`, `okf`, `telemetry`, `usage`, `exposure`, `search`, ... | Merged per key, recursively; a local value that is not a table replaces the shared one. |
| `marketplace` | | Merged per key; `[[marketplace.plugins]]` merges entry by entry on `name`. |
| `presets` | | Ordered union: local entries are appended and duplicates collapse (a table beats a bare name; two tables merge, the local one winning). `"!name"` drops a shared preset. |
| Named lists | `mcp_servers`, `includes`, `installed_skills`, `installed_marketplaces`, `installed_plugins`, `plugins`, `marketplaces`, `scopes`, `roles`, `verifiers`, `plugins_authoring`, `marketplaces_authoring` | Entries merge by `name` (`scopes` by `path` when unnamed). A local entry with `remove = true` deletes the shared one; a new name is appended. |
| `builtins` | | The local value replaces the shared one. |
| `version` | | Must be a string equal to the shared version, if set. |
| `governance`, `signing`, `default_layers` | | Ignored with a warning: they are shared policy. |

Other rules:

- Unknown keys are an error. Error messages name keys and entry positions, never values.
- `validate` reports a literal credential in an MCP `env` or header value as `AR015`, in the overlay as in `config.toml`. Reference an environment variable (`"${GITHUB_TOKEN}"`) and keep the value in your shell or `.env`.
- Dropping a preset, or removing an entry, that the shared configuration does not have is a warning.
- `remove` is not valid on preset tables; use `"!name"`.
- Every named-list entry needs a `name` (a `scopes` entry a `name` or `path`). A duplicate local entry, or one that matches several shared entries, is an error.
- A local include or `local_override` may point outside the project; in the committed `config.toml` it may not. An overlay that is tracked by git gets the same check as `config.toml`, with a warning.
- The `[llm]` network and credential keys and the `[telemetry]` export keys are honored only from the user configuration, never from `config.toml` or the overlay. See the [trust model](trust-model.md).

## Managing local configuration

### `ai-rulez local`

| Command | Action |
| --- | --- |
| `local init` | Create a commented `config.local.toml` skeleton |
| `local show` | Print every key the overlay sets and the shared value it replaces (`--format json`, `--reveal`) |
| `local set <path> [value]` | Set a key (`--string`, `--stdin`) |
| `local unset <path>` | Remove a key |
| `local path` | Print the overlay path |

```bash
ai-rulez local set default dev
ai-rulez local set presets '["codex", "!cursor"]'
ai-rulez local set mcp_servers.github.command npx
printf %s "$TOKEN" | ai-rulez local set mcp_servers.github.env.GITHUB_TOKEN --stdin
ai-rulez local set 'mcp_servers["foo.bar"].command' npx
ai-rulez local show --format json
```

- **Values** are parsed as TOML literals, falling back to a plain string. Env and header values and text fields (`url`, `command`, `source`, `path`, `ref`, `description`, `name`, `transport`, `default`, `*_version`) are always strings. `--string` forces a string.
- **Secrets** go through `--stdin` (stored as a string), so they stay out of shell history.
- **Dotted names**: write a segment that contains a dot in brackets and double quotes, `mcp_servers["foo.bar"].command`.
- **Redaction**: `local show` prints values only for keys known to hold no credentials (`name`, `description`, `default`, `presets`, `gitignore`, `compact`, `builtins`, `profiles.*`, `defaults.effort*`, `defaults.omit_agent_fields`, `rules.mode*`, `header.style|hashes|timestamp`, `mcp.self_server`, `mcp.self_server_version`, and `transport`, `enabled`, `remove` of list entries), and only when the value has the expected type. Every other key shows `<redacted>`; `--reveal` prints everything.
- **Safety**: every write takes a file lock, writes the ignore entries first and validates the merged configuration; on failure the previous file is restored.
- The subcommands honour the global `--config` and `--config-dir`.

`profile list`, `include list` and `skill list` show the shared configuration and print how many entries the overlay adds; `local show` lists them.

### `--local` on other commands

`--local` on CLI commands, and `local: true` on the MCP tools, send a change to the local layer:

| Target | Commands |
| --- | --- |
| `.ai-rulez/local/` | `add`, `remove`, `list`, `show` and `edit` of `rule`, `context`, `skill`, `agent`, `command` (create `local/checks/` files by hand) |
| Overlay | `profile add\|remove\|set-default`, `include add\|remove`, `skill install\|remove` |

```bash
ai-rulez add rule local-paths --local                   # .ai-rulez/local/rules/local-paths.md
ai-rulez add skill my-debug-skill --local
ai-rulez add rule team-notes --local --domain backend   # .ai-rulez/local/domains/backend/rules/
ai-rulez profile add dev backend my-local-domain --local
ai-rulez include add scratch ../scratch-rules --local
```

- `--local --domain <name>` writes to `.ai-rulez/local/domains/<name>/`; the local domain exists once it has content. `domain add|remove` and the MCP domain tools have no local form.
- Removing a shared include or installed skill with `--local` writes `remove = true` to the overlay. A shared profile cannot be removed locally.

## Drift guard

The overlay can change files the team shares (an overlay MCP server adds an entry to `.mcp.json`, for example). To keep local values out of tracked files, `generate` also renders the shared view and classifies every output:

| Class | Meaning | Result |
| --- | --- | --- |
| local-only | Only the merged view produces the file | Written; kept out of git |
| drift | Both views produce the file, with different content | Written only if the file is git-ignored and untracked |
| suppressed | Only the shared view produces the file (the overlay dropped a preset or entry) | Removed, even when it is tracked (see the [warning](#config-overlay) on dropping presets) |

The run is blocked, writes nothing and exits non-zero when a drift file is tracked or not ignored, when a local-only file is tracked, or when git cannot say which files are tracked. The error names paths only, never content. `generate --dry-run` prints the plan and exits non-zero when it would be blocked:

```text
local-only: .claude/rules/scratch.local.md
drift: .mcp.json
allowed: .gemini/settings.json (drift, but git-ignored)
suppressed: .cursor/mcp.json
blocked: .mcp.json (shared output is tracked and would change)
```

Ways forward:

- Git-ignore the files, or untrack them.
- `generate --no-local` generates the shared view only, the one a teammate sees.
- `generate --allow-local-drift` writes anyway, which can put non-secret overlay values into tracked files. A generated MCP file that would carry a resolved secret is still refused unless it is git-ignored. The flag is CLI-only; the MCP server cannot bypass the guard.

If the shared view itself fails to render, the run fails: without it, local outputs cannot be told apart from shared ones. This happens, for example, when the overlay points a remote include at a local checkout while `ai-rulez.lock` is stale for it. `--allow-local-drift` skips lock enforcement for the includes the overlay replaces, with a warning, so you can try an include change from a local checkout before the lock is updated. Any other failure of the shared view needs a fix to the shared configuration, `ai-rulez lock`, or `--no-local`.

`Source-Hash` values in shared outputs come from the shared view, so a teammate regenerating sees no hash change; local-only outputs carry a hash of their local inputs, with the overlay entering it only in redacted form.

## `--no-local`

`--no-local` ignores both layers. `generate`, `validate`, `scan`, `doctor`, `tokens`, `cost`, `catalog`, `config layers`, `roles`, `verifiers`, `review`, `llm` and the other commands that load the project accept it; `verify` always checks the shared view. A `--no-local` run neither deletes your local files nor removes their ignore entries. Use it in hooks and CI so results do not depend on one machine's overlay; see [Git hooks](poly-hooks.md).

## Security notes

- The overlay and the local tree are written owner-only, and their ignore entries are written before the files; writers fail closed if the entries cannot be written.
- `local show` withholds values by default, and the MCP `read_config` tool returns the overlay's key paths only.
- The overlay enters generated headers only in redacted form: env and header values and arguments are replaced, and URLs lose their credentials, query and fragment.
- Do not use `--allow-local-drift` in shared scripts.

## See also

- [Configuration reference](configuration.md#local-overlay)
- [CLI Commands](cli.md#local-configuration): `ai-rulez local` and `--local`
- [MCP Server](mcp-server.md): the `local` tool parameter
- [Rules](rules.md): rules mode and rule files
