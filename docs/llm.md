# LLM access

`internal/llm.Client` is the one Go interface for reaching a language model, so the provider, the cost controls and the privacy rules live in one place. Its consumers are the rubric grader of `eval run`, `review --semantic`, `review calibrate`, `review fix` and `review explain`, the LLM-backed `[[verifiers]]`, the embeddings behind `search` (skill search index), and the diagnostics `llm doctor` (and its `--ping`) and the `llm` section of `ai-rulez doctor`.

**Nothing calls a model unless you turn it on.** `allow_network` defaults to `false`; every call is refused with a message that names the setting.

## Configuration

```toml
# .ai-rulez/config.toml   (allow_network, base_url and api_key_env below only take effect from user scope, see "Trust rule")
[llm]
provider        = "openai"                 # provider prefix; the literllm backend routes on provider/model
model           = "gpt-4o-mini"
backend         = "auto"                   # auto | openaicompat | literllm
base_url        = "https://llm-gateway.internal.example/v1"   # optional; see "Data egress"
api_key_env     = "OPENAI_API_KEY"         # the NAME of an environment variable, never a key
embedding_model = "text-embedding-3-small"
max_cost_usd    = 2.00                     # stop before the estimate would exceed this
max_tokens      = 500000                   # prompt + completion, whole run
max_calls       = 200
cache           = true                     # default true
allow_network   = false                    # default false; nothing is sent unless true
```

Other keys: `timeout_seconds` (default 60, covers the retries), `max_retries` (default 3, `-1` disables, at most 10), `price_input_per_mtok` and `price_output_per_mtok` (USD per million tokens; override the built-in price table, needed for cost limits on models the table does not know; user scope only, see below). An override prices only the model you set in user scope (`model` in the user config file or `AI_RULEZ_LLM_MODEL`); a model a repository picks, or a per-request model, is priced from the built-in table and, under a cost limit, refused when the table does not know it.

### Trust rule

A committed repository config is attacker-controlled input: cloning a repository must not switch on network use, choose the host a prompt goes to, or pick which environment variable is sent as a credential. So, like `[telemetry]`:

| Key | Repository config and `config.local.*` | User config file / `AI_RULEZ_LLM_*` |
| --- | --- | --- |
| `allow_network`, `base_url`, `api_key_env`, `allow_plain_http`, `plain_http_hosts`, `price_input_per_mtok`, `price_output_per_mtok` | **ignored**, reported by `llm doctor`, `ai-rulez doctor` and strict finding `AR9L1` | honoured |
| `provider`, `model`, `backend`, `embedding_model`, `cache`, `timeout_seconds`, `max_retries` | honoured, with one exception for the `literllm` backend (below) | honoured |
| `max_cost_usd`, `max_tokens`, `max_calls` | honoured, but can only tighten: the lower non-zero value of repository and user wins | honoured |

**Provider routing and the key.** With the `literllm` backend the `provider` (or a `provider/` prefix in `model`) decides which service receives the request, and with it the API key. So when the key variable comes from user scope but the provider or model prefix comes only from the repository config, `literllm` refuses to start (`AR9L0`) and `llm doctor` reports it; set `provider` and `model` in the user config file or `AI_RULEZ_LLM_PROVIDER` / `AI_RULEZ_LLM_MODEL`. The check also applies when you have no user config file at all and configure the key only through `AI_RULEZ_LLM_*`: a repository `provider` or `model` prefix is still repository-chosen routing unless `AI_RULEZ_LLM_PROVIDER` / `AI_RULEZ_LLM_MODEL` set it. A repository `model` whose prefix equals the provider you configured in user scope is fine. A repository `embedding_model` with a `provider/` prefix is repository-chosen routing too. The same rule covers a per-request model (a verifier's `llm.model`, `[search.embeddings] model`): the `literllm` backend sends a prefixed request model only when its prefix is the configured provider, pins a bare one to that provider, and otherwise refuses the call with `AR9L0` before anything is sent. The `openaicompat` backend sends the key only to the user-scope `base_url` (or the OpenAI default), so it is unaffected.

See the [trust model](trust-model.md) for every knob that reaches the network or runs a process.

The local overlay lives in the checkout, so it counts as repository scope. Opt in once on your machine in the user config file (`$XDG_CONFIG_HOME/ai-rulez/config.toml`, else `~/.config/ai-rulez/config.toml`):

```toml
[llm]
allow_network = true
base_url      = "https://llm-gateway.internal.example/v1"
api_key_env   = "OPENAI_API_KEY"
```

or export `AI_RULEZ_LLM_ALLOW_NETWORK=1` (and `_BASE_URL`, `_API_KEY_ENV`). Precedence, highest first: environment, user config file, repository config, defaults.

### Environment overrides

`AI_RULEZ_LLM_PROVIDER`, `_MODEL`, `_BACKEND`, `_BASE_URL`, `_API_KEY_ENV`, `_EMBEDDING_MODEL`, `_MAX_COST_USD`, `_MAX_TOKENS`, `_MAX_CALLS`, `_TIMEOUT_SECONDS`, `_CACHE`, `_ALLOW_NETWORK`, `_ALLOW_PLAIN_HTTP`, `_PLAIN_HTTP_HOSTS` (comma-separated). An environment value wins over the config file. An unparsable value is an `AR9L0` error, not a silent default.

### Validation: `AR9L0 llm-config-invalid`

`validate --strict`, the JSON schema and `ai-rulez doctor` reject an unknown `backend`, a secret where a variable name belongs (`api_key = ...`, or a key-looking `api_key_env` such as `sk-proj-...` or `AKIA...`), credentials or a query string in `base_url`, plain `http://` to a non-loopback host when an API key is sent (use `https`; `localhost`, `127.0.0.1` and `::1` may use `http`; see "Plain-http gateways" for the explicit opt-in), `allow_plain_http` without `plain_http_hosts`, a `max_retries` above 10, and negative limits. A message never repeats the offending value, and `llm doctor`, `ai-rulez doctor` and the JSON report show `api_key_env` only when it is a valid variable name. The literal-secret key check (`api_key = ...`) covers both `config.toml` and `config.local.*`. A repository key the trust rule ignores is reported as `AR9L1`.

### Plain-http gateways

A gateway on a private network that terminates TLS in a sidecar or service mesh may only speak `http://`. By default a key is never sent over plain http except to a loopback host. The explicit opt-in, honoured only in user scope (the user config file or the environment, never a committed repository config, where it is ignored and reported as `AR9L1`):

```toml
# ~/.config/ai-rulez/config.toml
[llm]
base_url         = "http://gateway.internal:8080/v1"
api_key_env      = "GATEWAY_KEY"
allow_plain_http = true
plain_http_hosts = ["gateway.internal:8080"]   # required; matched exactly against base_url's host as written
```

or `AI_RULEZ_LLM_ALLOW_PLAIN_HTTP=1` with `AI_RULEZ_LLM_PLAIN_HTTP_HOSTS=gateway.internal:8080`. A key is sent only to a host on the list. The match is exact and case-insensitive on `host` or `host:port` as it appears in `base_url`: an entry `gateway.internal` does not cover `http://gateway.internal:8080`, and `gateway.internal:8080` does not cover another port. Redirects to another host stay refused. `llm doctor` prints a warning while the opt-in is in use (the same warning is in `ai-rulez doctor`). The trade-off: the key and every prompt cross that network segment unencrypted, so anyone on the path can read or replay them. Prefer `https`, or a loopback tunnel (`ssh -L 8080:gateway.internal:8080`, then `base_url = "http://127.0.0.1:8080/v1"`, which needs no opt-in).

## Commands

```text
ai-rulez llm doctor [--ping] [--format json]   resolved backend, model, base_url host (never the key),
                                        whether the key variable is set, network gate, cache dir, limits
ai-rulez llm estimate <file>            approximate tokens and worst-case cost of sending the file; no call
```

`llm doctor` is also a section of `ai-rulez doctor` (check `llm`, silent when there is no `[llm]` table). `--ping` makes one 1-token call and refuses unless `allow_network` is true.

## What it does for every call

Outermost first: network gate and timeout, cache, retry, budget, backend.

- **Budget, fail closed.** Before each provider call the guard reserves its worst case (estimated prompt plus completion cap) against `max_cost_usd`, `max_tokens` and `max_calls` in one atomic step, counting what calls still in flight hold, so concurrent calls cannot jointly overshoot a limit; it refuses if the worst case does not fit. When the call ends the reservation is replaced by the actual usage. A request without `max_tokens` gets a 1024-token completion cap while a budget is active. If a provider reports no usage, the worst case is charged. A call is priced by the model you requested, never by the model name the provider reports (a reported name can only raise the price). With `max_cost_usd` set and no known price for the requested model, the call is refused. A rejection the provider documents as unbilled (HTTP 400, 401, 403, 404, 422, or an auth, config or budget error raised before anything was sent) releases its tokens and cost and keeps only the call count. Everything else may have run upstream, so the worst case stays charged: a 429, any 5xx (including a 504 gateway timeout), a timeout, a transport failure, an undecodable reply, a cancelled context. Retries count as calls. Cache hits cost nothing. Token counts are estimates from UTF-8 length (one token per three bytes), deliberately high.
- **Cache.** Keyed by an endpoint identity (provider, base URL scheme, host and path, resolved backend, the name of the key variable, never its value, and the price table version or override), the model, the request and its `PromptVersion`, so changing the gateway, backend, key variable, prices, model or prompt version misses. Entries hold model output only. They live outside the repository, in the user cache directory `~/.cache/ai-rulez/llm/<project hash>/` (mode 0700, files 0600, written atomically), so nothing a checkout contains can plant one. Every entry carries an HMAC-SHA256 over identity, key and payload, computed with a per-user secret (`$XDG_CONFIG_HOME/ai-rulez/llm-cache.key`, else `~/.config/ai-rulez/llm-cache.key`; 32 random bytes, mode 0600, created on first use; the file must be a regular 0600 file in a directory that is not group- or world-writable, and a loose or symlinked file is replaced or refused; `XDG_CONFIG_HOME` is read from the environment, so do not set it from an untrusted `.envrc`). A modified, truncated, oversized, symlinked or planted entry fails the check, counts as a miss and is deleted; entry reads are size-capped. A missing or damaged secret is regenerated, which invalidates every older entry; if no secret can be stored the cache is off, never unauthenticated. A cache restored from CI (another job, an untrusted branch) is only valid when the secret matches, so a restored cache from a different secret simply misses. Opt out with `cache = false`, `AI_RULEZ_LLM_CACHE=0` or `NoCache` (on the options or a request).
- **Retry.** Rate limits, 5xx and transport errors (for `literllm`, the `Network` and `InternalError` variants, and any variant liter-llm marks transient) retry with exponential backoff and full jitter, honouring `Retry-After` (capped at one hour and at the 30 s backoff ceiling). Authentication, context-length, budget and config errors and malformed replies never retry.
- **Error classification.** The native binding returns flat `"[code] message"` strings; one function (`classifyNative`) maps them to typed errors, and a table-driven corpus test (`native_classify_test.go`) pins the real strings so a reworded upstream message fails CI. A message it does not recognise becomes a permanent provider error: never retried, and an error is never written to the cache. An empty reply with no error from the binding (upstream liter-llm #246) is a permanent error, never a success. Once liter-llm exposes typed errors (upstream #244) the adapter will classify by them and keep the text match as the fallback; no released version does yet, so there is no minimum version to require.
- **Typed errors.** `errors.Is(err, llm.ErrRateLimit | ErrAuth | ErrContextLength | ErrProvider | ErrBudget | ErrNetworkDisabled | ErrConfig | ErrTimeout)`; `*llm.Error` carries the HTTP status and `Retry-After`.
- **Redaction.** Logs (`llm.WithLogging`) and dry-run output carry counts, sizes, a short content hash, usage and cost, never prompt or completion text and never keys. Provider error bodies are scrubbed of key-looking text before they enter an error. A value that is, in its entirety, an environment lookup (`os.environ["X"]`, `os.Getenv("X")`, `process.env.X`, `$X`, `${X}`) names a secret without holding one and is not masked; a lookup that carries a literal, such as `${X:-sk-...}`, is.
- **Dry run.** `Options.DryRun` returns a client that prints what would be sent (model, sizes, estimated tokens and cost; message text only with `ShowContent`) and returns `llm.ErrDryRun`.

## Data egress and residency

When `allow_network` is true, the prompt text of each feature is sent to the configured endpoint. What that is depends on the caller. Today only `llm doctor --ping` sends anything (a one-token `ping`). For a future judge-style feature it would be a rubric and the transcript of an agent session, which can contain source code, file paths, tool output and anything a user typed. ai-rulez adds no other data, and never sends config files, keys or environment.

To keep data inside your network, point `base_url` at a gateway you run (a LiteLLM proxy, a Bedrock or Azure OpenAI gateway in your VPC, Ollama or vLLM on an internal host) and use `api_key_env` for the gateway credential. The `openaicompat` backend speaks only the OpenAI chat and embeddings shapes and talks to the configured host alone: it never follows an HTTP redirect (a 3xx is a provider error), so a prompt is not re-sent to another host; `llm doctor` shows the host so a reviewer can check it. `base_url` must not embed credentials or a query string (`AR9L0`). The default endpoint (used only when `provider` is `openai` or empty and no `base_url` is set) is `https://api.openai.com/v1`. A key is sent only as an `Authorization: Bearer` header to that host, and only over `https` unless the host is loopback. `base_url`, `allow_network` and `api_key_env` are honoured only from user scope (see the trust rule above).

Model-side retention and training terms are the provider's; check them before sending transcripts that contain private code.

### Cost controls

Set `max_cost_usd`, `max_tokens` and `max_calls` for any feature that loops. Use `ai-rulez llm estimate <file>` to see what a prompt costs first (tokens are estimated at one per three UTF-8 bytes, which overestimates English text and holds for CJK). The built-in price table is small and approximate (OpenAI `gpt-4o`/`gpt-4.1` families and embeddings, Gemini `gemini-2.5-flash`, `gemini-2.5-flash-lite` and `gemini-embedding-001`, Claude families) and only for budget estimates; set `price_input_per_mtok` / `price_output_per_mtok` for anything else. Cost in responses is an estimate from that table, not a bill.

## Backends

| Backend | Build | Notes |
| --- | --- | --- |
| `openaicompat` | always (pure Go, `net/http`) | chat (JSON-schema `response_format`), embeddings; no streaming; no new dependencies |
| `literllm` | `-tags literllm`, cgo, separate module | experimental; same request/response shapes over the liter-llm FFI; 174 providers via `provider/model` |

`backend = "auto"` picks `literllm` when it is compiled in and `openaicompat` otherwise. Choosing `literllm` in a binary without it is an `AR9L0` error that says how to build it.

For `openaicompat` the `model` is sent verbatim (so a gateway sees exactly the name you configured); `provider` is used for cost lookup and for the `literllm` backend's `provider/model` routing.

## The optional `literllm` backend

[liter-llm](https://github.com/xberg-io/liter-llm) is a Rust client with one interface for 174 providers (`provider/model` names, keys from the provider's environment variable, its own cache, budget, rate limit and cost tracking). Its Go module is `github.com/xberg-io/liter-llm/packages/go/v2`, a cgo wrapper over the Rust library `libliter_llm_ffi`.

What ai-rulez relies on, ported to liter-llm 2.1.3 and checked against its release commit (`4ad174d`, built locally) on macOS arm64. 2.1.3 is not published yet: until it is, `internal/llm/literllm/go.mod` stays on v2.1.2, which does not compile against this adapter (it needs `ChatWithContext` and typed errors), so a `literllm` build needs a workspace `replace` to a 2.1.3 checkout. When v2.1.3 ships, bump the one `require` line in `internal/llm/literllm/go.mod` and its `go.sum`.

- `CreateClient(apiKey, baseURL, timeoutSecs, maxRetries, modelHint)`, `ChatWithContext(ctx, req)`, `EmbedWithContext(ctx, req)` and `Free()`; request and response JSON is OpenAI-shaped, so the adapter reuses the same encoder and decoder as `openaicompat`. `response_format` with a JSON schema reaches the server. The configured `provider/model` is passed as the `modelHint`, which is what lets liter-llm strip the provider prefix when `base_url` is set.
- Typed errors: `*literllm.Error` carries the variant (`Authentication`, `RateLimited`, `Timeout`, ...), HTTP status, a transient flag and `RetryAfter`. The adapter classifies by variant and never reads message text; a failure that is not typed is a permanent provider error.
- Batch embeddings: Gemini's native route behind liter-llm answers a batch with a single vector whatever the batch size. The backend detects the wrong vector count, retries that batch as one request per input, and remembers it, so later batches go out one input per request without repeating the wasted batch call. The collapsed batch is still billed, so its tokens are added to the response and every provider request counts against `max_calls`; set `[search] batch_size = 1` to avoid the first wasted request.
- Nothing else: ai-rulez keeps its own cache, budget, retry and redaction so they behave the same for both backends (liter-llm's budget is USD-only and it does not return a per-response cost).

How it is linked, and why it is not in the default build:

- The binding's API exists only when cgo is on (its code is all `import "C"`), and it needs the native library at link time, so importing it would break `CGO_ENABLED=0` builds. The release tarball `liter-llm-go-v2.1.3-<platform>.tar.gz` holds `include/liter_llm.h`, the static `lib/libliter_llm_ffi.a` (about 169 MB on macOS arm64) and a dynamic library (about 22 MB).
- ai-rulez therefore keeps the binding in a nested module, `internal/llm/literllm` (its `go.mod` is the only place the binding is required), and a file guarded by `//go:build literllm && cgo` in `internal/llm` that registers it. The main module's `go.mod` and `go.sum` never mention liter-llm, `go list -deps ./...` shows nothing from it, and `CGO_ENABLED=0 go build ./...` is unchanged.
- The adapter is a thin shell over JSON: the request and response mapping and the error classification live in cgo-free code that the default tests cover with a stub `NativeClient`.

### Building a release with it

1. Download the tarball for the target platform and verify it against the `.sha256` sidecar from the same release. Copy only `lib/libliter_llm_ffi.a` into an otherwise empty directory (if a dynamic library sits next to it, `-l` prefers that one).
2. The repository ships `literllm.work`, a workspace file that uses both the root module and `internal/llm/literllm`. It is not named `go.work`, so ordinary builds never pick it up; select it with `GOWORK`. `GOWORK=$PWD/literllm.work go list -tags literllm ./internal/llm` checks that the wiring resolves.

3. Build:

   ```sh
   GOWORK=$PWD/literllm.work CGO_ENABLED=1 \
     CGO_LDFLAGS="-L/path/to/static-lib-dir" \
     go build -tags literllm -o ai-rulez ./cmd/ai-rulez
   ```

   Without `GOWORK=...` the bridge module is not part of the build and `-tags literllm` fails to resolve its import.

The bridge module's own tests are compiled only with `-tags literllm` (and cgo), so a plain `go test ./...` inside
`internal/llm/literllm` finds nothing to build or run when the binding or the static library is not available. To run
them, set `CGO_LDFLAGS` as above and `go test -tags literllm ./...` from that directory.

Since 2.1.3 the binding's cgo preamble declares the system libraries (macOS frameworks, `-lm -ldl -lpthread -lrt` on Linux), so `-L` is the only flag. Its own `-L` points at a bundled dynamic library; yours comes first, so the static one wins when it is the only `libliter_llm_ffi` in your directory.

### Verified and not verified

Verified here, on macOS arm64 with the locally built 2.1.3 (`4ad174d`): the module compiles and links statically with `-L` alone (binary 81 MB against 42 MB for the default build, which is unchanged); the bridge module's tests pass against the real library (chat, embeddings, idempotent `Free`, use-after-`Free` returns an error, typed `Authentication` / `RateLimited` (with `Retry-After`) / `Timeout` errors, prompt cancellation through the context, provider-prefix stripping with a `base_url`); the `internal/llm` tests pass with `-tags literllm`; `llm doctor --ping` returns `ok` through both backends against Gemini.

Live parity (`AI_RULEZ_LIVE_LLM=1 go test -tags literllm -run TestLive ./internal/llm`, needs `GEMINI_API_KEY`) against Gemini, `gemini-2.5-flash-lite` and `gemini-embedding-001`: chat, JSON-schema `response_format` and embeddings succeed on both backends (`openaicompat` through Google's OpenAI-compatible endpoint, `literllm` through its native Gemini route), and a bad key (Gemini answers HTTP 400, not 401, so both report a permanent `provider` error) and a 1 ms timeout (`timeout`, transient) are classified identically.

Not verified: Linux, Windows and macOS x86_64 builds; providers other than Gemini; concurrent use of one client from several goroutines; running on a macOS older than the one the library was built on (2.1.3 pins the C objects to 11.0, not checked here).

### Limitations

- **Binary size.** Linking it adds about 39 MB to a 42 MB CLI (81 MB total, macOS arm64). The release asset has no slim feature set (upstream #250 and #252 only changed the documentation).
- **Provider schema dialects.** liter-llm translates `response_format` into the provider's native form, and Gemini's native API rejects `additionalProperties` in a schema (HTTP 400). ai-rulez's own schemas therefore leave it out and refuse extra fields when decoding the reply; a schema you pass through the library yourself must do the same for Gemini.
- **Model prefix.** With a `base_url`, liter-llm strips only the `provider/` prefix named by the model hint (now the configured `provider`), so `provider = "openai"`, `model = "gpt-4o-mini"` reaches a strict server as `gpt-4o-mini`. Without a configured provider the model is sent verbatim.
- **Platforms.** Release assets exist for macOS (arm64, x86_64), Linux glibc (x86_64, aarch64) and Windows (x86_64, aarch64); none for musl.
- **Status: experimental.** The upstream issues the adapter worked around are fixed in 2.1.3 (#244 typed errors, #245 context cancellation, #246 no `(nil, nil)`, #247 and #248 static link and macOS deployment target, #249 prefix stripping, #250 to #252 docs and client config), and live parity with `openaicompat` passes for Gemini. It stays experimental because 2.1.3 is unreleased (the pin is still 2.1.2 and a build needs a local `replace`), only macOS arm64 and only one provider have been exercised.

### Licensing

liter-llm is MIT licensed (Copyright 2026 Kreuzberg, Inc.). The Go module is a wrapper around a Rust library whose dependency tree carries other licenses (its `deny.toml` allows Apache-2.0, MIT, BSD, ISC, Zlib, MPL-2.0, OpenSSL, among others), and its provider table is derived from LiteLLM's (MIT, attribution in liter-llm's `ATTRIBUTIONS.md`). A release that bundles the static library redistributes that code: ship the MIT notice and the third-party notices of the library (`cargo about` or the upstream attribution file) with the binary. The default build contains none of it.

Version pin: `github.com/xberg-io/liter-llm/packages/go/v2 v2.1.2`, in `internal/llm/literllm/go.mod` only. The adapter needs v2.1.3 or later; bump it when that release is published.

## Using it from a feature

```go
lc, err := cfg.ResolvedLLM()            // cfg is the loaded *config.Config; env overrides applied
if err != nil { return err }
client, err := llm.New(lc, llm.Options{ConfigDir: cfg.ConfigDir})
if err != nil { return err }            // AR9L0; a disabled network surfaces on the first call
defer client.Close()

v, err := llm.Judge(ctx, client, rubric, transcript)   // v.Score in [0,1], v.Rationale
```

`llm.Judge` uses structured output at temperature 0 and `llm.JudgePromptVersion` as the cache version. It asks for at least `llm.DefaultJudgeCompletionTokens` (2048) completion tokens, because a reasoning model spends its thinking out of that budget before the verdict (`JudgeOptions.MinCompletionTokens` raises it). Change the judge prompt or schema and bump that constant. The transcript is sent as data between two marker lines that carry a token derived from the request's own content (so the transcript cannot predict or close it), and the system prompt tells the model to ignore instructions inside it. The reply must be exactly one JSON object with both `score` (in [0,1]) and `rationale`; anything else is a provider error, and a reply that fails that check is never written to the cache (a cached one is treated as a miss). Before sending, the rubric and transcript are scanned for secret-looking text (bearer tokens, `sk-...`, `AKIA...`, `api_key=...`); `Judge` refuses when it finds any, and `llm.JudgeWith(..., llm.JudgeOptions{RedactSecrets: true})` sends them masked instead. The scan is a pattern match, not a guarantee. A feature must treat `errors.Is(err, llm.ErrNetworkDisabled)` as "refuse with a clear message" and `llm.ErrDryRun` as "skipped". In tests use `llm.NewFake()` (deterministic, no network) and `llm.Wrap(fake, cfg, opts)` to get the real middleware around it.
