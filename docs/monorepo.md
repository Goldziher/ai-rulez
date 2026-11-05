# Monorepos and large projects

A large repository needs each agent to see the instructions for the code it is working on, and not much more.
ai-rulez gives you three tools, which you can combine:

| Tool | Use it when | What you get |
| --- | --- | --- |
| [Domains and profiles](domains.md) | Teams or services share one repository and one configuration | One set of root files, with the content of the selected domains |
| [`[[scopes]]`](#scopes) | Packages need their own instructions, loaded when an agent works there | Root files plus `<package>/AGENTS.md`, `<package>/CLAUDE.md`, ... and path-scoped rule files |
| [Nested configurations](#nested-configurations) | Packages are owned separately and keep their own `.ai-rulez/` | One independent project per directory, generated with `--recursive` |

<p align="center">
  <img src="assets/monorepo.gif" alt="ai-rulez generate --recursive over two nested roots, recursive validation, and a token budget check" width="820" />
</p>

## Example

A repository with shared rules, an `api` and a `web` domain, and one scope per package:

```text
.ai-rulez/
  config.toml
  rules/security.md                    # every package
  domains/api/rules/errors.md          # packages/api only
  domains/api/rules/go-style.md        # paths: ["**/*.go"]
  domains/web/rules/components.md      # packages/web only
packages/api/
packages/web/
```

```toml
# .ai-rulez/config.toml
version = "5.0"
name = "acme"
presets = ["claude", "cursor", "codex"]

default = "api"

[profiles]
api = ["api"]
web = ["web"]

[[scopes]]
path = "packages/api"
profile = "api"                  # presets default to codex and claude

[[scopes]]
path = "packages/web"
profile = "web"
presets = ["claude", "cursor"]
```

`ai-rulez generate` writes:

```text
AGENTS.md                                  # root: security
CLAUDE.md                                  # @AGENTS.md
packages/api/AGENTS.md                     # errors, go-style
packages/api/CLAUDE.md                     # @AGENTS.md
packages/web/AGENTS.md                     # components
packages/web/CLAUDE.md
.claude/rules/packages-api/go-style.md     # paths: [packages/api/**/*.go]
...
```

Each scope file holds only the scope's own content: tools load a nested `AGENTS.md` or `CLAUDE.md` on top of the root
file, so repeating the root rules would load them twice.

## Domains and profiles

Put cross-cutting content (architecture, security, git workflow) at the root of `.ai-rulez/`, and team- or
service-specific content in `domains/<name>/`. A profile selects domains:

```toml
default = "full"

[profiles]
full = ["backend", "frontend", "qa"]
backend = ["backend", "qa"]
frontend = ["frontend", "qa"]
```

```bash
ai-rulez generate --profile backend      # root content plus backend and qa
```

See [Domains and profiles](domains.md) for the layout and [Roles](roles.md) to select content per kind of task
rather than per team.

## Scopes

A `[[scopes]]` entry writes a second set of root files in a subdirectory, from its own profile:

| Key | Default | Meaning |
| --- | --- | --- |
| `name` | | Label used in messages |
| `path` | required | Directory relative to the project root; no glob characters |
| `profile` | the active profile | Profile whose domains the scope renders |
| `presets` | `["codex", "claude"]` | Presets that write files in the scope |

A domain the root output already contains is not repeated in a scope, and a scope left with nothing writes no files.
See [`scopes`](configuration.md#scopes) for the full reference.

### Scoped rule files

Root files (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, ...) are written in the scope directory, but rule files of a scope
go to the **root** rules folder, with the scope path as a qualifier and glob prefix. Not every tool reads a nested
rules folder (Copilot reads only the repository root), so this is the one placement that every preset with a rules
folder reads.

| Preset | Scoped rule file for `packages/api`, rule `style` |
| --- | --- |
| Claude | `.claude/rules/packages-api/style.md` |
| Cursor | `.cursor/rules/packages-api/style.mdc` |
| Copilot | `.github/instructions/packages-api/style.instructions.md` |
| Devin, Cline, Junie | `<rules dir>/packages-api--style.md` |
| Antigravity | `.agents/rules/packages-api--style.md` |

- Globs are relative to the scope: `paths: ["**/*.go"]` becomes `packages/api/**/*.go`, and a rule without globs
  (or with only negated globs) gets `packages/api/**`.
- A glob that leaves the scope with `..` skips that rule for the scope, with a warning.
- `auto` and `manual` rules keep their mode, so they are not limited to the scope; `generate` warns once per scope.
- With `[rules] mode = "inline"`, only path-scoped items move to the root folder; the rest stays in the scope's root
  file.
- Two rules that map to the same file get a `-<6 hex>` suffix with a warning (see [File names](rules.md#file-names)).
  Two scopes that produce the same qualifier (`packages/api` and `packages-api`) fail; rename one.

Limitations:

- Copilot (`.github/copilot-instructions.md`) and Junie read their root instruction file at the repository root
  only, so content left inline in a scope's copy never reaches them, and `generate` warns. Use path-scoped rules or
  `[rules] mode = "split"` for those presets.
- Custom presets whose rules output is not split keep a scope's rules in the scope directory, with a warning.

## Nested configurations

When a package keeps its own `.ai-rulez/` (or `.agents/ai-rulez.toml`), it is an independent project. Process every
configuration below the current directory with `--recursive`:

```text
$ ai-rulez generate --recursive
...
✅ Total: Generated 18 file(s) from 2 config(s)

$ ai-rulez generate --check --recursive
[1/2] .ai-rulez/config.toml (47ms)
[2/2] services/billing/.ai-rulez/config.toml (42ms)
INFO  Generated files are up to date roots=2

$ ai-rulez validate --recursive
[2/2] services/billing/.ai-rulez/config.toml (79ms)
validation: no findings in 2 root(s)
```

```toml
# services/billing/.ai-rulez/config.toml
version = "5.0"
name = "billing"
presets = ["claude", "gemini"]
```

- Roots are processed concurrently, one worker per CPU. `generate --check --recursive` and `validate --recursive`
  report in path order, so output and exit code are the same on every run; `--quiet` and `--format json` silence
  the `[n/N]` progress lines.
- A root that authors a plugin or marketplace and configures no preset is built as a bundle, as
  `generate --plugin` would, and skipped by `generate --check --recursive`; check it with `verify --plugin`. See
  [Authoring plugins](plugins.md).
- Use `--debug` to see where time goes on a large repository.

### The shared `.gitignore` block

With `gitignore = true`, a recursive run writes one managed block in the `.gitignore` at the top of each git
repository, holding the patterns of every root inside it (`svc/CLAUDE.md`, `svc/.claude/skills/`, ...).
`generate --recursive --check` verifies each block once.

- A nested directory with its own `.git` gets its own block; a repository whose root sets `gitignore = false` is
  never written for other roots.
- The root's own entries are unanchored (`AGENTS.md`, `CLAUDE.md`), so they also match nested roots' files.
- To commit generated files, list them in each root's [`gitignore_exclude`](configuration.md#gitignore_exclude),
  for example `gitignore_exclude = ["**/AGENTS.md", "**/CLAUDE.md", ".claude/"]`. Each root applies its list to its
  own outputs only.

## Nested instructions over MCP

Packages often already hold `AGENTS.md`, `CLAUDE.md`, `.claude/`, `.agents/` or `skills/`, and a harness loads them
only once it works in that directory. An agent planning at the root, or changing several packages, can ask the MCP
server what applies to a path:

```bash
ai-rulez mcp --serve-skills --discover-skills
```

`find_skill`, `find_context` and `load_context` then take a `path` or `scope`; the nearest scope wins a skill name.
The index reads the files that exist, so it needs no `generate` run. It is cached between runs
(`[mcp] index_cache`), refreshed while the server runs, and an agent can add a skill to a scope with `create_skill`
when `allow_write` is on. See:

- [Monorepos, scopes and nested context](mcp-server.md#monorepos-scopes-and-nested-context)
- [The index cache](mcp-server.md#the-index-cache)
- [Live refresh of the index](mcp-server.md#live-refresh-of-the-index)
- [Writing a skill into a scope](mcp-server.md#writing-a-skill-into-a-scope)

## See also

- [AGENTS.md and .agents/skills](agents-md.md#scopes-local-files-and-hashes): what a scope's `AGENTS.md` contains per
  preset
- [Glob-scoped rules and AGENTS.md](rules.md#glob-scoped-rules-and-agentsmd)
