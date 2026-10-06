# Approvals

A scan says content looks clean; an approval says a named person read it and accepted it. `ai-rulez approve` records
that in [`ai-rulez.lock`](lockfile.md), bound to the exact content digest, and `[governance]` makes chosen content
unusable until it is approved. Nothing here judges content: an approval is a human assertion, never a safety proof.
It is also not authentication: see [Approvals are assertions](#approvals-are-assertions).

## Record an approval

```console
$ ai-rulez approve --list
KIND     ID      DIGEST         STATUS   REVIEWER           EXPIRES
include  shared  51e0a1b2c3d4…  stale    alice@example.org  2027-01-05
hook     Stop:*:0  b410c2f91a2b…  missing  -                  -
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
  (`rule`, `context`, `skill`, `agent`, `command`, `check`, `hook`, `role`, `settings`, `local-include`) and the
  remote entries of the lock: `include`, `installed-skill`, `source`, `served` (a served skill, with the serve view as
  its domain). Only pinned content can be approved: run `ai-rulez lock` first. `approve` never fetches.
- **What approve shows.** The files behind the item, the security scan findings, and any earlier approval. It refuses
  content with an error-level finding unless you name its code with `--accept`; the accepted codes are stored with the
  record. Without `--yes` it asks on a terminal and refuses elsewhere, so a script cannot approve by accident.
- **Reviewer.** `--reviewer`, else `$AI_RULEZ_REVIEWER`, else the git `user.email`. It is a free-form string: ai-rulez
  has no identity notion. Use a handle if you do not want an email in a committed file. It is stored and compared
  lower-cased and trimmed, so `Alice@Example.org` and `alice@example.org` are one reviewer (for `min_approvers`,
  `approvers` and `--revoke --reviewer`).
- **Expiry.** `--expires YYYY-MM-DD` (not already past), else today plus `[governance] max_age`, else none. An
  approval holds through its expiry date. Expiry is judged by the wall clock: `SOURCE_DATE_EPOCH` never revives an
  expired approval.
- **Re-approving** replaces that reviewer's earlier record of the item; other reviewers' records stay until they approve
  again. `--revoke <item>` removes the records of an item (`--reviewer` limits it); `--prune` removes records of content
  that no longer exists or whose digest changed.
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

Role outputs (`[roles]` `pin = true`) are pinned in the lock but are not approval subjects: approve the role item
(`role:<name>`) and the content the role selects.

Approvals sit outside the lock's `tree` digest: adding one changes no pin and no output.

## Requiring approvals

```toml
[governance]
require_approval = ["remote", "kind:hook", "kind:mcp_server"]
exempt           = ["skill:acme-internal/*"]
min_approvers    = 1
approvers        = ["alice@example.org", "bob@example.org"]
max_age          = "365d"
enforce          = true
```

| Key | Meaning |
| --- | --- |
| `require_approval` | `remote` (includes, installed skills, skill sources and served skills that come from a remote), `local` (every authored item), `all` (both), `kind:<kind>` (for example `kind:hook`, `kind:skill`). `kind:mcp_server` selects the pinned MCP server declarations |
| `exempt` | Globs over `kind:id` or `kind:domain/id`; `*` matches any run of characters, `?` one. An exempt item never needs approval |
| `min_approvers` | Distinct reviewers needed for one digest. Default 1; one reviewer twice counts once |
| `approvers` | Allowlist of reviewer strings, matched lower-cased and trimmed. A typo guard and an audit aid for `asserted` records: it is not authentication (see below), unless an [organization policy](policy.md) pins the list |
| `max_age` | Default lifetime of a new approval: `365d` or a Go duration such as `720h` |
| `enforce` | Make `lock --check`, `generate --locked` and `mcp --serve-skills` fail. `validate --strict` reports regardless |

`[governance]` is shared policy: a machine-local config overlay cannot set or relax it (the key is ignored with a
warning). An [organization policy](policy.md) can set a floor under it: `enforce` on, `require_approval` selectors the
repository's `exempt` cannot narrow, a minimum `min_approvers`, and the only `approvers` that count. A repository that
loosens one is clamped and reported as `AR740`.

Under `enforce` the check never fails open: a missing `ai-rulez.lock`, or one that pins no content, is itself `AR710`
in `lock --check`, `generate --locked`, `validate --strict` and the skills server (`mcp --serve-skills`). Without `enforce` nothing is refused. A record that claims an `assurance` other than `asserted` (a
forged `signed`) does not count.

## Where it is enforced

| Where | Behaviour |
| --- | --- |
| `validate --strict` | AR710 to AR715 at their default severities, `enforce` or not |
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
| `AR716` | `approval-with-change` | An approval was added in the same change as the content it approves (only with `approve --verify-base` or `validate --strict --approvals-base`) |

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
touches `[[approval]]` or `[governance]` as sensitive. Approvals are not yet part of the signed lock subject
(`approvals_digest` stays empty, see [Signing the lock](lockfile.md#signing-the-lock)), and there are no signed or
review-linked assurance levels yet.

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
- **Assurance.** Only `asserted` is written. `review-linked`, `signed`, CODEOWNERS and forge lookups are later phases.
- **Remote is decided by the lock.** A served skill counts as remote when its lock entry names a ref or commit or a git
  URL source; a skill authored in the project is only selected by `kind:served`.
- **Diff.** `approve --diff` lists files, findings and the previous approval; it does not print a text diff because
  the lock keeps digests, not the previously approved bytes. Use `git diff` and `lock --diff` for the text.
- **Reviewer default.** Read from `$AI_RULEZ_REVIEWER` or the `user.email` of the git configuration files, without
  running git.
- **Time.** Expiry compares the UTC date of the wall clock; `SOURCE_DATE_EPOCH` only stamps `approved_at`.
- **Self-review.** `AR716` is decided from lock pins at a git revision, not from commit authors, so it works with
  squash merges and needs no network.
- **Not done yet.** Deny list (`AR717`), CODEOWNERS and team mapping, forge review links,
  signed approvals, `approvals_digest` in the lock subject, an approval column in the catalog site.
