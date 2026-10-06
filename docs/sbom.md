# SBOM

`ai-rulez sbom` prints a [CycloneDX](https://cyclonedx.org/) 1.6 JSON bill of materials of the project's AI
configuration. Instructions an agent follows are a supply chain; the SBOM lists it in the format security tooling
already ingests.

```bash
ai-rulez sbom --format cyclonedx > ai-bom.cdx.json
ai-rulez sbom -o ai-bom.cdx.json
```

Flags: `--format cyclonedx` (the only format), `-o`/`--output file`, `-n`/`--config-dir name`. Nothing is rendered and
nothing is written except `--output`. The machine-local overlay (`config.local.*`, `local/`) is never included, so
the document is the same on every checkout.

## What it lists

| CycloneDX | Content |
| --- | --- |
| `components[]`, type `data` | Every rule, context file, skill, agent, command, check, hook and role (and the permission and managed-settings pins), with `ai-rulez:kind`, `ai-rulez:domain`, `ai-rulez:path`, `ai-rulez:digest`; `version` and `ai-rulez:owner` from the item's frontmatter |
| `components[]`, type `data` | Each remote include, installed skill and `[[skill_sources]]` entry: `purl`, `vcs` external reference, `ai-rulez:ref`, and the commit and digest pinned in `ai-rulez.lock` |
| `components[]`, type `application` | Each local MCP server (command based): name, command executable, enabled, profiles, env key names, and a heuristic `purl` |
| `services[]` | Each remote MCP server (`url`): redacted endpoint, `authenticated` when headers are set, `x-trust-boundary` |
| `metadata.component` | The project, with `ai-rulez:tree`, `ai-rulez:lock` (`present`/`absent`) and `ai-rulez:lock-in-sync` |
| `dependencies[]` | The project depends on every component and service |

Items come from the same digests as [`ai-rulez.lock`](lockfile.md), computed from the sources (CRLF normalised), so the
SBOM digest of an item equals the lock's digest of it. `ai-rulez:lock-in-sync` is `false` when the lock no longer
matches the sources.

### Package URLs

Remote sources: `pkg:github/<owner>/<repo>@<commit or ref>#<path>` (also `gitlab`, `bitbucket`); any other host is
`pkg:generic/<name>@<version>?vcs_url=git+https://host/path`. A local path or `file://` source gets no purl and its
path is not emitted.

MCP servers are guessed from the launcher, and marked `ai-rulez:purl-source = heuristic`:

| Command | Purl |
| --- | --- |
| `npx`, `bunx`, `pnpm dlx`, `yarn dlx` | `pkg:npm/%40scope/name@version` |
| `uvx`, `pipx run`, `uv tool run` | `pkg:pypi/name@version` (`==` or `@` pins, `--from`) |
| `docker run`, `podman run` | `pkg:oci/name@sha256:...?repository_url=...&tag=...` |
| `go run module@version` | `pkg:golang/module@version` |

Anything else (a binary path, a local script, a package URL or tarball) gets no purl. Arguments are only read to find
the package and are never emitted.

## Determinism

No `timestamp`. Components, services, properties and dependencies are sorted; the same project yields the same bytes on
every run, operating system and line ending. The `serialNumber` is a UUIDv5 over the lock tree (the `tree` of
`ai-rulez.lock` when it pins content, otherwise the tree computed from the sources and the remote pins) and the list
of component references, so it changes when the content, an MCP server or a source is added or removed, and not
otherwise. `metadata.tools` records the ai-rulez version, so the bytes also change when you upgrade.

## No secrets

- Environment and header **values** of MCP servers are never read into the document; only the key names are listed
  (`ai-rulez:env-keys`, `ai-rulez:header-keys`).
- URLs lose userinfo, query and fragment (`https://u:p@host/mcp?key=x` becomes `https://host/mcp`). A URL that is a
  `${VAR}` placeholder is omitted. A secret inside the URL path cannot be told from the path and is kept: do not put
  one there.
- The digest of the generated MCP settings is left out, because it covers env and header values.
- ai-rulez digests are `sha256` tree digests of ai-rulez's own scheme, not file hashes, so they appear only in
  `ai-rulez:`-namespaced properties and never in CycloneDX `hashes`.

## Validating

The output validates against the CycloneDX 1.6 JSON schema (`bom-1.6.schema.json`). The repository's tests do this
against a vendored copy under `internal/sbom/testdata`; any CycloneDX tool accepts the file.

## Design decisions

- **Format and version**: CycloneDX 1.6 JSON only. SPDX is not offered.
- **Item component type**: `data`. Rules, skills and the like are content, not software.
- **Serial number**: derived, not random, so a diff of two SBOMs of an unchanged project is empty. The component
  references are mixed in because MCP servers are not pinned by the lock.
- **MCP purls are guesses**: they identify what the launcher would download, not what is installed. They are marked
  as heuristic so a scanner or reviewer can treat them accordingly.
- **Settings pins**: the `mcp-servers` settings pin is omitted (secrets), every other settings pin is listed.
- **Findings** (`AR75x`) are not emitted in this slice; the block stays reserved in
  [strict validation](strict-validation.md#code-ranges).
