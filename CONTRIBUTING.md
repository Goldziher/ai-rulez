# Contributing to airules

## Prerequisites

- Go 1.24.4+
- Node.js 18+ (for commit hooks)
- [Task](https://taskfile.dev) (optional, for running tasks)

## Setup

```bash
# Clone the repository
git clone https://github.com/Goldziher/ai-rulez.git
cd ai-rulez

# Run setup (installs dependencies and git hooks)
task setup

# Or manually:
go mod tidy
go install github.com/evilmartians/lefthook@latest
lefthook install
pnpm install
```

## Development

```bash
# Run tests
task test
# or: go test -v ./...

# Run linters
task lint
# or: golangci-lint run

# Build
task build
# or: go build -o airules .
```

## Commit Guidelines

We use [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `test:` Test changes
- `chore:` Maintenance tasks

Examples:
```bash
git commit -m "feat: add support for sections in rules"
git commit -m "fix: correct priority sorting order"
git commit -m "docs: update README with examples"
```

## Pull Request Process

1. Fork and create a feature branch
2. Make your changes
3. Ensure tests pass and linting is clean
4. Commit with a conventional commit message
5. Push and create a pull request

## Project Structure

```
airules/
├── cmd/airules/    # CLI commands
├── internal/        # Internal packages
│   ├── config/      # Configuration handling
│   ├── generator/   # Output file generation
│   └── templates/   # Template rendering
├── schema/          # JSON Schema definitions
└── testing/         # Test data and scenarios
```