---
title: "OKF (Open Knowledge Format)"
description: "OKF is a directory of markdown files with YAML frontmatter, meant to be read by people and by agents."
---

[OKF](https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/main/SPEC.md) is a directory of markdown files
with YAML frontmatter, meant to be read by people and by agents. ai-rulez implements OKF spec 0.2. You can:

- export your rules, context, skills, agents, commands and checks as an OKF bundle, and keep it in sync with the `okf`
  preset;
- import an OKF bundle into `.ai-rulez/`, or use one directly as an [include](#okf-bundles-as-includes);
- lint any bundle with `ai-rulez okf validate`;
- store `.ai-rulez/` itself as an OKF bundle (the [native tree](#the-native-tree), which `init` creates).

## Example

```bash
ai-rulez export okf
```

```text
Wrote 4 files to /home/dev/acme/docs/okf (2 rules)
```

```text
docs/okf/
├── .okf-export.json
├── index.md
└── rules/
    ├── commit-messages.md
    ├── index.md
    └── typescript.md
```

```markdown
<!-- docs/okf/index.md -->
---
okf_version: "0.2"
---

# Subdirectories

- [rules](rules/index.md) - Rules exported from ai-rulez
```

```markdown
<!-- docs/okf/rules/typescript.md -->
---
type: Decision
title: Typescript
x-ai-rulez:
  kind: rule
  id: typescript
  metadata:
    paths:
      - src/**/*.{ts,tsx}
---

Prefer `type` over `interface` for unions. No `any`.
```

Any OKF reader sees a `Decision` concept titled "Typescript". ai-rulez reads the `x-ai-rulez` key back, so
`export okf`, `import okf`, `export okf` is byte-identical.

Importing a bundle someone else wrote:

```bash
ai-rulez okf validate ../kb
ai-rulez import okf ../kb --dry-run
```

```text
would create     .ai-rulez/context/tables-orders.md
would create     .ai-rulez/skills/runbooks-deploy/SKILL.md
index style: body
Summary: 2 created, 0 overwritten, 0 unchanged, 0 conflicts, 0 skipped
```

A `BigQuery Table` concept at `tables/orders.md` became context, and a `Playbook` at `runbooks/deploy.md` became a
skill. See [Import mapping](#import-mapping).

## Mapping

Each ai-rulez item becomes one OKF concept. The `type` is for OKF readers; the identity lives in the `x-ai-rulez`
extension key.

| ai-rulez                  | OKF `type` written | Notes                                                                    |
| ------------------------- | ------------------ | ------------------------------------------------------------------------ |
| rule                      | `Decision`         |                                                                          |
| context                   | `Concept`          |                                                                          |
| skill                     | `Playbook`         | `SKILL.md`; `references/`, `scripts/` and `assets/` next to it          |
| agent, command, check     | `Reference`        | OKF has no such types; `x-ai-rulez.kind` restores them                  |
| skill resource (markdown) | `Reference`        | wrapped in frontmatter so the bundle stays conformant; unwrapped on import |

Set `type` (or `title`) in an item's `okf` frontmatter map to write a different value, for example
`okf: {type: Policy}` on a rule.

Frontmatter of every exported concept:

```yaml
---
type: Decision
title: Testing                 # the id in words, unless okf.title is set
description: One line          # the item's description, when it has one
x-ai-rulez:
  kind: rule                   # rule | context | skill | agent | command | check | skill-resource
  id: testing
  domain: backend              # only for domain content
  metadata:                    # every other frontmatter key, with its YAML type
    priority: high
    globs: ["**/*_test.go"]
---
```

Layout of an exported bundle:

```text
index.md                              okf_version, then one entry per kind directory
rules/index.md  rules/<id>.md
context/<id>.md
skills/<id>/SKILL.md                  + index.md, references/, scripts/, assets/
agents/<id>.md  commands/<id>.md  checks/<id>.md
domains/<domain>/<kind>/...           same shape for domain content
```

- Every directory with a concept gets an `index.md`, entries sorted by path, each with the concept's description.
- No `log.md`, timestamps or `generated` fields are written, so the output is deterministic; history is git's.
- Bodies are copied byte for byte. Executable bits of scripts are kept.
- A name that is not safe in a path is written to a transliterated path and restored from `x-ai-rulez.id` on import.
  An item named `index` or `log` is stored as `index_.md` / `log_.md`, since OKF reserves those names.

### Import mapping

A concept with `x-ai-rulez.kind` is imported as that kind. Otherwise its `type` decides (case-insensitive, `-`, `_`
and spaces ignored):

| OKF type                                                            | Becomes |
| ------------------------------------------------------------------- | ------- |
| `decision`, `rule`, `convention`, `policy`, `guideline`, `standard` | rule    |
| `howto`, `playbook`, `runbook`, `procedure`, `skill`                | skill   |
| anything else (`concept`, `reference`, `metric`, `BigQuery Table`, ...) | context |

- The item id is the concept's path with `/` turned into `-` (`runbooks/deploy.md` becomes the skill
  `runbooks-deploy`). Rename and refine afterwards: a skill needs a description that says when to use it.
- `--into rules|context|skills` forces every concept to one kind; `--domain <name>` places everything in a domain.
- OKF keys with no ai-rulez equivalent (`tags`, `resource`, `sources`, `status`, `stale_after`, a foreign `type` or
  `title`, ...) are kept in an `okf` frontmatter map, and `export okf` writes them back at the top level:

  ```yaml
  ---
  description: The orders table
  okf:
    type: BigQuery Table
    title: Orders
    tags: [sales]
  ---
  ```

- Links between imported concepts are rewritten to the relative path of the created file
  (`/runbooks/deploy.md` becomes `../skills/runbooks-deploy/SKILL.md`). A link to a file that was not imported is
  left as written and reported as `AR9B9`. On export, relative links between exported items become bundle-absolute
  (`/context/architecture.md`). Links in code blocks are never touched.
- Non-markdown files are imported only from `references/`, `scripts/` or `assets/` next to a skill. `log.md` and
  `index.md` are not imported.
- `import okf` writes the files with their keys at the top level. In a [native tree](#the-native-tree), run
  `ai-rulez migrate okf` afterwards so `validate` sees the OKF shape.

### Import report

`import okf --format json` prints:

```json
{
  "schema_version": 1,
  "source": "../kb",
  "target": ".ai-rulez",
  "dry_run": false,
  "index_style": "body",
  "actions": [
    { "path": "context/tables-orders.md", "kind": "context", "status": "created", "source": "tables/orders.md" },
    { "path": "skills/runbooks-deploy/SKILL.md", "kind": "skills", "status": "created", "source": "runbooks/deploy.md" }
  ],
  "findings": [],
  "security": [],
  "skipped": []
}
```

| Field         | Meaning                                                                                     |
| ------------- | ------------------------------------------------------------------------------------------- |
| `actions`     | one entry per file: `path` (relative to `target`), `kind`, `status` and the bundle `source` |
| `status`      | `created`, `overwritten`, `unchanged` or `conflict` (the file exists and differs)           |
| `findings`    | `AR9B*` findings for the bundle, with `code`, `name`, `severity`, `path`, `line`, `message` |
| `security`    | `AR001`-`AR011` findings; any error refuses the whole import                                |
| `skipped`     | files not imported (symlinks, unsupported files)                                            |

Exit codes: `0` done; `2` refused by the security scan, or files exist and differ (rerun with `--force` to
overwrite); `1` the bundle could not be read.

### Titles and conflicts

`title` is the item id in words (`code-style` is "Code Style") unless the item has `okf.title`. An import keeps a
bundle title that differs as `okf.title`, so an edited title survives export, edit, `import --force`, export.

- `import okf` never overwrites a file that differs unless you pass `--force`; then the bundle wins.
- `export okf` and `generate` write the sources' title, so a title edited only in the bundle is drift (`AR9B5`).
  Run `generate` to restore the source, or `import okf --force` to adopt the edit.

### Index styles

Two `index.md` schemes are in use. ai-rulez reads both and writes the one you configure; concept files are identical
in both.

| Style              | `index_style`   | Shape                                                                                  |
| ------------------ | --------------- | -------------------------------------------------------------------------------------- |
| Body (default)     | `"body"`        | OKF 0.2: no frontmatter except `okf_version` at the root; `# Concepts` and `# Subdirectories` headings with `- [Title](file.md) - description` bullets |
| Frontmatter        | `"frontmatter"` | `title`, `version: 0.1.0` and an `entries` list in the frontmatter, no body (the scheme described on the [OKF website](https://okf.md/)); the root also keeps `okf_version` |

```yaml
---
okf_version: "0.2"
title: Index
version: 0.1.0
entries:
  - title: rules
    path: rules/index.md
    description: Rules exported from ai-rulez
---
```

Set it with `[okf] index_style` or `export okf --index-style` (the flag wins). `generate --check` and
`export okf --check` compare the configured style, so a bundle in the other style is drift.

## The native tree

`ai-rulez init` creates `.ai-rulez/` as an OKF bundle, and `ai-rulez migrate okf` converts an older tree in place:
the frontmatter of every item moves under `x-ai-rulez.metadata`, `type` and `title` are added, and each directory gets
an `index.md` (the root one names `okf_version`). The generated output does not change.

```bash
ai-rulez migrate okf --dry-run   # list what would change
ai-rulez migrate okf             # convert; running it again changes nothing
ai-rulez migrate okf --check     # exit 2 while files still need migrating
```

- `type`, `title` and `x-ai-rulez` are reserved keys. The loader reads `x-ai-rulez.metadata` (and any other
  top-level key) as the item's frontmatter.
- Only `type` is required (without it, `AR9B1`). `title`, `description` and `x-ai-rulez` are optional:
  `x-ai-rulez.kind` defaults to the directory and `x-ai-rulez.id` to the file or skill directory name. `add` and
  `init` write all of them.
- `add`, `remove` and the MCP CRUD tools write this form and refresh the `index.md` files. Once the root `index.md`
  lists its directories, `generate` refreshes stale indexes too, including after you add files by hand. For files
  without OKF keys, `migrate okf` adds them and rewrites the indexes in one step.
- `validate` runs the OKF checks (`AR9B*`) on such a tree, together with every other analyzer; `--analyzer okf` runs
  them alone.
- A tree without a root `index.md` uses the older layout. It still loads, with a deprecation notice from `validate`,
  `generate` and `list`. See [Migrating to v5](/ai-rulez/migration-v5/#okf-is-the-format-of-ai-rulez).

What `migrate okf` does not do:

- **Reserved names.** If an item is named `index` or `log` (`rules/index.md`), the migration refuses before writing
  anything, names the files and exits 1. Rename them and run it again.
- **Symlinks** are never followed or rewritten. They are listed as `[skipped]`, and the run exits 1; replace the link
  with the real file. A file whose frontmatter does not parse is skipped the same way.
- **Backups.** Before rewriting, the originals are copied to `<config dir>.bak-okf-<timestamp>/`. `validate` ignores
  that directory; delete it once you have reviewed the result.

## Codes

`AR9B0`-`AR9B9` are the OKF codes, reported by `okf validate`, `validate` and `doctor`. See
[Strict validation](/ai-rulez/strict-validation/).

| Code    | Name                     | Default | Finds                                                                                             |
| ------- | ------------------------ | ------- | ------------------------------------------------------------------------------------------------- |
| `AR9B0` | `okf-index-mismatch`     | warning | an index entry that points at a missing file, a concept or subdirectory the index does not list, or an index whose titles or descriptions are stale |
| `AR9B1` | `okf-type-invalid`       | error   | unparseable frontmatter, a missing or empty `type`, or an unknown `x-ai-rulez.kind`; a `kind` that disagrees with its directory is a warning |
| `AR9B2` | `okf-link-broken`        | warning | a relative or bundle-relative link whose target is not in the bundle                              |
| `AR9B3` | `okf-version-invalid`    | warning | a root `okf_version` that is not `MAJOR.MINOR` (info when it is not `0.2`, or for a frontmatter-style root index you did not configure); an error from `okf validate` when there is no root `index.md` with `okf_version` |
| `AR9B4` | `okf-orphan`             | info    | a concept no index entry or link reaches (only when the bundle has an index)                     |
| `AR9B5` | `okf-export-drift`       | error   | the configured bundle differs from what `export okf` would write now                              |
| `AR9B6` | `okf-reserved-structure` | error   | frontmatter keys an `index.md` may not have; a `log.md` heading that is not an ISO date (warning) |
| `AR9B7` | `okf-title-duplicate`    | info    | two concepts in one directory with the same title                                                 |
| `AR9B8` | `okf-path-unsafe`        | error   | a symlink, a path escaping the bundle, a file over the size limit, or two paths that differ only in case |
| `AR9B9` | `okf-lossy-mapping`      | info    | from `import okf`: `x-ai-rulez` data that cannot be mapped, or a link to a file that was not imported |

Unknown `type` values, unknown keys and missing optional fields are allowed by the spec and never reported.

## Commands

| Command                                                                       | Does                                                       |
| ----------------------------------------------------------------------------- | ---------------------------------------------------------- |
| [`export okf`](/ai-rulez/cli-reference/#ai-rulez-export-okf) `[--output-dir dir] [--profile p \| --role r] [--include kinds] [--index-style s] [--check]` | write, or with `--check` compare, the bundle |
| [`import okf`](/ai-rulez/cli-reference/#ai-rulez-import-okf) `<dir\|git-url> [--into kind] [--domain d] [--dry-run] [--force]` | bundle to `.ai-rulez/` sources |
| [`migrate okf`](/ai-rulez/cli-reference/#ai-rulez-migrate-okf) `[--dry-run] [--check]` | convert `.ai-rulez/` in place to an OKF bundle            |
| [`okf validate`](/ai-rulez/cli-reference/#ai-rulez-okf-validate) `<dir\|git-url> [--fail-on sev] [--format json]` | lint any bundle; exit 2 on findings at `--fail-on` (default `error`) |
| [`convert --from okf`](/ai-rulez/cli-reference/#ai-rulez-convert)                     | the import mapping through convert's plan, lossiness report and `--merge`; finds a bundle at the source root or in `docs/okf` |

- `export okf --role r` exports the slice of content a [role](/ai-rulez/roles/) selects, like `generate --role`. Served skills
  are exported like static ones; `skill_mode` is not exported.
- `export okf` records the files it wrote in `.okf-export.json` and only ever removes files listed there, so files
  you add to the bundle directory are never pruned. It refuses an `--output-dir` that is, contains or lies inside the
  configuration directory.
- A remote source is `https://host/org/repo[.git][@ref][#subdir]`, `git@host:org/repo[@ref]` or
  `file:///path/repo[@ref]`. Plain `http://` is refused. Git runs without hooks, prompts or submodules.

## Configuration

```toml
presets = ["claude", "cursor", "okf"]   # the okf preset keeps the bundle in sync on generate

[okf]
dir = "docs/okf"                         # default
include = ["rules", "context", "skills"] # default: every kind
spec = "0.2"                             # the only accepted value
index_style = "body"                     # or "frontmatter"
```

| Key           | Type   | Default    | Meaning                                                      |
| ------------- | ------ | ---------- | ------------------------------------------------------------ |
| `dir`         | string | `docs/okf` | where `export okf` and the preset write the bundle           |
| `include`     | list   | all kinds  | kinds to export: `rules`, `context`, `skills`, `agents`, `commands`, `checks` |
| `spec`        | string | `0.2`      | OKF version written                                          |
| `index_style` | string | `body`     | see [Index styles](#index-styles)                            |

The preset exports the profile `generate` runs with, from your own shared sources: never `.ai-rulez/local/`, and not
domains or items that come from builtins or includes. The bundle is committed documentation: `gitignore = true` never
ignores it, and it has no generated-by banner. `generate --check` and `doctor` report a hand-edited, missing or stale
bundle file as drift. While `generate --role` runs, the preset leaves the bundle untouched.

## OKF bundles as includes

An OKF bundle can feed `generate` directly, without a one-time import:

```toml
[[includes]]
name = "team-kb"
source = "https://github.com/acme/knowledge"
ref = "v1.2"
path = "bundles/platform"     # directory of the bundle inside the source
format = "okf"
include = ["rules", "skills"]
```

The bundle is converted with the import mapping at load time and merged like any other [include](/ai-rulez/includes/): same
precedence, same cache, same [lock file](/ai-rulez/lockfile/) pinning, same `install_to`. The security scan runs on the
converted text, and a bundle with an error-level finding is refused as a whole.

## Security

- Import never follows symlinks in a bundle (each is skipped with `AR9B8`) and writes only below the target
  directory. Ids and resource paths that are not plain names are rejected.
- All text, scripts included, is scanned with `AR001`-`AR011` before the first write; one error-level finding (a
  secret, hidden Unicode, `curl | sh`) refuses the whole import. `ignore` comments in imported text are not honored.
- Bundles are bounded (50,000 files, 8 MiB per file). Nothing in a bundle is executed.
- OKF content is instructions for agents: importing it from a stranger is like adding any other include. Read the
  `--dry-run` output first.

## CI

```bash
ai-rulez generate --check                               # fails when docs/okf is stale (okf preset)
ai-rulez validate                                       # AR9B* for the configured bundle
ai-rulez okf validate docs/okf --fail-on warning --format json
```

Validate a third-party bundle before importing it:

```bash
ai-rulez okf validate https://github.com/GoogleCloudPlatform/open-knowledge-format@main#bundles/acme_retail
ai-rulez import okf https://github.com/GoogleCloudPlatform/open-knowledge-format@main#bundles/acme_retail \
  --domain acme --dry-run
```

## Troubleshooting

| Symptom                                       | Cause and fix                                                                                   |
| --------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `AR9B1` on a file you wrote by hand           | The file has no `type`. Add it, or run `ai-rulez migrate okf`.                                   |
| `AR9B0` / `AR9B4` after adding files by hand  | The `index.md` listings do not include them yet. Run `ai-rulez generate`.                       |
| A `.ai-rulez.bak-okf-*` directory appears     | A backup of rewritten files. Delete it after review, and do not commit it.                       |

## Limitations

- Only OKF 0.2 is written. OKF 0.1 bundles (`timestamp`, `# Citations`) are read as they are.
- The trust, provenance and lifecycle fields (`sources`, `generated`, `verified`, `status`, `stale_after`) and the
  `Attested Computation` type are preserved but have no effect in ai-rulez.
- A foreign concept maps to exactly one kind.
- Agents, commands and checks travel as `Reference` with `x-ai-rulez.kind`. A tool that drops unknown keys loses
  that information.
- Key order and comments in a source file's frontmatter are normalized by an export.
