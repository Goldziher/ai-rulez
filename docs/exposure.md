# Session exposure and areas

A large repository has parts that belong to different teams: services, products, shared tooling. Context loaded at
the start of every session (the root `CLAUDE.md`, unconditional rules, the skill listing) should not name one of
those parts unless every session needs it, and a skill that belongs to one part should say so. The `[exposure]` table
describes the parts as an **area map**: which directories each area claims, who owns it, and the words that name it.

This page describes the table, how the map is matched, and the commands that show what a Claude Code session loads
and where each artifact belongs. The rules and reports that build on it (start-of-session exposure, area term
leakage, collisions between areas) follow in later releases; today `validate` checks the map itself (`AR9W5`).
Without an `[exposure]` table no rule on this page has any effect; the commands work without one.

- [Configuration](#configuration)
- [The area map](#the-area-map)
- [Matching terms](#matching-terms)
- [Classifying an artifact](#classifying-an-artifact)
- [The owner of a skill](#the-owner-of-a-skill)
- [Commands](#commands)
- [Validation](#validation)

## Configuration

```toml
[exposure]
harness = "claude"                 # only "claude"
context_window = 200000            # tokens (default 200000)
plugins = "enabled"                # none | enabled (default) | all: plugin skills counted at start
areas_file = "tools/context/areas.toml"   # optional: the map in its own file
start_budget = 0                   # most estimated start tokens; 0 (default) is off
collision_min_score = 0            # word overlap between areas; 0 (default) is off

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

Every key is optional. `start_budget`, `collision_min_score`, `plugins`, `context_window` and `declare` are read by
the exposure report and context evals, which arrive in later releases; they are validated now so a table written
today keeps working.

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
paths = ["deploy", "dev_tools"]
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

Case folding follows Python's `re.IGNORECASE`, the reference the format comes from: `K` (U+212A, the Kelvin sign)
matches `k`, a final `ς` is the lowercase of `Σ`. A test holds the matcher to vectors recorded from the Python
implementation, which pins CPython 3.11 (Unicode 14.0). A character that is unassigned in that Unicode version may
match differently.

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
`dev_tools/plugins/ops` is owned by the area that claims `dev_tools`, whatever its skills' descriptions say. Place
a plugin outside every claim with `[skill_areas]`.

An explicit entry is the way to place a skill that still lives at the repository root: once it moves under the
area's directory, the path claim gives the same answer and the entry can go.

## Commands

Both commands are offline, free and deterministic. They inspect the project root (the directory of the
configuration, else the working directory) or `--root DIR`, and read the area map of the `[exposure]` table or of
`--areas FILE`, a file in the `areas_file` format that replaces the configured map, so a map can be tried before
it is configured. With `--root` the configuration is still `-C/--config` when it is given (one that cannot be read
or is invalid fails the command), else the one `--config-dir` names in `DIR`, else the one discovered from `DIR`
upwards, so `--root` on a subdirectory of a project uses the project's map; `--no-local` applies either way. The
`areas_file` of the configuration stays relative to the configuration's project root; the path claims of the map are
matched against artifact paths relative to `--root`. Exit codes: `0` the report was printed, `1` the command could not run (a bad flag, an invalid
configuration or area map, whose problems are listed with their lines as `AR9W5` reports them).

| Flag | Commands | Meaning |
| --- | --- | --- |
| `--format text\|json` | both | Output format |
| `--plugins none\|enabled\|all` | both | Marketplace plugins whose skills are listed: none, the ones the root `.claude/settings*.json` enable, or all; default `[exposure] plugins`, else `enabled` |
| `--areas FILE` | both | Area map file to use instead of the `[exposure]` table |
| `--root DIR` | both | Repository root to inspect |
| `--with-text` | `inventory` | Include each artifact's text in the JSON (`--format json` only; with text output it is an error) |

### `ai-rulez exposure inventory`

Every native Claude Code artifact a session started at the repository root can see. The load model follows Claude
Code 2.1.306 as measured:

- **Start of the session:** the root `CLAUDE.md` (and `.claude/CLAUDE.md`), unconditional root rules, the listing of
  root skills (`.claude/skills/<name>/SKILL.md`, and `.claude/commands/**/*.md` as `a:b` skills), root agents
  (`.claude/agents/<name>.md`), the server names of the root `.mcp.json`, and the enabled plugins with their skills
  (`plugin:skill`).
- **On demand:** anything in a subdirectory, and anything gated by `paths:`, once a matching file is touched.
- **Manual:** `disable-model-invocation: true` or `skillOverrides` `user-invocable-only`: only the user runs it.
- **Unknown:** a root rule or skill whose frontmatter does not parse while it names a gate (`paths:`,
  `disable-model-invocation`, `user-invocable`), or one that was not read (over the read limit, or a symlink to
  outside the root; `frontmatter.ok` is false and the error starts with `not read:`); whether it loads at start
  cannot be known. A start count includes it (fail closed), as the text summary does.
- `skillOverrides` `off` removes a skill and `name-only` blanks its description; `.claude/settings.local.json`
  applies over `.claude/settings.json`; `claudeMdExcludes` globs exclude `CLAUDE.md` files and rules.
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
  "plugins": {"mode": "enabled", "marketplace": "tools", "available": [{"name": "ops", "path": "dev_tools/plugins/ops"}], "enabled": ["ops"], "loaded": ["ops"]},
  "artifacts": [
    {"kind": "skill", "source": "plugin", "name": "probe", "qualified_name": "ops:probe", "plugin": "ops",
     "plugin_path": "dev_tools/plugins/ops", "path": "dev_tools/plugins/ops/skills/probe/SKILL.md", "scope": ".",
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
were tried: the artifact's scope, or a plugin skill's plugin directory. Diff the JSON against another classifier to
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
     "path": "dev_tools/plugins/ops/skills/probe/SKILL.md", "scope": ".", "area_scope": "dev_tools/plugins/ops",
     "phase": "start", "area": "platform", "area_source": "path"}
  ],
  "counts": {"billing": 4, "platform": 3, "unclassified": 12},
  "problems": []
}
```

## Validation

`AR9W5 exposure-map-invalid` (error) reports a map that cannot be used: a duplicate, reserved or non-kebab area name,
a bad path claim (absolute, trailing slash, empty, `.` or `..` segment, `*` other than at the end), a `skill_areas`
value that names no area, an `areas_file` that is missing, outside the project or has an unknown key, both
`areas_file` and inline areas, and invalid scalar keys. A finding on the file names its line. A map with any problem
is not used at all, so every rule that reads it stays quiet until it is fixed. See
[Strict validation](strict-validation.md#ar9w5-exposure-map-invalid).
