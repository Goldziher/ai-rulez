# Verifiers

`[[verifiers]]` declares deterministic, read-only checks over the repository, run by `ai-rulez verifiers run`.
Use them for facts about the repo that your instructions depend on: a file is present, no `TODO(` is left in
shipped code, `package.json` pins the Node version the rules mention, generated files are in sync.

A verifier never uses the network, never starts a process and never writes. The same input gives the same
result.

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
ai-rulez verifiers run --json           # machine-readable
ai-rulez verifiers run --strict         # warning-severity failures also fail
ai-rulez verifiers run --name readme    # only the named verifier (repeatable)
ai-rulez verifiers list                 # what is declared, without evaluating
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

## Local overlay

`config.local.toml` may declare `[[verifiers]]`; entries are merged by `name`, and a local entry replaces the
shared one with the same name.

## Not yet

A `command` type that runs a program and checks its exit status is planned; it will reuse the hardened command
runner, so the type is rejected by validation until then. Verifiers declared per domain, `all`/`any`/`not`
composition and changed-files-only runs are not available.
