# Permissions

The top-level `[permissions]` block is one allow/ask/deny list for every harness that has a native
permission surface. You write Claude Code permission rules once in `.ai-rulez/config.toml`;
`ai-rulez generate` translates them into each harness's own file and format.

```toml
[permissions]
allow = ["Bash(npm run test:*)", "Read(./src/**)", "WebFetch(domain:example.com)"]
ask   = ["Bash(git push:*)"]
deny  = ["Bash(rm -rf:*)", "Read(./.env)", "mcp__untrusted__run"]
```

## Rule syntax

A rule is `Tool` or `Tool(specifier)`, as in Claude Code:

| Rule | Meaning |
| --- | --- |
| `Bash`, `Read`, `Edit`, `WebFetch` | every call of the tool (a bare allow is reported by `validate` as over-broad) |
| `Bash(npm run test:*)` | a command and any arguments; `:*` is the legacy form of `npm run test *` |
| `Bash(git status)` | exactly that command |
| `Bash(git * --force)` | a command glob |
| `Read(./.env)`, `Edit(src/**)`, `Write(dist/**)` | a path glob: `./x` or `x` is relative to the working directory, `/x` to the project root, `//x` is absolute, `~/x` is in the home directory |
| `WebFetch(domain:example.com)` | a host |
| `mcp__github__create_issue`, `mcp__github` | an MCP tool, or every tool of a server |
| `Task(Explore)` | a subagent |

The parser is shared by every translator (`internal/generator/settings`), so a rule means the same thing
everywhere. A rule that does not parse is skipped with a warning.

## What a translator may and may not do

- **A rule is never widened.** When a harness cannot express an allow exactly (it matches command
  prefixes where you wrote an exact command, one `edit` permission covers every edit tool, `?` is a
  wildcard where Claude treats it as a literal), the allow is skipped with a warning instead of being
  approximated into a broader one.
- **A deny is never lost silently.** A deny rule a harness cannot enforce is reported as
  `SECURITY: [permissions] deny rule "..." is NOT enforced by <harness>: <why>`. When a harness has no
  way to enforce any deny rule, or your preset has no documented permission file at all, `generate`
  says so as well. Enforce such rules another way (a sandbox, a hook) or leave the harness out.
- **Ask rules** are written where the harness has an ask list. Where it prompts for everything that is not
  allowed, an ask rule is not written and one warning says so; it only matters under a broader allow.
- **Your own rules survive.** Array entries and map members are owned one by one, and comments in JSONC,
  TOML and YAML documents stay. Removing a rule from `config.toml` removes exactly that entry on the next
  `generate`; `clean` takes back what `generate` wrote and leaves the rest. A setting you already have
  with another value is kept, and a warning names it, so a rule of yours is never relaxed.
- A `[[scopes]]` subdirectory renders no permissions: they are project-wide.

## Harnesses

Sources are the vendor documentation or source read on 2026-10-05. "Rules" lists what can be
expressed; everything else is skipped as described above.

| Harness | File | Format | Allow / ask / deny | Rules |
| --- | --- | --- | --- | --- |
| `claude` | `.claude/settings.json` | JSON | yes / yes / yes | all (verbatim; see `docs/settings.md`) |
| `codex` | `.codex/rules/ai-rulez.rules` | Starlark | yes / `prompt` / `forbidden` | command prefixes only |
| `opencode` | `opencode.json` | JSONC | yes / yes / yes | commands, read, edit, webfetch, websearch, task |
| `kilo` | `kilo.jsonc` | JSONC | yes / yes / yes | as `opencode` |
| `mimocode` | `.mimocode/mimocode.jsonc` | JSONC | yes / yes / yes | as `opencode` |
| `gemini` | `.gemini/settings.json` | JSON | yes / no / yes | command prefixes; whole tools |
| `cursor` | `.cursor/cli.json` | JSON | yes / no / yes | commands, read, write, webfetch, mcp |
| `copilot` | `.vscode/settings.json` | JSONC | yes / yes / prompt only | commands, edits, urls |
| `copilot-cli` | `.github/copilot/settings.json` | JSONC | yes / no / yes | URLs only |
| `zoocode` | `.vscode/settings.json` | JSONC | yes / no / yes | command prefixes |
| `devin` | `.devin/config.json` | JSONC | yes / yes / yes | commands, read, write, fetch, mcp |
| `qwen` | `.qwen/settings.json` | JSON | yes / yes / yes | Claude syntax |
| `codebuddy` | `.codebuddy/settings.json` | JSON | yes / yes / yes | Claude syntax |
| `commandcode` | `.commandcode/settings.json` | JSON | yes / yes / yes | Claude syntax (`Shell`) |
| `qoder` | `.qoder/settings.json` | JSON | yes / yes / yes | Claude syntax, no WebFetch |
| `letta` | `.letta/settings.json` | JSON | yes / `alwaysAsk` / yes | Bash, Read, Edit |
| `grok` | `.grok/config.toml` | TOML | yes / yes / yes | commands, read, edit, mcp, whole web tools |
| `vibe` | `.vibe/config.toml` | TOML | yes / no / yes | command prefixes; deny of read and edit paths; whole-tool deny |
| `poolside` | `.poolside/settings.yaml` | YAML | yes / no / yes | commands, paths |
| `omp` | `.omp/config.yml` | YAML | yes / yes / yes | commands; whole tools |
| `augment` | `.augment/settings.json` | JSONC | yes / no / yes | commands; whole tools |
| `zed` | `~/.config/zed/settings.json` (`--user` only) | JSONC | yes / yes / yes | commands, edit (deny/ask), fetch, mcp tools |
| `hermes` | `~/.hermes/config.yaml` (`--user` only) | YAML | yes / no / yes | commands |
| `kimi` | `~/.kimi-code/config.toml` (`--user` only) | TOML | yes / yes / yes | Bash, bare Read |

`--user` also writes the user-level counterpart of every harness above that has one, to the same
places as its other user-scope files: `~/.claude/settings.json`, `~/.codex/rules/ai-rulez.rules`,
`~/.gemini/settings.json`, `~/.config/opencode/opencode.json`, `~/.config/kilo/kilo.jsonc`,
`~/.config/mimocode/mimocode.jsonc`, `~/.copilot/settings.json`, `~/.qwen/settings.json`,
`~/.codebuddy/settings.json`, `~/.commandcode/settings.json`, `~/.letta/settings.json`,
`~/.grok/config.toml`, `~/.vibe/config.toml`, `~/.config/poolside/settings.yaml`,
`~/.omp/agent/config.yml`, `~/.augment/settings.json`, `~/.qoder/settings.json`,
`~/.config/devin/config.json` (the `permissions` key, next to the `hooks` key). `cursor`, `copilot` and
`zoocode` have a project file only (their user-level files are a different document, or share a path with
another generated file).

### Sources and details

- **codex**: <https://learn.chatgpt.com/docs/agent-configuration/rules> (marked experimental by Codex),
  <https://learn.chatgpt.com/docs/config-file/config-reference>. `prefix_rule(pattern=[argv tokens],
  decision=...)`; the most restrictive match wins. Paths and domains live in permission profiles
  (`default_permissions`), which cannot be combined with `sandbox_mode`, so read, edit, fetch and MCP rules
  are not generated. The rules file is owned by ai-rulez; put your own rules in another `*.rules` file.
- **opencode, kilo, mimocode**: <https://opencode.ai/docs/permissions/>,
  <https://kilo.ai/docs/code-with-ai/platforms/cli>, <https://mimo.xiaomi.com/mimocode/permissions>. The
  last matching rule wins and the merge writes a map in key order, so a rule that would be evaluated
  after an overlapping, stricter one (yours or ours) is skipped rather than allowed to relax it. `Bash`
  prefixes become `cmd *`; a `WebFetch` domain becomes `https://host/*` (deny also covers `http` and the
  bare host). MCP tool names are not a documented permission key, so MCP rules are skipped.
- **gemini**: <https://geminicli.com/docs/reference/configuration/>,
  <https://geminicli.com/docs/reference/policy-engine/>. Project policy files
  (`.gemini/policies/*.toml`) are documented as non-functional, so `tools.allowed` and `tools.exclude` of
  `settings.json` are used. They match command prefixes and whole tools, never a path or a domain.
- **cursor**: <https://cursor.com/docs/cli/reference/permissions>. `Shell(git)` covers every `git`
  command, so only single-word prefixes can be allowed; a multi-word deny uses `Shell(git:push*)`.
  `ask` has no equivalent and prompts for whatever is not allowed. `permissions.allow` and
  `permissions.deny` are both required fields of `cli.json`
  (<https://cursor.com/docs/cli/reference/configuration>, checked 2026-10-06), so a deny-only config still
  writes `"allow": []`. ai-rulez owns an empty array it created and `clean` removes it with the file; an
  `allow: []` you wrote stays.
- **copilot**: <https://code.visualstudio.com/docs/agents/run/approvals>,
  <https://code.visualstudio.com/docs/chat/review-code-edits>. `true` auto-approves and `false` always
  asks. VS Code has no hard deny, so a deny rule is written as `false` and reported as not enforced. Read and MCP
  rules have no setting. `copilot` and `zoocode` share `.vscode/settings.json`; the document holds the keys
  of whichever of the two presets is configured.
- **zoocode**: Zoo-Code `src/package.json` and `src/core/auto-approval/commands.ts`. The longest
  matching prefix wins and a tie goes to deny, so an allow that is longer than a deny prefix of it is
  skipped. Command auto-approval also needs the extension's auto-approve toggle, which is not a settings
  file value.
- **copilot-cli**: <https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference>.
  Tool permissions are session flags; only `allowedUrls` and `deniedUrls` can be committed.
- **devin**: <https://docs.devin.ai/cli/reference/permissions>.
- **qwen, codebuddy, commandcode, qoder, letta**: <https://qwenlm.github.io/qwen-code-docs/en/users/configuration/settings/>
  (the dedicated permissions page was not reachable, so its details beyond the documented
  `permissions.{allow,ask,deny}` shape are unverified), <https://www.codebuddy.ai/docs/cli/permissions>,
  <https://commandcode.ai/docs/permissions>, <https://docs.qoder.com/cli/permissions>,
  <https://docs.letta.com/letta-code/permissions>. Command Code calls the shell tool `Shell`, and Command
  Code and Qoder match a bare path at any depth, so paths are written project-rooted (`/src/**`). Qoder's
  Edit rule covers Write, so a Write allow is skipped. Letta's `ask` list does not override a broader
  allow, so ask rules go to `alwaysAsk`, which is documented in its source only.
- **grok**: <https://docs.x.ai/build/settings/reference>, <https://docs.x.ai/build/features/permissions>.
  Project config may hold `[permission]`. The WebFetch domain syntax is not documented, so only a bare
  `WebFetch` rule is written.
- **vibe**: github.com/mistralai/mistral-vibe (`vibe/core/tools`). Setting `allowlist` or `denylist` replaces
  Vibe's shipped default for that list. Path deny rules are written as `*/<glob>` because Vibe matches the
  absolute path; an allow cannot be scoped, so it is skipped.
- **poolside**: <https://docs.poolside.ai/tool-permissions>, <https://docs.poolside.ai/settings-file-reference>.
  The shared project file takes project-relative paths only.
- **omp**: github.com/can1357/oh-my-pi `docs/settings.md`, `docs/approval-mode.md`. `bash.patterns` is
  first-match-wins; ai-rulez appends deny, ask, then allow after your own patterns, so a pattern of yours that
  matches first still wins. There is no per-path or per-domain matcher.
- **augment**: <https://docs.augmentcode.com/cli/permissions>. Only allow and deny exist; a command rule is
  a `shellInputRegex`; no path or domain matcher.
- **zed**: <https://zed.dev/docs/ai/tool-permissions>. `agent.tool_permissions` is read from the user
  settings, so it is written by `--user` only. A project run prints one warning. Read tools are not
  permission-gated in Zed, so read rules are skipped (use `private_files`).
- **hermes, kimi**: <https://hermes-agent.nousresearch.com/docs/user-guide/security>,
  <https://moonshotai.github.io/kimi-code/en/configuration/config-files.html>. User-level files only.

## Not translated

These harnesses have no documented, committable permission file that can express the rules, so they get
nothing from `[permissions]` and `generate` names them in a warning:

| Harness | Why |
| --- | --- |
| `amp` | Current Amp docs removed `amp.permissions` ("Amp does not ask for approval ... use a plugin"); only whole-tool `amp.tools.disable` remains |
| `pi` | Pi has no permission mechanism (it relies on isolation); `defaultTools` only enables or disables whole tools |
| `cline` | Cline documents command permissions only as the `CLINE_COMMAND_PERMISSIONS` environment variable, not a file |
| `kiro` | The permissions model in `.kiro/agents/*.json` is documented with conflicting shapes, and the workspace permissions file lives outside the repository |
| `factory` | The project settings path is documented as `settings.local.json`, and `commandDenylist` means "ask", not "deny" |
| `crush` | `crush.json` is deprecated for `crushrc`; its schema has `allowed_tools` (whole tools) and no deny |
| `rovodev` | The location of the bash rules differs between Atlassian's pages and their match order is undocumented |
| `takt` | Only a coarse permission mode (`readonly`, `edit`, `full`), no rules |
| `antigravity` | Only the user-level CLI file is documented; the project path is not |
| `goose`, `junie`, `deepagents`, `warp` | User-level files with whole-tool, allow-only, exact-command or regex semantics that cannot hold the rules without widening them |

## Matching semantics that change a rule

These are the places where a harness matches differently from Claude Code, and what ai-rulez does about it.

- **Whole-command regular expressions (`augment`, `zed`).** The harness tests one regex against the command
  string, so `^git log(\s|$)` would also match `git log; rm -rf /`. An allow is written as
  `^git log([ \t][^;&|\n\r`$()<>]*)?$`: the command, then arguments free of separators, substitution,
  grouping and redirections (a command using them is not auto-approved, which is the safe direction). A deny
  is written broader: Augment's matches the command after a separator, a subshell or a backtick
  (`(^|[\s;&|(`])git push(...)`), and Zed's stays anchored because Zed tests every chained sub-command and
  the raw command. A deny cannot see through a wrapper (`env git push`, `sh -c 'git push'`, a variable): enforce
  important denies with a sandbox as well. Zed allows are written `case_sensitive`.
- **Bare prefixes (`zoocode`, `gemini`, `copilot`).** Zoo Code and Gemini match with a plain `startsWith`, so
  `git` would approve `gitk`: allow prefixes are written with a trailing space (`git `). VS Code documents
  string keys without a word boundary, so an allow is written as the anchored regex `/^git status(\s|$)/`.
  Denies stay plain prefixes: a broader deny only blocks more. Vibe was verified to match
  `command == prefix or command.startswith(prefix + " ")` and needs no change.
- **Paths (`opencode`, `kilo`, `mimocode`).** Patterns are matched against the path relative to the worktree
  (OpenCode's read tool passes `path.relative(worktree, file)`), and `*` matches `/` too. An allow with a
  single `*` (`Read(./src/*.go)`) or `?` would widen, and a home or absolute path allow would never apply, so
  these are skipped with a warning. A deny is written for the root and for any depth (`.env` and `**/.env`);
  a home or absolute deny is written as given but reported as `SECURITY: ... may not be enforced`, since the
  harness matches relative paths.
- **Domains.** `WebFetch(domain:Host.COM)` is lowercased (hostnames are case-insensitive, most patterns are not).
  OpenCode-family denies also cover `https://host:*` and `http://host:*`, because `https://host/*` does not
  match a URL with a port.

## Lossy cases at a glance

- A command glob (`Bash(git * --force)`) is skipped wherever the harness matches prefixes only (`codex`,
  `gemini`, `cursor`, `devin`, `vibe`, `zoocode`).
- A path or domain rule is skipped wherever the harness approves whole tools only (`gemini`, `omp`,
  `augment`, `vibe` allow, `codex`).
- Skipped allow rules mean a prompt where you wanted none; skipped deny rules are reported as above.

Run `ai-rulez generate --dry-run` to see the warnings without writing files.
