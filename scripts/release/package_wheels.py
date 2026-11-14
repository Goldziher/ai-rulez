"""Wrap the release archives into platform wheels using only zipfile."""

import argparse
import base64
import hashlib
import re
import sys
import tempfile
import zipfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import _archives  # noqa: E402

REPO_ROOT = Path(__file__).resolve().parents[2]
PYPI_DIR = REPO_ROOT / "release" / "pypi"
GLIBC_RE = re.compile(rb"GLIBC_2\.(\d+)")
ZIP_DATE = (1980, 1, 1, 0, 0, 0)

# Mirrors the setup() call in release/pypi/setup.py; a unit test keeps the summary in step.
METADATA = {
    "Name": "ai-rulez",
    "Summary": (
        "Author, generate, validate and govern rules, skills, plugins and MCP configuration for 52 AI coding "
        "assistants from one .ai-rulez/ source. Lock, sign and SBOM your agent configuration, review it with "
        "verified model evidence, and publish plugin bundles. CLI, MCP server and Go API."
    ),
    "Author": "Na'aman Hirschfeld",
    "Author-email": "nhirschfeld@gmail.com",
    "License-Expression": "MIT AND Apache-2.0 AND Unicode-3.0",
    "Requires-Python": ">=3.11",
    "Keywords": (
        "ai-rules,agents-md,agent-skills,agent-plugins,claude-code,cursor,codex,copilot,mcp,ai-governance,"
        "sbom,sigstore,supply-chain,llms-txt,cli"
    ),
    "Project-URL": [
        "Homepage, https://goldziher.github.io/ai-rulez/",
        "Documentation, https://goldziher.github.io/ai-rulez/",
        "Bug Reports, https://github.com/Goldziher/ai-rulez/issues",
        "Source, https://github.com/Goldziher/ai-rulez",
        "Changelog, https://github.com/Goldziher/ai-rulez/blob/main/CHANGELOG.md",
        "Funding, https://github.com/sponsors/Goldziher",
    ],
    "Classifier": [
        "Development Status :: 5 - Production/Stable",
        "Intended Audience :: Developers",
        "Topic :: Software Development :: Code Generators",
        "Topic :: Software Development :: Quality Assurance",
        "Topic :: Security",
        "Topic :: System :: Software Distribution",
        "Topic :: Software Development :: Pre-processors",
        "Topic :: Text Processing :: General",
        "Topic :: Utilities",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3 :: Only",
        "Programming Language :: Python :: 3.11",
        "Programming Language :: Python :: 3.12",
        "Programming Language :: Python :: 3.13",
        "Programming Language :: Python :: 3.14",
        "Programming Language :: Go",
        "Operating System :: MacOS",
        "Operating System :: Microsoft :: Windows",
        "Operating System :: POSIX :: Linux",
        "Environment :: Console",
    ],
}

CLI_SOURCE = """import os
import subprocess
import sys
from pathlib import Path


def main() -> None:
    name = "ai-rulez.exe" if sys.platform == "win32" else "ai-rulez"
    binary = Path(__file__).parent / "bin" / name
    if not binary.is_file():
        sys.exit(f"ai-rulez: bundled binary not found at {binary}")
    argv = [str(binary), *sys.argv[1:]]
    if sys.platform == "win32":
        sys.exit(subprocess.run(argv).returncode)
    os.execv(str(binary), argv)


if __name__ == "__main__":
    main()
"""


def pep440(version: str) -> str:
    return re.sub(r"-rc\.(\d+)$", r"rc\1", version)


def glibc_minor(binary: Path) -> int:
    minors = [int(m) for m in GLIBC_RE.findall(binary.read_bytes())]
    if not minors:
        raise _archives.ArchiveError(f"no GLIBC_2.<minor> symbol found in {binary.parent.name}/{binary.name}")
    return max(minors)


def platform_tag(item: _archives.Extracted) -> str:
    if item.os == "darwin":
        return "macosx_14_0_arm64" if item.arch == "arm64" else "macosx_14_0_x86_64"
    if item.os == "windows":
        return "win_amd64"
    machine = "aarch64" if item.arch == "arm64" else "x86_64"
    return f"manylinux_2_{glibc_minor(item.binary)}_{machine}"


def metadata_text(version: str, license_files: list[str], long_description: str) -> str:
    lines = ["Metadata-Version: 2.4", f"Name: {METADATA['Name']}", f"Version: {version}"]
    for key in ("Summary", "Author", "Author-email", "License-Expression", "Keywords", "Requires-Python"):
        lines.append(f"{key}: {METADATA[key]}")
    lines += [f"Project-URL: {url}" for url in METADATA["Project-URL"]]
    lines += [f"Classifier: {value}" for value in METADATA["Classifier"]]
    lines.append("Description-Content-Type: text/markdown")
    lines += [f"License-File: {name}" for name in license_files]
    return "\n".join(lines) + "\n\n" + long_description


def record_row(name: str, data: bytes) -> str:
    digest = base64.urlsafe_b64encode(hashlib.sha256(data).digest()).rstrip(b"=").decode()
    return f"{name},sha256={digest},{len(data)}"


def build_wheel(item: _archives.Extracted, version: str, out: Path) -> Path:
    py_version = pep440(version)
    tag = platform_tag(item)
    dist_info = f"ai_rulez-{py_version}.dist-info"
    assert item.license and item.notices and item.licenses_dir
    legal = {"LICENSE": item.license.read_bytes(), "THIRD_PARTY_NOTICES.md": item.notices.read_bytes()}
    for path in sorted(item.licenses_dir.rglob("*")):
        if path.is_file():
            legal[f"LICENSES/{path.relative_to(item.licenses_dir).as_posix()}"] = path.read_bytes()
    readme = PYPI_DIR / "README.md"
    if not readme.is_file():
        readme = REPO_ROOT / "README.md"
    init = (PYPI_DIR / "ai_rulez" / "__init__.py").read_text(encoding="utf-8")
    init = re.sub(r'__version__ = "[^"]*"', f'__version__ = "{py_version}"', init)
    metadata = metadata_text(py_version, list(legal), readme.read_text(encoding="utf-8"))
    files: dict[str, tuple[bytes, int]] = {
        "ai_rulez/__init__.py": (init.encode(), 0o644),
        "ai_rulez/cli.py": (CLI_SOURCE.encode(), 0o644),
        f"ai_rulez/bin/{item.binary.name}": (item.binary.read_bytes(), 0o644 if item.os == "windows" else 0o755),
        f"{dist_info}/METADATA": (metadata.encode(), 0o644),
        f"{dist_info}/WHEEL": (
            f"Wheel-Version: 1.0\nGenerator: ai-rulez-release\nRoot-Is-Purelib: false\nTag: py3-none-{tag}\n".encode(),
            0o644,
        ),
        f"{dist_info}/entry_points.txt": (b"[console_scripts]\nai-rulez = ai_rulez.cli:main\n", 0o644),
    }
    for dll in item.dlls:
        files[f"ai_rulez/bin/{dll.name}"] = (dll.read_bytes(), 0o644)
    for name, data in legal.items():
        files[f"{dist_info}/licenses/{name}"] = (data, 0o644)
    record_name = f"{dist_info}/RECORD"
    rows = [record_row(name, data) for name, (data, _) in sorted(files.items())]
    rows.append(f"{record_name},,")
    files[record_name] = (("\n".join(rows) + "\n").encode(), 0o644)

    wheel = out / f"ai_rulez-{py_version}-py3-none-{tag}.whl"
    with zipfile.ZipFile(wheel, "w", zipfile.ZIP_DEFLATED) as zf:
        for name in sorted(files, key=lambda n: (n == record_name, n)):
            data, mode = files[name]
            info = zipfile.ZipInfo(name, ZIP_DATE)
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = (0o100000 | mode) << 16
            zf.writestr(info, data)
    return wheel


def package_all(archives: Path, version: str, out: Path) -> list[Path]:
    out.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="ai-rulez-wheel-") as work:
        extracted = _archives.extract_all(archives, version, Path(work))
        return [build_wheel(extracted[target], version, out) for target in _archives.TARGETS]


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archives", type=Path, required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        wheels = package_all(args.archives, args.version, args.out)
    except _archives.ArchiveError as error:
        print(f"package_wheels: {error}", file=sys.stderr)
        return 1
    for wheel in wheels:
        print(wheel)
    return 0


if __name__ == "__main__":
    sys.exit(main())
