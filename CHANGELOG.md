# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project adheres to Semantic Versioning.

## [Unreleased]

### Added

- **`targets` accepts negations, and the `baz` preset has a per-rule opt-in.** `targets: ["!baz"]` excludes a rule from one preset (a negation always wins; a rule with only negations still goes everywhere else), `[rules.baz] include = [...]` and `baz: true` frontmatter opt rules in under `baz_default = "none"`, and the directory-target workaround is documented ([#366](https://github.com/Goldziher/ai-rulez/issues/366)).
- **`[rules] baz_unscoped = "skip"` keeps directory-less path-scoped rules out of the root `AGENTS.md`.** A rule scoped by `**/*.go` or `**/BUILD` cannot be nested for Baz, so it was inlined into the root file that Claude also imports; `skip` drops it when `baz` is the only preset writing that file (a file shared with `codex` and the others keeps it). The default `"root"` is unchanged, and the `baz_budget` warning now names the inlined rules ([#365](https://github.com/Goldziher/ai-rulez/issues/365)).
- **`[rules] baz_dedupe_max = K` collapses a rule copied into more than K directories.** The rule is written once to the directory covering all its globs, up to the root `AGENTS.md` when they share only the repository root (never with `baz_unscoped = "skip"`), instead of one copy per directory ([#367](https://github.com/Goldziher/ai-rulez/issues/367)).

### Fixed

- **`docs/baz.md` no longer claims Claude Code loads a nested `AGENTS.md` whenever its directory has no `CLAUDE.md`.** Claude Code reads `AGENTS.md` natively only when no `CLAUDE.md` exists in the working directory or above it (v2.1.277 or later), so with the `claude` preset's root `CLAUDE.md` shim a baz-only nested file is not loaded; the page now states the three cases and what the documentation does not settle ([#369](https://github.com/Goldziher/ai-rulez/issues/369)).

## [5.0.3] - 2026-10-12

### Added

- **One MCP server serves the skills and the authoring tools.** `ai-rulez mcp --serve-skills` now registers the config tools alongside `find_skill`/`load_skill` and the `skill://` resources, in one process under one name (`ai-rulez`), so a project no longer runs `ai-rulez` and `ai-rulez-skills` side by side; `--read-only` registers only the read-only tools, the generated `.mcp.json` carries a single `ai-rulez` entry running `mcp --serve-skills`, and `AR993` counts that entry ([#358](https://github.com/Goldziher/ai-rulez/issues/358)).
- **`ai-rulez mcp --serve-skills --daemon` shares one long-lived server per repository.** The session becomes a stdio proxy: it connects to a per-repo daemon (a `0600` socket in a `0700` directory under the user cache, keyed by repository toplevel, ai-rulez version and the serving flags), spawns it on first use, and serves in-process on any failure. The daemon exits after 15 minutes with no client, removes its socket, and reloads the catalog on change; the mode is not offered on Windows ([#321](https://github.com/Goldziher/ai-rulez/issues/321)).
- **`[plugin] shared_resources` copies shared directories into every bundled skill.** Each listed directory (e.g. `references/`) is written under every skill for every runtime and recorded in the provenance sidecar, a relative `../../references/x.md` link from `SKILL.md` is rewritten to the copy, and a name clash with a skill's own file is an error rather than a silent overwrite ([#353](https://github.com/Goldziher/ai-rulez/issues/353)).
- **`[plugin] provenance_headers = false` writes bundled files without the generated header.** Markdown/JS/TS/PY outputs are byte-identical to their sources while `.ai-rulez-generated.json` still records every output, so `verify --plugin`, obsolete pruning and `publish --emit agent-plugins` are unchanged and a header-less file ai-rulez did not record stays hand-written ([#354](https://github.com/Goldziher/ai-rulez/issues/354)).
- **`[plugin] version_from` reads the plugin version from an existing manifest.** `version_from = "package.json"` (also `pyproject.toml` and `Cargo.toml`) resolves `project.version`/`package.version` for the manifest, the lock and `verify --plugin`, so a bump by release-please or changesets shows as drift; it is mutually exclusive with `version`, and `validate` reports a missing file, a missing key or both keys set ([#355](https://github.com/Goldziher/ai-rulez/issues/355)).
- **The Pi runtime can reuse the shared skills tree and keep its own `package.json`.** `[plugin.pi] skills_dir` writes each skill once into that tree and points `pi.skills` at it instead of a second copy under `.pi/skills/`, and `package_json = "merge"` updates only the keys ai-rulez owns, keeping `scripts`, `devDependencies` and the rest ([#356](https://github.com/Goldziher/ai-rulez/issues/356)).
- **`AR9T0 root-component-shipped` warns when a single-plugin root holds paths hosts load by default.** A plugin whose bundle root is the repository root is reported for each default component it ships there (`.mcp.json`, `hooks/`, `agents/`, `commands/`, `settings.json`, ...), naming the host that loads it, with `content_root` or a monorepo member as the fix ([#357](https://github.com/Goldziher/ai-rulez/issues/357)).
- **A plugin can set its package manager and its OpenCode dependency.** `[plugin] package_manager` emits `packageManager` in the generated OpenCode/Pi root `package.json`, and `[plugin] opencode_dependency` overrides the `@opencode/plugin` constraint (default bumped to `^2.0.26`). The JSON serializer no longer escapes `&` as `\u0026` ([#350](https://github.com/Goldziher/ai-rulez/issues/350)).

### Fixed

- **The manifest-free orphan walk leaves a committed plugin bundle alone.** `generate --plugin` writes its bundle into the project tree with the same generated header the generator writes, so `generate --check` reported every bundle file as an orphan and `generate` deleted them; the walk now honours the bundle's `.ai-rulez-generated.json` provenance sidecar ([#350](https://github.com/Goldziher/ai-rulez/issues/350)).
- **A hosted MCP server's `oauth` block reaches `.mcp.json` for the `copilot` and `cursor` presets.** Only the `mcp` sidecar wrote it, so `generate` failed with "presets write different content to the same path" whenever `copilot` or `cursor` was active ([#334](https://github.com/Goldziher/ai-rulez/issues/334)).
- **A top-level key written after a `[[table]]` array header is a load error.** The swallowed-key check only descended `[table]` maps, so a key after `[[mcp_servers]]` was silently read as a key of the last element and ignored ([#344](https://github.com/Goldziher/ai-rulez/issues/344)).
- **`generate --plugin` rewrites an existing generated header instead of stacking another.** A plugin whose `content_root` overlaps its output tree (`content_root = "."`) reads its own decorated output as the source, so each run added a header block, `verify --plugin` never converged and `publish --emit agent-plugins` failed at the `AR9N0` preflight ([#351](https://github.com/Goldziher/ai-rulez/issues/351)).
- **`AR301` no longer reads prose as a skill pointer.** A backticked term before the noun "skill" ("a `both` skill") and a `skills/` path segment that is not this project's tree (`src/mastra/public/skills/shared/<name>/SKILL.md`) were reported as unknown references; the matcher now requires a pointer form, so the real ones (`reference: skill x`, `skills/x`, `.claude/skills/x/SKILL.md`) still fire ([#352](https://github.com/Goldziher/ai-rulez/issues/352)).

## [5.0.2] - 2026-10-11

### Added

- **Profiles select domains with `*` and `!name`.** A profile entry may be a glob (`*`, `backend-*`) or a negation (`!legacy`) as well as an exact domain name; a profile that is only negations is an error, and `profile add` accepts the same spellings ([#324](https://github.com/Goldziher/ai-rulez/issues/324)).
- **`[claude] claude_md = "none"` writes `AGENTS.md` without the `CLAUDE.md` import shim**, for projects that want Claude Code to read `AGENTS.md` alone. The default stays `"shim"` ([#317](https://github.com/Goldziher/ai-rulez/issues/317)).
- **`AR9Q0 claude-md-shadows-agents-md`** reports a hand-written `CLAUDE.md` that shadows the generated `AGENTS.md`; opt in with `[lint] agents_md_only` (allow list: `agents_md_only_allow`) ([#318](https://github.com/Goldziher/ai-rulez/issues/318)).
- **`AR9R0 served-skill-bundled-files` and `AR9R1 both-dmi-pointer`** warn when a `served` skill ships `scripts/` that are never written to disk, and when static content points at a `delivery: both` skill whose frontmatter disables model invocation (the Skill tool refuses it). The served-skill docs recommend `delivery: both` when a skill runs bundled scripts ([#336](https://github.com/Goldziher/ai-rulez/issues/336), [#337](https://github.com/Goldziher/ai-rulez/issues/337)).
- **`doctor`, and `validate --strict --mcp-user-scope`, report MCP servers shadowed in a harness user scope** with a different endpoint, naming both scopes and the removal command, and generated servers that differ only by name while pointing at the same url ([#330](https://github.com/Goldziher/ai-rulez/issues/330)).
- **`[[mcp_servers]]` expresses an explicit `type` and a pre-registered OAuth client.** A hosted server takes `oauth = { client_id, callback_port, scopes }`, rendered in `.mcp.json` for Claude and VS Code and as a TOML `oauth` table for Codex, and skipped with a warning by a harness that cannot express it. `AR602` warns when a hosted server names no `client_id`, or when one name resolves to two endpoints across sources ([#334](https://github.com/Goldziher/ai-rulez/issues/334)).
- **`tokens --format json` carries a stable `always_loaded` key**, and its diagnostics (including the over-budget line) go to stderr so stdout stays one parseable document ([#340](https://github.com/Goldziher/ai-rulez/issues/340)).
- **The `baz` preset's nested surface is controllable.** `[rules] baz_default = "none"` nests only items whose `targets` select `baz`, `baz_dedupe = true` writes one shared parent `AGENTS.md` when several globs share a parent, and `baz_budget` warns above a byte budget. Every default is unchanged ([#341](https://github.com/Goldziher/ai-rulez/issues/341)).
- **`[plugin] include` bundles the files a plugin's MCP command runs.** Globs relative to the content root are copied verbatim into the Agent Plugins package and recorded in `.ai-rulez-generated.json` with content hashes, so `verify --plugin` and `publish --emit agent-plugins` count them and `AR9O2` stops firing once a `./` command resolves to an included file ([#349](https://github.com/Goldziher/ai-rulez/issues/349)).

### Performance

- **The rendered serve catalog is cached on disk.** `ai-rulez mcp --serve-skills` re-rendered every served skill on each start; the rendered catalog is now cached (HMAC-authenticated, re-admitted on restore, and guarded against symlinks and oversized files), so a warm start is about 0.2 s ([#320](https://github.com/Goldziher/ai-rulez/issues/320)).
- **The per-file security scan runs once across a worker pool.** `validate` and the `generate` secret gate scanned every file serially and re-read the same file once per caller; the scan now runs once per file across `GOMAXPROCS` workers, each file is read once per run, and content reached by more than one item is scanned once. Findings, their order and every exit code are unchanged. On the lint benchmark the full run drops from 110 ms to 45 ms and the security analyzer from 71 ms to 15 ms ([#312](https://github.com/Goldziher/ai-rulez/issues/312)).
- **`generate` no longer runs the whole security family before writing.** Its secret gate read only AR001 but ran every security rule (shell, injection, hidden characters, hosts, blobs) over every file; it now runs the credential patterns alone, with the same AR001 findings. On a 590-file project, `generate` CPU (user) fell from 1.4 s to 0.65 s. `validate` builds the split text of each file once per run instead of up to three times and skips body-scan patterns on lines that cannot match (user CPU 1.9 s to 1.6 s). Output is unchanged.

### Fixed

- **`generate --check` reports an orphaned generated file without the manifest.** A generated file whose source was removed was not reported when the gitignored `.generated-manifest.json` was absent (CI, a fresh checkout), because the orphan pass relied on it. `--check` now also reports any file carrying a Content-Hash header that no output emits ([#313](https://github.com/Goldziher/ai-rulez/issues/313)).
- **Progress frames are no longer written to a non-terminal stream.** With stdout/stderr redirected, `generate`, `validate` and `--check` wrote spinner and bar frames with ANSI codes and carriage returns into the output; the animated UI now draws only on a terminal, and `NO_COLOR` also suppresses it ([#314](https://github.com/Goldziher/ai-rulez/issues/314)).
- **`.mcp.json` entries have one shape.** A configured server was written with `"disabled": false`, a key Claude Code does not document for an active server, while the self-server entry had `"type": "stdio"`; `disabled` is now written only for a server that is turned off, and every stdio entry carries the same `type: stdio` ([#315](https://github.com/Goldziher/ai-rulez/issues/315)).
- **A skill, rule or agent file that cannot be loaded is reported.** A `SKILL.md` over the 8 MiB content limit (or any other read failure) was dropped from the output with no message from `generate` or `validate`; the scan now logs `failed to load skill file|content file|agent file` with the path, as it already did for commands.
- **AR002 accepts the emoji presentation selector after U+2139, U+203C, U+2049, U+3030 and U+303D** (`ℹ️`, `‼️`, `⁉️`, `〰️`, `〽️`). They are letter or punctuation bases, which the rule rejected as a possible hidden payload.
- **`migrate v5` pins `[mcp] self_server = false`**, so a migrated project no longer gains the ai-rulez MCP server in `.mcp.json` (once per nested root in a monorepo) until it opts in.
- **One managed `.gitignore` block for a recursive run.** A recursive `generate` wrote the block into every nested root; it is now written once at the git toplevel, and an explicit `gitignore = false` is warned about rather than overridden ([#316](https://github.com/Goldziher/ai-rulez/issues/316)).
- **A plugin-only root no longer writes a header-only instruction file.** A nested root that declares only `[plugin]`/`[[plugin.mcp]]` but keeps a preset wrote a `CLAUDE.md`/`AGENTS.md` holding nothing but the generated header; the root file and the shared `AGENTS.md` are now skipped with a warning naming the file and the preset ([#327](https://github.com/Goldziher/ai-rulez/issues/327)).
- **`installed_skills` and `skill_sources` honor `include`/`exclude`.** An entry that selects skills by glob still resolved its root as a single skill directory, so `installed_skills` errored with `expected SKILL.md at skills/<name>` and `skill_sources` served nothing; when `include`/`exclude` is set and no path is given the entry is now a directory of skill directories, and a glob that matches nothing is reported ([#329](https://github.com/Goldziher/ai-rulez/issues/329), [#331](https://github.com/Goldziher/ai-rulez/issues/331)).
- **`AR301` and `AR990` share one reference matcher.** `AR301` treated a backticked term followed by the plural noun "skills" as a pointer, so prose failed validation; `AR990` only saw backticked names, so the documented non-backticked pointer forms (`reference: skill x`, `skills/x`, `.claude/skills/x/SKILL.md`, `subagent_type: x`) went unreported. One matcher now backs both ([#335](https://github.com/Goldziher/ai-rulez/issues/335), [#339](https://github.com/Goldziher/ai-rulez/issues/339)).
- **`AR990`, `AR992` and `AR402` no longer misfire.** `AR992` reported a preset that writes no skills tree (claude+baz) on every generate; `AR990` saw a bare backticked served-skill name but not a `load_skill` pointer in the same file; and `AR402` rejected a `references/` path that names a sibling skill's file. Each is now consistent with its own rule ([#342](https://github.com/Goldziher/ai-rulez/issues/342), [#343](https://github.com/Goldziher/ai-rulez/issues/343), [#346](https://github.com/Goldziher/ai-rulez/issues/346)).
- **`AR990` fires only for always-loaded content.** A served skill naming a served sibling is not reported ([#319](https://github.com/Goldziher/ai-rulez/issues/319)).
- **`AR201`/`AR401` resolve names from a scope's emitted directory**, so a container image tag in a scope is no longer a false positive ([#325](https://github.com/Goldziher/ai-rulez/issues/325)).
- **`validate --recursive` reports findings in its summary and counts the same roots as `generate --recursive`.** It printed "All N config(s) are valid" while exiting 2, and counted a different root set ([#338](https://github.com/Goldziher/ai-rulez/issues/338)).
- **`validate` names the nested roots a non-recursive run skips** ([#326](https://github.com/Goldziher/ai-rulez/issues/326)).
- **Plugin `${PLUGIN_ROOT}` is rewritten in a plugin's `args` and `env`, not only its `command`** ([#322](https://github.com/Goldziher/ai-rulez/issues/322)).
- **`generate` overwrites a committed output in a fresh worktree without `--force`.** With no `.generated-manifest.json`, a bundled skill asset ai-rulez had written was refused as a file "ai-rulez did not write"; a raw output below a directory this run renders a skill file into is now treated as ai-rulez's own copy, while a hand-written file and a symlink stay refused ([#332](https://github.com/Goldziher/ai-rulez/issues/332)).
- **Stale generated files are pruned and reported without a manifest.** A removed agent's `.claude/agents/<name>.md` stayed on disk and `--check` exited 0; the retired-file set now includes every header-carrying orphan the render no longer emits, shared by `generate` and `--check` ([#333](https://github.com/Goldziher/ai-rulez/issues/333)).
- **Bundled skill assets are tracked in the manifest and reported as orphans.** A skill's `scripts/` copy carries no header, so a committed manifest now prunes one whose source is gone and `--check` reports it; with no manifest a header-less asset is left alone ([#328](https://github.com/Goldziher/ai-rulez/issues/328)).
- **`tokens` counts the `AGENTS.md` a canonical `CLAUDE.md` imports.** With `agents_md = true` the always-loaded total counted the import shim but not the `AGENTS.md` it loads, understating a budget gate; a runtime whose preset imports `AGENTS.md` now counts that file once ([#347](https://github.com/Goldziher/ai-rulez/issues/347)).
- **A top-level key written after a `[table]` header is a load error, not a silent no-op.** TOML read `agents_md`, `builtins` or `gitignore` after `[rules]` as keys of that table and ignored them, so the settings stopped applying with only a warning; loading now names the key, the table and the fix ([#344](https://github.com/Goldziher/ai-rulez/issues/344)).
- **A generated file left by a removed nested root is reported and pruned.** `generate --check --recursive` said up to date when a whole nested root was deleted and its `CLAUDE.md`/`.claude/rules/*` remained; the manifest-free orphan pass now walks outside the current output roots, so `--check` reports it and `generate` removes it ([#345](https://github.com/Goldziher/ai-rulez/issues/345)).
- **A key unknown to a nested table is no longer reported as a misplaced top-level key.** `description` in a `[[plugin.mcp]]` entry was told to "move it above the first [table]", which would change its meaning; the hint now applies only where TOML really can have swallowed a top-level key ([#348](https://github.com/Goldziher/ai-rulez/issues/348)).

### Documentation

- **Corrected the Claude Code rules-folder nesting claim** ([#323](https://github.com/Goldziher/ai-rulez/issues/323)).

## [5.0.1] - 2026-10-11

### Added

- **Compose roles.** `--role` takes a comma-separated list, as do `roles show`, `roles resolve`, `tokens --role` and `mcp --serve-skills --serve-role`. A composition is the union of its members: the domains and items they keep are combined, and where two members set `skill_mode` or `delivery` for the same skill the more specific key wins and, on a tie, the role listed last. A composition is pinnable — `ai-rulez lock --role a,b` records the union under the canonical name `a,b`. See [Roles](https://goldziher.github.io/ai-rulez/roles/).
- **`AI_RULEZ_PROFILE` and `AI_RULEZ_ROLE` select the active profile and role** when the matching flag is not given; an explicit flag always wins ([#309](https://github.com/Goldziher/ai-rulez/issues/309)).
- **A `domain` field on `[[mcp_servers]]`, `[[installed_skills]]` and `[[skill_sources]]`** ties a remote or fetched resource to a domain, so a profile or a role selects it by domain exactly as it selects local content. `profiles` keeps working and is an additional filter ([#308](https://github.com/Goldziher/ai-rulez/issues/308)).
- **`installed_skills` gains the `skill_sources` conveniences**: `include` and `exclude` select several skills from one repository with a single entry, `name_prefix` namespaces their names, and `trust` sets the repository-scoped scan level ([#310](https://github.com/Goldziher/ai-rulez/issues/310)).

### Fixed

- **A kept obsolete plugin file is no longer forgotten after regeneration.** An obsolete generated file you edited by hand is preserved with a warning, and `verify --plugin` now keeps failing while it exists, as documented, instead of passing once the provenance sidecar is rewritten ([#307](https://github.com/Goldziher/ai-rulez/issues/307)).

### Documentation

- Answer how [profiles and roles](https://goldziher.github.io/ai-rulez/roles/#profiles-vs-roles) differ, how [installed skills and skill sources](https://goldziher.github.io/ai-rulez/installed-skills/#installed-skills-vs-skill-sources) differ, and how to [compose roles](https://goldziher.github.io/ai-rulez/roles/#composing-roles).

## [5.0.0] - 2026-10-10

Run `ai-rulez migrate v5` (`--dry-run` to preview, `--check` for CI); upgrade steps are in [Migrating to v5](https://goldziher.github.io/ai-rulez/migration-v5/).

### Positioning

v5 repositions ai-rulez as a standards-compliant lifecycle tool for agent knowledge and capabilities: author in one source of truth, generate for every harness, bundle and publish to the open formats, lint and validate against each standard's own schema, and govern through signing, approvals and policy. The README, the documentation site (organized as Author, Generate, Bundle and publish, Validate, Govern, Operate) and the package metadata follow that story. "Planned" means the code does not yet generate and validate the standard. Supported today: OKF (Open Knowledge Format v0.2), Agent Plugins, ARD, Agent Skills, AGENTS.md, llms.txt, MCP server config, CycloneDX/SPDX SBOM, in-toto/DSSE/Sigstore and OpenTelemetry (OTLP); the MCP server card and "agent bundle" are planned. Pinned spec versions and conformance status are in [Standards](https://goldziher.github.io/ai-rulez/standards/).

### Breaking

- **Config format is v5.** A `version = "5.0"` project is required; a 4.x config is rejected with `run ai-rulez migrate v5` and a 2.x/3.x config tells you to install ai-rulez 4.x first. YAML/JSON configs (`config.yaml`, `config.json`, local variants) and separate `mcp.toml`/`mcp.yaml`/`mcp.json` files are no longer read — `migrate v5` converts them to `config.toml` and `[[mcp_servers]]`. `migrate v4` and `init --format yaml|json` are removed, and `migrate` now has real subcommands (`migrate v5`, `migrate okf`; the `migrate 5` / `migrate v5.0` spellings are gone). See [Migrating to v5](https://goldziher.github.io/ai-rulez/migration-v5/).
- **Go module is `github.com/Goldziher/ai-rulez/v5`**: install with `go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest`. npm, PyPI and Homebrew are unaffected.
- **One flag taxonomy.** The only shorthands are `-C -D -T -q -y -n -o`; `-n` is `--dry-run` everywhere and `--config-dir` has no shorthand (declared once, globally). `--out` is `--output` (a file) or `--output-dir` (a directory); `generate --strict` is `--strict-config`, `lock --strict` is `--refuse-findings`, `sign --policy` is `--org-policy`; `--json`/`-j` is `--format json`; `generate --no-fetch` is `--offline`; the confirmation skip of `clean` and the `remove` commands is `--yes`/`-y`. No command takes a config path argument any more: use `ai-rulez -C <path> validate`. A removed spelling fails with a hint naming its replacement.
- **Removed aliases and flags**: `g`, `clear`, `v`, `check`, the hidden `mcp --transport`/`--address`/`--port`, `--verbose`/`-V`, and the dead `generate --update-gitignore`, `--no-configure-cli-mcp`/`-M` and `--skip-cli-mcp`/`-S`. Unprefixed `DEBUG`, `QUIET` and `VERBOSE` no longer change the CLI; use `AI_RULEZ_DEBUG`/`AI_RULEZ_QUIET`. `guard` is now listed in `--help`.
- **Command results go to stdout, diagnostics to stderr.** `list`, `domain|profile|include|skill list`, `clean --dry-run` and `lock --check` print their result on stdout; `-q` hides only progress and success lines. The content commands (`add`, `remove`, `domain`, `profile`, `include`, `skill`) print the created/removed/rewritten path on stdout, take `--format json`, honor the global `--config-dir`/`-C`, and gain the new `show` and `edit` verbs; `add check` takes `--severity`, `--tools` and `--targets`. `ai-rulez --version` and `version` print to stdout, and a group command with no subcommand prints help and exits 0 (was 1).
- **One error rendering and one exit path.** Every failure prints `Error: ...` (then validation errors and a `Hint:`) on stderr and, under `--format json`, a `{"status":"error", ...}` document on stdout. Exit codes: `0` ok, `1` could not run, `2` findings/drift/failed gate, `3` (`lock` only) served skills left unpinned. A declined or non-interactive confirmation exits 1; prompts go to stderr.
- **Report commands share one flag vocabulary and one JSON contract.** `sbom --format cyclonedx|spdx-json` is `sbom --type`; `--format` is `text|json`; `telemetry hook --format json|toml` is `--syntax`; `eval run` prints `text` by default (`--format markdown` keeps the old output). Every `--format json` document carries `schema_version`, and lists return `{"schema_version":1,"items":[...]}` instead of a bare array. Each report has a published schema under `schema/` (table in `docs/cli.md#json-contracts`).
- **`verify` checks attestations and provenance only.** Drift is `generate --check`, which re-renders and also catches hand edits and deleted files; a bare `ai-rulez verify` exits 1 and names it.
- **`validate` runs the content checks by default.** `--config-only` restores the old config-only run; `--strict` now means warnings fail (`--fail-on warning`), so `validate` can exit 2 where it exited 0. Malformed YAML frontmatter is a finding, `AR306 frontmatter-malformed` (`generate` still exits 1), and a configuration that loosens the organization policy (`AR740`) exits 2 from `generate`, `validate`, `lock`, recursive runs and `doctor` instead of 1.
- **`generate --locked` and `--frozen` fail closed without an `ai-rulez.lock`** (exit 2, hint to run `ai-rulez lock`).
- **`clean` keeps generated files you edited by hand** unless `--include-edited` is passed; `--yes` only skips the confirmation prompt.
- **`usage ...` and `report usage|evals` fold into `telemetry ...`** with no aliases (`telemetry hook|record|feedback|report [evals]`); `[lint.budget]`/`[lint.tolerate]` are renamed `[lint.ratchet]` (JSON `ratchet_exceeded`), while `[lint.budgets.<kind>]` size limits keep their name.
- **New defaults.** `agents_md = true`: `AGENTS.md` is canonical and `CLAUDE.md` imports it, with `agents_md = false` for per-harness files. Generated headers carry only the per-file `Content-Hash` (`[header] hashes = "full"` restores `Source-Hash`). The managed `.gitignore` block is opt-in (`gitignore = true` or `generate --gitignore`).
- **`liter-llm` is the only model backend and builds need cgo.** Every model call goes through [liter-llm](https://github.com/xberg-io/liter-llm) 2.2.0, linked statically, so ai-rulez is built with `CGO_ENABLED=1` and a C toolchain; release binaries grow about 35 MB. The `[llm] backend` key and `AI_RULEZ_LLM_BACKEND` are gone (an OpenAI-compatible gateway is a `base_url`). Build from source with `task setup:llm` once.
- **Stricter names and values for content, the CLI and the MCP tools.** A name must not end in `.md`, contain whitespace or start with `.` or `-`; `--targets` rejects unknown words; `remove` checks the item exists; `domain remove` refuses while a profile lists the domain; `add skill` without a description scaffolds one that passes `validate`.
- **Locks.** `[lock] enforce` defaults to true whenever `ai-rulez.lock` exists; every pin uses one hashing scheme (sha256, algorithm-prefixed Merkle tree) and scripts are hashed byte for byte; a lock without content pins fails `--check`; a lock that pins role outputs is written `version = 2`. Locks written by earlier v5 builds read as stale once — run `ai-rulez lock`.
- **Remote sources.** Plain `http://` and `git://` are rejected (use `https://`, `ssh://`/`git@host:path` or a local path); the git token (`AI_RULEZ_GIT_TOKEN`, `--token`) is sent only to `github.com` or the hosts in `AI_RULEZ_GIT_TOKEN_HOSTS`, as a host-scoped header rather than inside the URL.
- **A committed config cannot reach outside the project**: a local include, skill-source path, custom preset `path`, provider spec path, `okf.dir` or `marketplace.output_dir` that escapes it (or names `.git`/`.ai-rulez`) is refused, and a `file://` include or skill source must name a repository inside the project (override with `config.local.toml`, the user config or `AI_RULEZ_ALLOW_FILE_URLS=1`). Content symlinks in the project's own `.ai-rulez/` are followed only when the target is inside the project; `generate` refuses symlinks that leave it or point into `.git` or credential files and stops on `AR001` secrets.
- **Security scanning.** `[lint.security] scan_imports` is on by default (`generate` scans imported content and stops at error-level findings; `AR012` is an error under `[lock] enforce`); `ai-rulez scan` reports only the `security` analyzer; `lock` and `update` scan what they pin and refuse error findings unless `--accept-findings`; staged `[[lint.external]]` scanners run confined by default (`isolation = "auto"`, no network unless `egress = true`).
- **`init --from` runs through `convert --write`** with the same scan, validation and lossiness report; an existing config directory is moved aside and restored if the import fails.
- **Preset changes.** `windsurf` is renamed `devin` (the preset, its `.windsurf/` output directory and the `windsurf_model` frontmatter key; there is no alias — rename in `config.toml` and agent files and delete the old outputs), and the `continue-dev` preset is removed with no replacement (`doctor` reports both). `[[plugins]]` no longer writes `.claude/plugins.json` or `.codex/plugins.json` (no tool reads them; `generate` warns).
- **Claude Code MCP servers are written only to `.mcp.json`**, so resolved MCP env values do not land in a committed settings file; an entry an earlier version wrote into `.claude/settings.json` is removed on the next `generate`/`clean`.
- **User-scope and signed data.** `[telemetry] service_name` is user scope only (a repository value is ignored and reported as `AR9K1`). Eval results are signed per user (`eval-results.key`): an unsigned or edited record is `unverified`, re-run and ignored, and CI must use `eval run --force`. The usage log is version 3 (`digest`, `digest_scheme`, `event_id`, salted `session`), and `schema/catalog.schema.json` describes catalog version 2.
- **MCP tools are stricter.** Arguments are validated against typed schemas (`additionalProperties: false`), so an unknown argument, wrong type, out-of-enum or missing required argument is an error result instead of being coerced; `validate_config` and other failures return `isError: true` instead of a success document; `working_directory`, `config_file` and `config_dir` are confined to the server's start directory (`mcp --root <dir>` to change it, `mcp --allow-any-dir` to lift it); the authoring server no longer advertises `tools.listChanged`; `roles resolve --format json` spells the key `honored`.
- **`[mcp] self_server` defaults to `true`**: `generate` adds the ai-rulez MCP server to `.mcp.json` (and each harness's own MCP file); opt out with `[mcp] self_server = false` or `generate --no-self-mcp`.
- **`improve pr` confines what it runs** (scrubbed git environment, no hooks, only the push keeps transport credentials; `--isolation` cuts the network for the worktree `generate`/`lock` and `--allow-network` leaves it on), and the scheduled repair workflow template pins its actions to a commit SHA.

### Added

- **OKF (Open Knowledge Format, spec v0.2).** `export okf`, `import okf` and `okf validate` read and write deterministic bundles, `[[includes]] format = "okf"` uses one as a source, the opt-in `okf` preset keeps a bundle in sync with `generate`, and the `.ai-rulez/` tree itself can be an OKF bundle (`migrate okf`). See `docs/okf.md`.
- **llms.txt** via the `llms-txt` preset (`llms.txt` and optional `llms-full.txt`), linted by `validate --strict`. See `docs/llms-txt.md`.
- **Agent Plugins (agent-plugins.org) packages.** `generate --plugin` writes a portable `plugin.json`, `skills/` and `mcp.json` conformant to the published 1.0.0 specification (or the 1.1.0 draft via `[plugin] spec`), and the `copilot` runtime and the Codex root manifest write the same package. `validate --strict` loads it as a conformant client would (`AR9O0`-`AR9O5`), `verify --plugin` compares it against its sources, `publish --emit agent-plugins` emits a clean directory and `convert --from agent-plugins` imports one. See `docs/agent-plugins.md`.
- **Agentic Resource Discovery (ARD)** manifest (`[ard]`, `publish emit ard`) and **Pi** packages (`[plugin] runtimes = ["pi"]`). See `docs/ard.md`, `docs/plugins.md`.
- **New MCP tools** for what the CLI already did (`scan_content`, `token_report`, `cost_report`, `sbom`, `okf_validate`, `approvals_status`, `policy_show`, `list_verifiers`, `list_builtins`, and agent/command CRUD); `generate_outputs` takes `profile`/`role`/`check`/`offline`; the authoring server gains prompts (`author-skill`, `add-rule`, `review-config`, `trim-context`), resources (`ai-rulez://config`, `ai-rulez://catalog`, item URIs) and cancellation. Approving, signing, locking, evaluating and publishing stay CLI-only.
- **Shell completion** of enum values and project names (rules, context, skills, agents, commands, checks, domains, profiles, roles, includes), and a generated command reference (`docs/cli-reference.md`) with examples for every command.
- **Dynamic skills and roles.** `delivery: static|served|both` lets `mcp --serve-skills` serve skills on demand with `find_skill`, `load_skill` and `list_skill_resources`; `[[roles]]` maps a job to a slice of the content with `generate --role`, `roles list|show|resolve` and a generated `roles.json`; skills can be served from a git repository or directory via `[[skill_sources]]`, pinned in the lock. See `docs/mcp-server.md`, `docs/roles.md`.
- **Content lock.** `ai-rulez.lock` pins the version, every authored item, the generated outputs, remote sources and served skills, with `lock --check`, `lock --diff`, `lock --content-only`, `lock --subject`, `--locked`/`--frozen` and version-constraint pins moved by `ai-rulez update`. See `docs/lockfile.md`.
- **`checks` content kind**: code-review guidelines in `.ai-rulez/checks/` rendered for the review tools that read a repository file, with a marker block that keeps your own text. See `docs/checks.md`.
- **`generate --watch`** live regeneration, and **`generate --emit-plan FILE`** for a deterministic plan of every file.
- **`ai-rulez doctor`**: read-only diagnostics (removed presets, drift, unresolved MCP placeholders, lock drift, missing tools), exit 2 on errors.
- **Hooks and permissions across harnesses.** Top-level `[[hooks]]` and `[permissions]` are translated into each harness's native format and merged into your settings files key by key, so hand-written entries survive; matchers are translated by tool name; plugin-based harnesses (OpenCode, Kilo, MiMo Code, Pi, Amp) get a generated hook module. See `docs/settings.md`, `docs/permissions.md`.
- **User-level output.** `generate --user` renders a user config into each preset's per-user locations and `clean --user` removes exactly that. See `docs/user-scope.md`.
- **`validate --strict`**: deep content validation with stable `AR` codes, report formats (`text|json|sarif|github|junit|markdown`), baselines, budgets/ratchet, `--fix`, `--since`/`--changed`, `--analyzer`, a risk score and `--explain`. See `docs/strict-validation.md`.
- **Security checks `AR001`-`AR011`** in strict validation and the dedicated `ai-rulez scan` command (secrets, hidden characters, prompt injection, credential access, unrestricted tools, an outbound host allow-list and merged `[[lint.external]]` results), plus harness trap lint. See `docs/harness-traps.md`.
- **Reports and analysis**: `tokens` (prompt-token cost by load bucket, including the item listing), `cost` (context-cost offenders), `catalog` (`--format json` and a self-contained `--html` site), `sbom` (CycloneDX 1.6 and SPDX 2.3) and `search` (lexical plus an optional hybrid embeddings index). See `docs/sbom.md`, `docs/search.md`.
- **Governance.** `approve` records reviewer approvals in the lock bound to the content digest, with `[governance]` policy (approvers, `min_approvers`, `CODEOWNERS`, expiry, sign, `approve --from-github-review`). `sign` writes Sigstore bundles (key, KMS or keyless) for the lock, bundles, skills, SBOM and policy, and `verify --attestation` checks them against `[signing]`; `verify --self` verifies the running binary against its release provenance. An organization policy from outside the repository (`--policy`, the managed path, `--discover-org`) bounds sources, lint, locking, governance, signing, scanners, MCP, hooks and network use, and `validate --show-policy` prints the effective policy. `publish` preflights, builds a reproducible archive and releases to GitHub, npm or OCI with `--execute --yes`. See `docs/signing.md`, `docs/policy.md`, `docs/publish.md`.
- **`review`** scores items against a rubric (optional LLM judge with `--semantic`, calibration and a gate), and **`improve`** (experimental) runs an external optimizer behind a held-out eval gate. See `docs/review.md`, `docs/improve.md`.
- **`eval run`** executes harness-neutral skill eval cases (pass rate, trigger precision/recall, ablation, token cost) with adapters and a result cache. See `docs/evals.md`.
- **Telemetry** (opt-in, identifier-only): `telemetry hook|record|flush|doctor` and an OTLP/HTTP export through a bounded offline outbox, plus `report usage` sections for rules, agents and context. See `docs/telemetry.md`.
- **`convert`** reads existing tool files into an equivalent `.ai-rulez/` tree with a lossiness report (native, rulesync, skills-lock, APM, Tessl and OKF importers), and `init --from` uses it.
- **`pkg/airulez`** (experimental): load, validate, plan and apply a project from Go with no working directory, environment, clock or subprocess. See `docs/embedding.md`.
- **Preset and plugin outputs**: the `baz` preset; native MCP, command and user-scope outputs for many existing presets; per-domain plugins and marketplaces (`[marketplace]`); `[placement]` to keep skills and commands plugin-only; `.codex/config.toml` merged as a document. See `docs/plugins.md`.

### Changed

- `publish --to npm` preserves the runtime metadata (Pi resources, OpenCode entry points, dependencies, keywords, license) of the package it publishes while owning the name, version, `files` list and `publishConfig`.
- `validate_config` and `scan_content` show project-relative paths and carry the whole lint document.
- `generate` validates `config.toml` against the schema and warns about unknown keys (`--strict-config` or `AI_RULEZ_STRICT=1` fails), and prints a summary of new or changed commands (hook commands, MCP servers, managed env, plugin enablement, permissions) even with `--quiet` (`--yes` or `AI_RULEZ_ACK_COMMANDS=1` silences it).
- `clean` keeps a generated file whose body was edited by hand and warns; `generate --dry-run` marks current files `unchanged:` and edited files `edited:`.
- The lock and `generate --check` return exit codes per run, so `3` works with `--recursive`; a served skill the security scan refuses is reported and left unpinned (`lock` exits 3) unless `lock --refuse-findings`.
- `tokens` counts the item listing for harnesses that list skills, commands or agents, so `--budget` gates the number the prompt really carries.
- `liter-llm` is pinned at v2.2.3, and the LLM response cache is version 3.
- Several presets write their native MCP, command and agent shapes more accurately (for example `opencode` uses `mcp.<name>` with `enabled`, `codex` writes skills to `.agents/skills`, the root `.mcp.json` follows the `mcp` preset shape, and `antigravity` writes commands as skills).

### Fixed

- `migrate v5` drops the rejected 4.x `[[skills]]` table, keeps CRLF in `config.toml` and preserves comments; `migrate okf` writes atomically with a backup and refuses to destroy a rule named `index` or `log`.
- `validate` on an OKF bundle reports the OKF verdict before "Configuration is valid" and writes the OKF document under `--format json`; `-q` no longer prints the OKF summary or strict totals.
- `generate --check` no longer reports a file `generate` just wrote as `edited`.
- Content is never read, written or deleted through a symlink inside the configuration directory; `add`, `edit`, `show`, `remove` and the MCP CRUD tools refuse symlinked domains, directories and files.
- `edit` and the MCP update tools keep an item's priority, targets, description and other frontmatter when only the content changes.
- No log line or error prints a credentialed include or skill source; `include add` and `skill install` warn and redact.
- `export okf --output-dir` can no longer delete your configuration, and `import okf` strips group/world-write bits and redacts credentials; OKF export, import, export is byte-identical.
- Locking fails closed: an enforced lock fails when an include cannot be resolved, file modes in digests are OS-independent, and `validate --strict` reports `AR981` instead of skipping an unverifiable lock.
- Running inside a git hook no longer corrupts the repository (every git subprocess drops `GIT_DIR` and the other inherited git variables).
- `[llm]` trust and safety: `allow_network`, `base_url`, `api_key_env` and price overrides are honoured only from user scope, a pasted key is never echoed, the budget cannot be overshot by concurrent calls, and `timeout_seconds`/`max_cost_usd` reject unusable values.
- The `mcp --serve-skills` server bounds served-skill metadata, applies the session budget to `get_skill`/`read_skill_file`/`resources/read`, handles skill-source name collisions, and fetches a locked commit by SHA even after a tag moves.
- Hook merges read commented JSONC/TOML/YAML documents, and presets that share one document union their owned keys instead of conflicting.

### Removed

- The hidden `mcp --transport`, `--address` and `--port` flags (the server speaks stdio only).
- `ai-rulez migrate`, `init --format`, legacy `mcp.yaml`/`mcp.toml`/`mcp.json` loading and the V2/V3 config readers.
- The `compression` option (a no-op since v3.13) and the legacy `init --from` importer engine.
- The process-wide preset registry and other legacy Go APIs; `pkg/airulez` is the supported embedding API.

### Security

- MCP `--root` confinement runs on the symlink-resolved path it judged, so a link swapped between the check and the tool's use cannot redirect a call outside the root.
- The `run_verifiers` MCP tool never starts a program (command predicates are refused even with `AI_RULEZ_VERIFIERS_ALLOW_EXEC` set).
- The committed generated manifest no longer licenses anything: a forged claim cannot strip permissions, env or allow keys from your settings, and the local manifest stores array elements as digests rather than values.
- `ai-rulez.lock` no longer pins resolved secrets or the checkout path, and lock writes never follow a committed or linked symlink.
- Output paths cannot escape the project: a custom preset, provider spec, `okf.dir` or `marketplace.output_dir` can no longer make `generate` write outside the project or into `.git`, and symlinked output directories are not followed.
- The git token never leaves allowlisted hosts and never appears in process arguments or a cached clone's `.git/config`; include and skill caches are `0700` and keyed by name plus a URL hash.
- Imported content is scanned: symlinks in includes, installed skills and OKF bundles are skipped, files are size-capped, a local `[[skill_sources]]` path must resolve inside the project, and credentials in a source URL are refused (`AR035`).
- Machine-local files are never written or read through a symlink, and the LLM cache secret must be a regular `0600` file.
- Hook `script` paths are validated and shell-quoted, permission translations do not widen or bypass rules, and hook plugin matchers that Go and JavaScript read differently are skipped with a warning.

## [4.24.2] - 2026-10-04

### Fixed

- Internal/test-only fixes; no user-visible change.

## [4.24.1] - 2026-10-04

### Fixed

- Internal/test-only fixes; no user-visible change.

## [4.24.0] - 2026-10-04

### Added

- **`pi` preset** for the [pi](https://pi.dev) coding agent: a shared `AGENTS.md`, skills in `.agents/skills`, agents in `.pi/agents`, and MCP servers in `.pi/mcp.json`.

### Changed

- `AGENTS.md` is now shared by `pi` alongside `codex`, `opencode`, `xum` and `amp`.

## [4.23.1] - 2026-10-03

### Added

- Xum stdio MCP `env` is written as a shell assignment prefix.
- `generate` warns once about a v1-shaped OpenCode plugin (OpenCode v2 does not run it).

### Changed

- **MCP servers in shared settings documents are owned one by one**, so a server you wrote by hand survives `generate` and `clean`.
- A plugin `name` must match `^[a-z0-9][a-z0-9._-]*$` and contain no `..` for every runtime.

### Fixed

- Local content reaches each tool through a file it loads (`GEMINI.local.md`, `AGENTS.local.md`, `AGENTS.override.md`, `.junie/rules/ai-rulez.local.md`, ...); the old non-working names are removed on upgrade.
- `clean` leaves no ai-rulez keys behind in hand-authored settings documents.

## [4.23.0] - 2026-10-03

### Added

- **`config.local.*` overlay** and `ai-rulez local` for machine-local configuration, plus local skills, agents, commands and domains under `.ai-rulez/local/`.
- **`agents_md`**: renders one shared `AGENTS.md` and `.agents/skills` tree for many presets.
- A shared baseline and drift guard: a teammate-view run never deletes your local outputs, and drift on a tracked shared file stops generation unless `--allow-local-drift`; `--no-local` renders the teammate view.

### Changed

- `.gitignore` management adds only what git does not already ignore.

### Fixed

- Gemini agents are written to `.gemini/agents/`, the location Gemini CLI loads.
- Xum disabled stdio MCP servers are written as disabled.

### Security

- Generated files that contain resolved MCP secrets are written `0600`.
- Include and skill sources are logged with URL credentials redacted.

## [4.22.2] - 2026-10-03

### Fixed

- Generator schema v8 forces a one-time rewrite of existing generated files.
- Cursor agents are written to `.cursor/agents/`.
- OpenCode agent model, variant, `hidden`, `temperature` and `top_p` are written in the shapes OpenCode accepts (an unqualified model is omitted with a warning).

## [4.22.1] - 2026-10-03

### Fixed

- Cursor `globs` is written as a bare comma list.
- Negated globs (`!x`) are dropped from every dialect with a warning.
- `AGENTS.md` is identical across `codex`, `opencode`, `xum` and `amp`.
- Rules no longer vanish when a rule file name is taken (written as `<id>.ai-rulez.md`), and names that map to the same file id no longer abort generation (later ones get a stable suffix).

## [4.22.0] - 2026-10-02

### Added

- Rule `activation` frontmatter (`always`, `glob`, `auto`, `manual`), plus `[rules] mode` and `mode_by_preset`.
- Copilot path-specific instructions (`.github/instructions/*.instructions.md`).
- Local rules written as native rule files.

### Changed

- **BREAKING: rules are split into native rules folders by default** (`[rules] mode = "split"`): Claude writes `.claude/rules`, Copilot `.github/instructions`, Junie `.junie/rules` and Antigravity `.agents/rules`. Set `[rules] mode = "inline"` (or `mode_by_preset`) for the old layout.
- Frontmatter `targets` now restrict rule outputs, and scoped rule files go to the root rules folder with the scope path as a prefix.

### Fixed

- Include and skill-source URLs no longer leak credentials into logs or errors.
- Cline, Continue and Windsurf rules honour `paths`/`globs`; Cursor context rules are applied automatically.
- Generated rule files are gitignored per file, and a hand-written rule file is never overwritten.

## [4.21.0] - 2026-10-02

### Added

- **`headers` on `[[mcp_servers]]`**: remote servers can send HTTP headers (`Authorization = "Bearer ${TOKEN}"`).
- **`[header] hashes`** (`full`, `content`, `none`) chooses the freshness lines; `content` stops one edit from rewriting every generated file.

### Fixed

- The secret guard now also covers `opencode.json`.

## [4.20.1] - 2026-10-02

### Fixed

- CRUD commands and the MCP CRUD tools work in a project using the `.config/ai-rulez/` layout.

## [4.20.0] - 2026-10-02

### Added

- **`[mcp] self_server`**: `generate` can add ai-rulez's own MCP server to the project `.mcp.json`.
- **`validate --recursive`** (`-r`).

### Fixed

- `generate --recursive` exits non-zero when any root fails.
- `clean` no longer removes a merged settings document that holds content ai-rulez did not write.
- `validate --quiet` is honored.

## [4.19.0] - 2026-10-02

### Added

- Configuration discovery also accepts `.config/ai-rulez/` (the [`.config` proposal](https://github.com/pi0/config-dir)) as a fallback; `.ai-rulez/` still wins when both exist.

### Changed

- Generated headers name the real config directory.

## [4.18.0] - 2026-10-01

### Added

- **`[header] text`** replaces the generated banner with your own multi-line text while keeping the freshness lines.

### Fixed

- Rules and context render in priority order again (critical to minimal).
- Regeneration is forced once after the ordering change.
- A generated `opencode.json` stays a managed artifact (it owns `$schema`).

## [4.17.0] - 2026-09-30

### Added

- **Path-scoped rules** via `globs`/`paths` frontmatter (Claude `.claude/rules/<id>.md`, Cursor `.mdc` with `alwaysApply: false`).
- Cursor rule frontmatter (`alwaysApply`/`globs`/`description`).
- Profile-scoped `[[mcp_servers]]` and `[[installed_skills]]` (`profiles` list).
- `defaults.omit_agent_fields`.

### Fixed

- Generated agents are spawnable as subagents (OpenCode `mode: all`, Xum `subagent.runnable`).
- Scoped outputs no longer repeat the root content.

## [4.16.0] - 2026-09-30

### Added

- **`${PROJECT_ROOT}`** can be used in an MCP server's `command` or `args`, resolved at generation time.

### Fixed

- `validate` checks the raw config against the JSON schema (unknown keys and bad enums are reported).
- `include add --merge-strategy` accepts the values the resolver uses.
- The `popular` pseudo-preset emits the curated provider set, and documentation was corrected.

## [4.15.0] - 2026-09-30

### Added

- **Profile-scoped builtin domains** (`builtin:<name>`, e.g. `builtin:docker` loads a pack for one profile only).
- **OpenCode v2 preset output** (`opencode.json` with `mcp.servers`) and an OpenCode v2 plugin adapter.

### Changed

- Merged JSON documents can own a nested key path (for example `mcp.servers`).

## [4.14.1] - 2026-09-29

### Fixed

- Internal/test-only fixes; no runtime behavior changed.

## [4.14.0] - 2026-09-29

### Added

- **`xum` preset** for the [Xum](https://xum.coder.com) coding agent (`AGENTS.md`, `.xum/skills`, `.xum/agents`, `.xum/mcp.jsonc`).
- **Provider-backed custom presets**: a `[[presets]]` entry may reference a declarative provider spec instead of a template.

### Fixed

- CRUD commands no longer write a spurious `config.yaml` into a TOML project (a mutation loses the comments of a hand-commented `config.toml`).

## [4.13.0] - 2026-09-27

### Added

- **`ai-rulez tokens`** reports the prompt-token cost of the configuration by load bucket (`always`, `conditional`, `on demand`, `unmodeled`), with `--budget`, `--compare-profiles` and a `--tokenizer`.
- **Composed profiles**: `--profile base,backend` (or `default = "base,backend"`) resolves to the de-duplicated union of their domains.

### Changed

- **The `Generated:` header line is now off by default** (output is byte-reproducible); `[header] timestamp = true` opts in.
- **Builtin rules are roughly halved**, with narrow guidance (code-quality, testing, token-efficiency, docker, observability) moved into skills that load on demand.

### Fixed

- Skills and commands now honour source precedence (root > domain > include > builtin); the previous inversion silently dropped root and domain items.
- `generate` removes the directories its own stale-file pass emptied.

## [4.12.1] - 2026-09-25

### Fixed

- `generate` no longer deletes H1-like lines from inside fenced code blocks.
- A merged settings document keeps its indentation and CRLF line endings.

### Changed

- Removed the unused `internal/scanner` package.

## [4.12.0] - 2026-09-25

### Added

- **Bundled hook scripts for plugins** (`script` field) plus the hook action `args`, `timeout`, `if` and `status_message`.
- **Directory form for commands** (`commands/<name>/COMMAND.md` with `references/`).

### Changed

- **Settings documents are merged, not overwritten** (`.claude/settings.json`, `.mcp.json`, `.gemini/settings.json`, `.agents/settings.json`, `.amp/settings.json`): ai-rulez owns specific keys and preserves the rest.
- `.gitignore` lists the subdirectories ai-rulez writes (`.claude/skills/`) rather than whole assistant directories.
- `init --setup-hooks` preserves comments, key order and indentation.

### Fixed

- Non-canonical skill/command subdirectories warn; `validate` reports skill/command output-id collisions; a plugin passthrough source outside the project (including through a symlink) is refused.

## [4.11.5] - 2026-09-19

### Fixed

- Skill includes pinned to a full commit SHA resolve deterministically and fail closed.
- Schema validation compiles with a fresh compiler per call.

### Changed

- Go toolchain to 1.27 and dependency upgrades.

## [4.11.4] - 2026-09-18

### Fixed

- The managed `.gitignore` block is spliced back in place instead of being reordered.
- `validate` fails on malformed content frontmatter, and a skill with malformed frontmatter is no longer dropped from output.
- Includes pinned to a full commit SHA fail closed instead of falling back to stale cache.

## [4.11.3] - 2026-08-24

### Added

- `[header] timestamp = false` omits the `Generated:` line.

### Fixed

- `generate` is reproducible across checkout paths.

## [4.11.2] - 2026-08-08

### Fixed

- `ai-rulez mcp` no longer wedges when a host sends `initialize` twice on one stdio session.

## [4.11.1] - 2026-07-31

### Fixed

- `generate` no longer inlines the full rules block into every generated skill and agent file.
- A skill whose source frontmatter fails to parse no longer emits a second raw frontmatter block.

## [4.11.0] - 2026-07-22

### Added

- **`ai-rulez clean`** removes the files produced by `generate` (the inverse of `generate`), with `--dry-run`, `--force`, `--keep-gitignore` and `--keep-manifest`, exposed as the `clean_outputs` MCP tool.

### Fixed

- MCP `update_rule`/`update_context`/`update_skill` no longer fail with `file already exists`.

## [4.10.0] - 2026-07-22

### Added

- **Local-override content** under `.ai-rulez/local/` (rules and context), emitted only to per-preset `.local` files and gitignored, with `--local` on `add rule`/`add context`.
- **Bare/flattened include layout**: included repositories no longer need an `.ai-rulez/` wrapper.

### Changed

- Builtin language, binding, OWASP and dependency conventions now emit as on-demand skills instead of always-inlined rules, shrinking the generated root files.
- The default header style is now `minimal`.

## [4.9.4] - 2026-07-12

### Changed

- Builtin language and binding rules are tool-agnostic (opinionated tools are framed as examples).

### Fixed

- The Python builtin no longer references a TypeScript type, and vite+ typos are fixed.

## [4.9.3] - 2026-07-12

### Fixed

- Windows drive-relative plugin paths (`C:outside`) are rejected on every platform.

### Changed

- Poly catalog npx/uvx execution paths are pinned for reproducible hooks.

## [4.9.2] - 2026-07-12

### Fixed

- Plugin path validation is consistent across operating systems.

## [4.9.1] - 2026-07-12

### Fixed

- Poly hook installation paths use the `version` subcommand.

## [4.9.0] - 2026-07-12

### Added

- Reusable `poly-hooks.toml` catalog with selectable hooks and guarded execution paths.
- Recursive plugin generation and verification (`--if-configured`).
- Plugin generation and verification hooks for Poly and pre-commit.

## [4.8.0] - 2026-07-12

### Added

- **Native Hermes Agent** generation (`hermes` preset, `.hermes.md`, project plugins and PyPI entry points).
- `plugin.content_root`, `plugin.hermes.source` and `plugin.hermes.requires_python`.
- `ai-rulez verify --plugin` for plugin freshness and provenance.

### Fixed

- Authored Markdown whitespace is preserved around generated headers.

## [4.7.0] - 2026-07-11

### Added

- Complete Codex plugin bundles (root MCP config, marketplace metadata, recursive skill resources).
- OpenCode plugin generation from `.ai-rulez/opencode/index.js`.
- Deterministic plugin provenance (generated-file headers and `.ai-rulez-generated.json` sidecars).

## [4.6.0] - 2026-07-08

### Added

- **Plugin and marketplace authoring** via `ai-rulez generate --plugin` (Claude, Cursor, Codex, Gemini, Kimi, OpenCode, Factory), with a `[plugin]` block, hooks, `${PLUGIN_ROOT}`, and single-plugin or monorepo marketplaces.
- Agent **`extends`** directive: an agent inherits a lower-precedence agent's body and frontmatter and appends its own.

### Changed

- Agent precedence is deterministic (same-named agents collapse to the highest-precedence definition).

## [4.5.0] - 2026-07-02

### Added

- Rule and context **deduplication by name** with source precedence (root > domain > include > builtin).
- `compact` option to omit per-rule priority annotations, and per-rule builtin exclusion (`!domain/rule`).
- MCP tool annotations and server title/instructions.

### Changed

- Remote MCP servers are emitted in each tool's own format (no empty `command`/`transport`); stdio is unchanged.
- Preset rendering moved to a declarative provider DSL, and lint/format moved to poly.

## [4.4.1] - 2026-06-07

### Fixed

- `.ai-rulez/.generated-manifest.json` is gitignored when gitignore management is enabled.

## [4.4.0] - 2026-06-07

### Added

- Per-preset model overrides for agents (`<preset>_model`, e.g. `claude_model`) with `defaults.model_by_preset` as the fallback; the legacy `model:` still works.

### Fixed

- The TOML loader no longer drops the `[defaults]` table.

## [4.3.0] - 2026-05-30

### Added

- `generate --gitignore` (with `-i`); the old `--update-gitignore` remains a deprecated alias.
- Collision-safe short flags across commands.
- MCP environment placeholder resolution from `--env`, the process environment, `.env` and `--env-file`.

### Fixed

- Secret-bearing MCP outputs fail generation when their path is not gitignored, and resolved secrets are redacted before source-hash calculation.

## [4.2.2] - 2026-05-26

### Added

- `polyglot-bindings` builtin (Rust-core, native ABI, FFI ownership, cross-language errors).

### Changed

- The TypeScript builtin recommends `oxfmt` with `oxlint`.

## [4.2.1] - 2026-05-21

### Fixed

- Cursor/Codex skill descriptions are quoted, so descriptions containing colons load.

## [4.2.0] - 2026-05-21

### Added

- **Manifest-scoped generation cleanup**: `generate` deletes only files recorded in `.generated-manifest.json`.
- **Real dry-run output** (`create-dir`, `write-file`, `delete-stale`) without touching the filesystem.
- **Custom config directory** (`--config-dir`), exact config-path loading, and `[[scopes]]` subfolder outputs.

### Fixed

- Hand-written files in assistant output directories are preserved during regeneration.

## [4.1.6] - 2026-05-03

### Changed

- **Git fetches use sparse checkout** (`--depth 1 --filter=blob:none --sparse`), fixing "response body too large" and cutting network/disk use; requires git ≥ 2.25.
- **BLAKE3-based cache invalidation** replaces the time-based TTL.

## [4.1.5] - 2026-05-02

### Changed

- **Skill resources are no longer concatenated into `SKILL.md`**: `references/`, `scripts/` and `assets/` are emitted as separate files, and `SKILL.md` carries a `## Resources` index (progressive disclosure).

### Security

- A malicious installed skill cannot follow a symlink out of its directory.

## [4.1.4] - 2026-05-01

### Fixed

- Internal test-client rewrite; no user-visible change.

## [4.1.3] - 2026-04-30

### Fixed

- **`generate --recursive` on a polyglot monorepo** no longer takes minutes or aborts on a broken symlink: it prunes build and cache directories, skips shared rule libraries, processes configs in parallel and memoizes tree scans (minutes to about a second, steady state).

## [4.1.2] - 2026-04-30

### Added

- Per-subagent reasoning effort for Codex (`model_reasoning_effort`) and OpenCode (`reasoningEffort`).

## [4.1.1] - 2026-04-30

### Added

- Reasoning effort extended to Codex, Amp and Windsurf, with `defaults.effort_by_preset` and MCP `default_effort_by_preset`.

## [4.1.0] - 2026-04-29

### Added

- **Reasoning effort on Claude Code subagents** (`effort` frontmatter: low/medium/high/xhigh/max/inherit).
- `defaults.effort` project default and MCP `default_effort`.

## [4.0.8] - 2026-04-27

### Added

- **`Source-Hash` header line** and hash injection for YAML-frontmatter files (hashes as YAML comments).

### Changed

- Output is sorted alphabetically by name and normalized to a single trailing newline (one round of reordered output).

### Fixed

- Generation is deterministic (domain iteration sorted); the `tools` list is no longer corrupted into a Go slice string; the `mcp` preset validates; skip-on-content-hash fires for frontmatter+banner files.

## [4.0.7] - 2026-04-27

### Fixed

- `--debug` and `--update-gitignore` now take effect.
- `.gitignore` no longer duplicates a pattern already present outside the managed fence.

### Changed

- Intentional-behavior warnings and verbose generation logs demoted to debug.

## [4.0.6] - 2026-04-25

### Fixed

- The MCP schema accepts V4 configs, and generated headers reflect the actual config filename.

## [4.0.5] - 2026-04-25

### Fixed

- Config discovery finds `config.toml`/`config.json`, and recursive generation no longer skips them.

## [4.0.4] - 2026-04-25

### Fixed

- Pre-commit hooks fire on `.ai-rulez/` changes, stale versions in scripts/wrappers are updated, and init templates default to config version 4.0.

## [4.0.3] - 2026-04-24

### Fixed

- Gitignore uses subdirectory patterns for shared directories (for example `.github/agents/`).

## [4.0.2] - 2026-04-24

### Added

- **Builtin agents** (code-reviewer, test-writer, security-auditor, docs-writer, devops-engineer, release-engineer) and new rules (verification-before-completion, systematic-debugging, testing-anti-patterns).

### Changed

- README redesigned around builtin capabilities and the development workflow.

## [4.0.1] - 2026-04-24

### Added

- New builtins `cicd`, `docker`, `observability`; new rules `no-ai-signatures`, `agent-workflow`, `communication-style`, `tdd-workflow`, `anti-patterns`; more domains auto-included.

### Fixed

- `migrate v4` preserves MCP servers.

## [4.0.0] - 2026-04-23

### Changed

- **BREAKING: TOML is the default config format** (`init` writes `config.toml`); existing YAML configs still work.
- **BREAKING: MCP servers are inline** under `[[mcp_servers]]` (legacy `mcp.yaml` still loads with a deprecation warning).
- **BREAKING: V2 compatibility is removed** (config types, migration, validator, generators), along with the deprecated `CompressionConfig`, `--skip-mcp`, `--auto-migrate` and `migrate v3`.

### Added

- Agent generation for all presets, context rendering for per-file presets, Claude MCP/plugins output, Cursor/Copilot MCP, Codex plugins/commands, Copilot commands, TOML config support, `[[plugins]]`/`[[marketplaces]]` and `migrate v4`.

### Removed

- V2 types, loader, migration, validators and generators; V1/V2 schemas; the V2 template engine; MkDocs config and the generated `site/`.

## [3.14.2] - 2026-04-20

### Fixed

- **Critical: `generate` no longer deletes non-generated files in `.github/`** (workflows, CODEOWNERS, issue templates).

## [3.14.1] - 2026-04-20

### Fixed

- Added the CI/Taskfile tasks the workflows referenced, and fixed Windows path-separator tests.

## [3.14.0] - 2026-04-20

### Added

- **`antigravity` preset** (`GEMINI.md`, `.agents/settings.json`, skills/agents under `.agents/`).
- Skills and agents for Gemini, Cursor, Codex, Amp, Windsurf, Copilot, Cline, Junie and OpenCode; Cursor skills moved to `.agents/skills`.
- `vite-plus` builtin; MCP read tools; `working_directory`; enum constraints; `read_config`/`update_config`; `dry_run`/`recursive`; tool annotations; `builtins show`.
- **Content hash** in generated headers (files skipped when unchanged) and an include cache TTL with `--no-fetch`.
- Fenced, idempotent `.gitignore` updates.

## [3.13.1] - 2026-04-19

### Fixed

- Frontmatter parsing falls back to raw map parsing; a missing skill description is a warning; cache locations are unified under `~/.cache/ai-rulez/`.

## [3.13.0] - 2026-04-19

### Added

- **`agent-delegation` builtin** (an `## Agents` roster in generated output, disable with `!agent-delegation`).
- New binding builtins (`jni-rs`, `extendr`, `cgo`) and three token-efficiency rules.

### Changed

- `compression` is a no-op and deprecated (its stopword removal made output ungrammatical); the compression package was removed.

## [3.12.0] - 2026-04-17

### Added

- **Installed skills**: `ai-rulez skill install/remove/list`, fetched at generate time (`installed_skills` config, with MCP tools).
- A distributable `ai-rulez` skill and `docs/llms.txt`.

### Fixed

- YAML config saving preserves ordering, comments and formatting, and include/skill caches are cleared before each fetch.

## [3.11.5] - 2026-04-15

### Fixed

- The Claude preset emits no header comments in skill, agent or command files.

## [3.11.4] - 2026-04-15

### Added

- **`local_override` for includes** (use a local directory instead of a git fetch), and minimal headers for skills/agents.

### Fixed

- Claude frontmatter is emitted as the first line; skills carry `user_invocable` and `description`; included domains are always available; output is deterministic.

## [3.11.3] - 2026-03-29

### Fixed

- Includes fail fast when the resolver is not registered.

## [3.11.2] - 2026-03-25

### Fixed

- `.github/` is no longer ignored as a directory, and files inside fully-managed directories are not listed individually.

## [3.11.1] - 2026-03-25

### Added

- R language builtin (tidyverse, testthat, CRAN, extendr).

## [3.11.0] - 2026-03-25

### Fixed

- Include domains are no longer duplicated, Claude output shrinks from ~13 MB to ~440 KB, and stale files are cleaned before writing.

### Changed

- Local rules in `CLAUDE.md` use `@path` lazy-loading references.

## [3.10.0] - 2026-03-18

### Fixed

- Included skills expose their frontmatter, and included domains are discovered and usable in profiles.

## [3.9.0] - 2026-03-11

### Added

- Root command docs (`build`, `fix`, `lint`, `review`, `test`) and a repository-managed pre-commit config.

### Changed

- Replaced lefthook with the prek/pre-commit toolchain.

### Fixed

- Codex skills always carry a `description`; V3 validation rejects a missing description; E2E test binary setup is stable.

## [3.8.3] - 2026-03-08

### Fixed

- Domain content from includes is no longer duplicated into root slices.

## [3.8.2] - 2026-03-08

### Fixed

- The Claude preset collects domain skills, agents and commands, and the `default-commands` builtin generates correctly.

### Changed

- Language builtins expanded with linting, SAST, coverage and benchmark tooling.

## [3.8.1] - 2026-03-07

### Changed

- Replaced simple compression with a token-reduction engine (levels `off`/`light`/`moderate`/`aggressive`/`maximum`, markdown-aware).

### Fixed

- Flat ai-rulez structures are detected at sub-paths.

## [3.8.0] - 2026-03-07

### Added

- **Built-in domains**: 23 embedded content domains shipped with the binary.
- **`builtins` config field** (`true`/`false`/list/`!name`) and `ai-rulez builtins list`.
- `/iterate` and `/parallelize` slash commands.

## [3.7.3] - 2026-02-19

### Fixed

- The publish workflow resolves Go from `go.mod`.

## [3.7.2] - 2026-02-16

### Fixed

- Claude skill rendering respects frontmatter `targets`, and the npm installer supports offline/private-registry bundled binaries.

## [3.7.1] - 2026-02-16

### Fixed

- Windsurf trigger frontmatter safely quotes `description` and `glob`.

## [3.7.0] - 2026-02-16

### Fixed

- Includes now carry agents, and Codex skills always carry a `description`.

## [3.6.1] - 2026-01-07

### Fixed

- Windows releases use `.zip` instead of `.tar.gz`.

## [3.6.0] - 2026-01-05

### Changed

- Skills and commands are written to dedicated directories instead of inlined into root files across every preset (Claude's `CLAUDE.md` dropped ~77%, from 14K to 3.2K lines), reducing prompt tokens.

## [3.5.0] - 2026-01-04

### Added

- **V3-native command system**: file-based slash commands in `.ai-rulez/commands/` with YAML frontmatter, profile-aware and rendered per preset (`migrate v3` for V2 commands).
- Configurable prompt compression and context-file optimization (a required `summary`; context rendered as summaries with `@` links, ~34% smaller).

### Changed

- Remote include cache moved to the system cache directory.

### Fixed

- Includes merge commands from remote sources, and the default profile includes commands.

## 3.4.1 - 2026-01-03

### Added

- SSH git clone support for private repositories, self-hosted GitLab, and repositories whose root is the ai-rulez structure.

## 3.4.0 - 2026-01-03

### Added

- Configurable header styles (`detailed`/`compact`/`minimal`), `CLAUDE.md` generation, and markdown formatting with markdownlint compliance.

## 3.3.2 - 2026-01-02

### Fixed

- SSH git URL conversion in includes, with multiple SSH URL formats accepted.

## 3.3.1 - 2025-12-31

### Fixed

- SSH git URL detection (`git@host:path` was treated as a local path).

## 3.3.0 - 2025-12-31

### Fixed

- Complete agents support in the includes system.

## 3.2.2 - 2025-12-28

### Added

- `init` creates root and domain agent directories, and generated MCP config includes the ai-rulez server.

## 3.2.1 - 2025-12-28

### Fixed

- Gitignore uses relative paths and no longer ignores `.ai-rulez/`.

## 3.2.0 - 2025-12-28

### Added

- Full agent/subagent support in V3 (`.ai-rulez/agents/`), agent metadata, and Claude Code subagent output.
- Auto-migration of V2 configs (interactive in a terminal, silent in CI; `--auto-migrate`).

## 3.1.0 - 2025-12-27

### Added

- `migrate` command for V2 to V3 configuration migration.

## 3.0.0 - 2025-12-27

### Added

- Directory-based configuration (`.ai-rulez/`), CRUD CLI commands, 22 MCP tools, domains, profiles and includes.

### Changed

- **BREAKING: configuration changed from a single YAML file to a directory structure**, and `init` no longer uses AI agents for dynamic initialization.

### Removed

- **BREAKING: the `enforce` command and V2 dynamic init are removed**; single-file YAML support is dropped.

## 2.4.0 - 2025-10-22

### Fixed

- CLI MCP interference with template-generated files, and output type values in AI-generated configs.

## 2.3.0 - 2025-10-05

### Added

- AI-powered rule enforcement, automatic gitignore management, and the Junie preset.

## 2.2.0 - 2025-09-20

### Added

- MCP file-based configuration for Claude and other tools.

## 2.1.0 - 2025-08-20

### Added

- CLI MCP integration for Claude, and an improved init phase.

### Fixed

- Cross-platform binary extensions, and the Homebrew formula structure.

## 2.0.0 - 2025-07-30

### Changed

- **BREAKING: schema v2** with a priority enum, unified `name` (was `title`), and named target resolution.

## 1.6.0 - 2025-07-10

### Added

- Target filtering and named targets.

## 1.5.0 - 2025-06-25

### Added

- Initial release: core configuration generation, the rule definition system and a template-based generator.
