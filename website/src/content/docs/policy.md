---
title: "Organization policy"
description: "An organization policy file sets validation, lock, approval, signing and MCP requirements a repository may tighten but never loosen."
---

An organization policy is a floor a repository may raise but never lower. A security or platform team writes it once,
and `ai-rulez generate`, `validate`, `lock` and the MCP servers of every repository enforce it. Without a policy nothing
changes.

A policy that lived in the repository it governs could be deleted by the pull request that violates it, so a policy is
found only **outside** the repository: a flag, an environment variable or a managed path.

## Example

```toml
# ai-rulez-policy.toml
policy_version = 1
name = "acme baseline"

[sources]
allowed_hosts  = ["github.com/acme"]
require_pinned = true

[lint]
required_codes = ["AR001", "AR005"]

[lint.severity_floor]
AR008 = "error"

[lock]
enforce = true

[governance]
enforce       = true
min_approvers = 2

[mcp]
allowed_commands = ["uvx"]
```

A repository that tries to soften a severity and runs an MCP server through `npx`:

```toml
# .ai-rulez/config.toml (excerpt)
[[mcp_servers]]
name = "fs"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "."]

[lint.severity]
AR008 = "warning"
```

```console
$ ai-rulez validate --show-policy --policy /etc/acme/ai-rulez-policy.toml
policy: 1 layer
  flag      /etc/acme/ai-rulez-policy.toml  sha256:71f8b74b3054…  (acme baseline)
effective (origin in brackets)
  governance.enforce        = true                [flag]
  governance.min_approvers  = 2                   [flag]
  lint.required_codes       = ["AR001", "AR005"]  [flag]
  lint.severity_floor.AR008 = error               [flag]
  lock.enforce              = true                [flag]
  mcp.allowed_commands      = ["uvx"]             [flag]
  sources.allowed_hosts     = ["github.com/acme"] [flag]
  sources.require_pinned    = true                [flag]
repo overrides rejected: 2
  AR740 /work/acme/.ai-rulez/config.toml:27  [lint.severity] AR008 = "warning" is below the policy floor "error" (origin: flag); "error" is enforced
  AR748 /work/acme/.ai-rulez/config.toml:22  [[mcp_servers]] "fs" runs "npx", which is not in mcp.allowed_commands ["uvx"] (origin: flag); the server is not loaded

$ AI_RULEZ_POLICY=/etc/acme/ai-rulez-policy.toml ai-rulez generate
Error: the configuration loosens the organization policy:
  AR740 /work/acme/.ai-rulez/config.toml:27  [lint.severity] AR008 = "warning" is below the policy floor "error" (origin: env); "error" is enforced
  AR748 /work/acme/.ai-rulez/config.toml:22  [[mcp_servers]] "fs" runs "npx", which is not in mcp.allowed_commands ["uvx"] (origin: env); the server is not loaded
...
```

Both exit `2`. The policy values are enforced either way (`AR008` stays an error, the server is not loaded); the
repository has to remove the loosening attempts for the run to succeed.

## Where a policy comes from

Every layer found is loaded and merged tighten-only; no layer overrides another.

| Layer | Source | Use |
| --- | --- | --- |
| `flag` | `--policy <file or https URL>` | a CI step the organization owns |
| `env` | `AI_RULEZ_POLICY=<file or https URL>` | managed machines, CI images |
| `org` | the policy of the repository's GitHub owner, with `--discover-org` or `[policy] discover = "org"` in the user config | a convenience for an organization's repositories ([Organization discovery](#organization-discovery)); not an anchor |
| `managed` | `/etc/ai-rulez/policy.toml` (Linux and other Unix), `/Library/Application Support/ai-rulez/policy.toml` (macOS), `%ProgramData%\ai-rulez\policy.toml` (Windows) | a machine-wide baseline |

- A flag or variable that is set but cannot be loaded is `AR742`, never a skip: breaking the path does not switch the
  policy off. An absent managed file is not an error; a present but unreadable or invalid one is.
- A policy file is a regular file of at most 256 KiB. The same file named twice is one layer.
- Make the managed file writable by administrators only. Nothing a repository controls can move it (on Windows, keep
  `ProgramData` out of repository-controlled environment files).
- Set `AI_RULEZ_POLICY` from the trusted side (organization secrets and variables, a required workflow), not in a
  workflow file that a pull request can edit.

### Policy URLs

A policy may be an `https` URL, so one file serves every repository:

- **A digest is required**: `https://policy.acme.example/base.toml@sha256:<hex>`, or `--policy-digest` with `--policy`,
  or `AI_RULEZ_POLICY_DIGEST` with `AI_RULEZ_POLICY`. The digest is the SHA-256 of the file with CRLF normalized to LF,
  as `validate --show-policy` prints it. A missing or mismatched digest is `AR741`.
- **`--policy-trust-tofu`** records the digest of an unpinned URL once, only in a terminal (a pipe or CI refuses), and
  prints it so you can pin it.
- **https only**: no credentials in the URL and none sent, at most 256 KiB, redirects only on the same host (at most 5).
- **Cache**: a good copy is kept in `~/.cache/ai-rulez/policy`, authenticated with a per-user secret, so a checkout or
  restored cache cannot plant a policy.
- **Offline**: when the URL cannot be reached, the cached copy of the pinned digest stands in for at most `max_stale`
  (default `7d`; `--policy-max-stale` or `AI_RULEZ_POLICY_MAX_STALE`; `0` allows none). `--policy-offline` (or
  `AI_RULEZ_POLICY_OFFLINE=1`) uses the cache without asking the network. Past `max_stale`, or with no cached copy, the
  run fails with `AR742`.

### Organization discovery

`--discover-org`, or this in the user config, also loads `ai-rulez-policy.toml` from the root of the public
`<owner>/.github` repository, where `<owner>` is the GitHub owner of the repository's `origin` remote:

```toml
# ~/.config/ai-rulez/config.toml  (user config, outside every repository)
[policy]
discover = "org"

[policy.digests]
acme = "sha256:<hex>"
```

It is a policy URL like any other: the digest comes from `[policy.digests]` or `--policy-trust-tofu`, never from the
repository. Once a digest is pinned, a 404 is `AR742` (a withdrawn policy is not "no policy"). A remote that is not on
GitHub skips discovery with a warning, unless `--discover-org` was given on the command line.

Discovery is a convenience, not an anchor: the owner comes from the repository's own git configuration, so a pull
request can point `origin` elsewhere. A looser organization policy cannot loosen anything, but enforcement belongs in
`--policy`, `AI_RULEZ_POLICY` or the managed path. The `[telemetry]` and `[llm]` network locks read only those
anchored layers.

### Signed policies

A signature lets an organization publish a policy without every machine pinning its digest. Publish the Sigstore
bundle next to the policy as `<policy>.sigstore.json` (for a URL, the same URL plus the suffix). Either form verifies:

- `ai-rulez sign --org-policy policy.toml --key <key>` (or `--keyless`), which refuses a file the loader would reject;
- `cosign sign-blob --bundle policy.toml.sigstore.json policy.toml`.

Who may sign is configured outside the repository: `--policy-signer-key <pem>` (repeatable), `--policy-signer-identity`
with `--policy-signer-issuer`, `--policy-trusted-root`, the matching `AI_RULEZ_POLICY_SIGNER_KEY`,
`AI_RULEZ_POLICY_SIGNER_IDENTITY`, `AI_RULEZ_POLICY_SIGNER_ISSUER` and `AI_RULEZ_POLICY_TRUSTED_ROOT` variables, or the
user config:

```toml
# ~/.config/ai-rulez/config.toml
[policy]
require_signature = true
tlog = "required"            # required (default with an identity signer), optional or off (default with keys only)

[[policy.signers]]
identity = "https://github.com/acme/policy/.github/workflows/sign.yml@refs/heads/main"
issuer   = "https://token.actions.githubusercontent.com"

[[policy.signers]]
key_file = "/etc/ai-rulez/policy-signer.pub"
```

- Every source adds trusted signers; any can require a signature (`--policy-require-signed`,
  `AI_RULEZ_POLICY_REQUIRE_SIGNED=1`, `require_signature`).
- With a trusted signer configured, a signature that is present must verify even when a digest pin matches. A URL
  policy with a verified signature needs no digest.
- The check uses the same machinery as [lock signing](/ai-rulez/signing/), including a per-user rollback mark, so an older
  signed policy cannot be replayed. Every failure is `AR746` and fails closed.
- Policies reached through `extends` and the organization policy are checked the same way.

### `extends`

A team policy can build on the organization's:

```toml
policy_version = 1
name = "payments team"
extends = ["org.toml", "https://policy.acme.example/base.toml@sha256:<hex>"]   # relative to this file, or pinned URLs
```

Each parent is a layer of its own and everything folds tighten-only. A value the child states that is weaker than its
parent's (a lower severity floor, an allowlist entry the parent does not cover, a higher budget, `enforce = false` where
the parent turns it on) is `AR743`, because it would have no effect. A key the child does not mention is fine.

Limits: at most 8 entries per `extends`, 5 hops deep, 32 policies in one load, no cycles. A URL parent needs its
`@sha256:<hex>` pin, and a policy fetched from a URL cannot extend a local file. A parent that cannot be loaded fails the
whole load (`AR742`).

## The policy file

TOML, data only. An unknown key is an error (`AR743`), because a typo must not silently loosen the policy. Rule codes
may be written as codes (`AR001`) or names (`secret-detected`); a code this ai-rulez does not know is an error asking
you to upgrade. `policy_version = 1` is required. The format is
[`schema/policy.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/policy.schema.json).

```toml
# ai-rulez-policy.toml
policy_version = 1
name = "acme baseline"

[sources]
allowed_hosts  = ["github.com/acme", "*.acme.example"]           # includes, installed skills, skill sources
deny_hosts     = ["github.com/acme/archived"]
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
allowed_hosts = ["github.com", "*.acme.example"]                # bounds the repository's own list
scan_imports  = "error"
directive_tags = ["assistant"]                                  # always checked by AR018; the repository may add more
trusted_orgs   = ["anthropics", "github"]                       # the repository may name only these

[lint.scanner_policy]
preset = "baseline"                                             # the weakest scanner preset
required = ["agnix"]                                            # always required
fail_on = "warning"                                             # the most permissive threshold for scanner findings
isolation = "require"                                           # the weakest isolation of staged scanners
allow_egress = []                                               # the scanners --allow-egress may enable; [] forbids all

[lint.capability]
max_network_commands = 3                                        # AR030 limit

[lint.budgets.skill]
max_tokens = 4000                                               # AR902 limit per content kind

[lint.load_budgets]
claude-skill-listing = 1200                                     # AR964 limit by id

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
enforce          = true                                         # approvals are enforced
require_approval = ["remote", "kind:hook"]                      # the repository's exempt cannot narrow these
min_approvers    = 2
approvers        = ["alice@example.org", "bob@example.org"]     # only these reviewers count
min_assurance    = "review-linked"                              # an asserted approval does not count
forbid_self_approval = true
approvers_from   = "CODEOWNERS"                                 # approvers must also own the item's path

[signing]
require_verified = ["lock"]                                     # the lock must carry a verified attestation
tlog             = "required"
max_age          = "180d"
min_hash_version = 1
allow_repo_identities = false                                   # the repository may trust only the signers below

[signing.thresholds]
lock = 2

[[signing.trust]]
subject  = "lock"
identity = "https://github.com/acme/ai-config/.github/workflows/release.yml@refs/heads/main"
issuer   = "https://token.actions.githubusercontent.com"

[mcp]
allowed_commands = ["npx", "uvx"]                               # the only commands a stdio MCP server may run
deny_transports  = ["http", "sse"]                              # no remote MCP endpoint
read_only        = true                                         # `ai-rulez mcp` registers only its read-only tools

[hooks]
allow = false                                                   # any [[hooks]] group is a violation
```

### Host patterns

Patterns in `sources.allowed_hosts` and `sources.deny_hosts` are a host, optionally followed by a path prefix, compared
case-insensitively:

| Pattern | Matches |
| --- | --- |
| `github.com` | the host, any path |
| `*.acme.example` | `acme.example` and every subdomain |
| `github.com/acme` | any path under the segment `acme`, not `acme2` |
| `github.com/acme-*` | a segment may carry one `*` at its start or end |
| `github.com/*/rules` | `*` is exactly one segment |

A bare `*` host is refused. `https://`, `git@host:org/repo` and `ssh://` sources all give a host and a path; a user, a
port, a trailing dot and letter case are ignored. A source whose location cannot be read unambiguously is refused as
not allowed. A local path names no host and is not governed by `sources`. `lint.security.allowed_hosts` lists hosts
only (no path).

## How a repository is bounded

Each key has one direction. The policy value is enforced, and any attempt to loosen it is reported. A repository that
does not touch a key is not reported: the policy value is just used.

| Policy key | Effective value | A repository loosens it by | Code |
| --- | --- | --- | --- |
| `sources.allowed_hosts` | sources the list covers | a source from another host; it is not loaded | `AR745` |
| `sources.deny_hosts` | union of layers | a source from a denied host; it is not loaded | `AR745` |
| `sources.deny_digests` | union of layers; works with the lock's [`[[deny]]`](/ai-rulez/approvals/#deny-list) | a remote source whose pinned digest is listed is not loaded; a listed authored item is reported | `AR747` |
| `sources.require_pinned`, `lock.enforce` | the lock is enforced | `[lock] enforce = false` | `AR740` |
| `lock.include_outputs` | output digests are pinned | `[lock] include_outputs = false` | `AR740` |
| `sources.min_release_age` | the longer age | a younger `[lock]` or per-source age, including `"0"` | `AR740` |
| `sources.min_release_age_source` | the stronger of `forge` and `first-seen` | a weaker `[lock] min_release_age_source` | `AR740` |
| `lint.required_codes` | union of layers | `[lint.severity] CODE = "off"` or `[lint] ignore` | `AR744` |
| `lint.severity_floor` | the higher severity | a lower `[lint.severity]`, or ignoring a floored code | `AR740` |
| `lint.no_inline_ignore` | union of layers | an `ai-rulez-lint-ignore` comment for a listed code; the finding is still reported | `AR740` |
| `lint.max_findings.<code>` | the lower ceiling (`0` allows none); baselines, `[lint.ratchet]` and ignores do not apply | going over the ceiling | `AR749` |
| `lint.security.allowed_hosts` | repository entries the list covers; the policy list when the repository sets none | an entry the list does not cover; it is dropped | `AR740` |
| `lint.security.scan_imports` | the stricter level (`off` < `warn` < unset < `error`) | a weaker explicit level | `AR740` |
| `lint.security.directive_tags` | union, plus the repository's own tags | (the repository only adds) | none |
| `lint.security.trusted_orgs` | repository entries the list names; an empty policy list trusts no organization | an entry the list does not name; it is dropped | `AR740` |
| `lint.capability.max_network_commands` | the lower value | a higher explicit value | `AR740` |
| `lint.budgets.<kind>.max_lines`, `.max_tokens` | the lower value | a higher explicit value | `AR740` |
| `lint.load_budgets.<id>` | the lower value | a higher explicit value | `AR740` |
| `lint.scanner_policy.preset` | the stronger preset (`off` < `baseline` < `strict`) | a weaker explicit preset | `AR740` |
| `lint.scanner_policy.required` | union with the repository's list | (the repository only adds) | none |
| `lint.scanner_policy.fail_on` | the stricter threshold | a looser explicit threshold | `AR740` |
| `lint.scanner_policy.isolation` | the stronger level (`none` < `auto` < `require`) | a weaker explicit level | `AR740` |
| `lint.scanner_policy.allow_egress` | repository entries the list names; an empty list allows none | an entry the list does not name; it is dropped | `AR740` |
| `telemetry.allow_network`, `llm.allow_network` | `false`, in every scope including user and environment | `allow_network = true` in the repository | `AR740` |
| `guard.generated` | `true` | a `[guard]` table without `generated = true` | `AR740` |
| `governance.enforce` | `true` | a `[governance]` table without `enforce = true` | `AR740` |
| `governance.require_approval` | union; `exempt` does not apply to the policy's selectors | (nothing to report) | none |
| `governance.min_approvers` | the larger value | a lower explicit value | `AR740` |
| `governance.approvers` | repository entries the list names; an empty list means nobody may approve | an entry the list does not name; it is dropped | `AR740` |
| `governance.min_assurance` | the stronger level (`asserted` < `review-linked` < `signed`) | a weaker explicit value | `AR740` |
| `governance.forbid_self_approval` | `true` | a `[governance]` table without it | `AR740` |
| `governance.approvers_from` | the repository's value, or `"CODEOWNERS"`; a policy cannot name a path | a `[governance]` table without `approvers_from` | `AR740` |
| `signing.require_verified` | union with the repository's `[signing] require` | (the repository only adds) | none |
| `signing.tlog` | the stricter mode (`off` < `optional` < `required`) | a weaker explicit mode | `AR740` |
| `signing.max_age` | the shorter age | a longer explicit age | `AR740` |
| `signing.min_hash_version` | the larger value | a lower explicit value | `AR740` |
| `signing.trust`, `signing.allow_repo_identities` | per subject the list names: the repository's signers the list also names, else the policy's entries. An empty list (or `allow_repo_identities = false` alone) governs every subject and trusts nobody else. A `key_file` is never allowed in the policy. An entry names its signer with `identity` or an anchored `identity_regexp`, plus `issuer` | a signer the list does not name for a governed subject; it is dropped | `AR740` |
| `signing.thresholds` | per subject, the larger value | a lower explicit value | `AR740` |
| `mcp.allowed_commands` | stdio servers whose command the list names; an empty list allows none | a server running another command; it is not loaded | `AR748` |
| `mcp.deny_transports` | union of layers | a server on a denied transport (`stdio`, `http`, `sse`); it is not loaded | `AR748` |
| `mcp.read_only` | `true` | `[mcp] read_only = false`; `--read-only=false` cannot turn it off either | `AR740` |
| `hooks.allow` | `false` forbids every hook group | a `[[hooks]]` group; it is not loaded | `AR748` |

`[hooks]` and `[mcp]` also bound imported content: an agent, skill or command from an include or installed skill that
declares `hooks` or `mcpServers` in its frontmatter loses that key (`AR748`, reported against the imported file).
Built-in packs are not bounded.

`AR740` to `AR749` are always errors: `[lint.severity]` cannot soften them and no suppression hides them. For every code
in `required_codes` or raised by `severity_floor`, inline ignore comments, `[lint] ignore_paths`,
`[[lint.ignore_rules]]`, `[[lint.accept]]`, baselines and `[lint.ratchet]` are not applied, and one `AR740` names each
attempt. A policy that cannot be loaded also locks telemetry export and LLM network use.

Approvals are records in the repository's own lock, so the `[governance]` floor matters; pair it with the CI check in
[Approvals are assertions](/ai-rulez/approvals/#approvals-are-assertions).

<a id="rolling-out-with-policy-mode-warn"></a>

## Rolling out with `--policy-mode warn`

`--policy-mode warn` lets a policy land before every repository complies. The policy values are still enforced, but
each loosening attempt is a warning instead of an error: `generate` and `validate` log the violations and carry on, and
`validate --show-policy` exits `0` and prints `mode: warn`. A policy that cannot be loaded or is invalid
(`AR741`-`AR743`) still fails. Set the mode on the trusted side, next to the policy.

```console
$ ai-rulez generate --policy /etc/acme/ai-rulez-policy.toml --policy-mode warn
WARN  Organization policy violation (--policy-mode warn): AR740 /work/acme/.ai-rulez/config.toml:27  [lint.severity] AR008 = "warning" is below the policy floor "error" (origin: flag); "error" is enforced
...
INFO  Generation complete files=12
```

<a id="validate-show-policy"></a>

## `validate --show-policy`

Prints the loaded layers with their digests, the effective value and origin of every key, and what the repository tried
to loosen (see the [example](#example)). `--format json` writes `schema/policy-effective.schema.json`: `layers[{origin,
source, name, digest}]`, `effective`, `provenance` (dotted key to the layer, joined with `+` when several contribute),
`overrides{accepted, rejected}` and `violations[]`. Exit `2` when the repository loosens the policy.

## What honors the policy

| Command | Behavior on a configuration that loosens the policy |
| --- | --- |
| `generate`, `generate --recursive` | refuses to run, exit `2` |
| `validate` (any mode), `validate --show-policy` | exit `2`; `validate` lists each attempt as an `AR74x` finding; `validate --config-only` refuses with the policy error |
| `lock` | refuses to write `ai-rulez.lock`, exit `2`; `lock --check` reports a mismatch |
| `doctor` | reports each attempt in the `policy` check, exit `2` |
| `mcp --serve-skills` | fails to start |
| `mcp` | the `generate_outputs` and `validate_config` tools refuse it |
| `approve`, `catalog diff` and the other commands | load configuration under the policy |

Remote sources the policy refuses are dropped before anything is fetched.

## Troubleshooting

| Code | Cause | Fix |
| --- | --- | --- |
| `AR740` `policy-loosened` | the repository sets a weaker value than the policy | remove the repository value, or ask the policy owners |
| `AR741` `policy-digest-mismatch` | a URL policy has no pin, or its content does not match | pin the digest `validate --show-policy` prints |
| `AR742` `policy-unavailable` | a named policy file or URL cannot be loaded, a cached copy is past `max_stale`, or a parent of `extends` fails | fix the path or network; the run never continues without the policy |
| `AR743` `policy-invalid` | unknown key, wrong `policy_version`, or a child of `extends` weaker than its parent | fix the policy file |
| `AR744` `policy-required-missing` | a required code is turned off or ignored | remove the `off` or ignore |
| `AR745` `source-not-allowed` | a source's host is not allowed, or denied | use an allowed source |
| `AR746` `policy-signature-invalid` | the policy's signature is missing, invalid, from an untrusted signer or older than one already seen | re-sign or fix the signer configuration |
| `AR747` `digest-denied` | pinned content is on `sources.deny_digests` | change the content |
| `AR748` `capability-not-allowed` | an MCP server or hook the policy forbids | remove it, or use an allowed command or transport |
| `AR749` `policy-budget-exceeded` | more findings of a code than `lint.max_findings` allows | fix findings below the ceiling |

```console
$ ai-rulez validate --policy /etc/acme/missing.toml
Error: AR742: policy /etc/acme/missing.toml (from flag) cannot be used: open /etc/acme/missing.toml: no such file or directory; ai-rulez fails closed instead of running without it
```
