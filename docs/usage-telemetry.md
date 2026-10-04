# Usage telemetry

Which generated skills are ever used, which fire when they should not, and which can be retired? A runtime event
only answers that if it can be tied back to a source skill, its owner and its version. ai-rulez supports that in
three opt-in pieces. None of them makes a network call, and none is enabled by default.

1. a **skills index** written by `generate`,
2. a **hook template** that records skill invocations as identifier-only log lines,
3. a **report** that joins a log with the index.

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
hand; nothing installs it. It registers `ai-rulez usage record` for the two ways a skill is used:

| Claude Code event | Fires when | Field read |
| --- | --- | --- |
| `PreToolUse`, matcher `Skill` | the model calls the Skill tool | `tool_input.skill` |
| `UserPromptExpansion` | the user types a skill's slash command | `command_name` |

Both events were checked against Claude Code 2.1.289. Other harnesses are not covered because their skill events
have not been verified; the recorder ignores events it does not recognize.

Each invocation appends one line to the log (default `.ai-rulez/local/usage.jsonl`, which is machine-local, so a log
is not committed by accident):

```json
{"ts":"2026-10-04T17:20:08Z","event":"skill_invoked","skill":"deploy-staging","id":"deploy-staging","hash":"blake3:0699fc6b...","session":"923460f0-...","invocation":"tool","harness":"claude"}
```

What is recorded: the skill name as the harness reported it, the id it resolves to in the index (a `plugin:` prefix
is dropped), the hash from the index at that moment, the session id, and whether the model or the user invoked it.
What is **not**: prompts, slash-command arguments, tool inputs other than the skill name, transcripts or file
contents. The recorder decodes only those fields, so nothing else can reach the log.

`--log FILE` chooses another file. `--sink-command CMD` runs `CMD` through the shell with the line on its standard
input, for teams that ship lines to their own collector; that command, not ai-rulez, decides where a line goes.
`--index FILE` points at a non-default index. `ai-rulez usage record` never fails a session: problems go to standard
error and the exit status stays 0.

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

`--json` prints the same data as JSON. The command reports and exits 0.
