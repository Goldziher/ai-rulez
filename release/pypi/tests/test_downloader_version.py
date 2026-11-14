import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from ai_rulez import downloader  # noqa: E402


class ReleaseVersionTest(unittest.TestCase):
    def test_cases(self):
        for given, want in [
            ("5.1.0", "5.1.0"),
            ("5.1.0rc1", "5.1.0-rc.1"),
            ("5.1.0rc12", "5.1.0-rc.12"),
            ("5.1.0-rc.1", "5.1.0-rc.1"),
        ]:
            self.assertEqual(downloader.release_version(given), want, given)


class UrlTest(unittest.TestCase):
    def setUp(self):
        patcher = mock.patch.object(downloader, "get_platform", return_value=("linux", "amd64"))
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_binary_url_uses_semver_tag_and_archive(self):
        self.assertEqual(
            downloader.get_binary_url("5.1.0rc1"),
            "https://github.com/Goldziher/ai-rulez/releases/download/v5.1.0-rc.1/"
            "ai-rulez_5.1.0-rc.1_linux_amd64.tar.gz",
        )

    def test_checksums_url(self):
        self.assertEqual(
            downloader.get_checksums_url("5.1.0rc1"),
            "https://github.com/Goldziher/ai-rulez/releases/download/v5.1.0-rc.1/checksums.txt",
        )

    def test_stable_unchanged(self):
        self.assertIn("/v5.12.2/ai-rulez_5.12.2_linux_amd64.tar.gz", downloader.get_binary_url("5.12.2"))


if __name__ == "__main__":
    unittest.main()
