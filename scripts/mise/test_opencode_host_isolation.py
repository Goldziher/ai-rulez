"""Offline controls for the optional host system-configuration preflight."""

import importlib.util
import os
import sys
import tempfile
import types
import unittest
from pathlib import Path
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("opencode_host", Path(__file__).with_name("test_opencode_host.py"))
assert SPEC is not None and SPEC.loader is not None
HOST = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(HOST)
ACTUAL_USER = "os-account"
ACTUAL_PATH = Path("/Library/Managed Preferences") / ACTUAL_USER / "ai.opencode.managed.plist"


class SystemIsolationTests(unittest.TestCase):
    def test_spoofed_login_cannot_hide_actual_account_configuration(self) -> None:
        login = dict.fromkeys(["LOGNAME", "USER", "LNAME", "USERNAME"], "spoofed-login")
        account = types.SimpleNamespace(getpwuid=lambda _uid: types.SimpleNamespace(pw_name=ACTUAL_USER))
        seen = []

        def exists(path: Path) -> bool:
            seen.append(path)
            return path == ACTUAL_PATH

        with (
            patch.dict(os.environ, login),
            patch.dict(sys.modules, {"pwd": account}),
            patch.object(HOST.sys, "platform", "darwin"),
            patch.object(Path, "exists", exists),
            patch.object(
                HOST, "checked", side_effect=AssertionError("forbidden external command before isolation refusal")
            ) as launch,
            tempfile.TemporaryDirectory() as directory,
        ):
            with self.assertRaisesRegex(RuntimeError, "Refusing to read outside host configuration") as error:
                HOST.run(Path(directory))
            self.assertIn(str(ACTUAL_PATH), str(error.exception))
            self.assertIn(ACTUAL_PATH, seen)
            launch.assert_not_called()

    def test_absent_system_configuration_allows_preflight(self) -> None:
        account = types.SimpleNamespace(getpwuid=lambda _uid: types.SimpleNamespace(pw_name=ACTUAL_USER))
        seen = []

        def absent(path: Path) -> bool:
            seen.append(path)
            return False

        with (
            patch.dict(sys.modules, {"pwd": account}),
            patch.object(HOST.sys, "platform", "darwin"),
            patch.object(Path, "exists", absent),
        ):
            HOST.check_system_isolation()
        self.assertEqual(
            seen,
            [
                Path("/Library/Application Support/opencode"),
                Path("/Library/Managed Preferences/ai.opencode.managed.plist"),
                ACTUAL_PATH,
            ],
        )


if __name__ == "__main__":
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(SystemIsolationTests)
    assert suite.countTestCases() == 2
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    assert result.testsRun == 2 and not result.skipped
    raise SystemExit(0 if result.wasSuccessful() else 1)
