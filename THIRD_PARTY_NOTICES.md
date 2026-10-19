# Third-party notices

ai-rulez is licensed under the [MIT License](LICENSE). Some files in this repository are derived from projects
under other licenses. This file lists each upstream project, its license and the files derived from it. See
[Derived rules and tests](CONTRIBUTING.md#derived-rules-and-tests) for how to add an entry.

Vendored copies with their own notice file (`internal/ard/schema/NOTICE.txt`, `internal/judge/prompts/NOTICE` and
the `NOTICE.txt` files under `testdata/`) are listed there, not here.

## NVIDIA SkillSpector

- **Source:** <https://github.com/NVIDIA/SkillSpector>
- **License:** [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0), full text in
  [`LICENSES/Apache-2.0.txt`](LICENSES/Apache-2.0.txt); the project ships no `NOTICE` file.
- **Copyright:** Copyright 2026 NVIDIA CORPORATION & AFFILIATES
- **What is derived:** behaviour and regular expressions of the static pattern analyzers, and test fixtures from the
  pattern tests, reimplemented as RE2 rules and Go table tests in `internal/lint`.
- **Modification notice (Apache-2.0 section 4(b)):** every derived file is marked as modified in its header. The
  changes are a rewrite from Python to Go, translation of patterns to RE2, and replacement of the SkillSpector
  finding model with ai-rulez rule codes.
- **Derived files**, all taken from upstream commit `cf5cac7ba5c756a6e0ebf52dc9763f49422f889f`:

| File | Derived from (SkillSpector rule ids) |
| --- | --- |
| `internal/lint/ar_prose.go`, `internal/lint/ar_prose_test.go` | AR1-AR3, E4, P3 (AR041, AR042) |
| `internal/lint/ar_exec.go`, `internal/lint/ar_exec_test.go` | RA2, DS1-DS4, SSRF1-SSRF3 (AR043-AR045) |
| `internal/lint/ar_hidden.go`, `internal/lint/ar_hidden_test.go` | SC8, SC9, P3, P9, AE4, AE6 (AR051-AR053); partly derived, some fixtures and patterns are original |
| `internal/lint/ar_manifest.go`, `internal/lint/ar_manifest_data.go`, `internal/lint/ar_manifest_test.go` | SC1, SC6, SC10 (AR054-AR056); SC10 and the semantic-version expression come from `src/skillspector/dependency_sources.py` and `src/skillspector/mcp_registry.py`, and some fixtures from `tests/nodes/analyzers/test_dependency_sources.py` and `test_dependency_source_budget_behavior.py` |

The rules AR046-AR050 (`internal/lint/ar_malware.go`), AR057 (`internal/lint/ar_mcpmeta.go`) and AR058
(`internal/lint/ar_declared.go`) are written from public threat-family behaviour. SkillSpector's YARA files
(`yara_rules/*.yar`) were read for reference when choosing which families to cover; no YARA text is copied (checked: no
common substring of 40 or more characters), and no rule name, description or string identifier is reused.

### Header for derived files

Put this block at the top of every Go file that contains derived rules or tests, ahead of the `package` clause.
Replace `<sha>` with the upstream commit the content was taken from (a full 40-character hash), list the upstream files
after it, and describe the changes on the second line.

```go
// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Derived from NVIDIA/SkillSpector at commit <sha> (<upstream files>).
// Modified by the ai-rulez authors: <what changed>.
// See THIRD_PARTY_NOTICES.md.
```

`scripts/transcode-skillspector-tests.py` writes this header for the tables it generates; the output is a fragment to
merge into a file that already carries the header once.

## Neo23x0 signature-base

SkillSpector's YARA rules state that they are based on [Neo23x0/signature-base](https://github.com/Neo23x0/signature-base),
which is licensed under the Detection Rule License (DRL) 1.1 (the `LICENSE` file of that repository).
ai-rulez does not copy those YARA files. A pattern that comes from such a rule is re-derived as an RE2 regular
expression and keeps the attribution DRL-1.1 asks for: the original rule name, its author and reference, and a link
to the license.

- **Derived files:** none yet. List each one as
  `<file>: rule <original rule> by <rule author>, signature-base at <upstream commit>`.

## Unicode confusables data

`internal/lint/ar_confusables.go` holds 84 mappings selected from the Unicode Consortium's confusables data
(UTS #39, `confusables.txt`), which SkillSpector also uses as its source. The table is not copied from SkillSpector.

Copyright (c) 1991-2026 Unicode, Inc. All rights reserved. Distributed under the Terms of Use in
<https://www.unicode.org/copyright.html>. Permission is hereby granted, free of charge, to any person obtaining a copy of
the Unicode data files and any associated documentation (the "Data Files") to deal in the Data Files without
restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, and/or sell
copies of the Data Files, provided that either (a) this copyright and permission notice appear with all copies of the
Data Files, or (b) this copyright and permission notice appear in associated Documentation. The Data Files are provided
"AS IS", without warranty of any kind. See <https://www.unicode.org/license.txt> for the full Unicode License v3.

## Redistribution

Release archives (`ai-rulez_<version>_<os>_<arch>.tar.gz` and `.zip`), the Homebrew formula and the npm and PyPI
packages include this file, `LICENSE` and `LICENSES/`, so the Apache-2.0 text accompanies the binary.
