# Run generate in remote sessions

A remote (cloud) agent session starts from a fresh clone of the repository. Generated files are there only when they
are committed, and they are current only when nobody changed a source since the last `generate`. Run
`ai-rulez generate` when the session starts, and the agent always works with the outputs of the committed sources.

```bash
# the environment's setup script
npm install -g ai-rulez
ai-rulez generate --yes
```

## Choosing an approach

| Approach | Harnesses | Runs |
| -------- | --------- | ---- |
| [A setup script](#setup-script) | any harness whose cloud environment runs a setup script | once, when the environment is built |
| [`[session_start]`](#the-session_start-hook) | Claude Code (on by default) | at every remote session start |
| [A `SessionStart` hook of your own](#a-sessionstart-hook-for-other-harnesses) | harnesses with a `SessionStart` event | at every session start, local ones included |
| [Commit the outputs](#committed-outputs) | any harness | never; CI checks they are current |

The approaches combine: a project can commit its outputs, check them in CI and still regenerate in Claude Code's
remote sessions.

## Setup script

Most cloud agent environments run a setup script before the agent starts. Install ai-rulez there and run `generate`:

```bash
pip install ai-rulez          # or: npm install -g ai-rulez, uv tool install ai-rulez, brew install goldziher/tap/ai-rulez
ai-rulez generate --yes --locked
```

`--yes` silences the notice about new hook and MCP commands. Use `--locked` when you commit `ai-rulez.lock`: it
fetches exactly the pinned remote includes and installed skills, and fails when the lock does not cover them. Without
a lock file, drop it.

`ai-rulez session-start --when always` is an alternative that never fails the script: it runs `generate` with a
timeout, uses `--locked` automatically when `ai-rulez.lock` exists, and prints one line on stderr when something goes
wrong (see [What the command does](#what-the-command-does)). It also prints the Claude Code briefing JSON on stdout,
which a setup script can ignore; set `[session_start] prompt = false` to leave it out.

The environment's network policy must allow the package registry you install from, and the hosts of any remote
`includes`.

## The `[session_start]` hook

Currently supports Claude Code only: it relies on Claude Code's `CLAUDE_CODE_REMOTE` variable and on its
`SessionStart` output format.

For a project that generates for `claude`, `generate` adds a `SessionStart` hook to `.claude/settings.json` that runs
`ai-rulez session-start`. In a remote session the command regenerates the outputs and gives the agent a short
briefing; in a local session it does nothing. No configuration is needed; these are the keys and their defaults:

```toml
[session_start]
# generate = true        # false removes the hook (and the briefing with it)
# when = "remote"        # "remote": act only when CLAUDE_CODE_REMOTE is set; "always": every session
# mode = "auto"          # auto | locked | frozen | plain
# timeout = 120          # seconds the whole run may take before it is killed
# version = "5.10.0"     # pin npx and uvx to a release; default is the latest published one
# command = ["ai-rulez"] # replaces the launcher; runs without a shell; excludes version
# prompt = true          # inject the briefing
# prompt_text = "..."    # replaces the briefing; {{server}} is the ai-rulez MCP server's name
```

| Key | Type | Default | Meaning |
| --- | ---- | ------- | ------- |
| `generate` | boolean | `true` | Add the hook |
| `when` | string | `"remote"` | `"remote"` acts only when `CLAUDE_CODE_REMOTE` is `1` or `true` (any case); `"always"` acts in every session |
| `mode` | string | `"auto"` | How `generate` uses the lock file; see [Modes](#modes) |
| `timeout` | integer | `120` | Seconds the whole run may take (1 to 900) |
| `version` | string | latest release | The version the `npx` and `uvx` fallbacks run |
| `command` | list | none | Run this launcher (plus `session-start`) directly, without a shell; excludes `version` |
| `prompt` | boolean | `true` | Return the briefing |
| `prompt_text` | string | built in | Replaces the briefing; `{{server}}` expands to the MCP server's name |

`generate = false` with any other key is a validation error. `generate --user` never adds the hook.

### What is written

`generate` merges one entry into the `SessionStart` array of `.claude/settings.json` and leaves the rest of the file
alone. `generate = false`, or `ai-rulez clean`, removes only that entry.

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "sh -c 'case \"${CLAUDE_CODE_REMOTE:-}\" in 1|[Tt][Rr][Uu][Ee]) ;; *) exit 0;; esac; ... exit 0'",
            "timeout": 150
          }
        ]
      }
    ]
  }
}
```

The hook `timeout` is the run timeout plus 30 seconds, so the command's own bound fires first. A project whose own
`[[hooks]]` already run `ai-rulez session-start` on `SessionStart` keeps its group and gets no second one. Other `SessionStart` hooks of yours stay next to the managed one.

The hook exists in a remote clone only when `.claude/settings.json` is committed. With `gitignore = true`, or a
`.gitignore` that covers `.claude/`, the clone has no hook and nothing runs. `ai-rulez doctor` warns about both cases.
In a fresh clone, `generate` leaves the committed hook in place even though the clone has no machine-local manifest.

### How the hook finds ai-rulez

The hook is a short `sh` script that tries, in order, and stops at the first that runs:

1. `ai-rulez session-start` (an `ai-rulez` on `PATH`)
2. `npx -y ai-rulez@latest session-start`
3. `uvx --from ai-rulez@latest ai-rulez session-start`

`version` pins the fallbacks (`npx -y ai-rulez@<version>`, `uvx --from ai-rulez==<version>`). When none of them runs,
the script prints one line on stderr and exits 0, so the session starts. `ai-rulez doctor` prints this order.

With `when = "remote"` the environment check comes first, so on a developer's machine the hook returns immediately and
`npx` downloads nothing. The `npx` and `uvx` attempts run from an empty temporary directory, so a project `.npmrc`,
`node_modules/.bin` or uv configuration cannot substitute the package.

`command = ["ai-rulez"]` replaces the script with `ai-rulez session-start` run directly. Use it when the environment
installs the binary, and on Windows hosts, where the default launcher needs a POSIX shell. The command checks
`CLAUDE_CODE_REMOTE` itself, so `when` still applies.

### The briefing

With `prompt = true` the command returns

```json
{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"..."}}
```

and Claude Code adds `additionalContext` to the start of the conversation. The text is built from the configuration
(at most 2 KiB, no timestamp) and says:

1. that the generated rules, skills, agents and commands come from the sources in the configuration directory, and
   that a change belongs there;
2. when the ai-rulez MCP server is enabled (`[mcp] self_server`), that it offers tools to create, update, validate and
   generate the configuration;
3. when skills are served (`delivery = "served"` or a skill source), how to find and load them, as
   `[mcp] skills_instructions` writes it into the instruction files;
4. when the generate step failed, one line saying so.

`prompt_text` replaces items 1 to 3; the failure line stays. `prompt = false` drops the briefing and keeps the
regeneration.

## A `SessionStart` hook for other harnesses

For a harness with a `SessionStart` event (Codex, Gemini CLI, Cursor, Copilot and others; see
[Hooks and settings](settings.md#events)), declare the hook yourself:

```toml
[[hooks]]
event = "SessionStart"
targets = ["codex", "gemini", "cursor"]
[[hooks.hooks]]
command = "ai-rulez generate --yes --quiet"
timeout = 120
```

The hook runs at every session start, local sessions included, because these harnesses set no variable ai-rulez can
use to tell a remote session apart. The briefing is not available: its JSON format is Claude Code's. Some harnesses
ask you to trust project hooks before they run them (see [Trust and review](settings.md#trust-and-review)).

A `SessionStart` group of your own that runs `ai-rulez session-start` replaces the built-in hook for every harness,
whatever its `targets`. Add `claude` to its `targets` when Claude Code should keep it.

## Committed outputs

Commit the generated files and fail CI when they drift from the sources:

```bash
ai-rulez generate --check   # exits 2 and lists the differing files
```

A fresh clone then has every output, current as of the last commit, with nothing to run. See
[GitHub Actions](github-actions.md).

## What the command does

`ai-rulez session-start` is what the hook runs, and you can run it yourself:

```bash
echo '{"source":"startup"}' | CLAUDE_CODE_REMOTE=true ai-rulez session-start
ai-rulez session-start --when always --generate-mode plain
```

| Step | Behavior |
| ---- | -------- |
| Project | `CLAUDE_PROJECT_DIR`, else the payload's `cwd`, else the working directory; the configuration is found by walking up to the git work tree. No configuration: silent no-op. Outside a work tree: one line on stderr. |
| Gate | Unless `when` is `"always"`, `CLAUDE_CODE_REMOTE` must be `1` or `true`; otherwise it exits 0 and prints nothing. |
| Generate | On the payload `source` `startup`, `resume` or none. `clear` and `compact` keep the session's working tree, so they only get the briefing. |
| Briefing | The JSON document on stdout, unless `prompt = false`. |
| Exit | Always 0. |

Flags override the configuration: `--when`, `--generate-mode` (the `mode` key) and `--timeout` (seconds).

`generate` runs as a child process (`generate --yes --quiet`), so the timeout kills its whole process tree and stdout
carries only the JSON document. The child gets a scrubbed environment: the base variables, proxy and certificate
settings, and `AI_RULEZ_*` settings, but no tokens or other credentials of the session. A lock in the temporary
directory keeps two hooks that start together from running two generates at once.

A failure is one line on stderr, the existing files stay as they were, and the briefing says so:

```text
ai-rulez session-start: generate failed: <reason>; generated files may be out of date
```

### Modes

| `mode` | `generate` runs with |
| ------ | -------------------- |
| `auto` (default) | `--locked` when `ai-rulez.lock` exists, otherwise no lock flag |
| `locked` | `--locked` (fails when `ai-rulez.lock` is missing) |
| `frozen` | `--frozen`: only the local cache, verified against the lock |
| `plain` | no lock flag |

A locked run tries `--locked --offline` first and goes online once only if that fails. The command never writes
`ai-rulez.lock`; that stays with `ai-rulez lock`. See the [lock file](lockfile.md).

### Network policy

Remote environments usually reach the network through a proxy that allows named hosts. The hook needs
`registry.npmjs.org` for `npx` (or `pypi.org` for `uvx`) when `ai-rulez` is not installed, plus the hosts of any
remote `includes`. A request the policy refuses (HTTP 403 or 407) is not retried, and the error names the host:

```text
ai-rulez session-start: generate failed: the network policy denied github.com; generated files may be out of date
```

Allow the host, or install `ai-rulez` in the setup script and set `command = ["ai-rulez"]`. `ai-rulez doctor`, run in a
remote session, probes the npm registry and reports a denied or unreachable one.

## Troubleshooting

| Symptom | Cause | Fix |
| ------- | ----- | --- |
| Nothing runs in a remote Claude Code session | `.claude/settings.json` is not committed or is gitignored | Commit it; `ai-rulez doctor` reports this |
| `no ai-rulez, npx or uvx could run; skipped` | None of the launchers is installed | Install ai-rulez, Node or uv in the setup script |
| `the network policy denied <host>` | The proxy blocks the registry or an include host | Allow the host, or install ai-rulez and set `command` |
| Regeneration in every local session | `when = "always"`, or a hand-written `SessionStart` hook | Use `when = "remote"`, or a setup script |
| `generate --check` shows drift for the hook | The committed settings differ from what `generate` renders | Run `generate` and commit `.claude/settings.json` |
