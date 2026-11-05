# Schema

ai-rulez publishes JSON Schemas for its configuration file and for every JSON document its commands print. Use the
configuration schema for editor completion and validation; use the report schemas to parse `--format json` output in
scripts and CI.

## Configuration schema

`schema/ai-rules.schema.json` describes `.ai-rulez/config.toml` (and a root `ai-rulez.toml`). The schema is embedded
in the CLI: `ai-rulez validate` converts the TOML to JSON, validates it against the schema, then runs the structural
checks.

```toml
# .ai-rulez/config.toml
version = "5.0"
name = "billing"
presets = ["cursor", "codex"]
preset = "claude"
```

```bash
ai-rulez validate
```

```text
Error: configuration has 1 unknown or invalid key(s) in .../.ai-rulez/config.toml

Validation errors:
  - .../.ai-rulez/config.toml: unknown key "preset" (did you mean "presets"?)

Hint: Fix the keys above; a "did you mean" names the closest known key
```

The schema sets `additionalProperties: false` at the top level, so a misspelled key is an error, not a silent no-op.

### Editor support

With the Even Better TOML extension in VS Code, associate the file with the schema in `.vscode/settings.json`:

```json
{
  "evenBetterToml.schema.associations": {
    ".ai-rulez/config.toml": "https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules.schema.json"
  }
}
```

The `main` URL tracks the development tip. To match the CLI you run, replace `main` with the release tag. The
configuration also accepts a `schema` key (the TOML spelling of `$schema`), so a file that carries a schema reference
still validates.

### Required keys

| Key | Type | Constraint |
| --- | ---- | ---------- |
| `version` | string | Must be `"5.0"`. An older version fails with a hint to run `ai-rulez migrate v5`. |
| `name` | string | Project name, non-empty. |

### Top-level keys

Every other key is optional. The table lists each key once; the linked page has the details.

| Key | Purpose | See |
| --- | ------- | --- |
| `description` | Project description | [Configuration](configuration.md) |
| `presets` | Harnesses to generate for (built-in names or custom preset objects) | [Supported harnesses](harnesses.md), [Custom presets](profiles.md) |
| `default`, `profiles` | Named domain combinations and the default one | [Domains & profiles](domains.md) |
| `default_layers` | `config.<name>.toml` layers always merged over this file | [Configuration](configuration.md) |
| `scopes` | Extra output roots, such as a subfolder `AGENTS.md` or `CLAUDE.md` | [Monorepo](monorepo.md) |
| `recursive_exclude` | Directories `generate --recursive` never searches | [Monorepo](monorepo.md) |
| `source` | Where content is read from: `ai-rulez` (default) or `agents` | [.agents as a source](agents-source.md) |
| `agents_md`, `agents_md_split` | Shared `AGENTS.md` and `.agents/skills` output; split `AGENTS.md` by heading | [AGENTS.md](agents-md.md), [.agents as a source](agents-source.md) |
| `rules`, `compact` | How rules are written (`split` or `inline`) and whether priority labels are shown | [Rules](rules.md) |
| `header` | Header of generated files | [Configuration](configuration.md) |
| `defaults` | Global and per-preset effort and model | [Configuration](configuration.md) |
| `builtins` | Built-in domains (`true`, `false`, or a list with `!` exclusions) | [Configuration](configuration.md) |
| `gitignore`, `gitignore_exclude` | Managed `.gitignore` block (off by default) and patterns kept out of it | [Configuration](configuration.md) |
| `skip_outputs` | Output kinds ai-rulez never writes, overwrites or deletes | [Configuration](configuration.md) |
| `codex_skills_dir`, `codex` | Codex skill directory and options | [Configuration](configuration.md) |
| `claude` | Claude Code options (`[claude.settings]`, `[claude.skills]`) | [Configuration](configuration.md) |
| `bundle_exclude` | Files left out of skill and command resource bundles | [Configuration](configuration.md) |
| `includes` | Content sources (git or local) to merge | [Includes](includes.md) |
| `installed_skills` | Skills installed from other repositories | [Installed skills](installed-skills.md) |
| `installed_marketplaces`, `installed_plugins`, `enabled_plugins` | Plugins the project imports or enables | [Installed plugins](installed-plugins.md) |
| `plugins`, `marketplaces` | Declarations only; no output is written for them | [Configuration](configuration.md) |
| `plugin`, `plugins_authoring`, `marketplace`, `marketplaces_authoring`, `placement` | Authoring distributable plugin bundles and marketplace indexes | [Authoring plugins](plugins.md) |
| `mcp_servers` | MCP server definitions written into each harness's MCP file | [Configuration](configuration.md#mcp_servers) |
| `mcp` | ai-rulez's own MCP entry and the settings of `ai-rulez mcp` | [MCP server](mcp-server.md#settings-in-the-configuration) |
| `skills`, `domains`, `delivery`, `skill_sources` | Dynamic skill loading: delivery defaults, the `dynamic-skills` stub, served skill sources | [MCP server](mcp-server.md#dynamic-skill-loading) |
| `search` | Ranking of `ai-rulez search` and `find_skill` | [Skill search](search.md) |
| `hooks`, `permissions`, `guard` | Hooks, permission rules and the generated-file guard | [Hooks and settings](settings.md), [Permissions](permissions.md) |
| `session_start` | `SessionStart` hook that runs generate in remote sessions | [Run generate in remote sessions](session-start.md) |
| `roles`, `role_manifest` | Roles and the `roles.json` manifest | [Roles](roles.md) |
| `lock` | Content pinning in `ai-rulez.lock` | [Lock file](lockfile.md) |
| `governance` | Which pinned content needs an approval | [Approvals](approvals.md) |
| `signing` | Who may sign the lock | [Signing](signing.md) |
| `catalog` | Defaults of `ai-rulez catalog` | [Catalog](catalog.md) |
| `lint` | Strict-validation settings | [Strict validation](strict-validation.md) |
| `verifiers`, `verifiers_settings` | Deterministic repository checks and their limits | [Verifiers](verifiers.md) |
| `exposure` | Session exposure and the area map | [Session exposure and areas](exposure.md) |
| `okf` | Open Knowledge Format bundle | [OKF](okf.md) |
| `llms_txt` | `llms.txt` output | [llms.txt](llms-txt.md) |
| `ard` | Agentic Resource Discovery manifest | [Agentic resource discovery](ard.md) |
| `publish` | What `ai-rulez publish` ships | [Publish](publish.md) |
| `usage` | Usage telemetry | [Usage telemetry](usage-telemetry.md) |
| `telemetry` | Item-load telemetry and OTLP export | [Item-load telemetry](telemetry.md) |
| `llm` | Model access for features that call a language model | [LLM access](llm.md) |
| `review`, `judge`, `improve` | Settings of `ai-rulez review`, `judge` and `improve` | [Review](review.md), [Judge](judge.md), [Improve](improve.md) |
| `$schema`, `schema`, `$comment` | Schema reference and a free-form note; ignored by generation | |

Some keys are honoured only from the user configuration or the environment, never from a repository (for example
the network and credential keys of `[llm]` and `[telemetry]`). The [trust model](trust-model.md) lists them.

### Content frontmatter

Rule, context and skill files are markdown with optional YAML frontmatter. Two keys are constrained:

| Key | Values |
| --- | ------ |
| `priority` | `critical`, `high`, `medium` (default), `low`, `minimal` |
| `targets` | Preset names (`claude`), root files (`CLAUDE.md`), paths or base names, directory prefixes (`.cursor/rules/`), globs (`.cursor/rules/*`) or `*`. See [Targets](rules.md#targets). |

```markdown
---
priority: high
targets: ["cursor", "CLAUDE.md"]
---

# Money types

Store amounts as integer cents.
```

## Local overlay schema

`schema/ai-rules-local.schema.json` describes the machine-local `config.local.toml`. It accepts the same keys as the
main schema without requiring `version` and `name`, and adds the overlay-only markers: `remove = true` on a named list
entry and `"!name"` in `presets`. `ai-rulez validate`, `ai-rulez local set` and the MCP `validate_config` tool check
the overlay against it. See [Local configuration](local-overrides.md#config-overlay).

## Report and file schemas

The documents carry a `schema_version` field (`schema` in the plan document) where the Version column gives a
number. The schemas below describe those documents and the files ai-rulez reads or writes. A failure that happens
before a command has a report prints the error document instead; see [JSON contracts](cli.md#json-contracts).

### Configuration and declarations

| File | Version | Describes |
| ---- | ------- | --------- |
| `schema/ai-rules.schema.json` | config `"5.0"` | `.ai-rulez/config.toml` and `ai-rulez.toml` |
| `schema/ai-rules-local.schema.json` | config `"5.0"` | `config.local.toml` overlay |
| `schema/provider.schema.json` | none | Declarative provider spec of a custom preset; see [Custom presets](profiles.md) |
| `schema/policy.schema.json` | `policy_version` | Organization policy file (`--policy`, `AI_RULEZ_POLICY`, the managed path); see [Organization policy](policy.md) |
| `schema/verifiers-spec.schema.json` | none | `.ai-rulez/verifiers/*.toml` declaration files; see [Verifiers](verifiers.md) |
| `schema/eval-case.schema.json` | 1 | `*.eval.yaml`, `.yml` and `.json` skill eval cases; see [Evals](evals.md) |

### Generation, validation and cost

| File | Version | Describes |
| ---- | ------- | --------- |
| `schema/plan.schema.json` | 1 | `generate --emit-plan FILE` and `Plan.JSON()` of the [Go API](embedding.md): every file a run would write, merge or remove, with digests; see [Embedding the plan](cli.md#embedding-the-plan) |
| `schema/validate-report.schema.json` | 1 | `validate --format json` and `scan --format json` |
| `schema/doctor-report.schema.json` | 1 | `doctor --format json` |
| `schema/convert-report.schema.json` | 1 | `convert --format json`: each input construct as mapped, approximated or dropped |
| `schema/change-result.schema.json` | 1 | `add`, `remove`, `edit`, `domain`, `profile`, `include`, `skill install` and `skill remove` with `--format json` |
| `schema/tokens-report.schema.json` | 1 | `tokens --format json` |
| `schema/cost-report.schema.json` | 1 | `cost --format json` |
| `schema/cache-status.schema.json` | 1 | `cache status --format json`: where the index cache is, its size and entry counts |
| `schema/cache-clear.schema.json` | 1 | `cache clear --format json`: the cache directories removed |
| `schema/error-document.schema.json` | 1 | What a `--format json` command prints when it fails before it has a report |

### Lock, approvals and signing

| File | Version | Describes |
| ---- | ------- | --------- |
| `schema/lock-diff.schema.json` | 1 | `lock --diff --format json` and `lock --check --format json`; see [Lock file](lockfile.md) |
| `schema/lock-outdated.schema.json` | 1 | `lock --outdated --format json`: sources with a version constraint and the newer tags they could move to |
| `schema/lock-subject.schema.json` | 1 | `lock --subject --format json`: the digest a signature commits to |
| `schema/update.schema.json` | 1 | `update --format json`: the pins that move, or would with `--dry-run` |
| `schema/approve-list.schema.json` | 1 | `approve --list --format json`; see [Approvals](approvals.md) |
| `schema/verify-approvals.schema.json` | 1 | `verify --approvals --format json` |
| `schema/verify-attestation.schema.json` | 1 | `verify --attestation --format json` |
| `schema/sign-report.schema.json` | 1 | `sign --format json`; see [Signing](signing.md) |
| `schema/policy-effective.schema.json` | 1 | `validate --show-policy --format json`: the policy layers, the origin of every value and what the repository tried to loosen |
| `schema/sbom-report.schema.json` | 1 | `sbom --format json` when the run reports instead of printing the SBOM (a failed gate, `--check`, `-o`); see [SBOM](sbom.md) |

### Roles and catalog

| File | Version | Describes |
| ---- | ------- | --------- |
| `schema/roles-manifest.schema.json` | 1 | `roles.json` and `roles list --format json`; see [Roles](roles.md) |
| `schema/roles-show.schema.json` | 1 | `roles show <name> --format json` |
| `schema/roles-resolve.schema.json` | 1 | `roles resolve <name> --format json` |
| `schema/catalog.v1.schema.json` | 1 | `catalog --format json` (the default) |
| `schema/catalog.schema.json` | 2 | `catalog --format json --schema-version 2` and the `catalog.json` of `catalog --html`; see [Catalog](catalog.md) |
| `schema/catalog-diff.schema.json` | 1 | `catalog diff --format json` |

### Skills, evals and review

| File | Version | Describes |
| ---- | ------- | --------- |
| `schema/search.v1.schema.json` | 1 | `search <query> --format json`; see [Skill search](search.md) |
| `schema/search-eval.v1.schema.json` | 1 | `search --eval <cases.yaml> --format json`, also the file `--output` writes |
| `schema/eval-report.schema.json` | 1 | `eval run --format json` |
| `schema/eval-activation.v1.schema.json` | 1 | `eval run --mode activation --format json`; see [Activation mode](evals.md#activation-mode) |
| `schema/review-report.schema.json` | 1 | `review --format json`; see [Review](review.md) |
| `schema/judge-report.schema.json` | 2 | `judge --format json`; see [Judge](judge.md) |
| `schema/improve-report.schema.json` | 1 | `report.json` of an `improve` run; see [Improve](improve.md) |
| `schema/okf-validate.schema.json` | 1 | `okf validate --format json`; see [OKF](okf.md) |
| `schema/export-okf.schema.json` | 1 | `export okf --format json`: the files written, or the drift of `--check` |

### Session exposure

| File | Version | Describes |
| ---- | ------- | --------- |
| `schema/exposure-report.v1.schema.json` | 1 | `exposure --format json`; see [Session exposure and areas](exposure.md) |
| `schema/exposure-inventory.v1.schema.json` | 1 | `exposure inventory --format json` |
| `schema/exposure-areas.v1.schema.json` | 1 | `exposure areas --format json` |
| `schema/exposure-collisions.v1.schema.json` | 1 | `exposure collisions --format json` |
| `schema/exposure-leakage.v1.schema.json` | 1 | `exposure leakage --format json` |

### Verifiers, scanners and telemetry

| File | Version | Describes |
| ---- | ------- | --------- |
| `schema/verifiers-report.schema.json` | none | `verifiers run --format json` and the MCP `run_verifiers` result; see [Verifiers](verifiers.md) |
| `schema/verifiers-list.schema.json` | 1 | `verifiers list --format json` |
| `schema/verifiers-test.schema.json` | 1 | `verifiers test --format json` |
| `schema/verifiers-explain.schema.json` | 1 | `verifiers explain <name> --format json` |
| `schema/scanners-list.schema.json` | 1 | `scanners list --format json` |
| `schema/scanners-doctor.schema.json` | 1 | `scanners doctor --format json` |
| `schema/telemetry-doctor.schema.json` | 1 | `telemetry doctor --format json`; see [Item-load telemetry](telemetry.md) |

### Publishing and plugins

| File | Version | Describes |
| ---- | ------- | --------- |
| `schema/publish-manifest.schema.json` | 1 | `<name>-<version>.manifest.json` written by `publish`: the bundle's files, source, lock and digests; see [Publish](publish.md) |
| `schema/publish-plan.schema.json` | 1 | `publish-plan.json` and `publish --format json` |
| `schema/publish-verify.schema.json` | 1 | `publish verify --format json` |
| `schema/publish-emit.schema.json` | 1 | `publish emit <emitter> --format json` |
| `schema/verify-plugin.schema.json` | 1 | `verify --plugin --format json`; see [Authoring plugins](plugins.md) |

Each schema's `$id` is its URL on the `main` branch, for example
`https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/validate-report.schema.json`.

## Validating

```bash
ai-rulez validate                              # the configuration found from the current directory
ai-rulez -C .ai-rulez/config.toml validate     # a specific file
ai-rulez validate --strict                     # also fail on warnings
```

A configuration with an unsupported `version` fails before any other check:

```text
Error: config version "4.0" is no longer supported: run `ai-rulez migrate v5`

Hint: Run `ai-rulez migrate v5` to rewrite this configuration for v5 (add --dry-run to preview the change list)
```

See [Strict validation](strict-validation.md) for the content checks and their codes.
