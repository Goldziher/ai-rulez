# ai-rulez ⚡

> **Lightning-fast CLI tool (written in Go) for managing AI assistant rules**

A high-performance CLI tool for generating configuration files for Claude, Cursor, Windsurf, and other AI assistants from a single, centralized YAML configuration.

**Version 1.1.0** - Now with comprehensive installation testing and improved CI/CD!

## 🚀 Features

- ⚡ **Blazing Fast**: Written in Go for maximum performance and cross-platform compatibility
- 🔧 **Multi-Assistant Support**: Generate configs for Claude (CLAUDE.md), Cursor (.cursorrules), Windsurf (.windsurfrules), and more
- 📝 **Single Source of Truth**: Maintain all your AI rules in one YAML configuration
- 🎯 **Smart Templates**: Built-in and custom templates with full Go template syntax
- 🔍 **Validation**: Comprehensive configuration validation with JSON Schema
- 📦 **Modular Rules**: Include system for rule composition with circular dependency detection  
- 📚 **Sections Support**: Mix informative content (docs, guidelines) with rules
- 🔄 **Git Integration**: Perfect for pre-commit hooks and CI/CD workflows
- ⚡ **Incremental Generation**: Only writes files when content changes (performance optimized)
- 🎨 **Smart Sorting**: Dual sorting by priority and name for consistent output

## 📦 Installation

### pip (Recommended for Python users)
```bash
pip install ai-rulez
```
*Automatically downloads and manages the Go binary for your platform*  
**Requirements**: Python 3.9+ (LTS and above)

### npm (Recommended for Node.js users)
```bash
# Global installation
npm install -g ai-rulez

# Local project installation  
npm install --save-dev ai-rulez
```
*Automatically downloads and manages the Go binary for your platform*  
**Requirements**: Node.js 20+ (LTS and above)

### Go (Direct installation)
```bash
go install github.com/Goldziher/ai-rulez@latest
```

### Homebrew (Coming Soon)
```bash
brew install goldziher/tap/ai-rulez
```

### Direct Download
Download pre-built binaries from [GitHub Releases](https://github.com/Goldziher/ai-rulez/releases) for:
- macOS (Intel and Apple Silicon)
- Linux (x64, ARM64, x86)  
- Windows (x64, x86)

## 🎯 Quick Start

1. **Create a configuration file** (`ai-rulez.yaml`):

```yaml
metadata:
  name: "My AI Rules" 
  version: "1.0.0"

rules:
  - name: "Code Style"
    priority: 10
    content: |
      - Use TypeScript strict mode
      - Prefer functional components
      - Use meaningful variable names

  - name: "Testing"
    priority: 5
    content: |
      - Write unit tests for all functions
      - Use describe/it pattern  
      - Aim for 80% code coverage

outputs:
  - file: "CLAUDE.md"
    template: "claude"
  - file: ".cursorrules"
    template: "cursor"
  - file: ".windsurfrules"
    template: "windsurf"
```

2. **Generate configuration files**:

```bash
ai-rulez generate
```

This creates `CLAUDE.md`, `.cursorrules`, and `.windsurfrules` with your rules properly formatted for each AI assistant.

### Alternative: Initialize from template

```bash
# Initialize with basic template
ai-rulez init "My Project"

# With specific templates
ai-rulez init --template react "My React App"
ai-rulez init --template typescript "My TS Project"
```

## Configuration Format

### Basic Example
```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json

metadata:
  name: "My Project"
  version: "1.0.0"
  description: "Project coding standards"

outputs:
  - file: "claude.md"
  - file: ".cursorrules"
  - file: ".windsurfrules"

rules:
  - name: "Code Quality"
    priority: 10  # Higher number = higher priority
    content: |
      - Write clean, maintainable code
      - Follow SOLID principles
      - Add meaningful comments

  - name: "Testing"
    priority: 5
    content: "Write unit tests for all new features"
```

### With Sections and Templates
```yaml
metadata:
  name: "Advanced Project"

sections:
  - title: "Introduction"
    priority: 100  # Appears first
    content: |
      # Project Guidelines
      
      Welcome! This document outlines our coding standards.

outputs:
  - file: "GUIDELINES.md"
    template: |
      # {{.ProjectName}} Guidelines
      
      {{range .AllContent}}
      {{if .IsRule}}## {{.Title}} (Priority: {{.Priority}})
      {{.Content}}
      {{else}}{{.Content}}{{end}}
      {{end}}
  
  - file: "rules/detailed.md"
    template: "@templates/custom.tmpl"  # File reference

rules:
  - name: "API Design"
    priority: 10
    content: "Follow RESTful conventions"

sections:
  - title: "Contributing"
    priority: 1  # Appears last
    content: |
      ## How to Contribute
      
      Please read our contribution guidelines...
```

### Configuration Schema

- **metadata**: Project information
  - `name` (required): Project name
  - `version`: Semantic version
  - `description`: Project description

- **outputs** (required): Output file definitions
  - `file`: Output file path (relative to config)
  - `template`: Template to use (optional)
    - Built-in: `"default"`, `"documentation"`
    - File reference: `"@path/to/template.tmpl"`
    - Inline: Multi-line template string

- **rules**: Coding rules and guidelines
  - `name` (required): Rule identifier
  - `priority`: Integer ≥ 1 (default: 1)
  - `content` (required): Rule description

- **sections**: Informative text blocks
  - `title` (required): Section identifier
  - `priority`: Integer ≥ 1 (default: 1)
  - `content` (required): Markdown content (rendered as-is)

- **includes**: External rule files to include
  - Paths relative to config file
  - Supports nested includes
  - Circular dependencies detected

## Sorting and Output Order

All content (rules and sections) uses dual sorting:
1. **Primary**: By priority (descending) - higher numbers first
2. **Secondary**: By title/name (ascending) - alphabetical order

This ensures consistent, predictable output across regenerations.

## 🛠️ Commands

```bash
# Generate all configuration files
ai-rulez generate

# Validate configuration
ai-rulez validate

# Generate recursively in subdirectories  
ai-rulez generate --recursive

# Preview output without writing files
ai-rulez generate --dry-run

# Initialize new project
ai-rulez init "My Project"

# Show help
ai-rulez --help
```

## 🎨 Template Variables

| Variable | Type | Description |
|----------|------|-------------|
| `{{.ProjectName}}` | string | Project name |
| `{{.Version}}` | string | Version string |
| `{{.Description}}` | string | Project description |
| `{{.Rules}}` | []Rule | Rules array (sorted) |
| `{{.Sections}}` | []Section | Sections array (sorted) |
| `{{.AllContent}}` | []ContentItem | Combined rules + sections (sorted) |
| `{{.Timestamp}}` | time.Time | Generation timestamp |
| `{{.RuleCount}}` | int | Number of rules |
| `{{.SectionCount}}` | int | Number of sections |

## 📚 Command Reference

### `ai-rulez init [project-name]`
Initialize a new configuration file.

**Options:**
- `--template, -t`: Template to use (`basic`, `react`, `typescript`)

### `ai-rulez generate [config-file]`
Generate output files from configuration. Files are only written if content changes.

**Config File Discovery:**
- Without arguments: Searches for `.ai-rulez.yaml` or `ai-rulez.yaml` starting from current directory, traversing upward to find the first config file
- With `--recursive` flag: Finds and processes all config files in the current directory tree
- With explicit path: Uses the specified config file

**Options:**
- `--recursive, -r`: Recursively find and process all ai-rulez configuration files
- `--dry-run`: Validate configuration and show what would be generated without writing files

### `ai-rulez validate [config-file]`
Validate configuration file against schema.

## Editor Support

Add the schema reference to your YAML files for:
- Auto-completion
- Inline documentation
- Real-time validation

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json
```

## Development

### Prerequisites
- Go 1.22+
- Task (taskfile.dev)
- golangci-lint v2
- lefthook (for git hooks)

### Setup
```bash
go mod download
task install-tools
lefthook install
```

### Common Tasks
```bash
task test      # Run tests
task lint      # Run linting
task fmt       # Format code
task build     # Build binary
```

### Project Structure
```
ai-rulez/
├── cmd/ai-rulez/     # CLI commands
├── internal/         # Internal packages
│   ├── config/       # Configuration and validation
│   ├── generator/    # Output generation
│   └── templates/    # Template rendering
├── schema/           # JSON Schema definitions
├── examples/         # Example configurations
└── testing/          # Test scenarios and data
```

## Pre-commit Hooks

ai-rulez can be integrated with git pre-commit hooks to automatically validate or generate files when committing changes.

### Using pre-commit

Add to your `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v1.0.0  # Use the latest version
    hooks:
      # Validate configuration only (recommended for most projects)
      - id: ai-rulez-validate
      
      # Or generate files automatically on commit
      - id: ai-rulez-generate
      
      # Or process all config files recursively
      - id: ai-rulez-recursive
```

**Hook Options:**
- `ai-rulez-validate`: Validates configuration files using `--dry-run` mode
- `ai-rulez-generate`: Generates output files from configuration
- `ai-rulez-recursive`: Processes all ai-rulez config files in the repository

### Using lefthook

Add to your `lefthook.yml`:

```yaml
pre-commit:
  commands:
    ai-rulez:
      glob: "{.ai-rulez.yaml,ai-rulez.yaml}"
      run: ai-rulez generate --dry-run
      
    # Or to auto-generate files:
    # ai-rulez:
    #   glob: "{.ai-rulez.yaml,ai-rulez.yaml}"
    #   run: ai-rulez generate && git add .
```

### Manual Setup

For other git hook managers or manual setup:

```bash
# Validate only (recommended)
ai-rulez generate --dry-run

# Generate and stage files
ai-rulez generate && git add .

# Process all configs recursively
ai-rulez generate --recursive
```

**Performance Notes:**
- Use `--dry-run` for validation-only mode (fastest)
- The tool uses incremental generation (only writes when content changes)
- Consider using file glob patterns to only run when config files change

## 📋 Changelog

### v1.1.0 (2025-06-30)
- ✅ Added comprehensive integration tests for Python and JavaScript installation scripts
- ✅ Implemented multi-platform CI test matrix (Windows, macOS, Linux)
- ✅ Fixed Windows-specific installation issues
- ✅ Added support for Python 3.9+ (LTS) and Node.js 20+ (LTS)
- ✅ Added ruff and biome for Python/JavaScript linting
- ✅ Improved error handling and cross-platform compatibility
- ✅ Updated documentation with LTS version requirements
- 🔧 Fixed golangci-lint configuration issues
- 🔧 Enhanced lefthook pre-commit hooks

### v1.0.0 (2025-06-26)
- 🎉 Initial release
- ⚡ Lightning-fast Go implementation
- 📦 Python and npm package wrappers
- 🎯 Support for Claude, Cursor, and Windsurf
- 📝 YAML-based configuration
- 🔍 JSON Schema validation
- 📚 Modular rules with includes
- 🔄 Incremental file generation

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## 📄 License

MIT License - see [LICENSE](LICENSE)

## 🔗 Links

- **GitHub Repository**: [https://github.com/Goldziher/ai-rulez](https://github.com/Goldziher/ai-rulez)
- **Documentation**: [README](https://github.com/Goldziher/ai-rulez#readme)
- **Issues**: [Bug Reports & Feature Requests](https://github.com/Goldziher/ai-rulez/issues)
- **Releases**: [GitHub Releases](https://github.com/Goldziher/ai-rulez/releases)
- **PyPI Package**: [https://pypi.org/project/ai-rulez/](https://pypi.org/project/ai-rulez/)
- **npm Package**: [https://www.npmjs.com/package/ai-rulez](https://www.npmjs.com/package/ai-rulez)
- **JSON Schema**: [ai-rules-v1.schema.json](https://github.com/Goldziher/ai-rulez/blob/main/schema/ai-rules-v1.schema.json)

---

**Performance Note**: The Python and npm packages are lightweight wrappers around the Go binary. The actual tool is written in Go for maximum performance, fast startup times, and efficient cross-platform binary distribution.