---
type: Concept
title: Generation Pipeline
x-ai-rulez:
  kind: context
  id: generation-pipeline
  metadata:
    priority: high
    targets:
      - CLAUDE.md
      - .cursor/rules/*
      - GEMINI.md
      - AGENTS.md
      - .hermes.md
    summary: Config loading, merging, validation, preset rendering and writing, including the agents source, recursive runs and plugin bundles.
---

## Generation Pipeline

Pipeline: load -> merge -> validate -> render -> write. The full description is in `docs/architecture.md`.

- Load: `internal/project` wires the resolvers into `internal/config`, which reads `.ai-rulez/config.toml` (or `.agents/ai-rulez.toml`), the `default_layers` / `AI_RULEZ_LAYERS` `config.<name>.toml` layers, the machine-local overlay and the plugin and marketplace definition files, then scans the content tree and resolves includes, installed skills and builtin packs. All reads go through a `workspace` view.
- Source layouts: `.ai-rulez/` (an OKF bundle), `.agents/` plus `AGENTS.md` (`source = "agents"`, or implied by `.agents/ai-rulez.toml`), and an OKF bundle. With the agents source `internal/agentssource` adds the root `AGENTS.md` (one item, or one per `##` heading with `agents_md_split = true`) and every nested `AGENTS.md` (context scoped to its directory), and drops generated files found under `.agents`. `generate` never writes `AGENTS.md` or anything under `.agents` for this source.
- Render: `internal/generator` resolves the profile (or `--role`), collects MCP servers and renders each preset into a plan (`Plan`), then applies it with one of four appliers (disk, dry run, check, describe). Declarative provider specs (`internal/generator/providers/builtin/*.toml`) define 42 of the 52 harness presets (the `mcp` spec renders the MCP output); ten are Go presets in `internal/generator/presets`. `okf` and `llms-txt` are output presets outside that count.
- Settings files (`.claude/settings.json`, `opencode.json`, `.codex/config.toml`, ...) are merged document by document: ai-rulez owns the keys, array elements and map members it writes and preserves the rest, comments included (JSONC, TOML, YAML).
- Two presets writing the same path must render identical bytes; `generate` fails and names them otherwise.
- Write: the managed `.gitignore` block is maintained only when `gitignore = true` is set (or `generate --gitignore` is passed); `gitignore_exclude` keeps outputs you commit out of it. Machine-local outputs are always excluded. `generate --check` compares the plan with the disk (and the block) and exits 2 on drift; `--dry-run` previews; `--emit-plan` writes the plan as JSON.
- `agents_md` defaults to true: `AGENTS.md` is canonical and `CLAUDE.md` imports `@AGENTS.md`. Headers carry only the per-file `Content-Hash` unless `[header] hashes = "full"`.
- Recursive runs: `generate --recursive`, `generate --check --recursive` and `validate --recursive` process the roots concurrently, one worker per CPU at most; check and validate print results in root order with a `[n/N] path (elapsed)` line. With `gitignore = true` each git repository gets one managed block for its own roots; the block never depends on local files or `--no-local` (overlay outputs go to `.git/info/exclude`). A plugin-authoring root (plugin definitions, no preset) is built in plugin mode, skipped by `--check` (use `verify --plugin`) and never gets the self MCP server.
- Plugins: `generate --plugin` builds plugin bundles and marketplace indexes from `[plugin]` / `[marketplace]` or from `[[plugins_authoring]]` / `[[marketplaces_authoring]]` (inline or in `plugins/<name>.toml` and `marketplaces/<name>.toml`); `verify --plugin` checks them and `publish` packages them. Agent Plugins packages (`internal/agentplugins`), ARD manifests (`internal/ard`) and MCP Server Cards (`internal/mcpcard`) are emitted by `publish --emit`.

Rendering flow:

1. Load config and content tree.
2. Resolve profile (default or specified) or role.
3. Generate preset outputs (plus MCP output when servers exist).
4. Write files (and update the managed gitignore block when enabled).

Profile resolution, including the `default` profile, is described under Profiles and Domains.
