# Best Practices for Large Projects

As a project grows, a single, monolithic `ai_rulez.yaml` file can become difficult to manage and may provide too much irrelevant context to your AI assistant. `ai-rulez` is designed to solve this problem through a combination of multiple configurations, inheritance, and composition.

This guide outlines best practices for managing AI context at scale.

---

## The Core Strategy: Scoped Configurations

The most effective strategy for large projects is to create multiple, smaller `ai_rulez.yaml` files, each scoped to a specific part of your project.

!!! success "The Goal: High-Relevance, Low-Token Context"

    By scoping configurations, you provide your AI assistant with only the context it needs for the task at hand. A developer working on the frontend gets frontend-specific rules, while the backend developer gets API patterns. This results in faster, more accurate, and more relevant AI responses.

A typical layout might look like this:

```
my-project/
├── ai-rulez.yaml           # ⬅️ Root Config: High-level architecture & shared agents
├── backend/
│   └── ai-rulez.yaml       # ⬅️ Scoped Config: API patterns, database rules
└── frontend/
    └── ai_rulez.yaml       # ⬅️ Scoped Config: Component patterns, state management
```

When you run `ai-rulez generate` from the root, it automatically discovers and processes all of these files, creating a comprehensive set of instructions for your tools.

---

## Best Practice: The Root Configuration

Your root `ai_rulez.yaml` should not contain low-level implementation details. Its purpose is to define the high-level view of your project.

!!! tip "What to put in the Root Config"

    - **System Architecture:** A high-level overview of the tech stack and how the pieces fit together.
    - **Agent Definitions:** Define your specialized agents and, crucially, how they should delegate to one another.
    - **Cross-Cutting Concerns:** Rules for things that apply everywhere, like logging, error handling, and security policies.

```yaml
# /ai_rulez.yaml
metadata:
  name: "My Full-Stack Project"

rules:
  - name: "System Architecture"
    priority: critical
    content: "This is a microservices project with a React frontend and a Go backend using gRPC."

agents:
  - name: "solution-architect"
    description: "For high-level system design questions."
    system_prompt: "You are a solution architect. For implementation details, delegate to the `frontend-expert` or `backend-expert` agents."

outputs:
  - path: "SYSTEM_OVERVIEW.md"
```

## Best Practice: Scoped Configurations

Scoped configurations, like `backend/ai_rulez.yaml`, should contain the specific, detailed context for that part of the codebase.

!!! tip "What to put in a Scoped Config"

    - **Technology-Specific Patterns:** Provide concrete code examples for your frameworks and libraries.
    - **Domain Logic:** Explain the business rules and logic for that specific service or feature.
    - **API Contracts:** Detail the expected request/response shapes for your APIs.

```yaml
# /backend/ai_rulez.yaml
metadata:
  name: "Backend API"

rules:
  - name: "Go API Patterns"
    priority: high
    content: "All new endpoints must include structured logging via the `slog` package and OpenTelemetry tracing."

outputs:
  - path: "BACKEND_RULES.md"
```

## Best Practice: Composable Configurations

For maximum maintainability, avoid duplicating rules. Use `extends` and `includes` to create a single source of truth for your standards.

!!! info "`extends` vs. `includes`"

    - Use `extends` for **inheritance**. A project `extends` a single base configuration (e.g., your company's default setup).
    - Use `includes` for **composition**. A project `includes` multiple, modular sets of rules (e.g., security standards, framework-specific patterns).

**`/shared/base-go-service.yaml` (A reusable base config)**
```yaml
metadata:
  description: "Base configuration for all Go microservices."
rules:
  - name: "Go Best Practices"
    content: "Use structured logging (slog), handle all errors, and write unit tests."
    priority: critical
```

**`/backend/user-service/ai_rulez.yaml` (Inherits the base and adds its own logic)**
```yaml
# Inherit all rules from the company-wide Go base file
extends: "../../shared/base-go-service.yaml"

metadata:
  name: "User Service API"

rules:
  - name: "Service-Specific Logic"
    content: "This service is responsible for JWT authentication."
    priority: high
```

By combining these patterns, you can create a highly maintainable, scalable, and effective set of configurations that will grow with your project.
