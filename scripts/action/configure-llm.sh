#!/usr/bin/env bash

# Export only explicit action inputs. Caller env and configuration defaults remain intact.
# Values are data, never shell code; the runner reads this file after the step exits.
set -euo pipefail

: "${GITHUB_ENV:?GITHUB_ENV must name the runner environment file}"

export_input() {
  local name="$1" input value delimiter normalized
  input="INPUT_LLM_$name"
  value="${!input:-}"
  [[ -n "$value" ]] || return 0

  # GitHub accepts multiline env values only when the delimiter is absent on every line.
  # Ignore CR while checking too, because the runner recognizes CRLF line endings.
  normalized="${value//$'\r'/}"
  while :; do
    delimiter="AI_RULEZ_ENV_${RANDOM}_${RANDOM}_${RANDOM}"
    [[ $'\n'"$normalized"$'\n' != *$'\n'"$delimiter"$'\n'* ]] && break
  done
  printf 'AI_RULEZ_LLM_%s<<%s\n%s\n%s\n' "$name" "$delimiter" "$value" "$delimiter" >>"$GITHUB_ENV"
}

for name in \
  PROVIDER MODEL BASE_URL API_KEY_ENV EMBEDDING_MODEL \
  MAX_COST_USD MAX_TOKENS MAX_CALLS TIMEOUT_SECONDS CACHE ALLOW_NETWORK \
  ALLOW_PLAIN_HTTP PLAIN_HTTP_HOSTS PRICE_INPUT_PER_MTOK PRICE_OUTPUT_PER_MTOK TEMPERATURE; do
  export_input "$name"
done
