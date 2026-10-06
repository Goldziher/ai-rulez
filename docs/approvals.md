# Approvals

A scan says content looks clean; an approval says a named person read it and accepted it. `ai-rulez approve` records
that in [`ai-rulez.lock`](lockfile.md), bound to the exact content digest, and `[governance]` makes chosen content
unusable until it is approved. Nothing here judges content: an approval is a human assertion, never a safety proof.

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
  has no identity notion. Use a handle if you do not want an email in a committed file.
- **Expiry.** `--expires YYYY-MM-DD`, else today plus `[governance] max_age`, else none. An approval holds through its
  expiry date.
- **Re-approving** replaces that reviewer's earlier record of the item; other reviewers' records stay until they approve
  again. `--revoke <item>` removes the records of an item (`--reviewer` limits it); `--prune` removes records of content
  that no longer exists or whose digest changed.
- **Time.** `approved_at` is stored once and re-emitted byte for byte when the lock is rewritten, so `ai-rulez lock`
  never makes an approval diff. `--at` and `SOURCE_DATE_EPOCH` make a run reproducible.
- **Output safety.** Everything printed from content (paths, findings, notes) has control, bidirectional and
  zero-width characters replaced by `\u{XXXX}`, so a diff cannot hide text from the reviewer. `--note` is scanned for
  secrets (AR001) and refused when it holds one, because the lock is committed.

## When an approval stops applying

An approval is bound to one digest, the same digest the lock pins ([hashing scheme](lockfile.md#hashing-scheme)). So it
goes **stale** on any change to the bytes of the item or of any file its digest covers (a skill's scripts and
resources, a hook's script, the remote tree), and on an executable-bit flip. A CRLF-only edit does not change the
digest and never stales an approval. A remote source that moves to a new commit with an identical tree keeps its
approval. `ai-rulez lock` keeps every approval: stale ones stay until the content is approved again or `--prune` removes
them; a full re-lock drops, with a warning (`AR715`), approvals of content that no longer exists. A renamed item must
be approved again under its new name.

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
| `approvers` | Allowlist of reviewer strings, matched exactly. It is a typo guard and an audit aid for `asserted` records, not security |
| `max_age` | Default lifetime of a new approval: `365d` or a Go duration such as `720h` |
| `enforce` | Make `lock --check`, `generate --locked` and `mcp --serve-skills` fail. `validate --strict` reports regardless |

`[governance]` is shared policy: a machine-local config overlay cannot set or relax it (the key is ignored with a
warning).

## Where it is enforced

| Where | Behaviour |
| --- | --- |
| `validate --strict` | AR710 to AR715 at their default severities, `enforce` or not |
| `lock --check`, `generate --locked` | With `enforce = true`, exit 2 for content whose approval is missing, stale, expired, unauthorized or insufficient. `lock --diff --format json` lists them with `scope: "approval"` (`schema/lock-diff.schema.json`). Without `enforce` the diff only carries a note |
| `mcp --serve-skills` | With `enforce = true`, a served skill the policy selects (`remote`, `all`, `kind:served`) and that lacks a valid approval of its served digest is refused, with the AR71x code. Provenance gains `approved` and `approvers`. Without a lock nothing is checked (`lock` itself builds without one) |
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

## What this does not protect against

An `asserted` approval is a string in a committed file. Whoever can edit the lock can add one, so its strength is
that of the review of the lock change: protect the lock path with branch protection and code owners, and treat a
pull request that touches `[[approval]]` or `[governance]` as sensitive. Approvals are not yet part of the signed
lock subject (`approvals_digest` stays empty, see [Signing the lock](lockfile.md#signing-the-lock)), and there are no
signed or review-linked assurance levels yet.

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
- **Time.** Expiry compares the UTC date; `SOURCE_DATE_EPOCH` sets "now".
- **Not done yet.** Deny list (`AR717`), self-review check (`AR716`), CODEOWNERS and team mapping, forge review links,
  signed approvals, `approvals_digest` in the lock subject, an approval column in the catalog site.
