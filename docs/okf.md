# OKF (Open Knowledge Format)

ai-rulez can export its rules, context, skills, agents and commands as an
[OKF](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md)
bundle, import an existing OKF bundle into `.ai-rulez/`, keep a bundle in sync
through the opt-in `okf` preset, and lint any bundle with `ai-rulez okf validate`.

OKF is a directory of markdown files with YAML frontmatter, meant to be read by
people and by agents. ai-rulez implements **OKF spec v0.2** (`okf_spec = "0.2"`).
The reader is tolerant, the writer is strict.

## Sources and what was verified

Research was done on 2026-10-05.

| Source | URL | Status |
| --- | --- | --- |
| Normative spec | `GoogleCloudPlatform/knowledge-catalog`, `okf/SPEC.md` | Read in full. Pinned at repo commit `58e16bdb7a34430f055ea57e84655cff37000c03`; the file last changed in `62432a095456147ee71e70ac6e4dc0d2dea3ac30` (2026-08-21, "every timestamp is an ISO 8601 datetime with an explicit offset") |
| Spec, new home | `GoogleCloudPlatform/open-knowledge-format`, `SPEC.md` | Byte-identical to the above at commit `ad30107c31c06aec8a7d5636e0d1058118604e6f`; the ecosystem map says the older `knowledge-catalog/okf` copy is frozen |
| Reference bundles | `open-knowledge-format/bundles/{acme_retail,ga4,stackoverflow,crypto_bitcoin}` | Read `acme_retail`; a trimmed copy is vendored as a test fixture (see [Fixtures](#fixtures-and-licences)) |
| Third-party site | https://okf.md/ (`/`, `/spec/`, `/quickstart/`, `/validator/`, `/tools/`, `/ecosystem-map/`, `/skill/`, `/faq/`) | Read. It is a guide around the spec, not the spec |
| Skill repo | https://github.com/fabricioctelles/skills (`skills/okf-open-knowledge-format`) | Read the listing; its `validate.sh` (E1-E4, W1-W7) is the only "validator" with exit codes |
| Candidate Go implementations | `cwest/okfctl`, `openknowledge-sh/openknowledge` | Evaluated, see [Build or borrow](#build-or-borrow) |

### Spec facts (verified against SPEC.md v0.2)

- A bundle is a directory tree of UTF-8 markdown files. A concept is any `.md`
  file other than the reserved `index.md` and `log.md`; its ID is the path minus
  `.md`. Directory layout is the producer's choice.
- Frontmatter: only `type` is REQUIRED and must be a non-empty string. Recommended:
  `title`, `description`, `resource`, `tags`. v0.2 adds the optional families
  `sources`, `generated`, `verified`, `status` (`draft`, `stable`, `deprecated`),
  `stale_after`, and the `Attested Computation` type with `runtime`, `parameters`,
  `computation`, `executor`, `attester`. All timestamps are ISO 8601 with an explicit
  UTC offset. `timestamp` and the body `# Citations` list are v0.1 leftovers.
- Type values are free text, not registered. Consumers MUST tolerate unknown types.
  Unknown extra keys MUST be preserved on round trip and MUST NOT cause rejection.
- Links are standard markdown links, either bundle-relative (`/tables/x.md`,
  recommended) or relative (`./x.md`). Consumers MUST tolerate broken links.
- `index.md` may appear in any directory. It has NO frontmatter, except that the
  bundle-root one MAY carry `okf_version`. Body: headings, each followed by bullets
  `* [Title](url) - description`. Subdirectory entries use `subdir/` in the spec text
  and `subdir/index.md` in the official `acme_retail` bundle; both occur.
- `log.md` is optional: `## YYYY-MM-DD` headings, newest first, prose bullets.
- Conformance is exactly three rules: every non-reserved `.md` has parseable
  frontmatter; every frontmatter has a non-empty `type`; present `index.md` and
  `log.md` follow their structure. A consumer MUST NOT reject a bundle for missing
  optional fields, unknown types, unknown keys, broken links or missing index files.
- Versioning is `<major>.<minor>`, declared as `okf_version: "0.2"` in the root
  `index.md`. Consumers that do not know the version should read best-effort.

### Where okf.md and the spec disagree

The marketing site describes a different shape than the normative spec. ai-rulez
follows the spec and reads the site's shape only leniently.

| okf.md home page claims | Normative SPEC.md v0.2 |
| --- | --- |
| "Three rules": index.md per bundle, typed frontmatter, git-native history | Three rules: parseable frontmatter, non-empty `type`, reserved files well formed. `index.md` is optional. Git is a recommendation, not a rule |
| `index.md` frontmatter has `title`, `version` (semver `0.1.0`) and `entries` | `index.md` has no frontmatter except `okf_version` at the root; the listing is the body |
| `type` is one of `concept`, `howto`, `reference`, `decision`, `metric` | Free text; the examples are `Metric`, `Playbook`, `Reference`, `BigQuery Table`, `Attested Computation` |
| "semantic versioning protecting the investment" | `<major>.<minor>` only |
| Browser validator, "coming soon" | No validator is specified |
| Footer: MIT | The spec repo is Apache-2.0; the `skills` repo is Apache-2.0 too |

The site's `validate.sh` (in `fabricioctelles/skills`) exits with its error count,
and treats a non-directory argument as exit 1. It checks E1 no frontmatter, E2 empty
`type`, E3 frontmatter in a nested `index.md`, E4 an Attested Computation without
`runtime`; warnings W1 (no title or description), W3 (legacy `timestamp`), W5 (`log.md`
without ISO headings), W6, W7 (stale), and an unknown `status`.

### Assumed or uncertain

- The spec is young (v0.1 on 2026-06-12, v0.2 on 2026-07-24), pre-1.0, and single
  vendor. Fields may still be renamed. ai-rulez pins 0.2 and treats any other declared
  version as best-effort (AR9B3, info).
- Whether `index.md` entries must list every concept is **not** required by the spec
  ("entries SHOULD include the description"). AR9B0 is therefore a warning, not an error.
- `x-ai-rulez` as an extension key is allowed by section 4.1 (any extra key), but no
  OKF validator is known to reject it; the Rust and Go linters listed on okf.md check
  `type` and reserved files only. Not tested against every third-party linter.
- Whether other tools preserve unknown keys on their own round trip is unverified.

## Build or borrow

ai-rulez needs a small bundle model, a conformance validator and an index writer.
Both Go candidates are Apache-2.0, as is the spec; ai-rulez is MIT, so copying
would need attribution notices.

| Criterion | `cwest/okfctl` (a0b5072) | `openknowledge-sh/openknowledge` (1d6b0e4) |
| --- | --- | --- |
| License | Apache-2.0, copyright Google LLC headers | Apache-2.0 |
| Usable as a library | No: everything is `internal/`, Cobra CLI | No: module `packages/cli`, everything `internal/` |
| Spec correctness | Floor matches v0.2 (parseable frontmatter, non-empty `type`, index frontmatter rules incl. the `okf_version` carve-out). Puts `okf_version` in a `.okf` sidecar and scaffolds the index without it | Not audited in depth; built around a claims/RDF model, not the bare spec |
| Dependencies | cobra, yaml.v3, goldmark + goldmark-meta (small) | mangle-go, goRDFlib, antlr, jsonschema, fsnotify, flock, x/term, and a `telemetry` package |
| Size of the part we need | `internal/okf` is ~7.2k lines for authoring, search, eval, migrate, promote; the conformance floor (`validate.go`, `frontmatter.go`, `reserved.go`) is ~330 lines | Large, not separable |
| Tests | 60 test files in `internal/okf`, five fixture bundles in `testdata/` | 109 test files |
| Fit for ai-rulez | Different index shape (tags suffix, shape text); API built around its own Bundle and Node | Telemetry and heavy deps rule it out |

**Decision: (c) write our own against SPEC.md, borrow only naming.** The floor is
about 300 lines in Go with `yaml.v3`, which ai-rulez already uses, so copying
would buy no maintained code and would add NOTICE obligations and okfctl's index
conventions. We reuse okfctl's *check vocabulary* so its users recognise the codes
(table below) and we rebuilt its five fixture *cases* (bad-frontmatter, empty-type,
no-type, unknown-type, good-bundle) as our own tiny fixtures. Nothing is copied, so
no NOTICE entry is needed. openknowledge is not used (telemetry, dependencies).

| okfctl check | ai-rulez code |
| --- | --- |
| `orphan` | AR9B4 `okf-orphan` |
| `broken-link` | AR9B2 `okf-link-broken` |
| `spec-version` | AR9B3 `okf-version-invalid` |
| `type-hygiene` / missing type / unparseable frontmatter (floor) | AR9B1 `okf-type-invalid` |
| index shape and `index check` drift | AR9B0 `okf-index-mismatch` |
| reserved-file frontmatter / log headings (floor) | AR9B6 `okf-reserved-structure` |
| (none) export drift | AR9B5 `okf-export-drift` |

## Mapping

ai-rulez concepts become OKF concepts. The real identity lives in the `x-ai-rulez`
extension key, so a round trip is lossless; the OKF `type` is only for OKF readers.

Layout of an exported bundle (`docs/okf/` by default):

```
index.md                        okf_version + one section per kind
rules/index.md  rules/<id>.md
context/<id>.md
skills/<id>/SKILL.md            + references/, scripts/, assets/ next to it
agents/<id>.md
commands/<id>.md
domains/<domain>/<kind>/...     same shape for domain content
```

| ai-rulez | OKF `type` written | Notes |
| --- | --- | --- |
| rule | `Decision` | A rule states a decision the team made. Override with the `okf_type` frontmatter key |
| context | `Concept` | Background knowledge |
| skill | `Playbook` | The SKILL.md body is the procedure; resources ship next to it |
| agent | `Reference` | OKF has no agent type. `x-ai-rulez.kind: agent` restores it |
| command | `Reference` | Same, `kind: command` |
| skill resource (markdown) | `Reference` | Wrapped in frontmatter so the bundle stays conformant; stripped on import. Non-markdown resources are copied verbatim |
| metric | n/a | ai-rulez has no metric concept; an imported `Metric` becomes context |

Nothing is skipped silently: agents and commands are exported as `Reference` and
only left out when `--include` / `include` excludes them, and the command prints
what it wrote per kind.

Frontmatter written for every concept:

```yaml
---
type: Decision
title: Testing
description: One line taken from the item's description
x-ai-rulez:
  kind: rule          # rule | context | skill | agent | command | skill-resource
  id: testing
  domain: backend     # only for domain content
  metadata:           # priority, targets, globs, paths, owner, version, ... with their YAML types
    priority: high
    globs: ["**/*_test.go"]
---
```

No timestamps, no `generated.by` (it would change with the tool version). History is
git's job. `index.md` entries are sorted and each carries the description.

### Import mapping

`type` (case-insensitive, `-`/`_`/space ignored) decides the target when the concept has
no `x-ai-rulez.kind`:

| OKF type | ai-rulez |
| --- | --- |
| `decision`, `rule`, `convention`, `policy`, `guideline`, `standard` | rule |
| `howto`, `playbook`, `runbook`, `procedure`, `skill` | skill |
| `concept`, `reference`, `metric`, anything else (incl. `Attested Computation`) | context |

`--into rules|context|skills` forces every concept to that kind. OKF keys with no
ai-rulez equivalent (`tags`, `resource`, `sources`, `status`, `stale_after`, ...) are kept
as extra frontmatter so they survive a later export.

## Codes

See [strict validation](strict-validation.md). AR9B0-AR9B9 are reserved for OKF.

| Code | Name | Default | Finds |
| --- | --- | --- | --- |
| AR9B0 | `okf-index-mismatch` | warning | An `index.md` entry points at a missing file, or a directory with an `index.md` has a concept or subdirectory it does not list |
| AR9B1 | `okf-type-invalid` | error | Unparseable frontmatter, or `type` missing or empty (conformance rules 1 and 2). Unknown type *values* are allowed by the spec and not reported |
| AR9B2 | `okf-link-broken` | warning | A relative or bundle-relative markdown link whose target is not in the bundle (the spec tolerates these, so never an error by default) |
| AR9B3 | `okf-version-invalid` | warning | Root `okf_version` is not `MAJOR.MINOR`; info when it is well formed but not `0.2` |
| AR9B4 | `okf-orphan` | info | A concept reachable from no index entry and no link (only checked when the bundle has an index) |
| AR9B5 | `okf-export-drift` | error | The configured bundle differs from what `export okf` would write now (project lint only) |
| AR9B6 | `okf-reserved-structure` | error | Frontmatter in a nested `index.md`, keys other than `okf_version` in the root one, or `log.md` headings that are not ISO dates |
| AR9B7 | `okf-title-duplicate` | info | Two concepts in one directory share a title |
| AR9B8 | `okf-path-unsafe` | error | A symlink, a path escaping the bundle, or two paths differing only in case |
| AR9B9 | `okf-lossy-mapping` | info | A concept carries `x-ai-rulez` data this version cannot map (unknown `kind`, invalid metadata) and imports as plain context |
