"""Golden and behaviour tests for the SkillSpector test transcoder."""

import contextlib
import importlib.util
import io
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).parent
SPEC = importlib.util.spec_from_file_location("transcode", HERE / "transcode-skillspector-tests.py")
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)
DATA = HERE / "testdata" / "skillspector"
UPSTREAM = "0" * 40
# The fixture is stored as test_sample.py.txt so pytest does not collect it as a test module;
# setUpModule copies it under the name the transcoder looks for (test_*.py).
TESTS = None


def setUpModule():
    global TESTS
    tmp = tempfile.TemporaryDirectory()
    unittest.addModuleCleanup(tmp.cleanup)
    TESTS = Path(tmp.name) / "tests"
    TESTS.mkdir()
    (TESTS / "test_sample.py").write_bytes((DATA / "test_sample.py.txt").read_bytes())


class TranscodeTest(unittest.TestCase):
    def render(self, pattern, mapping=None):
        return MODULE.transcode(TESTS, pattern, mapping or {}, UPSTREAM)

    def test_matches_golden_file(self):
        text, count = self.render("AR1|DS1|SSRF[12]|SC8", {"AR1": "AR100"})
        self.assertEqual(count, 6)
        self.assertEqual(text, (DATA / "sample.golden.go.txt").read_text(encoding="utf-8"))

    def test_header_marks_modification_and_upstream(self):
        text, _ = self.render("DS1")
        self.assertIn("SPDX-License-Identifier: Apache-2.0", text)
        self.assertIn(f"commit {UPSTREAM}", text)
        self.assertIn("Modified by the ai-rulez authors", text)
        self.assertIn("tests/test_sample.py", text)

    def test_unmatched_family_yields_no_rows(self):
        self.assertEqual(self.render("ZZ9")[1], 0)

    def test_rule_without_mapping_keeps_upstream_id(self):
        text, _ = self.render("AR1")
        self.assertIn('"AR1:SKILL.md:0"', text)

    def test_parametrized_rows_are_numbered(self):
        text, _ = self.render("AR1", {"AR1": "AR100"})
        self.assertIn('"negative parametrized 1"', text)
        self.assertIn('"negative parametrized 2"', text)

    def test_negative_membership_becomes_absent(self):
        text, _ = self.render("SSRF[12]")
        self.assertIn('absent: []string{"SSRF2"}', text)

    def test_go_string_escapes_bytes_and_controls(self):
        self.assertEqual(MODULE.go_string(b"\x00\xffa"), '"\\x00\\xffa"')
        self.assertEqual(MODULE.go_string("a\u200bb\n"), '"a\\u200bb\\n"')

    def test_go_string_escapes_byte_order_mark_and_combining_marks(self):
        self.assertEqual(MODULE.go_string("\ufeffi\u034f"), '"\\ufeffi\\u034f"')

    def test_unresolvable_content_is_skipped(self):
        tree = TESTS / "test_sample.py"
        self.assertTrue(all(sc.files for sc in MODULE.extract(tree)))


def git(cwd, *args):
    subprocess.run(
        ["git", "-c", "user.name=t", "-c", "user.email=t@example.com", *args],
        cwd=cwd,
        check=True,
        capture_output=True,
    )


def init_repo(path):
    path.mkdir(parents=True, exist_ok=True)
    git(path, "init", "-q")
    (path / "f").write_text("x")
    git(path, "add", "f")
    git(path, "commit", "-q", "-m", "init")


class UpstreamCommitTest(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name).resolve()

    def run_main(self, tests, *extra):
        args = ["--tests", str(tests), "--rule", "ZZ9", *extra]
        with open(self.root / "err", "w") as sink:
            old = sys.stdout, sys.stderr
            sys.stdout = sys.stderr = sink
            try:
                return MODULE.main(args)
            finally:
                sys.stdout, sys.stderr = old

    def test_checkout_root_yields_full_sha(self):
        repo = self.root / "ss"
        init_repo(repo)
        (repo / "tests").mkdir()
        sha = MODULE.upstream_commit(repo / "tests")
        self.assertRegex(sha, r"^[0-9a-f]{40}$")

    def test_non_git_directory_is_an_error(self):
        (self.root / "plain" / "tests").mkdir(parents=True)
        with self.assertRaises(MODULE.UpstreamError):
            MODULE.upstream_commit(self.root / "plain" / "tests")
        self.assertEqual(self.run_main(self.root / "plain" / "tests"), 2)

    def test_copy_nested_in_another_repo_is_an_error(self):
        init_repo(self.root / "other")
        (self.root / "other" / "copy" / "tests").mkdir(parents=True)
        with self.assertRaises(MODULE.UpstreamError):
            MODULE.upstream_commit(self.root / "other" / "copy" / "tests")
        self.assertEqual(self.run_main(self.root / "other" / "copy" / "tests"), 2)

    def test_short_or_malformed_upstream_is_an_error(self):
        for bad in ("abc1234", "UNKNOWN", "G" * 40, "A" * 40):
            with self.subTest(bad=bad):
                self.assertEqual(self.run_main(TESTS, "--upstream", bad), 2)

    def test_explicit_full_sha_is_accepted(self):
        self.assertEqual(self.run_main(TESTS, "--upstream", UPSTREAM), 0)


class ParseMappingTest(unittest.TestCase):
    def test_valid_entries_are_parsed(self):
        self.assertEqual(
            MODULE.parse_mapping(["DS1=AR044", " SC10 = AR056 "]),
            {"DS1": "AR044", "SC10": "AR056"},
        )

    def test_malformed_entry_is_a_value_error(self):
        for bad in ("DS1", "DS1=", "=AR044", "DS1=AR44", "ds1=AR044", "DS1=AR044=x"):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                MODULE.parse_mapping([bad])

    def test_main_reports_malformed_map_as_usage_error(self):
        err = io.StringIO()
        args = ["--tests", str(TESTS), "--rule", "DS1", "--map", "DS1", "--upstream", UPSTREAM]
        with contextlib.redirect_stderr(err), self.assertRaises(SystemExit) as ctx:
            MODULE.main(args)
        self.assertEqual(ctx.exception.code, 2)
        self.assertIn("SSID=ARNNN", err.getvalue())


if __name__ == "__main__":
    unittest.main()
