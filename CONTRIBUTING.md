# Contributing to ai-rulez

Thank you for your interest in contributing! This guide provides everything you need to get started with development.

## Getting Started

### Prerequisites

- **Go 1.27.2+**, matching `go.mod`
- **A C toolchain and cgo** for the liter-llm native library
- **[Task](https://taskfile.dev)** for build scripts
- **Poly** for linting, formatting and Git hooks
- **Python 3.11+** for governance hook setup (`tomllib`)
- **The reviewed ai-rulez CLI source build pinned in [CI](.github/workflows/ai-rulez-validation.yml)**, available on `PATH` before installing governance hooks

Node.js 22+ and pnpm are needed when working on the npm wrapper. Documentation builds use Python and uv.

### Setup

Install the prerequisites first. The `setup` task installs golangci-lint, configures governance hooks and downloads Go modules; it does not install Poly, Python or the ai-rulez CLI.

```bash
# 1. Clone the repository
git clone https://github.com/Goldziher/ai-rulez.git
cd ai-rulez

# 2. Run the setup task
task setup
```

---

## Pre-commit hooks

Install the git hooks with `task setup` (or `poly hooks install` directly). On
every commit, poly runs lint, format, and file-safety checks; the commit-msg
hook validates the message. Run all hooks manually with
`poly hooks run pre-commit --all-files`.

## Repository governance and public skills

Author contributor guidance in `.ai-rulez/`. The root `config.toml` selects
`config.plugins.toml` through `default_layers = ["plugins"]`; the latter holds
consumer marketplace declarations. The native CLI owns the generated headers and
machine-local ownership sidecars. Use the reviewed source revision pinned in CI,
then run `task governance:check` after editing sources. Install its system-channel
Poly hooks with `task governance:hooks:install`; preserve existing machine preferences.

The installable product skill is under `skills/ai-rulez/`, separate from internal
maintainer skills under `.ai-rulez/skills/`. Audit its CLI examples and resource
references when changing a public command. Version 5.4.0 includes the governance fixes developed on `main`;
the CI source pin identifies the reviewed implementation used for plugin validation.

## Architectural Overview

`ai-rulez` is designed with a clean, layered architecture that separates data, logic, and presentation. Understanding this structure is key to contributing effectively.

### Core packages

- **`internal/config`** defines configuration types, loads TOML configuration and reads the content tree. YAML remains in Markdown frontmatter, eval cases and legacy migration inputs.
- **`internal/project`** assembles project loading, includes and installed skills.
- **`internal/crud`** shares content and configuration mutations between the CLI and MCP server. Its `Operator` interface is in `internal/crud/crud.go`; content operations and file handling live in `content.go`, `files.go` and related files.
- Domain packages own the other behavior: generation in `internal/generator`, validation in `internal/lint`, model review in `internal/judge`, and locking in `internal/lockfile` and `internal/lockrun`. See [Architecture](docs/architecture.md) for the full package map.
- **`pkg/airulez`** exposes the public Go API over explicit workspaces. Preserve its documented API and isolation guarantees when changing shared internals.

### CLI and MCP adapters

Cobra commands live directly under `cmd/commands`; `root.go` registers the top-level commands. CRUD commands use
`add.go`, `remove.go`, `list.go` and `show_edit.go`. MCP tool definitions live in `internal/mcp/tools.go` and related
tool files, with handlers under `internal/mcp/handlers`.

Keep input parsing and response formatting in these adapters. Put reusable behavior in the package that owns the
operation, then call that shared implementation from both interfaces where the feature is exposed.

## How to Add a New CRUD Operation

1. Inspect the closest existing operation and its tests. Extend the relevant config or content types and schemas under `schema/` only when the data format changes; content items do not all require a new field on `Config`.
2. Implement the mutation in `internal/crud`, extending `Operator` when appropriate. Preserve path confinement, the `.agents` source behavior, local overlay rules and atomic writes.
3. Expose the operation through the appropriate `add`, `remove`, `list`, `show` or `edit` command in `cmd/commands`. Follow its existing subcommand registration pattern; register a new top-level command in `cmd/commands/root.go` only when needed.
4. If MCP exposes the operation, update its tool definition and handler under `internal/mcp`. Preserve read-only mode restrictions and tool annotations.
5. Add tests beside the implementation and cover the public interfaces in `tests/e2e/cli/crud_test.go` and `tests/e2e/mcp_server_test.go` as appropriate. Update report schemas, CLI documentation and the public skill when their contracts change.

## Development Workflow

### Building and Testing

The project uses [Task](https://taskfile.dev) for all build and test operations.

```bash
# Build the binary to ./bin/ai-rulez, linking liter-llm statically as a release does
# (downloads and sha256-verifies the library once; needs cgo and a C toolchain)
task build

# Run all unit tests
task test

# Run all end-to-end tests
task test:e2e

# Run the checks CI runs, before committing
task lint       # poly fmt --check + poly lint
task check      # go vet + golangci-lint
task test:all   # unit + platform + e2e
```

### Updating golden files

A golden-file test fails with a diff when its output changes on purpose. Rewrite the files, then review the diff before committing:

```bash
task test:golden:update
```

Most packages read `UPDATE_GOLDEN=1`; the task sets it and reruns those packages. The other regeneration switches are per test and documented in the failure message:

| Variable or flag | Rewrites | Run |
| --- | --- | --- |
| `UPDATE_GOLDEN=1` | golden files of the packages above | `UPDATE_GOLDEN=1 go test ./path/to/pkg` |
| `-update-tokens-golden` | the tokens report golden | `go test ./internal/generator -run Golden_TokenReportJSON -update-tokens-golden` |
| `UPDATE_DOCS=1` | generated tables in `docs/` | `UPDATE_DOCS=1 go test ./internal/lint -run TestRuleReferenceDoc` |
| `UPDATE_SCHEMA=1`, `UPDATE_LOCAL_SCHEMA=1` | preset enums and the local schema in `schema/` | `UPDATE_SCHEMA=1 go test ./schema -run TestPresetEnums` |
| `UPDATE_ALLOWLIST=1` | the architecture-lint allowlist | `UPDATE_ALLOWLIST=1 go test ./tests/archlint` |

### Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/); the commit-msg hook validates the format.

```bash
# Good commit messages
feat(cli): add crud commands for 'new-entity'
fix(mcp): correct parameter handling in update_rule tool
docs(contributing): clarify project architecture
```

### Derived rules and tests

ai-rulez is MIT licensed. A rule, pattern or test derived from a project under another license, such as
[NVIDIA SkillSpector](https://github.com/NVIDIA/SkillSpector) (Apache-2.0), must keep that license's conditions:

1. **Keep the upstream header and mark the change.** Retain the original copyright and SPDX lines, and state that the
   file was modified (Apache-2.0 section 4(b)). Record the upstream repository, path and commit in the header:

   ```go
   // SPDX-License-Identifier: Apache-2.0
   // Copyright 2026 NVIDIA CORPORATION & AFFILIATES
   //
   // Derived from https://github.com/NVIDIA/SkillSpector/blob/<commit>/<path>
   // at commit <full-commit-sha>.
   // Modified by the ai-rulez contributors: <one line on what changed>.
   // The modifications are licensed under the MIT License; see LICENSE.
   ```

   Use the comment syntax of the file (`#` for TOML or YAML, `<!-- -->` for Markdown). Copy the copyright line from the
   upstream file; if it has none, use the one in the upstream `LICENSE`.

2. **Add a notices entry.** List the file, upstream path and commit under its project in
   [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Keep the full text of each upstream license under `LICENSES/`
   (`LICENSES/Apache-2.0.txt` for Apache-2.0), copied unmodified from the official source; add it with the first file
   derived under that license.

3. **Do not copy YARA rules from signature-base.** SkillSpector's YARA files are based on
   [Neo23x0/signature-base](https://github.com/Neo23x0/signature-base) (DRL-1.1). Re-derive such a pattern as an RE2
   regular expression instead of copying the file, and credit the original rule name, its author and reference, and the
   DRL-1.1 license in the header and in `THIRD_PARTY_NOTICES.md`.

Contributions that go the other way, from ai-rulez to an upstream project, follow that project's terms; SkillSpector
requires a DCO sign-off (`git commit -s`). This does not apply to commits in this repository.

### Pull Request Process

1.  Create a feature branch from `main`.
2.  Make your changes, following the architectural guidelines.
3.  Add or update unit and E2E tests for your changes.
4.  Ensure all checks pass by running `task lint`, `task check`, and `task test:all`.
5.  Push your branch and open a pull request with a title that follows the Conventional Commit format.

---

## Releasing

A version tag push triggers [the publish workflow](.github/workflows/publish.yaml). Follow the complete
[maintainer release guide](docs/maintainers/release.md), including version synchronization, golden updates and verification.

1. Run `task set-version -- X.Y.Z`, `scripts/release-bump.sh check X.Y.Z` and `uv lock`. Update the changelog and generated documentation, commit the release changes, and wait for CI on `main` to pass.
2. Create a signed tag at that verified commit and push it:

   ```bash
   git tag -s vX.Y.Z -m 'Release vX.Y.Z'
   git push origin vX.Y.Z
   ```

3. The workflow creates or reuses a draft GitHub release, builds and smoke-tests each native platform archive, and uploads checksums and provenance. Linux and macOS use static linking; Windows can ship a DLL beside the executable. Only after the artifacts are complete does it publish the release, npm and PyPI packages, and Homebrew formula.

A draft created through `gh release create --verify-tag --draft` after the signed tag is pushed is compatible with
this workflow. Check for an existing draft first because the workflow may already have created it. Keep it a draft
until the workflow completes its artifact checks. The release versions come from the committed manifests and tag;
the workflow checks their consistency before publishing.

---

## Private Registry Distribution

Community contributors can build and publish their own npm packages to private registries for internal distribution or testing before PR merge.

### How It Works

The npm package supports an **offline mode**: if platform-specific binaries are present in the `bin/` directory, they will be used directly instead of downloading from GitHub.

| Scenario         | `bin/` directory  | Behavior                             |
| ---------------- | ----------------- | ------------------------------------ |
| Public npm       | Empty             | Downloads from GitHub Releases       |
| Private registry | Contains binaries | Uses local binaries, no network call |

### Build and Publish

```bash
# 1. Build binaries natively on each Linux/macOS target (cgo + the liter-llm static library).
#    Targets: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64; no cross-compiling.
eval "$(bash scripts/setup-liter-llm.sh)"   # exports CGO_ENABLED=1 and CGO_LDFLAGS (verified, static)
go build -o release/npm/bin/ai-rulez-darwin-arm64 ./cmd/ai-rulez   # on a darwin/arm64 host
# ... repeat on a host of each other Linux/macOS target

# 2. (Optional) Modify package.json for your scope/version
cd release/npm
# Edit name to "@yourscope/ai-rulez", adjust version if needed

# 3. Publish to your private registry
pnpm publish --registry=https://your-private-registry/

# 4. Install and use
pnpm install @yourscope/ai-rulez --registry=https://your-private-registry/
ai-rulez generate  # Uses local binary, no GitHub access needed
```

For Windows, use `scripts/package-release.sh windows/amd64` with `VERSION` set, as described in the
[release guide](docs/maintainers/release.md). Name the packaged executable `ai-rulez-windows-amd64.exe`;
if packaging uses the DLL fallback, include `liter_llm_ffi.dll` beside it under `release/npm/bin/`.

### Binary Naming Convention

Binaries must follow this naming pattern: `ai-rulez-{os}-{arch}[.exe]`

- **os**: `linux`, `darwin`, `windows`
- **arch**: `amd64`, `arm64` (Windows supports `amd64` only)
- **.exe**: Required for Windows only
