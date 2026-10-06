# SBOM

`ai-rulez sbom` prints a bill of materials of the project's AI configuration as
[CycloneDX](https://cyclonedx.org/) 1.6 or [SPDX](https://spdx.dev/) 2.3 JSON. Instructions an agent follows are a
supply chain; the SBOM lists it in the formats security tooling already ingests.

```bash
ai-rulez sbom > ai-bom.cdx.json                              # CycloneDX 1.6
ai-rulez sbom --format spdx-json -o ai-bom.spdx.json         # SPDX 2.3
ai-rulez sbom --files skills --role backend --require-lock   # a role's slice, files hashed, lock must match
ai-rulez sbom -o ai-bom.cdx.json --check                     # CI: fail when the committed SBOM is stale
```

| Flag | Meaning |
| --- | --- |
| `--format cyclonedx\|spdx-json` | Output format (default `cyclonedx`; `spdx` is accepted for `spdx-json`) |
| `-o`, `--output file` | Write the document to a file instead of stdout |
| `--files none\|skills\|all` | List the files of items with their plain SHA-256 (default `none`) |
| `--profile name`, `--role name` | Describe one profile's or role's slice |
| `--include-outputs` | List the generated files with their output digest |
| `--no-approvals`, `--redact-reviewers` | Leave approval status out, or replace reviewer identities with a salted hash |
| `--verify` | Verify the lock attestation and record the result |
| `--require-lock` | Exit 2 (`AR752`) unless `ai-rulez.lock` matches the sources |
| `--strict-pins` | Exit 2 (`AR750`, `AR751`) when an MCP package or remote source is not pinned or has no package URL |
| `--check` | With `-o`: exit 2 (`AR753`) when the committed SBOM differs from a fresh one |
| `--timestamp [RFC3339\|now]` | Record a time; `SOURCE_DATE_EPOCH` is honoured when the flag is absent |
| `--online` | Contact remote includes and skill sources (`git ls-remote`) |
| `-n`, `--config-dir name` | Configuration directory name |

Exit codes: `0` ok, `1` could not run, `2` a gate (`--require-lock`, `--strict-pins`, `--check`) failed. When a gate
fails nothing is written.

Nothing is rendered and nothing is written except `--output`. `sbom` **does not use the network by default**: remote
includes and skill sources are read from `ai-rulez.lock` and the local cache (run `ai-rulez generate` or
`ai-rulez lock` once to fill it), and one that is neither pinned nor cached is warned about and listed without
content. `--online` lets it contact the remotes the way `generate` does, which is only needed to resolve a moving ref.
The machine-local overlay (`config.local.*`, `local/`) is never included, so the document is the same on every
checkout.

## What it lists

| CycloneDX | SPDX 2.3 | Content |
| --- | --- | --- |
| `components[]`, type `data` | `packages[]`, related to the project by `CONTAINS` | Every rule, context file, skill, agent, command, check, hook and role (and the permission and managed-settings pins), with `ai-rulez:kind`, `ai-rulez:domain`, `ai-rulez:path`, `ai-rulez:digest`; `version` and `ai-rulez:owner` from the item's frontmatter; `description` and `licenses` from the frontmatter |
| nested `components[]`, type `file` | `files[]`, related to their package by `CONTAINS` | The files of an item (`--files`), and always the scripts of a skill |
| `components[]`, type `data` | `packages[]`, related by `DEPENDS_ON` | Each remote include, installed skill and `[[skill_sources]]` entry: `purl`, `vcs` reference (SPDX `downloadLocation` `git+https://host/path@commit`), `ai-rulez:ref`, and the commit and digest pinned in `ai-rulez.lock` |
| `components[]`, type `application` | `packages[]`, related by `DEPENDS_ON` | Each local MCP server (command based): name, command executable, enabled, profiles, env key names, and a `purl` |
| `services[]` | `packages[]`, related by `DEPENDS_ON` | Each remote MCP server (`url`): endpoint reduced to scheme and host, `authenticated` when headers are set. SPDX 2.3 has no service type: the package says "remote service" in its comment |
| `components[]`, type `file` | `packages[]` | Generated outputs (`--include-outputs`) with `ai-rulez:output-digest` |
| `metadata.component` | the root package `SPDXRef-Config`, described by the document | The project, with `ai-rulez:tree`, `ai-rulez:lock` (`present`/`absent`), `ai-rulez:lock-in-sync`, and the slice (`ai-rulez:profile`, `ai-rulez:role`, `ai-rulez:files`, `ai-rulez:lock-scope` when the lock pins only skills) |
| `dependencies[]` | `relationships[]` | The project depends on every component and service; a role depends on the items it keeps; a skill or agent depends on the skills its `skills:` frontmatter names |

Items come from the same digests as [`ai-rulez.lock`](lockfile.md), computed from the sources (CRLF normalised), so the
SBOM digest of an item equals the lock's digest of it. `ai-rulez:lock-in-sync` is `false` when the lock no longer
matches the sources.

SPDX has no property bag, so every `ai-rulez:` property of a package is written to its `comment`, one `name=value` per
line. SPDX identifiers are the component reference with the characters SPDX does not allow replaced and a hash of the
original appended, so two references never share one.

### Files and hashes

With `--files skills` the files of every skill are listed, with `--files all` those of every item (rules, context,
agents, commands, checks, skills). Each file has its **plain SHA-256** of the bytes on disk (CycloneDX `hashes`; SPDX
`checksums` with SHA-1 as well, because SPDX 2.3 requires one on every file, and a package verification code). Plain
hashes follow the checkout's line endings: they differ between an LF and a CRLF checkout, while the tree digests in
`ai-rulez:digest` do not. Without `--files` the scripts of a skill (files under `scripts/` or with an executable bit) are
still listed, marked `ai-rulez:executes = true` and without a hash, so a reviewer sees the code that runs on a
developer's machine and the default document stays byte-identical across line endings.

### Licenses

The `license` of a skill's frontmatter becomes the component's license. An SPDX id is `license.id`, an SPDX expression
of ids (`MIT OR Apache-2.0`, `GPL-2.0-only WITH Classpath-exception-2.0`) is `expression`, and anything else is
`license.name`. SPDX gets the id or expression as `licenseDeclared`, or a `LicenseRef-` (with the declared string in
`hasExtractedLicensingInfos`) for free text; `licenseConcluded` is always `NOASSERTION`. A license is never guessed.
Text from frontmatter (`description`, `license`) has control, format and bidirectional characters removed and is capped
at 1 KiB, so it cannot smuggle hidden content to the tool that reads the SBOM.

### Package URLs

Remote sources: `pkg:github/<owner>/<repo>@<commit or ref>#<path>` (also `gitlab`, `bitbucket`); any other host is
`pkg:generic/<name>@<version>?vcs_url=git+https://host/path`. A local path or `file://` source gets no purl and no
location (`ai-rulez:source-location` is `local`). The in-repository `path` of a source is emitted as `ai-rulez:path`
unless it is absolute or contains `..`, in which case it is left out.

An MCP server declares its package with the optional `package` key (a purl, never written to any harness file):

```toml
[[mcp_servers]]
name = "fs"
command = "/usr/local/bin/fs-server"
package = "pkg:npm/%40modelcontextprotocol/server-filesystem@1.2.3"
```

A declared package is used as it is (`ai-rulez:purl-source = declared`). Without one the package is guessed from the
launcher and marked `ai-rulez:purl-source = heuristic`:

| Command | Purl |
| --- | --- |
| `npx`, `bunx`, `pnpm dlx`, `yarn dlx` | `pkg:npm/%40scope/name@version` |
| `uvx`, `pipx run`, `uv tool run` | `pkg:pypi/name@version` (`==` or `@` pins, `--from`) |
| `docker run`, `podman run` | `pkg:oci/name@sha256:...?repository_url=...&tag=...` |
| `go run module@version` | `pkg:golang/module@version` |

Anything else (a binary path, a local script, a package URL or tarball) gets no purl. Arguments are only read to find
the package and are never emitted. A declared `package` that is not a package URL, or that carries credentials in a
qualifier, is ignored (and reported as `AR751`).

### Pinning

A component is **pinned** when it names one release: an exact version (`1.2.3`, `==0.4.0`, `v1.2.3`), an image digest, a
declared purl with an exact version, or a git source with a commit (from the lock, or a commit SHA as `ref`).
`ai-rulez:pinned` says `true` or `false`. An unpinned package has **no version in its purl** (a scanner would match
`latest` or a range against the wrong release) and `ai-rulez:requested-version` keeps what was asked for. `--strict-pins`
fails on:

- `AR750` an MCP package or git source that is not pinned (a range, `latest`, a bare major, an image tag, an include on
  a branch with no lock);
- `AR751` an MCP server with no package URL at all.

Both are `info` findings: they are listed only with `--strict-pins`, which turns them into exit 2.

## Scoping

`--profile` keeps the root content and the domains the profile names (a composed value works), and drops MCP servers
and installed skills scoped to other profiles. `--role` keeps the items the role selects, the context files, the role
itself and the non-selectable items (hooks, settings); remote sources and MCP servers are listed in full because roles
select content, not servers. The slice is stated on the project component, and a scoped document has its own serial
number. An unknown profile or role is an error. `--profile` and `--role` can be combined.

`--require-lock` makes the SBOM a statement about the pinned content: it fails with `AR752` when there is no lock, or the
lock no longer matches the sources.

## Approvals and signature

With `[governance]` configured, or approval records in the lock, every pinned item and source gets its approval status
from the same evaluation as [`ai-rulez approve`](approvals.md): `ai-rulez:approval` is `approved`, `not-required`,
`missing`, `stale`, `expired`, `unauthorized` or `insufficient`; `ai-rulez:approvers` lists the reviewers of the current
digest, `ai-rulez:approval-assurance` is the weakest assurance among the counted approvals (`asserted`, `review-linked` or `signed`) and `ai-rulez:approval-expires` the earliest expiry. Expiry is judged
by the wall clock, except under `--check`, which judges it at the time the committed document records (so an approval
that expires later does not make an unchanged SBOM drift; `approve --list` and `validate --strict` report the expiry). SPDX also gets one `REVIEW` annotation per approving record (with its assurance), dated with the record's `approved_at`.
Approvals are claims recorded in the lock, not proof of review.

Reviewer identities are personal data. `--redact-reviewers` replaces each with `reviewer-` and eight hex digits of a
SHA-256 over the lock tree and the normalised identity (stable within one lock, not comparable across projects). The salt is public, so an identity that can be guessed (an
email, a login) can be confirmed against the token: set `AI_RULEZ_SBOM_REDACT_KEY` to key the hash (HMAC-SHA-256, still
stable for one key) or use `--no-approvals` for a public document;
`--no-approvals` leaves approvals out altogether.

`--verify` verifies the lock attestation offline against `[signing]` trust, the way `ai-rulez verify --attestation`
does, without touching the rollback state, and records `ai-rulez:signature` (`verified`, `absent` or `invalid`, with
`ai-rulez:signature-code`) and, when verified, `ai-rulez:signer`, `ai-rulez:signer-issuer` and `ai-rulez:signed-at` on
the project component (an SPDX `OTHER` annotation on the root package). It exits 1 when no trusted signer is configured or
the lock pins nothing, because the check could not run.

## Checking a committed SBOM

`sbom -o file --check` generates the document in memory and compares it with `file`, which it never rewrites. It exits 0
when they agree, and 2 (`AR753`) when they differ or the file is missing, naming what was added, removed or changed:

```text
AR753 sbom-drift: sbom.cdx.json: differs from the SBOM generated now; regenerate it with `ai-rulez sbom -o sbom.cdx.json`
  components: added ai-rulez:item:rule::r2
  components: changed ai-rulez:item:rule::r1
```

The ai-rulez version recorded in the document (`metadata.tools`, SPDX `creationInfo.creators`) is ignored, so upgrading
the tool does not fail the check on its own. A `--timestamp` that moves (`now`) always differs: do not use one with
`--check`, or pin it with `SOURCE_DATE_EPOCH`.

## In `validate --strict`

`validate --strict` builds the SBOM in memory (nothing is written, no network) and reports the same findings under the
`config` analyzer, against `config.toml`:

- `AR750` and `AR751` (`info`) for MCP packages and remote sources that cannot be given an exact version or a package
  URL, as `--strict-pins` does (an unpinned `npx` package is also `AR012`; the two say it from the lint and the SBOM
  side);
- `AR752` (`error`) when `ai-rulez.lock` exists and no longer matches the sources. A project without a lock is not a
  finding here: `--require-lock` is the opt-in for that. The lock drift codes (`AR98x`) name what changed;
- `AR753` (`error`) when a committed CycloneDX SBOM at the project root (`ai-bom.cdx.json` or `sbom.cdx.json`) differs
  from a fresh one. The document says how it was made (`ai-rulez:files`, `profile`, `role`, the listed outputs, its
  timestamp), so no flags are needed; whether `--no-approvals` was used is not recorded, so the document is current when
  either reading matches. A file another tool made, an SPDX file, a document with hashed reviewers (`--redact-reviewers`)
  or a recorded attestation check (`--verify`) is skipped: use `sbom --check` for those, which takes the flags itself.

## Signing the SBOM

An SBOM proves nothing about who made it. Sign it with `ai-rulez sign --sbom sbom.cdx.json --key cosign.key` (see
[Signing](signing.md)), which attests the document's SHA-256 (the file's bytes, whatever the format) with a key or a
keyless identity and writes `sbom.cdx.json.sigstore.json` next to it. Check it with
`ai-rulez verify --sbom sbom.cdx.json` against `[[signing.trust]]` entries with `subject = "sbom"`. `sbom` itself writes
no signature, and a later `sbom -o` that changes the file makes the old attestation fail (`AR724`).

## Determinism

No `timestamp` unless `--timestamp` or `SOURCE_DATE_EPOCH` is given. SPDX requires `creationInfo.created`, so it carries
the fixed placeholder `1970-01-01T00:00:00Z` (with a comment saying so) by default. Components, services, properties,
files, relationships and dependencies are sorted; the same project yields the same bytes on every run, operating
system and line ending (see Files and hashes for the one exception, plain file hashes). The CycloneDX `serialNumber` and
the SPDX `documentNamespace` are a UUIDv5 over the lock tree (the `tree` of `ai-rulez.lock` when it pins content,
otherwise the tree computed from the sources and the remote pins), the list of component references and the slice
options, so they change when the content, an MCP server, a source or the slice changes, and not otherwise.
`metadata.tools` records the ai-rulez version, so the bytes also change when you upgrade.

## No secrets

- Environment and header **values** of MCP servers are never read into the document; only the key names are listed
  (`ai-rulez:env-keys`, `ai-rulez:header-keys`).
- URLs lose userinfo, query and fragment (`https://u:p@host/mcp?key=x` becomes `https://host/mcp`). A URL that is a
  `${VAR}` placeholder is omitted. A secret inside the URL path cannot be told from the path and is kept: do not put
  one there.
- The digest of the generated MCP settings is left out, because it covers env and header values.
- ai-rulez digests are `sha256` tree digests of ai-rulez's own scheme, not file hashes, so they appear only in
  `ai-rulez:`-namespaced properties and never in CycloneDX `hashes` or SPDX `checksums`.

## Validating

Both outputs validate against the upstream JSON schemas: CycloneDX `bom-1.6.schema.json` and SPDX 2.3.1
`spdx-schema.json`. The repository's tests do this against copies vendored under `internal/sbom/testdata`, whose
digests are pinned in a test, so updating one is a reviewed change. Any CycloneDX or SPDX tool accepts the files.

## Design decisions

- **Formats**: CycloneDX 1.6 and SPDX 2.3 JSON. SPDX 3 and newer CycloneDX minors are not offered; `--spec-version` does
  not exist yet.
- **One build, two renderings**: the SPDX document is derived from the same model as the CycloneDX one, so the two
  always agree.
- **Item component type**: `data`. Rules, skills and the like are content, not software.
- **Serial number**: derived, not random, so a diff of two SBOMs of an unchanged project is empty. The component
  references are mixed in because MCP servers are not pinned by the lock.
- **SPDX `created`**: a fixed placeholder, not the clock, so the output stays reproducible. Pass `--timestamp` for a
  real time.
- **MCP purls**: a declared `package` is authoritative; a guess identifies what the launcher would download, not what
  is installed, and is marked as heuristic.
- **Settings pins**: the `mcp-servers` settings pin is omitted (secrets), every other settings pin is listed.
- **Findings** (`AR750`-`AR753`) are reported by `sbom` and by `validate --strict`; see
  [In `validate --strict`](#in-validate-strict).
- **Not offered**: `--include-builtins`, `--redact-hosts`, `--spec-version`, a `[sbom]` config table and `sbom --sign`.
