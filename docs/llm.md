# LLM access

The `[llm]` table configures the one model connection every model-backed ai-rulez feature shares: the provider and
model, the spend caps, the response cache and the privacy rules. Nothing calls a model unless you turn it on:
`allow_network` defaults to `false`, and every call is refused with a message that names the setting.

These features use it:

| Feature | What it sends |
| --- | --- |
| [`judge`](judge.md) | the agent configuration files it reviews, plus the rules and instruction files that apply to them |
| [`review --semantic`](review.md), `review calibrate`, `review fix` | skill, agent and command descriptions (and bodies with `--content full`) |
| [`eval run --grader builtin`](evals.md#the-built-in-grader) | the rubric and the transcript an eval runner returned |
| [`improve --with builtin:review-fix`](improve.md#bundled-adapters) | the `SKILL.md` being improved |
| LLM-backed [`[[verifiers]]`](verifiers.md) and `verifiers suggest` | the verifier's prompt and inputs |
| [`search`](search.md) embeddings | skill text, with `embedding_model` set |
| `llm doctor --ping` | a one-token `ping` |

## Example

Opt in once in your **user** config file (`$XDG_CONFIG_HOME/ai-rulez/config.toml`, else
`~/.config/ai-rulez/config.toml`):

```toml
# ~/.config/ai-rulez/config.toml
[llm]
provider      = "anthropic"
model         = "claude-haiku-4-5"
api_key_env   = "ANTHROPIC_API_KEY"   # the NAME of the variable, never the key
allow_network = true
max_cost_usd  = 1.00
```

Check what ai-rulez would use, and what a prompt costs, without calling anything:

```console
$ ai-rulez llm doctor
liter-llm:       v2.4.4
model:           anthropic/claude-haiku-4-5
embedding model: (none)
base_url host:   (provider default)
api key env:     ANTHROPIC_API_KEY (set: true)
network allowed: true
cache:           /home/dev/.cache/ai-rulez/llm/0b2e76eb32b753f0
limits:          max_cost_usd=1 max_tokens=0 max_calls=0 (0 = unlimited)

$ ai-rulez llm estimate .ai-rulez/skills/deploy-staging/SKILL.md
model:         anthropic/claude-haiku-4-5
file bytes:    277
prompt tokens: ~100
max output:    1024
worst-case cost: <= $0.005220
(approximate; nothing was sent)
```

`ai-rulez llm doctor --ping` makes one 1-token call to confirm the key and endpoint work.

## Providers

ai-rulez reaches models through [liter-llm](https://github.com/xberg-io/liter-llm), which routes on a
`provider/model` name. Set `provider` and a bare `model`, or put the prefix in `model` (`openai/gpt-4o-mini`).

| Provider | `provider` | Example model | Credentials |
| --- | --- | --- | --- |
| OpenAI | `openai` | `gpt-4o-mini` | `OPENAI_API_KEY`, or the variable `api_key_env` names |
| Anthropic | `anthropic` | `claude-haiku-4-5` | `ANTHROPIC_API_KEY`, or `api_key_env` |
| Google Gemini | `gemini` | `gemini-2.5-flash` | `GEMINI_API_KEY`, or `api_key_env` |
| Google Vertex AI | `vertex_ai` | `gemini-2.5-flash` | the provider's credentials, or `api_key_env` |
| Amazon Bedrock | `bedrock` | `global.anthropic.claude-sonnet-5-5` | the AWS credential chain, see [Amazon Bedrock](#amazon-bedrock) |
| OpenAI-compatible gateway (LiteLLM proxy, vLLM, Ollama, Azure OpenAI or Bedrock gateways) | `openai`, or the gateway's provider | the gateway's model name | `api_key_env` for the gateway credential, with `base_url` |

liter-llm supports further providers under their own prefixes; the ones above are the supported providers.
With a `base_url`, a named provider keeps its own wire format, so a Gemini or Vertex model behind a gateway still
speaks the Gemini protocol.

When no `api_key_env` is set, the provider's own key variable is read.

## Configuration

```toml
[llm]
provider        = "openai"
model           = "gpt-4o-mini"
embedding_model = "text-embedding-3-small"
max_cost_usd    = 2.00
max_tokens      = 500000
max_calls       = 200
cache           = true

# User scope only (see "Trust rule"):
allow_network   = false
base_url        = "https://llm-gateway.acme.example/v1"
api_key_env     = "OPENAI_API_KEY"
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `provider` | string | none | Provider prefix for `provider/model` routing. |
| `model` | string | none | Chat model, bare or with a `provider/` prefix. |
| `embedding_model` | string | none | Embedding model for search. |
| `allow_network` | bool | `false` | Allow any provider call. User scope only. |
| `base_url` | string | the provider's endpoint | Gateway or model-server URL. No credentials or query string. User scope only. |
| `api_key_env` | string | the provider's variable | Name of the environment variable that holds the key. User scope only. |
| `max_cost_usd` | float | `0` (unlimited) | Stop before the worst-case estimate would exceed this. |
| `max_tokens` | int | `0` (unlimited) | Prompt plus completion tokens for the whole run. |
| `max_calls` | int | `0` (unlimited) | Provider calls for the whole run. |
| `cache` | bool | `true` | Use the response cache. |
| `timeout_seconds` | int | `60` | Time limit per call including retries; at most 3600. |
| `max_retries` | int | `3` | Retries of transient failures; `-1` disables, at most 10. |
| `price_input_per_mtok`, `price_output_per_mtok` | float | built-in price table | USD per million tokens for the model you set in user scope. Needed for a cost cap on a model the table does not know. User scope only. |
| `allow_plain_http`, `plain_http_hosts` | bool, list | `false`, `[]` | Send a key over plain http to the listed hosts; see [Plain-http gateways](#plain-http-gateways). User scope only. |

Individual commands add their own caps on top (`judge --max-cost`, `review --max-cost`, `eval run --grader-max-cost`).
Whichever limit is reached first stops the run.

### Environment variables

Precedence, highest first: environment, user config file, repository config, defaults. An unparsable value is an
`AR9L0` error, not a silent default.

| Variable | Meaning |
| --- | --- |
| `AI_RULEZ_LLM_PROVIDER`, `_MODEL`, `_EMBEDDING_MODEL` | Routing and models. |
| `AI_RULEZ_LLM_ALLOW_NETWORK`, `_BASE_URL`, `_API_KEY_ENV` | The user-scope keys of the same name. |
| `AI_RULEZ_LLM_MAX_COST_USD`, `_MAX_TOKENS`, `_MAX_CALLS`, `_TIMEOUT_SECONDS`, `_CACHE` | Limits and cache. |
| `AI_RULEZ_LLM_ALLOW_PLAIN_HTTP`, `_PLAIN_HTTP_HOSTS` | Plain-http opt-in; hosts comma-separated. |
| `AI_RULEZ_LLM_PRICE_INPUT_PER_MTOK`, `_PRICE_OUTPUT_PER_MTOK` | Price override for the model named by `AI_RULEZ_LLM_MODEL`. |
| `AI_RULEZ_LLM_TEMPERATURE=omit` | Leave `temperature` out of every request, for a model that rejects it. Environment only. |

The [GitHub Action](github-actions.md) accepts matching `llm-*` inputs. Keep credentials in the job's environment.

### Trust rule

A committed repository config is input from whoever wrote it: cloning a repository must not switch on network use,
choose the host a prompt goes to, or pick which of your keys is sent.

| Key | Repository config and `config.local.*` | User config file and `AI_RULEZ_LLM_*` |
| --- | --- | --- |
| `allow_network`, `base_url`, `api_key_env`, `allow_plain_http`, `plain_http_hosts`, `price_input_per_mtok`, `price_output_per_mtok` | ignored, reported by `llm doctor`, `ai-rulez doctor` and `AR9L1` | honored |
| `provider`, `model`, `embedding_model`, `cache`, `timeout_seconds`, `max_retries` | honored, except for provider routing (below) | honored |
| `max_cost_usd`, `max_tokens`, `max_calls` | can only tighten: the lower non-zero value wins | honored |

The local overlay lives in the checkout, so it counts as repository scope.

```console
$ ai-rulez llm doctor
...
warning:         repository [llm] keys ignored (user scope only: set them in the user config file ~/.config/ai-rulez/config.toml or via AI_RULEZ_LLM_*): base_url
```

**Provider routing and the key.** The provider decides which service receives the request, and with it your key. When
the provider (or a `provider/` prefix in `model` or `embedding_model`) comes only from the repository config, ai-rulez
refuses to send a key (`AR9L0`), whether the key comes from `api_key_env` or from the provider's own variable. Set
`provider` and `model` in the user config file, or with `AI_RULEZ_LLM_PROVIDER` and `AI_RULEZ_LLM_MODEL`. A repository
`model` whose prefix matches the provider you set is fine. A per-request model (a verifier's `llm.model`, a search
embedding model) is held to the same rule: a bare name is pinned to your provider, and a prefix that names another
provider is refused before anything is sent.

See the [trust model](trust-model.md) for every setting that reaches the network or runs a process.

### Plain-http gateways

By default a key is never sent over plain http except to a loopback host (`localhost`, `127.0.0.1`, `::1`). For a
private gateway that only speaks http, opt in from user scope and list the host exactly as it appears in `base_url`:

```toml
# ~/.config/ai-rulez/config.toml
[llm]
base_url         = "http://gateway.internal:8080/v1"
api_key_env      = "GATEWAY_KEY"
allow_plain_http = true
plain_http_hosts = ["gateway.internal:8080"]
```

The match is exact and case-insensitive on `host` or `host:port`: `gateway.internal` does not cover
`gateway.internal:8080`. `llm doctor` warns while the opt-in is in use. The key and every prompt cross that network
unencrypted, so prefer https, or a loopback tunnel (`ssh -L 8080:gateway.internal:8080`, then
`base_url = "http://127.0.0.1:8080/v1"`, which needs no opt-in).

## What happens on every call

- **Budget, fail closed.** Before each call, the worst case (estimated prompt plus completion cap) is reserved against
  `max_cost_usd`, `max_tokens` and `max_calls`; a call that does not fit is refused. Afterwards the reservation is
  replaced by the reported usage. Tokens are estimated at one per three UTF-8 bytes, deliberately high. Thinking tokens
  count as completion. A rejection the provider does not bill (400, 401, 403, 404, 422) releases its cost; a 429, 5xx,
  timeout or transport failure keeps the worst case charged, since it may have run upstream. With `max_cost_usd` set,
  a model with no known price is refused.
- **Prices** come from liter-llm's model catalog plus a few fixed entries (the `haiku`, `sonnet` and `opus` short names
  eval cases use, and the Claude families). A Bedrock id is reduced to the model it names before the lookup. Set
  `price_input_per_mtok` and `price_output_per_mtok` for any other model. Reported costs are estimates, not a bill.
- **Cache.** Answers are cached in `~/.cache/ai-rulez/llm/<project hash>/`, outside the repository, keyed by the
  endpoint (provider, base URL, key variable name, never its value), the model, the request and the feature's prompt
  version. Every entry is authenticated with a per-user secret (`$XDG_CONFIG_HOME/ai-rulez/llm-cache.key`, mode 0600):
  a modified or planted entry is discarded, and a cache restored from another machine simply misses. A cache hit costs
  nothing. Turn it off with `cache = false` or `AI_RULEZ_LLM_CACHE=0`.
- **Retry.** 429, 500, 502, 503, 504, 529 and transport failures are retried up to `max_retries` times with jittered
  backoff, honoring `Retry-After`. Authentication, context-length and other 4xx errors are not retried.
- **Redaction.** Logs and estimates carry counts, sizes, hashes and cost, never prompt or completion text and never
  keys. Provider error text has the configured key and key-shaped tokens masked, and URLs lose their query string.
- **Temperature** is left out for models that reject it (Claude 5.x, OpenAI reasoning models), and a request rejected
  for its temperature is retried once without it.

### Cost controls

Set `max_cost_usd`, `max_tokens` and `max_calls` for any feature that loops, and run the feature's estimate first:
`judge --estimate`, `review --estimate`, `eval run --dry-run`, or `ai-rulez llm estimate <file>` for a single prompt
(`--max-output N` sets the completion tokens to assume; default 1024). Use `--format json` on `llm` commands for
machine-readable output.

## Data egress and residency

With `allow_network` on, each feature sends the text listed in the table at the top of this page to the configured
endpoint, and nothing else: no config files, keys or environment. `judge` prints the egress host and models before it
sends anything, and only runs with `--allow-llm`. An eval grader sends a transcript of an agent session, which can
contain source code, file paths, tool output and anything a user typed.

To keep data inside your network, point `base_url` at a gateway you run (a LiteLLM proxy, a Bedrock or Azure OpenAI
gateway in your VPC, vLLM or Ollama on an internal host) and use `api_key_env` for its credential. `llm doctor` shows
the host so a reviewer can check it. HTTP redirects are refused, never followed, so a prompt is never re-sent to
another host. A key is sent only to the configured host and only over https, except to a loopback host or a listed
plain-http host.

Retention and training terms are the provider's; check them before sending transcripts that contain private code.

### Amazon Bedrock

A `bedrock/...` model is signed with AWS credentials from the AWS SDK's default chain: environment variables,
`AWS_PROFILE` and the shared config (SSO and assume-role profiles included), web identity
(`AWS_WEB_IDENTITY_TOKEN_FILE`), and container and instance roles. A Bedrock API key (`AWS_BEARER_TOKEN_BEDROCK`) or a
key pair in the environment is used as is. Temporary credentials are renewed before they expire, and a call refused as
unauthenticated renews them and is tried once more, so a long run does not fail halfway.

The region comes from `AWS_REGION`, `AWS_DEFAULT_REGION` or the profile, and the egress host is
`bedrock-runtime.<region>.amazonaws.com`. Inference-profile ids (`global.anthropic.claude-...`, `us.`, `eu.`, ...) are
priced.

```bash
export AWS_PROFILE=acme-dev AWS_REGION=eu-north-1
export AI_RULEZ_LLM_ALLOW_NETWORK=1
ai-rulez judge --allow-llm --estimate      # Bedrock is judge's default provider
```

## Validation: AR9L0 and AR9L1

| Code | Name | Severity | Finds |
| --- | --- | --- | --- |
| `AR9L0` | `llm-config-invalid` | error | A literal secret where a variable name belongs (`api_key = ...`, or a key-shaped `api_key_env` such as `sk-proj-...`); credentials or a query in `base_url`; plain `http://` to a non-loopback host when a key is sent; `allow_plain_http` without `plain_http_hosts`; `max_retries` above 10; `timeout_seconds` above 3600; negative or non-finite limits and prices; provider routing chosen by the repository. |
| `AR9L1` | `llm-untrusted-key` | warning | A repository `[llm]` table (or its local overlay) sets a key only user scope honors. |

`validate`, the JSON schema, `llm doctor` and `ai-rulez doctor` report them. A message never repeats the offending
value.

```console
$ ai-rulez validate
.ai-rulez/config.toml:12: warning AR9L1 llm-untrusted-key: llm: repository [llm] keys ignored (user scope only: ...): base_url
```

## Commands

| Command | Meaning |
| --- | --- |
| `ai-rulez llm doctor [--ping]` | Print the liter-llm version, model, `base_url` host (never the key), whether the key variable is set, the network gate, cache directory and limits. `--ping` makes one 1-token call and needs `allow_network`. Needs a project configuration. Also a section of `ai-rulez doctor`. |
| `ai-rulez llm estimate <file> [--max-output N]` | Approximate tokens and worst-case cost of sending the file as a prompt. No call. Works with only the user configuration. |
