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
| `[llm] allow_network`, `base_url`, `api_key_env`, `price_input_per_mtok`, `price_output_per_mtok` | **ignored**, reported by `llm doctor`, `doctor` and `AR9L1` | honoured (`AI_RULEZ_LLM_*` too) | - | environment, user file, repository (ignored), defaults |
| `[llm] max_cost_usd`, `max_tokens`, `max_calls` | honoured, can only tighten (the lower non-zero value wins) | honoured | - | the lower of repository and user; the environment overrides both |
| `[llm] provider`, `model`, `backend`, `cache`, `timeout_seconds`, `max_retries` | honoured | honoured | - | environment, user file, repository, defaults |
| `[telemetry] allow_network`, `otlp_endpoint`, `otlp_protocol`, `headers_env`, `include_paths`, `include_session`, `salt_file` | **ignored**, reported by `telemetry doctor` and `AR9K0` | honoured (`AI_RULEZ_TELEMETRY_*` too) | - | kill switches (`AI_RULEZ_TELEMETRY=off`, `DO_NOT_TRACK=1`), environment, user file, repository (ignored), defaults |
| `[telemetry] enabled`, `service_name`, `sample` | honoured (a repository can at most switch on the local log) | honoured | - | as above |
| Git credentials for private includes, installed skills and skill sources | **no key exists**: a repository cannot name a token or a credential variable | `AI_RULEZ_GIT_TOKEN` | `--token` | flag, environment |
| Scanner commands and egress (`[[lint.external]]`, `egress`, `env_pass`) | may **declare** a scanner and its `egress` (declaring `egress = false` scrubs the environment; a network flag on such a scanner is refused) | - | `validate --external` runs scanners; `--allow-egress=<name>` lets an `egress = true` scanner run | the command line decides; without `--external` nothing runs, without `--allow-egress` an egress scanner is blocked (`AR9E4`) |
| Verifiers (`[[verifiers]]`) | declares read-only predicates: never a network call, never a process, never a write | - | `verifiers run` | a `command` predicate is rejected by the validator; if one is ever added it is user scope only, like `--runner-command` |
| Eval execution (`command` runner, `command_exit` assertions) | cases are files in the repository, but they only run when asked | - | `--runner-command` picks the shell command; `--allow-exec` lets `command_exit` assertions execute | the command line; without `--allow-exec` a `command_exit` assertion is not run |
| Hooks installation (`[[hooks]]`, `telemetry hook`) | `[[hooks]]` are repository content that `generate` writes into the harness settings files (committed, reviewed, pinned by `ai-rulez.lock`, scanned by the security rules); a hook that leaves the project cannot be pinned and fails `lock --check` | `generate --user` installs user-level hooks from the user config | `telemetry hook` only prints; nothing is installed until you merge it or add it to `[[hooks]]` | review in the pull request; the lock detects a changed script |
| Remote sources (`[[includes]]`, `[[installed_skills]]`, `[[skill_sources]]`) | may name a source; `http://` is rejected (https, ssh and local paths only) | - | - | a remote content change is caught by the lock digest; imported content is security-scanned before `generate` writes it |
| `[lock] enforce` | the repository's own policy; defaults to on whenever `ai-rulez.lock` exists | - | `generate --locked` / `--frozen` require the lock whatever this says | flag, then `[lock] enforce` |
| `[lint.security] scan_imports` | honoured; on by default; `off` opts out for that repository | - | - | the repository's setting |

Two things are deliberately not knobs: a repository cannot turn on network access for the model, and it cannot give
itself credentials. A hostile checkout can therefore not spend your API budget, exfiltrate a prompt to its own
endpoint, or make a scanner or an eval run a program.

## Git credentials

A token (`AI_RULEZ_GIT_TOKEN` or `--token`) is sent to the host of every HTTPS include or installed skill the
repository names. Use a token with the least scope that can read what you include, and review the hosts a repository
names before running `generate` with a token in the environment. Skill sources run git with no credential helper and
no prompt, so they fetch public repositories or use an SSH agent.

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
