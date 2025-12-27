# Configuration Schema

The `ai-rulez` configuration is validated against a formal JSON Schema to provide editor support like autocompletion and real-time validation.

## V3 Schema

**For current V3 projects**, the schema is embedded in the CLI validation. Simply edit your `.ai-rulez/config.yaml` file and the CLI will validate it.

```yaml
# .ai-rulez/config.yaml
version: "3.0"
name: "My Project"
```

Most modern editors support YAML schema references. Add this to enable autocompletion:

```yaml
# yaml-language-server: $schema=https://github.com/Goldziher/ai-rulez/schema/ai-rules-v3.schema.json
version: "3.0"
name: "My Project"
```

## V2 Schema (Legacy)

If you're still using V2 (single `ai-rulez.yaml` file), enable schema validation with:

```yaml
$schema: "https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json"

metadata:
  name: "My Project"
# ... rest of your configuration
```

Most modern editors will automatically detect this and provide assistance.

---

## V3 Validation Rules

### Required Fields

- **`version`**: Must be exactly `"3.0"`
- **`name`**: Project name (required, non-empty string)

### Optional Fields

- **`description`**: Project description
- **`presets`**: List of tool presets (e.g., `claude`, `cursor`, `gemini`)
- **`profiles`**: Named profiles specifying which domains to include
- **`default`**: Default profile name
- **`gitignore`**: Whether to update .gitignore (default: true)

### Field Constraints

- **`version`**: Must be exactly `"3.0"` (V3 schema)
- **`name`**: Non-empty string
- **`priority`** (in markdown frontmatter): One of `critical`, `high`, `medium`, `low`, `minimal`
- **`targets`** (in markdown frontmatter): File glob patterns (e.g., `CLAUDE.md`, `.cursor/rules/*`)

### File Structure

V3 uses markdown files with optional YAML frontmatter:

```markdown
---
priority: high
targets: ["CLAUDE.md"]
custom_field: value
---

# Title

Content here
```

---

## V2 Validation Rules (Legacy)

This documentation applies to V2 configurations. For details on V2 schema constraints, see:

The full V2 JSON Schema is available at:
[schema/ai-rules-v2.schema.json](https://github.com/Goldziher/ai-rulez/blob/main/schema/ai-rules-v2.schema.json)

---

## Validation Commands

Use the `ai-rulez validate` command to check your configuration against the schema at any time:

```bash
# Validate current directory
ai-rulez validate

# Validate specific config
ai-rulez validate .ai-rulez/config.yaml

# Verbose output
ai-rulez validate --verbose
```
