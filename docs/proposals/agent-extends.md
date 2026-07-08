# Proposal: `extends` — customize an inherited/builtin agent by appending a message

Status: proposed (design handoff)
Motivation: eliminate agent-definition duplication and a confirmed silent-override bug.

## Problem

To customize a built-in agent (e.g. `code-reviewer`) today, a repo or shared module must copy the
**entire** agent definition into `.ai-rulez/agents/<name>.md` and redefine it. This causes:

1. **Duplication / drift** — the full builtin body is copied into every module that wants a small tweak.
2. **A silent-override bug (confirmed)** — a same-named override only wins if it re-declares a `tools:`
   block. Without `tools:`, the generic **builtin body silently wins** and the override is discarded.
   Verified in `xberg-io/xberg`: the `code-reviewer`, `docs-writer`, and `security-auditor` overrides in
   `modules/core` (no `tools:` block) never reach any generated file — the generic builtin body is emitted
   instead. Adding a `tools:` block flips the winner. This discriminator is undocumented and surprising.

## Proposal

Add an optional `extends` field to agent frontmatter. When set, ai-rulez resolves a **base agent** and
produces a merged agent = base + this agent's overrides, instead of a full replacement.

```markdown
---
name: code-reviewer
extends: code-reviewer          # inherit the resolved base agent of this name (from lower layers)
model: sonnet                   # optional override; omitted fields inherit from base
effort: high                    # optional override
# tools omitted -> inherit base tools
---
Additional project-specific review checks (Rust polyglot):

- No `.unwrap()`/`.expect()` in library code
- FFI: a SAFETY comment on every `unsafe`, pointer validation, ownership documented
- API parity: cross-language changes keep feature parity across all bindings
```

Generated result = base `code-reviewer` body + `\n\n` + the appended message above, with `model`/`effort`
overridden and `tools` inherited from the base.

## Semantics

- **Layering.** ai-rulez resolves agents across three layers: builtins -> includes (modules) -> local.
  `extends: <name>` binds to the resolved same-named agent from the layers **below** the current one:
  - a local agent extends an included-module or builtin agent of that name;
  - a module agent extends a builtin agent of that name.
- **Self-name is the common case.** `name: code-reviewer` + `extends: code-reviewer` means "take the
  code-reviewer that would otherwise be resolved from lower layers, and append to it."
- **Frontmatter merge.** Fields set on the extending agent override the base; omitted fields inherit
  (`model`, `effort`, `tools`, `description`).
- **Body merge.** `base_body + "\n\n" + extending_body` (append-only in v1).
- **Chaining.** `A extends B`, `B extends builtin` concatenates in resolution order
  (builtin -> B -> A).
- **Missing base -> hard error.** If no lower-layer agent named `<extends>` exists, fail generation with
  a clear message (do not silently emit only the message). Detect and reject cycles.

## Also fix the existing full-override bug (recommended, same release)

For agents that do NOT use `extends` (full redefinition), a same-named override should **win regardless of
whether it declares a `tools:` block**. The current tools:-block discriminator is the root of the silent
drop above. Either fix the merge to let the last (highest-layer) full definition win unconditionally, or
document the requirement loudly. `extends` is the ergonomic path; this is the correctness backstop.

## Implementation touchpoints (Go)

- `internal/config/types.go` — add `Extends string` to the Agent struct (yaml/json/toml tags:
  `extends,omitempty`).
- Frontmatter parsing — parse `extends`.
- Agent resolution/merge (generator) — when building the by-name agent set across layers, if an agent has
  `extends`, resolve the base of that name from lower layers, merge frontmatter (extender wins), and
  concatenate bodies. Preserve builtin -> module -> local ordering.
- `internal/config/validation.go` — validate the `extends` target exists; reject cycles.
- Emit the merged frontmatter + concatenated body to all agent outputs (`.claude/agents`,
  `.codex/agents`, etc.). Codex output continues to ignore `model` (uses `effort`).

## Docs

Add an "Extending built-in agents" section to `docs/configuration.md` (draft in
`docs/proposals/agent-extends-docs.md`), with the example above and the precedence rules.

## Adoption in xberg-io modules (follow-up config PR, no tool change)

Convert `modules/core` `code-reviewer`, `docs-writer`, `security-auditor` (and any other full-copy
overrides) from full redefinition to `extends: <name>` + a short append message. Removes the duplicated
builtin body from each module file and eliminates the fragile override. (Interim: those three now carry a
`tools:` block so the override wins today; replace with `extends` once shipped.)
