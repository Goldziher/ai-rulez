# Session exposure and areas

A large repository has parts that belong to different teams: services, products, shared tooling. Context loaded at
the start of every session (the root `CLAUDE.md`, unconditional rules, the skill listing) should not name one of
those parts unless every session needs it, and a skill that belongs to one part should say so. The `[exposure]` table
describes the parts as an **area map**: which directories each area claims, who owns it, and the words that name it.

This page describes the table, how the map is matched, and the commands that show what a Claude Code session loads,
what that costs at the start of every session, and where each artifact belongs. `validate` checks the map itself
(`AR9W5`), area terms in the text a session sees at its start (`AR9W0`, `AR9W1`, see
[Area term leakage](#area-term-leakage)), `skill_areas` entries that name no skill (`AR9W6`), the start-of-session
budget (`AR9W3`), the skill listing budget (`AR964`), a frontmatter gate that cannot be read (`AR9W4`) and, when
`collision_min_score` is set, listings of different areas that overlap (`AR9W2`). Without an `[exposure]` table no
rule on this page has any effect except `AR9W4`, which needs none; the commands work without one.

- [Configuration](#configuration)
- [The area map](#the-area-map)
- [Matching terms](#matching-terms)
- [Classifying an artifact](#classifying-an-artifact)
- [The owner of a skill](#the-owner-of-a-skill)
- [Commands](#commands)
- [A gate that cannot be read](#a-gate-that-cannot-be-read)
- [Start-of-session budgets](#start-of-session-budgets)
- [Area term leakage](#area-term-leakage)
- [Validation](#validation)

## Configuration

```toml
[exposure]
harness = "claude"                 # only "claude"
context_window = 200000            # tokens (default 200000)
plugins = "enabled"                # none | enabled (default) | all: plugin skills counted at start
areas_file = "tools/context/areas.toml"   # optional: the map in its own file
start_budget = 0                   # most estimated start tokens; 0 (default) is off
collision_min_score = 0            # AR9W2 word overlap between areas; 0 (default) is off

[[exposure.areas]]
name = "billing"                   # unique, lower-kebab
description = "the billing service and its jobs"   # finishes "I work on ..."
owner = "team-billing"             # free text, never parsed
paths = ["services/billing_*", "services/invoices", "!services/billing_legacy"]
terms = ["invoice", "ledger_sync"]
ambiguous = ["account"]
shared = false                     # repository-wide tooling: terms classify, never leak
declare = "services/billing/"      # optional: what a session of this area declares it works on

[exposure.skill_areas]             # explicit owners; win over path claims and terms
deploy-staging = "platform"
```

Every key is optional. `plugins`, `context_window` and `start_budget` are read by `ai-rulez exposure` and by
`validate` (see [Start-of-session budgets](#start-of-session-budgets)); `collision_min_score` turns on `AR9W2` and is
the default `--min-score` of `exposure collisions`; `plugins` is also the default `--plugins` of the subcommands.
`declare` is read by context evals, which arrive in a later release, and is validated now so a table written today
keeps working.

The map is either inline (`[[exposure.areas]]` and `[exposure.skill_areas]`) or in `areas_file`, never both. The
file is resolved from the project root, must stay inside it, and holds the same fields under shorter names:

```toml
# tools/context/areas.toml
[[area]]
name = "billing"
owner = "team-billing"
paths = ["services/billing_*"]
terms = ["invoice"]

[[area]]
name = "platform"
paths = ["infra", "tools"]
shared = true
terms = ["terraform"]

[skill_areas]
deploy-staging = "platform"
```

A key the file does not define is an error, not ignored, so a misspelled key is reported.

## The area map

- **`name`** is unique and lower-kebab (`a-z`, `0-9`, single hyphens). `unclassified` is reserved: it is the area of
  anything the map does not place.
- **`paths`** are repository-relative directory prefixes with no leading or trailing slash. `services/invoices`
  claims that directory and everything below it, not `services/invoices2`. A trailing `*` makes a name prefix:
  `services/billing_*` claims `services/billing_api` and `services/billing_jobs/x`, and `vendor/*` claims everything
  below `vendor` but not `vendor` itself. A `!` path excludes: an area
  never claims a directory one of its own excludes cover. When several areas claim a directory, the longest
  matching path wins, and the first area in the map wins a tie.
- **`terms`** are the words that name the area. They are trimmed, lowercased and deduplicated. A word may be a term
  of several areas.
- **`ambiguous`** are words the area uses in its own sense that other areas also use (`account`, `query`). They never
  decide an area on their own. A word that is both a term and ambiguous in one area is a term.
- **`shared`** marks repository-wide tooling (a build system, CI): its terms still classify artifacts, but naming
  them at session start is not leakage.
- **`owner`** and **`description`** are reported as they are.

## Matching terms

Terms are matched case-insensitively:

- A term that contains `_`, `/` or `.` (`ledger_sync`, `@acme/ui`, `api.v2`) matches anywhere, as a substring.
- Any other term matches only as a whole word: the characters before and after it must not be ASCII letters or
  digits. `invoice` matches `Invoice`, `INVOICE_ID` and `team_invoice`, but not `invoices` or `reinvoice`.
- Longer terms are tried first, so `ledger_sync` wins over `ledger` where both match.
- Repository path literals are blanked out first: a path claim's first segment followed by `/`
  (`services/billing_api/README.md`) up to the next space, quote, backtick, bracket, comma or semicolon. Naming a
  location is not naming an area. A path is not a literal when a letter, digit, `_`, `/`, `.`, `@` or `-` precedes it.

Case folding follows Python's `re.IGNORECASE` on str patterns (CPython 3.11, Unicode 14.0): `K` (U+212A, the Kelvin
sign) matches `k`, a final `ς` is the lowercase of `Σ`. A test holds the matcher to recorded vectors. A character
that is unassigned in Unicode 14.0 may match differently.

## Classifying an artifact

An artifact (a skill, a rule, a `CLAUDE.md`) in a directory, with a text, belongs to:

1. the area with the most specific path claim on the directory;
2. otherwise, the area with the most distinct terms in the text (ambiguous words do not count); a tie gives none;
3. otherwise `unclassified`.

## The owner of a skill

The owner of a skill is resolved in this order, and reported with its source:

| Source | When |
| --- | --- |
| `skill_areas` | `[skill_areas]` names the skill. A plugin skill is looked up as `plugin:skill`, then by its bare name |
| `path` | an area claims the skill's directory: for a repository skill, the directory that holds its `.claude` directory; for a plugin skill, the plugin's directory |
| `terms` | the terms of the skill's text decide (see above): a repository skill's listing text (`name: description`); a plugin skill's qualified name (`plugin:skill`), never its description |
| `none` | nothing does: the owner is `unclassified` |

A plugin skill is listed at the repository root, but it belongs where its plugin lives: a plugin under
`tools/plugins/ops` is owned by the area that claims `tools`, whatever its skills' descriptions say. Place a plugin
outside every claim with `[skill_areas]`.

An explicit entry is the way to place a skill that still lives at the repository root: once it moves under the
area's directory, the path claim gives the same answer and the entry can go.

## Commands

The commands are offline, free and deterministic. They inspect the project root (the directory of the
configuration, else the working directory) or `--root DIR`, and read the area map of the `[exposure]` table or of
`--areas FILE`, a file in the `areas_file` format that replaces the configured map, so a map can be tried before
it is configured.

`--root DIR` inspects what a Claude Code session started in `DIR` sees, for every command, `exposure leakage`
included: the inventory is of `DIR`, and the paths the commands print are relative to it. The
configuration is still `-C/--config` when it is given (one that cannot be read or is invalid fails the command),
else the one `--config-dir` names in `DIR`, else the one discovered from `DIR` upwards, so `--root` on a
subdirectory of a project uses the project's map; `--no-local` applies either way. The `areas_file` of the
configuration and the path claims of the map stay relative to the configuration's project root: below it, `DIR`'s
path is put in front of every scope before the claims are tried, so `--root services/billing` matches the claim
`services/billing` for what lies at `DIR` itself. A `DIR` outside the project root is matched as its own root.

Exit codes: `0` the report was printed, `1` the command could not run (a bad flag, an invalid configuration or area
map, whose problems are listed with the file and line `AR9W5` reports; a problem of the inline map whose entry
cannot be found names `[exposure]`), `2` (`exposure` only) the start tokens exceed `[exposure] start_budget`;
`exposure leakage` exits `2` on a violation or a stale acceptance.

| Flag | Commands | Meaning |
| --- | --- | --- |
| `--format text\|json` | all | Output format |
| `--plugins none\|enabled\|all` | all | Marketplace plugins whose skills are listed: none, the ones the root `.claude/settings*.json` enable, or all; default `[exposure] plugins`, else `enabled` |
| `--areas FILE` | all | Area map file to use instead of the `[exposure]` table |
| `--root DIR` | all | Directory to inspect, as a session started there sees it; default the project root, else the working directory |
| `--context-window N` | `exposure` | Model context window in tokens, for the skill listing budget; default `[exposure] context_window`, else 200000 |
| `--harness claude` | `exposure` | Harness whose load model applies; only `claude` |
| `--with-text` | `inventory` | Include each artifact's text in the JSON (`--format json` only; with text output it is an error) |
| `--min-score N` | `collisions` | Lowest score reported, 0 to 1; default `[exposure] collision_min_score` when set, else 0.1 |
| `--limit N` | `collisions` | Pairs printed, highest score first (default 20); 0 prints every pair |

### `ai-rulez exposure`

Without a subcommand, `exposure` reports what a session started at the repository root costs before it touches a
file, and what waits:

- **Start tokens:** the estimated tokens of every start-of-session artifact of the [inventory](#ai-rulez-exposure-inventory)
  (the same estimate as its `tokens`), in total and by kind. A root rule or skill of phase `unknown` counts as start:
  a reader that guards the start of a session fails closed.
- **The skill listing:** the start-of-session skills (plugin skills included, as `--plugins` selects them), the
  characters of their listing lines, and Claude Code's budget for the listing: 1% of the context window, counted at
  4 characters per token, so 8000 characters at 200000 tokens. Over the budget Claude Code keeps every skill name but
  drops descriptions, starting with the skills invoked least, and the descriptions carry the words a request is
  matched against. `descriptions_capped` counts the listed skills whose description and `when_to_use` pass the
  per-skill cap of 1536 characters.
- **Phases:** the artifacts per phase and kind, and the tokens each phase would add if all of it loaded.
- **The heaviest** five artifacts at start (text only).

The JSON follows [`schema/exposure-report.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-report.v1.schema.json).
`items` lists every artifact of the inventory with its qualified name, phase and tokens, and its `area` when a map is
configured or given:

```json
{
  "schema_version": 1,
  "harness": "claude",
  "root": "/work/repo",
  "plugins": {"mode": "enabled", "marketplace": "tools", "available": [{"name": "ops", "path": "tools/plugins/ops"}], "enabled": ["ops"], "loaded": ["ops"]},
  "start_budget": 6000,
  "listing": {"context_window": 200000, "skills": 24, "chars": 8210, "budget_chars": 8000, "over_budget_chars": 210, "descriptions_capped": 1},
  "start_tokens": 3120,
  "tokens_by_phase": {"start": 3120, "on_demand": 48210, "manual": 940},
  "start_tokens_by_kind": {"skill": 1650, "claude_md": 1020, "agent": 440, "mcp_server": 10, "settings": 0},
  "counts_by_phase_and_kind": {"start": {"skill": 24, "agent": 5, "mcp_server": 3, "claude_md": 1, "settings": 1},
                               "on_demand": {"skill": 60, "rule": 40, "claude_md": 20}, "manual": {"skill": 12}},
  "items": [
    {"kind": "skill", "name": "ops:probe", "path": "tools/plugins/ops/skills/probe/SKILL.md", "scope": ".",
     "phase": "start", "tokens": 8}
  ]
}
```

`counts_by_phase_and_kind` always holds `start`, `on_demand` and `manual`, and `unknown` when an artifact has that
phase; `tokens_by_phase` holds the phases that have artifacts.

### `ai-rulez exposure inventory`

Every native Claude Code artifact a session started at the repository root can see. The load model (what loads at
start, on demand and manually) was measured on Claude Code 2.1.306; how Claude Code reads a gate it cannot parse was
measured separately, on 2.1.312 (see [A gate that cannot be read](#a-gate-that-cannot-be-read)):

- **Start of the session:** the root `CLAUDE.md` (and `.claude/CLAUDE.md`), unconditional root rules, the listing of
  root skills (`.claude/skills/<name>/SKILL.md`, and `.claude/commands/**/*.md` as `a:b` skills), root agents
  (`.claude/agents/<name>.md`), the server names of the root `.mcp.json`, and the enabled plugins with their skills
  (`plugin:skill`).
- **On demand:** anything in a subdirectory, and anything gated by `paths:`, once a matching file is touched.
- **Manual:** `disable-model-invocation: true` or `skillOverrides` `user-invocable-only`: only the user runs it.
- **Unknown:** a root rule or skill whose frontmatter does not parse while it names a gate (`paths:`,
  `disable-model-invocation`, `user-invocable`), or whose gate value has the wrong type, or one that was not read
  (over the read limit, or a symlink to outside the root; `frontmatter.ok` is false and the error starts with
  `not read:`); whether it loads at start cannot be known. A start count includes it (fail closed), as the text
  summary does (see [A gate that cannot be read](#a-gate-that-cannot-be-read)).
- `skillOverrides` `off` removes a skill and `name-only` blanks its description; `.claude/settings.local.json`
  applies over `.claude/settings.json`; `claudeMdExcludes` globs exclude `CLAUDE.md` files and rules. An enabled
  plugin is listed from `.claude/settings.json` when that file enables it, even if `.claude/settings.local.json` does
  too, and from `.claude/settings.local.json` only when that file alone enables it.
- The listing text of a skill is `name: description when_to_use`, whitespace squashed, capped at 1536 characters.
  `tokens` estimates every text as its UTF-8 bytes divided by 3.9, rounded up (0 for an empty text). A file over
  the 1 MiB read limit is kept unread with a problem; its `text_bytes` is the file size and `tokens` the estimate
  of that size, so a start budget still counts it.
- A UTF-8 byte order mark at the start of a file is stripped before the frontmatter is parsed: the frontmatter
  after it is read as usual, and the mark is not part of the text, `text_bytes` or `text_sha256`.

Not modeled: `@path` imports, `CLAUDE.local.md`, user and managed settings, symlinked directories, and the agents,
commands and MCP servers of plugins. A file the inventory cannot read is listed under `problems`.

The JSON follows [`schema/exposure-inventory.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-inventory.v1.schema.json).
With an area map every artifact carries `area` and `area_source`; the text of each artifact is included only with
`--with-text`:

```json
{
  "schema_version": 1,
  "harness": "claude",
  "root": "/work/repo",
  "settings": {"skill_overrides": {}, "claude_md_excludes": [], "enabled_plugins": ["ops@tools"]},
  "plugins": {"mode": "enabled", "marketplace": "tools", "available": [{"name": "ops", "path": "tools/plugins/ops"}], "enabled": ["ops"], "loaded": ["ops"]},
  "artifacts": [
    {"kind": "skill", "source": "plugin", "name": "probe", "qualified_name": "ops:probe", "plugin": "ops",
     "plugin_path": "tools/plugins/ops", "path": "tools/plugins/ops/skills/probe/SKILL.md", "scope": ".",
     "visibility": "always", "phase": "start", "paths": [], "listing_text": "ops:probe: Probes a service.",
     "listing_chars": 28, "truncated": false, "text_bytes": 28, "text_sha256": "…", "tokens": 8, "line_offset": 0,
     "frontmatter": {"ok": true}, "area": "platform", "area_source": "path"}
  ],
  "problems": []
}
```

### `ai-rulez exposure areas`

The area map as loaded (terms and ambiguous words normalized) and, for every artifact of the inventory, its area
and how it was decided: `skill_areas`, `path`, `terms` or `none`. `area_scope` is the directory whose path claims
were tried: the artifact's scope, or a plugin skill's plugin directory, relative to the project root of the map
(with `--root`'s path in front when `--root` lies below it). Diff the JSON against another classifier to
compare them. `areas_file` is the file the map came from, relative to `--root` with forward slashes, or absolute
(with forward slashes) when it lies outside the root; it is empty for the inline form. `counts` has every area
of the map and `unclassified`, 0 when no artifact is in it. `problems` repeats the inventory's: an artifact that
was not read is classified on an empty text. Without an area map the command fails. The JSON follows
[`schema/exposure-areas.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-areas.v1.schema.json):

```json
{
  "schema_version": 1,
  "root": "/work/repo",
  "areas_file": "tools/context/areas.toml",
  "plugins": "enabled",
  "areas": [{"name": "billing", "description": "", "owner": "team-billing", "paths": ["services/billing_*"],
             "terms": ["invoice"], "ambiguous": [], "shared": false, "declare": ""}],
  "skill_areas": {"deploy-staging": "platform"},
  "artifacts": [
    {"kind": "skill", "source": "plugin", "name": "probe", "qualified_name": "ops:probe", "plugin": "ops",
     "path": "tools/plugins/ops/skills/probe/SKILL.md", "scope": ".", "area_scope": "tools/plugins/ops",
     "phase": "start", "area": "platform", "area_source": "path"}
  ],
  "counts": {"billing": 4, "platform": 3, "unclassified": 12},
  "problems": []
}
```

### `ai-rulez exposure collisions`

The model chooses a skill or agent by its listing, so two listings that share most of their words compete for the
same prompts, and a session working on one area can load the instructions of another. The command compares every
skill and agent listed at the start of a session (phase `start`, and phase `unknown`, which counts as start here
as everywhere) with every other one of a different area, and ranks the pairs:

- **Words** of a listing are the runs of a lowercase ASCII letter followed by two or more lowercase ASCII letters or
  digits in its qualified name and listing text, lowercased first, less a fixed list of stopwords ("the", "use",
  "when", "task", "user", ...). `k8s` and `s3bucket` are words; `v2` and `3d` are not.
- **Areas** are the ones `exposure areas` reports: a skill's owner (`skill_areas`, then the path claim, then the
  terms), an agent's classification. `unclassified` counts as an area of its own, so an unclassified listing collides
  with a classified one but never with another unclassified one.
- **Score** is the Jaccard index: shared words over the words of either listing. A pair with no shared word is never
  reported, even at `--min-score 0`.
- **Order** is score, highest first, then the path of the first listing, then the second; the first of a pair is the
  one listed first in the inventory.

`total` counts every pair at or over the score; `collisions` keeps the first `--limit`. The command needs an area
map. The JSON follows
[`schema/exposure-collisions.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-collisions.v1.schema.json):

```json
{
  "schema_version": 1,
  "root": "/work/repo",
  "plugins": "enabled",
  "min_score": 0.1,
  "artifacts_compared": 3,
  "total": 1,
  "collisions": [
    {"score": 0.5, "shared": ["customer", "details", "look", "name"],
     "first": ".claude/skills/charge/SKILL.md", "first_kind": "skill", "first_name": "charge", "first_area": "billing",
     "second": ".claude/skills/rank/SKILL.md", "second_kind": "skill", "second_name": "rank", "second_area": "search"}
  ]
}
```

To separate two listings, lead each description with what sets it apart (the area's own nouns), not with the words
every skill of the repository uses.

### `ai-rulez exposure leakage`

The area terms in the text a session sees at its start, as `validate` reports them (`AR9W0`, `AR9W1`), each with its
acceptance, and the `[[lint.accept]]` entries of those rules that accepted nothing. It runs the `exposure` rules of
`validate` with the area map and plugin mode of the flags, so `[lint.severity]`, `[lint] ignore_paths`,
`[[lint.ignore_rules]]` and inline `ai-rulez-lint-ignore` comments apply as they do there; a baseline does not. A
finding is a `violation` when it is an error (`AR9W0` by default), a `warning` below that, and `accepted` when an entry
accepts it. With `--root DIR` the session is the one started in `DIR`: its `CLAUDE.md`, rules, skills, agents,
`.mcp.json` and settings, with paths relative to `DIR`, while the configuration, the map and `[[lint.accept]]` stay
the project's (an entry's `path` is relative to the project root, as in `validate`). An entry whose `path` matches
no file under `DIR` is not judged, so it is not reported as stale.

A start-of-session artifact whose file was not read (over the 1 MiB read limit, or a symlink to outside the root)
loads in every session, but no leak in it can be found. It is listed under `unchecked` with the reason, and as an
`unchecked:` line of the text output; it is not a finding and does not change the exit code. Read it, or bring it
under the limit, to have it checked.

The command fails (exit `2`) on a violation or a stale acceptance. Without an area map it fails (exit `1`). The JSON
follows
[`schema/exposure-leakage.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-leakage.v1.schema.json):

```json
{
  "schema_version": 1,
  "root": "/work/repo",
  "plugins": "enabled",
  "areas_file": "tools/context/areas.toml",
  "artifacts_checked": 14,
  "findings": [
    {"severity": "accepted", "code": "AR9W0", "area": "billing", "term": "invoice", "kind": "mcp_server",
     "name": "invoice-db", "path": ".mcp.json", "scope": ".", "line": 1, "excerpt": "invoice-db",
     "reason": "server names stay in the root .mcp.json"},
    {"severity": "violation", "code": "AR9W0", "area": "billing", "term": "invoice", "kind": "claude_md",
     "name": "CLAUDE.md", "path": "CLAUDE.md", "scope": ".", "line": 12,
     "excerpt": "Run the invoice reconciler before a release.", "reason": ""},
    {"severity": "warning", "code": "AR9W1", "area": "billing/search", "term": "account", "kind": "skill",
     "name": "lookup", "path": ".claude/skills/lookup/SKILL.md", "scope": ".", "line": 1,
     "excerpt": "lookup: Looks up an account.", "reason": ""}
  ],
  "unchecked": [
    {"path": ".claude/rules/generated.md", "kind": "rule", "name": "generated",
     "reason": "1500000 bytes, over the read limit of 1048576"}
  ],
  "stale_acceptances": [
    "lint.accept[2] (AR9W0 in CLAUDE.md) accepted no finding; remove it or fix its path or match"
  ]
}
```

## A gate that cannot be read

`paths:`, `disable-model-invocation` and `user-invocable` decide when Claude Code shows a skill or rule. When the
frontmatter around them is not valid YAML, what happens depends on Claude Code's own parser. Measured on Claude Code
2.1.312 with synthetic skills in a throwaway repository (the skill listing at the start of a `claude -p` session, then
after a file the `paths:` glob matches was read):

| Frontmatter of a skill with `paths: ["src/**"]` | Claude Code | ai-rulez reads |
|---|---|---|
| valid | hidden until `src/a.txt` is read | path gated, on demand |
| `description: Use when: testing` (an unquoted colon and space) | recovered: hidden until `src/a.txt` is read, the description kept | does not parse |
| `description: "Use when testing` (an unclosed quote) | dropped: listed at start, the description is the first body line | does not parse |
| `paths` indented with a tab | dropped: listed at start, the description is the first body line | does not parse |

So a broken block may keep its gate or lose it, and nothing short of Claude Code's parser tells which. A block that
parses but gives a gate a value of the wrong type leaves the same doubt: `paths` that is neither a string nor a list
of strings (`paths: 5`, a list holding a number, an empty `paths:`), or `disable-model-invocation` that is not `true`
or `false` (see [YAML 1.1 booleans](#yaml-11-booleans)). ai-rulez does not guess:

- `exposure inventory` gives such a root rule or skill the phase `unknown`, which the start-of-session reports count as
  start (fail closed).
- The MCP server never returns such a rule for a `find_context` `path` query, and a `scope` query lists it with
  `"paths_unreadable": true`; a rule or skill whose `paths` value has the wrong type is served the same way. A skill
  whose frontmatter does not parse is not served at all.
- `validate` reports it as `AR9W4 exposure-gate-unreadable` (warning), next to the `AR306` for a block that does not
  parse, for the claude preset and for a run without a configuration.

Quoting the value YAML cannot read (`description: "Use when: testing"`), and writing a gate as a string or a list of
strings (`paths`) or as `true` or `false`, fixes all three.

A known limitation: only `paths:` fails closed in the MCP server. A Cursor rule (`.cursor/rules/*.mdc`) whose broken
block names `globs:` (and not `alwaysApply: true`) is served as a rule that applies everywhere, as if it had no
globs; `AR306` reports the block.

### YAML 1.1 booleans

`disable-model-invocation` and `user-invocable` are booleans. YAML 1.1 also reads `yes`, `no`, `on` and `off` (and
`y`, `n`; lower case, capitalized or upper case) as booleans; YAML 1.2, and parsers that follow it, read them as
strings. Claude Code 2.1.312 follows YAML 1.1 here: a skill with `disable-model-invocation: yes` or `on` was kept out
of the model's listing and one with `no` was listed. ai-rulez parses YAML 1.2 (`gopkg.in/yaml.v3`), so it reads `yes`
as the string `"yes"`: `validate` reports the value as `AR304` (not a boolean) and as `AR9W4`, and `exposure
inventory` gives a root skill the phase `unknown` with the frontmatter error `"disable-model-invocation" is not a
boolean`. Write `true` and `false`, which every reader agrees on.

## Start-of-session budgets

Two checks of `validate` read the start-of-session report, both only with an `[exposure]` table, both computed as
`ai-rulez exposure` computes them with `[exposure] plugins` and `context_window`:

- **`AR9W3 exposure-start-budget`** reports start tokens over `[exposure] start_budget`. It is off while
  `start_budget` is 0 (the default); setting it turns the rule on at error. `ai-rulez exposure` exits `2` in the same
  case, so either can gate CI.
- **`AR964 load-budget-exceeded`**, row `claude-skill-listing-total` (warning past the budget, info from 90% of it),
  reports a start-of-session skill listing over Claude Code's listing budget, with the claude preset configured.
  `[lint.load_budgets] claude-skill-listing-total = N` replaces the computed budget with a fixed one.

Both findings sit on the configuration, but the change that moves them is usually elsewhere: under `validate
--changed` they depend on every start-of-session artifact and on the files that shape the session (see
[Under --changed](#under-changed)).

```toml
[exposure]
context_window = 200000            # the listing budget is 8000 characters
start_budget = 6000                # AR9W3 above 6000 estimated start tokens
```

## Area term leakage

Text a session sees at its start is in the context of every task. A word there that names one area pulls every
session toward it, so the rules read the start-of-session artifacts of the inventory: the root `CLAUDE.md` (and
`.claude/CLAUDE.md`), unconditional root rules, the listing line of each root skill and agent, the server names of the
root `.mcp.json` and the enabled plugin keys, and, with `plugins = "enabled"` or `"all"`, the listings of the plugin
skills that load. An artifact of unknown phase ([a gate that cannot be read](#a-gate-that-cannot-be-read)) counts as
start: the check fails closed. An artifact whose file was not read has no text to check; `exposure leakage` lists it
as unchecked, and `validate` does not report it.

- **`AR9W0 exposure-area-term`** (error): a term of an area that is not `shared`, on a non-blank line, outside path
  literals (see [Matching terms](#matching-terms)). Each line reports each term once per area that lists it, in line
  order, and the terms of a line by term and then area map order. The line is the file line of a `CLAUDE.md` or rule
  line (frontmatter counted), and `1` for a listing line, a server name or a plugin key.
- **`AR9W1 exposure-area-term-ambiguous`** (warning): a skill or agent listing that names no term at all (a shared
  area's included) but some ambiguous words; one finding per word, on line `1`, its area every area that uses the
  word, sorted and joined by `/` (`billing/search`).
- **`AR9W6 exposure-skill-area-unknown`** (warning): a `skill_areas` key that names no skill of the inventory, neither
  a qualified name (`plugin:skill`) nor a plugin skill's bare name. A plugin that is not enabled still counts. It is
  reported on the key's line in the areas file or the configuration.

The message of `AR9W0` and `AR9W1` is a stable contract, so an acceptance can name the artifact or the term:

```text
<kind> "<name>" at session start names term "<term>" of area "<area>"
```

Accept a finding with a reason in `[[lint.accept]]`; `match` is a case-sensitive substring of the message, and `path`
the file of the finding: `.mcp.json` for a server, and for a plugin key the settings file that lists it,
`.claude/settings.json` when that file enables the plugin. Only a plugin that `.claude/settings.local.json` alone
enables is reported there; that file is machine-local and usually untracked, so the finding, and the entry that
accepts it, exist only on the machines that have it. An entry that accepts nothing is reported as `AR9I0` and fails
`exposure leakage`:

```toml
[[lint.accept]]
rule = "AR9W0"
path = ".mcp.json"
match = 'mcp_server "billing-'
reason = "server names stay in the root .mcp.json; their tools are deferred"

[[lint.accept]]
rule = "AR9W0"
path = "CLAUDE.md"
match = 'names term "invoice"'
reason = "the root map names each area once to route to its guide"
```

### Under `--changed`

Under `validate --changed` (or `--since`) a finding is kept when its file changed or a file it depends on did, the
way an `AR701` duplicate depends on the other file of the pair. Every exposure finding depends on the files that
shape the session: the areas file, the configuration file, `.claude/settings.json`, `.claude/settings.local.json` and
the repository marketplace. Besides those:

- A leak finding (`AR9W0`, `AR9W1`) sits on its artifact and is kept when that file changed.
- `AR9W3` and the `AR964` `claude-skill-listing-total` finding sit on the configuration and depend on every
  start-of-session artifact, so the change that pushes a session past the budget keeps them.
- An `AR9W2` finding sits on the first listing of its pair and depends on the second, so editing either listing into
  a collision keeps it; the summary finding past the 20th pair sits on the configuration and depends on every listing
  it counts.
- An `AR9W6` finding sits on the areas file or the configuration and is kept when that file changed.

A dependency belongs to a file, not to one finding: while `AR9W3`, the listing total or the `AR9W2` summary stands,
any other finding on the configuration is kept too when a start-of-session artifact changed. `AR9I0` sits on the
configuration file and, as for every rule, is kept only when the configuration changed, so a change that fixes the
last finding an entry accepted shows the stale entry only in a full run.

## Validation

`AR9W3 exposure-start-budget` (off, error with `start_budget`) is described in [Start-of-session budgets](#start-of-session-budgets).

`AR9W5 exposure-map-invalid` (error) reports a map that cannot be used: a duplicate, reserved or non-kebab area name,
a bad path claim (absolute, trailing slash, empty, `.` or `..` segment, `*` other than at the end), a `skill_areas`
value that names no area, an `areas_file` that is missing, outside the project or has an unknown key, both
`areas_file` and inline areas, and invalid scalar keys. A finding on the file names its line. A map with any problem
is not used at all, so every rule that reads it stays quiet until it is fixed. See
[Strict validation](strict-validation.md#ar9w5-exposure-map-invalid).

`AR9W2 exposure-area-collision` is off until `[exposure] collision_min_score` is set, which turns it on at warning
with that threshold; a `[lint.severity]` entry turns it on at the default 0.1. It reports the 20 highest pairs
`exposure collisions` would at that score (its default `--limit`), each on the description line of the pair's first
listing, with a message of the form `skill "charge" of area "billing" and skill "rank" of area "search" list
overlapping words at session start (score 0.50, at least 0.10): customer, details, look, name`. When more pairs reach
the score, one more finding at the same severity, on the `collision_min_score` line of the configuration, counts the
rest (`and 35 more pairs ... see ai-rulez exposure collisions`), so a large repository gets 21 findings, not
thousands. The plugin skills counted are those of `[exposure] plugins`. See
[Strict validation](strict-validation.md#ar9w2-exposure-area-collision).

`AR9W4 exposure-gate-unreadable` (warning) reports a skill, command or rule whose frontmatter does not parse (`AR306`)
while a line of the block names `paths`, `disable-model-invocation` or `user-invocable`, on that line, and one whose
block parses but sets `paths` to something other than a string or a list of strings, or `disable-model-invocation`
to something other than `true` or `false`, on the first such key: the cases `exposure inventory` reads as phase
`unknown`. It needs no
`[exposure]` table and runs for the claude preset and for a run without a configuration. See
[A gate that cannot be read](#a-gate-that-cannot-be-read) and
[Strict validation](strict-validation.md#ar9w4-exposure-gate-unreadable).
