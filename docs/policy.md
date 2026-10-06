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
| `flag` | `--policy <file>` | a CI step the organization owns |
| `env` | `AI_RULEZ_POLICY=<file>` | managed machines, CI images |
| `managed` | `/etc/ai-rulez/policy.toml` (Linux and others), `/Library/Application Support/ai-rulez/policy.toml` (macOS), `%ProgramData%\ai-rulez\policy.toml` (Windows) | a machine-wide baseline |

A flag or variable that is set but cannot be loaded is an error (`AR742`), never a skip: breaking the path must not
switch the policy off. An absent managed file is not an error; a present but unreadable or invalid one is. A policy
file must be a regular file of at most 256 KiB. The same file named twice is one layer.

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

[lint]
required_codes = ["AR001", "AR005", "AR008"]                    # may be neither turned off nor ignored

[lint.severity_floor]
AR001 = "error"
AR008 = "warning"

[lint.security]
allowed_hosts = ["github.com", "*.example.org"]                 # bounds the repository's own list
scan_imports  = "error"

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
| `sources.require_pinned`, `lock.enforce` | the lock is enforced | `[lock] enforce = false` | `AR740` |
| `lock.include_outputs` | output digests are pinned | `[lock] include_outputs = false` | `AR740` |
| `lint.required_codes` | union of layers | `[lint.severity] CODE = "off"` or `[lint] ignore` | `AR744` |
| `lint.severity_floor` | the higher severity of the layers | a lower `[lint.severity]`, or `[lint] ignore` of a floored code | `AR740` |
| `lint.security.allowed_hosts` | repository entries the list covers; the policy list when it sets none | an entry the list does not provably cover; the entry is dropped | `AR740` |
| `lint.security.scan_imports` | the stricter level (`off` < `warn` < unset < `error`) | a weaker explicit level | `AR740` |
| `telemetry.allow_network`, `llm.allow_network` | `false` | `allow_network = true` in the repository (already ignored by the trust rule, now also reported) | `AR740` |
| `guard.generated` | `true` | a `[guard]` table without `generated = true` | `AR740` |
| `governance.enforce` | `true` | a `[governance]` table without `enforce = true` | `AR740` |
| `governance.require_approval` | union with the repository's selectors; the policy's are not narrowed by `exempt` | (nothing to report: `exempt` is simply not applied to them) | none |
| `governance.min_approvers` | the larger value | a lower explicit `min_approvers` | `AR740` |
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
union, severities and scan levels take the stricter value, and switches turn on. It is commutative, idempotent and
associative, and an empty policy is its identity, so the order of layers does not matter beyond naming the origin.
The intersection of two allowlists keeps an entry of one list when an entry of the other covers it.

`covers(policy, repo)` decides whether a repository pattern is inside a policy pattern. It is segment-based and
conservative: when it cannot prove that every location the repository pattern matches is also matched by the policy
pattern, the answer is no. A false reject is safe; a false accept is not (a fuzz test checks this against a brute-force
universe).

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
to LF. Exit code 1 when the repository loosens the policy. The policy file format is
`schema/policy.schema.json`.

## What honors the policy

- `validate` and `validate --strict`: the clamp is applied at load. `--strict` reports each attempt as an `AR74x`
  finding; plain `validate` fails with the same lines. The severity floor and required codes are enforced inside the
  lint run, so a floored finding cannot be demoted by a lint profile.
- `generate` refuses to run on a configuration that loosens the policy, and so does each root of `generate
  --recursive` and `validate --recursive`; a root that fails is reported and the exit code is 1.
- The MCP servers: `mcp --serve-skills` fails to start on a configuration that loosens the policy, and the
  `generate_outputs` and `validate_config` tools of `mcp` refuse it (`validate_config` answers `valid: false`).
- No suppression hides a protected code: for every code in `required_codes` or raised by `severity_floor` (and
  AR740-AR745 themselves), `ai-rulez-lint-ignore` comments, `[lint] ignore_paths`, baseline entries and
  `[lint.tolerate]` budgets are not applied. The finding stays at its enforced severity and counts toward the exit
  code, and one AR740 finding per route names the attempt (`[lint] ignore_paths`, `ai-rulez-lint-ignore comment`,
  `baseline`, `[lint.tolerate]`) and the codes it tried to hide.
- Remote sources a policy refuses are dropped before anything is fetched.

## Design decisions

- **Phase 1 scope.** Policy from `--policy`, `AI_RULEZ_POLICY` and the managed path, over the keys that already exist
  in `config.toml`. URL policies with digests, `extends`, org-repo discovery, `--policy-digest`, `--policy-mode warn`,
  and the signing, MCP and hooks sections are later phases (`AR741` is reserved for a digest mismatch).
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
- **Known gaps.** `no_inline_ignore` and budget ceilings for unprotected codes are later phases. A policy value is not
  checked against the secret scanner yet.
- **No policy, no change.** Without a flag, variable or managed file the loader does nothing (tested).
