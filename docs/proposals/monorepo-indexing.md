# Monorepo indexing over MCP

Status: accepted, in implementation. Branch `claude/monorepo-index`.

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

### Write path

`create_skill` and `update_skill` accept `scope`; a skill is created at `<scope>/<harness skills dir>/<name>/SKILL.md`. Rules:

- The target must resolve inside the project, with no symlink escapes, and must be a scope the index knows or an existing directory.
- Only skills directories of presets the configuration enables are writable, and generated copies (manifest-listed or with a verifying `Content-Hash` header) are refused.
- The content is scanned before it is written, as `install_skill` does, and the tool refuses on findings.
- Writes need the explicit consent flag `allow_write` on the tool call, as `judge` does for models.
- A nested write edits the harness file directly. It is not ai-rulez content and is not generated down; with an `ai-rulez.lock` present the new file is reported as drift until `lock` records it.

Instruction files and rules are read-only over MCP in this iteration.

## Phases

1. Scope model and index (kinds, scopes, resolution, collisions), `find_skill` `path`/`scope`, catalog `scopes`.
2. Nested instruction files, rules and agents: `find_context`, `load_context`, `context://` resources, server-instruction summary.
3. Cache (user, repo fallback, off), `ai-rulez cache clear`, doctor reporting, and the watcher with `list_changed`.
4. Scoped write path.

Each phase is test-first, ships with its documentation, and leaves the previous behavior unchanged when the feature is off.

## Open points

- Whether `[[scopes]]` names should flow into scope labels in the catalog. Default: yes, as an alias only.
- A cap on index size for very large repositories (default 20000 items, with a start-up warning past it).
