# CLI Reference

## Global Flags

- `--config <path>` — Override config discovery
- `--verbose, -V` — Enable verbose output
- `--debug, -D` — Enable debug output
- `--quiet, -q` — Suppress progress bars and non-essential output
- `--token, -T <token>` — Git access token for private repos (or `AI_RULEZ_GIT_TOKEN` env var)

## Core Commands

### `ai-rulez init`

Initialize `.ai-rulez/` directory with configuration.

Flags:

- `--domains, -d <names>` — Comma-separated initial domains
- `--skip-content, -s` — Skip example content files
- `--from, -F <source>` — Import from existing tool files, such as `auto`
- `--setup-hooks, -H` — Configure git hooks
- `--yes, -y` — Automatically answer yes to prompts
- `--config-dir <path>` — Directory to scaffold (default `.ai-rulez`; use `.config/ai-rulez` for the `.config/` convention)

### `ai-rulez generate`

Render outputs for all configured presets.

Flags:

- `--profile <name>` — Select a specific profile
- `--role <name>` — Generate the slice of content a `[[roles]]` entry selects (mutually exclusive with `--profile`; works with `--user`)
- `--locked` / `--frozen` — Require `ai-rulez.lock` to match remote sources and authored content (`--frozen` never uses the network); exit 2 on a source difference
- `--dry-run, -d` — Print planned directories, writes, and stale generated-file deletions without mutating files
- `--gitignore, -i` — Opt in to the managed `.gitignore` block with generated output patterns (off by default; same as `gitignore = true`)
- `--config-dir, -n <name>` — Use a non-default config directory name instead of `.ai-rulez`
- `--recursive, -r` — Generate for every discovered config directory
- `--offline` — Use cached includes and skills without fetching
- `--env, -e KEY=VALUE` — MCP env override; repeatable
- `--env-file, -E <path>` — Dotenv file for MCP placeholders; repeatable
- `--no-local` — Ignore the machine-local `config.local.*` overlay and `local/` content
- `--strict` — Fail on unknown or invalid config keys instead of warning (env `AI_RULEZ_STRICT=1`)
- `--yes, -y` — With `--user`, skip the confirmation; always, silence the summary of new hook and MCP commands (env `AI_RULEZ_ACK_COMMANDS=1`)
- `--plugin` — Generate distributable plugin bundles and a marketplace index from the `[plugin]` block
- `--if-configured` — With `--plugin`, skip successfully when plugin authoring is not configured
- `--watch, -w` — Generate, then regenerate on every change to `.ai-rulez/`, the local overlay and local includes (not combinable with `--dry-run`, `--check`, `--user`, `--plugin` or `--recursive`)
- `--check` — Render in memory and compare with the disk without writing; exits 2 on drift
- `--user` — Render the user config (`~/.config/ai-rulez`, or `--config <dir>`) into the per-user directories of each harness; `--yes, -y` skips the confirmation

`--update-gitignore` remains as a hidden deprecated alias for `--gitignore`; `--no-configure-cli-mcp, -M` and `--skip-cli-mcp, -S` are hidden no-ops.

Exit codes: `0` ok, `1` a config failed to load or generate, `2` `--check` found drift or `--locked`/`--frozen` found a source that differs from the lock.

### `ai-rulez clean`

Remove generated outputs. `--dry-run` previews, `--force` skips the prompt and also removes generated files edited by hand (otherwise kept with a warning), `--user` removes what `generate --user` wrote, `--profile`, `--keep-gitignore`, `--keep-manifest`.

### `ai-rulez validate`

Check configuration and content structure for errors. Exit `0` valid, `1` invalid or cannot load, `2` `--strict` findings at or above `--fail-on`.

Flags:

- `--config-dir <name>` — Use a non-default config directory name instead of `.ai-rulez`
- `--recursive, -r`, `--no-local`, `--repo-root <dir>`
- `--strict` — Deep content validation with stable finding codes; exits 2 on findings at or above `--fail-on`
- With `--strict` (or any `--format`): `--format text|json|sarif|github|junit|markdown`, `--output <file>`, `--fail-on error|warning|info|none`, `--analyzer <names>`, `--lint-profile default|strict|permissive`, `--external` and `--allow-egress <scanner>`, `--baseline <file>`, `--update-baseline --baseline-reason <text>`, `--strict-baseline`, `--since <rev>` / `--changed` with `--since-depth <n|all>` and `--since-max-files <n>`, `--fix` / `--fix-unsafe` with `--dry-run`
- `--explain <code>` — Print what a rule checks, why, examples and how to suppress it

### `ai-rulez scan`

Security checks only (`AR0xx`): secrets, hidden characters, prompt injection, risky shell, unpinned remotes. Same output, baseline and changed-only flags as `validate --strict`, plus `--external` for `[[lint.external]]` scanners (`--write-baseline --reason`, `--scanner-baseline`, `--show-suppressed`). `ai-rulez scanners list|doctor [--external] [--format json]` inspects those scanners.

### `ai-rulez doctor`

Read-only diagnostics with `error`, `warning` and `info` findings: config validity, removed presets (`windsurf` is
`devin`; `continue-dev` is gone), generated-output drift, generated paths git does not ignore, unresolved MCP
`${VAR}` placeholders, missing hook scripts, lock drift and preset tools missing from `PATH`. Flags: `--strict`,
`--format text|json`, `--profile`, `--no-local`.
Exits 2 on errors (and warnings with `--strict`), 1 when the configuration cannot be loaded.

### `ai-rulez verify`

Check generated files against their `Content-Hash` offline (`--plugin` for plugin bundles).

### `ai-rulez export okf` / `import okf` / `okf validate`

Open Knowledge Format (OKF v0.2) support, see `docs/okf.md`.

- `ai-rulez export okf [--out <dir>] [--profile <p>] [--include rules,context,skills,agents,commands,checks] [--check]` — write the content as a deterministic OKF bundle (default `okf.dir`, `docs/okf`); `--check` writes nothing and exits 2 on drift
- `ai-rulez import okf <dir|git-url[@ref][#subdir]> [--into rules|context|skills] [--domain <d>] [--dry-run] [--force]` — convert a bundle into `.ai-rulez/` sources; never overwrites without `--force`, scans imported text (AR001-AR011) first
- `ai-rulez okf validate <dir|git-url> [--format json] [--fail-on error|warning|info|none]` — lint any bundle (AR9B0-AR9B9)

### `ai-rulez convert`

Import another tool's files (`--from native`, `rulesync`, `skills-lock` or `auto`) into `.ai-rulez/` with a lossiness report (`mapped`, `approximated`, `dropped`, `needs-action`, `unsupported`). Writes nothing without `--write`; flags `--dry-run`, `--force`, `--domain`, `--format text|json`, `--report`, `--fail-on`, `--allow-findings`, `--list`.

### `ai-rulez mcp`

Start the MCP server for AI assistant integrations. `ai-rulez mcp --serve-skills` serves skills read-only (`find_skill`, `load_skill`, `list_skill_resources`): `--profile`, `--role`, `--targets`, `--domain`, `--allow`, `--deny`, `--source`, `--include-static`, `--frozen`, `--offline`, `--budget-bytes`, `--usage-log`, `--usage-sink`, `--no-watch`, `--reload-interval`.

### `ai-rulez guard`

Hidden PreToolUse hook added by `[guard] generated = true`: exits 2 when a tool call edits a generated file, 0 otherwise (fails open).

## CRUD Commands

### Rules

- `ai-rulez add rule <name> [--domain <d>] [--priority <p>] [--content <c>]`
- `ai-rulez remove rule <name> [--domain <d>] [--force]`
- `ai-rulez list rules [--domain <d>] [--format json]`

### Context

- `ai-rulez add context <name> [--domain <d>] [--priority <p>] [--content <c>]`
- `ai-rulez remove context <name> [--domain <d>] [--force]`
- `ai-rulez list context [--domain <d>] [--format json]`

### Skills

- `ai-rulez add skill <name> [--domain <d>] [--description <desc>]`
- `ai-rulez remove skill <name> [--domain <d>] [--force]`
- `ai-rulez list skills [--domain <d>] [--format json]`
- `ai-rulez list --placement [--profile <p>]` — where each skill and command ends up (core or plugin-only)

`add`, `remove` and `list` also take `--local` for the machine-local tree `.ai-rulez/local/`, and `agent`, `command` and `check` work like `rule`.

### Agents, Commands and Checks

- `ai-rulez add agent|command|check <name> [--domain <d>] [--description <desc>] [--content <c>]`
- `ai-rulez remove agent|command|check <name> [--domain <d>] [--force]`
- `ai-rulez list agents|commands|checks [--domain <d>] [--format json]`

Checks are code-review guidelines (`.ai-rulez/checks/<name>.md`) and have no `--local`.

### Domains

- `ai-rulez domain add <name> [--description <desc>]`
- `ai-rulez domain remove <name> [--force]`
- `ai-rulez domain list [--format json]`

### Profiles

- `ai-rulez profile add <name> <domains...>`
- `ai-rulez profile add <name> <domains...> [--set-default, -s]`
- `ai-rulez profile remove <name>`
- `ai-rulez profile list [--format json]`
- `ai-rulez profile set-default <name>`

### Includes

- `ai-rulez include add <name> <source> [--path <p>] [--ref <r>] [--include <types>] [--merge-strategy <s>] [--install-to <t>]`
- `ai-rulez include remove <name> [--force]`
- `ai-rulez include list [--format json]`

### Installed Skills

- `ai-rulez skill install <name> --source <url> [--path <p>] [--ref <r>]`
- `ai-rulez skill remove <name> [--force]`
- `ai-rulez skill list [--format json]`
- `ai-rulez skill update [name...]` — Re-pin installed skills in `ai-rulez.lock`

Commands that print JSON take `--format text|json`; `--json` is a hidden, deprecated alias.

## Roles, Lock and Catalog

- `ai-rulez roles list|show <name>|resolve <name> [--format json]` — Inspect `[[roles]]`; `list --format json` is the `roles.json` manifest
- `ai-rulez lock [name...]` — Pin remote includes, installed skills, skill sources, authored content and outputs in `ai-rulez.lock`. Flags: `--check` (offline, exit 2 and names each difference; exit 1 with no lock file), `--diff`, `--subject [--output <file>]`, `--outdated [--fail-on-outdated]`, `--content-only`, `--format text|json` (with `--check`, `--diff`, `--outdated`, `--subject`), `--kind include|skill|source|served`, `--profile`, `--role`, `--include-static`, `--source`, `--strict`, `--recursive`. Exit `3`: served skills left unpinned by the security scan
- `ai-rulez update [name...] [--dry-run] [--allow-downgrade] [--accept-moved-tag] [--kind include|skill|source] [--format json]` — Move `version` range pins to the newest allowed tag
- `ai-rulez catalog [--format json] [--schema-version 1|2]` — Items with owner, version, tokens, roles and lock status; `--html <dir>` writes a static site (`--role`, `--include-excerpt`, `--indexable`, `--clean`, `--base-title`, `--allow-findings AR001`)
- `ai-rulez sbom [--format cyclonedx] [--online] [-o <file>]` — CycloneDX 1.6 bill of materials
- `ai-rulez tokens [--role <name>] [--by-role] [--budget <n>] [--compare-profiles <p>] [--tokenizer ...] [--format json]` — Prompt-token surface per runtime; `ai-rulez cost [--target <preset>] [--top <n>] [--budget <n>] [--format text|json|markdown]` names the biggest offenders

## Verification, Search and Evals

- `ai-rulez verifiers run|list|explain|test` — Deterministic repo checks from `[[verifiers]]` and `.ai-rulez/verifiers/*.toml`; `run` takes `--since <rev>`, `--staged`, `--all`, `--rule`, `--name`, `--format text|json|sarif|junit`, `--out`, `--fail-on`, `--strict`, `--strict-applicability`
- `ai-rulez search <query> [--limit <n>] [--format json]` ranks the skills a skills server would serve (same selection flags as `mcp --serve-skills`); `search --eval <cases.yaml> [--k] [--min] [--baseline] [--max-flips] [--out]` measures the ranking
- `ai-rulez eval run [skill...]` — Run skill evals through a runner and score them (`--harness`, `--runner`, `--runner-command`, `--ablation`, `--dry-run`, `--max-cost`, `--changed-only`, `--force`, `--threshold`, `--format json|markdown|junit`); results are signed per user, so CI needs `--force`
- `ai-rulez usage hook|record|export|feedback`, `ai-rulez telemetry hook|record|flush|preview|doctor`, `ai-rulez report usage|evals` — Opt-in, identifier-only usage and item-load telemetry
- `ai-rulez llm doctor [--ping]` / `llm estimate <file>` — Inspect `[llm]` access without calling a model
- `ai-rulez local init|show|set|unset|path` — Manage the machine-local `config.local.*` overlay

## Exit Codes

`0` success, `1` the command could not run, `2` findings, drift or a failed gate (`validate --strict`, `scan`, `generate --check`, `verify`, `lock --check`, `doctor`, `verifiers`, `eval`, budgets, ...), `3` `lock` only: served skills left unpinned.

## Other Commands

### `ai-rulez version`

Print version information.

### `ai-rulez builtins list`

List available built-in domains.
