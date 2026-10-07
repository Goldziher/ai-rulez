#!/usr/bin/env bash

# lint.sh format-checks and lints the Bats suites, which poly does not see (it has no engine for
# .bats). The helpers under tests/bats and tests/lib are .bash files that poly already lints.
#
# Four shellcheck codes are disabled, all structural to Bats, as in xberg-io/actions: a stub body
# is single-quoted on purpose (SC2016, SC2183) and `run` forks, so every export in setup() reads
# as subshell-local (SC2030, SC2031).

set -euo pipefail

root="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
cd "$root"

for tool in shfmt shellcheck; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "lint.sh: ${tool} is not on PATH" >&2
    exit 1
  }
done

suites=()
while IFS= read -r -d '' suite; do
  suites+=("$suite")
done < <(git ls-files -z -- '*.bats')

if [[ ${#suites[@]} -eq 0 ]]; then
  echo "lint.sh: no .bats suites are tracked" >&2
  exit 1
fi

# Two-space indent, as poly's shfmt formats the repository's .sh files.
shfmt -ln bats -i 2 -d "${suites[@]}"
shellcheck -s bash -e SC2016,SC2183,SC2030,SC2031 "${suites[@]}"
echo "linted ${#suites[@]} Bats suites"
