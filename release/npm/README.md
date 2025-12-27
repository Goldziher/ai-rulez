# ai-rulez

Directory-based AI governance for development teams.

[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![npm downloads](https://img.shields.io/npm/dm/ai-rulez)](https://www.npmjs.com/package/ai-rulez)

**Documentation:** [goldziher.github.io/ai-rulez](https://goldziher.github.io/ai-rulez/)

---

## What is ai-rulez?

ai-rulez V3 uses a directory-based configuration (`.ai-rulez/`) to organize rules, context, and domain-specific guidance for your AI tools. Write once, generate for all your AI assistants (Claude, Cursor, Windsurf, Copilot, Gemini, and more).

**Core capabilities:**
- Directory-based configuration in `.ai-rulez/`
- Domain-specific rules with profile-based generation
- Compose rules from local packages or Git repositories
- Generate native configs for all major AI tools
- Configure MCP servers for IDE integration
- AI-powered code validation and auto-fixing

---

## Installation

```bash
npm install -g ai-rulez
```

Or use without installation:

```bash
npx ai-rulez@latest init "My Project"
```

---

## Quick Start

### 1. Initialize

```bash
ai-rulez init "My Project" --preset claude
```

This creates a `.ai-rulez/` directory with:
```
.ai-rulez/
├── config.yaml       # Configuration
├── rules/            # Shared rules
├── context/          # Project context
└── domains/          # Optional domain-specific rules
```

### 2. Add Rules

```bash
# Edit .ai-rulez/rules/coding-standards.md
echo "# Coding Standards

Follow idiomatic patterns for your language..." > .ai-rulez/rules/coding-standards.md

# Add project context
echo "# Project Info

This is our SaaS platform..." > .ai-rulez/context/project-info.md
```

### 3. Generate Configuration

```bash
ai-rulez generate
```

Generates native configs: `CLAUDE.md`, `.cursorrules`, `.windsurfrules`, etc.

### 4. Enforce Rules (Optional)

```bash
# Check for violations
ai-rulez enforce --agent claude

# Apply fixes
ai-rulez enforce --agent claude --fix
```

---

## Configuration Example

```yaml
# .ai-rulez/config.yaml
version: "3.0"
name: "My Project"
presets:
  - claude
  - cursor

# Define profiles
profiles:
  backend:
    - backend
  frontend:
    - frontend
  full:
    - backend
    - frontend
```

---

## Common Commands

```bash
# Initialize a project
ai-rulez init "Project Name" --preset claude

# Generate all outputs
ai-rulez generate

# Generate for specific profile
ai-rulez generate --profile frontend

# Validate configuration
ai-rulez validate

# Check for rule violations
ai-rulez enforce --agent claude

# Apply auto-fixes
ai-rulez enforce --agent claude --fix

# Multi-agent review
ai-rulez enforce --agent claude --review --review-iterations 2
```

---

## Learn More

- [Full Documentation](https://goldziher.github.io/ai-rulez/)
- [Configuration Guide](https://goldziher.github.io/ai-rulez/configuration/)
- [Enforcement Guide](https://goldziher.github.io/ai-rulez/enforcement/)
- [CLI Reference](https://goldziher.github.io/ai-rulez/cli/)
- [GitHub](https://github.com/Goldziher/ai-rulez)

---

## Installing Other Platforms

ai-rulez is available for:
- **npm** (Node.js) – `npm install -g ai-rulez`
- **Homebrew** – `brew install goldziher/tap/ai-rulez`
- **Python/pip** – `pip install ai-rulez`
- **Go** – `go install github.com/Goldziher/ai-rulez/cmd@latest`
