# Harness traps

Some harnesses accept a file that looks valid and then silently ignore it: a rule written to `.cursor/rules/x.md`
is never read, and a Copilot path file named `go.md` is skipped. Nothing errors, so the author concludes "the model
ignores my instructions". `ai-rulez validate` reports these as harness trap findings, codes `AR9C1` to
`AR9CA`.

Each trap carries the harness, the vendor page that says so, a verbatim quote from that page, the date it was
checked, and a fix. In `--format json` these are the optional fields `harness`, `evidence`, `verified_on` and
`hint`; the text report prints them under the finding.

```console
$ ai-rulez validate
.cursor/rules/api-guidelines.md:1: warning AR9C1 cursor-rule-extension-ignored: Cursor ignores files in .cursor/rules that are not .mdc (extension ".md")
      fix: Rename it to .mdc and add frontmatter (description, globs, alwaysApply), or move the text to AGENTS.md.
      evidence: https://cursor.com/docs/rules (verified 2026-10-05)
```

## When a trap runs

- Only for a harness you use: a trap runs when its harness is in `presets`, or when you list it in
  `[lint.traps] extra_harnesses` (for hand-written files with no preset, such as a team that keeps `.cursor/rules`
  by hand).
- Over the files git tracks (or the directory tree outside git) below the lint root, at the root or in a nested
  package. A file `generate` would write is read from the plan, so a generated file is judged as the harness will see it
  whether or not `generate` has run; a hand-written file is read from disk.
  Traps that apply to generated files (`AR9C1`, `AR9C3` to `AR9C9`) also scan the gitignored output directories of
  their scope (`.claude/skills`, `.claude/agents`, `.kiro/agents`, `.kiro/steering`, `.devin/rules`, ...) at the lint root and keep the files that carry the ai-rulez banner, because
  Claude Code outputs are gitignored and git does not list them. A gitignored hand-written file is not checked.
- Severity is `warning` for a hand-written file (it could be a README). A file that carries the ai-rulez banner
  and is certainly ignored by the harness is an `error`, because it means a preset is broken. An explicit
  `[lint.severity]` entry always wins.
- Silence a trap with `ignore = ["AR9C1"]`, `[lint.severity]`, or `ignore_paths`.

```toml
[lint.traps]
extra_harnesses = ["cursor", "copilot"]
max_table_age_days = 90   # AR9C0 when a row's verified_on is older; 0 (default) is off
```

## Size limits and key spelling

`AR9C8` and `AR9C9` use the `size-over` predicate over the generated files on disk: a Claude skill whose `description`
plus `when_to_use` is over 1,536 characters, the Codex `AGENTS.md` chain from the lint root down (32 KiB, or
`[codex] project_doc_max_bytes` when set; 0 turns it off), a Devin rule file over 12,000 characters, an Antigravity
rule file over 24,000 bytes, the root Kilo `REVIEW.md` over 10,000 characters (Kilo reads only the root file). The numbers live once in `internal/harnesslimits/limits.toml`, each with vendor URL, quote
and `verified_on`; the generator's own soft warnings read the same table. `AR964` checks the sources before generation;
`AR9C9` checks what was written. Antigravity counts bytes after expanding `@[label](path)` includes, so a file under
the limit on disk can still be cut.

`AR9C7` (`key-misspelt`) flags a frontmatter key in `.claude/skills/*/SKILL.md` or `.claude/agents/*.md` that equals a
documented key ignoring case, hyphens and underscores but is not spelled exactly (`user_invocable`, `max_turns`).
Claude Code ignores such a key without an error. `settings.json` keys are not checked: the settings page documents no
silent-ignore behaviour for them.

## Kiro traps

`AR9C5` (`json-key-required-if`) flags a `.kiro/agents/*.json` custom agent that has no `resources` (or an empty one)
while `.kiro/steering/*.md` exists in the same package: Kiro does not load steering files into a custom agent unless
the agent lists them. ai-rulez writes Markdown agents, so this fires on hand-written JSON agents. `AR9C6`
(`frontmatter-first`) flags a steering file whose `inclusion` frontmatter follows a blank line or other text; Kiro
documents that it must come first. Neither is marked inert: the vendor page states the rule, not what Kiro does with
a file that breaks it.

Traps judge the final bytes `generate` would write (including its headers) for files ai-rulez owns, and the files on
disk for everything else. A file that exists, is not generated and that a run would leave alone keeps its on-disk
content. The generator implements `lint.PlannedFiles`, which breaks the import cycle between the two packages.

## Project traps

A project adds rows of its own in `.ai-rulez/traps/*.toml`, using the layout of the built-in table. Project rows run
for every project that has them (no preset gate), report `AR9CA`, and may use any predicate of the closed
vocabulary (`ext-not-in`, `name-suffix-required`, `frontmatter-enum`, `frontmatter-missing-all`, `key-misspelt`,
`size-over`, `frontmatter-first`, `json-key-required-if`). `source`, `quote` and `verified_on` are optional. A row
needs `name`, `harness` (a label), `message` and a scope with `dir` or `suffix`; `scope.dir` must stay inside the
project, use forward slashes (`.` is the project root), and a `suffix` that starts with neither `.` nor `/`
(`REVIEW.md`) names a whole file name. `ext-not-in` needs `allowed`. `size-over` takes its own `limit` and `measure` (`file-chars`, `file-bytes` or `frontmatter-chars`). An
invalid row or file (unknown field, unknown kind) is reported as `AR9CA` and skipped. At most 64 files of 256 KiB.

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

## Autofix

`ai-rulez validate --strict --fix` applies the safe trap fixes. Today that is `AR9C7` and project `AR9CA` rows of kind `key-misspelt`: it renames a misspelt
frontmatter key (`user_invocable` to `user-invocable`) in a hand-written skill or agent file, which may live outside
`.ai-rulez/`, and only while the documented key is absent (a file that has both spellings is reported, not edited). A generated output is never edited (fix its source), and the other traps have no mechanical fix:
renaming or moving a file, or choosing `resources`, is a decision.

## Keeping the table fresh

`task harness:verify` lists every trap and limit row with its source URL and `verified_on`, and fails when a row is
older than `[lint.traps] max_table_age_days` from the repository config (90 when unset; `HARNESS_MAX_AGE_DAYS`
overrides). It does not use the network. `HARNESS_VERIFY_FETCH=1 task harness:verify` also fetches each source and
fails when a quote is no longer on its page.

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

`AR9C0` to `AR9CA` belong to harness traps. The review design has its
own block, `AR9G`. The full allocation table is in [Strict validation](strict-validation.md#code-ranges). Codes are
stable and never reused once released.

## Not covered yet

Only traps with a verified quote and date ship. Searched on 2026-10-06 and left out:

- Codex skills listing: the docs (learn.chatgpt.com/docs/build-skills) say the list "uses at most 2% of the model's
  context window, or 8,000 characters when the context window is unknown" and that descriptions are shortened first.
  The budget depends on the model, so there is no fixed number to check a file against.
- `settings.json` key spelling: the Claude Code settings page documents no silent-ignore behaviour for unknown keys;
  the evidence found is GitHub issues, which is not a vendor citation.
- Project files that need approval or trust (Codex and pi trusted projects, GitLab Duo `--enable-project-hooks`,
  zcode, Amp and Trae MCP approval): no vendor citation gathered yet.

Each needs a vendor citation before it becomes a rule.
