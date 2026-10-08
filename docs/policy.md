# Organization policy

An organization policy is a floor a repository may raise but never lower. A security team writes it once; every
repository's `ai-rulez validate` and `ai-rulez generate` enforce it. Without a policy nothing changes.

The point of the design: a policy that lives in the repository it governs can be deleted by the pull request that
violates it. So a policy is found only **outside** the repository (a flag, an environment variable, a managed path),
never inside it. The repository can add restrictions; it cannot remove one.

## Where a policy comes from

Layers, strongest anchor first. Every layer found is loaded and merged tighten-only (there is no "override").

| Layer | Source | Use |
| --- | --- | --- |
| `flag` | `--policy <file or https URL>` | a CI step the organization owns |
| `env` | `AI_RULEZ_POLICY=<file or https URL>` | managed machines, CI images |
| `org` | the policy of the repository's GitHub owner, with `--discover-org` or `[policy] discover = "org"` in the user config | a convenience for an organization's repositories (see below; not an anchor) |
| `managed` | `/etc/ai-rulez/policy.toml` (Linux and others), `/Library/Application Support/ai-rulez/policy.toml` (macOS), `%ProgramData%\ai-rulez\policy.toml` (Windows) | a machine-wide baseline |

A flag or variable that is set but cannot be loaded is an error (`AR742`), never a skip: breaking the path must not
switch the policy off. An absent managed file is not an error; a present but unreadable or invalid one is. A policy
file must be a regular file of at most 256 KiB. The same file named twice is one layer.

### Managed locations

The managed file is `/etc/ai-rulez/policy.toml` on Linux and other Unix systems,
`/Library/Application Support/ai-rulez/policy.toml` on macOS and `%ProgramData%\ai-rulez\policy.toml` on Windows
(`C:\ProgramData` when `ProgramData` is not an absolute path: a relative value would resolve against the repository
being evaluated). Make the file writable by administrators only. Apart from the `ProgramData` system variable on
Windows (keep it out of repository-controlled environment files), nothing moves it: a flag or variable a repository could
set would let it replace the managed anchor with an empty file. (The end-to-end tests build a binary with the location
moved by a link-time variable; a release binary never sets it.)

### Policy URLs

A policy may be an `https` URL, so one file serves every repository. The rules are strict, because a URL is
content someone else controls:

- **A digest is required.** Pin it on the reference (`https://policy.example.org/base.toml@sha256:<hex>`), with
  `--policy-digest` for `--policy`, or `AI_RULEZ_POLICY_DIGEST` for `AI_RULEZ_POLICY`. The digest is the SHA-256 of
  the file with CRLF normalized to LF, the one `validate --show-policy` prints. A URL with no digest is not loaded
  (`AR741`). A digest that does not match is `AR741` and fails closed; a cached copy never papers over it.
- **`--policy-trust-tofu`** records the digest of an unpinned URL once, and only in a terminal (a pipe or CI refuses),
  in the user cache with an HMAC. Later runs use the recorded digest. The warning it prints names the digest to pin.
- **https only**, no credentials in the URL (`file:` and `http:` are refused), no credentials sent, the response is at
  most 256 KiB, redirects stay on the same host over https (at most 5), and the content type is ignored.
- **Cache.** A good copy is stored by URL and digest in the user cache (`~/.cache/ai-rulez/policy`), with an HMAC under
  a per-user secret kept in the user config directory. An entry that fails its HMAC, or whose body no longer hashes to
  the pin, is a miss, so a checkout or restored cache cannot plant a policy.
- **Offline and `max_stale`.** When the URL cannot be reached (any network error or non-200 answer), the cached copy
  of the pinned digest stands in for it for at most `max_stale` (default `7d`: `--policy-max-stale`, or
  `AI_RULEZ_POLICY_MAX_STALE`; `0` allows none). `--policy-offline` (or `AI_RULEZ_POLICY_OFFLINE=1`) uses the cache
  without asking the network. Past `max_stale`, with no cached copy, or with a copy stamped more than five minutes in
  the future (fetched while the clock ran ahead), the run fails with `AR742`. There is no
  "skip the policy because it is unreachable". `validate --show-policy` marks a layer served from the cache.

### Organization discovery

`--discover-org` (or `[policy] discover = "org"` in the user config, `~/.config/ai-rulez/config.toml`) also loads
`ai-rulez-policy.toml` from the root of the `<owner>/.github` repository, where `<owner>` is the GitHub owner of the
repository's `origin` remote (`https://github.com/<owner>/...`, `git@github.com:<owner>/...`, `ssh://`). It is
fetched from `raw.githubusercontent.com` (public `.github` repositories only) and is the weakest layer. It is the
same as any policy URL: a digest is required, from `[policy.digests]` in the user config or recorded once with
`--policy-trust-tofu`, never from the repository.

```toml
# ~/.config/ai-rulez/config.toml  (the user config, outside every repository)
[policy]
discover = "org"

[policy.digests]
example-org = "sha256:<hex>"
```

An owner with no policy file (HTTP 404) has no organization policy only while nothing anchors one: a pinned digest, a
digest recorded by `--policy-trust-tofu` or a required signature turns a 404 into `AR742`, like any other failure
(a withdrawn policy is not "no policy"), with
the cached copy standing in for at most `max_stale`. The owner is fetched once per run however many repositories
share it. A remote that is not on GitHub, or no remote at all, skips discovery with a warning, unless
`--discover-org` was given on the command line, which then fails closed.

Discovery is a **convenience, not an anchor**. The owner comes from the repository's own git configuration, so a
pull request can point `origin` at an owner whose policy is looser. A looser org policy cannot loosen anything (layers
only add restrictions), but discovery alone does not bind a repository either. Enforce with `--policy`,
`AI_RULEZ_POLICY` or the managed path, and use discovery to add the owner's policy on top. The `telemetry` and `llm`
network locks read only the anchored layers, since they apply before any repository is known.

### Signed policies

A signature lets an organization publish a policy without every machine pinning its digest. Publish the Sigstore
bundle next to the policy, as `<policy>.sigstore.json` (for a URL, at the same URL plus the suffix). Two forms verify:

- a DSSE attestation over a policy statement (predicate `https://github.com/Goldziher/ai-rulez/attestations/policy/v1`,
  subject `policy.toml` with the policy's digest), made with `ai-rulez sign --policy <file> --key <key>` (or `--keyless`; it refuses a file the loader would reject, and writes the bundle next to it), and
- a `cosign sign-blob --bundle policy.toml.sigstore.json policy.toml` message signature over the file's exact bytes.

Who may sign comes from outside the repository, like the policy: `--policy-signer-key <pem>` (repeatable),
`--policy-signer-identity` with `--policy-signer-issuer`, `--policy-trusted-root`, the matching
`AI_RULEZ_POLICY_SIGNER_*` and `AI_RULEZ_POLICY_TRUSTED_ROOT` variables, or the user config:

```toml
# ~/.config/ai-rulez/config.toml
[policy]
require_signature = true
tlog = "required"            # required (default with an identity signer), optional or off (default with keys only)

[[policy.signers]]
identity = "https://github.com/example-org/policy/.github/workflows/sign.yml@refs/heads/main"
issuer   = "https://token.actions.githubusercontent.com"

[[policy.signers]]
key_file = "/etc/ai-rulez/policy-signer.pub"
```

The sources are merged: every one adds trusted signers and any can set the requirement. The signer is checked for the
subject `policy` with the same machinery as the lock attestation ([Signing](signing.md)): signature, certificate chain
and log proof, signer allowed, statement covering this policy (`AR724`), and a per-user rollback mark so an older signed
policy cannot be replayed (`AR727`). Every failure is `AR746` and fails closed.

- With a trusted signer configured, a signature that is present must verify even when a digest pin matches. An
  unsigned policy loads on its pin alone unless `--policy-require-signed` (or `require_signature`) says otherwise.
- A **URL policy with a verified signature needs no digest**: the signature vouches for the content. Without a
  signature the missing pin is the error (`AR741`).
- The cache keeps the bundle with the body, and a cached copy is verified again on every load. An unreachable
  signature fails closed when signatures are required or the policy is not pinned; otherwise the pin vouches for it.
- Policies reached by `extends` and the organization policy are checked the same way, one by one.
- Signatures are not looked at when no trusted signer is configured and none is required.

### `extends`

A policy may extend others, so a team policy can build on the organization's:

```toml
policy_version = 1
name = "payments team"
extends = ["org.toml", "https://policy.example.org/base.toml@sha256:<hex>"]   # relative to this file, or pinned URLs
```

Each parent is a layer of its own (origin `extends`, listed after the policy that names it) and everything folds
tighten-only, so an extending policy can only add restrictions. Each hop must itself tighten its parent: a value the
child states that is weaker than what it extends (a lower severity floor, an allowlist entry the parent does not cover,
a higher budget, a shorter release age, a weaker scan level, ...) is `AR743`, because the fold would ignore it and the
author would believe it took effect. Switches are compared by what the child wrote: `require_pinned = false`,
`[lock] enforce = false`, `include_outputs = false`, `[guard] generated = false`, `[governance] enforce = false` or
`forbid_self_approval = false`, `[hooks] allow = true`, and `allow_network = true` under `[telemetry]` or `[llm]` are
loosenings when the parent turns the restriction on; `false` next to a parent that left it off is not. A key the child
does not mention is not a loosening.

Limits: at most 8 entries per `extends`, 5 hops deep, 32 policies in one load, no cycles (the cycle is named in the
error). A URL parent needs its `@sha256:<hex>` pin (no trust-on-first-use inside a chain); a policy fetched from a URL
cannot extend a local file. A parent that cannot be loaded fails the whole load closed (`AR742`). A policy reached
twice (a diamond, or an anchor that is also a parent) is one layer.

Policy is never read from the repository or from `config.local.*`. Set `AI_RULEZ_POLICY` from the trusted side
(organization secrets and variables, a required workflow), not from a repository-level workflow file that a fork pull
request can edit.

## The policy file

TOML, data only. An unknown key is an error (`AR743`) because a typo must not silently loosen the policy.

```toml
# ai-rulez-policy.toml
policy_version = 1
name = "example-org baseline"

[sources]
allowed_hosts  = ["github.com/example-org", "*.example.org"]   # includes, installed skills, skill sources
deny_hosts     = ["github.com/example-org/archived"]
require_pinned = true                                           # the lock is enforced; an unpinned remote is an error
min_release_age = "7d"                                          # the youngest tag any source may adopt
min_release_age_source = "first-seen"                           # release time from the forge or first sighting, never a committer's date
deny_digests    = ["sha256:0000000000000000000000000000000000000000000000000000000000000000"]  # known-bad content, blocked everywhere

[lint]
required_codes = ["AR001", "AR005", "AR008"]                    # may be neither turned off nor ignored

no_inline_ignore = ["AR001"]                                    # ai-rulez-lint-ignore comments are not honored for these

[lint.severity_floor]
AR001 = "error"
AR008 = "warning"

[lint.max_findings]
AR005 = 0                                                       # more than this many findings of a code is AR749

[lint.security]
allowed_hosts = ["github.com", "*.example.org"]                 # bounds the repository's own list
scan_imports  = "error"
directive_tags = ["assistant"]                                  # always checked by AR018; the repository may add more
trusted_orgs   = ["anthropics", "github"]                       # the repository may name only these

[lint.scanner_policy]
preset = "baseline"                                             # the weakest scanner preset; the repository may use a stronger one
required = ["agnix"]                                            # always required; the repository's own list is added
fail_on = "warning"                                             # the most permissive threshold for scanner findings
isolation = "require"                                           # the weakest isolation of staged scanners
allow_egress = []                                               # the scanners --allow-egress may enable; [] forbids every egress scanner

[lint.capability]
max_network_commands = 3                                        # AR030 limit; the repository may set a lower one

[lint.budgets.skill]
max_tokens = 4000                                               # AR902 limit per content kind; the repository may set a lower one

[lint.load_budgets]
claude-skill-listing = 1200                                     # AR964 limit by id; the repository may set a lower one

[lock]
enforce         = true
include_outputs = true

[telemetry]
allow_network = false                                           # export off, whatever the user scope says

[llm]
allow_network = false

[guard]
generated = true

[governance]
enforce          = true                                         # approvals are enforced, whatever the repository says
require_approval = ["remote", "kind:hook"]                      # always required; the repository's exempt cannot narrow it
min_approvers    = 2
approvers        = ["alice@example.org", "bob@example.org"]     # only these reviewers count
min_assurance    = "review-linked"                              # an asserted approval does not count
forbid_self_approval = true                                     # an author cannot approve their own change
approvers_from   = "CODEOWNERS"                                 # approvers must also own the item's path (the repository picks the file)

[signing]
require_verified = ["lock"]                                     # the lock must carry a verified attestation
tlog             = "required"                                   # the weakest transparency-log mode
max_age          = "180d"                                       # the oldest signature the repository may accept
min_hash_version = 1
allow_repo_identities = false                                   # the repository may trust only the signers below

[signing.thresholds]
lock = 2                                                        # at least two distinct signers must have signed the lock

[[signing.trust]]
subject  = "lock"
identity = "https://github.com/example-org/ai-config/.github/workflows/release.yml@refs/heads/main"
issuer   = "https://token.actions.githubusercontent.com"

[mcp]
allowed_commands = ["npx", "uvx"]                               # the only commands a stdio MCP server may run
deny_transports  = ["http", "sse"]                              # no remote MCP endpoint

[hooks]
allow = false                                                   # any [[hooks]] group is a violation
```

Rule codes may be written as codes (`AR001`) or names (`secret-detected`). A code this ai-rulez does not know is an
error: the policy is newer than the binary, and the message asks to upgrade. `policy_version = 1` is required; a
higher number is refused.

### Host patterns

A pattern is a host, optionally followed by a path prefix, compared case-insensitively:

| Pattern | Matches |
| --- | --- |
| `github.com` | the host, any path |
| `*.example.org` | `example.org` and every subdomain (the same rule as `lint.security.allowed_hosts`) |
| `github.com/example-org` | any path under the segment `example-org`, not `example-org2` |
| `github.com/example-*` | a segment may carry one `*` at its start or end |
| `github.com/*/rules` | `*` is exactly one segment |

Matching is by path prefix, so a trailing `/**` means the same as leaving it out. A bare `*` host is refused. A source
is read from its URL: `https://github.com/example-org/rules.git`, `git@github.com:example-org/rules.git` and
`ssh://git@host:2222/org/repo` all give a host and a path. A local path names no host and is not governed by
`sources`.

`lint.security.allowed_hosts` lists hosts only (no path), because it bounds the hosts URLs inside content may point
to.

## How a repository is bounded

Each key has one direction. The policy value is **enforced** (clamped) and any attempt to loosen it is **reported**.

| Key | Effective value | A repository loosens it by | Code |
| --- | --- | --- | --- |
| `sources.allowed_hosts` | sources the list covers | a source from a host the list does not cover; the source is not loaded | `AR745` |
| `sources.deny_hosts` | union of layers | a source from a denied host; the source is not loaded | `AR745` |
| `sources.deny_digests` | union of layers; the organization's deny list, the counterpart of the lock's `[[deny]]` ([Approvals](approvals.md#deny-list)); both block, so a digest on either is denied | a include, installed skill or skill source whose pinned digest is listed is not loaded; a listed authored item is reported (`ai-rulez.lock` names the digests, so keep the lock enforced) | `AR747` |
| `sources.require_pinned`, `lock.enforce` | the lock is enforced | `[lock] enforce = false` | `AR740` |
| `lock.include_outputs` | output digests are pinned | `[lock] include_outputs = false` | `AR740` |
| `lint.required_codes` | union of layers | `[lint.severity] CODE = "off"` or `[lint] ignore` | `AR744` |
| `lint.severity_floor` | the higher severity of the layers | a lower `[lint.severity]`, or `[lint] ignore` of a floored code | `AR740` |
| `lint.no_inline_ignore` | union of layers | an `ai-rulez-lint-ignore` comment for a listed code; the finding is still reported | `AR740` |
| `lint.max_findings.<code>` | the lower ceiling per code (`0` allows none). The code is protected: baselines, `[lint.ratchet]` and ignores do not apply | (a ceiling is not a repository key; going over it is reported) | `AR749` |
| `lint.security.allowed_hosts` | repository entries the list covers; the policy list when it sets none | an entry the list does not provably cover; the entry is dropped | `AR740` |
| `lint.security.scan_imports` | the stricter level (`off` < `warn` < unset < `error`) | a weaker explicit level | `AR740` |
| `lint.security.directive_tags` | union of layers, then the repository's own tags | (nothing to report: the repository's list only adds) | none |
| `lint.security.trusted_orgs` | the repository's entries the list names; the policy list when it sets none or none is left. An empty policy list trusts no organization | an entry the list does not name; the entry is dropped | `AR740` |
| `lint.capability.max_network_commands` | the lower value; an unset repository value is the lower of the policy bound and the built-in 5 | a higher explicit value | `AR740` |
| `sources.min_release_age` | the longer age; an unset `[lock]` or per-source age takes it | a younger `[lock] min_release_age` or per-source age, including `"0"` | `AR740` |
| `sources.min_release_age_source` | the stronger of `forge` and `first-seen`; an unset `[lock]` source takes `forge`, and keeps `auto` under `first-seen` | a weaker `[lock] min_release_age_source`: `commit`, and `auto` or `first-seen` under `forge` (`auto` can fall back to `first-seen`) | `AR740` |
| `lint.budgets.<kind>.max_lines`, `.max_tokens` | the lower value per kind and field; an unset one is the lower of the policy bound and the built-in budget | a higher explicit `[lint.budgets.<kind>]` value | `AR740` |
| `lint.load_budgets.<id>` | the lower value per id; an unset one is the lower of the policy bound and the built-in limit | a higher explicit value | `AR740` |
| `lint.scanner_policy.preset` | the stronger preset (`off` < `baseline` < `strict`) | a weaker explicit preset (an unset one takes the policy's) | `AR740` |
| `lint.scanner_policy.required` | union with the repository's list | (nothing to report: the repository's list only adds) | none |
| `lint.scanner_policy.fail_on` | the stricter threshold (`error` < `warning` < `info`) | a looser explicit threshold | `AR740` |
| `lint.scanner_policy.isolation` | the stronger level (`none` < `auto` < `require`) | a weaker explicit level | `AR740` |
| `lint.scanner_policy.allow_egress` | the repository's entries the policy names; the policy list when it sets none. An empty policy list allows no scanner to send content off the machine | an entry the list does not name; the entry is dropped | `AR740` |
| `telemetry.allow_network`, `llm.allow_network` | `false` | `allow_network = true` in the repository (already ignored by the trust rule, now also reported) | `AR740` |
| `guard.generated` | `true` | a `[guard]` table without `generated = true` | `AR740` |
| `governance.enforce` | `true` | a `[governance]` table without `enforce = true` | `AR740` |
| `governance.require_approval` | union with the repository's selectors; the policy's are not narrowed by `exempt` | (nothing to report: `exempt` is simply not applied to them) | none |
| `governance.min_approvers` | the larger value | a lower explicit `min_approvers` | `AR740` |
| `governance.min_assurance` | the stronger level (`asserted` < `review-linked` < `signed`) | a weaker explicit `min_assurance` | `AR740` |
| `governance.forbid_self_approval` | `true` | a `[governance]` table without `forbid_self_approval = true` | `AR740` |
| `governance.approvers_from` | the repository's own value, or `"CODEOWNERS"` when it has none; a policy cannot name a path | a `[governance]` table without `approvers_from` | `AR740` |
| `signing.require_verified` | union with the repository's `[signing] require` | (nothing to report: the repository's list only adds) | none |
| `signing.tlog` | the stricter mode (`off` < `optional` < `required`) | a weaker explicit `[signing] tlog` | `AR740` |
| `signing.max_age` | the shorter age; an unset one takes the policy's | a longer explicit `[signing] max_age` | `AR740` |
| `signing.min_hash_version` | the larger value | a lower explicit `[signing] min_hash_version` | `AR740` |
| `signing.trust`, `signing.allow_repo_identities` | per subject (`lock`, `bundle`, `skill`, `sbom`, `approval`), for the subjects the list names: the repository's signers the list names (compared on subject, identity or `identity_regexp`, and issuer); the policy's entries for the subject when none is left. Entries for subjects the list does not name are not touched. An empty list (or `allow_repo_identities = false` alone) governs every subject and trusts nobody, so verification fails closed. A `key_file` is never in the list | a signer the list does not name for a governed subject, including any `key_file` or shorthand `identity`; the entry is dropped | `AR740` |
| `signing.thresholds` | per subject, the larger value; a subject the repository leaves unset takes the floor | a lower explicit `[signing.thresholds]` value | `AR740` |
| `mcp.allowed_commands` | the stdio servers whose command the list names (compared as written); an empty list allows none | an MCP server running another command; the server is not loaded. An imported agent, skill or command with such an inline `mcpServers` entry loses the whole `mcpServers` key | `AR748` |
| `mcp.deny_transports` | union of layers | an MCP server on a denied transport (`stdio`, `http`, `sse`); the server is not loaded (inline `mcpServers` of imported content: as above) | `AR748` |
| `hooks.allow` | `false` forbids every hook group | a `[[hooks]]` group; it is not loaded. An imported agent, skill or command that declares `hooks` in its frontmatter loses the key | `AR748` |
| `governance.approvers` | the repository's entries that the list names; the policy list when it sets none or none is left. An empty policy list means nobody may approve | an entry the list does not name; the entry is dropped | `AR740` |

`[governance]` matters because approvals are records in the repository's own lock: without a floor the repository
could set `approvers`, `min_approvers`, `exempt` or `enforce` to whatever lets its change through. Reviewers are
compared lower-cased and trimmed. Even with the floor an `asserted` approval is not authentication; see
[Approvals](approvals.md#approvals-are-assertions) for the CI check that makes the floor meaningful.

A repository that does not touch a key is not reported: the policy value is just used. `AR740` and the other policy
codes are always errors; `[lint.severity]` cannot soften them and `[lint] ignore` cannot hide them. The telemetry
and LLM locks hold in user scope and the environment too, since the point is to stop a machine from enabling what the
organization forbids. A policy that cannot be loaded locks both.

### Merging layers

`Merge` combines two policies into the tighter one, key by key: allowlists intersect, denylists and required sets
union, severities and scan levels take the stricter value, upper bounds (`max_network_commands`, `load_budgets`) take the
lower value, and switches turn on. It is commutative, idempotent and
associative, and an empty policy is its identity, so the order of layers does not matter beyond naming the origin.
The intersection of two allowlists keeps an entry of one list when an entry of the other covers it.

`covers(policy, repo)` decides whether a repository pattern is inside a policy pattern. It is segment-based and
conservative: when it cannot prove that every location the repository pattern matches is also matched by the policy
pattern, the answer is no. A false reject is safe; a false accept is not (a fuzz test checks this against a brute-force
universe).

## Rolling a policy out: `--policy-mode warn`

`--policy-mode warn` lets a policy land before every repository complies. The policy values are still enforced (the
clamp is unchanged), but each loosening attempt is reported as a warning instead of an error: the `AR74x` findings
become warnings, `generate` and `validate` log the violations and carry on, `validate --show-policy` exits `0` and
prints `mode: warn`, and the JSON report has `"mode": "warn"`. Ai-rulez logs a warning at load so the mode is never
silent. An unavailable or invalid policy (`AR741`-`AR743`) still fails closed, so `warn` can only relax how a
violation is judged, never whether the policy is loaded. Set it on the trusted side, with the policy itself: a
repository cannot set it.

## `validate --show-policy`

```console
$ ai-rulez validate --show-policy
policy: 2 layers
  env       /ci/team.toml              sha256:a90b00000000…  (team)
  managed   /etc/ai-rulez/policy.toml  sha256:5c1e00000000…  (example-org baseline)
effective (origin in brackets)
  lint.severity_floor.AR008 = error          [env]
  lock.enforce              = true           [env]
  sources.allowed_hosts     = ["github.com/example-org"] [managed]
repo overrides accepted: lint.security.allowed_hosts (narrowed to ["github.com"])
repo overrides rejected: 1
  AR740 .ai-rulez/config.toml:12  [lint.severity] AR008 = "warning" is below the policy floor "error" (origin: env); "error" is enforced
```

`--format json` writes the same report (`schema/policy-effective.schema.json`): `layers[{origin, source, name,
digest}]`, `effective`, `provenance` (dotted key to the layer, joined with `+` when several layers contribute to a
set), `overrides{accepted, rejected}` and `violations[]`. The digest is the SHA-256 of the file with CRLF normalized
to LF. Exit code 2 when the repository loosens the policy (the same code `generate` and `validate` give). The policy file format is
`schema/policy.schema.json`.

## What honors the policy

- `generate`, `validate` and `validate --show-policy`: the clamp is applied at load. A configuration that loosens the
  policy exits `2` from `generate`, from `validate` (with or without `--strict` or `--config-only`) and from
  `validate --show-policy`; `validate --strict` reports each attempt as an `AR74x` finding. The severity floor and
  required codes are enforced inside the lint run, so a floored finding cannot be demoted by a lint profile.
- `generate` refuses to run on a configuration that loosens the policy, and so does each root of `generate
  --recursive` and `validate --recursive`. Loosening is exit `2` everywhere: when every failed root loosened the
  policy (or drifted from the lock) a recursive run exits `2`, and `1` when some root failed for another reason.
- `lock` refuses to write `ai-rulez.lock` for such a configuration (exit `2`, naming the `AR740` lines), and
  `lock --check` reports a mismatch (exit `2`). `doctor` and `doctor --policy` report each attempt as an `AR740`
  error finding in the `policy` check (a warning under `--policy-mode warn`) and exit `2`.
- The MCP servers: `mcp --serve-skills` fails to start on a configuration that loosens the policy, and the
  `generate_outputs` and `validate_config` tools of `mcp` refuse it (`validate_config` answers `valid: false`).
- No suppression hides a protected code: for every code in `required_codes` or raised by `severity_floor` (and
  AR740-AR745 themselves), `ai-rulez-lint-ignore` comments, `[lint] ignore_paths`, baseline entries and
  `[lint.ratchet]` budgets are not applied. The finding stays at its enforced severity and counts toward the exit
  code, and one AR740 finding per route names the attempt (`[lint] ignore_paths`, `ai-rulez-lint-ignore comment`,
  `baseline`, `[lint.ratchet]`) and the codes it tried to hide.
- Remote sources a policy refuses are dropped before anything is fetched.

## Design decisions

- **Discovery reads the owner from `origin`.** A repository can change its own remote, so the org layer is additive
  only. The design asked whether discovering from the git remote is acceptable at all (it is an implicit fetch):
  it is opt-in per user or per invocation, pinned by digest, and fails closed once demanded.
- **`deny_digests` reports `AR747`, the lock's `[[deny]]` reports `AR717`.** They are two lists with one effect: the
  content is blocked. `AR717` is the repository's own list in `ai-rulez.lock` (`approve --revoke --deny`); `AR747` is the
  organization's list, which the repository cannot edit or remove an entry from. The codes stay apart so a finding says
  whose list it came from. The policy list is checked wherever the policy is applied (`generate`, `validate`, `lock
  --check`), not by `approve`: approving a digest the policy denies is possible but changes nothing, because
  generation still refuses it. The lock is the source of the digests, which is why a repository without a lock has
  nothing to check.
- **Governance floors.** `min_assurance`, `forbid_self_approval` and `approvers_from` are floors like `min_approvers`.
  Without them a repository could lower its own assurance in the pull request that needs the approval. They are
  assertions about the lock and CODEOWNERS the repository itself carries, so pair them with branch protection (see
  [Approvals](approvals.md)); the policy cannot name a CODEOWNERS path, only demand that one is used.
- **Signing trust is per subject.** `[[signing.trust]]` rows may name any trust subject. The list clamps only the
  subjects it names (all of them when empty), so a policy that lists lock signers does not silently drop the
  repository's skill, bundle, sbom or approval signers. `[signing.thresholds]` is a per-subject floor on k of n.
- **`extends` is checked, not just folded.** Merging already makes the chain at least as strict as each parent, so a
  looser child value would be silently dropped. Reporting it (`AR743`) is the safer choice: a team lead who writes a
  weaker value learns it does nothing.
- **URL policies.** `AR741` is a missing or mismatched digest; a stale or absent cache after a failed fetch is `AR742`.
  Trust-on-first-use is terminal-only so that nobody can script past the review of a digest.
- **Rollback of a signed policy.** A signature with a signing time (a keyless bundle, a log entry or the policy
  attestation's `issued_at`) is checked against the newest time this machine verified (`AR727`) and against clock
  skew. A key-signed `cosign sign-blob` bundle without a log entry has no time: the machine instead remembers the
  bodies a newer one replaced and refuses them (`AR727`). That covers the versions this machine has seen, like the time
  check; a fresh machine starts empty, and skew cannot be judged.
- **Fail closed everywhere.** A demanded policy that cannot be read is `AR742`, and an unusable policy locks
  telemetry and LLM network use.
- **An explicit empty allowlist means nothing is allowed.** `lint.security.allowed_hosts = []` in a policy is
  enforced as a single host that never resolves (`policy.invalid`), because an empty list in `config.toml` switches
  the host check off.
- **Unset is not a violation.** A repository that leaves a key alone gets the policy value silently; only an explicit
  weaker value is reported.
- **`[guard]`.** The config type cannot tell an explicit `generated = false` from an unset table, so the guard is
  reported only when the repository wrote a `[guard]` table without `generated = true`.
- **Provenance** names the strongest layer that decides a key (a tie goes to the stronger anchor), and every
  contributing layer for a set.
- **`max_findings` is a gate, not a forgiveness.** Up to the ceiling the findings keep their own severity; one more is
  `AR749`. The code is protected, so baselines, `[lint.ratchet]` and ignores cannot absorb it. The design's
  `[lint.size] max_tokens` is the existing `[lint.budgets.<kind>]` table here.
- **Imported hooks and MCP servers.** MCP servers cannot be imported from an include, but an agent, skill or command
  an include or an installed skill delivers can declare `hooks` and `mcpServers` in its frontmatter. `[hooks]` and
  `[mcp]` bound those too, once the content is loaded: a file outside the project's own configuration directory (or
  inside a local include's directory) is imported. A violation unloads the key and is `AR748`, reported against the
  imported file. Every spelling of the key counts (`mcpServers`, `mcp-servers`, `mcp_servers`, any letter case): the
  renderers that pass these keys through to a harness and the policy read one registry of the keys that execute or
  connect. Builtin packs are not bounded (they ship with the binary).
- **Every command is bound.** Each command's loads, CRUD operations and MCP setup run under the policy, and so do the
  configurations `generate` loads itself (monorepo members, the shared view for `roles.json`, the drift baseline).
  `catalog diff` and `approve` load earlier revisions from a temporary directory, but `--discover-org` still reads the
  remote of the real project.
- **Known gaps.** A policy value is not checked against the secret scanner yet. `deny_digests`
  bounds what the repository's own `ai-rulez.lock` says.
- **No policy, no change.** Without a flag, variable or managed file the loader does nothing (tested).
