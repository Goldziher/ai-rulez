#!/usr/bin/env bash
# Builds the project the validate tape records in. Usage: validate.fixture.sh <dir>
# A testing rule is scoped to src/**, but the tests live in tests/, so strict validation reports AR101.
# Outputs are generated from the corrected rule first, so the tape's later edit is the only drift.
set -euo pipefail

dir="${1:?usage: validate.fixture.sh <dir>}"

rm -rf "$dir"
mkdir -p "$dir/tests"
cd "$dir"
git init -q
git config user.email demo@example.com
git config user.name demo

ai-rulez init acme --yes >/dev/null 2>&1
printf "test('lists devices', () => {})\n" >tests/api.test.ts
cat >.ai-rulez/rules/testing.md <<'MD'
---
type: Decision
title: Testing
x-ai-rulez:
  kind: rule
  id: testing
  metadata:
    priority: high
paths:
  - "tests/**/*.test.ts"
activation: glob
---

# Testing Standards

- Every bug fix ships with a regression test
- Keep tests fast and deterministic
MD
printf '* [Testing](testing.md)\n' >>.ai-rulez/rules/index.md
git add tests
ai-rulez generate >/dev/null 2>&1
sed -i.bak 's#tests/\*\*#src/**#' .ai-rulez/rules/testing.md
rm .ai-rulez/rules/testing.md.bak
