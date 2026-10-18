import copy
import io
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import generate
import tomllib
from hb import fetch, merge, rustenum, tomlout

from tests import helpers

DATA = helpers.HERE.parent.parent / "internal" / "harnessnames" / "data"

TABLE = {
    "schema": 1,
    "preset": "demo",
    "harness_version": "v1",
    "verified_on": "2026-01-01",
    "completeness": "complete",
    "severity": "warning",
    "invocation_spaces": [["command", "skill"], ["agent"]],
    "namespaced": False,
    "source": [
        {"id": "s1", "url": "https://github.com/o/r/blob/v1/src/cmds.rs#L10", "verified_on": "2026-01-01"},
        {"id": "s2", "url": "https://example.com/doc.md", "quote": "q", "verified_on": "2026-01-01"},
    ],
    "group": [
        {"kind": "command", "source": "s1", "user_collision": "builtin_wins", "match": "exact", "names": ["a", "b"]},
        {
            "kind": "command",
            "source": "s1",
            "user_collision": "user_wins",
            "match": "exact",
            "names": ["c"],
            "evidence": "s2",
        },
    ],
}
PIN = {
    "extractor": "demo",
    "repo": "o/r",
    "ref": "v2",
    "harness_version": "v2",
    "target": [
        {"key": "x", "kind": "command", "collision": "builtin_wins", "exclude": ["Hidden"], "add": ["z"]},
        {"key": "y", "kind": "command", "collision": "user_wins"},
    ],
}


class TomlOutTest(unittest.TestCase):
    def test_every_data_file_round_trips_byte_for_byte(self):
        for path in sorted(DATA.glob("*.toml")):
            with self.subTest(path.name):
                text = path.read_text(encoding="utf-8")
                self.assertEqual(tomlout.dump(tomllib.loads(text)), text)


class QuoteStringTest(unittest.TestCase):
    def test_control_characters_and_quotes_survive_a_round_trip(self) -> None:
        for text in ("a\nb", 'say "x"', "back\\slash", "tab\there", "bell\x07", "plain"):
            with self.subTest(text):
                self.assertEqual(tomllib.loads("v = " + tomlout.q(text))["v"], text)


class MergeTest(unittest.TestCase):
    def results(self):
        return {
            "x": {"names": ["b", "a", "hidden", "n"], "aliases": {"a": ["al", "Hidden"], "gone": ["q"]}},
            "y": {"names": ["c"]},
        }

    def test_merge_replaces_selected_groups_and_keeps_curated_fields(self):
        # Arrange
        table = copy.deepcopy(TABLE)
        # Act
        got = merge.merge(table, PIN, self.results())
        # Assert
        self.assertEqual(got["group"][0]["names"], ["a", "b", "n", "z"])
        self.assertEqual(got["group"][0]["aliases"], {"a": ["al"]})
        self.assertEqual(got["group"][1]["evidence"], "s2")
        self.assertEqual(got["group"][0]["user_collision"], "builtin_wins")
        self.assertEqual(got["harness_version"], "v2")
        self.assertEqual(got["source"][0]["url"], "https://github.com/o/r/blob/v2/src/cmds.rs")
        self.assertEqual(table, TABLE, "the input table is not modified")

    def test_merge_fails_when_a_target_matches_no_group_or_several(self):
        for target in ({"key": "x", "kind": "skill"}, {"key": "x", "kind": "command"}):
            with self.subTest(target):
                pin = {**PIN, "target": [target]}
                with self.assertRaises(merge.MergeError):
                    merge.merge(TABLE, pin, {"x": {"names": ["a"]}})

    def test_merge_fails_on_a_result_without_target_and_on_empty_names(self):
        res = self.results()
        with self.assertRaises(merge.MergeError):
            merge.merge(TABLE, PIN, {**res, "extra": {"names": ["n"]}})
        with self.assertRaises(merge.MergeError):
            merge.merge(TABLE, PIN, {**res, "y": {"names": []}})

    def test_merge_fails_on_a_name_listed_in_two_groups(self):
        res = self.results()
        res["y"] = {"names": ["a"]}
        with self.assertRaises(merge.MergeError):
            merge.merge(TABLE, PIN, res)

    def test_tree_urls_follow_the_ref(self):
        table = copy.deepcopy(TABLE)
        table["source"][0]["url"] = "https://github.com/o/r/tree/v1/dir"
        merge.apply_ref(table, PIN)
        self.assertEqual(table["source"][0]["url"], "https://github.com/o/r/tree/v2/dir")

    def test_diff_names_lists_additions_removals_and_ref_changes(self):
        new = merge.merge(TABLE, PIN, self.results())
        lines = merge.diff_names(TABLE, new)
        self.assertIn("group 1 (command/builtin_wins): + n, z", lines)
        self.assertTrue(any(line.startswith("source s1:") for line in lines))


class RustEnumTest(unittest.TestCase):
    SRC = """
    pub enum Cmd {
        // comment, with comma
        Model,
        #[strum(serialize = "setup")]
        ElevateSandbox,
        #[strum(to_string = "pwd", serialize = "cwd")]
        Pwd,
        #[strum(disabled)]
        Hidden,
        DebugConfig, /* trailing */
    }
    """

    def test_names_aliases_and_disabled_variants(self):
        got = rustenum.commands(self.SRC, "Cmd")
        self.assertEqual(got["names"], ["model", "setup", "pwd", "debug-config"])
        self.assertEqual(got["aliases"], {"pwd": ["cwd"]})

    def test_missing_enum_is_an_error(self):
        with self.assertRaises(ValueError):
            rustenum.commands(self.SRC, "Other")


class FetchTest(unittest.TestCase):
    def test_only_plain_https_urls(self):
        for url in ("http://example.com/x", "file:///etc/passwd", "https://user:pw@example.com/x"):
            with self.subTest(url), self.assertRaises(fetch.FetchError):
                fetch.http_get(url)

    def test_frontmatter_field(self):
        text = '---\nname: "pdf"\ndescription: d\n---\nbody\nname: no\n'
        self.assertEqual(fetch.frontmatter_field(text, "name"), "pdf")
        self.assertIsNone(fetch.frontmatter_field("no front matter", "name"))


class GenerateTest(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(self.tmp))
        (self.tmp / "data").mkdir()
        (self.tmp / "data" / "demo.toml").write_text(tomlout.dump(TABLE), encoding="utf-8", newline="\n")
        (self.tmp / "data" / "other.toml").write_text(
            tomlout.dump({**TABLE, "preset": "other"}), encoding="utf-8", newline="\n"
        )
        self.pins = self.tmp / "pins.toml"
        self.pins.write_text(
            '[demo]\nextractor = "codex"\nrepo = "o/r"\nref = "v1"\n'
            '[[demo.target]]\nkey = "x"\nkind = "command"\ncollision = "builtin_wins"\n'
            '[[demo.target]]\nkey = "y"\nkind = "command"\ncollision = "user_wins"\n'
            '[other]\nextractor = "none"\nreason = "no public registry"\n',
            encoding="utf-8",
            newline="\n",
        )

    def run_main(self, *args, results=None):
        out = io.StringIO()
        res = results if results is not None else {"x": {"names": ["a", "b"]}, "y": {"names": ["c"]}}
        argv = [*args, "--data", str(self.tmp / "data"), "--pins", str(self.pins), "--today", "2026-02-02"]
        with (
            mock.patch.object(generate, "run_extractor", return_value=res),
            mock.patch("sys.stderr", new_callable=io.StringIO) as err,
        ):
            code = generate.main(argv, out)
        return code, out.getvalue(), err.getvalue()

    def test_check_passes_when_the_data_matches(self):
        code, out, _ = self.run_main("--check")
        self.assertEqual(code, 0)
        self.assertIn("demo: up to date", out)

    def test_check_fails_with_exit_1_and_names_the_difference(self):
        code, out, _ = self.run_main("--check", results={"x": {"names": ["a", "b", "new"]}, "y": {"names": ["c"]}})
        self.assertEqual(code, 1)
        self.assertIn("+ new", out)

    def test_write_updates_names_and_dates_then_check_passes(self):
        res = {"x": {"names": ["a", "new"]}, "y": {"names": ["c"]}}
        self.assertEqual(self.run_main("--write", results=res)[0], 0)
        written = tomllib.loads((self.tmp / "data" / "demo.toml").read_text(encoding="utf-8"))
        self.assertEqual(written["group"][0]["names"], ["a", "new"])
        self.assertEqual(written["verified_on"], "2026-02-02")
        self.assertEqual(self.run_main("--check", results=res)[0], 0)

    def test_extractor_failure_is_exit_2(self):
        with (
            mock.patch.object(generate, "run_extractor", side_effect=fetch.FetchError("boom")),
            mock.patch("sys.stderr", new_callable=io.StringIO),
        ):
            code = generate.main(["--check", "--data", str(self.tmp / "data"), "--pins", str(self.pins)], io.StringIO())
        self.assertEqual(code, 2)

    def test_a_data_file_without_a_pin_is_exit_2(self):
        (self.tmp / "data" / "third.toml").write_text(
            tomlout.dump({**TABLE, "preset": "third"}), encoding="utf-8", newline="\n"
        )
        code, _, err = self.run_main("--check")
        self.assertEqual(code, 2)
        self.assertIn("third", err)

    def test_none_needs_a_reason_and_unknown_keys_are_rejected(self):
        for text in ('[demo]\nextractor = "none"\n', '[demo]\nextractor = "none"\nreason = "r"\nbogus = 1\n'):
            self.pins.write_text(text, encoding="utf-8", newline="\n")
            with self.subTest(text), self.assertRaises(generate.PinError):
                generate.load_pins(self.pins)

    def test_set_pin_ref_changes_only_the_named_table(self):
        text = '[a]\nref = "v1"\n[b]\nref = "v1"\n'
        self.assertEqual(generate.set_pin_ref(text, "b", "v1", "v2"), '[a]\nref = "v1"\n[b]\nref = "v2"\n')

    def test_the_committed_pins_cover_every_data_file(self) -> None:
        pins = generate.load_pins(helpers.HERE / "pins.toml")
        self.assertEqual(generate.check_coverage(pins, DATA), [])


if __name__ == "__main__":
    unittest.main()
