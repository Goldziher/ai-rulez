# Managing only remote skills and plugins

Use this when you write `CLAUDE.md`, `AGENTS.md` and your own skills by hand and want ai-rulez to manage only the
configured components: skills installed from a remote repository, plugins, MCP servers, hooks and permissions, all
pinned in `ai-rulez.lock`. ai-rulez never writes, overwrites or deletes the hand-written files.

## Configure

```toml
version = "5.0"
presets = ["claude", "codex"]

# Hand-written: CLAUDE.md, AGENTS.md and every other root instruction file.
skip_outputs = ["instructions"]

[[installed_skills]]
name = "kreuzberg"
source = "https://github.com/kreuzberg-dev/kreuzberg"
version = "^1.2"

[[mcp_servers]]
name = "docs"
command = "uvx"
args = ["docs-mcp"]
```

`instructions` skips every root instruction file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, the files of the other
presets, their `*.local.md` and `AGENTS.override.md` variants, and each scope's root). Installed skills, MCP servers,
hooks and permissions are still emitted. Skills you write by hand under `.claude/skills/` or `.agents/skills/` are
left alone as long as their names differ from the skills ai-rulez writes: ai-rulez only deletes files that carry its
own `Content-Hash` header or that its manifest lists. Do not add `skills` to `skip_outputs` unless you want to drop
installed skills too; it skips every skill directory.

For a narrower choice, `agents_md` skips only the shared `AGENTS.md` and `.agents/skills`, and `claude_md` skips only
`CLAUDE.md`. See [`skip_outputs`](configuration.md#skip_outputs) for all values.

## Lock and generate

```bash
ai-rulez lock                 # pin the remote skills and the generated outputs
ai-rulez generate --locked    # fail if ai-rulez.lock does not match
ai-rulez generate --check     # report drift; hand-written files are never reported
```

To pick up new releases of the remote skills on every run, add `refresh_on_generate = true` to `[lock]`, or run
`ai-rulez generate --refresh-remote` once. The refresh re-resolves each source to the newest version its constraint
allows, scans the new trees and rewrites `ai-rulez.lock`; a scan finding aborts before any file is written (review it, then add `--accept-findings`). See
[Refreshing on generate](lockfile.md#refreshing-on-generate).

## What happens to existing files

- A hand-written file at a skipped path is never read as drift, never reported as an orphan and never removed by
  `generate` or `clean`.
- A file ai-rulez generated earlier and that is now skipped is left in place. `generate` warns once that it is no
  longer managed and drops it from the manifest, so you can edit or delete it yourself. The notice is recorded in the
  machine-local manifest, so later runs of `generate`, `generate --check` and `clean` do not repeat it; deleting the
  file clears the record.
- `generate` stays quiet about a missing `AGENTS.md` when the skip is intentional.
