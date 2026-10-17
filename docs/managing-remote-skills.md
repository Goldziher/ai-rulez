# Managing only remote skills and plugins

Use this when you write `CLAUDE.md`, `AGENTS.md` and your own skills by hand and want ai-rulez to manage the
configured components: skills installed from a remote repository, plugins, MCP servers, hooks and permissions, pinned
in `ai-rulez.lock` (plus its own MCP server entry, see [below](#the-ai-rulez-mcp-server)). ai-rulez never writes, overwrites or deletes the hand-written files.

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

## The ai-rulez MCP server

`[mcp] self_server` defaults to `true`, so `generate` also adds an `ai-rulez` entry (`npx -y ai-rulez@latest mcp`) to
`.mcp.json`, and `ai-rulez.lock` gains `[[served]]` pins for the skills and an `.mcp.json` output pin. When skills are
served the entry runs `mcp --serve-skills`. To opt out set `self_server = false` under `[mcp]` or pass
`generate --no-self-mcp`; see [`[mcp]`](configuration.md#mcp).

## What happens to existing files

- A hand-written file at a skipped path is never read as drift, never reported as an orphan and never removed by
  `generate` or `clean`.
- A file ai-rulez generated earlier and that is now skipped is left in place. `generate` warns once that it is no
  longer managed and drops it from the manifest, so you can edit or delete it yourself. The notice is recorded in the
  machine-local manifest, so later runs of `generate`, `generate --check` and `clean` do not repeat it; deleting the
  file clears the record.
- `generate` stays quiet about a missing `AGENTS.md` when the skip is intentional.

## Troubleshooting

- **`refusing to overwrite 1 existing file(s) ai-rulez cannot prove it wrote`.** `generate` wants to write a path
  where a file exists that carries no ai-rulez `Content-Hash` header and is not in the manifest, so it may be
  hand-written. Nothing is written. Either import the file (`ai-rulez convert --write`), move or delete it, or pass
  `generate --force` to overwrite it. To keep a hand-written file at a path ai-rulez would otherwise render, list that
  output in `skip_outputs`.
- **An installed skill has the same name as a hand-written skill.** In `.claude/skills/<name>/` or
  `.agents/skills/<name>/` this is the refusal above. Against a skill in `.ai-rulez/skills/` (directory layout) the
  local skill wins and `generate` warns `Installed skill name conflicts with local skill, skipping`. Rename or remove
  one of them (`ai-rulez skill remove <name>` drops the installed entry).
- **`generate --refresh-remote` refuses with "the security scan of the new tree has error findings".** Nothing was
  written and `ai-rulez.lock` is unchanged. Review the listed findings in the new version first. If you accept
  them, run `ai-rulez generate --refresh-remote --accept-findings` to pin the new version and generate in one step
  (or `ai-rulez update --accept-findings`, then `ai-rulez generate`; `ai-rulez lock --accept-findings` re-pins what is
  already pinned and does not move it). `generate --refresh-remote --accept-findings` also lets the accepted tree
  through the import scan of that run, so the lock and the outputs agree. The acceptance is not recorded: a later
  `generate` scans the imported content again at the entry's `trust` level, and error findings still stop it unless you
  set `trust = "warn"` on the entry or `[lint.security] scan_imports = "warn"`. The served-skill scan uses the same
  `trust` of the entry.
