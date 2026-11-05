# Embedding (Go API)

The package `github.com/Goldziher/ai-rulez/v5/pkg/airulez` runs the ai-rulez engine from Go: load a project, validate
it, plan or apply a generate run, and write or check its lock. Use it in a service that renders configuration for many
repositories, in a CI bot that reviews pull requests, or in tests that need generated output without a CLI process.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/Goldziher/ai-rulez/v5/pkg/airulez"
)

func main() {
	ctx := context.Background()

	ws := airulez.NewMemWorkspace()
	ws.Set(".ai-rulez/config.toml", "version = \"5.0\"\nname = \"billing\"\npresets = [\"claude\", \"cursor\"]\n", 0o644)
	ws.Set(".ai-rulez/rules/money.md", "# Money\n\nStore amounts as integer cents.\n", 0o644)

	project, err := airulez.Load(ctx, airulez.Options{Workspace: ws})
	if err != nil {
		log.Fatal(err)
	}
	plan, err := project.Plan(ctx, airulez.PlanOptions{})
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range plan.Files {
		fmt.Println(f.Action, f.Path)
	}
	fmt.Println("digest", plan.Digest)
}
```

```text
write .claude/settings.json
write .cursor/mcp.json
write .mcp.json
write AGENTS.md
write CLAUDE.md
digest 4cec194d5229d0f359d3e87078bb3a2ada5cc5e17c3c608c40c236116e00b3e1
```

Nothing was read from disk, no process was started and nothing was written.

The package is Experimental; see [Versioning](#versioning). Everything under `internal/` is private and carries no
compatibility promise.

## Workspaces

A `Workspace` is a read-only view of a project tree: an `io/fs` file system plus `Lstat` and `ReadLink`. Names are
slash-separated and relative to the root; `..` and absolute names are rejected.

| Constructor | Reads | Can be written by `Generate` |
| ----------- | ----- | ---------------------------- |
| `DirWorkspace(dir)` | A directory of the real file system | Yes |
| `NewMemWorkspace()` | Files you add with `Set(name, content, mode)` | No |
| `GitSnapshot(ctx, repoDir, rev, runner)` | The tree of a commit, with nothing checked out | No |

`GitSnapshot` runs git through the `Runner` you pass: `nil` uses the real git, `DenyAll()` makes the call fail.

- A symlink resolves only inside the workspace. A refused symlink is reported to `Options.Logger`, and validation
  fails on one in the project's own content.
- Machine-local files (`config.local.toml`, `.ai-rulez/local/`) are ignored unless you set `Options.WithLocal`. They
  carry the trust of the machine that wrote them, which a service loading someone else's tree should not grant. A
  snapshot never has them.

## Loading

`Load(ctx, Options)` reads the project the workspace holds. Every field except `Workspace` is optional.

| Field | Default | Meaning |
| ----- | ------- | ------- |
| `Workspace` | required | The project tree |
| `Runner` | `DenyAll()` | Starts the external commands a load needs (git, to resolve a remote include or to bundle skill files) |
| `Env` | empty | The environment the load reads (`MapEnv` is a fixed one) |
| `Clock` | wall clock | Stamps generated files that carry a time (`FixedClock(t)` pins it) |
| `Logger` | drops everything | Receives every warning of a load, validation or plan; a `*slog.Logger` satisfies it |
| `Remote` | `false` | Fetch the remote includes and installed skills the configuration declares, through `Runner` |
| `GitToken` | empty | Token for remote fetches; sent only to hosts the fetch code allowlists |
| `WithLocal` | `false` | Read the machine-local overlay and content of a directory workspace |

A load has no ambient authority: it reads no working directory, no process environment and starts no process unless
you pass them. No organization policy applies; the `--policy` and `AI_RULEZ_POLICY` settings of the command line do
not reach the library.

An `ai-rulez.lock` is enforced as by `ai-rulez generate`: when the lock exists and `[lock] enforce` is not `false`, a
remote source the lock does not cover fails `Load`.

Two `Project` values share no state, so a service can plan different projects concurrently. The operations of one
`Project` are serialized; load one per request or per worker.

`Project.Name()` returns the project name and `Project.Presets()` the configured presets in order.

## Planning

`Project.Plan(ctx, PlanOptions{Profile, Role})` lists every file a generate run would write, merge into or remove,
and writes nothing. An empty `Profile` is the configured default; `Role` renders a role's slice instead.

| Field | Meaning |
| ----- | ------- |
| `Files` | One `PlanFile` per output, sorted by path: `Path`, `Action` (`write`, `merge` or `mkdir`), `Mode` (octal), `Size` and `SHA256` of the rendered content, and the flags `LocalOnly`, `Sensitive` (may carry a secret; no size or digest) and `Raw` |
| `Removals` | What a run takes back: `Path` and `Reason` (`stale`, `unmerge` or `delete`) |
| `Digest` | SHA-256 of the canonical plan document |
| `JSON()` | The plan document, indented and deterministic, as described by `schema/plan.schema.json` and printed by `ai-rulez generate --emit-plan` |

The plan reads existing outputs from the workspace too: the previous manifest, merged documents such as
`.claude/settings.json`, and hand-edited files. A workspace in memory or in a commit therefore plans its own removals
the way a directory does. As a consequence `Digest` covers the sources *and* the existing outputs: the same sources in
an empty directory and in one holding earlier outputs give different digests.

## Generating

`Project.Generate(ctx, GenerateOptions{Profile, Mode})` applies a plan:

| Mode | Does | Result |
| ---- | ---- | ------ |
| `Write` | Writes the outputs; directory workspaces only | `Written`, the number of files written |
| `DryRun` | Lists the actions, writes nothing | `Lines` |
| `Check` | Reports files that differ from the plan, writes nothing | `Drift`, each with `Path` and `Kind` (`missing`, `stale`, `edited`, `orphan` or `blocked`) |

These are the same appliers as `generate`, `generate --dry-run` and `generate --check`.

```go
ws, err := airulez.DirWorkspace("/srv/checkouts/billing")
if err != nil {
	log.Fatal(err)
}
project, err := airulez.Load(ctx, airulez.Options{Workspace: ws})
if err != nil {
	log.Fatal(err)
}
res, err := project.Generate(ctx, airulez.GenerateOptions{Mode: airulez.Check})
if err != nil {
	log.Fatal(err)
}
for _, d := range res.Drift {
	fmt.Println(d.Kind, d.Path)
}
```

`Write` on a memory or snapshot workspace fails with `CodeDiskRequired` before anything is read or written.

## Validating

`Project.Validate(ctx, ValidateOptions{Strict})` checks the configuration and returns a `Report` of `Finding`s
(`Code`, `Severity`, `File`, `Line`, `Message`); `Report.OK()` is true when nothing was found. `Strict` here means "run
the content checks", unlike the CLI's `validate --strict` (which only fails on warnings): with it, `Validate` also
runs the content and security checks with their stable `AR` codes, which needs a directory workspace. Git, used to
list tracked files, runs through `Options.Runner`; with the default `DenyAll()` the directory is walked instead. A
canceled context stops validation between steps.

## Locks

`Project.Lock(ctx, LockOptions)` writes `.ai-rulez/ai-rulez.lock` as `ai-rulez lock` does, with the same gates: the
minimum release age of version ranges, the security scan (`AR001`-`AR009`) of every remote tree about to be pinned and
of every served skill, the deny list (`AR717`) and the carry-over of approvals. The zero `LockOptions` pins everything.

| Field | Meaning |
| ----- | ------- |
| `Names` | Refresh only these remote includes, installed skills, skill sources or served skills |
| `Kind` | Refresh only one kind: `include`, `skill`, `source` or `served` |
| `ContentOnly` | Re-pin authored content and outputs only, keeping the remote pins, without the network |
| `Profile` | Profile whose outputs are pinned; empty is the one the lock recorded, else the default |
| `Roles` | Also pin the rendered outputs of every role |
| `AcceptFindings` | Pin a remote tree whose scan has error findings |
| `Strict` | Fail when the scan refuses a served skill, instead of leaving it unpinned |
| `ToolVersion` | ai-rulez version recorded in the lock |

The result has the lock `Path`, the `Tree` digest over every content pin, and `Unpinned`: the served skills the scan
refused and left out (without `Strict`).

- A remote tree with error findings, or under `Strict` a refused served skill, fails with `CodeFindings` and writes
  nothing. `errors.As(err, &fe)` with a `*airulez.FindingsError` gives each one as a `LockFinding` with its `Kind`,
  `Name`, `View` and `AR` code.
- Remote sources are re-resolved only when the project was loaded with `Options.Remote`; release times are then also
  asked of the forge over HTTPS.
- The `Project` keeps the configuration it was loaded with. Load again to plan against the new lock.

`Project.LockCheck(ctx, LockCheckOptions{Profile, ToolVersion})` is `ai-rulez lock --check` without the network. It
compares the lock with the sources, the rendered outputs and the cached remote content and returns `LockStatus`:
`InSync`, `Changes` (one line per difference) and `Notes` (differences that do not fail the check). Cached content
that disagrees with the lock is drift, not an error. The remote tag and signature checks of the command are not part of
it. A project with no lock and no `[lock] enforce` fails with `CodeLock`.

`Lock` and `LockCheck` need a directory workspace; any other fails with `CodeDiskRequired`.

## Running commands

The engine starts external commands only through a `Runner`:

```go
type Runner interface {
	Run(ctx context.Context, spec Spec) Result
}
```

`Spec` carries `Argv`, `Dir`, `Env` (the complete child environment unless `InheritEnv`), `Stdin`, `Timeout` and
`MaxOutput`. `Result` carries a `Status` (`ok`, `exit`, `timeout`, `unavailable` or `error`), `ExitCode`, `Stdout`,
`Stderr`, truncation flags and timing; a failure is a status, never a Go error. Implement it to deny, record, sandbox
or fake commands. `DenyAll()` refuses every one and is the default.

## Errors

Failures a caller needs to tell apart are `*airulez.Error` values with a stable `Code`; the cause is reachable with
`errors.Is` and `errors.As`.

| Code | Constant | Meaning |
| ---- | -------- | ------- |
| `load` | `CodeLoad` | The project could not be loaded |
| `plan` | `CodePlan` | The plan could not be rendered |
| `validate` | `CodeValidate` | Validation could not run (as opposed to findings) |
| `apply` | `CodeApply` | A generate run failed while applying its plan |
| `disk-required` | `CodeDiskRequired` | `Generate` with `Write`, strict `Validate`, `Lock` or `LockCheck` on a workspace that is not a directory; `errors.Is(err, airulez.ErrDiskRequired)` |
| `refused` | `CodeRefused` | A run would overwrite a file ai-rulez cannot prove it wrote (a hand-written file or a symlink); nothing was written; `errors.Is(err, airulez.ErrRefused)` |
| `lock` | `CodeLock` | `Lock` or `LockCheck` could not run |
| `findings` | `CodeFindings` | The security scan refused content while locking; the cause is a `*FindingsError` |

```go
_, err = project.Generate(ctx, airulez.GenerateOptions{Mode: airulez.Write})
var e *airulez.Error
if errors.As(err, &e) && e.Code == airulez.CodeDiskRequired {
	// plan only: this workspace lives in memory
}
```

## Not in the API

The agent subcommand, file watching, the `init` prompts, the scanners, usage recording, the eval runners and the MCP
server are available only from the command line.

## Versioning

- Minor releases add to the API; a breaking change comes only with a new major version and a new `/vN` module path.
- Every API is marked Experimental in its godoc for its first minor release, then follows these rules.
- The plan document has its own `schema` number: a new field is additive, a removed or re-typed field raises the
  number and is announced in the [changelog](CHANGELOG.md).
