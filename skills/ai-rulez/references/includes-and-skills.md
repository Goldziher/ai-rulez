# Includes and Installed Skills

## Includes

Includes let you import rules, context, skills, and MCP configuration from external git repositories or local paths.

### Adding Includes

```bash
# From a git repository
ai-rulez include add shared-rules https://github.com/org/shared-rules

# With specific content types
ai-rulez include add shared-rules https://github.com/org/shared-rules --include rules,context

# Install into a domain
ai-rulez include add backend-rules https://github.com/org/backend --install-to domains/backend

# With merge strategy
ai-rulez include add overrides https://github.com/org/overrides --merge-strategy include-override

# Local path
ai-rulez include add local-rules ./path/to/local
```

### Include Config Options

```toml
[[includes]]
name = "shared-rules"
source = "https://github.com/org/shared-rules"
path = "modules/core"              # Subdirectory within repo containing .ai-rulez/
ref = "v2.0"                       # Branch, tag, or commit
include = ["rules", "context"]     # Content types (default: all)
merge_strategy = "local-override"
install_to = "domains/shared"      # Install as domain instead of merging at root
local_override = "../local-shared" # Use local path for development
```

### How Includes Work

1. During config load, each include source is fetched with sparse git checkout or local scan
2. The source must contain an `.ai-rulez/` directory with content
3. Content is merged with local content using the configured merge strategy
4. Domains from includes are marked as `FromInclude` and always included regardless of profile

### Local Override

The `local_override` field lets you point to a local checkout during development. If the path exists, it's used instead of the git source. If it doesn't exist, the include is silently skipped.

## Installed Skills

Installed skills are named skills fetched from external repositories at generation time. Unlike includes (which import `.ai-rulez/` content), installed skills expect a `skills/<name>/SKILL.md` structure at the repository root.

### Installing Skills

```bash
# Install by name (defaults to skills/<name>/ path in repo)
ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg

# With explicit path
ai-rulez skill install my-skill --source https://github.com/org/repo --path custom/skill/path

# With specific git ref
ai-rulez skill install my-skill --source https://github.com/org/repo --ref v2.0
```

### Installed Skills Config

```toml
[[installed_skills]]
name = "kreuzberg"
source = "https://github.com/kreuzberg-dev/kreuzberg"
path = "skills/kreuzberg"       # Optional, defaults to skills/<name>
ref = "main"                    # Optional git ref
local_override = "../kreuzberg" # Optional local dev path
```

### Skill Directory Structure

```text
repo-root/
  skills/
    my-skill/
      SKILL.md                 # Required - main skill content with frontmatter
      references/              # Optional - additional reference documents
        api-reference.md
        configuration.md
      scripts/                 # Optional - executable helper scripts
      assets/                  # Optional - static files or images
```

### How Installed Skills Work

1. During `ai-rulez generate`, each installed skill is fetched from its source
2. `SKILL.md` is read and parsed (frontmatter + content)
3. `references/`, `scripts/`, and `assets/` are loaded as skill resources
4. Presets with skill directories emit resources as sibling files and add a resource index to `SKILL.md`
5. The skill is added to the root-level skills in the content tree
6. Local skills with the same name take precedence (installed skill is skipped with a warning)

### Creating Distributable Skills

To make your project's skill installable:

1. Create `skills/<name>/SKILL.md` at your repository root
2. Add YAML frontmatter with `name`, `description`, and optional `metadata`
3. Write comprehensive instructions in the skill body
4. Optionally add `references/*.md` for detailed reference documentation

```yaml
---
name: my-library
description: >-
  Brief description of what this skill covers and when to use it.
license: MIT
metadata:
  author: your-org
  version: "1.0"
  repository: https://github.com/your-org/your-repo
---
# My Library

Instructions for AI assistants working with your library...
```
