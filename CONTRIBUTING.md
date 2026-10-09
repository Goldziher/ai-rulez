# Contributing to ai-rulez

Thank you for your interest in contributing! This guide provides everything you need to get started with development.

## Getting Started

### Prerequisites

- **Go 1.27+**
- **Node.js 22+** (for commit hooks)
- **[Task](https://taskfile.dev)** (for running build scripts)

### Setup

The fastest way to set up your development environment is to use the `setup` task. This command installs all necessary dependencies and configures Git hooks for you.

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

## Architectural Overview

`ai-rulez` is designed with a clean, layered architecture that separates data, logic, and presentation. Understanding this structure is key to contributing effectively.

### The Core: `internal/config` and `internal/crud`

- **`internal/config`**: This is the single source of truth for the application's data structures. All YAML parsing and the definitions for `Rule`, `Agent`, `MCPServer`, etc., live here.

- **`internal/crud`**: This package contains the **centralized, shared logic** for all Create, Read, Update, and Delete (CRUD) operations. It takes simple data structures, modifies the configuration in memory, and handles writing back to the `ai_rulez.yaml` file. **Nearly all business logic should be in this package.**

### The Presentation Layers: `cmd` and `mcp`

The CLI and the MCP server are treated as thin "presentation layers." They are responsible for handling user/client input and calling the core `crud` logic. They should contain minimal business logic themselves.

- **`cmd/commands/crud`**: This is where the `cobra` CLI commands are defined. A typical command's only job is to parse flags and call the appropriate function from the `internal/crud` package.

- **`internal/mcp/handlers`**: This is where the MCP server's tool handlers are defined. A handler's only job is to parse incoming MCP parameters and call the appropriate function from the `internal/crud` package.

!!! success "The Golden Rule of Contributing"
When adding a new feature or fixing a bug, the logic should almost always be implemented in the `internal/crud` package first. The CLI command and the MCP handler should then be simple wrappers around that core logic. This ensures consistent behavior across both interfaces.

---

## How to Add a New CRUD Command

Here is the step-by-step process for adding CRUD operations for a new entity (e.g., `new_entity`):

1.  **Update the Schema:** Add the `new_entity` definition to `schema/ai-rules.schema.json`.
2.  **Update the Config Struct:** Add the `NewEntity` struct to `internal/config/types.go` and the `[]NewEntity` slice to the main `Config` struct.
3.  **Update the CRUD Logic:** Add a new case for `new_entity` in the main switch statement in `internal/crud/crud.go`.
4.  **Create the CLI Command File:** Create a new file, `cmd/commands/crud/new_entity.go`, and define the `cobra` commands (`Add`, `Get`, `List`, `Update`, `Delete`). These commands should parse flags and call the `crud` helper functions.
5.  **Register the CLI Commands:** Add the new commands to their parent commands (`AddCmd`, `GetCmd`, etc.) in `cmd/commands/root.go`.
6.  **Add MCP Tools & Handlers:** Add the tool definitions to `internal/mcp/tools.go` and create the handlers in a new `internal/mcp/handlers/new_entity.go` file. The handlers should simply call the `crud` helper functions.
7.  **Add Tests:** Add a new `TestNewEntityCRUD_FullCycle` test to `testing/e2e/cli/crud_test.go` and `testing/e2e/mcp_server_test.go`.

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

We use [Conventional Commits](https://www.conventionalcommits.org/). This is required for our automated release process.

```bash
# Good commit messages
feat(cli): add crud commands for 'new-entity'
fix(mcp): correct parameter handling in update_rule tool
docs(contributing): clarify project architecture
```

### Pull Request Process

1.  Create a feature branch from `main`.
2.  Make your changes, following the architectural guidelines.
3.  Add or update unit and E2E tests for your changes.
4.  Ensure all checks pass by running `task lint`, `task check`, and `task test:all`.
5.  Push your branch and open a pull request with a title that follows the Conventional Commit format.

---

## Releasing

Releases are fully automated using GitHub Actions and are triggered when a new tag is pushed to the `main` branch.

### How It Works

1.  **Tag Push**: Bump every version surface with `scripts/release-bump.sh set X.Y.Z`, merge it, then push a tag with the format `vX.Y.Z` (e.g., `v2.0.1`).

    ```bash
    git tag v2.0.1
    git push origin v2.0.1
    ```

2.  **CI/CD Pipeline**: The push event triggers the `.github/workflows/publish.yaml` workflow, which handles the entire release process:
    - **Native builds**: Each platform archive is built on its own runner (cgo, with the liter-llm native library linked statically), checksummed, attested and attached to a GitHub Release. See [docs/maintainers/release.md](docs/maintainers/release.md).
    - **Homebrew**: The formula in `Goldziher/homebrew-tap` is rendered from the release checksums.
    - **PyPI Publishing**: The package is built and published to PyPI. Its version (`release/pypi/**/__init__.py`) must already equal the tag; run `scripts/release-bump.sh set <version>` before tagging.
    - **npm Publishing**: The `release/npm/package.json` version must already equal the tag (same bump script); the package is published to npm.

!!! danger "Do Not Release Manually"
Manual releases are strongly discouraged. The CI pipeline is the single source of truth for versioning. Releasing locally will result in version mismatches (as the `__version__` string will not be updated) and should be avoided.

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
# 1. Build binaries natively on each target platform (cgo + the liter-llm static library; no cross-compiling).
#    Supported: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64.
eval "$(bash scripts/setup-liter-llm.sh)"   # exports CGO_ENABLED=1 and CGO_LDFLAGS (verified, static)
go build -o release/npm/bin/ai-rulez-darwin-arm64 ./cmd/ai-rulez   # on a darwin/arm64 host
# ... repeat on a host of each other platform (the Windows binary is named ai-rulez-windows-amd64.exe)

# 2. (Optional) Modify package.json for your scope/version
cd release/npm
# Edit name to "@yourscope/ai-rulez", adjust version if needed

# 3. Publish to your private registry
npm publish --registry=https://your-private-registry/

# 4. Install and use
npm install @yourscope/ai-rulez --registry=https://your-private-registry/
ai-rulez generate  # Uses local binary, no GitHub access needed
```

### Binary Naming Convention

Binaries must follow this naming pattern: `ai-rulez-{os}-{arch}[.exe]`

- **os**: `linux`, `darwin`, `windows`
- **arch**: `amd64`, `arm64`, `386`
- **.exe**: Required for Windows only
