# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project adheres to Semantic Versioning.

## [Unreleased]

## [5.9.2] - 2026-10-28

### Fixed

- **The macOS binaries run on macOS 14 again, and the release builds without the retiring `macos-14` runner.**
  The minimum macOS of a darwin binary followed the runner image that built it, so 5.9.0's Intel binary required
  macOS 15. Both darwin builds now run on the macOS 26 runners (`macos-26`, `macos-26-intel`; GitHub retires `macos-14`
  on 2026-11-02) and target macOS 14.0 (`MACOSX_DEPLOYMENT_TARGET`), and the publish workflow rejects a darwin binary
  that needs a newer macOS. 5.9.1 was never published as a GitHub release (its Apple silicon build got no runner), so its
  npm and PyPI packages cannot download a binary; use 5.9.2.

## [5.9.1] - 2026-10-28

### Fixed

- **`AR964` counts each skill once in the Codex skill listing.** A copy of a skill under another directory, such
  as a public mirror at `skills/<name>/SKILL.md`, a plugin bundle's `.ai-rulez/skills` or its harness outputs, was
  added to the `codex-skill-listing` total again, although Codex lists one skill per name. The total now counts
  each skill name once and prefers the configured copy, so projects no longer have to raise
  `[lint.load_budgets] codex-skill-listing` to stay clean ([#513](https://github.com/Goldziher/ai-rulez/issues/513)).

## [5.9.0] - 2026-10-28

### Added

- **`[exposure]` area map and `AR9W5 exposure-map-invalid`.** A new `[exposure]` table describes the areas of a
  repository: the directories each one claims (`!` excludes, a trailing `*` name prefix, longest claim wins), its
  owner, the words that name it and the ones it shares with other areas, inline as `[[exposure.areas]]` and
  `[exposure.skill_areas]` or in an `areas_file`. Terms match case-insensitively, as substrings when they hold `_`,
  `/` or `.` and as whole words otherwise, outside repository path literals; an artifact belongs to the most specific
  path claim, else the area whose terms dominate its text, and a skill's owner is its `skill_areas` entry, then the
  path claim (a plugin skill's: its plugin's directory), then the terms. `validate` reports a map it cannot use
  (invalid scalar keys, duplicate, reserved or malformed names, bad path claims, `skill_areas` values naming no area,
  an unreadable or misspelled `areas_file`) as `AR9W5`, an error, on the line of the offending entry; without an
  `[exposure]` table nothing is checked. See [Session exposure and areas](https://goldziher.github.io/ai-rulez/exposure/).
- **`ai-rulez exposure inventory` and `ai-rulez exposure areas`.** `exposure inventory` lists every Claude Code
  artifact a session started at the repository root can see (CLAUDE.md files, rules, skills, commands, agents,
  `.mcp.json` servers, settings and enabled plugins) with its phase (start, on demand, manual or unknown),
  visibility, listing text, estimated tokens and frontmatter status, after `skillOverrides`, `claudeMdExcludes` and
  the repository marketplace (`--plugins none|enabled|all`, `--with-text`). `exposure areas` shows the area map as
  loaded and the area of every artifact with how it was decided (`skill_areas`, `path`, `terms` or `none`), for
  diffing against another classifier; a plugin skill is owned by its plugin's directory. Both take `--areas FILE` to
  try a map before configuring it and `--root DIR` (which still honours `-C/--config`, `--config-dir` and
  `--no-local`, and otherwise discovers the configuration from `DIR` upwards; the path claims stay relative to the
  project root, so `--root services/billing` matches the claim `services/billing`), and print
  `schema/exposure-inventory.v1.schema.json` and `schema/exposure-areas.v1.schema.json` with `--format json`.
  Tokens are estimated as bytes / 3.9 rounded up; a file over the read limit is kept unread and estimated from its
  size. Outside a git repository the file walk lists symlinked files, as git does, so `CLAUDE.md -> AGENTS.md` is
  read there too.
- **`ai-rulez exposure`: the start-of-session report, `AR9W3 exposure-start-budget` and the Claude skill listing
  budget.** Run without a subcommand, `exposure` reports what a Claude Code session started at the repository root
  costs before it touches a file: the estimated start tokens in total and by kind (a root artifact of phase
  `unknown` counts as start), the skill listing against Claude Code's listing budget (1% of the context window at 4
  characters per token, 8000 characters at 200000 tokens), the capped descriptions, the artifacts and tokens by phase
  and kind, and the heaviest artifacts at start (`--context-window N`, `--plugins none|enabled|all`,
  `--harness claude`, `--areas FILE`, `--root DIR`; `--format json` prints `schema/exposure-report.v1.schema.json`).
  It exits 2 when the start tokens exceed the new `[exposure] start_budget`, which `validate` also reports as
  `AR9W3`, an error, off while `start_budget` is unset. With an `[exposure]` table and the claude preset, `AR964`
  gains the row `claude-skill-listing-total` (warning past the budget, info from 90%), measured over the skills a
  session lists at start with `[exposure] plugins` and `context_window`; `[lint.load_budgets]
  claude-skill-listing-total` sets a fixed budget instead. Both findings sit on the configuration and, under
  `validate --changed`, are kept when any start-of-session artifact, the configuration, the root settings or the
  marketplace changed, so the change that pushes a session past the budget does not pass unseen.
- **Area term leakage: `AR9W0`, `AR9W1`, `AR9W6` and `ai-rulez exposure leakage`.** With an `[exposure]` area map,
  `validate` reads the text a Claude Code session sees at its start (the root `CLAUDE.md`, unconditional root rules,
  the skill and agent listing lines, `.mcp.json` server names, enabled plugin keys; an artifact of unknown phase
  counts as start) and reports each term of an area that is not shared, outside path literals, as `AR9W0`
  (`exposure-area-term`, an error), and each listed skill or agent that names only ambiguous words as `AR9W1`
  (`exposure-area-term-ambiguous`, a warning). The message `<kind> "<name>" at session start names term "<term>" of
  area "<area>"` is a stable contract, so `[[lint.accept]]` entries accept findings by artifact or term, and an entry
  that accepts nothing is `AR9I0`. A plugin both `.claude/settings.json` and `.claude/settings.local.json` enable is
  reported on the tracked `settings.json`. `AR9W6` (`exposure-skill-area-unknown`, a warning) reports a
  `skill_areas` key that names no skill. Under `--changed`, every leak is kept when the areas file, the
  configuration, the root settings or the marketplace change. `ai-rulez exposure leakage` prints the same findings
  with their acceptance, the start-of-session artifacts whose file could not be read (`unchecked`, not findings) and
  the stale acceptances (`--areas`, `--plugins`, `--root DIR` to inspect the session of `DIR` with paths relative to
  it; `schema/exposure-leakage.v1.schema.json` with `--format json`) and exits `2` on a violation or a stale
  acceptance. See [Area term leakage](https://goldziher.github.io/ai-rulez/exposure/#area-term-leakage).
- **`ai-rulez exposure collisions` and `AR9W2 exposure-area-collision`.** `exposure collisions` compares the listing
  of every skill and agent a session sees at start with every other one of a different area (unclassified counts as
  one) and ranks the pairs by the Jaccard index of their words (lowercase runs of a letter and two or more letters or
  digits, less common stopwords), then by path, with the shared words (`--min-score`, default `[exposure]
  collision_min_score` or 0.1; `--limit`, default 20, 0 for all; `--areas`, `--root`, `--plugins`;
  `schema/exposure-collisions.v1.schema.json` with `--format json`). `validate` reports the same pairs as `AR9W2`, off
  until `[exposure] collision_min_score` is set, which turns it on at warning: the 20 highest on the description line
  of the pair's first listing, and the rest in one summary finding on the configuration. Under `--changed` a pair's
  finding is kept when either listing, the area map or the configuration changed.
- **`AR9W4 exposure-gate-unreadable`.** `validate` warns about a skill, command or rule whose frontmatter does not
  parse (`AR306`) while a line of the block names `paths`, `disable-model-invocation` or `user-invocable`, and about
  one whose block parses but sets `paths` or `disable-model-invocation` to a value of the wrong type (`paths: 5`,
  `disable-model-invocation: yes`): Claude Code recovers some broken blocks and keeps the gate, and drops others
  whole, so the item may load at session start in every session. Reported on the line of the first gate key, for the
  claude preset and for a run without a configuration; no `[exposure]` table is needed. `exposure inventory` gives
  such a root item the phase `unknown`, counted as start. See
  [A gate that cannot be read](https://goldziher.github.io/ai-rulez/exposure/#a-gate-that-cannot-be-read), which also
  records how Claude Code reads the YAML 1.1 booleans `yes`, `on` and `no`.

### Changed

- **Repository index items keep the frontmatter parse error.** An item whose frontmatter is not valid YAML, is not a
  mapping or never closes is indexed as before (a value of the wrong type or a complex key still keeps the other
  keys, a syntax error keeps none), but now carries the decode error instead of dropping it, so a reader can tell a
  lost `paths:` gate from an absent one. The index cache format moves to version 5, so a cache written by an earlier
  version is rebuilt once.

### Fixed

- **`find_context` no longer returns a rule or skill whose `paths:` gate cannot be read for every path.** A Claude
  Code rule whose frontmatter names `paths:` but does not parse (an unquoted `:` in a value, an unclosed quote or
  bracket, a tab indent, a block that never closes), or a rule or skill whose `paths` value is neither a string nor a
  list of strings (`paths: 5`), was indexed with no restriction, so a `path` query returned it for any file, although
  Claude Code may still gate it. Where it applies cannot be known, so it now fails closed: a `path` query never
  returns it, and a `scope` query lists it with `"paths_unreadable": true`. A rule whose broken block names no
  `paths:` and a rule whose YAML type error still decodes its `paths:` are unchanged, as is loading by name; a skill
  whose frontmatter does not parse was, and is, not served. A Cursor rule whose broken block names `globs:` still
  applies everywhere (a known limitation). See [Monorepos: scopes and nested
  context](https://goldziher.github.io/ai-rulez/mcp-server/#monorepos-scopes-and-nested-context).
- **Microsoft Defender no longer reports the binary or a checkout as malware.** The patterns of the malware rules
  `AR046` to `AR049` and their test cases quoted web-shell, reverse-shell, miner and credential-dump payloads in plain
  text, which anti-malware scanners match. They are now embedded obscured (`internal/lint/ar_malware_rules.dat`,
  `internal/lint/testdata/ar_malware_cases.dat`), the rule descriptions and documentation describe the payloads
  without quoting them, and `go run ./scripts/obscure decode|encode` edits the tables. Detection is unchanged
  ([#512](https://github.com/Goldziher/ai-rulez/issues/512)).

## [5.8.0] - 2026-10-27

### Added

- **`[[enabled_plugins]]` enables a plugin from another marketplace for Claude Code, Codex and OpenCode from one
  declaration.** `id = "basemind@basemind"` with an optional `marketplace = { name, source, type }` writes
  `enabledPlugins` and `extraKnownMarketplaces` in `.claude/settings.json`, a `[plugins."<id>"]` table with
  `enabled = true` in `.codex/config.toml`, and, with `opencode = "<npm package>"`, the package in the `plugin` array
  of `opencode.json`. ai-rulez owns only the entries it writes, and `generate --user` writes the same entries to the
  user's files. Ids, marketplace sources (`github`, `git`, `url`, `directory`; no `http://` or `git://`) and npm
  packages are validated at load. An id that is also in `[claude.settings] disable_plugins`, a marketplace named
  like the project's registered own `[marketplace]`, and a `directory` marketplace in the user config are errors;
  an id also in `enable_plugins` is a warning. See `docs/plugins.md`
  ([#500](https://github.com/Goldziher/ai-rulez/issues/500)).
- **`Goldziher/ai-rulez/check@v5` runs the full governance gate in one step.** The sub-action installs the CLI with
  the root action's resolve, cache and checksum-verified download scripts, then for every configuration root runs
  `migrate okf --check`, `validate --strict --no-local`, `lock --check` (when a lock exists), `generate` and
  `generate --check` with `--strict-config --no-local`, and optionally `verify --plugin` and `tokens --budget`.
  Afterwards `git diff --exit-code HEAD` must be clean, no new untracked file may appear, and at least one tracked file
  must be examined. Roots come from `config-directories` or from every tracked configuration;
  `expected-config-count` fails a run that finds a different number. A job summary lists every command, the roots
  checked and the files examined. Inputs: `version`, `config-directories`, `expected-config-count`, `plugin`,
  `strict`, `local` and `token-budget` ([#505](https://github.com/Goldziher/ai-rulez/issues/505)).
- **`Goldziher/ai-rulez/update@v5` keeps version-range pins current on a schedule.** It runs
  `ai-rulez lock --outdated --format json` and stops with a notice when nothing is updatable; otherwise it runs
  `ai-rulez update` (a pin the pre-pin scan refuses fails the job), `generate --no-local` and `lock --content-only`,
  commits the result as `github-actions[bot]` and opens or refreshes a pull request from `ai-rulez/update` with the
  outdated table in its body, or pushes to the checked-out branch with `direct: true`. The branch is replaced with
  `--force-with-lease`, and the token reaches git and `gh` only through the environment. Inputs: `version`, `names`,
  `kind`, `direct`, `branch`, `token` and `working-directory`; `min_release_age` stays a per-source configuration
  setting because `update` has no flag for it ([#506](https://github.com/Goldziher/ai-rulez/issues/506)).
- **MCP tools `lock_outdated`, `update_plan`, `update` and `lock_diff` mirror `lock --outdated`, `update --dry-run`,
  `update` and `lock --diff`.** An agent maintaining an organization's governance could see that a lock was stale
  (`lock_status`) but not which sources had newer tags, or move them, without the CLI. The tools run the same code as
  the commands (now `internal/pinupdate`), so `update` keeps the pre-pin security scan and the refusals (`AR730`,
  `AR731`, `AR732`, scan findings), returned as an error result that carries the update document and writes nothing.
  The consents that override a refusal (`--accept-moved-tag`, `--accept-findings`, `--allow-downgrade`) are not tool
  arguments. With `[mcp] read_only = true` only `lock_outdated`, `update_plan` and `lock_diff` are listed. The first
  three read the remotes' tags over the network. See [Pin Tools](https://goldziher.github.io/ai-rulez/mcp-server/#pin-tools-lock_outdated-update_plan-update)
  ([#502](https://github.com/Goldziher/ai-rulez/issues/502)).
- **`[rules] scoped_fallback_group` folds the `scoped_fallback = "skill"` skills into fewer ones.** The fallback wrote
  one `rule-<id>` skill per glob-scoped item, which pushed the Codex skill listing to 15,000-18,000 characters against
  a budget of about 8,000 in repositories with shared modules. `include` writes one `scoped-<include>` skill per include
  or builtin pack, `directory` one `scoped-<dir>` skill per directory the globs point into; a grouped skill holds each
  item under a heading with its globs, its `paths` are their union and its description stays under 160 characters. The
  default, `none`, leaves output unchanged
  ([#503](https://github.com/Goldziher/ai-rulez/issues/503)).
- **`fallback: false` in a rule or context frontmatter keeps a glob-scoped item out of the folderless-preset fallback.**
  It is neither inlined into the shared `AGENTS.md` nor written as a skill, so only presets with path scoping render it.
  `validate` rejects a value that is not a boolean (`AR304`)
  ([#503](https://github.com/Goldziher/ai-rulez/issues/503)).

### Changed

- **Dependencies are upgraded and the GitHub Actions are pinned to major tags.** Go modules include oras-go
  2.7.0, liter-llm 2.4.1 and tiktoken-go/tokenizer 0.10.0; the docs lockfile, pnpm 12.15.0 and golangci-lint
  v2.15.0 are current. Workflow and action references use `@vN` (`actions/checkout@v7`), and setup-uv keeps a full
  tag because it publishes no major tag.
- **The floating `v5` action tag is now an annotated tag pushed with `--force-with-lease`.** After a stable
  release publishes, the release workflow derives the tag from the release's major version (`v5.8.0` moves `v5`),
  writes an annotated tag and pushes it with `--force-with-lease=refs/tags/v5:<previous>`, so a tag moved by anyone
  else in between fails the push instead of being overwritten; the first push requires that the tag not exist.
  Release candidates still never move it, and the no-rollback checks are unchanged. `uses: Goldziher/ai-rulez@v5`
  with `version: latest` keeps a workflow on the newest 5.x action and CLI with no per-release edits
  ([#504](https://github.com/Goldziher/ai-rulez/issues/504)).
- **`AR964` counts the `scoped_fallback` skills in the Codex skill listing and warns past the limit.** The listing
  total included only authored skills and was reported as info; it now adds the generated `rule-*`, `context-*` and
  `scoped-*` skills, says how many of the listed skills they are, warns when the total exceeds the limit (so
  `validate --strict` fails) and reports info from 90% of it
  ([#503](https://github.com/Goldziher/ai-rulez/issues/503)).

### Removed

- **The `skillspector` scanner profile.** `profile = "skillspector"` in `[[lint.external]]` is no longer accepted: the configuration fails to load with `Value skillspector should be one of the allowed values: agnix, claude-plugin-validate, cisco-skill-scanner, snyk-agent-scan`. To keep running NVIDIA SkillSpector, declare the command in the entry yourself (`command = ["skillspector", "scan", "--no-llm", "--format", "sarif", "{skill_dirs}"]`, `format = "sarif"`, `inputs = ["skills"]`, `egress = false`, `batch_size = 1`, `version = ">=2, <3"`, `message_properties = ["evidence", "snippet"]`). The profile's `--no-llm` requirement, its `--transitive`, `--mcp-registry` and `--mcp-registry-compare` deny-list and its dropping of the `SC4` "OSV.dev unreachable" notice are not applied to such an entry, so keep `--no-llm` in the command yourself. The profile keys `required_flags` and `offline_notices`, and a profile-level default for `message_properties`, are gone with it; no other profile used them, and an entry's own `message_properties` still works. The `--competitors` run of `bench/judge` no longer compares against SkillSpector (`-skillspector-bin` is removed), and the SkillSpector test transcoder under `scripts/` is deleted. The checks and judge prompts derived from SkillSpector stay, with their Apache-2.0 notices.

### Fixed

- **The `update` sub-action also acts on sources that were never locked or whose locked tag is gone.** It counted only
  `updatable` rows from `lock --outdated`, so a run with only `not-locked` or `tag-missing` sources reported nothing
  to do although `ai-rulez update` pins them. Those rows now count, and the pull request body, job summary and commit
  message list them with a Status column.
- **`scoped_fallback` skill descriptions no longer split a multi-byte character.** Descriptions are cut at 160 or 250
  bytes, and the cut could land inside a Cyrillic letter or an emoji in an item title, leaving invalid UTF-8 in the
  `SKILL.md` frontmatter. The cut now falls on a character boundary.

- **`schema/lock-outdated.schema.json` and `schema/update.schema.json` accept the `marketplace` and `plugin` kinds.**
  `lock --outdated` and `update` report installed marketplaces and plugins that use a `version` constraint, but the
  schemas listed only `include`, `skill` and `source`, so a validator rejected that output. The enums now match the
  kinds `--kind` accepts.

- **`--config .agents` loads a `source = "agents"` project.** Naming the `.agents` folder (or `.agents/.ai-rulez`)
  failed with "path is outside the workspace" because the load workspace was rooted at `.agents` while the project
  directory is its parent; it is now rooted at the project, as `--config .agents/ai-rulez.toml` and `--config
  .ai-rulez` already were. The workspace still ends at the project, so a committed config cannot point outside it.

- **`generate --recursive` builds a plugin-authoring root as a plugin bundle, and the default self MCP server is never
  added to one.** A nested root such as `plugin/.ai-rulez/` that authors a plugin or marketplace and configures no
  preset got a normal generate, which wrote `npx -y ai-rulez@latest mcp` into the shipped `.mcp.json` and
  `.ai-rulez/.generated-manifest.local.json` and `.ai-rulez/local/` into its `.gitignore`, so `verify --plugin` failed.
  The recursive run now generates such a root as `generate --plugin` does, `generate --check --recursive` skips it with
  an info line (`verify --plugin` covers it), and a plugin-authoring root gets no self server whatever the mode
  ([#498](https://github.com/Goldziher/ai-rulez/issues/498)).
- **`[claude.settings] enable_plugins` and `disable_plugins` accept `name@marketplace` ids and no longer need a
  `[marketplace]` for them.** A qualified id was rejected as "not a valid plugin name", and any `manage = true` block
  failed with "needs a [marketplace] name" even when it listed only ids of another marketplace. Qualified ids are
  written as given; bare names are still qualified with the local marketplace, which is registered only when one is
  declared.
- **The managed `.gitignore` block is the same on every machine and with or without `--no-local`.** It listed
  `.ai-rulez/config.local.*` only while a `config.local.*` file existed, and the local manifest, the overlay-created
  outputs (`CLAUDE.local.md`, a hook-only `.claude/settings.json`) and their `*.local.*` patterns only when the run
  loaded local inputs, so each contributor's regeneration rewrote the committed file and `generate --check --no-local`
  in CI reported drift that was not real. With `gitignore = true` the block now always lists `config.local.*` (and its
  `.config.local.*` lock files), `local/` and the machine-local manifest, and never lists a path that only this
  machine's local inputs add: those go to `.git/info/exclude`. With `gitignore = false`, a machine-local manifest that
  is the only local file is excluded through `.git/info/exclude` too instead of creating a tracked `.gitignore`.
  Directory outputs only the overlay adds (`.gemini/agents/` from an overlay `gemini` preset) are excluded as
  `/dir/` patterns too; a directory the shared render also writes stays shared. Fixes #499.
- **`generate --recursive` writes each root's patterns only to the `.gitignore` of its own git repository.** It put
  every nested root's patterns into the `.gitignore` of the directory it ran in, even for a root with
  `gitignore = false` and for a nested repository whose own `.gitignore` was never written. Each repository now gets
  its own managed block holding the roots inside it, a nested directory with its own `.git` gets its own block, and a
  repository whose roots all set `gitignore = false` is left alone. `generate --check --recursive` judges each block.
  Fixes #497.
- **Roots of one repository no longer drop each other's `.git/info/exclude` block in `generate --recursive`.** The
  roots rewrote the shared exclude file concurrently, so one root's rewrite could remove another root's block (on
  Windows the rename failed with "Access is denied"). The rewrites are now serialized, and on Windows a rename that a
  `git` reading the file blocks is retried for up to about two seconds.
- **Built-in name tables match their upstream docs again.** Antigravity lists `/codesearch`, `/plan` and `/grill-me`
  in its CLI reference table and no longer lists `/fast` or `/planning`, so the hand-kept groups for the first three
  are folded into the generated one. Command Code adds `/btw`, Kimi Code `/import-sessions`, and Devin drops `/loop`
  and makes `/rename-session` and `/title` aliases of `/rename`. The Claude Code extractor reads the built-in
  subagents from bold headings as well as tabs, since the page dropped its tabs, and a `/design consent` subcommand row
  no longer turns the bundled `/design` skill into a command ([#496](https://github.com/Goldziher/ai-rulez/issues/496)).
- **A stale OKF `index.md` after a skill `description` edit is refreshed by `generate` and reported by `validate`
  and `generate --check`.** The indexes of a migrated configuration directory were rewritten only by `add`,
  `remove` and the MCP tools, so `migrate okf --check` alone went red on a commit every other check accepted.
  `generate` now refreshes them, `validate` reports a stale one as `AR9B0` at its `index.md`, and
  `generate --check` lists it as stale, all through the comparison `migrate okf --check` uses
  ([#501](https://github.com/Goldziher/ai-rulez/issues/501)).
- **`validate` no longer reads the `.ai-rulez.bak-okf-<timestamp>/` backup that `migrate okf` leaves.** Its copies
  of the skills were reported as `AR701` and `AR703` duplicates of the real ones; any `.<name>.bak-*` directory is
  now skipped ([#501](https://github.com/Goldziher/ai-rulez/issues/501)).
- **`AR964` counts skills and rules from includes and installed skills in the load budgets.** The Codex
  skill-listing and `AGENTS.md` chain totals summed only content authored under the config directory, so a project
  whose skills come from `[[includes]]` (remote, local or a `config.local.toml` overlay) or from installed skills got
  a figure far below what `generate` writes and Codex lists, or no finding. The totals now cover every item the
  preset emits, the message says how many skills come from includes, and an included skill over the Claude listing
  cap is reported on `config.toml` ([#508](https://github.com/Goldziher/ai-rulez/issues/508)).
- **An unknown `x-ai-rulez.kind` in OKF frontmatter, or a kind that disagrees with the concept's directory, is now
  reported.** `kind: bogus` in a `context/` concept passed `validate --strict` and `okf validate` with no findings and
  was rendered as context, because a configuration directory classifies by directory and ignored the value. Both
  commands (and the `okf_validate` tool) now report an unknown kind as an `AR9B1` error naming the allowed values
  (`rules`, `context`, `skills`, `agents`, `commands`, `checks`, `skill-resource`) and a known kind that names another
  directory, such as `kind: skill` under `context/`, as an `AR9B1` warning, which fails `--strict`
  ([#509](https://github.com/Goldziher/ai-rulez/issues/509)).
- **A local include honors `path`, and fails when it does not exist.** `path` was silently ignored for an include whose
  `source` is a directory, so the source root was loaded instead of the subdirectory, also when `path` named a
  directory that does not exist. The include now resolves `<source>/<path>` under the same containment and symlink
  rules as `source`, fails with an error naming the include and the missing path (as a git include does), and refuses
  a `path` that climbs out of the source with `..`. `validate`, the local-include pin in `ai-rulez.lock` (its path and
  digest cover only the subdirectory) and `include add --path` follow the same rule
  ([#510](https://github.com/Goldziher/ai-rulez/issues/510)).
- **`generate --allow-local-drift` works when an overlay replaces an include whose lock entry is stale.** An overlay
  that replaced a remote include with a local path could not be used while the shared config had a pending change for
  that include (a new `ref` or `version` range): the shared baseline render ran against the stale lock and aborted.
  With `--allow-local-drift` the baseline no longer enforces the lock for the includes the overlay replaces (their
  content is replaced anyway) and logs a warning that the lock is stale for them; everything else stays enforced, so
  machine-local outputs are still classified, excluded through `.git/info/exclude` and kept out of the shared
  `.gitignore`. Any other baseline failure still fails the run, and without the flag the error stays, its hint naming
  `--allow-local-drift` ([#507](https://github.com/Goldziher/ai-rulez/issues/507)).

## [5.7.0] - 2026-10-25

### Added

- **`[rules] scoped_fallback = "skill"` serves glob-scoped rules as path-gated skills instead of inlining them into
  the shared `AGENTS.md`.** With `agents_md = true` and a preset that has no rules folder (`codex`, `opencode`, `pi`,
  ...), `AGENTS.md` inlined every glob-scoped rule and context file, so every preset that reads it loaded them at the
  start of each session: Cursor, Copilot and Junie directly, and Claude through the `CLAUDE.md` import shim, beside its
  own path-scoped copy in `.claude/rules`. In one measured repository this put 17 language rules into every session and
  pushed `AGENTS.md` past Codex's 32 KiB read limit. With `skill`, `AGENTS.md` keeps the always-on items, and each
  glob-scoped item becomes `.agents/skills/rule-<id>` (or `context-<id>`) with the item's positive globs as `paths`, so
  Claude Code and Cursor gate it by file and Codex loads it on demand. It applies when every folderless reader of
  `AGENTS.md` also reads `.agents/skills`, and not in a scope run. The default, `inline`, leaves output unchanged.
  See [Glob-scoped rules and AGENTS.md](https://goldziher.github.io/ai-rulez/rules/#glob-scoped-rules-and-agentsmd).
- **`generate` warns when the shared `AGENTS.md` inlines glob-scoped items that a preset with a rules folder also
  loads**, naming the presets and pointing to `scoped_fallback = "skill"`.
- **`AR9R2 served-dmi-unreachable`** warns when a `delivery: served` skill sets `disable-model-invocation: true`: a
  served skill is never written to disk (no slash command) and the skills server now hides a user-only skill from
  the model, so nobody can run it. Set `delivery: both`, or `[mcp] serve_user_invoked_skills = true`. `AR9R1` now
  reports a `load_skill`/`find_skill` pointer to a `both` user-only skill too, unless that setting is on. Both
  honor only the hyphenated `disable-model-invocation`, the spelling Claude Code and the server read.

### Changed

- **Builtin skill descriptions fit the 250-character listing limit.** 22 builtin skills (`code-quality-standards`,
  `testing-conventions`, `task-runner`, the binding skills and others) had descriptions of up to 585 characters,
  listed in every session; harnesses that budget the skill listing truncated them and lost the trigger words. Each is
  now at most 250 characters, leads with what the skill covers and ends with when to use it. Skill bodies are unchanged.

### Fixed

- **`AR701`, `AR702` and `AR703` no longer treat a generated copy of a skill as a second definition.** A plugin
  bundle's generated `skills/<name>/SKILL.md` (and a byte-identical public export of it) repeats the description and
  the name of the skill it was generated from, so `validate` reported each copy as a duplicate description of its own
  source and, run from the plugin's configuration, as a collapsed duplicate, and `--strict` failed. `validate` now
  renders the plugin bundles of the configuration in memory, as `verify --plugin` does (a `[marketplace]` member the
  way its producer renders it), and a skill, agent or command file counts as the same item as a definition only when
  its bytes equal what the generator renders from that definition now (CRLF line endings aside). Generated headers and
  `.ai-rulez-generated.json` grant nothing: a hand-written file with a copied header and sidecar, an edited copy, a copy
  left stale by a later edit of its definition and an independently written skill with the same name or description
  are still reported, and every other rule, the security rules first, still reads the generated copies
  ([#485](https://github.com/Goldziher/ai-rulez/issues/485)).
- **`find_skill` and `load_skill` no longer hand a user-only skill to the model.** A skill with
  `disable-model-invocation: true` (Claude Code's flag for a skill only the user runs, with a slash command) was
  ranked by `find_skill`, listed by `search_skills` and the `skill://` resources, and served by `load_skill`. The
  skills server now leaves such a skill out of all of them and `load_skill` answers `<name> is user-invoked only
  (disable-model-invocation)`. The authoring tools of `mcp --serve-skills` (`read_skill`, `list_skills`
  and the `ai-rulez://skills/<name>` resource, including `skill_discovery.served`) and the per-scope skill
  counts follow the same rule, and so does `create_skill` (the error for a taken name no longer shows the file path).
  A skill is held by its own hyphenated `disable-model-invocation` key, whatever the catalog admitted. The
  governance and audit tools (`catalog`, `list_roles`, `resolve_role`, `token_report`, `cost_report`, `sbom`,
  `lock_status`, `doctor`, `validate_config`, `scan_content`) still list such a skill by name with its size and lock
  state, because they audit the whole configuration; they never return its content. The plain `ai-rulez mcp` server
  still reads every skill. `get_skill` answers the same message for a `skill://` URI. `ai-rulez.lock` is unchanged.
  `[mcp] serve_user_invoked_skills = true` serves them as before (a running `--daemon` reads the setting when it
  starts).
- **A path-scoped `find_skill` no longer returns skills whose `paths:` do not reach the path.** Only rules were
  narrowed by their `paths:` or `globs:`; a skill such as `.claude/skills/kafka-conventions` gated on
  `backend/services/**` still came back for `products/web`, ranked by text alone, although Claude Code hides a
  path-gated skill until a matching file is read. The skill index now reads `paths:` from `SKILL.md`, and `find_skill`
  and `find_context`/`load_context` with `path` keep a rule or skill only when its globs match the file, or, for a
  directory, could match a path below it (a directory used to match no glob and hid every path-gated rule). A skill
  without `paths:`, a `scope` query and loading by name are unchanged. The monorepo index cache format moves to
  version 3, so existing caches are rebuilt once.
- **The watcher no longer misses a directory created while it attaches a tree.** Since 5.6.3 the tree was listed
  before any of its directories were watched, so a directory created in between (for example right after a watched
  `.ai-rulez/` was deleted and re-created) was never watched and edits inside it triggered no regeneration or index
  refresh. The tree is now listed again once the watches are in place.
- **`validate` no longer reports a valid `builtins` list as invalid beside another error.** When a configuration
  failed the schema for any reason, such as an unknown key, the rejected boolean branch of `builtins` (boolean or
  list) was reported too, as `builtins.type: Value is array but should be boolean`. Only the real errors are reported.
- **MCP tools accept a call without arguments.** A client that sent `arguments: null` (or none) to a tool of a server
  confined to its project root, such as `list_skills`, made the tool panic. Absent arguments now mean no arguments.
- **`resolve_role` answers for a role that exists.** Its output schema declared `role` a string while the tool
  returns the role object, so every successful call failed the schema check of the MCP SDK.

## [5.6.3] - 2026-10-24

### Added

- **`recursive_exclude` and `generate --recursive --exclude`.** `generate --recursive` (and `validate --recursive`)
  searched every directory, so a repository with deliberately invalid configurations in test fixtures failed the whole
  run. `testdata` directories are no longer searched, and `recursive_exclude = ["tests/fixtures"]` in the
  configuration of the directory the run starts from (`.ai-rulez/config.toml`, `.agents/ai-rulez.toml` or
  `.config/ai-rulez/config.toml`), or `--exclude` (repeatable), names more. The `generate_outputs` MCP tool honours
  both (`recursive` with `exclude`).

- **The live-reload watcher no longer scales its descriptors with the repository.** `mcp --serve-skills` watched every
  directory of the project; on macOS and the BSDs (kqueue) each watched directory and each file in it costs an open
  descriptor, so one server held 50,000 to 110,000 in a large monorepo and a few servers overflowed the system file
  table. The watcher now has a budget of 2,048 descriptors, spends it breadth first (shallow convention directories
  such as `.claude` and `skills` first), and warns once when the budget is reached; the periodic poll covers what is
  left unwatched. Each daemon has its own budget, so many worktrees cost a few thousand descriptors each, not tens of
  thousands. `no_watch = true` still disables the watcher ([#480](https://github.com/Goldziher/ai-rulez/issues/480)).
- **The shared daemon no longer pins the checkout.** `mcp --daemon-server` changes to its private cache directory after
  start, so a throwaway worktree can be removed, and exits when its repository directory is gone instead of waiting out
  the idle timeout. The new `[mcp] daemon_idle_timeout` (a Go duration such as `"2m"`) sets that timeout, 15 minutes by
  default. The daemon's stderr (a watch budget reached, why it exited) now goes to `daemon.log` beside its socket
  instead of the null device ([#481](https://github.com/Goldziher/ai-rulez/issues/481)).
- **A read-only skills server registers only the skill tools by default.** `mcp --serve-skills` with `read_only`
  registered 43 tools, about 92 KB of schema sent on every request. It now registers `find_skill`, `load_skill`,
  `search_skills`, `read_skill_file` and `list_skill_resources` (a few KB) and no prompts; `[mcp] authoring = true`
  restores the full read-only set and the `review-config` prompt, and `tools = [...]` still names the list explicitly
  ([#482](https://github.com/Goldziher/ai-rulez/issues/482)).
- **A long home directory no longer silently disables the daemon.** When the socket path under the cache is too long
  (104 bytes on Linux, 100 on macOS and the BSDs, with a margin), the socket moves to
  `$XDG_RUNTIME_DIR` or the temp directory under `ai-rulez-<uid>/`. The daemon, its log and the proxy only use it when
  this user owns both that directory and the one inside it with mode 0700, and the log is never opened through a link,
  so another user on a shared `/tmp` cannot redirect them. If no path fits, the session logs a warning (it was debug
  level) and serves in-process ([#483](https://github.com/Goldziher/ai-rulez/issues/483)).

## [5.6.2] - 2026-10-21

### Added

- **`[[lint.accept]]`: accept one finding with a required reason.** An entry names a `rule`, a `path` glob and a
  `match` substring of the finding's message (or a `line`, or both), plus a `reason` that config validation requires.
  The matching findings stay in the report, marked accepted with the reason, and do not fail the run; a second finding
  of the same rule in the file stays reported. An entry that accepted nothing is reported as the new `AR9I0
  accept-unused` warning, except when the run could not have produced the finding (rule off, analyzer not selected,
  or the file not linted by `--path`/`--glob`), so stale acceptances do not accumulate. A code the organization policy
  protects is never accepted (`AR740`). `validate --update-baseline` does not write accepted findings to the baseline.
  (#479)
- **`validate --tracked-only` and `[lint] tracked_only = true`.** `validate` and `scan` lint every file that is not
  git-ignored, tracked or not, so an untracked build artifact such as a `__pycache__/*.pyc` (AR051) made results differ
  between machines. With the option the run reports no findings in files outside the git index, except the files
  `--changed` or `--since` names; the files are still walked and their findings are dropped after the run. Outside a
  git work tree it logs a warning and keeps every finding. (#476)

### Fixed

- **The scanner flags no longer fail a run without `--external`.** `validate` and `scan` rejected `--no-scan-cache`,
  `--write-baseline`, `--reason`, `--show-suppressed` and `--scanner-baseline` with "require --external", so a CI
  command had to branch on whether a scanner was configured. Without `--external` these flags now do nothing and one
  warning on stderr names the ones that were ignored; `--reason`, `--reason-for`, `--min-severity` and `--rule` still
  need `--write-baseline`, and `--write-baseline` still needs `--reason`. (#477)
- **`validate` reports a malformed skill frontmatter once.** A skill whose frontmatter is not valid YAML (for example
  `description: Args: a, b`) printed a `WARN Ignoring malformed YAML frontmatter` line on stderr in addition to the
  `AR306` finding, which broke machine-readable output. The lint pass now leaves the warning out, since `AR306` carries
  it; commands that do not validate, such as `list`, still warn. (#478)
- **`validate --update-baseline` no longer records findings in untracked files.** A teammate's or CI's checkout does
  not have those files, so their baseline entries were dead weight and `lint-baseline.json` differed between machines.
  They are now skipped, and the run logs how many. (#476)
- **A scanner result without `locations` is no longer reported on `.ai-rulez/config.toml:1`.** The finding looked as
  if it were about the configuration, and a baseline entry on that file stopped matching once the scanner added a
  location. A run that was handed exactly one skill (`batch_size = 1`) now attributes such a result to that skill's
  `SKILL.md`; any other run reports it without a file, with `(no location reported by scanner)` at the end of the
  message. Cached scanner results from earlier versions are scanned again. (#474)
- **A finding without a file no longer takes the working directory as its path.** Such a scanner result got the path
  `.` (or the name of the directory `validate` ran in), so `--tracked-only` dropped it, its fingerprint depended on where
  the command ran, and SARIF, GitHub and JUnit output named `.` as its location. It now has an empty path, a fingerprint
  that is the same from every directory, and no location in the output formats.
- **`[[lint.accept]]` and the baseline no longer interfere.** `--update-baseline` copied config-accepted findings into
  `lint-baseline.json` (and refused security ones without `--baseline-reason`), and applying a baseline replaced their
  reason and counted them twice. The baseline now leaves them alone.
- **`validate --external` shows progress during a long scanner run.** A scanner with `batch_size = 1` over hundreds of
  skills ran for minutes without a line of output, so CI and agent shells could not tell it from a hang. Each scanner
  now prints `name: N batches to run` on stderr when it starts (every scanner, `1 batch to run` for a single run) and,
  only when it runs two or more batches, `name: batch 4/13` as each finishes, also with `parallel`. The lines are information level: `--quiet` drops them and the report on stdout is unchanged. (#475)

## [5.6.1] - 2026-10-20

### Changed

- **Built with liter-llm 2.3.0.** The release binaries link the liter-llm 2.3.0 native library. The other Go
  modules (go-python-packaging 0.13.0, oras-go 2.6.4 and their dependencies), the docs site dependencies and the pnpm
  version the npm package is published with are updated as well.

### Fixed

- **The AR041 to AR058 checks no longer slow every scan by about half.** In 5.6.0 each rule looked for its trigger
  words one at a time, and the malware rules read the whole file once per word, so `validate`, `scan` and the MCP
  skill scan took about 45% longer than in 5.5.3. All of a rule set's words are now found in one pass, which brings
  the cost of the new rules to about 12%; findings are unchanged.
- **`validate --path` above the configuration's directory outside git.** In a tree git does not track, the project
  root was the configuration's directory, so `--config sub/.ai-rulez/config.toml --path docs/AGENTS.md` failed with
  "outside the project root" although you named the file. A `--path` above it now widens the root to the nearest
  directory that holds the configuration and every named path, when that directory lies inside the working directory.
  Git work trees, `--repo-root`, `--glob` and `[lint] paths` are unchanged. (#470)

## [5.6.0] - 2026-10-19

### Added

- **`message_properties` appends SARIF result properties to a scanner finding's message.** A `[[lint.external]]` entry lists property paths (`evidence`, `snippet`, a dotted `a.b`, or `location.snippet` for the standard region snippet) whose sanitized values are appended as `[evidence: ...; snippet: ...]` within `max_message`; the `skillspector` profile sets `evidence` and `snippet`, so a CI annotation shows the matched text instead of only the rule title ([#468](https://github.com/Goldziher/ai-rulez/issues/468)).
- **Built-in name tables are generated from each harness's registry and checked every week.** `tools/harness-builtins/generate.py` reads the commands, bundled skills and agents of 46 of the 52 harnesses from a public registry (a Rust, TypeScript, Go or Python source file, a file tree, or a structured docs page) at the tag or commit pinned in `pins.toml` and rewrites the names and aliases of the matching groups in `internal/harnessnames/data/<preset>.toml`; source ids, quotes, `user_collision`, `evidence`, `invocation_spaces` and `completeness` stay as written. `--check` exits 1 when a data file differs from its pinned source, `--bump` shows the names a newer tag would change (`--bump --apply` moves the pin), and the six harnesses without a registry (`aiassistant`, `amp`, `baz`, `bob`, `muse`, `trae`) are `extractor = "none"` with the reason. `check_sources.py` now verifies every quote of every table, fails on a recorded outcome that has no quote, and prints the verified and failed counts per harness. `task harness:builtins:check` and `task harness:builtins:refresh` run them, and `.github/workflows/harness-builtins.yml` runs the check every Monday and opens (or updates) one issue on drift. The data files shrank where one registry feeds many names: hermes, takt, codewhale and openclaw went from about 160 single-name groups to a handful. Running the extractors corrected the data: names that are not commands were removed (`open` in xum, `export` and `remember` in Letta, `share`, `unshare`, `details` and the `scout` agent in OpenCode, `feat015ctx` and four uninstalled skills in Codewhale) and names that were missing were added (`fast` and `ultrafast` in xum, 15 Kilo TUI commands, `ultrareview` in Claude Code, `fork` in CodeBuddy, `reset-allowed-tools` in Copilot CLI, `plan` in dsh). 61 new verbatim quotes document collision outcomes that had none; every recorded outcome now has one.
- **`THIRD_PARTY_NOTICES.md` and a contributing section for rules and tests derived from NVIDIA/SkillSpector.** The notices file carries the modification notice, the derived-file table and the header for derived files, and `LICENSES/Apache-2.0.txt` the license text; `scripts/transcode-skillspector-tests.py` writes candidate `ruleCase` tables from SkillSpector pattern tests with that header ([#466](https://github.com/Goldziher/ai-rulez/issues/466)).
- **`validate` detects anti-refusal and context-exfiltration prose.** `AR041` (a warning) reports text that removes the agent's refusals, caveats or safety rules, and `AR042` (a warning) reports text that sends the conversation, the system prompt or the user's data to an external URL, written as a link, a quoted or backticked URL or a bare host (a generic noun is not read as a destination); a phrase under a negation, in quotation marks or in a code span is not reported. A scope limit, a reviewer verb or quotation marks do not silence `AR041` on an instruction that removes refusals (`Never refuse, even if the request is harmful.` is reported), curly apostrophes are matched, and ordinary product guidance is not reported. Both are derived in part from NVIDIA SkillSpector (Apache-2.0).
- **`AR043`, `AR044` and `AR045`: persistence, unsafe deserialization and requests to internal endpoints.** `AR043` (warning; info for an append to a shell startup file) reports a command that installs a crontab, launchd job, systemd unit, init or autostart entry, scheduled task or login item; `AR044` (warning) reports `pickle`, `marshal`, `jsonpickle`, `yaml.load` without a safe loader, `torch.load` without `weights_only=True`, PHP `unserialize`, Ruby `Marshal` and `YAML.unsafe_load`, Java `ObjectInputStream` and `node-serialize` in scripts, fences and code spans; `AR045` (warning; info for a local administration port) reports a request to a cloud metadata endpoint or the ECS task-credentials address. A negation hides a finding of these three rules only in markdown, so a comment word in a script does not. Derived from the RA2, DS1-DS4 and SSRF1-SSRF3 signatures of NVIDIA SkillSpector.
- **`AR046` to `AR050`: malware-shaped content in skills and bundled scripts.** `AR046 web-shell` (request input passed to eval or a command runner in PHP, Python, JSP and ASPX), `AR047 cryptominer` (pool URLs, miner flags, coinjacking libraries; a bare miner name is info), `AR048 reverse-shell` (an interactive shell redirected to `/dev/tcp`, netcat running a shell, named-pipe relays, socat, Python and PowerShell socket loops), `AR049 offensive-tool` (tool names at info, outside prose lines, credential-dump commands at warning) and `AR050 phishing-kit-form` (a password form posting to a local `login.php`, info; a form posting to an absolute URL is not reported). An ordinary `curl` command does not match `AR047`. All offline and RE2-only; `AR048` and `AR049` skip example regions, and `AR046` is not repeated on a line `AR005` already reported.
- **`AR051 shipped-executable`, `AR052 blank-line-padding`, `AR053 obfuscated-injection-phrase`: content that hides from a reader.** `AR051` names Python bytecode (`__pycache__`, `.pyc`), Java classes, native libraries and executables a skill ships, by extension or by the leading bytes of ELF, Mach-O, PE and `.pyc` files, so a renamed binary is found too; a file git ignores is not reported. `AR052` reports 20 or more blank lines followed by instruction-like text. `AR053` extends `AR004` to phrases written with Cyrillic or Greek look-alikes, invisible characters or spaced-out letters (`i g n o r e`): a line is normalized once (NFKC, a table of look-alike letters, spaced letters joined) and only a line that holds an `AR004` phrase after that, and not before, is reported. A plain ASCII line without a spaced run costs one byte scan. All three are warnings. The rules are modelled on NVIDIA SkillSpector (Apache-2.0) checks SC8, SC9, P3 and AE4/AE6; aligned table padding is deliberately not counted.
- **Dependency-manifest and MCP-metadata rules: `AR054` to `AR058`.** `AR054` reports a bundled `requirements*.txt`, `package.json` or `pyproject.toml` dependency without a pinned version, `AR055` a package index or registry redirect (`--index-url`, `--extra-index-url`, `--trusted-host`, `registry=`) in a manifest, config or install command, and `AR056` (info) a dependency name one edit from a popular PyPI or npm package, taken from lists of the most downloaded package names. Manifests at the skill root are scanned too, and a manifest with a BOM, in UTF-16, oversized or not valid UTF-8 is reported as not analyzable. `AR057` applies the hidden-character, HTML-comment and injection-phrase detections of `AR002` to `AR004` to the description and tool metadata of MCP server definitions, with or without a configuration. `AR058` (info) reports commands a skill runs that its `allowed-tools` do not cover. `AR051` reports once per file and `AR058` once per command, so a baseline entry for one file does not accept a second and `--changed` sees a new one. The manifest rules follow the supply-chain checks of NVIDIA SkillSpector (Apache-2.0).
- **Release archives, the Homebrew formula and the npm and PyPI packages ship `THIRD_PARTY_NOTICES.md` and the license texts in `LICENSES/`**, and the transcoder refuses a `--tests` directory that is not a git checkout root or a commit that is not 40 hex digits ([#466](https://github.com/Goldziher/ai-rulez/issues/466)).

### Changed

- **New findings can fail an existing run.** `AR014` on `env | curl ...` in an inline code span is an error (exit 2 by default), and the new warning rules fail `--strict`; baseline or `[lint.severity]`-override them where intended. A line such as `>> ~/.zshrc` is reported by `AR006` and, as info, by `AR043`.
- **`AR006` leaves a write to a scheduler, init or autostart location to `AR043`.** `cat > ~/Library/LaunchAgents/x.plist` is reported once, as persistence; an append to a shell startup file is still reported by `AR006` as before.

### Fixed

- **The `skillspector` profile drops SkillSpector's "OSV.dev unreachable" `SC4` notice when egress is denied.** SkillSpector has no offline switch, so with `egress = false` it reported one low `SC4` result per `requirements*.txt` that the lookup failed. Profiles can now list `offline_notices` (a rule id and a message fragment, both must match); an `egress = false` entry drops those results, and real `SC4` vulnerability findings are kept ([#469](https://github.com/Goldziher/ai-rulez/issues/469)).
- **`--write-baseline` drops scanner baseline entries that match no current finding.** A rewrite kept every existing entry, so stale fingerprints piled up next to the new ones (93 stale plus 67 new gave 160 entries) until `--strict-baseline` forced a manual cleanup. The rewrite now reflects the current findings; entries of a scanner that did not run or failed, and of findings that `--min-severity` or `--rule` leave out, are still kept ([#467](https://github.com/Goldziher/ai-rulez/issues/467)).
- **`AR014` reports `env | curl ...` and a credential-file upload written inside an inline code span** of a prose line, as it already did on a bare line and in a fenced block ([#462](https://github.com/Goldziher/ai-rulez/issues/462)).
- **A `[[skill_sources]]` directory written with backslashes is a skill root for `AR808`.** `url = '.\vendor'` (a Windows spelling) never matched, so a reserved name in it went unreported; a drive path such as `C:\vendor` is no longer read as a project directory.
- **Kimi Code's `/tower` command is a built-in name.** The weekly registry check found it in the slash-command reference, so a custom `tower` command or skill is now reported by `AR9U0` for `kimi` ([#472](https://github.com/Goldziher/ai-rulez/issues/472)).

## [5.5.3] - 2026-10-18

### Added

- **`[[lint.ignore_rules]]`: drop chosen rules in chosen files** (`rules = ["AR901", "AR9*"]`, `paths = ["playground/**"]`, optional `reason`). Codes, names and wildcard patterns over codes are accepted and validated when the configuration loads; the route is refused for policy-protected codes like `ignore_paths`, and ignored findings never reach the baseline or the ratchet. The table is `ignore_rules` because `[lint] ignore` is already the list of codes dropped everywhere ([#460](https://github.com/Goldziher/ai-rulez/issues/460)).

- **`[lint] skill_roots` and `validate --exclude <glob>`.** `skill_roots` lists the directories AR808 treats as harness skill roots, replacing the defaults; `--exclude` leaves matching files out of the files `validate` and `scan` find by convention, `--path` and `--glob`, with or without a configuration. `[lint] ignore_paths` is now described in `validate --help` and in `validate --explain AR808` ([#457](https://github.com/Goldziher/ai-rulez/issues/457)).

- **`AR9U0 harness-builtin-name-collision` and `AR9U1 harness-reserved-name`: a skill, command or agent named like a built-in of any harness.** `internal/harnessnames` holds one table per preset (52 files, about 2,800 built-in commands, bundled skills and agents, each group with the page it was taken from and, where the harness documents the outcome, a verbatim quote) and what the harness does with a user item of the same name: it replaces the built-in (Claude Code, OpenCode, Qwen Code, Kilo), the built-in stays and the item is renamed or unreachable (Gemini CLI, Cline, Goose, Augment), or both are listed (Codex). `validate` compares every output with each harness that loads it: the presets that write it and every harness that reads the directory it lands in, such as `.agents/skills`, whether or not its preset is enabled. Names are compared NFKC-normalized and case-folded, aliases included. A built-in with an undocumented outcome is information, listed once per item; an error needs a quoted outcome in a table set to `error` and a preset the project writes for (a harness that only reads a shared directory is a warning). Plugin-only content is exempt. Acknowledge a collision with `[lint.builtin_names] allow = ["gemini:help"]` (or `lint.allow_overrides`), declare names the tables cannot know with `[lint.builtin_names.extra]`, add a table in `.ai-rulez/harness-names/<preset>.toml`, and turn the rules off with `enabled = false`; AR9U1 (Claude Code `synced`, `anthropic-skills`, `anthropic-skills:*`) cannot be allowed. The tables are in the AR9C0 age check and `task harness:verify` re-fetches their quotes. A project with a skill named `help`, `review`, `plan` or `init` may need an `allow` entry. `tools/harness-builtins` converts the compiled data set and checks sources The `AR9U` block was reserved for a UI; that claim moves to `AR9V` ([#447](https://github.com/Goldziher/ai-rulez/issues/447)).

- **`--min-severity`, `--rule` and `--reason-for` for `--write-baseline`.** `validate --external --write-baseline` and `scan --external --write-baseline` recorded every finding with one `--reason`. `--min-severity <level>` and repeated `--rule <glob>` (scanner rule id, `scanner:rule` or `AR` code) now narrow what is written; findings left out stay reported and keep an existing entry. `--reason-for RULE=text` sets the reason per rule, and `{rule}`, `{severity}` and `{scanner}` expand in `--reason` and `--reason-for`. A new entry stores only the first line of the message (160 characters at most), which keeps a large baseline small; the format is unchanged, so existing baselines and `--strict-baseline` work as before ([#458](https://github.com/Goldziher/ai-rulez/issues/458)).

### Changed

- **AR808 reads its Claude Code names from the new table and reserves aliases.** A skill replaces a built-in command but not its aliases, so a skill named `new` (an alias of `/clear`), `settings`, `share`, `undo` or `rc` is reported as a warning (the skill loads; only the alias keeps running the built-in), not an error; the bundled skills synced from a claude.ai account (`pdf`, `xlsx`) are not reported. AR9U0 skips the Claude Code commands and bundled skills, so no name is reported twice. Authored content that the plan does not write where Claude Code reads it (a plugin-only skill, invoked as `/plugin:skill`) is no longer reported.

### Fixed

- **`validate --external --changed` lists less.** (Not a fix for the scanner's own run time, which is its process start and analysis, [#461](https://github.com/Goldziher/ai-rulez/issues/461).) A scanner handed `{skill_dirs}` already ran only the skills with a changed file, but the run first listed the supporting files of every skill (a directory walk, a `git` call and a read per skill) and then discarded them. The files of a skill that holds no changed path are now left out of that listing, and the changed skills are listed as before, so the findings are the same. In a 400-skill, 10,000-file repository with two edited skills, `validate --strict --external --changed` took 14.4 to 16.7 s and now takes 1.5 to 2.3 s (1.4 s without `--external`) ([#461](https://github.com/Goldziher/ai-rulez/issues/461)).

- **AR401 no longer reports a documented path that git ignores.** A missing path matched by `.gitignore`, `.git/info/exclude` or the global excludes file (`.claude/settings.local.json`, `.claude/personal-preferences.md`) is a local file, not stale guidance. `[lint] allow_paths`, `[[lint.ignore_rules]]` and an inline `ai-rulez-lint-ignore` comment cover placeholder paths, and are now listed in `validate --explain AR401`. git is asked once per run for all missing paths ([#459](https://github.com/Goldziher/ai-rulez/issues/459)).

- **The scanner cache log counts only cached skills.** `validate --external --no-scan-cache --changed` logged `Scanner results served from the cache` for the skills `--changed` left out although the scanner ran for the others ([#461](https://github.com/Goldziher/ai-rulez/issues/461)).

- **AR808 only checks skills and commands where the harness loads them from.** A hand-written `SKILL.md` is compared with the Claude Code built-ins only inside `.claude/skills` (or `.claude/commands`), a plugin's `skills/` directory, a local `[[skill_sources]]` directory or a `skill_roots` glob; an application's own `x/src/public/skills/code-review/SKILL.md` is still linted for format but no longer reports `shadows the built-in /code-review`. Authored `.ai-rulez/` content is always checked ([#457](https://github.com/Goldziher/ai-rulez/issues/457)).
- **AR402 no longer reads the code span in a link's text as a resource reference.** `[`b` `references/s.md`](../../b/references/s.md)` was reported as a missing `references/s.md` although the link target exists. A code span inside the text of an inline link is its label; the link target is checked instead (an unresolved target is reported as a broken link, not as AR402) ([#456](https://github.com/Goldziher/ai-rulez/issues/456)).

## [5.5.2] - 2026-10-18

### Added

- **`[mcp] serve_skills` makes a bare `ai-rulez mcp` serve skills.** The key (default `false`) behaves as `--serve-skills`, so a client entry can be `ai-rulez mcp` alone; the flag wins, and `--serve-skills=false` turns the key off. It is in the schema and accepted by `validate` ([#454](https://github.com/Goldziher/ai-rulez/issues/454)).
- **`skillspector` scanner profile for NVIDIA SkillSpector.** `profile = "skillspector"` runs `skillspector scan --no-llm --format sarif` one skill per run, checks the `>=2, <3` version range, and keeps the scanner offline: `--no-llm` is required and `--transitive`, `--mcp-registry` and `--mcp-registry-compare` are deny-listed. Profiles can now set `batch_size`, `version` and `required_flags`, which entries inherit and override. Opt-in, not part of `baseline` or `strict`; contract-tested against SkillSpector 2.12.0, with a recipe in `docs/strict-validation.md` ([#448](https://github.com/Goldziher/ai-rulez/issues/448)).

### Fixed

- **The `lint.allow_overrides` scope matching has one implementation.** `generate` and `validate` (`AR703`) shared the matching only by copy; both now call `presets.CollapseAllowed`, with no change in behaviour.
- **`scanners doctor --external` reports a timed-out version probe.** For a scanner without a `version` range the timeout was discarded and the row read `unknown (the version command printed nothing usable)`; it now says `version probe timed out after 10s`.
- **An explicit `AR9E6` severity is no longer overridden when every scanner result is dropped.** The escalation to an error applies only while the rule has its default severity; `[lint.severity] AR9E6 = "info"` (or `"warning"`) is kept, and `off` still disables it ([#448](https://github.com/Goldziher/ai-rulez/issues/448)).
- **Cached scanner results written by earlier builds are not served.** The path normalisation and the out-of-scope batch fields changed how a report maps to findings, so the scan cache mapping version is bumped and older entries miss once ([#448](https://github.com/Goldziher/ai-rulez/issues/448)).
- **A server started without `--serve-skills` warns about skills-only `[mcp]` keys.** `discover_skills`, `allow`, `deny`, `exclude`, `no_watch` and `daemon` were accepted and silently ignored, leaving a client with the authoring tools and no `find_skill`. `mcp` now logs one warning at startup naming the keys that are set and ignored, and says to pass `--serve-skills` or set `[mcp] serve_skills` ([#455](https://github.com/Goldziher/ai-rulez/issues/455)).
- **`{project}` names the project root in a scanner command, and the stage working directory is documented.** A relative `command[0]` was resolved against the project but a relative script argument (`["sh", "tools/t.sh"]`) against the stage, so it failed with exit status 127. `{project}/tools/t.sh` now works, the script it names is still hashed into the cache key, and the difference is documented next to `command` ([#450](https://github.com/Goldziher/ai-rulez/issues/450)).
- **A skill-relative scanner result resolves against the skill of its own batch.** With `batch_size = 1` and `{skill_dirs}`, `SKILL.md` and `references/x.md` were matched against every staged skill, found ambiguous and dropped as `AR9E6`. Each batch now resolves against the skills it was handed; a path that is still ambiguous in a multi-skill batch is dropped as before, and `AR9E6` is an error when every result of a run was dropped ([#448](https://github.com/Goldziher/ai-rulez/issues/448)).
- **The scanner version probe has a `version_timeout` and a timeout is reported as one.** The probe for `version` was fixed at 10 s and a slow one read as `version output ""` (`AR9E2`), so the scanner silently did not run. `version_timeout = "30s"` on `[[lint.external]]` raises it (default unchanged), and a timed-out probe says `version probe timed out after 10s` ([#449](https://github.com/Goldziher/ai-rulez/issues/449)).
- **`generate` honours `lint.allow_overrides` in the duplicate warning.** A local skill that shadows a builtin and is listed in `lint.allow_overrides` (as `name` or `domain/name`) no longer logs `Duplicate skill collapsed` on every run, matching `validate` and `AR703` ([#444](https://github.com/Goldziher/ai-rulez/issues/444)).
- **A local skill may shadow a builtin command, and a local command a builtin skill.** The two kinds share one output id, so a project item with the id of a builtin of the other kind failed `validate` and `generate` with `skill and command ids collide`. The project item now wins, the builtin is dropped, and the collapse is reported as `AR703` and in the `generate` duplicate warning, silenced by `lint.allow_overrides`. Two project items, or two builtins, of different kinds with one id stay an error ([#445](https://github.com/Goldziher/ai-rulez/issues/445)).
- **A local domain named like a builtin pack warns instead of silently skipping the pack.** The pack's items were dropped with only a debug line. `generate` and `validate` now log `Builtin pack skipped` once per pack, naming the items that are lost (those a same-named project item does not shadow) ([#446](https://github.com/Goldziher/ai-rulez/issues/446)).

## [5.5.1] - 2026-10-17

### Added

- **`[mcp]` takes the `mcp` server settings, and `--tools` limits the tools.** `read_only`, `daemon`, `no_watch`, `allow` and `deny` under `[mcp]` (also in root `ai-rulez.toml` and the layered and local configs) set what the flags of the same name set, so `.mcp.json` can be `ai-rulez mcp` alone; a flag given on the command line wins. `tools = [...]` and the new `--tools` flag register only the named tools, on `mcp` and `mcp --serve-skills`; an unknown name stops the server and lists the valid ones. The organization policy gains `[mcp] read_only = true`, which neither a key nor a flag can loosen (`read_only = false` in the repository is reported as `AR740`). See [Settings in the configuration](https://goldziher.github.io/ai-rulez/mcp-server/#settings-in-the-configuration) ([#440](https://github.com/Goldziher/ai-rulez/issues/440)).
- **`mcp --exclude <glob>` and `[mcp] exclude` keep paths out of repository discovery.** Globs relative to the project root (`**` crosses directories, a pattern without a slash matches at any depth, a trailing slash names a directory) leave skill directories and the context index alike unread, before any file is classified, and the watcher ignores them. The flag adds to the configured list. See [Excluding paths](https://goldziher.github.io/ai-rulez/mcp-server/#excluding-paths) ([#438](https://github.com/Goldziher/ai-rulez/issues/438)).
- **`AR808 reserved-name`: a skill or command named like a Claude Code built-in is an error.** A project skill replaces the built-in of the same name, so `.claude/skills/init` silently disabled `/init`. `validate` now compares skill and command names and their `name:` frontmatter, for hand-written files and without a config too, against the built-in commands and bundled skills of the Claude Code commands reference (aliases are not reserved), case-insensitively with `_` and spaces read as `-`. An intentional override is listed in `lint.allow_overrides` as `init` or `claude:init`. Projects with a skill named `review`, `plan`, `run`, `debug` and the like now fail `validate` until they rename it or acknowledge it. See [Strict validation](https://goldziher.github.io/ai-rulez/strict-validation/) ([#442](https://github.com/Goldziher/ai-rulez/issues/442)).

### Fixed

- **`mcp --read-only` no longer advertises authoring.** The server instructions are built from the registered tools, so a read-only server names only the tools it has and no longer points to the CRUD tools, `generate_outputs` or `--read-only`; an authoring prompt is listed only when every tool it calls is registered ([#434](https://github.com/Goldziher/ai-rulez/issues/434)).
- **`mcp --serve-skills --discover-skills` works without a configuration.** In a repository with no `.ai-rulez/` or `ai-rulez.toml`, `ai-rulez://catalog` is built from the discovered skills and context (with `scopes` and `skill_discovery`), `ai-rulez://config` and the item templates are not listed, and only the tools that work without a configuration are registered ([#435](https://github.com/Goldziher/ai-rulez/issues/435)).
- **A renamed discovered skill is reported.** When two repository skills share a name and one is served under its path, the server logs a start-up warning with both paths and the served name, and the catalog lists it under `skill_discovery.renamed` ([#436](https://github.com/Goldziher/ai-rulez/issues/436)).
- **`presets = []` no longer empties the context index.** Rules, agents and commands are indexed from the conventions present in the repository whatever the presets; the presets still choose the skill directories ([#437](https://github.com/Goldziher/ai-rulez/issues/437)).
- **Scan findings in repository context name the file.** An instruction file, rule or agent is reported as `context://./CLAUDE.md:2` instead of `skill://CLAUDE.md/CLAUDE.md:2`, and the message says the file is withheld, which an error finding does at every trust level ([#439](https://github.com/Goldziher/ai-rulez/issues/439)).
- **`find_context` with `path` honours rule globs.** A Claude Code rule's `paths:` and a Cursor rule's `globs:` (relative to the directory holding the harness folder) now narrow the rules returned for a file; rules without them apply to their whole scope, and `scope` queries are unchanged. The index cache format moved to version 2 and is rebuilt once ([#441](https://github.com/Goldziher/ai-rulez/issues/441)).
- **A repository skill wins over a builtin of the same name.** `mcp --discover-skills` served the builtin and dropped `.claude/skills/<name>`; the repository skill is now served and a start-up line names the builtin it replaces. `validate` reports a hand-written skill that collides with a configured skill as `AR703`, as it already did for `.ai-rulez/skills`, silenced by `lint.allow_overrides` ([#443](https://github.com/Goldziher/ai-rulez/issues/443)).
- The catalog reports no served or shadowed repository skills as `[]`, not `null`.
- On Windows, editing a file in an ordinary directory no longer rebuilds the repository index: a directory event now counts only when the directory can hold an instruction file, skill, rule or agent.
- **Pi prompts keep working links to the skills.** A command's relative links are written against `commands/`, so `../skills/x/SKILL.md` reaches the root `skills/` tree, but the Pi prompt generated from it sits two levels down in `.pi/prompts/` and its skills may live in a `[plugin.pi] skills_dir` of their own. The prompt copied the link unchanged, so with `skills_dir = "skills"` it resolved to `.pi/skills/x/SKILL.md`, which does not exist. Relative links that climb out of the command are now resolved from `commands/` and rewritten from `.pi/prompts/` to where the target is (`../../skills/x/SKILL.md`); with the default `.pi/skills` they are unchanged ([#452](https://github.com/Goldziher/ai-rulez/issues/452)).

## [5.5.0] - 2026-10-17

### Added

- **`mcp --serve-skills --discover-skills[=repo|all]` and `[mcp] discover_skills` serve the skills already in the repository.** `repo` indexes `skills/`, `.claude/skills` at any depth and the configured presets' skill directories through the same `skill://` resources, `find_skill` and `load_skill`, honouring `.gitignore`, skipping generated copies, and under the same scan, budget and `--allow`/`--deny` rules; `all` adds installed and included skills whatever their delivery. Name collisions follow the skill precedence (ai-rulez-defined first); shadowed skills are a start-up warning and appear in the `ai-rulez://catalog` resource. See [Discovering skills already in the repository](https://goldziher.github.io/ai-rulez/mcp-server/#discovering-skills-already-in-the-repository) ([#432](https://github.com/Goldziher/ai-rulez/issues/432)).
- **`installed_skills` `source` accepts `~`, `owner/repo`, `github:owner/repo` and `file://`.** `~` paths follow the same trust rule as any local path; shorthand expands to an https URL, leaving lock keys of existing entries unchanged.
- **Installed skills are discovered recursively and in harness directories.** Skills are found to six levels deep, and an entry with no `path` and nothing at `skills/<name>` searches `skills/`, `.claude/skills/`, `.agents/skills/` and the other harness skill directories.
- **`ai-rulez skill install` and the MCP `install_skill` tool take every `installed_skills` field**: `--include`, `--exclude`, `--name-prefix`, `--trust`, `--resources`, `--version`, `--tag-prefix`, `--include-prerelease`, `--min-release-age`, `--domain`, `--profile` and `--local-override` (the tool arguments have the field names). See [MCP server](https://goldziher.github.io/ai-rulez/mcp-server/).
- **Import plugins and marketplaces from other harnesses.** `[[installed_marketplaces]]` and `[[installed_plugins]]` read Claude Code, Codex and Cursor marketplace indexes (a repository, GitHub `owner/repo`, a local directory or an https `marketplace.json`) and Claude Code, Codex, Cursor, Gemini extension and Agent Plugins plugin layouts, and turn their skills, agents, commands, rules, hooks and MCP servers into ai-rulez content for every preset. Entries are pinned in `ai-rulez.lock` (new `marketplace` and `plugin` kinds, honored by `--locked`, `--frozen` and `--offline`, moved by `ai-rulez update --kind plugin|marketplace`), scanned before they are pinned and at `generate`, and covered by approvals, deny digests and the organization policy. Hooks are imported only with `enable_hooks = true`; a hook or MCP server that uses `${CLAUDE_PLUGIN_ROOT}` is kept only for a plugin inside the project. New commands: `ai-rulez marketplace add|remove|list|update` and `ai-rulez plugin install|remove|list|update`. See [Installed Plugins](https://goldziher.github.io/ai-rulez/installed-plugins/).
- **`resources` on an `installed_skills` entry sets what is bundled.** It replaces the default with name, path or glob patterns, a leading `!` removing what an earlier pattern added, for example `["*", "!docs"]` or `["rules"]` ([#419](https://github.com/Goldziher/ai-rulez/issues/419)).
- `ai-rulez generate --refresh-remote` (and `[lock] refresh_on_generate = true`) re-resolves remote includes, installed skills and skill sources to their newest allowed version and rewrites `ai-rulez.lock` before outputs are written. The pre-pin security scan runs first; a refused tree aborts the run with nothing written, and `generate --accept-findings` pins it anyway after review, as `lock --accept-findings`. Without the flag nothing changes.
- `skip_outputs` (also accepted in `config.local.toml`) lists output kinds ai-rulez never writes, overwrites or deletes: `instructions`, `agents_md`, `claude_md`, `rules`, `skills`, `agents` and `commands`. A project can hand-write `CLAUDE.md`, `AGENTS.md` or skills and let ai-rulez emit only installed skills, plugins, MCP servers and hooks. A file an earlier run generated and that is now skipped stays in place with a one-time warning. See [Managing only remote skills and plugins](https://goldziher.github.io/ai-rulez/managing-remote-skills/).
- A configuration without a `.ai-rulez/` folder: `ai-rulez.toml` at the project root (overlay `ai-rulez.local.toml`) declares installed skills, skill sources, includes, MCP servers, hooks, permissions and lint settings, with `ai-rulez.lock` beside it. `generate` writes no instruction files and leaves hand-written `CLAUDE.md` and `AGENTS.md` alone; the manifests are `.ai-rulez-manifest.json` and `.ai-rulez-manifest.local.json` in the root. A config directory still wins over the file. `ai-rulez init --minimal` creates it. See [Config without a .ai-rulez folder](https://goldziher.github.io/ai-rulez/config-without-folder/).
- **`[session_start]` and `ai-rulez session-start` set a remote session up with no hand-written hook.** `generate` adds a `SessionStart` hook to `.claude/settings.json` that runs `ai-rulez session-start`. In a remote (cloud) session (`CLAUDE_CODE_REMOTE=true`) it regenerates the outputs of the fresh clone (`--locked` when `ai-rulez.lock` exists, offline first) and returns `additionalContext` that says the generated files are managed by ai-rulez and, when enabled, how to use its MCP server and served skills (`find_skill`, then `load_skill`). Locally it does nothing unless `when = "always"`. The hook tries an installed `ai-rulez`, then `npx -y ai-rulez@latest`, then `uvx --from ai-rulez@latest ai-rulez` (the latest published release, run from an empty private directory so a project `.npmrc`, `node_modules/.bin` or uv configuration cannot substitute the package; `version` pins a release instead), and never fails a session: the command always exits 0, is bounded by `timeout` (default 120 s, process tree killed), runs generate with a scrubbed environment under a lock file, and reports a problem as one stderr line. `ai-rulez doctor` lists the launcher order and warns when the committed settings lack the hook, git ignores them, or the npm registry is denied or unreachable in a remote session. Keys: `generate`, `when`, `mode`, `timeout`, `version`, `command`, `prompt`, `prompt_text`; see [Run generate when a remote session starts](https://goldziher.github.io/ai-rulez/session-start/).
- **A slow scanner can be batched, run in parallel and given more than 15 minutes.** `timeout` was capped at 15 minutes for one run that received every skill. `batch_size` on `[[lint.external]]` splits `{skill_dirs}` into runs of at most that many skills, each with its own `timeout` and cached on its own; `parallel` (1 to 16, default 1) runs batches concurrently. `[lint.scanner_policy] max_timeout` raises the ceiling of a run's `timeout` from 15 minutes up to a hard 6 hours; an entry cannot raise it ([#427](https://github.com/Goldziher/ai-rulez/issues/427)). See [Strict validation](https://goldziher.github.io/ai-rulez/strict-validation/).
- **Scanner results are cached per skill, and `--changed` narrows the run.** A scanner handed `{skill_dirs}` was cached as one blob and always received every skill, so one edited skill re-scanned all of them. Its result is now keyed per skill (tree hash of the skill, the command without the directories, scanner identity, environment), only the skills without a cached result are handed to the scanner, and under `--changed` or `--since` only skills with a changed file are. `--no-scan-cache` still bypasses the cache and `--dry-run` shows `hit` or `miss` for each skill ([#426](https://github.com/Goldziher/ai-rulez/issues/426)). See [Strict validation](https://goldziher.github.io/ai-rulez/strict-validation/).
- **External scanners see every file of a skill directory.** Staging copied only `SKILL.md`, `scripts/`, `references/` and `assets/`, so a file at the skill root or in another folder was never scanned. Every regular file now reaches the scanner, under the limits of an installed skill (no symlinks, no credential-like names, no `node_modules`/`.git`/virtualenvs, no file over 8 MiB). `stage_globs` and `stage_exclude` on `[[lint.external]]` narrow the set, and `--dry-run` lists the same files ([#428](https://github.com/Goldziher/ai-rulez/issues/428)). See [Strict validation](https://goldziher.github.io/ai-rulez/strict-validation/).
- **The scanner result cache follows the script a scanner runs.** A scanner written as `python3 scan.py` was keyed by the `python3` binary, so editing `scan.py` kept serving the old findings. The first existing file argument of a known interpreter (`sh`, `bash`, `python`, `node`, `deno`, `bun`, `ruby`, `perl`, `uv run`, `pipx run` and others) is now hashed into the key, and the new `cache_files` list on `[[lint.external]]` names other files a scanner loads (globs relative to the project). Existing entries are not reused ([#424](https://github.com/Goldziher/ai-rulez/issues/424)). See [Strict validation](https://goldziher.github.io/ai-rulez/strict-validation/).
- **Scanner messages are no longer cut at 500 characters without a way around it.** `max_message` in `[lint.scanner_policy]` and on a `[[lint.external]]` entry (the entry wins) sets how many characters of a finding message text output keeps. A cut message ends in `...`, `--debug` counts the cut ones per scanner, and the JSON report (`full_message`) and SARIF output (`fullMessage` property) keep the whole text ([#425](https://github.com/Goldziher/ai-rulez/issues/425)). See [Strict validation](https://goldziher.github.io/ai-rulez/strict-validation/).
- **`validate --strict --external` can tolerate scanner warnings.** `--strict` meant `--fail-on warning` for every finding, and `--strict --fail-on error` is rejected, so any scanner warning failed the run. `fail_on` in `[lint.scanner_policy]`, and now on a `[[lint.external]]` entry, sets the failing threshold of scanner findings (`AR011`) independently of `--strict` and `--fail-on`: `fail_on = "error"` reports scanner warnings without failing on them. A scanner threshold replaces the run's for that scanner's findings, so a policy `fail_on = "error"` no longer lets `--strict` fail on a scanner warning ([#429](https://github.com/Goldziher/ai-rulez/issues/429)). See [Strict validation](https://goldziher.github.io/ai-rulez/strict-validation/).
- **A scanner `version` pin is checked against the scanner, not its launcher, and `scanners doctor` applies the same check.** The range was matched against `command[0] --version`, which is `python3` or `node` for a script scanner. The new `version_command` on `[[lint.external]]` names the argv that prints the scanner's version, and `scanners doctor --external` now reports `not healthy` for a scanner outside its range instead of `ok`, exactly where `validate --external` refuses to run it ([#431](https://github.com/Goldziher/ai-rulez/issues/431)). See [Strict validation](https://goldziher.github.io/ai-rulez/strict-validation/).
- **`find_skill` takes `path` and `scope` when the server discovers the repository.** `path` keeps the skills that apply to a file (its scope and the ancestor scopes, the nearest scope winning a skill name); `scope` lists one scope, with the scopes below it when `recursive` is true. Each discovered result names its `scope`. See [Monorepos: scopes and nested context](https://goldziher.github.io/ai-rulez/mcp-server/#monorepos-scopes-and-nested-context).
- **The `ai-rulez://catalog` resource gains a `scopes` section** with each repository scope's per-kind counts and sources, narrowed by `?path=`, `?scope=` and `&recursive=true`; `skill_discovery.index` reports the index size. See [Monorepos: scopes and nested context](https://goldziher.github.io/ai-rulez/mcp-server/#monorepos-scopes-and-nested-context).
- **`find_context`, `load_context` and `context://<scope>/<name>` resources serve nested instruction files, rules and agents** (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, Copilot instructions, rules and agents directories of the enabled presets) under the same scan, session budget and `--allow`/`--deny` rules as skills; the body is read from the repository on each call, never from the index. Registered only with `--discover-skills` or `[mcp] discover_skills`. See [Monorepos: scopes and nested context](https://goldziher.github.io/ai-rulez/mcp-server/#monorepos-scopes-and-nested-context).
- **The server instructions name the first eight repository scopes by depth** (then "and N more") when discovery is on, and the repository index holds at most 20000 items, warning at start-up when it keeps only the shallowest scopes.
- **The repository index is cached between server starts.** Summaries of the indexed files (by path, size, modification time and git tree) and the scan verdicts of content with no finding (by content hash, scan level and scanner) are kept in `<user cache>/ai-rulez/monorepo-index/<repo-id>/`, never a file body, with mode 0600 through write-and-rename under a lock file. The index file carries an HMAC with a per-user secret, so an edited, planted, corrupt or version-mismatched file is discarded and rebuilt; a cache entry never bypasses `--allow`/`--deny`, and `load_context` scans the body it reads on every call. On the benchmark monorepo a warm start takes about a tenth of a cold one. See [The index cache](https://goldziher.github.io/ai-rulez/mcp-server/#the-index-cache).
- **`[mcp] index_cache = "user" | "repo" | "off"` places the index cache.** `user` (the default) falls back to a gitignored `.ai-rulez/.cache/` (`.ai-rulez-cache/` in the root layout, excluded through `.git/info/exclude`) when the user cache is not writable; `repo` always uses it; `off` keeps none. See [Configuration](https://goldziher.github.io/ai-rulez/configuration/).
- **`ai-rulez cache status` and `ai-rulez cache clear [--all]` show and drop the index cache**, with `--format json` (`schema/cache-status.schema.json`, `schema/cache-clear.schema.json`). They work in a directory with no configuration. See [CLI reference](https://goldziher.github.io/ai-rulez/cli-reference/).
- **`ai-rulez doctor` reports the index cache** (new `index-cache` check): which location is in use, its size and counts, and a warning when it is not writable or its index file is corrupt, of another version or not authentic.
- **The repository index refreshes while the server runs.** A debounced file watcher (fsnotify) rebuilds the catalog when a nested `AGENTS.md`, `CLAUDE.md`, `SKILL.md`, a `.claude`, `.agents`, `.codex`, `.cursor`, `.github` or `skills` entry, or a directory appears, changes or goes, and the server announces `resources/list_changed` through the new `ai-rulez://index` resource. `--no-watch` turns it off. See [Live refresh of the index](https://goldziher.github.io/ai-rulez/mcp-server/#live-refresh-of-the-index).
- **`create_skill` and `update_skill` write a skill into a nested scope.** With `scope` (an existing project directory), `allow_write: true` and an optional `harness`, the skill goes to `<scope>/<harness skills dir>/<name>/SKILL.md` after symlink, enabled-preset, generated-copy and strict security-scan checks; it is the harness's own file, not ai-rulez content, and with `ai-rulez.lock` present it is reported as drift until `ai-rulez lock`. `update_skill` is now annotated destructive and not idempotent. See [Writing a skill into a scope](https://goldziher.github.io/ai-rulez/mcp-server/#writing-a-skill-into-a-scope).

### Changed

- **A configuration without presets is valid: a lint-only configuration.** `presets` may be omitted or `[]` (it no longer fails with `at least one preset is required`, nor does the schema require `minItems: 1`). `generate` then prints one line, writes nothing and exits 0; `generate --check` and `doctor` report no drift; `validate`, `scan` and `[[lint.external]]` work. See [Lint-only configuration](https://goldziher.github.io/ai-rulez/config-without-folder/#lint-only-configuration) ([#430](https://github.com/Goldziher/ai-rulez/issues/430)).
- **A project that generates for Claude Code now gets a `SessionStart` hook in `.claude/settings.json` by default.** The generated file changes on the next `generate` (a new file, or a new entry merged beside your own hooks), the hook appears in the "new hook commands" notice. The hook returns at once outside a remote session. Its `npx` and `uvx` fallbacks run the latest published `ai-rulez` instead of the version of the binary that generated the file, so the generated line no longer changes with each release; set `[session_start] version` to pin one. Set `[session_start] generate = false` to opt out and remove it. Other harnesses are not affected.
- **Skill discovery finds `skills/` and the enabled presets' skill directories at any depth**, as it already did for `.claude/skills`, and each discovered skill in `skill_discovery.served` carries its `scope`. A `skills/` directory inside a hidden directory such as `.agents/` is left to that convention.

### Fixed

- **Installed-plugin notices appear once, and `marketplace add` and `plugin install` confirm what they wrote.** "Installed plugin hooks are not imported" and the name-conflict warnings were logged once per load, so twice or more per command. Both commands now print a one-line confirmation on stderr (hidden by `--quiet` and `--format json`). See [Installed Plugins](https://goldziher.github.io/ai-rulez/installed-plugins/).
- **Installed skills log fewer false alarms.** A symlink inside a whole-bundle skill is refused with a warning (it was logged as an error while the run succeeded), and "Unrecognized skill subdirectory" is no longer printed for a directory the entry's `resources` patterns name, included or excluded with `!`, or leave out with `["none"]`.
- **`generate --refresh-remote --accept-findings` generates from the tree it accepted.** The lock was rewritten to the accepted commit and `generate` then failed the import scan, leaving the lock and the outputs out of step. The accepted tree now passes the scan of that run; the next plain `generate` needs `trust = "warn"` on the entry, which the error hint names. The served-skill scan now uses the installed skill entry's `trust` instead of the default. See [Managing only remote skills and plugins](https://goldziher.github.io/ai-rulez/managing-remote-skills/).
- **CRUD commands and the MCP tools find the project from a subdirectory.** `list`, `add`, `skill list`, `domain`, `profile` and `include` failed with ".ai-rulez directory not found" below the project root, in both layouts; they now walk up like `generate` (stopping at the git root, never above `$HOME`), as do the MCP tools. MCP `init_project` still initializes the given directory, but refuses when a directory above it in the same repository holds a configuration in any layout.
- **`mcp --serve-skills --discover-skills` works in a repository with no configuration**, as the docs say, instead of exiting with "load configuration: .ai-rulez directory not found". Without discovery the error remains.
- MCP `init_project` now refuses in a directory whose configuration is `ai-rulez.toml`, instead of scaffolding a `.ai-rulez/` that would shadow it.
- `generate --watch` now also watches local installed-skill sources, `skill_sources`, and installed marketplaces and plugins, not only includes and `local_override`.
- **`skill install` and `skill list` report `git` for `owner/repo`, `github:` and `ssh://` sources**, which they logged and listed as `local`.
- **`ai-rulez local show` prints `skip_outputs`** instead of `<redacted>`: a list of output kinds holds no secret.
- **Schema and docs match the binary:** the `refresh_on_generate` text names all ten flags that skip it, and `[delivery.stub] description = ""` is documented as the same as leaving the key out.
- **`name_prefix` on `installed_skills` and `installed_plugins` prefixes the skill's output directory, not only its `name`.** Two entries that resolve skills with the same directory name and different prefixes used to fail with `duplicate output ids`, and `name:` disagreed with the directory. A prefixed skill is now written to `<prefix><dir>/`, for example `.claude/skills/vendor-pdf/`. Output paths of existing prefixed skills change (the old directory is removed as stale); lock keys do not.
- **Commands are written for `codex`, `antigravity`, `devin` and `warp` under the default `agents_md = true`.** Their command skills belong to the shared `.agents/skills` tree, which was rendered from skills only, so no command appeared and nothing warned. The tree now carries them (with `agents/openai.yaml` for Codex); on Devin the command skill in the shared tree now has `disable-model-invocation: true` instead of `triggers: [user]` (with `agents_md = false` Devin's own `.devin/skills/<command>/SKILL.md` keeps `triggers: [user]`). See [agents.md sharing](https://goldziher.github.io/ai-rulez/agents-md/).
- **Commands find the configuration from a subdirectory.** `generate`, `clean`, `lock`, `validate` and the other project commands walk up from the working directory to the nearest directory holding a config (`.agents/ai-rulez.toml`, `.ai-rulez/config.toml`, `.config/ai-rulez/config.toml` or `ai-rulez.toml`), stopping at the repository root and never above the home directory, as [Configuration](https://goldziher.github.io/ai-rulez/configuration/) always said; outputs stay relative to the config root. `-C/--config` and `--config-dir` still win. The no-config error names the directories searched.
- **A local `installed_skills` source must stay inside the project.** A committed entry could name any path, unlike includes and `[[skill_sources]]`; it now fails with the same error, with the same exceptions (`config.local.toml`, the user config).
- **A local installed skill is pinned in `ai-rulez.lock`** as a `local-skill` item, so `generate --locked` fails when its files change. Run `ai-rulez lock` once to record the pins.
- **A local installed marketplace or plugin is pinned in `ai-rulez.lock`** as a `local-marketplace` or `local-plugin` item holding the tree digest of its directory, so editing a `hooks.json` or hook script makes `lock --check` and `generate --locked` fail. Run `ai-rulez lock` once to record the pins. See [Lock file](https://goldziher.github.io/ai-rulez/lockfile/).
- **`ai-rulez lock` and `mcp --serve-skills` work on a lint-only configuration** (no `presets`, or `presets = []`). They failed with `no preset configured that can render skills`; such a project serves no skills, so the lock pins none.
- **`skip_outputs = ["commands"]` and `["skills"]` classify a command written as a skill by its source.** A command rendered as `.claude/skills/<command>` or `.agents/skills/<command>` was counted by its path, so `commands` still wrote it and `skills` dropped it. It now follows `commands`; a command whose name a skill already has is not written, so that path stays a skill.
- **A stale empty directory is pruned in a configuration without a `.ai-rulez/` folder.** `.claude/skills/<old>` stayed after a `name_prefix` change because the project root counted as the configuration directory.
- **The `generate --refresh-remote` refusal names the flag that exists.** The docs said `generate` has no `--accept-findings`; `generate --refresh-remote --accept-findings` pins the reviewed trees, and the refusal now says so.
- **`ai-rulez init` refuses a project configured by `ai-rulez.toml`.** It created `.ai-rulez/` beside the file, which then shadowed it. `init --force` keeps the file as `ai-rulez.toml.bak-<timestamp>` and scaffolds; `init --minimal` already refused.
- **`generate --user` loads the user config in user scope**, so its includes and installed skills may point outside the home project.
- **An installed skill is bundled whole, not only `references/`, `scripts/` and `assets/`.** Skills that keep documents beside `SKILL.md` (`cli.md`) or in a folder of their own (`rules/`, `templates/`) arrived without those files, so the links in `SKILL.md` were broken and generate warned that the folder was dropped. Every file in the skill directory is now bundled and listed in the generated `Resources` section, except `SKILL.md`, `evals/`, `agents/` and hidden files; a project's own skills keep the canonical layout ([#419](https://github.com/Goldziher/ai-rulez/issues/419)).
- **A whole-bundled installed skill is bounded.** Credential-like files (`id_rsa*`, `*.pem`, `*.key`, `credentials.json`, `*.tfstate` and the like) are never bundled, with a warning naming each; a single file over 8 MiB is skipped with a warning instead of failing the skill; a skill over 2000 files or 16 MiB fails with an error naming the limit and pointing at `resources`, and the skills of one entry together stay under 20000 files and 64 MiB, past which the entry fails with an error pointing at `include` and `resources`. See [Installed skills](https://goldziher.github.io/ai-rulez/installed-skills/).
- **`skip_outputs = ["instructions"]` no longer skips files that merely share a root file's name.** A skill resource, rule or docs page named `CLAUDE.md`, `AGENTS.md` or `GEMINI.md` is still written; only root files at the project root and directly in a configured scope are skipped.
- **Telemetry in a root-layout project reads only `ai-rulez.toml`** and its local overlay, not a stray `config.toml` or `config.json` at the project root.
- **`generate` in a fresh clone no longer strips hooks from a committed `.claude/settings.json`.** A clone has no machine-local record of the earlier run, so the hooks (and the self server of `.mcp.json`) already in the committed file looked like the user's; the upgrade fallback then took them back as "no longer rendered" and left `{}`. A run takes back nothing it still renders. Known limitation: a clone cannot retract a hook the configuration has since dropped, because the committed manifest is not trusted as proof of what ai-rulez wrote; the stale group stays in the committed `.claude/settings.json` and `generate --check` reports no drift for it until it is removed by hand (see [Settings](https://goldziher.github.io/ai-rulez/settings/)). The Claude Code settings document, when it carries ai-rulez-owned hook groups, is no longer pinned as an output of `ai-rulez.lock` (a clone cannot tell it is wholly ai-rulez's, so `lock --check` and `validate` reported `AR982` there).

## [5.4.1] - 2026-10-16

### Added

- The official GitHub Action accepts 16 optional `llm-*` inputs matching the CLI's environment overrides. Settings persist to later job steps; omitted inputs preserve inherited configuration. Credentials remain in the caller's environment, and setup makes no model requests.
- A GitHub Actions guide documents all inputs, environment precedence, provider credentials, Bedrock, outputs, and the distinction between the action ref and installed binary version. CI smoke-tests the composite action on Linux, macOS and Windows.

### Fixed

- Successful stable v5 publications create or advance the floating `v5` action tag after GitHub, npm, PyPI and Homebrew publication. Serialized updates reject prereleases, incomplete publications, divergent histories and rollback from older release reruns; full release tags remain unchanged.
- Reject malformed action binary versions before writing runner outputs, and remove the unsupported `AI_RULEZ_LLM_BACKEND` variable from the LLM documentation.
- The setup action honors the requested release even when another CLI is on `PATH`. Download progress stays off the returned binary path, and Windows installs retain the fallback native DLL beside the executable.

## [5.4.0] - 2026-10-16

### Added

- **`ai-rulez judge` reviews agent configuration with verified evidence.** Three reviewers (security `JR-S01`-`JR-S15`, governance `JR-G01`-`JR-G07`, quality `JR-Q01`-`JR-Q13`) read every unit (a skill with its scripts and references, an agent, command, rule, `CLAUDE.md`, `AGENTS.md`, MCP or settings file; nested `.claude/` directories at any depth) together with the leads of `validate` and `scan`. Reviewer candidates carry a verbatim quote and a line range that Go checks against the text that was sent (a failed one is corrected once, then deleted and counted); failed rubric criteria without a verifiable quote are preserved as unit-level findings; an adjudicator keeps, downgrades, merges or drops what survives, asymmetrically (a security finding is dropped only with a concrete benign context and a verbatim counter-quote); scores, statuses and the decision are computed in code. Text in a reviewed file is inert data behind nonce-fenced trust tiers. Findings carry a stable id, a chain of quoted steps and the strongest benign reading; `--explain-drops` lists everything dropped. Exit codes: 0 pass, 1 could not complete, 2 findings at or above `fail_on`.
- **`judge` governance reads your repository's own standards.** Path-scoped rules of every ancestor `.claude/rules/**/*.md` and the nested `CLAUDE.md`, `CLAUDE.local.md` and `AGENTS.md` files with their `@imports` are attached to the unit; a governance finding must quote the file and the convention it breaks.
- **`judge` runs rubric skills.** A skill with `## C1 - title` sections is a rubric: `--skill NAME|path:DIR`, `--skill-for CATEGORY=NAME` and `[judge.skills]` apply it, every criterion is reported as `pass`, `fail` or `not_applicable`, and a failure is a `JR-G06` finding.
- **`judge --changed[=REV]`, `--since REV` and `--impact`.** Only the units changed since `REV` (`HEAD` for a bare `--changed`) are judged, and with `--impact` (the default) so are the units a changed rule or `CLAUDE.md` applies to.
- **A `[judge]` config table** sets `categories`, `fail_on`, `min_confidence`, `attach_rules`, `attach_claude_md`, rubric skills with pins, prompt extensions and overrides (an override from a repository file needs `--allow-prompt-override`), a traps file and quote-pinned, expiring `[[judge.suppress]]` entries. Models, spend caps, rate limits, consent and the API key are not the repository's to set: such keys are ignored and reported.
- **`judge --format sarif`, a v2 JSON report and a `judge` MCP tool.** SARIF maps findings to `ai-rulez/JR-xxx` rules with `security-severity`, `codeFlows` and tool notifications for coverage gaps; `schema/judge-report.schema.json` describes the JSON. The MCP tool takes no model, price or cap and needs `allow_llm` in the call and in the user's `[judge]` table; `estimate` always works.
- **`judge` coverage, caps and cache.** A unit over the per-call budget is split into overlapping chunks; `--max-cost` (default 2.00 USD), `--max-tokens` and `--max-calls` admit calls in priority order and report what did not fit; `--concurrency` and `--rpm` bound the calls; answers are cached for 30 days (`--cache-readonly` serves hits only). A unit not fully reviewed is `partial` and the run `incomplete`.
- **`bench/judge/`, the evaluation of judge.** A synthetic corpus of 183 labelled units (malicious samples for every security code, benign lookalikes, false-positive traps with a malicious twin each, quality defects, a governance fixture repository, clean units), offline tests of the verifier and convention attachment, and `task bench:judge`, which runs the pipeline on Bedrock and prints precision, recall and F1 against labels.
- **`judge` works through Amazon Bedrock.** The egress host is derived from `AWS_REGION`, `AWS_DEFAULT_REGION` or the `AWS_PROFILE` profile (`bedrock-runtime.<region>.amazonaws.com`), so no `AI_RULEZ_LLM_BASE_URL` is needed; inference-profile ids are priced from the catalog and `--price-input` / `--price-output` price any other model; `AI_RULEZ_LLM_TEMPERATURE=omit` leaves `temperature` out for every model (#414).
- **`validate` detects three more agent-targeted attacks.** `AR036` (credential-store instruction in prose) and `AR037` (a bundled script that reads the environment or a credential file and sends it over HTTP) are warnings, `AR038` (an instruction to run commands without asking) is a note; `AR003` and `AR004` also flag an HTML comment that addresses the agent or tells it to hide a step (#415).
- **New builtin guidance for testing, review, configuration and decision records.** `ai-governance` gains the `generated-files` rule and the `config-placement` and `skill-authoring` skills; `documentation` gains the `adr-immutability` rule and the `adr-workflow` skill; `security` gains the `customer-data-boundary` rule. Existing builtins tighten: `testing-conventions` and `tdd-workflow` (never edit code under test to pass, verify a test can fail), `code-quality-standards` (scoped suppressions, `TODO(PROJ-123)` markers), `agent-workflow`, `verification-before-completion`, `secrets-handling` (no default for a missing secret) and the `code-reviewer` and `test-writer` agents.

### Changed

- **Dependencies and release tooling are current.** Upgrade Go modules, documentation dependencies, CI actions and pinned tools, including liter-llm 2.2.4, Registry 3.1.2, OpenTelemetry Collector 0.162.0 and Vault 2.2.0. Windows CLI tests run in isolated, exhaustive process shards alongside the remaining packages; native Go cache paths are used, and failed Windows runs preserve valid compiled objects.
- **The README documents the full capability set.** A feature table links authoring, generation, validation, judge reviews, evaluation, governance, publishing, MCP and the Go API to the detailed guides. Installation, contributor and release instructions are corrected.

- **`validate` also lints the hand-written harness files of a configured project.** A `.claude/skills/x/SKILL.md`, a nested `CLAUDE.md` or a `.claude/rules` file that the configuration does not list used to be skipped, so `validate` and `scan` reported a project clean that had findings. They are linted now; the files `generate` recorded and files with the generated header stay outputs. `catalog` and `publish` keep to the configured content.
- **`--changed` takes an optional base revision on `validate`, `scan` and `judge`.** A bare `--changed` is unchanged (it compares with `HEAD`: uncommitted and untracked work); `--changed=REV` compares with `REV`, so `--changed=origin/main` replaces `--since origin/main`. Write the value with `=`: a separate word stays a path argument. `--since REV` keeps working as the same thing. `--changed` was a boolean flag before, so `--changed=false` is no longer accepted; leave the flag off instead.
- **`validate` is an order of magnitude faster on a large repository.** On a monorepo of about 90,000 tracked files, 230 skills and deeply nested `.claude` directories (macOS, 18 cores, heavily loaded machine), config-free `validate --strict` went from 195 s to 14.6 s, `--path` with three files from 11.1 s to 2.1 s and `--changed` from 60 s to 5.9 s, with identical findings. The time was spent in git: each `SKILL.md` started two git processes that read the whole index, and `git ls-files --others` walked the work tree on one core. A run now lists the files once and shares the listing between target resolution, skills and the changed-files narrowing; the ignore state of all entry files is one batched `git check-ignore`; untracked files of a repository with a large index come from a parallel directory walk; and the Python files of the `pytest -m` marker scan are read concurrently. Small repositories keep the single `git ls-files` call.
- **Every command that calls a model resolves and refreshes the AWS credential chain for a `bedrock/` model.** `judge`, `eval`, `verifiers`, `review`, `improve` and `search` share one client: the default chain (environment, `AWS_PROFILE`, SSO, assume-role, web identity, container and instance roles) is resolved through the AWS SDK's cache, temporary credentials are renewed before they expire, and a call refused as unauthenticated renews them and is tried once more. `llm doctor` names the Bedrock host.
- **Claude 5.x models get no `temperature` up front.** The parameter is left out instead of being sent, rejected with HTTP 400 and retried; the retry remains for other models.
- **The response cache can expire entries and be read-only** (`llm.Options.CacheMaxAge`, `CacheReadOnly`). A reply that a caller validated is cached even when a provider stamped a content-filter stop on a whole, valid reply.

### Fixed

- **Private npm packages include the Windows fallback DLL.** The package file list retains `bin/liter_llm_ffi.dll` beside a prebundled executable, so offline packages keep the native library needed by a DLL-linked build.
- **Virtual Go API workspaces produce relative output plans on Windows.** Memory and snapshot workspaces use a fully qualified synthetic root, preventing duplicated output paths, incorrect stale-output removals and inconsistent plan digests. Disk-write and path-confinement restrictions are unchanged.
- **Strict validation distinguishes generated OKF listings and ignored unmanaged files.** Bundle index and log files are recognized as generated listings; ignored discovery-only fixtures no longer consume authored load budgets, while configured sources remain budgeted.

- **Judge prompt inputs stay within their intended boundaries.** Repository rubric and trap paths cannot escape the project or traverse symlinks; every outgoing model request redacts credential patterns and configured secret values. Scoped rules also attach other rules with overlapping globs.
- **Large-repository discovery includes directories that replace tracked files.** The optimized untracked walk now agrees with Git when a tracked file becomes a directory containing new instruction files.
- **More `validate` checks and fewer misleading messages.** New `AR039` (a committed `.claude/settings.json` that allows `Bash(*)` or every tool, or sets `defaultMode: bypassPermissions`) and `AR040` (a settings key one or two edits from a known key, a probable typo); `AR963` reports a marketplace entry whose local `source` directory is missing; `AR101` reports an unclosed `[` or `{` in a `paths` glob as malformed instead of "matches no tracked file"; a `--path` directory names the files it leaves out; the summary line says `validation:` rather than `strict validation:`; config-free runs no longer print "Unrecognized skill subdirectory" warnings for content that is only linted; `AR101` is not raised when the repository index lists no file.
- **CLI.** `sbom --help` and the docs use `--type spdx-json` (`--format` is the report format); `init --yes` never prompts to replace an existing configuration directory; `init --format json` prints only the result document; the `.ai-rulez.bak-*` directory `init --force` leaves is excluded from git; `convert --dry-run --format json` with nothing to convert prints an empty report on stdout; `ai-rulez mcp --read-only` no longer registers the `judge` tool.
- **`judge`.** A unit is withheld only for credentials the `AR001` scan recognises, and the message names file:line and pattern; SARIF output validates against the 2.1.0 schema (no prose-only `fixes`); ignored `[judge]`/`[llm]` repository settings are reported as notices (text, JSON, SARIF); `--estimate` covers every unit, counts calls once and exits 1 when over `--max-cost`; a failed rubric criterion always becomes a `JR-G06` finding; a named path through a symlink is refused.
- **`judge` never follows a symlink, and `mcp --read-only` no longer registers `judge`.** A named path that was a symlink to an in-repo file or directory, or lay below one, was resolved and judged although the docs say symlinks are never followed; it is now refused. The `judge` tool sends content to a model provider and spends money, so the read-only MCP tool set excludes it.
- **`judge` no longer loses a failed rubric criterion that has no verifiable quote.** A criterion reported as `fail` in `criteria_results` with no `JR-G06` finding (an absence such as a missing `## Verify` heading) or with a quote that is not in the file was dropped by grounding. It now surfaces as a unit-level `JR-G06` finding (`absence: true`, `origin: criteria`, quote unverified when the reviewer gave one that did not verify), lowers the governance score, and the dropped candidate is listed with its `rubric_id` under `--explain-drops`.
- **`judge --estimate` is consistent and honest about the cap.** It estimates every unit instead of failing at the 200-unit limit (a notice says a real run would be refused), counts one definition of calls (`estimate.calls` planned, `estimate.calls_fit` under the caps, in the note too), includes the adjudicator schema in its prompt estimate, and warns and exits 1 when the estimate exceeds `--max-cost`. A test holds the prompt estimate within 15% of a tokenizer count of the prompts actually built.
- **`judge` says what it ignores in the repository config.** A repository `[llm]` provider, model or base URL, and `model`/`provider`/`base_url` in a repository `[judge]` table (which `validate` rejects as unknown keys while `judge` used to proceed silently), now produce a notice naming the key and where to set it; any other unknown `[judge]` key is an error as in `validate`. All judge notices are also emitted as SARIF `toolExecutionNotifications`.
- **`judge --format sarif` is valid SARIF 2.1.0 again.** A suggested fix was emitted as `fixes:[{description}]`, which lacks the required `artifactChanges`; the suggestion now goes into the result message and `properties.suggestedFix`. A test validates the output against the SARIF 2.1.0 schema.
- **`judge` withholds a unit only for a credential the security scan (AR001) recognises.** The secret gate matched any `token`/`secret`/`api_key` assignment, so code such as `aws_session_token=credentials["SessionToken"]` or `secret = vault.read_secret_value(name)` was withheld although `validate` called it clean. Both now share `internal/secretpat`; the withhold reason names `file:line` and the pattern, never the secret.
- **Fewer false positives in `validate --strict` reference and credential checks** (22 errors on a large monorepo went to 2, both genuinely dangling). AR402 also resolves a skill's `scripts/`, `references/` and `assets/` paths from the area root (the directory that holds the skill's `.claude/` directory), and from the skill a sentence names ("see skill `x`, `references/y.md`"); the files of a skill no local file defines are not checked, and placeholder paths (`references/services/X.md`, `...`, globs) are skipped. AR301 ignores plugin-qualified names (`pr-review-toolkit:code-reviewer`), the skills the Claude Code harness and its bundled plugins provide (`simplify`, `loop`, `security-review`, `dataviz`, ...; `lint.known_names` covers project-specific externals), a plain word before "rule" (a CSS `display` rule; a hyphenated name or "rule file" still counts), and names a sentence denies or puts in the past ("the former `x` skill", "NOT a registered slash command", "Don't add a `references/patterns/` tier"); an unknown `` `/name` skill`` is information instead of an error, since a user-level skill is not in the repository. AR001 no longer reports a snake_case identifier such as `saq_tn3270_scan_password` as a credential.
- **AR301 no longer reports prose about a linter rule.** A singular `` `x` rule `` after a possessive or a tool name (`eslint's`, `ruff`, `lint`) is prose, and rule references are not reported in a tree with no rule items; `lint.known_names` silences a name (#416).
- **Config-free `validate` lints `.claude/{rules,commands,agents}` at any depth.** Only the root `.claude` directory was discovered, so a nested `pkg/.claude/rules/x.md` was neither linted nor indexed for `AR301` (#411).
- **AR101 resolves `paths:`/`globs` of a nested `.claude/rules/*.md` relative to the directory holding `.claude`.** Such globs were matched from the project root and reported as matching no file (#412).

### Removed

- **The v1 per-file score report of `judge`**, its `--rubric` and `--min-score` flags and the built-in `builtin:instruction-quality` rubric.

## [5.3.0] - 2026-10-14

### Added

- **`ai-rulez judge [paths...]` grades instruction files with a model, with no configuration file.** With no paths it judges `AGENTS.md` at any depth and the `.agents/` and `.ai-rulez/` Markdown; named files and directories work anywhere in the project, symlinks are never followed and generated files are skipped. The model comes from `--model provider/model` and `AI_RULEZ_LLM_*` variables and the key from the provider's own variable; no repository or user config is read. A real run needs `--allow-llm`, prints the egress host and model first, refuses a non-https endpoint (loopback excepted), a model with no known price under `--max-cost` (default 0.25 USD) and a worst case above the cap, and honours an organization policy that forbids model use. A file with secret-looking text is not sent and counts as a finding unless `--redact` masks it; long files keep head and tail. `--estimate` (`--dry-run`) sends nothing. The built-in rubric `builtin:instruction-quality` (or `--rubric <file>`) scores clarity, actionability, non-contradiction, scope, secrets and stale commands. Exit 0 all pass, 1 could not run, 2 below `--min-score` or skipped for secrets. `--format json` follows `schema/judge-report.schema.json`; see [Judge](https://goldziher.github.io/ai-rulez/judge/).
- **The agents source reads an `AGENTS.md` in a subdirectory as context scoped to that directory.** `svc/api/AGENTS.md` becomes the item `agents-svc-api` with the path scope `svc/api/**`, rendered like any path-scoped context, and tools that read `AGENTS.md` themselves get no copy. The walk skips symlinks, hidden, dependency and build directories and anything `.gitignore` ignores, stops at a subdirectory with its own ai-rulez configuration, is bounded in depth and size, and never reads a file with the generated banner.
- **`agents_md_split = true` reads `AGENTS.md` as one context item per `##` heading.** With the agents source the items are named `agents-<heading slug>` (`agents` for the text before the first heading, `-2`, `-3` for a repeated heading) and `list context`, `remove context` and the MCP list and delete tools address them; removing one deletes that section from `AGENTS.md` and leaves the rest of the file byte for byte. Without the setting `AGENTS.md` stays one item named `agents`, now shown by `list context`, and `remove` refuses to touch it. `validate` rejects the setting without the agents source.
- **`validate --outputs` and `[lint] lint_outputs = true` lint the generated files.** The security, references and descriptions analyzers run over the bytes `generate` writes (or would write, whether or not it has run), including files a past run recorded that the plan no longer writes. A finding carries `"source": "output"` in JSON, `properties.source` in SARIF and `[generated output]` in text; a generated copy of an authored item is skipped and a finding an authored file already has is dropped, so a problem is reported and counted once, at the authored path. Findings on outputs carry no fix. With no configuration, the files that carry the generated header are the outputs. `schema/validate-report.schema.json` documents `source`.
- **`validate`, `scan` and `doctor` work in a repository with no configuration file.** When nothing is found in the working directory or above they print `no ai-rulez config found; linting detected files` on stderr and use an implicit configuration (no presets, includes or hooks, default `[lint]`, the git toplevel or the working directory as the root). `AGENTS.md`, `CLAUDE.md`, every `SKILL.md`, `.claude/{skills,agents,commands,rules}`, `.agents/**`, `.cursor/rules`, an `.ai-rulez/` tree without `config.toml`, plugin manifests, hook files and `.mcp.json` are found by convention and treated as hand-written (a file with the generated header is left out). `--path` and `--glob` replace the detection, nothing is ever written, the baseline needs `--baseline <file>` unless `.ai-rulez/` exists, and the exit codes are unchanged. `generate`, `clean`, `lock` and the other commands that need a configuration now fail with `run ai-rulez init`.
- **`validate` and `scan` lint files outside the content tree with `--path`, `--glob` and `[lint] paths`.** Each takes files or directories (globs are relative to the project root), repeatable; every one must resolve inside the repository root once symlinks are followed, and untracked files that the repository does not ignore are included. A plugin manifest, a marketplace, a hook file or an MCP file is recognized by its content, so one kept in an unusual directory gets the same checks (`AR963`, `AR507`, `AR6xx`) as the one in `.claude-plugin/`, `hooks/` or `.mcp.json`; `SKILL.md`, `AGENTS.md`, `CLAUDE.md` and files under `agents/`, `commands/`, `rules/` and `checks/` are classified by name and place. A file with the generated header is skipped, `--update-baseline` refuses `--path` and `--glob`, and `--fix` still never edits a generated or out-of-root file.
- **A new [Architecture](https://goldziher.github.io/ai-rulez/architecture/) page** describes the load, merge, validate, render and write pipeline, the three source layouts, recursive runs, the plugin pipeline, the lint pipeline and the package map. The shipped `ai-rulez` skill, the READMEs and `docs/monorepo.md` now describe the agents source, config-free lint, `judge` and the ordering of recursive output as the code behaves.

### Fixed

- **`validate --recursive` no longer drops a `--path` or `--glob` file that lies outside every root.** Each root linted only the picks below its own directory, so a pick elsewhere in the repository (a root-level `README.md`, a `docs/` folder) was never linted and the run could pass. Such files are now linted once at the repository root with the implicit configuration, named in the notices, and counted in the exit code.
- **`--path <directory>` honours `.gitignore` and never walks `.git`.** A directory pick (`--path .` included) walked the whole directory, so an ignored `node_modules` failed the run on the 20,000-file limit or had its contents linted, and `.git` was read. In a git repository the directory now takes the tracked files and the untracked files git does not ignore; outside git it is walked skipping the usual dependency, build, cache and hidden directories below it.
- **A hand-written file can no longer hide from config-free `validate` by quoting the generated marker.** `AGENTS.md`, `CLAUDE.md` and the other lint targets, and the harness traps, took any file with `Generated by ai-rulez` or `AI-RULEZ :: GENERATED FILE` anywhere in its first 2 KB for generated output and skipped it silently, so a `sub/AGENTS.md` with the marker under a heading and a hostile body was never scanned. One detector now serves `generate`, the lint targets and the traps: the marker must be the file's own header (the first comment after a byte order mark and frontmatter, or a `Content-Hash` comment in a skill's or agent's frontmatter), and a stored `Content-Hash` must match the body. A conventional file left out as generated is named in the `notices` instead of being dropped silently; a header alone is still text anyone can write, so read that list.
- **The harness traps recognize the real `AI-RULEZ :: GENERATED FILE` header.** Only the old `Generated by ai-rulez` banner counted, so a file `generate` wrote was judged as hand-written (a warning, with an autofix that `generate` would overwrite) and a gitignored output with the real header was never scanned. Such a file is now an error that names a preset bug, with no fix, and gitignored outputs are scanned.
- **The docs state that a named non-markdown file gets only the credential scan.** A text file named with `--path` or `--glob` that is no content kind is checked for `AR001` and `secret_patterns` only; the injection, hidden-character and link rules apply to markdown and the recognized JSON kinds.
- **`--recursive` reports a not-linted file once.** A file picked with `--path` or `--glob` that two roots both skipped appeared twice in the `notices` of the combined report and on stderr; duplicates are now dropped, in first-seen order.
- **`judge` reports every file it skips, and a skipped named file no longer aborts a list.** A file over 2 MiB, a binary file and a generated file named on the command line or found below a directory are `skipped` entries with a reason (a named file over 2 MiB used to fail the whole run); a run with no paths still leaves generated files out silently. The summary's `calls`, `tokens` and `cost_usd` were 0 with `--max-cost 0` and no `--max-tokens`; they now count the answered calls. Cap amounts in messages keep four decimals, so a `$0.0004` cap is no longer shown as `$0.00`. The docs now say that `judge` has no `--max-calls`, grades files one after another, and passes a key only from `AI_RULEZ_LLM_API_KEY_ENV` to a custom endpoint.
- **`judge` refuses a real run when it cannot tell which host receives the file content.** With a provider ai-rulez does not know (anything but openai, anthropic, gemini, mistral, groq, deepseek, xai, openrouter and cohere, whose hosts come from liter-llm's provider registry) or a bare model, and no `AI_RULEZ_LLM_BASE_URL`, the egress line used to say "the provider's default endpoint" and the run went ahead. It now stops and names `AI_RULEZ_LLM_BASE_URL`; `--estimate` still works.
- **`judge` strips terminal escapes from everything it prints and defuses fence markers of any length.** A file name, a provider error or a rubric path with an escape sequence or line break reached the terminal and the JSON report; control characters now become spaces in every string of the text and JSON output. A run of five or eight `<`/`>` in a judged file survived the old three-bracket replacement and could still form a fence; a space now breaks every run before it reaches three.
- **`judge` no longer sends a secret in a rubric, a path or a file's copy of the API key.** A `--rubric` file with secret-looking text, or the value of the API key variable, is refused (`--redact` does not apply to a rubric). The value of `AI_RULEZ_LLM_API_KEY_ENV`'s variable and of the provider's own key variable is treated like an `AR001` secret in judged files: the file is skipped, or masked with `--redact`, and the value is never printed. A file path is masked the same way in the prompt and the report.
- **`judge` refuses the home directory and `/` outside a project, and bounds its directory walk.** Run from `$HOME` or `/` with no git repository, it listed unrelated projects' `AGENTS.md` files and could walk for minutes; it now stops with the "not a project" refusal `validate` gives, also when paths are named. Outside git the walk stops with an error past 100,000 entries, and default discovery skips directories deeper than 8 levels.
- **`judge` lists a named file it skips instead of reporting "nothing to judge", and refuses default discovery in a folder that is not a project.** Naming only a file over 2 MiB or a generated file ended with `nothing to judge` and no report, and a binary file alone exited 0 although nothing was judged. The skipped file is now an entry with its reason and `bytes`, and the run exits 1 with `nothing judged` (a file skipped for secrets still exits 2). In a directory with no repository and no `AGENTS.md`, `.agents/` or ai-rulez config at its root, a run with no paths now refuses and asks for explicit paths, instead of judging the `AGENTS.md` of every unrelated project below it.
- **`scan --path .env` no longer reports "no findings" for a file it never read.** A file named with `--path` or `--glob` that was not a recognized kind (`.env`, `.yaml`, any other extension), was over 2 MiB, was not a regular file or left the project through a symlink was dropped without a word. Now every such file is reported as `<file>: not linted (<reason>)` on stderr and in a new `notices` array of the JSON report, and a text file of any extension that you name is scanned for credentials (`AR001`) as plain text (at most 2 MiB, never through a symlink). A file named with `--path` that cannot be read, is not a regular file or is over 2 MiB fails the run with exit 1; the same file matched by a `--glob` is a notice. `schema/validate-report.schema.json` documents `notices`, and `docs/cli-reference.md` now says that a `--path` that does not exist or leaves the project root exits 1.
- **`--path agents.md` is `AGENTS.md` on a case-insensitive file system.** On macOS and Windows the lower-case spelling was not classified as an instruction file, and naming both spellings linted the file twice. A path now takes the spelling the file system stores.
- **`--outputs` no longer hides a different secret of the same type.** A finding on a generated file was dropped when an authored file had one with the same rule and message, and AR001 masks the secret, so a stale generated copy holding a rotated key looked like the authored one. The flagged line is now part of the comparison.
- **A project without configuration no longer lints the files `generate` wrote as if they were hand-written.** Only the old `Generated by ai-rulez` banner was recognized, so an `AGENTS.md` or `CLAUDE.md` with the real `AI-RULEZ :: GENERATED FILE` header was linted as a source, untagged, and its findings were reported twice next to the authored ones. It is now skipped, and with `--outputs` linted and marked `"source": "output"`.
- **`validate --recursive` in a repository with no configuration no longer passes unchecked.** It printed `No configuration files found` and exited 0 even with a secret in `a/AGENTS.md`. It now lints the implicit root exactly as the run without `--recursive` does (`--path` and `--glob` honoured), so the findings and exit codes match; `--config-only` still reports that no configuration was found.
- **`schema/validate-report.schema.json` declares the finding key `file`, not `path`.** `validate --format json` and `scan --format json` have always emitted `file`; the schema described a key that never appeared. A test now compares the keys findings emit with the schema, and checks that SARIF carries `source` in `results[].properties`.
- **The refusal to lint a home or filesystem-root directory no longer suggests `--path` or `--glob`.** Those flags do not lift the refusal; the hint now says to run the command from inside a project directory.
- **A nested `AGENTS.md` in a directory named with glob metacharacters is scoped to that directory only.** `app[1]/AGENTS.md` produced the scope `app[1]/**`, which matches `app1`; the directory name is now escaped (`app\[1\]/**`).
- **Nested `AGENTS.md` item names no longer depend on the walk or on `.gitignore`.** Directories whose slugs collide (`a/b`, `a-b`, `a b`; `app[1]`, `app1`) or sanitize to nothing got `-md` or numeric suffixes that changed when an unrelated directory was added or ignored, which churned the outputs and the lock. They now carry a short hash of their path, decided over all directories found before `.gitignore` is applied.
- **Removing an `AGENTS.md` section is atomic and safer.** `remove context` on a split item now writes a temporary file and renames it over `AGENTS.md`, keeps the file mode, refuses a symlinked `AGENTS.md`, and aborts when the file changed in between instead of overwriting the edit. A leading BOM is kept, CRLF files work, and a file with lone CR line endings gets a clear error instead of "could not locate the sections".
- **`remove context` on a split `AGENTS.md` item names `AGENTS.md`, not a file that never existed.** It printed `.agents/context/agents-<name>.md`; it now prints `AGENTS.md` and says which section was removed (`--format json` carries the same path).
- **Nested `AGENTS.md` discovery says when `.gitignore` is not applied and ignores symlinked configuration.** Outside a git repository nothing is ignored; a root `.gitignore` there now produces a one-line warning, and the behaviour is documented. A symlinked `config.toml` or `ai-rulez.toml` no longer marks a subdirectory as a project of its own.
- **`read_skill_file` and `list_skill_resources` reject out-of-range `offset` and `length` instead of converting them.** A JSON number such as `1e30` or `NaN` was cast to `int`, which Go leaves platform-dependent. One shared check now accepts whole numbers from 0 to 2147483647 and answers anything else with an error naming the argument; a negative `list_skill_resources` offset used to be clamped to 0 and is now an error too.
- **`find_skill`, `search_skills` and `load_skill` reject out-of-range `limit` and `budget_bytes` instead of converting them.** The same `1e30` or `NaN` cast to `int` that `offset` and `length` had; the arguments still treat zero or a negative number as the default, but a non-finite, fractional or larger-than-2147483647 value is now an error naming the argument.
- **`read_context` and `update_context` (and `show`/`edit context`) handle `AGENTS.md` items.** For a split `AGENTS.md` item the read returned the whole file (or failed with "outside the configuration directory") and the update failed with "file not found". The read now returns just that section, and the update rewrites just that section atomically (mode, line endings and BOM kept, refused when the content would add, drop or rename a `##` heading or the file changed meanwhile). The unsplit item `agents` reads as the whole file and `update`/`remove` refuse it with a message saying why.
- **`generate --watch` and `doctor` cover nested `AGENTS.md` files with the agents source.** Watch mode watched only the root `AGENTS.md` and `.agents`, so editing a nested file regenerated nothing; it now watches the nested files the loader reads (a file created while it runs is picked up after the next run). `doctor` now reports a generated nested `AGENTS.md` left in the source, as it did for the root file. The docs also state that outside a git repository the whole tree is read and that a `## Agents` heading becomes `agents-agents`.
- **Updating the last section of a split `AGENTS.md` works for CRLF files and keeps the file's ending.** `edit context` and `update_context` on the last section of a CRLF file always failed with a bare-CR error, because a `\r` was added to the final line even when no `\n` follows it, and an LF file lost its final newline. A line now gets its `\r` only when a newline follows, and the file keeps or lacks its trailing newline as it had.
- **The docs state two limits of the split and nested `AGENTS.md` sources.** Every item of a split `AGENTS.md` pins the whole file in `ai-rulez.lock`, so editing one section reports "source changed" for every item of that file in `lock --check` and `generate --check`. Adding a nested directory whose slug collides with an existing one (`a b` next to `a-b`) adds the path hash to both, so the existing `agents-a-b` is renamed too.
- **OKF metadata is projected into the native plugin bundles.** A skill, command or agent written in Open Knowledge Format (`type:` plus an `x-ai-rulez:` block) was copied into the `claude`, `agent-plugins`, `opencode` and `pi` bundles with that frontmatter, which the hosts do not read. The bundled `SKILL.md` and command and agent files now carry native frontmatter (`name`, `description`, `license`, ... at the top level, `type`, `title` and `x-ai-rulez` removed), body and source file unchanged.
- **A combined Pi and OpenCode bundle refreshes the OpenCode `package.json` metadata, and OpenCode uses the Pi package name.** With `package_json = "merge"` an authored root `package.json` kept a stale `type`, `main`, `exports` and `@opencode/plugin` range, and the two runtimes could name the package differently; the OpenCode entry point and dependency are now written over the authored ones, `[plugin.pi] package_name` names the combined package, and `verify --plugin` checks those keys.
- **A non-object `dependencies` value no longer panics the npm manifest merge.** A Pi or OpenCode `package.json` whose `dependencies` was not an object made the combined-bundle merge panic; it is now left to the disagreement check.
- **`AR9T0` recognizes the root `plugin.json` and `mcp.json` written for `agent-plugins` and `copilot`.** Only the Codex `root` layout counted as writing them, so a bundle with `runtimes = ["codex", "agent-plugins"]` (Codex on the default `legacy` layout) was reported for files it ships on purpose. The two runtimes now count like the Codex `root` layout, which changes the `AR9T0` findings for such a bundle.
- **A combined Pi and OpenCode `package.json` only takes the OpenCode-owned keys from the OpenCode manifest.** With `package_json = "merge"` the OpenCode manifest overwrote every Pi key except `files` and `keywords` (`name`, `version`, `description`, `scripts`, ...) and skipped the disagreement check. It now refreshes only `type`, `main`, `exports`, `dependencies`, `homepage`, `license` and `packageManager`; any other key that differs fails with `runtime npm manifests disagree`.
- **A credential in an MCP server URL path is no longer published.** `publish --emit mcp-server-card` and the ARD manifest skip, with a note that never prints the URL, a remote server whose path has a segment that looks like a key (`/mcp/sk-abc123`, a `ghp_` token, an AWS key id, or an opaque token of 32 or more letters and digits). The path is otherwise emitted as written, which the docs now state.

## [5.2.1] - 2026-10-14

### Fixed

- **A live reload no longer restores a served catalog that is missing a skill saved a moment earlier.** The catalog cache was keyed after the configuration load, so a save finishing in between cached the half-saved render under the finished file's key. The cache is now written only when the files did not change during the build.
- **`generate --check` names the toplevel `.gitignore` relative to the real working directory.** A directory reached through a symlink or a Windows short name made the path climb out and back in.
- **On Windows, `plugins/` or `marketplaces/` being a file is reported instead of read as no definitions.** Windows lists a file as path-not-found.

## [5.2.0] - 2026-10-14

### Added

- **Served skills that bundle scripts can now be used over MCP.** `list_skill_resources`, `load_skill`, `get_skill` and the `skills/list`, `skills/get` and `resources/list` metadata report each file's `mode`, `executable` and an `interpreter` hint; `load_skill` adds `files_access` saying the files are not on disk and how to fetch and run them; and `read_skill_file` takes `encoding: base64`, `offset` and `length`, so binary assets and large files no longer fail with "is binary" and are read in chunks of at most 256 KiB under the session budget. A `materialize_skill` tool is not added; the docs give the client-side recipe ([#405](https://github.com/Goldziher/ai-rulez/issues/405)).
- **`publish --emit mcp-server-card` writes one MCP Server Card per remote MCP server, and the ARD manifest embeds the same card.** The card follows SEP-2127 (Final) as defined by the experimental server-card extension, vendored at a pinned commit and checked by new conformance tests. Header values and environment values are never written (a header becomes a required input, marked secret when its name or value is a credential), and stdio servers have no card. The ARD `mcp-server-card+json` entries now carry this card instead of the earlier ad hoc `transport` object and no longer list stdio servers. The standards table lists the card as partial (experimental extension); the "agent bundle" row says no upstream spec exists ([#392](https://github.com/Goldziher/ai-rulez/issues/392), [#393](https://github.com/Goldziher/ai-rulez/issues/393)).
- **`gitignore_exclude` keeps committed outputs out of the managed `.gitignore` block, and `generate` warns about tracked files and negation conflicts.** Entries use gitignore notation (`AGENTS.md`, `**/CLAUDE.md`, `.claude/skills/`) and never apply to machine-local or secret outputs; `gitignore` stays a boolean. `generate` warns once when the block matches a file git tracks (the whole recursive block is checked, nested paths named, unanchored `AGENTS.md`/`CLAUDE.md` lines called out) and when a `!pattern` of yours un-ignores a managed line, naming its `file:line` ([#398](https://github.com/Goldziher/ai-rulez/issues/398)).
- **`default_layers = [...]` in `config.toml` activates committed `config.<name>.toml` layers without an environment variable.** They merge before `AI_RULEZ_LAYERS`; a listed layer with no file is an error, and `ai-rulez config layers` shows each layer's origin (`default`, `env`) in text and JSON ([#401](https://github.com/Goldziher/ai-rulez/issues/401)).
- **`[[marketplace.plugins]]` merges by `name` across layers and the local overlay.** A layer can add, override or drop (`remove = true`) one plugin without restating the list; it used to replace the whole array. The new `plugins_authoring` and `marketplaces_authoring` lists merge the same way ([#401](https://github.com/Goldziher/ai-rulez/issues/401)).
- **Several plugins and several marketplaces from one root.** `[[plugins_authoring]]` entries (or `.ai-rulez/plugins/<name>.toml`) each bundle only the skills, commands, agents, domains, MCP servers and hooks they select, with their own runtimes and author; `[[marketplaces_authoring]]` entries (or `.ai-rulez/marketplaces/<name>.toml`) each write their own index, provenance sidecar and `plugins/<name>/` bundles to their `output_dir`. `[[marketplace.plugins]]` accepts the same new fields. `generate --plugin` and `verify --plugin` process them all in one run; `--plugin-name` and `--marketplace-name` limit a run, and `verify --plugin --format json` adds `plugins` and `marketplaces` statuses and names the plugin and marketplace on each `drift` entry. The single `[plugin]`, `[marketplace]` members and nested-root layouts are unchanged ([#401](https://github.com/Goldziher/ai-rulez/issues/401)).
- **`AR9T1 plugin-selector-unmatched`** warns about a plugin selector that matches no content, and `validate` rejects duplicate plugin or marketplace names, a marketplace that lists an unknown plugin or none, two marketplaces with one `output_dir`, `[plugin]` with `[[plugins_authoring]]`, `[marketplace]` with `[[marketplaces_authoring]]`, and two member plugins sharing a name ([#401](https://github.com/Goldziher/ai-rulez/issues/401)).
- **`.agents` and the root `AGENTS.md` can be the source of truth.** `source = "agents"` (or a configuration in `.agents/ai-rulez.toml`, preferred when present, with the manifest, lock and `local/` in `.agents/.ai-rulez/`) reads `AGENTS.md`, Agent Skills in `.agents/skills`, `.agents/rules|agents|checks` and the ai-rulez extensions `.agents/commands|context|domains/<name>`, and translates them to every other tool without ever writing `AGENTS.md`, a scope's `AGENTS.md` or anything under `.agents/`. Every other tool gets its full generated files (`agents_md` is forced off) and tools that read `AGENTS.md` themselves get no copy of it. A generated header marks a file as output so it is never read back as source, `clean` never removes the source, and the lock, watch, guard, doctor, `add`/`remove`/`list`, the MCP tools and recursive discovery learn the layout. `init --source agents` (and the `init_project` argument of the same name) starts one, `convert --into .agents` and `init --source agents --from ...` migrate other tools' files into it. See [.agents as a source](https://goldziher.github.io/ai-rulez/agents-source/).
- **A root-level `action.yml` lets any workflow `uses: Goldziher/ai-rulez@<ref>` directly.** It installs a sha256-verified release binary, resolving `latest` or a pinned version, caches the download and adds `ai-rulez` to `PATH`; `scripts/pre-commit/run-ai-rulez.sh` gained `AI_RULEZ_RESOLVE_ONLY=1`, which prints the resolved binary's path instead of running it.

### Changed

- **Behaviour change: the `mcp-server-card+json` entries of the ARD manifest changed shape.** Each entry's `data` is now a full MCP Server Card (SEP-2127) instead of the earlier ad hoc object: the `transport` object is replaced by the card's `remotes` list, and stdio servers, which a card cannot describe, are no longer listed at all. A consumer that read `transport` or looked for a stdio server in the manifest must read `remotes` instead, and a manifest published before 5.2.0 differs from one regenerated now.

### Fixed

- **The `default_layers` description says where layers sit in an `.agents` project** (`.agents/.ai-rulez/`), in the schemas, the type docs and the MCP server docs.
- **A UTF-8 BOM before a generated banner no longer hides it.** The generated-file check strips a leading BOM, so such files are still recognized as generated by `generate` and `convert`.
- **The compact generated header keeps its wording for every default project.** The `STRUCTURE:` line was reworded for all projects, which changed every compact-header `Content-Hash`; only an `.agents` source project uses the prefixed directory names now.
- **Interrupting a recursive `validate` or `generate --check` stops it from starting more roots, and loggers made with attributes or groups share one write lock.** The workers kept taking roots after Ctrl-C, and the run now exits 1 naming the interruption instead of reporting the roots it did read as the whole result. Child loggers (`With`, `WithGroup`) each had their own lock on the shared stderr stream, so concurrent records could interleave.
- **A recursive `validate --strict` loads the git trees of different repositories in parallel.** The tree cache held its lock for the whole load, so roots of separate repositories queued behind each other; now only roots of one repository share a load.
- **The shared git index listing of a recursive run no longer answers where `git ls-files -- :(literal)` would differ.** A path below a submodule now goes to git (as before the listing), which fails closed there.
- **`validate --recursive --strict --external` checks one root at a time.** The scanners' dry-run plan went to stdout and `--write-baseline` wrote one file from several roots at once; the output could interleave and the baseline lose entries.
- **Warnings from the roots of `validate --recursive` and `generate --check --recursive` come out in root order.** Refused symlinked content, `--gitignore` notices, policy warnings and configuration-load log lines were written from the worker goroutines, so their order changed from run to run; each root now collects them and the run prints them with that root's result.
- **`init --source agents` warns about a configuration it shadows or is shadowed by.** Writing `.agents/ai-rulez.toml` beside an existing `.ai-rulez/config.toml` (which is then ignored), or a `config.toml` beside an existing `.agents/ai-rulez.toml`, said nothing until `doctor` ran.
- **A hand-written file that quotes the generated header is no longer taken for output.** The check looked for `AI-RULEZ ::` anywhere in the first 2 KB, so an `AGENTS.md` documenting ai-rulez was ignored as source (and `convert` dropped such a file); the banner must now be the file's header, and `# Content-Hash:` lines count only inside the frontmatter. CRLF files are still recognised.
- **Layers of an `.agents` project are documented and their messages name the right directory.** `config.<name>.toml`, `config.local.toml` and the plugin definition files are read from `.agents/.ai-rulez/` (not beside `.agents/ai-rulez.toml`); `config layers` and the `default_layers` missing-file error now name that path. The `.agents` page also documents that the first `#` heading of `AGENTS.md` is dropped when rendered, that `list`, `remove` and the MCP tools cannot address the `agents` context, and that a symlinked `AGENTS.md` is never followed.
- **With `source = "agents"`, the generated header and the new-commands warning name the real source.** The header said `Source: .agents/.ai-rulez/ai-rulez.toml` and "modify .agents/.ai-rulez/ content" (or `.ai-rulez/`), and the warning named a `config.toml` that does not exist; they now name `.agents/ai-rulez.toml` (or `.ai-rulez/config.toml`) and `.agents/`.
- **`source = "agents"` projects no longer get the legacy-layout notice, and `migrate okf` refuses them.** The notice named the unread `.ai-rulez` tree, and `migrate okf` would have rewritten it (and made `index.md` files there) instead of the `.agents` content.
- **With `source = "agents"`, an output below a symlinked directory no longer writes into the source.** `.claude/rules -> ../.agents/rules` or `.claude/skills/foo -> ../../.agents/skills/foo` bypassed the filter that keeps `generate` off `.agents`; paths are now resolved through symlinks first and such outputs are dropped with a warning.
- **`AR981` names the drifted file in an `.agents` project.** The path was joined to the state directory (`.agents/.ai-rulez/.agents/rules/x.md`) instead of the project root.
- **With `source = "agents"`, `.github/copilot-instructions.md` no longer repeats the text of `AGENTS.md`.** The exclusion of tools that read `AGENTS.md` themselves was dropped for a root file several presets share, so Copilot still got a copy; the file is now negated directly when every configured preset writing it reads `AGENTS.md`. Gemini CLI is no longer treated as a reader, since `agents_md` is off and its settings load only `GEMINI.md`.
- **`generate --plugin` no longer writes into the `.agents` source.** With `source = "agents"` the Codex index `.agents/plugins/marketplace.json` (and any bundle placed under `.agents`) silently replaced a hand-written file; plugin outputs on `AGENTS.md` or under `.agents/` are now skipped with a warning.
- **`validate` no longer reports OKF findings twice when the `[okf]` bundle is the configuration directory under another spelling.** A relative path, a trailing separator or a symlink to it defeated the comparison.
- **A plugin sidecar generated on Windows records no file modes.** It recorded `0644` for every file (Windows has no executable bit), so `verify --plugin` on Linux reported `mode` drift for each script ([#403](https://github.com/Goldziher/ai-rulez/issues/403)).
- **`generate --check` judges the managed `.gitignore` block as a set and follows a symlinked `.gitignore`.** A block with a line `generate` would remove was reported in sync; it is now drift, as is a missing entry. With a symlinked `.gitignore` (git does not read it) `--check` reported nothing; it now compares the `ai-rulez` block in `.git/info/exclude`. Outside a git repository `--check` also counts machine-local outputs that `generate` puts in `.gitignore`.
- **`validate` rejects a marketplace `output_dir` that sits inside another's.** Only identical directories were refused, so `out` and `out/inner` (or an empty `output_dir` beside any other) wrote one marketplace's index and bundles into the other's tree.
- **`AR9T1` points at the plugin's own definition.** It anchored on the first `name = "<plugin>"` line of `config.toml`, which could be a marketplace or another table, and ignored `.ai-rulez/plugins/<name>.toml`; it now reports the plugin's table or its definition file.
- **A `plugins/` or `marketplaces/` directory that cannot be listed fails the load, and unknown keys in a definition file are errors.** Any `ReadDir` failure (a permission error, a file where the directory should be) was read as "no definitions" and the plugins vanished silently. Unknown keys in `.ai-rulez/plugins/*.toml` and `.ai-rulez/marketplaces/*.toml` already fail the load with the file and key named; they are not ignored like unknown keys in `config.toml` layers.
- **A leading `/` in a `gitignore_exclude` entry anchors it at the root.** `/AGENTS.md` also excluded `svc/AGENTS.md`; it now names only the root file, as in a `.gitignore`.
- **The server card namespace derived from `github.com/owner/name` is `io.github.owner`, and other hosts add the owner.** It was `com.github`, shared by every repository on GitHub, and `host/owner/name` dropped the owner.
- **A remote MCP server URL with a query, fragment or `${...}` placeholder is no longer written into the server card and ARD manifest.** `?api_key=...` or `?token=${TOKEN}` was published verbatim; the server is now skipped with a warning that does not repeat the URL.
- **A directory symlink in a skill no longer bundles the credentials behind it.** Files and subdirectories reached through a followed link (`references/ref -> docs/`, or a link to the project root) skipped the credential check a linked file gets, so `docs/.env`, a nested `id_rsa` or `.git/` was copied into the output of `generate` and `generate --plugin`; they are now skipped with a warning.
- **`AR001` now detects the AWS secret access key.** Only the access key id (`AKIA`/`ASIA`) was reported; a 40-character value assigned to a name containing `aws_secret_access_key` (any case, `=` or `:`, optionally quoted) is now a finding with the value masked, and is redacted before text goes to a model or a publish step ([#409](https://github.com/Goldziher/ai-rulez/issues/409)).
- **The AR021 documentation no longer claims `references/` is skipped.** A skill's `references/` files are scanned like `SKILL.md` (exempt them with `[lint] example_paths = ["**/references/**"]`); `examples/` and `templates/` are not scanned at all ([#410](https://github.com/Goldziher/ai-rulez/issues/410)).
- **The shared `.agents/skills` tree honours `delivery`.** With `agents_md` on, codex, cursor, gemini, copilot, opencode, zed and the other presets reading it wrote `served` skills with their `scripts/` and `assets/` and no `dynamic-skills` stub, so `validate --strict` reported `AR991` on its own output. The tree now leaves served skills out and carries the stub, as the claude tree does; if a preset reading it cannot call MCP, the tree keeps the served skills as files and adds the stub ([#402](https://github.com/Goldziher/ai-rulez/issues/402)).
- **`AR501` and `AR502` resolve `${CLAUDE_SKILL_DIR}` in skill frontmatter hooks.** A hook command reaching a bundled script through `${CLAUDE_SKILL_DIR}/...` or `$CLAUDE_SKILL_DIR/...` is checked against the skill's own directory for existence and, when run directly, the executable bit; a missing script passed `validate --strict` before ([#406](https://github.com/Goldziher/ai-rulez/issues/406)).
- **`builtins` accepts a positive `domain/name` to load a single item of a pack, and `builtin:domain/name` works in profiles.** A positive `domain/name` was read as a pack called `domain/name` and silently ignored; it now loads only that item (still applied after `!domain`, so `["security/secrets-handling", "!security"]` keeps just that item; `domain` wins over `domain/name`). A root `!domain/name` is now honored for packs loaded through `builtin:<pack>`. Behavior change: an unknown positive entry (`builtins = ["nope"]`, a typo in an item) now fails `validate` with the valid choices instead of being ignored; unknown exclusions remain warnings. `builtins show` labels its item names as `builtins` identifiers ([#399](https://github.com/Goldziher/ai-rulez/issues/399)).
- **`validate` runs every analyzer in an OKF-layout project, with the OKF findings next to theirs.** A configuration directory with a root `index.md` (what `init` scaffolds) used to stop at the `AR9B*` checks: the references (AR101, AR201), security (AR001, AR004, AR005), resource (AR402) and hook analyzers never ran, `--analyzer references` was ignored, and with `--strict` any OKF warning ended the run. The OKF findings now go through the lint run, so `[lint]` severities, `--fail-on`, the baseline and `--analyzer` (including `--analyzer okf`) apply to them, and `--recursive` reports them too. Exit codes change: a project that passed only because the other analyzers were skipped now exits 2 on their findings, and an OKF error no longer hides them. A strict run that exits 2 no longer prints "Configuration is valid" ([#396](https://github.com/Goldziher/ai-rulez/issues/396), [#408](https://github.com/Goldziher/ai-rulez/issues/408)).
- **`generate --plugin` quotes the path of a `script` hook without `args`.** It wrote `${CLAUDE_PLUGIN_ROOT}/hooks/<name>` bare, which `validate --strict` flags as AR963; the Claude, Cursor and Gemini hook files now carry `"<root>/hooks/<name>"`. A hook with `args` is exec form and is unchanged ([#404](https://github.com/Goldziher/ai-rulez/issues/404)).
- **`generate --plugin` follows an in-project symlink inside a skill directory.** It dropped such a link without a word while plain `generate` copied the target, so the bundle shipped without the file and `verify --plugin` passed. Both modes now apply one policy: a link to a file or directory inside the project is followed, any other is skipped with a warning naming it ([#407](https://github.com/Goldziher/ai-rulez/issues/407)).
- **`verify --plugin` detects a lost executable bit.** `.ai-rulez-generated.json` now records each output's mode and a bundled hook or skill script that is no longer executable is reported as drift of kind `mode` (also in the `--format json` `drift` array and `schema/verify-plugin.schema.json`). A sidecar from an earlier release, which has no modes, still verifies until the next `generate --plugin` ([#403](https://github.com/Goldziher/ai-rulez/issues/403)).
- **`generate` no longer inserts a blank line under `# END ai-rulez`, and a `!dir/` negation is honored like a `!file` one.** The lines after the managed block are kept byte for byte, and `!.claude/skills/` now drops `.claude/skills/` from the block as `!CLAUDE.md` drops `CLAUDE.md`; `doctor` no longer reports an output you re-included on purpose, nor suggests a `generate` that would change nothing ([#398](https://github.com/Goldziher/ai-rulez/issues/398)).
- **`generate --check` verifies the managed `.gitignore` block.** A missing block, or one lacking an entry `generate` would add, is reported as `missing:` / `stale: .gitignore (managed block)` (`note: "managed block"` in `--format json`) and exits 2; a recursive check judges the shared toplevel block once after the last root ([#400](https://github.com/Goldziher/ai-rulez/issues/400)).
- **The machine-local manifest is gitignored whenever it exists, with `gitignore = false` too.** `generate` ignored `.ai-rulez/.generated-manifest.local.json` only for machine-local outputs or `gitignore = true`, so `git add -A` staged a digests-only manifest and every later run warned that git tracks it; the entry no longer pulls in `.ai-rulez/local/` or the overlay patterns ([#397](https://github.com/Goldziher/ai-rulez/issues/397)).
- **Errors for a committed config layer name the layer file.** They said "local config" and "the local overlay"; they now read `unknown key(s) in layer config.lint.toml: ...` and point at the layer files ([#401](https://github.com/Goldziher/ai-rulez/issues/401)).
- **`validate --recursive` and `generate --check --recursive` are faster on large monorepos.** The roots are now checked concurrently (at most `GOMAXPROCS`) and reported in path order, so output and exit codes are unchanged; the roots of a repository share one listing of the git index instead of reading it once per root to ask whether the config is tracked; each root prints a `[n/N] path (time)` progress line on stderr (not under `--quiet` or `--format json`); `doctor --debug` logs the duration of each check. This improves the time but the issue stays open for the reporter's profile ([#376](https://github.com/Goldziher/ai-rulez/issues/376)).

## [5.1.1] - 2026-10-13

### Fixed

- **`generate --dry-run` no longer lists `create-dir` entries.** A directory marker creates nothing since #379, so the plan listed directories a real run never made ([#394](https://github.com/Goldziher/ai-rulez/issues/394)).
- **A baz rule whose glob points inside a nested ai-rulez root stays in the root `AGENTS.md`.** `generate` wrote `svc/web/AGENTS.md` for it while the nested root's own run reported it as an orphan, so `generate --check --recursive` failed on every run; the rule is now inlined into the root file in every mode and `generate` warns naming it ([#395](https://github.com/Goldziher/ai-rulez/issues/395)).

## [5.1.0] - 2026-10-12

### Added

- **`[plugin.pi] package_name` and `skills_entry = "directory"` for the Pi runtime.** `package_name` sets the npm name instead of `@<owner>/pi-<plugin-name>`, and under `package_json = "merge"` an authored `name` is now kept when it is unset; `skills_entry = "directory"` writes `"./skills"` to `pi.skills` and `"skills/"` to `files` instead of one entry per skill. `files` lists `README.md` and `assets/` only when they exist in the bundle ([#383](https://github.com/Goldziher/ai-rulez/issues/383)).
- **`verify --plugin` names the outputs that drifted.** Text output lists each path with its kind (`changed`, `missing`, `stale-source`, `obsolete`) and `--format json` carries a `drift` array of `{path, kind}`; the existing fields are unchanged ([#381](https://github.com/Goldziher/ai-rulez/issues/381)).
- **`[plugin] skills_dir_owned = true` declares the bundle's skills directory fully owned.** `verify --plugin` then reports a skill directory or file it did not generate (a ghost left by a rename, a stray file directly under `skills/`) as drift of kind `extra`, and `generate --plugin` removes it; the default stays off, so an unrecorded skill directory is never touched ([#387](https://github.com/Goldziher/ai-rulez/issues/387)).

### Fixed

- **`generate --plugin` removes the directories it leaves empty when it deletes unrecorded files.** `skills/<skill>/old/deep/` is removed bottom-up; a directory that still holds anything stays ([#387](https://github.com/Goldziher/ai-rulez/issues/387)).
- **`generate` no longer leaves empty `.claude/agents`, `.claude/skills` and `.claude/rules` directories in roots without such content.** Output directories are now created only when a file is written into them, and empty ones an earlier run created are removed; a directory holding any file is never touched; `generate --plan` no longer lists those directories as `mkdir` entries ([#379](https://github.com/Goldziher/ai-rulez/issues/379)).
- **`AR990` no longer flags a single code span `load_skill <name>`.** That is the form its own message recommends; a bare name in backticks, a source path or another call form on the same line is still reported ([#377](https://github.com/Goldziher/ai-rulez/issues/377)).
- **`AR9T0` looks for component paths only under the plugin's own root.** A `.mcp.json` in a parent directory of a nested plugin was reported as if the plugin shipped it ([#378](https://github.com/Goldziher/ai-rulez/issues/378)).
- **`AR9T0` also covers a root `plugin.json` that ai-rulez does not write.** Codex prefers it as the manifest, so a stray one under the default `legacy` layout is reported; a root `CLAUDE.md` is not on the list because Claude Code does not load one from a plugin root ([#384](https://github.com/Goldziher/ai-rulez/issues/384)).
- **`verify --plugin` reports unrecorded files and symlinks in a bundle's skills tree, and `generate --plugin` removes the files.** A file inside a generated skill directory that `.ai-rulez-generated.json` does not list is drift of kind `extra`, and a recorded output that is a symlink is kind `symlink`; `generate --plugin` deletes the unrecorded files (`--dry-run` shows `delete-stale:`) and never touches a skill directory it does not generate or your sources ([#380](https://github.com/Goldziher/ai-rulez/issues/380)).
- **`generate --plugin` no longer writes through a symlinked output, and `verify --plugin` reports a symlinked directory.** A generated file that is a symlink, or has a symlinked parent directory, now fails generation (and `--dry-run`) with an error naming the link, and the link target is left unchanged; `verify --plugin` reports the link as drift of kind `symlink` ([#386](https://github.com/Goldziher/ai-rulez/issues/386)).
- **`generate --plugin`, `verify --plugin` and `validate` no longer warn that a skill's `agents/` directory is unrecognized.** `agents/openai.yaml` (Codex display metadata) is bundled by the plugin passthrough, so the warning said the opposite of what happens, and it repeated for every skill ([#382](https://github.com/Goldziher/ai-rulez/issues/382)).

## [5.0.5] - 2026-10-12

### Added

- **`[rules] baz_rest = "skip"` keeps rules baz does not select out of the root `AGENTS.md`.** Under `baz_default = "none"` the rules that do not opt in, and rules excluded by `!baz`, were still inlined into the root file (127 KB in one project); `skip` drops those path-scoped items when `baz` is the only preset writing that file. The default `"root"` is unchanged ([#371](https://github.com/Goldziher/ai-rulez/issues/371)).
- **`[mcp] self_server_launcher = "binary-then-npx"` starts an installed `ai-rulez` and falls back to npx.** The generated `.mcp.json` entry is a `sh -c` script with `--serve-skills` passed as an argument, so a project needs no hand-written shell wrapper; `self_server_version` pins the npx fallback and it excludes `self_server_command` ([#372](https://github.com/Goldziher/ai-rulez/issues/372)).

- **Layered configuration: `config.<name>.toml` files merged over `config.toml`.** `AI_RULEZ_LAYERS=ai,custom` (or `AI_RULEZ_PROFILE`, or `config.WithLayers`) activates layers in order, a later layer winning; tables merge per key, named lists such as `mcp_servers` and `installed_skills` merge by name, and `config.local.toml` still applies last. `ai-rulez config layers` lists the layers and which are active; a layered view is never written back as `config.toml` ([#370](https://github.com/Goldziher/ai-rulez/issues/370)).

### Fixed

- **Loading a project with many domains runs one `git ls-files` for the configuration directory, not one per domain's skills and commands directory.** Each listing took about a second in a repository with an 89,000-file index, so a 28-domain project spent half a minute in the content scan; the per-directory listing remains the fallback when the configuration directory is not covered (a nested repository, no git) ([#376](https://github.com/Goldziher/ai-rulez/issues/376)).
- **`generate --check` no longer opens every file of the work tree to look for orphaned outputs.** The walk for the generated files of a removed nested root read a header from each file below the project root, which took over a minute on an 89,000-file monorepo with no output; it now reads only files a preset can write (a root file name such as `CLAUDE.md`, or a file below an output directory such as `.claude/`), and `--debug` logs how many files it visited and how long it took ([#376](https://github.com/Goldziher/ai-rulez/issues/376)).
- **`validate` and `generate` warn when `[profiles]` defines no `default`.** A root config with `[profiles]` but no `default` profile or `default =` setting generates the built-in default, which sees only root content, global builtins and included domains, so every other domain's root outputs were removed with no message. The warning names the domains and suggests `default = ["*"]` ([#374](https://github.com/Goldziher/ai-rulez/issues/374)).
- **`generate` says where a baz rule went when its glob points into `.ai-rulez/` or `.git/`.** No nested `AGENTS.md` is written there (it would be read back as a source file), so the rule was inlined into the root file or, under `baz_unscoped = "skip"`, dropped, without a word; both cases now warn and name the rule ([#375](https://github.com/Goldziher/ai-rulez/issues/375)).
- **`generate` warns about an MCP server entry left behind after its `[[mcp_servers]]` block is removed.** An entry is taken out of `.mcp.json` only on the machine that recorded writing it (the gitignored local manifest), so a fresh clone kept it silently. The committed manifest now lists, as a hint only, the server entries written from the shared configuration, and `generate` warns once per leftover entry and names the file; it never removes an entry on that evidence ([#373](https://github.com/Goldziher/ai-rulez/issues/373)).
- **`--serve-skills` is no longer appended to a `self_server_command` that is a `sh -c` script.** The flag landed in `$0` of the script; such a command is now left as written, and `docs/configuration.md` documents the `"$@"` plus trailing `"sh"` pattern ([#372](https://github.com/Goldziher/ai-rulez/issues/372)).

## [5.0.4] - 2026-10-12

### Added

- **`AR990` flags path-style and bare pointers to served skills.** A static file that names a served skill by source path (`.ai-rulez/domains/<d>/skills/<name>/SKILL.md`, `.../skills/<name>/references/x.md`) or by a hyphenated bare name in prose (`see skill dev-cycle`) is now reported, and the diagnostic suggests `load_skill <name>`; a non-hyphenated word or another tree's `skills/` directory stays clean for `AR301` ([#363](https://github.com/Goldziher/ai-rulez/issues/363)).
- **`[delivery.stub]` customises the generated `dynamic-skills` stub.** `description` replaces the description every session lists, and `body` or `body_file` (relative to the config directory) replaces the text, which may use `{{server}}` and must still name `find_skill` and `load_skill`; `validate` rejects a blank description, both body forms, unknown variables and a missing or escaping file ([#362](https://github.com/Goldziher/ai-rulez/issues/362)).
- **Instruction files tell agents how to load served skills.** While skills are served, `AGENTS.md` and each MCP-capable harness's root file get a short "Unlisted skills" section: call `find_skill`, then `load_skill`, on the serving MCP server, with the `.ai-rulez/skills/<name>/SKILL.md` fallback. `[mcp] skills_instructions = false` leaves it out and `skills_instructions_text` replaces it (`{{server}}`, `{{skills_dir}}`) ([#359](https://github.com/Goldziher/ai-rulez/issues/359)).
- **`[header] nested` shortens the header of a root inside another root.** The default `compact` writes one line, `Generated by ai-rulez from <path>; do not edit`, in place of the outer root's 10 to 50 line banner, `omit` writes none and `inherit` keeps the full header; the `Content-Hash` line is kept, so `generate --check` stays valid. A custom `[header] text` is kept unless `nested` is set explicitly ([#364](https://github.com/Goldziher/ai-rulez/issues/364)).
- **`[header] mcp_line = false` drops the MCP server line from generated headers.** Without it the line is derived from `[mcp]` ([#360](https://github.com/Goldziher/ai-rulez/issues/360)).
- **`targets` accepts negations, and the `baz` preset has a per-rule opt-in.** `targets: ["!baz"]` excludes a rule from one preset (a negation always wins; a rule with only negations still goes everywhere else), `[rules.baz] include = [...]` and `baz: true` frontmatter opt rules in under `baz_default = "none"`, and the directory-target workaround is documented ([#366](https://github.com/Goldziher/ai-rulez/issues/366)).
- **`[rules] baz_unscoped = "skip"` keeps directory-less path-scoped rules out of the root `AGENTS.md`.** A rule scoped by `**/*.go` or `**/BUILD` cannot be nested for Baz, so it was inlined into the root file that Claude also imports; `skip` drops it when `baz` is the only preset writing that file (a file shared with `codex` and the others keeps it). The default `"root"` is unchanged, and the `baz_budget` warning now names the inlined rules ([#365](https://github.com/Goldziher/ai-rulez/issues/365)).
- **`[rules] baz_dedupe_max = K` collapses a rule copied into more than K directories.** The rule is written once to the directory covering all its globs, up to the root `AGENTS.md` when they share only the repository root (never with `baz_unscoped = "skip"`), instead of one copy per directory ([#367](https://github.com/Goldziher/ai-rulez/issues/367)).

### Fixed

- **A non-recursive `generate` no longer deletes the outputs of a nested root.** The orphan walk of a top-level output directory (for example `packages/` from a scope) descended into a nested root below it and removed that root's committed `CLAUDE.md`/`.claude/rules` files; it now stops at any directory holding its own config, as the stray-tree walk already did ([#368](https://github.com/Goldziher/ai-rulez/issues/368)).
- **The header's MCP line names the server the project launches.** It was a hardcoded `npx -y ai-rulez@latest mcp`; it now follows `self_server_command`, `self_server_version` and `--serve-skills`, and is left out when `[mcp] self_server = false` ([#360](https://github.com/Goldziher/ai-rulez/issues/360)).
- **A nested root's generated header names its own config.** The `Source:` line and the directory hints of a root below the repository root read `products/em/webapps/.ai-rulez/config.toml`, its path from the git work tree root, instead of the repository-root `.ai-rulez/config.toml`; `generate`, `generate --recursive` and `generate --check` agree on it ([#361](https://github.com/Goldziher/ai-rulez/issues/361)).
- **`docs/baz.md` no longer claims Claude Code loads a nested `AGENTS.md` whenever its directory has no `CLAUDE.md`.** Claude Code reads `AGENTS.md` natively only when no `CLAUDE.md` exists in the working directory or above it (v2.1.277 or later), so with the `claude` preset's root `CLAUDE.md` shim a baz-only nested file is not loaded; the page now states the three cases and what the documentation does not settle ([#369](https://github.com/Goldziher/ai-rulez/issues/369)).

## [5.0.3] - 2026-10-12

### Added

- **One MCP server serves the skills and the authoring tools.** `ai-rulez mcp --serve-skills` now registers the config tools alongside `find_skill`/`load_skill` and the `skill://` resources, in one process under one name (`ai-rulez`), so a project no longer runs `ai-rulez` and `ai-rulez-skills` side by side; `--read-only` registers only the read-only tools, the generated `.mcp.json` carries a single `ai-rulez` entry running `mcp --serve-skills`, and `AR993` counts that entry ([#358](https://github.com/Goldziher/ai-rulez/issues/358)).
- **`ai-rulez mcp --serve-skills --daemon` shares one long-lived server per repository.** The session becomes a stdio proxy: it connects to a per-repo daemon (a `0600` socket in a `0700` directory under the user cache, keyed by repository toplevel, ai-rulez version and the serving flags), spawns it on first use, and serves in-process on any failure. The daemon exits after 15 minutes with no client, removes its socket, and reloads the catalog on change; the mode is not offered on Windows ([#321](https://github.com/Goldziher/ai-rulez/issues/321)).
- **`[plugin] shared_resources` copies shared directories into every bundled skill.** Each listed directory (e.g. `references/`) is written under every skill for every runtime and recorded in the provenance sidecar, a relative `../../references/x.md` link from `SKILL.md` is rewritten to the copy, and a name clash with a skill's own file is an error rather than a silent overwrite ([#353](https://github.com/Goldziher/ai-rulez/issues/353)).
- **`[plugin] provenance_headers = false` writes bundled files without the generated header.** Markdown/JS/TS/PY outputs are byte-identical to their sources while `.ai-rulez-generated.json` still records every output, so `verify --plugin`, obsolete pruning and `publish --emit agent-plugins` are unchanged and a header-less file ai-rulez did not record stays hand-written ([#354](https://github.com/Goldziher/ai-rulez/issues/354)).
- **`[plugin] version_from` reads the plugin version from an existing manifest.** `version_from = "package.json"` (also `pyproject.toml` and `Cargo.toml`) resolves `project.version`/`package.version` for the manifest, the lock and `verify --plugin`, so a bump by release-please or changesets shows as drift; it is mutually exclusive with `version`, and `validate` reports a missing file, a missing key or both keys set ([#355](https://github.com/Goldziher/ai-rulez/issues/355)).
- **The Pi runtime can reuse the shared skills tree and keep its own `package.json`.** `[plugin.pi] skills_dir` writes each skill once into that tree and points `pi.skills` at it instead of a second copy under `.pi/skills/`, and `package_json = "merge"` updates only the keys ai-rulez owns, keeping `scripts`, `devDependencies` and the rest ([#356](https://github.com/Goldziher/ai-rulez/issues/356)).
- **`AR9T0 root-component-shipped` warns when a single-plugin root holds paths hosts load by default.** A plugin whose bundle root is the repository root is reported for each default component it ships there (`.mcp.json`, `hooks/`, `agents/`, `commands/`, `settings.json`, ...), naming the host that loads it, with `content_root` or a monorepo member as the fix ([#357](https://github.com/Goldziher/ai-rulez/issues/357)).
- **A plugin can set its package manager and its OpenCode dependency.** `[plugin] package_manager` emits `packageManager` in the generated OpenCode/Pi root `package.json`, and `[plugin] opencode_dependency` overrides the `@opencode/plugin` constraint (default bumped to `^2.0.26`). The JSON serializer no longer escapes `&` as `\u0026` ([#350](https://github.com/Goldziher/ai-rulez/issues/350)).

### Fixed

- **The manifest-free orphan walk leaves a committed plugin bundle alone.** `generate --plugin` writes its bundle into the project tree with the same generated header the generator writes, so `generate --check` reported every bundle file as an orphan and `generate` deleted them; the walk now honours the bundle's `.ai-rulez-generated.json` provenance sidecar ([#350](https://github.com/Goldziher/ai-rulez/issues/350)).
- **A hosted MCP server's `oauth` block reaches `.mcp.json` for the `copilot` and `cursor` presets.** Only the `mcp` sidecar wrote it, so `generate` failed with "presets write different content to the same path" whenever `copilot` or `cursor` was active ([#334](https://github.com/Goldziher/ai-rulez/issues/334)).
- **A top-level key written after a `[[table]]` array header is a load error.** The swallowed-key check only descended `[table]` maps, so a key after `[[mcp_servers]]` was silently read as a key of the last element and ignored ([#344](https://github.com/Goldziher/ai-rulez/issues/344)).
- **`generate --plugin` rewrites an existing generated header instead of stacking another.** A plugin whose `content_root` overlaps its output tree (`content_root = "."`) reads its own decorated output as the source, so each run added a header block, `verify --plugin` never converged and `publish --emit agent-plugins` failed at the `AR9N0` preflight ([#351](https://github.com/Goldziher/ai-rulez/issues/351)).
- **`AR301` no longer reads prose as a skill pointer.** A backticked term before the noun "skill" ("a `both` skill") and a `skills/` path segment that is not this project's tree (`src/mastra/public/skills/shared/<name>/SKILL.md`) were reported as unknown references; the matcher now requires a pointer form, so the real ones (`reference: skill x`, `skills/x`, `.claude/skills/x/SKILL.md`) still fire ([#352](https://github.com/Goldziher/ai-rulez/issues/352)).

## [5.0.2] - 2026-10-11

### Added

- **Profiles select domains with `*` and `!name`.** A profile entry may be a glob (`*`, `backend-*`) or a negation (`!legacy`) as well as an exact domain name; a profile that is only negations is an error, and `profile add` accepts the same spellings ([#324](https://github.com/Goldziher/ai-rulez/issues/324)).
- **`[claude] claude_md = "none"` writes `AGENTS.md` without the `CLAUDE.md` import shim**, for projects that want Claude Code to read `AGENTS.md` alone. The default stays `"shim"` ([#317](https://github.com/Goldziher/ai-rulez/issues/317)).
- **`AR9Q0 claude-md-shadows-agents-md`** reports a hand-written `CLAUDE.md` that shadows the generated `AGENTS.md`; opt in with `[lint] agents_md_only` (allow list: `agents_md_only_allow`) ([#318](https://github.com/Goldziher/ai-rulez/issues/318)).
- **`AR9R0 served-skill-bundled-files` and `AR9R1 both-dmi-pointer`** warn when a `served` skill ships `scripts/` that are never written to disk, and when static content points at a `delivery: both` skill whose frontmatter disables model invocation (the Skill tool refuses it). The served-skill docs recommend `delivery: both` when a skill runs bundled scripts ([#336](https://github.com/Goldziher/ai-rulez/issues/336), [#337](https://github.com/Goldziher/ai-rulez/issues/337)).
- **`doctor`, and `validate --strict --mcp-user-scope`, report MCP servers shadowed in a harness user scope** with a different endpoint, naming both scopes and the removal command, and generated servers that differ only by name while pointing at the same url ([#330](https://github.com/Goldziher/ai-rulez/issues/330)).
- **`[[mcp_servers]]` expresses an explicit `type` and a pre-registered OAuth client.** A hosted server takes `oauth = { client_id, callback_port, scopes }`, rendered in `.mcp.json` for Claude and VS Code and as a TOML `oauth` table for Codex, and skipped with a warning by a harness that cannot express it. `AR602` warns when a hosted server names no `client_id`, or when one name resolves to two endpoints across sources ([#334](https://github.com/Goldziher/ai-rulez/issues/334)).
- **`tokens --format json` carries a stable `always_loaded` key**, and its diagnostics (including the over-budget line) go to stderr so stdout stays one parseable document ([#340](https://github.com/Goldziher/ai-rulez/issues/340)).
- **The `baz` preset's nested surface is controllable.** `[rules] baz_default = "none"` nests only items whose `targets` select `baz`, `baz_dedupe = true` writes one shared parent `AGENTS.md` when several globs share a parent, and `baz_budget` warns above a byte budget. Every default is unchanged ([#341](https://github.com/Goldziher/ai-rulez/issues/341)).
- **`[plugin] include` bundles the files a plugin's MCP command runs.** Globs relative to the content root are copied verbatim into the Agent Plugins package and recorded in `.ai-rulez-generated.json` with content hashes, so `verify --plugin` and `publish --emit agent-plugins` count them and `AR9O2` stops firing once a `./` command resolves to an included file ([#349](https://github.com/Goldziher/ai-rulez/issues/349)).

### Performance

- **The rendered serve catalog is cached on disk.** `ai-rulez mcp --serve-skills` re-rendered every served skill on each start; the rendered catalog is now cached (HMAC-authenticated, re-admitted on restore, and guarded against symlinks and oversized files), so a warm start is about 0.2 s ([#320](https://github.com/Goldziher/ai-rulez/issues/320)).
- **The per-file security scan runs once across a worker pool.** `validate` and the `generate` secret gate scanned every file serially and re-read the same file once per caller; the scan now runs once per file across `GOMAXPROCS` workers, each file is read once per run, and content reached by more than one item is scanned once. Findings, their order and every exit code are unchanged. On the lint benchmark the full run drops from 110 ms to 45 ms and the security analyzer from 71 ms to 15 ms ([#312](https://github.com/Goldziher/ai-rulez/issues/312)).
- **`generate` no longer runs the whole security family before writing.** Its secret gate read only AR001 but ran every security rule (shell, injection, hidden characters, hosts, blobs) over every file; it now runs the credential patterns alone, with the same AR001 findings. On a 590-file project, `generate` CPU (user) fell from 1.4 s to 0.65 s. `validate` builds the split text of each file once per run instead of up to three times and skips body-scan patterns on lines that cannot match (user CPU 1.9 s to 1.6 s). Output is unchanged.

### Fixed

- **`generate --check` reports an orphaned generated file without the manifest.** A generated file whose source was removed was not reported when the gitignored `.generated-manifest.json` was absent (CI, a fresh checkout), because the orphan pass relied on it. `--check` now also reports any file carrying a Content-Hash header that no output emits ([#313](https://github.com/Goldziher/ai-rulez/issues/313)).
- **Progress frames are no longer written to a non-terminal stream.** With stdout/stderr redirected, `generate`, `validate` and `--check` wrote spinner and bar frames with ANSI codes and carriage returns into the output; the animated UI now draws only on a terminal, and `NO_COLOR` also suppresses it ([#314](https://github.com/Goldziher/ai-rulez/issues/314)).
- **`.mcp.json` entries have one shape.** A configured server was written with `"disabled": false`, a key Claude Code does not document for an active server, while the self-server entry had `"type": "stdio"`; `disabled` is now written only for a server that is turned off, and every stdio entry carries the same `type: stdio` ([#315](https://github.com/Goldziher/ai-rulez/issues/315)).
- **A skill, rule or agent file that cannot be loaded is reported.** A `SKILL.md` over the 8 MiB content limit (or any other read failure) was dropped from the output with no message from `generate` or `validate`; the scan now logs `failed to load skill file|content file|agent file` with the path, as it already did for commands.
- **AR002 accepts the emoji presentation selector after U+2139, U+203C, U+2049, U+3030 and U+303D** (`ℹ️`, `‼️`, `⁉️`, `〰️`, `〽️`). They are letter or punctuation bases, which the rule rejected as a possible hidden payload.
- **`migrate v5` pins `[mcp] self_server = false`**, so a migrated project no longer gains the ai-rulez MCP server in `.mcp.json` (once per nested root in a monorepo) until it opts in.
- **One managed `.gitignore` block for a recursive run.** A recursive `generate` wrote the block into every nested root; it is now written once at the git toplevel, and an explicit `gitignore = false` is warned about rather than overridden ([#316](https://github.com/Goldziher/ai-rulez/issues/316)).
- **A plugin-only root no longer writes a header-only instruction file.** A nested root that declares only `[plugin]`/`[[plugin.mcp]]` but keeps a preset wrote a `CLAUDE.md`/`AGENTS.md` holding nothing but the generated header; the root file and the shared `AGENTS.md` are now skipped with a warning naming the file and the preset ([#327](https://github.com/Goldziher/ai-rulez/issues/327)).
- **`installed_skills` and `skill_sources` honor `include`/`exclude`.** An entry that selects skills by glob still resolved its root as a single skill directory, so `installed_skills` errored with `expected SKILL.md at skills/<name>` and `skill_sources` served nothing; when `include`/`exclude` is set and no path is given the entry is now a directory of skill directories, and a glob that matches nothing is reported ([#329](https://github.com/Goldziher/ai-rulez/issues/329), [#331](https://github.com/Goldziher/ai-rulez/issues/331)).
- **`AR301` and `AR990` share one reference matcher.** `AR301` treated a backticked term followed by the plural noun "skills" as a pointer, so prose failed validation; `AR990` only saw backticked names, so the documented non-backticked pointer forms (`reference: skill x`, `skills/x`, `.claude/skills/x/SKILL.md`, `subagent_type: x`) went unreported. One matcher now backs both ([#335](https://github.com/Goldziher/ai-rulez/issues/335), [#339](https://github.com/Goldziher/ai-rulez/issues/339)).
- **`AR990`, `AR992` and `AR402` no longer misfire.** `AR992` reported a preset that writes no skills tree (claude+baz) on every generate; `AR990` saw a bare backticked served-skill name but not a `load_skill` pointer in the same file; and `AR402` rejected a `references/` path that names a sibling skill's file. Each is now consistent with its own rule ([#342](https://github.com/Goldziher/ai-rulez/issues/342), [#343](https://github.com/Goldziher/ai-rulez/issues/343), [#346](https://github.com/Goldziher/ai-rulez/issues/346)).
- **`AR990` fires only for always-loaded content.** A served skill naming a served sibling is not reported ([#319](https://github.com/Goldziher/ai-rulez/issues/319)).
- **`AR201`/`AR401` resolve names from a scope's emitted directory**, so a container image tag in a scope is no longer a false positive ([#325](https://github.com/Goldziher/ai-rulez/issues/325)).
- **`validate --recursive` reports findings in its summary and counts the same roots as `generate --recursive`.** It printed "All N config(s) are valid" while exiting 2, and counted a different root set ([#338](https://github.com/Goldziher/ai-rulez/issues/338)).
- **`validate` names the nested roots a non-recursive run skips** ([#326](https://github.com/Goldziher/ai-rulez/issues/326)).
- **Plugin `${PLUGIN_ROOT}` is rewritten in a plugin's `args` and `env`, not only its `command`** ([#322](https://github.com/Goldziher/ai-rulez/issues/322)).
- **`generate` overwrites a committed output in a fresh worktree without `--force`.** With no `.generated-manifest.json`, a bundled skill asset ai-rulez had written was refused as a file "ai-rulez did not write"; a raw output below a directory this run renders a skill file into is now treated as ai-rulez's own copy, while a hand-written file and a symlink stay refused ([#332](https://github.com/Goldziher/ai-rulez/issues/332)).
- **Stale generated files are pruned and reported without a manifest.** A removed agent's `.claude/agents/<name>.md` stayed on disk and `--check` exited 0; the retired-file set now includes every header-carrying orphan the render no longer emits, shared by `generate` and `--check` ([#333](https://github.com/Goldziher/ai-rulez/issues/333)).
- **Bundled skill assets are tracked in the manifest and reported as orphans.** A skill's `scripts/` copy carries no header, so a committed manifest now prunes one whose source is gone and `--check` reports it; with no manifest a header-less asset is left alone ([#328](https://github.com/Goldziher/ai-rulez/issues/328)).
- **`tokens` counts the `AGENTS.md` a canonical `CLAUDE.md` imports.** With `agents_md = true` the always-loaded total counted the import shim but not the `AGENTS.md` it loads, understating a budget gate; a runtime whose preset imports `AGENTS.md` now counts that file once ([#347](https://github.com/Goldziher/ai-rulez/issues/347)).
- **A top-level key written after a `[table]` header is a load error, not a silent no-op.** TOML read `agents_md`, `builtins` or `gitignore` after `[rules]` as keys of that table and ignored them, so the settings stopped applying with only a warning; loading now names the key, the table and the fix ([#344](https://github.com/Goldziher/ai-rulez/issues/344)).
- **A generated file left by a removed nested root is reported and pruned.** `generate --check --recursive` said up to date when a whole nested root was deleted and its `CLAUDE.md`/`.claude/rules/*` remained; the manifest-free orphan pass now walks outside the current output roots, so `--check` reports it and `generate` removes it ([#345](https://github.com/Goldziher/ai-rulez/issues/345)).
- **A key unknown to a nested table is no longer reported as a misplaced top-level key.** `description` in a `[[plugin.mcp]]` entry was told to "move it above the first [table]", which would change its meaning; the hint now applies only where TOML really can have swallowed a top-level key ([#348](https://github.com/Goldziher/ai-rulez/issues/348)).

### Documentation

- **Corrected the Claude Code rules-folder nesting claim** ([#323](https://github.com/Goldziher/ai-rulez/issues/323)).

## [5.0.1] - 2026-10-11

### Added

- **Compose roles.** `--role` takes a comma-separated list, as do `roles show`, `roles resolve`, `tokens --role` and `mcp --serve-skills --serve-role`. A composition is the union of its members: the domains and items they keep are combined, and where two members set `skill_mode` or `delivery` for the same skill the more specific key wins and, on a tie, the role listed last. A composition is pinnable — `ai-rulez lock --role a,b` records the union under the canonical name `a,b`. See [Roles](https://goldziher.github.io/ai-rulez/roles/).
- **`AI_RULEZ_PROFILE` and `AI_RULEZ_ROLE` select the active profile and role** when the matching flag is not given; an explicit flag always wins ([#309](https://github.com/Goldziher/ai-rulez/issues/309)).
- **A `domain` field on `[[mcp_servers]]`, `[[installed_skills]]` and `[[skill_sources]]`** ties a remote or fetched resource to a domain, so a profile or a role selects it by domain exactly as it selects local content. `profiles` keeps working and is an additional filter ([#308](https://github.com/Goldziher/ai-rulez/issues/308)).
- **`installed_skills` gains the `skill_sources` conveniences**: `include` and `exclude` select several skills from one repository with a single entry, `name_prefix` namespaces their names, and `trust` sets the repository-scoped scan level ([#310](https://github.com/Goldziher/ai-rulez/issues/310)).

### Fixed

- **A kept obsolete plugin file is no longer forgotten after regeneration.** An obsolete generated file you edited by hand is preserved with a warning, and `verify --plugin` now keeps failing while it exists, as documented, instead of passing once the provenance sidecar is rewritten ([#307](https://github.com/Goldziher/ai-rulez/issues/307)).

### Documentation

- Answer how [profiles and roles](https://goldziher.github.io/ai-rulez/roles/#profiles-vs-roles) differ, how [installed skills and skill sources](https://goldziher.github.io/ai-rulez/installed-skills/#installed-skills-vs-skill-sources) differ, and how to [compose roles](https://goldziher.github.io/ai-rulez/roles/#composing-roles).

## [5.0.0] - 2026-10-10

Run `ai-rulez migrate v5` (`--dry-run` to preview, `--check` for CI); upgrade steps are in [Migrating to v5](https://goldziher.github.io/ai-rulez/migration-v5/).

### Positioning

v5 repositions ai-rulez as a standards-compliant lifecycle tool for agent knowledge and capabilities: author in one source of truth, generate for every harness, bundle and publish to the open formats, lint and validate against each standard's own schema, and govern through signing, approvals and policy. The README, the documentation site (organized as Author, Generate, Bundle and publish, Validate, Govern, Operate) and the package metadata follow that story. "Planned" means the code does not yet generate and validate the standard. Supported today: OKF (Open Knowledge Format v0.2), Agent Plugins, ARD, Agent Skills, AGENTS.md, llms.txt, MCP server config, CycloneDX/SPDX SBOM, in-toto/DSSE/Sigstore and OpenTelemetry (OTLP); the MCP server card and "agent bundle" are planned. Pinned spec versions and conformance status are in [Standards](https://goldziher.github.io/ai-rulez/standards/).

### Breaking

- **Config format is v5.** A `version = "5.0"` project is required; a 4.x config is rejected with `run ai-rulez migrate v5` and a 2.x/3.x config tells you to install ai-rulez 4.x first. YAML/JSON configs (`config.yaml`, `config.json`, local variants) and separate `mcp.toml`/`mcp.yaml`/`mcp.json` files are no longer read — `migrate v5` converts them to `config.toml` and `[[mcp_servers]]`. `migrate v4` and `init --format yaml|json` are removed, and `migrate` now has real subcommands (`migrate v5`, `migrate okf`; the `migrate 5` / `migrate v5.0` spellings are gone). See [Migrating to v5](https://goldziher.github.io/ai-rulez/migration-v5/).
- **Go module is `github.com/Goldziher/ai-rulez/v5`**: install with `go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest`. npm, PyPI and Homebrew are unaffected.
- **One flag taxonomy.** The only shorthands are `-C -D -T -q -y -n -o`; `-n` is `--dry-run` everywhere and `--config-dir` has no shorthand (declared once, globally). `--out` is `--output` (a file) or `--output-dir` (a directory); `generate --strict` is `--strict-config`, `lock --strict` is `--refuse-findings`, `sign --policy` is `--org-policy`; `--json`/`-j` is `--format json`; `generate --no-fetch` is `--offline`; the confirmation skip of `clean` and the `remove` commands is `--yes`/`-y`. No command takes a config path argument any more: use `ai-rulez -C <path> validate`. A removed spelling fails with a hint naming its replacement.
- **Removed aliases and flags**: `g`, `clear`, `v`, `check`, the hidden `mcp --transport`/`--address`/`--port`, `--verbose`/`-V`, and the dead `generate --update-gitignore`, `--no-configure-cli-mcp`/`-M` and `--skip-cli-mcp`/`-S`. Unprefixed `DEBUG`, `QUIET` and `VERBOSE` no longer change the CLI; use `AI_RULEZ_DEBUG`/`AI_RULEZ_QUIET`. `guard` is now listed in `--help`.
- **Command results go to stdout, diagnostics to stderr.** `list`, `domain|profile|include|skill list`, `clean --dry-run` and `lock --check` print their result on stdout; `-q` hides only progress and success lines. The content commands (`add`, `remove`, `domain`, `profile`, `include`, `skill`) print the created/removed/rewritten path on stdout, take `--format json`, honor the global `--config-dir`/`-C`, and gain the new `show` and `edit` verbs; `add check` takes `--severity`, `--tools` and `--targets`. `ai-rulez --version` and `version` print to stdout, and a group command with no subcommand prints help and exits 0 (was 1).
- **One error rendering and one exit path.** Every failure prints `Error: ...` (then validation errors and a `Hint:`) on stderr and, under `--format json`, a `{"status":"error", ...}` document on stdout. Exit codes: `0` ok, `1` could not run, `2` findings/drift/failed gate, `3` (`lock` only) served skills left unpinned. A declined or non-interactive confirmation exits 1; prompts go to stderr.
- **Report commands share one flag vocabulary and one JSON contract.** `sbom --format cyclonedx|spdx-json` is `sbom --type`; `--format` is `text|json`; `telemetry hook --format json|toml` is `--syntax`; `eval run` prints `text` by default (`--format markdown` keeps the old output). Every `--format json` document carries `schema_version`, and lists return `{"schema_version":1,"items":[...]}` instead of a bare array. Each report has a published schema under `schema/` (table in `docs/cli.md#json-contracts`).
- **`verify` checks attestations and provenance only.** Drift is `generate --check`, which re-renders and also catches hand edits and deleted files; a bare `ai-rulez verify` exits 1 and names it.
- **`validate` runs the content checks by default.** `--config-only` restores the old config-only run; `--strict` now means warnings fail (`--fail-on warning`), so `validate` can exit 2 where it exited 0. Malformed YAML frontmatter is a finding, `AR306 frontmatter-malformed` (`generate` still exits 1), and a configuration that loosens the organization policy (`AR740`) exits 2 from `generate`, `validate`, `lock`, recursive runs and `doctor` instead of 1.
- **`generate --locked` and `--frozen` fail closed without an `ai-rulez.lock`** (exit 2, hint to run `ai-rulez lock`).
- **`clean` keeps generated files you edited by hand** unless `--include-edited` is passed; `--yes` only skips the confirmation prompt.
- **`usage ...` and `report usage|evals` fold into `telemetry ...`** with no aliases (`telemetry hook|record|feedback|report [evals]`); `[lint.budget]`/`[lint.tolerate]` are renamed `[lint.ratchet]` (JSON `ratchet_exceeded`), while `[lint.budgets.<kind>]` size limits keep their name.
- **New defaults.** `agents_md = true`: `AGENTS.md` is canonical and `CLAUDE.md` imports it, with `agents_md = false` for per-harness files. Generated headers carry only the per-file `Content-Hash` (`[header] hashes = "full"` restores `Source-Hash`). The managed `.gitignore` block is opt-in (`gitignore = true` or `generate --gitignore`).
- **`liter-llm` is the only model backend and builds need cgo.** Every model call goes through [liter-llm](https://github.com/xberg-io/liter-llm) 2.2.0, linked statically, so ai-rulez is built with `CGO_ENABLED=1` and a C toolchain; release binaries grow about 35 MB. The `[llm] backend` key and `AI_RULEZ_LLM_BACKEND` are gone (an OpenAI-compatible gateway is a `base_url`). Build from source with `task setup:llm` once.
- **Stricter names and values for content, the CLI and the MCP tools.** A name must not end in `.md`, contain whitespace or start with `.` or `-`; `--targets` rejects unknown words; `remove` checks the item exists; `domain remove` refuses while a profile lists the domain; `add skill` without a description scaffolds one that passes `validate`.
- **Locks.** `[lock] enforce` defaults to true whenever `ai-rulez.lock` exists; every pin uses one hashing scheme (sha256, algorithm-prefixed Merkle tree) and scripts are hashed byte for byte; a lock without content pins fails `--check`; a lock that pins role outputs is written `version = 2`. Locks written by earlier v5 builds read as stale once — run `ai-rulez lock`.
- **Remote sources.** Plain `http://` and `git://` are rejected (use `https://`, `ssh://`/`git@host:path` or a local path); the git token (`AI_RULEZ_GIT_TOKEN`, `--token`) is sent only to `github.com` or the hosts in `AI_RULEZ_GIT_TOKEN_HOSTS`, as a host-scoped header rather than inside the URL.
- **A committed config cannot reach outside the project**: a local include, skill-source path, custom preset `path`, provider spec path, `okf.dir` or `marketplace.output_dir` that escapes it (or names `.git`/`.ai-rulez`) is refused, and a `file://` include or skill source must name a repository inside the project (override with `config.local.toml`, the user config or `AI_RULEZ_ALLOW_FILE_URLS=1`). Content symlinks in the project's own `.ai-rulez/` are followed only when the target is inside the project; `generate` refuses symlinks that leave it or point into `.git` or credential files and stops on `AR001` secrets.
- **Security scanning.** `[lint.security] scan_imports` is on by default (`generate` scans imported content and stops at error-level findings; `AR012` is an error under `[lock] enforce`); `ai-rulez scan` reports only the `security` analyzer; `lock` and `update` scan what they pin and refuse error findings unless `--accept-findings`; staged `[[lint.external]]` scanners run confined by default (`isolation = "auto"`, no network unless `egress = true`).
- **`init --from` runs through `convert --write`** with the same scan, validation and lossiness report; an existing config directory is moved aside and restored if the import fails.
- **Preset changes.** `windsurf` is renamed `devin` (the preset, its `.windsurf/` output directory and the `windsurf_model` frontmatter key; there is no alias — rename in `config.toml` and agent files and delete the old outputs), and the `continue-dev` preset is removed with no replacement (`doctor` reports both). `[[plugins]]` no longer writes `.claude/plugins.json` or `.codex/plugins.json` (no tool reads them; `generate` warns).
- **Claude Code MCP servers are written only to `.mcp.json`**, so resolved MCP env values do not land in a committed settings file; an entry an earlier version wrote into `.claude/settings.json` is removed on the next `generate`/`clean`.
- **User-scope and signed data.** `[telemetry] service_name` is user scope only (a repository value is ignored and reported as `AR9K1`). Eval results are signed per user (`eval-results.key`): an unsigned or edited record is `unverified`, re-run and ignored, and CI must use `eval run --force`. The usage log is version 3 (`digest`, `digest_scheme`, `event_id`, salted `session`), and `schema/catalog.schema.json` describes catalog version 2.
- **MCP tools are stricter.** Arguments are validated against typed schemas (`additionalProperties: false`), so an unknown argument, wrong type, out-of-enum or missing required argument is an error result instead of being coerced; `validate_config` and other failures return `isError: true` instead of a success document; `working_directory`, `config_file` and `config_dir` are confined to the server's start directory (`mcp --root <dir>` to change it, `mcp --allow-any-dir` to lift it); the authoring server no longer advertises `tools.listChanged`; `roles resolve --format json` spells the key `honored`.
- **`[mcp] self_server` defaults to `true`**: `generate` adds the ai-rulez MCP server to `.mcp.json` (and each harness's own MCP file); opt out with `[mcp] self_server = false` or `generate --no-self-mcp`.
- **`improve pr` confines what it runs** (scrubbed git environment, no hooks, only the push keeps transport credentials; `--isolation` cuts the network for the worktree `generate`/`lock` and `--allow-network` leaves it on), and the scheduled repair workflow template pins its actions to a commit SHA.

### Added

- **OKF (Open Knowledge Format, spec v0.2).** `export okf`, `import okf` and `okf validate` read and write deterministic bundles, `[[includes]] format = "okf"` uses one as a source, the opt-in `okf` preset keeps a bundle in sync with `generate`, and the `.ai-rulez/` tree itself can be an OKF bundle (`migrate okf`). See `docs/okf.md`.
- **llms.txt** via the `llms-txt` preset (`llms.txt` and optional `llms-full.txt`), linted by `validate --strict`. See `docs/llms-txt.md`.
- **Agent Plugins (agent-plugins.org) packages.** `generate --plugin` writes a portable `plugin.json`, `skills/` and `mcp.json` conformant to the published 1.0.0 specification (or the 1.1.0 draft via `[plugin] spec`), and the `copilot` runtime and the Codex root manifest write the same package. `validate --strict` loads it as a conformant client would (`AR9O0`-`AR9O5`), `verify --plugin` compares it against its sources, `publish --emit agent-plugins` emits a clean directory and `convert --from agent-plugins` imports one. See `docs/agent-plugins.md`.
- **Agentic Resource Discovery (ARD)** manifest (`[ard]`, `publish emit ard`) and **Pi** packages (`[plugin] runtimes = ["pi"]`). See `docs/ard.md`, `docs/plugins.md`.
- **New MCP tools** for what the CLI already did (`scan_content`, `token_report`, `cost_report`, `sbom`, `okf_validate`, `approvals_status`, `policy_show`, `list_verifiers`, `list_builtins`, and agent/command CRUD); `generate_outputs` takes `profile`/`role`/`check`/`offline`; the authoring server gains prompts (`author-skill`, `add-rule`, `review-config`, `trim-context`), resources (`ai-rulez://config`, `ai-rulez://catalog`, item URIs) and cancellation. Approving, signing, locking, evaluating and publishing stay CLI-only.
- **Shell completion** of enum values and project names (rules, context, skills, agents, commands, checks, domains, profiles, roles, includes), and a generated command reference (`docs/cli-reference.md`) with examples for every command.
- **Dynamic skills and roles.** `delivery: static|served|both` lets `mcp --serve-skills` serve skills on demand with `find_skill`, `load_skill` and `list_skill_resources`; `[[roles]]` maps a job to a slice of the content with `generate --role`, `roles list|show|resolve` and a generated `roles.json`; skills can be served from a git repository or directory via `[[skill_sources]]`, pinned in the lock. See `docs/mcp-server.md`, `docs/roles.md`.
- **Content lock.** `ai-rulez.lock` pins the version, every authored item, the generated outputs, remote sources and served skills, with `lock --check`, `lock --diff`, `lock --content-only`, `lock --subject`, `--locked`/`--frozen` and version-constraint pins moved by `ai-rulez update`. See `docs/lockfile.md`.
- **`checks` content kind**: code-review guidelines in `.ai-rulez/checks/` rendered for the review tools that read a repository file, with a marker block that keeps your own text. See `docs/checks.md`.
- **`generate --watch`** live regeneration, and **`generate --emit-plan FILE`** for a deterministic plan of every file.
- **`ai-rulez doctor`**: read-only diagnostics (removed presets, drift, unresolved MCP placeholders, lock drift, missing tools), exit 2 on errors.
- **Hooks and permissions across harnesses.** Top-level `[[hooks]]` and `[permissions]` are translated into each harness's native format and merged into your settings files key by key, so hand-written entries survive; matchers are translated by tool name; plugin-based harnesses (OpenCode, Kilo, MiMo Code, Pi, Amp) get a generated hook module. See `docs/settings.md`, `docs/permissions.md`.
- **User-level output.** `generate --user` renders a user config into each preset's per-user locations and `clean --user` removes exactly that. See `docs/user-scope.md`.
- **`validate --strict`**: deep content validation with stable `AR` codes, report formats (`text|json|sarif|github|junit|markdown`), baselines, budgets/ratchet, `--fix`, `--since`/`--changed`, `--analyzer`, a risk score and `--explain`. See `docs/strict-validation.md`.
- **Security checks `AR001`-`AR011`** in strict validation and the dedicated `ai-rulez scan` command (secrets, hidden characters, prompt injection, credential access, unrestricted tools, an outbound host allow-list and merged `[[lint.external]]` results), plus harness trap lint. See `docs/harness-traps.md`.
- **Reports and analysis**: `tokens` (prompt-token cost by load bucket, including the item listing), `cost` (context-cost offenders), `catalog` (`--format json` and a self-contained `--html` site), `sbom` (CycloneDX 1.6 and SPDX 2.3) and `search` (lexical plus an optional hybrid embeddings index). See `docs/sbom.md`, `docs/search.md`.
- **Governance.** `approve` records reviewer approvals in the lock bound to the content digest, with `[governance]` policy (approvers, `min_approvers`, `CODEOWNERS`, expiry, sign, `approve --from-github-review`). `sign` writes Sigstore bundles (key, KMS or keyless) for the lock, bundles, skills, SBOM and policy, and `verify --attestation` checks them against `[signing]`; `verify --self` verifies the running binary against its release provenance. An organization policy from outside the repository (`--policy`, the managed path, `--discover-org`) bounds sources, lint, locking, governance, signing, scanners, MCP, hooks and network use, and `validate --show-policy` prints the effective policy. `publish` preflights, builds a reproducible archive and releases to GitHub, npm or OCI with `--execute --yes`. See `docs/signing.md`, `docs/policy.md`, `docs/publish.md`.
- **`review`** scores items against a rubric (optional LLM judge with `--semantic`, calibration and a gate), and **`improve`** (experimental) runs an external optimizer behind a held-out eval gate. See `docs/review.md`, `docs/improve.md`.
- **`eval run`** executes harness-neutral skill eval cases (pass rate, trigger precision/recall, ablation, token cost) with adapters and a result cache. See `docs/evals.md`.
- **Telemetry** (opt-in, identifier-only): `telemetry hook|record|flush|doctor` and an OTLP/HTTP export through a bounded offline outbox, plus `report usage` sections for rules, agents and context. See `docs/telemetry.md`.
- **`convert`** reads existing tool files into an equivalent `.ai-rulez/` tree with a lossiness report (native, rulesync, skills-lock, APM, Tessl and OKF importers), and `init --from` uses it.
- **`pkg/airulez`** (experimental): load, validate, plan and apply a project from Go with no working directory, environment, clock or subprocess. See `docs/embedding.md`.
- **Preset and plugin outputs**: the `baz` preset; native MCP, command and user-scope outputs for many existing presets; per-domain plugins and marketplaces (`[marketplace]`); `[placement]` to keep skills and commands plugin-only; `.codex/config.toml` merged as a document. See `docs/plugins.md`.

### Changed

- `publish --to npm` preserves the runtime metadata (Pi resources, OpenCode entry points, dependencies, keywords, license) of the package it publishes while owning the name, version, `files` list and `publishConfig`.
- `validate_config` and `scan_content` show project-relative paths and carry the whole lint document.
- `generate` validates `config.toml` against the schema and warns about unknown keys (`--strict-config` or `AI_RULEZ_STRICT=1` fails), and prints a summary of new or changed commands (hook commands, MCP servers, managed env, plugin enablement, permissions) even with `--quiet` (`--yes` or `AI_RULEZ_ACK_COMMANDS=1` silences it).
- `clean` keeps a generated file whose body was edited by hand and warns; `generate --dry-run` marks current files `unchanged:` and edited files `edited:`.
- The lock and `generate --check` return exit codes per run, so `3` works with `--recursive`; a served skill the security scan refuses is reported and left unpinned (`lock` exits 3) unless `lock --refuse-findings`.
- `tokens` counts the item listing for harnesses that list skills, commands or agents, so `--budget` gates the number the prompt really carries.
- `liter-llm` is pinned at v2.2.3, and the LLM response cache is version 3.
- Several presets write their native MCP, command and agent shapes more accurately (for example `opencode` uses `mcp.<name>` with `enabled`, `codex` writes skills to `.agents/skills`, the root `.mcp.json` follows the `mcp` preset shape, and `antigravity` writes commands as skills).

### Fixed

- `migrate v5` drops the rejected 4.x `[[skills]]` table, keeps CRLF in `config.toml` and preserves comments; `migrate okf` writes atomically with a backup and refuses to destroy a rule named `index` or `log`.
- `validate` on an OKF bundle reports the OKF verdict before "Configuration is valid" and writes the OKF document under `--format json`; `-q` no longer prints the OKF summary or strict totals.
- `generate --check` no longer reports a file `generate` just wrote as `edited`.
- Content is never read, written or deleted through a symlink inside the configuration directory; `add`, `edit`, `show`, `remove` and the MCP CRUD tools refuse symlinked domains, directories and files.
- `edit` and the MCP update tools keep an item's priority, targets, description and other frontmatter when only the content changes.
- No log line or error prints a credentialed include or skill source; `include add` and `skill install` warn and redact.
- `export okf --output-dir` can no longer delete your configuration, and `import okf` strips group/world-write bits and redacts credentials; OKF export, import, export is byte-identical.
- Locking fails closed: an enforced lock fails when an include cannot be resolved, file modes in digests are OS-independent, and `validate --strict` reports `AR981` instead of skipping an unverifiable lock.
- Running inside a git hook no longer corrupts the repository (every git subprocess drops `GIT_DIR` and the other inherited git variables).
- `[llm]` trust and safety: `allow_network`, `base_url`, `api_key_env` and price overrides are honoured only from user scope, a pasted key is never echoed, the budget cannot be overshot by concurrent calls, and `timeout_seconds`/`max_cost_usd` reject unusable values.
- The `mcp --serve-skills` server bounds served-skill metadata, applies the session budget to `get_skill`/`read_skill_file`/`resources/read`, handles skill-source name collisions, and fetches a locked commit by SHA even after a tag moves.
- Hook merges read commented JSONC/TOML/YAML documents, and presets that share one document union their owned keys instead of conflicting.

### Removed

- The hidden `mcp --transport`, `--address` and `--port` flags (the server speaks stdio only).
- `ai-rulez migrate`, `init --format`, legacy `mcp.yaml`/`mcp.toml`/`mcp.json` loading and the V2/V3 config readers.
- The `compression` option (a no-op since v3.13) and the legacy `init --from` importer engine.
- The process-wide preset registry and other legacy Go APIs; `pkg/airulez` is the supported embedding API.

### Security

- MCP `--root` confinement runs on the symlink-resolved path it judged, so a link swapped between the check and the tool's use cannot redirect a call outside the root.
- The `run_verifiers` MCP tool never starts a program (command predicates are refused even with `AI_RULEZ_VERIFIERS_ALLOW_EXEC` set).
- The committed generated manifest no longer licenses anything: a forged claim cannot strip permissions, env or allow keys from your settings, and the local manifest stores array elements as digests rather than values.
- `ai-rulez.lock` no longer pins resolved secrets or the checkout path, and lock writes never follow a committed or linked symlink.
- Output paths cannot escape the project: a custom preset, provider spec, `okf.dir` or `marketplace.output_dir` can no longer make `generate` write outside the project or into `.git`, and symlinked output directories are not followed.
- The git token never leaves allowlisted hosts and never appears in process arguments or a cached clone's `.git/config`; include and skill caches are `0700` and keyed by name plus a URL hash.
- Imported content is scanned: symlinks in includes, installed skills and OKF bundles are skipped, files are size-capped, a local `[[skill_sources]]` path must resolve inside the project, and credentials in a source URL are refused (`AR035`).
- Machine-local files are never written or read through a symlink, and the LLM cache secret must be a regular `0600` file.
- Hook `script` paths are validated and shell-quoted, permission translations do not widen or bypass rules, and hook plugin matchers that Go and JavaScript read differently are skipped with a warning.

## [4.24.2] - 2026-10-04

### Fixed

- Internal/test-only fixes; no user-visible change.

## [4.24.1] - 2026-10-04

### Fixed

- Internal/test-only fixes; no user-visible change.

## [4.24.0] - 2026-10-04

### Added

- **`pi` preset** for the [pi](https://pi.dev) coding agent: a shared `AGENTS.md`, skills in `.agents/skills`, agents in `.pi/agents`, and MCP servers in `.pi/mcp.json`.

### Changed

- `AGENTS.md` is now shared by `pi` alongside `codex`, `opencode`, `xum` and `amp`.

## [4.23.1] - 2026-10-03

### Added

- Xum stdio MCP `env` is written as a shell assignment prefix.
- `generate` warns once about a v1-shaped OpenCode plugin (OpenCode v2 does not run it).

### Changed

- **MCP servers in shared settings documents are owned one by one**, so a server you wrote by hand survives `generate` and `clean`.
- A plugin `name` must match `^[a-z0-9][a-z0-9._-]*$` and contain no `..` for every runtime.

### Fixed

- Local content reaches each tool through a file it loads (`GEMINI.local.md`, `AGENTS.local.md`, `AGENTS.override.md`, `.junie/rules/ai-rulez.local.md`, ...); the old non-working names are removed on upgrade.
- `clean` leaves no ai-rulez keys behind in hand-authored settings documents.

## [4.23.0] - 2026-10-03

### Added

- **`config.local.*` overlay** and `ai-rulez local` for machine-local configuration, plus local skills, agents, commands and domains under `.ai-rulez/local/`.
- **`agents_md`**: renders one shared `AGENTS.md` and `.agents/skills` tree for many presets.
- A shared baseline and drift guard: a teammate-view run never deletes your local outputs, and drift on a tracked shared file stops generation unless `--allow-local-drift`; `--no-local` renders the teammate view.

### Changed

- `.gitignore` management adds only what git does not already ignore.

### Fixed

- Gemini agents are written to `.gemini/agents/`, the location Gemini CLI loads.
- Xum disabled stdio MCP servers are written as disabled.

### Security

- Generated files that contain resolved MCP secrets are written `0600`.
- Include and skill sources are logged with URL credentials redacted.

## [4.22.2] - 2026-10-03

### Fixed

- Generator schema v8 forces a one-time rewrite of existing generated files.
- Cursor agents are written to `.cursor/agents/`.
- OpenCode agent model, variant, `hidden`, `temperature` and `top_p` are written in the shapes OpenCode accepts (an unqualified model is omitted with a warning).

## [4.22.1] - 2026-10-03

### Fixed

- Cursor `globs` is written as a bare comma list.
- Negated globs (`!x`) are dropped from every dialect with a warning.
- `AGENTS.md` is identical across `codex`, `opencode`, `xum` and `amp`.
- Rules no longer vanish when a rule file name is taken (written as `<id>.ai-rulez.md`), and names that map to the same file id no longer abort generation (later ones get a stable suffix).

## [4.22.0] - 2026-10-02

### Added

- Rule `activation` frontmatter (`always`, `glob`, `auto`, `manual`), plus `[rules] mode` and `mode_by_preset`.
- Copilot path-specific instructions (`.github/instructions/*.instructions.md`).
- Local rules written as native rule files.

### Changed

- **BREAKING: rules are split into native rules folders by default** (`[rules] mode = "split"`): Claude writes `.claude/rules`, Copilot `.github/instructions`, Junie `.junie/rules` and Antigravity `.agents/rules`. Set `[rules] mode = "inline"` (or `mode_by_preset`) for the old layout.
- Frontmatter `targets` now restrict rule outputs, and scoped rule files go to the root rules folder with the scope path as a prefix.

### Fixed

- Include and skill-source URLs no longer leak credentials into logs or errors.
- Cline, Continue and Windsurf rules honour `paths`/`globs`; Cursor context rules are applied automatically.
- Generated rule files are gitignored per file, and a hand-written rule file is never overwritten.

## [4.21.0] - 2026-10-02

### Added

- **`headers` on `[[mcp_servers]]`**: remote servers can send HTTP headers (`Authorization = "Bearer ${TOKEN}"`).
- **`[header] hashes`** (`full`, `content`, `none`) chooses the freshness lines; `content` stops one edit from rewriting every generated file.

### Fixed

- The secret guard now also covers `opencode.json`.

## [4.20.1] - 2026-10-02

### Fixed

- CRUD commands and the MCP CRUD tools work in a project using the `.config/ai-rulez/` layout.

## [4.20.0] - 2026-10-02

### Added

- **`[mcp] self_server`**: `generate` can add ai-rulez's own MCP server to the project `.mcp.json`.
- **`validate --recursive`** (`-r`).

### Fixed

- `generate --recursive` exits non-zero when any root fails.
- `clean` no longer removes a merged settings document that holds content ai-rulez did not write.
- `validate --quiet` is honored.

## [4.19.0] - 2026-10-02

### Added

- Configuration discovery also accepts `.config/ai-rulez/` (the [`.config` proposal](https://github.com/pi0/config-dir)) as a fallback; `.ai-rulez/` still wins when both exist.

### Changed

- Generated headers name the real config directory.

## [4.18.0] - 2026-10-01

### Added

- **`[header] text`** replaces the generated banner with your own multi-line text while keeping the freshness lines.

### Fixed

- Rules and context render in priority order again (critical to minimal).
- Regeneration is forced once after the ordering change.
- A generated `opencode.json` stays a managed artifact (it owns `$schema`).

## [4.17.0] - 2026-09-30

### Added

- **Path-scoped rules** via `globs`/`paths` frontmatter (Claude `.claude/rules/<id>.md`, Cursor `.mdc` with `alwaysApply: false`).
- Cursor rule frontmatter (`alwaysApply`/`globs`/`description`).
- Profile-scoped `[[mcp_servers]]` and `[[installed_skills]]` (`profiles` list).
- `defaults.omit_agent_fields`.

### Fixed

- Generated agents are spawnable as subagents (OpenCode `mode: all`, Xum `subagent.runnable`).
- Scoped outputs no longer repeat the root content.

## [4.16.0] - 2026-09-30

### Added

- **`${PROJECT_ROOT}`** can be used in an MCP server's `command` or `args`, resolved at generation time.

### Fixed

- `validate` checks the raw config against the JSON schema (unknown keys and bad enums are reported).
- `include add --merge-strategy` accepts the values the resolver uses.
- The `popular` pseudo-preset emits the curated provider set, and documentation was corrected.

## [4.15.0] - 2026-09-30

### Added

- **Profile-scoped builtin domains** (`builtin:<name>`, e.g. `builtin:docker` loads a pack for one profile only).
- **OpenCode v2 preset output** (`opencode.json` with `mcp.servers`) and an OpenCode v2 plugin adapter.

### Changed

- Merged JSON documents can own a nested key path (for example `mcp.servers`).

## [4.14.1] - 2026-09-29

### Fixed

- Internal/test-only fixes; no runtime behavior changed.

## [4.14.0] - 2026-09-29

### Added

- **`xum` preset** for the [Xum](https://xum.coder.com) coding agent (`AGENTS.md`, `.xum/skills`, `.xum/agents`, `.xum/mcp.jsonc`).
- **Provider-backed custom presets**: a `[[presets]]` entry may reference a declarative provider spec instead of a template.

### Fixed

- CRUD commands no longer write a spurious `config.yaml` into a TOML project (a mutation loses the comments of a hand-commented `config.toml`).

## [4.13.0] - 2026-09-27

### Added

- **`ai-rulez tokens`** reports the prompt-token cost of the configuration by load bucket (`always`, `conditional`, `on demand`, `unmodeled`), with `--budget`, `--compare-profiles` and a `--tokenizer`.
- **Composed profiles**: `--profile base,backend` (or `default = "base,backend"`) resolves to the de-duplicated union of their domains.

### Changed

- **The `Generated:` header line is now off by default** (output is byte-reproducible); `[header] timestamp = true` opts in.
- **Builtin rules are roughly halved**, with narrow guidance (code-quality, testing, token-efficiency, docker, observability) moved into skills that load on demand.

### Fixed

- Skills and commands now honour source precedence (root > domain > include > builtin); the previous inversion silently dropped root and domain items.
- `generate` removes the directories its own stale-file pass emptied.

## [4.12.1] - 2026-09-25

### Fixed

- `generate` no longer deletes H1-like lines from inside fenced code blocks.
- A merged settings document keeps its indentation and CRLF line endings.

### Changed

- Removed the unused `internal/scanner` package.

## [4.12.0] - 2026-09-25

### Added

- **Bundled hook scripts for plugins** (`script` field) plus the hook action `args`, `timeout`, `if` and `status_message`.
- **Directory form for commands** (`commands/<name>/COMMAND.md` with `references/`).

### Changed

- **Settings documents are merged, not overwritten** (`.claude/settings.json`, `.mcp.json`, `.gemini/settings.json`, `.agents/settings.json`, `.amp/settings.json`): ai-rulez owns specific keys and preserves the rest.
- `.gitignore` lists the subdirectories ai-rulez writes (`.claude/skills/`) rather than whole assistant directories.
- `init --setup-hooks` preserves comments, key order and indentation.

### Fixed

- Non-canonical skill/command subdirectories warn; `validate` reports skill/command output-id collisions; a plugin passthrough source outside the project (including through a symlink) is refused.

## [4.11.5] - 2026-09-19

### Fixed

- Skill includes pinned to a full commit SHA resolve deterministically and fail closed.
- Schema validation compiles with a fresh compiler per call.

### Changed

- Go toolchain to 1.27 and dependency upgrades.

## [4.11.4] - 2026-09-18

### Fixed

- The managed `.gitignore` block is spliced back in place instead of being reordered.
- `validate` fails on malformed content frontmatter, and a skill with malformed frontmatter is no longer dropped from output.
- Includes pinned to a full commit SHA fail closed instead of falling back to stale cache.

## [4.11.3] - 2026-08-24

### Added

- `[header] timestamp = false` omits the `Generated:` line.

### Fixed

- `generate` is reproducible across checkout paths.

## [4.11.2] - 2026-08-08

### Fixed

- `ai-rulez mcp` no longer wedges when a host sends `initialize` twice on one stdio session.

## [4.11.1] - 2026-07-31

### Fixed

- `generate` no longer inlines the full rules block into every generated skill and agent file.
- A skill whose source frontmatter fails to parse no longer emits a second raw frontmatter block.

## [4.11.0] - 2026-07-22

### Added

- **`ai-rulez clean`** removes the files produced by `generate` (the inverse of `generate`), with `--dry-run`, `--force`, `--keep-gitignore` and `--keep-manifest`, exposed as the `clean_outputs` MCP tool.

### Fixed

- MCP `update_rule`/`update_context`/`update_skill` no longer fail with `file already exists`.

## [4.10.0] - 2026-07-22

### Added

- **Local-override content** under `.ai-rulez/local/` (rules and context), emitted only to per-preset `.local` files and gitignored, with `--local` on `add rule`/`add context`.
- **Bare/flattened include layout**: included repositories no longer need an `.ai-rulez/` wrapper.

### Changed

- Builtin language, binding, OWASP and dependency conventions now emit as on-demand skills instead of always-inlined rules, shrinking the generated root files.
- The default header style is now `minimal`.

## [4.9.4] - 2026-07-12

### Changed

- Builtin language and binding rules are tool-agnostic (opinionated tools are framed as examples).

### Fixed

- The Python builtin no longer references a TypeScript type, and vite+ typos are fixed.

## [4.9.3] - 2026-07-12

### Fixed

- Windows drive-relative plugin paths (`C:outside`) are rejected on every platform.

### Changed

- Poly catalog npx/uvx execution paths are pinned for reproducible hooks.

## [4.9.2] - 2026-07-12

### Fixed

- Plugin path validation is consistent across operating systems.

## [4.9.1] - 2026-07-12

### Fixed

- Poly hook installation paths use the `version` subcommand.

## [4.9.0] - 2026-07-12

### Added

- Reusable `poly-hooks.toml` catalog with selectable hooks and guarded execution paths.
- Recursive plugin generation and verification (`--if-configured`).
- Plugin generation and verification hooks for Poly and pre-commit.

## [4.8.0] - 2026-07-12

### Added

- **Native Hermes Agent** generation (`hermes` preset, `.hermes.md`, project plugins and PyPI entry points).
- `plugin.content_root`, `plugin.hermes.source` and `plugin.hermes.requires_python`.
- `ai-rulez verify --plugin` for plugin freshness and provenance.

### Fixed

- Authored Markdown whitespace is preserved around generated headers.

## [4.7.0] - 2026-07-11

### Added

- Complete Codex plugin bundles (root MCP config, marketplace metadata, recursive skill resources).
- OpenCode plugin generation from `.ai-rulez/opencode/index.js`.
- Deterministic plugin provenance (generated-file headers and `.ai-rulez-generated.json` sidecars).

## [4.6.0] - 2026-07-08

### Added

- **Plugin and marketplace authoring** via `ai-rulez generate --plugin` (Claude, Cursor, Codex, Gemini, Kimi, OpenCode, Factory), with a `[plugin]` block, hooks, `${PLUGIN_ROOT}`, and single-plugin or monorepo marketplaces.
- Agent **`extends`** directive: an agent inherits a lower-precedence agent's body and frontmatter and appends its own.

### Changed

- Agent precedence is deterministic (same-named agents collapse to the highest-precedence definition).

## [4.5.0] - 2026-07-02

### Added

- Rule and context **deduplication by name** with source precedence (root > domain > include > builtin).
- `compact` option to omit per-rule priority annotations, and per-rule builtin exclusion (`!domain/rule`).
- MCP tool annotations and server title/instructions.

### Changed

- Remote MCP servers are emitted in each tool's own format (no empty `command`/`transport`); stdio is unchanged.
- Preset rendering moved to a declarative provider DSL, and lint/format moved to poly.

## [4.4.1] - 2026-06-07

### Fixed

- `.ai-rulez/.generated-manifest.json` is gitignored when gitignore management is enabled.

## [4.4.0] - 2026-06-07

### Added

- Per-preset model overrides for agents (`<preset>_model`, e.g. `claude_model`) with `defaults.model_by_preset` as the fallback; the legacy `model:` still works.

### Fixed

- The TOML loader no longer drops the `[defaults]` table.

## [4.3.0] - 2026-05-30

### Added

- `generate --gitignore` (with `-i`); the old `--update-gitignore` remains a deprecated alias.
- Collision-safe short flags across commands.
- MCP environment placeholder resolution from `--env`, the process environment, `.env` and `--env-file`.

### Fixed

- Secret-bearing MCP outputs fail generation when their path is not gitignored, and resolved secrets are redacted before source-hash calculation.

## [4.2.2] - 2026-05-26

### Added

- `polyglot-bindings` builtin (Rust-core, native ABI, FFI ownership, cross-language errors).

### Changed

- The TypeScript builtin recommends `oxfmt` with `oxlint`.

## [4.2.1] - 2026-05-21

### Fixed

- Cursor/Codex skill descriptions are quoted, so descriptions containing colons load.

## [4.2.0] - 2026-05-21

### Added

- **Manifest-scoped generation cleanup**: `generate` deletes only files recorded in `.generated-manifest.json`.
- **Real dry-run output** (`create-dir`, `write-file`, `delete-stale`) without touching the filesystem.
- **Custom config directory** (`--config-dir`), exact config-path loading, and `[[scopes]]` subfolder outputs.

### Fixed

- Hand-written files in assistant output directories are preserved during regeneration.

## [4.1.6] - 2026-05-03

### Changed

- **Git fetches use sparse checkout** (`--depth 1 --filter=blob:none --sparse`), fixing "response body too large" and cutting network/disk use; requires git ≥ 2.25.
- **BLAKE3-based cache invalidation** replaces the time-based TTL.

## [4.1.5] - 2026-05-02

### Changed

- **Skill resources are no longer concatenated into `SKILL.md`**: `references/`, `scripts/` and `assets/` are emitted as separate files, and `SKILL.md` carries a `## Resources` index (progressive disclosure).

### Security

- A malicious installed skill cannot follow a symlink out of its directory.

## [4.1.4] - 2026-05-01

### Fixed

- Internal test-client rewrite; no user-visible change.

## [4.1.3] - 2026-04-30

### Fixed

- **`generate --recursive` on a polyglot monorepo** no longer takes minutes or aborts on a broken symlink: it prunes build and cache directories, skips shared rule libraries, processes configs in parallel and memoizes tree scans (minutes to about a second, steady state).

## [4.1.2] - 2026-04-30

### Added

- Per-subagent reasoning effort for Codex (`model_reasoning_effort`) and OpenCode (`reasoningEffort`).

## [4.1.1] - 2026-04-30

### Added

- Reasoning effort extended to Codex, Amp and Windsurf, with `defaults.effort_by_preset` and MCP `default_effort_by_preset`.

## [4.1.0] - 2026-04-29

### Added

- **Reasoning effort on Claude Code subagents** (`effort` frontmatter: low/medium/high/xhigh/max/inherit).
- `defaults.effort` project default and MCP `default_effort`.

## [4.0.8] - 2026-04-27

### Added

- **`Source-Hash` header line** and hash injection for YAML-frontmatter files (hashes as YAML comments).

### Changed

- Output is sorted alphabetically by name and normalized to a single trailing newline (one round of reordered output).

### Fixed

- Generation is deterministic (domain iteration sorted); the `tools` list is no longer corrupted into a Go slice string; the `mcp` preset validates; skip-on-content-hash fires for frontmatter+banner files.

## [4.0.7] - 2026-04-27

### Fixed

- `--debug` and `--update-gitignore` now take effect.
- `.gitignore` no longer duplicates a pattern already present outside the managed fence.

### Changed

- Intentional-behavior warnings and verbose generation logs demoted to debug.

## [4.0.6] - 2026-04-25

### Fixed

- The MCP schema accepts V4 configs, and generated headers reflect the actual config filename.

## [4.0.5] - 2026-04-25

### Fixed

- Config discovery finds `config.toml`/`config.json`, and recursive generation no longer skips them.

## [4.0.4] - 2026-04-25

### Fixed

- Pre-commit hooks fire on `.ai-rulez/` changes, stale versions in scripts/wrappers are updated, and init templates default to config version 4.0.

## [4.0.3] - 2026-04-24

### Fixed

- Gitignore uses subdirectory patterns for shared directories (for example `.github/agents/`).

## [4.0.2] - 2026-04-24

### Added

- **Builtin agents** (code-reviewer, test-writer, security-auditor, docs-writer, devops-engineer, release-engineer) and new rules (verification-before-completion, systematic-debugging, testing-anti-patterns).

### Changed

- README redesigned around builtin capabilities and the development workflow.

## [4.0.1] - 2026-04-24

### Added

- New builtins `cicd`, `docker`, `observability`; new rules `no-ai-signatures`, `agent-workflow`, `communication-style`, `tdd-workflow`, `anti-patterns`; more domains auto-included.

### Fixed

- `migrate v4` preserves MCP servers.

## [4.0.0] - 2026-04-23

### Changed

- **BREAKING: TOML is the default config format** (`init` writes `config.toml`); existing YAML configs still work.
- **BREAKING: MCP servers are inline** under `[[mcp_servers]]` (legacy `mcp.yaml` still loads with a deprecation warning).
- **BREAKING: V2 compatibility is removed** (config types, migration, validator, generators), along with the deprecated `CompressionConfig`, `--skip-mcp`, `--auto-migrate` and `migrate v3`.

### Added

- Agent generation for all presets, context rendering for per-file presets, Claude MCP/plugins output, Cursor/Copilot MCP, Codex plugins/commands, Copilot commands, TOML config support, `[[plugins]]`/`[[marketplaces]]` and `migrate v4`.

### Removed

- V2 types, loader, migration, validators and generators; V1/V2 schemas; the V2 template engine; MkDocs config and the generated `site/`.

## [3.14.2] - 2026-04-20

### Fixed

- **Critical: `generate` no longer deletes non-generated files in `.github/`** (workflows, CODEOWNERS, issue templates).

## [3.14.1] - 2026-04-20

### Fixed

- Added the CI/Taskfile tasks the workflows referenced, and fixed Windows path-separator tests.

## [3.14.0] - 2026-04-20

### Added

- **`antigravity` preset** (`GEMINI.md`, `.agents/settings.json`, skills/agents under `.agents/`).
- Skills and agents for Gemini, Cursor, Codex, Amp, Windsurf, Copilot, Cline, Junie and OpenCode; Cursor skills moved to `.agents/skills`.
- `vite-plus` builtin; MCP read tools; `working_directory`; enum constraints; `read_config`/`update_config`; `dry_run`/`recursive`; tool annotations; `builtins show`.
- **Content hash** in generated headers (files skipped when unchanged) and an include cache TTL with `--no-fetch`.
- Fenced, idempotent `.gitignore` updates.

## [3.13.1] - 2026-04-19

### Fixed

- Frontmatter parsing falls back to raw map parsing; a missing skill description is a warning; cache locations are unified under `~/.cache/ai-rulez/`.

## [3.13.0] - 2026-04-19

### Added

- **`agent-delegation` builtin** (an `## Agents` roster in generated output, disable with `!agent-delegation`).
- New binding builtins (`jni-rs`, `extendr`, `cgo`) and three token-efficiency rules.

### Changed

- `compression` is a no-op and deprecated (its stopword removal made output ungrammatical); the compression package was removed.

## [3.12.0] - 2026-04-17

### Added

- **Installed skills**: `ai-rulez skill install/remove/list`, fetched at generate time (`installed_skills` config, with MCP tools).
- A distributable `ai-rulez` skill and `docs/llms.txt`.

### Fixed

- YAML config saving preserves ordering, comments and formatting, and include/skill caches are cleared before each fetch.

## [3.11.5] - 2026-04-15

### Fixed

- The Claude preset emits no header comments in skill, agent or command files.

## [3.11.4] - 2026-04-15

### Added

- **`local_override` for includes** (use a local directory instead of a git fetch), and minimal headers for skills/agents.

### Fixed

- Claude frontmatter is emitted as the first line; skills carry `user_invocable` and `description`; included domains are always available; output is deterministic.

## [3.11.3] - 2026-03-29

### Fixed

- Includes fail fast when the resolver is not registered.

## [3.11.2] - 2026-03-25

### Fixed

- `.github/` is no longer ignored as a directory, and files inside fully-managed directories are not listed individually.

## [3.11.1] - 2026-03-25

### Added

- R language builtin (tidyverse, testthat, CRAN, extendr).

## [3.11.0] - 2026-03-25

### Fixed

- Include domains are no longer duplicated, Claude output shrinks from ~13 MB to ~440 KB, and stale files are cleaned before writing.

### Changed

- Local rules in `CLAUDE.md` use `@path` lazy-loading references.

## [3.10.0] - 2026-03-18

### Fixed

- Included skills expose their frontmatter, and included domains are discovered and usable in profiles.

## [3.9.0] - 2026-03-11

### Added

- Root command docs (`build`, `fix`, `lint`, `review`, `test`) and a repository-managed pre-commit config.

### Changed

- Replaced lefthook with the prek/pre-commit toolchain.

### Fixed

- Codex skills always carry a `description`; V3 validation rejects a missing description; E2E test binary setup is stable.

## [3.8.3] - 2026-03-08

### Fixed

- Domain content from includes is no longer duplicated into root slices.

## [3.8.2] - 2026-03-08

### Fixed

- The Claude preset collects domain skills, agents and commands, and the `default-commands` builtin generates correctly.

### Changed

- Language builtins expanded with linting, SAST, coverage and benchmark tooling.

## [3.8.1] - 2026-03-07

### Changed

- Replaced simple compression with a token-reduction engine (levels `off`/`light`/`moderate`/`aggressive`/`maximum`, markdown-aware).

### Fixed

- Flat ai-rulez structures are detected at sub-paths.

## [3.8.0] - 2026-03-07

### Added

- **Built-in domains**: 23 embedded content domains shipped with the binary.
- **`builtins` config field** (`true`/`false`/list/`!name`) and `ai-rulez builtins list`.
- `/iterate` and `/parallelize` slash commands.

## [3.7.3] - 2026-02-19

### Fixed

- The publish workflow resolves Go from `go.mod`.

## [3.7.2] - 2026-02-16

### Fixed

- Claude skill rendering respects frontmatter `targets`, and the npm installer supports offline/private-registry bundled binaries.

## [3.7.1] - 2026-02-16

### Fixed

- Windsurf trigger frontmatter safely quotes `description` and `glob`.

## [3.7.0] - 2026-02-16

### Fixed

- Includes now carry agents, and Codex skills always carry a `description`.

## [3.6.1] - 2026-01-07

### Fixed

- Windows releases use `.zip` instead of `.tar.gz`.

## [3.6.0] - 2026-01-05

### Changed

- Skills and commands are written to dedicated directories instead of inlined into root files across every preset (Claude's `CLAUDE.md` dropped ~77%, from 14K to 3.2K lines), reducing prompt tokens.

## [3.5.0] - 2026-01-04

### Added

- **V3-native command system**: file-based slash commands in `.ai-rulez/commands/` with YAML frontmatter, profile-aware and rendered per preset (`migrate v3` for V2 commands).
- Configurable prompt compression and context-file optimization (a required `summary`; context rendered as summaries with `@` links, ~34% smaller).

### Changed

- Remote include cache moved to the system cache directory.

### Fixed

- Includes merge commands from remote sources, and the default profile includes commands.

## 3.4.1 - 2026-01-03

### Added

- SSH git clone support for private repositories, self-hosted GitLab, and repositories whose root is the ai-rulez structure.

## 3.4.0 - 2026-01-03

### Added

- Configurable header styles (`detailed`/`compact`/`minimal`), `CLAUDE.md` generation, and markdown formatting with markdownlint compliance.

## 3.3.2 - 2026-01-02

### Fixed

- SSH git URL conversion in includes, with multiple SSH URL formats accepted.

## 3.3.1 - 2025-12-31

### Fixed

- SSH git URL detection (`git@host:path` was treated as a local path).

## 3.3.0 - 2025-12-31

### Fixed

- Complete agents support in the includes system.

## 3.2.2 - 2025-12-28

### Added

- `init` creates root and domain agent directories, and generated MCP config includes the ai-rulez server.

## 3.2.1 - 2025-12-28

### Fixed

- Gitignore uses relative paths and no longer ignores `.ai-rulez/`.

## 3.2.0 - 2025-12-28

### Added

- Full agent/subagent support in V3 (`.ai-rulez/agents/`), agent metadata, and Claude Code subagent output.
- Auto-migration of V2 configs (interactive in a terminal, silent in CI; `--auto-migrate`).

## 3.1.0 - 2025-12-27

### Added

- `migrate` command for V2 to V3 configuration migration.

## 3.0.0 - 2025-12-27

### Added

- Directory-based configuration (`.ai-rulez/`), CRUD CLI commands, 22 MCP tools, domains, profiles and includes.

### Changed

- **BREAKING: configuration changed from a single YAML file to a directory structure**, and `init` no longer uses AI agents for dynamic initialization.

### Removed

- **BREAKING: the `enforce` command and V2 dynamic init are removed**; single-file YAML support is dropped.

## 2.4.0 - 2025-10-22

### Fixed

- CLI MCP interference with template-generated files, and output type values in AI-generated configs.

## 2.3.0 - 2025-10-05

### Added

- AI-powered rule enforcement, automatic gitignore management, and the Junie preset.

## 2.2.0 - 2025-09-20

### Added

- MCP file-based configuration for Claude and other tools.

## 2.1.0 - 2025-08-20

### Added

- CLI MCP integration for Claude, and an improved init phase.

### Fixed

- Cross-platform binary extensions, and the Homebrew formula structure.

## 2.0.0 - 2025-07-30

### Changed

- **BREAKING: schema v2** with a priority enum, unified `name` (was `title`), and named target resolution.

## 1.6.0 - 2025-07-10

### Added

- Target filtering and named targets.

## 1.5.0 - 2025-06-25

### Added

- Initial release: core configuration generation, the rule definition system and a template-based generator.
