# Migrating to v5

This page lists every breaking change of v5 with the action it needs. The [changelog](CHANGELOG.md#unreleased) has
the complete list of additions and fixes. The config format `version` stays `"4.0"`: a v4 `config.toml` loads
unchanged. V2 and V3 configs are no longer read, and `ai-rulez migrate` is gone; see
[V2 and V3 configs](#v2-and-v3-configs).

Run `ai-rulez doctor` first. It reports removed presets with a replacement, drifted outputs and generated paths
git does not ignore. Then run `ai-rulez generate`, review the diff, run `ai-rulez lock` once if you use a lock
(see [Lock file and enforcement](#lock-file-and-enforcement)), and commit the sources, outputs and lock together.

## Checklist

| Change | Action |
| ------ | ------ |
| V2/V3 configs are no longer read: `config.yaml`, `config.yml`, `config.json`, `config.local.yaml`/`.yml`/`.json`, `mcp.yaml`/`.toml`/`.json` and the flat `ai-rulez.yaml`; `migrate` and `init --format` are removed; `version = "3.0"` is rejected | Migrate with ai-rulez 4.x first (`npx ai-rulez@4 migrate v4`), then upgrade; see [V2 and V3 configs](#v2-and-v3-configs) |
| Go module is `github.com/Goldziher/ai-rulez/v5` and the entry point is `cmd/ai-rulez` | `go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest`; update Go imports |
| `windsurf` is renamed `devin`; `continue-dev` is removed | Rename or remove the preset, delete old outputs |
| `[lock] enforce` is on whenever `ai-rulez.lock` exists | Commit a current lock, or set `enforce = false` |
| The lock `tree` digest, pinned sources and frontmatter hook scripts changed | Run `ai-rulez lock` once |
| Exit codes follow one contract, `lock` adds `3` | Update scripts that match exit codes |
| `lock --check` without a lock file exits 1 | Run `ai-rulez lock` first, or expect 1 |
| `http://` and `git://` remotes are rejected | Switch to `https://` or `ssh://` |
| The git token goes only to allowlisted hosts | Set `AI_RULEZ_GIT_TOKEN_HOSTS` for non-GitHub hosts |
| A committed config cannot reach outside the project | Move such paths to `config.local.toml` or the user config |
| Symlinked content must resolve inside the project | Replace links that leave the project |
| `scan_imports` is on by default | Fix findings, or set `scan_imports = "off"` |
| `scan` runs only the `security` analyzer | Use `validate --strict` for the other findings |
| Claude MCP servers are written only to `.mcp.json` | None; `.claude/settings.json` loses `mcpServers` |
| `[telemetry] service_name` is user scope only | Move it to the user config or the environment |
| Eval results are signed per user | Use `eval run --force` in CI |
| Usage log is version 3 | None; older logs still read |
| `schema/catalog.schema.json` is catalog version 2 | Point validators of version 1 at `schema/catalog.v1.schema.json` |
| `generate` warns about unknown config keys and new commands | Fix the keys; set `--yes` or `AI_RULEZ_ACK_COMMANDS=1` in CI |
| `[lint.budget]` is renamed `[lint.tolerate]` | Rename; the old name still works and warns |
| `--json` is deprecated for `--format json` | Switch scripts to `--format json` |
| Staged scanners are confined under `isolation = "auto"` (the default) wherever a backend works | A scanner that writes outside its scratch directory now fails (`AR9E3`): point it at `TMPDIR`/`HOME`, or set `isolation = "none"`; see [Isolation](strict-validation.md#isolation) |
| Custom preset and provider output paths are validated | Remove `..`, absolute and `.git` paths |
| `lock` and `update` scan every remote tree they pin to something new | Fix error findings, or pass `--accept-findings` after reviewing them |
| `init --from` runs through `convert --write` | Expect one context item per root file (`convert --split-headings` splits it); see [`init --from`](#init-from) |
| `generate --check` reports `blocked:` for a shared file a machine-local input would change | Commit or drop the local change, or run `generate --allow-local-drift` |
| The forge client (release dates, review-linked approvals) has its own host allowlist | GitHub Enterprise: set `AI_RULEZ_FORGE_HOSTS` |
| An organization policy at the managed path is read automatically | None unless the machine has one; see [Organization policy](#organization-policy) |
| Review-linked approvals count only reviews of the final head by members or named approvers; `max_age` is a ceiling | Re-run `approve --from-github-review` after new pushes; see [Approvals](#approvals) |
| Go APIs under `internal/` changed; `pkg/airulez` is the supported API | See [Go API](#go-api) |
| The `compression` option is gone (it was a no-op since v3.13) | Delete it; a config that still sets it loads and `generate` warns about the unknown key, but `validate` and `generate --strict` fail |

## V2 and V3 configs

v5 reads one config format: `.ai-rulez/config.toml` (or `.config/ai-rulez/config.toml`) with its content tree,
`config.local.toml` and the user `config.toml`. A project that holds only an older file stops with an error that names
the file and exits `1`; `ai-rulez doctor` reports the same file. Nothing is converted for you in v5, so migrate before
you upgrade:

```bash
# V3: .ai-rulez/config.yaml (or .yml / .json), config.local.*, mcp.yaml|toml|json
npx ai-rulez@4 migrate v4

# V2: a flat ai-rulez.yaml (also .ai-rulez.yaml, ai_rulez.yaml and the .yml forms)
# move it to .ai-rulez/config.yaml first, then run the command above
```

Then upgrade to v5, run `ai-rulez generate` and commit `config.toml` with the outputs. The 4.x command writes
`config.toml`, converts a `config.local.*` overlay to `config.local.toml` and folds a separate MCP file into
`[[mcp_servers]]`. `version = "3.0"` is rejected with `version = "4.0"` as the fix. `ai-rulez init` always writes
`config.toml`, so its `--format` flag is gone, and `init --from` writes TOML too. `convert` leaves a `config.yaml` it finds
alone and reports it as `manual`.

A V3 file nested in a subdirectory is reported by `generate --recursive` with the same error as a root one (it is not
read), and the run exits `1`.

The MCP `init_project` tool writes the same `config.toml` layout as `ai-rulez init` (it wrote a V2 `config.yaml`) and
refuses to overwrite an existing configuration.

## `init --from`

`init --from` now runs `convert --write` with its sources: importer names (`auto`, `native`, `rulesync`, ...) or the
project paths it always took (`.claude`, `.cursor`, `CLAUDE.md`). It gets convert's scan, validation and lossiness
report. Differences from v4:

- A root file such as `CLAUDE.md` becomes one context item. Use
  `ai-rulez convert --split-headings` to split it.
- MCP files, hooks and permissions are imported too (hooks and `allow` rules as a commented block you review first).
- The sources are checked in a scratch directory first. An existing configuration directory is moved aside until the
  import has been written and is restored when the import fails, so a failed `init --from` leaves the old
  configuration in place.
- Symlinks in the imported repository are never followed, and files over 2 MiB are skipped.

`convert --fetch`, new in v5, reads remote rulesync and APM sources over `https://` only; ssh, scp-style, `file://` and
local sources are reported as `needs-action` instead of being cloned.

## Go module path and install

The Go module is now `github.com/Goldziher/ai-rulez/v5`, and the main package moved from `cmd/` to `cmd/ai-rulez`.
Code that imports ai-rulez packages, or installs the CLI with `go install`, must use the new path:

```bash
go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest
```

The npm, PyPI and Homebrew distributions are unaffected.

## Presets

| Change | Action |
| ------ | ------ |
| `windsurf` is renamed `devin`. The output directory `.windsurf/` is now `.devin/` and the agent frontmatter key `windsurf_model` is now `devin_model`. There is no alias. | Rename the preset in `config.toml`, rename `windsurf_model` keys in agent files, and delete the old `.windsurf/` outputs. |
| `continue-dev` is removed. It has no replacement. | Remove it from `presets`. `doctor` reports it as an error. |
| `antigravity` no longer adds the `ai-rulez` MCP server on its own. | Set `[mcp] self_server = true` to keep it, as for every other preset. An entry an earlier version wrote into `.agents/mcp_config.json` is removed on the next `generate` unless you set it. |
| `amp` no longer writes `.agents/agents`, which Amp does not read. | None. Agents are listed in `AGENTS.md`. `amp_model` has no effect. |
| `codex` and `antigravity` write commands as skills (`.agents/skills/<id>/SKILL.md`) instead of `.codex/prompts` and workflows. | None. Files from the old layout are removed on `generate`. |
| `codex` writes skills to `.agents/skills`, not `.codex/skills`. | Set `codex_skills_dir = ".codex/skills"` to keep the old location. |
| `cursor` user-level skills go to `~/.agents/skills`, shared with `codex`, `gemini` and `pi`. | Run `generate --user`; `clean --user` removes the old copies recorded in the manifest. |
| `opencode` writes MCP servers as `mcp.<name>` with `enabled`, not `mcp.servers.<name>` with `disabled`. | None. Members recorded under `mcp.servers` are removed. |
| `zoocode` inlines rules into `AGENTS.md` and no longer writes `.roo/rules`. | None. |

## Generation behavior

- **Divergent shared outputs fail.** When two presets render different content to the same path, `generate`
  fails and names them instead of keeping the last one. Make the outputs agree, for example with
  `agents_md = true` or `rules.mode = "inline"`. See [Supported harnesses](harnesses.md#cross-cutting-behavior).
  `qoder` next to a tool that reads `${VAR}` references in the shared `.mcp.json` (`claude`, `cursor`, `copilot`,
  `codebuddy`, `commandcode`, `reasonix`) also fails, naming both, instead of writing a resolved secret.
- **Claude MCP servers move out of `.claude/settings.json`.** Claude Code reads project MCP servers from
  `.mcp.json`, which references `${VAR}`; `.claude/settings.json` no longer receives `mcpServers`, so resolved env
  values do not land in a committed file. An entry an earlier version wrote there is removed on the next
  `generate` or `clean` while it is still the value ai-rulez wrote. The settings file is written only when
  `[claude.settings]` manage, `[[hooks]]`, `[permissions]` or `[claude.settings.managed]` apply.
- **`generate` warns about unknown config keys** in `config.toml` and `config.local.toml`, naming the nearest known
  key. `generate --strict` (or `AI_RULEZ_STRICT=1`) fails instead. `validate` fails on them as before.
- **`generate` summarises new or changed commands**: hook commands, command-based MCP servers, `[permissions]
  allow` rules, `[claude.settings.managed] env`, plugin enablement and `http`/`prompt` hooks that are new since the
  previous run on this machine (all of them in a fresh clone), printed even with `--quiet`. It only warns. Pass
  `--yes` or set `AI_RULEZ_ACK_COMMANDS=1` in CI to silence it.
- **`clean` keeps hand-edited generated files** with a warning; `clean --force` removes them. The committed
  `.ai-rulez/.generated-manifest.json` lists merged-document paths only; claims and digests live in the gitignored
  `.generated-manifest.local.json`. A forged committed manifest cannot make `generate` or `clean` delete or strip
  anything: a file is removed only when its own `Content-Hash` (or the local digest) proves ai-rulez wrote it.
- **Check outputs are committed, not gitignored.** Hosted reviewers read checks from the base branch, so the
  files under [Checks](checks.md) are never added to the managed `.gitignore` block. An entry an earlier version
  added is removed on the next `generate`.
- **`generate --dry-run` prints `unchanged:` and `edited:`** for files that need no write. Scripts that match
  `write-file:` for every file need updating.
- **`tokens` totals include the item listing.** `always` and `headline_always` now count the skill, command and
  agent listing, so `--budget` can fail where it passed. The previous figures are `always_legacy`,
  `conditional_legacy` and `headline_always_legacy`.
- **Skill `evals/` directories are not bundled into plugins** unless `[plugin] include_evals = true`.
- **Custom preset and provider paths are validated.** A custom preset `path`, a provider spec path
  (`root.file`, `outputs.*.dir`, sidecars), `okf.dir` and `marketplace.output_dir` are rejected when they contain
  `..`, are absolute or drive-qualified, or name `.git`, `.ai-rulez`, `.hg` or `.svn`. `generate` fails closed on any
  write outside the project or inside `.git`. A custom preset that writes a CI or tool-executed file
  (`.github/workflows/`, `Makefile`, ...) appears as `exec-file` in the command summary.

## Lock file and enforcement

- **`ai-rulez.lock` pins content, under one hashing scheme.** `lock` now also pins the ai-rulez version, every
  authored rule, context file, skill, agent, command, hook, role and settings source, the generated outputs and
  served skills, as `sha256:` digests. Remote includes, OKF includes, installed skills and skill sources use the
  same scheme (there is no second, older per-file hash). Scripts (`.sh`, `.py`, `.js`, ...) are hashed byte for byte:
  a changed line ending in a script is a changed digest. `lock --check` exits 2 on a lock without content pins,
  whatever `[lock] enforce` says. A lock with another `version` is refused with the instruction to run
  `ai-rulez lock` again. A lock that pins role outputs (`[roles]` `pin = true`) is written as `version = 2`, which an
  older v5 build refuses instead of misreading the role pins; a lock without role pins stays `version = 1`.
  See [Lock file](lockfile.md).
- **Run `ai-rulez lock` once after upgrading**, review the diff and commit the file. The lock reads as stale until
  you do, for these reasons:
  - The `tree` digest now also covers the `source`, `ref` and `path` of remote entries and the `view` of served
    entries, so relabelling or swapping entries is detected. A lock written by an earlier v5 build reports the tree
    digest as stale.
  - `lock` pins the project scripts run by agent, skill and command frontmatter `hooks`. Editing such a script makes
    `lock --check` exit 2; locks of items with frontmatter hook scripts need `lock` once.
  - Local-path includes are pinned as `local-include` items, and include skills are recorded as
    `include:<name>/<path>` instead of a machine-specific cache path.
  - Include and skill sources are recorded exactly as written in the config, and a `file://` source stays
    machine-independent. Locks written earlier keep working until the next `lock`.
- **Symlinks in a pinned tree are pinned by their link target.** A symlink inside an include, installed skill, skill
  source or OKF tree is never followed and no longer blocks locking: the link target string is part of the digest, so
  a retargeted link is detected. Junctions and other irregular entries are pinned by path only. A symlinked root
  directory is still refused. Trees without symlinks keep their digest. File modes digest by the owner execute bit
  only, as git records it.
- **`[lock] enforce` defaults to `true` whenever `ai-rulez.lock` exists.** Set `enforce = false` to opt out. A remote
  include or installed skill the lock does not cover makes `generate` fail (as `--locked` always did) and `AR010`
  an error, and an include that cannot be resolved is an error instead of a skipped warning. `generate --frozen` and
  `--locked` are unchanged: they require the lock whether or not enforcement is on.
- **`[lock] enforce = true` is strict.** It makes `validate --strict` report `AR981` (source drift) and `AR982`
  (output drift), makes `generate --locked` fail on drift, and makes the skills server refuse a served skill that
  the lock does not pin or whose digest differs. A corrupt lock, a lock of another `version` or a source that cannot be
  snapshotted is an `AR981` finding, not a logged skip.
- **`generate --check` verifies authored content against an enforced lock**, like `--locked`.
- **The lock digest ignores the project-wide `Source-Hash` header**, so editing an unrelated file no longer changes
  the digest of a served skill.
- **A served skill the security scan refuses no longer stops `lock`.** It is left unpinned, `lock` exits `3`, and
  `lock --strict` restores the fail-without-writing behavior.
- **`lock` and `update` scan what they pin.** Every remote tree pinned to something new is scanned (`AR001`-`AR009`)
  first; an error finding refuses the pin (exit `2`, nothing written) unless `--accept-findings`.
- **`generate --check` classifies machine-local inputs like `generate`.** With a `config.local.*` overlay or `local/`
  content it no longer reports every output as `stale`. A shared file the local input would change, and that
  `generate` refuses to write (tracked or not ignored), is reported as `blocked: <path>` with exit `2`;
  `--allow-local-drift` accepts it.

## Exit codes

All commands follow one contract: `0` success, `1` the command could not run (invalid configuration, missing input,
tool error), `2` findings, drift or a failed gate, `3` only for `lock`. Scripts that matched `1` for a drift result
need to match `2`.

| Command | `0` | `1` | `2` | `3` |
| ------- | --- | --- | --- | --- |
| `generate` | Written | Failed to load, validate or generate (any root with `--recursive`) | `--check` found drift; `--locked`/`--frozen` source differs from the lock; recursive run where every failure is lock drift | |
| `validate` | Valid | Invalid configuration | `--strict`: findings at or above `--fail-on` | |
| `scan` | Clean | Cannot run | Findings at or above `--fail-on` | |
| `verify` | Verified | Cannot run (no manifest) | Files differ | |
| `lock` | Written or verified | Cannot run; `--check` with no `ai-rulez.lock`; unknown name | `--check` found drift (also a lock without content pins, or no lock under `enforce`); `--outdated` moved tag or unsatisfiable constraint; `--fail-on-outdated` | Lock written, served skills left unpinned by the scan |
| `update` | Done or nothing to do | Cannot run | A source was refused (`AR730`, `AR731`, `AR732`); nothing written | |
| `doctor` | No errors | Configuration does not load | An error (or, with `--strict`, a warning) | |
| `verifiers run` | None failed | Nothing failed but the run could not complete | A verifier failed at `--fail-on` | |
| `eval run` | All pass | Flags invalid | A skill failed its threshold, errored or has invalid cases | |
| `tokens`, `cost` | Within budget | Configuration cannot load | Over `--budget` or `--on-demand-budget` | |
| `convert` | Done | Cannot run, or would overwrite files | Blocked by the scan or validation, or `--fail-on` matched | |
| `export okf`, `import okf`, `okf validate` | Done | Cannot run | Drift (`--check`), lint findings, files not overwritten, or refused by the scan | |
| `search --eval` | Pass | Cannot run (`AR9D2`) | A gate failed (`AR9D4`) | |
| `scanners doctor` | Healthy | Configuration does not load or a name is unknown | A checked scanner is missing or misconfigured | |
| `guard` (hook) | Allowed | | The call edits a generated file | |

Unknown subcommands (`telemetry bogus`) exit `1`. When a command covers several roots
(`--recursive`, for `generate --check` and `lock`) the most severe code wins: `1`, then `2`, then `3`.

## Supply-chain defaults

- **Plain `http://` and `git://` remotes are rejected.** `git://` is unauthenticated and can be rewritten in
  transit. A remote include, OKF include, installed skill or skill source must use `https://`,
  `ssh://` / `git@host:path`, or a local `file://` URL or path. The error names the source and says to switch to
  `https://`; `include add` refuses them too. Include sources accept a leading `git+` (`git+https://host/org/repo`).
- **The git token goes only to allowlisted hosts.** `AI_RULEZ_GIT_TOKEN` / `--token` is sent to `github.com` by
  default, or to the hosts in `AI_RULEZ_GIT_TOKEN_HOSTS` (comma separated, environment only; when set it replaces the
  default, so list `github.com` too), as a host-scoped header rather than inside the URL, and only over `https://`.
  Set the variable to keep using a token with GitLab, Bitbucket or a self-hosted host; other hosts get no token and a
  warning. Caches written by earlier versions are scrubbed.
- **The forge token has its own allowlist.** The forge client (release dates for `min_release_age`, review-linked
  approvals) sends the GitHub token (`GITHUB_TOKEN`, `GH_TOKEN` or `gh auth token`) only to `github.com` or the hosts
  in `AI_RULEZ_FORGE_HOSTS` (comma separated, environment only), never to every `AI_RULEZ_GIT_TOKEN_HOSTS` host. For
  GitHub Enterprise Server, add its host to `AI_RULEZ_FORGE_HOSTS`. See [Forge client](forge.md).
- **A committed config cannot point outside the project.** A local include (`source` or `local_override`) that
  resolves outside the project after symlinks (`../victim`, an absolute path) is a fatal error. It is still allowed
  in `config.local.toml` and the user config. A local `[[skill_sources]]` `url` or `path` must resolve inside the
  project (`mcp --source <dir>` and the user config may point anywhere). `local_override` on an include or installed
  skill in the committed config is refused under `generate --locked`, `--frozen` and an enforced lock, because it
  bypasses the pins; set it in `config.local.toml`.
- **Imported content is scanned by default.** `[lint.security] scan_imports` is on when unset: `generate` scans
  includes and installed skills before writing anything and stops at an error-level finding. Skills from includes
  (and any skill file outside the project) are scanned at the strict level. Set `scan_imports = "off"` to opt out, or
  `"warn"` to log only.
- **Unpinned MCP packages (`AR012`)** stay a warning in `validate`, and are an error whenever `[lock] enforce` is on
  (whenever `ai-rulez.lock` exists, unless `enforce = false`), together with `AR010`.
- **Content symlinks follow one policy.** In the project's own `.ai-rulez/` (including domains, skill and command
  resources), a symlinked file or directory is followed only when its fully resolved target is inside the repository
  root (the git top level, else the directory holding `.ai-rulez`); the refusal names that root. An enclosing
  repository widens that root only when it tracks the project (its index holds `.ai-rulez/config.toml`): a project in a
  monorepo may link to its siblings, but a project that merely sits below a `$HOME` dotfiles repository is held to its
  own directory. `GIT_CEILING_DIRECTORIES` entries are resolved through symlinks, as git does. A project not yet added
  to its repository has its own directory as the root until `git add`. A symlinked `config.toml` or `config.local.toml`
  follows the same
  boundary: a target outside the root is a load error. Any other link used to be dropped silently; it is now
  refused with a warning that is shown even with `--quiet`, and `ai-rulez validate` reports it as an error. Symlinks
  in includes (git or local), installed skills, skill sources and OKF bundles are never followed and are skipped with
  a warning (an installed skill with a symlinked `SKILL.md` is refused). `init --from` never follows symlinks.
  Repository content read at load time is capped at 8 MiB per file; a larger file is an error.
- **`scan` is security-only.** `ai-rulez scan` runs only the `security` analyzer's checks. Hook and config findings
  (`AR504`, `AR9K0`, ...) that used to appear in its report belong to `validate --strict`.

## Scanner isolation

Staged scanners (`[[lint.external]]` with `inputs`) run confined under `isolation = "auto"`, the default, wherever a
backend works: macOS `sandbox-exec`, Linux `bwrap` or `unshare`. A confined scanner has no network (unless it declares
`egress = true`) and cannot write outside its scratch directory, so one that writes elsewhere now fails with `AR9E3`.
Point it at `TMPDIR`/`HOME`, or set `isolation = "none"` on the entry or in `[lint.scanner_policy]`. Without a backend,
`auto` runs unconfined and notes `AR9E7`; `isolation = "require"` refuses to run instead. See
[Isolation](strict-validation.md#isolation).

## Organization policy

v5 reads a tighten-only organization policy from outside the repository: `--policy`, `AI_RULEZ_POLICY`, or the
managed path (`/etc/ai-rulez/policy.toml`, `/Library/Application Support/ai-rulez/policy.toml`,
`%ProgramData%\ai-rulez\policy.toml`). Nothing changes without one. When one applies, a repository value that
loosens it is clamped and reported (`AR740`), and `generate` and `validate` refuse such a configuration unless
`--policy-mode warn`. See [Organization policy](policy.md).

## Approvals

Approvals (`ai-rulez approve`, `[governance]`) are new in v5. Pre-release v5 builds counted some approvals that no
longer count:

- **Review-linked approvals need the pull request's final head.** `approve --from-github-review` records a review only
  when it was made on the head the pull request ends with; a review of an earlier push does not count. Re-run it after
  new pushes.
- **Outsider approvals do not count.** A review counts only when the reviewer's `author_association` is `OWNER`,
  `MEMBER` or `COLLABORATOR`, or `approvers` or CODEOWNERS name them, so a drive-by approval on a public repository is
  ignored. The digest is recomputed from the files at the reviewed commit, not taken from the lock committed there.
- **`[governance] max_age` is a ceiling.** An approval stops counting (`AR712`) once `approved_at` plus `max_age` has
  passed, whatever its `expires` says, and `approve --expires` beyond it is refused.

See [Approvals](approvals.md).

## Go API

Packages under `internal/` are not a public API, and v5 changed several of them. Code that embeds ai-rulez should use
`github.com/Goldziher/ai-rulez/v5/pkg/airulez` (experimental, see [Embedding](embedding.md)):

- The process-wide preset registry is gone: `config.GetPresetGenerator`, `config.PresetRegistry`,
  `config.RegisterPreset` and `config.RegisterRulesDir` are removed; each generation carries its own registry.
- `config.LoadConfig` fails when the config declares includes or installed skills unless the load is given
  `config.WithResolvers` (or `config.WithoutRemote`).
- `config.SetPolicyEnforcer` is gone: the organization policy belongs to the load (`config.WithPolicy`).
- The V2/V3 helpers (`config.DetectConfigVersion`, `config.VersionDir`, `config.ConfigVersionV3`, `Config.IsV3`,
  `config.DecodeLegacyMCPFile`, `config.MigrateLocalOverlayToTOML`, `LocalOverlay.Format`) are removed.
- In `pkg/airulez`, the machine-local overlay (`config.local.toml`, `.ai-rulez/local/`) is read only with
  `Options.WithLocal`; `Options.WithoutLocal` is removed.

## Trust rule for `[llm]` and `[telemetry]`

`allow_network`, `base_url`, `api_key_env` and the price overrides of `[llm]`, and the egress keys of `[telemetry]`
(`allow_network`, `otlp_endpoint`, `headers_env`, `service_name`, `resource`, ...), are honoured only from the user
config file (`~/.config/ai-rulez/config.toml`) or the `AI_RULEZ_LLM_*` / `AI_RULEZ_TELEMETRY_*` variables. A
repository `config.toml` or `config.local.*` that sets them is ignored and reported by `llm doctor`,
`telemetry doctor`, `ai-rulez doctor` and the strict findings `AR9L1` and `AR9K1`. A literal key in either table is
`AR9L0` or `AR9K0`. If a repository relied on setting these, move them to the user config file. `[telemetry]
service_name` is the v5 addition to this list: it labels data on the user's collector, so a repository value is
ignored. See [LLM access](llm.md) and [Telemetry](telemetry.md).

The same rule now covers every egress or execution knob (scanner egress, eval execution, git credentials, hooks) and is
documented once, with a table of each knob's scope and precedence, in the [trust model](trust-model.md).

## Hooks, validation and the rule registry

- **`validate --strict` has more rules.** About thirty new codes (`AR304`, `AR305`, `AR403`, `AR504`-`AR507`,
  `AR601`, `AR602`, `AR805`-`AR807`, `AR963`, `AR964`, `AR971`-`AR982`, `AR9A0`-`AR9B9`, `AR9C*`, `AR9K*`, `AR9L*`
  and more) can fail a CI job that passed before. Every code is listed in [Strict validation](strict-validation.md);
  lower or turn off a rule with `[lint.severity]`, or accept current findings with
  `validate --strict --update-baseline`. Rule codes are stable and are never renumbered.
- **`[lint.budget]` is renamed `[lint.tolerate]`** (tolerated findings per rule), so it no longer reads like
  `[lint.budgets.<kind>]` (size budgets). `[lint.budget]` still works and warns. The report says
  `over its tolerated count of N`; the JSON key `budgets_exceeded` is unchanged.
- **Top-level `[[hooks]]` validation is stricter**: a `script` outside `A-Za-z0-9._/-` is rejected, a missing or
  non-executable script is `AR504`/`AR505`, and every generated shell line quotes the path.
- **Served skills (`delivery = "served"`) are left out of the harness skill trees** and reach the model through
  `ai-rulez mcp --serve-skills`; a static reference to one is `AR990`. Skills default to `static`, so nothing
  changes until you opt in.
- **Eval results are signed per user.** `eval run` signs each stored record with a per-user key
  (`eval-results.key`, in the user config directory). A record without a valid signature, such as one committed from
  another machine, is `unverified`: `eval run` re-runs it, and `AR997`, `AR998`, `report evals` and `report usage`
  ignore it. CI has no key; gate on `eval run --force`. See [Evals](evals.md).
- **Usage log version 3.** The `session` field is a salted hash of the harness session id, not the raw id, and lines
  carry `digest`, `digest_scheme` and `event_id` (the lock's skill digest). Version 1 and 2 logs still read.
- **`--json` is replaced by `--format json`.** Every command that printed JSON takes `--format text|json` (some add
  `sarif`, `junit`, `markdown`); `--json` stays as a hidden alias that warns. An unknown `--format` value is rejected
  with the allowed values. `lock --format` is accepted only with `--check`, `--diff`, `--outdated` or `--subject`.
- **`schema/catalog.schema.json` is catalog version 2.** The version 1 schema moved to
  `schema/catalog.v1.schema.json`; `catalog --format json` still prints version 1 unless `--schema-version 2`.

## Where to look

- [Supported harnesses](harnesses.md): the preset list and what each writes.
- [Hooks, permissions and settings keys](settings.md), [Permissions](permissions.md) and
  [User-level configuration](user-scope.md): top-level `[[hooks]]`, `[permissions]` and `generate --user`.
- [Lock file](lockfile.md) and [Trust model](trust-model.md): the lock scheme and what a repository may set.
- [Changelog](CHANGELOG.md#unreleased): the complete list for the unreleased version.
