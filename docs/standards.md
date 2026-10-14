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
| [MCP server card](https://modelcontextprotocol.io/seps/2127-mcp-server-cards) | SEP-2127 (Final); the schema is `schema.json` of the experimental [server-card extension](https://github.com/modelcontextprotocol/experimental-ext-server-card) (Apache-2.0) at `526201b` | partial (experimental extension, pinned `526201b`) | One `<server>.json` per remote `[[mcp_servers]]` entry from `publish --emit mcp-server-card`, and the same card embedded as `application/mcp-server-card+json` entries in `ard.json` ([page](mcp-server-card.md)) | `TestMCPServerCardGoldensConformToTheExtensionSchema`, `TestMCPServerCardBuilderOutputConformsToTheExtensionSchema`, `TestMCPServerCardSchemaRejectsWhatACardMustNotBe` | The schema is generated from `schema.ts` by an experimental extension, not yet a published versioned schema; it can change. A card describes remote servers only, so stdio servers get none. Identity beyond name, version and description (title, icons, repository, website), URL variables and `_meta` are not generated. `validate` does not check cards; only the conformance tests do. |
| "Agent bundle" | No upstream spec is defined or pinned | planned | Nothing | None | Tracked in [#393](https://github.com/Goldziher/ai-rulez/issues/393). No specification exists to target, so nothing is generated or claimed. |
| [CycloneDX](https://cyclonedx.org/) | 1.6 | conformant | `ai-rulez sbom` ([page](sbom.md)) | `TestCycloneDXOutputConformsToTheOfficial16Schema` | The test validates structure. NTIA minimum-element and license-expression policy checks are not run. |
| [SPDX](https://spdx.dev/) | 2.3 (JSON) | conformant | `ai-rulez sbom --type spdx-json` | `TestSPDXOutputConformsToTheOfficial23Schema` | Structure only. The in-package tests also check unique identifiers and resolving relationships. |
| [in-toto Statement](https://github.com/in-toto/attestation) and [DSSE](https://github.com/secure-systems-lab/dsse) | Statement v1; DSSE v1 | partial | Signed statements inside Sigstore bundles from `ai-rulez sign`; `verify` checks them ([page](signing.md)) | `TestSignedStatementIsADSSEEnvelopeOverAnInTotoStatement` | Neither project publishes a JSON Schema, so the vendored schemas are hand-written from the specifications. Sigstore bundle structure and certificate chains are covered by `internal/signing` tests, not by a conformance schema. The `v0.1` statement type is accepted on verification only. |
| OKF | v0.2 | not covered here | Native format ([page](okf.md)) | `internal/okf` | Tracked in [#281](https://github.com/Goldziher/ai-rulez/issues/281). |
| OpenTelemetry (OTLP) | n/a | not covered here | Export only ([page](telemetry.md)) | n/a | No document we write is validated against an OTLP schema. |

## Standards first, extensions second

Future ai-rulez versions will always follow the AGENTS.md and Agent Skills standards. With `source = "agents"` the
root `AGENTS.md` and `.agents/skills/<name>/SKILL.md` are read as those standards define them. The folders ai-rulez
adds beside them (`.agents/rules`, `agents`, `checks`, `commands`, `context` and `domains`) are extensions, named after
what they hold so that a later version of a standard supersedes them. Rules, agents and checks follow the
Antigravity and Amp conventions; `commands`, `context` and `domains` are ai-rulez's own. See
[.agents as a source](agents-source.md); `TestGenerateTranslatesTheSourceAndLeavesItAlone` and the `agents-source`
goldens back the claim that the standards' files are read and never rewritten. AGENTS.md defines no internal
structure, so reading it as one item per `##` heading (`agents_md_split = true`) is an ai-rulez convention, off by
default and covered by `TestAgentsMDSplitMakesOneItemPerHeading`.

## Reading a status

A conformant status says the documents the tests produce validate. It does not say every possible configuration does:
the tests use representative projects (rules, context, skills, stdio and remote MCP servers). A schema accepting a
document also does not mean a given client will load it; the per-harness behaviour is in [harness traps](harness-traps.md).
