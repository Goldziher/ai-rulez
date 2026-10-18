"""Golden and behaviour tests for the SkillSpector test transcoder."""

import importlib.util
import sys
import unittest
from pathlib import Path

HERE = Path(__file__).parent
SPEC = importlib.util.spec_from_file_location(
    "transcode", HERE / "transcode-skillspector-tests.py"
)
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)
DATA = HERE / "testdata" / "skillspector"
UPSTREAM = "0" * 40


class TranscodeTest(unittest.TestCase):
    def render(self, pattern, mapping=None):
        return MODULE.transcode(DATA, pattern, mapping or {}, UPSTREAM)

    def test_matches_golden_file(self):
        text, count = self.render("AR1|DS1|SSRF[12]|SC8", {"AR1": "AR100"})
        self.assertEqual(count, 6)
        self.assertEqual(
            text, (DATA / "sample.golden.go.txt").read_text(encoding="utf-8")
        )

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
        tree = DATA / "test_sample.py"
        self.assertTrue(all(sc.files for sc in MODULE.extract(tree)))


if __name__ == "__main__":
    unittest.main()
