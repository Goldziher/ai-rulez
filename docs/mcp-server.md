# Enabling the MCP Server

The `ai-rulez` MCP (Model Context Protocol) server allows your AI assistant to programmatically and safely interact with your `.ai-rulez/` configuration. Since V4 uses a file-based approach with inline MCP configuration, you define servers directly in `config.toml`, and the MCP server provides read, CRUD, validation, and generation tools for AI assistants.

**You do not need to start the server manually.** Your AI assistant will start it automatically based on the configuration you provide.

---

## Configuration Examples

To enable the server, add one of the following snippets to your AI assistant's configuration file (e.g., Cursor's `settings.json`), or define it inline in `.ai-rulez/config.toml`. The top-level key (`mcpServers` below) differs between clients; check your assistant's documentation.

### Using `npx` (Recommended for Node.js users)

This method ensures you are always using the latest version of `ai-rulez` without needing to install it globally.

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "npx",
      "args": ["-y", "ai-rulez@latest", "mcp"]
    }
  }
}
```

### Using `uvx` (Recommended for Python users)

This method uses `uvx` to run `ai-rulez` in an ephemeral environment.

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "uvx",
      "args": ["ai-rulez", "mcp"]
    }
  }
}
```

### Using a Local Go Installation

If you have installed `ai-rulez` locally with `go install`.

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "ai-rulez",
      "args": ["mcp"]
    }
  }
}
```

### Inline in `config.toml` (V4 Recommended)

V4 supports defining MCP servers directly in your `.ai-rulez/config.toml`:

```toml
[[mcp_servers]]
name = "ai-rulez"
command = "npx"
args = ["-y", "ai-rulez@latest", "mcp"]

[[mcp_servers]]
name = "grafana"
command = "uvx"
args = ["mcp-grafana"]
env = { GRAFANA_URL = "http://localhost:3000", GRAFANA_SERVICE_ACCOUNT_TOKEN = "${GRAFANA_SERVICE_ACCOUNT_TOKEN}" }

[[mcp_servers]]
name = "remote-api"
transport = "http"
url = "https://mcp.example.com/mcp"
headers = { Authorization = "Bearer ${REMOTE_API_TOKEN}" }
```

MCP env and header values may contain `${VAR}` placeholders. `ai-rulez generate` resolves them from repeated
`--env KEY=VALUE` flags, process environment variables, then dotenv files. Placeholder names must
match `[A-Za-z_][A-Za-z0-9_]*`. By default, `.env` is loaded from the generation base directory. If
any `--env-file PATH` flags are supplied, the default `.env` is not loaded; multiple files are merged
in flag order, with later files winning. Generation fails if a placeholder cannot be resolved.

### Generated Self-Entry (`[mcp] self_server`)

Instead of declaring the ai-rulez server yourself, let `generate` add it to the project `.mcp.json`,
pinned to the version of the ai-rulez binary that ran it:

```toml
[mcp]
self_server = true
```

The entry is merged into an existing `.mcp.json`, so hand-authored servers survive, and
`.claude/settings.json` is not touched. See [Configuration: `mcp`](configuration.md#mcp) for
`self_server_version`, `self_server_command`, and the interaction with `[[mcp_servers]]`.

Generated MCP config files contain resolved values. If a value came from a placeholder, or if an env
key contains `TOKEN`, `SECRET`, `PASSWORD`, `KEY`, or `CREDENTIAL`, generation fails before writing
unless `.mcp.json`, `.claude/settings.json`, `.gemini/settings.json`, `.agents/settings.json`,
`opencode.json`, or a scoped variant is gitignored or covered by planned `--gitignore` patterns.
Secret-bearing values are redacted before source-hash calculation, not from generated MCP config
files. Any generated file that contains a resolved secret value is written with mode `0600`.

Servers can also be defined per machine in the [`config.local.*` overlay](local-overrides.md), which
keeps credentials out of the shared config.

Set `enabled = false` on an inline `[[mcp_servers]]` entry to skip it in generated MCP outputs.

---

## Server Capabilities

When enabled, the MCP server provides your AI assistant with access to your configuration and CRUD operations. The server supports:

- **Read Configuration**: Inspect rules, context, skills, profiles, and presets
- **Generate Outputs**: Programmatically trigger generation of tool-specific files
- **Validate Configuration**: Check configuration validity and report errors
- **CRUD Operations**: Create, read, update, and delete domains, rules, context, skills, includes, and profiles

The MCP server enables AI assistants to:

1. **Understand your setup** by reading configuration and content files
2. **Modify configuration** programmatically via CRUD tools
3. **Generate outputs** after changes
4. **Validate changes** before committing

This approach ensures your configuration remains auditable and version-controlled, while allowing AI assistants to help you manage it efficiently.

## Typical Workflow

### With Your Editor

1. **Edit files** directly in `.ai-rulez/`:

   ```bash
   # Edit rules, context, skills, or config.toml
   vim .ai-rulez/rules/code-quality.md
   ```

2. **Use MCP server** (via AI assistant) to generate:

   ```bash
   ai-rulez generate
   ```

3. **Commit changes**:

   ```bash
   git add .ai-rulez/   # plus generated files only if gitignore = false
   git commit -m "docs: update AI guidelines"
   ```

### With Claude CLI

If using the Claude CLI with the ai-rulez MCP server, you can ask Claude to help:

- "Review my `.ai-rulez/` configuration and suggest improvements"
- "Generate outputs for my new rules"
- "Validate that my profiles are correct"
- "Create a new backend domain and add database rules"
- "Add an include from our shared rules repository"

The server provides Claude with access to read and modify your configuration, while you maintain full control over final decisions.

---

## MCP Tools Reference

The ai-rulez MCP server exposes 36 tools for programmatic configuration management. These tools allow AI assistants to initialize projects, generate and clean outputs, validate configuration, inspect builtins, and create, read, update, and delete configuration elements.

Every tool below accepts an optional `working_directory` parameter (the directory to operate in),
except the two utility tools `get_version` and `show_builtin`. The per-tool parameter lists below
name only the parameters specific to that tool unless noted.

### Local configuration (`local: true`)

Tools that change content or config accept an optional `local` boolean that redirects the change to
the machine-local layer instead of the shared one. See [Local Configuration](local-overrides.md).

| Tools | `local: true` targets |
| ----- | --------------------- |
| `create_*`, `read_*`, `update_*`, `delete_*`, `list_*` for rule, context and skill | The `.ai-rulez/local/` content tree (`domain` selects `local/domains/<name>/`) |
| `add_include`, `remove_include`, `install_skill`, `uninstall_skill`, `update_config`, `add_profile`, `remove_profile`, `set_default_profile` | The `config.local.*` overlay |

Not accepted: `create_domain`, `delete_domain`, `list_domains`, `list_includes`, `list_profiles`,
`list_installed_skills`. Those list tools show the shared layer only.

The drift guard applies to `generate_outputs` too. `--allow-local-drift` is CLI-only: the MCP server
cannot write overlay-derived values over tracked shared files. Pass `no_local: true` to generate the
shared view, or fix the reported paths.

### Response keys

Responses of the list tools serialize Go structs without JSON tags, so item keys are capitalised
(`Name`, `Path`), unlike the lower-case envelope keys (`success`, `operation`, `count`).

### Project and Utility Tools

#### `generate_outputs`

Generate output files from the current configuration.

**Parameters:**

- `config_file` (optional, string): Path to the root configuration file
- `config_dir` (optional, string): Configuration directory name (default: `.ai-rulez`)
- `dry_run` (optional, boolean): Preview changes without writing files
- `recursive` (optional, boolean): Generate for all subdirectories containing `.ai-rulez/`
- `no_local` (optional, boolean): Ignore the machine-local `config.local.*` overlay and `local/` content (the view a teammate sees)
- `working_directory` (optional, string): Directory to operate in

#### `clean_outputs`

Remove the files produced by `generate_outputs` (its inverse): the generated assistant files, the generated manifest (and local manifest with the local rule files it records), and the ai-rulez managed `.gitignore` block. The `.ai-rulez/` source tree is never touched, and generated directories are removed only once empty.

**Parameters:**

- `config_file` (optional, string): Path to the root configuration file
- `config_dir` (optional, string): Configuration directory name (default: `.ai-rulez`)
- `dry_run` (optional, boolean): Preview what would be removed without deleting
- `keep_gitignore` (optional, boolean): Leave the ai-rulez managed block in `.gitignore`
- `keep_manifest` (optional, boolean): Leave the generated manifest in place
- `working_directory` (optional, string): Directory to operate in

#### `validate_config`

Validate the configuration file, including all includes. A `config.local.*` overlay, when present, is
also validated against the local schema (`schema/ai-rules-local.schema.json`). Errors are redacted:
URL credentials and quoted source values are removed, since the merged config can contain local secrets.

**Parameters:**

- `config_file` (optional, string): Path to the root configuration file
- `config_dir` (optional, string): Configuration directory name (default: `.ai-rulez`)
- `no_local` (optional, boolean): Validate without the machine-local overlay
- `working_directory` (optional, string): Directory to operate in

#### `init_project`

Initialize a new ai-rulez project in the current directory. Unlike the CLI `init` (which defaults to
TOML), this writes `.ai-rulez/config.yaml`.

**Parameters:**

- `project_name` (optional, string): Project name
- `providers` (optional, array): Providers to enable, such as `claude` or `cursor`
- `with_agents` (optional, boolean): Include sample agent configurations
- `all_providers` (optional, boolean): Enable the curated set of tool presets
- `popular_providers` (optional, boolean): Same curated set — a shortcut for `all_providers`
- `working_directory` (optional, string): Directory to operate in

#### `get_version`

Return the ai-rulez version.

**Parameters:** (none)

#### `show_builtin`

Show the full content of a builtin domain.

**Parameters:**

- `name` (required, string): Builtin domain name, such as `security`, `rust`, or `typescript`

### Domain Tools

#### `create_domain`

Create a new domain with subdirectories for rules, context, and skills.

**Parameters:**

- `name` (required, string): Domain name (alphanumeric and underscores, 1-50 characters)
- `description` (optional, string): Description of the domain

**Response:**

```json
{
  "success": true,
  "operation": "create_domain",
  "name": "backend",
  "path": ".ai-rulez/domains/backend",
  "message": "Domain created successfully"
}
```

**Example:**

```text
Create a domain called "backend" for backend services
```

#### `delete_domain`

Delete a domain and all its contents.

**Parameters:**

- `name` (required, string): Domain name to delete

**Response:**

```json
{
  "success": true,
  "operation": "delete_domain",
  "name": "backend",
  "message": "Domain deleted successfully"
}
```

#### `list_domains`

List all domains in the `.ai-rulez/` directory.

**Parameters:** (none)

**Response:**

```json
{
  "success": true,
  "operation": "list_domains",
  "domains": [
    {
      "Name": "backend",
      "Path": ".ai-rulez/domains/backend",
      "Description": "Backend services"
    }
  ],
  "count": 1
}
```

### Rule Tools

#### `create_rule`

Create a new rule file with optional YAML frontmatter.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `content` (optional, string): Markdown content with optional YAML frontmatter
- `domain` (optional, string): Domain name (if not specified, creates in root)
- `priority` (optional, string): Priority level - critical, high, medium, low, minimal. Default: medium
- `targets` (optional, array): Target providers (e.g., ["claude", "cursor"])
- `local` (optional, boolean): Create in `.ai-rulez/local/` (gitignored) instead of the shared tree

**Response:**

```json
{
  "success": true,
  "operation": "create_rule",
  "path": ".ai-rulez/rules/code-quality.md",
  "name": "code-quality",
  "domain": null,
  "message": "Rule created successfully"
}
```

#### `update_rule`

Update an existing rule file.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `content` (required, string): New markdown content
- `domain` (optional, string): Domain name
- `priority` (optional, string): Priority level
- `targets` (optional, array): Target providers
- `local` (optional, boolean): Update the file in `.ai-rulez/local/`

**Response:** Same as create_rule

#### `read_rule`

Read a rule file.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Read from `.ai-rulez/local/`
- `working_directory` (optional, string): Directory to operate in

#### `delete_rule`

Delete a rule file.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Delete from `.ai-rulez/local/`

**Response:**

```json
{
  "success": true,
  "operation": "delete_rule",
  "name": "code-quality",
  "domain": null,
  "message": "Rule deleted successfully"
}
```

#### `list_rules`

List all rules in the root or a specific domain.

**Parameters:**

- `domain` (optional, string): Domain name (lists root rules if not specified)
- `local` (optional, boolean): List `.ai-rulez/local/` instead of the shared tree

**Response:**

```json
{
  "success": true,
  "operation": "list_rules",
  "domain": null,
  "rules": [
    {
      "Name": "code-quality",
      "Path": ".ai-rulez/rules/code-quality.md",
      "Type": "rules",
      "Domain": "",
      "Priority": "high",
      "Targets": ["claude", "cursor"]
    }
  ],
  "count": 1
}
```

### Context Tools

#### `create_context`

Create a new context file (documentation/reference material).

**Parameters:**

- `name` (required, string): Context filename without .md extension
- `content` (optional, string): Markdown content with optional YAML frontmatter
- `domain` (optional, string): Domain name
- `priority` (optional, string): Priority level
- `targets` (optional, array): Target providers
- `local` (optional, boolean): Create in `.ai-rulez/local/` (gitignored)

**Response:** Similar to create_rule

#### `update_context`

Update an existing context file.

**Parameters:** Same as create_context, with content as required

**Response:** Similar to create_rule

#### `read_context`

Read a context file.

**Parameters:**

- `name` (required, string): Context filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Read from `.ai-rulez/local/`
- `working_directory` (optional, string): Directory to operate in

#### `delete_context`

Delete a context file.

**Parameters:**

- `name` (required, string): Context filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Delete from `.ai-rulez/local/`

**Response:** Similar to delete_rule

#### `list_context`

List all context files in the root or a specific domain.

**Parameters:**

- `domain` (optional, string): Domain name
- `local` (optional, boolean): List `.ai-rulez/local/` instead of the shared tree

**Response:** Similar to list_rules (items use the capitalised `Name`, `Path`, `Type`, `Domain`, `Priority`, `Targets` keys), with a "context" key instead of "rules"

### Skill Tools

#### `create_skill`

Create a new skill file (AI prompt/expert definition).

**Parameters:**

- `name` (required, string): Skill filename without .md extension
- `content` (optional, string): Markdown content with optional YAML frontmatter
- `domain` (optional, string): Domain name
- `priority` (optional, string): Priority level
- `targets` (optional, array): Target providers
- `local` (optional, boolean): Create in `.ai-rulez/local/` (gitignored)

**Response:** Similar to create_rule

#### `update_skill`

Update an existing skill file.

**Parameters:** Same as create_skill, with content as required

**Response:** Similar to create_rule

#### `read_skill`

Read a skill file.

**Parameters:**

- `name` (required, string): Skill filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Read from `.ai-rulez/local/`
- `working_directory` (optional, string): Directory to operate in

#### `delete_skill`

Delete a skill file.

**Parameters:**

- `name` (required, string): Skill filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Delete from `.ai-rulez/local/`

**Response:** Similar to delete_rule

#### `list_skills`

List all skill files in the root or a specific domain.

**Parameters:**

- `domain` (optional, string): Domain name
- `local` (optional, boolean): List `.ai-rulez/local/` instead of the shared tree

**Response:** Similar to list_rules (items use the capitalised `Name`, `Path`, `Type`, `Domain`, `Priority`, `Targets` keys), with a "skills" key instead of "rules"

### Include Tools

#### `add_include`

Add a new include source (git URL or local path) to the configuration.

**Parameters:**

- `name` (required, string): Include name (unique identifier)
- `source` (required, string): Git URL (`https://github.com/org/repo`) or local path (`./packages/shared`)
- `path` (optional, string): Path within git repository where .ai-rulez/ content is located
- `ref` (optional, string): Git reference - branch, tag, or commit hash (git sources only). Defaults to the repository's default branch (`HEAD`)
- `include` (optional, array): Content types to include - rules, context, skills, agents, commands
- `merge_strategy` (optional, string): Merge strategy - local-override (default), include-override, error
- `install_to` (optional, string): Installation target path in .ai-rulez/
- `local` (optional, boolean): Add the include to the `config.local.*` overlay instead of the shared config

**Response:**

```json
{
  "success": true,
  "operation": "add_include",
  "name": "corporate-rules",
  "source": "https://github.com/myorg/shared-rules",
  "message": "Include added successfully"
}
```

#### `remove_include`

Remove an include source from the configuration.

**Parameters:**

- `name` (required, string): Include name to remove
- `local` (optional, boolean): Remove through the overlay; an include from the shared config is hidden with `remove = true`

**Response:**

```json
{
  "success": true,
  "operation": "remove_include",
  "name": "corporate-rules",
  "message": "Include removed successfully"
}
```

#### `list_includes`

List all include sources in the configuration.

**Parameters:** (none)

**Response:**

```json
{
  "success": true,
  "operation": "list_includes",
  "includes": [
    {
      "Name": "corporate-rules",
      "Source": "https://github.com/myorg/shared-rules",
      "Type": "git"
    }
  ],
  "count": 1
}
```

### Installed Skill Tools

#### `install_skill`

Install a named skill from a git repository or local path.

**Parameters:**

- `name` (required, string): Skill name (unique identifier)
- `source` (required, string): Git URL or local filesystem path
- `path` (optional, string): Path within repo to skill directory (defaults to `skills/<name>`)
- `ref` (optional, string): Git reference (branch, tag, commit)
- `local` (optional, boolean): Record the skill in the `config.local.*` overlay instead of the shared config

**Response:**

```json
{
  "success": true,
  "operation": "install_skill",
  "name": "kreuzberg",
  "source": "https://github.com/kreuzberg-dev/kreuzberg",
  "message": "Skill installed successfully"
}
```

#### `uninstall_skill`

Remove an installed skill from the configuration.

**Parameters:**

- `name` (required, string): Skill name to remove
- `local` (optional, boolean): Remove through the overlay; a skill from the shared config is hidden with `remove = true`

**Response:**

```json
{
  "success": true,
  "operation": "uninstall_skill",
  "name": "kreuzberg",
  "message": "Skill uninstalled successfully"
}
```

#### `list_installed_skills`

List all installed skills.

**Parameters:** None

**Response:**

```json
{
  "success": true,
  "operation": "list_installed_skills",
  "installed_skills": [
    {
      "Name": "kreuzberg",
      "Source": "https://github.com/kreuzberg-dev/kreuzberg",
      "Path": "skills/kreuzberg",
      "Ref": "",
      "Type": "git"
    }
  ],
  "count": 1
}
```

### Config Tools

#### `read_config`

Read the current project configuration as structured JSON. The result is the shared view (as if loaded
with `--no-local`), so a read-modify-write loop never copies local values into the shared config.

**Parameters:**

- `working_directory` (optional, string): Directory to operate in

**Response keys:** `name`, `description`, `presets`, `profiles`, `builtins`, `includes`, `gitignore`,
`default_effort`, `default_effort_by_preset`, `rules_mode` and `rules_mode_by_preset`. The last four are
always present, empty when unset. When a `config.local.*` overlay exists, `local_overlay` reports
`{ "path": "...", "keys": ["presets", "mcp_servers.github.env.GITHUB_TOKEN", ...] }`: key paths only,
never values.

#### `update_config`

Update supported project configuration fields.

**Parameters:**

- `name` (optional, string): Project name
- `description` (optional, string): Project description
- `builtins` (optional, array): Builtin names to enable
- `gitignore` (optional, boolean): Whether generation updates `.gitignore`
- `default_effort` (optional, string): Default reasoning effort
- `default_effort_by_preset` (optional, object): Per-preset reasoning effort overrides
- `rules_mode` (optional, string): Default rules output mode, `split` or `inline`; empty string clears it
- `rules_mode_by_preset` (optional, object): Per-preset rules mode overrides. The map replaces the existing one; an entry with value `""` removes that preset's override, and `{}` or `null` clears them all
- `local` (optional, boolean): Write the supplied fields to the `config.local.*` overlay instead of the shared config. Clearing a field (empty string or empty map, including `name` and `description`) removes that key from the overlay instead of writing an empty value
- `working_directory` (optional, string): Directory to operate in

### Profile Tools

#### `add_profile`

Create a new profile with a set of domains.

**Parameters:**

- `name` (required, string): Profile name (unique identifier)
- `domains` (required, array): List of domain names to include in the profile
- `local` (optional, boolean): Define the profile in the `config.local.*` overlay; it may reference local domains

**Response:**

```json
{
  "success": true,
  "operation": "add_profile",
  "name": "full",
  "domains": ["backend", "frontend", "qa"],
  "message": "Profile added successfully"
}
```

#### `remove_profile`

Remove a profile from the configuration.

**Parameters:**

- `name` (required, string): Profile name to remove
- `local` (optional, boolean): Remove a profile defined in the overlay; a shared profile cannot be removed locally (error)

**Response:**

```json
{
  "success": true,
  "operation": "remove_profile",
  "name": "staging",
  "message": "Profile removed successfully"
}
```

#### `set_default_profile`

Set a profile as the default for generation.

**Parameters:**

- `name` (required, string): Profile name to set as default
- `local` (optional, boolean): Set `default` in the overlay instead of the shared config

**Response:**

```json
{
  "success": true,
  "operation": "set_default_profile",
  "name": "full",
  "message": "Default profile set successfully"
}
```

#### `list_profiles`

List all profiles in the configuration.

**Parameters:** (none)

**Response:**

```json
{
  "success": true,
  "operation": "list_profiles",
  "profiles": [
    {
      "Name": "full",
      "Domains": ["backend", "frontend", "qa"],
      "IsDefault": true
    },
    {
      "Name": "backend",
      "Domains": ["backend", "qa"],
      "IsDefault": false
    }
  ],
  "count": 2
}
```

---

## Common MCP Workflows

### Creating a Domain with Rules

```text
User: "Create a backend domain and add a database standards rule"

MCP Tool Sequence:
1. create_domain(name: "backend", description: "Backend services")
2. create_rule(name: "database-standards", domain: "backend", priority: "high", content: "...")
3. generate_outputs() - regenerate configurations
```

### Adding an External Include

```text
User: "Add our corporate rules from GitHub"

MCP Tool Sequence:
1. add_include(name: "corporate", source: "https://github.com/myorg/rules", ref: "main")
2. validate_config() - check if include is valid
3. generate_outputs() - regenerate with included content
```

### Setting Up Team Profiles

```text
User: "Create backend and frontend profiles for our team separation"

MCP Tool Sequence:
1. create_domain(name: "backend")
2. create_domain(name: "frontend")
3. add_profile(name: "backend", domains: ["backend"])
4. add_profile(name: "frontend", domains: ["frontend"])
5. set_default_profile(name: "backend")
6. generate_outputs()
```

### Bulk Content Creation

```text
User: "Add security rules to the backend domain"

MCP Tool Sequence:
1. create_rule(name: "authentication", domain: "backend", content: "...")
2. create_rule(name: "encryption", domain: "backend", content: "...")
3. create_context(name: "security-architecture", domain: "backend", content: "...")
4. validate_config()
5. generate_outputs()
```
