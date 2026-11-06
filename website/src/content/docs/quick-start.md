---
title: "Quick start"
description: "Set up ai-rulez in a repository, add a rule, and generate the files for five assistants."
---

Set up ai-rulez in a repository, add a rule, and generate the files for five assistants.

<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/quickstart.gif" alt="ai-rulez init and generate: one .ai-rulez source becomes the native files of several AI tools" width="820" />
</p>

You need the `ai-rulez` CLI ([Installation](/ai-rulez/installation/)). To try it without installing, replace `ai-rulez` with
`npx ai-rulez@latest` or `uvx ai-rulez` in every command below.

:::note[Already have instruction files?]
If the repository already has a hand-written `CLAUDE.md`, `AGENTS.md`, `.cursor/rules/` or similar, start with
`ai-rulez init --from auto` instead of `ai-rulez init acme` in step 1; see
[Starting from existing files](#starting-from-existing-files). `generate` refuses to overwrite files it did not
write.
:::

## 1. Create the configuration

Run `init` at the root of your repository:

```bash
ai-rulez init acme
```

```text
Created .ai-rulez/ for acme
...
Example content created:
  - rules/code-quality.md
  - context/architecture.md
  - skills/code-reviewer/SKILL.md
  - skills/ai-rulez/SKILL.md
```

`.ai-rulez/` now holds the configuration and some example content you can edit or delete:

```text
.ai-rulez/
├── config.toml
├── index.md
├── rules/
│   ├── code-quality.md
│   └── index.md
├── context/
│   ├── architecture.md
│   └── index.md
├── skills/
│   ├── ai-rulez/
│   │   ├── SKILL.md
│   │   └── index.md
│   ├── code-reviewer/
│   │   ├── SKILL.md
│   │   └── index.md
│   └── index.md
├── agents/
└── domains/
```

| Directory | Holds | Loaded by the assistant |
| --------- | ----- | ----------------------- |
| `rules/` | Instructions the assistant must follow | Always, or only for matching files |
| `context/` | Background: architecture, conventions, glossary | Always |
| `skills/` | `SKILL.md` playbooks with optional scripts and references | On demand, when the task matches the description |
| `agents/` | Subagent definitions | When delegated to |
| `domains/` | The same folders, per team or subsystem | When a profile selects the domain |

The `index.md` files list the content of each folder. `ai-rulez` maintains them; you do not edit them.

## 2. Choose your tools

Open `.ai-rulez/config.toml` and set `presets` to the assistants your team uses:

```toml
version = "5.0"
name = "acme"
presets = ["claude", "cursor", "codex", "copilot", "gemini"]
```

The comment above `presets` in the generated file lists all 52 built-in presets; [Supported harnesses](/ai-rulez/harnesses/)
describes what each one writes.

## 3. Add a rule

```bash
ai-rulez add rule testing --priority high --content '# Testing

- Every bug fix ships with a regression test.
- Run `make test` before you commit.'
```

```text
Rule added successfully
.ai-rulez/rules/testing.md
```

The file it writes:

```markdown
---
type: Decision
title: Testing
x-ai-rulez:
  kind: rule
  id: testing
  metadata:
    priority: high
---

# Testing

- Every bug fix ships with a regression test.
- Run `make test` before you commit.
```

You can also create files by hand. `.ai-rulez/` is an [OKF](/ai-rulez/okf/) bundle, so each file should have a `type`
(`Decision` for rules, `Concept` for context, `Playbook` for skills); `title` is optional. ai-rulez settings such as
`priority` and `paths` go under `x-ai-rulez.metadata`. See [Rules](/ai-rulez/rules/) for path-scoped rules and activation modes.

## 4. Generate

```bash
ai-rulez generate
```

```text
Warning: generate will write 1 new or changed command(s) that your tools will run (from .../acme/.ai-rulez/config.toml):
  hook       SessionStart: sh -c '...'  -> claude
Review them before you start a tool in this project. Silence with --yes or AI_RULEZ_ACK_COMMANDS=1.
INFO  Generating with configuration profile=default
INFO  Generation complete files=12
```

`generate` lists every command it adds to a tool's configuration so you can review it. The `SessionStart` hook here is
the default [remote-session hook](/ai-rulez/session-start/), which runs `ai-rulez generate` when a cloud session starts.

The files it wrote:

| File | Used by |
| ---- | ------- |
| `AGENTS.md` | All rules and context, for Codex, Cursor, Copilot and (through `.gemini/settings.json`) Gemini CLI |
| `CLAUDE.md` | Claude Code; contains `@AGENTS.md`, so it loads the same instructions |
| `.agents/skills/<name>/SKILL.md` | Skills for the tools that read `.agents/skills` |
| `.claude/skills/<name>/SKILL.md` | Skills for Claude Code |
| `.gemini/settings.json` | Tells Gemini CLI to read `AGENTS.md` |
| `.mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json`, `.codex/config.toml` | The ai-rulez MCP server, in each tool's format |
| `.claude/settings.json` | The `SessionStart` hook |

The start of `AGENTS.md`:

```markdown
<!--
🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT
Project: acme
Source: .ai-rulez/config.toml
...
-->

# acme

## Rules

### code-quality

**Priority:** high
...

### testing

**Priority:** high

- Every bug fix ships with a regression test.
- Run `make test` before you commit.

## Context

### architecture
...
```

To get one instruction file per tool
(`GEMINI.md`, `.github/copilot-instructions.md`, ...) instead of a shared `AGENTS.md`, set `agents_md = false`; see
[AGENTS.md and .agents/skills](/ai-rulez/agents-md/).

## 5. Validate and commit

```bash
ai-rulez validate          # content, link and security checks
ai-rulez generate --check  # exits 2 if a generated file differs from the source
```

```text
validation: no findings in 1 root(s)
INFO  Generated files are up to date roots=1
```

Commit the source and the generated files together:

```bash
git add .ai-rulez AGENTS.md CLAUDE.md .agents .claude .codex .cursor .gemini .vscode .mcp.json
git commit -m "Add ai-rulez configuration"
```

Generated files are committed by default, so every teammate and CI job sees the same instructions. To keep them out of
git instead, set `gitignore = true` in `config.toml`; `generate` then maintains a managed block in `.gitignore`.

## Day-to-day use

| Task | Command |
| ---- | ------- |
| Regenerate after editing `.ai-rulez/` | `ai-rulez generate` |
| Regenerate on every change | `ai-rulez generate --watch` |
| Add a skill, agent or command | `ai-rulez add skill db-migrations`, `ai-rulez add agent reviewer`, `ai-rulez add command release` |
| Add a personal, gitignored rule | `ai-rulez add rule my-notes --local` ([Local configuration](/ai-rulez/local-overrides/)) |
| Remove everything `generate` wrote | `ai-rulez clean` |
| Diagnose the setup | `ai-rulez doctor` |

## Starting from existing files

If the repository already has a hand-written `CLAUDE.md`, `AGENTS.md`, `.cursor/rules/` or similar, import it
instead of starting from the example content. In a repository without `.ai-rulez/`:

```bash
ai-rulez init --from auto
```

If you already ran `ai-rulez init`, `init --from` refuses to replace the existing `.ai-rulez/`; import into it
instead:

```bash
ai-rulez convert --write
```

Both run [convert](/ai-rulez/cli/#ai-rulez-convert), which maps each file into `.ai-rulez/` and prints what it mapped,
approximated or dropped. The next `generate` replaces the originals with generated files. Without an import,
`generate` stops at the existing file:

```text
Error: refusing to overwrite 1 existing file(s) ai-rulez cannot prove it wrote: CLAUDE.md (an existing file ai-rulez did not write). Import the file with `ai-rulez convert --write`, move or delete it, or pass --force to overwrite it
```

## Troubleshooting

| Symptom | Fix |
| ------- | --- |
| ``error AR9B1 okf-type-invalid: frontmatter has no non-empty `type` `` | A hand-written file has frontmatter without `type`. Add `type` (step 3), or create the file with `ai-rulez add`. |
| `warning AR9B1 okf-type-invalid: no frontmatter block` | The file is plain Markdown. It still loads; add frontmatter with `type`, or run `ai-rulez migrate okf` to add it. |
| `warning AR9B0 okf-index-mismatch: concept ... is not listed` | Run `ai-rulez generate`; it refreshes the `index.md` listings. |
| A rule does not appear in a tool's file | Check the rule's `targets` and `paths` ([Rules](/ai-rulez/rules/)) and that the tool is in `presets`. |
| `generate --check` fails in CI | Someone edited `.ai-rulez/` without regenerating. Run `ai-rulez generate` and commit the result. |

## Next steps

- [Configuration](/ai-rulez/configuration/): every key in `config.toml`.
- [Domains & profiles](/ai-rulez/domains/): content per team, selected with `--profile`.
- [Skill frontmatter](/ai-rulez/skills/) and [Rules](/ai-rulez/rules/): activation, scoping and per-tool output.
- [Includes](/ai-rulez/includes/) and [Installed skills](/ai-rulez/installed-skills/): share content across repositories.
- [GitHub Actions](/ai-rulez/github-actions/): check generated files and run validation in CI.
- [Examples](/ai-rulez/examples/): complete configurations for common setups.
