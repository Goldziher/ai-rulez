#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${CGO_LDFLAGS:-}" ]]; then
  exports="$(bash scripts/setup-liter-llm.sh)"
  eval "$exports"
fi
exec "$@"
