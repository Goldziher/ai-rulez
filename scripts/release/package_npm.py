"""Wrap the release archives into per-platform npm packages plus the root manifest."""

import argparse
import json
import shutil
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import _archives  # noqa: E402

REPO_ROOT = Path(__file__).resolve().parents[2]
LICENSE_EXPRESSION = "MIT AND Apache-2.0 AND Unicode-3.0"
NODE_OS = {"linux": "linux", "darwin": "darwin", "windows": "win32"}
NODE_CPU = {"amd64": "x64", "arm64": "arm64"}


def write_json(path: Path, data: dict) -> None:
    path.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def package_name(os_name: str, arch: str) -> str:
    return f"@ai-rulez/{NODE_OS[os_name]}-{NODE_CPU[arch]}"


def package_one(item: _archives.Extracted, version: str, base: dict, out: Path) -> str:
    name = package_name(item.os, item.arch)
    node_os, node_cpu = NODE_OS[item.os], NODE_CPU[item.arch]
    pkg_dir = out / f"{node_os}-{node_cpu}"
    bin_dir = pkg_dir / "bin"
    bin_dir.mkdir(parents=True)
    for source in [item.binary, *item.dlls]:
        shutil.copyfile(source, bin_dir / source.name)
    (bin_dir / item.binary.name).chmod(0o644 if item.os == "windows" else 0o755)
    assert item.license and item.notices and item.readme and item.licenses_dir
    shutil.copyfile(item.license, pkg_dir / "LICENSE")
    shutil.copyfile(item.notices, pkg_dir / "THIRD_PARTY_NOTICES.md")
    shutil.copytree(item.licenses_dir, pkg_dir / "LICENSES")
    (pkg_dir / "README.md").write_text(
        f"# {name}\n\nPrebuilt ai-rulez binary for {node_os} {node_cpu}. "
        "It is installed automatically by the `ai-rulez` package; install that instead.\n",
        encoding="utf-8",
    )
    manifest = {
        "name": name,
        "version": version,
        "description": f"Prebuilt ai-rulez binary for {node_os} {node_cpu}",
        "license": LICENSE_EXPRESSION,
        "repository": base["repository"],
        "homepage": base["homepage"],
        "author": base["author"],
        "os": [node_os],
        "cpu": [node_cpu],
        "files": ["bin", "LICENSE", "LICENSES", "THIRD_PARTY_NOTICES.md", "README.md"],
        "publishConfig": {"access": "public"},
    }
    if item.os == "linux":
        manifest["libc"] = ["glibc"]
    write_json(pkg_dir / "package.json", manifest)
    return name


def package_all(archives: Path, version: str, out: Path) -> list[str]:
    base = json.loads((REPO_ROOT / "release/npm/package.json").read_text(encoding="utf-8"))
    out.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="ai-rulez-npm-") as work:
        extracted = _archives.extract_all(archives, version, Path(work))
        names = [package_one(extracted[target], version, base, out) for target in _archives.TARGETS]
    root_dir = out / "root"
    root_dir.mkdir()
    root = dict(base, version=version)
    root["optionalDependencies"] = {name: version for name in sorted(names)}
    write_json(root_dir / "package.json", root)
    return names


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archives", type=Path, required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        names = package_all(args.archives, args.version, args.out)
    except _archives.ArchiveError as error:
        print(f"package_npm: {error}", file=sys.stderr)
        return 1
    print(json.dumps({"packages": names, "root": "ai-rulez"}, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
