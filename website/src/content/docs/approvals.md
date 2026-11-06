---
title: "Approvals"
description: "Record that a named person reviewed a piece of content and accepted its digest, and require approvals in CI."
---

A scan says content looks clean; an approval records that a named person read it and accepted it. `ai-rulez approve`
writes the approval into [`ai-rulez.lock`](/ai-rulez/lockfile/), bound to the content's exact digest, and `[governance]` makes
selected content unusable until it is approved. An approval is a human assertion, never a safety proof, and on its own
it is not authentication (see [Approvals are assertions](#approvals-are-assertions)).

## Example

Require approval for every hook and skill:

```toml
# .ai-rulez/config.toml (excerpt)
[governance]
require_approval = ["kind:hook", "kind:skill"]
approvers        = ["alice@example.org", "bob@example.org"]
max_age          = "365d"
enforce          = true
```

After `ai-rulez lock`, nothing is approved yet and the lock check fails:

```console
$ ai-rulez approve --list
KIND   ID                 DIGEST         STATUS   REVIEWER  EXPIRES  ASSURANCE
hook   PreToolUse:Bash:0  b99ff8c5372c…  missing  -         -        -
skill  deploy             70094bee7309…  missing  -         -        -
2 require approval: 0 ok, 0 stale, 2 missing, 0 expired, 0 unauthorized, 0 insufficient

$ ai-rulez lock --check
ai-rulez.lock does not match /work/acme/.ai-rulez:
  approval: AR710 missing: hook:PreToolUse:Bash:0 requires approval and has none; review it, then run `ai-rulez approve hook:PreToolUse:Bash:0`
  approval: AR710 missing: skill:deploy requires approval and has none; review it, then run `ai-rulez approve skill:deploy`
...
```

Review the content, then approve it:

```console
$ ai-rulez approve --diff skill:deploy
skill:deploy  sha256:70094bee730997ae05d11499b29571a05dde6cb5f17a4af98ed29465ba50d855
  SKILL.md  112 bytes
  scripts/run.sh  28 bytes (executable)
  scan: no findings

$ ai-rulez approve skill:deploy --note "read scripts/run.sh" --yes
...
approved skill:deploy at 70094bee7309… by alice@example.org (assurance=asserted, expires 2027-11-05)

$ ai-rulez approve hook:PreToolUse:Bash:0 --reviewer bob@example.org --yes
approved hook:PreToolUse:Bash:0 at b99ff8c5372c… by bob@example.org (assurance=asserted, expires 2027-11-05)
```

Each approval is an `[[approval]]` table in the lock:

```toml
[[approval]]
kind = 'skill'
id = 'deploy'
digest = 'sha256:70094bee730997ae05d11499b29571a05dde6cb5f17a4af98ed29465ba50d855'
reviewer = 'alice@example.org'
assurance = 'asserted'
approved_at = '2026-11-05T00:00:00Z'
expires = '2027-11-05'
note = 'read scripts/run.sh'
```

Commit the lock. When the skill changes, its approval goes stale and the check fails again:

```text
approval: AR711 stale: skill:deploy was approved at sha256:70094bee7309… but its content is now sha256:015acba08ec1…; review the change, then run `ai-rulez approve skill:deploy`
```

## Recording an approval

| Topic | Behavior |
| --- | --- |
| Naming | `kind:id` or `kind:domain/id`; a bare id works when unambiguous. Kinds are the pinned item kinds (`rule`, `context`, `skill`, `agent`, `command`, `check`, `hook`, `role`, `settings`, `verifier`, `rubric`, `local-include`, `local-skill`, `local-marketplace`, `local-plugin`) and the lock's remote entries: `include`, `installed-skill`, `source`, `served` (a served skill, with its serve view as the domain), `marketplace`, `installed-plugin`, and `role-output` ([Role outputs](#role-outputs)) |
| Pinned only | only content in the lock can be approved: run `ai-rulez lock` first. `approve` never fetches |
| What it shows | the files behind the item, the security scan findings and any earlier approval. Content with an error-level finding is refused unless you name its code with `--accept`; accepted codes are stored with the record |
| Confirmation | without `--yes` it asks on a terminal and refuses elsewhere, so a script cannot approve by accident |
| Reviewer | `--reviewer`, else `$AI_RULEZ_REVIEWER`, else the git `user.email`. A free-form string, compared lower-cased and trimmed; `github:alice`, `@alice` and `alice` are one reviewer. Use a handle if you do not want an email in a committed file |
| Expiry | `--expires YYYY-MM-DD`, else today plus `[governance] max_age`, else none. An approval holds through its expiry date, judged by the wall clock |
| Time | `approved_at` is now, or `--at`, or `SOURCE_DATE_EPOCH`, for reproducible runs. It is re-emitted byte for byte when the lock is rewritten, so `ai-rulez lock` never makes an approval diff |
| Re-approving | replaces that reviewer's earlier record of the item; other reviewers' records stay |
| Removing | `--revoke <item>` removes the records of an item (`--reviewer` limits it to one reviewer); `--prune` removes records of content that no longer exists or whose digest changed |
| Output safety | control, bidirectional and zero-width characters in printed paths, findings and notes are shown as `\u{XXXX}`. `--note` and `--reason` are scanned for secrets and refused when they hold one |

## When an approval stops applying

An approval is bound to the digest the lock pins ([hashing scheme](/ai-rulez/lockfile/#hashing-scheme)). It goes **stale**
(`AR711`) on any change to the bytes of the item or of any file its digest covers (a skill's scripts and resources, a
hook's script, a remote tree) and on an executable-bit flip. A CRLF-only edit of a markdown or other text file does not
change the digest; the same edit of a script does. A remote source that moves to a new commit with an identical tree
keeps its approval.

`ai-rulez lock` keeps every approval. Stale ones stay until the content is approved again or `approve --prune` removes
them; approvals of content that no longer exists are dropped with a warning (`AR715`). A renamed item must be
approved again under its new name.

## Requiring approvals

```toml
[governance]
require_approval = ["remote", "kind:hook", "kind:mcp_server"]
exempt           = ["skill:acme-internal/*"]
min_approvers    = 2
approvers        = ["alice@example.org", "bob@example.org", "@acme/security"]
approvers_from   = "CODEOWNERS"
min_assurance    = "review-linked"
forbid_self_approval = true
max_age          = "365d"
enforce          = true

[governance.teams]
"@acme/security" = ["carol@example.org", "github:dave"]
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `require_approval` | list | `[]` | What needs approval: `remote` (includes, installed skills, skill sources and remote served skills), `local` (every authored item), `all`, or `kind:<kind>` with a kind from the naming row above or `mcp_server` (the pinned `[[mcp_servers]]` declarations) |
| `exempt` | list | `[]` | Globs over `kind:id` or `kind:domain/id` (`*` any run of characters, `?` one) that never need approval |
| `min_approvers` | int | `1` | Distinct reviewers needed for one digest |
| `approvers` | list | any reviewer | Allowlist of reviewer strings. An `@org/team` entry stands for its members ([Who may approve](#who-may-approve)) |
| `approvers_from` | string | none | `"CODEOWNERS"` (in `.github/`, the repository root or `docs/`) or a path to a CODEOWNERS file inside the project: only owners of an item's path may approve it |
| `[governance.teams]` | table | none | Maps `"@org/team"` to the reviewers it stands for, offline |
| `min_assurance` | string | `"asserted"` | The weakest [assurance level](#assurance-levels) that counts |
| `forbid_self_approval` | bool | `false` | An approval by an author of the content does not count ([Self-approval](#self-approval)) |
| `max_age` | string | none | Default and maximum lifetime of an approval, `365d` or a Go duration such as `720h`. Past `approved_at` plus `max_age` an approval is expired (`AR712`) whatever its `expires` says |
| `enforce` | bool | `false` | Make `lock --check`, `generate --locked` and `mcp --serve-skills` fail. `validate` reports either way |

`[governance]` in the machine-local overlay is ignored with a warning. An [organization policy](/ai-rulez/policy/) can set a
floor under it: `enforce` on, selectors that `exempt` cannot narrow, a minimum `min_approvers` and `min_assurance`,
`forbid_self_approval`, `approvers_from = "CODEOWNERS"`, and the only `approvers` that count. A repository that loosens
one is clamped and reported as `AR740`.

Under `enforce` the check never fails open: a missing `ai-rulez.lock`, or one that pins no content, is `AR710`.

## Where it is enforced

| Where | Behavior |
| --- | --- |
| `validate` | `AR710` to `AR719` at their default severities, with or without `enforce` |
| `lock --check`, `generate --locked` | with `enforce`, exit `2` for content whose approval is missing, stale, expired, unauthorized or insufficient. `lock --diff --format json` lists them with `scope: "approval"` |
| `mcp --serve-skills` | with `enforce`, a selected served skill without a valid approval of its served digest is refused with its `AR71x` code. Provenance gains `approved` and `approvers` |
| `catalog --format json --schema-version 2` | `items[].approval` is `{required, status, reviewers, assurance, expires}` (`schema/catalog.schema.json`) |
| `approve --list --format json` | the policy and the status of every item (`schema/approve-list.schema.json`); `--all` also lists content that needs no approval |
| `sbom` | per-component `ai-rulez:approval` status ([SBOM](/ai-rulez/sbom/#approvals-and-signature)) |

| Code | Name | Meaning |
| --- | --- | --- |
| `AR710` | `approval-missing` | Required, no record |
| `AR711` | `approval-stale` | Records exist, none for the current digest |
| `AR712` | `approval-expired` | Every record of the current digest is past `expires` or `max_age` |
| `AR713` | `approver-not-authorized` | Every record of the current digest is by a reviewer outside `approvers` or CODEOWNERS |
| `AR714` | `approval-insufficient` | Fewer distinct valid reviewers than `min_approvers`, or none at `min_assurance` |
| `AR715` | `approval-orphan` | A record names content that no longer exists (warning) |
| `AR716` | `approval-with-change` | An approval arrived in the same change as its content, or by an author of it ([Approvals are assertions](#approvals-are-assertions)) |
| `AR717` | `approval-denied` | The digest is on the [deny list](#deny-list) |
| `AR718` | `approval-unverified` | Every approval of the current digest claims an assurance that could not be verified |
| `AR719` | `approver-unresolved` | `approvers_from` or a team cannot be resolved, so nobody is authorized by it |

## Assurance levels

| Level | Written by | The record carries | How it is checked |
| --- | --- | --- | --- |
| `asserted` | `approve` | a free-form reviewer string | not at all: it is as strong as the review of the change that adds it |
| `review-linked` | `approve --from-github-review` | `github:<login>` and `ref`, the URL of an approving pull request review | offline, the `ref` must be a well-formed review link of the `origin` repository; `verify --approvals --online` asks the forge |
| `signed` | `approve --sign` | a DSSE attestation under `.ai-rulez/attestations/`; the signer is the reviewer | offline, against `[[signing.trust]]` entries with `subject = "approval"` |

With `min_assurance = "signed"` an `asserted` record never counts. A record that claims a level it cannot back (a
`signed` one whose attestation does not verify, a `review-linked` one without a `ref`) does not count (`AR718`).
`approve --list` shows the weakest assurance among the approvals that apply.

## Who may approve

Without `approvers` and `approvers_from`, any reviewer string counts. With both set, both must hold.

- `approvers` is an allowlist; an `@org/team` entry expands through `[governance.teams]`, or from the forge with
  `approve --resolve-teams` (a token with `read:org`, see [Forge client](/ai-rulez/forge/)). `validate` and the skills server
  never call the forge, so list teams in `[governance.teams]` for them.
- `approvers_from = "CODEOWNERS"` resolves the owners of the item's source path: the file of a rule, the directory of
  a skill, or `.ai-rulez/ai-rulez.lock` for content without a source file (includes, installed skills, sources,
  served skills, role outputs). Owners can be `@user`, `@org/team` or an email.

CODEOWNERS is matched with GitHub's rules: gitignore-like patterns, the last matching line wins, a pattern without a
slash matches at any depth, a leading `/` anchors, a trailing `/` owns everything below, `docs/*` owns only the files
directly in `docs/`, `**` crosses directories. Lines with `!` or `[abc]` are ignored, as GitHub does.

Everything fails closed: a CODEOWNERS file that cannot be found, a path no line covers and a team with no known members
authorize nobody, and `validate` says why (`AR719`).

## Review-linked approvals

```console
$ git fetch origin pull/42/head
$ ai-rulez approve include:shared --from-github-review 42 --yes
approved include:shared at 51e0a1b2c3d4… by github:alice (assurance=review-linked, expires 2027-11-05)
```

`--from-github-review <pr>` records one `review-linked` approval per qualifying reviewer of the pull request
(`--reviewer github:alice` picks one). The repository comes from the git `origin` (or `GITHUB_REPOSITORY`) and its host
must be on the [forge allowlist](/ai-rulez/forge/#safety-properties). A review counts when all of these hold:

- it is the reviewer's latest decisive review and it is `APPROVED` (a later `CHANGES_REQUESTED` or `DISMISSED`
  withdraws it);
- the reviewer is named by `approvers` or CODEOWNERS for the item, or is an `OWNER`, `MEMBER` or `COLLABORATOR` of the
  repository with a `write`, `maintain` or `admin` role (the role is read when the token has push access; otherwise the
  association is the evidence). A drive-by approval on a public repository approves nothing;
- the review was made on the pull request's **final head commit**. A review of an earlier head approves nothing: push,
  then ask for a new review;
- the content at that commit has the digest being approved. Authored content is recomputed from the files of that
  commit, never read from the lock committed there; content only the lock describes (remote includes, installed skills,
  remote served skills, role outputs) is taken from the lock at that commit. The head must be in the local clone;
- the reviewer passes `approvers` and `approvers_from`, and with `forbid_self_approval` is not the pull request author.

Approving in the same pull request that changes the content is still `AR716`: record the approval from the merged pull
request, in a later change (see the [workflow](#approval-bot-workflow)).

## Signed approvals

```console
$ ai-rulez approve hook:PreToolUse:Bash:0 --sign --key approver.key --yes
...
approved hook:PreToolUse:Bash:0 at b99ff8c5372c… by key:sha256:c49716c6aeb4b89ba6176d8b7e7951b9a8801e92498d9104b4e4c58f8d8e0ac9 (assurance=signed, expires 2027-11-05)
```

```toml
[[approval]]
kind = 'hook'
id = 'PreToolUse:Bash:0'
digest = 'sha256:b99ff8c5372c10a70d8862804dbbbc53d26999e3a78f97ebfea20b4cd1312fd3'
reviewer = 'key:sha256:c49716c6aeb4b89ba6176d8b7e7951b9a8801e92498d9104b4e4c58f8d8e0ac9'
assurance = 'signed'
approved_at = '2026-11-05T00:00:00Z'
expires = '2027-11-05'
attestation = 'sha256:a001cd034d1a7a18282a2ec965cf38aaa5908df98c90c40d76a5d9186c6923de'
```

`--sign` signs an in-toto statement (predicate `https://github.com/Goldziher/ai-rulez/attestations/approval/v1`) over the
item digest, the expiry and the accepted findings, and writes the bundle to
`.ai-rulez/attestations/<sha256>.sigstore.json`. Commit it with the lock. `--key` signs offline; `--keyless` uses a
Fulcio certificate and a public Rekor entry, with the same flags as [`sign`](/ai-rulez/signing/#sign). The verified signer
replaces the reviewer string: `key:<fingerprint>` for a key, the certificate identity for keyless. `--reviewer` does
not combine with `--sign`. With `approvers` set, list the signer as it is recorded (`key:sha256:…` or the identity).

Name who may sign approvals with `[[signing.trust]]` entries whose `subject` is `"approval"`:

```toml
[governance]
require_approval = ["remote"]
min_assurance    = "signed"
approvers        = ["https://github.com/acme/ai-config/.github/workflows/approve.yml@refs/heads/main"]

[[signing.trust]]
subject  = "approval"
identity = "https://github.com/acme/ai-config/.github/workflows/approve.yml@refs/heads/main"
issuer   = "https://token.actions.githubusercontent.com"
```

A signed record counts only when its bundle verifies against those entries, covers exactly this item and digest, and
agrees with the record (reviewer, `approved_at`, expiry, accepted findings), so editing the lock after signing breaks it
(`AR718`). For keyless signing, `approvers` and CODEOWNERS are checked before anything reaches the public log. The
approval set is part of the signed [lock subject](/ai-rulez/lockfile/#signing-the-lock): sign the lock after the last
`approve`.

## Deny list

```console
$ ai-rulez approve --revoke hook:PreToolUse:Bash:0 --deny --reason "runs curl | sh"
revoked 1 approval(s) of hook:PreToolUse:Bash:0
denied hook:PreToolUse:Bash:0 sha256:b99ff8c5372c10a70d8862804dbbbc53d26999e3a78f97ebfea20b4cd1312fd3
```

```toml
[[deny]]
digest = 'sha256:b99ff8c5372c10a70d8862804dbbbc53d26999e3a78f97ebfea20b4cd1312fd3'
reason = 'runs curl | sh'
```

A denied digest can be neither approved nor used, whether or not `[governance]` selects the item:

- `approve` refuses it, `validate` reports `AR717` for any pinned content with that digest, and the skills server never
  serves a denied skill;
- `ai-rulez lock` refuses to pin it: change the content and lock again;
- the entry names the digest, not the item, so it survives re-locking and blocks a rollback to the old bytes.

Remove an entry by editing the lock in a reviewed change. An organization policy's `sources.deny_digests` is a second,
organization-wide list (`AR747`, see [Organization policy](/ai-rulez/policy/)).

## Role outputs

A role whose outputs are pinned ([`pin = true`](/ai-rulez/lockfile/#composing-with-roles), or `lock --roles`) has an approval
subject of kind `role-output` (`role-output:backend`): the rendering the role produces, not only the sources it selects.

- Only `require_approval = ["kind:role-output"]` selects it; `all`, `local` and `remote` do not.
- `approve role-output:backend` renders the role in memory, shows the files and scans them. It refuses when the
  rendering differs from the pin: run `ai-rulez lock --roles` first.
- `role:backend` (the role declaration) and `role-output:backend` are separate subjects.

## Self-approval

With `forbid_self_approval = true` an approval does not count when its reviewer authored a commit that touched the item
since a base revision:

- `approve` counts commit authors since `--base <rev>`, else the branch's upstream, else `origin/HEAD`, and refuses
  when none can be found;
- `approve --verify-base <rev>` and `validate --approvals-base <rev>` report such approvals as `AR716`;
- with `--from-github-review`, the pull request author's own review is skipped.

Authors are matched by commit email (with `.mailmap`) or the GitHub noreply address of a `github:` login against the
reviewer string. It catches honest mistakes, not collusion. A key signature names no person, so with
`forbid_self_approval` it is `AR716` unless its trust entry names the owner:

```toml
[[signing.trust]]
subject  = "approval"
key_file = ".ai-rulez/keys/alice.pub"
reviewer = "alice@example.org"
```

## Verifying approvals

```console
$ ai-rulez verify --approvals
OK    signed        hook:PreToolUse:Bash:0  key:sha256:c49716c6aeb4b89ba6176d8b7e7951b9a8801e92498d9104b4e4c58f8d8e0ac9
SKIP  review-linked include:shared  github:alice: needs --online: a review link cannot be verified offline

$ ai-rulez verify --approvals --online
OK    signed        hook:PreToolUse:Bash:0  key:sha256:c49716c6aeb4b89ba6176d8b7e7951b9a8801e92498d9104b4e4c58f8d8e0ac9
OK    review-linked include:shared  github:alice
```

`verify --approvals` re-checks the approvals above `asserted` that apply to the current content: signed ones against
their attestations offline, and with `--online` review-linked ones against the forge (the review still exists, still
approves, and was made on content with the approved digest). Exit `0` every checked approval holds, `2` one does not
(`AR718`), `1` the check could not run, including an unreachable forge or a missing token. `--format json` follows
`schema/verify-approvals.schema.json`. Run it in CI wherever `min_assurance = "review-linked"` is the policy.

## Approval-bot workflow

The strongest setup needs no person to hold a signing identity: a workflow records approvals from the reviews of a
merged pull request, in a follow-up pull request that a code owner of `ai-rulez.lock` merges.

```yaml
# .github/workflows/approve.yml
name: record approvals
on:
  pull_request:
    types: [closed]
permissions:
  contents: write        # push the approval branch
  pull-requests: write   # open the follow-up pull request
jobs:
  approve:
    if: github.event.pull_request.merged == true
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
        with:
          ref: ${{ github.event.repository.default_branch }}
          fetch-depth: 0
      - run: git fetch origin "pull/${{ github.event.pull_request.number }}/head"
      - uses: Goldziher/ai-rulez@v5
        with:
          version: 5.11.1
      - name: Record the approvals of the merged pull request
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          ai-rulez approve --list --format json | jq -r '.items[] | select(.status != "ok") | .ref' > pending.txt
          xargs -r ai-rulez approve --from-github-review "${{ github.event.pull_request.number }}" --yes < pending.txt
          ai-rulez verify --approvals --online
      - uses: peter-evans/create-pull-request@v6
        with:
          branch: approvals/${{ github.event.pull_request.number }}
          title: "chore: record approvals of #${{ github.event.pull_request.number }}"
          add-paths: .ai-rulez/ai-rulez.lock
```

- Pin the actions by commit.
- Put `ai-rulez.lock` under CODEOWNERS and require that owner's review on the follow-up pull request.
- To sign instead, add `id-token: write`, call `approve --sign --keyless`, and trust the workflow in
  `[[signing.trust]]` with `subject = "approval"`. The identity is the workflow file at a ref: protect both.
- A pull request from a fork gets a read-only `GITHUB_TOKEN`. Do not widen it, and never run pull request code in a
  workflow that holds a write token.

## Approvals are assertions

An `asserted` approval is a string in a committed file. ai-rulez does not know who ran `approve`, and anyone who can
edit the lock can add a record for any reviewer. Its strength is the review of the change to the lock. The
`[governance]` table itself is changed by the same pull request it gates, which is why an organization
[policy](/ai-rulez/policy/) can set a floor under it.

Run this on every pull request whose base branch is trusted:

```bash
ai-rulez approve --verify-base origin/main        # exit 2 on AR716
ai-rulez validate --approvals-base origin/main
```

Both compare the lock with the one at the merge base and report (`AR716`):

- every new `[[approval]]` whose item is also new or changed in the same change;
- a change to the CODEOWNERS file `approvers_from` names, or to the `[governance]` table;
- a new approval whose reviewer did not own the item in CODEOWNERS as it was at the merge base.

An approval of content the base already pinned at the same digest is a later review and passes. An unknown revision is
an error (exit `1`), never a pass. A finding means the change cannot vouch for itself: land the content first and
approve it in a following change, or require a second reviewer through code owners on `ai-rulez.lock`. Treat any pull
request that touches `[[approval]]`, `[[deny]]` or `[governance]` as sensitive.

Where approvals must hold against a hostile change, set `min_assurance = "review-linked"` or `"signed"` and run
`verify --approvals --online` in CI.

## Troubleshooting

| Message | Cause | Fix |
| --- | --- | --- |
| `not a terminal: refusing to approve without --yes` | `approve` in a script or CI | pass `--yes` |
| `… is not in [governance] approvers` | the reviewer, or the signing key's `key:sha256:…` identity, is not allowlisted | add it to `approvers`, or approve as an allowlisted reviewer |
| `… changed while it was being reviewed; nothing was written` | the content's digest changed between the review and the write | review it again, then re-run `approve` |
| `AR711 stale` after `ai-rulez lock` | the content changed since it was approved | `approve --diff <item>`, then `approve <item>` |
| `AR717 … is on the deny list … and cannot be approved` | the digest is in `[[deny]]` or the policy's `deny_digests` | change the content |
| `AR719 approver-unresolved` | no CODEOWNERS file, no line covers the path, or a team has no known members | add the CODEOWNERS line or `[governance.teams]` entry, or use `--resolve-teams` |
| `AR716` on a pull request | the approval arrived with the content it approves | approve in a later change |
