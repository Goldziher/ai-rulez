# AI-Rulez Documentation

<p align="center">
  <img src="assets/ai-rulez-banner.svg" alt="AI-Rulez" width="820" />
</p>

AI-Rulez is a standards-compliant lifecycle tool for agent knowledge and capabilities. Keep your rules, context, skills,
agents, commands, hooks, permissions and MCP servers in one source of truth, `.ai-rulez/`, and take them through every
stage:

```text
author  ->  generate  ->  bundle  ->  lint / validate  ->  govern  ->  publish
```

The claim is "standards compliant": each format ai-rulez names is checked against that standard's own schema or rules.
[Standards](standards.md) lists the pinned spec versions, the conformance tests and the known gaps.

## The lifecycle

| Section | What you do | Start with |
| ------- | ----------- | ---------- |
| **Author** | Write rules, context, skills, agents and checks as markdown; organize them with domains, profiles, roles and includes. `.ai-rulez/` is an [OKF](okf.md) bundle: OKF is the internal format, and `init`, `add` and the MCP tools write it. A pre-OKF tree still loads, with a deprecation notice, until v6; `ai-rulez migrate okf` converts it. | [Configuration](configuration.md), [Rules](rules.md), [Skill frontmatter](skills.md) |
| **Generate** | Render native files for 52 harnesses, per project or per user, with hooks and permissions translated for each. | [Supported harnesses](harnesses.md), [AGENTS.md](agents-md.md), [User-level configuration](user-scope.md) |
| **Bundle and publish** | Package plugin bundles, Agent Plugins and ARD manifests, then release them to GitHub, npm, OCI and marketplaces. | [Authoring plugins](plugins.md), [Agent Plugins](agent-plugins.md), [ARD](ard.md), [Publish](publish.md) |
| **Validate** | Content and security checks with stable `AR` codes, deterministic verifiers and the per-standard validators. | [Strict validation](strict-validation.md), [Verifiers](verifiers.md) |
| **Govern** | Pin content in a lock, record reviewer approvals, sign with Sigstore, set an organization policy and ship an SBOM. | [Trust model](trust-model.md), [Lock file](lockfile.md), [Signing](signing.md), [SBOM](sbom.md) |
| **Operate** | Serve the configuration and skills over MCP, export telemetry, run evals and improve skills. | [MCP server](mcp-server.md), [Telemetry](telemetry.md), [Evals](evals.md) |

## Get started

1. [Install](installation.md) ai-rulez (`npx ai-rulez@latest` needs no install).
2. Follow the [Quick Start](quick-start.md): `ai-rulez init`, pick presets, `ai-rulez generate`.
3. Run `ai-rulez validate --strict`, then commit `.ai-rulez/` together with the generated files.

```bash
ai-rulez init "my-project"
ai-rulez generate
ai-rulez validate --strict
```

Coming from 4.x? Run `ai-rulez migrate v5 --dry-run` and read [Migrating to v5](migration-v5.md).

## Project layout

```text
project-root/
├── .ai-rulez/
│   ├── config.toml           # Presets, profiles, lifecycle settings
│   ├── rules/                # Mandatory constraints
│   ├── context/              # Reference documentation
│   ├── skills/               # Skills (SKILL.md)
│   ├── agents/, commands/    # Subagents and slash commands
│   ├── checks/               # Code-review guidelines
│   └── domains/              # Team or subsystem content
├── CLAUDE.md                 # Generated for Claude (rules go to .claude/rules/)
├── .cursor/rules/            # Generated for Cursor
├── GEMINI.md                 # Generated for Gemini
└── .github/copilot-instructions.md
```

Generated files are committed by default. Set `gitignore = true` to keep them out of git through a managed
`.gitignore` block instead.

## Reference

- [CLI commands](cli.md): every command and flag; `ai-rulez --help` and `ai-rulez <command> --help` are authoritative.
- [Schema](schema.md): the JSON schemas of the configuration and of every `--format json` document.
- [Embedding (Go API)](embedding.md): use ai-rulez as a library.
- [Changelog](CHANGELOG.md).

## Getting help

- **CLI help**: `ai-rulez --help`, `ai-rulez generate --help`, and so on.
- **Rule explanations**: `ai-rulez validate --explain AR001`.
- **Diagnostics**: `ai-rulez doctor` reports drift, removed presets and missing tools without changing anything.
- **Issues**: report problems on [GitHub](https://github.com/Goldziher/ai-rulez/issues).

This documentation covers **ai-rulez v5** (`version = "5.0"` in `config.toml`).
