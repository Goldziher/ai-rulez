# Signing

A digest proves that bytes did not change. A signature says who produced them. `ai-rulez sign --lock` signs the
[lock](lockfile.md) into a [Sigstore](https://www.sigstore.dev) bundle, and `ai-rulez verify --attestation` checks it
offline against a policy that names who may sign. The same machinery signs [plugin bundles, published skills and SBOM
files](#bundles-skills-and-sboms), optionally with [SLSA provenance](#slsa-provenance), [KMS keys](#kms-keys) and
[k-of-n signers](#thresholds), and can [gate served skills](#served-skills-and-publisher-signed-skills). Signing is
opt-in; nothing is enforced until `[signing] require` says so.

A valid signature means "produced by X". It does not mean the content is safe: a malicious but validly signed skill
still needs [approval](approvals.md) and a [scan](strict-validation.md). Rollback to an older validly signed lock is
bounded by `max_age` and detected per machine, not prevented (see [Freshness and rollback](#freshness-and-rollback)).

## What is signed

The bundle holds a DSSE envelope (`application/vnd.in-toto+json`) over an in-toto statement v1. Its subject is the
**lock subject** that [`lock --subject`](lockfile.md#signing-the-lock) prints, not the bytes of `ai-rulez.lock`, so a
signature survives line-ending changes and TOML re-ordering, and a pin edited by hand changes the subject.

```json
{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [{ "name": "ai-rulez.lock", "digest": { "sha256": "<lock subject hex>" } }],
  "predicateType": "https://github.com/Goldziher/ai-rulez/attestations/lock/v1",
  "predicate": {
    "hash_version": 1,
    "tree": "sha256:...",
    "approvals_digest": "",
    "scope": "all",
    "outputs_pinned": true,
    "ai_rulez_version": "5.0.0",
    "repository": "https://github.com/example-org/ai-config",
    "ref": "refs/heads/main",
    "issued_at": "2026-10-06T12:00:00Z"
  }
}
```

`repository` and `ref` come from the GitHub Actions variables or the git origin (credentials removed) and only
identify the project for the rollback state; they are claims, not proof. `--embed-items` adds the pinned item ids and
digests (`items`) so a reviewer can see what changed between two signed states; it is off by default because ids can be
sensitive in a private repository. `approvals_digest` is the digest of the `[[approval]]` and `[[deny]]` records, and
empty when the lock has none (see [Lock file](lockfile.md#signing-the-lock)); the subject covers them, so re-sign after
the last `approve`.

The bundle is written next to the lock: `.ai-rulez/ai-rulez.lock.sigstore.json` (`[signing] attestation` or
`sign --output` change it). Commit it.

## Sign

```console
$ ai-rulez sign --lock --key cosign.key
Signed ai-rulez.lock signer=key sha256:91be... subject=sha256:60e6... bundle=.ai-rulez/ai-rulez.lock.sigstore.json
```

Run it after the final `ai-rulez lock`: any change to the lock invalidates the signature.

| Mode | Command | Network | Needs |
| --- | --- | --- | --- |
| Key | `sign --lock --key <file>` | none (`--tlog` adds a Rekor entry) | an ECDSA P-256/P-384/P-521 or ed25519 PEM key: PKCS#8, or a cosign key |
| KMS | `sign --lock --key awskms:///alias/release` | the KMS (`--tlog` adds a Rekor entry) | a [KMS key](#kms-keys) and the provider's credentials |
| Keyless | `sign --lock --keyless` | Fulcio and Rekor | an OIDC token |

A key's password is read from `AI_RULEZ_SIGNING_KEY_PASSWORD`, then `COSIGN_PASSWORD` (`--key-password-env` names
another variable); it is never a flag. Keys generated with `cosign generate-key-pair` work as they are.

Keyless signing exchanges an OIDC token for a short-lived Fulcio certificate and records the signature in Rekor. The
token comes from the variable `--identity-token-env` names, else the GitHub Actions runtime (`permissions: id-token:
write`), else `--interactive` opens a browser. **Your OIDC identity, the certificate and the subject digest go to the
transparency log, which is public** unless `--rekor-url` and `--fulcio-url` point at a private Sigstore deployment.
`sign` says so before it starts. Use key mode for a private repository whose identities you do not want logged.

```yaml
# GitHub Actions, keyless
permissions: { id-token: write, contents: read }
steps:
  - run: ai-rulez lock --check
  - run: ai-rulez sign --lock --keyless
  - run: ai-rulez verify --attestation
```

## Policy

`[signing]` in `.ai-rulez/config.toml` says who may sign and how fresh a signature must be:

```toml
[signing]
require = ["lock"]              # lock | served | skill; lock --check and generate --locked fail without a valid attestation
max_age = "180d"                # signature older than this fails (AR723)
tlog = "required"               # required | optional | off
trusted_root = "keys/trusted_root.json"   # keyless verification; inside the project
min_hash_version = 1

# one trusted keyless signer (shorthand)
identity = "https://github.com/example-org/ai-config/.github/workflows/release.yml@refs/heads/main"
issuer = "https://token.actions.githubusercontent.com"
# or one trusted key (shorthand)
# key_file = "keys/release.pub"

# the full form: repeatable, with validity windows
[[signing.trust]]
subject = "lock"                # lock (default) | bundle | skill | sbom
identity_regexp = "^https://github\\.com/example-org/[^/]+/\\.github/workflows/release\\.yml@refs/heads/main$"
issuer = "https://token.actions.githubusercontent.com"
valid_from = "2026-01-01"
valid_until = "2027-01-01"      # forces a reviewed renewal

[[signing.trust]]
key_file = "keys/release.pub"
```

- `identity` and `issuer` are matched **exactly** against the certificate's subject alternative name and OIDC issuer.
  `identity_regexp` must be anchored with `^` and `$` and is matched against the whole identity; an unanchored pattern
  is rejected at load time (`AR722`). An identity entry always needs an `issuer`.
- A key entry trusts a PEM public key (`key_file`), matched by its SHA-256 fingerprint.
- `subject` scopes an entry to what the signer may vouch for: the `lock` (the default), a plugin `bundle`, a published
  `skill`, an `sbom` or an `approval` (who may sign an [approval](approvals.md#signed-approvals)). A release key trusted
  for the lock does not vouch for a skill or an approval. `source` (skill entries only)
  narrows a publisher to one `[[skill_sources]]` or `[[installed_skills]]` name, so a signer trusted for one source
  cannot vouch for another. The `identity`/`key_file` shorthands and `require` are about the lock only.
- `valid_from` and `valid_until` bound the **signing time** (the log time) the entry accepts, so an expiry is a reviewed
  event. A date bound is inclusive and UTC.
- `tlog` defaults to `required` when a certificate identity is trusted and to `off` when only keys are. `off` works with
  keys only (a certificate is meaningful only at the time a log recorded it). `optional` accepts a log entry or a signed
  timestamp, and for a key signature neither. A key bundle with a log entry and no trusted root is accepted on its
  signature alone: the entry is not checked, so the signing time stays unknown (`weak`). `required` fails it with
  `AR725`.
- `key_file`, `trusted_root` and `attestation` are project-relative and must stay inside it; a committed config cannot
  point at files elsewhere on the machine. A symlink out of the project is refused.
- A machine-local config overlay cannot add a signer: `[signing]` there is ignored with a warning.

Verification without any trusted signer for the subject is an error (exit `1`), not "accept any valid signature": the policy, or
`--public-key` or `--identity` with `--issuer`, must say who may sign.

The repository can edit its own `[signing]` table, so a pull request can weaken it or name its own signer.
`verify --attestation` prints a warning when the trusted signers come from the repository alone; `--public-key` or
`--identity` with `--issuer` from CI configuration outside the repository removes it. Protect the table with code review
and branch protection, or enforce it from outside the repository with an [organization policy](policy.md).

## Verify

```console
$ ai-rulez verify --attestation
OK  lock  signer=https://github.com/example-org/ai-config/.github/workflows/release.yml@refs/heads/main
    issuer=https://token.actions.githubusercontent.com  logged=2026-10-04T09:12:03Z  age=2d  hash_version=1
$ echo $?
0
```

Verification is offline: it reads the bundle, the lock and a trusted root, and sends nothing. It checks, in order, the
signature, the certificate chain and log proof (`AR721`, `AR725`, `AR726`), that the signed digest is the lock's
recomputed subject and `hash_version` (`AR724`), the signer against `[signing]` (`AR722`), `max_age` and a signing
time in the future (`AR723`) and rollback (`AR727`). A lock edited after signing by an untrusted signer therefore
reports `AR724`, not `AR722`. `--format json` prints `schema/verify-attestation.schema.json`. Exit codes: `0` verified, `1` the check
could not run (no trusted signer, no trusted root for a certificate bundle, unreadable lock), `2` verification failed.

The same checks run, without `verify`, wherever `[signing] require = ["lock"]` applies: `lock --check`,
`generate --locked` (and `--frozen`) and `validate --strict` report the same `AR720` to `AR727` codes.

### Trusted root

A keyless certificate is checked against a Sigstore trusted root: the Fulcio and Rekor roots and, when present, the CT
log keys. Verification looks for it in this order: `--trusted-root <file>`, `[signing] trusted_root` (inside the
project), then the cache that `ai-rulez trust update` fills (`~/.cache/ai-rulez/sigstore/trusted_root.json`).
`trust update` fetches the public-good root over TUF and is **the only command that touches the network for
verification**. For a private Sigstore deployment pass its root file. Key-signed bundles need no root unless they carry
a log entry.

A trusted root is itself a trust anchor. A root committed to the repository (`trusted_root`) is as trustworthy as
the repository's review process; for a stronger guarantee supply `--trusted-root` from CI configuration outside the
repository.

### Freshness and rollback

- **`max_age`** compares the time a transparency log or timestamp authority saw the signature with now. A key bundle
  without a log has no such time, so freshness falls back to the `issued_at` the signer wrote into the statement. The
  signer controls that claim, so such a result is reported `weak` and proves nothing about time. Use a log entry
  (`sign --tlog`, or keyless) when freshness matters.
- A signing time more than five minutes ahead of the local clock fails with `AR723` ("in the future"), for a log time
  and for the signer's own `issued_at` claim alike: a future claim would otherwise pass every `max_age` and pin the
  rollback mark ahead.
- **Rollback** is detected with a per-user high-water mark: the latest signing time verified for each signer in each
  project. The project is the `repository` claim plus the config directory's path inside its checkout (so the roots of a
  monorepo are separate), or the absolute config directory when there is no claim or no checkout. The claim is the
  signer's, so it never selects a mark by itself: a signer cannot touch another signer's mark. An older attestation than
  one this machine already verified fails with `AR727`, and the message names the state file to delete if the newer
  attestation was wrong. The state is a file outside the repository
  (`$XDG_STATE_HOME/ai-rulez/signing-state.json`, else `~/.local/state/ai-rulez/`) authenticated with an HMAC under a
  per-user secret (`$XDG_CONFIG_HOME/ai-rulez/signing-state.key`, the pattern the LLM cache uses), so a checkout cannot
  plant or reset it. A file that fails its HMAC is discarded with a warning. A fresh machine or a CI runner starts empty,
  so CI relies on `max_age` and branch protection. An attacker who can present an old bundle to a fresh machine within
  `max_age` succeeds: set `max_age` to your release cadence. `verify --no-state` skips the state.
- `lock --check`, `generate --locked` and `validate --strict` read the state but never write it; only
  `verify --attestation` advances it.

## Bundles, skills and SBOMs

```console
$ ai-rulez sign --bundle dist/acme-plugin --key release.key
Signed bundle path=dist/acme-plugin signer=key sha256:91be... subject=sha256:4f1a... bundle=dist/acme-plugin/.ai-rulez.sigstore.json
$ ai-rulez verify --bundle dist/acme-plugin --public-key release.pub
OK  bundle  signer=sha256:91be...
    issuer=none (key)  logged=no  age=3s
    digest=sha256:4f1a...
```

| Subject | Command | Statement subject | Sidecar |
| --- | --- | --- | --- |
| Plugin bundle | `sign --bundle <dir>` | tree digest of every file in the directory (`ai-rulez/plugin-bundle/v1`) | `<dir>/.ai-rulez.sigstore.json` |
| Published skill | `sign --skill <dir>` | tree digest of the skill directory (`ai-rulez/published-skill/v1`) | `<dir>/.ai-rulez.sigstore.json` |
| SBOM | `sign --sbom <file>` | sha256 of the file's bytes | `<file>.sigstore.json` |

The directory digest uses the same scheme as the lock: sorted paths, text files line-ending normalized, the executable
bit part of the digest. Adding, removing or editing a file, or flipping the executable bit, invalidates the signature
(`AR724`). The attestation files (`.ai-rulez.sigstore.json`, numbered co-signatures and the provenance file) at the root
of the directory are not part of what they sign; the same name deeper in the tree is content. A symlink or any irregular
file in the directory is refused: a signature over "where the link pointed" would not cover what an agent reads through
it. The root `.git` directory is skipped; a `.git` directory or file anywhere deeper is refused, since an agent could
read it and no signature would cover it. The directory's own name is not part of the match, so a bundle checked out under another name verifies.

An SBOM is signed by its bytes, whatever its format (`ai-rulez sbom`, SPDX, CycloneDX): the signature says who produced
that exact file, not that it is complete. The predicate types are
`https://github.com/Goldziher/ai-rulez/attestations/{bundle,skill,sbom}/v1`, each carrying the digest, the ai-rulez
version, the `repository` and `ref` claims and `issued_at`.

`verify --bundle <dir>`, `--skill <dir>` and `--sbom <file>` imply `--attestation` and use the same checks and exit
codes as the lock (`--attestation-file` names another sidecar). They read `[signing]` from the project when there is
one; a consumer with no project passes `--public-key`, or `--identity` with `--issuer`, and no config is needed.
Rollback marks are kept per signer, project and artifact name.

## Thresholds

`[signing.thresholds]` asks for k distinct trusted signers of a subject:

```toml
[signing.thresholds]
lock = 2

[[signing.trust]]
key_file = "keys/alice.pub"

[[signing.trust]]
key_file = "keys/bob.pub"
```

Each signer writes their own bundle: `sign --lock --key alice.key`, then `sign --lock --key bob.key --append`, which
writes `ai-rulez.lock.2.sigstore.json` next to `ai-rulez.lock.sigstore.json` (numbered files are found by name; at most
16). Verification checks every file and counts distinct accepted signers: a key by fingerprint, a certificate by
identity and issuer, so two bundles by one signer count once. A file that fails is ignored while enough others verify;
with none valid the first failure is reported, and with some but too few the result is `AR728`. Each subject has its own
threshold (`lock`, `bundle`, `skill`, `sbom`; default 1), and a threshold of k needs at least k trust entries for the
subject, or an `identity_regexp`, which may match many identities. It applies to `lock --check`, `generate --locked`,
`validate --strict`, `verify --bundle`, `--skill` and `--sbom` and the served-skill gates. Provenance is signed by the
builder alone and is not subject to the bundle's threshold.

## SLSA provenance

`sign --bundle <dir> --provenance` also writes `.ai-rulez.provenance.sigstore.json`: an in-toto statement with the
[SLSA provenance v1](https://slsa.dev/spec/v1.0/provenance) predicate (`https://slsa.dev/provenance/v1`) whose subject is
the bundle's tree digest. It records the build type
(`https://github.com/Goldziher/ai-rulez/buildtypes/plugin-bundle/v1`), the repository, ref and commit (from the GitHub
Actions variables, else the checkout), the invocation (the CI run URL) and a builder id: `--builder-id`, else the
workflow reference `GITHUB_WORKFLOW_REF` (so the id names the workflow file and ref that ran), else
`https://github.com/Goldziher/ai-rulez/builders/cli/v1`.

ai-rulez does not run a hermetic build, so this is the signer's own account of where the bundle was generated, not a
platform's attestation: at most SLSA build level 1. Its value is tying a bundle to a repository, commit and workflow in
a form standard tools read. Provenance written by a CI builder such as `slsa-github-generator` verifies the same way,
whatever its build type.

```toml
[signing]
require_provenance = true      # verify --bundle demands it (or pass --require-provenance)
builders = ["https://github.com/example-org/plugin/.github/workflows/release.yml@refs/heads/main"]
```

A provenance file that exists is always verified, required or not. It is judged like the bundle attestation (the
`bundle` trust entries, freshness, rollback), must name the bundle's digest (`AR724`), and when `builders` is set its
builder id must be in the list. A missing file under `require_provenance`, a predicate that is not SLSA provenance v1
and a builder outside the list are `AR729`.

## KMS keys

`--key` accepts a cosign key URI: `awskms:///alias/release`,
`gcpkms://projects/p/locations/l/keyRings/r/cryptoKeys/k`, `azurekms://vault.vault.azure.net/key` or
`hashivault://key`, through sigstore's KMS providers (the ones cosign uses; a `sigstore-kms-<name>` plugin binary adds
others). The private key never leaves the KMS: ai-rulez sends a digest and gets a signature. Credentials come from the
provider's usual environment (`AWS_*`, `GOOGLE_APPLICATION_CREDENTIALS`, `AZURE_*`, `VAULT_*`), never from a flag, and a
query string in the URI is removed from error messages. Use ECDSA P-256 (the cosign default) or ed25519 keys; RSA keys
are not supported.

The bundle names the key by fingerprint, like any key, so verification stays offline. Export the public key once and
trust it by file: `ai-rulez sign --lock --key awskms:///alias/release --public-key-out keys/release.pub` (or
`cosign public-key --key awskms:///alias/release`), then `key_file = "keys/release.pub"`.

Binary size: the four providers add about 14 MB to an unstripped build (59.3 MB to 73.6 MB) and about 10 MB to a
stripped release build (42.2 MB to 52.1 MB). That is under the 15 MB budget, so they are in every build rather than
behind a build tag. Offline tests use sigstore's `fakekms://` provider; a live test runs only with `AI_RULEZ_LIVE_KMS=1`
and `AI_RULEZ_LIVE_KMS_KEY=<key URI>`.

## Served skills and publisher-signed skills

`ai-rulez mcp --serve-skills` refuses skills the policy does not vouch for. It uses the refusal path of the security
scan and the lock, so `load_skill` says why:

```toml
[signing]
require = ["served", "skill"]

[[signing.trust]]              # the lock's signers, for "served"
key_file = "keys/release.pub"

[[signing.trust]]              # a publisher, for one skill source
subject = "skill"
source = "shared"
key_file = "keys/publisher.pub"
```

- **`served`** (consumer-signed, the recommended default): the lock must carry a valid attestation (the lock's trust
  entries and thresholds; otherwise every skill is refused with `AR720` to `AR728`), and it implies the lock
  enforcement of `[lock] enforce`: a skill the signed lock does not pin with exactly its served digest is refused with
  `AR995`. The consumer's release workflow signs the lock, which pins the served digests.
- **`skill`** (publisher-signed): every skill that came from a `[[skill_sources]]` or `[[installed_skills]]` entry must
  carry `.ai-rulez.sigstore.json` in its directory (`ai-rulez sign --skill`), signed by a `subject = "skill"` trust
  entry for that source. A skill authored in the project is not remote and is not checked. A remote skill from another
  origin (an include) has nowhere to carry an attestation and is refused; cover includes with `served`. The server reads
  the rollback state per signer and skill and never writes it.

The server recomputes the skill's digest from the files on disk and, for a skill source, compares each served file with
the bytes that were signed, so a file that changes between being read and being verified, or a file the signature does
not cover, is `AR724`; an installed skill, which is rendered, is checked for coverage only (every served file must be a
signed file). Only the `name:` line of `SKILL.md` may differ, because a source serves it with the served name.
Verdicts are computed when the catalog is built and again on every reload, which swaps the catalog whole. Attestation
files are never served, scanned or part of the served digest.

`generate` applies `skill` to the installed skills it writes into harness trees (static delivery; a skill with
`delivery = "served"` is gated when it is served). Each needs a valid `.ai-rulez.sigstore.json` whose digest covers the
directory on disk; otherwise nothing is written and the error lists each skill with its `AR72x` code. The check runs
before the content scan (`scan_imports`), so an unsigned or tampered skill is refused as such rather than scanned as
trusted. Dry runs and plugin bundles skip it, like the scan.

## Cosign interoperability

`ai-rulez` writes a standard Sigstore bundle, so cosign can verify it. Because the payload is an in-toto attestation,
use `verify-blob-attestation` with the subject digest `lock --subject` prints:

```bash
cosign verify-blob-attestation --bundle .ai-rulez/ai-rulez.lock.sigstore.json \
  --key cosign.pub --type https://github.com/Goldziher/ai-rulez/attestations/lock/v1 \
  --digest "$(ai-rulez lock --subject --format json | jq -r .subject | cut -d: -f2)" --digestAlg sha256 \
  --insecure-ignore-tlog=true   # only for a key bundle signed without --tlog
```

(`cosign verify-blob` checks a blob signature, not an attestation, so it does not apply to this bundle.)

The other direction also works: `verify --attestation` accepts the message-signature bundle that the
[`cosign sign-blob` recipe](lockfile.md#sign-in-the-release-workflow) writes over `lock-subject.json`, recomputing that
file from the lock, and the bundle `cosign attest-blob --type <predicate type> --hash <digest> --predicate <file>`
writes (in-toto statement v0.1 is accepted besides v1; the predicate carries the fields shown in the example above). Such a bundle names no repository and has no `issued_at`, so without a log entry it has no signing time: `max_age`
cannot be met and no rollback mark applies.

## Codes

| Code | Name | Meaning |
| --- | --- | --- |
| `AR720` | `signature-missing` | `require` asks for a signed lock and no attestation exists |
| `AR721` | `signature-invalid` | bad bundle, envelope, signature, certificate chain or log proof |
| `AR722` | `signer-not-trusted` | identity, issuer or key matches no trust entry or is outside its validity window; an unanchored `identity_regexp` |
| `AR723` | `signature-stale` | older than `max_age`, no time to measure it, or a signing time in the future |
| `AR724` | `attestation-subject-mismatch` | the signed digest or `hash_version` differs from the lock (it changed after signing), or is below `min_hash_version` |
| `AR725` | `trusted-root-unavailable` | a certificate bundle and no trusted root |
| `AR726` | `tlog-proof-missing` | `tlog = "required"` and the bundle has no log entry |
| `AR727` | `signature-rollback` | older than the newest attestation this machine verified |
| `AR728` | `signature-threshold-not-met` | fewer distinct trusted signers than `[signing.thresholds]` asks for |
| `AR729` | `provenance-invalid` | SLSA provenance missing under `require_provenance`, not SLSA v1, or from a builder outside `builders` |

See [Strict validation](strict-validation.md#ar720-signature-missing) for each code.

## Reusing the signing API

The `internal/signing` package signs and verifies any in-toto statement; the lock is its first user. Its package
documentation lists the calls: `NewStatement`, `SignStatement` with a `KeySigner` or `KeylessSigner`, `Verifier.Verify`,
`TrustSet.Check`, `CheckFresh` and the HMAC-protected `State`. Features that sign something else (approvals, an SBOM, a
policy, a bundle) pick a predicate type URI and reuse them.

## Not done yet

- `verify --self` for ai-rulez's own releases.

Live tests run only with `AI_RULEZ_LIVE_SIGSTORE=1` (keyless) or `AI_RULEZ_LIVE_KMS=1` (a cloud KMS key) and are never
part of the default test run.
