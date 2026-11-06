---
title: "Poly hooks"
description: "Run ai-rulez generate --check and validate on commit with Poly, using the hook catalog in poly-hooks.toml."
---

ai-rulez publishes a hook catalog for [Poly](https://github.com/Goldziher/poly), `poly-hooks.toml` at the root of
its repository. Point your `poly.toml` at it and select the hooks you need to validate the configuration, check for
drift or regenerate output before each commit. The catalog needs Poly 0.14.0 or later.

## Example

Add the source to your repository's `poly.toml`:

```toml
[[hooks.sources]]
id = "ai-rulez"
git = "https://github.com/Goldziher/ai-rulez.git"
revision = "v5.11.1"
hooks = ["ai-rulez-validate", "ai-rulez-check"]
```

Choose how the hook runs ai-rulez in your gitignored `poly.local.toml`:

```toml
[hook_preferences]
channels = ["system", "npm"]
```

Resolve the revision, commit the lock and install the Git shims:

```bash
poly hooks update
git add poly.toml poly-hooks.lock
poly hooks install
```

Every commit now runs `ai-rulez validate --no-local` and `ai-rulez generate --check --no-local`.

## Available hooks

| Hook | Runs | Purpose | Triggers on |
| --- | --- | --- | --- |
| `ai-rulez-validate` | `validate --no-local` | Validate the configuration and content | `.ai-rulez/` changes |
| `ai-rulez-check` | `generate --check --no-local` | Fail when generated files are out of date | every commit |
| `ai-rulez-generate` | `generate --no-local` | Regenerate the root project | `.ai-rulez/` changes |
| `ai-rulez-recursive` | `generate --recursive --no-local` | Regenerate every project in the repository | `.ai-rulez/` changes |
| `ai-rulez-plugin-generate` | `generate --recursive --plugin --if-configured` | Regenerate plugin bundles and marketplaces | `.ai-rulez/` changes |
| `ai-rulez-plugin-verify` | `verify --recursive --plugin --if-configured` | Verify plugin bundles without writing | every commit |

`.ai-rulez/` changes means the root `.ai-rulez/` and any nested one. `ai-rulez-validate` is the non-mutating default;
repositories that publish plugins add `ai-rulez-plugin-verify`:

```toml
hooks = ["ai-rulez-validate", "ai-rulez-check", "ai-rulez-plugin-verify"]
```

The two read-only checks, `ai-rulez-check` and `ai-rulez-plugin-verify`, run on every commit, including one that
only deletes files, so a change to a preset, a custom output path or a generated file cannot skip them.

`--if-configured` makes the plugin hooks succeed when the repository has no [plugin authoring](/ai-rulez/plugins/)
configuration: a `[plugin]` block, a `[marketplace]` with `members` or `[marketplace.from_domains]`, or
`[[plugins_authoring]]` / `[[marketplaces_authoring]]` entries. `[[plugins]]` and `[[marketplaces]]` declarations write
nothing and do not count. Real generation or verification errors still fail the hook. A marketplace root renders or
verifies its member plugins and indexes together.

### Machine-local configuration in hooks

The `validate` and `generate` hooks pass `--no-local`: they use the shared view a teammate or CI sees and ignore your
gitignored `config.local.*` overlay and `.ai-rulez/local/` content. Machine-local outputs are left in place; run
`ai-rulez generate` without the flag to refresh them. Plugin generation and verification always use the shared view.
Do not add `--allow-local-drift` to a hook. See [Local configuration](/ai-rulez/local-overrides/#drift-guard).

## Sources

A Git source needs an explicit `revision` and a non-empty `hooks` list. An unknown hook ID is an error, and hooks added
to a later catalog never run unless you select them. Hook runs use the commit locked in `poly-hooks.lock`; run
`poly hooks update` after you change `revision`.

For hook development, use a local checkout instead of `git` and `revision`:

```toml
[[hooks.sources]]
id = "ai-rulez"
path = "../ai-rulez"
hooks = ["ai-rulez-validate"]
```

The path may be relative or absolute. Poly reads the catalog from it on every run and does not lock it. A source sets
exactly one of `git` and `path`.

## Execution channels

Each hook has two channels:

| Channel | Check | Runs |
| --- | --- | --- |
| `system` | `command -v ai-rulez` | the installed `ai-rulez` |
| `npm` | `command -v npx` | `npx -y ai-rulez@latest` |

Poly tries the channels in the order of `[hook_preferences] channels` and runs the first whose check succeeds. With an
external hook source configured, a missing preference list, or a source with no eligible channel, is an error. When
the selected command fails, the hook fails; it does not fall through to the next channel.

The catalog revision pins the hook definitions, not the ai-rulez version. `system` runs whatever version is installed,
so install the version you reviewed in local setup and CI. `npm` runs whatever `latest` resolves to; list only
`system` when a review gate must not move with a release.

:::caution
Hook catalogs run commands with your user permissions. Review changes to the source and the lock before you run
or install hooks.
:::

## Catalog format

Only the producer repository has a `poly-hooks.toml`; consumers do not create one. Each hook has at least one
execution path, and Poly appends the hook's `args` to the chosen path's `run` command:

```toml
version = 1

[[hooks]]
id = "ai-rulez-validate"
stages = ["pre-commit"]
args = ["validate", "--no-local"]
files = [".ai-rulez/**", "**/.ai-rulez/**"]
workspace = true
pass_filenames = false

[[hooks.paths]]
channel = "system"
check = "command -v ai-rulez"
run = "ai-rulez"

[[hooks.paths]]
channel = "npm"
check = "command -v npx"
run = "npx -y ai-rulez@latest"
```

Workspace hooks run from the repository root. Duplicate hook IDs, duplicate channels within a hook, empty paths and
unsupported catalog versions are errors.

The root hooks check the repository's own configuration once; they do not treat plugin-authoring configurations as
projects of their own, which `ai-rulez-plugin-verify` covers.
