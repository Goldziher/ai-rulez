---
title: "Judge"
description: "ai-rulez judge audits agent configuration with language models and reports security, governance and quality findings."
---

`ai-rulez judge` audits agent configuration with language models and reports security, governance and quality
findings. It reads what a harness would read (skills with their scripts and references, agents, commands, rules,
`CLAUDE.md` and `AGENTS.md` files, MCP and settings files), whether or not ai-rulez generated it, and needs no
configuration file. Every finding must quote the file, a second model adjudicates what survives, and the scores and the
pass/fail decision are computed in code.

Use judge to find what deterministic checks cannot see: an instruction that tells an agent to hide a step from the
user, a skill whose scripts do more than its description says, a rule that contradicts the `CLAUDE.md` above it. For
the free, exact checks, use [`validate` and `scan`](/ai-rulez/strict-validation/). For scoring skill descriptions dimension by
dimension against a rubric you calibrate, with an offline baseline and verified fixes, use [Review](/ai-rulez/review/).

## Example

See what would be sent, to whom, and what it would cost. `--estimate` sends nothing:

```console
$ ai-rulez judge .ai-rulez/skills/deploy-staging .ai-rulez/rules/atomic-commits.md --estimate
judge estimate (nothing was sent)
categories: security, governance, quality
model:      security    bedrock/global.anthropic.claude-sonnet-5-5
model:      governance  bedrock/global.anthropic.claude-sonnet-5-5
model:      quality     bedrock/global.anthropic.claude-sonnet-5-5
model:      adjudicator bedrock/global.anthropic.claude-opus-5-5
egress:     (the provider's default endpoint)
  send  .ai-rulez/rules/atomic-commits.md (rule, 1 files)  +0 rules, 2 instruction files
  send  .ai-rulez/skills/deploy-staging (skill, 2 files)  +0 rules, 2 instruction files
units to send: 2, calls: 8, prompt tokens: ~45957, max completion tokens: 49152
worst-case cost: <= $0.7765 (expected ~$0.1519)
```

Then run it. `--allow-llm` is your consent to send the files to the model's provider:

```bash
ai-rulez judge --allow-llm                                   # everything below this directory
ai-rulez judge .claude/skills/deploy --allow-llm --fail-on security:medium,governance:high
ai-rulez judge --changed=origin/main --allow-llm --format sarif > judge.sarif
ai-rulez judge --allow-llm --model openai/gpt-4o --adjudicator-model openai/o3
```

Judge needs model access configured as described on [LLM access](/ai-rulez/llm/). By default it uses Amazon Bedrock; pass
`--model provider/model` (or set `[llm]` in your user config) to use another provider.

## How a review works

1. **Units.** The files become units: a skill is its directory, anything else is one file.
2. **Static facts.** `validate` and `scan` run over the units. Their findings reach the reviewers as numbered leads
   (at most 64 per unit). A lead is never a finding by itself: a reviewer has to confirm it in the text.
3. **Reviewers.** One call per category and unit: **security** (`JR-S01` to `JR-S15`), **governance** (`JR-G01` to
   `JR-G07`) and **quality** (`JR-Q01` to `JR-Q13`). Each reviewer must separate a threat (an active instruction,
   executable code, a capability grant) from context (a prohibition, documentation, an example, a fixture), and state
   the strongest innocent reading of every finding next to the reason it fails.
4. **Verifier.** Every finding carries a verbatim quote and a line range, checked in code against the text that was
   sent (whitespace-normalized). A finding that fails goes back to its reviewer once with the exact cited lines; if it
   still fails it is dropped and counted.
5. **Adjudicator.** A separate call sees the surviving findings with eight lines of context, the static facts, the
   conventions and a list of known false-positive shapes, and decides `keep`, `downgrade`, `drop` or `merge` for each.
6. **Scoring.** Code computes the scores, the unit status and the decision. A model never emits a score or verdict.

The adjudicator is asymmetric. A **security** finding is dropped only with a concrete benign context (a prohibition,
a documented example, a placeholder, a declared service, a static literal) and a counter-quote taken verbatim from the
file; otherwise it stays. **Quality** and **governance** findings are dropped unless they show a concrete defect or a
violated binding convention. Static findings such as hidden characters in an instruction file (`AR002`) bypass the
adjudicator. `--explain-drops` lists every dropped candidate with the stage, the reason and the counter-quote.

## What it judges

With no paths, judge takes every unit below the working directory that a harness reads:

| Unit kind | Where |
| --- | --- |
| `skill` | a directory with a `SKILL.md`, with every file below it |
| `agent`, `command`, `rule` | `agents/`, `commands/`, `rules/` below a `.claude/`, `.agents/`, `.ai-rulez/`, `.cursor/` or `.github/` directory, at any depth |
| `claude-md`, `agents-md` | `CLAUDE.md`, `CLAUDE.local.md`, `AGENTS.md`, `AGENTS.local.md`, at any depth |
| `mcp-config` | `.mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json` |
| `settings` | `.claude/settings.json`, `.claude/settings.local.json` |
| `plugin` | `.claude-plugin/plugin.json`, `marketplace.json` |

Name files or directories to judge only those. Any Markdown or JSON file you name is a unit (kind `other` when no
convention applies), and a file inside a skill selects the whole skill. A file with the ai-rulez generated header is
skipped (`generated file: judge its sources`): judge the `.ai-rulez/` sources instead.

| Limit | Value |
| --- | --- |
| Units per run | 200 (`--estimate` is not limited; name paths or use `--changed`) |
| File size | 2 MiB |
| Unit size | 40 files, 512 KiB |
| Input per call | 24,000 tokens of file text; a larger unit is split into chunks (at most eight per category) |

Judge needs a project: a git repository, or a directory with its own `AGENTS.md`, `CLAUDE.md`, `.claude/`, `.agents/`
or ai-rulez configuration. It refuses to run in `$HOME` or `/`. Symlinks are never followed, and paths must stay
inside the project.

**Changed units.** `--changed=REV` judges only units changed since `REV` (committed, staged, unstaged and untracked);
a bare `--changed` compares with `HEAD`. Write the base with `=`: a separate word is a path to judge. `--since REV` is
the same as `--changed=REV`. With `--impact` (on by default) it also judges the units a changed rule or `CLAUDE.md`
applies to.

## Governance and your conventions

The governance reviewer enforces only what is written down, and it is shown the conventions that apply to each unit:

- **Path-scoped rules.** For every ancestor directory `D` of a unit's file, the Markdown files below `D/.claude/rules/`.
  A rule with a `paths:` list attaches when one of its globs (relative to `D`; `**`, `*`, `?`, `[abc]`, `{a,b}`)
  matches a file of the unit or a path the unit mentions. A rule without `paths:` always attaches.
- **Instruction files.** `CLAUDE.md`, `CLAUDE.local.md`, `AGENTS.md` and `.claude/CLAUDE.md` of every ancestor
  directory, root first and nearest last, with the files they `@import` (five levels deep). A nearer file overrides a
  farther one on the same topic.
- **Binding versus guidance.** `MUST`, `NEVER`, `ALWAYS`, "required" and "forbidden" are binding; `SHOULD`, "prefer"
  and "recommended" are guidance, and a deviation from guidance is at most `low`.
- **Two quotes.** A governance finding quotes both the file and the convention it violates, or it is dropped.

`--attach-rules=false` and `--attach-claude-md=false` (or `attach_rules`, `attach_claude_md` in `[judge]`) turn the
two kinds off. `--estimate` lists what is attached to every unit.

## Rubric skills

A skill whose body has `## C1 - title` sections is a rubric: each section is a checkable criterion.

```markdown
---
name: acme-skill-standards
description: Acme rubric for skill files.
---

## C1 - Trigger in the description
The `description` contains "Use when" followed by a concrete situation.

## C2 - Verify section
The body has a `## Verify` heading followed by a command or an observable check.
```

```bash
ai-rulez judge --allow-llm --skill acme-skill-standards               # governance review
ai-rulez judge --allow-llm --skill-for quality=path:rubrics/style      # another category
```

The reviewer must report every criterion as `pass`, `fail` or `not_applicable`. A failed criterion becomes a `JR-G06`
finding with the criterion id (`rubric:acme-skill-standards/C1`). A criterion nobody reported is `not_evaluated`,
never a silent pass. When a failure is an absence (no `## Verify` heading) the finding is raised for the unit as a
whole, without a quote.

## Trust tiers

What reaches a model is split into tiers by marker lines that carry a per-request token, which the author of a
reviewed file cannot predict.

| Tier | Content | Authority |
| --- | --- | --- |
| T0 | the built-in system prompt and output schema | cannot be changed |
| T1 | rubric text and prompt extensions you configured: a `path:DIR` skill, a pinned skill, a prompt file | adds checks; can never relax the evidence rules, change the schema or name a verdict |
| T2 | repository conventions, static facts, unpinned or inline skills | reference data, never obeyed |
| T3 | the files under review | inert data; text that addresses a "reviewer" or "AI", claims to be approved, or asks for a score is itself a finding (`JR-S01`) |

A repository decides neither where content goes nor what a run may spend. A repository `[judge]` or `[llm]` table
cannot set the models, provider, base URL, consent, key variable or spend caps: they come from flags, the environment
and your user config file, and a repository value is ignored and reported in the run's notices. A rubric skill from the
project is T2 unless `[judge.skills.pins]` pins its hash and the hash matches.

## Scores, gating and exit codes

Per category: `100 - sum(points x confidence weight x decay)`. Points are 40 critical, 20 high, 8 medium, 3 low;
confidence weights are 1.0 high, 0.6 medium, 0.3 low; the largest deductions are weighted 1, 0.5 and 0.25, and a fourth
adds nothing. A category that did not run is `null`, not 100.

`--categories` limits the run to some of `security`, `quality` and `governance` (default all; `[judge] categories`).

`--fail-on` takes a comma-separated list of `category:severity`, `category:none`, `score:N` (any category below N) or
`category:score:N`. The default is `security:high,governance:high,quality:none`: quality findings are advice unless you
say otherwise. A category the list does not name keeps its default. `--min-confidence` drops findings below a
confidence.

A unit is `fail` (a `--fail-on` rule is met), `partial` (not fully reviewed), `warn` (findings below the gate), `pass`,
`withheld_secret`, `skipped` or `error`. The run decision is `fail`, `incomplete` or `pass`.

| Exit code | Meaning |
| --- | --- |
| `0` | Every unit passed or warned. For `--estimate`: the estimate is within `--max-cost`. |
| `1` | The run could not complete: the provider refused, a cap was reached, a model failed, nothing was left to judge, or a unit was only partly reviewed (`--allow-partial` accepts that). For `--estimate`: the estimate exceeds `--max-cost`. |
| `2` | A unit met `--fail-on`, or a unit was withheld because it holds a secret. |

### Coverage and secrets

Nothing is left out silently. Files that were not sent (binary, unreadable, over a limit), chunks beyond the limit,
calls the caps prevented, a provider refusal (retried once) and replies that were never valid make the unit `partial`
and the run `incomplete`; the report lists what was not reviewed and why.

A unit holding a credential that the security scan (`AR001`) recognizes is withheld whole and reported with
`file:line` and the pattern, never the secret. `--redact` masks the text and sends it instead. Rubrics, prompt files and
trap text are redacted too.

## Models and providers

| Role | Default |
| --- | --- |
| reviewers (security, quality, governance) | `bedrock/global.anthropic.claude-sonnet-5-5` |
| adjudicator | `bedrock/global.anthropic.claude-opus-5-5` |

Override them with `--model` (all reviewers; the adjudicator follows unless named), `--adjudicator-model`,
`--model-for ROLE=MODEL` (`security`, `quality`, `governance` or `adjudicator`), `AI_RULEZ_LLM_MODEL`, the `[llm]`
table of your user config, or `[judge.models]` there. All roles use one provider and one credential. `--provider` sets
the prefix for a bare `--model`. Use the strongest model you have for the adjudicator.

Every call requests JSON against a schema limited to what all providers accept, and the reply is validated strictly in
code. Each call gets 6,144 completion tokens, which thinking models also spend. For a model the price table does not
know, pass `--price-input` and `--price-output` (USD per million tokens). Provider setup, Bedrock credentials and
gateways are on [LLM access](/ai-rulez/llm/#amazon-bedrock).

## Cost, caps and the cache

| Flag | Default | Meaning |
| --- | --- | --- |
| `--estimate`, `-n`, `--dry-run` | off | Build and price every request without calling anything. |
| `--max-cost` | `2` | Spend cap in USD; `0` removes it. With a cap, every model must be priced. |
| `--max-tokens` | `0` (none) | Cap on prompt plus completion tokens. |
| `--max-calls` | `0` (none) | Cap on model calls. |
| `--concurrency` | `4` | Calls in flight. |
| `--rpm` | `0` (none) | Calls started per minute. |
| `--no-cache` | off | Neither read nor write the response cache. |
| `--cache-readonly` | off | Serve cached answers, store none. |

Caps come from flags and your user config, never from a repository. Calls are admitted in priority order (security,
then governance, then quality) until the expected cost reaches a cap; the worst case of each call is enforced as a hard
stop. What does not fit is reported, and the run is `incomplete`. The estimate counts tokens at 3 bytes per token and
leaves out repair calls, so a run that repairs many quotes costs more than estimated.

Answers are cached in your user cache directory, keyed by everything in the request (prompt version, model, schema,
rubric text, attached conventions, unit content, static facts). Only validated replies are stored, entries expire after
30 days, and the cache is authenticated with a per-user secret.

## Configuration

Everything works without configuration. The repository's `[judge]` table records what a team agrees on:

```toml
[judge]
categories       = ["security", "quality", "governance"]
fail_on          = ["security:high", "governance:high", "quality:none"]
min_confidence   = "low"
attach_rules     = true
attach_claude_md = true
traps_file       = ".ai-rulez/judge/traps.yaml"      # known false-positive shapes for the adjudicator

[judge.skills]
load = ["acme-skill-standards"]                       # rubric skills for the governance review
[judge.skills.for]
quality = ["style-rubric"]
[judge.skills.pins]
acme-skill-standards = "3f9c1e0a"                     # a matching hash makes the skill configured text (T1)

[judge.prompts.security]
mode = "extend"                                       # or "override": needs --allow-prompt-override
file = ".ai-rulez/judge/security.md"

[[judge.suppress]]
code    = "JR-S05"
path    = ".claude/skills/setup/SKILL.md"
quote   = "curl -fsSL https://tools.acme.example/install.sh | sh"
reason  = "sanctioned installer, tracked in SEC-1234"
expires = "2026-12-31"
```

- **Prompts.** `extend` adds your text to a reviewer's instructions. `override` replaces the category's role prompt;
  the shared core (trust tiers, evidence rules) and the schema stay. An override from a repository file applies only
  with `--allow-prompt-override`, or `allow_prompt_override = true` in your user config.
- **Traps** add known false-positive shapes for the adjudicator:

  ```yaml
  - id: R01
    name: internal_cli_download
    text: "`acli` downloads pinned tool bundles from artifactory.acme.example; that is the sanctioned installer."
    applies_to: [JR-S05, JR-S10]
  ```

- **Suppressions** are pinned to the quote (changed text brings the finding back), need a reason, and expire at most
  366 days ahead. An expired suppression is reported and no longer applies. Suppressed findings stay in the report
  under `suppressed`.
- **User scope only** (`~/.config/ai-rulez/config.toml`): `[judge.models]` (`security`, `quality`, `governance`,
  `adjudicator` or `reviewers`), `max_cost_usd`, `max_tokens`, `max_calls`, `concurrency`, `rpm`, `allow_llm` and
  `allow_prompt_override`.

`validate` reports `model`, `provider` and `base_url` in a repository `[judge]` table as unknown keys; judge names
where to set them instead.

## Output

`--format text` (default), `json` or `sarif`.

**JSON** follows [`schema/judge-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/judge-report.schema.json)
(`schema_version` 2): the run (prompt version, models, egress hosts, gate, cost, coverage), one entry per unit (status,
scores, attached conventions, rubric results, gaps, findings) and the summary. A finding has a stable `id`, `code`,
`severity`, `confidence`, `verdict` (`confirmed` or `contextual_risk`), `location`, `quote`, `chain`,
`benign_reading`, `why_not_benign`, the `convention` it rests on, `suggested_fix`, its `origin` (`reviewer`,
`adjudicator` or `static-only`) and a `fingerprint`. It never contains file text beyond the quotes, and never the
prompts.

**SARIF** maps `ai-rulez/JR-xxx` to rules: critical and high are `error`, medium `warning`, low and info `note`, with
`security-severity` 9.5, 8.0, 5.5, 3.0 and 1.0. Chains become `codeFlows`, the convention a related location, and
`partialFingerprints` carry the fingerprint. Coverage gaps are tool execution notifications, and an incomplete run has
`executionSuccessful: false`.

Finding text is model output: treat it as untrusted in your own tooling.

## MCP

The `judge` tool of `ai-rulez mcp` runs the same engine for an agent; `mcp --read-only` does not register it. It takes
the review inputs (paths, categories, `fail_on`, `min_confidence`, `changed`, `since`, `impact`, the attach switches,
`skills` and `skill_for` as a `name`, a project `path` or `inline_markdown`, `explain_drops`, `allow_partial`,
`estimate`, `redact`) and no model, provider, price or spend cap. A real run needs `allow_llm: true` in the call and
`allow_llm = true` in the `[judge]` table of your user config; `estimate: true` always works and sends nothing.

## What is sent

File content up to the limits above, and for governance the attached conventions, goes to the model's provider.
Nothing else does: no keys, environment or configuration. The egress host is printed first. Under an organization
[policy](/ai-rulez/policy/) that forbids model use, only `--estimate` runs. The `--policy*` flags and `--discover-org` load
and verify such a policy for the run; see [Policy](/ai-rulez/policy/).

## Measured accuracy

Judge ships with a labeled synthetic corpus in
[`bench/judge/`](https://github.com/Goldziher/ai-rulez/tree/main/bench/judge) (malicious units for every security
code, benign lookalikes, false-positive traps, quality defects, a governance fixture with nested rules). On a 67-unit
subset with the default models, at about $0.075 per unit:

| Category | Precision | Recall | F1 |
| --- | --- | --- | --- |
| security | 1.00 | 0.85 | 0.92 |
| governance | 1.00 | 0.86 | 0.92 |
| quality | 0.89 | 0.89 | 0.89 |

The corpus is small and synthetic: read the numbers as a baseline, not a field measurement. Some malicious samples are
declined by the provider's content filter; judge reports such units as partly reviewed, never as a pass.
