# AI Rules Configuration Schema

This directory contains JSON Schema definitions for ai_rules configuration files.

## Current Version

- **v1**: `ai-rules-v1.schema.json` - The current stable schema

## Using the Schema

### In Your Configuration File

Add the `$schema` property to your `ai_rules.yaml` file to enable editor support:

```yaml
$schema: https://github.com/Goldziher/ai_rules/schema/ai-rules-v1.schema.json

metadata:
  name: "My Project"
  version: "1.0.0"
# ... rest of your configuration
```

### Editor Support

Most modern editors (VS Code, IntelliJ, etc.) will automatically:
- Provide autocompletion for properties
- Show inline documentation
- Validate your configuration
- Highlight errors

### Template Format

The schema supports three types of templates:

1. **Built-in templates**: Single word identifiers
   ```yaml
   template: "default"
   template: "documentation"
   ```

2. **File references**: Start with `@` followed by path
   ```yaml
   template: "@templates/custom.tmpl"
   template: "@./my-template.tmpl"
   ```

3. **Inline templates**: Multiline strings with Go template syntax
   ```yaml
   template: |
     # {{.ProjectName}}
     {{range .Rules}}
     - {{.Name}}: {{.Content}}
     {{end}}
   ```

## Template Variables

Available variables in templates:

| Variable | Type | Description |
|----------|------|-------------|
| `{{.ProjectName}}` | string | Project name from metadata |
| `{{.Version}}` | string | Version from metadata |
| `{{.Description}}` | string | Description from metadata |
| `{{.Rules}}` | []Rule | Array of rules only |
| `{{.Sections}}` | []Section | Array of sections only |
| `{{.AllContent}}` | []ContentItem | Rules and sections combined and sorted |
| `{{.Timestamp}}` | time.Time | Generation timestamp |
| `{{.RuleCount}}` | int | Total number of rules |
| `{{.SectionCount}}` | int | Total number of sections |

Each rule in `{{.Rules}}` has:
- `{{.Name}}` - Rule name
- `{{.Priority}}` - Priority number (higher = more important, default: 1)
- `{{.Content}}` - Rule content

Each section in `{{.Sections}}` has:
- `{{.Title}}` - Section title
- `{{.Priority}}` - Priority number (higher = more important, default: 1)
- `{{.Content}}` - Section content (markdown, rendered as-is)

Each item in `{{.AllContent}}` has:
- `{{.Title}}` - Name/Title of the content
- `{{.Priority}}` - Priority number
- `{{.Content}}` - Content text
- `{{.IsRule}}` - Boolean: true for rules, false for sections
- `{{.Type}}` - String: "rule" or "section"

## Sorting Order

All content (rules and sections) is sorted using dual sorting:
1. **Primary sort**: By priority (descending) - higher numbers first
2. **Secondary sort**: By title/name (ascending) - alphabetical order

This ensures consistent output order across regenerations.

## Schema Versioning

We use semantic versioning for the schema:
- **Major version**: Breaking changes (new required fields, removed fields)
- **Minor version**: New optional features
- **Patch version**: Documentation updates, clarifications

## Local Development

To use a local schema file instead of the GitHub URL:

```yaml
$schema: ../schema/ai-rules-v1.schema.json
```

Or use a relative path from your project root.