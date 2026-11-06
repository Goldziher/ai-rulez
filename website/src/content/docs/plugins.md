---
title: "Authoring plugins"
description: "ai-rulez generate --plugin packages the skills, commands, agents, MCP servers and hooks of your .ai-rulez/ project as installable plugin bundles for Claude Code, Cursor, Codex, Gemini CLI, Kimi, OpenCode, Factory, Hermes, Pi, GitHub Copilot and Agent Plugins clients, plus a marketplace index."
---

`ai-rulez generate --plugin` packages the skills, commands, agents, MCP servers and hooks of your `.ai-rulez/` project
as installable **plugin bundles** for Claude Code, Cursor, Codex, Gemini CLI, Kimi, OpenCode, Factory, Hermes, Pi,
GitHub Copilot and [Agent Plugins](/ai-rulez/agent-plugins/) clients, plus a **marketplace index**. Where plain `generate`
configures assistants inside your repository, a plugin bundle is something other people install into their own tools,
without running ai-rulez.

The `[plugin]` block carries the packaging metadata; the content comes from your existing skills, commands and agents.
To turn a bundle into a release archive and upload it, see [Publish](/ai-rulez/publish/).

:::note[Authoring vs. installing]
This page covers the producer side: `[plugin]`, `[[plugins_authoring]]` and `[marketplace]`. To install or enable
someone else's plugin, see [Installed plugins](/ai-rulez/installed-plugins/) and
[Enable a plugin from another marketplace](#enable-a-plugin-from-another-marketplace).
:::

## Quick start

```toml
# .ai-rulez/config.toml
version = "5.0"
name = "acme-tools"

[plugin]
name = "acme-tools"
version = "1.2.0"
description = "Code review skills and an MCP server for Acme repositories."
license = "MIT"
repository = "https://github.com/acme/acme-tools"
keywords = ["review", "mcp"]
runtimes = ["claude", "cursor", "codex", "gemini"]

[plugin.author]
name = "Acme Tooling"
email = "tooling@acme.example"

[[plugin.mcp]]
name = "acme-docs"
command = "${PLUGIN_ROOT}/scripts/mcp-launch.sh"
args = ["serve"]

# Codex requires an interface block
[plugin.interface]
display_name = "Acme Tools"
short_description = "Acme code review."
long_description = "Review skills and a docs MCP server for Acme repositories."
developer_name = "Acme Tooling"
category = "Developer Tools"
capabilities = ["Read"]
default_prompt = ["Review this pull request"]
```

The project also has a skill in `.ai-rulez/skills/review-pr/` (`SKILL.md`, `index.md` and `scripts/lint.sh`, in the
[OKF](/ai-rulez/okf/) layout), a command `.ai-rulez/commands/triage.md`, and the
launcher `scripts/mcp-launch.sh` at the repository root. A skill directory is bundled whole.

```bash
ai-rulez generate --plugin --dry-run   # list what would be written
ai-rulez generate --plugin
```

```text
plugin bundle:
write-file: .claude-plugin/plugin.json
write-file: skills/review-pr/SKILL.md
write-file: skills/review-pr/index.md
write-file: skills/review-pr/scripts/lint.sh
write-file: commands/triage.md
write-file: .cursor-plugin/plugin.json
write-file: .cursor-plugin/skills/review-pr/SKILL.md
write-file: .cursor-plugin/skills/review-pr/index.md
write-file: .cursor-plugin/skills/review-pr/scripts/lint.sh
write-file: .cursor-plugin/commands/triage.md
write-file: .codex-plugin/plugin.json
write-file: .mcp.json
write-file: gemini-extension.json
write-file: .claude-plugin/marketplace.json
write-file: .ai-rulez-generated.json
```

A single-plugin repository is its own bundle: the files land at the repository root, and the marketplace index lists
the plugin with `"source": "./"`. `.claude-plugin/plugin.json`:

```json
{
  "name": "acme-tools",
  "description": "Code review skills and an MCP server for Acme repositories.",
  "version": "1.2.0",
  "author": {
    "name": "Acme Tooling",
    "email": "tooling@acme.example"
  },
  "repository": "https://github.com/acme/acme-tools",
  "license": "MIT",
  "keywords": ["review", "mcp"],
  "mcpServers": {
    "acme-docs": {
      "command": "${CLAUDE_PLUGIN_ROOT}/scripts/mcp-launch.sh",
      "args": ["serve"]
    }
  }
}
```

The same server is `./scripts/mcp-launch.sh` in `.mcp.json` (Codex) and `.cursor-plugin/plugin.json`, and
`${extensionPath}/scripts/mcp-launch.sh` in `gemini-extension.json`.

Commit the bundle and keep it in sync:

```bash
ai-rulez verify --plugin      # exit 2 when a bundle file was edited, removed or added by hand
ai-rulez validate             # manifest, root-component and version checks
```

The root-component (`AR9T0`) and version-drift (`AR961`) findings are warnings; `validate --strict` makes them fail.

```text
Drifted plugin outputs:
  changed      .claude-plugin/plugin.json
  extra        skills/review-pr/notes.md

Error: generated plugin output is stale

Hint: Run ai-rulez generate --plugin and commit the regenerated output
```

Users then add the repository as a marketplace or extension in their tool (for Claude Code,
`claude plugin marketplace add acme/acme-tools`; Gemini CLI, `gemini extensions install <url>`; for Cursor and Codex,
see the vendor's plugin documentation linked from [Supported Harnesses](/ai-rulez/harnesses/)). Another ai-rulez project can
install it with [Installed plugins](/ai-rulez/installed-plugins/).

## Runtimes

`runtimes` selects the bundles. Without it, the eight default runtimes are generated; `pi`, `copilot` and
`agent-plugins` are opt-in and must be listed.

| Runtime | Default | Manifest | Bundles |
| --- | --- | --- | --- |
| `claude` | yes | `.claude-plugin/plugin.json` | skills, commands, agents, hooks, MCP, statusline; writes the marketplace index |
| `cursor` | yes | `.cursor-plugin/plugin.json` | skills, commands, hooks (`.cursor-plugin/hooks/hooks.json`), MCP |
| `codex` | yes | `.codex-plugin/plugin.json` and `.mcp.json`, or a root `plugin.json` ([layouts](#codex)) | skills, MCP |
| `gemini` | yes | `gemini-extension.json` | MCP and hooks inline; commands opt-in |
| `kimi` | yes | `kimi.plugin.json` | skills, MCP, interface |
| `opencode` | yes | `.opencode/plugins/<name>.js` and `package.json` | skills, commands, agents, MCP ([adapter](#opencode)) |
| `factory` | yes | `.factory-plugin/plugin.json` | metadata only |
| `hermes` | yes | `.hermes/plugins/<name>/` and `.hermes/package/` | skills, commands, agents ([adapter](#hermes)) |
| `pi` | opt-in | `package.json`, `.pi/skills/`, `.pi/prompts/` | skills, commands as prompt templates |
| `copilot` | opt-in | root `plugin.json`, `mcp.json`, `.github/plugin/marketplace.json` | skills, agents, MCP |
| `agent-plugins` | opt-in | root `plugin.json`, `mcp.json` | skills, MCP ([Agent Plugins](/ai-rulez/agent-plugins/)) |

A component a runtime has no documented format for is not generated, and `generate` warns when you configured one
(for example commands or hooks for `copilot`, or MCP servers for `pi`).

Skills and their supporting files are copied verbatim. Commands are converted where the runtime needs another format
(Gemini TOML, Pi prompt templates). Content written in [Open Knowledge Format](/ai-rulez/okf/) gets native frontmatter in the
bundle: `type`, `title` and `x-ai-rulez` are removed and the entries of `x-ai-rulez.metadata` move to the top level.

## Plugin metadata

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `name` | string | required | Lowercase letters, digits, `.`, `_` and `-`, starting with a letter or digit, no `..`. Becomes a directory and file name in every runtime. `agent-plugins`, `copilot` and the Codex root layout also require at most 64 characters, an alphanumeric last character, and no `_` or `--` |
| `description` | string | required | One-line description |
| `version` | string | required | A semantic version. Clients that install from git keep their cached copy until it changes, see [Version drift](#version-drift) |
| `version_from` | string | none | `package.json`, `pyproject.toml` (`project.version`) or `Cargo.toml` (`package.version`) to read the version from; excludes `version` |
| `display_name` | string | none | Human-readable name |
| `homepage`, `repository`, `license` | string | none | Copied into the manifests |
| `category`, `keywords`, `tags` | string, string list | none | Marketplace metadata |
| `brand_color`, `icon`, `logo` | string | none | UI metadata |
| `runtimes` | string list | the eight defaults | Bundles to generate |
| `content_root` | string | the config directory | Project-relative directory holding plugin-only `skills/`, `commands/` and `agents/`, to keep the shipped payload apart from your repository's own content |
| `include_domains` | string list | none | Domains (names or globs) whose content is bundled too, see [Domain content](#domain-content-and-per-domain-plugins) |
| `include_evals` | bool | `false` | Bundle each skill's `evals/` and the project's `.ai-rulez/evals/`, see [Evals](/ai-rulez/evals/) |
| `include` | string list | none | Globs of extra files for the Agent Plugins package, see [Bundling files](/ai-rulez/agent-plugins/#bundling-files) |
| `shared_resources` | string list | none | Directories copied into every bundled skill, see [Shared resources](#shared-resources-across-skills) |
| `provenance_headers` | bool | `true` | Write the generated-file header into bundled Markdown, JS, TS and Python files |
| `skills_dir_owned` | bool | `false` | Treat the bundle's skills directory as fully generated, see [Drift](#drift-and-cleanup) |
| `spec` | string | `1.0.0` | Agent Plugins version (`1.0.0` or `1.1.0`) for `agent-plugins`, `copilot` and the Codex root layout |
| `package_manager` | string | none | `packageManager` of the OpenCode and Pi `package.json`, such as `pnpm@9.15.0` |
| `opencode_dependency` | string | `^2.0.26` | Version range of the `@opencode/plugin` dependency |
| `opencode_dependency_enabled` | bool | `true` | `false` drops the `@opencode/plugin` dependency |

| Table | Keys |
| --- | --- |
| `[plugin.author]` | `name`, `email`, `url`. `name` is required for the Claude marketplace index (its `owner`) and for Codex |
| `[[plugin.mcp]]` | `name`, `command`, `args`, `env`, `transport`, `url`, see [MCP servers](#mcp-servers) |
| `[[plugin.hooks]]` | `event`, `matcher`, `[[plugin.hooks.hooks]]`, see [Hooks](#hooks) |
| `[plugin.statusline]` | `script`, `command`; Claude Code only |
| `[plugin.interface]` | the Codex and Kimi UI block, see [Codex](#codex) |
| `[plugin.codex]`, `[plugin.cursor]`, `[plugin.gemini]`, `[plugin.kimi]`, `[plugin.hermes]`, `[plugin.pi]` | see [Runtime-specific settings](#runtime-specific-settings) |

A version bump can live in one place with `version_from`:

```toml
[plugin]
name = "acme-tools"
description = "Code review skills and an MCP server for Acme repositories."
version_from = "package.json"
```

The resolved version replaces `version` in every manifest and in `ai-rulez.lock`, so a bump in `package.json` shows as
drift until you regenerate. `validate` reports a missing file or version key.

## MCP servers

`[[plugin.mcp]]` declares the plugin's MCP servers. When the plugin declares none, the project's enabled
`[[mcp_servers]]` are bundled instead.

Write launch paths once with `${PLUGIN_ROOT}`; each runtime gets its own form:

| Runtime | `${PLUGIN_ROOT}/scripts/run.sh` becomes |
| --- | --- |
| Claude | `${CLAUDE_PLUGIN_ROOT}/scripts/run.sh` |
| Gemini | `${extensionPath}/scripts/run.sh` |
| Cursor, Codex, Kimi, Agent Plugins, Copilot | `./scripts/run.sh` |
| OpenCode | `${PLUGIN_ROOT}/scripts/run.sh`, expanded by the adapter at load time |

- Remote servers use `transport = "http"` (or `"sse"`) and a `url`.
- Header values are never bundled; `generate` warns when a server has `headers`.
- A disabled server is carried as `disabled: true` into the OpenCode bundle; the other runtimes have no such flag, so
  it is bundled enabled and `generate` warns.
- Files the command points at must exist in the installed plugin. In a single-plugin repository the plugin root is the
  repository root, so keep the launcher there (`scripts/mcp-launch.sh` above).

For Gemini, declare the environment variables a server needs as extension settings; Gemini prompts for them and
stores `sensitive` values in its keychain:

```toml
[[plugin.gemini.settings]]
name = "Docs token"
description = "Token for the Acme docs server."
env_var = "ACME_DOCS_TOKEN"
sensitive = true
```

Setting names and environment variables must be unique; environment variable names use letters, digits and
underscores.

## Hooks

Hooks are declared once and rendered into each runtime's hook format (Claude `hooks/hooks.json`, Cursor
`.cursor-plugin/hooks/hooks.json`, Gemini inline in `gemini-extension.json`).

```toml
[[plugin.hooks]]
event = "SessionStart"
matcher = "startup"

[[plugin.hooks.hooks]]
script = "scripts/bootstrap.sh"        # relative to the project root; copied to hooks/bootstrap.sh
timeout = 30
status_message = "Preparing acme-tools..."

[[plugin.hooks]]
event = "PreToolUse"
matcher = "Bash"

[[plugin.hooks.hooks]]
command = "acme-guard check"           # must exist on the user's machine
if = "Bash(git push *)"
```

The Claude rendering of the first hook:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {
            "type": "command",
            "command": "\"${CLAUDE_PLUGIN_ROOT}/hooks/bootstrap.sh\"",
            "timeout": 30,
            "async": false,
            "statusMessage": "Preparing acme-tools..."
          }
        ]
      }
    ]
  }
}
```

| Key | Meaning |
| --- | --- |
| `event` | A Claude Code lifecycle event: `SessionStart`, `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Stop`, ... An unknown event is a warning, not an error |
| `matcher` | The event's matcher. Events without a subject (`UserPromptSubmit`, `Stop`, `PostToolBatch`, ...) ignore it, and `validate` warns |
| `type` | Handler type; default `command` |
| `command` | An executable already on the user's machine, passed through verbatim |
| `script` | A project file bundled into the plugin's `hooks/` directory and run from there. Exactly one of `command` and `script` |
| `args` | Arguments; with `args` the executable is spawned directly, without a shell |
| `timeout` | Seconds; `0` uses the runtime default |
| `async` | Run without blocking (default `false`) |
| `if` | One permission rule such as `"Bash(git *)"` or `"Edit(*.ts)"`. Claude Code evaluates it only on `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest` and `PermissionDenied`; on other events a handler with `if` never runs, and `validate` warns |
| `status_message` | Shown while the handler runs (`statusMessage`) |

A `SessionStart` hook has no declarative guard: a bootstrap script must check for itself whether its work is done.

## Shared resources across skills

Skills in one plugin often link to the same reference docs or templates. `shared_resources` copies directories
(relative to the content root) into **every** bundled skill, for every runtime, so a skill still works when installed
on its own:

```toml
[plugin]
shared_resources = ["references", "templates"]
```

- `references/style.md` lands at `skills/<name>/references/style.md` in each skill.
- A `SKILL.md` link that climbs out to the shared directory (`../../references/style.md`) is rewritten to the copy
  (`references/style.md`).
- A shared file that clashes with a file the skill already has is an error.
- The copies are recorded like the skill's own files, so `verify --plugin` reports changes.

A symlink inside a skill directory follows the same policy as plain `generate`
([Symlinks in content](/ai-rulez/configuration/#symlinks-in-content)): a link to a target inside the project is copied, any
other link is skipped with a warning.

## Runtime-specific settings

### Codex

Codex requires `[plugin.author] name` and a `[plugin.interface]` block with `display_name`, `short_description`,
`long_description`, `developer_name`, `category`, `capabilities` and `default_prompt`. Optional interface keys:
`website_url`, `privacy_policy_url`, `terms_of_service_url`, `brand_color`, `composer_icon`, `logo`, `logo_dark`,
`screenshots`. Kimi reads the same block.

```toml
[plugin.codex]
manifest = "root"     # legacy (default) | root | both
marketplace = true    # also write .agents/plugins/marketplace.json for a single-plugin repository
```

| `manifest` | Writes |
| --- | --- |
| `legacy` | `.codex-plugin/plugin.json` with the `interface` block, and `.mcp.json` |
| `root` | the Agent Plugins layout Codex prefers: a root `plugin.json` with the interface under `extensions["com.openai"]`, `skills/` and a root `mcp.json` |
| `both` | both manifests |

The root layout follows `[plugin] spec` and the Agent Plugins name rules, and shares the root `plugin.json` with the
`agent-plugins` and `copilot` runtimes. With [`source = "agents"`](/ai-rulez/agents-source/#what-generate-writes),
`.agents/` belongs to the project and the Codex marketplace index is skipped with a warning.

### Copilot

`runtimes = ["copilot"]` writes the Agent Plugins layout that Copilot CLI and the Copilot cloud agent read: a root
`plugin.json`, `skills/<name>/`, a root `mcp.json`, custom agents at `com.github.copilot/agents/<name>.agent.md`, and a
single-plugin marketplace at `.github/plugin/marketplace.json`. Copilot documents no file format for plugin commands
or hooks, so they are not written.

### Cursor

`[plugin.cursor] marketplace = true` also writes `.cursor-plugin/marketplace.json` for a single-plugin repository
(`source` is `.`). For marketplaces with several plugins, set `[marketplace] cursor_index = true`. To publish a Cursor
team marketplace, see the [`cursor-team-marketplace` emitter](/ai-rulez/publish/#emitters).

### Gemini

| Key | Meaning |
| --- | --- |
| `[plugin.gemini] context_file_name` | `contextFileName` of the extension (default `GEMINI.md`) |
| `[plugin.gemini] commands` | `true` bundles each command as `commands/<name>.toml`; `$ARGUMENTS` becomes `{{args}}`. Shell (`!{...}`) and file (`@{...}`) injections are not translated |
| `[[plugin.gemini.settings]]` | `name`, `description`, `env_var`, `sensitive`, see [MCP servers](#mcp-servers) |

### Kimi

`[plugin.kimi]` takes `skill_instructions` and `session_start_skill`, written to `kimi.plugin.json`.

### Pi packages

```toml
[plugin]
name = "acme-tools"
version = "1.2.0"
description = "Skills and prompts for Acme repositories."
repository = "https://github.com/acme/acme-tools"
runtimes = ["pi"]
```

```bash
ai-rulez generate --plugin
pi install git:github.com/acme/acme-tools
```

The package holds `.pi/skills/<name>/` and `.pi/prompts/<name>.md` (rendered from your commands) and a root
`package.json` with the `pi-package` keyword, the generated files in `files`, and a `pi` block listing the skills and
prompts. On GitHub its name is `@<owner>/pi-<plugin-name>` (`@acme/pi-acme-tools`). Pi packages carry skills and
prompts only; MCP servers, hooks and agents are skipped with a warning.

| `[plugin.pi]` key | Default | Meaning |
| --- | --- | --- |
| `skills_dir` | `.pi/skills` | Where skills are written. Set `skills` to reuse the root `skills/` tree the Claude, Codex and Agent Plugins bundles write, instead of a second copy |
| `skills_entry` | `each` | `directory` lists the skills directory once in `pi.skills` and `files`, so adding a skill does not change `package.json` |
| `package_name` | `@<owner>/pi-<plugin-name>` | Explicit npm name |
| `package_json` | `generate` | `merge` updates only the keys ai-rulez owns (`name`, `version`, `description`, `author`, `repository`, `keywords`, `pi`, `files`) and keeps the rest (`devDependencies`, `scripts`, ...); an authored `name` is kept unless `package_name` is set. `verify --plugin` then checks only the owned keys |

With `runtimes = ["pi", "opencode"]` both share one `package.json`, named `@<owner>/opencode-<plugin-name>`, with
OpenCode's entry points and Pi's resources. If the two would render a different shared key, generation fails with
"runtime npm manifests disagree".

To publish on npm (which also lists the package in the [Pi package catalog](https://pi.dev/packages)), see
[Publish: npm](/ai-rulez/publish/#npm); the published name is `<scope>/<plugin-name>`:

```bash
ai-rulez publish --to npm --npm-scope @acme --public --execute --yes
pi install npm:@acme/acme-tools
```

### OpenCode

The OpenCode bundle is a v2 plugin, a default export `{ id, setup(ctx) }`:

| File | Content |
| --- | --- |
| `.opencode/plugins/<name>.js` | your adapter `.ai-rulez/opencode/index.js`, copied verbatim; without one, a generated module that only registers the bundled content |
| `.opencode/ai-rulez-content.js` | `registerBundledContent(ctx)`, which registers the bundled skills, commands, agents and MCP servers |
| `.opencode/ai-rulez-bundle.json` | the MCP servers and agent settings resolved at generation time |
| `.opencode/skills/`, `commands/`, `agents/` | the content |
| `package.json` | `main` and `exports` pointing at the plugin, and a dependency on `@opencode/plugin` |

OpenCode does not discover the content directories of an installed plugin, so a hand-written adapter must call the
helper from `setup`:

```js
import { registerBundledContent } from "../ai-rulez-content.js";

export default {
  id: "acme-tools",
  async setup(ctx) {
    await registerBundledContent(ctx);
  },
};
```

- **MCP servers** become OpenCode `local` (command array and `environment`) or `remote` (`url`) servers. At load time
  `${PLUGIN_ROOT}` expands to the plugin directory and `${NAME}` to `process.env.NAME` (empty when unset). A plugin runs
  inside OpenCode and can read its whole environment, so install only plugins you trust. Top-level directories an MCP
  server reaches through `${PLUGIN_ROOT}/...` are added to `package.json` `files`.
- **Agents** take their model as the `opencode` preset resolves it (`opencode_model`, then
  `defaults.model_by_preset.opencode`, then `model`). A bare alias such as `sonnet` is omitted with a warning, so the
  agent inherits the session model.
- **SDK dependency.** For an adapter that does not import `@opencode/plugin`, set
  `[plugin] opencode_dependency_enabled = false`.

To use the bundle:

1. **Copy it into a project.** Copy `.opencode/plugins/<name>.js`, `.opencode/ai-rulez-content.js`,
   `.opencode/ai-rulez-bundle.json` and the `.opencode/skills/`, `commands/` and `agents/` directories, plus any
   top-level directory an MCP server uses (such as `scripts/`), keeping their paths. Restart OpenCode.
2. **Point `opencode.json` at a directory.** Add `"plugins": ["./vendor/acme-tools"]`, where the directory is a copy of
   the bundle with an `index.js` at its root: `export { default } from "./.opencode/plugins/acme-tools.js";`. OpenCode
   loads a directory's `index.js` and ignores `package.json` `main`.

`generate` warns, without failing, when it finds a v1 OpenCode plugin (an exported function that returns hooks, or an
import of `@opencode-ai/plugin`) in `.ai-rulez/opencode/index.js`, in `.opencode/plugin(s)/`, or in a local path of
`opencode.json`. OpenCode v2 does not run v1 plugins; see the
[v1 migration guide](https://opencode.ai/v2/docs/build/plugins/migrate-v1).

### Hermes

Put Hermes registrations in `.ai-rulez/hermes/index.py` (or point `[plugin.hermes] source` at another
project-relative file). The generator copies it to `.hermes/plugins/<name>/hermes.py`, writes `__init__.py` and
`plugin.yaml`, and bundles the skills, commands and agents. The same content is packaged as a Python project under
`.hermes/package/`, published as `<name>-hermes-plugin`. Without a source file, a no-op `register(ctx)` is generated.
Hermes loads project plugins only with `HERMES_ENABLE_PROJECT_PLUGINS=true`.

```toml
[plugin.hermes]
source = "plugin/hermes.py"
requires_python = ">=3.11"
dependencies = ["httpx>=0.28", "example-tool[cli]; python_version >= '3.11'"]
```

`dependencies` are PEP 508 requirements from a package index; direct references (`name @ url`) are rejected. They are
written to the wheel's `project.dependencies` and to `python_dependencies` in both Hermes manifests. Nothing is
installed at generation time.

## Single plugin vs. monorepo

A repository with one `[plugin]` block is a **single-plugin** marketplace whose only entry has `source: "./"`. The
bundle root is the repository root, so every path a host loads from a plugin root ships to each user. `validate`
and `verify --plugin` warn (`AR9T0 root-component-shipped`) about such a path that exists at the root but is
not part of the bundle: `.mcp.json`, `hooks/`, `agents/`, `commands/`, `settings.json`, `bin/`, a root `plugin.json`,
and the other Claude Code and Codex plugin paths. A maintainer's own `.mcp.json` would otherwise be installed for
everyone. Set `[plugin] content_root`, or move the plugin into a member directory.

For a **monorepo** with several plugins, each its own ai-rulez project, the root declares a `[marketplace]` with
`members`:

```toml
# .ai-rulez/config.toml at the repository root
[marketplace]
name = "acme"
description = "Curated Acme plugins."
members = ["plugins/review", "plugins/notify"]

[marketplace.owner]
name = "Acme Tooling"
email = "tooling@acme.example"
```

Each member (`plugins/review/.ai-rulez/config.toml`, ...) has its own `[plugin]` block. `generate --plugin` at the root
renders every member's bundle in its directory and writes one root index, `.claude-plugin/marketplace.json` (and
`.agents/plugins/marketplace.json` for Codex), listing each member as `source: "./plugins/<dir>"`. Members write no
index of their own.

## Several plugins from one root

One project can also define several plugins that each select part of the root's content, without separate member
projects:

```toml
# .ai-rulez/config.toml
[[plugins_authoring]]
name = "acme-review"
version = "1.2.0"
description = "Code review skills."
runtimes = ["claude", "cursor"]
skills = ["review-*"]            # by name or glob
commands = ["triage"]

[plugins_authoring.author]
name = "Acme Tooling"

[[plugins_authoring]]
name = "acme-notify"
version = "0.3.0"
description = "Desktop notifications."
runtimes = ["claude"]
skills = ["notify-*"]
```

```text
.claude-plugin/marketplace.json
.ai-rulez-generated.json
plugins/acme-notify/.claude-plugin/plugin.json
plugins/acme-notify/skills/notify-desktop/SKILL.md
plugins/acme-review/.claude-plugin/plugin.json
plugins/acme-review/.cursor-plugin/plugin.json
plugins/acme-review/commands/triage.md
plugins/acme-review/skills/review-pr/SKILL.md
...
```

The index is named after the project and lists `./plugins/acme-notify` and `./plugins/acme-review`.

An entry bundles **only** what it selects; an empty selector selects nothing. A selector that matches nothing is a
warning (`AR9T1`).

| Key | Meaning |
| --- | --- |
| `name`, `description`, `version`, `author`, `license`, `homepage`, `repository`, `category`, `keywords` | metadata, as in `[plugin]` |
| `runtimes` | bundles to write |
| `skills`, `commands`, `agents` | root content, by name or glob |
| `domains` | domains whose skills, commands and agents are bundled |
| `mcp_servers` | names of root `[[mcp_servers]]`; none by default |
| `hooks` | hook groups, as `[[plugin.hooks]]` |
| `default_enabled`, `relevance` | marketplace entry fields, see [Marketplace entries](#marketplace-entries) |

The runtime tables (`[plugin.codex]`, `[plugin.interface]`, `[plugin.pi]`, ...), `include`, `shared_resources` and
`content_root` are not available per definition; a plugin that needs them stays a `[plugin]` in its own member
project. `[plugin]` cannot be combined with `[[plugins_authoring]]`.

Each definition can live in its own file, `.ai-rulez/plugins/<name>.toml`, with the entry's keys at the top level
(`name` may be omitted and must equal the file name when present). A
[layer](/ai-rulez/configuration/#layered-configuration) can add, override (by `name`) or remove (`remove = true`) an entry.

| Command | Effect |
| --- | --- |
| `generate --plugin --plugin-name acme-review` | writes only that plugin; the indexes are left alone |
| `verify --plugin --plugin-name acme-review` | checks only that plugin |
| `verify --plugin --format json` | adds a `plugins` array with each plugin's status |

### Several marketplaces from one root

Each `[[marketplaces_authoring]]` entry is one marketplace that lists plugin definitions by name, so a plugin can be
offered in several:

```toml
[[marketplaces_authoring]]
name = "acme-public"
description = "Published plugins"
plugins = ["acme-review", "acme-notify"]
output_dir = "marketplaces/public"

[[marketplaces_authoring]]
name = "acme-internal"
plugins = ["acme-review"]
output_dir = "marketplaces/internal"
cursor_index = true
```

Each marketplace gets `<output_dir>/.claude-plugin/marketplace.json` (plus the Codex index when a listed plugin targets
Codex, and the Cursor index with `cursor_index`), its own provenance sidecar, and `<output_dir>/plugins/<name>/` for
each plugin, so each directory can be published on its own. `output_dir` values must be unique and not nested inside
each other. `[marketplace]` and `[[marketplaces_authoring]]` cannot be combined. Use
`--marketplace-name NAME` with `generate --plugin` and `verify --plugin` to work on one marketplace. A marketplace can
also live in `.ai-rulez/marketplaces/<name>.toml`.

[`publish`](/ai-rulez/publish/#several-plugins) releases only the Claude index closest to the project root.

## Domain content and per-domain plugins

A project that keeps content in `.ai-rulez/domains/<name>/` can ship it as plugins.

To add domain content to the `[plugin]` bundle, list the domains:

```toml
[plugin]
include_domains = ["backend", "ui"]   # or ["*"]
```

On a name collision the root item wins, then the domain earlier in name order. Builtin domains are never bundled.

To ship **one plugin per domain**:

```toml
[marketplace]
name = "acme"
description = "Acme skill packs"
output_dir = "tools/acme-marketplace"   # default: the project root

[marketplace.from_domains]
name_prefix = "acme-"        # plugin "acme-backend" from domains/backend
exclude = ["scratch"]        # include defaults to every domain
version = "1.0.0"            # default: the [plugin] version, else 1.0.0
default_enabled = false
runtimes = ["claude"]        # default: the [plugin] runtimes, else the defaults
```

`generate --plugin` writes `<output_dir>/plugins/<name>/` for each domain and one
`<output_dir>/.claude-plugin/marketplace.json` with `source: "./plugins/<name>"` entries (and the Codex index when a
plugin targets `codex`). A domain without bundleable content is skipped with a warning. When a domain disappears, the
next `generate --plugin` deletes the files it generated for that plugin.

In this mode the `[plugin]` block is optional and only supplies defaults (author, license, homepage, repository,
version, category, keywords, runtimes). `owner` falls back to the `[plugin]` author. `members` and domain plugins can
be combined only when `output_dir` is unset.

### Marketplace entries

`[[marketplace.plugins]]` declares a plugin by hand, mixing root content and domains; it replaces a derived plugin of
the same name and takes the same keys as `[[plugins_authoring]]`:

```toml
[[marketplace.plugins]]
name = "acme-essentials"
skills = ["development-standards", "personal-*"]
domains = []
default_enabled = true

[marketplace.plugins.relevance]        # when Claude Code suggests the plugin
topic = "Terraform"

[marketplace.plugins.relevance.signals]
cli = ["terraform"]
files_read = ["**/*.tf"]
```

Entries carry `version`, `category`, `keywords`, `defaultEnabled` and `relevance` (signals `cwd`, `cli`, `hosts`,
`files_read`, `manifest_deps`; at least one).

### Keep a skill out of `.claude/skills`

By default every skill and command is also written to `.claude/skills` by the `claude` preset. `[placement]` makes
some plugin-only:

```toml
[placement]
default = "core"                          # core or plugin
plugin = ["domains/*"]                    # names or globs; "domains/<domain>/<name>" also matches
core = ["domains/backend/python-conventions", "git-*"]
honor_targets = true                      # skills whose frontmatter `targets` omit claude are kept out too
```

An item's frontmatter `placement: core|plugin` wins, then `core`, then `plugin`, then `default`. Placement only changes
the Claude preset's `.claude/skills` output: plugin bundles always contain the item, and other presets write every
skill.

`ai-rulez list --placement` prints each skill and command with its destination, its domain and the plugins that bundle
it, and flags a plugin-only item that no enabled or catalogued plugin bundles.

### Register the marketplace in `.claude/settings.json`

```toml
[claude.settings]
manage = true                          # default false
register_marketplace = true            # default when managing: extraKnownMarketplaces.<marketplace name>
enable_plugins = ["acme-essentials"]   # enabledPlugins."acme-essentials@acme": true
disable_plugins = []                   # ...: false
# marketplace_source = { source = "github", repo = "acme/skills" }   # default: a directory source at output_dir
# auto_update = true
```

Only these entries are owned: every other key in the file is kept, an entry removed from the config is removed on the
next `generate`, and `clean` removes what was written. The default source is a `directory` relative to the repository
(`"./tools/acme-marketplace"`). Claude Code applies `extraKnownMarketplaces` only after the user trusts the folder.

`enable_plugins` and `disable_plugins` take a bare plugin name (qualified with the `[marketplace]` name) or a full
`name@marketplace` id. A block with only full ids needs no `[marketplace]`.

In a linked git worktree, Claude Code resolves a relative `directory` source against the main checkout, so plugin edits
in the worktree are not visible to it; `validate` warns about this. Set `marketplace_source` to an absolute
`directory` or a `github`/`git` source to test from a worktree.

### Enable a plugin from another marketplace

`[[enabled_plugins]]` switches on a plugin this project does not author, for the Claude Code, Codex and OpenCode
presets at once:

```toml
[[enabled_plugins]]
id = "lint-kit@acme-market"                 # <name>@<marketplace>, required
marketplace = { name = "acme-market", source = "acme/plugins", type = "github" }   # optional
opencode = "@acme/lint-kit-opencode"        # optional npm package for OpenCode
```

| Preset | File | Written |
| --- | --- | --- |
| `claude` | `.claude/settings.json` | `enabledPlugins."<id>": true`, and with `marketplace`, `extraKnownMarketplaces.<name>` |
| `codex` | `.codex/config.toml` | `[plugins."<id>"]` with `enabled = true` |
| `opencode` | `opencode.json` | the `opencode` package appended to `plugin`, only when `opencode` is set |

| `marketplace` key | Meaning |
| --- | --- |
| `name` | must equal the marketplace part of `id` |
| `type` | `github` (`source` is `owner/repo`), `git` (an `https://` or `ssh://` URL, or `git@host:path`), `url` (an `https://` URL) or `directory` (a project-relative path) |
| `source` | the location, in the form `type` requires; `http://` and `git://` are rejected |
| `ref` | a branch or tag, for `github` and `git` |

Leave `marketplace` out when the harness already knows it. Only the written entries are owned, as for
`[claude.settings]`, and the declaration needs no `[claude.settings] manage`. With `generate --user` the entries go to
the user-level files; a `directory` marketplace is an error there. `generate` lists the marketplace registration and
the plugin as items to acknowledge, like hooks.

Conflicts are errors at load: an id in both `[[enabled_plugins]]` and `disable_plugins`, or a `marketplace` that would
replace the project's own registered `[marketplace]`.

### Catalog skill

```toml
[marketplace.catalog_skill]
enabled = true                 # default false
name = "plugin-catalog"
```

Generates a skill listing each plugin, its default state, the skills it bundles and how to install or enable it. It is
written like a root skill to the preset outputs and is never bundled into a plugin. A root skill with the same name
wins.

## Drift and cleanup

<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/plugin-drift.gif" alt="ai-rulez verify --plugin reporting changed and extra files in a plugin bundle, then generate --plugin restoring it" width="820" />
</p>

Every bundle has a `.ai-rulez-generated.json` sidecar that records a content hash and file mode for each generated
file. Bundled Markdown, JavaScript, TypeScript and Python files also carry a generated-file header after their
frontmatter (turn it off with `provenance_headers = false`; the sidecar still records the hashes).

`ai-rulez verify --plugin` compares the bundle with the sidecar and its sources without writing anything, and exits 2
on drift:

| Drift kind | Meaning |
| --- | --- |
| `changed` | a generated file was edited |
| `missing` | a generated file was deleted |
| `stale-source` | the sources changed since the bundle was generated |
| `obsolete` | a file an earlier run generated that the sources no longer produce, kept because you edited it |
| `extra` | an unrecorded file inside a generated skill directory (or, with `skills_dir_owned`, anywhere in the skills directory) |
| `mode` | a file's executable bit differs from the record, for example after a zip round trip. A sidecar written on Windows records no modes; regenerate on Linux or macOS if the bundle ships scripts |
| `symlink` | a generated output path is a symlink |

`generate --plugin` repairs drift:

- It deletes generated files whose source was removed, then the directories they leave empty. An obsolete file you
  edited is kept with a warning, and `verify --plugin` fails until you remove it.
- It deletes unrecorded files inside generated skill directories (a stray `notes.md`). Your sources (the config
  directory and `content_root`) are never touched.
- It refuses to write through a symlink.
- `generate --plugin --dry-run` lists each deletion as `delete-stale: <path>` and each kept file as
  `keep-obsolete: <path>`.

`skills_dir_owned = true` declares the skills directory (`skills/`, `.pi/skills`, ...) fully generated: a leftover skill
directory from a rename, or any stray file there, is reported as `extra` and removed. A Pi `skills_dir` that points
into your sources cannot be owned.

### Version drift

A client that installed a plugin from a git-hosted marketplace keeps its cached copy until the plugin's version
changes. `ai-rulez validate` reports `AR961 plugin-version-drift` (warning) when a plugin's generated content differs
from its committed sidecar at `HEAD` while its version is unchanged. Bump `version` with every content change.

### CI and hooks

```bash
ai-rulez generate --recursive --plugin --if-configured
ai-rulez verify --recursive --plugin --if-configured
```

`--if-configured` succeeds without doing anything in a project that has no `[plugin]` or marketplace authoring
configuration, so a shared hook catalog can run on every repository; it does not hide errors. A marketplace root is
processed once with its members. See [Poly hooks](/ai-rulez/poly-hooks/) for a pre-commit setup and the
[`check` action](/ai-rulez/github-actions/#governance-gate-check) (`plugin: true`) for CI.

A plugin-only root (no preset) is built in plugin mode by plain `generate --recursive`; see
[Nested configurations](/ai-rulez/monorepo/#nested-configurations).

## See also

- [Agent Plugins](/ai-rulez/agent-plugins/)
- [Publish](/ai-rulez/publish/)
- [Installed plugins](/ai-rulez/installed-plugins/)
