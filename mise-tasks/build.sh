#!/usr/bin/env bash
#MISE description="Build the CLI binary as a release does: liter-llm linked statically (plain `go build` links the shared library the module ships, which only runs on this machine)"
# Runs without mise as well (`bash mise-tasks/build.sh`): xberg-io/actions/setup-ai-rulez builds a pinned
# source revision this way. ~keep
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
if [ -z "${CGO_LDFLAGS:-}" ]; then
  eval "$(bash scripts/setup-liter-llm.sh)"
fi
go build -ldflags="-s -w" -o bin/ai-rulez ./cmd/ai-rulez
