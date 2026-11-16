# End-to-end skill discovery benchmark

This experiment compares native skill discovery and filesystem reads with ai-rulez MCP discovery and loading in fresh
Claude Code and Codex sessions. The fixture is synthetic. No model runs or results are included.

## Fixture

From the repository root, run:

```sh
work=$(mktemp -d)
python3 -I bench/skill-discovery/generate.py "$work/fixture"
```

Fixture version 1 has 8- and 128-skill catalogs. The flat case places every skill at the repository root. The nested
case places the dispatch skill in `services/amber`, a similar returns skill in the sibling `services/cobalt`, and the other
skills at the root or in the sibling. The native and MCP copies have byte-identical skill contents. Worksheet files
have no answer codes. Expected codes appear only in the skill bodies and in the `tasks.json` answer key outside the workspace.
The scope control asks for a dispatch code in the returns scope. The unrelated control needs no skill.

## Session setup

Use Claude Code **2.1.325** and Codex CLI **0.169.0-alpha.11.1** for the initial series. Record the exact
ai-rulez commit and binary SHA-256, OS, architecture, harness executable versions, full provider model identifiers and revisions,
model effort, authentication route, tokenizer if used, tool settings, and all commands and environment overrides
with every run. Explicitly set the full model ID and effort for each harness and keep them fixed across its arms.
A run on different harness or model versions is a separate series.

Make a fresh copy of the chosen workspace per session, initialize it as a Git repository, and start the harness in
that workspace root for every task. Use an isolated harness home with only authentication and the benchmark
configuration. Disable user/project plugins, hooks, third-party servers, custom instructions, memory, and session
reuse. Keep built-in tools, read permissions, and provider settings the same for paired sessions. Record any
settings enforced by the provider or harness that cannot be disabled. Keep server and harness logs outside the
test workspace. Do not send the answer key or parent fixture path to the model.

- Native uses `claude-native` or `codex-native` with the harness's native discovery and no benchmark MCP server.
- MCP uses `mcp-workspace`. A separate stdio server starts in its corresponding `mcp-catalog` outside the harness
  workspace. It uses the same repository-relative paths as the workspace. Start the server with
  `ai-rulez mcp --serve-skills --discover-skills=repo --read-only --no-watch --offline --tools find_skill,load_skill`.
  Use the absolute path to the pinned binary. Keep the server per session. Before collecting results, verify the
  advertised tools and verify that a lookup with `path: "services/amber/dispatch.txt"` can find and load the dispatch
  skill. The same lookup with `services/cobalt/dispatch.txt` must exclude the dispatch skill. Use a separate server
  instance for this check.
- Attribution controls use the same workspace without skills and without MCP, and the MCP workspace with an empty
  server catalog and the same tool schemas. Use these for startup measurements independently of task sessions.

For Claude, send the prompt on stdin and capture `claude -p --output-format stream-json --verbose
--no-session-persistence --strict-mcp-config --model "$CLAUDE_MODEL" --effort "$CLAUDE_EFFORT"`. In the MCP arm add
`--mcp-config "$MCP_JSON"` with an absolute JSON file containing the server. Use a shell launcher to set the server
working directory if the client cannot set it directly. For Codex, send the prompt on stdin and capture
`codex exec --ephemeral --json --sandbox read-only -m "$CODEX_MODEL"
-c "model_reasoning_effort=\"$CODEX_EFFORT\"" -`. In the MCP arm add the server under `[mcp_servers.skills]` in
the isolated Codex configuration, with the same launcher and arguments as Claude. Keep the native configuration
empty of MCP servers. Do a smoke run for each harness setup and archive the exact command lines and effective configs.
Preserve the harness's discovery defaults, including deferred tool schemas where applicable.

Replay the task prompts verbatim from `tasks.json`. Do not name a skill or tell the agent which discovery interface
to use. For `sibling-walk`, start fresh, then send both turns in the same session. In this arm, replace the
single-shot persistence flags with isolated session storage and resume by session ID for turn two. Verify
the session ID stays the same and that both worksheet paths were visited. Record metadata counts, model
usage, and body loads separately at each turn. Discovery behavior is part of the measurement. Run 20 fresh pairs per harness, catalog size, layout and
applicable task. Alternate arm order per pair and run serially. Apply the same 180-second session timeout and retain
all attempts. Keep startup-attribution runs separate and use the same no-skill prompt, `Reply exactly READY.`
Fix the cache policy before running and report cached and uncached tokens separately wherever the provider exposes them.

## Recording and scoring

Save one JSONL record per attempt with the run and pair IDs, fixture version, exact config and transcript references,
catalog size, layout, task, arm order, timestamps, input/output/cache token counts, first selected skill, loaded skill
names, final answer, and outcome. Use monotonic timestamps and record the model request and tool-event boundary used.

- **Startup latency** is harness process launch to the first model request, when observable. Also record launch to the
  first assistant event as a separate end-to-end measure.
- **Startup tokens** are the input tokens on the first model request, before skill body reads. Compare against
  the empty controls to estimate the native metadata and MCP schema contributions. Report deferred schema tokens
  at the turn when they are exposed. If prompt sections are observable, tokenize them separately, identifying the
  tokenizer. Record unavailable measurements as null. The full MCP payload byte size is a transport metric.
- **Task tokens** are input and output use over the task session, with cache counts broken out. Keep per-turn totals
  so readers can see the first request and subsequent calls. Track skill-body bytes and separately attributable
  body tokens from native reads or MCP tool results where the transcript permits.
- **Time to first use** is prompt submission to the first successful full read of the relevant skill body. Record
  time to first selection separately. A discovery listing without a body does not count as a load.
- **Selection and load** on positive tasks require the expected skill. On controls, record any skill selected or
  loaded. A direct filesystem read of the external catalog in the MCP arm is a protocol violation.
- **Completion** uses exact equality to the expected answer after trimming whitespace. Record completion independently
  of selection and load. Keep wrong answers, wrong-scope loads, missed discovery, tool errors, provider errors,
  protocol violations, timeouts, and instrumentation gaps visible as separate outcomes.

Publish the fixture revision, pinned environment and configuration, scoring code or queries, and attempt records
with aggregate counts and paired deltas. Report rates with all attempts in the denominator. Summarize latency
among observed successes alongside the number of missed or censored events. Record the actual catalog visibility
reported by each harness where available, since metadata limits and nested discovery rules may reduce it.
