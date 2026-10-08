#!/usr/bin/env bash
# Repository handling: read-only snapshots, config probes and the "prep"
# helpers that bring a throwaway copy into the shape a phase needs. Sourced by
# run.sh.

# ------------------------------------------------------------ source guard

# source_fingerprint prints HEAD plus a digest of the working-tree status of a
# source repository. It only reads: GIT_OPTIONAL_LOCKS=0 keeps git from
# refreshing the index.
source_fingerprint() {
	local repo="$1" head status
	head="$(GIT_OPTIONAL_LOCKS=0 git -C "$repo" rev-parse HEAD 2>/dev/null)"
	status="$(GIT_OPTIONAL_LOCKS=0 git -C "$repo" status --porcelain=v1 --untracked-files=no 2>/dev/null | shasum -a 256 | cut -d' ' -f1)"
	printf '%s %s' "$head" "$status"
}

# ----------------------------------------------------------------- snapshot

# snapshot_repo copies the committed tree of a source repository into
# $SNAP/<id> with git archive and makes it a one-commit git repository of its
# own, so phases that need history (publish, worktrees, --since) have it.
# CORPUS_ARCHIVE_EXCLUDE may list pathspecs (space separated) to leave out.
snapshot_repo() {
	local src="$1" dst="$SNAP/$2"
	local -a spec=(.)
	local ex
	for ex in ${CORPUS_ARCHIVE_EXCLUDE:-}; do
		spec+=(":(exclude)$ex")
	done
	mkdir -p "$dst"
	GIT_OPTIONAL_LOCKS=0 GIT_INDEX_FILE="$WORK/archive.index" \
		git -C "$src" archive --format=tar HEAD -- "${spec[@]}" | tar -x -C "$dst" || return 1
	rm -f "$WORK/archive.index"
	(
		cd "$dst" || exit 1
		sgit init -q . && sgit add -A && sgit commit -q -m "corpus snapshot" --no-gpg-sign
	)
}

# ------------------------------------------------------------ config probes

# cfg_path prints the configuration file of the current directory, if any.
cfg_path() {
	local f
	for f in .ai-rulez/config.toml .config/ai-rulez/config.toml; do
		if [ -f "$f" ]; then
			echo "$f"
			return 0
		fi
	done
	return 1
}

# cfg_version prints the major version declared by the configuration.
cfg_version() {
	local f
	f="$(cfg_path)" || return 1
	sed -n -E "s/^version[[:space:]]*=[[:space:]]*[\"']?([0-9]+)(\.[0-9]+)?[\"']?.*/\\1/p" "$f" | head -n 1
}

# cfg_has reports whether the config has a line matching an extended regex.
cfg_has() {
	local f
	f="$(cfg_path)" || return 1
	grep -qE -- "$1" "$f"
}

# cfg_dir prints the configuration directory ('.ai-rulez' by default).
cfg_dir() {
	local f
	f="$(cfg_path)" || return 1
	dirname "$f"
}

# cfg_table_keys prints the keys of a top-level TOML table, one per line.
# Usage: cfg_table_keys profiles
cfg_table_keys() {
	local f
	f="$(cfg_path)" || return 1
	awk -v tbl="$1" '
		/^\[/ { inside = ($0 == "[" tbl "]"); next }
		inside && match($0, /^[A-Za-z0-9_"'"'"'.-]+[[:space:]]*=/) {
			k = substr($0, 1, RLENGTH - 1)
			gsub(/[[:space:]"'"'"']/, "", k)
			print k
		}' "$f"
}

# cfg_append adds text to the end of the config file.
cfg_append() {
	local f
	f="$(cfg_path)" || return 1
	printf '\n%s\n' "$1" >>"$f"
}

# ------------------------------------------------------------- prep helpers

# git_commit_all commits the working tree of the current directory, tagging it
# when a tag is given. Throwaway copies only.
git_commit_all() {
	[ -d .git ] || sgit init -q .
	sgit add -A
	sgit commit -q -m "corpus" --allow-empty --no-gpg-sign
	if [ -n "${1:-}" ]; then
		sgit tag -f "$1"
	fi
}

# prep_v5 brings the config to version 5 with the migrator. A config that is
# already v5, or absent, is left alone. Returns 1 when the migration failed.
prep_v5() {
	cfg_path >/dev/null || return 0
	local v
	v="$(cfg_version)"
	if [ -n "$v" ] && [ "$v" -lt 5 ]; then
		ar migrate v5
		if [ "$AR_RC" -ne 0 ]; then
			fail_note "prep: migrate v5 exit $AR_RC ($(last_msg))"
			return 1
		fi
	fi
	return 0
}

# strip_remote_sources removes the config tables that fetch from the network
# (git or https includes, installed skills, skill sources). Local path sources
# stay. Prints the number of tables removed.
strip_remote_sources() {
	local f tmp removed
	f="$(cfg_path)" || {
		echo 0
		return 0
	}
	tmp="$f.corpus-tmp"
	removed="$(
		awk -v out="$tmp" '
			function flush(   i) {
				if (n > 0) {
					if (remote) { dropped++ } else { for (i = 1; i <= n; i++) print buf[i] > out }
				}
				n = 0; remote = 0; held = 0
			}
			/^\[/ {
				flush()
				if ($0 ~ /^\[\[(includes|installed_skills|skill_sources)\]\]/) { held = 1 }
			}
			{
				if (held) {
					buf[++n] = $0
					if ($0 ~ /^[[:space:]]*(source|url|repo)[[:space:]]*=[[:space:]]*["'"'"'](https?:|git@|ssh:|git\+|git:)/) { remote = 1 }
				} else {
					print $0 > out
				}
			}
			END { flush(); print dropped + 0 }
		' "$f"
	)"
	if [ -f "$tmp" ]; then
		mv "$tmp" "$f"
	else
		: >"$f"
	fi
	echo "$removed"
}

# prep_hermetic is prep_v5 plus the removal of every network source, so the
# phases that must run without the network (lock, frozen, signing) can.
prep_hermetic() {
	prep_v5 || return 1
	# shellcheck disable=SC2034 # read by the phases that report it
	HERMETIC_STRIPPED="$(strip_remote_sources)"
	return 0
}

# first_item sets FIRST_ITEM to "kind:name" of the first item that
# 'approve --list' offers (of the given kind, when one is passed), else "".
# Usage: first_item [kind]
first_item() {
	ar approve --list
	# shellcheck disable=SC2034 # read by the signing phase
	FIRST_ITEM="$(awk -v kind="${1:-}" 'NR > 1 && (kind == "" || $1 == kind) { print $1 ":" $2; exit }' "$AR_LOG")"
}

# mk_subproject builds a small standalone project in <dir> from the repository's
# own top-level content (rules, context, skills, agents, commands), under a
# fresh v5 config with the given extra TOML. The repository's config is not
# reused, so every feature phase gets a project that is valid on its own while
# still being fed real-world content.
# Usage: mk_subproject <dir> <name> <extra-toml> [presets-toml-array]
mk_subproject() {
	local dir="$1" name="$2" extra="$3" presets="${4:-[\"claude\"]}" kind src
	local cdir
	cdir="$(cfg_dir 2>/dev/null || echo .ai-rulez)"
	mkdir -p "$dir/.ai-rulez"
	for kind in rules context skills agents commands; do
		src="$cdir/$kind"
		if [ -d "$src" ]; then
			cp -R "$src" "$dir/.ai-rulez/$kind"
		fi
	done
	# Never bring along a machine-local or generated tree.
	rm -rf "$dir/.ai-rulez/local"
	if [ -z "$(find "$dir/.ai-rulez/skills" -name SKILL.md 2>/dev/null | head -n 1)" ]; then
		mkdir -p "$dir/.ai-rulez/skills/corpus-sample"
		cat >"$dir/.ai-rulez/skills/corpus-sample/SKILL.md" <<'EOF'
---
name: corpus-sample
description: Sample skill added by the corpus harness when a repository has none.
---

# Corpus sample

Say hello.
EOF
	fi
	cat >"$dir/.ai-rulez/config.toml" <<EOF
version = "5.0"
name = "$name"
description = "Corpus sub-project built from repository content."
presets = $presets

$extra
EOF
}
