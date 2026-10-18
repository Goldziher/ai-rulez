import io
import tempfile
import unittest
from pathlib import Path

import check_sources
from hb import fetch, tomlout

from tests.test_framework import TABLE

PAGE = "Built-in commands win.\n  A   skill replaces the built-in."


def table(quote, outcome="user_wins", preset="demo"):
    t = {**TABLE, "preset": preset}
    t["source"] = [
        {"id": "s1", "url": "https://example.com/a.md", "verified_on": "2026-01-01"},
        {"id": "s2", "url": "https://example.com/b.md", "quote": quote, "verified_on": "2026-01-01"},
    ]
    t["group"] = [
        {
            "kind": "command",
            "source": "s1",
            "evidence": "s2",
            "user_collision": outcome,
            "match": "exact",
            "names": ["a"],
        }
    ]
    return t


class CheckSourcesTest(unittest.TestCase):
    def run_check(self, tables, pages, *args):
        tmp = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(tmp))
        for name, t in tables.items():
            tomlout.save(tmp / f"{name}.toml", t)

        def get(url):
            if url not in pages:
                raise fetch.FetchError(f"{url}: HTTP 404")
            return pages[url]

        out = io.StringIO()
        code = check_sources.main(["--data", str(tmp), "--today", "2026-02-02", *args], out, get)
        return code, out.getvalue(), tmp

    def test_a_quote_on_the_page_passes_whitespace_insensitively(self):
        code, out, _ = self.run_check(
            {"demo": table("A skill replaces the built-in.")}, {"https://example.com/b.md": PAGE}
        )
        self.assertEqual(code, 0)
        self.assertIn("demo", out)

    def test_a_missing_quote_is_exit_1(self):
        code, out, _ = self.run_check({"demo": table("never said")}, {"https://example.com/b.md": PAGE})
        self.assertEqual(code, 1)
        self.assertIn("quote MISSING", out)

    def test_an_outcome_without_a_quote_is_exit_1(self):
        t = table("x")
        del t["source"][1]["quote"]
        code, out, _ = self.run_check({"demo": t}, {})
        self.assertEqual(code, 1)
        self.assertIn("records an outcome without a quote", out)

    def test_unknown_outcomes_need_no_quote(self):
        t = table("x", outcome="unknown")
        del t["source"][1]["quote"]
        self.assertEqual(self.run_check({"demo": t}, {})[0], 0)

    def test_an_unreachable_page_is_exit_2(self):
        self.assertEqual(self.run_check({"demo": table("q")}, {})[0], 2)

    def test_stamp_dates_only_the_sources_whose_quote_was_found(self):
        tables = {"ok": table("A skill replaces the built-in.", preset="ok")}
        code, _, tmp = self.run_check(tables, {"https://example.com/b.md": PAGE}, "--stamp")
        self.assertEqual(code, 0)
        got = tomlout.load(tmp / "ok.toml")
        self.assertEqual([s["verified_on"] for s in got["source"]], ["2026-01-01", "2026-02-02"])


if __name__ == "__main__":
    unittest.main()
