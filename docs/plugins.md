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

The **marketplace index** (`.claude-plugin/marketplace.json`) is emitted alongside when `claude` is
among the bundle's runtimes. (A monorepo root also emits a Codex index at
`.agents/plugins/marketplace.json`; see the monorepo section.)

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
native v2 `opencode.json` owning `$schema` and `mcp.servers`, merged so any other keys in a
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

## Field reference

`[plugin]`: `name` (lowercase letters, digits, `.`, `_` and `-`, starting with a letter or digit, no `..`; it
becomes a directory and file name in every runtime), `version` (required); `display_name`, `description`, `homepage`,
`repository`, `license`, `category`, `brand_color`, `icon`, `logo`, `keywords`,
`tags`, `runtimes`, `content_root` (project-relative directory of plugin-only
`skills/`, `commands/`, and `agents/`). Sub-tables: `[plugin.author]` (`name`/`email`/`url`),
`[[plugin.mcp]]`, `[[plugin.hooks]]` (+ `[[plugin.hooks.hooks]]`),
`[plugin.statusline]` (`script`/`command`, Claude-only), `[plugin.interface]`
(Codex/Kimi UI block), `[plugin.gemini]` (`context_file_name`), `[plugin.kimi]`
(`skill_instructions`/`session_start_skill`), `[plugin.hermes]`
(`source`/`requires_python`).

`[marketplace]`: `name` (required); `description`, `members`, and `[marketplace.owner]`.
