---
title: "Improve (experimental)"
description: "ai-rulez improve run <skill> --with CMD runs an optimizer (a program that rewrites a skill) on a throwaway copy of the skill and keeps its candidate only if the skill does measurably better on eval cases the optimizer never saw."
---

`ai-rulez improve run <skill> --with CMD` runs an optimizer (a program that rewrites a skill) on a throwaway copy of
the skill and keeps its candidate only if the skill does measurably better on eval cases the optimizer never saw.
ai-rulez provides the safety around the optimizer, not the optimizer itself: a disposable workspace, a withheld
held-out set, a strict acceptance gate, a diff policy, a cost ceiling, and a reviewed apply step or pull request.

:::caution[Experimental]
Flags, the optimizer protocol and `report.json` may change before this is stable. Every run prints a warning.
:::

## Example

Tag at least three eval cases of the skill `holdout`, one of them negative (see [Evals](/ai-rulez/evals/#case-format)):

```yaml
# .ai-rulez/skills/deploy-staging/evals/holdout.eval.yaml
schema_version: 1
cases:
  - id: ho-ship
    prompt: Ship the payments service to staging
    expect_trigger: true
    tags: [holdout]
  - id: ho-rollout
    prompt: Roll out web to the staging cluster
    expect_trigger: true
    tags: [holdout]
  - id: ho-prod
    prompt: Explain our production on-call rota
    expect_trigger: false
    tags: [holdout]
```

Commit, then look at the plan. `--dry-run` runs nothing:

```console
$ ai-rulez improve run deploy-staging --with 'python3 optimize.py' --runner-command 'python3 eval-runner.py' --harness codex --max-cost 2 --dry-run
improve deploy-staging (experimental)
  optimizer:   python3 /home/dev/acme/optimize.py (run without a shell, in a throwaway copy)
  split:       3 train case(s), 3 held-out case(s) (tag)
  gate:        gain >= 5 points and one stable win, <= 0 regression(s), 3 round(s), 3 held-out evaluation(s)
               held-out >= 3 case(s) (share 30% when untagged), SKILL.md growth <= 1.25x, 95% interval not required
  estimate:    72 agent runs, about $1.10 of evals; --max-cost $2.00
  sends out:   the skill text and the 3 train case(s) go to whatever the optimizer calls; held-out cases never leave ai-rulez
  environment: scrubbed (no credentials are passed)
  egress:      none declared (ai-rulez cannot block the optimizer's network access; use a container or CI egress policy)
  isolation:   none (the optimizer runs as you, unconfined, ...)
  run:         .ai-rulez/local/improve/imp-649891f5 (nothing outside it changes until `improve apply`)

Dry run: nothing was run and nothing was written.
```

Run it, review the result, then apply it or open a pull request:

```bash
ai-rulez improve run deploy-staging --with 'python3 optimize.py' --runner-command 'python3 eval-runner.py' --harness codex --max-cost 2 --yes
ai-rulez improve show imp-649891f5
ai-rulez improve apply imp-649891f5          # write into the working tree, no commit
ai-rulez lock && ai-rulez eval run deploy-staging && ai-rulez validate
ai-rulez improve pr imp-649891f5 --draft     # or: a pull request from an isolated worktree
```

A run whose baseline already passes every held-out case stops before the optimizer spends anything:

```text
Run imp-649891f5 for deploy-staging: no-candidate
Baseline held-out pass rate 100% (3 case(s))
nothing to gain on held-out: the baseline already passes every held-out case or the required gain cannot be reached
Spent $0.27 of $2.00 (evals $0.27, optimizer-reported $0.00)
```

## Commands

| Command | What it does | Exit status |
| --- | --- | --- |
| `improve run <skill>` | Run the optimizer and gate its candidates. `--format json` prints the run result as JSON. | 0 accepted, 2 no acceptable candidate (report written), 1 refused or could not run |
| `improve show <run-id>` | Print the report and the diff. `--format json` prints `improve-show/1`. | |
| `improve apply <run-id>` | Write an accepted candidate into the skill, after confirmation (`--yes` skips it). Never commits. | |
| `improve pr <run-id>` | Commit the candidate on a new branch from an isolated worktree and open a pull request. | |
| `improve clean [run-id]` | Delete saved runs (`--all`, `--dry-run`; `--all` asks unless `--yes`). | |
| `improve adapters [name]` | List the bundled adapters, or print one's template. | |

## Preconditions

A run is refused, with the reason, unless:

1. The skill is authored in the project (under `skills/` or a domain), not an include, installed skill or built-in.
2. It has eval cases (`AR962`) and they parse (`AR996`).
3. At least three scored held-out cases exist, one of them negative (`expect_trigger: false` or a near miss)
   (`AR9J2`).
4. The skill and its evals have no uncommitted changes (in a git repository; otherwise a warning).
5. `--max-cost` is set and the eval estimate fits under it.
6. The secret scan is clean for the skill files and the train cases the optimizer will receive.

## Held-out split

Cases tagged `--holdout-tag` (default `holdout`) are held out. With no tagged case, a hash of the case id decides,
holding out about `--holdout-fraction` (default 0.3) of the cases, the same ones every run. Near-miss expansions stay
with their parent case. A train prompt that repeats a held-out prompt is reported as a warning.

The optimizer receives train cases only. Held-out prompts, assertions and rubrics are never written to the workspace,
the request, the environment or the logs. Per round the optimizer learns only a one-word decision (`accepted`,
`rejected: below gain`, `rejected: regression`, ...), never per-case held-out results.

## Optimizer protocol v1

The command runs without a shell. Relative paths in `--with` are made absolute against the directory you run improve
in (the program when it contains a path separator, and any later argument that names an existing file). Its working
directory is `.ai-rulez/local/improve/<run-id>/workspace/`, `AI_RULEZ_IMPROVE_PROTOCOL=1` is set, the request arrives
on standard input and the response is read from standard output (at most 1 MiB). It edits the files under `<skill>/`
in the working directory.

```json
{ "version": 1, "run_id": "imp-9f3a1c2d", "round": 1,
  "skill": { "id": "deploy", "dir": "deploy", "digest": "sha256:..." },
  "train_cases": [ { "id": "deploy-basic", "prompt": "...", "expect_trigger": true, "assertions": [] } ],
  "train_scores": { "pass_rate": 0.67, "trigger_precision": 1.0, "trigger_recall": 0.5 },
  "constraints": { "editable": ["SKILL.md", "references/**"],
                   "frontmatter_immutable": ["name", "allowed-tools", "disable-model-invocation", "model"],
                   "max_skill_tokens": 2400, "forbid": ["scripts/**", "assets/**"] },
  "budget": { "max_cost_usd": 2.0, "timeout_s": 1200 },
  "history": ["rejected: below gain"],
  "previous": { "round": 1, "decision": "rejected: policy", "reasons": ["AR9J3 outside-editable: ... (scripts/run.sh)"],
                "summary": "Added a helper script.", "workspace_kept": false } }
```

```json
{ "version": 1, "summary": "Tightened the description.", "changed": ["SKILL.md"], "cost_usd": 0.42, "notes": "optional" }
```

| Request field | Meaning |
| --- | --- |
| `train_cases`, `train_scores` | The cases the optimizer may use and the baseline's scores on them. |
| `constraints` | What the diff policy allows. |
| `budget` | What this round may spend and how long it may take. |
| `history` | The decisions of earlier rounds. |
| `previous` | How the last round ended (absent in round 1): the decision, the reasons when the round was rejected before the held-out set was consulted, the optimizer's own summary, and whether the workspace still holds that attempt. |

| Response field | Meaning |
| --- | --- |
| `version` | Must be `1`. |
| `summary`, `notes` | Free text. Treated as untrusted: control characters are removed and they are never fed to a model. |
| `changed` | Informational; ai-rulez computes the real diff. |
| `cost_usd` | What the optimizer spent. Counts against `--max-cost`; an optimizer that never reports a cost is flagged. |

A crash, timeout, invalid JSON or wrong version rejects that round only. The round is still charged: the `cost_usd` of
a well-formed answer printed before the failure, else the round's whole `budget.max_cost_usd`.

## Bundled adapters

`ai-rulez improve adapters` lists them; `improve adapters <name>` prints a template. Run a runnable one with
`--with builtin:<name>` or `--adapter <name>`. An adapter runs as a child process with the same sandbox, scrubbed
environment, diff policy and held-out gate as any other optimizer.

| Adapter | Kind | What it does |
| --- | --- | --- |
| `builtin:noop` | runnable | Changes nothing and reports no cost; for trying the protocol. |
| `builtin:review-fix` | runnable | Judges `SKILL.md` with the review rubric and applies a verified fix. |
| `shell` | template | A shell script that calls any tool on the workspace copy. |
| `research` | recipe | How to wire a research optimizer; a recipe only, not runnable. |
| `repair-workflow` | template | A scheduled GitHub Actions workflow, see [Scheduled repair workflow](#scheduled-repair-workflow). |

### `builtin:review-fix`

`builtin:review-fix` runs the [`review fix`](/ai-rulez/review/#review-fix) pipeline on `SKILL.md`: the judge (rubric
`builtin:skill-quality`, content `full`) rates the skill, and for stable findings the fixer proposes edits that must
pass the review checks (only the description and body change, growth within `[review.fix] max_growth_percent` and the
run's `max_skill_tokens`, no new link, credential or security finding, and a judge of another model rates the targeted
dimensions better and no other worse). A verified fix is written to the workspace, and the held-out gate decides whether
it is kept.

- It needs `[llm] model` and `allow_network = true` in the **user** config (the same opt-in as `review --semantic`, see
  [LLM access](/ai-rulez/llm/)), and a fixer model different from the judge: `--adapter-model` or `[review.fix] model`, with
  `--adapter-judge-model` for the judge. `--allow-same-model` overrides this, with the self-preference risk. Missing
  settings refuse the run before anything starts (`AR9J9`).
- Pin model ids: a fixer that must differ from the judge cannot share its alias.
- The key is never passed to the child: the `api_key_env` variable name is forwarded, and the provider host is declared
  as egress unless you pass `--egress`.
- The adapter's model spend is reported as the optimizer cost, bounded by the run's remaining budget.
- Each round is a new process; the request's `previous` field tells the fixer how the last round ended, so it does not
  repeat a rejected attempt.

## Diff policy

Checked after every round, before any eval spend. A violation is `AR9J3`: the round is rejected and the workspace
restored to the last good state.

- A change outside `editable` (default `SKILL.md` and `references/**`), including new, deleted or forbidden files.
- A symlink, hard link, special or oversized file, or an executable-bit change.
- A change to `name`, `allowed-tools`, `disable-model-invocation` or `model` (`--allow-frontmatter` frees all but
  `name`).
- A new reference to `scripts/` (`--allow-scripts` frees `scripts/**` and `assets/**`).
- `SKILL.md` above `[improve] max_skill_growth` (default 1.25) times its original token count.
- A new security finding (`AR0xx`) in a changed file; findings already in the original never block.
- A write outside the skill directory: the workspace root, the read-only original, the train cases or the authored
  skill.

A held-out assertion string that appears in the candidate but not in the original is reported as a possible leak.

## Sibling trigger guard

A rewritten description can pull prompts away from another skill. When a candidate changes what the `find_skill`
ranker reads (`name`, `description`, `triggers`, `keywords`), the trigger cases of every other skill are re-run with
the offline ranker of [activation mode](/ai-rulez/evals/#activation-mode), with the original and with the candidate in place.
A sibling whose trigger recall drops rejects the round (`AR9J4`) before any held-out spend. The guard is free and
always on when another skill has trigger cases. A sibling that cannot be copied safely is listed as `unmeasured`.

`--sibling-native` adds a second guard on the harness's own model (`claude-native`, or a `--runner-command` that
declares the activation capability): each sibling prompt runs `--sibling-runs` times (default 3) for the original and
for each candidate. It costs money, counted against `--max-cost`, and a guard that cannot run rejects the round. A
body-only edit skips it. Model runs are noisy: raise `--sibling-runs` for a large suite.

## Acceptance gate

Baseline and candidate are both measured in the run on the held-out set, `--runs` times each (default 3), taking the
majority outcome per case. A candidate is accepted only if all of these hold:

- At least one held-out case was scored by both, and the candidate was scored on every case the baseline was.
- The held-out pass-rate gain is at least `--min-gain` (default 0.05), with at least one stable win.
- At most `--max-regressions` (default 0) held-out cases flip from pass to fail.
- Trigger precision and recall do not drop, and near-miss false positives do not rise.
- No sibling lost trigger recall, and the diff policy held.

Cases that are not unanimous across runs are `unstable`: shown, never counted as wins, while a majority pass-to-fail
flip still counts as a loss. `--max-holdout-evals` (default 3) bounds how often the held-out set is consumed, and
`--max-rounds` (default 3) how often the optimizer runs. The best accepted round is kept (highest gain, earliest on
ties), or the first with `--stop-at-first-accept`. When several rounds were scored on the held-out set the reported
gain is optimistic, and `apply` and the pull request body say "selected among N held-out evaluations".

**Confidence interval.** Every comparison reports a bootstrap 95% interval of the gain, reproducible for the same run.
With 5 to 20 cases almost nothing is significant, so the interval informs rather than gates: fewer than 8 held-out
cases, or an interval that includes zero, is marked `underpowered` (`AR9J5`). `--require-ci-above-zero` (or `[improve]
require_ci_above_zero`) makes the interval's lower end a gate condition; the decision is then
`rejected: below confidence`.

## Cost

`--max-cost` is required. It covers the measured eval cost plus the optimizer's reported cost, and the pre-run estimate
must fit under it. When spend reaches it the loop stops (`stopped: over budget`) and keeps the best accepted
candidate. An eval runner that reports no cost is charged the whole remaining budget. A candidate whose held-out
measurement the budget cut short is rejected (`rejected: over budget`).

## Security

The optimizer is untrusted code running as you. It starts without a shell, in a scrubbed environment (`PATH`, locale,
`TZ` and the names in `--env-pass`; `HOME` and the temp directories point into the run directory), with a timeout
(`--timeout`, default 20m) that kills its process tree, and capped output. A credential-like name in `--env-pass` is
refused unless `--egress HOST` declares where data goes; the declaration is printed and recorded but **not enforced**.

**Sandbox.** `--isolation auto|require` (or `[improve] isolation`) confines the optimizer: macOS `sandbox-exec`,
Linux bubblewrap, or `unshare` (network only). Writes are limited to the run's workspace, home and temp directories,
and the network is denied unless `--egress` declares a host. `require` refuses to run without a working backend
(`AR9J7`); `auto` confines when it can and says so when it cannot; the default `none` leaves the optimizer unconfined.
Reads are never restricted.

The held-out gate protects against an honest optimizer that overfits, not a hostile one. A process running as you can
read the held-out case files and the per-user key that signs reports, and the sandbox does not hide them. If you do not
trust the optimizer, run `improve run` in a container or CI job with no access beyond the run workspace, and an egress
policy.

**A repository cannot choose what runs.** A repository `[improve] optimizer` or `env_pass` (including in
`config.local.*`) is used only with `--trust-repo-optimizer`; otherwise it is ignored and reported (`AR9J6`). The same
keys in your user config apply without the flag.

**A repository may tighten the gate, not weaken it.** A repository value of `min_gain`, `max_regressions`,
`holdout_fraction`, `max_skill_growth`, `runs`, `max_rounds` or `max_holdout_evals` applies only when it is at least as
strict as the default (gain at least 0.05, 0 regressions, held-out share at least 0.3, growth at most 1.25, at least 3
runs, at most 3 rounds and 3 held-out evaluations). `require_ci_above_zero = true` and a higher `min_holdout_cases`
always apply. A looser repository value is dropped with an `AR9J6` warning, and `validate` reports it too. Your user
config and flags may set any value.

What leaves the machine: the skill text and the train cases, to whatever the optimizer calls. The consent summary says
so and needs `--yes` or an interactive confirmation.

## Configuration

Defaults of `improve run`. A flag always wins; the user config wins over the repository config.

```toml
[improve]
holdout_tag = "holdout"
holdout_fraction = 0.3
min_holdout_cases = 3        # never below 3
min_gain = 0.05
max_regressions = 0
runs = 3
max_rounds = 3
max_holdout_evals = 3
max_skill_growth = 1.25      # at most 2
require_ci_above_zero = false
isolation = "none"           # none | auto | require
env_pass = []                # repository value used only with --trust-repo-optimizer
# optimizer = "..."          # a command, or builtin:<adapter>; same trust rule
```

Other `run` flags pass through to the eval runner: `--runner-command`, `--harness`, `--model`, `--claude-bin`,
`--runner-arg`, `--judge-model`, `--allow-exec`, `--eval-timeout`, `--price-in`, `--price-out` and `--date` (see
[Evals](/ai-rulez/evals/#running-evals)).

## Run output

A run lives under `.ai-rulez/local/improve/<run-id>/` (machine-local, git-ignored; ids look like `imp-649891f5`):
`plan.json`, `original/`, `workspace/`, `train/`, `rounds/<n>/candidate/`, `report.json`
([`improve-report/1`](/ai-rulez/schema/)) and, when a candidate was accepted, `diff.patch` (applies with `git apply`).
`report.json` records the split by case id, every round's decision and reasons, before and after scores with the
interval, the sibling guards, wins and losses, costs, declared egress, forwarded variable names (not values) and the
isolation. A run that stops on an error after measuring still writes its report with the spend so far and exits 1.

If the optimizer changes anything in the run directory outside `workspace/`, `home/` and `tmp/`, or the authored skill,
the round is rejected (`outside-workspace`) and everything is restored. `show` and `apply` print the optimizer's and
the candidate's text with control and bidi characters replaced, so a candidate cannot write terminal escapes.

## Apply

`improve apply <run-id>` shows the diff, asks for confirmation (`--yes` skips it) and writes the files, all or nothing.
It never commits. It refuses with `AR9J1` when the skill changed since the run (so a second apply fails), when the
saved candidate no longer matches its digest, or when the candidate now breaks the diff policy.

`improve run` signs `report.json` with the per-user key (`eval-results.key` in the user config directory, shared with
`eval run`). `apply` and `pr` refuse a run whose signature is missing, was edited, or came from another machine or
user. The diff policy is recomputed from the defaults and the flags you pass now: edits to `scripts/` and `assets/`
need `--allow-scripts`, and frontmatter changes need `--allow-frontmatter`, even if the run used them.

## Pull request

`improve pr <run-id>` makes the change reviewable without touching your checkout:

1. Creates a linked git worktree on a new branch `ai-rulez/improve/<skill>-<digest8>`, started from `--base` (default:
   your current branch; a detached HEAD needs `--base`).
2. Checks that the skill at the base has the digest the run measured (`AR9J1` otherwise), re-checks the diff policy and
   writes the candidate.
3. Runs `ai-rulez generate --yes` and, when the project has a lock, `ai-rulez lock`. With `--run-evals` it also runs
   `ai-rulez eval run <skill> --changed-only` (plus any `--eval-arg`), which calls the eval runner and costs money;
   without it, the command prints what to run on the branch. A failing step leaves no branch.
4. Stages the skill, the lock, the eval results and any generated outputs git tracks, and makes one commit with git
   hooks skipped. The worktree is removed; the branch stays.
5. When the remote (`--remote`, default `origin`) exists, `gh` is on `PATH`, the base is a pushed branch and you
   confirm (or pass `--yes`), pushes the branch and runs `gh pr create` (`--draft` for a draft). Otherwise, or with
   `--no-push`, it prints the two commands.

The worktree commands run with a scrubbed environment; pass the names an eval runner needs with `--env-pass`.
`--isolation auto|require` confines them to the worktree, ai-rulez's cache and data directories and the temp
directory, and cuts the network for `generate` and `lock` (`--allow-network` keeps it for uncached remote includes).
A harness that keeps state elsewhere cannot run `--run-evals` confined. Git and `gh` never run through a shell, no
repository git hook runs, and only the push receives transport credentials.

The pull request body states what changed, the held-out table with the interval, the guards that held, cost, egress,
the description before and after, the optimizer's own words (fenced, as untrusted text), a reviewer checklist, and that
the change is **not approved**. Refusals carry `AR9J8`.

## Scheduled repair workflow

A model upgrade can make a skill that passed its evals fail. `ai-rulez improve adapters repair-workflow` prints a
GitHub Actions workflow (save it as `.github/workflows/ai-rulez-repair.yml`) that repairs such skills on a schedule:

1. `eval run --model $EVAL_MODEL --max-cost $EVAL_BUDGET --format json --no-write` finds the skills that now fail.
2. For each one, `improve run <skill> --adapter review-fix --sibling-native --max-cost $REPAIR_BUDGET --yes` repairs
   it under the usual gate.
3. A run that exits 0 gets `improve pr <run-id> --draft --yes`. A run with no acceptable candidate opens nothing.

Nothing is merged, approved or applied for you. The workflow has no `pull_request` trigger, so a fork's pull request
never reaches its secrets, and its permissions are `contents: write` and `pull-requests: write`. Each spending step has
its own ceiling in the `env` block. `builtin:review-fix` needs the `[llm]` user config the template writes; replace that
step with `--with` to use your own optimizer. `improve run` and `improve pr` must run in the same job, because `pr`
needs the report signature of the machine that made the run.

## Codes

| Code | Name | Meaning |
| --- | --- | --- |
| `AR9J1` | `improve-run-stale` | The skill changed since the run (or differs at the base of `pr`). |
| `AR9J2` | `improve-no-holdout` | Too few held-out cases. |
| `AR9J3` | `improve-policy-violation` | A candidate round broke the diff policy. |
| `AR9J4` | `improve-sibling-regression` | A candidate lowered another skill's trigger recall. |
| `AR9J5` | `improve-underpowered` | The held-out gain cannot be told from zero. |
| `AR9J6` | `improve-repo-optimizer-ignored` | A repository `[improve]` optimizer, `env_pass` or looser gate value was not used. |
| `AR9J7` | `improve-isolation-unavailable` | The requested isolation could not be applied. |
| `AR9J8` | `improve-pr-refused` | `improve pr` refused. |
| `AR9J9` | `improve-adapter-refused` | A bundled adapter could not run. |

They appear in `improve` output and the report; `validate` reports only `AR9J6`. See
[strict validation](/ai-rulez/strict-validation/).
