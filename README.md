# ai-rulez

<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/logo.png" alt="ai-rulez logo" width="200" style="border-radius: 15%; overflow: hidden;">
</p>

**Modern, directory-based AI governance. Structure your rules. Control your assistants.**

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8)](https://go.dev)
[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-orange)](https://github.com/Goldziher/homebrew-tap)

**Documentation:** [goldziher.github.io/ai-rulez](https://goldziher.github.io/ai-rulez/)

---

## Overview

ai-rulez V3 uses a directory-based configuration structure (`.ai-rulez/`) to organize your AI governance rules, context, and domain-specific guidance. Configure once, generate for all your tools.

- **Directory-based config** – Store rules and context in organized, versionable `.ai-rulez/` directories
- **Domain organization** – Separate rules by domain (backend, frontend, etc.) with profile-based generation
- **Profile system** – Use profiles to generate different configs for different domains or use cases
- **Include/Import system** – Compose rules from local packages or git repositories with merge strategies
- **Multi-preset support** – Generate configs for Claude, Cursor, Windsurf, Copilot, Gemini, and more in one command
- **MCP server integration** – Native support for MCP servers in generated configs
- **AI-powered enforcement** – Run `ai-rulez enforce` to validate rules with AI agents, apply fixes, and gate merges

<p align="center">
  <img src="docs/assets/ai-rulez-python-demo.gif" alt="AI-Rulez Configuration Demo" width="90%">
</p>

---

## Quick Start

### 1. Initialize Your Project

```bash
# Create a new .ai-rulez/ directory with scaffold
npx ai-rulez@latest init "My Project" --preset claude

# This creates:
# .ai-rulez/
#   config.yaml          (configuration)
#   rules/               (shared rules)
#   context/             (shared context)
#   domains/             (optional domain-specific rules)
```

### 2. Add Your Rules

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
npx ai-rulez@latest generate
# Creates: CLAUDE.md, .cursorrules, .windsurfrules, etc.
```

### 4. Enforce Rules (Optional)

```bash
# Check for violations
npx ai-rulez@latest enforce --agent claude

# Apply fixes
npx ai-rulez@latest enforce --agent claude --fix
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

## Example V3 Configuration

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

## Key Features

### Directory-Based Organization
Store all configuration in a single, versionable `.ai-rulez/` directory alongside your code.

### Domain Separation
Create domain-specific rules (`domains/backend/`, `domains/frontend/`, etc.) for specialized guidance without mixing concerns.

### Profile System
Define profiles to group domains for different use cases. Generate different configs based on the selected profile.

### Include & Import
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

## Installation

### Quick Start (No Installation Required)

```bash
npx ai-rulez@latest init "My Project"
npx ai-rulez@latest generate
npx ai-rulez@latest enforce --agent claude
```

### Global Installation

**Homebrew:**
```bash
brew install goldziher/tap/ai-rulez
```

**npm:**
```bash
npm install -g ai-rulez
```

**Python:**
```bash
pip install ai-rulez
```

**Go:**
```bash
go install github.com/Goldziher/ai-rulez/cmd@latest
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

## Governance & Automation

- **Git hooks** – Use pre-commit, lefthook, or husky to run `ai-rulez validate` and `ai-rulez generate` on commit.
- **CI/CD** – Integrate into GitHub Actions, GitLab CI, or other pipelines to gate merges on rule violations.
- **MCP integration** – Expose rules to Claude CLI, Cursor, and other MCP-aware tools for real-time guidance.

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
