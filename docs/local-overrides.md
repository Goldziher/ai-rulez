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

Each preset that emits a single-file markdown root gets a `.local` sibling. Presets whose output is a
directory of split rule files (`cursor`, `windsurf`, `copilot`, …) have no single-file root and are
**skipped** — they produce no local variant.

Duplicate local paths collapse: `codex`, `opencode`, and `amp` all root on `AGENTS.md`, so they share
a single `AGENTS.local.md`.

| Preset     | Committed root       | Local override output |
| ---------- | -------------------- | --------------------- |
| `claude`   | `CLAUDE.md`          | `CLAUDE.local.md`     |
| `codex`    | `AGENTS.md`          | `AGENTS.local.md`     |
| `opencode` | `AGENTS.md`          | `AGENTS.local.md`     |
| `amp`      | `AGENTS.md`          | `AGENTS.local.md`     |
| `gemini`   | `GEMINI.md`          | `GEMINI.local.md`     |

Any other preset with a single-file markdown root follows the same `<root>` → `<root>.local` rule
(for example `hermes` → `.hermes.local.md`, `junie` → `.junie/guidelines.local.md`).

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
