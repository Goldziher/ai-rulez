# ai_rules

A CLI tool for managing AI assistant rules with modular configuration and template support.

## Features

- **Unified Configuration**: Define rules once in YAML format
- **Multiple Outputs**: Generate rules for Claude, Cursor, Windsurf, and any AI assistant
- **Template System**: Built-in and custom templates with full Go template syntax
- **Include System**: Modular rule composition with circular dependency detection
- **Schema Validation**: JSON Schema validation with editor support
- **Sections Support**: Mix informative content (docs, guidelines) with rules
- **Smart Sorting**: Dual sorting by priority and name for consistent output
- **Incremental Generation**: Only writes files when content changes

## Installation

### From Source
```bash
go install github.com/Goldziher/ai_rules@latest
```

### From npm (Coming Soon)
```bash
npm install -g ai_rules
```

### From pip (Coming Soon)
```bash
pip install ai_rules
```

## Quick Start

1. Initialize a new project:
```bash
ai_rules init "My Project"

# With templates
ai_rules init --template react "My React App"
ai_rules init --template typescript "My TS Project"
```

2. Edit the generated `ai_rules.yaml` file

3. Generate rule files:
```bash
ai_rules generate

# Or specify a config file
ai_rules generate path/to/config.yaml
```

## Configuration Format

### Basic Example
```yaml
$schema: https://github.com/Goldziher/ai_rules/schema/ai-rules-v1.schema.json

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

## Template Variables

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

## Commands

### `ai_rules init [project-name]`
Initialize a new configuration file.

Options:
- `--template, -t`: Template to use (`basic`, `react`, `typescript`)

### `ai_rules generate [config-file]`
Generate output files from configuration. Files are only written if content changes.

Default: `ai_rules.yaml` in current directory

### `ai_rules validate [config-file]`
Validate configuration file against schema.

## Editor Support

Add the schema reference to your YAML files for:
- Auto-completion
- Inline documentation
- Real-time validation

```yaml
$schema: https://github.com/Goldziher/ai_rules/schema/ai-rules-v1.schema.json
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
ai_rules/
├── cmd/ai_rules/     # CLI commands
├── internal/         # Internal packages
│   ├── config/       # Configuration and validation
│   ├── generator/    # Output generation
│   └── templates/    # Template rendering
├── schema/           # JSON Schema definitions
├── examples/         # Example configurations
└── testing/          # Test scenarios and data
```

## License

MIT