<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/ai-rulez-banner.png" alt="AI-Rulez" width="820" />
</p>

# ai-rulez

The standards-compliant lifecycle tool for agent knowledge and capabilities.

[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![PyPI downloads](https://img.shields.io/pypi/dm/ai-rulez)](https://pypi.org/project/ai-rulez/)

**Documentation:** [goldziher.github.io/ai-rulez](https://goldziher.github.io/ai-rulez/)

---

## What is ai-rulez?

ai-rulez keeps your rules, context, skills, agents and MCP servers in one `.ai-rulez/` source of truth and takes them through the lifecycle: author, generate native configs for 52 harnesses, bundle (OKF, Agent Plugins, ARD), lint and validate against each standard, govern (lock, approvals, Sigstore signing, policy, SBOM) and publish. Standards and their status: [docs/standards](https://goldziher.github.io/ai-rulez/standards/).

**Key features:**

- **Directory-based** – One `.ai-rulez/` directory for all your AI tooling
- **Multi-tool generation** – Generate configs for all major AI assistants from one source
- **Plugin publishing** – `generate --plugin` packages the same source into distributable plugin bundles and runtime-specific marketplace indexes (Claude, Cursor, Codex, Gemini, Kimi, OpenCode, Factory, Hermes; opt-in Pi, Copilot, and Agent Plugins)
- **Pi packages** – Opt in with `[plugin] runtimes = ["pi"]` to bundle skills and prompt templates for Git or npm installation; npm publishing preserves runtime metadata and uses `<configured-scope>/<plugin-name>`
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
uvx ai-rulez init "My Project"

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
- **Go** – build from source: `git clone https://github.com/Goldziher/ai-rulez && cd ai-rulez && go build -o ai-rulez ./cmd/ai-rulez`
- **npm** (Node.js) – `npm install -g ai-rulez`
