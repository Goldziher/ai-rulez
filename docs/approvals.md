# Approvals

A scan says content looks clean; an approval says a named person read it and accepted it. `ai-rulez approve` records
that in [`ai-rulez.lock`](lockfile.md), bound to the exact content digest, and `[governance]` makes chosen content
unusable until it is approved. Nothing here judges content: an approval is a human assertion, never a safety proof.
It is also not authentication: see [Approvals are assertions](#approvals-are-assertions).

## Record an approval

```console
$ ai-rulez approve --list
KIND     ID        DIGEST         STATUS   REVIEWER           EXPIRES     ASSURANCE
include  shared    51e0a1b2c3d4…  stale    alice@example.org  2027-01-05  asserted
hook     Stop:*:0  b410c2f91a2b…  missing  -                  -           -
2 require approval: 0 ok, 1 stale, 1 missing, 0 expired, 0 unauthorized, 0 insufficient

$ ai-rulez approve --diff include:shared
include:shared  sha256:51e0…
  previously approved at 9f2c1d0a7b3e… (now changed) by alice@example.org at 2026-10-05T12:00:00Z
  skills/deploy/SKILL.md  412 bytes
  skills/deploy/scripts/run.sh  96 bytes (executable)
  scan: 1 finding(s)
    AR005 error skills/deploy/scripts/run.sh: ...

$ ai-rulez approve include:shared --reviewer alice@example.org --note "read run.sh" --accept AR005 --yes
approved include:shared at 51e0a1b2c3d4… (assurance=asserted, expires 2027-10-05)
```

The record is a `[[approval]]` table in the lock:

```toml
[[approval]]
kind = "include"
id = "shared"
digest = "sha256:51e0…"
reviewer = "alice@example.org"
assurance = "asserted"
approved_at = "2026-10-05T12:00:00Z"
expires = "2027-10-05"
note = "read run.sh"
accepted_findings = ["AR005"]
```

- **Naming.** `kind:id` or `kind:domain/id`; a bare id works when unambiguous. Kinds are the pinned item kinds
  (`rule`, `context`, `skill`, `agent`, `command`, `check`, `hook`, `role`, `settings`, `verifier`, `rubric`, `local-include`) and the
  remote entries of the lock: `include`, `installed-skill`, `source`, `served` (a served skill, with the serve view as
  its domain), and `role-output` (the pinned outputs of a role, see [Role outputs](#role-outputs)). Only pinned content can be approved: run `ai-rulez lock` first. `approve` never fetches.
- **What approve shows.** The files behind the item, the security scan findings, and any earlier approval. It refuses
  content with an error-level finding unless you name its code with `--accept`; the accepted codes are stored with the
  record. Without `--yes` it asks on a terminal and refuses elsewhere, so a script cannot approve by accident.
- **Reviewer.** `--reviewer`, else `$AI_RULEZ_REVIEWER`, else the git `user.email`. It is a free-form string: ai-rulez
  has no identity notion. Use a handle if you do not want an email in a committed file. It is stored and compared
  lower-cased and trimmed, so `Alice@Example.org` and `alice@example.org` are one reviewer (for `min_approvers`,
  `approvers` and `--revoke --reviewer`).
- **Expiry.** `--expires YYYY-MM-DD` (not already past, and not later than `approved_at` plus `max_age`), else today plus `[governance] max_age`, else none. An
  approval holds through its expiry date. Expiry is judged by the wall clock: `SOURCE_DATE_EPOCH` never revives an
  expired approval.
- **Re-approving** replaces that reviewer's earlier record of the item; other reviewers' records stay until they approve
  again. `--revoke <item>` removes the records of an item (`--reviewer` limits it); `--prune` removes records of content
  that no longer exists or whose digest changed. Both also delete the `attestations/*.sigstore.json` bundle of a removed signed
  record unless another record still names it. `--reviewer` is compared as an identity (`github:Alice` and `alice` match).
- **Time.** `approved_at` is stored once and re-emitted byte for byte when the lock is rewritten, so `ai-rulez lock`
  never makes an approval diff. `--at` (not in the future) and `SOURCE_DATE_EPOCH` make the stamp reproducible; they
  never move the clock expiry is judged by.
- **Output safety.** Everything printed from content (paths, findings, notes) has control, bidirectional and
  zero-width characters replaced by `\u{XXXX}`, so a diff cannot hide text from the reviewer. `--note` is scanned for
  secrets (AR001) and refused when it holds one, because the lock is committed.

## When an approval stops applying

An approval is bound to one digest, the same digest the lock pins ([hashing scheme](lockfile.md#hashing-scheme)). So it
goes **stale** on any change to the bytes of the item or of any file its digest covers (a skill's scripts and
resources, a hook's script, the remote tree), and on an executable-bit flip. A CRLF-only edit of text content (rules,
markdown, a skill's `SKILL.md` and references) does not change the digest and never stales an approval; scripts and
other non-text files are hashed byte for byte ([lockfile.md](lockfile.md#hashing-scheme)), so converting their line
endings does. A served skill's digest ignores the generated `Content-Hash`, `Source-Hash` and `Generated` header
lines, which would otherwise move with a CRLF-only edit. A remote source that moves to a new commit with an identical tree keeps its
approval. `ai-rulez lock` keeps every approval: stale ones stay until the content is approved again or `--prune` removes
them; a full re-lock drops, with a warning (`AR715`), approvals of content that no longer exists. A renamed item must
be approved again under its new name. Items are pinned whatever the lock's profile (it only selects the pinned outputs), so relocking under another
profile or running `approve --prune` keeps the approval of an item that only some profile renders.

## Role outputs

A role whose outputs are pinned (`[[roles]]` `pin = true`, or `lock --roles`) has one aggregate pin in the lock: the
digest of everything `generate --role <name>` would write. That pin is an approval subject of kind `role-output`
(`role-output:dev`), so a team can approve what a role actually renders, not only the sources it selects from.

- It is selected only by `require_approval = ["kind:role-output"]`. `all`, `local` and `remote` do not select it, so
  pinning a role never widens an existing policy.
- `approve role-output:dev` renders the role (nothing is written), shows the rendered files and runs the security scan
  over them. It refuses when the rendering differs from the pin: run `ai-rulez lock --roles`, then approve.
- The approval goes stale when the role's outputs change and the lock is re-pinned, like any other digest. A role item
  (`role:<name>`) and the outputs of the role are separate subjects: approving one does not approve the other.

Approvals sit outside the lock's `tree` digest: adding one changes no pin and no output.

## Requiring approvals

```toml
[governance]
require_approval = ["remote", "kind:hook", "kind:mcp_server"]
exempt           = ["skill:acme-internal/*"]
min_approvers    = 1
approvers        = ["alice@example.org", "bob@example.org"]
approvers_from   = "CODEOWNERS"      # only the owners of an item's path may approve it
min_assurance    = "asserted"        # asserted | review-linked | signed
forbid_self_approval = false
max_age          = "365d"
enforce          = true

[governance.teams]                   # who "@acme/security" stands for, offline
"@acme/security" = ["alice@example.org", "github:bob"]
```

| Key | Meaning |
| --- | --- |
| `require_approval` | `remote` (includes, installed skills, skill sources and served skills that come from a remote), `local` (every authored item), `all` (both), `kind:<kind>` (for example `kind:hook`, `kind:skill`). `kind:mcp_server` selects the pinned MCP server declarations |
| `exempt` | Globs over `kind:id` or `kind:domain/id`; `*` matches any run of characters, `?` one. An exempt item never needs approval |
| `min_approvers` | Distinct reviewers needed for one digest. Default 1; one reviewer twice counts once |
| `approvers` | Allowlist of reviewer strings, matched lower-cased and trimmed (`github:alice`, `@alice` and `alice` are one reviewer). An `@org/team` entry stands for the members `[governance.teams]` lists. A typo guard and an audit aid for `asserted` records: it is not authentication (see below), unless an [organization policy](policy.md) pins the list |
| `approvers_from` | `"CODEOWNERS"` (found in `.github/`, the repository root or `docs/`, as the forge looks) or a path inside the project to a CODEOWNERS file. Only the owners of an item's source path may approve it; see [Who may approve](#who-may-approve) |
| `[governance.teams]` | Maps `"@org/team"` to the reviewers it stands for. Used by `approvers` and CODEOWNERS entries |
| `min_assurance` | The weakest [assurance level](#assurance-levels) that counts. Default `asserted` |
| `forbid_self_approval` | An approval by an author of the content it approves does not count; see [Self-approval](#self-approval) |
| `max_age` | Default and maximum lifetime of an approval: `365d` or a Go duration such as `720h`. An approval stops counting (`AR712`) once `approved_at` plus `max_age` has passed, whatever its `expires` says |
| `enforce` | Make `lock --check`, `generate --locked` and `mcp --serve-skills` fail. `validate --strict` reports regardless |

`[governance]` is shared policy: a machine-local config overlay cannot set or relax it (the key is ignored with a
warning). An [organization policy](policy.md) can set a floor under it: `enforce` on, `require_approval` selectors the
repository's `exempt` cannot narrow, a minimum `min_approvers`, and the only `approvers` that count. A repository that
loosens one is clamped and reported as `AR740`.

Under `enforce` the check never fails open: a missing `ai-rulez.lock`, or one that pins no content, is itself `AR710`
in `lock --check`, `generate --locked`, `validate --strict` and the skills server (`mcp --serve-skills`). Without `enforce` nothing is refused. A record that claims an `assurance` it cannot back (a `signed` one whose attestation does not verify, a
`review-linked` one with no `ref`, an unknown level) does not count (`AR718`).

## Where it is enforced

| Where | Behaviour |
| --- | --- |
| `validate --strict` | AR710 to AR719 at their default severities, `enforce` or not |
| `lock --check`, `generate --locked` | With `enforce = true`, exit 2 for content whose approval is missing, stale, expired, unauthorized or insufficient. `lock --diff --format json` lists them with `scope: "approval"` (`schema/lock-diff.schema.json`). Without `enforce` the diff only carries a note |
| `mcp --serve-skills` | With `enforce = true`, a served skill the policy selects (`remote`, `all`, `kind:served`) and that lacks a valid approval of its served digest is refused, with the AR71x code; with no lock every selected skill is refused (`AR710`). Provenance gains `approved` and `approvers`. The server also refuses unpinned or changed skills with `AR995` whenever a lock exists, even without a `[lock]` table |
| `catalog --format json --schema-version 2` | `items[].approval` is `{required, status, reviewers, assurance, expires}`, `null` for an item that needs no approval and has no record (`schema/catalog.schema.json`) |
| `approve --list --format json` | The policy and the status of every item (`schema/approve-list.schema.json`) |

| Code | Name | Meaning |
| --- | --- | --- |
| `AR710` | `approval-missing` | Required, no record |
| `AR711` | `approval-stale` | Records exist, none for the current digest |
| `AR712` | `approval-expired` | Every record of the current digest is past `expires` (a malformed date counts as expired) |
| `AR713` | `approver-not-authorized` | Every record of the current digest is by a reviewer outside `approvers` |
| `AR714` | `approval-insufficient` | Fewer distinct valid reviewers than `min_approvers` |
| `AR715` | `approval-orphan` | A record names content that no longer exists (warning) |
| `AR716` | `approval-with-change` | An approval was added in the same change as the content it approves, or (with `forbid_self_approval`) the reviewer authored a change to it (only with `approve --verify-base` or `validate --strict --approvals-base`); also a change to CODEOWNERS or `[governance]` in the range, and a new approval by a reviewer who did not own the item at the merge base |
| `AR717` | `approval-denied` | The content's digest is on the `[[deny]]` list |
| `AR718` | `approval-unverified` | Every approval of the current digest claims an assurance (`signed`, `review-linked`) that could not be verified |
| `AR719` | `approver-unresolved` | `approvers_from` or a team cannot be resolved, so nobody is authorized by it |

## Assurance levels

| Level | What the record carries | How it is checked |
| --- | --- | --- |
| `asserted` | A free-form reviewer string. | Not at all: it is as strong as the review of the change that adds it (see [below](#approvals-are-assertions)). |
| `review-linked` | The reviewer as `github:<login>` and a `ref`: the URL of an approving review of a pull request. | Offline it counts as claimed. `verify --approvals --online` asks the forge. |
| `signed` | A DSSE attestation of the approval under `attestations/`, and the signer's identity as the reviewer. | Offline, against `[[signing.trust]]` entries with `subject = "approval"`. |

`min_assurance` sets the weakest level that counts. A record below it does not count and the item reports
`AR714 approval-insufficient`; with `min_assurance = "signed"` an `asserted` record never satisfies the policy. A
`review-linked` record is a claim in a committed file until `verify --approvals --online` re-checks it, which is why it
ranks below `signed`: run that check in CI where `review-linked` is the policy.

`approve --list` shows the weakest assurance among the approvals that apply, and `approvals_digest` in the
[lock subject](lockfile.md#signing-the-lock) covers every record, so a signed lock also vouches for who approved what.

## Who may approve

Without `approvers` and `approvers_from`, any reviewer string counts. Two keys narrow it, and both must hold when both
are set:

- `approvers` is an allowlist. An `@org/team` entry expands through `[governance.teams]`.
- `approvers_from = "CODEOWNERS"` resolves the owners of the item's source path: the file of a rule, the directory of a
  skill, `.ai-rulez/ai-rulez.lock` for content with no source file of its own (includes, installed skills, sources,
  served skills, role outputs). The reviewer must be one of them; a team owner counts through its members. Owners can
  be `@user`, `@org/team` or an email; `github:alice`, `@alice` and `alice` are one person.

CODEOWNERS is matched with the documented forge rules: patterns are gitignore-like, the last matching line wins, a
pattern without a slash matches at any depth, a leading `/` anchors it, a trailing `/` or a directory name owns
everything below, `docs/*` owns the files directly in `docs/` but not below, and `**` crosses directories. Negation
(`!`) and character classes (`[abc]`) are not CODEOWNERS syntax and a line using them is ignored, as the forge does.
This was implemented from the documentation and tested against a table of cases, not against the forge itself.

Everything fails closed: a CODEOWNERS file that cannot be found, a path no line covers and a team with no known members
authorize nobody, and `validate --strict` says why (`AR719`). Teams are expanded offline from `[governance.teams]`;
`approve --resolve-teams` (and `approve --list --resolve-teams`) reads the members of the teams it needs from the forge
instead, with the token the [forge client](forge.md) finds in the environment (`read:org` is needed for the members of
a team). Nothing is cached: `validate` and the skills server never call the forge, so list the team in
`[governance.teams]` for them.

An organization [policy](policy.md) sets floors for `min_assurance` (the stronger level wins), `forbid_self_approval`
(on if either sets it) and `approvers_from = "CODEOWNERS"` (`AR740` when the repository turns it off). Only `teams` has
no floor; the repository can change it in the same pull request it would gate.

## Review-linked approvals

```console
$ ai-rulez approve include:shared --from-github-review 42 --yes
approved include:shared at 51e0a1b2c3d4… by github:alice (assurance=review-linked, expires 2027-10-05)
```

`--from-github-review <pr>` records one `review-linked` approval per approving reviewer of pull request 42 (`--reviewer
github:alice` picks one). The repository comes from the git origin (or `GITHUB_REPOSITORY`) and must be on the forge
host allowlist ([forge client](forge.md#safety-properties)). A review counts when all of these hold:

- it is the reviewer's latest decisive review and it is `APPROVED` (a later `CHANGES_REQUESTED` or `DISMISSED`
  withdraws it, a comment neither approves nor withdraws it);
- the reviewer is an `OWNER`, `MEMBER` or `COLLABORATOR` of the repository (the review's `author_association`), or is
  named by `approvers` or CODEOWNERS for the item. Anyone can review a public repository, so a drive-by `APPROVED` from
  anyone else approves nothing. The reviewer must also hold a `write`, `maintain` or `admin` role on the repository
  (read with `GET /repos/{o}/{r}/collaborators/{login}/permission`): `MEMBER` of a public repository's organization only
  reads it. A token that cannot read roles (it needs push access) leaves the association as the evidence;
  any other failure of that lookup is an error;
- **the commit the reviewer saw is the pull request's final head** (read with `GET /pulls/{n}`). A review of an earlier
  head approves nothing, even when the content looks the same: push, then ask for a new review (or have the branch
  protection dismiss stale reviews);
- the content at that head has the digest being approved. Authored content is **recomputed from the files of that
  commit**, never read from the lock committed there, so a lock edited to claim the new digest cannot vouch for content
  the reviewer did not see. Content only the lock describes (remote includes, installed skills, role outputs) cannot be
  recomputed offline: the lock at that commit stands in for it. The head must be in the local clone
  (`git fetch origin pull/42/head`);
- the reviewer passes `approvers` and, with `approvers_from`, owns the path, and with `forbid_self_approval` is not the
  pull request author. A reviewer who fails these is skipped with a note while another remains.

Offline, a `review-linked` record counts only when its `ref` is a well-formed review link (`https://host/owner/repo/pull/N#pullrequestreview-ID`) of the repository the `origin` remote names (host, owner and name compared case-insensitively). With no readable `origin` only the form is checked; `verify --approvals --online` asks the forge.

Listings that hit the forge page cap (`ErrTruncated`) are never counted. The record's `ref` is the review URL, which
names the pull request and review. Approving at the same pull request that changes the content is still `AR716`:
record the approval in a later change, from the merged pull request.

## Signed approvals

```console
$ ai-rulez approve skill:deploy --sign --key approver.key --yes
approved skill:deploy at 2da78adbc350… by key:sha256:91be… (assurance=signed, expires 2027-10-05)
```

`--sign` signs an in-toto statement (predicate `https://github.com/Goldziher/ai-rulez/attestations/approval/v1`) whose
subject is the item digest and whose predicate repeats the item, the expiry and the findings the reviewer accepted. The
bundle is written to `.ai-rulez/attestations/<sha256 of the bundle>.sigstore.json` and the record names it
(`attestation = "sha256:…"`). Key signing is offline; `--keyless` uses a Fulcio certificate and a Rekor entry, with the
same flags as [`sign`](signing.md). The signer's verified identity replaces the free-form reviewer: the certificate
identity for keyless, `key:<fingerprint>` for a key. `--reviewer` does not combine with `--sign`.

A keyless signature goes to a public log and cannot be taken back, so `approvers` and CODEOWNERS are checked **before**
anything is signed. The identity is read from the ID token (its verified `email`, or `job_workflow_ref` of a GitHub
Actions token). When an allowlist or `approvers_from` is set and the token's certificate identity cannot be told, the
approval is refused with nothing logged: sign with `--key` or a token of a readable shape.

Say who may sign approvals with `[[signing.trust]]` entries whose `subject` is `"approval"`:

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
agrees with the record (reviewer, expiry, accepted findings), so editing the lock after signing breaks it (`AR718`).
`tlog` follows `[signing]` (required by default for certificate identities). Rollback state and `max_age` are lock
features and do not apply. A keyless signature is logged before the allowlist is checked, so an unauthorized signer
leaves a log entry and no record.

The approval set is part of the signed [lock subject](lockfile.md#signing-the-lock): adding an approval changes the
subject, so sign the lock after the last `approve`.

## Deny list

```console
$ ai-rulez approve --revoke include:shared --deny --reason "exfiltrates ~/.ssh in scripts/setup.sh"
revoked 1 approval(s) of include:shared
denied include:shared sha256:7ab0…
```

```toml
[[deny]]
digest = "sha256:7ab0…"
reason = "exfiltrates ~/.ssh in scripts/setup.sh"
```

`--reason` is committed to the lock, so it is scanned for secrets and refused when one is found.

A denied digest can be neither approved nor used, whether or not `[governance]` selects the item:

- `approve` refuses it, `validate --strict` reports `AR717` for any pinned content with that digest, and a denied
  skill is never served by `mcp --serve-skills` (refusal `AR717`);
- `ai-rulez lock` refuses to pin it and leaves the lock as it was; change the content and lock again;
- the entry names the digest, not the item, so it survives re-locking and keeps blocking a rollback to the old bytes;
- the policy's `sources.deny_digests` is a second, organization-wide list (`AR747`). Both lists block, and `approve`
  refuses a digest on either: approving content that generation refuses would change nothing.

`[[deny]]` entries sit outside the tree digest, are carried over by `lock` and `update`, and are covered by
`approvals_digest`. Remove an entry by editing the lock in a reviewed change. An organization policy cannot ship a
shared deny list yet.

## Self-approval

`AR716` already catches an approval that arrives in the same change as its content (`--verify-base`). With
`forbid_self_approval = true` an approval also does not count when its reviewer authored a commit that touched the item
since the base revision:

- `approve` counts commit authors from `--base <rev>`, else the branch's upstream, else `origin/HEAD`, and refuses
  when none can be found rather than skip the check;
- `approve --verify-base <rev>` and `validate --strict --approvals-base <rev>` report approvals by an author as
  `AR716` with the author's email;
- with `--from-github-review`, the pull request author's own review is skipped.

The paths counted are the item's source (a file or skill directory), or the lock file for content with no source of
its own. The match is heuristic: the commit author email, or the GitHub noreply address of a `github:` login, against
the reviewer string, with `.mailmap` applied by git. A reviewer recorded under a name that matches none of the
author's addresses is not caught. It limits honest mistakes, not collusion; a second reviewer and code owners on the
lock are the control.

A signed approval is matched by its verified signer, not by the record's reviewer string. A key signature (`key:<id>`) names
no author, so with `forbid_self_approval` it is reported as `AR716`: sign with a keyless identity instead, or name the
key's owner in its trust entry (`reviewer = "alice@example.org"` on the `[[signing.trust]]` entry with `key_file`,
see [signing](signing.md#policy)); the owner's identity is then matched with commit authors like any other reviewer.

## Verifying approvals

```console
$ ai-rulez verify --approvals --online
OK    signed        skill:deploy  key:sha256:91be…
OK    review-linked include:shared  github:alice
$ ai-rulez verify --approvals
OK    signed        skill:deploy  key:sha256:91be…
SKIP  review-linked include:shared  github:alice: needs --online: a review link cannot be verified offline
```

`verify --approvals` re-checks the approvals above `asserted` that apply to the current content: signed ones against
their attestations offline, and with `--online` review-linked ones against the forge (the review still exists, still
approves, and was made on content whose lock pin is the approved digest). Records of content that changed are not
checked: they no longer apply. Exit `0` every checked approval holds, `2` one does not (`AR718`), `1` the check could
not run, including a forge that cannot be reached or a token that is missing: a check that could not ask is never a
pass. `--format json` follows `schema/verify-approvals.schema.json`.

## Approval-bot workflow

The strongest setup needs no one to hold a signing identity. A workflow runs when a pull request that changes content
has the reviews branch protection asks for, records the approval from those reviews and signs it with the workflow's
own identity, in a follow-up pull request that a code owner of `ai-rulez.lock` merges.

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
    # Only a merged pull request: the content must have landed before it is approved.
    if: github.event.pull_request.merged == true
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.repository.default_branch }}
          fetch-depth: 0
      - run: git fetch origin "pull/${{ github.event.pull_request.number }}/head"
      - run: npm install --global ai-rulez
      - name: Record the approvals of the merged pull request
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          ai-rulez lock --check
          ai-rulez approve --list --format json | jq -r '.items[] | select(.status != "ok") | .ref' > pending.txt
          xargs -r ai-rulez approve --from-github-review "${{ github.event.pull_request.number }}" --yes < pending.txt
          ai-rulez verify --approvals --online
      - uses: peter-evans/create-pull-request@v6
        with:
          branch: approvals/${{ github.event.pull_request.number }}
          title: "chore: record approvals of #${{ github.event.pull_request.number }}"
          add-paths: .ai-rulez/ai-rulez.lock
```

Notes on the pattern. Pin the actions by commit; the versions above are illustrative.

- Run `ai-rulez approve --verify-base origin/main` and `validate --strict --approvals-base origin/main` on pull
  requests that touch the lock, so a pull request cannot carry both content and its approval.
- Put `ai-rulez.lock` under CODEOWNERS and require that owner's review on the follow-up pull request: the approval of
  the approvals.
- To sign instead of linking reviews, add `id-token: write` to the permissions, call `approve --sign --keyless`, and
  trust the workflow in `[[signing.trust]]` with `subject = "approval"`. The identity is the workflow definition at a
  ref, so protect that ref and that file.
- A merged pull request from a fork gets a read-only `GITHUB_TOKEN`, so the workflow cannot push for it. Do not give it
  more access, and never check out and execute pull request code in a workflow that holds a write token.

## Approvals are assertions

An `asserted` approval is a string in a committed file. It is an assertion, not authentication: ai-rulez does not
know who ran `approve`, and whoever can edit the lock can add a record for any reviewer. Its strength is that of the
review of the change to the lock. The control is the CI recipe below plus branch protection and code owners on the lock
path; `[governance]` settings in the repository (`approvers`, `min_approvers`, `exempt`, `enforce`) are changed by the
same pull request they would gate, which is why an organization [policy](policy.md) can set a floor under them.

CI recipe. Run it on a pull request whose base branch is trusted:

```console
ai-rulez approve --verify-base origin/main        # exit 2 on AR716
ai-rulez validate --strict --approvals-base origin/main
```

Both compare the lock with the one at the merge base of the revision and `HEAD`, and report (`AR716`) every
`[[approval]]` that is new and whose item is also new or changed, pins compared, so no checkout of the base tree is
needed. An approval of content the base already pinned at the same digest is a later review and passes. An unknown
revision, or a configuration outside a git work tree, is an error (exit 1), never a pass; a base without a lock counts
every approval as new. A finding means the change cannot vouch for itself: require a second reviewer (code owners on
`ai-rulez.lock`) or land the content first and approve it in a following change. Also treat any pull request that
touches `[[approval]]`, `[[deny]]` or `[governance]` as sensitive.

With a base revision the same checks also read who may approve from the merge base, not from the change: `AR716` is
raised when the CODEOWNERS file `approvers_from` names, or the `[governance]` table of `config.toml`, differs between
the merge base and the working tree, and for each new approval whose reviewer does not own the item in CODEOWNERS as it
was at the merge base. A pull request cannot authorize itself by adding its author to CODEOWNERS. Without a base
revision `approvers_from` reads the working tree, so `asserted` is not a security boundary: set
`min_assurance = "review-linked"` or higher where approvals must hold against a hostile change. The stronger [assurance levels](#assurance-levels)
exist for the cases where that review is not enough: `review-linked` ties a record to a forge review, `signed` to a
signer a `[[signing.trust]]` entry names, and the approval set is part of the signed
[lock subject](lockfile.md#signing-the-lock).

## Design decisions

Taken from the proposed defaults of [#213](https://github.com/Goldziher/ai-rulez/issues/213) unless noted.

- **One file.** Approvals live in `ai-rulez.lock`, not a separate file, so `lock --diff` stays the single review
  artifact. Records are sorted by kind, domain, id, digest and reviewer, one table each, so merges stay clean.
- **Outside the tree digest.** An approval changes no pin and no `tree`, and does not bump the lock version.
- **No new pinned kind.** The issue sketches an `mcp_server` item kind. Adding one would change the tree digest of
  every project with MCP servers, so this slice keeps the existing `settings` item `mcp-servers` and lets
  `kind:mcp_server` select it. Approving it covers all MCP server declarations at once.
- **`lock --diff` scope.** `approval` is added to the scope enum without bumping the schema version; consumers must
  ignore unknown scopes.
- **Assurance.** `approve` writes `asserted` by default, `review-linked` with `--from-github-review` and `signed` with
  `--sign`. Offline, a `review-linked` record counts as claimed (it needs a `ref`); only `verify --approvals --online`
  asks the forge. A signed record's reviewer is the verified signer, never the string in the record.
- **Review linkage.** The design asks that the review be "on a commit that contains the digest". The forge client has no
  "pull request by number" call, so the check is: the review is approving and current, the forge lists the pull request
  among those containing the reviewed commit, and the lock at that commit pinned the approved digest. It does not
  recompute the digest from the reviewed tree, so it trusts that `lock --check` passed there.
- **Deny list location.** `[[deny]]` lives in the lock, as the design sketches, and is covered by `approvals_digest`. It
  is refused at pin time by `lock`, and enforced at serve time without `[governance]`.
- **Codes.** `AR717` is the deny list, `AR718` an assurance that cannot be verified, `AR719` an `approvers_from` or team
  that cannot be resolved. Insufficient assurance is `AR714`, as in the design; `forbid_self_approval` is `AR716`.
- **Role outputs.** Subject kind `role-output`, selected only by `kind:role-output`.
- **Self-approval.** Authors are matched by email (heuristic, offline); the PR author is also checked for
  `--from-github-review`.
- **Remote is decided by the lock.** A served skill counts as remote when its lock entry names a ref or commit or a git
  URL source; a skill authored in the project is only selected by `kind:served`.
- **Diff.** `approve --diff` lists files, findings and the previous approval; it does not print a text diff because
  the lock keeps digests, not the previously approved bytes. Use `git diff` and `lock --diff` for the text.
- **Reviewer default.** Read from `$AI_RULEZ_REVIEWER` or the `user.email` of the git configuration files, without
  running git.
- **Time.** Expiry compares the UTC date of the wall clock; `SOURCE_DATE_EPOCH` only stamps `approved_at`.
- **Self-review.** `AR716` is decided from lock pins at a git revision, not from commit authors, so it works with
  squash merges and needs no network.
- **Not done yet.** An approval column in the catalog site, organization-policy floors for `approvers_from`,
  `min_assurance`, `forbid_self_approval` and a shared deny list, and an `mcp_server` item kind of its own.
