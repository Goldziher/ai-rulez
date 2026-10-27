# Judge benchmark

The evaluation of [`ai-rulez judge`](../../docs/judge.md). Everything under `corpus/` is **synthetic**: fake hosts
(`*.example`, `*.example-cdn.io`), fake credentials, no customer data. The malicious samples exist to be flagged and are never
executed. The credentials in the `JR-S14` units (for example `mal-s14-c/.claude/settings.json`) are deliberate
placeholders shaped to trip secret scanners, never real tokens; reports of them as leaks are expected and need no rotation.

| Path | What |
| ---- | ---- |
| `corpus/units/<id>/` | one labeled unit per directory, in Claude Code layout (`.claude/skills/...`, `CLAUDE.md`, `.mcp.json`, ...) |
| `corpus/governance-repo/` | a fictitious monorepo with nested `CLAUDE.md` files, `@imports`, path-scoped rules (brace globs, globs relative to a nested `.claude`) and the units judged against them |
| `rubrics/acme-skill-standards/` | the rubric skill of the `JR-G06` units |
| `manifest/*.yaml` | the labels: group, the findings a correct review reports (`code`, `path`, a `contains` quote, a minimum severity), the categories that must stay clean, and for governance units the exact set of conventions the attachment must produce |

Groups: `malicious` (every `JR-S` code, three styles each: plain, routine-framed, split across files, non-English, hidden
in a comment), `benign` lookalikes (two per code), `trap` and `twin` (each of the 14 known false-positive shapes of the
adjudicator, with a malicious twin that differs by the one property that makes it real), `quality` and `qbenign`,
`governance` and `gbenign`, and `clean` units that a strict reviewer should leave alone.

## Offline

```console
go test ./bench/judge/...        # or: task bench:judge:offline
```

Checks that every label points at text that exists, that the corpus covers every code and trap, that default discovery finds
each unit (nested `.claude/` directories included), that the verifier grounds a quote of every expected finding and rejects an
invented one, that the attachment algorithm produces exactly the labeled rules and instruction files for the governance
repository, that secret units are withheld, and that the scoring code is right.

## Live (spends money)

```console
task bench:judge -- --max-cost 5                              # everything, hard cap 5 USD
task bench:judge -- --only mal-s03-a,ben-s03-a,trap-t01,twin-t01
task bench:judge -- --estimate                                # worst-case cost, sends nothing
task bench:judge -- --score /path/to/merged.json              # score a stored report again
```

The runner copies the corpus to a throwaway git repository (so ancestor conventions stop at the corpus root), runs
`ai-rulez judge --format json --explain-drops` in batches, and prints, from the labels: precision, recall and F1 per code
and category, localization (the right file and quote), the false-positive rate of the trap units and the share of malicious
twins found, findings on clean units, false positives per 100 units, the adjudicator's precision before and after, grounding
(the share of candidates the verifier had to drop), and cost. It uses `AWS_PROFILE=default` and `AWS_REGION=eu-north-1`
(Bedrock) unless told otherwise. Run it when the prompt version changes.

`cmd/judgebench --competitors` runs the offline mode of Cisco skill-scanner over the same corpus and scores
it with the same labels, after mapping its categories onto the `JR-S` codes (`competitors.go`).
