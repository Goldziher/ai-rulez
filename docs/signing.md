# Signing

A digest proves that bytes did not change. A signature says who produced them. `ai-rulez sign --lock` signs the
[lock](lockfile.md) into a [Sigstore](https://www.sigstore.dev) bundle, and `ai-rulez verify --attestation` checks it
offline against a policy that names who may sign. Signing is opt-in; nothing is enforced until `[signing] require`
says so.

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
sensitive in a private repository. `approvals_digest` is empty until the approval set is part of the subject (see
[Lock file](lockfile.md#signing-the-lock)).

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
require = ["lock"]              # lock --check and generate --locked fail without a valid attestation
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
subject = "lock"
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

Verification without any trusted signer is an error (exit `1`), not "accept any valid signature": the policy, or
`--public-key` or `--identity` with `--issuer`, must say who may sign.

The repository can edit its own `[signing]` table, so a pull request can weaken it. Protect the table with code review
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
signature, the certificate chain and log proof (`AR721`, `AR725`, `AR726`), the signer against `[signing]` (`AR722`),
that the signed digest is the lock's recomputed subject and `hash_version` (`AR724`), `max_age` (`AR723`) and rollback
(`AR727`). `--format json` prints `schema/verify-attestation.schema.json`. Exit codes: `0` verified, `1` the check
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

`AR728` and `AR729` are reserved. See [Strict validation](strict-validation.md#ar720-signature-missing) for each code.

## Reusing the signing API

The `internal/signing` package signs and verifies any in-toto statement; the lock is its first user. Its package
documentation lists the calls: `NewStatement`, `SignStatement` with a `KeySigner` or `KeylessSigner`, `Verifier.Verify`,
`TrustSet.Check`, `CheckFresh` and the HMAC-protected `State`. Features that sign something else (approvals, an SBOM, a
policy, a bundle) pick a predicate type URI and reuse them.

## Not done yet

Signing plugin bundles and gating served skills on signatures, publisher-signed skills, SLSA provenance, KMS keys
through cosign and multi-party thresholds are tracked in [#263](https://github.com/Goldziher/ai-rulez/issues/263).
Live keyless tests run only with `AI_RULEZ_LIVE_SIGSTORE=1` and are never part of the default test run.
