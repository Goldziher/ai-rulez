# Publish

`ai-rulez publish` turns the generated [plugin bundle](plugins.md) into release artifacts: a reproducible tar.gz, a
manifest, checksums and a plan. Building is local and offline. Only `--execute --yes` sends anything, and it does so by
running the platform CLI (`gh`) with a fixed argv, so ai-rulez never handles credentials.

```bash
ai-rulez generate --plugin && ai-rulez lock     # commit the result first
ai-rulez publish --to github-release --dry-run   # preflight, then print artifacts and commands
ai-rulez publish --to github-release --execute --yes
ai-rulez publish verify dist
```

## Pipeline

1. Resolve: `[plugin] name` and `version` are required (set the version yourself; see AR961).
2. Preflight, in-process, stopping before anything is written: `validate --strict` (same baseline and budget handling,
   never looser than `error`), `lock --check`, `verify --plugin`, and a secret scan of every bundle file with the
   security scan's patterns. A symlink in the bundle is an error. The machine-local overlay is never loaded.
3. Build the dist directory.
4. With `--execute --yes`, run `gh`.

The tree must be clean (`--allow-dirty` waives it; the manifest then records `dirty = true`). Changes under the dist
directory are ignored.

## Dist directory

| File | Content |
| --- | --- |
| `<name>-<version>.tar.gz` | the bundle: every file `generate --plugin` writes for the configured runtimes |
| `<name>-<version>.manifest.json` | files with digests, source, lock tie, bundle digest ([schema](schema.md)) |
| `ai-rulez.lock` | a copy of the repository's lock without its `[[approval]]` records (reviewer emails and notes stay in the repository). Tree, content pins and output pins are unchanged, so `ai-rulez lock --check` against the copy behaves as against the original; only `[[approval]]` checks differ. With no approvals the copy is byte-identical |
| `SHA256SUMS` | `sha256sum -c` format, every file except itself and the plan |
| `RELEASE_NOTES.md` | bundle, runtimes, lock tree, commit |
| `publish-plan.json` | artifacts with digests and the exact argv `--execute` runs; no timestamps or local paths |
| `emit/*` | output of `--template` files |

A dist directory must be new, empty, or the output of an earlier publish (its artifacts are replaced; other files are
kept, and any other non-empty directory is refused). The directory is staged beside the target and installed by rename, the
plan last, so a crash never leaves a directory publish cannot reuse. A symlink at an artifact path, at a directory
component or as `--dist` itself is refused, never written or removed through.

## Determinism

Equal bundle files and mtime give a byte-identical archive across runs, operating systems and umasks: entries sorted
bytewise, regular files only, uid/gid 0 with no names, modes 0644 or 0755, GNU tar headers (no PAX), gzip level 9 with
no name or time in the header. The mtime is `SOURCE_DATE_EPOCH`, else the committer time of `HEAD`, else 0. The gzip
bytes depend on the Go toolchain that built ai-rulez (`compress/flate`), so reproducibility holds per toolchain:
build releases with one pinned Go version. `publish verify` checks digests and contents, never by recompressing.
`internal/publish/testdata/archive-digests.txt` pins the digest of a fixed archive per Go release line; after a toolchain
bump run `UPDATE_GOLDEN=1 go test ./internal/publish -run TestBuildArchive_MatchesTheGolden` and review the diff.
The source tree check lists every untracked file (`--untracked-files=all`) and git queries time out after 30 seconds.

## GitHub release

`--to github-release` uses `--tag` (default `v<version>`, which must already exist on the remote: `--verify-tag`) and
`--repo` (default `[plugin] repository`, else the origin remote; `OWNER/REPO` or `HOST/OWNER/REPO`). Tag and repo are
validated against an allowlist before reaching an argv. `--execute` checks `gh release view` first and refuses an
existing release (releases are immutable) unless `--force`, which runs `gh release upload --clobber` instead. gh gets
only a fixed set of variables: `PATH`, `HOME`, `USER`, `TMPDIR`, `TMP`, `TEMP`, `TZ`, `LANG`,
`LANGUAGE`, `LC_*` (non-secret), `NO_COLOR=1`, `TERM=dumb` (plus the Windows system variables), and `GH_TOKEN`,
`GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, `GH_HOST`, `GH_CONFIG_DIR`, `XDG_CONFIG_HOME`,
`XDG_STATE_HOME`, `XDG_DATA_HOME`, proxy and certificate settings. The printed `would run` line is shell-quoted; gh
missing is error `AR9N4` with an install hint.

## Verify

`publish verify <dir>` checks `SHA256SUMS` against the files (a duplicate entry is a mismatch), flags every file in the
directory that `SHA256SUMS` does not list (the plan and `SHA256SUMS` itself excepted), checks the manifest against the
archive (every file, size and digest), the lock copy against `lock.file_digest`, and its `tree` and `version` against
the manifest's `lock.tree` and `lock.version`, and the archive against the determinism rules. The plan is checked too:
each artifact's path, size and digest against the file and `SHA256SUMS`, and its commands and uploads against the
`gh release create` argv `--execute` would build for the manifest, so an edited plan cannot smuggle in a command. Files
and the archive's total uncompressed size are capped at 512 MiB. Exit 0 verified, 2 mismatch, 1 unreadable directory.

## Template emitter

`--template file.json.tmpl` renders a Go `text/template` into `dist/emit/file.json` for an operator to upload to a
channel without a native emitter. Fields: `Name`, `Version`, `Tag`, `Repo`, `Commit`, `AIRulezVersion`, `Runtimes`,
`BundleFile`, `BundleDigest`, `BundleSize`, `LockTree`, `LockDigest`, `Files`; function `json`. An unknown field is an
error.

## Rule codes

`AR9N0` preflight failed, `AR9N1` unsafe bundle, `AR9N2` secret found, `AR9N3` unreleasable source, `AR9N4` upload
failed, `AR9N5` verify mismatch. They appear in errors only; `validate --explain AR9N2` describes them.

## Design decisions

- Preflight reuses the in-process code of `validate --strict`, `lock --check` and `verify --plugin`, not subprocesses.
- The plan lists only the create command; the existence check and `--force` upload are decided at execution time.
- A tree outside git, or without a commit, counts as dirty: it cannot be tied to a commit.
- `--force` replaces assets rather than deleting the release.
- Only a single `[plugin]` is published; a `[marketplace]`-only root is refused.

## Not yet

npm, OCI and signing targets, Cursor/Port/AWS/Kiro emitters, a marketplace index pinned to the release, release notes
from the lock diff, `--runtime` filtering and approval/signature policy gates. `approval`, `signature` and `sbom` are
`null` in the manifest until those exist.
