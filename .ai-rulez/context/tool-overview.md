---
type: Concept
title: Tool Overview
x-ai-rulez:
  kind: context
  id: tool-overview
  metadata:
    priority: high
    targets:
      - CLAUDE.md
      - GEMINI.md
      - .cursor/rules/*
      - .devin/*
      - .github/copilot-instructions.md
      - AGENTS.md
      - .hermes.md
    summary: High-level overview of the lifecycle, source layouts, presets, validation, governance and typical AI-Rulez workflows.
---

## AI-Rulez Overview

AI-Rulez is a standards-compliant lifecycle tool for agent knowledge and capabilities. It keeps rules, context, skills,
agents, commands, hooks, permissions and MCP servers in one source of truth, `.ai-rulez/`, and takes them through
author -> generate -> bundle -> lint/validate -> govern -> publish. The source can also be the open standards themselves:
`source = "agents"` reads the root `AGENTS.md` (and nested ones) and `.agents/`.

- Author: `.ai-rulez/` markdown and `config.toml` (an OKF bundle), or `AGENTS.md` plus `.agents/` with the configuration in `.agents/ai-rulez.toml` (`docs/agents-source.md`); `agents_md_split = true` reads `AGENTS.md` as one item per `##` heading.
- Generate: native files for 52 built-in harness presets, including Claude Code, Cursor, Codex, Copilot, Gemini CLI,
  OpenCode, Devin, and Kilo (`docs/harnesses.md` has the per-preset feature matrix). With the agents source, `AGENTS.md` and `.agents/` are read and never written.
- Bundle and publish: plugin bundles and marketplace indexes (`generate --plugin`, from `[plugin]` or several `[[plugins_authoring]]` / `[[marketplaces_authoring]]` definitions), Agent Plugins, ARD manifests, MCP Server Cards and OKF bundles (`export okf`, `publish --emit`).
- Validate: `validate`, `scan` and `okf validate` with stable `AR` codes and each standard's own schema or rules. `validate` needs no config: without one it lints the files that follow a convention, and `--path`, `--glob` and `--outputs` widen what it checks. `judge` reviews agent configuration with models (verified quotes, an adjudicator, repo-aware governance), also without config.
- Govern: `ai-rulez.lock`, approvals, Sigstore signing, organization policy and SBOM.
- CI: the root `action.yml` is a composite GitHub Action that installs a sha256-verified binary (`uses: Goldziher/ai-rulez@<ref>`).
- Standards named as supported must be generated, validated and conformance-tested; `docs/standards.md` carries the
  pinned spec versions and test status, and the README marks unimplemented ones "planned". Do not claim conformance
  ahead of the matching implementation and test.

Key concepts:

- Config (`version = "5.0"`) lives in `.ai-rulez/config.toml` plus `rules/`, `context/`, `skills/`, `agents/`, `commands/`, and `checks/`. `config.<name>.toml` layers are merged over it (`default_layers`, `AI_RULEZ_LAYERS`), then the machine-local `config.local.toml`.
- Profiles select which domains (directories under `domains/` in the config directory) are included in a generation.
- Presets define output formats and paths; `ai-rulez generate` renders all configured presets. Most presets are declarative provider specs under `internal/generator/providers/builtin/*.toml`; ten are Go presets in `internal/generator/presets`.
- Includes let you merge shared rule sets into local content before generation.
- MCP server settings live inline in `.ai-rulez/config.toml` as `[[mcp_servers]]` entries and can be generated alongside presets; remote servers can be published as MCP Server Cards.
- `[[hooks]]` and `[permissions]` are written once and translated into each harness's native format (36 and 24 harnesses). A rule or hook a harness cannot express is skipped with a warning, never approximated.
- `checks` are code-review guidelines rendered for the review tools that read a repository file.
- `ai-rulez generate --user` renders a user config into per-user directories; settings files you edit by hand are merged key by key and keep their comments.
- `ai-rulez.lock` pins remote includes, installed skills, skill sources, authored content and generated outputs; it is enforced whenever it exists. `includes`, installed skills and skill sources accept `version = "^1.2"` ranges that `ai-rulez update` moves.
- `[[roles]]` select a slice of the content (`generate --role`); `delivery = "served"` skills are served on demand by `ai-rulez mcp --serve-skills` instead of written to the harness trees.
- `validate` and `scan` run deep content and security checks with stable `AR` codes; `[[verifiers]]` run deterministic repo checks; `sbom`, `catalog`, `tokens` and `cost` report what the configuration contains and costs.
- Monorepos: `--recursive` runs `generate`, `generate --check` and `validate` over every nested root concurrently, with ordered output and one managed `.gitignore` block when `gitignore = true`.
- v5 behaviour: the Go module is `github.com/Goldziher/ai-rulez/v5`; `http://` and `git://` remotes are rejected and the git token goes only to allowlisted hosts; a committed config cannot point outside the project; content symlinks must resolve inside the project; Claude MCP servers are written only to `.mcp.json`; exit codes are `0` ok, `1` could not run, `2` findings or drift, `3` unpinned served skills (`lock`).
- Removed or renamed presets: `windsurf` is `devin`; `continue-dev` is gone.

Typical workflow:

1. Update `.ai-rulez/` sources (config, rules, context, skills, agents, checks, domains).
2. Run `ai-rulez generate` (optionally `--profile`, `--dry-run` or `--watch`).
3. Commit `.ai-rulez/` and generated outputs together.

Migration:

- YAML and JSON configs and older config versions are not read in v5; `ai-rulez migrate v5` converts a 4.x project (2.x and 3.x go through ai-rulez 4.x first).
