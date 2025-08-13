# ai-rulez ⚡

Lightning-fast CLI tool and MCP server for managing AI assistant rules across Claude, Cursor, Windsurf, and other tools from a single YAML configuration.

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8)](https://go.dev)
[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-orange)](https://github.com/Goldziher/homebrew-tap)

## Quick Start

```bash
# Install (choose your preferred method)
brew install goldziher/tap/ai-rulez     # macOS/Linux via Homebrew
npm install -g ai-rulez                 # Node.js/npm
pip install ai-rulez                    # Python/pip
go install github.com/Goldziher/ai-rulez@latest  # Go

# Initialize your project
ai-rulez init my-project

# Edit ai-rulez.yaml to add your rules

# Generate AI assistant files
ai-rulez generate
```

## Features

- **Single Source of Truth** - One YAML file manages all AI assistant configurations
- **Lightning Fast** - Written in Go, with incremental generation (only writes changes)  
- **Multi-Platform** - Supports Claude, Cursor, Windsurf, and custom outputs
- **MCP Server** - Built-in Model Context Protocol server for dynamic AI integration
- **Complete CRUD Operations** - Add, update, delete rules, sections, agents, and outputs via CLI
- **Remote Includes** - Share rules via URLs with caching and SSRF protection
- **AI Agents** - Define specialized sub-agents with tools and system prompts
- **Rich Templates** - Built-in and custom templates with full Go template syntax
- **Enterprise Ready** - Security features, validation, and comprehensive testing
- **Git Integration** - Pre-commit hooks and automated configuration management

## Installation

### macOS/Linux (Homebrew)
```bash
brew install goldziher/tap/ai-rulez
```

### Node.js (npm/yarn/pnpm)
```bash
npm install -g ai-rulez
yarn global add ai-rulez
pnpm add -g ai-rulez
```

### Python (pip)
```bash
pip install ai-rulez
pipx install ai-rulez  # Recommended for global install
```

### Go
```bash
go install github.com/Goldziher/ai-rulez@latest
```

### From Source
```bash
git clone https://github.com/Goldziher/ai-rulez.git
cd ai-rulez
go build -o ai-rulez ./cmd
```

## Complete Configuration Example

Here's a comprehensive example showing all major features with explanatory comments:

```yaml
# ai-rulez.yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json

metadata:
  name: "My Project"
  version: "1.0.0"
  description: "AI assistant configuration for my project"

# Include shared configurations (remote or local)
includes:
  # Organization standards from GitHub
  - "https://raw.githubusercontent.com/myorg/standards/main/backend.yaml"
  # Team-specific rules
  - "./team-rules.yaml"
  # Personal preferences (not committed to git)
  - "./ai-rulez.local.yaml"

# Output configurations - where to generate files
outputs:
  # Claude - single markdown file
  - path: "CLAUDE.md"
    template: "default"  # Built-in template
  
  # Cursor - new directory format with .mdc files
  - path: ".cursor/rules/"
    type: "rule"
    naming_scheme: "rules.mdc"  # All rules in one file named rules.mdc
  
  # Windsurf
  - path: ".windsurfrules"
  
  # Generate individual agent files for Claude
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{name}.md"  # One file per agent
  
  # Custom output with inline template
  - file: "CONTRIBUTING.md"
    template: |
      # Contributing to {{.ProjectName}}
      
      ## Priority Guidelines (8+ priority)
      {{range .Rules}}{{if ge .Priority 8}}
      ### {{.Name}}
      {{.Content}}
      {{end}}{{end}}

# Define specialized AI agents (sub-agents for Claude)
agents:
  - name: "code-reviewer"
    id: "reviewer-001"  # Optional: for precise matching in overrides
    description: "Reviews code for quality and security"
    priority: 10  # Higher priority agents appear first
    tools:  # Tools the agent can use
      - "read_file"
      - "search"
      - "grep"
    system_prompt: |
      You are a senior code reviewer focusing on:
      - Code quality and maintainability
      - Security vulnerabilities
      - Performance implications
      - Best practices
      
      Always provide constructive feedback with specific suggestions.

  - name: "test-writer"
    id: "tester-001"
    description: "Writes comprehensive tests"
    priority: 8
    tools:
      - "read_file"
      - "write_file"
      - "run_command"
    system_prompt: |
      You are a test automation expert. Write tests that:
      - Cover edge cases
      - Are readable and maintainable
      - Follow testing best practices
      - Achieve high code coverage

# Rules - your coding standards and guidelines
rules:
  - name: "Code Quality"
    id: "quality-001"  # Optional: for precise matching
    priority: 10  # 1-10, higher = more important
    content: |
      - Write clean, readable, and maintainable code
      - Follow SOLID principles
      - Keep functions small and focused
      - Use meaningful variable names
  
  - name: "Testing Standards"
    id: "testing-001"
    priority: 9
    content: |
      - Write unit tests for all new features
      - Maintain minimum 80% code coverage
      - Use TDD when appropriate
      - Test edge cases and error conditions
  
  - name: "Documentation"
    priority: 7
    content: |
      - Document all public APIs
      - Include usage examples
      - Keep README up to date
      - Use clear commit messages

# Sections - informational content that appears before rules
sections:
  - title: "Project Overview"
    priority: 100  # Sections typically have higher priority to appear first
    content: |
      ## Architecture
      
      This project follows a microservices architecture with:
      - Frontend: React + TypeScript
      - Backend: Node.js + Express
      - Database: PostgreSQL
      
      ## Development Workflow
      
      1. Create feature branch from main
      2. Write tests first (TDD)
      3. Implement feature
      4. Run linters and tests
      5. Create PR for review

  - title: "Agent Usage Instructions"
    priority: 95
    content: |
      ## Using AI Agents
      
      This project includes specialized agents in `.claude/agents/`:
      - **code-reviewer**: For code reviews and quality checks
      - **test-writer**: For generating test cases
      
      Use these agents by referencing their specific prompts for domain tasks.

# User-specific overrides (highest priority)
userRulez:
  - name: "Personal Preferences"
    content: |
      - Use 2 spaces for indentation (not 4)
      - Prefer const over let
      - Always use semicolons in JavaScript
  
  # Override an existing rule by ID
  - id: "testing-001"
    content: |
      - Write unit tests for all new features
      - Maintain minimum 90% code coverage (personal goal)
      - Use TDD always (not just when appropriate)
      - Test edge cases and error conditions
```

## Core Commands

### Initialize and Generate
```bash
ai-rulez init                    # Interactive setup
ai-rulez init my-project         # With project name
ai-rulez init --setup-hooks      # Auto-configure git hooks

ai-rulez generate                # Generate all outputs
ai-rulez generate --dry-run      # Preview changes
ai-rulez generate --update-gitignore  # Update .gitignore
ai-rulez generate -r              # Recursive (all subdirs)
```

### Validate
```bash
ai-rulez validate                # Validate current config
ai-rulez validate config.yaml    # Validate specific file
ai-rulez validate -r              # Validate recursively
```

### CRUD Operations
```bash
# Rules and sections
ai-rulez add rule "API Security" --id "sec-001"
ai-rulez add section "Overview"
ai-rulez update rule "API Security"
ai-rulez delete rule "API Security"

# Agents with full CRUD support
ai-rulez add agent code-reviewer --description "Reviews code quality" \
  --priority 8 --tools "Read,Grep,Edit" --system-prompt "You are a code reviewer..."
ai-rulez update agent code-reviewer --priority 9
ai-rulez delete agent code-reviewer

# Outputs  
ai-rulez add output "docs/rules.md"
ai-rulez update output "docs/rules.md"
ai-rulez delete output "docs/rules.md"
```

## Advanced Features

### Remote Includes

Share configurations across projects and teams:

```yaml
includes:
  # Public GitHub configurations
  - "https://raw.githubusercontent.com/myorg/standards/main/backend.yaml"
  
  # Private API endpoints
  - "https://api.company.com/ai-rules/security.yaml"
  
  # Local overrides
  - "./team-rules.yaml"
  - "./ai-rulez.local.yaml"  # Personal preferences (gitignored)
```

**Security Features:**
- SSRF protection (blocks private IPs, metadata endpoints)
- Content validation and size limits
- 3-level caching (memory: 5min, disk: 24h, HTTP: ETags)

### Directory Outputs with Naming Schemes

Generate multiple files with configurable naming:

```yaml
outputs:
  # Agents with custom naming
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{priority:02d}-{name}.md"
  
  # Rules organized by priority
  - path: "docs/rules/"
    type: "rule"
    naming_scheme: "rule-{index:03d}-{name}.md"
```

**Naming Placeholders:**
- `{name}` - Item name (sanitized)
- `{type}` - Item type (rule/agent/section)
- `{priority}` - Priority value
- `{index}` - Sequential number
- Format specifiers: `{index:03d}`, `{priority:02d}`

### Templates

Use custom templates with full Go template syntax:

```yaml
outputs:
  # External template file
  - file: "docs/handbook.md"
    template: "@templates/handbook.tmpl"
```

**Available Variables:**
- `{{.ProjectName}}`, `{{.Version}}`, `{{.Description}}`
- `{{.Rules}}`, `{{.Sections}}`, `{{.Agents}}`
- `{{.Timestamp}}`, `{{.RuleCount}}`, `{{.SectionCount}}`
- `{{.AllContent}}` - Combined rules and sections

## MCP Server Integration

ai-rulez includes a built-in Model Context Protocol server for AI assistants.

### Claude Desktop Configuration
Add to Claude's configuration file:

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "npx",
      "args": ["ai-rulez@latest", "mcp"]
    }
  }
}
```

### Starting the Server
```bash
ai-rulez mcp         # Standalone
npx ai-rulez mcp     # Via npx
```

## Git Integration

### Pre-commit Hooks

Using pre-commit framework (uses Python package, no Go required):
```yaml
# .pre-commit-config.yaml
repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v1.5.0
    hooks:
      - id: ai-rulez-validate    # Validate configuration
      - id: ai-rulez-generate    # Auto-generate files
```

Using Lefthook:
```yaml
# lefthook.yml
pre-commit:
  commands:
    ai-rulez:
      run: ai-rulez generate --dry-run
```

Automatic Setup:
```bash
ai-rulez init --setup-hooks
```

## Configuration Schema

Full JSON Schema available at [`schema/ai-rules-v1.schema.json`](schema/ai-rules-v1.schema.json)

### Validation
```bash
ai-rulez validate        # Validate against schema
ai-rulez validate -v     # Show detailed errors
```

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Setup
```bash
# Initialize project with git hooks
ai-rulez init dev --setup-hooks

# Or manually setup
git clone https://github.com/Goldziher/ai-rulez.git
cd ai-rulez
task setup      # Install dependencies
task test       # Run tests
task build      # Build binary
```

### Running Tests
```bash
task test              # Unit tests
task test:integration  # Integration tests
task test:all         # All tests
task coverage         # Coverage report
```