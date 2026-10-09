# Standards conformance

ai-rulez claims conformance with a standard only when a passing test backs the claim. Every test named here is in
[`tests/conformance`](https://github.com/Goldziher/ai-rulez/tree/main/tests/conformance) and runs with `go test ./...`. It
validates a golden or freshly generated document against the standard's own schema, vendored and pinned under
`tests/conformance/schemas`, with no network access. The README in that directory says how to refresh a schema.

Status values:

- **conformant**: our output validates against the standard's official machine-readable schema in a test.
- **partial**: tested, but the standard has no official schema (the test transcribes its written rules), or only part of
  what we emit is covered. The gaps column says what is missing.
- **planned**: implemented or intended, with no conformance test yet.

| Standard | Pinned version | Status | What we generate and validate | Test | Known gaps |
| --- | --- | --- | --- | --- | --- |
| [Agent Plugins](https://agent-plugins.org) | 1.0.0 (published, default) and 1.1.0 (working draft); [schema repo](https://github.com/agentplugins/agent-plugins-spec) at `ff8ab5e` | conformant | `plugin.json`, `mcp.json` and `skills/` from `generate --plugin`; read back and checked by `validate` ([page](agent-plugins.md)) | `TestAgentPluginsGoldenConformsToTheOfficialSchemas`, `TestAgentPluginsGeneratedByTheCLIConformToTheOfficialSchemas` | The official name pattern uses a lookahead RE2 cannot compile; the test applies the same rule in Go. Extension namespaces are not schema-checked beyond the base manifest. |
| [ARD](https://agenticresourcediscovery.org/) | 0.91 (proposal); [ard-spec](https://github.com/ards-project/ard-spec) at `b76f235` | conformant | `ard.json` from `publish --emit ard`; entry and manifest schema plus the URN grammar ([page](ard.md)) | `TestARDManifestGoldenConformsToTheEntrySchema`, `TestARDValidatorAcceptsTheGoldenAndNamesItsSpecVersion` | The spec is a proposal; media types and term names can change. The plugin entry type is an ai-rulez vendor type. The golden comes from the in-package model, not a release. |
| [llms.txt](https://llmstxt.org/) | No versioned release; format as published at llmstxt.org | partial | `llms.txt` and `llms-full.txt` from the `llms-txt` preset, and for the docs site ([page](llms-txt.md)) | `TestGeneratedLLMSTxtFollowsTheFormat`, `TestPublishedDocsSiteLLMSTxtFollowsTheFormat` | No official schema. The test transcribes the format (H1, optional blockquote, H2 link lists). `llms-full.txt` is checked for its H1 only; the format does not define it. |
| [Agent Skills](https://agentskills.io/specification) | No versioned release; specification as published at agentskills.io | partial | `SKILL.md` for every skill, with its resources, under `.claude/skills`, `.agents/skills` and `skills/` ([page](skills.md)) | `TestGeneratedSkillsFollowTheAgentSkillsSpecification`, plus the skill checks in the Agent Plugins tests | No official schema. The test transcribes the frontmatter rules (name, description, compatibility, license, metadata, allowed-tools). Resource layout is not checked. |
| [AGENTS.md](https://agents.md) | No versioned release | partial | `AGENTS.md` at the project root ([page](agents-md.md)) | `TestGeneratedAgentsMDIsPlainMarkdownAtTheRoot` | The format has no schema and no required fields. The test checks that the file is plain Markdown at the root with the project's guidance. Nested `AGENTS.md` files in monorepos are not covered. |
| MCP server configuration | The `mcp.json` schema of Agent Plugins 1.0.0 and 1.1.0 | partial | `mcp.json` (stdio, streamable-http and sse servers) | `TestMCPServerEntriesUseTheSchemaTransportTypes`, plus the `mcp.json` schema check in the Agent Plugins tests | The per-harness files (`.mcp.json`, `.codex/config.toml` and others) follow each tool's own format and are not validated against a schema here. |
| MCP server card | None pinned | planned | Embedded as `application/mcp-server-card+json` entries in `ard.json` | None | No official card schema is vendored; the card shape is only checked as part of the ARD entry. |
| [CycloneDX](https://cyclonedx.org/) | 1.6 | conformant | `ai-rulez sbom` ([page](sbom.md)) | `TestCycloneDXOutputConformsToTheOfficial16Schema` | The test validates structure. NTIA minimum-element and license-expression policy checks are not run. |
| [SPDX](https://spdx.dev/) | 2.3 (JSON) | conformant | `ai-rulez sbom --type spdx-json` | `TestSPDXOutputConformsToTheOfficial23Schema` | Structure only. The in-package tests also check unique identifiers and resolving relationships. |
| [in-toto Statement](https://github.com/in-toto/attestation) and [DSSE](https://github.com/secure-systems-lab/dsse) | Statement v1; DSSE v1 | partial | Signed statements inside Sigstore bundles from `ai-rulez sign`; `verify` checks them ([page](signing.md)) | `TestSignedStatementIsADSSEEnvelopeOverAnInTotoStatement` | Neither project publishes a JSON Schema, so the vendored schemas are hand-written from the specifications. Sigstore bundle structure and certificate chains are covered by `internal/signing` tests, not by a conformance schema. The `v0.1` statement type is accepted on verification only. |
| OKF | v0.2 | not covered here | Native format ([page](okf.md)) | `internal/okf` | Tracked in [#281](https://github.com/Goldziher/ai-rulez/issues/281). |
| OpenTelemetry (OTLP) | n/a | not covered here | Export only ([page](telemetry.md)) | n/a | No document we write is validated against an OTLP schema. |

## Reading a status

A conformant status says the documents the tests produce validate. It does not say every possible configuration does:
the tests use representative projects (rules, context, skills, stdio and remote MCP servers). A schema accepting a
document also does not mean a given client will load it; the per-harness behaviour is in [harness traps](harness-traps.md).
