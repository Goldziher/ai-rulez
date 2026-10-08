#!/usr/bin/env bash
# Phases about configuration handling: upgrade, repair, content that carries no
# config, shared conventions, sparse checkouts, local includes, profiles,
# roles and recursion. Sourced by run.sh.

# stamp_tree drops a marker file, and changed_since lists the files created or
# modified after it (the .git directory excluded) plus the net file count.
stamp_tree() {
	TREE_STAMP="$PH_LOGS/stamp.$1"
	: >"$TREE_STAMP"
	TREE_COUNT_BEFORE="$(count_files "${2:-.}")"
	sleep 1
}
count_files() { (cd "$1" && find . -path ./.git -prune -o -type f -print | wc -l | tr -d ' '); }
changed_since() {
	(cd "$1" && find . -path ./.git -prune -o -type f -newer "$TREE_STAMP" -print | head -n 5 | tr '\n' ' ')
}

# manifest_outputs prints the paths a generated-file manifest lists (the v4
# array form and the v5 object form), for the manifests that exist.
manifest_outputs() {
	local m
	for m in .ai-rulez/.generated-manifest.json .ai-rulez-generated.json; do
		[ -f "$m" ] || continue
		jq -r '
			(.files // .outputs // []) as $f
			| if ($f | type) == "array" then $f[] | (if type == "string" then . else (.path // empty) end)
			  elif ($f | type) == "object" then $f | keys[]
			  else empty end' "$m" 2>/dev/null
	done | sort -u
}

# --------------------------------------------------------------------- upgrade

phase_upgrade() {
	fresh_copy
	cd "$W" || fail "no work dir"
	cfg_path >/dev/null || skip "no ai-rulez configuration"
	local v legacy_n stale=0 path
	v="$(cfg_version)"
	manifest_outputs >"$PH_LOGS/legacy-outputs.txt"
	legacy_n="$(wc -l <"$PH_LOGS/legacy-outputs.txt" | tr -d ' ')"
	if [ "${v:-0}" -lt 5 ]; then
		ar migrate v5 --dry-run
		expect_rc 0 "migrate v5 --dry-run"
		ar migrate v5
		expect_rc 0 "migrate v5"
		ar migrate v5
		expect_rc 0 "migrate v5 (second run)"
		expect_grep 'unchanged|migrated 0|already' "$AR_LOG" "second migrate reports nothing to do"
		[ "$(cfg_version)" = "5" ] || fail_note "config version is $(cfg_version) after migrate"
	fi
	ar generate --offline
	expect_rc 0 "generate after upgrade"
	ar generate --offline
	expect_rc 0 "second generate"
	ar generate --offline --check
	expect_rc 0 "generate --check converges"
	# Legacy outputs: every path the old manifest listed is either regenerated,
	# removed, or reported by name as left alone; none may silently vanish
	# while the new manifest does not know it.
	while IFS= read -r path; do
		[ -n "$path" ] || continue
		[ -e "$path" ] || continue
		manifest_outputs | grep -qxF "$path" || stale=$((stale + 1))
	done <"$PH_LOGS/legacy-outputs.txt"
	finish "config v${v:-?}, $legacy_n legacy outputs ($stale left on disk, not regenerated)"
}

# -------------------------------------------------------------------- repaired

phase_repaired() {
	fresh_copy
	cd "$W" || fail "no work dir"
	cfg_path >/dev/null || skip "no ai-rulez configuration"
	ar validate
	if [ "$AR_RC" -eq 0 ]; then
		skip "configuration loads as it is, nothing to repair"
	fi
	local broken_msg
	broken_msg="$(last_msg)"
	ar migrate v5
	expect_rc 0 "repair with migrate v5" || finish
	ar validate --config-only
	expect_rc 0 "validate --config-only after repair"
	ar validate --offline 2>/dev/null
	ar validate
	expect_rc_in "validate after repair" 0 2
	ar generate --offline
	expect_rc 0 "generate after repair"
	ar doctor
	expect_rc_in "doctor after repair" 0 1 2
	ar generate --offline --check
	expect_rc 0 "repaired outputs are in sync"
	finish "repaired (was: ${broken_msg:0:80})"
}

# ------------------------------------------------------------ skipped_content

# Content that carries no configuration must be skipped cleanly: every command
# either works on what it finds or says how to start, and none writes a file.
phase_skipped_content() {
	fresh_copy
	cd "$W" || fail "no work dir"
	local had=no
	cfg_path >/dev/null && had=yes
	rm -rf .ai-rulez .config/ai-rulez ai-rulez.yaml ai-rulez.yml ai-rulez.json .ai-rulez.toml
	stamp_tree skipped
	local cmd
	for cmd in "validate" "generate --offline" "generate --check" "lock --check" "doctor" "list" \
		"verify" "sbom" "catalog --format json" "clean --dry-run" "roles list" "verifiers run" "tokens"; do
		# shellcheck disable=SC2086
		ar $cmd
		if [ "$AR_RC" -ne 0 ] && [ ! -s "$AR_LOG" ]; then
			fail_note "$cmd: exit $AR_RC with no message"
		fi
	done
	local changed
	changed="$(changed_since .)"
	[ -z "$changed" ] || fail_note "wrote files without a config: $changed"
	[ "$(count_files .)" = "$TREE_COUNT_BEFORE" ] || fail_note "file count changed without a config"
	finish "config-less copy handled (original had config: $had)"
}

# ----------------------------------------------------------- agent_conventions

# Shared "conventions" repositories arrive as remote includes. Two things are
# checked without the network: a repository whose includes are unreachable
# degrades with named warnings instead of hanging or failing, and a git include
# served from a local repository is pinned by the lock and honoured by
# --frozen (the pin wins over a newer commit upstream).
phase_agent_conventions() {
	fresh_copy
	cd "$W" || fail "no work dir"
	cfg_path >/dev/null || skip "no ai-rulez configuration"
	prep_v5 || finish
	local remote_n part1=0
	remote_n="$(grep -cE '^source[[:space:]]*=[[:space:]]*["'"'"'](https?:|git@|ssh:|git\+)' "$(cfg_path)")"
	if [ "$remote_n" -gt 0 ]; then
		part1=1
		ar generate --offline
		expect_rc 0 "generate --offline with $remote_n unreachable remote sources"
		expect_grep 'offline|cache' "$AR_LOG" "unreachable remotes are named in the output"
		ar generate --frozen
		if [ "$AR_RC" -eq 0 ]; then
			fail_note "generate --frozen succeeded without a lock covering $remote_n remote sources"
		fi
	fi
	# Local git repository acting as the conventions remote.
	# The physical path: a file:// include is compared with the project's
	# resolved directory (see repros/symlinked-project-file-url.sh).
	local conv head1 head2
	conv="$(pwd -P)/corpus-conv"
	mkdir -p "$conv/modules/core/rules"
	printf '# Conv rule\n\nCORPUS-CONV-MARKER-ONE\n' >"$conv/modules/core/rules/corpus-conv-rule.md"
	(cd "$conv" && sgit init -q . && sgit add -A && sgit commit -q -m one --no-gpg-sign)
	head1="$(cd "$conv" && env -i "${SB_ENV[@]}" git rev-parse HEAD)"
	strip_remote_sources >/dev/null
	cfg_append "[[includes]]
name = \"corpus-conv\"
source = \"file://$conv\"
path = \"modules/core\"
merge_strategy = \"local-override\""
	ar generate
	expect_rc 0 "generate with a local git include" || finish
	grep -rqF CORPUS-CONV-MARKER-ONE --include='*.md' --exclude-dir=.git --exclude-dir=corpus-conv . ||
		fail_note "included rule missing from the generated outputs"
	ar lock
	expect_rc 0 "lock with a local git include"
	grep -qF "$head1" "$(cd "$(cfg_dir)" && pwd)/ai-rulez.lock" 2>/dev/null ||
		fail_note "lock does not record the include commit"
	printf '# Conv rule\n\nCORPUS-CONV-MARKER-TWO\n' >"$conv/modules/core/rules/corpus-conv-rule.md"
	(cd "$conv" && sgit add -A && sgit commit -q -m two --no-gpg-sign)
	head2="$(cd "$conv" && env -i "${SB_ENV[@]}" git rev-parse HEAD)"
	[ "$head1" != "$head2" ] || fail_note "harness: second upstream commit has the same id"
	ar generate --frozen
	expect_rc 0 "generate --frozen with the include pinned"
	if grep -rqF CORPUS-CONV-MARKER-TWO --include='*.md' --exclude-dir=.git --exclude-dir=corpus-conv .; then
		fail_note "--frozen followed the new upstream commit instead of the pin"
	fi
	ar lock
	expect_rc 0 "re-lock after upstream moved"
	ar generate
	expect_rc 0 "generate after re-lock"
	grep -rqF CORPUS-CONV-MARKER-TWO --include='*.md' --exclude-dir=.git --exclude-dir=corpus-conv . ||
		fail_note "re-locked include does not carry the new commit"
	finish "local git include pinned and frozen$([ "$part1" = 1 ] && echo "; $remote_n unreachable remotes degrade")"
}

# ---------------------------------------------------------------------- sparse

phase_sparse() {
	fresh_copy
	cd "$W" || fail "no work dir"
	cfg_path >/dev/null || skip "no ai-rulez configuration"
	prep_v5 || finish
	local d
	# A: a real git sparse checkout without the content directories.
	sgit sparse-checkout init --no-cone
	sgit sparse-checkout set '/*' '!/.ai-rulez/rules/' '!/.ai-rulez/context/' '!/.ai-rulez/skills/' \
		'!/.ai-rulez/agents/' '!/.ai-rulez/commands/' '!/.ai-rulez/domains/'
	local missing=0
	for d in rules context skills agents commands domains; do
		[ -e ".ai-rulez/$d" ] || missing=$((missing + 1))
	done
	local cmd
	for cmd in "validate" "generate --offline" "doctor" "list" "lock --check" "sbom"; do
		# shellcheck disable=SC2086
		ar $cmd
		expect_rc_in "sparse (content absent): $cmd" 0 1 2
	done
	# B: the configuration file absent, content present.
	fresh_copy "$W.b"
	cd "$W.b" || fail "no work dir"
	rm -f "$(cfg_path)"
	stamp_tree sparse-b
	ar validate
	if [ "$AR_RC" -eq 0 ]; then
		fail_note "validate succeeded with the config file absent"
	fi
	ar generate --offline
	[ -z "$(changed_since .)" ] || fail_note "generate wrote files without a config file: $(changed_since .)"
	cd "$W" || fail "no work dir"
	rm -rf "$W.b"
	finish "sparse checkout ($missing content dirs absent) and missing config handled"
}

# -------------------------------------------------------------- local_includes

phase_local_includes() {
	fresh_copy
	cd "$W" || fail "no work dir"
	cfg_path >/dev/null || skip "no ai-rulez configuration"
	prep_hermetic || finish
	mkdir -p corpus-shared/rules
	printf '# Shared\n\nCORPUS-LOCAL-INCLUDE-ONE\n' >corpus-shared/rules/corpus-shared-rule.md
	cfg_append '[[includes]]
name = "corpus-local"
source = "./corpus-shared"
merge_strategy = "local-override"'
	ar generate --offline
	expect_rc 0 "generate with a local include" || finish
	grep -rqF CORPUS-LOCAL-INCLUDE-ONE --include='*.md' --include='*.mdc' --exclude-dir=.git --exclude-dir=corpus-shared . ||
		fail_note "local include content missing from the outputs"
	ar include list
	expect_rc 0 "include list"
	expect_grep 'corpus-local' "$AR_LOG" "include list names the include"
	printf '# Shared\n\nCORPUS-LOCAL-INCLUDE-TWO\n' >corpus-shared/rules/corpus-shared-rule.md
	ar generate --offline --check
	if [ "$AR_RC" -eq 0 ]; then
		fail_note "generate --check missed an edit in a local include"
	fi
	ar generate --offline
	expect_rc 0 "regenerate after editing the include"
	if grep -rqF CORPUS-LOCAL-INCLUDE-ONE --include='*.md' --exclude-dir=.git --exclude-dir=corpus-shared .; then
		fail_note "stale include content survived regeneration"
	fi
	ar generate --offline --check
	expect_rc 0 "in sync after regeneration"
	# A path that leaves the project must be refused, not followed.
	fresh_copy "$W.esc"
	mkdir -p "$W.outside/rules"
	printf '# Out\n\nCORPUS-ESCAPE-MARKER\n' >"$W.outside/rules/out.md"
	cd "$W.esc" || fail "no work dir"
	prep_hermetic || finish
	cfg_append "[[includes]]
name = \"corpus-escape\"
source = \"../$(basename "$W.outside")\""
	ar generate --offline
	if [ "$AR_RC" -eq 0 ]; then
		fail_note "an include outside the project was accepted"
	fi
	if grep -rqF CORPUS-ESCAPE-MARKER --exclude-dir=.git . 2>/dev/null; then
		fail_note "content outside the project reached the outputs"
	fi
	cd "$W" || fail "no work dir"
	rm -rf "$W.esc" "$W.outside"
	finish "local include generated, edited and bounded"
}

# --------------------------------------------------------------------- profile

phase_profile() {
	fresh_copy
	cd "$W" || fail "no work dir"
	cfg_path >/dev/null || skip "no ai-rulez configuration"
	prep_hermetic || finish
	local profiles p n=0 first="" second=""
	profiles="$(cfg_table_keys profiles)"
	[ -n "$profiles" ] || skip "no profiles declared"
	ar profile list
	expect_rc 0 "profile list"
	for p in $profiles; do
		n=$((n + 1))
		[ -n "$first" ] || first="$p"
		[ -n "$second" ] || [ "$p" = "$first" ] || second="$p"
		ar generate --offline --profile "$p"
		expect_rc 0 "generate --profile $p"
		ar generate --offline --profile "$p" --check
		expect_rc 0 "--profile $p --check"
		ar validate --offline --profile "$p" 2>/dev/null
		ar tokens --profile "$p"
		expect_rc_in "tokens --profile $p" 0 2
	done
	if [ -n "$second" ]; then
		ar generate --offline --profile "$first,$second"
		expect_rc 0 "generate --profile $first,$second"
	fi
	ar generate --offline --profile corpus-no-such-profile
	if [ "$AR_RC" -eq 0 ]; then
		fail_note "an unknown profile was accepted"
	fi
	finish "$n declared profile(s) generated and checked"
}

# ------------------------------------------------------------------------ role

phase_role() {
	fresh_copy
	cd "$W" || fail "no work dir"
	cfg_path >/dev/null || skip "no ai-rulez configuration"
	prep_hermetic || finish
	local roles r n=0
	ar_to "$PH_LOGS/roles.json" roles list --format json
	roles="$(jq -r '.roles // [] | .[].name' "$PH_LOGS/roles.json" 2>/dev/null)"
	[ -n "$roles" ] || skip "no roles declared"
	for r in $roles; do
		n=$((n + 1))
		ar roles show "$r"
		expect_rc 0 "roles show $r"
		ar roles resolve "$r"
		expect_rc 0 "roles resolve $r"
		ar generate --offline --role "$r"
		expect_rc 0 "generate --role $r"
		ar sbom --role "$r" -o "$PH_LOGS/sbom-$r.json"
		expect_rc 0 "sbom --role $r"
	done
	finish "$n declared role(s) resolved and generated"
}

# ------------------------------------------------------------------- recursive

phase_recursive() {
	local parent="$W"
	fresh_copy "$parent/main"
	cd "$parent/main" || fail "no work dir"
	cfg_path >/dev/null || skip "no ai-rulez configuration"
	sgit worktree add "$parent/wt" -b corpus-wt || skip "git worktree unavailable"
	local d
	for d in main wt; do
		cd "$parent/$d" || fail "no work dir"
		prep_hermetic || finish
	done
	# Fixture trees hold deliberately invalid configurations (old versions,
	# renamed tables); they are test data, not projects, so leave them out of
	# the recursive scan. Only the private copies are touched.
	for d in main wt; do
		find "$parent/$d" -type d \( -name testdata -o -name fixtures \) -prune -exec rm -rf {} + 2>/dev/null
	done
	cd "$parent" || fail "no work dir"
	ar generate -r --offline
	expect_rc 0 "generate -r over a checkout and its worktree"
	for d in main wt; do
		if [ -z "$(ls "$parent/$d"/CLAUDE.md "$parent/$d"/AGENTS.md "$parent/$d"/.claude 2>/dev/null)" ]; then
			fail_note "no outputs written in $d"
		fi
	done
	ar generate -r --offline --check
	expect_rc 0 "generate -r --check"
	ar validate -r
	expect_rc_in "validate -r" 0 2
	finish "recursive run covered the checkout and its worktree"
}
