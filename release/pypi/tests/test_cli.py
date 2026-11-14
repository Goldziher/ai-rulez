import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from ai_rulez import cli  # noqa: E402


def make_package(binary_name):
    root = tempfile.mkdtemp()
    if binary_name:
        bin_dir = Path(root) / "bin"
        bin_dir.mkdir()
        (bin_dir / binary_name).write_text("")
    return root


class FindBundledBinaryTest(unittest.TestCase):
    def test_posix_name(self):
        root = make_package("ai-rulez")
        self.assertEqual(
            cli.find_bundled_binary(root, "linux"), os.path.join(root, "bin", "ai-rulez")
        )

    def test_windows_name(self):
        root = make_package("ai-rulez.exe")
        self.assertEqual(
            cli.find_bundled_binary(root, "win32"),
            os.path.join(root, "bin", "ai-rulez.exe"),
        )

    def test_absent(self):
        self.assertIsNone(cli.find_bundled_binary(make_package(None), "linux"))

    def test_wrong_name_for_system(self):
        self.assertIsNone(cli.find_bundled_binary(make_package("ai-rulez"), "win32"))


class MainTest(unittest.TestCase):
    def run_main(self, bundled, returncode=0):
        with mock.patch.object(cli, "find_bundled_binary", return_value=bundled), \
            mock.patch.object(cli, "ensure_binary", return_value="/cache/ai-rulez") as ensure, \
            mock.patch.object(cli.subprocess, "run") as run, \
            mock.patch.object(sys, "argv", ["ai-rulez", "--version"]):
            run.return_value.returncode = returncode
            with self.assertRaises(SystemExit) as exit_info:
                cli.main()
        return ensure, run, exit_info.exception.code

    def test_bundled_preferred(self):
        ensure, run, code = self.run_main("/site/ai_rulez/bin/ai-rulez", 3)
        ensure.assert_not_called()
        run.assert_called_once_with(["/site/ai_rulez/bin/ai-rulez", "--version"], check=False)
        self.assertEqual(code, 3)

    def test_fallback_when_absent(self):
        ensure, run, code = self.run_main(None)
        ensure.assert_called_once_with()
        run.assert_called_once_with(["/cache/ai-rulez", "--version"], check=False)
        self.assertEqual(code, 0)

    def test_signal_exit_maps_to_128_plus_signal(self):
        _, _, code = self.run_main("/b", -9)
        self.assertEqual(code, 137)


if __name__ == "__main__":
    unittest.main()
