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

Configure in `config.yaml`:

```yaml
presets:
  - claude       # → CLAUDE.md
  - cursor       # → .cursor/rules/*.mdc
  - gemini       # → GEMINI.md
  - copilot      # → .github/copilot-instructions.md
  - continue-dev # → .continue/rules/
  - windsurf     # → .windsurfrules
  - cline        # → .clinerules
  - codex        # → AGENTS.md
  - amp          # → AMP.md
  - junie        # → .junie/guidelines.md
  - opencode     # → OPENCODE.md
```

## Custom Presets

Define custom output formats:

```yaml
presets:
  - name: team-guide
    type: markdown        # Single markdown file
    path: docs/AI_GUIDE.md

  - name: rules-dir
    type: directory       # Directory of files
    path: .ai-rules/

  - name: config-json
    type: json            # JSON output
    path: .ai-config.json
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

## Compression

Reduce token usage in generated outputs:

- **off**: No compression (default)
- **light**: Punctuation and whitespace cleanup
- **moderate**: + stopword removal
- **aggressive**: + sentence selection
- **maximum**: + semantic scoring and hypernym compression
