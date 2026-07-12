# Authoring Plugins

`ai-rulez generate --plugin` packages your `.ai-rulez/` project into distributable
**plugin bundles** and a **marketplace index** for the Claude, Cursor, Codex, Gemini,
Kimi, OpenCode, and Factory runtimes. Where the normal `generate` writes in-repo
assistant config, `--plugin` produces installable artifacts other people can add to
their own tools — reaching users who never run ai-rulez.

Your skills, commands, and agents are the payload; the `[plugin]` block adds the
packaging metadata.

!!! note "Authoring vs. installing"
    The `[plugin]` / `[marketplace]` blocks documented here are the **producer**
    (authoring) side. They are distinct from the `[[plugins]]` / `[[marketplaces]]`
    arrays, which are the **consumer** side (declaring plugins to *install* from a
    marketplace, rendered into `.claude/plugins.json`).

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

| Runtime  | Manifest | Notes |
|----------|----------|-------|
| Claude   | `.claude-plugin/plugin.json` | skills/commands/agents auto-discovered from top-level dirs |
| Cursor   | `.cursor-plugin/plugin.json` (+ `hooks/hooks.json`) | bundles its own `skills/`, `commands/` |
| Codex    | `.codex-plugin/plugin.json` (+ `.mcp.json`) | MCP referenced via external file; rich `interface` block |
| Gemini   | `gemini-extension.json` | inline MCP + hooks, context file reference |
| Kimi     | `kimi.plugin.json` | `sessionStart`, `skillInstructions`, `interface` |
| OpenCode | `.opencode/plugins/<plugin-name>.js` (+ `package.json`) | copies the authored adapter or emits a documented no-op scaffold |
| Factory  | `.factory-plugin/plugin.json` | metadata-only |
| Hermes   | `.hermes/plugins/<plugin-name>/` and `.hermes/package/` | project plugin plus buildable Python entry-point package |

The **marketplace index** (`.claude-plugin/marketplace.json`) is emitted alongside.

Content files (SKILL.md, commands, agents) are copied **verbatim** from your source
into each runtime's directories — never re-rendered — so a bundled skill is identical
to the authored one.

### OpenCode adapter

Put OpenCode-specific tools and hooks in `.ai-rulez/opencode/index.js`. The generator
copies that source verbatim to `.opencode/plugins/<plugin-name>.js` and generates the
package metadata OpenCode needs. Keep shared skills, commands, agents, and MCP settings
in their normal `.ai-rulez` sources.

When the source entrypoint is absent, generation emits a documented no-op module. The
module keeps the plugin loadable and tells you where to create the user-owned source;
it does not guess tool schemas, subprocess arguments, or business logic.

### Hermes adapter

Put Hermes-specific registrations in `.ai-rulez/hermes/index.py`. The generator
copies it to `.hermes/plugins/<plugin-name>/__init__.py`, writes `plugin.yaml`, and
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

Strict JSON manifests and binary assets cannot safely contain comments. Each plugin
therefore includes `.ai-rulez-generated.json`, which records the same source hash and
a sorted content-hash entry for every generated output. Monorepo roots include a
separate sidecar covering the aggregate marketplace files. Validators can recompute
these hashes without adding unsupported fields to runtime manifests.

### MCP launch variable

Write MCP launch commands once with the canonical `${PLUGIN_ROOT}`; each runtime
rewrites it to the form it expects — `${CLAUDE_PLUGIN_ROOT}` (Claude),
`${extensionPath}` (Gemini), or plugin-relative `./` (Cursor, Codex, Kimi).

### Hooks

Declare lifecycle hooks once; they render into each runtime's hook format with the
correct root variable (`${CURSOR_PLUGIN_ROOT:-.}` for Cursor hooks, and so on).

```toml
[[plugin.hooks]]
event = "SessionStart"
matcher = "startup|resume"

[[plugin.hooks.hooks]]
type = "command"
command = '"${PLUGIN_ROOT}/hooks/run-hook.cmd" session-start'
async = false
```

Executable glue referenced by hooks (`run-hook.cmd`, launch scripts, a Claude
`statusline.sh`) stays hand-authored and is passed through, not synthesized.

### Restricting runtimes

```toml
[plugin]
# ...
runtimes = ["claude", "cursor"]   # omit to emit all supported runtimes
```

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
emits a single root `marketplace.json` listing each with `source: "./plugins/<name>"`.
Members do not emit their own marketplace index.

## Field reference

`[plugin]`: `name`, `version` (required); `display_name`, `description`, `homepage`,
`repository`, `license`, `category`, `brand_color`, `icon`, `logo`, `keywords`,
`tags`, `runtimes`. Sub-tables: `[plugin.author]` (`name`/`email`/`url`),
`[[plugin.mcp]]`, `[[plugin.hooks]]` (+ `[[plugin.hooks.hooks]]`),
`[plugin.statusline]` (`script`/`command`, Claude-only), `[plugin.interface]`
(Codex/Kimi UI block), `[plugin.gemini]` (`context_file_name`), `[plugin.kimi]`
(`skill_instructions`/`session_start_skill`).

`[marketplace]`: `name` (required); `description`, `members`, and `[marketplace.owner]`.
