# GitHub Actions

The official action is `Goldziher/ai-rulez` at the repository root. It installs a release binary,
checks its SHA-256 against the release's `checksums.txt`, caches the download by OS, architecture
and version, and adds `ai-rulez` to `PATH`. It supports Linux and macOS on amd64/arm64 and Windows
on amd64. No Go, Node or Python installation is needed by the action. Linux requires glibc 2.36
or newer; Alpine/musl and Windows arm64 are unsupported.

The action sets up the CLI and optional LLM configuration. Run commands in subsequent steps;
installation does not generate files, validate content or call a model.

The action honors the requested binary version even when another `ai-rulez` is already on `PATH`.
Windows archives that need `liter_llm_ffi.dll` retain it beside the executable.

## Action ref and binary version

These are two independent choices:

- `uses: Goldziher/ai-rulez@v5` selects the action implementation from the floating **v5 tag**.
  The publish workflow moves it to the commit of each successfully published stable v5 release,
  after GitHub, npm, PyPI and Homebrew publication. Release candidates never move it, and rerunning
  an older release cannot move it backward.
- `with: version: 5.6.2` selects the **CLI binary**. Accepts `X.Y.Z`, `vX.Y.Z`, or `latest`
  (the default). `version: v5` is unsupported: the floating tag is an action ref, not a binary release.
  Omitting `version` downloads the latest public GitHub release, even when the action ref is pinned.

Pin a reviewed full release tag or commit SHA for reproducible action code, and set `version` too
for a reproducible binary. `@v5` intentionally receives compatible action updates; `latest` may
eventually install a newer major CLI. The full `vX.Y.Z` release tags remain unchanged.

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
          version: 5.6.2
      - run: ai-rulez generate --check
      - run: ai-rulez validate --strict
      - run: ai-rulez lock --check
```

See [lock checks in CI](lockfile.md#ci-usage) for the distinction between lock drift and generated
output drift. Command arguments, including `--config`, `--profile`, `--format` and judge-specific
flags, belong on the following `run:` command. The action does not interpret or execute an argument string.

## LLM configuration inputs

All LLM inputs are optional and default to an empty string. A nonempty input sets its corresponding
`AI_RULEZ_LLM_*` variable for later steps in the same job using GitHub's environment file. Empty
inputs preserve existing environment configuration. Inputs are passed as literal values, never
executed or printed. The CLI validates values when an LLM-aware command reads them; setup itself
does not contact a provider or test credentials.

| Input | Environment variable | Value |
| --- | --- | --- |
| `llm-provider` | `AI_RULEZ_LLM_PROVIDER` | Provider, for example `openai` or `bedrock` |
| `llm-model` | `AI_RULEZ_LLM_MODEL` | Model, optionally prefixed with `provider/` |
| `llm-base-url` | `AI_RULEZ_LLM_BASE_URL` | Optional trusted gateway URL |
| `llm-api-key-env` | `AI_RULEZ_LLM_API_KEY_ENV` | **Name** of the variable holding the key; never the key itself |
| `llm-embedding-model` | `AI_RULEZ_LLM_EMBEDDING_MODEL` | Embedding model for search |
| `llm-max-cost-usd` | `AI_RULEZ_LLM_MAX_COST_USD` | Nonnegative USD limit for a run |
| `llm-max-tokens` | `AI_RULEZ_LLM_MAX_TOKENS` | Nonnegative total token limit |
| `llm-max-calls` | `AI_RULEZ_LLM_MAX_CALLS` | Nonnegative provider call limit |
| `llm-timeout-seconds` | `AI_RULEZ_LLM_TIMEOUT_SECONDS` | Timeout including retries, at most 3600 seconds |
| `llm-cache` | `AI_RULEZ_LLM_CACHE` | `true` or `false` |
| `llm-allow-network` | `AI_RULEZ_LLM_ALLOW_NETWORK` | `true` explicitly permits model requests; default remains disabled |
| `llm-allow-plain-http` | `AI_RULEZ_LLM_ALLOW_PLAIN_HTTP` | Explicit plain-HTTP gateway opt-in |
| `llm-plain-http-hosts` | `AI_RULEZ_LLM_PLAIN_HTTP_HOSTS` | Comma-separated exact allowed gateway hosts/ports |
| `llm-price-input-per-mtok` | `AI_RULEZ_LLM_PRICE_INPUT_PER_MTOK` | Nonnegative USD per million input tokens for the configured model |
| `llm-price-output-per-mtok` | `AI_RULEZ_LLM_PRICE_OUTPUT_PER_MTOK` | Nonnegative USD per million output tokens for the configured model |
| `llm-temperature` | `AI_RULEZ_LLM_TEMPERATURE` | `omit` to leave temperature out of requests |

For defaults, budget semantics and provider routing, see [LLM access](llm.md). There is no
`AI_RULEZ_LLM_BACKEND` or `AI_RULEZ_LLM_MAX_RETRIES` override; configure `max_retries` in the user
configuration file. Network and credential settings in a repository config cannot enable requests.

## Credentials and environment variables

Set provider credentials through GitHub secrets on the **command step** or job, or use an OIDC
credential action before running the command. Setting `env:` only on the setup action does not
persist it into a later command step. Job environment variables are inherited by all steps; a
later step's own `env:` can override a variable exported by the action.

```yaml
steps:
  - uses: actions/checkout@v7
    with:
      fetch-depth: 2
  - uses: Goldziher/ai-rulez@v5
    with:
      version: 5.6.2
      llm-provider: openai
      llm-model: gpt-4o-mini
      llm-api-key-env: OPENAI_API_KEY
      llm-allow-network: 'true'
      llm-max-cost-usd: '2.00'
      llm-max-calls: '20'
  - name: Review agent configuration
    env:
      OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
    run: ai-rulez judge --allow-llm --changed=HEAD~1 --format json
```

Fetch enough Git history for the comparison you choose (for example `fetch-depth: 2` on checkout
for `HEAD~1`). `judge` additionally requires `--allow-llm`; the action does not grant that consent.
Only run credentialed model reviews on changes you trust. GitHub does not normally provide secrets
to pull requests from forks.

You can skip the LLM inputs and use the same variables directly:

Run `llm doctor` from a repository with an ai-rulez project configuration. Unlike `judge`, this
diagnostic requires a project; `llm estimate` can run without one.

```yaml
- uses: Goldziher/ai-rulez@v5
  with:
    version: 5.6.2
- name: Inspect LLM configuration without a request
  env:
    AI_RULEZ_LLM_PROVIDER: openai
    AI_RULEZ_LLM_MODEL: gpt-4o-mini
    AI_RULEZ_LLM_API_KEY_ENV: OPENAI_API_KEY
    OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
  run: ai-rulez llm doctor
```

For [Bedrock](llm.md#amazon-bedrock), configure AWS credentials using the AWS SDK credential chain
(including GitHub OIDC), supply `AWS_REGION` on the command step or job, and set
`llm-model: bedrock/<model-or-inference-profile-id>`. No `llm-base-url` or `llm-api-key-env` is
needed for normal AWS authentication. Use ordinary `env:` for provider-specific settings such as
AWS region, profile and bearer-token variables.

## Outputs

| Output | Meaning |
| --- | --- |
| `version` | Resolved installed CLI version, without the leading `v` |
| `path` | Absolute path to the installed binary |

Give the action step `id: setup` to read `${{ steps.setup.outputs.version }}` and
`${{ steps.setup.outputs.path }}`. The CLI is also available by name in later steps.
