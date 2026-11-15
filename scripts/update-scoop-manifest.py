#!/usr/bin/env python3
"""Update an existing Scoop manifest from a stable release's published checksums."""

import argparse
import json
import re
from pathlib import Path

RELEASES = "https://github.com/Goldziher/ai-rulez/releases/download"


def stable_version(version: str) -> tuple[int, ...]:
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        raise ValueError(f"expected a stable X.Y.Z version, got {version!r}")
    return tuple(map(int, version.split(".")))


def update(version: str, checksums: Path, manifest_path: Path) -> None:
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    if stable_version(version) < stable_version(manifest["version"]):
        return

    archive = f"ai-rulez_{version}_windows_amd64.zip"
    hashes = []
    for line in checksums.read_text(encoding="utf-8").splitlines():
        fields = line.split()
        if len(fields) == 2 and fields[1].removeprefix("*") == archive:
            hashes.append(fields[0])
    if len(hashes) != 1 or not re.fullmatch(r"[0-9a-f]{64}", hashes[0]):
        raise ValueError(f"expected exactly one valid SHA-256 for {archive} in {checksums}")

    manifest["version"] = version
    manifest["architecture"]["64bit"].update(
        url=f"{RELEASES}/v{version}/{archive}",
        hash=hashes[0],
    )
    manifest_path.write_text(json.dumps(manifest, indent=4) + "\n", encoding="utf-8")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version")
    parser.add_argument("checksums", type=Path)
    parser.add_argument("manifest", type=Path)
    args = parser.parse_args()
    try:
        update(args.version, args.checksums, args.manifest)
    except (OSError, ValueError, KeyError) as error:
        parser.exit(1, f"{error}\n")


if __name__ == "__main__":
    main()
