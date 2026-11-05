# Installed skills

An `[[installed_skills]]` entry pulls one or more [Agent Skills](https://agentskills.io) from a git repository or a
local directory and writes them into every harness's skills folder, as if they were your own. Use it to ship a
library's usage guide with the library, or to share team skills across repositories without a full
[include](includes.md).

## Example

```bash
ai-rulez skill install ai-rulez --source Goldziher/ai-rulez --trust warn
ai-rulez generate
ai-rulez lock
```

`skill install` adds the entry to `.ai-rulez/config.toml`:

```toml
[[installed_skills]]
name = 'ai-rulez'
source = 'Goldziher/ai-rulez'
trust = 'warn'
```

`trust = 'warn'` is needed for this skill because its text runs `npx ai-rulez@latest`, which the security scan
reports as an unpinned package run (`AR021`). Without it, `generate` still writes the skill, but `ai-rulez lock`
leaves it out of the served-skill pins and exits 3 (see [Security](#security)).

`generate` fetches `skills/ai-rulez/` from the repository and writes it, with its bundled files, to each preset's
skills folder. With `presets = ["claude", "copilot"]`:

```text
.agents/skills/ai-rulez/SKILL.md
.agents/skills/ai-rulez/references/cli-reference.md
...
.claude/skills/ai-rulez/SKILL.md
.claude/skills/ai-rulez/references/cli-reference.md
...
```

`ai-rulez lock` pins the commit (see [Pinning](#pinning)).

!!! tip "No `.ai-rulez/` folder needed"
    A project that keeps hand-written `CLAUDE.md` and `AGENTS.md` can declare `[[installed_skills]]` in a root
    `ai-rulez.toml` (`ai-rulez init --minimal`). See [Config without a .ai-rulez folder](config-without-folder.md)
    and [Managing only remote skills](managing-remote-skills.md).

## Installed skills vs skill sources

ai-rulez reaches skills in another repository in two ways:

| | Installed skill (`[[installed_skills]]`) | Skill source (`[[skill_sources]]`) |
| --- | --- | --- |
| What happens | The skill is fetched and written into the harness skill folders | The repository is served over MCP; nothing is written |
| How the agent sees it | Like a local skill, listed up front | Found on demand with `find_skill` |
| Scoped by | Profiles, roles, `domain`, `profiles` | `domain`, the [skills server](mcp-server.md#dynamic-skill-loading) and a role's `delivery` |
| Use it for | A few skills the agent should always have | A large catalog the agent should search instead of paying its tokens up front |

Both are pinned in `ai-rulez.lock`, scanned before use, and accept `include`, `exclude`, `name_prefix` and `trust`.
`ai-rulez skill install`, `list`, `update` and `remove` manage installed skills; skill sources are declared by hand
in `config.toml` (they use `url` instead of `source`). See [Skill sources](mcp-server.md#skill-sources).

## Several skills from one repository

When `path` names a directory of skill directories, one entry installs several skills. `include` and `exclude` match
directory names; `name_prefix` keeps the names apart from your own:

```bash
ai-rulez skill install vendor --source ./vendor/acme-skills --include 'db-*' --name-prefix acme-
```

```toml
[[installed_skills]]
name = 'vendor'
source = './vendor/acme-skills'
include = ['db-*']
name_prefix = 'acme-'
```

With `vendor/acme-skills/skills/{db-migrate,db-seed,api-style}/SKILL.md` and `presets = ["cursor", "codex"]`,
`generate` writes:

```text
.agents/skills/acme-db-migrate/SKILL.md
.agents/skills/acme-db-migrate/cli.md
.agents/skills/acme-db-migrate/references/notes.md
.agents/skills/acme-db-seed/SKILL.md
.agents/skills/acme-db-seed/references/notes.md
```

`name` is then only the entry's label: `skill list` and `skill remove` use it.

## Configuration

```toml
[[installed_skills]]
name = "ai-rulez"
source = "https://github.com/Goldziher/ai-rulez"
version = "^5"                # newest tag in this range, moved by `ai-rulez update`

[[installed_skills]]
name = "my-lib"
source = "acme/my-lib"
path = "libs/my-lib/skill"    # default: skills/<name>
ref = "v2.0.1"
domain = "backend"            # selected by profiles and roles like backend content
trust = "warn"
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `name` | string | required | Entry name. For a single-skill entry, also the skill name and the default directory `skills/<name>` |
| `source` | string | required | Where the skills come from; see [Source forms](#source-forms) |
| `path` | string | `skills/<name>`, or `skills` with `include`/`exclude` | Directory in the source holding the skill, or a directory of skills |
| `ref` | string | default branch | Branch, tag or commit |
| `version` | string | | Semver range of the source's tags (`^1.2`, `~2.1.0`), instead of `ref` |
| `tag_prefix` | string | | With `version`: only tags with this prefix (`v`, `skills/`) |
| `include_prerelease` | bool | `false` | With `version`: let pre-release tags match |
| `min_release_age` | string | | With `version`: skip tags younger than this (`7d`, `12h`) |
| `include` | list of globs | all | Keep skill directories whose name matches |
| `exclude` | list of globs | none | Drop matching directories; wins over `include` |
| `name_prefix` | string | | Prepended to every skill name and output directory (`acme-db-migrate/`) |
| `domain` | string | root | Put the skills in a [domain](domains.md) |
| `profiles` | list | all | Include the skills only in these profiles |
| `resources` | list of patterns | whole directory | What is bundled besides `references/`, `scripts/`, `assets/`; see [below](#what-is-bundled-with-a-skill) |
| `local_override` | string | | A local checkout used instead of the source; see [Local development](#local-development) |
| `trust` | `error` \| `warn` | unset | Scan level of the imported skills; see [Security](#security) |

An entry is a single skill when the directory at `path` holds a `SKILL.md`, and a directory of skills otherwise.

## Source forms

| Form | Example | Notes |
| --- | --- | --- |
| Git URL | `https://github.com/acme/skills`, `git@github.com:acme/skills.git` | `https` and `ssh`; `http://` is refused |
| GitHub shorthand | `acme/skills`, `github:acme/skills` | Recorded in the lock as `https://github.com/acme/skills`. A bare `acme/skills` that names an existing directory in the project is a local path; `github:` is always GitHub |
| `file://` URL | `file:///srv/skills` | A git repository is cloned; any other directory is read in place |
| Local directory | `./vendor/skills`, `/abs/path` | Read in place |
| Home directory | `~/skills` | Local, expanded to your home directory (`~user/` is not supported) |

A local path in the committed config must resolve inside the project, symlinks included. A path outside it works
only from `config.local.toml` (`skill install --local`) or from the [user config](user-scope.md):

```bash
ai-rulez skill install lint --source ~/skills --local
```

Set `AI_RULEZ_ALLOW_FILE_URLS=1` to let a `file://` source point outside the project.

Never put a token in the URL: `skill install` refuses `https://user:token@host/...`, and `validate` reports a
committed one as `AR035`. Authenticate with your git credential helper or `AI_RULEZ_GIT_TOKEN`.

## How skills are found

Below the entry's `path`, every directory holding a `SKILL.md` is a skill, up to six levels deep. Hidden directories,
`node_modules` and symlinks are skipped. If two skills share a directory name, the shallower one wins and the other
is skipped with a warning. One entry resolves to at most 200 skills. A skill whose `SKILL.md` cannot be read is
skipped with a warning, and a missing `description` only warns (the skill name is used instead); set it anyway, since
the description is how an agent decides when to load the skill.

When `path` is not set and nothing is at `skills/<name>`, the source is searched where harnesses keep skills: a
`SKILL.md` at the root (one skill named after the entry), then `skills/`, `.claude/skills/`, `.agents/skills/`,
`.codex/skills/`, `.cursor/skills/`, `.gemini/skills/`, `.github/skills/` and `.opencode/skills/`, in that order.
So a repository that keeps its skills in `.claude/skills/review/` installs with:

```bash
ai-rulez skill install review --source acme/tools
```

A local skill with the same name always wins; the installed one is skipped with
`Installed skill name conflicts with local skill, skipping`. Use `name_prefix` to keep both.

## What is bundled with a skill

An installed skill arrives whole: every file in its directory is bundled, including documents beside `SKILL.md`
(`cli.md`) and folders of any name (`rules/`, `templates/`). `generate` lists them in a `Resources` section of the
generated `SKILL.md` (documents under References, scripts under Scripts, the rest under Assets) and writes each at
its relative path, so links in `SKILL.md` keep working:

```markdown
## Resources

This skill bundles supporting files. Read them on demand when the task calls for them — don't bulk-load.

### References

- [`cli.md`](cli.md) — cli doc
- [`references/notes.md`](references/notes.md) — ref
```

Left out by default: `evals/`, `agents/` (tool display metadata such as `openai.yml`) and hidden files. Git-ignored
files and the [`bundle_exclude`](configuration.md#bundle_exclude) defaults (`node_modules`, `.venv`, ...) are
skipped as for any skill. Your own skills under `.ai-rulez/skills/` keep the canonical layout; only installed
skills are bundled whole.

`resources` replaces the default. A pattern is a name, a path or a glob; a bare name matches at any depth, and `!`
removes what an earlier pattern added. `references/`, `scripts/` and `assets/` are always bundled.

```toml
resources = ["*", "!docs", "!templates", "!evals", "!agents", "!.*"]  # everything except docs/ and templates/
resources = ["rules"]                                                  # only rules/ besides the canonical folders
resources = ["*"]                                                      # also evals/, agents/ and hidden files
```

Limits that `resources` cannot lift:

- Credential-like files (private keys, `*.pem`, `*.key`, `.env.*`, `credentials.json`, Terraform state, ...),
  symlinks and special files are never bundled; each skipped credential file is named in a warning.
- A file over 8 MiB is skipped with a warning.
- One skill may bundle at most 2000 files and 16 MiB; one entry at most 20000 files and 64 MiB. Past that, the entry
  fails with an error. Narrow it with `include`/`exclude` or `resources`.

## Pinning

`ref` defaults to the moving default branch. `ai-rulez lock` records the resolved commit and a content digest in
`ai-rulez.lock`:

```toml
[[skill]]
name = 'ai-rulez'
source = 'https://github.com/Goldziher/ai-rulez'
path = 'skills/ai-rulez'
commit = '99407aa72968580a77a949258e399686acfa31d9'
digest = 'sha256:af3882d30acc484af8ff29699eb914891317ea06ec76440ec158982298e763cc'
```

Later runs fetch exactly that commit and fail if the content does not match the digest. A skill read from a local
directory is pinned as a `local-skill` item with the directory's tree digest, so editing it makes `lock --check` and
`generate --locked` fail until you run `ai-rulez lock` again.

| Command | Effect |
| --- | --- |
| `ai-rulez lock` | Pin every unpinned entry |
| `ai-rulez skill update [name...]` | Re-pin entries to the current commit of their `ref` |
| `ai-rulez update` | Move `version` entries to the newest allowed tag |
| `ai-rulez generate --locked` | Fail when an entry is missing from the lock or does not match |
| `ai-rulez generate --frozen` | Same, and never use the network |

See [Lock file](lockfile.md).

## Security

Imported skill text is instruction text, so it is scanned for secrets, hidden characters, risky commands and
unpinned package runs ([Security checks](strict-validation.md#security-checks)) at three points:

| When | What blocks |
| --- | --- |
| `ai-rulez lock`, before pinning a new tree | Error findings refuse the pin (exit 2) unless you pass `--accept-findings` |
| `generate` | An error finding stops the run before anything is written. `trust = "error"` makes every finding an error, `trust = "warn"` every finding a warning; unset, [`[lint.security] scan_imports`](strict-validation.md#security-checks) decides (by default each finding keeps its own severity) |
| The [skills server](mcp-server.md#security-scan) and its `[[served]]` lock pins | `error`, or unset: any finding refuses the skill. `warn`: only findings that are errors in their own right. Unset, `[lint.security] scan_imports = "warn"` lowers the default to `warn` |

When the served view refuses a skill, `lock` still writes the lock, leaves that skill unpinned and exits 3
(`1 served skill(s) were left unpinned because the security scan refuses them`). Fix the finding, set
`trust = "warn"`, or pass `--refuse-findings` to fail without writing instead.

## Local development

`local_override` points an entry at a local checkout while you work on the skill:

```toml
# .ai-rulez/config.local.toml
[[installed_skills]]
name = "my-lib"
source = "acme/my-lib"
local_override = "../my-lib"
```

The override is the checkout root; `path` (default `skills/<name>`) is resolved inside it. If that directory has no
`SKILL.md`, the skill is skipped, not fetched from the source. Set it in `config.local.toml`: a `local_override` in
the committed config fails whenever the lock is enforced (an `ai-rulez.lock` exists, or under `--locked` or
`--frozen`).

## Commands

| Command | Purpose |
| --- | --- |
| `ai-rulez skill install <name> --source <src> [flags]` | Add an entry. Every key above has a flag (`--path`, `--ref`, `--version`, `--include`, `--name-prefix`, `--resources`, `--trust`, `--domain`, `--profile`, `--local-override`, ...) |
| `ai-rulez skill install ... --local` | Write the entry to the gitignored `config.local.toml` instead |
| `ai-rulez skill list [--format json]` | List entries |
| `ai-rulez skill remove <name> [--yes]` | Remove an entry. With `--local`, a shared entry is hidden on this machine (`remove = true` in the overlay) |
| `ai-rulez skill update [name...]` | Re-pin in `ai-rulez.lock` |

```text
$ ai-rulez skill list
Installed skills:
  • vendor [local]
    Source: ./vendor/acme-skills
    Path: skills
```

The MCP server offers the same operations as `install_skill` (the same fields as `skill install`, plus `local`),
`uninstall_skill` and `list_installed_skills`. Full flag tables: [CLI reference](cli-reference.md#ai-rulez-skill-install).

## Publishing a skill

To make a skill installable from your repository:

1. Put it at `skills/<name>/SKILL.md`, with `name` and `description` frontmatter (see [Skills](skills.md)).
2. Add `references/`, `scripts/` or `assets/` as needed; any other files beside `SKILL.md` are bundled too.
3. Tag releases so consumers can use `version = "^1"`.

Consumers then run `ai-rulez skill install <name> --source <owner>/<repo>`.

## Compared with includes

| | [Includes](includes.md) | Installed skills |
| --- | --- | --- |
| Imports | Rules, context, skills, agents, commands | Skills only |
| Source layout | A `.ai-rulez/` directory or a bare layout | A directory with `SKILL.md`, or a directory of them |
| Name conflicts | Configurable merge strategy | The local skill wins |
| Domains | Into a chosen domain | Root, or the `domain` key |
| Use for | Sharing a whole governance setup | Adding skills for a library or tool |

To import a whole harness plugin (skills, agents, commands, hooks and MCP servers), see
[Installed plugins](installed-plugins.md).
