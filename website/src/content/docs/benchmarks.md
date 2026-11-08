---
title: "Scanner benchmark"
description: "How the static checks of ai-rulez compare with other agent-skill scanners on a public dataset, and how to rerun the comparison yourself."
---

`ai-rulez scan` is measured against other open-source skill scanners on a public dataset we did not write. Everything
here is pinned, so anyone can rerun it and get the same numbers.

<!-- skillbench:start -->

1384 skills (839 malicious, 545 benign) from the MaliciousSkillBench source-disjoint `test` partition, static, text-only, no LLM. Scored on each tool's own verdict; intervals are 95% clustered bootstrap.

| Tool | Version | Recall | False-positive rate | Precision | F1 |
| ---- | ------- | ------ | ------------------- | --------- | -- |
| ai-rulez | 5.11.1 | 0.195 (0.139-0.271) | 0.338 (0.297-0.378) | 0.471 | 0.276 (0.210-0.354) |
| skill-scanner | 2.3.2 | 0.077 (0.044-0.125) | 0.040 (0.025-0.058) | 0.747 | 0.140 (0.082-0.216) |
| skillspector | 2.13.4 | 0.113 (0.073-0.167) | 0.194 (0.161-0.228) | 0.473 | 0.183 (0.124-0.254) |

<!-- skillbench:end -->

That table is the last held-out read, of release 5.11.1. It is rewritten only when a release is measured on `test`.

## The unreleased build

Measured on `dev`, the partition rules are tuned on, so these numbers flatter ai-rulez: on held-out sources they have
been lower before. `ai-rulez-threat` is `scan --fail-on threat` and `ai-rulez` is `--fail-on warning` (see
[What is measured](#what-is-measured)). Competitor verdicts come from `bench/skills/cache/`.

<!-- skillbench-dev:start -->

8213 skills (6523 malicious, 1690 benign) from the MaliciousSkillBench source-disjoint `dev` partition, static, text-only, no LLM. Scored on each tool's own verdict; intervals are 95% clustered bootstrap.

| Tool | Version | Recall | False-positive rate | Precision | F1 |
| ---- | ------- | ------ | ------------------- | --------- | -- |
| ai-rulez-threat | unreleased | 0.225 (0.209-0.242) | 0.007 (0.004-0.011) | 0.992 | 0.367 (0.345-0.389) |
| ai-rulez | unreleased | 0.437 (0.420-0.454) | 0.074 (0.062-0.087) | 0.958 | 0.600 (0.584-0.616) |
| skill-scanner | 2.3.2 | 0.312 (0.298-0.326) | 0.008 (0.004-0.013) | 0.993 | 0.475 (0.459-0.491) |
| skillspector | 2.13.4 | 0.323 (0.307-0.340) | 0.155 (0.138-0.173) | 0.889 | 0.474 (0.456-0.493) |

<!-- skillbench-dev:end -->

On real, widely used skills, where every flag is a false alarm:

<!-- skillbench-clean:start -->

563 real skills from the `clean` track, all treated as benign, static, text-only, no LLM. A flag is a false alarm.

| Tool | Version | Skills flagged | Share flagged |
| ---- | ------- | -------------- | ------------- |
| ai-rulez-threat | unreleased | 12 of 563 | 2.1% (1.1-3.4) |
| ai-rulez | unreleased | 53 of 563 | 9.4% (7.1-11.9) |
| skill-scanner | 2.3.2 | 26 of 563 | 4.6% (3.0-6.4) |
| skillspector | 2.13.4 | 167 of 563 | 29.7% (25.9-33.4) |

<!-- skillbench-clean:end -->

Read this as **static, text-only, no LLM**. The dataset holds `SKILL.md` text, so analyzers that read bundled scripts
have nothing to work on, and no tool here calls a model. Static checks miss most attacks written in plain prose: use
this page to compare tools, not to conclude that a clean scan means a safe skill. The model-backed
[`judge`](/ai-rulez/judge/) is not part of this table.

## What is measured

| | |
| --- | --- |
| Dataset | [MaliciousSkillBench](https://huggingface.co/datasets/ProtectSkills/MaliciousSkillBench) ([paper](https://arxiv.org/abs/2608.19901)), pinned to one revision and verified by SHA-256 |
| Population | The source-disjoint `test` partition: 1,384 skills, 839 malicious and 545 benign, from three sources held out of tuning. Cisco's skill-scanner gates its own releases on the same partition |
| Tools | `ai-rulez scan`, Cisco [skill-scanner](https://github.com/cisco-ai-defense/skill-scanner) and NVIDIA [SkillSpector](https://github.com/NVIDIA/SkillSpector), at the versions in the table, in their offline modes |
| Environment | One image pinned by digest, Python tools locked with hashes, no network, one process per sample, an empty environment |

Each tool is scored on **its own verdict**, not on a threshold we picked: `ai-rulez scan` exiting 2, Cisco's `is_safe`,
and SkillSpector's `recommendation`. ai-rulez gets two rows from the same binary and the same samples. `ai-rulez-threat`
runs `--fail-on threat`, which fails on findings that describe an attack, and is the one to compare: Cisco's verdict is
also limited to threats. `ai-rulez` runs `--fail-on warning`, which also fails on capability and hygiene findings such as
`allowed-tools` naming Bash. Releases before the threat gate, 5.11.1 included, have only the second row. A tool that fails on a sample counts as not detecting it, and the
number of samples never changes. Intervals are 95% bootstrap intervals that resample whole structural families, because
near-duplicate skills are not independent evidence. Each run is repeated, and a sample that gets a different answer
between runs is reported as unstable.

## Tuning and the holdout

Numbers go bad when a tool is tuned on its own test. So rules of ai-rulez are changed only against `dev`: the other
partitions, minus every row that shares a structural family or a hash with `test`. The `test` partition is read a fixed
number of times, and each read is logged before it runs in
[`bench/skills/HOLDOUT.md`](https://github.com/Goldziher/ai-rulez/blob/main/bench/skills/HOLDOUT.md). New rules are also
checked against real, widely used skills, because a synthetic benign set flatters a rule.

## What to keep in mind

- All three tools miss most attacks here. On this population ai-rulez has the highest recall and F1 and the highest
  false-positive rate of the three; Cisco skill-scanner flags the least and is the most precise. Which matters more
  depends on whether you are triaging or gating.
- On 563 real skills from Anthropic, Vercel, OpenAI, GitHub and superpowers, pinned by commit and all treated as benign,
  ai-rulez 5.11.1 flags 9.1%, Cisco skill-scanner 4.6% and SkillSpector 29.7%
  ([`bench/skills/clean/`](https://github.com/Goldziher/ai-rulez/tree/main/bench/skills/clean)). Some of those skills do
  run risky commands, so not every flag is wrong, but none is malicious.
- The dataset paper reports lower recall for the two other tools (0.025 for Cisco, 0.000 for SkillSpector). We score
  each tool's verdict, on newer versions; at the highest severity alone our runs give 0.014 and 0.006, so the gap looks
  like a stricter threshold, though we have not confirmed that.

- ai-rulez has read SkillSpector: some of its rules derive from that project (see `THIRD_PARTY_NOTICES.md`). The two
  overlap by construction.
- We chose the protocol and we maintain one of the tools. The protocol is in
  [`tools/skillbench/protocol.toml`](https://github.com/Goldziher/ai-rulez/blob/main/tools/skillbench/protocol.toml);
  disagree with it there.
- 61% of the population is malicious, which is far above what a real marketplace looks like, so precision here is higher
  than precision in practice. Recall and the false-positive rate do not depend on that mix.
- Snyk agent-scan is not included: it calls a hosted service, so it cannot run offline.

## Rerun it

You need Docker and Go.

```bash
git clone https://github.com/Goldziher/ai-rulez && cd ai-rulez
TRACK=test ALLOW_HOLDOUT=1 tools/skillbench/bench.sh
```

The samples include real malware text. They are read as text and never run, are written outside any repository, and
never appear in a result. Do not install them. The published results are in
[`bench/skills/`](https://github.com/Goldziher/ai-rulez/tree/main/bench/skills): `summary.json` holds these tables and the
per-source breakdowns, and `verdicts.jsonl` holds one verdict per tool and sample, so you can derive your own numbers.
Maintainers can run the same thing from the `skillbench` workflow in GitHub Actions.

## Iterate without rescanning

A full run takes hours because Cisco skill-scanner and SkillSpector start once per sample. Their verdicts are kept in
`bench/skills/cache/`, one file per tool and version, holding verdicts only, by sample digest:

```bash
tools/skillbench/iterate.sh                 # dev, a build of this worktree
TRACK=clean tools/skillbench/iterate.sh
```

The dataset and samples stay in `$XDG_CACHE_HOME/ai-rulez/skillbench` (set `WORK` to move them), so a second run
skips the download. They are malicious text: keep that directory out of any repository and any agent skill folder.

The key is the tool, its version, the source of its adapter, the architecture, the network mode and the digest of the
image recipe and lock file, so a change to any of them is a miss. What the cache relies on is that verdicts reproduce,
which they did on every sample of the test and clean tracks across three repetitions. Findings are less strict: Cisco
reported one extra low-severity rule on one of 563 skills in one of three runs, with the same verdict. A full run
therefore rescans `VERIFY` cached samples (25 by default), fails if a verdict moved, and logs if only findings did.
A timeout is kept so the denominators match, and every full run retries it. `iterate.sh` refuses `test` and `full`:
reading the holdout stays a logged full run.

## Attribution

MaliciousSkillBench is published by its authors under CC-BY-4.0 for the dataset records, and its skill text keeps the
terms of each upstream source, so this repository publishes verdicts and never the text.
