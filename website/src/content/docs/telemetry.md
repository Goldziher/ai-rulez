---
title: "Item-load telemetry and OTLP export"
description: "Item-load telemetry records which rules, agents and context files a harness actually loaded, so you can see how many rules a session carries and which ones never load."
---

Item-load telemetry records which rules, agents and context files a harness actually loaded, so you can see how many
rules a session carries and which ones never load. It can also ship those identifier-only events, together with the
[skill usage](/ai-rulez/usage-telemetry/) lines, to an OpenTelemetry (OTLP) collector you run. Everything is off by default,
the local log needs no network, and a repository can never switch on network export for the people who clone it.

This page covers item events, consent, export and the OTLP format. Skill usage, the hook setup, feedback and the
reports are on [Usage telemetry](/ai-rulez/usage-telemetry/).

Recording rule and context loads currently supports Claude Code only (its `InstructionsLoaded` hook). Agent runs are
also recorded for Codex and Cursor.

## Example

Wire the hooks (see [Wiring the hooks](/ai-rulez/usage-telemetry/#wiring-the-hooks)) and consent to export from your own
machine. `telemetry enable` turns recording on for you; `[telemetry] enabled = true` in the repository is only needed
for local-only recording without export:

```toml
# .ai-rulez/config.toml (excerpt)
[telemetry]
enabled = true
```

```console
$ ai-rulez telemetry enable --endpoint https://collector.acme.example:4318
consent recorded for https://collector.acme.example:4318 (http/json)
  stored in ~/.config/ai-rulez/telemetry-consent.json (mode 0600)
  exports: event.name, ai_rulez.item.kind, ai_rulez.item.id, ai_rulez.item.digest, ...
  withheld: ai_rulez.item.path, ai_rulez.session
  cursor: at the end of the usage log; only events recorded from now on are exported (--backfill sends history)
export is on. Hooks record events (ai-rulez telemetry hook); preview what is sent with `ai-rulez telemetry preview`.

$ ai-rulez telemetry status
telemetry: export on, recording on
consent:   granted by the consent record (2026-11-05T16:22:33.245Z) for collector.acme.example:4318
endpoint:  collector.acme.example:4318 (http/json, host only)
...
pending:   0 in the outbox, 0 in the usage log past the cursor
last flush: never
totals:    sent 0, dropped 0, rejected 0, failed flushes 0 (0 in a row)
```

From then on, hooks record events and flush them to the collector in the background. `ai-rulez telemetry disable`
withdraws consent.

## Commands

| Command | What it does |
| --- | --- |
| `telemetry enable --endpoint URL` | Store your consent to export to one collector. |
| `telemetry disable` | Delete the consent record. Export stops; the log and outbox are kept. |
| `telemetry status` | On or off, consent, pending events, delivery failures. |
| `telemetry doctor` | Resolved settings with their source, consent state, outbox and last flush. |
| `telemetry preview` | Print exactly what an export would send. Sends nothing. |
| `telemetry flush` | Send the outbox now (hooks do this in the background). |
| `telemetry export --to file FILE` | Write the usage log as an OTLP JSON file. No network, no consent needed. |
| `telemetry export --to otlp` | Push the usage log past the export cursor to the consented collector. |
| `telemetry prune --keep-days N` | Delete log lines older than N days that were already exported. |
| `telemetry report` | Skill usage plus rules per session and never-loaded items (see [below](#the-item-report)). |
| `telemetry hook`, `telemetry record` | The hook template and the command it runs ([Usage telemetry](/ai-rulez/usage-telemetry/#wiring-the-hooks)). |

`status` and `doctor` take `--format json`. `enable`, `disable`, `status`, `doctor`, `flush`, `preview`, `prune` and
`record` accept `--root` (default `$CLAUDE_PROJECT_DIR`, else the directory holding the configuration).

## What is collected

One `item_event` line per load, appended to the same `.ai-rulez/local/usage.jsonl` as skill usage:

```json
{"v":1,"event":"item_event","ts":"2026-11-05T16:22:17Z","event_id":"084c1be76a6c5207","kind":"rule","id":"atomic-commits","source":"hook","harness":"claude","session":"abf5a79b8deede5d","outcome":"loaded","load_reason":"session_start","memory_type":"Project"}
```

| Field | Meaning | Example |
| --- | --- | --- |
| `v`, `event`, `ts` | Schema version (`1`), `item_event`, UTC time | `1` |
| `event_id` | 16 hex digits; stays the same when a line is replayed, so a backend can de-duplicate | `084c1be76a6c5207` |
| `kind` | `skill`, `rule`, `agent`, `command` or `context` | `rule` |
| `id` | A rule's file name without `.md`, an agent type, a skill name, or a context file's repository-relative path | `atomic-commits` |
| `path` | Repository-relative path, exported only with `include_paths = true` | `.claude/rules/atomic-commits.md` |
| `digest`, `digest_scheme` | Content digest when known; for skills the canonical `ai-rulez/skill/v1` digest | `blake3:fa37c294...` |
| `source` | `hook`, `mcp` or `cli` | `hook` |
| `harness`, `role` | As given to the recorder | `claude` |
| `served` | The load came through the MCP skills server | `false` |
| `session` | Salted hash of the harness session id, never the raw id; exported only with `include_session = true` | `abf5a79b8deede5d` |
| `outcome` | `loaded`, `used` or `abandoned` (an agent's stop event is `used`) | `loaded` |
| `load_reason` | Claude Code: `session_start`, `nested_traversal`, `path_glob_match`, `include`, `compact`; agents: `subagent_start`, `subagent_stop`; MCP: `read`, `list` | `session_start` |
| `memory_type` | Claude Code's `User`, `Project`, `Local` or `Managed` | `Project` |
| `duration_ms` | Agent run time, from the paired start and stop events (Cursor reports it directly) | `2500` |

A file outside the repository (a user-level `~/.claude/rules/x.md`) contributes only its base name.

With telemetry on, `ai-rulez mcp` also records `source=mcp` events: after a successful `read_skill`, `read_rule`,
`read_context` or `get_skill` (reason `read`), and after `list_*` and `search_skills` calls with the id `_list`
(reason `list`; reports count these under load reasons and never list them as items). A rule's generated
file is mapped back to the source rule by its file name.

**Never collected**: prompts, model output, tool inputs or results, file contents, command arguments, transcripts,
user, host or repository names, environment values, absolute paths, the raw session id and feedback notes. The hook
handler decodes only the fields above from a hook payload, so other payload fields (`prompt`, `transcript_path`,
`last_assistant_message`, ...) cannot reach an event.

### Eval results

An export can also carry `eval_result` events (`--with-evals` on `export` and `preview`): one per verified result in
`.ai-rulez/eval-results.json`, built at export time and never written to the usage log. It holds the skill id, the
canonical digest the run covered, the harness, the run date and the four scores (`pass_rate`, `trigger_precision`,
`trigger_recall`, `ablation_delta`), and no case, prompt, output or load field. Results that are not signed with your
key, and activation-only results, are never exported. See [Evals](/ai-rulez/evals/#scores-and-the-results-file).

## The item report

`ai-rulez telemetry report` adds these sections when the log holds item events:

```console
$ ai-rulez telemetry report
...
Item loads: 4 events in 1 sessions
Rules per session: median 1.0, mean 1.00, max 1 (over 1 sessions)

Rules loaded (1)
  atomic-commits                                1 loads     1 sessions  last 2026-11-05T16:22:17Z  [session_start=1]
Rules never loaded (0)

Agents loaded (1)
  code-reviewer                                 1 loads     1 sessions  last 2026-11-05T16:22:17Z  [subagent_start=1]
Agents never loaded (0)

Context files loaded (1)
  CLAUDE.md                                     1 loads     1 sessions  last 2026-11-05T16:22:17Z  [session_start=1]
Context files never loaded (1)
  AGENTS.md
...
```

"Never loaded" joins the log with the generated manifest, which a metrics backend cannot do: an item that never
loaded has no series. Flags are listed under [Reports](/ai-rulez/usage-telemetry/#reports).

## Configuration

```toml
[telemetry]
enabled         = false        # local recording into .ai-rulez/local/usage.jsonl
sample          = 1.0          # fraction of sessions exported, decided per session hash (0 to 1)

# User scope only (the user config file or AI_RULEZ_TELEMETRY_*); ignored in a repository config:
allow_network   = false        # consent as a setting; `telemetry enable` is the other way
otlp_endpoint   = "https://collector.acme.example:4318"
otlp_protocol   = "http/json"  # http/json, http/protobuf or grpc
headers_env     = ["OTLP_HEADERS"]   # NAMES of variables holding "k=v,k2=v2"
service_name    = "ai-rulez"
include_paths   = false        # export item paths
include_session = false        # export the salted session hash
salt_file       = ""

[telemetry.resource]           # extra OTLP resource attributes; user scope only
team = "platform"
"deployment.environment" = "prod"
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `enabled` | bool | `false` | Record item events locally. |
| `sample` | float | `1.0` | Share of sessions exported, decided per session hash (`0` exports nothing). The local log records every session. |
| `allow_network` | bool | `false` | Consent to export without a consent record. User scope only. |
| `otlp_endpoint` | string | none | Collector base URL (`https://`; `http://` only for a loopback host). For `grpc`, `host[:port]`. User scope only. |
| `otlp_protocol` | string | `http/json` | `http/json`, `http/protobuf` or `grpc`. User scope only. |
| `headers_env` | list | `[]` | Names of environment variables whose `k=v,k2=v2` values become request headers. Read at flush time, never written anywhere. User scope only. |
| `service_name` | string | `ai-rulez` | The `service.name` resource attribute. User scope only. |
| `include_paths` | bool | `false` | Export `ai_rulez.item.path`. User scope only. |
| `include_session` | bool | `false` | Export `ai_rulez.session`. User scope only. |
| `salt_file` | string | `usage.salt` beside the log | File holding the session-hash salt. User scope only. |
| `resource` | table | none | Up to 8 resource attributes. Keys are lower-case dotted names of at most 64 characters, values non-empty strings of at most 128. The `service.*`, `host.*`, `user.*`, `process.*`, `os.*`, `cloud.*`, `k8s.*`, `container.*`, `telemetry.*` and `ai_rulez.*` namespaces are reserved. User scope only. |

### Environment variables

Precedence, highest first: kill switches, environment, user config, repository config, defaults.

| Variable | Meaning |
| --- | --- |
| `AI_RULEZ_TELEMETRY=off`, `DO_NOT_TRACK=1` | Kill switches: nothing is recorded or exported. |
| `AI_RULEZ_TELEMETRY` | Turn recording on or off. |
| `AI_RULEZ_TELEMETRY_ENDPOINT`, `_PROTOCOL`, `_ALLOW_NETWORK`, `_SERVICE_NAME`, `_SAMPLE`, `_INCLUDE_PATHS`, `_INCLUDE_SESSION`, `_SALT_FILE` | The keys of the same name. |
| `AI_RULEZ_TELEMETRY_HEADERS_ENV` | Comma-separated variable names. |
| `AI_RULEZ_TELEMETRY_RESOURCE` | `key=value,key2=value2`; overrides user-config entries per key. |

### The trust rule

A committed repository config is input from whoever wrote it: cloning a repository must not start sending data to a
host its author picked.

| Key | Repository config and `config.local.*` | User config and environment |
| --- | --- | --- |
| `enabled`, `sample` | honored (local recording only) | honored |
| every other key | ignored, reported by `telemetry doctor` and `AR9K1` | honored |

The local overlay lives in the checkout, so it counts as repository scope. Network export is active only when
recording is on, consent is present and a valid endpoint remains after this filtering, so a repository can at most turn
on the local log. The [trust model](/ai-rulez/trust-model/) applies the same rule to every setting that reaches the network.

### The consent record

`telemetry enable` stores your consent in `$XDG_CONFIG_HOME/ai-rulez/telemetry-consent.json` (else
`~/.config/ai-rulez/`), mode 0600:

```json
{
  "version": 1,
  "endpoint": "https://collector.acme.example:4318",
  "protocol": "http/json",
  "scope": { "fields": ["event.name", "ai_rulez.item.kind", "..."], "fields_hash": "a1f742a2..." },
  "granted_at": "2026-11-05T16:22:33.245Z",
  "ai_rulez_version": "5.10.0"
}
```

```bash
ai-rulez telemetry enable --endpoint https://collector.acme.example:4318
ai-rulez telemetry enable --endpoint collector.acme.example:4317 --protocol grpc --include-session
ai-rulez telemetry enable --endpoint https://collector.acme.example:4318 --backfill   # also send existing history
ai-rulez telemetry disable
```

| `enable` flag | Meaning |
| --- | --- |
| `--endpoint` | Collector endpoint (default: the configured one). |
| `--protocol` | `http/json` (default), `http/protobuf` or `grpc`. |
| `--include-paths`, `--include-session` | Also consent to exporting item paths or the session hash. |
| `--backfill` | Also export the events already in the usage log. |

- **Only you can grant it.** The record is read from the user config directory only; a repository cannot grant, edit
  or point to one. A record that is group- or world-writable, a symlink, malformed or has an unknown field is ignored,
  and `telemetry status` says why.
- **It covers what you agreed to.** It names the endpoint, the protocol and a hash of the exported attribute names. A
  different endpoint or protocol, an opt-in you turned on later (`include_paths`, `include_session`), or a newer
  release that exports more attributes makes it stale: nothing is sent until you run `enable` again.
- **It supplies, never overrides.** With a valid record, recording turns on for you, and the consented endpoint,
  protocol and opt-ins are used where your user config and environment set none. `AI_RULEZ_TELEMETRY_ALLOW_NETWORK=0`
  refuses export even with a record. The kill switches and an organization [policy](/ai-rulez/policy/) win over everything.
- **It is not retroactive.** Only events recorded after `enable` are sent, unless you pass `--backfill`.
- **`disable` deletes it.** Export stops, and so does recording that the record turned on. Recording that your config
  or `AI_RULEZ_TELEMETRY` turns on continues, and `disable` says so. The usage log and outbox are kept.

## How export works

Hooks are short-lived, so the recorder never touches the network:

1. A hook appends the event to the usage log and, while export is active, to an outbox
   (`.ai-rulez/local/telemetry-outbox.jsonl`, mode 0600, at most 10,000 events, oldest dropped first). If the outbox
   is busy for 25 ms the event is not queued (a later catch-up queues it); the harness is never delayed.
2. When the outbox is due, the recorder starts one detached `telemetry flush --background` (at most one a minute,
   8-second deadline, always exits 0). A long-running `ai-rulez mcp` server flushes from a timer and once on exit.
3. A flush first catches up: it queues log lines past the [export cursor](#the-export-cursor-and-catch-up) that the
   outbox does not hold yet. It then sends batches of 200 events to `/v1/logs` and `/v1/metrics`, gzipped.
4. 408, 425, 429, every 5xx except 501 and 505, and network errors are retried with exponential backoff (1 s, 2 s,
   4 s, plus jitter; `Retry-After` honored up to 30 s); if retries run out, the events stay in the outbox for the next
   flush. Every other status (400, 401, 403, 404, 413, 501, 505, a 3xx redirect, ...) drops the batch and counts it as
   rejected. Redirects are never followed.
5. Delivery is at-least-once. Backends de-duplicate on `ai_rulez.event_id`, which is the same on every attempt.

Background failures are silent to the harness but recorded: `telemetry status` shows the last error and the
delivered, dropped, rejected and failed-flush counts. Stored errors hold a status code or a reason such as `timeout`,
never the URL or a header.

| `flush` flag | Default | Meaning |
| --- | --- | --- |
| `--timeout` | `8s` | Overall deadline; at most `30s`. |
| `--background` | off | Silent mode used by hooks: exit 0 whatever happens. |

### The export cursor and catch-up

The usage log is the source of truth; the outbox is only a delivery queue. The cursor
(`.ai-rulez/local/telemetry-cursor.json`) records how far into the log events are accounted for, plus the ids of the
last 4,096 delivered or rejected events, so a catch-up never sends an event twice.

- A cursor that was never set is placed at the end of the log, and nothing is queued: history is exported only when
  you ask (`telemetry enable --backfill`, `telemetry export --to otlp --all`).
- One catch-up queues at most 2,000 events; the next flush picks up the rest.
- Sampling (`sample`) applies to catch-up events too.
- A rotated or truncated log is detected and read from the start.

### Pushing by hand

`telemetry export --to otlp` runs the same catch-up and delivery in the foreground. It refuses without consent and
exits 1 when delivery fails, so a CI job notices.

```console
$ ai-rulez telemetry export --to otlp
queued 12 events from the usage log and 0 eval results (3 already queued or delivered); delivered 12 in 1 batches
$ ai-rulez telemetry export --to otlp --dry-run
would queue 0 events from the usage log (0 already queued or delivered), 0 eval results; 0 events already wait in the outbox (nothing sent)
```

| `export` flag | Default | Meaning |
| --- | --- | --- |
| `--to` | required | `file` or `otlp`. |
| `--file` | | Destination for `--to file` (or pass it as the argument). |
| `--log` | `<config dir>/local/usage.jsonl` | Usage log to export. |
| `--all` | off | `--to otlp`: start from the beginning of the log instead of the cursor. |
| `--max-batches` | `10` | `--to otlp`: stop after this many catch-up rounds of up to 2,000 events. |
| `--with-evals` | off | Also export verified [eval results](#eval-results). |
| `--evals` | `<config dir>/eval-results.json` | Eval results file for `--with-evals`. |
| `-n`, `--dry-run` | off | Report only: write, queue and send nothing. |

### Exporting to a file

`telemetry export --to file` writes the usage log as OTLP JSON for an air-gapped collector, a cross-repository job or
your own tooling. It is local: no network, no consent record, and it works whether or not `[telemetry]` is enabled or a
kill switch is set.

```console
$ ai-rulez telemetry export --to file usage.ndjson
wrote 7 events in 1 batches to usage.ndjson
```

- Each line is one logs request (`resourceLogs`), exactly the body of an OTLP/HTTP `POST /v1/logs`, so the
  OpenTelemetry Collector's `otlpjsonfile` receiver reads it as is. The file carries logs only, no metrics.
- It is built by the same encoder and allowlist as the network export: unknown log fields are never read, invalid
  values are dropped, and the session and path attributes stay out unless `include_session` and `include_paths` are
  on in your user config.
- The output is deterministic (log order, de-duplicated by `event_id`, no clock read) and replaced atomically.

`telemetry report evals --from-otlp` reads such files back; see [Joining several logs](/ai-rulez/usage-telemetry/#joining-several-logs).

### Pruning the usage log

```bash
ai-rulez telemetry prune --keep-days 90 --dry-run
```

`telemetry prune --keep-days N` deletes log lines older than N days, but only lines behind the export cursor: an event
still waiting to be sent is kept however old. With no cursor (export was never on) age alone decides. The file is
rewritten atomically and the cursor moves to match.

| Flag | Meaning |
| --- | --- |
| `--keep-days N` | Keep lines from the last N days (required). |
| `--ignore-cursor` | Prune by age alone, including lines not yet exported. |
| `--log FILE` | Prune another log (it has no cursor, so age alone decides). |
| `-n`, `--dry-run` | Report what would be removed. |

The prune refuses when the cursor describes another log (the file was replaced since the last flush); run
`telemetry flush` first, or pass `--ignore-cursor`. It waits up to five seconds for running hooks and exits 1
(`usage log is locked`) if they do not finish.

## Previewing an export

`telemetry preview` encodes pending events with the exporter's own encoder and prints each request, without opening a
connection. It works whether or not export is on, so you can audit the payload before you consent.

```console
$ ai-rulez telemetry preview --limit 2
source: .ai-rulez/local/usage.jsonl (usage log) (7 events, previewing 2)
export: off (no consent ...; no endpoint ...); this is what would be sent once it is on
timestamps: the observation and metric times below are the newest previewed event's; a flush stamps its own clock

POST <no endpoint configured>/v1/logs  (2 events, gzip, 494 bytes)
{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"ai-rulez"}}, ...

POST <no endpoint configured>/v1/metrics  (2 events, gzip, 414 bytes)
{"resourceMetrics":[ ... ]}

fields exported: event.name, ai_rulez.item.kind, ai_rulez.item.id, ...
fields withheld: ai_rulez.item.path, ai_rulez.session

Nothing was sent.
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--limit N` | `5` | Preview the first N events (`0` for all). |
| `--log FILE` | the outbox when export is on, else the usage log | Preview another log. |
| `--with-evals` | off | Also preview the eval results `export --with-evals` would send. |

The destination shows only the endpoint's scheme, host and path; headers are never printed. `fields withheld` lists
the attributes whose opt-in (`include_paths`, `include_session`) is off. The `http/protobuf` and `grpc` transports send
the same messages as the JSON shown.

## OTLP mapping

Resource attributes: `service.name` (default `ai-rulez`), `service.version`, `ai_rulez.schema_version`, then your
`[telemetry.resource]` labels. No host, process, OS or user attributes are added.

**Logs**: one record per event, severity `INFO`, body `item <outcome>`, `event.name` = `ai_rulez.item.<outcome>`. An
eval result is a record with body `eval result` and `event.name` = `ai_rulez.eval.result`.

| Attribute | Event field | Exported when |
| --- | --- | --- |
| `ai_rulez.item.kind`, `ai_rulez.item.id` | `kind`, `id` | always |
| `ai_rulez.item.digest`, `ai_rulez.item.digest_scheme` | `digest`, `digest_scheme` | known |
| `ai_rulez.item.path` | `path` | `include_paths` |
| `ai_rulez.source`, `ai_rulez.harness`, `ai_rulez.role`, `ai_rulez.served` | same names | always |
| `ai_rulez.session` | `session` | `include_session` |
| `ai_rulez.outcome`, `ai_rulez.load_reason`, `ai_rulez.memory_type` | same names | always |
| `ai_rulez.duration_ms` | `duration_ms` | known |
| `ai_rulez.event_id` | `event_id` | always |
| `ai_rulez.eval.pass_rate`, `.trigger_precision`, `.trigger_recall`, `.ablation_delta` | scores | eval results only |

**Metrics** (delta temporality):

| Metric | Type | Labels |
| --- | --- | --- |
| `ai_rulez.item.loads` | monotonic sum, `{load}` | `kind`, `id`, `harness`, `role`, `served`, `digest_short` (first 12 hex digits) |
| `ai_rulez.item.outcomes` | monotonic sum, `{load}` (`used` or `abandoned`) | the labels above plus `outcome` |
| `ai_rulez.agent.duration` | histogram, `ms`, bounds 100, 500, 1000, 5000, 15000, 60000, 300000 | `kind`, `id`, `harness` |
| `ai_rulez.skill.eval.pass_rate`, `.trigger_precision`, `.trigger_recall`, `.ablation_delta` | gauge, `1` | `id`, `harness` |

Session and path are never metric labels.

## Transports

| `otlp_protocol` | Wire | Endpoint |
| --- | --- | --- |
| `http/json` (default) | OTLP/HTTP, gzipped JSON | base URL; `/v1/logs` and `/v1/metrics` are appended |
| `http/protobuf` | OTLP/HTTP, gzipped protobuf | same |
| `grpc` | OTLP/gRPC `LogsService` and `MetricsService`, gzip | `host[:port]` (default port 4317), no path; TLS unless `http://` to a loopback host |

The exported attributes are identical on all three. Headers from `headers_env` travel as HTTP headers or gRPC metadata.
gRPC `Unavailable`, `ResourceExhausted`, `DeadlineExceeded`, `Aborted`, `Canceled`, `Internal` and `Unknown` are
retried; every other code rejects the batch.

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

The Prometheus exporter renders `ai_rulez.item.loads` as `ai_rulez_item_loads_total` (names depend on your pipeline):

```promql
# Top skills by role over the last week
topk(10, sum by (id, role) (increase(ai_rulez_item_loads_total{kind="skill"}[7d])))

# Items whose series exist but that were not loaded for 30 days
sum by (kind, id) (increase(ai_rulez_item_loads_total[30d])) == 0
```

Load reasons are on the log records, not the metrics. For "never loaded" and rules per session, use
[`telemetry report`](#the-item-report), which reads the local log exactly.

### Joining with the harness's own telemetry

Some harnesses export their own OpenTelemetry data; Claude Code, for example, exports token, cost and session metrics
when `CLAUDE_CODE_ENABLE_TELEMETRY=1` and the standard `OTEL_*` variables are set. ai-rulez adds only what a harness
cannot know: which ai-rulez item a load was. Point both at the same collector and give both the same labels
(`OTEL_RESOURCE_ATTRIBUTES` for the harness, `[telemetry.resource]` for ai-rulez) to join them.

## Data egress

Only the allowlisted attributes above leave the machine, to the one endpoint you configured, over https (http only to
loopback). ai-rulez operates no service and has no default endpoint, so residency is wherever your collector runs.
Item ids can themselves be sensitive (a private skill name); they go only to your collector. The session salt stays on
the machine, so sessions cannot be linked across machines. Delete `.ai-rulez/local/telemetry-*` to discard the
outbox, cursor and state.

## Validation codes

| Code | Name | Severity | Finds |
| --- | --- | --- | --- |
| `AR9K0` | `telemetry-config-invalid` | error | An invalid `[telemetry]` value: `sample` outside 0 to 1, an unknown protocol, a non-https endpoint to a non-loopback host, credentials, a query or a fragment in the endpoint, a grpc endpoint with a path, a `headers_env` entry that is not a variable name (a pasted token is rejected and never echoed), an invalid `[telemetry.resource]` table. Export stays off. |
| `AR9K1` | `telemetry-repo-key-ignored` | warning | The repository config sets a key that only user scope honors. |

```console
$ ai-rulez telemetry enable --endpoint http://collector.acme.example:4318
Error: telemetry.otlp_endpoint: plain http is only allowed for a loopback host (use https)
```
