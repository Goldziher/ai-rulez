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
    summary: The ai-rulez command set - generate, validate, judge, lock, publish, CRUD helpers, global flags and exit codes.
---

## CLI and CRUD Commands

Core commands:

- `ai-rulez init` initializes `.ai-rulez/` with optional presets or domains; `--source agents` writes `.agents/ai-rulez.toml` and a starter `AGENTS.md` instead.
- `ai-rulez generate` renders outputs; `--profile` selects domains, `--role` a role; `--dry-run` previews; `--watch` regenerates on change (including nested `AGENTS.md`); `--check` reports drift without writing; `--locked`/`--frozen` require `ai-rulez.lock` to match; `--strict-config` fails on unknown config keys; `--recursive` processes every nested root; `--plugin` builds plugin bundles and marketplace indexes; `--user` renders the user config into the home directory. It warns about unknown keys and prints new hook and MCP commands (`--yes` or `AI_RULEZ_ACK_COMMANDS=1` silences it).
- `ai-rulez clean` removes generated outputs (the inverse of `generate`); `--dry-run` previews, `--yes` skips the prompt (required in non-interactive shells), `--include-edited` also removes generated files edited by hand; `--user` removes what `generate --user` wrote.
- `ai-rulez validate` checks config and content structure (`--strict` adds deep content checks with `AR` codes, `--fix`, `--since`, `--baseline`). It needs no config: without one it lints the files that follow a convention (`AGENTS.md`, `CLAUDE.md`, `SKILL.md`, `.claude/`, `.agents/`, `.cursor/rules`, plugin manifests, hooks, MCP files) and writes nothing. `--path` and `--glob` lint named files, `--outputs` also lints the generated files, `--analyzer` runs selected analyzers. `ai-rulez scan` runs only the security checks.
- `ai-rulez judge` reviews agent configuration with security, quality and governance models, verified quotes, an adjudicator and computed scores. It attaches scoped repository conventions, needs `--allow-llm`, and `--estimate` sends nothing. The MCP tool also requires user consent.
- `ai-rulez doctor` runs read-only diagnostics (removed presets, drift, unresolved MCP placeholders, lock drift, missing tools, shadowed or unread `.agents` content); it exits 2 on errors.
- `ai-rulez verify` checks signatures, approvals and plugin provenance (`--attestation`, `--approvals`, `--self`, `--plugin`); drift is `generate --check`.
- `ai-rulez lock` pins remote sources, authored content and outputs in `ai-rulez.lock` (`--check`, `--diff`, `--subject`, `--outdated`); `ai-rulez update` moves `version` range pins; `skill update` re-pins installed skills.
- `ai-rulez publish` packages the plugin bundle into checksummed release artifacts (`--emit` runs emitters such as `agent-plugins`, `ard` and `mcp-server-card`); `sign` signs the lock, a bundle, a skill or an SBOM.
- `ai-rulez verifiers run|list|explain|test` runs the deterministic repo checks; `sbom`, `catalog` (`--html`), `tokens`, `cost`, `roles`, `search`, `review`, `rubric` and `eval run` inspect and score the configuration.
- `ai-rulez convert` imports existing tool files (native, rulesync, apm, tessl, agent-plugins, okf, skills-lock; `--into .agents` targets the agents layout); `export okf`, `import okf` and `okf validate` handle Open Knowledge Format bundles.
- `ai-rulez telemetry record|report|feedback|enable|export ...` is opt-in, identifier-only item-load and skill-usage telemetry; `ai-rulez llm doctor|estimate` is read-only LLM diagnostics; `guard` is the hidden PreToolUse hook behind `[guard] generated = true`.
- YAML and JSON configs (`config.yaml`, `config.json`, flat `ai-rulez.yaml`) and a `config.toml` older than version 5.0 are not read: loading fails with an error naming the file and `ai-rulez migrate v5`, which rewrites a 4.x project.
- `ai-rulez mcp` starts the MCP server (usually launched by the assistant); `--serve-skills` serves skills read-only.

Exit codes: `0` ok, `1` could not run, `2` findings, drift or a failed gate, `3` (`lock` only) served skills left unpinned.

CRUD helpers manage file-based content:

- `ai-rulez add rule|context|skill|agent|command|check` creates content files.
- `ai-rulez remove rule|context|skill|agent|command|check` removes content files; `show` prints and `edit` replaces one item. With `source = "agents"` and `agents_md_split = true` these address one `##` section of `AGENTS.md`; nested `AGENTS.md` files are edited by hand.
- `ai-rulez list rules|context|skills|agents|commands|checks` lists items (`--placement` shows core versus plugin-only).
- `ai-rulez domain add|remove|list` and `ai-rulez profile add|remove|set-default|list` manage scopes.
- `ai-rulez include add|remove|list` manages remote includes; `ai-rulez skill install|remove|list|update` manages installed skills.

Global flags:

- `--config` overrides config discovery; `--config-dir` names the config directory.
- `--debug` and `--quiet` control logging and progress. `--verbose` was removed; use `--debug`.
- `--format text|json` replaces the removed `--json`/`-j` on every command that prints JSON.

Config discovery checks for `.ai-rulez/config.toml` first, then `.config/ai-rulez/config.toml`, and `.agents/ai-rulez.toml` (which wins over `.ai-rulez/config.toml` when both exist).
