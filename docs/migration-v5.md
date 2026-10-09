# Migrating to v5

This page collects the breaking changes of the v5 release and what `ai-rulez migrate v5` does about each. The
detail of every change is in the [changelog](CHANGELOG.md).

v5 also changes what ai-rulez is: a standards-compliant lifecycle tool for agent knowledge and capabilities (author,
generate, bundle, validate, govern, publish). Nothing in your 4.x configuration needs to change for that; the positioning
decides which formats the tool generates and checks. [Standards](standards.md) carries the pinned spec versions and test
status. In short:

| Standard | Status | Generate / bundle | Lint / validate |
| -------- | ------ | ----------------- | --------------- |
| OKF (Open Knowledge Format, v0.2) | supported | `export okf`, the `okf` preset | `okf validate` |
| Agent Plugins | supported | `generate --plugin`, `publish --emit agent-plugins` | `validate --strict` |
| ARD (Agentic Resource Discovery) | supported | `publish --emit ard` | `validate` |
| Agent Skills | supported | `generate` | `validate` (partial) |
| AGENTS.md | supported | `generate` | `validate` |
| llms.txt | supported | the `llms-txt` preset | `validate` |
| MCP server config | supported | `generate` | `validate` (partial) |
| MCP server card | planned | not generated | none |
| CycloneDX / SPDX SBOM | supported | `sbom` | `sbom --check` |
| in-toto / DSSE / Sigstore | supported | `sign` | `verify --attestation` |
| OpenTelemetry (OTLP) | supported | `telemetry export --to otlp` | not applicable |
| "Agent bundle" | planned | spec to be confirmed | spec to be confirmed |

## Upgrade in four steps

1. Commit or stash your work, then preview: `ai-rulez migrate v5 --dry-run` prints the change list and writes nothing.
2. Run `ai-rulez migrate v5`. It rewrites the project in place (add `--recursive` for a monorepo with several
   `.ai-rulez/` directories). `--check` exits 2 while anything still needs migrating, for CI.
3. Run `ai-rulez doctor`. It reports removed presets with a replacement, drifted outputs and generated paths git
   does not ignore.
4. Run `ai-rulez generate`, review the diff and commit the sources and outputs together.

`migrate` only reads a **4.x** project. A 2.x or 3.x project must first be migrated to 4.0 with ai-rulez 4.x
(`npx ai-rulez@4 migrate v4`), then with `ai-rulez migrate v5`. Every v5 command that meets an older version stops
with one actionable error: 2.x and 3.x say "install ai-rulez 4.x to migrate it to 4.0, then run `ai-rulez migrate
v5`", 4.x says "run `ai-rulez migrate v5`".

## What `migrate v5` does

| Rule | Rewrite |
| ---- | ------- |
| `version` | `version = "4.x"` becomes `version = "5.0"`, keeping the rest of the line (including a trailing comment). |
| `convert-format` | A 4.x `config.yaml`, `config.yml` or `config.json` is converted to `config.toml` with keys, order and comments kept, and the old file is removed. The `$schema` key becomes `schema`. |
| `lint-ratchet` | `[lint.budget]` and `[lint.tolerate]` are renamed `[lint.ratchet]` (per-rule finding counts). `[lint.budgets.<kind>]` (size limits) is unchanged. |
| `preset-rename` | The `windsurf` preset becomes `devin` (outputs move from `.windsurf/` to `.devin/`, delete the old directory) and the removed `continue-dev` preset is dropped. With `--write`, `windsurf_model` in agent frontmatter becomes `devin_model`. |
| `mcp-merge` | A legacy `mcp.toml`, `mcp.yaml` or `mcp.json` (4.x read the first of them) is folded into `[[mcp_servers]]` of `config.toml` and removed. When `config.toml` already defines `mcp_servers`, nothing is merged: the file is left in place and the report warns, so you can move the servers over by hand. |
| `pin-default` | The three defaults v5 changes are pinned to their 4.x value so the generated output does not move: `agents_md = false`, `gitignore = true` and `[header] hashes = "full"`. Each pin carries a comment; delete the line to take the v5 default. A key you already set is never touched. |
| `local-overlay` | `config.local.yaml`, `.yml` and `.json` become `config.local.toml` (mode 0600). |
| `command-rename` | `ai-rulez usage ...` (`hook`, `record`, `feedback`, `export`, `prune`) and `ai-rulez report usage|evals` inside hook, verifier and script commands become `ai-rulez telemetry ...`. |
| `frontmatter-alias` | The pre-4.24 frontmatter spellings `permission_mode` and `user_invocable` (Claude Code ignores them) become `permissionMode` and `user-invocable`. Without `--write` they are only reported; with it the markdown sources are rewritten. |

Options:

- `--dry-run`: compute and print the change list, write nothing.
- `--check`: like `--dry-run`, and exit 2 when a project still needs migration (0 when none does).
- `--adopt-defaults`: do not pin the 4.x defaults; take the v5 ones (`agents_md = true`, no managed `.gitignore`
  block, content-only headers). Regenerate and review the diff.
- `--write`: also rewrite frontmatter aliases in the `.ai-rulez` markdown sources.
- `--recursive`: migrate every project found below the current directory.
- `--format json`: a machine-readable report (`schema_version`, one entry per project with `status`, `changes`,
  `warnings` and `error`).

A migrated project is a fixed point: running `migrate v5` again changes nothing. The rewrite is text-level, so
comments survive, and the result is decoded again before it is written; a file that would not load is reported and
left alone. Exit codes: 0 migrated or nothing to do, 1 a project could not be migrated, 2 `--check` found work.

YAML and JSON configs are not loaded in v5 (only `config.toml` is), so `migrate` converts a `config.yaml`,
`config.yml` or `config.json` to `config.toml`; see below.

## Breaking changes

Each entry says what changed, what `migrate v5` does, and what remains for you.

### Config format and files

- **`version = "5.0"` is the only accepted config version.** A `4.x` config is rejected with
  `run ai-rulez migrate v5`; `2.x`/`3.x` with the instruction to install ai-rulez 4.x first.
  *Migrate:* rewrites the version.
- **YAML and JSON configs are no longer loaded** (`config.yaml`, `config.yml`, `config.json`, the
  `config.local.*` forms and the YAML or JSON form of `mcp.*`). Any command that finds one stops with `YAML and JSON
  configs are no longer read ...: run ai-rulez migrate v5`. *Migrate:* converts them to TOML. The YAML frontmatter of
  markdown content is unaffected.
- **Separate `mcp.toml`, `mcp.yaml`, `mcp.json` files are no longer read.** *Migrate:* merges them into
  `config.toml`. `ai-rules-mcp.schema.json` is removed.
- **`init --format yaml|json` is removed**; `init` writes `config.toml`.
- **`migrate v4` is removed.** `migrate` takes one target, `v5`.
- **`[lint.budget]` is now `[lint.ratchet]`** (and `ratchet_exceeded` in the JSON report, `Over ratchet` in
  Markdown). It never meant a size limit; `[lint.budgets.<kind>]` is. *Migrate:* renames the table.

*Before:*

```toml
version = "4.0"

[lint.budget]
AR401 = 3
```

*After `ai-rulez migrate v5`:*

```toml
version = "5.0"
# Pinned by `ai-rulez migrate v5`: the 4.x default. Remove this line to take the v5 default.
agents_md = false
# Pinned by `ai-rulez migrate v5`: the 4.x default. Remove this line to take the v5 default.
gitignore = true

[lint.ratchet]
AR401 = 3

# Pinned by `ai-rulez migrate v5`: the 4.x default. Remove this line to take the v5 default.
[header]
hashes = "full"
```

### Defaults

- **`agents_md = true` by default.** `AGENTS.md` is the single canonical instruction file (root and nested
  scopes); `CLAUDE.md` is a shim that imports `@AGENTS.md`; skills go to `.agents/skills` where the harness reads
  them. Set `agents_md = false` for the old per-harness files. *Migrate:* pins `agents_md = false` unless you pass
  `--adopt-defaults`.
- **Generated headers carry only the per-file `Content-Hash`.** The project-wide `Source-Hash` line, which
  rewrote every generated file on any edit, is gone by default. `[header] hashes = "full"` brings it back and
  `"none"` drops both. *Migrate:* pins `hashes = "full"`.
- **The managed `.gitignore` block is opt-in.** `gitignore` defaults to off, because committed outputs should not
  flip-flop in and out of the ignore block. Machine-local outputs (`config.local.*`, `local/` content) are still
  excluded automatically. `generate --gitignore` or `gitignore = true` turns the block on. *Migrate:* pins
  `gitignore = true`.

### Commands and flags

- **`validate` runs the content checks by default.** What `validate --strict` did is now plain `validate`
  (globs that match nothing, dead links, missing hooks, oversize content, the security rules). `--config-only`
  keeps the old config-only behavior. `--strict` now means *warnings fail*, the same as `--fail-on warning`, as it
  already did for `doctor` and `verifiers run`; it cannot be combined with `--config-only` or another `--fail-on`.
  A CI job that ran `ai-rulez validate` and passed can now exit 2 on findings: fix them, lower a rule with
  `[lint.severity]`, record them with `--update-baseline`, or pin the old check with `validate --config-only`.
  Replace `validate --strict` by `validate` in scripts (keep `--strict` only when warnings should fail).
- **`usage ...` and `report usage|evals` are folded into `telemetry ...`, with no aliases:**

  | 4.x | 5.0 |
  | --- | --- |
  | `ai-rulez usage hook` | `ai-rulez telemetry hook` |
  | `ai-rulez usage record` | `ai-rulez telemetry record` |
  | `ai-rulez usage feedback` | `ai-rulez telemetry feedback` |
  | `ai-rulez report usage <log>` | `ai-rulez telemetry report [log]` |
  | `ai-rulez report evals` | `ai-rulez telemetry report evals` |

  `telemetry record` handles skill loads and item loads in one command, so one hook block records both. Hook
  blocks already written into `.claude/settings.json` by hand run the old command and must be regenerated with
  `ai-rulez telemetry hook`. *Migrate:* rewrites the commands inside `config.toml` hooks and verifiers.
- **One flag vocabulary.** `--json` / `-j` is gone everywhere: use `--format json` (`--format text` is the
  default). `generate --no-fetch` / `-f` is now `--offline`, as on `mcp`. The confirmation skip of `clean` and of
  `remove` / `domain remove` / `profile remove` / `include remove` / `skill remove` is now `--yes` / `-y`
  (it was `--force`). `convert --force`, `eval run --force` and `import okf --force` (overwrite) are unchanged.
- **Removed deprecated flags:** `generate --update-gitignore` (use `--gitignore`), `--no-configure-cli-mcp` / `-M`
  and `--skip-cli-mcp` / `-S` (they had no effect).
- **The content commands report what they changed.** `add`, `remove`, `domain add|remove` and `edit` print the created,
  removed or rewritten path alone on a line of stdout (the "added successfully" sentence is on stderr and `-q`
  hides it), and `add`, `remove`, `edit`, `domain`, `profile`, `include` and `skill install|remove` take
  `--format json` for a `{"status", "type", "name", "path", ...}` document (schema `schema/change-result.schema.json`).
  *Migrate:* scripts that scraped the `INFO ... successfully` lines read the path from stdout or use `--format json`.
- **`show` and `edit` complete the verbs** (`list`, `show`, `add`, `edit`, `remove`): `ai-rulez show rule style`,
  `ai-rulez edit rule style --content ...`. They are the CLI side of the MCP `read_*` and `update_*` tools.
- **Group commands print help.** `ai-rulez list` with no subcommand used to exit 1 with "specify what to list"; like
  `add`, `remove`, `domain`, `profile`, `include`, `skill`, `builtins` and `migrate` it now prints its help and exits 0.
- **Stricter content names and values** (also for the MCP tools): a name must not end in `.md`, contain whitespace or
  start with `.` or `-`; `--targets` must be preset names, paths or globs (`--targets claude,bogus` is an error);
  `minimal` is a valid `--priority`, as the flag help always said. `remove` checks the item exists before it asks
  to confirm. `domain remove` refuses while a profile lists the domain (it used to leave a dangling profile).
  `add skill` without `--description` writes a placeholder that passes `validate` (it was the bare name, which
  failed AR802). *Migrate:* rename files that violate the rules; `validate` flags them.
- **`migrate` has real subcommands.** `ai-rulez migrate v5` and `ai-rulez migrate okf` are unchanged as command
  lines, but are now subcommands: `migrate` alone prints help, `migrate v4` or `migrate banana` is an unknown
  command, and `migrate v5` / `migrate okf` list in shell completion. `--dry-run`, `--check` and `--format` work
  before or after the target; `--adopt-defaults`, `--write` and `--recursive` belong to `v5` and `okf` rejects them.
  The spellings `migrate 5`, `migrate V5` and `migrate v5.0` are gone.
- **Removed aliases and dead flags.** The aliases `g` (of `generate`), `clear` (of `clean`), `v` and `check` (of
  `validate`) and `check` (of `list checks`) are removed; use the full names (`gen` and `val` stay). The hidden,
  never-implemented `mcp --transport`, `--address` and `--port` are removed (the server is stdio only).
- **`guard` is listed in `--help`.** It stays a hook the harness runs.
- **`init` prints the created directory** on stdout (and takes `--format json`); its file listing and next steps are
  on stderr, and the replace prompt is on stderr too. `init --config-dir` is the global flag.
- **MCP `init_project` honors `with_agents`** (it creates `agents/code-reviewer.md`; the flag was accepted and ignored).

- **Exit codes follow one contract**: 0 ok, 1 the command could not run (or the configuration is invalid or an older
  version), 2 findings or drift. See [the CLI reference](cli.md#exit-codes). `lock` keeps its documented codes.

### Output streams, errors and environment

- **Results go to stdout, diagnostics to stderr, and `-q` never hides a result.** `list rules|context|skills|agents|commands|checks`,
  `domain list`, `profile list`, `include list`, `skill list`, `clean --dry-run` and `lock --check` used to print through
  the logger on stderr, so `-q` erased them. They now print on stdout. *Migrate:* scripts that read stderr for these
  lists read stdout, or use `--format json`. `-q` now removes progress, information and success lines only; warnings,
  errors and hints stay (it used to hide warnings too).
- **One error rendering for every command.** `Error: <message>`, an optional `Validation errors:` list and
  `Hint: <hint>` on stderr. The `ERROR Failed to <verb> ... error=... hint=...` log-style errors of the `add`, `remove`,
  `list`, `domain`, `profile` and `include` family, the doubled `ERROR` plus `Error:` lines of `validate` and `scan`,
  and the bare messages of `review`, `eval run` and `telemetry enable` are gone. Under `--format json` a failure also
  writes `{"status": "error", "error": ..., "hint": ..., "exit_code": n}` to stdout (it carries `schema_version` like
  every document). *Migrate:* scripts that grep stderr for the old wording match `Error:`; scripts that parse stdout
  under `--format json` now always get a document.
- **Exit codes are unchanged** (`0` ok, `1` could not run, usage errors included, `2` findings or drift, `3` `lock` only),
  but are now produced in one place. A refused confirmation (`clean`, `remove`, `--yes` missing in a non-interactive
  shell) exits `1`, as before; `generate --user --clean` declining a prompt used to exit `0` and now exits `1`.
- **Confirmation prompts are written to stderr**, not stdout, so a piped stdout is never polluted.
- **Unprefixed environment variables are ignored.** `DEBUG`, `QUIET` and `VERBOSE` used to change the CLI's output
  (they were read by an unprefixed `AutomaticEnv`). Use `AI_RULEZ_DEBUG=1` / `AI_RULEZ_QUIET=1`.
  `AI_RULEZ_*` variables are listed in [the CLI reference](cli.md#environment-variables).
- **`~/.ai-rulez.{toml,yaml,json}` and `./.ai-rulez.*` are no longer read** by the root command. They were never used
  for settings; the lookup only printed "Using config file".
- **`--verbose` / `-V` is removed** (it did nothing beyond that line). Use `--debug` / `-D`.
- **`--config-dir` is a global flag.** Commands that declare their own `--config-dir` / `-n` keep it. `init` and
  `migrate` no longer declare one: they read the global flag. The content commands (`add`, `remove`, `show`, `edit`,
  `list`, `domain`, `profile`, `include`, `skill`) honor the global `--config-dir <name>` and `-C <dir>` (they used to
  ignore both and only auto-detect `.ai-rulez`, then `.config/ai-rulez`). *Migrate:* a script that passed
  `--config-dir` to one of them and relied on it being ignored now acts on that directory.
- **`--format text|json` on more commands:** `generate` (`--check`, `--dry-run`, the summary), `clean`, `sign`,
  `export okf` and `version`.

### Outputs

- **`[[plugins]]` no longer writes `.claude/plugins.json` or `.codex/plugins.json`.** No tool read them. The table
  is still accepted and `generate` warns that it has no effect. Use `[claude.settings] manage = true` with
  `enable_plugins` for Claude Code and `[plugins."name@marketplace"] enabled = true` in `.codex/config.toml`
  for Codex. *Migrate:* warns; delete the files it left behind.
- **Every `--format json` document carries `schema_version`.** List commands (`list`, `domain list`,
  `profile list`, `include list`, `skill list`, `builtins list`) and multi-profile `tokens` now return
  `{"schema_version": 1, "items": [...]}` instead of a bare array. Schemas: `schema/validate-report`, `cost-report`,
  `telemetry-doctor`, `eval-report`, `okf-validate`, `catalog`, `roles-manifest`, `lock-diff` and `convert-report`
  (`*.schema.json`). A change to a document's shape bumps its `schema_version`.
- **`ai-rulez.lock`, `[lock] enforce`, `http://` sources, `scan_imports`** and the trust model: see the sections
  below.

## Other changes

The changes below came with the same release; none is touched by `migrate v5`.

| Change | Action |
| ------ | ------ |
| Go module is `github.com/Goldziher/ai-rulez/v5` and the entry point is `cmd/ai-rulez` | `go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest`; update Go imports |
| `windsurf` is renamed `devin`; `continue-dev` is removed | Rename or remove the preset, delete old outputs |
| `[lock] enforce` is on whenever `ai-rulez.lock` exists | Commit a current lock, or set `enforce = false` |
| The lock `tree` digest, pinned sources and frontmatter hook scripts changed | Run `ai-rulez lock` once |
| Exit codes follow one contract, `lock` adds `3` | Update scripts that match exit codes |
| `lock --check` without a lock file exits 1 | Run `ai-rulez lock` first, or expect 1 |
| `http://` and `git://` remotes are rejected | Switch to `https://` or `ssh://` |
| The git token goes only to allowlisted hosts | Set `AI_RULEZ_GIT_TOKEN_HOSTS` for non-GitHub hosts |
| A committed config cannot reach outside the project | Move such paths to `config.local.toml` or the user config |
| Symlinked content must resolve inside the project | Replace links that leave the project |
| `scan_imports` is on by default | Fix findings, or set `scan_imports = "off"` |
| `scan` runs only the `security` analyzer | Use `validate` for the other findings |
| Claude MCP servers are written only to `.mcp.json` | None; `.claude/settings.json` loses `mcpServers` |
| `[telemetry] service_name` is user scope only | Move it to the user config or the environment |
| Eval results are signed per user | Use `eval run --force` in CI |
| Usage log is version 3 | None; older logs still read |
| `schema/catalog.schema.json` is catalog version 2 | Point validators of version 1 at `schema/catalog.v1.schema.json` |
| `generate` warns about unknown config keys and new commands | Fix the keys; set `--yes` or `AI_RULEZ_ACK_COMMANDS=1` in CI |
| Staged scanners are confined under `isolation = "auto"` (the default) wherever a backend works | A scanner that writes outside its scratch directory now fails (`AR9E3`): point it at `TMPDIR`/`HOME`, or set `isolation = "none"`; see [Isolation](strict-validation.md#isolation) |
| Custom preset and provider output paths are validated | Remove `..`, absolute and `.git` paths |
| `lock` and `update` scan every remote tree they pin to something new | Fix error findings, or pass `--accept-findings` after reviewing them |
| `generate` refuses an existing file it did not write (a hand-written `CLAUDE.md`) and never writes through a symlinked output | Run `ai-rulez convert --write` to import the file (the first `generate` then replaces it), move it, or pass `generate --force`; see [Existing files](cli.md#existing-files-generate-will-not-overwrite) |
| `init --from` runs through `convert --write` | Expect one context item per root file (`convert --split-headings` splits it); see [`init --from`](cli.md#init---from) |
| `generate --check` reports `blocked:` for a shared file a machine-local input would change | Commit or drop the local change, or run `generate --allow-local-drift` |
| The forge client (release dates, review-linked approvals) has its own host allowlist | GitHub Enterprise: set `AI_RULEZ_FORGE_HOSTS` |
| An organization policy at the managed path is read automatically | None unless the machine has one; see [Organization policy](#organization-policy) |
| Review-linked approvals count only reviews of the final head by members or named approvers; `max_age` is a ceiling | Re-run `approve --from-github-review` after new pushes; see [Approvals](#approvals) |
| Go APIs under `internal/` changed; `pkg/airulez` is the supported API | See [Go API](#go-api) |
| The `compression` option is gone (it was a no-op since v3.13) | Delete it; a config that still sets it loads and `generate` warns about the unknown key, but `validate` and `generate --strict` fail |


## OKF is the format of `.ai-rulez/`

`.ai-rulez/` is becoming an [OKF](okf.md) bundle: each concept carries `type`, `title` and `x-ai-rulez` frontmatter and
every directory has an `index.md`. The current layout keeps loading during the deprecation window, so nothing breaks
and nothing needs to change on upgrade. To convert a project:

```bash
ai-rulez migrate okf --dry-run   # list what would change
ai-rulez migrate okf             # convert in place; run it again and nothing changes
ai-rulez generate --check        # the generated files are byte-identical
```

`migrate okf` moves the frontmatter of each rule, context file, skill, agent, command and check under
`x-ai-rulez.metadata`, adds `type` (`Decision` for rules, `Concept` for context, `Playbook` for skills, `Reference`
otherwise; an existing `type` or `title` is kept) and writes the `index.md` files. Bodies are not touched, and neither
are skill and command resources (`references/`, `scripts/`, `assets/`). A file with an unclosed frontmatter block is
skipped and reported. `--check` exits 2 while a tree still needs migration.

Things to know:

- `type`, `title` and `x-ai-rulez` are reserved frontmatter keys. A native file that used `type` or `title` as its own
  key no longer sees it as metadata.
- `index.md` and `log.md` that only list entries (headings, bullet links) are listings, not content. A rule that happens
  to be called `index` with prose in it is still a rule.
- `validate` runs `okf validate` on a tree that has a root `index.md` and reports the `AR9B*` findings, failing at
  `--fail-on` (default `error`).
- Not yet: `add`, `init` and the MCP CRUD tools still write the native layout, and a custom `title` is not restored by
  `export okf` from a migrated tree.

## `init --from`

`init --from` now runs `convert --write` with its sources: importer names (`auto`, `native`, `rulesync`, ...) or the
project paths it always took (`.claude`, `.cursor`, `CLAUDE.md`). It gets convert's scan, validation and lossiness
report. Differences from v4:

- A root file such as `CLAUDE.md` becomes one context item. Use
  `ai-rulez convert --split-headings` to split it.
- MCP files, hooks and permissions are imported too (hooks and `allow` rules as a commented block you review first).
- The sources are checked in a scratch directory first. An existing configuration directory is moved aside until the
  import has been written and is restored when the import fails, so a failed `init --from` leaves the old
  configuration in place.
- Symlinks in the imported repository are never followed, and files over 2 MiB are skipped.

`convert --fetch`, new in v5, reads remote rulesync and APM sources over `https://` only; ssh, scp-style, `file://` and
local sources are reported as `needs-action` instead of being cloned.

## Presets

| Change | Action |
| ------ | ------ |
| `windsurf` is renamed `devin`. The output directory `.windsurf/` is now `.devin/` and the agent frontmatter key `windsurf_model` is now `devin_model`. There is no alias. | Rename the preset in `config.toml`, rename `windsurf_model` keys in agent files, and delete the old `.windsurf/` outputs. |
| `continue-dev` is removed. It has no replacement. | Remove it from `presets`. `doctor` reports it as an error. |
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
  `qoder` next to a tool that reads `${VAR}` references in the shared `.mcp.json` (`claude`, `cursor`, `copilot`,
  `codebuddy`, `commandcode`, `reasonix`) also fails, naming both, instead of writing a resolved secret.
- **Claude MCP servers move out of `.claude/settings.json`.** Claude Code reads project MCP servers from
  `.mcp.json`, which references `${VAR}`; `.claude/settings.json` no longer receives `mcpServers`, so resolved env
  values do not land in a committed file. An entry an earlier version wrote there is removed on the next
  `generate` or `clean` while it is still the value ai-rulez wrote. The settings file is written only when
  `[claude.settings]` manage, `[[hooks]]`, `[permissions]` or `[claude.settings.managed]` apply.
- **`generate` warns about unknown config keys** in `config.toml` and `config.local.toml`, naming the nearest known
  key. `generate --strict` (or `AI_RULEZ_STRICT=1`) fails instead. `validate` fails on them as before.
- **`generate` summarises new or changed commands**: hook commands, command-based MCP servers, `[permissions]
  allow` rules, `[claude.settings.managed] env`, plugin enablement and `http`/`prompt` hooks that are new since the
  previous run on this machine (all of them in a fresh clone), printed even with `--quiet`. It only warns. Pass
  `--yes` or set `AI_RULEZ_ACK_COMMANDS=1` in CI to silence it.
- **`clean` keeps hand-edited generated files** with a warning; `clean --force` removes them. The committed
  `.ai-rulez/.generated-manifest.json` lists merged-document paths only; claims and digests live in the gitignored
  `.generated-manifest.local.json`. A forged committed manifest cannot make `generate` or `clean` delete or strip
  anything: a file is removed only when its own `Content-Hash` (or the local digest) proves ai-rulez wrote it.
- **Check outputs are committed, not gitignored.** Hosted reviewers read checks from the base branch, so the
  files under [Checks](checks.md) are never added to the managed `.gitignore` block. An entry an earlier version
  added is removed on the next `generate`.
- **`generate --dry-run` prints `unchanged:` and `edited:`** for files that need no write. Scripts that match
  `write-file:` for every file need updating.
- **`tokens` totals include the item listing.** `always` and `headline_always` now count the skill, command and
  agent listing, so `--budget` can fail where it passed. The previous figures are `always_legacy`,
  `conditional_legacy` and `headline_always_legacy`.
- **Skill `evals/` directories are not bundled into plugins** unless `[plugin] include_evals = true`.
- **Custom preset and provider paths are validated.** A custom preset `path`, a provider spec path
  (`root.file`, `outputs.*.dir`, sidecars), `okf.dir` and `marketplace.output_dir` are rejected when they contain
  `..`, are absolute or drive-qualified, or name `.git`, `.ai-rulez`, `.hg` or `.svn`. `generate` fails closed on any
  write outside the project or inside `.git`. A custom preset that writes a CI or tool-executed file
  (`.github/workflows/`, `Makefile`, ...) appears as `exec-file` in the command summary.

## Lock file and enforcement

- **`ai-rulez.lock` pins content, under one hashing scheme.** `lock` now also pins the ai-rulez version, every
  authored rule, context file, skill, agent, command, hook, role and settings source, the generated outputs and
  served skills, as `sha256:` digests. Remote includes, OKF includes, installed skills and skill sources use the
  same scheme (there is no second, older per-file hash). Scripts (`.sh`, `.py`, `.js`, ...) are hashed byte for byte:
  a changed line ending in a script is a changed digest. `lock --check` exits 2 on a lock without content pins,
  whatever `[lock] enforce` says. A lock with another `version` is refused with the instruction to run
  `ai-rulez lock` again. A lock that pins role outputs (`[roles]` `pin = true`) is written as `version = 2`, which an
  older v5 build refuses instead of misreading the role pins; a lock without role pins stays `version = 1`.
  See [Lock file](lockfile.md).
- **Run `ai-rulez lock` once after upgrading**, review the diff and commit the file. The lock reads as stale until
  you do, for these reasons:
  - The `tree` digest now also covers the `source`, `ref` and `path` of remote entries and the `view` of served
    entries, so relabeling or swapping entries is detected. A lock written by an earlier v5 build reports the tree
    digest as stale.
  - `lock` pins the project scripts run by agent, skill and command frontmatter `hooks`. Editing such a script makes
    `lock --check` exit 2; locks of items with frontmatter hook scripts need `lock` once.
  - Local-path includes are pinned as `local-include` items, and include skills are recorded as
    `include:<name>/<path>` instead of a machine-specific cache path.
  - Include and skill sources are recorded exactly as written in the config, and a `file://` source stays
    machine-independent. Locks written earlier keep working until the next `lock`.
- **Symlinks in a pinned tree are pinned by their link target.** A symlink inside an include, installed skill, skill
  source or OKF tree is never followed and no longer blocks locking: the link target string is part of the digest, so
  a retargeted link is detected. Junctions and other irregular entries are pinned by path only. A symlinked root
  directory is still refused. Trees without symlinks keep their digest. File modes digest by the owner execute bit
  only, as git records it.
- **`[lock] enforce` defaults to `true` whenever `ai-rulez.lock` exists.** Set `enforce = false` to opt out. A remote
  include or installed skill the lock does not cover makes `generate` fail (as `--locked` always did) and `AR010`
  an error, and an include that cannot be resolved is an error instead of a skipped warning. `generate --frozen` and
  `--locked` are unchanged: they require the lock whether or not enforcement is on.
- **`[[skills]]` is not a config key.** `skills` is the dynamic-loading table, so a `[[skills]]` array of tables stops the load with an error naming the line. Keep skills in `.ai-rulez/skills/<name>/SKILL.md`, install them with `ai-rulez skill install` (`[[installed_skills]]`), or point at a repository with `[[skill_sources]]`.
- **An include that cannot be resolved is an error, with or without a lock.** Before, an unreachable remote include
  (no network, a deleted repository, no cached copy) was a warning and `validate`, `doctor`, `generate` and
  `generate --check` exited `0` while rendering without it. They now exit `1` and name the include. `--no-fetch`
  keeps the old behaviour (a warning, the include skipped).
- **`[lock] enforce = true` is strict.** It makes `validate --strict` report `AR981` (source drift) and `AR982`
  (output drift), makes `generate --locked` fail on drift, and makes the skills server refuse a served skill that
  the lock does not pin or whose digest differs. A corrupt lock, a lock of another `version` or a source that cannot be
  snapshotted is an `AR981` finding, not a logged skip.
- **`generate --check` verifies authored content against an enforced lock**, like `--locked`.
- **The lock digest ignores the project-wide `Source-Hash` header**, so editing an unrelated file no longer changes
  the digest of a served skill.
- **A served skill the security scan refuses no longer stops `lock`.** It is left unpinned, `lock` exits `3`, and
  `lock --strict` restores the fail-without-writing behavior.
- **`lock` and `update` scan what they pin.** Every remote tree pinned to something new is scanned (`AR001`-`AR009`)
  first; an error finding refuses the pin (exit `2`, nothing written) unless `--accept-findings`.
- **`generate --check` classifies machine-local inputs like `generate`.** With a `config.local.*` overlay or `local/`
  content it no longer reports every output as `stale`. A shared file the local input would change, and that
  `generate` refuses to write (tracked or not ignored), is reported as `blocked: <path>` with exit `2`;
  `--allow-local-drift` accepts it.

## Exit codes

All commands follow one contract: `0` success, `1` the command could not run (invalid configuration, missing input,
tool error), `2` findings, drift or a failed gate, `3` only for `lock`. Scripts that matched `1` for a drift result
need to match `2`.

| Command | `0` | `1` | `2` | `3` |
| ------- | --- | --- | --- | --- |
| `generate` | Written | Failed to load, validate or generate (any root with `--recursive`) | `--check` found drift; `--locked`/`--frozen` source differs from the lock; recursive run where every failure is lock drift | |
| `validate` | Valid | Invalid configuration | `--strict`: findings at or above `--fail-on` | |
| `scan` | Clean | Cannot run | Findings at or above `--fail-on` | |
| `verify` | Verified | Cannot run (no manifest) | Files differ | |
| `lock` | Written or verified | Cannot run; `--check` with no `ai-rulez.lock`; unknown name | `--check` found drift (also a lock without content pins, or no lock under `enforce`); `--outdated` moved tag or unsatisfiable constraint; `--fail-on-outdated` | Lock written, served skills left unpinned by the scan |
| `update` | Done or nothing to do | Cannot run | A source was refused (`AR730`, `AR731`, `AR732`); nothing written | |
| `doctor` | No errors | Configuration does not load | An error (or, with `--strict`, a warning) | |
| `verifiers run` | None failed | Nothing failed but the run could not complete | A verifier failed at `--fail-on` | |
| `eval run` | All pass | Flags invalid | A skill failed its threshold, errored or has invalid cases | |
| `tokens`, `cost` | Within budget | Configuration cannot load | Over `--budget` or `--on-demand-budget` | |
| `convert` | Done | Cannot run, or would overwrite files | Blocked by the scan or validation, or `--fail-on` matched | |
| `export okf`, `import okf`, `okf validate` | Done | Cannot run | Drift (`--check`), lint findings, files not overwritten, or refused by the scan | |
| `search --eval` | Pass | Cannot run (`AR9D2`) | A gate failed (`AR9D4`) | |
| `scanners doctor` | Healthy | Configuration does not load or a name is unknown | A checked scanner is missing or misconfigured | |
| `guard` (hook) | Allowed | | The call edits a generated file | |

Unknown subcommands (`telemetry bogus`) exit `1`. When a command covers several roots
(`--recursive`, for `generate --check` and `lock`) the most severe code wins: `1`, then `2`, then `3`.

## Supply-chain defaults

- **Plain `http://` and `git://` remotes are rejected.** `git://` is unauthenticated and can be rewritten in
  transit. A remote include, OKF include, installed skill or skill source must use `https://`,
  `ssh://` / `git@host:path`, or a local `file://` URL or path. The error names the source and says to switch to
  `https://`; `include add` refuses them too. Include sources accept a leading `git+` (`git+https://host/org/repo`).
- **The git token goes only to allowlisted hosts.** `AI_RULEZ_GIT_TOKEN` / `--token` is sent to `github.com` by
  default, or to the hosts in `AI_RULEZ_GIT_TOKEN_HOSTS` (comma separated, environment only; when set it replaces the
  default, so list `github.com` too), as a host-scoped header rather than inside the URL, and only over `https://`.
  Set the variable to keep using a token with GitLab, Bitbucket or a self-hosted host; other hosts get no token and a
  warning. Caches written by earlier versions are scrubbed.
- **The forge token has its own allowlist.** The forge client (release dates for `min_release_age`, review-linked
  approvals) sends the GitHub token (`GITHUB_TOKEN`, `GH_TOKEN` or `gh auth token`) only to `github.com` or the hosts
  in `AI_RULEZ_FORGE_HOSTS` (comma separated, environment only), never to every `AI_RULEZ_GIT_TOKEN_HOSTS` host. For
  GitHub Enterprise Server, add its host to `AI_RULEZ_FORGE_HOSTS`. See [Forge client](forge.md).
- **A committed config cannot point outside the project.** A local include (`source` or `local_override`) that
  resolves outside the project after symlinks (`../victim`, an absolute path) is a fatal error. It is still allowed
  in `config.local.toml` and the user config. A local `[[skill_sources]]` `url` or `path` must resolve inside the
  project (`mcp --source <dir>` and the user config may point anywhere). `local_override` on an include or installed
  skill in the committed config is refused under `generate --locked`, `--frozen` and an enforced lock, because it
  bypasses the pins; set it in `config.local.toml`.
- **Imported content is scanned by default.** `[lint.security] scan_imports` is on when unset: `generate` scans
  includes and installed skills before writing anything and stops at an error-level finding. Skills from includes
  (and any skill file outside the project) are scanned at the strict level. Set `scan_imports = "off"` to opt out, or
  `"warn"` to log only.
- **Unpinned MCP packages (`AR012`)** stay a warning in `validate`, and are an error whenever `[lock] enforce` is on
  (whenever `ai-rulez.lock` exists, unless `enforce = false`), together with `AR010`.
- **Content symlinks follow one policy.** In the project's own `.ai-rulez/` (including domains, skill and command
  resources), a symlinked file or directory is followed only when its fully resolved target is inside the repository
  root (the git top level, else the directory holding `.ai-rulez`); the refusal names that root. An enclosing
  repository widens that root only when it tracks the project (its index holds `.ai-rulez/config.toml`): a project in a
  monorepo may link to its siblings, but a project that merely sits below a `$HOME` dotfiles repository is held to its
  own directory. `GIT_CEILING_DIRECTORIES` entries are resolved through symlinks, as git does. A project not yet added
  to its repository has its own directory as the root until `git add`. A symlinked `config.toml` or `config.local.toml`
  follows the same
  boundary: a target outside the root is a load error. Any other link used to be dropped silently; it is now
  refused with a warning that is shown even with `--quiet`, and `ai-rulez validate` reports it as an error. Symlinks
  in includes (git or local), installed skills, skill sources and OKF bundles are never followed and are skipped with
  a warning (an installed skill with a symlinked `SKILL.md` is refused). `init --from` never follows symlinks.
  Repository content read at load time is capped at 8 MiB per file; a larger file is an error.
- **`scan` is security-only.** `ai-rulez scan` runs only the `security` analyzer's checks. Hook and config findings
  (`AR504`, `AR9K0`, ...) that used to appear in its report belong to `validate --strict`.

## Scanner isolation

Staged scanners (`[[lint.external]]` with `inputs`) run confined under `isolation = "auto"`, the default, wherever a
backend works: macOS `sandbox-exec`, Linux `bwrap` or `unshare`. A confined scanner has no network (unless it declares
`egress = true`) and cannot write outside its scratch directory, so one that writes elsewhere now fails with `AR9E3`.
Point it at `TMPDIR`/`HOME`, or set `isolation = "none"` on the entry or in `[lint.scanner_policy]`. Without a backend,
`auto` runs unconfined and notes `AR9E7`; `isolation = "require"` refuses to run instead. See
[Isolation](strict-validation.md#isolation).

## Organization policy

v5 reads a tighten-only organization policy from outside the repository: `--policy`, `AI_RULEZ_POLICY`, or the
managed path (`/etc/ai-rulez/policy.toml`, `/Library/Application Support/ai-rulez/policy.toml`,
`%ProgramData%\ai-rulez\policy.toml`). Nothing changes without one. When one applies, a repository value that
loosens it is clamped and reported (`AR740`), and `generate` and `validate` refuse such a configuration unless
`--policy-mode warn`. See [Organization policy](policy.md).

## Approvals

Approvals (`ai-rulez approve`, `[governance]`) are new in v5. Pre-release v5 builds counted some approvals that no
longer count:

- **Review-linked approvals need the pull request's final head.** `approve --from-github-review` records a review only
  when it was made on the head the pull request ends with; a review of an earlier push does not count. Re-run it after
  new pushes.
- **Outsider approvals do not count.** A review counts only when the reviewer's `author_association` is `OWNER`,
  `MEMBER` or `COLLABORATOR`, or `approvers` or CODEOWNERS name them, so a drive-by approval on a public repository is
  ignored. The digest is recomputed from the files at the reviewed commit, not taken from the lock committed there.
- **`[governance] max_age` is a ceiling.** An approval stops counting (`AR712`) once `approved_at` plus `max_age` has
  passed, whatever its `expires` says, and `approve --expires` beyond it is refused.

See [Approvals](approvals.md).

## Go API

Packages under `internal/` are not a public API, and v5 changed several of them. Code that embeds ai-rulez should use
`github.com/Goldziher/ai-rulez/v5/pkg/airulez` (experimental, see [Embedding](embedding.md)):

- The process-wide preset registry is gone: `config.GetPresetGenerator`, `config.PresetRegistry`,
  `config.RegisterPreset` and `config.RegisterRulesDir` are removed; each generation carries its own registry.
- `config.LoadConfig` fails when the config declares includes or installed skills unless the load is given
  `config.WithResolvers` (or `config.WithoutRemote`).
- `config.SetPolicyEnforcer` is gone: the organization policy belongs to the load (`config.WithPolicy`).
- The V2/V3 helpers (`config.DetectConfigVersion`, `config.VersionDir`, `config.ConfigVersionV3`, `Config.IsV3`,
  `config.DecodeLegacyMCPFile`, `config.MigrateLocalOverlayToTOML`, `LocalOverlay.Format`) are removed.
- In `pkg/airulez`, the machine-local overlay (`config.local.toml`, `.ai-rulez/local/`) is read only with
  `Options.WithLocal`; `Options.WithoutLocal` is removed.

## Trust rule for `[llm]` and `[telemetry]`

`allow_network`, `base_url`, `api_key_env` and the price overrides of `[llm]`, and the egress keys of `[telemetry]`
(`allow_network`, `otlp_endpoint`, `headers_env`, `service_name`, `resource`, ...), are honoured only from the user
config file (`~/.config/ai-rulez/config.toml`) or the `AI_RULEZ_LLM_*` / `AI_RULEZ_TELEMETRY_*` variables. A
repository `config.toml` or `config.local.*` that sets them is ignored and reported by `llm doctor`,
`telemetry doctor`, `ai-rulez doctor` and the strict findings `AR9L1` and `AR9K1`. A literal key in either table is
`AR9L0` or `AR9K0`. If a repository relied on setting these, move them to the user config file. `[telemetry]
service_name` is the v5 addition to this list: it labels data on the user's collector, so a repository value is
ignored. See [LLM access](llm.md) and [Telemetry](telemetry.md).

The same rule now covers every egress or execution knob (scanner egress, eval execution, git credentials, hooks) and is
documented once, with a table of each knob's scope and precedence, in the [trust model](trust-model.md).

## Hooks, validation and the rule registry

- **`validate --strict` has more rules.** About thirty new codes (`AR304`, `AR305`, `AR403`, `AR504`-`AR507`,
  `AR601`, `AR602`, `AR805`-`AR807`, `AR963`, `AR964`, `AR971`-`AR982`, `AR9A0`-`AR9B9`, `AR9C*`, `AR9K*`, `AR9L*`
  and more) can fail a CI job that passed before. Every code is listed in [Strict validation](strict-validation.md);
  lower or turn off a rule with `[lint.severity]`, or accept current findings with
  `validate --strict --update-baseline`. Rule codes are stable and are never renumbered.
- **`[lint.budget]` is renamed `[lint.tolerate]`** (tolerated findings per rule), so it no longer reads like
  `[lint.budgets.<kind>]` (size budgets). `[lint.budget]` still works and warns. The report says
  `over its tolerated count of N`; the JSON key `budgets_exceeded` is unchanged.
- **Top-level `[[hooks]]` validation is stricter**: a `script` outside `A-Za-z0-9._/-` is rejected, a missing or
  non-executable script is `AR504`/`AR505`, and every generated shell line quotes the path.
- **Served skills (`delivery = "served"`) are left out of the harness skill trees** and reach the model through
  `ai-rulez mcp --serve-skills`; a static reference to one is `AR990`. Skills default to `static`, so nothing
  changes until you opt in.
- **Eval results are signed per user.** `eval run` signs each stored record with a per-user key
  (`eval-results.key`, in the user config directory). A record without a valid signature, such as one committed from
  another machine, is `unverified`: `eval run` re-runs it, and `AR997`, `AR998`, `report evals` and `report usage`
  ignore it. CI has no key; gate on `eval run --force`. See [Evals](evals.md).
- **Usage log version 3.** The `session` field is a salted hash of the harness session id, not the raw id, and lines
  carry `digest`, `digest_scheme` and `event_id` (the lock's skill digest). Version 1 and 2 logs still read.
- **`--json` is replaced by `--format json`.** Every command that printed JSON takes `--format text|json` (some add
  `sarif`, `junit`, `markdown`); `--json` stays as a hidden alias that warns. An unknown `--format` value is rejected
  with the allowed values. `lock --format` is accepted only with `--check`, `--diff`, `--outdated` or `--subject`.
- **`schema/catalog.schema.json` is catalog version 2.** The version 1 schema moved to
  `schema/catalog.v1.schema.json`; `catalog --format json` still prints version 1 unless `--schema-version 2`.

## Where to look

- [Supported harnesses](harnesses.md): the preset list and what each writes.
- [Hooks, permissions and settings keys](settings.md), [Permissions](permissions.md) and
  [User-level configuration](user-scope.md): top-level `[[hooks]]`, `[permissions]` and `generate --user`.
- [Lock file](lockfile.md) and [Trust model](trust-model.md): the lock scheme and what a repository may set.
- [Changelog](CHANGELOG.md#unreleased): the complete list for the unreleased version.
