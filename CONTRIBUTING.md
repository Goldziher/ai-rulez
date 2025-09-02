# Contributing to ai-rulez

Thank you for your interest in contributing to ai-rulez! This guide will help you get started with development.

## Prerequisites

- **Go 1.24+** - Required for building the binary
- **Node.js 20+** - Required for npm package and commit hooks  
- **Python 3.8+** - Required for PyPI package
- **[Task](https://taskfile.dev)** - Required for running build tasks
- **[golangci-lint](https://golangci-lint.run/)** - Required for linting
- **[lefthook](https://github.com/evilmartians/lefthook)** - Required for git hooks
- **pnpm** - Required for Node.js package management

## Development Setup

### 1. Clone the Repository

```bash
git clone https://github.com/Goldziher/ai-rulez.git
cd ai-rulez
```

### 2. Run Setup Task

The easiest way to set up the development environment:

```bash
task setup
```

This will:
- Install Go dependencies (`go mod tidy`)
- Install lefthook for git hooks
- Install Node.js dependencies with pnpm
- Set up pre-commit hooks
- Install golangci-lint

### 3. Manual Setup (Alternative)

If you prefer to set up manually:

```bash
# Install Go dependencies
go mod tidy

# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install lefthook
go install github.com/evilmartians/lefthook@latest
lefthook install

# Install Node.js dependencies
pnpm install

# The schema is now embedded directly from schema/ directory - no sync needed
```

## Build System (Taskfile)

The project uses [Task](https://taskfile.dev) as its build system. The Taskfile provides consistent commands for building, testing, and maintaining the project.

### Core Development Tasks

```bash
# Build the binary
task build

# Install the binary to $GOPATH/bin
task install

# Run unit tests
task test

# Run tests with coverage
task test:coverage

# Run tests with race detection
task test:race

# Run all CI checks (lint, format, tests)
task ci
```

### Testing Tasks

```bash
# Unit tests only
task test

# Integration tests
task test:integration

# End-to-end tests (all suites)
task test:e2e

# Specific e2e test suites
task test:e2e:init      # Init provider tests
task test:e2e:cli       # CLI integration tests
task test:e2e:agents    # Agent tests
task test:e2e:profile   # Profile tests

# Benchmarks
task test:benchmark

# All tests (unit + integration)
task test:all
```

### Code Quality Tasks

```bash
# Run linting
task lint

# Fix linting issues automatically
task lint:fix

# Format code
task fmt

# Tidy Go modules
task tidy

# Full development cycle (tidy, format, lint, test)
task dev
```

### Utility Tasks

```bash
# Clean build artifacts
task clean

# Build for multiple platforms
task build:all

# Update dependencies
task deps
```

## Project Structure

```
ai-rulez/
├── cmd/                    # CLI entry point and commands
│   ├── main.go            # Main entry point
│   ├── agent.go           # Agent CRUD commands
│   ├── agent_test.go      # Agent command tests
│   ├── mcp.go             # MCP server implementation
│   └── mcp_handlers.go    # MCP tool handlers
├── internal/              # Internal packages
│   ├── config/           # Configuration loading and validation
│   │   ├── loader.go     # Config file loading
│   │   ├── crud.go       # CRUD operations
│   │   ├── remote.go     # Remote includes support
│   │   └── validate.go   # Schema validation
│   ├── generator/        # Output generation
│   │   ├── generator.go  # Main generation logic
│   │   └── templates.go  # Template handling
│   ├── mcp/             # Model Context Protocol
│   │   └── tools.go     # MCP tool definitions
│   └── templates/       # Built-in templates
├── schema/               # JSON Schema definitions
│   └── ai-rules-v2.schema.json
├── build/                # Package build files
│   ├── npm/             # npm package files
│   │   ├── bin/         # Wrapper script
│   │   ├── package.json
│   │   └── install.js
│   └── python/          # Python package files
│       ├── setup.py
│       └── ai_rulez/
├── e2e/                  # End-to-end tests
│   └── suites/          # Test suites
├── test/                 # Test data
│   └── integration/     # Integration test files
├── testing/             # Test scenarios and fixtures
├── examples/            # Example configurations
├── .github/             # GitHub Actions workflows
│   └── workflows/
│       ├── ci.yaml     # CI pipeline
│       └── release.yaml # Release automation
├── .goreleaser.yaml     # GoReleaser configuration
├── taskfile.yaml        # Task definitions
├── lefthook.yml         # Git hooks configuration
└── .pre-commit-hooks.yaml # Pre-commit hook definitions
```

## Testing

### Running Tests

```bash
# Unit tests only
task test

# Integration tests
task test:integration

# End-to-end tests
task test:e2e

# All tests with coverage
task test:all
task coverage

# Test with race detection
task test:race
```

### AI Agent Integration Tests

Some integration tests require AI agents to be installed locally. These tests are gated by an environment variable to prevent CI failures:

```bash
# Run agent integration tests (requires AI tools like claude, gemini, etc.)
AI_RULEZ_INTEGRATION_TEST=1 go test ./e2e -run TestAgentIntegration

# Or use the task command with the environment variable
AI_RULEZ_INTEGRATION_TEST=1 task test:e2e
```

The agent integration tests will:
- Detect available AI CLI tools (claude, gemini, amp, codex, cursor-agent)
- Test preset configurations for each tool
- Test agent-assisted configuration generation
- Validate generated configurations

### Writing Tests

- Place unit tests next to the code they test (e.g., `config_test.go`)
- Use table-driven tests for multiple test cases
- Use `testify/assert` for assertions
- Aim for >80% code coverage
- Gate integration tests with environment variables when they require external tools
- Test error conditions and edge cases

## Code Style

### Go Code

- Follow [Effective Go](https://golang.org/doc/effective_go.html) guidelines
- Use `gofmt` for formatting (automatic via `task fmt`)
- Use `golangci-lint` for linting (automatic via `task lint`)
- Keep functions small and focused
- Handle errors explicitly
- Use meaningful variable and function names

### Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```bash
# Features
git commit -m "feat: add support for remote includes"
git commit -m "feat(mcp): add new tool for rule validation"

# Bug fixes
git commit -m "fix: correct priority sorting in rules"
git commit -m "fix(npm): resolve npx execution issue"

# Documentation
git commit -m "docs: update README with examples"
git commit -m "docs(api): add MCP tool documentation"

# Tests
git commit -m "test: add coverage for agent CRUD operations"
git commit -m "test(e2e): add remote includes test suite"

# Chores
git commit -m "chore: update dependencies"
git commit -m "chore(ci): optimize GitHub Actions workflow"
```

## Pull Request Process

1. **Fork the repository** and create a feature branch
2. **Make your changes** following the code style guidelines
3. **Add or update tests** for your changes
4. **Run the full test suite** with `task ci`
5. **Commit with conventional commit messages**
6. **Push your branch** and create a pull request
7. **Ensure CI passes** and address review feedback

### PR Title Format

Use the same conventional commit format for PR titles:
- `feat: add new feature`
- `fix: resolve issue`
- `docs: update documentation`

## Release Process

Releases are automated via GitHub Actions when a new tag is pushed:

```bash
# Create a new release
git tag v1.5.2
git push origin v1.5.2
```

The release workflow will:
1. Run all tests
2. Build binaries with GoReleaser
3. Publish to npm registry
4. Publish to PyPI
5. Update Homebrew tap
6. Create GitHub release with changelog

## Examples

For usage examples and configuration samples, see the [examples directory](examples/).

## Getting Help

- **Issues**: [GitHub Issues](https://github.com/Goldziher/ai-rulez/issues)
- **Discussions**: [GitHub Discussions](https://github.com/Goldziher/ai-rulez/discussions)
- **Documentation**: [README.md](README.md)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.