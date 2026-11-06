---
title: "Installed plugins and marketplaces"
description: "Import a harness plugin (Claude Code, Codex, Cursor, Gemini CLI or Agent Plugins) and render its skills, agents, commands, rules, hooks and MCP servers into every preset you generate."
---

Import a harness plugin (Claude Code, Codex, Cursor, Gemini CLI or [Agent Plugins](/ai-rulez/agent-plugins/)) and render its
skills, agents, commands, rules, hooks and MCP servers into every preset you generate. An
`[[installed_marketplaces]]` entry names a plugin marketplace; an `[[installed_plugins]]` entry imports one plugin
from a marketplace or straight from a repository. Imported content is pinned in `ai-rulez.lock` and scanned like
[installed skills](/ai-rulez/installed-skills/).

To build a plugin from your own content instead, see [Authoring plugins](/ai-rulez/plugins/).

## Example

A marketplace in `vendor/acme-market` with one plugin:

```text
vendor/acme-market/
  .claude-plugin/marketplace.json      # {"name": "acme", "plugins": [{"name": "formatter", "source": "./plugins/formatter"}]}
  plugins/formatter/
    .claude-plugin/plugin.json
    skills/format-code/SKILL.md
    agents/style-reviewer.md
    commands/git/commit.md
    hooks/hooks.json
    .mcp.json
```

```bash
ai-rulez marketplace add acme --source ./vendor/acme-market
ai-rulez plugin install formatter --from acme
ai-rulez generate
```

The two commands add:

```toml
[[installed_marketplaces]]
name = 'acme'
source = './vendor/acme-market'

[[installed_plugins]]
name = 'formatter'
marketplace = 'acme'
```

With `presets = ["claude", "cursor", "gemini"]`, `generate` writes the plugin's content into each tool's own
folders:

```text
INFO  Installed plugin hooks are not imported; set enable_hooks = true to import them plugin=formatter hooks=1
...
.agents/skills/format-code/SKILL.md
.claude/agents/style-reviewer.md
.claude/skills/format-code/SKILL.md
.claude/skills/git-commit/SKILL.md
.cursor/agents/style-reviewer.md
.cursor/commands/git-commit.md
.cursor/mcp.json
.gemini/agents/style-reviewer.md
.gemini/settings.json
.mcp.json
```

A remote marketplace works the same way:

```bash
ai-rulez marketplace add acme --source acme/plugins --version "^1.2"
ai-rulez plugin install review-tools --source https://github.com/acme/review-tools --ref v2.0.1
ai-rulez lock
```

## Supported formats

The layout is detected from the manifest; set `format` (`--layout`) to force `claude`, `codex`, `cursor`, `gemini`
or `agent-plugins`.

| Harness | Marketplace index | Plugin manifest | Imported |
| --- | --- | --- | --- |
| Claude Code | `.claude-plugin/marketplace.json` | `.claude-plugin/plugin.json` (optional) | `skills/`, `agents/`, `commands/`, `hooks/hooks.json`, `.mcp.json`, and the paths or inline objects the manifest's `skills`, `agents`, `commands`, `hooks` and `mcpServers` name |
| Codex | `.agents/plugins/marketplace.json` | `.codex-plugin/plugin.json` | `skills/`, `.mcp.json`, `hooks.json`, and the manifest's `skills`, `mcpServers` and `hooks` |
| Cursor | `.cursor-plugin/marketplace.json` | `.cursor-plugin/plugin.json` | `rules/` (as rules), `agents/`, `skills/`, `commands/`, `hooks/hooks.json`, `.mcp.json` or `mcp.json` |
| Gemini CLI extension | none | `gemini-extension.json` | `mcpServers`, the `contextFileName` file (as a rule), `commands/*.toml` (as commands), `skills/`, `agents/` |
| Agent Plugins | none | root `plugin.json` with an `agent-plugins.org` `$schema` | `skills/`, `mcp.json`, and the `com.anthropic.claude-code` (agents, commands, hooks) and `io.github.goldziher.ai-rulez` (rules) extensions |

- A directory with Claude Code's default folders and no manifest is a Claude Code plugin.
- A marketplace entry with `strict = false` that declares its own components needs no manifest.
- Skills are bundled whole, as for an [installed skill](/ai-rulez/installed-skills/#what-is-bundled-with-a-skill).
- Agent, command and rule files keep their frontmatter. A nested command `git/commit.md` becomes `git-commit`.
- A Gemini TOML command becomes a prompt; its `{{args}}` and `!{...}` placeholders are kept as written.

## Source forms

| Form | Example |
| --- | --- |
| GitHub shorthand | `acme/plugins`, `github:acme/plugins` |
| Git URL | `https://github.com/acme/plugins`, `git@github.com:acme/plugins.git`, `file:///srv/plugins` |
| Local directory | `./vendor/marketplace` (starts with `./`, `../` or `/`) |
| Index URL (marketplace only) | `https://example.com/marketplace.json` |

The rules of installed skills apply: `https` only, no credentials in a URL, no `..` in `path`, and a local path in
the committed config must stay inside the project (`config.local.toml` and the user config may leave it). A
`file://` repository outside the project needs `AI_RULEZ_ALLOW_FILE_URLS=1`.

A marketplace entry can name its plugin as:

- a relative path in the marketplace repository (`"./plugins/x"`; `metadata.pluginRoot` is prepended to a bare
  name);
- `{"source": "github", "repo": "owner/repo", "ref": "...", "sha": "..."}`;
- `{"source": "url" | "git", "url": "...", "ref": "...", "sha": "..."}`;
- `{"source": "git-subdir", "url": "...", "path": "sub/dir"}`, or Codex's `{"source": "local", "path": "./x"}`.

A relative path that is absolute, contains `..` or passes through a symlink is refused. Entries with other source
types (npm, pip) are listed as not installable.

## Configuration

### `[[installed_marketplaces]]`

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `name` | string | required | Name that `[[installed_plugins]] marketplace` refers to |
| `source` | string | required | See [Source forms](#source-forms) |
| `path` | string | repository root | Directory holding the index |
| `ref` | string | default branch | Branch, tag or commit |
| `version`, `tag_prefix`, `include_prerelease`, `min_release_age` | | | Tag range instead of `ref`, as on [installed skills](/ai-rulez/installed-skills/#configuration) |
| `trust` | `error` \| `warn` | unset | Scan level of the plugins read through it; `error` makes every finding an error |

### `[[installed_plugins]]`

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `name` | string | required | Plugin name |
| `marketplace` | string | | An installed marketplace to take it from |
| `plugin` | string | `name` | The marketplace entry, when it is called differently (`--entry`) |
| `source`, `path`, `ref` | string | | A plugin repository or directory, instead of `marketplace` |
| `version`, `tag_prefix`, `include_prerelease`, `min_release_age` | | | Tag range, for a plugin from `source` only |
| `format` | string | detected | Force the layout (`--layout`) |
| `include` | list | all | Import only these kinds: `skills`, `agents`, `commands`, `hooks`, `mcp`, `rules` |
| `exclude` | list | none | Skip these kinds; wins over `include` |
| `name_prefix` | string | | Prepended to every imported name and to skill output directories |
| `domain` | string | root | Put the content in a [domain](/ai-rulez/domains/) |
| `profiles` | list | all | Include the content only in these profiles |
| `trust` | `error` \| `warn` | the marketplace's | Scan level |
| `enable_hooks` | bool | `false` | Import the plugin's hooks |
| `enable_mcp` | bool | `true` | Import the plugin's MCP servers |

A plugin from a marketplace follows the marketplace's `ref` or `version`; setting them on the plugin is an error.
`domain` and `profiles` scope everything except hooks, which are global.

When an imported name already exists (your own content first, then earlier plugins), the existing item wins and the
later one is skipped with a warning. Set `name_prefix` to keep both.

## Hooks and MCP servers

- **Hooks are off by default.** A hook runs a command on every machine that uses the project, so set
  `enable_hooks = true` (`--enable-hooks`) to import them. Imported hooks are written only for the harness the
  plugin was made for.
- **MCP servers** are imported unless `enable_mcp = false`. Literal credentials in `env`, `args`, `url` or `headers`
  are replaced by `${VAR}` references and reported. The servers go through the same MCP checks and `[mcp]` policy as
  your own.
- **Plugin root variables.** A hook or MCP server that refers to `${CLAUDE_PLUGIN_ROOT}`, `${CODEX_PLUGIN_ROOT}`,
  `${PLUGIN_ROOT}` or Gemini's `${extensionPath}` is rewritten to the plugin's project-relative path when the plugin is
  a local directory inside the project:

    ```json
    "command": "./vendor/acme-market/plugins/formatter/fmt.sh"
    ```

  For a fetched plugin there is no such directory, so the hook or server is not imported (run with `--debug` to see
  each skipped component). `${CLAUDE_PLUGIN_DATA}` is never supported.
- **Permissions** are never imported: a plugin cannot add an allow rule.

## Security

- Fetches share the include machinery: the git cache, credential handling (a token is sent only to allowlisted
  hosts) and the `sources.allowed_hosts` / `sources.deny_hosts` policy, which also applies to repositories a
  marketplace names.
- `ai-rulez lock` scans every file of a new marketplace or plugin tree, hook scripts and MCP definitions included,
  before pinning it. Error findings refuse the pin (exit 2) unless you pass `--accept-findings`. `trust = "error"`
  makes every finding an error.
- `generate` scans the imported content, hook commands, MCP definitions and the scripts they run again
  ([`[lint.security] scan_imports`](/ai-rulez/strict-validation/#security-checks)).
- Symlinks are never followed, files and manifests are size-bounded, and a malformed manifest fails the load with an
  error naming the file.
- The [organization policy](/ai-rulez/policy/) bounds plugin hooks and MCP servers (`AR748`); `sources.deny_digests` and
  [approvals](/ai-rulez/approvals/) cover `marketplace` and `installed-plugin` entries.

## Lock, update and refresh

`ai-rulez lock` pins remote marketplaces and plugins as `[[marketplace]]` and `[[plugin]]` entries. A plugin from a
marketplace is keyed `marketplace:<name>` and records where the index pointed as `origin`:

```toml
[[marketplace]]
name = 'acme'
source = 'https://github.com/acme/plugins'
commit = 'c13e0a4aef9a900cdca038afcf128cd21c3b1993'
digest = 'sha256:43808c6c446c9c2049b656805e85b6844a7dc013c16f85ce647318a32526dbdf'

[[plugin]]
name = 'formatter'
source = 'marketplace:acme'
commit = 'c13e0a4aef9a900cdca038afcf128cd21c3b1993'
digest = 'sha256:143280174a3af8f900dc0b4ba618ed2eb9e9c8b42d2270e1cebaacfb2a9c7627'
origin = 'https://github.com/acme/plugins#plugins/formatter'
```

| Source | What the lock records |
| --- | --- |
| Remote repository | `[[marketplace]]` / `[[plugin]]` with commit and tree digest |
| `https://.../marketplace.json` | A digest, no commit |
| Local directory inside the project | A `local-marketplace` or `local-plugin` item with the directory's tree digest (a local marketplace's plugins are covered by it) |

Editing any file of a local plugin, such as `hooks/hooks.json` or a hook script, makes `lock --check` and
`generate --locked` fail until you run `ai-rulez lock` again.

| Command | Effect |
| --- | --- |
| `ai-rulez lock --check` | Compare the lock offline |
| `ai-rulez lock [name] --kind marketplace\|plugin` | Refresh pins; refreshing a marketplace refreshes its plugins |
| `ai-rulez marketplace update [name...]`, `ai-rulez plugin update [name...]` | Re-pin plain refs to the current commit |
| `ai-rulez update --kind marketplace\|plugin [name]` | Move `version` entries to the newest allowed tag |
| `ai-rulez generate --locked` / `--frozen` | Fail on a missing, stale or mismatching pin (a moved branch, a retagged release, changed bytes) |
| `ai-rulez generate --offline` | Read the cache only |

`ai-rulez sbom` and `ai-rulez approve` list these entries. See [Lock file](/ai-rulez/lockfile/).

## Commands

| Command | Purpose |
| --- | --- |
| `ai-rulez marketplace add <name> --source <src>` | Add a marketplace (`--path`, `--ref`, `--version`, `--trust`, ...) |
| `ai-rulez marketplace list`, `remove <name>` | List or remove marketplaces |
| `ai-rulez plugin install <name> --from <marketplace>` | Install a plugin from a marketplace |
| `ai-rulez plugin install <name> --source <src>` | Install a plugin from a repository or directory |
| `ai-rulez plugin list`, `remove <name>` | List or remove plugins |

```bash
ai-rulez plugin install tools --source acme/tools --exclude hooks
ai-rulez plugin install tools --source acme/tools --enable-hooks --name-prefix acme-
```

Full flag tables: [CLI reference](/ai-rulez/cli-reference/#ai-rulez-plugin-install). To change an entry on one machine only,
edit `config.local.toml` (see [Local configuration](/ai-rulez/local-overrides/)).

To enable a plugin in a harness's own settings without importing it, use
[`[[enabled_plugins]]`](/ai-rulez/configuration/#enabled_plugins) (see [Migrating to v5](/ai-rulez/migration-v5/)).
