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
  reported on.
- **Prose, not code.** Fenced code blocks and inline code spans are ignored for links; backticked tokens are only
  read as paths or slash commands.

## Findings

| Code | Name | Default | Finds |
| --- | --- | --- | --- |
| AR101 | `glob-no-match` | error | A `paths`/`globs` pattern in a rule or context file matches no file tracked by git |
| AR201 | `link-unresolved` | error | A relative markdown link (or image, or reference definition) points at a file that does not exist |
| AR202 | `anchor-unresolved` | warning | `file.md#anchor` where the target has no heading producing that anchor |
| AR301 | `reference-unknown` | error | Prose names a skill, agent, rule or command that does not exist (`` `x-y` skill ``, `skill `x``, `/x-y`, `Skill(x)`, `subagent_type: x`) |
| AR302 | `frontmatter-skill-unknown` | error | Frontmatter `skills:` lists a skill that does not exist |
| AR401 | `path-missing` | warning | A backticked repo path (first segment is a top-level entry of the repo) does not exist |
| AR402 | `skill-resource-missing` | error | A `references/`, `scripts/` or `assets/` path exists neither in the skill or command nor in the repo |
| AR501 | `hook-missing` | error | A `.claude/settings.json` hook command runs a `$CLAUDE_PROJECT_DIR/...` file that does not exist |
| AR502 | `hook-not-executable` | error | That hook file is executed directly but lacks the executable bit |
| AR503 | `script-not-executable` | warning | A skill `scripts/` file with a shebang lacks the executable bit |
| AR601 | `mcp-command-not-found` | warning | A stdio `[[mcp_servers]]` `command` is not on `PATH` (or not an existing relative file) |
| AR701 | `description-duplicate` | warning | Two skills, agents or commands have identical descriptions |
| AR702 | `description-near-duplicate` | warning | Descriptions overlap at or above `near_duplicate_threshold` (word-set Jaccard) |
| AR801 | `description-missing` | warning | A skill, agent or command has no `description` |
| AR802 | `description-length` | warning | Description shorter than `min_length` (default 20) or longer than `max_length` (default 1024, the Agent Skills limit) |
| AR803 | `description-style` | off | Description does not say when to use the item; turned on by `require_use_when = true` |
| AR804 | `skill-name-invalid` | warning | Skill `name` is not lowercase letters/digits/single hyphens, exceeds 64 characters, or differs from its directory (Agent Skills specification) |
| AR901 | `size-lines` | warning | Item exceeds its line budget |
| AR902 | `size-tokens` | warning | Item exceeds its token budget (cl100k_base, an approximation) |
| AR951 | `metadata-missing` | error | Item lacks a frontmatter key listed in `require_metadata` (no key is required by default) |
| AR961 | `plugin-version-drift` | warning | A generated plugin's content changed since `HEAD` but its manifest `version` did not, so clients that cache the plugin keep the old copy (only for configs with `[plugin]` or `[marketplace]`; needs a git repository) |

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
