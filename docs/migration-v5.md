# Migrating to v5

This page collects the breaking changes of the v5 release. It is a work in progress: entries are added as
changes land, and each links to the detail in the [changelog](CHANGELOG.md).

Run `ai-rulez doctor` first. It reports removed presets with a replacement, drifted outputs and generated paths
git does not ignore. Then run `ai-rulez generate`, review the diff and commit the sources and outputs together.

## Presets

| Change | Action |
| ------ | ------ |
| `windsurf` is renamed `devin`. The output directory `.windsurf/` is now `.devin/` and the agent frontmatter key `windsurf_model` is now `devin_model`. There is no alias. See [changelog](CHANGELOG.md#unreleased), "`windsurf` is renamed `devin`". | Rename the preset in `config.toml`, rename `windsurf_model` keys in agent files, and delete the old `.windsurf/` outputs. |
| `continue-dev` is removed. It has no replacement. See [changelog](CHANGELOG.md#unreleased), "Removed". | Remove it from `presets`. `doctor` reports it as an error. |
| `antigravity` no longer adds the `ai-rulez` MCP server on its own. | Set `[mcp] self_server = true` to keep it, as for every other preset. An entry an earlier version wrote into `.agents/mcp_config.json` is removed on the next `generate` unless you set it. |
| `amp` no longer writes `.agents/agents`, which Amp does not read. | None. Agents are listed in `AGENTS.md`. `amp_model` has no effect. |
| `codex` and `antigravity` write commands as skills (`.agents/skills/<id>/SKILL.md`) instead of `.codex/prompts` and workflows. | None. Files from the old layout are removed on `generate`. |
| `codex` writes skills to `.agents/skills`, not `.codex/skills`. | Set `codex_skills_dir = ".codex/skills"` to keep the old location. |
| `cursor` user-level skills go to `~/.agents/skills`, shared with `codex`, `gemini` and `pi`. | Run `generate --user`; `clean --user` removes the old copies recorded in the manifest. |
| `opencode` writes MCP servers as `mcp.<name>` with `enabled`, not `mcp.servers.<name>` with `disabled`. | None. Members recorded under `mcp.servers` are removed. |
| `zoocode` inlines rules into `AGENTS.md` and no longer writes `.roo/rules`. | None. |

## Generation behavior

- **Divergent shared outputs fail.** When two presets render different content to the same path, `generate`
  fails and names them instead of keeping the last one. Make the outputs agree, for example with
  `agents_md = true` or `rules.mode = "inline"`. See [Supported harnesses](harnesses.md#cross-cutting-behavior).
- **Check outputs are committed, not gitignored.** Hosted reviewers read checks from the base branch, so the
  files under [Checks](checks.md) are never added to the managed `.gitignore` block. An entry an earlier version
  added is removed on the next `generate`.
- **`generate --dry-run` prints `unchanged:` and `edited:`** for files that need no write. Scripts that match
  `write-file:` for every file need updating. See the [changelog](CHANGELOG.md#unreleased).
- **`tokens` totals include the item listing.** `always` and `headline_always` now count the skill, command and
  agent listing, so `--budget` can fail where it passed. The previous figures are `always_legacy`,
  `conditional_legacy` and `headline_always_legacy`.
- **Skill `evals/` directories are not bundled into plugins** unless `[plugin] include_evals = true`.

## Go module path

The Go module is now `github.com/Goldziher/ai-rulez/v5`. Code that imports ai-rulez packages, or installs the
CLI with `go install`, must use the `/v5` path (for example `go install github.com/Goldziher/ai-rulez/v5/cmd@latest`).
The npm, PyPI and Homebrew distributions are unaffected.

## Where to look

- [Supported harnesses](harnesses.md): the preset list and what each writes.
- [Hooks, permissions and settings keys](settings.md), [Permissions](permissions.md) and
  [User-level configuration](user-scope.md): new top-level `[[hooks]]`, `[permissions]` and `generate --user`.
- [Changelog](CHANGELOG.md#unreleased): the complete list under "Changed" and "Removed" of the unreleased version.
