# Configuration Schema

ai-rulez uses JSON Schema to validate configuration files and provide editor support.

## Schema URL

```yaml
$schema: "https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json"
```

Add this to your `ai_rulez.yaml` for editor autocompletion and validation:

```yaml
$schema: "https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json"
metadata:
  name: "My Project"
rules:
  - name: "example"
    content: "Example rule"
outputs:
  - path: "CLAUDE.md"
```

## Key Validation Rules

### Required Fields
- `metadata.name` - Project name (minimum 1 character)
- `outputs` - At least one output configuration
- Each `rule` must have `name` and `content`
- Each `section` must have `title` and `content` 
- Each `agent` must have `name` and `description`

### Field Constraints
- **Version**: Must follow semantic versioning (e.g., `1.0.0`)
- **Priority**: Integer ≥ 1 (defaults to 1)
- **Agent names**: Must be lowercase with hyphens only (`^[a-z][a-z0-9-]*$`)
- **Includes**: Must be valid HTTP/HTTPS URLs or local file paths
- **Output paths**: Must have either `path` or `file` field (prefer `path`)

### Advanced Features
- **Targets**: Glob patterns for applying rules/sections to specific outputs
- **Templates**: Built-in names, file references (`@file.tmpl`), or inline templates
- **User Rules**: Personal overrides in `user_rulez` section

## Complete Schema Reference

The full JSON Schema is available at: [schema/ai-rules-v1.schema.json](https://github.com/Goldziher/ai-rulez/blob/main/schema/ai-rules-v1.schema.json)

### Schema Features
- ✅ Complete validation for all configuration fields
- ✅ Editor autocompletion support (VS Code, IntelliJ, etc.)
- ✅ Detailed error messages with field descriptions
- ✅ Examples and constraints for each field
- ✅ Support for remote includes validation
- ✅ Template validation (built-in, file references, inline)

### Editor Setup

Most modern editors automatically detect JSON Schema from the `$schema` field. For manual setup:

**VS Code**: Install the YAML extension by Red Hat
**IntelliJ/WebStorm**: Built-in YAML support with schema detection
**Vim/Neovim**: Use coc-yaml or similar LSP plugins

## Validation Examples

### Valid Configuration
```yaml
$schema: "https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json"
metadata:
  name: "My Project"
  version: "1.0.0"
rules:
  - name: "code-quality"
    content: "Write clean code"
    priority: 10
outputs:
  - path: "CLAUDE.md"
```

### Common Validation Errors
```yaml
# ❌ Missing required name
metadata: {}

# ❌ Invalid version format  
metadata:
  name: "Project"
  version: "1.0"  # Should be "1.0.0"

# ❌ Invalid agent name
agents:
  - name: "Code_Reviewer"  # Should be "code-reviewer"
    description: "Reviews code"

# ❌ Missing outputs
metadata:
  name: "Project"
# outputs: []  # At least one output required
```

Use `ai-rulez validate` to check your configuration against the schema.