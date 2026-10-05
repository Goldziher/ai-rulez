# Item-load telemetry and OTLP export

[Usage telemetry](usage-telemetry.md) answers "which skills are ever invoked". This page covers the next step:
which **rules, agents, context files and skills** a harness actually loaded, how many rules a session carries, and
optionally shipping those identifier-only events to an OpenTelemetry collector. Everything is off by default, the
local log works with no network, and a repository can never turn on network export for the people who clone it.

```console
$ ai-rulez telemetry hook              # hooks that record loads (merge into .claude/settings.json)
$ ai-rulez telemetry hook --format toml   # the same as [[hooks]] for config.toml; generate writes them
$ ai-rulez telemetry doctor            # resolved config, consent, buffer, last flush
$ ai-rulez telemetry flush             # ship the outbox now
$ ai-rulez report usage .ai-rulez/local/usage.jsonl   # rules per session, never-loaded rules, load reasons
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
| `digest` | content digest when known (`blake3:...`); skills use the index hash, rules the hash of the generated file | `blake3:0699fc6b...` |
| `source` | `hook`, `mcp` or `cli` | `hook` |
| `harness`, `role` | as given to the recorder | `claude`, `backend` |
| `served` | the load came through the MCP skills server | `false` |
| `session` | salted hash of the harness session id (16 hex), never the raw id | `5b1c0e9a7d3f2a64` |
| `outcome` | `loaded`, `used` or `abandoned` | `loaded` |
| `load_reason` | why it loaded (Claude Code: `session_start`, `nested_traversal`, `path_glob_match`, `include`, `compact`; agents: `subagent_start`, `subagent_stop`; MCP: `read`, `list`) | `path_glob_match` |
| `memory_type` | Claude Code's `User`, `Project`, `Local` or `Managed` | `Project` |
| `duration_ms` | subagent run time, from the paired Start and Stop events | `2500` |

Skill events are written to the usage log in the existing v2 line format, so `usage` readers are unaffected. The
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

Inferred, not verified: the mapping from a rule's generated file (`.claude/rules/<name>.md`) to the source rule id is
by file name, which is how ai-rulez names generated rules; `SubagentStop` carries no duration, so it is computed from
the paired `SubagentStart` and is absent when the Start was missed. Codex and Cursor have no verified equivalent of
`InstructionsLoaded` or the subagent events in this repository, so `telemetry hook --harness codex|cursor` prints the
skill-load template of `usage hook` unchanged.

## Consent and configuration

```toml
# .ai-rulez/config.toml, the local overlay, or the user config (~/.config/ai-rulez/config.toml)
[telemetry]
enabled         = false                    # local recording of item events
allow_network   = false                    # OTLP export needs this AND an endpoint; user scope only
otlp_endpoint   = "https://collector.example.org:4318"   # user scope only
otlp_protocol   = "http/json"              # the only implemented value
headers_env     = ["OTLP_HEADERS"]         # env var NAMES holding "k=v,k2=v2"; user scope only
service_name    = "ai-rulez"
sample          = 1.0                      # fraction of sessions exported, decided per session hash
include_paths   = false                    # user scope only
include_session = false                    # add the salted session hash to exported logs; user scope only
salt_file       = ""                       # user scope only
```

### The trust rule

A committed repository config is attacker-controlled input: cloning a repository must not start sending data to a
host the author picked. So:

| Key | Repository config and local overlay | User config / `AI_RULEZ_TELEMETRY_*` |
| --- | --- | --- |
| `enabled` | honored (local recording only, into a gitignored file) | honored |
| `service_name`, `sample` | honored | honored |
| `allow_network`, `otlp_endpoint`, `otlp_protocol`, `headers_env`, `include_paths`, `include_session`, `salt_file` | **ignored**, reported by `telemetry doctor` and strict finding `AR9K1` | honored |

The local overlay (`config.local.*`) counts as repository scope: it is machine-local by convention but lives in the
checkout. Network export is active only when `enabled`, `allow_network` and a valid endpoint are all present after
this filtering, so a repository can at most switch on the local log.

See the [trust model](trust-model.md) for the same rule across every knob.

Precedence, highest first: kill switches (`AI_RULEZ_TELEMETRY=off`, `DO_NOT_TRACK=1`: nothing is recorded or
exported) > environment (`AI_RULEZ_TELEMETRY`, `AI_RULEZ_TELEMETRY_ENDPOINT`, `_PROTOCOL`, `_ALLOW_NETWORK`,
`_HEADERS_ENV` as a comma list of names, `_SERVICE_NAME`, `_SAMPLE`, `_INCLUDE_PATHS`, `_INCLUDE_SESSION`,
`_SALT_FILE`) > user config > repository config > defaults.

Validation (`AR9K0`): https is required except for a loopback host; no credentials, query or fragment in the
endpoint; `headers_env` accepts an environment variable name only (`^[A-Z][A-Z0-9_]*$`, and a 20-character name
without an underscore is refused as an access-key id), so a pasted token or `Authorization=Bearer ...` is rejected and
never echoed back; `sample` is 0..1. Headers are read from the environment at flush time and never written anywhere.

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
3. The flush sends batches of 200 events to `/v1/logs` and `/v1/metrics` as gzipped OTLP/HTTP JSON, retrying 429, 502,
   503, 504 and network errors with exponential backoff (1 s, 2 s, 4 s, plus jitter; `Retry-After` honored up to
   30 s) and giving up until the next flush, which leaves the events in the outbox. 400, 401, 403, 404 and 413 are
   permanent: the batch is dropped and counted as rejected. Redirects are never followed. One flusher runs at a time.
4. Delivery is at-least-once; backends de-duplicate on `ai_rulez.event_id`.
5. A long-lived `ai-rulez mcp` server flushes from a timer and once more on exit.

Errors kept in `telemetry-state.json` contain a status code or a reason such as `timeout`, never the URL or a header.

## OTLP mapping

Resource attributes: `service.name` (default `ai-rulez`), `service.version`, `ai_rulez.schema_version`. No host,
process, OS or user attributes are added: there is no SDK resource detector.

**Logs**: one record per event, severity `INFO`, body `item <outcome>`, attribute `event.name` =
`ai_rulez.item.<outcome>`, time = the event time.

| Attribute | Event field | Gate |
| --- | --- | --- |
| `ai_rulez.item.kind`, `ai_rulez.item.id` | `kind`, `id` | |
| `ai_rulez.item.digest` | `digest` | |
| `ai_rulez.item.path` | `path` | `include_paths` |
| `ai_rulez.source`, `ai_rulez.harness`, `ai_rulez.role`, `ai_rulez.served` | same names | |
| `ai_rulez.session` | `session` | `include_session` |
| `ai_rulez.outcome`, `ai_rulez.load_reason`, `ai_rulez.memory_type` | same names | |
| `ai_rulez.duration_ms` | `duration_ms` | |
| `ai_rulez.event_id` | `event_id` | |

The table is `telemetry.Allowlist` in `internal/telemetry/allowlist.go`; a test fails when `Event` gains a field that
is neither listed nor mapped to the record envelope, so a new field cannot leave the machine by accident.

**Metrics** (delta temporality, batch-scoped start time):

| Metric | Type | Labels |
| --- | --- | --- |
| `ai_rulez.item.loads` | monotonic sum, `{load}` | `kind`, `id`, `harness`, `role`, `served`, `digest_short` (first 12 hex of the digest) |
| `ai_rulez.item.outcomes` | monotonic sum, `{load}` (outcome `used` or `abandoned`) | the labels above plus `outcome` |
| `ai_rulez.agent.duration` | histogram, `ms`, bounds 100, 500, 1000, 5000, 15000, 60000, 300000 | `kind`, `id`, `harness` |

Session and path are never metric labels.

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
`OTEL_RESOURCE_ATTRIBUTES` for Claude Code; ai-rulez adds no custom resource attributes, so use the collector's
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
assumes a long-lived process while hooks exit in milliseconds. Protobuf and gRPC are left for a later phase behind the
same `Encoder` seam; `otlp_protocol` rejects them today instead of silently falling back.

## Strict-validation codes

| Code | Name | Severity | Finds |
| --- | --- | --- | --- |
| `AR9K0` | `telemetry-config-invalid` | error | invalid `[telemetry]` value: bad range or enum, unsupported protocol, non-https endpoint, credentials in the endpoint, a literal credential in `headers_env` |
| `AR9K1` | `telemetry-repo-key-ignored` | warning | the repository config sets a key only user scope honors |
