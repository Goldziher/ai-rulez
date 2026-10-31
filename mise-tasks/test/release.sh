#!/usr/bin/env bash
#MISE description="Run the model-layer tests against the static liter-llm library, the way CI does"
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../.."
if [ -z "${CGO_LDFLAGS:-}" ]; then
  eval "$(bash scripts/setup-liter-llm.sh)"
fi
go test ./internal/llm/... ./internal/pricing/... ./internal/tokens/... ./cmd/...
