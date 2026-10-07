# Agent Plugins

ai-rulez packages a project's skills and MCP servers as an [Agent Plugins](https://agent-plugins.org) plugin directory, so
any client that conforms to the standard can load them. The same code builds the package, validates it and reads it back
(`internal/agentplugins`); it checks `plugin.json` and `mcp.json` against the official JSON Schemas, which are vendored and
pinned for each supported version.

| Spec | Status | `$schema` of `plugin.json` |
| --- | --- | --- |
| 1.0.0 | published, the default | `https://agent-plugins.org/schemas/1.0.0/plugin.schema.json` |
| 1.1.0 | working draft | `https://agent-plugins.org/schemas/1.1.0/plugin.schema.json` |

`mcp.json` carries the matching `mcp.schema.json` identifier. The two files always name the same version.

## Enable it

```toml
[plugin]
name = "acme.tools"
version = "1.2.0"
description = "Acme review tooling."
runtimes = ["agent-plugins"]
spec = "1.1.0"          # optional; 1.0.0 when unset
```

```bash
ai-rulez generate --plugin     # writes plugin.json, skills/ and mcp.json at the project root
ai-rulez verify --plugin       # fails when a generated file was edited or no longer matches its sources
ai-rulez validate --strict     # AR9O0-AR9O5 for anything a conformant client would skip
```

The `copilot` runtime and the `codex` runtime with `manifest = "root"` write the same package, so they follow `spec` too.

## Field mapping

| ai-rulez | Package | Notes |
| --- | --- | --- |
| `[plugin] name` | `plugin.json` `name` | 1-64 characters from `a-z0-9.-`, starting and ending alphanumeric, no `--` or `..` |
| `[plugin] version`, `description`, `homepage`, `repository`, `license`, `keywords` | the same keys | |
| `[plugin.author]` `name`, `email`, `url` | `author` | an empty author is omitted |
| `[plugin] spec` | `$schema` of `plugin.json` and `mcp.json` | |
| `[plugin.interface]` (codex root layout) | `extensions["com.openai"].interface` | keys are written sorted |
| skills (`.ai-rulez/skills/<name>/`) | `skills/<name>/SKILL.md`, with `scripts/`, `references/` and other files | `SKILL.md` is copied byte for byte; only the immediate children of `skills/` are read by clients |
| `[[mcp_servers]]` or `[[plugin.mcp]]`, stdio | `mcp.json` `mcpServers.<name>` with `type = "stdio"`, `command`, `args`, `env` | see [MCP servers](#mcp-servers) |
| `[[mcp_servers]]` or `[[plugin.mcp]]`, `transport = "http"` | `type = "streamable-http"`, `url` | `url` must be HTTPS, or HTTP to localhost |
| `transport = "sse"` | `type = "sse"`, `url` | only when configured explicitly |
| `enabled = false` | not packaged | the format has no flag; reported as `AR9O4` |
| `headers` | `headers` (remote servers) | plugin bundles do not carry project server headers today |
| agents, commands, hooks | not written | they belong in a client's extension namespace, see [Extensions](#extensions) |
| rules, context | not written | the `io.github.goldziher.ai-rulez` namespace is read on import |

## MCP servers

The specification expands only `${PLUGIN_ROOT}` and `${PLUGIN_DATA}`, and only in `args`, `env` values and `cwd`.

- `command` is one token: a bare name (`npx`) or a `./` path inside the plugin. A `${PLUGIN_ROOT}/bin/server` command is
  rewritten to `./bin/server` (a lossless rewrite, reported as info).
- An `env` entry that only forwards a variable of the same name (`API_KEY = "${API_KEY}"`) is left out: no client
  expands it, so the client has to supply `API_KEY` itself. This is reported as `AR9O3` (warning).
- Any other `${VAR}` (in `args`, `env`, `url`, `headers`) is rejected: the server is not packaged and the finding is an
  error (`AR9O3`). `generate --plugin` logs it, `validate --strict` fails on it and `publish` stops.
- `PLUGIN_ROOT` and `PLUGIN_DATA` may not be set in `env`: the client supplies them.

## Extensions

Content that only one client understands lives under a reverse-domain namespace, either as manifest data
(`plugin.json` `extensions.<namespace>`) or as files (`<namespace>/`). Clients ignore namespaces they do not implement.

| Namespace | Content |
| --- | --- |
| `com.anthropic.claude-code` | `agents/<name>.md`, `commands/<name>.md`, `hooks/hooks.json` |
| `com.openai` | the Codex `interface` block (manifest data) |
| `io.github.goldziher.ai-rulez` | `rules/<name>.md`, `context/<name>.md` |
| `com.github.copilot` | `agents/<name>.agent.md` (the `copilot` runtime) |

`generate --plugin` writes the Codex block and the Copilot agents; the other namespaces are read on import and written by
the library when a caller supplies them.

## Determinism and verification

The package is sorted, has no timestamps and is rendered with two-space JSON and a trailing newline. Every generated file
is recorded with its `Content-Hash` in `.ai-rulez-generated.json`, so `generate --check` and `verify --plugin` cover
`plugin.json`, `mcp.json` and the skills, and `publish` pins the whole bundle in its manifest and `SHA256SUMS`.

## Validation and the AR9O codes

`validate --strict` builds the package in memory and validates it as a client loads it. `publish` runs the same check on
the bundle it packages, in `--dry-run` as well, and fails with the first error's code and exit status 2.

| Code | Meaning |
| --- | --- |
| `AR9O0` | `plugin.json` is missing or invalid, the spec is unsupported, or an extension namespace is invalid |
| `AR9O1` | a skill breaks the Agent Skills rules (name, description, frontmatter) or is not at `skills/<name>/SKILL.md` |
| `AR9O2` | `mcp.json` or a server entry fails the schema or the rules for `command`, `url` and headers |
| `AR9O3` | an unsupported `${VAR}` placeholder |
| `AR9O4` | a field or server that cannot be packaged was left out |
| `AR9O5` | a path or link that resolves outside the plugin root, or a file that cannot be read |

`validate --explain AR9O3` prints the rule. Only single-plugin projects are checked by `validate --strict`; the members of
a marketplace are checked when they are published.

## Publish

The specification defines no archive, registry or signature. ai-rulez reuses its [publish](publish.md) pipeline: the
bundle with the `agent-plugins` runtime travels in the release archive, the npm package or the OCI artifact, and is signed
with the rest of the release (`--sign-key`, `--sign-keyless`). These distribution steps sit outside the specification.

`--emit agent-plugins` (or `[[publish.emitters]] name = "agent-plugins"`) writes each plugin as a clean directory,
`emit/agent-plugins/<name>/`, holding only the Agent Plugins files (no `.claude-plugin/`, no other runtime). The directory
is read back and written again by the library, so anything a client would skip is reported rather than shipped. The option
`spec` converts the package to another version: `options = { spec = "1.1.0" }`.

## Import

```bash
ai-rulez convert --from agent-plugins ./some-plugin --write
```

`convert` reads an Agent Plugins directory into `.ai-rulez/`: `plugin.json` becomes the `[plugin]` block (with
`runtimes = ["agent-plugins"]` and `spec` when it is not 1.0.0), skills keep their `scripts/` and `references/`, `mcp.json`
becomes `[[mcp_servers]]`, and the Claude Code, Copilot and ai-rulez namespaces become agents, commands and rules. The
report lists what has no equivalent: bundled root files, unknown namespaces, `hooks/hooks.json` and a server `cwd`. A
manifest without `version` or `description` cannot form a `[plugin]` block, so none is written and the report says so. The
provenance header ai-rulez writes into generated Markdown is removed on import.

Export, import and export again give identical bytes; a test pins this for 1.0.0 and 1.1.0.

## Differences from earlier output

The `agent-plugins` runtime used to write the manifest without checking it. Building with the library changes the output
only where the specification requires:

- A skill without a `description`, or whose `name` differs from its directory, is not packaged (clients skip it).
- An MCP server with a `${VAR}` placeholder other than `${PLUGIN_ROOT}` and `${PLUGIN_DATA}` is not packaged.
- A disabled server is not packaged (it used to be bundled enabled).
- A `env` entry `KEY = "${KEY}"` is left out.
- The Codex `interface` block is written with sorted keys.
