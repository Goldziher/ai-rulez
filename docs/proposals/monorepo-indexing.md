# Monorepo indexing over MCP

Status: implemented (phases 1 to 4 done). Branch `claude/monorepo-index`.

## Problem

A harness loads a nested `CLAUDE.md`, `AGENTS.md`, `.claude/`, `.codex/` or `.agents/` tree only once it works inside that directory. A planner at the repository root, or an agent planning a change across packages, never sees them. `mcp --serve-skills --discover-skills=repo` already serves skills found at any depth, but it serves them as a flat list with no notion of where they apply, it ignores nested instruction files, rules and agents, it builds its catalog once at start-up, and it cannot write to a nested location.

## Goals

1. Let an agent ask "what applies to this path?" and get the nearest scope's items plus its ancestors', as a harness would resolve them.
2. Serve nested instruction files and rules, not only skills.
3. Stay fast on a large tree and stay current without a restart.
4. Create and update skills at a nested location.

Non-goals: generating anything (the index is read from files that already exist), and replacing a harness's own loading.

## Design

### Scope model

A scope is a directory that holds at least one convention file or directory: `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.claude/`, `.agents/`, `.codex/`, `.cursor/rules`, `.github/copilot-instructions.md` or `skills/`. Scopes are inferred from the tree; `[[scopes]]` entries in the configuration add a name or override a path but are not required. The root is the scope `.`.

Every indexed item carries:

| Field | Meaning |
|---|---|
| `kind` | `skill`, `instruction`, `rule`, `agent`, `command` |
| `scope` | The directory the item lives under, relative to the project root, `/`-separated |
| `source` | The convention it came from (`claude`, `agents`, `codex`, `cursor`, `copilot`, `gemini`, `skills-dir`) |
| `name` | The item name; a colliding name is namespaced by path, as in discovery today |
| `description` | The frontmatter description, or the first heading of an instruction file |
| `hash` | Content hash, which keys the scan verdict and the cache |

Resolution for a path `p`: the scopes that are `p` or an ancestor of `p`, nearest first. The nearest scope wins a name collision, matching how harnesses shadow.

### MCP surface

- `find_skill` and the catalog accept `path` (resolve for this path) and `scope` (this exact scope, plus descendants when `recursive` is true).
- A new `find_context` tool and `context://<scope>/<name>` resources serve nested instruction files, rules and agents. They apply the same scan, budget and `--allow`/`--deny` rules as skills. Search returns summaries; `load_context` returns the body, read from the repository at call time.
- `ai-rulez://catalog` gains a `scopes` section: each scope with its item counts and sources, so a planner can see the shape of the repository in one read.
- Server instructions carry a short capped summary of the scopes (the first N by depth, then "and M more") so a planner knows the nested content exists.

Enabled by `[mcp] discover_skills = "repo" | "all"` (the existing key, extended to cover all kinds) and `--discover-skills`. Off by default.

### Index and cache

The index stores summaries only: it never stores file bodies, so a stale index cannot serve stale instructions.

- **Default location: the user cache.** `<cache>/ai-rulez/monorepo-index/<repo-id>/`, where `<cache>` comes from the existing `config.CacheDir` helper (`$XDG_CACHE_HOME`, else `~/.cache` on Linux and the platform cache directory elsewhere; `AI_RULEZ_HOME` overrides) and `<repo-id>` is a hash of the canonical project root. Nothing is written into the repository.
- **Fallback.** If the user cache is not writable, a gitignored directory in the project is used (`.ai-rulez/.cache/`, or `.ai-rulez-cache/` in the root layout), excluded through `.git/info/exclude` so the working tree stays clean. `ai-rulez doctor` reports which location is in use.
- **Setting.** `[mcp] index_cache = "user" | "repo" | "off"`. Default `user` with the fallback above.
- **Entry key.** Path, size and mtime; the file list comes from `git ls-files --cached --others --exclude-standard` (a plain walk without git). The scan verdict is stored against the content hash and re-run when it changes.
- **Safety.** Files are written with mode 0600 through write-and-rename; several servers on one repository share the cache under a lock file. A cache entry never bypasses the scan or the allow/deny rules, which run again at load time. A corrupt or version-mismatched cache is discarded and rebuilt.
- **Freshness.** On start the server loads the cache and re-stats only changed paths. While running, a watcher (fsnotify, debounced) updates the index incrementally and the server sends `resources/list_changed`, so a new `.claude/skills` is picked up without a restart. `ai-rulez cache clear` drops the cache.

### Measured cost

A cold build of the index (`repoindex.Build`, no cache) on a synthetic monorepo takes 11 ms for 100 files, 47 ms for
5000 files over 100 scopes and 129 ms for 20000 files over 500 scopes (1500 items) with `git ls-files`; 202 ms with
the directory walk. Of that, the listing is 36 ms (git) or 67 ms (walk) for 20000 files and reading the items about
20-35 us each. `Resolve` of a path on the built index is 2.4 us. So a cache of the file list and the item summaries
would save at most 5-130 ms per start, which is small next to the rest of server start-up: building the catalog of the
same monorepo, with the security scan of every served item, takes 65 ms, 0.3 s and 1.2 s for 30, 300 and 1500 items,
and the scan is most of it. The cache is therefore worth building for the scan verdict keyed by content hash, not for
the index. `docs/performance.md` has the tables and the machine they were taken on.

### Write path

`create_skill` and `update_skill` accept `scope`; a skill is created at `<scope>/<harness skills dir>/<name>/SKILL.md`. Rules:

- The target must resolve inside the project, with no symlink escapes, and must be a scope the index knows or an existing directory.
- Only skills directories of presets the configuration enables are writable, and generated copies (manifest-listed or with a verifying `Content-Hash` header) are refused.
- The content is scanned before it is written, as `install_skill` does, and the tool refuses on findings.
- Writes need the explicit consent flag `allow_write` on the tool call, as `judge` does for models.
- A nested write edits the harness file directly. It is not ai-rulez content and is not generated down; with an `ai-rulez.lock` present the new file is reported as drift until `lock` records it.

Instruction files and rules are read-only over MCP in this iteration.

## Phases

1. Scope model and index (kinds, scopes, resolution, collisions), `find_skill` `path`/`scope`, catalog `scopes`. **Done** (`internal/repoindex`).
2. Nested instruction files, rules and agents: `find_context`, `load_context`, `context://` resources, server-instruction summary. **Done.**
3. Cache (user, repo fallback, off), `ai-rulez cache clear`, doctor reporting, and the watcher with `list_changed`. **Done** (`internal/indexcache`).
4. Scoped write path. **Done.**

Each phase is test-first, ships with its documentation, and leaves the previous behavior unchanged when the feature is off.

## Implementation notes (phases 1 and 2)

- Shadowing by name applies to skills, agents and commands. Instruction files and rules add up across scopes, as harnesses load them.
- Commands are indexed and counted in the catalog but not served by `find_context`.
- `skills/` and the enabled presets' skill directories are skill roots at any depth, not only at the project root.
- `find_context` and `load_context` are registered only when discovery is on, so the tool list is unchanged otherwise.
- The catalog narrows through query parameters on the `ai-rulez://catalog` resource; the `catalog` tool is unchanged.
- `context://` is a resource template and is not enumerated by `resources/list`.

## Implementation notes (phases 3 and 4)

- The cache stores two things: the summary of each indexed file (keyed by path, size, modification time and the HEAD
  tree; a file modified within two seconds before a save is not trusted, as git's racy-timestamp rule) and the scan
  verdict of content with no finding at all, keyed by the scan level and the content hashes. A verdict is dropped when
  the ai-rulez build or the `[lint]` settings change.
- The proposal says the scan runs again at load time. `load_context` scans the body it reads on every call. For a
  skill, the catalog build is its load, and it trusts a cached clean verdict instead of scanning again. The verdict is
  only trusted if the index file's HMAC verifies. The secret is per user, kept outside the cache
  (`~/.config/ai-rulez/index-cache.key`), so a planted or edited file is discarded; a skill with any finding is
  scanned every time.
- Live refresh rebuilds the whole catalog on a debounced fsnotify event; it is incremental in cost (the cache),
  not in structure. `resources/list_changed` is sent by re-registering the `ai-rulez://index` resource, because
  `context://` is a template that `resources/list` does not enumerate. The server instructions are not rewritten.
- The write path accepts any existing directory of the project as a scope (every scope the index knows is one), so
  it does not need the index. It adds an optional `harness` argument to pick the preset's skills directory. The CLI
  has no equivalent: a person edits the file. `update_skill` is annotated destructive and not idempotent.
- `watch` skips `.git`, `node_modules`, `vendor`, `target`, `dist`, `build` and other output directories by name
  rather than reading `.gitignore`, so a gitignored directory with another name is watched.

## Implementation notes (discovery fixes)

- Rules, agents and commands are indexed in every known convention; `presets` only chooses the skill directories
  searched. A lint-only configuration (`presets = []`) therefore serves the same context as no configuration.
- `[mcp] exclude` and `--exclude` are globs applied to the file listing before any file is classified or read, so an
  excluded file costs nothing and marks no scope. The watcher ignores the same paths.
- A rule's `paths:` (Claude Code) or `globs:` (Cursor) are kept in the index and its cache entries (cache format 2),
  and `find_context` with `path` filters on them, relative to the scope directory.
- Scan findings of a file of context name its `context://` URI; an error finding withholds the file at every trust
  level.
- Two repository skills with one name: the later is served under its path and reported in `skill_discovery.renamed`.

## Open points

- Whether `[[scopes]]` names should flow into scope labels in the catalog. Default: yes, as an alias only.
- A cap on index size for very large repositories (default 20000 items, with a start-up warning past it).
