# GitHub metadata and release notes template

A maintainer applies these by hand; nothing here is automated. They match the lifecycle positioning in the README.
This page is not in the site navigation.

## Repository description

```text
Standards-compliant lifecycle tool for agent knowledge and capabilities: author, generate, bundle, validate, govern, publish.
```

GitHub limits the description to 350 characters; this is under 130.

## Homepage

```text
https://goldziher.github.io/ai-rulez/
```

## Topics

```text
okf agent-plugins ard agent-skills agents-md llms-txt mcp model-context-protocol sbom cyclonedx spdx sigstore in-toto
supply-chain ai-agents claude-code cli golang
```

Apply with the GitHub CLI:

```bash
gh repo edit Goldziher/ai-rulez \
  --description "Standards-compliant lifecycle tool for agent knowledge and capabilities: author, generate, bundle, validate, govern, publish." \
  --homepage "https://goldziher.github.io/ai-rulez/" \
  --add-topic okf --add-topic agent-plugins --add-topic ard --add-topic agent-skills --add-topic agents-md \
  --add-topic llms-txt --add-topic mcp --add-topic model-context-protocol --add-topic sbom --add-topic cyclonedx \
  --add-topic spdx --add-topic sigstore --add-topic in-toto --add-topic supply-chain --add-topic ai-agents \
  --add-topic claude-code --add-topic cli --add-topic golang
```

GitHub allows at most 20 topics; this is 18.

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
