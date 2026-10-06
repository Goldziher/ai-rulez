# Verifiers

`[[verifiers]]` declares deterministic, read-only checks over the repository, run by `ai-rulez verifiers run`.
Use them for facts about the repo that your instructions depend on: a file is present, no `TODO(` is left in
shipped code, `package.json` pins the Node version the rules mention, generated files are in sync.

A verifier never uses the network, never starts a process and never writes. The same input gives the same
result.

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
ai-rulez verifiers list [--format json]        # what is declared, without evaluating
ai-rulez verifiers explain <name>       # what it checks, the rule it enforces, how to fix it
ai-rulez verifiers test [name...]       # run the self-test examples offline
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
| `AR9H5` `verifier-dead-scope` | `when_changed` matches no file of the repository; only with `--strict-applicability` |

They are reported by the `verifiers` commands, not by `validate`; `ai-rulez validate --explain AR9H1` describes them.

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
- The verifier verb is `verifiers run`; `validate --strict --verifiers` is not provided.
- `linguist-generated` files are not excluded by default; use `exclude`.
- `diff-added` ships in this slice; it needs a base, and with `--all` every line is added.
- SARIF `ruleId` is `AR9H1/<verifier id>`, as proposed.
- Verifiers are not marked `inactive` when their rule is outside the active profile; a target that exists
  anywhere in the content tree counts.

## Local overlay

`config.local.toml` may declare `[[verifiers]]`; entries are merged by `name`, and a local entry replaces the
shared one with the same name.

## Not yet

Not available: the `command` predicate that runs a program and checks its exit status (it will reuse the
hardened command runner, behind `--allow-exec`), LLM checklist verifiers, `verifiers suggest`, lock pinning of
verifiers (kind `verifier`) and import restrictions for verifiers that arrive through includes, `AR9H3` and
`AR9H4`, the `AR9H6` missing-examples rule and the `[verifiers_settings]` table.
