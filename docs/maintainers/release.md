# Cutting a release

This page is for maintainers. It is not in the site navigation.

ai-rulez links the [liter-llm](https://github.com/xberg-io/liter-llm) native library through cgo, so a release binary
cannot be cross-compiled from one runner. Each platform archive is built natively on its own runner by
`.github/workflows/publish.yaml`. There is no GoReleaser.

## Cut a release

1. Bump every surface that carries the version, merge the result, and let CI go green:

   ```bash
   scripts/release-bump.sh set 5.1.0        # or 5.1.0-rc.1
   ```

   The script rewrites `release/npm/package.json`, `release/pypi/__init__.py`, `release/pypi/ai_rulez/__init__.py`
   (PEP 440 form, `5.1.0rc1`) and the default of `var version` in `cmd/ai-rulez/main.go`. `check <version>` verifies
   them without writing.

2. Optionally rehearse the packaging on your own machine (see [Dry run](#dry-run)).
3. Tag and push: `git tag v5.1.0 && git push origin v5.1.0`.

The workflow does the rest:

| Job               | What it does                                                                                              |
| ----------------- | --------------------------------------------------------------------------------------------------------- |
| `meta`            | Parses the tag, fails unless every version surface equals it (`release-bump.sh check`) and the liter-llm version resolves from `go.mod`. Detects what is already published. |
| `create_release`  | Creates the GitHub release as a **draft** (a `-rc.N` tag is marked pre-release).                          |
| `build`           | One job per platform (below): fetch the pinned native library, `go build` with cgo, smoke-test, package.  |
| `checksums`       | Verifies the archive set is exactly the expected five, writes `checksums.txt`, uploads everything to the draft. |
| `provenance`      | SLSA provenance over every binary and archive; uploads one `<archive>.sigstore.json` per archive.         |
| `finalize_release`| Promotes the draft only when archives, `checksums.txt` and provenance are all attached.                   |
| `publish_npm` / `publish_pypi` / `publish_homebrew` | Publish after the release is public. Homebrew is skipped for `-rc.N`.            |

A failed platform build therefore leaves a hidden draft, never a live release with missing binaries.

To run the workflow by hand, use **workflow_dispatch** with the tag, and choose the tag as the ref in "Use workflow
from". The `meta` job enforces this: when the dispatch did not come from `refs/tags/<tag>` it fails with a clear error
and publishes nothing. Provenance is signed with the run's ref identity, `ai-rulez verify --self` pins `refs/tags/v...`,
and a run from a branch would otherwise sign `refs/heads/...` and produce binaries every one of which fails
`verify --self`. Reruns and the heal flow are unaffected: re-run from the tag's ref (or re-run the failed jobs of a tag
push), and the same ref identity is carried.

## Platform matrix

| Archive                                   | Runner             | Built in                | Status |
| ----------------------------------------- | ------------------ | ----------------------- | ------ |
| `ai-rulez_<v>_linux_amd64.tar.gz`         | `ubuntu-latest`    | `debian:bookworm`       | same path as arm64, first exercised by CI |
| `ai-rulez_<v>_linux_arm64.tar.gz`         | `ubuntu-24.04-arm` | `debian:bookworm`       | verified (static build and run in a bookworm container) |
| `ai-rulez_<v>_darwin_arm64.tar.gz`        | `macos-14`         | host                    | verified (built and run locally) |
| `ai-rulez_<v>_darwin_amd64.tar.gz`        | `macos-15-intel`   | host                    | same code path as arm64, first exercised by CI |
| `ai-rulez_<v>_windows_amd64.zip`          | `windows-latest`   | host (MinGW gcc)        | **unverified**, see below |

`windows/arm64` is not supported. Archive names, layout (`ai-rulez`/`ai-rulez.exe`, `README.md`, `LICENSE` at the
archive root) and `checksums.txt` are unchanged from the GoReleaser era, so the npm and PyPI wrappers and the Homebrew
formula work as before; both wrappers still fail closed when `checksums.txt` or the archive's entry is missing or does
not match.

Builds use `-trimpath -buildvcs=false -ldflags "-s -w -X main.version=<version>"`; archives have fixed mtimes, sorted
entries and no owner or gzip timestamp, so two builds of the same inputs on the same toolchain are byte-identical.

## How liter-llm is linked

The version comes from `go.mod` (`scripts/liter-llm-version.sh`); a pseudo-version fails. A `go.mod` bump therefore bumps the native asset, and
`LITER_LLM_VERSION` set to anything else is an error.

`scripts/setup-liter-llm.sh` downloads `liter-llm-go-v<version>-<platform>.tar.gz` and its `.sha256` sidecar from the
liter-llm GitHub release, **verifies the sha256, and fails closed** on a missing, malformed or mismatching sidecar. It
extracts only `lib/libliter_llm_ffi.a` into an otherwise empty directory and points cgo at it with
`CGO_LDFLAGS=-L<dir>`. The directory must not contain the shared library: given both, the linker prefers the shared
one and the binary then needs it at runtime. The sidecar comes from the same origin as the archive, so it protects
against corruption and a swapped asset on a mirror, not against a compromised liter-llm release.

`scripts/package-release.sh` then runs the smoke test (`ai-rulez version`, `ai-rulez llm doctor`) and the workflow
checks the packaged binary has no dependency on `liter_llm_ffi` (`otool -L` / `ldd`).

Plain `go build` and `go test` need none of this: the liter-llm Go module ships a shared library per platform in its own
`.lib/` directory and links it with an absolute rpath, which is fine on the machine that built it and wrong for anything
shipped. `task build` and the CI jobs use the static library as a release does. Get the same environment with:

```bash
eval "$(scripts/setup-liter-llm.sh)"          # exports CGO_ENABLED=1 and CGO_LDFLAGS
```

or, in a workflow, the `.github/actions/setup-liter-llm` composite action (exports through `$GITHUB_ENV`).

### glibc floor, no musl

Linux archives are built in `debian:bookworm`, so they need glibc 2.36 or newer. The `build` job fails if the highest
`GLIBC_x.y` symbol the binary needs exceeds 2.36. The native library is glibc-only, so there is no musl or Alpine
build, and the wrappers do not work on Alpine.

### Windows (unverified)

The Windows native asset ships `liter_llm_ffi.lib`, an MSVC static archive, while the Go toolchain's cgo uses MinGW
gcc. Linking one with the other has not been tested. `scripts/package-release.sh` therefore runs with
`WINDOWS_LINK=auto`: it tries the static link and smoke-tests the result; if either fails it rebuilds against
`liter_llm_ffi.dll` and ships the DLL next to `ai-rulez.exe` in the zip. The npm and PyPI wrappers copy a DLL from the
archive beside the binary. The `build` job logs a warning when the fallback was used, and a failure of both paths fails
the job, so the first release will tell.

## Dry run

```bash
scripts/release-dry-run.sh            # builds the host platform's archive with VERSION=0.0.0-dryrun
```

It runs the real `package-release.sh`, builds twice to check reproducibility, writes a `checksums.txt` in the format the
wrappers parse, extracts the archive, runs the binary, checks the archive layout and that no shared liter-llm library
is needed, and renders the Homebrew formula.

## Rerunning

Every stage can be rerun for the same tag, from the Actions UI ("Re-run failed jobs") or by workflow_dispatch:

- `meta` inspects the release. If the archives and `checksums.txt` are all attached it skips `create_release`,
  `build` and `checksums` (published checksums never change under users), and if the provenance bundles are present it
  skips `provenance` too. Only the missing stages run.
- An existing draft is reused; uploads use `--clobber`; `finalize_release` is skipped for an already-public release.
- `publish_npm` and `publish_pypi` skip versions that already exist (PyPI also uses `skip-existing`).
- `publish_homebrew` renders the formula again and does nothing when it is unchanged.

To rebuild a release from scratch, delete its assets (or the draft) and rerun.

## Provenance

The `provenance` job uses `actions/attest-build-provenance` over each binary and archive, signed with this workflow's
identity (`.github/workflows/publish.yaml@refs/tags/v...`, which must stay a top-level workflow of this repository).
Each archive gets `<archive-name>.sigstore.json`. `ai-rulez verify --self` checks the running binary's sha256 against
the bundle next to it or `--attestation-file`; see [Signing](../signing.md#verifying-ai-rulez-itself). The binary is
attested after it is unpacked from the published archive, so the attested bytes are the shipped bytes. A Windows DLL
shipped by the fallback path is covered by the archive digest but is not itself a subject.

## Required secrets

`HOMEBREW_TAP_GITHUB_TOKEN` (push access to `Goldziher/homebrew-tap`) for `publish_homebrew`. npm and PyPI use trusted
publishing (OIDC); no tokens are stored.
