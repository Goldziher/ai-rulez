# Improve (experimental)

`ai-rulez improve run <skill> --with CMD` runs an external optimizer (a tool that rewrites a skill against a
benchmark) on a throwaway copy of the skill and accepts a candidate only if a held-out eval set improves. ai-rulez
implements no optimizer. It owns the parts that make one safe to use on real sources: a disposable workspace,
a withheld held-out set, a strict acceptance gate, a diff policy, a cost ceiling and a human-reviewed apply step.

> **Experimental.** Flags, the optimizer protocol and `report.json` may change before this is stable. The command
> prints a warning on every run. Phase 1 only: the sibling trigger guard, bootstrap confidence intervals, `show`,
> `clean`, `improve pr` and bundled adapters are not implemented.

## Quick start

```bash
# 1. tag at least three eval cases `holdout` (one negative), keep others as training cases
# 2. see the plan and the estimate; nothing runs
ai-rulez improve run deploy --with 'python optimize.py' --max-cost 5 --dry-run
# 3. run it
ai-rulez improve run deploy --with 'python optimize.py' --max-cost 5 --yes
# 4. review .ai-rulez/local/improve/<run-id>/diff.patch and report.json, then
ai-rulez improve apply <run-id>
ai-rulez lock && ai-rulez eval run deploy && ai-rulez validate --strict
```

Exit status: 0 candidate accepted, 2 no acceptable candidate (report written), 1 refused or could not run.

## Preconditions

A run is refused, with the reason, unless:

1. The skill is authored (under `skills/` or a domain), not from an include, an installed skill or a built-in.
2. Eval cases exist (`AR962`) and parse (`AR996`).
3. At least three scored held-out cases exist, one of them negative (`expect_trigger: false` or a near miss) (`AR9J2`).
4. The skill and its evals have no uncommitted changes (checked when the project is a git repository; otherwise a warning).
5. `--max-cost` is set and the eval estimate does not exceed it.
6. The secret scan is clean for the skill files and the train cases the optimizer will receive.

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
  "history": ["rejected: below gain"] }
```

```json
{ "version": 1, "summary": "Tightened the description.", "changed": ["SKILL.md"], "cost_usd": 0.42, "notes": "optional" }
```

`changed` is informational: ai-rulez computes the real diff. `cost_usd` is self-reported and counts against
`--max-cost`; a run whose optimizer never reports a cost is flagged. `summary` and `notes` are untrusted: control
characters are removed and they are never fed to a model. A crash, timeout, invalid JSON or wrong version rejects
that round only.

## Diff policy

Checked after every round, before any eval spend. A violation is `AR9J3` and rejects the round (the workspace is
restored to the last good state):

- a change outside `editable` (default `SKILL.md`, `references/**`), including new, deleted or forbidden files;
- a symlink, hard link, special or oversized file; an executable-bit change;
- a change to `name`, `allowed-tools`, `disable-model-invocation` or `model` (`--allow-frontmatter` frees all but `name`);
- a new reference to `scripts/` (`--allow-scripts` frees `scripts/**` and `assets/**`);
- `SKILL.md` above 1.25 times the original token count;
- a new security finding (`AR0xx`) in a changed file; findings already in the original never block;
- a write outside the skill directory: the workspace root, the read-only original, the train cases or the authored skill.

A held-out assertion string that appears in the candidate but not in the original is reported as a warning (possible leak).

## Acceptance gate

Baseline and candidate are both measured in the run on the held-out set, `--runs` times each, with the majority
outcome per case. A candidate is accepted only if all hold:

- at least one held-out case was scored by both arms, and the candidate was scored on every case the baseline was
  (a case the candidate could not be scored on counts as a loss, never as a pass; a gain of 0 with `--min-gain 0`
  is not evidence);
- held-out pass rate gain >= `--min-gain`;
- held-out cases flipping pass to fail <= `--max-regressions`;
- trigger precision and recall do not drop, and near-miss false positives do not rise;
- the diff policy held.

Cases that are not unanimous across runs are `unstable`: shown, but never counted as wins or losses. A baseline that
already passes everything exits 2 before the optimizer runs. `--max-holdout-evals` bounds how often the held-out
set is consumed. With fewer than 8 held-out cases the report says `underpowered`. Each round continues from the
optimizer's own state; the best accepted round (highest gain, earliest on ties) is kept, or the first with
`--stop-at-first-accept`.

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
report, and is **not enforced**. ai-rulez cannot sandbox file system or network access, and the run directory sits
inside the project, so use a container or CI egress policy for anything beyond local experiments.

The held-out gate guards against an honest-but-overfitting optimizer, not a hostile one. The held-out cases are
files in the authored skill's `evals/` directory, readable by absolute path by a process running as you, and the
optimizer is such a process. A hostile optimizer can read them, forge a run, or read the per-user key described
below. Run `improve run` in a container (or CI job) with no access to the repository checkout beyond the run
workspace and an egress policy if the optimizer is not code you trust.

What leaves the machine: the skill text and the train cases, to whatever the optimizer calls. The consent summary
says so and needs `--yes` or an interactive confirmation. Held-out cases never leave ai-rulez.

## Output and apply

A run lives under `.ai-rulez/local/improve/<run-id>/` (machine-local, git-ignored): `plan.json`, `original/`,
`workspace/`, `train/`, `rounds/<n>/candidate/`, `report.json` ([`improve-report/1`](schema.md)) and, when accepted,
`diff.patch` (applies with `git apply`). `report.json` records the split by case id, every round's decision and
reasons, before/after scores, wins and losses, costs, declared egress, the environment variable names (not values)
and the description change.

Before each optimizer call the run directory outside `workspace/`, `home/` and `tmp/` (plan, original copy, train
cases, earlier rounds) is snapshotted. If the optimizer changed any of it, or the authored skill, the round is
rejected (`outside-workspace`) and everything is restored from the snapshot. A run id is `imp-<8 hex>` with an
optional `-<n>` suffix. `apply` takes `--format text|json`; with `json`, stdout holds the result document and the
diff and prompts go to stderr.

`improve apply <run-id>` shows the diff, asks for confirmation (`--yes` skips it) and writes the files. It refuses with
`AR9J1` if the skill changed since the run (so a second apply fails), if the saved candidate no longer matches its
digest, or if the candidate now breaks the diff policy. It never commits.

`improve run` signs `report.json` with an HMAC under the per-user key (`eval-results.key` in the user config
directory, outside the repository, shared with `eval run`) and stores it as `report.mac`. `apply` refuses a run
whose report is missing the MAC, was edited, or was made on another machine or by another user. The diff policy is
recomputed on apply from the defaults and the flags, never taken from the report: edits to `scripts/` and
`assets/` need `--allow-scripts`, and changes to `allowed-tools`, `model` or `disable-model-invocation` need
`--allow-frontmatter`, even if the run itself used them. Files are written all or nothing: if a write fails, the
files already changed are restored.

## Codes

| Code | Name | Meaning |
|---|---|---|
| `AR9J1` | `improve-run-stale` | the skill changed since the run |
| `AR9J2` | `improve-no-holdout` | too few held-out cases |
| `AR9J3` | `improve-policy-violation` | a candidate round broke the diff policy |

They appear in `improve` output and the report only; `validate` never emits them. See [strict validation](strict-validation.md).

## Design decisions

- **The gate is the product.** The optimizer wins only by improving a score it never saw, without regressions, within budget.
- **Gate on paired no-regression, report intervals later.** Suites of 5-20 cases cannot clear a significance test, so teams would switch the gate off.
- **Baseline re-measured every run**, never read from `eval-results.json`, to avoid comparing across model versions.
- **No auto-merge, no auto-commit.** The gate reduces review effort; it does not replace it.
- **Run state is machine-local.** Nothing durable is written to the project until `apply`.
- **Subcommand `run`.** `improve run <skill>` keeps `improve` a pure command group; a skill named `apply` cannot collide with `improve apply`.
- **Deferred:** sibling trigger guard (needs activation mode), bootstrap CI, `show`, `clean`, `improve pr`, bundled adapters, `[improve]` config keys.
