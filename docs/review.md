# Review

`ai-rulez review` scores skills, agents and commands against a rubric you own. Without flags it is **offline**: the
score comes from the lint findings each rubric dimension names, no model is called, and nothing leaves the machine.
With `--semantic` it adds an **LLM judge** for what lint cannot see: a vague or overlapping trigger, a body that
contradicts its description, injection intent, scope creep. The judge is advisory, cached, bounded by spend caps, and
may gate a build only after `review calibrate` measured it against a labelled golden set. `review fix` proposes a
patch for its findings and never writes without `--apply`. Design: [#220](https://github.com/Goldziher/ai-rulez/issues/220),
follow-up [#269](https://github.com/Goldziher/ai-rulez/issues/269).

- [Quick start](#quick-start)
- [How the offline score is built](#how-the-offline-score-is-built)
- [Pre-filters](#pre-filters)
- [Estimate and the egress manifest](#estimate-and-the-egress-manifest)
- [The judge: `--semantic`](#the-judge---semantic)
- [Calibration and the gate](#calibration-and-the-gate)
- [Drift and model comparison](#drift-and-model-comparison)
- [`review fix`](#review-fix)
- [Selecting items](#selecting-items)
- [Baselines](#baselines)
- [Rubrics](#rubrics)
- [Configuration](#configuration)
- [Output and exit codes](#output-and-exit-codes)
- [Design decisions](#design-decisions)

## Quick start

```console
ai-rulez review                          # offline score of every skill, agent and command
ai-rulez review skill:deploy             # one item: an id, a name or a path
ai-rulez review --estimate               # egress manifest and cost range of a judged run; sends nothing
ai-rulez review --semantic               # add the judge (needs [llm] allow_network = true in user scope)
ai-rulez review --semantic --content full --k 3 --max-cost 0.25
ai-rulez review calibrate --golden tests/review/golden/skill-quality
ai-rulez review --semantic --gate        # exit 2 on a stable fail of a calibrated dimension
ai-rulez review fix skill:deploy --model gemini-2.5-flash   # print a patch; --apply writes it
ai-rulez review explain AR9G2            # what a code means, plus the rubric's definitions
ai-rulez rubric lint                     # check .ai-rulez/rubrics/*
```

```text
rubric skill-quality@2 (builtin:skill-quality, sha256:15e8abaa)  model gemini/gemini-2.5-flash-lite (resolved gemini/gemini-2.5-flash-lite)  votes<=3  content full  advisory
skill:deploy                 skill    score 100/100  semantic 45/100
  AR9G1  trigger-quality      fail - The description does not name a concrete trigger or when not to use the skill.  [judge, agree 2/2]
  AR9G2  overlap              fail "Update docs when CLI behavior changes." - overlaps docs-and-site.  [judge, unstable agree 2/3]
skill:leak                   skill    withheld: AR001 secret-detected: never sent to a judge
3 items: 2 scored, 1 withheld, 0 excluded, 0 skipped; mean score 100; mean semantic score 45 over 2 judged
2 items judged, 0 answers cached; calls 9, tokens 11204, cost $0.0113 (cap $0.5)
calibration: missing
  no calibration record: run `ai-rulez review calibrate`
```

## How the offline score is built

Each rubric dimension lists `twins`: the lint rules that already say something about it (for example
`trigger-quality` has `AR801`-`AR803`). For every item the dimension gets a verdict from the findings on that item
(and, for a skill or command, on its resource files):

| Twin finding | Verdict | Value |
| --- | --- | --- |
| none, or only `info` | `pass` | 1 |
| a warning | `warn` | 0.5 |
| an error | `fail` | 0 |

`score = round(100 * sum(weight_i * value_i) / sum(weight_i))` over the dimensions that were scored. A dimension
with no twin (the built-in `instruction-conflict`) is listed as `not-scored` and left out of both sums. The weights
and the formula are printed in `--format json` under `rubric`.

Read the offline score as a floor from deterministic checks, not as a judgement: `pass` means no lint rule fired, not
that a person would call the description good. A finding of a dimension below `pass` is reported with the dimension's
code (`AR9G1`-`AR9G7`, see [Rule codes](#rule-codes)), at the dimension's severity ceiling (`warning` or `info`, never
`error`). Every finding is advisory and carries a fingerprint that survives line moves.

## Pre-filters

Run before anything is planned or sent, and they decide what is ever eligible to leave the machine:

- An item with a secret or hidden-character finding (`AR001`, `AR002`) on its file or its skill resources is
  **withheld**: no score, no manifest entry, an `AR9G0` note. The decision does not rest on the lint findings alone:
  review scans the item's own text for a credential or a hidden character directly, so `[lint] ignore`,
  `[lint.severity]`, `ignore_paths` and an inline `ai-rulez-lint-ignore` cannot make an item eligible to leave the
  machine. With `[review] on_secret = "redact"` an item whose **own text** holds a credential is sent with every
  credential replaced by `[REDACTED:AR001]` and marked `redacted`; it is still withheld when the masked text still
  looks like it holds one, when the credential is in a resource file, or when it has a hidden character (never
  redacted).
- A twin **error** pre-empts the dimension: the judge is not asked, and the dimension keeps the lint verdict.
- `[review] exclude` globs (matched against the name, the id and the path) exclude an item.
- Content from includes, installed skills, skill sources and builtins is skipped unless `--include-imports`. It is
  never read when skipped.
- An item the rubric does not apply to (`applies_to`) or that cannot be read is skipped with the reason.

Nothing is dropped silently: withheld, excluded and skipped items are listed with their reason.

## Estimate and the egress manifest

`--estimate` prints a manifest of what a judged run **would** send and sends nothing (`"sent": false`). It works
offline and is the recommended first step.

```text
egress manifest (estimate; nothing is sent by --estimate)
  model claude-haiku-4-5  host gateway.internal  network allowed: false  content: descriptions
  skill:deploy                 2 call(s)   2735 bytes  sha256 dd57c18f4bb0
  withheld (never sent): skill:leak
  2 items, 4-12 calls, ~2167-6501 input tokens, up to 8400 output tokens
  cost $0.0102 to $0.0485 (cap $0.50, 300 calls)
```

- **What**: per item, the planned calls (an `intrinsic` call with the item, a `contextual` call with the item's
  description and up to `max_siblings` sibling descriptions), their bytes, and the SHA-256 of the data block. Never
  the content. `--content descriptions` (default) sends the name, description and frontmatter keys; `--content full`
  adds the frontmatter and the body, head and tail truncated at the rubric's `max_item_tokens`.
- **Where**: the host of the resolved `[llm]` base URL (`(provider default)` when unset) and whether
  `allow_network` is on. `[llm]` follows the [trust rule](llm.md#trust-rule): a repository `base_url` is ignored and
  reported as ignored.
- **How much**: tokens are estimated at one per three bytes (conservative); cost uses the model's price from the
  built-in price table or `[llm] price_input_per_mtok`/`price_output_per_mtok`. The lower bound is one vote per call
  and a 400-token answer; the upper bound is `votes.max` votes per call at `max_output_tokens`.
- **Caps**: `--max-cost` / `--max-calls`, else `[review] max_cost_usd` / `max_calls`, else 0.50 USD and 300 calls
  (`--max-cost 0` means unlimited). The estimate is **refused** (exit 1, listed under `refused`) when even the lower
  bound exceeds a cap, or when the model has no price and a cost cap is set. An organization policy that forbids
  model calls refuses the estimate too.
- `--show-prompt` prints the exact planned messages. The nonce of the data fence is shown as
  `<nonce:000000000000000000000000>` so the plan and its hash are reproducible; a real run derives a 96-bit one.

## The judge: `--semantic`

```console
ai-rulez review --semantic [--content descriptions|full] [--k N] [--model M | --models A,B]
                           [--max-cost USD] [--max-calls N] [--no-cache] [--cache-dir DIR]
                           [--gate [--gate-level info|warning|error]] [--baseline F] [--write-baseline F]
```

Both opt-ins must hold: `--semantic` on the command line and `[llm] allow_network = true` in **user scope** (the user
config file or `AI_RULEZ_LLM_ALLOW_NETWORK`; a repository config cannot switch the network on, see the
[trust rule](llm.md#trust-rule)). A model must be configured (`[llm] model` or `--model`). Nothing is sent before
every check passed, including the estimate against the spend caps.

**What the judge is asked.** At most two calls per item: an `intrinsic` call (the item alone) and a `contextual`
call (the item's description plus up to `max_siblings` sibling descriptions, chosen by the same word-set similarity
as `AR702`). The messages are a fixed system prompt (the rubric's `system.md`, else the built-in one) and one user
message holding the rubric's dimension definitions and the item inside a fence whose delimiter carries a nonce:

```text
<<<DATA-7f3a91c2e0b14d65a8c3f9d1 item=skill:deploy>>>
name: deploy
description: Helps with deployments
...
<<<END-DATA-7f3a91c2e0b14d65a8c3f9d1>>>
```

The nonce is derived from the request, so the author of the item cannot predict or close it and the response cache
stays effective. The model is told the fenced text is data, never instructions, and that text addressed to the
reviewer is itself a finding. No tools or function calling are exposed to it. The reply is requested as JSON through a
schema (no `additionalProperties`: Gemini rejects it) and decoded **strictly**: an unknown field, data after the
object, a missing or repeated dimension, or a verdict other than `pass`, `warn` or `fail` makes the reply invalid. An
invalid reply is retried once; then the dimension is `error`, never `pass`.

**Content modes.** `descriptions` (default) sends the name, description and frontmatter keys. A dimension that can
only be answered from the body (`needs_body = true`: `body-accuracy`, `injection-intent`, `scope-creep`,
`instruction-conflict`, `body-structure` in the built-in rubric) is then **not judged** and is reported `skipped`
("needs --content full") instead of being guessed from a description. `full` adds the frontmatter and the body (and a
1.5 KB excerpt of each sibling body for the contextual call). A truncated body caps that item's verdicts at `info`.

**Evidence.** Every `quote` a verdict cites must appear verbatim (whitespace-normalised) in what was sent. A quote
that does not is dropped and counted (`hallucinated_evidence` in the report); a `warn` or `fail` left with no valid
quote is dropped and reported as a `pass`, unless the dimension has `allow_absence` (the problem is that something is
missing). Model-reported confidence is not requested: it is poorly calibrated. Confidence is the vote agreement.

**Votes.** The first vote runs at `first_temperature`. If it passes a dimension, that is accepted (precision over
recall). A flagged dimension gets up to `--k` votes (default the rubric's `votes.max`, 3) at `extra_temperature`, each
with the dimensions reshuffled and the siblings reversed (vote 2) or shuffled (later votes) from a seed derived from
the item, so a re-run reproduces every vote and each vote is cached on its own. Voting stops early once two votes agree
on every flagged dimension. The verdict is the **median** (`pass < warn < fail`, a tie goes to the lower level);
`agreement` is the share of votes equal to it. Below `instability_threshold` (0.67, so `2/3` is below it) the dimension
is `unstable`: it is reported at `info`, counts as a pass in the score and never gates.

**Score.** `semantic_score` uses the same formula over the dimensions the judge answered. Pre-empted, skipped and
errored dimensions are left out of both sums.

**Cache and caps.** Answers are cached by the model layer ([LLM access](llm.md)): endpoint identity, model, the whole
request and `PromptVersion` (`review/<rubric id>@<version>+<8 hex of the prompt digest>`), so an edit to the rubric,
`system.md` or the template invalidates it even without a version bump. A cache hit costs no budget and no network, so
an unchanged project costs nothing on re-run. `--max-cost` and `--max-calls` are enforced by the budget before every
call (worst case reserved) and the run **stops** when one is reached: the report is `incomplete`, lists the items it
did not judge, and exits 0 (1 with `--gate`, because it cannot vouch). A repository `[review]` cap can only **lower**
the default (a hostile repository must not raise what a run may spend); the user config file, the environment and the
flags can set any value.

**Where it goes.** The only egress is the model layer. A user-scope `[review] allowed_hosts` (or
`AI_RULEZ_REVIEW_ALLOWED_HOSTS`) restricts the hosts a judged run may send to (`provider-default` names the provider's
own endpoint); a run to another host is refused before anything is sent. A repository `allowed_hosts` is ignored and
reported. See [Data egress](llm.md#data-egress-and-residency).

**Findings.** One AR code per dimension (`AR9G1`-`AR9G7`), at the dimension's severity ceiling, `advisory: true`,
`origin: "llm-judge"`, with the first quote, the rationale, the suggestion, the vote agreement and a fingerprint of
`code | item | dimension | normalised first quote`. SARIF results carry the same.

## Calibration and the gate

The judge may fail a build only after it was **measured**. `review calibrate` judges every case of a golden set the
way `--semantic` does (all `k` votes are asked, so consistency can be measured) and compares the verdicts with the
adjudicated human labels.

```console
ai-rulez review calibrate --golden tests/review/golden/skill-quality --model gemini-2.5-flash-lite
ai-rulez review calibrate --models gemini-2.5-flash-lite,gemini-2.5-flash   # compare, writes nothing
ai-rulez review calibrate --compare .ai-rulez/calibration/skill-quality.builtin.json   # drift check
```

A golden case (`golden/*.golden.yaml`, paths relative to the golden directory) is labelled by at least two labelers and
adjudicated:

```yaml
schema_version: 1
id: vague-trigger-01
kind: skill
item: { path: fixtures/vague-trigger-01/SKILL.md }
siblings: [fixtures/shared/sibling-a/SKILL.md]
labelers:
  - { id: a, labels: { trigger-quality: fail, overlap: pass } }
  - { id: b, labels: { trigger-quality: warn, overlap: pass } }
adjudicated: { trigger-quality: fail, overlap: pass }
probes: [pad, reorder, rename, canary]
```

Per dimension the record holds: quadratic-weighted **kappa** against the adjudicated labels; **precision**, **recall**
and F1 of "flagged" (not `pass`) with Wilson 95% intervals; the labelers' own **kappa** (below
`min_human_kappa` the dimension is `ill-defined`: fix the rubric, not the model); the judge's **consistency** across the
`k` votes (share of cases with identical votes, and Fleiss' kappa); the **metamorphic probes**: `pad` (neutral filler
appended; the verdict must not improve: verbosity bias), `reorder` (siblings reversed; the verdict must not change:
position bias), `rename` (the item renamed; the verdict must not change: name priors) and `canary` (text addressed to
the reviewer; `injection-intent` must flag it and no verdict may improve); and the **calibration curve** (vote
agreement against measured precision). A dimension with fewer than 6 labelled cases is `uncalibrated`.
The record `status` is `pass` only when no calibrated dimension is `fail` or `ill-defined`, at least one passes, and the
set has `golden_min_items` cases. The thresholds are the rubric's `[calibration]` table
(`min_weighted_kappa`, `min_consistency`, `min_human_kappa`, `min_recall.<dimension>`, `min_precision`, `min_probe`,
`max_age_days`).

The record is `calibration.json` in the rubric directory, or `.ai-rulez/calibration/<id>.builtin.json` for a built-in
rubric. It binds the result to: the **rubric digest** (rubric.toml and system.md, not the record itself), the **prompt
digest** (template, system prompt, reply schema, rubric file), the **golden digest** (cases and fixtures), the
**resolved model id** (what the provider answered with, not the alias asked for) and the content mode and `k`.

`review --semantic --gate` is **refused** (exit 1, nothing spent) unless a record matches all of those, passed, and is
younger than `max_age_days` (`[review.gate] calibration_max_age_days` overrides it). It is also refused for a floating
alias (`...-latest`) and when the model that answers is not the one the record names. It then exits 2 when a **stable**
`fail` verdict exists on a dimension whose calibration passed, at or above `--gate-level` (the lowest severity ceiling
that gates: `warning` by default; a judge never reports above `warning`, so `error` gates no judged verdict). Unstable,
pre-empted, errored, truncated and baselined verdicts never gate; dimensions the record did not calibrate never gate.
`[review.gate] require_calibration = false` lets `--gate` run without a record (every dimension counts and `AR9G9`
still says so).

**`AR9G9 judge-calibration-stale`** (info) is reported by every judged run whose model is a floating alias or answered
under another id, or that has no matching record (missing, stale with the reasons, or failed).

`calibrate` needs the same opt-in as `--semantic` and is bounded by `--max-cost` (default 2.00 USD) and `--max-calls`
(default 1000); a calibration cut short writes nothing. Exit 0 when the judge passes, 2 when it does not or drifted.

## Drift and model comparison

`review calibrate --compare calibration.json` is the scheduled drift job: it calibrates again, compares with the
committed record, prints every golden verdict that changed, and exits 2 when a dimension's kappa fell by more than 0.05,
a dimension stopped passing, the resolved model changed, or the judge no longer meets its thresholds. Replay tests need
no network: the review package's own tests serve recorded replies through the `Fake` backend.

`--models A,B` (on `review --semantic` and on `calibrate`) judges with each model and reports pairwise agreement and
kappa per dimension, and the items the models judge differently, as "review first". The first model is the primary one
(its findings are the report's); the spend caps are split between the models. On `calibrate` it measures each model
against the labels and writes no record.

## `review fix`

```console
ai-rulez review fix [id|name|path...] [--finding FINGERPRINT] [--model FIXER] [--judge-model JUDGE]
                    [--out fix.patch] [--apply] [--allow-same-model] [--patch fix.patch]
```

For each authored item with a stable judged finding, the fixer proposes **exact-text edits** (`old` must occur once in
the file; the unified diff is built from them, not asked of the model). A proposal is kept only when all of this holds,
with up to two attempts (the second sees why the first was rejected), else the item reports "no safe fix":

1. the edits apply exactly and change the file;
2. the frontmatter still parses and **only `description` changed** in it: the `name` and the tool list are untouched;
3. the item grew by at most `[review.fix] max_growth_percent` (default 25%, never less than 200 bytes);
4. the security scan finds nothing new, no credential or hidden character was added, and no link was added;
5. a judge, **a different model from the fixer** unless `--allow-same-model` (self-preference), rates every targeted
   dimension better and no other dimension worse.

Only authored content inside the configuration directory is touched: items from includes, installed skills, builtins,
the machine-local overlay and generated outputs are reported as not fixable. The patch (to `--out` or standard output)
has a header with the item digest, the finding, the models and the rubric version, then a unified diff:

```text
# ai-rulez review fix
# item: skill:deploy
# path: .ai-rulez/skills/deploy/SKILL.md
# digest: sha256:2da78adb...
# finding: 51c0e2... AR9G1 trigger-quality
--- a/.ai-rulez/skills/deploy/SKILL.md
+++ b/.ai-rulez/skills/deploy/SKILL.md
@@ -1,5 +1,5 @@
```

`--apply` writes the verified edits to the item files. It refuses a file whose digest differs from the patch header
(**stale patch**), a file with uncommitted changes in git or outside a git repository, and never touches more than the
item file; it keeps the file mode and replaces the file atomically, then tells you to run `ai-rulez lock` (the lock
pins the changed item; with `[governance]` approvals the change needs a new approval). A second run proposes nothing
(empty patch, exit 0): the fix was rated better and that answer is cached. `--patch FILE` takes a patch written
earlier instead of asking a model: it verifies the digest, the hunks, the checks and the security scan, and `--apply`
writes it. Exit 2 when a finding had no safe fix. The fix settings are `[review.fix]`; the judge model is `--judge-model`,
else `[llm] model`; the fixer is `--model`, else `[review.fix] model`.

## Selecting items

- `[id|name|path...]`: an id such as `skill:deploy` or `skill:backend/deploy`, a name, or a path; a selector that matches
  nothing is an error.
- `--since REV`: only items whose file (or, for a skill or command directory, any file below it) changed since the git
  revision (committed, staged, unstaged and untracked). Siblings are still drawn from every item.
- `--role NAME` / `--profile NAME`: only the content slice `generate --role` or `--profile` renders (`--profile a,b`
  composes). The two are exclusive. A role cannot change the rubric.

## Baselines

`--write-baseline FILE` writes the fingerprints of this run's findings; `--baseline FILE` hides them in a later run
(they are listed in `--format json` as `baselined`, left out of the text and SARIF output, and never gate), so a team
adopts the review without fixing everything first. `--baseline` also accepts an earlier `--format json` report. A
finding whose quote changed has another fingerprint and is new. Drift of the judge itself is
[`calibrate --compare`](#drift-and-model-comparison).

## Rubrics

A rubric is a user-owned, versioned directory, never written into any per-tool output tree (no context cost). It is
pinned in `ai-rulez.lock` as kind `rubric` (id = the directory name; the digest covers **every file** under the
directory: rubric.toml, system.md, the golden cases and fixtures, calibration.json), so a silent edit of a rubric, its
prompt or its labels, and a re-calibration, show in review as lock drift (`AR981`) until `ai-rulez lock` runs. A rubric
directory with a symlink is a lock problem, not a skipped file. See [Lockfile](lockfile.md).

```text
.ai-rulez/rubrics/<id>/
  rubric.toml                 # dimensions, weights, limits, votes, thresholds
  system.md                   # optional system prompt override
  golden/*.golden.yaml        # human-labelled cases for calibration
  fixtures/...                # the items the cases point at
  calibration.json            # written by `review calibrate`
```

`builtin:skill-quality` is embedded in the binary. Use `--rubric <id>` or `[review] rubric`. A rubric that fails
`rubric lint` is refused, so a review never scores against something other than what the file says.

```toml
schema_version = 1
id = "my-rubric"                 # must equal the directory name
version = 1                      # bump on any change that can alter output
applies_to = ["skill", "agent", "command"]   # also: rule

[limits]
max_item_tokens = 6000           # longer bodies are head+tail truncated; verdicts are capped at info
max_siblings = 5
max_output_tokens = 700

[votes]
first_temperature = 0.0
extra_temperature = 0.7
max = 3
instability_threshold = 0.67

[calibration]
golden_min_items = 40
min_weighted_kappa = 0.60
min_consistency = 0.85
min_human_kappa = 0.70
min_precision = 0.70
min_probe = 0.95
max_age_days = 90
[calibration.min_recall]
injection-intent = 0.90

[[dimension]]
id = "trigger-quality"
code = "AR9G1"                   # AR9G1-AR9G7: the finding code of this dimension
group = "intrinsic"              # intrinsic: the item alone; contextual: needs siblings
weight = 0.6                     # weights must sum to 1
severity = "warning"             # ceiling: warning or info, never error
twins = ["AR801", "AR802"]       # lint rules that pre-empt and score this dimension
allow_absence = true             # a missing element needs no quote
needs_body = false               # true: judged only with --content full
question = "Does the description say when to use the skill?"
pass = "Names a concrete trigger and a non-trigger."
warn = "Names the task but the trigger is generic."
fail = "Could apply to almost any request."
```

### `ai-rulez rubric`

| Command | Purpose |
| --- | --- |
| `rubric list` | The built-in and project rubrics with version and digest |
| `rubric show [id]` | Dimensions, weights, twins and the formula |
| `rubric lint [id...]` | Check rubrics (default: every project rubric; `builtin:<id>` is accepted). `--format text\|json\|sarif`. Exit 2 with findings, 1 when unreadable |

`rubric lint` reports `AR9G8` for: a wrong `schema_version`, an `id` that is not the directory name, weights that do
not sum to 1, duplicate dimension ids or codes, a code outside `AR9G1`-`AR9G7`, a twin that is not a registered lint
rule, a `severity` of `error`, thresholds out of range, an unknown key, a golden case with fewer than two labelers,
an unknown dimension or verdict, a path with `..`, an unknown probe, a calibration record for another rubric, and
any symlinked rubric directory or file.

### Rule codes

`AR9G0`-`AR9G9` are allocated ([code ranges](strict-validation.md#code-ranges)). `AR9G1`-`AR9G7` are the built-in
dimensions, `AR9G0` is the run note (an item withheld, excluded, skipped, or not judged because the run stopped),
`AR9G8` is `rubric-invalid`, `AR9G9` is `judge-calibration-stale`. `validate` does not emit them; `review` and
`rubric lint` do. `review explain AR9G2` prints the explanation and the rubric's definitions.

## Configuration

```toml
[review]
rubric = "builtin:skill-quality"   # or a directory name under .ai-rulez/rubrics
content = "descriptions"           # descriptions | full
exclude = ["internal-*"]           # name globs
max_cost_usd = 0.50                # a repository value can only lower the default
max_calls = 300
on_secret = "withhold"             # withhold | redact
allowed_hosts = []                 # honoured only from the user config file

[review.gate]
level = "warning"                  # info | warning | error
require_calibration = true         # false: --gate runs without a matching record
calibration_max_age_days = 90

[review.fix]
model = ""                         # must differ from the judge unless --allow-same-model
max_growth_percent = 25
```

The user config file (`$XDG_CONFIG_HOME/ai-rulez/config.toml`) may also carry `[review]` with `allowed_hosts`,
`max_cost_usd` and `max_calls`; `AI_RULEZ_REVIEW_ALLOWED_HOSTS` (comma-separated) overrides the list. An invalid
`[review]` table is an error. For a judged run to price a model that is not in the built-in table with a cost cap set,
set `[llm] price_input_per_mtok` and `price_output_per_mtok` in user scope.

## Output and exit codes

`--format text` (default), `json` (schema [review-report.schema.json](schema.md), `review-report/1`) or `sarif`.
SARIF results use the dimension codes as rule ids, mark `advisory: true` and `origin: "lint-twin"` or `"llm-judge"`,
and carry `aiRulezReviewFingerprint/v1`. `--out FILE` writes the report to a file. The JSON report adds, for a judged
run: `run` (calls, cached answers, tokens, cost, the cap, `incomplete` and the unjudged items, the requested and
resolved models, `hallucinated_evidence`), `items[].semantic`, `findings[]`, `egress`, `calibration`, `gate`, `models`
and `baseline`.

| Code | Meaning |
| --- | --- |
| 0 | Reviewed. Advisory findings never change it |
| 1 | Could not run or was refused: bad flag or config, invalid rubric, no network opt-in or model, a host outside `allowed_hosts`, an estimate over a cap, `--gate` without a matching calibration, an incomplete run with `--gate` |
| 2 | `--gate` failed; `calibrate` did not pass or drifted; `review fix` had a finding with no safe fix; `rubric lint` found problems |

## Design decisions

- **`review` exists offline.** The deterministic score is useful alone, so it ships; the judge is opt-in on top.
- **Double opt-in and fail closed.** `--semantic` and `allow_network` in user scope; the cost caps are checked against
  the estimate before the first call and enforced per call.
- **Withhold by default, never send a hidden character.** `on_secret = "redact"` is an explicit choice and re-scans the
  masked text.
- **Descriptions are the default content, and body-only dimensions are skipped, not guessed.** A judge that is shown a
  description and asked whether the body is accurate answers `pass` with nothing to cite; reporting that as a result is
  worse than reporting nothing. The cost is that the default run judges two dimensions; `--content full` judges all.
- **Evidence or nothing.** A verdict the item cannot back is dropped and counted, not reported.
- **Gating is earned per rubric and model.** A record binds the result to the exact rubric, prompt, golden set, model id
  and content mode; anything else is stale. `require_calibration = false` is the explicit opt-out.
- **Dimensions are gated individually.** A dimension that did not pass calibration, or that nobody labelled, is advisory
  even when others gate.
- **Repository config cannot raise spend or widen hosts.** Caps can only be lowered by a repository; `allowed_hosts`
  is user scope only.
- **The fixer proposes edits, not a diff.** Exact-text edits are verifiable and a diff is generated from them; models
  write unreliable hunks.
- **The fixer and the judge differ.** A model verifying its own edit is biased toward it; `--allow-same-model` is the
  explicit override.
- **Rubrics from the repository are data.** Never executed; symlinks are refused rather than followed.
- **Lint twins are a code list, not a mapping language.** The rule's own severity decides the verdict.
- **Dimension codes are `AR9G1`-`AR9G7` only.** A custom rubric can reorder, reweight or retarget dimensions but not
  add an eighth code.
- **Price table.** Prices come from the model layer (`internal/llm`); `review` keeps no second copy.
