---
title: "GitHub Actions"
description: "Run ai-rulez in GitHub Actions: install a verified binary, gate pull requests on drift and validation, and open scheduled pin-update pull requests."
---

ai-rulez ships three composite actions:

| Action | What it does |
| --- | --- |
| `Goldziher/ai-rulez@v5` | installs the CLI and, optionally, exports LLM settings for later steps |
| `Goldziher/ai-rulez/check@v5` | installs the CLI and runs the full governance gate on every configuration root |
| `Goldziher/ai-rulez/update@v5` | installs the CLI, moves version-range pins with `ai-rulez update`, and opens a pull request |

All three download a release binary, verify its SHA-256 against the release's `checksums.txt`, cache it by OS,
architecture and version, and put `ai-rulez` on `PATH`. They need no Go, Node or Python.

| Platform | Supported |
| --- | --- |
| Linux amd64, arm64 | yes (glibc 2.36 or newer; Alpine/musl is not supported) |
| macOS amd64, arm64 | yes |
| Windows amd64 | yes |
| Windows arm64 | no |

## Quick start

```yaml
name: Check agent configuration
on:
  pull_request:
  push:
    branches: [main]
permissions:
  contents: read
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: Goldziher/ai-rulez@v5
        with:
          version: 5.11.1
      - run: ai-rulez generate --check
      - run: ai-rulez validate --strict
      - run: ai-rulez lock --check
```

The setup action only installs the CLI; it does not generate, validate or call a model. Put every command and its
flags (`--config`, `--profile`, `--format`, ...) in later `run:` steps. See [lock checks in CI](/ai-rulez/lockfile/#ci-usage)
for the difference between lock drift and generated-output drift.

## Action ref and CLI version

`uses:` and `with: version` are independent choices:

- **Action ref** (`@v5`): the action code. `v5` is a floating tag that moves to each stable 5.x release once it is
  fully published. Release candidates never move it. Pin a full release tag (`@v5.10.0`) or a commit SHA for
  reproducible action code.
- **`version`**: the CLI binary. Accepts `X.Y.Z`, `vX.Y.Z`, a release candidate `X.Y.Z-rc.N`, or `latest` (the
  default). `latest` resolves to the newest published stable release, never a draft or release candidate. `v5` is not
  a valid `version`: it is an action ref, not a release.

Omitting `version` installs the latest release even when the action ref is pinned, and `latest` may eventually install
a newer major version. Set `version` for a reproducible binary.

The action installs the requested version even when another `ai-rulez` is already on `PATH`.

### Inputs and outputs of the setup action

| Input | Default | Meaning |
| --- | --- | --- |
| `version` | `latest` | CLI version to install |
| `llm-*` | empty | LLM settings exported to later steps; see [LLM configuration inputs](#llm-configuration-inputs) |

| Output | Meaning |
| --- | --- |
| `version` | the installed CLI version, without the leading `v` |
| `path` | absolute path to the installed binary |

Give the step an `id` to read them:

```yaml
- id: setup
  uses: Goldziher/ai-rulez@v5
- run: echo "ai-rulez ${{ steps.setup.outputs.version }} at ${{ steps.setup.outputs.path }}"
```

## Governance gate: `check`

`Goldziher/ai-rulez/check@v5` installs the CLI the same way and runs the whole gate, so a workflow does not have to list
the commands:

```yaml
name: ai-rulez
on:
  pull_request:
  push:
    branches: [main]
permissions:
  contents: read
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: Goldziher/ai-rulez/check@v5
        with:
          version: 5.11.1
          expected-config-count: "2"   # the root and one module
          plugin: "true"
```

For every configuration root, in its own directory, it runs:

1. `ai-rulez migrate okf --check` (skipped for a `source = "agents"` project);
2. `ai-rulez validate --strict --no-local`;
3. `ai-rulez lock --check`, when the root has an `ai-rulez.lock`;
4. `ai-rulez generate --strict-config --no-local`, then `ai-rulez generate --check --strict-config --no-local`;
5. `ai-rulez verify --plugin`, with `plugin: true`;
6. `ai-rulez tokens --budget N --no-local`, with `token-budget: N`.

Then `git diff --exit-code HEAD` must be clean and generation must have left no new untracked file. The gate stops at
the first failing command, and fails when the checkout has no tracked files, so an empty or wrong checkout cannot pass.
It writes a job summary listing each command and its result.

| Input | Default | Meaning |
| --- | --- | --- |
| `version` | `latest` | CLI version, as for the setup action |
| `config-directories` | discover | Newline-separated configuration directories (`.ai-rulez`, `.agents`, `.config/ai-rulez`, or the project root holding one), relative to the workspace. Empty discovers every **tracked** `.ai-rulez/config.toml`, `.config/ai-rulez/config.toml` and `.agents/ai-rulez.toml` |
| `expected-config-count` | none | Fail unless exactly this many roots are checked, so a renamed or deleted module fails instead of silently checking fewer |
| `plugin` | `false` | Also run `verify --plugin` |
| `strict` | `true` | `false` drops `--strict` from `validate` and `--strict-config` from `generate` |
| `local` | `false` | `true` reads the machine-local overlay; `false` passes `--no-local` |
| `token-budget` | none | Run `tokens --budget N`: fail when the always-loaded token count exceeds `N` |

Booleans accept only `true` or `false`, counts only positive integers, and directories only relative paths without
`..`.

| Output | Meaning |
| --- | --- |
| `version` | the installed CLI version |
| `roots` | the number of configuration roots checked |
| `files-examined` | the number of tracked files the clean-checkout check examined |

`token-budget` is one ceiling on the always-loaded count. A per-harness budget and the MCP tool-schema cost are not
measured; run `ai-rulez tokens` in a step for those.

## Scheduled pin updates: `update`

Dependabot and Renovate do not read `ai-rulez.lock`, and nothing runs `ai-rulez update` on its own.
`Goldziher/ai-rulez/update@v5` runs it on a schedule and moves the version-range pins (`version = "^1.2"`) of includes,
installed skills, skill sources, marketplaces and plugins:

```yaml
name: ai-rulez update
on:
  schedule:
    - cron: "17 6 * * 1"   # Mondays
  workflow_dispatch:
permissions:
  contents: write
  pull-requests: write
jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: Goldziher/ai-rulez/update@v5
        with:
          version: 5.11.1
```

The action:

1. runs `ai-rulez lock --outdated --format json`, and stops with a notice (`updated: false`) when no source has an
   update its constraint allows;
2. runs `ai-rulez update`. The pre-pin security scan applies: a refused source fails the job with its `AR` code and
   nothing is committed;
3. runs `ai-rulez generate --no-local`, then `ai-rulez lock --content-only`, and stops with a notice when nothing
   changed;
4. commits every change as `github-actions[bot]`, pushes it to `branch`, and opens a pull request into the checked-out
   branch (or refreshes the open one). The pull request body lists each source with its kind, locked tag and new tag.
   With `direct: true` it pushes the commit to the checked-out branch instead.

| Input | Default | Meaning |
| --- | --- | --- |
| `version` | `latest` | CLI version, as for the setup action |
| `names` | all | Sources to update, separated by spaces or newlines |
| `kind` | all | `include`, `skill`, `source`, `marketplace` or `plugin` |
| `direct` | `false` | `true` pushes to the checked-out branch instead of opening a pull request |
| `branch` | `ai-rulez/update` | Pull request branch. The action owns it and replaces it on each run with `--force-with-lease`, so a push by someone else in between fails the run instead of being lost |
| `token` | `github.token` | Token for `git push` and `gh`. It reaches git and `gh` through environment variables, never the command line. When `GITHUB_TOKEN` is unset it is also exported as `GITHUB_TOKEN`, which `ai-rulez` uses to read github.com release dates for `min_release_age` |
| `working-directory` | `.` | Project root holding the configuration, relative to the workspace |

| Output | Meaning |
| --- | --- |
| `version` | the installed CLI version |
| `updated` | `true` when a commit was pushed |
| `commit` | SHA of the pushed commit |
| `pull-request-url` | URL of the opened or refreshed pull request (pull request mode only) |

Requirements and caveats:

- The job needs `contents: write`, plus `pull-requests: write` in pull request mode.
- The checkout must be on a branch (not a detached HEAD) with no uncommitted changes.
- A pull request opened with the default `GITHUB_TOKEN` does not start other workflows. Pass a GitHub App or
  fine-grained token as `token` when the pull request must run CI.
- To hold back tags that are too new, set `min_release_age` on each source in the configuration; the action has no
  input for it.

## LLM configuration inputs

The setup action can export LLM settings for later steps in the same job. Every input is optional. A nonempty input sets
its `AI_RULEZ_LLM_*` variable through `GITHUB_ENV`; an empty input leaves the existing environment alone. Values are
passed literally, never executed or printed. The action does not contact a provider; the CLI validates the values when
an LLM-aware command reads them.

| Input | Environment variable | Value |
| --- | --- | --- |
| `llm-provider` | `AI_RULEZ_LLM_PROVIDER` | Provider, for example `openai` or `bedrock` |
| `llm-model` | `AI_RULEZ_LLM_MODEL` | Model, optionally prefixed with `provider/` |
| `llm-base-url` | `AI_RULEZ_LLM_BASE_URL` | Gateway or local model server URL |
| `llm-api-key-env` | `AI_RULEZ_LLM_API_KEY_ENV` | The **name** of the variable holding the key, never the key itself |
| `llm-embedding-model` | `AI_RULEZ_LLM_EMBEDDING_MODEL` | Embedding model for search |
| `llm-max-cost-usd` | `AI_RULEZ_LLM_MAX_COST_USD` | Maximum estimated spend in USD per run; `0` is unlimited |
| `llm-max-tokens` | `AI_RULEZ_LLM_MAX_TOKENS` | Maximum total prompt and completion tokens; `0` is unlimited |
| `llm-max-calls` | `AI_RULEZ_LLM_MAX_CALLS` | Maximum provider calls; `0` is unlimited |
| `llm-timeout-seconds` | `AI_RULEZ_LLM_TIMEOUT_SECONDS` | Per-call timeout including retries, at most 3600 |
| `llm-cache` | `AI_RULEZ_LLM_CACHE` | `true` or `false` |
| `llm-allow-network` | `AI_RULEZ_LLM_ALLOW_NETWORK` | `true` permits model requests; unset leaves them disabled |
| `llm-allow-plain-http` | `AI_RULEZ_LLM_ALLOW_PLAIN_HTTP` | `true` allows credentials over plain HTTP to the hosts in `llm-plain-http-hosts` |
| `llm-plain-http-hosts` | `AI_RULEZ_LLM_PLAIN_HTTP_HOSTS` | Comma-separated `host` or `host:port` allowlist for plain HTTP |
| `llm-price-input-per-mtok` | `AI_RULEZ_LLM_PRICE_INPUT_PER_MTOK` | Input price override, USD per million tokens |
| `llm-price-output-per-mtok` | `AI_RULEZ_LLM_PRICE_OUTPUT_PER_MTOK` | Output price override, USD per million tokens |
| `llm-temperature` | `AI_RULEZ_LLM_TEMPERATURE` | `omit` to leave temperature out of requests |

There is no input for retries; set `max_retries` in the user configuration. Network and credential settings in a
repository configuration cannot enable requests. For defaults, budgets and provider routing, see [LLM access](/ai-rulez/llm/).

## Credentials

Set the provider key on the **command step** (or the job), not on the setup action: `env:` on the setup step does not
reach later steps.

```yaml
steps:
  - uses: actions/checkout@v7
    with:
      fetch-depth: 2                 # history for --changed=HEAD~1
  - uses: Goldziher/ai-rulez@v5
    with:
      version: 5.11.1
      llm-provider: openai
      llm-model: gpt-4o-mini
      llm-api-key-env: OPENAI_API_KEY
      llm-allow-network: "true"
      llm-max-cost-usd: "2.00"
      llm-max-calls: "20"
  - name: Review agent configuration
    env:
      OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
    run: ai-rulez judge --allow-llm --changed=HEAD~1 --format json
```

- `judge` also needs `--allow-llm`; the action does not grant that consent.
- Run credentialed model reviews only on changes you trust. GitHub does not normally give secrets to pull requests from
  forks.
- Job-level `env:` reaches every step; a step's own `env:` overrides a variable the action exported.

You can skip the `llm-*` inputs and set the variables directly. `llm doctor` checks the configuration without making a
request; it must run in a repository with an ai-rulez configuration.

```yaml
- uses: Goldziher/ai-rulez@v5
  with:
    version: 5.11.1
- name: Inspect LLM configuration
  env:
    AI_RULEZ_LLM_PROVIDER: openai
    AI_RULEZ_LLM_MODEL: gpt-4o-mini
    AI_RULEZ_LLM_API_KEY_ENV: OPENAI_API_KEY
    OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
  run: ai-rulez llm doctor
```

For [Bedrock](/ai-rulez/llm/#amazon-bedrock), configure AWS credentials through the AWS SDK credential chain (GitHub OIDC
works), set `AWS_REGION` on the command step or job, and use `llm-model: bedrock/<model-or-inference-profile-id>`.
`llm-base-url` and `llm-api-key-env` are not needed.

## See also

- [Lock file](/ai-rulez/lockfile/#ci-usage)
- [Strict validation](/ai-rulez/strict-validation/)
- [Publish](/ai-rulez/publish/), for building release artifacts in CI
