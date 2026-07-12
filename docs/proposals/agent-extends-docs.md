# Extending built-in agents

<!-- DRAFT for inclusion in docs/configuration.md once `extends` ships. -->

Built-in and shared-module agents give you a solid base (`code-reviewer`, `docs-writer`,
`security-auditor`, `polyglot-architect`, language specialists, ...). When you only need to _add_
project-specific guidance, don't copy the whole agent — **extend** it.

## Append a message to a built-in agent

Create `.ai-rulez/agents/<name>.md` with the same `name`, set `extends` to that name, and put your
additional instructions in the body:

```markdown
---
name: code-reviewer
extends: code-reviewer
---

Also enforce, for this repo:

- No `.unwrap()`/`.expect()` in library code.
- Every `unsafe` block carries a SAFETY comment.
- Cross-language changes keep API parity across all bindings.
```

The generated `code-reviewer` is the base agent's full instructions **plus** your message appended. You
did not restate the base checklist — you added to it.

## Override model / effort / tools while extending

Frontmatter fields you set win over the base; fields you omit are inherited:

```markdown
---
name: docs-writer
extends: docs-writer
model: opus # upgrade just this agent's model
effort: high
# tools omitted -> inherited from the base docs-writer
---

When documenting the public API, include a runnable example in every supported language.
```

## How resolution works

`extends: <name>` takes the same-named agent that would otherwise be resolved from the layers **below**
you — built-ins, then included modules — and appends your body to it. So:

- A **repo** agent extends a **module** or **built-in** agent.
- A **module** agent extends a **built-in** agent.
- Chains compose in order (built-in -> module -> repo).

If no lower-layer agent of that name exists, generation fails with an error — `extends` never silently
produces just your message.

## When to extend vs. redefine vs. disable

- **Extend** (`extends:`) — you want the base plus a few additions. Preferred; no duplication.
- **Redefine** — omit `extends` and write a complete agent to fully replace the base. Use only when the
  base does not fit at all.
- **Disable** — drop the whole built-in bundle with `builtins = ["...", "!ai-governance"]`, or a single
  rule with `"!git-workflow/commit-messages"`, when you don't want the base at all.
