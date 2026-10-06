# Catalog

`ai-rulez catalog` describes everything a repository defines: rules, context, skills, agents, commands and
checks, with owner, version, size, the digest `ai-rulez.lock` pins, the roles that keep each item and the lock
status. It prints a table, JSON for tools, or a static website.

## JSON

```bash
ai-rulez catalog --format json                      # version 1 (default)
ai-rulez catalog --format json --schema-version 2   # version 2
```

Version 1 ([`schema/catalog.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/catalog.v1.schema.json))
stays the default for one minor release so existing consumers keep working. Version 2
([`schema/catalog.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/catalog.schema.json)) is a
strict superset of the version 1 fields, plus:

| Field | Meaning |
| ----- | ------- |
| `generated_by`, `project` | tool name and version; project name, description and lock tree digest |
| `items[].ref` | stable key `kind/domain/id` (domain `-` when none); a repeated key gets `#2`, `#3` |
| `items[].description`, `source` | description from the frontmatter (verbatim; a consumer escapes it); `local` or `include` |
| `items[].load_cost` | `listing_tokens` (what the harness always lists: skill name and description), `body_tokens` (loaded on use), `resource_tokens` and `resources` (bundled files) |
| `items[].lint` | `status` (`ok`, `warn`, `error`), counts and findings, from the same engine as `validate --strict`; absent when lint could not run |
| `items[].excerpt` | first 2 KiB of the body, plain text; off with `--include-excerpt=false` |
| `items[].approval` | `null`, or `{required, status, reviewers, assurance, expires}` when `[governance]` requires approval of the item or the lock records one; see [Approvals](approvals.md) |
| `mcp_servers` | the project's MCP servers: `ref`, `name`, `transport`, `command_basename`, `enabled`, `profiles`, `pinned` (exact version or digest in a package-runner launch; `null` when not applicable), and the *names* of `env` and `headers` entries, each marked `literal` or with the variable it references (`ref`); `warnings` flags an unpinned launch or a credential written as a literal |
| `lint` | project totals, counts per code, and the findings no item owns |
| `notes` | why a section is missing or narrowed |

MCP servers never expose arguments, URLs, env values or header values: a catalog is published, and those carry launch
secrets and internal hostnames. Only the executable's file name and the names of env and header entries appear.

Paths are relative to the configuration directory; no absolute path of the machine appears. A consumer must refuse
a `schema_version` it does not know. The MCP `catalog` tool prints version 1, equal to the CLI default.

## Static website

```bash
ai-rulez catalog --html site/
ai-rulez catalog --html site/ --role backend --clean
```

Writes a directory with an overview (search by name, domain, owner, kind and lint status), one page per item and
per role, the MCP servers, the lock status, the lint findings, an About page, `catalog.json` (the version 2 document the pages are
rendered from), `assets/catalog.css`, `assets/catalog.js` and `robots.txt`.

- **Offline.** All links are relative, so the site works from `file://` and under any URL path. Nothing is fetched:
  no fonts, CDN, analytics or `fetch`. Every page is complete without JavaScript; the script only adds filtering and
  copy buttons.
- **Reproducible.** The same input gives the same bytes: no timestamps, no absolute paths, sorted everywhere. The
  footer shows the tool version and the sha256 of `catalog.json`, not a date. Regenerate and diff to detect a
  tampered hosted copy.
- **Escaped.** Every string from the repository goes through `html/template` contextual escaping. Links are built
  from slugged keys, never from source text; source URLs are text. Invisible and direction-changing characters
  (bidi controls, zero-width characters) are replaced with U+FFFD and the item is marked "contains hidden
  characters". Bodies are shown as plain text in `<pre>`, never rendered.
- **Content-Security-Policy.** Each page carries `default-src 'none'; img-src 'self' data:; style-src 'self';
  script-src 'self'; base-uri 'none'; form-action 'none'` in a meta tag, and no inline script or style. A host
  should send the same as response headers plus `frame-ancestors 'none'`, which a meta tag cannot set.
- **Output directory.** It must be new, empty or contain the marker `.ai-rulez-catalog` (written by an earlier run,
  listing each file it wrote with its SHA-256). `--clean` removes only a listed file that has the shape of a
  site file (`index.html`, `items/`, `roles/`, `assets/`, `catalog.json`, ...) and still holds the recorded bytes;
  without the marker the run is refused, so `--html .` cannot overwrite a project, and a directory with a `.git` or
  `.ai-rulez` folder is always refused. The marker is written before the files, so an interrupted run leaves the
  directory marked. Writes never follow a symlink out of the directory.
- **Secrets.** The run is refused when the secret scanner (`AR001`) flagged an item and the site would publish its
  excerpt or description; remove the secret, or pass `--allow-findings AR001` (discouraged).

Flags: `--role R` (items role `R` keeps), `--include-excerpt` (default on), `--indexable` (no `robots.txt`, no
`noindex`; excerpts default off), `--clean`, `--base-title T`, `--allow-findings`.

A published catalog exposes names, descriptions, owners, token costs and lint findings: treat it like the
configuration directory it describes.

## Design decisions

- **Builder stays in `internal/govview`.** The issue names `internal/catalog`; the builder already lives in
  `internal/govview` next to the roles and lock views that the CLI and the MCP tools share, and renaming would touch
  every caller for no behaviour change. The renderer is a separate package, `internal/catalogsite`.
- **Dual emit.** Version 1 is the default and `--schema-version 2` opts in (the issue's proposed answer). The HTML
  command always builds version 2.
- **Excerpt default.** On locally, off with `--indexable` (the issue's proposal); `--include-excerpt` overrides.
- **Secret check reuses `AR001`.** No new rule code.
- **Lint at the configured level.** Findings come from the in-process lint engine with the project's `[lint]`
  settings, not the strict level.
- **No `catalog-data.js`.** The pages are server-rendered and the filter reads the table rows, so a second copy of the
  data is not needed; `catalog.json` is the machine contract.
- **Approval.** The overview has an Approval column (the status, or `not required`); the item page shows the status with
  `(required)`, the reviewers and the expiry, escaped like every other value.
- **No pagination.** All rows are in the page; the filter hides rows.

## Not yet built

eval and usage sections, `--check` freshness gate, `--no-lint-messages`, `--no-owners`,
`--link-sources`, `--single-file` and Markdown rendering of bodies are later phases of the
design.
