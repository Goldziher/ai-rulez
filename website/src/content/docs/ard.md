---
title: "Agentic Resource Discovery (ard.json)"
description: "The ard emitter publishes your skills, remote MCP servers and plugin for discovery through Agentic Resource Discovery (ARD): one ard.json manifest that registries and agents fetch from https://<domain>/.well-known/ard.json."
---

The `ard` emitter publishes your skills, remote MCP servers and plugin for discovery through
[Agentic Resource Discovery](https://agenticresourcediscovery.org/) (ARD): one `ard.json` manifest that registries and
agents fetch from `https://<domain>/.well-known/ard.json`.

:::caution[The specification is a proposal]
ai-rulez implements ARD **spec v0.91** and validates the manifest against the entry schema of
[ards-project/ard-spec](https://github.com/ards-project/ard-spec) at commit `b76f235`. Media types and field names
can still change.
:::

## Example

```toml
# .ai-rulez/config.toml
version = "5.0"
name = "acme-tools"

[plugin]
name = "acme-tools"
version = "1.2.0"
description = "Code review skills and an MCP server for Acme repositories."
repository = "https://github.com/acme/acme-tools"
keywords = ["review", "mcp"]
runtimes = ["agent-plugins", "claude"]

[plugin.author]
name = "Acme Tooling"

[[mcp_servers]]
name = "acme-search"
description = "Search Acme documentation."
transport = "http"
url = "https://docs.acme.example/mcp"
headers = { Authorization = "Bearer ${DOCS_TOKEN}" }

[ard]
publisher = "acme.example"
namespace = "tools"

[ard.queries]
acme-search = ["search the acme docs", "find the acme api reference"]
```

With one skill, `review-pr`, in `.ai-rulez/skills/`:

```bash
ai-rulez publish emit ard --output-dir ard
```

```text
WARN  ard: AR9S3 urn:air:acme.example:tools:acme-tools: no representativeQueries: ...
WARN  ard: AR9S3 urn:air:acme.example:tools:review-pr: no representativeQueries: ...
wrote ard/ard.json
```

`ard/ard.json` (trimmed):

```json
{
  "entries": [
    {
      "identifier": "urn:air:acme.example:tools:acme-search",
      "displayName": "acme-search",
      "type": "application/mcp-server-card+json",
      "data": {
        "$schema": "https://static.modelcontextprotocol.io/schemas/v1/server-card.schema.json",
        "description": "Search Acme documentation.",
        "name": "example.acme/acme-search",
        "remotes": [ ... ],
        "version": "1.2.0"
      },
      "description": "Search Acme documentation.",
      "representativeQueries": ["search the acme docs", "find the acme api reference"],
      "updatedAt": "2026-11-05T16:23:33Z"
    },
    {
      "identifier": "urn:air:acme.example:tools:acme-tools",
      "displayName": "acme-tools",
      "type": "application/vnd.ai-rulez.plugin+json",
      "url": "https://github.com/acme/acme-tools/releases/download/v1.2.0/acme-tools-1.2.0.tar.gz",
      "description": "Code review skills and an MCP server for Acme repositories.",
      "tags": ["agent-plugins", "mcp", "review"],
      "version": "1.2.0",
      "updatedAt": "2026-11-05T16:23:33Z"
    },
    {
      "identifier": "urn:air:acme.example:tools:review-pr",
      "displayName": "review-pr",
      "type": "application/ai-skill+md",
      "url": "https://raw.githubusercontent.com/acme/acme-tools/v1.2.0/.ai-rulez/skills/review-pr/SKILL.md",
      "description": "Review a pull request against Acme conventions. Use when asked to review a diff or PR.",
      "updatedAt": "2026-11-05T16:23:33Z"
    }
  ]
}
```

The `AR9S3` warnings say the skill and the plugin have no representative queries; see
[Representative queries](#representative-queries) for where they come from.

## Configuration

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `publisher` | string | required | The fully qualified domain name that serves `/.well-known/ard.json` |
| `namespace` | string | required | A short name for this project's resources; colons separate sub-segments (`team:tools`) |
| `base_url` | string | none | An `https` URL where the skill files are served. When set, skill entries point at `<base_url>/skills/<name>/SKILL.md` and the emitter writes those files next to `ard.json` |
| `plugin_type` | string | `application/vnd.ai-rulez.plugin+json` | The media type of the plugin entry |
| `[ard.queries]` | table of string lists | none | Representative queries by resource name; replaces the derived queries of that resource |

Every identifier is `urn:air:<publisher>:<namespace>:<name>`. Configuration loading rejects a publisher that is not a
fully qualified domain name, and a namespace or name with characters outside letters, digits, `.`, `_` and `-`.

## Generate

| Command | Output |
| --- | --- |
| `ai-rulez publish emit ard [--output-dir DIR]` | only the manifest (and skill files with `base_url`), in `DIR` (default `emit/ard/`) |
| `ai-rulez publish --emit ard` | `dist/emit/ard/ard.json`, as part of a [release](/ai-rulez/publish/) |

Add `[[publish.emitters]] name = "ard"` to run it on every publish. Both commands run the
[publish preflight](/ai-rulez/publish/#pipeline) first, so the project must be committed and locked, with the plugin bundle
generated.

## Entries

| Resource | `type` | Location |
| --- | --- | --- |
| Root skill | `application/ai-skill+md` | `url`: with `base_url`, `<base_url>/skills/<name>/SKILL.md`; otherwise the raw `SKILL.md` on GitHub at the release tag `v<version>` |
| Remote MCP server (enabled `[[mcp_servers]]` or `[[plugin.mcp]]` with a URL) | `application/mcp-server-card+json` | `data`: the [MCP server card](/ai-rulez/mcp-server-card/) |
| Plugin | `plugin_type` | `url`: the release archive on the repository's releases page, or `<base_url>/<archive>` when there is no repository |

- Only root skills are listed; domain skills are not.
- Stdio MCP servers are left out, because a server card describes remote endpoints. A remote server whose URL is not
  `https` is an error (`AR9S2`). Servers whose URL could carry a secret are skipped the same way as for [server cards](/ai-rulez/mcp-server-card/#skipped-servers). Header and environment
  values never reach the manifest.
- Raw skill URLs are only known for github.com. For another host, set `base_url`.
- A plugin built with the [Agent Plugins](/ai-rulez/agent-plugins/) runtime is tagged `agent-plugins`.

| Field | Source |
| --- | --- |
| `displayName` | the resource name |
| `description` | the skill's or server's description, or the plugin description |
| `version` | a skill's `metadata.version` frontmatter; the plugin version |
| `tags` | skill `keywords` frontmatter; plugin `keywords` |
| `capabilities` | the plugin `category` |
| `updatedAt` | the release time: `SOURCE_DATE_EPOCH`, else the commit time. The same commit always gives the same bytes |

## Representative queries

Registries build their search index from `representativeQueries`. Each entry needs 2 to 5; outside that range is a
warning (`AR9S3`). For a skill they are taken, in order, from:

1. the frontmatter list `representative_queries`;
2. the prompts of the skill's eval cases that expect it to trigger (`expect_trigger: true`), excluding near misses;
3. the frontmatter list `triggers`.

A plugin takes the first query of each of its skills, then the second, and so on. An MCP server has nothing to derive
queries from, so list them under `[ard.queries]`. An `[ard.queries]` entry replaces the derived queries of that
resource.

```markdown
---
name: review-pr
description: Review a pull request against Acme conventions. Use when asked to review a diff or PR.
representative_queries:
  - review this pull request
  - check my diff against the acme conventions
---
```

## Validation

`validate` builds the manifest in memory and checks it against the pinned schema. `publish` runs the same
check and prints the emitter's warnings in `--dry-run` too.

| Code | Severity | Meaning |
| --- | --- | --- |
| [AR9S0](/ai-rulez/strict-validation/#ar9s0-ard-manifest-invalid) | error | the manifest fails the schema |
| [AR9S1](/ai-rulez/strict-validation/#ar9s1-ard-identifier-invalid) | error | an identifier breaks the `urn:air` grammar, or two resources share one |
| [AR9S2](/ai-rulez/strict-validation/#ar9s2-ard-entry-invalid) | error | an entry has both or neither of `url` and `data`, or a non-https `url` |
| [AR9S3](/ai-rulez/strict-validation/#ar9s3-ard-queries-out-of-range) | warning | fewer than 2 or more than 5 representative queries |
| [AR9S4](/ai-rulez/strict-validation/#ar9s4-ard-not-declared) | error | the `ard` emitter is requested without an `[ard]` table |

## Hosting

Serve the manifest at `https://<publisher>/.well-known/ard.json` with:

- HTTPS only;
- `Content-Type: application/json`;
- `Access-Control-Allow-Origin: *`, so browser-based agents can read it.

With `base_url`, copy the whole output directory (`ard.json` and `skills/`) to the host.

Other ways to point consumers at the manifest:

- **HTML link.** Add `<link rel="ard" href="https://acme.example/ard.json">` to a page's `<head>`.
- **DNS.** The specification describes DNS-based discovery but does not settle the record format yet. ai-rulez
  writes no DNS record; check the current specification before adding one.

## Not generated

The optional `trustManifest` is not written: no trust framework can verify a Sigstore identity as a domain credential.

## See also

- [MCP server cards](/ai-rulez/mcp-server-card/)
- [Publish](/ai-rulez/publish/)
