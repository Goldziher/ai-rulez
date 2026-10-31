#!/usr/bin/env bash
#MISE description="Set version across all packages (usage: mise run set-version 1.2.3)"
#USAGE arg "<version>" help="The release version, e.g. 5.9.9"
set -euo pipefail

version="${usage_version:?usage: mise run set-version <version>}"

echo "Setting version to ${version} across all packages..."

# Go CLI version
sed -i '' 's/Version  = ".*"/Version  = "'"$version"'"/' cmd/commands/root.go
sed -i '' 's/var version = ".*"/var version = "'"$version"'"/' cmd/ai-rulez/main.go

# npm release wrapper
sed -i '' 's/"version": ".*"/"version": "'"$version"'"/' release/npm/package.json

# pyproject.toml
sed -i '' 's/^version = ".*"/version = "'"$version"'"/' pyproject.toml

# PyPI wrapper __version__
sed -i '' 's/__version__ = ".*"/__version__ = "'"$version"'"/' release/pypi/__init__.py
sed -i '' 's/__version__ = ".*"/__version__ = "'"$version"'"/' release/pypi/ai_rulez/__init__.py

# Pre-commit hook rev (internal/hooks/setup.go)
sed -i '' 's/officialPreCommitRev  = "v[^"]*"/officialPreCommitRev  = "v'"$version"'"/' internal/hooks/setup.go

# Pre-commit bootstrap script default version (scripts/pre-commit/run-ai-rulez.sh); the pattern matches a
# literal ${...}. ~keep
# shellcheck disable=SC2016
sed -i '' 's/AI_RULEZ_VERSION="\${AI_RULEZ_VERSION:-v[^"]*}"/AI_RULEZ_VERSION="\${AI_RULEZ_VERSION:-v'"$version"'}"/' \
  scripts/pre-commit/run-ai-rulez.sh

# README pre-commit example + poly hook source revision
sed -i '' 's/rev: v[0-9][0-9.]*/rev: v'"$version"'/' README.md
sed -i '' 's/revision = "v[0-9][0-9.]*"/revision = "v'"$version"'"/g' README.md

# Poly hook catalog + docs: pinned ai-rulez versions (npx/uvx) and revisions
for f in poly-hooks.toml docs/poly-hooks.md; do
  sed -i '' 's/ai-rulez@[0-9][0-9.]*/ai-rulez@'"$version"'/g' "$f"
  sed -i '' 's/ai-rulez==[0-9][0-9.]*/ai-rulez=='"$version"'/g' "$f"
done
sed -i '' 's/revision = "v[0-9][0-9.]*"/revision = "v'"$version"'"/g' docs/poly-hooks.md

echo "Done. Updated version to ${version} in:"
echo "  - cmd/commands/root.go"
echo "  - cmd/ai-rulez/main.go"
echo "  - release/npm/package.json"
echo "  - pyproject.toml"
echo "  - release/pypi/__init__.py"
echo "  - release/pypi/ai_rulez/__init__.py"
echo "  - internal/hooks/setup.go"
echo "  - scripts/pre-commit/run-ai-rulez.sh"
echo "  - README.md"
echo "  - poly-hooks.toml"
echo "  - docs/poly-hooks.md"
