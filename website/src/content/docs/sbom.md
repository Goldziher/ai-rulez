---
title: "SBOM"
description: "ai-rulez sbom writes a bill of materials of your AI configuration as CycloneDX 1.6 or SPDX 2.3 JSON: every rule, skill, hook and agent, every remote source and MCP server, with digests, pins and approval status."
---

`ai-rulez sbom` writes a bill of materials of your AI configuration as [CycloneDX](https://cyclonedx.org/) 1.6 or
[SPDX](https://spdx.dev/) 2.3 JSON: every rule, skill, hook and agent, every remote source and MCP server, with digests,
pins and approval status. Use it to feed the instructions your assistants follow into the security tooling you already
run.

## Example

```bash
ai-rulez sbom -o ai-bom.cdx.json                 # CycloneDX 1.6
ai-rulez sbom --type spdx-json -o ai-bom.spdx.json
ai-rulez sbom -o ai-bom.cdx.json --check         # CI: fail when the committed SBOM is stale
```

An excerpt of the CycloneDX document for a project with a skill, a hook and an MCP server:

```json
{
  "bomFormat": "CycloneDX",
  "specVersion": "1.6",
  "serialNumber": "urn:uuid:d5158f98-fd54-50e7-9a88-28d84eceef9e",
  "version": 1,
  "metadata": {
    "tools": { "components": [{ "type": "application", "name": "ai-rulez", "version": "5.10.0" }] },
    "component": {
      "type": "application",
      "bom-ref": "ai-rulez:project",
      "name": "acme",
      "properties": [
        { "name": "ai-rulez:lock", "value": "present" },
        { "name": "ai-rulez:lock-in-sync", "value": "true" },
        { "name": "ai-rulez:tree", "value": "sha256:4dd87fd5ae23b632928921220800a83b679b58a58cd0e2fa3127cd5196dfcbee" }
      ]
    }
  },
  "components": [
    {
      "type": "data",
      "bom-ref": "ai-rulez:item:skill::deploy",
      "group": "skill",
      "name": "deploy",
      "version": "1.2.0",
      "description": "Deploy the service",
      "properties": [
        { "name": "ai-rulez:approval", "value": "stale" },
        { "name": "ai-rulez:digest", "value": "sha256:70094bee730997ae05d11499b29571a05dde6cb5f17a4af98ed29465ba50d855" },
        { "name": "ai-rulez:kind", "value": "skill" },
        { "name": "ai-rulez:owner", "value": "platform" },
        { "name": "ai-rulez:path", "value": "skills/deploy" }
      ],
      "components": [
        {
          "type": "file",
          "bom-ref": "ai-rulez:item:skill::deploy:file:skills/deploy/scripts/run.sh",
          "name": "skills/deploy/scripts/run.sh",
          "properties": [
            { "name": "ai-rulez:executes", "value": "true" },
            { "name": "ai-rulez:kind", "value": "file" }
          ]
        }
      ]
    },
    {
      "type": "application",
      "bom-ref": "ai-rulez:mcp:fs",
      "name": "fs",
      "purl": "pkg:npm/%40modelcontextprotocol/server-filesystem",
      "properties": [
        { "name": "ai-rulez:enabled", "value": "true" },
        { "name": "ai-rulez:kind", "value": "mcp-server" },
        { "name": "ai-rulez:mcp-command", "value": "npx" },
        { "name": "ai-rulez:pinned", "value": "false" },
        { "name": "ai-rulez:purl-source", "value": "heuristic" }
      ]
    }
  ],
  ...
}
```

`sbom` renders nothing and writes nothing but `--output`. It does not use the network by default: remote sources are
read from `ai-rulez.lock` and the local cache (run `ai-rulez lock` or `generate` once to fill it). The machine-local
overlay is never included, so the document is the same on every checkout.

## Options

| Flag | Default | Meaning |
| --- | --- | --- |
| `--type cyclonedx\|spdx-json` | `cyclonedx` | Document type; `spdx` is accepted for `spdx-json` |
| `-o`, `--output <file>` | stdout | Write the document to a file |
| `--files none\|skills\|all` | `none` | List the files of skills, or of every item, with their plain SHA-256 |
| `--profile <name>`, `--role <name>` | all content | Describe one profile's or role's slice ([Scoping](#scoping)); they combine |
| `--include-outputs` | off | List the generated files with their output digest |
| `--no-approvals` | off | Leave approval status out |
| `--redact-reviewers` | off | Replace reviewer identities with a salted hash |
| `--verify` | off | Verify the lock attestation and record the result |
| `--require-lock` | off | Exit `2` (`AR752`) unless `ai-rulez.lock` matches the sources |
| `--strict-pins` | off | Exit `2` (`AR750`, `AR751`) when an MCP package or remote source is not pinned or has no package URL |
| `--check` | off | With `-o`: exit `2` (`AR753`) when the committed file differs from a fresh document |
| `--timestamp <RFC3339\|now>` | none | Record a time; `SOURCE_DATE_EPOCH` is used when the flag is absent |
| `--online` | off | Contact remote includes and skill sources (`git ls-remote`), only needed to resolve a moving ref |
| `--format text\|json` | `text` | Report format of a run that prints no document (`--check`, `-o`, a failed gate); `json` follows `schema/sbom-report.schema.json` |

Exit codes: `0` ok, `1` could not run, `2` a gate (`--require-lock`, `--strict-pins`, `--check`) failed. When a gate
fails nothing is written.

## What it lists

| CycloneDX | SPDX 2.3 | Content |
| --- | --- | --- |
| `components[]`, type `data` | `packages[]`, `CONTAINS` | Every rule, context file, skill, agent, command, check, hook and role, and the permissions and managed-settings pins: `ai-rulez:kind`, `ai-rulez:domain`, `ai-rulez:path`, `ai-rulez:digest`, and `version`, `ai-rulez:owner`, `description` and `licenses` from frontmatter |
| nested `components[]`, type `file` | `files[]`, `CONTAINS` | The files of an item with `--files`; always the scripts of a skill |
| `components[]`, type `data` | `packages[]`, `DEPENDS_ON` | Each remote include, installed skill and skill source: `purl`, VCS reference, `ai-rulez:ref`, and the commit and digest pinned in the lock |
| `components[]`, type `application` | `packages[]`, `DEPENDS_ON` | Each command-based MCP server: name, command, enabled, profiles, env key names, `purl` |
| `services[]` | `packages[]`, `DEPENDS_ON` | Each remote MCP server: endpoint reduced to scheme and host, `authenticated` when headers are set |
| `components[]`, type `file` | `packages[]` | Generated outputs with `--include-outputs`, with `ai-rulez:output-digest` |
| `metadata.component` | root package `SPDXRef-Config` | The project: `ai-rulez:tree`, `ai-rulez:lock` (`present`/`absent`), `ai-rulez:lock-in-sync`, and the slice (`ai-rulez:profile`, `ai-rulez:role`, `ai-rulez:files`) |
| `dependencies[]` | `relationships[]` | The project depends on every component and service; a role on the items it keeps; a skill or agent on the skills its `skills:` frontmatter names |

The item digests are the [lock's digests](/ai-rulez/lockfile/#hashing-scheme), so the SBOM digest of an item equals the lock's.
SPDX has no property bag, so `ai-rulez:` properties go to each package's `comment`, one `name=value` per line.

### Files and hashes

With `--files skills` the files of every skill are listed, with `--files all` those of every item. Each file has the
plain SHA-256 of its bytes on disk (CycloneDX `hashes`; SPDX `checksums`, with the SHA-1 SPDX 2.3 requires). Plain
hashes follow the checkout's line endings, while `ai-rulez:digest` does not. Without `--files`, a skill's scripts
(under `scripts/` or executable) are still listed, marked `ai-rulez:executes = true` and without a hash, so a reviewer
sees the code that runs on a developer's machine.

### Licenses

The `license` in a skill's frontmatter becomes the component's license: an SPDX id is `license.id`, an SPDX expression
(`MIT OR Apache-2.0`) is `expression`, anything else is `license.name` (SPDX: a `LicenseRef-`). A license is never
guessed; `licenseConcluded` is always `NOASSERTION`. Frontmatter text has control and bidirectional characters removed
and is capped at 1 KiB.

### Package URLs and pinning

Remote sources get `pkg:github/<owner>/<repo>@<commit or ref>#<path>` (also `gitlab`, `bitbucket`), or
`pkg:generic/<name>@<version>?vcs_url=git+https://host/path` for other hosts. A local source gets no purl.

An MCP server can declare its package with the optional `package` key (a purl; never written to any harness file):

```toml
[[mcp_servers]]
name = "fs"
command = "/usr/local/bin/fs-server"
package = "pkg:npm/%40modelcontextprotocol/server-filesystem@1.2.3"
```

A declared package is used as written (`ai-rulez:purl-source = declared`). Otherwise the package is derived from the
launcher (`ai-rulez:purl-source = heuristic`):

| Command | Purl |
| --- | --- |
| `npx`, `bunx`, `pnpm dlx`, `yarn dlx` | `pkg:npm/%40scope/name@version` |
| `uvx`, `pipx run`, `uv tool run` | `pkg:pypi/name@version` (from `==` or `@` pins, and `--from`) |
| `docker run`, `podman run` | `pkg:oci/name@sha256:...?repository_url=...&tag=...` |
| `go run module@version` | `pkg:golang/module@version` |

Anything else (a binary path, a local script) gets no purl. Arguments are read only to find the package and are never
emitted.

A component is **pinned** (`ai-rulez:pinned = true`) when it names one release: an exact version, an image digest, or a
git source with a commit. An unpinned package has no version in its purl, so a scanner does not match the wrong
release; `ai-rulez:requested-version` keeps what was asked for. `--strict-pins` turns these into failures:

```console
$ ai-rulez sbom --strict-pins
AR750 sbom-component-unpinned: mcp-server fs: the package has no version; pin it (for example name@1.2.3)
```

- `AR750`: an MCP package or git source that is not pinned (a range, `latest`, a branch with no lock);
- `AR751`: an MCP server with no package URL, or a declared `package` that is not a valid purl.

## Scoping

`--profile` keeps the root content and the domains the profile names, and drops MCP servers and installed skills
scoped to other profiles. `--role` keeps the items the role selects, the context files, the role itself, hooks and
settings; remote sources and MCP servers stay, because roles select content, not servers. The slice is recorded on the
project component, and a scoped document has its own serial number. An unknown profile or role is an error.

## Approvals and signature

With `[governance]` configured or approval records in the lock, every item and source gets its status from the same
evaluation as [`ai-rulez approve`](/ai-rulez/approvals/):

| Property | Value |
| --- | --- |
| `ai-rulez:approval` | `approved`, `not-required`, `missing`, `stale`, `expired`, `unauthorized` or `insufficient` |
| `ai-rulez:approvers` | the reviewers of the current digest |
| `ai-rulez:approval-assurance` | the weakest assurance among the counted approvals |
| `ai-rulez:approval-expires` | the earliest expiry |

SPDX also gets one `REVIEW` annotation per approving record, dated with its `approved_at`. Approvals are claims recorded
in the lock, not proof of review.

Reviewer identities are personal data. `--redact-reviewers` replaces each with `reviewer-` and eight hex digits
(`reviewer-b70c2f8e`), stable within one lock. The default salt is public, so a guessable identity can be confirmed
against it: set `AI_RULEZ_SBOM_REDACT_KEY` to key the hash, or use `--no-approvals` for a public document.

`--verify` checks the lock attestation offline against `[signing]` trust, as
[`verify --attestation`](/ai-rulez/signing/#verify) does, and records `ai-rulez:signature` (`verified`, `absent` or `invalid`,
with `ai-rulez:signature-code`) and, when verified, `ai-rulez:signer`, `ai-rulez:signer-issuer` and `ai-rulez:signed-at`
on the project component. It exits `1` when no trusted signer is configured.

## Checking a committed SBOM

`sbom -o <file> --check` builds the document in memory and compares it with the file, which it never rewrites:

```console
$ ai-rulez sbom -o ai-bom.cdx.json --check
AR753 sbom-drift: ai-bom.cdx.json: differs from the SBOM generated now; regenerate it with `ai-rulez sbom -o ai-bom.cdx.json`
  components: added ai-rulez:item:rule::review
  dependencies: changed ai-rulez:project
  serialNumber changed: "urn:uuid:fa3d4992-3838-52e2-b892-7b169dc26cca" -> "urn:uuid:55518116-92c2-540b-8cc6-43bc7fee1505"
  ...
```

It exits `0` when they agree and `2` when they differ or the file is missing. The recorded ai-rulez version is ignored,
so upgrading does not fail the check by itself. Do not combine `--timestamp now` with `--check`; pin the time with
`SOURCE_DATE_EPOCH` instead. Approval expiry under `--check` is judged at the time the document records, so an approval
that expires later does not make an unchanged SBOM drift.

## In `validate`

`validate` builds the SBOM in memory and reports, under the `config` analyzer:

| Code | Severity | When |
| --- | --- | --- |
| `AR750`, `AR751` | `info` | MCP packages and remote sources without an exact version or package URL |
| `AR752` | `error` | `ai-rulez.lock` exists and no longer matches the sources (a project without a lock is not a finding) |
| `AR753` | `error` | a committed CycloneDX SBOM at the project root (`ai-bom.cdx.json` or `sbom.cdx.json`) differs from a fresh one |

For `AR753`, the document records how it was made (`--files`, profile, role, outputs, timestamp), so no flags are
needed. SPDX files, files another tool made, and documents made with `--redact-reviewers` or `--verify` are skipped:
check those with `sbom --check` and the same flags.

## Signing the SBOM

An SBOM proves nothing about who made it. Sign it and verify it with a trust entry for the `sbom` subject:

```console
$ ai-rulez sign --sbom ai-bom.cdx.json --key release.key
INFO  Signed sbom path=ai-bom.cdx.json signer=key sha256:c49716c6… subject=sha256:757766f3… bundle=ai-bom.cdx.json.sigstore.json
$ ai-rulez verify --sbom ai-bom.cdx.json --public-key release.pub
OK  sbom  signer=sha256:c49716c6aeb4b89ba6176d8b7e7951b9a8801e92498d9104b4e4c58f8d8e0ac9
    issuer=none (key)  logged=no  age=0s
    no transparency log or timestamp: the signing time is the signer's own claim (weak freshness)
    digest=sha256:757766f3c240073573d92a87b6698aad8f2349bc7352932ff83a4301a60ce80f
```

The signature covers the file's bytes. Regenerating the file with different content makes the old attestation fail
(`AR724`). In a project, trust signers with `[[signing.trust]]` entries with `subject = "sbom"` ([Signing](/ai-rulez/signing/)).

## Determinism

The same project yields the same bytes on every run, operating system and line ending (except plain file hashes):

- no timestamp unless `--timestamp` or `SOURCE_DATE_EPOCH` is given; SPDX `creationInfo.created` is the placeholder
  `1970-01-01T00:00:00Z`, with a comment saying so;
- components, properties, files, relationships and dependencies are sorted;
- the CycloneDX `serialNumber` and the SPDX `documentNamespace` are derived from the lock tree, the component list and
  the slice, so they change when the content, an MCP server, a source or the slice changes;
- `metadata.tools` records the ai-rulez version, so the bytes change when you upgrade.

## No secrets

- MCP server env and header **values** are never read; only key names are listed (`ai-rulez:env-keys`,
  `ai-rulez:header-keys`).
- URLs lose userinfo, query and fragment (`https://u:p@host/mcp?key=x` becomes `https://host/mcp`); a `${VAR}` URL is
  omitted. A secret in a URL path cannot be told apart from the path: do not put one there.
- The `mcp-servers` settings digest is left out, because it covers env and header values.
- ai-rulez tree digests appear only in `ai-rulez:` properties, never in CycloneDX `hashes` or SPDX `checksums`.

Both outputs validate against the upstream CycloneDX 1.6 and SPDX 2.3 JSON schemas.
