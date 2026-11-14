"""Offline tests for the wheel packager."""

import base64
import hashlib
import importlib.util
import re
import sys
import unittest
import zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import _archives  # noqa: E402
from test_package_npm import VERSION, Base, make_archives, make_tar, write_checksums  # noqa: E402

spec = importlib.util.spec_from_file_location("package_wheels", HERE / "package_wheels.py")
assert spec is not None and spec.loader is not None
package_wheels = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package_wheels)

PY_VERSION = "1.2.3rc4"
DIST_INFO = f"ai_rulez-{PY_VERSION}.dist-info"
EXPECTED = {
    f"ai_rulez-{PY_VERSION}-py3-none-manylinux_2_34_x86_64.whl",
    f"ai_rulez-{PY_VERSION}-py3-none-manylinux_2_31_aarch64.whl",
    f"ai_rulez-{PY_VERSION}-py3-none-macosx_14_0_x86_64.whl",
    f"ai_rulez-{PY_VERSION}-py3-none-macosx_14_0_arm64.whl",
    f"ai_rulez-{PY_VERSION}-py3-none-win_amd64.whl",
}


def record_hash(data: bytes) -> str:
    return "sha256=" + base64.urlsafe_b64encode(hashlib.sha256(data).digest()).rstrip(b"=").decode()


class WheelPackaging(Base):
    def build(self, **kwargs) -> list[Path]:
        make_archives(self.archives, **kwargs)
        return package_wheels.package_all(self.archives, VERSION, self.out)

    def test_five_wheels_with_exact_names(self) -> None:
        wheels = self.build()
        self.assertEqual(len(wheels), 5)
        self.assertEqual({w.name for w in wheels}, EXPECTED)
        self.assertEqual({p.name for p in self.out.iterdir()}, EXPECTED)

    def test_stable_version_is_unchanged(self) -> None:
        self.assertEqual(package_wheels.pep440("5.12.2"), "5.12.2")
        self.assertEqual(package_wheels.pep440("5.12.2-rc.10"), "5.12.2rc10")

    def test_record_hashes_are_valid(self) -> None:
        for wheel in self.build():
            with zipfile.ZipFile(wheel) as zf:
                rows = zf.read(f"{DIST_INFO}/RECORD").decode().splitlines()
                recorded = {}
                for row in rows:
                    name, digest, size = row.rsplit(",", 2)
                    recorded[name] = (digest, size)
                self.assertEqual(set(recorded), set(zf.namelist()), wheel.name)
                self.assertEqual(recorded[f"{DIST_INFO}/RECORD"], ("", ""))
                for name in zf.namelist():
                    if name.endswith("/RECORD"):
                        continue
                    data = zf.read(name)
                    self.assertEqual(recorded[name], (record_hash(data), str(len(data))), name)

    def test_record_hash_check_detects_tampering(self) -> None:
        wheel = next(w for w in self.build() if "win_amd64" in w.name)
        with zipfile.ZipFile(wheel) as zf:
            recorded = {r.split(",")[0]: r.split(",")[1] for r in zf.read(f"{DIST_INFO}/RECORD").decode().splitlines()}
            self.assertNotEqual(recorded["ai_rulez/bin/ai-rulez.exe"], record_hash(b"other"))
            self.assertEqual(recorded["ai_rulez/bin/ai-rulez.exe"], record_hash(zf.read("ai_rulez/bin/ai-rulez.exe")))

    def test_layout_and_modes(self) -> None:
        for wheel in self.build():
            windows = "win_amd64" in wheel.name
            with zipfile.ZipFile(wheel) as zf:
                names = set(zf.namelist())
                binary = "ai_rulez/bin/ai-rulez.exe" if windows else "ai_rulez/bin/ai-rulez"
                for required in (
                    "ai_rulez/__init__.py",
                    "ai_rulez/cli.py",
                    binary,
                    f"{DIST_INFO}/METADATA",
                    f"{DIST_INFO}/WHEEL",
                    f"{DIST_INFO}/entry_points.txt",
                    f"{DIST_INFO}/licenses/LICENSE",
                    f"{DIST_INFO}/licenses/LICENSES/MIT.txt",
                    f"{DIST_INFO}/licenses/THIRD_PARTY_NOTICES.md",
                ):
                    self.assertIn(required, names, wheel.name)
                for info in zf.infolist():
                    self.assertEqual(info.date_time, (1980, 1, 1, 0, 0, 0))
                    self.assertEqual(info.compress_type, zipfile.ZIP_DEFLATED)
                    expected = 0o755 if info.filename == binary and not windows else 0o644
                    self.assertEqual((info.external_attr >> 16) & 0o777, expected, info.filename)
                self.assertEqual(
                    zf.read(f"{DIST_INFO}/entry_points.txt"), b"[console_scripts]\nai-rulez = ai_rulez.cli:main\n"
                )
                self.assertIn(f'__version__ = "{PY_VERSION}"', zf.read("ai_rulez/__init__.py").decode())

    def test_dll_only_when_archive_had_it(self) -> None:
        wheels = self.build()
        for wheel in wheels:
            with zipfile.ZipFile(wheel) as zf:
                dlls = [n for n in zf.namelist() if n.endswith(".dll")]
                self.assertEqual(dlls, ["ai_rulez/bin/liter_llm_ffi.dll"] if "win_amd64" in wheel.name else [])

    def test_no_dll_when_absent(self) -> None:
        for wheel in self.build(dll=False):
            with zipfile.ZipFile(wheel) as zf:
                self.assertEqual([n for n in zf.namelist() if n.endswith(".dll")], [])

    def test_wheel_and_metadata_files(self) -> None:
        wheel = next(w for w in self.build() if "manylinux_2_34_x86_64" in w.name)
        with zipfile.ZipFile(wheel) as zf:
            self.assertEqual(
                zf.read(f"{DIST_INFO}/WHEEL").decode(),
                "Wheel-Version: 1.0\nGenerator: ai-rulez-release\nRoot-Is-Purelib: false\nTag: py3-none-manylinux_2_34_x86_64\n",
            )
            metadata = zf.read(f"{DIST_INFO}/METADATA").decode()
        head, _, body = metadata.partition("\n\n")
        for line in (
            f"Version: {PY_VERSION}",
            "Name: ai-rulez",
            "Requires-Python: >=3.11",
            "License-Expression: MIT AND Apache-2.0 AND Unicode-3.0",
            "Description-Content-Type: text/markdown",
            "License-File: LICENSES/MIT.txt",
        ):
            self.assertIn(line, head.splitlines())
        self.assertIn("# ai-rulez", body)

    def test_metadata_matches_setup_py(self) -> None:
        setup = (HERE.parents[1] / "release/pypi/setup.py").read_text()
        description = re.search(r'description="([^"]*)"', setup)
        assert description is not None
        self.assertEqual(package_wheels.METADATA["Summary"], description.group(1))
        classifiers = re.findall(
            r'^\s+"((?:Development|Intended|Topic|Programming|Operating|Environment)[^"]*)",$', setup, re.M
        )
        self.assertEqual(package_wheels.METADATA["Classifier"], classifiers)
        keywords = re.search(r"keywords=\[(.*?)\]", setup, re.S)
        assert keywords is not None
        self.assertEqual(package_wheels.METADATA["Keywords"], ",".join(re.findall(r'"([^"]+)"', keywords.group(1))))

    def test_glibc_symbol_absent(self) -> None:
        make_archives(self.archives)
        name = _archives.archive_name(VERSION, "linux", "amd64")
        make_tar(
            self.archives / name,
            {
                "ai-rulez": b"\x7fELF no symbols",
                "LICENSE": b"l",
                "README.md": b"r",
                "THIRD_PARTY_NOTICES.md": b"n",
                "LICENSES/MIT.txt": b"m",
            },
        )
        write_checksums(self.archives)
        with self.assertRaises(_archives.ArchiveError) as caught:
            package_wheels.package_all(self.archives, VERSION, self.out)
        self.assertIn("no GLIBC_2.<minor> symbol found", str(caught.exception))

    def test_glibc_uses_max_minor(self) -> None:
        binary = self.tmp / "bin"
        binary.write_bytes(b"GLIBC_2.9 GLIBC_2.17 GLIBC_2.28 GLIBC_2.5")
        self.assertEqual(package_wheels.glibc_minor(binary), 28)


if __name__ == "__main__":
    unittest.main()
