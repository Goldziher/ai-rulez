---
title: "Signing"
description: "Sign the lock file, SBOM and plugin bundles with Sigstore or a key, and verify signatures against trusted identities."
---

A digest proves that bytes did not change; a signature says who produced them. `ai-rulez sign --lock` signs the
[lock](/ai-rulez/lockfile/) into a [Sigstore](https://www.sigstore.dev) bundle, and `ai-rulez verify --attestation` checks it
offline against a policy that names who may sign. The same commands sign [plugin bundles, published skills and SBOM
files](#bundles-skills-and-sboms), support [KMS keys](#kms-keys), [k-of-n signers](#thresholds) and
[SLSA provenance](#slsa-provenance), and can [gate served skills](#served-skills-and-publisher-signed-skills).

Signing is opt-in; nothing is enforced until `[signing] require` says so. A valid signature means "produced by this
identity", not "safe": a validly signed skill still needs [approval](/ai-rulez/approvals/) and the
[security checks](/ai-rulez/strict-validation/#security-checks).

## Example

Sign with a key, offline:

```bash
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out release.key   # or: cosign generate-key-pair
ai-rulez lock
ai-rulez sign --lock --key release.key --public-key-out .ai-rulez/keys/release.pub
```

```text
INFO  Signed ai-rulez.lock signer=key sha256:c49716c6aeb4b89ba6176d8b7e7951b9a8801e92498d9104b4e4c58f8d8e0ac9 subject=sha256:f4a4a2b1382eab3578b7f76eb4495651fc208f6161aaa83ef94618f64ae1e0a2 bundle=/work/acme/.ai-rulez/ai-rulez.lock.sigstore.json
```

Name the trusted key and require the signature:

```toml
# .ai-rulez/config.toml (excerpt)
[signing]
require  = ["lock"]
key_file = ".ai-rulez/keys/release.pub"
```

Commit `.ai-rulez/ai-rulez.lock.sigstore.json` and the public key. Verify:

```console
$ ai-rulez verify --attestation
warning: the trusted signers come from this repository's own [signing] configuration: whoever can change the repository can change who may sign. Pass --public-key or --identity with --issuer from CI configuration outside the repository to pin them
OK  lock  signer=sha256:c49716c6aeb4b89ba6176d8b7e7951b9a8801e92498d9104b4e4c58f8d8e0ac9
    issuer=none (key)  logged=no  age=7s  hash_version=1
    no transparency log or timestamp: the signing time is the signer's own claim (weak freshness)
```

Any later change to the lock, including a new approval, invalidates the signature. `lock --check`, `generate --locked`
and `validate` then fail:

```text
ai-rulez.lock does not carry the signature [signing] require asks for:
  AR724 attestation-subject-mismatch: ai-rulez.lock changed since it was signed (its subject is sha256:1b13f187d8e956cb156b25f9a0610264879b584b55b93f8929433d41478e6dad)
```

Re-sign after the final `ai-rulez lock` and `ai-rulez approve`.

## What is signed

The bundle holds a DSSE envelope over an in-toto statement v1. Its subject is the **lock subject** that
[`lock --subject`](/ai-rulez/lockfile/#signing-the-lock) prints, not the bytes of `ai-rulez.lock`, so a signature survives
line-ending changes and TOML re-ordering, while a hand-edited pin or approval changes the subject.

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
    "ai_rulez_version": "5.10.0",
    "repository": "https://github.com/acme/ai-config",
    "ref": "refs/heads/main",
    "issued_at": "2026-11-05T12:00:00Z"
  }
}
```

`repository` and `ref` come from the GitHub Actions variables or the git origin (credentials removed). They identify
the project for rollback detection; they are claims, not proof. `sign --embed-items` adds the pinned item ids and
digests (`items`), so a reviewer can compare two signed states; it is off by default because ids can be sensitive.

The bundle is written to `.ai-rulez/ai-rulez.lock.sigstore.json`; `[signing] attestation` or `sign --output` change it.

## Sign

| Mode | Command | Network | Needs |
| --- | --- | --- | --- |
| Key | `sign --lock --key <file>` | none; `--tlog` adds a Rekor entry | an ECDSA P-256/P-384/P-521 or ed25519 PEM key (PKCS#8 or a cosign key) |
| KMS | `sign --lock --key awskms:///alias/release` | the KMS; `--tlog` adds a Rekor entry | a [KMS key](#kms-keys) and the provider's credentials |
| Keyless | `sign --lock --keyless` | Fulcio and Rekor | an OIDC token |

A key's password is read from `AI_RULEZ_SIGNING_KEY_PASSWORD`, then `COSIGN_PASSWORD` (`--key-password-env` names
another variable); it is never a flag. `--public-key-out <file>` writes the key's public half for `key_file`.

Keyless signing exchanges an OIDC token for a short-lived Fulcio certificate and records the signature in Rekor. The
token comes from the variable `--identity-token-env` names, else the GitHub Actions runtime (`permissions: id-token:
write`), else `--interactive` opens a browser. **Your OIDC identity, the certificate and the subject digest go to the
public transparency log** unless `--rekor-url` and `--fulcio-url` point at a private Sigstore deployment. Use key mode
when you do not want identities logged.

```yaml
# GitHub Actions, keyless
permissions: { id-token: write, contents: read }
steps:
  - run: ai-rulez lock --check
  - run: ai-rulez sign --lock --keyless
  - run: ai-rulez verify --attestation
```

`sign --format json` prints `schema/sign-report.schema.json`.

## Policy

`[signing]` in `.ai-rulez/config.toml` says what must be signed, who may sign and how fresh a signature must be:

```toml
[signing]
require = ["lock"]
max_age = "180d"
tlog = "required"
trusted_root = ".ai-rulez/keys/trusted_root.json"
min_hash_version = 1

# shorthand: one trusted keyless signer for the lock
identity = "https://github.com/acme/ai-config/.github/workflows/release.yml@refs/heads/main"
issuer = "https://token.actions.githubusercontent.com"
# or one trusted key for the lock
# key_file = ".ai-rulez/keys/release.pub"

# full form: repeatable, scoped, with validity windows
[[signing.trust]]
subject = "lock"
identity_regexp = "^https://github\\.com/acme/[^/]+/\\.github/workflows/release\\.yml@refs/heads/main$"
issuer = "https://token.actions.githubusercontent.com"
valid_from = "2026-01-01"
valid_until = "2027-01-01"

[[signing.trust]]
key_file = ".ai-rulez/keys/release.pub"
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `require` | list | `[]` | Subjects that must carry a valid attestation: `lock` (`lock --check`, `generate --locked` and `validate` fail without one), `served` and `skill` ([served skills](#served-skills-and-publisher-signed-skills)) |
| `max_age` | string | none | Oldest acceptable signature (`180d`, `720h`); older is `AR723` |
| `tlog` | string | `required` with a certificate identity, `off` with keys only | `required`, `optional` (a log entry or a signed timestamp) or `off` (keys only) |
| `trusted_root` | string | the `trust update` cache | Sigstore trusted root for keyless verification |
| `min_hash_version` | int | none | Reject attestations of an older hash scheme (`AR724`) |
| `attestation` | string | `ai-rulez.lock.sigstore.json` | Lock bundle path, relative to the configuration directory |
| `identity`, `issuer` | string | none | Shorthand: one trusted keyless signer for the lock |
| `key_file` | string | none | Shorthand: one trusted public key for the lock |
| `[[signing.trust]]` | table list | none | Full trust entries (below) |
| `[signing.thresholds]` | table | `1` per subject | [k-of-n signers](#thresholds) |
| `require_provenance`, `builders` | bool, list | `false`, any | [SLSA provenance](#slsa-provenance) requirements |

| `[[signing.trust]]` key | Meaning |
| --- | --- |
| `subject` | What the signer may vouch for: `lock` (default), `bundle`, `skill`, `sbom` or `approval` ([signed approvals](/ai-rulez/approvals/#signed-approvals)). A key trusted for the lock does not vouch for a skill |
| `identity`, `identity_regexp`, `issuer` | A keyless signer. `identity` and `issuer` are matched exactly; `identity_regexp` is matched against the whole identity and must be anchored with `^` and `$` (otherwise `AR722`). An identity always needs an `issuer` |
| `key_file` | A PEM public key, matched by its SHA-256 fingerprint |
| `reviewer` | The person a key belongs to (`alice@example.org`, `github:alice`), for thresholds and [self-approval](/ai-rulez/approvals/#self-approval) checks |
| `source` | Skill entries only: limit a publisher to one `[[skill_sources]]` or `[[installed_skills]]` name |
| `valid_from`, `valid_until` | Inclusive UTC bounds on the signing time the entry accepts, so an expiry is a reviewed event |

`key_file`, `trusted_root` and `attestation` must stay inside the project; `key_file` and `trusted_root` are relative
to the project root. A symlink out of the project is refused. `[signing]` in the machine-local overlay is ignored with a
warning, so it cannot add a signer.

Verification without a trusted signer for the subject is an error (exit `1`), never "accept any valid signature".

The repository can edit its own `[signing]` table, so a pull request can weaken it or name its own signer.
`verify --attestation` warns when the signers come from the repository alone. `--public-key` and `--identity` with
`--issuer` **add** signers; they do not replace the repository's, so they protect you only when the repository's table
is empty. To stop a pull request from naming its own signer, restrict the signers with an
[organization policy](/ai-rulez/policy/) (`[[signing.trust]]` in the policy, or `allow_repo_identities = false`), or fail CI
when the `[signing]` table changes.

## Verify

```console
$ ai-rulez verify --attestation --public-key ci/release.pub
OK  lock  signer=sha256:c49716c6aeb4b89ba6176d8b7e7951b9a8801e92498d9104b4e4c58f8d8e0ac9
    issuer=none (key)  logged=no  age=0s  hash_version=1
    no transparency log or timestamp: the signing time is the signer's own claim (weak freshness)
```

A keyless signature reports its identity and log time instead:

```text
OK  lock  signer=https://github.com/acme/ai-config/.github/workflows/release.yml@refs/heads/main
    issuer=https://token.actions.githubusercontent.com  logged=2026-11-04T09:12:03Z  age=1d  hash_version=1
```

Verification is offline: it reads the bundle, the lock and a trusted root, and sends nothing. It checks the signature,
certificate chain and log proof (`AR721`, `AR725`, `AR726`), that the signed digest is the lock's recomputed subject
(`AR724`), the signer against the policy (`AR722`), `max_age` and future timestamps (`AR723`), and rollback (`AR727`).

| Exit code | Meaning |
| --- | --- |
| `0` | verified |
| `1` | could not check: no trusted signer, no trusted root for a certificate bundle, unreadable lock |
| `2` | verification failed (`FAIL  lock  AR724 …`) |

`--format json` prints `schema/verify-attestation.schema.json`. With `[signing] require = ["lock"]`, `lock --check`,
`generate --locked` (and `--frozen`) and `validate` run the same checks and report the same `AR720` to `AR728` codes.

### Trusted root

A keyless certificate is checked against a Sigstore trusted root (the Fulcio and Rekor roots and CT log keys). It is
read from `--trusted-root <file>`, then `[signing] trusted_root`, then the cache `ai-rulez trust update` fills
(`~/.cache/ai-rulez/sigstore/trusted_root.json`). `trust update` fetches the public-good root over TUF and is the only
verification step that uses the network. For a private Sigstore deployment pass its root file. Key-signed bundles need
no root unless they carry a log entry.

A root committed to the repository is as trustworthy as the repository's review process; for a stronger guarantee pass
`--trusted-root` from CI configuration.

### Freshness and rollback

- **`max_age`** compares the time a transparency log or timestamp authority saw the signature with now. A key bundle
  without a log entry has only the `issued_at` the signer wrote, reported as `weak`: use `sign --tlog` or keyless when
  freshness matters. A signing time more than five minutes in the future fails with `AR723`.
- **Rollback** is detected per machine: `verify --attestation` records the newest signing time it verified for each
  signer and project, and an older attestation fails with `AR727` (the message names the state file to delete if the
  newer one was wrong). A bundle with no time records the lock versions this machine has seen instead.
- The state lives outside the checkout (`$XDG_STATE_HOME/ai-rulez/signing-state.json`, else
  `~/.local/state/ai-rulez/`) and is authenticated with a per-user secret, so a repository cannot plant or reset it.
  `lock --check`, `generate --locked` and `validate` read it; only `verify --attestation` advances it. `--no-state`
  skips it.
- A fresh machine or CI runner starts with no state, so an old valid bundle passes there within `max_age`. Set
  `max_age` to your release cadence.

## Bundles, skills and SBOMs

```console
$ ai-rulez sign --bundle dist/acme-plugin --key release.key
INFO  Signed bundle path=dist/acme-plugin signer=key sha256:91be... subject=sha256:4f1a... bundle=dist/acme-plugin/.ai-rulez.sigstore.json
$ ai-rulez verify --bundle dist/acme-plugin --public-key release.pub
OK  bundle  signer=sha256:91be...
    issuer=none (key)  logged=no  age=3s
    digest=sha256:4f1a...
```

| Subject | Sign | Verify | Statement subject | Sidecar |
| --- | --- | --- | --- | --- |
| Plugin bundle | `sign --bundle <dir>` | `verify --bundle <dir>` | tree digest of every file in the directory | `<dir>/.ai-rulez.sigstore.json` |
| Published skill | `sign --skill <dir>` | `verify --skill <dir> [--source <name>]` | tree digest of the skill directory | `<dir>/.ai-rulez.sigstore.json` |
| SBOM | `sign --sbom <file>` | `verify --sbom <file>` | SHA-256 of the file's bytes | `<file>.sigstore.json` |
| Organization policy | `sign --org-policy <file>` | loaded by the [policy](/ai-rulez/policy/#signed-policies) loader | the policy digest | `<file>.sigstore.json` |

The directory digest uses the lock's [hashing scheme](/ai-rulez/lockfile/#hashing-scheme): adding, removing or editing a file,
or flipping an executable bit, invalidates the signature (`AR724`). The attestation files at the root of the directory
are not part of what they sign. A symlink, an irregular file or a nested `.git` in the directory is refused. The
directory's own name is not part of the digest.

The verify commands take the same checks and exit codes as the lock (`--attestation-file` names another sidecar). They
read `[[signing.trust]]` entries for their subject when a project exists; a consumer without a project passes
`--public-key`, or `--identity` with `--issuer`.

## Thresholds

`[signing.thresholds]` asks for k distinct trusted signers of a subject:

```toml
[signing.thresholds]
lock = 2

[[signing.trust]]
key_file = ".ai-rulez/keys/alice.pub"
reviewer = "alice@example.org"

[[signing.trust]]
key_file = ".ai-rulez/keys/bob.pub"
reviewer = "bob@example.org"
```

```bash
ai-rulez sign --lock --key alice.key
ai-rulez sign --lock --key bob.key --append     # writes ai-rulez.lock.2.sigstore.json
```

Each signer writes their own bundle (at most 16). Verification counts distinct people: a certificate by its identity,
a key by its entry's `reviewer` (else its fingerprint), so two bundles by one person count once. Too few valid signers is
`AR728`. A threshold of k needs at least k trust entries for the subject, or an `identity_regexp`. Thresholds apply per
subject (`lock`, `bundle`, `skill`, `sbom`) wherever that subject is verified.

## SLSA provenance

`sign --bundle <dir> --provenance` also writes `.ai-rulez.provenance.sigstore.json`: an in-toto statement with the
[SLSA provenance v1](https://slsa.dev/spec/v1.0/provenance) predicate over the bundle's tree digest. It records the
repository, ref and commit, the CI run URL, and a builder id: `--builder-id`, else `GITHUB_WORKFLOW_REF`, else
`https://github.com/Goldziher/ai-rulez/builders/cli/v1`.

ai-rulez does not run a hermetic build, so this is the signer's own account of where the bundle was generated (at most
SLSA build level 1). Provenance from a CI builder such as `slsa-github-generator` verifies the same way.

```toml
[signing]
require_provenance = true      # verify --bundle demands it (or pass --require-provenance)
builders = ["https://github.com/acme/plugin/.github/workflows/release.yml@refs/heads/main"]
```

A provenance file that exists is always verified, like the bundle attestation. A missing file under
`require_provenance`, a predicate that is not SLSA provenance v1, or a builder outside `builders` is `AR729`.

## KMS keys

`--key` accepts a cosign key URI: `awskms:///alias/release`,
`gcpkms://projects/p/locations/l/keyRings/r/cryptoKeys/k`, `azurekms://vault.vault.azure.net/key` or
`hashivault://key` (a `sigstore-kms-<name>` plugin binary adds others). The private key never leaves the KMS.
Credentials come from the provider's usual environment (`AWS_*`, `GOOGLE_APPLICATION_CREDENTIALS`, `AZURE_*`,
`VAULT_*`), never from a flag. Use ECDSA P-256 or ed25519 keys; RSA keys are not supported.

The bundle names the key by fingerprint, so verification stays offline. Export the public key once and trust it by file:

```bash
ai-rulez sign --lock --key awskms:///alias/release --public-key-out .ai-rulez/keys/release.pub
```

## Served skills and publisher-signed skills

`ai-rulez mcp --serve-skills` can refuse skills the policy does not vouch for; `load_skill` then says why.

```toml
[signing]
require = ["served", "skill"]

[[signing.trust]]              # the lock's signers, for "served"
key_file = ".ai-rulez/keys/release.pub"

[[signing.trust]]              # a publisher, for one skill source
subject = "skill"
source = "shared"
key_file = ".ai-rulez/keys/publisher.pub"
```

| `require` | Meaning |
| --- | --- |
| `served` | The lock must carry a valid attestation (otherwise every skill is refused with `AR720` to `AR728`), and a skill the signed lock does not pin with exactly its served digest is refused (`AR995`). Your own release workflow signs the lock. Recommended |
| `skill` | Every skill from a `[[skill_sources]]` or `[[installed_skills]]` entry must carry `.ai-rulez.sigstore.json` (`ai-rulez sign --skill`) signed by a `subject = "skill"` entry for that source. Project skills are not checked; a skill from an include cannot carry an attestation and is refused, so cover includes with `served` |

The server recomputes each skill's digest from the files on disk and compares each served file with the signed bytes,
so a file changed after signing, or not covered by the signature, is `AR724`. Only the `name:` line of `SKILL.md` may
differ, because a source serves it under the served name.

`generate` applies `skill` to the installed skills it writes into harness directories: each needs a valid attestation
covering the directory on disk, otherwise nothing is written and the error lists each skill with its `AR72x` code. Only
a dry run skips the check.

## Verifying ai-rulez itself

`ai-rulez verify --self` checks the running binary against the Sigstore bundle its release published: a SLSA
provenance statement whose subject is the binary's SHA-256, signed by the release workflow pinned in the binary:

- identity `^https://github\.com/Goldziher/ai-rulez/\.github/workflows/publish\.yaml@refs/tags/v[0-9][^/]*$`;
- issuer `https://token.actions.githubusercontent.com`;
- a transparency-log proof.

```bash
ai-rulez trust update                       # once: cache the public-good trusted root
ai-rulez verify --self --attestation-file ai-rulez_5.10.0_linux_amd64.sigstore.json
```

Without `--attestation-file` the bundle is read from `<binary>.sigstore.json` next to the resolved binary. For a build
from another workflow, `--public-key` or `--identity` with `--issuer` replace the pinned identity. Exit `0` official
build; `1` could not check (`AR720` no bundle, `AR725` no usable trusted root); `2` the check failed (`AR722` another
signer, `AR724` different bytes).

Limits:

- **A tampered binary can lie.** `verify --self` runs the binary it verifies. When the binary itself is in doubt, verify
  it with a trusted copy or compare `sha256sum` with the release page.
- **No rollback protection.** An older, genuinely signed release verifies; compare the reported version with the one you
  expect.
- **The trusted root is not embedded.** By default it is the user cache `trust update` writes; pass `--trusted-root`
  with a read-only copy where that matters.

## Cosign interoperability

`ai-rulez` writes a standard Sigstore bundle. Because the payload is an in-toto attestation, verify it with
`cosign verify-blob-attestation` and the subject digest `lock --subject` prints:

```bash
cosign verify-blob-attestation --bundle .ai-rulez/ai-rulez.lock.sigstore.json \
  --key cosign.pub --type https://github.com/Goldziher/ai-rulez/attestations/lock/v1 \
  --digest "$(ai-rulez lock --subject --format json | jq -r .subject | cut -d: -f2)" --digestAlg sha256 \
  --insecure-ignore-tlog=true   # only for a key bundle signed without --tlog
```

The other direction works too: `verify --attestation` accepts the blob-signature bundle of the
[`cosign sign-blob` recipe](/ai-rulez/lockfile/#sign-in-the-release-workflow), and a bundle written by
`cosign attest-blob --type <predicate type>`. Such bundles name no repository and have no `issued_at`, so without a log
entry they have no signing time: `max_age` cannot be met and no rollback mark applies.

## Codes

| Code | Name | Meaning | Fix |
| --- | --- | --- | --- |
| `AR720` | `signature-missing` | `require` asks for a signature and none exists | `ai-rulez sign --lock` and commit the bundle |
| `AR721` | `signature-invalid` | bad bundle, envelope, signature, certificate chain or log proof | re-sign |
| `AR722` | `signer-not-trusted` | the signer matches no trust entry, is outside its validity window, or an `identity_regexp` is unanchored | add or fix the trust entry, or sign with a trusted key |
| `AR723` | `signature-stale` | older than `max_age`, no time to measure it, or a signing time in the future | re-sign; add `--tlog` for a trustworthy time |
| `AR724` | `attestation-subject-mismatch` | the lock (or bundle, skill, SBOM) changed after signing, or the hash version is below `min_hash_version` | review the change, then re-sign |
| `AR725` | `trusted-root-unavailable` | a certificate bundle and no trusted root | `ai-rulez trust update` or `--trusted-root` |
| `AR726` | `tlog-proof-missing` | `tlog = "required"` and the bundle has no log entry | sign keyless or with `--tlog`, or set `tlog` for key-only trust |
| `AR727` | `signature-rollback` | older than the newest attestation this machine verified | find out why an older lock is checked out; delete the named state file if the newer one was wrong |
| `AR728` | `signature-threshold-not-met` | fewer distinct trusted signers than `[signing.thresholds]` | collect the missing signatures with `sign --append` |
| `AR729` | `provenance-invalid` | provenance missing under `require_provenance`, not SLSA v1, or from a builder outside `builders` | sign with `--provenance` from an allowed builder |

See [Strict validation](/ai-rulez/strict-validation/#ar720-signature-missing) for each code.
