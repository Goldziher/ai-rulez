# Installed Skills

Install named skills from external repositories. Skills are fetched dynamically at generate time and included in your outputs — no local copy needed.

## Overview

Installed skills let you pull specialized AI instructions from any git repository or local path. Unlike [includes](includes.md) (which import `.ai-rulez/` content), installed skills expect a simpler structure: a `skills/<name>/SKILL.md` file at the repository root.

Use installed skills to:

- Add library-specific instructions (e.g., how to use a framework or SDK)
- Share team skills across projects without full includes
- Distribute AI guidance alongside your library

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
```

### Fields

| Field            | Required | Description                                                      |
| ---------------- | -------- | ---------------------------------------------------------------- |
| `name`           | Yes      | Unique skill name                                                |
| `source`         | Yes      | Git URL or local path to the repository                          |
| `path`           | No       | Path within repo to skill directory. Defaults to `skills/<name>` |
| `ref`            | No       | Git ref (branch, tag, or commit). Defaults to the repository's default branch (`HEAD`) |
| `local_override` | No       | Local path override for development                              |
| `profiles`       | No       | Restrict the skill to the named profiles. Omit to include it in every profile |

## CLI Commands

### `ai-rulez skill install <name> --source <url> [flags]`

Install a named skill.

**Flags:**

- `--source <url>` (required): Git URL or local path
- `--path <dir>` (optional): Path within repo to skill directory
- `--ref <ref>` (optional): Git ref (branch, tag, commit)
- `--local` (optional): Record the skill in your gitignored `config.local.*` overlay instead of `config.toml`, so it applies on this machine only. See [Local Configuration](local-overrides.md)

**Examples:**

```bash
# From a git repo (skill at skills/kreuzberg/)
ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg

# With explicit path and ref
ai-rulez skill install my-lib --source https://github.com/org/repo --path libs/my-lib --ref v2.0

# From a local path
ai-rulez skill install local-skill --source ../my-other-repo
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
3. Reference files are concatenated into the skill content
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

`ref` defaults to the repository's default branch (`HEAD`), which moves. Run `ai-rulez lock` (or `ai-rulez skill update <name>`) to record the resolved commit and a content digest in `.ai-rulez/ai-rulez.lock`. Later `generate` runs fetch exactly that commit and fail if the files do not match the digest; `generate --locked` fails when a skill is not covered, and `--frozen` never uses the network. See the [Lock Command](cli.md#lock-command). Imported skill text is instruction text: with `[lint.security] scan_imports`, it is also scanned for secrets, hidden characters and risky commands before it is written ([Security checks](strict-validation.md#security-checks)).

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
| Domain support   | Can install to specific domains                        | Root-level only                           |
| Use case         | Share full governance configs                          | Add library/tool-specific AI instructions |
