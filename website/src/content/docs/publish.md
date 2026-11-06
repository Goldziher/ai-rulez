---
title: "Publish"
description: "ai-rulez publish turns the generated plugin bundle into release artifacts: a reproducible tar.gz, a manifest, checksums, release notes and a plan."
---

`ai-rulez publish` turns the generated [plugin bundle](/ai-rulez/plugins/) into release artifacts: a reproducible `tar.gz`, a
manifest, checksums, release notes and a plan. Building is local and offline. Nothing leaves the machine unless you pass
`--execute --yes`, and then the upload goes through the platform's own tooling (`gh` for GitHub releases, `npm` for
npm, the Docker credential store for OCI registries). ai-rulez never stores or logs a credential.

[Emitters](#emitters) render extra formats from the same plugin; the [`ard`](/ai-rulez/ard/),
[`mcp-server-card`](/ai-rulez/mcp-server-card/) and [`agent-plugins`](/ai-rulez/agent-plugins/) emitters each have their own page.

## Quick start

From a project with a `[plugin]` block (see [Authoring plugins](/ai-rulez/plugins/#quick-start)):

```bash
ai-rulez generate --plugin
ai-rulez lock
git add -A && git commit -m "acme-tools 1.2.0"
git tag v1.2.0 && git push --tags

ai-rulez publish --to github-release --dry-run      # preflight, then print the artifacts and the command
ai-rulez publish --to github-release --execute --yes
ai-rulez publish verify dist
```

The dry run prints what it would write and run:

```text
preflight   validate --strict ok | lock ok | verify --plugin ok | secrets 0
signature   none
artifacts   would write 6 files to /work/acme-tools/dist
            RELEASE_NOTES.md                         sha256:188cc8d8...  403 bytes
            SHA256SUMS                               sha256:c0df2f4d...  350 bytes
            acme-tools-1.2.0.manifest.json           sha256:d4a01a9e...  3299 bytes
            acme-tools-1.2.0.tar.gz                  sha256:110cc9fa...  1937 bytes
            ai-rulez.lock                            sha256:d248d9a0...  645 bytes
            publish-plan.json                        sha256:3564a26b...  2016 bytes
would run   gh release create v1.2.0 --repo acme/acme-tools --title 'acme-tools 1.2.0' --notes-file RELEASE_NOTES.md --verify-tag acme-tools-1.2.0.tar.gz acme-tools-1.2.0.manifest.json ai-rulez.lock SHA256SUMS
credentials gh authentication (GH_TOKEN or `gh auth login`); not read by ai-rulez
```

Without `--to`, `ai-rulez publish` only builds `dist/`. `publish verify` checks a dist directory or a downloaded
release:

```text
verified acme-tools 1.2.0: 4 files match SHA256SUMS, the manifest and the archive (signature: none)
```

## Pipeline

1. **Resolve.** `[plugin] name` and `version` are required.
2. **Preflight**, stopping before anything is written:
    - `validate` (the content checks, failing at your `fail_on`, at most `error`);
    - `lock --check`;
    - `verify --plugin`;
    - a secret scan of every bundle file, using the security scan's patterns.

   A symlink in the bundle is an error, and the machine-local overlay is never loaded. When the bundle holds an
   [Agent Plugins](/ai-rulez/agent-plugins/) package, it is checked against the specification (`AR9O0`-`AR9O5`), in
   `--dry-run` as well.
3. **Policy gates** from [`[publish]`](#configuration): `require_signature` (`AR9N7`) and `require_approved`
   (`AR9N8`).
4. **Build** the dist directory, signing the archive when asked.
5. **Upload**, only with `--to` and `--execute --yes`.

The working tree must be clean and every bundle file must be tracked by git (`AR9N3`). `--allow-dirty` waives both;
the manifest then records `dirty = true`. Changes under the dist directory are ignored.

## Dist directory

| File | Content |
| --- | --- |
| `<name>-<version>.tar.gz` | the bundle: every file `generate --plugin` writes for the published runtimes |
| `<name>-<version>.manifest.json` | each file with its size and digest, the source (repository, commit, dirty flag), the lock tree, the runtimes, and the signature, SBOM and approval records ([schema](/ai-rulez/schema/)) |
| `ai-rulez.lock` | a copy of the lock without its `[[approval]]` records, so reviewer emails and notes stay in the repository. `lock --check` against the copy behaves as against the original |
| `SHA256SUMS` | `sha256sum -c` format; every file except itself and the plan |
| `RELEASE_NOTES.md` | the bundle digest, runtimes, lock tree, commit, and the [changes since the previous release](#release-notes) |
| `publish-plan.json` | the artifacts with digests and the exact commands `--execute` runs; no timestamps or local paths |
| `<name>-<version>.tar.gz.sigstore.json` | with signing: the archive signature |
| `<name>-<version>.attestation.sigstore.json` | with signing: a signed statement binding the name, version, archive, lock and SBOM |
| `<name>-<version>.sbom.cdx.json` | with `--sbom`: the project's CycloneDX [SBOM](/ai-rulez/sbom/) |
| `marketplace/` | with `--marketplace`: the [pinned marketplace index](#pinned-marketplace-and-channels) |
| `emit/<emitter>/` | output of [emitters](#emitters) and `--template` files |
| `npm/`, `oci/manifest.json` | the npm package directory, or the OCI manifest, for those targets |

The archive holds only files that `generate --plugin` writes. A file you keep in the repository yourself, such as an
MCP launcher script at the repository root, is not in it; ship such files with
[`[plugin] include`](/ai-rulez/agent-plugins/#bundling-files) or install the plugin from git.

`--dist DIR` changes the directory (default `dist`). It must be new, empty, or the output of an earlier publish, whose
artifacts are replaced; any other non-empty directory is refused.

The archive is byte-identical for the same bundle and commit: entries sorted, regular files only, uid/gid 0, modes
`0644` or `0755`, no gzip name or time. The mtime is `SOURCE_DATE_EPOCH`, else the committer time of `HEAD`. Build
releases with one ai-rulez version: the gzip bytes can differ between versions. The signature is the only artifact that
differs between runs.

## Targets

| `--to` | Upload | Credentials |
| --- | --- | --- |
| `github-release` | `gh release create` | `gh` (`GH_TOKEN`, `GITHUB_TOKEN` or `gh auth login`) |
| `npm` | `npm pack`, then `npm publish` | your npm config (`npm login`, `NODE_AUTH_TOKEN`) |
| `oci` | an OCI artifact pushed to a registry | the Docker credential store (`docker login`) |

### GitHub release

```bash
ai-rulez publish --to github-release --execute --yes
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--tag` | `v<version>` | release tag; it must already exist on the remote (`gh ... --verify-tag`) |
| `--repo` | `[publish.github_release] repo`, else `[plugin] repository`, else the origin remote | `OWNER/REPO` or `HOST/OWNER/REPO` |
| `--force` | off | upload into an existing release, replacing its assets (`gh release upload --clobber`) |

Without `--force`, `--execute` refuses a release that already exists. A signed release also uploads the signature and
attestation; an SBOM release uploads the SBOM. `gh` runs with a filtered environment (the system basics, the `GH_*` and
`GITHUB_*` token and host variables, `XDG_*` directories, proxy and certificate settings). A missing `gh` is `AR9N4`.

### npm

```bash
ai-rulez publish --to npm --npm-scope @acme --dry-run
```

```text
would run   npm pack --ignore-scripts --pack-destination npm ./npm/package
would run   npm publish ./npm/acme-acme-tools-1.2.0.tgz --access restricted --ignore-scripts
registry    https://registry.npmjs.org/ (npm runs from an empty temporary directory with explicit --userconfig and --globalconfig; no project .npmrc applies)
credentials npm authentication (npm login, or NODE_AUTH_TOKEN in an .npmrc); not read by ai-rulez
```

The package is written to `npm/package/`: the bundle files plus a release `package.json`.

- **Name.** `<scope>/<plugin name>`. A scope is required (`[publish.npm] scope` or `--npm-scope`), so nothing lands in
  the unscoped public namespace by accident. A generated runtime name such as `@acme/pi-acme-tools` becomes
  `@acme/acme-tools`.
- **Access.** `restricted` unless `[publish.npm] access = "public"` or `--public`.
- **Dist-tag.** `--channel NAME`. A prerelease version (`1.4.0-rc.1`) needs one (`AR9N6`).
- **`package.json`.** Carries `files`, the repository, and an `ai-rulez` key with the archive digest, lock tree and
  runtimes. Existing runtime metadata (Pi resources, OpenCode entry points and dependencies, keywords, license) is kept.
  A manifest with `private: true` or any `scripts` field is rejected. When the bundle has its own runtime
  `package.json` (OpenCode, Pi), that manifest is rewritten, so the bundle's `.ai-rulez-generated.json` is left out of
  the npm package; the release archive keeps it.
- **Existing versions.** `--execute` re-verifies the dist directory, runs `npm view` and refuses a version the registry
  already has.
- **Isolation.** npm runs from an empty temporary directory with your user and global npm config named explicitly, so a
  `.npmrc` committed in the repository never applies. `--execute` asks npm for the effective registry and refuses one
  that is not `https://`.
- **Custom registry.** A registry set by `[publish.npm] registry` in the committed configuration would receive your
  npm token, so it is refused unless you confirm it with `--confirm-registry URL`. The public registry and a registry
  from your own npm configuration need no confirmation.
- **Signing.** The signature covers the release archive, not the tarball npm packs. `require_signature` therefore fails
  an npm release (`AR9N7`), and a signed npm release prints a warning. Publish the signed archive with
  `--to github-release` or `--to oci`.
- **Provenance.** `npm publish --provenance` only works inside CI with an OIDC identity, so it is not part of the plan.
  Run `publish --to npm` without `--execute` in that CI job, then run the two planned commands yourself with
  `--provenance` added to `npm publish`.

### OCI

```bash
ai-rulez publish --to oci --oci-ref ghcr.io/acme/acme-tools --dry-run
```

```text
push        ghcr.io/acme/acme-tools:1.2.0 (manifest sha256:34e071ec...)
credentials registry credentials from the Docker credential store (`docker login`), read only for the registry named by ref
```

`--oci-ref` (or `[publish.oci] ref`) is a repository `host/path` without a tag; the tag is the plugin version (`+`
becomes `_`). The artifact layout:

```text
artifactType  application/vnd.ai-rulez.bundle.v1
config        application/vnd.ai-rulez.manifest.v1+json      the bundle manifest
layers        application/vnd.ai-rulez.bundle.v1.tar+gzip     the archive
              application/vnd.ai-rulez.lock.v1+toml           the lock
              application/vnd.cyclonedx+json                  the SBOM, with --sbom
              application/vnd.dev.sigstore.bundle.v0.3+json   the signature and attestation, when signed
annotations   org.opencontainers.image.{created,version,source,revision}
```

The manifest is reproducible and its digest is recorded in the plan; `--execute` rebuilds it from the dist files and
refuses to push if it differs. Because a tag is mutable, `--execute` also refuses to move an existing tag to a different
digest unless `--force`; pushing the identical artifact again is a no-op. Plain HTTP is used only for loopback
registries. Consumers should pin by digest: `ghcr.io/acme/acme-tools@sha256:...`.

### Several plugins

A `[marketplace]` with `members` or domain plugins publishes one bundle per plugin, taken from the generated Claude
marketplace index (so the `claude` runtime must be generated):

```text
dist/plugins/<name>/   a complete dist directory per plugin
dist/aggregate/        plugins.json, marketplace/ and emit/ for all plugins, with SHA256SUMS and a plan
```

- `--only NAME` limits the plugins.
- Targets are per plugin: GitHub tags `<name>-v<version>`, npm packages `<scope>/<name>`, OCI repositories
  `<ref>/<name>`. `--tag` does not apply.
- `--runtime` keeps only the listed runtimes of each plugin; a member that ships none of them is refused (`AR9N6`).
- `--execute` checks every plugin's target first and uploads nothing if any check fails. A failure part-way (a
  registry outage) names the plugins already published; fix the cause and rerun, with `--force` where a release is
  already out.
- `publish verify dist` verifies every plugin and the aggregate checksums, and fails (`AR9N5`) when a listed plugin
  directory is missing or changed or an unlisted one is present. `plugins.json` is not signed; trust comes from each
  plugin's own signed release.

`publish` reads the Claude index closest to the project root, so with several
[`[[marketplaces_authoring]]`](/ai-rulez/plugins/#several-marketplaces-from-one-root) entries it publishes only that one.

## Pinned marketplace and channels

`--marketplace` writes `marketplace/.claude-plugin/marketplace.json`: the bundle's Claude marketplace index with every
relative `source` replaced by one pinned to the release commit:

```json
"source": {
  "source": "github",
  "repo": "acme/acme-tools",
  "ref": "v1.2.0",
  "sha": "ee285bdde14184518b0317f7ea3bf1f2de93094b"
}
```

| Plugin location | Source type |
| --- | --- |
| repository root on github.com | `github` (`repo`, `ref`, `sha`) |
| repository root on another host | `url` (`url`, `ref`, `sha`) |
| a subdirectory | `git-subdir` (`url`, `path`, `ref`, `sha`) |

Claude Code checks out `sha`, so the index stays valid if the tag moves. Every other field of the entry is kept.

`--channel NAME` writes `marketplace/<name>/.claude-plugin/marketplace.json` instead, pinned to the ref that
`[publish.marketplace.channels]` names for that channel (the release tag when the channel is not listed). Commit each
channel's index to the branch users register as a marketplace. `--channel` is also the npm dist-tag.

## Emitters

An emitter renders the files a distribution channel needs from the published plugin, without network access.
`--emit NAME` (repeatable) or `[[publish.emitters]]` runs it into `dist/emit/<name>/`. `ai-rulez publish emit NAME
[--output-dir DIR]` writes only that emitter's files (default `emit/<name>/`) and no release; it still runs the full
[preflight](#pipeline), so the project must be committed, locked and its bundle up to date.

| Emitter | Output | Status |
| --- | --- | --- |
| `cursor-team-marketplace` | `.cursor-plugin/marketplace.json` and `plugins/<name>/`, ready to commit to the repository a Cursor team marketplace imports; needs the `cursor` runtime | verified |
| `agent-plugins` | one validated [Agent Plugins](/ai-rulez/agent-plugins/#publishing) directory per plugin, without other runtimes' files; needs the `agent-plugins` or `copilot` runtime, or `codex` with the root layout; option `spec` | verified |
| `ard` | the [Agentic Resource Discovery](/ai-rulez/ard/) manifest `ard.json`; needs `[ard]` | verified |
| `mcp-server-card` | one [MCP server card](/ai-rulez/mcp-server-card/) per remote MCP server; options `namespace`, `version` | verified |
| `port` | one [Port](https://docs.port.io/api-reference/create-an-entity/) entity JSON per plugin and skill, plus `index.json`; option `blueprint` (default `agent_skill`) | experimental |
| `aws-agent-registry` | one [`CreateRegistryRecord`](https://docs.aws.amazon.com/agent-registry-control/latest/APIReference/API_CreateRegistryRecord.html) request body per skill and per plugin | experimental |
| `kiro-steering` | [Kiro steering](https://kiro.dev/docs/steering/) files at `.kiro/steering/*.md` from the root rules and skills, plus `distribution.json` for MDM packaging | experimental |
| `template` | any text, from a Go `text/template` over the release (`--template FILE`) | verified |

Experimental emitters write formats that have no published schema to test against. They refuse to run without
`--experimental` (`AR9N6`) and then report `AR9N9` as a warning.

A template sees the fields `Name`, `Version`, `Tag`, `Repo`, `Commit`, `AIRulezVersion`, `Runtimes`, `BundleFile`,
`BundleDigest`, `BundleSize`, `LockTree`, `LockDigest` and `Files`, and the function `json`:

```text
{{ .Name }} {{ .Version }} ({{ .BundleDigest }}) from {{ .Repo }}@{{ .Commit }}
```

## Signing

```bash
ai-rulez publish --to github-release --sign-key cosign.key --execute --yes   # a key
ai-rulez publish --to github-release --sign-keyless --execute --yes          # Fulcio + Rekor, e.g. in CI
```

| Flag | Meaning |
| --- | --- |
| `--sign-key FILE` | sign with an ECDSA or ed25519 PEM private key (cosign keys work). The password comes from `AI_RULEZ_SIGNING_KEY_PASSWORD`, `COSIGN_PASSWORD`, or the variable named by `--sign-key-password-env` |
| `--sign-tlog` | with `--sign-key`: also record the signature in the Rekor log |
| `--sign-keyless` | sign with a short-lived Fulcio certificate and record it in Rekor. The certificate names your identity in a public log, so use a key for a private repository |
| `--sign-token-env`, `--sign-interactive`, `--fulcio-url`, `--rekor-url` | keyless options |

`--dry-run` signs nothing and contacts nothing. Signing uses the same code as [`ai-rulez sign`](/ai-rulez/signing/).

A release is signed twice:

- `<name>-<version>.tar.gz.sigstore.json` is a signature over the exact archive bytes, the form
  `cosign sign-blob --bundle` writes, so `cosign verify-blob --bundle <file>.sigstore.json --key cosign.pub <file>`
  verifies it.
- `<name>-<version>.attestation.sigstore.json` is a signed in-toto statement whose subjects are the archive, the lock
  copy and the SBOM, and which records the plugin name, version, lock tree, source and approval summary. It ties the
  unsigned manifest to the signed files.

`publish verify` reports a signed release as `unverified` until you name who you trust:

```bash
ai-rulez publish verify dist --key cosign.pub
ai-rulez publish verify ghcr.io/acme/acme-tools@sha256:... \
  --identity https://github.com/acme/acme-tools/.github/workflows/release.yml@refs/tags/v1.2.0 \
  --issuer https://token.actions.githubusercontent.com
```

```text
verified acme-tools 1.2.0: 6 files match SHA256SUMS, the manifest and the archive (signature: verified)
signer: key sha256:afde1a64...
```

Naming a trusted signer (`--key`, `--identity`) implies `--require-signature`, so a release whose signature was
stripped fails. A keyless check uses `--trusted-root`, or the root `ai-rulez trust update` cached.

## Policy gates

```toml
[publish]
require_signature = true   # AR9N7: publish fails unless it signs the archive
require_approved  = true   # AR9N8: publish fails unless the governance policy's items are approved
```

`require_approved` uses [approvals](/ai-rulez/approvals/): every item the `[governance]` policy selects needs a valid approval
in `ai-rulez.lock`. It also fails when no policy selects anything. When a policy is active, the manifest records
`approval = { required, approved }` whether or not the gate is on.

## SBOM

`--sbom` runs `ai-rulez sbom` and ships the CycloneDX document next to the archive. The manifest records it, the OCI
artifact carries it as a layer, and `publish verify` checks it. The SBOM has no timestamp and no approval status, so it
keeps the release reproducible and no reviewer identity leaves the repository. See [SBOM](/ai-rulez/sbom/).

## Release notes

`RELEASE_NOTES.md` lists what changed in the lock since the previous release: added, changed and removed items (kind,
id, domain, short digest) and remote pins, never content. The previous release is the closest tag reachable from `HEAD`
other than the release tag (for a multi-plugin release, the closest earlier `<name>-v*` tag). `--since TAG` picks
another tag and fails when that tag holds no lock. Without a previous tag, the section is omitted.

## Runtime filtering

`--runtime R` (repeatable) or `[publish] runtimes` limits the bundle to some of the plugin runtimes. The full bundle is
verified first; the subset is rendered again by the same generator, so shared files are identical. A runtime that is
not in `[plugin] runtimes` is an error (`AR9N6`). `--runtime` wins over the table.

## Configuration

```toml
[publish]
runtimes = ["claude", "cursor"]   # default: the [plugin] runtimes
require_signature = false
require_approved = false
allow_dirty = false

[publish.github_release]
repo = "acme/acme-tools"          # default: [plugin] repository, else origin

[publish.oci]
ref = "ghcr.io/acme/acme-tools"   # no tag; the tag is the version

[publish.npm]
scope = "@acme"
access = "restricted"             # or "public"
registry = "https://npm.acme.example"

[publish.marketplace.channels]
canary = "main"                   # channel -> the git ref its index pins

[[publish.emitters]]
name = "agent-plugins"
options = { spec = "1.1.0" }

[[publish.emitters]]
name = "template"
template = "tools/release-entity.tmpl"   # inside the project, never through a symlink
output = "release-entity.json"
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `runtimes` | string list | the `[plugin] runtimes` | runtimes to publish |
| `require_signature` | bool | `false` | fail unless the archive is signed (`AR9N7`) |
| `require_approved` | bool | `false` | fail unless the governance policy's items are approved (`AR9N8`) |
| `allow_dirty` | bool | `false` | allow uncommitted changes, as `--allow-dirty` |
| `github_release.repo` | string | `[plugin] repository`, else origin | the release repository |
| `oci.ref` | string | none | OCI repository, without a tag |
| `npm.scope` | string | none | npm scope, required for `--to npm` |
| `npm.access` | string | `restricted` | `restricted` or `public` |
| `npm.registry` | string | your npm config | registry URL; needs `--confirm-registry` |
| `marketplace.channels` | table | none | channel name to git ref |
| `[[emitters]]` | `name`, `options`, `template`, `output` | none | emitters run on every publish |

There are no credential keys: registries authenticate through their own tools and environment. An unknown key under
`[publish]` fails the schema check. Flags win over the table, but a flag never lowers a policy the table sets.

## Verify

`ai-rulez publish verify <dir|oci-ref>` checks, offline:

- every `SHA256SUMS` entry, and that no unlisted file is present (the plan and `SHA256SUMS` excepted);
- the manifest against the archive (every file, size and digest);
- the lock copy against the manifest;
- the plan against the manifest, so an edited plan cannot run a different command;
- the archive's determinism rules;
- with a trusted signer, the signature and the attestation;
- that the `name` and `version` in the archive's own runtime manifests match the release manifest.

An OCI reference is pulled into a temporary directory first; `verify` prints the digest it resolved to and warns when
you named a tag. Each file, and the archive's total uncompressed size, is capped at 512 MiB. Exit status: 0 verified,
1 unreadable, 2 mismatch.

## Exit codes and errors

`publish` exits 0 when done, 1 when the run could not complete, and 2 when a gate failed. `--format json` prints the
result as JSON (on `publish`, `publish emit` and `publish verify`). The `--policy*` and `--discover-org` flags apply
an organization policy to the preflight, as for `validate`; see [Organization policy](/ai-rulez/policy/).

| Code | Meaning |
| --- | --- |
| `AR9N0` | a preflight gate failed: strict validation, the lock check or plugin verification |
| `AR9N1` | the bundle holds a symlink, a path outside the project, or a name that cannot name a release file |
| `AR9N2` | the secret scan found a credential in the bundle |
| `AR9N3` | no version, a dirty tree, an untracked bundle file, or no commit |
| `AR9N4` | the upload failed: `gh` or `npm` missing, the release or version exists, or the tool exited non-zero |
| `AR9N5` | `publish verify` found a mismatch |
| `AR9N6` | invalid `[publish]` table, flag or option: unknown emitter, runtime or channel, a bad tag, scope or OCI reference |
| `AR9N7` | `require_signature` is set and the bundle is unsigned, or its signature does not verify |
| `AR9N8` | `require_approved` is set and selected content lacks a valid approval |
| `AR9N9` | an experimental emitter ran (warning) |

`ai-rulez validate --explain AR9N3` prints a code's explanation.
