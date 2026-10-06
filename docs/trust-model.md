# Trust model

A repository's `.ai-rulez/` configuration is content someone else may have written: you clone it, run
`ai-rulez generate`, and it should not be able to spend your money, send your data anywhere, or run code you did not
ask for. ai-rulez therefore separates **what a repository may set** from **what only you may enable**.

The rule is one sentence: **repository configuration can only tighten; enabling anything that leaves the machine or
runs a process needs user scope (the user config file or an environment variable) or an explicit flag on the
command line.** Everything in a repository is reviewable and pinnable (see [Lock file](lockfile.md)); nothing in a
repository switches on egress or execution by itself.

## Scopes

| Scope | Where | Who controls it |
| --- | --- | --- |
| **Repository** | `.ai-rulez/config.toml` and the local overlay `config.local.*` (it lives in the checkout, so it counts as repository scope) | whoever can commit to the repository |
| **User** | `$XDG_CONFIG_HOME/ai-rulez/config.toml`, else `~/.config/ai-rulez/config.toml` | you, on this machine |
| **Environment** | `AI_RULEZ_*` variables, `DO_NOT_TRACK` | you, in this shell |
| **Command line** | flags such as `--allow-egress`, `--allow-exec`, `--runner-command`, `--token` | you, for this run |

The user config file has one location and one helper, `config.UserConfigFile` (built on `config.UserConfigDir`,
which `generate --user` uses too). The `[llm]` and `[telemetry]` resolvers both read it from there.

## Every knob

| Knob | Repository config | User config / environment | Command line | Precedence (highest first) |
| --- | --- | --- | --- | --- |
| `[llm] allow_network`, `base_url`, `api_key_env`, `allow_plain_http`, `plain_http_hosts`, `price_input_per_mtok`, `price_output_per_mtok` | **ignored**, reported by `llm doctor`, `doctor` and `AR9L1` | honoured (`AI_RULEZ_LLM_*` too) | - | environment, user file, repository (ignored), defaults |
| `[llm] max_cost_usd`, `max_tokens`, `max_calls` | honoured, can only tighten (the lower non-zero value wins) | honoured | - | the lower of repository and user; the environment overrides both |
| `[llm] provider`, `model`, `backend`, `cache`, `timeout_seconds`, `max_retries` | honoured; with the `literllm` backend a repository `provider` or `provider/` model prefix cannot receive a user-scope API key (refused) | honoured | - | environment, user file, repository, defaults |
| `[telemetry] allow_network`, `otlp_endpoint`, `otlp_protocol`, `headers_env`, `service_name`, `include_paths`, `include_session`, `salt_file` | **ignored**, reported by `telemetry doctor` and `AR9K0` | honoured (`AI_RULEZ_TELEMETRY_*` too) | - | kill switches (`AI_RULEZ_TELEMETRY=off`, `DO_NOT_TRACK=1`), environment, user file, repository (ignored), defaults |
| Telemetry consent record (`telemetry enable`, user config dir, mode 0600) | **never read as consent from the repository**: the record lives outside the project and a repository config cannot create or edit it | grants `allow_network` while it still matches the effective endpoint, protocol and exported field set; supplies the endpoint, protocol and the `include_paths`/`include_session` gates the user left unset | `telemetry enable` writes it, `telemetry disable` removes it | an environment `AI_RULEZ_TELEMETRY_ALLOW_NETWORK=0` refuses over the record, and an environment or user-file setting of `include_paths`/`include_session` is never overridden by it (an opt-out makes the record stale); kill switches and an organization policy win over it |
| `[telemetry] enabled`, `sample` | honoured (a repository can at most switch on the local log) | honoured | - | as above |
| Git credentials for private includes, installed skills and skill sources | **no key exists**: a repository cannot name a token or a credential variable | `AI_RULEZ_GIT_TOKEN` | `--token` | flag, environment |
| Scanner commands and egress (`[[lint.external]]`, `egress`, `env_pass`) | may **declare** a scanner and its `egress` (declaring `egress = false` scrubs the environment; a network flag on such a scanner is refused) | - | `validate --external` runs scanners; `--allow-egress=<name>` lets an `egress = true` scanner run | the command line decides; without `--external` nothing runs, without `--allow-egress` an egress scanner is blocked (`AR9E4`) |
| Skill search (`[search]`, `[search.embeddings]`) | `mode`, `fields`, `fusion`, `index_dir` (must stay inside the config directory) and the rest are honoured: they only change ranking, and a hybrid search still needs the user-scope `[llm] allow_network`. `[search.embeddings] command` runs a program, so it is **ignored** with a warning | `[search]` keys honoured; `[search.embeddings] command` honoured; `AI_RULEZ_SEARCH_MODE` | `--mode`; `--allow-exec` honours a repository `command` for that run | environment and flags, user file, repository; the MCP server never honours a repository `command`. See [Skill search](search.md#data-egress) |
| Verifiers (`[[verifiers]]`, `.ai-rulez/verifiers/`) | declares predicates. Content predicates are read-only; a `command` predicate and an `llm` predicate may be **declared** but nothing runs or is sent without the flags on the right. Changed file names are passed to a command with `./` before a leading `-`, so a file name cannot become an option | - | `verifiers run` and `verifiers test` run `command` predicates only with `--allow-exec` (or `AI_RULEZ_VERIFIERS_ALLOW_EXEC=1`); `llm` predicates only with `--allow-llm` plus the user-scope `[llm] allow_network`; an included verifier needs `[verifiers_settings] trust_exec_from` and a lock pin | the command line decides; `generate`, `validate`, hooks and the MCP `run_verifiers` tool never run a command or call a model (a refused `command` is `AR9H3`, an `llm` verifier is skipped, `AR9H4`); `verifiers suggest --write` creates a new file and refuses a symlinked `verifiers/` directory |
| Eval execution (`command` runner, `command_exit` assertions) | cases are files in the repository, but they only run when asked | - | `--runner-command` picks the shell command; `--allow-exec` lets `command_exit` assertions execute | the command line; without `--allow-exec` a `command_exit` assertion is not run |
| Hooks installation (`[[hooks]]`, `telemetry hook`) | `[[hooks]]` are repository content that `generate` writes into the harness settings files (committed, reviewed, pinned by `ai-rulez.lock`, scanned by the security rules); a hook that leaves the project cannot be pinned and fails `lock --check` | `generate --user` installs user-level hooks from the user config | `telemetry hook` only prints; nothing is installed until you merge it or add it to `[[hooks]]` | review in the pull request; the lock detects a changed script |
| Remote sources (`[[includes]]`, `[[installed_skills]]`, `[[skill_sources]]`) | may name a source; `http://` is rejected (https, ssh and local paths only) | - | - | a remote content change is caught by the lock digest; imported content is security-scanned before `generate` writes it |
| `[lock] enforce` | the repository's own policy; defaults to on whenever `ai-rulez.lock` exists | - | `generate --locked` / `--frozen` require the lock whatever this says | flag, then `[lock] enforce` |
| `[lint.security] scan_imports` | honoured; on by default; `off` opts out for that repository | - | - | the repository's setting, never weaker than the organization policy |
| Organization policy (`--policy`, `AI_RULEZ_POLICY`, managed path) | **not read**: a policy is never discovered inside the repository, so a pull request cannot delete it | the policy file (`AI_RULEZ_POLICY`, or the managed path `/etc/ai-rulez/policy.toml` and its macOS and Windows equivalents) | `--policy <file>` | the folded policy bounds the repository: allowlists intersect, denylists union, severities and switches only tighten; a loosening attempt is clamped and reported as `AR740`-`AR745` (see [Organization policy](policy.md)) |

Two things are deliberately not knobs: a repository cannot turn on network access for the model, and it cannot give
itself credentials. A hostile checkout can therefore not spend your API budget, exfiltrate a prompt to its own
endpoint, or make a scanner or an eval run a program.

## Git credentials

A token (`AI_RULEZ_GIT_TOKEN` or `--token`) is sent only to hosts you allowlist, never to a host a repository
names on its own: it goes to `github.com` by default, or to the comma-separated hosts in `AI_RULEZ_GIT_TOKEN_HOSTS`
(environment only, which replaces the default). It travels as a per-command `Authorization` header scoped to
that host, so it is in neither the URL, the process arguments, nor the cached clone's `.git/config`. A repository
that names another HTTPS host gets no token and a warning. Plain `http://`, `git://`, ssh and file sources never
receive it. Use a token with the least scope that can read what you include. Skill sources run git with no credential helper and
no prompt, so they fetch public repositories or use an SSH agent.

## What the checks cover

The trust rule governs what ai-rulez itself reads and what it generates. The repository-declared environment check (a
hook or MCP server `env` that sets `AI_RULEZ_TELEMETRY_*` or `AI_RULEZ_LLM_*` to redirect export or model traffic) covers
only the configs ai-rulez generates. A hand-written `.claude/settings.json`, `.mcp.json` or any other harness file in
the repository is outside its control: review those files like code, and set `AI_RULEZ_TELEMETRY=off` or
`DO_NOT_TRACK=1` when you open a checkout you do not trust.

The same limit applies to the response cache and the sink command: the `[llm]` cache lives under your user cache
directory and is authenticated with a per-user secret, and `usage record --sink-command` always runs with a 3 second
timeout and an output cap, but a sink command is code you chose to run.

## What `doctor` and the strict rules tell you

- `ai-rulez doctor` and `llm doctor` list repository `[llm]` keys that were ignored and where the user file was read.
- `telemetry doctor` lists ignored `[telemetry]` keys and every validation problem.
- `AR9L1` and `AR9K0` report the same under `validate --strict`.

## Checklist for a new privileged setting

If you add a setting that reaches the network, spends money or starts a process:

1. Read it only from the user config file (`config.UserConfigFile`), the environment or a flag.
2. If the repository may also set it, let it only lower a limit, never raise one.
3. Report a repository value that was ignored by name.
4. Add a row to the table above.
