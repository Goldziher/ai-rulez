# llms.txt

[llms.txt](https://llmstxt.org/) is a markdown file at the root of a site or repository that tells language models
what is there and where to read it. ai-rulez supports it in two ways:

- the `llms-txt` preset renders your rules, context and skills as `llms.txt` (and, on request, `llms-full.txt`) in
  your project;
- this documentation site publishes its own [`/llms.txt`](https://goldziher.github.io/ai-rulez/llms.txt) and
  [`/llms-full.txt`](https://goldziher.github.io/ai-rulez/llms-full.txt).

## The format

The file is markdown with this shape, in this order:

1. an H1 with the project name (the only required element);
2. a blockquote with a short summary;
3. optional detail paragraphs, with no headings;
4. any number of H2 sections, each a markdown list of `[name](url)` links with an optional `: note` after the link.

An H2 section named `Optional` holds secondary links an agent may skip when it needs a shorter context.

```markdown
# Project

> One paragraph that says what the project is.

## Rules

- [Style](.ai-rulez/rules/style.md): Naming and layout

## Optional

- [Reviewer agent](.ai-rulez/agents/reviewer.md): Reviews pull requests
```

`llms-full.txt` has no fixed format. ai-rulez writes the title and summary, then every page as an H2 section holding
its full text, with the page's own headings moved two levels down.

## The `llms-txt` preset

```toml
presets = ["claude", "llms-txt"]

[llms_txt]
full = true          # also write llms-full.txt
```

`generate` writes `llms.txt` at the project root. Both files are committed documentation, written verbatim with no
generated-by banner (the format needs the H1 on the first line), and kept current by `generate --check` like every
other output.

| Key | Default | Meaning |
| --- | --- | --- |
| `dir` | project root | Directory the files are written to. Must not be inside `.git` or the configuration directory |
| `title` | `name` | The H1 |
| `summary` | `description` | The blockquote under the title |
| `full` | `false` | Also write `llms-full.txt` |
| `include` | `rules`, `context`, `skills` | Kinds to list. `agents` and `commands` are listed in the `Optional` section |

Each item is one link to its source file under `.ai-rulez/`, relative to the output directory. The note is the
`description` from the item's frontmatter, or else the first prose line of its text, cut at 200 characters. Items are
sorted by name, the project's own content first, then each domain (by name) with the domain named in the note.
Output depends only on the content, so it is byte-stable across runs. A monorepo scope run and a role run do not write
the files: they describe the whole project.

## Validation

`validate --strict` checks the generated `llms.txt` against the format and reports these codes (see
[Strict validation](strict-validation.md#llmstxt-checks)). `llms-full.txt` is not an index and is not checked.

| Code | Name | Default |
| --- | --- | --- |
| AR9P0 | `llmstxt-title-missing` | error |
| AR9P1 | `llmstxt-summary-misplaced` | warning |
| AR9P2 | `llmstxt-heading-invalid` | error |
| AR9P3 | `llmstxt-link-entry-invalid` | error |
| AR9P4 | `llmstxt-optional-misplaced` | warning |
| AR9P5 | `llmstxt-section-duplicate-or-empty` | warning |
| AR9P6 | `llmstxt-link-target-invalid` | warning |

`ai-rulez validate --explain AR9P3` describes one. They belong to the `llmstxt` analyzer.

## This site

`docs/llms.txt` indexes every page in the `zensical.toml` nav with a one-line description (the page's `description`
frontmatter, or its first sentence), grouped like the nav. Proposals and the changelog go in the `Optional` section.
`docs/llms-full.txt` is every page concatenated, except the changelog. Both are generated from `zensical.toml` and
`docs/`, checked in, and copied to the site root by the build.

```sh
task docs:llms          # regenerate after editing the docs or the nav
task docs:llms:check    # what CI runs: fails when the files are stale
```

The docs workflow runs the check before building the site, and a Go test runs it with the rest of the suite, so a
docs change without a regeneration fails.
