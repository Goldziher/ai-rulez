---
title: "Agent Plugins"
description: "The agent-plugins plugin runtime packages your skills and MCP servers as an Agent Plugins directory: a root plugin.json, a skills/ tree and an mcp.json, which any client that implements the standard can load."
---

The `agent-plugins` plugin runtime packages your skills and MCP servers as an [Agent Plugins](https://agent-plugins.org)
directory: a root `plugin.json`, a `skills/` tree and an `mcp.json`, which any client that implements the standard can
load. ai-rulez validates the package against the official JSON Schemas of the spec version you choose, and drops (and
reports) anything a conformant client would skip.

| Spec | Status | `$schema` of `plugin.json` |
| --- | --- | --- |
| `1.0.0` | published; the default | `https://agent-plugins.org/schemas/1.0.0/plugin.schema.json` |
| `1.1.0` | working draft | `https://agent-plugins.org/schemas/1.1.0/plugin.schema.json` |

`mcp.json` carries the matching `mcp.schema.json`. The `copilot` runtime and the `codex` runtime with
`[plugin.codex] manifest = "root"` write the same package and follow the same `spec` (see
[Authoring plugins](/ai-rulez/plugins/#runtime-specific-settings)).

## Example

```toml
# .ai-rulez/config.toml
version = "5.0"
name = "acme-tools"

[plugin]
name = "acme-tools"
version = "1.2.0"
description = "Code review skills and an MCP server for Acme repositories."
license = "MIT"
repository = "https://github.com/acme/acme-tools"
keywords = ["review", "mcp"]
runtimes = ["agent-plugins"]
include = ["scripts/**"]          # ship .ai-rulez/scripts/ in the package
# spec = "1.1.0"                  # optional; 1.0.0 when unset

[plugin.author]
name = "Acme Tooling"
email = "tooling@acme.example"

[[plugin.mcp]]
name = "acme-docs"
command = "${PLUGIN_ROOT}/scripts/mcp-launch.sh"
args = ["serve"]
env = { ACME_TOKEN = "${ACME_TOKEN}" }
```

The source tree holds `.ai-rulez/skills/review-pr/` (a `SKILL.md` and `scripts/lint.sh`) and
`.ai-rulez/scripts/mcp-launch.sh`.

```bash
ai-rulez generate --plugin
```

```text
WARN  Agent Plugins: env ACME_TOKEN = "${ACME_TOKEN}" is dropped: Agent Plugins expands no environment placeholder, so the client must supply ACME_TOKEN itself ...
INFO  Plugin generation complete files=...
```

The package is written at the project root:

```text
plugin.json
mcp.json
skills/review-pr/SKILL.md
skills/review-pr/scripts/lint.sh
scripts/mcp-launch.sh
.ai-rulez-generated.json
```

`plugin.json`:

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "acme-tools",
  "version": "1.2.0",
  "description": "Code review skills and an MCP server for Acme repositories.",
  "author": {
    "name": "Acme Tooling",
    "email": "tooling@acme.example"
  },
  "repository": "https://github.com/acme/acme-tools",
  "license": "MIT",
  "keywords": ["review", "mcp"]
}
```

`mcp.json`:

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers": {
    "acme-docs": {
      "type": "stdio",
      "command": "./scripts/mcp-launch.sh",
      "args": ["serve"]
    }
  }
}
```

The `${PLUGIN_ROOT}/` command became `./`, and the `ACME_TOKEN` forward was dropped: the client must provide that
variable itself. The drop is reported as `AR9O3`, so `validate --strict` exits 2 until you remove the `env` line.

Check the package:

```bash
ai-rulez verify --plugin      # fails when a generated file was edited or no longer matches its sources
ai-rulez validate --strict    # AR9O0-AR9O5 for anything a conformant client would skip
```

## Field mapping

| ai-rulez | Package | Notes |
| --- | --- | --- |
| `[plugin] name` | `plugin.json` `name` | 1-64 characters from `a-z0-9.-`, starting and ending alphanumeric, no `--` or `..` |
| `[plugin] version`, `description`, `homepage`, `repository`, `license`, `keywords` | the same keys | `version` may come from `version_from` |
| `[plugin.author]` `name`, `email`, `url` | `author` | omitted when empty |
| `[plugin] spec` | `$schema` of `plugin.json` and `mcp.json` | |
| `[plugin.interface]` (codex root layout only) | `extensions["com.openai"].interface` | keys sorted |
| skills (`.ai-rulez/skills/<name>/`) | `skills/<name>/`, with `SKILL.md`, `scripts/`, `references/` and other files | a skill without a `description`, or whose `name` differs from its directory, is not packaged |
| `[[plugin.mcp]]`, or `[[mcp_servers]]` when the plugin declares none | `mcp.json` `mcpServers.<name>` | see [MCP servers](#mcp-servers) |
| `enabled = false` on a server | not packaged | the format has no disabled flag; reported as `AR9O4` |
| `[plugin] include` | the matched files, at their relative path | see [Bundling files](#bundling-files) |
| agents, commands, hooks | not written | outside the standard; see [Extensions](#extensions) |
| rules, context | not written | read back on [import](#import) only |

## MCP servers

| Server | `mcp.json` entry |
| --- | --- |
| stdio (`command`) | `type = "stdio"`, `command`, `args`, `env` |
| `transport = "http"` | `type = "streamable-http"`, `url`; the URL must be HTTPS, or HTTP to localhost |
| `transport = "sse"` | `type = "sse"`, `url` |

Clients expand only `${PLUGIN_ROOT}` and `${PLUGIN_DATA}`, and only in `args`, `env` values and `cwd`. ai-rulez
applies these rules:

- `command` is one token: a bare name (`npx`) or a `./` path inside the package. `${PLUGIN_ROOT}/bin/server` is
  rewritten to `./bin/server`. A `./` path that is not in the package is a warning (`AR9O2`); ship the file with
  [`include`](#bundling-files).
- An `env` entry that only forwards a variable of the same name (`API_KEY = "${API_KEY}"`) is left out, with a
  warning (`AR9O3`). The client has to supply `API_KEY`.
- Any other `${VAR}` in `args`, `env`, `url` or `headers` is an error (`AR9O3`): the server is not packaged,
  `validate` fails and `publish` stops.
- `PLUGIN_ROOT` and `PLUGIN_DATA` may not be set in `env`; the client supplies them.

## Bundling files

A package may carry arbitrary files beside `plugin.json`, `mcp.json` and `skills/`, and a stdio `command` may point
into them. `[plugin] include` lists globs of files to copy:

- Globs are matched against the content root: `[plugin] content_root` when set, otherwise the config directory
  (`.ai-rulez/`). The matches are written at the same relative path at the package root, so
  `.ai-rulez/scripts/mcp-launch.sh` becomes `scripts/mcp-launch.sh`. For the other runtimes, which have no `include`,
  keep such a file at the repository root yourself (see [Authoring plugins](/ai-rulez/plugins/#mcp-servers)).
- `**` crosses directories, `*` and `?` do not, and a trailing `/` matches everything below a directory.
- Matched files are copied verbatim, keeping their relative path and executable bit, and are recorded in
  `.ai-rulez-generated.json`, so `verify --plugin` reports a changed or missing copy.
- `plugin.json`, `mcp.json`, `skills/` and the provenance sidecar are never matched, so a broad glob cannot overwrite
  them.
- A glob that is absolute or contains `..` is rejected when the configuration loads.

`include` applies to the packages this page describes (`agent-plugins`, `copilot`, and `codex` with the root layout).
For copying shared reference files into every skill, see
[Shared resources across skills](/ai-rulez/plugins/#shared-resources-across-skills).

## Extensions

Content only one client understands goes under a reverse-domain namespace, either as manifest data
(`plugin.json` `extensions.<namespace>`) or as files (`<namespace>/...`). Clients ignore namespaces they do not
implement.

| Namespace | Content | Written by `generate --plugin` |
| --- | --- | --- |
| `com.openai` | the Codex `interface` block (manifest data) | with the `codex` root layout |
| `com.github.copilot` | `agents/<name>.agent.md` | with the `copilot` runtime |
| `com.anthropic.claude-code` | `agents/<name>.md`, `commands/<name>.md`, `hooks/hooks.json` | no; read on import |
| `io.github.goldziher.ai-rulez` | `rules/<name>.md`, `context/<name>.md` | no; read on import |

## Validation

`validate` builds the package in memory and checks it as a client would load it. `publish` runs the same check
on the bundle it packages, in `--dry-run` as well, and stops with exit status 2 on the first error. Only single-plugin
projects are checked by `validate`; marketplace members are checked when they are published.

| Code | Meaning |
| --- | --- |
| `AR9O0` | `plugin.json` is missing or invalid, the spec version is unsupported, or an extension namespace is invalid |
| `AR9O1` | a skill breaks the Agent Skills rules (name, description, frontmatter) or is not at `skills/<name>/SKILL.md` |
| `AR9O2` | `mcp.json` or a server fails the schema or the rules for `command`, `url` and headers |
| `AR9O3` | an unsupported `${VAR}` placeholder |
| `AR9O4` | a field or server that cannot be packaged was left out (warning) |
| `AR9O5` | a path or link resolves outside the package root, or a file cannot be read |

`ai-rulez validate --explain AR9O3` prints the rule with examples.

Every generated file is recorded with its `Content-Hash` in `.ai-rulez-generated.json`, so `verify --plugin` covers `plugin.json`, `mcp.json`, the skills and included files. The output is deterministic: sorted,
no timestamps, two-space JSON with a trailing newline.

## Publishing

The standard defines no archive, registry or signature. The package travels in the regular [publish](/ai-rulez/publish/)
outputs (release archive, npm package or OCI artifact) and is signed with the rest of the release.

To get a directory that holds only the Agent Plugins files, without the other runtimes' manifests, use the
`agent-plugins` emitter:

```bash
ai-rulez publish emit agent-plugins --output-dir out
```

```text
out/acme-tools/plugin.json
out/acme-tools/mcp.json
out/acme-tools/scripts/mcp-launch.sh
out/acme-tools/skills/review-pr/SKILL.md
out/acme-tools/skills/review-pr/scripts/lint.sh
```

Like every `publish` command, `publish emit` runs the [publish preflight](/ai-rulez/publish/#pipeline) first: the project must
be committed and locked (`ai-rulez lock`), and the bundle up to date. The emitter reads the generated package back,
validates it and writes it again, one directory per plugin. To convert
the package to another spec version, set the `spec` option:

```toml
[[publish.emitters]]
name = "agent-plugins"
options = { spec = "1.1.0" }
```

## Import

`ai-rulez convert` reads an Agent Plugins directory into an `.ai-rulez/` tree:

```bash
ai-rulez convert --from agent-plugins --source ./some-plugin --into "$PWD/.ai-rulez" --write
```

`--into` is relative to `--source` unless absolute; without it the tree is written to `./some-plugin/.ai-rulez/`.

| Package | Becomes |
| --- | --- |
| `plugin.json` | the `[plugin]` block, with `runtimes = ["agent-plugins"]` (and `spec` when not 1.0.0) |
| `skills/<name>/` | skills, with their `scripts/`, `references/` and other files |
| `mcp.json` | `[[mcp_servers]]` |
| `com.anthropic.claude-code`, `com.github.copilot`, `io.github.goldziher.ai-rulez` | agents, commands and rules |

The report lists what has no equivalent: other files at the package root, unknown namespaces, `hooks/hooks.json` and a
server `cwd`. A manifest without `version` or `description` cannot form a `[plugin]` block, so none is written and the
report says so. The generated-file header ai-rulez writes into Markdown is removed on import.

## See also

- [Authoring plugins](/ai-rulez/plugins/): all plugin runtimes and the `[plugin]` block.
- [Publish](/ai-rulez/publish/)
