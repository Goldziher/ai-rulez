# MCP server

`ai-rulez mcp` is a Model Context Protocol server that lets your coding assistant read, edit, validate and generate
your `.ai-rulez/` configuration, and serve skills and nested repository context on demand instead of loading them into
every session. Any MCP client can use it: Claude Code, Cursor, Codex, VS Code with Copilot, Gemini CLI, opencode and
others.

You do not start the server yourself. `ai-rulez generate` registers it in the project MCP file of each configured
harness, and the assistant launches it over stdio when a session starts:

```toml
# .ai-rulez/config.toml
version = "5.0"
name = "web"
presets = ["claude", "cursor", "codex"]
```

```bash
ai-rulez generate
```

`.mcp.json` (Claude Code) now holds the entry below; `.cursor/mcp.json` and `.codex/config.toml` get the same one
in their own format:

```json
{
  "mcpServers": {
    "ai-rulez": {
      "args": ["-y", "ai-rulez@latest", "mcp"],
      "command": "npx",
      "type": "stdio"
    }
  }
}
```

Then ask the assistant, for example, "add a billing domain with a rule about money types and regenerate".

## What the server offers

| Capability | What the assistant can do | Started with |
| ---------- | ------------------------- | ------------ |
| Authoring | Create, read, update and delete rules, context, skills, agents, commands, checks, domains, profiles, includes and installed skills; edit settings; `generate_outputs` and `clean_outputs` | `ai-rulez mcp` |
| Validation and reports | `validate_config`, `scan_content`, `doctor`, token, cost and SBOM reports, policy, approvals, verifiers | `ai-rulez mcp` |
| Governance | Roles, the content catalog, lock status and diff, version-range updates | `ai-rulez mcp` |
| Skill serving | `find_skill` and `load_skill`, the `skill://` resources and the MCP Skills extension, so skills cost tokens only when used | `ai-rulez mcp --serve-skills` |
| Nested context | `find_context` and `load_context` for the `AGENTS.md`, `CLAUDE.md`, rules and agents of subdirectories, and skills already in the repository | `ai-rulez mcp --serve-skills --discover-skills` |
| Prompts | `author-skill`, `add-rule`, `review-config`, `trim-context` | `ai-rulez mcp` |
| Resources | `ai-rulez://config`, `ai-rulez://catalog`, one item per URI | `ai-rulez mcp` |

`--serve-skills` keeps the authoring tools, so one process and one MCP entry cover both. `--read-only` registers only
the tools that change nothing, and `--tools` registers an exact list. Every flag can be pinned in the
[`[mcp]` table](#settings-in-the-configuration) instead.

<p align="center">
  <img src="assets/skills-mcp.gif" alt="ai-rulez tokens before and after serving skills over MCP, then find_skill and load_skill" width="820" />
</p>

The server speaks stdio only. The client launches it as a subprocess, so there is no port, TLS or authentication to
operate.

## Registering the server

### Generated entries

By default `generate` adds an `ai-rulez` entry to the project MCP file of each configured harness that has one. The
entry merges into an existing file: servers you added by hand stay.

| Preset | File | Key |
| ------ | ---- | --- |
| `claude` | `.mcp.json` | `mcpServers` (with `"type": "stdio"`) |
| `cursor` | `.cursor/mcp.json` | `mcpServers` |
| `codex` | `.codex/config.toml` | `[mcp_servers.ai-rulez]` |
| `copilot` | `.vscode/mcp.json` | `servers` (with `"type": "stdio"`) |
| `opencode` | `opencode.json` | `mcp` (with `"type": "local"` and `command` as one array) |
| `devin` | `.devin/mcp_config.json` | `mcpServers` |
| `antigravity` | `.agents/mcp_config.json` | `mcpServers` (no `type`) |
| `xum` | `.xum/mcp.jsonc` | `servers` (the command as one string) |
| `gemini` | `.gemini/settings.json` | `mcpServers`, only when the project also declares `[[mcp_servers]]` |

Other harnesses with an MCP file (for example `zed` in `.zed/settings.json` and `kiro` in `.kiro/settings/mcp.json`)
receive the servers of `[[mcp_servers]]` but no self entry. Declare ai-rulez there to reach them:

```toml
[[mcp_servers]]
name = "ai-rulez"
command = "ai-rulez"
args = ["mcp"]
```

A declared server named `ai-rulez` replaces the self entry in every file.

The self entry runs `npx -y ai-rulez@latest mcp`. When the project [serves skills](#dynamic-skill-loading), it runs
`mcp --serve-skills`. Change it in `[mcp]`:

```toml
[mcp]
self_server = true                        # default; false (or generate --no-self-mcp) writes no entry
self_server_version = "5.10.0"            # pin npx to a release instead of latest
self_server_launcher = "binary-then-npx"  # run an installed ai-rulez, else npx
# self_server_command = ["ai-rulez", "mcp"]  # replace the whole command
```

See [Configuration: `mcp`](configuration.md#mcp) for every key, and
[Configuration: `mcp_servers`](configuration.md#mcp_servers) for declaring other servers, `${VAR}` placeholders and
secrets.

### Registering by hand

Without `generate`, or for a user-level setup, add the entry to the client's own file. The command is the same
everywhere; only the file and the key differ.

**Claude Code**: `.mcp.json` in the project, or the CLI:

```bash
claude mcp add --scope project ai-rulez -- npx -y ai-rulez@latest mcp
```

**Cursor**: `.cursor/mcp.json` in the project, or `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "ai-rulez": { "command": "npx", "args": ["-y", "ai-rulez@latest", "mcp"] }
  }
}
```

**Codex**: `.codex/config.toml` in the project, or `~/.codex/config.toml`:

```toml
[mcp_servers.ai-rulez]
command = "npx"
args = ["-y", "ai-rulez@latest", "mcp"]
```

**VS Code with Copilot**: `.vscode/mcp.json`:

```json
{
  "servers": {
    "ai-rulez": { "type": "stdio", "command": "npx", "args": ["-y", "ai-rulez@latest", "mcp"] }
  }
}
```

**Gemini CLI**: `.gemini/settings.json`, under `mcpServers` with the same shape as Cursor.

**opencode**: `opencode.json`:

```json
{
  "mcp": {
    "ai-rulez": { "type": "local", "command": ["npx", "-y", "ai-rulez@latest", "mcp"], "enabled": true }
  }
}
```

Other launchers work the same way: `uvx ai-rulez mcp` for the PyPI package, or `ai-rulez mcp` for an installed binary.
Add `--serve-skills` (and the other flags below) to `args`, or set them in [`[mcp]`](#settings-in-the-configuration)
so every client entry stays the same.

### Launchers and client timeouts

A cold `npx -y ai-rulez@latest` downloads the package (about 30 MB) before the server starts, and some clients give up
after 30 seconds.

- **Run an installed binary.** Install once (`npm i -g ai-rulez`, `uv tool install ai-rulez`, Homebrew, or
  `go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest`; see [Installation](installation.md)) and set
  `self_server_launcher = "binary-then-npx"` or `self_server_command = ["ai-rulez", "mcp"]`. If you keep npx, pin
  `self_server_version` so a cold start does not also resolve `latest`.
- **Raise the client's timeout.** Claude Code reads `MCP_TIMEOUT` in milliseconds; `export MCP_TIMEOUT=60000` covers
  a cold start on a large repository.

The skills server also caches its catalog on disk and can share one process per repository
([Shared daemon](#shared-daemon)).

## A worked session

The assistant calls the tools; you see the effect in `.ai-rulez/` and the generated files. This session runs against
the project above with the plain authoring server.

1. Inspect what exists:

    ```jsonc
    // list_rules {}
    {"success": true, "operation": "list_rules", "domain": "", "count": 1,
     "rules": [{"name": "code-quality", "path": "/work/web/.ai-rulez/rules/code-quality.md", "type": "rules"}]}
    ```

2. Create a domain and a rule in it:

    ```jsonc
    // create_domain {"name": "billing", "description": "Billing service"}
    {"success": true, "operation": "create_domain", "name": "billing",
     "path": "/work/web/.ai-rulez/domains/billing", "message": "Domain created successfully"}

    // create_rule {"name": "money-types", "domain": "billing", "priority": "high",
    //              "content": "# Money types\n\nStore amounts as integer cents. Never use floats for money.\n"}
    {"success": true, "operation": "create_rule", "name": "money-types", "domain": "billing",
     "path": "/work/web/.ai-rulez/domains/billing/rules/money-types.md", "message": "Rule created successfully"}
    ```

3. Validate. A finding at the `fail_on` level would make the call an error result:

    ```jsonc
    // validate_config {}
    {"valid": true, "fail_on": "error", "errors": [], "warnings": [], "findings": [],
     "summary": {"total": 0, "errors": 0, "warnings": 0, "infos": 0, "by_code": {}}, ...}
    ```

4. Preview, then generate:

    ```jsonc
    // generate_outputs {"dry_run": true}
    {"message": "Dry run complete",
     "plan": ["profile: default", "write-file: AGENTS.md", "unchanged: CLAUDE.md", "unchanged: .mcp.json", ...]}

    // generate_outputs {}
    {"message": "Outputs generated successfully", "config": "/work/web/.ai-rulez"}
    ```

5. Make the domain selectable for the team:

    ```jsonc
    // add_profile {"name": "backend", "domains": ["billing"]}
    {"success": true, "operation": "add_profile", "name": "backend", "domains": ["billing"],
     "message": "Profile added successfully"}
    ```

The `review-config` prompt runs the same read-only checks and summarizes what to fix; `author-skill` drafts a skill,
creates it and checks it with `scan_content`, `validate_config` and `cost_report`.

## CLI and MCP parity

Every command of the CLI has a tool twin, or sits on one side with a stated reason. Both sides run the same library
code. The tables below are generated from the source.

<!-- parity:begin -->

**On both sides.** The command and the tool run the same library code. Rows marked *compared* are also run on one fixture by the parity test, which checks the command's `--format json` document and the tool's `structuredContent` field for field.

| Command | MCP tool | Server | Compared |
| ------- | -------- | ------ | -------- |
| `add agent` | `create_agent` | authoring |  |
| `list agents` | `list_agents` | authoring |  |
| `show agent` | `read_agent` | authoring |  |
| `remove agent` | `delete_agent` | authoring |  |
| `approve --list` | `approvals_status` | authoring | yes |
| `builtins list` | `list_builtins` | authoring |  |
| `builtins show` | `show_builtin` | authoring |  |
| `catalog` | `catalog` | authoring |  |
| `add check` | `create_check` | authoring |  |
| `list checks` | `list_checks` | authoring |  |
| `show check` | `read_check` | authoring |  |
| `remove check` | `delete_check` | authoring |  |
| `clean` | `clean_outputs` | authoring |  |
| `add command` | `create_command` | authoring |  |
| `list commands` | `list_commands` | authoring |  |
| `show command` | `read_command` | authoring |  |
| `remove command` | `delete_command` | authoring |  |
| `add context` | `create_context` | authoring |  |
| `list context` | `list_context` | authoring |  |
| `show context` | `read_context` | authoring |  |
| `remove context` | `delete_context` | authoring |  |
| `cost` | `cost_report` | authoring | yes |
| `doctor` | `doctor` | authoring |  |
| `domain add` | `create_domain` | authoring |  |
| `domain list` | `list_domains` | authoring |  |
| `domain remove` | `delete_domain` | authoring |  |
| `generate` | `generate_outputs` | authoring |  |
| `include add` | `add_include` | authoring |  |
| `include list` | `list_includes` | authoring |  |
| `include remove` | `remove_include` | authoring |  |
| `init` | `init_project` | authoring |  |
| `skill install` | `install_skill` | authoring |  |
| `skill list` | `list_installed_skills` | authoring |  |
| `skill remove` | `uninstall_skill` | authoring |  |
| `judge` | `judge` | authoring |  |
| `lock --check` | `lock_status` | authoring | yes |
| `lock --diff` | `lock_diff` | authoring |  |
| `lock --outdated` | `lock_outdated` | authoring |  |
| `okf validate` | `okf_validate` | authoring | yes |
| `validate --show-policy` | `policy_show` | authoring | yes |
| `profile add` | `add_profile` | authoring |  |
| `profile list` | `list_profiles` | authoring |  |
| `profile remove` | `remove_profile` | authoring |  |
| `profile set-default` | `set_default_profile` | authoring |  |
| `roles list` | `list_roles` | authoring |  |
| `roles resolve` | `resolve_role` | authoring |  |
| `add rule` | `create_rule` | authoring |  |
| `list rules` | `list_rules` | authoring |  |
| `show rule` | `read_rule` | authoring |  |
| `remove rule` | `delete_rule` | authoring |  |
| `sbom` | `sbom` | authoring | yes |
| `scan` | `scan_content` | authoring | yes |
| `search` | `find_skill` | skills | yes |
| `add skill` | `create_skill` | authoring |  |
| `list skills` | `list_skills` | authoring |  |
| `show skill` | `read_skill` | authoring |  |
| `remove skill` | `delete_skill` | authoring |  |
| `tokens` | `token_report` | authoring | yes |
| `update` | `update` | authoring |  |
| `update --dry-run` | `update_plan` | authoring |  |
| `validate` | `validate_config` | authoring | yes |
| `verifiers list` | `list_verifiers` | authoring |  |
| `verifiers run` | `run_verifiers` | authoring |  |
| `version` | `get_version` | authoring |  |

**Only on the command line**, deliberately. These write trust, spend money, use the network or credentials, prompt, or run for the length of a process.

| Command | Why it is not a tool |
| ------- | -------------------- |
| `edit agent` | partial field edits and stdin input; update_agent is the whole-item replacement for a tool client |
| `approve` | approving, revoking, denying and signing are identity-bound human attestations; an agent must not make them |
| `cache *` | inspects and deletes the index cache the MCP server keeps in the user cache directory; a tool client has no use for the host's disk layout, and deleting is a human decision |
| `catalog diff` | compares two catalogs through git subprocesses |
| `edit check` | partial field edits and stdin input; update_check is the whole-item replacement for a tool client |
| `edit command` | partial field edits and stdin input; update_command is the whole-item replacement for a tool client |
| `config layers` | lists the config.<name>.toml files beside config.toml (in .agents/.ai-rulez/ for an .agents project) and which are active; a tool client reads the merged settings through read_config |
| `edit context` | partial field edits and stdin input; update_context is the whole-item replacement for a tool client |
| `convert`, `migrate *` | one-shot conversions that read untrusted tool files and rewrite the tree; gated by --write or a preview |
| `eval *`, `improve *` | run models and harness subprocesses and spend money |
| `exposure` | offline report of what a Claude Code session loads at its start and on demand, run by a maintainer; no tool exposes it |
| `exposure areas` | offline view of the area map of the [exposure] settings; no tool exposes it |
| `exposure collisions` | offline report of skill and agent listings that collide; no tool exposes it |
| `exposure inventory` | offline listing of the session inventory; no tool exposes it |
| `exposure leakage` | offline report of area terms in start-of-session text; validate runs the same rules |
| `guard` | a hook entry point the harness calls on tool use |
| `skill update` | re-pins installed skills over the network and rewrites the lock |
| `list` | prints where skills and commands land per harness; generate_outputs dry_run returns the same plan |
| `llm *` | model credentials and cost estimates for network calls |
| `local *` | the machine-local overlay is per developer; tools reach it through the local argument of the item tools and update_config |
| `lock` | writes the lock, the trust anchor of the project, after network pinning and scan acceptance; an agent must not mint trust |
| `marketplace *` | adds, removes and updates installed marketplaces: network fetches that rewrite the configuration and the lock |
| `mcp` | starts the MCP server itself |
| `export okf`, `import okf` | write a whole bundle or import a foreign one into the tree |
| `plugin *` | installs, removes and updates plugins imported from marketplaces: network fetches, hook and MCP imports that run code |
| `publish *` | builds and signs distribution artifacts and may push them |
| `review *`, `rubric *` | spends model tokens in its default mode; the offline heuristics are not exposed yet |
| `roles show` | a presentation of one role; resolve_role returns what the role keeps |
| `edit rule` | partial field edits and stdin input; update_rule is the whole-item replacement for a tool client |
| `scanners *` | run external scanner programs, possibly with egress |
| `session-start` | a hook entry point the harness calls when a session starts; it runs generate |
| `sign`, `trust update` | signs with a key, an OIDC identity or a transparency log: credentials and egress |
| `edit skill` | partial field edits and stdin input; update_skill is the whole-item replacement for a tool client |
| `telemetry *` | consent and network egress of usage data |
| `verifiers calibrate`, `verifiers explain`, `verifiers suggest`, `verifiers test` | calibrate, suggest and test run models or programs; explain prints the help text of a verifier |
| `verify` | verifies signatures, attestations and downloaded artifacts with the signing backend; the drift half is generate_outputs check and doctor |

**Only on MCP**, deliberately.

| MCP tool | Server | Why it has no command |
| -------- | ------ | --------------------- |
| `update_agent` | authoring | `edit` changes single fields (description, priority, targets) as well as the body, and reads the body from stdin; the tool replaces the whole item in one atomic call |
| `update_check` | authoring | `edit` changes single fields (description, priority, targets) as well as the body, and reads the body from stdin; the tool replaces the whole item in one atomic call |
| `update_command` | authoring | `edit` changes single fields (description, priority, targets) as well as the body, and reads the body from stdin; the tool replaces the whole item in one atomic call |
| `read_config` | authoring | the command line reads config.toml with an editor or cat; a tool client has no filesystem, so it reads the parsed settings here |
| `update_config` | authoring | the command line edits config.toml directly or through `local set`; a tool client needs a typed edit that keeps comments and ordering |
| `update_context` | authoring | `edit` changes single fields (description, priority, targets) as well as the body, and reads the body from stdin; the tool replaces the whole item in one atomic call |
| `get_skill` | skills | returns a served skill with provenance and digests to a client that only speaks tools |
| `list_skill_resources` | skills | lists the files of a served skill for a client that has no skill:// resource support |
| `load_skill` | skills | reads one served skill file under the session budget; the CLI reads the file from disk |
| `read_skill_file` | skills | reads a supporting file of a served skill by its skill:// URI |
| `update_rule` | authoring | `edit` changes single fields (description, priority, targets) as well as the body, and reads the body from stdin; the tool replaces the whole item in one atomic call |
| `search_skills` | skills | lexical listing for a client that only speaks tools; the ranker the CLI `search` shares is find_skill |
| `update_skill` | authoring | `edit` changes single fields (description, priority, targets) as well as the body, and reads the body from stdin; the tool replaces the whole item in one atomic call |

<!-- parity:end -->

## Tools

`ai-rulez mcp` registers the authoring, report and governance tools below. `--serve-skills` adds six skill tools
and `--discover-skills` two context tools. A client sees the exact list in `tools/list`; each tool has a `title`, a
description and JSON input and output schemas.

### Common arguments

| Argument | Tools | Meaning |
| -------- | ----- | ------- |
| `working_directory` | All except `get_version`, `list_builtins`, `show_builtin` and the skill and context tools | Directory to operate in. Relative paths resolve against the server's start directory. |
| `config_file`, `config_dir` | Generation, validation, `doctor`, report (except `okf_validate`), governance and pin tools | Root configuration file, or configuration directory name (default `.ai-rulez`) |
| `no_local` | Most of the same tools | Ignore the machine-local `config.local.*` overlay and `local/` content |
| `local` | Content, include, installed-skill, profile and `update_config` tools | Write to the machine-local layer instead of the shared one |

Like the CLI, a tool walks up from `working_directory` to the nearest directory with a configuration, stopping at the
git root and never above your home directory. `init_project`, and a call that passes `config_file` or `config_dir`,
do not walk up.

**Confinement.** `working_directory`, `config_file` and `config_dir` must lie inside the directory the server was
started in, after symlinks are resolved. Start the server with `--root <dir>` to confine it elsewhere, or with
`--allow-any-dir` to lift the check.

**`local: true`.** Content tools (`create_*`, `read_*`, `update_*`, `delete_*`, `list_*` for rules, context, skills,
agents and commands) use the `.ai-rulez/local/` tree (`domain` selects `local/domains/<name>/`). `add_include`,
`remove_include`, `install_skill`, `uninstall_skill`, `update_config`, `add_profile`, `remove_profile` and
`set_default_profile` write the `config.local.*` overlay. Checks, domains and the remaining list tools are shared only.
See [Local configuration](local-overrides.md).

### Results and errors

- Every tool returns `structuredContent` and the same JSON as a text block, for clients that read only text.
- Input schemas set `additionalProperties: false`. An unknown argument, a wrong type, a value outside an enum or a
  missing required argument is an error result before the tool runs.
- A failure is an error result (`isError: true`): a configuration that does not load, findings at `fail_on`, a lock
  check without a lock, arguments that fail validation. The text carries the CLI's hint. Tools whose result is a
  report (`validate_config`, `doctor`, the report tools) also return the report in `structuredContent`.
- Annotations: `readOnlyHint`, `destructiveHint` and `idempotentHint` say what a tool writes. `openWorldHint` is true
  only on `judge`, `lock_outdated`, `update_plan` and `update`, which use the network.
- Items of rules, context and skills use lower-case keys (`name`, `path`, `type`); items of domains, profiles, includes
  and installed skills use capitalised keys (`Name`, `Path`, `Domains`, `IsDefault`). Paths are absolute.
- `generate_outputs` with `recursive`, and `token_report` over several targets, send `notifications/progress` when
  the request carries a progress token, and stop when the client cancels.

### Content tools

Rules, context, skills, agents, commands and checks share one pattern:

| Tool | Arguments (required in bold) |
| ---- | ---------------------------- |
| `create_rule`, `create_context` | **`name`**, `content`, `domain`, `priority`, `targets`, `local` |
| `update_rule`, `update_context` | **`name`**, **`content`**, `domain`, `priority`, `targets`, `local` |
| `create_skill` | **`name`**, `content`, `description`, `domain`, `priority`, `targets`, `local`, `scope`, `harness`, `allow_write` |
| `update_skill` | **`name`**, **`content`**, `domain`, `priority`, `targets`, `local`, `scope`, `harness`, `allow_write` |
| `create_agent`, `create_command` | **`name`**, `description`, `content`, `domain`, `local` |
| `update_agent`, `update_command` | **`name`**, **`content`**, `domain`, `local` |
| `create_check` | **`name`**, `content`, `description`, `severity`, `tools`, `targets`, `domain` |
| `update_check` | **`name`**, `domain`, and `content` or any of `description`, `severity`, `tools`, `targets` |
| `read_*`, `delete_*` | **`name`**, `domain`, `local` (not for checks) |
| `list_rules`, `list_context`, `list_skills`, `list_agents`, `list_commands`, `list_checks` | `domain`, `local` (not for checks) |

- `name` is the file name without `.md`; `domain` selects `domains/<name>/` (the root when empty).
- `priority` is `critical`, `high`, `medium`, `low` or `minimal`; `targets` is a list of presets or output paths (see
  [Targets](rules.md#targets)); `severity` is `low`, `medium`, `high` or `critical` (see [Checks](checks.md)).
- `content` is markdown with optional frontmatter. With no `content`, `create_skill` builds a `SKILL.md` from
  `description`, and `create_agent` and `create_command` write a template.
- `update_*` replaces the whole item in one atomic write. `update_check` without frontmatter in `content` keeps the
  existing frontmatter.
- With `source = "agents"` and `agents_md_split = true`, `read_context` and `update_context` work on one `##` section
  of `AGENTS.md`; an update that would add, drop or rename a heading is refused. See [.agents as a source](agents-source.md).
- `scope` writes a skill into a repository directory instead of the ai-rulez tree; see
  [Writing a skill into a scope](#writing-a-skill-into-a-scope).

### Project structure tools

| Tool | Arguments | Does |
| ---- | --------- | ---- |
| `create_domain` | **`name`**, `description` | Create `domains/<name>/` with `rules/`, `context/` and `skills/` |
| `delete_domain` | **`name`** | Delete a domain and everything in it |
| `list_domains` | | List the domains |
| `add_profile` | **`name`**, **`domains`**, `local` | Define a profile |
| `remove_profile` | **`name`**, `local` | Remove a profile; a shared profile cannot be removed locally |
| `set_default_profile` | **`name`**, `local` | Set `default` |
| `list_profiles` | | List profiles and which is the default |
| `add_include` | **`name`**, **`source`**, `path`, `ref`, `include`, `merge_strategy`, `install_to`, `local` | Add an [include](includes.md) |
| `remove_include` | **`name`**, `local` | Remove an include; with `local` a shared one is hidden with `remove = true` |
| `list_includes` | | List includes (shared layer) |
| `install_skill` | **`name`**, **`source`**, `path`, `ref`, `version`, `include`, `exclude`, `name_prefix`, `trust`, `resources`, `tag_prefix`, `include_prerelease`, `min_release_age`, `domain`, `profiles`, `local_override`, `local` | Add an [installed skill](installed-skills.md); the arguments are its config fields |
| `uninstall_skill` | **`name`**, `local` | Remove an installed skill |
| `list_installed_skills` | | List installed skills (shared layer) |

`merge_strategy` is `local-override` (default), `include-override` or `error`. A `source` URL with a credential in it
(`https://user:token@host/...`) is refused, because it would be written into `config.toml` in clear; use
`AI_RULEZ_GIT_TOKEN` or a git credential helper.

### Configuration and generation tools

| Tool | Arguments | Does |
| ---- | --------- | ---- |
| `init_project` | `project_name`, `providers`, `all_providers`, `popular_providers`, `with_agents`, `source` | Create the layout `ai-rulez init` creates; refuses an existing configuration, including one in a parent directory of the same repository |
| `read_config` | | The parsed settings: `name`, `description`, `presets`, `profiles`, `builtins`, `includes`, `gitignore`, `default_effort`, `default_effort_by_preset`, `rules_mode`, `rules_mode_by_preset`, and `local_overlay` (key paths only, never values) |
| `update_config` | `name`, `description`, `builtins`, `gitignore`, `agents_md`, `default_effort`, `default_effort_by_preset`, `rules_mode`, `rules_mode_by_preset`, `local` | Change those settings, keeping comments and ordering; an empty string or map clears a key |
| `generate_outputs` | `dry_run`, `check`, `offline`, `profile`, `role`, `recursive`, `exclude`, `no_self_mcp` | Run `generate` |
| `clean_outputs` | `dry_run`, `keep_gitignore`, `keep_manifest` | Remove the generated files, the manifest and the managed `.gitignore` block; never touches `.ai-rulez/` |
| `validate_config` | `fail_on`, `strict`, `config_only`, `lint_profile`, `analyzers` | Run `validate`; the result is the `validate --format json` document plus `valid`, `errors` and `warnings` |
| `doctor` | `profile`, `strict` | Run `doctor` |
| `get_version` | | The ai-rulez version |
| `list_builtins`, `show_builtin` | `show_builtin`: **`name`** | The builtin domains, or one in full |

`read_config` returns the shared view, so a read-modify-write loop never copies local values into the shared file.

`generate_outputs` with `check` writes nothing and returns `{"status", "roots", "blocked", "differing"}`; drift is an
error result, as `generate --check` exits 2. Because there is no terminal to show them, a run that writes new hook, MCP,
plugin or permission commands returns them in `new_commands`. The drift guard of `generate` applies too:
`--allow-local-drift` is CLI-only, so pass `no_local: true` to generate the shared view.

### Report tools

Read-only twins of commands. Each returns the document the command prints with `--format json`, uses no network
(remote content comes from the lock and the cache), and turns a gate the command would exit 2 on into an error result
that still carries the document.

| Tool | Command | Arguments |
| ---- | ------- | --------- |
| `scan_content` | `scan` | `fail_on`, `lint_profile` |
| `token_report` | `tokens` | `profile`, `role`, `by_role`, `compare_profiles`, `tokenizer`, `budget` |
| `cost_report` | `cost` | `profile`, `target`, `top`, `tokenizer`, `budget`, `on_demand_budget` |
| `sbom` | `sbom` | `type` (`cyclonedx`, `spdx-json`), `files`, `profile`, `role`, `include_outputs`, `no_approvals` |
| `okf_validate` | `okf validate` | **`bundle`** (a directory in the project), `fail_on` |
| `approvals_status` | `approve --list` | `all` |
| `policy_show` | `validate --show-policy` | |
| `list_verifiers` | `verifiers list` | |
| `run_verifiers` | `verifiers run` | `name`, `strict`, `since`, `staged`, `rule` |

`run_verifiers` executes nothing: a command predicate reports an error and an LLM verifier is skipped. Approving,
signing, writing an SBOM file and `sbom --check` stay on the command line.

### Governance tools

Read-only, no network: remote includes and installed skills resolve from the local cache only, and one that is not
cached is left out with a note. Each returns the JSON of the matching command.

| Tool | Command | Arguments |
| ---- | ------- | --------- |
| `list_roles` | `roles list --format json` | |
| `resolve_role` | `roles resolve <role> --format json` | **`role`**, `limit` |
| `catalog` | `catalog --format json` | `kind` (`rule`, `skill`, `agent`, `command`, `check`, `context`), `role`, `limit` |
| `lock_status` | `lock --check --format json` | `kind` (`include`, `skill`, `source`, `served`, `content`), `profile`, `role`, `targets`, `include_static`, `sources` |
| `lock_diff` | `lock --diff --format json` | `profile`, `role`, `targets`, `include_static`, `sources` |

`resolve_role` and `catalog` list at most `limit` items (default 200, maximum 1000); a cut list carries
`"truncated": true` and `total_items`. `lock_status` on a project that was never locked is an error result, as
`lock --check` exits 1; `lock_diff` never fails for a missing lock. Both follow `schema/lock-diff.schema.json`, and
`lock_status` always ignores the machine-local overlay. Writing the lock stays on the command line.

### Pin tools: lock_outdated, update_plan, update

These three run the code of `ai-rulez lock --outdated`, `ai-rulez update --dry-run` and `ai-rulez update`. They ask the
remotes the project already declares for their tags, so they use the network, and send nothing of the project.
`config.toml` is never edited.

| Tool | Command | Writes | With `--read-only` |
| ---- | ------- | ------ | ------------------ |
| `lock_outdated` | `lock --outdated --format json` | nothing | listed |
| `update_plan` | `update --dry-run --format json` | nothing (refreshes the local cache) | listed |
| `update` | `update [name...] --format json` | `ai-rulez.lock` | not registered |

All three take the common arguments plus `kind` (`include`, `skill`, `source`, `marketplace` or `plugin`) and `names`,
the sources to limit the run to (default: every source with a `version` constraint). A name that matches no such
source is an error.

`lock_outdated` returns `schema/lock-outdated.schema.json`; `update_plan` and `update` return
`schema/update.schema.json`. A successful `update` adds a text block with the next step: `ai-rulez generate`, then
`ai-rulez lock`.

`update_plan` and `update` run the pre-pin security scan (`AR001`-`AR009`) over every new tree, and refuse a tag that
moved since it was pinned (`AR732`), an unsatisfiable constraint (`AR730`), an invalid one (`AR731`) and a tag held back
by `min_release_age` (`AR733`). A refusal is an error result carrying the update document, with the refused sources
under `blocked` or the scan under `updates[].scan`; nothing is written.

The tools take no `accept_findings`, `accept_moved_tag`, `allow_downgrade`, `major` or `write_config` argument: an
agent cannot override a refusal. A person reviews it and runs `ai-rulez update --accept-moved-tag` or
`--accept-findings`. See [Lock file](lockfile.md).

### Judge

`judge` runs [`ai-rulez judge`](judge.md): model reviewers over the project's agent configuration. It sends file content
to your model provider and spends money, so it requires `allow_llm: true` on the call and `[judge] allow_llm = true` in
the user configuration. `estimate: true` sends nothing and returns the units, the models and the cost estimate. The
other arguments mirror the command's flags (`paths`, `categories`, `fail_on`, `min_confidence`, `changed`, `since`,
`skills`, `redact`, ...). `--read-only` does not register it.

### Prompts

| Prompt | Arguments | Expands to |
| ------ | --------- | ---------- |
| `author-skill` | `name`, `purpose` | Draft a skill, create it, then check it with `scan_content`, `validate_config` and `cost_report` |
| `add-rule` | `name`, `guideline`, `priority` (optional) | Check for an existing rule, then create or update one and validate |
| `review-config` | `focus` (optional) | Run the read-only checks and summarize what to fix, changing nothing |
| `trim-context` | `budget` (optional) | Find the largest always-loaded items and propose edits before applying them |

A prompt is listed only when every tool it calls is registered, so a read-only server offers `review-config` alone.

### Resources

| URI | Content |
| --- | ------- |
| `ai-rulez://config` | The parsed settings, as `read_config` |
| `ai-rulez://catalog` | The content catalog, as `catalog`. With discovery it adds `skill_discovery` and `scopes`, narrowed by `?path=`, `?scope=` and `&recursive=true` |
| `ai-rulez://{kind}/{name}` | The markdown of one root item; `kind` is `rules`, `context`, `skills`, `checks`, `agents` or `commands` |
| `ai-rulez://domains/{domain}/{kind}/{name}` | The same for an item of a domain |
| `skill://{name}/{path}` | With `--serve-skills`: every file of a served skill |
| `context://{scope}/{name}` | With discovery: one nested instruction file, rule or agent, read from the repository on each read |
| `ai-rulez://index` | With discovery: the index generation, item and scope counts and the cache report |

Resources are read-only and confined like the tools. An unknown URI is a resource-not-found error.

## Serving skills

`ai-rulez mcp --serve-skills` serves the skills of a profile to the agent, alongside the authoring tools, from one
process. It implements the MCP [Skills extension](https://modelcontextprotocol.io/seps/2640-skills-extension)
(`io.modelcontextprotocol/skills`), so a client with MCP but no local skills directory can discover and load skills
with no files on disk.

```bash
ai-rulez mcp --serve-skills
ai-rulez mcp --serve-skills --role backend
ai-rulez mcp --serve-skills --profile backend --targets cursor --domain api --deny 'internal-*'
ai-rulez mcp --serve-skills --read-only
```

The skills are rendered in memory at start-up by the same code as `generate`, so every served file is byte-identical
to what `generate` writes for that profile and preset. Nothing is written.

### What is exposed

| Surface | Detail |
| ------- | ------ |
| `find_skill` | **`task`**, `limit` (default 5, max 20), `role`, `path`, `scope`, `recursive`. Ranked matches with name, description, score, domain and digest. |
| `load_skill` | **`name`**, `path` (a file inside the skill, default `SKILL.md`), `budget_bytes`. The file, `provenance`, `digest`, an index of the skill's other files, and `files_access` when it has any. |
| `list_skill_resources` | **`name`**, `offset`. Every file of the skill with path, URI, size, MIME type, digest, `mode`, `executable` and an `interpreter` hint. |
| `search_skills` | `query`, `limit`, `domain`. A lexical listing; an empty query lists every skill. |
| `get_skill` | **`name`**. `SKILL.md` with provenance and per-file digests. |
| `read_skill_file` | **`uri`**, `encoding`, `offset`, `length`. One file by `skill://` URI; see [Served files](#served-files-scripts-and-assets). |
| Resources | `skill://<name>/<path>`, for example `skill://pdf-processing/references/FORMS.md`, with the digest in `_meta` |
| Extension methods | `skills/list` and `skills/get` return `{uri, frontmatter, resources: [{uri, digest, size}]}` with `sha256:` digests; `resources/directory/read` lists the children of `skill://<name>` or a subdirectory |

All skill tools are read-only. The server sends `notifications/resources/list_changed` when the served set changes.

**Ranking.** `find_skill` uses the [skill search ranker](search.md#the-ranker): lexical BM25 by default, hybrid with
an embedding index, falling back to lexical (and saying why in `ranking` and `degraded`) when vectors are unavailable.
`search_skills` stays lexical.

**Roles.** `role` on `find_skill` ranks the role's skills first and marks the rest `in_role: false`. It is a ranking
lens, not access control: `--role` at start decides what the server can load at all.

**Budget.** A session may read `--budget-bytes` bytes of skill and context content (default 262144, 256 KiB; `-1`
removes the cap). `load_skill`, `get_skill`, `read_skill_file`, `load_context` and `resources/read` share it; listing is
free and a repeated read is charged again. A read past the remainder is refused and not charged. `budget_bytes` on one
call truncates that file and reports `truncated` and `total_bytes`.

**Provenance.** Results carry `digest` (over every file of the skill, so it changes when any file does), `lock_digest`
(what the lock pins), `locked`, `source` (the authored path or the repository), `ref`, `pinned` and `commit` for a
remote skill, `delivery`, and `scan_warnings`. Log the `digest` to record exactly what a session loaded.

### Serve flags

| Flag | Meaning |
| ---- | ------- |
| `--profile` | Profile whose skills are served (default: the configured default) |
| `--role` | Serve only the skills a [role](roles.md) keeps, with the delivery the role gives them; also the default role of `find_skill`. Excludes `--profile`. |
| `--targets` | Preset whose rendering is served (default: the first configured preset with skills) |
| `--domain` | Only skills of these domains (repeatable); `root` selects skills in no domain |
| `--allow`, `--deny` | Globs on the skill name; `--deny` wins |
| `--include-static` | Also serve skills whose delivery is `static` |
| `--source` | Serve a [skill source](#skill-sources) as well: `[git+]<url>[@<tag\|commit>][#<subdir>]` or a directory (repeatable) |
| `--discover-skills[=repo\|all\|off]` | Serve the skills and context already in the repository; see [Discovering skills](#discovering-skills-already-in-the-repository). Write the value with `=`. |
| `--exclude <glob>` | Paths discovery never reads (repeatable); see [Excluding paths](#excluding-paths) |
| `--frozen` | No network; `ai-rulez.lock` must cover every remote include, installed skill and skill source |
| `--offline` | No network; use the lock and the cache |
| `--budget-bytes` | Session read budget (see above) |
| `--max-clone-bytes` | Clone size limit for git skill sources that set no `max_clone_bytes` |
| `--usage-log`, `--usage-sink` | Record each `load_skill` ([Usage telemetry](#usage-telemetry)) |
| `--no-watch`, `--reload-interval` | Turn [live reload](#live-reload) off, or change the 2 s interval |
| `--daemon` | Share one server per repository ([Shared daemon](#shared-daemon)) |

These flags are rejected without `--serve-skills`. `--read-only`, `--tools`, `--root` and `--allow-any-dir` apply to
both modes, as do the organization policy flags (`--policy`, `--policy-digest`, `--discover-org` and the other
`--policy-*` flags; see [Organization policy](policy.md)).

Notes for client authors:

- Two served skills with the same name stop the server at start-up: the name is the `skill://` path.
- A skill without a `description` is served with its name as the description, and a start-up warning names it.
- A skill whose frontmatter is not valid YAML is refused: `load_skill` and `get_skill` answer `malformed-frontmatter`
  with the reason.
- `skills/list` returns the whole catalog in one page.

With `--read-only`, a skills server registers only `find_skill`, `load_skill`, `search_skills`, `read_skill_file` and
`list_skill_resources`, and no prompts: the full tool list is about 160–170 KB of schema that a client sends on
every request. Set `[mcp] authoring = true` to add the read-only authoring tools, or `tools = [...]` for an exact list.

## Dynamic skill loading

With static generation every skill's name and description is listed in every session; measure what that costs with
`ai-rulez tokens`. Dynamic skill loading serves the skills you choose over MCP instead, so the agent pays for one only when it
asks:

1. **`delivery`** decides per skill: written to the harness skill trees (`static`), served over MCP (`served`), or
   both.
2. **`generate`** leaves `served` skills out of every static tree and writes one small stub skill, `dynamic-skills`
   (about 100 tokens), that tells the agent to call `find_skill` and `load_skill`. The self entry then runs
   `mcp --serve-skills`.
3. **`ai-rulez mcp --serve-skills`** serves them, scans each one before serving, enforces the lock and reloads when
   files change.

```markdown
<!-- .ai-rulez/skills/db-migrations/SKILL.md (frontmatter) -->
---
name: db-migrations
description: Plan and run database schema changes safely. Use when changing a table.
delivery: served
triggers: [schema change, alembic migration, add a column]
---
```

```toml
[skills]
delivery = "static"        # project default; static when unset

[domains.billing]
delivery = "served"        # every skill of the billing domain
```

The first match wins: the skill's `delivery` frontmatter, the role's `delivery` map (see
[Roles](roles.md#delivery)), `[domains.<name>] delivery`, `[skills] delivery`, then `static`. An invalid frontmatter
value is reported as `AR994` and the next level applies. `triggers` (a list, or a comma-separated string) and
`keywords` are searched by `find_skill`. `delivery` is not written into generated frontmatter.

When no skill sets a delivery anywhere, `--serve-skills` serves every skill. Once any skill is `served` or `both`, only
those are served; `--include-static` adds the rest.

### Choosing the delivery

| Delivery | Use for |
| -------- | ------- |
| `static` | Core conventions the agent needs in nearly every session: code style, commit rules, the repository map |
| `served` | Domain skills used in some sessions: billing, migrations, a vendor API, incident response. The bulk of a large catalog. |
| `both` | A skill that ships `scripts/` or `assets/` the agent runs by relative path, or one that must stay visible to one harness while others find it by search |

- **The model must choose to search.** A served skill is invisible until the agent calls `find_skill`. Write
  `description` and `triggers` in the words a user would use, and keep must-not-miss skills `static`.
- **Always-loaded content cannot rely on a served skill.** `AR990` warns when a rule or static skill names one.
- **Bundled files need `both`.** A served skill's `scripts/` and `assets/` are never written to disk, so a relative
  path into them resolves to nothing (`AR9R0`). `references/` are readable over MCP either way.
- **`disable-model-invocation: true`** marks a skill as user-only. The skills server hides it from the model:
  `find_skill`, `search_skills` and the `skill://` resources leave it out and `load_skill` refuses it. Combined with
  `served` nobody can run it (`AR9R2`); combined with `both`, a rule that points at it by name leaves the model declining
  (`AR9R1`). Use `both` with a file-path pointer, drop the flag, or set `[mcp] serve_user_invoked_skills = true`.

### What generate does

- A `served` skill is not written to any preset's skill tree; `static` and `both` skills are.
- When any skill is `served` or `both`, or a `[[skill_sources]]` entry exists, the `dynamic-skills` stub is added to
  every preset whose harness can call MCP tools (in `.agents/skills` when `agents_md` is on). An authored skill named
  `dynamic-skills` replaces the stub. [`[delivery.stub]`](configuration.md#deliverystub) customises its text.
- A preset whose harness cannot call MCP (for example `cline` or `rovodev`) keeps served skills as static files and
  gets no stub, with a warning (`AR992`). Skills of `[[skill_sources]]` never reach such a preset. See
  [Installed skills vs skill sources](installed-skills.md#installed-skills-vs-skill-sources).
- `AR993` warns when skills are served but nothing runs `--serve-skills` (`self_server = false` and no
  `[[mcp_servers]]` entry with the flag).

To serve a second view (another profile, role or source) as its own server, declare it:

```toml
[[mcp_servers]]
name = "ai-rulez-backend"
command = "ai-rulez"
args = ["mcp", "--serve-skills", "--role", "backend"]
```

### Skill sources

A source is a repository or directory of skill directories that is served and never written to the static trees.

```toml
[[skill_sources]]
name = "acme"                                  # identifies the source in the lock
url = "https://github.com/acme/skills.git"     # https, ssh or file URL, or a local directory
ref = "v1.2.0"                                 # a tag or full commit SHA; or version = "^1.2"
path = "skills"                                # subdirectory whose children are skills
include = ["pdf-*", "sql"]                     # directory-name globs; exclude wins
exclude = ["*-wip"]
name_prefix = "acme-"                          # served as acme-pdf-forms
trust = "error"                                # scan level: error (default) or warn
```

| Key | Default | Meaning |
| --- | ------- | ------- |
| `version`, `tag_prefix`, `include_prerelease` | | A semver range resolved by `ai-rulez lock` and moved only by `ai-rulez update`; excludes `ref`. See [Version constraints](lockfile.md#version-constraints). |
| `max_skills` | 200 | Skills the source may load |
| `max_bytes` | 64 MiB | Bytes of skill files it may load |
| `max_clone_bytes` | 256 MiB | Size of the git clone; also `AI_RULEZ_MAX_CLONE_BYTES` or `--max-clone-bytes` |
| `max_clone_files` | 20000 | Entries of the git clone |

- **Pinning.** `ai-rulez lock` records the commit and tree digest of each source. With the lock, serving fetches the
  pinned commit and fails when the tree does not match. A branch, or a tag the lock does not cover, is reported as
  unpinned (`AR010`).
- **Private repositories.** Git runs without hooks, prompts, submodules or credential helpers, over `https`, `ssh` and
  `file` only. A private repository needs an `ssh` URL that works non-interactively (an ssh-agent key).
- **Clones.** A git source is fetched as a partial clone, and with `path` only that subdirectory is checked out. A
  clone that grows past `max_clone_bytes` is stopped and leaves nothing in the cache.
- **Cache.** Trees are cached per commit under `~/.cache/ai-rulez/skill-sources/`. `--frozen` needs the lock and a
  cached commit; `--offline` uses the lock's commit or one an earlier online run recorded.
- **Local directories.** A local `url` in the project config must resolve inside the project. `--source <dir>` and
  sources in your user configuration may point anywhere.
- **Names.** A source skill is served under its directory name plus `name_prefix`. One that collides with a project
  skill or an earlier source's skill is skipped; the project's skill wins.

### Served files: scripts and assets

A served skill's files stay in the server. To run a bundled script, a client fetches it, writes it out and restores its
mode.

- `list_skill_resources`, `load_skill`, `get_skill` and `read_skill_file` report per file `mode` (such as `"0755"`),
  `executable`, and an `interpreter` hint from the shebang or extension (`sh`, `python3`, `node`, ...). The extension
  methods carry the same in `_meta`.
- `read_skill_file` returns text, or base64 for a binary file (or with `encoding: "base64"`). One call returns at most
  256 KiB; continue with `offset` while the reply carries `next_offset`.
- To materialise a skill: for each entry of `list_skill_resources`, refuse a `path` that is absolute or contains `..`,
  read the file with `read_skill_file {uri, encoding: "base64", offset}` until `next_offset` is absent, write the
  bytes under a directory you own and `chmod` to `mode`.

Or set `delivery: both` and run `ai-rulez generate`, which writes the files to disk.

### Security scan

Every served skill is scanned before it is served, with the rules of `ai-rulez scan`: secrets, hidden characters,
prompt-injection phrases, risky shell, unrestricted `allowed-tools`, encoded blobs. Inline `ai-rulez-lint-ignore`
comments are not honoured. A skill that fails is not served: it is absent from every listing, a warning names the
finding on stderr, and `load_skill` says why.

| Level | Blocks | Default for |
| ----- | ------ | ----------- |
| `error` | Any finding | Skill sources, installed skills and skills from `[[includes]]` (`lint.security.scan_imports = "warn"` lowers the last two) |
| `warn` | Only findings that are errors by their own severity (secrets, hidden characters, risky shell) | Skills authored in the project |

A file the scan cannot read (binary, or over 512 KiB) is `AR989`. At `error` it is not served; a `SKILL.md` that
cannot be scanned refuses the skill. At `warn` it is served with the warning.

### Lock enforcement

`ai-rulez lock` records a `[[served]]` entry with the digest of every skill the server would serve, and a `[[source]]`
entry per skill source. With `[lock] enforce` (the default whenever `ai-rulez.lock` exists), the server refuses a
skill the lock does not pin or whose digest differs (`AR995`).

The lock pins each *serve view* separately: the profile, role, `--targets`, `--include-static` and `--source` a server
starts with. A plain `lock` pins the default view, every role and every view the lock already records. Pin another view
with the flags the server uses:

```bash
ai-rulez lock --role backend
ai-rulez lock --targets cursor
ai-rulez lock --source git+https://github.com/acme/skills@v1.2.0#skills
ai-rulez mcp --serve-skills --role backend --frozen
```

A skill the scan refuses is not pinned: `lock` reports it, pins the rest and exits 3, or with `--refuse-findings`
writes nothing. See [Lock file](lockfile.md#served-skills-and-skill-sources).

### Live reload

The server checks the configuration directory, local includes, local policy files and local skill sources every two
seconds (`--reload-interval`). When a file changes it rebuilds the catalog and swaps it in, and sends
`notifications/resources/list_changed` only when a served skill was added, removed or edited. A failed rebuild, or a
`SKILL.md` caught half-saved, keeps the previous catalog serving and is retried. Git sources are immutable per commit
and are not re-fetched.

Every minute (also with `--no-watch`) the server re-checks approval expiry and signature validity, and stops serving a
skill whose approval expired, whose signature no longer verifies, or whose digest an organization policy denies
(`AR747`, `AR717`).

### Start-up cost and the catalog cache

Rendering and scanning every skill of a large monorepo takes several seconds. The admitted catalog is cached in
`~/.cache/ai-rulez/serve/<project-hash>/catalog.json`, keyed by the files live reload watches and the view's flags,
and authenticated with a key in `~/.config/ai-rulez/serve-cache.key`. A restart with unchanged content restores it
without rendering or scanning; any change, or an entry that fails its check, rebuilds. The cache is off while
discovery is on (discovery has the [index cache](#the-index-cache) instead).

### Shared daemon

`--daemon` (or `[mcp] daemon = true`) makes one long-lived process per repository build the catalog once and serve
every session over a private unix socket. The first session starts the daemon; later sessions connect in milliseconds
and proxy their stdin and stdout to it.

```bash
ai-rulez mcp --serve-skills --daemon
```

- **Identity.** A daemon is keyed by the repository, the ai-rulez version and the command line. A different MCP entry
  or version gets its own daemon; sessions in subdirectories of one checkout share one. Content edits are picked up by
  live reload.
- **Lifetime.** It exits after `daemon_idle_timeout` (default 15 minutes) with no client, and at once when its
  repository is removed. Its log is `daemon.log` beside the socket under `~/.cache/ai-rulez/daemons/`.
- **Security.** The socket is mode `0600` in a `0700` directory. A client refuses a socket owned by another user or
  open to group or others, and serves in-process instead.
- **Fallback.** On any failure (no daemon can start, a stale or unsafe socket, a version or view mismatch) the session
  is served in-process exactly as without `--daemon`. On Windows `--daemon` always serves in-process, with a warning.

Settings such as `serve_user_invoked_skills` are read when the daemon starts; restart it after changing them.

### Usage telemetry

Nothing is recorded unless you opt in with `--usage-log <file>`, `--usage-sink <command>` or
`[usage] skills_index = true` (which logs to `<config dir>/local/usage.jsonl`). Each successful `load_skill` is then one
identifier-only JSON line: time, skill name, a salted session hash, the client name, the role, the content hash and
digest. A sink receives the same line on stdin from a queue of 256 records, so a slow sink never delays a load. See
[Usage telemetry](usage-telemetry.md).

### Validation codes

| Code | Severity | Meaning |
| ---- | -------- | ------- |
| `AR989` | warning or error | A served file cannot be scanned (binary or over 512 KiB) |
| `AR990` | warning | Always-loaded content names a served skill |
| `AR991` | error | A harness that can call MCP has no `dynamic-skills` stub |
| `AR992` | warning | A harness without MCP keeps served skills as static files |
| `AR993` | warning | Skills are served but no MCP entry runs `--serve-skills` |
| `AR994` | error | `delivery` is not `static`, `served` or `both` |
| `AR995` | error | `[lock] enforce` and a served skill is unpinned or its digest differs |

See [Strict validation](strict-validation.md) for the full list.

## Repository discovery

### Discovering skills already in the repository

A repository often keeps skills ai-rulez does not define: a `skills/` folder, `.claude/skills` in a package, a
directory set up for another harness. `--discover-skills` (or `[mcp] discover_skills`) serves them through the same
`find_skill`, `load_skill` and `skill://` resources, under the same scan, budget and `--allow`/`--deny` rules, with no
`generate` run and no copy.

```toml
[mcp]
serve_skills = true
discover_skills = "repo"   # off (default), repo or all
```

```bash
ai-rulez mcp --serve-skills --discover-skills        # repo
ai-rulez mcp --serve-skills --discover-skills=all
```

| Mode | Serves |
| ---- | ------ |
| `off` | The skills ai-rulez defines and the skill sources |
| `repo` | The above, plus every skill directory found in the working tree |
| `all` | `repo`, plus installed and included skills whatever their delivery |

Either mode also indexes nested instruction files, rules and agents ([Monorepos](#monorepos-scopes-and-nested-context)).

**Where it looks.** A directory with a `SKILL.md`, at most six levels deep, below:

- `skills/` (not inside a hidden directory);
- `.claude/skills`;
- the skill directory of each configured preset (`.agents/skills`, `.cursor/skills`, `.github/skills`, ...).

In a git repository only files git lists are read (tracked, or untracked and not ignored), so `.gitignore` applies.
`.ai-rulez/`, `.git/` and `node_modules/` are never searched and symlinks are not followed. A skill directory ai-rulez
generated is skipped, so nothing is served twice. A skill has the limits of a skill source (2 MiB a file, 8 MiB and
2000 files a skill, 500 skills in all).

**Names.** A skill is served under its frontmatter `name` (or its directory name). The skills ai-rulez defines and the
skill sources claim names first and *shadow* a repository skill of the same name; a builtin skill is the exception and
is replaced by a repository skill. Among repository skills the shallowest wins, and a later one is served under its
path with `/` turned into `.` and leading dots dropped: `pkg/.claude/skills/bar` becomes `pkg.claude.skills.bar`.
Every rename and shadowing is a start-up warning and is listed under `skill_discovery` in `ai-rulez://catalog`.

**Trust and lock.** A discovered skill is scanned at the level of a skill authored in the project. Under
`[lock] enforce` it must be pinned: set `discover_skills` in the configuration, not only the flag, so `ai-rulez lock`
sees it.

**Without a configuration.** `--discover-skills` works in a directory with no `.ai-rulez/`. It then behaves as if the
`claude` and `codex` presets were configured, and registers only the tools that need no configuration: the skill and
context tools, `get_version`, `list_builtins`, `show_builtin`, `policy_show`, `okf_validate` and `init_project`.

```bash
cd ~/src/web            # has .claude/skills/deploy/SKILL.md, no .ai-rulez/
ai-rulez mcp --serve-skills --discover-skills
```

### Monorepos, scopes and nested context

A harness loads a nested `AGENTS.md`, `CLAUDE.md` or `.claude/` tree only once it works inside that directory, so a
planner at the repository root never sees it. With discovery on, the server indexes the repository by **scope** and
answers "what applies to this path?" the way a harness would.

**Scopes.** A scope is a directory that holds `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.claude/`, `.agents/`, `.codex/`,
`.cursor/rules`, `.github/copilot-instructions.md` or a `skills/` directory with a skill. The root is the scope `.`.
Scopes are inferred from the files; nothing is configured, and the `presets` list does not matter (only the skill
directories follow it). Each indexed item has a `kind` (`skill`, `instruction`, `rule`, `agent`, `command`), a `source`
(`claude`, `agents`, `codex`, `cursor`, `copilot`, `gemini`, `skills-dir`), a `name`, a `description` and a content
`hash`.

**What applies to a path.** The scope of the path and its ancestors, nearest first. A skill, agent or command is
shadowed by one of the same name in a nearer scope; instruction files and rules add up.

For a repository with `pkg/api/AGENTS.md`, `pkg/api/.claude/rules/handlers.md` and
`pkg/api/.claude/skills/lint/SKILL.md`:

```jsonc
// find_skill {"task": "lint the code", "path": "pkg/api/handlers/user.go"}
{"task": "lint the code", "path": "pkg/api/handlers/user.go", "scopes": ["pkg/api", "."], "count": 2,
 "results": [
  {"name": "lint", "scope": "pkg/api", "score": 1.83, "description": "Lint the api package with golangci-lint", ...},
  {"name": "code-reviewer", "score": 1.67, "description": "Specialized agent for code review and quality assurance", ...}]}
```

A skill ai-rulez defines belongs to no scope and applies everywhere. `scope` asks for one exact scope (`"."` is the
root), and `recursive: true` adds the scopes below it. `path` and `scope` are mutually exclusive.

**`find_context`** takes the same `path`, `scope` and `recursive`, plus `task` to rank by, `kind` (`instruction`, `rule`
or `agent`) and `limit` (default 10, max 50), and returns summaries:

```jsonc
// find_context {"path": "pkg/api/handlers/user.go"}
{"count": 2, "total": 2, "scopes": ["pkg/api", "."],
 "results": [
  {"name": "AGENTS.md", "kind": "instruction", "scope": "pkg/api", "source": "agents",
   "description": "API conventions", "uri": "context://pkg/api/AGENTS.md", "hash": "sha256:..."},
  {"name": "handlers", "kind": "rule", "scope": "pkg/api", "source": "claude", "paths": ["handlers/**"],
   "description": "Handler rules", "uri": "context://pkg/api/handlers", "hash": "sha256:..."}]}
```

**`load_context`** reads one item, by `scope` and `name` or by `uri`, with an optional `budget_bytes`. The text is read
from the repository at call time, never from the index:

```jsonc
// load_context {"scope": "pkg/api", "name": "AGENTS.md"}
{"name": "AGENTS.md", "kind": "instruction", "scope": "pkg/api", "uri": "context://pkg/api/AGENTS.md",
 "content": "# API conventions\n\nHandlers return typed errors.\n", "bytes": 49, "truncated": false,
 "hash": "sha256:...", "budget_remaining_bytes": 262095}
```

The result carries `changed_since_index: true` when the file changed since the index was built. Commands are indexed
and counted but not served as context.

The context tools apply the skill rules: `--allow` and `--deny` match an item's `name` or `scope/name`; the security
scan runs when the index is built and again on every load, and a file with an error finding is withheld; reads count
against the session budget. Domains, roles, the lock and approvals apply to skills only.

**Catalog and instructions.** `ai-rulez://catalog` gains a `scopes` array with per-kind counts and sources, narrowed
with `?path=`, `?scope=` and `&recursive=true`. The server instructions name the first eight scopes with their counts
(`pkg/api (1 skill, 1 instruction, 1 rule)`), so a planner knows nested content exists without a call.

**Limits.** The index holds at most 20000 items, keeping the shallowest scopes (`skill_discovery.index.truncated` says
when it cut). An instruction, rule or agent file over 1 MiB is skipped. `path`, `scope` and `uri` must be relative to the
project, without `..`, backslashes or control characters.

#### Rules and skills with `paths`

A rule or skill can name the files it applies to: `paths:` in the frontmatter of a Claude Code rule or `SKILL.md`, and
`globs:` of a Cursor `.mdc` rule that is not `alwaysApply`. With a `path` query, `find_context` and `find_skill` return
such an item only when the path matches one of its globs. The globs are relative to the scope directory:
`pkg/api/.claude/rules/handlers.md` with `paths: "handlers/**"` applies to `pkg/api/handlers/user.go`, not to
`pkg/api/other/handlers/user.go`. `**/*.py` matches at any depth, `*.py` only directly in the scope; a Cursor glob such as
`*.tsx` also matches at any depth.

- A directory `path` keeps an item when some glob could match a path below it.
- A `scope` query lists every rule and skill of the scope whatever its globs; loading one by name is unchanged.
- A rule whose `paths:` cannot be read (broken YAML, or a value that is not a string or list of strings) fails closed:
  a `path` query never returns it, and a `scope` query lists it with `"paths_unreadable": true`. `ai-rulez validate`
  reports `AR306` and `AR9W4`. A Cursor rule with broken `globs:` is served as applying everywhere until fixed.

### Writing a skill into a scope

`create_skill` and `update_skill` with `scope` write a harness skill file into a repository directory, where a harness
working there loads it, instead of editing ai-rulez content. `allow_write: true` is required on the call.

```jsonc
// create_skill {"scope": "pkg/web", "name": "e2e", "description": "Run the Playwright end-to-end tests of the web app",
//               "allow_write": true}
{"success": true, "operation": "create_skill", "name": "e2e", "scope": "pkg/web", "harness": "claude",
 "path": "pkg/web/.claude/skills/e2e/SKILL.md",
 "message": "Skill written to pkg/web/.claude/skills/e2e/SKILL.md; it is the harness's file, not ai-rulez content"}
```

- **Where.** `<scope>/<harness skills dir>/<name>/SKILL.md`. `harness` names an enabled preset; by default the first
  enabled preset that keeps skills in a directory (`claude` is `.claude/skills`, `codex` is `.agents/skills`).
- **Checks.** The scope must be an existing project directory with no symlink on the way; a path that leaves the
  project is refused. A generated copy is refused (edit the source under `.ai-rulez/`). The content is scanned at the
  strictest level and any finding refuses the call.
- **Create and update.** `create_skill` refuses an existing skill; `update_skill` refuses a missing one. Without
  `content`, `create_skill` builds a `SKILL.md` from `description`.
- **Lock.** The file is not ai-rulez content and is not generated. With an `ai-rulez.lock`, the result carries `lock`
  because the new file is drift until `ai-rulez lock` records it.

`scope` cannot be combined with `domain`, `priority`, `targets` or `local`. Instruction files and rules stay read-only
over MCP, and a `--read-only` server does not register the writing tools.

### The index cache

A cold build of the index is dominated by the security scan of every item. The index cache keeps the summary of each
indexed file (name, description and content hash, keyed by path, size and modification time) and the scan verdicts of
content found clean, so the next start skips that work. It never stores a file body.

```toml
[mcp]
index_cache = "user"   # user (default), repo or off
```

| Mode | Location |
| ---- | -------- |
| `user` | `<user cache>/ai-rulez/monorepo-index/<repo-id>/index.json`, nothing in the repository. Falls back to `repo` when the user cache is not writable. |
| `repo` | `.ai-rulez/.cache/monorepo-index/` (`.ai-rulez-cache/monorepo-index/` in the root layout), listed in `.git/info/exclude` |
| `off` | No cache |

```bash
ai-rulez cache status    # location, size, entries and problems
ai-rulez cache clear     # drop it
```

`ai-rulez doctor` reports the cache too.

- **Freshness.** A summary is reused while the file's size and modification time and the git tree are unchanged. A new
  tree drops the summaries and keeps the verdicts, which are keyed by content. A new ai-rulez build or changed `[lint]`
  settings drop the verdicts.
- **Safety.** The file is written with mode `0600` under a lock, so several servers on one repository share it. It
  carries an HMAC with a per-user key outside the cache (`~/.config/ai-rulez/index-cache.key`); a file that was edited,
  truncated or planted is discarded and rebuilt. A cached verdict is used only for the exact bytes and trust level it
  was recorded for, `--allow` and `--deny` run on every build, and `load_context` scans what it reads every time.

A warm start of a large monorepo takes a fraction of a cold one.

### Live refresh of the index

While `--serve-skills --discover-skills` runs, a file watcher follows the project tree with a 300 ms debounce. When an
instruction file, a `SKILL.md`, anything under `.claude`, `.agents`, `.codex`, `.cursor`, `.github` or `skills`, or a
directory appears, changes or goes, the server rebuilds the index. The rebuild re-reads only the changed files and
scans only content without a clean verdict.

When the index changed, the server sends one `notifications/resources/list_changed`; the client then calls
`find_context` or `find_skill` again. Reading `ai-rulez://index` returns the current generation, the item and scope
counts and the cache report:

```json
{"generation": 1, "items": 3, "scopes": 2, "found": 9, "truncated": false,
 "cache": {"mode": "user", "dir": "/home/dev/.cache/ai-rulez/monorepo-index/8674292ae5a9225b",
           "hits": 0, "misses": 9, "entries": 9, "verdicts": 5}}
```

- `.git`, `node_modules`, `vendor`, `target`, `dist`, `build`, excluded paths and the configuration directory (which
  live reload polls) are not watched.
- The watcher holds at most 2048 descriptors, watching shallow directories first; past that, and when the operating
  system's watch limit is reached, the server warns and refreshes on the poll interval only.
- `--no-watch` (or `no_watch = true`) turns the watcher off with the rest of live reload.
- The server instructions are written at start-up and are not rewritten.

### Excluding paths

`--exclude <glob>` (repeatable) and `[mcp] exclude` keep paths out of discovery entirely:

```toml
[mcp]
discover_skills = "repo"
exclude = ["third_party/**", "**/fixtures/"]
```

```bash
ai-rulez mcp --serve-skills --discover-skills --exclude 'examples/**' --exclude '**/testdata/'
```

- Globs are relative to the project root with slashes. `**` crosses directories, `*` and `?` stay within one, `{a,b}`
  alternates. A pattern without a slash (`AGENTS.md`, `legacy`) matches at any depth, and a pattern naming a
  directory (`third_party`, `third_party/`, `third_party/**`) excludes everything below it.
- The flag's globs are added to the configuration's.
- An excluded file is dropped before anything is read: no parsing, scan or cache entry. The globs apply to skill
  directories and the whole context index (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, Copilot instructions, rules, agents
  and commands). An excluded directory marks no scope, and the watcher ignores it.
- A glob that leaves the project (`../x`) or is empty is rejected. `--exclude` without discovery is an error; the
  configuration key is ignored while discovery is off.

## Settings in the configuration

Pin how `ai-rulez mcp` runs in the `[mcp]` table, so every launcher (`.mcp.json`, `.cursor/mcp.json`,
`.codex/config.toml`, a teammate's editor) runs the same server without repeating flags.

```toml
[mcp]
serve_skills = true                                   # a bare `ai-rulez mcp` serves skills
discover_skills = "repo"
read_only = true
tools = ["find_skill", "load_skill", "find_context", "load_context"]
deny = ["draft-*"]
daemon = true
```

| Key | Flag | Applies | Meaning |
| --- | ---- | ------- | ------- |
| `serve_skills` | `--serve-skills` | always | Serve skills from a bare `ai-rulez mcp` |
| `read_only` | `--read-only` | always | Register only tools that change nothing |
| `tools` | `--tools` | always | Register exactly these tools; an unknown name stops the server with the valid list |
| `authoring` | | serving | Keep the read-only authoring tools and `review-config` on a read-only skills server |
| `allow`, `deny` | `--allow`, `--deny` | serving | Globs on served skill and context names |
| `discover_skills` | `--discover-skills` | serving | `off`, `repo` or `all` |
| `exclude` | `--exclude` (adds) | serving | Paths discovery never reads |
| `index_cache` | | serving | `user`, `repo` or `off` |
| `no_watch` | `--no-watch` | serving | Turn live reload and the watcher off |
| `daemon` | `--daemon` | serving | Share one server per repository |
| `daemon_idle_timeout` | | serving | How long the daemon outlives its last client, as a duration (default `15m`) |
| `serve_user_invoked_skills` | | serving | Also serve skills that set `disable-model-invocation: true` |

The `self_server*` and `skills_instructions*` keys of the same table control generation; see
[Configuration: `mcp`](configuration.md#mcp).

- **A flag wins.** A flag on the command line replaces the key, including `--read-only=false` and an empty `--tools=`.
- **Serving keys need serving.** A "serving" key set while skills are not served logs one warning naming it. The same
  setting as a flag stops the server with `requires --serve-skills`.
- **Layers.** The keys are read from `.ai-rulez/config.toml`, a root `ai-rulez.toml`, `config.<name>.toml` layers and
  `config.local.toml`; a list in a later layer replaces the earlier one.
- **Read at start.** Change a key, then restart the server (or let a daemon idle out).
- **Policy only tightens.** An [organization policy](policy.md) with `[mcp] read_only = true` keeps the server read-only
  whatever the project or the flags say; a project that sets `read_only = false` under it is reported as `AR740`.
- **The surface follows the tools.** The server instructions and the prompts list only what is registered, so a
  read-only server does not tell the model to use `generate_outputs`.

## Troubleshooting

| Symptom | Cause and fix |
| ------- | ------------- |
| The client shows the server as failed or timed out on first use | A cold `npx` download. Install the binary and use `self_server_launcher = "binary-then-npx"`, or raise the client timeout (`MCP_TIMEOUT=60000` for Claude Code). |
| No `ai-rulez` entry in `.gemini/settings.json` | Gemini gets the self entry only when `[[mcp_servers]]` is non-empty. Declare `ai-rulez` there. |
| A tool answers that `working_directory` "is outside the directory this server is allowed to use" | The server is confined to its start directory. Start it with `--root <dir>` or `--allow-any-dir`. |
| `find_skill` does not return a skill | It is `static` and the project serves only `served`/`both` skills (use `--include-static`), it sets `disable-model-invocation`, the scan refused it (see stderr), or the lock does not pin it (`AR995`; run `ai-rulez lock`). |
| `Error: --exclude requires --serve-skills` | A serving flag on a plain `ai-rulez mcp`. Add `--serve-skills` or set `serve_skills = true`. |
| `find_context` is not listed | Discovery is off. Set `discover_skills` or pass `--discover-skills`. |
| A setting change has no effect | Settings are read at start. Restart the client's server, or the daemon. |

## See also

- [Configuration: `mcp`](configuration.md#mcp) and [`mcp_servers`](configuration.md#mcp_servers)
- [Monorepo](monorepo.md)
- [Skill search](search.md)
- [Lock file](lockfile.md)
- [CLI: `mcp`](cli.md)
