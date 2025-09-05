# Configuration Examples

Use these examples as a starting point for your own `ai-rulez.yml` configuration, progressing from a minimal setup to a complete, real-world application.

---

### 1. Minimal Configuration

The simplest possible setup for a single AI assistant.

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "My Project"

rules:
  - name: "Tech Stack"
    content: "This project uses Go and PostgreSQL."
    priority: critical

outputs:
  - path: "CLAUDE.md"
```

### 2. Multi-Platform Configuration

Generate synchronized instructions for multiple AI tools from a single source.

```yaml
metadata:
  name: "My Multi-Platform Project"

rules:
  - name: "General Workflow"
    content: "All code must be reviewed and tested before merging."
    priority: critical

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/rules.mdc"
  - path: ".github/copilot-instructions.md"
```

### 3. Using Agents and Targeted Rules

Define specialized agents and use `targets` to direct rules to specific outputs.

```yaml
metadata:
  name: "Project with Agents"

outputs:
  - path: "CLAUDE.md"
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{name}.md"

agents:
  - name: "database-expert"
    description: "For questions about schema design and query optimization."
    system_prompt: "You are an expert in PostgreSQL..."

rules:
  - name: "Agent Usage Instructions"
    content: "For database questions, please use the @database-expert agent."
    targets: ["CLAUDE.md"] # This rule only appears in the main Claude file
```

### 4. Full Configuration with Tools & Commands

A complete, real-world example showcasing all major features, including MCP server integration and custom slash commands.

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "SaaS Platform API"
  version: "3.0.0"

extends: "./shared/base-go-service.yaml"

includes:
  - "./shared/security-standards.yaml"

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/rules.mdc"
  - path: ".continue/rules/"
    type: "rule"
    naming_scheme: "{name}.md"

agents:
  - name: "go-expert"
    description: "For Go-specific questions about our backend services."
    model: "claude-3-opus-20240229"  # Optional: for providers that support it (e.g., Continue.dev)
    system_prompt: "You are an expert in Go, gRPC, and PostgreSQL..."

rules:
  - name: "API Design"
    content: "All new endpoints must follow RESTful principles and be documented in the OpenAPI spec."
    priority: critical

# Integrate external tools like a GitHub server
mcp_servers:
  - name: "github"
    id: "github-prod"
    description: "Provides context from GitHub issues and pull requests."
    command: "npx"
    args: ["-y", "@mcp/server-github"]
    env: {"GITHUB_TOKEN": "${GITHUB_TOKEN}"}

# Define custom slash commands for your AI assistant
commands:
  - name: "new-endpoint"
    id: "cmd-new-endpoint"
    description: "Scaffold a new API endpoint"
    usage: "/new-endpoint <path> <method>"
    system_prompt: "Generate the boilerplate code for a new RESTful endpoint..."
    aliases: ["endpoint"]
```

### 5. Composable Configuration with `extends` and `includes`

Build powerful, maintainable configurations by composing multiple files.

**`base.yaml` (Company-wide standards)**
```yaml
metadata:
  description: "Base configuration for all projects at our company."

rules:
  - name: "Security Policy"
    content: "All services must pass a security audit before deployment."
    priority: critical
```

**`backend-service.yaml` (Your project's configuration)**
```yaml
# Inherit all rules from the company-wide base file
extends: "./base.yaml"

# Also merge in the shared Go language standards
includes:
  - "./shared/go-rules.yaml"

metadata:
  name: "User Service"

rules:
  - name: "Service-Specific Logic"
    content: "This service handles user authentication and profile data."
```
