---
title: "Managing only remote skills and plugins"
description: "Use this setup when you write CLAUDE.md, AGENTS.md and your own skills by hand and want ai-rulez to manage only the parts that come from elsewhere: installed skills, plugins, MCP servers, hooks and permissions, all pinned in ai-rulez.lock."
---

Use this setup when you write `CLAUDE.md`, `AGENTS.md` and your own skills by hand and want ai-rulez to manage only
the parts that come from elsewhere: [installed skills](/ai-rulez/installed-skills/), [plugins](/ai-rulez/installed-plugins/), MCP
servers, hooks and permissions, all pinned in `ai-rulez.lock`. ai-rulez never writes, overwrites or deletes your
hand-written instruction files.

There are two ways to set it up:

| Setup | Configuration | Use it when |
| --- | --- | --- |
| Root file | `ai-rulez.toml` at the project root, no `.ai-rulez/` folder | You have no ai-rulez content of your own |
| Skipped outputs | `.ai-rulez/config.toml` with `skip_outputs = ["instructions"]` | You also keep skills or path-scoped, auto or manual rules in `.ai-rulez/`. Always-on rules and context are not rendered anywhere in this setup |

## Example

```bash
ai-rulez init --minimal
ai-rulez skill install ai-rulez --source Goldziher/ai-rulez --version '^5' --trust warn
```

```toml
# ai-rulez.toml
version = "5.0"
name = "acme"
presets = ["claude", "codex"]

[[installed_skills]]
name = 'ai-rulez'
source = 'Goldziher/ai-rulez'
version = '^5'
trust = 'warn'    # the skill runs `npx ai-rulez@latest`, an unpinned package run (AR021)

[[mcp_servers]]
name = "docs"
command = "uvx"
args = ["docs-mcp==1.4.0"]
```

```bash
ai-rulez lock        # pin the skills and the generated outputs
ai-rulez generate
```

`ai-rulez.lock` records the tag the range resolved to:

```toml
[[skill]]
name = 'ai-rulez'
source = 'https://github.com/Goldziher/ai-rulez'
path = 'skills/ai-rulez'
ref = '^5'
tag = 'v5.10.0'
commit = 'bfd148a002b552141a32c496a31c507a3b1467f8'
...
```

`generate` writes the skill into `.claude/skills/` and `.agents/skills/`, the MCP servers into `.mcp.json` and
`.codex/config.toml`, and leaves `CLAUDE.md` and `AGENTS.md` exactly as you wrote them. See
[Config without a .ai-rulez folder](/ai-rulez/config-without-folder/) for everything the root file supports.

## Skipping instruction files

In a project with a `.ai-rulez/` folder, list the files you write by hand in
[`skip_outputs`](/ai-rulez/configuration/#skip_outputs):

```toml
# .ai-rulez/config.toml
version = "5.0"
name = "acme"
presets = ["claude", "codex"]
skip_outputs = ["instructions"]
```

| Value | Skips |
| --- | --- |
| `instructions` | Every root instruction file: `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` and the other presets' files, their `*.local.md` and `AGENTS.override.md` variants, and each scope's root files |
| `agents_md` | Only the shared `AGENTS.md` and `.agents/skills` |
| `claude_md` | Only `CLAUDE.md` |

Do not add `skills`: it skips every skill directory, installed skills included.

Skills you write by hand in `.claude/skills/` or `.agents/skills/` are safe as long as their names differ from the
skills ai-rulez writes: ai-rulez deletes only files that carry its `Content-Hash` header or that its manifest lists.

## Keeping skills current

```bash
ai-rulez generate --locked     # fail if ai-rulez.lock does not match
ai-rulez generate --check      # report drift; skipped files are never reported
ai-rulez update                # move version ranges to the newest allowed tag
```

To pick up new releases on every run, set `refresh_on_generate = true` under `[lock]`, or run
`ai-rulez generate --refresh-remote` once. The refresh resolves each source to the newest version its constraint
allows, scans the new trees and rewrites `ai-rulez.lock`. A scan finding stops the run before any file is written;
review it, then add `--accept-findings`. See [Refreshing on generate](/ai-rulez/lockfile/#refreshing-on-generate).

## The ai-rulez MCP server

By default (`[mcp] self_server = true`), `generate` also adds an `ai-rulez` entry to the MCP file of each
configured harness (`.mcp.json`, `.cursor/mcp.json`, `.codex/config.toml`, ...; see
[Generated entries](/ai-rulez/mcp-server/#generated-entries)). In `.mcp.json`:

```json
"ai-rulez": {
  "args": ["-y", "ai-rulez@latest", "mcp"],
  "command": "npx",
  "type": "stdio"
}
```

When skills are served, the entry runs `mcp --serve-skills`, and `ai-rulez.lock` gains `[[served]]` pins. To leave
it out, set `self_server = false` under `[mcp]` or pass `generate --no-self-mcp`. See [MCP server](/ai-rulez/mcp-server/).

## Existing files

- A hand-written file at a skipped path is never reported as drift or as an orphan, and never removed by `generate`
  or `clean`.
- A file ai-rulez generated earlier at a path that is now skipped is left in place. `generate` warns once that it is
  no longer managed and drops it from the manifest; edit or delete it yourself.
- `generate` does not warn about a missing `AGENTS.md` when you skip it on purpose.

## Troubleshooting

**`refusing to overwrite 1 existing file(s) ai-rulez cannot prove it wrote: CLAUDE.md`**

`generate` would write a path where a file exists that has no ai-rulez header and is not in the manifest. Nothing is
written. Add the output to `skip_outputs` to keep your file, import it with `ai-rulez convert --write`, move it, or
pass `generate --force` to overwrite it.

**An installed skill has the same name as a hand-written skill**

In `.claude/skills/<name>/` or `.agents/skills/<name>/` this is the refusal above. Against a skill in
`.ai-rulez/skills/`, your skill wins and `generate` warns `Installed skill name conflicts with local skill, skipping`.
Rename one of them, set `name_prefix` on the entry, or remove it with `ai-rulez skill remove <name>`.

**`generate --refresh-remote` refuses: "the security scan of the new tree has error findings"**

Nothing was written and `ai-rulez.lock` is unchanged. Review the findings in the new version. To accept them:

```bash
ai-rulez generate --refresh-remote --accept-findings
```

This pins the new version and lets it through that run's import scan. The acceptance is not recorded: error
findings stop a later `generate` again unless you set `trust = "warn"` on the entry or
`[lint.security] scan_imports = "warn"` (see [Installed skills: Security](/ai-rulez/installed-skills/#security)). `ai-rulez lock --accept-findings` re-pins what is already
pinned; it does not move to a new version.
