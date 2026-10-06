# Checks

Checks are code-review guidelines: per-topic instructions a review tool applies to a pull or merge request. They are a content kind of their own, next to rules, context, skills, agents and commands.

## Source files

```text
.ai-rulez/checks/<name>.md
.ai-rulez/domains/<domain>/checks/<name>.md
```

The file name is the check's identity and may contain only letters, digits, `.`, `_` and `-` (`ai-rulez validate` rejects anything else, since the name reaches output paths and section markers).

```md
---
description: Flags common security issues
severity: high
tools: [Read, Grep]
targets: [cursor, kilo]
---

Review the diff for injection vulnerabilities, hardcoded secrets and unsafe
deserialization. Report each finding with a file and line reference.
```

| Key           | Meaning                                                                                  |
| ------------- | ---------------------------------------------------------------------------------------- |
| `description` | Short summary. Used as the text when the body is empty.                                  |
| `severity`    | `low`, `medium`, `high` or `critical`. Any other value fails validation.                 |
| `tools`       | Tool names the check may use. Only Amp has a place for them.                             |
| `targets`     | Presets the check applies to, like every other content kind. Default: every preset.      |

At render time the names are checked again (includes bring checks that never went through validation): a name outside `[A-Za-z0-9._-]` is skipped with a warning, and two names that differ only in case count as one check (their files would collide on a case-insensitive file system); the one from the higher-precedence source is kept.

Checks follow profiles like other content: root checks are always included, domain checks when the domain is in the active profile. A root check shadows a domain check of the same name.

## Managing checks

```bash
ai-rulez add check security -s "Security issues"
ai-rulez add check perf --domain backend
ai-rulez list checks [--domain backend] [--format json]
ai-rulez remove check security [--domain backend] [--force]
```

The MCP server exposes `create_check`, `read_check`, `update_check`, `delete_check` and `list_checks`. An include can import them with `include = ["checks"]`.

`create_check` and `update_check` write the frontmatter with a YAML encoder, so a description such as `Flags: injection # not a comment` is quoted correctly, and they validate `severity` and `targets` (a preset name, `*`, or a path or glob such as `src/**`; a misspelt preset is rejected instead of selecting nothing). `update_check` needs `content`, at least one field (`description`, `severity`, `tools`, `targets`), or both, and rejects an empty update. Content without frontmatter replaces the body and keeps the check's own frontmatter, comments and unknown keys; fields set on it; content with its own frontmatter replaces the file (the fields are still applied over it).

## Where they are written

| Preset       | Output                                          | Shape                                               |
| ------------ | ----------------------------------------------- | --------------------------------------------------- |
| `cursor`     | `.cursor/BUGBOT.md`                             | one block, one section per check (Bugbot)           |
| `kilo`       | `REVIEW.md`                                     | one block, one section per check                    |
| `qwen`       | `.qwen/review-rules.md`                         | one block, one section per check                    |
| `factory`    | `.factory/skills/review-guidelines/SKILL.md`    | one block in a skill with `name`/`description` frontmatter |
| `rovodev`    | `.rovodev/.review-agent.md`                     | one block, one section per check                    |
| `amp`        | `.agents/checks/<name>.md`                      | one file per check                                  |
| `augment`    | `.augment/code_review_guidelines.yaml`          | merged into the hand-written YAML                   |
| `gitlab-duo` | `.gitlab/duo/mr-review-instructions.yaml`       | merged into the hand-written YAML                   |

The aggregate files are shared with you. ai-rulez writes only a marker-delimited block:

```md
<!-- ai-rulez:checks:begin -->
<!-- ai-rulez:check:security -->

## security

Review the diff for injection vulnerabilities...
<!-- ai-rulez:checks:end -->
```

Everything outside the two marker lines is yours and is kept: a hand-written `REVIEW.md` gets the block appended after a blank line, text above and below an existing block survives every `generate`, and `clean` (or removing the last check) takes back only the block. A file ai-rulez created that holds nothing else is deleted with its block; one that holds your text is never deleted, never git-ignored, and `generate` fails with an error rather than touch a file whose markers do not pair up. Do not edit inside the block (the next `generate` rewrites it) or put the marker lines in a check's text. For `factory` the `name`/`description` frontmatter is written once, when the file is created.

Each check is a marker line `<!-- ai-rulez:check:<name> -->`, a `## <name>` heading and the check text (the description when the text is empty). They take no other metadata: severity and tools have no equivalent in those tools. The Cursor file is not written by `generate --user`: no per-user Bugbot file is documented.

Amp files carry `name`, `description`, `severity-default` (from `severity`) and `tools` in the frontmatter.

Augment gets one area per check, keyed by the check name, with the text as the rule description and `globs: ["**"]`. Augment's severity scale stops at `high`, so `critical` is written as `high` and an unset severity as `medium`. GitLab Duo gets one `instructions` group per check (`name`, `instructions`), with no file filter.

Both YAML files are documented as hand-written, so ai-rulez merges: it owns only the areas or groups it renders (matched by name), keeps everything else, including `file_paths_to_ignore` and any other key, and removes its own entries when a check disappears. An area or group you wrote under a check's name is yours: it is never replaced or removed, the check of that name is not written there, and `generate` warns (rename one of them). An entry ai-rulez wrote earlier is updated while it still equals what was written; once you edit it, it is yours too. Comments, key order and quoting of the groups and areas you wrote survive: an unchanged GitLab group keeps its source text byte for byte, and only changed or new groups are re-rendered (an area ai-rulez writes is rendered fresh).

Not generated: Augment area grouping and per-area globs, GitLab `fileFilters`, Takt quality gates, Hermes pre-verify specs, and JetBrains AI Assistant (its self-review path is a per-user IDE setting).

## Notes per tool

- Kilo reads `REVIEW.md` from the pull request base branch, only once **Use REVIEW.md** is on in the Kilo web app, and truncates it at 10,000 characters. `generate` warns when the file (your text and the block) is longer.
- Qwen Code `/review` and Bugbot also read these files from the base branch. Bugbot additionally reads nested `<dir>/.cursor/BUGBOT.md` files, which ai-rulez does not generate.
- A hosted reviewer only sees files that are committed. Check outputs are never added to the managed `.gitignore` block, even with `gitignore = true`, so commit them. An entry an earlier version added is removed on the next `generate`.
- The Amp format is taken from the rulesync documentation; Amp's own manual does not describe it, so verify it against your Amp version.
- A skill named `review-guidelines` collides with the Factory output; `generate` fails with an error until one of them is renamed.
- Machine-local checks (`.ai-rulez/local/checks/`) get a gitignored file of their own only for presets that write one file per check (Amp); the shared aggregate and YAML files never take a local check, and a warning names the presets that could not write it. A local check may not share a name with a shared one.
- `ai-rulez doctor` and `ai-rulez tokens` include the check files; the token report lists them as surface whose loading ai-rulez does not model.
