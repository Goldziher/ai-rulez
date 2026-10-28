# Architecture

How ai-rulez turns a source tree into the native files of 52 harnesses, and where each step lives in the code. Read this
before changing the load, render or lint paths; for what each option does, see [Configuration](configuration.md).

## The pipeline

Every command that reads a project goes through the same stages:

```text
load  ->  merge  ->  validate  ->  render  ->  write
```

| Stage | What happens | Code |
| ----- | ------------ | ---- |
| **Load** | Find the configuration (`.ai-rulez/config.toml`, `.config/ai-rulez/config.toml` or `.agents/ai-rulez.toml`), read it through a `workspace` view, scan the content tree and fetch what it references. | `internal/project`, `internal/config`, `internal/workspace` |
| **Merge** | Layer `config.<name>.toml` files (`default_layers`, `AI_RULEZ_LAYERS`) and the machine-local `config.local.toml` over the base file, fold in the `plugins/` and `marketplaces/` definition files, resolve includes, installed skills and builtin packs. | `internal/config`, `internal/includes`, `internal/skillsource`, `internal/builtins` |
| **Validate** | Reject an invalid configuration (unknown or conflicting keys, a profile that names nothing, `[plugin]` combined with `[[plugins_authoring]]`) and enforce the [organization policy](policy.md) before anything is rendered; `validate` adds the content checks. | `Config.Validate`, `internal/policy`, `internal/lint` |
| **Render** | Resolve the profile (or `--role`), collect MCP servers and render every preset into a plan: the files to write, merge into or remove, each with a digest. Nothing is written yet. | `internal/generator` |
| **Write** | Apply the plan: write outputs, merge settings documents, remove stale files, update the manifest, lock and the managed `.gitignore` block. | `internal/generator` (appliers) |

`internal/config` cannot import the packages that implement includes, installed skills, the `.agents` source or the
presets, and they import it, so `internal/project` joins them into one set of resolvers. The CLI, the MCP server and the
public API in `pkg/airulez` all load a project through it.

### Plan and apply

`generate` is `Plan` followed by `Apply` with an applier. The plan is the same whichever applier runs it, which is why the
modes cannot disagree about what a run would do:

| Applier | Used by | Effect |
| ------- | ------- | ------ |
| disk | `generate` | writes the plan |
| dry run | `generate --dry-run` | lists what the disk applier would do |
| check | `generate --check` | compares the plan with the disk and the managed `.gitignore` block, writes nothing, exits 2 on drift |
| describe | `generate --emit-plan`, `pkg/airulez` | reports the plan as a JSON document ([schema](https://github.com/Goldziher/ai-rulez/blob/main/schema/plan.schema.json)) |

Two presets that write the same path must render identical bytes, or the plan fails and names both. Settings files such as
`.claude/settings.json`, `opencode.json` and `.codex/config.toml` are merged document by document: ai-rulez owns the keys
it writes and keeps the rest, comments included.

### Presets

The 52 harness presets are the entries the registry returns as individual presets. 42 are declarative provider specs
(`internal/generator/providers/builtin/*.toml`, plus the `mcp` spec), ten are Go presets in
`internal/generator/presets` (`antigravity`, `baz`, `cline`, `codex`, `copilot`, `cursor`, `devin`, `gemini`, `opencode`,
`xum`). `okf` and `llms-txt` are output presets that are not harnesses and are not counted. `tests/golden` records the
output of every preset, so a new preset cannot land without a golden. See [Supported harnesses](harnesses.md).

## Three source layouts

The same pipeline reads three layouts. They differ only in the load stage; everything after it works on the same content
tree.

| Layout | Selected by | Content lives in | Written by `generate` |
| ------ | ----------- | ---------------- | --------------------- |
| `.ai-rulez` | the default | `.ai-rulez/` (`rules/`, `context/`, `skills/`, `agents/`, `commands/`, `checks/`, `domains/`) | every preset's files, including `AGENTS.md` |
| `.agents` and `AGENTS.md` | `source = "agents"`, or a configuration in `.agents/ai-rulez.toml` | the root and nested `AGENTS.md` and `.agents/` | every preset's files **except** `AGENTS.md` and anything under `.agents/` |
| OKF | an `index.md` bundle; the `okf` preset | an [OKF](okf.md) bundle, which is also what `.ai-rulez/` is internally | the bundle, with `export okf` |

**`.ai-rulez` as OKF.** `.ai-rulez/` is an OKF bundle: `init`, `add` and the MCP tools write concepts with `type`, `title`
and `x-ai-rulez` frontmatter and refresh the `index.md` files. `internal/okfbridge` maps content to and from a bundle,
`internal/okf` reads and validates bundles. A tree without a root `index.md` is the legacy layout and still loads with a
deprecation notice.

**`.agents` and `AGENTS.md`.** `internal/agentssource` completes what the content scan found under `.agents`: it adds the
root `AGENTS.md` as the context item `agents` (or one item per `##` heading with `agents_md_split = true`), reads every
nested `AGENTS.md` as context scoped to its directory (`agents-<dir>`, glob `<dir>/**`), and drops files that carry the
generated header, so a project that switched source is not fed its own output. The nested walk honours `.gitignore` inside a
git repository, skips symlinks and dependency trees, and stops at a directory that is a project of its own. On the way out,
the generator drops every output that would land on `AGENTS.md` or under `.agents/` (also through a symlink), so `generate`
and `generate --check` leave the standard's files alone. `generate --watch` watches `AGENTS.md`, `.agents` and the nested
files found at each run. Details: [.agents as a source](agents-source.md).

**OKF.** `export okf` and `import okf` convert between a bundle and the content tree; `okf validate <dir>` lints any bundle
with the `AR9B` codes without a configuration. See [OKF](okf.md).

## Recursive runs

`generate --recursive`, `generate --check --recursive` and `validate --recursive` find every configuration root below the
working directory and process them concurrently on at most one worker per CPU (`GOMAXPROCS`; `validate --external` uses
one). Two properties keep the result deterministic:

- **Ordered output.** For `generate --check --recursive` and `validate --recursive` the roots finish in any order but are
  emitted in path order, each as a `[n/N] path (elapsed)` line on stderr. A slow first root delays the print, never
  reorders it, and the exit code does not depend on timing. `--quiet` and a structured `--format` silence the lines.
- **Per-root warnings.** Each root collects the log records its work raises (`internal/diag`, `rootNotes` in
  `cmd/commands`) and the run prints them with that root's result. The writing `generate --recursive` shows a progress
  counter and logs a root's load warnings as they happen.

A recursive run with `gitignore = true` writes one managed block per git repository, at that repository's toplevel,
holding the patterns of the roots inside it, and `--check` verifies each block once after the last root. A root's
patterns never reach the `.gitignore` of another repository or of a root with `gitignore = false`. See
[Monorepo](monorepo.md).

## The managed `.gitignore` block

With `gitignore = true` (or `generate --gitignore`) ai-rulez maintains a `# BEGIN ai-rulez` / `# END ai-rulez` block in
`.gitignore`, and edits nothing outside it. The default is off, because generated files are meant to be committed.

- The entries are the specific generated files and owned subtrees, never an assistant directory as a whole.
- Before writing, the generator asks git (`git check-ignore`) about each entry against every ignore source with its own
  block left out. An entry already ignored by your rules, or re-included by a negation of yours, is left out of the block.
- `gitignore_exclude` lists generated outputs you commit on purpose; they stay out of the block while every other entry stays
  managed. Machine-local and secret outputs are never excluded.
- The block is the same on every machine and with or without `--no-local`. It always lists `.ai-rulez/config.local.*`
  (and the overlay's `.config.local.*` lock and temp files), `.ai-rulez/local/` and the machine-local manifest, whether
  or not those exist. It never lists a path only because this machine's local inputs add it: outputs the overlay or
  `local/` content creates (`CLAUDE.local.md`, a hook-only `.claude/settings.json`, local skills) are kept in
  `.git/info/exclude`, which is not committed.
- With `gitignore = false` the machine-local inputs that exist (`config.local.*`, `local/`) are still ignored, and an
  MCP config holding secrets is refused unless git ignores it. A machine-local manifest that is the only local file goes
  to `.git/info/exclude`, so `generate` does not write a tracked `.gitignore` for a project that turned it off.
- `generate --check` reports a block that differs from what a run would write as drift, and `generate` warns when a tracked
  file matches a line of the block.

Code: `internal/gitignore` (the fenced block), `internal/generator/gitignore*.go` (patterns, probes, the recursive sink).

## Plugins, marketplaces and Agent Plugins

`generate --plugin` is a second render over the same content tree. It builds distributable bundles instead of project
files:

1. **Definitions.** `[plugin]` bundles the whole root. Several plugins come from `[[plugins_authoring]]` entries (or
   `[[marketplace.plugins]]`), each selecting skills, commands, agents, domains, MCP servers and hooks by name or glob; an
   empty selector bundles nothing. `[[marketplaces_authoring]]` entries each list plugin definitions by name. Each definition
   can live in `plugins/<name>.toml` or `marketplaces/<name>.toml`, and a layer can add, override or remove one by `name`.
   `[plugin]` and `[[plugins_authoring]]` cannot be combined, nor `[marketplace]` and `[[marketplaces_authoring]]`.
2. **Bundles.** For each plugin and each runtime (`claude`, `codex`, `cursor`, `gemini`, `kimi`, `opencode`, `factory`,
   `hermes`, and the opt-in `pi`, `copilot` and `agent-plugins`) `internal/generator/plugin` writes the runtime's manifest and
   content under `<output_dir>/plugins/<name>/`, with provenance headers.
3. **Marketplace indexes.** One index per marketplace (`.claude-plugin/marketplace.json`, plus the Codex and Cursor indexes
   where a listed plugin targets them), each directory publishable on its own and each with a provenance sidecar.
4. **Verify.** `verify --plugin` regenerates in memory and reports drift per plugin and marketplace.
5. **Publish.** `publish` runs `validate --strict`, `lock --check`, `verify --plugin` and a secret scan, then writes a
   byte-reproducible archive, manifest, `SHA256SUMS`, the lock and release notes; `--to` with `--execute --yes` pushes to a
   GitHub release, npm or an OCI registry. Emitters turn the published plugin into other formats: `agent-plugins`
   ([Agent Plugins](agent-plugins.md), `internal/agentplugins`), `ard` ([ARD](ard.md), `internal/ard`) and `mcp-server-card`
   ([MCP server cards](mcp-server-card.md), `internal/mcpcard`), plus the Cursor team marketplace and experimental ones.
   Emitters are pure functions of the manifest: no network, clock or file system.

`internal/agentplugins` and `internal/ard` do not import the config loader; callers map their content to a model, which is
what makes their output deterministic and testable against the vendored schemas in `tests/conformance`. See
[Authoring plugins](plugins.md) and [Publish](publish.md).

## The lint pipeline

`validate` and `scan` run on the same engine (`internal/lint`); `scan` selects only the security analyzers.

```text
configuration or implicit root  ->  targets  ->  analyzers  ->  filters  ->  report  ->  exit code
```

1. **Configuration or implicit root.** With a configuration, the content tree is the subject. Without one, `validate`, `scan`
   and `doctor` use an *implicit* configuration: no presets, hooks or MCP servers, default `[lint]` settings, the repository
   toplevel as the root. The content is found by convention (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `SKILL.md`, `.claude/`,
   `.agents/`, `.cursor/rules`, plugin manifests, hook files, `.mcp.json`) and treated as hand-written. A line on stderr
   says so; nothing is written. `generate`, `clean` and `lock` still need a configuration.
2. **Targets.** `--path`, `--glob` and `[lint] paths` add files outside the content tree, such as a marketplace in a
   directory of its own; with an implicit root they replace the detection. Every target must resolve inside the repository
   root. A file is classified by its content (a plugin manifest, a marketplace, a hook file, an MCP file) or by its name and
   place (`SKILL.md`, `AGENTS.md`), and a file that cannot be linted is named, not dropped. A text file of no known kind gets
   the secret scan only.
3. **Outputs.** `--outputs` (or `[lint] lint_outputs`) also lints the generated files: the bytes `generate` writes or would
   write, plus the files its last run recorded. Only the analyzers that judge what a model reads (security, references,
   descriptions) run over them, a finding is marked as output, and a problem the authored source already has is reported once.
4. **Analyzers.** `security`, `references`, `hooks`, `mcp`, `duplicates`, `descriptions`, `budgets`, `metadata`, `plugin`,
   `config`, `roles`, `lock`, `delivery`, `evals`, `okf`, `llmstxt`, `traps`, `convert`, `search` and `verifiers`;
   `--analyzer` runs a chosen subset. Each finding has a stable `AR` code, a severity and a `file:line`.
5. **Filters.** `[lint.severity]` and `[lint.ignore]`, the baseline, and `--since` / `--changed` (with the reference hops that
   pull in dependent files).
6. **Report and exit code.** Text, JSON, SARIF, GitHub annotations, JUnit or Markdown. `0` valid, `1` the configuration is
   invalid or a named target could not be read, `2` findings at or above `--fail-on`.

`--recursive` repeats this per root and merges the findings. See [Strict validation](strict-validation.md).

## Judge

`ai-rulez judge` is the one lint-adjacent command that calls a model. It runs in any repository: the models come from flags,
`AI_RULEZ_LLM_*` variables and the user config, never from the repository, and the key from the provider's own variable.

`cmd/commands/judge*.go` discovers the units (a skill directory, or one file: agents, commands, rules, `CLAUDE.md`,
`AGENTS.md`, MCP and settings files, `.claude/` at any depth), runs the `validate` analyzers over them for static facts, and
reads the `[judge]` table. `internal/judge` is the engine and does no I/O of its own: it attaches the rules and `CLAUDE.md`
files that apply to each unit, builds the nonce-fenced prompts, plans the calls under the caps, verifies every quote against
the text that was sent, asks the adjudicator, and computes scores, statuses and the decision in code. `internal/llm` is the
single abstraction that calls the model (and resolves and refreshes AWS credentials for Bedrock). Nothing is sent without
`--allow-llm`, `--estimate` shows the units, the egress host, the models and the cost estimate and sends nothing, and
`--max-cost` caps the spend. `bench/judge` is its evaluation. See [Judge](judge.md); for scored review with calibration, see
[Review](review.md).

## Package map

Every package below `internal/`, derived from the tree. Sub-packages are named where they carry the logic.

| Package | Role |
| ------- | ---- |
| `agentplugins` | Builds, validates and imports Agent Plugins packages (1.0.0 and 1.1.0) |
| `agentssource` | Completes the `source = "agents"` content tree: root and nested `AGENTS.md`, generated files dropped |
| `ambient` | Names what library code may not reach for on its own (working directory, environment, clock) |
| `areas` | Loads the `[exposure]` area map, matches paths and terms to areas and classifies artifacts and skills by owner |
| `approval` | Decides whether pinned content has the reviewer approvals the policy requires |
| `ard` | Builds and validates Agentic Resource Discovery manifests |
| `builtins` | Embedded builtin packs: language packs and universal rules |
| `catalogsite` | Renders the catalog document as a static site |
| `clidocs` | Renders the command reference in `docs/` from the Cobra tree |
| `config` | Loads and merges configuration, layers, plugin definitions and the content tree; validates it |
| `contentlock` | Computes and compares the content pins stored in `ai-rulez.lock` |
| `cost` | Reports which content items cost the most context |
| `crud` | File-based create, read, update and delete of rules, context, skills, agents, commands, checks, domains and profiles |
| `diag` | Collects the warnings of one generate run, de-duplicated |
| `doctor` | Read-only diagnostics over a project |
| `evals` | Harness-neutral eval case format for skills and the runners that score them |
| `exposure` | The Claude Code session load model: every native artifact of a repository with its phase (start, on demand, manual or unknown), visibility, listing text and frontmatter parse status, after the root project settings and the repository marketplace; the start-of-session report and budget, area term leakage and listing collisions ([Session exposure](exposure.md)) |
| `fmdiff` | Differential fuzzing of the frontmatter splitters |
| `forge` | Minimal read-only client for the hosting forge of a git remote |
| `frontmatter` | Splits a leading YAML frontmatter block from markdown |
| `generator` | Plans and applies a generate run: presets, plugins, drift, clean, manifest, `.gitignore` block, user scope |
| `gitignore` | Reads and writes the fenced managed block in `.gitignore` files |
| `gitutil` | The few repository questions generation needs (toplevel, tracked files, ignore checks) |
| `govview` | Builds the read-only governance documents shared by the CLI and the MCP server |
| `guard` | Decides whether an agent's tool call edits a generated file (`[guard]` hook) |
| `harnesslimits` | Table of documented harness size limits |
| `hooks` | Detects the git hook system a repository uses |
| `importer` | The importers behind `convert` (native, rulesync, apm, tessl, agent-plugins, okf, skills-lock) |
| `improve` | Runs an external skill optimizer behind a held-out eval gate (experimental) |
| `includes` | Resolves local and git includes, installed skills and their pins |
| `jsondoc` | Writes the JSON documents of `--format json` |
| `judge` | The `judge` engine: units, attachment of rules, prompts, verifier, adjudicator, scoring, reports |
| `lint` | The `validate` and `scan` engine: analyzers, `AR` codes, baseline, fixes, external scanners |
| `llm` | The single abstraction for calling a language model, with consent, cache and egress gates |
| `llmstxt` | Renders and validates `llms.txt` files |
| `lockfile` | Reads, writes and verifies `ai-rulez.lock` |
| `lockrun` | Resolves remote sources and writes `ai-rulez.lock` for one project |
| `logger` | Structured logging used by every package |
| `markdown` | Markdown formatting for generated files |
| `mcp` | The MCP server: authoring tools and the read-only skills server |
| `mcpcard` | Builds MCP Server Cards from the remote `[[mcp_servers]]` entries |
| `migrate` | `migrate v5` (4.x to 5.0) and the content-tree to OKF conversion |
| `okf` | Reads, validates and writes Open Knowledge Format bundles |
| `okfbridge` | Maps ai-rulez content to and from OKF bundles |
| `opencodev1` | Detects OpenCode v1-shaped plugins |
| `parity` | The checked-in map between the command line and its documentation and MCP tools |
| `parser` | Frontmatter parsing for content files |
| `policy` | The tighten-only organization policy |
| `preflight` | Lists the commands a generate run is about to write into configuration |
| `pricing` | Prices model calls and eval estimates |
| `progress` | Progress counters and yes/no prompts |
| `project` | Loads a configuration with the include, installed-skill and agents-source resolvers wired in |
| `publish` | Turns a verified plugin bundle into release artifacts and emitter output |
| `releasetime` | Finds when a git tag was released, for minimum-age gates |
| `render` | Separates what a command produces from what it says about it |
| `repoindex` | Indexes the skills, instruction files, rules and agents a repository keeps at any depth by scope, and resolves a path to what applies there; an item keeps the error of a frontmatter block that does not decode |
| `indexcache` | Keeps the repository index summaries and scan verdicts between runs of the MCP server: location (user cache or project), authenticated index file, lock, status and clear |
| `review` | Scores skills, agents, commands and rules against a rubric |
| `roles` | Builds the roles manifest |
| `runner` | Runs an external command with the limits a tool needs |
| `safefs` | File primitives for machine-local files (no symlink following) |
| `sandbox` | Confines a child process: no network, no writes outside a directory |
| `sarifout` | SARIF 2.1.0 output pieces shared by lint and scanners |
| `sbom` | Renders the AI configuration as a CycloneDX or SPDX bill of materials |
| `secretpat` | Credential patterns shared by the security scan and `judge` |
| `semver` | Semantic versions and constraints |
| `signing` | Signs and verifies attestations with Sigstore |
| `skillsearch` | Ranks skills against a free-text query |
| `skillsource` | Resolves `[[skill_sources]]` and `--source` arguments |
| `tagresolve` | Resolves a version constraint against a remote's tags |
| `telemetry` | Records and optionally exports identifier-only item-load events |
| `templates` | Templates for `init` and generated files |
| `testutil` | Helpers shared by tests |
| `tokens` | Offline, deterministic token counting |
| `toolnames` | How each harness names its tools |
| `urlredact` | Hides credentials in URLs and text that echoes them |
| `usage` | Skill usage telemetry without storing prompts or content |
| `utils` | Name sanitizing |
| `verifiers` | Runs the deterministic repo checks declared as `[[verifiers]]` |
| `versionpatch` | Rewrites the version constraint of one source in the configuration |
| `walkutil` | Filesystem-walk helpers |
| `watch` | Re-runs a task when files change |
| `workspace` | The read view of a project tree that loading goes through (directory, memory or commit) |

Packages inside `internal/generator`: `presets` (Go presets), `providers` (the declarative specs and their loader), `registry`
(assembles the preset registry), `plugin` (plugin bundles and marketplace indexes), `settings`, `jsonmerge`, `tomlmerge`,
`yamlmerge` and `docmerge` (document-by-document merging), `hookplugins` (generated hook plugin modules), `rulefiles`,
`targetmatch` and `userscope` (the user-level layout of each harness).

`pkg/airulez` is the public Go API over the same engine: load a project from a directory, memory or a commit, validate it, plan
or apply a generate run, and write or check the lock. `tests/archlint` keeps ambient authority out of `internal/`.
