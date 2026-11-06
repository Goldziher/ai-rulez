---
title: "CLI"
description: "ai-rulez turns one .ai-rulez/ source tree into the instruction files, rules, skills, MCP configuration, hooks and permissions of every coding assistant you list in presets."
---

`ai-rulez` turns one `.ai-rulez/` source tree into the instruction files, rules, skills, MCP configuration, hooks and
permissions of every coding assistant you list in `presets`. The commands are grouped below by task, with the flags
you use most; every flag of every command, with its default, is in the [Command reference](/ai-rulez/cli-reference/).

## A first run

```bash
ai-rulez init acme --domains backend,frontend
# edit .ai-rulez/config.toml: presets = ["claude", "cursor", "codex"]
ai-rulez add rule api-errors --domain backend --priority high \
  --content "Return RFC 9457 problem details from every handler."
ai-rulez generate --dry-run
```

```text
profile: default
write-file: AGENTS.md
write-file: .agents/skills/ai-rulez/SKILL.md
write-file: .agents/skills/code-reviewer/SKILL.md
write-file: CLAUDE.md
write-file: .claude/skills/ai-rulez/SKILL.md
write-file: .claude/skills/code-reviewer/SKILL.md
write-file: .claude/settings.json
write-file: .codex/config.toml
write-file: .cursor/mcp.json
write-file: .mcp.json
```

```bash
ai-rulez generate     # write the files
ai-rulez validate     # lint the configuration and content
ai-rulez generate --check   # in CI: exit 2 when a generated file differs from its source
```

## Global flags

These flags work on every command.

| Flag | Meaning |
| ---- | ------- |
| `-C`, `--config <path>` | The project to use: a configuration directory (`.ai-rulez`, `.agents`) or a config file (`.agents/ai-rulez.toml`, `ai-rulez.toml`). No command takes the config path as an argument |
| `--config-dir <name>` | Configuration directory name below the working directory (default `.ai-rulez`, then `.config/ai-rulez`) |
| `-T`, `--token <token>` | Git access token for private includes and skill sources (or `AI_RULEZ_GIT_TOKEN`) |
| `-D`, `--debug` | Debug logging on stderr (or `AI_RULEZ_DEBUG=1`) |
| `-q`, `--quiet` | Hide progress and information lines on stderr (or `AI_RULEZ_QUIET=1`); results, warnings and errors stay |
| `-h`, `--help` | Help for a command |
| `-v`, `--version` | Root command only: print `ai-rulez version <version>`, the same as `ai-rulez version` |

### Policy flags

Commands that evaluate the [organization policy](/ai-rulez/policy/) also take the `--policy*` flags: `approve`, `catalog`,
`catalog diff`, `cost`, `doctor`, `generate`, `judge`, `lock`, `mcp`, `publish`, `sbom`, `scan`, `tokens`, `update`,
`validate` and `verify`. Every other command still honors `AI_RULEZ_POLICY` and the managed policy path.

| Flag | Meaning |
| ---- | ------- |
| `--policy <file or URL>` | Organization policy (tighten-only); also `AI_RULEZ_POLICY` and the managed path |
| `--policy-digest sha256:<hex>` | Digest the policy must have; a URL policy is never loaded without one (`AR741`) |
| `--policy-offline` | Load a URL policy from the user cache only |
| `--policy-max-stale <dur>` | How long a cached URL policy may stand in for an unreachable URL (default `7d`, `0` for never) |
| `--discover-org` | Also load `ai-rulez-policy.toml` from the repository owner's `<owner>/.github` repository (needs a digest) |
| `--policy-require-signed` | Refuse a policy without a valid signature (`AR746`); name signers with `--policy-signer-key` or `--policy-signer-identity` and `--policy-signer-issuer` (plus `--policy-trusted-root` for keyless) |
| `--policy-trust-tofu` | Record the digest of an unpinned policy URL once, in a terminal only |
| `--policy-mode enforce\|warn` | With `warn`, a repository that loosens the policy produces warnings instead of failing |

### Flag conventions

Seven shorthands exist, and each means the same thing on every command:

| Short | Long |
| ----- | ---- |
| `-C` | `--config` |
| `-D` | `--debug` |
| `-T` | `--token` |
| `-q` | `--quiet` |
| `-y` | `--yes` |
| `-n` | `--dry-run` |
| `-o` | `--output` |

Some flag names have one meaning everywhere:

| Flag | Meaning |
| ---- | ------- |
| `--dry-run` | Compute and report; write nothing |
| `--check` | Compare with what is committed or recorded, write nothing, exit `2` on a difference (`generate`, `lock`, `sbom`, `export okf`, `migrate v5`, `migrate okf`, `catalog --html`) |
| `--strict` | Warnings fail as well as errors, the same as `--fail-on warning` (`validate`, `doctor`, `verifiers run`) |
| `--strict-config` | `generate` only: an unknown or invalid configuration key fails instead of warning |
| `--refuse-findings` | `lock` only: fail without writing when the security scan refuses a served skill |
| `--output <file>` / `--output-dir <dir>` | Write the result to a file / into a directory |
| `--format` | Encoding of the result: `text` or `json`, plus `sarif`, `junit`, `markdown` or `github` where a command supports them. An unknown value is rejected with the allowed list |
| `--yes` | Skip a confirmation prompt; required in a non-interactive shell for `clean`, `remove`, `approve` and `generate --user` |

Log colors are off when `NO_COLOR` is set, `TERM=dumb`, or stderr is not a terminal; progress bars draw only on a
terminal, so redirected output carries no ANSI codes.

## Finding the project

Without `-C`, a command walks up from the working directory to the repository root (never above your home
directory) and uses the first configuration it finds: `.agents/ai-rulez.toml`, `.ai-rulez/config.toml`,
`.config/ai-rulez/config.toml` or `ai-rulez.toml`. The order and tie-breaks are in
[Configuration discovery](/ai-rulez/configuration/#configuration-discovery).

Outputs are written relative to the directory that holds the configuration. `config.toml` is the only format read:
a `config.yaml`, `config.json`, a flat `ai-rulez.yaml` or a `version = "4.0"` config stops with exit `1`; convert it
with [`ai-rulez migrate v5`](#migrate-a-4x-project). Move a flat `ai-rulez.yaml` to `.ai-rulez/config.yaml` first.

```bash
ai-rulez generate                         # discover the project
ai-rulez -C services/billing/.ai-rulez generate
ai-rulez --config-dir ai-policy generate  # ./ai-policy/config.toml
```

## Exit codes

Every command uses the same codes, so a script can tell a broken setup from a failed check:

| Code | Meaning |
| ---- | ------- |
| `0` | Success |
| `1` | The command could not run: no or invalid configuration, bad flags, an unknown subcommand, a tool or network error, a declined confirmation |
| `2` | The command ran and found something: findings, drift, a failed gate or budget, a refused source |
| `3` | `lock` only: the lock was written, but served skills the security scan refuses were left unpinned |

With `--recursive`, the most severe code across roots wins: `1`, then `2`, then `3`.

What `2` means for the common commands:

| Command | Exit `2` when |
| ------- | ------------- |
| `validate`, `scan` | Findings at or above `--fail-on` (default `error`; `--strict` means `warning`) |
| `generate --check` | A generated file differs from its source |
| `generate --locked`, `--frozen` | An authored source differs from the lock's content pins |
| `lock --check` | The lock does not match the configuration, the sources or the outputs |
| `lock --outdated` | A pinned tag moved or was deleted, or a constraint cannot be satisfied; with `--fail-on-outdated`, any available update |
| `doctor` | An error finding (a warning with `--strict`) |
| `tokens --budget`, `cost --budget` | The always-loaded (or on-demand) tokens exceed the ceiling |
| `convert` | The security scan or validation blocked the write, or `--fail-on` matched |
| `verify` | A signature, approval or plugin bundle does not verify |
| `update` | A source was refused and nothing was written |
| `guard` | The agent tried to edit a generated file |
| `migrate v5 --check`, `migrate okf --check` | A project still needs migration |
| `verifiers run`, `eval run`, `search --eval`, `review --gate`, `judge` | A check, threshold or gate failed |
| `generate`, `validate`, `lock`, `doctor` | The repository loosens the organization policy |

### Output streams

- **stdout is the result**: a list, a plan, a report, a created path, in the requested `--format`. `-q` never hides
  it, so `ai-rulez list rules -q` and `ai-rulez clean --dry-run -q` still print it.
- **stderr is everything else**: progress, confirmations (`Rule added successfully`), warnings, errors, hints and
  prompts.
- Every failure prints `Error: <message>`, then `Hint: <hint>` when there is one, on stderr.
- `clean` and every `remove` ask for confirmation on a terminal; elsewhere they exit `1` with the hint
  `pass --yes to skip the confirmation prompt (required in non-interactive shells)`.

### JSON contracts

With `--format json`, stdout carries exactly one JSON object that starts with `"schema_version"`. A list is wrapped
as `{"schema_version": 1, "items": [...]}`. A change that breaks consumers bumps the version and the schema file; the
schemas are published in [`schema/`](https://github.com/Goldziher/ai-rulez/tree/main/schema).

When a command fails before it has a report, stdout carries the error document
([`schema/error-document.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/error-document.schema.json))
and the exit code is repeated in it:

```json
{
  "schema_version": 1,
  "status": "error",
  "error": "no ai-rulez config found: run `ai-rulez init`: ...",
  "hint": "Run 'ai-rulez init' to create a new configuration file\n...",
  "exit_code": 1
}
```

A command that printed its report and exits `2` (findings, drift, a failed gate) adds no error document.

The report commands and their schemas:

| Command | Schema |
| ------- | ------ |
| `validate --format json`, `scan --format json` | [`validate-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/validate-report.schema.json) |
| `doctor --format json` | [`doctor-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/doctor-report.schema.json) |
| `tokens --format json` | [`tokens-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/tokens-report.schema.json) |
| `cost --format json` | [`cost-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/cost-report.schema.json) |
| `exposure --format json` | [`exposure-report.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-report.v1.schema.json) |
| `exposure inventory --format json` | [`exposure-inventory.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-inventory.v1.schema.json) |
| `exposure areas --format json` | [`exposure-areas.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-areas.v1.schema.json) |
| `exposure collisions --format json` | [`exposure-collisions.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-collisions.v1.schema.json) |
| `exposure leakage --format json` | [`exposure-leakage.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/exposure-leakage.v1.schema.json) |
| `catalog --format json` | [`catalog.v1.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/catalog.v1.schema.json) |
| `catalog diff --format json` | [`catalog-diff.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/catalog-diff.schema.json) |
| `verify --attestation --format json` | [`verify-attestation.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verify-attestation.schema.json) |
| `verify --approvals --format json` | [`verify-approvals.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verify-approvals.schema.json) |
| `verify --plugin --format json` | [`verify-plugin.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verify-plugin.schema.json) |
| `lock --check --format json`, `lock --diff --format json` | [`lock-diff.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/lock-diff.schema.json) |
| `lock --outdated --format json` | [`lock-outdated.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/lock-outdated.schema.json) |
| `lock --subject --format json` | [`lock-subject.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/lock-subject.schema.json) |
| `update --format json` | [`update.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/update.schema.json) |
| `sbom --format json` | [`sbom-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/sbom-report.schema.json) |
| `sign --format json` | [`sign-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/sign-report.schema.json) |
| `scanners list --format json` | [`scanners-list.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/scanners-list.schema.json) |
| `scanners doctor --format json` | [`scanners-doctor.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/scanners-doctor.schema.json) |
| `roles list --format json` | [`roles-manifest.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/roles-manifest.schema.json) |
| `roles show --format json` | [`roles-show.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/roles-show.schema.json) |
| `roles resolve --format json` | [`roles-resolve.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/roles-resolve.schema.json) |
| `verifiers run --format json` | [`verifiers-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verifiers-report.schema.json) |
| `verifiers list --format json` | [`verifiers-list.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verifiers-list.schema.json) |
| `verifiers test --format json` | [`verifiers-test.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verifiers-test.schema.json) |
| `verifiers explain --format json` | [`verifiers-explain.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/verifiers-explain.schema.json) |
| `export okf --format json` | [`export-okf.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/export-okf.schema.json) |
| `okf validate --format json` | [`okf-validate.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/okf-validate.schema.json) |
| `publish --format json` | [`publish-plan.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/publish-plan.schema.json) |
| `publish emit --format json` | [`publish-emit.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/publish-emit.schema.json) |
| `publish verify --format json` | [`publish-verify.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/publish-verify.schema.json) |
| `eval run --format json` | [`eval-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/eval-report.schema.json) |
| `judge --format json` | [`judge-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/judge-report.schema.json) |
| `cache status --format json` | [`cache-status.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/cache-status.schema.json) |
| `cache clear --format json` | [`cache-clear.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/cache-clear.schema.json) |

The content commands (`add`, `remove`, `edit`, `domain`, `profile`, `include`, `skill install|remove`) print a
[change result](#changing-content-from-scripts) instead.

## Set up a project

### `ai-rulez init`

Creates `.ai-rulez/config.toml` and an example content tree (an [OKF](/ai-rulez/okf/) bundle: rules, context and skills
with `index.md` listings). The project name defaults to the directory name. The generated config enables `claude`
and lists every built-in preset in a comment; add the tools you use to `presets` (see
[Supported harnesses](/ai-rulez/harnesses/)).

| Flag | Use it to |
| ---- | --------- |
| `--domains backend,frontend` | Create domain directories ([Domains & profiles](/ai-rulez/domains/)) |
| `--from auto` | Import existing tool files instead of example content ([`init --from`](#init-from)) |
| `--minimal` | Write only a root `ai-rulez.toml`, no `.ai-rulez/` ([Config without a folder](/ai-rulez/config-without-folder/)) |
| `--source agents` | Keep the content in `AGENTS.md` and `.agents/` ([.agents as a source](/ai-rulez/agents-source/)) |
| `--setup-hooks` | Add `ai-rulez validate` to an existing lefthook, pre-commit or husky setup |
| `--force` | Replace an existing configuration directory; the old one is kept as `<dir>.bak-<timestamp>` |

```bash
ai-rulez init acme --domains backend,frontend
```

```text
Created .ai-rulez/ for acme
...
Example content created:
  - rules/code-quality.md
  - context/architecture.md
  - skills/code-reviewer/SKILL.md
  - skills/ai-rulez/SKILL.md
...
```

`init` never deletes an existing configuration: without `--force` (or a yes at the prompt) it refuses. `--yes` only
skips prompts and never replaces a directory. When a hand-written `CLAUDE.md`, `AGENTS.md` or `GEMINI.md` already
exists, `init` warns that `generate` will not overwrite it; import it with `init --from` or
[`convert`](#ai-rulez-convert).

### Content commands

`add`, `edit`, `show`, `list` and `remove` manage the files under `.ai-rulez/`. Each takes a kind: `rule`,
`context`, `skill`, `agent`, `command` or `check` (`list` takes the plural: `rules`, `skills`, ...).

| Flag | Meaning |
| ---- | ------- |
| `--domain <name>` | Work in `domains/<name>/` instead of the root (root content is in every profile) |
| `--content <text>` | Body of the file; without it `add` writes a template. `edit --content -` reads stdin |
| `--priority critical\|high\|medium\|low\|minimal` | Rules, context and skills (default `medium`) |
| `--targets claude,cursor` | Limit the item to these presets or paths ([Rules](/ai-rulez/rules/)) |
| `--description <text>` | Skills, agents, commands and checks |
| `--local` | Work in the machine-local tree `.ai-rulez/local/` (not for checks) |
| `--yes` | `remove`: skip the confirmation |

```bash
ai-rulez add rule api-errors --domain backend --priority high \
  --content "Return RFC 9457 problem details from every handler."
ai-rulez add skill deploy --description "Deploy the billing service with the release checklist"
ai-rulez edit rule code-quality --content "Keep functions under 50 lines."
ai-rulez show rule code-quality
ai-rulez list rules --domain backend
ai-rulez remove rule api-errors --domain backend --yes
```

```text
Rule added successfully
.ai-rulez/domains/backend/rules/api-errors.md
```

Names are file names: no `/`, whitespace, leading `.` or `-`, and no `.md` (it is added). `edit` keeps what the file
declares: bare content replaces only the body, and `--priority` or `--targets` change only that field; content that
starts with its own frontmatter replaces the whole file. `list` without `--domain` lists the root items.
`list --placement` shows which skills and commands end up in the core output or only in a plugin.

Code-review checks (`add check`) take `--severity low|medium|high|critical` and `--tools`; see [Checks](/ai-rulez/checks/).

### Domains and profiles

```bash
ai-rulez domain add qa --description "Test suites"
ai-rulez profile add backend backend qa --set-default   # profile "backend" = domains backend and qa
ai-rulez profile list
ai-rulez generate --profile backend
```

`domain remove` deletes the domain directory and refuses while a profile still lists the domain.
`profile set-default <name>` changes the default. See [Domains & profiles](/ai-rulez/domains/).

### Includes, installed skills and plugins

These commands edit entries in `config.toml`; they print nothing on stdout in text mode.

```bash
ai-rulez include add shared https://github.com/acme/ai-rules --ref v2.1.0
ai-rulez skill install deploy --source https://github.com/acme/skills --version "^1.2"
ai-rulez skill install lint --source ./tools/skills
ai-rulez skill list
```

| Command | Key flags | Page |
| ------- | --------- | ---- |
| `include add <name> <source>` | `--ref`, `--path`, `--include rules,context,skills,agents,commands`, `--merge-strategy local-override\|include-override\|error` | [Includes](/ai-rulez/includes/) |
| `skill install <name> --source <url\|path>` | `--ref` or `--version <range>`, `--path` (default `skills/<name>`), `--include`/`--exclude` globs for a directory of skills, `--domain` | [Installed skills](/ai-rulez/installed-skills/) |
| `skill update [name...]` | Re-pin installed skills in `ai-rulez.lock` (the same as `lock --kind skill`) | [Lock file](/ai-rulez/lockfile/) |
| `plugin install`, `marketplace add` | Import plugins from a marketplace, repository or directory | [Installed plugins](/ai-rulez/installed-plugins/) |

A source URL with a credential in it (`https://user:token@host/...`) is refused, because it would be stored in the
committed config. Authenticate with the git credential helper or `AI_RULEZ_GIT_TOKEN` instead. A local path source
in the shared config must stay inside the project; use `--local` for a path outside it. `include remove` and
`skill remove` work even when the project no longer loads.

### Changing content from scripts

`add`, `remove`, `edit`, `domain add|remove`, `profile add|remove|set-default`, `include add|remove` and
`skill install|remove` accept `--format json` and print one
[change result](https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/change-result.schema.json):

```bash
ai-rulez add context release-process --content "Releases ship on Tuesdays." --format json
```

```json
{
  "schema_version": 1,
  "status": "created",
  "type": "context",
  "name": "release-process",
  "path": ".ai-rulez/context/release-process.md"
}
```

`status` is `created`, `removed` or `updated`; `domain`, `source`, `domains`, `default` and `local` appear when they
apply. In text mode, `add`, `remove`, `edit` and `domain add|remove` print the path they changed on stdout.

## Local configuration

Two layers stay on your machine and are never committed: the `.ai-rulez/local/` content tree (`--local` on
`add`, `edit`, `remove` and `list`) and the `config.local.toml` overlay (`ai-rulez local`, and `--local` on
`profile`, `include` and `skill install|remove`). The full guide is [Local configuration](/ai-rulez/local-overrides/).

| Command | Does |
| ------- | ---- |
| `local init` | Write a commented `config.local.toml` |
| `local show [--reveal]` | Keys the overlay sets, with the shared value each replaces; credential-bearing values are shown as `<redacted>` unless `--reveal` |
| `local set <path> <value>` | Set a key; the value is a TOML literal, else a string (`--string` forces a string) |
| `local set <path> --stdin` | Read the value from stdin, so a secret stays out of shell history |
| `local unset <path>` | Remove a key |
| `local path` | Print the overlay path |

```bash
ai-rulez local set presets '["codex", "!cursor"]'
ai-rulez local set mcp_servers.github.command npx
printf %s "$GITHUB_TOKEN" | ai-rulez local set mcp_servers.github.env.GITHUB_TOKEN --stdin
```

Every change is validated against the merged config and rolled back if invalid. `--local` on `include remove` or
`skill remove` of a shared entry hides it on this machine (`remove = true` in the overlay).

When local configuration changes a file that is shared with the team, `generate` refuses to write it if git tracks
it; `--allow-local-drift` writes it anyway (review `git diff` before committing). `--no-local` on `generate`,
`validate`, `tokens` and most report commands renders the view a teammate without your local files sees. See
[Drift guard](/ai-rulez/local-overrides/#drift-guard).

## Generate outputs

### `ai-rulez generate`

Renders every preset in `presets` for the active profile and writes the files. Alias: `gen`.

| Flag | Use it to |
| ---- | --------- |
| `--profile <name>` | Generate another profile; a comma-separated list composes several ([Composing profiles](/ai-rulez/domains/#composing-profiles)) |
| `--role <name>` | Generate the slice of content a [role](/ai-rulez/roles/) selects |
| `-n`, `--dry-run` | List what would be written (`write-file:`, `unchanged:`, `edited:`, `blocked:`), write nothing |
| `--check` | Write nothing; exit `2` when a file on disk differs ([Detecting drift](#detecting-drift)) |
| `--force` | Overwrite an existing file ai-rulez did not write ([Existing files](#existing-files-generate-will-not-overwrite)) |
| `--recursive` | Process every configuration below the working directory (monorepos; `--exclude <glob>` skips directories) |
| `--watch` | Regenerate on every change ([Watch mode](#watch-mode)) |
| `--locked`, `--frozen` | CI: require `ai-rulez.lock` to cover every remote source and match the content pins; `--frozen` also never uses the network ([Lock file](/ai-rulez/lockfile/)) |
| `--env KEY=VALUE`, `--env-file <file>` | Values for MCP `${VAR}` placeholders (default `.env`) |
| `--gitignore` | Maintain the managed `.gitignore` block (also `gitignore = true` in the config) |
| `--no-local` | Ignore machine-local configuration |
| `-y`, `--yes` | Skip the summary of new hook and MCP commands (also `AI_RULEZ_ACK_COMMANDS=1`); required with `--user` off a terminal |

Other modes: `--plugin` builds distributable plugin bundles ([Authoring plugins](/ai-rulez/plugins/)), `--user` writes your
user-level configuration into each tool's home directory ([User-level configuration](/ai-rulez/user-scope/)), and
`--emit-plan` prints the plan as JSON ([Embedding the plan](#embedding-the-plan)).

```bash
ai-rulez generate
```

```text
INFO  Generating with configuration profile=default
INFO  Generation complete files=10
```

Before writing, `generate` prints every hook command, command-based MCP server and `[permissions] allow` rule that is
new or changed since the last run on this machine (all of them in a fresh clone), even with `-q`:

```text
Warning: generate will write 1 new or changed command(s) that your tools will run (from .ai-rulez/config.toml):
  hook       SessionStart: sh -c 'case "${CLAUDE_CODE_REMOTE:-}" in ...  -> claude
Review them before you start a tool in this project. Silence with --yes or AI_RULEZ_ACK_COMMANDS=1.
```

An MCP `${VAR}` placeholder that no environment variable, `.env` file or `--env` value resolves stops the run:

```text
Error: unresolved MCP env placeholders: github.env.GITHUB_TOKEN references ${GITHUB_TOKEN}

Hint: Set missing values with --env KEY=VALUE, an environment variable, or .env
```

With `--format json`, stdout is `{"schema_version": 1, "status": "generated", "files_written": 10}`
(`"status": "dry_run"` with a `plan` for `--dry-run`).

### Existing files generate will not overwrite

`generate` replaces a file only when it can prove ai-rulez wrote it: the file carries the generated header or
`Content-Hash`, the manifest of an earlier run lists it, or `convert --write` imported it and it is unchanged since.
Any other file at an output path, such as a hand-written `AGENTS.md`, stops the run before anything is written:

```text
Error: refusing to overwrite 1 existing file(s) ai-rulez cannot prove it wrote: AGENTS.md (an existing file
ai-rulez did not write). Import the file with `ai-rulez convert --write`, move or delete it, or pass --force to
overwrite it
```

Choose one:

- `ai-rulez convert --write` imports the file into `.ai-rulez/`; the next `generate` replaces it.
- Move or delete the file.
- `ai-rulez generate --force` overwrites it.

`generate --dry-run` shows the same case as `blocked: AGENTS.md (an existing file ai-rulez did not write)` and exits
non-zero. Exceptions:

- A hand-written file in a rule folder (`.cursor/rules/react.mdc`) is kept, and the generated rule is written under
  another name (`react.ai-rulez.mdc`) with a warning.
- Shared settings documents (`.claude/settings.json`, `.mcp.json`, `opencode.json`, ...) are merged into, not
  replaced.
- A symlinked output is never written through. `CLAUDE.md -> AGENTS.md` is accepted when `AGENTS.md` is generated in
  the same run; a link to anything else is refused.

### Detecting drift

`generate --check` renders in memory, writes nothing, and prints one line per file that differs. Commit the
generated files and run it in CI:

```bash
ai-rulez generate --check --no-local
```

```text
edited: CLAUDE.md
1 generated file(s) differ from their sources; run `ai-rulez generate` and commit the result
```

| Line | Meaning |
| ---- | ------- |
| `missing: <path>` | A generated file is not on disk |
| `stale: <path>` | The sources changed; `generate` would rewrite the file |
| `edited: <path>` | The file was edited by hand (its body no longer matches its `Content-Hash`); `generate` overwrites the edit |
| `orphan: <path>` | Nothing renders this file any more; `generate` would delete it |
| `blocked: <path>` | Machine-local configuration would change a tracked file (see [Local configuration](#local-configuration)) |

The managed `.gitignore` block is checked too (`stale: .gitignore (managed block)`). Exit codes: `0` in sync, `2` a
file differs, `1` the check could not run (including an unset MCP placeholder: `--check` renders the outputs and
needs the same values as `generate`). `--format json` prints `{"status": "drift", "differing": [{"kind": "edited",
"path": "CLAUDE.md"}], ...}`. `--check` cannot be combined with `--dry-run`, `--plugin` (use `verify --plugin`) or
`--gitignore`.

### Watch mode

`generate --watch` generates once, then regenerates whenever `.ai-rulez/`, the local overlay, or a local include
changes. Remote includes are not polled. Stop it with Ctrl-C.

- Changes are debounced (300 ms), so a `git checkout` triggers one run; runs never overlap.
- A configuration that fails to load is logged and watching continues, so the next save can fix it.
- Generated files and editor swap files never trigger a run.
- On Linux, a low `fs.inotify.max_user_watches` limit produces a warning; raise it if changes are missed.

`--watch` cannot be combined with `--dry-run`, `--check`, `--user`, `--plugin` or `--recursive`.

### Embedding the plan

`generate --emit-plan <file>` (`-` for stdout) writes what a run would do as JSON and changes nothing on disk. The
document follows [`schema/plan.schema.json`](/ai-rulez/schema/):

- `files`: every output, sorted by path, with `action` (`write`, or `merge` into a document you share with
  ai-rulez), `mode`, `size` and `sha256` of the content;
- `removals`: files an earlier run wrote that this run no longer renders, and documents ai-rulez takes its entries out
  of.

```bash
ai-rulez generate --emit-plan plan.json --profile backend
```

The plan holds no secrets: MCP placeholders stay as written, and an output that may carry a secret has no digest. It
honors `--profile`, `--role` and `--no-local`. The [Go API](/ai-rulez/embedding/) returns the same document.

### `ai-rulez clean`

Removes what `generate` wrote: the generated files, the directories that become empty, the generated manifests, the
managed `.gitignore` block and the keys ai-rulez merged into shared settings documents. The `.ai-rulez/` sources are
never touched.

| Flag | Use it to |
| ---- | --------- |
| `-n`, `--dry-run` | Print the plan only |
| `-y`, `--yes` | Skip the confirmation; required in a non-interactive shell (otherwise exit `1`, nothing removed) |
| `--include-edited` | Also remove generated files you edited by hand (kept with a warning by default) |
| `--keep-gitignore`, `--keep-manifest` | Leave the `.gitignore` block or the manifest in place |
| `--profile <name>`, `--user` | Clean another profile's outputs, or what `generate --user` wrote |

```bash
ai-rulez clean --dry-run
```

```text
Clean plan (profile backend)
  remove file: .agents/skills/code-reviewer/SKILL.md
  remove file: .claude/settings.json
  remove file: .codex/config.toml
  remove file: AGENTS.md
  remove file: CLAUDE.md
  ...
  remove dir (if empty): .claude
  remove manifest: .ai-rulez/.generated-manifest.json
Dry run: no files were removed
```

`clean` deletes a file only when it can prove ai-rulez wrote it (the same proof as
[`generate`](#existing-files-generate-will-not-overwrite)); anything else is kept and named in a
`Stale files not removed` warning. It never removes a symlink, never deletes a settings document that existed before
ai-rulez wrote to it, and keeps the generated file that replaced an original `convert` imported. `clean` has no
`--recursive`; run it in each root.

<a id="guard-command"></a>

### Hooks the tools run: `guard` and `session-start`

These commands are run by a coding assistant's hooks, not by people.

`ai-rulez guard` is the `PreToolUse` hook that `generate` installs when `[guard] generated = true`
([Hooks and settings](/ai-rulez/settings/)). It reads the hook payload on stdin and exits `2`, with the source file to edit instead on
stderr, when the tool call would edit a generated file. Everything else, including a payload it cannot parse, exits
`0`.

```bash
echo '{"tool_name":"Edit","tool_input":{"file_path":"AGENTS.md"}}' | ai-rulez guard; echo $?
```

`ai-rulez session-start` is the `SessionStart` hook that regenerates the outputs when a remote Claude Code session
starts; see [Run generate in remote sessions](/ai-rulez/session-start/). It always exits `0`.

## Import existing configuration

<a id="convert-command"></a>

### `ai-rulez convert`

Reads another tool's files and writes the equivalent `.ai-rulez/` tree, with a report that classifies every input
construct. It never modifies the source files, never runs anything it imports, uses the network only with
`--fetch`, and writes only with `--write`.

```bash
ai-rulez convert --dry-run
```

```text
source      native (.)
into        .ai-rulez
plan        1 rules, 1 context, 1 mcp servers
written     (nothing written)

CREATE      config.toml
CREATE      context/claude.md
CREATE      rules/react.md

NEEDS-ACTION  .mcp.json  mcpServers.github.env.GITHUB_TOKEN  literal credential replaced by a ${GITHUB_TOKEN} reference; export the variable before generating

Summary: 3 mapped, 0 approximated, 0 dropped, 1 needs-action, 0 unsupported.
Security scan: 1 finding(s).
  warning AR012 .ai-rulez/config.toml:9 MCP server "github" runs "@modelcontextprotocol/server-github", but no version is pinned; ...
Validation of the planned tree: 0 error(s), 0 warning(s).

Dry run: nothing was written. Rerun with --write to create the files.
```

Then run `ai-rulez convert --write` and `ai-rulez generate`. The first `generate` replaces the root files `convert`
imported (`CLAUDE.md`, `AGENTS.md`, ...) as long as they are unchanged since; commit `.ai-rulez/.converted.json`,
which records them. Imported files in rule folders are not replaced: `generate` keeps `.cursor/rules/react.mdc` and
writes `react.ai-rulez.mdc` beside it with a warning, so delete the imported rule-folder files after
`convert --write`.

| Flag | Use it to |
| ---- | --------- |
| `-n`, `--dry-run` / `--write` | Preview or write. With neither, a terminal gets a dry run and a script is refused |
| `--from <importers>` | Choose importers (default `auto`: every importer that detects something); `--list` shows what each detects |
| `--domain <name>` | Put the imported content in `domains/<name>/`, safe next to an existing tree |
| `--merge` | Add to an existing tree without touching its files: a name that is taken gets `-imported` |
| `--force` | Overwrite existing content files that differ. `config.toml` is always merged, never replaced; when it gains entries it is rewritten and loses its comments |
| `--split-headings` | Split a root file such as `CLAUDE.md` into one context per H2 heading (default: one context per file) |
| `--enable-hooks`, `--enable-permissions` | Write imported hooks and `allow` rules live; by default they are commented out for review |
| `--fetch` | Read the remote git sources the input names (https only) |
| `--lock` | Run `ai-rulez lock` after a successful `--write`. The lock renders the outputs, so set the `${VAR}` placeholders that replaced literal MCP credentials first; otherwise `lock` fails after the files are written: set them and run `ai-rulez lock` |
| `--fail-on <statuses>` | Exit `2` when a finding has one of these statuses (the files are still written) |
| `--delivery static\|served\|both` | How the imported skills reach the agent. Without `--domain` it sets `[skills] delivery`, which also applies to skills already in the project; prefer it with `--domain` |
| `--format json`, `--report <file>` | Machine-readable report ([`schema/convert-report.schema.json`](https://github.com/Goldziher/ai-rulez/blob/main/schema/convert-report.schema.json)) |

`--source`, `--into`, `--keep-names`, `--best-effort` and `--list` are in the
[Command reference](/ai-rulez/cli-reference/#ai-rulez-convert).

#### Importers

| Importer | Reads |
| -------- | ----- |
| `native` | Root files (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `.github/copilot-instructions.md`, `.cursorrules`, ...), rule folders (`.cursor/rules`, `.github/instructions`, `.kiro/steering`, `.windsurf/rules`, `.clinerules`, ...), skills, agents and commands of every preset, MCP files (`.mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json`, ...), and hooks and permissions (`.claude/settings.json`, `.codex/hooks.json`, `.cursor/hooks.json`, ...) |
| `rulesync` | A [rulesync](https://github.com/dyoshikawa/rulesync) project: `rulesync.jsonc` and `.rulesync/` |
| `apm` | A [Microsoft APM](https://github.com/microsoft/apm) project: `apm.yml`, `.apm/`, `apm_modules/`, `apm.lock.yaml` |
| `tessl` | `tessl.json` and the vendored `.tessl/` plugins, with their eval scenarios as [eval cases](/ai-rulez/evals/) |
| `okf` | An [OKF](/ai-rulez/okf/) bundle at the source root or in `docs/okf` (the same mapping as `import okf`) |
| `skills-lock` | `skills-lock.json` of the Vercel skills CLI, as `[[installed_skills]]` |
| `agent-plugins` | An [Agent Plugins](/ai-rulez/agent-plugins/) directory: `plugin.json`, `skills/`, `mcp.json` |

Next to a rulesync, APM or Tessl project, `auto` skips `native`, because the tool files there are that tool's
generated output. `--from native,rulesync` reads both.

#### Report statuses

| Status | Code | Meaning |
| ------ | ---- | ------- |
| `mapped` | | Carried over unchanged in meaning |
| `approximated` | `AR9F1` | Carried over with a change in meaning; review it (for example a tool-specific frontmatter key that only some presets render) |
| `dropped` | `AR9F2` | Not carried; the finding says why (for example a key no ai-rulez field holds, or a file ai-rulez generated) |
| `needs-action` | `AR9F3` | You have to do something: set an environment variable, run `ai-rulez lock`, review a commented hook |
| `unsupported` | `AR9F4` | ai-rulez has no equivalent |

`AR9F5` marks a write blocked by the security scan, and `AR9F0` an input file that cannot be parsed at all. These
codes appear in the convert report only, never in `validate`.

#### What convert changes on the way in

- **Frontmatter** is translated: Cursor `alwaysApply`/`globs`, Copilot `applyTo`, Kiro `inclusion` and Windsurf
  `trigger` become `activation`, `globs` and `description`.
- **Secrets**: a literal credential in an MCP server's `env`, `headers`, arguments or URL is replaced by a `${VAR}`
  reference (`needs-action`); the value is never printed or written. Set the variable before you run `generate`.
- **Hooks and `allow` rules** are written as a commented block at the end of `config.toml` until you pass
  `--enable-hooks` or `--enable-permissions`, or uncomment the lines yourself. `ask` and `deny` rules are always live.
  Each hook keeps the tool it came from in `targets`. See [Hook translation](#hook-translation).
- **Presets** are inferred from files only one tool owns (`.cursor/rules` implies `cursor`); `AGENTS.md` alone
  implies none, and `claude` is used and reported.
- **Duplicates**: identical content in several files (`CLAUDE.md` and `AGENTS.md`) is imported once.
- Files ai-rulez generated are never read back as sources.

#### Safety and exit codes

Before writing, the planned tree is validated and security-scanned in a scratch directory. A secret or risky command
stops the run with exit `2`, nothing written, and the finding is reported at the source file and line it came from.
Remove it from the source, or pass `--allow-findings AR001` to write anyway. Writes are atomic: if one fails, every
file written so far is restored.

| Exit | Meaning |
| ---- | ------- |
| `0` | Done (the report may list losses), or nothing to convert |
| `1` | Could not run, or would overwrite an existing file that differs (use `--merge`, `--domain` or `--force`) |
| `2` | Blocked by the scan or validation, or `--fail-on` matched |

#### Importer mappings

Run `ai-rulez convert --from <importer> --dry-run` to see every construct of your project that does not carry over,
with the reason. The main mappings:

**rulesync**

| rulesync | ai-rulez | Notes |
| -------- | -------- | ----- |
| `rules/*.md` | `rules/` | `targets` map to presets: `claudecode` is `claude`, `codexcli` is `codex`, `*` is everywhere. With no known target left, the rule reaches no output until you fix `targets` |
| `rules/*.md` with `root: true` | `context/` | Context is what ai-rulez writes into every root file |
| `rules/*.md` with `localRoot: true` | `.ai-rulez/local/context/` | Machine-local; `.ai-rulez/local/` is added to `.gitignore` |
| `commands/**/*.md` | `commands/` | Nested paths become the name: `git/commit.md` is `git-commit` |
| `subagents/**/*.md` | `agents/` | `claudecode.model`, `tools`, `skills` and `effort` are lifted to the top level, so they apply to every preset; other tool sections are dropped |
| `skills/<name>/`, `checks/*.md` | `skills/<name>/`, `checks/` | Resource files copied |
| `mcp.jsonc` / `mcp.json` | `[[mcp_servers]]` | `local` is `stdio`, `streamable-http` is `http` |
| `rulesync.jsonc` `targets` | `presets` | |
| `rulesync.jsonc` `sources` | `[[installed_skills]]`, `rules/` | Only with `--fetch` |
| `hooks.jsonc`, `permissions.jsonc` | `[[hooks]]`, `[permissions]` | Commented out by default |
| `rulesync.local.jsonc` | | `needs-action`: move personal overrides to `config.local.toml` |

**APM**

| APM | ai-rulez | Notes |
| --- | -------- | ----- |
| `.apm/instructions/*.instructions.md` | `rules/` | `applyTo` becomes `globs` |
| `.apm/agents/`, `.apm/chatmodes/` | `agents/` | |
| `.apm/prompts/*.prompt.md` | `commands/` | |
| `.apm/skills/<name>/`, `.apm/context/` | `skills/<name>/`, `context/` | |
| `.apm/hooks/*.json` | `[[hooks]]` | Commented out by default; scripts are not copied |
| `apm.yml` `target` | `presets` | `vscode` and `copilot` are `copilot`, `windsurf` is `devin` |
| `apm.yml` `dependencies.apm` | Package content | Installed or local packages are imported as files; others need `--fetch` |
| `apm.yml` `dependencies.mcp` | `[[mcp_servers]]` | Self-defined servers only; a bare registry name is `unsupported` |

**Tessl**

| Tessl | ai-rulez | Notes |
| ----- | -------- | ----- |
| `.tessl/plugins/.../skills/<name>/` | `skills/<name>/` | |
| `.tessl/plugins/.../rules/*.md` | `rules/` | |
| `evals/<scenario>/task.md` + `criteria.json` | `skills/<skill>/evals/<scenario>.eval.yaml` | The checklist becomes the rubric; weights are dropped |
| `docs/` | | Dropped: copy what the agent needs into `context/` |

Nothing is fetched from the Tessl registry: run `tessl install` before converting.

**`--fetch` results**

| Input | Result |
| ----- | ------ |
| rulesync source with `skills` | One pinned `[[installed_skills]]` entry per selected skill |
| rulesync source with `rules` | The selected rule files copied into `rules/` |
| APM dependency that is a single skill (`owner/repo/skills/name`) | One `[[installed_skills]]` entry |
| Other APM dependency | The package's primitives copied into the tree; its own dependencies are not followed |

#### Hook translation

- Event names become Claude Code's: Gemini `BeforeTool` is `PreToolUse`, Cursor `beforeSubmitPrompt` is
  `UserPromptSubmit`, rulesync `sessionStart` is `SessionStart`.
- Millisecond timeouts become seconds, rounded up.
- A matcher in a tool's own vocabulary (Gemini `run_shell_command`, Cursor `Shell`) is kept as `matchers.<harness>`.
- An event with no ai-rulez equivalent (Cursor `afterFileEdit`) and a handler that is not a command (`prompt`, `http`,
  `agent`) are `unsupported`.
- Cursor `.cursor/cli.json` permissions are rewritten: `Shell(cmd:args*)` becomes `Bash(cmd args:*)`, `Write` becomes
  `Edit`, `Mcp(server:tool)` becomes `mcp__server__tool` (`approximated`).

<a id="init-from"></a>

<a id="importing-with-init-from"></a>

#### Importing with `init --from`

`ai-rulez init --from <sources>` runs `convert --write` while creating the project. Sources are importer names
(`auto`, `rulesync`, ...) or project paths (`.claude,.cursor,CLAUDE.md`), which limit the native importer to them.

```bash
ai-rulez init web --from auto
```

```text
plan        1 context
written     2 file(s)

CREATE      config.toml
CREATE      context/agents.md

APPROXIMATED  (project)  presets -> presets  no tool-specific file identifies a preset (AGENTS.md and .agents/skills are shared by many tools); defaulting to claude, adjust `presets` in config.toml
...
```

The import is checked in a scratch directory first, so a failed import leaves an existing `.ai-rulez/` alone. A root
file becomes one context item; use `convert --split-headings` to split it.

<a id="okf-commands"></a>

### OKF bundles

[Open Knowledge Format](/ai-rulez/okf/) export, import and lint:

```bash
ai-rulez export okf --output-dir docs/okf        # write rules, context, skills, ... as a bundle
ai-rulez export okf --check                      # CI: exit 2 when the bundle on disk is stale
ai-rulez import okf ./vendor/kb --domain kb      # bundle (directory or git URL) into .ai-rulez/
ai-rulez okf validate https://github.com/acme/kb # lint any bundle (AR9B0-AR9B9)
ai-rulez migrate okf --dry-run                   # convert .ai-rulez/ itself to an OKF bundle, in place
```

`export okf` writes to `okf.dir` (default `docs/okf`) and refuses a non-empty directory that is not already a bundle.
`import okf` never overwrites a file unless `--force`, and refuses the import when the security scan finds an error.
All exit `2` when they ran and found problems.

### Migrate a 4.x project

```bash
ai-rulez migrate v5 --dry-run    # print the change list
ai-rulez migrate v5              # rewrite the project
ai-rulez migrate v5 --check      # CI: exit 2 while a project still needs migration
ai-rulez generate
```

`migrate v5` converts a YAML or JSON config to `config.toml`, sets `version = "5.0"`, renames removed presets and
keys, and keeps the four changed defaults (`agents_md`, `gitignore`, `[header] hashes`, `[mcp] self_server`) at their 4.x
values unless `--adopt-defaults`. `--recursive` migrates every
project below the working directory; `--write` also rewrites deprecated frontmatter spellings. Running it twice changes
nothing. A 2.x or 3.x project must first go through ai-rulez 4.x. See [Migrating to v5](/ai-rulez/migration-v5/).

## Validate and inspect

### `ai-rulez validate`

Checks the configuration against its schema, then lints the content: references, globs, hooks, MCP servers,
descriptions, budgets, duplicates and the security checks. Every finding has a code; the list is in
[Strict validation](/ai-rulez/strict-validation/). Alias: `val`.

| Flag | Use it to |
| ---- | --------- |
| `--strict` / `--fail-on warning\|info\|none` | Choose the lowest severity that exits `2` (default `error`) |
| `--format json\|sarif\|github\|junit\|markdown`, `-o <file>` | Report for CI and code scanning |
| `--changed[=REV]`, `--since <rev>` | Report only files changed since a revision, and the files that refer to them |
| `--explain <code>` | Print what a rule checks, a bad and good example, and how to suppress it |
| `--fix` (`-n` for a diff) | Apply the safe automatic fixes to authored sources |
| `--baseline <file>`, `--update-baseline` | Accept the current findings and fail only on new ones |
| `--recursive`, `--no-local`, `--config-only` | Every root; the shared view; the configuration file only |

```bash
ai-rulez validate --strict
```

```text
.ai-rulez/rules/index.md:1: warning AR9B0 okf-index-mismatch: concept tests.md is not listed
.ai-rulez/rules/tests.md:1: info AR9B4 okf-orphan: no index entry or concept links to this file

validation: 0 error(s), 1 warning(s), 1 info across 1 root(s)
  AR9B0 okf-index-mismatch           1
  AR9B4 okf-orphan                   1
...
```

Exit codes: `0` valid, `1` the configuration is invalid or does not load, `2` findings at or above `--fail-on`.
Without any configuration, `validate` lints the instruction files it detects
([Without a configuration file](/ai-rulez/strict-validation/#without-a-configuration-file)).

```bash
ai-rulez validate --explain AR012
ai-rulez validate --changed=origin/main --format github   # pull-request annotations
ai-rulez validate --fix --dry-run                          # show the fixes as a diff
```

### `ai-rulez scan`

The security checks only (`AR0xx`): secrets, hidden characters, prompt injection, risky shell, unpinned remotes and
MCP packages, exfiltration and malware-shaped content. Offline and deterministic. It takes the reporting, baseline and
changed-only flags of `validate`. `--external` also runs the scanners declared in `[[lint.external]]`; `ai-rulez
scanners list` and `ai-rulez scanners doctor --all` show whether they are installed and configured. See
[Strict validation](/ai-rulez/strict-validation/#security-checks).

```bash
ai-rulez scan --format sarif -o scan.sarif
ai-rulez scan --external --dry-run    # what each external scanner would run
```

### `ai-rulez doctor`

Read-only diagnostics of the setup, offline. It reports configuration errors, unknown presets, unresolved MCP
placeholders, uncached remote includes, drift, outputs git does not ignore, settings documents that no longer parse,
missing hook scripts, lock mismatches, the `SessionStart` hook and tool binaries not on `PATH`.

```bash
ai-rulez doctor
```

```text
SEVERITY  CHECK          MESSAGE
warning   drift          AGENTS.md: generated file is stale
                           fix: run `ai-rulez generate`
info      tools          preset "codex": codex not found on PATH; its files are still generated
...
0 errors, 3 warnings, 3 info
```

Exit `0` with no errors, `2` with an error (or a warning with `--strict`), `1` when it cannot run. `--profile`
chooses the profile rendered for the drift checks; `--format json` follows `schema/doctor-report.schema.json`.

<a id="tokens-command"></a>

### `ai-rulez tokens`

Reports how many prompt tokens the generated files cost, per runtime (one tool's files) and by when they load. It
measures the rendered output in memory, so it is correct before you run `generate`.

| Bucket | Paid |
| ------ | ---- |
| `always` | On every request: the root instructions file and the skill, command and agent listing a tool shows at session start |
| `conditional` | Only in some modes: path-scoped rule files, machine-local files |
| `on demand` | When the item is opened: skill, command and agent bodies |
| `unmodeled` | Cost ai-rulez cannot model, such as MCP tool schemas |

```bash
ai-rulez tokens
```

```text
Token surface — profile "backend"
Tokenizer: cl100k_base (approximate)

runtime claude — 5 files
  always loaded                                                 545
    CLAUDE.md                                                   135
    AGENTS.md (imported)                                        321
    skill listing (2)                                            89
  conditional                                                    45
    path-scoped rule files                                       45
  on demand                                                     724
    skill bodies (2)                                            561
...
```

| Flag | Use it to |
| ---- | --------- |
| `--budget <n>` | Exit `2` when the largest runtime's always-loaded tokens exceed `n` |
| `--compare-profiles <p>` | One column per profile (repeat the flag) |
| `--by-role`, `--role <name>` | Compare or report [roles](/ai-rulez/roles/) |
| `--format json` | `always_loaded` is the figure a budget gate reads |

The numbers are approximate (the default counter is the `cl100k_base` vocabulary) and cover only the files ai-rulez
generates, not the tool's own system prompt. Runtimes are not additive: one session loads one tool's files, so the
headline is the largest runtime. Use the numbers to compare profiles and edits.

To cut the always-loaded cost, shorten skill descriptions, scope rules with globs, and drop the agent roster from
root files with `builtins = ["!agent-delegation"]` ([Configuration](/ai-rulez/configuration/)).

<a id="cost-command"></a>

### `ai-rulez cost`

Ranks the rules, context files, skills, agents and commands that cost the most, split into always-loaded and
on-demand tokens, so you know what to trim.

```bash
ai-rulez cost --top 3
```

```text
Context cost - profile "backend", target claude (cl100k_base, approximate)

  always loaded         545 tokens
  conditional            45 tokens
  on demand             724 tokens

Top always-loaded items (estimate from sources):
        88   16%  context architecture
        52   10%  rule code-quality
        50    9%  skill ai-rulez

Top on-demand items (estimate from sources):
       471   65%  skill ai-rulez
        90   12%  skill code-reviewer
...
```

`--target <preset>` picks the runtime (default the largest), `--format markdown` suits a pull-request comment, and
`--budget <n>` / `--on-demand-budget <n>` exit `2` when exceeded. The per-item figures are estimates from the sources
and do not add up exactly to the runtime totals.

### `ai-rulez verifiers`

Runs the deterministic repository checks declared as `[[verifiers]]` or under `.ai-rulez/verifiers/` (a file exists,
a regex is forbidden, generated files are in sync, ...). Verifiers never write.

```bash
ai-rulez verifiers run --since origin/main
ai-rulez verifiers list
ai-rulez verifiers explain no-console-log
ai-rulez verifiers test        # run each verifier's examples offline
```

`command` predicates run only with `--allow-exec`, and `llm` predicates only with `--allow-llm`. `--format` takes
`json`, `sarif` or `junit`. Exit `2` when a verifier fails at `--fail-on` severity. See [Verifiers](/ai-rulez/verifiers/).

### More inspection commands

| Command | Shows | Page |
| ------- | ----- | ---- |
| `ai-rulez config layers` | The `config.<name>.toml` layers and which are active | [Configuration](/ai-rulez/configuration/) |
| `ai-rulez roles list\|show\|resolve` | Roles, their parents and the items each keeps | [Roles](/ai-rulez/roles/) |
| `ai-rulez catalog` | Every item with owner, version, tokens, roles and lock status; `--html <dir>` writes a static site, `catalog diff` compares two | [Catalog](/ai-rulez/catalog/) |
| `ai-rulez exposure` | What a session loads at start and what it costs, against the `[exposure]` area map. Currently supports Claude Code only | [Session exposure](/ai-rulez/exposure/) |
| `ai-rulez builtins list\|show` | The built-in domains bundled in the binary | [Configuration](/ai-rulez/configuration/) |
| `ai-rulez llm doctor\|estimate` | The resolved `[llm]` model setup; the cost of a prompt, without calling a model | [LLM access](/ai-rulez/llm/) |

## Pin and verify

<a id="lock-command"></a>

### `ai-rulez lock`

Writes `ai-rulez.lock` in the configuration directory. It pins the commit and file-tree digest of every remote
include and installed skill, a digest of every authored rule, context file, skill, agent, command, hook and role, and
a digest of every generated output. Commit it. Once it exists, `generate` fetches exactly the pinned commits and fails
when the fetched files differ. See [Lock file](/ai-rulez/lockfile/).

```bash
ai-rulez lock                  # pin everything (network for remote sources)
ai-rulez lock shared           # re-pin one include or skill, keep the rest
ai-rulez lock --content-only   # re-pin authored content and outputs, offline
ai-rulez lock --check          # CI, offline: exit 2 when anything differs
ai-rulez lock --diff           # what `lock` would change; exits 0
ai-rulez lock --outdated       # sources whose version range allows a newer tag
```

```text
$ ai-rulez lock --check
ai-rulez.lock does not match /home/dev/acme/.ai-rulez:
  source changed  rule tests (rules/tests.md)
  output changed  output .claude/rules/tests.md
  output changed  output AGENTS.md
run `ai-rulez lock` to refresh it (after reviewing the change with `ai-rulez lock --diff`)
```

| Flag | Use it to |
| ---- | --------- |
| `--check` | Verify the lock offline; exit `2` naming each difference (and verify the signature when `[signing] require` names the lock) |
| `--diff` | Review a lock change in a pull request (`--format json`) |
| `--outdated` (`--fail-on-outdated`) | List available updates within each `version` range; [`update`](#ai-rulez-update) moves the pins |
| `--kind include\|skill\|source\|served` | Limit a refresh to one kind |
| `--subject` | Print the digest a signature over the lock covers ([Signing the lock](/ai-rulez/lockfile/#signing-the-lock)) |
| `--accept-findings` | Pin a source although the security scan of its new tree has error findings |

Exit codes: `0` ok, `1` could not run (including `--check` without a lock), `2` `--check` found drift or `--outdated`
found a moved tag or unsatisfiable range, `3` written but served skills the scan refuses were left unpinned
(`--refuse-findings` makes that exit `2` with nothing written).

`validate` reports a remote source that follows a moving ref without a pin as `AR010`. While `ai-rulez.lock` exists,
enforcement is on: `validate` also reports content drift (`AR981`, `AR982`), `AR010` becomes an error, and `generate`
refuses a remote source the lock does not cover. `[lock] enforce = false` opts out. In CI, use `generate --locked`
(or `--frozen` to also stay offline). See [Lock file](/ai-rulez/lockfile/).

### `ai-rulez update`

Moves the pins of sources that declare a version range (`version = "^1.2"`) to the newest tag the range allows. Only
the lock changes.

```bash
ai-rulez lock --outdated
ai-rulez update --dry-run                   # tags, commits, changed files, scan result
ai-rulez update shared
ai-rulez update --kind skill
ai-rulez update --major --write-config      # also take a new major version, rewriting `version`
```

A tag that moved since it was pinned is refused (`AR732`) until you pass `--accept-moved-tag`; a downgrade needs
`--allow-downgrade`. Exit `2` when a source is refused, with nothing written. `skill update` re-pins plain refs and
keeps range pins; `update` moves range pins. See [Version constraints](/ai-rulez/lockfile/#version-constraints).

### `ai-rulez approve`

Records in the lock that a reviewer read content with a given digest, for the items `[governance]` says need
approval. Items are named `kind:id` (`include:shared`, `skill:backend/deploy`).

```bash
ai-rulez approve --list
ai-rulez approve --diff include:shared
ai-rulez approve include:shared --reviewer alice@example.org --note "read run.sh" --yes
ai-rulez approve --revoke include:shared
ai-rulez approve --verify-base origin/main   # CI: approvals added together with their content (AR716)
```

`--sign` signs the approval, and `--from-github-review <pr>` records the approving reviews of a pull request. See
[Approvals](/ai-rulez/approvals/).

### `ai-rulez sign`

Signs one subject into a Sigstore bundle: the lock (`--lock`), a plugin bundle (`--bundle <dir>`), a skill directory
(`--skill <dir>`), an SBOM (`--sbom <file>`) or an organization policy (`--org-policy <file>`).

```bash
ai-rulez sign --lock --key cosign.key        # key file, offline
ai-rulez sign --lock --keyless               # Fulcio and Rekor (network; the identity goes to a public log)
ai-rulez sign --bundle dist/acme-plugin --key awskms:///alias/release --provenance
```

The key password comes from `AI_RULEZ_SIGNING_KEY_PASSWORD` or `COSIGN_PASSWORD`. See [Signing](/ai-rulez/signing/).

<a id="verify-command"></a>

### `ai-rulez verify`

Verifies signatures, approvals and provenance. It needs a mode; for generated-file drift use
[`generate --check`](#detecting-drift).

| Mode | Verifies |
| ---- | -------- |
| `--attestation` | The signed lock against the `[signing]` policy, offline (`--bundle`, `--skill`, `--sbom` for other subjects) |
| `--approvals` | Signed and review-linked approvals of the current content (`--online` checks the forge) |
| `--plugin` | Generated plugin bundles against their provenance hashes (`--recursive`, `--if-configured` for CI) |
| `--self` | This ai-rulez binary against its release's Sigstore bundle |

```bash
ai-rulez verify --attestation
ai-rulez verify --recursive --plugin --if-configured
```

Exit `0` verified, `1` cannot run (including no mode), `2` verification failed. `ai-rulez trust update` caches the
Sigstore trusted root for offline verification of keyless signatures.

### `ai-rulez sbom`

Prints a CycloneDX 1.6 (default) or SPDX 2.3 (`--type spdx-json`) bill of materials of the AI configuration:
authored items, remote sources, MCP servers and approval status. Output is byte-identical across runs and holds no
secrets.

```bash
ai-rulez sbom -o sbom.cdx.json
ai-rulez sbom -o sbom.cdx.json --check    # CI: exit 2 when the committed file is stale
```

`--require-lock` and `--strict-pins` fail when content is not pinned. See [SBOM](/ai-rulez/sbom/).

## Package and publish

| Command | Does | Page |
| ------- | ---- | ---- |
| `ai-rulez generate --plugin` | Build distributable plugin bundles and a marketplace index from `[plugin]` | [Authoring plugins](/ai-rulez/plugins/) |
| `ai-rulez verify --plugin` | Check the bundles against their sources | [Authoring plugins](/ai-rulez/plugins/) |
| `ai-rulez publish` | Validate, gate and write reproducible, checksummed release artifacts to `dist/`; `--to github-release\|npm\|oci --execute --yes` uploads them | [Publish](/ai-rulez/publish/) |
| `ai-rulez publish verify <dir>` | Recompute every digest and verify the signature of a release | [Publish](/ai-rulez/publish/) |

`publish` writes nothing with `--dry-run` and uploads only with `--execute --yes`.

## Review and evaluate skills

These commands send content to a model only when you allow it (`--allow-llm`, or `--semantic` with
`[llm] allow_network = true`), and print the cost estimate first with `--estimate`. Model setup is in
[LLM access](/ai-rulez/llm/).

| Command | Does | Page |
| ------- | ---- | ---- |
| `ai-rulez review` | Score skills, agents, commands and rules against a rubric; offline by default, `--semantic` adds a judge | [Review](/ai-rulez/review/) |
| `ai-rulez rubric list\|show\|lint` | Inspect and lint rubrics | [Review](/ai-rulez/review/) |
| `ai-rulez judge` | Review agent configuration for security, quality and governance with verified quotes; needs no config | [Judge](/ai-rulez/judge/) |
| `ai-rulez eval run` | Run skill eval cases through a runner and score them | [Evals](/ai-rulez/evals/) |
| `ai-rulez improve` | (experimental) Improve a skill with an external optimizer behind a held-out eval gate | [Improve](/ai-rulez/improve/) |

```bash
ai-rulez review                                # offline rubric score
ai-rulez judge --estimate                      # units, models and cost; sends nothing
ai-rulez eval run --mode activation --surface retrieval   # does the right skill rank first? offline, free
```

<a id="search-command"></a>

### `ai-rulez search`

Ranks the skills `mcp --serve-skills` serves against a query, with the same ranker as the `find_skill` tool: lexical
BM25F by default (offline), or hybrid with an embedding index.

```bash
ai-rulez search "deploy to staging"
ai-rulez search index --dry-run        # what building the embedding index would send
ai-rulez search --eval cases.yaml --min top1=0.8
```

`--limit` (default 5), `--mode lexical|hybrid|vector` and `--explain` shape a query. `search --eval` measures the
ranking against labeled queries and exits `2` when a `--min` floor fails. See [Skill search](/ai-rulez/search/).

## Telemetry

Opt-in usage logging, feedback and OTLP export. Nothing is sent until you consent with `ai-rulez telemetry enable`;
`DO_NOT_TRACK=1` and `AI_RULEZ_TELEMETRY=off` always win.

```bash
ai-rulez telemetry hook --harness claude     # print the hooks that record skill loads
ai-rulez telemetry report                    # used, never used and changed-since-used skills
ai-rulez telemetry feedback deploy --kind stale
ai-rulez telemetry status
```

See [Usage telemetry](/ai-rulez/usage-telemetry/) and [Item-load telemetry](/ai-rulez/telemetry/).

## MCP server

`ai-rulez mcp` starts the Model Context Protocol server, which lets an assistant read and change the configuration.
`ai-rulez mcp --serve-skills` instead serves the skills read-only (`find_skill`, `load_skill`, ...), filtered with
`--profile`, `--role`, `--domain`, `--allow` and `--deny`. See [MCP server](/ai-rulez/mcp-server/).

```json
{"mcpServers": {"ai-rulez": {"command": "npx", "args": ["-y", "ai-rulez@latest", "mcp"]}}}
```

`generate` adds this entry to `.mcp.json` unless `--no-self-mcp`.

`ai-rulez cache status` and `ai-rulez cache clear` show and drop the repository index cache that
`mcp --serve-skills --discover-skills` keeps ([The index cache](/ai-rulez/mcp-server/#the-index-cache)).

## Shell completion

`ai-rulez completion bash|zsh|fish|powershell` prints a completion script. It completes subcommands, flags, the
values of enum flags (`--format`, `--fail-on`, `--priority`, ...) and names from the project: rules, skills, domains,
profiles, roles, includes and installed skills.

```bash
source <(ai-rulez completion bash)                                # ~/.bashrc (needs bash-completion)
source <(ai-rulez completion zsh)                                 # ~/.zshrc
ai-rulez completion fish | source                                 # ~/.config/fish/config.fish
ai-rulez completion powershell | Out-String | Invoke-Expression   # PowerShell profile
```

[Installation](/ai-rulez/installation/#shell-completion) shows the same lines per shell.

## Environment Variables

ai-rulez reads `AI_RULEZ_*` variables for its own settings, plus `NO_COLOR`, `TERM`, `DO_NOT_TRACK`, `XDG_*`,
`GITHUB_TOKEN`/`GH_TOKEN`, `COSIGN_PASSWORD` and the variable a config names with `api_key_env`. A flag wins over its variable, and a
variable wins over the user config file. A repository config cannot set any of them, so the credential and network
switches are environment-only. Booleans accept `1`/`true` and `0`/`false`.

**General**

| Variable | Flag | Meaning |
| -------- | ---- | ------- |
| `AI_RULEZ_DEBUG` | `--debug` | Debug logging on stderr |
| `AI_RULEZ_QUIET` | `--quiet` | Hide progress and information lines (results, warnings, errors stay) |
| `AI_RULEZ_LAYERS` | | Comma-separated `config.<name>.toml` layers merged over `config.toml`, in order (falls back to `AI_RULEZ_PROFILE`); see `ai-rulez config layers` |
| `AI_RULEZ_PROFILE` | `--profile` | Active profile (or comma-separated composition) |
| `AI_RULEZ_ROLE` | `--role` | Active role (or comma-separated composition); also the role telemetry records |
| `AI_RULEZ_GIT_TOKEN` | `--token` / `-T` | Git access token for private includes and skill sources |
| `AI_RULEZ_GIT_TOKEN_HOSTS` | | Hosts the git token may be sent to (default `github.com`) |
| `AI_RULEZ_FORGE_HOSTS` | | Hosts the forge API client may contact (default `github.com`); `GITHUB_TOKEN` / `GH_TOKEN` supply its token |
| `AI_RULEZ_REPO_ROOT` | `--repo-root` | Repository root for validation and scans |
| `AI_RULEZ_STRICT` | `generate --strict-config` | Fail on unknown or invalid configuration keys |
| `AI_RULEZ_ACK_COMMANDS` | `generate --yes` | Silence the summary of commands a configuration will run |
| `AI_RULEZ_VERIFIERS_ALLOW_EXEC` | `verifiers run --allow-exec` | Let `command` verifiers run programs |
| `AI_RULEZ_ALLOW_FILE_URLS` | | `1` lets a `file://` git source outside the project resolve from the project config |
| `AI_RULEZ_MAX_CLONE_BYTES`, `AI_RULEZ_MAX_CLONE_FILES` | `mcp --max-clone-bytes` | Largest git skill source clone (default 256 MiB, 20000 files) |
| `AI_RULEZ_HOME` | | Extra directory `improve pr` lets its worktree commands write |
| `AI_RULEZ_TODAY` | | Pin "today" (`YYYY-MM-DD`) for baseline expiry, for reproducible runs |
| `AI_RULEZ_EVAL_DATE` | `eval run --date` | Date recorded in eval results |

**Organization policy** ([Organization policy](/ai-rulez/policy/))

| Variable | Flag |
| -------- | ---- |
| `AI_RULEZ_POLICY` | `--policy` |
| `AI_RULEZ_POLICY_DIGEST` | `--policy-digest` |
| `AI_RULEZ_POLICY_OFFLINE` | `--policy-offline` |
| `AI_RULEZ_POLICY_MAX_STALE` | `--policy-max-stale` |
| `AI_RULEZ_POLICY_REQUIRE_SIGNED` | `--policy-require-signed` |
| `AI_RULEZ_POLICY_SIGNER_KEY` | `--policy-signer-key` |
| `AI_RULEZ_POLICY_SIGNER_IDENTITY`, `AI_RULEZ_POLICY_SIGNER_ISSUER` | `--policy-signer-identity`, `--policy-signer-issuer` |
| `AI_RULEZ_POLICY_TRUSTED_ROOT` | `--policy-trusted-root` |

**LLM, review and search** ([LLM access](/ai-rulez/llm/))

`AI_RULEZ_LLM_PROVIDER`, `AI_RULEZ_LLM_MODEL`, `AI_RULEZ_LLM_BASE_URL`, `AI_RULEZ_LLM_API_KEY_ENV`,
`AI_RULEZ_LLM_EMBEDDING_MODEL`, `AI_RULEZ_LLM_MAX_COST_USD`, `AI_RULEZ_LLM_MAX_TOKENS`, `AI_RULEZ_LLM_MAX_CALLS`,
`AI_RULEZ_LLM_TIMEOUT_SECONDS`, `AI_RULEZ_LLM_CACHE`, `AI_RULEZ_LLM_ALLOW_NETWORK`, `AI_RULEZ_LLM_ALLOW_PLAIN_HTTP`,
`AI_RULEZ_LLM_PLAIN_HTTP_HOSTS`, `AI_RULEZ_LLM_PRICE_INPUT_PER_MTOK`, `AI_RULEZ_LLM_PRICE_OUTPUT_PER_MTOK`,
`AI_RULEZ_LLM_TEMPERATURE` (`omit` leaves it out of the request). Review: `AI_RULEZ_REVIEW_ALLOWED_HOSTS`. Search:
`AI_RULEZ_SEARCH_MODE` (overrides `[search] mode`), `AI_RULEZ_SEARCH_LOG_QUERIES` (turns the query log on or off).

**Telemetry** ([Item-load telemetry](/ai-rulez/telemetry/); user scope only)

`AI_RULEZ_TELEMETRY`, `AI_RULEZ_TELEMETRY_ENDPOINT`, `AI_RULEZ_TELEMETRY_PROTOCOL`, `AI_RULEZ_TELEMETRY_ALLOW_NETWORK`,
`AI_RULEZ_TELEMETRY_HEADERS_ENV`, `AI_RULEZ_TELEMETRY_SERVICE_NAME`, `AI_RULEZ_TELEMETRY_SAMPLE`,
`AI_RULEZ_TELEMETRY_INCLUDE_PATHS`, `AI_RULEZ_TELEMETRY_INCLUDE_SESSION`, `AI_RULEZ_TELEMETRY_SALT_FILE`,
`AI_RULEZ_TELEMETRY_RESOURCE`, `AI_RULEZ_USAGE_SALT` (session-hash salt; wins over `--salt-file`).

**Approvals, signing and SBOM**

| Variable | Meaning |
| -------- | ------- |
| `AI_RULEZ_REVIEWER` | Default reviewer recorded by `approve` (else git `user.email`) |
| `AI_RULEZ_SIGNING_KEY_PASSWORD` | Password of a `--key` signing key (then `COSIGN_PASSWORD`) |
| `AI_RULEZ_SBOM_REDACT_KEY` | Key for the salted hash `sbom --redact-reviewers` uses |

**Set by ai-rulez for the programs it starts** (do not set them yourself): `AI_RULEZ_RUN_TOKEN`,
`AI_RULEZ_EVAL_PROTOCOL`, `AI_RULEZ_EVAL_SKILL`, `AI_RULEZ_EVAL_MODE` (eval runner commands),
`AI_RULEZ_IMPROVE_PROTOCOL` (improve runner commands).

## CI recipe

```bash
ai-rulez generate --check --no-local           # committed outputs match the sources
ai-rulez validate --no-local --format github   # exit 2: fix the content; exit 1: fix the setup
ai-rulez lock --check                          # when you commit ai-rulez.lock
```

[GitHub Actions](/ai-rulez/github-actions/) has a ready-made workflow.

## Troubleshooting

| Symptom | Fix |
| ------- | --- |
| `no ai-rulez config found` | Run the command inside the project, run `ai-rulez init`, or name it with `-C` |
| `refusing to overwrite ... AGENTS.md` | The file is hand-written: `ai-rulez convert --write`, move it, or `generate --force` ([details](#existing-files-generate-will-not-overwrite)) |
| `unresolved MCP env placeholders` | Export the variable, add it to `.env`, or pass `--env KEY=VALUE` |
| `clean` or `remove` exits `1` in CI | Pass `--yes` |
| `pass --write to convert or --dry-run to preview` | `convert` in a script needs an explicit mode |
| `verify no longer checks generated files for drift` | Use `generate --check`; `verify` needs `--attestation`, `--approvals`, `--self` or `--plugin` |
| A config in YAML or `version = "4.0"` is refused | `ai-rulez migrate v5` |

## Command index

Every command in the binary. Flags, defaults and examples of each are in the [Command reference](/ai-rulez/cli-reference/).

<!-- BEGIN GENERATED: command-index (go run ./scripts/clidocs) -->

| Command | Summary |
| ------- | ------- |
| `ai-rulez add` | Add content to your rules |
| `ai-rulez add agent` | Add a new agent |
| `ai-rulez add check` | Add a new code-review check |
| `ai-rulez add command` | Add a new command |
| `ai-rulez add context` | Add a new context file |
| `ai-rulez add rule` | Add a new rule |
| `ai-rulez add skill` | Add a new skill |
| `ai-rulez approve` | Record that you reviewed content, bound to its digest, in ai-rulez.lock |
| `ai-rulez builtins` | Manage built-in domains |
| `ai-rulez builtins list` | List all available built-in domains |
| `ai-rulez builtins show` | Show the full content of a built-in domain |
| `ai-rulez cache` | Show or clear the repository index cache |
| `ai-rulez cache clear` | Remove the index cache of this project |
| `ai-rulez cache status` | Show where the index cache is, its size and its entry count |
| `ai-rulez catalog` | List every item with owner, version, tokens, roles and lock status, or a static site |
| `ai-rulez catalog diff` | Compare two catalogs: JSON files or git revisions |
| `ai-rulez clean` | Remove files produced by generate |
| `ai-rulez completion` | Generate the autocompletion script for the specified shell |
| `ai-rulez completion bash` | Generate the autocompletion script for bash |
| `ai-rulez completion fish` | Generate the autocompletion script for fish |
| `ai-rulez completion powershell` | Generate the autocompletion script for powershell |
| `ai-rulez completion zsh` | Generate the autocompletion script for zsh |
| `ai-rulez config` | Show how the configuration is composed (read-only) |
| `ai-rulez config layers` | List the config.<name>.toml layers and which are active |
| `ai-rulez convert` | Convert existing AI tool files into an .ai-rulez/ tree |
| `ai-rulez cost` | Report which skills, rules and context cost the most prompt tokens |
| `ai-rulez doctor` | Diagnose the project's ai-rulez setup (read-only) |
| `ai-rulez domain` | Manage domains |
| `ai-rulez domain add` | Add a new domain |
| `ai-rulez domain list` | List all domains |
| `ai-rulez domain remove` | Remove a domain |
| `ai-rulez edit` | Replace the content of a rule, context, skill, agent, command or check |
| `ai-rulez edit agent` | Edit an agent |
| `ai-rulez edit check` | Edit a code-review check |
| `ai-rulez edit command` | Edit a command |
| `ai-rulez edit context` | Edit a context file |
| `ai-rulez edit rule` | Edit a rule |
| `ai-rulez edit skill` | Edit a skill |
| `ai-rulez eval` | Run skill evals and score them |
| `ai-rulez eval calibrate-estimate` | Propose estimate assumptions measured from recorded eval runs |
| `ai-rulez eval import` | Import eval scenarios from another tool as eval cases |
| `ai-rulez eval run` | Run the eval cases of skills through a pluggable runner and score them |
| `ai-rulez export` | Export ai-rulez content to another format |
| `ai-rulez export okf` | Export rules, context, skills and more as an OKF bundle |
| `ai-rulez exposure` | Report what a Claude Code session loads at start, and inspect its artifacts and areas |
| `ai-rulez exposure areas` | Show the area map and the area of every artifact, with how it was decided |
| `ai-rulez exposure collisions` | Rank skill and agent pairs of different areas whose listings overlap |
| `ai-rulez exposure inventory` | List every artifact a session can see, with its phase, visibility and parse status |
| `ai-rulez exposure leakage` | Report area terms in the text a session sees at its start (AR9W0, AR9W1) |
| `ai-rulez generate` | Generate AI assistant rule files from configuration |
| `ai-rulez guard` | Block agent edits to generated files (PreToolUse hook) |
| `ai-rulez import` | Import content from another format into .ai-rulez/ |
| `ai-rulez import okf` | Import an OKF bundle into .ai-rulez/ |
| `ai-rulez improve` | (experimental) Improve skills with an external optimizer behind a held-out eval gate |
| `ai-rulez improve adapters` | (experimental) List the bundled optimizer adapters, or print a template |
| `ai-rulez improve apply` | (experimental) Write an accepted improve candidate into the skill (no commit) |
| `ai-rulez improve clean` | (experimental) Delete saved improve runs |
| `ai-rulez improve pr` | (experimental) Open a pull request for an accepted improve run from an isolated worktree |
| `ai-rulez improve run` | (experimental) Run an external optimizer on a copy of a skill and gate its candidate |
| `ai-rulez improve show` | (experimental) Show a saved improve run: decisions, scores, costs and the diff |
| `ai-rulez include` | Manage includes |
| `ai-rulez include add` | Add an include source |
| `ai-rulez include list` | List all includes |
| `ai-rulez include remove` | Remove an include source |
| `ai-rulez init` | Initialize a new AI rules configuration |
| `ai-rulez judge` | Review agent configuration with models: security, quality and governance, with verified evidence |
| `ai-rulez list` | List rules, context, and skills |
| `ai-rulez list agents` | List all agents |
| `ai-rulez list checks` | List all code-review checks |
| `ai-rulez list commands` | List all commands |
| `ai-rulez list context` | List all context files |
| `ai-rulez list rules` | List all rules |
| `ai-rulez list skills` | List all skills |
| `ai-rulez llm` | Inspect the [llm] model-access configuration (read-only) |
| `ai-rulez llm doctor` | Print the resolved LLM setup, optionally with one 1-token call |
| `ai-rulez llm estimate` | Estimate the tokens and cost of sending a file as a prompt (no call) |
| `ai-rulez local` | Manage the machine-local config overlay |
| `ai-rulez local init` | Create a commented config.local skeleton |
| `ai-rulez local path` | Print the local overlay file path |
| `ai-rulez local set` | Set a key in the local overlay |
| `ai-rulez local show` | Show what the local overlay overrides |
| `ai-rulez local unset` | Remove a key from the local overlay |
| `ai-rulez lock` | Pin remote includes, installed skills and authored content in ai-rulez.lock |
| `ai-rulez marketplace` | Manage installed plugin marketplaces |
| `ai-rulez marketplace add` | Add a plugin marketplace from a repository, a local directory or an https URL |
| `ai-rulez marketplace list` | List installed marketplaces |
| `ai-rulez marketplace remove` | Remove an installed marketplace |
| `ai-rulez marketplace update` | Re-pin installed marketplaces (and their plugins) in ai-rulez.lock to the current remote commit |
| `ai-rulez mcp` | Start Model Context Protocol (MCP) server |
| `ai-rulez migrate` | Migrate a 4.x configuration to the 5.0 format, or a content tree to OKF |
| `ai-rulez migrate okf` | Convert the .ai-rulez/ content tree to an OKF bundle, in place |
| `ai-rulez migrate v5` | Migrate a 4.x configuration to the 5.0 format |
| `ai-rulez okf` | Work with OKF (Open Knowledge Format) bundles |
| `ai-rulez okf validate` | Lint an OKF bundle (works on third-party bundles) |
| `ai-rulez plugin` | Manage installed plugins imported from marketplaces or repositories |
| `ai-rulez plugin install` | Install a plugin from a marketplace, a repository or a local directory |
| `ai-rulez plugin list` | List installed plugins |
| `ai-rulez plugin remove` | Remove an installed plugin |
| `ai-rulez plugin update` | Re-pin installed plugins in ai-rulez.lock to their current remote commit |
| `ai-rulez profile` | Manage profiles |
| `ai-rulez profile add` | Add a new profile |
| `ai-rulez profile list` | List all profiles |
| `ai-rulez profile remove` | Remove a profile |
| `ai-rulez profile set-default` | Set the default profile |
| `ai-rulez publish` | Package the plugin bundle into deterministic, checksummed release artifacts |
| `ai-rulez publish emit` | Write only the files of one emitter, without a release |
| `ai-rulez publish verify` | Recompute the checksums, manifest, archive and signature of a release |
| `ai-rulez remove` | Remove content from your rules |
| `ai-rulez remove agent` | Remove an agent |
| `ai-rulez remove check` | Remove a code-review check |
| `ai-rulez remove command` | Remove a command |
| `ai-rulez remove context` | Remove context |
| `ai-rulez remove rule` | Remove a rule |
| `ai-rulez remove skill` | Remove a skill |
| `ai-rulez review` | Score skills, agents, commands and rules against a rubric, offline or with an LLM judge |
| `ai-rulez review calibrate` | Measure the judge against a rubric's golden set (needed before gating) |
| `ai-rulez review explain` | Explain a review code (AR9G0-AR9G9): what it means and how to fix it |
| `ai-rulez review fix` | Propose a verified patch for the judge's findings; writes only when asked |
| `ai-rulez roles` | List, inspect and resolve [[roles]] |
| `ai-rulez roles list` | List the roles with their item counts and token estimates |
| `ai-rulez roles resolve` | List the items a role keeps, with sizes and skill modes |
| `ai-rulez roles show` | Show a role as declared and with its parent merged in |
| `ai-rulez rubric` | List, show and lint review rubrics |
| `ai-rulez rubric lint` | Check rubrics, golden files and calibration records (AR9G8) |
| `ai-rulez rubric list` | List the built-in and project rubrics |
| `ai-rulez rubric show` | Print a rubric's dimensions, weights and scoring formula |
| `ai-rulez sbom` | Print a CycloneDX or SPDX software bill of materials of the AI configuration |
| `ai-rulez scan` | Scan skills, rules and scripts for secrets, hidden text, injection and risky shell |
| `ai-rulez scanners` | Inspect the external scanners configured in [[lint.external]] |
| `ai-rulez scanners doctor` | Check named scanners: binary, version, egress, environment and configuration |
| `ai-rulez scanners list` | List the configured scanners with their egress declaration and whether they are installed |
| `ai-rulez search` | Rank the served skills against a query, build the embedding index, or evaluate the ranking |
| `ai-rulez session-start` | Regenerate outputs and brief the agent when a remote session starts (SessionStart hook) |
| `ai-rulez show` | Show the content of a rule, context, skill, agent, command or check |
| `ai-rulez show agent` | Show an agent |
| `ai-rulez show check` | Show a code-review check |
| `ai-rulez show command` | Show a command |
| `ai-rulez show context` | Show a context file |
| `ai-rulez show rule` | Show a rule |
| `ai-rulez show skill` | Show a skill |
| `ai-rulez sign` | Sign the lock, a plugin bundle, a skill or an SBOM into a Sigstore bundle |
| `ai-rulez skill` | Manage installed skills |
| `ai-rulez skill install` | Install a named skill from a git repository or local path |
| `ai-rulez skill list` | List all installed skills |
| `ai-rulez skill remove` | Remove an installed skill |
| `ai-rulez skill update` | Re-pin installed skills in ai-rulez.lock to their current remote commit |
| `ai-rulez telemetry` | Item-load telemetry: record rule, agent and context loads; optional OTLP export |
| `ai-rulez telemetry disable` | Withdraw telemetry consent |
| `ai-rulez telemetry doctor` | Show the resolved telemetry configuration, consent state and buffer |
| `ai-rulez telemetry enable` | Consent to sending telemetry to a collector (stored per user) |
| `ai-rulez telemetry export` | Write the usage log as an OTLP JSON file, or push it to the collector |
| `ai-rulez telemetry feedback` | Record that a skill misled you, is stale, wrong or great |
| `ai-rulez telemetry flush` | Send the local outbox to the OTLP collector |
| `ai-rulez telemetry hook` | Print the hooks that record skill, rule, context and agent loads |
| `ai-rulez telemetry preview` | Print exactly what an export would send, without sending anything |
| `ai-rulez telemetry prune` | Delete usage-log lines older than N days that were already exported |
| `ai-rulez telemetry record` | Record one skill or item load from a hook event on stdin |
| `ai-rulez telemetry report` | List never-used skills and skills edited since they were used |
| `ai-rulez telemetry report evals` | Rank skills to prune or rewrite from eval scores joined with usage |
| `ai-rulez telemetry status` | Show whether telemetry is on, who consented, and whether delivery works |
| `ai-rulez tokens` | Report the prompt-token cost of generated artifacts |
| `ai-rulez trust` | Manage the Sigstore trusted root used to verify keyless attestations |
| `ai-rulez trust update` | Fetch the public-good Sigstore trusted root and cache it for offline verification |
| `ai-rulez update` | Move the pins of sources that use a version constraint to the newest allowed tag |
| `ai-rulez validate` | Validate AI rules configuration and content |
| `ai-rulez verifiers` | Run the deterministic repo checks declared as [[verifiers]] |
| `ai-rulez verifiers calibrate` | Measure how reliable an llm verifier's failures are, so it may gate |
| `ai-rulez verifiers explain` | Explain what a verifier checks, the rule it enforces and how to fix it |
| `ai-rulez verifiers list` | List the declared verifiers |
| `ai-rulez verifiers run` | Evaluate the verifiers and report pass or fail for each |
| `ai-rulez verifiers suggest` | Propose verifiers for a rule with a model (a dry run that prints and writes nothing) |
| `ai-rulez verifiers test` | Run the self-test examples of the verifiers offline |
| `ai-rulez verify` | Verify attestations, approvals and plugin provenance |
| `ai-rulez version` | Print the version number of ai-rulez |

<!-- END GENERATED: command-index -->
