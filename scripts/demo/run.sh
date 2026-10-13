#!/usr/bin/env bash
# Records or checks the README terminal demos. Run from the repository root.
#   run.sh record [name...]   re-record the named tapes (default: all) with the ai-rulez on PATH
#   run.sh check              parse every tape and fail when a committed GIF is missing or over budget
set -euo pipefail

max_bytes=$((1024 * 1024))
tapes_dir=docs/assets/tapes

names() {
	if [ "$#" -gt 0 ]; then
		printf '%s\n' "$@"
		return
	fi
	for tape in "$tapes_dir"/*.tape; do
		name="$(basename "$tape" .tape)"
		[ "$name" = settings ] || printf '%s\n' "$name"
	done
}

cmd="${1:-}"
shift || true

case "$cmd" in
record)
	command -v vhs >/dev/null || { echo "vhs is required: brew install vhs" >&2; exit 1; }
	command -v ai-rulez >/dev/null || { echo "ai-rulez must be on PATH (task build, then put bin/ on PATH)" >&2; exit 1; }
	for name in $(names "$@"); do
		vhs "$tapes_dir/$name.tape"
		size="$(wc -c <"docs/assets/$name.gif")"
		[ "$size" -le "$max_bytes" ] || { echo "docs/assets/$name.gif is $size bytes (budget $max_bytes)" >&2; exit 1; }
	done
	;;
check)
	command -v vhs >/dev/null || { echo "vhs is required: brew install vhs" >&2; exit 1; }
	status=0
	for name in $(names); do
		vhs validate "$tapes_dir/$name.tape" >/dev/null || status=1
		gif="docs/assets/$name.gif"
		if [ ! -f "$gif" ]; then
			echo "missing $gif" >&2
			status=1
		elif [ "$(wc -c <"$gif")" -gt "$max_bytes" ]; then
			echo "$gif is over the $max_bytes byte budget" >&2
			status=1
		fi
	done
	exit "$status"
	;;
*)
	echo "usage: run.sh record [name...] | check" >&2
	exit 2
	;;
esac
