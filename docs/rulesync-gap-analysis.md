# Rulesync vs ai-rulez — capability and harness gap analysis

Source analysed: `dyoshikawa/rulesync` (clone at `/tmp/rulesync`, HEAD `f17ee8e`).
Our side: ai-rulez `4.24.x` at the time of writing.

Status: `continue-dev` has been **removed**, and the legacy `windsurf` preset has
been **renamed to `devin`** (Windsurf was rebranded to Devin upstream). The plan
below closes the remaining gaps against a more complete competitor — adopt any
harness that is **not** deprecated and identify capability gaps.

---

## 1. Mental model: rulesync is feature-first, we are preset-first

- **Rulesync** models **9 cross-cutting features** and, per feature, a flat
  editor of `*-<feature>.ts` generators — one per tool. The tool list is derived
  from the union of the feature tuples (`src/types/tool-target-tuples.ts`), so
  tools and features grow independently. **66 tool targets**, though many are
  plugin-packaging or legacy aliases.
  Features: `rules`, `commands`, `subagents`, `skills`, `mcp`, `ignore`,
  `hooks`, `permissions`, `checks` (+ `shared`).
- **ai-rulez** models **one preset per tool**, each preset rendering the content
  features it supports. We have **14 tool presets + `mcp`** and **5 content
  features**: rules, context, skills, agents, commands, MCP.
- Consequence: rulesync can say "Kilo Code supports rules+commands+subagents+
  skills+mcp+ignore+hooks+permissions+checks"; we can only express "this preset
  writes these paths". Our DSL (`provider.toml`) is genuinely close in spirit to
  their spec — good foundation to close gaps.

### Feature name mapping (ours → theirs)

| ai-rulez | rulesync | Notes |
| --- | --- | --- |
| rules | rules | ✅ parity in concept |
| context | (folded into rules) | rulesync has no separate "context" kind |
| skills | skills | ✅ |
| agents | subagents | ✅ (same idea, different dirs per tool) |
| commands | commands | ✅ |
| MCP servers | mcp | ✅ |
| — | **ignore** | deprecated upstream, superseded by permissions |
| — | **hooks** | ❌ we have none (only our own plugin hooks) |
| — | **permissions** | ❌ we have none |
| — | **checks** | ❌ we have none (code-review guidelines) |

---

## 2. Tool roster: what they have that we don't (modern & non-deprecated)

Rulesync targets **55 modern, active** tools. Our 14 (+mcp). The overlap is the
common set; the **missing harnesses** (modern, non-deprecated, not a plugin or
legacy alias) are the interesting list.

### 2.1 Already covered by us (overlap)

`claudecode`, `cursor`, `copilot`, `cline`, `cline`, `codexcli`, `opencode`,
`antigravity-ide`+`antigravity-cli` (our single `antigravity`), `junie`,
`hermesagent`, `amp`, `pi`, `zed`? (no — see below), `warp`? (no), `goose`? (no).
Also `windsurf` → they renamed to `devin`; our `devin` preset is the current name
of the same product.

> Note: rulesync **has no standalone `gemini` target** — Gemini lineage is served
> by `antigravity-*`. We have a dedicated `gemini` preset, so we are *ahead*
> there (assuming Gemini CLI still warrants a target).

### 2.2 Modern harnesses rulesync supports that we are MISSING

Ordered roughly by adoption/importance (jarvis = a coding agent people actually
use today, not a plugin/legacy alias):

| # | rulesync target | What it is | Their outputs (rules / skills / subagents / commands / mcp) | Priority |
| - | --- | --- | --- | --- |
| 1 | `goose` | Block's Goose CLI | `.goosehints` / `.goose/skills` / `.goose/agents` / `.goose/recipes` / `.goose/config.yaml` | **high** |
| 2 | `zed` | Zed editor AI | `.rules` (+ global `AGENTS.md`) / `.agents/skills` / — / — / `.zed/settings.json` | **high** |
| 3 | `warp` | Warp terminal agent | `AGENTS.md` / `.warp/skills` / — / `.warp/skills` / `.warp/.mcp.json` | **high** |
| 4 | `roo`→`zoocode` | Roo Code / Zoo Code (VS Code) | `AGENTS.md` + `.roo/rules` / `.roo/skills` / `.roomodes` / `.roo/commands` / `.roo/mcp.json` | **high** |
| 5 | `kilo` | Kilo Code (Roo fork, active) | `AGENTS.md` + `.kilo/rules` / `.kilo/skills` / `.kilo/agents` / `.kilo/commands` / `.kilo.jsonc` | **high** |
| 6 | `qwencode` | Alibaba Qwen Code | `QWEN.md` + `.qwen/rules` / `.qwen/skills` / `.qwen/agents` / `.qwen/commands` / `.qwen/settings.json` | medium |
| 7 | `kiro-cli`/`kiro-ide` | AWS Kiro | `AGENTS.md` + `.kiro/steering` / `.kiro/skills` / `.kiro/agents` / `.kiro/prompts` / `.kiro/mcp.json` | medium |
| 8 | `augmentcode` | Augment Code | `AGENTS.md` + `.augment/rules` / `.augment/skills` / `.augment/agents` / `.augment/commands` / `.augment/settings.json` | medium |
| 9 | `factorydroid` | Factory Droid | `AGENTS.md` / `.factory/skills` / `.factory/droids` / `.factory/commands` / `.factory/mcp.json` | medium |
| 10 | `crush` | Charm Crush | `CRUSH.md` + `.crush/…` / `.crush/skills` / — / — / `.crush.json` | medium |
| 11 | `copilotcli` | GitHub Copilot CLI | rules via `AGENTS.md` / `.github/skills` / `.copilot/agents` / — / `.github/mcp.json` | medium |
| 12 | `kimi-code` | Moonshot Kimi Code | `AGENTS.md` / `.kimi-code/skills` / `.kimi-code/agents` / — / `.kimi-code/mcp.json` | low |
| 13 | `qoder` | Alibaba Qoder | `AGENTS.md` + `.qoder/rules` / `.qoder/skills` / `.qoder/agents` / `.qoder/commands` / `.mcp.json` | low |
| 14 | `deepagents` | LangChain deepagents-cli | `AGENTS.md` / `.deepagents/skills` / `.deepagents/agents` / — / `.deepagents/.mcp.json` | low |
| 15 | `bob` | IBM Bob | `AGENTS.md` + `.bob/rules` / `.bob/skills` / `.bob/custom_modes.yaml` / `.bob/commands` / `.bob/mcp.json` | low |
| 16 | `amp` (we have) | — | — | — |
| 17 | `codebuddy` | Tencent CodeBuddy | `CODEBUDDY.md` + `.codebuddy/…` | low |
| 18 | `cortexcode` | Snowflake Cortex | `AGENTS.md` / `.cortex/skills` / `.cortex/agents` | low |
| 19 | `rovodev` | Atlassian Rovo Dev | `.rovodev/…` | low |
| 20 | `takt` | Takt | `.takt/facets/…` | low |
| 21 | `trae` | ByteDance Trae | `.trae/rules`, `.trae/skills`, `.trae/mcp.json` | low |
| 22 | `vibe` | Mistral Vibe | `.vibe/…` | low |
| 23 | `zcode` | Z.ai ZCode | `.zcode/…` | low |
| 24 | `pool` | Poolside | `.poolside/settings.yaml` | low |
| 25 | `omp` | oh-my-pi | `.omp/…` | low |
| 26 | `mimocode` | OpenCode fork | `.mimocode/…` | low |
| 27 | `openclaw` | OpenClaw | `~/.openclaw/workspace/AGENTS.md` | low |
| 28 | `reasonix` | Reasonix | `REASONIX.md` + `.reasonix/…` | low |
| 29 | `replit` | Replit agent | `replit.md`, `.agents/skills` | low |
| 30 | `dsh` | DeepSeek Harness | `AGENTS.md`, `.dsh/skills` | low |
| 31 | `dsh`/`musecode`/`codebuff`/`commandcode`/`codewhale`/`grokcli`/`gitlabduo`/`lettacode`/`codebuddy`/`zcode`/`vibe`/… | long tail of newer/regional agents | — | low |

**Deprecated upstream (do NOT adopt):** `continue` (EOL — our `continue-dev`
preset, now removed), `roo` (EOL → use `zoocode`), `tabnine` (legacy
CLI; new Tabnine is an OpenCode distro), `kiro` (alias).

**Plugin/packaging targets (not a tool to add):** `*-plugin`,
`claudecode-legacy`, `augmentcode-legacy`, `agentsmd`, `agentsskills`.

---

## 3. Capability gaps (features, not tools)

These are the four whole **features** we don't have. Ranked by value:

### 3.1 `permissions` — allow/ask/deny policy (highest value)

Rulesync reads one canonical `.rulesync/permissions.jsonc` and renders each
tool's native allow/ask/deny surface:

| Tool | Our equivalent today | Theirs |
| --- | --- | --- |
| claudecode | none | `.claude/settings.json` `permissions.{allow,ask,deny}` |
| codexcli | none | `.codex/config.toml` |
| opencode | none | `opencode.json` `permission` |
| copilot | none | `.vscode/settings.json` `chat.tools.terminal.autoApprove` |
| cursor | none | `.cursor/cli.json` |
| amp | none | `.amp/settings.json` `amp.tools.disable` |
| pi | none | `.pi/settings.json` `defaultTools` |
| zed | none | `.zed/settings.json` `agent.tool_permissions` |

**Why it matters to us:** we already merge `.claude/settings.json`,
`opencode.json`, `.agents/settings.json`, `.amp/settings.json` (owning a couple
of keys). Adding a `permissions` feature is a natural extension of our existing
`jsonmerge.OwnedKey` machinery — we can own the `permissions` key the same way
we own `mcpServers`. `ignore` is deprecated upstream *in favour of permissions*,
so we should implement permissions, **not** ignore.

### 3.2 `hooks` — lifecycle event callbacks

Canonical `.rulesync/hooks.jsonc` with ~60 events and 6 handler types
(`command`, `prompt`, `http`, `agent`, `mcp_tool`, `function`), rendered to
`.claude/settings.json` `hooks`, `.cursor/hooks.json`, `.codex/hooks.json`,
`.github/hooks/*.json`, generated JS/TS plugins (opencode, amp, pi), etc.

**Gap assessment:** large surface and highly tool-specific. Likely **medium**
value for us now; a subset (`command` hooks into the settings documents we
already merge) would capture most real usage. Lower priority than permissions.

### 3.3 `checks` — code-review guideline files

`.rulesync/checks/*.md` → `BUGBOT.md` (cursor), `REVIEW.md` (kilo),
`.qwen/review-rules.md`, `.augment/code_review_guidelines.yaml`,
`.gitlab/duo/mr-review-instructions.yaml`, `.factory/skills/review-guidelines/SKILL.md`,
`.hermes/plugins/rulesync-checks/…`, `.takt/config.yaml`.

**Gap assessment:** niche (11 targets), but small to implement — it is just
"rules with a different destination". Could be modeled as a new **content kind**
(`checks`) routed per preset, or as rules with a `targets`-style output selector.
**Low/medium** priority.

### 3.4 `ignore` — per-tool ignore globs (DEPRECATED upstream)

`.cursorignore`, `.clineignore`, `.aiderignore`, `.geminiignore`, `.aiignore`,
`.zedignore`, `.rooignore`, `.qwenignore`, etc. Rulesync marks this
**deprecated, superseded by permissions**.

**Recommendation: skip.** Implement permissions instead (which also covers the
Claude `permissions.deny` rendering that ignore used to do).

### 3.5 Other rulesync capabilities worth noting (not tool features)

- **Plugin packaging** (`--output-roots`): generate a tool's components *inside*
  an existing plugin bundle (claude/augment/antigravity/zcode/vibe/devin/kimi
  plugins). We generate an OpenCode plugin; we do not package for these.
- **Simulated features** (`--simulate-commands/-subagents/-skills`): emit
  synthetic skills for tools that lack native commands/subagents. We have a
  partial analogue (commands-as-skills for claude).
- **Global mode** (`--global`, `~/.config/...`, `HERMES_HOME`, …): we write only
  project scope. Rulesync writes user-scope files too.
- **Shared-config JSONC comment-preserving edits**: we do this already for
  `opencode.json` (comments force a warning); rulesync edits in place preserving
  comments/key order — a robustness gap in our merge for JSONC files.
- **`fetch`/`install`/lockfile declarative sources**: we have `include add` /
  `skill` install; rulesync adds lockfiles (`rulesync.lock`, `.lock.json`).
- **`retire-targets`, watch mode, `doctor`, MCP server, `--check`/`--dry-run`**:
  we have `generate --dry-run`, `clean`, `tokens`, `verify` and an MCP server;
  we lack watch mode and `doctor`.

---

## 4. Recommended plan

### 4.1 Done: `continue-dev` removed; `windsurf` renamed to `devin`

`continue-dev` was removed (Rulesync removed/flagged Continue, which joined
Cursor 2026-06-18). The legacy `windsurf` preset was renamed to `devin`, matching
Windsurf's rebrand to Devin; there is no `windsurf` alias.

What changed (non-test):

- `internal/config/preset_names.go` — removed `PresetContinue`.
- `internal/config/types.go` — removed from `builtInPresets`.
- `internal/config/shared_outputs.go` — removed the consumer entry.
- `internal/config/validation.go` — removed any special mention.
- `internal/generator/presets/continue_dev.go` — deleted.
- `internal/generator/presets/effort.go` — dropped `continueDevPresetName` mapping.
- `internal/generator/presets/local_rules.go`, `skill_resources.go` — dropped refs.
- `internal/generator/providers/spec.go` — dropped from any enum/doc list.
- `internal/importer/importer.go` — dropped detection.
- `internal/mcp/handlers/{constants,project,utilities}.go` — dropped refs.
- `internal/templates/providers.go` — dropped refs.
- `internal/agents/{agents,chain}.go` — dropped refs.
- The `windsurf` preset, its `.windsurf` output directory and the `windsurf_model`
  frontmatter key were renamed to `devin`, `.devin` and `devin_model`.
- Tests + e2e fixtures: deleted `continue_dev_test.go` and every list entry.
- Docs/schemas/skills + `release/{npm,pypi}` descriptions.

`ai-rulez migrate` can warn on a config that still names `continue-dev` or the old
`windsurf` preset.

### 4.2 Close tool-harness gaps (adopt, in order)

Phase 1 (high adoption, straightforward to model):

1. `goose`
2. `zed`
3. `warp`
4. `roo`→`zoocode` (model as `zoocode`, note it supersedes `roo`)
5. `kilo`
6. `qwencode`

Phase 2 (medium):
7. `kiro-cli` (+ `kiro-ide`)
8. `augmentcode`
9. `factorydroid`
10. `crush`
11. `copilotcli`

Phase 3 (long tail, on request): kimi-code, qoder, deepagents, bob, trae, vibe,
zcode, pool, omp, mimocode, reasonix, replit, dsh, musecode, commandcode,
codewhale, grokcli, gitlabduo, lettacode, cortexcode, rovodev, takt, openclaw,
codebuddy, codebuff.

### 4.3 Close feature gaps

1. **`permissions`** (highest): new top-level `[permissions]` in `config.toml`,
   merged per tool into the settings documents we already own/manage
   (claude/opencode/codex/copilot/cursor/amp/pi/zed). Reuse `jsonmerge`.
2. **`checks`** (medium): new content kind `checks/` rendered to each preset's
   review file; small, reuses rules rendering.
3. **`hooks`** (medium): start with `command` hooks merged into the settings
   documents; TS/JS plugin generation later.
4. **`ignore`** (skip — deprecated; covered by permissions).

### 4.4 Robustness parity

- JSONC comment-preserving edits for shared settings documents (currently we
  warn and skip when comments are present).
- Optional `--global` (user-scope) output.
- Watch mode and `doctor`.

---

## 5. Key reference paths in the clone

- Tool matrix: `src/types/tool-target-tuples.ts`, `src/types/tool-targets.ts`.
- Per-tool paths: `src/constants/<tool>-paths.ts`.
- Feature processors: `src/features/<feature>/{*-processor.ts, <tool>-<feature>.ts}`.
- Permissions model: `src/types/permissions.ts`.
- Hooks model: `src/features/hooks/` (event + handler enums).
- Docs: `docs/reference/{supported-tools.md, file-formats.md}`, `docs/guide/`.
