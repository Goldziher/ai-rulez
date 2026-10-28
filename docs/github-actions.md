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

- `uses: Goldziher/ai-rulez@v5` selects the action implementation from the floating **v5 tag**,
  an annotated tag that the publish workflow moves to the commit of each successfully published
  stable 5.x release, after GitHub, npm, PyPI and Homebrew publication. The tag is derived from
  the release's major version. Release candidates never move it, and rerunning an older release
  cannot move it backward.
- `with: version: 5.9.2` selects the **CLI binary**. Accepts `X.Y.Z`, `vX.Y.Z`, a release
  candidate `X.Y.Z-rc.N`, or `latest` (the default). `latest` follows GitHub's
  `releases/latest` redirect, which names the newest published stable release (never a draft or a
  release candidate), and the download is then checked against that release's `checksums.txt`.
  `version: v5` is unsupported: the floating tag is an action ref, not a binary release. Omitting
  `version` downloads the latest release, even when the action ref is pinned.

To follow every 5.x release with no per-release edits, combine the two floating choices:

```yaml
- uses: Goldziher/ai-rulez@v5
  with:
    version: latest
```

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
          version: 5.9.2
      - run: ai-rulez generate --check
      - run: ai-rulez validate --strict
      - run: ai-rulez lock --check
```

See [lock checks in CI](lockfile.md#ci-usage) for the distinction between lock drift and generated
output drift. Command arguments, including `--config`, `--profile`, `--format` and judge-specific
flags, belong on the following `run:` command. The action does not interpret or execute an argument string.

## Governance gate: `check`

`Goldziher/ai-rulez/check@v5` installs the CLI the same way as the root action (the same
resolve, cache and checksum-verified download scripts) and runs the full gate, so a workflow does
not have to assemble it from single commands:

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
          version: 5.9.2
          expected-config-count: "2" # the root and one module
```

For every configuration root, in order, it runs:

1. `ai-rulez migrate okf --check` (skipped for a `source = "agents"` project, which has no
   `.ai-rulez` content tree to convert);
2. `ai-rulez validate --strict --no-local`;
3. `ai-rulez lock --check`, when the root has an `ai-rulez.lock`;
4. `ai-rulez generate --strict-config --no-local`, then `ai-rulez generate --check --strict-config --no-local`;
5. `ai-rulez verify --plugin` with `plugin: true`, and `ai-rulez tokens --budget N --no-local` with
   `token-budget: N`.

Then `git diff --exit-code HEAD` must be clean and generation must have left no new untracked file.
The gate fails when the checkout has no tracked files, so an empty or wrong checkout cannot pass. It
stops at the first failing command and writes a job summary listing every command with its result,
the number of roots checked and the number of tracked files examined.

| Input | Default | Meaning |
| --- | --- | --- |
| `version` | `latest` | CLI version, as for the root action |
| `config-directories` | discover | Newline-separated configuration directories (`.ai-rulez`, `.agents`, `.config/ai-rulez`, or the project root holding one), relative to the workspace. Empty discovers every **tracked** `.ai-rulez/config.toml`, `.config/ai-rulez/config.toml` and `.agents/ai-rulez.toml` |
| `expected-config-count` | none | Fail unless exactly this many roots are checked, so a renamed or deleted module fails instead of silently checking fewer roots |
| `plugin` | `false` | Also run `verify --plugin` |
| `strict` | `true` | `false` drops `--strict` from `validate` and `--strict-config` from `generate` |
| `local` | `false` | `true` reads the machine-local overlay; `false` passes `--no-local` |
| `token-budget` | none | Run `tokens --budget N`: fail when the always-loaded token count exceeds `N` |

Booleans accept only `true` or `false`, counts only positive integers, and directories only
relative paths without `..`. Outputs: `version`, `roots` (the number checked) and
`files-examined`. Each command runs in its project root with configuration discovery, as a local
run there would. `tokens --budget` is a single ceiling on the headline always-loaded count; a
per-harness budget (`claude=N,codex=N`) and the MCP tool-schema cost are not measured.

## Scheduled pin updates: `update`

`ai-rulez update` moves version-range pins (`version = "^1.2"`) of includes, installed skills,
skill sources, marketplaces and plugins, and nothing runs it on its own; Dependabot and Renovate do
not read `ai-rulez.lock`. `Goldziher/ai-rulez/update@v5` runs it on a schedule:

```yaml
name: ai-rulez update
on:
  schedule:
    - cron: "17 6 * * 1" # Mondays
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
          version: 5.9.2
```

In order, it:

1. runs `ai-rulez lock --outdated --format json` and stops with a notice (`updated: false`) when no
   source has an update its constraint allows;
2. runs `ai-rulez update`. The pre-pin security scan applies: a refused source fails the job with
   its `AR` code and nothing is committed;
3. runs `ai-rulez generate --no-local`, then `ai-rulez lock --content-only`;
4. commits every change in the project as `github-actions[bot]` and, by default, pushes it to
   `branch` and opens a pull request into the checked-out branch, or refreshes the open one. The
   PR body holds the outdated table (kind, name, source, locked tag, new tag). With `direct: true`
   it pushes the commit to the checked-out branch instead, with the same list in the commit message.

| Input | Default | Meaning |
| --- | --- | --- |
| `version` | `latest` | CLI version, as for the root action |
| `names` | all | Sources to update, separated by spaces or newlines (passed to `lock --outdated` and `update`) |
| `kind` | all | `include`, `skill`, `source`, `marketplace` or `plugin` |
| `direct` | `false` | `true` pushes to the checked-out branch instead of opening a pull request |
| `branch` | `ai-rulez/update` | Pull request branch. The action owns it: each run replaces it with `--force-with-lease` from the value it read, so a push by someone else in between fails the run instead of being lost |
| `token` | `github.token` | Token for `git push` and `gh`. It reaches git through `GIT_CONFIG_*` environment variables and `gh` through `GH_TOKEN`, never the command line or a file. When `GITHUB_TOKEN` is unset it is also exported as `GITHUB_TOKEN`, which `ai-rulez` uses only for github.com release dates (`min_release_age`) |
| `working-directory` | `.` | Project root holding the configuration, relative to the workspace |

Outputs: `version`, `updated` (`true` when a commit was pushed), `commit` and
`pull-request-url`. The job needs `contents: write`, and `pull-requests: write` for the pull
request mode. The checkout must be on a branch (not detached) with no uncommitted changes. A
pull request opened with the default `GITHUB_TOKEN` does not start other workflows; pass a GitHub
App or fine-grained token as `token` when the pull request must run CI. There is no
`min-release-age` input: `ai-rulez update` has no such flag, and each source's `min_release_age`
in the configuration already holds back tags that are too new (`AR733`).

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
      version: 5.9.2
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
    version: 5.9.2
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
