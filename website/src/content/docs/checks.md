---
title: "Checks"
description: "A check is a code-review guideline: one topic a review tool (Cursor Bugbot, Kilo Code Reviews, GitLab Duo, Augment, Amp, ...) applies to every pull or merge request."
---

A check is a code-review guideline: one topic a review tool (Cursor Bugbot, Kilo Code Reviews, GitLab Duo, Augment,
Amp, ...) applies to every pull or merge request. You write each check once in `.ai-rulez/checks/`, and `generate`
writes it into the review file each enabled tool reads.

## Example

```bash
ai-rulez add check security --severity high --description "Flags common security issues"
```

```markdown
<!-- .ai-rulez/checks/security.md -->
---
type: Reference
title: Security
description: Flags common security issues
x-ai-rulez:
  kind: check
  id: security
  metadata:
    severity: high
---

Review the diff for injection vulnerabilities, hardcoded secrets and unsafe
deserialization. Report each finding with a file and line reference.
```

```toml
# .ai-rulez/config.toml
presets = ["cursor", "kilo", "amp", "augment"]
```

After `ai-rulez generate`, Cursor Bugbot and Kilo get a section in a shared markdown file. If `REVIEW.md` already
holds your own text, the block is appended below it:

```markdown
<!-- REVIEW.md -->
This repository has no review notes yet.

<!-- ai-rulez:checks:begin -->
<!-- ai-rulez:check:security -->

## security

Review the diff for injection vulnerabilities, hardcoded secrets and unsafe
deserialization. Report each finding with a file and line reference.
<!-- ai-rulez:checks:end -->
```

Amp gets one file per check, with its metadata in the frontmatter (`tools` too, when the check sets them):

```markdown
<!-- .agents/checks/security.md -->
---
description: Flags common security issues
name: security
severity-default: high
---

Review the diff for injection vulnerabilities, hardcoded secrets and unsafe
deserialization. Report each finding with a file and line reference.
```

Augment gets an area in its YAML file:

```yaml
# .augment/code_review_guidelines.yaml
areas:
  security:
    description: Flags common security issues
    globs:
      - '**'
    rules:
      - description: |-
          Review the diff for injection vulnerabilities, hardcoded secrets and unsafe
          deserialization. Report each finding with a file and line reference.
        id: security
        severity: high
```

## Source files

```text
.ai-rulez/checks/<name>.md
.ai-rulez/domains/<domain>/checks/<name>.md
.ai-rulez/local/checks/<name>.md        machine-local, gitignored
```

The file name is the check's identity. It may contain only letters, digits, `.`, `_` and `-`, since it becomes part
of output paths and section markers; `validate` rejects anything else. As for [rules](/ai-rulez/rules/#rule-files), `type` is
required in a tree created by `init`; create checks with `ai-rulez add check`, or run `ai-rulez generate` after
writing one by hand so the `index.md` listings include it. Two names that differ only in case count as
one check, and the higher-precedence source wins.

Root checks are always included; domain checks are included when the domain is in the active
[profile](/ai-rulez/domains/) or [role](/ai-rulez/roles/). A root check shadows a domain check of the same name.

| Key           | Type   | Default      | Meaning                                                                         |
| ------------- | ------ | ------------ | ------------------------------------------------------------------------------- |
| `description` | string | none         | Short summary. Used as the text when the body is empty.                         |
| `severity`    | string | none         | `low`, `medium`, `high` or `critical`. Any other value fails validation.        |
| `tools`       | list   | none         | Tools the check may use. Only Amp has a place for them.                         |
| `targets`     | list   | every preset | Presets, paths or globs the check is written to, as for [rules](/ai-rulez/rules/#targets). `add check` and `update_check` reject a misspelt preset name. |

## Managing checks

```bash
ai-rulez add check security --description "Security issues" --severity high
ai-rulez add check sql-injection --domain backend --targets "src/**/*.go"
ai-rulez add check perf --tools Read,Grep --severity medium
ai-rulez list checks [--domain backend] [--format json]
ai-rulez remove check security [--domain backend] [--yes]
```

The [MCP server](/ai-rulez/mcp-server/) has `create_check`, `read_check`, `update_check`, `delete_check` and `list_checks`.
`update_check` takes new `content`, fields (`description`, `severity`, `tools`, `targets`), or both. Content without
frontmatter replaces only the body and keeps the existing frontmatter.

An [include](/ai-rulez/includes/) can import checks with `include = ["checks"]`.

## Where they are written

| Preset       | Output                                       | Shape                                                       |
| ------------ | -------------------------------------------- | ----------------------------------------------------------- |
| `cursor`     | `.cursor/BUGBOT.md`                          | one block, one section per check (Bugbot)                   |
| `kilo`       | `REVIEW.md`                                  | one block, one section per check                            |
| `qwen`       | `.qwen/review-rules.md`                      | one block, one section per check                            |
| `rovodev`    | `.rovodev/.review-agent.md`                  | one block, one section per check                            |
| `factory`    | `.factory/skills/review-guidelines/SKILL.md` | one block in a skill with `name` and `description` frontmatter |
| `amp`        | `.agents/checks/<name>.md`                   | one file per check                                          |
| `augment`    | `.augment/code_review_guidelines.yaml`       | one area per check, merged into your YAML                   |
| `gitlab-duo` | `.gitlab/duo/mr-review-instructions.yaml`    | one `instructions` group per check, merged into your YAML   |

### Shared markdown files

ai-rulez owns only the block between `<!-- ai-rulez:checks:begin -->` and `<!-- ai-rulez:checks:end -->`. Each check
in it is a `<!-- ai-rulez:check:<name> -->` marker, a `## <name>` heading and the check text:

```md
<!-- ai-rulez:checks:begin -->
<!-- ai-rulez:check:security -->

## security

Review the diff for injection vulnerabilities...
<!-- ai-rulez:checks:end -->
```

- Text outside the block is yours and survives every `generate`. An existing file gets the block appended.
- `clean`, or removing the last check, removes only the block. A file that held nothing else is deleted; a file with
  your text is never deleted or gitignored.
- Do not edit inside the block (the next `generate` rewrites it) and do not put the marker lines in a check's text.
  `generate` fails rather than touch a file whose markers do not pair up.
- Severity and tools have no equivalent in these files and are not written.
- For `factory`, the skill frontmatter is written once, when the file is created. A skill of your own named
  `review-guidelines` collides with it, and `generate` fails until one is renamed.

### Augment and GitLab Duo YAML

Both files are documented as hand-written, so ai-rulez merges into them. It owns only the areas (Augment) or groups
(GitLab Duo) named after its checks, keeps everything else (`file_paths_to_ignore`, your own entries, comments and key
order), and removes its entries when a check is deleted.

- An entry you wrote under a check's name is yours: it is never replaced, the check is not written there, and
  `generate` warns. Rename one of them.
- An entry ai-rulez wrote becomes yours once you edit it.
- Augment's severity scale stops at `high`: `critical` is written as `high`, and an unset severity as `medium`. Every
  area gets `globs: ["**"]`.
- GitLab Duo groups have `name` and `instructions` only, with no file filter.

## Tool notes

- **Commit the outputs.** A hosted reviewer reads only committed files, usually from the base branch of the pull
  request. Check outputs are never added to the managed `.gitignore` block, even with `gitignore = true`.
- **Kilo** reads `REVIEW.md` only when **Use REVIEW.md** is enabled in the Kilo web app, and truncates it at 10,000
  characters. `generate` warns when the file, your text included, is longer.
- **Bugbot** also reads nested `<dir>/.cursor/BUGBOT.md` files, which ai-rulez does not generate. `generate --user`
  writes no Bugbot file.
- **Amp**: the format follows the rulesync documentation, since Amp's own manual does not describe it. Verify it
  against your Amp version.
- **Local checks** (`.ai-rulez/local/checks/`) are written only for presets with one file per check (Amp), as a
  gitignored file. Shared files never take a local check, and a warning names the presets that skipped it. A local
  check may not share a name with a shared one.
- `ai-rulez doctor` and `ai-rulez tokens` include the check files.

Not generated: Augment per-area globs, GitLab `fileFilters`, Takt quality gates, Hermes pre-verify specs, and
JetBrains AI Assistant (its self-review path is a per-user IDE setting).
