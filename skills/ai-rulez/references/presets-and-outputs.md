# Presets and Output Generation

## Generation Flow

1. Load `.ai-rulez/config.yaml` and scan content directories
2. Load builtins (lowest priority)
3. Resolve includes (merge external content)
4. Resolve installed skills (fetch and add to content tree)
5. Select profile → determine active domains
6. Generate preset outputs from combined content
7. Write files and update `.gitignore`

## Built-in Presets

Configure in `config.toml`:

```toml
presets = [
  "claude",       # → CLAUDE.md
  "cursor",       # → .cursor/rules/*.mdc
  "gemini",       # → GEMINI.md
  "copilot",      # → .github/copilot-instructions.md
  "continue-dev", # → .continue/rules/
  "windsurf",     # → .windsurfrules
  "cline",        # → .clinerules
  "codex",        # → AGENTS.md
  "amp",          # → AMP.md
  "junie",        # → .junie/guidelines.md
  "opencode"      # → OPENCODE.md
]
```

## Reasoning Effort Per Preset

When `defaults.effort` or `defaults.effort_by_preset` is set, presets with native effort support emit it:

| Preset | File | Field | Scope |
|--------|------|-------|-------|
| `claude` | `.claude/agents/<id>.md` | `effort` | per-agent |
| `codex` | `.codex/config.toml` | `model_reasoning_effort` | global |
| `amp` | `.amp/settings.json` | `amp.anthropic.effort` | global |
| `windsurf` | `.windsurf/agents/<id>.md` | `reasoning_effort` | per-agent |

Resolution order: per-agent metadata → `defaults.effort_by_preset[<preset>]` → `defaults.effort` → omit. Each preset maps the canonical tier to its own vocabulary (e.g. Codex caps at `xhigh`/drops `inherit`; Amp uses `max` instead of `xhigh`). Other presets (cursor, copilot, gemini, junie, opencode, antigravity, cline, continue-dev) silently skip — those tools expose effort via UI toggles or user-managed config files we don't generate.

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

## Profile-based Generation

```bash
# Generate with default profile
ai-rulez generate

# Generate with specific profile
ai-rulez generate --profile backend

# Preview without writing
ai-rulez generate --dry-run
```

## Content Priority

Content is rendered in priority order: critical → high → medium → low.

## Target Filtering

Use `targets` in frontmatter to limit content to specific outputs:

```yaml
---
targets:
  - CLAUDE.md           # Only included in Claude output
  - .cursor/rules/*     # Only included in Cursor output
---
```

Content without `targets` is included in all outputs.

## Header Styles

Control generated file headers:

- **detailed**: Full header with tool info, generation notice, and instructions
- **compact**: Brief header with tool name and generation notice
- **minimal**: Bare minimum header

## MCP Server Integration

MCP servers are now configured inline in `config.toml` under `[mcp]` sections, allowing assistants to access ai-rulez functionality without a separate service.
