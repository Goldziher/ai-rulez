# Verifiers

`[[verifiers]]` declares deterministic, read-only checks over the repository, run by `ai-rulez verifiers run`.
Use them for facts about the repo that your instructions depend on: a file is present, no `TODO(` is left in
shipped code, `package.json` pins the Node version the rules mention, generated files are in sync.

A verifier never writes. It uses the network or starts a process only through two predicates of a rule-linked
verifier that you opt into per run: `command` (`--allow-exec`) and `llm` (`--allow-llm`). Without those flags the same
input gives the same result.

Two forms exist. The flat `[[verifiers]]` entries of `config.toml` (with a `type`) check the whole repository. The
rule-linked specs ([next section](#rule-linked-verifiers)) attach a check to the rule or skill it enforces, run
on changed files only, and can combine predicates. A spec lives in `.ai-rulez/verifiers/*.toml` or, without a
`type`, inline under `[[verifiers]]` in `config.toml`.

```toml
[[verifiers]]
name = "readme"
description = "Every project has a README"
type = "file_exists"
path = "README.md"

[[verifiers]]
name = "no-todo"
type = "forbid"
glob = "src/**/*.go"
pattern = 'TODO\('
severity = "warning"

[[verifiers]]
name = "node-version"
type = "key_equals"
path = "package.json"
key = "engines.node"
equals = ">=20"

[[verifiers]]
name = "outputs-current"
type = "generated_in_sync"
```

## Commands

```bash
ai-rulez verifiers run                  # table, exit 2 when an error-severity verifier fails
ai-rulez verifiers run --format json    # machine-readable (--json is a deprecated alias and conflicts with another --format)
ai-rulez verifiers run --strict         # warning-severity failures also fail (--fail-on warning)
ai-rulez verifiers run --name readme    # only the named verifier (repeatable)
ai-rulez verifiers run --since origin/main   # only what changed since the merge base
ai-rulez verifiers run --staged         # only what is staged (pre-commit)
ai-rulez verifiers run --rule database  # only verifiers that enforce this rule or skill
ai-rulez verifiers run --format sarif --out verifiers.sarif   # also junit, json, text
ai-rulez verifiers run --allow-exec     # also run `command` predicates (trusted refs only)
ai-rulez verifiers run --allow-llm --max-cost 0.25   # also evaluate `llm` checklists (sends the changed lines to the model)
ai-rulez verifiers run --estimate       # what the llm verifiers would send and cost; calls nothing
ai-rulez verifiers run --profile web    # which rules count as active (--role does the same for a role)
ai-rulez verifiers list [--format json]        # what is declared, without evaluating
ai-rulez verifiers explain <name>       # what it checks, the rule it enforces, how to fix it
ai-rulez verifiers test [name...]       # run the self-test examples offline (--allow-exec for command predicates)
ai-rulez verifiers suggest database     # ask the model to propose verifiers for the rule (dry run; --write saves them)
ai-rulez validate --strict --verifiers  # one run and one report for lint and verifiers (never a command or a model)
```

| Code | Meaning |
| --- | --- |
| 0 | No verifier failed at a failing severity |
| 2 | At least one verifier failed (`error`, or `warning` with `--strict`), even if another could not be evaluated |
| 1 | Nothing failed, but the run could not complete: the config does not load or validate, a `--name` is unknown, or a verifier could not be evaluated (status `error`) |

These match `ai-rulez doctor`. Failure takes precedence over "could not run" so that exit `1` never hides a real
failure; the report always lists both. A failing `info` verifier is reported but never fails the run. The MCP
server exposes the same run as the read-only `run_verifiers` tool. `generated_in_sync` runs the real comparison
there, except that the server never uses the network: a project that declares includes or installed skills gets
status `error` for it, never a pass.

## Fields

`name` (required, letters, digits, `.`, `_`, `-`, unique), `type` (required), `description`, and `severity`
(`error` default, `warning`, `info`). The remaining fields depend on the type. Paths are relative to the project
root and may not leave it. Everything below is checked when the config loads (`validate`, `verifiers list` and
`verifiers run` all report it): unknown types, fields that do not apply to the type, globs that do not compile or
that expand to more than 64 `{a,b}` alternatives, a `key_equals` file that is not `.json`, `.yaml`, `.yml` or
`.toml`, a malformed `key`, and a `generated_in_sync` profile that is not defined.

| Type | Passes when | Fields |
| --- | --- | --- |
| `file_exists` | the path (file or directory) exists | `path` |
| `file_absent` | the path does not exist | `path` |
| `glob_count` | the number of files matching `glob` is within bounds | `glob`, `min` and/or `max`, `exclude` |
| `regex` | every file matching `glob` contains `pattern` (at least one file must match) | `glob`, `pattern`, `exclude` |
| `forbid` | no file matching `glob` contains `pattern`; failures list `file:line` | `glob`, `pattern`, `exclude` |
| `key_equals` | the dotted `key` in a `.json`, `.yaml`, `.yml` or `.toml` file equals `equals` | `path`, `key`, `equals` |
| `generated_in_sync` | a fresh render of the profile matches the generated files on disk | `profile` |

- **Globs.** `**` crosses directories, `*` and `?` do not, `{a,b}` alternates, a pattern without a slash matches
  at any depth, and a trailing `/` matches everything below. Files are found by walking the project, so
  untracked files count; `.git`, `node_modules`, `.venv` and `__pycache__` are not entered. Symlinks are neither
  followed nor listed. Use `exclude` to drop more. A directory the walk cannot read is a finding for the
  glob-based verifiers that would have looked into it (the result would be incomplete), not an abort of the run;
  an `exclude` that covers it silences that.
- **Patterns** are Go (RE2) regular expressions matched against the whole file; use `(?m)` for line anchors.
- **Files that cannot be fully checked.** `regex` and `forbid` read at most 5 MiB per file and skip binary files
  (a NUL byte). A matched file that is binary or larger than 5 MiB is never counted as checked: when nothing
  else decides the outcome the verifier ends with status `error`, naming the files, so a scan is not silently
  weaker than it looks. Narrow `glob` or add the files to `exclude`. A violation that was found still fails the
  verifier (`forbid`: any hit; `regex`: a pattern missing from a fully read file), and a `regex` match inside
  the first 5 MiB counts. The `N file(s)` shown on a pass counts only files that were actually checked.
  `forbid` reports at most the first 10 `file:line` locations and stops scanning once it has found 11.
- **`file_exists` / `file_absent`** follow symlinks like `stat`. A live symlink exists. A dangling symlink (one
  whose target is missing) does not exist, so `file_exists` fails on it and `file_absent` passes. A symlink, at
  any point of the path, that resolves outside the project is status `error`: a probe never reports on what lies
  beyond the root, and a missing file behind such a link cannot be told apart from a present one.
- **`key_equals`** walks maps by key and lists by index. A key is dot-separated; write a literal dot in a segment
  as `\.` or quote the segment, so `dependencies.lodash\.merge` and `dependencies["lodash.merge"]` both address
  the `lodash.merge` entry, `list.0.id` and `list[0].id` both address a list element, and `\\` is a literal
  backslash. `equals` is a string compared with the value rendered as text: `true` and `3` match a boolean and a
  number, JSON numbers keep their written form (`1.0`, `12345678901234567890`), and TOML and YAML date-times are
  RFC 3339 (`2024-01-02T03:04:05Z`, `2024-01-02`). A missing key, a missing file, a non-scalar value and a
  `null` all fail, with different messages; `null` (YAML `~` included) never equals any string, not even `""`,
  while an empty string equals `""`. YAML keys that are not strings (`1: one`) are matched by their text. A file
  over 5 MiB is status `error`.
- **`generated_in_sync`** uses the same comparison as `generate --check`, and resolves includes like it does.
  `--profile` fills in the profile of verifiers that name none.
- **Output.** Control and non-printable characters in file names and messages (ANSI escapes, newlines) are shown
  as `\xNN` or `\uNNNN`, in the table and in JSON, so a hostile file name cannot rewrite the terminal. A
  canceled run (Ctrl-C) marks the verifiers it did not reach as `error`.

## Rule-linked verifiers

A file `.ai-rulez/verifiers/<anything>.toml` holds `[[verifiers]]` tables. Each names the item it enforces,
so a failure reports the rule or skill that declared the check. The directory is never rendered into generated
outputs (it costs no context). Declarations are checked on every run; one that cannot be used is reported as
`AR9H2` (status `error`, exit `1` when nothing failed) and never silently skipped.

```toml
[[verifiers]]
id = "migrations-have-down"
rule = "database"                  # exactly one of rule, skill, agent, command; `id` or `domain/id`
anchor = "## Migrations"           # optional heading in the rule file; findings point at its line
severity = "error"                 # error | warning (default) | info
message = "Every migration needs a '-- down' section."
fix = "Add a '-- down' section that reverses the change."
when_changed = ["db/migrations/*.sql"]   # scope: changed files matching these globs
exclude = ["db/migrations/legacy/**"]

[verifiers.require.regex]          # exactly one predicate table
regex = "(?m)^-- down\\b"
in = "same-file"

[[verifiers.examples]]             # self-tests for `verifiers test`
name = "with down passes"
files = { "db/migrations/0001.sql" = "CREATE TABLE t();\n-- down\nDROP TABLE t;\n" }
changed = ["db/migrations/0001.sql"]
expect = "pass"
```

`id` is lowercase letters, digits, `.`, `_`, `-`, and unique across `config.toml` and every spec file. Unknown
keys are an error.

The same fields can be declared inline in `config.toml`: an entry without `type` is a spec, its `name` is the
`id`, and `[verifiers.require.*]` / `[[verifiers.examples]]` nest under it. `config.toml` checks the shape
(exactly one of `rule`, `skill`, `agent`, `command`, a `require`, valid globs); the predicate tree and the
enforced item are checked on every run and reported as `AR9H2` like a file spec.

```toml
[[verifiers]]
name = "no-todo"
rule = "style"
when_changed = ["src/**/*.go"]

[verifiers.require.forbid]
regex = "TODO"
``` The format has a JSON Schema,
[`schema/verifiers-spec.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verifiers-spec.schema.json).

### Predicates

| Predicate | Fields | Holds when |
| --- | --- | --- |
| `regex` | `regex`, `in`, `files` | the RE2 pattern matches in each scoped file (`same-file`, default), in each scoped file's added lines (`diff-added`), or in at least one file (`any-file`) |
| `forbid` | same as `regex` | the pattern does not match |
| `file_exists` | `path`, `exists` (default true) | the path exists (or, with `exists = false`, does not); `path` may use a template |
| `paired` | `for_each`, `requires_changed` or `requires_exists` | for every scoped file matching `for_each`, the derived path was changed too, or exists |
| `glob_count` | `files`, `exclude`, `min`, `max` | the number of repository files matching `files` is within bounds |
| `command` | `argv`, `pass_files`, `timeout_s`, `expect_exit` | the program exits with `expect_exit` (default 0); needs `--allow-exec`, see [Command predicate](#command-predicate) |
| `all` / `any` / `not` | arrays of predicate tables (`not`: one table) | boolean composition, at most 4 levels deep |

Combinators repeat the table: `[[verifiers.require.all]]` followed by `[verifiers.require.all.regex]`, or
`[verifiers.require.not.file_exists]`.

- **Scope.** `when_changed` selects the changed, still existing files the verifier looks at (all files when no
  `--since` or `--staged` is given). It is required when a predicate examines scoped files (`same-file` and
  `diff-added` `regex` and `forbid`, `paired`, a templated `file_exists`). Without it the verifier is
  whole-repository and runs on every invocation. When `when_changed` selects nothing the status is
  `not_applicable` and the run is not failed.
- **Full tree.** Predicates resolve against the whole repository, not only the diff: `requires_exists`,
  `any-file`, `glob_count` and `file_exists` see unchanged files. The repository is the tracked files plus
  untracked files that are not ignored (a directory walk outside git); symlinks are never listed or followed.
- **`diff-added`** counts only added lines: `forbid` with it is the usual way to ratchet a rule on legacy code, so
  only new violations fail. With `--all` and in self-tests every line counts as added. A rename keeps its old
  path, so a moved file is not treated as new.
- **Templates** in `paired` and `file_exists`: `{path}` (the scoped file), `{dir}`, `{stem}` (file name without
  extension), `{ext}` (with the dot) and `{rel}`. `for_each` is a literal path whose `{rel}` captures the rest
  (`src/api/{rel}.py` matches `src/api/v1/users.py` with `rel = v1/users`), or, without `{rel}`, a glob. The
  derived path must stay inside the project; `..`, an absolute path and an unknown variable are errors.
- **Matching.** Patterns are RE2 (linear time, no backreferences, so a hostile pattern cannot stall a run). Files
  are matched with line endings normalised to LF. Binary files and files over 5 MiB are skipped and listed in
  the result's `notes`. A finding quotes at most 120 characters of the matched text with credential-looking
  text masked, and at most 50 findings are kept per verifier.

### Command predicate

`command` is the escape hatch for what a linter already expresses: call the linter instead of re-implementing it.

```toml
[[verifiers]]
id = "lockfile-in-sync"
skill = "dependency-updates"
severity = "warning"
when_changed = ["package.json", "package-lock.json"]

[verifiers.require.command]
argv = ["npm", "ci", "--dry-run"]   # no shell: argv[0] is looked up on PATH or is a path inside the project
pass_files = "stdin0"               # optional: "args" appends the scoped files (a name starting with "-" gets "./"), "stdin0" writes them NUL-separated to stdin
timeout_s = 120                     # optional, capped by [verifiers_settings] max_timeout_s (default 300)
expect_exit = 0                     # optional
```

- **Opt-in.** Nothing runs unless you pass `--allow-exec` (`verifiers run`, `verifiers test`) or set
  `AI_RULEZ_VERIFIERS_ALLOW_EXEC=1` (CI). No other flag implies it, and `generate`, `validate`, hooks and the MCP
  `run_verifiers` tool never run a verifier command. In CI, pass it only for trusted refs: a fork's pull request can
  carry its own `[[verifiers]]`.
- **Refused or not run is an error, never a pass.** Without `--allow-exec`, a program that is not installed, cannot be
  started or times out, the verifier ends with status `error` and `AR9H3`, so the run exits `1` when nothing failed.
  This holds under `not` too: a refusal is not inverted into a pass.
- **Execution.** The program runs through the same hardened runner as the lint scanners: no shell, the project root as
  working directory, stdin closed (or the NUL-separated files), at most 64 KiB captured per stream, the whole process
  group killed at the timeout. On a non-zero exit the finding shows the first line of stderr (else stdout), cut at 120
  characters with credential-looking text masked.
- **Environment allowlist.** The child gets only `PATH`, `HOME`, `USER`, `TMPDIR`, `TZ`, the locale (`LANG`, `LC_*`),
  `CI`, `NO_COLOR` and `TERM=dumb`, plus the names listed in `[verifiers_settings] command_env`. A verifier cannot read
  CI secrets; `command_env` refuses credential-looking and proxy names (`*_TOKEN`, `*_SECRET`, `*_API_KEY`, `HTTPS_PROXY`...).
- **No network sandbox.** ai-rulez cannot sandbox network use portably. Enforce it with an OS sandbox or the CI egress
  policy.
- **Imported verifiers** (from an include) cannot use `command` unless the include is allowlisted, see
  [Imported verifiers](#imported-verifiers).
- The flat form has no `command` type; a `type = "command"` entry is rejected with a pointer to the spec form.

### Changed-only runs

`--since REV` evaluates the files changed between the merge base of `REV` and `HEAD` and the working tree
(committed, staged, unstaged and untracked, renames detected). `--staged` evaluates what is staged; file contents
are read from the working tree. Both go through the hardened git helpers, never use the network, and
**fail with exit `1` when the base cannot be used**: a revision that does not exist, a repository without a
common ancestor (a shallow clone: fetch the base ref first), a directory that is not a repository, or `--staged`
in a repository without commits. A missing base never passes vacuously. `--since`, `--staged` and `--all` are
mutually exclusive. Added lines are read from a diff that always uses `a/` and `b/` prefixes, whatever
`diff.noprefix`, `diff.mnemonicPrefix` or `diff.srcPrefix` say in the user's git configuration.

Flat `[[verifiers]]` (with a `type`) check the whole repository and **ignore `--since` and `--staged`**: only specs
are narrowed to changed files.

### Output and failure mapping

Every finding carries the verifier, its `AR9H1` code, the subject `file:line`, the message, the `fix`, and the
target: the rule or skill, its source file and the `anchor` line.

```console
$ ai-rulez verifiers run --since origin/main
STATUS  SEVERITY  NAME                      MESSAGE
fail    error     migrations-have-down      Every migration needs a '-- down' section.
fail    error     new-endpoints-have-tests  src/api/orders.py: expected tests/api/test_orders.py to be changed too

AR9H1 migrations-have-down (rule "database", .ai-rulez/rules/database.md:3)
  db/migrations/0042.sql  pattern `(?m)^-- down\b` not found
  fix: Add a '-- down' section that reverses the change.
AR9H1 new-endpoints-have-tests (rule "api-conventions", .ai-rulez/rules/api-conventions.md)
  src/api/orders.py  expected tests/api/test_orders.py to be changed too

0 passed, 2 failed, 0 could not run (since origin/main)
```

| Format | Shape |
| --- | --- |
| `text` | the table above plus a block per failure (default) |
| `json` | summary, `mode` and results; follows [`schema/verifiers-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verifiers-report.schema.json) |
| `sarif` | SARIF 2.1.0: `ruleId` is `AR9H1/<verifier id>`, the location is the subject file, `relatedLocations` is the declaring rule (with the anchor line), `properties.rule` is the rule id, and `partialFingerprints` hashes verifier, path and normalised matched text so a line shift keeps the identity. A flat verifier has no code, so its `ruleId` is the verifier name; its results are located at the offending file (and line for `forbid`) and the rule text is the verifier's description |
| `junit` | one suite per rule (`rule:<id>`, `skill:<id>`, or `config` for a flat verifier), one case per verifier and subject; invalid declarations are `error`, `not_applicable` is `skipped`. A failure below the `--fail-on` threshold (a warning without `--strict`) is a passing case with the text in `system-out`, so the report agrees with the exit code |

`--out FILE` writes the report there (atomically) instead of stdout. `--fail-on error|warning|info|none` sets the
lowest failing severity (`--strict` is `--fail-on warning`); exit codes are unchanged.

### Codes

| Code | Meaning |
| --- | --- |
| `AR9H1` `verifier-failed` | a predicate did not hold; severity is the verifier's own |
| `AR9H2` `verifier-invalid` | bad regex, unknown or missing target, a missing `anchor` heading, two predicates, a bad template or example, an unknown key, a duplicate `id`, a symlinked or oversized file |
| `AR9H3` `verifier-command-failed-to-run` | a `command` predicate was refused (no `--allow-exec`, an untrusted include), could not start, or timed out; status `error` |
| `AR9H4` `verifier-llm-skipped` | an `llm` verifier was not evaluated (LLM use off, over `--max-cost`, every hunk withheld, every changed file unreadable, an unusable reply, `--estimate`); status `skipped`, severity `info`, never a pass |
| `AR9H5` `verifier-dead-scope` | `when_changed` matches no file of the repository; with `--strict-applicability` or `[verifiers_settings] warn_dead` |
| `AR9H6` `verifier-no-examples` | a spec verifier has no `[[verifiers.examples]]`; only with `[verifiers_settings] require_examples`; severity `warning` |

They are reported by the `verifiers` commands, and by `validate --strict --verifiers`; `ai-rulez validate --explain AR9H1`
describes them. Two more statuses never fail a run: `skipped` (an `llm` verifier that was not evaluated) and `inactive`
(see [Active profile and role](#active-profile-and-role)).

### Self-tests

`verifiers test [name...]` runs each verifier's `[[verifiers.examples]]` offline. An example gets a temporary
directory with its `files`; the `changed` files count as changed and entirely added, so no git repository is
involved. `expect` is `pass`, `fail` or `not_applicable`. Exit `0` when every example matches, `2` when one
does not or a declaration is invalid, `1` when the config does not load or a name is unknown. Verifiers
without examples are listed.

### Design decisions

- Rule-linked specs live in `.ai-rulez/verifiers/*.toml`; flat `[[verifiers]]` in `config.toml` keep working
  unchanged and have no rule link. Rule-linked fields (`rule`, `fix`, `when_changed`, combinators) may also be
  declared inline in `config.toml` by omitting `type`. A spec `id` may not repeat a `config.toml` name.
- `severity` defaults to `warning` for specs (as the design proposes) and to `error` for flat entries (their
  existing behaviour).
- Predicates are tables (`[verifiers.require.regex]`), not keys on `require`, so `regex` is one name for the
  predicate and its pattern field.
- A missing `anchor` heading is `AR9H2`, so the finding-to-rule mapping cannot rot.
- The verifier verb is `verifiers run`; `validate --strict --verifiers` runs the same verifiers inside the lint report
  (all files, no command, no model).
- `linguist-generated` files are not excluded by default; use `exclude`.
- `diff-added` ships in this slice; it needs a base, and with `--all` every line is added.
- SARIF `ruleId` is `AR9H1/<verifier id>`, as proposed.
- A verifier whose rule or skill is outside the active profile or role is `inactive` (reported, never failed), as the
  design proposes. A target that does not exist anywhere is still `AR9H2`.
- The `command` and `llm` predicates exist only in the spec form. A flat `type = "command"` is rejected.
- Imports follow the design: an include's verifiers cannot run a command unless the include is in `trust_exec_from`
  and pinned in `ai-rulez.lock`. An include's verifiers are loaded only from `<include>/verifiers/*.toml`; an include
  with an `include = [...]` filter brings none.
- An installed skill may ship verifiers in `<skill>/verifiers/*.toml` (the same file format). They load with the skill,
  report as `skill:<name>/verifiers/<file>`, and are covered by the skill's pin in `ai-rulez.lock`. They are data only: a
  skill's verifier can never use the `command` predicate, whatever `trust_exec_from` says.
- An `llm` verdict is advisory and capped at `warning` unless the verifier is calibrated and the run passes
  `--gate-llm` (see [Calibrating llm verifiers](#calibrating-llm-verifiers)). The record is a lighter cousin of the review
  calibration: it measures precision and recall of the `fail` verdict on labelled examples, not agreement or consistency.

## Local overlay

`config.local.toml` may declare `[[verifiers]]`; entries are merged by `name`, and a local entry replaces the
shared one with the same name.

## Active profile and role

A spec verifier names the rule, skill, agent or command it enforces. When that item is not part of the active
selection, the verifier is `inactive`: shown in the report, never failed, never run. The selection follows `generate`:
`--role`, else `--profile`, else the configured `default`, else the built-in `default` profile (all content when no
profiles are defined; root content plus builtin and include domains when profiles exist but `default` is not among
them). `--profile` also still names the profile of `generated_in_sync` verifiers that name none. An unknown profile or
role exits `1`.

## Imported verifiers

An include may carry `<include>/verifiers/*.toml`. Those declarations load like your own (same validation, same
`AR9H2` for a bad one, source shown as `include:<name>/verifiers/<file>`) with one restriction: a verifier from an
include may not use the `command` predicate, anywhere in its tree, unless

1. the include is named in `[verifiers_settings] trust_exec_from`, **and**
2. the include is pinned in `ai-rulez.lock` (a commit and digest for a remote include, a tree digest for a local path).

Otherwise the declaration is `AR9H2` and nothing runs, whatever `--allow-exec` says. `--allow-exec` is still required for
a trusted import. Imported `llm` verifiers are allowed; they are advisory and still need `--allow-llm`.

## LLM checklist verifiers

```toml
[[verifiers]]
id = "errors-are-actionable"
rule = "error-handling"
severity = "warning"                 # the ceiling: an llm verifier never reports above warning
when_changed = ["src/**/*.go"]       # required

[verifiers.require.llm]
checklist = [
  "New error messages say what the caller can do about the failure.",
  "No error swallows the original cause without wrapping it.",
]
model = ""                           # default: the [llm] model
max_diff_bytes = 24000               # changed text per call (1024 to 200000); larger changes are split by file and line
```

`llm` is allowed only as the root predicate or directly under `all`; under `any` or `not` an advisory verdict would
decide the result. Inside `all`, the deterministic members run first (a member that contains an `llm` predicate is
deferred too); if any fails, the model is not asked and the deterministic failure is the result. A member that could
not be evaluated, such as a refused `command`, does not hide a sibling's failure: the failure is the result (exit `2`)
and the refusal is noted. With no failure, the refusal is the result (`error`, exit `1`).

- **Opt-in and gated.** `--allow-llm` plus `allow_network = true` and a model in the **user** config (or
  `AI_RULEZ_LLM_*`); a repository config cannot turn the network on. Otherwise the verifier is `skipped` with `AR9H4`
  and the reason, visibly, never as a pass. The user `[llm]` settings are read only when a selected verifier uses
  `llm`, so a broken one cannot break a project that has none. `ai-rulez doctor` and `ai-rulez llm doctor` show the setup.
- **What is sent.** The added lines of the scoped files plus three lines of context, as numbered lines
  (`L12+ text`, `L9: context`), fenced between marker lines that carry a per-request token derived from the content.
  A hunk with a credential-looking string or a hidden character (zero-width, bidi, control) is withheld, noted, and
  never sent; if every hunk is withheld, or every changed file is binary or larger than `max_file_bytes`, the verifier is
  skipped, never passed. Nothing else from the repository leaves the machine. Lines are cut at 400 bytes (on a
  character boundary).
- **Structured, strictly decoded.** The reply schema uses no `additionalProperties` (Gemini rejects it); the reply is
  decoded strictly instead and anything extra, missing or out of range makes the reply unusable (skipped). Each
  checklist item gets `pass`, `fail` or `not_applicable`; a `fail` must name a file and quote the added line verbatim.
  The quote must be the whole added line (with at least three letters or digits) or a fragment of at least 10
  characters that starts and ends on a word boundary. A `fail` whose quote is not found that way on an added line of
  that file is dropped and counted in the notes.
- **Cache and cost.** Calls go through `internal/llm`: temperature 0, the response cache keyed by the request (changed
  text, checklist, prompt version, model), so a re-run on an unchanged diff costs nothing (shown as `from cache`).
  `--max-cost` (default $0.50, `0` removes it) refuses a call whose worst-case cost would exceed what is left, and the
  `[llm]` budget (`max_cost_usd`, `max_tokens`, `max_calls`) applies on top. An unknown price with a cap set refuses.
  `--estimate` prints, per call, the files and byte counts (never content) and the cost bound, and calls nothing.
- **Advisory, unless calibrated.** Results carry `advisory: true`; `severity = "error"` is reported as `warning`.
  `verifiers test` does not run the examples of an `llm` verifier (it cannot know what a model answers).
  [Calibration](#calibrating-llm-verifiers) is what lets one gate.
- Provider errors, timeouts and budget refusals skip the verifier (`AR9H4`) rather than fail the run, and are visible.

## Calibrating llm verifiers

A model's `fail` is a claim, not a fact. To let one fail the run, label examples and measure it:

```toml
[[verifiers.examples]]
name = "swallows the cause"
changed = ["a.go"]
expect = "fail"
[verifiers.examples.files]
"a.go" = "package a\nfunc f() error { return errors.New(\"failed\") }\n"
```

`ai-rulez verifiers calibrate [name...] --allow-llm` runs every example of the `llm` verifiers through the model (each
example's files count as entirely added) and records, per verifier, how often a `fail` verdict was right (precision) and
how many real failures it found (recall), with 95% Wilson intervals, in
`.ai-rulez/verifiers/calibration/<id>.json`. Commit it. The record passes at a precision of at least 0.80 on at least
10 examples, at least 3 expected to `fail` and at least 3 to `pass`, none of them unevaluated (a refusal, an unusable
reply or a budget stop). It needs `--allow-llm` and `allow_network` like a run; `--estimate` prints what would be sent;
`--no-write` only prints. Exit `2` when a record does not pass (it is still written).

`verifiers run --gate-llm` then lets a failing `llm` verifier declared `severity = "error"` keep that severity, with
`advisory` false and a note naming the figures, but only while the record is current: the same `llm` predicate
(checklist, model, `max_diff_bytes`), examples, prompt version and model it was measured with, and a passing status.
Anything else (no record, a failed bar, an edited checklist, another model) leaves the verdict capped at `warning` and
the note says why. Without `--gate-llm` nothing changes.

## Suggesting verifiers

`ai-rulez verifiers suggest <id>` (`--kind rule|skill|agent|command`) asks the model for up to `--max-proposals` (5)
candidates for a prose rule and prints the ones that survive deterministic checks. It is a dry run: **nothing is
written** unless you pass `--write`, which saves the usable candidates to a new
`.ai-rulez/verifiers/suggested-<id>.toml` and refuses to overwrite one.

What is sent: the item text and a repository summary (up to 60 directory names, the 12 commonest file extensions).
No file content. The same gates apply as for `llm` verifiers (`--allow-llm`, user-scope `allow_network`,
`--max-cost`, `--estimate`). Candidates are limited to `forbid`, `regex`, `file_exists`, `paired` and `glob_count`; a
suggestion never contains a `command` or `llm` predicate, and `error` is lowered to `warning`.

Each candidate is checked without the model: it must pass the same validation as a hand-written spec (RE2 compiles,
globs, templates, scope), the pass and fail example the model supplied must behave as claimed when run offline
(otherwise it is rejected), and it is run against the repository to count its findings today. A candidate that fails
widely is a ratchet candidate (`in = "diff-added"`) or too broad; a rule that cannot be checked mechanically gets
`No verifier proposed` with the model's reason.

Each usable candidate is also replayed over the last `--replay N` merged diffs (default 10, at most 100; 0 turns it off): every
commit of the first-parent history (a merge commit, a squash commit or a direct commit) is evaluated the way a CI run
on that change would have been, with the changed files against the tree as of that commit. A merged change passed
review, so a candidate that "would have flagged" many of them is noisy: the line `replay: would have flagged 2 of 8
merged diff(s)` names the first few. A project outside a repository, a root commit, or a revision too large to extract
is noted and skipped, never fatal. In a live run against Gemini (`gemini-2.5-flash-lite`) on this repository's architecture rule, one of five candidates survived; the others were rejected because their own examples did not behave as claimed. Exit `0` even with no proposal, `1` when it could not run.

## Settings

```toml
[verifiers_settings]          # not [verifiers]: TOML cannot use one key as both a table and an array
max_timeout_s = 300           # cap on a command predicate's timeout_s (1 to 900)
max_file_bytes = 5242880      # largest file a content predicate reads; a larger one is skipped with a note (default 5 MiB)
require_examples = false      # report a spec verifier without examples (AR9H6)
warn_dead = false             # report a when_changed that matches no file on every run (AR9H5)
trust_exec_from = []          # includes whose verifiers may use `command` (they must also be pinned in the lock)
command_env = ["MY_FLAG"]     # extra environment variable names passed to commands
```

`command_env` refuses credential-looking and proxy names, `trust_exec_from` must name a declared include, and the
table is pinned in the lock (item `settings` / `verifiers-settings`), so widening trust shows in review.

## Lock

Every verifier declaration is pinned in `ai-rulez.lock` as kind `verifier` (see [Lockfile](lockfile.md)): flat and
inline entries of `config.toml`, and each `[[verifiers]]` table of `.ai-rulez/verifiers/*.toml`. Lowering a severity,
widening an `exclude` or deleting a verifier changes the pin, so `lock --check` and `[lock] enforce` make "someone
weakened the check" visible in review. A local include's `verifiers/` directory is part of that include's pin; a remote
include's tree is already pinned by commit and digest.

## Not done

- Calibration measures precision and recall of one verdict on the verifier's own examples. The review design's
  consistency votes, metamorphic probes and calibration curve are not part of it.
- The model verdicts of `verifiers run` over MCP (`run_verifiers`) never gate: the tool sends nothing to a model.
