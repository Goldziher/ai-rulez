# Publish

`ai-rulez publish` turns the generated [plugin bundle](plugins.md) into release artifacts: a reproducible tar.gz, a
manifest, checksums and a plan. Building is local and offline. Only `--execute --yes` sends anything, and it does so
through the platform's own CLI (`gh`, `npm`) or, for OCI, through oras-go with your Docker credentials. ai-rulez never
stores or logs a credential.

```bash
ai-rulez generate --plugin && ai-rulez lock     # commit the result first
ai-rulez publish --to github-release --dry-run   # preflight, then print artifacts and commands
ai-rulez publish --to github-release --execute --yes
ai-rulez publish verify dist
```

## Pipeline

1. Resolve: `[plugin] name` and `version` are required (set the version yourself; see AR961).
2. Preflight, in-process, stopping before anything is written: `validate --strict` (same baseline and budget handling,
   never looser than `error`), `lock --check`, `verify --plugin`, and a secret scan of every bundle file with the
   security scan's patterns. A symlink in the bundle is an error. The machine-local overlay is never loaded.
3. Policy gates from [`[publish]`](#configuration): `require_approved` (AR9N8) and `require_signature` (AR9N7). They run
   before anything is signed or built.
4. Build the dist directory, signing the archive when asked.
5. With `--execute --yes`, upload.

The tree must be clean (`--allow-dirty` waives it; the manifest then records `dirty = true`). Changes under the dist
directory are ignored.

## Dist directory

| File | Content |
| --- | --- |
| `<name>-<version>.tar.gz` | the bundle: every file `generate --plugin` writes for the published runtimes |
| `<name>-<version>.manifest.json` | files with digests, source, lock tie, bundle digest, and the approval, signature and SBOM slots ([schema](schema.md)) |
| `ai-rulez.lock` | a copy of the repository's lock without its `[[approval]]` records (reviewer emails and notes stay in the repository). Tree, content pins and output pins are unchanged, so `ai-rulez lock --check` against the copy behaves as against the original; only `[[approval]]` checks differ. With no approvals the copy is byte-identical |
| `<name>-<version>.tar.gz.sigstore.json` | with `--sign-key` or `--sign-keyless`: the signature, see [Signing](#signing) |
| `<name>-<version>.attestation.sigstore.json` | with signing: the signed statement that binds name, version and the archive, lock and SBOM digests |
| `<name>-<version>.sbom.cdx.json` | with `--sbom`: the CycloneDX SBOM of the project ([SBOM](sbom.md)) |
| `SHA256SUMS` | `sha256sum -c` format, every file except itself and the plan |
| `RELEASE_NOTES.md` | bundle, runtimes, lock tree, commit, and the [changes since the previous release](#release-notes) |
| `publish-plan.json` | artifacts with digests and the exact argv `--execute` runs; no timestamps or local paths |
| `marketplace/` | with `--marketplace`: the [pinned marketplace index](#pinned-marketplace-and-channels) |
| `emit/*` | output of `--template` files and [emitters](#emitters) |
| `npm/`, `oci/manifest.json` | the npm package directory, or the packed OCI manifest, of those targets |

A dist directory must be new, empty, or the output of an earlier publish (its artifacts are replaced; other files are
kept, and any other non-empty directory is refused). The directory is staged beside the target and installed by rename, the
plan last, so a crash never leaves a directory publish cannot reuse. A symlink at an artifact path, at a directory
component or as `--dist` itself is refused, never written or removed through.

## Determinism

Equal bundle files and mtime give a byte-identical archive across runs, operating systems and umasks: entries sorted
bytewise, regular files only, uid/gid 0 with no names, modes 0644 or 0755, GNU tar headers (no PAX), gzip level 9 with
no name or time in the header. The mtime is `SOURCE_DATE_EPOCH`, else the committer time of `HEAD`, else 0. The gzip
bytes depend on the Go toolchain that built ai-rulez (`compress/flate`), so reproducibility holds per toolchain:
build releases with one pinned Go version. `publish verify` checks digests and contents, never by recompressing.
`internal/publish/testdata/archive-digests.txt` pins the digest of a fixed archive per Go release line; after a toolchain
bump run `UPDATE_GOLDEN=1 go test ./internal/publish -run TestBuildArchive_MatchesTheGolden` and review the diff.
The source tree check lists every untracked file (`--untracked-files=all`) and git queries time out after 30 seconds.
The signature is the one artifact that differs between runs (signing is not deterministic); the archive it signs does not.

## Targets

### GitHub release

`--to github-release` uses `--tag` (default `v<version>`, which must already exist on the remote: `--verify-tag`) and
`--repo` (default `[publish.github_release] repo`, else `[plugin] repository`, else the origin remote; `OWNER/REPO` or
`HOST/OWNER/REPO`). Tag and repo are validated against an allowlist before reaching an argv. `--execute` checks
`gh release view` first and refuses an existing release (releases are immutable) unless `--force`, which runs
`gh release upload --clobber` instead. A signed release uploads the signature, an SBOM release the SBOM. gh gets
only a fixed set of variables: `PATH`, `HOME`, `USER`, `TMPDIR`, `TMP`, `TEMP`, `TZ`, `LANG`,
`LANGUAGE`, `LC_*` (non-secret), `NO_COLOR=1`, `TERM=dumb` (plus the Windows system variables), and `GH_TOKEN`,
`GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, `GH_HOST`, `GH_CONFIG_DIR`, `XDG_CONFIG_HOME`,
`XDG_STATE_HOME`, `XDG_DATA_HOME`, proxy and certificate settings. The printed `would run` line is shell-quoted; gh
missing is error `AR9N4` with an install hint.

### npm

`--to npm` writes `npm/package/` (the bundle files plus a generated `package.json`) and plans two commands with a fixed argv,
run from the dist directory:

```text
npm pack --ignore-scripts --pack-destination npm ./npm/package
npm publish ./npm/<scope>-<name>-<version>.tgz --access restricted --ignore-scripts [--registry URL] [--tag CHANNEL]
```

The paths carry a leading `./` on purpose: npm reads a bare `npm/package` as the GitHub repository `npm/package`. A test
packs the planned directory with the real npm when it is installed.

- The package is `<scope>/<plugin name>`. A scope is required (`[publish.npm] scope` or `--npm-scope`), so a package never
  lands in the public unscoped namespace by accident. The plugin name must be a valid npm name (lower case) and the version
  a semantic version.
- Access is `restricted` unless `[publish.npm] access = "public"` or `--public`. `--channel` is the dist-tag.
- `package.json` carries `files` (the bundle's top-level entries), the repository, and an `ai-rulez` key with the archive
  digest, lock tree and runtimes. A bundle that already has a `package.json` (the `opencode` runtime) is refused; drop the
  runtime with `--runtime`.
- `--execute` first runs `npm view <package>@<version> version` and refuses a version the registry has (npm versions are
  immutable), then packs and publishes. npm gets a filtered environment: the base set plus `NODE_AUTH_TOKEN`, `NPM_TOKEN`,
  `NPM_CONFIG_*` for the user config, registry and cache, proxy and certificate settings. Its output is redacted before it
  is shown.
- npm provenance (`--provenance`) is not offered: it depends on the CI environment, which a plan must not.

### OCI

`--to oci` packs the bundle as an OCI image manifest with oras-go and pushes it to `--oci-ref` (or `[publish.oci] ref`),
a repository `host/path` without a tag; the tag is the plugin version (`+` becomes `_`).

```text
artifactType  application/vnd.ai-rulez.bundle.v1
config        application/vnd.ai-rulez.manifest.v1+json   the bundle manifest
layers        application/vnd.ai-rulez.bundle.v1.tar+gzip  the archive
              application/vnd.ai-rulez.lock.v1+toml        the lock
              application/vnd.cyclonedx+json               the SBOM, with --sbom
              application/vnd.dev.sigstore.bundle.v0.3+json  the signature and the attestation, when signed
annotations   org.opencontainers.image.{created,version,source,revision}
```

The media types are custom, as the design proposed; this is the one place to change if an agent-skill convention emerges.
`created` is the archive's fixed mtime, so the manifest is reproducible: `oci/manifest.json` is written at build time and its
digest is `oci_digest` in the plan, the digest the registry reports after the push. `--execute` rebuilds the manifest from the
dist files and refuses to push unless it matches the plan. Registry credentials come from the Docker credential store
(`DOCKER_CONFIG`, `~/.docker/config.json` and its helpers), the same place `docker login` writes them, and are used only for
the registry the reference names. Plain HTTP is used for loopback registries only. Pin consumers by digest:
`host/path@sha256:...`.

### Several plugins

A `[marketplace]` with `members` or domain plugins (`from_domains`, `[[marketplace.plugins]]`) publishes one bundle per
plugin. The plugins are the entries of the generated Claude marketplace index, so the claude runtime must be generated.

```text
dist/plugins/<name>/        a complete dist directory per plugin (publish verify works on it)
dist/aggregate/             marketplace/ and emit/ for all plugins, with SHA256SUMS and a plan
```

`--only NAME` limits the plugins. `--to` runs per plugin: GitHub tags are `<name>-v<version>`, the npm package is
`<scope>/<name>`, the OCI repository is `<ref>/<name>`. `--tag` does not apply; a pinned index needs a ref from
`[publish.marketplace] channels` or `--tag`. `--runtime` applies to domain plugins; members are separate projects with their
own configuration and cannot be filtered. `publish verify dist` verifies every plugin and the aggregate checksums.

## Pinned marketplace and channels

`--marketplace` writes `marketplace/.claude-plugin/marketplace.json`: the Claude marketplace index of the bundle with every
relative `source` replaced by one pinned to the release commit. The source types and fields follow the
[Claude Code marketplace reference](https://code.claude.com/docs/en/plugins/marketplace-reference) (checked 2026-10-06):

```json
{ "source": "github", "repo": "acme/skills", "ref": "v1.4.0", "sha": "<40-hex commit>" }
{ "source": "git-subdir", "url": "https://github.com/acme/skills.git", "path": "plugins/extra", "ref": "v1.4.0", "sha": "..." }
{ "source": "url", "url": "https://git.example.com/acme/skills.git", "ref": "v1.4.0", "sha": "..." }
```

`github` is used for a plugin at the repository root on github.com, `url` for one at the root on another host, and
`git-subdir` otherwise; `sha` is the commit of the tree being published, which must be clean. Claude Code checks out `sha`,
so the index stays valid after the tag moves. Every other field of the entry (version, category, relevance) is kept.

`--channel NAME` writes `marketplace/<name>/.claude-plugin/marketplace.json` instead, with the `ref` named in
`[publish.marketplace] channels` (a channel not listed pins the release tag). Commit the index of each channel to the branch
users register as a marketplace. `--channel` is also the npm dist-tag.

## Emitters

An emitter renders the files a managed channel needs from the published plugin. It is a pure function: no network, no clock,
no file system. `--emit NAME` (repeatable) or `[[publish.emitters]]` runs it into `emit/<name>/`;
`ai-rulez publish emit NAME [--out dir]` writes only those files and no release. Output is byte-reproducible.

| Emitter | Output | Status |
| --- | --- | --- |
| `cursor-team-marketplace` | `.cursor-plugin/marketplace.json` and `plugins/<name>/` ready to commit to the repository a Cursor team marketplace imports | verified |
| `port` | one [Port](https://docs.port.io/api-reference/create-an-entity/) entity JSON per plugin and skill, plus `index.json` naming the request each file is the body of | experimental |
| `aws-agent-registry` | one `CreateRegistryRecord` request body per skill (`SKILL`, `agentSkillsDefinition` with the `SKILL.md`) and one `CUSTOM` record per plugin | experimental |
| `kiro-steering` | `.kiro/steering/*.md` from the root rules and the skills, and `distribution.json` listing the files and digests for MDM packaging | experimental |
| template | `--template FILE` or `[[publish.emitters]] name = "template"`: any text from a Go template over the manifest | verified |

- **Cursor.** Cursor documents `.cursor-plugin/marketplace.json` (`name` in kebab-case, `owner.name`, `plugins[].name` and
  `source`) at the repository root and no way to pin a git ref, so the release is the commit that carries the tree. Tests
  check the index against that documented shape. The bundle must carry the cursor runtime.
- **Experimental emitters** write formats that vendors document but publish no schema ai-rulez can test against, so they
  refuse to run without `--experimental` (`AR9N6`) and then report `AR9N9`. They are compiled in, tested by golden files, and
  never call the target system. Sources and the dates they were last read:
  - Port: the entity body `identifier`, `title`, `properties`, `relations`; the blueprint is yours (`options = { blueprint = "..." }`,
    default `agent_skill`), the property names (`kind`, `version`, `description`, `repository`, `commit`, digests) are a
    proposal you map onto it. Read 2026-10-06.
  - AWS Agent Registry: the [CreateRegistryRecord](https://docs.aws.amazon.com/agent-registry-control/latest/APIReference/API_CreateRegistryRecord.html)
    body and its name, version, description and tag constraints, which the emitter enforces. The registry id is the URI
    parameter, supplied when you create the record. Read 2026-10-06.
  - Kiro: [steering](https://kiro.dev/docs/steering/) files with the front matter `inclusion` (`always`, `fileMatch` with
    `fileMatchPattern`, `manual`, `auto` with `name` and `description`). A rule's activation chooses the mode. Kiro only reads
    `~/.kiro/steering` for global steering and `.kiro/steering` per workspace; how the files reach a machine is your MDM's job.
    Read 2026-10-06.

## Signing

`--sign-key FILE` signs the archive with a PEM key (ECDSA or ed25519; cosign keys work; the password comes from
`AI_RULEZ_SIGNING_KEY_PASSWORD` or `COSIGN_PASSWORD`, or the variable named by `--sign-key-password-env`). `--sign-keyless`
signs with a Fulcio certificate and a Rekor entry (`--sign-token-env`, `--sign-interactive`, `--fulcio-url`, `--rekor-url`;
the certificate names your identity and goes to a public log, so use a key for a private repository). `--dry-run` signs
nothing and contacts nothing: it prints `would sign` and shows the signature files in the plan as placeholders. Both go through
[`internal/signing`](signing.md), the same code as `ai-rulez sign`.

The signature is a Sigstore bundle holding a message signature over the exact bytes of the tar.gz, the form
`cosign sign-blob --bundle` writes, so `cosign verify-blob --bundle <name>.tar.gz.sigstore.json --key cosign.pub <name>.tar.gz`
verifies a release signed here. The manifest's `signature` records the file and what the bundle claims about its signer.

A signature over the archive alone does not bind the manifest, the lock copy or the SBOM, so a release is signed twice:
the archive (above) and a DSSE in-toto statement, `<name>-<version>.attestation.sigstore.json`, of predicate type
`https://github.com/Goldziher/ai-rulez/attestations/publish/v1`. Its subjects are the archive, the lock copy and the SBOM
(sha256) and its predicate carries the plugin `name`, `version`, lock tree and those digests. The manifest cannot be signed
itself (it records the signature), so the statement is what ties it to the signed files. Keyless signing makes two
certificates and two log entries. `publish verify` with a trusted signer checks the statement's signature and signer, that
it names the manifest's name and version, and that the digests equal the files in the directory; a signed release without
the statement is a mismatch (`AR9N7`). Independently of signing, verify compares the `name` and `version` in the archive's
own runtime manifests (`.claude-plugin/plugin.json` and the like) with the manifest's, so an archive relabelled as another
plugin or version is flagged.

`publish verify` reports a signed bundle as `unverified` until you name who to trust: `--key PUBLIC.pem` (repeatable), or
`--identity` and `--issuer` for a keyless signature, with `--trusted-root` or the root `ai-rulez trust update` cached. A valid
signature alone only says who signed. `--require-signature` fails an unsigned or unverified bundle; naming a trusted signer
(`--key`, `--identity`) implies it, so a release whose signature was stripped fails instead of verifying clean.

## Policy gates

```toml
[publish]
require_signature = true   # AR9N7: publish fails unless it signs the archive
require_approved  = true   # AR9N8: publish fails unless the governance policy's items are approved
```

`require_approved` reuses [approvals](approvals.md): every item the `[governance]` policy selects must have a valid
approval in `ai-rulez.lock` (missing, stale, expired, unauthorized and insufficient approvals fail it, naming the items). With no
policy selecting anything it fails, because there is nothing to approve against. When a policy is active the manifest records
`approval = { required, approved }` whether or not the gate is on; the shipped lock never carries the approval records.

## SBOM

`--sbom` runs `ai-rulez sbom` on the project and ships the CycloneDX document next to the archive. The manifest's `sbom`
names the file and its digest, the OCI artifact carries it as a layer, and verify checks it against `SHA256SUMS`. The SBOM is
deterministic (no timestamp) and leaves out the approval status, so no reviewer identity leaves the repository; it does not
disturb a reproducible release.

## Release notes

`RELEASE_NOTES.md` lists what changed in the lock since the previous release: added, changed and removed authored items
(kind, id, domain, first 12 hex of the digest) and remote pins, never content. The previous release is the closest tag
reachable from `HEAD` other than the release tag; `--since TAG` names another and is an error when that tag holds no lock.
Without a previous tag, or one without a lock, the section is omitted.

## Runtime filtering

`--runtime R` (repeatable) or `[publish] runtimes` limits the bundle to some of the plugin runtimes. The full set is
verified against the files on disk first; the subset is rendered again by the same generator from a copy of the
configuration, so shared files are identical. A runtime that is not in `[plugin] runtimes` is an error (`AR9N6`).
`--runtime` wins over the table.

## Configuration

```toml
[publish]
runtimes = ["claude", "cursor"]   # default: the [plugin] runtimes
require_signature = false
require_approved = false
allow_dirty = false

[publish.github_release]
repo = "acme/skills"              # default: [plugin] repository, else origin

[publish.oci]
ref = "ghcr.io/acme/skills/conventions"   # no tag; the tag is the version

[publish.npm]
scope = "@acme"
access = "restricted"             # or "public"
registry = "https://npm.example.com"

[publish.marketplace.channels]
canary = "main"                   # channel -> the git ref its index pins

[[publish.emitters]]
name = "port"
options = { blueprint = "agent_skill" }

[[publish.emitters]]
name = "template"
template = "tools/port-entity.tmpl"   # inside the project, never through a symlink
output = "port-entity.json"
```

There are no credential keys: forges and registries authenticate through their own CLIs and environment, and an unknown key
under `[publish]` fails the schema check. Flags win over the table, and a flag never lowers a policy the table sets.

## Verify

`publish verify <dir|oci-ref>` checks `SHA256SUMS` against the files (a duplicate entry is a mismatch), flags every file in the
directory that `SHA256SUMS` does not list (the plan and `SHA256SUMS` itself excepted), checks the manifest against the
archive (every file, size and digest), the lock copy against `lock.file_digest`, and its `tree` and `version` against
the manifest's `lock.tree` and `lock.version`, and the archive against the determinism rules. The signature and SBOM files
the manifest names must be listed. The plan is checked too: each artifact's path, size and digest against the file and
`SHA256SUMS`, and its commands against what `--execute` would build for the manifest (the `gh release create` argv, the npm
pack and publish argv, or the OCI reference and a manifest rebuilt from the dist files), so an edited plan cannot smuggle in a
command. Files and the archive's total uncompressed size are capped at 512 MiB. An OCI reference is pulled into a temporary
directory first. Exit 0 verified, 2 mismatch, 1 unreadable directory.

## Testing without a registry

Tests use fake `gh` and `npm` runners, an in-memory OCI registry (go-containerregistry's `registry` package behind
`httptest`), and golden files for every emitter. A live round trip against `registry:2` runs with
`AI_RULEZ_LIVE_PUBLISH=1 go test ./internal/publish/oci -run Live` and needs Docker. Nothing in the repository publishes
anything.
