---
title: "Architecture"
description: "How ai-rulez turns a source tree into the native files of 52 assistants: the pipeline stages, source layouts and what each mode does."
---

How ai-rulez turns a source tree into the native files of 52 assistants. For what each option does, see
[Configuration](/ai-rulez/configuration/); to embed the engine, see [Embedding](/ai-rulez/embedding/).

## The pipeline

Every command that reads a project goes through the same stages:

```text
load  ->  merge  ->  validate  ->  render  ->  write
```

| Stage | What happens |
| ----- | ------------ |
| **Load** | Find the configuration, read it through a `workspace` view, scan the content tree and fetch what it references. |
| **Merge** | Layer `config.<name>.toml` files and the machine-local `config.local.toml` over the base file, fold in `plugins/` and `marketplaces/` definition files, and resolve includes, installed skills, installed plugins and builtin packs. |
| **Validate** | Reject an invalid configuration (unknown or conflicting keys, a profile that names nothing) and apply the [organization policy](/ai-rulez/policy/) before anything is rendered. `validate` adds the content checks. |
| **Render** | Resolve the profile (or `--role`), collect MCP servers and render every preset into a plan: the files to write, merge into or remove, each with a digest. Nothing is written yet. |
| **Write** | Apply the plan: write outputs, merge settings documents, remove stale files, and update the manifest, the lock and the managed `.gitignore` block. |

### Plan and apply

`generate` is `Plan` followed by `Apply` with an applier. Every mode uses the same plan, so they cannot disagree
about what a run would do:

| Applier | Used by | Effect |
| ------- | ------- | ------ |
| disk | `generate` | Writes the plan |
| dry run | `generate --dry-run` | Lists what the disk applier would do |
| check | `generate --check` | Compares the plan with the disk and the managed `.gitignore` block; writes nothing; exits 2 on drift |
| describe | `generate --emit-plan`, the Go API | Reports the plan as a JSON document ([`schema/plan.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/plan.schema.json)) |

Two presets that write the same path must render identical bytes, or the plan fails and names both. Settings files such
as `.claude/settings.json`, `opencode.json` and `.codex/config.toml` are merged document by document: ai-rulez owns the
keys it writes and keeps everything else, comments included.

### Presets

| Kind | Presets |
| ---- | ------- |
| Declarative provider specs | 42 harnesses, plus the shared `mcp` spec for `.mcp.json` |
| Go presets | `antigravity`, `baz`, `cline`, `codex`, `copilot`, `cursor`, `devin`, `gemini`, `opencode`, `xum` |
| Output presets (not harnesses) | `llms-txt`, `okf` |
| Custom presets | Markdown, directory or JSON outputs, or a user-supplied provider spec ([Custom presets](/ai-rulez/profiles/)) |

See [Supported harnesses](/ai-rulez/harnesses/).

## Source layouts

The load stage reads four layouts. Everything after it works on the same content tree.

| Layout | Selected by | Content lives in | `generate` writes |
| ------ | ----------- | ---------------- | ----------------- |
| `.ai-rulez/` | `.ai-rulez/config.toml` or `.config/ai-rulez/config.toml` | `.ai-rulez/` (`rules/`, `context/`, `skills/`, `agents/`, `commands/`, `checks/`, `domains/`) | Every preset's files, including `AGENTS.md` |
| `.agents/` and `AGENTS.md` | `source = "agents"`, or a configuration at `.agents/ai-rulez.toml` | The root and nested `AGENTS.md` files and `.agents/` | Every preset's files **except** `AGENTS.md` and anything under `.agents/` |
| Root file only | `ai-rulez.toml` at the project root (`init --minimal`) | No content tree; only what the file declares (installed skills, MCP servers, hooks, permissions, lint settings) | Skills, MCP and settings files; never an instruction file ([Config without a .ai-rulez folder](/ai-rulez/config-without-folder/)) |
| OKF bundle | `export okf`, `import okf`, `okf validate`, `format = "okf"` includes | Any [OKF](/ai-rulez/okf/) bundle | The bundle, with `export okf` |

The layout is picked in the order given in [Configuration discovery](/ai-rulez/configuration/#configuration-discovery).

**`.ai-rulez/` as OKF.** `.ai-rulez/` is itself an OKF bundle: `init`, `add` and the MCP tools write concepts with
`type`, `title` and `x-ai-rulez` frontmatter and refresh the `index.md` files. A tree without a root `index.md` is the pre-OKF layout;
it still loads with a deprecation notice, and `migrate okf` converts it.

**`.agents/` and `AGENTS.md`.** The content scan under `.agents/` adds the root
`AGENTS.md` as the context item `agents` (or one item per `##` heading with `agents_md_split = true`), reads each nested
`AGENTS.md` as context scoped to its directory (`agents-<dir>`, glob `<dir>/**`), and drops files that carry the
generated header, so a project is never fed its own output. The nested walk honors `.gitignore`, skips symlinks and
dependency trees, and stops at a directory that is a project of its own. On output, the generator drops everything
that would land on `AGENTS.md` or under `.agents/` (also through a symlink). See [.agents as a source](/ai-rulez/agents-source/).

## Recursive runs

`generate --recursive`, `generate --check --recursive` and `validate --recursive` find every configuration root below
the working directory and process them concurrently, on at most `GOMAXPROCS` workers. The
result is deterministic:

- **Ordered output.** Roots finish in any order but are reported in path order, each as a `[n/N] path (elapsed)` line
  on stderr. The exit code does not depend on timing. `--quiet` and a structured `--format` silence the lines.
- **Per-root warnings.** Each root collects the log records its work produces, and the run prints them with that root's result.

With `gitignore = true`, a recursive run writes one managed `.gitignore` block per git repository, at its top level,
holding the patterns of the roots inside it. See [Monorepo](/ai-rulez/monorepo/).

## The managed `.gitignore` block

With `gitignore = true` (or `generate --gitignore`), ai-rulez maintains a `# BEGIN ai-rulez` / `# END ai-rulez` block in
`.gitignore` that lists the generated files, the same on every machine, and edits nothing outside it. Outputs that only
a developer's local inputs create go to `.git/info/exclude`. The entries, probes and exclusions are described in
[`gitignore`](/ai-rulez/configuration/#gitignore).

## Plugins, marketplaces and Agent Plugins

`generate --plugin` is a second render over the same content tree. It builds distributable bundles instead of project
files:

1. **Definitions.** `[plugin]` bundles the whole root. `[[plugins_authoring]]` (or `[[marketplace.plugins]]`) defines
   several plugins, each selecting skills, commands, agents, domains, MCP servers and hooks by name or glob.
   `[[marketplaces_authoring]]` groups plugin definitions into marketplaces. Definitions can also live in
   `plugins/<name>.toml` and `marketplaces/<name>.toml`.
2. **Bundles.** For each plugin and runtime (`claude`, `codex`, `cursor`, `gemini`, `kimi`, `opencode`, `factory`,
   `hermes`, and the opt-in `pi`, `copilot` and `agent-plugins`), ai-rulez writes the runtime's
   manifest and content under `<output_dir>/plugins/<name>/`, with provenance headers.
3. **Marketplace indexes.** One index per marketplace (`.claude-plugin/marketplace.json`, plus the Codex and Cursor
   indexes where a listed plugin targets them), each with a provenance sidecar.
4. **Verify.** `verify --plugin` regenerates in memory and reports drift per plugin and marketplace.
5. **Publish.** `publish` runs `validate --strict`, `lock --check`, `verify --plugin` and a secret scan, then writes a
   byte-reproducible archive, manifest, `SHA256SUMS` and release notes. `--to` with
   `--execute --yes` uploads to a GitHub release, npm or an OCI registry. Emitters turn the plugin into other formats:
   `agent-plugins`, `ard` and `mcp-server-card`.

Emitters are deterministic: the same plugin always yields the same bytes, validated against the vendored schemas in the conformance tests. See [Authoring plugins](/ai-rulez/plugins/) and
[Publish](/ai-rulez/publish/).

## The lint pipeline

`validate` and `scan` run on the same engine; `scan` selects only the security analyzer.

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

`--recursive` repeats this per root and merges the findings. See [Strict validation](/ai-rulez/strict-validation/).

## Judge

`ai-rulez judge` is the lint-adjacent command that calls a model. It runs in any repository: models come from flags,
`AI_RULEZ_LLM_*` variables and the user config, never from the repository.

The command discovers the units (a skill directory, or a single agent, command, rule, instruction, MCP or
settings file), runs the `validate` analyzers over them for static facts, and reads the `[judge]` table. The engine
attaches the rules that apply to each unit, builds the prompts, plans the calls under the spending caps, verifies every
quoted piece of evidence against the text that was sent, asks the adjudicator, and computes scores and the decision in
code. Nothing is sent without `--allow-llm`; `--estimate` shows the plan and cost without sending. See
[Judge](/ai-rulez/judge/) and [Review](/ai-rulez/review/).
