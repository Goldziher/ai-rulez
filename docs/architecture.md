# Architecture

How ai-rulez turns a source tree into the native files of 52 assistants, and where each step lives in the code. This
page is for contributors and for anyone embedding the engine; for what each option does, see
[Configuration](configuration.md).

## Repository layout

| Path | Contents |
| ---- | -------- |
| `cmd/ai-rulez` | The `main` package of the CLI |
| `cmd/commands` | The Cobra command tree: flags, output rendering and exit codes for every command |
| `internal/` | The engine; no compatibility promise ([package map](#package-map)) |
| `pkg/airulez` | The public Go API ([Embedding](embedding.md)) |
| `schema/` | JSON schemas of the configuration and of every `--format json` report |
| `tests/` | Golden, end-to-end, integration, conformance and architecture tests |
| `docs/` | This documentation; `docs/cli-reference.md` and `docs/llms*.txt` are generated |

## The pipeline

Every command that reads a project goes through the same stages:

```text
load  ->  merge  ->  validate  ->  render  ->  write
```

| Stage | What happens | Code |
| ----- | ------------ | ---- |
| **Load** | Find the configuration, read it through a `workspace` view, scan the content tree and fetch what it references. | `internal/project`, `internal/config`, `internal/workspace` |
| **Merge** | Layer `config.<name>.toml` files and the machine-local `config.local.toml` over the base file, fold in `plugins/` and `marketplaces/` definition files, and resolve includes, installed skills, installed plugins and builtin packs. | `internal/config`, `internal/includes`, `internal/skillsource`, `internal/pluginresolve`, `internal/builtins` |
| **Validate** | Reject an invalid configuration (unknown or conflicting keys, a profile that names nothing) and apply the [organization policy](policy.md) before anything is rendered. `validate` adds the content checks. | `Config.Validate`, `internal/policy`, `internal/lint` |
| **Render** | Resolve the profile (or `--role`), collect MCP servers and render every preset into a plan: the files to write, merge into or remove, each with a digest. Nothing is written yet. | `internal/generator` |
| **Write** | Apply the plan: write outputs, merge settings documents, remove stale files, and update the manifest, the lock and the managed `.gitignore` block. | `internal/generator` (appliers) |

`internal/config` cannot import the packages that implement includes, installed skills, the `.agents` source or the
presets, because they import it. `internal/project` wires them into one set of resolvers; the CLI, the MCP server and
`pkg/airulez` all load a project through it.

### Plan and apply

`generate` is `Plan` followed by `Apply` with an applier. Every mode uses the same plan, so they cannot disagree
about what a run would do:

| Applier | Used by | Effect |
| ------- | ------- | ------ |
| disk | `generate` | Writes the plan |
| dry run | `generate --dry-run` | Lists what the disk applier would do |
| check | `generate --check` | Compares the plan with the disk and the managed `.gitignore` block; writes nothing; exits 2 on drift |
| describe | `generate --emit-plan`, `pkg/airulez` | Reports the plan as a JSON document ([`schema/plan.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/plan.schema.json)) |

Two presets that write the same path must render identical bytes, or the plan fails and names both. Settings files such
as `.claude/settings.json`, `opencode.json` and `.codex/config.toml` are merged document by document: ai-rulez owns the
keys it writes and keeps everything else, comments included.

### Presets

Presets are registered in `internal/generator/registry`; nothing registers itself from an `init()`.

| Kind | Where | Presets |
| ---- | ----- | ------- |
| Declarative provider specs | `internal/generator/providers/builtin/*.toml` | 42 harnesses, plus the shared `mcp` spec for `.mcp.json` |
| Go presets | `internal/generator/presets` | `antigravity`, `baz`, `cline`, `codex`, `copilot`, `cursor`, `devin`, `gemini`, `opencode`, `xum` |
| Output presets (not harnesses) | `internal/generator/presets` | `llms-txt`, `okf` |
| Custom presets | `config.toml` | Markdown, directory or JSON outputs, or a user-supplied provider spec ([Custom presets](profiles.md)) |

`tests/golden` records the output of every built-in preset. See [Supported harnesses](harnesses.md).

## Source layouts

The load stage reads four layouts. Everything after it works on the same content tree.

| Layout | Selected by | Content lives in | `generate` writes |
| ------ | ----------- | ---------------- | ----------------- |
| `.ai-rulez/` | `.ai-rulez/config.toml` or `.config/ai-rulez/config.toml` | `.ai-rulez/` (`rules/`, `context/`, `skills/`, `agents/`, `commands/`, `checks/`, `domains/`) | Every preset's files, including `AGENTS.md` |
| `.agents/` and `AGENTS.md` | `source = "agents"`, or a configuration at `.agents/ai-rulez.toml` | The root and nested `AGENTS.md` files and `.agents/` | Every preset's files **except** `AGENTS.md` and anything under `.agents/` |
| Root file only | `ai-rulez.toml` at the project root (`init --minimal`) | No content tree; only what the file declares (installed skills, MCP servers, hooks, permissions, lint settings) | Skills, MCP and settings files; never an instruction file ([Config without a .ai-rulez folder](config-without-folder.md)) |
| OKF bundle | `export okf`, `import okf`, `okf validate`, `format = "okf"` includes | Any [OKF](okf.md) bundle | The bundle, with `export okf` |

`internal/config` (`FindProjectRoot`) picks the layout in the order given in
[Configuration discovery](configuration.md#configuration-discovery).

**`.ai-rulez/` as OKF.** `.ai-rulez/` is itself an OKF bundle: `init`, `add` and the MCP tools write concepts with
`type`, `title` and `x-ai-rulez` frontmatter and refresh the `index.md` files. `internal/okfbridge` maps content to and
from a bundle and `internal/okf` reads and validates bundles. A tree without a root `index.md` is the pre-OKF layout;
it still loads with a deprecation notice, and `migrate okf` converts it.

**`.agents/` and `AGENTS.md`.** `internal/agentssource` completes the content scan under `.agents/`: it adds the root
`AGENTS.md` as the context item `agents` (or one item per `##` heading with `agents_md_split = true`), reads each nested
`AGENTS.md` as context scoped to its directory (`agents-<dir>`, glob `<dir>/**`), and drops files that carry the
generated header, so a project is never fed its own output. The nested walk honors `.gitignore`, skips symlinks and
dependency trees, and stops at a directory that is a project of its own. On output, the generator drops everything
that would land on `AGENTS.md` or under `.agents/` (also through a symlink). See [.agents as a source](agents-source.md).

## Recursive runs

`generate --recursive`, `generate --check --recursive` and `validate --recursive` find every configuration root below
the working directory and process them concurrently, on at most `GOMAXPROCS` workers (`cmd/commands/roots.go`). The
result is deterministic:

- **Ordered output.** Roots finish in any order but are reported in path order, each as a `[n/N] path (elapsed)` line
  on stderr. The exit code does not depend on timing. `--quiet` and a structured `--format` silence the lines.
- **Per-root warnings.** Each root collects the log records its work produces (`internal/diag`,
  `cmd/commands/roots_notes.go`), and the run prints them with that root's result.

With `gitignore = true`, a recursive run writes one managed `.gitignore` block per git repository, at its top level,
holding the patterns of the roots inside it. See [Monorepo](monorepo.md).

## The managed `.gitignore` block

With `gitignore = true` (or `generate --gitignore`), ai-rulez maintains a `# BEGIN ai-rulez` / `# END ai-rulez` block in
`.gitignore` that lists the generated files, the same on every machine, and edits nothing outside it. Outputs that only
a developer's local inputs create go to `.git/info/exclude`. The entries, probes and exclusions are described in
[`gitignore`](configuration.md#gitignore).

Code: `internal/gitignore` (the fenced block) and `internal/generator/gitignore*.go` (patterns, probes, the recursive
sink).

## Plugins, marketplaces and Agent Plugins

`generate --plugin` is a second render over the same content tree. It builds distributable bundles instead of project
files:

1. **Definitions.** `[plugin]` bundles the whole root. `[[plugins_authoring]]` (or `[[marketplace.plugins]]`) defines
   several plugins, each selecting skills, commands, agents, domains, MCP servers and hooks by name or glob.
   `[[marketplaces_authoring]]` groups plugin definitions into marketplaces. Definitions can also live in
   `plugins/<name>.toml` and `marketplaces/<name>.toml`.
2. **Bundles.** For each plugin and runtime (`claude`, `codex`, `cursor`, `gemini`, `kimi`, `opencode`, `factory`,
   `hermes`, and the opt-in `pi`, `copilot` and `agent-plugins`), `internal/generator/plugin` writes the runtime's
   manifest and content under `<output_dir>/plugins/<name>/`, with provenance headers.
3. **Marketplace indexes.** One index per marketplace (`.claude-plugin/marketplace.json`, plus the Codex and Cursor
   indexes where a listed plugin targets them), each with a provenance sidecar.
4. **Verify.** `verify --plugin` regenerates in memory and reports drift per plugin and marketplace.
5. **Publish.** `publish` runs `validate --strict`, `lock --check`, `verify --plugin` and a secret scan, then writes a
   byte-reproducible archive, manifest, `SHA256SUMS` and release notes (`internal/publish`). `--to` with
   `--execute --yes` uploads to a GitHub release, npm or an OCI registry. Emitters turn the plugin into other formats:
   `agent-plugins` (`internal/agentplugins`), `ard` (`internal/ard`) and `mcp-server-card` (`internal/mcpcard`).

Emitters are pure functions of the manifest: no network, clock or file system. `internal/agentplugins` and
`internal/ard` do not import the config loader; callers map content to a model, which keeps their output deterministic
and testable against the vendored schemas in `tests/conformance`. See [Authoring plugins](plugins.md) and
[Publish](publish.md).

## The lint pipeline

`validate` and `scan` run on the same engine, `internal/lint`; `scan` selects only the security analyzer.

```text
configuration or implicit root  ->  targets  ->  analyzers  ->  filters  ->  report  ->  exit code
```

1. **Configuration or implicit root.** With a configuration, the content tree is the subject. Without one, `validate`,
   `scan` and `doctor` use an implicit configuration (no presets, default `[lint]` settings, the repository top level as
   the root) and find content by convention: `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `SKILL.md`, `.claude/`, `.agents/`,
   `.cursor/rules`, plugin manifests, hook files and `.mcp.json`. `generate`, `clean` and `lock` still need a
   configuration.
2. **Targets.** `--path`, `--glob` and `[lint] paths` add files outside the content tree. A file is classified by its
   content (plugin manifest, marketplace, hook file, MCP file) or by its name and place (`SKILL.md`, `AGENTS.md`); a text
   file of no known kind gets the secret scan only.
3. **Outputs.** `--outputs` (or `[lint] lint_outputs`) also lints generated files. Only the analyzers that judge what a
   model reads (security, references, descriptions) run over them, and a problem the source already has is reported
   once.
4. **Analyzers.** `security`, `references`, `hooks`, `mcp`, `duplicates`, `descriptions`, `budgets`, `metadata`,
   `plugin`, `config`, `roles`, `lock`, `delivery`, `evals`, `okf`, `llmstxt`, `traps`, `convert`, `exposure`, `search`
   and `verifiers`; `--analyzer` runs a subset. Each finding has a stable `AR` code, a severity and a `file:line`.
5. **Filters.** `[lint.severity]`, `[lint.ignore]`, the baseline, and `--since` / `--changed`.
6. **Report and exit code.** Text, JSON, SARIF, GitHub annotations, JUnit or Markdown. Exit `0` valid, `1` invalid
   configuration or an unreadable named target, `2` findings at or above `--fail-on`.

`--recursive` repeats this per root and merges the findings. See [Strict validation](strict-validation.md).

## Judge

`ai-rulez judge` is the lint-adjacent command that calls a model. It runs in any repository: models come from flags,
`AI_RULEZ_LLM_*` variables and the user config, never from the repository.

`cmd/commands/judge*.go` discovers the units (a skill directory, or a single agent, command, rule, instruction, MCP or
settings file), runs the `validate` analyzers over them for static facts, and reads the `[judge]` table.
`internal/judge` is the engine and does no I/O of its own: it attaches the rules that apply to each unit, builds the
prompts, plans the calls under the spending caps, verifies every quoted piece of evidence against the text that was
sent, asks the adjudicator, and computes scores and the decision in code. `internal/llm` is the single abstraction for
model calls. Nothing is sent without `--allow-llm`; `--estimate` shows the plan and cost without sending.
`bench/judge` is its evaluation. See [Judge](judge.md) and [Review](review.md).

## Package map

Every top-level package in `internal/`.

| Package | Role |
| ------- | ---- |
| `agentplugins` | Builds, validates and imports Agent Plugins packages (1.0.0 and 1.1.0) |
| `agentssource` | Completes the `source = "agents"` content tree: root and nested `AGENTS.md`, generated files dropped |
| `ambient` | Names what library code may not reach for on its own (working directory, environment, clock) |
| `approval` | Decides whether pinned content has the reviewer approvals the policy requires |
| `ard` | Builds and validates Agentic Resource Discovery manifests |
| `areas` | Loads the `[exposure]` area map and classifies paths, terms and skills by owner |
| `builtins` | Embedded builtin packs: language packs and universal rules |
| `catalogsite` | Renders the catalog document as a static site |
| `clidocs` | Renders `docs/cli-reference.md` from the Cobra tree |
| `config` | Loads and merges configuration, layers, plugin definitions and the content tree; validates it |
| `contentlock` | Computes and compares the content pins stored in `ai-rulez.lock` |
| `cost` | Reports which content items cost the most context |
| `crud` | Creates, reads, updates and deletes content items, domains and profiles (CLI and MCP tools) |
| `diag` | Collects the warnings of one run, de-duplicated |
| `doctor` | Read-only project diagnostics |
| `evals` | Harness-neutral eval case format for skills and the runners that score them |
| `exposure` | The Claude Code session load model: what a session loads at start and on demand, the start budget, area term leakage and listing collisions ([Session exposure](exposure.md)) |
| `fmdiff` | Differential fuzzing of the frontmatter splitters |
| `forge` | Minimal read-only client for the forge hosting a git remote |
| `frontmatter` | Splits a leading YAML frontmatter block from Markdown |
| `generator` | Plans and applies a generate run: presets, plugins, drift, clean, manifest, `.gitignore` block, user scope |
| `genmark` | Decides whether a file is one ai-rulez generated (shared by the generator and the linter) |
| `gitignore` | Reads and writes the fenced managed block in `.gitignore` files |
| `gitutil` | The repository questions generation needs (top level, tracked files, ignore checks) |
| `govview` | Builds the read-only governance documents shared by the CLI and the MCP server |
| `guard` | Decides whether an agent's tool call edits a generated file (`[guard]` hook) |
| `harnesslimits` | Documented harness size limits |
| `harnessnames` | Each harness's built-in command, skill and agent names, for the reserved-name checks |
| `hooks` | Detects the git hook manager a repository uses (`init --setup-hooks`) |
| `importer` | The importers behind `convert` (native, rulesync, apm, tessl, agent-plugins, okf, skills-lock) |
| `improve` | Runs an external skill optimizer behind a held-out eval gate (experimental) |
| `includes` | Resolves local and git includes, installed skills and their pins |
| `indexcache` | Keeps the MCP server's repository index and scan verdicts between runs |
| `jsondoc` | Writes the `--format json` documents |
| `judge` | The `judge` engine: units, rule attachment, prompts, evidence verification, adjudication, scoring |
| `lint` | The `validate` and `scan` engine: analyzers, `AR` codes, baseline, fixes, external scanners |
| `llm` | The single abstraction for calling a language model, with consent, cache and egress gates |
| `llmstxt` | Renders and validates `llms.txt` files |
| `lockfile` | Reads, writes and verifies `ai-rulez.lock` |
| `lockrun` | Resolves remote sources and writes `ai-rulez.lock` for one project |
| `logger` | Structured logging |
| `markdown` | Markdown formatting for generated files |
| `mcp` | The MCP server: authoring tools and the skills server |
| `mcpcard` | Builds MCP Server Cards from remote `[[mcp_servers]]` entries |
| `migrate` | `migrate v5` (4.x to 5.0) and `migrate okf` (pre-OKF tree to OKF) |
| `obscure` | Encodes embedded detection patterns so they do not appear as plain text in the source or binary |
| `okf` | Reads, validates and writes Open Knowledge Format bundles |
| `okfbridge` | Maps ai-rulez content to and from OKF bundles |
| `opencodev1` | Detects OpenCode v1-shaped plugins |
| `parity` | The checked-in map between CLI commands, their documentation and the MCP tools |
| `parser` | Frontmatter parsing for content files |
| `pinupdate` | Moves version-constraint pins and reports newer tags (`lock --outdated`, `update`, the MCP pin tools) |
| `pluginimport` | Reads other harnesses' plugin and marketplace layouts into ai-rulez content |
| `pluginresolve` | Resolves `[[installed_marketplaces]]` and `[[installed_plugins]]` |
| `policy` | The tighten-only organization policy |
| `preflight` | Lists the commands a generate run is about to write into tool configuration |
| `pricing` | Prices model calls and eval estimates |
| `progress` | Progress counters and yes/no prompts |
| `project` | Loads a configuration with the include, installed-skill, plugin and agents-source resolvers wired in |
| `publish` | Turns a verified plugin bundle into release artifacts and emitter output |
| `releasetime` | Finds when a git tag was released, for minimum-age gates |
| `render` | Separates what a command produces from what it says about it |
| `repoindex` | Indexes the skills, instruction files, rules and agents a repository keeps at any depth, and resolves what applies to a path |
| `review` | Scores skills, agents, commands and rules against a rubric |
| `roles` | Builds the roles manifest |
| `runner` | Runs an external command with the limits a tool needs |
| `safefs` | File primitives for machine-local files (no symlink following) |
| `sandbox` | Confines a child process: no network, no writes outside a directory |
| `sarifout` | SARIF 2.1.0 output shared by lint and scanners |
| `sbom` | Renders the AI configuration as a CycloneDX or SPDX bill of materials |
| `secretpat` | Credential patterns shared by the security scan and `judge` |
| `semver` | Semantic versions and constraints |
| `sessionstart` | `ai-rulez session-start`: regenerates outputs when a remote session starts, never failing the session |
| `signing` | Signs and verifies attestations with Sigstore |
| `skillsearch` | Ranks skills against a free-text query |
| `skillsource` | Resolves `[[skill_sources]]` and `--source` arguments |
| `tagresolve` | Resolves a version constraint against a remote's tags |
| `telemetry` | Records and optionally exports identifier-only item-load events |
| `templates` | Templates for `init` and generated files |
| `testutil` | Helpers shared by tests |
| `tokens` | Offline, deterministic token counting |
| `toolnames` | How each harness names its tools |
| `urlredact` | Hides credentials in URLs and in text that echoes them |
| `usage` | Skill usage telemetry without storing prompts or content |
| `utils` | Name sanitizing |
| `verifiers` | Runs the deterministic repository checks declared as `[[verifiers]]` |
| `versionpatch` | Rewrites the version constraint of one source in the configuration |
| `walkutil` | File-system walk helpers |
| `watch` | Re-runs a task when files change (`generate --watch`) |
| `workspace` | The read view of a project tree that loading goes through (directory, memory or git commit) |

Packages inside `internal/generator`:

| Package | Role |
| ------- | ---- |
| `presets` | The Go presets and custom presets |
| `providers` | The declarative provider specs and their loader |
| `registry` | Assembles the preset registry |
| `plugin` | Plugin bundles and marketplace indexes |
| `settings` | Renders `[[hooks]]`, `[permissions]` and managed settings into each harness's settings documents |
| `jsonmerge`, `tomlmerge`, `yamlmerge`, `docmerge` | Document-by-document merging into existing files |
| `hookplugins` | Renders `[[hooks]]` as code for harnesses that load hook plugins (OpenCode, Kilo, MiMo Code, Pi, Amp) |
| `rulefiles` | Rendering primitives shared by the Go presets and provider specs: item identifiers and the inline `## Rules` / `## Context` sections |
| `targetmatch` | Matching of `targets` against outputs |
| `userscope` | The user-level layout of each harness (`generate --user`) |

`pkg/airulez` is the public API over the same engine: load a project from a directory, memory or a git commit, validate
it, plan or apply a generate run, and write or check the lock. `tests/archlint` keeps ambient authority (working
directory, environment, clock) out of `internal/` library code.
