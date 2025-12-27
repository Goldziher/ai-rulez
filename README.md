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

## What is ai-rulez?

ai-rulez organizes your AI assistant rules, context, and domain-specific guidance in a single `.ai-rulez/` directory. Write once, generate native configurations for Claude, Cursor, Windsurf, Copilot, Gemini, and more.

**Key features:**
- **Directory-based** – One `.ai-rulez/` directory for all your AI tooling
- **Multi-tool generation** – Generate configs for all major AI assistants from one source
- **Domain separation** – Organize rules by backend, frontend, QA, or any domain
- **Profiles** – Define profiles for different teams or use cases
- **Includes** – Compose from local packages or Git repositories
- **CRUD operations** – Manage configuration programmatically via CLI or MCP

---

## Installation

### Quick Start (No Installation)

**npm:**
```bash
npx ai-rulez@latest init "My Project"
```

**uv:**
```bash
uvx ai-rulez init "My Project"
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

## Quick Example

```bash
# Initialize a new project
ai-rulez init "My Project" --preset claude

# Add a rule
ai-rulez add rule coding-standards --priority high

# Generate configs for all tools
ai-rulez generate
```

This creates `CLAUDE.md`, `.cursorrules`, and other native configs from your `.ai-rulez/` directory.

---

## Learn More

**[Full Documentation →](https://goldziher.github.io/ai-rulez/)**

---

## Contributing

We welcome contributions! Read [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.
