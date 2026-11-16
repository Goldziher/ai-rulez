# Claude Code MCP discovery follow-up

The first [pilot](2026-11-16.md) compared native skills, bare MCP, and MCP with the generated
`dynamic-skills` skill. Its stub arm omitted the generated root instructions. AI-Rulez v5.13.3
already generated an `Unlisted skills` section that tells the assistant about the MCP catalog.
This follow-up separately measures the stub, the released root instructions, and updated root guidance.
The [sanitized dataset](claude-discovery-ablation.jsonl) contains 192 turns.

## What we measured

Claude Code 2.1.325 ran Sonnet 4.6 at low effort on the eight-skill nested fixture. Every pair had
a native arm and an MCP arm, with arm order alternating by pair. The MCP server advertised only
`find_skill` and `load_skill`. In the positive cases, the second worksheet question resumed the
same session and moved from `services/amber` to its sibling `services/cobalt`.

Two other prompts checked behavior when no skill should be applied. The wrong-scope prompt asks for an outbound
dispatch code at an inbound returns worksheet. The catalog contains a similar return workflow in that scope.
A correct response finds no applicable guidance and supplies no workflow code. The unrelated prompt asks
an arithmetic question and should trigger no repository lookup.

A scoped search counts a `find_skill` call with the requested worksheet path. A correct load requires the body
marker from the expected skill in tool output. Answer success means the expected code appears in the answer,
including answers with surrounding prose. These counts separate discovery, applying a skill, and answering the
user. The [per-turn data](claude-discovery-ablation.jsonl) also includes tool calls, tool errors, latency, and tokens.

## Released behavior and isolated controls

| MCP arm | Scoped positive searches | Correct positive loads | Positive code in answer | Wrong-scope correct without load | Unrelated lookups |
| --- | ---: | ---: | ---: | ---: | ---: |
| v5.13.3 stub only, default tool search | 1/4 | 1/4 | 1/4 | 0/2 | 0/2 |
| v5.13.3 stub only, tool search explicitly enabled | 4/4 | 4/4 | 4/4 | 0/2 | 0/2 |
| v5.13.3 stub only, upfront tools | 2/4 | 2/4 | 2/4 | 0/2 | 0/2 |
| v5.13.3 generated standalone `CLAUDE.md` | 4/4 | 4/4 | 4/4 | 0/2 | 0/2 |
| v5.13.3 generated default `CLAUDE.md` and `AGENTS.md` | 4/4 | 4/4 | 4/4 | 0/2 | 0/2 |
| v5.13.3 with prototype search-first instructions | 4/4 | 4/4 | 4/4 | 0/2 | 0/2 |

All released-root and prototype wrong-scope turns searched the correct path, but then applied the return workflow
to the dispatch question. Correct scope filtering alone did not establish workflow applicability.

The stub-only samples varied even with tool search available. We recorded no stub invocations in these runs.
Loading MCP tools upfront removed tool-search calls but still missed positive lookups. The v5.13.3 shared-root
run shows that discovery can already work with the complete generated instructions. The first pilot's stub-only
numbers represent a reduced installation.

## Revised instructions

| MCP arm | Scoped positive searches | Correct positive loads | Positive code in answer | Wrong-scope correct without load | Unrelated lookups |
| --- | ---: | ---: | ---: | ---: | ---: |
| Initial revision, standalone root | 6/6 | 6/6 | 6/6 | 0/3 | 0/3 |
| Initial revision, default shared root | 6/6 | 5/6 | 5/6 | 2/3 | 0/3 |
| Final revision, standalone root | 6/6 | 6/6 | 6/6 | 0/3 | 0/3 |
| Final revision, default shared root | 6/6 | 6/6 | 6/6 | 1/3 | 0/3 |

The initial revision emphasized checking that the match covers the requested operation. In one resumed turn it
found the correct skill and then rejected it. Claude inferred the workflow from the name `dispatch.txt`, which in
the second scope contains an inbound returns worksheet. It had neither read the worksheet nor loaded the skill.
The final revision tells Claude to use the requested operation and read the referenced file to resolve ambiguity.
It found, loaded, and used the expected skill on all 12 positive turns across both layouts. All 48 native positive
turns across the unchanged native arms also loaded the expected body and returned its code.

In the final wrong-scope turns, Claude asked for the dispatch code at the correct path. The server filtered out
the outbound dispatch skill. Five of six turns still loaded the adjacent return workflow from that scope.
One correctly answered that no dispatch guidance applied without loading the return workflow. This is a workflow
selection error after successful discovery. Output format is measured separately in the data. For example,
a response may include the right code with extra prose even when it was asked for an exact answer.

## Reproduce

All runs used the same signed-in Claude account, fixture version 1 and tool permissions. Provider caches
were left at their defaults. Separate variants overlapped on one macOS arm64 host. The small sample describes these sessions.
It provides no guarantee for other prompts, models, or catalogs.

The released binary was AI-Rulez v5.13.3. Both revision binaries were built from base commit
`0bcd71cc7f42db57f07793c79aad9ddc3982faec` with discovery instruction changes. The final
discovery instructions ship in v5.13.4. Use that release binary to reproduce the final guidance. The initial
revision was an intermediate build used to diagnose the resumed-turn failure.

| Binary | SHA-256 |
| --- | --- |
| Released | `b411218f0aabbe3a19475d6dbbe96c417279b97dce83f91903b61b26470d19a6` |
| Initial revision | `4bbcd3a7e9ec35bbdcc6588fe9e50c68749aef4840da71e0fac995e3a974a5f3` |
| Final revision | `f95370751bfda76ec17ee89d7e0105f57a0761027d3cc84a1de45acbf2c1de1c` |

Use the test binary's absolute path. The CLI must be authenticated, and the runner logs should stay private.
For each variant below, run the same commands from the repository root. `released`, `candidate`, and `precise` should point to locally verified binaries. The sanitized data records the binary SHA-256 for each turn and omits paths outside the synthetic fixture,
prompt responses, session identifiers, and private logs.

```sh
umask 077
work=$(mktemp -d)
measure() {
  binary=$1
  label=$2
  variant=$3
  pairs=$4
  shift 4
  python3 bench/skill-discovery/run.py claude "$binary" "$work/$label" \
    --claude-discovery "$variant" --size small --layout nested --pairs "$pairs" \
    --task sibling-walk --task wrong-scope --task unrelated "$@" > "$work/$label.jsonl"
}
measure "$released" default-stub default-stub 2
measure "$released" stub stub 2
measure "$released" upfront upfront 2
measure "$released" root root 2
measure "$released" shared-root shared-root 2
measure "$released" prototype rule 2 --claude-rule bench/skill-discovery/claude-search-first.md
measure "$candidate" candidate-root root 3
measure "$candidate" candidate-shared-root shared-root 3
measure "$precise" precise-root root 3
measure "$precise" precise-shared-root shared-root 3
python3 bench/skill-discovery/publish_results.py "$work/"*.jsonl > "$work/published.jsonl"
```
