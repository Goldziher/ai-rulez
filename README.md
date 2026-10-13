<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/ai-rulez-banner.png" alt="AI-Rulez" width="820" />
</p>

<p align="center">
  <strong>The standards-compliant lifecycle tool for agentic governance</strong>
</p>

<p align="center">
  <a href="https://www.npmjs.com/package/ai-rulez"><img src="https://img.shields.io/npm/v/ai-rulez?color=blue" alt="npm version"></a>
  <a href="https://pypi.org/project/ai-rulez/"><img src="https://img.shields.io/pypi/v/ai-rulez?color=blue" alt="PyPI version"></a>
  <a href="https://github.com/Goldziher/homebrew-tap"><img src="https://img.shields.io/github/v/release/Goldziher/ai-rulez?label=homebrew&color=blue" alt="Homebrew version"></a>
  <a href="https://pkg.go.dev/github.com/Goldziher/ai-rulez/v5"><img src="https://img.shields.io/github/v/release/Goldziher/ai-rulez?label=go&color=blue" alt="Go version"></a>
  <a href="https://github.com/Goldziher/ai-rulez/blob/main/LICENSE"><img src="https://img.shields.io/github/license/Goldziher/ai-rulez?color=blue" alt="License"></a>
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

Most tools in this space stop at "turn my rules into markdown." ai-rulez treats agent knowledge like software you
ship: versioned, linted, locked, signed and audited, with an organization policy layer that enforces a floor no
repository can loosen.

- **One source, 52 targets.** Author once in `.ai-rulez/`; generate native files for every harness your team uses instead of hand-copying rules across tools.
- **A real lock file.** `ai-rulez.lock` pins remote includes, installed skills and authored content the way `go.sum` or `package-lock.json` pin dependencies.
- **Supply-chain-grade trust.** Sigstore signing, in-toto/DSSE attestations, CycloneDX/SPDX SBOMs and reviewer approvals, so "what prompt is actually running in production" has a verifiable answer.
- **Org-wide policy, not just repo config.** A central [policy](docs/policy.md) sets tighten-only floors; `verify` and CI gates enforce it everywhere.

| Stage | What happens | Commands | Docs |
| ----- | ------------ | -------- | ---- |
| **Author** | Rules, context, skills, agents, commands and checks as markdown in `.ai-rulez/`, with domains, profiles, roles and includes. `.ai-rulez/` is an [OKF](docs/okf.md) bundle: OKF is the internal format, and `init`, `add` and the MCP tools write it. A pre-OKF tree still loads, with a deprecation notice; `migrate okf` converts it. | `init`, `add`, `convert`, `import`, `migrate okf` | [Author](docs/configuration.md) |
| **Generate** | Native files for 52 harnesses (Claude Code, Cursor, Codex, Copilot, Gemini CLI, OpenCode, Devin, Kilo and more), per project or per user. | `generate`, `doctor`, `clean` | [Harnesses](docs/harnesses.md) |
| **Bundle** | Distributable plugin bundles, an Agent Plugins package, an OKF bundle, an ARD manifest, `AGENTS.md` and `llms.txt`. | `generate --plugin`, `export okf` | [Plugins](docs/plugins.md) |
| **Lint and validate** | Content and security checks with stable `AR` codes, plus each standard's own schema or rules. | `validate`, `scan`, `okf validate`, `verify` | [Validate](docs/strict-validation.md) |
| **Govern** | A content-pinning lock, reviewer approvals, Sigstore signing, an organization policy and an SBOM. | `lock`, `approve`, `sign`, `verify --attestation`, `sbom` | [Trust model](docs/trust-model.md) |
| **Publish** | Reproducible, checksummed release artifacts to GitHub releases, npm, OCI and marketplaces. | `publish` | [Publish](docs/publish.md) |

<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/quickstart.gif" alt="ai-rulez init and generate: one .ai-rulez source becomes the native files of four AI tools" width="820" />
</p>

## Supported tools

ai-rulez ships 52 built-in presets — one per AI coding assistant, CLI agent or IDE integration — each writing that
tool's own file layout, frontmatter and settings documents from the same `.ai-rulez/` source. Mix and match any
combination in `presets = [...]`; the full feature matrix (rules, skills, agents, commands, MCP, hooks, permissions,
checks, user scope) is in [Supported harnesses](docs/harnesses.md), and a provider that isn't listed yet still gets
full parity through a [custom provider-backed preset](docs/configuration.md#provider-backed-presets-full-parity).

<details>
<summary><strong>All 52 supported tools</strong></summary>

| Preset | Tool |
| ------ | ---- |
| `aiassistant` | [JetBrains AI Assistant](https://www.jetbrains.com/help/ai-assistant/) |
| `amp` | [Sourcegraph Amp](https://ampcode.com) |
| `antigravity` | [Google Antigravity](https://antigravity.google/docs) |
| `augment` | [Augment Code](https://docs.augmentcode.com) |
| `baz` | [Baz](https://baz.ai/docs/agents/skills-and-instructions) |
| `bob` | [IBM Bob](https://bob.ibm.com/docs) |
| `claude` | [Claude Code](https://code.claude.com/docs) |
| `cline` | [Cline](https://docs.cline.bot) |
| `codebuddy` | [CodeBuddy Code](https://www.codebuddy.ai/docs/cli) |
| `codebuff` | [Codebuff](https://www.codebuff.com/docs) |
| `codewhale` | [Codewhale](https://github.com/codewhale-hq/Codewhale) |
| `codex` | [OpenAI Codex](https://developers.openai.com/codex) |
| `commandcode` | [Command Code](https://commandcode.ai/docs) |
| `copilot` | [GitHub Copilot](https://docs.github.com/en/copilot) |
| `copilot-cli` | [GitHub Copilot CLI](https://docs.github.com/en/copilot/how-tos/copilot-cli) |
| `cortex` | [Snowflake Cortex Code](https://docs.snowflake.com/en/user-guide/cortex-code/extensibility) |
| `crush` | [Crush](https://github.com/charmbracelet/crush) |
| `cursor` | [Cursor](https://cursor.com/docs) |
| `deepagents` | [Deep Agents Code](https://docs.langchain.com/oss/deepagents/code) |
| `devin` | [Devin CLI](https://docs.devin.ai/cli) |
| `dsh` | [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) |
| `factory` | [Factory Droid](https://docs.factory.ai) |
| `gemini` | [Gemini CLI](https://geminicli.com/docs/) |
| `gitlab-duo` | [GitLab Duo CLI](https://docs.gitlab.com/user/gitlab_duo_cli/customize/) |
| `goose` | [goose](https://goose-docs.ai) |
| `grok` | [Grok Build CLI](https://docs.x.ai/build/overview) |
| `hermes` | [Hermes Agent](https://hermes-agent.nousresearch.com/docs) |
| `junie` | [JetBrains Junie](https://junie.jetbrains.com) |
| `kilo` | [Kilo Code](https://kilo.ai/docs) |
| `kimi` | [Kimi Code](https://moonshotai.github.io/kimi-code/) |
| `kiro` | [Kiro](https://kiro.dev/docs) |
| `letta` | [Letta Code](https://docs.letta.com) |
| `mimocode` | [MiMo Code](https://mimo.xiaomi.com/mimocode) |
| `muse` | [Meta Muse Code](https://dev.meta.ai/docs/muse-code) |
| `omp` | [oh-my-pi](https://github.com/can1357/oh-my-pi) |
| `openclaw` | [OpenClaw](https://docs.openclaw.ai) |
| `opencode` | [OpenCode](https://opencode.ai/docs) |
| `pi` | [pi](https://pi.dev/docs/latest) |
| `poolside` | [Poolside Pool](https://docs.poolside.ai) |
| `qoder` | [Qoder](https://docs.qoder.com/cli/quickstart) |
| `qwen` | [Qwen Code](https://qwenlm.github.io/qwen-code-docs/en/) |
| `reasonix` | [Reasonix](https://github.com/esengine/DeepSeek-Reasonix) |
| `replit` | [Replit Agent](https://docs.replit.com/features/agent/overview) |
| `rovodev` | [Atlassian Rovo Dev](https://www.atlassian.com/software/rovo-dev) |
| `takt` | [Takt](https://github.com/nrslib/takt) |
| `trae` | [Trae](https://docs.trae.ai/ide/rules) |
| `vibe` | [Mistral Vibe](https://docs.mistral.ai/vibe) |
| `warp` | [Warp](https://docs.warp.dev) |
| `xum` | [Xum](https://xum.coder.com) |
| `zcode` | [ZCode](https://zcode.z.ai/en/docs) |
| `zed` | [Zed](https://zed.dev/docs/ai) |
| `zoocode` | [Zoo Code](https://docs.zoocode.dev) |

</details>

## Standards

Generation isn't limited to harness files — ai-rulez aims to be compliant with the major emerging standards in the agentic ecosystem. A standard is marked **supported** when the code generates it and
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

## Usage

Every option below produces the same `.ai-rulez/` tree and the same generated output — pick however your project
already runs tools. Each one walks through init, generate and validate; [Bundle and publish](#bundle-and-publish),
[Validate](#validate) and [Govern](#govern) below cover what comes after.

```toml
# .ai-rulez/config.toml
presets = ["claude", "cursor", "codex", "copilot", "gemini"]   # any of the 52 harnesses
```

<details>
<summary><strong>npx (Node.js, no install)</strong></summary>

```bash
npx ai-rulez@latest init
npx ai-rulez@latest generate
npx ai-rulez@latest validate --strict
```

</details>

<details>
<summary><strong>uvx (Python, no install)</strong></summary>

```bash
uvx ai-rulez init
uvx ai-rulez generate
uvx ai-rulez validate --strict
```

</details>

<details>
<summary><strong>Homebrew (macOS / Linux)</strong></summary>

```bash
brew install goldziher/tap/ai-rulez
ai-rulez init
ai-rulez generate
ai-rulez validate --strict
```

</details>

<details>
<summary><strong>Go</strong></summary>

```bash
go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest
ai-rulez init
ai-rulez generate
ai-rulez validate --strict
```

The module path needs `/v5`; without it `@latest` resolves to an old build.

</details>

<details>
<summary><strong>npm (global)</strong></summary>

```bash
npm install -g ai-rulez
ai-rulez init
ai-rulez generate
ai-rulez validate --strict
```

</details>

<details>
<summary><strong>uv tool</strong></summary>

```bash
uv tool install ai-rulez
ai-rulez init
ai-rulez generate
ai-rulez validate --strict
```

</details>

<details>
<summary><strong>pip / pipx</strong></summary>

```bash
pip install ai-rulez
# or, isolated:
pipx install ai-rulez

ai-rulez init
ai-rulez generate
ai-rulez validate --strict
```

</details>

<details>
<summary><strong>Day-to-day commands</strong></summary>

```bash
ai-rulez generate --watch                # regenerate on every change to .ai-rulez/
ai-rulez generate --check                # CI: exit 2 when committed outputs differ from the sources
ai-rulez doctor                          # read-only diagnostics: drift, removed presets, missing tools
ai-rulez generate --user                 # the same, for your home directory (~/.claude, ~/.codex, ...)
```

</details>

Prefer the project-level [`.config/` convention](https://github.com/pi0/config-dir)? ai-rulez also discovers
`.config/ai-rulez/`, and `ai-rulez init --config-dir .config/ai-rulez` scaffolds it.

## Bundle and publish

Bundling and publishing need a `[plugin]` block (`name`, `version` and `description`) in `config.toml`:

<details>
<summary><strong>Show commands</strong></summary>

```bash
ai-rulez generate --plugin               # plugin bundles and a marketplace index from the [plugin] block
ai-rulez export okf                      # an OKF bundle of your rules, context and skills (docs/okf)
ai-rulez publish --emit agent-plugins --emit ard --dry-run
ai-rulez publish --to github-release --execute --yes
```

</details>

`publish` runs a preflight first (`validate --strict`, `lock --check`, `verify --plugin`, a secret scan and the policy
gates), writes a byte-reproducible tar.gz with `SHA256SUMS`, and uploads only when you pass `--execute --yes`. See
[Publish](docs/publish.md) and [Authoring plugins](docs/plugins.md).

Pi packages are opt-in with `[plugin] runtimes = ["pi"]`: skills and prompt templates can be installed
from Git or npm. Publishing to npm preserves Pi and OpenCode runtime metadata while using the
configured scope and plugin name for the released package.

`verify --plugin` reports a hand-edited or stray file in a bundle as drift, and `generate --plugin` restores it:

<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/plugin-drift.gif" alt="ai-rulez verify --plugin reporting changed and extra files in a plugin bundle, then generate --plugin restoring it" width="820" />
</p>

## Validate

<p align="center">
  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/validate.gif" alt="ai-rulez validate --strict flags a defect, --explain describes it, and generate --check fails on drift" width="820" />
</p>

<details>
<summary><strong>Show commands</strong></summary>

```bash
ai-rulez validate                        # config plus content checks, exit 2 on findings
ai-rulez validate --explain AR001        # what a rule checks and how to suppress it
ai-rulez validate --format sarif --output ai-rulez.sarif
ai-rulez okf validate docs/okf           # lint any OKF bundle, ai-rulez's or a third party's
ai-rulez generate --check                # generated files still match their sources
```

</details>

## Govern

<details>
<summary><strong>Show commands</strong></summary>

```bash
ai-rulez lock                            # pin remote includes, installed skills and authored content
ai-rulez approve --list                  # what still needs a reviewer's approval
ai-rulez sign --lock                     # a Sigstore bundle (DSSE over an in-toto statement) for the lock
ai-rulez verify --attestation            # check it offline against the [signing] policy
ai-rulez sbom -o ai-bom.cdx.json         # CycloneDX 1.6 (or --type spdx-json)
```

</details>

An organization [policy](docs/policy.md) sets tighten-only floors a repository cannot loosen. See the
[trust model](docs/trust-model.md).

## Operate

- **MCP server.** `ai-rulez mcp` lets an assistant manage the configuration; with `--serve-skills` one server serves
  skills on demand and the config tools together (`--read-only` keeps only the read-only tools).
  See [MCP server](docs/mcp-server.md).
  Served skills cost nothing until an assistant asks for them:

  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/skills-mcp.gif" alt="ai-rulez tokens before and after serving skills over MCP, then find_skill and load_skill" width="780" />

- **Monorepos.** Nested roots each carry their own `.ai-rulez/`; `generate --recursive` and
  `validate --recursive` walk them all, and `tokens --budget` gates what every session loads.
  See [Monorepos](docs/monorepo.md).

  <img src="https://raw.githubusercontent.com/Goldziher/ai-rulez/main/docs/assets/monorepo.gif" alt="ai-rulez generate --recursive over two nested roots, recursive validation, and a token budget check" width="780" />

- **Telemetry.** Item-load records with an optional OTLP export. See [telemetry](docs/telemetry.md).
- **Evals and improve.** `ai-rulez eval` scores skills; `improve` (experimental) optimizes them behind a held-out eval
  gate. See [evals](docs/evals.md) and [improve](docs/improve.md).
- **Cost and inventory.** `tokens`, `cost`, `catalog` and `search` report what the configuration contains and costs.

Existing project on 4.x? Run `ai-rulez migrate v5 --dry-run`, then see [Migrating to v5](docs/migration-v5.md).

## Documentation

Full documentation at [goldziher.github.io/ai-rulez](https://goldziher.github.io/ai-rulez/).

## Git Hooks

ai-rulez supports multiple pre-commit hook harnesses.

<details>
<summary><strong>pre-commit/prek</strong></summary>

Add to `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v5.1.1
    hooks:
      - id: ai-rulez-recursive   # generate: write outputs for every nested .ai-rulez/ root
      - id: ai-rulez-validate    # validate --recursive: content, security and link checks
```

Every hook runs `scripts/pre-commit/run-ai-rulez.sh` and triggers only on root or nested `.ai-rulez/` changes:

| Hook id | Runs |
| --- | --- |
| `ai-rulez-generate` | `generate`, the root `.ai-rulez/` only |
| `ai-rulez-recursive` | `generate --recursive`, every nested root |
| `ai-rulez-validate` | `validate --recursive` — a real validation run, not a dry run |
| `ai-rulez-plugin-generate` | `generate --recursive --plugin --if-configured` |
| `ai-rulez-plugin-verify` | `verify --recursive --plugin --if-configured` |

Add `ai-rulez-plugin-generate` and `ai-rulez-plugin-verify` too if the repository ships plugin bundles; the
`--if-configured` flag makes them no-ops on consumer-only repositories.

</details>

<details>
<summary><strong>Poly</strong></summary>

Add ai-rulez as a managed source in your existing `poly.toml` and select the hooks your
repository needs:

```toml
[[hooks.sources]]
id = "ai-rulez"
git = "https://github.com/Goldziher/ai-rulez.git"
revision = "v5.1.1"
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

</details>

<details>
<summary><strong>Lefthook</strong></summary>

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

In a monorepo, `ai-rulez generate --recursive` and `ai-rulez validate --recursive` process every nested
`.ai-rulez/` root, report all failures, and exit non-zero if any root failed.

Or run `ai-rulez init --setup-hooks` while initializing a repo to wire hooks in automatically.

</details>

## GitHub Actions

ai-rulez ships its own composite Action ([`action.yml`](action.yml) at the repository root), so any workflow can
`uses: Goldziher/ai-rulez@<ref>` directly — no subpath. It downloads a release binary, verifies its sha256 against
the release's `checksums.txt`, caches it (keyed by OS, arch and the resolved version) and adds it to `PATH`. No Go
toolchain, Node or Python needed in the job. It reuses the same `scripts/pre-commit/run-ai-rulez.sh`
download-and-verify logic the pre-commit hooks above use, so the two stay in sync.

| | |
| --- | --- |
| **Input** `version` | `X.Y.Z`, `vX.Y.Z`, or `latest` (default). Optional — omit it to always use the newest release. |
| **Output** `version` | The resolved version, without the leading `v`. |
| **Output** `path` | Absolute path to the installed `ai-rulez` binary. |

<details>
<summary><strong>Show workflow</strong></summary>

```yaml
name: ai-rulez
on:
  pull_request:
  push:
    branches: [main]
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Goldziher/ai-rulez@main   # pin a release tag once one exists past v5.1.0 (the tag predates action.yml)
        # with:
        #   version: 5.1.0   # optional; omit (or "latest") to always use the newest release
      - run: ai-rulez generate --check   # the committed output matches the sources
      - run: ai-rulez validate --strict  # content and security checks, warnings fail
      - run: ai-rulez lock --check        # pins were updated when the sources were
```

</details>

See [CI usage](docs/lockfile.md#ci-usage) for the lock-check-vs-generate-check distinction and a scheduled
"outdated sources" report job.
