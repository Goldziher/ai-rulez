<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/ai-rulez-banner.png" alt="AI-Rulez" width="820" />
</p>

<h1 align="center">ai-rulez</h1>

<p align="center">
  <strong>The standards-compliant lifecycle tool for agent knowledge and capabilities: author, generate, bundle, validate, govern, publish</strong>
</p>

<p align="center">
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

## What it is

ai-rulez is a standards-compliant lifecycle tool for agent knowledge and capabilities. You keep rules, context, skills,
agents, commands, hooks, permissions and MCP servers in one source of truth, `.ai-rulez/`, and ai-rulez takes them
through the whole lifecycle:

```text
author  ->  generate  ->  bundle  ->  lint / validate  ->  govern  ->  publish
```

| Stage | What happens | Commands | Docs |
| ----- | ------------ | -------- | ---- |
| **Author** | Rules, context, skills, agents, commands and checks as markdown in `.ai-rulez/`, with domains, profiles, roles and includes. `.ai-rulez/` is an [OKF](docs/okf.md) bundle: OKF is the internal format, and `init`, `add` and the MCP tools write it. A pre-OKF tree still loads, with a deprecation notice, until v6; `migrate okf` converts it. | `init`, `add`, `convert`, `import`, `migrate okf` | [Author](docs/configuration.md) |
| **Generate** | Native files for 52 harnesses (Claude Code, Cursor, Codex, Copilot, Gemini CLI, OpenCode, Devin, Kilo and more), per project or per user. | `generate`, `doctor`, `clean` | [Harnesses](docs/harnesses.md) |
| **Bundle** | Distributable plugin bundles, an Agent Plugins package, an OKF bundle, an ARD manifest, `AGENTS.md` and `llms.txt`. | `generate --plugin`, `export okf` | [Plugins](docs/plugins.md) |
| **Lint and validate** | Content and security checks with stable `AR` codes, plus each standard's own schema or rules. | `validate`, `scan`, `okf validate`, `verify` | [Validate](docs/strict-validation.md) |
| **Govern** | A content-pinning lock, reviewer approvals, Sigstore signing, an organization policy and an SBOM. | `lock`, `approve`, `sign`, `verify --attestation`, `sbom` | [Trust model](docs/trust-model.md) |
| **Publish** | Reproducible, checksummed release artifacts to GitHub releases, npm, OCI and marketplaces. | `publish` | [Publish](docs/publish.md) |

## Standards

The claim is "standards compliant", so each format ai-rulez names is generated or bundled from the same sources and
checked against that standard's own schema or rules. A standard is marked **supported** when the code generates it and
validates it today, and **planned** when it does not yet.
[docs/standards.md](docs/standards.md) carries the pinned spec version, the conformance test suite and the known gaps for
each row, and is the source of truth for test status.

| Standard | Status | Generate / bundle | Lint / validate |
| -------- | ------ | ----------------- | --------------- |
| [OKF](docs/okf.md) (Open Knowledge Format, v0.2), the internal format of `.ai-rulez/` | supported | `export okf`, the `okf` preset | `okf validate` |
| [Agent Plugins](docs/agent-plugins.md) (agent-plugins.org) | supported | `generate --plugin`, `publish --emit agent-plugins` | `validate --strict` (AR9O codes) |
| [ARD](docs/ard.md) (Agentic Resource Discovery, a proposal) | supported | `publish --emit ard` | `validate` (entry schema, URN grammar) |
| [Agent Skills](docs/skills.md) (agentskills.io) | supported | `generate` | `validate` (frontmatter checks; partial) |
| [AGENTS.md](docs/agents-md.md) | supported | `generate` | `validate` |
| [llms.txt](docs/llms-txt.md) | supported | the `llms-txt` preset | `validate` |
| MCP server config | supported | `generate` | `validate` (partial) |
| MCP server card | planned | not generated | none |
| [CycloneDX / SPDX SBOM](docs/sbom.md) | supported | `sbom` | `sbom --check` |
| [in-toto / DSSE / Sigstore](docs/signing.md) | supported | `sign` | `verify --attestation` |
| [OpenTelemetry (OTLP)](docs/telemetry.md) | supported | `telemetry export --to otlp` | not applicable |
| "Agent bundle" | planned | spec to be confirmed | spec to be confirmed |

## Quick start

```bash
npx ai-rulez@latest init                 # author: scaffold .ai-rulez/ with config.toml
```

```toml
# .ai-rulez/config.toml
presets = ["claude", "cursor", "codex", "copilot", "gemini"]   # any of the 52 harnesses
```

```bash
npx ai-rulez@latest generate             # generate: write native files for each preset
npx ai-rulez@latest validate --strict    # validate: content and security checks, warnings fail
npx ai-rulez@latest lock                 # govern: pin includes, skills and authored content in ai-rulez.lock
npx ai-rulez@latest export okf           # bundle: an OKF bundle of your knowledge in docs/okf
```

Day-to-day:

```bash
ai-rulez generate --watch                # regenerate on every change to .ai-rulez/
ai-rulez generate --check                # CI: exit 2 when committed outputs differ from the sources
ai-rulez doctor                          # read-only diagnostics: drift, removed presets, missing tools
ai-rulez generate --user                 # the same, for your home directory (~/.claude, ~/.codex, ...)
```

Prefer the project-level [`.config/` convention](https://github.com/pi0/config-dir)? ai-rulez also discovers
`.config/ai-rulez/`, and `ai-rulez init --config-dir .config/ai-rulez` scaffolds it.

## Bundle and publish

Bundling and publishing need a `[plugin]` block (`name`, `version` and `description`) in `config.toml`:

```bash
ai-rulez generate --plugin               # plugin bundles and a marketplace index from the [plugin] block
ai-rulez export okf                      # an OKF bundle of your rules, context and skills (docs/okf)
ai-rulez publish --emit agent-plugins --emit ard --dry-run
ai-rulez publish --to github-release --execute --yes
```

`publish` runs a preflight first (`validate --strict`, `lock --check`, `verify --plugin`, a secret scan and the policy
gates), writes a byte-reproducible tar.gz with `SHA256SUMS`, and uploads only when you pass `--execute --yes`. See
[Publish](docs/publish.md) and [Authoring plugins](docs/plugins.md).

## Validate

```bash
ai-rulez validate                        # config plus content checks, exit 2 on findings
ai-rulez validate --explain AR001        # what a rule checks and how to suppress it
ai-rulez validate --format sarif --output ai-rulez.sarif
ai-rulez okf validate docs/okf           # lint any OKF bundle, ai-rulez's or a third party's
ai-rulez verify                          # generated files still match their headers
```

## Govern

```bash
ai-rulez lock                            # pin remote includes, installed skills and authored content
ai-rulez approve --list                  # what still needs a reviewer's approval
ai-rulez sign --lock                     # a Sigstore bundle (DSSE over an in-toto statement) for the lock
ai-rulez verify --attestation            # check it offline against the [signing] policy
ai-rulez sbom -o ai-bom.cdx.json         # CycloneDX 1.6 (or --type spdx-json)
```

An organization [policy](docs/policy.md) sets tighten-only floors a repository cannot loosen. See the
[trust model](docs/trust-model.md).

## Operate

- **MCP server.** `ai-rulez mcp` lets an assistant manage the configuration; with `--serve-skills` it serves skills on
  demand. See [MCP server](docs/mcp-server.md).
- **Telemetry.** Item-load records with an optional OTLP export. See [telemetry](docs/telemetry.md).
- **Evals and improve.** `ai-rulez eval` scores skills; `improve` (experimental) optimizes them behind a held-out eval
  gate. See [evals](docs/evals.md) and [improve](docs/improve.md).
- **Cost and inventory.** `tokens`, `cost`, `catalog` and `search` report what the configuration contains and costs.

Existing project on 4.x? Run `ai-rulez migrate v5 --dry-run`, then see [Migrating to v5](docs/migration-v5.md).

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
<summary><strong>Go</strong></summary>

```bash
go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest
```

The module path needs `/v5`; without it `@latest` resolves to an old build.

</details>

<details>
<summary><strong>pre-commit hook</strong></summary>

Add to `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v5.0.0
    hooks:
      - id: ai-rulez-recursive # generate outputs across the repo
      - id: ai-rulez-validate # dry-run validation
```

Available hook ids: `ai-rulez-validate`, `ai-rulez-generate`,
`ai-rulez-recursive`, `ai-rulez-plugin-generate`, and
`ai-rulez-plugin-verify`. They trigger on root or nested `.ai-rulez/` changes.
</details>

<details>
<summary><strong>poly hook source</strong></summary>

Add ai-rulez as a managed source in your existing `poly.toml` and select the hooks your
repository needs. This requires Poly 0.14.0+; pin `revision` to the AI-Rulez release you want (the
hooks run that release):

```toml
[[hooks.sources]]
id = "ai-rulez"
git = "https://github.com/Goldziher/ai-rulez.git"
revision = "v5.0.0"
hooks = ["ai-rulez-recursive", "ai-rulez-plugin-verify"]
```

The source also provides `ai-rulez-validate`, `ai-rulez-generate`,
and `ai-rulez-plugin-generate`.
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
      run: ai-rulez generate --recursive --no-local
```

Hooks run on each developer's machine and would otherwise load that developer's gitignored
`config.local.toml` overlay and `.ai-rulez/local/` content. Pass `--no-local` (also accepted by `validate`
and `tokens`) where a hook should see only the shared configuration, as a teammate or CI does. If you do
not, `generate` still refuses to write local-derived changes into tracked shared files; never add
`--allow-local-drift` to a hook. See [Poly hooks guide](docs/poly-hooks.md#machine-local-configuration-in-hooks).

In a monorepo, `ai-rulez generate --recursive` and `ai-rulez validate --recursive` (`-r`) process every nested
`.ai-rulez/` root, report all failures, and exit non-zero if any root failed.

Or run `ai-rulez init --setup-hooks` while initializing a repo to wire hooks in automatically.
</details>

## Documentation

Full documentation at [goldziher.github.io/ai-rulez](https://goldziher.github.io/ai-rulez/).

## License

MIT
