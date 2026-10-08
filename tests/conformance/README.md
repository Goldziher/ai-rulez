# Conformance tests

This package backs the claims of [docs/standards.md](../../docs/standards.md). Each test validates a document ai-rulez
produces, either a checked-in golden or the output of the CLI built from the working tree, against the standard's own
schema. The schemas are vendored under `schemas/` and pinned by SHA-256 in `schemas_test.go`. The tests never use the
network; the only external step is the refresh below, done by hand.

```bash
go test ./tests/conformance/
```

## Vendored schemas

| Directory | Standard and version | Source |
| --- | --- | --- |
| `schemas/agent-plugins/1.0.0`, `1.1.0` | Agent Plugins 1.0.0 (published), 1.1.0 (draft) | https://github.com/agentplugins/agent-plugins-spec at commit `ff8ab5e392cc87bd88d87c060815a87490e51003` |
| `schemas/ard` | ARD 0.91 (proposal), entry schema | https://github.com/ards-project/ard-spec at commit `b76f235a8f461876ad4f1e77abd0eb0eb302b48d` |
| `schemas/cyclonedx/1.6` | CycloneDX 1.6, with its SPDX-license and JSF dependencies | https://github.com/CycloneDX/specification, `schema/bom-1.6.schema.json`, `spdx.schema.json`, `jsf-0.82.schema.json` |
| `schemas/spdx/2.3` | SPDX 2.3.1 JSON schema | https://github.com/spdx/spdx-spec, the SPDX 2.3 JSON schema (`spdx-schema.json`) |
| `schemas/in-toto/statement-v1.schema.json` | in-toto Statement v1 | Hand-written from https://github.com/in-toto/attestation/blob/main/spec/v1/statement.md. Upstream publishes no JSON Schema. |
| `schemas/dsse/envelope.schema.json` | DSSE v1 envelope | Hand-written from https://github.com/secure-systems-lab/dsse (`envelope.md`, `protocol.md`). Upstream publishes no JSON Schema. |

Agent Skills, AGENTS.md and llms.txt have no machine-readable schema. Their rules are transcribed into Go
(`skills_test.go`, `agentsmd_test.go`, `llmstxt_test.go`) and the test comments name the specification section.

The product embeds its own copies of the Agent Plugins, ARD and CycloneDX/SPDX schemas
(`internal/agentplugins/schemas`, `internal/ard/schema`, `internal/sbom/testdata`).
`TestVendoredSchemasArePinned` fails when a copy here and the embedded one differ, so the two move together.

## Refresh a schema

1. Fetch the new file from the upstream source in the table, into the directory named there. Keep the file byte for byte;
   do not reformat it.
2. Update the commit or version in the table above, in `docs/standards.md`, and in the owning package
   (`SchemaSourceCommit` in `internal/agentplugins/schema.go`, `SchemaCommit` and `SchemaSHA256` in `internal/ard/schema.go`).
3. Copy the file over the product's embedded copy, then run `shasum -a 256` on it and update the digest in
   `schemas_test.go` and in the owning package's own pin test.
4. Run `go test ./tests/conformance/ ./internal/agentplugins/ ./internal/ard/ ./internal/sbom/`. A failure is either a
   real nonconformance in our output or an upstream rule change to review.
5. A new Agent Plugins version also needs an entry in `agentplugins.Specs`. The plugin `name` pattern uses a lookahead
   that RE2 cannot compile; `agentplugins_test.go` strips the keyword after asserting the upstream pattern is unchanged,
   so a changed pattern fails loudly and `validPluginName` is re-derived.

The two hand-written schemas carry no upstream digest. Change them only with the specification text in hand.
