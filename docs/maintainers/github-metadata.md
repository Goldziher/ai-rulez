# GitHub metadata and release notes template

A maintainer applies these by hand; nothing here is automated. They match the lifecycle positioning in the README.
This page is not in the site navigation.

## Repository description

```text
Author, generate, validate and govern AI agent rules, skills, plugins and MCP configuration across 52 coding assistants. Review with verified model evidence, evaluate skills, lock and sign dependencies, and publish bundles. CLI, MCP server and Go API.
```

GitHub limits the description to 350 characters; this description is within that limit.

## Homepage

```text
https://goldziher.github.io/ai-rulez/
```

## Topics

```text
okf agent-plugins ard agent-skills agents-md llms-txt mcp model-context-protocol sbom cyclonedx spdx sigstore in-toto
supply-chain ai-agents claude-code cli golang ai-governance llm-evaluation
```

Apply with the GitHub CLI:

```bash
gh repo edit Goldziher/ai-rulez \
  --description "Author, generate, validate and govern AI agent rules, skills, plugins and MCP configuration across 52 coding assistants. Review with verified model evidence, evaluate skills, lock and sign dependencies, and publish bundles. CLI, MCP server and Go API." \
  --homepage "https://goldziher.github.io/ai-rulez/" \
  --add-topic okf --add-topic agent-plugins --add-topic ard --add-topic agent-skills --add-topic agents-md \
  --add-topic llms-txt --add-topic mcp --add-topic model-context-protocol --add-topic sbom --add-topic cyclonedx \
  --add-topic spdx --add-topic sigstore --add-topic in-toto --add-topic supply-chain --add-topic ai-agents \
  --add-topic claude-code --add-topic cli --add-topic golang \
  --add-topic ai-governance --add-topic llm-evaluation
```

GitHub allows at most 20 topics; this uses all 20.

## Release notes template

Keep a Changelog sections stay in `CHANGELOG.md`; the GitHub release carries this shorter form. Replace the
placeholders, and take the standards table from [docs/standards.md](../standards.md) so the pinned versions are the ones
the release was tested against.

```markdown
## ai-rulez vX.Y.Z

One sentence on what this release changes for a user.

### Highlights

- Lifecycle stage and what is new in it (author, generate, bundle, validate, govern, publish).
- ...

### Standards

| Standard | Pinned spec | Change in this release |
| -------- | ----------- | ---------------------- |
| OKF | 0.2 | none |
| Agent Plugins | 1.0.0 | none |
| ARD | 0.91 | none |

### Breaking changes

- None. (Or: the change and the line of `ai-rulez migrate` that handles it; see docs/migration-v5.md.)

### Upgrade

    npx ai-rulez@latest generate --check

### Verify the download

    ai-rulez verify --self

Full changelog: https://github.com/Goldziher/ai-rulez/blob/main/CHANGELOG.md
```
