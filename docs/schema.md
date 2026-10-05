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
- **`schema`** (`$schema` in YAML/JSON): URL of the JSON Schema, for editor support. TOML configs use the key `schema`
- **`rules`**: Rules output mode (`mode`, `mode_by_preset`, `baz_scoped`; see [Rules](rules.md#rules-mode))
- **`scopes`**: Additional scoped output roots. Their root files (`AGENTS.md`/`CLAUDE.md`) stay in the subfolder; rule files go to the root rules folders
- **`gitignore`**: Whether to update .gitignore with generated output patterns (default: true)
- **`includes`**: External content sources to merge
- **`installed_skills`**: Skills to install from external repositories
- **`header`**: Header style/timestamp for generated files
- **`defaults`**: Global `effort` and per-preset `effort_by_preset` / `model_by_preset`
- **`builtins`**: Built-in domains (`true`, `false`, or a list with `!` exclusions)
- **`compact`**: Omit per-rule priority annotations from inline sections
- **`agents_md`**: Render `AGENTS.md` and `.agents/skills` once for the presets that read them (default `false`)
- **`mcp_servers`**: Array of MCP server configurations
- **`mcp`**: Project-level MCP generation options (`self_server`, `self_server_version`, `self_server_command`)
- **`plugins`**: Array of plugin installs from a marketplace (consumer side)
- **`marketplaces`**: Array of marketplace sources
- **`plugin`**: Producer-side authoring block for a distributable plugin bundle
- **`marketplace`**: Producer-side marketplace index authoring block

### Field Constraints

- **`version`**: Must be `"4.0"` or `"3.0"`
- **`name`**: Non-empty string
- **`priority`** (in markdown frontmatter): One of `critical`, `high`, `medium`, `low`, `minimal`
- **`targets`** (in markdown frontmatter): Selects output files, as preset names (`claude`), root files (`CLAUDE.md`), paths or base names, directory prefixes (`.cursor/rules/`), globs (`.cursor/rules/*`), or `*`. See [Targets](rules.md#targets)
- **`mcp_servers[].name`**: Unique identifier for each server
- **`mcp_servers[].description`**: Human-readable description
- **`mcp_servers[].command`**: Command to execute for local `stdio` servers (npx, uvx, ai-rulez, etc.)
- **`mcp_servers[].args`**: Command arguments for local servers
- **`mcp_servers[].profiles`**: Restrict the server to the named profiles
- **`mcp_servers[].transport`**: `stdio`, `http`, or `sse` (default: `stdio`)
- **`mcp_servers[].url`**: Remote MCP URL for `http` or `sse` transports
- **`mcp_servers[].headers`**: HTTP headers for `http`/`sse` servers; values may contain `${VAR}` placeholders resolved by `generate`
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
| `schema/ai-rules-local.schema.json` | JSON Schema | V4    | Machine-local `config.local.*` overlay; used by `validate`, `local set` and the MCP `validate_config` tool |
| `schema/provider.schema.json`     | JSON Schema | V4      | Declarative provider spec referenced by `[[presets]] provider = "..."`; see [Provider-backed Presets](configuration.md#provider-backed-presets-full-parity) |
| `schema/roles-manifest.schema.json` | JSON Schema | v1    | `roles.json` and `roles list --format json`; see [Roles](roles.md#the-roles-manifest) |
| `schema/catalog.schema.json`      | JSON Schema | v1      | `ai-rulez catalog --format json` |
| `schema/lock-diff.schema.json`    | JSON Schema | v1      | `ai-rulez lock --diff --format json`; see [Lock file](lockfile.md) |

Access them at (versioned to the release; `main` is the tip):

- Main schema: `https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules.schema.json`
- MCP schema: `https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules-mcp.schema.json`
- Local overlay schema: `https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules-local.schema.json`
- Provider schema: `https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/provider.schema.json`
- Roles manifest schema: `https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/roles-manifest.schema.json`

The local overlay schema accepts the same keys as the main schema without the requirement that
`version` and `name` be present, and adds the overlay-only markers (`remove = true` on named-list
entries, `"!name"` in `presets`). See [Local Configuration](local-overrides.md#config-overlay).

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
