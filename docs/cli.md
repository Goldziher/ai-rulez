# CLI Reference

All AI-Rulez CLI commands and flags.

## Command Overview

### Core Commands

| Command                         | Description                                         |
| ------------------------------- | --------------------------------------------------- |
| `ai-rulez init`                 | Initialize V4 directory-based configuration         |
| `ai-rulez generate`             | Generate presets for specific profile               |
| `ai-rulez clean`                | Remove files produced by `generate`                 |
| `ai-rulez validate`             | Validate configuration                              |
| `ai-rulez verify`               | Verify generated files against their hashes (`--plugin` for plugin bundles) |
| `ai-rulez lock`                 | Pin remote includes, installed skills and authored content in `ai-rulez.lock` ([Lock file](lockfile.md)) |
| `ai-rulez roles`                | List, show and resolve `[[roles]]` ([Roles](roles.md)) |
| `ai-rulez catalog`              | Items with owner, version, tokens, roles and lock status (`--format json`) |
| `ai-rulez doctor`               | Read-only diagnostics for the project's setup ([details](#doctor-command)) |
| `ai-rulez scan`                 | Security checks on skills, rules and scripts         |
| `ai-rulez migrate`              | Migrate configuration versions (migrate v4 command) |
| `ai-rulez tokens`               | Report the prompt-token cost of generated artifacts |
| `ai-rulez version`              | Show version                                        |
| `ai-rulez mcp`                  | Start MCP server                                    |
| `ai-rulez local`                | Manage the machine-local config overlay ([details](#local-configuration)) |
| `ai-rulez builtins list`        | List available built-in domains                     |
| `ai-rulez builtins show <name>` | Show bundled content for a built-in domain          |

Aliases: `generate` → `gen`, `g`; `clean` → `clear`; `validate` → `val`, `v`, `check`.

### CRUD Commands (Configuration Management)

| Command                                           | Description              |
| ------------------------------------------------- | ------------------------ |
| `ai-rulez domain add/remove/list`                 | Manage domains           |
| `ai-rulez add rule/context/skill/agent/command`   | Create content files     |
| `ai-rulez remove rule/context/skill/agent/command` | Delete content files    |
| `ai-rulez list rules/context/skills/agents/commands` | List content files    |
| `ai-rulez include add/remove/list`                | Manage external includes |
| `ai-rulez skill install/remove/list`              | Manage installed skills  |
| `ai-rulez profile add/remove/list`                | Manage profiles          |
| `ai-rulez profile set-default`                    | Set default profile      |

`add`, `remove` and `list` take `--local` to work on the machine-local tree `.ai-rulez/local/`;
`include add|remove`, `skill install|remove` and `profile add|remove|set-default` take `--local` to
write to the `config.local.*` overlay. See [Local Configuration](local-overrides.md).

## CRUD Commands

AI-Rulez provides CRUD commands to programmatically modify your V4 `.ai-rulez/` configuration. These commands allow you to create domains, add rules/context/skills/agents/commands, manage includes, and organize profiles.

`add agent` and `add command` take `--domain`/`-d`, `--description`/`-s`, `--content`/`-c` and
`--local`. `remove agent|command` and `list agents|commands` take the same flags as their rule/skill
counterparts.

`add check`, `remove check` and `list checks` manage code-review guidelines (see [Checks](checks.md));
they take `--domain`/`-d` (and `--description`/`-s`, `--content`/`-c` for `add`) but have no `--local`.

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

- `--force` / `-f` (optional): Skip confirmation prompt

**Examples:**

Remove a domain (with confirmation):

```bash
ai-rulez domain remove backend
```

Remove without confirmation:

```bash
ai-rulez domain remove backend --force
```

#### `ai-rulez domain list [flags]`

List all domains in the `.ai-rulez/` directory.

**Syntax:**

```bash
ai-rulez domain list [flags]
```

**Flags:**

- `--json` / `-j` (optional): Output as JSON

**Examples:**

List domains:

```bash
ai-rulez domain list
```

List as JSON:

```bash
ai-rulez domain list --json
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
- `--force` / `-f` (optional): Skip confirmation
- `--local` (optional): Remove from the machine-local tree `.ai-rulez/local/`

**Examples:**

Remove a root rule:

```bash
ai-rulez remove rule code-quality
```

Remove a domain rule:

```bash
ai-rulez remove rule database-standards --domain backend --force
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
- `--force` / `-f` (optional): Skip confirmation
- `--local` (optional): Remove from the machine-local tree `.ai-rulez/local/`

**Examples:**

```bash
ai-rulez remove context architecture
ai-rulez remove context backend-design --domain backend --force
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
- `--force` / `-f` (optional): Skip confirmation
- `--local` (optional): Remove from the machine-local tree `.ai-rulez/local/`

**Examples:**

```bash
ai-rulez remove skill code-reviewer
ai-rulez remove skill performance-optimizer --domain backend --force
```

#### `ai-rulez list rules [flags]`

List all rule files.

**Syntax:**

```bash
ai-rulez list rules [flags]
```

**Flags:**

- `--domain <name>` / `-d` (optional): List rules in specific domain only
- `--json` / `-j` (optional): Output as JSON
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
- `--json` / `-j` (optional): Output as JSON
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
- `--json` / `-j` (optional): Output as JSON
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

- `--force` / `-f` (optional): Skip confirmation prompt
- `--local` (optional): Remove through the `config.local.*` overlay. A skill installed in the shared config is hidden on this machine with `remove = true`

**Examples:**

```bash
ai-rulez skill remove kreuzberg
ai-rulez skill remove my-lib --force
```

#### `ai-rulez skill list [flags]`

List all installed skills.

**Flags:**

- `--json` / `-j` (optional): Output as JSON

**Examples:**

```bash
ai-rulez skill list
ai-rulez skill list --json
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

- `--force` / `-f` (optional): Skip confirmation
- `--local` (optional): Remove through the `config.local.*` overlay. An include defined in the shared config is hidden on this machine with `remove = true`

**Examples:**

```bash
ai-rulez include remove corporate-rules
ai-rulez include remove shared-patterns --force
```

#### `ai-rulez include list [flags]`

List all include sources.

**Syntax:**

```bash
ai-rulez include list [flags]
```

**Flags:**

- `--json` / `-j` (optional): Output as JSON

**Examples:**

```bash
ai-rulez include list
ai-rulez include list --json
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

- `--force` / `-f` (optional): Skip confirmation
- `--local` (optional): Remove a profile defined in the overlay. A profile from the shared config cannot be removed locally (error)

**Examples:**

```bash
ai-rulez profile remove staging
ai-rulez profile remove development --force
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

- `--json` / `-j` (optional): Output as JSON

**Examples:**

```bash
ai-rulez profile list
ai-rulez profile list --json
```

---

## Local Configuration

Machine-local configuration has two layers: the `.ai-rulez/local/` content tree (managed with `--local`
on `add`, `remove` and `list`) and the `config.local.{toml,yaml,yml,json}` overlay (managed with
`ai-rulez local` and `--local` on the config commands). The full guide is
[Local Configuration](local-overrides.md); this section is the command reference.

### `ai-rulez local <subcommand>`

Manage `config.local.{toml,yaml,yml,json}`, the machine-local overlay merged onto the shared config at load time. It sits beside `config.toml`, is gitignored, and is never written into the shared file.

```bash
ai-rulez local init                     # skeleton in the main config's format (commented for TOML and YAML, {} for JSON)
ai-rulez local show [--json] [--reveal] # keys the overlay sets, with the shared value each replaces
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

Shared outputs keep the baseline `Source-Hash`, so your headers match a teammate's; local-only outputs carry a hash of their local inputs. `generate --dry-run` prints `local-only:`, `drift:`, `allowed:` (a drift file that is git-ignored), `suppressed:` and `blocked:` lines, and exits non-zero when any line is `blocked:` (the real run would refuse). A run with `--no-local` (or `generate --plugin`, which never uses local config) does not delete your local files.

## Builtins Command

### `ai-rulez builtins list [flags]`

List all built-in domains embedded in the `ai-rulez` binary.

**Flags:**

- `--json` / `-j` (optional): Output as JSON

### `ai-rulez builtins show <name> [flags]`

Show the full rules, context, skills, agents, and commands for a built-in domain.

**Arguments:**

- `<name>` (required): Built-in domain name, such as `security`, `go`, or `typescript`

**Flags:**

- `--json` / `-j` (optional): Output as JSON

---

## Initialization Command

### `ai-rulez init [project-name]`

Initialize a new V4 directory-based configuration.

**Syntax:**

```bash
ai-rulez init [project-name] [flags]
```

**Arguments:**

- `[project-name]` (optional): The project name. If omitted, the current directory's base name is used (falling back to `MyProject`). Nothing is prompted.

**V4-specific Flags:**

| Flag                    | Type    | Default | Description                                                          |
| ----------------------- | ------- | ------- | -------------------------------------------------------------------- |
| `--format` / `-f`       | string  | `toml`  | Configuration format: `toml`, `yaml`, or `json`                      |
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

**General Flags:**

| Flag        | Type    | Description           |
| ----------- | ------- | --------------------- |
| `--verbose` | boolean | Enable verbose output |
| `--debug`   | boolean | Enable debug output   |

**Examples:**

Basic V4 initialization (TOML format):

```bash
ai-rulez init "my-project"
```

V4 with YAML format:

```bash
ai-rulez init "my-project" --format yaml
```

V4 with multiple domains:

```bash
ai-rulez init "my-project" --domains "backend,frontend,qa"
```

V4 with example content skipped:

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
| `--gitignore` / `-i`            | boolean | (from config) | Update `.gitignore` with generated output patterns git does not already ignore (a rule or `!` override of yours wins)                                                                                                      |
| `--recursive` / `-r`            | boolean | false         | Find and process configs recursively; exits non-zero if any root fails (the others are still processed)                                                 |
| `--no-fetch` / `-f`             | boolean | false         | Skip fetching remote includes and use cached content                                                                                                    |
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
| `--yes` / `-y`                  | boolean | false         | With `--user`, write without the confirmation prompt (required in a non-interactive shell)                                                              |

`--token` / `-T` is a global flag (see [Global Flags](#global-flags)); it is not generate-specific.
`--update-gitignore` still works as a hidden deprecated alias for `--gitignore` for backward compatibility.
`--no-configure-cli-mcp` / `-M` and `--skip-cli-mcp` / `-S` are hidden deprecated no-ops kept so existing scripts keep working: `generate` only writes MCP config files and never configures CLI tools, so there is nothing to skip.

`--dry-run` lists each file as `write-file:` (it would be written), `unchanged:` (already current) or `edited:` (changed by hand; `generate` leaves it alone until its sources change).

#### Detecting drift

Teams that commit generated files can gate CI on `generate --check`. It renders in memory, never writes or deletes, and prints one line per file that differs:

| Line | Meaning |
| --- | --- |
| `missing: <path>` | A generated file is not on disk |
| `stale: <path>` | The sources changed (or the rendering did); `generate` would rewrite it |
| `edited: <path>` | The body no longer matches the `Content-Hash` in its own header: a hand edit |
| `orphan: <path>` | Listed in the previous manifest, no longer rendered; `generate` would delete it |

Exit codes: `0` nothing differs, `1` the check could not run (configuration invalid, a nested root failed to load), `2` at least one file differs. A trailing-newline-only difference is not reported, because `generate` normalizes it. With `[header] hashes = "none"` there is no hash to compare, so every difference is `stale`. `--check` cannot be combined with `--dry-run`, `--plugin` (use `verify --plugin`) or `--gitignore`.

#### Watch mode

`generate --watch` (`-w`) generates once, then keeps running and regenerates whenever the sources change. It watches `.ai-rulez/` recursively (subdirectories created later included), the machine-local overlay files, the config file, and every `includes` / `local_override` source that is a local path. Remote includes are not polled.

- Changes are debounced for 300 ms, so a save storm or `git checkout` produces one run.
- Runs never overlap. A change that arrives during a run triggers exactly one more run afterwards.
- Generated output, the generated manifests and editor swap/backup files (`*.swp`, `*~`, `.#*`, `4913`, ...) never trigger a run.
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

Generate and update .gitignore:

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

### `ai-rulez clean [config-path]`

Remove the files and directories that `generate` produced — the inverse of `generate`. This deletes the generated assistant outputs (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `.claude/`, `.codex/`, generated skills, etc.), the generated manifest (`.ai-rulez/.generated-manifest.json`), the local manifest (`.ai-rulez/.generated-manifest.local.json`) and the local rule files recorded in it, this project's block in `.git/info/exclude`, and the ai-rulez managed block in `.gitignore`. The ignore entries for the `config.local.*` overlay and `.ai-rulez/local/` stay in `.gitignore`: `clean` removes generated outputs, not the machine-local sources beside them.

The `.ai-rulez/` source tree is never touched. Generated directories are only removed once they are empty, so any files you authored inside a generated directory are preserved.

Settings documents you share with ai-rulez (`.claude/settings.json`, `.gemini/settings.json`, `opencode.json`, `.mcp.json`, ...) are not deleted: `clean` removes only the keys, MCP server entries and array elements ai-rulez merged into them (including a stale entry that holds a resolved secret), and deletes the file only when nothing else is left. The plan lists them as `remove ai-rulez keys from:`. See [Settings documents shared with you](local-overrides.md#settings-documents-shared-with-you).

By default `clean` lists what it will remove and asks for confirmation. In non-interactive shells the prompt declines automatically — pass `--force` there.

**Syntax:**

```bash
ai-rulez clean [config-path] [flags]
```

**Flags:**

| Flag                  | Type    | Default            | Description                                             |
| --------------------- | ------- | ------------------ | ------------------------------------------------------- |
| `--dry-run` / `-d`    | boolean | false              | Show what would be removed without deleting anything    |
| `--force` / `-y`      | boolean | false              | Skip the confirmation prompt                            |
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
ai-rulez clean --force
```

Remove generated files but keep the `.gitignore` block and manifest:

```bash
ai-rulez clean --force --keep-gitignore --keep-manifest
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

Verify one producer:

```bash
ai-rulez verify --plugin
```

Verify every plugin producer and marketplace in a repository:

```bash
ai-rulez verify --recursive --plugin --if-configured
```

When a `[plugin]` block exists but no bundle was generated, `verify --plugin` fails with `plugin bundle not generated; run `ai-rulez generate --plugin``. `--if-configured` only skips a project with no plugin configuration; add `--if-generated` to also skip until the bundle exists.

Recursive verification treats a marketplace root and its members as one atomic
producer. Consumer-only plugin installation declarations are skipped. Missing
authoring configuration is ignored only with `--if-configured`; stale, missing,
or invalid generated outputs still fail verification.

## Doctor Command

### `ai-rulez doctor [config-file]`

Read-only diagnostics. It never writes, and reports each problem as an `error`, a `warning` or `info`.

```bash
ai-rulez doctor [config-file] [--strict] [--json] [--profile <name>] [--no-local] [--config-dir <name>]
```

| Check | Reports | Severity |
| --- | --- | --- |
| `config` | The configuration fails to load, match the schema or validate (same as `validate`) | error |
| `presets` | An unknown or removed preset name, with a suggestion: `windsurf` is now `devin`, `continue-dev` has no replacement, a typo gets a "did you mean" | error |
| `mcp-env` | An MCP `${VAR}` placeholder in `env` or `headers` that no environment variable, `.env` file or `--env` value resolves (`${PROJECT_ROOT}` always resolves). Only servers active in the selected profile are checked | warning |
| `drift` | Generated files that are `missing`, `stale`, `edited` or `orphan` (the machinery behind [`generate --check`](#detecting-drift)) | warning |
| `gitignore` | Generated paths `generate` wants git to ignore (committed outputs with `gitignore = true`, machine-local and secret outputs always) that git does not ignore; asked through `git check-ignore`, skipped outside a git repository | warning |
| `documents` | A shared settings document ai-rulez merges into (`.claude/settings.json`, `.mcp.json`, `.codex/config.toml`, ...) that no longer parses as JSON, JSONC, TOML or YAML | error |
| `hooks` | A `[[hooks]]` script that does not exist or is not executable. The script paths are checked directly, so the result does not depend on git state or on `[lint]` overrides of `AR504` and `AR505` | error |
| `lock` | `ai-rulez.lock` does not match the remote includes and installed skills (checked offline, as in `lock --check`) | warning |
| `tools` | The binary behind a preset (`claude`, `codex`, `gemini`, ...) is not on `PATH`; presets whose binary is not known are skipped | info |

When outputs cannot be rendered at all (for example because an MCP placeholder is unset), `drift` says so and the `gitignore` check is skipped; the `mcp-env` finding names the cause.

`doctor` never uses the network and never writes the include cache: it loads the configuration without resolving includes and installed skills. A project that declares them gets one `info` finding on `drift` saying the comparison was skipped (`generate --check` does it against the resolved content), and the `gitignore` check is skipped with it.

| Flag | Description |
| --- | --- |
| `--strict` | Also exit non-zero on warnings |
| `--json` | Print `{"root", "summary": {"error", "warning", "info"}, "findings": [{"check", "severity", "message", "path", "hint"}]}` instead of the table |
| `--profile` / `-p` | Profile rendered for the `drift` and `gitignore` checks |
| `--no-local` | Ignore the machine-local overlay and `local/` content |
| `--config-dir` / `-n` | Configuration directory name for non-default layouts |

Exit codes: `0` no errors (and, with `--strict`, no warnings), `2` at least one finding at the failing severity, `1` doctor could not run: the configuration does not load at all (the `config` finding is still printed) or the report could not be written. The MCP server exposes the same checks as the read-only `doctor` tool, with URL credentials removed from every message, hint and path.

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
| `--json` / `-j`       | boolean | false              | Emit the report as JSON                                          |
| `--budget` / `-b`     | int     | 0                  | Exit 2 when the headline always-loaded count exceeds this ceiling |
| `--compare-profiles`  | strings | none               | One profile per column of a comparison table; repeat the flag per column |
| `--tokenizer`         | string  | `cl100k_base`      | `cl100k_base` (offline BPE) or `estimate` (byte ratio)           |
| `--no-local`          | boolean | false              | Ignore the machine-local overlay and `local/` content            |
| `--profile` / `-p`    | string  | configured default | Profile to report on; a comma-separated list composes several    |
| `--role`              | string  |                    | Report on a [role](roles.md)'s slice instead of a profile        |
| `--by-role`           | boolean | false              | One comparison column per declared role                          |
| `--config-dir` / `-n` | string  | `.ai-rulez`        | Configuration directory name for non-default layouts             |

```bash
ai-rulez tokens
ai-rulez tokens --json
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
| `amp`, `antigravity`, `baz`, `continue-dev`, `hermes`, `xum` | not modeled | no listing is reported |

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

## Usage Commands

Opt-in usage telemetry, documented in [Usage telemetry](usage-telemetry.md).

| Command | Purpose |
| --- | --- |
| `ai-rulez usage hook [-o file] [--harness claude\|codex\|cursor] [--role r] [--log f] [--sink-command c] [--index f] [--executable e]` | Print (or write) the hooks block that records skill invocations; other harnesses warn and print nothing |
| `ai-rulez usage record [--harness h] [--outcome o] [--role r] [--served] [--salt-file f] [--log f] [--sink-command c] [--index f]` | Read one hook event on stdin and append an identifier-only JSON line; always exits 0 |
| `ai-rulez usage feedback <skill> --kind misled\|stale\|wrong\|great [--note-file f] [--log f] [--harness h] [--role r]` | Append an identifier-only feedback record; the note text stays in `feedback-notes/` |
| `ai-rulez report usage <log> [--index f] [--feedback f] [--evals f] [--json] [-n dir]` | Join a usage log with `skills-index.json`, feedback and eval scores: used, never used, changed since used, unknown |
| `ai-rulez report evals [--usage-log f] [--feedback f] [--results f] [--min-pass-rate r] [--min-trigger r] [--json] [-n dir]` | Rank skills to rewrite, prune, review or keep from eval scores joined with usage and feedback |

## Eval Commands

Documented in [Evals](evals.md).

| Command | Purpose |
| --- | --- |
| `ai-rulez eval run [skill...] [--harness h] [--runner claude-plugin-eval\|command] [--runner-command c] [--ablation] [--dry-run] [--format json\|markdown\|junit] [--out dir] [--max-cost usd] [--changed-only] [--base ref] [--date d] [--force] [--threshold r] [--allow-exec] [--model m] [--runs n]` | Run eval cases through a runner, score each skill and record `.ai-rulez/eval-results.json`. Exit 2 when a skill fails its threshold, errors, or has invalid cases |

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
| `--strict`            | boolean | Also run deep content checks (dead globs, links, references, hooks, size); exits 2 on findings. See [Strict validation](strict-validation.md) |
| `--format`            | string  | With `--strict`: `text` (default) or `json` |
| `--fail-on`           | string  | With `--strict`: lowest severity that exits 2 (`error` default, `warning`, `info`, `none`) |
| `--external`          | boolean | With `--strict`: also run the `[[lint.external]]` scanners and merge their findings |
| `--verbose`           | boolean | Enable verbose output                                |
| `--debug`             | boolean | Enable debug output                                  |

**Examples:**

Run the deep content checks, as JSON, across every root:

```bash
ai-rulez validate --strict --recursive --format json
```

Validate every config in a monorepo (all roots are checked; exit status 1 if any fails):

```bash
ai-rulez validate --recursive
```

Validate current configuration:

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

For V4 configs the raw file is also checked against `schema/ai-rules.schema.json`, so an unknown key
or a value outside an enum fails rather than being silently dropped. The structural checks are:

- A `config.local.*` overlay, when present, is checked against `schema/ai-rules-local.schema.json`, and the merged config is validated. The output names the overlay file and prints a one-line summary of overridden, added and removed key paths, never values
- `version` is `"3.0"` or `"4.0"`
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
- a digest of each generated output, and one `tree` digest over everything.

Local-path sources live in the repository and are not locked as remotes. Credentials in a source URL are redacted.

```bash
ai-rulez lock                     # pin everything (uses the network for remotes)
ai-rulez lock shared              # re-pin one include or skill, keep the other pins (and the content pins)
ai-rulez skill update kreuzberg   # same, for installed skills only
ai-rulez lock --content-only      # re-pin authored content and outputs; offline
ai-rulez lock --check             # verify everything, offline; exit 2 and name each difference
ai-rulez lock --diff              # what `lock` would change, for a pull request
ai-rulez lock --diff --format json
```

With a lock present, `generate` fetches the **locked commit** instead of the moving ref, so two runs produce
identical output even after the remote moved, and verifies the digest of what it fetched. A mismatch fails the run
(a damaged cache is repaired by fetching the pinned commit again first; a remote that serves different bytes for
the same commit is a hard failure). A source the lock does not cover is fetched as before, with the advice to run
`ai-rulez lock`. `generate` never writes the lock.

| Flag | Description |
| --- | --- |
| `--check` | Verify the lock against the configuration, the sources, the rendered outputs and any cached remote content; exit 2 naming each added, removed or changed item and whether its source or its output changed |
| `--diff` | Print how the sources and outputs differ from the lock; exits 0. `--format json` follows `schema/lock-diff.schema.json` |
| `--content-only` | Re-pin authored content and outputs only: no network, remote pins kept |
| `--format text\|json` | Output format of `--diff` |
| `--profile <name>` | Profile whose outputs are pinned (default: the profile recorded in the lock, else the configured default) |
| `--kind include\|skill` | Limit a refresh to one kind |
| `--recursive` / `-r` | Process every nested root |

CI: `generate --locked` fails when the lock is missing or does not cover a configured remote source, or when an
authored source no longer matches the lock's content pins (exit 2); `generate --frozen` additionally never touches
the network. `validate` logs a warning for each remote source that follows a moving ref without a pin, and
`validate --strict` reports it as `AR010` (raise it to an error with `[lint.severity]`). With `[lock] enforce = true`
it also reports content drift as `AR981` / `AR982`. Pinning `ref` to a full commit SHA also counts as pinned.

Signature or attestation verification is not implemented: the lock proves the bytes did not change since you
reviewed them, not who published them.

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
written. `--format json` is versioned (`schema/catalog.schema.json`) and is meant for a UI or an audit script; see
[Integrating an identity tool or UI](roles.md#integrating-an-identity-tool-or-ui).

## Scan Command

### `ai-rulez scan [config-path]`

Security checks only, the `AR0xx` family of [strict validation](strict-validation.md#security-checks): secrets, hidden characters, prompt-injection phrases, risky shell, unrestricted `allowed-tools`, outbound hosts, unpinned remotes. Offline and deterministic. Flags: `--recursive`, `--format text|json`, `--fail-on`, `--external`, `--no-local`, `--config-dir`. Exit `0` clean, `1` cannot run, `2` findings at or above `--fail-on`.

## Migrate Command

### `ai-rulez migrate v4`

Migrate configuration from V3 (YAML) to V4 (TOML format).

**Syntax:**

```bash
ai-rulez migrate v4
```

**Arguments:**

- `v4`, `4`, or `4.0` (required): Target configuration version.

The migrate command has no command-local flags.

**Examples:**

Migrate current directory:

```bash
ai-rulez migrate v4
```

**What It Does:**

1. Finds `.ai-rulez/` in the current directory
2. Converts a `config.local.yaml`, `.yml` or `.json` overlay to `config.local.toml` (owner-only, `$schema` becomes `schema`), whether or not `config.toml` already exists. If more than one `config.local.*` file exists the overlay is left alone with a warning
3. Returns without further changes if `.ai-rulez/config.toml` already exists
4. Loads the existing shared configuration (without the overlay), including legacy MCP files if present
5. Writes `.ai-rulez/config.toml` with `version = "4.0"`
6. Removes old `.ai-rulez/config.yaml`, `.ai-rulez/config.json`, `.ai-rulez/mcp.yaml`, `.ai-rulez/mcp.toml`, and `.ai-rulez/mcp.json` files

**After Migration:**

- `.ai-rulez/config.toml` — new V4 configuration (TOML)
- `.ai-rulez/config.local.toml` — the converted overlay, if one existed
- All other files remain unchanged

Then regenerate outputs:

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
ai-rulez mcp --serve-skills [--profile <p>] [--source <src>] [--role <r>] [--frozen]
```

With `--serve-skills` the server is read-only and serves skills: `find_skill`, `load_skill`,
`list_skill_resources` and the `skill://` resources. Flags of that mode: `--profile`, `--targets`, `--domain`,
`--allow`, `--deny`, `--source` (repeatable), `--role`, `--frozen`, `--offline`, `--include-static`,
`--budget-bytes`, `--usage-log`, `--usage-sink`, `--no-watch`, `--reload-interval`.

See the [MCP Server Documentation](mcp-server.md) and [Dynamic skill loading](mcp-server.md#dynamic-skill-loading)
for more details.

## Global Flags

These flags work with all commands:

| Flag               | Type    | Description                                                                     |
| ------------------ | ------- | ------------------------------------------------------------------------------- |
| `--config` / `-C`  | string  | Config file path (auto-discovered if not specified)                             |
| `--token` / `-T`   | string  | Git access token for private repositories (or use `AI_RULEZ_GIT_TOKEN` env var) |
| `--verbose` / `-V` | boolean | Enable verbose output                                                           |
| `--debug` / `-D`   | boolean | Enable debug output                                                             |
| `--quiet` / `-q`   | boolean | Suppress progress bars and non-essential output                                 |
| `--help` / `-h`    | boolean | Show help for a command                                                         |

Most command-local flags also have shorthands. Common mappings are `--domain -d`, `--force -f`,
`--json -j`, `--priority -p`, `--targets -t`, `--content -c`, `--description -s`, `--path -p`,
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
2. **Directory config**: `.ai-rulez/config.toml`, `.ai-rulez/config.yaml`, `.ai-rulez/config.yml`, or `.ai-rulez/config.json`
3. **Project convention**: the same four filenames under `.config/ai-rulez/`, used only when discovering the default layout (an explicit `--config-dir` is honoured exactly and never falls back)
4. **Legacy flat V2 config**: `ai-rulez.yaml`, `ai-rulez.yml`, `.ai-rulez.yaml`, `.ai-rulez.yml`, `ai_rulez.yaml`, `ai_rulez.yml`, `.ai_rulez.yaml`, or `.ai_rulez.yml` are discovered for migration
5. **Error**: No configuration found

The search walks up from the current directory. Legacy flat V2 config files are migration inputs. Use `ai-rulez migrate v4` before running V4
generation workflows. `.ai-rulez/` and `.config/ai-rulez/` are checked before the legacy flat filenames.

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

The CLI uses standard exit codes:

| Code | Meaning                                                          |
| ---- | ---------------------------------------------------------------- |
| 0    | Success                                                          |
| 1    | Error (config not found, validation failed, bad flags, etc.)     |
| 2    | `tokens --budget` exceeded — a hook can tell over-budget from failure; also `validate --strict` / `scan` findings, drift reported by `generate --check` / `verify`, and `lock --check` mismatches |

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
ai-rulez remove rule old-guideline --force
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
git add .ai-rulez/   # plus generated files only if gitignore = false
git commit -m "docs: update AI assistant guidelines"
```

### CI/CD Integration

```bash
#!/bin/bash
# Simple CI/CD script

# Validate configuration (--no-local: ignore any machine-local overlay)
ai-rulez validate --no-local || exit 1

# Generate all outputs
ai-rulez generate --no-local || exit 1

# Check for uncommitted changes
if ! git diff --quiet CLAUDE.md .cursor/ GEMINI.md; then
  echo "Generated files are out of sync"
  echo "Run: ai-rulez generate"
  exit 1
fi
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
