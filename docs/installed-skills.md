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

Installed skills are defined in `.ai-rulez/config.yaml`:

```yaml
installed_skills:
  - name: kreuzberg
    source: https://github.com/kreuzberg-dev/kreuzberg
  - name: ai-rulez
    source: https://github.com/Goldziher/ai-rulez
    ref: main
  - name: custom-skill
    source: https://github.com/org/repo
    path: custom/skill/path    # defaults to skills/<name>
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique skill name |
| `source` | Yes | Git URL or local path to the repository |
| `path` | No | Path within repo to skill directory. Defaults to `skills/<name>` |
| `ref` | No | Git ref (branch, tag, or commit). Defaults to `main` |
| `local_override` | No | Local path override for development |

## CLI Commands

### `ai-rulez skill install <name> --source <url> [flags]`

Install a named skill.

**Flags:**

- `--source <url>` (required): Git URL or local path
- `--path <dir>` (optional): Path within repo to skill directory
- `--ref <ref>` (optional): Git ref (branch, tag, commit)

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

- `--force` (optional): Skip confirmation prompt

### `ai-rulez skill list [flags]`

List all installed skills.

**Flags:**

- `--json` (optional): Output as JSON

## How It Works

1. During `ai-rulez generate`, each installed skill is fetched from its source
2. The skill directory (`SKILL.md` + optional `references/`) is read
3. Reference files are concatenated into the skill content
4. The skill is added to the root-level content tree
5. If a local skill has the same name, the local skill wins (installed skill is skipped with a warning)

## Skill Directory Structure

Installed skills expect this layout in the source repository:

```
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

The `description` field is required for validation.

### References

Files in `references/` are read alphabetically and appended to the skill content under `## Reference: <filename>` headings. This lets you separate detailed API docs, configuration guides, etc. from the main skill instructions.

## Local Override

For development workflows, use `local_override` to point to a local checkout instead of fetching from git:

```yaml
installed_skills:
  - name: my-lib
    source: https://github.com/org/my-lib
    local_override: ../my-lib
```

If the local path exists and contains the skill, it's used. If it doesn't exist, the skill is skipped (not fetched from git).

## MCP Tools

The MCP server exposes three tools for managing installed skills:

- `install_skill` — Install a skill (params: `name`, `source`, `path`, `ref`)
- `uninstall_skill` — Remove a skill (params: `name`)
- `list_installed_skills` — List all installed skills

## Creating Distributable Skills

To make your project's skill installable by others:

1. Create `skills/<project-name>/SKILL.md` at your repository root
2. Add YAML frontmatter with `name`, `description`, and optional `metadata`
3. Write comprehensive instructions in the skill body
4. Optionally add `references/*.md` for detailed reference documentation
5. Users install with: `ai-rulez skill install <name> --source <your-repo-url>`

## Comparison with Includes

| Feature | Includes | Installed Skills |
|---------|----------|-----------------|
| Content types | Rules, context, skills, agents, MCP | Skills only |
| Source structure | Requires `.ai-rulez/` directory | Requires `skills/<name>/SKILL.md` |
| Merge strategy | Configurable (local-override, include-override, error) | Local skills always win |
| Domain support | Can install to specific domains | Root-level only |
| Use case | Share full governance configs | Add library/tool-specific AI instructions |
