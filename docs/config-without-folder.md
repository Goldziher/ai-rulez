# Config without a .ai-rulez folder

Keep the whole ai-rulez configuration in one file, `ai-rulez.toml` at the project root, and no `.ai-rulez/` directory.

## When to use it

Use it when the repository already has hand-written `CLAUDE.md`, `AGENTS.md` and skills, and you only want ai-rulez for what a config file can declare:

- remote `[[installed_skills]]`, `[[skill_sources]]` and `[[includes]]`
- `[[mcp_servers]]`, `[[hooks]]` and `[permissions]`, translated into each tool's native format
- `[lint]` settings for `ai-rulez validate`
- `ai-rulez.lock`, which pins the remote sources and the generated outputs

It also adds ai-rulez's own MCP server to `.mcp.json` unless you [turn that off](#the-ai-rulez-mcp-server).

If you want ai-rulez to own the rules, context, skills and agents, use the [directory layout](configuration.md) instead.

## Create it

```bash
ai-rulez init --minimal
```

This writes only `ai-rulez.toml`, with a `#:schema` first line (editors such as taplo and Even Better TOML read it to find the [JSON schema](schema.md)) and commented examples of `[[installed_skills]]` and `[lint]`. `--minimal` cannot be combined with `--source`, `--from`, `--domains` or `--config-dir`. An existing file is only replaced with `--force`.

```toml
version = "5.0"
name = "my-project"
presets = ["claude", "codex"]

[[installed_skills]]
name = "kreuzberg"
source = "https://github.com/kreuzberg-dev/kreuzberg"
```

## What it does

`ai-rulez generate` installs the skills into each tool's skill folder, and writes MCP servers, hooks and permissions. It writes no instruction file: `AGENTS.md`, `CLAUDE.md`, rule files and the `llms-txt` and `okf` indexes are never rendered, and a hand-written file at those paths is never overwritten or deleted. `clean` removes only what `generate` wrote.

Files it creates in the project root:

| File | Purpose | Committed |
| --- | --- | --- |
| `ai-rulez.toml` | The configuration | yes |
| `ai-rulez.lock` | Pins from `ai-rulez lock` | yes |
| `.ai-rulez-manifest.json` | The paths `generate` wrote | yes |
| `.ai-rulez-manifest.local.json` | Machine-local claims and digests | no, ignored |
| `ai-rulez.local.toml` | Optional [machine-local overlay](local-overrides.md) | no, ignored |

The managed `.gitignore` block covers the local manifest, the overlay and its lock. With `gitignore = true` it also lists the generated outputs and `.ai-rulez-manifest.json`.

## Discovery order

Per directory, while walking up from the working directory to the repository root (never above the home directory): `.agents/ai-rulez.toml`, `.ai-rulez/config.toml`, `.config/ai-rulez/config.toml`, then `ai-rulez.toml`. A config directory always wins; when one exists beside an `ai-rulez.toml`, `generate` and `doctor` warn that the file is ignored. `--config ai-rulez.toml` (or `--config .`) selects the file explicitly. `generate --recursive` finds an `ai-rulez.toml` that has no config directory beside it. A flat `ai-rulez.yaml` is still refused with the `ai-rulez migrate v5` message.

## How it differs from the `.agents` layout

Both keep the configuration in a file named `ai-rulez.toml`. In the [`.agents` source](agents-source.md) the file lives in `.agents/`, the content is AGENTS.md and `.agents/`, and the manifest, lock and local state sit in `.agents/.ai-rulez/`. In this layout the file is in the project root, there is no content at all, and the state files sit in the root with the names above.

## CLI behavior

- `skill install|remove`, `include add|remove`, `lock`, `update --write-config` and the `local` commands edit `ai-rulez.toml` and `ai-rulez.local.toml`. Rewriting through a command drops comments, as it does for `config.toml`.
- `add|remove rule|context|skill|agent|command|check` and `domain add` fail: there is no content tree.
- `validate` and `scan` lint the configuration, the lock, the installed and included content, and any hand-written instruction files they detect. `doctor` does not lint instruction files; it reports drift, a missing generated file and the other environment checks. An empty content tree is not an error.

## Lint-only configuration

A project that uses ai-rulez only as a linter needs no preset. Leave `presets` out, or set `presets = []`:

```toml
version = "5.0"
name = "lint"

[lint]
profile = "strict"
```

With no preset (and no `[plugin]` block) `validate`, `scan`, `[[lint.external]]`, `lock`, `sbom` and the other read commands work as usual. `generate` prints `No presets are configured: nothing to generate`, writes nothing and exits 0, so hand-written files such as `AGENTS.md` or `.claude/skills/` are never touched, and `agents_md = true` (the default) does not make it write `AGENTS.md`. `generate --check` and `doctor` report no drift. Files an earlier run wrote stay until you run `ai-rulez clean`.

## The ai-rulez MCP server

`[mcp] self_server` defaults to `true`, so `generate` also writes `.mcp.json` with an `ai-rulez` server entry
(`mcp --serve-skills` when skills are served), and `ai-rulez.lock` gains `[[served]]` pins and an `.mcp.json` output
pin. Set `self_server = false` under `[mcp]`, or pass `generate --no-self-mcp`, to keep `.mcp.json` limited to your
`[[mcp_servers]]`.

## Limits

- No content is read: `rules/`, `context/`, `skills/`, `agents/`, `commands/`, `checks/`, `domains/` and `verifiers/` folders in the project root are not ai-rulez content. Verifiers come from `[[verifiers]]` and imports.
- Rules or context brought in by an `[[includes]]` entry have no instruction file to land in; only the files that are not root instruction files (skills, rule files of tools that read a rules folder) are written.
- `default_layers` and `config.<name>.toml` layers are not supported.
- Opt-in state (skills index, role manifest, scanner baseline, eval results) is written to the project root, and usage logs to `.ai-rulez-local.d/`. Telemetry commands and `improve` still look for `.ai-rulez/`.
- `ai-rulez mcp` `init_project` creates the directory layout only.

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
  already pinned and does not move it). `generate` scans the imported content again at the entry's `trust` level, so
  error findings still stop it unless you set `trust = "warn"` on the entry or `[lint.security] scan_imports = "warn"`.
