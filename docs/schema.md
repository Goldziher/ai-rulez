# Configuration Schema

The `ai-rulez` configuration is validated against a formal JSON Schema to provide editor support like autocompletion and real-time validation.

## V4 Schema

**For current V4 projects**, the schema is embedded in the CLI. `ai-rulez validate` validates the raw
config file against it (converting TOML to JSON first) and then runs the structural checks, so edit
your `.ai-rulez/config.toml` and run `ai-rulez validate`.

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
    ".ai-rulez/config.toml": "https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules.schema.json"
  }
}
```

## V3 Schema (Backward Compatible)

The same schema accepts `version = "3.0"`, so V3 YAML configs get editor support too:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules.schema.json
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
- **`scopes`**: Additional scoped output roots (subfolder `AGENTS.md`/`CLAUDE.md`)
- **`gitignore`**: Whether to update .gitignore with generated output patterns (default: true)
- **`includes`**: External content sources to merge
- **`installed_skills`**: Skills to install from external repositories
- **`header`**: Header style/timestamp for generated files
- **`defaults`**: Global `effort` and per-preset `effort_by_preset` / `model_by_preset`
- **`builtins`**: Built-in domains (`true`, `false`, or a list with `!` exclusions)
- **`compact`**: Omit per-rule priority annotations from inline sections
- **`mcp_servers`**: Array of MCP server configurations
- **`plugins`**: Array of plugin installs from a marketplace (consumer side)
- **`marketplaces`**: Array of marketplace sources
- **`plugin`**: Producer-side authoring block for a distributable plugin bundle
- **`marketplace`**: Producer-side marketplace index authoring block

### Field Constraints

- **`version`**: Must be `"4.0"` or `"3.0"`
- **`name`**: Non-empty string
- **`priority`** (in markdown frontmatter): One of `critical`, `high`, `medium`, `low`, `minimal`
- **`targets`** (in markdown frontmatter): File glob patterns (e.g., `CLAUDE.md`, `.cursor/rules/*`)
- **`mcp_servers[].name`**: Unique identifier for each server
- **`mcp_servers[].command`**: Command to execute for local `stdio` servers (npx, uvx, ai-rulez, etc.)
- **`mcp_servers[].transport`**: `stdio`, `http`, or `sse` (default: `stdio`)
- **`mcp_servers[].url`**: Remote MCP URL for `http` or `sse` transports
- **`mcp_servers[].enabled`**: Set to `false` to omit the server from generated MCP outputs
- **`mcp_servers[].env`**: Environment variables; values may contain `${VAR}` placeholders resolved by `generate`

MCP placeholder names must match `[A-Za-z_][A-Za-z0-9_]*`. Generation resolves them from
`--env KEY=VALUE`, process env, then dotenv files. Secret-bearing MCP outputs must be gitignored
before generated configs are written.

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

| File                              | Format      | Version | Notes                                               |
| --------------------------------- | ----------- | ------- | --------------------------------------------------- |
| `schema/ai-rules.schema.json`     | JSON Schema | V4/V3   | Config schema; accepts `version` `"4.0"` and `"3.0"` |
| `schema/ai-rules-mcp.schema.json` | JSON Schema | V4      | Standalone schema for MCP server configurations      |

Access them at (versioned to the release; `main` is the tip):

- Main schema: `https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules.schema.json`
- MCP schema: `https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules-mcp.schema.json`

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
