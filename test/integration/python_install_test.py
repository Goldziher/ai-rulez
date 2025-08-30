import os
import platform
import shutil
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


class PythonInstallationTest(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.mkdtemp(prefix="ai-rulez-python-test-")
        self.python_package_dir = os.path.join(self.temp_dir, "python-package")

        build_dir = os.path.join(os.path.dirname(__file__), "../../build/python")
        shutil.copytree(build_dir, self.python_package_dir)

        sys.path.insert(0, self.python_package_dir)

    def tearDown(self):
        if sys.path[0] == self.python_package_dir:
            sys.path.pop(0)
        if os.path.exists(self.temp_dir):
            shutil.rmtree(self.temp_dir, ignore_errors=True)

    def test_platform_detection(self):
        from ai_rulez.downloader import get_platform

        platform_name, arch = get_platform()

        self.assertIn(platform_name, ["darwin", "linux", "windows"])
        self.assertIn(arch, ["amd64", "arm64", "386"])

        current_system = platform.system().lower()
        if current_system == "darwin":
            self.assertEqual(platform_name, "darwin")
        elif current_system == "linux":
            self.assertEqual(platform_name, "linux")
        elif current_system == "windows":
            self.assertEqual(platform_name, "windows")

    def test_binary_url_generation(self):
        from ai_rulez.downloader import get_binary_url, get_checksums_url

        version = "1.0.0"
        binary_url = get_binary_url(version)
        checksums_url = get_checksums_url(version)

        self.assertIn("github.com/Goldziher/ai-rulez", binary_url)
        self.assertIn(f"v{version}", binary_url)
        self.assertIn("github.com/Goldziher/ai-rulez", checksums_url)
        self.assertIn("checksums.txt", checksums_url)

    def test_checksum_calculation(self):
        from ai_rulez.downloader import calculate_sha256

        test_file = os.path.join(self.temp_dir, "test.txt")
        test_content = b"Hello, World!"

        with open(test_file, "wb") as f:
            f.write(test_content)

        checksum = calculate_sha256(test_file)

        self.assertEqual(len(checksum), 64)
        self.assertTrue(all(c in "0123456789abcdef" for c in checksum))

        checksum2 = calculate_sha256(test_file)
        self.assertEqual(checksum, checksum2)

    def test_checksum_parsing(self):
        from ai_rulez.downloader import get_expected_checksum

        checksums_content = """
abc123  ai-rulez_1.0.0_linux_amd64.tar.gz
def456  ai-rulez_1.0.0_windows_amd64.zip
789ghi  ai-rulez_1.0.0_darwin_amd64.tar.gz
        """.strip()

        linux_hash = get_expected_checksum(checksums_content, "ai-rulez_1.0.0_linux_amd64.tar.gz")
        windows_hash = get_expected_checksum(checksums_content, "ai-rulez_1.0.0_windows_amd64.zip")
        nonexistent_hash = get_expected_checksum(checksums_content, "nonexistent.tar.gz")

        self.assertEqual(linux_hash, "abc123")
        self.assertEqual(windows_hash, "def456")
        self.assertIsNone(nonexistent_hash)

    def test_binary_path_generation(self):
        from ai_rulez.downloader import get_binary_path

        binary_path = get_binary_path()

        self.assertIn(".cache", str(binary_path))
        self.assertIn("ai-rulez", str(binary_path))

        if platform.system().lower() == "windows":
            self.assertTrue(str(binary_path).endswith(".exe"))
        else:
            self.assertFalse(str(binary_path).endswith(".exe"))

    def test_binary_verification(self):
        from ai_rulez.downloader import verify_binary

        nonexistent_path = os.path.join(self.temp_dir, "nonexistent")
        self.assertFalse(verify_binary(nonexistent_path))

        empty_file = os.path.join(self.temp_dir, "empty")
        Path(empty_file).touch()
        self.assertFalse(verify_binary(empty_file))

        non_exec_file = os.path.join(self.temp_dir, "non_exec")
        with open(non_exec_file, "w") as f:
            f.write("not executable")
        self.assertFalse(verify_binary(non_exec_file))

    def test_version_cache_management(self):
        from ai_rulez.downloader import (
            is_binary_current_version,
            update_cache_version,
        )

        with patch("ai_rulez.downloader.__version__", "1.0.0"):
            self.assertFalse(is_binary_current_version())

            update_cache_version()

            self.assertTrue(is_binary_current_version())

            with patch("ai_rulez.downloader.__version__", "2.0.0"):
                self.assertFalse(is_binary_current_version())

    def test_download_error_handling(self):
        from ai_rulez.downloader import download_file_with_retries

        invalid_url = "https://nonexistent.domain/file.txt"
        dest_path = os.path.join(self.temp_dir, "download_test")

        with self.assertRaises(RuntimeError):
            download_file_with_retries(invalid_url, dest_path, "test file")

    @patch("ai_rulez.downloader.download_and_verify_binary")
    @patch("ai_rulez.downloader.verify_binary")
    @patch("ai_rulez.downloader.is_binary_current_version")
    def test_ensure_binary_flow(self, mock_is_current, mock_verify, mock_download):
        from ai_rulez.downloader import ensure_binary

        mock_is_current.return_value = True
        mock_verify.return_value = True

        with patch("ai_rulez.downloader.get_binary_path") as mock_path:
            mock_path.return_value = Path("/mock/path/ai-rulez")
            mock_path.return_value.exists.return_value = True

            result = ensure_binary()
            self.assertEqual(result, "/mock/path/ai-rulez")
            mock_download.assert_not_called()

    def test_cli_integration(self):
        mock_binary = os.path.join(self.temp_dir, "ai-rulez")
        if platform.system().lower() == "windows":
            mock_binary += ".exe"

        mock_script = """#!/bin/bash
if [ "$1" = "--version" ]; then
    echo "ai-rulez 1.0.0"
    exit 0
fi
echo "Mock ai-rulez called with: $@"
"""

        with open(mock_binary, "w") as f:
            f.write(mock_script)
        os.chmod(mock_binary, 0o755)

        from ai_rulez.cli import main

        with patch("ai_rulez.downloader.ensure_binary", return_value=mock_binary):
            with patch("sys.argv", ["ai-rulez", "--version"]):
                try:
                    main()
                except SystemExit:
                    pass


def run_integration_tests():
    unittest.main(verbosity=2, exit=False)


if __name__ == "__main__":
    run_integration_tests()
