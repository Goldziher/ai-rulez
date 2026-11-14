---
status: accepted
date: 2026-11-14
decision-makers: ["Project maintainers"]
consulted: []
informed: []
---

# Ship release binaries through the npm and PyPI registries

## Context and Problem Statement

`npx ai-rulez` and `uvx ai-rulez` install a launcher that downloads the Go binary and `checksums.txt` from GitHub
Releases on first run. Agent sandboxes such as Codex often allow the package registries but not github.com or its
release-asset hosts, so the first run fails even though the launcher installed. The 5.12.2 release made the npm
downloader honour `HTTPS_PROXY`, which only helps where github.com is reachable through a proxy.

## Decision Drivers

* The binary must install wherever `npm install` or `pip install` works, with no second network destination.
* Release binaries are built natively with cgo per platform and attested once; they must not be rebuilt for packaging.
* The Linux binary links glibc dynamically, and the Windows archive may carry `liter_llm_ffi.dll` beside the executable.
* Checksum verification must keep failing closed.
* Existing installs, `--no-optional` installs and platforms without a prebuilt package must keep working.

## Considered Options

* Per-platform packages: scoped npm packages as `optionalDependencies` and platform-tagged PyPI wheels, wrapping the released archives.
* Keep the runtime download and document a setup-phase install.
* Adopt gowheels to produce the packages.
* One npm package and one wheel bundling every platform.

## Decision Outcome

Chosen option: "Per-platform packages", because it removes the GitHub dependency from the install path and reuses
the attested release bytes.

The release workflow downloads the finalized archives and `checksums.txt`, verifies every archive against it, and
wraps the binaries in five `@ai-rulez/<os>-<cpu>` npm packages (with `os`, `cpu` and, on Linux, `libc` fields) and
five PyPI wheels tagged `macosx_14_0_*`, `win_amd64` and `manylinux_2_<N>_*`, where N comes from the highest glibc
symbol in the binary. The root `ai-rulez` package lists the platform packages as exact-version optional dependencies and
is published last. The existing pure wheel and sdist remain. Both launchers use the bundled binary first and fall back
to the checksum-verified GitHub download. No musl build exists, so Linux musl reports that instead of downloading.

### Consequences

* Good, because installs need only the registry that delivered the launcher.
* Good, because the bytes in the packages are the bytes the provenance job already attested.
* Good, because the fallback keeps `--no-optional` installs and unlisted platforms working where GitHub is reachable.
* Bad, because every release publishes five more npm packages and five wheels of roughly 35 MB each, which counts against the PyPI project size limit.
* Bad, because each new npm package needs a trusted publisher configured once, after a placeholder publish.
* Bad, because the packaging scripts are code we own instead of a third-party tool.

### Confirmation

`scripts/release/test_package_npm.py` and `scripts/release/test_package_wheels.py` cover checksum failure modes,
package and wheel metadata, tags and archive contents. `scripts/release-dry-run.sh` installs the host wheel and npm
package into clean environments and runs the binary with the GitHub downloader unreachable.

## Pros and Cons of the Options

### Per-platform packages

* Good, because it matches the esbuild and biome pattern and needs no network beyond the registry.
* Good, because it reuses the existing archives, checksums and provenance.
* Bad, because it adds publish jobs, package registrations and a packaging script to maintain.

### Keep the runtime download and document a setup-phase install

* Good, because it needs no release changes.
* Bad, because every restricted-network user must know to warm the binary in a phase that has network access.
* Bad, because `npx -y ai-rulez@latest` can still fetch a newer version later and fail.

### Adopt gowheels

* Good, because it already writes wheel and npm layouts from a GitHub release.
* Bad, because it assumes static binaries and a single file, and its build mode forces `CGO_ENABLED=0`.
* Bad, because its docs show no handling of a sidecar DLL, and the project is young with no releases listed.

### One package bundling every platform

* Good, because the existing packaged-binary path already supports it.
* Bad, because every install downloads about five times the bytes it uses.

## More Information

* [gowheels](https://github.com/StevenACoffman/gowheels)
* [Cutting a release](../docs/maintainers/release.md)
