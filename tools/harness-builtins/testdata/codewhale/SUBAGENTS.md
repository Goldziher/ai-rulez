<!-- Excerpt of https://github.com/codewhale-hq/Codewhale/blob/v0.10.1/docs/SUBAGENTS.md -->


| Role          | Stance                                 | Writes? | Network? | Shell posture | Typical use                                  |
|---------------|----------------------------------------|---------|----------|---------------|----------------------------------------------|
| `general`     | flexible; do whatever the parent says  | yes     | yes      | yes           | the default; multi-step tasks                |
| `explore`     | read-only; map the relevant code fast  | no      | yes      | bounded inspection | "find every call site of `Foo`; check the PR with gh" |
| `planner`     | analyse and produce a strategy         | no      | yes      | read-only probes | "design the migration; don't execute"        |
| `reviewer`    | read-and-grade with severity scores    | no      | yes      | bounded inspection | "audit this PR for bugs"                     |
| `implement`   | land a specific change with min edit   | yes     | yes      | yes           | "rewrite `bar.rs::Foo::bar` to do X"         |
| `test`        | run tests / validation, report outcome | no      | yes      | bounded verification (no writes) | "verify the diff with the bounded test checks; report PASS/FAIL" |
| `advisor`     | short-lived, high-reasoning counsel     | no      | yes      | none          | "what are we missing in this design?"        |
| `custom`      | explicit narrow tool allowlist         | inherits | inherits | inherits     | hand-picked tools on the parent's posture    |


Prose between.

### Aliases

The model can spell each role multiple ways:

| Canonical     | Aliases                                                          |
|---------------|------------------------------------------------------------------|
| `general`     | `worker`, `default`, `general-purpose`, `general_purpose`         |
| `explore`     | `scout`, `explorer`, `exploration`                               |
| `planner`     | `plan`, `planning`, `awaiter`                                    |
| `reviewer`    | `review`, `code-review`, `code_review`                           |
| `implement`   | `builder`, `implementer`, `implementation`                       |
| `test`        | `verifier`, `verify`, `verification`, `validator`, `tester`       |
| `advisor`     | `consultant`, `oracle` (compatibility input only)                 |
| `custom`      | (none; explicit `allowed_tools` array required)                  |

