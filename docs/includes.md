# Includes

An include pulls rules, context, skills, agents, commands and checks from another directory or git repository into
your project, so an organization's standards are written once and shared by many repositories. Included content is
merged with your own before `generate` renders it, and your own content wins on a name collision.

## Example

```toml
# .ai-rulez/config.toml
version = "5.0"
name = "web"
presets = ["claude", "cursor", "codex"]

[[includes]]
name = "standards"
source = "https://github.com/acme/ai-standards"
ref = "v1.2.0"
include = ["rules", "skills"]
```

The repository holds plain content folders, no configuration needed:

```text
ai-standards/
├── rules/
│   └── security.md
└── skills/
    └── security-review/
        └── SKILL.md
```

```markdown
<!-- ai-standards/rules/security.md -->
---
priority: critical
---

Never hardcode secrets. Validate all user input.
```

```bash
ai-rulez lock       # pin v1.2.0 to its commit
ai-rulez generate
```

The included rule lands in the shared `AGENTS.md` like one of your own:

```markdown
<!-- AGENTS.md -->
# web

## Rules

### security

**Priority:** critical

Never hardcode secrets. Validate all user input.
```

and the skill is written to `.claude/skills/security-review/` and `.agents/skills/security-review/` (Cursor and
Codex). If you add your own `.ai-rulez/rules/security.md`, it replaces the included one.

## Include options

| Key                  | Type   | Default            | Meaning                                                                                       |
| -------------------- | ------ | ------------------ | --------------------------------------------------------------------------------------------- |
| `name`               | string | required           | identifies the include in the lock, the cache and messages                                    |
| `source`             | string | required           | a git URL or a local directory; see [Sources](#sources)                                       |
| `path`               | string | source root        | subdirectory of the source to read (`modules/core`); a missing path fails the load            |
| `ref`                | string | the default branch | git branch, tag or commit                                                                     |
| `version`            | string | none               | git only: a semver range (`^1.2`, `~2.1.0`) resolved against tags, instead of `ref`; see [Version constraints](lockfile.md#version-constraints) |
| `tag_prefix`         | string | none               | with `version`: the tag prefix to match (`v`, `rules-`)                                       |
| `include_prerelease` | bool   | `false`            | with `version`: allow prerelease tags                                                         |
| `min_release_age`    | string | none               | with `version`: skip tags younger than this (`"7d"`)                                          |
| `include`            | list   | every kind         | kinds to import: `rules`, `context`, `skills`, `agents`, `commands`, `checks`                 |
| `install_to`         | string | root               | import the content into this domain instead of the root                                       |
| `merge_strategy`     | string | `local-override`   | who wins a name collision; see [Precedence](#precedence)                                      |
| `local_override`     | string | none               | a local directory used instead of `source`, for developing the include; see below             |
| `format`             | string | ai-rulez layout    | `okf` reads the source as an [OKF bundle](okf.md#okf-bundles-as-includes)                     |

- `include` filters content kinds, not domains: domains of an include are always carried over.
- MCP servers, hooks and other configuration are not imported, and an include's own `includes` are ignored. To
  combine several layers, list each one in your project.
- `local_override` is resolved against the project directory, with `path` appended. If the directory does not exist,
  the include is skipped with an info line; the remote source is not used instead. Because it bypasses the lock, it is
  honored only from `config.local.toml` or when no lock is enforced: `generate --locked`, `--frozen` and an enforced
  lock fail on a `local_override` in the committed config.

`ai-rulez include add|list|remove` edits the list:

```bash
ai-rulez include add standards https://github.com/acme/ai-standards --ref v1.2.0 --include rules,skills
ai-rulez include add mine ../my-rules --local      # into config.local.toml, for your machine only
ai-rulez include list
```

`--local` (and `local: true` on the MCP tool `add_include`) writes the include to `config.local.toml`. Overlay
entries merge with the shared list by `name`, and `remove = true` hides a shared include on your machine; see
[Local configuration](local-overrides.md).

## Sources

| Form                | Example                                                             |
| ------------------- | ------------------------------------------------------------------- |
| HTTPS git           | `https://github.com/acme/ai-standards.git` (`.git` optional)        |
| SSH git             | `git@github.com:acme/ai-standards.git`, `ssh://git@host/acme/repo.git` |
| `git+` prefix       | `git+https://host/acme/repo` (same include and cache as without it) |
| local git repository| `file:///srv/git/ai-standards`                                      |
| local directory     | `./shared/standards`, `vendor/rules`                                |

Plain `http://` and `git://` remotes are rejected. SSH URLs use your git and SSH agent setup, which works for private
and self-hosted servers.

!!! warning "Local paths must stay inside the project"
    A local `source`, `path` or `local_override` in the project's committed config must resolve inside the project,
    symlinks included; otherwise loading stops with an error naming the path. This stops a repository from pulling
    `../victim` or `~/.config` into its generated files. For a directory elsewhere on your machine, declare the include
    in `config.local.toml` (`include add --local`) or in your user config (`generate --user`). The same holds for a
    `file://` URL; set `AI_RULEZ_ALLOW_FILE_URLS=1` in your environment to allow one in a project config.

### Repository structure support

An include source can be laid out two ways, and ai-rulez detects which:

1. **Bare**: the directory has `rules/`, `context/`, `skills/`, `agents/`, `commands/` or `checks/` directly, at the
   root or under `path`. This is the simplest layout for a shared module.

   ```text
   shared-modules/
   └── modules/
       └── core/
           ├── rules/
           └── skills/
   ```

   ```toml
   [[includes]]
   name = "core"
   source = "https://github.com/acme/shared-modules.git"
   path = "modules/core"
   ```

2. **Wrapped**: the content is in a `.ai-rulez/` folder, as in a project that uses ai-rulez itself.

ai-rulez looks for a `.ai-rulez/` folder first (at the source root or under `path`), then for the bare folders.

`ai-rulez include add` with a local path accepts only the wrapped layout. Add a bare local directory by editing
`config.toml`.

## Precedence

Your own content always wins. Includes are folded in the order they are declared, each with its own
`merge_strategy`:

| `merge_strategy`           | On a name collision with what is already merged             |
| -------------------------- | ----------------------------------------------------------- |
| `local-override` (default) | the existing content wins: your project, then earlier includes |
| `include-override`         | this include wins, also over your project content           |
| `error`                    | `generate` fails, naming the include                        |

```toml
[[includes]]
name = "org"
source = "https://github.com/acme/org-rules"      # wins over "team" by default

[[includes]]
name = "team"
source = "https://github.com/acme/team-rules"
merge_strategy = "include-override"               # wins over "org" and over your own files
```

An item that collides across layers (for example a root rule from one include and a domain rule from an
`install_to` include) is collapsed by [name deduplication](configuration.md#deduplication-by-name): root wins over
domains, and `generate` warns with both paths:

```text
WARN  Duplicate rule collapsed name=security kept=shared/standards/rules/security.md dropped=shared/team/rules/security.md
```

## Private repositories

Over SSH, your SSH keys and agent are used. Over HTTPS, set a token:

```bash
export AI_RULEZ_GIT_TOKEN="ghp_..."     # or pass --token on the command line
ai-rulez generate
```

The token is sent only to `github.com` unless you list other hosts in the environment (a project file cannot widen
this):

```bash
export AI_RULEZ_GIT_TOKEN_HOSTS="github.com,gitlab.example.com"
```

The list replaces the default. An include on any other host is fetched without the token, with a warning. The token
is passed to git as a header for that host, so it does not appear in process arguments or in the cached clone's
`.git/config`. Grant it read-only repository access (`repo` on GitHub classic tokens, `read_repository` on GitLab).

Never put credentials in the URL: `include add` refuses `https://user:token@host/...`, and `validate` reports an
already committed credentialed source as `AR035`.

In CI:

```yaml
# GitHub Actions
- name: Generate
  env:
    AI_RULEZ_GIT_TOKEN: ${{ secrets.GIT_TOKEN }}
  run: npx ai-rulez@latest generate --check
```

```yaml
# GitLab CI
generate:
  script:
    - export AI_RULEZ_GIT_TOKEN="$CI_JOB_TOKEN"
    - ai-rulez generate --check
```

## Pinning and caching

A git include without `ref` follows the default branch, so two machines can render different output. `ai-rulez lock`
records each git include's resolved commit and a content digest in `.ai-rulez/ai-rulez.lock`; later runs fetch that
commit and verify the digest. A branch or tag that is not locked is reported as `AR010`.

| Command                        | Behavior                                                                  |
| ------------------------------ | ------------------------------------------------------------------------- |
| `ai-rulez lock`                | pin every include (and your own content) in `ai-rulez.lock`              |
| `ai-rulez update`              | move `version` pins to the newest matching tag                            |
| `ai-rulez generate --locked`   | fail on a source the lock does not cover (use in CI)                      |
| `ai-rulez generate --frozen`   | use only the cache, never the network, and verify it against the lock     |
| `ai-rulez generate --offline`  | use cached content; an include that cannot be resolved is skipped with a warning |

Remote includes are cached under `~/.cache/ai-rulez/includes/<name>-<hash>/` (mode `0700`), one tree per URL, path,
ref and pinned commit, so two checkouts that pin different commits never share a tree. See [Lock file](lockfile.md).

## Errors and safety

- An include that cannot be fetched or merged is an error for `generate`, `generate --check`, `validate`, `doctor`
  and every other command that loads the configuration. An unreachable remote falls back to its cached copy when one
  exists. Only `--offline` turns a missing include into a warning.
- Included content never follows symlinks: a symlinked file or folder is skipped with a warning naming it.

## Patterns

**Organization standards in one repository**, included everywhere:

```toml
[[includes]]
name = "org"
source = "https://github.com/acme/org-standards"
version = "^2.0"
```

**Several layers.** Includes do not nest, so list each layer, most general first:

```toml
[[includes]]
name = "org"
source = "https://github.com/acme/org-standards"

[[includes]]
name = "go"
source = "https://github.com/acme/go-guidelines"

[[includes]]
name = "payments-team"
source = "https://github.com/acme/payments-rules"
install_to = "payments"     # imported as the payments domain
```

**A monorepo** keeps shared content in a folder of the repository and includes it from each project's config, or
uses [`[[scopes]]`](monorepo.md) to render nested outputs from one configuration.

**Developing an include** next to its consumers: point `local_override` at your checkout in `config.local.toml`, so
the committed config keeps its pinned source:

```toml
# .ai-rulez/config.local.toml
[[includes]]
name = "org"
local_override = "../org-standards"
```

`generate` then refuses to write tracked shared files (such as `AGENTS.md`) that the local override would change.
Pass `--allow-local-drift` to write them while you test, or `--no-local` to render the committed view. See
[Local configuration](local-overrides.md).

## Troubleshooting

| Symptom                                            | Cause and fix                                                                                   |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `local path ... is outside the project`            | Move the include to `config.local.toml` or the user config, or copy the content into the project. |
| `conflict detected in rules between base and include` | An include with `merge_strategy = "error"` has an item with an existing name. Rename one.     |
| Included item is missing                           | Your own item, or an earlier include, has the same name; or `include` leaves the kind out.      |
| `Directory must contain .ai-rulez/ subdirectory`   | `include add` with a bare local directory; add the entry to `config.toml` by hand.               |
| Output differs between machines                    | The include follows a branch. Run `ai-rulez lock` and commit `ai-rulez.lock`.                    |
| Include ignored, no error                          | A `local_override` that names a missing directory skips the include; run with `--debug`.        |
