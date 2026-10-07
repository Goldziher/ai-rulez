#!/usr/bin/env bash

# run.sh runs the Bats suites under tests/bats with the vendored xberg-bats helpers, the same way
# CI does. Arguments go to bats; with none it runs every suite.
#
#   scripts/bats/run.sh
#   scripts/bats/run.sh tests/bats/scripts/run-ai-rulez.bats --filter checksum
#
# It builds ai-rulez from this checkout once and exports AI_RULEZ_BATS_BIN for the suites that
# need the binary; set AI_RULEZ_BATS_BIN yourself to reuse a build.

set -euo pipefail

root="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"

command -v bats >/dev/null 2>&1 || {
  echo "run.sh: bats is not on PATH (brew install bats-core, or apt-get install bats)" >&2
  exit 1
}

# bats_load_library searches BATS_LIB_PATH and nothing else.
export BATS_LIB_PATH="${root}/tests/lib${BATS_LIB_PATH:+:${BATS_LIB_PATH}}"

build_dir=""
cleanup() {
  [[ -z "$build_dir" ]] || rm -rf "$build_dir"
}
trap cleanup EXIT

if [[ -z "${AI_RULEZ_BATS_BIN:-}" ]]; then
  command -v go >/dev/null 2>&1 || {
    echo "run.sh: go is required to build ai-rulez (or set AI_RULEZ_BATS_BIN)" >&2
    exit 1
  }
  build_dir="$(mktemp -d)"
  (cd "$root" && go build -o "${build_dir}/ai-rulez" ./cmd/ai-rulez)
  export AI_RULEZ_BATS_BIN="${build_dir}/ai-rulez"
fi

if [[ $# -eq 0 ]]; then
  set -- "${root}/tests/bats"
fi

bats --recursive --print-output-on-failure "$@"
