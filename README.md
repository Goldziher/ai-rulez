# ai-rulez ⚡

Lightning-fast CLI tool and MCP server for managing AI assistant rules across Claude, Cursor, Windsurf, and other tools from a single YAML configuration.

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8)](https://go.dev)
[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-orange)](https://github.com/Goldziher/homebrew-tap)

## Quick Start

You can either run ai-rulez directly or install it as a global tool:

### Run Directly (No Installation)

#### Using Go
```bash
go run github.com/Goldziher/ai-rulez/cmd@latest --help
```

#### Using Python
```bash
uvx ai-rulez --help
```

#### Using Node
```bash
npx ai-rulez@latest --help
```

### Install Globally

Install ai-rulez as a global tool:

#### Homebrew (macOS/Linux)
```bash
brew install goldziher/tap/ai-rulez
```

#### Go
```bash
go install github.com/Goldziher/ai-rulez/cmd@latest
```

#### Python (pip)
```bash
pip install --user ai-rulez
```

#### Node.js (npm)
```bash
npm install -g ai-rulez
```

### Basic Usage

#### Initialize a project

Begin by initializing a project configuration file (`ai-rulez.yaml`) in the current directory:

```bash
ai-rulez init
```

Optionally use arguments to configure the schema:

##### Initialize with project name and common providers
```bash
ai-rulez init "My Project" --claude --cursor --windsurf
```

##### Initialize with all providers, sub-agents support and sections
```bash
ai-rulez init "My Project" --all --with-agents --with-sections
```

#### Generate AI assistant files
```bash
ai-rulez generate
```

#### Validate configuration
```bash
ai-rulez validate
```

### MCP Server Configuration

`ai-rulez` has a built-in MCP server that allows AI assistants to dynamically manage configuration. 

Some examples configurations:

#### Claude Desktop Compatible

```json
{
  "mcpServers": {
    "browser-mcp": {
      "command": "npx",
      "args": ["-y", "ai-rulez@latest", "mcp"]
    }
  }
}
```

#### Cursor Compatible

```json
{
  "ai-rulez": {
    "command": "npx",
    "args": ["-y", "ai-rulez@latest", "mcp"]
  }
}
```

#### Claude Code

Run the following command in your project root:

```shell
claude mcp add ai-rulez "npx" "-y" "ai-rulez@latest" "mcp"
```

### Note on MCP and Package Management

All the above examples use `npx` to run ai-rulez for mcp, but you can replace the command to use `uvx`, or `go`, and of course you can also use `pnpm` etc. 

## Configuration File Format

```yaml
# ai-rulez.yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v1.schema.json

metadata:
  name: "My Project"
  version: "1.0.0"
  description: "AI assistant configuration for my project"

# Define reusable target groups (optional)
targets:
  claude-files:
    - "CLAUDE.md"
    - ".claude/**/*.md"
  
  cursor-files:
    - ".cursor/**/*.mdc"
    - ".cursorrules"
  
  docs:
    - "docs/*.md"
    - "*.md"

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
    targets: ["@claude-files"]  # Only for Claude using named target
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
    targets: ["@claude-files"]  # Only for Claude
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
    # No targets = applies to all outputs (default behavior)
    content: |
      - Write clean, readable, and maintainable code
      - Follow SOLID principles
      - Keep functions small and focused
      - Use meaningful variable names
  
  - name: "Claude-Specific Instructions"
    priority: 9
    targets: ["CLAUDE.md"]  # Only for Claude (inline target)
    content: |
      - Use Claude's Task tool for specialized operations
      - Leverage MCP tools when available
      - Follow structured reasoning approach
  
  - name: "Cursor-Specific Instructions"
    priority: 9
    targets: ["@cursor-files"]  # Only for Cursor (named target)
    content: |
      - Use Cursor's AI features for code generation
      - Leverage Cursor's context awareness
      - Follow Cursor's best practices
  
  - name: "Testing Standards"
    id: "testing-001"
    priority: 9
    targets: ["@docs", "*.md"]  # Mix named and inline targets
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

## CLI Reference

### Initialize and Generate

#### Interactive setup
```bash
ai-rulez init
```

#### With project name
```bash
ai-rulez init my-project
```

#### Auto-configure git hooks
```bash
ai-rulez init --setup-hooks
```

#### Generate all outputs
```bash
ai-rulez generate
```

#### Preview changes (dry run)
```bash
ai-rulez generate --dry-run
```

#### Generate and update .gitignore
```bash
ai-rulez generate --update-gitignore
```

#### Recursive generation (all subdirectories)
```bash
ai-rulez generate -r
```

### Validate

#### Validate current config
```bash
ai-rulez validate
```

#### Validate specific file
```bash
ai-rulez validate config.yaml
```

#### Validate recursively
```bash
ai-rulez validate -r
```

### CRUD Operations

#### Rules

##### Add rule
```bash
ai-rulez add rule "API Security" --id "sec-001"
```

##### Update rule
```bash
ai-rulez update rule "API Security"
```

##### Delete rule
```bash
ai-rulez delete rule "API Security"
```

#### Sections

##### Add section
```bash
ai-rulez add section "Overview"
```

#### Agents

##### Add agent with full configuration
```bash
ai-rulez add agent code-reviewer --description "Reviews code quality" --priority 8 --tools "Read,Grep,Edit" --system-prompt "You are a code reviewer..."
```

##### Update agent priority
```bash
ai-rulez update agent code-reviewer --priority 9
```

##### Delete agent
```bash
ai-rulez delete agent code-reviewer
```

#### Outputs

##### Add output
```bash
ai-rulez add output "docs/rules.md"
```

##### Update output
```bash
ai-rulez update output "docs/rules.md"
```

##### Delete output
```bash
ai-rulez delete output "docs/rules.md"
```

## Advanced Features

### Target Filtering

Control which rules, sections, and agents appear in specific outputs using target patterns:

#### Inline Targets
Specify target patterns directly in rules, sections, or agents:

```yaml
rules:
  - name: "Claude-Only Rule"
    targets: ["CLAUDE.md"]  # Only appears in CLAUDE.md
    content: "This rule only applies to Claude"
  
  - name: "Markdown Files"
    targets: ["*.md"]  # Appears in all .md files
    content: "This rule applies to all markdown files"
  
  - name: "Universal Rule"
    # No targets = appears in all outputs (default)
    content: "This rule applies everywhere"
```

#### Named Targets
Define reusable target groups and reference them with `@` prefix:

```yaml
# Define named target groups
targets:
  claude-files:
    - "CLAUDE.md"
    - ".claude/**/*.md"
  
  docs:
    - "docs/*.md"
    - "README.md"

# Reference named targets
rules:
  - name: "Claude-Specific"
    targets: ["@claude-files"]  # Uses named target
    content: "Only for Claude files"
  
  - name: "Mixed Targets"
    targets: ["@docs", "*.txt"]  # Mix named and inline
    content: "Appears in docs and text files"

agents:
  - name: "code-reviewer"
    targets: ["@claude-files"]  # Agent only for Claude
    description: "Reviews code quality"
```

#### Supported Patterns
- **Exact match**: `"CLAUDE.md"` - matches exactly CLAUDE.md
- **Glob patterns**: `"*.md"` - matches all .md files
- **Directory patterns**: `"docs/*"` - matches files in docs/
- **Recursive patterns**: `".claude/**/*.md"` - matches .md files in .claude/ and subdirectories
- **Named references**: `"@target-name"` - references a named target group

When no `targets` are specified, the rule/section/agent appears in all outputs.

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

## Pre-commit Hooks

### Using pre-commit (requires Golang >= 1.18 to be installed):
```yaml
# .pre-commit-config.yaml
repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v1.6.0
    hooks:
      - id: ai-rulez-validate    # Validate configuration
      - id: ai-rulez-generate    # Auto-generate files
```

### Using Lefthook:
```yaml
# lefthook.yml
pre-commit:
  commands:
    ai-rulez:
      run: npx -y ai-rulez@latest generate
```

Note, you can auto configure hooks with the `--setup-hooks` flag when initializing the project, e.g.:

```bash
npx -y ai-rulez init --setup-hooks
```

## Examples

For complete configuration examples and templates, see the [examples directory](examples/).

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details on:
- Development setup with Task
- Running tests and linting
- Project structure
- Commit conventions
- Pull request process