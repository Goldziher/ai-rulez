# Run generate when a remote session starts

A remote (cloud) agent session starts from a fresh clone. The generated files are only there when they are
committed, and they are only current when nobody changed a source since the last `generate`. `[session_start]`
closes that gap without a hand-written hook: `ai-rulez generate` adds a `SessionStart` hook to
`.claude/settings.json` that runs `ai-rulez session-start`, which regenerates the outputs and gives the agent a
short briefing.

It is on by default for a project that generates for Claude Code, and it does nothing in a local session.

```toml
[session_start]
# generate = true        # false removes the hook (and the prompt with it)
# when = "remote"        # "remote": act only when CLAUDE_CODE_REMOTE is set; "always": every session
# mode = "auto"          # auto | locked | frozen | plain
# timeout = 120          # seconds the whole run may take before it is killed
# version = "5.6.3"      # pin npx and uvx to a release; default is the latest published one
# command = ["ai-rulez"] # replaces the launcher; runs without a shell; excludes version
# prompt = true          # inject the short briefing
# prompt_text = "..."    # replaces the briefing; {{server}} is the ai-rulez MCP server's name
```

The table is separate from `[[hooks]]` for the same reason `[guard]` is: TOML does not allow `[hooks]` beside
the `[[hooks]]` array. The hook is synthesized at generation time and is never written back to `config.toml`.

## What is written

`generate` merges one entry into the `SessionStart` array of `.claude/settings.json` and leaves the rest of the file
alone (other hooks, other keys, comments). Turning the feature off, or `ai-rulez clean`, removes only that entry.

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "sh -c 'case \"${CLAUDE_CODE_REMOTE:-}\" in 1|[Tt][Rr][Uu][Ee]) ;; *) exit 0;; esac; if command -v ai-rulez ... exit 0'",
            "timeout": 150
          }
        ]
      }
    ]
  }
}
```

The `timeout` is the run timeout plus 30 seconds, so the command's own bound always fires first.

A project that already declares a `SessionStart` hook running `ai-rulez session-start` keeps its own entry and gets
no second one. Another `SessionStart` hook is unrelated and stays next to the managed one.

Only Claude Code gets the hook. `CLAUDE_CODE_REMOTE` is a Claude Code variable, and Claude Code is the harness
that documents `SessionStart` output as session context; a project that generates for no Claude preset gets nothing,
and no warning.

## It has to be committed

A remote session sees only committed files. The hook exists in the clone only when the generated
`.claude/settings.json` is committed. If you ignore `.claude/` (for example with `gitignore = true`), the clone has
no hook and nothing runs. `ai-rulez doctor` warns about both cases: settings that lack the hook, and settings that
git ignores.

## Fresh clones

A remote session starts from a clone that has the committed outputs and none of the machine-local files, among them
`.ai-rulez/.generated-manifest.local.json`, which records which hook entries ai-rulez wrote. `generate` in such a clone
leaves a committed `.claude/settings.json` as it is: the hook already in it is rendered again, not taken back. The same
holds for `lock --check`, `validate` and `generate --check`. `ai-rulez.lock` does not pin a settings document that holds
hook groups ai-rulez owns, because a clone cannot tell the file is wholly ai-rulez's.

## How the hook finds ai-rulez

The hook is a short `sh` script that tries, in order, and stops at the first that runs:

1. `ai-rulez` on `PATH`: `ai-rulez session-start`
2. `npx -y ai-rulez@latest session-start`
3. `uvx --from ai-rulez@latest ai-rulez session-start`

The fallbacks run the latest published release, so the generated line does not change when ai-rulez does. Set
`version` in the table to pin one (`npx -y ai-rulez@<version>`, `uvx --from ai-rulez==<version>`); it is never a
range. An attempt that cannot start, or exits non-zero, falls through to the next. When none runs the script prints
one line on stderr and exits 0, so the session starts. `ai-rulez doctor` prints this order.

The `npx` and `uvx` attempts run from a new, empty private directory under `TMPDIR` (or `/tmp`), not from the
project, so a project `.npmrc`, `node_modules/.bin` or uv configuration cannot substitute the package that runs. The
script exports `CLAUDE_PROJECT_DIR` first (keeping the harness's value), and `session-start` reads the project from
it, then from the hook payload's `cwd`, so it never depends on its own working directory. The installed `ai-rulez`
runs in the project directory. The lock file that serializes two hooks lives in a private per-user directory of the
temp directory, and the failure reason injected into the agent's context is cut to a short line of letters, digits
and basic punctuation.

`when = "remote"` puts the environment check at the top of the script, before any launcher: on a developer's machine
the hook returns immediately and never makes `npx` download anything. `CLAUDE_CODE_REMOTE` is `true` in a remote
session; `1` and `true` (any case) count.

`command = ["ai-rulez"]` replaces the script with `ai-rulez session-start` run directly, with no shell. Use it when
the environment installs the binary, and on Windows hosts, where the default `sh` launcher needs a POSIX shell (Claude
Code on Windows uses Git Bash when it is installed). The command checks `CLAUDE_CODE_REMOTE` itself, so `when` is
honoured either way.

## What the command does

`ai-rulez session-start` reads the hook payload from stdin (empty or invalid input is fine) and decides:

| Step | Behaviour |
| ---- | --------- |
| Project | `CLAUDE_PROJECT_DIR`, else the payload's absolute `cwd`, else the working directory; the configuration is found by walking up to the git work tree. No configuration: silent no-op. Outside a work tree: one line on stderr. |
| Gate | `when = "remote"` (default) needs `CLAUDE_CODE_REMOTE`; otherwise exit 0 and print nothing. |
| Generate | On `source` `startup`, `resume` or none. `clear` and `compact` keep the working tree of a running session, so they only get the briefing. |
| Briefing | JSON on stdout, on every source, unless `prompt = false`. |
| Exit | Always 0. |

`generate` runs as a child process (`generate --yes --quiet`), so the timeout kills its whole process tree and its
output cannot reach stdout, which carries only the hook's JSON document. The child gets a scrubbed environment: the
base variables, proxy and certificate settings, and `AI_RULEZ_*` settings. Tokens and other credentials of the session
are not passed on. A lock file in the temp directory (never in the repository, so a read-only checkout works) keeps two
hooks that start together, a start and a resume, from running two generates at once; a lock older than the timeout plus
30 seconds is taken over.

A failure is one line on stderr (`ai-rulez session-start: generate failed: <reason>; generated files may be out of
date`), existing files are left as they were (generate writes atomically), and the briefing says so.

### Modes

| `mode` | generate runs |
| ------ | ------------- |
| `auto` (default) | `--locked` when `ai-rulez.lock` exists, otherwise plain |
| `locked` | `--locked` |
| `frozen` | `--frozen`: only the local cache, verified against the lock |
| `plain` | no lock flag |

A locked run tries `--locked --offline` first. With a warm cache the lock covers everything and no network is used;
otherwise it goes online once. The hook never writes `ai-rulez.lock`: that stays with `ai-rulez lock`. A lock that
disagrees with the sources is a failure of the generate step, reported like any other.

`generate` does not use the lock to change what it writes; `--locked` makes it refuse remote includes and installed
skills that moved from the pinned commits. See the [lock file](lockfile.md).

### Network policy

Remote environments usually reach the network through a policy proxy that allows named hosts. The hook needs
`registry.npmjs.org` for `npx` (or `pypi.org` for `uvx`) when `ai-rulez` is not installed, and the hosts of any remote
`includes`. A request the policy refuses (HTTP 403 or 407) is not retried; the line names the host:
`generate failed: the network policy denied github.com`. Either allow the host, or install `ai-rulez` in the
environment's setup script and use `command = ["ai-rulez"]`. `ai-rulez doctor`, run inside a remote session, probes
the npm registry and reports a denied or unreachable one.

## The briefing

With `prompt = true` (default) the hook returns

```json
{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"..."}}
```

and Claude Code adds `additionalContext` to the start of the conversation. It is built from the configuration, is at
most 2 KiB, has control characters removed, and has no timestamp. It states:

1. The generated rules, skills, agents and commands (harness folders, `AGENTS.md`, `CLAUDE.md`) come from the sources in
   the configuration directory; a change goes there and `ai-rulez generate` renders it.
2. When the ai-rulez MCP server is enabled (`[mcp] self_server`), that it offers tools to create, update, validate and
   generate the configuration.
3. When skills are served (`delivery = "served"` or a skill source), the "Unlisted skills" section that `[mcp]
   skills_instructions` writes into the instruction files: call `find_skill`, then `load_skill`, on the named server. It
   follows `skills_instructions = false` and `skills_instructions_text`.
4. When the generate step failed, one line saying so.

`prompt_text` replaces 1 to 3 (the failure line stays). `{{server}}` expands to the server's name. `prompt = false`
leaves the briefing out and keeps the regeneration.

Claude Code treats imperative, system-style wording as a possible injection, so the default text states facts.

## Opting out

```toml
[session_start]
generate = false
```

removes the hook on the next `generate`, and with it the briefing. Settings written by hand stay. `when = "always"`
makes the hook act in local sessions as well; that regenerates your working tree at every session start, so use it
with care.

`generate --user` never adds the hook: it belongs to a project's clone.

## Trust

The hook is a command that runs at session start, so it appears in the "new hook commands" notice `generate` prints
the first time it is written and whenever it changes. `--yes` or `AI_RULEZ_ACK_COMMANDS=1` silences the notice. It
also shows up in `generate --check` as drift when the committed settings differ, and in the output claims `clean` uses.
