"""Offline tests for the npm packager and the shared archive verifier."""

import hashlib
import importlib.util
import io
import json
import os
import sys
import tarfile
import tempfile
import unittest
import zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import _archives  # noqa: E402


def _load(name: str):
    spec = importlib.util.spec_from_file_location(name, HERE / f"{name}.py")
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


package_npm = _load("package_npm")

VERSION = "1.2.3-rc.4"
LEGAL = {
    "LICENSE": b"license",
    "README.md": b"readme",
    "THIRD_PARTY_NOTICES.md": b"notices",
    "LICENSES/MIT.txt": b"mit",
}
GLIBC = {"amd64": b"\x7fELF GLIBC_2.34 GLIBC_2.17 end", "arm64": b"\x7fELF GLIBC_2.31 GLIBC_2.17 end"}


def make_tar(path: Path, files: dict[str, bytes], extra=None) -> None:
    with tarfile.open(path, "w:gz") as tar:
        for name, data in files.items():
            info = tarfile.TarInfo(name)
            info.size = len(data)
            info.mode = 0o644
            tar.addfile(info, io.BytesIO(data))
        for info in extra or []:
            tar.addfile(info)


def make_zip(path: Path, files: dict[str, bytes]) -> None:
    with zipfile.ZipFile(path, "w") as zf:
        for name, data in files.items():
            zf.writestr(name, data)


def write_checksums(directory: Path) -> None:
    rows = []
    for path in sorted(directory.iterdir()):
        if path.name.endswith((".tar.gz", ".zip")):
            rows.append(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}")
    (directory / "checksums.txt").write_text("\n".join(rows) + "\n")


def make_archives(directory: Path, version: str = VERSION, dll: bool = True) -> None:
    for os_name, arch in _archives.TARGETS:
        name = _archives.archive_name(version, os_name, arch)
        if os_name == "linux":
            make_tar(directory / name, {"ai-rulez": GLIBC[arch], **LEGAL})
        elif os_name == "darwin":
            make_tar(directory / name, {"ai-rulez": b"macho " + arch.encode(), **LEGAL})
        else:
            files = {"ai-rulez.exe": b"MZ exe", **LEGAL}
            if dll:
                files["liter_llm_ffi.dll"] = b"MZ dll"
            make_zip(directory / name, files)
    write_checksums(directory)


class Base(unittest.TestCase):
    def setUp(self) -> None:
        temporary = tempfile.TemporaryDirectory(prefix="ai-rulez-pkg-")
        self.addCleanup(temporary.cleanup)
        self.tmp = Path(temporary.name)
        self.archives = self.tmp / "archives"
        self.archives.mkdir()
        self.out = self.tmp / "out"


class NpmPackaging(Base):
    def run_npm(self, **kwargs) -> list[str]:
        make_archives(self.archives, **kwargs)
        return package_npm.package_all(self.archives, VERSION, self.out)

    def manifest(self, directory: str) -> dict:
        return json.loads((self.out / directory / "package.json").read_text())

    def test_five_packages_with_expected_fields(self) -> None:
        names = self.run_npm()
        self.assertEqual(
            sorted(names),
            [
                "@ai-rulez/darwin-arm64",
                "@ai-rulez/darwin-x64",
                "@ai-rulez/linux-arm64",
                "@ai-rulez/linux-x64",
                "@ai-rulez/win32-x64",
            ],
        )
        dirs = sorted(p.name for p in self.out.iterdir())
        self.assertEqual(dirs, ["darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64", "root", "win32-x64"])
        expected = {
            "linux-x64": ("linux", "x64", ["glibc"]),
            "linux-arm64": ("linux", "arm64", ["glibc"]),
            "darwin-x64": ("darwin", "x64", None),
            "darwin-arm64": ("darwin", "arm64", None),
            "win32-x64": ("win32", "x64", None),
        }
        for directory, (os_name, cpu, libc) in expected.items():
            data = self.manifest(directory)
            self.assertEqual(data["os"], [os_name])
            self.assertEqual(data["cpu"], [cpu])
            self.assertEqual(data.get("libc"), libc)
            self.assertEqual(data["version"], VERSION)
            self.assertEqual(data["license"], "MIT AND Apache-2.0 AND Unicode-3.0")
            self.assertEqual(data["publishConfig"], {"access": "public"})
            self.assertEqual(data["files"], ["bin", "LICENSE", "LICENSES", "THIRD_PARTY_NOTICES.md", "README.md"])
            self.assertTrue((self.out / directory / "LICENSES" / "MIT.txt").is_file())
            self.assertIn(os_name, (self.out / directory / "README.md").read_text())

    def test_only_packages_the_selected_target(self) -> None:
        make_archives(self.archives)
        keep = _archives.archive_name(VERSION, "linux", "amd64")
        for path in self.archives.glob("ai-rulez_*"):
            if path.name != keep:
                path.unlink()
        names = package_npm.package_all(self.archives, VERSION, self.out, _archives.parse_only("linux_amd64"))
        self.assertEqual(names, ["@ai-rulez/linux-x64"])
        self.assertEqual(sorted(p.name for p in self.out.iterdir()), ["linux-x64", "root"])

    def test_only_still_rejects_an_unselected_archive(self) -> None:
        make_archives(self.archives)
        with self.assertRaisesRegex(_archives.ArchiveError, "unexpected extra archive"):
            package_npm.package_all(self.archives, VERSION, self.out, _archives.parse_only("linux_amd64"))

    def test_only_rejects_unknown_target(self) -> None:
        with self.assertRaisesRegex(_archives.ArchiveError, "unknown target 'plan9_amd64'"):
            _archives.parse_only("plan9_amd64")

    def test_root_optional_dependencies_are_exact(self) -> None:
        self.run_npm()
        raw = (self.out / "root" / "package.json").read_text()
        self.assertTrue(raw.endswith("\n"))
        manifest = json.loads(raw)
        self.assertEqual(manifest["version"], VERSION)
        deps = manifest["optionalDependencies"]
        self.assertEqual(len(deps), 5)
        self.assertEqual(set(deps.values()), {VERSION})
        self.assertEqual(list(deps), sorted(deps))

    def test_dll_only_when_archive_had_it(self) -> None:
        self.run_npm()
        self.assertTrue((self.out / "win32-x64/bin/liter_llm_ffi.dll").is_file())
        self.assertEqual([p.name for p in (self.out / "win32-x64/bin").glob("*.dll")], ["liter_llm_ffi.dll"])

    def test_no_dll_when_absent(self) -> None:
        self.run_npm(dll=False)
        self.assertEqual(sorted(p.name for p in (self.out / "win32-x64/bin").iterdir()), ["ai-rulez.exe"])

    @unittest.skipIf(os.name == "nt", "POSIX mode bits")
    def test_exec_bit_on_posix_binaries(self) -> None:
        self.run_npm()
        for directory in ("linux-x64", "linux-arm64", "darwin-x64", "darwin-arm64"):
            mode = (self.out / directory / "bin" / "ai-rulez").stat().st_mode & 0o777
            self.assertEqual(mode, 0o755, directory)

    def test_cli_prints_json_summary(self) -> None:
        make_archives(self.archives)
        stdout = io.StringIO()
        old, sys.stdout = sys.stdout, stdout
        try:
            code = package_npm.main(["--archives", str(self.archives), "--version", VERSION, "--out", str(self.out)])
        finally:
            sys.stdout = old
        self.assertEqual(code, 0)
        self.assertEqual(len(json.loads(stdout.getvalue())["packages"]), 5)


class ArchiveVerification(Base):
    def expect(self, message: str, version: str = VERSION) -> None:
        with self.assertRaises(_archives.ArchiveError) as caught:
            _archives.extract_all(self.archives, version, self.tmp / "work")
        self.assertIn(message, str(caught.exception))

    def test_checksum_mismatch(self) -> None:
        make_archives(self.archives)
        name = _archives.archive_name(VERSION, "linux", "amd64")
        with (self.archives / name).open("ab") as handle:
            handle.write(b"tamper")
        self.expect(f"checksum mismatch for {name}")

    def test_conflicting_duplicate_checksum_entry(self) -> None:
        make_archives(self.archives)
        name = _archives.archive_name(VERSION, "linux", "amd64")
        sums = self.archives / "checksums.txt"
        sums.write_text(sums.read_text() + f"{'0' * 64}  {name}\n")
        self.expect(f"checksums.txt lists {name} twice with different digests")

    def test_identical_duplicate_checksum_entry_is_accepted(self) -> None:
        make_archives(self.archives)
        sums = self.archives / "checksums.txt"
        sums.write_text(sums.read_text() + sums.read_text().splitlines()[0] + "\n")
        self.assertEqual(len(package_npm.package_all(self.archives, VERSION, self.out)), 5)

    def test_version_with_trailing_newline_is_rejected(self) -> None:
        with self.assertRaisesRegex(_archives.ArchiveError, "invalid version"):
            _archives.check_version(VERSION + "\n")

    def test_missing_checksums_file(self) -> None:
        make_archives(self.archives)
        (self.archives / "checksums.txt").unlink()
        self.expect("checksums.txt not found")

    def test_missing_checksum_entry(self) -> None:
        make_archives(self.archives)
        name = _archives.archive_name(VERSION, "darwin", "arm64")
        lines = [ln for ln in (self.archives / "checksums.txt").read_text().splitlines() if name not in ln]
        (self.archives / "checksums.txt").write_text("\n".join(lines) + "\n")
        self.expect(f"no entry for {name}")

    def test_missing_archive(self) -> None:
        make_archives(self.archives)
        name = _archives.archive_name(VERSION, "windows", "amd64")
        (self.archives / name).unlink()
        self.expect(f"missing archive: {name}")

    def test_extra_archive(self) -> None:
        make_archives(self.archives)
        make_tar(self.archives / "ai-rulez_1.2.3-rc.4_freebsd_amd64.tar.gz", {"ai-rulez": b"x"})
        write_checksums(self.archives)
        self.expect("unexpected extra archive: ai-rulez_1.2.3-rc.4_freebsd_amd64.tar.gz")

    def test_path_traversal_member(self) -> None:
        make_archives(self.archives)
        name = _archives.archive_name(VERSION, "linux", "amd64")
        make_tar(self.archives / name, {"ai-rulez": b"x", "../evil": b"x"})
        write_checksums(self.archives)
        self.expect("unsafe archive member '../evil'")
        self.assertFalse((self.tmp / "evil").exists())

    def test_absolute_member(self) -> None:
        make_archives(self.archives)
        name = _archives.archive_name(VERSION, "linux", "arm64")
        make_tar(self.archives / name, {"/etc/evil": b"x"})
        write_checksums(self.archives)
        self.expect("unsafe archive member '/etc/evil'")

    def test_symlink_member(self) -> None:
        make_archives(self.archives)
        name = _archives.archive_name(VERSION, "darwin", "amd64")
        link = tarfile.TarInfo("ai-rulez")
        link.type = tarfile.SYMTYPE
        link.linkname = "/bin/sh"
        make_tar(self.archives / name, {}, extra=[link])
        write_checksums(self.archives)
        self.expect("link member 'ai-rulez'")

    def test_invalid_version(self) -> None:
        make_archives(self.archives)
        self.expect("invalid version", version="v1.2")


if __name__ == "__main__":
    unittest.main()
