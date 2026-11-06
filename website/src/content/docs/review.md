---
title: "Review"
description: "ai-rulez review scores your skills, agents and commands against a rubric: is the trigger concrete, does the description overlap a sibling, does the body match what it promises."
---

`ai-rulez review` scores your skills, agents and commands against a rubric: is the trigger concrete, does the
description overlap a sibling, does the body match what it promises. Without flags it is offline: each rubric
dimension is scored from the lint findings that bear on it, and nothing leaves the machine. `--semantic` adds a model
judge for what lint cannot see; the judge is advisory, cached and capped, and it can fail a build only after
`review calibrate` has measured it against human labels. `review fix` proposes a verified patch for its findings.

Review scores the quality of authored items dimension by dimension. To audit all agent configuration for security,
governance and quality problems with quoted evidence, use [Judge](/ai-rulez/judge/). Model access is configured on
[LLM access](/ai-rulez/llm/).

## Example

```console
$ ai-rulez review
rubric skill-quality@2 (builtin:skill-quality, sha256:537e621b)  offline: lint evidence only, no model call
rule:atomic-commits          rule     skipped: rubric skill-quality does not apply to rule items
skill:deploy-staging         skill    score 100/100
skill:release-notes          skill    score 100/100
3 items: 2 scored, 0 withheld, 0 excluded, 1 skipped; mean score 100
```

Before adding the judge, see what a judged run would send and cost. `--estimate` sends nothing:

```console
$ ai-rulez review --estimate --model gemini-2.5-flash-lite
...
egress manifest (estimate; nothing is sent by --estimate)
  model gemini-2.5-flash-lite  host (provider default)  network allowed: false  content: descriptions
  skill:deploy-staging         2 call(s)   2829 bytes  sha256 adb401f984a3
  skill:release-notes          2 call(s)   2800 bytes  sha256 ac147370b05a
  2 items, 4-12 calls, ~3038-9114 input tokens, up to 18000 output tokens
  cost $0.0009 to $0.0081 (cap $0.5, 300 calls)
```

With `[llm] allow_network = true` in your user config, a judged run looks like this:

```text
$ ai-rulez review --semantic --content full
rubric skill-quality@2 (builtin:skill-quality, sha256:537e621b)  model gemini/gemini-2.5-flash-lite (resolved gemini/gemini-2.5-flash-lite)  votes<=3  content full  advisory
skill:deploy-staging         skill    score 100/100  semantic 45/100
  AR9G1  trigger-quality      fail - The description does not name a concrete trigger or when not to use the skill.  [judge, agree 2/2]
  AR9G2  overlap              fail "Write the release notes for the rollout." - overlaps release-notes.  [judge, unstable agree 2/3]
skill:leak                   skill    withheld: AR001 secret-detected: never sent to a judge
3 items: 2 scored, 1 withheld, 0 excluded, 0 skipped; mean score 100; mean semantic score 45 over 2 judged
2 items judged, 0 answers cached; calls 9, tokens 11204, cost $0.0113 (cap $0.5)
calibration: missing
  no calibration record: run `ai-rulez review calibrate`
```

Other commands:

```bash
ai-rulez review skill:deploy-staging             # one item: an id, a name or a path
ai-rulez review calibrate --golden tests/review/golden/skill-quality
ai-rulez review --semantic --gate                # exit 2 on a stable fail of a calibrated dimension
ai-rulez review fix skill:deploy-staging --model gemini-2.5-flash   # print a patch; --apply writes it
ai-rulez review explain AR9G2                    # what a code means, with the rubric's definitions
ai-rulez rubric lint                             # check .ai-rulez/rubrics/*
```

## The offline score

Each rubric dimension lists `twins`: the lint rules that already say something about it (`trigger-quality` has
`AR801`-`AR803`). The dimension's verdict comes from the twin findings on the item and, for a skill or command, on its
resource files:

| Twin finding | Verdict | Value |
| --- | --- | --- |
| none, or only `info` | `pass` | 1 |
| a warning | `warn` | 0.5 |
| an error | `fail` | 0 |

`score = round(100 * sum(weight_i * value_i) / sum(weight_i))` over the scored dimensions. A dimension with no twin
(the built-in `instruction-conflict`) is `not-scored` and left out. `pass` means no lint rule fired, not that a person
would call the description good: read the offline score as a floor.

A dimension below `pass` is reported with its code (`AR9G1`-`AR9G7`, see [Rule codes](#rule-codes)) at the dimension's
severity ceiling (`warning` or `info`, never `error`). Every finding is advisory and carries a fingerprint that
survives line moves.

The built-in rubric `builtin:skill-quality` (`ai-rulez rubric show`):

| Dimension | Code | Weight | Group | Body needed | Twins |
| --- | --- | --- | --- | --- | --- |
| `trigger-quality` | `AR9G1` | 0.30 | intrinsic | no | `AR801`, `AR802`, `AR803` |
| `overlap` | `AR9G2` | 0.25 | contextual | no | `AR701`, `AR702` |
| `body-accuracy` | `AR9G3` | 0.15 | intrinsic | yes | `AR201`, `AR202`, `AR301`, `AR302`, `AR401`, `AR402` |
| `injection-intent` | `AR9G4` | 0.10 | intrinsic | yes | `AR003`, `AR004`, `AR017`, `AR018`, `AR020` |
| `scope-creep` | `AR9G5` | 0.10 | intrinsic | yes | `AR007`, `AR013` |
| `instruction-conflict` | `AR9G6` | 0.05 | contextual | yes | none |
| `body-structure` | `AR9G7` | 0.05 | intrinsic | yes | `AR805`, `AR806`, `AR901`, `AR902` |

It applies to skills, agents and commands.

## What is never sent

These filters run before anything is planned, and decide what may ever leave the machine:

- An item with a credential or hidden character (`AR001`, `AR002`) in its file or its skill resources is **withheld**:
  no score, no manifest entry, an `AR9G0` note. Review scans the text itself, so lint ignores and severity overrides
  cannot make such an item eligible. With `[review] on_secret = "redact"`, an item whose own text holds a credential
  is sent with it replaced by `[REDACTED:AR001]`; it is still withheld when the masked text still looks like a
  credential, when the credential is in a resource file, or when it has a hidden character.
- A twin **error** pre-empts its dimension: the judge is not asked, and the lint verdict stands.
- `[review] exclude` globs (matched against name, id and path) exclude an item.
- Content from includes, installed skills, skill sources and builtins is skipped unless `--include-imports`.
- An item the rubric does not apply to, or that cannot be read, is skipped with the reason.

Withheld, excluded and skipped items are always listed with their reason.

## Estimate and the egress manifest

`--estimate` prints what a judged run would send and sends nothing. It works offline and is the recommended first
step.

- **What**: per item, the planned calls (an `intrinsic` call with the item, a `contextual` call with its description
  and up to `max_siblings` sibling descriptions), their size, and the SHA-256 of the data. Never the content.
- **Where**: the host of the resolved `[llm] base_url` (`(provider default)` when unset) and whether `allow_network`
  is on. A repository `base_url` is ignored, by the [trust rule](/ai-rulez/llm/#trust-rule).
- **How much**: tokens at one per three bytes; the low bound is one vote per call and a 400-token answer, the high
  bound `votes.max` votes at `max_output_tokens`. Prices come from the built-in table or `[llm]
  price_input_per_mtok`/`price_output_per_mtok`.
- **Caps**: `--max-cost`/`--max-calls`, else `[review] max_cost_usd`/`max_calls`, else 0.50 USD and 300 calls
  (`--max-cost 0` is unlimited). A run is refused (exit 1) when even the low bound exceeds a cap, or when the model has
  no price and a cost cap is set.
- `--show-prompt` prints the exact planned messages.

<a id="the-judge-semantic"></a>

## The judge: `--semantic`

```text
ai-rulez review --semantic [--content descriptions|full] [--k N] [--model M | --models A,B]
                           [--max-cost USD] [--max-calls N] [--no-cache] [--cache-dir DIR]
                           [--gate [--gate-level info|warning|error]] [--baseline F] [--write-baseline F]
```

Two opt-ins are required: `--semantic`, and `[llm] allow_network = true` in user scope (the user config file or
`AI_RULEZ_LLM_ALLOW_NETWORK`; see the [trust rule](/ai-rulez/llm/#trust-rule)). A model must be set (`[llm] model` or
`--model`). Nothing is sent until every check, including the caps, has passed.

**Content.** `--content descriptions` (default) sends the name, description and frontmatter keys. Dimensions that need
the body (`body-accuracy`, `injection-intent`, `scope-creep`, `instruction-conflict`, `body-structure`) are then
`skipped` ("needs --content full"), not guessed, so a default run judges `trigger-quality` and `overlap`.
`--content full` adds the frontmatter and body, truncated head and tail at the rubric's `max_item_tokens`. A truncated
item's verdicts are capped at `info`, and `--gate` refuses to run on it.

**Prompt.** The item is sent inside a fence whose delimiter carries a nonce derived from the request, so the item's
author cannot close it. The model is told the fenced text is data, and that text addressed to the reviewer is itself a
finding. No tools are exposed. The reply must match a JSON schema exactly; an invalid reply is retried once, then the
dimension is `error`, never `pass`.

**Evidence.** Every quote a verdict cites must appear verbatim (whitespace-normalized) in what was sent, and be 8
characters to 20 words long. A quote that does not is dropped and counted (`hallucinated_evidence`); a `warn` or
`fail` left with no valid quote is reported as `pass`, unless the dimension sets `allow_absence` (the problem is that
something is missing).

**Votes.** The first vote runs at `first_temperature`. A pass is accepted, except for `injection-intent`, which is
always voted on again. A flagged dimension gets up to `--k` votes (default the rubric's `votes.max`, 3) with the
dimensions and siblings reshuffled, stopping once two votes agree. The verdict is the median (`pass < warn < fail`);
`agreement` is the share of votes equal to it. Below `instability_threshold` (0.67) the dimension is `unstable`: it is
reported at `info`, counts as a pass and never gates.

**Semantic score.** The same formula over the dimensions the judge answered.

**Cache and caps.** Answers are cached by the [model layer](/ai-rulez/llm/), keyed by the request and a prompt version that
includes a digest of the rubric and its `system.md`, so editing the rubric invalidates them. A cache hit costs nothing,
so an unchanged project costs nothing to re-run. When `--max-cost` or `--max-calls` is reached the run stops: the
report is `incomplete`, lists the items it did not judge, and exits 0 (1 with `--gate`). A repository `[review]` cap
can only lower the default; the user config, environment and flags can set any value.

**Hosts.** A user-scope `[review] allowed_hosts` (or `AI_RULEZ_REVIEW_ALLOWED_HOSTS`, comma-separated) restricts where
a judged run may send; `provider-default` names the provider's own endpoint. A run to another host is refused before
anything is sent. Without a list, review warns and accepts any host. See
[Data egress](/ai-rulez/llm/#data-egress-and-residency).

**Findings.** One code per dimension (`AR9G1`-`AR9G7`), at the dimension's severity ceiling, marked `advisory: true`
and `origin: "llm-judge"`, with the first quote, rationale, suggestion, vote agreement and a fingerprint of
`code | item | dimension | normalized first quote`.

## Calibration and the gate

The judge may fail a build only after it was measured. `review calibrate` judges every case of a golden set the way
`--semantic` does (all `k` votes) and compares the verdicts with adjudicated human labels.

```bash
ai-rulez review calibrate --golden tests/review/golden/skill-quality --model gemini-2.5-flash-lite
ai-rulez review calibrate --models gemini-2.5-flash-lite,gemini-2.5-flash   # compare models; writes nothing
ai-rulez review calibrate --compare .ai-rulez/calibration/skill-quality.builtin.json   # drift check
```

A golden case (`golden/*.golden.yaml`, paths relative to the golden directory) is labeled by at least two people and
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

Per dimension the calibration measures:

| Measure | Meaning | Threshold (`[calibration]`) |
| --- | --- | --- |
| weighted kappa | agreement with the adjudicated labels | `min_weighted_kappa` |
| precision, recall | of "flagged" (not `pass`), with Wilson 95% intervals | `min_precision`, `min_recall.<dimension>` |
| human kappa | agreement between the labelers; below it the dimension is `ill-defined` (fix the rubric, not the model) | `min_human_kappa` |
| consistency | share of cases with identical votes across `k` | `min_consistency` |
| probes | `pad` (filler must not improve a verdict), `reorder` (sibling order must not change it), `rename` (the name must not change it), `canary` (text addressed to the reviewer must be flagged by `injection-intent`) | `min_probe` |

A dimension with fewer than 6 labeled cases is `uncalibrated`. The record passes when no calibrated dimension is `fail`
or `ill-defined`, at least one passes, and the set has `golden_min_items` cases.

The record is `calibration.json` in the rubric directory, or `.ai-rulez/calibration/<id>.builtin.json` for a built-in
rubric. It binds the result to the rubric digest, the prompt digest, the golden-set digest, the resolved model id (what
the provider answered with, not the alias), the content mode and `k`.

`review --semantic --gate` is refused (exit 1, nothing spent) unless a record matches all of those, passed, and is
younger than `max_age_days` (`[review.gate] calibration_max_age_days` overrides it). It is also refused for a floating
alias (`...-latest`) and when the answering model differs from the record's. It then exits 2 on a **stable** `fail`
of a dimension whose calibration passed, at or above `--gate-level` (default `warning`). Unstable, pre-empted,
errored, truncated and baselined verdicts never gate, and an errored or truncated item makes the gated run refuse as
incomplete. `[review.gate] require_calibration = false` lets `--gate` run without a record.

`AR9G9 judge-calibration-stale` (info) is reported by every judged run on a floating alias, under another model id, or
without a matching record.

`calibrate` needs the same opt-in as `--semantic` and is capped by `--max-cost` (default 2.00 USD) and `--max-calls`
(default 1000); a run cut short writes nothing. It exits 0 when the judge passes and 2 when it does not or drifted.
`--content` defaults to `full` for `calibrate` (the record binds the content mode, so calibrate with the mode you gate
with). `--no-probes` skips the probes, `--no-write` and `-o FILE` control where the record goes.

## Drift and model comparison

`review calibrate --compare calibration.json` is the scheduled drift job: it calibrates again, compares with the
committed record, prints every golden verdict that changed, and exits 2 when a dimension's kappa fell by more than
0.05, a dimension stopped passing, the resolved model changed, or the judge no longer meets its thresholds.

`--models A,B` (on `review --semantic` and `calibrate`) judges with each model and reports pairwise agreement and kappa
per dimension, and lists the items the models disagree on as "review first". The first model is the primary one; the
caps are split between the models. On `calibrate` it writes no record.

## `review fix`

```text
ai-rulez review fix [id|name|path...] [--finding FINGERPRINT] [--model FIXER] [--judge-model JUDGE]
                    [--output fix.patch] [--apply] [--allow-same-model] [--patch fix.patch]
```

For each authored item with a stable judged finding, the fixer model proposes exact-text edits (each `old` text must
occur once in the file); ai-rulez builds the unified diff from them. A proposal is kept only when all of this holds,
with up to two attempts (the second sees why the first failed), else the item reports "no safe fix":

1. The edits apply exactly and change the file.
2. In the frontmatter, only `description` changed: `name` and the tool list are untouched.
3. The item grew by at most `[review.fix] max_growth_percent` (default 25%, at least 200 bytes allowed).
4. The security scan finds nothing new, and no credential, hidden character, link or executable line was added.
5. A judge that is a **different model from the fixer** (unless `--allow-same-model`) rates every targeted dimension
   better and no other worse.

Only authored content in the configuration directory is touched; items from includes, installed skills, builtins, the
local overlay and generated outputs are reported as not fixable. The patch goes to `--output` or standard output, with
a header naming the item digest, finding, models and rubric version:

```text
# ai-rulez review fix
# item: skill:deploy-staging
# path: .ai-rulez/skills/deploy-staging/SKILL.md
# digest: sha256:2da78adb...
# finding: 51c0e2... AR9G1 trigger-quality
--- a/.ai-rulez/skills/deploy-staging/SKILL.md
+++ b/.ai-rulez/skills/deploy-staging/SKILL.md
@@ -1,5 +1,5 @@
...
```

`--apply` writes the verified edits. It refuses a file whose digest differs from the patch header (stale patch), and a
file with uncommitted changes or outside a git repository; every file is checked before the first is written. Run
`ai-rulez lock` afterwards (with [approvals](/ai-rulez/approvals/), the change needs a new approval). `--patch FILE` verifies
and applies a patch from an earlier run without calling a model; it may only name items a fix run could edit.

The fixer is `--model`, else `[review.fix] model`; the judge is `--judge-model`, else `[llm] model`. `review fix`
defaults to `--content full`. It exits 2 when a finding had no safe fix. The same pipeline backs
[`improve --with builtin:review-fix`](/ai-rulez/improve/#bundled-adapters), where a held-out eval gate decides whether the fix
is kept.

## Selecting items

- `[id|name|path...]`: an id such as `skill:deploy-staging` or `skill:ops/deploy-staging`, a name, or a path. A selector that
  matches nothing is an error.
- `--since REV`: only items whose files changed since the revision (committed, staged, unstaged, untracked). Siblings
  are still drawn from every item.
- `--role NAME` / `--profile NAME`: only the content slice `generate --role` or `--profile` renders. The two are
  exclusive.
- `--no-local`: ignore the machine-local overlay and `local/` content.

`--concurrency N` (default 4, at most 16) sets how many items `review`, `review fix` and `review calibrate` judge at
once.

## Baselines

`--write-baseline FILE` writes the fingerprints of this run's findings; `--baseline FILE` hides them in a later run, so
a team can adopt review without fixing everything first. Baselined findings are listed as `baselined` in JSON, left
out of text and SARIF, and never gate. An earlier `--format json` report also works as a baseline. A finding whose
quote changed has a new fingerprint.

## Rubrics

A rubric is a versioned directory you own. It is never written into a harness's output tree, and
[`ai-rulez lock`](/ai-rulez/lockfile/) pins it as kind `rubric` over every file in the directory, so an edit to a rubric, its
prompt, its labels or its calibration shows as lock drift (`AR981`) until you lock again.

```text
.ai-rulez/rubrics/<id>/
  rubric.toml                 # dimensions, weights, limits, votes, thresholds
  system.md                   # optional system prompt override
  golden/*.golden.yaml        # human-labeled cases for calibration
  fixtures/...                # the items the cases point at
  calibration.json            # written by `review calibrate`
```

`builtin:skill-quality` is embedded in the binary. Select a rubric with `--rubric <id>` or `[review] rubric`. A rubric
that fails `rubric lint` is refused.

```toml
schema_version = 1
id = "my-rubric"                 # must equal the directory name
version = 1                      # bump on any change that can alter output
applies_to = ["skill", "agent", "command"]   # also: rule

[limits]
max_item_tokens = 6000           # longer bodies are head+tail truncated; verdicts are capped at info
max_siblings = 5
max_output_tokens = 700          # the judge's completion cap; unset means 1500

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
code = "AR9G1"                   # AR9G1-AR9G7
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

A custom rubric can reorder, reweight or retarget dimensions, but uses only the codes `AR9G1`-`AR9G7`.

### `ai-rulez rubric`

| Command | Purpose |
| --- | --- |
| `rubric list` | The built-in and project rubrics with version and digest. |
| `rubric show [id]` | Dimensions, weights, twins and the formula. |
| `rubric lint [id...]` | Check rubrics (default: every project rubric; `builtin:<id>` is accepted). `--format text\|json\|sarif`. Exit 2 with findings, 1 when unreadable. |

`rubric lint` reports `AR9G8` for: a wrong `schema_version`, an `id` other than the directory name, weights that do not
sum to 1, duplicate dimension ids or codes, a code outside `AR9G1`-`AR9G7`, a twin that is not a lint rule, a
`severity` of `error`, thresholds out of range, an unknown key, a golden case with fewer than two labelers, an unknown
dimension, verdict or probe, a path with `..`, a calibration record for another rubric, and any symlink.

### Rule codes

| Code | Meaning |
| --- | --- |
| `AR9G0` | Run note: an item was withheld, excluded, skipped, or not judged because the run stopped. |
| `AR9G1`-`AR9G7` | The rubric's dimensions (built-in: see the table under [The offline score](#the-offline-score)). |
| `AR9G8` | `rubric-invalid`. |
| `AR9G9` | `judge-calibration-stale`. |

`review` and `rubric lint` emit them; `validate` does not. `review explain AR9G2` prints the explanation and the
rubric's definitions. See [code ranges](/ai-rulez/strict-validation/#code-ranges).

## Configuration

```toml
[review]
rubric = "builtin:skill-quality"   # or a directory name under .ai-rulez/rubrics
content = "descriptions"           # descriptions | full
exclude = ["internal-*"]           # name globs
max_cost_usd = 0.50                # a repository value can only lower the default
max_calls = 300
on_secret = "withhold"             # withhold | redact
allowed_hosts = []                 # honored only from the user config file

[review.gate]
level = "warning"                  # info | warning | error
require_calibration = true         # false: --gate runs without a matching record
calibration_max_age_days = 90

[review.fix]
model = ""                         # must differ from the judge unless --allow-same-model
max_growth_percent = 25
```

| Key | Default | Meaning |
| --- | --- | --- |
| `rubric` | `builtin:skill-quality` | Rubric to score against. |
| `content` | `descriptions` | What a judge receives. |
| `exclude` | `[]` | Items never reviewed (globs on name, id and path). |
| `max_cost_usd`, `max_calls` | `0.50`, `300` | Caps of a judged run. A repository value can only lower them. |
| `on_secret` | `withhold` | `withhold` or `redact` an item whose own text holds a credential. |
| `allowed_hosts` | `[]` (any, with a warning) | Hosts a judged run may send to. User config only; `AI_RULEZ_REVIEW_ALLOWED_HOSTS` overrides. |
| `gate.level` | `warning` | Lowest severity ceiling that gates. |
| `gate.require_calibration` | `true` | Refuse `--gate` without a matching calibration record. |
| `gate.calibration_max_age_days` | the rubric's `max_age_days` | Maximum age of the record. |
| `fix.model` | none | Fixer model. |
| `fix.max_growth_percent` | `25` | Most an item may grow in a fix. |

An invalid `[review]` table is an error. To price a model the built-in table does not know while a cost cap is set,
set `[llm] price_input_per_mtok` and `price_output_per_mtok` in user scope.

## Output and exit codes

`--format text` (default), `json` ([`review-report.schema.json`](/ai-rulez/schema/), `review-report/1`) or `sarif`.
`-o FILE` writes the report to a file. SARIF results use the dimension codes as rule ids, carry `advisory: true`,
`origin: "lint-twin"` or `"llm-judge"`, and the `aiRulezReviewFingerprint/v1` fingerprint. A judged run's JSON adds
`run` (calls, cache hits, tokens, cost, caps, `incomplete`, requested and resolved models, `hallucinated_evidence`),
`items[].semantic`, `findings`, `egress`, `calibration`, `gate`, `models` and `baseline`.

| Code | Meaning |
| --- | --- |
| 0 | Reviewed. Advisory findings never change it. |
| 1 | Could not run or was refused: a bad flag or config, an invalid rubric, no network opt-in or model, a host outside `allowed_hosts`, an estimate over a cap, `--gate` without a matching calibration, an incomplete run with `--gate`. |
| 2 | `--gate` failed; `calibrate` did not pass or drifted; `review fix` found no safe fix; `rubric lint` found problems. |
