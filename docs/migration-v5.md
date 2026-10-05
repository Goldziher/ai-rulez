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

## Lock file and enforcement

- **`ai-rulez.lock` pins content, under one hashing scheme.** `lock` now also pins the ai-rulez version, every
  authored rule, context file, skill, agent, command, hook, role and settings source, the generated outputs and
  served skills, as `sha256:` digests. Remote includes, OKF includes, installed skills and skill sources use the
  same scheme (there is no second, older per-file hash). Scripts (`.sh`, `.py`, `.js`, ...) are hashed byte for byte:
  a changed line ending in a script is a changed digest. `lock --check` exits 2 on a lock without content pins,
  whatever `[lock] enforce` says. A lock with another `version` is refused with the instruction to run
  `ai-rulez lock` again. Run it once, review the diff and commit the file. See [Lock file](lockfile.md).
- **`[lock] enforce = true` is strict.** It makes `validate --strict` report `AR981` (source drift) and `AR982`
  (output drift), makes `generate --locked` fail on drift, and makes the skills server refuse a served skill that
  the lock does not pin or whose digest differs. A corrupt lock, a lock of another `version` or a source that cannot be
  snapshotted is an `AR981` finding, not a logged skip.
- **The lock digest ignores the project-wide `Source-Hash` header**, so editing an unrelated file no longer changes
  the digest of a served skill.

## Trust rule for `[llm]` and `[telemetry]`

`allow_network`, `base_url`, `api_key_env` and the price overrides of `[llm]`, and the egress keys of `[telemetry]`
(`allow_network`, `otlp_endpoint`, `headers_env`, ...), are honoured only from the user config file
(`~/.config/ai-rulez/config.toml`) or the `AI_RULEZ_LLM_*` / `AI_RULEZ_TELEMETRY_*` variables. A repository
`config.toml` or `config.local.*` that sets them is ignored and reported by `llm doctor`, `telemetry doctor`,
`ai-rulez doctor` and the strict findings `AR9L1` and `AR9K1`. A literal key in either table is `AR9L0` or
`AR9K0`. If a repository relied on setting these, move them to the user config file. See [LLM access](llm.md)
and [Telemetry](telemetry.md).

## Exit codes

The new commands follow one contract: `0` success, `1` the command could not run (invalid configuration, missing
input), `2` findings, drift or a failed gate. This applies to `lock --check`, `generate --check`/`--locked`,
`validate --strict` (including the new `AR9*` families), `verifiers run`, `eval run`, `cost --budget` and
`tokens --budget`. Scripts that treated every non-zero status alike are unaffected; scripts that
matched `1` for a drift result need to match `2`.

## Hooks, validation and the rule registry

- **`validate --strict` has more rules.** About thirty new codes (`AR304`, `AR305`, `AR403`, `AR504`-`AR507`,
  `AR601`, `AR602`, `AR805`, `AR963`, `AR964`, `AR971`-`AR982`, `AR9A0`-`AR9B9`, `AR9K*`, `AR9L*` and more) can
  fail a CI job that passed before. Every code is listed in [Strict validation](strict-validation.md); lower
  or turn off a rule with `[lint.severity]`, or accept current findings with `validate --strict --update-baseline`.
  Rule codes are stable and are never renumbered.
- **Top-level `[[hooks]]` validation is stricter**: a `script` outside `A-Za-z0-9._/-` is rejected, a missing or
  non-executable script is `AR504`/`AR505`, and every generated shell line quotes the path.
- **Served skills (`delivery = "served"`) are left out of the harness skill trees** and reach the model through
  `ai-rulez mcp --serve-skills`; a static reference to one is `AR990`. Skills default to `static`, so nothing
  changes until you opt in.
- **The usage log's `session` field is a salted hash** of the harness session id, not the raw id.

## Go module path

The Go module is now `github.com/Goldziher/ai-rulez/v5`. Code that imports ai-rulez packages, or installs the
CLI with `go install`, must use the `/v5` path (for example `go install github.com/Goldziher/ai-rulez/v5/cmd@latest`).
The npm, PyPI and Homebrew distributions are unaffected.

## Where to look

- [Supported harnesses](harnesses.md): the preset list and what each writes.
- [Hooks, permissions and settings keys](settings.md), [Permissions](permissions.md) and
  [User-level configuration](user-scope.md): new top-level `[[hooks]]`, `[permissions]` and `generate --user`.
- [Changelog](CHANGELOG.md#unreleased): the complete list under "Changed" and "Removed" of the unreleased version.
