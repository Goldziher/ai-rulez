# MCP server cards

ai-rulez can write an [MCP Server Card](https://modelcontextprotocol.io/seps/2127-mcp-server-cards) (SEP-2127, Final) for
each remote MCP server of the project: a small JSON document a client reads before it connects, naming the server and the
endpoints to connect to.

!!! warning "The schema comes from an experimental extension"
    The card schema is `schema.json` of the experimental
    [server-card extension](https://github.com/modelcontextprotocol/experimental-ext-server-card), generated from
    `schema.ts`. ai-rulez pins it at commit `526201b` (Apache-2.0), vendored under `tests/conformance/schemas/mcp-server-card`,
    and the conformance tests check every card against it. The shape can change before a versioned schema is published.

## Generate

```bash
ai-rulez publish --emit mcp-server-card              # dist/emit/mcp-server-card/<server>.json, in the release
ai-rulez publish emit mcp-server-card --output-dir cards   # only the cards, without a release
```

The emitter is also available as `[[publish.emitters]] name = "mcp-server-card"`.

```toml
[[mcp_servers]]
name = "docs"
description = "Search the docs."
transport = "http"
url = "https://docs.acme.example/mcp"
headers = { Authorization = "Bearer ${DOCS_TOKEN}" }
```

becomes `docs.json`:

```json
{
  "$schema": "https://static.modelcontextprotocol.io/schemas/v1/server-card.schema.json",
  "name": "io.github.acme/docs",
  "description": "Search the docs.",
  "version": "1.4.0",
  "remotes": [
    {
      "type": "streamable-http",
      "url": "https://docs.acme.example/mcp",
      "headers": [{ "name": "Authorization", "isRequired": true, "isSecret": true }]
    }
  ]
}
```

## What a card holds

- **Remote servers only.** A card describes endpoints, not commands, so a stdio server gets no card; the emitter reports
  each one it skips. A server with `transport = "http"` (or a `url` and no transport) is `streamable-http`; `sse` stays `sse`.
- **No secrets.** Header values and environment values are never written. Each header becomes a required input named after
  the header, marked `isSecret` when its name is credential-like (such as `Authorization`), it is listed as secret, or its
  value is a `${VAR}` reference. A URL with credentials is refused.
- **Name.** `<namespace>/<server>`, where the server part has any character outside letters, digits, `.`, `_` and `-`
  replaced by `-`. The namespace is the emitter option `namespace`, else the reversed `[ard]` publisher (`acme.example`
  is `example.acme`), else derived from the repository (`owner/name` and `github.com/owner/name` are `io.github.owner`; `host/owner/name` on another host is the reversed host followed by the owner, such as `test.acme.git.acme`).
- **Version.** The emitter option `version`, else the plugin version, else `0.0.0`.
- **Description.** The server's description cut to the schema's 100 characters, or `MCP server <name>` without one.

## With ARD

The [ARD](ard.md) manifest lists each remote server as an `application/mcp-server-card+json` entry whose `data` is the same
card, built by the same code; ARD additionally requires an `https` URL. Stdio servers are left out of the manifest.

## Not generated

`title`, `icons`, `repository`, `websiteUrl`, URL variables, `supportedProtocolVersions` and `_meta` are not generated
yet, and `validate` does not read cards back; the conformance tests are the check.
