# Session exposure and areas

In a large repository, every session starts with the same context: the root instruction file, the unconditional
rules, the listing of root skills and agents, and the MCP server names. When that text names one team's service, every
session is pulled toward it, and a skill that belongs to one part of the tree competes with the skills of every other
part. An **area map** in the `[exposure]` table names the parts of the repository (the areas), the directories each one
claims and the words that identify it. ai-rulez then shows what a session loads at its start, which area every
artifact belongs to, and where an area's words leak into the shared start-of-session text.

Currently supports Claude Code only (`harness = "claude"`): the commands read the native Claude Code files
(`CLAUDE.md`, `.claude/`, `.mcp.json`, enabled plugins), whichever presets generated them.

## Example

A monorepo with two services and shared tooling:

```text
acme/
├── .ai-rulez/
│   ├── config.toml
│   ├── context/architecture.md      # as created by ai-rulez init
│   ├── rules/code-quality.md        # as created by ai-rulez init
│   └── skills/
│       ├── account-lookup/SKILL.md
│       ├── ci-debug/SKILL.md
│       └── invoice-retry/SKILL.md
├── services/
│   ├── billing/
│   └── search/
└── tools/ci/
```

`ai-rulez generate` writes the three skills to `.claude/skills/` at the root, so all three are listed at the start of
every session. Describe the areas in `.ai-rulez/config.toml`:

```toml
# .ai-rulez/config.toml (excerpt)
presets = ["claude", "cursor"]

[exposure]
harness = "claude"

[[exposure.areas]]
name = "billing"
owner = "team-billing"
paths = ["services/billing"]
terms = ["invoice", "ledger"]
ambiguous = ["account"]

[[exposure.areas]]
name = "search"
owner = "team-search"
paths = ["services/search"]
terms = ["reindex", "ranking"]
ambiguous = ["account"]

[[exposure.areas]]
name = "platform"
owner = "team-platform"
paths = ["tools"]
terms = ["terraform", "ci"]
shared = true                # repository-wide tooling: its terms may appear at session start
```

`ai-rulez exposure leakage` reports the area terms in the start-of-session text:

```text
$ ai-rulez exposure leakage
Leakage at session start: 6 artifacts checked
violations: 2  warnings: 1  accepted: 0
violations by area:
  billing: 2
...
findings:
  warning AR9W1 billing/search/account .claude/skills/account-lookup/SKILL.md:1: account-lookup: Look up an account by email or id. Use when asked who owns an account.
  violation AR9W0 billing/invoice .claude/skills/invoice-retry/SKILL.md:1: invoice-retry: Retry failed invoice payments and re-run the ledger export. Use when an invoice is stuck in pending.
  violation AR9W0 billing/ledger .claude/skills/invoice-retry/SKILL.md:1: invoice-retry: Retry failed invoice payments and re-run the ledger export. Use when an invoice is stuck in pending.
```

`ai-rulez validate` reports the same findings, so the check runs in CI with the rest of [validation](strict-validation.md):

```text
$ ai-rulez validate
.claude/skills/account-lookup/SKILL.md:1: warning AR9W1 exposure-area-term-ambiguous: skill "account-lookup" at session start names term "account" of area "billing/search"
.claude/skills/invoice-retry/SKILL.md:1: error AR9W0 exposure-area-term: skill "invoice-retry" at session start names term "invoice" of area "billing"
.claude/skills/invoice-retry/SKILL.md:1: error AR9W0 exposure-area-term: skill "invoice-retry" at session start names term "ledger" of area "billing"

validation: 2 error(s), 1 warning(s), 0 info across 1 root(s)
...
```

Fix them:

1. Move `invoice-retry` into a nested root for the billing service (see [Monorepo](monorepo.md)): move the skill
   directory to `services/billing/.ai-rulez/skills/invoice-retry/`, add `services/billing/.ai-rulez/config.toml`, and
   run `ai-rulez generate --recursive`. The skill is now written to `services/billing/.claude/skills/`, which Claude
   Code loads only when a session works in that directory.

```toml
# services/billing/.ai-rulez/config.toml
version = "5.0"
name = "billing"
presets = ["claude"]
```

2. `account-lookup` serves every team. Place it explicitly and accept the ambiguous word with a reason:

```toml
[exposure.skill_areas]
account-lookup = "billing"

[[lint.accept]]
rule = "AR9W1"
path = ".claude/skills/account-lookup/SKILL.md"
match = 'names term "account"'
reason = "support staff look up accounts of every service"
```

`ai-rulez exposure inventory` shows what loads when, and which area each artifact belongs to:

```text
$ ai-rulez exposure inventory
Session inventory of /work/acme (claude; plugins enabled, loaded: none)

PHASE      KIND        NAME                        TOKENS  AREA                   PATH
start      mcp_server  ai-rulez                    3       unclassified (none)    .mcp.json
start      claude_md   CLAUDE.md                   385     unclassified (none)    CLAUDE.md
start      settings    settings.json               0       unclassified (none)    .claude/settings.json
start      skill       account-lookup              23      billing (skill_areas)  .claude/skills/account-lookup/SKILL.md
start      skill       ci-debug                    27      platform (terms)       .claude/skills/ci-debug/SKILL.md
on_demand  mcp_server  ai-rulez                    3       billing (path)         services/billing/.mcp.json
on_demand  claude_md   services/billing/CLAUDE.md  96      billing (path)         services/billing/CLAUDE.md
on_demand  settings    settings.json               0       billing (path)         services/billing/.claude/settings.json
on_demand  skill       invoice-retry               30      billing (path)         services/billing/.claude/skills/invoice-retry/SKILL.md

9 artifacts: 5 start (438 tokens, unknown included), 4 on_demand, 0 manual, 0 unknown
```

`ai-rulez exposure` sums up what a session started at the root costs before it touches a file:

```text
$ ai-rulez exposure
start of session: 5 artifacts, ~438 tokens
skill listing: 2 skills, 189 chars of 8000 budget (200000 token window)
start tokens by kind:
  claude_md: 385
  skill: 50
  mcp_server: 3
  settings: 0
artifacts by phase and kind:
  start: skill 2, claude_md 1, mcp_server 1, settings 1
  on_demand: claude_md 1, mcp_server 1, settings 1, skill 1
  manual: none
tokens if loaded, by phase:
  start: 438
  on_demand: 129
heaviest at start:
  ~385 claude_md CLAUDE.md
  ~27 skill .claude/skills/ci-debug/SKILL.md
...
```

`exposure leakage` now exits 0 and lists the accepted finding with its reason. To keep the start of every session
small, add `start_budget` (see [Start-of-session budgets](#start-of-session-budgets)).

## Configuration

Every key is optional. Without an `[exposure]` table the commands still work (without areas), and only `AR9W4` of the
validation rules runs.

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `harness` | string | `"claude"` | Harness whose load model applies; only `"claude"` |
| `context_window` | integer | `200000` | Model context window in tokens, for the skill listing budget |
| `plugins` | string | `"enabled"` | Repository-marketplace plugins whose skills are listed at start: `none`, `enabled` (those `.claude/settings*.json` enable) or `all` |
| `start_budget` | integer | `0` (off) | Most estimated start-of-session tokens; turns on `AR9W3` |
| `collision_min_score` | number | `0` (off) | Lowest overlap score between listings of different areas; turns on `AR9W2` and is the default `--min-score` of `exposure collisions` |
| `areas_file` | string | | Area map in its own file, relative to the project root |
| `[[exposure.areas]]` | array of tables | | The areas (inline form) |
| `[exposure.skill_areas]` | table | | Explicit owners of skills: skill name (or `plugin:skill`) to area name |

Keys of an area:

| Key | Type | Meaning |
| --- | --- | --- |
| `name` | string | Required. Unique, lower-kebab (`a-z`, `0-9`, single hyphens). `unclassified` is reserved |
| `paths` | array of strings | Directories the area claims, relative to the repository root (see [Path claims](#path-claims)) |
| `terms` | array of strings | Words that name the area (see [Term matching](#term-matching)) |
| `ambiguous` | array of strings | Words the area shares with others (`account`, `query`); they never decide an area on their own |
| `shared` | boolean | Repository-wide tooling: its terms classify artifacts but are allowed at session start |
| `owner` | string | Free text, reported as is |
| `description` | string | Free text, reported as is |
| `declare` | string | Repository path a session of this area works on; validated and reported by `exposure areas` |

### Areas file

The map lives either inline or in `areas_file`, never both. The file must stay inside the project root and uses
`[[area]]` and `[skill_areas]`. An unknown key in it is an error.

```toml
# .ai-rulez/config.toml (excerpt)
[exposure]
areas_file = "tools/context/areas.toml"
```

```toml
# tools/context/areas.toml
[[area]]
name = "billing"
owner = "team-billing"
paths = ["services/billing"]
terms = ["invoice", "ledger"]

[[area]]
name = "platform"
paths = ["tools"]
shared = true
terms = ["terraform"]

[skill_areas]
account-lookup = "billing"
```

## Matching

### Path claims

- A path is a directory prefix with no leading or trailing slash: `services/billing` claims that directory and
  everything below it, not `services/billing2`.
- A trailing `*` makes a name prefix: `services/billing_*` claims `services/billing_api` and `services/billing_jobs/x`.
  `vendor/*` claims everything below `vendor`, not `vendor` itself.
- A leading `!` excludes: `"!services/billing_legacy"` keeps the area from claiming that directory.
- When several areas claim a directory, the longest matching path wins; on a tie, the area listed first wins.

### Term matching

- Terms and ambiguous words are trimmed and lowercased; matching ignores case.
- A term that contains `_`, `/` or `.` (`ledger_sync`, `@acme/ui`, `api.v2`) matches anywhere in the text.
- Any other term matches as a whole word, where only ASCII letters and digits continue a word: `invoice` matches
  `Invoice`, `INVOICE_ID` and `team_invoice`, not `invoices` or `reinvoice`.
- Longer terms are tried first, so `ledger_sync` wins over `ledger`.
- Repository paths are not words: a path that starts with the first segment of a path claim followed by `/`
  (`services/billing/README.md`) is ignored up to the next space, quote, backtick, bracket, comma or semicolon.
- A word that is both a term and ambiguous in one area counts as a term.

### Classification

An artifact (a skill, rule, agent, `CLAUDE.md`, MCP server) gets the first area that applies:

1. for a skill, its entry in `skill_areas` (a plugin skill is looked up as `plugin:skill`, then by its bare name);
2. the area with the most specific path claim on the artifact's directory (for a skill, the directory that holds its
   `.claude` directory; for a plugin skill, the plugin's directory);
3. the area with the most distinct terms in its text, ambiguous words not counted; a tie gives none. A skill's text is
   its listing line; a plugin skill's is its qualified name;
4. otherwise `unclassified`.

`exposure areas` and `exposure inventory` report the result with its source: `skill_areas`, `path`, `terms` or `none`.
A skill at the repository root is best placed with `skill_areas` until it moves under its area's directory, where the
path claim gives the same answer.

## What a session loads

The inventory follows Claude Code's load model for a session started at the inspected root:

| Phase | Artifacts |
| --- | --- |
| `start` | The root `CLAUDE.md` and `.claude/CLAUDE.md`, rules without `paths:`, the listing line of every root skill (`.claude/skills/<name>/SKILL.md`, and `.claude/commands/**/*.md`), root agents, the server names of the root `.mcp.json`, and the enabled plugins with their skills |
| `on_demand` | Anything in a subdirectory, and anything gated by `paths:`, once a matching file is touched |
| `manual` | Skills with `disable-model-invocation: true` or set to `user-invocable-only` in `skillOverrides`: only the user runs them |
| `unknown` | A root rule or skill whose gate cannot be read (see [A gate that cannot be read](#a-gate-that-cannot-be-read)); counted as `start` |

- `.claude/settings.local.json` applies over `.claude/settings.json`. `skillOverrides` `off` removes a skill,
  `name-only` blanks its description, and `claudeMdExcludes` globs exclude `CLAUDE.md` files and rules.
- A skill's listing line is `name: description when_to_use`, capped at 1536 characters.
- Tokens are estimated as UTF-8 bytes divided by 3.9. A file over 1 MiB is not read; its size still counts.
- `@path` imports in a `CLAUDE.md` or rule are followed up to five levels, and their text counts toward the importing
  file's tokens. An import that cannot be followed (an absolute or `~` path, a path outside the project, an unreadable
  file) is listed in `import_problems`.
- Not modeled: `CLAUDE.local.md`, user and managed settings, symlinked directories, and the agents, commands and MCP
  servers of plugins.

## Commands

All commands are offline and deterministic. They inspect the project root, or `--root DIR` to see what a session
started in `DIR` sees, with paths printed relative to `DIR`. The configuration is `-C` when given, else the one
discovered from `DIR` upwards, so on a directory with a nested root (`--root services/billing` in the example), pass
`-C .ai-rulez` (or `--areas`) to keep the project's area map. `--areas FILE` replaces the configured map with a file in the [areas file](#areas-file) format, so you can try
a map before you configure it.

| Flag | Commands | Meaning |
| --- | --- | --- |
| `--format text\|json` | all | Output format |
| `--plugins none\|enabled\|all` | all | Plugins whose skills are listed; default `[exposure] plugins`, else `enabled` |
| `--areas FILE` | all | Area map file to use instead of the `[exposure]` table |
| `--root DIR` | all | Directory to inspect; default the project root |
| `--context-window N` | `exposure` | Context window for the listing budget; default `[exposure] context_window`, else 200000 |
| `--harness claude` | `exposure` | Harness whose load model applies; only `claude` |
| `--with-text` | `inventory` | Include each artifact's text (with `--format json` only) |
| `--min-score N` | `collisions` | Lowest score reported, 0 to 1; default `[exposure] collision_min_score`, else 0.1 |
| `--limit N` | `collisions` | Pairs printed, highest score first (default 20; 0 prints all) |

| Exit code | Meaning |
| --- | --- |
| 0 | The report was printed |
| 1 | The command could not run: a bad flag, an invalid configuration or area map, or no area map for `areas`, `collisions` and `leakage` |
| 2 | `exposure`: start tokens exceed `start_budget`. `exposure leakage`: a violation or a stale acceptance |

### `ai-rulez exposure`

The start-of-session report: estimated start tokens in total and by kind, the skill listing against Claude Code's
listing budget, the artifacts per phase, and the heaviest artifacts at start. The listing budget is 1% of the context
window at 4 characters per token: 8000 characters at 200000 tokens. Past it, Claude Code keeps the skill names but drops
descriptions, which carry the words a request is matched against. `descriptions_capped` in the JSON counts skills whose
listing passes the 1536-character cap. JSON:
[`schema/exposure-report.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-report.v1.schema.json).

### `ai-rulez exposure inventory`

Every artifact with its kind, phase, visibility (`always`, `path_gated`, `manual_only`), listing text, tokens,
frontmatter status and, with an area map, its area and `area_source`. JSON:
[`schema/exposure-inventory.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-inventory.v1.schema.json).

```json
{"kind": "skill", "source": "file", "name": "account-lookup", "qualified_name": "account-lookup",
 "path": ".claude/skills/account-lookup/SKILL.md", "scope": ".", "visibility": "always", "phase": "start",
 "paths": [], "listing_text": "account-lookup: Look up an account by email or id. Use when asked who owns an account.",
 "listing_chars": 86, "truncated": false, "text_bytes": 86, "text_sha256": "e589…", "tokens": 23,
 "line_offset": 0, "frontmatter": {"ok": true}, "area": "billing", "area_source": "skill_areas"}
```

### `ai-rulez exposure areas`

The area map as loaded (terms normalized) and the area of every artifact with its source. Use the JSON to compare
the map with another classifier. JSON:
[`schema/exposure-areas.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-areas.v1.schema.json).

```text
$ ai-rulez exposure areas
Area map of /work/acme: the [exposure] table (3 areas, 1 skill_areas)

AREA          OWNER          SHARED  ARTIFACTS  PATHS
billing       team-billing   no      5          services/billing
search        team-search    no      0          services/search
platform      team-platform  yes     1          tools
unclassified                         3
...
```

### `ai-rulez exposure collisions`

The model picks a skill or agent by its listing, so two listings of different areas that share most of their words
compete for the same requests. The command compares every listing at session start with every listing of another area
and ranks the pairs by the Jaccard index of their words (shared words over all words of both). Words are lowercase
runs of a letter and two or more letters or digits, less common stopwords. `unclassified` counts as an area of its
own. JSON:
[`schema/exposure-collisions.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-collisions.v1.schema.json).

With a `search-lookup` skill ("Look up a document by email or id in the search index. Use when asked who owns a
document.") added at the root of the example:

```text
$ ai-rulez exposure collisions --min-score 0
Collisions of /work/acme (plugins enabled, min score 0.00): 1 of 1 pair among 3 listings

SCORE  FIRST                 AREA     SECOND               AREA          SHARED
0.60   skill account-lookup  billing  skill search-lookup  unclassified  asked, email, look, lookup, owns, who
```

To separate two listings, start each description with the nouns of its own area rather than the words every skill
of the repository uses.

### `ai-rulez exposure leakage`

The `AR9W0` and `AR9W1` findings of `validate` (see [Area term leakage](#area-term-leakage)), each marked
`violation`, `warning` or `accepted`, plus the `[[lint.accept]]` entries for those rules that accepted nothing.
`[lint.severity]`, `ignore_paths`, `[[lint.ignore_rules]]` and inline ignore comments apply; a baseline does not.
A start-of-session file that could not be read is listed as `unchecked`; it is not a finding. JSON:
[`schema/exposure-leakage.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-leakage.v1.schema.json).

## Area term leakage

The leakage rules read the text a session sees at its start: the root `CLAUDE.md` and `.claude/CLAUDE.md`,
unconditional root rules, the listing line of each root skill and agent, the server names of the root `.mcp.json`,
the enabled plugin names and, with `plugins = "enabled"` or `"all"`, the listings of the plugin skills that load.

| Code | Default | Reports |
| --- | --- | --- |
| `AR9W0` `exposure-area-term` | error | A term of an area that is not `shared`, on any line of that text |
| `AR9W1` `exposure-area-term-ambiguous` | warning | A skill or agent listing that names no term at all but some ambiguous words; the area is every area that uses the word, joined by `/` (`billing/search`) |
| `AR9W6` `exposure-skill-area-unknown` | warning | A `skill_areas` key that names no skill of the inventory, on its line in the areas file or the configuration |

Text that a `CLAUDE.md` pulls in with an `@path` import (such as the `@AGENTS.md` that `generate` writes) counts toward
start tokens, but is not checked for terms.

The message of `AR9W0` and `AR9W1` is stable, so an acceptance can match on it:

```text
<kind> "<name>" at session start names term "<term>" of area "<area>"
```

Accept a finding in `[[lint.accept]]` with a reason. `match` is a case-sensitive substring of the message; `path` is
the file of the finding (`.mcp.json` for a server, the settings file that enables a plugin). An entry that accepts
nothing is reported as `AR9I0` and fails `exposure leakage`. See
[Accepting one finding](strict-validation.md#accepting-one-finding).

```toml
[[lint.accept]]
rule = "AR9W0"
path = ".mcp.json"
match = 'mcp_server "billing-'
reason = "server names stay in the root .mcp.json; their tools are deferred"
```

## Start-of-session budgets

Two checks read the start-of-session report, computed with `[exposure] plugins` and `context_window`:

- **`AR9W3` `exposure-start-budget`** reports start tokens over `[exposure] start_budget`. Setting the budget turns
  the rule on at error, and `ai-rulez exposure` exits 2 in the same case. A start-of-session `@path` import that cannot
  be followed also fails the budget, because the total cannot be known.
- **`AR964` `load-budget-exceeded`**, row `claude-skill-listing-total`, warns when the start-of-session skill listing
  is past Claude Code's listing budget (info from 90% of it), with the `claude` preset configured.
  `[lint.load_budgets] claude-skill-listing-total = N` replaces the computed budget.

```toml
[exposure]
context_window = 200000            # listing budget: 8000 characters
start_budget = 6000                # AR9W3 above 6000 estimated start tokens
```

In the example repository, `start_budget = 300` gives:

```text
$ ai-rulez validate
.ai-rulez/config.toml:38: error AR9W3 exposure-start-budget: a session started at the repository root loads about 438 estimated tokens (5 artifacts), 138 past the [exposure] start_budget of 300; run `ai-rulez exposure` for the heaviest artifacts
```

## Listing collisions

`AR9W2` `exposure-area-collision` is off until `[exposure] collision_min_score` is set, which turns it on at warning
with that threshold. It reports the 20 highest pairs `exposure collisions` finds, each on the description line of the
pair's first listing, and one more finding on the configuration that counts the rest. With
`collision_min_score = 0.3` and the `search-lookup` skill above:

```text
.claude/skills/account-lookup/SKILL.md:2: warning AR9W2 exposure-area-collision: skill "account-lookup" of area "billing" and skill "search-lookup" of area "unclassified" list overlapping words at session start (score 0.60, at least 0.30): asked, email, look, lookup, owns, who
```

## A gate that cannot be read

`paths:`, `disable-model-invocation` and `user-invocable` decide when Claude Code shows a skill or rule. When the
frontmatter around them is not valid YAML, Claude Code sometimes recovers the gate and sometimes drops it:

| Frontmatter of a skill with `paths: ["src/**"]` | Claude Code |
| --- | --- |
| `description: Use when: testing` (unquoted colon) | Recovers: hidden until a file under `src/` is read |
| `description: "Use when testing` (unclosed quote) | Drops the gate: listed at start, description replaced by the first body line |
| `paths` indented with a tab | Drops the gate: listed at start |

The same doubt applies to a gate with a value of the wrong type: `paths` that is not a string or a list of strings,
or `disable-model-invocation` that is not `true` or `false`. Claude Code reads `yes`, `no`, `on` and `off` as booleans
(YAML 1.1); ai-rulez reads them as strings (YAML 1.2), so write `true` and `false`.

ai-rulez does not guess:

- `exposure inventory` gives such a root rule or skill the phase `unknown`, and the start-of-session reports count it
  as start.
- `validate` reports `AR9W4` `exposure-gate-unreadable` (warning) next to `AR306` for frontmatter that does not parse.
  It runs for the `claude` preset and without a configuration, with or without an `[exposure]` table.
- The [MCP server](mcp-server.md) never returns such a rule for a `find_context` path query, and does not serve a skill
  whose frontmatter does not parse. Only `paths:` fails closed there: a Cursor rule (`.cursor/rules/*.mdc`) whose broken
  frontmatter names `globs:` is served as a rule that applies everywhere.

Quote the value YAML cannot read (`description: "Use when: testing"`) and write gates with valid types.

## Changed-only runs

Under `validate --changed` or `--since`, an exposure finding is kept when its own file changed or when one of the
files that shape the session changed: the areas file, the configuration, `.claude/settings.json`,
`.claude/settings.local.json` and the repository marketplace. In addition:

- `AR9W3` and the `AR964` listing total sit on the configuration and are kept when any start-of-session artifact
  changed.
- `AR9W2` sits on the first listing of a pair and is kept when either listing changed.

## Rule codes

| Code | Name | Default | Reports |
| --- | --- | --- | --- |
| `AR9W0` | `exposure-area-term` | error | A term of a non-shared area in start-of-session text |
| `AR9W1` | `exposure-area-term-ambiguous` | warning | A start-of-session listing that names only ambiguous words |
| `AR9W2` | `exposure-area-collision` | off | Overlapping listings of different areas; on with `collision_min_score` |
| `AR9W3` | `exposure-start-budget` | off | Start tokens over `start_budget`; on (error) with `start_budget` |
| `AR9W4` | `exposure-gate-unreadable` | warning | A gate whose frontmatter does not parse or whose value has the wrong type |
| `AR9W5` | `exposure-map-invalid` | error | An unusable `[exposure]` table or area map; while it stands, no rule that reads the map runs |
| `AR9W6` | `exposure-skill-area-unknown` | warning | A `skill_areas` key that names no skill |

`AR9W5` covers a duplicate, reserved or non-kebab area name, a bad path claim (absolute, trailing slash, empty, `.` or
`..` segment, `*` other than at the end), a `skill_areas` value that names no area, an `areas_file` that is missing,
outside the project or has an unknown key, both `areas_file` and inline areas, and invalid scalar keys.

Every rule's entry, with examples, is in the [rule reference](strict-validation.md#rule-reference).
