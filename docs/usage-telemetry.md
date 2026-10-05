# Usage telemetry

Which generated skills are ever used, which fire when they should not, and which can be retired? A runtime event
only answers that if it can be tied back to a source skill, its owner and its version. ai-rulez supports that in
four opt-in pieces. None of them makes a network call, and none is enabled by default.

1. a **skills index** written by `generate`,
2. a **hook template** that records skill invocations as identifier-only log lines,
3. **feedback records** (`usage feedback`) for "this skill misled me",
4. a **report** that joins a log with the index, the feedback and the [eval scores](evals.md).

## The skills index

```toml
[usage]
skills_index = true
```

`ai-rulez generate` then writes `.ai-rulez/skills-index.json` (next to the config; it is recorded in the generated
manifest, so turning the option off removes it). It is derived from the source tree and the rendered outputs, not
from files on disk, and is byte-stable across runs, so it is safe to commit.

```json
{
  "schema_version": 1,
  "skills": [
    {
      "id": "deploy-staging",
      "kind": "skill",
      "domain": "ops",
      "source": ".ai-rulez/domains/ops/skills/deploy-staging/SKILL.md",
      "hash": "blake3:0699fc6b...",
      "owner": "team-a",
      "version": "1.2.0",
      "outputs": {
        "claude": [".claude/skills/deploy-staging/SKILL.md"],
        "codex": [".agents/skills/deploy-staging/SKILL.md"]
      }
    }
  ]
}
```

- `id` is the skill's directory name, which is the name every harness invokes it by. It is the stable identity:
  generated skills already carry it as `name`.
- `kind` is `skill`, or `command` for a command that a harness runs as a skill (the `claude` preset writes commands
  into `.claude/skills`); such a command is listed and invoked like a skill, so its usage is logged the same way.
- `hash` is a blake3 digest of the authored skill: `SKILL.md` as written, then each bundled resource in path order.
  It changes when, and only when, the authored skill changes.
- `owner` and `version` are read from the `owner` and `version` frontmatter keys when set. Add
  `require_metadata` to [`[lint]`](strict-validation.md) to make them mandatory.
- `outputs` lists, per preset, the `SKILL.md` files that preset writes. A path shared by several presets
  (`.agents/skills`) is listed under each.
- Records are sorted by `id`, then `source`. The index covers the active profile at the repository root; scoped
  (monorepo) outputs and machine-local skills are not listed.

## Recording invocations

```bash
ai-rulez usage hook                 # print the hooks block
ai-rulez usage hook -o hooks.json   # or write it to a file
```

The block is a template for the `hooks` key of `.claude/settings.json` (or a plugin's hooks file). Merge it in by
hand; nothing installs it. To have `generate` keep it in sync, declare the same commands as top-level
[`[[hooks]]`](settings.md) (the payload fields above are read by `usage record` itself, so the command is all a
harness needs); ai-rulez then writes them into each supported harness's own hooks file next to your other hooks,
without touching hooks you wrote by hand. The template registers `ai-rulez usage record` for the two ways a skill is used:

| Claude Code event | Fires when | Field read |
| --- | --- | --- |
| `PreToolUse`, matcher `Skill` | the model calls the Skill tool | `tool_input.skill` |
| `UserPromptExpansion` | the user types a skill's slash command | `command_name` |

Both events were checked against Claude Code 2.1.289. The recorder ignores events it does not recognize.

### Other harnesses

```bash
ai-rulez usage hook --harness codex    # merge into .codex/hooks.json
ai-rulez usage hook --harness cursor   # merge into .cursor/hooks.json
```

Codex and Cursor have no Skill tool: they load a skill by reading its `SKILL.md`. Their templates register
`ai-rulez usage record --harness <name>` on the `PreToolUse` (Codex, matcher `Bash`) and `preToolUse` (Cursor, matcher
`Shell`) events, whose names and matchers come from ai-rulez's own [hook support](settings.md). The recorder then logs a
skill load when `tool_input.command`, `file_path` or `path` contains `skills/<id>/SKILL.md`, and only the `<id>`
is kept; the rest of the command is matched and discarded. The payload field names for those two harnesses are
inferred, not verified: check the log after wiring the hook. Any other harness (`--harness gemini`, `copilot`, ...)
prints a warning to standard error and no template, because its skill-load payload is not documented here.

Each invocation appends one line to the log (default `.ai-rulez/local/usage.jsonl`, which is machine-local, so a log
is not committed by accident):

```json
{"v":2,"ts":"2026-10-04T17:20:08Z","event":"skill_invoked","skill":"deploy-staging","id":"deploy-staging","hash":"blake3:0699fc6b...","session":"5b1c0e9a7d3f2a64","invocation":"tool","harness":"claude","outcome":"loaded"}
```

| Field | Meaning |
| --- | --- |
| `v` | Log format version, `2`. Lines without it are version 1. |
| `ts`, `event`, `skill`, `id`, `hash`, `invocation`, `harness` | As before: when, the name the harness reported, the index id (a `plugin:` prefix is dropped), the index hash at that moment, `tool`, `slash` or `read`, and the harness. |
| `session` | A **salted hash** of the harness session id: 16 hex digits of `sha256(salt, id)`. The salt is 16 random bytes in `usage.salt` beside the log (mode 0600, machine-local) or `$AI_RULEZ_USAGE_SALT`. The raw id is never written; when no salt can be obtained the field is omitted. |
| `outcome` | `loaded` (the hook saw the skill load), `used` or `abandoned`. The recorder itself only knows `loaded`; a hook you wire to a later event can pass `--outcome used`. Omitted for an unknown value. |
| `served` | `true` for loads that came through the MCP server rather than from disk (set by the MCP skill server; `--served` on the command). Omitted when false. |
| `role` | The active role, from `--role`. Omitted when unset. |

What is recorded: the skill name as the harness reported it, the id it resolves to in the index, the hash from the
index at that moment, the salted session hash, how the skill was invoked, the harness, and the fields above.
What is **not**: prompts, slash-command arguments, tool inputs other than the skill name (or, for Codex and Cursor,
the `SKILL.md` path the id was read from), transcripts, raw session ids or file contents. The recorder decodes only
those fields, so nothing else can reach the log.

Compatibility: new fields are additive. `report usage` reads version 1 lines (which may hold a raw session id written
before salting) and lines from later versions (unknown fields are ignored). Older `ai-rulez` releases read version 2
lines and ignore the new fields.

`--log FILE` chooses another file. `--sink-command CMD` runs `CMD` through the shell with the line on its standard
input, for teams that ship lines to their own collector; that command, not ai-rulez, decides where a line goes.
`--index FILE` points at a non-default index. `ai-rulez usage record` never fails a session: problems go to standard
error and the exit status stays 0.

## Feedback

```bash
ai-rulez usage feedback deploy-staging --kind stale --note-file ./why.txt
```

Records that a skill `misled` you, is `stale`, is `wrong`, or was `great`. One line goes to
`.ai-rulez/local/feedback.jsonl` (machine-local; `--log` to change it): version, timestamp, the skill and its index id,
the index hash, the kind, and `--harness` and `--role` when given. With `--note-file` the note's text is copied to
`feedback-notes/<timestamp>-<id>-<kind>.txt` beside the log (mode 0600, at most 64 KB) and only that file name is
recorded. The note is never part of a log line, the skills index, `eval-results.json` or any hash, so it cannot leave
the machine unless you copy it. Feedback does not change a skill by itself; `report usage` shows the counts and
[`report evals`](evals.md#reports) treats more `misled`/`wrong`/`stale` than `great` as a reason to rewrite.

## The report

```console
$ ai-rulez report usage .ai-rulez/local/usage.jsonl
Skill usage: 42 events

Used (3)
  deploy-staging                                    30  last 2026-10-04T17:20:08Z
  ...

Never used (2)
  release-notes
  legacy-helper                                     owner team-b

Changed since used (1)
  deploy-staging           logged blake3:0699fc6b..., now blake3:a1b2c3d4...

Not in the index (0)
```

- **Never used**: indexed skills with no logged invocation, the retirement candidates.
- **Changed since used**: skills with log lines recorded at a hash other than the current one. The evidence may
  describe an older version, so review before acting on it.
- **Not in the index**: logged ids the index does not know (renamed, removed, or from outside the project).

Each row also shows the feedback counts for that skill (`feedback: misled 2, great 1`) and its recorded eval pass rate
(`eval: 75% (failing)`) when `feedback.jsonl` sits beside the log and `.ai-rulez/eval-results.json` exists; `--feedback`
and `--evals` point at other files (a named file must exist).

`--json` prints the same data as JSON (rows gain `feedback` and `eval` members, and the report `feedback_events`).
The command reports and exits 0.
