# Enabling the MCP Server

The `ai-rulez` MCP (Model Context Protocol) server allows your AI assistant to programmatically and safely interact with your `.ai-rulez/` configuration. Since V4 uses a file-based approach with inline MCP configuration, you define servers directly in `config.toml`, and the MCP server provides read, CRUD, validation, and generation tools for AI assistants.

**You do not need to start the server manually.** Your AI assistant will start it automatically based on the configuration you provide.

---

## Configuration Examples

To enable the server, add one of the following snippets to your AI assistant's configuration file (e.g., Cursor's `settings.json`), or define it inline in `.ai-rulez/config.toml`. The top-level key (`mcpServers` below) differs between clients; check your assistant's documentation.

### Using `npx` (Recommended for Node.js users)

This method ensures you are always using the latest version of `ai-rulez` without needing to install it globally.

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "npx",
      "args": ["-y", "ai-rulez@latest", "mcp"]
    }
  }
}
```

### Using `uvx` (Recommended for Python users)

This method uses `uvx` to run `ai-rulez` in an ephemeral environment.

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "uvx",
      "args": ["ai-rulez", "mcp"]
    }
  }
}
```

### Using a Local Go Installation

If you have installed `ai-rulez` locally with `go install`.

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "ai-rulez",
      "args": ["mcp"]
    }
  }
}
```

### Inline in `config.toml` (V4 Recommended)

V4 supports defining MCP servers directly in your `.ai-rulez/config.toml`:

```toml
[[mcp_servers]]
name = "ai-rulez"
command = "npx"
args = ["-y", "ai-rulez@latest", "mcp"]

[[mcp_servers]]
name = "grafana"
command = "uvx"
args = ["mcp-grafana"]
env = { GRAFANA_URL = "http://localhost:3000", GRAFANA_SERVICE_ACCOUNT_TOKEN = "${GRAFANA_SERVICE_ACCOUNT_TOKEN}" }

[[mcp_servers]]
name = "remote-api"
transport = "http"
url = "https://mcp.example.com/mcp"
headers = { Authorization = "Bearer ${REMOTE_API_TOKEN}" }
```

MCP env and header values may contain `${VAR}` placeholders. `ai-rulez generate` resolves them from repeated
`--env KEY=VALUE` flags, process environment variables, then dotenv files. Placeholder names must
match `[A-Za-z_][A-Za-z0-9_]*`. By default, `.env` is loaded from the generation base directory. If
any `--env-file PATH` flags are supplied, the default `.env` is not loaded; multiple files are merged
in flag order, with later files winning. Generation fails if a placeholder cannot be resolved.

### Generated Self-Entry (`[mcp] self_server`)

Instead of declaring the ai-rulez server yourself, let `generate` add it to the project `.mcp.json`,
pinned to the version of the ai-rulez binary that ran it:

```toml
[mcp]
self_server = true
```

The entry is merged into an existing `.mcp.json`, so hand-authored servers survive, and
`.claude/settings.json` is not touched (Claude Code reads project MCP servers from `.mcp.json` only). See [Configuration: `mcp`](configuration.md#mcp) for
`self_server_version`, `self_server_command`, and the interaction with `[[mcp_servers]]`.

Generated MCP config files contain resolved values. If a value came from a placeholder, or if an env
key contains `TOKEN`, `SECRET`, `PASSWORD`, `KEY`, or `CREDENTIAL`, generation fails before writing
unless `.mcp.json`, `.gemini/settings.json`, `.agents/settings.json`,
`opencode.json`, or a scoped variant is gitignored or covered by planned `--gitignore` patterns.
Secret-bearing values are redacted before source-hash calculation, not from generated MCP config
files. Any generated file that contains a resolved secret value is written with mode `0600`.

Servers can also be defined per machine in the [`config.local.*` overlay](local-overrides.md), which
keeps credentials out of the shared config.

Set `enabled = false` on an inline `[[mcp_servers]]` entry to skip it in generated MCP outputs.

---

## Server Capabilities

When enabled, the MCP server provides your AI assistant with access to your configuration and CRUD operations. The server supports:

- **Read Configuration**: Inspect rules, context, skills, profiles, and presets
- **Generate Outputs**: Programmatically trigger generation of tool-specific files. There is no terminal over MCP, so
  the hook, MCP, env, plugin and allow-rule commands the run newly wrote are returned as `new_commands` (and written to
  stderr); an unchanged run omits the field. See [Configuration](configuration.md#checks-generate-runs-before-it-writes)
- **Validate Configuration**: Check configuration validity and report errors
- **CRUD Operations**: Create, read, update, and delete domains, rules, context, skills, includes, and profiles

The MCP server enables AI assistants to:

1. **Understand your setup** by reading configuration and content files
2. **Modify configuration** programmatically via CRUD tools
3. **Generate outputs** after changes
4. **Validate changes** before committing

This approach ensures your configuration remains auditable and version-controlled, while allowing AI assistants to help you manage it efficiently.

## Serving Skills (`--serve-skills`)

`ai-rulez mcp --serve-skills` starts a different, **read-only** server for consumers of skills rather
than authors of configuration. It implements the MCP [Skills extension](https://modelcontextprotocol.io/seps/2640-skills-extension)
(`io.modelcontextprotocol/skills`, SEP-2640, status *Final*, verified 2026-10-04), so a client that
supports MCP but not a local skills directory can discover and load the skills of a profile with no
files on disk. The authoring tools (create, update, delete, generate, ...) are **not registered** in
this mode. It never writes `.claude/settings.json`, so `--role` does not touch the role's `skillOverrides`
(`generate --role` does, and restores the hand-written values it replaced).

```bash
ai-rulez mcp --serve-skills --profile backend
ai-rulez mcp --serve-skills --profile backend --targets claude --domain api --deny 'internal-*'
```

| Flag | Meaning |
| ---- | ------- |
| `--profile` | Profile whose skills are served. Default: the configured default profile. A profile is the role: it selects the domains whose skills apply. |
| `--targets` | Preset whose rendering is served (its frontmatter dialect and placement rules). Default: the first configured preset that produces skills. |
| `--domain` | Keep only skills of these domains (repeatable); `root` selects skills owned by no domain. A name that is not a domain of the project is an error at start. |
| `--allow` / `--deny` | Glob patterns on the skill name. `--allow` keeps only matching skills; `--deny` always wins. |

The skill set is rendered once at start-up, in memory, by the same code as `generate`, so every
`SKILL.md` and supporting file is byte-identical to what `generate` writes for the same profile and
preset. Nothing is written. Edits are picked up by [live reload](#live-reload).
When the filters remove every skill, the start-up warning names each filter and how many skills it leaves.

### What is exposed

- **Resources**: every file of a skill as `skill://<name>/<path>` (`skill://pdf-processing/SKILL.md`,
  `skill://pdf-processing/references/FORMS.md`), readable with `resources/read`. `_meta` carries
  `io.modelcontextprotocol.skills/digest`.
- **`skills/list`** and **`skills/get`** (JSON-RPC methods defined by the extension): the entry of a
  skill is `{uri, frontmatter, resources: [{uri, digest, size}]}` with `sha256:<hex>` digests, so a
  client can verify what it loads. An unknown URI returns `-32602`. The server declares
  `capabilities.extensions["io.modelcontextprotocol/skills"]` and the `resources` capability.
  The server also implements the extension's optional `resources/directory/read` and declares
  `directoryRead: true`: given `skill://<name>` or one of its subdirectories (no trailing slash), it returns the
  direct children as resources (files with their `mimeType` and size, subdirectories as `inode/directory`), and
  `-32602` for anything that is not such a directory. It is read-only and lists only files `resources/read` serves.
- **Tools** for clients that only speak tools, all annotated read-only: `search_skills(query, limit, domain)`
  ranks by name, keywords (frontmatter `keywords`), description and domain, lexically and
  deterministically (an empty query lists everything); `get_skill(name)` returns `SKILL.md` plus
  provenance and per-file digests; `read_skill_file(uri)` returns a supporting file.

### Provenance

Search and `get_skill` results carry `digest` (a sha256 over the digests of all the skill's files, so
it changes when any file does), `source` (the authored path, or the repository of an installed
skill), and, for installed skills, `ref` and `pinned` (true when `ref` is a full commit SHA). Log the
`digest` to record exactly what a session loaded. The digest covers the skill's whole file tree, including files
the server does not serve (unscannable files at `trust = "error"`), exactly like `lock_digest`; the served files
are listed with their own digests.

### Notes and limits

- The Go SDK dispatches only the methods it knows, so `skills/list` and `skills/get` are answered in a
  thin JSON-RPC layer in front of the SDK; everything else, including resources and tools, is served
  by the SDK normally. The stdio transport is the only one `ai-rulez mcp` offers.
- A malformed line never ends the session. Input that is not valid JSON gets `-32700`; a frame that is not a
  JSON-RPC 2.0 request (wrong or missing `jsonrpc`, an object or boolean `id`, an empty or invalid batch) gets
  `-32600`; a line over 16 MiB is dropped with `-32600`. The server keeps reading after each. A `resources/read`
  of an unknown URI gets the not-found error with the URI escaped.
- `skills/list` returns the whole catalog in one page (no `nextCursor`).
- A skill URI uses the skill name as its path, so two served skills must not share a name; the server
  refuses to start when they do.
- The extension requires every skill to have a `name` and a `description`. A skill without a
  description is served under its name as the description, and a warning on stderr names it (add a
  `description` so `find_skill` can rank it). The served bytes are not rewritten. A skill whose frontmatter
  cannot be parsed, or whose name is not a valid `skill://` path segment, is skipped with a warning; the
  rest are unaffected. A skill whose frontmatter is not valid YAML (`validate` fails on it) is refused:
  `load_skill` and `get_skill` answer `malformed-frontmatter` with the reason, and `generate` fails the same way.
- Semantic ranking is opt-in: with `[search] mode = "hybrid"` and an index built by `ai-rulez search index`, `find_skill` fuses BM25F with embedding similarity (see [Skill search](search.md)). By default the ranking is lexical.

## Dynamic skill loading

Static generation lists every skill (its name, description and about 27 tokens of framing) in every
session: roughly 14.5k tokens for 190 skills. **Dynamic skill loading** serves the skills you choose over
MCP instead, so the agent pays for them only when it asks. Three pieces work together:

1. **`delivery`** decides per skill whether it is written to the harness skill trees (`static`), served
   over MCP (`served`), or both.
2. **`generate`** leaves served skills out of every static tree and writes one small stub skill
   (`dynamic-skills`, about 100 tokens) that tells the agent to call `find_skill` and `load_skill`.
3. **`ai-rulez mcp --serve-skills`** serves those skills with `find_skill`, `load_skill` and
   `list_skill_resources`, scans each one before serving, can pin them in `ai-rulez.lock`, and reloads
   when their files change.

### Decision guide

| Keep the skill | When |
| -------------- | ---- |
| `static` | A core convention the agent needs in nearly every session (code style, commit rules, the repository map). The listing costs a few tokens and it never depends on the model deciding to search. |
| `served` | Domain skills used in some sessions: billing, migrations, a vendor API, incident response. The bulk of a large catalog. |
| `both` | A skill that must always be visible to one harness but that other harnesses or roles should also find by search. |

Rules of thumb: serve domain skills, keep a handful of core convention skills static. Three caveats:

- **The model has to choose to call `find_skill`.** A served skill is invisible until it does. Write
  `description` and `triggers` for search (what the user is trying to do, in their words), and keep the skills
  that must not be missed `static`.
- **An MCP-only harness is not enough for every harness.** A harness that cannot call MCP tools gets served
  skills as static files, with a warning (`AR992`), never silently dropped. Served skills also need an MCP
  server entry that runs `ai-rulez mcp --serve-skills` (`AR993` warns when there is none).
- **Served skills are not in the context until loaded.** Nothing that is always loaded (a rule, a static skill)
  can rely on them being there; `AR990` warns when static content names a served skill.

### Choosing the delivery

```yaml
---
name: db-migrations
description: Plan and run database schema changes safely. Use when changing a table.
delivery: served
triggers: [schema change, alembic migration, add a column]
---
```

```toml
[skills]
delivery = "static"        # global default; static when unset

[domains.billing]
delivery = "served"        # every skill of the billing domain
```

Precedence, first match wins: the skill's own `delivery` frontmatter, then the role's `delivery` map (when a role
is being rendered or served, see [Roles](roles.md#delivery)), then `[domains.<name>] delivery`, then
`[skills] delivery`, then `static`. An invalid value is ignored (the next level applies) and reported as `AR994`;
invalid config values fail `validate`. `config.Config.EffectiveDelivery(skill, domain, roleOverride)` is the
single function that resolves all of this.

`triggers` is a list of phrases that should make the agent look for the skill (a comma-separated string works
too). `keywords` are searched as well. `delivery` and `triggers` are ai-rulez keys: `delivery` is not written
into generated frontmatter.

### What `generate` does

- A skill whose delivery is `served` is not written to any preset's skill tree. `static` and `both` skills are.
- When any skill is `served` or `both`, or a `[[skill_sources]]` entry is configured (its skills are served,
  never written), one stub skill named `dynamic-skills` is added to the root skills of
  every preset whose harness can call MCP tools. A skill you author with that name, at the root or in a domain
  that is active, is used instead of the stub; if several are authored, a warning asks you to rename all but one.
- A preset whose harness cannot call MCP (for example `cline`, `rovodev`, or any custom preset) keeps every
  served skill as a static file and gets no stub. Each such preset is named in a warning (`AR992`).
  Skills of `[[skill_sources]]` are served only, by design: they never reach a preset without MCP support
  (nothing is written for them), and `generate` and `validate --strict` warn (`AR992`) for each such preset
  when `[[skill_sources]]` is configured.
- `ai-rulez mcp --serve-skills` renders the served skills itself; it never depends on the generated trees.

### Serving

```bash
ai-rulez mcp --serve-skills                       # the skills whose delivery is served or both
ai-rulez mcp --serve-skills --role backend        # the backend role's skills, with the role's delivery
ai-rulez mcp --serve-skills --source git+https://github.com/acme/skills@v1.2.0#skills/
ai-rulez mcp --serve-skills --frozen              # lock required, no network
```

Register it in `config.toml` so every MCP-capable harness launches it. The generated `dynamic-skills` stub names
the `[[mcp_servers]]` entry whose `args` include `--serve-skills` (`ai-rulez-skills` when none does):

```toml
[[mcp_servers]]
name = "ai-rulez-skills"
command = "ai-rulez"
args = ["mcp", "--serve-skills"]
```

A project that sets no delivery anywhere serves every skill, as `--serve-skills` always did. Once any skill is
`served` or `both`, only those are served; `--include-static` adds the rest.

| Flag | Meaning |
| ---- | ------- |
| `--source` | Serve the skills of a source as well (repeatable, see [Skill sources](#skill-sources)). |
| `--role` | A role of `[[roles]]`. The server serves only the skills the role keeps, with the delivery the role gives them (a skill the role delivers `static` is not served unless `--include-static` is set), and `find_skill` ranks within the role by default. Mutually exclusive with `--profile`; an unknown role is an error at start. |
| `--frozen` | Never use the network and require `ai-rulez.lock` to cover every remote include, installed skill and skill source. |
| `--offline` | Never use the network; use the lock and the cache when present. |
| `--include-static` | Also serve skills whose delivery is static. |
| `--max-clone-bytes` | Clone size limit in bytes for git skill sources that set no `max_clone_bytes`. Overrides `AI_RULEZ_MAX_CLONE_BYTES`; 0 (default) defers to it, then to 256 MiB. |
| `--budget-bytes` | Bytes of skill content a session may read, through `load_skill`, `get_skill`, `read_skill_file` and `resources/read` together. Default 262144 (256 KiB); any negative value removes the cap. |
| `--usage-log`, `--usage-sink` | Where to record each `load_skill` (see [Usage telemetry](#usage-telemetry)). |
| `--no-watch`, `--reload-interval` | Turn live reload off, or change the two-second check interval. |

The serve-mode flags above (and `--profile`, `--targets`, `--domain`, `--allow`, `--deny`) are rejected without
`--serve-skills`. The authoring tools are never registered in this mode: there is no tool that writes.

### Tools

| Tool | Arguments | Result |
| ---- | --------- | ------ |
| `find_skill` | `task` (required), `limit` (default 5, max 20), `role` | Ranked matches: name, description, score, domain, digest. With a role, `in_role` and the role's skills first. |
| `load_skill` | `name` (required), `path`, `budget_bytes` | The file (`SKILL.md` by default), `provenance`, `digest`, and an index of the skill's other files. |
| `list_skill_resources` | `name` (required) | Every file of the skill with path, URI, size, MIME type and digest, without loading them. |

`search_skills`, `get_skill` and `read_skill_file`, the `skill://` resources, `skills/list` and `skills/get` keep
working as described above. All tools are annotated read-only.

- **Ranking.** `find_skill` scores BM25 over four fields with weights name 3, triggers 2.5, keywords 2 and
  description 1, after lowercasing, dropping stopwords and a light stemmer (`migrations` matches `migration`).
  By default it is lexical and deterministic: score descending, then name. With `[search] mode = "hybrid"` (or
  `vector`) and an index built by [`ai-rulez search index`](search.md), the same ranker used by
  `ai-rulez search` fuses that list with cosine similarity over the index by reciprocal rank fusion. The
  server never builds the index and never embeds a skill: it loads the index files (and reloads them when they
  change) and embeds only the query, bounded by `query_timeout_ms` and an in-memory cache of 256 queries,
  through the `[llm]` network gate, budget and cache. Any failure (no index, an index of another model, the
  network disabled, a timeout, the budget) answers lexically and says why: the result gains `"ranking":
  "lexical"` and `"degraded": "no_index" | "provider_unavailable" | "timeout" | "budget" |
  "network_disabled"`. A hybrid result also carries `ranking`, and each match `lexical_rank` and
  `vector_rank` (0: not in that list); a skill edited since it was indexed ranks lexically only and has
  `stale_vector: true`. A server with the default lexical mode returns exactly the fields listed above.
  `search_skills` stays lexical: it is the listing and filter tool. With `[search] log_queries = true` the
  server also records each `find_skill` query and the skill the session loads next (see
  [Skill search](search.md#query-mining)).
- **Roles.** `role` is resolved against the project's `[[roles]]` (`mcp.RolesFromConfig`): a skill is in scope
  when the role keeps it (its domains and `skills` include and exclude selectors, `extends` merged in). Matches
  inside the scope come first, then the rest marked `in_role: false`; an unknown role is an error. The `role`
  argument is a ranking lens the caller chooses, not access control: what a server can load is decided by
  `--role` at start, which builds the catalog without the skills outside the role (they cannot be found,
  loaded, listed or read by name). A skill that
  comes from a `[[skill_sources]]` entry is matched by name against the role's `skills` selectors. The role is
  read again on every live reload.
- **Budget.** Each session may read `--budget-bytes` bytes of skill content; `load_skill`, `get_skill`,
  `read_skill_file` and `resources/read` draw on the same budget (listing tools cost nothing, and a repeated
  read is charged again). A read that would exceed the
  remainder is refused and is not charged (`load_skill` reports the bytes left; `resources/read` fails with
  JSON-RPC error `-32600`); the server tracks at most 1024 sessions; `budget_bytes` on one call truncates that
  call's file (at a character boundary) and reports `truncated` and `total_bytes`. Supporting files count too.
- **Path.** `path` is relative to the skill. Absolute paths, `..`, backslashes and names that are not valid
  `skill://` path segments are rejected. A path with `.` or `..` segments that normalises to a file inside the
  skill (`references/../SKILL.md`) is accepted as that file; one that would leave the skill is rejected. The
  result's `uri` is the URI of the file loaded (`skill://<name>/<path>`). Binary files are read with `resources/read`.
- **Provenance.** Every result carries `provenance`: `digest` (served bytes), `lock_digest` (what the lock pins),
  `locked`, `source`, `ref`, `pinned`, `commit` for a source skill, `delivery`, and `scan_warnings`.

### Skill sources

A source is a repository (or a directory) of skill directories, served and never written to the static trees.

```toml
[[skill_sources]]
name = "acme"                                  # identifies the source in the lock
url = "https://github.com/acme/skills.git"     # git URL (https, ssh or file; a leading git+ is accepted) or a local directory
ref = "v1.2.0"                                 # a tag or a full commit SHA (or version = "^1.2", see below)
path = "skills"                                # subdirectory whose children are skills
include = ["pdf-*", "sql"]                     # directory-name globs; exclude wins
exclude = ["*-wip"]
name_prefix = "acme-"                          # served as acme-pdf-forms, and SKILL.md name is rewritten
trust = "error"                                # scan level: error (default) or warn
max_skills = 200                               # optional: skills the source may load (default 200)
max_bytes = 67108864                           # optional: bytes of skill files it may load (default 64 MiB)
max_clone_bytes = 268435456                    # optional: size limit of the git clone (default 256 MiB)
max_clone_files = 20000                        # optional: entries of the git clone (default 20000)
```

A source can ask for a version range instead of a ref: `version = "^1.2"` (also `tag_prefix`,
`include_prerelease`; `ref` and `version` are exclusive). The range is resolved against the repository's semver tags
by `ai-rulez lock` and moved only by `ai-rulez update`; serving uses the pinned commit and never resolves a range.
See [Version constraints](lockfile.md#version-constraints).

`--source` takes the same thing on the command line: `[git+]<url>[@<tag|commit>][#<subdir>]` or a directory,
for example `git+https://host/org/repo@v1.2.0#skills/`. The ref separator is the last `@` after the final `/`,
so `git@host:org/repo.git` and `https://user@host/...` keep their user info. The source is named
`cli-<repository>`. In a directory with no `.ai-rulez`, `--source` alone is enough to serve.

- **Pinning.** A tag is resolved to the commit it points at (an annotated tag is peeled), a full SHA is used as
  is. `ai-rulez lock` records the commit and the tree digest (the same `sha256` scheme as includes) as a `[[source]]`
  entry in `ai-rulez.lock`. With the lock, the pinned commit is fetched by its SHA even if the tag or branch has
  since moved, and the fetched tree must match the digest or serving fails. A server that refuses fetching a
  commit by SHA is asked for the ref with its history instead; if the pinned commit is no longer reachable from
  it (history was rewritten), the fetch fails closed and says so. The `commit` in a lock entry must be a full hex
  SHA.
- **Unpinned refs.** A branch, a tag the lock does not cover, or no ref, follows a moving ref; it is reported as
  unpinned (`AR010`, and a warning at serve time) until the lock covers it.
- **Transports and private repositories.** Git runs without hooks, credential helpers, prompts or submodules and
  only speaks `https`, `ssh` and `file` (never `ext::`; plain `http://` is not fetched). A `url` or `ref` that
  starts with `-` is rejected, as git would read it as an option. Credentials are not injected and the git
  credential helpers are off, so a private repository needs an `ssh` URL that works non-interactively (an
  ssh-agent key); a private `https` repository is not supported. Each git command is cut off after five minutes.
- **Limits.** A source may load at most `max_skills` skills (default 200) and `max_bytes` bytes of skill files
  (default 64 MiB); a larger source is an error naming the key. A skill has at most 2000 files.
- **Clone size.** A git source is fetched as a partial clone (`--filter`), and with a `path` only that
  subdirectory is checked out (a sparse checkout), so a large repository costs what the skills cost. The clone,
  git data and checkout together, is capped at `max_clone_bytes` (default 256 MiB; the `AI_RULEZ_MAX_CLONE_BYTES`
  environment variable, or `mcp --serve-skills --max-clone-bytes <n>`, sets it for every source that does not set its
  own; the flag wins over the environment variable, and a source's own `max_clone_bytes` wins over both). The size is watched while git runs:
  a clone that grows past the limit is stopped, fails with an error naming `max_clone_bytes` and the source
  (`skillsource.ErrCloneTooLarge`), and leaves nothing in the cache. Set `path`, or raise the limit.
- **Cache integrity.** A source the lock covers is checked against the lock's digest. A source whose `ref` is a full
  commit SHA but which the lock does not cover is checked against a digest recorded next to the cached tree
  (`<tree>.digest`, written atomically, outside the tree so it never changes the tree digest, readable only by you).
  If the tree no longer matches, or has no record (a cache written by an older release), it is fetched again;
  with `--offline`/`--frozen` there is no fetch, so serving fails with a "cache damaged" error until you run once
  online. The record catches damage and edits of the tree; it is not a defence against someone who can also
  rewrite the record, which is what the lock is for.
- **Cache and network.** Trees are cached per commit under `~/.cache/ai-rulez/skill-sources/`. `--frozen` requires
  the lock to cover the source and never touches the network (the commit must be cached). `--offline` does not
  require the lock: it uses the lock's commit, or the commit an earlier online run recorded for that ref.
- **Local directories** need no network; a lock entry pins their digest, and serving refuses a changed tree. A
  local `url`/`path` declared in the project's config must resolve, after symlinks, inside the project (a relative
  one is relative to the project root); a committed config cannot point the server at another user's skill
  directory. `--source <dir>` on the command line and sources in your user config may point anywhere.
- **Safety.** Symlinks below the source are never followed; a `path` of a git source that goes through a
  symlink is refused, and a local source directory that is itself a symlink is resolved and digested through
  the link. A file over 2 MiB is dropped with a warning, a skill over 8 MiB is skipped with a warning. A source
  skill is served under its directory name (with `name_prefix`), whatever its `name:` says (SKILL.md is
  rewritten), and a skill whose name collides with one already served is skipped (set `name_prefix`); the
  project's skill wins.
- **Symlinked skills.** A skill directory that is a symlink out of the project is dropped by the server with a
  warning, while `validate` fails on it: fix the link rather than relying on the warning.

### Security scan

Every served skill is scanned before it is served, with the rules of `ai-rulez scan` (secrets,
hidden characters, prompt-injection phrases, risky shell, unrestricted `allowed-tools`, encoded blobs) over
`SKILL.md` and every text file of at most 512 KiB. A file the scan cannot read (binary: a NUL byte or invalid
UTF-8; or over 512 KiB) is reported as `AR989`. At `trust = "error"` (remote sources, installed skills) such a
file is not served at all (`load_skill`, `resources/read` and the file list omit it; provenance lists it under
`unserved_unscannable_files`, and the lock digest still covers it), and a `SKILL.md` that cannot be scanned
refuses the skill. At `trust = "warn"` (skills authored in the project) the file is served with the warning.
A skill that fails is not served: it is absent from `resources/list`,
`skills/list` and `find_skill`, a warning names the finding on stderr, and `load_skill` says why. Inline
`ai-rulez-lint-ignore` comments are not honored. `validate --strict` runs this same scan over the served
skills, authored and from sources alike, and reports `AR989` for each file it cannot read (see the table below).
The level is `trust` for a source skill:

| Level | Blocks |
| ----- | ------ |
| `error` | Any finding (every finding counts as an error). Default for source skills and for skills installed from a git repository (`lint.security.scan_imports = "warn"` lowers installed skills to `warn`). |
| `warn` | Findings that are errors by their own severity (secrets, hidden characters, risky shell). Default for skills authored in the project. |

Skills that come from an `[[includes]]` entry are remote content: they are scanned at `error` like installed skills
(`scan_imports = "warn"` lowers them to `warn`), as is any skill file outside the project root. This holds wherever
the include lives, including a local include inside the project (`source = "./shared"`). Their lock `source`
is `include:<name>/<path>`, identical on every machine.

### Lock enforcement

`ai-rulez lock` also records, for every skill the server would serve, a `[[served]]` entry with its name and
lock digest (and a `[[source]]` entry per skill source). With

```toml
[lock]
enforce = true   # the default whenever ai-rulez.lock exists; false opts out
```

the server refuses a served skill whose digest differs from the lock, and one the lock does not pin, with
`AR995`. `validate --strict` reports the same. The lock digest is a `sha256:` tree digest in the same scheme as
the other pins ([Lock file](lockfile.md#served-skills-and-skill-sources), domain `ai-rulez/served-skill/v1`). It
covers the rendered files but not the generated header lines that change without the skill changing (a whole
`Source-Hash: <algorithm>:<hex>` or `Content-Hash: <algorithm>:<hex>` line and the `Generated:` date stamp, in the first 40 lines of a rendered
file), so editing one skill does not invalidate the others. Files of a skill source are digested exactly as
they are: nothing in them is ignored. The digest is of
the rendering the view selects: the default preset's, or another preset's with `--targets` (a view of its own, see
below). Skills that only a role serves are pinned too: `lock` builds the unscoped view and the view of every
role. `ai-rulez lock --kind served|source` refreshes one kind; `lock --check` verifies both without the network.

`[[served]]` entries are recorded per serve view: the way the server is started selects a set of skills, and
the lock pins each set under a `view` key (see [Lock file](lockfile.md#served-skills-and-skill-sources)). A plain
`lock` pins the default view (the configured `[[skill_sources]]`, the default profile and preset, the delivery
rules), every role, and every view the lock already records. To pin another view, give `lock` the flags the
server runs with:

```bash
ai-rulez lock --role backend                       # skills the backend role serves
ai-rulez lock --profile team --include-static      # profile view that also serves static skills
ai-rulez lock --targets cursor                     # skills as the cursor preset renders them
ai-rulez lock --source git+https://host/org/skills@v1.2.0#skills   # a command-line source, pinned with its commit
ai-rulez mcp --serve-skills --role backend --frozen
```

`mcp --serve-skills` and `lock --check` read the pins of the view they run with, so a server started with
`--role`, `--profile`, `--targets`, `--include-static` or `--source` finds its skills pinned once `lock` has pinned that view;
otherwise every skill it adds is refused with `AR995`. A lock written before views existed (no `view` keys)
still covers every view, as long as the digests match. `--targets` is part of the view (`targets:<preset>`): another
preset's rendering has its own pins, and `lock --targets <preset>` pins it.

A skill the security scan refuses is not pinned. By default `lock` reports it, pins every other skill, writes the
lock and exits 3; `validate --strict` lists the refused skills of skill sources with their scan code (`AR0xx`). With
`lock --strict` any refusal stops `lock` from writing the lock. Either way a refused skill has to be fixed or
excluded before it can be served under enforcement.

### Usage telemetry

Each successful `load_skill` goes through the usage recorder as one identifier-only JSON line: time, skill name,
salted session hash (a stdio connection has no transport session id, so the server gives each connection a random
one; a new connection gets a new hash and a new byte budget, the same pipe keeps both; the salt file is created next
to the log on first use), harness (the MCP client name), the role the server runs under, `outcome: "loaded"`, content
hash from the skills index, the served digest, and `served: true` (log format `v: 3`, the same as hook-recorded
loads).
A supporting file loaded with `path` is logged with `resource: true` and is not counted again by
`ai-rulez report usage`. Nothing is written until you opt in: pass `--usage-log <file>` or `--usage-sink <command>`,
or enable `[usage] skills_index = true`, which logs to `<config dir>/local/usage.jsonl`.

A `--usage-sink` receives exactly the line the log gets, salted session included. Without a `--usage-log` the salt
lives in `<config dir>/local/usage.salt`. The sink command runs in the background: records wait in a queue of 256,
so a slow or hanging sink never delays `load_skill`. A full queue drops the new record and logs a warning with the
number dropped. Queued records are delivered at shutdown, waiting up to three seconds.

### Live reload

The server checks the configuration directory (and local source directories) every two seconds. When a file
changes it rebuilds the catalog and swaps it in. `notifications/resources/list_changed` is sent only when the
catalog changed (a skill added, removed or edited); an edit that leaves every served skill identical sends nothing. A rebuild that
fails keeps the previous catalog serving and is retried with a growing pause (up to a minute) until it works or
a new edit changes the files (a new edit ends the pause at once). The usage log and its salt file (`--usage-log`
and `usage.salt` beside it, compared by absolute path) and any `.jsonl` file directly inside a `local/` directory,
at any depth, do not count as changes. Polling keeps the dependency set unchanged; git sources are immutable
per commit and are not re-fetched.

### Strict validation

| Code | Severity | Meaning |
| ---- | -------- | ------- |
| `AR990` | warning | Static content names a served skill |
| `AR991` | error | A harness that can call MCP has no `dynamic-skills` stub |
| `AR992` | warning | A harness without MCP keeps served skills as static files |
| `AR993` | warning | Skills are served but no `[[mcp_servers]]` entry runs `--serve-skills` |
| `AR994` | error | `delivery` frontmatter is not static, served or both |
| `AR995` | error | `[lock] enforce` and a served skill is unpinned or its digest differs |
| `AR989` | warning / error | A served file cannot be scanned (binary or over 512 KiB). An error for `SKILL.md` at any trust level (the server refuses the skill). For a supporting file: a warning at `trust = "warn"` (served with the warning) and, at `trust = "error"`, a finding that names the unserved files |

See [Strict validation](strict-validation.md) for the full table.

## Typical Workflow

### With Your Editor

1. **Edit files** directly in `.ai-rulez/`:

   ```bash
   # Edit rules, context, skills, or config.toml
   vim .ai-rulez/rules/code-quality.md
   ```

2. **Use MCP server** (via AI assistant) to generate:

   ```bash
   ai-rulez generate
   ```

3. **Commit changes**:

   ```bash
   git add .ai-rulez/   # plus generated files only if gitignore = false
   git commit -m "docs: update AI guidelines"
   ```

### With Claude CLI

If using the Claude CLI with the ai-rulez MCP server, you can ask Claude to help:

- "Review my `.ai-rulez/` configuration and suggest improvements"
- "Generate outputs for my new rules"
- "Validate that my profiles are correct"
- "Create a new backend domain and add database rules"
- "Add an include from our shared rules repository"

The server provides Claude with access to read and modify your configuration, while you maintain full control over final decisions.

---

## MCP Tools Reference

The ai-rulez MCP server exposes 36 tools for programmatic configuration management. These tools allow AI assistants to initialize projects, generate and clean outputs, validate configuration, inspect builtins, and create, read, update, and delete configuration elements.

Every tool below accepts an optional `working_directory` parameter (the directory to operate in),
except the two utility tools `get_version` and `show_builtin`. The per-tool parameter lists below
name only the parameters specific to that tool unless noted.

### Local configuration (`local: true`)

Tools that change content or config accept an optional `local` boolean that redirects the change to
the machine-local layer instead of the shared one. See [Local Configuration](local-overrides.md).

| Tools | `local: true` targets |
| ----- | --------------------- |
| `create_*`, `read_*`, `update_*`, `delete_*`, `list_*` for rule, context and skill | The `.ai-rulez/local/` content tree (`domain` selects `local/domains/<name>/`) |
| `add_include`, `remove_include`, `install_skill`, `uninstall_skill`, `update_config`, `add_profile`, `remove_profile`, `set_default_profile` | The `config.local.*` overlay |

Not accepted: `create_domain`, `delete_domain`, `list_domains`, `list_includes`, `list_profiles`,
`list_installed_skills`. Those list tools show the shared layer only.

The drift guard applies to `generate_outputs` too. `--allow-local-drift` is CLI-only: the MCP server
cannot write overlay-derived values over tracked shared files. Pass `no_local: true` to generate the
shared view, or fix the reported paths.

### Response keys

Responses of the list tools serialize Go structs without JSON tags, so item keys are capitalised
(`Name`, `Path`), unlike the lower-case envelope keys (`success`, `operation`, `count`).

### Project and Utility Tools

#### `generate_outputs`

Generate output files from the current configuration.

**Parameters:**

- `config_file` (optional, string): Path to the root configuration file
- `config_dir` (optional, string): Configuration directory name (default: `.ai-rulez`)
- `dry_run` (optional, boolean): Preview changes without writing files
- `recursive` (optional, boolean): Generate for all subdirectories containing `.ai-rulez/`
- `no_local` (optional, boolean): Ignore the machine-local `config.local.*` overlay and `local/` content (the view a teammate sees)
- `working_directory` (optional, string): Directory to operate in

#### `clean_outputs`

Remove the files produced by `generate_outputs` (its inverse): the generated assistant files, the generated manifest (and local manifest with the local rule files it records), and the ai-rulez managed `.gitignore` block. The `.ai-rulez/` source tree is never touched, and generated directories are removed only once empty.

**Parameters:**

- `config_file` (optional, string): Path to the root configuration file
- `config_dir` (optional, string): Configuration directory name (default: `.ai-rulez`)
- `dry_run` (optional, boolean): Preview what would be removed without deleting
- `keep_gitignore` (optional, boolean): Leave the ai-rulez managed block in `.gitignore`
- `keep_manifest` (optional, boolean): Leave the generated manifest in place
- `working_directory` (optional, string): Directory to operate in

#### `validate_config`

Validate the configuration file, including all includes. A `config.local.*` overlay, when present, is
also validated against the local schema (`schema/ai-rules-local.schema.json`). Errors are redacted:
URL credentials and quoted source values are removed, since the merged config can contain local secrets.

**Parameters:**

- `config_file` (optional, string): Path to the root configuration file
- `config_dir` (optional, string): Configuration directory name (default: `.ai-rulez`)
- `no_local` (optional, boolean): Validate without the machine-local overlay
- `working_directory` (optional, string): Directory to operate in

#### `init_project`

Initialize a new ai-rulez project in the current directory with the layout `ai-rulez init` creates:
`.ai-rulez/config.toml` plus the `rules/`, `context/`, `skills/`, `agents/` and `domains/` directories. It refuses
to overwrite an existing configuration.

**Parameters:**

- `project_name` (optional, string): Project name
- `providers` (optional, array): Providers to enable, such as `claude` or `cursor`
- `with_agents` (optional, boolean): Accepted for compatibility; `agents/` is always created
- `all_providers` (optional, boolean): Enable the curated set of tool presets
- `popular_providers` (optional, boolean): Same curated set — a shortcut for `all_providers`
- `working_directory` (optional, string): Directory to operate in

#### `get_version`

Return the ai-rulez version.

**Parameters:** (none)

#### `show_builtin`

Show the full content of a builtin domain.

**Parameters:**

- `name` (required, string): Builtin domain name, such as `security`, `rust`, or `typescript`

### Domain Tools

#### `create_domain`

Create a new domain with subdirectories for rules, context, and skills.

**Parameters:**

- `name` (required, string): Domain name (alphanumeric and underscores, 1-50 characters)
- `description` (optional, string): Description of the domain

**Response:**

```json
{
  "success": true,
  "operation": "create_domain",
  "name": "backend",
  "path": ".ai-rulez/domains/backend",
  "message": "Domain created successfully"
}
```

**Example:**

```text
Create a domain called "backend" for backend services
```

#### `delete_domain`

Delete a domain and all its contents.

**Parameters:**

- `name` (required, string): Domain name to delete

**Response:**

```json
{
  "success": true,
  "operation": "delete_domain",
  "name": "backend",
  "message": "Domain deleted successfully"
}
```

#### `list_domains`

List all domains in the `.ai-rulez/` directory.

**Parameters:** (none)

**Response:**

```json
{
  "success": true,
  "operation": "list_domains",
  "domains": [
    {
      "Name": "backend",
      "Path": ".ai-rulez/domains/backend",
      "Description": "Backend services"
    }
  ],
  "count": 1
}
```

### Rule Tools

#### `create_rule`

Create a new rule file with optional YAML frontmatter.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `content` (optional, string): Markdown content with optional YAML frontmatter
- `domain` (optional, string): Domain name (if not specified, creates in root)
- `priority` (optional, string): Priority level - critical, high, medium, low, minimal. Default: medium
- `targets` (optional, array): Target providers (e.g., ["claude", "cursor"])
- `local` (optional, boolean): Create in `.ai-rulez/local/` (gitignored) instead of the shared tree

**Response:**

```json
{
  "success": true,
  "operation": "create_rule",
  "path": ".ai-rulez/rules/code-quality.md",
  "name": "code-quality",
  "domain": "",
  "message": "Rule created successfully"
}
```

#### `update_rule`

Update an existing rule file.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `content` (required, string): New markdown content
- `domain` (optional, string): Domain name
- `priority` (optional, string): Priority level
- `targets` (optional, array): Target providers
- `local` (optional, boolean): Update the file in `.ai-rulez/local/`

**Response:** Same as create_rule

#### `read_rule`

Read a rule file.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Read from `.ai-rulez/local/`
- `working_directory` (optional, string): Directory to operate in

#### `delete_rule`

Delete a rule file.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Delete from `.ai-rulez/local/`

**Response:**

```json
{
  "success": true,
  "operation": "delete_rule",
  "name": "code-quality",
  "domain": "",
  "message": "Rule deleted successfully"
}
```

#### `list_rules`

List all rules in the root or a specific domain.

**Parameters:**

- `domain` (optional, string): Domain name (lists root rules if not specified)
- `local` (optional, boolean): List `.ai-rulez/local/` instead of the shared tree

**Response:**

```json
{
  "success": true,
  "operation": "list_rules",
  "domain": "",
  "rules": [
    {
      "Name": "code-quality",
      "Path": ".ai-rulez/rules/code-quality.md",
      "Type": "rules",
      "Domain": "",
      "Priority": "high",
      "Targets": ["claude", "cursor"]
    }
  ],
  "count": 1
}
```

### Context Tools

#### `create_context`

Create a new context file (documentation/reference material).

**Parameters:**

- `name` (required, string): Context filename without .md extension
- `content` (optional, string): Markdown content with optional YAML frontmatter
- `domain` (optional, string): Domain name
- `priority` (optional, string): Priority level
- `targets` (optional, array): Target providers
- `local` (optional, boolean): Create in `.ai-rulez/local/` (gitignored)

**Response:** Similar to create_rule

#### `update_context`

Update an existing context file.

**Parameters:** Same as create_context, with content as required

**Response:** Similar to create_rule

#### `read_context`

Read a context file.

**Parameters:**

- `name` (required, string): Context filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Read from `.ai-rulez/local/`
- `working_directory` (optional, string): Directory to operate in

#### `delete_context`

Delete a context file.

**Parameters:**

- `name` (required, string): Context filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Delete from `.ai-rulez/local/`

**Response:** Similar to delete_rule

#### `list_context`

List all context files in the root or a specific domain.

**Parameters:**

- `domain` (optional, string): Domain name
- `local` (optional, boolean): List `.ai-rulez/local/` instead of the shared tree

**Response:** Similar to list_rules (items use the capitalised `Name`, `Path`, `Type`, `Domain`, `Priority`, `Targets` keys), with a "context" key instead of "rules"

### Skill Tools

#### `create_skill`

Create a new skill file (AI prompt/expert definition).

**Parameters:**

- `name` (required, string): Skill filename without .md extension
- `content` (optional, string): Markdown content with optional YAML frontmatter
- `domain` (optional, string): Domain name
- `priority` (optional, string): Priority level
- `targets` (optional, array): Target providers
- `local` (optional, boolean): Create in `.ai-rulez/local/` (gitignored)

**Response:** Similar to create_rule

#### `update_skill`

Update an existing skill file.

**Parameters:** Same as create_skill, with content as required

**Response:** Similar to create_rule

#### `read_skill`

Read a skill file.

**Parameters:**

- `name` (required, string): Skill filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Read from `.ai-rulez/local/`
- `working_directory` (optional, string): Directory to operate in

#### `delete_skill`

Delete a skill file.

**Parameters:**

- `name` (required, string): Skill filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Delete from `.ai-rulez/local/`

**Response:** Similar to delete_rule

#### `list_skills`

List all skill files in the root or a specific domain.

**Parameters:**

- `domain` (optional, string): Domain name
- `local` (optional, boolean): List `.ai-rulez/local/` instead of the shared tree

**Response:** Similar to list_rules (items use the capitalised `Name`, `Path`, `Type`, `Domain`, `Priority`, `Targets` keys), with a "skills" key instead of "rules"

### Check Tools

Checks are code-review guidelines (see [Checks](checks.md)). They live in the shared tree only: there is no `local` argument.

| Tool           | Arguments                                                                                          |
| -------------- | -------------------------------------------------------------------------------------------------- |
| `create_check` | `name`, `content`, `description`, `severity` (low\|medium\|high\|critical), `tools`, `targets`, `domain` |
| `read_check`   | `name`, `domain`                                                                                   |
| `update_check` | `name`, `domain`, and `content` and/or any of `description`, `severity`, `tools`, `targets`         |
| `delete_check` | `name`, `domain`                                                                                   |
| `list_checks`  | `domain`                                                                                           |

Names are limited to `[A-Za-z0-9._-]`. Content without frontmatter gets one built from the structured arguments (marshalled as YAML, `severity` and `targets` validated). `update_check` rejects a call with neither `content` nor a field; content without frontmatter replaces the body and keeps the existing frontmatter, and fields are set on it.

### Include Tools

#### `add_include`

Add a new include source (git URL or local path) to the configuration.

**Parameters:**

- `name` (required, string): Include name (unique identifier)
- `source` (required, string): Git URL (`https://github.com/org/repo`) or local path (`./packages/shared`)
- `path` (optional, string): Path within git repository where .ai-rulez/ content is located
- `ref` (optional, string): Git reference - branch, tag, or commit hash (git sources only). Defaults to the repository's default branch (`HEAD`)
- `include` (optional, array): Content types to include - rules, context, skills, agents, commands
- `merge_strategy` (optional, string): Merge strategy - local-override (default), include-override, error
- `install_to` (optional, string): Installation target path in .ai-rulez/
- `local` (optional, boolean): Add the include to the `config.local.*` overlay instead of the shared config

**Response:**

```json
{
  "success": true,
  "operation": "add_include",
  "name": "corporate-rules",
  "source": "https://github.com/myorg/shared-rules",
  "message": "Include added successfully"
}
```

#### `remove_include`

Remove an include source from the configuration.

**Parameters:**

- `name` (required, string): Include name to remove
- `local` (optional, boolean): Remove through the overlay; an include from the shared config is hidden with `remove = true`

**Response:**

```json
{
  "success": true,
  "operation": "remove_include",
  "name": "corporate-rules",
  "message": "Include removed successfully"
}
```

#### `list_includes`

List all include sources in the configuration.

**Parameters:** (none)

**Response:**

```json
{
  "success": true,
  "operation": "list_includes",
  "includes": [
    {
      "Name": "corporate-rules",
      "Source": "https://github.com/myorg/shared-rules",
      "Type": "git"
    }
  ],
  "count": 1
}
```

### Installed Skill Tools

#### `install_skill`

Install a named skill from a git repository or local path.

**Parameters:**

- `name` (required, string): Skill name (unique identifier)
- `source` (required, string): Git URL or local filesystem path
- `path` (optional, string): Path within repo to skill directory (defaults to `skills/<name>`)
- `ref` (optional, string): Git reference (branch, tag, commit)
- `local` (optional, boolean): Record the skill in the `config.local.*` overlay instead of the shared config

**Response:**

```json
{
  "success": true,
  "operation": "install_skill",
  "name": "kreuzberg",
  "source": "https://github.com/kreuzberg-dev/kreuzberg",
  "message": "Skill installed successfully"
}
```

#### `uninstall_skill`

Remove an installed skill from the configuration.

**Parameters:**

- `name` (required, string): Skill name to remove
- `local` (optional, boolean): Remove through the overlay; a skill from the shared config is hidden with `remove = true`

**Response:**

```json
{
  "success": true,
  "operation": "uninstall_skill",
  "name": "kreuzberg",
  "message": "Skill uninstalled successfully"
}
```

#### `list_installed_skills`

List all installed skills.

**Parameters:** None

**Response:**

```json
{
  "success": true,
  "operation": "list_installed_skills",
  "installed_skills": [
    {
      "Name": "kreuzberg",
      "Source": "https://github.com/kreuzberg-dev/kreuzberg",
      "Path": "skills/kreuzberg",
      "Ref": "",
      "Type": "git"
    }
  ],
  "count": 1
}
```

### Config Tools

#### `read_config`

Read the current project configuration as structured JSON. The result is the shared view (as if loaded
with `--no-local`), so a read-modify-write loop never copies local values into the shared config.

**Parameters:**

- `working_directory` (optional, string): Directory to operate in

**Response keys:** `name`, `description`, `presets`, `profiles`, `builtins`, `includes`, `gitignore`,
`default_effort`, `default_effort_by_preset`, `rules_mode` and `rules_mode_by_preset`. The last four are
always present, empty when unset. When a `config.local.*` overlay exists, `local_overlay` reports
`{ "path": "...", "keys": ["presets", "mcp_servers.github.env.GITHUB_TOKEN", ...] }`: key paths only,
never values.

#### `update_config`

Update supported project configuration fields.

**Parameters:**

- `name` (optional, string): Project name
- `description` (optional, string): Project description
- `builtins` (optional, array): Builtin names to enable
- `gitignore` (optional, boolean): Whether generation updates `.gitignore`
- `default_effort` (optional, string): Default reasoning effort
- `default_effort_by_preset` (optional, object): Per-preset reasoning effort overrides
- `rules_mode` (optional, string): Default rules output mode, `split` or `inline`; empty string clears it
- `rules_mode_by_preset` (optional, object): Per-preset rules mode overrides. The map replaces the existing one; an entry with value `""` removes that preset's override, and `{}` or `null` clears them all
- `local` (optional, boolean): Write the supplied fields to the `config.local.*` overlay instead of the shared config. Clearing a field (empty string or empty map, including `name` and `description`) removes that key from the overlay instead of writing an empty value
- `working_directory` (optional, string): Directory to operate in

### Profile Tools

#### `add_profile`

Create a new profile with a set of domains.

**Parameters:**

- `name` (required, string): Profile name (unique identifier)
- `domains` (required, array): List of domain names to include in the profile
- `local` (optional, boolean): Define the profile in the `config.local.*` overlay; it may reference local domains

**Response:**

```json
{
  "success": true,
  "operation": "add_profile",
  "name": "full",
  "domains": ["backend", "frontend", "qa"],
  "message": "Profile added successfully"
}
```

#### `remove_profile`

Remove a profile from the configuration.

**Parameters:**

- `name` (required, string): Profile name to remove
- `local` (optional, boolean): Remove a profile defined in the overlay; a shared profile cannot be removed locally (error)

**Response:**

```json
{
  "success": true,
  "operation": "remove_profile",
  "name": "staging",
  "message": "Profile removed successfully"
}
```

#### `set_default_profile`

Set a profile as the default for generation.

**Parameters:**

- `name` (required, string): Profile name to set as default
- `local` (optional, boolean): Set `default` in the overlay instead of the shared config

**Response:**

```json
{
  "success": true,
  "operation": "set_default_profile",
  "name": "full",
  "message": "Default profile set successfully"
}
```

#### `list_profiles`

List all profiles in the configuration.

**Parameters:** (none)

**Response:**

```json
{
  "success": true,
  "operation": "list_profiles",
  "profiles": [
    {
      "Name": "full",
      "Domains": ["backend", "frontend", "qa"],
      "IsDefault": true
    },
    {
      "Name": "backend",
      "Domains": ["backend", "qa"],
      "IsDefault": false
    }
  ],
  "count": 2
}
```

---

### Governance Tools

Four read-only tools expose the roles, lock and catalog surface of the CLI. They never write a file and never use
the network: remote includes and installed skills resolve from the local cache only, and one that is not cached is
left out (the result then carries a second text block saying so). Each returns the JSON of the matching CLI command
with `--format json`, compacted onto one line, so a client can parse either source the same way. They are annotated
read-only and registered on the authoring server only (not on `--serve-skills`). Mutating lock operations
(`lock`, `update`) stay CLI-only.

All four take the common parameters `config_file`, `config_dir`, `no_local` and `working_directory`
(`lock_status` always ignores the machine-local overlay, as `lock --check` does).

| Tool | CLI equivalent | Parameters |
| --- | --- | --- |
| `list_roles` | `roles list --format json` | common only |
| `resolve_role` | `roles resolve <role> --format json` | `role` (required), `limit` |
| `lock_status` | `lock --check --format json` | `kind`, `profile`, `role`, `targets`, `include_static`, `sources` |
| `catalog` | `catalog --format json` | `kind`, `role`, `limit` |

**Output limits.** `resolve_role` and `catalog` list at most `limit` items (default 200, maximum 1000; a smaller
value wins). When the list is cut the document gains `"truncated": true` and `"total_items": <matching items>`;
`totals` and `roles` still describe every item. `list_roles` returns one entry per declared role, and
`lock_status` one change per differing item; neither is capped.

#### `resolve_role`

What a person holding a role gets: the document of [`roles resolve`](roles.md), with `role.items` capped at `limit`.

| Parameter | Type | Description |
| --- | --- | --- |
| `role` | string | Role name, as listed by `list_roles` (required) |
| `limit` | number | Maximum items returned |

#### `lock_status`

The comparison of `ai-rulez.lock` with the authored sources, the generated outputs, the skill sources and the served
skills ([`lock --check`](lockfile.md)), following `schema/lock-diff.schema.json`. A project without a lock is in
sync unless `[lock] enforce = true`.

| Parameter | Type | Description |
| --- | --- | --- |
| `kind` | string | List only the changes of one kind: `include`, `skill`, `source`, `served`, or `content` (authored items and generated outputs). `in_sync` still covers the whole lock |
| `profile` | string | Profile whose outputs are compared (default: the profile recorded in the lock); also selects the serve view |
| `role` | string | Compare only this role's outputs and check the skills it serves as a view of their own (`lock --role`) |
| `targets` | string | Also check the view that serves this preset's rendering of the skills (`lock --targets`) |
| `include_static` | boolean | Also check the view that serves static skills too (`lock --include-static`) |
| `sources` | string[] | Also check the view with these extra skill sources (`lock --source`) |

#### `catalog`

Every rule, skill, agent, command and context file with owner, version, tokens, digest, roles and the lock status
(see [`catalog`](roles.md#integrating-an-identity-tool-or-ui)).

| Parameter | Type | Description |
| --- | --- | --- |
| `kind` | string | Only items of this kind: `rule`, `skill`, `agent`, `command`, `check` or `context` |
| `role` | string | Only the items this role keeps; `roles` shrinks to that role |
| `limit` | number | Maximum items returned |

---

## Common MCP Workflows

### Creating a Domain with Rules

```text
User: "Create a backend domain and add a database standards rule"

MCP Tool Sequence:
1. create_domain(name: "backend", description: "Backend services")
2. create_rule(name: "database-standards", domain: "backend", priority: "high", content: "...")
3. generate_outputs() - regenerate configurations
```

### Adding an External Include

```text
User: "Add our corporate rules from GitHub"

MCP Tool Sequence:
1. add_include(name: "corporate", source: "https://github.com/myorg/rules", ref: "main")
2. validate_config() - check if include is valid
3. generate_outputs() - regenerate with included content
```

### Setting Up Team Profiles

```text
User: "Create backend and frontend profiles for our team separation"

MCP Tool Sequence:
1. create_domain(name: "backend")
2. create_domain(name: "frontend")
3. add_profile(name: "backend", domains: ["backend"])
4. add_profile(name: "frontend", domains: ["frontend"])
5. set_default_profile(name: "backend")
6. generate_outputs()
```

### Bulk Content Creation

```text
User: "Add security rules to the backend domain"

MCP Tool Sequence:
1. create_rule(name: "authentication", domain: "backend", content: "...")
2. create_rule(name: "encryption", domain: "backend", content: "...")
3. create_context(name: "security-architecture", domain: "backend", content: "...")
4. validate_config()
5. generate_outputs()
```
