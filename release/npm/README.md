<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/ai-rulez-banner.png" alt="AI-Rulez" width="820" />
</p>

# ai-rulez

The standards-compliant lifecycle tool for agent knowledge and capabilities.

[![NPM Version](https://img.shields.io/npm/v/ai-rulez)](https://www.npmjs.com/package/ai-rulez)
[![npm downloads](https://img.shields.io/npm/dm/ai-rulez)](https://www.npmjs.com/package/ai-rulez)

**Documentation:** [goldziher.github.io/ai-rulez](https://goldziher.github.io/ai-rulez/)

---

## What is ai-rulez?

ai-rulez keeps your rules, context, skills, agents and MCP servers in one `.ai-rulez/` source of truth and takes them through the lifecycle: author, generate native configs for 52 harnesses, bundle (OKF, Agent Plugins, ARD), lint and validate against each standard, govern (lock, approvals, Sigstore signing, policy, SBOM) and publish. Standards and their status: [docs/standards](https://goldziher.github.io/ai-rulez/standards/).

**Key features:**

- **Directory-based** – One `.ai-rulez/` directory for all your AI tooling
- **Standards as the source** – Or keep `AGENTS.md` and `.agents/` as the source (`source = "agents"`); ai-rulez translates them to every other tool and never writes into them
- **Multi-tool generation** – Generate configs for all major AI assistants from one source
- **Plugin publishing** – `generate --plugin` packages the same source into distributable plugin bundles and runtime-specific marketplace indexes (Claude, Cursor, Codex, Gemini, Kimi, OpenCode, Factory, Hermes; opt-in Pi, Copilot, and Agent Plugins)
- **Pi packages** – Opt in with `[plugin] runtimes = ["pi"]` to bundle skills and prompt templates for Git or npm installation; npm publishing preserves runtime metadata and uses `<configured-scope>/<plugin-name>`
- **Domain separation** – Organize rules by backend, frontend, QA, or any domain
- **Profiles** – Define profiles for different teams or use cases
- **Includes** – Compose from local packages or Git repositories
- **CRUD operations** – Manage configuration programmatically via CLI or MCP
- **Lint without a config** – `ai-rulez validate --strict` checks `AGENTS.md`, `SKILL.md`, `.claude/`, `.agents/`, plugin manifests, hooks and MCP files in any repository; `--path`, `--glob` and `--outputs` widen the scope
- **Judge** – `ai-rulez judge` reviews agent configuration for security, quality and governance with verified evidence and optional rubric skills; no project config is required. `--estimate` sends nothing; a real run needs `--allow-llm` or standing consent in your user config.
- **GitHub Action** – `uses: Goldziher/ai-rulez@v5` installs a sha256-verified binary in CI and accepts optional LLM configuration inputs. The floating `v5` action ref advances after stable v5 publication; set `version` separately to pin the CLI. `Goldziher/ai-rulez/check@v5` runs the full governance gate (validate, lock, generated-output drift, clean checkout); `Goldziher/ai-rulez/update@v5` opens a scheduled pin-update pull request. [Action guide](https://goldziher.github.io/ai-rulez/github-actions/).

---

## Installation

**npm:**

```bash
pnpm add -g ai-rulez
```

**Or use without installation:**

```bash
pnpm dlx ai-rulez@latest init "My Project"
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

If the Windows executable needs `liter_llm_ffi.dll`, include it as `bin/liter_llm_ffi.dll`;
the installer places it beside the executable.

---

## Quick Example

```bash
# Initialize a new project
pnpm dlx ai-rulez@latest init "My Project"

# Add a rule
pnpm dlx ai-rulez@latest add rule coding-standards --priority high

# Generate configs for the selected tools
pnpm dlx ai-rulez@latest generate
```

This generates native configuration for the presets selected during `init`. Shared instructions use `AGENTS.md`; tool-specific content uses paths such as `.cursor/rules/`.

---

## Learn More

**[Full Documentation →](https://goldziher.github.io/ai-rulez/)** · [Complete feature table](https://github.com/Goldziher/ai-rulez#features)

---

## Other Platforms

- **Homebrew** (macOS/Linux) – `brew install goldziher/tap/ai-rulez`
- **Go** – clone the repository and run `task build` for a statically linked executable; see [source installation](https://goldziher.github.io/ai-rulez/installation/).
- **uv** (Python) – `uv tool install ai-rulez`
- **pip** (Python) – `pip install ai-rulez`
