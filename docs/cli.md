# CLI Reference

All AI-Rulez CLI commands and flags.

## Command Overview

### Core Commands

| Command                         | Description                                         |
| ------------------------------- | --------------------------------------------------- |
| `ai-rulez init`                 | Initialize directory-based configuration            |
| `ai-rulez convert`              | Convert existing tool files into `.ai-rulez/` with a lossiness report ([details](#convert-command)) |
| `ai-rulez generate`             | Generate presets for specific profile               |
| `ai-rulez clean`                | Remove files produced by `generate`                 |
| `ai-rulez validate`             | Validate configuration                              |
| `ai-rulez cost`                 | Report the biggest context-cost offenders           |
| `ai-rulez verify`               | Verify generated files against their hashes (`--plugin` for plugin bundles) |
| `ai-rulez lock`                 | Pin remote includes, installed skills and authored content in `ai-rulez.lock` ([Lock file](lockfile.md)) |
| `ai-rulez sign`                 | Sign the lock, a plugin bundle, a skill, an SBOM or a policy into a Sigstore bundle ([details](#sign-command)) |
| `ai-rulez trust update`         | Cache the Sigstore trusted root used to verify keyless attestations ([details](#trust-command)) |
| `ai-rulez approve`              | Record, list and revoke reviewer approvals bound to a content digest ([Approvals](approvals.md)) |
| `ai-rulez update`               | Move pins of sources that use a `version` range to the newest allowed tag ([details](#update-command)) |
| `ai-rulez roles`                | List, show and resolve `[[roles]]` ([Roles](roles.md)) |
| `ai-rulez catalog`              | Items with owner, version, tokens, roles and lock status (`--format json`); `--html <dir>` writes a static site, `catalog diff` compares two catalogs ([Catalog](catalog.md)) |
| `ai-rulez sbom`                 | CycloneDX 1.6 or SPDX 2.3 bill of materials of the AI configuration ([SBOM](sbom.md)) |
| `ai-rulez publish`              | Deterministic, checksummed release artifacts of the plugin bundle, optionally signed; `--to github-release\|npm\|oci --execute --yes` uploads ([Publish](publish.md)) |
| `ai-rulez doctor`               | Read-only diagnostics for the project's setup ([details](#doctor-command)) |
| `ai-rulez guard`                | Hidden PreToolUse hook that blocks agent edits to generated files ([details](#guard-command)) |
| `ai-rulez llm doctor` / `llm estimate` | Inspect the `[llm]` model-access setup and estimate prompt cost, without calling a model ([details](llm.md)) |
| `ai-rulez verifiers run/list/explain/test/calibrate/suggest` | Run the deterministic repo checks declared as `[[verifiers]]` or under `.ai-rulez/verifiers/` ([details](#verifiers-command)) |
| `ai-rulez scan`                 | Security checks on skills, rules and scripts         |
| `ai-rulez scanners list/doctor` | Inspect the `[[lint.external]]` scanners ([details](#scan-command)) |
| `ai-rulez migrate v5`           | Migrate a 4.x project to 5.0 ([details](#migrate-command)) |
| `ai-rulez tokens`               | Report the prompt-token cost of generated artifacts |
| `ai-rulez search`               | Rank skills against a query (lexical or hybrid with embeddings); `index`, `status`, `mine`; `--eval` measures the ranking ([details](#search-command)) |
| `ai-rulez eval run`             | Run skill evals and score them ([details](#eval-commands)) |
| `ai-rulez eval import`          | Import eval scenarios from another tool as eval cases ([details](evals.md#importing-scenarios)) |
| `ai-rulez eval calibrate-estimate` | Propose cost-estimate assumptions measured from recorded runs ([details](evals.md#calibrating-the-estimate)) |
| `ai-rulez review` / `rubric`    | Score skills against a rubric: offline, or with an LLM judge (`--semantic`), calibration, a calibrated gate and `review fix` ([details](#review-commands)) |
| `ai-rulez improve`              | (experimental) Improve a skill with an external optimizer behind a held-out eval gate ([Improve](improve.md)) |
| `ai-rulez usage` / `report`     | Opt-in usage log, feedback and reports ([details](#usage-commands)) |
| `ai-rulez telemetry`            | Item-load telemetry and opt-in OTLP export ([details](#usage-commands)) |
| `ai-rulez export okf` / `import okf` / `okf validate` | Open Knowledge Format bundles ([details](#okf-commands)) |
| `ai-rulez llm`                  | Inspect the `[llm]` setup ([details](#llm-commands)) |
| `ai-rulez version`              | Show version                                        |
| `ai-rulez mcp`                  | Start MCP server (`--serve-skills` serves skills read-only) |
| `ai-rulez local`                | Manage the machine-local config overlay ([details](#local-configuration)) |
| `ai-rulez builtins list`        | List available built-in domains                     |
| `ai-rulez builtins show <name>` | Show bundled content for a built-in domain          |

Aliases: `generate` → `gen`, `g`; `clean` → `clear`; `validate` → `val`, `v`, `check`.

### CRUD Commands (Configuration Management)

| Command                                           | Description              |
| ------------------------------------------------- | ------------------------ |
| `ai-rulez domain add/remove/list`                 | Manage domains           |
| `ai-rulez add rule/context/skill/agent/command/check` | Create content files |
| `ai-rulez remove rule/context/skill/agent/command/check` | Delete content files |
| `ai-rulez list rules/context/skills/agents/commands/checks` | List content files |
| `ai-rulez include add/remove/list`                | Manage external includes |
| `ai-rulez skill install/remove/list/update`       | Manage installed skills; `update` re-pins them in `ai-rulez.lock` |
| `ai-rulez profile add/remove/list`                | Manage profiles          |
| `ai-rulez profile set-default`                    | Set default profile      |

`add`, `remove` and `list` take `--local` to work on the machine-local tree `.ai-rulez/local/`;
`include add|remove`, `skill install|remove` and `profile add|remove|set-default` take `--local` to
write to the `config.local.*` overlay. See [Local Configuration](local-overrides.md).

## CRUD Commands

AI-Rulez provides CRUD commands to programmatically modify your `.ai-rulez/` configuration. These commands allow you to create domains, add rules/context/skills/agents/commands, manage includes, and organize profiles.

`add agent` and `add command` take `--domain`/`-d`, `--description`/`-s`, `--content`/`-c` and
`--local`. `remove agent|command` and `list agents|commands` take the same flags as their rule/skill
counterparts.

`add check`, `remove check` and `list checks` manage code-review guidelines (see [Checks](checks.md));
they take `--domain`/`-d` (and `--description`/`-s`, `--content`/`-c` for `add`; `--force`/`-f` for `remove`) but have no `--local`.

`ai-rulez list --placement [--profile <name>]` prints where every skill and command ends up (core or plugin-only),
the plugins that bundle it, and flags plugin-only items nothing makes reachable.

`ai-rulez skill update [name...]` re-resolves the named installed skills (all without names) and records the commit
and digest in `ai-rulez.lock`; it is `ai-rulez lock --kind skill`. A skill with a `version` range keeps a pin that still
satisfies it; [`ai-rulez update`](#update-command) moves range pins.

### Domain Management

#### `ai-rulez domain add <name> [flags]`

Create a new domain with subdirectories for rules, context, and skills.

**Syntax:**

```bash
ai-rulez domain add <name> [flags]
```

**Arguments:**

- `<name>` (required): Domain name. Alphanumeric, hyphens, and underscores, 1-50 characters; must start and end with an alphanumeric character.

**Flags:**

- `--description <text>` / `-s` (optional): Description of the domain

**Examples:**

Create a backend domain:

```bash
ai-rulez domain add backend
```

Create a domain with description:

```bash
ai-rulez domain add frontend --description "React web application"
```

#### `ai-rulez domain remove <name> [flags]`

Delete a domain and all its contents.

**Syntax:**

```bash
ai-rulez domain remove <name> [flags]
```

**Arguments:**

- `<name>` (required): Domain name to delete

**Flags:**

- `--yes` / `-y` (optional): Skip confirmation prompt

**Examples:**

Remove a domain (with confirmation):

```bash
ai-rulez domain remove backend
```

Remove without confirmation:

```bash
ai-rulez domain remove backend --yes
```

#### `ai-rulez domain list [flags]`

List all domains in the `.ai-rulez/` directory.

**Syntax:**

```bash
ai-rulez domain list [flags]
```

**Flags:**

- `--format text|json` (optional, default `text`): `json` prints JSON.

**Examples:**

List domains:

```bash
ai-rulez domain list
```

List as JSON (a wrapper document, `{"schema_version": 1, "items": [...]}`):

```bash
ai-rulez domain list --format json
```

### Content Management

Rules, context, and skills can be added to the root (always included) or to specific domains (profile-dependent).

#### `ai-rulez add rule <name> [flags]`

Create a new rule file with optional YAML frontmatter.

**Syntax:**

```bash
ai-rulez add rule <name> [flags]
```

**Arguments:**

- `<name>` (required): Rule filename without `.md` extension

**Flags:**

- `--domain <name>` / `-d` (optional): Domain name. If not specified, creates in root rules directory
- `--local` (optional): Write to the machine-local tree `.ai-rulez/local/rules/` instead of the shared tree. The output is gitignored and never committed. Combine with `--domain` to write to `.ai-rulez/local/domains/<name>/rules/`. See [Local Configuration](local-overrides.md).
- `--priority <level>` / `-p` (optional): Priority: critical, high, medium, low, minimal. Default: medium
- `--targets <list>` / `-t` (optional): Comma-separated list of target providers (claude, cursor, etc.)
- `--content <text>` / `-c` (optional): Rule content. Uses a template if omitted.

**Examples:**

Create a root rule:

```bash
ai-rulez add rule code-quality
```

Create a domain-specific rule:

```bash
ai-rulez add rule database-standards --domain backend --priority high
```

Create a machine-local override rule (gitignored, personal to this checkout):

```bash
ai-rulez add rule my-scratch-notes --local
```

Create with specific targets:

```bash
ai-rulez add rule performance --targets claude,cursor --priority high
```

#### `ai-rulez add context <name> [flags]`

Create a new context file (documentation/reference material).

**Syntax:**

```bash
ai-rulez add context <name> [flags]
```

**Arguments:**

- `<name>` (required): Context filename without `.md` extension

**Flags:**

- `--domain <name>` / `-d` (optional): Domain name. If not specified, creates in root context directory
- `--local` (optional): Write to the machine-local tree `.ai-rulez/local/context/` instead of the shared tree. The output is gitignored and never committed. Combine with `--domain` to write to `.ai-rulez/local/domains/<name>/context/`. See [Local Configuration](local-overrides.md).
- `--priority <level>` / `-p` (optional): Priority: critical, high, medium, low, minimal. Default: medium
- `--content <text>` / `-c` (optional): Context content. Uses a template if omitted.

**Examples:**

Create root context:

```bash
ai-rulez add context architecture
```

Create domain context:

```bash
ai-rulez add context database-design --domain backend
```

Create a machine-local override context (gitignored, personal to this checkout):

```bash
ai-rulez add context local-env-notes --local
```

#### `ai-rulez add skill <name> [flags]`

Create a new skill file (AI prompt/expert definition).

**Syntax:**

```bash
ai-rulez add skill <name> [flags]
```

**Arguments:**

- `<name>` (required): Skill filename without `.md` extension

**Flags:**

- `--domain <name>` / `-d` (optional): Domain name. If not specified, creates in root skills directory
- `--description <text>` / `-s` (optional): Skill description
- `--local` (optional): Write to the machine-local tree `.ai-rulez/local/skills/` (gitignored)
- `--content <text>` / `-c` (optional): Skill content. Uses a template if omitted.

**Examples:**

Create a root skill:

```bash
ai-rulez add skill code-reviewer
```

Create a domain-specific skill:

```bash
ai-rulez add skill performance-optimizer --domain backend --description "Optimization workflow"
```

#### `ai-rulez remove rule <name> [flags]`

Delete a rule file.

**Syntax:**

```bash
ai-rulez remove rule <name> [flags]
```

**Arguments:**

- `<name>` (required): Rule filename without `.md` extension

**Flags:**

- `--domain <name>` / `-d` (optional): Domain name
- `--yes` / `-y` (optional): Skip confirmation
- `--local` (optional): Remove from the machine-local tree `.ai-rulez/local/`

**Examples:**

Remove a root rule:

```bash
ai-rulez remove rule code-quality
```

Remove a domain rule:

```bash
ai-rulez remove rule database-standards --domain backend --yes
```

#### `ai-rulez remove context <name> [flags]`

Delete a context file.

**Syntax:**

```bash
ai-rulez remove context <name> [flags]
```

**Arguments:**

- `<name>` (required): Context filename without `.md` extension

**Flags:**

- `--domain <name>` / `-d` (optional): Domain name
- `--yes` / `-y` (optional): Skip confirmation
- `--local` (optional): Remove from the machine-local tree `.ai-rulez/local/`

**Examples:**

```bash
ai-rulez remove context architecture
ai-rulez remove context backend-design --domain backend --yes
```

#### `ai-rulez remove skill <name> [flags]`

Delete a skill file.

**Syntax:**

```bash
ai-rulez remove skill <name> [flags]
```

**Arguments:**

- `<name>` (required): Skill filename without `.md` extension

**Flags:**

- `--domain <name>` / `-d` (optional): Domain name
- `--yes` / `-y` (optional): Skip confirmation
- `--local` (optional): Remove from the machine-local tree `.ai-rulez/local/`

**Examples:**

```bash
ai-rulez remove skill code-reviewer
ai-rulez remove skill performance-optimizer --domain backend --yes
```

#### `ai-rulez list rules [flags]`

List all rule files.

**Syntax:**

```bash
ai-rulez list rules [flags]
```

**Flags:**

- `--domain <name>` / `-d` (optional): List rules in specific domain only
- `--format text|json` (optional, default `text`): `json` prints JSON.
- `--local` (optional): List the machine-local tree `.ai-rulez/local/` instead of the shared content

**Examples:**

List all rules:

```bash
ai-rulez list rules
```

List domain rules:

```bash
ai-rulez list rules --domain backend
```

#### `ai-rulez list context [flags]`

List all context files.

**Syntax:**

```bash
ai-rulez list context [flags]
```

**Flags:**

- `--domain <name>` / `-d` (optional): List context in specific domain only
- `--format text|json` (optional, default `text`): `json` prints JSON.
- `--local` (optional): List the machine-local tree `.ai-rulez/local/` instead of the shared content

**Examples:**

```bash
ai-rulez list context
ai-rulez list context --domain backend
```

#### `ai-rulez list skills [flags]`

List all skill files.

**Syntax:**

```bash
ai-rulez list skills [flags]
```

**Flags:**

- `--domain <name>` / `-d` (optional): List skills in specific domain only
- `--format text|json` (optional, default `text`): `json` prints JSON.
- `--local` (optional): List the machine-local tree `.ai-rulez/local/` instead of the shared content

**Examples:**

```bash
ai-rulez list skills
ai-rulez list skills --domain backend
```

### Installed Skill Management

Install named skills from external repositories. See [Installed Skills](installed-skills.md) for full details.

#### `ai-rulez skill install <name> --source <url> [flags]`

Install a named skill from a git repository or local path.

**Arguments:**

- `<name>` (required): Skill name (unique identifier)

**Flags:**

- `--source <url>` / `-s` (required): Git URL or local path
- `--path <dir>` / `-p` (optional): Path within repo to skill directory (defaults to `skills/<name>`)
- `--ref <ref>` / `-r` (optional): Git reference (branch, tag, commit)
- `--local` (optional): Record the skill in the machine-local `config.local.*` overlay instead of the shared config

**Examples:**

```bash
ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg
ai-rulez skill install my-lib --source https://github.com/org/repo --path custom/path --ref v2.0
ai-rulez skill install local-skill --source ../my-other-repo
```

#### `ai-rulez skill remove <name> [flags]`

Remove an installed skill from the configuration.

**Arguments:**

- `<name>` (required): Skill name to remove

**Flags:**

- `--yes` / `-y` (optional): Skip confirmation prompt
- `--local` (optional): Remove through the `config.local.*` overlay. A skill installed in the shared config is hidden on this machine with `remove = true`

**Examples:**

```bash
ai-rulez skill remove kreuzberg
ai-rulez skill remove my-lib --yes
```

#### `ai-rulez skill list [flags]`

List all installed skills.

**Flags:**

- `--format text|json` (optional, default `text`): `json` prints JSON.

**Examples:**

```bash
ai-rulez skill list
ai-rulez skill list --format json
```

### Include Management

Manage external rule sources (git repositories or local packages).

#### `ai-rulez include add <name> <source> [flags]`

Add a new include source to the configuration.

**Syntax:**

```bash
ai-rulez include add <name> <source> [flags]
```

**Arguments:**

- `<name>` (required): Unique identifier for this include
- `<source>` (required): Git URL (e.g., `https://github.com/org/repo`) or local path (e.g., `./packages/shared`)

**Flags:**

- `--path <dir>` / `-p` (optional): Path within git repository where `.ai-rulez/` content is located
- `--ref <branch>` / `-r` (optional): Git reference (branch, tag, commit). Defaults to the repository's default branch (`HEAD`), not necessarily `main`.
- `--include <types>` / `-i` (optional): Comma-separated content types (default `rules,context,skills`): `rules,context,skills,agents,commands`
- `--merge-strategy <strategy>` / `-m` (optional): Merge strategy: `local-override` (default), `include-override`, or `error`
- `--install-to <path>` / `-t` (optional): Installation target path in `.ai-rulez/`
- `--local` (optional): Add the include to the machine-local `config.local.*` overlay instead of the shared config

**Examples:**

Add a git-based include:

```bash
ai-rulez include add corporate-rules https://github.com/myorg/shared-rules
```

Add with custom path and ref:

```bash
ai-rulez include add shared-patterns https://github.com/myorg/repo --path .ai-rulez --ref develop
```

Add local include:

```bash
ai-rulez include add backend-package ./packages/backend
```

#### `ai-rulez include remove <name> [flags]`

Remove an include source.

**Syntax:**

```bash
ai-rulez include remove <name> [flags]
```

**Arguments:**

- `<name>` (required): Include name to remove

**Flags:**

- `--yes` / `-y` (optional): Skip confirmation
- `--local` (optional): Remove through the `config.local.*` overlay. An include defined in the shared config is hidden on this machine with `remove = true`

**Examples:**

```bash
ai-rulez include remove corporate-rules
ai-rulez include remove shared-patterns --yes
```

#### `ai-rulez include list [flags]`

List all include sources.

**Syntax:**

```bash
ai-rulez include list [flags]
```

**Flags:**

- `--format text|json` (optional, default `text`): `json` prints JSON.

**Examples:**

```bash
ai-rulez include list
ai-rulez include list --format json
```

### Profile Management

Organize domains into named profiles for targeted generation.

#### `ai-rulez profile add <name> <domains...> [flags]`

Create a new profile.

**Syntax:**

```bash
ai-rulez profile add <name> <domains...> [flags]
```

**Arguments:**

- `<name>` (required): Profile name
- `<domains...>` (required): Space-separated list of domain names to include

**Flags:**

- `--set-default` / `-s` (optional): Set this as the default profile
- `--local` (optional): Define the profile in the machine-local `config.local.*` overlay. It may reference local domains

**Examples:**

Create a backend profile:

```bash
ai-rulez profile add backend backend qa
```

Create and set as default:

```bash
ai-rulez profile add full backend frontend qa --set-default
```

#### `ai-rulez profile remove <name> [flags]`

Delete a profile.

**Syntax:**

```bash
ai-rulez profile remove <name> [flags]
```

**Arguments:**

- `<name>` (required): Profile name to remove

**Flags:**

- `--yes` / `-y` (optional): Skip confirmation
- `--local` (optional): Remove a profile defined in the overlay. A profile from the shared config cannot be removed locally (error)

**Examples:**

```bash
ai-rulez profile remove staging
ai-rulez profile remove development --yes
```

#### `ai-rulez profile set-default <name> [flags]`

Set a profile as the default for generation.

**Syntax:**

```bash
ai-rulez profile set-default <name> [flags]
```

**Arguments:**

- `<name>` (required): Profile name to set as default

**Flags:**

- `--local` (optional): Set `default` in the machine-local overlay instead of the shared config

**Examples:**

```bash
ai-rulez profile set-default full
```

#### `ai-rulez profile list [flags]`

List all profiles.

**Syntax:**

```bash
ai-rulez profile list [flags]
```

**Flags:**

- `--format text|json` (optional, default `text`): `json` prints JSON.

**Examples:**

```bash
ai-rulez profile list
ai-rulez profile list --format json
```

---

## Local Configuration

Machine-local configuration has two layers: the `.ai-rulez/local/` content tree (managed with `--local`
on `add`, `remove` and `list`) and the `config.local.toml` overlay (managed with
`ai-rulez local` and `--local` on the config commands). The full guide is
[Local Configuration](local-overrides.md); this section is the command reference.

### `ai-rulez local <subcommand>`

Manage `config.local.toml`, the machine-local overlay merged onto the shared config at load time. It sits beside `config.toml`, is gitignored, and is never written into the shared file.

```bash
ai-rulez local init                     # commented config.local.toml skeleton
ai-rulez local show [--format json] [--reveal] # keys the overlay sets, with the shared value each replaces
ai-rulez local set <path> <value>       # value is a TOML literal, falling back to a plain string
ai-rulez local set <path> --stdin       # read the value from standard input
ai-rulez local unset <path>
ai-rulez local path
```

The subcommands honour the global `--config` / `-C` and their own persistent `--config-dir` / `-n`.

Paths are dotted keys. For lists of named entries (`mcp_servers`, `plugins`, `includes`, `installed_skills`, `marketplaces`, `scopes`) the segment after the list is the entry name. A segment that contains a dot is written in brackets with double quotes:

```bash
ai-rulez local set default dev
ai-rulez local set presets '["codex", "!cursor"]'
ai-rulez local set mcp_servers.github.command npx
ai-rulez local set 'mcp_servers["foo.bar"].command' npx
printf %s "$GITHUB_TOKEN" | ai-rulez local set mcp_servers.github.env.GITHUB_TOKEN --stdin
```

Pass secrets with `--stdin`, not as an argument, so they stay out of shell history and the process list.

Every change is validated against the local schema and the merged config (offline: remote includes are read from cache only) and rolled back if it is invalid. A new MCP server must have a `command` or `url`. Edits take an advisory lock (`.config.local.lock`) so two commands cannot interleave.

`local show` is default-deny: it prints values only for keys known to hold no credentials (`name`, `description`, `default`, `presets`, `gitignore`, `compact`, `builtins`, profile names, the enum and bool settings under `defaults`, `rules` and `header` (`style`, `hashes`, `timestamp`), an entry's own `transport`, `enabled` and `remove`, `mcp.self_server`, `mcp.self_server_version`); every other key is listed by path with `<redacted>`, on both the overlay and the shared side. `--reveal` prints everything.

`local set` stores values under `env`/`headers` and known text fields (`url`, `command`, `source`, `path`, `ref`, `description`, `name`, `transport`, `default`, `*_version`) as strings; `--string` forces a string and `--stdin` reads the value from standard input so a secret stays out of shell history.

The overlay is written owner-only (`0600`) through a temp file and rename, and `local` refuses to replace a symlinked `config.local.*`. On Windows the file mode is not enforced by the OS, so rely on your user profile directory permissions there; the advisory lock is also not taken.

The `--local` flag on `profile add|remove|set-default`, `include add|remove` and `skill install|remove` writes to the overlay instead of the shared config. Removing a shared include or installed skill writes `remove = true` for it; `profile remove --local` of a shared profile is an error ("Shared profiles cannot be removed locally"). The MCP tools `update_config`, `add_profile`, `remove_profile`, `set_default_profile`, `add_include`, `remove_include`, `install_skill` and `uninstall_skill` accept `local: true`.

### Local overrides and generate

When a `config.local.*` overlay or `local/` content exists, `generate` also renders the shared baseline (the config as a teammate without them sees it) and compares:

- **local-only** files exist only because of your local config (an extra preset, local content). They are git-ignored (`*.local.*` names through the managed `.gitignore` block, other names through `.git/info/exclude`) and listed in the gitignored `.ai-rulez/.generated-manifest.local.json`, never in the committed manifest. Generation stops, listing paths only, if such a file is tracked by git.
- **drift** files are shared outputs your local config would change. Generation stops, listing paths only, if such a file is tracked by git or not ignored; pass `--allow-local-drift` to write it anyway.
- **suppressed** files are shared outputs your local config no longer produces (for example a preset dropped with `"!claude"`). They are left alone, never deleted.

`.git/info/exclude` is per clone and shared by linked worktrees, so each project gets its own block, delimited by `# BEGIN ai-rulez local: <absolute config dir>` / `# END ...` and anchored at the repository root; `generate` only ever rewrites its own block, removes it once the project has no overlay-derived outputs, and never touches other lines or other projects' blocks. Paths git's exclude file cannot hold (and every path outside a repository) go to the managed `.gitignore` block instead, with a warning.

Whether a path is tracked or ignored is asked of git in one call each; if git fails inside a repository the guard assumes every affected file is tracked and refuses. If a `.gitignore` rule un-ignores a machine-local or secret-bearing output, the overlay or the `local/` tree, or `.gitignore` is a symbolic link (entries then go to `.git/info/exclude`), see [Local overrides](local-overrides.md); the run is refused with the paths listed when something would stay committable. `--allow-local-drift` is command-line only (MCP clients cannot pass it) and can write non-secret overlay values into files that are tracked and shared, so review `git diff` before committing. It does not bypass the secret guard: a generated MCP config that would carry resolved secrets (`env`, `headers`, URL credentials, secret flags) is still refused unless it is git-ignored.

Shared outputs keep the baseline hashes, so your headers match a teammate's; local-only outputs carry a hash of their local inputs. `generate --dry-run` prints `local-only:`, `drift:`, `allowed:` (a drift file that is git-ignored), `suppressed:` and `blocked:` lines, and exits non-zero when any line is `blocked:` (the real run would refuse). A run with `--no-local` (or `generate --plugin`, which never uses local config) does not delete your local files.

## Builtins Command

### `ai-rulez builtins list [flags]`

List all built-in domains embedded in the `ai-rulez` binary.

**Flags:**

- `--format text|json` (optional, default `text`): `json` prints JSON.

### `ai-rulez builtins show <name> [flags]`

Show the full rules, context, skills, agents, and commands for a built-in domain.

**Arguments:**

- `<name>` (required): Built-in domain name, such as `security`, `go`, or `typescript`

**Flags:**

- `--format text|json` (optional, default `text`): `json` prints JSON.

---

## Convert Command

### `ai-rulez convert`

Read another tool's configuration and produce an equivalent `.ai-rulez/` tree, with a report that lists every input construct as `mapped`, `approximated`, `dropped`, `needs-action` or `unsupported`. It never modifies the source files, never runs anything, uses the network only with `--fetch`, and writes nothing unless `--write` is given.

```bash
ai-rulez convert --dry-run                        # what would carry over, and what would be lost
ai-rulez convert --write                          # write .ai-rulez/
ai-rulez convert --from native,skills-lock --write
ai-rulez convert --from rulesync --dry-run        # import a rulesync project
ai-rulez convert --from apm --write --fetch --lock  # import an APM project with its remote packages, then pin
ai-rulez convert --from tessl --write             # vendored Tessl plugins and their eval scenarios
ai-rulez convert --from okf --write --domain kb   # an OKF bundle, into a domain
ai-rulez convert --write --merge                  # add beside an existing tree, keeping every file of it
ai-rulez convert --write --enable-hooks           # imported hooks live (they are commented out by default)
ai-rulez convert --dry-run --format json --report convert.json
ai-rulez convert --write --domain imported        # import beside an existing tree
ai-rulez convert --list                           # importers and what each detects here
```

**Flags:**

| Flag | Description |
| ---- | ----------- |
| `--from` | Importers, comma separated: `native`, `rulesync`, `apm`, `tessl`, `okf`, `skills-lock` or `auto` (default). `auto` runs every importer that detects something, `skills-lock` first, so the skills it tracks are not also copied. When a rulesync, APM or Tessl project is detected, `auto` leaves `native` out, because the tool files next to their inputs (`.rulesync/`, `.apm/`, `.tessl/`) are their generated output; use `--from native,rulesync` to read both. |
| `--source DIR` | Directory to read (default `.`). Reads are rooted there; symlinks are never followed, files over 2 MiB and inputs over 64 MiB are skipped with a finding. |
| `--into DIR` | Config directory (default `.ai-rulez`). A relative path is resolved against `--source`, an absolute path is used as is. Nothing is ever written through a symlink at or below it: a symlinked config directory, content directory or target file stops the run (exit 1) with nothing written. |
| `--domain NAME` | Put the imported rules, context, skills, agents and commands under `domains/NAME/`. |
| `--dry-run` / `--write` | Preview or write. With neither flag a terminal gets a dry run and a script is refused, so CI never converts by surprise. |
| `--force` | Overwrite existing content files (rules, context, skills, ...) whose content differs. Without it, an existing differing file stops the write (exit 1, nothing written). `--force` never replaces `config.toml` (see below). Files with identical content are `unchanged`, so a second run is a no-op. |
| `--merge` | Add beside an existing tree without touching any file of it: an item whose file exists with other content is imported as `NAME-imported` (then `NAME-imported-2`, ...; a skill's `name:` follows), an identical one is `unchanged`, so a second run adds nothing. Excludes `--force`. See [Merging](#merging-names-and-delivery). |
| `--keep-names` | Never rename to settle a collision: between imported items (normally a stable hash suffix) or with an existing file under `--merge` (normally `-imported`). The collision is an error and nothing is written. |
| `--delivery static\|served\|both` | Sets how the imported skills reach the agent: `[skills] delivery`, or `[domains.NAME] delivery` with `--domain`. An existing value wins and the difference is a `needs-action` finding. With no skill to apply to, a finding says so and nothing is written. |
| `--fetch` | Read the remote git sources the input names (rulesync `sources`, APM dependencies that are not installed) over the network. See [Fetching](#fetching-remote-sources). Without it nothing is fetched and each source is a `needs-action` finding. |
| `--lock` | After a successful `--write`, run `ai-rulez lock` on the converted config, which pins remote sources, authored content and outputs. Needs `--write`; convert never imports a foreign hash as a pin. The lock renders the outputs, so the `${VAR}` placeholders that replaced literal MCP credentials must be set first (otherwise `lock` fails after the files were written, exit 1: set them and run `ai-rulez lock`). Without it, a conversion that produced `[[installed_skills]]` ends with `Next: run ai-rulez lock`. |
| `--enable-hooks` | Write imported hooks as live `[[hooks]]`. By default they are a commented block of `config.toml`. See [Hooks and permissions](#hooks-and-permissions). |
| `--enable-permissions` | Write imported `allow` rules as live `[permissions]`. By default they are commented; `ask` and `deny` rules are always live. |
| `--allow-findings CODES` | Write despite scan findings of these codes (for example `AR001`). Discouraged; see "Blocked scan" below. |
| `--format text\|json` | Report format. JSON follows [`schema/convert-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/convert-report.schema.json) (`schema_version: 1`). |
| `--report FILE` | Also write the report to a file. |
| `--fail-on` | Exit 2 when a finding has one of these statuses (`approximated`, `dropped`, `needs-action`, `unsupported`). |
| `--best-effort` | Import the known fields of an unrecognised format version instead of stopping. |
| `--split-headings` | Split root files such as `CLAUDE.md` into one context per H2 heading (default: one context per file). |
| `--list` | Show each importer and the files it detects in `--source`. |

**Importers:**

| Importer | Reads | Writes |
| -------- | ----- | ------ |
| `native` | Root files (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `QWEN.md`, `.github/copilot-instructions.md`, `.junie/guidelines.md`, `.cursorrules`, ...), rule folders (`.cursor/rules`, `.github/instructions`, `.kiro/steering`, `.windsurf/rules`, `.roo/rules`, `.clinerules`, `.claude/rules`, `.devin/rules`, `.qwen/rules`, ...), skills (`.claude/skills`, `.agents/skills`, `.kiro/skills`, ...), agents, commands and prompts of every built-in preset, plus MCP files (`.mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json`, `.kiro/settings/mcp.json`, `.roo/mcp.json`, `.gemini/settings.json`, `.qwen/settings.json`), and the hooks and permissions of `.claude/settings.json`, `.codex/hooks.json`, `.gemini/settings.json`, `.cursor/hooks.json`, `.cursor/cli.json` and `.github/hooks/*.json`. Locations come from the preset layouts, so they follow `generate`. | `rules/`, `context/`, `skills/` (with their resource files), `agents/`, `commands/`, `[[mcp_servers]]`, `[[hooks]]`, `[permissions]` and `presets` in `config.toml` |
| `rulesync` | `rulesync.jsonc` and the input root `.rulesync/` (or each entry of `inputRoots`): `rules/`, `commands/`, `subagents/`, `skills/<name>/`, `checks/`, `mcp.jsonc` (or `mcp.json`), `hooks.jsonc`, `permissions.jsonc`, `.aiignore`, `.rulesyncignore`, `rulesync.lock`. See [rulesync](#rulesync). | `rules/`, `context/`, `skills/` (with resources), `agents/`, `commands/`, `checks/`, `[[mcp_servers]]` and `presets` in `config.toml` |
| `apm` | `apm.yml`, `.apm/` primitives, installed `apm_modules/`, local path dependencies, `apm.lock.yaml`. See [APM](#apm). | `rules/`, `context/`, `skills/`, `agents/`, `commands/`, `[[mcp_servers]]`, `[[hooks]]`, `presets` |
| `tessl` | `tessl.json` and the vendored `.tessl/plugins/<workspace>/<plugin>/`. See [Tessl](#tessl). | `skills/` (with `evals/*.eval.yaml`), `rules/` |
| `okf` | An OKF bundle: an `index.md` naming `okf_version` at the source root or in `docs/okf`. See [OKF](#okf). | whatever `import okf` writes |
| `skills-lock` | `skills-lock.json` (lock file version 1 of the Vercel [skills CLI](https://github.com/vercel-labs/skills)) | `[[installed_skills]]` (`source` as a git URL, `ref`, `path` from `skillPath`) |

`config.toml` is merged, never replaced, with or without `--force`: new `presets`, `[[mcp_servers]]` and `[[installed_skills]]` are appended (existing entries win; a differing imported entry of the same name is a `needs-action` finding), and the file is rewritten with the config writer, which does not keep comments. When nothing is new it is `unchanged` and untouched. A V3 config (`config.yaml`, `config.yml`, `config.json`), which ai-rulez no longer reads, is left alone and reported as `manual`; a `config.toml` that cannot be parsed stops the run. When nothing identifies a preset and the config already has presets, none is added.

Rule frontmatter is translated, not copied: Cursor `alwaysApply`/`globs`/`description`, Copilot `applyTo`, Kiro `inclusion`/`fileMatchPattern`, Devin and Windsurf `trigger`, and `paths` become `activation`, `globs` and `description`; any other key is reported as `dropped`. A Cursor rule with `alwaysApply: true` and `globs` is always on, so its globs are dropped (`approximated`). Frontmatter that is not valid YAML as written (an unquoted `globs: **/*.ts`) is read leniently with block scalars and lists intact, and reported as `approximated`. Skills, agents and commands are copied with their frontmatter, and every key ai-rulez has no field for (Claude `color`, `permissionMode`; Copilot `handoffs`, `target`, `mode`, `agent`; ...) is reported as `approximated`, because only presets that copy unknown keys render it; a comma-separated Claude `tools` string becomes a list. The `name:` of a `SKILL.md` follows its directory when the directory is renamed to a valid name or suffixed after a collision. A name with no ASCII letters or digits (`日本語`) gets a stable derived name (`unnamed-<hash>`), reported as `approximated`. Presets are inferred only from files one tool owns (`.cursor/rules` implies `cursor`, `.windsurf` implies `devin`, `.roo` implies `zoocode`); shared files such as `AGENTS.md` and `.agents/skills` imply none, and with no other evidence `claude` is used and reported. A command file `convert` imported (`.claude/commands/daily.md`) is removed by the first `generate` once the generated `.claude/skills/daily/SKILL.md` replaces it and the file and its copy under `.ai-rulez/commands/` are both unchanged since the import, so the command does not appear twice; `clean` writes it back from that copy as it removes the skill. An edited file stays, and a skill whose command cannot be written back byte for byte is kept by `clean`. A root file that is a symlink onto an imported `AGENTS.md` (`CLAUDE.md -> AGENTS.md`) is not imported, but `codex` (which writes `AGENTS.md`) and the preset the link name implies (`claude`) are added, so the first `generate` writes `AGENTS.md` and keeps the link. Files ai-rulez generated are never read back as source: the `AI-RULEZ ::` header, the `GENERATED FILE` and `Generated by ai-rulez` banners (also in skill resource files), the `Content-Hash` and `Source-Hash` lines, and every path in `.ai-rulez/.generated-manifest.json` are skipped with a `dropped` finding, as are root files that only contain an `@path` pointer. A path that exists but cannot be read (permissions, a symlink, an invalid `.claude/settings.json`) is reported as `dropped` with the error, never silently ignored. Identical content found in several files (for example `CLAUDE.md` and `AGENTS.md`) is imported once; different content under one name gets a stable hash suffix.

A root file such as `CLAUDE.md` or `AGENTS.md` becomes a context file. Frontmatter keys no tool reads (`title`, `applies_to`, `updated`) are kept under `metadata:` so `validate` does not report them as `AR303`, and the report lists them as `approximated`.

When `native` and `skills-lock` run together, skills named in `skills-lock.json` are imported only as `[[installed_skills]]`, not copied from `.agents/skills`. The lock's `computedHash` is never carried (its scheme differs from the `ai-rulez.lock` tree digest) and is reported as `needs-action`; run `ai-rulez lock` after converting. `node_modules` and `local` skill sources are `unsupported`; global skill locks are out of scope. A lock source must be an `https://`, `ssh://` or `git@host:path` URL: a leading `-`, a transport helper such as `ext::`, `file://`, `git://`, plain `http://` and URLs with embedded credentials are `unsupported`, as are a `ref` starting with `-` and a `skillPath` that is absolute or contains `..`. The planned `installed_skills` go through the same field validation as at config load, without any network access.

MCP servers keep `command`, `args`, `env`, `url`, `headers` and transport. Every string is checked with the security scan's secret detectors: a literal under a credential-looking name (`*_KEY`, `*_TOKEN`, `Authorization`, ...), a recognised token or key in any value whatever its name, a connection string with a password, `--api-key x` and `--token=x` style arguments (and `KEY=value` arguments), URL userinfo and credential query parameters are replaced by a `${VAR}` reference and reported as `needs-action`; the value is never printed or written. Only an exact `$VAR` or `${VAR}` (upper case) counts as an existing reference. The scan of the planned tree also reads `config.toml`, so a secret that gets through (for example in a `description`) blocks the write.

#### Hooks and permissions

Hooks and permission rules of the tool files above (and of rulesync's `hooks.jsonc` and `permissions.jsonc`) become `[[hooks]]` and `[permissions]`. **Imported hooks are never enabled by the importer**: a hook runs a command on your machine, and an allow rule widens what every harness may do (the rule was written for one tool, `[permissions]` renders into all of them).

| What | Default | With |
| ---- | ------- | ---- |
| `[[hooks]]` | written as a commented block at the end of `config.toml` (`# [[hooks]]`, ...), plus a `needs-action` finding | `--enable-hooks` writes them as live TOML |
| `allow` rules | commented, `needs-action` | `--enable-permissions` |
| `ask` and `deny` rules | live: they only narrow what a harness may do | |

A comment is inert to every ai-rulez command, so nothing runs until you remove the leading `# `. The scan and validation cover the commented text too: a hook with a secret or a `curl | sh` blocks the write, and an invalid hook is a validation error, enabled or not. A second run does not repeat a block `config.toml` already holds; into an existing config, live hooks and rules are added when not already declared.

Each hook group keeps the harness it came from in `targets`, so a hook that ran in one tool does not start running in the others; a matcher written in a tool's own vocabulary (Gemini `run_shell_command`, Cursor `Shell`) is kept as `matchers.<harness>`. The same hook found in several tools' files is declared once for all of them. Event names are translated to Claude Code's (Gemini `BeforeTool` is `PreToolUse`, Cursor `beforeSubmitPrompt` is `UserPromptSubmit`, rulesync `sessionStart` is `SessionStart`); millisecond timeouts become seconds (rounded up); `command`, `args`, `timeout`, `async`, `if` and `statusMessage` are carried. Anything else is reported, never guessed:

- an event with no ai-rulez hook event (Cursor `afterFileEdit`, rulesync `beforeShellExecution`) is `unsupported`, listed;
- a handler that is not a command (`prompt`, `http`, `agent`, `mcp_tool`) is `unsupported`; handler keys with no equivalent (`shell`, `env`, `failClosed`, ...) are `dropped`, listed;
- the guard hook ai-rulez writes itself (`ai-rulez guard`) and `.github/hooks/ai-rulez.json` are not read back as source;
- a command that runs a script of the rulesync tree (`.rulesync/hooks/...`) or of a package (`${CLAUDE_PLUGIN_ROOT}`) is `needs-action`: copy the script into the project and use a `script` path;
- hooks declared inline in `.codex/config.toml` are `needs-action` (move them to `.codex/hooks.json`);
- rulesync per-tool blocks for tools without a hook harness are `unsupported`.

Permission rules are Claude Code rules: `.claude/settings.json` `permissions.allow|ask|deny` verbatim (`defaultMode`, `additionalDirectories` and other keys are `dropped`); rulesync `permission.<category>.<pattern> = action` becomes `Tool(pattern)` (`bash` is `Bash`, `write` and `notebookedit` path rules `Edit`, a bare `*` pattern the bare tool; a category `*` is `unsupported`; tool-scoped blocks such as `claudecode.permission` are `dropped`); Cursor `.cursor/cli.json` rules are rewritten (`Shell` to `Bash`, `Shell(cmd:args*)` to `Bash(cmd args:*)` and `Shell(cmd:args)` to `Bash(cmd args)`, `Write` to `Edit`, `Mcp(server:tool)` to `mcp__server__tool`, `approximated`); a deny whose arguments hold an inner glob is widened to its literal prefix, an allow of that kind is `unsupported`, and the rules a previous `generate` wrote into `cli.json` (named by the manifest) are `dropped` as generated. Gemini's `tools.allowed`/`exclude` and Codex's `.codex/rules` are `unsupported` rather than inverted. Other keys of `.claude/settings.json` (`model`, `env`, `statusLine`, ...) are `dropped`, one finding each.

#### Merging, names and delivery

`--merge` is for adding to a project that already has content: every existing file is left byte for byte, and an item whose path is taken by other content is written as `NAME-imported` (a skill's `name:` follows; the report's finding names both). The suffix is reused when the file it chose last time is still there, so a second run changes nothing. `config.toml` is merged as always. With `--keep-names` nothing is renamed: under `--merge` the collision stays a conflict (exit 1), and between imported items it is an error naming both sources. `--force` and `--merge` exclude each other.

`--delivery` writes the default for the imported skills. `[skills] delivery` is the default of every skill, including ones already in the project, so prefer `--domain NAME` (writes `[domains.NAME] delivery`) next to an existing tree.

#### Fetching remote sources

`--fetch` reads the git sources an input names but does not hold: rulesync `sources` (`github` and `git` transports; the `npm` transport is `unsupported`) and APM dependencies that are not installed. It uses the skill-source fetcher (`[[skill_sources]]`: the same clone size and file limits, cache and credential rules), only for `https://` URLs; `http://`, `git://`, `file://`, `ssh`, URLs with credentials, and paths that leave the repository are `unsupported` before any request is made. A lock file of the input (`rulesync.lock`, `apm.lock.yaml`) decides which commit is read.

| Input | Result |
| ----- | ------ |
| rulesync source with `skills` | `[[installed_skills]]`, one per selected skill (`"*"` or no selection: every skill found), pinned: a tag or commit the input named is kept as `ref`, a branch or the default branch is replaced by the commit that was read. A selected skill that does not exist is `needs-action`; when two sources provide a name the first wins (`approximated`) |
| rulesync source with `rules` | the selected `.md` files of `rulesPath` (default `rules`) copied into `rules/`, like rulesync's own `.curated` rules (no link kept) |
| APM dependency that is a single skill (`owner/repo/skills/name`) | one `[[installed_skills]]` entry |
| other APM dependency | the package's `.apm/` primitives (or `SKILL.md`, `skills/`, `agents/`, `commands/`) copied into the tree; its own dependencies are not followed (`needs-action`) |

Everything fetched is planned like any other input: copied text is scanned in the planned tree, and the text of the skills that are referenced rather than copied is scanned as well, under the name `host/owner/repo@<commit>:path`. A blocking finding exits 2 and writes nothing. A failed fetch, or a resolved commit that is not a full hash, stops the run with nothing written. `--dry-run --fetch` fetches (the cache is filled) and writes nothing. Pass `--lock` to pin the installed skills in `ai-rulez.lock` right after the write.

#### APM

`--from apm` reads a [Microsoft APM](https://github.com/microsoft/apm) project. The layout is read from the public documentation and from rulesync's APM-compatible reader, not verified against a release of `apm`.

| APM | ai-rulez | Notes |
| --- | -------- | ----- |
| `.apm/instructions/*.instructions.md` | `rules/` | `applyTo` becomes `globs` (`**` is always on), `description` carries over |
| `.apm/agents/*.agent.md`, `.apm/chatmodes/*.chatmode.md` | `agents/` | other keys are kept and reported like any agent |
| `.apm/prompts/*.prompt.md` | `commands/` | |
| `.apm/skills/<name>/` | `skills/<name>/` | resources copied |
| `.apm/context/*.context.md` | `context/` | |
| `.apm/hooks/*.json` | `[[hooks]]` | disabled by default, see above; scripts are not copied |
| `apm.yml` `target` | `presets` | `vscode` and `copilot` are `copilot`, `windsurf` is `devin`; `all` is `needs-action` |
| `apm.yml` `dependencies.apm` | the package content | installed in `apm_modules/<owner>/<repo>` or a local path inside the project: imported as local files, the dependency link is not kept (`approximated`). Not installed: a remote source, `needs-action` without `--fetch`. SSH, marketplace, absolute and escaping paths are `unsupported` |
| `apm.yml` `dependencies.mcp` | `[[mcp_servers]]` | self-defined servers (name, transport, command, args, env, url, headers) with the usual secret replacement; a bare registry name is `unsupported`: convert never queries a registry |
| `apm_modules/` packages `apm.yml` does not name | the package content | transitive dependencies |
| `apm.lock.yaml` | none | the resolved commits decide what `--fetch` reads; `content_hash` is not carried (`needs-action`, run `ai-rulez lock`) |
| `apm.yml` name, version, description, author, `scripts`, `devDependencies`, `apm-policy.yml` | none | `dropped` / `unsupported` with the reason; scripts run commands of the apm runtime |

#### Tessl

`--from tessl` reads `tessl.json` and the vendored plugins under `.tessl/plugins/<workspace>/<plugin>/` (a version directory below it is found by the version `tessl.json` names, or when it is the only one). The layout follows the Tessl documentation and is not verified against a release. Nothing is fetched from the registry: a dependency that is not on disk (`mode` `managed`, or not vendored) is `needs-action`; run `tessl install` and convert again, or point `--source` at a populated directory. A project that is itself a plugin (`.tessl-plugin/` at its root) is read the same way.

| Tessl | ai-rulez | Notes |
| ----- | -------- | ----- |
| `skills/<name>/` | `skills/<name>/` | the provenance (`workspace/plugin@version`) is in the report; the registry link is not kept |
| `rules/*.md` | `rules/` | |
| `evals/<scenario>/task.md` + `criteria.json` | `skills/<skill>/evals/<scenario>.eval.yaml` | one [eval case](evals.md#case-format) per scenario: `prompt` is the task, `expect_trigger: true`, the criteria's `context` and checklist become the `rubric` (weights dropped, `approximated`). No `criteria.json` is a trigger-only case. A plugin's evals go to its only skill, else the skill named like the plugin, else the first one (reported) |
| `docs/` | none | `dropped`: documentation is reference material Tessl serves; copy what the agent needs into `context/` |
| `.tessl-plugin/plugin.json` | none | `dropped`: it describes the dependency, not this project (the provenance finding records it) |
| `verify` | none | `unsupported` |
| `.tessl/RULES.md` and the managed `AGENTS.md` block | none | generated by Tessl, skipped; `auto` leaves `native` out next to Tessl |

#### OKF

`--from okf` is `import okf` through convert: the same bundle mapping (see [OKF](okf.md)), with the lossiness report, the scan before write and `--domain`. A bundle is found at the source root or in `docs/okf`. Findings of the bundle (lossy links, skipped files) become report findings. A skill's script keeps its executable bit, here and in the native, rulesync and APM importers.

#### rulesync

`--from rulesync` reads a [rulesync](https://github.com/dyoshikawa/rulesync) project. The layout follows rulesync's own source (`docs/reference/file-formats.md`, `src/constants/rulesync-paths.ts`); `rulesync.jsonc` is parsed as JSONC (comments, trailing commas). A `rulesync.jsonc` that cannot be parsed stops the run with `AR9F0`.

| rulesync | ai-rulez | Notes |
| -------- | -------- | ----- |
| `rules/*.md` | `rules/` | `description` and `globs` carry over; `**/*` means always on. `targets` map 1:1 to preset names (`claudecode` is `claude`, `codexcli` is `codex`, `agentsmd` is the root file `AGENTS.md`); `*` means everywhere. A name with no preset is `approximated`; if none is left the original names are kept in `targets`, so the rule reaches no output until reviewed (`needs-action`). |
| `rules/*.md` with `root: true` | `context/` | Root rules go into every tool's root file, which is what context does (`approximated`). |
| `rules/*.md` with `localRoot: true` | `local/context/` | Rulesync's personal root file (`CLAUDE.local.md`) goes to the machine-local tree `.ai-rulez/local/` (see [Local Configuration](local-overrides.md)), never to a committed output (`approximated`). The files are written owner-only (`0600`, directories `0700`), `--domain` puts them in `local/domains/NAME/`, and the project `.gitignore` gets a `.ai-rulez/local/` entry so the text cannot be committed before `generate` takes over that job. The scan covers them like any planned file. |
| `cursor`, `devin`, `antigravity`, `kiro`, `claudecode.paths` sections of a rule | `activation`, `globs`, `description` | Only used when the rule's own `globs` say nothing: Cursor `alwaysApply: true`, Devin and Antigravity `trigger`, Kiro `inclusion` and `fileMatchPattern`, Claude `paths`, Cursor `globs` and `description`. Reported `approximated`, because the result applies to every tool. Every other key of every tool section (Copilot `name` and `excludeAgent`, Trae `scene`, ...) is `dropped`, one finding per section naming its keys. |
| `commands/**/*.md` | `commands/` | `description` and `targets`; nested directories become part of the name (`git/commit.md` is `git-commit`, `approximated`). Tool sections are `dropped`. |
| `subagents/**/*.md` | `agents/` | `name`, `description`, `targets`; `claudecode.model` (not `inherit`), `tools`, `skills` and `effort` are lifted to the top level (`approximated`, they apply to every preset). The rest of the `claudecode` section (`permissionMode`, `maxTurns`, `color`, ...) and every other tool section are `dropped`. |
| `skills/<name>/` | `skills/<name>/` | `SKILL.md` frontmatter (`description`, `license`, `compatibility`, `metadata`, `targets`) and every resource file are copied; `claudecode.allowed-tools` is lifted to `allowed-tools`; `disable-model-invocation` and `user-invocable` are kept as written (`approximated`). The `name` follows the directory. |
| `checks/*.md` | `checks/` | `description`, `severity` (low, medium, high, critical), `tools`, `targets`. |
| `mcp.jsonc` / `mcp.json` | `[[mcp_servers]]` | Same field handling and secret replacement as the native importer; `local` is `stdio`, `streamable-http` is `http`. Per-server `targets` (`approximated`), `enabledTools` and `disabledTools` (`dropped`), tool-scoped `{tool}.mcpServers` blocks (`dropped`, listed) are not carried. `mcp.jsonc` wins over `mcp.json`. |
| `rulesync.jsonc` `targets` | `presets` | Object and array forms; per-tool feature selection is `approximated`; `*` is `needs-action`; tools without a preset (`agentsskills`, plugin targets, `continue`, `tabnine`) are `unsupported`. |
| `rulesync.jsonc` `sources`, `rulesync.lock` | `[[installed_skills]]`, `rules/` with `--fetch` | Without `--fetch` a source is a `needs-action` finding and nothing is read. With it, selected skills become pinned `[[installed_skills]]` and selected rules are copied, see [Fetching](#fetching-remote-sources); `rulesync.lock`'s `resolvedRef` decides the commit, its integrity hashes are not carried. Content already installed under `rules/.curated/` and `skills/.curated/` is imported as local files. |
| `hooks.jsonc`, `permissions.jsonc` | `[[hooks]]`, `[permissions]` | The shared `hooks` block applies to every harness; `claudecode`, `codexcli`, `cursor`, `copilot` and `copilotcli` blocks are restricted to that harness. Disabled by default, see [Hooks and permissions](#hooks-and-permissions). |
| `.aiignore`, `.rulesyncignore` | none | `dropped`: ignore is deprecated upstream and ai-rulez has no ignore feature; deny reads with `[permissions]`. |
| `rulesync.local.jsonc` | none | `needs-action`: personal overrides belong in `.ai-rulez/config.local.toml`. |
| other `rulesync.jsonc` keys (`outputRoots`, `delete`, `language`, `simulate*`, ...) | none | `dropped` with the reason; generation settings of rulesync have no ai-rulez counterpart. |

A later entry of `inputRoots` overrides a same-named item of an earlier one (`approximated`); roots that are absolute or leave the source directory are `unsupported`. Anything under `.rulesync/` that is not a rulesync input is `dropped`.

**Design decisions.** Root rules become context, not `priority: critical` rules, because context is what ai-rulez writes into every root file. `localRoot` rules go to the machine-local tree, since they are personal and the rest of the output is committed. A tool section is never merged into the item, so a per-tool override cannot silently widen to every tool; the few lifted keys are reported. `native` is skipped by `auto` next to rulesync so generated files are not imported twice. Hooks and allow rules are written disabled (open question 5 of the design, decided as proposed: never enabled automatically).

**First generate after convert.** `convert --write` records the native files it imported (and root files that only hold an `@path` pointer) with their SHA-256 in `.ai-rulez/.converted.json`. The first `generate` replaces those files without `--force` as long as they are unchanged since; a file `convert` did not import still stops `generate` (see [Existing files](#existing-files-generate-will-not-overwrite)). `clean` keeps the generated file that replaced such an original. Commit the record with `.ai-rulez/`.

**Blocked scan.** When the scan finds an error-level problem (a secret, a risky command), `convert` prints each finding at the **source** file and line it came from (`.rulesync/rules/x.md:33`), with the planned `.ai-rulez/` path in parentheses, exits 2 and writes nothing. Remove the text from the source and run again. To write anyway, pass `--allow-findings AR001` (repeatable, comma separated): the finding stays in the report, marked `allowed`, and only that code is let through. Findings in the generated `config.toml` keep their planned path. In `--format json` the source location is `file`/`line` and the planned one is `planned`.

**Safety:** the planned tree is loaded and security-scanned (the `AR0xx` family of `validate`) in a scratch directory before anything is written. A blocking finding or a validation error exits 2 with `AR9F5`/validation messages and writes nothing; the security findings are reported even when validation fails, and inline `ai-rulez-lint-ignore` comments in converted text are not honoured. Writes are atomic per file; if one fails, every file written so far is restored or removed, the directories the run created are removed, and anything that could not be undone is named in the error.

**Report codes:** `AR9F1` approximated, `AR9F2` dropped, `AR9F3` needs-action, `AR9F4` unsupported, `AR9F5` blocked by the scan; `AR9F0` marks an input file that cannot be parsed at all and appears only in the error that stops the run. They have their own range, apart from the `AR9E0`-`AR9E4` scanner codes, and appear in the convert report only; `validate` does not emit them.

**Exit codes:** 0 done (the report may contain losses) or nothing to convert, 1 could not run or would overwrite existing files, 2 blocked by the scan or validation, or `--fail-on` matched.

**Nothing to convert.** When no importer finds anything to import (no tool files, or only files `ai-rulez generate` wrote), `convert` prints `Nothing to convert: ...` with a hint, writes nothing and exits 0, so a script converting many repositories keeps going. With `--format json` the message goes to stderr and stdout stays empty.

#### `init --from`

`ai-rulez init --from` runs `convert --write` with its sources: importer names (`auto`, `native`, `rulesync`, ...) or the project paths it always took (`.claude`, `.cursor`, `CLAUDE.md`), which limit the native importer to those paths. The sources are checked in a scratch directory first, so a source that cannot be imported, or a blocked scan, leaves an existing `.ai-rulez/` alone. When you confirm replacing an existing configuration directory (or pass `--yes`), it is moved aside to `<dir>.replaced-<pid>` while the import writes and restored if the write fails; it is deleted only after the import succeeded. Compared with the engine it replaced: a root file such as `CLAUDE.md` is one context item (`convert --split-headings` splits it), and MCP files, hooks and permissions are imported, with the report printed. [`import okf`](#ai-rulez-import-okf) stays as the direct entry point to the OKF mapping; `convert --from okf` is the same mapping with convert's report.

## Initialization Command

### `ai-rulez init [project-name]`

Initialize a new directory-based configuration. It writes `.ai-rulez/config.toml`.

**Syntax:**

```bash
ai-rulez init [project-name] [flags]
```

**Arguments:**

- `[project-name]` (optional): The project name. If omitted, the current directory's base name is used (falling back to `MyProject`). Nothing is prompted.

**Flags:**

| Flag                    | Type    | Default | Description                                                          |
| ----------------------- | ------- | ------- | -------------------------------------------------------------------- |
| `--domains` / `-d`      | string  | (none)  | Comma-separated list of domain names to create                       |
| `--skip-content` / `-s` | boolean | false   | Skip creating example content files                                  |
| `--from` / `-F`         | string  | (none)  | Import from existing tool files, such as `auto` or `.claude,.cursor` |
| `--setup-hooks` / `-H`  | boolean | false   | Configure Git hooks after initialization                             |
| `--yes` / `-y`          | boolean | false   | Automatically answer yes to prompts                                  |
| `--config-dir`          | string  | `.ai-rulez` | Directory to scaffold; use `.config/ai-rulez` for the `.config/` convention |

`--setup-hooks` detects an existing lefthook, pre-commit, or husky setup and adds ai-rulez to it in
place; if none of the three is present it logs a message and does nothing rather than failing. The two YAML configurations (`lefthook.yml`,
`.pre-commit-config.yaml`) are edited node by node, so existing comments, key order and indentation
width survive. Husky has no configuration file to preserve — the validation step is appended to
`.husky/pre-commit`. Re-running is a no-op once ai-rulez is already wired in.

ai-rulez is safe to run inside a git hook. Git exports `GIT_DIR`, `GIT_INDEX_FILE` and related variables to hooks, which would make every nested `git` call act on the hook's repository; ai-rulez removes `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_COMMON_DIR`, `GIT_PREFIX`, `GIT_OBJECT_DIRECTORY`, `GIT_ALTERNATE_OBJECT_DIRECTORIES` and `GIT_NAMESPACE` from the environment of every git subprocess it starts (and of `telemetry` sink commands) and targets repositories only with an explicit `-C`.
The generated config lists the built-in presets in a comment (`ai-rulez init --help` prints them too) and
enables `claude`. Add the harnesses you use to `presets`; all 52 and what each supports are in
[Supported harnesses](harnesses.md). The former `windsurf` preset is `devin`, and `continue-dev` is removed.

**General Flags:**

| Flag        | Type    | Description           |
| ----------- | ------- | --------------------- |
| `--verbose` | boolean | Enable verbose output |
| `--debug`   | boolean | Enable debug output   |

**Examples:**

Basic initialization:

```bash
ai-rulez init "my-project"
```

V4 with multiple domains:

```bash
ai-rulez init "my-project" --domains "backend,frontend,qa"
```

With example content skipped:

```bash
ai-rulez init "my-project" --skip-content
```

Scaffold the project-level `.config/` convention instead of `.ai-rulez/`:

```bash
ai-rulez init "my-project" --config-dir .config/ai-rulez
```

## Generate Command

### `ai-rulez generate [config-file]`

Generate AI assistant rule files from configuration.

**Syntax:**

```bash
ai-rulez generate [config-file] [flags]
```

**Arguments:**

- `[config-file]` (optional): Path to configuration file or directory. If not provided, auto-detected.

**Generation Flags:**

| Flag               | Type   | Default       | Description         |
| ------------------ | ------ | ------------- | ------------------- |
| `--profile` / `-p` | string | (from config) | Profile to generate; a comma-separated list composes several ([Composing Profiles](domains.md#composing-profiles)) |
| `--role`           | string |               | Generate the slice of content a [role](roles.md) selects, instead of a profile. Mutually exclusive with `--profile` and `--plugin`; works with `--user`, `--check` and `--dry-run` |

**General Flags:**

| Flag                            | Type    | Default       | Description                                                                                                                                             |
| ------------------------------- | ------- | ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--dry-run` / `-d`              | boolean | false         | Show what would be generated without writing files                                                                                                      |
| `--force`                       | boolean | false         | Overwrite an existing file ai-rulez cannot prove it wrote (see [Existing files](#existing-files-generate-will-not-overwrite)). A symlinked output stays refused |
| `--gitignore` / `-i`            | boolean | (from config) | Update `.gitignore` with generated output patterns git does not already ignore (a rule or `!` override of yours wins)                                                                                                      |
| `--recursive` / `-r`            | boolean | false         | Find and process configs recursively; exits non-zero if any root fails (the others are still processed)                                                 |
| `--offline` / `-f`             | boolean | false         | Skip fetching remote includes and use cached content                                                                                                    |
| `--verify-tags`                 | boolean | false         | Before generating, ask the remotes whether a tag pinned in `ai-rulez.lock` moved (`AR732`, exit 2) or was deleted (`AR735`); needs the network. Also `[lock] verify_tags = true` |
| `--emit-plan FILE`              | string  |               | Write the generation plan as JSON to FILE (`-` for stdout) and apply nothing ([details](#embedding-the-plan)) |
| `--no-local`                    | boolean | false         | Ignore the machine-local `config.local.*` overlay and `local/` content: generate the view a teammate without them sees. Also on `validate` and `tokens` (`verify` always checks the shared view) |
| `--check`                       | boolean | false         | Write nothing; compare the sources with the files on disk, list the differing ones (`missing:`, `stale:`, `edited:`, `orphan:`) and exit 2 on drift. Works with `--recursive`, `--profile`, `--no-local` ([details](#detecting-drift)) |
| `--locked`                      | boolean | false         | Require `ai-rulez.lock` to cover every remote include and installed skill, fetch exactly the pinned commits, and fail (exit 2) when an authored source differs from the lock's content pins (CI mode, see [Lock Command](#lock-command)). Never writes the lock |
| `--watch` / `-w`                | boolean | false         | Generate, then watch the configuration directory and local include sources and regenerate on every change; stops on Ctrl-C ([details](#watch-mode)) |
| `--frozen`                      | boolean | false         | `--locked` and never use the network: resolve only from the local cache, verified against the lock |
| `--allow-local-drift`           | boolean | false         | Write output even when machine-local config would change files shared with the team (see [Local Configuration](#local-configuration))                  |
| `--config-dir` / `-n`           | string  | `.ai-rulez`   | Configuration directory name for non-default layouts                                                                                                    |
| `--env` / `-e`                  | string  |               | MCP env override in `KEY=VALUE` form; repeatable                                                                                                        |
| `--env-file` / `-E`             | string  | `.env`        | Dotenv file for MCP env placeholders; repeatable                                                                                                        |
| `--plugin`                      | boolean | false         | Generate distributable plugin bundles and a marketplace index from the `[plugin]` block instead of in-repo config (see [Authoring Plugins](plugins.md)) |
| `--if-configured`               | boolean | false         | With `--plugin`, skip successfully when plugin authoring is not configured                                                                              |
| `--user`                        | boolean | false         | Generate the user config (`~/.config/ai-rulez`, or `--config <dir>`) into the home directories each harness reads; lists every path first (see [User-level configuration](user-scope.md)) |
| `--yes` / `-y`                  | boolean | false         | With `--user`, write without the confirmation prompt (required in a non-interactive shell); always, do not warn about new hook and MCP commands (same as `AI_RULEZ_ACK_COMMANDS=1`) |
| `--strict`                      | boolean | false         | Fail on unknown or invalid configuration keys instead of warning (env `AI_RULEZ_STRICT=1`). `generate` checks `config.toml` and `config.local.*` against the schema either way; this is not `validate` (deep content checks) |

`--token` / `-T` is a global flag (see [Global Flags](#global-flags)); it is not generate-specific.
`--update-gitignore`, `--no-configure-cli-mcp` / `-M` and `--skip-cli-mcp` / `-S` were removed in v5 (use `--gitignore`; the others had no effect).

`--dry-run` lists each file as `write-file:` (it would be written), `unchanged:` (already current) or `edited:` (changed by hand; `generate` overwrites the edit).

Before writing, `generate` also prints a summary of hook commands, command-based MCP servers, `[permissions] allow` rules and other command-bearing settings that are new or changed since the previous run on this machine (all of them in a fresh clone), even with `--quiet`. It only warns; `--yes` or `AI_RULEZ_ACK_COMMANDS=1` silences it. The MCP `generate_outputs` tool returns the same summary as `new_commands`. With `--watch` the summary is printed on each regeneration (what is new since the previous run), so a hook or MCP command that arrives with a pulled change is shown by the run that writes it.

Exit codes: `0` written, `1` a configuration failed to load, validate or generate (every root of a `--recursive` run is still processed), `2` `--check` found drift or `--locked`/`--frozen` found an authored source that differs from the lock (also a recursive run where every failure is lock drift). See [Exit Codes](#exit-codes).

#### Detecting drift

Teams that commit generated files can gate CI on `generate --check`. It renders in memory, never writes or deletes, and prints one line per file that differs:

| Line | Meaning |
| --- | --- |
| `missing: <path>` | A generated file is not on disk |
| `stale: <path>` | The sources changed (or the rendering did); `generate` would rewrite it |
| `edited: <path>` | The body no longer matches the `Content-Hash` in its own header: a hand edit; `generate` overwrites it, so the edit is lost |
| `orphan: <path>` | Listed in the previous manifest, no longer rendered; `generate` would delete it |
| `blocked: <path>` | A machine-local input (overlay or `local/` tree) would change this file and `generate` refuses to write it because it is tracked or not git-ignored; the same refusal `generate` and `generate --dry-run` report. `--allow-local-drift` lifts it |

With a machine-local overlay or `local/` content, `--check` classifies the outputs like `generate` does, so files that `generate` keeps for the local inputs and that are in sync are not reported. `--check` does not print the summary of rules whose activation a tool cannot express; `generate` prints it.

Exit codes: `0` nothing differs, `1` the check could not run (configuration invalid, a nested root failed to load, or an environment variable an MCP server references is unset: `--check` renders the outputs and needs the same secrets as `generate`), `2` at least one file differs. A trailing-newline-only difference is not reported, because `generate` normalizes it. With `[header] hashes = "none"` there is no hash to compare, so every difference is `stale`. `--check` cannot be combined with `--dry-run`, `--plugin` (use `verify --plugin`) or `--gitignore`.

#### Watch mode

`generate --watch` (`-w`) generates once, then keeps running and regenerates whenever the sources change. It watches `.ai-rulez/` recursively (subdirectories created later included), the machine-local overlay files, the config file, and every `includes` / `local_override` source that is a local path. Remote includes are not polled.

- Changes are debounced for 300 ms, so a save storm or `git checkout` produces one run.
- Runs never overlap. A change that arrives during a run triggers exactly one more run afterwards.
- Generated output, the generated manifests and editor swap/backup files (`*.swp`, `*~`, `.#*`, `4913`, ...) never trigger a run.
- Every regeneration runs the same preflight as a one-shot `generate`, including the summary of new hook and MCP commands.
- A configuration that fails to load or validate is logged and watching continues, so you can fix it and the next save regenerates.
- If `.ai-rulez/` is deleted and re-created (for example by switching branches), it is watched again.
- `SIGINT` / `SIGTERM` stop it cleanly, after any run in progress finishes. The first one cancels the run in progress; a second Ctrl-C kills the process immediately.
- Files the previous run recorded as generated (the manifests list them) never trigger a run, even when an include source is a tree that also holds outputs. Editor and VCS names (`.git`, `node_modules`, swap files) are ignored below the watched root only, so a project that itself lives under a `node_modules` directory is still watched.
- Symlinked watch roots and symlinked subdirectories are followed (a link back to a parent directory is not).
- A local include removed from the configuration stops being watched after the next run.
- If the operating system refuses to watch a directory (a limit such as `fs.inotify.max_user_watches` on Linux, or the open-file limit on macOS and BSD), one warning per run says so, with the hint to raise the limit; changes in the directories that could not be added are not noticed.

`--watch` cannot be combined with `--dry-run`, `--check`, `--user`, `--plugin` or `--recursive`; it watches one configuration and writes on every change. Other flags (`--profile`, `--no-local`, `--env`, ...) apply to every run.

**Examples:**

Generate with default profile:

```bash
ai-rulez generate
```

Generate specific profile:

```bash
ai-rulez generate --profile backend
```

Dry-run to preview generation:

```bash
ai-rulez generate --dry-run --profile backend
```

Generate and maintain the managed .gitignore block (opt-in):

```bash
ai-rulez generate --profile full --gitignore
```

Generate MCP configs with secret env values:

```bash
ai-rulez generate --env GRAFANA_SERVICE_ACCOUNT_TOKEN="$GRAFANA_SERVICE_ACCOUNT_TOKEN"
```

Package the project as distributable plugin bundles:

```bash
ai-rulez generate --plugin
```

Generate recursively in monorepo:

```bash
ai-rulez generate --recursive
```

Every discovered root is processed and all errors are printed; the exit status is 1 if any root failed to
load, validate, or generate (also with `--dry-run`). With `--profile`, a root that does not define the named
profile fails with a `profile not found` error for that root rather than falling back to default content.

Generate from a non-default configuration directory:

```bash
ai-rulez generate --config-dir ai-policy
```

Generate with private repository authentication:

```bash
# Using environment variable (recommended)
export AI_RULEZ_GIT_TOKEN="ghp_your_github_token_here"
ai-rulez generate

# Using CLI flag
ai-rulez generate --token "ghp_your_github_token_here"
```

### Embedding the plan

`generate --emit-plan FILE` renders everything in memory and writes the result as JSON, without writing, deleting or git-ignoring anything. Go code can get the same value from `generator.PlanOutputs(ctx, cfg, generator.PlanOptions{Profile, Role})`; `config.WithHost` injects the environment, clock, process runner and logger of the load (the zero host is the real process).

```bash
ai-rulez generate --emit-plan plan.json --profile backend
```

The document follows [`schema/plan.schema.json`](schema.md):

- `files`: every output sorted by path, with `action` (`write`, `merge` into a document the consumer owns, `mkdir`), `mode`, and the `size` and `sha256` of the content `generate` writes, minus the header's `Generated:` stamp and hash lines (so with `[header] hashes = "none"` and no timestamp the digest is that of the file on disk).
- `removals`: files an earlier run recorded and this one no longer renders (`stale`), and documents ai-rulez takes its earlier entries out of (`unmerge`, `delete`).

The plan is deterministic and holds no secret: MCP placeholders such as `${TOKEN}` stay as written, and an output that may carry a secret (`sensitive`), or whose content holds a credential the security scan detects, has no digest. It is conservative about that flag, so a plan may call a document sensitive that a real run finds clean. It reads the project the way a run does (the previous manifest, merged documents) and honours `--profile`, `--role` and `--no-local`; it runs the read-only part of the generate preflight (the schema check, `--strict`, and the role selection warnings) and records nothing, so a later run still warns about new commands. `--emit-plan` is refused with `--watch`, `--check`, `--user`, `--recursive` and `--plugin`, which never reach the plan or render something else. The plan lists neither the `.gitignore` update nor the `.ai-rulez/.generated-manifest.json` record `generate` writes: it covers rendered outputs and removals only.

Design decisions:

- `generate` is not yet built on the plan. `PlanOutputs` and `generate` share the rendering code (`collectOutputs`, the stale and unmerge planning); the write path still interleaves side effects (gitignore, secret guards) with writes, and splitting it is a separate change guarded by `tests/golden`.
- The plan carries digests, not content, so it stays small and cannot leak. Callers that need bytes render through `generate`.
- `renderer` is the generator's render-schema version: a renderer change changes digests, and the plan says so.

### Profile Selection

When using profile-based configuration:

1. **No `--profile` flag**: Uses default profile from config
2. **`--profile backend`**: Generates content for the `backend` profile
3. **Invalid profile**: Shows error and lists available profiles

Example with profile hierarchy:

```toml
# .ai-rulez/config.toml
default = "full"

[profiles]
full = ["backend", "frontend", "qa"]
backend = ["backend"]
frontend = ["frontend"]
```

```bash
# Uses "full" profile (default)
ai-rulez generate

# Uses "backend" profile only
ai-rulez generate --profile backend

# Uses "frontend" profile only
ai-rulez generate --profile frontend
```

## Clean Command

### Existing files generate will not overwrite

`generate` replaces a file only when it can prove ai-rulez wrote it: the file carries a `Content-Hash` or a generated banner, its bytes already equal the rendering, the generated manifest lists it and it carries a banner or matches the digest this machine recorded for it, or `convert --write` imported it and it is unchanged since (`.ai-rulez/.converted.json` records the digest of each imported file). Any other existing file at an output path, such as a hand-written `CLAUDE.md` in a repository you just ran `init` in, stops the run before anything is written: exit `1`, naming the file with the way out. Import it with `ai-rulez convert --write`, move or delete it, or pass `--force` to overwrite it. `--dry-run` prints `blocked: CLAUDE.md (an existing file ai-rulez did not write)` and exits non-zero; `--check` lists the path as `blocked`. An output whose path crosses a symlink leading out of the project (a `.cursor` folder linked elsewhere) is refused the same way, before any file is written or removed. `init` warns when a hand-written `CLAUDE.md`, `AGENTS.md` or `GEMINI.md` already exists and points at `convert --write`; `--check` names the same remedy instead of telling you to run `generate`. A manifest-listed file whose rendering carries no generated header (JSON, the Codex agent TOML) and that this machine recorded no digest for (an older ai-rulez wrote it, or the checkout is a fresh clone) is replaced with a warning; once it differs from the recorded digest it counts as hand-edited and is refused like any other unprovable file. `clean` applies the same proof and, where `generate` only warns, keeps the file. Rule folders keep their own guard (a hand-written rule file is skipped with a warning, or the generated rule is renamed), merged settings documents are merged into, and an empty file is replaced. `generate --user` applies the same proof to the files under your home directory.

A symlinked output is never written through. `CLAUDE.md -> AGENTS.md` is accepted when `AGENTS.md` is itself generated in the same run: the link stays as you made it, `AGENTS.md` gets its own content, and the link is not recorded as generated (so `clean` leaves it). A link to anything else is refused, naming the link and its target; remove the link, or point it at a generated path.

### `ai-rulez clean [config-path]`

Remove the files and directories that `generate` produced — the inverse of `generate`. This deletes the generated assistant outputs (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `.claude/`, `.codex/`, generated skills, etc.), the generated manifest (`.ai-rulez/.generated-manifest.json`), the local manifest (`.ai-rulez/.generated-manifest.local.json`) and the local rule files recorded in it, this project's block in `.git/info/exclude`, and the ai-rulez managed block in `.gitignore`. The ignore entries for the `config.local.*` overlay and `.ai-rulez/local/` stay in `.gitignore`: `clean` removes generated outputs, not the machine-local sources beside them. The committed manifest is treated as untrusted (see [What ai-rulez may change](settings.md#what-ai-rulez-may-change)): `generate` and `clean` delete a manifest-listed file only when its path is one a preset writes and the file proves it is ai-rulez output (its `Content-Hash` matches its body, or its SHA-256 equals the `digests` entry this machine recorded in the gitignored local manifest for header-less files). Anything else is kept and reported in one `Stale files not removed` warning per reason that names the files; on a fresh clone a header-less stale file is therefore reported, not deleted. On an upgrade, the Claude MCP servers an older version wrote into a manifest-listed `.claude/settings.json` are taken out of it when they are exactly that rendering (they now live in `.mcp.json` only). A generated file whose body was edited by hand is kept with a warning unless `--force` is given (`generate` rewrites it instead). The generated file that replaced an original `convert` imported (listed in `.ai-rulez/.converted.json`) is kept as well, even with `--force`: the original text lives under `.ai-rulez/`, and removing it would leave neither; delete it by hand if you no longer want it. A symlink is never removed, whoever made it. A merged settings document (`.claude/settings.json`, `.mcp.json`, ...) is deleted only when this machine recorded writing it whole and its digest still matches; a document that existed before ai-rulez first wrote to it is never deleted. Neither `generate` nor `clean` removes a path whose parent directory resolves through a symlink outside the project. Manifests written by older versions have no digests, so their header-less leftovers stay until deleted by hand. In a project with a `[plugin]` block or a plugin marketplace, `clean` also removes the bundle `generate --plugin` wrote: each file whose bytes still equal what `generate --plugin` renders, and the folders that leaves empty; an edited bundle file is kept with a warning. `clean` has no `--recursive`: run it in each root. It never needs MCP secrets: unset `${VAR}` placeholders do not stop it. A git-tracked `.ai-rulez/.generated-manifest.local.json` is ignored by `generate` and kept by `clean`, with a warning to `git rm --cached` it.

The `.ai-rulez/` source tree is never touched. Generated directories are only removed once they are empty, so any files you authored inside a generated directory are preserved.

Settings documents you share with ai-rulez (`.claude/settings.json`, `.gemini/settings.json`, `opencode.json`, `.mcp.json`, ...) are not deleted: `clean` removes only the keys, MCP server entries and array elements ai-rulez merged into them (including a stale entry that holds a resolved secret), and deletes the file only when nothing else is left. The plan lists them as `remove ai-rulez keys from:`. See [Settings documents shared with you](local-overrides.md#settings-documents-shared-with-you).

By default `clean` lists what it will remove and asks for confirmation. In non-interactive shells the prompt declines automatically — pass `--yes` there.

**Syntax:**

```bash
ai-rulez clean [config-path] [flags]
```

**Flags:**

| Flag                  | Type    | Default            | Description                                             |
| --------------------- | ------- | ------------------ | ------------------------------------------------------- |
| `--dry-run` / `-d`    | boolean | false              | Show what would be removed without deleting anything    |
| `--force` / `-y`      | boolean | false              | Skip the confirmation prompt and also remove generated files edited by hand (otherwise kept with a warning) |
| `--profile` / `-p`    | string  | configured default | Profile whose outputs to remove                         |
| `--config-dir` / `-n` | string  | `.ai-rulez`        | Configuration directory name for non-default layouts    |
| `--keep-gitignore`    | boolean | false              | Leave the ai-rulez managed block in `.gitignore`        |
| `--keep-manifest`     | boolean | false              | Leave the generated manifest in place                   |
| `--user`              | boolean | false              | Remove what `generate --user` wrote into the home directory, as recorded in `~/.config/ai-rulez/.generated-manifest.json` (see [User-level configuration](user-scope.md)) |

Preview what would be removed:

```bash
ai-rulez clean --dry-run
```

Remove all generated files without a prompt:

```bash
ai-rulez clean --yes
```

Remove generated files but keep the `.gitignore` block and manifest:

```bash
ai-rulez clean --yes --keep-gitignore --keep-manifest
```

## Verify Command

### `ai-rulez verify [config-path]`

Verify generated files without modifying them.

Without `--plugin`, every file listed in `.ai-rulez/.generated-manifest.json` must exist and still match the `Content-Hash` in its own header. This is fast and offline, and catches hand edits and deleted files; it does not re-render, so a source that changed since the last `generate` is caught by [`generate --check`](#detecting-drift) instead. Output and exit codes are the same (`0` verified, `1` cannot run, e.g. no manifest, `2` files differ). With `--plugin`, generated plugin bundles are checked against their provenance hashes.

**Syntax:**

```bash
ai-rulez verify [config-path] [--plugin] [flags]
```

**Flags:**

| Flag                  | Type    | Default            | Description                                                |
| --------------------- | ------- | ------------------ | ---------------------------------------------------------- |
| `--plugin`            | boolean | false              | Verify generated plugin bundles and marketplace files      |
| `--recursive` / `-r`  | boolean | false              | Find and verify plugin producers recursively               |
| `--if-configured`     | boolean | false              | Succeed without work when no plugin producer is configured |
| `--if-generated`      | boolean | false              | With `--plugin`, succeed without work when the bundle has not been generated yet |
| `--profile` / `-p`    | string  | configured default | Profile used when the plugin was generated                 |
| `--config-dir` / `-n` | string  | `.ai-rulez`        | Configuration directory name for non-default layouts       |
| `--attestation`       | boolean | false              | Verify the signed lock offline against the `[signing]` policy (see [Signing](signing.md)) |
| `--attestation-file`  | string  | next to the lock   | With `--attestation`: the bundle to verify                 |
| `--lock`              | boolean | false              | With `--attestation`: verify the lock attestation (the default and only lock subject) |
| `--approvals`         | boolean | false              | Re-check the signed and review-linked approvals that apply to the current content (see below) |
| `--online`            | boolean | false              | With `--approvals`: also check review-linked approvals against the forge (network and a token) |
| `--self`              | boolean | false              | Verify this ai-rulez binary against its release's Sigstore bundle (see [Signing](signing.md#verifying-ai-rulez-itself)) |
| `--bundle`, `--skill` | string  | none               | Verify the attestation of this plugin bundle or published skill directory (implies `--attestation`) |
| `--sbom`              | string  | none               | Verify the attestation of this SBOM file (implies `--attestation`) |
| `--source`            | string  | none               | With `--skill`: apply `[[signing.trust]]` entries scoped to this skill source or installed skill |
| `--require-provenance` | boolean | false             | With `--bundle`: require verified SLSA provenance next to the attestation |
| `--trusted-root`      | string  | cache of `trust update` | With `--attestation`: Sigstore trusted root file for keyless bundles |
| `--public-key`        | string  | none               | With `--attestation`: also trust this PEM public key (repeatable) |
| `--identity` / `--issuer` | string | none            | With `--attestation`: also trust this certificate identity and its OIDC issuer |
| `--no-state`          | boolean | false              | With `--attestation`: skip the per-user rollback state     |
| `--format`            | string  | `text`             | With `--attestation` or `--approvals`: `text` or `json` (`schema/verify-attestation.schema.json`, `schema/verify-approvals.schema.json`) |

Verify the signed lock (exit `0` verified, `1` cannot run, `2` verification failed with an `AR720` to `AR727` code):

```bash
ai-rulez verify --attestation
ai-rulez verify --attestation --public-key release.pub --format json
```

Re-check the approvals recorded in the lock above the `asserted` level (`--approvals`): every signed approval against its
attestation and the `[[signing.trust]]` entries for approvals, offline, and with `--online` every review-linked approval
against the forge (the review still exists, still approves, and was made on content whose lock pin is the approved
digest). Exit `0` every checked approval holds, `1` the check could not run (including a forge that cannot be reached),
`2` an approval no longer holds (`AR718`). `--format json` follows `schema/verify-approvals.schema.json`. See
[Approvals](approvals.md#verifying-approvals).

```bash
ai-rulez verify --approvals
ai-rulez verify --approvals --online --format json
```

Verify one producer:

```bash
ai-rulez verify --plugin
```

Verify every plugin producer and marketplace in a repository:

```bash
ai-rulez verify --recursive --plugin --if-configured
```

Exit codes: `0` the bundle matches its sources, `1` the check could not run (invalid configuration, no plugin configuration), `2` a bundle file is missing, stale, obsolete or fails its provenance hash.

When a `[plugin]` block exists but no bundle was generated, `verify --plugin` fails with `plugin bundle not generated; run `ai-rulez generate --plugin``. `--if-configured` only skips a project with no plugin configuration; add `--if-generated` to also skip until the bundle exists.

Recursive verification treats a marketplace root and its members as one atomic
producer. Consumer-only plugin installation declarations are skipped. Missing
authoring configuration is ignored only with `--if-configured`; stale, missing,
or invalid generated outputs still fail verification.

## Guard Command

### `ai-rulez guard`

Hidden command that harnesses run, not people. `generate` adds it as a `PreToolUse` hook when `[guard] generated = true` ([Settings](settings.md#guard)). It reads the hook payload (JSON) on stdin and checks the file the tool call edits against `.ai-rulez/.generated-manifest.json` and `.generated-manifest.local.json`.

Exit codes: `2` the call edits a generated file (the reason, with the source to edit, is on stderr); `0` everything else, including files merged into a settings document, paths outside the project, read-only tools and a payload that cannot be parsed. The guard fails open.

```bash
echo '{"tool_name":"Edit","tool_input":{"file_path":"AGENTS.md"}}' | ai-rulez guard; echo $?
```

## Doctor Command

### `ai-rulez doctor [config-file]`

Read-only diagnostics. It never writes, and reports each problem as an `error`, a `warning` or `info`.

```bash
ai-rulez doctor [config-file] [--strict] [--format json] [--profile <name>] [--no-local] [--config-dir <name>]
```

| Check | Reports | Severity |
| --- | --- | --- |
| `config` | The configuration fails to load, match the schema or validate (same as `validate`) | error |
| `presets` | An unknown or removed preset name, with a suggestion: `windsurf` is now `devin`, `continue-dev` has no replacement, a typo gets a "did you mean" | error |
| `mcp-env` | An MCP `${VAR}` placeholder in `env` or `headers` that no environment variable, `.env` file or `--env` value resolves (`${PROJECT_ROOT}` always resolves). Only servers active in the selected profile are checked | warning |
| `includes` | A remote include with no cached copy. Doctor never fetches, so it cannot see whether the remote is reachable; `generate` fails when an include can be neither fetched nor read from the cache | warning |
| `drift` | Generated files that are `missing`, `stale`, `edited` or `orphan` (the machinery behind [`generate --check`](#detecting-drift)) | warning |
| `gitignore` | Generated paths `generate` wants git to ignore (committed outputs with `gitignore = true`, machine-local and secret outputs always) that git does not ignore; asked through `git check-ignore`, skipped outside a git repository | warning |
| `documents` | A shared settings document ai-rulez merges into (`.claude/settings.json`, `.mcp.json`, `.codex/config.toml`, ...) that no longer parses as JSON, JSONC, TOML or YAML | error |
| `hooks` | A `[[hooks]]` script that does not exist or is not executable. The script paths are checked directly, so the result does not depend on git state or on `[lint]` overrides of `AR504` and `AR505` | error |
| `lock` | `ai-rulez.lock` does not match the remote includes and installed skills, or authored content differs from its content pins (checked offline against the sources, as in `lock --check`; an error when `[lock] enforce = true`) | warning |
| `tools` | The binary behind a preset (`claude`, `codex`, `gemini`, ...) is not on `PATH`; presets whose binary is not known are skipped | info |

When outputs cannot be rendered at all (for example because an MCP placeholder is unset), `drift` says so and the `gitignore` check is skipped; the `mcp-env` finding names the cause.

`doctor` never uses the network and never writes the include cache: it loads the configuration without resolving includes and installed skills. A project that declares them gets one `info` finding on `drift` saying the comparison was skipped (`generate --check` does it against the resolved content), and the `gitignore` check is skipped with it.

| Flag | Description |
| --- | --- |
| `--strict` | Also exit non-zero on warnings |
| `--format text\|json` | `json` prints `{"root", "summary": {"error", "warning", "info"}, "findings": [{"check", "severity", "message", "path", "hint"}]}` instead of the table |
| `--profile` / `-p` | Profile rendered for the `drift` and `gitignore` checks |
| `--no-local` | Ignore the machine-local overlay and `local/` content |
| `--config-dir` / `-n` | Configuration directory name for non-default layouts |

Exit codes: `0` no errors (and, with `--strict`, no warnings), `2` at least one finding at the failing severity, `1` doctor could not run: the configuration does not load at all (the `config` finding is still printed) or the report could not be written. The MCP server exposes the same checks as the read-only `doctor` tool, with URL credentials removed from every message, hint and path.

## Cost Command

### `ai-rulez cost [config-file]`

Find what to trim. `tokens` reports the token surface per runtime and bucket; `cost` adds the per-item view:
the biggest offenders among rules, context files, skills, agents and commands, split into what is paid on every
request and what is paid only when an item is opened.

- **Always loaded** per item: a rule or context body (a path-scoped rule is *conditional* instead), or the name
  and description of a listed skill, agent or command plus the 27-token listing framing used by `tokens` (an item
  with `disable-model-invocation: true` is not listed).
- **On demand** per item: the body of a skill, agent or command.
- The runtime totals at the top are the `ai-rulez tokens` figures for the target. The per-item table is an estimate
  from the sources and does not add up to them exactly; both are shown, never summed.

```bash
ai-rulez cost                                   # largest runtime, top 10 offenders
ai-rulez cost --target codex --top 5
ai-rulez cost --format markdown                 # for a PR comment
ai-rulez cost --format json
ai-rulez cost --budget 8000 --on-demand-budget 60000   # exit 2 when over, naming the top 3 offenders
```

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--format` | string | `text` | `text`, `json` or `markdown` |
| `--target` | string | largest runtime | Preset whose runtime totals to report |
| `--top` | int | 10 | How many offenders to list per bucket |
| `--budget` / `-b` | int | 0 | Exit 2 when the target's always-loaded tokens exceed this |
| `--on-demand-budget` | int | 0 | Exit 2 when the target's on-demand tokens exceed this |
| `--profile` / `-p` | string | configured default | Profile to report on |
| `--tokenizer` | string | `cl100k_base` | `cl100k_base` or `estimate` |
| `--no-local`, `--config-dir` | | | As for `tokens` |

Exit `0` within budget, `2` over a ceiling, `1` the configuration could not be loaded.

## Verifiers Command

### `ai-rulez verifiers run|list|explain|test|calibrate|suggest`

Run the read-only, deterministic repo checks declared as `[[verifiers]]` in `config.toml` (a file exists or is absent, a glob matches a bounded number of files, a regex is required or forbidden, a JSON/YAML/TOML key has a value, generated files are in sync) and as rule-linked specs under `.ai-rulez/verifiers/*.toml` (paired files, `all`/`any`/`not`, changed-only scope; a failure names the rule or skill it enforces). Verifiers never write. Two predicates of a rule-linked verifier are opt-in per run: `command` starts a program (`--allow-exec`) and `llm` sends the changed lines to a model (`--allow-llm`); without the flags nothing runs and nothing leaves the machine. Types, fields and semantics are in [Verifiers](verifiers.md) and the [`verifiers` reference](configuration.md#verifiers).

```bash
ai-rulez verifiers run [config-file] [--since <rev> | --staged | --all] [--rule <id>] [--name <name>]... [--format text|json|sarif|junit] [--out <file>] [--fail-on error|warning|info|none] [--strict] [--strict-applicability] [--profile <name>] [--role <name>] [--allow-exec] [--allow-llm] [--gate-llm] [--max-cost <usd>] [--estimate] [--no-local] [--config-dir <name>]
ai-rulez verifiers list [config-file] [--format json] [--no-local] [--config-dir <name>]
ai-rulez verifiers explain <name> [config-file]
ai-rulez verifiers test [name...] [--allow-exec]
ai-rulez verifiers calibrate [name...] [--allow-llm] [--max-cost <usd>] [--estimate] [--no-write] [--format json]
ai-rulez verifiers suggest <id> [--kind rule|skill|agent|command] [--max-proposals <n>] [--replay <n>] [--write] [--allow-llm] [--max-cost <usd>] [--estimate] [--format json]
```

| Flag | Description |
| --- | --- |
| `--since <rev>` | Evaluate only files changed since the merge base of `<rev>` and `HEAD`, plus uncommitted and untracked files. A revision that does not exist or share history with `HEAD` (a shallow clone) is an error (exit `1`), never a pass |
| `--staged` | Evaluate only staged changes (exit `1` outside a repository or without commits) |
| `--all` | Evaluate every file (the default); exclusive with `--since` and `--staged` |
| `--rule <id>` | Run only the verifiers that enforce this rule, skill, agent or command |
| `--format` | `text` (default), `json` ([`schema/verifiers-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verifiers-report.schema.json)), `sarif` or `junit` |
| `--out <file>` | Write the report to a file instead of stdout |
| `--fail-on` | Lowest failing severity: `error` (default), `warning`, `info` or `none` |
| `--strict` | Same as `--fail-on warning` |
| `--strict-applicability` | Report a verifier whose `when_changed` matches no file of the repository (`AR9H5`) |
| `--name` | Run only the named verifier (repeatable) |
| `--profile` / `-p` | Active profile: the profile of `generated_in_sync` verifiers that name none, and which rules count as active (a verifier whose rule is outside it is `inactive`) |
| `--role` | Active role, same meaning |
| `--allow-exec` | Let `command` predicates run a program (env `AI_RULEZ_VERIFIERS_ALLOW_EXEC=1` in CI). Without it a command verifier is `AR9H3`, exit `1`. Never implied by another flag |
| `--allow-llm` | Evaluate `llm` verifiers: sends the changed lines to the configured model. Needs `allow_network = true` in the user config; otherwise the verifier is `skipped` (`AR9H4`) |
| `--gate-llm` | Let a failing `llm` verifier declared at `error` keep that severity when its calibration record (`verifiers calibrate`) is current and meets the bar; otherwise every `llm` verdict is capped at `warning` |
| `--max-cost <usd>` | Most an `llm` run may cost (default `0.50`, `0` removes the cap; `[llm]` limits still apply) |
| `--estimate` | Print which files and how many bytes the `llm` verifiers would send and the cost bound; calls nothing |
| `--no-local` | Ignore the machine-local overlay and `local/` content |
| `--config-dir` / `-n` | Configuration directory name for non-default layouts |

`list` prints what is declared (the rule or skill each verifier enforces, invalid declarations included) without evaluating it. `explain` prints what one verifier checks, the item it enforces, its scope and its fix. `test` runs the `[[verifiers.examples]]` of each spec offline (exit `0` all match, `2` one does not or a declaration is invalid, `1` the configuration does not load or a name is unknown). All commands (and `validate`) check the `config.toml` verifiers first: unknown types, fields that do not apply to the type, globs that do not compile or expand to more than 64 brace alternatives, an unsupported `key_equals` file extension or malformed key, and a `generated_in_sync` profile that is not defined are rejected at load time. Invalid spec files are reported as `AR9H2`. `calibrate` measures the precision and recall of an `llm` verifier's `fail` verdict on its labelled examples and records it in `.ai-rulez/verifiers/calibration/<id>.json` (`--no-write` prints it only); `run --gate-llm` reads that record. `suggest` asks the model for candidate verifiers for a rule and prints the ones that pass deterministic checks; it never writes without `--write` (see [Verifiers](verifiers.md#suggesting-verifiers)). `[verifiers_settings]` holds the limits and policy (`max_timeout_s`, `max_file_bytes`, `require_examples`, `warn_dead`, `trust_exec_from`, `command_env`).

Exit codes: `0` no verifier failed at the `--fail-on` severity, `2` at least one failed, even when another verifier could not be evaluated (a failure is never hidden behind exit `1`; the report shows both), `1` nothing failed but the run could not complete: the configuration does not load or validate, a `--name` is unknown, the `--since` base cannot be used, or a verifier could not be evaluated (status `error`). A failing `info` verifier never fails the run unless `--fail-on info`. The MCP server exposes the same run as the read-only `run_verifiers` tool (with `since`, `staged` and `rule` parameters); it does not resolve includes, so `generated_in_sync` reports `error` there for a project that declares includes or installed skills.

## Tokens Command

### `ai-rulez tokens [config-file]`

Report how many prompt tokens the generated configuration costs, split by when an
agent loads it.

Artifacts are measured as rendered strings in memory — nothing is read back off
disk, so the report is correct even before `generate` has ever run, and a stale
output tree cannot corrupt the numbers.

**Syntax:**

```bash
ai-rulez tokens [config-file] [flags]
```

**Flags:**

| Flag                  | Type    | Default            | Description                                                      |
| --------------------- | ------- | ------------------ | ---------------------------------------------------------------- |
| `--format`            | string  | `text`             | `text` or `json` |
| `--budget` / `-b`     | int     | 0                  | Exit 2 when the headline always-loaded count exceeds this ceiling |
| `--compare-profiles`  | strings | none               | One profile per column of a comparison table; repeat the flag per column |
| `--tokenizer`         | string  | `cl100k_base`      | `cl100k_base` (offline BPE) or `estimate` (byte ratio)           |
| `--no-local`          | boolean | false              | Ignore the machine-local overlay and `local/` content            |
| `--profile` / `-p`    | string  | configured default | Profile to report on; a comma-separated list composes several    |
| `--role`              | string  |                    | Report on a [role](roles.md)'s slice instead of a profile; skills the role delivers as `served` are left out of the listing and named in `served_skills` |
| `--by-role`           | boolean | false              | One comparison column per declared role                          |
| `--config-dir` / `-n` | string  | `.ai-rulez`        | Configuration directory name for non-default layouts             |

```bash
ai-rulez tokens
ai-rulez tokens --format json
ai-rulez tokens --compare-profiles base --compare-profiles backend --compare-profiles full
ai-rulez tokens --compare-profiles base --compare-profiles base,backend
ai-rulez tokens --budget 6000
ai-rulez tokens --role backend
ai-rulez tokens --by-role
```

### Reading the report

Output is grouped per runtime (one provider's files) and, within a runtime, by
when the surface is loaded:

| Bucket        | Meaning                                                                       |
| ------------- | ----------------------------------------------------------------------------- |
| `always`      | Paid on every request: the root instructions file and the item listing (below) |
| `conditional` | Paid in some harness modes only — path-scoped rule files and machine-local files, labelled `(machine-local)` for roots |
| `on demand`   | Paid when the artifact is opened: skill, command and agent bodies             |
| `unmodeled`   | Cost ai-rulez cannot model, such as the tool schemas an MCP manifest implies  |

Native rule files (`.claude/rules`, `.cursor/rules`, `.github/instructions`, ...) are
bucketed by their activation, read from each file's frontmatter: always-on files count
as `always`, path-scoped files as `conditional` ("path-scoped rule files"),
agent-requested rules split into an `always` description and an `on demand` body, and
manual rules are `on demand`. Machine-local rule files (`*.local.*`) count as `conditional`.

The root instructions file is broken down per section, and rules and context are
listed individually so an expensive one can be named. The item listing, bodies and
file overhead are separate lines: they are loaded on different schedules, and a
single per-file total hides which part is actually costing anything.

#### The item listing

A harness that supports skills does not wait for a skill to be opened. At session
start it puts a listing of the skills (and, for some, commands and agents) it can
load into the prompt: one entry per item with its name and description, and for
Codex and pi its path. That listing is paid on every request, whether or not a
skill is ever used, and it is usually the largest always-loaded cost a skill tree
adds. Each runtime whose harness lists items gets one `skill listing` line (and
`command listing` / `agent listing` where the harness lists those), with the
names, descriptions, paths and per-entry framing as children. The listing is
included in the runtime's `always` figure, in the headline and in `--budget`.

| Harness                                  | Lists                       | Source of the model |
| ---------------------------------------- | --------------------------- | ------------------- |
| `claude`                                 | skills, commands, agents (descriptions cut at 1,536 characters) | documented, measured |
| `codex`                                  | skills, with path           | documented          |
| `pi`                                     | skills, with path           | documented          |
| `gemini`, `opencode`, `devin`, `cline`, `junie` | skills            | documented          |
| `cursor`, `copilot`                      | skills                      | implied by the docs, not stated as a per-request listing |
| `amp`, `antigravity`, `baz`, `hermes`, `xum` | not modeled | no listing is reported |

An item with `disable-model-invocation: true` is not offered to the model and is
not listed. Each entry costs the token count of its name, description and path
plus a framing constant of 27 tokens (`listing_entry_overhead` in the JSON).

The figure is an estimate, and so is the constant. Calibration method, which you
can repeat on your own harness version:

```bash
mkdir p && cd p && git init -q
# empty project
claude -p "reply with the single word ok" --output-format json \
  --setting-sources project --strict-mcp-config --mcp-config '{"mcpServers":{}}'
# then add 100 skills with ~165-character descriptions under .claude/skills and rerun
```

The prompt size is `input_tokens + cache_creation_input_tokens +
cache_read_input_tokens`. On Claude Code 2.1.289, 100 such skills added 6,402
tokens (64 per skill); `ai-rulez tokens` reports 6,400 for the same skills as an
ai-rulez source. The constant is the difference between that per-skill figure and
the `cl100k_base` count of the name and description (3 + 34 tokens), so it also
absorbs the tokenizer gap on that text.

What the estimate does not model: a harness bounds its listing and shortens or
omits entries beyond it (Codex: a share of the context window; Claude Code: a total
budget in addition to the 1,536-character per-entry cap, which capped the same probe
at about 9.6k tokens from 100 skills up). A very large skill set is therefore an
upper estimate. `truncated_descriptions` counts entries whose description exceeds a
known per-entry limit. With `agents_md`, skills written only to the shared
`.agents/skills` tree are not attributed to a preset.

The earlier model counted only skill and command names (and agent descriptions) as
`always` and skill descriptions as `conditional`. Those figures remain in the JSON
as `always_legacy`, `conditional_legacy` (per runtime) and `headline_always_legacy`,
and the text report prints the pre-listing headline next to the listing share, so a
number recorded before this change stays comparable. `headline_always` itself now
includes the listing, so a `--budget` that passed before can fail; raise the ceiling
or trim descriptions. Provider specs declare the listing with a `[listing]` table
(`skills`, `commands`, `agents`, `include_path`, `description_limit`).

`--budget` compares against the headline figure and exits `2` when it is exceeded,
which is distinct from `1` so a hook can tell "over budget" from "the command
failed".

### What the numbers are not

- **Approximate.** Claude's tokenizer is not published. The default counter uses
  the embedded `cl100k_base` BPE vocabulary, which measured 8% low against one
  real 19,230-byte instruction file. Use the numbers to compare profiles, and to
  compare before an edit against after — not as absolute truth.
- **Not a session total.** ai-rulez counts only the artifacts it generates. The
  agent harness adds a fixed floor of its own system prompt and tool schemas, plus
  per-artifact overhead, neither of which ai-rulez can see.
- **Not additive across runtimes.** One session loads one runtime's root
  instructions file, so emitting both `CLAUDE.md` and `AGENTS.md` costs one of
  them. The headline is the largest single runtime, not the sum.
- **Noisy in the provenance lines.** A blake3 hex digest is incompressible, and two
  digests of the same length do not tokenize to the same count, so a few tokens per
  artifact move between two profiles for no reason you can act on.

`--tokenizer estimate` replaces the tokenizer with a bytes-per-token ratio. It is
labeled as an estimate in the output because the real ratio has been measured
between 1.81 and 5.30 bytes per token across whole trees, so a tree-level total
from it can be wrong by a factor of three.

### Acting on the report

The `agents_delegation` line under the root instructions file is the cheapest thing to
cut: it restates every agent's name and description in a file loaded on every request,
while `.claude/agents/*.md` already carry the same text on demand. Drop it with
`builtins = ["!agent-delegation"]` — the agent files are still generated, so nothing is
lost. See [Configuration](configuration.md#drop-the-agents-roster-from-root-files).

## Telemetry Commands

Opt-in usage and item-load telemetry, documented in [Usage telemetry](usage-telemetry.md) and
[Telemetry](telemetry.md). One namespace replaces the 4.x `usage ...` and `telemetry report|evals` commands.

| Command | Purpose |
| --- | --- |
| `ai-rulez telemetry hook [--harness claude\|codex\|cursor] [--role r] [--format json\|toml] [-o file] [--log f] [--sink-command c] [--index f] [--executable e]` | Print (or write) the hooks that record skill, rule, context and agent loads (Claude Code: `InstructionsLoaded`, `SubagentStart`, `SubagentStop` plus the skill hooks; codex and cursor: the skill hook) as a hooks block or `[[hooks]]` groups; other harnesses warn and print nothing |
| `ai-rulez telemetry record [--harness h] [--role r] [--outcome o] [--served] [--salt-file f] [--log f] [--sink-command c] [--index f] [--root dir]` | Read one hook event on stdin: a skill invocation appends an identifier-only JSON line to the usage log, an item load is recorded when `[telemetry] enabled`; silent, always exits 0 |
| `ai-rulez telemetry feedback <skill> --kind misled\|stale\|wrong\|great [--note-file f] [--log f] [--harness h] [--role r]` | Append an identifier-only feedback record; the note text stays in `feedback-notes/` |
| `ai-rulez telemetry report [log] [--index f] [--feedback f] [--evals f] [--items] [--format json] [-n dir]` | Join a usage log (default `.ai-rulez/local/usage.jsonl`) with `skills-index.json`, feedback and eval scores: used, never used, changed since used, unknown; rule, agent and context sections when the log holds item events |
| `ai-rulez telemetry report evals [--usage f]... [--from-otlp] [--feedback f] [--results f] [--min-pass-rate r] [--min-trigger r] [--format json] [-n dir]` | Rank skills to rewrite, prune, review or keep from eval scores joined with usage and feedback |
| `ai-rulez telemetry export --to file <path> [--file f] [--log f] [--with-evals] [--dry-run] [-n dir]` | Write the usage log as an OTLP JSON file (one logs request per line, allowlisted identifier-only fields, deterministic); no network |
| `ai-rulez telemetry export --to otlp [--all] [--with-evals] [--max-batches n] [--dry-run]` | Push the usage log past the export cursor (and eval results) to the consented collector; exits 1 when delivery fails |
| `ai-rulez telemetry prune --keep-days n [--dry-run] [--ignore-cursor] [--log f]` | Delete usage-log lines older than n days that are behind the export cursor |
| `ai-rulez telemetry enable [--endpoint url] [--protocol http/json\|http/protobuf\|grpc] [--include-session] [--include-paths] [--backfill]` | Store your consent for one collector (per user, mode 0600; a repository cannot grant it) |
| `ai-rulez telemetry status [--format json]` | Recording and export on or off, consent state, pending events, cursor, failed flushes |
| `ai-rulez telemetry disable` | Withdraw consent |
| `ai-rulez telemetry flush [--background] [--timeout d] [--root dir]` | Send the local outbox to the OTLP collector with retry and backoff |
| `ai-rulez telemetry preview [--log f] [--limit n] [--with-evals] [--root dir] [-n dir]` | Print the exact OTLP requests an export would send (destination, body, exported and withheld fields) from the outbox or the usage log; sends nothing |
| `ai-rulez telemetry doctor [--format json] [--root dir]` | Show the resolved telemetry config and where each key came from, consent, endpoint host, buffer and last flush |

## Eval Commands

Documented in [Evals](evals.md).

| Command | Purpose |
| --- | --- |
| `ai-rulez eval run [skill...] [--harness h] [--runner claude-plugin-eval\|command\|claude-native\|codex-native] [--runner-command c] [--ablation] [--dry-run\|--estimate] [--mode cases\|activation] [--surface retrieval\|native] [--scope domain\|all] [--description-from file] [--format json\|markdown\|junit] [--out dir] [--max-cost usd] [--max-cost-mode expected\|high] [--grader runner\|builtin] [--allow-llm] [--grader-max-cost usd] [--changed-only] [--base ref] [--date d] [--force] [--threshold r] [--allow-exec] [--model m] [--runs n] [--timeout d] [--claude-bin b] [--codex-bin b] [--runner-arg a] [--judge-model m] [--results f] [--no-write] [--price-in usd] [--price-out usd] [-n dir]` | Run eval cases through a runner, score each skill and record `.ai-rulez/eval-results.json`. `--dry-run`/`--estimate` prints a cost range; `--mode activation --surface retrieval` measures offline, for free, whether the right skill ranks first, and `--surface native` asks a harness's model over repeated runs, after a runner capability handshake ([Activation mode](evals.md#activation-mode)). `--description-from file` measures a candidate description of one skill without editing it or recording the result ([Comparing descriptions](evals.md#comparing-descriptions)). `--grader builtin` grades rubrics with the `[llm]` model from the runner's transcript, with `--allow-llm` and a spend cap ([The built-in grader](evals.md#the-built-in-grader)). Exit 2 when a skill fails its threshold, errors, or has invalid cases |
| `ai-rulez eval import --from tessl <path>... [--skill id] [--out dir] [--dry-run] [--lift-assertions] [--rubric-mode single\|items] [--report f] [--force] [--format text\|json] [-n dir]` | Import scenarios (a task plus a weighted checklist) from another tool as eval cases, offline, listing every field that has no counterpart as unmapped (`AR9A5`, informational). Nothing is written unless every scenario maps; existing files need `--force` ([Importing scenarios](evals.md#importing-scenarios)) |
| `ai-rulez eval calibrate-estimate [--results f] [--harness h] [--model m] [--min-samples n] [--format text\|json] [-n dir]` | Propose `[lint.evals.estimate]` assumptions measured from the recorded runs (per harness, model and kind of run), with the token error before and after; changes nothing ([Calibrating the estimate](evals.md#calibrating-the-estimate)) |

## Review Commands

Documented in [Review](review.md).

### `ai-rulez review [id|name|path...]`

Score skills, agents and commands against a rubric. Offline by default (lint evidence, no model); `--semantic` adds an LLM judge.

| Flag | Meaning |
| --- | --- |
| `--rubric id` | `builtin:<id>` or a directory under `.ai-rulez/rubrics` (default `[review] rubric`, else `builtin:skill-quality`) |
| `--content descriptions\|full` | What a judge receives (default `[review] content`, else `descriptions`); body-only dimensions are judged only with `full` |
| `--semantic` | Add the judge. Needs `[llm] allow_network = true` in user scope and a model; findings are advisory |
| `--estimate`, `--show-prompt` | Print the egress manifest and cost range (and the exact messages); send nothing |
| `--model m`, `--models a,b` | Model to judge with (default `[llm] model`); several models are compared |
| `--k n` | Most votes for a flagged dimension (default the rubric's `votes.max`) |
| `--max-cost usd`, `--max-calls n` | Spend caps (defaults `[review]`, else 0.50 and 300; `--max-cost 0` is unlimited) |
| `--no-cache`, `--cache-dir d` | Bypass or relocate the response cache |
| `--concurrency n` | Items judged at once (default 4, at most 16) |
| `--gate`, `--gate-level info\|warning\|error` | Exit 2 on a stable fail verdict of a calibrated dimension; refused without a matching calibration |
| `--baseline f`, `--write-baseline f` | Hide the findings in a baseline or earlier report; write a baseline |
| `--since rev`, `--role r`, `--profile p` | Only items changed since a revision; only the content slice of a role or profile |
| `--include-imports` | Also review content from includes, installed skills and builtins |
| `--format text\|json\|sarif`, `--out file` | Output format and destination |
| `-n dir`, `--no-local` | Config directory name; ignore the machine-local overlay |

Exit codes: 0 reviewed, 1 could not run or was refused, 2 `--gate` failed.

### `ai-rulez review calibrate|fix|explain`

| Command | Meaning |
| --- | --- |
| `review calibrate [--rubric r] [--golden dir] [--model m \| --models a,b] [--k n] [--content full\|descriptions] [--compare f] [--no-probes] [--no-write] [--no-cache] [--out f] [--max-cost usd] [--max-calls n] [--concurrency n] [--format text\|json]` | Measure the judge against the golden set and write `calibration.json`; `--compare` is the drift check. Defaults: `--content full`, `--max-cost 2.00`, `--max-calls 1000`. Exit 2 when it does not pass or drifted |
| `review fix [id\|name\|path...] [--finding fp] [--model fixer] [--judge-model m] [--rubric r] [--content full\|descriptions] [--k n] [--since rev] [--profile p] [--role r] [--max-cost usd] [--max-calls n] [--no-cache] [--concurrency n] [--out f] [--apply] [--allow-same-model] [--patch f] [--format text\|json]` | Propose a verified patch for stable judged findings; never writes without `--apply`. `--content` defaults to `full` here (a fix needs the body). Exit 2 when a finding has no safe fix |
| `review explain CODE` | Explain `AR9G0`-`AR9G9` with the rubric's definitions |

### `ai-rulez rubric list|show|lint`

List the built-in and project rubrics, print one rubric's dimensions and weights, or lint rubrics, golden files and calibration
records (`AR9G8`; `--format text\|json\|sarif`; exit 2 with findings).

## Search Command

Documented in [Skill Search](search.md).

### `ai-rulez search <query>`

Rank the skills `mcp --serve-skills` would serve against a query, with the ranker `find_skill` uses: lexical BM25F (offline, deterministic) by default, or hybrid with an embedding index.

| Flag | Meaning |
| --- | --- |
| `--limit n` | Maximum results (default 5, max 20) |
| `--format text\|json` | Output format (default `text`) |
| `--mode lexical\|hybrid\|vector` | Ranking for this run (default `[search] mode`, then lexical) |
| `--explain` | Show each skill's rank in the lexical and vector lists and how the query was embedded |
| `--allow-exec` | Honor `[search.embeddings] command` from the repository config (it runs a program) |
| `--profile`, `--targets`, `--domain`, `--allow`, `--deny`, `--source`, `--role`, `--include-static`, `--offline`, `--frozen` | Select the catalog, as for `mcp --serve-skills` |

A hybrid or vector search that cannot embed the query ranks lexically and reports `degraded` (`no_index`, `provider_unavailable`, `timeout`, `budget`, `network_disabled`). With the default `[search] fusion = "auto"`, hybrid ranks by cosine alone while every skill has a current vector. With `[search] vector_min_sim` set, a vector ranking with no skill above it returns nothing and reports `abstained` (see [Abstaining](search.md#abstaining)).

### `ai-rulez search index` and `ai-rulez search status`

`index` embeds the served skills and writes the index `find_skill` and `search --mode hybrid` read; only skills whose embedded text changed are sent. `--dry-run` shows the host, the number of texts and bytes and an estimate first; `--rebuild` re-embeds everything; `--items a,b` also re-embeds those skills although their text did not change (every changed or unindexed skill is embedded and sent regardless). A skill whose text looks like it holds a secret is withheld (`AR9D3`). A batch the provider rejects is split until the refused skill stands alone, which is skipped and named. Exit 2 when a skipped skill, a budget or a provider stop left skills without a vector (the finished ones are written). `status` reports the index against the served skills (`none`, `unreadable`, `incompatible`, `stale`, `fresh`) without any network call. See [Skill Search](search.md#building-the-index).

### `ai-rulez search mine`

Turn the opt-in query log (`[search] log_queries = true` in the user config, or `AI_RULEZ_SEARCH_LOG_QUERIES=1`) into candidate cases: `--out file` (default stdout), `--min-count n`, `--purge` (delete the log afterwards).

### `ai-rulez improve run <skill> --with CMD` (experimental)

Runs an external optimizer on a throwaway copy of an authored skill and accepts its candidate only if a held-out eval set improves without regressions. The authored skill is untouched until `improve apply`, which never commits. Required: `--with` and `--max-cost`.

| Flag | Default | Meaning |
|---|---|---|
| `--with CMD` | | optimizer command, run without a shell (words, or a JSON array) |
| `--holdout-tag TAG` / `--holdout-fraction F` | `holdout` / `0.3` | held-out cases by tag, else a deterministic split |
| `--min-gain G` / `--max-regressions N` | `0.05` / `0` | acceptance gate on the held-out set |
| `--max-rounds N` / `--max-holdout-evals N` | `3` / `3` | optimizer invocations / held-out evaluations |
| `--max-cost USD` | required | ceiling for evals plus optimizer-reported cost |
| `--runs N` | `3` | eval runs per case, majority vote |
| `--timeout D` | `20m` | per optimizer invocation |
| `--eval-timeout D` | `30m` | per eval runner call |
| `--env-pass A,B` / `--egress H,I` | | forwarded environment names / declared hosts (credential-like names need `--egress`) |
| `--allow-frontmatter` / `--allow-scripts` | off | widen the diff policy |
| `--adapter NAME` | | a bundled optimizer, the same as `--with builtin:NAME` (`improve adapters`) |
| `--adapter-model M` / `--adapter-judge-model M` / `--allow-same-model` | | `builtin:review-fix`: fixer and judge models; they must differ unless allowed |
| `--isolation none\|auto\|require` | `none` | confine the optimizer with the process sandbox (`AR9J7`) |
| `--require-ci-above-zero` | off | also require the 95% bootstrap interval of the gain to exclude zero |
| `--sibling-native` / `--sibling-runs N` | off / `3` | also run the sibling trigger guard on the harness's model (costs money, counted against `--max-cost`); `N` repetitions per prompt |
| `--trust-repo-optimizer` | off | use `[improve] optimizer` and `env_pass` from a repository config (`AR9J6`) |
| `--runner-command`, `--harness`, `--model`, ... | | the eval runner, as for `eval run` |
| `--yes`, `--dry-run`, `--format json`, `--stop-at-first-accept` | | skip the prompt, plan only, JSON report |

Defaults can come from the `[improve]` table; a flag wins.

Exit 0: candidate accepted. Exit 2: no acceptable candidate (report written). Exit 1: refused or could not run. See [Improve](improve.md).

### `ai-rulez improve apply <run-id>` (experimental)

Shows the diff of an accepted run and writes it into the skill after confirmation (`--yes` skips it). Refuses with `AR9J1` when the skill changed since the run. `--allow-frontmatter` and `--allow-scripts` widen the diff policy as for `run`; `--format text|json`.

### `ai-rulez improve show|clean|pr|adapters` (experimental)

- `improve show <run-id>`: the report of a saved run (rounds, held-out comparison with its bootstrap interval, sibling guard, costs) and the diff; `--format json` prints `improve-show/1`.
- `improve clean [<run-id>|--all]`: delete saved runs (`--dry-run`, `--yes`).
- `improve pr <run-id>`: branch and pull request from an isolated worktree (`--base`, `--remote`, `--draft`, `--no-push`, `--run-evals`, `--eval-arg`, `--env-pass`, `--isolation none|auto|require`, `--allow-frontmatter`, `--allow-scripts`, `--yes`); the ai-rulez commands it runs in the worktree get a scrubbed environment, so pass the eval runner's credentials by name with `--env-pass`. Refusals carry `AR9J8`.
- `improve adapters [name]`: list the bundled optimizers, or print a template (`shell`, `research`, and `repair-workflow`, a scheduled GitHub Actions workflow that repairs skills after a model change).

### `ai-rulez search --eval <cases.yaml>`

Measure the ranking against labelled queries: top-1, recall@k, hit@k, MRR and, for graded cases, nDCG@k. With positive and negative cases and a vector or hybrid mode it also prints the `[search] vector_min_sim` that best separates them (`calibration`, `abstain` rate; see [Abstaining](search.md#abstaining)).

| Flag | Meaning |
| --- | --- |
| `--k n` | Cut-off of recall@k, hit@k and nDCG@k (default: the file's `k`, then 5) |
| `--mode a,b` | One or more of `lexical`, `vector`, `hybrid`, or `all`; the first is gated. Several modes are compared with paired intervals |
| `--from-evals` | Also (or only) use cases derived from the skills' eval-runner cases |
| `--min metric=value,...` | Fail when a metric (`top1`, `recall`, `hit`, `mrr`, `ndcg`) of the primary mode is below the floor |
| `--baseline result.json` | Compare with an earlier `--out` file; counts cases that went from found to missed |
| `--max-flips n` | With `--baseline`: allowed regressions (default 0) |
| `--out result.json` | Also write the result as JSON |

Exit codes: 0 pass, 1 cannot run (bad flags, invalid cases file `AR9D2`, no catalog, a vector mode with no index or with degraded embeddings), 2 a gate failed (`AR9D4`).

## Validation Command

### `ai-rulez validate [config-path]`

Validate configuration without generating files.

**Syntax:**

```bash
ai-rulez validate [config-path] [flags]
```

**Arguments:**

- `[config-path]` (optional): Path to configuration file. If not provided, auto-detected.

**Flags:**

| Flag                  | Type    | Description                                          |
| --------------------- | ------- | ---------------------------------------------------- |
| `--recursive` / `-r`  | boolean | Validate every discovered config; exits non-zero if any is invalid |
| `--config-dir` / `-n` | string  | Configuration directory name for non-default layouts |
| `--no-local`          | boolean | Skip the machine-local overlay and `local/` content: validate the shared view |
| `--config-only`       | boolean | Check the configuration file only and skip the content checks (dead globs, links, references, hooks, size). See [Strict validation](strict-validation.md) |
| `--strict`            | boolean | Fail on warnings as well as errors (the same as `--fail-on warning`); cannot be combined with `--config-only` or another `--fail-on` |
| `--show-policy`       | boolean | Print the effective [organization policy](policy.md) with the origin of every value and what the repository tried to loosen (text, or `--format json`), then exit; exit 1 when the repository loosens it |
| `--format`            | string  | `text` (default), `json`, `sarif`, `github`, `junit` or `markdown`; any value other than `text` writes the findings in that format |
| `--output`            | string  | Write the report to this file instead of stdout |
| `--fail-on`           | string  | Lowest severity that exits 2 (`error` default, `warning`, `info`, `none`) |
| `--external`          | boolean | Also run the `[[lint.external]]` scanners and merge their findings |
| `--allow-egress`      | strings | With `--external`: allow the named scanners that declare `egress = true` to run (repeatable; a name no `[[lint.external]]` declares is an error) |
| `--write-baseline`, `--reason`, `--scanner-baseline`, `--show-suppressed` | | With `--external`: the scanner baseline flags ([Scan Command](#scan-command)) |
| `--no-scan-cache` | boolean | With `--external`: ignore and do not update the scanner result cache |
| `--dry-run` | boolean | With `--external` (and no `--fix`): print what each scanner would run and start nothing ([Scan Command](#scan-command)) |
| `--baseline`          | string  | Accept the findings in this baseline file (default `<config dir>/lint-baseline.json` when present); only new findings fail |
| `--update-baseline`   | boolean | Record every current finding in the baseline (keeps reasons, drops stale entries) and exit 0. Refused with `--fix`, `--since`/`--changed`, `--analyzer`, `--lint-profile` and a shared `--baseline` across roots |
| `--baseline-reason`   | string  | With `--update-baseline`: the reason stored on new entries (required for security findings) |
| `--strict-baseline`   | boolean | Exit 2 when the baseline has stale or expired entries (ratchet) |
| `--since`             | string  | Report only findings in files changed since this git revision and in files that refer to them (the whole tree is still resolved) |
| `--changed`           | boolean | Shorthand for `--since HEAD` (uncommitted and untracked changes) |
| `--verifiers`         | boolean | Also evaluate the verifiers (never a command or a model) and report them as `AR9H1`-`AR9H6` findings |
| `--approvals-base`    | string  | Also report approvals added since this git revision for content that also changed since it (`AR716`, see [Approvals](approvals.md#approvals-are-assertions)) |
| `--since-depth`       | string  | With `--since` or `--changed`: how many reference hops to follow from the changed files, a number or `all` (default `1`); each JSON finding carries a `hop` (`changed`, `dependent`, `transitive(n)`) |
| `--since-max-files`   | int     | With `--since` or `--changed`: report at most this many files besides the changed ones, nearest first (`0`: no cap) |
| `--repo-root`         | string  | Repository root that repo-relative paths and git-tracked globs resolve against (env `AI_RULEZ_REPO_ROOT`; default the git top level, else the config's parent); an error outside a git repository |
| `--fix`               | boolean | With `--strict`: apply the safe automatic fixes to authored sources: executable bits (`AR502`, `AR503`, `AR505`), frontmatter key renames (`AR303`), quoted booleans (`AR304`), unclosed fences (`AR806`) and missing final newlines (`AR807`). The harness trap fixes (`AR9C7`, and `AR9CA` `key-misspelt` rows from `.ai-rulez/traps/*.toml`) also rewrite a misspelled frontmatter key in a hand-written harness file outside `.ai-rulez/`. Never generated outputs or security findings |
| `--fix-unsafe`        | boolean | With `--strict`: also apply fixes that can change meaning (skill name normalization, `AR804`); implies `--fix` |
| `--dry-run`           | boolean | With `--fix`/`--fix-unsafe`: print the unified diff (applies with `git apply`) and change nothing |
| `--analyzer`          | strings | Run only these analyzers (repeatable or comma separated; replaces `[lint] analyzers`; unknown names are rejected): `security`, `references`, `hooks`, `mcp`, `duplicates`, `descriptions`, `budgets`, `metadata`, `plugin`, `config`, `roles`, `lock`, `delivery`, `evals`, `okf`, `traps`, `convert`, `search`, `verifiers` |
| `--lint-profile`      | string  | Lint preset `default`, `strict` or `permissive` (overrides `[lint] profile`; not the generation `--profile`) |
| `--explain`           | string  | Print what a rule (code or name) checks, why, a bad and a good example, how to suppress it and its docs link, then exit (`--format json` for a record) |
| `--verbose`           | boolean | Enable verbose output                                |
| `--debug`             | boolean | Enable debug output                                  |

Exit codes: `0` valid, `1` the configuration is invalid or could not be loaded, `2` findings at or above `--fail-on`.

**Examples:**

Explain a rule:

```bash
ai-rulez validate --explain AR401
```

Run the checks as JSON across every root:

```bash
ai-rulez validate --recursive --format json
```

Fail on warnings too:

```bash
ai-rulez validate
```

Check only the configuration file, as `validate` did before v5:

```bash
ai-rulez validate --config-only
```

Validate every config in a monorepo (all roots are checked; exit status 1 if any config is invalid, 2 on findings):

```bash
ai-rulez validate --recursive
```

Validate current configuration (configuration checks, then the deep content checks):

```bash
ai-rulez validate
```

Validate specific config file:

```bash
ai-rulez validate .ai-rulez/config.toml
```

With verbose output:

```bash
ai-rulez validate --verbose
```

### What Gets Validated

The raw config file is also checked against `schema/ai-rules.schema.json`, so an unknown key
or a value outside an enum fails rather than being silently dropped. The structural checks are:

- A `config.local.*` overlay, when present, is checked against `schema/ai-rules-local.schema.json`, and the merged config is validated. The output names the overlay file and prints a one-line summary of overridden, added and removed key paths, never values
- `version` is `"4.0"` (`"3.0"` is rejected)
- `name` is present and non-empty
- All preset names are valid
- A `builtin:<name>` reference in a profile names a real builtin
- `default`, if set, names an existing profile
- **A profile referencing a domain not present in the content tree is a warning, not a failure** (and
  is downgraded to a debug hint when includes are configured, since the domain may come from an include)
- File paths are accessible
- No two skills, or two commands, in the same scope resolve to the same output id. The flat
  (`commands/deploy.md`) and directory (`commands/deploy/COMMAND.md`) forms resolve identically, so
  declaring both is a collision rather than an override.
- No skill and command share an output id. Both render to `.claude/skills/{id}/SKILL.md`, differing
  only in whether the item is user-invocable, so a shared id silently overwrites one with the other.
  This check pools root and every domain, because the output layout has no domain segment.

## Lock Command

### `ai-rulez lock [name...]`

`ai-rulez.lock` (commit it) pins the parts of your AI configuration that an attacker, or an honest mistake, could
change without anyone noticing. See [Lock file](lockfile.md) for the threat model, the hashing scheme and how to
review a lock diff. It records:

- for every git include and installed skill: the source, path and requested ref, the commit the ref resolved to,
  and a `sha256` digest of the imported file tree;
- for every authored rule, context file, skill (with its resources), agent, command, hook and role: a `sha256:<hex>`
  digest of the raw files, with its id, domain, owner and version;
- for every `[[skill_sources]]` entry (`[[source]]`: commit and tree digest) and every skill the skills server
  serves (`[[served]]`: a `sha256:` digest in the same scheme as the content pins; skills that only a role serves
  are included): the entries [`[lock] enforce`](lockfile.md#served-skills-and-skill-sources) checks at serve time;
- a digest of each generated output, and one `tree` digest over everything;
- one `[[scan]]` record per staged external scanner with a cached result for the current content (what was scanned and the outcome; `lock` starts no scanner, so run `scan --external` first; see [Lock file](lockfile.md#scan-records)).

Local-path includes are pinned as `local-include` items, and project scripts run by agent, skill and command frontmatter `hooks` are pinned with their item. Sources are recorded as written in the config, with credentials in a URL redacted. A symlink inside a pinned tree is pinned by its link target.

```bash
ai-rulez lock                     # pin everything (uses the network for remotes)
ai-rulez lock shared              # re-pin one include or skill, keep the other pins (and the content pins)
ai-rulez skill update kreuzberg   # same, for installed skills only
ai-rulez lock --content-only      # re-pin authored content and outputs; offline
ai-rulez lock --check             # verify everything, offline; exit 2 and name each difference
ai-rulez lock --diff              # what `lock` would change, for a pull request
ai-rulez lock --diff --format json
ai-rulez lock --check --format json   # the same document, exit code still 2 on drift
ai-rulez lock --subject               # the digest to sign with cosign (see Lock file, "Signing the lock")
ai-rulez lock --outdated              # sources whose version constraint allows a newer tag (network)
ai-rulez lock --outdated --format json --fail-on-outdated
```

With a lock present, `generate` fetches the **locked commit** instead of the moving ref, so two runs produce
identical output even after the remote moved, and verifies the digest of what it fetched. A mismatch fails the run
(a damaged cache is repaired by fetching the pinned commit again first; a remote that serves different bytes for
the same commit is a hard failure). A source the lock does not cover is fetched as before, with the advice to run
`ai-rulez lock`. `generate` never writes the lock.

| Flag | Description |
| --- | --- |
| `--check` | Verify the lock against the configuration, the sources, the rendered outputs and any cached remote content; exit 2 naming each added, removed or changed item and whether its source or its output changed. When `[signing] require` names the lock it also verifies the signed attestation (`AR720` to `AR727`, see [Signing](signing.md)) |
| `--diff` | Print how the sources and outputs differ from the lock; exits 0. `--format json` follows `schema/lock-diff.schema.json` |
| `--subject` | Print the lock-subject digest (the value a signature over the lock commits to) and what it is computed from; reads the lock only, offline. `--format json` prints the statement (`schema/lock-subject.schema.json`), `--output <file>` writes it. Exit 2 when the stored `tree` does not match the entries. See [Signing the lock](lockfile.md#signing-the-lock) |
| `--outdated` | List the remote includes, installed skills and skill sources that use a `version` constraint with the pinned, allowed and latest tag; reads tags only and writes nothing. `--format json` follows `schema/lock-outdated.schema.json`. Exit 2 for a moved tag (`AR732`) or an unsatisfiable constraint (`AR730`), and with `--fail-on-outdated` for any allowed update. Names and `--kind` limit it. See [Version constraints](lockfile.md#version-constraints) |
| `--fail-on-outdated` | With `--outdated`: exit 2 when any source has an allowed update |
| `--verify-tags` | With `--check`: ask the remotes whether a pinned tag moved (`AR732`, exit 2) or was deleted (`AR735`, warning); needs the network. Also `[lock] verify_tags = true`. See [Checking pinned tags online](lockfile.md#checking-pinned-tags-online) |
| `--offline` | With `--outdated`: refuses to run (it needs the network); `--check` is the offline verification |
| `--accept-findings` | Pin a source although the security scan (AR001-AR009) of its new tree has error findings (otherwise refused, exit 2, nothing written) |
| `--content-only` | Re-pin authored content and outputs only: no network, remote pins kept (served digests of local skills are recomputed when that works offline) |
| `--format text\|json` | Output format of `--check`, `--diff`, `--outdated` and `--subject`; any other mode rejects it. With `--check` the JSON goes to stdout and the exit code still gates |
| `--output <file>` | With `--subject`: write the JSON statement to this file (not with `--recursive`) |
| `--profile <name>` | Profile whose outputs are pinned (default: the profile recorded in the lock, else the configured default) |
| `--role <name>` | Also pin the skills this role serves, as a view of their own (see `mcp --serve-skills --role`), and the rendered outputs of the role. With `--check` or `--diff`, limits the role-output comparison to that role |
| `--roles` | Also pin the rendered outputs of every role, as one digest per role; roles with `pin = true` are always pinned. Not with `--check`, `--diff`, `--kind` or names. See [Composing with roles](lockfile.md#composing-with-roles) |
| `--targets <preset>` | Also pin the view that serves this preset's rendering of the skills (as `mcp --serve-skills --targets`) |
| `--include-static` | Also pin the view that serves static skills too |
| `--source <src>` | Also pin the view with this extra skill source (repeatable, as `mcp --serve-skills --source`). A view is recorded next to the default one as `[[served]]` entries with a `view` key, and a plain `lock` re-pins views recorded earlier |
| `--strict` | Fail without writing (exit 2) when the security scan refuses any served skill (default: leave that skill unpinned, pin the rest and exit 3) |
| `--kind include\|skill\|source\|served` | Limit a refresh to one kind |
| `--recursive` / `-r` | Process every nested root |
| `--config-dir` / `-n` | Configuration directory name for non-default layouts |

Exit codes: `0` ok, `1` the command could not run (a tool error; also `--check` with no `ai-rulez.lock`, or an unknown name), `2` `--check` found drift (also a lock without content pins; with `[lock] enforce`, a missing lock too), or `--outdated` found a moved tag (`AR732`), a deleted one (`AR735`) or an unsatisfiable constraint (`AR730`), or `--strict` refused a served skill the security scan refuses (nothing written), `3` the lock was written but served skills were left unpinned because the security scan refuses them. Over several roots (`--recursive`) the most severe code wins: `1`, then `2`, then `3`.

CI: `generate --locked` fails when the lock is missing or does not cover a configured remote source, or when an
authored source no longer matches the lock's content pins (exit 2); `generate --frozen` additionally never touches
the network. `validate` logs a warning for each remote source that follows a moving ref without a pin, and
`validate` reports it as `AR010` (a warning; an error under enforcement, or raise it with `[lint.severity]`). Enforcement is on
whenever `ai-rulez.lock` exists (`[lock] enforce = false` opts out); it also reports content drift as `AR981` / `AR982`, and `generate`
refuses a remote source the lock does not cover, as `--locked` does. Pinning `ref` to a full commit SHA also counts as pinned.

The lock proves the bytes did not change since you reviewed them, not who published them. To add that, sign it with
[`ai-rulez sign --lock`](#sign-command) and verify with `verify --attestation`, or sign `lock --subject` with `cosign`
([recipe](lockfile.md#signing-the-lock)).

## Sign Command

### `ai-rulez sign [config-path]`

Signs one subject into a Sigstore bundle (a DSSE envelope over an in-toto statement): the lock-subject statement of
`ai-rulez.lock` (written next to the lock), a plugin bundle, a skill directory, an SBOM file or an organization policy. Verify it with
[`verify --attestation`](#ai-rulez-verify-config-path). See [Signing](signing.md) for the policy, keyless and KMS
signing, thresholds, SLSA provenance and cosign interoperability.

```bash
ai-rulez sign --lock --key cosign.key        # key mode, offline
ai-rulez sign --lock --keyless               # Fulcio + Rekor; the identity and digest go to a public log
ai-rulez sign --lock --key awskms:///alias/release   # a key held in a KMS
ai-rulez sign --bundle dist/acme-plugin --key cosign.key --provenance
ai-rulez sign --skill skills/deploy --key cosign.key
ai-rulez sign --sbom sbom.cdx.json --key cosign.key
ai-rulez sign --policy ai-rulez-policy.toml --key cosign.key   # an organization policy ([Policy](policy.md#signed-policies))
ai-rulez sign --lock --key second.key --append       # a second signer, for [signing] thresholds
```

| Flag | Description |
| --- | --- |
| `--lock` | Sign the lock-subject statement |
| `--bundle <dir>` | Sign a plugin bundle: the tree digest of its files, to `<dir>/.ai-rulez.sigstore.json` |
| `--skill <dir>` | Sign a skill directory a publisher ships, to `<dir>/.ai-rulez.sigstore.json` |
| `--sbom <file>` | Sign any SBOM file, to `<file>.sigstore.json` |
| `--policy <file>` | Sign an organization policy file, to `<file>.sigstore.json`; the file must parse as a policy |
| `--provenance` | With `--bundle`: also write SLSA v1 provenance (`.ai-rulez.provenance.sigstore.json`) |
| `--builder-id <id>` | With `--provenance`: the builder id to record (default: the GitHub Actions workflow reference, else ai-rulez's own) |
| `--append` | Write a numbered co-signature file (`X.2.sigstore.json`) beside the existing attestation, for `[signing] thresholds` |
| `--key <file or URI>` | PEM private key (ECDSA P-256/P-384/P-521 or ed25519; PKCS#8 or a cosign key), or a KMS key URI (`awskms://`, `gcpkms://`, `azurekms://`, `hashivault://`). The password comes from `AI_RULEZ_SIGNING_KEY_PASSWORD` or `COSIGN_PASSWORD` |
| `--public-key-out <file>` | With `--key`: write the key's PEM public key, to name it in `[signing]` |
| `--key-password-env <VAR>` | Read the key password from this variable instead |
| `--keyless` | Sign with a short-lived Fulcio certificate and a Rekor log entry (network) |
| `--identity-token-env <VAR>` | With `--keyless`: variable holding the OIDC token (default: the GitHub Actions runtime token) |
| `--interactive` | With `--keyless`: open a browser for the OIDC login when no token is available |
| `--fulcio-url`, `--rekor-url` | Use another Sigstore deployment (default: the public-good instances) |
| `--tlog` | With `--key`: also record the signature in Rekor |
| `--embed-items` | With `--lock`: put the pinned item ids and digests in the statement |
| `--output <file>` | Write the bundle here instead of the default sidecar |

Exit codes: `0` signed, `1` the command could not run, `2` the lock's tree does not match its entries.

## Trust Command

### `ai-rulez trust update`

Fetches the public-good Sigstore trusted root over TUF and caches it in `~/.cache/ai-rulez/sigstore/trusted_root.json`
for offline verification of keyless attestations. It is the only command that touches the network for verification.

## Approve Command

### `ai-rulez approve [item...]`

Records in `ai-rulez.lock` that a reviewer read the content with a given digest and accepted it, and lists, revokes and
prunes those records. `[governance]` decides what needs one. See [Approvals](approvals.md) for the model, the policy
keys and the enforcement points.

```bash
ai-rulez approve --list                         # status of everything that needs approval (--all: every pinned item)
ai-rulez approve --list --format json           # schema/approve-list.schema.json
ai-rulez approve --diff include:shared          # files, scan findings and the previous approval; writes nothing
ai-rulez approve include:shared --reviewer alice@example.org --note "read run.sh" --yes
ai-rulez approve include:shared --accept AR005  # accept a finding you read (stored with the record)
ai-rulez approve --revoke include:shared        # --reviewer limits it to one reviewer's records
ai-rulez approve --prune                        # drop records of removed or changed content
ai-rulez approve --verify-base origin/main      # CI: approvals added together with the content they approve (AR716)
ai-rulez approve include:shared --from-github-review 42 --yes   # link the approving reviews of pull request 42
ai-rulez approve include:shared --sign --key approver.key --yes # signed approval (DSSE); --keyless uses Fulcio and Rekor
ai-rulez approve --revoke include:shared --deny --reason "exfiltrates ~/.ssh"   # revoke, and deny the digest (AR717)
```

| Flag | Description |
| --- | --- |
| `--list`, `--all` | List what needs approval; `--all` adds pinned content that needs none |
| `--diff` | Show files, scan findings and earlier approvals of the named items |
| `--revoke`, `--prune` | Remove records |
| `--verify-base <rev>` | Compare the lock with the one at the merge base of `<rev>` and `HEAD` and report every approval added since for content that was added or changed since (`AR716`); exit `2` when there is one; writes nothing |
| `--from-github-review <pr>` | Record one `review-linked` approval per approving review of the pull request (reviewer `github:<login>`, `ref` the review URL), after checking through the forge API; needs the network and a token |
| `--sign` | Sign the approval as an in-toto statement (DSSE) with `--key` or `--keyless`; the signer's identity is the reviewer. Takes the signing flags of [`sign`](#sign-command): `--key`, `--key-password-env`, `--keyless`, `--identity-token-env`, `--interactive`, `--fulcio-url`, `--rekor-url`, `--tlog` |
| `--resolve-teams` | Expand `@org/team` entries of `approvers` and CODEOWNERS from the forge (needs `read:org`), for `--list` and approving |
| `--deny`, `--reason` | With `--revoke`: add the item's digest to the `[[deny]]` list, with a reason |
| `--base <rev>` | With `forbid_self_approval`: count commit authors since this revision (default: the branch's upstream) |
| `--reviewer` | Reviewer to record (default `$AI_RULEZ_REVIEWER`, else git `user.email`); with `--from-github-review` it picks one of the approving reviewers |
| `--note` | Free-text note, scanned for secrets |
| `--accept <code>` | Accept an error-level scan finding by code (repeatable) |
| `--expires`, `--at` | Expiry date (default today + `[governance] max_age`); approval time for reproducible runs (not in the future) |
| `--yes` | Do not ask; required without a terminal |
| `--format json` | With `--list` |

Items are named `kind:id` or `kind:domain/id` (`skill:backend/deploy`, `include:shared`, `hook:PreToolUse:*:0`); only
pinned content can be approved. Exit codes: `0` ok, `1` the command could not run or refused (an unknown or ambiguous
item, an error-level finding, no `--yes` off a terminal, a reviewer outside `[governance] approvers`), `2` `--verify-base`
found an approval added together with its content. The reviewer is stored lower-cased and trimmed.

## Update Command

### `ai-rulez update [name...]`

Move the lock entries of includes, installed skills and skill sources that ask for a version range
(`version = "^1.2"`) to the newest tag the range allows. Only the lock changes. See
[Version constraints](lockfile.md#version-constraints) for resolution, the moved-tag and downgrade defenses and the
design decisions.

```bash
ai-rulez lock --outdated          # what has newer tags
ai-rulez update --dry-run         # what update would change; fetches the new trees into the cache, writes no lock
ai-rulez update shared            # one source
ai-rulez update --kind skill      # every installed skill with a range
ai-rulez update --accept-moved-tag shared   # re-pin a tag that moved, after reviewing the new commit
```

| Flag | Description |
| --- | --- |
| `--dry-run` | Show what would change (tags, commits, tree digests, changed files, scan result); write nothing |
| `--allow-downgrade` | Allow a tag with lower precedence than the pinned one |
| `--accept-findings` | Write a pin although the security scan of the new tree has error findings (otherwise refused, exit 2) |
| `--major` | Only sources with a newer major version than their constraint allows: print the constraint that takes it |
| `--write-config` | With `--major`: rewrite only the `version` value of those sources in `config.toml` and move their pins; restored if anything is refused |
| `--accept-moved-tag` | Re-pin a tag that now points to another commit (`AR732`) |
| `--kind include\|skill\|source` | Limit the update to one kind |
| `--format text\|json` | Output format; JSON follows `schema/update.schema.json` |
| `--offline` | Refuses to run: update reads the remote's tags |

Exit codes: `0` done or nothing to do, `1` could not run, `2` a source was refused (`AR732`, `AR730`, `AR731`, or scan
findings) and nothing was written. A source with `min_release_age` skips tags that are too young (`AR733`). `ai-rulez skill update` re-resolves plain refs and keeps range pins; `update` moves them.

## Roles Command

### `ai-rulez roles list|show|resolve`

Inspect `[[roles]]`. Every subcommand accepts `--format text|json`, `--no-local` and `--config-dir`.

| Command | Output |
| --- | --- |
| `roles list` | Every role with its parent, domains, item counts and token estimate. `--format json` prints the `roles.json` document |
| `roles show <name>` | The role as declared and, when it extends another, with the parent merged in, plus its problems (AR971 to AR973) |
| `roles resolve <name>` | The items the role keeps, with kind, domain, id, skill mode, bytes and tokens |

Generate for a role with `ai-rulez generate --role <name>` (or `generate --user --role <name>`). See [Roles](roles.md).

## Catalog Command

### `ai-rulez catalog`

Print every rule, context file, skill, agent and command with its id, domain, source, owner, version, size, the
sha256 digest `ai-rulez.lock` pins, the roles that keep it, a summary of every role and the lock status. Nothing is
written. `--format json` is versioned and is meant for a UI or an audit script; see
[Integrating an identity tool or UI](roles.md#integrating-an-identity-tool-or-ui).

```bash
ai-rulez catalog [--format text|json] [--schema-version 1|2]
ai-rulez catalog --html <dir> [--role R] [--include-excerpt=false] [--indexable] [--clean] [--base-title T] [--allow-findings AR001]
                 [--render-markdown] [--max-items-per-page N] [--no-owners] [--with-eval[=FILE]] [--with-usage[=FILE]] [--check]
ai-rulez catalog diff <from> [<to>] [--format text|json] [--exit-code]
```

- `--schema-version` picks the JSON version: `1` (default, `schema/catalog.v1.schema.json`) or `2`
  (`schema/catalog.schema.json`: description, source, load cost, lint result and excerpt per item).
- `--html <dir>` writes a static website of the version 2 catalog. It opens from `file://`, makes no network
  request and gives the same bytes for the same input. The directory must be new, empty or marked with
  `.ai-rulez-catalog`; `--clean` removes files an earlier run wrote that are gone now. Refused when the secret
  scanner (`AR001`) flagged an item, unless `--allow-findings AR001`.
- `--role R` keeps only the items role `R` keeps. `--include-excerpt` (default on) controls the body excerpt;
  `--indexable` allows crawlers and turns excerpts off unless asked for.
- `--check` writes nothing and exits `2` when the directory differs from the site that would be generated (a changed,
  missing or unexpected file), for a CI freshness gate.
- `--with-eval[=FILE]` and `--with-usage[=FILE]` add each skill's recorded eval result and use count (aggregates only)
  from `eval-results.json` and `local/usage.jsonl`, or the file named. A missing file is a note, not an error.
- `--render-markdown` renders excerpts as sanitized Markdown (raw HTML shown as text, links shown as text);
  `--max-items-per-page` pages the overview; `--no-owners` leaves owner names out. `[catalog]` in `config.toml` sets
  the defaults for these and for `--base-title`, `--include-excerpt` and `--indexable`.
- The site also has an MCP servers page (names only, never env or header values) and a dependency graph.
- `catalog diff <from> [<to>]` compares two catalogs, each a catalog JSON file (`--schema-version 2`) or a git
  revision read through `git archive` (no checkout, no network); with one argument the other side is the current
  project. `--exit-code` exits `2` when they differ.

See [Catalog](catalog.md).

## SBOM Command

### `ai-rulez sbom`

```bash
ai-rulez sbom [--format cyclonedx|spdx-json] [-o file] [--files none|skills|all] [--profile P] [--role R]
              [--include-outputs] [--no-approvals] [--redact-reviewers] [--verify] [--require-lock]
              [--strict-pins] [--check] [--timestamp [RFC3339|now]] [--online] [-n config-dir]
```

Print a CycloneDX 1.6 or SPDX 2.3 JSON bill of materials: authored items (with licenses, optionally their files with
plain SHA-256), remote sources, MCP servers (purl from the `package` key or a launcher heuristic), approval status and,
with `--verify`, the lock attestation. Remote sources come from the lock and the cache; `--online` also allows
`git ls-remote`. No timestamp unless asked, no secrets, byte-identical across runs, operating systems and line endings.
The machine-local overlay is never included. Nothing is rendered; the only file written is `-o`. `--require-lock`,
`--strict-pins` and `--check` (compare the committed `-o` file, which is never rewritten) exit 2 on failure (`AR752`,
`AR750`/`AR751`, `AR753`). Sign the document with `ai-rulez sign --sbom`. See [SBOM](sbom.md).

## Publish Command

### `ai-rulez publish [verify <dir|oci-ref> | emit <emitter>]`

```bash
ai-rulez publish [--dist dist] [--to github-release|npm|oci] [--tag v1.4.0] [--repo OWNER/REPO] [--channel NAME]
                 [--oci-ref host/path] [--npm-scope @acme] [--public] [--confirm-registry URL]
                 [--sign-key FILE [--sign-key-password-env VAR] [--sign-tlog] | --sign-keyless [--sign-token-env VAR] [--sign-interactive]]
                 [--fulcio-url URL] [--rekor-url URL] [--sbom] [--marketplace] [--emit NAME]... [--experimental]
                 [--runtime R]... [--only NAME]... [--since TAG] [--profile P]
                 [--dry-run | --execute --yes [--force]] [--allow-dirty] [--template file]... [--format text|json]
ai-rulez publish verify <dir|oci-ref> [--key PUBLIC.pem]... [--identity ID --issuer URL] [--trusted-root file] [--require-signature] [--format text|json]
ai-rulez publish emit <emitter> [--out dir] [--experimental] [--channel NAME] [--runtime R]... [--profile P] [--allow-dirty]
```

`--confirm-registry URL` is required with `--to npm --execute` when the committed `[publish.npm]` config names a registry other than the public one.

Runs `validate`, `lock --check`, `verify --plugin`, a secret scan and the `[publish]` policy gates
(`require_approved`, `require_signature`), then writes a reproducible `<name>-<version>.tar.gz`, its manifest,
`SHA256SUMS`, a copy of `ai-rulez.lock`, `RELEASE_NOTES.md` (with the lock changes since the previous tag) and
`publish-plan.json` to `--dist`. `--dry-run` writes nothing. Only `--execute --yes` leaves the machine: `gh release create`,
`npm pack` then `npm publish`, or an OCI push with oras-go, as the plan shows. `--sign-key` and `--sign-keyless` sign the
archive, `--sbom` ships the SBOM, `--marketplace` writes a Claude marketplace index pinned to the release commit (one per
`--channel`), `--emit` runs an emitter and `--runtime` limits the bundle. A `[marketplace]` with members or domain plugins
publishes one bundle per plugin (`--only`). `publish verify` recomputes every digest of a dist directory (or a pulled OCI
artifact) offline and verifies the signature against the keys or identity you name. Exit codes: 0 done, 1 could not
complete, 2 a gate or verification failed. Codes `AR9N0`-`AR9N9`. See [Publish](publish.md).

## Scan Command

### `ai-rulez scan [config-path]`

Security checks only, the `AR0xx` family of [strict validation](strict-validation.md#security-checks): secrets, hidden characters, prompt-injection phrases, risky shell, unrestricted `allowed-tools`, outbound hosts, unpinned remotes. Offline and deterministic. `scan` runs only the `security` analyzer, so hook and config findings (`AR504`, `AR9K0`, ...) belong to `validate`. Flags: `--recursive`/`-r`, `--format text|json|sarif|github|junit|markdown`, `--output`, `--fail-on`, `--lint-profile`, `--external`, `--allow-egress`, `--baseline`, `--update-baseline`, `--baseline-reason`, `--strict-baseline`, `--changed`, `--since`, `--since-depth`, `--since-max-files`, `--repo-root`, `--no-local`, `--config-dir`/`-n`. The baseline and changed-only flags mean the same as on [`validate`](#validation-command) (without needing `--strict`). Exit `0` clean, `1` cannot run, `2` findings at or above `--fail-on`.

With `--external`, the scanners of `[[lint.external]]` also run (see [External scanners](strict-validation.md#staged-input-severity-and-baseline)). Scanner flags, also on `validate`:

| Flag | Meaning |
| --- | --- |
| `--allow-egress <name>` | Allow a scanner declared `egress = true` to run (repeatable) |
| `--write-baseline --reason <text>` | Accept every current scanner finding in `.ai-rulez/scanner-baseline.json` and exit as if clean; `--reason` is required |
| `--scanner-baseline <file>` | Use another scanner baseline file |
| `--show-suppressed` | Also show results the scanner marked suppressed, as `info` |
| `--no-scan-cache` | Ignore and do not update the [scanner result cache](strict-validation.md#result-cache-and-dry-run) |
| `--dry-run` | Print each scanner's command (stage paths as `<stage>`), isolation, environment variable names, staged files and cache state, and start nothing; exit `0` |

`[lint.scanner_policy]`, the embedded profiles and presets, process isolation, the result cache and the lock records are described in [Strict validation](strict-validation.md#policy-presets-and-profiles). The `scan` command accepts `--dry-run` only with `--external`.

### `ai-rulez scanners list|doctor`

```text
ai-rulez scanners list [config-file] [--format text|json]
ai-rulez scanners doctor <name>... | --all [--external] [--format text|json]
```

`list` shows each scanner (including the members of the `[lint.scanner_policy]` preset) with its egress declaration, staged inputs, the presets that contain its profile and whether its binary is on `PATH`; it starts nothing. `doctor` checks the scanners you name (or `--all`): binary path, version (only with `--external`, the same consent as `scan --external`: the scanner, a program the repository names, is started once with `--version` in a scrubbed environment, 10 second timeout, confined as the scanner's `isolation` asks; otherwise "not probed (pass --external)"), egress (and what an egress profile's vendor receives), profile, preset, `required`, version range, `env_pass`, inputs, the isolation this system would apply, timeout and configuration problems. Exit `0` healthy, `2` a checked scanner is missing, misconfigured or has a network flag on an `egress = false` entry, `1` the configuration does not load or a name is unknown. `--format json` prints `{"scanners": [...]}` (name, command, path, found, egress, format, inputs, env_pass, timeout_seconds, problems, status, healthy, profile, presets, from_preset, required, version_range, data_sent, isolation, isolation_backend, and for `doctor` the probed version) with the same exit codes.

## OKF Commands

[Open Knowledge Format](okf.md) export, import and lint. Exit codes: `0` success, `2` the command ran and found
problems (drift with `--check`, lint findings at `--fail-on`, files an import did not overwrite, an import refused by
the security scan), `1` it could not run.

### `ai-rulez export okf`

```bash
ai-rulez export okf [config-file] [--out dir] [--profile p | --role r] [--include rules,context,skills,agents,commands,checks] [--index-style body|frontmatter] [--check] [--config-dir n]
```

Writes rules, context, skills, agents, commands and checks as an OKF v0.2 bundle. Without `--out` the bundle goes
to `okf.dir` (default `docs/okf`). `--out` replaces the contents of that directory, but only when it is empty or
already a bundle (a root `index.md` with `okf_version`); any other directory is refused. `--check` writes nothing and
exits 2 when the bundle on disk differs. `--index-style` (default `okf.index_style`, else `body`) picks the `index.md` scheme. Machine-local content is never exported.

### `ai-rulez import okf`

```bash
ai-rulez import okf <dir|git-url[@ref][#subdir]> [--into rules|context|skills] [--domain d] [--dry-run] [--force] [--format json] [--config-dir n]
```

Converts the concepts of a bundle into `.ai-rulez/` sources: rules, context and skills by `type`, and also agents, commands
and checks when a concept says so in `x-ai-rulez.kind` (`--into` only forces rules, context or skills). The target directory must exist (`--config-dir` selects a
non-default one). Existing files are never overwritten unless `--force`; identical files are reported as unchanged, so a
second run changes nothing. Links between imported concepts are rewritten to the created files; symlinks in the bundle are skipped with a warning. Imported text goes through the `AR001`-`AR011` security scan first and the import is refused
with nothing written when it finds an error. A git source is fetched shallowly into a temporary directory.

### `ai-rulez okf validate`

```bash
ai-rulez okf validate <dir|git-url[@ref][#subdir]> [--format text|json] [--fail-on error|warning|info|none]
```

Lints any OKF bundle with the `AR9B0`-`AR9B9` checks. The default `--fail-on error` only fails on conformance problems. A directory without a root `index.md` naming `okf_version` (an empty one, say) is not a bundle: it reports an `AR9B3` error and exits `2`.

## LLM Commands

Read-only inspection of the `[llm]` model-access setup; neither command sends a prompt unless `doctor --ping` is allowed. See [LLM access](llm.md).

```bash
ai-rulez llm doctor [config-file] [--ping] [--format text|json] [--no-local] [--config-dir <name>]
ai-rulez llm estimate <file> [--max-output <tokens>] [--format text|json]
```

`doctor` prints the resolved backend, model, endpoint host, whether the key variable is set (never its value), whether network use is allowed and the cache directory; `--ping` makes one 1-token call and refuses unless `allow_network = true`. `estimate` approximates the prompt tokens of a file and the worst-case cost offline. A failure exits `1`.

## Migrate Command

### `ai-rulez migrate v5`

Rewrite a 4.x project for ai-rulez 5.0. `migrate` reads 4.x only: a 2.x or 3.x project must first be migrated to
4.0 with ai-rulez 4.x. The full list of breaking changes and the before/after of each is in
[Migrating to v5](migration-v5.md).

**Syntax:**

```bash
ai-rulez migrate v5 [--dry-run] [--check] [--adopt-defaults] [--write] [--recursive] [--config-dir name] [--format text|json]
```

**Flags:**

- `--dry-run`: print the change list and write nothing.
- `--check`: write nothing and exit 2 when a project still needs migration.
- `--adopt-defaults`: do not pin the 4.x defaults (`agents_md = false`, `gitignore = true`, `[header] hashes = "full"`); take the v5 ones.
- `--write`: also rewrite the deprecated frontmatter spellings `permission_mode` and `user_invocable` in the markdown sources.
- `--recursive`: migrate every project found below the current directory.
- `--config-dir`: migrate one config directory name instead of `.ai-rulez` (then `.config/ai-rulez`).
- `--format json`: a machine-readable report with `schema_version`, one entry per project and a summary.

**What It Does (per project):**

1. Converts a 4.x `config.yaml`, `config.yml` or `config.json` to `config.toml`, keeping keys, order and comments, and removes the old file.
2. Sets `version = "5.0"` in place, keeping the rest of the line.
3. Renames `[lint.budget]` and `[lint.tolerate]` to `[lint.ratchet]`, the `windsurf` preset to `devin` and drops the removed `continue-dev` preset.
4. Merges a legacy `mcp.toml`, `mcp.yaml` or `mcp.json` into `[[mcp_servers]]` and removes it (left alone with a warning when `config.toml` already has `mcp_servers`).
5. Pins the three changed defaults to their 4.x values unless `--adopt-defaults`.
6. Converts a `config.local.yaml`, `.yml` or `.json` overlay to `config.local.toml` (owner-only).
7. Rewrites `ai-rulez usage ...` and `telemetry report|evals` to `telemetry ...` inside the hook, verifier and script commands of `config.toml`.
8. Warns about `[[plugins]]` (no longer written to any file) and about a 4.x file it left in place.

The result is decoded with the v5 loader before anything is written; a project that would not load is reported with
its error and left untouched. Running `migrate v5` on a migrated project changes nothing.

**Exit codes:** `0` migrated or nothing to do, `1` a project could not be migrated (or an unsupported target),
`2` `--check` found a project that needs migration.

Then regenerate outputs and review the diff:

```bash
ai-rulez generate
```

## Version Command

### `ai-rulez version`

Show the current version and build information.

```bash
ai-rulez version
```

## MCP Server

### `ai-rulez mcp`

Starts the Model Context Protocol (MCP) server to allow AI assistants to programmatically interact with your configuration.

```bash
ai-rulez mcp
ai-rulez mcp --serve-skills [--profile <p> | --role <r>] [--source <src>] [--frozen]
```

With `--serve-skills` the server is read-only and serves skills: `find_skill`, `load_skill`,
`list_skill_resources` and the `skill://` resources. Flags of that mode: `--profile`, `--targets`, `--domain`,
`--allow`, `--deny`, `--source` (repeatable), `--role`, `--frozen`, `--offline`, `--include-static`,
`--budget-bytes`, `--max-clone-bytes` (overrides `AI_RULEZ_MAX_CLONE_BYTES`), `--usage-log`, `--usage-sink`,
`--no-watch`, `--reload-interval`. `--role` names a role of
`[[roles]]`: the server serves only that role's skills, with the role's [delivery](roles.md#delivery).

See the [MCP Server Documentation](mcp-server.md) and [Dynamic skill loading](mcp-server.md#dynamic-skill-loading)
for more details.

## Global Flags

These flags work with all commands:

| Flag               | Type    | Description                                                                     |
| ------------------ | ------- | ------------------------------------------------------------------------------- |
| `--config` / `-C`  | string  | Config file path (auto-discovered if not specified)                             |
| `--token` / `-T`   | string  | Git access token for private repositories (or use `AI_RULEZ_GIT_TOKEN` env var) |
| `--policy`         | string  | [Organization policy](policy.md) file (tighten-only); also `AI_RULEZ_POLICY` and the managed path |
| `--policy-digest`  | string  | The digest (`sha256:<hex>`) the `--policy` file or URL must have; a URL policy is never loaded without one (`AR741`) |
| `--policy-offline` | boolean | Load a URL policy from the user cache only (also `AI_RULEZ_POLICY_OFFLINE=1`) |
| `--policy-max-stale` | string | How long a cached URL policy may stand in for an unreachable URL (`7d` default, `0` for none; also `AI_RULEZ_POLICY_MAX_STALE`) |
| `--discover-org`   | boolean | Also load the organization policy of the repository's GitHub owner (`ai-rulez-policy.toml` in `<owner>/.github`); needs a digest (`[policy.digests]` in the user config, or `--policy-trust-tofu`). Also `[policy] discover = "org"` in the user config |
| `--policy-require-signed` | boolean | Refuse a [policy](policy.md) with no valid signature (`<policy>.sigstore.json` next to it, `AR746`); needs a trusted signer. Also `AI_RULEZ_POLICY_REQUIRE_SIGNED=1` |
| `--policy-signer-key` | string list | PEM public key trusted to sign the policy (repeatable; also `AI_RULEZ_POLICY_SIGNER_KEY`) |
| `--policy-signer-identity`, `--policy-signer-issuer` | string | A certificate identity and its OIDC issuer trusted to sign the policy (keyless) |
| `--policy-trusted-root` | string | Sigstore trusted root file for keyless policy signatures |
| `--policy-trust-tofu` | boolean | Record the digest of an unpinned `--policy` URL once, in a terminal only |
| `--policy-mode`    | string  | `enforce` (default) or `warn`: with `warn` a repository that loosens the [policy](policy.md) is reported as warnings and the run does not fail; the policy values are still enforced |
| `--verbose` / `-V` | boolean | Enable verbose output                                                           |
| `--debug` / `-D`   | boolean | Enable debug output                                                             |
| `--quiet` / `-q`   | boolean | Suppress progress bars and non-essential output                                 |
| `--help` / `-h`    | boolean | Show help for a command                                                         |
| `--version` / `-v` | boolean | Print `ai-rulez version <version>` (root command only; same as `ai-rulez version`) |

Every command that can print JSON takes `--format text|json` (some add `sarif`, `junit`, `markdown` and more; an unknown value is rejected with the allowed list). `--json` is accepted wherever `--format json` exists; it is hidden from help and warns that it is deprecated. Log colors are off when `NO_COLOR` is set, when `TERM=dumb`, or when stderr is not a terminal. Most command-local flags also have shorthands. Common mappings are `--domain -d`, `--force -f`,
`--priority -p`, `--targets -t`, `--content -c`, `--description -s`, `--path -p`,
and `--ref -r`.

**Examples:**

Generate with debug output:

```bash
ai-rulez generate --debug
```

Quiet mode (minimal output):

```bash
ai-rulez generate --quiet
```

Show help for init:

```bash
ai-rulez init --help
```

## Configuration Detection

Commands that load a project directory use the following config order:

1. **Explicit path**: Via `--config` flag or command argument
2. **Directory config**: `.ai-rulez/config.toml`
3. **Project convention**: `.config/ai-rulez/config.toml`, used only when discovering the default layout (an explicit `--config-dir` is honoured exactly and never falls back)
4. **Error**: No configuration found

The search walks up from the current directory. `config.toml` is the only config format read. A project that has only a YAML or JSON config (`.ai-rulez/config.yaml`, `.yml` or `.json`, or a `config.local.yaml`, `.yml` or `.json` overlay), or a flat V2 `ai-rulez.yaml` (also `.ai-rulez.yaml`, `ai_rulez.yaml` and their `.yml` forms), stops with an error that names the file and exits `1`; run `ai-rulez migrate v5` to convert it (a 2.x or 3.x project goes through ai-rulez 4.x first). A `config.toml` whose `version` is `4.0` stops the same way. `doctor` reports the same file.

Example detection flow:

```bash
cd /path/to/project

# Detects .ai-rulez/ if it exists
ai-rulez generate

# Use explicit path
ai-rulez generate .ai-rulez/config.toml

# Specify config via flag
ai-rulez generate --config ./ai-policy/config.toml
```

## Exit Codes

Every command follows one contract (`lock` adds `3`, see [Lock file](lockfile.md)), so a script can tell a failed run from a failed check:

| Code | Meaning |
| ---- | ------- |
| 0    | Success |
| 1    | The command could not run: configuration not found or invalid (`validate` included), bad flags, an unknown subcommand, a V2/V3 config file (see [Configuration Detection](#configuration-detection)), a tool or network error, `lock --check` with no `ai-rulez.lock`, `verify` with no manifest |
| 2    | The command ran and found something: `validate --strict` and `scan` findings at or above `--fail-on`; drift from `generate --check`, `verify`, `export okf --check`, `lock --check` (also `--locked`/`--frozen` source drift); `lock --strict` refusing a served skill the security scan refuses; `lock --outdated` with a moved tag, a deleted tag or an unsatisfiable constraint (and any update with `--fail-on-outdated`); `update` refusing a source; `doctor` errors (warnings with `--strict`); `migrate v5 --check` finding a project to migrate; `verifiers run` or `verifiers test` failures; `eval run` below its threshold, erroring or with invalid cases; `tokens --budget` and `cost --budget` exceeded; `convert` blocked by the scan or `--fail-on`; `okf validate` findings and `import okf` refused or not overwriting; `search --eval` gate failed; `scanners doctor` finding a bad scanner; `guard` blocking an edit to a generated file |
| 3    | `lock` only: the lock was written, but served skills were left unpinned because the security scan refuses them (`lock --strict` exits 2 instead) |

When a command covers several roots (`--recursive`), the most severe code wins: `1`, then `2`, then `3`.

The contract is covered by a table test over the built binary (`tests/e2e/cli/exit_codes_test.go`), so a
CI step can rely on it: `0` pass, `2` fix the content, `1` fix the setup.

## Output Examples

### Initialize Output

```text
✅ Created .ai-rulez/ directory structure

Directory structure:
  .ai-rulez/
  ├── config.toml
  ├── rules/         # Base rules (always included)
  ├── context/       # Base context (always included)
  ├── skills/        # Base skills (always included)
  ├── agents/        # Base agents (always included)
  └── domains/       # Domain-specific content

Example content created:
  - rules/code-quality.md
  - context/architecture.md
  - skills/code-reviewer/SKILL.md
  - skills/ai-rulez/SKILL.md

Domain directories created:
  - domains/backend/
  - domains/frontend/
  - domains/qa/

Next steps:
  1. Edit .ai-rulez/config.toml to customize presets and profiles
  2. Add your rules, context, and skills to the appropriate directories
  3. Run 'ai-rulez generate' to create tool-specific outputs
```

### Generate Output

```text
✅ Generated 3 file(s) successfully
  - CLAUDE.md
  - .cursor/rules/example.mdc
  - docs/AI_GUIDE.md
```

### Validation Output

```text
INFO  Configuration is valid path=/path/to/project/.ai-rulez
INFO
Configuration summary:
INFO   - Rules: count=5
INFO   - Context files: count=10
INFO   - Skills: count=8
INFO   - Domains: count=9
INFO     - Domain rules: count=35
INFO     - Domain context: count=2
INFO   - Presets: count=12
```

### Validation Error

```text
❌ Configuration validation failed

errors:
  - Profile "backend" references undefined domain "backend"
  - Preset "claude" not found

Run 'ai-rulez validate --verbose' for details
```

## Common CRUD Workflows

### Creating a Domain with Rules

```bash
# Create a domain
ai-rulez domain add backend --description "Backend services"

# Add rules to the domain
ai-rulez add rule database-standards --domain backend --priority high
ai-rulez add rule api-design --domain backend --priority high

# Add context
ai-rulez add context architecture --domain backend

# Validate and generate
ai-rulez validate
ai-rulez generate
```

### Setting Up Team-Based Profiles

```bash
# Create domains for each team
ai-rulez domain add backend
ai-rulez domain add frontend
ai-rulez domain add qa

# Create team-specific profiles
ai-rulez profile add full backend frontend qa --set-default
ai-rulez profile add backend backend qa
ai-rulez profile add frontend frontend qa

# Generate for a specific profile
ai-rulez generate --profile backend
```

### Adding External Rules from Git

```bash
# Add corporate rules include
ai-rulez include add corporate-rules https://github.com/myorg/shared-rules --ref main

# Verify include was added
ai-rulez include list

# Validate with included content
ai-rulez validate

# Regenerate with included content
ai-rulez generate
```

### Managing Content Across Domains

```bash
# Add shared rule to root (always included)
ai-rulez add rule code-style --priority high

# Add domain-specific rule
ai-rulez add rule database-standards --domain backend --priority high
ai-rulez add rule accessibility --domain frontend --priority high

# List all rules
ai-rulez list rules

# List domain-specific rules
ai-rulez list rules --domain backend
ai-rulez list rules --domain frontend

# Remove a rule
ai-rulez remove rule old-guideline --yes
```

---

## Common Workflows

### Set Up a New Project

```bash
# Initialize with domains
ai-rulez init "my-project" --domains "backend,frontend,qa"

# Review generated structure
ls -la .ai-rulez/

# Create/edit content files
# (edit .ai-rulez/rules/, .ai-rulez/context/, etc.)

# Validate configuration
ai-rulez validate

# Generate outputs
ai-rulez generate
```

### Generate Multiple Profiles

```bash
# Generate full profile
ai-rulez generate --profile full

# Generate backend-only profile
ai-rulez generate --profile backend

# Generate frontend-only profile
ai-rulez generate --profile frontend
```

### Update AI Configuration After Changes

```bash
# Edit your content
vim .ai-rulez/rules/my-rule.md

# Validate it's still correct
ai-rulez validate

# Regenerate outputs
ai-rulez generate

# Commit changes
git add .ai-rulez/   # plus the generated files unless gitignore = true
git commit -m "docs: update AI assistant guidelines"
```

### CI/CD Integration

```bash
#!/bin/bash
# Simple CI/CD script

# Validate configuration and content (--no-local: ignore any machine-local overlay).
# Exit 2 means findings, exit 1 means the setup is broken.
ai-rulez validate --no-local || exit 1

# Fail when the committed generated files differ from the sources (exit 2 on drift)
ai-rulez generate --check --no-local || {
  echo "Generated files are out of sync"
  echo "Run: ai-rulez generate"
  exit 1
}
```

## Troubleshooting

### Command not found

```bash
# Make sure AI-Rulez is installed
which ai-rulez

# Or check version
ai-rulez version
```

### Configuration not found

```bash
# Check current directory
ls -la .ai-rulez/
ls -la .ai-rulez/config.toml

# Or specify explicitly
ai-rulez generate --config /path/to/.ai-rulez/config.toml
```

### Invalid profile name

```bash
# List available profiles
ai-rulez validate --verbose

# Use a valid profile name
ai-rulez generate --profile backend
```

### Generated files not updated

```bash
# Check if files were actually generated
ai-rulez generate --dry-run

# Force regeneration
rm CLAUDE.md .cursor/rules/*
ai-rulez generate
```

## Help and Documentation

Get help for any command:

```bash
ai-rulez --help
ai-rulez init --help
ai-rulez generate --help
ai-rulez validate --help
```

For more detailed documentation:

- **[Configuration Reference](configuration.md)**: Config options
- **[Quick Start](quick-start.md)**: Getting started
- **[Domains & Profiles](domains.md)**: Team organization
