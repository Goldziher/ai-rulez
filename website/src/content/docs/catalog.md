---
title: "Catalog"
description: "ai-rulez catalog lists everything a project defines (rules, context, skills, agents, commands and checks) with owner, version, token cost, lock digest, the roles that keep each item and its lint status."
---

`ai-rulez catalog` lists everything a project defines (rules, context, skills, agents, commands and checks) with
owner, version, token cost, lock digest, the roles that keep each item and its lint status. It prints a table, JSON
for tools, or a static website you can publish for reviewers. `ai-rulez catalog diff` compares two catalogs, for
example in a pull request.

## Example

```text
$ ai-rulez catalog
KIND     DOMAIN  ID             OWNER  VERSION  TOKENS  ROLES
context  -       architecture   -      -        122     0
rule     -       code-quality   -      -        88      0
skill    -       ai-rulez       -      -        540     0
skill    -       code-reviewer  -      -        148     0

4 item(s), 0 role(s); lock: none
```

```text
$ ai-rulez catalog --html site/
wrote 14 files to site/ (4 items, 0 roles, 0 MCP servers)   catalog digest sha256:e5ed17e3...
lint: 0 errors, 0 warnings   lock: none
```

`OWNER` and `VERSION` come from the items' frontmatter; `ROLES` counts the [roles](/ai-rulez/roles/) that keep the item.

## JSON

```bash
ai-rulez catalog --format json                      # schema version 1 (default)
ai-rulez catalog --format json --schema-version 2
```

Version 1 ([`schema/catalog.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/catalog.v1.schema.json))
is the default and what the MCP `catalog` tool returns. Version 2
([`schema/catalog.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/catalog.schema.json)) contains
every version 1 field plus:

| Field | Meaning |
| --- | --- |
| `generated_by`, `project` | Tool name and version; project name, description and lock tree digest |
| `items[].ref` | Stable key `kind/domain/id` (domain `-` for root), such as `skill/-/code-reviewer`; a repeated key gets `#2`, `#3` |
| `items[].description` | From the frontmatter, verbatim (escape it when you render it) |
| `items[].source` | `{"type": "local"}` or `{"type": "include"}` |
| `items[].load_cost` | `listing_tokens` (what a harness always lists, such as a skill's name and description), `body_tokens` (loaded on use), `resource_tokens` and `resources` (bundled files) |
| `items[].lint` | `status` (`ok`, `warn`, `error`), counts and findings, from the same engine as `validate`; absent when lint could not run |
| `items[].excerpt` | First 2 KiB of the body as plain text; off with `--include-excerpt=false` |
| `items[].approval` | `null`, or `{required, status, reviewers, assurance, expires}`; see [Approvals](/ai-rulez/approvals/) |
| `items[].eval`, `items[].usage` | With `--with-eval` / `--with-usage`; see [Eval and usage](#eval-and-usage) |
| `mcp_servers` | Each server's `ref`, `name`, `transport`, `command_basename`, `enabled`, `profiles`, `pinned`, and the names of its `env` and `headers` entries, marked `literal` or with the variable they reference; `warnings` flags an unpinned launch or a literal credential |
| `edges` | `{from, to, kind: "uses"}`: item `from` names skill `to` in its `skills:` frontmatter |
| `lint` | Project totals, counts per code, and findings no item owns |
| `notes` | Why a section is missing or narrowed |

A catalog never contains MCP arguments, URLs, env values or header values, which can hold secrets and internal
hostnames. Paths are relative to the configuration directory. A consumer should refuse a `schema_version` it does not
know.

```json
{
  "ref": "skill/-/code-reviewer",
  "kind": "skill",
  "id": "code-reviewer",
  "path": "skills/code-reviewer/SKILL.md",
  "description": "Specialized agent for code review and quality assurance",
  "delivery": "static",
  "source": {"type": "local"},
  "bytes": 685,
  "tokens": 148,
  "digest": "sha256:67cd967c...",
  "load_cost": {"listing_tokens": 13, "body_tokens": 148, "resource_tokens": 0, "resources": 0},
  "lint": {"status": "ok", "counts": {"error": 0, "warning": 0, "info": 0}, "findings": []},
  "approval": null,
  "roles": []
}
```

## Eval and usage

```bash
ai-rulez catalog --format json --schema-version 2 --with-eval --with-usage
ai-rulez catalog --html site/ --with-eval=results/eval-results.json --with-usage=logs/usage.jsonl
```

Both are off by default. Without a value they read `eval-results.json` in the configuration directory (written by
[evals](/ai-rulez/evals/)) and `local/usage.jsonl` (written by the [usage hooks](/ai-rulez/usage-telemetry/)). A missing file is
noted in `notes` and the fields are omitted; a file that cannot be parsed is an error. To name a file literally
called `default`, write `./default`.

Only aggregates are copied:

- `eval`: `cases`, `pass_rate`, `passing`, `ablation_delta`, `trigger_precision`, `trigger_recall`, `stale` (the
  skill changed since the run), `verified` (signed by this machine's key) and `date`.
- `usage`: `invocations` and `last_seen` (a day). Sessions, harnesses, outcomes and feedback never enter the catalog.

Two skills that share an id cannot be told apart in the usage log, so their usage is left out (and said in `notes`).
The site gains Eval and Uses columns and per-item Eval and Usage tables.

## Comparing catalogs

```bash
ai-rulez catalog diff main                         # a revision against the working tree
ai-rulez catalog diff v5.10.0 HEAD --format json
ai-rulez catalog diff before.json after.json --exit-code
```

```text
$ ai-rulez catalog diff HEAD
catalog diff: HEAD (404870d64340) -> working tree
totals: 4 -> 5 items   listing tokens 37 -> 37   body tokens 898 -> 901
items: 1 added, 0 removed, 0 changed
  + rule/-/naming   (3 tokens)
lint: 0 errors, 0 warnings, 0 infos -> 0 errors, 2 warnings, 1 infos
```

The diff covers items (matched by `ref`; a change lists the fields that differ, such as digest, description, owner,
tokens, lint or approval status, roles), MCP servers, roles, dependency edges and lint totals. JSON output follows
[`schema/catalog-diff.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/catalog-diff.schema.json).

Each argument is a catalog JSON file (version 2, or a site's `catalog.json`) or a git revision. With one argument the
other side is the working tree. A revision is read from git without touching your checkout, and both sides are built
from the shared configuration only:

- the machine-local overlay is left out;
- remote includes and installed skills are not resolved (no network), so compare two `catalog.json` files to cover
  them;
- untracked and ignored files are not part of a revision.

| Exit code | Meaning |
| --- | --- |
| `0` | Done (also when the catalogs differ, unless `--exit-code`) |
| `1` | The command could not run |
| `2` | With `--exit-code`: the catalogs differ |

## Static website

```bash
ai-rulez catalog --html site/
ai-rulez catalog --html site/ --role backend --clean
```

The site has an overview (filter by name, domain, owner, kind and lint status), a page per item and per role, the
MCP servers, a dependency graph, the lock status, the lint findings, an About page, `catalog.json` (the version 2
document) and `robots.txt`.

- **Self-contained.** Links are relative, so it works from `file://` or any URL path. Nothing is fetched (no fonts,
  CDN or analytics), and every page works without JavaScript; the script only adds filtering, paging and copy
  buttons.
- **Reproducible.** The same input gives the same bytes. The footer shows the tool version and the SHA-256 of
  `catalog.json` instead of a date, so you can regenerate and compare to detect a tampered hosted copy.
- **Safe to render.** Every string is HTML-escaped; links are built from slugs, never from source text. Invisible
  and direction-changing characters are replaced with U+FFFD and the item is marked "contains hidden characters".
  Bodies are shown as plain text unless you enable [Markdown excerpts](#markdown-excerpts).
- **Strict Content-Security-Policy.** Each page carries `default-src 'none'; img-src 'self' data:; style-src 'self';
  script-src 'self'; base-uri 'none'; form-action 'none'`. Send the same as a response header from your host, plus
  `frame-ancestors 'none'`.
- **Protected output directory.** The directory must be new, empty, or hold the `.ai-rulez-catalog` marker an
  earlier run wrote. A directory with `.git` or `.ai-rulez` is always refused. `--clean` removes only files the
  marker lists that still hold the recorded bytes.
- **No secrets.** The run is refused when an item the site would publish has a secret finding (`AR001`) in its
  excerpt or description. Remove the secret, or pass `--allow-findings AR001`.

The dependency graph (`graph.html`) draws the items that name skills in their `skills:` frontmatter, left to right,
as a static SVG with the same edges listed in a table. A name resolves to the skill in the same domain, else the root
skill, else the only skill with that name; anything ambiguous or unresolved is left out and listed in `notes`. Above
150 items only the table is shown.

A published catalog exposes names, descriptions, owners, token costs and lint findings: publish it only where the
configuration itself may be read.

### Freshness check

```bash
ai-rulez catalog --html site/ --check
```

Renders the site in memory and compares it with the directory, writing nothing:

```text
site/ differs from the catalog site that would be generated:
  unexpected  items/extra.html
run `ai-rulez catalog --html site/` to regenerate it
Error: catalog site is out of date
```

Exit `0` when the directory matches, `2` when a file is changed, missing or unexpected (or the directory does not
exist), `1` when the check could not run. Use it in CI to keep a committed site current. `--check` and `--clean`
cannot be combined.

### Markdown excerpts

`--render-markdown` (or `render_markdown = true`) shows excerpts as formatted Markdown: paragraphs, headings (shifted
to `h3`-`h6`), lists, quotes, code, emphasis and rules. It stays safe:

- raw HTML is shown as text;
- links and images are not followed: the label or alt text is kept and the destination is shown as text;
- nesting and size are capped.

The excerpt is still the first 2 KiB of the body, so a long document may be cut mid-structure.

## Configuration

```toml
[catalog]
title = "Acme catalog"       # site title (--base-title)
include_excerpt = true       # body excerpts (--include-excerpt)
exclude_owners = false       # leave owner names out (--no-owners)
indexable = false            # let search engines index the site (--indexable)
max_items_per_page = 200     # overview rows per page (--max-items-per-page)
render_markdown = false      # Markdown excerpts (--render-markdown)
```

| Key | Default | Meaning |
| --- | --- | --- |
| `title` | `AI-Rulez catalog` | Site title |
| `include_excerpt` | `true`, `false` when `indexable` | Include body excerpts (site and version 2 JSON) |
| `exclude_owners` | `false` | Leave owner names out of the site and version 2 JSON |
| `indexable` | `false` | Drop `robots.txt` and `noindex` |
| `max_items_per_page` | `200` (`0` means 200) | Rows per overview page; all rows stay in `index.html`, so find-in-page and the no-JavaScript view show everything |
| `render_markdown` | `false` | Render excerpts as Markdown |

Every key is optional; a flag overrides it. The table can also be set in `config.local.toml`.

## Flags

| Flag | Meaning |
| --- | --- |
| `--format text\|json`, `--schema-version 1\|2` | Output format |
| `--html <dir>` | Write the site |
| `--role <name>` | With `--html`: only the items this role keeps |
| `--check`, `--clean` | With `--html`: compare without writing; remove files the site no longer has |
| `--base-title`, `--include-excerpt`, `--no-owners`, `--indexable`, `--max-items-per-page`, `--render-markdown` | See [Configuration](#configuration) |
| `--allow-findings <codes>` | With `--html`: publish despite these findings |
| `--with-eval[=FILE]`, `--with-usage[=FILE]` | Add eval and usage aggregates |
| `--no-local` | Ignore the machine-local overlay |

See the [CLI reference](/ai-rulez/cli-reference/#ai-rulez-catalog).
