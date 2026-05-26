# CLI Reference

## Global Flags

- `--config <path>` — Override config discovery
- `--verbose` — Enable verbose output
- `--debug` — Enable debug output
- `--quiet, -q` — Suppress progress bars and non-essential output
- `--token <token>` — Git access token for private repos (or `AI_RULEZ_GIT_TOKEN` env var)

## Core Commands

### `ai-rulez init`

Initialize `.ai-rulez/` directory with configuration.

Flags:

- `--preset <name>` — Use a preset template
- `--domain <name>` — Create initial domains

### `ai-rulez generate`

Render outputs for all configured presets.

Flags:

- `--profile <name>` — Select a specific profile
- `--dry-run` — Print planned directories, writes, and stale generated-file deletions without mutating files
- `--config-dir <name>` — Use a non-default config directory name instead of `.ai-rulez`
- `--recursive, -r` — Generate for every discovered config directory
- `--update-gitignore` — Force `.gitignore` updates for generated files

### `ai-rulez validate`

Check configuration and content structure for errors.

Flags:

- `--config-dir <name>` — Use a non-default config directory name instead of `.ai-rulez`

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
- `ai-rulez profile remove <name>`
- `ai-rulez profile list [--json]`

### Includes

- `ai-rulez include add <name> <source> [--path <p>] [--ref <r>] [--include <types>] [--merge-strategy <s>] [--install-to <t>]`
- `ai-rulez include remove <name> [--force]`
- `ai-rulez include list [--json]`

### Installed Skills

- `ai-rulez skill install <name> --source <url> [--path <p>] [--ref <r>]`
- `ai-rulez skill remove <name> [--force]`
- `ai-rulez skill list [--json]`

## Other Commands

### `ai-rulez version`

Print version information.

### `ai-rulez builtins list`

List available built-in domains.
