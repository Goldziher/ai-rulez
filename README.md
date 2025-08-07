# ai-rulez ⚡

> **Lightning-fast CLI tool (written in Go) for managing AI assistant rules**

A high-performance CLI tool for generating configuration files for Claude, Cursor, Windsurf, and other AI assistants from a single, centralized YAML configuration.

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
- 🔧 **Local Overrides**: ID-based rule overriding with `.local.yaml` files for personal customization
- 🤖 **AI-First Headers**: Auto-generated headers that clearly instruct AI assistants not to edit files directly
- 🌐 **MCP Server**: Built-in Model Context Protocol server for direct AI assistant integration
- ✏️ **Rule Management**: CLI commands to add, update, and delete rules, sections, and outputs
- 🔀 **Multiple Config Formats**: Supports various naming conventions (.ai-rulez.yaml, ai_rulez.yaml, etc.)
- 🤖 **Agent Support**: Define AI agents with tools, system prompts, and custom configurations
- 📂 **Directory Outputs**: Generate multiple files in directories with customizable naming schemes

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

#### As a Go Tool (Recommended for Go projects)
```bash
# Add to go.mod tools directive
go mod edit -require=github.com/Goldziher/ai-rulez/cmd@latest

# Run directly without installation
go run github.com/Goldziher/ai-rulez/cmd@latest generate --recursive

# Install globally
go install github.com/Goldziher/ai-rulez/cmd@latest
```

#### Traditional Installation
```bash
go install github.com/Goldziher/ai-rulez@latest
```

### Homebrew
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
  - path: "CLAUDE.md"
    template: "claude"
  - path: ".cursorrules"
    template: "cursor"
  - path: ".windsurfrules"
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
  - path: "claude.md"
  - path: ".cursorrules"
  - path: ".windsurfrules"

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
  - path: "GUIDELINES.md"
    template: |
      # {{.ProjectName}} Guidelines
      
      {{range .AllContent}}
      {{if .IsRule}}## {{.Title}} (Priority: {{.Priority}})
      {{.Content}}
      {{else}}{{.Content}}{{end}}
      {{end}}
  
  - path: "rules/detailed.md"
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

### With Agents and Directory Outputs
```yaml
metadata:
  name: "AI-Powered Project"
  version: "2.0.0"

# Generate individual agent files in a directory
outputs:
  - path: ".claude/agents/"  # Directory output (note trailing slash)
    type: "agent"
    naming_scheme: "{name}.md"  # Custom naming pattern
  
  - path: "docs/agents/"
    type: "agent"
    naming_scheme: "agent-{index:03d}-{priority:02d}-{name}.md"
  
  - path: "CLAUDE.md"  # Regular file output
    template: "claude"

# Define AI agents with specific capabilities
agents:
  - name: "code-reviewer"
    description: "Reviews code for quality and best practices"
    priority: 10
    tools:
      - "read_file"
      - "search_code"
      - "analyze_complexity"
    system_prompt: |
      You are a meticulous code reviewer focused on:
      - Code quality and maintainability
      - Performance optimization
      - Security best practices
  
  - name: "test-writer"
    description: "Generates comprehensive test suites"
    priority: 8
    tools:
      - "read_file"
      - "write_file"
      - "run_tests"
    template: |
      # Test Writer Agent
      
      ## Purpose
      {{.Description}}
      
      ## Available Tools
      {{range .Tools}}- {{.}}
      {{end}}

rules:
  - name: "Code Standards"
    priority: 10
    content: "Follow team coding standards"
```

### Configuration Schema

- **metadata**: Project information
  - `name` (required): Project name
  - `version`: Semantic version
  - `description`: Project description

- **outputs** (required): Output file/directory definitions
  - `path` or `file`: Output path (relative to config)
    - Single file: `"CLAUDE.md"`
    - Directory: `"agents/"` (trailing slash)
  - `type`: Output type for directories (`"agent"`, `"rule"`, etc.)
  - `naming_scheme`: Custom naming pattern for directory outputs
    - Variables: `{name}`, `{type}`, `{priority}`, `{index}`
    - Format specifiers: `{index:03d}`, `{priority:02d}`
  - `template`: Template to use (optional)
    - Built-in: `"default"`, `"documentation"`
    - File reference: `"@path/to/template.tmpl"`
    - Inline: Multi-line template string

- **rules**: Coding rules and guidelines
  - `id`: Optional unique identifier for precise overriding
  - `name` (required): Rule identifier
  - `priority`: Integer ≥ 1 (default: 1)
  - `content` (required): Rule description

- **sections**: Informative text blocks
  - `id`: Optional unique identifier for precise overriding
  - `title` (required): Section identifier
  - `priority`: Integer ≥ 1 (default: 1)
  - `content` (required): Markdown content (rendered as-is)

- **agents**: AI agent definitions
  - `id`: Optional unique identifier for precise overriding
  - `name` (required): Agent identifier
  - `description` (required): Agent purpose/role description
  - `priority`: Integer ≥ 1 (default: 1)
  - `tools`: List of tools available to the agent
  - `system_prompt`: Custom system prompt for the agent
  - `template`: Custom template for agent output

- **includes**: External rule files to include
  - Paths relative to config file
  - Supports nested includes
  - Circular dependencies detected

### Local Configuration Overrides

AI Rulez supports local configuration overrides through `.local.yaml` files that allow developers to customize shared configurations without affecting the committed config:

- **Local files**: `{config-name}.local.yaml` (e.g., `ai-rulez.local.yaml`)
  - Automatically loaded if present
  - Highest precedence (overrides main config)
  - Should be added to `.gitignore`
  - Uses ID-based overriding for precise control

- **Rule/Section IDs**: Optional `id` field for rules and sections
  - Enables precise overriding by ID instead of name
  - Backward compatible (name-based merging still works)

#### User Rules vs. Coding Rules

When creating AI rules, it's important to distinguish between two types of instructions:

- **Coding Rules**: Technical guidelines about code quality, architecture, testing, etc.
  - Examples: "Use TypeScript strict mode", "Write unit tests", "Follow REST conventions"
  - Should be in the main configuration file committed to version control

- **User Rules**: Personal preferences about communication style and interaction
  - Examples: "Be concise in responses", "Use casual tone", "Address me as 'Chief'", "Always explain your reasoning"
  - Perfect for `.local.yaml` files as they're personal and shouldn't affect the whole team
  - Allow individual developers to customize AI behavior without impacting others

**Example:**

Main config (`ai-rulez.yaml`):
```yaml
rules:
  - id: "code-style"
    name: "Code Style"
    content: "Use consistent formatting"
  - name: "Testing"
    content: "Write comprehensive tests"
```

Local overrides (`ai-rulez.local.yaml`):
```yaml
rules:
  - id: "code-style"  # Same ID = override
    name: "Code Style (Local)"
    priority: 15
    content: "LOCAL: Use 2 spaces, semicolons required"
  - name: "Communication Style"
    content: "Be concise and direct. Address me as 'Boss'. Always ask for clarification before making assumptions."
  - name: "Response Format"
    content: "Provide code examples for every suggestion. Use bullet points for lists."
```

## Sorting and Output Order

All content (rules and sections) uses dual sorting:
1. **Primary**: By priority (descending) - higher numbers first
2. **Secondary**: By title/name (ascending) - alphabetical order

This ensures consistent, predictable output across regenerations.

## Gitignore Integration

AI Rulez can automatically update `.gitignore` files to include generated output files when using the `--update-gitignore` flag with the `generate` command:

```bash
# Update .gitignore with generated files
ai-rulez generate --update-gitignore

# Works with recursive mode too
ai-rulez generate --recursive --update-gitignore
```

**How it works:**
- Finds the `.gitignore` file in the same directory as each configuration file
- Adds output file names (e.g., `CLAUDE.md`, `.cursorrules`) if they're not already ignored
- Creates a new `.gitignore` file if one doesn't exist
- Adds a comment section to group AI-generated files
- Skips files that are already covered by existing patterns (e.g., `*.md` would cover `CLAUDE.md`)

**Example `.gitignore` addition:**
```gitignore
# AI Rules generated files
CLAUDE.md
.cursorrules
.windsurfrules
```

This feature is especially useful in team environments where you want to ensure generated files don't get committed to version control.

## AI-First Design: Generated File Headers

AI Rulez automatically adds comprehensive headers to all generated files that clearly communicate to both AI assistants and developers that the files are generated and should not be edited directly:

```markdown
<!-- 
🤖 GENERATED FILE - DO NOT EDIT DIRECTLY
===========================================

This file was automatically generated by ai-rulez from ai-rulez.yaml.

⚠️  IMPORTANT FOR AI ASSISTANTS AND DEVELOPERS:
- DO NOT modify this file directly
- DO NOT add, remove, or change rules in this file
- Changes made here will be OVERWRITTEN on next generation

✅ TO UPDATE RULES:
1. Edit the source configuration: ai-rulez.yaml
2. Regenerate this file: ai-rulez generate
3. The updated CLAUDE.md will be created automatically

📝 Generated: 2025-07-06 00:06:15
📁 Source: ai-rulez.yaml
🎯 Target: CLAUDE.md
📊 Content: 7 rules, 0 sections, 2 agents

Learn more: https://github.com/Goldziher/ai-rulez
===========================================
-->
```

**Key Features:**
- **AI Assistant Targeted**: Clear, structured instructions that AI agents can easily parse and understand
- **Actionable Guidance**: Specific steps on how to properly update rules
- **File Metadata**: Shows source config, target file, generation time, and content statistics
- **Consistent Format**: Uses HTML comments that work across all output formats (Markdown, text files, etc.)

This ensures that AI assistants like Claude, Cursor, and others understand that these are generated files and will guide users to edit the source configuration instead of the generated output.

## 🛠️ Commands

### Core Commands

```bash
# Generate all configuration files
ai-rulez generate

# Validate configuration
ai-rulez validate

# Generate recursively in subdirectories  
ai-rulez generate --recursive

# Preview output without writing files
ai-rulez generate --dry-run

# Update .gitignore files with generated output files
ai-rulez generate --update-gitignore

# Initialize new project
ai-rulez init "My Project"

# Start MCP server for AI assistant integration
npx ai-rulez mcp

# Show version
ai-rulez version

# Show help
ai-rulez --help
```

### Managing Rules, Sections, and Outputs

```bash
# Add new rule
ai-rulez add rule "Testing Standards" --priority 8

# Add new section  
ai-rulez add section "Architecture Guidelines" --priority 10

# Add new output file
ai-rulez add output "docs/AI_RULES.md" --template documentation

# Update existing rule
ai-rulez update rule "Testing Standards" --content "Write comprehensive tests"

# Update existing section
ai-rulez update section "Architecture Guidelines" --priority 15

# Update output template
ai-rulez update output "docs/AI_RULES.md" --template custom

# Delete rule
ai-rulez delete rule "Old Rule"

# Delete section
ai-rulez delete section "Deprecated Section"

# Delete output
ai-rulez delete output "unused-file.md"
```

## 🤖 MCP Server Integration

ai-rulez includes a built-in MCP (Model Context Protocol) server that allows AI assistants like Claude Desktop to interact with your AI rules configuration directly.

### Starting the MCP Server

```bash
# Using npx (recommended - no installation needed)
npx ai-rulez mcp

# Or if installed globally
ai-rulez mcp
```

This starts the MCP server in stdio mode, which can be configured in your AI assistant's MCP settings.

### Available MCP Tools

The MCP server exposes these tools:

- **get_rules**: Retrieve rules with filtering by priority and name
- **get_sections**: Get documentation sections from configuration
- **generate_output**: Generate AI rules files with gitignore support
- **validate_config**: Validate configuration files
- **list_templates**: List available project templates
- **add_rule/section/output**: Add new items to configuration
- **update_rule/section/output**: Update existing items
- **delete_rule/section/output**: Remove items from configuration

### Configuring Claude Desktop

#### Quick Setup with Claude CLI (Recommended)
```bash
# Add ai-rulez MCP server using Claude's CLI
claude mcp add ai-rulez npx ai-rulez@latest mcp
```

#### Manual Configuration
Add to your Claude Desktop MCP configuration:

##### Using npx (no installation required)
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

##### Alternative: Global installation
If you have ai-rulez installed globally:

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "ai-rulez",
      "args": ["mcp"]
    }
  }
}
```

This enables Claude to directly manage your AI rules, generate files, and validate configurations without leaving the chat interface.

## 🎨 Template Variables

| Variable | Type | Description |
|----------|------|-------------|
| `{{.ProjectName}}` | string | Project name |
| `{{.Version}}` | string | Version string |
| `{{.Description}}` | string | Project description |
| `{{.Rules}}` | []Rule | Rules array (sorted) |
| `{{.Sections}}` | []Section | Sections array (sorted) |
| `{{.Agents}}` | []Agent | Agents array (sorted) |
| `{{.AllContent}}` | []ContentItem | Combined rules + sections (sorted) |
| `{{.Timestamp}}` | time.Time | Generation timestamp |
| `{{.RuleCount}}` | int | Number of rules |
| `{{.SectionCount}}` | int | Number of sections |
| `{{.AgentCount}}` | int | Number of agents |
| `{{.ConfigFile}}` | string | Source configuration file |
| `{{.OutputFile}}` | string | Target output file |

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
- `--update-gitignore`: Update .gitignore files to include generated output files

### `ai-rulez validate [config-file]`
Validate configuration file against schema.

### `ai-rulez mcp`
Start MCP server for AI assistant integration. Runs in stdio mode for communication with MCP-compatible tools.

```bash
# Recommended: using npx (no installation required)
npx ai-rulez mcp

# Alternative: if globally installed
ai-rulez mcp
```

### `ai-rulez add`
Add new items to configuration. Has three subcommands:

#### `ai-rulez add rule [name]`
Add a new rule to the configuration.

**Options:**
- `--priority, -p`: Priority level (1-10, default: 5)
- `--config, -c`: Config file path (auto-discover if not provided)

#### `ai-rulez add section [title]`
Add a new documentation section to the configuration.

**Options:**
- `--priority, -p`: Priority level (default: 5)
- `--config, -c`: Config file path (auto-discover if not provided)

#### `ai-rulez add output [filename]`
Add a new output file to the configuration.

**Options:**
- `--template, -t`: Template to use for the output (optional)
- `--config, -c`: Config file path (auto-discover if not provided)

### `ai-rulez update`
Update existing items in configuration. Has three subcommands:

#### `ai-rulez update rule [name]`
Update an existing rule's content or priority.

**Options:**
- `--content`: New content for the rule (optional, will prompt if not provided)
- `--priority, -p`: New priority level (optional)
- `--config, -c`: Config file path (auto-discover if not provided)

#### `ai-rulez update section [title]`
Update an existing section's content or priority.

**Options:**
- `--content`: New content for the section (optional, will prompt if not provided)
- `--priority, -p`: New priority level (optional)
- `--config, -c`: Config file path (auto-discover if not provided)

#### `ai-rulez update output [filename]`
Update an output file's template.

**Options:**
- `--template, -t`: New template for the output (required)
- `--config, -c`: Config file path (auto-discover if not provided)

### `ai-rulez delete`
Delete items from configuration. Has three subcommands:

#### `ai-rulez delete rule [name]`
Delete a rule from the configuration.

**Options:**
- `--config, -c`: Config file path (auto-discover if not provided)

#### `ai-rulez delete section [title]`
Delete a section from the configuration.

**Options:**
- `--config, -c`: Config file path (auto-discover if not provided)

#### `ai-rulez delete output [filename]`
Delete an output file from the configuration.

**Options:**
- `--config, -c`: Config file path (auto-discover if not provided)

### `ai-rulez version`
Print the version of ai-rulez.

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