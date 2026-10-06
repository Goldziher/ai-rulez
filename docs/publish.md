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
| `ai-rulez.lock` | a copy, same bytes as the repository's |
| `SHA256SUMS` | `sha256sum -c` format, every file except itself and the plan |
| `RELEASE_NOTES.md` | bundle, runtimes, lock tree, commit |
| `publish-plan.json` | artifacts with digests and the exact argv `--execute` runs; no timestamps or local paths |
| `emit/*` | output of `--template` files |

A dist directory must be new, empty, or the output of an earlier publish (its artifacts are replaced; other files are
kept, and any other non-empty directory is refused).

## Determinism

Equal bundle files and mtime give a byte-identical archive across runs, operating systems and umasks: entries sorted
bytewise, regular files only, uid/gid 0 with no names, modes 0644 or 0755, GNU tar headers (no PAX), gzip level 9 with
no name or time in the header. The mtime is `SOURCE_DATE_EPOCH`, else the committer time of `HEAD`, else 0. The gzip
bytes are those of the Go toolchain that built ai-rulez; `publish verify` therefore checks digests and contents, never
by recompressing.

## GitHub release

`--to github-release` uses `--tag` (default `v<version>`, which must already exist on the remote: `--verify-tag`) and
`--repo` (default `[plugin] repository`, else the origin remote; `OWNER/REPO` or `HOST/OWNER/REPO`). Tag and repo are
validated against an allowlist before reaching an argv. `--execute` checks `gh release view` first and refuses an
existing release (releases are immutable) unless `--force`, which runs `gh release upload --clobber` instead. gh gets
only its own variables (`GH_TOKEN`, `GITHUB_TOKEN`, `GH_HOST`, `GH_CONFIG_DIR`, proxy and certificate settings); gh
missing is error `AR9N4` with an install hint.

## Verify

`publish verify <dir>` checks `SHA256SUMS` against the files, the manifest against the archive (every file, size and
digest), the lock copy against `lock.file_digest`, the plan against `SHA256SUMS`, and the archive against the
determinism rules. Exit 0 verified, 2 mismatch, 1 unreadable directory.

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
