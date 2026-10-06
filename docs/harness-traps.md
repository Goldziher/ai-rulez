# Harness traps

Some harnesses accept a file that looks valid and then silently ignore it: a rule written to `.cursor/rules/x.md`
is never read, and a Copilot path file named `go.md` is skipped. Nothing errors, so the author concludes "the model
ignores my instructions". `ai-rulez validate --strict` reports these as harness trap findings, codes `AR9C1` to
`AR9C9`.

Each trap carries the harness, the vendor page that says so, a verbatim quote from that page, the date it was
checked, and a fix. In `--format json` these are the optional fields `harness`, `evidence`, `verified_on` and
`hint`; the text report prints them under the finding.

```console
$ ai-rulez validate --strict
.cursor/rules/api-guidelines.md:1: warning AR9C1 cursor-rule-extension-ignored: Cursor ignores files in .cursor/rules that are not .mdc (extension ".md")
      fix: Rename it to .mdc and add frontmatter (description, globs, alwaysApply), or move the text to AGENTS.md.
      evidence: https://cursor.com/docs/rules (verified 2026-10-05)
```

## When a trap runs

- Only for a harness you use: a trap runs when its harness is in `presets`, or when you list it in
  `[lint.traps] extra_harnesses` (for hand-written files with no preset, such as a team that keeps `.cursor/rules`
  by hand).
- Over the files git tracks (or the directory tree outside git) below the lint root, at the root or in a nested
  package. It reads files on disk, so a generated file is seen as last written; run `ai-rulez generate` first.
  Traps that apply to generated files (`AR9C7` to `AR9C9`) also scan the gitignored output directories
  (`.claude/skills`, `.claude/agents`) at the lint root and keep the files that carry the ai-rulez banner, because
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
rule file over 24,000 bytes. The numbers live once in `internal/harnesslimits/limits.toml`, each with vendor URL, quote
and `verified_on`; the generator's own soft warnings read the same table. `AR964` checks the sources before generation;
`AR9C9` checks what was written. Antigravity counts bytes after expanding `@[label](path)` includes, so a file under
the limit on disk can still be cut.

`AR9C7` (`key-misspelt`) flags a frontmatter key in `.claude/skills/*/SKILL.md` or `.claude/agents/*.md` that equals a
documented key ignoring case, hyphens and underscores but is not spelled exactly (`user_invocable`, `max_turns`).
Claude Code ignores such a key without an error. `settings.json` keys are not checked: the settings page documents no
silent-ignore behaviour for them.

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
| AR9C2 | `cursor-rule-not-applied` | cursor | no | this Cursor rule has no description, globs or alwaysApply, so it applies only when @-mentioned in chat | Set alwaysApply: true, add globs, or add a description so the agent can request it. Ignore this finding if manual-only is intended. | [cursor](https://cursor.com/docs/rules) `Included only when you @-mention the rule in chat.` (verified 2026-10-05) |
| AR9C3 | `copilot-exclude-agent-invalid` | copilot | no | excludeAgent must be "code-review" or "cloud-agent" | The current docs list "code-review" and "cloud-agent"; the older "coding-agent" spelling is still accepted. Any other value is undocumented. | [copilot](https://docs.github.com/en/copilot/how-tos/configure-custom-instructions/add-repository-instructions) `Use either "code-review" or "cloud-agent"` (verified 2026-10-05) |
| AR9C4 | `copilot-instructions-suffix` | copilot | yes | Copilot reads path-specific instructions only from files named \*.instructions.md | Rename the file to &lt;name&gt;.instructions.md, or move the text to .github/copilot-instructions.md. | [copilot](https://docs.github.com/en/copilot/how-tos/configure-custom-instructions/add-repository-instructions) `The file name must end with .instructions.md` (verified 2026-10-05) |
| AR9C7 | `claude-frontmatter-key-spelling` | claude | yes | Claude Code ignores this skill frontmatter key because it is not spelled exactly as documented | Skill field names are lowercase words separated by hyphens (disable-model-invocation, user-invocable, allowed-tools), except when_to_use. | [claude](https://code.claude.com/docs/en/skills) `A field name must match the table exactly, hyphens included: Claude Code ignores a field it doesn't recognize without reporting an error.` (verified 2026-10-06) |
| AR9C7 | `claude-frontmatter-key-spelling` | claude | yes | Claude Code ignores this subagent frontmatter key because it is not spelled exactly as documented | Multi-word subagent field names are camelCase (maxTurns, disallowedTools, permissionMode, mcpServers). | [claude](https://code.claude.com/docs/en/sub-agents) `Claude Code ignores a field it doesn't recognize without reporting an error.` (verified 2026-10-06) |
| AR9C8 | `claude-listing-truncated` | claude | no | description plus when_to_use is past the Claude Code skill listing cap, so the rest is cut off | Put the key use case first and keep description and when_to_use together under the cap; move detail into the skill body. | [claude](https://code.claude.com/docs/en/skills) `the combined description and when_to_use text is truncated at 1,536 characters in the skill listing` (verified 2026-10-06) |
| AR9C9 | `harness-limit-exceeded` | codex | no | the AGENTS.md files Codex reads for this directory add up to more than project_doc_max_bytes, so the rest is not loaded | Shorten the rules, split them into skills, or raise project_doc_max_bytes in Codex and in [codex] of the ai-rulez config. | [codex](https://learn.chatgpt.com/docs/agent-configuration/agents-md) `stops adding files once the combined size reaches the limit defined by project_doc_max_bytes (32 KiB by default)` (verified 2026-10-06) |
| AR9C9 | `harness-limit-exceeded` | devin | no | this Devin rule file is past the per-file limit, so it is truncated | Split the rule into several files or move detail into a skill. | [devin](https://docs.devin.ai/desktop/cascade/memories) `Workspace rule files are limited to 12,000 characters each.` (verified 2026-10-06) |
| AR9C9 | `harness-limit-exceeded` | antigravity | no | this Antigravity rule file is past the per-file limit, so it is truncated | Split the rule into several files or shorten it. The harness counts bytes after expanding includes, so a file under the limit on disk can still be cut. | [antigravity](https://antigravity.google/docs/rules) `24 KB (24,000 bytes) per-file limit` (verified 2026-10-06) |
<!-- traps:end -->

## Code ranges

`AR9C0` to `AR9C9` belong to harness traps (`AR9C5` and `AR9C6` are free for the next traps). The review design has its
own block, `AR9G`. The full allocation table is in [Strict validation](strict-validation.md#code-ranges). Codes are
stable and never reused once released.

## Not covered yet

Only traps with a verified quote and date ship. These are planned and not implemented: Kiro steering and custom
agent traps, the Kilo REVIEW.md 10,000 character limit, project-defined trap rows, and project files
that need approval or trust (Codex and pi trusted projects, GitLab Duo `--enable-project-hooks`, zcode, Amp and
Trae MCP approval). Each needs a vendor citation before it becomes a rule.
