# Configuration

## Basic Structure

Start with the minimal configuration, then add complexity as needed.

### Minimal Configuration

```yaml
metadata:
  name: "My Project"

rules:
  - name: "code-quality"
    content: "Write clean, readable code"

outputs:
  - path: "CLAUDE.md"
```

### Add Priorities

Control the order of rules in generated files:

```yaml
metadata:
  name: "My Project"

rules:
  - name: "security"
    content: "Validate all inputs"
    priority: critical          # Higher = appears first
    
  - name: "code-quality"
    content: "Write clean, readable code"
    priority: medium

outputs:
  - path: "CLAUDE.md"
```

### Multiple AI Platforms

Generate files for different AI tools:

```yaml
metadata:
  name: "My Project"

rules:
  - name: "code-quality"
    content: "Write clean, readable code"
    priority: critical

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/rules.mdc"
  - path: ".github/copilot-instructions.md"
```

### Complete Structure

All available configuration options:

```yaml
metadata:
  name: "My Project"
  description: "Optional description"
  version: "1.0.0"

rules:
  - name: "rule-name"
    content: "Rule description"
    priority: critical

sections:
  - name: "Section Name" 
    content: "Longer documentation content"
    priority: medium

agents:
  - name: "agent-name"
    description: "Agent purpose"
    system_prompt: "You are a..."
    tools: ["read", "write"]
    priority: high

outputs:
  - path: "CLAUDE.md"
```

---

## Configuration Details

### Rules

Individual instructions for AI assistants:

```yaml
rules:
  - name: "typescript-strict"
    content: "Use TypeScript strict mode with explicit return types"
    priority: critical              # Higher priority = appears first
```

### Sections

Longer documentation blocks:

```yaml
sections:
  - name: "Project Setup"
    content: |
      This project uses:
      - TypeScript 5.0+
      - React 18
    priority: critical
```

### Agents (Claude Only)

Specialized sub-agents for specific tasks:

```yaml
agents:
  - name: "code-reviewer"
    description: "Reviews code for best practices"
    system_prompt: "You are a senior code reviewer focused on quality"
    tools: ["read", "grep", "edit"]
    priority: critical
```

**Available Tools**: `read`, `write`, `edit`, `grep`, `bash`, `multiedit`

### Outputs

Define which files to generate:

```yaml
outputs:
  # Single files (most common)
  - path: "CLAUDE.md"
  - path: ".cursor/rules/rules.mdc"
  - path: ".github/copilot-instructions.md"
    
  # Directory outputs  
  - path: ".windsurf/"
    type: "rule"
    naming_scheme: "{name}.md"           # One file per rule
    
  - path: ".claude/agents/"
    type: "agent" 
    naming_scheme: "{name}.md"           # One file per agent
```

**Output Field Options:**

- **`path`**: File or directory path (recommended)
- **`file`**: ⚠️ **DEPRECATED** - Use `path` instead. Will be removed in v2.0
- **`type`**: Content type for directory outputs (`rule`, `section`, `agent`)  
- **`naming_scheme`**: File naming pattern for directory outputs
- **`template`**: Custom template (optional - uses default if not specified)

**Available Templates:**

- **`default`**: Standard markdown template (used when no template specified)
- **Inline templates**: Define custom templates directly in configuration:

```yaml
outputs:
  - path: "CUSTOM.md"
    template: |
      # {{.ProjectName}}
      {{range .Rules}}
      ## {{.Name}} (Priority: {{.Priority}})
      {{.Content}}
      {{end}}
```

### Remote Includes

Share configurations across projects:

```yaml
includes:
  - "https://raw.githubusercontent.com/myorg/standards/main/backend.yaml"

rules:
  - name: "project-specific"
    content: "This rule is specific to this project"
```

Remote rules are merged with local ones. Local rules override remote rules with the same name.

### Advanced Configuration

#### User-Specific Rules

Override rules for specific users or environments:

```yaml
user_rulez:
  rules:
    - name: "dev-specific"
      content: "Development-only rule"
      priority: critical
  sections:
    - name: "Dev Environment"
      content: "Local development setup"
  agents:
    - name: "dev-helper"
      description: "Development assistant"
```

#### Targets (Advanced)

Target specific file patterns or directories:

```yaml  
targets:
  backend: ["src/**/*.go", "internal/**/*.go"]
  frontend: ["web/**/*.ts", "web/**/*.tsx"]
```

*Note: This is an advanced feature - most users should use the standard `outputs` configuration.*

---

## Validation

```bash
ai-rulez validate
```

See the [Examples page](examples.md) for complete configurations.