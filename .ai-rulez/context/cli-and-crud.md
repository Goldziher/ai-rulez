---
type: Concept
title: Cli And Crud
x-ai-rulez:
  kind: context
  id: cli-and-crud
  metadata:
    priority: high
    targets:
      - CLAUDE.md
      - GEMINI.md
      - .cursor/rules/*
      - .devin/*
      - AGENTS.md
      - .hermes.md
    summary: Core commands (init, generate, validate, lock, verifiers, mcp), exit codes and CRUD helpers for managing configuration.
---

## CLI and CRUD Commands

Core commands:

- `ai-rulez init` initializes `.ai-rulez/` with optional presets or domains.
- `ai-rulez generate` renders outputs; `--profile` selects domains, `--role` a role; `--dry-run` previews; `--watch` regenerates on change; `--check` reports drift without writing; `--locked`/`--frozen` require `ai-rulez.lock` to match; `--strict-config` fails on unknown config keys; `--user` renders the user config into the home directory. It warns about unknown keys and prints new hook and MCP commands (`--yes` or `AI_RULEZ_ACK_COMMANDS=1` silences it).
- `ai-rulez clean` removes generated outputs (the inverse of `generate`); `--dry-run` previews, `--force` skips the prompt and removes hand-edited files; `--user` removes what `generate --user` wrote.
- `ai-rulez validate` checks config and content structure (`--strict` adds deep content checks with `AR` codes, `--fix`, `--since`, `--baseline`); `ai-rulez scan` runs only the security checks.
- `ai-rulez doctor` runs read-only diagnostics (removed presets, drift, unresolved MCP placeholders, lock drift, missing tools); it exits 2 on errors.
- `ai-rulez verify` checks signatures, approvals and plugin provenance (`--attestation`, `--approvals`, `--self`, `--plugin`); drift is `generate --check`.
- `ai-rulez lock` pins remote sources, authored content and outputs in `ai-rulez.lock` (`--check`, `--diff`, `--subject`, `--outdated`); `ai-rulez update` moves `version` range pins; `skill update` re-pins installed skills.
- `ai-rulez verifiers run|list|explain|test` runs the deterministic repo checks; `ai-rulez sbom`, `catalog` (`--html`), `tokens`, `cost`, `roles`, `search` and `eval run` inspect and score the configuration.
- `ai-rulez convert` imports existing tool files (native, rulesync, skills-lock); `export okf`, `import okf` and `okf validate` handle Open Knowledge Format bundles.
- `ai-rulez usage`, `telemetry`, `report` and `llm` are opt-in, identifier-only usage telemetry and read-only LLM diagnostics; `guard` is the hidden PreToolUse hook behind `[guard] generated = true`.
- YAML and JSON configs (`config.yaml`, `config.json`, flat `ai-rulez.yaml`) and a `config.toml` older than version 5.0 are not read: loading fails with an error naming the file and `ai-rulez migrate v5`, which rewrites a 4.x project.
- `ai-rulez mcp` starts the MCP server (usually launched by the assistant); `--serve-skills` serves skills read-only.

Exit codes: `0` ok, `1` could not run, `2` findings, drift or a failed gate, `3` (`lock` only) served skills left unpinned.

CRUD helpers manage file-based content:

- `ai-rulez add rule|context|skill|agent|command|check` creates content files.
- `ai-rulez remove rule|context|skill|agent|command|check` removes content files.
- `ai-rulez list rules|context|skills|agents|commands|checks` lists items (`--placement` shows core versus plugin-only).
- `ai-rulez domain add|remove|list` and `ai-rulez profile add|remove|set-default|list` manage scopes.
- `ai-rulez include add|remove|list` manages remote includes; `ai-rulez skill install|remove|list|update` manages installed skills.

Global flags:

- `--config` overrides config discovery.
- `--debug`, `--verbose`, and `--quiet` control logging and progress.
- `--format text|json` replaces the removed `--json`/`-j` on every command that prints JSON.

Config discovery checks for `.ai-rulez/config.toml` first, then `.config/ai-rulez/config.toml`.
