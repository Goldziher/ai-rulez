---
title: "Standards conformance"
description: "ai-rulez writes several open formats besides each assistant's native files."
---

ai-rulez writes several open formats besides each assistant's native files. For each standard, the version ai-rulez
targets, what it generates, how conformance is checked and the known gaps are below.

A claim of conformance is backed by a test in
[`tests/conformance`](https://github.com/Goldziher/ai-rulez/tree/main/tests/conformance). Each test validates a
document ai-rulez produces (a checked-in sample or the output of the freshly built CLI) against the standard's own
schema, vendored and pinned under `tests/conformance/schemas`. The tests run offline (`go test ./tests/conformance/`).

## Status

| Status | Meaning |
| ------ | ------- |
| **conformant** | The output validates against the standard's official machine-readable schema. |
| **partial** | The standard has no official schema (the test encodes its written rules), or only part of the output is covered. The gaps column says what is missing. |
| **not covered** | ai-rulez reads or writes the format, but no conformance test checks it against the standard. |

| Standard | Version | Status | What ai-rulez generates | Test | Known gaps |
| --- | --- | --- | --- | --- | --- |
| [Agent Plugins](https://agent-plugins.org) | 1.0.0 (published, default) and 1.1.0 (working draft); [schema repo](https://github.com/agentplugins/agent-plugins-spec) at `ff8ab5e` | conformant | `plugin.json`, `mcp.json` and `skills/` from `generate --plugin`; `validate` reads them back ([page](/ai-rulez/agent-plugins/)) | [`agentplugins_test.go`](https://github.com/Goldziher/ai-rulez/blob/main/tests/conformance/agentplugins_test.go) | The official plugin-name pattern uses a lookahead that Go's regular expressions cannot compile, so the test applies the same rule in code. Extension namespaces are not schema-checked beyond the base manifest. |
| [ARD](https://agenticresourcediscovery.org/) | 0.91 (proposal); [ard-spec](https://github.com/ards-project/ard-spec) at `b76f235` | conformant | `ard.json` from `publish --emit ard`, checked against the entry and manifest schema and the URN grammar ([page](/ai-rulez/ard/)) | [`ard_test.go`](https://github.com/Goldziher/ai-rulez/blob/main/tests/conformance/ard_test.go) | ARD is a proposal; media types and term names can change. The plugin entry type is an ai-rulez vendor type. |
| [llms.txt](https://llmstxt.org/) | No versioned release; the format published at llmstxt.org | partial | `llms.txt` and `llms-full.txt` from the `llms-txt` preset ([page](/ai-rulez/llms-txt/)) | [`llmstxt_test.go`](https://github.com/Goldziher/ai-rulez/blob/main/tests/conformance/llmstxt_test.go) | No official schema; the test checks the published structure (H1, optional blockquote, H2 link lists). `llms-full.txt` is checked for its H1 only, because the format does not define it. |
| [Agent Skills](https://agentskills.io/specification) | No versioned release; the specification published at agentskills.io | partial | `SKILL.md` and its resources for every skill, under `.claude/skills`, `.agents/skills`, `skills/` and the other skill folders ([page](/ai-rulez/skills/)) | [`skills_test.go`](https://github.com/Goldziher/ai-rulez/blob/main/tests/conformance/skills_test.go) | No official schema; the test checks the frontmatter rules (`name`, `description`, `compatibility`, `license`, `metadata`, `allowed-tools`). The resource layout is not checked. |
| [AGENTS.md](https://agents.md) | No versioned release | partial | `AGENTS.md` at the project root and in nested scopes ([page](/ai-rulez/agents-md/)) | [`agentsmd_test.go`](https://github.com/Goldziher/ai-rulez/blob/main/tests/conformance/agentsmd_test.go) | The format has no schema and no required fields; the test checks that the root file is plain Markdown carrying the project's guidance. Nested files are not covered. |
| MCP server configuration | The `mcp.json` schema of Agent Plugins 1.0.0 and 1.1.0 | partial | `mcp.json` with stdio, streamable HTTP and SSE servers | [`mcp_test.go`](https://github.com/Goldziher/ai-rulez/blob/main/tests/conformance/mcp_test.go) | The per-tool files (`.mcp.json`, `.cursor/mcp.json`, `.codex/config.toml` and others) follow each tool's own format and are not validated against a schema. |
| [MCP Server Card](https://modelcontextprotocol.io/seps/2127-mcp-server-cards) | SEP-2127 (Final); `schema.json` of the experimental [server-card extension](https://github.com/modelcontextprotocol/experimental-ext-server-card) at `526201b` | partial | One `<server>.json` per remote `[[mcp_servers]]` entry from `publish --emit mcp-server-card`, and the same cards embedded in `ard.json` ([page](/ai-rulez/mcp-server-card/)) | [`mcpcard_test.go`](https://github.com/Goldziher/ai-rulez/blob/main/tests/conformance/mcpcard_test.go) | The schema comes from an experimental extension and can change. Cards describe remote servers only, so stdio servers get none. Title, icons, repository, website, URL variables and `_meta` are not generated. `validate` does not check cards. |
| [CycloneDX](https://cyclonedx.org/) | 1.6 | conformant | `ai-rulez sbom` ([page](/ai-rulez/sbom/)) | [`sbom_test.go`](https://github.com/Goldziher/ai-rulez/blob/main/tests/conformance/sbom_test.go) | Structure only; NTIA minimum-element and license-expression checks are not run. |
| [SPDX](https://spdx.dev/) | 2.3 (JSON) | conformant | `ai-rulez sbom --type spdx-json` ([page](/ai-rulez/sbom/)) | [`sbom_test.go`](https://github.com/Goldziher/ai-rulez/blob/main/tests/conformance/sbom_test.go) | Structure only. |
| [in-toto Statement](https://github.com/in-toto/attestation) and [DSSE](https://github.com/secure-systems-lab/dsse) | Statement v1; DSSE v1 | partial | Signed statements inside Sigstore bundles from `ai-rulez sign`, checked by `ai-rulez verify` ([page](/ai-rulez/signing/)) | [`attestation_test.go`](https://github.com/Goldziher/ai-rulez/blob/main/tests/conformance/attestation_test.go) | Neither project publishes a JSON Schema, so the schemas are written from the specifications. Sigstore bundle structure and certificate chains are not covered by a conformance schema. Statement `v0.1` is accepted when verifying only. |
| [OKF](/ai-rulez/okf/) | 0.2 | not covered | `.ai-rulez/` is an OKF bundle; `export okf` writes one and `okf validate` lints any bundle ([page](/ai-rulez/okf/)) | — | No conformance test against an external schema. |
| OpenTelemetry (OTLP) | — | not covered | Item-load telemetry export with `telemetry export --to otlp` ([page](/ai-rulez/telemetry/)) | — | No document ai-rulez writes is validated against an OTLP schema. |

## Standard files and ai-rulez extensions

With `source = "agents"`, ai-rulez reads the root `AGENTS.md` and `.agents/skills/<name>/SKILL.md` as the AGENTS.md
and Agent Skills standards define them, and never rewrites them. The other folders it reads under `.agents/` are
extensions:

| Folder | Convention |
| ------ | ---------- |
| `.agents/skills/` | Agent Skills standard |
| `.agents/rules/`, `.agents/agents/`, `.agents/checks/` | Antigravity and Amp conventions |
| `.agents/commands/`, `.agents/context/`, `.agents/domains/` | ai-rulez |

Nested `AGENTS.md` files are read as the standard describes: each applies to its directory's subtree. Splitting the
root `AGENTS.md` into one item per `##` heading (`agents_md_split = true`) is an ai-rulez convention and is off by
default. See [.agents as a source](/ai-rulez/agents-source/).

## Reading a status

A conformant status means the documents produced from representative projects (rules, context, skills, stdio and
remote MCP servers) validate. It does not prove that every configuration does, or that a particular client loads the
result; per-tool loading behavior is in [Harness traps](/ai-rulez/harness-traps/).
