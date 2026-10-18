import unittest

from tests import helpers


class TaktTest(unittest.TestCase):
    def setUp(self):
        self.ex = helpers.load_extractor("takt")
        self.paths = [
            line for line in helpers.read("takt", "tree.txt").splitlines() if line and not line.startswith("#")
        ]

    def test_facet_names_are_the_file_stems_of_the_directory(self):
        # Arrange / Act
        got = self.ex.facet_names(self.paths, "instructions")
        # Assert
        self.assertEqual(len(got), 3)
        self.assertIn("adjudicate-review-findings", got)
        self.assertNotIn("fallback-notice", got)

    def test_facet_names_ignore_other_extensions_and_nested_files(self):
        # Arrange
        base = self.ex.FACETS + "knowledge/"
        paths = [base + "a.md", base + "b.yaml", base + "nested/c.md", self.ex.FACETS + "policies/d.md"]
        # Act
        got = self.ex.facet_names(paths, "knowledge")
        # Assert
        self.assertEqual(got, ["a"])

    def test_extract_maps_the_three_facet_types_to_the_three_kinds(self):
        # Arrange
        ctx = helpers.context("nrslib/takt", "c1", tree=self.paths)
        # Act
        got = self.ex.extract(ctx)
        # Assert
        self.assertEqual(sorted(got), ["agent", "command", "skill"])
        self.assertIn("architecture", got["skill"]["names"])
        self.assertIn("ai-antipattern-reviewer", got["agent"]["names"])

    def test_a_missing_directory_is_an_error_not_an_empty_result(self):
        # Arrange / Act / Assert
        with self.assertRaises(ValueError):
            self.ex.facet_names(self.paths, "personas-renamed")


if __name__ == "__main__":
    unittest.main()
