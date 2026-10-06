# Evals

ai-rulez gives eval cases a supported place to live, keeps them out of the context your agents load, ships them in
plugin bundles when you ask, and can report skills that have none. It also defines a harness-neutral case format,
runs the cases through a pluggable runner (`ai-rulez eval run`), scores each skill (pass rate, trigger precision
and recall, ablation delta, token cost), records the scores next to the skill's content digest, and gates on them in
[strict validation](strict-validation.md). It does not implement an agent or a grader of its own: a runner does the
running, and a model grader is whatever the runner provides.

- [Case format](#case-format)
- [Running evals](#running-evals)
- [Scores and the results file](#scores-and-the-results-file)
- [Linting cases and results](#linting-cases-and-results)
- [Reports](#reports)
- [Running evals in CI](#running-evals-in-ci)

## Layout

```text
.ai-rulez/
  skills/
    deploy-staging/
      SKILL.md
      evals/                 # cases for this skill
        trigger-basic.eval.yaml
        prompts/
          long-request.md    # referenced by prompt_file
        fixtures/
          notes.txt          # referenced by files[].source
  evals/                     # optional: cases filed per skill name, away from the skill directory
    deploy-staging/
      handoff.eval.yaml
    README.md
```

- `skills/<name>/evals/` is a recognized directory. `generate` does not warn about it, and it is **never** written
  into any per-tool skill tree (`.claude/skills`, `.agents/skills`, ...): eval cases must not cost context.
- `.ai-rulez/evals/` is an optional project-level tree. Use it when you prefer to keep cases away from the skill
  directory. Cases are filed per skill: `.ai-rulez/evals/<skill-name>/` counts as that skill's cases, and a case
  file outside such a directory belongs to no skill and is never run.
- ai-rulez parses only files named `*.eval.yaml`, `*.eval.yml` or `*.eval.json` (the [case format](#case-format)
  below). Everything else in those directories (another tool's `case.yaml`, `prompt.md`, graders, fixtures) is yours:
  ai-rulez never parses it, but it is hashed into the skill's cases digest (so editing a fixture re-runs the skill)
  and copied byte for byte into plugin bundles. `.DS_Store`-style junk, `results/` and `node_modules/` are left out
  of the hash.

## Case format

A case file is YAML or JSON. It holds a `cases` list, or one case written at the top level (its `id` then defaults to
the file name). The JSON schema is [`schema/eval-case.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/eval-case.schema.json).

```yaml
# .ai-rulez/skills/deploy-staging/evals/deploy.eval.yaml
schema_version: 1
cases:
  - id: deploy-basic
    description: The plain request fires the skill and names the cluster
    prompt: Deploy the billing service to staging
    expect_trigger: true
    near_miss:                       # look similar, must NOT fire the skill
      - Explain how our staging and production environments differ
      - Roll back the last production deploy
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
    rubric_min_score: 0.8            # default 0.7
    model: haiku                     # overrides --model for this case
    tags: [smoke, deploy]

  - id: unrelated-question
    prompt: What is the capital of France?
    expect_trigger: false
```

| Field | Meaning |
| --- | --- |
| `id` | Lowercase letters, digits, `.`, `_`, `-`. Unique per skill. Required inside a `cases` list. |
| `prompt` / `prompt_file` | The user prompt, inline or a file next to the case file. Exactly one. |
| `expect_trigger` | **Required.** Whether the skill should fire for the prompt. |
| `near_miss` | Prompts that look like they should fire the skill but must not. Each becomes a derived negative case `<id>.near-miss-<n>` tagged `near-miss`. Only on `expect_trigger: true` cases. |
| `files` | Fixtures: `path` (relative to the working directory) with inline `content` or a `source` file. |
| `assertions` | Deterministic checks, below. |
| `rubric`, `rubric_min_score` | Judged by a model grader that the runner supplies. |
| `model`, `tags` | Per-case model override, free tags. |

Assertions:

| `type` | Fields | Holds when |
| --- | --- | --- |
| `contains` / `not_contains` | `value`, optional `path` | the final answer (or the file at `path`) does / does not contain the text |
| `regex` | `value` (RE2), optional `path` | the answer (or file) matches |
| `file_exists` | `path`, `exists` (default true) | the file is / is not in the working directory |
| `command_exit` | `command`, `exit_code` (default 0) | the command, run through the shell in the working directory, exits with that status; **needs `--allow-exec`** because it executes text from a case file |

Every path must be relative and stay inside its directory (no leading `/` or `~`, no `..`). `prompt_file` and
`files[].source` are also checked after symlinks are resolved: a symlink (to a file or to a directory) whose target
lies outside the eval directory is refused, and so is an eval directory that itself resolves outside the config
directory. `files[].path` names a file for the runner to create in the working directory; ai-rulez validates it is
relative and inside, and the runner must keep it there. A case with no
assertions and no rubric is a trigger-only case: it measures recall (positive) or precision (negative) and nothing else.
Unknown fields are errors, so a typo cannot silently disable an assertion. Problems are reported by
`ai-rulez validate --strict` as `AR996 eval-case-invalid` with the file and line.

## Running evals

```bash
ai-rulez eval run                                   # every skill with cases, claude-plugin-eval runner
ai-rulez eval run deploy-staging --ablation         # one skill, also run it without the skill
ai-rulez eval run --dry-run                         # what would run and roughly what it costs
ai-rulez eval run --runner command --runner-command ./my-runner.sh --harness codex
ai-rulez eval run --format junit --out eval-report  # eval-report/eval-report.xml
```

`eval run [skill...]` flags:

| Flag | Meaning |
| --- | --- |
| `--harness` | Harness the cases run against (default `claude`); recorded in the results. |
| `--runner`, `--runner-command` | `claude-plugin-eval` or `command`. The default is `claude-plugin-eval` for the claude harness and `command` when `--runner-command` is set. |
| `--claude-bin`, `--runner-arg`, `--runs`, `--judge-model` | `claude-plugin-eval` only: the executable, extra arguments (repeatable, for example `--runner-arg --trust-plugin`), runs per case, grader model. |
| `--timeout` | Time limit for one skill with either runner (default 30m). When it ends the runner's whole process tree is killed (a process group on Unix); the output the runner may produce is capped (8 MiB for `claude`, 64 MiB for a `command` runner's answer). |
| `--model` | Model for the cases. |
| `--ablation` | Also run every case without the skill and report the delta. |
| `--dry-run` | List what would run with an estimated cost. Calls no runner, writes nothing. |
| `--format`, `--out dir` | `json`, `markdown` (default) or `junit`; with `--out` the report goes to `<dir>/eval-report.<md\|json\|xml>`. |
| `--max-cost USD` | Cost control, see [below](#cost-controls). |
| `--date`, `$AI_RULEZ_EVAL_DATE` | The date recorded in the results. The clock is never read, so equal inputs give an equal file. |
| `--changed-only`, `--base REF` | Only skills with files changed against `REF` (default `HEAD`; committed, uncommitted and untracked). |
| `--force` | Ignore the [result cache](#caching). |
| `--threshold R` | Pass rate (0 to 1) a skill needs (default `[lint.evals] min_pass_rate`, else 1). `0` records scores without gating on them; a value outside 0-1 is rejected. |
| `--allow-exec` | Run `command_exit` assertions. |
| `--no-write`, `--results FILE` | Do not update, or use another, results file. |
| `--price-in`, `--price-out` | USD per million tokens for the estimate (default by model tier). |

Every flag is validated before any runner is started: an unknown `--format`, a `--max-cost`, `--price-in` or
`--price-out` that is NaN, infinite or negative, or a `--threshold` outside 0-1 is an error that costs nothing. Each
skill's result is written to the results file (atomically) as soon as it finishes, so a crash or Ctrl-C keeps the
skills that already ran; the rest are reported as not run and the command exits non-zero.

Exit status: `0` when every selected skill passes, `2` when a skill is below its threshold, errored, was skipped over
budget, or has invalid cases, `1` for a failure to run at all.

### The claude-plugin-eval runner

Builds a throwaway plugin containing the skill (without its `evals/`) and translates each case into the directory
format `claude plugin eval` reads: `evals/<case>/prompt.md` plus `graders/*.md`. A `tool_used: Skill` grader observes
whether the skill fired (inverted for `expect_trigger: false`); `contains`, `not_contains`, `regex` and `file_exists`
become `regex` and `file_exists` graders; `rubric` becomes an `llm` grader. It then runs

```text
claude plugin eval <plugin dir> --json <file> --no-publish --threshold 0 --ablation with-without|none [--runs N] [--model M] [--judge-model M] [--max-cost-usd X]
```

and reads the per-run JSON back, taking a majority vote over a case's runs. Any result file, results directory or case
directory left by an earlier run is deleted first, and a run that times out (`--timeout`) or is interrupted is an
error: a partial result is never scored. A non-zero exit that still wrote a complete result is scored, and the cases it
missed count as errors. On Windows the whole process tree is killed through a Job Object. Cases the tool cannot express
(`command_exit` assertions, `files` fixtures) are reported as **skipped**, not failed, and left out of the score.
The adapter never adds `--trust-plugin` itself: pass `--runner-arg --trust-plugin` once you trust the plugin
directory. It was written against the help text and interview prompt of Claude Code 2.1.289; the JSON shape it reads
(`cases[].arms.with|without[]` with `graders[]`, `costUsd`, `error`) was not checked against a live run, and an
unrecognized document is an error rather than a silent pass. `claude plugin eval` publishes its report to claude.ai by
default; the adapter always passes `--no-publish`.

### The command runner

`--runner-command CMD` runs `CMD` through the shell once per skill. It receives the request on standard input and
prints the response on standard output; standard error passes through. `AI_RULEZ_EVAL_PROTOCOL=1` and
`AI_RULEZ_EVAL_SKILL=<id>` are in its environment.

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

Cases are self-contained: near misses are expanded and `prompt_file` and fixture `source` are inlined. The response:

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

- `triggered` is required for the `with` arm. The `without` arm (only with `--ablation`) needs no `triggered`.
- Give `output` (and `work_dir`, for file assertions) and ai-rulez grades the assertions itself; or give `passed` to
  report your own verdict on the outcome checks, which wins over local grading. `rubric_score` (0-1) is required for
  cases with a `rubric` unless `passed` is set.
- `skipped: true` with a `reason` leaves a case out of the score; `error` counts the case as a failure.
- Costs and tokens are optional. The protocol version must be `1`; an unknown case or arm is an error.

### Caching

A skill is not re-run when the results file already holds a run with the same cache key: the skill's sha256 digest,
the digest of its eval material (cases, fixtures, rubrics and graders), the runner and its own settings (the
`--runner-command` text and the content hash of its first word when that is a file, so editing the script re-runs; or `--claude-bin`, `--runs`, `--judge-model` and `--runner-arg` for `claude-plugin-eval`),
harness, model, the ablation setting, `--allow-exec` (it changes how `command_exit` assertions grade) and the
ai-rulez version. Editing a case or the skill, or changing any of those, re-runs it. `--force` ignores the cache.
Only real grading results are cached: a run in which any case errored (rate limit, missing credentials, no result
from the runner) or nothing was scored is not stored and is retried on the next run, so an outage never turns into a
cached failure. A failed grade (a case that ran and did not pass) is a result and is cached. The pass/fail verdict is recomputed from the stored
score against the current threshold, so lowering `--threshold` needs no re-run.

**Records are signed per user.** `eval-results.json` is a committed file, and the cache key is an unkeyed hash anyone
can compute, so a pull request could add a record that claims a pass. Each record therefore carries a `mac`
(HMAC-SHA256 of the record) made with a per-user key, `eval-results.key` in the user config directory (`$XDG_CONFIG_HOME/ai-rulez/`, else
`~/.config/ai-rulez/`; 32 random bytes, mode 0600, never in the repository). A record without a valid `mac` for your
key (committed from another machine, hand-edited, or when no key can be stored) is **unverified**: it is still shown
and linted, but it never satisfies a cache hit. The skill re-runs, the run reports a warning (`the stored result is
unverified ... re-running`), and the fresh record is signed. A record is also required to match the skill and cases
digests on disk. Consequence: results committed by CI or a teammate re-run once on each machine; a CI job that
restores `eval-results.json` from its own cache replays only if it keeps the same key file. Treat the key like
`llm-cache.key`: `XDG_CONFIG_HOME` set from an untrusted `.envrc` relocates it. A CI gate should still use
`eval run --force`, or verify the provenance of `eval-results.json`, since anyone who can edit the file can also
delete the `mac`.

### Cost controls

- `--dry-run` prints the number of agent runs, estimated tokens and USD. The estimate is deterministic and offline:
  the harness's own overhead (2,000 tokens), the prompt, fixtures, the skill's `SKILL.md` (counted with the embedded
  `cl100k_base` tokenizer, an approximation), 600 output tokens per run, an extra grader call per rubric, times the
  runs per case (3 for `claude-plugin-eval` unless `--runs`; the same number is passed to claude with `--runs`, so the estimate and the run agree), times two arms with `--ablation`. Prices come from a
  model tier (haiku, sonnet, opus; sonnet for anything else) and go stale: override with `--price-in` and `--price-out`.
  Treat it as an order of magnitude.
- `--max-cost USD` is an advisory budget for the whole run (all skills together), not a per-skill cap and not a hard limit. It refuses to start when the estimate exceeds it, hands each runner the remaining budget
  (`max_cost_usd`; `claude-plugin-eval` passes it as `--max-cost-usd`), and skips the remaining skills once the
  reported spend reaches it (status `skipped-over-budget`, exit 2). It is checked between skills; inside one skill only the runner can enforce it, and `--timeout` bounds the time. When a runner reports more than the budget it was given, the report carries a warning. A runner that reports no cost and no tokens at all is assumed to have spent the whole remaining budget (with a warning), so the run stops instead of continuing on an unknown spend. Spend is counted conservatively: the larger of
  the sum of per-case costs and the runner's own total, tokens priced with `--price-in`/`--price-out` when no cost is
  reported, and a runner call that fails is assumed to have spent the whole remaining budget, so the run stops. A
  `command` runner should enforce `max_cost_usd` itself: it is the only one that knows what it spends. Negative or
  non-finite costs and token counts in a response are rejected.
- `--changed-only`, the cache and `--runs` keep the number of runs down; `tags` and skill names narrow it by hand.

## Scores and the results file

Per skill, over the run's cases (near misses included, skipped cases excluded):

| Score | Definition |
| --- | --- |
| `pass_rate` | passed cases / scored cases. A case passes when the skill fired exactly as `expect_trigger` says **and** its assertions and rubric hold. Runner errors count as failures. |
| `trigger_precision` | TP / (TP + FP) over the `with` runs, where TP is a positive case that fired and FP is a negative case (near misses included) that fired. `null` with no denominator. |
| `trigger_recall` | TP / (TP + FN). `null` with no denominator. |
| `near_miss_false_positives` | near-miss cases where the skill fired. |
| `ablation_delta` | outcome pass rate with the skill minus without it, over the positive cases that have assertions or a rubric and ran in both arms. `null` without `--ablation` data. |
| `skill_tokens`, `run_tokens`, `cost_usd` | approximate tokens of `SKILL.md`; tokens and USD the runner reported. |

Rates are rounded to four decimals.

`eval run` records each skill in **`.ai-rulez/eval-results.json`** (commit it; it is deterministic, sorted by skill id,
and holds no prompts or outputs):

```json
{
  "schema_version": 1,
  "skills": [
    {
      "id": "deploy-staging",
      "digest": "sha256:...",            // the skill's authored content when the run happened
      "cases_digest": "sha256:...",
      "lock_digest": "sha256:...",       // the lock's digest of the skill; what usage logs join on
      "cache_key": "sha256:...",
      "runner": "claude-plugin-eval", "harness": "claude", "model": "haiku", "ablation": true,
      "date": "2026-10-05",             // from --date / $AI_RULEZ_EVAL_DATE, omitted when neither is set
      "passing": true,
      "score": { "cases": 3, "pass_rate": 1, "trigger_precision": 1, "trigger_recall": 1, "ablation_delta": 0.5, "...": "..." },
      "last_pass": { "digest": "sha256:...", "date": "2026-10-05" }
    }
  ]
}
```

The skill digest is the sha256 of every regular file under the skill directory except its top-level `evals/`, in path
order (`path NUL length NUL bytes`); editing a case does not make the skill look edited. It is the cache and
freshness key. `lock_digest` is the same skill under the [lock's](lockfile.md) scheme (`ai-rulez/skill/v1`: `SKILL.md`
plus `references/`, `scripts/` and `assets/`, again without `evals/`), the identity the [usage log](usage-telemetry.md)
carries as `digest`. A record without it (written by an earlier release) joins with usage by skill id only until the
skill is evaluated again; a cached run backfills it. `last_pass` survives a later
failing run, which is what freshness compares against.

## Linting cases and results

Four more rules join `AR962` in [strict validation](strict-validation.md):

| Code | Name | Default | Reports |
| --- | --- | --- | --- |
| `AR996` | `eval-case-invalid` | error | A `*.eval.yaml`/`*.eval.yml`/`*.eval.json` file is malformed: unknown field, missing `expect_trigger` or prompt, bad assertion, invalid regex, unsafe path, duplicate id |
| `AR997` | `eval-stale` | off | The skill changed after its last recorded **passing** run. `[lint.evals] require_fresh = "warn"` or `"error"` turns it on |
| `AR998` | `eval-score-low` | off | The recorded pass rate is below `[lint.evals] min_pass_rate` (0-1); setting it turns the rule on at error |
| `AR9A0` | `eval-results-invalid` | error | `eval-results.json` cannot be parsed or has an unsupported `schema_version` |

Only records signed with your own key (`eval-results.key` in the user config directory, written by `eval run`) count
as evidence. A record without a valid signature (committed from another machine, edited by hand or forged) is
unverified: when `AR997` or `AR998` is enabled it is reported with an "unverified" message instead of being treated as
a passing run. Lint never creates the key, so on a machine that has never run `eval run` every record is unverified.

```toml
[lint.evals]
require = true            # AR962: skills need cases
require_fresh = "error"   # AR997: edited after the last passing eval
min_pass_rate = 0.8       # AR998, and the default pass mark of eval run
```

A skill with no recorded passing run is not reported stale (that is what `AR962` and the score are for).

## Reports

`ai-rulez report evals` joins the results with the usage log and the feedback log (see
[Usage telemetry](usage-telemetry.md)) and recommends an action per skill. The rules are fixed:

| Action | When |
| --- | --- |
| `rewrite` | pass rate below `--min-pass-rate` (0.8), trigger precision or recall below `--min-trigger` (0.8), a negative ablation delta, edited since the last passing eval, or `misled`+`wrong`+`stale` feedback outweighing `great` |
| `prune` | not rewrite; a usage log was given; never used in it; and evals do not show it helping (no record, or ablation delta of 5 points or less) |
| `review` | not rewrite or prune, but unused while evals show value, or no eval results yet |
| `keep` | everything else |

Rows are ordered by action, then by number of reasons, then by pass rate (worst first), then by `SKILL.md` size.
Without a usage log nothing is concluded about use. `--json` prints the same data. `report usage` shows feedback
counts and the eval pass rate next to each skill.

With a usage log each row also carries a `join` class (and `join_uses`, the uses behind it by class) saying how far
the usage evidence is tied to the evaluated skill:

| Class | Meaning |
| --- | --- |
| `exact` | a use was logged at the skill digest the eval ran on (the record's `lock_digest`) |
| `stale` | the uses carry a digest, but none is the evaluated one: the score describes another version of the skill |
| `legacy` | the record has no `lock_digest`, or the uses have no canonical digest (version 2 log lines, an index without digests): matched by skill id only |
| `none` | the skill has no logged use, or no eval record |

The class does not change the recommended action; it says how much weight a row's usage deserves.

## Bundling cases into a plugin

Eval cases are authoring material, so a plugin bundle leaves them out by default. (Before this option existed a
skill's `evals/` directory was copied along with the rest of the skill directory; it is now excluded unless you opt
in.) Opt in with `include_evals`:

```toml
[plugin]
name = "billing"
version = "1.0.0"
include_evals = true
```

With it on, every runtime bundle that carries skills contains:

- `skills/<name>/evals/...` for each bundled skill, and
- `evals/...`, a copy of `.ai-rulez/evals/` (or `<content_root>/evals/` when `content_root` is set), at the bundle
  root next to `skills/`.

For a per-domain plugin (`[marketplace.from_domains]`) only `evals/<skill-name>/...` of the skills in that plugin is
bundled, so one plugin never carries another plugin's cases. The copy obeys the same rules as other bundled files:
regular files only, and a symlink that resolves outside the project is refused.

The provenance sidecar `.ai-rulez-generated.json` records a hash of every bundled file, cases included, so
`ai-rulez verify --plugin` fails when a case is edited or removed in the bundle.

## Linting for skills without cases

`evals-missing` (`AR962`) is part of [strict validation](strict-validation.md) and is **off by default**:

```toml
[lint.evals]
require = true                     # turns AR962 on at warning severity
allow = ["internal-*", "scratch"]  # skill names or globs exempt from the check

[lint.severity]
evals-missing = "error"            # or set the severity directly; this wins over require
```

A skill passes when `skills/<name>/evals/` or `.ai-rulez/evals/<name>/` contains at least one non-hidden file.
`.gitkeep` does not count.

```console
$ ai-rulez validate --strict
.ai-rulez/skills/deploy-staging/SKILL.md:2  warning  AR962 evals-missing  skill "deploy-staging" has no eval cases ...
```

## Running evals in CI

```bash
set -euo pipefail

ai-rulez validate --strict                 # AR962/AR996/AR997/AR998 included when enabled; exit 2 on findings at or above fail_on
ai-rulez generate --plugin                 # writes the bundle, cases included with include_evals = true
ai-rulez verify --plugin                   # exit non-zero if any bundled file differs from its recorded hash

# Only the skills this change touched, with a JUnit report CI can display.
ai-rulez eval run --changed-only --base origin/main \
    --ablation --max-cost 5 --date "$(date -u +%F)" \
    --format junit --out eval-report --runner-arg --trust-plugin
```

- **Against the git diff.** `--changed-only --base origin/main` runs the skills that have a changed or untracked file
  below their directory or below `.ai-rulez/evals/<skill>/`. Make the base ref available (`git fetch origin main`, or a
  full-depth checkout). Without `--changed-only` every skill with cases is selected.
- **JUnit.** `--format junit --out eval-report` writes `eval-report/eval-report.xml`: one suite per skill, one test case
  per eval case (near misses included), failures for failed cases, errors for runner errors and invalid case files,
  skipped for skipped or dry-run entries, and the scores as suite properties. The file has no timestamps or timings, so
  it is byte-stable. Point your CI's test reporter at it.
- **Caching by digest.** Commit `.ai-rulez/eval-results.json`, or restore it from your CI cache. A skill whose digest
  and cases digest match the stored run is reported as `cached` with its stored score and is not run again, so an
  unchanged skill costs nothing even without `--changed-only` (only records signed with your own key replay; see [Caching](#caching)). Commit the updated file (or save it back to the cache)
  after a run that changed it; `validate --strict` with `require_fresh` then fails the next change that edits a skill
  without re-running its evals.
- **Cost controls.** Run `eval run --dry-run` first to see the estimate; set `--max-cost` so a runaway suite stops
  (estimate above the cap: refuse to start; spend reaching it: skip the rest, exit 2); keep `--runs` low in CI and
  `--ablation` on only where you track the delta; use a cheap `--model` or per-case `model` for smoke cases; a
  `--date` is required for a dated result and comes from CI, never from the clock inside ai-rulez.
- **Exit codes.** `eval run` exits `0` clean, `2` for a failing/errored/over-budget/invalid skill, `1` when it could not
  run at all. `validate --strict` exits `0` clean, `1` for an invalid configuration and `2` for findings at or above
  `fail_on`; `verify --plugin` exits non-zero on any mismatch.
- **Secrets and trust.** The runner runs the harness as the CI user with whatever credentials that job has. Evaluate
  only skills and cases you trust; `command_exit` assertions and `--runner-arg --trust-plugin` are opt-ins for that
  reason. ai-rulez itself makes no network call; the runner you choose does.
