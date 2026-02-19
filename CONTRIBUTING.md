# Contributing to ai-rulez

Thank you for your interest in contributing! This guide provides everything you need to get started with development.

## Getting Started

### Prerequisites

- **Go 1.26+**
- **Node.js 20+** (for commit hooks)
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

1.  **Update the Schema:** Add the `new_entity` definition to `schema/ai-rules-v2.schema.json`.
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
# Build the binary to ./bin/ai-rulez
task build

# Run all unit tests
task test

# Run all end-to-end tests
task test:e2e

# Run all checks (lint, format, test) before committing
task ci
```

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
4.  Ensure all checks pass by running `task ci`.
5.  Push your branch and open a pull request with a title that follows the Conventional Commit format.

---

## Releasing

Releases are fully automated using GitHub Actions and are triggered when a new tag is pushed to the `main` branch.

### How It Works

1.  **Tag Push**: To create a new release, push a tag to `main` with the format `vX.Y.Z` (e.g., `v2.0.1`).
    ```bash
    git tag v2.0.1
    git push origin v2.0.1
    ```
2.  **CI/CD Pipeline**: The push event triggers the `.github/workflows/publish.yaml` workflow, which handles the entire release process:
    - **GoReleaser**: Builds binaries for all supported platforms and creates a GitHub Release.
    - **PyPI Publishing**: The Python package version in `release/pypi/ai_rulez/__init__.py` is automatically updated with the tag version, and the package is built and published to PyPI.
    - **npm Publishing**: The `package.json` version is updated, and the package is published to npm.

!!! danger "Do Not Release Manually"
    Manual releases are strongly discouraged. The CI pipeline is the single source of truth for versioning. Releasing locally will result in version mismatches (as the `__version__` string will not be updated) and should be avoided.

---

## Private Registry Distribution

Community contributors can build and publish their own npm packages to private registries for internal distribution or testing before PR merge.

### How It Works

The npm package supports an **offline mode**: if platform-specific binaries are present in the `bin/` directory, they will be used directly instead of downloading from GitHub.

| Scenario | `bin/` directory | Behavior |
|----------|------------------|----------|
| Public npm | Empty | Downloads from GitHub Releases |
| Private registry | Contains binaries | Uses local binaries, no network call |

### Build and Publish

```bash
# 1. Build binaries for target platforms (refer to .goreleaser.yaml for all platforms)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o release/npm/bin/ai-rulez-linux-amd64 ./cmd
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o release/npm/bin/ai-rulez-darwin-arm64 ./cmd
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o release/npm/bin/ai-rulez-windows-amd64.exe ./cmd
# ... add more platforms as needed

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
