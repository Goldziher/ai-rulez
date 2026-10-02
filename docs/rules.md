# Rules and native rules folders

Most AI tools can load rules from a folder, one file per rule, each with its own activation: always on, only when matching files are in play, only when the model finds the description relevant, or only when invoked by hand. ai-rulez writes your `.ai-rulez/rules/` into those folders.

Why it matters:

- **Path-scoped rules load only when relevant.** A rule for `**/*.tsx` costs nothing while you work on Go.
- **Rules and context are separated.** The root file (`CLAUDE.md`, `.github/copilot-instructions.md`, ...) keeps context and the delegation notes; rules live in the tool's rules folder.

## Source frontmatter

Activation is declared in the rule's frontmatter:

```markdown
---
priority: high
paths:
  - "src/**/*.{ts,tsx}"
  - "tests/**"
activation: glob
---

# TypeScript conventions
```

| Field         | Type             | Meaning                                                                       |
| ------------- | ---------------- | ----------------------------------------------------------------------------- |
| `paths`       | list or string   | Files the rule applies to. `globs` is a synonym.                              |
| `activation`  | string           | `always`, `glob`, `auto` or `manual`. Optional; derived when absent.          |
| `description` | string           | What the rule is for. Required for `auto`; used by the model to decide.       |

`paths` and `globs` accept a YAML list or a comma-separated string (`"a/**, b/**"`). Commas inside braces (`*.{ts,tsx}`) or brackets do not split, and a backslash-escaped comma is kept. Entries are trimmed and deduplicated.

| Activation | Applies                                          | Requires      |
| ---------- | ------------------------------------------------ | ------------- |
| `always`   | in every interaction                             | no `paths`    |
| `glob`     | when files matching `paths` are in play          | `paths`       |
| `auto`     | when the model judges the description relevant   | `description` |
| `manual`   | only when explicitly invoked                     | nothing       |

`validate` rejects an unknown mode, `glob` without `paths`, `auto` without a description, and `always` with `paths`.

### Resolution and legacy fields

When `activation` is absent, the mode is resolved in this order (first match wins):

1. `activation`
2. Legacy Windsurf `trigger` (`always_on`, `glob`, `model_decision`, `manual`) and `glob`
3. Legacy Cursor `alwaysApply` (`true` is always; `false` is glob if paths are set, else auto if a description is set, else manual)
4. Derived: `glob` if `paths` are set, otherwise `always`

A description alone never makes a rule `auto`.

## Output per tool

The table shows the file each preset writes and the frontmatter it emits for each activation.

| Tool (preset)                 | File                                         | always                         | glob                                              | auto                         | manual                       |
| ----------------------------- | -------------------------------------------- | ------------------------------ | ------------------------------------------------- | ---------------------------- | ---------------------------- |
| Claude (`claude`)             | `.claude/rules/<id>.md`                      | none                           | `paths: [...]`                                    | always-on, warning           | always-on, warning           |
| Cursor (`cursor`)             | `.cursor/rules/<id>.mdc`                     | `alwaysApply: true`            | `globs: "a,b"`, `alwaysApply: false`              | `description`                | `alwaysApply: false`         |
| Windsurf (`windsurf`)         | `.windsurf/rules/<id>.md`                    | `trigger: always_on`           | `trigger: glob`, `globs: "a,b"`                   | `trigger: model_decision`, `description` | `trigger: manual` |
| Antigravity (`antigravity`)   | `.agents/rules/<id>.md`                      | `trigger: always_on`           | `trigger: glob`, `globs: "a,b"`                   | `trigger: model_decision`, `description` | `trigger: manual` |
| Copilot (`copilot`)           | `.github/instructions/<id>.instructions.md`  | `applyTo: "**"`                | `applyTo: "a,b"`                                  | `description` only           | no frontmatter               |
| Cline (`cline`)               | `.clinerules/<id>.md`                        | none                           | `paths: [...]`                                    | always-on, warning           | always-on, warning           |
| Continue (`continue-dev`)     | `.continue/rules/<id>.md`                    | `name`, `alwaysApply: true`    | `name`, `globs: [...]`, `alwaysApply: false`      | `name`, `description`        | `name`, `alwaysApply: false` |
| Junie (`junie`)               | `.junie/rules/<id>.md`                       | none                           | none; an `_Applies to: ..._` line, always loaded  | always-on, warning           | always-on, warning           |

Notes:

- Cursor also writes `description` on `always` and `glob` rules when one is set. It is left off `manual` rules because Cursor treats a description without globs as agent-requested.
- Cursor, Windsurf, Antigravity and Copilot take a comma-joined glob string, so brace patterns are expanded: `*.{ts,tsx}` becomes `*.ts,*.tsx`.
- Context files in a rules folder are named `context-<id>` and use the same frontmatter.
- Each generated file starts with a generated banner (except `.mdc`, whose frontmatter carries the hashes). Scoped (monorepo) rule placement is not covered here; see [Monorepo](monorepo.md).

### Fallbacks

Where a tool cannot express `auto` or `manual`, the rule is written as always-on and `generate` logs one aggregated warning naming the rules and the downgrade. Nothing is dropped. Claude, Cline and Junie have no description or manual mechanism. Copilot cannot express `manual` and expresses `auto` only through a `description`.

### Tools without a rules folder

`gemini`, `codex`, `opencode`, `amp`, `xum` and `hermes` read a single root file, so rules are inlined there. The activation is kept as text under the rule heading so the scope is not lost:

```markdown
## TypeScript conventions

_Applies to: `src/**/*.{ts,tsx}`, `tests/**`_
```

`auto` rules get `_When relevant: <description>_`. `manual` rules render as always-on and log a warning. Custom provider presets get the same text for inlined rules.

## `[rules] mode`

```toml
[rules]
mode = "inline"            # split | inline

[rules.mode_by_preset]
claude = "split"
copilot = "inline"
```

<!-- TODO(4.22.0): default flips to split -->

Default: `inline`.

| Mode     | Rules folder receives                  | Root file keeps                          |
| -------- | -------------------------------------- | ---------------------------------------- |
| `inline` | path-scoped rules and context only     | all other rules, all unscoped context    |
| `split`  | every rule, plus path-scoped context   | context and delegation notes, no rules   |

`mode_by_preset` overrides `mode` for one preset. Switching modes removes the stale files on the next `generate`.

Applies to `claude`, `copilot`, `antigravity` and `junie`. Junie has no glob activation, so in `inline` mode it writes no rule files; `split` writes `.junie/rules/`.

Cursor, Windsurf, Cline and Continue have no rules-bearing root file, so they always write one file per rule and `mode` has no effect on them. Context goes to the folder as well, except for Continue, which keeps it in its prompts file.

### Context

Context stays in the root file. Only path-scoped context moves to the rules folder (`context-<id>`), where the tool loads it for matching files.

### Antigravity and Gemini

Both presets write `GEMINI.md`, and the last writer wins. When both are enabled, Antigravity keeps all rules inline so `GEMINI.md` stays complete. Set `rules.mode_by_preset.antigravity` explicitly to override; ai-rulez then warns that rules may load twice or be missing from `GEMINI.md`.

### Copilot

- Instruction files without `applyTo` (`manual` rules, and `auto` rules that carry only a `description`) are not applied automatically on GitHub.com.
- In `split` mode always-on rules become `applyTo: "**"` files, which Copilot applies only when it has file context. Use `inline` for Copilot if a rule must apply to chat without file context.

### Claude and Copilot together

VS Code Copilot also reads `.claude/rules`. With both presets enabled, a rule can load twice. Path-scoped rules are written to both folders in either mode; `split` makes every rule load twice. Keep one of the two presets on `inline` to limit the overlap.

## Hand-written rule files

Generated rule files are gitignored one by one (for example `.claude/rules/x.md`), never the whole folder, so rules you write by hand in the same folder stay tracked. If a hand-written file collides with a generated rule name, `generate` warns and skips it instead of overwriting it; rename one of the two.

## Size limits

| Tool         | Limit                      |
| ------------ | -------------------------- |
| Windsurf     | 12000 characters per file  |
| Antigravity  | 24576 characters per file  |

A file over the limit produces a warning. Content is never truncated; split the rule instead.
