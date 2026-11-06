---
title: "Verifiers"
description: "Verifiers are read-only checks over the repository that back up what your rules and skills say: a file exists, no TODO( is left in shipped code, package.json pins the Node version a rule mentions, every migration has a down step."
---

Verifiers are read-only checks over the repository that back up what your rules and skills say: a file exists, no
`TODO(` is left in shipped code, `package.json` pins the Node version a rule mentions, every migration has a down
step. `ai-rulez verifiers run` evaluates them, and a failure names the rule or skill that declared the check.

## Example

A flat verifier in `config.toml` checks the whole repository:

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
```

A rule-linked verifier in `.ai-rulez/verifiers/database.toml` enforces the `database` rule on changed files, and
carries its own self-tests:

```toml
[[verifiers]]
id = "migrations-have-down"
rule = "database"
anchor = "## Migrations"
severity = "error"
message = "Every migration needs a '-- down' section."
fix = "Add a '-- down' section that reverses the change."
when_changed = ["db/migrations/*.sql"]

[verifiers.require.regex]
regex = "(?m)^-- down\\b"
in = "same-file"

[[verifiers.examples]]
name = "with down passes"
files = { "db/migrations/0001.sql" = "CREATE TABLE t();\n-- down\nDROP TABLE t;\n" }
changed = ["db/migrations/0001.sql"]
expect = "pass"

[[verifiers.examples]]
name = "without down fails"
files = { "db/migrations/0001.sql" = "CREATE TABLE t();\n" }
changed = ["db/migrations/0001.sql"]
expect = "fail"
```

```text
$ ai-rulez verifiers run --since HEAD
STATUS  SEVERITY  NAME                  MESSAGE
pass    error     readme
fail    warning   no-todo               forbidden pattern "TODO\\(" at src/main.go:2
fail    error     migrations-have-down  Every migration needs a '-- down' section.

 no-todo
  src/main.go:2  forbidden pattern "TODO\\("  [TODO(]
AR9H1 migrations-have-down (rule "database", .ai-rulez/rules/database.md:3)
  db/migrations/0002.sql  pattern `(?m)^-- down\b` not found
  fix: Add a '-- down' section that reverses the change.

1 passed, 2 failed, 0 could not run (since HEAD)
```

```text
$ ai-rulez verifiers test
ok   migrations-have-down: with down passes
ok   migrations-have-down: without down fails
2 of 2 examples passed
```

`ai-rulez validate --verifiers` adds the same results to the lint report as `AR9H1` findings, so one CI step covers
both:

```text
$ ai-rulez validate --verifiers
db/migrations/0002.sql:1: error AR9H1 verifier-failed: migrations-have-down: pattern `(?m)^-- down\b` not found (fix: Add a '-- down' section that reverses the change.)
src/main.go:2: warning AR9H1 verifier-failed: no-todo: forbidden pattern "TODO\\("
...
```

## Two forms

| | Flat verifier | Rule-linked verifier (spec) |
| --- | --- | --- |
| Declared in | `[[verifiers]]` in `config.toml`, with a `type` | `.ai-rulez/verifiers/*.toml`, or `[[verifiers]]` in `config.toml` without a `type` |
| Identified by | `name` | `id` (inline: `name`) |
| Linked to | nothing | one rule, skill, agent or command |
| Scope | the whole repository, always | changed files matching `when_changed`, or the whole repository |
| Checks | one `type` | a predicate tree (`regex`, `forbid`, `paired`, `command`, `llm`, `all`/`any`/`not`, ...) |
| Default severity | `error` | `warning` |

A verifier never writes. It starts a process only through a `command` predicate with `--allow-exec`, and uses the
network only through an `llm` predicate with `--allow-llm`. Without those flags the same input always gives the same
result.

## Commands

```bash
ai-rulez verifiers run                       # evaluate all verifiers
ai-rulez verifiers run --since origin/main   # specs look only at files changed since the merge base
ai-rulez verifiers run --staged              # specs look only at staged files (pre-commit)
ai-rulez verifiers run --name readme         # only this verifier (repeatable)
ai-rulez verifiers run --rule database       # only the verifiers that enforce this rule, skill, agent or command
ai-rulez verifiers run --strict              # warning failures also fail (--fail-on warning)
ai-rulez verifiers run --format sarif --output verifiers.sarif
ai-rulez verifiers run --allow-exec          # also run command predicates (trusted refs only)
ai-rulez verifiers run --allow-llm --max-cost 0.25   # also evaluate llm checklists
ai-rulez verifiers run --estimate            # what the llm verifiers would send and cost; calls nothing
ai-rulez verifiers list                      # what is declared
ai-rulez verifiers explain migrations-have-down
ai-rulez verifiers test [name...]            # run the self-test examples offline
ai-rulez verifiers calibrate [name...] --allow-llm
ai-rulez verifiers suggest database --allow-llm   # propose verifiers for a rule with a model (dry run)
ai-rulez validate --verifiers                # verifiers inside the lint report (never a command or a model)
```

| Exit code | Meaning |
| --- | --- |
| 0 | No verifier failed at a failing severity |
| 1 | Nothing failed, but the run could not complete: the configuration does not load, a `--name` is unknown, or a verifier could not be evaluated (status `error`) |
| 2 | At least one verifier failed (`error`, or `warning` with `--strict`) |

A failure wins over "could not run", and the report lists both. A failing `info` verifier is reported but does not fail
the run unless you pass `--fail-on info`. The [MCP server](/ai-rulez/mcp-server/) runs the same verifiers through its read-only `run_verifiers` tool; it never
runs a command or asks a model.

## Flat verifiers

`name` (required; letters, digits, `.`, `_`, `-`; unique), `type` (required), `description` and `severity` (`error`
default, `warning`, `info`), plus the fields of the type. Paths are relative to the project root and may not leave it.
The configuration load (`validate`, `verifiers list`, `verifiers run`) rejects unknown types, fields that do not apply
to the type, invalid globs, a `key_equals` file of another format and an undefined `generated_in_sync` profile.

| Type | Passes when | Fields |
| --- | --- | --- |
| `file_exists` | the path (file or directory) exists | `path` |
| `file_absent` | the path does not exist | `path` |
| `glob_count` | the number of files matching `glob` is within bounds | `glob`, `min` and/or `max`, `exclude` |
| `regex` | every file matching `glob` contains `pattern` (at least one file must match) | `glob`, `pattern`, `exclude` |
| `forbid` | no file matching `glob` contains `pattern`; failures list `file:line` | `glob`, `pattern`, `exclude` |
| `key_equals` | the dotted `key` in a `.json`, `.yaml`, `.yml` or `.toml` file equals `equals` | `path`, `key`, `equals` |
| `generated_in_sync` | a fresh render matches the generated files on disk, as `generate --check` does | `profile` |

- **Globs.** `**` crosses directories, `*` and `?` do not, `{a,b}` alternates, a pattern without a slash matches at any
  depth, a trailing `/` matches everything below. Untracked files count; `.git`, `node_modules`, `.venv` and
  `__pycache__` are skipped, symlinks are not followed. Use `exclude` to drop more.
- **Patterns** are RE2 regular expressions matched against the whole file; use `(?m)` for line anchors.
- **Large and binary files.** `regex` and `forbid` read at most 5 MiB per file and skip binary files. When such a file
  is the only thing that could decide the result, the verifier ends with status `error` and names it, so a scan never
  looks stronger than it is. `forbid` lists at most 10 locations.
- **`file_exists` / `file_absent`** follow symlinks. A dangling symlink does not exist; a symlink that resolves outside
  the project is status `error`.
- **`key_equals`** walks maps by key and lists by index: `engines.node`, `list.0.id` or `list[0].id`. Escape a dot in
  a key as `\.` or quote the segment (`dependencies["lodash.merge"]`). `equals` is compared with the value as text
  (`true`, `3`, `1.0`, `2024-01-02`). A missing key or file, a non-scalar value and `null` fail.
- **`generated_in_sync`** resolves includes like `generate`; `--profile` sets the profile of entries that name none.

## Rule-linked verifiers

A spec names the item it enforces, so a failure reports that rule or skill and the line of its `anchor`. Files in
`.ai-rulez/verifiers/` are never rendered into generated outputs. A declaration that cannot be used is reported as
`AR9H2` (status `error`), never skipped. The format has a JSON Schema:
[`schema/verifiers-spec.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verifiers-spec.schema.json).

| Key | Meaning |
| --- | --- |
| `id` | Required. Lowercase letters, digits, `.`, `_`, `-`; unique across `config.toml` and every spec file |
| `rule`, `skill`, `agent`, `command` | Exactly one: the item enforced, as `id` or `domain/id` |
| `anchor` | Heading in the item's file that findings point at; a missing heading is `AR9H2` |
| `severity` | `error`, `warning` (default) or `info` |
| `message`, `fix` | Shown with every failure |
| `when_changed` | Globs that select the scoped files (see below) |
| `exclude` | Globs removed from the scope |
| `[verifiers.require.<predicate>]` | Exactly one predicate table |
| `[[verifiers.examples]]` | Self-tests: `name`, `files`, `changed`, `expect` (`pass`, `fail`, `not_applicable`) |

The same keys work inline in `config.toml`: an entry without `type` is a spec, and its `name` is the `id`.

```toml
[[verifiers]]
name = "no-todo-in-go"
rule = "style"
when_changed = ["src/**/*.go"]

[verifiers.require.forbid]
regex = "TODO"
in = "diff-added"
```

### Predicates

| Predicate | Fields | Holds when |
| --- | --- | --- |
| `regex` | `regex`, `in`, `files` | the pattern matches in each scoped file (`in = "same-file"`, default), in each scoped file's added lines (`diff-added`), or in at least one file (`any-file`) |
| `forbid` | same as `regex` | the pattern does not match |
| `file_exists` | `path`, `exists` (default `true`) | the path exists, or with `exists = false` does not; `path` may use a template |
| `paired` | `for_each`, `requires_changed` or `requires_exists` | for every scoped file matching `for_each`, the derived path was changed too, or exists |
| `glob_count` | `files`, `exclude`, `min`, `max` | the number of repository files matching `files` is within bounds |
| `command` | `argv`, `pass_files`, `timeout_s`, `expect_exit` | the program exits with `expect_exit` (default 0); see [Command predicate](#command-predicate) |
| `llm` | `checklist`, `model`, `max_diff_bytes` | a model finds no checklist item violated; see [LLM checklist verifiers](#llm-checklist-verifiers) |
| `all` / `any` / `not` | arrays of predicate tables (`not`: one) | boolean composition, at most 4 levels deep |

Combinators repeat the table: `[[verifiers.require.all]]` followed by `[verifiers.require.all.regex]`, or
`[verifiers.require.not.file_exists]`.

- **Scope.** `when_changed` selects the changed files that still exist (all files when no `--since` or `--staged` is
  given). A predicate that examines scoped files (`same-file` and `diff-added`, `paired`, a templated `file_exists`)
  needs it. When it selects nothing, the status is `not_applicable` and the run does not fail.
- **Whole tree.** `requires_exists`, `any-file`, `glob_count` and `file_exists` see unchanged files too.
- **`diff-added`** looks only at added lines, so `forbid` with it ratchets a rule on legacy code: only new violations
  fail. With `--all` and in self-tests every line counts as added.
- **Templates** in `paired` and `file_exists`: `{path}`, `{dir}`, `{stem}` (file name without extension), `{ext}`
  (with the dot) and `{rel}`. In `for_each`, `{rel}` captures the rest of the path: `src/api/{rel}.py` matches
  `src/api/v1/users.py` with `rel = v1/users`. A derived path must stay inside the project.
- **Matching.** Line endings are normalized to LF. Binary files and files over 5 MiB are skipped and listed in the
  result's `notes`. A finding quotes at most 120 characters, with credential-looking text masked.

```toml
[[verifiers]]
id = "new-endpoints-have-tests"
rule = "api-conventions"
when_changed = ["src/api/**/*.py"]

[verifiers.require.paired]
for_each = "src/api/{rel}.py"
requires_changed = "tests/api/test_{stem}.py"
```

### Command predicate

`command` calls an existing linter instead of re-implementing it.

```toml
[[verifiers]]
id = "lockfile-in-sync"
skill = "dependency-updates"
when_changed = ["package.json", "package-lock.json"]

[verifiers.require.command]
argv = ["npm", "ci", "--dry-run"]   # no shell; argv[0] is looked up on PATH or is a path inside the project
pass_files = "stdin0"               # "args" appends the scoped files, "stdin0" writes them NUL-separated to stdin
timeout_s = 120                     # capped by [verifiers_settings] max_timeout_s (default 300)
expect_exit = 0
```

- **Opt-in.** Nothing runs without `--allow-exec` (`verifiers run`, `verifiers test`) or
  `AI_RULEZ_VERIFIERS_ALLOW_EXEC=1`. `generate`, `validate`, hooks and the MCP server never run a verifier command. In
  CI, pass it only for trusted refs: a pull request from a fork can carry its own verifiers.
- **Not run is an error, never a pass.** Without `--allow-exec`, or when the program is missing, cannot start or times
  out, the verifier ends with status `error` and `AR9H3`. Under `not` a refusal is not inverted into a pass.
- **Execution.** No shell, the project root as working directory, at most 64 KiB captured per stream, the process
  group killed at the timeout. On a non-zero exit the finding shows the first line of stderr (else stdout).
- **Environment.** The program gets only `PATH`, `HOME`, `USER`, `TMPDIR`, `TZ`, the locale variables, `CI`,
  `NO_COLOR` and `TERM=dumb`, plus the names in `[verifiers_settings] command_env`, which refuses credential-looking
  and proxy names.
- **No network sandbox.** Restrict network access with an OS sandbox or your CI egress policy.
- A verifier from an include may use `command` only when the include is trusted; see
  [Imported verifiers](#imported-verifiers). Flat verifiers have no `command` type.

### Changed-only runs

`--since REV` evaluates the files changed between the merge base of `REV` and `HEAD`, plus uncommitted and untracked
files (renames detected). `--staged` evaluates what is staged. `--since`, `--staged` and `--all` are mutually
exclusive. When the base cannot be used (an unknown revision, no common ancestor in a shallow clone, `--staged` in a
repository without commits) the run exits 1 instead of passing. Flat verifiers ignore `--since` and `--staged` and
always check the whole repository.

### Output

| Format | Shape |
| --- | --- |
| `text` | The table plus a block per failure (default) |
| `json` | Summary, mode and results; [`schema/verifiers-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verifiers-report.schema.json) |
| `sarif` | SARIF 2.1.0. `ruleId` is `AR9H1/<verifier id>` (a flat verifier's name), the location is the subject file, `relatedLocations` is the declaring rule's anchor line, and fingerprints survive line shifts |
| `junit` | One suite per rule (`rule:<id>`, `skill:<id>`, `config` for flat verifiers), one case per verifier and subject; `not_applicable` is `skipped`, a failure below `--fail-on` is a passing case |

`--output FILE` writes the report to a file. `--fail-on error|warning|info|none` sets the lowest failing severity.
Control characters in file names and messages are escaped, so a file name cannot rewrite the terminal.

### Codes

| Code | Meaning |
| --- | --- |
| `AR9H1` `verifier-failed` | A predicate did not hold; the severity is the verifier's own |
| `AR9H2` `verifier-invalid` | Bad regex, unknown or missing target, missing `anchor` heading, two predicates, bad template or example, unknown key, duplicate `id`, symlinked or oversized file |
| `AR9H3` `verifier-command-failed-to-run` | A `command` predicate was refused (no `--allow-exec`, an untrusted include), could not start, or timed out |
| `AR9H4` `verifier-llm-skipped` | An `llm` verifier was not evaluated (LLM use off, over budget, content withheld, unusable reply, `--estimate`); status `skipped`, never a pass |
| `AR9H5` `verifier-dead-scope` | `when_changed` matches no file; with `--strict-applicability` or `[verifiers_settings] warn_dead` |
| `AR9H6` `verifier-no-examples` | A spec has no `[[verifiers.examples]]`; with `[verifiers_settings] require_examples` |

`ai-rulez validate --explain AR9H1` describes each code. Two statuses never fail a run: `skipped` (an `llm` verifier
that was not evaluated) and `inactive` (see [Active profile and role](#active-profile-and-role)).

### Self-tests

`verifiers test [name...]` runs each spec's `[[verifiers.examples]]` offline in a temporary directory that holds the
example's `files`; the `changed` files count as changed and entirely added. Exit 0 when every example matches, 2 when
one does not or a declaration is invalid, 1 when the configuration does not load or a name is unknown. The examples
of an `llm` verifier are used by [calibration](#calibrating-llm-verifiers), not by `test`.

## LLM checklist verifiers

An `llm` predicate asks a model whether the added lines break a checklist. Its verdict is advisory: it is reported
at most as `warning` unless the verifier is [calibrated](#calibrating-llm-verifiers).

```toml
[[verifiers]]
id = "errors-are-actionable"
rule = "error-handling"
when_changed = ["src/**/*.go"]       # required

[verifiers.require.llm]
checklist = [
  "New error messages say what the caller can do about the failure.",
  "No error swallows the original cause without wrapping it.",
]
model = ""                           # default: the [llm] model
max_diff_bytes = 24000               # changed text per call (1024 to 200000); larger changes are split
```

- **Placement.** `llm` is the root predicate or a direct member of `all`. Inside `all`, the deterministic members run
  first; when one fails, the model is not asked.
- **Opt-in.** It needs `--allow-llm`, plus `allow_network = true` and a model in the user configuration (or
  `AI_RULEZ_LLM_*`); a repository configuration cannot turn the network on. See [LLM access](/ai-rulez/llm/). Otherwise the
  verifier is `skipped` with `AR9H4`.
- **What is sent.** The added lines of the scoped files with three lines of context. A hunk that holds a
  credential-looking string or a hidden character is withheld and never sent. Nothing else leaves the machine.
- **Strict replies.** Each checklist item gets `pass`, `fail` or `not_applicable`; a `fail` must quote an added line
  of the named file verbatim, or it is dropped.
- **Cost.** Responses are cached by request, so a re-run on an unchanged diff costs nothing. `--max-cost` (default
  $0.50, `0` removes the cap) refuses a call that could exceed what is left, on top of the `[llm]` budget.
  `--estimate` prints the files, byte counts and cost bound per call and calls nothing.
- Provider errors, timeouts and budget refusals skip the verifier (`AR9H4`) and are reported.

## Calibrating llm verifiers

A model's `fail` is a claim. To let one fail the run, label examples and measure how often it is right:

```toml
[[verifiers.examples]]
name = "swallows the cause"
changed = ["a.go"]
expect = "fail"
[verifiers.examples.files]
"a.go" = "package a\nfunc f() error { return errors.New(\"failed\") }\n"
```

`ai-rulez verifiers calibrate [name...] --allow-llm` runs every example of the `llm` verifiers through the model and
writes `.ai-rulez/verifiers/calibration/<id>.json`; commit it. The record holds the verifier, digests of the predicate
tree and the examples, the prompt version, the model and date, the confusion counts (`tp`, `fp`, `fn`, `tn`),
precision and recall with 95% Wilson intervals, and a `status`. It passes at a precision of at least 0.80 on at least
10 examples, with at least 3 expected to fail and 3 to pass, none unevaluated. `--estimate` prints what would be sent;
`--no-write` only prints. Exit 2 when a record does not pass (it is still written).

`verifiers run --gate-llm` then lets a failing `llm` verifier declared `severity = "error"` fail the run, as long as
its record is current (same predicate tree, `when_changed`, `exclude`, examples, prompt version and model) and passes.
Otherwise the verdict stays capped at `warning` and the report says why.

## Suggesting verifiers

`ai-rulez verifiers suggest <id>` (`--kind rule|skill|agent|command`) asks the model for up to `--max-proposals`
(default 5) verifiers for a prose rule and prints the ones that survive deterministic checks. Nothing is written
unless you pass `--write`, which saves them to a new `.ai-rulez/verifiers/suggested-<id>.toml`.

- **What is sent:** the item text and a repository summary (up to 60 directory names and the 12 most common file
  extensions), no file content. The gates of `llm` verifiers apply (`--allow-llm`, `allow_network`, `--max-cost`,
  `--estimate`).
- **What comes back:** only `forbid`, `regex`, `file_exists`, `paired` and `glob_count` predicates, never `command` or
  `llm`, at most `warning`.
- **Checks:** each candidate must pass the validation of a hand-written spec, its own pass and fail examples must
  behave as claimed offline, and it is run against the repository to count today's failures. A `forbid` without
  `in = "diff-added"` that matches an everyday library call or fails on more than 10 files is rejected.
- **Replay:** each candidate is replayed over the last `--replay N` merged changes (default 10, at most 100, 0 turns it
  off); one that would have flagged many reviewed changes is noisy, and the output says so
  (`replay: would have flagged 2 of 8 merged diff(s)`).

A rule that cannot be checked mechanically gets `No verifier proposed` with the model's reason. Exit 0 even with no
proposal, 1 when it could not run.

## Active profile and role

A spec whose rule, skill, agent or command is not part of the active selection is `inactive`: listed, never run,
never failed. The selection follows `generate`: `--role`, else `--profile`, else the configured `default` profile,
else the built-in `default` profile. A target that does not exist at all is `AR9H2`. An unknown profile or role exits 1.

## Imported verifiers

An include may carry `<include>/verifiers/*.toml`, and an installed skill `<skill>/verifiers/*.toml`. They load like
your own (shown as `include:<name>/verifiers/<file>` or `skill:<name>/verifiers/<file>`) and are pinned with their
source in `ai-rulez.lock`. An include with an `include = [...]` filter brings no verifiers.

A verifier from an include may use `command` only when:

1. the include is listed in `[verifiers_settings] trust_exec_from`, **and**
2. the include is pinned in `ai-rulez.lock`.

Otherwise the declaration is `AR9H2` and nothing runs, whatever `--allow-exec` says. A verifier from an installed
skill can never use `command`. Imported `llm` verifiers are allowed and still need `--allow-llm`.

## Local overlay

`config.local.toml` may declare `[[verifiers]]`; a local entry replaces the shared one with the same name. See
[Local configuration](/ai-rulez/local-overrides/).

## Settings

```toml
[verifiers_settings]          # not [verifiers]: that name is the array of verifiers
max_timeout_s = 300           # cap on a command predicate's timeout_s (1 to 900)
max_file_bytes = 5242880      # largest file a content predicate reads (default 5 MiB)
require_examples = false      # report a spec without examples (AR9H6)
warn_dead = false             # report a when_changed that matches no file (AR9H5)
trust_exec_from = []          # includes whose verifiers may use command (they must be pinned in the lock)
command_env = ["MY_FLAG"]     # extra environment variable names passed to commands
```

`trust_exec_from` must name a declared include. The table is pinned in the lock, so widening trust shows up in
review.

## Lock

Every verifier declaration is pinned in `ai-rulez.lock` as kind `verifier` (see [Lock file](/ai-rulez/lockfile/)). Lowering a
severity, widening an `exclude` or deleting a verifier changes the pin, so `lock --check` and `[lock] enforce` make a
weakened check visible in review.
