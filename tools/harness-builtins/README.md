# harness-builtins

Tools that keep `internal/harnessnames/data/<preset>.toml` (the per-harness tables of built-in command, skill
and agent names behind rules AR808, AR9U0 and AR9U1) in step with the public sources they were taken from.
Python 3.11+, standard library only, always run as `python3 -I`. The Go build embeds the data files and does not
use anything here.

## How a table is kept current

```text
pins.toml            where each harness keeps its built-ins and the tag or commit the data was read at
extractors/<x>.py    reads that registry (source or a structured docs page) and returns names and aliases
generate.py          merges the extractor output into the data file; everything else is kept
check_sources.py     checks every quote against its page
```

An extractor defines `extract(ctx)` and returns `{key: {"names": [...], "aliases": {name: [...]}}}`. `ctx.raw(path)`
reads a file of the pinned repository, `ctx.tree(prefix)` lists its files at the pin and `ctx.get(url)` reads a
public page; only plain https URLs are fetched. The `[[<preset>.target]]` tables of `pins.toml` say which group
each key feeds (by kind, by a part of the source URL and optionally by `user_collision`); each target must select
exactly one group. The merge replaces `names` and `aliases` of that group (sorted) and leaves source ids, quotes,
`user_collision`, `evidence`, `invocation_spaces` and `completeness` alone. `add` and `exclude` in a target
correct what the registry cannot show; say why in a comment.

A harness with no registry to read (closed source, names only in prose) has `extractor = "none"` and a `reason`
in `pins.toml`, so coverage is explicit. Every data file has exactly one pins entry, and `generate.py` refuses to
run over a data file without one.

## Commands

```sh
python3 -I tools/harness-builtins/generate.py --check [preset ...]   # exit 1 when a data file differs from its pinned source
python3 -I tools/harness-builtins/generate.py --write [preset ...]   # regenerate (also sets verified_on to today)
python3 -I tools/harness-builtins/generate.py --bump  [preset ...]   # show the names a newer tag or head would change
python3 -I tools/harness-builtins/generate.py --bump --apply codex   # move the pin and rewrite the data file
python3 -I tools/harness-builtins/check_sources.py [--names] [--stamp] [preset ...]
```

`generate.py` exits 0 up to date, 1 drift, 2 a source could not be read or an extractor failed (a changed format
is an error, never an empty result). `--bump` proposes the newest tag matching `tag_pattern` (or the head commit
when `track = "head"`) from `git ls-remote`. `check_sources.py` fetches every source with a quote, checks that the
quote occurs on the page (whitespace-insensitive) and that each group with a recorded collision outcome
(`user_wins`, `builtin_wins`, `both_listed`, `error`) has one; it prints verified and failed counts per harness
and exits 1 on a missing quote, 2 when a page cannot be fetched. `--stamp` sets `verified_on` of the sources whose
quote was found.

Task targets:

```sh
task harness:builtins:test      # offline unit tests (also run by go test ./internal/harnessnames when python3 exists)
task harness:builtins:check     # network: generate.py --check and check_sources.py
task harness:builtins:refresh   # network: generate.py --bump, writes nothing
```

`.github/workflows/harness-builtins.yml` runs the unit tests and the check every Monday and opens (or updates) one
issue when a data file drifted or a quote is gone.

## Tests

`tests/` holds the offline tests, run with `python3 -I -m unittest discover -s tests -t .` from this directory.
Parsers are tested against excerpts saved under `testdata/<preset>/` (an excerpt of the source named in its first
line); network tests do not exist. A new extractor needs a fixture, a test of the parser, a test of a changed
format, a pins entry and a `generate.py --check <preset>` that passes.

## Adding a harness or fixing a quote

Edit the data file with `hb.tomlout.load` / `save` (the layout is byte-stable), or by hand and re-run the unit
tests. A quote is verbatim text of the page; if no page states what the harness does with a user item of the same
name, the group's `user_collision` is `unknown`. `convert.py` and `quotes.toml` are the one-time converter of the
compiled data set of [issue #447](https://github.com/Goldziher/ai-rulez/issues/447); the data files are now the
source of truth and are not regenerated from them.
