# Configuration Examples

Use these examples as a starting point for your own `ai-rulez.yml` configuration, progressing from a minimal setup to a complete, real-world application.

---

### 1. AI-Generated Configuration (Recommended)

The best way to get started is with AI-powered configuration generation. Run this command and the AI will analyze your project:

```bash
ai-rulez init "My Project" --preset popular --use-agent claude
```

This automatically generates a comprehensive configuration tailored to your specific codebase. Here's what it might create for a Go microservice:

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "User Management Service"
  description: "Go-based microservice handling user authentication and profiles with PostgreSQL backend"
  version: "1.0.0"

presets:
  - "popular"  # Claude, Cursor, Windsurf, Copilot

rules:
  - name: "Go Code Standards"
    priority: high
    content: |
      Follow standard Go project layout (cmd/, internal/, pkg/). 
      Use meaningful package names and export only what needs to be public.
      Prefer composition over inheritance.
      
  - name: "Error Handling"
    priority: critical  
    content: |
      Always handle errors properly. Use wrapped errors with fmt.Errorf("context: %w", err).
      Never ignore errors - at minimum log them.
      Return errors as the last return value.

  - name: "Database Patterns"
    priority: high
    content: |
      Use repository pattern for data access. 
      Always use transactions for multi-table operations.
      Use prepared statements to prevent SQL injection.

sections:
  - name: "Service Architecture" 
    priority: critical
    content: |
      This service implements clean architecture with:
      - cmd/: Application entry points
      - internal/handlers/: HTTP handlers and routing
      - internal/service/: Business logic layer
      - internal/repository/: Data access layer
      - internal/models/: Domain models and DTOs

  - name: "API Standards"
    priority: high  
    content: |
      RESTful API design following OpenAPI 3.0 specification.
      All endpoints return JSON with consistent error format.
      Use HTTP status codes appropriately (200, 201, 400, 401, 404, 500).

agents:
  - name: "go-expert"
    description: "Go language specialist for backend development"
    system_prompt: |
      You are an expert Go developer specializing in microservices. 
      Focus on clean architecture, proper error handling, and performance.
      Always suggest testable patterns and consider concurrency safety.
      
  - name: "api-designer"
    description: "REST API design specialist"  
    system_prompt: |
      You specialize in designing clean, consistent REST APIs.
      Focus on proper HTTP semantics, clear request/response formats,
      and comprehensive error handling. Always consider API versioning.
```

**Key benefits of AI generation:**
- ✅ Automatically detects your tech stack (Go, PostgreSQL, REST APIs)  
- ✅ Creates project-specific rules and documentation
- ✅ Sets up specialized agents for your workflow
- ✅ Saves hours of manual configuration writing

### 2. Minimal Configuration (Using a Preset)

The simplest possible setup for a single AI assistant, using the recommended `presets` key.

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "My Project"

# Use a preset for a single tool
presets:
  - "claude"

rules:
  - name: "Tech Stack"
    content: "This project uses Go and PostgreSQL."
    priority: critical
```

### 3. Multi-Platform Configuration (Using Presets)

Use the `popular` preset to generate synchronized instructions for multiple AI tools from a single source.

```yaml
metadata:
  name: "My Multi-Platform Project"

# The "popular" preset includes Claude, Cursor, Windsurf, and Copilot
presets:
  - "popular"

rules:
  - name: "General Workflow"
    content: "All code must be reviewed and tested before merging."
    priority: critical
```

### 4. Advanced Configuration: Custom Outputs

For more advanced use cases, like generating specialized agent files, the `outputs` key gives you fine-grained control.

```yaml
metadata:
  name: "Project with Agents"

# Use outputs for custom file generation
outputs:
  - path: "CLAUDE.md" # Main instruction file
  - path: ".claude/agents/" # Directory for agent files
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

### 5. Combining Presets and Outputs

You can combine `presets` and `outputs` to use standard configurations while overriding or adding custom files.

```yaml
# Use the popular preset, but override the Claude file with a custom template
presets:
  - "popular"

outputs:
  - path: "CLAUDE.md" # This will override the default Claude output
    template:
      type: "file"
      value: "./my-custom-claude-template.tmpl"
  - path: "INTERNAL_DOCS.md" # Add a completely custom output
```

### 6. Full Configuration with Tools & Commands

A complete, real-world example showcasing all major features, including presets, MCP server integration, and custom slash commands.

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "SaaS Platform API"
  version: "3.0.0"

extends: "./shared/base-go-service.yaml"

includes:
  - "./shared/security-standards.yaml"

presets:
  - "claude"
  - "cursor"
  - "continue-dev"

agents:
  - name: "go-expert"
    description: "For Go-specific questions about our backend services."
    model: "claude-3-opus-20240229"
    system_prompt: "You are an expert in Go, gRPC, and PostgreSQL..."

rules:
  - name: "API Design"
    content: "All new endpoints must follow RESTful principles and be documented in the OpenAPI spec."
    priority: critical

# Integrate external tools like a GitHub server
mcp_servers:
  - name: "github"
    description: "Provides context from GitHub issues and pull requests."
    command: "npx"
    args: ["-y", "@mcp/server-github"]
    env: {"GITHUB_TOKEN": "${GITHUB_TOKEN}"}

# Define custom slash commands for your AI assistant
commands:
  - name: "new-endpoint"
    description: "Scaffold a new API endpoint"
    usage: "/new-endpoint <path> <method>"
    system_prompt: "Generate the boilerplate code for a new RESTful endpoint..."
```

### 7. Composable Configuration with `extends` and `includes`

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
