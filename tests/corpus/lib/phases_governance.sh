#!/usr/bin/env bash
# Phases about governance: the lock, signing and approvals, and injected
# configuration (hooks, permissions, roles, verifiers, machine-local overlay).
# Sourced by run.sh.

# first_content_file prints a markdown source file the lock pins (a rule, else
# a context file), for tampering tests.
first_content_file() {
  local d f
  d="$(cfg_dir)" || return 1
  for f in "$d"/rules/*.md "$d"/context/*.md; do
    [ -f "$f" ] && {
      echo "$f"
      return 0
    }
  done
  return 1
}

# ---------------------------------------------------------------------- frozen

phase_frozen() {
  fresh_copy
  cd "$W" || fail "no work dir"
  cfg_path >/dev/null || skip "no ai-rulez configuration"
  prep_hermetic || finish
  local lock sum_before sum_after file
  lock="$(cfg_dir)/ai-rulez.lock"
  ar generate --offline
  expect_rc 0 "generate" || finish
  ar lock
  expect_rc 0 "lock" || finish
  [ -f "$lock" ] || fail "lock command wrote no lock file"
  sum_before="$(shasum -a 256 "$lock" | cut -d' ' -f1)"
  ar generate --frozen
  expect_rc 0 "generate --frozen"
  ar generate --frozen --check
  expect_rc 0 "generate --frozen --check"
  ar generate --locked
  expect_rc 0 "generate --locked"
  ar lock --check
  expect_rc 0 "lock --check"
  [ -f "$lock" ] || fail_note "lock file disappeared during --frozen"
  sum_after="$(shasum -a 256 "$lock" 2>/dev/null | cut -d' ' -f1)"
  [ "$sum_before" = "$sum_after" ] || fail_note "lock file changed during --frozen/--locked runs"
  ar_to "$PH_LOGS/lock-check.json" lock --check --format json
  jq -e . "$PH_LOGS/lock-check.json" >/dev/null 2>&1 || fail_note "lock --check --format json is not JSON"
  # Tampering with pinned content must be caught.
  file="$(first_content_file)"
  if [ -n "$file" ]; then
    printf '\nCORPUS-TAMPER\n' >>"$file"
    ar lock --check
    if [ "$AR_RC" -eq 0 ]; then
      fail_note "lock --check passed after a pinned file was edited"
    fi
    ar lock --diff
    expect_rc_in "lock --diff after tampering" 0 2
    ar lock --content-only
    expect_rc 0 "lock --content-only re-pin"
    ar generate --offline
    expect_rc 0 "generate after re-pin"
    ar lock --check
    expect_rc 0 "lock --check after re-pin and regenerate"
  fi
  finish "frozen/locked generation kept the lock; tampering detected (${HERMETIC_STRIPPED:-0} remote sources stripped)"
}

# --------------------------------------------------------------------- signing

# make_key writes a throwaway ed25519 (else P-256) private key and its public
# key. Usage: make_key <priv> <pub>
make_key() {
  have_cmd openssl || return 1
  openssl genpkey -algorithm ed25519 -out "$1" >/dev/null 2>&1 ||
    openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$1" >/dev/null 2>&1 || return 1
  openssl pkey -in "$1" -pubout -out "$2" >/dev/null 2>&1
}

phase_signing() {
  fresh_copy
  cd "$W" || fail "no work dir"
  cfg_path >/dev/null || skip "no ai-rulez configuration"
  have_cmd openssl || skip "openssl not available to make a throwaway key"
  prep_hermetic || finish
  local keys="$PH_LOGS/keys"
  mkdir -p "$keys"
  make_key "$keys/priv.pem" "$keys/pub.pem" || skip "openssl cannot make a signing key"
  make_key "$keys/other-priv.pem" "$keys/other-pub.pem" || skip "openssl cannot make a second key"
  cp "$keys/pub.pem" ./corpus-signer.pub.pem
  local approvals=1
  if cfg_has '^\[governance\]'; then
    approvals=0
    ok_note "config declares [governance]: approvals not injected"
  else
    cfg_append '[governance]
require_approval = ["local"]
min_approvers = 1'
  fi
  cfg_append '[[signing.trust]]
subject = "approval"
reviewer = "corpus@example.invalid"
key_file = "corpus-signer.pub.pem"'
  ar generate --offline
  expect_rc 0 "generate" || finish
  ar lock
  expect_rc 0 "lock" || finish
  if [ "$approvals" -eq 1 ]; then
    first_item
    local asserted="$FIRST_ITEM" signed=""
    ar approve --list
    signed="$(awk 'NR > 2 && $1 != "" { print $1 ":" $2; exit }' "$AR_LOG")"
    if [ -z "$asserted" ] || [ -z "$signed" ] || [ "$asserted" = "$signed" ]; then
      ok_note "fewer than two pinned items: approval round trip reduced"
      signed="$asserted"
    fi
    if [ -n "$asserted" ]; then
      ar approve "$asserted" --reviewer corpus@example.invalid --note corpus --yes
      expect_rc 0 "approve $asserted"
      ar approve --sign --key "$keys/priv.pem" "$signed" --yes
      expect_rc 0 "approve --sign $signed"
      ar verify --approvals
      expect_rc 0 "verify --approvals"
      expect_grep 'signed' "$AR_LOG" "verify --approvals reports the signed approval"
      ar approve --revoke "$asserted"
      expect_rc 0 "approve --revoke"
    fi
  fi
  ar sign --lock --key "$keys/priv.pem" --public-key-out "$keys/pub-out.pem"
  expect_rc 0 "sign --lock --key" || finish
  [ -f "$(cfg_dir)/ai-rulez.lock.sigstore.json" ] || fail_note "sign wrote no bundle next to the lock"
  if [ -f "$keys/pub-out.pem" ] && ! cmp -s "$keys/pub-out.pem" "$keys/pub.pem"; then
    fail_note "--public-key-out differs from the public key openssl derived"
  fi
  ar verify --attestation --public-key "$keys/pub.pem" --no-state
  expect_rc 0 "verify --attestation with the signing key"
  ar verify --attestation --public-key "$keys/other-pub.pem" --no-state
  if [ "$AR_RC" -eq 0 ]; then
    fail_note "verify --attestation accepted an unrelated key"
  fi
  # An SBOM signs and verifies the same way.
  ar sbom -o "$W/corpus-sbom.json"
  expect_rc 0 "sbom -o"
  ar sign --sbom "$W/corpus-sbom.json" --key "$keys/priv.pem"
  expect_rc 0 "sign --sbom"
  ar verify --sbom "$W/corpus-sbom.json" --public-key "$keys/pub.pem" --no-state
  expect_rc 0 "verify --sbom"
  printf ' ' >>"$W/corpus-sbom.json"
  ar verify --sbom "$W/corpus-sbom.json" --public-key "$keys/pub.pem" --no-state
  if [ "$AR_RC" -eq 0 ]; then
    fail_note "verify --sbom accepted a modified SBOM"
  fi
  # Changed content invalidates the signed lock.
  local file
  file="$(first_content_file)"
  if [ -n "$file" ]; then
    printf '\nCORPUS-TAMPER\n' >>"$file"
    ar lock --content-only
    ar verify --attestation --public-key "$keys/pub.pem" --no-state
    if [ "$AR_RC" -eq 0 ]; then
      fail_note "the signature still verified after the lock was re-pinned"
    fi
  fi
  finish "key-based sign, verify, approve round trip$([ "$approvals" = 0 ] && echo ' (no approvals)')"
}

# -------------------------------------------------------------------- injected

phase_injected() {
  fresh_copy
  cd "$W" || fail "no work dir"
  cfg_path >/dev/null || skip "no ai-rulez configuration"
  prep_hermetic || finish
  local perms=1
  if cfg_has '^\[permissions\]'; then
    perms=0
    ok_note "[permissions] already declared: not injected"
  else
    cfg_append '[permissions]
allow = ["Bash(git status)"]
deny = ["Read(./corpus-injected-deny.txt)"]'
  fi
  cfg_append '[[hooks]]
event = "PreToolUse"
matcher = "Bash"
[[hooks.hooks]]
command = "echo corpus-injected-hook"
timeout = 5

[[roles]]
name = "corpus-injected-role"
description = "Role injected by the corpus harness"

[[verifiers]]
name = "corpus-readme-or-config"
description = "The configuration file exists"
type = "file_exists"
path = ".ai-rulez/config.toml"

[[verifiers]]
name = "corpus-forbidden-marker"
type = "forbid"
glob = "corpus-verifier-target.txt"
pattern = "CORPUS-FORBIDDEN"
severity = "error"'
  printf 'harmless text\n' >corpus-verifier-target.txt
  ar validate
  expect_rc_in "validate with injected config" 0 2
  ar generate --offline
  expect_rc 0 "generate with injected config" || finish
  if [ -f .claude/settings.json ]; then
    expect_grep 'corpus-injected-hook' .claude/settings.json "hook rendered into .claude/settings.json"
    if [ "$perms" -eq 1 ]; then
      expect_grep 'corpus-injected-deny' .claude/settings.json "deny rule rendered into .claude/settings.json"
    fi
  elif cfg_has '^presets.*claude'; then
    fail_note "claude preset wrote no .claude/settings.json"
  fi
  ar generate --offline --check
  expect_rc 0 "generate --check with injected config"
  # Roles.
  ar_to "$PH_LOGS/roles.json" roles list --format json
  jq -e '.roles[]? | select(.name == "corpus-injected-role")' "$PH_LOGS/roles.json" >/dev/null 2>&1 ||
    fail_note "injected role not listed"
  ar generate --offline --role corpus-injected-role
  expect_rc 0 "generate --role corpus-injected-role"
  # Verifiers: one passes, one fails only once the forbidden text appears.
  ar verifiers run --name corpus-readme-or-config --name corpus-forbidden-marker
  expect_rc 0 "verifiers run (clean)"
  printf 'CORPUS-FORBIDDEN\n' >corpus-verifier-target.txt
  ar verifiers run --name corpus-forbidden-marker
  expect_rc 2 "verifiers run (violation)"
  ar_to "$PH_LOGS/verifiers.json" verifiers run --format json --name corpus-forbidden-marker
  jq -e . "$PH_LOGS/verifiers.json" >/dev/null 2>&1 || fail_note "verifiers run --format json is not JSON"
  printf 'harmless text\n' >corpus-verifier-target.txt
  # Machine-local overlay: appears in the local view, not in the shared one.
  ar local init
  expect_rc 0 "local init"
  ar local show
  expect_rc 0 "local show"
  mkdir -p "$(cfg_dir)/local/rules"
  printf '# Local only\n\nCORPUS-LOCAL-ONLY-MARKER\n' >"$(cfg_dir)/local/rules/corpus-local-only.md"
  ar generate --offline --allow-local-drift
  expect_rc 0 "generate with local content"
  ar generate --offline --no-local --allow-local-drift
  expect_rc_in "generate --no-local" 0 1
  if grep -rqF CORPUS-LOCAL-ONLY-MARKER --exclude-dir=.git --exclude-dir=local . 2>/dev/null &&
    ! sgit check-ignore -q --no-index CLAUDE.md; then
    ok_note "local content present in the local view"
  fi
  finish "hooks, permissions, role, verifiers and local overlay injected"
}
