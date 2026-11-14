"""Verify and extract the release archives shared by the registry packagers."""

import hashlib
import re
import shutil
import tarfile
import zipfile
from dataclasses import dataclass, field
from pathlib import Path, PurePosixPath

TARGETS = (
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("windows", "amd64"),
)
VERSION_RE = re.compile(r"^\d+\.\d+\.\d+(-rc\.\d+)?$")
ARCHIVE_SUFFIXES = (".tar.gz", ".zip")
LEGAL_FILES = ("LICENSE", "THIRD_PARTY_NOTICES.md", "README.md")


class ArchiveError(Exception):
    pass


@dataclass
class Extracted:
    os: str
    arch: str
    root: Path
    binary: Path
    dlls: list[Path] = field(default_factory=list)
    license: Path | None = None
    licenses_dir: Path | None = None
    notices: Path | None = None
    readme: Path | None = None


def check_version(version: str) -> str:
    if not VERSION_RE.match(version):
        raise ArchiveError(f"invalid version {version!r}: expected X.Y.Z or X.Y.Z-rc.N")
    return version


def archive_name(version: str, os_name: str, arch: str) -> str:
    suffix = ".zip" if os_name == "windows" else ".tar.gz"
    return f"ai-rulez_{version}_{os_name}_{arch}{suffix}"


def parse_checksums(path: Path) -> dict[str, str]:
    if not path.is_file():
        raise ArchiveError(f"checksums.txt not found in {path.parent}")
    sums: dict[str, str] = {}
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if not line.strip():
            continue
        match = re.fullmatch(r"([0-9a-fA-F]{64}) [ *]?(\S.*)", line)
        if not match:
            raise ArchiveError(f"checksums.txt line {number} is not in sha256sum format")
        sums[match.group(2)] = match.group(1).lower()
    return sums


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _safe_relative(name: str) -> PurePosixPath:
    posix = PurePosixPath(name.replace("\\", "/"))
    if posix.is_absolute() or re.match(r"^[A-Za-z]:", name) or ".." in posix.parts:
        raise ArchiveError(f"unsafe archive member {name!r}")
    return posix


def _extract_tar(archive: Path, dest: Path) -> None:
    with tarfile.open(archive, "r:gz") as tar:
        members = tar.getmembers()
        for member in members:
            rel = _safe_relative(member.name)
            if member.issym() or member.islnk():
                raise ArchiveError(f"link member {member.name!r} in {archive.name}")
            if not (member.isfile() or member.isdir()):
                raise ArchiveError(f"unsupported member type {member.name!r} in {archive.name}")
            target = dest.joinpath(*rel.parts)
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            source = tar.extractfile(member)
            assert source is not None
            with source, target.open("wb") as out:
                shutil.copyfileobj(source, out)


def _extract_zip(archive: Path, dest: Path) -> None:
    with zipfile.ZipFile(archive) as zf:
        for info in zf.infolist():
            rel = _safe_relative(info.filename)
            if (info.external_attr >> 16) & 0o170000 == 0o120000:
                raise ArchiveError(f"symlink member {info.filename!r} in {archive.name}")
            target = dest.joinpath(*rel.parts)
            if info.is_dir():
                target.mkdir(parents=True, exist_ok=True)
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            with zf.open(info) as source, target.open("wb") as out:
                shutil.copyfileobj(source, out)


def _locate(root: Path, os_name: str, arch: str) -> Extracted:
    binary = root / ("ai-rulez.exe" if os_name == "windows" else "ai-rulez")
    if not binary.is_file():
        raise ArchiveError(f"{os_name}_{arch} archive has no {binary.name} at its root")
    found = Extracted(os_name, arch, root, binary, dlls=sorted(root.glob("*.dll")))
    found.license = root / "LICENSE"
    found.notices = root / "THIRD_PARTY_NOTICES.md"
    found.readme = root / "README.md"
    found.licenses_dir = root / "LICENSES"
    for label, path in (
        ("LICENSE", found.license),
        ("THIRD_PARTY_NOTICES.md", found.notices),
        ("README.md", found.readme),
    ):
        if not path.is_file():
            raise ArchiveError(f"{os_name}_{arch} archive is missing {label}")
    if not found.licenses_dir.is_dir():
        raise ArchiveError(f"{os_name}_{arch} archive is missing LICENSES/")
    return found


def parse_only(only: str | None) -> tuple[tuple[str, str], ...]:
    """Targets selected by a comma-separated `--only os_arch` list; every target when unset."""
    if not only:
        return TARGETS
    chosen = []
    for token in only.split(","):
        target = tuple(token.strip().split("_", 1))
        if target not in TARGETS:
            raise ArchiveError(f"unknown target {token!r}: expected one of {', '.join('_'.join(t) for t in TARGETS)}")
        if target not in chosen:
            chosen.append(target)
    return tuple(chosen)


def extract_all(
    archives: Path, version: str, work: Path, targets: tuple[tuple[str, str], ...] = TARGETS
) -> dict[tuple[str, str], Extracted]:
    """Verify every archive against checksums.txt, then extract each into work/<os>_<arch>.

    `targets` narrows the expected set for local rehearsals; the release workflow always uses all of them.
    """
    check_version(version)
    if not archives.is_dir():
        raise ArchiveError(f"archives directory {archives} does not exist")
    sums = parse_checksums(archives / "checksums.txt")
    expected = {archive_name(version, o, a): (o, a) for o, a in targets}
    present = {p.name for p in archives.iterdir() if p.name.endswith(ARCHIVE_SUFFIXES)}
    missing = sorted(set(expected) - present)
    if missing:
        raise ArchiveError(f"missing archive: {', '.join(missing)}")
    extra = sorted(present - set(expected))
    if extra:
        raise ArchiveError(f"unexpected extra archive: {', '.join(extra)}")
    result: dict[tuple[str, str], Extracted] = {}
    for name, (os_name, arch) in expected.items():
        if name not in sums:
            raise ArchiveError(f"checksums.txt has no entry for {name}")
        actual = sha256_file(archives / name)
        if actual != sums[name]:
            raise ArchiveError(f"checksum mismatch for {name}: expected {sums[name]}, got {actual}")
        dest = work / f"{os_name}_{arch}"
        dest.mkdir(parents=True)
        (_extract_zip if name.endswith(".zip") else _extract_tar)(archives / name, dest)
        result[(os_name, arch)] = _locate(dest, os_name, arch)
    return result
