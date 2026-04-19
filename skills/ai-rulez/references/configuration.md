# Configuration Reference

## config.yaml Schema

```yaml
version: "3.0"              # Required, must be "3.0"
name: my-project             # Required, project name

description: ""              # Optional project description

presets:                     # Output presets to generate
  - claude                   # Built-in preset (string)
  - cursor
  - name: custom             # Custom preset (object)
    type: markdown           # markdown | directory | json
    path: docs/AI_GUIDE.md

default: ""                  # Default profile name

profiles:                    # Profile → domain mappings
  backend: [backend, shared]
  frontend: [frontend, shared]

gitignore: true              # Auto-update .gitignore (default: true)

builtins: true               # Enable all built-in domains
# builtins: false            # Disable all built-in domains
# builtins:                  # Enable specific builtins
#   - go
#   - security
#   - "!ai-governance"       # Exclude auto-include

installed_skills:            # External skills to pull at generate time
  - name: kreuzberg
    source: https://github.com/kreuzberg-dev/kreuzberg
    path: skills/kreuzberg   # Optional (defaults to skills/<name>)
    ref: main                # Optional git ref
    local_override: ../local  # Optional local dev path

includes:                    # External content sources
  - name: shared-rules
    source: https://github.com/org/shared-rules
    path: ""                 # Subdirectory within repo
    ref: main                # Git ref
    include: [rules, context, skills]  # Content types to import
    merge_strategy: local-override     # local-override | include-override | error
    install_to: ""           # Install as domain (e.g., "domains/backend")
    local_override: ""       # Local dev path override

header:
  style: detailed            # detailed | compact | minimal

## compression is deprecated and now a no-op — condense content at source level instead
```

## Built-in Presets

| Preset | Output Path |
|--------|------------|
| claude | CLAUDE.md |
| cursor | .cursor/rules/ |
| gemini | GEMINI.md |
| copilot | .github/copilot-instructions.md |
| continue-dev | .continue/ |
| windsurf | .windsurfrules |
| cline | .clinerules |
| codex | AGENTS.md |
| amp | AMP.md |
| junie | .junie/guidelines.md |
| opencode | OPENCODE.md |

## Available Builtins

Language domains: `rust`, `python`, `typescript`, `go`, `java`, `ruby`, `php`, `elixir`, `csharp`

Cross-language binding domains: `pyo3`, `napi-rs`, `magnus`, `ext-php-rs`, `rustler`, `wasm`

Practice domains: `ai-governance` (auto), `security`, `git-workflow`, `code-quality`, `testing`, `token-efficiency`, `documentation`, `default-commands`

## Content Frontmatter Fields

```yaml
---
priority: medium         # critical | high | medium | low
targets:                 # Limit to specific output targets
  - CLAUDE.md
  - .cursor/rules/*
aliases:                 # Alternative names (commands)
  - alias1
usage: "Usage text"      # Command usage info
shortcut: "Ctrl+K"       # Keyboard shortcut (commands)
category: "Category"     # Content category
description: "Desc"      # Description (skills, agents)
---
```

## Merge Strategies

- **local-override** (default): Local content wins on name conflicts
- **include-override**: Included content wins on conflicts
- **error**: Fail immediately on any name conflict

## Profile Resolution

1. If `profiles["default"]` is explicitly defined → use it
2. If no profiles are defined → include root + all domains
3. If profiles exist but "default" is not among them → include root + builtin + FromInclude domains
