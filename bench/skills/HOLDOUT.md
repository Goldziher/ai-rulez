# Holdout reads

The test partition measures; it does not tune. Every read is logged here before it runs, and
`tools/skillbench/protocol.toml` fixes the budget at 3. `dev` excludes every row sharing a structural
family or hash with test, so rule changes cannot learn from it.

| # | Date | ai-rulez | Why | Result |
| - | ---- | -------- | --- | ------ |
| 1 | 2026-11-06 | 5.11.1 | Baseline, frozen before any rule change | `bench/skills/baseline-5.11.1/` |
| 2 | 2026-11-06 | 5.11.1 plus `AR059`-`AR061` and the severity changes (unreleased) | Measure the rules tuned on dev | `bench/skills/candidate/` |

## Results so far

- Read 1, released 5.11.1: recall 0.195, false-positive rate 0.338. All three repetitions byte-identical for every tool.
- Read 2, unreleased rules tuned on dev: recall 0.204, false-positive rate 0.363. On dev the same change moved recall
  from 0.338 to 0.499 and the rate from 0.062 to 0.079, so the gain did not carry over to held-out sources. A native
  build of 5.11.1 reproduced the released binary's verdicts byte for byte, so the difference is the rules.

One read is left. Per-rule results of test are not to be used for tuning.
