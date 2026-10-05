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

- `--format, -f <toml|yaml|json>` — Config format (default `toml`)
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
- `--gitignore, -i` — Update `.gitignore` with generated output patterns
- `--config-dir, -n <name>` — Use a non-default config directory name instead of `.ai-rulez`
- `--recursive, -r` — Generate for every discovered config directory
- `--no-fetch, -f` — Use cached includes and skills without fetching
- `--env, -e KEY=VALUE` — MCP env override; repeatable
- `--env-file, -E <path>` — Dotenv file for MCP placeholders; repeatable
- `--no-configure-cli-mcp, -M` / `--skip-cli-mcp, -S` — Skip configuring CLI-based MCP tools
- `--plugin` — Generate distributable plugin bundles and a marketplace index from the `[plugin]` block
- `--if-configured` — With `--plugin`, skip successfully when plugin authoring is not configured

`--update-gitignore` remains as a hidden deprecated alias for `--gitignore`.

### `ai-rulez validate`

Check configuration and content structure for errors.

Flags:

- `--config-dir <name>` — Use a non-default config directory name instead of `.ai-rulez`

### `ai-rulez export okf` / `import okf` / `okf validate`

Open Knowledge Format (OKF v0.2) support, see `docs/okf.md`.

- `ai-rulez export okf [--out <dir>] [--profile <p>] [--include rules,context,skills,agents,commands,checks] [--check]` — write the content as a deterministic OKF bundle (default `okf.dir`, `docs/okf`); `--check` writes nothing and exits 2 on drift
- `ai-rulez import okf <dir|git-url[@ref][#subdir]> [--into rules|context|skills] [--domain <d>] [--dry-run] [--force]` — convert a bundle into `.ai-rulez/` sources; never overwrites without `--force`, scans imported text (AR001-AR011) first
- `ai-rulez okf validate <dir|git-url> [--format json] [--fail-on error|warning|info|none]` — lint any bundle (AR9B0-AR9B9)

### `ai-rulez migrate v4`

Convert V3 `.ai-rulez/` YAML configuration to V4 `.ai-rulez/` TOML configuration.

### `ai-rulez mcp`

Start the MCP server for AI assistant integrations.

## CRUD Commands

### Rules

- `ai-rulez add rule <name> [--domain <d>] [--priority <p>] [--content <c>]`
- `ai-rulez remove rule <name> [--domain <d>] [--force]`
- `ai-rulez list rules [--domain <d>] [--json]`

### Context

- `ai-rulez add context <name> [--domain <d>] [--priority <p>] [--content <c>]`
- `ai-rulez remove context <name> [--domain <d>] [--force]`
- `ai-rulez list context [--domain <d>] [--json]`

### Skills

- `ai-rulez add skill <name> [--domain <d>] [--description <desc>]`
- `ai-rulez remove skill <name> [--domain <d>] [--force]`
- `ai-rulez list skills [--domain <d>] [--json]`

### Domains

- `ai-rulez domain add <name> [--description <desc>]`
- `ai-rulez domain remove <name> [--force]`
- `ai-rulez domain list [--json]`

### Profiles

- `ai-rulez profile add <name> <domains...>`
- `ai-rulez profile add <name> <domains...> [--set-default, -s]`
- `ai-rulez profile remove <name>`
- `ai-rulez profile list [--json]`
- `ai-rulez profile set-default <name>`

### Includes

- `ai-rulez include add <name> <source> [--path <p>] [--ref <r>] [--include <types>] [--merge-strategy <s>] [--install-to <t>]`
- `ai-rulez include remove <name> [--force]`
- `ai-rulez include list [--json]`

### Installed Skills

- `ai-rulez skill install <name> --source <url> [--path <p>] [--ref <r>]`
- `ai-rulez skill remove <name> [--force]`
- `ai-rulez skill list [--json]`

## Roles, Lock and Catalog

- `ai-rulez roles list|show <name>|resolve <name> [--format json]` — Inspect `[[roles]]`; `list --format json` is the `roles.json` manifest
- `ai-rulez lock [--check] [--diff] [--format json] [--content-only]` — Pin remote includes, installed skills, authored content and outputs in `ai-rulez.lock`; `--check` exits 2 and names each difference
- `ai-rulez catalog --format json` — Items with owner, version, tokens, roles and lock status
- `ai-rulez tokens --role <name>` / `--by-role` — Token surface per role

## Other Commands

### `ai-rulez version`

Print version information.

### `ai-rulez builtins list`

List available built-in domains.
