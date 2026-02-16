# ai-rulez

Directory-based AI governance for development teams.

[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![npm downloads](https://img.shields.io/npm/dm/ai-rulez)](https://www.npmjs.com/package/ai-rulez)

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

**npm:**
```bash
npm install -g ai-rulez
```

**Or use without installation:**
```bash
npx ai-rulez@latest init "My Project"
```

### Offline / private registry packaging

When publishing this package to a private registry, you can bundle platform
binaries in `bin/` so installs do not need GitHub release downloads.

Supported packaged binary names:
- `bin/ai-rulez-linux-amd64`
- `bin/ai-rulez-linux-arm64`
- `bin/ai-rulez-darwin-amd64`
- `bin/ai-rulez-darwin-arm64`
- `bin/ai-rulez-windows-amd64.exe` (or `bin/ai-rulez-windows-amd64`)

If a packaged binary is present, the installer uses it first and skips network
download.

---

## Quick Example

```bash
# Initialize a new project
npx ai-rulez@latest init "My Project" --preset claude

# Add a rule
npx ai-rulez@latest add rule coding-standards --priority high

# Generate configs for all tools
npx ai-rulez@latest generate
```

This creates `CLAUDE.md`, `.cursorrules`, and other native configs from your `.ai-rulez/` directory.

---

## Learn More

**[Full Documentation →](https://goldziher.github.io/ai-rulez/)**

---

## Other Platforms

- **Homebrew** (macOS/Linux) – `brew install goldziher/tap/ai-rulez`
- **Go** – `go install github.com/Goldziher/ai-rulez/cmd@latest`
- **uv** (Python) – `uv tool install ai-rulez`
- **pip** (Python) – `pip install ai-rulez`
