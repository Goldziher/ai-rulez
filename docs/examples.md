# Examples

Start with the simplest setup and progressively build more complex, real-world configurations.

## 1. Quick Start

The fastest way to get started:

```bash
# Create a new project with popular AI tools
ai-rulez init "My Project" --popular

# Or use specific providers
ai-rulez init "My API" --claude --gemini
ai-rulez init "My App" --claude --cursor --windsurf
```

This generates a complete configuration with agents, rules, and multi-platform support.

## 2. Minimal Custom Setup

Start from scratch with just the essentials:

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "My Project"

rules:
  - name: "Tech Stack"
    content: "React 19, TypeScript 5.8, Tailwind CSS"
    priority: critical

outputs:
  - path: "CLAUDE.md"
```

```bash
ai-rulez generate  # Creates CLAUDE.md with profile rules + your custom rules
```

## 3. Multi-Platform with Named Targets

Organize rules by target platform using named targets:

```yaml
metadata:
  name: "My Project"

targets:
  claude-only:
    - "CLAUDE.md" 
    - ".claude/**/*.md"
  
  all-platforms:
    - "CLAUDE.md"
    - ".cursor/**/*.mdc"
    - ".github/copilot-instructions.md"

rules:
  - name: "Agent Usage"
    targets: ["@claude-only"]
    content: "Always use specialized agents via Task tool for complex work"
    priority: critical
    
  - name: "Code Quality"
    targets: ["@all-platforms"]
    content: "Write clean, tested, documented code"
    priority: high

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/rules.mdc"
  - path: ".github/copilot-instructions.md"
```

## 4. Real-World Full-Stack Application

Production-ready configuration with specialized agents and comprehensive rules:

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "E-commerce Platform"
  version: "2.1.0"

targets:
  claude-agents:
    - ".claude/agents/*.md"
    
  all-platforms:
    - "CLAUDE.md"
    - ".cursor/**/*.mdc"
    - ".github/copilot-instructions.md"

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/rules.mdc"
  - path: ".github/copilot-instructions.md" 
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{name}.md"

agents:
  - name: "fullstack-architect"
    description: "System architecture and design decisions"
    targets: ["@claude-agents"]
    system_prompt: |
      Senior architect specializing in:
      - Scalable system design patterns
      - Database schema optimization
      - Performance and security architecture
      - Microservices and API design
      
  - name: "frontend-engineer"  
    description: "React/Next.js development specialist"
    targets: ["@claude-agents"]
    system_prompt: |
      Frontend expert focused on:
      - React 19 patterns and performance optimization
      - TypeScript strict mode and type safety
      - Accessibility (WCAG 2.1) and responsive design
      - State management with Zustand/Redux Toolkit
      
  - name: "backend-engineer"
    description: "API and database specialist" 
    targets: ["@claude-agents"]
    system_prompt: |
      Backend engineer specializing in:
      - RESTful API design and GraphQL
      - PostgreSQL optimization and migrations
      - Authentication and authorization (JWT, OAuth)
      - Caching strategies and performance tuning
      
  - name: "code-reviewer"
    description: "Code quality and security analysis"
    targets: ["@claude-agents"] 
    system_prompt: |
      Senior code reviewer ensuring:
      - Security vulnerability assessment
      - Performance optimization opportunities
      - Code maintainability and documentation
      - Testing coverage and quality

rules:
  - name: "System Workflow"
    priority: critical0
    content: |
      Follow systematic development process: design → implement → test → review
      
      **Always use specialized agents**: Use Task tool with appropriate agent for domain expertise
  
  - name: "Tech Stack"
    priority: high0
    content: |
      **Frontend**: Next.js 15, React 19, TypeScript 5.8, Tailwind CSS
      **Backend**: Node.js 20, Express/Fastify, PostgreSQL 16
      **Auth**: NextAuth.js with JWT, role-based access control
      **Testing**: Vitest, Playwright, React Testing Library
      **Infrastructure**: Docker, Vercel/AWS, GitHub Actions
      **Database**: PostgreSQL with Prisma ORM
      
  - name: "Development Standards"
    priority: high5
    content: |
      **Code Quality**:
      - TypeScript strict mode, no `any` types
      - ESLint + Prettier with Biome formatter
      - 90%+ test coverage requirement
      
      **Security**:
      - Input validation with Zod schemas
      - SQL injection prevention (parameterized queries)
      - XSS protection and Content Security Policy
      - Regular dependency security audits
      
      **Performance**:
      - React component memoization
      - Database query optimization
      - Image optimization and lazy loading
      - Bundle size monitoring
```

## 5. Configuration Inheritance (Extends)

Create specialized configurations that inherit from a base configuration:

### Base Configuration (shared across projects)
```yaml
# https://raw.githubusercontent.com/myorg/standards/main/typescript-base.yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  description: Base TypeScript configuration for all projects

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/rules.mdc"

rules:
  - name: "TypeScript Standards" 
    priority: critical
    content: |
      **Language**: TypeScript 5.8+ with strict mode enabled
      **Formatting**: ESLint + Biome with automated fixes
      **Testing**: Vitest for unit tests, Playwright for E2E
      **Build**: Vite for bundling and development server
      
  - name: "Code Quality"
    priority: high
    content: |
      - Use explicit return types for functions
      - No `any` types - use proper typing
      - Prefer composition over inheritance
      - Keep functions small and focused

agents:
  - name: "typescript-specialist"
    description: "TypeScript development specialist"
    system_prompt: |
      TypeScript expert focusing on:
      - Modern TypeScript patterns and best practices
      - Type safety and advanced type system features
      - Performance optimization and bundle analysis
      - Testing strategies with TypeScript
```

### Child Configuration (inherits and customizes)
```yaml
# Frontend project extends the base
extends: "https://raw.githubusercontent.com/myorg/standards/main/typescript-base.yaml"

metadata:
  name: "E-commerce Frontend"
  version: "2.1.0"

# Adds additional outputs beyond base
outputs:
  - path: ".github/copilot-instructions.md"

rules:
  - name: "Frontend Framework"  # New rule specific to frontend
    priority: critical
    content: |
      **Framework**: React 19 with concurrent features
      **State**: Zustand for client state, TanStack Query for server state
      **Styling**: Tailwind CSS v4 with design system tokens
      **Routing**: React Router v6 with data loading patterns

  - name: "Code Quality"        # Overrides base rule
    priority: critical          # Higher priority than base
    content: |
      **Frontend Code Quality**:
      - Component composition patterns
      - Custom hooks for reusable logic  
      - Proper error boundaries and loading states
      - Accessibility (WCAG 2.1 AA compliance)
      - Performance: lazy loading, code splitting, memoization

agents:
  - name: "react-specialist"    # Additional agent for frontend
    description: "React development and UI specialist" 
    system_prompt: |
      React expert specializing in:
      - Component design patterns and composition
      - State management and data fetching
      - Performance optimization (React DevTools, Profiler)
      - Accessibility and responsive design principles
```

**Result**: Child inherits base TypeScript standards + agents, adds React-specific rules and agents, and overrides the Code Quality rule with frontend-specific content.

## 6. Extends + Includes Combined (Maximum Flexibility)

Use both extends and includes together for maximum configuration power:

```yaml
# Project inherits base + composes additional pieces
extends: "https://company.com/base-typescript.yaml"
includes:
  - "https://company.com/security-standards.yaml"
  - "https://company.com/testing-standards.yaml"
  - "./local-team-rules.yaml"

metadata:
  name: "E-commerce Frontend"
  version: "2.1.0"

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/rules.mdc"
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{name}.md"

agents:
  - name: "ecommerce-specialist"
    description: "E-commerce domain expert"
    system_prompt: |
      E-commerce specialist focusing on:
      - Shopping cart and checkout flows
      - Payment processing integration
      - Product catalog management
      - Performance optimization for large catalogs

rules:
  - name: "E-commerce Patterns"
    priority: critical
    content: |
      **Shopping Cart**:
      - Persist cart state across sessions
      - Handle inventory updates in real-time
      - Implement optimistic UI updates
      
      **Product Catalog**:
      - Use virtualization for large product lists
      - Implement smart search with filters
      - Optimize images with next/image
      
      **Checkout Process**:
      - Multi-step form validation
      - Payment method flexibility
      - Order confirmation flows
```

**Processing Flow:**
1. **Extends**: Inherits base TypeScript configuration (standards, tooling, base agents)
2. **Includes**: Merges security standards, testing patterns, and local team rules
3. **Local**: Adds e-commerce specific agent and rules

**Benefits:**
- Base standards provide consistency across all projects
- Includes add specialized knowledge (security, testing) 
- Local config customizes for specific domain (e-commerce)
- Include files can themselves have extends for nested inheritance

## 7. Remote Configuration Sharing

Share common rules across projects:

```yaml
includes:
  - "https://raw.githubusercontent.com/myorg/standards/main/backend.yaml"
  
metadata:
  name: "User Service"
  
rules:
  - name: "service-specific"
    content: "Handle user authentication with JWT"
    priority: critical

outputs:
  - path: "CLAUDE.md"
```

Remote rules are merged with local ones. Local rules override remote rules with the same name.

---

## Language-Specific Examples

Build on the foundation above with these complete project examples.

### Modern Python API Service

FastAPI microservice with comprehensive setup:

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "User Management API"

agents:
  - name: "backend-engineer"
    description: "Python/FastAPI development specialist"
    system_prompt: |
      Python backend engineer specializing in:
      - FastAPI async patterns and dependency injection
      - SQLAlchemy 2.0 async ORM with Alembic migrations
      - Pydantic v2 models and validation
      - PostgreSQL optimization and indexing
      - JWT authentication and RBAC authorization
      - Docker containerization and deployment

rules:
  - name: "Python Standards"
    priority: critical
    content: |
      **Language**: Python 3.12+ with strict typing
      **Framework**: FastAPI with async/await patterns
      **Database**: PostgreSQL 16 with SQLAlchemy 2.0 async
      **Validation**: Pydantic v2 models with field validation
      **Testing**: pytest with async support, factoryboy for fixtures
      **Linting**: ruff for linting/formatting, mypy for type checking

  - name: "API Patterns" 
    priority: high
    content: |
      ```python
      from fastapi import FastAPI, Depends, HTTPException
      from sqlalchemy.ext.asyncio import AsyncSession
      
      @app.post("/users/", response_model=UserResponse)
      async def create_user(
          user_data: UserCreate,
          db: AsyncSession = Depends(get_db),
          current_user: User = Depends(get_current_user)
      ):
          # Input validation automatic via Pydantic
          # Authorization handled by dependency
          async with db.begin():
              user = await create_user_in_db(db, user_data)
              return UserResponse.model_validate(user)
      ```

outputs:
  - path: "CLAUDE.md"  
  - path: ".cursor/rules/rules.mdc"
  - path: ".claude/agents/"
    type: "agent"
```

### Next.js Full-Stack Application

Modern React app with TypeScript and comprehensive tooling:

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "SaaS Dashboard"

targets:
  claude-agents:
    - ".claude/agents/*.md"

agents:
  - name: "frontend-engineer"
    description: "Next.js/React development specialist"
    targets: ["@claude-agents"]
    system_prompt: |
      Frontend expert specializing in:
      - Next.js 15 App Router with server components
      - React 19 concurrent features and Suspense
      - TypeScript 5.8 strict mode with advanced types
      - Tailwind CSS v4 with design system
      - Zustand for state management
      - React Hook Form with Zod validation

rules:
  - name: "Tech Stack"
    priority: critical
    content: |
      **Framework**: Next.js 15 App Router with TypeScript 5.8
      **Styling**: Tailwind CSS v4 with custom design system
      **State**: Zustand for client state, SWR for server state
      **Forms**: React Hook Form with Zod schema validation
      **UI**: shadcn/ui components with Radix primitives
      **Testing**: Vitest + React Testing Library + Playwright
      **Auth**: NextAuth.js v5 with multiple providers
      
  - name: "Code Patterns"
    priority: high  
    content: |
      **Server Components**:
      ```tsx
      // app/dashboard/page.tsx
      export default async function DashboardPage() {
        const user = await getCurrentUser()
        const projects = await getProjects(user.id)
        
        return (
          <div className="grid grid-cols-12 gap-6">
            <Suspense fallback={<ProjectsSkeleton />}>
              <ProjectsList projects={projects} />
            </Suspense>
          </div>
        )
      }
      ```
      
      **Client Components with State**:
      ```tsx
      'use client'
      import { useProjectStore } from '@/stores/projects'
      import { useSWR } from 'swr'
      
      export function ProjectsList() {
        const { data: projects, error, isLoading } = useSWR(
          '/api/projects', 
          fetcher
        )
        
        if (error) return <ErrorBoundary error={error} />
        if (isLoading) return <ProjectsSkeleton />
        
        return <ProjectGrid projects={projects} />
      }
      ```

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/rules.mdc"
  - path: ".windsurf/rules.md"
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{name}.md"
```

### Go Microservice with gRPC

High-performance service with modern Go patterns:

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "Payment Service"

agents:
  - name: "go-engineer"
    description: "Go microservice development specialist"
    system_prompt: |
      Go backend engineer specializing in:
      - Microservice architecture with gRPC and REST
      - Database integration with sqlx and migrations
      - Observability with OpenTelemetry and structured logging
      - Testing strategies with testify and integration tests
      - Container deployment with Docker and Kubernetes

rules:
  - name: "Go Standards"
    priority: critical
    content: |
      **Language**: Go 1.23+ with generics and structured logging
      **Architecture**: Clean architecture with dependency injection
      **Database**: PostgreSQL with sqlx for queries, migrate for schema
      **API**: gRPC with Connect protocol, OpenAPI for REST endpoints  
      **Testing**: testify for assertions, dockertest for integration
      **Observability**: slog for logging, OpenTelemetry for tracing
      
  - name: "Service Patterns"
    priority: high
    content: |
      ```go
      // Domain service with dependency injection
      type PaymentService struct {
          repo PaymentRepository
          logger *slog.Logger
          tracer trace.Tracer
      }
      
      func (s *PaymentService) CreatePayment(
          ctx context.Context, 
          req *CreatePaymentRequest
      ) (*Payment, error) {
          ctx, span := s.tracer.Start(ctx, "payment.create")
          defer span.End()
          
          // Validate input
          if err := req.Validate(); err != nil {
              s.logger.Error("invalid payment request", 
                  "error", err, "user_id", req.UserID)
              return nil, fmt.Errorf("validation failed: %w", err)
          }
          
          // Business logic with transaction
          payment, err := s.repo.CreateWithTx(ctx, req)
          if err != nil {
              span.RecordError(err)
              return nil, fmt.Errorf("failed to create payment: %w", err)
          }
          
          s.logger.Info("payment created", 
              "payment_id", payment.ID, "amount", payment.Amount)
          return payment, nil
      }
      ```

outputs:
  - path: "CLAUDE.md"
  - path: "AGENTS.md"  # For AMP/Codex compatibility
  - path: ".cursor/rules/rules.mdc"
```

Start with the minimal examples above, then customize for your specific needs.