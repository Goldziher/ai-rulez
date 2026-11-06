---
title: "Evals"
description: "Evals check that a skill fires when it should, stays quiet when it should not, and does the job when it fires."
---

Evals check that a skill fires when it should, stays quiet when it should not, and does the job when it fires. You
write harness-neutral eval cases next to a skill; `ai-rulez eval run` sends them through a runner (an agent harness or
your own script), scores each skill and records the scores against the skill's content digest, and
[strict validation](/ai-rulez/strict-validation/) can fail a build when a skill is edited without re-running its evals or
scores below a threshold. ai-rulez does not run an agent itself: the runner does. Eval cases are never written into a
harness's skill tree, so they cost no context.

## Example

Two skills with cases:

```yaml
# .ai-rulez/skills/release-notes/evals/notes.eval.yaml
schema_version: 1
cases:
  - id: notes-basic
    prompt: Write the release notes for version 2.1
    expect_trigger: true
```

```yaml
# .ai-rulez/skills/deploy-staging/evals/deploy.eval.yaml
schema_version: 1
cases:
  - id: deploy-basic
    prompt: Deploy the billing service to staging
    expect_trigger: true
    near_miss:
      - Write release notes for the billing service
    assertions:
      - type: contains
        value: staging
  - id: unrelated
    prompt: What is the capital of France?
    expect_trigger: false
```

Check that the right skill would be chosen, offline and free:

```console
$ ai-rulez eval run --mode activation --surface retrieval
# Skill activation results

Surface `retrieval`, scope `domain`, cost $0.00.
...
| Skill | Status | Prompts | Recall | Precision | False activation | recall@1 | recall@3 | MRR |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| deploy-staging | ran, passing | 3 | 100% (21-100%) | 100% (21-100%) | 0% (0-66%) | 100% | 100% | 1.00 |
| release-notes | ran, passing | 1 | 100% (21-100%) | 100% (21-100%) | n/a | 100% | 100% | 1.00 |
```

Estimate a full run, then run the cases through a harness. Here a [command runner](#the-command-runner) drives Codex:

```console
$ ai-rulez eval run --runner-command ./codex-runner.sh --harness codex --dry-run
Skill eval results: runner `command`, harness `codex`, model `default`
Dry run: 4 agent runs, tokens in 6624 / 8279 / 11042, out 1200 / 2400 / 7200, estimated cost $0.06 (range $0.04 to $0.14; a rough estimate).
...
$ ai-rulez eval run --runner-command ./codex-runner.sh --harness codex --date 2026-11-05
Skill eval results: runner `command`, harness `codex`, model `default`, date 2026-11-05

SKILL           STATUS        CASES  PASS  PRECISION  RECALL  ABLATION  TOKENS  COST
deploy-staging  ran, passing  3      100%  100%       100%    n/a       69      $0.09
release-notes   ran, passing  1      100%  100%       100%    n/a       42      $0.03
```

The scores go to `.ai-rulez/eval-results.json`. A second run with nothing changed is served from it (`cached,
passing`) and costs nothing.

## Layout

```text
.ai-rulez/
  skills/
    deploy-staging/
      SKILL.md
      evals/                 # cases for this skill
        deploy.eval.yaml
        prompts/
          long-request.md    # referenced by prompt_file
        fixtures/
          notes.txt          # referenced by files[].source
  evals/                     # optional: cases filed per skill name, away from the skill
    deploy-staging/
      handoff.eval.yaml
```

- `skills/<name>/evals/` holds a skill's cases. It is never copied into a harness's skill tree (`.claude/skills`,
  `.agents/skills`, ...).
- `.ai-rulez/evals/<name>/` is an alternative location; the directory name is the skill. A case file outside such a
  directory belongs to no skill and never runs.
- Only `*.eval.yaml`, `*.eval.yml` and `*.eval.json` files are parsed. Anything else in those directories (another
  tool's files, prompts, fixtures) is yours: it is not parsed, but it is part of the skill's cases digest, so editing a
  fixture re-runs the skill, and it is bundled byte for byte when you ship cases in a [plugin](#bundling-cases-into-a-plugin).

## Case format

A case file holds a `cases` list, or one case at the top level (its `id` then defaults to the file name). The JSON
schema is [`schema/eval-case.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/eval-case.schema.json).

```yaml
schema_version: 1
cases:
  - id: deploy-basic
    description: The plain request fires the skill and names the cluster
    prompt: Deploy the billing service to staging
    expect_trigger: true
    near_miss:                       # look similar, must NOT fire the skill
      - Explain how our staging and production environments differ
    files:                           # created in the working directory first
      - path: services/billing/config.yaml
        content: "env: staging"
      - path: notes.txt
        source: fixtures/notes.txt   # copied from next to this file
    assertions:
      - type: contains
        value: staging-eu
      - type: not_contains
        value: production
      - type: regex
        value: 'deployed \d+ services?'
      - type: file_exists
        path: deploy.log
      - type: command_exit
        command: grep -q ok deploy.log
        exit_code: 0
    rubric: The answer names the staging cluster and the rollout command it ran.
    rubric_min_score: 0.8
    model: haiku
    tags: [smoke, deploy]
```

| Field | Required | Meaning |
| --- | --- | --- |
| `id` | in a `cases` list | Lowercase letters, digits, `.`, `_`, `-`; unique per skill. |
| `prompt` / `prompt_file` | one of the two | The user prompt, inline or in a file next to the case file. |
| `expect_trigger` | yes | Whether the skill should fire for the prompt. |
| `description` | no | What the case checks. |
| `near_miss` | no | Prompts that look similar but must not fire the skill. Each becomes a negative case `<id>.near-miss-<n>` tagged `near-miss`. Only on `expect_trigger: true` cases. |
| `files` | no | Fixtures: a relative `path` with inline `content` or a `source` file. |
| `assertions` | no | Deterministic checks, below. |
| `rubric` | no | Free-text criterion graded by a model: the runner's grader, or the [built-in grader](#the-built-in-grader). |
| `rubric_items` | no | A weighted checklist instead of `rubric` (mutually exclusive): up to 50 entries of `text` and `weight` (above 0, default 1). The score is the share of the total weight satisfied. A runner that only knows `rubric` receives the checklist rendered as text. |
| `rubric_min_score` | no | Score (0 to 1) a rubric needs. Default 0.7. |
| `model` | no | Model override for this case. |
| `tags` | no | Free tags. `holdout` marks cases for [`improve`](/ai-rulez/improve/). |

| Assertion `type` | Fields | Holds when |
| --- | --- | --- |
| `contains`, `not_contains` | `value`, optional `path` | The final answer (or the file at `path`) does or does not contain the text. |
| `regex` | `value` (RE2), optional `path` | The answer (or file) matches. |
| `file_exists` | `path`, `exists` (default `true`) | The file is or is not in the working directory. |
| `command_exit` | `command`, `exit_code` (default 0) | The command, run through the shell in the working directory, exits with that status. Needs `--allow-exec`. It gets a scrubbed environment (`PATH`, `HOME`, temp directories, locale, `CI`), never your credentials. |

A case with no assertions and no rubric is trigger-only: it measures recall (positive) or precision (negative).

Paths must be relative and stay inside their directory (no leading `/` or `~`, no `..`), also after symlinks are
resolved. Unknown fields are errors, so a typo cannot silently disable an assertion. `ai-rulez validate` reports
problems as `AR996 eval-case-invalid` with the file and line.

## Running evals

```bash
ai-rulez eval run                                   # every skill with cases
ai-rulez eval run deploy-staging --ablation         # one skill; also run without it and report the delta
ai-rulez eval run --dry-run                         # what would run and the cost range
ai-rulez eval run --mode activation --surface retrieval   # offline: does the right skill rank first?
ai-rulez eval run --runner-command ./my-runner.sh --harness codex
ai-rulez eval run --format junit --output-dir eval-report  # eval-report/eval-report.xml
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--harness` | `claude` | Harness the cases run against; recorded in the results. |
| `--runner` | see below | `claude-plugin-eval`, `command`, `claude-native` or `codex-native`. |
| `--runner-command CMD` | | Shell command for the `command` runner; selects it. |
| `--model` | | Model for the cases (a case's `model` wins). |
| `--runs N` | 3 (cases), 5 (activation) | Runs per case for `claude-plugin-eval`; repetitions per prompt for `--surface native`. |
| `--ablation` | off | Also run every case without the skill and report the delta. |
| `-n`, `--dry-run`, `--estimate` | off | List what would run with a cost range. Calls no runner and writes nothing. |
| `--mode` | `cases` | `cases` or `activation` (see [Activation mode](#activation-mode)). |
| `--surface`, `--scope`, `--description-from` | | Activation options. |
| `--grader`, `--allow-llm`, `--grader-max-cost` | `runner`, off, `0.25` | Who grades rubrics; see [The built-in grader](#the-built-in-grader). |
| `--threshold R` | `[lint.evals] min_pass_rate`, else 1 | Pass rate (0 to 1) a skill needs. `0` records scores without gating. |
| `--max-cost USD` | 0 (none) | Advisory run-wide budget; see [Cost controls](#cost-controls). |
| `--max-cost-mode` | `expected` (cases), `high` (activation) | Estimate figure that must fit under `--max-cost`. |
| `--price-in`, `--price-out` | by model tier (built-in price table) | USD per million tokens for the estimate. |
| `--changed-only`, `--base REF` | off, `HEAD` | Only skills with files changed against `REF` (committed, uncommitted and untracked). |
| `--force` | off | Ignore the [result cache](#caching). |
| `--allow-exec` | off | Run `command_exit` assertions. |
| `--timeout` | `30m` | Time limit for one skill; the runner's whole process tree is killed when it ends. |
| `--date` | `$AI_RULEZ_EVAL_DATE` | Date recorded in the results. The clock is never read, so equal inputs give an equal file. |
| `--format` | `text` | `text`, `json`, `markdown` or `junit`. |
| `--output-dir DIR` | | Write the report to `DIR/eval-report.<ext>` instead of standard output. |
| `--results FILE`, `--no-write` | `.ai-rulez/eval-results.json` | Use another results file, or do not update it. |
| `--claude-bin`, `--codex-bin` | `claude`, `codex` | Executables of the Claude and Codex runners. |
| `--runner-arg`, `--judge-model` | | `claude-plugin-eval` only: extra arguments (repeatable) and its grader model. |

Every flag is validated before a runner starts. Each skill's result is written to the results file as soon as it
finishes, so an interrupted run keeps the skills that already ran.

**Exit status**: `0` when every selected skill passes; `2` when a skill is below its threshold, errored, was skipped
over budget, or has invalid cases; `1` when the run could not start.

### Runners

| Runner | Harness | Selected by | What it does |
| --- | --- | --- | --- |
| `claude-plugin-eval` | Claude Code | default for `--harness claude` | Wraps the skill in a throwaway plugin and runs `claude plugin eval` on it. |
| `command` | any | `--runner-command CMD` | Runs your script with a JSON request on standard input. |
| `claude-native` | Claude Code | default for `--surface native` | Activation only; see [The native surface](#the-native-surface). |
| `codex-native` | Codex | `--surface native --harness codex` | Activation only; experimental. |

### The claude-plugin-eval runner

The runner builds a plugin containing the skill (without its `evals/`), translates each case into the format
`claude plugin eval` reads, and runs it with `--no-publish`, so no report is uploaded. Whether the skill fired is
observed from the Skill tool call; `contains`, `not_contains`, `regex` and `file_exists` become graders; a `rubric`
becomes the tool's model grader. A case's runs are decided by majority vote.

- Cases the tool cannot express (`command_exit` assertions, `files` fixtures) are reported as **skipped**, not failed.
- The runner never trusts the plugin directory for you. In a terminal, `claude` asks; in CI pass
  `--runner-arg --trust-plugin` once you trust the skill and its cases.
- `claude` gets a scrubbed environment: `PATH`, `HOME`, locale, `CLAUDE_CONFIG_DIR` and the `ANTHROPIC_*` and
  `CLAUDE_CODE_*` authentication and provider variables. Cloud and forge credentials are not passed.
- A run that times out or is interrupted is an error; a partial result is never scored.

### The command runner

`--runner-command CMD` runs `CMD` through the shell once per skill. It receives the request on standard input, prints
the response on standard output, and has `AI_RULEZ_EVAL_PROTOCOL=1` and `AI_RULEZ_EVAL_SKILL=<id>` in its
environment. Use it for any harness ai-rulez has no runner for.

The request has every case self-contained (near misses expanded, `prompt_file` and fixture sources inlined):

```json
{
  "version": 1,
  "skill": { "id": "deploy-staging", "dir": "/abs/.ai-rulez/skills/deploy-staging", "digest": "sha256:..." },
  "harness": "codex", "model": "haiku", "ablation": true, "max_cost_usd": 2.5,
  "cases": [
    { "id": "deploy-basic", "prompt": "Deploy the billing service to staging", "expect_trigger": true,
      "assertions": [{ "type": "contains", "value": "staging-eu" }], "rubric": "...", "tags": ["smoke"] },
    { "id": "deploy-basic.near-miss-1", "prompt": "Explain how ...", "expect_trigger": false, "near_miss_of": "deploy-basic" }
  ]
}
```

The response:

```json
{
  "version": 1,
  "results": [
    { "case": "deploy-basic", "arm": "with", "triggered": true, "output": "deployed 3 services to staging-eu",
      "work_dir": "/tmp/run-1", "rubric_score": 0.9, "input_tokens": 5200, "output_tokens": 410, "cost_usd": 0.031 },
    { "case": "deploy-basic", "arm": "without", "output": "I cannot deploy from here", "cost_usd": 0.012 }
  ],
  "cost_usd": 0.043
}
```

| Result field | Meaning |
| --- | --- |
| `case`, `arm` | The case id and `with` or `without` (the `without` arm only with `--ablation`). |
| `triggered` | Whether the skill fired. Required for the `with` arm. |
| `output`, `work_dir` | The final answer and the working directory. ai-rulez grades the assertions from them. |
| `passed` | Your own verdict on the outcome checks; it wins over local grading. |
| `rubric_score` | 0 to 1; required for a case with a rubric unless `passed` is set or `--grader builtin` grades it. |
| `skipped`, `reason` | Leave the case out of the score. |
| `error` | The case counts as a failure. |
| `input_tokens`, `output_tokens`, `cost_usd` | Optional; used for the budget and the estimate history. Negative or non-finite values are rejected. |

A case with `rubric_items` is also sent with `rubric` set to the rendered checklist, so a runner that only knows
`rubric` grades it correctly. The protocol version must be `1`, and an unknown case or arm is an error. A minimal runner
in Python:

```python
import json, sys

req = json.load(sys.stdin)
results = []
for case in req["cases"]:
    answer, fired, usage = run_agent(case["prompt"], skill_dir=req["skill"]["dir"])   # your harness call
    results.append({"case": case["id"], "arm": "with", "triggered": fired, "output": answer, **usage})
print(json.dumps({"version": 1, "results": results}))
```

### The built-in grader

By default a rubric is graded by the runner. `--grader builtin` grades it with the model configured in
[`[llm]`](/ai-rulez/llm/) instead, from the `output` the runner returned, so the grade does not depend on each runner's own
grader:

```bash
ai-rulez eval run --runner-command ./my-runner.sh --grader builtin --allow-llm --grader-max-cost 0.25
```

- **Consent.** Sending a transcript to a model needs `--allow-llm`, plus `allow_network = true` and an `[llm]` model in
  the user config or environment (never the repository). Without all of these the run is refused before any case
  starts. `--dry-run` sends nothing.
- **Budget.** `--grader-max-cost` (default $0.25) caps the grader's spend; the spend also counts towards `--max-cost`
  and is reported as `grader_cost_usd`.
- **Grading.** Every case with a `rubric` or `rubric_items`, in both arms, is graded at temperature 0 with a score in
  [0, 1] and a one-line rationale (`rubric_note` in the report). The score replaces the runner's `rubric_score`. When the
  runner also returned `passed`, both must pass. A result with no `output` fails the rubric; a failed grader call leaves
  the case ungraded, which scores as a failure.
- **Safety.** The transcript is sent as fenced data the grader is told not to obey, and text in it that addresses the
  grader or asks for a score scores 0. A rubric or transcript holding secret-looking text is not sent (the case is
  ungraded). Transcripts over 64 KiB are cut to their head and tail.
- **Choose the model deliberately.** The grade is only as good as the grader. Compare a cheap grader with a few
  transcripts you labeled yourself before trusting its pass rate; a small model tends to be lenient.
- With `claude-plugin-eval`, the tool still runs its own grader, and the built-in grade decides.

The grader's model and prompt version are part of the cache key, so changing the grader re-runs the skill.

### Caching

A skill is not re-run when `eval-results.json` already holds a run with the same cache key: the skill's digest, the
digest of its eval material (cases, fixtures, rubrics), the runner and its settings (including the content of a
`--runner-command` script), harness, model, `--ablation`, `--allow-exec` and the ai-rulez version. Editing a case or
the skill, or changing any of those, re-runs it. `--force` ignores the cache.

Only real results are cached: a run in which a case errored (rate limit, missing credentials, no result) is retried
next time, so an outage never becomes a cached failure. The pass/fail verdict is recomputed from the stored score, so
changing `--threshold` needs no re-run.

**Records are signed per user.** Each record carries a `mac` made with a per-user key, `eval-results.key` in the user
config directory (`$XDG_CONFIG_HOME/ai-rulez/`, else `~/.config/ai-rulez/`; never in the repository). A record without
a valid `mac` for your key (committed from another machine, or edited) is **unverified**: it is shown and linted, but it
never satisfies the cache, so the skill re-runs once on each machine and the new record is signed. Anyone who can edit
the file can also delete the `mac`, so a CI gate should run `eval run --force` or check where `eval-results.json` came
from.

### Cost controls

- **Estimate.** `--dry-run` prints the agent runs, tokens and USD as a range (low / expected / high). It is offline
  and deterministic: a harness overhead per run (2,000 tokens), the prompt, fixtures and `SKILL.md` (counted with the
  `cl100k_base` tokenizer), 600 output tokens per run, a grader call per rubric, times the runs per case, times two
  with `--ablation`. Low assumes 0.8x input and 0.5x output; high assumes a tool loop that reads everything again and
  3x output. Treat it as an order of magnitude.
- **Prices** come from the built-in price table (`haiku`, `sonnet` and `opus` are listed by short name). Without
  `--model` the estimate is priced as sonnet and says so. A model the table does not know is priced as sonnet, and with
  `--max-cost` it is refused unless `--price-in` and `--price-out` are given.
- **Tuning.** `[lint.evals.estimate]` overrides the assumptions, and
  [`eval calibrate-estimate`](#calibrating-the-estimate) proposes measured values. The harness overhead matters most:
  a real harness's system prompt and tool listing can be ten times the default.
- **Budget.** `--max-cost USD` covers the whole run. It refuses to start when the estimate exceeds it, hands each runner
  the remaining budget (`max_cost_usd`; `claude-plugin-eval` passes `--max-cost-usd`), and skips the remaining skills
  once the reported spend reaches it (status `skipped-over-budget`, exit 2). It is checked between skills; within a
  skill only the runner can enforce it. A runner that reports no cost or tokens, or that fails, is assumed to have
  spent the whole remaining budget, so the run stops instead of continuing on an unknown spend.
- **Fewer runs.** `--changed-only`, the cache, a low `--runs`, a cheap `--model` and naming skills keep runs down.

Every run records the estimate next to what the runner reported (`estimate_vs_actual` in the report, `estimate` in the
skill's record), so you can see how far off the estimate tends to be.

| `[lint.evals.estimate]` key | Default | Meaning |
| --- | --- | --- |
| `overhead_tokens` | 2000 | Harness input tokens per agent run. |
| `assumed_output_tokens` | 600 | Output tokens per case run. |
| `activation_output_tokens` | 150 | Output tokens per activation run. |
| `tool_loop_factor` | 1.0 | How many extra passes over the prompt, fixtures and skill the high estimate assumes. |
| `price_in_per_mtok`, `price_out_per_mtok` | the price table | What the runs are really billed at. `--price-in`/`--price-out` win. |

A zero value keeps the built-in default.

### Calibrating the estimate

```console
$ ai-rulez eval calibrate-estimate
cases, harness codex, model default: 2 run(s) (low confidence: too few runs to trust)
  overhead_tokens            2000 -> 5136
  assumed_output_tokens       600 -> 410
  token error (median)      +111% -> +0%; recorded cost error median 97%, p90 97%
  input price per MTok     $3 list -> $4.5865 billed (prompt caching)

Proposed for cases, harness codex, model default (add to .ai-rulez/config.toml or your user config):

[lint.evals.estimate]
overhead_tokens = 5136
assumed_output_tokens = 410
price_in_per_mtok = 4.5865  # list price $3: the harness bills cached input at a discount
```

`eval calibrate-estimate` fits the estimate's assumptions to what the recorded runs reported, per harness, model and
kind of run (case runs and activation runs separately). It moves the harness overhead and the output assumption by the
median gap between estimated and reported tokens, and proposes `price_in_per_mtok` when the billed cost implies an
input price more than 10% off the list price (harnesses that cache their prompt bill most input at a discount). It
writes nothing and makes no call.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--harness`, `--model` | all | Only runs of this harness or model. |
| `--min-samples N` | 3 | Runs a group needs before its proposal is not marked low-confidence. |
| `--results FILE` | `.ai-rulez/eval-results.json` | Results to read. |
| `--format` | `text` | `text` or `json`. |

Only records signed with your key count, and runs without a token split are left out.

## Activation mode

A full case run answers "does the skill do the job". Activation mode answers a cheaper question first: for these
prompts, is the right skill the one that gets chosen, and no other?

```bash
ai-rulez eval run --mode activation --surface retrieval
ai-rulez eval run deploy-staging --mode activation --surface retrieval --scope all --format json
ai-rulez eval run --mode activation --surface native --model haiku --runs 5
```

`--surface` has no default: choose `retrieval` (offline, free) or `native` (a harness's model, costs money).

Per skill, the report gives:

- **Recall**, **precision** and the **false activation** rate, each with a 95% Wilson interval (small samples are
  normal, and a bare percentage over four prompts says little).
- **recall@1**, **recall@3** and **MRR** over the positive prompts (retrieval), and the rank or rate per prompt.
- **Stolen by**: the siblings that won its positive prompts, with counts and shares, plus a top-level **confusion
  matrix** (`confusion[expected][won]`, `none` when nothing fired).

Only the prompt and `expect_trigger` are used; fixtures, assertions and rubrics are ignored. `--scope domain`
(default) makes a skill compete with its domain's skills plus the root skills; `--scope all` with every skill. A skill
alone in its scope gets a warning, since a stolen trigger cannot be measured. A skill passes when its share of passing
prompts reaches `--threshold` (default 1). Exit status is as for a case run.

The JSON report follows [`schema/eval-activation.v1.schema.json`](/ai-rulez/schema/); `text` and `markdown` are also
available, `junit` is not:

```json
{
  "schema_version": 1, "mode": "activation", "surface": "retrieval", "scope": "domain",
  "skills": [{
    "id": "deploy-staging", "status": "ran", "passing": false,
    "competing": ["deploy-staging", "release-notes"],
    "recall": { "value": 0.5, "n": 2, "interval": { "low": 0.09, "high": 0.91 } },
    "stolen_by": [{ "skill": "release-notes", "prompts": 1, "share": 0.5 }]
  }],
  "confusion": { "deploy-staging": { "deploy-staging": 1, "release-notes": 1 } }
}
```

Each measured skill's rates (never prompts) are recorded in `eval-results.json` under `activation`, signed like the
rest of the record, and judged by `AR9A1` and `AR9A2` once you set a threshold. An activation run leaves the case-run
result alone. `--dry-run` ranks and prints but records nothing.

### The retrieval surface

`retrieval` ranks every prompt (including near misses) against the competing skills with the offline
[skill search ranker](/ai-rulez/search/#the-ranker) the MCP `find_skill` tool uses. It calls no model, costs nothing and is deterministic. A skill fires for a
prompt when it ranks first.

It measures the ranker, not a model's choice, and the two can disagree: a model may load a generic sibling where the
ranker puts the right skill first. Use retrieval as a quick pre-check and `native` to confirm.

### Comparing descriptions

```bash
ai-rulez eval run deploy-staging --mode activation --surface retrieval --description-from candidate.txt
```

`--description-from FILE` measures a candidate description of one skill on the same prompts: the file's text (at most
16 KiB, UTF-8, no hidden characters) replaces the `description` in a scratch copy, on either surface. The source is not
edited and nothing is recorded, so you can repeat the run with other files and compare the reports.

### The native surface

```bash
ai-rulez eval run --mode activation --surface native --model haiku --runs 5
ai-rulez eval run deploy-staging --mode activation --surface native --dry-run   # the estimate, no model call
```

`native` asks a harness's model. For each skill it installs every competing skill at once, repeats each prompt
`--runs` times (default 5), stops each run after the first turn, and records which skills loaded. A prompt's
activation rate is the share of runs in which the skill under test loaded. A positive prompt passes at a rate of at
least 0.8, a negative one at most 0.2; a passing prompt that one more run going the other way would fail is marked
**borderline**. Repetition matters: the same prompt can fire in 5 of 5 runs once and 3 of 5 the next.

Results are stored and replayed (`cached`) while the skill, the competing set, the cases, the runner, the model, the
run count and the ai-rulez version are unchanged; `--force` repeats the measurement.

| Runner | Harness | How it works |
| --- | --- | --- |
| `claude-native` | `claude` (default) | Writes the competing skills into one throwaway plugin, reduced to `name` and `description`, and runs `claude -p` for one turn in an empty directory with only the Skill tool and no user, project or local settings. A Skill tool call is a load. Four runs are in flight at a time, and it enforces `--max-cost` itself. |
| `codex-native` | `codex` | Experimental. Writes the set to `.agents/skills/` in an empty directory and runs `codex exec` read-only. A read of a `SKILL.md` is a load; the run is stopped at the first read or after three commands. Codex reports no usage, so a capped (`--max-cost`) run is refused: run `--dry-run` for the estimate, then run uncapped. |
| `command` | any | Your script, after a capability handshake (below). |

Built-in harness skills and enabled plugins still compete with yours in `claude-native`, and a harness's own system
prompt makes each run cost far more input than the default estimate assumes. Run with a small model (`--model haiku`)
and [calibrate the estimate](#calibrating-the-estimate). A skill with an error-level security finding is refused before
any run.

`--max-cost` is checked against the **high** estimate before a native run starts (`--max-cost-mode expected` uses the
expected figure).

#### Native activation with a command runner

ai-rulez first probes the command with `{"version":1,"mode":"capabilities"}` (`AI_RULEZ_EVAL_MODE=capabilities` in
the environment). A runner that supports native activation answers:

```json
{ "version": 1, "capabilities": ["activation"], "surfaces": ["native"] }
```

Any other answer refuses the run (exit 1). The activation request then carries the installed set and the prompts, and
no fixtures, assertions or rubrics:

```json
{
  "version": 1, "mode": "activation", "surface": "native", "harness": "claude", "model": "haiku",
  "runs": 5, "max_turns": 1, "max_cost_usd": 0.5,
  "skill": { "id": "deploy-staging", "dir": "...", "digest": "sha256:...", "description": "..." },
  "skills": [
    { "id": "deploy-staging", "dir": "...", "digest": "sha256:...", "description": "Deploy a service to staging" },
    { "id": "release-notes", "dir": "...", "digest": "sha256:...", "description": "Write release notes" }
  ],
  "cases": [
    { "id": "deploy-basic", "prompt": "Deploy the billing service to staging", "expect_trigger": true, "target": "deploy-staging", "runs": 5 }
  ]
}
```

The response has one `with` result per case:

```json
{ "version": 1, "results": [
  { "case": "deploy-basic", "arm": "with", "runs": 5, "fired_counts": { "deploy-staging": 4, "release-notes": 1 },
    "input_tokens": 20500, "output_tokens": 600, "cost_usd": 0.0071 } ] }
```

`runs` (at least 1) and `fired_counts` (ids from `skills`, plus `none`; each at most `runs`) are required unless the
result has `error` or `skipped`. `errored_runs` counts repetitions that failed; when more failed than completed, the
prompt has no result and the skill is not scored.

## Importing scenarios

```bash
ai-rulez eval import --from tessl ./scenarios/add-health-endpoint --skill http-service
ai-rulez eval import --from tessl ./scenarios --output-dir ./tmp-cases --dry-run --report import.json
```

`eval import` turns scenarios written for another tool into case files. It reads local files only, contacts no
service and runs nothing it reads. Only `--from tessl` is supported.

A Tessl scenario is a task (`task.md`) plus a weighted checklist (`criteria.json`). The importer reads several common
spellings of each field and reports anything it does not map instead of guessing:

```json
{
  "scenario": "add-health-endpoint",
  "criteria": [
    { "name": "adds route", "description": "Registers GET /health on the router", "weight": 3 },
    { "name": "returns 200", "description": "The output contains \"200\"", "weight": 2 }
  ],
  "pass_threshold": 70
}
```

```console
$ ai-rulez eval import --from tessl ./scenarios/add-health-endpoint --skill http-service --lift-assertions --dry-run
would write add-health-endpoint.eval.yaml, add-health-endpoint.task.md (1 case)
mapped:   task -> prompt_file
mapped:   2 criteria -> rubric (weights kept as text)
mapped:   pass_threshold -> rubric_min_score 0.7 (read as a percent)
assumed:  expect_trigger: true (the scenario targets a skill)
assumed:  lifted contains and not_contains assertions match case-sensitively; the criteria they came from do not say so
lifted:   criterion "returns 200" -> contains "200"
```

| Input | Becomes |
| --- | --- |
| `task.md` | `prompt_file: <id>.task.md`, written beside the case |
| scenario name (`scenario`, `name`, `id`, `title`, else the directory) | case `id`, slugged; a repeated name gets `-2`, `-3` |
| (the scenario targets a skill) | `expect_trigger: true` and the tag `imported:tessl` (reported as an assumption) |
| `criteria` | a `rubric` with a numbered, weighted checklist (`--rubric-mode single`, default), or `rubric_items` with the weights kept (`--rubric-mode items`). A criterion of weight 0 is dropped. |
| `pass_threshold` (or `passing_score`, `pass_score`, `passing_threshold`, `threshold`) | `rubric_min_score`; a fraction (0 to 1) or a percent (above 1 up to 100, or `"70%"`); default 0.7 |
| `files`, `fixtures`, `starting_files`, `setup_files` | `files`, with sources copied to `fixtures/<id>/` |
| an `activation` block with `should_not_trigger` prompts | `near_miss` |
| anything else (`baseline`, `repeats`, `agent`, `model`, ...) | not imported: listed as unmapped (`AR9A5`) with the flag it corresponds to (`--ablation`, `--runs`, `--model`) |

| Flag | Meaning |
| --- | --- |
| `--from` | Input format: `tessl` (required). |
| `--skill` | Write into the skill's `evals/` directory. |
| `--output-dir` | Write here instead. |
| `--rubric-mode` | `single` (default) or `items`. |
| `--lift-assertions` | Also turn criteria phrased as a quoted mechanical check (`The file "x" exists`, `The output contains "y"`, and their negations) into assertions. The criterion stays in the rubric. Off by default, because a wrong lift changes what is graded. |
| `-n`, `--dry-run` | Map and report; write nothing. |
| `--force` | Overwrite existing files. |
| `--report FILE`, `--format json` | The machine-readable report. |

Nothing is written unless every scenario maps. Written files carry a provenance header (importer version and a sha256
of the inputs) and are checked by the case parser first. Inputs are capped at 2 MiB, must be UTF-8 regular files, and
are scanned: a credential (`AR001`) or hidden character (`AR002`) refuses the scenario, and an instruction-override
phrase (`AR004`) is flagged, since a criterion ends up in a rubric a model reads.

## Scores and the results file

Per skill, over the run's cases (near misses included, skipped cases excluded):

| Score | Definition |
| --- | --- |
| `pass_rate` | Passed cases / scored cases. A case passes when the skill fired exactly as `expect_trigger` says and its assertions and rubric hold. Runner errors count as failures. |
| `trigger_precision` | TP / (TP + FP) over the `with` runs: TP is a positive case that fired, FP a negative case (near misses included) that fired. `null` without a denominator. |
| `trigger_recall` | TP / (TP + FN). `null` without a denominator. |
| `near_miss_false_positives` | Near-miss cases where the skill fired. |
| `ablation_delta` | Outcome pass rate with the skill minus without it, over the positive cases with assertions or a rubric that ran in both arms. `null` without `--ablation`. |
| `skill_tokens`, `run_tokens`, `cost_usd` | Approximate tokens of `SKILL.md`; tokens and USD the runner reported. |

`eval run` records each skill in `.ai-rulez/eval-results.json`. Commit it: it is deterministic, sorted by skill id, and
holds no prompts or outputs.

```json
{
  "schema_version": 1,
  "skills": [
    {
      "id": "deploy-staging",
      "digest": "sha256:ba212c77889e...",
      "cases_digest": "sha256:baa7c21987fd...",
      "lock_digest": "sha256:11b96b692d42...",
      "cache_key": "sha256:a0e7a94844cc...",
      "runner": "command", "harness": "codex", "model": "default", "ablation": false,
      "date": "2026-11-05",
      "passing": true,
      "score": { "cases": 3, "scored": 3, "passed": 3, "pass_rate": 1, "trigger_precision": 1, "trigger_recall": 1, "...": "..." },
      "estimate": { "low_usd": 0.0284, "expected_usd": 0.0457, "high_usd": 0.106, "...": "..." },
      "last_pass": { "digest": "sha256:ba212c77889e...", "date": "2026-11-05" }
    }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `digest` | sha256 of every file under the skill directory except `evals/`: the cache and freshness key. Editing a case does not change it. |
| `cases_digest` | sha256 of the eval material. |
| `lock_digest` | The skill's [lock](/ai-rulez/lockfile/) digest (`ai-rulez/skill/v1`), which [usage logs](/ai-rulez/usage-telemetry/) carry; it joins usage with scores. |
| `date` | From `--date` or `$AI_RULEZ_EVAL_DATE`; omitted when neither is set. |
| `last_pass` | The digest and date of the last passing run. It survives a later failing run, and `AR997` compares against it. |
| `activation` | The latest activation measurement, if any. |
| `mac` | The per-user signature; see [Caching](#caching). |

## Linting cases and results

These rules are part of [strict validation](/ai-rulez/strict-validation/):

| Code | Name | Default | Reports |
| --- | --- | --- | --- |
| `AR962` | `evals-missing` | off | A skill has no eval cases. |
| `AR996` | `eval-case-invalid` | error | A case file is malformed: unknown field, missing `expect_trigger` or prompt, bad assertion, invalid regex, unsafe path, duplicate id. |
| `AR997` | `eval-stale` | off | The skill changed after its last recorded passing run. |
| `AR998` | `eval-score-low` | off | The recorded pass rate is below `min_pass_rate`. |
| `AR9A0` | `eval-results-invalid` | error | `eval-results.json` cannot be parsed or has an unsupported `schema_version`. |
| `AR9A1` | `activation-low` | off | The recorded activation recall or precision is below `min_activation_recall` or `min_activation_precision`. |
| `AR9A2` | `skill-confusable` | off | A sibling won at least `confusion_threshold` of the skill's positive activation prompts. |
| `AR9A3` | `activation-policy-conflict` | warning | A case expects a trigger for a skill whose frontmatter sets `disable-model-invocation: true` or `allow_implicit_invocation: false`, so it can never pass (or a negative case for such a skill, which can never fail). |
| `AR9A4` | `activation-prompt-names-skill` | off | A positive prompt contains the skill's name as a word: it tests explicit invocation, not whether the model chooses the skill. Enable with `[lint.severity] AR9A4 = "warning"`. |
| `AR9A5` | `eval-import-unmapped` | info | Reported by [`eval import`](#importing-scenarios) only: input fields with no counterpart. |

```toml
[lint.evals]
require = true                     # AR962 at warning
allow = ["internal-*", "scratch"]  # skills exempt from AR962
require_fresh = "error"            # AR997: "warn" or "error"
min_pass_rate = 0.8                # AR998 at error, and the default threshold of eval run
min_activation_recall = 0.8        # AR9A1 at error
min_activation_precision = 0.9     # AR9A1 at error
confusion_threshold = 0.25         # AR9A2 at warning

[lint.severity]
evals-missing = "error"            # a severity set directly wins over require
```

| `[lint.evals]` key | Turns on | Meaning |
| --- | --- | --- |
| `require` | `AR962` (warning) | Every skill needs cases: a non-hidden file in `skills/<name>/evals/` or `.ai-rulez/evals/<name>/` (`.gitkeep` does not count). |
| `allow` | | Skill names or globs exempt from `AR962`. |
| `require_fresh` | `AR997` | `"warn"` or `"error"`. A skill with no passing run is not reported stale. |
| `min_pass_rate` | `AR998` (error) | 0 to 1. |
| `min_activation_recall`, `min_activation_precision` | `AR9A1` (error) | 0 to 1. |
| `confusion_threshold` | `AR9A2` (warning) | 0 to 1. |

```console
$ ai-rulez validate
.ai-rulez/skills/deploy-staging/SKILL.md:2  warning  AR962 evals-missing  skill "deploy-staging" has no eval cases ...
```

Only records signed with your own key count as evidence. When `AR997`, `AR998`, `AR9A1` or `AR9A2` is on, an unsigned
record (committed from another machine, edited, or on a machine that never ran `eval run`) is reported as
"unverified" instead of passing. A record measured on an older skill digest is not judged by `AR9A1` and `AR9A2`.

**In CI** there is no `eval-results.key`, so every committed record is unverified and, with `require_fresh` or
`min_pass_rate` set, `validate` fails on every skill with a record. Either run `eval run` in the CI job (it creates a
key and records fresh results), or leave `AR997` and `AR998` off in CI (`[lint.severity] AR997 = "off"` in the CI
overlay, or a lint profile) and enforce evals in the job that runs them.

## Reports

`ai-rulez telemetry report evals` joins the results with the [usage log](/ai-rulez/usage-telemetry/) and feedback, and
recommends an action per skill:

```console
$ ai-rulez telemetry report evals
Skill evals joined with usage: 1 rewrite, 1 prune, 0 review, 0 keep

Rewrite
  deploy-staging                       pass 100%  prec 100%  recall 100%  delta n/a       used 3 join exact
      - feedback: 1 misled/wrong/stale against 0 great

Prune
  release-notes                        pass 100%  prec 100%  recall 100%  delta n/a       used 0 join none
      - never used in the usage log
      - no ablation evidence that it helps
      - costs about 42 tokens when loaded
```

| Action | When |
| --- | --- |
| `rewrite` | Pass rate below `--min-pass-rate` (0.8), trigger precision or recall below `--min-trigger` (0.8), a negative ablation delta, edited since the last passing eval, or `misled` + `wrong` + `stale` feedback outweighing `great`. |
| `prune` | Not a rewrite; a usage log was given and the skill was never used in it; and evals do not show it helping (no record, or an ablation delta of 5 points or less). |
| `review` | Neither, but unused while evals show value, or no eval results yet. |
| `keep` | Everything else. |

Rows are ordered by action, number of reasons, pass rate (worst first) and `SKILL.md` size. Records not signed with
your key are marked `unverified` and count as no eval results. Without a usage log nothing is concluded about use.

With a usage log, each row has a `join` class saying how closely the usage evidence matches the evaluated version:

| Class | Meaning |
| --- | --- |
| `exact` | A use was logged at the digest the eval ran on. |
| `stale` | The uses carry a digest, but not the evaluated one: the score describes another version. |
| `legacy` | The record or the uses have no canonical digest; matched by skill id only. |
| `none` | No logged use, or no eval record. |

| Flag | Default | Meaning |
| --- | --- | --- |
| `--usage FILE` (repeatable), `--usage-log` | `.ai-rulez/local/usage.jsonl`, when present | Usage logs; an event in several logs counts once. |
| `--from-otlp` | off | Read every `--usage` file as OTLP JSON ([`telemetry export --to file`](/ai-rulez/telemetry/#exporting-to-a-file)). |
| `--feedback FILE` | `feedback.jsonl` beside the first log | Feedback log. |
| `--results FILE` | `.ai-rulez/eval-results.json` | Eval results. |
| `--min-pass-rate`, `--min-trigger` | `0.8` | Rewrite thresholds. |
| `--format` | `text` | `text` or `json`. |

## Bundling cases into a plugin

A plugin bundle leaves eval cases out by default. Opt in with `include_evals`:

```toml
[plugin]
name = "billing"
version = "1.0.0"
include_evals = true
```

Every bundle that carries skills then contains `skills/<name>/evals/...` for each bundled skill, and `evals/...`, a copy
of `.ai-rulez/evals/` (or `<content_root>/evals/`), at the bundle root. A per-domain plugin
(`[marketplace.from_domains]`) carries only the cases of its own skills. The provenance sidecar records a hash of every
bundled file, so `ai-rulez verify --plugin` fails when a case is edited or removed in the bundle.

## Running evals in CI

```bash
set -euo pipefail

ai-rulez validate            # AR962/AR996/AR997/AR998 when enabled
ai-rulez generate --plugin   # the bundle, cases included with include_evals = true
ai-rulez verify --plugin

# Only the skills this change touched, with a JUnit report.
ai-rulez eval run --changed-only --base origin/main \
    --ablation --max-cost 5 --date "$(date -u +%F)" \
    --format junit --output-dir eval-report --runner-arg --trust-plugin
```

- **Against the diff.** `--changed-only --base origin/main` runs the skills with a changed or untracked file in their
  directory or in `.ai-rulez/evals/<skill>/`. Fetch the base ref first (`git fetch origin main`, or a full-depth
  checkout).
- **JUnit.** `--format junit --output-dir eval-report` writes `eval-report/eval-report.xml`: one suite per skill, one
  test case per eval case, failures for failed cases, errors for runner errors and invalid case files, skipped for
  skipped and dry-run entries, and the scores as suite properties. It has no timestamps, so it is byte-stable.
- **Caching.** Restore `.ai-rulez/eval-results.json` and the runner's `eval-results.key` from your CI cache to replay
  unchanged skills; a record signed on another machine re-runs. Commit or cache the updated file after the run.
- **Cost.** Run `--dry-run` first, set `--max-cost`, keep `--runs` low, and use `--ablation` only where you track the
  delta. Pass `--date` from CI to date the results.
- **Exit codes.** `eval run`: `0` clean, `2` a failing, errored, over-budget or invalid skill, `1` could not run.
  `validate`: `0` clean, `1` invalid configuration, `2` findings at or above `fail_on`. `verify --plugin` is non-zero on
  any mismatch.
- **Trust.** The runner runs the harness as the CI user with that job's credentials. Evaluate only skills and cases you
  trust; `command_exit` assertions (`--allow-exec`) and `--runner-arg --trust-plugin` are opt-ins for that reason.
  ai-rulez itself makes no network call during `eval run`, except the [built-in grader](#the-built-in-grader) when you
  enable it.
