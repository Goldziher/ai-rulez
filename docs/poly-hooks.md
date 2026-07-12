# Poly Hooks

AI-Rulez publishes a reusable Poly hook catalog in `poly-hooks.toml`. Consumer
repositories declare Git or local sources in their existing `poly.toml` and select
the hooks they need. Consumers do not create another `poly-hooks.toml`.

This integration requires AI-Rulez 4.9.0 or later and Poly 0.14.0 or later.

## Add AI-Rulez to a repository

Add a Git source to `poly.toml`:

```toml
[[hooks.sources]]
id = "ai-rulez"
git = "https://github.com/Goldziher/ai-rulez.git"
revision = "v4.9.0"
hooks = ["ai-rulez-validate"]
```

Use `ai-rulez-validate` as the non-mutating default. Plugin-producing repositories
should also verify their generated bundles:

```toml
hooks = ["ai-rulez-validate", "ai-rulez-plugin-verify"]
```

Every Git source requires an explicit revision and a nonempty hook selection.
Unknown hook IDs are rejected. Hooks added to a future version of the producer
catalog never activate unless you select them.

Resolve the revision, commit the lock, and install the Git shims:

```bash
poly hooks update
git add poly.toml poly-hooks.lock
poly hooks install
```

Normal hook runs use the locked commit and never move Git references. Run
`poly hooks update` explicitly after changing `revision` or when intentionally
refreshing a branch reference.

## Available hooks

| Hook | Command arguments | Purpose |
|------|-------------------|---------|
| `ai-rulez-validate` | `generate --dry-run` | Validate generation without writing files |
| `ai-rulez-generate` | `generate` | Regenerate the root project |
| `ai-rulez-recursive` | `generate --recursive` | Regenerate every project in a repository |
| `ai-rulez-plugin-generate` | `generate --recursive --plugin --if-configured` | Regenerate plugin producers and marketplaces |
| `ai-rulez-plugin-verify` | `verify --recursive --plugin --if-configured` | Verify plugin provenance without writing files |
| `ai-rulez-enforce` | `enforce --level error` | Enforce configured source-code rules |
| `ai-rulez-enforce-fix` | `enforce --fix --level error` | Enforce rules and apply fixes |

Generation and plugin hooks trigger for root or nested `.ai-rulez/` changes.
Enforcement hooks trigger for supported source-code extensions.

## Choose an execution path

Each hook provides guarded npx, uvx, and system execution paths. Configure their
order for the current machine in the gitignored `poly.local.toml`:

```toml
[hook_preferences]
channels = ["npx", "uvx", "system"]
```

Poly evaluates the configured channels in order and uses the first catalog path
whose `check` command succeeds. The npx and uvx paths install AI-Rulez on demand;
the system path uses an existing `ai-rulez` executable. `poly hooks install` runs
the selected path's optional `install` command to prepare its cache. A normal hook
run invokes `run` directly, which also installs on demand for npx and uvx.

Poly does not silently choose a channel when external hook sources are configured.
A missing preference list or a source with no eligible path is an error. If the
selected AI-Rulez command fails, the hook fails without falling through to another
path.

## Use a local source

For hook development, replace `git` and `revision` with `path`:

```toml
[[hooks.sources]]
id = "ai-rulez"
path = "../ai-rulez"
hooks = ["ai-rulez-validate"]
```

Local paths may be repository-relative, parent-relative, or absolute. Poly
canonicalizes the path, loads the producer catalog on every run, and does not create
a lock entry for it. A source must specify exactly one of `git` or `path`.

## Producer catalog

Only the producer repository owns `poly-hooks.toml`. A catalog contains one or more
hooks, and every hook contains at least one guarded execution path:

```toml
version = 1

[[hooks]]
id = "ai-rulez-validate"
stages = ["pre-commit"]
args = ["generate", "--dry-run"]
files = [".ai-rulez/**", "**/.ai-rulez/**"]
workspace = true
pass_filenames = false

[[hooks.paths]]
channel = "npx"
check = "command -v npx"
run = "npx -y ai-rulez@latest"
install = "npx -y ai-rulez@latest version"

[[hooks.paths]]
channel = "uvx"
check = "command -v uvx"
run = "uvx ai-rulez"
install = "uvx ai-rulez version"

[[hooks.paths]]
channel = "system"
check = "command -v ai-rulez"
run = "ai-rulez"
```

Poly appends the hook's `args` to the selected path's `run` command. `install` is
optional because paths such as `system` use an already-installed executable.
Workspace hooks run from the consumer repository root. Duplicate hook IDs, duplicate
channels within a hook, empty paths, and unsupported catalog versions are errors.

## Recursive plugin checks

Plugin authoring means a `[plugin]` block or a `[marketplace]` block with members.
Consumer-only `[[plugins]]` and `[[marketplaces]]` declarations do not count.

Recursive plugin commands treat a marketplace root atomically: the root renders or
verifies its member plugins and aggregate indexes, while member configs are excluded
from duplicate concurrent work. Standalone plugin producers are processed normally.
`--if-configured` only turns the absence of an authoring config into success; real
generation and verification errors still fail the hook.

!!! warning
    Hook catalogs execute commands with your user permissions. Review source and lock
    changes before running or installing hooks.
