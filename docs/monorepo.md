# Monorepo Configuration

ai-rulez is designed with monorepos in mind, allowing you to create multiple scoped configurations that reduce context size and improve AI assistant focus.

## Why Multiple Configurations?

In large codebases, a single massive rules file can overwhelm AI assistants with irrelevant context. Multiple configurations allow you to:

✅ **Reduce Context Size** - Each part of the codebase gets relevant rules only  
✅ **Improve Focus** - Frontend rules stay with frontend, backend rules with backend  
✅ **Technology-Specific Guidance** - Different stacks get different patterns  
✅ **Better Performance** - Smaller files = faster processing  
✅ **Easier Maintenance** - Update specific areas without affecting others  

## Configuration Patterns

### Pattern 1: Technology-Based Separation

Separate configurations by technology stack (most common):

```
my-project/
├── ai-rulez.yaml           # Root: General project rules + agents
├── backend/
│   └── ai-rulez.yaml       # Backend: API patterns, database rules
└── apps/
    └── ai-rulez.yaml       # Frontend: Component patterns, routing
```

**Root configuration** (`ai-rulez.yaml`):
```yaml
metadata:
  name: "MyProject"
  description: "Full-stack application - System Overview"

# Overall architecture and agent definitions
agents:
  - name: "fullstack-architect"
    description: "System architecture and design decisions"
    system_prompt: |
      You are a senior architect. For domain-specific work, delegate to:
      - backend-engineer: API and database tasks
      - frontend-engineer: Component and UI tasks
      - devsecops-engineer: Infrastructure and security

rules:
  - name: "Project Structure"
    priority: critical
    content: |
      **Monorepo Structure**:
      - `backend/` - Fastify API, PostgreSQL, Docker
      - `apps/` - Next.js applications, React components
      - `packages/` - Shared utilities, UI components

outputs:
  - file: "CLAUDE.md"
  - path: ".cursor/rules/"
    type: "rule"
    naming_scheme: "project-rules.mdc"
```

**Backend configuration** (`backend/ai-rulez.yaml`):
```yaml
metadata:
  name: "MyProject Backend"
  description: "API Server - Patterns and Guidelines"

rules:
  - name: "API Patterns"
    priority: high
    content: |
      **Route Pattern**:
      ```typescript
      app.post<{ Body: CreateInput }>('/items', {
        preHandler: app.auth([app.requireRole(Role.Admin)]),
        schema: { body: CreateSchema }
      }, async (request) => {
        const item = await createItem(request.body);
        return { data: item };
      });
      ```

  - name: "Database Operations"
    priority: high
    content: |
      - Use transactions for multi-table operations
      - Handle unique constraints gracefully
      - Use typed queries with proper error handling

outputs:
  - file: "CLAUDE.md"
  - file: ".cursor/rules/backend-rules.mdc"
```

**Frontend configuration** (`apps/ai-rulez.yaml`):
```yaml
metadata:
  name: "MyProject Frontend"
  description: "Next.js Applications - Patterns and Guidelines"

rules:
  - name: "Component Patterns"
    priority: high
    content: |
      **Component Structure**:
      ```typescript
      'use client';
      
      export function ItemCard({ item }: { item: Item }) {
        const { mutate } = useSWR(`/api/items/${item.id}`);
        return <div>...</div>;
      }
      ```

  - name: "State Management"
    priority: high
    content: |
      - Use Zustand for client state
      - Use SWR for server state
      - Prefer server components when possible

outputs:
  - file: "CLAUDE.md"
  - file: ".cursor/rules/frontend-rules.mdc"
```

### Pattern 2: Service-Based Separation

For microservices architectures:

```
project/
├── ai-rulez.yaml           # Root: Overall architecture
├── services/
│   ├── auth-service/
│   │   └── ai-rulez.yaml   # Authentication patterns
│   ├── payment-service/
│   │   └── ai-rulez.yaml   # Payment processing rules
│   └── notification-service/
│       └── ai-rulez.yaml   # Notification patterns
└── shared/
    └── ai-rulez.yaml       # Shared utilities and types
```

### Pattern 3: Feature-Based Separation

For feature-driven development:

```
app/
├── ai-rulez.yaml           # Root: App-wide patterns
├── features/
│   ├── auth/
│   │   └── ai-rulez.yaml   # Authentication feature rules
│   ├── dashboard/
│   │   └── ai-rulez.yaml   # Dashboard patterns
│   └── settings/
│       └── ai-rulez.yaml   # Settings UI patterns
└── shared/
    └── ai-rulez.yaml       # Shared components and utils
```

## Best Practices

### 1. Root Configuration Strategy

Your root `ai-rulez.yaml` should contain:

- **System-wide architecture rules**
- **Agent definitions with delegation patterns**
- **Technology stack overview**
- **Cross-cutting concerns (logging, error handling)**

```yaml
# Root ai-rulez.yaml
metadata:
  name: "Project Name"
  description: "System overview and architecture"

agents:
  - name: "system-architect"
    description: "Overall system design and cross-cutting concerns"
    system_prompt: |
      You are the system architect. When working on specific domains:
      - Use backend-engineer agent for API/database work
      - Use frontend-engineer agent for UI/component work
      - Use devsecops-engineer agent for infrastructure

rules:
  - name: "Architecture Overview" 
    priority: critical
    content: |
      **System Architecture**: Microservices with React frontend
      **Key Patterns**: Repository pattern, CQRS, Event sourcing
      **Technology**: Node.js, PostgreSQL, Redis, Docker

  - name: "Cross-Cutting Concerns"
    priority: high
    content: |
      - All services use structured logging with correlation IDs
      - Error handling follows standard HTTP status codes
      - Authentication via JWT with refresh tokens
```

### 2. Scoped Configuration Strategy

Each scoped configuration should:

- **Focus on specific technology/domain**
- **Include concrete code patterns**
- **Reference shared conventions from root**
- **Use appropriate priorities (lower than root)**

```yaml
# backend/ai-rulez.yaml
metadata:
  name: "Backend Services"
  description: "API patterns and database operations"

rules:
  - name: "API Route Pattern"
    priority: high  # Lower than root rules
    content: |
      **Standard Route**:
      ```javascript
      app.post('/api/users', validateRequest, async (req, res) => {
        const user = await userService.create(req.body);
        res.status(201).json({ data: user });
      });
      ```
```

### 3. Agent Delegation Patterns

Use agents to route work to the right expertise:

```yaml
# Root configuration
agents:
  - name: "fullstack-architect"
    system_prompt: |
      For domain-specific tasks, delegate to specialized agents:
      
      **Backend Tasks**: Use backend-engineer agent for:
      - API endpoint implementation
      - Database schema changes
      - Authentication middleware
      
      **Frontend Tasks**: Use frontend-engineer agent for:
      - React component development
      - State management implementation
      - User interface design
      
      **Infrastructure Tasks**: Use devsecops-engineer agent for:
      - CI/CD pipeline changes
      - Container configuration  
      - Security reviews
```

### 4. Output Organization

Organize outputs to match your project structure:

```yaml
# Root ai-rulez.yaml
outputs:
  - file: "CLAUDE.md"                    # System overview
  - path: ".cursor/rules/"
    type: "rule"
    naming_scheme: "system.mdc"

# backend/ai-rulez.yaml  
outputs:
  - file: "CLAUDE.md"                    # Backend-specific
  - path: "../.cursor/rules/"
    type: "rule"
    naming_scheme: "backend.mdc"

# frontend/ai-rulez.yaml
outputs:
  - file: "CLAUDE.md"                    # Frontend-specific
  - path: "../.cursor/rules/"
    type: "rule" 
    naming_scheme: "frontend.mdc"
```

This creates:
```
.cursor/rules/
├── system.mdc      # Root rules
├── backend.mdc     # Backend rules  
└── frontend.mdc    # Frontend rules
```

### 5. Rule Priority Management

Use priority levels to create rule hierarchy:

- **100-90**: System architecture and cross-cutting concerns (root)
- **89-80**: Technology stack patterns (scoped configs)
- **79-70**: Specific implementation details (scoped configs)
- **69-50**: Code style and conventions (scoped configs)

### 6. Shared Configurations with Remote Includes

Share common patterns across projects:

```yaml
# Root configuration
includes:
  - "https://your-org.com/ai-rules/typescript-standards.yaml"
  - "https://your-org.com/ai-rules/security-patterns.yaml"
  - "./local-overrides.yaml"

# Backend-specific
includes:
  - "https://your-org.com/ai-rules/nodejs-api-patterns.yaml"
  - "../shared-backend-rules.yaml"
```

## File Discovery

ai-rulez automatically discovers configurations in subdirectories:

```bash
# From project root, finds and processes all configs
ai-rulez generate

# From specific directory, processes only that config
cd backend && ai-rulez generate

# Explicit config file
ai-rulez generate --config custom-config.yaml
```

## Migration Strategy

### From Single File to Multiple Files

1. **Start with existing configuration**:
```yaml
# Current ai-rulez.yaml (too large)
rules:
  - name: "Frontend Patterns" 
  - name: "Backend Patterns"
  - name: "Database Rules"
  # ... 50 more rules
```

2. **Extract by domain**:
```bash
# Create domain-specific configs
mkdir -p backend frontend shared
# Move relevant rules to each file
```

3. **Update root to focus on architecture**:
```yaml
# New root ai-rulez.yaml
metadata:
  name: "Project Architecture"
  
rules:
  - name: "System Overview"
    priority: critical
    content: "High-level architecture and patterns"
    
agents:
  - name: "system-architect"
    # Define delegation patterns
```

4. **Test the split**:
```bash
# Verify all files generate correctly
ai-rulez generate
ai-rulez validate
```

## Real-World Examples

### Example 1: Value8 Cap Table Platform

```
value8/
├── ai-rulez.yaml           # System overview + agent definitions
├── backend/ai-rulez.yaml   # Fastify API patterns
└── apps/ai-rulez.yaml      # Next.js multi-zone architecture
```

- **Root**: 13 rules, 548 lines - System architecture, agent delegation
- **Backend**: 5 rules, 209 lines - Fastify patterns, database operations  
- **Apps**: 10 rules, 553 lines - Next.js components, authentication

### Example 2: GrantFlow AI Platform

```
grantflow/
└── ai-rulez.yaml           # Single file with specialized agents
```

- **Single file**: 20 rules, 370 lines - All rules with agent specialization
- **Agents handle domain separation**: python-engineer, typescript-engineer, devsecops-agent

## Common Patterns

### Pattern: Backend + Frontend Split
```yaml
# Root: agents + system architecture
# backend/: API patterns, database, authentication  
# frontend/: components, routing, state management
```

### Pattern: Microservices
```yaml
# Root: service communication, shared patterns
# service-*/: service-specific patterns and rules
# shared/: common utilities and types
```

### Pattern: Monolithic with Features
```yaml
# Root: app-wide architecture and routing
# features/*/: feature-specific patterns
# shared/: reusable components and utilities
```

Choose the pattern that matches your project structure and team organization.