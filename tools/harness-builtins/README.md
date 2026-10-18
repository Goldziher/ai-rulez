# harness-builtins

Maintenance scripts for `internal/harnessnames/data/<preset>.toml`, the per-harness tables of built-in
command, skill and agent names (rules AR808, AR9U0, AR9U1). Python 3.11+, standard library only.

- `convert.py` turns the compiled data set of [issue #447](https://github.com/Goldziher/ai-rulez/issues/447)
  (the issue body and its four comments, as markdown) into the data files. `quotes.toml` holds the verbatim
  quotes that were checked against the live page and the groups the tables could not express.

  ```sh
  python3 -I tools/harness-builtins/convert.py internal/harnessnames/data body.md c1.md c2.md c3.md c4.md
  ```

- `check_sources.py` fetches the sources of some presets, checks that every quote occurs on its page and that
  the listed names occur on the page that lists them. A missing name is reported, not fatal: a page may spell a
  name in a form the check does not see (an import such as `aboutCommand`).

  ```sh
  python3 -I tools/harness-builtins/check_sources.py internal/harnessnames/data claude gemini qwen
  ```

A file is data, not hand-written: change `quotes.toml` or re-run the converter, then re-run
`go test ./internal/harnessnames`. An entry with no source URL is not in the tables. A group records the
collision outcome (`user_wins`, `builtin_wins`, `both_listed`, `error`, `n/a`, `unknown`); only a group with a
verbatim quote for that outcome can make a rule report an error. Extractors that regenerate a file from a
harness's registry at a pinned tag are not written yet (follow-up of #447).
