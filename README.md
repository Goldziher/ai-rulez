# ai-rulez

<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/logo.png" alt="ai-rulez logo" width="200" style="border-radius: 15%; overflow: hidden;">
</p>

Directory-based AI governance for development teams.

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8)](https://go.dev)
[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-orange)](https://github.com/Goldziher/homebrew-tap)

**Documentation:** [goldziher.github.io/ai-rulez](https://goldziher.github.io/ai-rulez/)

---

## Overview

ai-rulez V3 uses a directory-based configuration (`.ai-rulez/`) to organize rules, context, and domain-specific guidance. Write once, generate for all your AI tools.

**Core capabilities:**
- **Directory-based configuration** – Organize rules, context, and domain content in a single `.ai-rulez/` directory
- **Domain separation** – Create domain-specific rules (backend, frontend, etc.) with profile-based generation
- **Profiles** – Group domains for different use cases, generate targeted configurations
- **Includes** – Compose rules from local packages or Git repositories with merge strategies
- **Multi-preset generation** – Generate native configs for Claude, Cursor, Windsurf, Copilot, Gemini, and more
- **MCP server support** – Configure MCP servers in your rules for IDE integration
- **AI enforcement** – Run `ai-rulez enforce` to validate code, catch violations, and apply fixes

<p align="center">
  <img src="docs/assets/ai-rulez-python-demo.gif" alt="ai-rulez configuration generation" width="90%">
</p>

---

## Quick Start

### 1. Initialize Your Project

```bash
# Create a new .ai-rulez/ directory with scaffold
ai-rulez init "My Project" --preset claude
```

Or without installation:

**npm:**
```bash
npx ai-rulez@latest init "My Project" --preset claude
```

**uv:**
```bash
uvx ai-rulez init "My Project" --preset claude
```

```
.ai-rulez/
  config.yaml          (configuration)
  rules/               (shared rules)
  context/             (shared context)
  domains/             (optional domain-specific rules)
```

### 2. Add Your Rules

**Option A: Using CRUD Commands (Programmatic)**

```bash
# Create a domain for backend rules
ai-rulez domain add backend

# Add a rule to the domain
ai-rulez add rule database-standards --domain backend --priority high

# Add context
ai-rulez add context architecture --domain backend

# Add a rule to root (always included)
ai-rulez add rule coding-standards --priority high
```

**Option B: Direct File Editing (Manual)**

```bash
# Edit .ai-rulez/rules/coding-standards.md
echo "# Coding Standards

Follow idiomatic patterns for your language..." > .ai-rulez/rules/coding-standards.md

# Add context about your project
echo "# Project Info

This is our SaaS platform..." > .ai-rulez/context/project-info.md
```

### 3. Generate Configuration

```bash
# Generate configs for all preset tools
ai-rulez generate
# Creates: CLAUDE.md, .cursorrules, .windsurfrules, etc.
```

### 4. Manage Configuration (CRUD Operations)

```bash
# Create profiles for different teams
ai-rulez profile add backend backend qa
ai-rulez profile add frontend frontend qa
ai-rulez profile set-default backend

# Add external rules from a git repository
ai-rulez include add corporate-rules https://github.com/myorg/shared-rules

# List your configuration
ai-rulez domain list
ai-rulez profile list
ai-rulez include list
```

### 5. Enforce Rules (Optional)

```bash
# Check for violations
ai-rulez enforce --agent claude

# Apply fixes
ai-rulez enforce --agent claude --fix
```

---

## V3 Directory Structure

```
.ai-rulez/
├── config.yaml                    # Core configuration
├── mcp.yaml                       # MCP servers (optional)
├── rules/                         # Shared rules
│   ├── coding-standards.md
│   └── performance.md
├── context/                       # Shared context
│   └── project-info.md
└── domains/                       # Domain-specific content
    ├── backend/
    │   └── rules/
    │       └── database.md
    └── frontend/
        └── rules/
            └── ui-patterns.md
```

---

## Configuration Examples

### Basic Setup

```yaml
# .ai-rulez/config.yaml
version: "3.0"
name: "My Project"
description: "AI governance rules"
presets:
  - claude
```

### With Domains and Profiles

```yaml
version: "3.0"
name: "My SaaS Platform"
description: "Full-stack governance"
presets:
  - claude
  - cursor

# Define profiles for different domains
profiles:
  backend:
    - backend
  frontend:
    - frontend
  full:
    - backend
    - frontend

default: backend
```

### With Local Includes

```yaml
version: "3.0"
name: "Main Project"
presets:
  - claude

# Include rules from local packages
includes:
  - name: backend-rules
    source: ./packages/backend
    include:
      - rules
      - context
    install_to: domains/backend

  - name: shared-rules
    source: ./shared
    include:
      - rules
      - context
    merge_strategy: local-override
```

### With Git Imports

```yaml
version: "3.0"
name: "Multi-org Project"
presets:
  - claude

includes:
  - name: corporate-rules
    source: https://github.com/myorg/shared-rules
    ref: main
    path: .ai-rulez
    include:
      - rules
      - context
    merge_strategy: local-override
```

---

## Installation

### Quick Start (No Installation)

**npm:**
```bash
npx ai-rulez@latest init "My Project"
npx ai-rulez@latest generate
```

**uv:**
```bash
uvx ai-rulez init "My Project"
uvx ai-rulez generate
```

### Global Installation

**Homebrew (macOS/Linux):**
```bash
brew install goldziher/tap/ai-rulez
```

**Go:**
```bash
go install github.com/Goldziher/ai-rulez/cmd@latest
```

**npm (Node.js):**
```bash
npm install -g ai-rulez
```

**uv (Python):**
```bash
uv tool install ai-rulez
```

**pip (Python):**
```bash
pip install ai-rulez
```

---

## Common Commands

```bash
# Initialize a new project
ai-rulez init "Project Name" --preset claude

# Generate all configured outputs
ai-rulez generate

# Generate for a specific profile
ai-rulez generate --profile frontend

# Check for rule violations
ai-rulez enforce --agent claude

# Apply auto-fixes
ai-rulez enforce --agent claude --fix

# Run multi-agent reviews
ai-rulez enforce --agent claude --review --review-iterations 2

# Validate configuration
ai-rulez validate
```

---

## Features

### Directory-Based Organization
Store all configuration in a single, versionable `.ai-rulez/` directory alongside your code.

### Domain Separation
Create domain-specific rules (`domains/backend/`, `domains/frontend/`, etc.) for specialized guidance.

### Profile System
Define profiles to group domains for different use cases. Generate different configs based on the selected profile.

### Includes & Imports
- **Local includes** – Compose rules from sibling packages with `source: ./path`
- **Git imports** – Pull rules from remote repositories with `source: https://github.com/org/repo` and `ref: branch`
- **Merge strategies** – Control how included content combines with your local rules

### Multi-Preset Generation
Generate configs for Claude, Cursor, Windsurf, Copilot, Gemini, and more—all from one configuration.

### MCP Server Support
Optionally configure MCP servers in `mcp.yaml` for integration with supported IDEs and tools.

### AI-Powered Enforcement
Use `ai-rulez enforce` to validate your codebase against your rules using Claude, Gemini, or other AI agents.

---

## Governance & Automation

- **Git hooks** – Use pre-commit, lefthook, or husky to run `ai-rulez validate` and `ai-rulez generate` on commit
- **CI/CD** – Integrate into GitHub Actions, GitLab CI, or other pipelines to gate merges on rule violations
- **MCP integration** – Expose rules to Claude CLI, Cursor, and other MCP-aware tools for real-time guidance

---

## Learn More

- [Documentation](https://goldziher.github.io/ai-rulez/) – Full guides and reference
- [Configuration Guide](https://goldziher.github.io/ai-rulez/configuration/) – V3 config details, domains, profiles, includes
- [Enforcement Guide](https://goldziher.github.io/ai-rulez/enforcement/) – Rule validation and auto-fixing
- [CLI Reference](https://goldziher.github.io/ai-rulez/cli/) – All commands and flags
- [GitHub](https://github.com/Goldziher/ai-rulez) – Issues, discussions, and contributions

---

## Contributing

We welcome contributions! Read the [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, coding standards, and release process details.
