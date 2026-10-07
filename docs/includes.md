# Includes System

Reuse configurations across multiple projects through inheritance and composition.

## Overview

Includes work through configuration inheritance:

1. Define common rules once in a shared configuration
2. Share rules, context, skills, agents, and commands across projects
3. Mix and match includes to create project-specific configurations
4. Track changes through version control

## How Includes Work

Includes allow one `.ai-rulez/` configuration to inherit content from other configurations. Use for:

- Organization-wide coding standards
- Framework-specific guidelines (React, Go, Python)
- Consistent security policies
- Team-specific workflows

## Basic Example

### Creating a Shared Configuration

Create a `.ai-rulez/` directory that others can include:

**`shared-rules/.ai-rulez/config.toml`:**

```toml
version = "4.0"
name = "shared-rules"
description = "Organization-wide AI rules"

presets = []

[profiles]
default = []
```

**`shared-rules/.ai-rulez/rules/security.md`:**

```markdown
---
priority: critical
---

# Security Standards

- Always validate user input
- Use parameterized queries
- Never hardcode secrets
- Rotate credentials regularly
```

### Including in Your Project

In your project's `.ai-rulez/config.toml`, reference the shared rules:

```toml
version = "4.0"
name = "my-project"

# Include rules from another directory
includes = [
  { name = "shared-rules", source = "../shared-rules/.ai-rulez" }
]

presets = ["claude", "cursor"]

[profiles]
default = []
```

Now your project includes all content from the shared configuration.

## Include Paths

Includes can be:

1. Relative paths: `../shared-rules/.ai-rulez`, `./team-guidelines/.ai-rulez`
2. Absolute paths: `/etc/ai-rulez-standards/.ai-rulez`
3. Git URLs: `https://github.com/org/shared-rules.git`, `git@github.com:org/shared-rules.git`

!!! warning "Local paths must stay inside the project"
    A local path (or `local_override`) in the project's committed config must resolve inside the project, after
    symlinks are resolved: a repository could otherwise name `../victim` or `~/.config` and have that content
    written into its generated outputs. A path outside the project stops loading with an error naming it (it is not skipped with a warning). Set it in the
    machine-local overlay (`config.local.toml`), declare the include in your user config (`generate --user`), or use a
    git include instead. The examples below that use `../` or an absolute path need one of these.

### Examples

**Sibling directory:**

```toml
[[includes]]
name = "shared-rules"
source = "../shared-rules"
include = ["rules", "context"]
merge_strategy = "local-override"
```

**Subdirectory:**

```toml
[[includes]]
name = "team-config"
source = "./config/shared"
include = ["rules", "skills"]
merge_strategy = "local-override"
```

**Git repository (HTTPS):**

```toml
[[includes]]
name = "org-standards"
source = "https://github.com/myorg/shared-rules.git"
ref = "main"
include = ["rules", "context", "skills", "agents"]
merge_strategy = "local-override"
```

**Git repository (SSH):**

```toml
[[includes]]
name = "company-policies"
source = "git@github.com:company/ai-rulez.git"
ref = "v1.2.3"
include = ["rules", "context"]
merge_strategy = "local-override"
```

**Multiple includes:**

```toml
[[includes]]
name = "team-guidelines"
source = "../team-guidelines"
include = ["rules", "context"]
merge_strategy = "local-override"

[[includes]]
name = "org-standards"
source = "git@gitlab.com:org/standards.git"
ref = "main"
include = ["rules", "skills"]
merge_strategy = "local-override"

[[includes]]
name = "security-policies"
source = "./security-policies"
include = ["rules"]
merge_strategy = "local-override"
```

### Supported Git URL Formats

- **HTTPS:** `https://github.com/owner/repo.git`
- **SSH:** `git@github.com:owner/repo.git`
- **SSH protocol:** `ssh://git@github.com/owner/repo.git`
- **GitLab:** `https://gitlab.com/owner/repo.git`, `git@gitlab.com:owner/repo.git`
- **`git+` prefix:** accepted, as for skill sources and `--source`: `git+https://host/org/repo` is the same include as `https://host/org/repo` and shares its cache. The lock records the source as you wrote it.
- **Self-hosted GitLab:** `git@git.example.com:owner/repo.git`, `https://git.example.com/owner/repo.git`

### SSH Cloning for Private Repositories

For private repositories that require SSH authentication, ai-rulez automatically uses `git clone` when it detects SSH URLs (`git@...` or `ssh://...`). This leverages your existing SSH key configuration.

**Benefits of SSH cloning:**

- Works with private repositories without needing access tokens
- Uses your configured SSH keys and agent
- Supports self-hosted Git servers (GitLab, Gitea, Gogs, etc.)
- Ideal for local development and multi-repo setups

**Example with SSH:**

```toml
[[includes]]
name = "private-rules"
source = "git@git.example.com:company/ai-rulez.git"
ref = "main"
include = ["rules", "context"]
merge_strategy = "local-override"
```

**Requirements:**

- Git must be installed and available in your PATH
- SSH keys must be configured for the git host
- SSH agent should be running (for passphrase-protected keys)

### Repository Structure Support

An include source does **not** have to wrap its content in a `.ai-rulez/` folder. ai-rulez
supports both a wrapped and a bare (flattened) layout and auto-detects which one a source uses.

1. **Wrapped structure**: Content lives inside a `.ai-rulez/` subdirectory

   ```text
   my-repo/
   ├── .ai-rulez/
   │   ├── config.toml
   │   ├── rules/
   │   ├── context/
   │   ├── skills/
   │   └── agents/
   └── other files...
   ```

2. **Bare / flattened structure** (recommended for shared-module repos): the directory exposes
   `rules/`, `context/`, `skills/`, and `agents/` directly — no `.ai-rulez/` wrapper needed. This
   works at the repository root **or** at any sub-path:

   ```text
   my-repo/
   └── modules/
       └── core/
           ├── rules/
           ├── context/
           ├── skills/
           └── agents/
   ```

   Point an include at the sub-path with the `path` field:

   ```toml
   [[includes]]
   name = "core"
   source = "https://github.com/org/shared-modules.git"
   path = "modules/core"     # resolves modules/core/rules, modules/core/skills, ...
   include = ["rules", "skills"]
   merge_strategy = "local-override"
   ```

ai-rulez detects the layout automatically: it first looks for a `.ai-rulez/` directory (at the
source root or under `path`), and otherwise treats a directory that contains any of `rules/`,
`context/`, `skills/`, `agents/`, or `commands/` as a bare ai-rulez structure. The flat layout keeps
shared modules — especially skill-first modules that ship mostly `skills/<id>/SKILL.md` — clean and
free of boilerplate wrapping.

!!! note
    `ai-rulez include add` with a **local** path validates that the directory contains a `.ai-rulez/`
    subdirectory, so a bare/flattened *local* source is rejected by the CLI even though the resolver
    accepts it. Add a bare local include by editing `config.toml` directly (git sources are not
    restricted this way).

### Private Repository Authentication (HTTPS)

When working with private Git repositories in includes, you can authenticate using an access token.

#### Using Environment Variable (Recommended)

Set the `AI_RULEZ_GIT_TOKEN` environment variable with your access token:

```bash
export AI_RULEZ_GIT_TOKEN="ghp_your_github_token_here"
ai-rulez generate
```

This is the recommended approach for CI/CD environments and automation scripts.

#### Which hosts receive the token

The token is sent only to `github.com` by default. To use it with another host, list the hosts in the environment
(a project file cannot widen this):

```bash
export AI_RULEZ_GIT_TOKEN_HOSTS="github.com,gitlab.example.com"
```

The list replaces the default. An include that names any other HTTPS host is fetched without the token and logs a
warning. The token is passed to git as a header scoped to the host, not embedded in the URL, so it does not appear in
process arguments or in the cached clone's `.git/config`. Plain `http://` and `git://` remotes are rejected.

#### Using CLI Flag

Pass the token directly via the `--token` flag:

```bash
ai-rulez generate --token "ghp_your_github_token_here"
```

#### Creating Access Tokens

**GitHub:**

1. Go to Settings → Developer settings → Personal access tokens
2. Click "Generate new token (classic)"
3. Select scopes:
   - `repo` - Required for accessing private repositories
4. Generate and copy the token

**GitLab:**

1. Go to User Settings → Access Tokens
2. Click "Add new token"
3. Select scopes:
   - `read_repository` - Required for reading private repositories
4. Create token and copy it

**Other Git Hosts:**

Most Git hosting platforms that support Bearer token authentication will work with ai-rulez. The token is sent as a Bearer token in the Authorization header when fetching repository archives.

#### Security Best Practices

- **Never commit tokens** to your repository or configuration files
- **Use environment variables** in CI/CD pipelines (GitHub Actions secrets, GitLab CI/CD variables, etc.)
- **Store tokens securely** using secret management systems (AWS Secrets Manager, HashiCorp Vault, etc.)
- **Rotate tokens regularly** to limit exposure from potential leaks
- **Use minimal permissions** - only grant the token access to what's needed (read-only repository access)
- **Use organization-level tokens** when possible to manage access centrally

#### Example: CI/CD Integration

**GitHub Actions:**

```yaml
name: Generate AI Rules
on: [push]
jobs:
  generate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Generate
        env:
          AI_RULEZ_GIT_TOKEN: ${{ secrets.GIT_TOKEN }}
        run: npx ai-rulez@latest generate
```

**GitLab CI:**

```yaml
generate:
  script:
    - export AI_RULEZ_GIT_TOKEN="$CI_JOB_TOKEN"
    - ai-rulez generate
```

### Include Options

- **`name`**: Unique identifier for the include
- **`source`**: Path or Git URL to the configuration
- **`path`**: (Optional) Sub-path within the source to resolve (e.g. `modules/core`). Supports the bare/flattened layout described above; defaults to the source root
- **`ref`**: (Git only) Branch, tag, or commit SHA. Defaults to the remote's default branch (`HEAD`), not necessarily `main`.
- **`version`**: (Git only) A semantic-version range such as `^1.2` or `~2.1.0`, resolved against the repository's tags and pinned in the lock; instead of `ref` (not both). `tag_prefix` and `include_prerelease` refine it, and `min_release_age = "7d"` holds back tags younger than that. Only `ai-rulez update` moves the pin. See [Version constraints](lockfile.md#version-constraints).
- **`include`**: List of content types to fetch: `rules`, `context`, `skills`, `agents`, `commands`. MCP servers are not importable from an include.
- **`install_to`**: (Optional) Import the included content into a specific domain instead of the root.
- **`local_override`**: (Optional) A local path used **instead of** `source` (for example a checkout you are developing). It is resolved against the project directory, and `path` is appended to it. If the directory does not exist, the include is skipped silently (an info line is logged); the remote source is not used as a fallback. `local_override` bypasses the lock, so it is honoured only when set in the machine-local overlay (`config.local.toml`) or when no lock is enforced: `generate --locked`, `--frozen` and an enforced lock fail on a `local_override` in the committed config.
- **`merge_strategy`**: How to handle conflicts:
  - `local-override`: local content takes precedence (default)
  - `include-override`: the included content takes precedence
  - `error`: fail generation on a conflict

Each include carries its own strategy. Includes are not recursive: an include contributes its own
content, and any `includes` declared in the included configuration are ignored. Domains from an
include are always carried over; `include` filters content kinds, not domains.

An include that cannot be created, fetched or merged is an error: the command exits `1` and names the
include. A remote include that is unreachable falls back to its cached copy when one exists; with no cache there
is nothing to render. This holds for `generate`, `generate --check`, `validate`, `doctor` and every other command
that loads the configuration, so a CI gate cannot pass on a checkout that renders without the include. Under
`--no-fetch` (cached content only) an include that cannot be resolved is logged as a warning and skipped instead,
and `ai-rulez lock` reports it as a problem of its own.

Included content never follows symlinks: a symlinked file or directory (a rule, skill, agent, command, context or
check file, `rules/`, `skills/` and so on, domains, or the include's `.ai-rulez/` itself) is skipped with a warning
naming it, so an include cannot point at an arbitrary local file. Replace the link with the real file or directory.
An installed skill whose `SKILL.md` is a symlink fails to resolve, with an error naming the link, and a symlinked path inside its clone is skipped with a warning naming it. (Your own project's
`.ai-rulez/` may use symlinks that stay inside the project; see [Configuration](configuration.md#symlinks-in-content).)
Include URLs are classified as git for `http(s)://`, `file://`, `ssh://`, `git://` and `user@host:path` (`http://` and `git://` are then rejected);
anything else is a local path. Remote includes are cached under `~/.cache/ai-rulez/includes/<name>-<hash of the
URL>` with mode `0700`.

### Machine-local includes and offline runs

- `ai-rulez include add <name> <source> --local` (and the MCP `add_include` with `local: true`) writes
  the include to your gitignored `config.local.*` overlay instead of the shared config, so a personal
  checkout can be included without affecting teammates. Overlay includes merge by name with the shared
  list; `remove = true` hides a shared include on your machine. See
  [Local Configuration](local-overrides.md).
- `ai-rulez generate --no-fetch` skips network fetches and uses cached content for remote includes.
  CRUD commands that validate a change (including the `--local` ones) read remote includes from cache
  only.

### Pinning remote includes

A git include without a `ref` follows the repository's default branch, so two machines can generate different output from the same config. `ai-rulez lock` records each remote include's resolved commit and a content digest in `.ai-rulez/ai-rulez.lock`; later `generate` runs fetch that commit and verify the digest. `generate --locked` (CI) fails on a source the lock does not cover, and `--frozen` never uses the network. `ai-rulez.lock` also pins your own authored content (`sha256` per rule, skill, hook and role, plus the generated outputs); see the [Lock file](lockfile.md) and the [Lock Command](cli.md#lock-command).

## Include Priority

Resolution starts from your local content and folds each include in declaration order, using that
include's strategy (default `local-override`, which means *the existing base wins*):

1. Your local configuration has the highest priority.
2. The first include's content becomes part of the base and therefore beats later includes of the same name.
3. A later include only wins where its `merge_strategy` is `include-override`.

```toml
[[includes]]
name = "base"
source = "../base-rules/.ai-rulez"   # loaded first

[[includes]]
name = "team"
source = "../team-rules/.ai-rulez"   # only overrides "base" with merge_strategy = "include-override"
merge_strategy = "include-override"
```

If `base` and `team` both define `rules/security.md`, `base` wins unless `team` sets
`merge_strategy = "include-override"`. Your own `rules/` directory always wins.

## Common Patterns

### Organization-Wide Standards

Create a central repository with baseline rules:

**Repository structure:**

```text
org-standards/
└── .ai-rulez/
    ├── config.toml
    ├── rules/
    │   ├── security.md
    │   ├── code-quality.md
    │   └── git-workflow.md
    └── context/
        └── company-values.md
```

**Each project includes it:**

```toml
[[includes]]
name = "standards"
source = "https://github.com/myorg/standards.git"
path = ".ai-rulez"
```

### Framework-Specific Rules

Create separate includes for each framework:

```text
frameworks/
├── go-backend/
│   └── .ai-rulez/
│       ├── config.toml
│       └── rules/
│           ├── project-layout.md
│           ├── error-handling.md
│           └── testing.md
├── react-frontend/
│   └── .ai-rulez/
│       └── rules/
│           ├── component-guidelines.md
│           ├── hooks-patterns.md
│           └── styling.md
└── python-ml/
    └── .ai-rulez/
        └── rules/
            ├── numpy-conventions.md
            └── ml-best-practices.md
```

**Your project uses them:**

```toml
presets = ["claude", "cursor"]

[[includes]]
name = "go-backend"
source = "../../frameworks/go-backend/.ai-rulez"

[[includes]]
name = "react-frontend"
source = "../../frameworks/react-frontend/.ai-rulez"

[profiles]
backend = ["backend"]
frontend = ["frontend"]
full = ["backend", "frontend"]
```

### Monorepo with Shared and Team-Specific Rules

**Repository structure:**

```text
monorepo/
├── shared-rules/.ai-rulez/     # Used by all teams
├── backend-team/
│   └── .ai-rulez/              # Includes shared + backend-specific
├── frontend-team/
│   └── .ai-rulez/              # Includes shared + frontend-specific
└── mobile-team/
    └── .ai-rulez/              # Includes shared + mobile-specific
```

**`backend-team/.ai-rulez/config.toml`:**

```toml
version = "4.0"
name = "backend-api"

includes = [
  { name = "shared-rules", source = "../shared-rules/.ai-rulez" }
]

presets = ["claude", "cursor"]

[profiles]
default = []
```

## Flat Composition

Includes do not nest. To combine several layers, list each one in the project's own `includes`:

```text
org-base/.ai-rulez/       rules/security.md
go-framework/.ai-rulez/   rules/testing.md
team-standards/.ai-rulez/ rules/review.md
my-project/.ai-rulez/     config.toml with one [[includes]] entry per directory above
```

An `includes` list inside `go-framework`'s own `config.toml` is ignored when `my-project` includes it,
so `org-base` must be listed in `my-project` too. Includes fold in declaration order (see
[Include Priority](#include-priority)).

## Collision Handling

When includes define the same file:

```text
shared-rules/.ai-rulez/rules/testing.md
go-framework/.ai-rulez/rules/testing.md
my-project/.ai-rulez/rules/testing.md
```

Under the default `local-override` strategy the earliest source wins, and your own content beats every
include:

1. `my-project/rules/testing.md` (your project content, highest priority)
2. `shared-rules/rules/testing.md` (first include becomes part of the base)
3. `go-framework/rules/testing.md` (only wins if it sets `merge_strategy = "include-override"`)

The losing copy is dropped; `generate` and `validate` log a `Duplicate rule collapsed` warning naming
the kept and dropped paths (see [Deduplication by Name](configuration.md#deduplication-by-name)). With
`merge_strategy = "error"`, the conflict fails generation instead.

## Best Practices

### Organize by Specificity

```text
org-wide-rules/           Applies to everything
team-rules/               Team-specific
framework-rules/          Technology-specific
project-rules/            Project-specific
```

### Use Clear Naming

Good:

- `org-standards/.ai-rulez`
- `react-best-practices/.ai-rulez`
- `backend-security/.ai-rulez`

Bad:

- `rules/.ai-rulez` (ambiguous)
- `base/.ai-rulez` (unclear scope)

### Document Includes

Add comments to your config explaining why includes are needed:

```toml
version = "4.0"
name = "my-backend"

# Organization-wide coding standards
# Go-specific conventions
# Backend team standards
includes = [
  { name = "org-standards", source = "../org-standards/.ai-rulez" },
  { name = "go-guidelines", source = "../go-guidelines/.ai-rulez" },
  { name = "backend-team", source = "../backend-team/.ai-rulez" }
]

presets = ["claude", "cursor"]
```

### Keep Includes Focused

Each include should have a single purpose:

```text
Good:
security-policies/       Rules for security
error-handling/          Rules for error handling
code-style/              Rules for code style

Bad:
everything/              Security, errors, style, testing, etc.
```

### Version Your Includes

Tag releases and reference specific versions:

```bash
git tag v1.0.0 org-standards/
```

```toml
[[includes]]
name = "standards"
source = "https://github.com/org/standards/.ai-rulez"
ref = "v1.0.0"
```

## Troubleshooting

### Include Path Not Found

```bash
ls -la ../shared-rules/.ai-rulez/config.toml
```

```toml
# Or point at an absolute path
[[includes]]
name = "shared"
source = "/path/to/shared-rules/.ai-rulez"
```

### Include Skipped or Nested Includes Ignored

Includes are not recursive, so cycles cannot occur. Give each include a unique name: duplicate names are
not rejected and are processed independently.

If an include's content is missing from the output, run `ai-rulez validate --verbose`: a failed
include is logged as `Failed to process include` and skipped (an error under `--locked`, `--frozen` or an enforced
lock), and a `local_override` path that does not exist skips the include silently.

### Conflicting Rules

Use project-level rules to override, or change which include wins with `merge_strategy`:

```toml
[[includes]]
name = "stricter"
source = "../stricter-rules/.ai-rulez"   # loads first, wins by default

[[includes]]
name = "lenient"
source = "../lenient-rules/.ai-rulez"
merge_strategy = "include-override"       # makes this one win instead
```

Your `.ai-rulez/rules/security.md` overrides every include.

### Content Not Merging

```bash
cat .ai-rulez/config.toml | grep includes
ls -la ../shared-rules/.ai-rulez/
ai-rulez validate --verbose
```

## Migration Path

If you're currently using separate configurations:

1. **Extract common rules** into a shared include:

   ```bash
   mkdir -p ../shared-rules/.ai-rulez/rules
   # Move common rules there
   ```

2. **Add include** to your config:

   ```toml
   [[includes]]
   name = "shared"
   source = "../shared-rules/.ai-rulez"
   ```

3. **Regenerate** and test:

   ```bash
   ai-rulez validate
   ai-rulez generate
   ```

4. **Commit** the include:

   ```bash
   git add ../shared-rules/
   git commit -m "chore: extract shared rules"
   ```

## Next Steps

- **[Configuration Reference](configuration.md)**: Advanced config options
- **[Domains & Profiles](domains.md)**: Team organization
- **[Quick Start](quick-start.md)**: Getting started
