# Enabling the MCP Server

The `ai-rulez` MCP (Model Context Protocol) server allows your AI assistant to programmatically and safely interact with your `.ai-rulez/` configuration. Since V4 uses a file-based approach with inline MCP configuration, you define servers directly in `config.toml`, and the MCP server provides read, CRUD, validation, and generation tools for AI assistants.

**You do not need to start the server manually.** Your AI assistant will start it automatically based on the configuration you provide.

With `--serve-skills` the same server also serves your skills on demand, so they stop costing tokens in every session:

<p align="center">
  <img src="assets/skills-mcp.gif" alt="ai-rulez tokens before and after serving skills over MCP, then find_skill and load_skill" width="820" />
</p>

---

## Configuration Examples

To enable the server, add one of the following snippets to your AI assistant's configuration file (e.g., Cursor's `settings.json`), or define it inline in `.ai-rulez/config.toml`. The top-level key (`mcpServers` below) differs between clients; check your assistant's documentation.

### Using `npx` (Recommended for Node.js users)

This method ensures you are always using the latest version of `ai-rulez` without needing to install it globally.

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "npx",
      "args": ["-y", "ai-rulez@latest", "mcp"]
    }
  }
}
```

### Using `uvx` (Recommended for Python users)

This method uses `uvx` to run `ai-rulez` in an ephemeral environment.

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "uvx",
      "args": ["ai-rulez", "mcp"]
    }
  }
}
```

### Using a Local Go Installation

If you have installed `ai-rulez` locally with `go install`.

```json
{
  "mcpServers": {
    "ai-rulez": {
      "command": "ai-rulez",
      "args": ["mcp"]
    }
  }
}
```

### Inline in `config.toml` (V4 Recommended)

V4 supports defining MCP servers directly in your `.ai-rulez/config.toml`:

```toml
[[mcp_servers]]
name = "ai-rulez"
command = "npx"
args = ["-y", "ai-rulez@latest", "mcp"]

[[mcp_servers]]
name = "grafana"
command = "uvx"
args = ["mcp-grafana"]
env = { GRAFANA_URL = "http://localhost:3000", GRAFANA_SERVICE_ACCOUNT_TOKEN = "${GRAFANA_SERVICE_ACCOUNT_TOKEN}" }

[[mcp_servers]]
name = "remote-api"
transport = "http"
url = "https://mcp.example.com/mcp"
headers = { Authorization = "Bearer ${REMOTE_API_TOKEN}" }

[[mcp_servers]]
name = "slack"
type = "http"                      # the same setting as transport, spelled as the harness's `type`
url = "https://mcp.slack.com/mcp"
[mcp_servers.oauth]                # a pre-registered OAuth client (hosted servers)
client_id = "1234567890.1234567890"
callback_port = 8080
scopes = ["channels:read", "chat:write"]
```

`oauth` is translated to each harness that has an `oauth` block: Claude Code's `.mcp.json` and VS Code's
`.vscode/mcp.json` get `{ "clientId", "callbackPort", "scopes" }`, and Codex's `.codex/config.toml` gets an
`[mcp_servers.<name>.oauth]` table with `client_id` and `callback_port`. A harness that cannot express it
skips the server with a warning instead of writing one that could not authenticate. `scan` reports an `oauth`
block with no `client_id` and two same-named servers that resolve to different endpoints or transports
(`AR602`).

MCP env and header values may contain `${VAR}` placeholders. `ai-rulez generate` resolves them from repeated
`--env KEY=VALUE` flags, process environment variables, then dotenv files. Placeholder names must
match `[A-Za-z_][A-Za-z0-9_]*`. By default, `.env` is loaded from the generation base directory. If
any `--env-file PATH` flags are supplied, the default `.env` is not loaded; multiple files are merged
in flag order, with later files winning. Generation fails if a placeholder cannot be resolved.

### Generated Self-Entry (`[mcp] self_server`)

`generate` adds the ai-rulez server to the project `.mcp.json` **by default**, pinned to the version
of the ai-rulez binary that ran it, so a project is MCP-ready out of the box. Turn it off with
`self_server = false` or `generate --no-self-mcp`:

```toml
[mcp]
self_server = false   # opt out; the default is true
```

The entry is merged into an existing `.mcp.json`, so hand-authored servers survive and the file is
ready for Claude Code (which reads project MCP servers from `.mcp.json`). Every entry has the same
shape: `"type": "stdio"` (or `http`/`sse`) and, only for a server turned off with `enabled = false`,
`"disabled": true`. See [Configuration: `mcp`](configuration.md#mcp) for
`self_server_version`, `self_server_command`, `skills_instructions`, and the interaction with `[[mcp_servers]]`.

Generated MCP config files contain resolved values. If a value came from a placeholder, or if an env
key contains `TOKEN`, `SECRET`, `PASSWORD`, `KEY`, or `CREDENTIAL`, generation fails before writing
unless `.mcp.json`, `.gemini/settings.json`, `.agents/settings.json`,
`opencode.json`, or a scoped variant is gitignored or covered by planned `--gitignore` patterns.
Secret-bearing values are redacted before source-hash calculation, not from generated MCP config
files. Any generated file that contains a resolved secret value is written with mode `0600`.

Servers can also be defined per machine in the [`config.local.*` overlay](local-overrides.md), which
keeps credentials out of the shared config.

Set `enabled = false` on an inline `[[mcp_servers]]` entry to skip it in generated MCP outputs.

---

## Server Capabilities

When enabled, the MCP server provides your AI assistant with access to your configuration and CRUD operations. The server supports:

- **Read Configuration**: Inspect rules, context, skills, profiles, and presets
- **Generate Outputs**: Programmatically trigger generation of tool-specific files. There is no terminal over MCP, so
  the hook, MCP, env, plugin and allow-rule commands the run newly wrote are returned as `new_commands` (and written to
  stderr); an unchanged run omits the field. See [Configuration](configuration.md#checks-generate-runs-before-it-writes)
- **Validate Configuration**: Check configuration validity and report errors
- **CRUD Operations**: Create, read, update, and delete domains, rules, context, skills, agents, commands, includes, and profiles
- **Reports**: Read-only twins of `scan`, `tokens`, `cost`, `sbom`, `okf validate`, `approve --list` and
  `validate --show-policy`, which return the document the command prints with `--format json`
- **Prompts and resources**: `author-skill`, `add-rule`, `review-config` and `trim-context` prompts, and
  `ai-rulez://config`, `ai-rulez://catalog` and `ai-rulez://{kind}/{name}` resources

The MCP server enables AI assistants to:

1. **Understand your setup** by reading configuration and content files
2. **Modify configuration** programmatically via CRUD tools
3. **Generate outputs** after changes
4. **Validate changes** before committing

This approach ensures your configuration remains auditable and version-controlled, while allowing AI assistants to help you manage it efficiently.

## CLI and MCP parity

The command line and the MCP tools are two views of one engine, and a table in the source decides which
capability lives where. Every runnable command and every tool belongs to exactly one row of
`internal/parity/capabilities.go`, either paired with its counterpart or left on one side with a stated reason.
`go test ./tests/parity` walks the real Cobra tree and lists the real tools of both servers through an in-memory
client, and fails when:

- a command or a tool is in no row, or a row names a command or tool that does not exist;
- a one-sided row has no reason;
- a paired command and tool differ in an input: each flag must have a tool argument of the same JSON type, enum and
  required-ness, or sit in an explicit, justified exclusion (and the other way round);
- a read-only pair, run on one fixture project, gives different documents. `validate`, `scan`, `tokens`, `cost`,
  `sbom`, `okf validate`, `lock --check`, `approve --list`, `validate --show-policy` and `search` are compared field for
  field, with each justified difference written next to the check.

The tables below are generated from the same rows (`go test ./tests/parity -run TestDocsMatchTheTable -update-docs`
rewrites them), and the test fails when they are stale.

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
| `guard` | a hook entry point the harness calls on tool use |
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
| `skill update`, `update` | moves pinned remote sources to newer tags over the network and rewrites the lock |
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

Added a command or a tool? The first parity test failure names the row to add.

---

## Serving Skills (`--serve-skills`)

`ai-rulez mcp --serve-skills` serves the skills of a profile to consumers as well as the authoring
tools, all from **one process** under the name `ai-rulez`. It implements the MCP
[Skills extension](https://modelcontextprotocol.io/seps/2640-skills-extension)
(`io.modelcontextprotocol/skills`, SEP-2640, status *Final*, verified 2026-10-04), so a client that
supports MCP but not a local skills directory can discover and load the skills of a profile with no
files on disk. The authoring tools (create, update, delete, generate, ...) are registered too, so a
project that serves skills needs one MCP entry rather than the self entry plus a separate skills
server; pass `--read-only` to register only the read-only config and report tools. It never writes
`.claude/settings.json`, so `--role` does not touch the role's `skillOverrides`
(`generate --role` does, and restores the hand-written values it replaced).

```bash
ai-rulez mcp --serve-skills --profile backend
ai-rulez mcp --serve-skills --profile backend --targets claude --domain api --deny 'internal-*'
ai-rulez mcp --serve-skills --read-only          # serve skills and inspect, never write
```

| Flag | Meaning |
| ---- | ------- |
| `--profile` | Profile whose skills are served. Default: the configured default profile. A profile is the role: it selects the domains whose skills apply. |
| `--targets` | Preset whose rendering is served (its frontmatter dialect and placement rules). Default: the first configured preset that produces skills. |
| `--domain` | Keep only skills of these domains (repeatable); `root` selects skills owned by no domain. A name that is not a domain of the project is an error at start. |
| `--allow` / `--deny` | Glob patterns on the skill name. `--allow` keeps only matching skills; `--deny` always wins. |
| `--read-only` | Register only the read-only authoring tools (validate, scan, doctor, the read/list and report tools). The server can inspect a project but never change it. The `judge` tool is not registered either: it sends file content to a model provider and spends money. |
| `--tools` | Register only these tools (repeatable or comma separated), for example `--tools find_skill,load_skill,find_context,load_context` for a server that can only read skills and context. Applies to `mcp` and to `mcp --serve-skills`. An unknown name is an error that lists the valid tools; a valid name the server would not register anyway (a writing tool under `--read-only`, a context tool without discovery) is simply absent. |
| `--root` / `--allow-any-dir` | Confine the authoring tools to a directory, as for plain `mcp`. |

The skill set is rendered once at start-up, in memory, by the same code as `generate`, so every
`SKILL.md` and supporting file is byte-identical to what `generate` writes for the same profile and
preset. Nothing is written. Edits are picked up by [live reload](#live-reload).
The rendered and admitted catalog is also cached on disk, so a restart restores it instead of
rendering and scanning every skill again ([Start-up cost and the catalog cache](#start-up-cost-and-the-catalog-cache)).
When the filters remove every skill, the start-up warning names each filter and how many skills it leaves.

### What is exposed

- **Resources**: every file of a skill as `skill://<name>/<path>` (`skill://pdf-processing/SKILL.md`,
  `skill://pdf-processing/references/FORMS.md`), readable with `resources/read`. `_meta` carries
  `io.modelcontextprotocol.skills/digest`.
- **`skills/list`** and **`skills/get`** (JSON-RPC methods defined by the extension): the entry of a
  skill is `{uri, frontmatter, resources: [{uri, digest, size}]}` with `sha256:<hex>` digests, so a
  client can verify what it loads. An unknown URI returns `-32602`. The server declares
  `capabilities.extensions["io.modelcontextprotocol/skills"]` and the `resources` capability.
  The server also implements the extension's optional `resources/directory/read` and declares
  `directoryRead: true`: given `skill://<name>` or one of its subdirectories (no trailing slash), it returns the
  direct children as resources (files with their `mimeType` and size, subdirectories as `inode/directory`), and
  `-32602` for anything that is not such a directory. It is read-only and lists only files `resources/read` serves.
- **Tools** for clients that only speak tools, all annotated read-only: `search_skills(query, limit, domain)`
  ranks by name, keywords (frontmatter `keywords`), description and domain, lexically and
  deterministically (an empty query lists everything); `get_skill(name)` returns `SKILL.md` plus
  provenance and per-file digests; `read_skill_file(uri, encoding, offset, length)` returns a supporting file
  (see [Served files](#served-files-scripts-and-assets)). `offset` and `length` (and the `offset` of
  `list_skill_resources`) must be whole numbers from 0 to 2147483647; a fractional, negative, non-finite or larger
  value is rejected with an error naming the argument. The `limit` of `find_skill` and `search_skills` and the
  `budget_bytes` of `load_skill` take zero or a negative number as "use the default"; a fractional, non-finite or
  larger-than-2147483647 value is rejected the same way.
- **The authoring surface**: the same tools, prompts and `ai-rulez://` resources as plain `ai-rulez mcp`
  (create, update, delete, validate, generate, reports), so the one server both serves skills and edits
  the project. With `--read-only` only the read-only tools are registered.

### Provenance

Search and `get_skill` results carry `digest` (a sha256 over the digests of all the skill's files, so
it changes when any file does), `source` (the authored path, or the repository of an installed
skill), and, for installed skills, `ref` and `pinned` (true when `ref` is a full commit SHA). Log the
`digest` to record exactly what a session loaded. The digest covers the skill's whole file tree, including files
the server does not serve (unscannable files at `trust = "error"`), exactly like `lock_digest`; the served files
are listed with their own digests.

### Notes and limits

- The Go SDK dispatches only the methods it knows, so `skills/list` and `skills/get` are answered in a
  thin JSON-RPC layer in front of the SDK; everything else, including resources and tools, is served
  by the SDK normally. The stdio transport is the only one `ai-rulez mcp` offers: the harness launches the
  server as a subprocess it already controls, so there is no port, TLS certificate or cross-origin policy to
  operate. A streamable-HTTP transport (and elicitation, sampling and roots) is deliberately not offered; a
  hosted deployment would need its own auth and CORS policy, and those capabilities wait on the 2026-07-28 spec
  revision.
- A malformed line never ends the session. Input that is not valid JSON gets `-32700`; a frame that is not a
  JSON-RPC 2.0 request (wrong or missing `jsonrpc`, an object or boolean `id`, an empty or invalid batch) gets
  `-32600`; a line over 16 MiB is dropped with `-32600`. The server keeps reading after each. A `resources/read`
  of an unknown URI gets the not-found error with the URI escaped.
- `skills/list` returns the whole catalog in one page (no `nextCursor`).
- A skill URI uses the skill name as its path, so two served skills must not share a name; the server
  refuses to start when they do.
- The extension requires every skill to have a `name` and a `description`. A skill without a
  description is served under its name as the description, and a warning on stderr names it (add a
  `description` so `find_skill` can rank it). The served bytes are not rewritten. A skill whose frontmatter
  cannot be parsed, or whose name is not a valid `skill://` path segment, is skipped with a warning; the
  rest are unaffected. A skill whose frontmatter is not valid YAML (`validate` fails on it) is refused:
  `load_skill` and `get_skill` answer `malformed-frontmatter` with the reason, and `generate` fails the same way.
- Semantic ranking is opt-in: with `[search] mode = "hybrid"` and an index built by `ai-rulez search index`, `find_skill` fuses BM25F with embedding similarity (see [Skill search](search.md)). By default the ranking is lexical.

## Dynamic skill loading

Static generation lists every skill (its name, description and about 27 tokens of framing) in every
session: roughly 14.5k tokens for 190 skills. **Dynamic skill loading** serves the skills you choose over
MCP instead, so the agent pays for them only when it asks. Three pieces work together:

1. **`delivery`** decides per skill whether it is written to the harness skill trees (`static`), served
   over MCP (`served`), or both.
2. **`generate`** leaves served skills out of every static tree and writes one small stub skill
   (`dynamic-skills`, about 100 tokens) that tells the agent to call `find_skill` and `load_skill`.
3. **`ai-rulez mcp --serve-skills`** serves those skills with `find_skill`, `load_skill`,
   `list_skill_resources` (and the tool-only `search_skills`, `get_skill` and `read_skill_file`), scans each one before serving, can pin them in `ai-rulez.lock`, and reloads
   when their files change.

### Decision guide

| Keep the skill | When |
| -------------- | ---- |
| `static` | A core convention the agent needs in nearly every session (code style, commit rules, the repository map). The listing costs a few tokens and it never depends on the model deciding to search. |
| `served` | Domain skills used in some sessions: billing, migrations, a vendor API, incident response. The bulk of a large catalog. |
| `both` | A skill that ships `scripts/` or `assets/` the agent runs by relative path, or that must always be visible to one harness while other harnesses or roles find it by search. |

Rules of thumb: serve domain skills, keep a handful of core convention skills static. Caveats:

- **The model has to choose to call `find_skill`.** A served skill is invisible until it does. Write
  `description` and `triggers` for search (what the user is trying to do, in their words), and keep the skills
  that must not be missed `static`.
- **An MCP-only harness is not enough for every harness.** A harness that cannot call MCP tools gets served
  skills as static files, with a warning (`AR992`), never silently dropped. Served skills also need an MCP
  server entry that runs `ai-rulez mcp --serve-skills`; the generated self entry does that whenever the
  project serves skills, and `AR993` warns only when nothing does (`self_server = false` and no such entry).
- **Served skills are not in the context until loaded.** Nothing that is always loaded (a rule, a static skill)
  can rely on them being there; `AR990` warns when static content names a served skill, and says the file will
  not be there when the pointer is a path.
- **Bundled files need `delivery: both`.** A `served` skill's `scripts/` and `assets/` are never written to
  disk, so a relative path into them (`scripts/foo.py`, the form SKILL.md usually uses) resolves to nothing;
  `AR9R0` warns. Set `delivery: both` when SKILL.md tells the agent to run a bundled script, or fetch the file
  over MCP and write it out first ([Served files](#served-files-scripts-and-assets)). `references/` are readable
  over MCP either way.
- **`disable-model-invocation` plus `both` is a trap.** `both` keeps the skill's file on disk, but
  `disable-model-invocation: true` hides the skill from the Skill tool, which refuses it and tells the model not
  to reproduce the workflow another way. A rule that points at it by name ("run the `x` skill", `Skill(x)`) then
  leaves the model declining even though it could read the file; `AR9R1` warns. Point at the file path
  (`.claude/skills/<id>/SKILL.md`), or set `[mcp] serve_user_invoked_skills = true` and use `load_skill` (the skills
  server hides a user-only skill from the model otherwise, so a `load_skill` or `find_skill` pointer fails too), or
  drop `disable-model-invocation`.
- **`disable-model-invocation` plus `served` is unreachable.** A `served` skill is never written to disk, so the
  user has no slash command for it, and the skills server hides it from the model: nobody can run it. `AR9R2`
  warns. Use `delivery: both`, or `[mcp] serve_user_invoked_skills = true`, or drop `disable-model-invocation`.

### Choosing the delivery

```yaml
---
name: db-migrations
description: Plan and run database schema changes safely. Use when changing a table.
delivery: served
triggers: [schema change, alembic migration, add a column]
---
```

```toml
[skills]
delivery = "static"        # global default; static when unset

[domains.billing]
delivery = "served"        # every skill of the billing domain
```

Precedence, first match wins: the skill's own `delivery` frontmatter, then the role's `delivery` map (when a role
is being rendered or served, see [Roles](roles.md#delivery)), then `[domains.<name>] delivery`, then
`[skills] delivery`, then `static`. An invalid value is ignored (the next level applies) and reported as `AR994`;
invalid config values fail `validate`. `config.Config.EffectiveDelivery(skill, domain, roleOverride)` is the
single function that resolves all of this.

`triggers` is a list of phrases that should make the agent look for the skill (a comma-separated string works
too). `keywords` are searched as well. `delivery` and `triggers` are ai-rulez keys: `delivery` is not written
into generated frontmatter.

### What `generate` does

- A skill whose delivery is `served` is not written to any preset's skill tree. `static` and `both` skills are.
- When any skill is `served` or `both`, or a `[[skill_sources]]` entry is configured (its skills are served,
  never written), one stub skill named `dynamic-skills` is added to the root skills of
  every preset whose harness can call MCP tools. With `agents_md` on, the presets that read the shared
  `.agents/skills` tree (codex, cursor, gemini, copilot, opencode, zed, ...) get the stub in that tree. A skill you author with that name, at the root or in a domain
  that is active, is used instead of the stub; if several are authored, a warning asks you to rename all but one.
  `[delivery.stub]` customises the generated stub's description and text, see
  [Configuration: `[delivery.stub]`](configuration.md#deliverystub).
- A preset whose harness cannot call MCP (for example `cline`, `rovodev`, or any custom preset) keeps every
  served skill as a static file and gets no stub. Each such preset is named in a warning (`AR992`).
  Skills of `[[skill_sources]]` are served only, by design: they never reach a preset without MCP support
  (nothing is written for them), and `generate` and `validate --strict` warn (`AR992`) for each such preset
  when `[[skill_sources]]` is configured. See
  [Installed skills vs skill sources](installed-skills.md#installed-skills-vs-skill-sources) to choose between them.
- `ai-rulez mcp --serve-skills` renders the served skills itself; it never depends on the generated trees.

### Serving

```bash
ai-rulez mcp --serve-skills                       # the skills whose delivery is served or both
ai-rulez mcp --serve-skills --role backend        # the backend role's skills, with the role's delivery
ai-rulez mcp --serve-skills --source git+https://github.com/acme/skills@v1.2.0#skills/
ai-rulez mcp --serve-skills --frozen              # lock required, no network
```

Register it in `config.toml` so every MCP-capable harness launches it. A project that serves skills
needs no entry of its own: the generated self entry runs `mcp --serve-skills` (see
[Generated Self-Entry](configuration.md#mcp)), so `.mcp.json` holds one `ai-rulez` entry. The
generated `dynamic-skills` stub names the self entry `ai-rulez`, or the `[[mcp_servers]]` entry whose
`args` include `--serve-skills` when you declare one:

```toml
[mcp]
self_server = true   # default; the ai-rulez entry runs `mcp --serve-skills` when skills are served

# Optional: only when self_server is off, or to serve a second view (another
# profile, role or source) as its own server.
[[mcp_servers]]
name = "ai-rulez-skills"
command = "ai-rulez"
args = ["mcp", "--serve-skills"]
```

A project that sets no delivery anywhere serves every skill, as `--serve-skills` always did. Once any skill is
`served` or `both`, only those are served; `--include-static` adds the rest.

| Flag | Meaning |
| ---- | ------- |
| `--source` | Serve the skills of a source as well (repeatable, see [Skill sources](#skill-sources)). |
| `--role` | A role of `[[roles]]`. The server serves only the skills the role keeps, with the delivery the role gives them (a skill the role delivers `static` is not served unless `--include-static` is set), and `find_skill` ranks within the role by default. Mutually exclusive with `--profile`; an unknown role is an error at start. |
| `--frozen` | Never use the network and require `ai-rulez.lock` to cover every remote include, installed skill and skill source. |
| `--offline` | Never use the network; use the lock and the cache when present. |
| `--include-static` | Also serve skills whose delivery is static. |
| `--discover-skills[=mode]` | Also serve the skills already in the repository: `repo` (the default when the flag has no value) or `all`; `off` turns a configured discovery off. Write the value with `=`. Either mode also indexes nested instruction files, rules and agents by scope. See [Discovering skills already in the repository](#discovering-skills-already-in-the-repository) and [Monorepos](#monorepos-scopes-and-nested-context). |
| `--exclude <glob>` | Leave the paths a glob matches out of repository discovery: skill directories and the context index alike. Repeatable; added to `[mcp] exclude`. Needs discovery (`--discover-skills` or `[mcp] discover_skills`). See [Excluding paths](#excluding-paths). |
| `--max-clone-bytes` | Clone size limit in bytes for git skill sources that set no `max_clone_bytes`. Overrides `AI_RULEZ_MAX_CLONE_BYTES`; 0 (default) defers to it, then to 256 MiB. |
| `--budget-bytes` | Bytes of skill content a session may read, through `load_skill`, `get_skill`, `read_skill_file` and `resources/read` together. Default 262144 (256 KiB); any negative value removes the cap. |
| `--usage-log`, `--usage-sink` | Where to record each `load_skill` (see [Usage telemetry](#usage-telemetry)). |
| `--no-watch`, `--reload-interval` | Turn live reload off, or change the two-second check interval. |
| `--daemon` | Share one long-lived skills server per repository: proxy this session's stdin/stdout to a per-repo daemon, starting it on first use, and serve in-process when none can be reached (see [Shared daemon](#shared-daemon)). |

The serve-mode flags above (and `--profile`, `--targets`, `--domain`, `--allow`, `--deny`) are rejected without
`--serve-skills`. The authoring tools are registered too, so one process serves both; `--read-only` registers
only the read-only tools, so a project can be inspected but never changed.

### Settings in the configuration

A project can pin how `mcp` runs in its `[mcp]` table, so every launcher (`.mcp.json`, a plugin, a teammate's
editor) does not repeat the flags. The keys are accepted in `.ai-rulez/config.toml`, a root `ai-rulez.toml`, the
`config.<name>.toml` layers and `config.local.toml`; a list in a later layer replaces the earlier one.

```toml
[mcp]
serve_skills = true                                               # --serve-skills (a bare `ai-rulez mcp` serves skills)
read_only = true                                                  # --read-only
tools     = ["find_skill", "load_skill", "find_context", "load_context"]   # --tools
allow     = ["review-*"]                                          # --allow (--serve-skills)
deny      = ["draft-*"]                                           # --deny (--serve-skills)
daemon    = true                                                  # --daemon (--serve-skills)
no_watch  = true                                                  # --no-watch (--serve-skills)
daemon_idle_timeout = "15m"                                       # how long the daemon outlives its last client
authoring = true                                                  # keep the authoring tools on a read-only skills server
serve_user_invoked_skills = true                                  # also serve skills that set disable-model-invocation
```

- **A small default surface.** `serve_skills` with `read_only` registers only `find_skill`, `load_skill`,
  `search_skills`, `read_skill_file` and `list_skill_resources`, and no prompts: the full tool list is about 92 KB of
  schema that a client sends on every request. Set `authoring = true` for the read-only authoring tools and the
  `review-config` prompt, or `tools = [...]` for an exact list.
- **User-only skills.** A skill that sets `disable-model-invocation: true` is Claude Code's user-only flag: the model
  must not see or invoke it, only the user, with a slash command. The server honours it. `find_skill`, `search_skills`,
  `list_skill_resources` and the `skill://` resources leave such a skill out, and `load_skill` / `get_skill` answer
  `<name> is user-invoked only (disable-model-invocation)`. `serve_user_invoked_skills = true` serves them anyway, for a
  client that has no slash commands and wants the model to reach them. `ai-rulez.lock` still pins them; only what the
  model sees changes. A server that serves skills and also carries the authoring tools (`mcp --serve-skills`, or
  `authoring = true`) applies the same rule to the tools that return or list skill content. A skill is held when its
  own `SKILL.md` sets the hyphenated key (the only spelling Claude Code honors), read from the file, so what the
  catalog admitted or filtered out does not matter. `read_skill` and `ai-rulez://skills/<name>` answer the same
  error, and `list_skills` leaves the skill out, as do the per-scope skill counts in the server instructions and the
  catalog `scopes` and `skill_discovery.served`. `create_skill` on a held name answers that the name is taken,
  without the file path. A user-only skill that skill discovery found in the repository is held by the directory the
  catalog recorded.
  The governance and audit tools (`catalog` and the `ai-rulez://catalog` resource, `list_roles`, `resolve_role`,
  `token_report`, `cost_report`, `sbom`, `lock_status`, `doctor`, `validate_config`, `scan_content`) still list a
  user-only skill by name, with its size, token count and lock state. They audit the whole configuration, and the
  lock pins every skill; a report that left one out would show a locked project as out of sync. They carry no
  content the model could invoke. The plain authoring server (`ai-rulez mcp` without
  `--serve-skills`) is exempt: it serves no skills to a model, it edits the configuration, and the flag only keeps
  a skill from the model's own invocation, so it still reads and lists every skill. A user-only skill on a serving
  server stays editable with `update_skill` and `delete_skill`, which never return its content. The setting is read
  when a server or daemon starts: restart a running `--daemon` (or let it idle out after `daemon_idle_timeout`)
  after changing it, because a daemon is keyed by its command line, not by the configuration it read.
- **Scope shadowing and directories.** A path query drops a skill that a nearer scope shadows by name, even when the
  nearer skill is itself hidden (user-only) or its `paths:` do not reach the path, so the farther skill of that name
  is not offered either. A directory query considers the skills whose globs could match a path below it, and does
  not look into descendant scopes (ask for `scope` with `recursive` for those).
- **Large repositories.** The live-reload watcher holds at most 2,048 descriptors (kqueue on macOS and the BSDs
  charges one per directory and per file in it), watching shallow directories first and warning once when the
  budget is reached; the poll covers the rest. `no_watch = true` turns the watcher off.
- **The daemon.** It outlives its client for `daemon_idle_timeout` (15 minutes by default) and exits at once when its
  repository is removed; it does not keep the checkout as its working directory. Its stderr goes to `daemon.log` beside
  the socket. When the cache path is too long for a unix socket, the socket goes under `$XDG_RUNTIME_DIR` or the temp
  directory in `ai-rulez-<uid>/`, used only when this user owns that directory and the one inside it with mode 0700.

- **Skills keys without serving.** `allow`, `deny`, `daemon`, `daemon_idle_timeout`, `authoring`, `serve_user_invoked_skills`, `no_watch`, `exclude`
  and `discover_skills` only apply when skills are served (`--serve-skills` or `serve_skills = true`). A server
  started without serving them logs one warning on stderr naming the keys that are set and ignored; the same settings
  as flags stop the server with `requires --serve-skills`, because a key may be shared by launches that do not serve
  skills.
- **A flag wins.** A flag given on the command line replaces the key, including `--read-only=false` and an empty
  `--tools=`. A flag that is not given leaves the key in charge.
- **Read at start.** The settings are read when the server starts; change them and restart the server. They are
  read from the project the server was started in (or the project above it), offline. A project that cannot be read is
  reported on stderr and the command line decides; a tool name that does not exist stops the server with the list of
  valid ones.
- **Policy only tightens.** An [organization policy](policy.md) with `[mcp] read_only = true` makes the server
  read-only whatever `[mcp] read_only` and `--read-only=false` say; a repository that sets `read_only = false` under
  it is reported as `AR740` and the server stays read-only.
- **The surface follows the tools.** The server instructions are written from the tools that are registered, and
  each authoring prompt (`add-rule`, `author-skill`, `review-config`, `trim-context`) is listed only when every
  tool it calls is. A read-only server therefore does not tell the model to use the CRUD tools or `generate_outputs`,
  and offers `review-config` alone.

### Discovering skills already in the repository

A repository often keeps skills that ai-rulez does not define: a `skills/` folder, `.claude/skills` in a package,
a folder a teammate set up for another harness. `--discover-skills` (or `[mcp] discover_skills`) serves them through
the same `skill://` resources, `find_skill` and `load_skill`, under the same security scan, session budget and
`--allow`/`--deny` rules, with no `generate` run and no copy.

```toml
[mcp]
discover_skills = "repo"   # "off" (default) | "repo" | "all"
```

```sh
ai-rulez mcp --serve-skills --discover-skills        # repo
ai-rulez mcp --serve-skills --discover-skills=all
```

| Mode | Serves |
| ---- | ------ |
| `off` | Today's behaviour: the skills ai-rulez defines and the skill sources. |
| `repo` | The above, plus every skill directory found in the working tree. |
| `all` | `repo`, plus the installed and included skills even when their delivery would keep them out of the served set. |

The flag wins over the configuration; a bare `--discover-skills` means `repo`. A project with no configuration at all
(or a [lint-only configuration](config-without-folder.md#lint-only-configuration)) can use it; without one it behaves
as if the `claude` and `codex` presets were configured, so `.claude/skills` and `.agents/skills` are searched.

**Where `repo` looks.** A directory with a `SKILL.md` below one of these roots, at any depth of the project and at
most six levels below the root:

- `skills/` (not inside a hidden directory such as `.agents/`, which belongs to its own convention);
- `.claude/skills`;
- the skill directory of each configured preset (`.agents/skills`, `.cursor/skills`, `.github/skills`, ...), the same
  list `convert` reads.

A `SKILL.md` inside another skill belongs to that skill and is not a skill of its own. The configuration directory
(`.ai-rulez/`), `.git/` and `node_modules/` are never searched, symlinks are not followed, and a skill is read with
the limits of a skill source (2 MiB a file, 8 MiB and 2000 files a skill, 500 skills in all).

**Ignored and generated files.** In a git repository the search covers the files `git ls-files --cached --others
--exclude-standard` lists, so everything `.gitignore` excludes is skipped. Without git it walks the tree and
only skips `.git` and `node_modules`. A skill directory ai-rulez itself wrote is left out, so a skill is not served
twice: one holding a file recorded in the generated manifest, or whose `SKILL.md` still carries the generated
`Content-Hash` header. While the manifest records a copy it stays out even when edited by hand; without the manifest
a copy whose body was edited fails the header check and is a repository skill.

**Names.** A skill is served under its frontmatter `name` (the directory name when it has none), and the first
claimant keeps a name in this order:

1. The skills ai-rulez defines (project, installed and included), then the skill sources: a repository skill with
   their name is *shadowed*, not served. A **builtin** skill is the exception: it ranks below repository skills, so a
   repository skill with its name (for example `.claude/skills/testing-conventions` beside `builtins = ["testing"]`)
   replaces it, as a skill under `.ai-rulez/skills/` does. The start-up log names the builtin that was replaced.
2. Repository skills, shallowest directory first, then by path. A later one with a name already taken is served
   under its path from the project root, `/` turned into `.` and the leading dot of each segment dropped:
   `pkg/.claude/skills/bar` becomes `pkg.claude.skills.bar` (its frontmatter `name` is rewritten to match).
   If that name is taken too, it is shadowed. Each rename is a start-up warning naming both directories and the served
   name, and is listed under `skill_discovery.renamed` (`name`, `served_as`, `path`, `kept_by`).

Every shadowed skill is a start-up warning and is listed, with its path and what shadows it, under
`skill_discovery.shadowed` in the `ai-rulez://catalog` resource, next to the served ones (`skill_discovery.served`)
and the count of generated directories left out (`generated_skipped`).

**Limits.** A discovered skill takes the default trust of a skill authored in the project (the scan blocks error
findings, warns on the rest). With `[lock] enforce`, it must be pinned like any served skill: set
`discover_skills` in the configuration, not only the flag, so `ai-rulez lock` sees it. The served catalog is not
cached while discovery is on, and live reload watches the skill roots found at start (and `skills/`, `.claude/skills`
and the preset directories at the project root when they exist): a new skill folder in an existing root is picked
up, a new `.claude/skills` elsewhere needs a restart.

**Without a configuration.** `--discover-skills` works in a directory with no `.ai-rulez/` or `ai-rulez.toml`. The
server then has no configuration to manage, so it registers only the tools that work without one (`find_skill`,
`load_skill`, `list_skill_resources`, `search_skills`, `get_skill`, `read_skill_file`, `find_context`,
`load_context`, `get_version`, `list_builtins`, `show_builtin`, `policy_show`, `okf_validate` and `init_project`)
and no authoring prompt. `ai-rulez://config` and the `ai-rulez://{kind}/{name}` item resources are not listed;
`ai-rulez://catalog` is, and is built from what was discovered: the served skills as `items`, the `scopes` with the
instruction files, rules and agents found, and the `skill_discovery` report.

### Monorepos: scopes and nested context

A harness loads a nested `AGENTS.md`, `CLAUDE.md`, `.claude/` or `.agents/` tree only once it works inside that
directory. A planner at the repository root never sees it. With discovery on (`--discover-skills` or
`[mcp] discover_skills = "repo" | "all"`), the server indexes the repository by **scope** and answers "what applies to
this path?" the way a harness would. With discovery off none of this exists: no extra tool, resource or instruction.

**Scopes.** A scope is a directory that holds `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.claude/`, `.agents/`,
`.codex/`, `.cursor/rules`, `.github/copilot-instructions.md` or a `skills/` directory with a skill in it. The project
root is the scope `.`. Scopes are inferred from the tree; nothing is configured. Each item in a scope has a `kind`
(`skill`, `instruction`, `rule`, `agent`, `command`), a `source` (`claude`, `agents`, `codex`, `cursor`, `copilot`,
`gemini`, `skills-dir`), a `name`, a `description` (the frontmatter `description`, else the first heading) and a content
`hash`. The index keeps summaries only, never a file body.

Instruction files, rules, agents and commands are indexed wherever a known convention keeps them (`.claude/`,
`.agents/`, `.cursor/`, `.github/`, ...): the files present decide, not the `presets` list, so a configuration with
`presets = []` (lint-only), `["claude"]` or no configuration at all serves the same context. Only the skill
directories follow the presets, as in [Where `repo` looks](#discovering-skills-already-in-the-repository).

**What applies to a path.** The scopes that are the path or one of its ancestors, nearest first. A skill, agent or
command is shadowed by one of the same name in a nearer scope, as harnesses shadow by name; instruction files and rules
add up, so none of them is shadowed. A skill name that two scopes share is served under its path from the project root
(`pkg/api/.claude/skills/lint` becomes `pkg.api.claude.skills.lint`), as in
[Discovering skills](#discovering-skills-already-in-the-repository).

**`find_skill` with `path` or `scope`.**

```json
{"task": "lint the code", "path": "pkg/api/handler.go"}
```

```json
{"task": "lint the code", "path": "pkg/api/handler.go",
 "scopes": ["pkg/api", "."],
 "results": [{"name": "pkg.api.claude.skills.lint", "scope": "pkg/api", "score": 1.4, "description": "Lint the api package"}]}
```

The root's own `lint` is shadowed for that path; for `pkg/web/index.ts` it is the one returned. A skill whose `paths:` frontmatter does not reach the path is left out (see [Rules and skills with `paths`](#rules-and-skills-with-paths)). `scope` lists one exact
scope, `"."` being the root; `recursive: true` adds the scopes below it. `path` and `scope` are mutually exclusive, and
`recursive` needs `scope`. A skill that ai-rulez defines or fetches belongs to no scope and applies everywhere. Each
result of a discovered skill names its `scope`. Without `path` and `scope` the call behaves as before.

**`find_context` and `load_context`.** Nested instruction files, rules and agents are served beside the skills.
`find_context` takes the same `path`, `scope` and `recursive`, plus an optional `task` to rank by name, scope, kind and
description (an item with no matching word is left out), `kind` (`instruction`, `rule` or `agent`) and `limit` (default
10, max 50). It returns summaries:

```json
{"path": "pkg/api/handler.go"}
```

```json
{"count": 4, "total": 4, "scopes": ["pkg/api", "."],
 "results": [
  {"name": "reviewer", "kind": "agent", "scope": "pkg/api", "source": "claude",
   "description": "Reviews changes to the api", "uri": "context://pkg/api/reviewer", "hash": "sha256:..."},
  {"name": "AGENTS.md", "kind": "instruction", "scope": "pkg/api", "source": "agents",
   "description": "API conventions", "uri": "context://pkg/api/AGENTS.md", "hash": "sha256:..."},
  {"name": "AGENTS.md", "kind": "instruction", "scope": ".", "source": "agents",
   "description": "Root conventions", "uri": "context://./AGENTS.md", "hash": "sha256:..."},
  {"name": "style", "kind": "rule", "scope": ".", "source": "claude",
   "description": "Code style for the whole repository", "uri": "context://./style", "hash": "sha256:..."}]}
```

**Rules and skills with `paths`.** A rule or a skill can name the files it applies to. With `path`, `find_context`
returns such a rule, and `find_skill` such a skill, only when the path matches one of its globs: `paths:` in the
frontmatter of a Claude Code rule or `SKILL.md` (Claude Code hides a path-gated skill until a matching file is read),
and `globs:` of a Cursor `.mdc` rule that is not `alwaysApply`. Cursor has no skills, so a skill's `globs:` is not read.
The globs are relative to the scope directory, the one that holds `.claude` (or `.cursor`), as in `ai-rulez validate`:
`pkg/api/.claude/rules/handlers.md` with `paths: "handlers/**"` applies to `pkg/api/handlers/user.go`, not to
`pkg/api/other/handlers/user.go`. `paths` globs are anchored at that directory (`*.py` is a file directly in it,
`**/*.py` any depth); Cursor `globs` also match a bare pattern such as `*.tsx` at any depth. A rule or skill without
either applies everywhere in its scope, and instruction files and agents are never narrowed. A `scope` query (no
`path`) lists every rule and skill of the scope whatever its globs, and loading one by name is unchanged. A rule's
result carries its `paths` or `globs`, so you can see what narrows it.

`path` may be a file or a directory. A directory keeps a rule or skill when some glob could match a path below it:
`backend/services/**/kafka*.py` is kept for `backend/services` and dropped for `products/web`, and `**/*.py` is kept for any
directory. A path that exists is a directory if the filesystem says so; one that does not exist is a file when its last
element has an extension. A restriction none of whose globs can be read does not hide the item.

`load_context` reads one, by `scope` and `name` or by `uri`, with an optional `budget_bytes`:

```json
{"scope": "pkg/api", "name": "AGENTS.md"}
```

It returns the text read **from the repository when you call**, never from the index, with the `hash` of what it read
(and `changed_since_index: true` when the file changed since start-up). The same text is the resource
`context://<scope>/<name>` (`context://./AGENTS.md` for the root), a resource template that `resources/list` does not
enumerate: find items with `find_context`. Commands are indexed and counted but not served as context.

The context tools apply what skills do: the `--allow` and `--deny` patterns match an item's name or `scope/name`
(`--deny AGENTS.md` hides every `AGENTS.md`; a non-empty `--allow` is an allowlist for context too); the security
scan runs when the index is built, so a file with an error finding is withheld (not listed, not loadable) and is a
start-up warning, and again on the content read at load time. The finding names the item by its `context://` URI and
line, for example `AR005 risky-shell-exec at context://./CLAUDE.md:2` (`skill://` stays for skills); the scan level
(`trust`) is a skill setting and does not change what happens to a file of context, since an error finding withholds it
and the lesser findings are logged while the file is served; reads count against the session budget (`--budget-bytes`), which `load_context`, `load_skill`,
`get_skill`, `read_skill_file` and `resources/read` share. A symlink is never followed and a generated copy is skipped.
Domains, roles, the lock and approvals apply to skills only.

**The `scopes` section of the catalog.** `ai-rulez://catalog` gains a `scopes` array: each scope with its per-kind
counts and sources, so a planner sees the shape of the repository in one read. The same query parameters narrow it:
`ai-rulez://catalog?path=pkg/api/handler.go`, `ai-rulez://catalog?scope=pkg/api` or
`ai-rulez://catalog?scope=.&recursive=true`.

```json
"scopes": [
  {"scope": ".", "sources": ["agents", "claude"], "counts": {"skill": 1, "instruction": 1, "rule": 1}},
  {"scope": "pkg/api", "sources": ["agents", "claude"], "counts": {"skill": 2, "instruction": 1, "agent": 1}}
]
```

The `skill_discovery.index` object of the same document reports `items`, `limit`, `found` and `truncated`.

**Server instructions.** The first eight scopes by depth are named in the server instructions with their counts
(`pkg/api (2 skills, 1 instruction, 1 agent)`), then "and M more", so a planner knows the nested content exists
without a call.

**Limits.** The index holds at most 20000 items. Past that it keeps the shallowest scopes and the server warns at
start-up; `skill_discovery.index.truncated` says so. Only files git lists (tracked, or untracked and not ignored; a
plain walk without git) are indexed, and an instruction file, rule or agent file over 1 MiB is skipped. `path`, `scope`
and `uri` are untrusted input: they must be relative to the project, free of backslashes and control characters, and
must not climb out with `..`; they select entries of the index and are never opened as paths. The index is built when the
server starts and rebuilt when the catalog reloads; the [file watcher](#live-refresh-of-the-index) picks a new
scope or instruction file up while the server runs, and `load_context` always shows the current text of a file the
index already knows.

#### The index cache

A cold build of the index is dominated by the security scan of every item. The index cache keeps what a start
computed so the next one does not: the summary of each indexed file (name, description and content hash, keyed by
path, size and modification time) and the scan verdict of content the scan found **nothing** in (keyed by a hash of
the scan level and of the names and content hashes of the scanned files). It never stores a file body.

- **Location.** `<user cache>/ai-rulez/monorepo-index/<repo-id>/index.json`, where `<repo-id>` is a hash of the
  canonical project root, so nothing is written into the repository. If the user cache is not writable the cache
  falls back to `.ai-rulez/.cache/monorepo-index/` (`.ai-rulez-cache/monorepo-index/` in the root layout), which is
  listed in `.git/info/exclude` so the working tree stays clean. `[mcp] index_cache = "user" | "repo" | "off"`
  chooses; `user` is the default and keeps the fallback, `repo` always uses the project directory, `off` keeps
  nothing. `ai-rulez cache status` and `ai-rulez doctor` report which location is in use, its size and its problems;
  `ai-rulez cache clear` drops it.
- **Freshness.** A file summary is used while its size and modification time are unchanged, the git tree state
  (`HEAD^{tree}`) is the one it was recorded in, and the file was not modified in the two seconds before the save
  (a file written in the same timestamp tick may have changed again). A new tree drops the summaries and keeps the
  verdicts, which are keyed by content. A verdict is dropped when the build of ai-rulez or the `[lint]` settings
  change.
- **Safety.** The index file is written with mode `0600` through write-and-rename, under a lock file so several
  servers on one repository share it and merge their entries. It carries an HMAC-SHA256 with a per-user secret kept
  outside the cache (`~/.config/ai-rulez/index-cache.key`, like the catalog cache's): a file that was edited,
  truncated, planted or written by another version fails its check and is discarded and rebuilt. A cache entry is
  a hint, never a decision: `--allow` and `--deny` run on every build, a verdict is only used for the exact bytes and
  trust level it was recorded for (a skill with any finding, even a warning, is scanned every time), and
  `load_context` scans the body it reads again on every call.
- **Cost.** `docs/performance.md` has the measured numbers; on the benchmark monorepo (1500 items over 500 scopes) a
  warm start takes about a tenth of a cold one.

#### Excluding paths

`--exclude <glob>` (repeatable) and `exclude = [...]` under `[mcp]` in `.ai-rulez/config.toml` (or the root
`ai-rulez.toml`) leave paths out of discovery:

```toml
[mcp]
discover_skills = "repo"
exclude = ["third_party/**", "**/fixtures/"]
```

```bash
ai-rulez mcp --serve-skills --discover-skills --exclude 'examples/**' --exclude '**/testdata/'
```

Globs are relative to the project root with slashes: `**` crosses directories, `*` and `?` stay within one, `{a,b}`
alternates, a pattern without a slash (`AGENTS.md`, `legacy`) matches at any depth, and a pattern naming a directory
(`third_party`, `third_party/`, `third_party/**`) excludes everything below it. A glob that leaves the project
(`../x`) or is empty is rejected. The flag's globs are added to the configuration's; neither replaces the other.

An excluded file is dropped from the file listing before anything is read, so it costs no parsing, scan or cache
entry. The globs apply to the skill directories and to the whole context index: `CLAUDE.md`, `AGENTS.md`,
`GEMINI.md`, Copilot instructions, rules, agents and commands. An excluded directory marks no scope. The file watcher
does not follow excluded paths either, so a change below one does not refresh the index. `--exclude` without
discovery is an error, as it would silently do nothing; the configuration key is ignored while discovery is off.

#### Live refresh of the index

While `--serve-skills --discover-skills` runs, a debounced file watcher (fsnotify, 300 ms) follows the project tree.
When an instruction file (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `copilot-instructions.md`), a `SKILL.md`, anything
under `.claude`, `.agents`, `.codex`, `.cursor`, `.github` or `skills`, or a directory appears, changes or goes, the
server rebuilds its catalog. The rebuild re-reads only the files whose size or modification time changed and scans
only content without a clean verdict, so it is incremental in cost. When the index changed the server re-registers
the `ai-rulez://index` resource with a new generation, which makes the SDK send one `resources/list_changed`; a client
then calls `find_context` or `find_skill` again. Reading `ai-rulez://index` returns the generation, the item and
scope counts and the cache report. `.git`, `node_modules`, `vendor`, `target`, `dist`, `build` and the configuration
directory (which the poll below covers) are not watched, a skipped change costs nothing, and `--no-watch` turns the
watcher off with the rest of live reload. If the operating system's limit on watched directories is reached the
server warns and the index refreshes on the poll interval only. The server instructions, written at start-up, are not
rewritten.

#### Writing a skill into a scope

`create_skill` and `update_skill` take `scope` (a project-relative directory), `allow_write` and, optionally,
`harness`. With `scope` the skill is written to `<scope>/<harness skills dir>/<name>/SKILL.md`, for example
`pkg/api/.claude/skills/lint/SKILL.md`, where a harness working in `pkg/api` loads it:

```json
{"scope": "pkg/api", "name": "lint", "description": "Lint the api package", "allow_write": true}
```

```json
{"success": true, "operation": "create_skill", "path": "pkg/api/.claude/skills/lint/SKILL.md", "name": "lint",
 "scope": "pkg/api", "harness": "claude", "message": "Skill written to ...", "lock": "ai-rulez.lock exists: ..."}
```

Rules, all checked before anything is created:

- `allow_write: true` is required on the call (default false), like `allow_llm` for `judge`.
- The scope must be an existing directory of the project (every scope the index knows is one). Neither it, nor any
  directory on the way to the skill, nor the skill directory or file may be a symlink; a path that leaves the
  project (`..`, an absolute path, a backslash) is refused; the write itself goes through an `os.Root` of the
  project, so a link planted after the checks cannot lead out.
- Only the skills directory of a preset the configuration enables is written: `harness` names it, else the first
  enabled preset that keeps skills in a directory (`claude` is `.claude/skills`, `codex` is `.agents/skills`).
- A generated copy is refused: a file listed in the generated manifest (for a skill, any file of its directory) or
  carrying a `Content-Hash` header that verifies. Edit the source under `.ai-rulez/` instead.
- The content is scanned as `install_skill` scans a skill, at the strictest trust level, and any finding refuses the
  call. With no `content`, `create_skill` builds a SKILL.md from `description`.
- `create_skill` refuses an existing skill; `update_skill` refuses a missing one. The file is written with mode
  `0644` through a temporary file and a rename, and calls on one server are serialized.
- A scoped write edits the harness file directly: it is not ai-rulez content and is not generated down. With an
  `ai-rulez.lock` present the result carries `lock`, because the new file is reported as drift until
  `ai-rulez lock` records it.

`scope` cannot be combined with `domain`, `priority`, `targets` or `local`. Instruction files and rules stay read-only
over MCP. The tools are annotated as writing and non-idempotent (`update_skill` replaces a file), so a read-only
server (`--read-only`) does not register them.

### Start-up cost and the catalog cache

Building the catalog renders every served skill and runs the security scan over it, which on a large
monorepo (hundreds of skills, each with reference files) takes several seconds. The result is a
function of the files the live reload fingerprints — the configuration directory, local includes, the
organization policy files and local skill sources — and of the view's options (`--profile`, `--role`,
`--targets`, `--domain`, `--allow`, `--deny`, `--include-static`, `--source`, `--frozen`, `--offline`),
so it is cached on disk keyed by exactly those:

- `~/.cache/ai-rulez/serve/<project-hash>/catalog.json` holds the rendered catalog; the project hash is
  a sha256 of the configuration directory, so two checkouts never collide.
- `~/.config/ai-rulez/serve-cache.key` (mode `0600`, created on first use) authenticates the entry.

A restart whose content and view are unchanged restores the catalog **without rendering or scanning
anything**; a content edit changes the fingerprint, so the entry is ignored and the catalog rebuilt.
An entry that is missing, truncated, edited, symlinked or fails its authentication counts as a miss
and is rebuilt, never served. Admission (approval expiry, signing key validity) is still re-judged on
the usual one-minute timer after a restore, so an entry written before an approval expired does not
serve that skill for longer than a fresh start would. The cache is skipped for a `lock` run's views and
for a build whose inputs the fingerprint cannot see (a programmatic load option or an injected clock),
and an entry over 64 MiB is not written; in those cases the server builds the catalog as before, only
slower.

#### Launchers and client timeouts

A cold start through `npx -y ai-rulez@latest` downloads the package (about 30 MB) before the process
even starts, and some clients time out at 30 s. Two things help:

- **Run the installed binary, not `npx`.** Install once (`npm i -g ai-rulez`, `uv tool install ai-rulez`,
  `go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest`, or a Homebrew tap) and point the
  MCP entry at `ai-rulez` directly. If you must use `npx`, pin the version (`npx -y ai-rulez@5.0.0`)
  so a cold start does not also resolve `latest`.

  ```json
  {
    "mcpServers": {
      "ai-rulez": {
        "command": "ai-rulez",
        "args": ["mcp", "--serve-skills"]
      }
    }
  }
  ```

- **Raise the client's connect timeout.** Claude Code reads `MCP_TIMEOUT` (milliseconds); `60000`
  covers a cold build on a large repository:

  ```bash
  export MCP_TIMEOUT=60000
  ```

The disk cache removes the repeated cost on every later start; the [shared daemon](#shared-daemon)
(`--daemon`) additionally removes the first build for every session after the first.

### Shared daemon

`ai-rulez mcp --serve-skills --daemon` makes one long-lived process per repository render the
catalog once and serve it to every session over a private unix socket. The first session spawns the
daemon (the same binary, detached in a new session, its standard streams on the null device) and
waits for the socket; every later session connects in milliseconds and is a thin proxy: it copies
its stdin/stdout to the daemon and nothing else. All sessions share the one rendered catalog, the
one search index and the one usage recorder, so the 6–8 s build of a large monorepo is paid once
rather than by each session and each subagent worktree.

```bash
ai-rulez mcp --serve-skills --daemon             # the sessions that share one catalog
```

- **Socket and key.** The daemon listens on `~/.cache/ai-rulez/daemons/<hash>/daemon.sock` (mode
  `0600`) inside a `0700` directory. The hash is a sha256 of the repository toplevel, the ai-rulez
  version, and the command line (the view — `--profile`/`--role`/`--targets`/`--include-static`,
  the `--domain`/`--allow`/`--deny` filters, `--source`, `--frozen`/`--offline`, the authoring
  `--root`/`--allow-any-dir`/`--read-only`, and the usage flags). A version change or a different
  MCP entry therefore starts and uses a different daemon, and two sessions in different
  subdirectories of one checkout share one; a subagent worktree, a separate checkout, has its own.
- **Content changes.** A content edit does not start a new daemon: the running one picks it up with
  its existing [live reload](#live-reload) (within the poll interval), exactly as an in-process
  server does. One daemon serves one repository through its edits, and exits when it is idle.
- **Idle timeout.** With no connected client for 15 minutes the daemon exits and removes its socket.
  A session that connects just after it leaves starts a new one; one that finds the socket gone
  serves in-process instead.
- **Security.** The socket lives in a `0700` directory the user owns and is `0600`. Before
  connecting, a proxy refuses a path that is not a socket, is owned by another user, or is readable
  or writable by group or others, and it never replaces such a socket with a daemon of its own; it
  serves in-process. The hello carries the protocol version and the serve view, and a daemon refuses
  a mismatched one, so a session is never served another view's skills.
- **Fallback.** On any failure — no daemon and none can be started, a socket that is missing,
  unsafe, stale or broken, a version or view mismatch, or a socket path over the unix limit — the
  process serves the session in-process, byte for byte as without `--daemon`. A broken or missing
  daemon never fails a session; the only cost is the build the daemon would have saved.
- **Windows.** The shared daemon is not offered on Windows: there is no unix-domain-socket permission
  model or detached-child session to reuse, so a session with `--daemon` serves in-process with a
  warning. Nothing else changes.
- **Limits.** The daemon is a stdio convenience: one catalog per repository, shared by the sessions
  that agree on the command line. It is not a network service and has no authentication beyond the
  socket's file permissions.

### Tools

| Tool | Arguments | Result |
| ---- | --------- | ------ |
| `find_skill` | `task` (required), `limit` (default 5, max 20), `role`, `path`, `scope`, `recursive` | Ranked matches: name, description, score, domain, digest. With a role, `in_role` and the role's skills first. With discovery, `path` or `scope` keeps the skills that apply there (see [Monorepos](#monorepos-scopes-and-nested-context)). |
| `load_skill` | `name` (required), `path`, `budget_bytes` | The file (`SKILL.md` by default), `provenance`, `digest`, an index of the skill's other files with their `mode`, and `files_access` (where the files live and how to use them) when the skill has any. |
| `list_skill_resources` | `name` (required), `offset` | Every file of the skill with path, URI, size, MIME type, digest, `mode`, `executable` and an `interpreter` hint, without loading them. |
| `find_context` | `task`, `path`, `scope`, `recursive`, `kind`, `limit` | With discovery only. Summaries of the nested instruction files, rules and agents that apply to a path or scope: name, kind, scope, source, description, `context://` URI, hash. |
| `load_context` | `scope` and `name`, or `uri`; `budget_bytes` | With discovery only. The text of one of them, read from the repository now, with its hash. |

`search_skills`, `get_skill` and `read_skill_file`, the `skill://` resources, `skills/list` and `skills/get` keep
working as described above. All tools are annotated read-only.

- **Ranking.** `find_skill` scores BM25 over four fields with weights name 3, triggers 2.5, keywords 2 and
  description 1, after lowercasing, dropping stopwords and a light stemmer (`migrations` matches `migration`).
  By default it is lexical and deterministic: score descending, then name. With `[search] mode = "hybrid"` (or
  `vector`) and an index built by [`ai-rulez search index`](search.md), the same ranker used by
  `ai-rulez search` ranks by cosine similarity over the index while it is fresh, and fuses it with that list by reciprocal rank fusion
  otherwise (`[search] fusion`). The
  server never builds the index and never embeds a skill: it loads the index files (and reloads them when they
  change) and embeds only the query, bounded by `query_timeout_ms` and an in-memory cache of 256 queries,
  through the `[llm]` network gate, budget and cache. Any failure (no index, an index of another model, the
  network disabled, a timeout, the budget) answers lexically and says why: the result gains `"ranking":
  "lexical"` and `"degraded": "no_index" | "provider_unavailable" | "timeout" | "budget" |
  "network_disabled"`. A hybrid result also carries `ranking`, and each match `lexical_rank` and
  `vector_rank` (0: not in that list); a skill edited since it was indexed ranks lexically only and has
  `stale_vector: true`. A server with the default lexical mode returns exactly the fields listed above.
  `search_skills` stays lexical: it is the listing and filter tool. With `[search] log_queries = true` in the user config (or `AI_RULEZ_SEARCH_LOG_QUERIES=1`) the
  server also records each `find_skill` query and the skill the session loads next (see
  [Skill search](search.md#query-mining)).
- **Roles.** `role` is resolved against the project's `[[roles]]` (`mcp.RolesFromConfig`): a skill is in scope
  when the role keeps it (its domains and `skills` include and exclude selectors, `extends` merged in). Matches
  inside the scope come first, then the rest marked `in_role: false`; an unknown role is an error. The `role`
  argument is a ranking lens the caller chooses, not access control: what a server can load is decided by
  `--role` at start, which builds the catalog without the skills outside the role (they cannot be found,
  loaded, listed or read by name). A skill that
  comes from a `[[skill_sources]]` entry is matched by name against the role's `skills` selectors. The role is
  read again on every live reload.
- **Budget.** Each session may read `--budget-bytes` bytes of skill content; `load_skill`, `get_skill`,
  `read_skill_file` and `resources/read` draw on the same budget (listing tools cost nothing, and a repeated
  read is charged again). A read that would exceed the
  remainder is refused and is not charged (`load_skill` reports the bytes left; `resources/read` fails with
  JSON-RPC error `-32600`); the server tracks at most 1024 sessions; `budget_bytes` on one call truncates that
  call's file (at a character boundary) and reports `truncated` and `total_bytes`. Supporting files count too.
- **Path.** `path` is relative to the skill. Absolute paths, `..`, backslashes and names that are not valid
  `skill://` path segments are rejected. A path with `.` or `..` segments that normalises to a file inside the
  skill (`references/../SKILL.md`) is accepted as that file; one that would leave the skill is rejected. The
  result's `uri` is the URI of the file loaded (`skill://<name>/<path>`). Binary files are read with `resources/read`.
- **Provenance.** Every result carries `provenance`: `digest` (served bytes), `lock_digest` (what the lock pins),
  `locked`, `source`, `ref`, `pinned`, `commit` for a source skill, `delivery`, and `scan_warnings`.

### Served files: scripts and assets

A served skill's files are held by the server; none is written to disk. To use a bundled script or asset a client
fetches it, writes it out and restores its mode.

- **Mode and interpreter.** `list_skill_resources`, the `resources` index of `load_skill`, `get_skill` and
  `read_skill_file` report per file `mode` (octal permission bits such as `"0755"`; `"0644"` when the source mode
  is unknown), `executable` (any execute bit) and, for a file with a shebang or a known extension (`.sh`, `.py`,
  `.js`, ...), an `interpreter` hint (the shebang line, or `sh`, `python3`, `node`, ...). `skills/list`,
  `skills/get` and `resources/list` carry the same under `_meta` as `io.modelcontextprotocol.skills/mode`,
  `.../executable` and `.../interpreter`; the existing `uri`, `digest` and `size` fields are unchanged, and the
  mode is not part of any digest or of `ai-rulez.lock`.
- **`files_access`.** `load_skill` adds this note when the skill has files besides `SKILL.md`: they are not on
  disk, fetch them with `read_skill_file`, restore the executable bit, or set `delivery = "both"` and run
  `ai-rulez generate`.
- **`read_skill_file(uri, encoding, offset, length)`.** `encoding` is `text` (default for text files) or `base64`
  (default for a binary file, which used to fail). A call returns at most 256 KiB, so a large file is read in
  chunks: `offset` and `length` are byte positions, the reply carries `size`, `offset`, `bytes` and, while more
  remains, `truncated` and `next_offset`. A text chunk never splits a UTF-8 character. Every returned byte is
  charged to the session budget like `resources/read`. Files the security scan cannot read are still refused
  where `AR989` refuses them today; nothing here serves them.
- **Recipe to materialise a skill.** For each entry of `list_skill_resources` (page with `next_offset`): create the
  parent directory under a directory you own, refusing any `path` that is absolute or contains `..`; read the
  file with `read_skill_file {uri, encoding: "base64", offset}` until `next_offset` is absent, concatenating the
  decoded chunks; write the bytes and `chmod` to `mode`.
  Resolve `SKILL.md` relative paths against that directory. A tool that does this inside the server
  (`materialize_skill`) is not provided: this server mode registers no tool that writes, and a safe writer needs a
  project root and an overwrite policy it does not have.

### Skill sources

A source is a repository (or a directory) of skill directories, served and never written to the static trees.

```toml
[[skill_sources]]
name = "acme"                                  # identifies the source in the lock
url = "https://github.com/acme/skills.git"     # git URL (https, ssh or file; a leading git+ is accepted) or a local directory
ref = "v1.2.0"                                 # a tag or a full commit SHA (or version = "^1.2", see below)
path = "skills"                                # subdirectory whose children are skills
include = ["pdf-*", "sql"]                     # directory-name globs; exclude wins
exclude = ["*-wip"]
name_prefix = "acme-"                          # served as acme-pdf-forms, and SKILL.md name is rewritten
trust = "error"                                # scan level: error (default) or warn
max_skills = 200                               # optional: skills the source may load (default 200)
max_bytes = 67108864                           # optional: bytes of skill files it may load (default 64 MiB)
max_clone_bytes = 268435456                    # optional: size limit of the git clone (default 256 MiB)
max_clone_files = 20000                        # optional: entries of the git clone (default 20000)
```

A source can ask for a version range instead of a ref: `version = "^1.2"` (also `tag_prefix`,
`include_prerelease`; `ref` and `version` are exclusive). The range is resolved against the repository's semver tags
by `ai-rulez lock` and moved only by `ai-rulez update`; serving uses the pinned commit and never resolves a range.
See [Version constraints](lockfile.md#version-constraints).

`--source` takes the same thing on the command line: `[git+]<url>[@<tag|commit>][#<subdir>]` or a directory,
for example `git+https://host/org/repo@v1.2.0#skills/`. The ref separator is the last `@` after the final `/`,
so `git@host:org/repo.git` and `https://user@host/...` keep their user info. The source is named
`cli-<repository>`. In a directory with no `.ai-rulez`, `--source` alone is enough to serve.

- **Pinning.** A tag is resolved to the commit it points at (an annotated tag is peeled), a full SHA is used as
  is. `ai-rulez lock` records the commit and the tree digest (the same `sha256` scheme as includes) as a `[[source]]`
  entry in `ai-rulez.lock`. With the lock, the pinned commit is fetched by its SHA even if the tag or branch has
  since moved, and the fetched tree must match the digest or serving fails. A server that refuses fetching a
  commit by SHA is asked for the ref with its history instead; if the pinned commit is no longer reachable from
  it (history was rewritten), the fetch fails closed and says so. The `commit` in a lock entry must be a full hex
  SHA.
- **Unpinned refs.** A branch, a tag the lock does not cover, or no ref, follows a moving ref; it is reported as
  unpinned (`AR010`, and a warning at serve time) until the lock covers it.
- **Transports and private repositories.** Git runs without hooks, credential helpers, prompts or submodules and
  only speaks `https`, `ssh` and `file` (never `ext::`; plain `http://` is not fetched). A `url` or `ref` that
  starts with `-` is rejected, as git would read it as an option. Credentials are not injected and the git
  credential helpers are off, so a private repository needs an `ssh` URL that works non-interactively (an
  ssh-agent key); a private `https` repository is not supported. Each git command is cut off after five minutes.
- **Limits.** A source may load at most `max_skills` skills (default 200) and `max_bytes` bytes of skill files
  (default 64 MiB); a larger source is an error naming the key. A skill has at most 2000 files.
- **Clone size.** A git source is fetched as a partial clone (`--filter`), and with a `path` only that
  subdirectory is checked out (a sparse checkout), so a large repository costs what the skills cost. The clone,
  git data and checkout together, is capped at `max_clone_bytes` (default 256 MiB; the `AI_RULEZ_MAX_CLONE_BYTES`
  environment variable, or `mcp --serve-skills --max-clone-bytes <n>`, sets it for every source that does not set its
  own; the flag wins over the environment variable, and a source's own `max_clone_bytes` wins over both). The size is watched while git runs:
  a clone that grows past the limit is stopped, fails with an error naming `max_clone_bytes` and the source
  (`skillsource.ErrCloneTooLarge`), and leaves nothing in the cache. Set `path`, or raise the limit.
- **Cache integrity.** A source the lock covers is checked against the lock's digest. A source whose `ref` is a full
  commit SHA but which the lock does not cover is checked against a digest recorded next to the cached tree
  (`<tree>.digest`, written atomically, outside the tree so it never changes the tree digest, readable only by you).
  If the tree no longer matches, or has no record (a cache written by an older release), it is fetched again;
  with `--offline`/`--frozen` there is no fetch, so serving fails with a "cache damaged" error until you run once
  online. The record catches damage and edits of the tree; it is not a defence against someone who can also
  rewrite the record, which is what the lock is for.
- **Cache and network.** Trees are cached per commit under `~/.cache/ai-rulez/skill-sources/`. `--frozen` requires
  the lock to cover the source and never touches the network (the commit must be cached). `--offline` does not
  require the lock: it uses the lock's commit, or the commit an earlier online run recorded for that ref.
- **Local directories** need no network; a lock entry pins their digest, and serving refuses a changed tree. A
  local `url`/`path` declared in the project's config must resolve, after symlinks, inside the project (a relative
  one is relative to the project root); a committed config cannot point the server at another user's skill
  directory. `--source <dir>` on the command line and sources in your user config may point anywhere.
- **Safety.** Symlinks below the source are never followed; a `path` of a git source that goes through a
  symlink is refused, and a local source directory that is itself a symlink is resolved and digested through
  the link. A file over 2 MiB is dropped with a warning, a skill over 8 MiB is skipped with a warning. A source
  skill is served under its directory name (with `name_prefix`), whatever its `name:` says (SKILL.md is
  rewritten), and a skill whose name collides with a project skill (served or static) or with an earlier
  source's skill is skipped (set `name_prefix`); the project's skill wins.
- **Symlinked skills.** A skill directory that is a symlink out of the project is dropped by the server with a
  warning, while `validate` fails on it: fix the link rather than relying on the warning.

### Security scan

Every served skill is scanned before it is served, with the rules of `ai-rulez scan` (secrets,
hidden characters, prompt-injection phrases, risky shell, unrestricted `allowed-tools`, encoded blobs) over
`SKILL.md` and every text file of at most 512 KiB. A file the scan cannot read (binary: a NUL byte or invalid
UTF-8; or over 512 KiB) is reported as `AR989`. At `trust = "error"` (remote sources, installed skills) such a
file is not served at all (`load_skill`, `resources/read` and the file list omit it; provenance lists it under
`unserved_unscannable_files`, and the lock digest still covers it), and a `SKILL.md` that cannot be scanned
refuses the skill. At `trust = "warn"` (skills authored in the project) the file is served with the warning.
A skill that fails is not served: it is absent from `resources/list`,
`skills/list` and `find_skill`, a warning names the finding on stderr, and `load_skill` says why. Inline
`ai-rulez-lint-ignore` comments are not honored. `validate --strict` runs this same scan over the served
skills, authored and from sources alike, and reports `AR989` for each file it cannot read (see the table below).
The level is `trust` for a source skill:

| Level | Blocks |
| ----- | ------ |
| `error` | Any finding (every finding counts as an error). Default for source skills and for skills installed from a git repository (`lint.security.scan_imports = "warn"` lowers installed skills to `warn`). |
| `warn` | Findings that are errors by their own severity (secrets, hidden characters, risky shell). Default for skills authored in the project. |

Skills that come from an `[[includes]]` entry are remote content: they are scanned at `error` like installed skills
(`scan_imports = "warn"` lowers them to `warn`), as is any skill file outside the project root. This holds wherever
the include lives, including a local include inside the project (`source = "./shared"`). Their lock `source`
is `include:<name>/<path>`, identical on every machine.

### Lock enforcement

`ai-rulez lock` also records, for every skill the server would serve, a `[[served]]` entry with its name and
lock digest (and a `[[source]]` entry per skill source). With

```toml
[lock]
enforce = true   # the default whenever ai-rulez.lock exists; false opts out
```

the server refuses a served skill whose digest differs from the lock, and one the lock does not pin, with
`AR995`. `validate --strict` reports the same. The lock digest is a `sha256:` tree digest in the same scheme as
the other pins ([Lock file](lockfile.md#served-skills-and-skill-sources), domain `ai-rulez/served-skill/v1`). It
covers the rendered files but not the generated header lines that change without the skill changing (a whole
`Source-Hash: <algorithm>:<hex>` or `Content-Hash: <algorithm>:<hex>` line and the `Generated:` date stamp, in the first 40 lines of a rendered
file), so editing one skill does not invalidate the others. Files of a skill source are digested exactly as
they are: nothing in them is ignored. The digest is of
the rendering the view selects: the default preset's, or another preset's with `--targets` (a view of its own, see
below). Skills that only a role serves are pinned too: `lock` builds the unscoped view and the view of every
role. `ai-rulez lock --kind served|source` refreshes one kind; `lock --check` verifies both without the network.

`[[served]]` entries are recorded per serve view: the way the server is started selects a set of skills, and
the lock pins each set under a `view` key (see [Lock file](lockfile.md#served-skills-and-skill-sources)). A plain
`lock` pins the default view (the configured `[[skill_sources]]`, the default profile and preset, the delivery
rules), every role, and every view the lock already records. To pin another view, give `lock` the flags the
server runs with:

```bash
ai-rulez lock --role backend                       # skills the backend role serves
ai-rulez lock --profile team --include-static      # profile view that also serves static skills
ai-rulez lock --targets cursor                     # skills as the cursor preset renders them
ai-rulez lock --source git+https://host/org/skills@v1.2.0#skills   # a command-line source, pinned with its commit
ai-rulez mcp --serve-skills --role backend --frozen
```

`mcp --serve-skills` and `lock --check` read the pins of the view they run with, so a server started with
`--role`, `--profile`, `--targets`, `--include-static` or `--source` finds its skills pinned once `lock` has pinned that view;
otherwise every skill it adds is refused with `AR995`. A lock written before views existed (no `view` keys)
still covers every view, as long as the digests match. `--targets` is part of the view (`targets:<preset>`): another
preset's rendering has its own pins, and `lock --targets <preset>` pins it.

A skill the security scan refuses is not pinned. By default `lock` reports it, pins every other skill, writes the
lock and exits 3; `validate --strict` lists the refused skills of skill sources with their scan code (`AR0xx`). With
`lock --refuse-findings` any refusal stops `lock` from writing the lock. Either way a refused skill has to be fixed or
excluded before it can be served under enforcement.

### Usage telemetry

Each successful `load_skill` goes through the usage recorder as one identifier-only JSON line: time, skill name,
salted session hash (a stdio connection has no transport session id, so the server gives each connection a random
one; a new connection gets a new hash and a new byte budget, the same pipe keeps both; the salt file is created next
to the log on first use), harness (the MCP client name), the role the server runs under, `outcome: "loaded"`, content
hash from the skills index, the served digest, and `served: true` (log format `v: 3`, the same as hook-recorded
loads).
A supporting file loaded with `path` is logged with `resource: true` and is not counted again by
`ai-rulez telemetry report`. Nothing is written until you opt in: pass `--usage-log <file>` or `--usage-sink <command>`,
or enable `[usage] skills_index = true`, which logs to `<config dir>/local/usage.jsonl`.

A `--usage-sink` receives exactly the line the log gets, salted session included. Without a `--usage-log` the salt
lives in `<config dir>/local/usage.salt`. The sink command runs in the background: records wait in a queue of 256,
so a slow or hanging sink never delays `load_skill`. A full queue drops the new record and logs a warning with the
number dropped. Queued records are delivered at shutdown (end of input, SIGTERM or Ctrl-C), waiting up to
three seconds.

### Live reload

The server checks the configuration directory, local include directories (and `local_override`), the local
files of the organization policy, and local source directories every two seconds. A file counts as changed
when its size, modification time or (up to 1 MiB) its bytes change, so `cp -p` and `rsync -t` are seen. When a
file changes it rebuilds the catalog and swaps it in. `notifications/resources/list_changed` is sent only when the
catalog changed (a skill added, removed or edited); an edit that leaves every served skill identical sends nothing. A rebuild that
fails keeps the previous catalog serving and is retried with a growing pause (up to a minute) until it works or
a new edit changes the files (a new edit ends the pause at once). A rebuild that finds the `SKILL.md` of a
served skill newly empty or with an unclosed frontmatter (an editor's save in progress) counts as failed, so
the previous version keeps serving until the save completes. The usage log and its salt file (`--usage-log`
and `usage.salt` beside it, compared by absolute path) and any `.jsonl` file directly inside a `local/` directory,
at any depth, do not count as changes. Polling keeps the dependency set unchanged; git sources are immutable
per commit and are not re-fetched.

Approval expiry and signing key validity depend on the clock, not on files: every minute (also with
`--no-watch`) the server judges the current build again and stops serving a skill whose approval expired or
whose signature no longer verifies. An organization policy whose `sources.deny_digests` names the pinned
digest of a served skill refuses that skill (AR747); the server keeps serving the others. A skill authored in
the project is also gated as its `skill:<name>` item: a denied authored digest refuses it (AR717), and
`require_approval` selectors that select the item (`all`, `local`, `kind:skill`) gate serving it under
`enforce`.

### Strict validation

| Code | Severity | Meaning |
| ---- | -------- | ------- |
| `AR990` | warning | Static content names a served skill |
| `AR991` | error | A harness that can call MCP has no `dynamic-skills` stub |
| `AR992` | warning | A harness without MCP keeps served skills as static files |
| `AR993` | warning | Skills are served but no `[[mcp_servers]]` entry runs `--serve-skills` |
| `AR994` | error | `delivery` frontmatter is not static, served or both |
| `AR995` | error | `[lock] enforce` and a served skill is unpinned or its digest differs |
| `AR989` | warning / error | A served file cannot be scanned (binary or over 512 KiB). An error for `SKILL.md` at any trust level (the server refuses the skill). For a supporting file: a warning at `trust = "warn"` (served with the warning) and, at `trust = "error"`, a finding that names the unserved files |

See [Strict validation](strict-validation.md) for the full table.

## Typical Workflow

### With Your Editor

1. **Edit files** directly in `.ai-rulez/`:

   ```bash
   # Edit rules, context, skills, or config.toml
   vim .ai-rulez/rules/code-quality.md
   ```

2. **Use MCP server** (via AI assistant) to generate:

   ```bash
   ai-rulez generate
   ```

3. **Commit changes**:

   ```bash
   git add .ai-rulez/   # plus generated files only if gitignore = false
   git commit -m "docs: update AI guidelines"
   ```

### With Claude CLI

If using the Claude CLI with the ai-rulez MCP server, you can ask Claude to help:

- "Review my `.ai-rulez/` configuration and suggest improvements"
- "Generate outputs for my new rules"
- "Validate that my profiles are correct"
- "Create a new backend domain and add database rules"
- "Add an include from our shared rules repository"

The server provides Claude with access to read and modify your configuration, while you maintain full control over final decisions.

---

## MCP Tools Reference

The ai-rulez MCP server exposes 66 tools for programmatic configuration management. These tools allow AI assistants to initialize projects, generate and clean outputs, validate configuration, inspect builtins, and create, read, update, and delete configuration elements.

Every tool below accepts an optional `working_directory` parameter (the directory to operate in),
except the two utility tools `get_version` and `show_builtin`. The per-tool parameter lists below
name only the parameters specific to that tool unless noted.

### Local configuration (`local: true`)

Tools that change content or config accept an optional `local` boolean that redirects the change to
the machine-local layer instead of the shared one. See [Local Configuration](local-overrides.md).

| Tools | `local: true` targets |
| ----- | --------------------- |
| `create_*`, `read_*`, `update_*`, `delete_*`, `list_*` for rule, context and skill | The `.ai-rulez/local/` content tree (`domain` selects `local/domains/<name>/`) |
| `add_include`, `remove_include`, `install_skill`, `uninstall_skill`, `update_config`, `add_profile`, `remove_profile`, `set_default_profile` | The `config.local.*` overlay |

Not accepted: `create_domain`, `delete_domain`, `list_domains`, `list_includes`, `list_profiles`,
`list_installed_skills`. Those list tools show the shared layer only.

The drift guard applies to `generate_outputs` too. `--allow-local-drift` is CLI-only: the MCP server
cannot write overlay-derived values over tracked shared files. Pass `no_local: true` to generate the
shared view, or fix the reported paths.

### Tool metadata, arguments and results

**Metadata.** Every tool has a human `title` and annotations: `readOnlyHint`, `destructiveHint` and `idempotentHint`
describe what it writes, and `openWorldHint` is `false` on all of them (they act on the local project tree only).
Read-only tools (`list_*`, `read_*`, `validate_config`, `scan_content`, `doctor`, `run_verifiers`, `get_version`,
`show_builtin`, the report tools, the governance tools and the skills-serving tools) never write.

**Arguments.** Input schemas are inferred from typed argument structs and enforced before a handler runs: an
unknown argument, a wrong type (`"local": "yes"`), a value outside an enum or a missing required argument is an
error result. Every schema sets `additionalProperties: false`.

**Results.** Every tool declares an `outputSchema` and returns `structuredContent` (the parsed result) together with
the same JSON as a text block, for clients that only read text. Output schemas list the stable fields only and allow
others, so a result can gain a field without breaking validation.

**Errors.** A failure is an error result (`isError: true`), never a success. That covers a configuration that does not
load (every `list_*` and `read_*` tool, as `ai-rulez list` exits 1), an invalid configuration, findings that reach
`fail_on`, a `lock_status` call on a project without a lock, `doctor` when the configuration cannot be read, and
arguments that fail validation. The error text carries the CLI's hint; errors that are reports (`validate_config`,
`doctor`) also return the report as `structuredContent`.

**Confinement.** `working_directory`, `config_file` and `config_dir` must lie inside the directory the server was
started in. A call without `working_directory` uses that directory; a relative one resolves against it; symlinks are
followed before the check. Start the server with `mcp --root <dir>` to confine it elsewhere, or with
`mcp --allow-any-dir` to lift the check. The skills-serving mode takes no directory argument.

**Project root.** Like the CLI, every tool except `init_project` walks up from `working_directory` to the nearest
directory that holds a configuration in any layout, stopping at the git root and never above the home directory. A
call that passes `config_file` or `config_dir` is not walked up.

**Capabilities.** The authoring server advertises `tools`, `prompts` and `resources`, none with `listChanged` (their
sets are fixed), pages lists at 100 entries, and sends the SDK's warnings and errors to stderr. Its lists carry a
cache hint (`ttlMs` five minutes, `public`); resource reads are never cacheable, since the project can change between
two reads. The skills-serving server's hints are `private`, one minute, and zero under live reload.

**Progress and cancellation.** Long calls (`generate_outputs` with `recursive`, `token_report` over several targets)
send `notifications/progress` when the request carries a progress token, and stop when the client cancels the request.

### Response keys

Items in the list tools are the file records of the CRUD layer, so their keys are capitalised (`Name`, `Path`),
unlike the lower-case envelope keys (`success`, `operation`, `count`).

### Project and Utility Tools

#### `generate_outputs`

Generate output files from the current configuration.

**Parameters:**

- `config_file` (optional, string): Path to the root configuration file
- `config_dir` (optional, string): Configuration directory name (default: `.ai-rulez`)
- `dry_run` (optional, boolean): Preview changes without writing files
- `check` (optional, boolean): Write nothing and report the generated files that differ from what the sources render (`generate --check`). The result is `{"status", "roots", "blocked", "differing": [{"kind", "path", "note"?}]}`, and drift is an error result, as the command exits 2
- `offline` (optional, boolean): Never use the network; includes and installed skills come from the lock and the cache (`--offline`)
- `profile` (optional, string): Profile to generate, or a comma-separated list to compose several (`--profile`)
- `role` (optional, string): Generate the slice of content a role selects (`--role`); mutually exclusive with `profile`
- `recursive` (optional, boolean): Generate for all subdirectories containing `.ai-rulez/`
- `exclude` (optional, array of strings): With `recursive`, directories never searched for configuration, as globs or paths relative to the working directory (`--exclude`); adds to `recursive_exclude`
- `no_local` (optional, boolean): Ignore the machine-local `config.local.*` overlay and `local/` content (the view a teammate sees)
- `no_self_mcp` (optional, boolean): Do not add ai-rulez's own MCP server to the generated MCP files (`--no-self-mcp`; the default is to add it)
- `working_directory` (optional, string): Directory to operate in

A call that supports progress (the client sends a progress token) reports one notification per project in a
`recursive` run, and a cancelled call stops before the next project.

#### `clean_outputs`

Remove the files produced by `generate_outputs` (its inverse): the generated assistant files, the generated manifest (and local manifest with the local rule files it records), and the ai-rulez managed `.gitignore` block. The `.ai-rulez/` source tree is never touched, and generated directories are removed only once empty.

**Parameters:**

- `config_file` (optional, string): Path to the root configuration file
- `config_dir` (optional, string): Configuration directory name (default: `.ai-rulez`)
- `dry_run` (optional, boolean): Preview what would be removed without deleting
- `keep_gitignore` (optional, boolean): Leave the ai-rulez managed block in `.gitignore`
- `keep_manifest` (optional, boolean): Leave the generated manifest in place
- `working_directory` (optional, string): Directory to operate in

#### `validate_config`

Validate the configuration with the same checks as [`ai-rulez validate`](cli.md): the configuration file against its
schema, structural validation (including all includes), the organization policy, the `config.local.*` overlay against
the local schema (`schema/ai-rules-local.schema.json`), and then every lint analyzer, with the baseline and ratchets
the CLI applies. The tool and the command share one code path, so they report the same findings against the same
threshold. Errors are redacted: URL credentials and quoted source values are removed, since the merged config can
contain local secrets.

The result is `{"valid", "fail_on", "config", "warnings", "errors", "roots", "findings", "summary", "risk"}`:
`warnings` and `errors` are the findings of that severity as `file:line: CODE message`, and `findings`, `summary` and
`risk` are the document of `validate --format json`. An invalid configuration, or findings at or above `fail_on`,
is an error result (`isError: true`) with the same document in `structuredContent`; findings below the threshold are
returned as warnings in a success result.

**Parameters:**

- `config_file` (optional, string): Path to the root configuration file
- `config_dir` (optional, string): Configuration directory name (default: `.ai-rulez`)
- `no_local` (optional, boolean): Validate without the machine-local overlay
- `fail_on` (optional, string): Lowest severity that fails the call: `error` (default; or the `[lint]` setting), `warning`, `info` or `none` (`--fail-on`)
- `strict` (optional, boolean): Fail on warnings too, the same as `fail_on: "warning"` (`--strict`)
- `config_only` (optional, boolean): Check the configuration only and skip the content checks (`--config-only`)
- `lint_profile` (optional, string): `default`, `strict` or `permissive` (`--lint-profile`)
- `analyzers` (optional, string[]): Run only these analyzers (`--analyzer`)
- `working_directory` (optional, string): Directory to operate in

#### `init_project`

Initialize a new ai-rulez project in the current directory with the layout `ai-rulez init` creates:
`.ai-rulez/config.toml` plus the `rules/`, `context/`, `skills/`, `agents/` and `domains/` directories. It refuses
to overwrite an existing configuration. It initializes exactly the given directory and does not walk up, but refuses
when a directory above it in the same repository already holds a configuration (the message names that file).

**Parameters:**

- `project_name` (optional, string): Project name
- `providers` (optional, array): Providers to enable, such as `claude` or `cursor`
- `with_agents` (optional, boolean): Also create a sample agent, `agents/code-reviewer.md`. `agents/` itself is always created.
- `all_providers` (optional, boolean): Enable the curated set of tool presets
- `popular_providers` (optional, boolean): Same curated set — a shortcut for `all_providers`
- `source` (optional, string): `ai-rulez` (default) or `agents`. With `agents` it writes `.agents/ai-rulez.toml` and a starter `AGENTS.md` (when there is none) instead of the `.ai-rulez/` layout; see [.agents as a source](agents-source.md)
- `working_directory` (optional, string): Directory to operate in

#### `get_version`

Return the ai-rulez version.

**Parameters:** (none)

#### `show_builtin`

Show the full content of a builtin domain.

**Parameters:**

- `name` (required, string): Builtin domain name, such as `security`, `rust`, or `typescript`

### Domain Tools

#### `create_domain`

Create a new domain with subdirectories for rules, context, and skills.

**Parameters:**

- `name` (required, string): Domain name (alphanumeric and underscores, 1-50 characters)
- `description` (optional, string): Description of the domain

**Response:**

```json
{
  "success": true,
  "operation": "create_domain",
  "name": "backend",
  "path": ".ai-rulez/domains/backend",
  "message": "Domain created successfully"
}
```

**Example:**

```text
Create a domain called "backend" for backend services
```

#### `delete_domain`

Delete a domain and all its contents.

**Parameters:**

- `name` (required, string): Domain name to delete

**Response:**

```json
{
  "success": true,
  "operation": "delete_domain",
  "name": "backend",
  "message": "Domain deleted successfully"
}
```

#### `list_domains`

List all domains in the `.ai-rulez/` directory.

**Parameters:** (none)

**Response:**

```json
{
  "success": true,
  "operation": "list_domains",
  "domains": [
    {
      "Name": "backend",
      "Path": ".ai-rulez/domains/backend",
      "Description": "Backend services"
    }
  ],
  "count": 1
}
```

### Rule Tools

#### `create_rule`

Create a new rule file with optional YAML frontmatter.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `content` (optional, string): Markdown content with optional YAML frontmatter
- `domain` (optional, string): Domain name (if not specified, creates in root)
- `priority` (optional, string): Priority level - critical, high, medium, low, minimal. Default: medium
- `targets` (optional, array): Target providers (e.g., ["claude", "cursor"])
- `local` (optional, boolean): Create in `.ai-rulez/local/` (gitignored) instead of the shared tree

**Response:**

```json
{
  "success": true,
  "operation": "create_rule",
  "path": ".ai-rulez/rules/code-quality.md",
  "name": "code-quality",
  "domain": "",
  "message": "Rule created successfully"
}
```

#### `update_rule`

Update an existing rule file.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `content` (required, string): New markdown content
- `domain` (optional, string): Domain name
- `priority` (optional, string): Priority level
- `targets` (optional, array): Target providers
- `local` (optional, boolean): Update the file in `.ai-rulez/local/`

**Response:** Same as create_rule

#### `read_rule`

Read a rule file.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Read from `.ai-rulez/local/`
- `working_directory` (optional, string): Directory to operate in

#### `delete_rule`

Delete a rule file.

**Parameters:**

- `name` (required, string): Rule filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Delete from `.ai-rulez/local/`

**Response:**

```json
{
  "success": true,
  "operation": "delete_rule",
  "name": "code-quality",
  "domain": "",
  "message": "Rule deleted successfully"
}
```

#### `list_rules`

List all rules in the root or a specific domain.

**Parameters:**

- `domain` (optional, string): Domain name (lists root rules if not specified)
- `local` (optional, boolean): List `.ai-rulez/local/` instead of the shared tree

**Response:**

```json
{
  "success": true,
  "operation": "list_rules",
  "domain": "",
  "rules": [
    {
      "Name": "code-quality",
      "Path": ".ai-rulez/rules/code-quality.md",
      "Type": "rules",
      "Domain": "",
      "Priority": "high",
      "Targets": ["claude", "cursor"]
    }
  ],
  "count": 1
}
```

### Context Tools

#### `create_context`

Create a new context file (documentation/reference material).

**Parameters:**

- `name` (required, string): Context filename without .md extension
- `content` (optional, string): Markdown content with optional YAML frontmatter
- `domain` (optional, string): Domain name
- `priority` (optional, string): Priority level
- `targets` (optional, array): Target providers
- `local` (optional, boolean): Create in `.ai-rulez/local/` (gitignored)

**Response:** Similar to create_rule

#### `update_context`

Update an existing context file.

**Parameters:** Same as create_context, with content as required

**Response:** Similar to create_rule

With `source = "agents"` and `agents_md_split = true`, an item of `AGENTS.md` (`agents`, `agents-<heading slug>`) is
updated in place: `content` replaces that one section, heading line included, atomically, and content that would add,
drop or rename a `##` heading is refused. The unsplit item `agents` is the whole file and is refused.

#### `read_context`

Read a context file.

**Parameters:**

- `name` (required, string): Context filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Read from `.ai-rulez/local/`
- `working_directory` (optional, string): Directory to operate in

For an item of `AGENTS.md` (`source = "agents"`) the result is that item only: its section with
`agents_md_split = true`, the whole file for the unsplit item `agents`. `path` is `AGENTS.md`.

#### `delete_context`

Delete a context file.

**Parameters:**

- `name` (required, string): Context filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Delete from `.ai-rulez/local/`

**Response:** Similar to delete_rule

#### `list_context`

List all context files in the root or a specific domain.

**Parameters:**

- `domain` (optional, string): Domain name
- `local` (optional, boolean): List `.ai-rulez/local/` instead of the shared tree

**Response:** Similar to list_rules (items use the capitalised `Name`, `Path`, `Type`, `Domain`, `Priority`, `Targets` keys), with a "context" key instead of "rules"

### Skill Tools

#### `create_skill`

Create a new skill file (AI prompt/expert definition).

**Parameters:**

- `name` (required, string): Skill filename without .md extension
- `content` (optional, string): Markdown content with optional YAML frontmatter
- `domain` (optional, string): Domain name
- `priority` (optional, string): Priority level
- `targets` (optional, array): Target providers
- `local` (optional, boolean): Create in `.ai-rulez/local/` (gitignored)
- `description` (optional, string): Frontmatter description, used when `content` is empty
- `scope` (optional, string): Write the skill into the repository at `<scope>/<harness skills dir>/<name>/SKILL.md` instead of editing ai-rulez content; needs `allow_write`
- `harness` (optional, string): With `scope`, the enabled preset whose skills directory is written
- `allow_write` (optional, boolean, default false): With `scope`, agree to write a harness file into the repository

**Response:** Similar to create_rule. With `scope` it carries `path` (project-relative), `scope`, `harness` and, when `ai-rulez.lock` exists, `lock`. See [Writing a skill into a scope](#writing-a-skill-into-a-scope).

#### `update_skill`

Update an existing skill file.

**Parameters:** Same as create_skill, with content as required (`scope`, `harness` and `allow_write` replace the harness's skill file)

**Response:** Similar to create_rule

#### `read_skill`

Read a skill file.

**Parameters:**

- `name` (required, string): Skill filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Read from `.ai-rulez/local/`
- `working_directory` (optional, string): Directory to operate in

#### `delete_skill`

Delete a skill file.

**Parameters:**

- `name` (required, string): Skill filename without .md extension
- `domain` (optional, string): Domain name
- `local` (optional, boolean): Delete from `.ai-rulez/local/`

**Response:** Similar to delete_rule

#### `list_skills`

List all skill files in the root or a specific domain.

**Parameters:**

- `domain` (optional, string): Domain name
- `local` (optional, boolean): List `.ai-rulez/local/` instead of the shared tree

**Response:** Similar to list_rules (items use the capitalised `Name`, `Path`, `Type`, `Domain`, `Priority`, `Targets` keys), with a "skills" key instead of "rules"

### Check Tools

Checks are code-review guidelines (see [Checks](checks.md)). They live in the shared tree only: there is no `local` argument.

| Tool           | Arguments                                                                                          |
| -------------- | -------------------------------------------------------------------------------------------------- |
| `create_check` | `name`, `content`, `description`, `severity` (low\|medium\|high\|critical), `tools`, `targets`, `domain` |
| `read_check`   | `name`, `domain`                                                                                   |
| `update_check` | `name`, `domain`, and `content` and/or any of `description`, `severity`, `tools`, `targets`         |
| `delete_check` | `name`, `domain`                                                                                   |
| `list_checks`  | `domain`                                                                                           |

Names are limited to `[A-Za-z0-9._-]`. Content without frontmatter gets one built from the structured arguments (marshalled as YAML, `severity` and `targets` validated). `update_check` rejects a call with neither `content` nor a field; content without frontmatter replaces the body and keeps the existing frontmatter, and fields are set on it.

### Include Tools

#### `add_include`

Add a new include source (git URL or local path) to the configuration.

**Parameters:**

- `name` (required, string): Include name (unique identifier)
- `source` (required, string): Git URL (`https://github.com/org/repo`) or local path (`./packages/shared`)
- `path` (optional, string): Path within git repository where .ai-rulez/ content is located
- `ref` (optional, string): Git reference - branch, tag, or commit hash (git sources only). Defaults to the repository's default branch (`HEAD`)
- `include` (optional, array): Content types to include - rules, context, skills, agents, commands
- `merge_strategy` (optional, string): Merge strategy - local-override (default), include-override, error
- `install_to` (optional, string): Installation target path in .ai-rulez/
- `local` (optional, boolean): Add the include to the `config.local.*` overlay instead of the shared config

A `source` URL that carries a credential in its userinfo (`https://user:token@host/...`) is refused, like the CLI: it
would be written in clear into `config.toml`. Use the git credential helper or the `AI_RULEZ_GIT_TOKEN` environment
variable; a committed credential is reported by `validate`/`scan` as `AR035`.

**Response:**

```json
{
  "success": true,
  "operation": "add_include",
  "name": "corporate-rules",
  "source": "https://github.com/myorg/shared-rules",
  "message": "Include added successfully"
}
```

#### `remove_include`

Remove an include source from the configuration.

**Parameters:**

- `name` (required, string): Include name to remove
- `local` (optional, boolean): Remove through the overlay; an include from the shared config is hidden with `remove = true`

**Response:**

```json
{
  "success": true,
  "operation": "remove_include",
  "name": "corporate-rules",
  "message": "Include removed successfully"
}
```

#### `list_includes`

List all include sources in the configuration.

**Parameters:** (none)

**Response:**

```json
{
  "success": true,
  "operation": "list_includes",
  "includes": [
    {
      "Name": "corporate-rules",
      "Source": "https://github.com/myorg/shared-rules",
      "Type": "git"
    }
  ],
  "count": 1
}
```

### Installed Skill Tools

#### `install_skill`

Install a named skill from a git repository or local path.

**Parameters:**

- `name` (required, string): Skill name (unique identifier)
- `source` (required, string): Git URL or local filesystem path
- `path` (optional, string): Path within repo to skill directory (defaults to `skills/<name>`)
- `ref` (optional, string): Git reference (branch, tag, commit)
- `include`, `exclude` (optional, string arrays): Globs of skill directory names to install, or skip, from a directory of skills; `exclude` wins
- `name_prefix` (optional, string): Prefix added to every installed skill name and output directory
- `trust` (optional, string): Scan level of the installed skills, `error` (default) or `warn`
- `resources` (optional, string array): File patterns to bundle with each skill; a leading `!` removes
- `version` (optional, string): Version range of the source's tags, such as `^1.2`, instead of `ref`
- `tag_prefix` (optional, string), `include_prerelease` (optional, boolean), `min_release_age` (optional, string): tag filter and release age gate for `version`
- `domain` (optional, string), `profiles` (optional, string array): Scope the skills to a domain or to profiles
- `local_override` (optional, string): Directory used instead of the source while it exists
- `local` (optional, boolean): Record the skill in the `config.local.*` overlay instead of the shared config

The arguments are the `installed_skills` fields of the same names, and `skill install` has a flag for each.

A `source` URL that carries a credential in its userinfo (`https://user:token@host/...`) is refused, like the CLI: it
would be written in clear into `config.toml`. Use the git credential helper or the `AI_RULEZ_GIT_TOKEN` environment
variable; a committed credential is reported by `validate`/`scan` as `AR035`.

**Response:**

```json
{
  "success": true,
  "operation": "install_skill",
  "name": "kreuzberg",
  "source": "https://github.com/kreuzberg-dev/kreuzberg",
  "message": "Skill installed successfully"
}
```

#### `uninstall_skill`

Remove an installed skill from the configuration.

**Parameters:**

- `name` (required, string): Skill name to remove
- `local` (optional, boolean): Remove through the overlay; a skill from the shared config is hidden with `remove = true`

**Response:**

```json
{
  "success": true,
  "operation": "uninstall_skill",
  "name": "kreuzberg",
  "message": "Skill uninstalled successfully"
}
```

#### `list_installed_skills`

List all installed skills.

**Parameters:** None

**Response:**

```json
{
  "success": true,
  "operation": "list_installed_skills",
  "installed_skills": [
    {
      "Name": "kreuzberg",
      "Source": "https://github.com/kreuzberg-dev/kreuzberg",
      "Path": "skills/kreuzberg",
      "Ref": "",
      "Type": "git"
    }
  ],
  "count": 1
}
```

### Config Tools

#### `read_config`

Read the current project configuration as structured JSON. The result is the shared view (as if loaded
with `--no-local`), so a read-modify-write loop never copies local values into the shared config.

**Parameters:**

- `working_directory` (optional, string): Directory to operate in

**Response keys:** `name`, `description`, `presets`, `profiles`, `builtins`, `includes`, `gitignore`,
`default_effort`, `default_effort_by_preset`, `rules_mode` and `rules_mode_by_preset`. The last four are
always present, empty when unset. When a `config.local.*` overlay exists, `local_overlay` reports
`{ "path": "...", "keys": ["presets", "mcp_servers.github.env.GITHUB_TOKEN", ...] }`: key paths only,
never values.

#### `update_config`

Update supported project configuration fields.

**Parameters:**

- `name` (optional, string): Project name
- `description` (optional, string): Project description
- `builtins` (optional, array): Builtin names to enable
- `gitignore` (optional, boolean): Whether generation updates `.gitignore`
- `default_effort` (optional, string): Default reasoning effort
- `default_effort_by_preset` (optional, object): Per-preset reasoning effort overrides
- `rules_mode` (optional, string): Default rules output mode, `split` or `inline`; empty string clears it
- `rules_mode_by_preset` (optional, object): Per-preset rules mode overrides. The map replaces the existing one; an entry with value `""` removes that preset's override, and `{}` or `null` clears them all
- `local` (optional, boolean): Write the supplied fields to the `config.local.*` overlay instead of the shared config. Clearing a field (empty string or empty map, including `name` and `description`) removes that key from the overlay instead of writing an empty value
- `working_directory` (optional, string): Directory to operate in

### Profile Tools

#### `add_profile`

Create a new profile with a set of domains.

**Parameters:**

- `name` (required, string): Profile name (unique identifier)
- `domains` (required, array): List of domain names to include in the profile
- `local` (optional, boolean): Define the profile in the `config.local.*` overlay; it may reference local domains

**Response:**

```json
{
  "success": true,
  "operation": "add_profile",
  "name": "full",
  "domains": ["backend", "frontend", "qa"],
  "message": "Profile added successfully"
}
```

#### `remove_profile`

Remove a profile from the configuration.

**Parameters:**

- `name` (required, string): Profile name to remove
- `local` (optional, boolean): Remove a profile defined in the overlay; a shared profile cannot be removed locally (error)

**Response:**

```json
{
  "success": true,
  "operation": "remove_profile",
  "name": "staging",
  "message": "Profile removed successfully"
}
```

#### `set_default_profile`

Set a profile as the default for generation.

**Parameters:**

- `name` (required, string): Profile name to set as default
- `local` (optional, boolean): Set `default` in the overlay instead of the shared config

**Response:**

```json
{
  "success": true,
  "operation": "set_default_profile",
  "name": "full",
  "message": "Default profile set successfully"
}
```

#### `list_profiles`

List all profiles in the configuration.

**Parameters:** (none)

**Response:**

```json
{
  "success": true,
  "operation": "list_profiles",
  "profiles": [
    {
      "Name": "full",
      "Domains": ["backend", "frontend", "qa"],
      "IsDefault": true
    },
    {
      "Name": "backend",
      "Domains": ["backend", "qa"],
      "IsDefault": false
    }
  ],
  "count": 2
}
```

---

### Agent and Command Tools

Agents and commands are flat markdown files whose frontmatter carries their own settings, so these tools take no
`priority` or `targets`. They mirror `add|remove|list agent|command`.

| Tool | Does | Parameters |
| ---- | ---- | ---------- |
| `create_agent`, `create_command` | Create the file; a template is written when `content` is empty | `name` (required), `description`, `content`, `domain`, `local` |
| `read_agent`, `read_command` | Read the content | `name` (required), `domain`, `local` |
| `update_agent`, `update_command` | Replace the content as given, atomically | `name` and `content` (required), `domain`, `local` |
| `delete_agent`, `delete_command` | Delete the file | `name` (required), `domain`, `local` |
| `list_agents`, `list_commands` | List the files of the root or a domain | `domain`, `local` |

### Report Tools

Read-only twins of commands. Each returns the document the command prints with `--format json`, built by the same
library code; none writes, and none uses the network (remote content comes from the lock and the cache). A gate the
command turns into exit 2 (an exceeded budget, a finding at `fail_on`) is an error result that still carries the
document in `structuredContent`.

| Tool | Command | Parameters beyond `working_directory`, `config_file`, `config_dir`, `no_local` |
| ---- | ------- | -------- |
| `scan_content` | `scan` | `fail_on`, `lint_profile`. The security checks only: secrets, hidden text, injection phrases, risky shell, unpinned sources. The result is the document of `validate_config` |
| `token_report` | `tokens` | `profile`, `role`, `by_role`, `compare_profiles`, `tokenizer`, `budget`. One target answers with the report, several with `{"schema_version", "items"}` |
| `cost_report` | `cost` | `profile`, `target`, `top`, `tokenizer`, `budget`, `on_demand_budget` |
| `sbom` | `sbom` | `type` (`cyclonedx`, `spdx-json`), `files`, `profile`, `role`, `include_outputs`, `no_approvals`. The reproducible document, without the machine-local overlay; writing (`-o`), `--check`, `--verify` and the CI gates stay on the command line |
| `okf_validate` | `okf validate` | `bundle` (required, a directory under the project), `fail_on`. A git URL is refused |
| `approvals_status` | `approve --list` | `all`. What needs approval and its status. Approving, revoking and signing are human decisions and stay on the command line |
| `policy_show` | `validate --show-policy` | The policy layers, the merged policy, where each key came from and what the configuration tried to loosen |
| `list_verifiers` | `verifiers list` | The declared verifiers |
| `list_builtins` | `builtins list` | The builtin domains and whether each is auto-included |

### Prompts

| Prompt | Arguments | Expands to |
| ------ | --------- | ---------- |
| `author-skill` | `name`, `purpose` | Draft a skill, create it, then check it with `scan_content`, `validate_config` and `cost_report` |
| `add-rule` | `name`, `guideline`, `priority` | Check for an existing rule, then create or update one and validate |
| `review-config` | `focus` | Run the read-only checks and summarize what to fix, changing nothing |
| `trim-context` | `budget` | Find the biggest always-loaded items and propose edits before applying them |

### Resources

| URI | Content |
| --- | ------- |
| `ai-rulez://config` | The parsed settings (`read_config`) |
| `ai-rulez://catalog` | The content catalog (`catalog`); with `--serve-skills --discover-skills` also `skill_discovery` and a `scopes` section, narrowed by `?path=`, `?scope=` and `&recursive=true` (see [Monorepos](#monorepos-scopes-and-nested-context)) |
| `context://{scope}/{name}` | With discovery: one nested instruction file, rule or agent, read from the repository on each read |
| `ai-rulez://{kind}/{name}` | The markdown of one root item; `kind` is `rules`, `context`, `skills`, `checks`, `agents` or `commands` |
| `ai-rulez://domains/{domain}/{kind}/{name}` | The same for an item of a domain |

Resources are read-only, confined like the tools to the directory the server was started in, and an unknown URI is a
resource-not-found error.

### Governance Tools

Four read-only tools expose the roles, lock and catalog surface of the CLI. They never write a file and never use
the network: remote includes and installed skills resolve from the local cache only, and one that is not cached is
left out (the result then carries a second text block saying so). Each returns the JSON of the matching CLI command
with `--format json`, compacted onto one line, so a client can parse either source the same way. They are annotated
read-only and registered on the authoring server only (not on `--serve-skills`). Mutating lock operations
(`lock`, `update`) stay CLI-only.

All four take the common parameters `config_file`, `config_dir`, `no_local` and `working_directory`
(`lock_status` always ignores the machine-local overlay, as `lock --check` does).

| Tool | CLI equivalent | Parameters |
| --- | --- | --- |
| `list_roles` | `roles list --format json` | common only |
| `resolve_role` | `roles resolve <role> --format json` | `role` (required), `limit` |
| `lock_status` | `lock --check --format json` | `kind`, `profile`, `role`, `targets`, `include_static`, `sources` |
| `catalog` | `catalog --format json` | `kind`, `role`, `limit` |

**Output limits.** `resolve_role` and `catalog` list at most `limit` items (default 200, maximum 1000; a smaller
value wins). When the list is cut the document gains `"truncated": true` and `"total_items": <matching items>`;
`totals` and `roles` still describe every item. `list_roles` returns one entry per declared role, and
`lock_status` one change per differing item; neither is capped.

#### `resolve_role`

What a person holding a role gets: the document of [`roles resolve`](roles.md), with `role.items` capped at `limit`.

| Parameter | Type | Description |
| --- | --- | --- |
| `role` | string | Role name, as listed by `list_roles` (required) |
| `limit` | number | Maximum items returned |

#### `lock_status`

The comparison of `ai-rulez.lock` with the authored sources, the generated outputs, the skill sources and the served
skills ([`lock --check`](lockfile.md)), following `schema/lock-diff.schema.json`. A project that was never locked
is an error result ("nothing to check", with the hint to run `ai-rulez lock`), as `lock --check` exits 1; under
`[lock] enforce = true` the missing lock is reported as drift instead.

| Parameter | Type | Description |
| --- | --- | --- |
| `kind` | string | List only the changes of one kind: `include`, `skill`, `source`, `served`, or `content` (authored items and generated outputs). `in_sync` still covers the whole lock |
| `profile` | string | Profile whose outputs are compared (default: the profile recorded in the lock); also selects the serve view |
| `role` | string | Compare only this role's outputs and check the skills it serves as a view of their own (`lock --role`) |
| `targets` | string | Also check the view that serves this preset's rendering of the skills (`lock --targets`) |
| `include_static` | boolean | Also check the view that serves static skills too (`lock --include-static`) |
| `sources` | string[] | Also check the view with these extra skill sources (`lock --source`) |

#### `catalog`

Every rule, skill, agent, command and context file with owner, version, tokens, digest, roles and the lock status
(see [`catalog`](roles.md#integrating-an-identity-tool-or-ui)).

| Parameter | Type | Description |
| --- | --- | --- |
| `kind` | string | Only items of this kind: `rule`, `skill`, `agent`, `command`, `check` or `context` |
| `role` | string | Only the items this role keeps; `roles` shrinks to that role |
| `limit` | number | Maximum items returned |

---

## Common MCP Workflows

### Creating a Domain with Rules

```text
User: "Create a backend domain and add a database standards rule"

MCP Tool Sequence:
1. create_domain(name: "backend", description: "Backend services")
2. create_rule(name: "database-standards", domain: "backend", priority: "high", content: "...")
3. generate_outputs() - regenerate configurations
```

### Adding an External Include

```text
User: "Add our corporate rules from GitHub"

MCP Tool Sequence:
1. add_include(name: "corporate", source: "https://github.com/myorg/rules", ref: "main")
2. validate_config() - check if include is valid
3. generate_outputs() - regenerate with included content
```

### Setting Up Team Profiles

```text
User: "Create backend and frontend profiles for our team separation"

MCP Tool Sequence:
1. create_domain(name: "backend")
2. create_domain(name: "frontend")
3. add_profile(name: "backend", domains: ["backend"])
4. add_profile(name: "frontend", domains: ["frontend"])
5. set_default_profile(name: "backend")
6. generate_outputs()
```

### Bulk Content Creation

```text
User: "Add security rules to the backend domain"

MCP Tool Sequence:
1. create_rule(name: "authentication", domain: "backend", content: "...")
2. create_rule(name: "encryption", domain: "backend", content: "...")
3. create_context(name: "security-architecture", domain: "backend", content: "...")
4. validate_config()
5. generate_outputs()
```
