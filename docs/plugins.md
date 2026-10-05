# Authoring Plugins

`ai-rulez generate --plugin` packages your `.ai-rulez/` project into distributable
**plugin bundles** and a **marketplace index** for the Claude, Cursor, Codex, Gemini,
Kimi, OpenCode, Factory, and Hermes runtimes, plus the opt-in Agent Plugins 1.0.0 standard.
Where the normal `generate` writes in-repo
assistant config, `--plugin` produces installable artifacts other people can add to
their own tools — reaching users who never run ai-rulez.

Your skills, commands, and agents are the payload; the `[plugin]` block adds the
packaging metadata.

!!! note "Authoring vs. installing"
The `[plugin]` / `[marketplace]` blocks documented here are the **producer**
(authoring) side. They are distinct from the `[[plugins]]` / `[[marketplaces]]`
arrays, which are the **consumer** side. `[[plugins]]` is rendered into
`.claude/plugins.json` and `.codex/plugins.json`; `[[marketplaces]]` is currently
recorded only and not emitted. See
[Consumer plugins](configuration.md#plugins) for the fields.

## Quick start

```toml
# .ai-rulez/config.toml
version = "4.0"
name = "my-tool"
description = "My project."

[plugin]
name = "my-tool"
description = "What my tool does."
version = "1.0.0"
license = "MIT"
homepage = "https://github.com/me/my-tool"
keywords = ["mcp", "tooling"]

[plugin.author]
name = "Jane Doe"
email = "jane@example.com"

[[plugin.mcp]]
name = "my-tool"
command = "${PLUGIN_ROOT}/scripts/mcp-launch.sh"
args = ["serve"]
```

```bash
ai-rulez generate --plugin          # write the bundles
ai-rulez generate --plugin --dry-run  # preview what would be written
```

## What gets generated

| Runtime  | Manifest                                                | Notes                                                            |
| -------- | ------------------------------------------------------- | ---------------------------------------------------------------- |
| Claude   | `.claude-plugin/plugin.json`                            | skills/commands/agents auto-discovered from top-level dirs       |
| Cursor   | `.cursor-plugin/plugin.json` (+ `hooks/hooks.json`)     | bundles its own `skills/`, `commands/`                           |
| Codex    | `.codex-plugin/plugin.json` (+ `.mcp.json`)             | MCP referenced via external file; rich `interface` block         |
| Gemini   | `gemini-extension.json`                                 | inline MCP + hooks, context file reference                       |
| Kimi     | `kimi.plugin.json`                                      | `sessionStart`, `skillInstructions`, `interface`                 |
| OpenCode | `.opencode/plugins/<plugin-name>.js` (+ `package.json`, `.opencode/ai-rulez-content.js`, bundled `.opencode/{skills,commands,agents}/`) | OpenCode v2 `{ id, setup }` adapter; copies the authored entrypoint or emits a scaffold that registers the bundled content |
| Factory  | `.factory-plugin/plugin.json`                           | metadata-only                                                    |
| Hermes   | `.hermes/plugins/<plugin-name>/` and `.hermes/package/` | project plugin plus buildable Python entry-point package         |
| Agent Plugins | `plugin.json`, `skills/`, `mcp.json`               | portable [Agent Plugins 1.0.0](https://agent-plugins.org) package; opt-in |
| Copilot  | `plugin.json`, `skills/`, `mcp.json`, `com.github.copilot/agents/<name>.agent.md`, `.github/plugin/marketplace.json` | GitHub Copilot plugin in the Agent Plugins 1.0 layout; opt-in |

The **marketplace index** (`.claude-plugin/marketplace.json`) is emitted alongside when `claude` is
among the bundle's runtimes. (A monorepo root also emits a Codex index at
`.agents/plugins/marketplace.json`; see the monorepo section.) The `copilot` runtime writes its own
index, and the Codex and Cursor indexes for a single-plugin repository are opt-in, see below.

### Runtime coverage

What each runtime bundles, checked against the vendors' documentation on 2026-10-04. A dash means
ai-rulez does not emit it: either the runtime has no such component, or the vendor documents no file
format for it (ai-rulez does not guess formats).

| Runtime | Skills | Commands | Agents | Hooks | MCP | Marketplace index | Vendor documentation |
| ------- | :----: | :------: | :----: | :---: | :-: | :---------------: | -------------------- |
| Claude | yes | yes | yes | yes | yes | yes | [plugins](https://code.claude.com/docs/en/plugins), [marketplaces](https://code.claude.com/docs/en/plugin-marketplaces) |
| Cursor | yes | yes | - | yes | manifest | opt-in (`[plugin.cursor] marketplace`, `[marketplace] cursor_index`) | [plugins](https://cursor.com/docs/plugins), [reference](https://cursor.com/docs/reference/plugins) |
| Codex | yes | - | - | - | yes | monorepo roots; single plugin opt-in (`[plugin.codex] marketplace`) | [build plugins](https://developers.openai.com/codex/plugins/build) |
| Copilot | yes | - | yes | - | yes | yes (`.github/plugin/marketplace.json`) | [CLI plugin reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference) |
| Gemini | - | opt-in (`[plugin.gemini] commands`) | - | yes | yes | - | [extension reference](https://geminicli.com/docs/extensions/reference/), [custom commands](https://geminicli.com/docs/cli/custom-commands/) |
| Agent Plugins | yes | - | - | - | yes | - | [agent-plugins.org](https://agent-plugins.org) |

Kimi, OpenCode, Factory and Hermes are unchanged. Known gaps, all because the format is undocumented
or outside the runtime: Codex and Copilot commands and hooks, Copilot rules and LSP, Gemini agents,
policies and `hooks/hooks.json` (hooks are inlined in `gemini-extension.json`), Cursor
agents/rules.

### Codex: root `plugin.json`

Codex documents a root `plugin.json` in the Agent Plugins format as the preferred manifest for new
packages and keeps `.codex-plugin/plugin.json` as a compatibility fallback. The Codex `interface`
block moves under `extensions.com.openai`, skills live in the fixed `skills/` directory and MCP servers
in a root `mcp.json` whose entries need a transport `type` (a renamed `.mcp.json` is not enough).
The default stays the legacy layout so existing bundles do not change:

```toml
[plugin.codex]
manifest = "root"      # legacy (default) | root | both
marketplace = true     # also write .agents/plugins/marketplace.json for this single-plugin repo
```

`both` writes the two manifests side by side. The root layout needs an Agent Plugins plugin name
(lowercase alphanumerics, `-` and `.`). When `codex` and `agent-plugins` (or `copilot`) are bundled
together they share one root `plugin.json`; the Codex overlay is only written when the `codex` runtime
is part of the bundle. The single-plugin Codex index uses `source = { source = "local", path = "./" }`.
Codex documents that a marketplace may list one plugin and that paths start with `./` and stay inside
the marketplace root; `./` itself as the plugin path was not exercised against a running Codex.

### Copilot

`runtimes = ["copilot"]` (opt-in) emits the Agent Plugins 1.0 layout that Copilot CLI and the cloud
agent read: a root `plugin.json` with the standard's `$schema`, `skills/<name>/`, a root `mcp.json`,
custom agents at `com.github.copilot/agents/<name>.agent.md` (copied verbatim) and a single-plugin
marketplace at `.github/plugin/marketplace.json` (`name`, `owner`, `metadata`, `plugins[]`, which
Copilot searches after a root `marketplace.json`). Copilot lists `com.github.copilot/commands/` and
`com.github.copilot/hooks/hooks.json` but documents neither file format, so commands and hooks are not
emitted and a warning says so. Enabling plugins (`enabledPlugins` in `.github/copilot/settings.json`) is
not generated: the vendor page does not show the settings syntax.

### Cursor marketplace

Cursor documents `.cursor-plugin/marketplace.json` for multi-plugin repositories (`name`, `owner.name`,
optional `metadata`, `plugins[]` with `name` and a relative `source` directory). It is opt-in:
`[plugin.cursor] marketplace = true` for a single-plugin repository (`source` is `.`), and
`[marketplace] cursor_index = true` next to the Claude index for members and domain plugins.

### Gemini commands

`[plugin.gemini] commands = true` also bundles each command as `commands/<name>.toml` (a required
`prompt`, an optional `description`); the Claude-style `$ARGUMENTS` becomes Gemini's `{{args}}`. Shell
(`!{...}`) and file (`@{...}`) injections are not translated.

Content files (SKILL.md, commands, agents) are copied **verbatim** from your source
into each runtime's directories — never re-rendered — so a bundled skill is identical
to the authored one.

### OpenCode adapter

The adapter targets OpenCode v2, whose plugin API differs from v1: a plugin default-exports
a definition `{ id, setup(ctx) }` (`Plugin.define` from `@opencode/plugin` is the identity
helper for it), and v1 plugins (an exported function that returns hooks) do not run. Put
OpenCode-specific tools and hooks in `.ai-rulez/opencode/index.js`. The generator copies that
source verbatim to `.opencode/plugins/<plugin-name>.js`, generates the `package.json` metadata
(`main` and `exports` point at that file; it depends on `@opencode/plugin`), and bundles the
plugin's skills, commands, and agents under `.opencode/`.

OpenCode only scans its own config directories, so it never discovers the `.opencode/skills`,
`commands` and `agents` of an npm-installed plugin. The generated entrypoint therefore calls
`registerBundledContent(ctx)` from the generated `.opencode/ai-rulez-content.js`, which
registers the bundled content through the skill, command, agent and MCP transforms. The helper
reads `.opencode/ai-rulez-bundle.json`, which holds the plugin's MCP servers and the agent
settings resolved at generation time.

- **MCP servers** become OpenCode `local` (command array plus `environment`) or `remote`
  (`url`) servers. At runtime `${PLUGIN_ROOT}` expands to the installed plugin directory and
  `${NAME}` to `process.env.NAME` (empty when unset), so no install path or secret is written
  into the package. Remote `headers` are not bundled, as for the other runtimes. Any top-level
  source path an MCP server references as `${PLUGIN_ROOT}/<path>` (command, args, env or url),
  such as `scripts/` for `${PLUGIN_ROOT}/scripts/run.sh`, is added to the generated
  `package.json` `files` list when it exists in the plugin source; a missing one is warned
  about and not listed. The other runtimes publish the repository tree or have no file list, so
  they have no equivalent gap.
- **Agents** take their model through the same resolution as the `opencode` preset
  (`opencode_model`, then `defaults.model_by_preset.opencode`, then `model`). A
  `provider/model` id passes through, and `#variant` (or the effort tier) becomes the model
  variant. A bare alias such as `sonnet` is omitted with one warning so the agent inherits the
  session model. `description`, `mode` (default `all`), `hidden`, `temperature` and `top_p` are
  mapped too; `tools` and `permission` are not carried by ai-rulez agents and so are not mapped. If you supply
your own `.ai-rulez/opencode/index.js` and the plugin bundles content, call it from `setup`:

```js
import { registerBundledContent } from "../ai-rulez-content.js";

export default {
  id: "my-plugin",
  async setup(ctx) {
    await registerBundledContent(ctx);
  },
};
```

The generated entrypoint does not import `@opencode/plugin` at runtime: OpenCode installs no
dependencies for local `.opencode/plugins/` files, so the import would fail to resolve. Keep
shared MCP settings in their normal `.ai-rulez` sources; the `opencode` preset writes them to
`opencode.json`.

When the source entrypoint is absent, generation emits a documented module that only registers
the bundled content. It keeps the plugin loadable and tells you where to create the user-owned
source; it does not guess tool schemas, subprocess arguments, or business logic.

#### Using the generated plugin

Checked against OpenCode 2.0.20 with a local server. These routes load the plugin; `<name>` is the
plugin name and `<bundle>` the directory `generate --plugin` wrote into.

1. **Copy it into a project.** OpenCode loads every plugin in a project's `.opencode/plugins/`.
   From `<bundle>`, copy into the consuming project:
   `.opencode/plugins/<name>.js`, `.opencode/ai-rulez-content.js`, `.opencode/ai-rulez-bundle.json`,
   `.opencode/skills/`, `.opencode/commands/` and `.opencode/agents/` (keeping those paths), plus every
   top-level directory an MCP server reaches through `${PLUGIN_ROOT}` (for example `scripts/`) at the
   project root: `${PLUGIN_ROOT}` is the parent of the `.opencode` directory that holds
   `ai-rulez-content.js`. Restart OpenCode or reload its configuration.
2. **Point `opencode.json` at a directory.** Add `"plugins": ["<dir>"]` (an absolute path, a path
   starting `./` or `../`, or a `file://` URL) where `<dir>` is a copy of `<bundle>` that also has
   an `index.js` at its root:

   ```js
   export { default } from "./.opencode/plugins/<name>.js";
   ```

   OpenCode loads the directory's root `index.js`. It ignores `main` and `exports` of the generated
   `package.json`, so a bare `<bundle>` is not loaded, and a path to a file is refused
   (`configured plugin path must be a directory`).
3. **Put it in a project as a directory plugin.** Copy the bundle's files as in route 1, but
   save the entrypoint as `.opencode/plugins/<name>/index.js` and change its helper import to
   `../../ai-rulez-content.js`.

The generated `package.json` (`main`, `exports`, `files`) exists for publishing to npm. Installing
from npm was not exercised for this release, so follow the OpenCode plugin documentation for that route.

Two properties of the bundled MCP servers are worth knowing before you install a third-party plugin:

- `${NAME}` in a server's command, arguments, environment or URL is expanded from the OpenCode
  process environment when the plugin loads. A plugin runs inside that process and can read all of its
  environment variables, so install only plugins you trust and do not treat `${VAR}` as a boundary.
- Remote `headers` are never bundled (generation warns), and a project server with `enabled = false`
  is carried into the OpenCode bundle as `disabled: true`. The other runtimes have no such flag in
  their bundles, so a disabled server is bundled enabled for them and generation warns about it.

#### v1 plugin warning and migration

OpenCode v2 does not run v1 plugins, and it only logs the refusal to its server log
(`Plugin must export a default definition with an id and an effect or setup function`). ai-rulez
therefore warns, once per file and without failing generation, when it finds a v1-shaped plugin:

- the authored `.ai-rulez/opencode/index.js` while generating a plugin bundle;
- any file in the project's `.opencode/plugin/` or `.opencode/plugins/` directories, and any
  local path in the `plugin`/`plugins` array of `opencode.json(c)` (npm package names are never
  fetched), while generating the `opencode` preset.

A file counts as v1 when it default-exports a function, exports an `async` function or arrow
function, or imports the v1 `@opencode-ai/plugin` package, and has no v2 marker
(`Plugin.define(...)`, or an `id` property, written with any value or shorthand, together with
`setup`/`effect`). Comments are removed before the check, but text inside string literals is kept,
so a glob such as `"src/**/*.js"` does not hide the code after it. To migrate, rename `plugin`
to `plugins` in the config, move the file to `.opencode/plugins/`, and replace the exported
function with a default export `{ id, async setup(ctx) { ... } }` that registers hooks and
transforms on `ctx`. See the official
[V1 plugin migration guide](https://opencode.ai/v2/docs/build/plugins/migrate-v1) and the
[plugins guide](https://opencode.ai/v2/docs/build/plugins).

The `opencode` preset itself is separate from plugin authoring. It emits `AGENTS.md`,
`.opencode/skills/`, and `.opencode/agents/`, and — only when `[[mcp_servers]]` are configured — a
native `opencode.json` owning `$schema` and the `mcp.<name>` entries, merged so any other keys in a
hand-authored `opencode.json` are preserved.

### Hermes adapter

Put Hermes-specific registrations in `.ai-rulez/hermes/index.py`. The generator
copies it to `.hermes/plugins/<plugin-name>/hermes.py`, writes a compatible
`__init__.py` plus `plugin.yaml`, and
bundles the plugin's skills, commands, and agents. The same adapter and content are
packaged under `.hermes/package/src/<name>_hermes_plugin/`; its generated
`pyproject.toml` publishes as `<name>-hermes-plugin`. If the source is absent, it emits
a documented no-op `register(ctx)` scaffold. Hermes project plugins are trusted code
and require `HERMES_ENABLE_PROJECT_PLUGINS=true`.

To reuse another canonical adapter instead of `.ai-rulez/hermes/index.py`, configure
a safe project-relative source:

```toml
[plugin.hermes]
source = "plugin/hermes.py"
```

By default, plugin payload comes from the resolved ai-rulez content tree. To keep
distributable content separate from developer governance, set a safe project-relative
content root containing `skills/`, `commands/`, and `agents/`:

```toml
[plugin]
content_root = "plugin"
```

### Generated-file provenance

Generated JavaScript, TypeScript, Python, and Markdown files include an ai-rulez warning plus
deterministic BLAKE3 `Content-Hash` and `Source-Hash` values. Markdown headers follow
YAML frontmatter so skill discovery remains valid.

Run `ai-rulez verify --plugin` to verify every output recorded by the provenance
sidecar without modifying or regenerating the package.

Regeneration compares each bundle's previous provenance inventory with its new
outputs. Obsolete generated files are removed only when their bytes still match
the recorded generated version; unrelated hand-written files are preserved.
Empty parent directories of removed files are pruned up to the bundle root.
`generate --plugin --dry-run` lists these removals as `delete-stale:` without
changing files. If an obsolete file was edited, generation stops before writing
and reports its path. Move or remove that file explicitly, then regenerate.
Malformed provenance or symlinked obsolete paths also stop generation safely.

Reusable hook catalogs can run plugin checks across mixed repositories with
`ai-rulez generate --recursive --plugin --if-configured` and
`ai-rulez verify --recursive --plugin --if-configured`. Both commands exit successfully
without doing work when the configuration has neither a producer `[plugin]`
block nor a `[marketplace]` block with members.

Recursive commands treat a marketplace root as one producer. The root processes
its members and aggregate indexes; member configs are not processed again in
parallel. Standalone `[plugin]` producers are still processed individually.
Consumer-only `[[plugins]]` and `[[marketplaces]]` declarations are skipped.
`--if-configured` does not suppress generation or verification errors.

See [Poly Hooks](poly-hooks.md) to add non-mutating plugin verification to a
repository's pre-commit stage.

Strict JSON manifests and binary assets cannot safely contain comments. Each plugin
therefore includes `.ai-rulez-generated.json`, which records the same source hash and
a sorted content-hash entry for every generated output. Monorepo roots include a
separate sidecar covering the aggregate marketplace files. Validators can recompute
these hashes without adding unsupported fields to runtime manifests.

### MCP launch variable

Write MCP launch commands once with the canonical `${PLUGIN_ROOT}`; each runtime
rewrites it to the form it expects — `${CLAUDE_PLUGIN_ROOT}` (Claude),
`${extensionPath}` (Gemini), plugin-relative `./` (Cursor, Codex, Kimi, Agent Plugins), or
the canonical variable unchanged (OpenCode, Factory).

### Hooks

Declare lifecycle hooks once; they render into each runtime's hook format with the
correct root variable (`${CURSOR_PLUGIN_ROOT:-.}` for Cursor hooks, and so on).

Each hook action requires exactly one of `command` or `script`:

- **`command`**: An executable that already exists in the consumer's environment. Passed through verbatim.
- **`script`**: A project-relative file ai-rulez bundles into the plugin's `hooks/` directory. The bundled script is addressable as `${PLUGIN_ROOT}/hooks/<basename>` and survives a fresh clone before generation has run. Use `script` for self-contained bootstrap hooks.

Additional fields:

- **`args`**: Array of arguments passed to the command/script. When set, the runtime spawns the executable directly (no shell).
- **`timeout`**: Handler timeout in seconds; zero uses the runtime default.
- **`async`**: Whether the handler runs asynchronously (default: false).
- **`if`**: Restricts the handler to matching tool calls, in permission-rule syntax — one rule such as `"Bash(git *)"` or `"Edit(*.ts)"`, with no boolean operators and no expression language. Claude Code evaluates it **only** on `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest` and `PermissionDenied`; on any other event a handler carrying `if` never runs at all, so it cannot be used to make a bootstrap hook conditional. `validate` warns when `if` appears on an event that ignores it.
- **`status_message`**: Message shown to the user while the handler runs (useful for blocking bootstrap scripts). Rendered as `statusMessage` in the runtime's hook file.

Example with bundled script. A bootstrap hook decides for itself whether its work is already done — there is no declarative guard for `SessionStart`:

```toml
[[plugin.hooks]]
event = "SessionStart"
matcher = "startup"

[[plugin.hooks.hooks]]
script = "scripts/bootstrap.sh"
args = ["--check"]
timeout = 30
status_message = "Bootstrapping plugin on first use..."
async = false
```

Example with external command:

```toml
[[plugin.hooks]]
event = "PreToolUse"

[[plugin.hooks.hooks]]
command = "my-tool validate"
async = true
```

Event names must match a Claude Code lifecycle event (`SessionStart`, `Setup`, `PreToolUse`, `PostToolUse`, etc.). An unknown event produces a warning rather than an error, so a config written for a newer Claude Code keeps working on an older ai-rulez. Events in `HookEventsWithoutMatcher` (such as `UserPromptSubmit`, `PostToolBatch`, `Stop`) have no matchable subject, so a declared matcher is silently ignored at runtime.

### Restricting runtimes

```toml
[plugin]
# ...
runtimes = ["claude", "cursor"]   # omit to emit all supported runtimes
```

The `agent-plugins` runtime is **opt-in** and is not part of the default set, so
adding it never changes existing bundles. Enable it explicitly:

```toml
[plugin]
runtimes = ["agent-plugins"]
```

It emits a root `plugin.json`, a root `skills/` directory, and (when the plugin
declares MCP servers) a root `mcp.json`, matching the Agent Plugins 1.0.0
standard. The plugin `name` must satisfy the standard's naming rules (1–64
lowercase alphanumerics/`-`/`.`, alphanumeric ends, no `--` or `..`). Commands,
agents, hooks, and marketplaces are outside Agent Plugins v1 and are not emitted
for this runtime.

## Single plugin vs. monorepo

A repo with one `[plugin]` block publishes a **single-plugin** marketplace whose sole
entry has `source: "./"`.

For a **monorepo** hosting several plugins, the root config declares only a
`[marketplace]` with `members`; each member is its own ai-rulez project:

```toml
# root .ai-rulez/config.toml
[marketplace]
name = "acme"
description = "Curated Acme plugins."
members = ["plugins/alpha", "plugins/beta"]

[marketplace.owner]
name = "Acme Inc"
email = "dev@acme.example"
```

Each member (`plugins/alpha/.ai-rulez/config.toml`, …) defines its own `[plugin]`
block. `generate --plugin` renders every member's bundle under its directory and
emits a single root marketplace index — `.claude-plugin/marketplace.json` (and the Codex
index at `.agents/plugins/marketplace.json`) — listing each with `source: "./plugins/<name>"`.
Members do not emit their own marketplace index.

## Domain content and per-domain plugins

A project that keeps its content in `.ai-rulez/domains/<name>/` can ship it as plugins
without hand-made member projects. Everything here is opt-in; a config without these keys
generates exactly what it did before.

### Bundle domain content into the `[plugin]` bundle

A `[plugin]` bundle holds the root skills, commands and agents only. List domains in
`include_domains` (names or globs) to bundle theirs too:

```toml
[plugin]
name = "acme-conventions"
version = "1.0.0"
include_domains = ["backend", "ui"]   # or ["*"]
```

On a name collision the root item wins, then the domain earlier in name order. Builtin
domains are never bundled.

### One plugin per domain

```toml
[marketplace]
name = "acme"
description = "Acme skill packs"
output_dir = "tools/acme-marketplace"   # default: the project root

[marketplace.from_domains]
name_prefix = "acme-"        # plugin "acme-backend" from domains/backend
exclude = ["scratch"]        # include defaults to every domain
version = "1.0.0"            # default: the [plugin] version, else 1.0.0
default_enabled = false      # marketplace entry defaultEnabled
runtimes = ["claude"]        # default: the [plugin] runtimes, else all
```

`generate --plugin` writes `<output_dir>/plugins/<name>/` for each domain (skills, commands
and agents, plus the runtime manifests and a provenance sidecar) and one
`<output_dir>/.claude-plugin/marketplace.json` whose entries use `source: "./plugins/<name>"`,
relative to the marketplace root as Claude Code requires. The Codex index
(`.agents/plugins/marketplace.json`) is written only when some plugin targets `codex`.
Domain names are lower-cased to form the plugin name; a name that still is not valid is an
error (use `exclude`). A domain without bundleable content is skipped with a warning.
`verify --plugin` checks the marketplace root and every plugin directory.

When a plugin's domain disappears (or its declaration is removed), the next
`generate --plugin` deletes the files it generated for that plugin and the directories they
leave empty. A directory counts as generated only when it carries ai-rulez's provenance
sidecar, so a hand-made directory under `plugins/` is never touched, and a file inside a stale
directory that ai-rulez did not generate is kept (with a warning). `generate --plugin --dry-run`
lists each removal as `delete-stale:`, and `verify --plugin` fails while a stale generated
directory exists. Removing the whole `[marketplace]` domain-plugin configuration cannot be
detected, because the output root is no longer known; delete that tree by hand.

#### Linked git worktrees

With `[claude.settings] manage = true` the generated `extraKnownMarketplaces` entry defaults to a
relative `directory` source (`./<output_dir>`). Claude Code's documentation does not describe how
such a source resolves in a linked worktree (`git worktree add`); the behaviour reported for it,
and assumed by the generator's own source comment, is that it resolves against the main checkout.
Vendor documentation that does cover worktrees says project-local settings follow the main
checkout, so treat edits to plugin content in a linked worktree as not visible to Claude Code until
they reach the main checkout, unless you point the marketplace somewhere absolute. `ai-rulez
validate` prints one warning when the directory source is relative and the project is in a linked
worktree (detected with `git rev-parse --git-dir` versus `--git-common-dir`). To test plugin changes
from a worktree, set `[claude.settings.marketplace_source]` to an absolute `directory` path or a
`github`/`git` source. The warning is advisory: nothing fails. This behaviour was not verified
against a running Claude Code; it is phrased as a caveat for that reason.

#### Placement report

`ai-rulez list --placement` (`--profile`, `--json`) prints every skill and command of the profile
with its destination: `core` (written to the assistants' own directories) or `plugin` (kept out of
them by `[placement]`), the domain it came from, and the plugins that bundle it. A plugin-only item
is flagged when no plugin bundles it, or when none of its plugins is enabled through
`[claude.settings] enable_plugins` and no `[marketplace.catalog_skill]` lists it, because then
nothing tells anyone the plugin exists.

#### Version drift

Claude Code's documentation says a client that installed a plugin from a git-hosted marketplace
keeps its cached copy until the plugin's version string changes (a plugin that declares no version
is tracked by commit, and a plugin loaded in place from a local marketplace is not controlled by
`version`). `ai-rulez validate --strict` therefore reports `AR961 plugin-version-drift` (warning)
when a generated plugin's content differs from its committed provenance sidecar at `HEAD` while the
version in its manifest is unchanged. Bump `version`, or omit it to track commits.

The `[plugin]` block is optional in this mode. When present it supplies defaults (author,
license, homepage, repository, version, category, keywords, runtimes) and is not bundled on
its own; declare a plugin for the root content with `[[marketplace.plugins]]` instead.
`owner` falls back to the `[plugin]` author. `members` and domain plugins may be combined,
but then `output_dir` must be unset.

Hand-declared plugins mix domains and root content, and replace a derived plugin of the
same name:

```toml
[[marketplace.plugins]]
name = "acme-essentials"
skills = ["development-standards", "personal-*"]   # root content, by name or glob
domains = []                                       # domains, by name or glob
default_enabled = true

[marketplace.plugins.relevance]                    # when Claude Code suggests the plugin
topic = "Terraform"
[marketplace.plugins.relevance.signals]
cli = ["terraform"]
files_read = ["**/*.tf"]
```

Entries carry `version`, `category`, `keywords`, `defaultEnabled` and `relevance`
(`cwd`, `cli`, `hosts`, `files_read`, `manifest_deps` signals, at least one). These are the
fields Claude Code documents for a marketplace entry; `claude plugin validate` accepts the output.
Monorepo `members` entries stay minimal.

### Keep a skill out of `.claude/skills`

By default every root and domain skill and command is written to `.claude/skills`. The
`[placement]` block moves some to plugin-only:

```toml
[placement]
default = "core"                          # core (today) or plugin
plugin = ["domains/*"]                    # names or globs; "domains/<domain>/<name>" also matches
core = ["domains/backend/python-conventions", "git-*"]
honor_targets = true                      # skills: apply frontmatter `targets`, as commands always do
```

An item's frontmatter `placement: core|plugin` wins, then `core`, then `plugin`, then `default`.
The key is removed from the generated frontmatter. Placement changes only the Claude preset's
`.claude/skills` output; plugin bundles always contain the item, so a plugin-only skill stays
reachable through the plugin that bundles it. Agents are not affected, and other presets
keep writing every skill. With `honor_targets` a skill whose `targets` do not name `claude`
is not written to `.claude/skills`.

### Register the marketplace in `.claude/settings.json`

```toml
[claude.settings]
manage = true                       # default false
register_marketplace = true         # default when managing: extraKnownMarketplaces.<marketplace.name>
enable_plugins = ["acme-essentials"]    # enabledPlugins."acme-essentials@acme": true
disable_plugins = []                    # ... : false
# marketplace_source = { source = "github", repo = "acme/skills" }   # default: a directory source at output_dir
# auto_update = true
```

Only those entries are owned, with the same machinery as `mcpServers`: every other key, marketplace
and plugin entry in the file survives byte-for-byte, an entry dropped from the config is removed
on the next `generate`, and `clean` removes what was written. A hand-written entry with the same
key is replaced by the configured value. The default `directory` source is the marketplace
root relative to the repository (`"./tools/acme-marketplace"`); Claude Code resolves it against the
main checkout. `extraKnownMarketplaces` takes effect only after the user trusts the folder.
Relative-path plugins listed in `enabledPlugins` load from the marketplace without an install
step; the key cannot force an install of a plugin from an external source.

### Catalog skill

```toml
[marketplace.catalog_skill]
enabled = true                 # default false
name = "plugin-catalog"
```

Generates a skill listing each plugin, its default state, the skills it bundles and how to
install or enable it (`claude plugin marketplace add`, `claude plugin install`,
`enabledPlugins`). It is written like a root skill to the preset skill outputs and is never bundled into a plugin. A
root skill with the same name wins.

## Field reference

`[plugin]`: `name` (lowercase letters, digits, `.`, `_` and `-`, starting with a letter or digit, no `..`; it
becomes a directory and file name in every runtime), `version` (required); `display_name`, `description`, `homepage`,
`repository`, `license`, `category`, `brand_color`, `icon`, `logo`, `keywords`,
`tags`, `runtimes`, `include_domains`, `include_evals` (bundle eval cases, see [Evals](evals.md)), `content_root` (project-relative directory of plugin-only
`skills/`, `commands/`, and `agents/`). Sub-tables: `[plugin.author]` (`name`/`email`/`url`),
`[[plugin.mcp]]`, `[[plugin.hooks]]` (+ `[[plugin.hooks.hooks]]`),
`[plugin.statusline]` (`script`/`command`, Claude-only), `[plugin.interface]`
(Codex/Kimi UI block), `[plugin.gemini]` (`context_file_name`), `[plugin.kimi]`
(`skill_instructions`/`session_start_skill`), `[plugin.hermes]`
(`source`/`requires_python`).

`[marketplace]`: `name` (required); `description`, `members`, `output_dir`, `[marketplace.owner]`,
`[marketplace.from_domains]`, `[[marketplace.plugins]]` and `[marketplace.catalog_skill]` (see
[Domain content and per-domain plugins](#domain-content-and-per-domain-plugins)). Related top-level
tables: `[placement]` and `[claude.settings]`.
