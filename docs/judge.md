# Judge

`ai-rulez judge` reviews agent configuration with language models: skills (with their scripts and references), agents,
slash commands, rules, `CLAUDE.md` and `AGENTS.md` files, MCP and settings files. Three reviewers read each unit, a
mechanical verifier checks that every finding quotes the file, an independent adjudicator decides what survives, and the
scores and the verdict are computed in code. It needs no configuration file.

```console
ai-rulez judge --estimate                      # what would be sent, to whom, and what it would cost; sends nothing
ai-rulez judge --allow-llm                     # review everything the harnesses read below this directory
ai-rulez judge .claude/skills/deploy --allow-llm --fail-on security:medium,governance:high
ai-rulez judge --changed=origin/main --allow-llm --format sarif > judge.sarif
ai-rulez judge --allow-llm --skill path:rubrics/acme --skill-for quality=style-rubric
```

The deterministic checks are [`validate` and `scan`](strict-validation.md): free, fast and exact on what they know. Judge
is for what rules cannot see: an instruction that tells an agent to hide a step from the user, a skill whose scripts do more
than its description says, a rule that contradicts the `CLAUDE.md` above it. For dimension-by-dimension scoring of skill
descriptions with calibration and a gate, see [Review](review.md); for the shared model access, see
[LLM access](llm.md).

## How a review works

1. **Units.** The files become units: a skill is its directory, anything else is one file. Discovery follows the
   conventions of the harnesses, `.claude/` directories at any depth included.
2. **Static facts.** `validate` and `scan` run over the units. Their findings reach the reviewers as numbered leads
   (`F1`, `F2`, ... at most 64 per unit). A fact is never a finding on its own: a reviewer has to verify it against the text.
3. **Reviewers.** One call per category and unit (more for a large unit, see [Large units](#large-units)): **security**
   (`JR-S01` to `JR-S15`), **governance** (`JR-G01` to `JR-G07`) and **quality** (`JR-Q01` to `JR-Q13`). Each has its own
   prompt and output schema. They are asked to separate a *threat* (an active instruction, executable code or a capability
   grant) from *context* (a prohibition, documentation, an example, a fixture), and to write the strongest innocent reading
   of every finding next to the reason it fails.
4. **Verifier.** Every finding carries a verbatim quote and a line range. Go checks the quote against the text that was
   really sent, after masking: whitespace is normalized, nothing else. A finding that fails goes back to the reviewer once,
   with the exact text at the cited lines; one that still fails is deleted and counted (`summary.grounding`). A chain of
   steps (an auto-run instruction, the script it runs) keeps only the steps whose quote verifies.
5. **Adjudicator.** A separate call sees the surviving candidates, a window of eight lines around each cited range, the
   static facts, the conventions, and a list of known false-positive shapes. For every candidate it decides `keep`,
   `downgrade`, `drop` or `merge`, and may link candidates into a chain or raise up to three findings of its own.
6. **Scoring.** Code, not a model, computes the scores, the unit status and the decision.

The adjudicator is asymmetric on purpose. A **security** candidate is dropped only with a concrete benign context (a
prohibition, a documented example, a placeholder, a declared service, a static literal, wording about a lint option) and a
counter-quote taken verbatim from the window; without both, ai-rulez keeps the finding and says why in the unit's notes.
**Quality** and **governance** candidates are dropped unless they show a concrete defect or a violated binding convention.
Findings that come from the static analyzers alone (hidden characters in an instruction file, `AR002`) are reported
without the adjudicator: a model cannot talk them away. `--explain-drops` lists every dropped candidate with the stage, the
reason code and the counter-quote.

A model never emits a score or a verdict, and text in a reviewed file cannot ask for one: the files are inert data (see
[Trust tiers](#trust-tiers)).

## What it judges

With no paths, judge takes every unit below the working directory that a harness reads:

| Unit kind | Where |
| --------- | ----- |
| `skill` | a directory with a `SKILL.md`, with every file below it (scripts, references) |
| `agent`, `command`, `rule` | `agents/`, `commands/`, `rules/` below a `.claude/`, `.agents/`, `.ai-rulez/`, `.cursor/` or `.github/` directory, at any depth |
| `claude-md`, `agents-md` | `CLAUDE.md`, `CLAUDE.local.md`, `AGENTS.md`, `AGENTS.local.md`, at any depth |
| `mcp-config` | `.mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json` |
| `settings` | `.claude/settings.json`, `.claude/settings.local.json` |
| `plugin` | `.claude-plugin/plugin.json`, `marketplace.json` |

Name files or directories to judge those instead; any Markdown or JSON file you name is a unit (kind `other` when no
convention applies), and a file inside a skill selects the whole skill. A file that carries the ai-rulez generated header
is skipped: judge its sources.

- It needs a project: run it inside a git repository, or a directory with its own `AGENTS.md`, `CLAUDE.md`, `.claude/`,
  `.agents/` or ai-rulez configuration. In `$HOME` or `/` it refuses. Symlinks are never followed (a named path that is a symlink, or lies below one, is refused), paths must stay inside
  the project, and one run takes at most 200 units (`--estimate` is not limited), files up to 2 MiB, units up to 40 files and 512 KiB.
- **`--changed[=REV]`** judges only the units that changed since `REV` (committed, staged, unstaged and untracked work);
  a bare **`--changed`** compares with `HEAD`, as in `validate` and `scan`. Write the base with `=` (`--changed=origin/main`):
  a separate word stays a path to judge. **`--since REV`** is the same as `--changed=REV` and stays supported. With **`--impact`** (on by default; `--impact=false` turns it off) it also judges the units a changed rule
  or `CLAUDE.md` is attached to, because a model has to see them again though they did not change.

## Governance knows your repository

The governance reviewer enforces only what is written down, and it is shown exactly what applies to the unit, resolved the
way Claude Code resolves it:

- **Path-scoped rules.** For every ancestor directory `D` of a file of the unit, the Markdown files below
  `D/.claude/rules/` (at any depth) are candidates. A rule with a `paths:` list attaches when one of its globs, taken
  relative to `D`, matches a file of the unit (`matches-file`) or a path the unit mentions (`matches-referenced-path`). A
  rule without `paths:` attaches always (`unconditional`; one over 6,000 tokens is sent as an index of its headings). Globs
  support `**`, `*`, `?`, `[abc]` and `{a,b}`, are anchored at `D` (`*.py` matches only directly in `D`), and have no
  negation. A rule never attaches to itself; a unit that is a rule also gets the rules whose scope overlaps its own
  (`overlapping-scope`), which is how scope mismatches and conflicting conventions are found.
- **Nested instruction files.** `CLAUDE.md`, `CLAUDE.local.md`, `AGENTS.md` and `.claude/CLAUDE.md` of every ancestor
  directory, root first and nearest last (`ancestor-instructions`), with the files they `@import` to a depth of five
  (`imported`). The unit's own files are excluded.
- **Binding versus guidance.** `MUST`, `NEVER`, `ALWAYS`, "required" and "forbidden" are binding; `SHOULD`, "prefer" and
  "recommended" are guidance, and a deviation from guidance is at most `low`.
- **Two quotes.** A governance finding quotes the file and the convention line it rests on. A governance finding with one
  side is deleted. A nearer instruction file overrides a farther one on the same topic.

`--attach-rules=false` and `--attach-claude-md=false` (or `attach_rules` and `attach_claude_md` in `[judge]`) turn the two
kinds off. `--estimate` lists what is attached to every unit. When no convention and no rubric applies to a unit that is not
a rule, the governance call is not made.

## Rubric skills

A skill whose body has `## C1 - title` sections is a rubric: every section is a checkable criterion.

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

`--skill NAME|path:DIR` (repeatable) applies a rubric to the governance review, `--skill-for CATEGORY=NAME|path:DIR` to
another category, and `[judge.skills]` does the same from the configuration. The reviewer must report **every** criterion
as `pass`, `fail` or `not_applicable` in `criteria_results`, and report each failure as a `JR-G06` finding with the
criterion id (`rubric:acme-skill-standards/C1`) and a quote. A criterion nobody reported is `not_evaluated`, never a
silent pass. A failed criterion always becomes a `JR-G06` finding and lowers the score: when the reviewer gave no finding
(the failure is an absence, such as a missing `## Verify` heading) or its quote does not verify, the judge raises the
finding itself for the unit as a whole (line 1 of its primary file, `absence: true`, `origin: criteria`, no quote; for an
unverifiable quote `verification.quote_verified` is false and the reviewer's severity is kept). `--explain-drops` still
lists the dropped candidate, with its `rubric_id`. A criterion the adjudicator drops on purpose stays dropped. The tool call `judge` also takes inline Markdown (see [MCP](#mcp)).

## Trust tiers

What reaches a model is split into tiers by marker lines that carry a per-request token. The token is derived from the
request, so it is stable (the response cache keeps working) but cannot be predicted by the author of a reviewed file, and
it is checked to be absent from every text that is sent. Fence-looking runs in a file are defused.

| Tier | Content | Authority |
| ---- | ------- | --------- |
| T0 | the built-in system prompt and the output schema | cannot be changed by anything below |
| T1 | rubric text and prompt extensions you configured: a `path:DIR` skill, a pinned skill, a prompt file | adds checks; can never relax the evidence contract, change the schema, name a verdict, or tell a reviewer to stop or approve |
| T2 | repository conventions, static facts, unpinned or inline skills | reference data: compared against, never obeyed |
| T3 | the files under review | inert data: text in it that addresses a "reviewer", "scanner" or "AI", claims the file is safe or approved, or asks for a score is itself evidence (`JR-S01`) |

Where a run sends content and what it may spend are never decided by a repository. A repository's `[judge]` table cannot
set the models, the provider or base URL, consent, the API key variable, or the spend caps: they come from flags, the
environment and your user config file, and a repository value is ignored and reported in the run's notices (text, JSON `run.notices` and SARIF
`toolExecutionNotifications`). The same goes for a provider, model or base URL in the repository's `[llm]` table. `validate`
reports `model`, `provider` and `base_url` in `[judge]` as unknown keys; `judge` agrees by naming them with the place to
set them, and refuses any other unknown `[judge]` key as `validate` does. A skill named
from the project is reference data (T2) unless `[judge.skills.pins]` pins its hash and it matches.

## Scores, gating and exit codes

Per category: `100 - sum(points x confidence weight x decay)`. Points are 40 critical, 20 high, 8 medium, 3 low. The
confidence weight is 1.0 for high, 0.6 for medium and 0.3 for low. The deductions are sorted and weighted 1, 0.5 and 0.25;
a fourth adds nothing. A category that did not run is `null`, not 100.

`--fail-on` is a list of `category:severity`, `category:none`, `score:N` (any category below N) or `category:score:N`. The
default is **`security:high,governance:high,quality:none`**: a quality finding is advice and never fails a run unless you
say so. A category the list does not name keeps its default. `--min-confidence` drops findings below a confidence.

A unit is `fail` (a `--fail-on` rule is met), `partial` (not fully reviewed), `warn` (findings below the gate), `pass`,
`withheld_secret`, `skipped` or `error`. The run decision is `fail`, `incomplete` or `pass`.

| Exit code | Meaning |
| --------- | ------- |
| `0` | every unit passed or warned (for `--estimate`: the estimate is within the spend cap) |
| `1` | the run could not complete: the provider refused, a cap was reached, a model failed, nothing was left to judge, or a unit was only partly reviewed (`--allow-partial` accepts that); for `--estimate`, the estimate exceeds `--max-cost` |
| `2` | a unit met `--fail-on`, or a unit was withheld because it holds a secret |

### Honest coverage

Nothing is silently left out. Every unit reports what was and was not reviewed: files that were not sent (binary,
unreadable, over a limit), chunks beyond the limit, calls the caps left unmade, a provider that refused, a reply that was
never valid. Those make the unit `partial` and the run `incomplete`; `coverage.not_reviewed` lists the spans and `gaps`
says why. A unit holding a credential that the security scan (`AR001`) recognises is **withheld** whole (nothing of it is sent) and
reported as a finding naming `file:line` and the pattern (never the secret); the judge and `validate` share one detector,
so code that merely names a secret (`token = credentials["SessionToken"]`) is reviewed. `--redact` masks the text and
sends it instead.

Repository-configured rubric paths and trap files must stay inside the project, including through
symlinks. An explicit CLI `--skill path:DIR` may name an external rubric. Rubrics, prompt overrides
and trap text are redacted before they are sent to a model, including configured credential values.

### Large units

A unit over the per-call budget (24,000 input tokens of file text) is split: whole files are packed together and a file
that is too big is cut into windows that overlap by 40 lines, at most eight chunks per category. Every chunk also carries a
deterministic digest of the lines of the other chunks that touch the network, a shell, credentials or the agent's
instructions, so an attack whose source and sink sit in different chunks stays visible. Quotes are verified against the
whole file, whichever chunk produced them.

## Models and providers

| Role | Default |
| ---- | ------- |
| reviewers (security, quality, governance) | `bedrock/global.anthropic.claude-sonnet-5-5` |
| adjudicator | `bedrock/global.anthropic.claude-opus-5-5` |

Set them with `--model` (all reviewers; the adjudicator follows unless named), `--adjudicator-model`,
`--model-for ROLE=MODEL`, `AI_RULEZ_LLM_MODEL`, the `[llm]` table of your user config, or `[judge.models]` there. All roles
use one provider and one credential. The egress host and the models are printed before anything is sent.

- **Amazon Bedrock** is called through the AWS credential chain (environment, `AWS_PROFILE` and shared config including
  SSO and assume-role, web identity, container and instance roles). The credentials are refreshed before they expire and
  once more if a call is refused as unauthenticated, so a long run does not fail halfway. The region comes from
  `AWS_REGION`, `AWS_DEFAULT_REGION` or the profile. Inference-profile ids (`global.anthropic.claude-...`) are priced.
- **Anthropic** and **OpenAI-compatible** endpoints (`--model openai/gpt-4o-mini`, `AI_RULEZ_LLM_BASE_URL`) work the same.
- **Structured output.** Every call asks for JSON that matches a schema limited to the subset all providers accept (no
  `minItems`, `maxItems`, `pattern`, `uniqueItems`; those limits are enforced in Go), and the reply is validated strictly in
  Go whichever way the provider enforced the schema.
- **Temperature** is omitted for Claude 5.x models, which reject it. The report records it (`provenance.temperature` is
  `null`). Thinking tokens spend the same completion budget, so each call gets 6,144 tokens.
- A reviewer whose provider stops a reply for its content policy without a usable answer is retried once, then reported
  as a refusal if it still cannot answer. A reply that is whole and valid is used whatever stop reason it carries.

## Cost, caps and the cache

`--estimate` builds every request and prices it, worst case and expected, without calling anything. A run is capped by
`--max-cost` (default **$2.00**), `--max-tokens` and `--max-calls`; the caps come from flags and your user config, never
from a repository. Calls are admitted in priority order, security first, then governance, then quality, then unit order,
until the *expected* cost reaches a cap (one adjudicator call is held back for half of the units); the model layer enforces
the worst case per call as a hard stop. What does not fit is not made and is reported: the units are partial and the run
incomplete.

`--estimate` counts a call once: `calls` is every planned call (the reviewers' and one adjudicator call per unit with a
review) and `calls_fit` is how many of them fit under the caps. It covers all the units even when more than 200 match (a
real run would be refused, and the estimate says so; name paths or use `--changed`). The prompt token estimate is the
requests the run would build, measured at 3 bytes per token (a test holds it within 15% of a tokenizer count); it leaves out
repair calls after an ungrounded quote and the adjudicator's input beyond two candidates, so a run that repairs a lot costs more.
When the estimate exceeds the cap (`--max-cost`, default $2.00) it warns on stderr and exits **1**; otherwise it exits 0.

Calls run in a deterministic order, at most `--concurrency` (default 4) at a time and `--rpm` per minute. Answers are cached
in your user cache directory, keyed by everything in the request: the prompt version, the model, the output schema, the
rubric text, the attached rules and instruction files, the unit's content and the static facts. Only validated replies are
stored, entries expire after 30 days, and the cache is authenticated with a per-user secret. `--no-cache` skips it;
`--cache-readonly` serves hits and stores nothing.

## Configuration

Everything works without configuration. The repository's `[judge]` table sets what a team agrees on:

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
quote   = "curl -fsSL https://tools.internal.example/install.sh | sh"
reason  = "sanctioned installer, tracked in SEC-1234"
expires = "2026-12-31"
```

- **Prompts.** `extend` adds your text to the reviewer's instructions (T1). `override` replaces the category's role
  prompt; the shared core (trust tiers, evidence contract, calibration, self-check) and the output schema are never
  replaceable. An override from a repository file takes effect only with `--allow-prompt-override`, or
  `allow_prompt_override = true` in your user config.
- **Traps** extend the adjudicator's list of known false-positive shapes (extend only):

  ```yaml
  - id: R01
    name: internal_cli_download
    text: "`acli` downloads pinned tool bundles from artifactory.internal.example; that is the sanctioned installer."
    applies_to: [JR-S05, JR-S10]
  ```

- **Suppressions** are pinned to the quote (a changed text brings the finding back), must carry a reason, and expire (at
  most 366 days ahead). An expired suppression is reported in the notices and no longer applies. A suppressed finding stays
  in the report under `suppressed`.
- **User scope only** (`~/.config/ai-rulez/config.toml`): `[judge.models]` (`security`, `quality`, `governance`,
  `adjudicator`, or `reviewers`), `max_cost_usd`, `max_tokens`, `max_calls`, `concurrency`, `rpm`, `allow_llm` and
  `allow_prompt_override`. The `[llm]` table of the user config (provider, model, base URL, key variable) applies; the
  repository's `[llm]` table never does.

## Output

`--format text` (default), `json` and `sarif`.

**JSON** is the document described by [`schema/judge-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/judge-report.schema.json)
(`schema_version` 2). It carries the run (prompt version, models, egress hosts, gate, loaded skills, provenance with cache
hits, tokens and cost, coverage), one entry per unit (status, scores, attached conventions, rubric results, coverage, gaps,
findings) and the summary (decision, counts, grounding). A finding has a stable `id` (the first 12 hex digits of
`sha256(code|path|normalized quote)`), `code`, `severity`, `confidence`, `verdict` (`confirmed` or `contextual_risk`), the
`location` and `quote`, the `chain` of steps, `benign_reading`, `why_not_benign`, the `convention` it rests on, a
`suggested_fix`, its `origin` (`reviewer`, `adjudicator` or `static-only`), what the adjudicator did, how it was verified
and a `fingerprint`. It never contains file text beyond the quotes of findings, and never the prompts.

**SARIF** maps `ai-rulez/JR-xxx` to rules; critical and high are `error`, medium `warning`, low and info `note`;
`security-severity` is 9.5, 8.0, 5.5, 3.0 and 1.0; chains become `codeFlows`, the convention a `relatedLocations` entry,
the suggested fix is appended to the result message and kept in `properties.suggestedFix` (it is prose, and a SARIF `fix`
needs `artifactChanges` with replacement text, so none is emitted), and `partialFingerprints` carry the fingerprint.
Coverage gaps and withheld units are tool execution notifications, and `executionSuccessful` is false for an incomplete run.

## MCP

The `judge` tool runs the same engine for an agent. `ai-rulez mcp --read-only` does not register it: it sends content to a model provider and spends money. It takes the review inputs (paths, categories, `fail_on`,
`min_confidence`, `changed` (a base revision; `HEAD` for uncommitted work, `true` is accepted as `HEAD`), `since`, `impact`, the attach switches, `skills` and `skill_for` as objects with a `name`, a `path`
inside the project or `inline_markdown`, `explain_drops`, `allow_partial`, `estimate`, `redact`) and **no model, provider,
price or spend cap**: those are yours. A tool client has no consent prompt, so a real run needs `allow_llm: true` in the
call **and** `allow_llm = true` in the `[judge]` table of your user config; `estimate: true` always works and sends nothing.
Rubric skills from a tool call are reference data (T2). A failed or incomplete run is an error result that still carries
the report.

## The benchmark

Judge ships with its own evaluation in [`bench/judge/`](https://github.com/Goldziher/ai-rulez/tree/main/bench/judge): a
synthetic corpus (malicious units for every security code, benign lookalikes, the false-positive traps with a malicious
twin each, quality defects, a governance fixture repository with nested rules and `CLAUDE.md` files, clean units) with
labels. `go test ./bench/judge` runs the verifier and the attachment algorithm over it without a model. `task bench:judge`
runs the model pipeline on Bedrock and prints precision, recall and F1 per code and category, the trap false-positive rate,
the share of malicious twins found, grounding, the adjudicator's effect and the cost. Run it again when the prompt version
(`judge/v3`) changes.

A first run (2026-10-15, prompt `judge/v3`, Bedrock `eu-north-1`, reviewers Claude Sonnet 5.5, adjudicator Claude Opus 5.5) on a
67-unit subset of the corpus (both a malicious sample and a benign lookalike for every security code, five trap and twin
pairs, nine quality defects, four clean units, the governance fixture) cost $5.04 for 211 calls ($0.075 per unit with the
retries and the adjudicator):

| | precision | recall | F1 |
| --- | --- | --- | --- |
| security | 1.00 | 0.85 | 0.92 |
| governance | 1.00 | 0.86 | 0.92 |
| quality | 0.89 | 0.89 | 0.89 |

No trap unit (a benign lookalike of a known false-positive shape) produced a security finding, and four of the five malicious
twins were found. No candidate was dropped as ungrounded (one needed the repair round); the adjudicator raised security precision from 0.94 to 1.00. Three malicious samples (an exfiltration of `~/.ssh`, a
disk-filling loop, a vendor health probe with a hidden upload) were still declined by the provider's content filter after
one retry: judge reports those units as partly reviewed (the run is `incomplete`, exit 1), never as a pass. On the 46
sampled units that hold a skill, the offline modes of two open-source scanners, with their categories mapped onto the
security codes and medium severity or above counted, scored F1 0.47 (Cisco skill-scanner: precision 1.00, recall 0.31) and
0.40 (SkillSpector: precision 0.42, recall 0.38) against 0.87 for judge (precision 1.00, recall 0.77). The corpus is small and
written by the same effort that wrote the prompts; read the numbers as a regression baseline, not as a field measurement.

## What is sent

File content, up to the limits above, goes to the model's provider; nothing else does (no keys, no environment, no
configuration). The egress host is printed first. A file with secret-looking text, or holding the value of a configured
API key, is withheld or masked. Everything printed is cleaned of control characters and secret-looking text. Under an
organization policy that forbids model use, only `--estimate` runs. Finding text is model output: treat it as untrusted
text in your own tooling.
