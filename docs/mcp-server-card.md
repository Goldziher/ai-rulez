# MCP server cards

The `mcp-server-card` emitter writes an [MCP Server Card](https://modelcontextprotocol.io/seps/2127-mcp-server-cards)
(SEP-2127) for each remote MCP server of your project. A card is a small JSON document that a client or registry reads
before it connects: it names the server and lists the endpoints and the headers the client must supply.

!!! warning "Experimental schema"
    The card schema comes from the experimental
    [server-card extension](https://github.com/modelcontextprotocol/experimental-ext-server-card) and can change before a
    versioned schema is published. ai-rulez pins it at commit `526201b`.

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
runtimes = ["claude"]

[plugin.author]
name = "Acme Tooling"

[[mcp_servers]]
name = "acme-search"
description = "Search Acme documentation."
transport = "http"
url = "https://docs.acme.example/mcp"
headers = { Authorization = "Bearer ${DOCS_TOKEN}" }
```

```bash
ai-rulez publish emit mcp-server-card --output-dir cards
```

```text
wrote cards/acme-search.json
```

`cards/acme-search.json`:

```json
{
  "$schema": "https://static.modelcontextprotocol.io/schemas/v1/server-card.schema.json",
  "name": "io.github.acme/acme-search",
  "description": "Search Acme documentation.",
  "version": "1.2.0",
  "remotes": [
    {
      "type": "streamable-http",
      "url": "https://docs.acme.example/mcp",
      "headers": [
        {
          "name": "Authorization",
          "isRequired": true,
          "isSecret": true
        }
      ]
    }
  ]
}
```

The header value (`Bearer ${DOCS_TOKEN}`) is not in the card; the card only says the client must send an
`Authorization` header and that it is a secret.

## Generate

| Command | Output |
| --- | --- |
| `ai-rulez publish emit mcp-server-card [--output-dir DIR]` | only the cards, in `DIR` (default `emit/mcp-server-card/`); no release is built |
| `ai-rulez publish --emit mcp-server-card` | the cards in `dist/emit/mcp-server-card/`, as part of a [release](publish.md) |

Both run the [publish preflight](publish.md#pipeline) first, so the project must be committed and locked
(`ai-rulez lock`), and its plugin bundle up to date. To run the emitter on every publish, declare it in the configuration:

```toml
[[publish.emitters]]
name = "mcp-server-card"
options = { namespace = "com.acme", version = "2.0.0" }   # both optional
```

| Option | Default | Meaning |
| --- | --- | --- |
| `namespace` | see [Name](#what-a-card-holds) | the reverse-DNS prefix of each card's `name` |
| `version` | the `[plugin]` version, else `0.0.0` | the `version` of every card |

The emitter reads the enabled servers of `[[mcp_servers]]` and `[[plugin.mcp]]`. It fails when none of them has a URL.

## What a card holds

| Field | Value |
| --- | --- |
| `name` | `<namespace>/<server>`. Characters outside letters, digits, `.`, `_` and `-` in the server name become `-`. The namespace is the `namespace` option, else the reversed `[ard] publisher` (`acme.example` gives `example.acme`), else derived from the repository: `github.com/acme/...` gives `io.github.acme`, another host such as `git.acme.test/team/...` gives `test.acme.git.team` |
| `description` | the server's `description`, cut to 100 characters, or `MCP server <name>` without one |
| `version` | the `version` option, else the plugin version, else `0.0.0` |
| `remotes[].type` | `streamable-http` for `transport = "http"` (or a `url` with no transport); `sse` for `transport = "sse"` |
| `remotes[].url` | the server URL, as written |
| `remotes[].headers` | one required input per header, by name. `isSecret` is set when the name looks like a credential (such as `Authorization`), or the value is a `${VAR}` placeholder, including one resolved from `.env` |

`title`, `icons`, `repository`, `websiteUrl`, URL variables, `supportedProtocolVersions` and `_meta` are not generated.

## Skipped servers

A card is public, so the emitter never writes a value and skips servers that could leak one. Each skipped server is
reported as a warning; the note never repeats a URL.

| Server | Result |
| --- | --- |
| stdio (a `command`, no `url`) | skipped: a card describes remote endpoints only |
| URL with a query string or a fragment | skipped |
| URL with an unresolved `${...}` placeholder | skipped |
| URL path segment that looks like a credential (a prefix such as `sk-`, `ghp_` or `AKIA`, a known token shape, or an opaque string of 32 or more letters and digits, including a UUID) | skipped |
| URL with credentials (`https://user:pass@host`) | refused with an error |

Environment values are never read into a card.

## With ARD

The [Agentic Resource Discovery](ard.md) manifest lists each remote server as an `application/mcp-server-card+json`
entry whose `data` is the same card. ARD additionally requires an `https` URL.

## See also

- [Publish](publish.md): the release pipeline and the other emitters.
- [Agentic Resource Discovery](ard.md).
