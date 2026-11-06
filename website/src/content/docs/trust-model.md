---
title: "Trust model"
description: "What ai-rulez treats as untrusted in a repository's .ai-rulez/ content, and how scanning, approvals, the lock file and signing contain it."
---

A repository's `.ai-rulez/` directory is content someone else may have written. ai-rulez lets you clone it and run
`ai-rulez generate` without it spending your money, sending your data anywhere or running code you did not ask for,
and gives you tools to pin, review, sign and bound everything it contributes.

The rule: **repository configuration can only tighten. Anything that leaves the machine or starts a process needs
user scope (your user config file or an environment variable) or an explicit flag.**

## Example

A cloned repository tries to send model traffic to its own endpoint:

```toml
# .ai-rulez/config.toml in the checkout
[llm]
allow_network = true
base_url = "https://llm.evil.example/v1"
```

ai-rulez ignores both keys and says so:

```console
$ ai-rulez llm doctor
...
network allowed: false
warning:         repository [llm] keys ignored (user scope only: set them in the user config file ~/.config/ai-rulez/config.toml or via AI_RULEZ_LLM_*): allow_network, base_url

$ ai-rulez validate --strict
.ai-rulez/config.toml:9: warning AR9L1 llm-untrusted-key: llm: repository [llm] keys ignored ...
```

To enable the network for the model, set `allow_network = true` under `[llm]` in your user config file, or
`AI_RULEZ_LLM_ALLOW_NETWORK=1` in your shell.

## Scopes

| Scope | Where | Who controls it |
| --- | --- | --- |
| Repository | `.ai-rulez/config.toml` and the machine-local overlay `config.local.*` (it lives in the checkout, so it counts as repository scope) | whoever can commit to the repository |
| User | `$XDG_CONFIG_HOME/ai-rulez/config.toml`, else `~/.config/ai-rulez/config.toml` | you, on this machine |
| Environment | `AI_RULEZ_*` variables, `DO_NOT_TRACK` | you, in this shell |
| Command line | flags such as `--allow-egress`, `--allow-exec`, `--runner-command`, `--token`, `--policy` | you, for this run |
| Organization | a [policy file](/ai-rulez/policy/) named by `--policy`, `AI_RULEZ_POLICY` or the managed path, never found inside the repository | your security or platform team |

## What a repository can set

| Setting | From the repository | Needs user scope, environment or a flag |
| --- | --- | --- |
| `[llm]` network, endpoint, key and price keys (`allow_network`, `base_url`, `api_key_env`, ...) | **ignored**, reported by `doctor`, `llm doctor` and `AR9L1`. Budget keys are honoured but can only tighten; `provider`, `model` and the other behavior keys are honoured. Per-key list: [LLM trust rule](/ai-rulez/llm/#trust-rule) | user file or `AI_RULEZ_LLM_*`; the environment wins over the user file |
| `[telemetry]` network and export keys (`allow_network`, `otlp_endpoint`, `headers_env`, ...) | **ignored**, reported by `telemetry doctor` and `AR9K1`; `enabled` and `sample` are honoured (a repository can at most switch on the local log). Per-key list: [telemetry trust rule](/ai-rulez/telemetry/#the-trust-rule) | user file, `AI_RULEZ_TELEMETRY_*`, or the consent record `ai-rulez telemetry enable` writes. `AI_RULEZ_TELEMETRY=off` and `DO_NOT_TRACK=1` override everything |
| Git credentials | **no key exists** | `AI_RULEZ_GIT_TOKEN` or `--token` (see [Credentials](#credentials)) |
| External scanners (`[[lint.external]]`) | may declare a scanner and its `egress` | nothing runs without `validate --external`; an `egress = true` scanner also needs `--allow-egress=<name>` (`AR9E4`) |
| `[search.embeddings] command`, `[search] log_queries` | **ignored** with a warning; the other `[search]` keys only change ranking and are honoured | user file; `--allow-exec` honours a repository `command` for one run; the MCP server never honours it. See [Skill search](/ai-rulez/search/#data-egress) |
| Verifiers (`[[verifiers]]`) | content predicates run; `command` and `llm` predicates may be declared | `verifiers run --allow-exec` (or `AI_RULEZ_VERIFIERS_ALLOW_EXEC=1`) runs commands; `--allow-llm` plus user-scope `[llm] allow_network` calls a model. A verifier from an include runs only when `[verifiers_settings] trust_exec_from` names it and the lock pins it. `generate`, `validate`, hooks and the MCP server never do either |
| Evals | cases are repository files | `--runner-command` picks the command; `--allow-exec` lets `command_exit` assertions run |
| `[[hooks]]` | written into the harness settings files by `generate`; pinned by the [lock](/ai-rulez/lockfile/) and scanned | review the pull request; `generate` lists new or changed commands and asks you to review them (silence with `--yes` or `AI_RULEZ_ACK_COMMANDS=1`); `generate --user` installs hooks from your user config |
| `[[mcp_servers]]` | written into the harness MCP files by `generate`; pinned as `settings:mcp-servers` and listed in the same command acknowledgment | review the pull request; an organization policy bounds them with `mcp.allowed_commands` and `mcp.deny_transports` |
| Remote sources (`[[includes]]`, `[[installed_skills]]`, `[[skill_sources]]`) | may name a source over https, ssh or a local path (`http://` is rejected) | a content change is caught by the lock digest, and imported content is security-scanned before it is written |
| `[lock]`, `[governance]`, `[signing]` | honoured, and editable by any pull request | an [organization policy](/ai-rulez/policy/) sets a floor the repository cannot lower. `verify --public-key` and `--identity`/`--issuer` only add signers; restricting who may sign takes a policy `[[signing.trust]]` list or `allow_repo_identities = false` |
| Organization policy | **never read** from the repository | `--policy`, `AI_RULEZ_POLICY`, or `/etc/ai-rulez/policy.toml` (and the macOS and Windows equivalents) |

A repository therefore cannot turn on network access for the model or telemetry, give itself credentials, or make a
scanner, verifier or eval run a program.

## Credentials

- **Git token** (`AI_RULEZ_GIT_TOKEN` or `--token`) for private includes and installed skills. It is sent only to
  `github.com`, or to the comma-separated hosts in `AI_RULEZ_GIT_TOKEN_HOSTS` (environment only; replaces the
  default). It travels as a per-command `Authorization` header, so it is not in the URL, the process arguments or the
  cached clone's `.git/config`. A source on another https host gets no token and a warning; `http://`, `git://`, ssh
  and file sources never get it. Skill sources run git with no credential helper and no prompt: use public
  repositories or an SSH agent.
- **GitHub API token** (`GITHUB_TOKEN`, `GH_TOKEN` or `gh auth token`) for release dates, review-linked approvals and `--resolve-teams`.
  It is a separate grant, sent only to `github.com` or the hosts in `AI_RULEZ_FORGE_HOSTS`. See
  [Forge client](/ai-rulez/forge/).

Use tokens with the least scope that can read what you include.

## Supply-chain controls

The trust rule keeps a checkout from acting on its own. The governance features make every change to what your
assistants are told explicit, reviewable and attributable:

| Question | Control | Page |
| --- | --- | --- |
| Did anything change since it was reviewed? | `ai-rulez.lock` pins every remote source, authored item and generated output by digest; `lock --check` and `generate --locked` fail on drift | [Lock file](/ai-rulez/lockfile/) |
| Who reviewed this content? | `ai-rulez approve` records a reviewer against a digest; `[governance]` makes content unusable until approved | [Approvals](/ai-rulez/approvals/) |
| Who vouches for the lock? | `ai-rulez sign` writes a Sigstore bundle; `verify --attestation` checks it offline against named signers | [Signing](/ai-rulez/signing/) |
| What may a repository never loosen? | an organization policy outside the repository clamps `[lock]`, `[governance]`, `[signing]`, sources, lint severities and more | [Organization policy](/ai-rulez/policy/) |
| What is the configuration made of? | `ai-rulez sbom` writes CycloneDX or SPDX for security tooling | [SBOM](/ai-rulez/sbom/) |
| How does ai-rulez talk to GitHub? | read-only, allowlisted hosts, token from the environment only | [Forge client](/ai-rulez/forge/) |

Each layer is an assertion of a different strength. A digest proves bytes did not change; an approval is a recorded
claim that someone looked; a signature proves which identity produced the lock; a policy holds only if it is set from
outside the repository. None of them judges whether content is safe: pair them with the
[security checks](/ai-rulez/strict-validation/#security-checks) and human review.

## Limits

- The checks cover the configuration ai-rulez reads and the files it generates. A hand-written `.claude/settings.json`,
  `.mcp.json`, `.cursor/mcp.json` or other harness file committed to the repository is outside its control: review
  those like code, and set `AI_RULEZ_TELEMETRY=off` or `DO_NOT_TRACK=1` when you open a checkout you do not trust.
- A `telemetry record --sink-command` is code you chose to run; it runs with a 3 second timeout and an output cap.
- `[lock]`, `[governance]` and `[signing]` in the repository are changed by the same pull request they gate. Protect
  them with branch protection and code owners, or enforce them with an [organization policy](/ai-rulez/policy/).
