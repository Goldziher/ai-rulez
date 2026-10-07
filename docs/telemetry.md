# Item-load telemetry and OTLP export

[Usage telemetry](usage-telemetry.md) answers "which skills are ever invoked". This page covers the next step:
which **rules, agents, context files and skills** a harness actually loaded, how many rules a session carries, and
optionally shipping those identifier-only events to an OpenTelemetry collector. Everything is off by default, the
local log works with no network, and a repository can never turn on network export for the people who clone it.

```console
ai-rulez telemetry hook              # hooks that record loads (merge into .claude/settings.json)
ai-rulez telemetry hook --format toml   # the same as [[hooks]] for config.toml; generate writes them
ai-rulez telemetry enable --endpoint https://collector.example.org:4318   # consent, stored per user
ai-rulez telemetry status            # on or off, consent, pending events, failed flushes
ai-rulez telemetry disable           # withdraw consent
ai-rulez telemetry doctor            # resolved config, consent, buffer, last flush
ai-rulez telemetry flush             # ship the outbox now
ai-rulez telemetry preview           # print exactly what an export would send; sends nothing
ai-rulez usage export --to file out.ndjson   # the same payload as a local OTLP JSON file, no network
ai-rulez usage export --to otlp      # push the usage log past the export cursor to the consented collector
ai-rulez usage prune --keep-days 90  # trim the usage log behind the export cursor
ai-rulez report usage .ai-rulez/local/usage.jsonl   # rules per session, never-loaded rules, load reasons
```

## What is collected

One event per load. The model is closed: these are all the fields, versioned by `v` (currently `1`).

| Field | Meaning | Example |
| --- | --- | --- |
| `v`, `event`, `ts` | schema version, always `item_event`, RFC 3339 UTC time | `1`, `item_event` |
| `event_id` | 16 hex; replays of the same spool line keep it, so a backend can de-duplicate | `9f3c0b51a7d2e648` |
| `kind` | `skill`, `rule`, `agent`, `command` or `context` | `rule` |
| `id` | the item's name: a rule's file name without `.md`, an agent type, a skill name, a context file's repo-relative path | `atomic-commits` |
| `path` | repo-relative path, only when `include_paths = true`, never absolute | `.claude/rules/atomic-commits.md` |
| `digest` | content digest when known: `blake3:...` (skills: the index hash, rules: the hash of the generated file) or `sha256:...` (a skill's canonical lock digest, or the digest of a served skill) | `blake3:0699fc6b...` |
| `digest_scheme` | how a `sha256:` digest was computed: `ai-rulez/skill/v1` (the canonical digest evals and the lock share) or `ai-rulez/served-skill/v1`. It travels with the digest so an export reader joins on the canonical one only | `ai-rulez/skill/v1` |
| `source` | `hook`, `mcp` or `cli` | `hook` |
| `harness`, `role` | as given to the recorder | `claude`, `backend` |
| `served` | the load came through the MCP skills server | `false` |
| `session` | salted hash of the harness session id (16 hex), never the raw id | `5b1c0e9a7d3f2a64` |
| `outcome` | `loaded`, `used` or `abandoned` | `loaded` |
| `load_reason` | why it loaded (Claude Code: `session_start`, `nested_traversal`, `path_glob_match`, `include`, `compact`; agents: `subagent_start`, `subagent_stop`; MCP: `read`, `list`) | `path_glob_match` |
| `memory_type` | Claude Code's `User`, `Project`, `Local` or `Managed` | `Project` |
| `duration_ms` | subagent run time, from the paired Start and Stop events | `2500` |
| `pass_rate`, `trigger_precision`, `trigger_recall`, `ablation_delta` | scores of an `eval_result` event only (0..1; the ablation delta -1..1); never on a load | `0.62` |

Besides `item_event`, an export can carry **`eval_result`** events: one per verified eval result, built from
`.ai-rulez/eval-results.json` at export time (`usage export --with-evals`, `telemetry preview --with-evals`), never
recorded in the usage log. It has the skill id, the canonical digest the run covered (`digest` +
`digest_scheme = ai-rulez/skill/v1`, absent for a result written before `lock_digest` existed), the harness the eval
ran on, the run date as the time, and the four scores. It carries no case, prompt or output and none of the load
fields (path, session, role, outcome). Its `event_id` is derived from what it says, so the same result is never sent
twice. Unsigned or foreign-signed records and activation-only records are not results and are never exported.

Skill events are written to the usage log in the existing v3 line format, so `usage` readers are unaffected. The
other kinds are new `item_event` lines in the same file; `report usage` ignores them in its skill sections and never
counts them as unreadable.

**Never collected**: prompts, model output, tool inputs or results, file contents, command arguments, transcripts,
user, host or repository names, environment values, absolute paths, the raw session id, free-text feedback notes.
The hook handler decodes only the fields in the table above from a hook payload; `prompt`, `transcript_path`,
`last_assistant_message`, `globs` and `trigger_file_path` are not declared, so they cannot reach an event. A file
outside the repository (a user-level `~/.claude/rules/x.md`) contributes only its base name.

## Hook payloads: verified and inferred

Checked against the hook input schemas embedded in Claude Code 2.1.289 (read-only inspection of the binary):

| Event | Fields used | Status |
| --- | --- | --- |
| `InstructionsLoaded` | `file_path`, `memory_type` (`User`\|`Project`\|`Local`\|`Managed`), `load_reason` (`session_start`\|`nested_traversal`\|`path_glob_match`\|`include`\|`compact`); also present and ignored: `globs`, `trigger_file_path`, `parent_file_path`. The hook matcher is the `load_reason`. | verified |
| `SubagentStart` | `agent_id`, `agent_type` | verified |
| `SubagentStop` | `agent_id`, `agent_type` (also `agent_transcript_path`, `last_assistant_message`, `stop_hook_active`: ignored) | verified |
| common | `session_id`, `cwd`, `hook_event_name` | verified |
| `PreToolUse` (Skill tool), `UserPromptExpansion` (`command_name`) | as [documented](usage-telemetry.md) | verified earlier |
| command hook `async: true`, `timeout` | handler fields | verified (`async` runs the hook in the background without blocking) |

Codex and Cursor, checked against their published hook documentation on 2026-10-06 (<https://developers.openai.com/codex/hooks>,
<https://cursor.com/docs/hooks>; documentation only, the binaries were not inspected):

| Harness | Event | Fields used | Status |
| --- | --- | --- | --- |
| Codex | `SubagentStart` | `session_id`, `cwd`, `agent_id`, `agent_type` | documented 2026-10-06 |
| Codex | `SubagentStop` | `session_id`, `cwd`, `agent_id`, `agent_type` (`last_assistant_message`, `agent_transcript_path`, `stop_hook_active`: ignored) | documented 2026-10-06 |
| Codex | `PreToolUse` (matcher `Bash`) | `tool_name`, `tool_input.command` (skill reads, see [usage telemetry](usage-telemetry.md)) | documented 2026-10-06 |
| Cursor | `subagentStart` | `conversation_id` (the session), `subagent_id`, `subagent_type` (`task`, `git_branch`, ...: ignored) | documented 2026-10-06 |
| Cursor | `subagentStop` | `conversation_id`, `subagent_type`, `duration_ms` (`summary`, `task`, `modified_files`, `agent_transcript_path`, ...: ignored) | documented 2026-10-06 |
| Cursor | `preToolUse` (matcher `Shell`, `Read`, ...) | `tool_name`, `tool_input` | documented 2026-10-06 |
| Codex, Cursor | rule, agent or context **load** event | none | not documented: Codex states no event reports loaded skills or instructions, and Cursor lists none; nothing is recorded |

`telemetry hook --harness codex|cursor` therefore prints the skill-read template of `usage hook` plus the subagent events
above, each running `telemetry record --harness <name>`. Cursor's `subagentStop` has no `subagent_id`, so the duration
comes from the payload's `duration_ms`; Codex has no duration, so it is computed from the paired `SubagentStart` as
for Claude Code.

Inferred, not verified: the mapping from a rule's generated file (`.claude/rules/<name>.md`) to the source rule id is
by file name, which is how ai-rulez names generated rules; `SubagentStop` carries no duration in Claude Code, so it is
computed from the paired `SubagentStart` and is absent when the Start was missed. For Codex and Cursor, the shape of
`tool_input` of a file-read tool (`file_path` or `path`) is inferred; the handler fields `async` and `timeout` are
written only for Cursor (documented there) and Claude Code. Rule, agent and context loads are still recorded for
Claude Code only; Codex and Cursor agents are recorded from the subagent events.

## Consent and configuration

```toml
# .ai-rulez/config.toml, the local overlay, or the user config (~/.config/ai-rulez/config.toml)
[telemetry]
enabled         = false                    # local recording of item events
allow_network   = false                    # consent as a setting (the stored record of `telemetry enable` is the other way); user scope only
otlp_endpoint   = "https://collector.example.org:4318"   # user scope only
otlp_protocol   = "http/json"              # or "http/protobuf" or "grpc"; same attributes on all three
headers_env     = ["OTLP_HEADERS"]         # env var NAMES holding "k=v,k2=v2"; user scope only
service_name    = "ai-rulez"               # user scope only: it labels data on your collector
sample          = 1.0                      # fraction of sessions exported, decided per session hash
include_paths   = false                    # user scope only
include_session = false                    # add the salted session hash to exported logs; user scope only
salt_file       = ""                       # user scope only

[telemetry.resource]                       # extra OTLP resource attributes; user scope only
team = "platform"
"deployment.environment" = "prod"
```

### The trust rule

A committed repository config is attacker-controlled input: cloning a repository must not start sending data to a
host the author picked. So:

| Key | Repository config and local overlay | User config / `AI_RULEZ_TELEMETRY_*` |
| --- | --- | --- |
| `enabled` | honored (local recording only, into `.ai-rulez/local/usage.jsonl`, which the managed `.gitignore` block covers while `gitignore = true`) | honored |
| `sample` | honored | honored |
| `allow_network`, `otlp_endpoint`, `otlp_protocol`, `headers_env`, `service_name`, `include_paths`, `include_session`, `salt_file`, `resource` | **ignored**, reported by `telemetry doctor` and strict finding `AR9K1` | honored |

The local overlay (`config.local.*`) counts as repository scope: it is machine-local by convention but lives in the
checkout. Network export is active only when recording is on, consent is present (a stored consent record, or
`allow_network` in user scope) and a valid endpoint is present after this filtering, so a repository can at most
switch on the local log.

See the [trust model](trust-model.md) for the same rule across every knob.

Precedence, highest first: kill switches (`AI_RULEZ_TELEMETRY=off`, `DO_NOT_TRACK=1`: nothing is recorded or
exported) > environment (`AI_RULEZ_TELEMETRY`, `AI_RULEZ_TELEMETRY_ENDPOINT`, `_PROTOCOL`, `_ALLOW_NETWORK`,
`_HEADERS_ENV` as a comma list of names, `_SERVICE_NAME`, `_SAMPLE`, `_INCLUDE_PATHS`, `_INCLUDE_SESSION`,
`_SALT_FILE`, `_RESOURCE` as `key=value,key2=value2`) > user config > repository config > defaults.

`[telemetry.resource]` labels the data at the source. Keys are lower-case dotted names (`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)*$`,
at most 64 characters), there are at most 8 entries, and a value is a non-empty string of at most 128 characters with no
control characters. The `service.*`, `host.*`, `user.*`, `process.*`, `os.*`, `cloud.*`, `k8s.*`, `container.*`,
`telemetry.*` and `ai_rulez.*` namespaces are reserved, so host or user data cannot be added. Environment entries
override user-config entries per key. A repository `[telemetry.resource]` is ignored (reported by `telemetry doctor`
and `AR9K1`); an invalid table is `AR9K0` and keeps export off. `telemetry doctor` lists the effective labels and their
source.

Validation (`AR9K0`): https is required except for a loopback host; no credentials, query or fragment in the
endpoint; `headers_env` accepts an environment variable name only (`^[A-Z][A-Z0-9_]*$`, and a 20-character name
without an underscore is refused as an access-key id), so a pasted token or `Authorization=Bearer ...` is rejected and
never echoed back; `sample` is 0..1. Headers are read from the environment at flush time and never written anywhere.

### The consent record

`telemetry enable` stores your consent as a small file, `$XDG_CONFIG_HOME/ai-rulez/telemetry-consent.json` (else
`~/.config/ai-rulez/`), mode 0600, written atomically:

```json
{
  "version": 1,
  "endpoint": "https://collector.example.org:4318",
  "protocol": "http/json",
  "scope": { "fields": ["event.name", "ai_rulez.item.kind", "..."], "fields_hash": "sha256 of the fields" },
  "granted_at": "2026-10-06T09:00:00Z",
  "ai_rulez_version": "5.0.0"
}
```

```console
ai-rulez telemetry enable --endpoint https://collector.example.org:4318
ai-rulez telemetry enable --endpoint collector.internal:4317 --protocol grpc --include-session
ai-rulez telemetry status
ai-rulez telemetry disable
```

- **Only you can grant it.** The record is read from the user config directory only. A repository's config, overlay or
  checkout cannot grant, edit or point to one; a record file planted in a repository is never read. A record that is
  group- or world-writable, a symlink, malformed or has an unknown field is not honored and `telemetry status` says why.
- **It covers what you agreed to, no more.** The record names the endpoint, the protocol and a hash of the exported
  attribute names. A different endpoint or protocol (environment or user config), an opt-in gate you turned on later
  (`include_session`, `include_paths`), or an allowlist that grew in a newer release makes it **stale**: nothing is sent
  until you run `enable` again. `status` and `doctor` name the reason.
- **It supplies, never overrides.** With a valid record, recording turns on for you and the endpoint, protocol and the
  opt-in gates you consented to are used where your user config and environment set none. `allow_network` in the user
  config or `AI_RULEZ_TELEMETRY_ALLOW_NETWORK=1` remain an equivalent grant; `AI_RULEZ_TELEMETRY_ALLOW_NETWORK=0` refuses
  even with a record. The kill switches and an organization policy win over everything.
- **It is not retroactive.** `enable` places the export cursor at the end of the current usage log, so only events
  recorded from now on are sent; `--backfill` places it at the start to send the existing history. Re-enabling is a fresh
  decision: opt-in gates from the previous record do not carry over.
- `disable` deletes the record. Export stops, and so does the local recording the record turned on; recording that the
  user or repository config or `AI_RULEZ_TELEMETRY` turns on continues, and `disable` says which. The usage log and the
  outbox are kept.

## Wiring the hooks

Merge the output of `ai-rulez telemetry hook` into `.claude/settings.json`, or let `generate` own it by pasting the
`[[hooks]]` form into `config.toml` (see [Hooks and permissions](settings.md)):

```toml
[[hooks]]
event = "InstructionsLoaded"
targets = ["claude"]
[[hooks.hooks]]
command = "ai-rulez telemetry record"
async = true
timeout = 5

[[hooks]]
event = "SubagentStart"
targets = ["claude"]
[[hooks.hooks]]
command = "ai-rulez telemetry record"
async = true
timeout = 5

[[hooks]]
event = "SubagentStop"
targets = ["claude"]
[[hooks.hooks]]
command = "ai-rulez telemetry record"
async = true
timeout = 5
```

`ai-rulez telemetry hook --format toml` prints these three groups plus the two skill groups (`PreToolUse` on `Skill`
and `UserPromptExpansion`, running `usage record`). `telemetry record` prints nothing on stdout, exits 0 on every
error, ignores events it does not handle, and does nothing when telemetry is off (not even creating the salt file).
The role comes from `--role` or `$AI_RULEZ_ROLE`.

## How recording and export work

Hooks are short-lived processes, so the recorder never touches the network:

1. The hook appends the event to `.ai-rulez/local/usage.jsonl` (local recording) and, when export is active, to a
   bounded outbox `.ai-rulez/local/telemetry-outbox.jsonl` (mode 0600, oldest events dropped first, about
   10,000 events). Taking the outbox lock waits at most 25 ms; on contention the event is dropped, never the harness
   delayed. A recorder call with a dead collector measures well under the 50 ms budget (there is a test for it).
2. When the outbox is big enough or the last flush is older than 5 minutes, the recorder starts one detached
   `ai-rulez telemetry flush --background` (at most one start a minute, no inherited descriptors, 8 second deadline).
3. The flush sends batches of 200 events to `/v1/logs` and `/v1/metrics` as gzipped OTLP/HTTP (JSON or protobuf; see
   [Transports](#transports)), retrying 429, 502,
   503, 504 and network errors with exponential backoff (1 s, 2 s, 4 s, plus jitter; `Retry-After` honored up to
   30 s) and giving up until the next flush, which leaves the events in the outbox. Every other status is
   permanent (400, 401, 403, 404, 413, a 3xx redirect, 500 and the rest): the batch is dropped and counted as rejected. Redirects are never followed. One flusher runs at a time, guarded by a lock file that holds the owner's token (release removes only its own lock; a lock left by a crash is taken over after 60 s, serialised so two processes cannot both take it). A flush is capped at 30 seconds whatever the caller asks, and `telemetry flush --timeout` above 30s is refused, so a running flush is never mistaken for a crashed one.
4. Delivery is at-least-once; backends de-duplicate on `ai_rulez.event_id`.
   A crash between the request and the bookkeeping, or two copies of one log, can deliver an event twice; the id is
   the same each time.
5. A long-lived `ai-rulez mcp` server flushes from a timer and once more on exit.

Errors kept in `telemetry-state.json` contain a status code or a reason such as `timeout`, never the URL or a header.

### The export cursor and catch-up

The usage log is the source of truth; the outbox is only the delivery queue the hooks fill. The **cursor**
(`.ai-rulez/local/telemetry-cursor.json`, mode 0600) records how far into the log events are accounted for: queued,
delivered, or older than your consent. It holds the log's identity (a hash of its first line, so a rotated or
truncated log is noticed and read from the start), a byte offset, the last event id and a ring of the last 4,096
delivered or rejected event ids.

Every flush begins with a **catch-up**: it reads the complete log lines past the cursor (a line a hook is still writing
waits for its newline), queues the events the outbox does not already hold and the ring does not remember as
delivered, and moves the cursor. That covers events that never reached the outbox (the spool was busy for 25 ms, or
they were recorded while the outbox was off) without sending anything twice. Rules:

- A cursor that was never set is placed at the **end** of the log by the first catch-up and nothing is queued: history is
  exported only on request (`telemetry enable --backfill`, `usage export --to otlp --all`).
- One catch-up queues at most 2,000 events; the rest is picked up by the next one.
- Sampling (`sample`) applies to catch-up events as it does when recording; `sample = 0` exports nothing.
- Under a consent record, a cursor placed before the consent was granted (another project, which kept recording while
  consent was off) skips the events recorded before the grant; `--all` still sends them on request.
- `usage export --to otlp` is the same pass run by hand, with `--all`, `--dry-run` and `--with-evals`; it refuses without
  consent and exits 1 when delivery fails so CI notices.

**Opportunistic flush.** A hook never waits on the network: after recording it may start one detached
`telemetry flush --background` (at most one a minute, only when the outbox is due), which is bounded by the 8 second
deadline and exits 0 whatever happens. The long-lived `mcp` server flushes from a timer and once on exit within 3
seconds. Failures are silent to the harness but not lost: each flush records its outcome, and `telemetry status` shows
the last error, the number of failed flushes and how many failed in a row, next to delivered, dropped and rejected
totals.

### Pruning the usage log

`usage prune --keep-days N` deletes usage-log lines older than N days, but only those **behind the cursor**: an event that
is still waiting to be sent is kept however old, so pruning never costs an export. With no cursor (export was never on)
age alone decides. A line with no readable `ts` is kept. The file is replaced atomically (mode 0600) and the cursor is
moved to match, so the next flush neither re-reads the log from the start nor skips an event. If the cursor describes
another log (the file was replaced since the last flush) the prune refuses; `telemetry flush` brings the cursor up to
date, or `--ignore-cursor` prunes by age alone. `--dry-run` reports without rewriting; a log other than the project's
(`--log`) has no cursor and is pruned by age. With `--ignore-cursor` the cursor still moves back by the bytes removed
before it, so events already read are not exported again. The prune holds the spool lock (the one flushes and
catch-ups take) from reading the cursor to writing it back, so it cannot race a background flush. The usage log is locked too: a
prune holds an exclusive lock on `<log>.lock` (`flock`, `LockFileEx` on Windows) from reading the log to renaming the
rewrite over it, and every recorder holds the same lock shared while it appends, so no line is written in between. A
prune waits up to five seconds for running recorders and exits 1 (`usage log is locked`) when they do not finish. A
recorder that cannot get the lock within two seconds appends anyway, because a hook must not stall its harness; the
prune therefore still re-reads a log that grows while it works and exits 1 if it keeps changing, and a line appended
without the lock in the last microseconds before the rename can still be lost. `--dry-run` takes no lock.

## OTLP mapping

Resource attributes: `service.name` (default `ai-rulez`), `service.version`, `ai_rulez.schema_version`, then the
`[telemetry.resource]` labels sorted by key. No host, process, OS or user attributes are added: there is no SDK
resource detector.

**Logs**: one record per event, severity `INFO`, body `item <outcome>`, attribute `event.name` =
`ai_rulez.item.<outcome>`, time = the event time. An `eval_result` event is a record with body `eval result` and
`event.name` = `ai_rulez.eval.result` (no `ai_rulez.served` attribute: a result is not a load).

| Attribute | Event field | Gate |
| --- | --- | --- |
| `ai_rulez.item.kind`, `ai_rulez.item.id` | `kind`, `id` | |
| `ai_rulez.item.digest` | `digest` | |
| `ai_rulez.item.digest_scheme` | `digest_scheme` | |
| `ai_rulez.item.path` | `path` | `include_paths` |
| `ai_rulez.source`, `ai_rulez.harness`, `ai_rulez.role`, `ai_rulez.served` | same names | |
| `ai_rulez.session` | `session` | `include_session` |
| `ai_rulez.outcome`, `ai_rulez.load_reason`, `ai_rulez.memory_type` | same names | |
| `ai_rulez.duration_ms` | `duration_ms` | |
| `ai_rulez.event_id` | `event_id` | |
| `ai_rulez.eval.pass_rate`, `.trigger_precision`, `.trigger_recall`, `.ablation_delta` | scores (doubles) | `eval_result` only |

The table is `telemetry.Allowlist` in `internal/telemetry/allowlist.go`; a test fails when `Event` gains a field that
is neither listed nor mapped to the record envelope, so a new field cannot leave the machine by accident.

**Metrics** (delta temporality, batch-scoped start time):

| Metric | Type | Labels |
| --- | --- | --- |
| `ai_rulez.item.loads` | monotonic sum, `{load}` | `kind`, `id`, `harness`, `role`, `served`, `digest_short` (first 12 hex of the digest) |
| `ai_rulez.item.outcomes` | monotonic sum, `{load}` (outcome `used` or `abandoned`) | the labels above plus `outcome` |
| `ai_rulez.agent.duration` | histogram, `ms`, bounds 100, 500, 1000, 5000, 15000, 60000, 300000 | `kind`, `id`, `harness` |
| `ai_rulez.skill.eval.pass_rate` | gauge, `1` | `id`, `harness` |
| `ai_rulez.skill.eval.trigger_precision`, `.trigger_recall`, `.ablation_delta` | gauge, `1` | `id`, `harness` |

Session and path are never metric labels. The eval gauges report the latest result per skill and harness in the batch
(point time = the run date); the digest a score is about is on the `ai_rulez.eval.result` log record, not a label, to
keep the series count per skill constant. A gauge point exists only for a score the result has.

## Transports

`otlp_protocol` (user scope only) picks the transport; the exported attributes are identical on all three.

| Value | Wire | Endpoint |
| --- | --- | --- |
| `http/json` (default) | OTLP/HTTP, gzipped JSON body, `Content-Type: application/json` | base URL; `/v1/logs` and `/v1/metrics` are appended |
| `http/protobuf` | OTLP/HTTP, gzipped protobuf body, `Content-Type: application/x-protobuf` | same as above |
| `grpc` | OTLP/gRPC `LogsService` and `MetricsService`, gzip-compressed calls | `host[:port]` (default port 4317), no path; a bare `host:port` means `https` (TLS), `http://` is loopback only and plaintext |

Headers from `headers_env` travel as HTTP headers or gRPC metadata (keys lower-cased). Retry classes are the same: gRPC
`Unavailable`, `ResourceExhausted`, `DeadlineExceeded`, `Aborted` and `Canceled` are retried (a server `RetryInfo` delay
replaces the backoff, capped at 30 s); every other code drops the batch as rejected. A grpc endpoint with a path is
`AR9K0`. `telemetry preview` always prints the JSON body: the protobuf and gRPC messages are decoded from exactly that JSON
(an unknown field fails the decode), and `transport_test.go` decodes all three transports and asserts they are the same
messages and that every attribute is on the allowlist.

## Previewing an export

`ai-rulez telemetry preview` encodes pending events with the same allowlisted encoder the exporter uses and prints
what a flush would send, without opening a connection or writing anything. It works whether or not export is
enabled or consented to, so you can audit the payload before turning it on.

```console
$ ai-rulez telemetry preview --limit 2
source: .ai-rulez/local/usage.jsonl (usage log) (118 events, previewing 2)
export: off (telemetry is not enabled; allow_network is not set in user scope; no otlp_endpoint in user scope); this is what would be sent once it is on

POST https://collector.example.org:4318/v1/logs  (2 events, gzip, 412 bytes)
{"resourceLogs":[ ... exact body ... ]}

POST https://collector.example.org:4318/v1/metrics  (2 events, gzip, 389 bytes)
{"resourceMetrics":[ ... exact body ... ]}

fields exported: event.name, ai_rulez.item.kind, ai_rulez.item.id, ...
fields withheld: ai_rulez.item.path, ai_rulez.session

Nothing was sent.
```

- The events come from the outbox when export is active (what the next flush sends), otherwise from the usage log;
  `--log FILE` previews another log. A log is read through the same validators as the exporter: a field that fails
  its pattern is dropped, an event whose kind or id fails is left out and counted, raw session ids from version 1
  lines are never exported, and an event with no `event_id` (log version 2 and older) gets one derived from its line text and line number.
- The preview is the exact body of each request except the observation time (`observedTimeUnixNano`) and the
  metric timestamps: the preview uses the newest previewed event's time so the output is reproducible, while a flush
  stamps its own clock. `--limit` also trims the batch, so a flush of more events sends larger requests.
- `--limit N` previews the first N events (default 5, `0` for all). Sampling applies to a log as it would on recording.
  `--with-evals` adds the `eval_result` events and eval gauges `usage export --with-evals` would send.
- The destination shows the scheme, host and path of the endpoint only: user info and query are dropped, headers
  (`headers_env`) are never shown; `telemetry doctor` prints a `headers_env` entry only when it is shaped like a variable name, and shows `(invalid, hidden)` for anything else. Without an endpoint it says `<no endpoint configured>`.
- `fields withheld` lists the allowlist attributes whose opt-in (`include_paths`, `include_session`) is closed.

## Example collector and queries

```yaml
receivers:
  otlp:
    protocols:
      http: { endpoint: 0.0.0.0:4318 }
processors:
  deltatocumulative: {}
  batch: {}
exporters:
  prometheus: { endpoint: 0.0.0.0:9464 }
  loki: { endpoint: http://loki:3100/loki/api/v1/push }
service:
  pipelines:
    metrics: { receivers: [otlp], processors: [deltatocumulative, batch], exporters: [prometheus] }
    logs: { receivers: [otlp], processors: [batch], exporters: [loki] }
```

The Prometheus exporter renders `ai_rulez.item.loads` as `ai_rulez_item_loads_total` (names and label spelling depend
on your pipeline; adapt the queries).

```promql
# Top skills by role over the last week
topk(10, sum by (id, role) (increase(ai_rulez_item_loads_total{kind="skill"}[7d])))

# Skills (or rules) whose series exist but that were not loaded for 30 days
sum by (kind, id) (increase(ai_rulez_item_loads_total[30d])) == 0

# Load volume by reason is a log query; metrics carry no reason label
```

Metrics cannot see an item that was never loaded, because no series exists for it. Use
`ai-rulez report usage` for "never loaded": it joins the log with the generated manifest. Rules loaded per session
needs the session, which is pseudonymous and off by default: with `include_session = true` the Loki/LogQL form is
roughly `sum by (ai_rulez_session) (count_over_time({service_name="ai-rulez"} | ai_rulez_item_kind="rule" [1d]))`,
but `report usage` computes the median and distribution exactly from the local log.

## Verifying against a real collector

The OTLP/HTTP JSON encoder is hand-written, so one opt-in test checks it against a real OpenTelemetry Collector
(`otel/opentelemetry-collector-contrib`, pinned by tag and digest in `internal/telemetry/otelcol_e2e_test.go`):

```console
task test:otelcol    # needs docker; sets AI_RULEZ_E2E_OTELCOL=1
```

The test starts the container with a file exporter, flushes a spool batch through the real exporter (gzip, logs and
metrics), and asserts that the collector accepted it, that every decoded log attribute is on the allowlist, that
`[telemetry.resource]` labels appear on both resources, and that the event ids and metric names round trip. It also
checks that the collector answers a valid body with 200 and a malformed one with 400 (the permanent-rejection class).
It is skipped unless docker is reachable and `AI_RULEZ_E2E_OTELCOL=1` is set, so `go test ./...` never pulls an image.
Last run: passed against collector 0.130.0 on 2026-10-06. `Retry-After` and partial-success handling stay covered by the
fake-server tests only, because a real collector does not emit them on demand.

A second opt-in test, `TestLiveCollectorAcceptsEveryTransport`, runs the same pinned collector with the OTLP `http` and
`grpc` receivers and sends one batch over each of `http/json`, `http/protobuf` and `grpc`:

```console
AI_RULEZ_LIVE_OTEL=1 go test ./internal/telemetry -run TestLiveCollector -v
```

It needs docker and is skipped otherwise. The default test run covers the three transports against local fakes: an
`httptest` server for both HTTP encodings and an in-process gRPC server.

## Data egress and residency

Only the allowlisted attributes above leave the machine, to the one endpoint the user configured, over https (http only
to loopback). ai-rulez operates no service and has no default endpoint: residency is whatever collector you run. Item ids
can themselves be sensitive (a private skill name): they go only to your collector. The salt that hashes sessions
stays on the machine, so sessions cannot be linked across machines. Delete `.ai-rulez/local/telemetry-*` to discard the
outbox and state.

## Complementing the harness's own OpenTelemetry export

Claude Code exports its own metrics and events (`claude_code.token.usage`, `claude_code.cost.usage`,
`claude_code.session.count`, `claude_code.subagent.spawn`, ...) when `CLAUDE_CODE_ENABLE_TELEMETRY=1` and the standard
`OTEL_METRICS_EXPORTER`, `OTEL_LOGS_EXPORTER`, `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_EXPORTER_OTLP_ENDPOINT` and
`OTEL_RESOURCE_ATTRIBUTES` variables are set (variable names verified in the binary; the metric list is what the
binary names). ai-rulez does not duplicate any of that: it adds only what the harness cannot know, which ai-rulez
item a load corresponds to. Point both at the same collector and give both the same team label (set
`OTEL_RESOURCE_ATTRIBUTES` for Claude Code; set the same labels in `[telemetry.resource]`, or use the collector's
resource processor) to join them.

## MCP server integration

With telemetry enabled, `ai-rulez mcp` records `source=mcp` events after a successful `read_skill`, `read_rule`,
`read_context`, `get_skill` (served) and after `list_*` / `search_skills` (id `_list`, reason `list`). With telemetry
off the handlers are untouched. A new read tool (for example `load_skill`) is covered by adding one line to
`telemetryTools` in `internal/mcp/telemetry.go`, or by calling `srv.EmitItem(ctx, telemetry.KindSkill, name, true)`
from the handler.

## Dependency decision

The exporter is a hand-written OTLP/HTTP JSON encoder on `net/http` (`internal/telemetry/otlp.go`), not the OpenTelemetry Go SDK. The SDK would add a large dependency tree to a tool that
reviewers read for its egress behavior, brings resource detectors that add host data by default, and its batching
assumes a long-lived process while hooks exit in milliseconds. `http/protobuf` and `grpc` add only the generated OTLP
messages (`go.opentelemetry.io/proto/otlp`) and `google.golang.org/grpc`: the encoder still builds the one JSON form,
which is decoded into the messages, so there is no second attribute mapping to keep in step. Together they add about
5.9 MB (14%) to the release binary, measured on darwin/arm64 (`transport.go`).

## Strict-validation codes

| Code | Name | Severity | Finds |
| --- | --- | --- | --- |
| `AR9K0` | `telemetry-config-invalid` | error | invalid `[telemetry]` value: bad range or enum, unsupported protocol, non-https endpoint, credentials in the endpoint, a literal credential in `headers_env` |
| `AR9K1` | `telemetry-repo-key-ignored` | warning | the repository config sets a key only user scope honors |
