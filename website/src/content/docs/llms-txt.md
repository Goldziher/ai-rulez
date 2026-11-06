---
title: "llms.txt"
description: "Generate llms.txt, an index that tells language models what your repository contains and where to read it."
---

[llms.txt](https://llmstxt.org/) is a Markdown index at the root of a site or repository that tells language models
what is there and where to read it. The `llms-txt` preset renders your rules, context and skills as `llms.txt`, and
optionally as `llms-full.txt` with the full text, so an agent or a docs crawler can find them without knowing the
ai-rulez layout.

## Example

```toml
# .ai-rulez/config.toml
version = "5.0"
name = "acme"
description = "Payments platform for Acme."
presets = ["claude", "llms-txt"]

[llms_txt]
full = true
include = ["rules", "skills", "agents"]
```

`ai-rulez generate` writes `llms.txt` at the project root:

```markdown
# acme

> Payments platform for Acme.

## Rules

- [Go style](.ai-rulez/rules/go-style.md): Use gofmt.
- [Security](.ai-rulez/rules/security.md): Never log secrets.

## Skills

- [release](.ai-rulez/skills/release/SKILL.md): Cut a release.

## Optional

- [reviewer](.ai-rulez/agents/reviewer.md): Reviews PRs.
```

and, with `full = true`, `llms-full.txt`:

```markdown
# acme

> Payments platform for Acme.

## Go style

Source: .ai-rulez/rules/go-style.md

### Go style

Use gofmt.
...
```

## Configuration

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `dir` | string | project root | Directory the files are written to, relative to the project root. Not inside `.git` or the configuration directory |
| `title` | string | `name` | The H1 |
| `summary` | string | `description` | The blockquote under the title |
| `full` | bool | `false` | Also write `llms-full.txt` |
| `include` | list | `["rules", "context", "skills"]` | Kinds to list: `rules`, `context`, `skills`, `agents`, `commands`. Agents and commands go in the `Optional` section |

## What is written

- **One link per item**, to its source file, relative to the output directory. The link text is the item's title;
  the note after it is the `description` from the frontmatter, or else the first line of prose, cut at 200
  characters.
- **Order:** the project's own items first, sorted by name, then each domain by name, with `Domain <name>.` at the
  start of the note.
- **`Optional`** holds secondary links (agents and commands) that an agent may skip to save context.
- **`llms-full.txt`** has the title and summary, then each item as an H2 section with a `Source:` line and its full
  text, the item's own headings moved two levels down.
- **No generated banner:** the format requires the H1 on the first line. Both files are still tracked in the manifest
  and checked by `generate --check`.
- Output depends only on the content, so it is the same on every run. A [scope](/ai-rulez/monorepo/#scopes) run or a
  [role](/ai-rulez/roles/) run does not write the files, since they describe the whole project.

## Validation

`ai-rulez validate` checks `llms.txt` against the format and reports `AR9P0`-`AR9P6`; the table of codes is in
[Strict validation](/ai-rulez/strict-validation/#llmstxt-checks). `llms-full.txt` is not an index and is not checked.
`ai-rulez validate --explain AR9P3` describes a code.

## The format

The file is Markdown with, in this order:

1. an H1 with the project name (the only required element);
2. a blockquote with a short summary;
3. optional paragraphs, with no headings;
4. any number of H2 sections, each a list of `[name](url)` links with an optional `: note`.

An H2 section named `Optional` holds links an agent may skip when it needs a shorter context. `llms-full.txt` has no
fixed format.

## This documentation site

The ai-rulez documentation publishes its own [`/llms.txt`](https://goldziher.github.io/ai-rulez/llms.txt), which
indexes every page with a one-line description, and [`/llms-full.txt`](https://goldziher.github.io/ai-rulez/llms-full.txt)
with every page's full text. Point an agent at either to give it the ai-rulez docs.
