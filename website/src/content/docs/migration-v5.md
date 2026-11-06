---
title: "Migrating to v5"
description: "Upgrade a 4.x project to ai-rulez 5. ai-rulez migrate v5 rewrites the configuration for you; the breaking changes below are the ones you handle yourself."
---

Upgrade a 4.x project to ai-rulez 5. `ai-rulez migrate v5` rewrites the configuration for you; the breaking changes
below are the ones you handle yourself. The complete list is in the [changelog](https://github.com/Goldziher/ai-rulez/blob/main/CHANGELOG.md).

## Upgrade in four steps

1. Commit or stash your work, then preview the change list: `ai-rulez migrate v5 --dry-run`. Nothing is written.
2. Run `ai-rulez migrate v5`. It rewrites the project in place. Add `--recursive` for a repository with several
   `.ai-rulez/` directories.
3. Run `ai-rulez doctor`. It reports removed presets with their replacement, drifted outputs and generated paths git
   does not ignore.
4. Run `ai-rulez generate`, review the diff, and commit the sources and outputs together.

Then run `ai-rulez lock` once if the project has an `ai-rulez.lock` (see [Lock file](#lock-file)), and update CI
scripts for the [changed commands and flags](#commands-and-flags).

`migrate v5` reads only 4.x projects. Migrate a 2.x or 3.x project to 4.0 with ai-rulez 4.x first
(`npx ai-rulez@4 migrate v4`), then run `ai-rulez migrate v5`. Until a project is migrated, every v5 command stops with
the next step:

```text
Error: config version "4.2" is no longer supported: run `ai-rulez migrate v5`
Error: config version "3.0" is no longer supported; install ai-rulez 4.x to migrate it to 4.0, then run `ai-rulez migrate v5`
```

## What `migrate v5` changes

A 4.x project:

```yaml
# .ai-rulez/config.yaml
# acme rules
version: "4.0"
name: acme
presets:
  - claude
  - windsurf
lint:
  budget:
    AR401: 3
```

```bash
ai-rulez migrate v5
```

```text
. (.ai-rulez): migrated
  [convert-format] .ai-rulez/config.yaml: converted to config.toml with keys, order and comments kept; config.yaml is removed
  [version] .ai-rulez/config.toml: version 4.0 -> 5.0
  [lint-ratchet] .ai-rulez/config.toml: [lint.budget] / [lint.tolerate] renamed to [lint.ratchet] (per-rule finding counts); size limits stay in [lint.budgets]
  [preset-rename] .ai-rulez/config.toml: preset windsurf renamed to devin (outputs move from .windsurf/ to .devin/; delete the old directory)
  [pin-default] .ai-rulez/config.toml: agents_md = false (v5 default: true, AGENTS.md is the canonical instruction file)
  [pin-default] .ai-rulez/config.toml: gitignore = true (v5 default: false, no managed .gitignore block)
  [pin-default] .ai-rulez/config.toml: [header] hashes = "full" (v5 default: "content", only the per-file Content-Hash)
  [pin-default] .ai-rulez/config.toml: [mcp] self_server = false (v5 default: true, generate adds the ai-rulez MCP server to .mcp.json)
migrated 1, unchanged 0, errors 0
```

The resulting `.ai-rulez/config.toml`:

```toml
# acme rules
version = "5.0"
name = "acme"
presets = ["claude", "devin"]
# Pinned by `ai-rulez migrate v5`: the 4.x default. Remove this line to take the v5 default.
agents_md = false
# Pinned by `ai-rulez migrate v5`: the 4.x default. Remove this line to take the v5 default.
gitignore = true

[lint]

[lint.ratchet]
AR401 = 3

# Pinned by `ai-rulez migrate v5`: the 4.x default. Remove this line to take the v5 default.
[header]
hashes = "full"

# Pinned by `ai-rulez migrate v5`: the 4.x default. Remove this line to take the v5 default.
[mcp]
self_server = false
```

Each change has a rule name, shown in brackets in the report:

| Rule | Rewrite |
| ---- | ------- |
| `version` | `version = "4.x"` becomes `version = "5.0"`; the rest of the line, including a comment, is kept. |
| `convert-format` | `config.yaml`, `config.yml` or `config.json` becomes `config.toml` with keys, order and comments kept, and the old file is removed. `$schema` becomes `schema`. |
| `local-overlay` | `config.local.yaml`, `.yml` or `.json` becomes `config.local.toml` (mode 0600). |
| `mcp-merge` | A separate `mcp.toml`, `mcp.yaml` or `mcp.json` is merged into `[[mcp_servers]]` in `config.toml` and removed. If `config.toml` already defines `mcp_servers`, the file is left in place with a warning; move the servers by hand. |
| `lint-ratchet` | `[lint.budget]` and `[lint.tolerate]` become `[lint.ratchet]` (per-rule finding counts). `[lint.budgets.<kind>]` (size limits) is unchanged. |
| `preset-rename` | The `windsurf` preset becomes `devin` and the removed `continue-dev` preset is dropped. With `--write`, `windsurf_model` in agent frontmatter becomes `devin_model`. |
| `skills-table` | A `[[skills]]` array is removed; skills in `.ai-rulez/skills/<name>/SKILL.md` are discovered without it. |
| `pin-default` | The four defaults that changed in v5 are pinned to their 4.x value so the generated output does not move: `agents_md = false`, `gitignore = true`, `[header] hashes = "full"` and `[mcp] self_server = false`. A key you already set is never touched. |
| `gitignore-block` | With `--adopt-defaults`, the managed `.gitignore` block is removed unless the configuration sets `gitignore = true`. |
| `command-rename` | `ai-rulez usage ...` and `ai-rulez report usage\|evals` inside hook, verifier and script commands become `ai-rulez telemetry ...`. |
| `frontmatter-alias` | `permission_mode` and `user_invocable` become `permissionMode` and `user-invocable`. Reported only; rewritten with `--write`. |

### Options

| Flag | Effect |
| ---- | ------ |
| `--dry-run`, `-n` | Print the change list; write nothing. |
| `--check` | Like `--dry-run`, and exit 2 while a project still needs migration. Use it in CI. |
| `--adopt-defaults` | Take the v5 defaults instead of pinning the 4.x ones. Regenerate and review the diff. |
| `--write` | Also rewrite frontmatter aliases in the Markdown sources. |
| `--recursive` | Migrate every project found below the selected directory. |
| `--format json` | A machine-readable report: `schema_version` and one entry per project with `status`, `changes`, `warnings` and `error`. |
| `-C <dir or file>` | Migrate the project at that configuration directory or file, for example `-C plugin/.ai-rulez`. |

The rewrite is text-level, so comments survive, and the result is loaded again before it is written; a file that would
not load is reported and left alone. Running `migrate v5` again on a migrated project changes nothing. Exit codes: `0`
migrated or nothing to do, `1` a project could not be migrated, `2` `--check` found work.

## Breaking changes

### Configuration files

- **Only `version = "5.0"` is accepted.**
- **Only TOML is read.** `config.yaml`, `config.yml`, `config.json`, their `config.local.*` forms and separate `mcp.*`
  files stop every command with
  ``Error: found <path>: YAML and JSON configs are no longer read; run `ai-rulez migrate v5` to convert it to config.toml``.
  Markdown frontmatter is still YAML.
- **`init --format yaml|json` is removed**; `init` writes `config.toml`.
- **`[lint.budget]` and `[lint.tolerate]` are refused**; the table is `[lint.ratchet]`. The JSON report key is
  `ratchet_exceeded`.
- **`[[skills]]` is not a configuration key.** `skills` is the `[skills]` dynamic-loading table. Keep skills in
  `.ai-rulez/skills/<name>/SKILL.md`, install them with `ai-rulez skill install`, or use `[[skill_sources]]`.
- **`[[plugins]]` has no effect** and `generate` warns about it. List the plugins to enable as
  `[[enabled_plugins]]` with `id = "name@marketplace"`; `generate` writes them to `.claude/settings.json` and
  `.codex/config.toml`. Delete any `.claude/plugins.json` or `.codex/plugins.json` an earlier version left.
- **The `compression` option is removed.** `generate` warns about it as an unknown key; `validate` and
  `generate --strict-config` fail.
- **`~/.ai-rulez.{toml,yaml,json}` and `./.ai-rulez.*` are not read.**

### Defaults

| Setting | v5 default | 4.x behavior | Keep the 4.x behavior with |
| ------- | ---------- | ------------ | -------------------------- |
| `agents_md` | `true`: `AGENTS.md` is the single instruction file, `CLAUDE.md` imports `@AGENTS.md`, skills go to `.agents/skills` where the tool reads them | One root file per tool | `agents_md = false` |
| `gitignore` | `false`: generated files are committed | Managed `.gitignore` block | `gitignore = true` |
| `[header] hashes` | `"content"`: only the per-file `Content-Hash` line | Also a project-wide `Source-Hash` line, which rewrote every file on any edit | `hashes = "full"` (`"none"` drops both) |
| `[mcp] self_server` | `true`: `generate` adds the ai-rulez MCP server to the MCP files it writes | Not added | `self_server = false`, or `generate --no-self-mcp` |
| `[lock] enforce` | `true` whenever `ai-rulez.lock` exists | Off | `enforce = false` |
| `[lint.security] scan_imports` | On: includes and installed skills are scanned before anything is written | Off | `scan_imports = "off"` (or `"warn"`) |

`migrate v5` pins the first four unless you pass `--adopt-defaults`. Machine-local files (`config.local.*`,
`.ai-rulez/local/`) are ignored by git whatever `gitignore` says.

### Commands and flags

- **`validate` runs the content checks by default**: dead links, globs that match nothing, missing hooks, oversize
  content and the security rules. A CI job that passed with `ai-rulez validate` can now exit 2. Fix the findings, lower
  a rule with `[lint.severity]`, record the current findings with `--update-baseline`, or keep the old configuration-only
  check with `validate --config-only`.
- **`--strict` means "warnings fail"** (`--fail-on warning`), and exists only on `validate`, `doctor` and
  `verifiers run`. Replace `validate --strict` with `validate` in scripts unless warnings should fail.
- **`verify` checks only signatures, approvals and plugin provenance** (`--attestation`, `--approvals`, `--self`,
  `--plugin`). Use `generate --check` to compare generated files with their sources; a bare `verify` exits 1 and says
  so.
- **`scan` runs only the security analyzer.** Hook and configuration findings are reported by `validate`.
- **`usage` and `report` are replaced by `telemetry`**, with no aliases. Regenerate hook blocks you wrote by hand
  with `ai-rulez telemetry hook`.

  | 4.x | 5.0 |
  | --- | --- |
  | `ai-rulez usage hook` | `ai-rulez telemetry hook` |
  | `ai-rulez usage record` | `ai-rulez telemetry record` (skill and item loads in one command) |
  | `ai-rulez usage feedback` | `ai-rulez telemetry feedback` |
  | `ai-rulez report usage <log>` | `ai-rulez telemetry report [log]` |
  | `ai-rulez report evals` | `ai-rulez telemetry report evals` |

- **Seven short flags, each with one meaning everywhere:** `-C` `--config`, `-D` `--debug`, `-T` `--token`, `-q`
  `--quiet`, `-y` `--yes`, `-n` `--dry-run` and `-o` `--output`. Every other short flag is removed; using one is an
  unknown-flag error.
- **Renamed and removed flags:**

  | 4.x | 5.0 |
  | --- | --- |
  | `--json`, `-j` | `--format json` |
  | `-n <dir>`, `--config-dir <dir>` on individual commands | `--config-dir <dir>`, a global flag with no short form |
  | `-d` (`--dry-run`) | `-n`, `--dry-run` |
  | `ai-rulez validate path/.ai-rulez` (positional configuration path, on every command) | `ai-rulez -C path/.ai-rulez validate` |
  | `generate --no-fetch`, `-f` | `generate --offline` |
  | `generate --strict` (unknown keys fail) | `generate --strict-config` (or `AI_RULEZ_STRICT=1`) |
  | `generate --update-gitignore` | `generate --gitignore` |
  | `clean --force`, `remove --force` and the other confirmation skips | `--yes`, `-y` |
  | `lock --strict` | `lock --refuse-findings` |
  | `sign --policy <file>` (sign a policy) | `sign --org-policy <file>` |
  | `sbom --format cyclonedx\|spdx-json` | `sbom --type cyclonedx\|spdx-json` |
  | `telemetry hook --format json\|toml` | `telemetry hook --syntax json\|toml` |
  | `export okf --out <dir>`, `eval run --out`, `eval import --out`, `publish emit --out` | `--output-dir <dir>` |
  | `review --out`, `search --out`, `verifiers run --out` and similar | `--output`, `-o` |
  | `--verbose`, `-V` | `--debug`, `-D` |
  | `--no-configure-cli-mcp`, `--skip-cli-mcp` | Removed (they had no effect) |
  | `mcp --transport`, `--address`, `--port` | Removed (the server is stdio only) |

  `convert --force`, `eval run --force` and `import okf --force` (overwrite or re-run) are unchanged. The `--policy`
  flags exist only on the commands that evaluate policy; `AI_RULEZ_POLICY` works everywhere. See
  [Flag conventions](/ai-rulez/cli/#flag-conventions).
- **Removed command aliases:** `g` (`generate`), `clear` (`clean`), `v` and `check` (`validate`) and `check`
  (`list checks`). `gen` and `val` remain.
- **`migrate` has subcommands**, `migrate v5` and `migrate okf`; `migrate` alone prints help and `migrate v4` is an
  unknown command. Group commands such as `list` print their help and exit 0 when given no subcommand.
- **Stricter content names**, also through the MCP tools: a name must not end in `.md`, contain whitespace or start with
  `.` or `-`; `--targets` must name presets, paths or globs. `domain remove` refuses while a profile lists the domain.
- **An unknown `builtins` entry fails `validate`** with the valid choices.
- **The content commands honor the global `--config-dir` and `-C`** (`add`, `remove`, `show`, `edit`, `list`, `domain`,
  `profile`, `include`, `skill`), so a script that passed one now acts on that directory.
- **`eval run` prints text by default**; pass `--format markdown` for the previous output.

### Output, errors and JSON

- **Results go to stdout, diagnostics to stderr.** Lists (`list`, `domain list`, `profile list`, `include list`,
  `skill list`), `clean --dry-run` and `lock --check` print on stdout; `-q` never hides them. `-q` hides progress and
  information but no longer hides warnings. Confirmation prompts are on stderr.
- **The content commands print the path they changed** on stdout (`add`, `remove`, `edit`, `domain add|remove`); the
  "added successfully" line is on stderr. They also take `--format json`.
- **One error format:** `Error: <message>`, an optional `Validation errors:` list and `Hint: <hint>` on stderr. Under
  `--format json`, a failure also prints an error document on stdout.
- **Every `--format json` document carries `schema_version`.** List commands and multi-profile `tokens` return
  `{"schema_version": 1, "items": [...]}` instead of a bare array; `publish verify --format json` always returns
  `{"schema_version": 1, "results": [...]}`. The schemas are in `schema/`; see [JSON contracts](/ai-rulez/cli/#json-contracts).
- **`schema/catalog.schema.json` describes catalog version 2.** Version 1 is `schema/catalog.v1.schema.json`, and
  `catalog --format json` prints version 1 unless you pass `--schema-version 2`.
- **Unprefixed environment variables are ignored.** Use `AI_RULEZ_DEBUG=1` and `AI_RULEZ_QUIET=1` instead of `DEBUG`
  and `QUIET`.

### Exit codes

Every command follows one contract: `0` success, `1` the command could not run, `2` findings, drift or a failed gate,
`3` only for `lock` (written, with served skills left unpinned). See [Exit codes](/ai-rulez/cli/#exit-codes) for each command.
Changes that affect scripts:

- `validate` exits 2 on content findings (see above).
- `lock --check` without a lock file exits 1; run `ai-rulez lock` first.
- Declining the `generate --user --clean` prompt exits 1.
- An include that cannot be resolved (no network, deleted repository, no cached copy) makes `validate`, `doctor` and
  `generate` exit 1 and name it. `generate --offline` skips it with a warning.

### Generation

- **`generate` refuses to overwrite a file it did not write**, such as a hand-written `CLAUDE.md`. Import it with
  `ai-rulez convert --write` (the next `generate` replaces it), move it, or pass `generate --force`. It never writes
  through a symlinked output.
- **Divergent shared outputs fail.** When two presets render different content to the same path, `generate` fails and
  names both. Make them agree, for example with `agents_md = true` or `[rules] mode = "inline"`.
- **Claude Code MCP servers are written only to `.mcp.json`**, which keeps `${VAR}` references; `.claude/settings.json`
  no longer receives `mcpServers`. An entry an earlier version wrote there is removed on the next `generate`.
- **`generate` warns about unknown configuration keys** and names the nearest known key; `--strict-config` fails
  instead.
- **`generate` lists new or changed commands** it writes into tool configuration (hooks, command-based MCP servers,
  `[permissions] allow` rules). It only warns; pass `--yes` or set `AI_RULEZ_ACK_COMMANDS=1` in CI.
- **`generate --dry-run` prints `unchanged:` and `edited:`** for files that need no write, besides `write-file:`.
- **`clean` keeps hand-edited generated files** with a warning; `clean --include-edited` removes them.
- **Check outputs are committed**: files generated from [checks](/ai-rulez/checks/) are never added to the managed
  `.gitignore` block.
- **`tokens` totals include the skill, command and agent listing**, so `--budget` can fail where it passed.
- **Skill `evals/` directories are not bundled into plugins** unless `[plugin] include_evals = true`.
- **Custom preset and provider output paths are validated**: `..`, absolute paths and paths naming `.git`,
  `.ai-rulez`, `.hg` or `.svn` are rejected.
- **`init --from` runs `convert --write`**: it takes importer names (`auto`, `native`, `rulesync`, ...) or project paths
  (`.claude`, `CLAUDE.md`), imports MCP files, hooks and permissions too, and turns a root file such as `CLAUDE.md` into
  one context item (`ai-rulez convert --split-headings` splits it by heading).

### Presets

| Change | Action |
| ------ | ------ |
| `windsurf` is renamed `devin`: outputs move from `.windsurf/` to `.devin/` and `windsurf_model` becomes `devin_model`. There is no alias. | `migrate v5` renames the preset; run it with `--write` for the frontmatter key, and delete `.windsurf/`. |
| `continue-dev` is removed, with no replacement. | `migrate v5` drops it; `doctor` reports it. |
| `codex` and `antigravity` write commands as skills (`.agents/skills/<id>/SKILL.md`). | None; the old files are removed on `generate`. |
| `codex` writes skills to `.agents/skills`. | Set `codex_skills_dir = ".codex/skills"` for the old location. |
| `cursor` user-level skills go to `~/.agents/skills`. | Run `generate --user`; `clean --user` removes the old copies. |
| `antigravity` no longer adds the ai-rulez MCP server itself. | None; `[mcp] self_server` (on by default) adds it. |
| `amp` no longer writes `.agents/agents`, which Amp does not read. | None. |
| `opencode` writes MCP servers as `mcp.<name>` with `enabled`. | None; the old `mcp.servers` entries are removed. |
| `zoocode` inlines rules into `AGENTS.md` instead of `.roo/rules`. | None. |

### Lock file

- **`ai-rulez.lock` pins content.** Besides remote sources, it pins the ai-rulez version, every authored rule, context
  file, skill, agent, command, hook and settings source, the generated outputs and served skills, all as `sha256:`
  digests. Run `ai-rulez lock` once after upgrading, review the diff and commit it; a lock from 4.x or an earlier v5
  build reads as stale until you do.
- **`[lock] enforce` is on whenever the lock exists.** A remote include or installed skill the lock does not cover
  makes `generate` fail, `AR010` and `AR012` become errors, and `validate` reports source and output drift (`AR981`,
  `AR982`). `generate --check` also verifies authored content against an enforced lock.
- **`lock` and `update` scan what they pin.** A remote tree pinned to something new is scanned first; an error finding
  refuses the pin unless you pass `--accept-findings`. A served skill the scan refuses is left unpinned and `lock`
  exits 3; `--refuse-findings` makes it fail instead.
- **`generate --check` reports `blocked: <path>`** for a shared file that a machine-local input would change;
  `--allow-local-drift` accepts it.

See [Lock file](/ai-rulez/lockfile/).

### Supply-chain defaults

- **`http://` and `git://` remotes are rejected.** Use `https://`, `ssh://` / `git@host:path`, or a local path.
- **The git token is sent only to allowlisted hosts**: `github.com` by default, or the hosts in
  `AI_RULEZ_GIT_TOKEN_HOSTS` (which replaces the default, so list `github.com` too). The forge client (release dates,
  review-linked approvals) has its own allowlist, `AI_RULEZ_FORGE_HOSTS`; add your GitHub Enterprise host there. See
  [Forge client](/ai-rulez/forge/).
- **A committed configuration cannot point outside the project.** A local include or `[[skill_sources]]` path that
  resolves outside the project is an error. Put such paths in `config.local.toml` or the user configuration.
- **Symlinked content must resolve inside the repository.** Other links are refused with a warning, and `validate`
  reports them. Symlinks inside includes, installed skills, skill sources and OKF bundles are never followed.
- **Repository content read at load time is capped at 8 MiB per file.**
- **Staged external scanners run confined** under `isolation = "auto"` where a sandbox backend exists. A scanner that
  writes outside its scratch directory fails with `AR9E3`; point it at `TMPDIR`/`HOME` or set `isolation = "none"`. See
  [Isolation](/ai-rulez/strict-validation/#isolation).
- **An organization policy is read from the managed path** (`/etc/ai-rulez/policy.toml`,
  `/Library/Application Support/ai-rulez/policy.toml`, `%ProgramData%\ai-rulez\policy.toml`) besides `--policy` and
  `AI_RULEZ_POLICY`. Nothing changes on a machine without one. See [Organization policy](/ai-rulez/policy/).

### Egress settings come from the user, not the repository

`allow_network`, `base_url`, `api_key_env` and the price overrides of `[llm]`, and the egress keys of `[telemetry]`
(`allow_network`, `otlp_endpoint`, `headers_env`, `service_name`, `resource`, ...), are honored only from the user
configuration (`~/.config/ai-rulez/config.toml`) or the `AI_RULEZ_LLM_*` / `AI_RULEZ_TELEMETRY_*` environment
variables. A repository that sets them is ignored and reported (`AR9L1`, `AR9K1`). Move these settings to your user
configuration. See [Trust model](/ai-rulez/trust-model/).

### Validation rules

- **New rule codes** can fail a CI job that passed on 4.x. Every code is listed in
  [Strict validation](/ai-rulez/strict-validation/). Lower or disable a rule with `[lint.severity]`, or accept the current
  findings with `validate --update-baseline`. Rule codes are never renumbered.
- **Top-level `[[hooks]]` are stricter**: a `script` path outside `A-Za-z0-9._/-` is rejected, and a missing or
  non-executable script is `AR504`/`AR505`.
- **Eval results are signed per user.** A record without a valid signature, such as one committed from another
  machine, is re-run by `eval run`. In CI, use `eval run --force`. See [Evals](/ai-rulez/evals/).
- **The usage log is version 3**: the session field is a salted hash. Version 1 and 2 logs are still read.

## OKF is the format of `.ai-rulez/`

`.ai-rulez/` is an [OKF](/ai-rulez/okf/) bundle: each content file carries a `type` (and usually `title` and `x-ai-rulez`)
in its frontmatter and each directory has an `index.md`. `init`, `add` and the MCP tools write this layout. A migrated 4.x tree keeps loading
in the earlier layout, with a deprecation notice. Convert it with:

```bash
ai-rulez migrate okf --dry-run   # list what would change
ai-rulez migrate okf             # convert in place
ai-rulez generate --check        # the generated files are unchanged
```

```text
[converted] rules/style.md
[index] index.md
[index] rules/index.md
would convert 1, indexes 2, unchanged 0, skipped 0
```

`migrate okf` moves each file's frontmatter under `x-ai-rulez.metadata`, adds `type` (`Decision` for rules, `Concept` for
context, `Playbook` for skills, `Reference` otherwise) and `title` unless they exist, and writes the `index.md` files.
Bodies and skill resources (`references/`, `scripts/`, `assets/`) are not touched. A file with an unclosed frontmatter
block is skipped and reported. `--check` exits 2 while a tree still needs migration. The originals of rewritten files
are copied to `.ai-rulez.bak-okf-<timestamp>/` at the project root; delete it once you have reviewed the result, and
do not commit it.

After the conversion:

- `type`, `title` and `x-ai-rulez` are reserved frontmatter keys.
- A file you add by hand should have a `type` (`title` is optional). Frontmatter without `type` is an `AR9B1` error; a
  file with no frontmatter still loads, with an `AR9B1` warning. `generate` refreshes the `index.md`
  listings.
- `validate` also runs the OKF checks (`AR9B*`).

## Go API

Code that embeds ai-rulez should use `github.com/Goldziher/ai-rulez/v5/pkg/airulez` (experimental; see
[Embedding](/ai-rulez/embedding/)). The module path is
`github.com/Goldziher/ai-rulez/v5` and the binary is `cmd/ai-rulez`:

```bash
go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest
```

In `pkg/airulez`, the machine-local overlay is read only with `Options.WithLocal`; `Options.WithoutLocal` is removed.

## See also

- [Supported harnesses](/ai-rulez/harnesses/): what each preset writes.
- [Hooks and settings](/ai-rulez/settings/) and [User-level configuration](/ai-rulez/user-scope/).
- [Lock file](/ai-rulez/lockfile/) and [Trust model](/ai-rulez/trust-model/).
- [Changelog](https://github.com/Goldziher/ai-rulez/blob/main/CHANGELOG.md): the complete list of changes in 5.0.0.
