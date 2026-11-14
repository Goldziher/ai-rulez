#!/usr/bin/env bash
# Install ai-rulez while the network is open, so an agent phase with restricted egress never has to
# download anything. Use it as the setup script of a sandboxed agent environment (for example a Codex
# cloud environment) or run it by hand before going offline.
#
# Usage: scripts/codex-setup.sh [version]    (default: the latest release, resolved now and then fixed)
# Run `ai-rulez ...` directly afterwards; `npx ai-rulez@latest` would resolve a newer version later.
set -euo pipefail

version="${1:-${AI_RULEZ_VERSION:-latest}}"
version="${version#v}"

if ! command -v npm >/dev/null 2>&1; then
  echo "npm is required (Node.js 22 or newer)" >&2
  exit 1
fi

npm install --global "ai-rulez@${version}"

# Releases before the registry-delivered binaries download the binary on first run; running it
# here pulls it in while the network is still available.
installed="$(ai-rulez --version)"
echo "installed: ${installed}"
