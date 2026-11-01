"""Offline controls for the benchmark reporter; never run benchmark workloads."""

import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "bench.sh"
BASH = shutil.which("bash")


class BenchstatTests(unittest.TestCase):
    def setUp(self) -> None:
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.receipt = self.root / "arguments"
        matches = re.findall(r"(?m)^show_benchstat\(\) \{\n.*?^\}", SCRIPT.read_text(), re.DOTALL)
        self.assertEqual(len(matches), 1, "exercise the single actual reporter function")
        self.function = matches[0]
        self.assertIsNotNone(BASH, "managed CI supplies Bash")
        for name in ["go", "curl"]:
            self.tool(name, 'printf forbidden >> "$FORBIDDEN"; exit 91')

    def tool(self, name: str, body: str) -> None:
        path = self.root / name
        path.write_text("#!/bin/sh\n" + body + "\n")
        path.chmod(0o755)

    def report(self) -> subprocess.CompletedProcess[str]:
        environment = {"PATH": ".", "RECEIPT": "arguments", "FORBIDDEN": "forbidden"}
        if "SYSTEMROOT" in os.environ:
            environment["SYSTEMROOT"] = os.environ["SYSTEMROOT"]
        result = subprocess.run(
            [
                BASH,
                "-euo",
                "pipefail",
                "-c",
                self.function + '\nBASELINE="base spaced.txt"; OUT="head spaced.txt"; show_benchstat',
            ],
            cwd=self.root,
            env=environment,
            capture_output=True,
            text=True,
            timeout=5,
        )
        self.assertFalse((self.root / "forbidden").exists(), "reporter must never bootstrap Go or curl")
        return result

    def test_missing_reporter_fails_with_managed_setup_instruction(self) -> None:
        result = self.report()
        self.assertEqual(result.returncode, 1)
        self.assertEqual(
            result.stderr, "benchstat is required: run mise install --locked go:golang.org/x/perf/cmd/benchstat\n"
        )
        self.assertFalse(self.receipt.exists())

    def test_failing_reporter_preserves_status_and_reports_failure(self) -> None:
        self.tool("benchstat", 'printf "%s\\n" "$@" > "$RECEIPT"; exit 7')
        result = self.report()
        self.assertEqual(result.returncode, 7)
        self.assertEqual(result.stderr, "benchstat report failed (exit 7)\n")
        self.assertEqual(self.receipt.read_text(), "base spaced.txt\nhead spaced.txt\n")

    def test_successful_reporter_receives_exact_paths(self) -> None:
        self.tool("benchstat", 'printf "%s\\n" "$@" > "$RECEIPT"; printf "report ready\\n"')
        result = self.report()
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "report ready\n")
        self.assertEqual(result.stderr, "")
        self.assertEqual(self.receipt.read_text(), "base spaced.txt\nhead spaced.txt\n")


if __name__ == "__main__":
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(BenchstatTests)
    assert suite.countTestCases() == 3
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    assert result.testsRun == 3 and not result.skipped
    raise SystemExit(0 if result.wasSuccessful() else 1)
