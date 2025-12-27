# Custom Presets

Create custom output formats and templates for tools not in the built-in list.

## What Are Presets?

Presets define how content is formatted and where it's output for different tools:

- Built-in presets: Pre-configured for Claude, Cursor, Gemini, Copilot, Windsurf, and others
- Custom presets: You define the format, path, and template

## Built-in Presets

AI-Rulez includes presets for popular tools:

| Preset | Output | Format |
|--------|--------|--------|
| `claude` | `CLAUDE.md` | Markdown |
| `cursor` | `.cursorrules` | Markdown |
| `gemini` | `GEMINI.md` | Markdown |
| `copilot` | `.github/copilot-instructions.md` | Markdown |
| `windsurf` | `.windsurf/rules/` | Directory |
| `continue-dev` | `.continue/config.py` | Python |
| `cline` | `.clinerules/` | Directory |

## Creating Custom Presets

### Basic Custom Preset

For a tool not in the built-in list:

```yaml
presets:
  - name: my-tool
    type: markdown
    path: docs/MY_TOOL.md
```

This generates `docs/MY_TOOL.md` with all your rules and context.

### With Custom Template

Control exactly how content is formatted:

```yaml
presets:
  - name: my-tool
    type: markdown
    path: docs/MY_TOOL.md
    template: |
      # AI Rules for {{ .Name }}

      ## Rules
      {{ range .Rules }}
      - **{{ .Name }}** ({{ .Priority }}): {{ .Content }}
      {{ end }}

      ## Context
      {{ range .Context }}
      - {{ .Name }}: {{ .Content }}
      {{ end }}
```

## Preset Types

### Markdown Type

Generates a single markdown file:

```yaml
presets:
  - name: development-guide
    type: markdown
    path: docs/AI_DEVELOPMENT_GUIDE.md
    template: |
      # AI Development Guide

      {{ range .Rules }}
      ## {{ .Name }}
      {{ .Content }}

      {{ end }}
```

### Directory Type

Generates individual files in a directory:

```yaml
presets:
  - name: agent-rules
    type: directory
    path: .claude/agents/
    naming_scheme: "{name}.md"
    template: |
      # {{ .Name }}

      {{ .Content }}
```

This creates one file per rule/context item.

### JSON Type

Generates JSON configuration:

```yaml
presets:
  - name: config-json
    type: json
    path: config/rules.json
    template: |
      {
        "name": "{{ .Name }}",
        "rules": [
          {{- range .Rules }}
          {
            "name": "{{ .Name }}",
            "priority": "{{ .Priority }}",
            "content": "{{ .Content | jsonEscape }}"
          }{{ if not (last .) }},{{ end }}
          {{- end }}
        ]
      }
```

## Template Language

Templates use Go's template syntax with access to your configuration:

### Available Data

```
.Name              Project name from config.yaml
.Description       Project description
.Rules             All rules (filtered by targets)
.Context           All context (filtered by targets)
.Skills            All skills (filtered by targets)
.Presets           All presets
.Profiles          All profiles
```

### Rule/Context/Skill Fields

Each item has:

```
.Name              Name of the rule/context/skill
.Content           Full markdown content
.Priority          Priority level (critical, high, etc.)
.Targets           Target presets this applies to
.Description       Description (for skills)
```

### Common Template Functions

**Looping:**
```
{{ range .Rules }}
  Name: {{ .Name }}
  Content: {{ .Content }}
{{ end }}
```

**Conditionals:**
```
{{ if eq .Priority "critical" }}
  CRITICAL: {{ .Name }}
{{ end }}
```

**Filters:**
```
{{ range .Rules | where "Priority" "high" }}
  {{ .Name }}
{{ end }}
```

**String operations:**
```
{{ .Name | lower }}              Convert to lowercase
{{ .Name | upper }}              Convert to uppercase
{{ .Content | truncate 100 }}    Truncate to 100 chars
```

## Common Custom Preset Examples

### Development Guide

```yaml
presets:
  - name: dev-guide
    type: markdown
    path: docs/DEVELOPMENT_GUIDE.md
    template: |
      # Development Guide

      Last updated: {{ now.Format "2006-01-02" }}

      ## Rules and Standards

      {{ range .Rules }}
      ### {{ .Name }} ({{ .Priority }})
      {{ .Content }}

      {{ end }}

      ## Architecture and Context

      {{ range .Context }}
      ### {{ .Name }}
      {{ .Content }}

      {{ end }}

      ## Expert Guidance

      {{ range .Skills }}
      ### {{ .Name }}
      {{ .Description }}

      {{ .Content }}

      {{ end }}
```

### JSON Configuration

```yaml
presets:
  - name: rules-json
    type: json
    path: config/rules.json
    template: |
      {
        "project": "{{ .Name }}",
        "description": "{{ .Description }}",
        "rules": [
          {{- range .Rules }}
          {
            "id": "{{ .Name | slugify }}",
            "name": "{{ .Name }}",
            "priority": "{{ .Priority }}",
            "content": "{{ .Content | jsonEscape }}",
            "targets": {{ .Targets | toJson }}
          }{{ if not (last .) }},{{ end }}
          {{- end }}
        ]
      }
```

### Agent Directory

```yaml
presets:
  - name: agents
    type: directory
    path: .claude/agents/
    naming_scheme: "{name}.md"
    template: |
      {{ if .Description }}
      ---
      description: "{{ .Description }}"
      priority: {{ .Priority }}
      ---

      {{ end }}
      # {{ .Name }}

      {{ .Content }}
```

### Plain Text Format

```yaml
presets:
  - name: text-rules
    type: markdown
    path: RULES.txt
    template: |
      {{ .Name }} - AI Assistant Rules
      Generated: {{ now }}

      RULES:
      {{ range .Rules }}
      [ {{ .Priority | upper }} ] {{ .Name }}
      {{ .Content | indent "  " }}

      {{ end }}
```

## Template Variables and Functions

### Date/Time

```
{{ now }}                         Current time
{{ now.Format "2006-01-02" }}     Formatted date
{{ now.Unix }}                    Unix timestamp
```

### String Functions

```
{{ .Name | lower }}               Lowercase
{{ .Name | upper }}               Uppercase
{{ .Name | title }}               Title case
{{ .Content | truncate 50 }}      Truncate to 50 chars
{{ .Content | trim }}             Remove whitespace
{{ .Content | replace "a" "b" }}  Replace text
```

### Collection Functions

```
{{ range .Rules }}                Loop through items
{{ .Name }}
{{ end }}

{{ first .Rules }}                First item
{{ last .Rules }}                 Last item
{{ len .Rules }}                  Count items

{{ .Rules | where "Priority" "high" }}  Filter by field
{{ .Rules | reverse }}            Reverse order
```

### JSON Functions

```
{{ .Targets | toJson }}           Convert to JSON array
{{ .Content | jsonEscape }}       Escape for JSON string
```

### Advanced Conditions

```
{{ if eq .Priority "critical" }}  Equal check
{{ if gt .Priority "high" }}      Greater than (string comparison)
{{ if contains .Name "test" }}    Contains substring
{{ if empty .Description }}       Is empty
{{ if not .Targets }}             Is falsy
```

## Targeting Rules to Presets

Control which content appears in which preset:

```yaml
presets:
  - name: claude
    type: markdown
    path: CLAUDE.md

  - name: internal-guide
    type: markdown
    path: docs/INTERNAL_GUIDE.md

rules:
  - name: "Public Rule"
    content: "This applies everywhere"
    targets: []                    # Empty = all presets

  - name: "Claude Only Rule"
    content: "Only in Claude instructions"
    targets:
      - CLAUDE.md

  - name: "Internal Rule"
    content: "Only in internal guide"
    targets:
      - docs/INTERNAL_GUIDE.md
```

## Priority Ordering

Content is ordered by priority level (highest to lowest):

```
critical
high
medium (default)
low
minimal
```

Templates automatically sort by priority. To customize:

```
{{ range (sortByPriority .Rules) }}
  {{ .Name }}
{{ end }}
```

## Combining Built-in and Custom

Use both in the same configuration:

```yaml
presets:
  # Built-in
  - claude
  - cursor
  - gemini

  # Custom
  - name: internal-guide
    type: markdown
    path: docs/AI_DEVELOPMENT_GUIDE.md

  - name: rules-database
    type: json
    path: config/rules.json
```

When you run `ai-rulez generate`, it creates:
- `CLAUDE.md` (built-in)
- `.cursorrules` (built-in)
- `GEMINI.md` (built-in)
- `docs/AI_DEVELOPMENT_GUIDE.md` (custom)
- `config/rules.json` (custom)

## Best Practices

### Start Simple

Begin with built-in presets, add custom ones only if needed:

```yaml
presets:
  - claude
  - cursor
  - gemini
```

### Use Clear Names

Names should indicate purpose:

```yaml
Good:
  - name: development-guide
  - name: api-specification
  - name: internal-rules

Bad:
  - name: output1
  - name: thing
```

### Document Your Template

Add comments explaining what it does:

```yaml
presets:
  - name: api-spec
    type: markdown
    path: docs/API_SPEC.md
    template: |
      # API Specification
      # Generated from AI-Rulez config
      # Updates automatically on config changes

      {{ range .Rules }}
      {{ .Name }}: {{ .Content }}
      {{ end }}
```

### Keep Paths Consistent

Use standard locations:

```
docs/       Documentation
config/     Configuration files
.editor/    Editor-specific configs
.tools/     Tool-specific configs
```

### Test Your Templates

Preview before committing:

```bash
ai-rulez generate --dry-run
cat docs/MY_TOOL.md
```

## Troubleshooting

### Template Syntax Error

Check for:
- Missing closing `{{ end }}`
- Unmatched braces
- Invalid function names

### Path Issues

Ensure paths:
- Don't start with `/`
- Use forward slashes `/`
- Are relative to project root

### Content Not Appearing

Check targeting:

```yaml
rules:
  - name: "Test Rule"
    targets:
      - CLAUDE.md      # Must match preset path exactly
```

## Advanced Example: Multi-Format Output

```yaml
presets:
  # Markdown for humans
  - name: human-guide
    type: markdown
    path: docs/AI_GUIDE.md
    template: |
      # AI Development Guide

      {{ range .Rules }}
      ## {{ .Name }}
      {{ .Content }}
      {{ end }}

  # JSON for machines
  - name: machine-readable
    type: json
    path: config/ai-rules.json
    template: |
      {
        "name": "{{ .Name }}",
        "rules": [
          {{- range .Rules }}
          {"name": "{{ .Name }}", "priority": "{{ .Priority }}"}{{ if not (last .) }},{{ end }}
          {{- end }}
        ]
      }

  # Plain text for simple tools
  - name: plain-text
    type: markdown
    path: RULES.txt
    template: |
      RULES FOR {{ .Name | upper }}
      {{ range .Rules }}
      - {{ .Name }}: {{ .Content | truncate 50 }}
      {{ end }}
```

## Next Steps

- **[Configuration Reference](configuration.md)**: All config options
- **[Quick Start](quick-start.md)**: Getting started
- **[Domains & Profiles](domains.md)**: Organizing by team
