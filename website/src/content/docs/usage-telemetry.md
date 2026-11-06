---
title: "Usage telemetry"
description: "Usage telemetry tells you which skills your team's agents actually use, which ones were edited since they were last used, and which ones never fire and can be retired."
---

Usage telemetry tells you which skills your team's agents actually use, which ones were edited since they were last
used, and which ones never fire and can be retired. Everything stays on your machine: a hook appends one
identifier-only line per skill load to a local log, and `ai-rulez telemetry report` joins that log with your source
skills, feedback and [eval scores](/ai-rulez/evals/). Nothing is enabled by default and nothing here makes a network call.

This page covers skill usage and the local log. Rule, agent and context-file loads, and optional export to an
OpenTelemetry collector, are on [Item-load telemetry and OTLP export](/ai-rulez/telemetry/).

## Example

Turn on the skills index and generate:

```toml
# .ai-rulez/config.toml (excerpt)
presets = ["claude", "codex", "cursor"]

[usage]
skills_index = true
```

```bash
ai-rulez generate
ai-rulez telemetry hook --syntax toml    # paste the printed [[hooks]] into config.toml, then generate again
```

Work as usual. Each skill load appends a line to `.ai-rulez/local/usage.jsonl`. Then:

```console
$ ai-rulez telemetry report
Skill usage: 3 events

Used (1)
  deploy-staging                                3  last 2026-11-05T16:22:17Z  feedback: stale 1

Never used (1)
  release-notes

Changed since used (0)

Not in the index (0)
```

`release-notes` was never loaded: it is a candidate for removal, or its description needs work.

## The skills index

With `[usage] skills_index = true`, `generate` writes `.ai-rulez/skills-index.json`. It is derived from the source
tree, byte-stable across runs and safe to commit. Turning the option off removes the file on the next `generate`.

```json
{
  "schema_version": 1,
  "skills": [
    {
      "id": "deploy-staging",
      "kind": "skill",
      "source": ".ai-rulez/skills/deploy-staging/SKILL.md",
      "hash": "blake3:43646786af80...",
      "digest": "sha256:11b96b692d42...",
      "owner": "platform",
      "version": "1.2.0",
      "outputs": {
        "claude": [".claude/skills/deploy-staging/SKILL.md"],
        "shared": [".agents/skills/deploy-staging/SKILL.md"]
      }
    }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `id` | The skill's directory name, which is the name every harness invokes it by. |
| `kind` | `skill`, or `command` for a command a harness runs as a skill (for example commands the `claude` preset writes into `.claude/skills`). |
| `domain` | The domain the skill belongs to; omitted for root skills. |
| `source` | The authored `SKILL.md`. |
| `hash` | blake3 of the authored skill (`SKILL.md`, then each bundled resource in path order). Changes when, and only when, the skill changes. |
| `digest` | The skill's canonical digest, the same one [`ai-rulez lock`](/ai-rulez/lockfile/) pins (scheme `ai-rulez/skill/v1`: `SKILL.md` plus `references/`, `scripts/` and `assets/`). The skill's `evals/` directory is not part of it, so editing a case does not make the skill look edited. Usage lines and eval results carry this digest, which is how a score is tied to the version that was used. |
| `owner`, `version` | From the `owner` and `version` frontmatter keys, when set. `[lint] require_metadata` can make them mandatory (see [strict validation](/ai-rulez/strict-validation/)). |
| `outputs` | Per preset, the `SKILL.md` files it writes. Paths that several presets share (such as `.agents/skills`) are listed under `shared`. |

The index covers the active profile at the repository root. Scoped (monorepo) outputs and machine-local skills are not
listed.

## Wiring the hooks

`ai-rulez telemetry hook` prints a hook template that runs `ai-rulez telemetry record` on the events that load a
skill. Nothing installs it for you: either merge the JSON into the harness's hooks file, or paste the `--syntax toml`
form into `config.toml` as [`[[hooks]]`](/ai-rulez/settings/) so `generate` keeps it in sync without touching hooks you wrote
by hand.

```bash
ai-rulez telemetry hook                     # Claude Code, JSON for .claude/settings.json
ai-rulez telemetry hook --syntax toml       # the same as [[hooks]] groups for config.toml
ai-rulez telemetry hook --harness codex     # merge into .codex/hooks.json
ai-rulez telemetry hook --harness cursor    # merge into .cursor/hooks.json
ai-rulez telemetry hook -o hooks.json       # write to a file
```

```toml
# printed by `ai-rulez telemetry hook --syntax toml` (abridged)
[[hooks]]
event = "PreToolUse"
targets = ["claude"]
matcher = "Skill"
[[hooks.hooks]]
command = "ai-rulez telemetry record --log \"${CLAUDE_PROJECT_DIR}/.ai-rulez/local/usage.jsonl\""
async = true
timeout = 5
...
```

The template registers these events. The skill events go to this page's log; the rule, agent and context events are
described on [Item-load telemetry](/ai-rulez/telemetry/#what-is-collected).

| Harness | Event (matcher) | Records |
| --- | --- | --- |
| Claude Code | `PreToolUse` (`Skill`) | a skill the model invoked (`tool_input.skill`), `invocation: "tool"` |
| Claude Code | `UserPromptExpansion` | a skill the user invoked by slash command (`command_name`), `invocation: "slash"` |
| Claude Code | `InstructionsLoaded`, `SubagentStart`, `SubagentStop` | rule, context-file and agent loads |
| Codex | `PreToolUse` (`Bash`) | a shell command that reads `skills/<id>/SKILL.md`, `invocation: "read"` |
| Codex | `SubagentStart`, `SubagentStop` | agent runs |
| Cursor | `preToolUse` (`Shell`) | a shell command that reads `skills/<id>/SKILL.md`, `invocation: "read"` |
| Cursor | `subagentStart`, `subagentStop` | agent runs |

Codex and Cursor have no skill tool: they load a skill by reading its `SKILL.md`, so the recorder infers a load from the
path. It counts a read tool (`Read`, `read_file`, `view`, `open`) given `skills/<id>/SKILL.md`, and a shell reader
(`cat`, `head`, `tail`, `less`, `more`, `bat`, `nl`, `sed` without `-i`, `Get-Content`) run on that path, including after
`cd x &&`, variable prefixes and inside pipelines. Writes and edits are never loads (`sed -i`, `tee`, a redirect into the
file, `git add`, `rm`, editor tools). The generated templates match only the shell tool (`Bash` for Codex, `Shell` for
Cursor); widen the matcher if you also want reads through the file-read tool. Check the log after wiring the hook; the
harnesses' payload fields may change.

Other harnesses (`--harness gemini`, `copilot`, ...) get a warning and no template, because their skill-load payload is
not documented:

```console
$ ai-rulez telemetry hook --harness gemini
warning: no usage hook template for the gemini harness: its skill-load hook payload is not documented in ai-rulez (supported: claude, codex, cursor)
```

### Hook and record flags

`telemetry hook` accepts the same `--log`, `--index`, `--role` and `--sink-command` flags as `record` and writes them
into the generated command.

| Flag | Command | Meaning |
| --- | --- | --- |
| `--harness` | `hook`, `record` | `claude` (default), `codex` or `cursor`. |
| `--syntax` | `hook` | `json` (default, a hooks block) or `toml` (`[[hooks]]` groups). |
| `--executable` | `hook` | Command the hook runs (default `ai-rulez`). Quoted as one word, so `npx -y ai-rulez` is not split. |
| `-o`, `--output` | `hook` | Write the template to a file instead of standard output. |
| `--log` | `hook`, `record` | Log file (default `.ai-rulez/local/usage.jsonl`). |
| `--index` | `hook`, `record` | Skills index used to resolve hashes and digests (default: `.ai-rulez/skills-index.json` below the hook payload's `cwd`; a payload without `cwd` logs no hash or digest). |
| `--role` | `hook`, `record` | Active role, recorded as given (else `$AI_RULEZ_ROLE`). |
| `--sink-command` | `hook`, `record` | Shell command that also receives each log line on standard input. |
| `--outcome` | `record` | `loaded` (default), `used` or `abandoned`, for a hook you wire to a later event. |
| `--served` | `record` | Mark the load as served by the MCP skills server. |
| `--salt-file` | `record` | File holding the session-hash salt (default `usage.salt` beside the log; `$AI_RULEZ_USAGE_SALT` wins). |
| `--root` | `record` | Project root (default `$CLAUDE_PROJECT_DIR`, else the directory holding the configuration). |

`ai-rulez telemetry record` never fails a session: it prints nothing on standard output, reports problems on standard
error and always exits 0. It ignores events it does not handle.

A sink command gets 3 seconds per line; after that its process group is killed. A failing sink does not drop the
entry, which still goes to the log. The same limits apply to `ai-rulez mcp --usage-sink`.

## The usage log

Each skill load appends one JSON line to the log, which lives under the machine-local `.ai-rulez/local/` directory and
is never committed:

```json
{"v":3,"ts":"2026-11-05T16:22:17Z","event":"skill_invoked","skill":"deploy-staging","id":"deploy-staging","hash":"blake3:43646786af80...","session":"abf5a79b8deede5d","invocation":"tool","harness":"claude","outcome":"loaded","digest":"sha256:11b96b692d42...","digest_scheme":"ai-rulez/skill/v1","event_id":"322f26b2184274ce"}
```

| Field | Meaning |
| --- | --- |
| `v` | Log format version, currently `3`. |
| `ts`, `event` | UTC time; `skill_invoked`. |
| `skill`, `id` | The name the harness reported, and the index id it resolves to (a `plugin:` prefix is dropped). |
| `hash` | The index hash at load time. |
| `digest`, `digest_scheme` | The canonical digest from the index and its scheme (`ai-rulez/skill/v1`). A load through the MCP skills server without an index digest logs the served skill's digest under `ai-rulez/served-skill/v1`, which does not join with eval results. Omitted when the skill is not in the index. |
| `invocation` | `tool`, `slash` or `read`. |
| `harness`, `role` | As given to the recorder; `role` is omitted when unset. |
| `session` | A salted hash of the harness session id (16 hex digits), never the raw id; see [What is collected](/ai-rulez/telemetry/#what-is-collected). The salt is in `usage.salt` beside the log. |
| `outcome` | `loaded`, `used` or `abandoned`. |
| `served` | `true` for a load through the MCP skills server; omitted otherwise. |
| `event_id` | 16 hex digits identifying the line, so a merged or replayed line is counted once. Derived from the machine salt, so it links nothing across machines. |

**Never recorded**: prompts, slash-command arguments, tool inputs other than the skill name (or, for Codex and Cursor,
the `SKILL.md` path), transcripts, raw session ids and file contents. The recorder decodes only the fields it needs, so
nothing else can reach the log.

The recorder refuses to write through a symlink: if the log or any directory between the project root and the log is a
symlink, the write fails. A committed symlink therefore cannot redirect the log. Point `--log` at a real directory.

Older log lines stay readable: version 1 lines (which may hold a raw session id; delete or rotate such a log if it must
not keep them) and version 2 lines (no digest or event id) are reported, and unknown fields of newer lines are ignored.

## Feedback

```bash
ai-rulez telemetry feedback deploy-staging --kind stale --note-file ./why.txt
```

Records that a skill `misled` you, is `stale`, is `wrong`, or was `great`. One line goes to
`.ai-rulez/local/feedback.jsonl` (change it with `--log`) with the skill, its index hash, the kind, and `--harness` and
`--role` when given. With `--note-file` the text is copied to `feedback-notes/` beside the log (mode 0600, at most
64 KB) and only the file name is logged. A note never enters a log line, the index or `eval-results.json`, so it does
not leave the machine unless you copy it.

Feedback changes nothing by itself: `telemetry report` shows the counts, and
[`telemetry report evals`](/ai-rulez/evals/#reports) recommends a rewrite when `misled`, `wrong` and `stale` outweigh `great`.

## Reports

```bash
ai-rulez telemetry report                          # default log, index, feedback and eval results
ai-rulez telemetry report path/to/usage.jsonl --format json
ai-rulez telemetry report evals                    # rank skills to keep, review, rewrite or prune
```

`telemetry report` lists:

| Section | Meaning |
| --- | --- |
| Used | Indexed skills with at least one logged load: count, last load, feedback counts and the recorded eval pass rate. |
| Never used | Indexed skills with no logged load: retirement candidates. |
| Changed since used | Skills whose logged loads carry a hash other than the current one: the usage evidence describes an older version. |
| Not in the index | Logged ids the index does not know (renamed, removed, or from outside the project). |

When the log holds rule, agent or context events, the report adds the item sections described in
[Item-load telemetry](/ai-rulez/telemetry/#the-item-report) (`--items` always includes them).

| Flag | Default | Meaning |
| --- | --- | --- |
| `[log]` argument | `.ai-rulez/local/usage.jsonl` | The usage log to read. |
| `--index` | `.ai-rulez/skills-index.json` | Skills index to join against. |
| `--feedback` | `feedback.jsonl` beside the log, when present | Feedback log to join; a named file must exist. |
| `--evals` | `.ai-rulez/eval-results.json`, when present | Eval results to join; a named file must exist. |
| `--items` | off | Always include the rule, agent and context sections. |
| `--format` | `text` | `text` or `json`. |

The command always exits 0.

### Joining several logs

`telemetry report evals` (documented on [Evals](/ai-rulez/evals/#reports)) accepts `--usage` more than once to merge the
usage of several repositories or machines. A line that appears in more than one log counts once, by `event_id`. With
`--from-otlp` every `--usage` file is read as OTLP JSON, as written by
[`telemetry export --to file`](/ai-rulez/telemetry/#exporting-to-a-file) or a collector's file exporter.

## Log maintenance

| Task | Command | Details |
| --- | --- | --- |
| Copy the log as an OTLP JSON file (no network) | `ai-rulez telemetry export --to file usage.ndjson` | [Exporting to a file](/ai-rulez/telemetry/#exporting-to-a-file) |
| Push the log to your collector | `ai-rulez telemetry export --to otlp` | [Export and consent](/ai-rulez/telemetry/#the-consent-record) |
| Delete old lines | `ai-rulez telemetry prune --keep-days 90` | [Pruning the usage log](/ai-rulez/telemetry/#pruning-the-usage-log) |

To discard everything, delete `.ai-rulez/local/usage.jsonl`, `usage.salt`, `feedback.jsonl` and `feedback-notes/`.
