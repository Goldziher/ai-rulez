---
title: "Worktree provisioning"
description: "ai-rulez worktree create and ai-rulez worktree setup carry a checkout's AI-Rulez setup into a linked git worktree: the generated outputs are regenerated from the destination's own committed configuration, and the machine-local inputs move only on explicit consent."
---

A fresh linked git worktree inherits the tracked configuration from its branch, but the
checkout-local AI-Rulez inputs stay behind. The `worktree` commands carry a source checkout's
setup into a linked worktree of the same repository.

Run both commands from the **source checkout** — the one that holds the configuration. The
destination must be a linked worktree of the same repository, sharing its git common directory.

## The two entry points

`worktree create` makes the linked worktree itself: a new branch from `--start-ref` (default: the
source checkout's HEAD) checked out at the destination path. The branch must be new and the path
free. Use it when the agent or a script owns creation.

```bash
ai-rulez worktree create ../.worktrees/billing/fix-mcp --branch fix-mcp
ai-rulez worktree create ../.worktrees/billing/fix-mcp --branch fix-mcp --start-ref main
```

`worktree setup` provisions a worktree that git or the agent's host already created. It refuses a
destination that is not a registered linked worktree of the same repository.

```bash
ai-rulez worktree setup ../.worktrees/billing/fix-mcp
```

## What a run does

1. **Regenerate the destination's outputs** from the destination's own committed configuration —
   the shared configuration plus whatever machine-local inputs the run carried. Generated
   manifests, cached artifacts and rendered credentials are never copied, and no file content is
   ever printed.
2. **Carry the machine-local inputs** — only with `--copy-local`, and only what exists in the
   source:

   | Input | Where |
   | ----- | ----- |
   | Config overlay | `<config dir>/config.local.toml` |
   | Local content tree | `<config dir>/local/` |
   | Hand-authored local files | `CLAUDE.local.md`, `AGENTS.local.md`, `AGENTS.override.md`, `GEMINI.local.md`, `.claude/settings.local.json` |

   `--copy-extra <path>` adds a caller-named file or directory on top of the allowlist; extras are
   named one by one, never a bulk copy.

3. **Plan before writing.** With `--dry-run` nothing is written and the generate step reports its
   plan. A real run computes the same plan first, then writes.

## Guards

- Every copy target must be **ignored and untracked** in the destination: a tracked path would
  stage a machine-local file, and an unignored one would pollute `git status` on every checkout of
  the branch. The [managed `.gitignore` block](/ai-rulez/local-overrides/) carries the local
  inputs; commit it in the source first.
- Copied files and directories get **owner-only permissions** (`0600`/`0700`), and the run never
  follows a symlink.
- A destination file that already exists with **different content** is a conflict: the run refuses
  before the first write and preserves the destination's edits. An identical file is a no-op.
- Both checkouts must hold the configuration in the **same layout** for overlays; otherwise the
  copied overlay would land where the destination's own `generate` never reads it, and the run
  refuses.
- Failures report a **retryable state** and leave the worktree and its branches intact.

## Credentials and secrets

The machine-local inputs can hold secrets (the overlay's MCP environment values, a
`.claude/settings.local.json` token). They are carried file by file with owner-only permissions and
are never printed, never staged, and never reported with contents. `--copy-extra` names what you
want; nothing else travels.

## JSON output

`--format json` prints one document following
`schema/worktree-provision.schema.json` (in this repository, published with the release): what was
carried (`copies`), the generate step's lines (`plan`), and `conflicts` when the run refused.

## Agents

The generated root instructions carry a short worktree section: after git or the host creates a
linked worktree, run `worktree setup` from the source checkout — or `worktree create` when the
agent owns creation — and prefer the `worktree_setup` MCP tool when the client exposes it. The
default session-start briefing says the same in one line. For `source = "agents"` projects, the
authored `AGENTS.md` is preserved; the guidance lands in a generated file the harness actually
loads (the `CLAUDE.md` shim for Claude Code), or place it by hand where no such file exists.
