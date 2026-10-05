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
- **Repository root.** The root defaults to the git toplevel of the configuration, else the directory that holds
  it. For a configuration checked out away from its repository (a scratch copy, a CI artifact) pass
  `--repo-root <dir>` or set `AI_RULEZ_REPO_ROOT`: paths, globs (`git ls-files`) and hook files then resolve
  against that directory instead of reporting paths that exist in the real repository as missing. `AR402`
  names both bases it tried: the skill directory and the repo root.
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
| AR010 | `unpinned-remote` | warning | A remote include, installed skill or `[[skill_sources]]` entry follows a moving ref and `ai-rulez.lock` does not pin it (a full commit SHA counts as pinned) |
| AR011 | `external-finding` | warning | A finding from a `[[lint.external]]` scanner (its own severity is kept) |
| AR101 | `glob-no-match` | error | A `paths`/`globs` pattern in a rule or context file matches no file tracked by git |
| AR201 | `link-unresolved` | error | A relative markdown link (or image, or reference definition) points at a file that does not exist |
| AR202 | `anchor-unresolved` | warning | `file.md#anchor` where the target has no heading producing that anchor |
| AR301 | `reference-unknown` | error | Prose names a skill, agent, rule or command that does not exist (`` `x-y` skill ``, `skill `x``, `/x-y`, `Skill(x)`, `subagent_type: x`). The kind word is loose: a name that exists as any other kind (rule, skill, agent, command, context) is not reported |
| AR302 | `frontmatter-skill-unknown` | error | Frontmatter `skills:` lists a skill that does not exist |
| AR303 | `frontmatter-key-unknown` | warning | A top-level frontmatter key no tool reads (`allowed_tools` for `allowed-tools`). Known keys are the Agent Skills specification, the Claude Code skill and subagent references and the keys ai-rulez reads; extend with `allowed_keys` |
| AR401 | `path-missing` | warning | A backticked repo path (first segment is a top-level entry of the repo) does not exist |
| AR402 | `skill-resource-missing` | error | A `references/`, `scripts/` or `assets/` path is found neither relative to the skill or command directory nor relative to the repo root (see `--repo-root`) |
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
| AR996 | `eval-case-invalid` | error | An eval case file (`*.eval.yaml`, `*.eval.yml`, `*.eval.json`) is malformed: unknown field, missing `expect_trigger` or prompt, bad assertion, invalid regex, unsafe path, duplicate id (see [Evals](evals.md#case-format)) |
| AR997 | `eval-stale` | off | A skill changed after its last recorded passing eval run; turned on by `[lint.evals] require_fresh = "warn"\|"error"` |
| AR998 | `eval-score-low` | off | A skill's recorded eval pass rate is below `[lint.evals] min_pass_rate`; setting that turns the rule on at error |
| AR9A0 | `eval-results-invalid` | error | `.ai-rulez/eval-results.json` cannot be parsed or has an unsupported `schema_version` |
| AR990 | `served-skill-referenced-statically` | warning | A static rule, context or skill names (`` `x` skill ``, `` `x` ``, `/x`, `Skill(x)`) a skill whose `delivery` is `served`; the harness cannot see it until the agent calls `find_skill`. `both` skills are static and are not reported (see [Dynamic skill loading](mcp-server.md#dynamic-skill-loading)) |
| AR991 | `delivery-stub-missing` | error | Skills are served but a configured harness that can call MCP has no `dynamic-skills` stub in its output (a skill of that name shadows it, or the preset renders no skills), so its agent is never told to call `find_skill` |
| AR992 | `delivery-static-fallback` | warning | A configured harness without MCP support keeps served skills as static files (nothing is dropped) |
| AR993 | `served-no-server` | warning | Skills are served but no `[[mcp_servers]]` entry runs `ai-rulez mcp --serve-skills` |
| AR994 | `delivery-invalid` | error | A skill's `delivery` frontmatter is not `static`, `served` or `both` (it is ignored and the skill keeps its inherited delivery) |
| AR995 | `served-lock-mismatch` | error | `[lock] enforce = true` and a served skill is not pinned in `ai-rulez.lock` or its digest differs; the server refuses to serve it |
| AR989 | `served-file-unscannable` | warning / error | A served skill file is binary (a NUL byte, invalid UTF-8) or larger than 512 KiB, so the security scan cannot read it. For a remote source (`trust = "error"`) the file is not served; a `SKILL.md` that cannot be scanned is an error and the skill is refused |
| AR971 | `role-reference-unknown` | error | A `[[roles]]` entry lists a domain that does not exist, or an include, exclude or `skill_mode` entry that matches no item (or matches only in a domain the role does not select). See [Roles](roles.md) |
| AR972 | `role-extends-invalid` | error | A role extends an unknown role, itself, or takes part in a cycle, or extends a role that itself extends another (inheritance is one level deep) |
| AR973 | `role-unreachable-dependency` | warning | An item a role keeps lists a skill in its `skills:` frontmatter that the role drops, or hides from the model with `skill_mode` `off` or `user-invocable-only` |
| AR981 | `lock-source-drift` | error | An authored item was added, removed or changed since `ai-rulez.lock` was written. Raised only when a lock exists and `[lock] enforce = true` (see [Lock file](lockfile.md)) |
| AR982 | `lock-output-drift` | error | A generated output differs from the digest in `ai-rulez.lock`. Same conditions as AR981 |
| AR9C0 | `llm-config-invalid` | error | The `[llm]` table is invalid: an unknown `backend`, a literal secret (`api_key = ...`, or a key where `api_key_env` wants a variable name), credentials or a query string in `base_url`, or a negative limit (see [LLM access](llm.md)) |

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
require_fresh = "warn"             # AR997: warn | error | off (default)
min_pass_rate = 0.8                # AR998: recorded pass rate floor; also the pass mark of `eval run`
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

## OKF bundle checks

A project that turns on the [`okf` preset](okf.md) (or sets `[okf] dir`) also has its Open Knowledge Format bundle
linted by `validate --strict` and `doctor`. The same checks run on any third-party bundle with
`ai-rulez okf validate <dir>`. Severities below are defaults; the spec says consumers must tolerate most of these
(broken links, a missing or partial index), so only conformance failures and export drift are errors.

| Code | Name | Default | Finds |
| --- | --- | --- | --- |
| AR9B0 | `okf-index-mismatch` | warning | An `index.md` entry points at a missing file, or a directory with an `index.md` has a concept or subdirectory it does not list |
| AR9B1 | `okf-type-invalid` | error | Unparseable frontmatter, or a concept with no non-empty `type` (OKF conformance rules 1 and 2) |
| AR9B2 | `okf-link-broken` | warning | A markdown link in a concept does not resolve to a file in the bundle |
| AR9B3 | `okf-version-invalid` | warning | The root `okf_version` is not `MAJOR.MINOR` (info when well formed but not `0.2`) |
| AR9B4 | `okf-orphan` | info | A concept no index entry and no link reaches (only when the bundle has an index) |
| AR9B5 | `okf-export-drift` | error | The bundle differs from what the `okf` preset would write now (project lint only) |
| AR9B6 | `okf-reserved-structure` | error | Frontmatter in a nested `index.md`, keys other than `okf_version` in the root one; a `log.md` heading that is not an ISO date is a warning |
| AR9B7 | `okf-title-duplicate` | info | Two concepts in one directory share a title |
| AR9B8 | `okf-path-unsafe` | error | A symlink, or paths differing only in case |
| AR9B9 | `okf-lossy-mapping` | info | Reserved for import notes: `x-ai-rulez` data that could not be mapped |

Severities are configured like any other code (`[lint.severity]`, `[lint.ignore]`). See [OKF](okf.md).

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
