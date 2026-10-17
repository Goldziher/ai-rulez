# Installed Skills

Install named skills from external repositories. Skills are fetched dynamically at generate time and included in your outputs — no local copy needed.

## Overview

Installed skills let you pull specialized AI instructions from any git repository or local path. Unlike [includes](includes.md) (which import `.ai-rulez/` content), installed skills expect a simpler structure: a `skills/<name>/SKILL.md` file at the repository root.

Use installed skills to:

- Add library-specific instructions (e.g., how to use a framework or SDK)
- Share team skills across projects without full includes
- Distribute AI guidance alongside your library

!!! tip "No `.ai-rulez/` folder needed"
    A repository with hand-written `CLAUDE.md` and `AGENTS.md` can declare `[[installed_skills]]` in a single root-level `ai-rulez.toml` (create it with `ai-rulez init --minimal`). `skill install` and `skill remove` edit that file. See [Config without a .ai-rulez folder](config-without-folder.md).

## Installed skills vs skill sources

ai-rulez can reach a skill in another repository two ways, and they differ in *what lands where*.

| | Installed skill (`[[installed_skills]]`) | Skill source (`[[skill_sources]]`) |
| --- | --- | --- |
| What it is | One named skill fetched at generate time | A repository whose skills are served on demand |
| Where it goes | Written into the harness skill trees, like a local skill | Never written; served over MCP and found with `find_skill` |
| Reaches the project as | A content file, so profiles and roles scope it | A served skill, so the [skills server](mcp-server.md#dynamic-skill-loading) and a role's `delivery` scope it |
| Use it for | A skill the agent should always have in context | A catalog of skills the agent should find on demand |

Both are pinned in `ai-rulez.lock` and scanned before they are used. Reach for an **installed skill** when the
skill belongs in the agent's static context (a library's usage guide), and for a **skill source** when you want many
skills available without paying their tokens up front (a vendor's whole skill catalog). Both select several skills
from one repository (`include`/`exclude`), namespace their names (`name_prefix`) and set the scan level of what
they import (`trust`).

`ai-rulez skill install`, `list`, `update` and `remove` manage installed skills; skill sources are declared in
`[[skill_sources]]` in `config.toml`.

## Quick Start

```bash
# Install a skill
ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg

# List installed skills
ai-rulez skill list

# Generate — the skill is fetched and included automatically
ai-rulez generate

# Remove when no longer needed
ai-rulez skill remove kreuzberg
```

## Configuration

Installed skills are defined in `.ai-rulez/config.toml`:

```toml
[[installed_skills]]
name = "kreuzberg"
source = "https://github.com/kreuzberg-dev/kreuzberg"

[[installed_skills]]
name = "ai-rulez"
source = "https://github.com/Goldziher/ai-rulez"
ref = "main"

[[installed_skills]]
name = "custom-skill"
source = "https://github.com/org/repo"
path = "custom/skill/path"    # defaults to skills/<name>

# One entry can install several skills from one repository.
[[installed_skills]]
name = "vendor"
source = "https://github.com/org/skills"
path = "skills"               # a directory of skill directories
include = ["db-*", "api"]     # keep only these directory names
exclude = ["*-wip"]           # drop these; exclude wins over include
name_prefix = "vendor-"       # every resolved skill is named, and written to, vendor-<dir>
trust = "warn"                # only error-severity scan findings block generate
```

### Fields

| Field            | Required | Description                                                      |
| ---------------- | -------- | ---------------------------------------------------------------- |
| `name`           | Yes      | Unique skill name                                                |
| `source`         | Yes      | Where the skill comes from; see [Source forms](#source-forms)    |
| `path`           | No       | Path within repo to the skill directory, or to a directory of skill directories. Defaults to `skills/<name>` |
| `ref`            | No       | Git ref (branch, tag, or commit). Defaults to the repository's default branch (`HEAD`) |
| `version`        | No       | Version range of the source's tags (`^1.2`), instead of `ref` |
| `tag_prefix`     | No       | With `version`, only consider tags that start with this prefix (`v`, `skills/`) |
| `include_prerelease` | No   | With `version`, let pre-release tags match. Default `false`      |
| `min_release_age` | No      | With `version`, skip tags younger than this (`7d`), so a brand-new release is not picked up at once |
| `local_override` | No       | Local path override for development                              |
| `profiles`       | No       | Restrict the skill to the named profiles. Omit to include it in every profile |
| `domain`         | No       | Tie the skill to a domain so a profile or a role selects it by domain, exactly as it selects local content. Omit for global |
| `include`        | No       | Keep only skills whose directory name matches one of these globs. Empty keeps every skill below `path` |
| `exclude`        | No       | Drop skills whose directory name matches one of these globs; wins over `include` |
| `name_prefix`    | No       | Prepend this to every resolved skill name and output directory (`.claude/skills/vendor-<dir>/`), so two entries cannot collide |
| `resources`      | No       | What is bundled from each skill directory besides `references/`, `scripts/` and `assets/`. The default is the whole directory (see below); setting it replaces that default |
| `trust`          | No       | Scan level of the imported skills: `error` (every finding counts, the default) or `warn` (only error-severity findings block) |

One entry resolves to one skill when the directory `path` names holds a `SKILL.md`, and to the
`SKILL.md`-bearing subdirectories of `path` otherwise, selected by `include`/`exclude` and named with
`name_prefix`. A single-skill entry behaves exactly as it did before these fields existed.

### What is bundled with a skill

An installed skill arrives whole. Besides `references/`, `scripts/` and `assets/`, every file in the skill directory
is bundled: documents beside `SKILL.md` (`cli.md`), folders of any name (`rules/`, `templates/`) and their
subfolders. Documents are listed under References in the generated `Resources` section of `SKILL.md`, interpreter
scripts under Scripts and everything else under Assets; each is written at its path below the skill directory, so the
links in `SKILL.md` keep working. Four things are left out: `SKILL.md` itself, `evals/` (eval cases cost context),
`agents/` (tool display metadata such as `openai.yml`) and hidden files. Files that git ignores, `node_modules`,
`.venv` and the other [`bundle_exclude`](configuration.md#bundle_exclude) defaults are skipped as for any skill.

`resources` replaces that default for the entry. A pattern is a name, a path or a glob; a name such as `rules`
matches that file or directory at any depth, and a leading `!` removes what an earlier pattern added:

```toml
resources = ["*", "!docs", "!templates", "!evals", "!agents", "!.*"]  # everything but docs/ and templates/
resources = ["rules"]                                                  # only rules/, plus the canonical folders
resources = ["*"]                                                      # also evals/, agents/ and hidden files
```

Whatever `resources` says, an installed skill never bundles credential-like files (`id_rsa*`, `id_ed25519*`,
`*.pem`, `*.key`, `*.p12`, `*.pfx`, `*.keystore`, `credentials.json`, `*.tfstate`, `.env.*`), symlinks, or special
files. Each skipped credential file is named in a warning. A single file over 8 MiB is skipped with a warning naming it.
A skill that bundles more than 2000 files or 16 MiB in total fails with an error naming the limit; narrow it with
`resources`, for example `["*", "!docs", "!evals", "!agents", "!.*"]`.

All the skills one `installed_skills` entry resolves together may bundle at most 20000 files and 64 MiB; past that the
entry fails with an error naming the limit. Select fewer skills with `include` / `exclude`, or narrow what each bundles
with `resources`.

A project's own skills under `.ai-rulez/skills/` keep the canonical layout; only installed skills are bundled whole.

## Source forms

`source` accepts what a harness would install from:

| Form | Example | Notes |
| --- | --- | --- |
| Git URL | `https://github.com/org/repo`, `git@github.com:org/repo.git` | `https`, `ssh` and scp-style; plain `http://` is refused |
| GitHub shorthand | `org/repo`, `github:org/repo` | Expands to `https://github.com/org/repo`, so the lock records the full URL. A bare `org/repo` that names an existing directory in the project stays a local path; `github:` is always a repository |
| `file://` URL | `file:///srv/skills` | A git repository is cloned; any other directory is read as a local path |
| Local directory | `./vendor/skills`, `/abs/path` | Read in place |
| Home directory | `~`, `~/skills` | Local, expanded to your home directory |

A local path (including `file://` and `~`) in the committed project config must resolve inside the project, symlinks
resolved, the same rule as local includes and `[[skill_sources]]`. A path outside it is allowed only from
`config.local.toml` (or `skill install --local`) and from the user config used by `generate --user`; in a committed
config it fails with a hint. `~user/path` is not supported. Set `AI_RULEZ_ALLOW_FILE_URLS=1` to let a `file://` git
source leave the project.

## Discovery

The entry root is `path` (default `skills/<name>`, or `skills` when `include`/`exclude` is set). Below it, every
directory that holds a `SKILL.md` is a skill, to six levels deep; a skill directory is not searched further.
Symlinks, hidden directories and `node_modules` are never entered. Two skills with the same directory name keep the
shallowest and the other is skipped with a warning. An entry may resolve to at most 200 skills. The search itself
stops after 5000 skill directories (in name order) with a warning; narrow the entry with `path`. A skill whose
`SKILL.md` cannot be read is skipped with a warning and the rest of the entry still resolves.

When the entry sets no `path` and nothing is at the default location, the source is searched where harnesses keep
skills: a `SKILL.md` at the root is one skill named after the entry; otherwise `skills/`, `.claude/skills/`,
`.agents/skills/`, `.codex/skills/`, `.cursor/skills/`, `.gemini/skills/`, `.github/skills/` and `.opencode/skills/`,
in that order. Without `include`/`exclude` the entry names one skill, the directory called `name`; with them, every
match across those directories. A git source is fetched whole for this search only when the default location is empty.

## CLI Commands

### `ai-rulez skill install <name> --source <url> [flags]`

Install a named skill.

**Flags:**

- `--source <url>` (required): Git URL or local path
- `--path <dir>` (optional): Path within repo to skill directory
- `--ref <ref>` (optional): Git ref (branch, tag, commit)
- `--include <glob>`, `--exclude <glob>` (optional, repeatable): Select skills from a directory of skills
- `--name-prefix <prefix>` (optional): Prefix added to every skill name
- `--trust error|warn` (optional): Scan level of the imported skills
- `--resources <pattern>` (optional, repeatable): What is bundled besides the canonical folders
- `--version <range>` (optional): Version range of the source's tags, instead of `--ref`
- `--tag-prefix <prefix>`, `--include-prerelease`, `--min-release-age <age>` (optional): Tag filter and release age gate for `--version`
- `--domain <name>` (optional): Place the skills in a domain, so a profile or role selects them by domain
- `--profile <name>` (optional, repeatable): Restrict the skills to these profiles
- `--local-override <dir>` (optional): Directory used instead of the source while it exists
- `--local` (optional): Record the skill in your gitignored `config.local.*` overlay instead of `config.toml`, so it applies on this machine only. See [Local Configuration](local-overrides.md)

`skill install` refuses a `--source` URL that carries a credential in its userinfo (`https://user:token@host/...`):
writing it would commit the secret to `config.toml` in clear. Keep the URL free of secrets and authenticate with the
git credential helper or the `AI_RULEZ_GIT_TOKEN` environment variable; `validate`/`scan` report an already committed
credentialed source as `AR035` (see [Strict validation](strict-validation.md)).

**Examples:**

```bash
# From a git repo (skill at skills/kreuzberg/)
ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg

# With explicit path and ref
ai-rulez skill install my-lib --source https://github.com/org/repo --path libs/my-lib --ref v2.0

# GitHub shorthand, several skills with a prefix
ai-rulez skill install vendor --source org/skills --include 'db-*' --name-prefix vendor-

# From a local path inside the project
ai-rulez skill install local-skill --source ./vendor/my-other-repo

# From outside the project: machine-local only
ai-rulez skill install mine --source ~/skills --local
```

### `ai-rulez skill remove <name> [flags]`

Remove an installed skill from the configuration.

**Flags:**

- `--yes`, `-y` (optional): Skip confirmation prompt
- `--local` (optional): Remove through the overlay. A skill defined in the shared config is hidden on this machine by writing `remove = true` to the overlay; the shared config is not changed

### `ai-rulez skill list [flags]`

List all installed skills.

**Flags:**

- `--format text|json` (optional, default `text`): `json` prints JSON

## How It Works

1. During `ai-rulez generate`, each installed skill is fetched from its source
2. The skill directory (`SKILL.md` + optional `references/`) is read
3. `references/`, `scripts/` and `assets/` are kept as separate files beside the generated `SKILL.md`
4. The skill is added to the root-level content tree
5. If a local skill has the same name, the local skill wins (installed skill is skipped with a warning)

## Skill Directory Structure

Installed skills expect this layout in the source repository:

```text
repo-root/
  skills/
    my-skill/
      SKILL.md              # Required: main skill content
      references/            # Optional: additional reference docs
        api-reference.md
        configuration.md
```

### SKILL.md Format

```yaml
---
name: my-library
description: >-
  Brief description of what this skill covers.
license: MIT
metadata:
  author: your-org
  version: "1.0"
  repository: https://github.com/your-org/your-repo
---
# My Library

Instructions for AI assistants working with your library...
```

A missing `description` produces a warning rather than an error; the skill name is used as a fallback. Set it anyway — the description is how the assistant decides when to load the skill.

### References

Files in `references/` (like `scripts/` and `assets/`) are kept as separate files next to the generated `SKILL.md`, for both path and git sources; they are not appended to the skill body. This lets you separate detailed API docs, configuration guides, etc. from the main skill instructions, and the assistant loads them on demand.

## Pinning with a lock file

`ref` defaults to the repository's default branch (`HEAD`), which moves. Run `ai-rulez lock` (or `ai-rulez skill update <name>`) to record the resolved commit and a content digest in `.ai-rulez/ai-rulez.lock`. Later `generate` runs fetch exactly that commit and fail if the files do not match the digest; `generate --locked` fails when a skill is not covered, and `--frozen` never uses the network. A skill read from a local directory is pinned too, as a `local-skill` item holding the tree digest of the skill directory (like a [local include](lockfile.md)); editing a file in it makes `lock --check` and `generate --locked` fail until `ai-rulez lock` is rerun. See the [Lock Command](cli.md#lock-command). Imported skill text is instruction text: with `[lint.security] scan_imports`, it is also scanned for secrets, hidden characters and risky commands before it is written ([Security checks](strict-validation.md#security-checks)).

To serve a repository of skills over MCP instead of writing them into the skill trees, use `[[skill_sources]]`
or `ai-rulez mcp --serve-skills --source`; they are pinned in the same lock (kind `source`). See
[Dynamic skill loading](mcp-server.md#skill-sources).

## Local Override

For development workflows, use `local_override` to point to a local checkout instead of fetching from git:

```toml
[[installed_skills]]
name = "my-lib"
source = "https://github.com/org/my-lib"
local_override = "../my-lib"
```

If the local path exists and contains the skill, it's used. If it doesn't exist, the skill is skipped (not fetched from git).

## MCP Tools

The MCP server exposes three tools for managing installed skills:

- `install_skill` — Install a skill (params: `name`, `source`, `path`, `ref`, `local`)
- `uninstall_skill` — Remove a skill (params: `name`, `local`)
- `list_installed_skills` — List all installed skills (shared layer only; takes no `local`)

`local: true` writes to the `config.local.*` overlay instead of the shared config.

## Creating Distributable Skills

To make your project's skill installable by others:

1. Create `skills/<project-name>/SKILL.md` at your repository root
2. Add YAML frontmatter with `name`, `description`, and optional `metadata`
3. Write comprehensive instructions in the skill body
4. Optionally add `references/*.md` for detailed reference documentation
5. Users install with: `ai-rulez skill install <name> --source <your-repo-url>`

## Comparison with Includes

| Feature          | Includes                                               | Installed Skills                          |
| ---------------- | ------------------------------------------------------ | ----------------------------------------- |
| Content types    | Rules, context, skills, agents, commands               | Skills only                               |
| Source structure | A `.ai-rulez/` directory or a bare layout (`rules/`, `skills/`, ...), see [Repository Structure Support](includes.md#repository-structure-support) | Requires `skills/<name>/SKILL.md` |
| Merge strategy   | Configurable (local-override, include-override, error) | Local skills always win                   |
| Domain support   | Can install to specific domains                        | Root level, or a domain through the `domain` field |
| Use case         | Share full governance configs                          | Add library/tool-specific AI instructions |
