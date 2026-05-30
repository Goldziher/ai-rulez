# Configuration Schema

The `ai-rulez` configuration is validated against a formal JSON Schema to provide editor support like autocompletion and real-time validation.

## V4 Schema

**For current V4 projects**, the schema is embedded in the CLI validation. Simply edit your `.ai-rulez/config.toml` file and the CLI will validate it.

```toml
# .ai-rulez/config.toml
version = "4.0"
name = "My Project"
```

Most modern editors support TOML schema references. Add this to enable autocompletion (for VS Code with TOML extension):

Add a `.vscode/settings.json` to reference the schema:

```json
{
  "evenBetterToml.schema.associations": {
    ".ai-rulez/config.toml": "https://github.com/Goldziher/ai-rulez/schema/ai-rules.schema.json"
  }
}
```

## V3 Schema (Backward Compatible)

**For V3 projects**, the schema is available for YAML:

```yaml
# yaml-language-server: $schema=https://github.com/Goldziher/ai-rulez/schema/ai-rules.schema.json
version: "3.0"
name: "My Project"
```

V4 accepts both `"4.0"` and `"3.0"` versions for backward compatibility.

---

## V4 Validation Rules

### Required Fields

- **`version`**: Must be `"4.0"` or `"3.0"` (for backward compatibility)
- **`name`**: Project name (required, non-empty string)

### Optional Fields

- **`description`**: Project description
- **`presets`**: List of tool presets (e.g., `claude`, `cursor`, `gemini`)
- **`profiles`**: Named profiles specifying which domains to include
- **`default`**: Default profile name
- **`gitignore`**: Whether to update .gitignore with generated output patterns (default: true)
- **`mcp_servers`**: Array of MCP server configurations
- **`plugins`**: Array of plugin configurations
- **`marketplaces`**: Array of marketplace integrations

### Field Constraints

- **`version`**: Must be `"4.0"` or `"3.0"`
- **`name`**: Non-empty string
- **`priority`** (in markdown frontmatter): One of `critical`, `high`, `medium`, `low`, `minimal`
- **`targets`** (in markdown frontmatter): File glob patterns (e.g., `CLAUDE.md`, `.cursor/rules/*`)
- **`mcp_servers[].name`**: Unique identifier for each server
- **`mcp_servers[].command`**: Command to execute (npx, uvx, ai-rulez, etc.)
- **`mcp_servers[].env`**: Environment variables; values may contain `${VAR}` placeholders resolved by `generate`

### File Structure

V4 uses TOML for configuration and markdown files with optional YAML frontmatter:

```toml
# config.toml
version = "4.0"
name = "My Project"

[[mcp_servers]]
name = "ai-rulez"
command = "npx"
args = ["-y", "ai-rulez@latest", "mcp"]
```

Content files use markdown with optional YAML frontmatter:

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

## Available Schemas

The schema files are available in the repository:

| File | Format | Version | Notes |
|------|--------|---------|-------|
| `schema/ai-rules.schema.json` | JSON Schema | V4/V3 | Current schema for V4 and V3 (backward compatible) |
| `schema/ai-rules-mcp.schema.json` | JSON Schema | V4 | Schema for MCP server configurations |

Access them at:

- Main schema: `https://github.com/Goldziher/ai-rulez/schema/ai-rules.schema.json`
- MCP schema: `https://github.com/Goldziher/ai-rulez/schema/ai-rules-mcp.schema.json`

---

## Validation Commands

Use the `ai-rulez validate` command to check your configuration against the schema at any time:

```bash
# Validate current directory
ai-rulez validate

# Validate specific config (TOML or YAML)
ai-rulez validate .ai-rulez/config.toml

# Verbose output
ai-rulez validate --verbose
```
