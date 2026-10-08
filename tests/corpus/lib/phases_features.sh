#!/usr/bin/env bash
# Phases about the newer output formats: Agent Plugins, Agentic Resource
# Discovery and llms.txt. Each builds a small standalone project from the
# repository's own rules, context and skills (see mk_subproject), so real
# content is exercised without the repository's own configuration. Sourced by
# run.sh.

# sub_commit commits the current directory (a throwaway sub-project) and tags it.
sub_commit() {
  git_commit_all "$1"
}

# plugin_fingerprint prints a digest of the generated Agent Plugins package.
plugin_fingerprint() {
  (find plugin.json mcp.json skills -type f 2>/dev/null | LC_ALL=C sort | xargs shasum -a 256 2>/dev/null | shasum -a 256 | cut -d' ' -f1)
}

# ---------------------------------------------------------------- agent_plugins

phase_agent_plugins() {
  fresh_copy
  cd "$W" || fail "no work dir"
  [ -d .ai-rulez ] || skip "no .ai-rulez content to package"
  local sub="$W/corpus-ap" name="corpus.ap"
  mk_subproject "$sub" "corpus-ap" '[plugin]
name = "corpus.ap"
version = "0.0.1"
description = "Agent Plugins package built by the corpus harness."
runtimes = ["agent-plugins"]'
  cd "$sub" || fail "no work dir"
  sub_commit v0.0.1
  ar generate --plugin
  expect_rc 0 "generate --plugin" || finish
  [ -f plugin.json ] || fail_note "generate --plugin wrote no plugin.json"
  jq -e '.name == "corpus.ap"' plugin.json >/dev/null 2>&1 || fail_note "plugin.json has the wrong name"
  local fp1 fp2
  fp1="$(plugin_fingerprint)"
  ar generate --plugin
  expect_rc 0 "generate --plugin (second run)"
  fp2="$(plugin_fingerprint)"
  [ "$fp1" = "$fp2" ] || fail_note "the package changed between two identical generate --plugin runs"
  ar verify --plugin
  expect_rc 0 "verify --plugin"
  ar validate --strict
  expect_rc_in "validate --strict" 0 2
  if grep -qE 'AR9O[0-5].*(error|ERROR)' "$AR_LOG"; then
    fail_note "validate --strict reports an Agent Plugins error: $(grep -E 'AR9O[0-5]' "$AR_LOG" | head -n 1 | cut -c1-120)"
  fi
  ar generate
  ar lock
  expect_rc 0 "lock"
  sub_commit v0.0.1
  ar publish emit agent-plugins --out "$PH_LOGS/emit1"
  skip_if_gated "publish emit"
  expect_rc 0 "publish emit agent-plugins" || finish
  ar publish emit agent-plugins --out "$PH_LOGS/emit2"
  same_tree "$PH_LOGS/emit1" "$PH_LOGS/emit2" "agent-plugins emitter"
  [ -f "$PH_LOGS/emit1/$name/plugin.json" ] || fail_note "emitter wrote no $name/plugin.json"
  # Round trip: read the emitted package back with convert and rebuild it.
  local rt="$W/corpus-rt"
  cp -R "$PH_LOGS/emit1/$name" "$rt"
  cd "$rt" || fail "no work dir"
  ar convert --from agent-plugins --write
  expect_rc 0 "convert --from agent-plugins --write" || finish
  ar generate --plugin
  expect_rc 0 "generate --plugin after convert"
  if ! cmp -s "$PH_LOGS/emit1/$name/plugin.json" plugin.json; then
    fail_note "plugin.json changed in the emit -> convert -> generate round trip"
  fi
  if ! diff -r "$PH_LOGS/emit1/$name/skills" skills >"$PH_LOGS/diff-roundtrip.txt" 2>&1; then
    fail_note "skills changed in the round trip ($(head -n 1 "$PH_LOGS/diff-roundtrip.txt" | cut -c1-100))"
  fi
  finish "agent-plugins package generated, verified, emitted twice and round-tripped through convert"
}

# ------------------------------------------------------------------------- ard

phase_ard() {
  fresh_copy
  cd "$W" || fail "no work dir"
  [ -d .ai-rulez ] || skip "no .ai-rulez content to publish"
  local sub="$W/corpus-ard"
  mk_subproject "$sub" "corpus-ard" '[plugin]
name = "corpus.ard"
version = "0.0.1"
description = "Package described by the corpus ARD manifest."
runtimes = ["agent-plugins"]

[ard]
publisher = "corpus.example.com"
namespace = "conventions"
base_url = "https://corpus.example.com/ard"'
  cd "$sub" || fail "no work dir"
  ar validate --strict
  expect_rc_in "validate --strict" 0 2
  if grep -qE 'AR9S[0-2]' "$AR_LOG"; then
    fail_note "validate --strict reports an ARD error: $(grep -E 'AR9S[0-2]' "$AR_LOG" | head -n 1 | cut -c1-120)"
  fi
  ar generate --plugin
  ar generate
  ar lock
  expect_rc 0 "lock" || finish
  sub_commit v0.0.1
  ar publish emit ard --out "$PH_LOGS/ard1"
  skip_if_gated "publish emit"
  expect_rc 0 "publish emit ard" || finish
  ar publish emit ard --out "$PH_LOGS/ard2"
  same_tree "$PH_LOGS/ard1" "$PH_LOGS/ard2" "ard emitter"
  local manifest="$PH_LOGS/ard1/ard.json" n
  jq -e . "$manifest" >/dev/null 2>&1 || fail "ard.json is not JSON"
  n="$(jq '.entries | length' "$manifest")"
  [ "${n:-0}" -ge 1 ] || fail_note "ard.json has no entries"
  jq -e '.entries | all((has("url") | not) != (has("data") | not))' "$manifest" >/dev/null 2>&1 ||
    fail_note "an ard entry does not carry exactly one of url and data"
  jq -e '.entries | all(.identifier | startswith("urn:air:corpus.example.com:conventions:"))' "$manifest" >/dev/null 2>&1 ||
    fail_note "an ard identifier does not follow urn:air:<publisher>:<namespace>:<name>"
  ar publish --dry-run --emit ard --dist "$PH_LOGS/dist"
  expect_rc 0 "publish --dry-run --emit ard"
  finish "ard.json with ${n:-0} entries emitted twice identically and validated"
}

# --------------------------------------------------------------------- llms_txt

phase_llms_txt() {
  fresh_copy
  cd "$W" || fail "no work dir"
  [ -d .ai-rulez ] || skip "no .ai-rulez content to index"
  local sub="$W/corpus-llms"
  mk_subproject "$sub" "corpus-llms" '[llms_txt]
full = true' '["claude", "llms-txt"]'
  cd "$sub" || fail "no work dir"
  git_commit_all
  ar generate
  expect_rc 0 "generate" || finish
  [ -f llms.txt ] || fail "generate wrote no llms.txt"
  [ -f llms-full.txt ] || fail_note "full = true wrote no llms-full.txt"
  head -n 1 llms.txt | grep -q '^# ' || fail_note "llms.txt does not start with an H1"
  grep -qE '^- \[[^]]+\]\([^)]+\)' llms.txt || fail_note "llms.txt lists no links"
  ar validate --strict --analyzer llmstxt
  expect_rc 0 "validate --strict --analyzer llmstxt on the generated llms.txt"
  if grep -qE 'AR9P[0-9]' "$AR_LOG"; then
    fail_note "llms.txt findings: $(grep -oE 'AR9P[0-9]' "$AR_LOG" | sort -u | tr '\n' ' ')"
  fi
  local sum1 sum2
  sum1="$(shasum -a 256 llms.txt llms-full.txt | shasum -a 256)"
  ar generate
  sum2="$(shasum -a 256 llms.txt llms-full.txt | shasum -a 256)"
  [ "$sum1" = "$sum2" ] || fail_note "llms.txt changed between identical runs"
  ar generate --check
  expect_rc 0 "generate --check"
  # validate --strict must catch a broken file.
  printf 'no title here\n' >llms.txt
  ar validate --strict --analyzer llmstxt
  if [ "$AR_RC" -eq 0 ]; then
    fail_note "validate --strict accepted an llms.txt without a title"
  fi
  finish "llms.txt and llms-full.txt generated, deterministic, valid; a broken file is rejected"
}
