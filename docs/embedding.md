# Embedding ai-rulez (Go API)

`github.com/Goldziher/ai-rulez/v5/pkg/airulez` loads a project, validates it and plans or applies a generate run from Go, inside a process that may serve many projects at once. It is **Experimental** for its first minor release: after that, `pkg/...` changes only additively inside a major version (CI runs `gorelease` against the last release). Everything under `internal/` is private.

```go
ws := airulez.NewMemWorkspace()
ws.Set(".ai-rulez/config.toml", "version = \"4.0\"\nname = \"svc\"\npresets = [\"claude\"]\n", 0o644)
ws.Set(".ai-rulez/rules/style.md", "# Style\n\nBe concise.\n", 0o644)

project, err := airulez.Load(ctx, airulez.Options{Workspace: ws})
plan, err := project.Plan(ctx, airulez.PlanOptions{})
fmt.Println(plan.Digest, len(plan.Files))
```

## Workspaces

A `Workspace` is the read view of a project tree (an `io/fs` file system plus `Lstat` and `ReadLink`). Three implementations ship:

| Constructor | Reads |
|---|---|
| `DirWorkspace(dir)` | a directory of the real file system |
| `NewMemWorkspace()` | files you `Set` in memory |
| `GitSnapshot(ctx, repoDir, rev, runner)` | the tree of a commit, nothing checked out |

A symlink resolves only inside the workspace. A snapshot has no machine-local files, like `--no-local`. A directory workspace ignores them too unless you set `Options.WithLocal`: machine-local files (`config.local.toml`, `.ai-rulez/local/`) carry the trust of the machine that wrote them, which a service loading someone else's tree should not grant.

## Plans

`Project.Plan` lists every file a run would write, merge into or remove, with a digest of the rendered content, and writes nothing. `Plan.Digest` is the SHA-256 of the canonical plan document (`schema/plan.schema.json`, also what `ai-rulez generate --emit-plan` prints): equal sources give equal digests whichever workspace they came from. The plan reads what already exists from the workspace itself: the previous manifest, merged documents such as `.claude/settings.json` and hand-edited outputs. A workspace in memory or in a commit therefore plans its own `removals` (stale files, entries taken back out of merged documents) the way a directory does. A machine-local record found in a commit is not believed, because a commit holds whatever its author chose. `Lock` is not part of the API yet: the content lock still reads the disk.

`Project.Generate` applies a run: `Write` (directory workspaces only; anything else fails with `CodeDiskRequired` before touching anything), `DryRun` (the action list) or `Check` (files that differ from the plan). The same appliers back `generate`, `generate --dry-run`, `generate --check` and `generate --emit-plan`.

`Project.Validate` checks the configuration; `ValidateOptions.Strict` also runs the content and security checks (stable `AR` codes) and needs a directory workspace. Git, used to index tracked files, runs through `Options.Runner`; with the default `DenyAll` the directory is walked instead.

## No ambient authority

Loading and planning use no working directory, no process environment, no clock and no subprocess unless you give them: pass `Options.Env`, `Options.Clock`, `Options.Runner` (default `DenyAll`; implement the `Runner` interface with the public `Spec` and `Result` types to deny, record or sandbox commands) and `Options.Logger` (every warning of a load, validation or plan goes there, and a service that gives none gets none). A load is bound by no organization policy unless one is given to it; the process-wide policy of the command line does not reach a library user. Remote includes and installed skills are fetched only with `Options.Remote`, through your `Runner`, with `Options.GitToken`. Two `Project`s share no state, so a service can plan different projects concurrently; the operations of one `Project` are serialized.

## Not in the API

The agent subcommand, file watching, `init` prompts, scanners, usage recording, the eval runners and the MCP server stay in the command line.

## Versioning

- Additive changes in minor releases; breaking changes only with a new major version and `/vN` path.
- New surface is `Experimental` for one minor release, marked in its godoc.
- The plan document has its own `schema` number: a new field is additive, a removed or re-typed one raises it and is announced in the changelog.
- Errors are `*airulez.Error` with a stable `Code` (`load`, `plan`, `validate`, `apply`, `disk-required`); the cause is reachable with `errors.Is` and `errors.As`.
