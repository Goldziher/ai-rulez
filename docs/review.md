# Review

`ai-rulez review` scores skills, agents and commands against a rubric you own. This build is **offline**: the score
comes from the lint findings each rubric dimension names, no model is called, and nothing leaves the machine. It also
plans what a model-judged run would send (`--estimate`), so you can read the egress manifest and the cost before any
judge exists in your setup. Design: [#220](https://github.com/Goldziher/ai-rulez/issues/220).

- [Quick start](#quick-start)
- [How the score is built](#how-the-score-is-built)
- [Pre-filters](#pre-filters)
- [Estimate and the egress manifest](#estimate-and-the-egress-manifest)
- [Rubrics](#rubrics)
- [Configuration](#configuration)
- [Output and exit codes](#output-and-exit-codes)
- [What is not in this build](#what-is-not-in-this-build)
- [Design decisions](#design-decisions)

## Quick start

```console
ai-rulez review                      # offline score of every skill, agent and command
ai-rulez review skill:deploy         # one item: an id, a name or a path
ai-rulez review --estimate           # egress manifest and cost range of a judged run; sends nothing
ai-rulez review --format sarif --out review.sarif
ai-rulez rubric lint                 # check .ai-rulez/rubrics/*
```

```text
rubric skill-quality@1 (builtin:skill-quality, sha256:6f1342b8)  offline: lint evidence only, no model call
skill:deploy                 skill    score 89/100
  AR9G4  injection-intent     warn AR004 text reads like an attempt to override instructions: "Ignore previous instructions"
skill:leak                   skill    withheld: AR001 secret-detected: never sent to a judge
3 items: 2 scored, 1 withheld, 0 excluded, 0 skipped; mean score 87
```

## How the score is built

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

Read the score as a floor from deterministic checks, not as a judgement: `pass` means no lint rule fired, not that a
person would call the description good. Semantic quality (a vague trigger, a body that contradicts its description) is
what a later judge adds.

A finding of a dimension below `pass` is reported with the dimension's code (`AR9G1`-`AR9G7`, see
[Rule codes](#rule-codes)), at the dimension's severity ceiling (`warning` or `info`, never `error`). Every finding is
advisory and carries a fingerprint that survives line moves.

## Pre-filters

Run before anything is planned, and they decide what is ever eligible to leave the machine:

- An item with a secret or hidden-character finding (`AR001`, `AR002`) on its file or its skill resources is
  **withheld**: no score, no manifest entry, `AR9G0` note. It is never redacted and sent. The decision does not
  rest on the lint findings alone: review scans the item's own text for a credential or a hidden character
  directly, so `[lint] ignore`, `[lint.severity]`, `ignore_paths` and an inline `ai-rulez-lint-ignore` cannot make an
  item eligible to leave the machine.
- A twin **error** pre-empts the dimension: a judge would not be asked, so the manifest leaves it out of the calls.
- `[review] exclude` globs (matched against the name, the id and the path) exclude an item.
- Content from includes, installed skills, skill sources and builtins is skipped unless `--include-imports`. It is
  never read when skipped.
- An item the rubric does not apply to (`applies_to`) or that cannot be read is skipped with the reason.

Nothing is dropped silently: withheld, excluded and skipped items are listed with their reason.

## Estimate and the egress manifest

`--estimate` adds a manifest of what a model-judged run **would** send. This build sends nothing (`"sent": false`).

```text
egress manifest (estimate; nothing is sent in this build)
  model claude-haiku-4-5  host gateway.internal  network allowed: false  content: descriptions
  skill:deploy                 2 call(s)   2735 bytes  sha256 dd57c18f4bb0
  withheld (never sent): skill:leak
  2 items, 4-12 calls, ~2167-6501 input tokens, up to 8400 output tokens
  cost $0.0102 to $0.0485 (cap $0.50, 300 calls)
```

- **What**: per item, the planned calls (an `intrinsic` call with the item, a `contextual` call with the item's
  description and up to `max_siblings` sibling descriptions), their bytes, and the SHA-256 of the data block. Never
  the content. `--content descriptions` (default) sends the name, description and frontmatter keys; `--content full`
  adds the body, head and tail truncated at the rubric's `max_item_tokens`.
- **Where**: the host of the resolved `[llm]` base URL (`(provider default)` when unset) and whether
  `allow_network` is on. `[llm]` follows the [trust rule](llm.md#trust-rule): a repository `base_url` is ignored and
  reported as ignored.
- **How much**: tokens are estimated at one per three bytes (conservative); cost uses the model's price from the
  built-in price table or `[llm] price_input_per_mtok`/`price_output_per_mtok` (the same table `llm estimate` uses).
  The lower bound is one vote per call and a 400-token answer; the upper bound is `votes.max` votes per call at
  `max_output_tokens`.
- **Caps**: `--max-cost` / `--max-calls`, else `[review] max_cost_usd` / `max_calls`, else 0.50 USD and 300 calls
  (`--max-cost 0` means unlimited). The estimate is **refused** (exit 1, listed
  under `refused`) when even the lower bound exceeds a cap, or when the model has no price and a cost cap is set.
  With no model configured nothing is priced and nothing is refused. An organization policy that forbids model calls
  (`[llm] allow_network = false`) refuses the estimate too.
- `--show-prompt` prints the exact planned messages. The nonce of the data fence is shown as `<nonce:000000000000000000000000>`, as long as a real 128-bit hex nonce, so the plan and its hash are
  reproducible and the byte counts realistic; a real run would draw a random one. An item's `sha256` covers the data
  of every call planned for it.

## Rubrics

A rubric is a user-owned, versioned directory. It is never written into any per-tool output tree (no context cost).

```text
.ai-rulez/rubrics/<id>/
  rubric.toml                 # dimensions, weights, limits, thresholds
  system.md                   # optional system prompt override
  golden/*.golden.yaml        # human-labelled cases for calibration (placeholder in this build)
  calibration.json            # written by a later `review calibrate` (placeholder)
```

`builtin:skill-quality` is embedded in the binary. Use `--rubric <id>` or `[review] rubric`. A rubric that fails
`rubric lint` is refused, so a review never scores against something other than what the file says.

```toml
schema_version = 1
id = "my-rubric"                 # must equal the directory name
version = 1                      # bump on any change that can alter output
applies_to = ["skill", "agent", "command"]   # also: rule

[[dimension]]
id = "trigger-quality"
code = "AR9G1"                   # AR9G1-AR9G7: the finding code of this dimension
group = "intrinsic"              # intrinsic: the item alone; contextual: needs siblings
weight = 0.6                     # weights must sum to 1
severity = "warning"             # ceiling: warning or info, never error
twins = ["AR801", "AR802"]       # lint rules that pre-empt and score this dimension
question = "Does the description say when to use the skill?"
pass = "Names a concrete trigger and a non-trigger."
warn = "Names the task but the trigger is generic."
fail = "Could apply to almost any request."
```

The optional `[limits]`, `[votes]` and `[calibration]` tables (see the built-in rubric via `ai-rulez rubric show`)
are validated and recorded; they take effect when a judge ships.

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
any symlinked rubric directory or file (`system.md`, golden and calibration files included). The rubric digest covers
every file, so a silent edit changes it.

### Rule codes

`AR9G0`-`AR9G8` are allocated ([code ranges](strict-validation.md#code-ranges)). `AR9G1`-`AR9G7` are the built-in
dimensions, `AR9G0` is the run note, `AR9G8` is `rubric-invalid`. `AR9G9` (judge-calibration-stale) is reserved for the
calibration phase. `validate` does not emit them; `review` and `rubric lint` do.

## Configuration

```toml
[review]
rubric = "builtin:skill-quality"   # or a directory name under .ai-rulez/rubrics
content = "descriptions"           # descriptions | full
exclude = ["internal-*"]           # name globs
max_cost_usd = 0.50                # recorded; bounds the estimate
max_calls = 300                    # recorded; bounds the estimate
```

An invalid `[review]` table (a `content` other than the two values, a negative cap, a bad glob) is an error.

## Output and exit codes

`--format text` (default), `json` (schema [review-report.schema.json](schema.md), `review-report/1`) or `sarif`.
SARIF results use the dimension codes as rule ids, mark `advisory: true` and `origin: "lint-twin"`, and carry
`aiRulezReviewFingerprint/v1`. `--out FILE` writes the report to a file.

| Code | Meaning |
| --- | --- |
| 0 | Reviewed |
| 1 | Could not run (bad flag or config, invalid rubric, no item matches a selector), or `--estimate` shows a real run would be refused |
| 2 | `rubric lint` found problems |

Findings never change the exit status of `review`: it has no gate in this build.

## What is not in this build

`--semantic` does not exist: there is no flag, and `review` never calls a model. Later phases of
[#220](https://github.com/Goldziher/ai-rulez/issues/220): the LLM judge with votes and evidence checks, a response
cache, SARIF for judged findings, `review calibrate` and calibrated `--gate`, `review fix`, `--since`, `--role` and
`--profile` selection, and rubric pinning in the lock file. Until then a rubric edit shows in `rubric list` digests,
not in `ai-rulez.lock`.

## Design decisions

- **`review` exists offline.** The issue leaves open whether phase 0 should be only rubric tooling; the deterministic
  score is useful alone, so it ships.
- **The score is lint evidence, not a judgement.** The alternative (scoring only after a model runs) would leave
  phase 0 with nothing to show. The cost is that `pass` means "no rule fired"; the report says `offline`.
- **No `--semantic` flag.** An error stub invites scripts to depend on it; absence makes an old binary fail with an
  unknown-flag error, as it should.
- **Withhold, never redact.** An item with a secret or hidden characters is not sent in any form (the issue's
  `on_secret = "withhold"` default); the `redact` mode and `on_secret` key wait for the judge.
- **Default content is descriptions.** The issue's proposed default: a smaller egress surface and cost.
- **Default caps 0.50 USD and 300 calls**, from the issue. A flag of `--max-cost 0` means unlimited, explicitly.
- **Unpriced model with a cost cap is refused.** Mirrors the budget layer a judge will sit on; set a price override or
  `--max-cost 0`.
- **Dimension codes are `AR9G1`-`AR9G7` only.** Findings need a registered code; a custom rubric can reorder, reweight
  or retarget dimensions but not add an eighth code.
- **Rubrics from the repository are data.** Never executed; symlinks are refused rather than followed, so a rubric
  cannot read outside the project.
- **Lint twins are a code list, not a mapping language.** A dimension's twins are rule codes in `rubric.toml`; the
  rule's own severity decides the verdict.
- **Price table.** Prices come from the model layer (`internal/llm`), the table `llm estimate` uses; `review` keeps no
  second copy.
