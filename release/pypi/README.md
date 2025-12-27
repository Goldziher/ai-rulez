# ai-rulez

Directory-based AI governance for development teams.

[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![PyPI downloads](https://img.shields.io/pypi/dm/ai-rulez)](https://pypi.org/project/ai-rulez/)

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

**uv:**
```bash
uv tool install ai-rulez
```

**pip:**
```bash
pip install ai-rulez
```

**Or use without installation:**
```bash
uvx ai-rulez init "My Project"
```

---

## Quick Example

```bash
# Initialize a new project
uvx ai-rulez init "My Project" --preset claude

# Add a rule
uvx ai-rulez add rule coding-standards --priority high

# Generate configs for all tools
uvx ai-rulez generate
```

This creates `CLAUDE.md`, `.cursorrules`, and other native configs from your `.ai-rulez/` directory.

---

## Learn More

**[Full Documentation →](https://goldziher.github.io/ai-rulez/)**

---

## Other Platforms

- **Homebrew** (macOS/Linux) – `brew install goldziher/tap/ai-rulez`
- **Go** – `go install github.com/Goldziher/ai-rulez/cmd@latest`
- **npm** (Node.js) – `npm install -g ai-rulez`
