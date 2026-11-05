# Harness traps

Some harnesses accept a file that looks valid and then silently ignore it: a rule in `.cursor/rules/x.md` is never
read, and a Copilot instructions file named `go.md` is skipped. Nothing errors, so the instructions just never reach
the model. `ai-rulez validate` reports these files as harness trap findings, codes `AR9C0` to `AR9CA`.

```console
$ ai-rulez validate
.claude/skills/x/SKILL.md:4: warning AR9C7 claude-frontmatter-key-spelling: Claude Code ignores this skill frontmatter key because it is not spelled exactly as documented ("user_invocable" is not a key; the key is "user-invocable")
      fix: Skill field names are lowercase words separated by hyphens (disable-model-invocation, user-invocable, allowed-tools), except when_to_use.
      evidence: https://code.claude.com/docs/en/skills (verified 2026-10-06)
.cursor/rules/api-guidelines.md:1: warning AR9C1 cursor-rule-extension-ignored: Cursor ignores files in .cursor/rules that are not .mdc (extension ".md")
      fix: Rename it to .mdc and add frontmatter (description, globs, alwaysApply), or move the text to AGENTS.md.
      evidence: https://cursor.com/docs/rules (verified 2026-10-05)
```

Each finding names the harness, a fix, and the vendor page that documents the behavior with the date the page was
checked. In `--format json` these are the fields `harness`, `hint`, `evidence` and `verified_on` (other fields of the
finding omitted here):

```json
{
  "code": "AR9C7",
  "name": "claude-frontmatter-key-spelling",
  "severity": "warning",
  "file": ".claude/skills/x/SKILL.md",
  "line": 4,
  "harness": "claude",
  "evidence": "https://code.claude.com/docs/en/skills",
  "verified_on": "2026-10-06",
  "hint": "Skill field names are lowercase words separated by hyphens (disable-model-invocation, user-invocable, allowed-tools), except when_to_use."
}
```

## When a trap runs

- **Only for a harness you use.** A trap runs when its harness is in `presets`, or listed in
  `[lint.traps] extra_harnesses` (for files you maintain by hand without the preset, such as a hand-kept
  `.cursor/rules`).
- **Over the files `generate` would write and the files on disk.** A file ai-rulez owns is judged by the bytes
  `generate` would write, whether or not `generate` has run; every other file is read from disk. Hand-written files
  are the ones git tracks (or the whole tree outside git). Generated files in gitignored output directories
  (`.claude/skills`, `.claude/agents`, `.kiro/steering`, `.devin/rules`, ...) are checked too, but a gitignored
  hand-written file is not.
- **Severity.** `warning` for a hand-written file (it could be a README). A generated file that the harness certainly
  ignores is an `error`, because it means a preset is broken. A `[lint.severity]` entry always wins.

```toml
[lint.traps]
extra_harnesses = ["cursor", "copilot"]
max_table_age_days = 90   # AR9C0 when a trap or limit row was verified longer ago; 0 (default) is off
```

Silence a trap with `ignore = ["AR9C1"]`, `[lint.severity]` or `ignore_paths`.

## Size limits and key spelling

`AR9C8` and `AR9C9` check the written files against documented size limits. `AR9C8` covers the Claude Code skill
listing; `AR9C9` covers the other limits:

| Code | Limit | Harness |
| ---- | ----- | ------- |
| `AR9C8` | `description` plus `when_to_use` over 1,536 characters in a skill listing | `claude` |
| `AR9C9` | The `AGENTS.md` chain from the lint root down over 32 KiB (or `[codex] project_doc_max_bytes`; `0` turns it off) | `codex` |
| `AR9C9` | A rule file over 12,000 characters | `devin` |
| `AR9C9` | A rule file over 24,000 bytes, counted after expanding `@[label](path)` includes | `antigravity` |
| `AR9C9` | The root `REVIEW.md` over 10,000 characters (Kilo reads only the root file) | `kilo` |

[`AR964`](strict-validation.md) checks the sources against documented load limits before generation.

`AR9C7` flags a frontmatter key in `.claude/skills/*/SKILL.md` or `.claude/agents/*.md` that matches a documented key
when case, hyphens and underscores are ignored, but is not spelled exactly (`user_invocable`, `max_turns`). Claude Code
ignores such a key without an error.

## Kiro traps

- `AR9C5` flags a `.kiro/agents/*.json` custom agent with no (or empty) `resources` while `.kiro/steering/*.md`
  exists in the same package: Kiro loads steering files into a custom agent only when the agent lists them. ai-rulez
  writes Markdown agents, so this fires on hand-written JSON agents.
- `AR9C6` flags a steering file whose `inclusion` frontmatter follows a blank line or other text; Kiro requires it to
  come first.

## Project traps

Add traps of your own in `.ai-rulez/traps/*.toml`. Project traps run whatever the presets and report `AR9CA`:

```toml
# .ai-rulez/traps/team.toml
[[trap]]
name = "docs-need-title"
harness = "team"
message = "docs pages need a title"
hint = "Add title: to the frontmatter."
[trap.scope]
dir = "docs"
suffix = ".md"
[trap.predicate]
kind = "frontmatter-missing-all"
keys = ["title"]
```

```text
docs/a.md:1: warning AR9CA project-trap: docs pages need a title
      fix: Add title: to the frontmatter.
```

| Field | Required | Meaning |
| ----- | -------- | ------- |
| `name`, `harness`, `message` | yes | Identity, a label for the harness, and the finding text |
| `hint` | no | The fix shown under the finding |
| `scope.dir`, `scope.suffix` | one of them | A directory inside the project (forward slashes, `.` for the root), and a suffix; a suffix that starts with neither `.` nor `/` (`REVIEW.md`) names a whole file name |
| `predicate.kind` | yes | `ext-not-in` (needs `allowed`), `name-suffix-required`, `frontmatter-enum`, `frontmatter-missing-all`, `key-misspelt`, `size-over` (takes `limit` and `measure`: `file-chars`, `file-bytes` or `frontmatter-chars`), `frontmatter-first`, `json-key-required-if` |
| `source`, `quote`, `verified_on` | no | The evidence shown with the finding |

An invalid file or row (an unknown field or kind) is reported as `AR9CA` and skipped. At most 64 files of 256 KiB each
are read.

## Autofix

`ai-rulez validate --fix` renames a misspelt frontmatter key (`AR9C7`, and project traps of kind `key-misspelt`) in
a hand-written skill or agent file, for example `user_invocable` to `user-invocable`, as long as the documented key is
not already present. Generated files are never edited: fix their source. The other traps have no mechanical fix,
because renaming or moving a file, or choosing `resources`, is your decision.

## The traps

"Inert" means the harness certainly ignores the file, not just applies it less often.

<!-- traps:begin -->
| Code | Name | Harness | Inert | Fires when | Fix | Evidence |
| --- | --- | --- | --- | --- | --- | --- |
| AR9C1 | `cursor-rule-extension-ignored` | cursor | yes | Cursor ignores files in .cursor/rules that are not .mdc | Rename it to .mdc and add frontmatter (description, globs, alwaysApply), or move the text to AGENTS.md. | [cursor](https://cursor.com/docs/rules) `A plain .md file in .cursor/rules is ignored by the rules system` (verified 2026-10-05) |
| AR9C2 | `cursor-rule-not-applied` | cursor | no | this Cursor rule has no description, globs or alwaysApply, so it applies only when @-mentioned in chat | Set alwaysApply: true, add globs, or add a description so the agent can request it. Ignore this finding if manual-only is intended. | [cursor](https://cursor.com/docs/rules) `Included only when you @-mention the rule in chat.` (verified 2026-10-06) |
| AR9C3 | `copilot-exclude-agent-invalid` | copilot | no | excludeAgent must be "code-review" or "cloud-agent" | The current docs list "code-review" and "cloud-agent"; the older "coding-agent" spelling is still accepted. Any other value is undocumented. | [copilot](https://docs.github.com/en/copilot/how-tos/configure-custom-instructions/add-repository-instructions) `Use either "code-review" or "cloud-agent"` (verified 2026-10-05) |
| AR9C4 | `copilot-instructions-suffix` | copilot | yes | Copilot reads path-specific instructions only from files named \*.instructions.md | Rename the file to &lt;name&gt;.instructions.md, or move the text to .github/copilot-instructions.md. | [copilot](https://docs.github.com/en/copilot/how-tos/configure-custom-instructions/add-repository-instructions) `The file name must end with .instructions.md` (verified 2026-10-05) |
| AR9C5 | `kiro-agent-steering-not-loaded` | kiro | no | this Kiro custom agent has no resources, so it does not load the steering files in .kiro/steering | Add the steering files to the agent's resources, for example "resources": ["file://.kiro/steering/\*\*/\*.md"]. | [kiro](https://kiro.dev/docs/steering/) `steering files are not automatically included. You must explicitly add them to the agent's resources configuration to load steering context.` (verified 2026-10-06) |
| AR9C6 | `kiro-steering-frontmatter-not-first` | kiro | no | this Kiro steering file has its inclusion frontmatter after a blank line or other text, so Kiro does not read it | Make the opening --- the first bytes of the file. | [kiro](https://kiro.dev/docs/steering/) `The inclusion configuration must be the first content in the file - no blank lines or content before it.` (verified 2026-10-06) |
| AR9C7 | `claude-frontmatter-key-spelling` | claude | yes | Claude Code ignores this skill frontmatter key because it is not spelled exactly as documented | Skill field names are lowercase words separated by hyphens (disable-model-invocation, user-invocable, allowed-tools), except when_to_use. | [claude](https://code.claude.com/docs/en/skills) `A field name must match the table exactly, hyphens included: Claude Code ignores a field it doesn't recognize without reporting an error.` (verified 2026-10-06) |
| AR9C7 | `claude-frontmatter-key-spelling` | claude | yes | Claude Code ignores this subagent frontmatter key because it is not spelled exactly as documented | Multi-word subagent field names are camelCase (maxTurns, disallowedTools, permissionMode, mcpServers). | [claude](https://code.claude.com/docs/en/sub-agents) `Claude Code ignores a field it doesn't recognize without reporting an error.` (verified 2026-10-06) |
| AR9C8 | `claude-listing-truncated` | claude | no | description plus when_to_use is past the Claude Code skill listing cap, so the rest is cut off | Put the key use case first and keep description and when_to_use together under the cap; move detail into the skill body. | [claude](https://code.claude.com/docs/en/skills) `the combined description and when_to_use text is truncated at 1,536 characters in the skill listing` (verified 2026-10-06) |
| AR9C9 | `harness-limit-exceeded` | codex | no | the AGENTS.md files Codex reads for this directory add up to more than project_doc_max_bytes, so the rest is not loaded | Shorten the rules, split them into skills, or raise project_doc_max_bytes in Codex and in [codex] of the ai-rulez config. | [codex](https://learn.chatgpt.com/docs/agent-configuration/agents-md) `stops adding files once the combined size reaches the limit defined by project_doc_max_bytes (32 KiB by default)` (verified 2026-10-06) |
| AR9C9 | `harness-limit-exceeded` | devin | no | this Devin rule file is past the per-file limit, so it is truncated | Split the rule into several files or move detail into a skill. | [devin](https://docs.devin.ai/desktop/cascade/memories) `Workspace rule files are limited to 12,000 characters each.` (verified 2026-10-06) |
| AR9C9 | `harness-limit-exceeded` | antigravity | no | this Antigravity rule file is past the per-file limit, so it is truncated | Split the rule into several files or shorten it. The harness counts bytes after expanding includes, so a file under the limit on disk can still be cut. | [antigravity](https://antigravity.google/docs/rules) `24 KB (24,000 bytes) per-file limit` (verified 2026-10-06) |
| AR9C9 | `harness-limit-exceeded` | kilo | no | this REVIEW.md is past the Kilo Code Reviews limit, so it is truncated | Shorten REVIEW.md; Kilo reads only the root file, from the base branch. | [kilo](https://kilo.ai/docs/automate/code-reviews/overview) `If it is longer than 10,000 characters, Kilo truncates it and notes that in the review summary footer.` (verified 2026-10-06) |
<!-- traps:end -->

## Code ranges

`AR9C0` to `AR9CA` belong to harness traps; see the allocation table in
[Strict validation](strict-validation.md#code-ranges). Codes are never reused once released.
