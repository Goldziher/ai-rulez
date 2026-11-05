# Config without a .ai-rulez folder

Keep the whole ai-rulez configuration in one file, `ai-rulez.toml` at the project root, with no `.ai-rulez/` directory. Use it when the repository already has hand-written `CLAUDE.md`, `AGENTS.md` and skills, and you want ai-rulez only for what a configuration file can declare.

## Create it

```bash
ai-rulez init --minimal
```

This writes only `ai-rulez.toml`: a `#:schema` first line (editors such as taplo and Even Better TOML read it to find the [JSON schema](schema.md)), `version`, `name`, `presets` and commented examples of `[[installed_skills]]` and `[lint]`. An existing file is replaced only with `--force`, and `--minimal` cannot be combined with `--source agents`, `--from`, `--domains` or `--config-dir`.

Then declare what ai-rulez should manage, for example:

```toml
version = "5.0"
name = "acme"
presets = ["claude", "codex"]

[[installed_skills]]
name = "kreuzberg"
source = "https://github.com/kreuzberg-dev/kreuzberg"
include = ["xberg*"]
```

```bash
ai-rulez generate
```

## What it does

A root `ai-rulez.toml` can declare:

- remote content: `[[installed_skills]]`, `[[installed_plugins]]`, `[[skill_sources]]` and `[[includes]]`
- `[[mcp_servers]]`, `[[hooks]]` and `[permissions]`, translated into each tool's native format
- `[lint]` settings for `ai-rulez validate`
- `ai-rulez.lock` settings (`[lock]`), pinning the remote sources and the generated outputs

`generate` installs the skills into each tool's skill folder and writes the MCP servers, hooks and permissions (for `claude`, including the default [`SessionStart` hook](session-start.md)). It writes no instruction file: `AGENTS.md`, `CLAUDE.md`, rule files and the `llms-txt` and `okf` indexes are never rendered, and a hand-written file at those paths is never overwritten or deleted. `clean` removes only what `generate` wrote.

Files in the project root:

| File | Purpose | Committed |
| --- | --- | --- |
| `ai-rulez.toml` | The configuration | yes |
| `ai-rulez.lock` | Pins written by `ai-rulez lock` | yes |
| `.ai-rulez-manifest.json` | The paths `generate` wrote | yes |
| `.ai-rulez-manifest.local.json` | Machine-local claims and digests | no, git-ignored |
| `ai-rulez.local.toml` | Optional [machine-local overlay](local-overrides.md#config-overlay) | no, git-ignored |

The local manifest is excluded through `.git/info/exclude`; once an overlay exists, the managed `.gitignore` block lists it and the local manifest. With `gitignore = true` the block also lists the overlay's lock and temp files (`.ai-rulez-local.*`), the generated outputs and `.ai-rulez-manifest.json`.

Everything else in the [configuration reference](configuration.md) applies, except what needs a content tree: settings that shape root instruction files and rule files (`agents_md`, `claude_md`, `compact`, `[[scopes]]`) have nothing to render, and layers are not supported; see [Limits](#limits).

## Discovery order

`ai-rulez.toml` comes last in the [discovery order](configuration.md#configuration-discovery), so a config directory always wins: when one exists beside an `ai-rulez.toml`, `generate` and `doctor` warn that the file is ignored:

```text
WARN  ai-rulez.toml is ignored: the .ai-rulez/ directory takes precedence hint=keep one configuration; move settings you still need into .ai-rulez/config.toml
```

`--config ai-rulez.toml` (or `--config .`) selects the file explicitly, and `generate --recursive` finds an `ai-rulez.toml` that has no config directory beside it. A flat `ai-rulez.yaml` is refused with a pointer to `ai-rulez migrate v5`.

The [`.agents` source](agents-source.md) also uses a file named `ai-rulez.toml`, but in `.agents/`, with `AGENTS.md` and `.agents/` as content and the manifest, lock and local state in `.agents/.ai-rulez/`. In this layout the file is in the project root, there is no content, and the state files sit in the root under the names above.

## CLI behavior

- `skill install|remove`, `include add|remove`, `lock`, `update --write-config` and the `local` commands edit `ai-rulez.toml` and `ai-rulez.local.toml`. A command that rewrites the file drops its comments.
- `add` and `remove` of `rule`, `context`, `skill`, `agent`, `command` and `check`, and `domain add`, fail: there is no content tree.

  ```text
  Error: Failed to add rule: cannot write content: no .ai-rulez/ directory (single-file configuration)
  ```

- `validate` and `scan` lint the configuration, the lock, installed and included content, and the hand-written instruction files they find. `doctor` reports drift, missing generated files and the environment checks, and does not lint instruction files. An empty content tree is not an error.

## Lint-only configuration

A project that uses ai-rulez only as a linter needs no preset. Leave `presets` out, or set `presets = []`:

```toml
version = "5.0"
name = "lint"

[lint]
profile = "strict"
```

With no preset (and no `[plugin]` block), `validate`, `scan`, `[[lint.external]]`, `lock`, `sbom` and the other read commands work as usual, and `generate` writes nothing and exits 0:

```text
INFO  No presets are configured: nothing to generate (lint-only configuration)
```

Hand-written files such as `AGENTS.md` or `.claude/skills/` are never touched, and the default `agents_md = true` does not make `generate` write `AGENTS.md`. `generate --check` and `doctor` report no drift. Files an earlier run wrote stay until you run `ai-rulez clean`. A lint-only configuration works the same in the `.ai-rulez/` directory layout.

## The ai-rulez MCP server

`[mcp] self_server` defaults to `true`, so `generate` writes `.mcp.json` with an `ai-rulez` server entry (`mcp --serve-skills` when skills are served), and `ai-rulez.lock` gains `[[served]]` pins and an `.mcp.json` output pin. To keep `.mcp.json` limited to your `[[mcp_servers]]`, turn it off:

```toml
[mcp]
self_server = false
```

`generate --no-self-mcp` does the same for one run.

## Limits

- No content is read: `rules/`, `context/`, `skills/`, `agents/`, `commands/`, `checks/`, `domains/` and `verifiers/` folders in the project root are not ai-rulez content. Verifiers come from `[[verifiers]]` and imports.
- Rules and context from an `[[includes]]` entry have no instruction file to land in; only outputs that are not root instruction files (skills, rule files of tools with a rules folder) are written.
- `default_layers` and `config.<name>.toml` layers are not supported.
- Opt-in state (skills index, role manifest, scanner baseline, eval results) is written to the project root, and usage logs to `.ai-rulez-local.d/`. The telemetry commands and `improve` look for `.ai-rulez/`.
- The MCP `init_project` tool creates the directory layout only.

## Troubleshooting

**`refusing to overwrite 1 existing file(s) ai-rulez cannot prove it wrote`.** `generate` would write a path where a file exists that has no ai-rulez `Content-Hash` header and is not in the manifest, so it may be hand-written. Nothing is written. Import the file (`ai-rulez convert --write`), move or delete it, or pass `generate --force` to overwrite it. To keep a hand-written file at a path ai-rulez would render, list that output kind in [`skip_outputs`](configuration.md#skip_outputs).

**An installed skill has the name of a hand-written skill.** In `.claude/skills/<name>/` or `.agents/skills/<name>/` this is the refusal above. Rename or remove one of them; `ai-rulez skill remove <name>` drops the installed entry.

**`generate --refresh-remote` refuses with "the security scan of the new tree has error findings".** Nothing was written and `ai-rulez.lock` is unchanged. Review the findings in the new version. To accept them, run `ai-rulez generate --refresh-remote --accept-findings`, or `ai-rulez update --accept-findings` and then `ai-rulez generate`. `ai-rulez lock --accept-findings` only re-pins what is already pinned; it does not move to the new version. `generate` scans imported content at the entry's `trust` level, so error findings stop it unless the entry sets `trust = "warn"` or `[lint.security] scan_imports = "warn"` is set.
