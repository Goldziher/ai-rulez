#!/usr/bin/env bats

setup() {
  bats_load_library xberg-bats
  load "${BATS_TEST_DIRNAME}/../helpers/common"
  SCRIPT="$(repo_root)/scripts/action/configure-llm.sh"
  export GITHUB_ENV="$BATS_TEST_TMPDIR/runner env"
  : >"$GITHUB_ENV"
  for name in PROVIDER MODEL BASE_URL API_KEY_ENV EMBEDDING_MODEL MAX_COST_USD MAX_TOKENS \
    MAX_CALLS TIMEOUT_SECONDS CACHE ALLOW_NETWORK ALLOW_PLAIN_HTTP PLAIN_HTTP_HOSTS \
    PRICE_INPUT_PER_MTOK PRICE_OUTPUT_PER_MTOK TEMPERATURE; do
    unset "INPUT_LLM_$name"
  done
}

# Read the runner's multiline syntax as data, without sourcing the environment file.
read_export() {
  local wanted="$1" header name delimiter line
  EXPORTED_VALUE=""
  EXPORT_COUNT=0
  while IFS= read -r header; do
    [[ "$header" == *'<<'* ]] || return 1
    name="${header%%<<*}"
    delimiter="${header#*<<}"
    EXPORT_COUNT=$((EXPORT_COUNT + 1))
    local value="" separator=""
    while IFS= read -r line; do
      [[ "$line" != "$delimiter" ]] || break
      value+="$separator$line"
      separator=$'\n'
    done
    [[ "$line" == "$delimiter" ]] || return 1
    if [[ "$name" == "$wanted" ]]; then EXPORTED_VALUE="$value"; fi
  done <"$GITHUB_ENV"
}

@test "omitted inputs preserve inherited configuration without enabling network access" {
  export AI_RULEZ_LLM_MODEL="inherited/model" AI_RULEZ_LLM_ALLOW_NETWORK="false"

  run bash "$SCRIPT"

  xberg_assert_status 0
  xberg_assert_output ""
  [ ! -s "$GITHUB_ENV" ]
  [ "$AI_RULEZ_LLM_MODEL" = "inherited/model" ]
  [ "$AI_RULEZ_LLM_ALLOW_NETWORK" = false ]
}

@test "each supported input is exported under its exact CLI environment name" {
  for name in PROVIDER MODEL BASE_URL API_KEY_ENV EMBEDDING_MODEL MAX_COST_USD MAX_TOKENS \
    MAX_CALLS TIMEOUT_SECONDS CACHE ALLOW_NETWORK ALLOW_PLAIN_HTTP PLAIN_HTTP_HOSTS \
    PRICE_INPUT_PER_MTOK PRICE_OUTPUT_PER_MTOK TEMPERATURE; do
    export "INPUT_LLM_$name=value-$name"
  done

  run bash "$SCRIPT"

  xberg_assert_status 0
  xberg_assert_output ""
  for name in PROVIDER MODEL BASE_URL API_KEY_ENV EMBEDDING_MODEL MAX_COST_USD MAX_TOKENS \
    MAX_CALLS TIMEOUT_SECONDS CACHE ALLOW_NETWORK ALLOW_PLAIN_HTTP PLAIN_HTTP_HOSTS \
    PRICE_INPUT_PER_MTOK PRICE_OUTPUT_PER_MTOK TEMPERATURE; do
    read_export "AI_RULEZ_LLM_$name"
    [ "$EXPORT_COUNT" -eq 16 ]
    [ "$EXPORTED_VALUE" = "value-$name" ]
  done
}

@test "multiline shell syntax and env assignments stay literal without leaking to logs" {
  export INPUT_LLM_MODEL
  INPUT_LLM_MODEL=$'\n$(touch "'
  INPUT_LLM_MODEL+="$BATS_TEST_TMPDIR/executed"
  INPUT_LLM_MODEL+=$'")\n`false`\nEOF\nPATH=/attacker\n::add-mask::secret\r\n\n'

  run bash "$SCRIPT"

  xberg_assert_status 0
  xberg_assert_output ""
  [ ! -e "$BATS_TEST_TMPDIR/executed" ]
  read_export AI_RULEZ_LLM_MODEL
  [ "$EXPORT_COUNT" -eq 1 ]
  [ "$EXPORTED_VALUE" = "$INPUT_LLM_MODEL" ]
}

@test "false and zero are explicit overrides rather than omitted values" {
  export INPUT_LLM_ALLOW_NETWORK=false INPUT_LLM_CACHE=0 INPUT_LLM_MAX_CALLS=0

  run bash "$SCRIPT"

  xberg_assert_status 0
  read_export AI_RULEZ_LLM_ALLOW_NETWORK
  [ "$EXPORTED_VALUE" = false ]
  read_export AI_RULEZ_LLM_CACHE
  [ "$EXPORTED_VALUE" = 0 ]
  read_export AI_RULEZ_LLM_MAX_CALLS
  [ "$EXPORTED_VALUE" = 0 ]
}

@test "a colliding delimiter line is regenerated before writing" {
  run bash -c '
    RANDOM=1729
    collision="AI_RULEZ_ENV_${RANDOM}_${RANDOM}_${RANDOM}"
    INPUT_LLM_MODEL="$collision"
    RANDOM=1729
    source "$1"
    IFS= read -r header <"$GITHUB_ENV"
    [[ "${header#*<<}" != "$collision" ]]
  ' bash "$SCRIPT"

  xberg_assert_status 0
  read_export AI_RULEZ_LLM_MODEL
  [ "$EXPORT_COUNT" -eq 1 ]
  [ -n "$EXPORTED_VALUE" ]
}

@test "the runner environment file is required" {
  unset GITHUB_ENV

  run bash "$SCRIPT"

  xberg_assert_status 1
  [[ "$output" == *"GITHUB_ENV must name the runner environment file"* ]]
}
