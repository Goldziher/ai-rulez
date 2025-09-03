# Configuration Schema

The `ai-rulez` configuration is validated against a formal JSON Schema to provide editor support like autocompletion and real-time validation.

## Enabling Schema Validation

To enable editor support, add the `$schema` key to the top of your `ai_rulez.yaml` file.

```yaml
$schema: "https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json"

metadata:
  name: "My Project"
# ... rest of your configuration
```

Most modern editors will automatically detect this and provide assistance.

---

## Key Validation Rules

### Required Fields

- `metadata.name`: Every project must have a name.
- `outputs`: At least one output must be defined.
- `rule`: Every rule must have a `name` and `content`.
- `section`: Every section must have a `name` and `content`.
- `agent`: Every agent must have a `name` and `description`.
- `mcp_server`: Every server must have a `name`.
- `command`: Every command must have a `name` and `description`.

### Field Constraints

- **`version`**: Must follow semantic versioning (e.g., `1.0.0`).
- **`priority`**: Must be one of `critical`, `high`, `medium`, `low`, or `minimal`.
- **`agent.name`**: Must be lowercase and can only contain hyphens (e.g., `code-reviewer`).
- **`command.name`**: Must be alphanumeric and can contain hyphens or underscores (e.g., `new-task`).
- **`output.path`**: Must be a valid file or directory path.

### Structured Templates

The `template` property for `outputs` and `agents` is a structured object:

```yaml
outputs:
  - path: "CUSTOM.md"
    template:
      type: "inline" # Can be "inline", "file", or "builtin"
      value: "# {{ .ProjectName }}\n{{ range .Rules }}{{ .Content }}{{ end }}"
```

---

## Complete Schema Reference

The full JSON Schema is available at:
[schema/ai-rules-v2.schema.json](https://github.com/Goldziher/ai-rulez/blob/main/schema/ai-rules-v2.schema.json)

Use the `ai-rulez validate` command to check your configuration against the schema at any time.
