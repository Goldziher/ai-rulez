# Installed Plugins and Marketplaces

If a harness can install a plugin or a marketplace, ai-rulez can import it, use it and lock it. An
`[[installed_marketplaces]]` entry names a plugin marketplace; an `[[installed_plugins]]` entry imports one
plugin, from a marketplace or straight from a repository. The skills, agents, commands, rules, hooks and MCP servers
the plugin ships become ordinary ai-rulez content, rendered into every preset you generate, pinned in
`ai-rulez.lock` and covered by the same security scan as [installed skills](installed-skills.md).

This is the consumer side. To build a plugin from your own content, see [Plugins](plugins.md)
(`ai-rulez generate --plugin`).

## Quick start

```toml
[[installed_marketplaces]]
name = "acme"
source = "acme/plugins"          # GitHub owner/repo; or a git URL, ./local/dir, https://.../marketplace.json
version = "^1.2"                 # optional: a tag range, moved only by `ai-rulez update`

[[installed_plugins]]
name = "formatter"
marketplace = "acme"
exclude = ["hooks"]              # skills, agents, commands, hooks, mcp, rules

[[installed_plugins]]
name = "review-tools"
source = "https://github.com/acme/review-tools"   # a plugin repository, no marketplace
ref = "v2.0.1"
enable_hooks = true              # hooks are off unless you opt in
```

```bash
ai-rulez marketplace add acme --source acme/plugins
ai-rulez plugin install formatter --from acme
ai-rulez lock          # pins the marketplace and the plugin (commit + tree digest), scans both
ai-rulez generate
```

## Formats

| Harness | Marketplace index | Plugin manifest | What is imported |
| ------- | ----------------- | --------------- | ---------------- |
| Claude Code | `.claude-plugin/marketplace.json` | `.claude-plugin/plugin.json` (optional) | `skills/`, `agents/`, `commands/`, `hooks/hooks.json`, `.mcp.json`, plus the `skills`, `agents`, `commands`, `hooks` and `mcpServers` paths or inline objects the manifest names |
| Codex | `.agents/plugins/marketplace.json` | `.codex-plugin/plugin.json` | `skills/`, `.mcp.json`, `hooks.json` and the manifest's `skills`, `mcpServers` and `hooks` fields |
| Cursor | `.cursor-plugin/marketplace.json` | `.cursor-plugin/plugin.json` | `rules/` (as rules), `agents/`, `skills/`, `commands/`, `hooks/hooks.json`, `.mcp.json` or `mcp.json` |
| Gemini CLI extension | none | `gemini-extension.json` | `mcpServers`, the `contextFileName` file(s) (as a rule), `commands/*.toml` (as commands), `skills/`, `agents/` |
| Agent Plugins | none | root `plugin.json` with an `agent-plugins.org` `$schema` | `skills/`, `mcp.json` and the `com.anthropic.claude-code` (agents, commands, hooks) and `io.github.goldziher.ai-rulez` (rules) extension namespaces |

The layout is detected from the manifest; `format` forces it. A directory with Claude's default directories and no
manifest is a Claude plugin, as Claude Code treats it. A marketplace entry with `strict = false` that declares its
own components needs no manifest in the plugin directory.

Skills are bundled whole, exactly like an [installed skill](installed-skills.md#what-is-bundled-with-a-skill).
Agent, command and rule files keep their frontmatter. A nested command `git/commit.md` is named `git-commit`. A
Gemini TOML command becomes a prompt: its `{{args}}` and `!{...}` placeholders are kept as written.

## Source forms

`source` of a marketplace or plugin accepts:

| Form | Example |
| ---- | ------- |
| GitHub shorthand | `acme/plugins`, `github:acme/plugins` |
| Git URL | `https://github.com/acme/plugins`, `git@github.com:acme/plugins.git`, `file:///srv/plugins` |
| Local directory | `./vendor/marketplace` (must start with `./`, `../` or `/`; inside the project) |
| Index URL (marketplace only) | `https://example.com/marketplace.json` |

A bare `owner/repo` is GitHub, as in Claude Code. The same safety rules as installed skills apply: https only (no
`http://`), no credentials in a URL, no git option or transport helper such as `ext::`, no `..` in a `path`, and a
local path from the committed config must stay inside the project (the machine-local overlay and the user config may
leave it). `file://` repositories outside the project need `AI_RULEZ_ALLOW_FILE_URLS=1`.

A marketplace entry names its plugin as:

- a relative path inside the marketplace repository (`"./plugins/x"`; `metadata.pluginRoot` is prepended to a bare
  name),
- `{"source": "github", "repo": "owner/repo", "ref": "...", "sha": "..."}`,
- `{"source": "url" | "git", "url": "...", "ref": "...", "sha": "..."}`,
- `{"source": "git-subdir", "url": "...", "path": "sub/dir"}`, or Codex's `{"source": "local", "path": "./x"}`.

A relative path that is absolute, contains `..` or goes through a symlink is refused, so an index cannot make
ai-rulez read outside the marketplace repository. An entry with a source type ai-rulez does not support (npm, pip) is
listed as not installable.

## Entry fields

`[[installed_marketplaces]]`: `name`, `source`, `path` (directory holding the index), `ref`, `version`, `tag_prefix`,
`include_prerelease`, `min_release_age`, `trust`.

`[[installed_plugins]]`: `name`, then either `marketplace` (and `plugin` when the entry is called otherwise) or
`source`, `path`, `ref`, `version`, `tag_prefix`, `include_prerelease`, `min_release_age`; and `format`, `include` /
`exclude` (component kinds), `name_prefix`, `domain`, `profiles`, `trust`, `enable_hooks` (default `false`),
`enable_mcp` (default `true`).

A plugin installed from a marketplace follows the marketplace's `ref`/`version`; setting them on the plugin is an
error. `domain` and `profiles` scope skills, agents, commands, rules and MCP servers like
[installed skills](installed-skills.md); hooks are global. A name that already exists (local content first, then
earlier plugins) wins and the later one is skipped with a warning; use `name_prefix` to keep both. The prefix is part of the name and of the skill's output directory
(`.claude/skills/<prefix><dir>/`).

## Hooks, MCP servers and permissions

- **Hooks are off by default.** A hook runs a command on every machine that uses the project, so, as with
  `convert --enable-hooks`, they are imported only with `enable_hooks = true`. A plugin with hooks that are not
  enabled is reported once. Imported hooks are restricted to the harness they were written for.
- **MCP servers** are imported unless `enable_mcp = false`. Literal credentials in `env`, `args`, `url` or `headers` are
  replaced by `${VAR}` references and reported; the servers go through the same MCP checks and `[mcp]` policy as your
  own.
- **`${CLAUDE_PLUGIN_ROOT}`.** The plugin directory does not exist in the generated tree, so a hook or MCP server that
  refers to it (`${CLAUDE_PLUGIN_ROOT}`, `${CODEX_PLUGIN_ROOT}`, `${PLUGIN_ROOT}`, Gemini's `${extensionPath}`) is
  rewritten to the project-relative path of the plugin only when the plugin is a local directory inside the project
  (`source = "./vendor/tools"`). For a fetched plugin the hook or server is refused and reported
  (`AR9F3`), never written pointing at nothing. `${CLAUDE_PLUGIN_DATA}` is always refused.
- **Permissions** are never imported: a plugin cannot add an allow rule.

## Security model

- Fetches use the include machinery: the same git cache, credential handling (a token goes only to allowlisted hosts) and
  `sources.allowed_hosts` / `sources.deny_hosts` policy, which also applies to a repository a marketplace names.
- Before `ai-rulez lock` pins a new tree it runs the security scan (AR001-AR009) over every file of the
  marketplace or plugin, hook scripts and MCP definitions included; error findings refuse the pin (exit 2) unless
  `--accept-findings`. The entry's `trust` sets the level (`error` makes every finding an error; a plugin inherits its
  marketplace's).
- `generate` scans the imported content again, plus the hook documents, MCP documents and the scripts they run, at the
  same level (`[lint.security] scan_imports`).
- Symlinks are never followed, files and manifests are size-bounded, names are validated, and a malformed manifest
  fails the load with an error naming the file.
- The organization policy's hooks and `[mcp]` limits bound plugin hooks and servers (AR748); `sources.deny_digests` and
  [approvals](approvals.md) apply to `marketplace` and `installed-plugin` entries.

## Lock, update and refresh

`ai-rulez lock` pins marketplaces and plugins fetched from a remote repository or an https URL in `ai-rulez.lock`, as
`[[marketplace]]` and `[[plugin]]` entries: the commit, the requested ref or constraint, the resolved tag and a tree
digest. A plugin installed from a marketplace is keyed
`marketplace:<name>` and records the repository the index named for it as `origin`; a plugin inside the marketplace
repository shares its commit and has its own digest. An https `marketplace.json` has a digest and no commit.

- **What is pinned, by source kind.** A remote repository: `[[marketplace]]` / `[[plugin]]` entries with commit and
  tree digest. An https `marketplace.json`: a digest, no commit. A local directory inside the project
  (`source = "./vendor/market"`): no `[[marketplace]]` or `[[plugin]]` entry, but a `local-marketplace` or
  `local-plugin` item holding the tree digest of the directory (a plugin of a local marketplace is covered by the
  marketplace item). Editing `hooks/hooks.json`, a hook script or any other file in it makes `lock --check` and
  `generate --locked` fail until `ai-rulez lock` is rerun. An installed skill read from a local directory is pinned
  as a `local-skill` item.
- `generate --locked` / `--frozen` and an enforced lock fail on a missing, stale or mismatching pin (a moved branch, a
  retagged release, changed bytes); `--offline` reads the cache only.
- `ai-rulez lock --check` compares the lock offline; `ai-rulez lock [name]` / `--kind marketplace|plugin` refreshes.
  Refreshing a marketplace refreshes the plugins installed from it.
- `ai-rulez update [--kind marketplace|plugin] [name]` moves entries that use a `version` range to the newest allowed tag
  (see [Lock file](lockfile.md)); `marketplace update` and `plugin update` re-pin plain refs.
- `ai-rulez sbom` and `approve` list these entries.

## CLI

```bash
ai-rulez marketplace add|remove|list|update
ai-rulez plugin install|remove|list|update
```

See the [CLI reference](cli.md). To change an entry for one machine only, edit `config.local.toml`.

## Migrating from `[[plugins]]` / `[[marketplaces]]`

In v4 the `[[plugins]]` and `[[marketplaces]]` tables described plugins to *enable* in a harness and were never read
by v5 beyond a warning. They do not import anything. Replace a `[[marketplaces]]` entry by
`[[installed_marketplaces]]` (`name`, `source`) and a `[[plugins]]` entry `name@marketplace` by
`[[installed_plugins]]` with `marketplace` set. Enabling a plugin in the harness itself is separate: keep that in the
harness settings.
