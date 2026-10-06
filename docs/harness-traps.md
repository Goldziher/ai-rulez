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
- Severity is `warning` for a hand-written file (it could be a README). A file that carries the ai-rulez banner
  and is certainly ignored by the harness is an `error`, because it means a preset is broken. An explicit
  `[lint.severity]` entry always wins.
- Silence a trap with `ignore = ["AR9C1"]`, `[lint.severity]`, or `ignore_paths`.

```toml
[lint.traps]
extra_harnesses = ["cursor", "copilot"]
```

## The traps

"Inert" means the harness certainly ignores the file, not just applies it less often.

<!-- traps:begin -->
| Code | Name | Harness | Inert | Fires when | Fix | Evidence |
| --- | --- | --- | --- | --- | --- | --- |
| AR9C1 | `cursor-rule-extension-ignored` | cursor | yes | Cursor ignores files in .cursor/rules that are not .mdc | Rename it to .mdc and add frontmatter (description, globs, alwaysApply), or move the text to AGENTS.md. | [cursor](https://cursor.com/docs/rules) `A plain .md file in .cursor/rules is ignored by the rules system` (verified 2026-10-05) |
| AR9C2 | `cursor-rule-not-applied` | cursor | no | this Cursor rule has no description, globs or alwaysApply, so it applies only when @-mentioned in chat | Set alwaysApply: true, add globs, or add a description so the agent can request it. Ignore this finding if manual-only is intended. | [cursor](https://cursor.com/docs/rules) `Included only when you @-mention the rule in chat.` (verified 2026-10-05) |
| AR9C3 | `copilot-exclude-agent-invalid` | copilot | no | excludeAgent must be "code-review" or "cloud-agent" | The current docs list "code-review" and "cloud-agent"; the older "coding-agent" spelling is still accepted. Any other value is undocumented. | [copilot](https://docs.github.com/en/copilot/how-tos/configure-custom-instructions/add-repository-instructions) `Use either "code-review" or "cloud-agent"` (verified 2026-10-05) |
| AR9C4 | `copilot-instructions-suffix` | copilot | yes | Copilot reads path-specific instructions only from files named \*.instructions.md | Rename the file to &lt;name&gt;.instructions.md, or move the text to .github/copilot-instructions.md. | [copilot](https://docs.github.com/en/copilot/how-tos/configure-custom-instructions/add-repository-instructions) `The file name must end with .instructions.md` (verified 2026-10-05) |
<!-- traps:end -->

## Code ranges

`AR9C0` to `AR9C9` belong to harness traps (`AR9C0` is unused and free for the next trap). The review design has its
own block, `AR9G`. The full allocation table is in [Strict validation](strict-validation.md#code-ranges). Codes are
stable and never reused once released.

## Not covered yet

Only traps with a verified quote and date ship. These are planned and not implemented: Kiro steering and custom
agent traps, Claude skill frontmatter spelling, instruction size limits (Codex 32 KiB AGENTS.md chain, Kilo
REVIEW.md 10,000 characters, Devin and Antigravity per-file caps; today a generate-time warning), and project files
that need approval or trust (Codex and pi trusted projects, GitLab Duo `--enable-project-hooks`, zcode, Amp and
Trae MCP approval). Each needs a vendor citation before it becomes a rule.
