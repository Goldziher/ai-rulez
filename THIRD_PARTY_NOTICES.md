# Third-party notices

ai-rulez is licensed under the [MIT License](LICENSE). Some files in this repository are derived from projects
under other licenses. This file lists each upstream project, its license and the files derived from it. See
[Derived rules and tests](CONTRIBUTING.md#derived-rules-and-tests) for how to add an entry.

Vendored copies with their own notice file (`internal/ard/schema/NOTICE.txt`, `internal/judge/prompts/NOTICE` and
the `NOTICE.txt` files under `testdata/`) are listed there, not here.

## NVIDIA SkillSpector

- **Source:** <https://github.com/NVIDIA/SkillSpector>
- **License:** [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0); the project ships no `NOTICE`
  file. The full license text goes in `LICENSES/Apache-2.0.txt`, added with the first derived file.
- **Copyright:** Copyright 2026 NVIDIA CORPORATION & AFFILIATES
- **Derived files:** none yet. List each one as `<file>: <upstream path> at <upstream commit>`.

## Neo23x0 signature-base

SkillSpector's YARA rules state that they are based on [Neo23x0/signature-base](https://github.com/Neo23x0/signature-base),
which is licensed under the Detection Rule License (DRL) 1.1 (the `LICENSE` file of that repository).
ai-rulez does not copy those YARA files. A pattern that comes from such a rule is re-derived as an RE2 regular
expression and keeps the attribution DRL-1.1 asks for: the original rule name, its author and reference, and a link
to the license.

- **Derived files:** none yet. List each one as
  `<file>: rule <original rule> by <rule author>, signature-base at <upstream commit>`.
