# Improve (experimental)

`ai-rulez improve run <skill> --with CMD` runs an external optimizer (a tool that rewrites a skill against a
benchmark) on a throwaway copy of the skill and accepts a candidate only if a held-out eval set improves. ai-rulez
implements no optimizer. It owns the parts that make one safe to use on real sources: a disposable workspace,
a withheld held-out set, a strict acceptance gate, a diff policy, a cost ceiling and a human-reviewed apply step
or pull request.

> **Experimental.** Flags, the optimizer protocol and `report.json` may change before this is stable. The command
> prints a warning on every run.

## Quick start

```bash
# 1. tag at least three eval cases `holdout` (one negative), keep others as training cases
# 2. see the plan and the estimate; nothing runs
ai-rulez improve run deploy --with 'python optimize.py' --max-cost 5 --dry-run
# 3. run it
ai-rulez improve run deploy --with 'python optimize.py' --max-cost 5 --yes
# 4. review the run, then write it into the working tree...
ai-rulez improve show <run-id>
ai-rulez improve apply <run-id>
ai-rulez lock && ai-rulez eval run deploy && ai-rulez validate
# ...or open a pull request from an isolated worktree
ai-rulez improve pr <run-id>
```

Commands: `run`, `apply`, `show`, `clean`, `pr`, `adapters`. Exit status of `run`: 0 candidate accepted, 2 no acceptable
candidate (report written), 1 refused or could not run.

## Preconditions

A run is refused, with the reason, unless:

1. The skill is authored (under `skills/` or a domain), not from an include, an installed skill or a built-in.
2. Eval cases exist (`AR962`) and parse (`AR996`).
3. At least three scored held-out cases exist, one of them negative (`expect_trigger: false` or a near miss) (`AR9J2`).
4. The skill and its evals have no uncommitted changes (checked when the project is a git repository; otherwise a warning).
5. `--max-cost` is set and the eval estimate does not exceed it.
6. The secret scan is clean for the skill files and the train cases (prompts, fixtures, assertions, `rubric` and
   `rubric_items`) the optimizer will receive.

## Held-out split

Cases tagged `--holdout-tag` (default `holdout`) are held out. With no tagged case, `hash(case id) mod 100 <
--holdout-fraction * 100` decides, deterministically. Near-miss expansions stay with their parent. A train prompt
that repeats a held-out prompt is reported as a warning.

**The optimizer receives train cases only.** Held-out prompts, assertions and rubrics are never written to the
workspace, the request, the environment or the logs. The optimizer learns only a one-word decision per round
(`accepted`, `rejected: below gain`, `rejected: regression`, ...), never per-case held-out results. Tests search
the request, environment, run directory and stderr for canary strings from held-out cases.

## Optimizer protocol v1

The command runs without a shell, with the working directory `.ai-rulez/local/improve/<run-id>/workspace/`,
`AI_RULEZ_IMPROVE_PROTOCOL=1`, the request on standard input and the response on standard output (at most 1 MiB).
It edits the files in `<skill>/` under the working directory.

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

`previous` (absent in round 1) lets a stateless optimizer avoid repeating itself: how the last round ended, the
optimizer's own summary of it, and whether the workspace still holds that attempt (a round the gate rejected) or was
reset to the state before it (a round rejected earlier, by the diff policy, the sibling guard or a failure). `reasons`
are given only for rounds decided before the held-out set was consulted; a held-out gate decision carries the one-word
decision and nothing else.

`changed` is informational: ai-rulez computes the real diff. `cost_usd` is self-reported and counts against
`--max-cost`; a run whose optimizer never reports a cost is flagged. `summary` and `notes` are untrusted: control
characters are removed and they are never fed to a model. A crash, timeout, invalid JSON or wrong version rejects
that round only.

## Bundled adapters

`ai-rulez improve adapters` lists them; `improve adapters <name>` prints a template. Select a runnable one with
`--with builtin:<name>` or `--adapter <name>`. An adapter is a child process of improve (`ai-rulez improve adapter
<name>`, hidden), so it gets the same sandbox, scrubbed environment, diff policy and held-out gate as any other
optimizer; nothing in the gate trusts it.

| Adapter | Kind | What it does |
|---|---|---|
| `builtin:noop` | runnable | changes nothing and reports no cost; for trying the protocol |
| `builtin:review-fix` | runnable | judges `SKILL.md` with the review rubric and applies a verified fix |
| `shell` | template | a shell script that calls any tool on the workspace copy |
| `research` | recipe | how to wire a research optimizer; its interface is unverified, so it is not shipped as supported |
| `repair-workflow` | template | a scheduled GitHub Actions workflow for [model-upgrade repair](#scheduled-repair-workflow) |

**`builtin:review-fix`** drives the review pipeline of [`review fix`](review.md) on `SKILL.md`: the judge
(rubric `builtin:skill-quality`, content `full`) rates the skill; for stable findings the fixer proposes exact-text
edits that must pass the review checks (only the description and body change, growth at most `[review.fix]
max_growth_percent` and within the run's `max_skill_tokens`, no new link or credential, no new security finding, and a
judge of another model rates the targeted dimensions better and no other worse). A verified fix is written to the workspace; the gate then decides
whether it is kept. It never sees held-out cases (it uses none).

- Needs `[llm] model` and `allow_network = true` in the **user** config, the same opt-in as `review --semantic`, and
  a fixer model different from the judge (`--adapter-model` or `[review.fix] model`; `--allow-same-model` overrides, with
  the self-preference risk). Any of these missing refuses the run before anything starts (`AR9J9`).
- The parent resolves the settings (user config, repository trust, `allowed_hosts`) and hands the child the resolved
  `[llm]` table as an argument. A key is never passed: the `api_key_env` variable name is added to `--env-pass`, and
  the provider host (`base_url` host, else `provider-default`) is declared as egress unless you pass `--egress`.
- Spend: the adapter's own model cost is reported as the optimizer cost, bounded by the run's remaining budget.
- A model that must be told apart from the judge cannot be the same alias: pin model ids.
- The adapter is a new process every round, so the request's `previous` field is its only memory. It hands the fixer, from
  the first attempt, how the last round ended (the decision, the reasons of a round decided before the held-out set was
  consulted, the adapter's own summary, and whether the file still holds that attempt), so a round after a rejection
  does not repeat it. This is the `FixInput.Feedback` hook of the [review fix](review.md#review-fix) API.

## Diff policy

Checked after every round, before any eval spend. A violation is `AR9J3` and rejects the round (the workspace is
restored to the last good state):

- a change outside `editable` (default `SKILL.md`, `references/**`), including new, deleted or forbidden files;
- a symlink, hard link, special or oversized file; an executable-bit change;
- a change to `name`, `allowed-tools`, `disable-model-invocation` or `model` (`--allow-frontmatter` frees all but `name`);
- a new reference to `scripts/` (`--allow-scripts` frees `scripts/**` and `assets/**`);
- `SKILL.md` above `[improve] max_skill_growth` (default 1.25) times the original token count;
- a new security finding (`AR0xx`) in a changed file; findings already in the original never block;
- a write outside the skill directory: the workspace root, the read-only original, the train cases or the authored skill.

A held-out assertion string that appears in the candidate but not in the original is reported as a warning (possible leak).

## Sibling trigger guard

A rewritten description can pull prompts away from another skill. When a candidate changes what the `find_skill`
ranker reads (`name`, `description`, `triggers`, `keywords`), the trigger cases of every other skill of the project
are re-run with the offline ranker, once with the original and once with the candidate in the target's place. A
sibling whose trigger recall drops rejects the round as a regression (`AR9J4`), before any held-out spend and
without consuming a held-out evaluation. The report lists each sibling's recall and the prompts it lost.

The guard uses the retrieval surface of activation mode: no model, no cost, the same result everywhere. It is on
whenever another skill has trigger cases (design question 5: mandatory, because it is free). It never copies the
target skill's eval cases into its scratch tree.

A sibling that cannot be copied safely (a symlinked or oversized `SKILL.md`, too many eval files) is left out of both
arms and listed under `unmeasured` in the report, with a warning on the round; the guard measures the others instead
of failing every round.

**Native surface (`--sibling-native`).** The offline ranker is not the model that picks skills in a harness, so a
description that the ranker leaves alone can still pull a sibling's prompts away from the real model. `--sibling-native`
adds a second guard after the free one passes: the siblings' trigger prompts run through the harness's model
(`claude-native`, or a `--runner-command` that declares the activation capability and the native surface), each prompt
`--sibling-runs` times (default 3), once for the original, measured on the first round and reused, and once per
candidate. A sibling whose trigger recall drops rejects the round (`AR9J4`) exactly like the free guard, and the
result is `siblings_native` in the report. It costs money: every call is a model call, the spend (`cost_usd` of the
guard) counts against `--max-cost`, the run stops when no budget is left, and a guard that cannot run rejects the
round rather than clearing it. A body-only edit skips it. Model runs are noisy: raise `--sibling-runs` for a
large suite, and read a single regression with that in mind. The consent summary prints the guard when it is on.

## Acceptance gate

Baseline and candidate are both measured in the run on the held-out set, `--runs` times each, with the majority
outcome per case. A candidate is accepted only if all hold:

- at least one held-out case was scored by both arms, and the candidate was scored on every case the baseline was
  (a case the candidate could not be scored on counts as a loss, never as a pass; a gain of 0 with `--min-gain 0`
  is not evidence);
- held-out pass rate gain >= `--min-gain` and at least one stable win (so `--min-gain 0` never accepts a candidate
  that gained nothing; unstable cases are not wins);
- held-out cases flipping pass to fail <= `--max-regressions`;
- trigger precision and recall do not drop, and near-miss false positives do not rise;
- no sibling lost trigger recall (above);
- the diff policy held.

Cases that are not unanimous across runs are `unstable`: shown, but never counted as wins or losses. A baseline that
already passes everything exits 2 before the optimizer runs. `--max-holdout-evals` bounds how often the held-out
set is consumed. Each round continues from the optimizer's own state; the best accepted round (highest gain,
earliest on ties) is kept, or the first with `--stop-at-first-accept`.

**Confidence interval.** Every comparison carries a percentile bootstrap 95% interval of the gain: 10,000 resamples
of the paired held-out cases, seeded from the case ids and outcomes so the same run always reports the same
interval. It is informative, not a gate: with 5 to 20 cases almost nothing is significant, and pretending otherwise
is worse than saying so. A run with fewer than 8 held-out cases, or an interval that includes zero, is marked
`underpowered` (`AR9J5`); the round warns about it only when it was accepted. Unstable cases count as no change in the
interval, as they do in the gate. `--require-ci-above-zero` (or `[improve] require_ci_above_zero`) makes the lower end of the
interval a gate condition, for suites large enough to clear it; the decision is then `rejected: below confidence`.

## Cost

`--max-cost` is required and covers measured eval cost plus optimizer-reported cost. The pre-run estimate (shown by
`--dry-run`) must not exceed it. During the run the loop stops when spend reaches it (`stopped: over budget`) and
keeps the best accepted candidate. An eval runner that reports no cost is charged the whole remaining budget, as
`eval run` does.

## Security

The optimizer is untrusted code running as you. It starts without a shell, in a scrubbed environment (`PATH`, locale, `TZ`
and names from `--env-pass`; `HOME` and the temp directories point into the run directory), with a timeout that
kills the process tree and capped output. A credential-like or proxy name in `--env-pass` is refused unless
`--egress HOST` declares where data goes; the declaration is printed in the consent summary and recorded in the
report, and is **not enforced** by ai-rulez itself.

**Optional sandbox.** `--isolation auto|require` (or `[improve] isolation`) wraps the optimizer in the process sandbox
of `internal/sandbox`: macOS `sandbox-exec`, Linux bubblewrap (or `unshare`, network only). Writes are limited to the
run's workspace, home and tmp directories, and the network is denied unless `--egress` declares a host. `require`
refuses to run when no backend can confine the process (`AR9J7`); `auto` confines when it can and says so when it cannot;
the default `none` keeps the optimizer unconfined, so existing optimizers keep working. The report records the mode, the
backend and what the backend enforces, and the consent summary states the same before anything runs: `unshare` (Linux
without bubblewrap) confines the network only, and `--egress` lifts the network denial. Reads are not restricted on any
backend: the sandbox does not hide the repository, the held-out cases or the per-user report key. `improve pr` takes
the same `--isolation` (and `[improve] isolation`) for the commands it runs in the worktree: see [Pull request](#pull-request).

The held-out gate guards against an honest-but-overfitting optimizer, not a hostile one. The held-out cases are
files in the authored skill's `evals/` directory, readable by absolute path by a process running as you, and the
optimizer is such a process. A hostile optimizer can read them, forge a run, or read the per-user key described
below. The key is a per-user file outside the repository, but it is readable by that same process, and `improve apply`
and `improve pr` re-check the diff policy against the live skill, not the held-out gate: a run forged by an optimizer
that read the key looks signed. Isolation `none` does not stop this; a sandbox does not either, because reads stay open.
Run `improve run` in a container (or CI job) with no access to the repository checkout beyond the run
workspace and an egress policy if the optimizer is not code you trust.

**A repository must not choose what runs.** `[improve] optimizer` and `env_pass` in a repository config (including
`config.local.toml`, which lives in the checkout) are used only with `--trust-repo-optimizer`; otherwise they are
reported and ignored (`AR9J6`). The same keys in the user config file are used without it.

**A repository may tighten the gate, not weaken it.** A repository `[improve]` table is honoured for `min_gain`,
`max_regressions`, `holdout_fraction`, `max_skill_growth`, `runs`, `max_rounds` and `max_holdout_evals` only when the
value is at least as strict as the default (gain >= 0.05, 0 regressions, held-out share >= 0.3, growth <= 1.25,
runs >= 3, rounds <= 3, held-out evaluations <= 3); `require_ci_above_zero = true` and a higher
`min_holdout_cases` always apply. A looser value is dropped with an `AR9J6` warning unless `--trust-repo-optimizer` is
given. `validate` reports the repository keys `improve run` will not honour (`AR9J6`, a warning). The user config file may set any value, and so may a flag. The consent summary prints the effective gate:
minimum gain, regressions, held-out floor and share, growth limit and the interval requirement.

What leaves the machine: the skill text and the train cases, to whatever the optimizer calls. The consent summary
says so and needs `--yes` or an interactive confirmation. Held-out cases never leave ai-rulez.

## `[improve]` configuration

Defaults of `improve run`; a flag always wins, the user config wins over the repository config.

```toml
[improve]
holdout_tag = "holdout"
holdout_fraction = 0.3
min_holdout_cases = 3        # never below 3; the split and the baseline need this many scored held-out cases
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

## Output, show and clean

A run lives under `.ai-rulez/local/improve/<run-id>/` (machine-local, git-ignored): `plan.json`, `original/`,
`workspace/`, `train/`, `rounds/<n>/candidate/`, `report.json` ([`improve-report/1`](schema.md)) and, when accepted,
`diff.patch` (applies with `git apply`). `report.json` records the split by case id, every round's decision and
reasons, before/after scores with the bootstrap interval, the sibling guard, wins and losses, costs, declared egress,
the environment variable names (not values), the isolation, the adapter and the description change.

A run that stops on an error after measuring (a crashing eval runner, say) still writes its signed `report.json` with the
rounds and the spend so far and the reason `stopped: ...`, then exits 1.

`improve show <run-id>` prints the report and the diff (`--format json` prints `improve-show/1`). The optimizer's
text and the candidate's text (violation paths and details, refusal messages, round reasons, file names) are printed with control
characters and bidi controls replaced, so a candidate cannot write terminal escapes, and a candidate file whose name holds
such characters breaks the diff policy; `apply` does the same for its diff. `improve clean <run-id>` or `--all` deletes runs
(`--dry-run`; `--all` asks unless `--yes`); a symlinked run or improve directory is unlinked or refused, never followed.

Before each optimizer call the run directory outside `workspace/`, `home/` and `tmp/` (plan, original copy, train
cases, earlier rounds) is snapshotted. If the optimizer changed any of it, or the authored skill, the round is
rejected (`outside-workspace`) and everything is restored from the snapshot. A run id is `imp-<8 hex>` with an
optional `-<n>` suffix.

## Apply

`improve apply <run-id>` shows the diff, asks for confirmation (`--yes` skips it) and writes the files. It refuses with
`AR9J1` if the skill changed since the run (so a second apply fails), if the saved candidate no longer matches its
digest, or if the candidate now breaks the diff policy. It never commits. With `--format json`, stdout holds the
result document and the diff and prompts go to stderr.

`improve run` signs `report.json` with an HMAC under the per-user key (`eval-results.key` in the user config
directory, outside the repository, shared with `eval run`) and stores it as `report.mac`. `apply` and `pr` refuse a run
whose report is missing the MAC, was edited, or was made on another machine or by another user. The diff policy is
recomputed from the defaults and the flags, never taken from the report: edits to `scripts/` and `assets/` need
`--allow-scripts`, and changes to `allowed-tools`, `model` or `disable-model-invocation` need `--allow-frontmatter`,
even if the run itself used them. Files are written all or nothing.

## Pull request

`improve pr <run-id> [--base BRANCH]` makes the change reviewable without touching your checkout:

1. Creates a linked git worktree (in a temporary directory) on a new branch `ai-rulez/improve/<skill>-<digest8>` started
   from `--base` (default: the branch you are on). A detached HEAD needs `--base`. An existing branch of that name is refused.
2. Checks that the skill at the base has the digest the run measured (`AR9J1` otherwise), that no component of the
   path to it is a symlink, re-checks the diff policy (with the growth factor the run used) and writes the candidate there.
3. Runs `ai-rulez generate --yes` in the worktree, so the outputs the lock pins are not stale; when git tracks any file of
   the generate manifest (the project commits its outputs), all of them are staged with the skill. A project that
   ignores its outputs gets none. A failing generate stops the pull request when the project has a lock and only warns
   without one. Then it runs `ai-rulez lock` when the project has a lock. These commands run with a scrubbed
   environment (the base variables and `XDG_*`/`AI_RULEZ_HOME`; credentials only by name with `--env-pass`, which an
   eval runner needs). With `--run-evals` it also runs
   `ai-rulez eval run <skill> --changed-only` (plus any `--eval-arg`) so `AR997` holds; that calls the eval runner and
   spends money, so it is off by default and the command says what to run on the branch instead. A failing step stops
   the pull request and leaves no branch.
4. Stages the skill, the lock and the eval results, and makes one commit (`chore(skills): improve <skill> ...`; git
   hooks are skipped). The worktree is removed; the branch stays.
5. When the remote exists, `gh` is on `PATH`, the base is a branch the remote has with no unpushed commits (a commit id,
   an unpushed branch or a base ahead of the remote would break or pollute the pull request), and you confirm (or pass
   `--yes`), pushes with `git push --set-upstream` and runs
   `gh pr create [--repo OWNER/REPO] --base B --head BRANCH --title T --body-file F [--draft]`; `--repo` names the
   repository the remote URL points at, so a fork receives the pull request on the fork. Otherwise (no remote, no `gh`,
   `--no-push`, not confirmed, an unusable base) it prints the two commands. The body file is written before the
   worktree exists, so a write failure leaves no branch. improve pr itself makes no network call, but the `generate` and `lock` it runs in the worktree fetch remote includes and sources as they do anywhere (and `eval run` calls the eval runner); git and `gh` use their
   own credentials, and `gh` gets a scrubbed environment plus the usual `GH_*`/proxy names.

**Isolation.** `improve pr --isolation auto|require` (or `[improve] isolation`) runs `generate`, `lock` and `eval run` in the
worktree under the same process sandbox: writable only below the worktree's project directory, ai-rulez's own
subdirectories of the user cache (`~/.cache/ai-rulez`, `$XDG_CACHE_HOME/ai-rulez`) and data (`$XDG_DATA_HOME/ai-rulez`)
roots, `AI_RULEZ_HOME` and the temp directory, where they exist (the two subdirectories are created), with the network
left on because `generate` and `lock` fetch remote includes and `eval run` calls a model. The candidate is the
optimizer's text, and `eval run` lets an agent act on it, so a hostile candidate cannot write outside those
directories. `require` refuses (`AR9J7`) before the worktree is made when no backend works; `auto` warns and runs
unconfined. The result (`isolation` in `--format json`) and the first line printed name the backend and what it enforces.
The version control and `gh` calls are never confined: they need your credentials. A harness that keeps state elsewhere
(`~/.claude`, say) cannot run `--run-evals` confined; use `--isolation none` for that step.

The body states what changed, the held-out table with the interval and the wins and losses, the guards that held, cost,
egress, the environment names forwarded, the description before and after, the optimizer's own words (untrusted: inside
code fences or code spans longer than anything they contain), a reviewer checklist, and that the change is **not
approved**. Nothing here sets approval. Refusals carry `AR9J8`.

## Scheduled repair workflow

A model upgrade can make a skill that passed its evals fail. `ai-rulez improve adapters repair-workflow` prints a
GitHub Actions workflow (save it as `.github/workflows/ai-rulez-repair.yml`) that does the loop on a schedule:

1. `eval run --model $EVAL_MODEL --max-cost $EVAL_BUDGET --format json --no-write` finds the skills that now fail
   (`jq` over `skills[]` with `passing: false`); exit status 1 (could not run) fails the job.
2. For each one, `improve run <skill> --adapter review-fix --sibling-native --max-cost $REPAIR_BUDGET --yes` repairs it
   under the usual gate: held-out set, sibling guards, diff policy, cost ceiling.
3. A run that exits 0 gets `improve pr <run-id> --draft --yes`: a draft pull request from an isolated worktree. A run
   with no acceptable candidate (exit 2) opens nothing.

Nothing is merged, approved or applied for you, and the pull request body says the change is not approved. The template
has no `pull_request` trigger, so a fork's pull request never reaches its secrets; its permissions are `contents: write`
and `pull-requests: write` for the branch and the pull request. It spends money in three places (the eval pass, each
repair, the optimizer's model), each with its own ceiling in the `env` block; the `builtin:review-fix` step also needs
the `[llm]` user config the template writes (delete that step and use `--with` for an optimizer of your own). The run
state lives in `.ai-rulez/local/` of the runner, and `improve pr` needs the report MAC of the machine that made the
run, so both commands must run in the same job, as they do here. A test keeps every flag the template passes in sync
with the CLI.

## Codes

| Code | Name | Meaning |
|---|---|---|
| `AR9J1` | `improve-run-stale` | the skill changed since the run (or differs at the base of `pr`) |
| `AR9J2` | `improve-no-holdout` | too few held-out cases |
| `AR9J3` | `improve-policy-violation` | a candidate round broke the diff policy |
| `AR9J4` | `improve-sibling-regression` | a candidate lowered another skill's trigger recall |
| `AR9J5` | `improve-underpowered` | the held-out gain cannot be told from zero |
| `AR9J6` | `improve-repo-optimizer-ignored` | a repository `[improve]` optimizer or `env_pass` was not used |
| `AR9J7` | `improve-isolation-unavailable` | the requested isolation could not be applied |
| `AR9J8` | `improve-pr-refused` | `improve pr` refused |
| `AR9J9` | `improve-adapter-refused` | a bundled adapter could not run |

They appear in `improve` output and the report only; `validate` never emits them. See [strict validation](strict-validation.md).

## Design decisions

- **The gate is the product.** The optimizer wins only by improving a score it never saw, without regressions, within budget.
- **Gate on paired no-regression, report intervals.** Suites of 5-20 cases cannot clear a significance test, so teams would
  switch the gate off. The interval is reported and flagged; `--require-ci-above-zero` is opt-in.
- **Baseline re-measured every run**, never read from `eval-results.json`, to avoid comparing across model versions.
- **No auto-merge, no auto-commit.** The gate reduces review effort; it does not replace it. `improve pr` commits only on a
  new branch of a throwaway worktree and states the change is not approved.
- **Run state is machine-local.** Nothing durable is written to the project until `apply` or a pull request.
- **Subcommand `run`.** `improve run <skill>` keeps `improve` a pure command group; a skill named `apply` cannot collide with `improve apply`.
- **Held-out marking is the `holdout` tag** (design question 1), which works today; a `split:` field can replace it later.
- **`--env-pass` has a config equivalent only from trusted scope** (question 2): `[improve] env_pass` in the user config,
  or in a repository config with `--trust-repo-optimizer`.
- **`improve pr` depends on the `gh` CLI** (question 3), not on a GitHub client of ai-rulez: less code and no extra egress to audit.
- **No research optimizer is documented as supported** (question 4) until someone verifies its interface; the `research`
  recipe says so.
- **The sibling guard is mandatory when a sibling has trigger cases** (question 5): it is free, because it uses the offline ranker.
- **Self-generated held-out cases** (question 6) are not detected; the report cannot tell who wrote a case. Treat a
  model-written held-out set with the scepticism the design notes call for.
- **The sandbox is opt-in** (`--isolation`, on `run` and on `pr`): turning it on by default would break optimizers that
  write caches elsewhere or reach a local model server without declaring egress, and harnesses that keep state outside
  the worktree.
- **The native sibling guard is opt-in** (`--sibling-native`): it needs a runner with the activation capability and
  costs money, so the free offline guard stays the default.
- **Not done:** the native guard for harnesses other than `claude` without a `--runner-command`.

The accepted candidate is the best of the rounds that were scored on the held-out set, so with more than one such round
the gain is optimistic. `improve apply` and the pull request body say "selected among N held-out evaluations" then;
`max_holdout_evals` bounds N.
