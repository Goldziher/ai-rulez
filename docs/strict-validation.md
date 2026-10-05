# Strict validation

`ai-rulez validate` checks that configuration parses and matches the schema. `ai-rulez validate --strict` goes
further and checks that the instruction content actually *works*: globs that select nothing, links and
references that point nowhere, hooks that cannot run. Each problem is a finding with a stable code, a severity
and a `file:line`.

```bash
ai-rulez validate --strict                       # text report, exit 2 on errors
ai-rulez validate --strict --format json         # machine-readable
ai-rulez validate --strict --recursive           # every nested root
ai-rulez validate --strict --fail-on warning     # warnings also fail
```

Strict mode runs after the normal validation passes, so a config that fails schema or structural validation
still exits 1 and never reaches the content checks.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Configuration valid and no finding at or above the fail threshold |
| 1 | Configuration invalid or could not be loaded (unchanged), or an invalid `[lint]` setting |
| 2 | Strict findings at or above the threshold (`--fail-on`, else `[lint] fail_on`, else `error`) |

`--fail-on` accepts `error`, `warning`, `info` or `none` (report, never fail). `--format` and `--fail-on` require
`--strict`. With `--format json` nothing but the JSON document is written to stdout.

## What is checked against what

- **Tracked files.** Globs, paths and hook files are resolved against `git ls-files` (the index), so a file that
  exists only in your working tree does not satisfy a glob. Outside a git repository the directory is walked
  instead.
- **Nested roots.** A root's globs and paths resolve from the repository top or from the root's own directory,
  whichever matches. A root may name skills, agents, commands and rules defined by an ancestor root or in a
  hand-authored `.claude/` directory of an ancestor, because assistants load the ancestors' instructions.
  `--recursive` lints every root and merges the findings.
- **Only files you own.** Content pulled in from builtins or includes is used to resolve names but is never
  reported on, except by the security checks when `[lint.security] scan_imports` is set.
- **Prose, not code.** Fenced code blocks and inline code spans are ignored for links; backticked tokens are only
  read as paths or slash commands.

## Findings

| Code | Name | Default | Finds |
| --- | --- | --- | --- |
| AR001 | `secret-detected` | error | A credential pattern (AWS, GitHub, Slack, Google, Stripe, Anthropic/OpenAI keys, private keys, JWTs, `password = "..."` with a mixed-character value, or a `secret_patterns` entry) in content or a script. The finding masks the match |
| AR002 | `hidden-characters` | error | Zero-width, bidirectional-control or Unicode tag characters (a joiner between two non-ASCII characters, as in emoji, and a leading byte order mark are fine) |
| AR003 | `html-comment-instruction` | warning | An HTML comment, invisible when rendered, with instruction-like text (`curl`, `eval`, `run:`, "secretly", injection phrases) |
| AR004 | `prompt-injection-phrase` | warning | Text that tries to override earlier instructions or hide actions from the user ("ignore previous instructions", "do not tell the user", ...) |
| AR005 | `risky-shell-exec` | error | `curl ... \| sh` (or an interpreter), `bash <(curl ...)`, `eval` of dynamic text, a base64 payload decoded into a shell. `eval "$(ssh-agent -s)"` and similar environment initializers are exempt |
| AR006 | `risky-shell-access` | warning | Reads of `~/.ssh`, `~/.aws`, `.netrc` and other credential locations, writes to `~/`, `/etc`, `/usr`..., `chmod 777` |
| AR007 | `tool-breadth` | warning | A skill or command `allowed-tools` entry that is unrestricted: `Bash`, `Bash(*)`, `*` (listed exceptions in `allowed_tools` pass) |
| AR008 | `outbound-host` | warning | A URL whose host is not in `[lint.security] allowed_hosts`; checked only when that list is set (`localhost` and `127.0.0.1` always pass) |
| AR009 | `encoded-blob` | warning | A base64-like run of 200 or more characters that a reviewer cannot read |
| AR010 | `unpinned-remote` | warning | A remote include or installed skill follows a moving ref and `ai-rulez.lock` does not pin it (a full commit SHA counts as pinned) |
| AR011 | `external-finding` | warning | A finding from a `[[lint.external]]` scanner (its own severity is kept) |
| AR101 | `glob-no-match` | error | A `paths`/`globs` pattern in a rule or context file matches no file tracked by git |
| AR201 | `link-unresolved` | error | A relative markdown link (or image, or reference definition) points at a file that does not exist |
| AR202 | `anchor-unresolved` | warning | `file.md#anchor` where the target has no heading producing that anchor |
| AR301 | `reference-unknown` | error | Prose names a skill, agent, rule or command that does not exist (`` `x-y` skill ``, `skill `x``, `/x-y`, `Skill(x)`, `subagent_type: x`) |
| AR302 | `frontmatter-skill-unknown` | error | Frontmatter `skills:` lists a skill that does not exist |
| AR303 | `frontmatter-key-unknown` | warning | A top-level frontmatter key no tool reads (`allowed_tools` for `allowed-tools`). Known keys are the Agent Skills specification, the Claude Code skill and subagent references and the keys ai-rulez reads; extend with `allowed_keys` |
| AR401 | `path-missing` | warning | A backticked repo path (first segment is a top-level entry of the repo) does not exist |
| AR402 | `skill-resource-missing` | error | A `references/`, `scripts/` or `assets/` path exists neither in the skill or command nor in the repo |
| AR501 | `hook-missing` | error | A `.claude/settings.json` hook command runs a `$CLAUDE_PROJECT_DIR/...` file that does not exist |
| AR502 | `hook-not-executable` | error | That hook file is executed directly but lacks the executable bit |
| AR503 | `script-not-executable` | warning | A skill `scripts/` file with a shebang lacks the executable bit |
| AR504 | `hook-source-missing` | error | A `script` of a top-level `[[hooks]]` entry in `config.toml` does not exist |
| AR505 | `hook-source-not-executable` | error | That `[[hooks]]` script lacks the executable bit |
| AR506 | `permission-overbroad` | warning | A `[permissions] allow` rule permits every call of a tool (`Bash`, `Bash(*)`, `*`) |
| AR601 | `mcp-command-not-found` | warning | A stdio `[[mcp_servers]]` `command` is not on `PATH` (or not an existing relative file) |
| AR701 | `description-duplicate` | warning | Two skills, agents or commands have identical descriptions |
| AR702 | `description-near-duplicate` | warning | Descriptions overlap at or above `near_duplicate_threshold` (word-set Jaccard) |
| AR703 | `duplicate-collapsed` | warning | Two sources define the same rule, context, skill or command name and generation silently keeps one (root over domains over includes over builtins). Message names both paths; allow intentional shadowing with `allow_overrides` |
| AR801 | `description-missing` | warning | A skill, agent or command has no `description` |
| AR802 | `description-length` | warning | Description shorter than `min_length` (default 20) or longer than `max_length` (default 1024, the Agent Skills limit) |
| AR803 | `description-style` | off | Description does not say when to use the item; turned on by `require_use_when = true` |
| AR804 | `skill-name-invalid` | warning | Skill `name` is not lowercase letters/digits/single hyphens, exceeds 64 characters, or differs from its directory (Agent Skills specification) |
| AR901 | `size-lines` | warning | Item exceeds its line budget |
| AR902 | `size-tokens` | warning | Item exceeds its token budget (cl100k_base, an approximation) |
| AR951 | `metadata-missing` | error | Item lacks a frontmatter key listed in `require_metadata`, or a `[lint.metadata.<key>] required = true` key (no key is required by default). A key inside the Agent Skills `metadata` map counts |
| AR952 | `metadata-invalid` | error | A `[lint.metadata.<key>]` value is not a date, is not one of the `values` of an enum, or is a date in the future |
| AR953 | `metadata-stale` | warning | A date older than `max_age_days` |
| AR954 | `superseded-by-missing` | error | `superseded_by: <name>` names an item that does not exist |
| AR961 | `plugin-version-drift` | warning | A generated plugin's content changed since `HEAD` but its manifest `version` did not, so clients that cache the plugin keep the old copy (only for configs with `[plugin]` or `[marketplace]`; needs a git repository) |
| AR962 | `evals-missing` | off | A skill has no eval cases; turned on by `[lint.evals] require = true` or a `[lint.severity]` entry (see [Evals](evals.md)) |

Codes are stable: they are never renumbered or reused. Both the code and the name are accepted everywhere a code
is configured.

## Configuration

Everything is optional. Put it in `config.toml` (or the YAML/JSON equivalent); a `config.local.*` overlay may
override individual `[lint]` keys.

```toml
[lint]
fail_on = "error"                  # error (default) | warning | none
ignore = ["AR803"]                 # codes or names dropped everywhere
ignore_paths = ["domains/legacy/**"]   # source files (relative to .ai-rulez/ or the repo) to skip
allow_paths = [".claude/**", "bazel-*/**"]   # repo paths that may be referenced without existing (AR401/AR402)
known_names = ["superpowers-brainstorm"]     # skills/agents/rules/commands provided outside this tree
allow_overrides = ["legacy-helper", "backend/deploy"]   # intentional shadowing (AR703): "name" or "domain/name"
allowed_keys = ["team"]                                 # extra frontmatter keys (AR303)

[lint.severity]                    # error | warning | info | off
AR401 = "error"
anchor-unresolved = "off"

[lint.description]
min_length = 30
max_length = 1024
require_use_when = true            # enables AR803 at warning
near_duplicate_threshold = 0.9

[lint.budgets.skill]               # kinds: rule, context, skill, agent, command
max_lines = 400
max_tokens = 4000                  # 0 keeps the default, a negative value removes the limit

[lint.require_metadata]
skill = ["owner"]
rule = ["owner"]

[lint.metadata.last_verified]      # typed metadata, top-level or inside the `metadata` map
type = "date"                      # string (default) | date | enum
max_age_days = 90                  # AR953 when older; a bad date is AR952
required = true                    # AR951 when absent
kinds = ["skill"]                  # default: every kind

[lint.metadata.tier]
type = "enum"
values = ["gold", "silver"]

[lint.security]
scan_imports = "error"             # off (default) | warn | error, see below
allowed_hosts = ["github.com", "*.example.org"]   # enables AR008
allowed_tools = ["Bash"]           # unrestricted allowed-tools entries that are accepted (AR007)
injection_phrases = ["as root user"]
secret_patterns = [{ name = "internal token", regex = "corp_[a-z0-9]{10}" }]

[[lint.external]]                  # run with --external only
name = "my-scanner"
command = ["my-scanner", "--sarif"]
format = "sarif"                   # sarif (default) | json

[lint.evals]
require = true                     # enables AR962 at warning
allow = ["scratch-*"]              # skills exempt from the check
```

Default budgets (lines / tokens): rule 200 / 2500, context 300 / 3000, skill 500 / 5000, agent 300 / 3000,
command 300 / 3000. The skill figures follow the Agent Skills recommendation (`SKILL.md` under 500 lines and
5000 tokens). Token counts use the embedded `cl100k_base` tokenizer and are approximate.

An unknown code, severity or content kind in `[lint]` is an error (exit 1) rather than a silently disabled check.

### Suppressing one finding

Add an `ai-rulez-lint-ignore` comment on the line before, or on, the offending line. Without codes it silences
every finding there:

```markdown
<!-- ai-rulez-lint-ignore: AR401, AR301 -->
See `legacy/old-tool/README.md` and the `retired-helper` skill.
```

## Security checks

`AR001` to `AR011` are deterministic and offline: they read text and report patterns, they never fetch or run
anything. `ai-rulez scan` runs only this family (same `--format`, `--fail-on`, `--recursive`, exit codes);
`validate --strict` runs it together with everything else.

They cover `SKILL.md`, rules, context, agents, commands, markdown references and the text files a skill ships
(scripts, data). Disable a rule everywhere with `ignore`, one file with `ignore_paths`, one line with
`ai-rulez-lint-ignore`. The patterns are heuristics tuned for precision, and a finding in prose that *documents*
a risky command needs an inline ignore.

**Imported content.** Owned files are checked by default. With `[lint.security] scan_imports`, content from
`includes` and `installed_skills` is scanned too: `validate --strict` reports it, and `generate` scans it *before
writing anything* and stops at level `error` (`warn` only logs). The level replaces the severity of findings in
imported text, and an `ai-rulez-lint-ignore` comment inside imported text is not honored, so an import cannot
silence the check on its own content. Pair it with `ai-rulez lock` so the scanned bytes are the pinned bytes.

**External scanners.** `[[lint.external]]` plugs in a classifier or a third-party scanner. The command is run from
the project root with the scanned file paths appended, and prints SARIF 2.1.0 (`runs[].results[]`) or a JSON list
of `{file, line, severity, rule, message}` to stdout; a non-zero exit is fine when the output parses. Findings are
merged as `AR011` (with the scanner's severity) into the text and `--format json` reports. A scanner that fails or
prints nothing parseable is itself reported. Because the command comes from the repository, it runs only when you
pass `--external`.

## JSON output

```json
{
  "roots": ["."],
  "findings": [
    {
      "code": "AR101", "name": "glob-no-match", "severity": "error",
      "file": ".ai-rulez/rules/scoped.md", "line": 4,
      "message": "glob \"nothing/**\" matches no tracked file, so this rule never applies",
      "root": "."
    }
  ],
  "summary": { "total": 1, "errors": 1, "warnings": 0, "infos": 0, "by_code": { "AR101": 1 } }
}
```

`file` is relative to the working directory when inside it. Findings are sorted by file, line and code.

## Relation to other checks

Strict mode does not repeat what `validate` already enforces (malformed frontmatter, duplicate output ids,
skill/command namespace collisions, plugin hook `script` existence, unresolved includes). `generate --check` and
`verify --plugin` cover drift of generated output; strict mode covers the health of the *sources*.

## Limits

- Name references are matched by pattern, so prose such as ``the skill `argument-hint` `` reports AR301; use
  `known_names` or an inline ignore for intentional cases.
- Hook checks read `.claude/settings.json` only, and only commands that start with `$CLAUDE_PROJECT_DIR` or
  `${CLAUDE_PROJECT_DIR}`.
- Anchors use GitHub-style heading slugs.
- The MCP command check depends on the `PATH` of the machine running the command.

## Additional rules (content, config and security)

<!-- lint-rules:begin -->
The rules below were added after the first release of strict validation. Each has a stable code, is deterministic
and offline, honors `[lint.severity]`, `[lint] ignore` and the inline `ai-rulez-lint-ignore` comment, and is listed
by `ai-rulez validate --strict --format json` under its name. "Default" is the severity when `[lint.severity]`
does not override it; a rule that reports mild and serious cases at different levels says so.

| Code | Name | Default | Finds |
| --- | --- | --- | --- |
| AR304 | `frontmatter-value-invalid` | warning | A frontmatter value the Claude Code skill or subagent reference does not accept: `effort` (`low`, `medium`, `high`, `xhigh`, `max`), `context` (`fork`), `shell` (`bash`, `powershell`), `permissionMode`, `memory`, `isolation`, `color`, a quoted or `yes`-style value where a YAML boolean is required, a non-integer `maxTurns`, a `model` that is neither an alias (`sonnet`, `opus`, `haiku`, `inherit`), a full model ID nor a known vendor ID, and a `paths`/`globs` value that is not a glob string or a list of glob strings. The message suggests the nearest valid value |
| AR305 | `tool-name-unknown` | warning | An `allowed-tools`, `disallowed-tools`, `tools` or `disallowedTools` entry that names no Claude Code tool (with a did-you-mean), a malformed `mcp__server__tool` name, unbalanced parentheses in a `Bash(...)` pattern, or a tool listed as both allowed and denied. MCP tools, `Bash(...)`/`WebFetch(...)` patterns and `Agent(name)` are valid. Tools provided elsewhere go in `lint.known_names`. Not checked when `claude` is not among the presets |
<!-- lint-rules:end -->
