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
- **`placement`**: Whether skills and commands are `core` (written to the harness trees) or `plugin`-only; see [Plugins](plugins.md)
- **`claude`**: Claude Code options: `[claude.settings]` (`manage`, `managed` entries of `.claude/settings.json`) and `[claude.skills]` (`hide_from_menu`)
- **`codex`**: Codex options, such as `project_doc_max_bytes` for the `AGENTS.md` size warning
- **`codex_skills_dir`**: Directory the `codex` preset writes skills to (default `.agents/skills`)
- **`bundle_exclude`**: Extra patterns left out when skill and command resources are copied or listed
- **`hooks`**: Top-level `[[hooks]]` rendered into each harness's native hook format; see [Hooks and permissions](settings.md)
- **`permissions`**: `allow`, `ask` and `deny` rules translated for each harness with a permission surface; see [Permissions](permissions.md)
- **`guard`**: `[guard] generated = true` adds a PreToolUse hook that blocks agent edits to generated files (`version`, `command` tune how it launches); see [Settings](settings.md#guard)
- **`roles`**: `[[roles]]` map a job to a slice of the shared content; see [Roles](roles.md)
- **`role_manifest`**: `enabled` writes `roles.json`; `skill_mode_fallback` (`drop` or `serve`) decides what an `off` skill does on a harness that cannot hide it
- **`domains`**: Per-domain settings keyed by domain name (`[domains.<name>] delivery`)
- **`skills`**: Defaults for every skill (`[skills] delivery = "static" | "served" | "both"`); see [MCP server](mcp-server.md#dynamic-skill-loading)
- **`skill_sources`**: `[[skill_sources]]` remote or local skill sources that are served over MCP and pinned in the lock
- **`lock`**: `[lock]` content pinning (`enforce`, `include_outputs`, `scope`); see [Lock file](lockfile.md)
- **`lint`**: `[lint]` strict-validation settings (severities, ignores, budgets, `tolerate`, `security`, `evals`, `traps`, `metadata`, `external` scanners); see [Strict validation](strict-validation.md)
- **`verifiers`**: `[[verifiers]]` deterministic repository checks run by `ai-rulez verifiers run`; see [Verifiers](verifiers.md)
- **`okf`**: `[okf]` bundle options (`dir`, `include`, `index_style`, `spec`); see [OKF](okf.md)
- **`usage`**: `[usage] skills_index` writes `skills-index.json` for usage reports; see [Usage telemetry](usage-telemetry.md)
- **`llm`**: `[llm]` model access (`backend`, `model`, `base_url`, `api_key_env`, budgets, `allow_network`, ...); network and credential keys are honoured only from user scope; see [LLM access](llm.md)
- **`telemetry`**: `[telemetry]` item-load telemetry and OTLP export; egress keys, `service_name` and `resource` are honoured only from user scope; see [Telemetry](telemetry.md)

Which of these a repository may set, and which only the user config file or the environment may enable, is in the [trust model](trust-model.md).

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
| `schema/catalog.schema.json`      | JSON Schema | v2      | `ai-rulez catalog --format json --schema-version 2` and the `catalog.json` of `catalog --html`; see [Catalog](catalog.md) |
| `schema/catalog.v1.schema.json`   | JSON Schema | v1      | `ai-rulez catalog --format json` (the default until the next minor release) |
| `schema/plan.schema.json`         | JSON Schema | v1      | `ai-rulez generate --emit-plan FILE` and `generator.PlanOutputs`: every file a run would write, merge or remove, with digests and no secrets; see [Embedding the plan](cli.md#embedding-the-plan) |
| `schema/lock-diff.schema.json`    | JSON Schema | v1      | `ai-rulez lock --diff --format json`; see [Lock file](lockfile.md) |
| `schema/lock-outdated.schema.json` | JSON Schema | v1     | `ai-rulez lock --outdated --format json`: sources with a version constraint and the newer tags they could move to |
| `schema/lock-subject.schema.json` | JSON Schema | v1      | `ai-rulez lock --subject --format json`: the lock-subject digest a signature commits to |
| `schema/update.schema.json`       | JSON Schema | v1      | `ai-rulez update --format json`: the pins that move, or would with `--dry-run` |
| `schema/search.v1.schema.json`    | JSON Schema | v1      | `ai-rulez search <query> --format json`: served skills ranked against a query |
| `schema/search-eval.v1.schema.json` | JSON Schema | v1    | `ai-rulez search --eval <cases.yaml> --format json`, also the file `--out` writes |
| `schema/convert-report.schema.json` | JSON Schema | v1    | `ai-rulez convert --format json`: every input construct as mapped, approximated or dropped |
| `schema/eval-case.schema.json`    | JSON Schema | v1      | `*.eval.yaml` / `.yml` / `.json` skill eval cases under `skills/<name>/` |
| `schema/verifiers-report.schema.json` | JSON Schema | v1  | `ai-rulez verifiers run --format json` and the MCP `run_verifiers` result; see [Verifiers](verifiers.md) |
| `schema/verifiers-spec.schema.json` | JSON Schema | v1    | `.ai-rulez/verifiers/*.toml` declaration files; see [Verifiers](verifiers.md#rule-linked-verifiers) |

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
