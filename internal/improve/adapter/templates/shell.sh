#!/bin/sh
# Template optimizer for `ai-rulez improve run <skill> --with ./optimize.sh` (protocol v1, docs/improve.md).
# improve starts this script without a shell wrapper, in the run's workspace directory, with the request
# on standard input and a scrubbed environment (PATH, locale and what you pass with --env-pass). It must
# print one JSON response on standard output and may edit only <skill>/SKILL.md and <skill>/references/**.
set -eu

request=$(cat)

# The skill directory, relative to the workspace (the current directory). Needs jq; any JSON tool works.
dir=$(printf '%s' "$request" | jq -r '.skill.dir')

# The train cases are in "$request" (.train_cases). Held-out cases are never sent.
# Replace the next line with the tool that rewrites the skill. It runs as you: keep it away from ../ and
# declare the hosts it talks to with --egress so they appear in the consent summary and the report.
your-tool rewrite --skill "$dir/SKILL.md" --cases-from-stdin <<EOF
$(printf '%s' "$request" | jq -c '.train_cases')
EOF

# Report what you changed and what it cost. "changed" is informational (improve computes the real diff);
# an optimizer that reports no cost is flagged in the report.
printf '{"version":1,"summary":"rewrote the skill with your-tool","changed":["SKILL.md"],"cost_usd":0}\n'
