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

`validate` rejects an unknown mode, `glob` without `paths`, `auto` without a description, and an explicit `activation: always` with `paths`. A legacy always-on activation (`trigger: always_on`, `alwaysApply: true`) with `paths` is not rejected: `validate` warns that the paths are ignored and the rule applies everywhere.

### Resolution and legacy fields

When `activation` is absent, the mode is resolved in this order (first match wins):

1. `activation`
2. Legacy Devin `trigger` (`always_on`, `glob`, `model_decision`, `manual`) and `glob`
3. Legacy Cursor `alwaysApply` (`true` is always; `false` is glob if paths are set, else auto if a description is set, else manual)
4. Derived: `glob` if `paths` are set, otherwise `always`

A description alone never makes a rule `auto`.

## Output per tool

The table shows the file each preset writes and the frontmatter it emits for each activation.

| Tool (preset)                 | File                                         | always                         | glob                                              | auto                         | manual                       |
| ----------------------------- | -------------------------------------------- | ------------------------------ | ------------------------------------------------- | ---------------------------- | ---------------------------- |
| Claude (`claude`)             | `.claude/rules/<id>.md`                      | none                           | `paths: [...]`                                    | always-on, warning           | always-on, warning           |
| Cursor (`cursor`)             | `.cursor/rules/<id>.mdc`                     | `alwaysApply: true`            | `globs: a,b` (bare), `alwaysApply: false`         | `description`, `alwaysApply: false` | `alwaysApply: false`  |
| Devin (`devin`)               | `.devin/rules/<id>.md`                       | `trigger: always_on`           | `trigger: glob`, `globs: "a,b"`                   | `trigger: model_decision`, `description` | `trigger: manual` |
| Antigravity (`antigravity`)   | `.agents/rules/<id>.md`                      | `trigger: always_on`           | `trigger: glob`, `globs: "a,b"`                   | `trigger: model_decision`, `description` | `trigger: manual` |
| Copilot (`copilot`)           | `.github/instructions/<id>.instructions.md`  | `applyTo: "**"`                | `applyTo: "a,b"`                                  | stays inline                 | stays inline                 |
| Cline (`cline`)               | `.clinerules/<id>.md`                        | none                           | `paths: [...]`                                    | always-on, warning           | always-on, warning           |
| Junie (`junie`)               | `.junie/rules/<id>.md`                       | none                           | none; an `_Applies to: ..._` line, always loaded  | always-on, warning           | always-on, warning           |

Notes:

- Cursor also writes `description` on `always` and `glob` rules when one is set. It is left off `manual` rules because Cursor treats a description without globs as agent-requested.
- Cursor, Devin, Antigravity and Copilot take a comma-joined glob string, so brace patterns are expanded: `*.{ts,tsx}` becomes `*.ts,*.tsx`.
- Cursor writes the `globs` line as the bare comma list its own rule files use (`globs: **/*.go,**/*.ts`), not as a quoted YAML string, because its parser is not a full YAML parser. Values outside a conservative character set keep the quotes. Devin and Antigravity parse real YAML, where a leading `*` is an alias, so they keep the quoted form (`globs: '**/*.go'`), as does Copilot's `applyTo`.
- No dialect can express negated globs (`!x`), so they are dropped from the frontmatter with a warning. A rule whose globs are all negated has no scope left: where the preset has a root file (Claude, Copilot, Antigravity, Junie, ...) the rule stays inline there; in presets without one (Cursor, Devin, Cline) it is written as an always-on file and logged as a downgrade. Each such rule is warned about once per run. Inside a [scope](monorepo.md#scoped-rule-files) the negation is dropped and the scope prefix becomes the positive glob: a rule with only negated globs gets `<path>/**`.
- Junie's `_Applies to: ..._` line formats each glob as a code span, so characters such as `_` and `*` are not read as emphasis.
- Context files in a rules folder are named `context-<id>` and use the same frontmatter.
- Each generated file has a generated banner after the frontmatter (see [Freshness hashes](#freshness-hashes)). Scoped (monorepo) rule placement is covered in [Monorepo](monorepo.md#scoped-rule-files).
- Copilot keeps `auto` and `manual` rules in `.github/copilot-instructions.md`, because an instructions file without `applyTo` is not applied automatically.

### Fallbacks

Where a tool cannot express `auto` or `manual`, the rule is written as always-on and `generate` logs one aggregated warning naming the rules and the downgrade. Nothing is dropped. Claude, Cline and Junie have no description or manual mechanism. Copilot has no per-file equivalent for either, so those rules stay inline in its root file.

### Freshness hashes

`Content-Hash` and `Source-Hash` lines are written inside the HTML comment banner that follows the frontmatter, never in the frontmatter, because the tools' frontmatter parsers are not documented to tolerate YAML comments. This applies to every rules-folder file, including `.mdc`. Files written by earlier versions with hashes in the frontmatter are rewritten once on the next `generate`. Hash lines follow `[header] hashes`.

### Tools without a rules folder

`gemini`, `codex`, `opencode`, `amp`, `xum`, `pi`, `baz` and `hermes` read a single root file, so rules are inlined there. The activation is kept as text under the rule heading so the scope is not lost:

```markdown
## TypeScript conventions

_Applies to: `src/**/*.{ts,tsx}`, `tests/**`_
```

`auto` rules get `_When relevant: <description>_`. `manual` rules render as always-on and log a warning. Custom provider presets get the same text for inlined rules.

## `[rules] mode`

```toml
[rules]
mode = "split"             # split (default) | inline

[rules.mode_by_preset]
claude = "inline"
copilot = "split"
```

Since 4.22.0 the default is `split`. To keep the previous behaviour, set `mode = "inline"` globally, or opt out for single presets with `mode_by_preset`.

| Mode     | Rules folder receives                  | Root file keeps                          |
| -------- | -------------------------------------- | ---------------------------------------- |
| `split`  | every rule, plus path-scoped context   | context and delegation notes, no rules   |
| `inline` | path-scoped rules and context only     | all other rules, all unscoped context    |

`mode_by_preset` overrides `mode` for one preset. Switching modes removes the stale files on the next `generate`.

What each preset writes:

| Preset                          | `split`                                                                 | `inline`                                                   |
| ------------------------------- | ----------------------------------------------------------------------- | ---------------------------------------------------------- |
| `claude`                        | every rule in `.claude/rules/*.md` (`paths` when scoped)                | path-scoped rules in `.claude/rules`; the rest in `CLAUDE.md` |
| `copilot`                       | rules in `.github/instructions/*.instructions.md` (`applyTo`)           | path-scoped rules only; `auto`, `manual` and negated-only-glob rules stay in `copilot-instructions.md` in both modes |
| `antigravity`                   | every rule in `.agents/rules`; inline if `gemini` is also enabled and `mode_by_preset` does not set it | path-scoped rules in `.agents/rules` |
| `junie`                         | every rule in `.junie/rules/*.md`                                       | no rule files; everything in the root `AGENTS.md`          |
| `cursor`, `devin`, `cline` | `.cursor/rules/*.mdc`, `.devin/rules`, `.clinerules`; `mode` has no effect | same |
| `gemini`, `codex`, `opencode`, `amp`, `xum`, `pi`, `hermes` | rules inline, with `_Applies to:_` / `_When relevant:_` lines | same |
| `baz`                           | rules inline in `AGENTS.md`; path-scoped ones in the `AGENTS.md` of their directory (`rules.baz_scoped`, see [Baz](baz.md)) | same |

Custom provider presets follow `mode` when their `outputs.rules` sets `split`.

Cursor, Devin and Cline have no rules-bearing root file, so they always write one file per rule. Unscoped context goes to the folder as well for Cursor, Devin and Cline (the always-file presets).

Root files always keep context (unscoped context in both modes) and the delegation notes.

### Context

Context stays in the root file. Only path-scoped context moves to the rules folder (`context-<id>`), where the tool loads it for matching files.

### Antigravity and Gemini

Both presets write `GEMINI.md`, and the last writer wins. When both are enabled, Antigravity keeps all rules inline so `GEMINI.md` stays complete. Set `rules.mode_by_preset.antigravity` explicitly to override; ai-rulez then warns that rules may load twice or be missing from `GEMINI.md`.

### Copilot

- Instruction files without `applyTo` are not applied automatically on GitHub.com, so `auto` and `manual` rules are never written as files; they stay in `copilot-instructions.md`.
- In `split` mode always-on rules become `applyTo: "**"` files, which Copilot applies only when it has file context. Use `inline` for Copilot if a rule must apply to chat without file context.

### Claude and Copilot together

VS Code Copilot also reads `.claude/rules`. With both presets enabled, a rule can load twice. Path-scoped rules are written to both folders in either mode; `split` makes every rule load twice. Keep one of the two presets on `inline` to limit the overlap.

### Junie and a root `AGENTS.md`

Junie looks for guidance in tiers: `.junie/AGENTS.md`, then root `AGENTS.md` together with `.junie/rules`, then the legacy `.junie/guidelines.md`. If another preset (`codex`, `opencode`, `amp`, `xum`, `pi`) writes a root `AGENTS.md`, Junie may prefer it over `.junie/guidelines.md`. Keep Junie content out of `guidelines.md` in that case, or use `split` so rules load from `.junie/rules`.

## Targets

Frontmatter `targets` restricts where a rule or context file is written. It applies to every rules-folder file and to every inlined root file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `.hermes.md`, `.junie/guidelines.md`, `.github/copilot-instructions.md`, and the `*.local.md` variants). Items without `targets` go everywhere.

```markdown
---
targets: ["claude", ".cursor/rules/"]
---
```

A target selects an output when it is one of:

| Form                        | Example                          |
| --------------------------- | -------------------------------- |
| preset name                 | `claude`, `cursor`               |
| root file (path or base name) | `CLAUDE.md`, `copilot-instructions.md` |
| exact path                  | `.claude/rules/go.md`            |
| base name                   | `go.md`                          |
| directory prefix            | `.cursor/rules/`                 |
| whole tree                  | `.cursor/rules/*`, `.cursor/**`, `.cursor/rules/**` |
| glob (`path.Match` syntax)  | `.claude/rules/*.md`             |
| everything                  | `*`                              |

Matching rules:

- A target naming a preset's root file (for example `CLAUDE.md`) also selects that preset's rule files. In `split` mode every rule goes to the rules folder, so such a rule lives in `.claude/rules/` and not in `CLAUDE.md`; in `inline` mode only path-scoped rules do.
- A whole-tree target is `<dir>/*` or `<dir>/**` with a literal directory (no wildcard in `<dir>`): it matches everything below `<dir>` at any depth, exactly like `<dir>/`. Only `*` and `**` alone match everything. Any other pattern is a `path.Match` glob against the full path or the base name, where `*` does not cross `/` and `**` is no deeper than `*`; so `.*/rules/**` matches only files directly in the folder.
- Paths compare case-insensitively; `\` and a leading `./` or `/` are accepted.
- `AGENTS.md` is shared by `codex`, `opencode`, `amp`, `xum` and `pi`, and `GEMINI.md` by `gemini` and `antigravity`. Naming any preset that writes a shared file, or the file itself, selects it for all of them, so the file stays identical whichever preset writes it. With `agents_md = true` every preset that reads the shared `AGENTS.md` is an owner, and rules-folder presets stop inlining always-on items into their own root files; see [AGENTS.md and .agents/skills](agents-md.md).
- A rule targeted only at a rules folder (for example `.junie/rules/`) is written there even in `inline` mode. Where inline mode writes no files for that folder (Junie, and providers without `inline_filter`), it is omitted.
- A rule targeted only at skill or agent files appears in no root file.
- A malformed glob (such as `[x`) never matches; `validate` and `generate` warn about it.

!!! note "Behaviour change in 4.22.0"
    `targets` used to restrict only targeted sections, commands and skills. A rule with `targets: ["CLAUDE.md"]` now stops appearing in other root files and rules folders. Review rules whose `targets` omit an output they should still reach.

## File names

A rule file is named `<id><ext>`. The id is the source name with spaces, `_` and path separators turned into `-`, every character outside `[A-Za-z0-9-]` dropped, and surrounding dashes trimmed. Case is kept (`Go-Style.md` stays `Go-Style.md`), but collisions are detected case-insensitively. A name with no ASCII letter or digit gets `rule-<8 hex>`, the first eight hex digits of the SHA-1 of the name.

When two sources map to the same file name (context files carry a `context-` prefix), the source whose path sorts first keeps the id and the later one is written as `<id>-<6 hex>`, the first six hex digits of the SHA-1 of its source path, with a warning. The result does not depend on scan order. Only if the suffixed name is taken too does `generate` fail, naming both sources. Two `[[scopes]]` whose paths produce the same file-name qualifier (compared case-insensitively) also fail; see [Monorepo](monorepo.md#scoped-rule-files).

## Local rules

Rules in `.ai-rulez/local/rules` follow the same routing as shared rules. Where a preset sends rules to its rules folder, a local rule is written as `<rulesdir>/<id>.local<ext>` (for example `.claude/rules/my-rule.local.md`) instead of the `*.local.md` root file, so the tool loads it natively. Two local rules that collide get `<id>-<hash>.local<ext>`. Local context, and rules a preset keeps inline, go to the file the tool loads for machine-local instructions: `CLAUDE.local.md`, `AGENTS.local.md` (xum and OpenCode, which lists it in `opencode.json`), `GEMINI.local.md` (Gemini CLI, which lists it in `.gemini/settings.json`), `AGENTS.override.md` (Codex, and Hermes with `agents_md`), a rules-folder file `ai-rulez.local.*` (Copilot, Junie, Antigravity), or nothing for Amp and Hermes without `agents_md`, which warn instead. Custom providers get no local output. The per-preset table is in [Local Configuration](local-overrides.md#generated-output).

## Scopes

For `[[scopes]]`, rule files are written to the root rules folder, with the scope path as a qualifier and glob prefix. `auto` and `manual` rules keep their mode and so are not limited to the scope; `generate` warns once per scope about them. File names are `<dir>/<scope-slug>/<id>` for Claude, Cursor and Copilot, `<scope-slug>--<id>` for Devin, Cline, Antigravity and Junie. See [Monorepo](monorepo.md#scoped-rule-files).

## Hand-written rule files

Generated rule files are gitignored one by one (for example `.claude/rules/x.md`), never the whole folder, so rules you write by hand in the same folder stay tracked. A hand-written file is never overwritten. Since 4.22.1, if it has the name of a generated rule, the generated rule is written as `<id>.ai-rulez<ext>` instead (`<id>.ai-rulez.instructions.md` for Copilot), `generate` warns, and the renamed file is recorded in the manifest and gitignored like any generated file. Rename or delete the hand-written file to put the rule back under its plain name; the next run removes the renamed file. If the renamed name is taken by a hand-written file too, the rule is skipped with a warning. Names matching `*.local.*` in a rules folder are reserved for [local rules](#local-rules): a hand-written file with such a name is skipped with a warning, and the local rule is not written.

## Size limits

| Tool         | Limit                      |
| ------------ | -------------------------- |
| Devin        | 12000 characters per file  |
| Antigravity  | 24576 characters per file  |

A file over the limit produces a warning. Content is never truncated; split the rule instead.
