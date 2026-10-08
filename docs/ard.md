# Agentic Resource Discovery (ard.json)

ai-rulez can publish your skills, MCP servers and plugin for discovery through
[Agentic Resource Discovery](https://agenticresourcediscovery.org/) (ARD): one `ard.json` manifest that a registry or an
agent fetches from `https://<domain>/.well-known/ard.json`.

!!! warning "The specification is a proposal"
    ARD is still a proposal. ai-rulez implements **spec v0.91** and validates the manifest against the entry schema of
    [ards-project/ard-spec](https://github.com/ards-project/ard-spec) commit `b76f235`, vendored under `internal/ard/schema`.
    The media type registry and the term names can change; the version is pinned in `ard.SpecVersion`.

## Configure

```toml
[ard]
publisher = "acme.example"   # the domain that serves /.well-known/ard.json (an FQDN)
namespace = "conventions"    # colons separate sub-segments: "team:tools"
# base_url = "https://acme.example/ard"   # where skills/<name>/SKILL.md is served (optional)
# plugin_type = "application/vnd.example.plugin+json"
# [ard.queries]                            # representative queries by resource name (optional)
# docs = ["search the acme docs", "find the api reference"]
```

An identifier is `urn:air:<publisher>:<namespace>:<name>`; `validate` rejects a publisher that is not a fully qualified
domain name and a namespace or name outside letters, digits, `.`, `_` and `-`.

## Generate

```bash
ai-rulez publish --emit ard          # dist/emit/ard/ard.json, in the release
ai-rulez publish emit ard --out ard  # only the manifest, without a release
```

The emitter is also available as `[[publish.emitters]] name = "ard"`. Needs a committed project and a release tag, as every
publish does.

| Resource | `type` | `url` or `data` |
| --- | --- | --- |
| Root skill | `application/ai-skill+md` | `url`: the raw `SKILL.md` at the release tag (GitHub), or `<base_url>/skills/<name>/SKILL.md` |
| MCP server (`[[mcp_servers]]`, `[[plugin.mcp]]`, enabled) | `application/mcp-server-card+json` | `data`: name, description and transport |
| Plugin | `application/vnd.ai-rulez.plugin+json` (override with `plugin_type`) | `url`: the release archive |

Exactly one of `url` and `data` is set. Environment values and headers of an MCP server never reach the manifest, and a
server URL with credentials is refused. With `base_url`, the emitter also writes `skills/<name>/SKILL.md` next to
`ard.json`, so hosting is a copy of the output directory. Only root skills are listed.

The spec registers no plugin type, so the plugin type is ai-rulez's own vendor type until one exists; the conformance tool
accepts it as an extension type. A plugin that builds an [Agent Plugins](agent-plugins.md) package is tagged `agent-plugins`.

**Fields.** `displayName` defaults to the name; `description`, `version` (a skill's `metadata.version`, a plugin's
version), `tags` (skill `keywords`, plugin keywords) and `capabilities` (the plugin category) come from the sources.
`updatedAt` is the release time (the commit time, or `SOURCE_DATE_EPOCH`), never the clock, so the same release always
produces the same bytes.

**Representative queries** (2 to 5 per entry) are taken, in order, from the skill frontmatter `representative_queries`,
the prompts of eval cases that expect the skill to trigger (`expect_trigger: true`, near misses excluded), then the
frontmatter `triggers`. A plugin takes one query from each of its skills in turn. An MCP server has nothing to derive them
from: list them under `[ard.queries]`. `[ard.queries]` replaces the derived queries of the named resource.

## Validate

`validate --strict` builds the manifest in memory with a placeholder location and checks it against the pinned schema:
[AR9S0](strict-validation.md#ar9s0-ard-manifest-invalid) (schema),
[AR9S1](strict-validation.md#ar9s1-ard-identifier-invalid) (identifier),
[AR9S2](strict-validation.md#ar9s2-ard-entry-invalid) (url or data),
[AR9S3](strict-validation.md#ar9s3-ard-queries-out-of-range) (queries, a warning) and
[AR9S4](strict-validation.md#ar9s4-ard-not-declared) (emitter without `[ard]`). `publish` runs the same gate, and prints
the warnings of the emitter in `--dry-run` too.

## Host the file

Serve the manifest at `https://<publisher>/.well-known/ard.json` with:

- HTTPS only;
- `Content-Type: application/json`;
- `Access-Control-Allow-Origin: *`, so browser-based agents can read it.

## Discovery besides `/.well-known`

- **HTML link.** A page can point at the manifest with `<link rel="ard" href="https://acme.example/ard.json">` in its
  `<head>`; consumers must honour `rel="ard"`.
- **DNS (optional).** If you cannot serve the well-known path, a DNS record can point at the file. The specification and the
  how-to guide disagree: the spec (section 5.1) speaks of Service Binding records with the example name
  `_entries._agents.example.com` and defines no record layout, while the guide shows a TXT record at
  `_catalog._agents.yourdomain.com` with the value `"url=https://host/ard.json"`. ai-rulez publishes no DNS record; pick
  one after checking the current spec.

## Why there is no `trustManifest`

The optional `trustManifest` binds an entry to a verifiable identity. The spec (section 4.5.1) requires that identity to be
a credential whose trust domain is the identifier's publisher, and leaves verification to a trust framework the manifest
names. ai-rulez signs with Sigstore keyless DSSE bundles, whose identity is a CI workflow or an OIDC account rather than a
credential issued by your domain, and no trust framework has been chosen. An emitted `trustManifest` would claim a binding
nobody can check, so it is left out. The signature of the release itself (`--sign-keyless`) is unaffected. The pinned schema
also spells the term `TrustManifest` while the spec text says `trustManifest`, so the name is unsettled.
