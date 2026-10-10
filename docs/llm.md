# LLM access

`internal/llm.Client` is the one Go interface for reaching a language model, so the provider, the cost controls and the privacy rules live in one place. Its consumers are the rubric grader of `eval run`, `review --semantic`, `review calibrate`, `review fix` and `review explain`, the LLM-backed `[[verifiers]]` and `verifiers suggest`, the `review-fix` adapter of `improve run`, the embeddings behind `search` (skill search index), and the diagnostics `llm doctor` (and its `--ping`) and the `llm` section of `ai-rulez doctor`.

**Nothing calls a model unless you turn it on.** `allow_network` defaults to `false`; every call is refused with a message that names the setting.

## Configuration

```toml
# .ai-rulez/config.toml   (allow_network, base_url and api_key_env below only take effect from user scope, see "Trust rule")
[llm]
provider        = "openai"                 # provider prefix; liter-llm routes on provider/model
model           = "gpt-4o-mini"
base_url        = "https://llm-gateway.internal.example/v1"   # optional; see "Data egress"
api_key_env     = "OPENAI_API_KEY"         # the NAME of an environment variable, never a key
embedding_model = "text-embedding-3-small"
max_cost_usd    = 2.00                     # stop before the estimate would exceed this
max_tokens      = 500000                   # prompt + completion, whole run
max_calls       = 200
cache           = true                     # default true
allow_network   = false                    # default false; nothing is sent unless true
```

Other keys: `timeout_seconds` (default 60, covers the retries, at most 3600), `max_retries` (default 3, `-1` disables, at most 10), `price_input_per_mtok` and `price_output_per_mtok` (USD per million tokens; override the built-in price table, needed for cost limits on models the table does not know; user scope only, see below). An override prices only the model you set in user scope (`model` in the user config file or `AI_RULEZ_LLM_MODEL`); a model a repository picks, or a per-request model, is priced from the built-in table and, under a cost limit, refused when the table does not know it. Costs and prices must be finite: TOML's `nan` and `inf` are an `AR9L0` error, and a non-finite repository cap never replaces yours.

### Trust rule

A committed repository config is attacker-controlled input: cloning a repository must not switch on network use, choose the host a prompt goes to, or pick which environment variable is sent as a credential. So, like `[telemetry]`:

| Key | Repository config and `config.local.*` | User config file / `AI_RULEZ_LLM_*` |
| --- | --- | --- |
| `allow_network`, `base_url`, `api_key_env`, `allow_plain_http`, `plain_http_hosts`, `price_input_per_mtok`, `price_output_per_mtok` | **ignored**, reported by `llm doctor`, `ai-rulez doctor` and strict finding `AR9L1` | honoured |
| `provider`, `model`, `embedding_model`, `cache`, `timeout_seconds`, `max_retries` | honoured, with one exception for provider routing (below) | honoured |
| `max_cost_usd`, `max_tokens`, `max_calls` | honoured, but can only tighten: the lower non-zero value of repository and user wins | honoured |

**Provider routing and the key.** The `provider` (or a `provider/` prefix in `model`) decides which service receives the request, and with it the API key. So when the key variable comes from user scope but the provider or model prefix comes only from the repository config, ai-rulez refuses to start (`AR9L0`) and `llm doctor` reports it; set `provider` and `model` in the user config file or `AI_RULEZ_LLM_PROVIDER` / `AI_RULEZ_LLM_MODEL`. The check also applies when you have no user config file at all and configure the key only through `AI_RULEZ_LLM_*`: a repository `provider` or `model` prefix is still repository-chosen routing unless `AI_RULEZ_LLM_PROVIDER` / `AI_RULEZ_LLM_MODEL` set it. A repository `model` whose prefix equals the provider you configured in user scope is fine. A repository `embedding_model` with a `provider/` prefix is repository-chosen routing too. The refusal holds without `api_key_env` as well: liter-llm then reads the routed provider's own key variable (`OPENAI_API_KEY`, ...), so the repository would still pick which of your keys is sent and billed. The same rule covers a per-request model (a verifier's `llm.model`, `[search.embeddings] model`): ai-rulez sends a prefixed request model only when its prefix is the configured provider, pins a bare one to that provider, and otherwise refuses the call with `AR9L0` before anything is sent.

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

`validate`, the JSON schema and `ai-rulez doctor` reject a secret where a variable name belongs (`api_key = ...`, or a key-looking `api_key_env` such as `sk-proj-...` or `AKIA...`), credentials or a query string in `base_url`, plain `http://` to a non-loopback host when an API key is sent (use `https`; `localhost`, `127.0.0.1` and `::1` may use `http`; see "Plain-http gateways" for the explicit opt-in), `allow_plain_http` without `plain_http_hosts`, a `max_retries` above 10, a `timeout_seconds` above 3600, and negative or non-finite (`nan`, `inf`) limits and prices. A message never repeats the offending value, and `llm doctor`, `ai-rulez doctor` and the JSON report show `api_key_env` only when it is a valid variable name. The literal-secret key check (`api_key = ...`) covers both `config.toml` and `config.local.*`. A repository key the trust rule ignores is reported as `AR9L1`.

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

or `AI_RULEZ_LLM_ALLOW_PLAIN_HTTP=1` with `AI_RULEZ_LLM_PLAIN_HTTP_HOSTS=gateway.internal:8080`. A key is sent only to a host on the list. The match is exact and case-insensitive on `host` or `host:port` as it appears in `base_url`: an entry `gateway.internal` does not cover `http://gateway.internal:8080`, and `gateway.internal:8080` does not cover another port. An HTTP redirect is refused rather than followed (see "Data egress"). `llm doctor` prints a warning while the opt-in is in use (the same warning is in `ai-rulez doctor`). The trade-off: the key and every prompt cross that network segment unencrypted, so anyone on the path can read or replay them. Prefer `https`, or a loopback tunnel (`ssh -L 8080:gateway.internal:8080`, then `base_url = "http://127.0.0.1:8080/v1"`, which needs no opt-in).

## Commands

```text
ai-rulez llm doctor [--ping] [--format json]   liter-llm version, model, base_url host (never the key),
                                        whether the key variable is set, network gate, cache dir, limits
ai-rulez llm estimate <file>            approximate tokens and worst-case cost of sending the file; no call
```

`llm doctor` is also a section of `ai-rulez doctor` (check `llm`, silent when there is no `[llm]` table). `--ping` makes one 1-token call and refuses unless `allow_network` is true.

## What it does for every call

Outermost first: network gate and timeout, cache, budget, liter-llm (which retries inside the call).

- **Budget, fail closed.** Before each provider call the guard reserves its worst case (estimated prompt plus completion cap) against `max_cost_usd`, `max_tokens` and `max_calls` in one atomic step, counting what calls still in flight hold, so concurrent calls cannot jointly overshoot a limit; it refuses if the worst case does not fit. When the call ends the reservation is replaced by the actual usage. A request without `max_tokens` gets a 1024-token completion cap while a budget is active. If a provider reports no usage, the worst case is charged. Thinking tokens count as completion: a reply is charged at least its `total_tokens` (Gemini leaves its thinking out of `completion_tokens` but bills it), and `completion_tokens_details.reasoning_tokens` larger than the completion is added to it. liter-llm 2.2.0 and later (upstream #253) report Gemini's thinking tokens in `completion_tokens` and `total_tokens` on all three Gemini routes (`gemini/`, `google_ai/`, `vertex_ai/`), so a call settles on the real reported usage like any other; only a successful reply with no usage at all is charged the worst case. Older liter-llm builds drop `thoughtsTokenCount` and under-report Gemini spend, so keep the pin at v2.2.0 or later. A call is priced by the model you requested, never by the model name the provider reports (a reported name can only raise the price). With `max_cost_usd` set and no known price for the requested model, the call is refused. A rejection the provider documents as unbilled (HTTP 400, 401, 403, 404, 422, or an auth, config or budget error raised before anything was sent) releases its tokens and cost and keeps only the call count. Everything else may have run upstream, so the worst case stays charged: a 429, any 5xx (including a 504 gateway timeout), a timeout, a transport failure, an undecodable reply, a cancelled context. liter-llm's own retries happen inside one call: the budget reserves the worst case once, counts one call and settles on the usage of the reply that came back (an attempt the provider rejected is not billed; one that timed out may have been, which is why `max_retries` is capped at 10 and a timeout charges the worst case). Cache hits cost nothing. Token counts are estimates from UTF-8 length (one token per three bytes), deliberately high: liter-llm's own `CountTokens` downloads a tokenizer from the network on first use, so it is not used for a spend gate. Cached prompt tokens reported by the provider are priced at the cache-read rate.
- **Cache.** Keyed by an endpoint identity (provider, base URL scheme, host and path, the name of the key variable, never its value, and the price table version or override), the model, the request and its `PromptVersion`, so changing the gateway, key variable, prices, model or prompt version misses. Entries hold model output only. They live outside the repository, in the user cache directory `~/.cache/ai-rulez/llm/<project hash>/` (mode 0700, files 0600, written atomically), so nothing a checkout contains can plant one. Every entry carries an HMAC-SHA256 over identity, key and payload, computed with a per-user secret (`$XDG_CONFIG_HOME/ai-rulez/llm-cache.key`, else `~/.config/ai-rulez/llm-cache.key`; 32 random bytes, mode 0600, created on first use; the file must be a regular 0600 file in a directory that is not group- or world-writable, and a loose or symlinked file is replaced or refused; `XDG_CONFIG_HOME` is read from the environment, so do not set it from an untrusted `.envrc`). A modified, truncated, oversized, symlinked or planted entry fails the check, counts as a miss and is deleted; entry reads are size-capped. A missing or damaged secret is regenerated, which invalidates every older entry; if no secret can be stored the cache is off, never unauthenticated. A cache restored from CI (another job, an untrusted branch) is only valid when the secret matches, so a restored cache from a different secret simply misses. Opt out with `cache = false`, `AI_RULEZ_LLM_CACHE=0` or `NoCache` (on the options or a request).
- **Retry.** liter-llm retries 429, 500, 502, 503, 504 and 529 and transport failures up to `max_retries` times (default 3) with jittered exponential backoff, honouring `Retry-After` on 429 and 529. Authentication, context-length, budget and config errors, other 4xx rejections and malformed replies are never retried. A persistent 503 costs exactly 1 + `max_retries` requests (`retry_test.go`).
- **Error classification.** liter-llm returns typed errors (variant sentinels, HTTP status, a transient flag, `Retry-After`); one method (`literLLM.classify`) maps them to `llm.Error` kinds by sentinel, never by message text, and a corpus test (`classify_test.go`) pins the behaviour through a local server so a changed upstream shape fails CI. An exhausted quota (`lit.ErrProviderQuotaExceeded`) is a permanent rate-limit error, and an over-long prompt is a context-length error. A failure that is not a liter-llm error is a permanent provider error, and an error is never written to the cache. An empty reply with no error is a permanent error, never a success.
- **Typed errors.** `errors.Is(err, llm.ErrRateLimit | ErrAuth | ErrContextLength | ErrProvider | ErrBudget | ErrNetworkDisabled | ErrConfig | ErrTimeout)`; `*llm.Error` carries the HTTP status and `Retry-After`.
- **Redaction.** Logs (`llm.WithLogging`) and dry-run output carry counts, sizes, a short content hash, usage and cost, never prompt or completion text and never keys. Provider error text is scrubbed before it enters an error: the configured key is removed by value, key-looking text is masked and a URL loses its query string. liter-llm redacts the configured secret from a provider error body and drops the URL from a network error, but a provider can still echo a bare key-shaped token or a URL with a query (`TODO(liter-llm#259)`, still missing in v2.2.3). A value that is, in its entirety, an environment lookup (`os.environ["X"]`, `os.Getenv("X")`, `process.env.X`, `$X`, `${X}`) names a secret without holding one and is not masked; a lookup that carries a literal, such as `${X:-sk-...}`, is.
- **Dry run.** `Options.DryRun` returns a client that prints what would be sent (model, sizes, estimated tokens and cost; message text only with `ShowContent`) and returns `llm.ErrDryRun`.

## Data egress and residency

When `allow_network` is true, the prompt text of each feature is sent to the configured endpoint. What that is depends on the caller. Today only `llm doctor --ping` sends anything (a one-token `ping`). For a future judge-style feature it would be a rubric and the transcript of an agent session, which can contain source code, file paths, tool output and anything a user typed. ai-rulez adds no other data, and never sends config files, keys or environment.

To keep data inside your network, point `base_url` at a gateway you run (a LiteLLM proxy, a Bedrock or Azure OpenAI gateway in your VPC, Ollama or vLLM on an internal host) and use `api_key_env` for the gateway credential. `llm doctor` shows the host so a reviewer can check it. liter-llm 2.2.2 and later refuse HTTP redirects: a gateway that answers 307 or 308 surfaces as a provider error and the prompt is never re-sent to the redirect target (`TestRedirectsAreNotFollowed`). Only point `base_url` at a host you trust. `base_url` must not embed credentials or a query string (`AR9L0`). The default endpoint (used only when `provider` is `openai` or empty and no `base_url` is set) is `https://api.openai.com/v1`. A key is sent only as an `Authorization: Bearer` header to that host, and only over `https` unless the host is loopback. `base_url`, `allow_network` and `api_key_env` are honoured only from user scope (see the trust rule above).

Model-side retention and training terms are the provider's; check them before sending transcripts that contain private code.

### Cost controls

Set `max_cost_usd`, `max_tokens` and `max_calls` for any feature that loops. Use `ai-rulez llm estimate <file>` to see what a prompt costs first (tokens are estimated at one per three UTF-8 bytes, which overestimates English text and holds for CJK). Prices come from liter-llm's embedded model catalog (`GetModelInfo` and `CompletionCostWithCache`, with context tiers and cache-read rates), which is approximate and only for budget estimates; `internal/pricing` adds a few fail-closed floors (see "Price floors"). Set `price_input_per_mtok` / `price_output_per_mtok` for anything else. Cost in responses is an estimate from that table, not a bill.

## liter-llm

ai-rulez reaches every provider through [liter-llm](https://github.com/xberg-io/liter-llm), a Rust client with one interface for 174 providers (`provider/model` names, keys from the provider's environment variable). Its Go module is `github.com/xberg-io/liter-llm/packages/go/v2`, a cgo wrapper over the Rust library `libliter_llm_ffi`. It is a normal dependency of the root module and the only provider backend: there is no build tag, no nested module and no hand-written HTTP client. An OpenAI-compatible gateway, Ollama or vLLM is a `base_url`; liter-llm keeps a named provider's own transform (Gemini, Vertex, Bedrock, Anthropic, Azure) under it and falls back to a generic OpenAI-compatible one only for an unlisted provider, stripping the `provider/` prefix from the model it sends.

### What liter-llm does and what ai-rulez keeps

| Responsibility | Owner | Where |
| --- | --- | --- |
| Providers, routing by `provider/model`, wire formats, auth headers | liter-llm | `client.go` builds a typed `ChatCompletionRequest` |
| Chat, embeddings, structured output (`response_format`) | liter-llm | JSON-schema output is native on OpenAI and Anthropic (upstream #265) |
| Streaming, rate limits, in-flight limits, cooldown, tracing, cost tracking | liter-llm provides them | unused: ai-rulez makes sequential calls and keeps its own accounting |
| Retries, jittered backoff, `Retry-After` | liter-llm (`max_retries`) | `classify` carries a reported `Retry-After` onto `llm.Error` |
| Error taxonomy (typed variants, status, transient flag) | liter-llm | `classify` maps each variant sentinel to an `llm.Error` kind |
| Usage: prompt, completion, total, reasoning and cached tokens | liter-llm | right, including Gemini thinking tokens from 2.2.0 (#253) and Gemini and Anthropic cache accounting (#254, #255); `usageOf` charges the largest of the reported figures |
| Price catalog, `GetModelInfo`, context tiers, cache-read rate | liter-llm | `internal/pricing` calls it first; floors only where the catalog has no row (below) |
| Per-response cost | partly | the Go binding has `CompletionCostWithCache` only; `Pricing.Cost` applies a `price_*_per_mtok` override first |
| Token counting | liter-llm provides `CountTokens`, but it downloads a tokenizer from the network on first use and fails for Gemini | not used: `internal/tokens` stays (offline, deterministic cl100k_base counts for reports; a conservative byte estimate for the spend gate) |
| Budget | ai-rulez policy | liter-llm's budget is USD-only and charges after the fact; ours reserves the worst case on cost, tokens and calls and fails closed |
| Response cache | ai-rulez policy | outside the repository, HMAC-authenticated per user (liter-llm's cache is in-memory or OpenDAL) |
| Network gate, trust model (user-scope-only keys), routing/key check, dry run, judge, prompt redaction, `AR9L0` validation | ai-rulez policy | `gate.go`, `trust.go`, `redact.go`, `config.go` |
| Response size limit | liter-llm (cap: ai-rulez) | the cap is passed as `max_response_bytes` (32 MiB) through `lit.CreateClientWithOptions`; the body is refused while it is read |
| Redirect refusal | liter-llm | refused since v2.2.2; a 307/308 surfaces as a provider error, see "Data egress" |

### Price floors

`GetModelInfo` is the price source. `internal/pricing` consults its own table only for a model the catalog has no entry for: the short names `haiku`, `sonnet` and `opus` that eval cases use, and the `claude-haiku`, `claude-sonnet` and `claude-opus` families (upstream #262, still missing in v2.2.3). It also drops a provider prefix before the lookup, since `gemini/`, `google_ai/` and `vertex_ai/` names are not found by the catalog (#260). The catalog now matches a model only on its exact name, so an unlisted realtime, audio, tts or transcribe variant is unpriced rather than silently charged at its base model's rate (#260). Each correction carries a `TODO(liter-llm#NNN)` and is deleted when the catalog covers it. A model nothing prices is unpriced, so a cost limit refuses it unless you set `price_input_per_mtok` and `price_output_per_mtok`; a paid model the catalog lists at 0/0 is unpriced too, because `GetModelInfo` reports it as not `PriceKnown` (#276). The price source is `literllm-<version>+floors-<revision>` (`floorsRevision` is `2`) and is part of the cache identity.

### Shims that remain

Each is marked `TODO(liter-llm#NNN)` in the code.

- `internal/llm/client.go`: `scrub` removes a key-shaped token or a URL query a provider can still echo (#259, still missing in v2.2.3); `needsMaxCompletionTokens` sends `max_completion_tokens` to a reasoning model behind a `base_url` (#264, partial: liter-llm renames the field for its own openai and azure providers only).
- `internal/pricing/pricing.go`: the floors for the Claude families and the short names (#262); `bare()` drops a provider prefix before the lookup (#260).

### Linking and distribution

The binding is cgo over a Rust library, so a build needs `CGO_ENABLED=1`, a C toolchain and the native library. What links depends on who builds:

- **Contributors** run `go build ./...`, `go test ./...` and `golangci-lint` with nothing to set up. The binding's module ships the shared library for every platform in its own `.lib/<platform>/` directory and its cgo directives point there with an absolute rpath, so the result runs on the machine that built it. That binary is not portable and nobody should ship it.
- **Release builds** link the **static** archive (`libliter_llm_ffi.a`), which makes each binary one self-contained executable with no sidecar library and no cache directory. `scripts/setup-liter-llm.sh` downloads the release asset for the version pinned in `go.mod`, verifies its `.sha256` sidecar (it fails closed), extracts only `libliter_llm_ffi.a` and prints `CGO_LDFLAGS=-L<dir>`; a `-L` that comes first on the link line and holds no shared library makes `-lliter_llm_ffi` resolve to the archive. `task build` and `task test:release` do this, `scripts/package-release.sh` checks the result with `otool -L` / `ldd` (no `liter_llm` dependency), and CI sets it up with `.github/actions/setup-liter-llm`.

A cgo `#cgo LDFLAGS` shim inside the repository cannot replace the environment variable: the binding's own `-L` and `-lliter_llm_ffi` come first on the link line and resolve to the shared library the module ships, so the binary keeps a load command for it (checked on macOS arm64; `-Wl,-dead_strip_dylibs`, which would drop it, is not allowed in `#cgo` flags). That is why `scripts/setup-liter-llm.sh` supplies the static archive through `CGO_LDFLAGS` rather than an in-repo cgo shim.

Verified against liter-llm (macOS arm64 on the pinned v2.2.3; the other rows on v2.2.0):

| Platform | Result | Verified on |
| --- | --- | --- |
| macOS arm64 | static link, one 42 MB executable, `otool -L` lists system libraries only; tests pass | v2.2.3 |
| Linux arm64 (glibc, Debian bookworm container) | static link, 43 MB, `ldd` lists libm, libgcc_s and libc only | v2.2.0 |
| Linux x86_64, macOS x86_64 | same asset layout; built by the release workflow on native runners | v2.2.0 |
| Windows x86_64 | not verifiable from a macOS host: the asset's static `.lib` is an MSVC archive while Go's cgo uses MinGW; the release workflow builds and smoke-tests it on a Windows runner and falls back to shipping `liter_llm_ffi.dll` beside `ai-rulez.exe` | v2.2.0 |
| Windows arm64, Linux musl | liter-llm v2.2.3 now ships `windows-aarch64` and `linux-*-musl` assets (#267 closed), and its release workflow builds and tests each; ai-rulez has not linked or verified them, and its release matrix stays the five platforms above (Linux glibc only, so a musl-based image needs the glibc binary) | not verified |

Binary size grows by about 35 MB per platform. The release workflow, glibc floor and the wrappers (npm, PyPI, Homebrew) are described in [maintainers/release.md](maintainers/release.md).

### Licensing

liter-llm is MIT licensed (Copyright 2026 Kreuzberg, Inc.). The Go module is a wrapper around a Rust library whose dependency tree carries other licenses (its `deny.toml` allows Apache-2.0, MIT, BSD, ISC, Zlib, MPL-2.0, OpenSSL, among others), and its provider table is derived from LiteLLM's (MIT, attribution in liter-llm's `ATTRIBUTIONS.md`). Every release binary now bundles that code: ship the MIT notice and the third-party notices of the library with it.

Version pin: `github.com/xberg-io/liter-llm/packages/go/v2 v2.2.3` in `go.mod`. v2.2.0 or later is required for exact Gemini thinking-token accounting (`module_test.go` pins that floor).

### Testing

The `internal/llm` tests run the real liter-llm client against local `httptest` servers: no network and no key. With a `base_url` liter-llm keeps the named provider's transform, so the Gemini (native and Vertex) and OpenAI request and usage mappings are asserted offline against a local server (`TestProviderTransformIsKeptWithABaseURL`); Bedrock is liter-llm's own tests' job. The live tests (`AI_RULEZ_LIVE_LLM=1`, `GEMINI_API_KEY`) cover Gemini end to end and are not part of PR CI; the `live-llm` job in `.github/workflows/live.yml` runs `go test ./internal/llm/... ./internal/evals/...` on `workflow_dispatch` and a weekly schedule. It needs the `GEMINI_API_KEY` repository secret (and optionally `ANTHROPIC_API_KEY` and `OPENAI_API_KEY`, unused by the current tests), writes its transcripts to `AI_RULEZ_LIVE_OUT` and uploads them with the test log as an artifact.

Verified live on 2026-10-10 against the pinned liter-llm v2.2.3: `AI_RULEZ_LIVE_LLM=1` with `GEMINI_API_KEY`, chat model `gemini-2.5-flash-lite` and embedding model `gemini-embedding-001`; `go test ./internal/llm/... ./internal/evals/...` passes. Thinking-token settlement charged the reply's reported `total_tokens` (prompt 35, completion 414). That run also exposed a live-only test bug, since fixed: `TestLiveBudgetMaxCallsAndUnknownPriceWithCostCap` used a priced model for its unknown-price branch, so the branch never exercised an unpriced model.

## Using it from a feature

```go
lc, err := cfg.ResolvedLLM()            // cfg is the loaded *config.Config; env overrides applied
if err != nil { return err }
client, err := llm.New(lc, llm.Options{ConfigDir: cfg.ConfigDir})
if err != nil { return err }            // AR9L0; a disabled network surfaces on the first call
defer client.Close()

v, err := llm.Judge(ctx, client, rubric, transcript)   // v.Score in [0,1], v.Rationale
```

`llm.Judge` uses structured output at temperature 0 (omitted for an OpenAI reasoning model, which rejects any temperature but its default) and `llm.JudgePromptVersion` as the cache version. It asks for at least `llm.DefaultJudgeCompletionTokens` (2048) completion tokens, because a reasoning model spends its thinking out of that budget before the verdict (`JudgeOptions.MinCompletionTokens` raises it). Change the judge prompt or schema and bump that constant. The transcript is sent as data between two marker lines that carry a token derived from the request's own content (so the transcript cannot predict or close it), and the system prompt tells the model to ignore instructions inside it. The reply must be exactly one JSON object with both `score` (in [0,1]) and `rationale`; anything else is a provider error, and a reply that fails that check is never written to the cache (a cached one is treated as a miss). Before sending, the rubric and transcript are scanned for secret-looking text (bearer tokens, `sk-...`, `AKIA...`, `api_key=...`, and every credential shape the security scan `AR001` recognises, such as `AIza...`, `ghp_...`, `xoxb-...` and private key blocks); `Judge` refuses when it finds any, and `llm.JudgeWith(..., llm.JudgeOptions{RedactSecrets: true})` sends them masked instead. The scan is a pattern match, not a guarantee. A feature must treat `errors.Is(err, llm.ErrNetworkDisabled)` as "refuse with a clear message" and `llm.ErrDryRun` as "skipped". In tests use `llm.NewFake()` (deterministic, no network) and `llm.Wrap(fake, cfg, opts)` to get the real middleware around it.
