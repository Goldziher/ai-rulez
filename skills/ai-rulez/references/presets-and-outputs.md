# Presets and Output Generation

## Generation Flow

1. Load the selected config root (default `.ai-rulez/config.toml`) and scan content directories
2. Load builtins (lowest priority)
3. Resolve includes (merge external content)
4. Resolve installed skills (fetch and add to content tree)
5. Select profile → determine active domains
6. Generate preset outputs from combined content, including configured scoped subfolder outputs
7. Remove stale files from `.generated-manifest.json`, write outputs, update the manifest, and (when `gitignore = true`) update the managed `.gitignore` block

## Built-in Presets

Configure in `config.toml`:

```toml
presets = [
  "claude",       # → CLAUDE.md
  "cursor",       # → .cursor/rules/*.mdc
  "gemini",       # → GEMINI.md
  "copilot",      # → .github/copilot-instructions.md
  "devin",        # → AGENTS.md, .devin/
  "cline",        # → .clinerules/, .cline/
  "codex",        # → AGENTS.md, .agents/skills/ and .codex/
  "amp",          # → AGENTS.md and .agents/ (.amp/settings.json)
  "junie",        # → AGENTS.md and .junie/
  "opencode",     # → AGENTS.md, .opencode/, opencode.json
  "hermes",       # → .hermes.md
  "antigravity",  # → .agents/, GEMINI.md
  "xum",          # → AGENTS.md, .xum/skills, .xum/agents, .xum/mcp.jsonc
  "pi",           # → AGENTS.md, .agents/skills, .pi/agents, .pi/mcp.json
  "baz",          # → AGENTS.md (root and nested), .agents/skills, .claude/agents
  "okf"           # → docs/okf/ (Open Knowledge Format bundle)
]
```

These are 15 of the 52 built-in presets. The others: `aiassistant`, `augment`, `bob`, `codebuddy`, `codebuff`,
`codewhale`, `commandcode`, `copilot-cli`, `cortex`, `crush`, `deepagents`, `dsh`, `factory`, `gitlab-duo`, `goose`,
`grok`, `kilo`, `kimi`, `kiro`, `letta`, `mimocode`, `muse`, `omp`, `openclaw`, `poolside`, `qoder`, `qwen`,
`reasonix`, `replit`, `rovodev`, `takt`, `trae`, `vibe`, `warp`, `zcode`, `zed`, `zoocode`. `docs/harnesses.md` is the
matrix of what each writes (rules folder, skills, agents, commands, MCP, hooks, permissions, checks, user scope).

Removed and renamed: `windsurf` is `devin` (`.windsurf/` is `.devin/`, `windsurf_model` is `devin_model`);
`continue-dev` is gone.

## Features by Preset Family

- **Hooks**: top-level `[[hooks]]` render for 36 harnesses, into the harness's settings file or a generated plugin
  module (`opencode`, `kilo`, `mimocode`, `pi`, `amp`). `junie`, `zcode`, `hermes` and `kimi` read hooks from the
  user config only, so they need `generate --user`.
- **Permissions**: top-level `[permissions]` render for 24 harnesses. `amp`, `pi`, `cline`, `kiro`, `factory`,
  `crush` and `rovodev` have no committable permission file and are named in a warning.
- **Checks**: `.ai-rulez/checks/` render for `cursor`, `kilo`, `qwen`, `factory`, `rovodev`, `amp`, `augment` and
  `gitlab-duo`.
- **User scope**: `generate --user` supports 47 presets; `aiassistant`, `baz`, `codebuff`, `replit` and `xum` have no
  user-level location.
- **Commands as skills**: `claude`, `codex`, `antigravity`, `devin` and `warp` write commands as skills.
- **Shared outputs**: presets that write the same path must render identical bytes, otherwise `generate` fails and
  names them.

## Reasoning Effort Per Preset

When `defaults.effort` or `defaults.effort_by_preset` is set, presets with native effort support emit it:

| Preset     | File                                            | Field                    | Scope                |
| ---------- | ----------------------------------------------- | ------------------------ | -------------------- |
| `claude`   | `.claude/agents/<id>.md`                        | `effort`                 | per-agent            |
| `codex`    | `.codex/config.toml`, `.codex/agents/<id>.toml` | `model_reasoning_effort` | global and per-agent |
| `amp`      | `.amp/settings.json`                            | `amp.anthropic.effort`   | global               |
| `devin`    | `.devin/agents/<id>.md`                         | `reasoning_effort`       | per-agent            |
| `opencode` | `.opencode/agents/<id>.md`                      | `variant`                | per-agent            |
| `xum`      | `.xum/agents/<id>.md`                           | `ai.thinkingLevel`       | per-agent            |
| `pi`       | `.pi/agents/<id>.md`                            | `thinking`               | per-agent            |

Resolution order: per-agent metadata → `defaults.effort_by_preset[<preset>]` → `defaults.effort` → omit. Each preset maps the canonical tier to its own vocabulary (e.g. Codex caps at `xhigh`/drops `inherit`; Amp uses `max` instead of `xhigh`). Other presets (cursor, copilot, gemini, junie, hermes, antigravity, cline) silently skip — those tools expose effort via UI toggles or user-managed config files we don't generate.

## Custom Presets

Define custom output formats:

```toml
[[presets]]
name = "team-guide"
type = "markdown"        # Single markdown file
path = "docs/AI_GUIDE.md"

[[presets]]
name = "rules-dir"
type = "directory"       # Directory of files
path = ".ai-rules/"

[[presets]]
name = "config-json"
type = "json"            # JSON output
path = ".ai-config.json"
```

For full parity with a built-in preset (skills, agents, commands, MCP sidecars),
point a custom preset at a declarative provider spec instead of a template:

```toml
[[presets]]
name = "my-tool"
provider = ".ai-rulez/providers/my-tool.toml"   # relative to the project root
```

The spec is validated against `schema/provider.schema.json` and its `name` must
match the preset's `name`.

## Profile-based Generation

```bash
# Generate with default profile
ai-rulez generate

# Generate with specific profile
ai-rulez generate --profile backend

# Preview without writing
ai-rulez generate --dry-run

# Use a non-default config directory
ai-rulez generate --config-dir ai-policy
```

## Content Priority

Content is rendered in priority order: critical → high → medium → low → minimal.

## Target Filtering

Use `targets` in frontmatter to limit content to specific outputs:

```yaml
---
targets:
  - CLAUDE.md # Only included in Claude output
  - .cursor/rules/* # Only included in Cursor output
---
```

Content without `targets` is included in all outputs.

## Header Styles

Control generated file headers:

- **detailed**: Full header with tool info, generation notice, and instructions
- **compact**: Brief header with tool name and generation notice
- **minimal**: Bare minimum header

## MCP Server Integration

MCP servers are configured inline in `config.toml` under `[[mcp_servers]]`, allowing assistants to access ai-rulez functionality without a separate service.

Env values may use `${VAR}` placeholders resolved by `generate` from `--env`, process env, and dotenv files. Generated MCP configs contain resolved values, so secret-bearing MCP outputs must be gitignored before they are written.

## Cleanup Safety

Generation no longer treats assistant directories as fully owned. Stale cleanup is manifest-based:

- The manifest lives at `<config-dir>/.generated-manifest.json`.
- Only files listed in the previous manifest can be deleted as stale.
- User-owned files in `.claude/`, `.codex/`, `.cursor/`, `.gemini/`, `.devin/`, `.cline/`, `.agents/`, `.opencode/`, `.junie/`, and custom output directories are preserved.
- `ai-rulez generate --dry-run` prints `write-file`, `create-dir`, and `delete-stale` entries without changing the filesystem.
