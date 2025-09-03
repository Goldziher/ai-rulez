# Quick Start

Get started instantly with presets, then customize for your project.

## 1. Create Configuration

**For popular AI tools (Recommended):**
```bash
ai-rulez init "My Project" --popular
```

**For specific providers:**
```bash
# Web application with Claude + Cursor
ai-rulez init "My App" --claude --cursor

# Backend API with multiple tools  
ai-rulez init "My API" --claude --gemini --copilot

# All supported platforms
ai-rulez init "My Project" --all
```

This creates `ai_rulez.yaml` with complete setup including agents, best practices, and multi-platform support.

## 2. Generate AI Files

```bash
ai-rulez generate
```

Creates all configured files:
- `CLAUDE.md` + `.claude/agents/` (Claude with specialized agents)
- `.cursor/rules/rules.mdc` (Cursor IDE rules)  
- `.github/copilot-instructions.md` (GitHub Copilot)
- `.windsurf/rules.md` (Windsurf editor)

## 3. Validate and Test

```bash
ai-rulez validate
```

✅ Validates configuration syntax and structure  
✅ Checks for common issues and provides warnings  
✅ Confirms all targets and references work correctly

---

## Customizing Your Configuration

### Add Project-Specific Rules

Add your tech stack and workflow details to `ai_rulez.yaml`:

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "My Project"
  version: "1.0.0"

rules:
  - name: "Tech Stack"
    priority: critical
    content: |
      **Frontend**: React 19, TypeScript 5.8, Tailwind CSS
      **Backend**: Node.js, PostgreSQL, Redis  
      **Testing**: Vitest, Playwright
      **Deployment**: Vercel, GitHub Actions
    
  - name: "Development Workflow"
    priority: high
    content: |
      1. Create feature branch from main
      2. Implement with TypeScript strict mode
      3. Write tests (90%+ coverage required)
      4. Use ESLint + Biome for formatting
      5. Open PR with comprehensive description

outputs:
  - path: "CLAUDE.md"
  - path: ".cursor/rules/rules.mdc"
```

### Add Specialized Agents

Create domain experts for complex tasks:

```yaml
agents:
  - name: "database-specialist"
    description: "PostgreSQL optimization and schema design"
    system_prompt: |
      Database expert specializing in:
      - PostgreSQL query optimization and indexing
      - Schema design and migration strategies
      - Performance monitoring and tuning
      - Data integrity and consistency patterns
      
  - name: "security-auditor"
    description: "Security vulnerability assessment and fixes"
    system_prompt: |
      Security specialist focusing on:
      - Input validation and sanitization
      - Authentication and authorization flaws
      - SQL injection and XSS prevention
      - Dependency vulnerability scanning

outputs:
  - path: "CLAUDE.md"
  - path: ".claude/agents/"
    type: "agent"
    naming_scheme: "{name}.md"
```

### Use CLI Commands

```bash
# Add a new rule
ai-rulez add rule "error-handling" --content "Handle all errors explicitly" --priority 8

# Add an output  
ai-rulez add output --path ".windsurf/rules.md"
```

### Configuration Inheritance (Extends)

Inherit from existing configurations instead of starting from scratch:

```yaml
# Extend a shared base configuration
extends: "https://raw.githubusercontent.com/myorg/standards/main/typescript-base.yaml"

metadata:
  name: "My Frontend Project"  # Override the base name

rules:
  - name: "Project Specific"   # Add project-specific rules
    priority: critical
    content: "Use React 19 with concurrent features"
```

**Using Extends + Includes Together:**
```yaml
# Maximum flexibility: inherit base + compose additional pieces
extends: "https://company.com/base-config.yaml"
includes:
  - "https://company.com/security-rules.yaml"
  - "./local-customizations.yaml"

metadata:
  name: "My Project"
  
rules:
  - name: "Project Specific"
    content: "Custom rule for this project"
```

**Benefits:**
- **Extends**: Inherit team/organization standards automatically
- **Includes**: Compose additional shared components (security, testing, etc.)
- **Combined**: Get base foundation + specialized add-ons + local customizations
- Maintain consistency across related projects while allowing flexibility
- Reduce configuration duplication

### Advanced: Add Agents (Claude)

For specialized sub-tasks:

```bash
ai-rulez add agent "code-reviewer" \
  --description "Reviews code quality" \
  --system-prompt "You are a senior code reviewer"
```

## Git Hooks (Optional)

Automatically validate configuration on git commit:

```bash
ai-rulez init "My Project" --setup-hooks
```

Works with lefthook, pre-commit, or husky.

## Next Steps

- [Learn about Configuration](configuration.md)
- [Explore CLI Commands](cli.md)  
- [See More Examples](examples.md)