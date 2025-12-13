# ai-rulez ⚡

<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/logo.png" alt="ai-rulez logo" width="200" style="border-radius: 15%; overflow: hidden;">
</p>

**AI-powered development governance. One config to rule them all.**

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8)](https://go.dev)
[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![PyPI Version](https://img.shields.io/pypi/v/ai-rulez)](https://pypi.org/project/ai-rulez/)
[![Homebrew](https://img.shields.io/badge/Homebrew-tap-orange)](https://github.com/Goldziher/homebrew-tap)

**Documentation:** [goldziher.github.io/ai-rulez](https://goldziher.github.io/ai-rulez/)

---

## Why ai-rulez

- One YAML file stays in sync with every AI assistant format (`CLAUDE.md`, `.cursorrules`, `.windsurfrules`, MCP configs, and more).
- AI agents enforce your rules in real time with optional auto-fix and review loops.
- Git hooks, CI, and MCP integrations keep governance in every stage of your workflow.
- Works for individuals and teams with presets, remote includes, and monorepo support.

<p align="center">
  <img src="docs/assets/ai-rulez-python-demo.gif" alt="AI-Rulez Configuration Demo" width="90%">
</p>

---

## What You Get

- **Universal configuration management** – `ai-rulez` analyzes your repo and generates native configuration files for every assistant.
- **AI-powered enforcement** – Run `ai-rulez enforce` to catch violations, apply fixes, and gate merges with configurable quality thresholds.
- **Automation built-in** – Smart `.gitignore` updates, Git hook integration (lefthook, pre-commit, husky), and CI ready commands.
- **MCP everywhere** – Automatically wire MCP servers into Claude CLI, Gemini CLI, Cursor, and other supported tools.

---

## Quick Start

### Configuration Management

```bash
# Analyze your project and scaffold configuration
npx ai-rulez@latest init "My Project" --preset popular

# Generate all assistant configuration files
npx ai-rulez@latest generate
```

### Rule Enforcement

```bash
# Check for violations (read-only)
uvx ai-rulez@latest enforce --agent claude

# Apply fixes or run multi-agent reviews
uvx ai-rulez@latest enforce --agent claude --fix
uvx ai-rulez@latest enforce --agent gemini --review --review-iterations 2
```

### Optional: Auto-Configure Git Hooks

```bash
# Detect pre-commit, lefthook, or husky and add ai-rulez validation
npx ai-rulez@latest init --setup-hooks
```

`--setup-hooks` adds the official hooks for the detected system and keeps existing configuration intact.

---

## Example `ai-rulez.yaml`

```yaml
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "My SaaS Platform"

presets:
  - "popular"  # Claude, Cursor, Windsurf, Copilot, Gemini

rules:
  - name: "Go Code Standards"
    priority: high
    content: "Follow standard Go project layout and export only what is necessary."

sections:
  - name: "Project Structure"
    priority: critical
    content: |
      - `cmd/`: Main application entry point
      - `internal/`: Private application code
      - `pkg/`: Public-facing libraries

agents:
  - name: "go-developer"
    description: "Expert Go developer focused on idiomatic code and tests."

mcp_servers:
  - name: "ai-rulez"
    command: "npx"
    args: ["-y", "ai-rulez@latest", "mcp"]
    description: "Configuration management server"
```

---

## Installation Options

- **No install required**  
  - `npx ai-rulez@latest ...` for Node/JS environments  
  - `uvx ai-rulez@latest ...` for Python users  
  - `go run github.com/Goldziher/ai-rulez/cmd@latest ...` for Go projects
- **Global installs**  
  - `brew install goldziher/tap/ai-rulez`  
  - `npm install -g ai-rulez`  
  - `pip install ai-rulez`  
  - `go install github.com/Goldziher/ai-rulez/cmd@latest`

---

## Git Hooks & Automation

- **Official pre-commit hooks**

  ```yaml
  repos:
    - repo: https://github.com/Goldziher/ai-rulez
      rev: v2.4.3
      hooks:
        - id: ai-rulez-validate
        - id: ai-rulez-generate
  ```

- **CLI hook setup** – Run `ai-rulez init --setup-hooks` to auto-configure pre-commit, lefthook, or husky if they are present in your repo.
- **No-Go requirement** – Hooks use `scripts/pre-commit/run-ai-rulez.sh`, which fetches the correct prebuilt binary on-demand (override with `AI_RULEZ_BINARY` if you already have one installed).
- **CI/CD examples** – See the [Enforcement guide](https://goldziher.github.io/ai-rulez/enforcement/) for GitHub Actions, pipeline tips, and advanced workflows.

---

## More to Explore

- [Quick Start](https://goldziher.github.io/ai-rulez/quick-start/) – Guided setup in minutes.
- [CLI Reference](https://goldziher.github.io/ai-rulez/cli/) – All commands and flags.
- [Configuration Guide](https://goldziher.github.io/ai-rulez/configuration/) – Advanced YAML features, includes, and presets.
- [Enforcement Guide](https://goldziher.github.io/ai-rulez/enforcement/) – Multi-agent reviews, automation, and reporting.
- [Migration Guide](https://goldziher.github.io/ai-rulez/migration-guide/) – Upgrading from v1.x.

---

## Contributing

We welcome contributions! Read the [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, coding standards, and release process details.
