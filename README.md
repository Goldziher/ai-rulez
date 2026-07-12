<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/ai-rulez-banner.png" alt="AI-Rulez" width="820" />
</p>

<h1 align="center">ai-rulez</h1>

<p align="center">
  <strong>A complete development workflow for AI coding tools</strong>
</p>

<p align="center">
  <a href="https://goreportcard.com/report/github.com/Goldziher/ai-rulez"><img src="https://goreportcard.com/badge/github.com/Goldziher/ai-rulez" alt="Go Report Card"></a>
  <a href="https://www.npmjs.com/package/ai-rulez"><img src="https://img.shields.io/npm/v/ai-rulez" alt="npm version"></a>
  <a href="https://pypi.org/project/ai-rulez/"><img src="https://img.shields.io/pypi/v/ai-rulez" alt="PyPI version"></a>
  <a href="https://github.com/Goldziher/ai-rulez/blob/main/LICENSE"><img src="https://img.shields.io/github/license/Goldziher/ai-rulez" alt="License"></a>
  <a href="https://goldziher.github.io/ai-rulez/"><img src="https://img.shields.io/badge/docs-ai--rulez-blue" alt="Documentation"></a>
</p>

<p align="center">
  <a href="https://goldziher.github.io/ai-rulez/"><strong>Documentation</strong></a> &middot;
  <a href="https://goldziher.github.io/ai-rulez/quick-start/"><strong>Quick Start</strong></a> &middot;
  <a href="https://goldziher.github.io/ai-rulez/examples/"><strong>Examples</strong></a>
</p>

---

## The Problem

Every AI coding tool wants its own config: Claude needs `CLAUDE.md`, Cursor wants `.cursor/rules/`, Copilot expects `.github/copilot-instructions.md`. Each has different formats, frontmatter, and directory conventions. If you use more than one tool, you're maintaining duplicate rules that inevitably drift apart.

## The Solution

Write your rules, context, skills, agents, and commands once in `.ai-rulez/`. Run `generate`. Get native configs for every tool you use.

```bash
npx ai-rulez@latest init && npx ai-rulez@latest generate
```

ai-rulez generates correct, tool-native output for **20 platforms**: Claude, Cursor, Windsurf, Copilot, Gemini, Cline, Continue.dev, Codex, OpenCode, Hermes, Amp, Junie, Antigravity, and more. Each preset respects the target tool's conventions — proper frontmatter, directory structure, file extensions, agent formats.

## Generate Plugins, Not Just Config

ai-rulez doesn't only write config into *your* repo — it also packages your project as **distributable plugins**. Run `ai-rulez generate --plugin` and the same `.ai-rulez/` source (skills, commands, agents, MCP servers) becomes installable **plugin bundles and a marketplace index** for Claude, Cursor, Codex, Gemini, Kimi, OpenCode, Factory, and Hermes Agent.

```bash
ai-rulez generate --plugin           # write plugin bundles + marketplace.json
ai-rulez generate --plugin --dry-run # preview
ai-rulez verify --plugin             # prove committed output matches its sources
```

Write MCP launch commands and hooks once with the canonical `${PLUGIN_ROOT}` variable; each runtime gets its own manifest with the variable and hook format rewritten to fit. Hermes generation emits both a project plugin and a buildable Python entry-point package. Use `plugin.content_root` to keep distributable skills separate from contributor governance. Supports single-plugin repos and monorepos (`[marketplace].members`), plus a Claude statusline passthrough. See [Authoring Plugins](docs/plugins.md).

## What Ships Out of the Box

ai-rulez isn't just a config generator. It ships with **33 builtin domains** containing opinionated rules, agents, and workflows that establish a professional development baseline immediately.

### Builtin Rules (auto-included)

These activate automatically. No configuration needed.

| Domain | What it enforces |
|--------|-----------------|
| **ai-governance** | No AI signatures in commits. Concise communication. Systematic debugging. Verification before claiming success. Critical review of subagent output. |
| **code-quality** | Anti-patterns prevention. Complexity limits. Dead code removal. Error handling standards. Readability. |
| **testing** | TDD workflow (red-green-refactor, no exceptions). Testing anti-patterns. Meaningful assertions. Test independence. |
| **git-workflow** | Atomic commits. Conventional commit messages. Safe operations. Branch hygiene. |
| **security** | Secrets handling. Input validation. Dependency auditing. Least privilege. |
| **token-efficiency** | Task runner usage. Incremental approach. Context preservation. Batch operations. |
| **agent-delegation** | Multi-agent coordination and delegation patterns. |

### Builtin Agents

Specialized agents ready to use as subagents:

| Agent | Domain | Model | What it does |
|-------|--------|-------|-------------|
| **code-reviewer** | ai-governance | sonnet | Reviews changes for correctness, security, and conventions. Reports by severity. |
| **test-writer** | testing | sonnet | Writes tests following strict TDD. Fails first, then implements. |
| **security-auditor** | security | sonnet | Audits dependencies, scans for CVEs, reviews input validation. |
| **docs-writer** | ai-governance | haiku | Writes clear, concise documentation. No fluff. |
| **devops-engineer** | cicd | haiku | CI/CD pipelines, GitHub Actions, Docker, deployment automation. |
| **release-engineer** | cicd | haiku | Version management, changelogs, multi-registry publishing. |

### Opt-in Domains

Enable these based on your stack:

**Languages** (10): `rust`, `python`, `typescript`, `go`, `java`, `ruby`, `php`, `elixir`, `csharp`, `r`

**Bindings** (10): `pyo3`, `napi-rs`, `magnus`, `ext-php-rs`, `rustler`, `wasm`, `jni-rs`, `extendr`, `cgo`, `vite-plus`

**Operational**: `cicd`, `docker`, `observability`, `documentation`, `polyglot-bindings`, `default-commands`

```toml
# .ai-rulez/config.toml
builtins = ["rust", "python", "pyo3", "cicd", "docker", "default-commands"]
```

## Content Types

| Type | Purpose | Example |
|------|---------|---------|
| **Rules** | What AI must/must not do | Security standards, coding conventions |
| **Context** | What AI should know | Architecture docs, domain knowledge |
| **Skills** | Reusable prompts and workflows | Deployment checklist, review protocol |
| **Agents** | Specialized AI personas | Code reviewer, performance engineer |
| **Commands** | Slash commands across tools | `/review`, `/deploy`, `/test` |

## Organization at Scale

ai-rulez scales from solo projects to large organizations:

**Domains** — Group content by feature, language, or team:

```text
.ai-rulez/domains/backend/rules/
.ai-rulez/domains/frontend/rules/
```

**Profiles** — Generate different configs for different audiences:

```toml
[profiles]
backend = ["backend", "database"]
frontend = ["frontend", "ui"]
```

**Remote Includes** — Share rules across repositories:

```toml
[[includes]]
name = "company-standards"
source = "https://github.com/company/ai-rules.git"
merge_strategy = "local-override"
```

**Reasoning effort across providers** — Tune how hard each AI tool thinks:

```yaml
# .ai-rulez/agents/security-reviewer.md
---
name: security-reviewer
description: Reviews code for security regressions
effort: high
---
```

```toml
# .ai-rulez/config.toml
[defaults]
effort = "medium"  # global default for every supported preset

[defaults.effort_by_preset]
codex = "high"     # overrides the global default for Codex
claude = "xhigh"   # …and for Claude
```

Accepted values: `low`, `medium`, `high`, `xhigh`, `max`, `inherit`. ai-rulez emits the right field per preset:

- **Claude** — `effort` in `.claude/agents/*.md` frontmatter (per-agent)
- **Codex** — `model_reasoning_effort` in `.codex/config.toml` and `.codex/agents/*.toml`
- **Amp** — `amp.anthropic.effort` in `.amp/settings.json` (global)
- **Windsurf** — `reasoning_effort` in `.windsurf/agents/*.md` frontmatter (per-agent)
- **Opencode** — `reasoningEffort` in `.opencode/agents/*.md` frontmatter (per-agent)

Each preset maps the value to its own vocabulary; tools without a documented config surface (Cursor, Copilot, Gemini, etc.) are silently skipped. See [docs/configuration.md](docs/configuration.md#defaults) for the full mapping table.

**Per-preset model selection for subagents** — Model strings differ per provider, so the same agent can declare a different model for each preset it targets:

```yaml
# .ai-rulez/agents/research-helper.md
---
name: research-helper
description: Multi-provider research subagent
claude_model: opus
copilot_model: gpt-5
cursor_model: claude-3.7-sonnet
---
```

```toml
# .ai-rulez/config.toml — project-wide defaults
[defaults.model_by_preset]
claude = "sonnet"   # used when an agent doesn't set its own claude_model
copilot = "gpt-5"
```

Per-agent `<preset>_model` wins over `defaults.model_by_preset`; the legacy single `model:` field on an agent is the lowest-priority fallback for backward compatibility.

**Installed Skills** — Pull reusable skills from external repos:

```toml
[[installed_skills]]
name = "kreuzberg"
source = "https://github.com/kreuzberg-dev/kreuzberg"
```

## MCP Server

ai-rulez includes a built-in MCP server with 35+ tools that lets AI assistants manage their own governance. Add rules, update context, generate configs — all programmatically.

```toml
[[mcp_servers]]
name = "ai-rulez"
command = "npx"
args = ["-y", "ai-rulez@latest", "mcp"]
```

## Installation

No install needed — `npx ai-rulez@latest <command>` works out of the box. Pick a permanent option below:

<details>
<summary><strong>Homebrew (macOS / Linux)</strong></summary>

```bash
brew install goldziher/tap/ai-rulez
```

</details>

<details>
<summary><strong>npx (no install)</strong></summary>

```bash
npx ai-rulez@latest <command>
```

</details>

<details>
<summary><strong>npm (global)</strong></summary>

```bash
npm install -g ai-rulez
```

</details>

<details>
<summary><strong>uvx (no install)</strong></summary>

```bash
uvx ai-rulez <command>
```

</details>

<details>
<summary><strong>uv tool</strong></summary>

```bash
uv tool install ai-rulez
```

</details>

<details>
<summary><strong>pip / pipx</strong></summary>

```bash
pip install ai-rulez
# or, isolated:
pipx install ai-rulez
```

</details>

<details>
<summary><strong>pre-commit hook</strong></summary>

Add to `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v4.9.3
    hooks:
      - id: ai-rulez-recursive   # generate outputs across the repo
      - id: ai-rulez-validate    # dry-run validation
```

Available hook ids: `ai-rulez-validate`, `ai-rulez-generate`,
`ai-rulez-recursive`, `ai-rulez-plugin-generate`,
`ai-rulez-plugin-verify`, `ai-rulez-enforce`, and
`ai-rulez-enforce-fix`. They trigger on root or nested `.ai-rulez/` changes.
</details>

<details>
<summary><strong>poly hook source</strong></summary>

Add ai-rulez as a managed source in your existing `poly.toml` and select the hooks your
repository needs. This requires AI-Rulez 4.9.0+ and Poly 0.14.0+:

```toml
[[hooks.sources]]
id = "ai-rulez"
git = "https://github.com/Goldziher/ai-rulez.git"
revision = "v4.9.3"
hooks = ["ai-rulez-recursive", "ai-rulez-plugin-verify"]
```

The source also provides `ai-rulez-validate`, `ai-rulez-generate`,
`ai-rulez-enforce`, `ai-rulez-enforce-fix`, and `ai-rulez-plugin-generate`.
Plugin hooks use `--if-configured`, so they skip consumer-only repositories that
do not contain a producer `[plugin]` or multi-member `[marketplace]` block.

Resolve and commit the source lock, then install the Git shims:

```bash
poly hooks update
git add poly.toml poly-hooks.lock
poly hooks install
```

See the [Poly hooks guide](docs/poly-hooks.md) for local sources, machine install
preferences, hook behavior, and the producer catalog.
</details>

<details>
<summary><strong>lefthook</strong></summary>

Add to `lefthook.yml`:

```yaml
pre-commit:
  commands:
    ai-rulez:
      glob: ".ai-rulez/**"
      run: ai-rulez generate --recursive
```

Or run `ai-rulez init --setup-hooks` while initializing a repo to wire hooks in automatically.
</details>

## Documentation

Full documentation at [goldziher.github.io/ai-rulez](https://goldziher.github.io/ai-rulez/).

## License

MIT
