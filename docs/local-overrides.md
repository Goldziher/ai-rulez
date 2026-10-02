# Local Overrides

Add personal, machine-local instructions that are generated into `*.local.md` files and **never
committed**. Use them for scratch notes, per-machine paths, experiments, or anything that belongs to
your checkout but not to the shared configuration.

## Overview

Content under `.ai-rulez/local/` is a private overlay:

- It is scanned separately from the shared `rules/` and `context/` trees.
- It generates to per-preset `*.local.md` variants of each single-file root (e.g. `CLAUDE.local.md`),
  which the tool loads alongside its committed root file.
- Both the generated `*.local.md` outputs **and** the `.ai-rulez/local/` source tree are gitignored
  **unconditionally** — even when `gitignore = false` in `config.toml`.
- The committed root files (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`) never contain local content, so a
  teammate who checks out your branch sees only the shared configuration.

Only `rules` and `context` are supported under `.ai-rulez/local/`. There are no local domains, skills,
or agents.

## Source Layout

```text
.ai-rulez/
├── config.toml
├── rules/              # shared, committed
├── context/            # shared, committed
└── local/              # machine-local, gitignored
    ├── rules/
    │   └── my-scratch-notes.md
    └── context/
        └── local-env-notes.md
```

## Generated Output

Each preset that emits a single-file markdown root gets a `.local` sibling for local context, and for
local rules the preset keeps in its root file. Presets whose output is a directory of rule files
(`cursor`, `windsurf`, `cline`, `continue-dev`) have no root file; they receive local rules as
[rule files](#local-rule-files) only.

Duplicate local paths collapse: `codex`, `opencode`, `amp`, and `xum` all root on `AGENTS.md`, so they
share a single `AGENTS.local.md`.

| Preset     | Committed root       | Local override output |
| ---------- | -------------------- | --------------------- |
| `claude`   | `CLAUDE.md`          | `CLAUDE.local.md`     |
| `codex`    | `AGENTS.md`          | `AGENTS.local.md`     |
| `opencode` | `AGENTS.md`          | `AGENTS.local.md`     |
| `amp`      | `AGENTS.md`          | `AGENTS.local.md`     |
| `xum`      | `AGENTS.md`          | `AGENTS.local.md`     |
| `gemini`   | `GEMINI.md`          | `GEMINI.local.md`     |

Any other preset with a single-file markdown root follows the same `<root>` → `<root>.local` rule
(for example `hermes` → `.hermes.local.md`, `junie` → `.junie/guidelines.local.md`).

## Local Rule Files

Where a built-in preset routes rules to rule files, a rule in `.ai-rulez/local/rules` is written as
`<rulesdir>/<id>.local<ext>` instead of landing in the `*.local.md` root, so the tool loads it natively:

| Preset   | Local rule file                              |
| -------- | -------------------------------------------- |
| `claude` | `.claude/rules/my-rule.local.md`             |
| `cursor` | `.cursor/rules/my-rule.local.mdc`            |
| `copilot`| `.github/instructions/my-rule.local.instructions.md` |

Routing is the same as for shared rules (see [Rules](rules.md#rules-mode)):

- With the default `[rules] mode = "split"`, every local rule becomes a rule file.
- With `mode = "inline"`, only path-scoped local rules do, for presets that scope rules.
- Cursor, Windsurf, Cline and Continue always write rule files.
- Copilot keeps `auto`, `manual` and negated-only-glob rules inline, as for shared rules.

Local context still goes to the `*.local.md` root file. Custom provider presets get no local rule files.

Bookkeeping:

- The files are gitignored through one `<rulesdir>/*.local.*` pattern per folder (for example
  `.claude/rules/*.local.*`), written before the files themselves.
- They are tracked in `.ai-rulez/.generated-manifest.local.json`, which is gitignored, not in the
  committed manifest. A teammate's `generate` therefore never deletes your local files; `clean` removes
  them through the local manifest.
- Names matching `*.local.*` in a rules folder are reserved. An existing hand-written file with such a
  name is skipped with a warning.
- Local rules or context a preset has no place for (for example local context with Cursor, or unscoped
  local rules with Copilot in inline mode) are not written. `generate` reports them in one warning per
  preset.

## CLI

Create local override content with the `--local` flag on `add rule` / `add context`:

```bash
# Machine-local rule → .ai-rulez/local/rules/my-scratch-notes.md
ai-rulez add rule my-scratch-notes --local

# Machine-local context → .ai-rulez/local/context/local-env-notes.md
ai-rulez add context local-env-notes --local
```

`--local` is mutually exclusive with `--domain` — local content can never belong to a domain, so
combining the two flags errors.

## Workflow

1. Add local content:

   ```bash
   ai-rulez add rule local-paths --local
   ```

2. Edit the generated source file under `.ai-rulez/local/rules/` (or `context/`).

3. Regenerate:

   ```bash
   ai-rulez generate
   ```

   This writes `CLAUDE.local.md` (and the other applicable `*.local.md` files) and ensures both the
   outputs and `.ai-rulez/local/` are listed in `.gitignore`.

4. Commit as usual. The `*.local.md` files and `.ai-rulez/local/` stay out of the commit; only the
   shared configuration and its committed root files are versioned.

## Notes

- The unconditional gitignore is a safety guarantee: local content is intended for personal or
  machine-specific instructions and must never leak into version control.
- Because local content is a separate overlay, removing `.ai-rulez/local/` (or an individual file
  under it) and regenerating cleanly drops the corresponding `*.local.md` output.

## Related

- [Configuration Reference](configuration.md#gitignore) — gitignore behavior
- [CLI Commands](cli.md) — `add rule` / `add context`
